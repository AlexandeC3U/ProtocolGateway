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

// PollingService orchestrates reading data from devices and publishing to MQTT.
// It supports multiple protocols through the ProtocolManager.
type PollingService struct {
	config          PollingConfig
	protocolManager *domain.ProtocolManager
	publisher       Publisher
	logger          zerolog.Logger
	metrics         *metrics.Registry
	devices         map[string]*devicePoller
	mu              sync.RWMutex
	started         atomic.Bool
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	workerPool      chan struct{}
	stats           *PollingStats
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
	device    *domain.Device
	stopChan  chan struct{}
	stopOnce  sync.Once
	running   atomic.Bool
	lastPoll  time.Time
	lastError error
	stats     deviceStats
	mu        sync.RWMutex
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

// startDevicePoller starts the polling loop for a device.
// Adds jitter to poll intervals to prevent synchronized bursts across devices.
func (s *PollingService) startDevicePoller(dp *devicePoller) {
	if dp.running.Load() {
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
