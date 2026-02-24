// Package service provides the core polling service that orchestrates
// reading data from devices and publishing to MQTT.
package service

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/rs/zerolog"
)

func sanitizeTopicSegment(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "#", "_")
	s = strings.ReplaceAll(s, "+", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.Trim(s, "_")
	return s
}

func topicForTag(prefix string, tag *domain.Tag) string {
	suffix := strings.TrimSpace(tag.TopicSuffix)
	if suffix == "" {
		suffix = tag.Name
	}
	if strings.TrimSpace(suffix) == "" {
		suffix = tag.ID
	}
	suffix = sanitizeTopicSegment(suffix)
	if suffix == "" {
		return prefix
	}
	return prefix + "/" + suffix
}

// dataPointPool reduces GC pressure by recycling slices
var dataPointPool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate with common capacity
		slice := make([]*domain.DataPoint, 0, 64)
		return &slice
	},
}

// Publisher interface defines the methods needed for publishing data.
type Publisher interface {
	Publish(ctx context.Context, dataPoint *domain.DataPoint) error
	PublishBatch(ctx context.Context, dataPoints []*domain.DataPoint) error
}

// SubscriptionHandler handles push-based data delivery for protocols that
// support server-side subscriptions (e.g., OPC UA Report-by-Exception).
// When a device is configured for subscriptions, the polling service delegates
// to this handler instead of running a polling loop.
type SubscriptionHandler interface {
	// Subscribe sets up server-side subscriptions for a device's tags.
	// The onData callback is invoked for each data point pushed by the server.
	Subscribe(ctx context.Context, device *domain.Device, tags []*domain.Tag, onData func(*domain.DataPoint)) error

	// Unsubscribe removes all subscriptions for a device.
	Unsubscribe(deviceID string) error
}

// PollingService orchestrates reading data from devices and publishing to MQTT.
// It supports multiple protocols through the ProtocolManager.
// For OPC UA devices with OPCUseSubscriptions=true, it delegates to a
// SubscriptionHandler for push-based data delivery instead of polling.
type PollingService struct {
	config              PollingConfig
	protocolManager     *domain.ProtocolManager
	publisher           Publisher
	subscriptionHandler SubscriptionHandler // Optional: handles OPC UA subscriptions
	logger              zerolog.Logger
	metrics             *metrics.Registry
	devices             map[string]*devicePoller
	mu                  sync.RWMutex
	started             atomic.Bool
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	workerPool          chan struct{}
	stats               *PollingStats
}

// PollingConfig holds configuration for the polling service.
type PollingConfig struct {
	WorkerCount     int
	BatchSize       int
	DefaultInterval time.Duration
	MaxRetries      int
	ShutdownTimeout time.Duration
}

// PollingStats tracks polling statistics.
type PollingStats struct {
	TotalPolls      atomic.Uint64
	SuccessPolls    atomic.Uint64
	FailedPolls     atomic.Uint64
	SkippedPolls    atomic.Uint64 // Polls skipped due to back-pressure
	PointsRead      atomic.Uint64
	PointsPublished atomic.Uint64
}

// devicePoller manages polling for a single device.
type devicePoller struct {
	device     *domain.Device
	stopChan   chan struct{}
	stopOnce   sync.Once
	running    atomic.Bool
	subscribed bool // true if using push-based subscriptions instead of polling
	lastPoll   time.Time
	lastError  error
	stats      deviceStats
	mu         sync.RWMutex
}

// deviceStats tracks per-device statistics.
type deviceStats struct {
	pollCount    atomic.Uint64
	errorCount   atomic.Uint64
	skippedCount atomic.Uint64 // Back-pressure skips
	pointsRead   atomic.Uint64
}

// NewPollingService creates a new polling service.
func NewPollingService(
	config PollingConfig,
	protocolManager *domain.ProtocolManager,
	publisher Publisher,
	logger zerolog.Logger,
	metricsReg *metrics.Registry,
) *PollingService {
	// Apply defaults
	if config.WorkerCount <= 0 {
		config.WorkerCount = 10
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.DefaultInterval <= 0 {
		config.DefaultInterval = 1 * time.Second
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 30 * time.Second
	}

	return &PollingService{
		config:          config,
		protocolManager: protocolManager,
		publisher:       publisher,
		logger:          logger.With().Str("component", "polling-service").Logger(),
		metrics:         metricsReg,
		devices:         make(map[string]*devicePoller),
		workerPool:      make(chan struct{}, config.WorkerCount),
		stats:           &PollingStats{},
	}
}

// SetSubscriptionHandler sets the handler for push-based subscriptions.
// Must be called before Start(). Devices with OPCUseSubscriptions=true
// will use this handler instead of polling.
func (s *PollingService) SetSubscriptionHandler(handler SubscriptionHandler) {
	s.subscriptionHandler = handler
}

// Start begins the polling service.
func (s *PollingService) Start(ctx context.Context) error {
	if s.started.Load() {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.started.Store(true)

	s.mu.RLock()
	deviceCount := len(s.devices)
	s.mu.RUnlock()

	s.logger.Info().
		Int("devices", deviceCount).
		Int("workers", s.config.WorkerCount).
		Msg("Starting polling service")

	// Start polling for all registered devices
	s.mu.RLock()
	for _, dp := range s.devices {
		s.startDevicePoller(dp)
	}
	s.mu.RUnlock()

	return nil
}

// Stop gracefully stops the polling service.
func (s *PollingService) Stop(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}

	s.logger.Info().Msg("Stopping polling service")

	// Cancel context to signal all pollers to stop
	s.cancel()

	// Wait for all pollers with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info().Msg("All pollers stopped")
	case <-ctx.Done():
		s.logger.Warn().Msg("Timeout waiting for pollers to stop")
	}

	s.started.Store(false)
	return nil
}

// RegisterDevice registers a device for polling.
func (s *PollingService) RegisterDevice(ctx context.Context, device *domain.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.devices[device.ID]; exists {
		return domain.ErrDeviceExists
	}

	if !device.Enabled {
		s.logger.Debug().Str("device_id", device.ID).Msg("Skipping disabled device")
		return nil
	}

	dp := &devicePoller{
		device:   device,
		stopChan: make(chan struct{}),
	}

	s.devices[device.ID] = dp

	s.logger.Info().
		Str("device_id", device.ID).
		Str("device_name", device.Name).
		Int("tags", len(device.Tags)).
		Dur("poll_interval", device.PollInterval).
		Msg("Registered device for polling")

	// If service is already started, start polling this device
	if s.started.Load() {
		s.startDevicePoller(dp)
	}

	return nil
}

// UnregisterDevice stops polling and removes a device.
func (s *PollingService) UnregisterDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dp, exists := s.devices[deviceID]
	if !exists {
		return domain.ErrDeviceNotFound
	}

	// If device was using subscriptions, unsubscribe
	if dp.subscribed && s.subscriptionHandler != nil {
		if err := s.subscriptionHandler.Unsubscribe(deviceID); err != nil {
			s.logger.Warn().Err(err).Str("device_id", deviceID).Msg("Failed to unsubscribe device")
		}
		dp.subscribed = false
	}

	// Stop the poller
	if dp.running.Load() {
		dp.stopOnce.Do(func() {
			close(dp.stopChan)
		})
	}

	delete(s.devices, deviceID)

	s.logger.Info().Str("device_id", deviceID).Msg("Unregistered device")
	return nil
}

// ReplaceDevice atomically updates a device's configuration while preserving
// polling runtime state (stats, last poll time, error history). If the poll
// interval changed, the poller goroutine is restarted with the new interval.
// If only tags or non-connection fields changed, the device pointer is swapped
// in-place and the next poll cycle picks up the new config automatically.
func (s *PollingService) ReplaceDevice(ctx context.Context, device *domain.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dp, exists := s.devices[device.ID]
	if !exists {
		// Device not registered — treat as a fresh registration.
		s.mu.Unlock()
		err := s.RegisterDevice(ctx, device)
		s.mu.Lock()
		return err
	}

	if !device.Enabled {
		// Device disabled — unregister it.
		if dp.subscribed && s.subscriptionHandler != nil {
			_ = s.subscriptionHandler.Unsubscribe(device.ID)
			dp.subscribed = false
		}
		if dp.running.Load() {
			dp.stopOnce.Do(func() {
				close(dp.stopChan)
			})
		}
		delete(s.devices, device.ID)
		s.logger.Info().Str("device_id", device.ID).Msg("Device disabled, unregistered")
		return nil
	}

	oldInterval := dp.device.PollInterval
	wasSubscribed := dp.subscribed
	wantsSubscription := device.Connection.OPCUseSubscriptions && device.Protocol == domain.ProtocolOPCUA && s.subscriptionHandler != nil

	// Swap the device pointer. The next pollDevice() call reads from dp.device,
	// so it will automatically use the new tags, connection config, etc.
	dp.mu.Lock()
	dp.device = device
	dp.mu.Unlock()

	s.logger.Info().
		Str("device_id", device.ID).
		Int("tags", len(device.Tags)).
		Dur("poll_interval", device.PollInterval).
		Msg("Replaced device configuration (stats preserved)")

	// Handle mode change: subscription ↔ polling
	if wasSubscribed && !wantsSubscription {
		// Switching from subscription to polling
		if s.subscriptionHandler != nil {
			_ = s.subscriptionHandler.Unsubscribe(device.ID)
		}
		dp.subscribed = false
		dp.stopChan = make(chan struct{})
		dp.stopOnce = sync.Once{}
		dp.running.Store(false)
		s.startDevicePoller(dp)
		return nil
	}
	if !wasSubscribed && wantsSubscription {
		// Switching from polling to subscription
		if dp.running.Load() {
			dp.stopOnce.Do(func() {
				close(dp.stopChan)
			})
		}
		dp.running.Store(false)
		dp.stopChan = make(chan struct{})
		dp.stopOnce = sync.Once{}
		s.startDevicePoller(dp) // Will detect OPCUseSubscriptions and delegate
		return nil
	}
	if wasSubscribed && wantsSubscription {
		// Still subscription mode — re-subscribe with new tags
		if s.subscriptionHandler != nil {
			_ = s.subscriptionHandler.Unsubscribe(device.ID)
		}
		dp.subscribed = false
		dp.running.Store(false)
		s.startDevicePoller(dp) // Will delegate to startDeviceSubscription
		return nil
	}

	// If the poll interval changed, we need to restart the poller goroutine
	// because the ticker was created with the old interval.
	if oldInterval != device.PollInterval && s.started.Load() {
		s.logger.Info().
			Str("device_id", device.ID).
			Dur("old_interval", oldInterval).
			Dur("new_interval", device.PollInterval).
			Msg("Poll interval changed, restarting poller")

		// Stop the old poller goroutine
		if dp.running.Load() {
			dp.stopOnce.Do(func() {
				close(dp.stopChan)
			})

			// Wait for the old poller to stop (with timeout to avoid deadlock)
			deadline := time.Now().Add(5 * time.Second)
			for dp.running.Load() && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			if dp.running.Load() {
				s.logger.Warn().
					Str("device_id", device.ID).
					Msg("Timeout waiting for old poller to stop, forcing restart")
				dp.running.Store(false)
			}
		}

		// Create a new stop channel and reset the once guard for the restart
		dp.stopChan = make(chan struct{})
		dp.stopOnce = sync.Once{}

		s.startDevicePoller(dp)
	}

	return nil
}

// startDevicePoller starts the polling loop for a device.
// For OPC UA devices with subscriptions enabled, it delegates to the
// subscription handler for push-based data delivery instead of polling.
// Adds jitter to poll intervals to prevent synchronized bursts across devices.
func (s *PollingService) startDevicePoller(dp *devicePoller) {
	if dp.running.Load() {
		return
	}

	// Check if this device should use subscriptions instead of polling
	if dp.device.Connection.OPCUseSubscriptions && dp.device.Protocol == domain.ProtocolOPCUA && s.subscriptionHandler != nil {
		s.startDeviceSubscription(dp)
		return
	}

	dp.running.Store(true)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		defer dp.running.Store(false)

		// Add jitter (0-10% of interval) to spread device polls over time
		// This prevents all devices from polling simultaneously
		jitterMax := dp.device.PollInterval / 10
		if jitterMax > 0 {
			jitter := time.Duration(rand.Int63n(int64(jitterMax)))
			time.Sleep(jitter)
		}

		s.logger.Debug().
			Str("device_id", dp.device.ID).
			Dur("interval", dp.device.PollInterval).
			Msg("Starting device poller")

		ticker := time.NewTicker(dp.device.PollInterval)
		defer ticker.Stop()

		// Initial poll
		s.pollDevice(dp)

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-dp.stopChan:
				return
			case <-ticker.C:
				s.pollDevice(dp)
			}
		}
	}()
}

// startDeviceSubscription sets up push-based subscriptions for an OPC UA device.
// The subscription handler receives data changes from the server and publishes
// them to MQTT via the onData callback — no polling loop is needed.
func (s *PollingService) startDeviceSubscription(dp *devicePoller) {
	tags := s.getEnabledTags(dp.device)
	if len(tags) == 0 {
		s.logger.Warn().Str("device_id", dp.device.ID).Msg("No enabled tags for subscription")
		return
	}

	// The onData callback publishes each data point to MQTT.
	// Topic is already set by the subscription manager.
	onData := func(dataPoint *domain.DataPoint) {
		s.stats.PointsRead.Add(1)
		dp.stats.pointsRead.Add(1)

		if dataPoint.Quality == domain.QualityGood {
			if err := s.publisher.Publish(s.ctx, dataPoint); err != nil {
				s.logger.Warn().
					Err(err).
					Str("device_id", dp.device.ID).
					Str("tag_id", dataPoint.TagID).
					Msg("Failed to publish subscription data point")
			} else {
				s.stats.PointsPublished.Add(1)
			}
		}
	}

	err := s.subscriptionHandler.Subscribe(s.ctx, dp.device, tags, onData)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("device_id", dp.device.ID).
			Int("tags", len(tags)).
			Msg("Failed to set up subscriptions, falling back to polling")

		// Fallback to polling on subscription failure
		dp.subscribed = false
		dp.running.Store(false)
		s.startDevicePoller(dp)
		return
	}

	dp.subscribed = true
	dp.running.Store(true)

	s.logger.Info().
		Str("device_id", dp.device.ID).
		Int("tags", len(tags)).
		Msg("Device using OPC UA subscriptions (push mode)")
}

// pollDevice performs a single poll cycle for a device.
// Implements back-pressure: skips poll if all workers are busy instead of blocking.
func (s *PollingService) pollDevice(dp *devicePoller) {
	// Try to acquire worker from pool (non-blocking with back-pressure)
	select {
	case s.workerPool <- struct{}{}:
		defer func() { <-s.workerPool }()
	case <-s.ctx.Done():
		return
	default:
		// All workers busy - skip this poll cycle (back-pressure)
		s.stats.SkippedPolls.Add(1)
		dp.stats.skippedCount.Add(1)
		s.logger.Debug().
			Str("device_id", dp.device.ID).
			Msg("Poll skipped: worker pool full (back-pressure)")
		return
	}

	s.stats.TotalPolls.Add(1)
	dp.stats.pollCount.Add(1)

	startTime := time.Now()

	// Get enabled tags
	tags := s.getEnabledTags(dp.device)
	if len(tags) == 0 {
		return
	}

	// Build a lookup map so we can safely assign topics by TagID.
	// Protocol adapters are not required to preserve slice order (and some don't).
	tagByID := make(map[string]*domain.Tag, len(tags))
	for _, tag := range tags {
		if tag != nil && tag.ID != "" {
			tagByID[tag.ID] = tag
		}
	}

	// Read all tags from the device using the appropriate protocol.
	// Use device timeout directly (not 2x) for faster failure detection.
	readCtx, readCancel := context.WithTimeout(s.ctx, dp.device.Connection.Timeout)
	defer readCancel()

	dataPoints, err := s.protocolManager.ReadTags(readCtx, dp.device, tags)
	if err != nil {
		if errors.Is(err, domain.ErrCircuitBreakerOpen) {
			// Circuit breaker open means the endpoint is unhealthy; don't spam error logs.
			s.stats.SkippedPolls.Add(1)
			dp.stats.skippedCount.Add(1)
			dp.mu.Lock()
			dp.lastError = err
			dp.mu.Unlock()

			// Record Prometheus skip metric
			if s.metrics != nil {
				s.metrics.RecordPollSkipped()
			}

			s.logger.Debug().
				Err(err).
				Str("device_id", dp.device.ID).
				Msg("Poll skipped: circuit breaker open")
			return
		}

		s.stats.FailedPolls.Add(1)
		dp.stats.errorCount.Add(1)
		dp.mu.Lock()
		dp.lastError = err
		dp.mu.Unlock()

		// Record Prometheus error metric
		if s.metrics != nil {
			s.metrics.RecordPollError(dp.device.ID, "read_error")
		}

		s.logger.Error().
			Err(err).
			Str("device_id", dp.device.ID).
			Msg("Failed to read tags")
		return
	}

	s.stats.SuccessPolls.Add(1)
	dp.mu.Lock()
	dp.lastPoll = time.Now()
	dp.lastError = nil
	dp.mu.Unlock()

	// Release all DataPoints back to pool when done (after publishing serializes them).
	defer func() {
		for _, point := range dataPoints {
			domain.ReleaseDataPoint(point)
		}
	}()

	// Get slice from pool to reduce GC pressure
	goodPointsPtr := dataPointPool.Get().(*[]*domain.DataPoint)
	goodPoints := (*goodPointsPtr)[:0] // Reset length, keep capacity
	defer func() {
		// Clear references before returning to pool
		for i := range goodPoints {
			goodPoints[i] = nil
		}
		*goodPointsPtr = goodPoints[:0]
		dataPointPool.Put(goodPointsPtr)
	}()

	// Set topics and filter good data points.
	// Do NOT assume datapoints are aligned with tags by index.
	for _, point := range dataPoints {
		if point == nil {
			continue
		}

		if tag := tagByID[point.TagID]; tag != nil {
			point.Topic = topicForTag(dp.device.UNSPrefix, tag)
		} else if suffix := sanitizeTopicSegment(point.TagID); suffix != "" {
			point.Topic = dp.device.UNSPrefix + "/" + suffix
		} else {
			point.Topic = dp.device.UNSPrefix
		}

		if point.Quality == domain.QualityGood {
			goodPoints = append(goodPoints, point)
		}
	}

	s.stats.PointsRead.Add(uint64(len(dataPoints)))
	dp.stats.pointsRead.Add(uint64(len(dataPoints)))

	// Calculate staleness for good data points relative to expected poll interval.
	pollInterval := dp.device.PollInterval
	for _, point := range goodPoints {
		point.CalculateStaleness(pollInterval)
	}

	// Publish good data points.
	// Use the service context for publishing so device read timeout doesn't
	// accidentally cancel publishing when reads consume most of the deadline.
	publishCtx := s.ctx
	if len(goodPoints) > 0 {
		if err := s.publisher.PublishBatch(publishCtx, goodPoints); err != nil {
			s.logger.Warn().
				Err(err).
				Str("device_id", dp.device.ID).
				Int("points", len(goodPoints)).
				Msg("Failed to publish some data points")
		} else {
			s.stats.PointsPublished.Add(uint64(len(goodPoints)))
		}
	}

	// Record poll duration for metrics
	duration := time.Since(startTime)

	// Record Prometheus metrics
	if s.metrics != nil {
		s.metrics.RecordPollSuccess(dp.device.ID, string(dp.device.Protocol), duration.Seconds(), len(dataPoints))
	}

	// Log poll completion
	s.logger.Debug().
		Str("device_id", dp.device.ID).
		Int("tags_read", len(dataPoints)).
		Int("good_points", len(goodPoints)).
		Dur("duration", duration).
		Msg("Poll cycle completed")
}

// getEnabledTags returns only the enabled tags for a device.
func (s *PollingService) getEnabledTags(device *domain.Device) []*domain.Tag {
	tags := make([]*domain.Tag, 0, len(device.Tags))
	for i := range device.Tags {
		if device.Tags[i].Enabled {
			tags = append(tags, &device.Tags[i])
		}
	}
	return tags
}

// GetDeviceStatus returns the status of a device.
func (s *PollingService) GetDeviceStatus(deviceID string) (*DeviceStatus, error) {
	s.mu.RLock()
	dp, exists := s.devices[deviceID]
	s.mu.RUnlock()

	if !exists {
		return nil, domain.ErrDeviceNotFound
	}

	dp.mu.RLock()
	defer dp.mu.RUnlock()

	status := &DeviceStatus{
		DeviceID:   deviceID,
		DeviceName: dp.device.Name,
		Running:    dp.running.Load(),
		LastPoll:   dp.lastPoll,
		LastError:  dp.lastError,
		PollCount:  dp.stats.pollCount.Load(),
		ErrorCount: dp.stats.errorCount.Load(),
		PointsRead: dp.stats.pointsRead.Load(),
	}

	if dp.lastError == nil && !dp.lastPoll.IsZero() {
		status.Status = domain.DeviceStatusOnline
	} else if dp.lastError != nil {
		status.Status = domain.DeviceStatusError
	} else {
		status.Status = domain.DeviceStatusUnknown
	}

	return status, nil
}

// DeviceStatus holds the current status of a polled device.
type DeviceStatus struct {
	DeviceID   string
	DeviceName string
	Status     domain.DeviceStatus
	Running    bool
	LastPoll   time.Time
	LastError  error
	PollCount  uint64
	ErrorCount uint64
	PointsRead uint64
}

// StatsSnapshot holds a point-in-time snapshot of polling statistics.
type StatsSnapshot struct {
	TotalPolls      uint64
	SuccessPolls    uint64
	FailedPolls     uint64
	SkippedPolls    uint64
	PointsRead      uint64
	PointsPublished uint64
}

// Stats returns a snapshot of the polling service statistics.
func (s *PollingService) Stats() StatsSnapshot {
	return StatsSnapshot{
		TotalPolls:      s.stats.TotalPolls.Load(),
		SuccessPolls:    s.stats.SuccessPolls.Load(),
		FailedPolls:     s.stats.FailedPolls.Load(),
		SkippedPolls:    s.stats.SkippedPolls.Load(),
		PointsRead:      s.stats.PointsRead.Load(),
		PointsPublished: s.stats.PointsPublished.Load(),
	}
}
