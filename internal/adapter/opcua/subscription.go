// Package opcua provides subscription management for OPC UA monitored items.
package opcua

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// SubscriptionManager manages OPC UA subscriptions for monitored items.
// Unlike Modbus polling, OPC UA supports server-side subscriptions where
// the server pushes data changes to the client (Report-by-Exception).
type SubscriptionManager struct {
	client          *Client
	subscriptions   map[string]*Subscription
	mu              sync.RWMutex
	logger          zerolog.Logger
	dataHandler     DataHandler
	publishInterval time.Duration
	queueSize       uint32
	running         atomic.Bool
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// Subscription represents an OPC UA subscription with its monitored items.
type Subscription struct {
	ID              uint32
	Device          *domain.Device
	Tags            map[string]*domain.Tag
	TagList         []*domain.Tag     // Ordered list for client handle lookup
	MonitoredItems  map[string]uint32 // tag ID -> monitored item ID
	LastValues      map[string]*domain.DataPoint
	opcuaSub        *opcua.Subscription
	notifyCh        chan *opcua.PublishNotificationData
	doneCh          chan struct{}  // Signals notification handler to stop
	wg              sync.WaitGroup // Waits for notification handler to exit
	mu              sync.RWMutex
	publishInterval time.Duration
	active          atomic.Bool
}

// DataHandler is called when new data is received from subscriptions.
type DataHandler func(dataPoint *domain.DataPoint)

// SubscriptionConfig holds configuration for subscriptions.
type SubscriptionConfig struct {
	// PublishInterval is how often the server should send notifications
	PublishInterval time.Duration

	// SamplingInterval is how often the server should sample values
	SamplingInterval time.Duration

	// QueueSize is the number of values to queue on the server
	QueueSize uint32

	// DiscardOldest determines whether to discard oldest or newest when queue is full
	DiscardOldest bool

	// DeadbandType is the deadband filter type (Absolute, Percent, None)
	DeadbandType string

	// DeadbandValue is the deadband threshold
	DeadbandValue float64
}

// DefaultSubscriptionConfig returns sensible defaults for subscriptions.
func DefaultSubscriptionConfig() SubscriptionConfig {
	return SubscriptionConfig{
		PublishInterval:  1 * time.Second,
		SamplingInterval: 500 * time.Millisecond,
		QueueSize:        10,
		DiscardOldest:    true,
		DeadbandType:     "None",
		DeadbandValue:    0,
	}
}

// NewSubscriptionManager creates a new subscription manager for an OPC UA client.
func NewSubscriptionManager(client *Client, handler DataHandler, logger zerolog.Logger) (*SubscriptionManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	sm := &SubscriptionManager{
		client:          client,
		subscriptions:   make(map[string]*Subscription),
		logger:          logger.With().Str("component", "opcua-subscription").Logger(),
		dataHandler:     handler,
		publishInterval: 1 * time.Second,
		queueSize:       10,
		ctx:             ctx,
		cancel:          cancel,
	}

	return sm, nil
}

// Start starts the subscription manager.
func (sm *SubscriptionManager) Start() error {
	if sm.running.Load() {
		return nil
	}

	if !sm.client.IsConnected() {
		return domain.ErrConnectionClosed
	}

	sm.running.Store(true)
	sm.logger.Info().Msg("Subscription manager started")

	return nil
}

// Stop stops the subscription manager and unsubscribes from all items.
func (sm *SubscriptionManager) Stop() error {
	if !sm.running.Load() {
		return nil
	}

	sm.cancel()
	sm.running.Store(false)

	// Unsubscribe from all
	sm.mu.Lock()
	for deviceID := range sm.subscriptions {
		_ = sm.unsubscribeDeviceLocked(deviceID)
	}
	sm.subscriptions = make(map[string]*Subscription)
	sm.mu.Unlock()

	sm.wg.Wait()
	sm.logger.Info().Msg("Subscription manager stopped")

	return nil
}

// Subscribe creates a subscription for a device and its tags.
func (sm *SubscriptionManager) Subscribe(device *domain.Device, tags []*domain.Tag, config SubscriptionConfig) error {
	if !sm.running.Load() {
		return domain.ErrServiceNotStarted
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if subscription already exists
	if _, exists := sm.subscriptions[device.ID]; exists {
		// Update existing subscription
		return sm.updateSubscriptionLocked(device, tags, config)
	}

	// Create new subscription
	sub := &Subscription{
		Device:          device,
		Tags:            make(map[string]*domain.Tag),
		TagList:         make([]*domain.Tag, 0, len(tags)),
		MonitoredItems:  make(map[string]uint32),
		LastValues:      make(map[string]*domain.DataPoint),
		notifyCh:        make(chan *opcua.PublishNotificationData, 100),
		doneCh:          make(chan struct{}),
		publishInterval: config.PublishInterval,
	}

	for _, tag := range tags {
		sub.Tags[tag.ID] = tag
		sub.TagList = append(sub.TagList, tag)
	}

	sm.subscriptions[device.ID] = sub

	// Create OPC UA subscription
	if err := sm.createOPCSubscription(sub, config); err != nil {
		delete(sm.subscriptions, device.ID)
		return err
	}

	sub.active.Store(true)
	sm.logger.Info().
		Str("device_id", device.ID).
		Int("tags", len(tags)).
		Dur("publish_interval", config.PublishInterval).
		Msg("Created subscription")

	return nil
}

// Unsubscribe removes a subscription for a device.
func (sm *SubscriptionManager) Unsubscribe(deviceID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.unsubscribeDeviceLocked(deviceID)
}

// unsubscribeDeviceLocked removes a subscription (must hold lock).
func (sm *SubscriptionManager) unsubscribeDeviceLocked(deviceID string) error {
	sub, exists := sm.subscriptions[deviceID]
	if !exists {
		return domain.ErrDeviceNotFound
	}

	sub.active.Store(false)

	// Signal the notification handler to stop first
	if sub.doneCh != nil {
		close(sub.doneCh)
	}

	// Cancel the OPC UA subscription
	if sub.opcuaSub != nil {
		if err := sub.opcuaSub.Cancel(sm.ctx); err != nil {
			sm.logger.Warn().
				Err(err).
				Str("device_id", deviceID).
				Msg("Error cancelling OPC UA subscription")
		}
	}

	// Wait for notification handler to exit before closing channel
	// This prevents panic from send-on-closed-channel
	sub.wg.Wait()

	// Now safe to close notification channel
	if sub.notifyCh != nil {
		close(sub.notifyCh)
	}

	delete(sm.subscriptions, deviceID)
	sm.logger.Info().Str("device_id", deviceID).Msg("Removed subscription")

	return nil
}

// createOPCSubscription creates the actual OPC UA subscription using the gopcua API.
func (sm *SubscriptionManager) createOPCSubscription(sub *Subscription, config SubscriptionConfig) error {
	if !sm.client.IsConnected() {
		return domain.ErrConnectionClosed
	}

	sm.client.mu.RLock()
	client := sm.client.client
	sm.client.mu.RUnlock()

	if client == nil {
		return domain.ErrConnectionClosed
	}

	// Create subscription parameters
	params := &opcua.SubscriptionParameters{
		Interval: config.PublishInterval,
	}

	// Create subscription with notification channel
	opcuaSub, err := client.Subscribe(sm.ctx, params, sub.notifyCh)
	if err != nil {
		return fmt.Errorf("%w: failed to create subscription: %v", domain.ErrOPCUASubscriptionFailed, err)
	}

	sub.opcuaSub = opcuaSub
	sub.ID = opcuaSub.SubscriptionID

	// Build monitored item requests using ua.MonitoredItemCreateRequest
	monitoredItemRequests := make([]*ua.MonitoredItemCreateRequest, 0, len(sub.Tags))

	clientHandle := uint32(0)
	for _, tag := range sub.TagList {
		nodeID, err := sm.client.getNodeID(tag.OPCNodeID)
		if err != nil {
			sm.logger.Warn().
				Err(err).
				Str("tag_id", tag.ID).
				Str("node_id", tag.OPCNodeID).
				Msg("Failed to parse node ID, skipping tag")
			continue
		}

		// Create monitored item request
		req := &ua.MonitoredItemCreateRequest{
			ItemToMonitor: &ua.ReadValueID{
				NodeID:       nodeID,
				AttributeID:  ua.AttributeIDValue,
				DataEncoding: &ua.QualifiedName{},
			},
			MonitoringMode: ua.MonitoringModeReporting,
			RequestedParameters: &ua.MonitoringParameters{
				ClientHandle:     clientHandle,
				SamplingInterval: float64(config.SamplingInterval.Milliseconds()),
				QueueSize:        config.QueueSize,
				DiscardOldest:    config.DiscardOldest,
			},
		}

		// Add deadband filter if specified
		if config.DeadbandType != "None" && config.DeadbandValue > 0 {
			req.RequestedParameters.Filter = sm.createDeadbandFilter(config)
		}

		monitoredItemRequests = append(monitoredItemRequests, req)
		clientHandle++
	}

	if len(monitoredItemRequests) == 0 {
		// Cancel the subscription since we have no items to monitor
		_ = opcuaSub.Cancel(sm.ctx)
		return fmt.Errorf("no valid tags to monitor")
	}

	// Add monitored items to the subscription using Monitor
	res, err := opcuaSub.Monitor(sm.ctx, ua.TimestampsToReturnBoth, monitoredItemRequests...)
	if err != nil {
		_ = opcuaSub.Cancel(sm.ctx)
		return fmt.Errorf("%w: failed to create monitored items: %v", domain.ErrOPCUASubscriptionFailed, err)
	}

	// Map monitored items to tags
	// res is *ua.CreateMonitoredItemsResponse, Results is the slice
	if res != nil && res.Results != nil {
		for i, result := range res.Results {
			if i >= len(sub.TagList) {
				break
			}
			if result.StatusCode == ua.StatusOK {
				sub.mu.Lock()
				sub.MonitoredItems[sub.TagList[i].ID] = result.MonitoredItemID
				sub.mu.Unlock()
				sm.logger.Debug().
					Str("tag_id", sub.TagList[i].ID).
					Uint32("monitored_item_id", result.MonitoredItemID).
					Msg("Created monitored item")
			} else {
				sm.logger.Warn().
					Str("tag_id", sub.TagList[i].ID).
					Uint32("status", uint32(result.StatusCode)).
					Msg("Failed to create monitored item")
			}
		}
	}

	// Start notification handler goroutine
	// Add to both manager wg (for global shutdown) and sub wg (for individual unsubscribe)
	sm.wg.Add(1)
	sub.wg.Add(1)
	go sm.handleNotifications(sub)

	sm.logger.Info().
		Str("device_id", sub.Device.ID).
		Uint32("subscription_id", sub.ID).
		Int("monitored_items", len(sub.MonitoredItems)).
		Msg("OPC UA subscription created")

	return nil
}

// handleNotifications processes notifications from the subscription channel.
// Uses both manager context and subscription doneCh for clean shutdown.
func (sm *SubscriptionManager) handleNotifications(sub *Subscription) {
	defer sm.wg.Done()
	defer sub.wg.Done() // Signal subscription that handler has exited

	sm.logger.Debug().
		Str("device_id", sub.Device.ID).
		Uint32("subscription_id", sub.ID).
		Msg("Starting notification handler")

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-sub.doneCh:
			// Subscription is being removed, exit cleanly
			sm.logger.Debug().
				Str("device_id", sub.Device.ID).
				Msg("Notification handler stopping (done signal)")
			return
		case notif, ok := <-sub.notifyCh:
			if !ok {
				sm.logger.Debug().
					Str("device_id", sub.Device.ID).
					Msg("Notification channel closed")
				return
			}

			if !sub.active.Load() {
				continue
			}

			sm.processNotification(sub, notif)
		}
	}
}

// processNotification processes a single publish notification.
func (sm *SubscriptionManager) processNotification(sub *Subscription, notif *opcua.PublishNotificationData) {
	if notif == nil || notif.Value == nil {
		return
	}

	// Process data change notifications
	switch n := notif.Value.(type) {
	case *ua.DataChangeNotification:
		for _, item := range n.MonitoredItems {
			sm.processDataChange(sub, item)
		}
	case *ua.EventNotificationList:
		// Handle events if needed in the future
		sm.logger.Debug().Msg("Received event notification (not processed)")
	}
}

// processDataChange processes a single data change notification.
func (sm *SubscriptionManager) processDataChange(sub *Subscription, item *ua.MonitoredItemNotification) {
	if item == nil || item.Value == nil {
		return
	}

	// Find the tag for this monitored item by client handle
	var tag *domain.Tag
	if int(item.ClientHandle) < len(sub.TagList) {
		tag = sub.TagList[item.ClientHandle]
	}

	if tag == nil {
		sm.logger.Warn().
			Uint32("client_handle", item.ClientHandle).
			Msg("Received notification for unknown tag")
		return
	}

	// Convert to data point
	dp := sm.client.processReadResult(item.Value, tag)
	suffix := strings.TrimSpace(tag.TopicSuffix)
	if suffix == "" {
		suffix = tag.Name
	}
	if strings.TrimSpace(suffix) == "" {
		suffix = tag.ID
	}
	suffix = strings.TrimSpace(suffix)
	suffix = strings.ReplaceAll(suffix, "/", "_")
	suffix = strings.ReplaceAll(suffix, "#", "_")
	suffix = strings.ReplaceAll(suffix, "+", "_")
	suffix = strings.ReplaceAll(suffix, " ", "_")
	suffix = strings.Trim(suffix, "_")
	if suffix == "" {
		dp.Topic = sub.Device.UNSPrefix
	} else {
		dp.Topic = sub.Device.UNSPrefix + "/" + suffix
	}

	// Update last value
	sub.mu.Lock()
	sub.LastValues[tag.ID] = dp
	sub.mu.Unlock()

	// Notify handler
	if sm.dataHandler != nil {
		sm.dataHandler(dp)
	}

	sm.client.stats.NotificationCount.Add(1)

	sm.logger.Debug().
		Str("tag_id", tag.ID).
		Interface("value", dp.Value).
		Msg("Processed data change notification")
}

// updateSubscriptionLocked updates an existing subscription with new tags (must hold lock).
func (sm *SubscriptionManager) updateSubscriptionLocked(device *domain.Device, tags []*domain.Tag, config SubscriptionConfig) error {
	// For simplicity, recreate the subscription
	// A more optimized implementation would add/remove individual monitored items
	if err := sm.unsubscribeDeviceLocked(device.ID); err != nil && err != domain.ErrDeviceNotFound {
		return err
	}

	// Re-add to subscriptions map and create new subscription
	sub := &Subscription{
		Device:          device,
		Tags:            make(map[string]*domain.Tag),
		TagList:         make([]*domain.Tag, 0, len(tags)),
		MonitoredItems:  make(map[string]uint32),
		LastValues:      make(map[string]*domain.DataPoint),
		notifyCh:        make(chan *opcua.PublishNotificationData, 100),
		publishInterval: config.PublishInterval,
	}

	for _, tag := range tags {
		sub.Tags[tag.ID] = tag
		sub.TagList = append(sub.TagList, tag)
	}

	sm.subscriptions[device.ID] = sub

	if err := sm.createOPCSubscription(sub, config); err != nil {
		delete(sm.subscriptions, device.ID)
		return err
	}

	sub.active.Store(true)
	return nil
}

// createDeadbandFilter creates an OPC UA deadband filter extension object.
func (sm *SubscriptionManager) createDeadbandFilter(config SubscriptionConfig) *ua.ExtensionObject {
	var deadbandType uint32
	switch config.DeadbandType {
	case "Absolute":
		deadbandType = 1 // AbsoluteDeadband
	case "Percent":
		deadbandType = 2 // PercentDeadband
	default:
		return nil
	}

	filter := &ua.DataChangeFilter{
		Trigger:       ua.DataChangeTriggerStatusValue,
		DeadbandType:  deadbandType,
		DeadbandValue: config.DeadbandValue,
	}

	return &ua.ExtensionObject{
		TypeID: &ua.ExpandedNodeID{
			NodeID: ua.NewNumericNodeID(0, 724), // DataChangeFilter encoding
		},
		Value: filter,
	}
}

// GetSubscription returns a subscription by device ID.
func (sm *SubscriptionManager) GetSubscription(deviceID string) (*Subscription, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sub, exists := sm.subscriptions[deviceID]
	return sub, exists
}

// GetLastValue returns the last received value for a tag.
func (sm *SubscriptionManager) GetLastValue(deviceID, tagID string) (*domain.DataPoint, bool) {
	sm.mu.RLock()
	sub, exists := sm.subscriptions[deviceID]
	sm.mu.RUnlock()

	if !exists {
		return nil, false
	}

	sub.mu.RLock()
	defer sub.mu.RUnlock()

	dp, exists := sub.LastValues[tagID]
	return dp, exists
}

// GetAllSubscriptions returns all active subscriptions.
func (sm *SubscriptionManager) GetAllSubscriptions() []*Subscription {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	subs := make([]*Subscription, 0, len(sm.subscriptions))
	for _, sub := range sm.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

// Stats returns subscription statistics.
func (sm *SubscriptionManager) Stats() SubscriptionStats {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	stats := SubscriptionStats{
		TotalSubscriptions: len(sm.subscriptions),
		Running:            sm.running.Load(),
	}

	for _, sub := range sm.subscriptions {
		sub.mu.RLock()
		stats.TotalMonitoredItems += len(sub.MonitoredItems)
		if sub.active.Load() {
			stats.ActiveSubscriptions++
		}
		sub.mu.RUnlock()
	}

	return stats
}

// SubscriptionStats contains subscription statistics.
type SubscriptionStats struct {
	TotalSubscriptions  int
	ActiveSubscriptions int
	TotalMonitoredItems int
	Running             bool
}

// IsActive returns whether a subscription is actively receiving data.
func (s *Subscription) IsActive() bool {
	return s.active.Load()
}

// GetLastValue returns the last received value for a tag.
func (s *Subscription) GetLastValue(tagID string) (*domain.DataPoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dp, exists := s.LastValues[tagID]
	return dp, exists
}

// GetAllLastValues returns all last received values.
func (s *Subscription) GetAllLastValues() map[string]*domain.DataPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make(map[string]*domain.DataPoint, len(s.LastValues))
	for k, v := range s.LastValues {
		values[k] = v
	}
	return values
}
