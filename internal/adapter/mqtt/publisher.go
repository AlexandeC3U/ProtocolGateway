// Package mqtt provides a production-grade MQTT publisher with automatic
// reconnection, message buffering, and comprehensive error handling.
package mqtt

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/rs/zerolog"
)

// Publisher handles publishing data points to the MQTT broker.
type Publisher struct {
	config        Config
	client        pahomqtt.Client
	logger        zerolog.Logger
	metrics       *metrics.Registry
	mu            sync.RWMutex
	connected     atomic.Bool
	reconnecting  atomic.Bool
	messageBuffer chan *BufferedMessage
	done          chan struct{}
	wg            sync.WaitGroup
	stats         *PublisherStats
	topicMu       sync.RWMutex
	topicStats    map[string]*TopicStat
}

// TopicStat tracks publish activity for a given topic.
// Used for the Web UI "Active Topics" view.
type TopicStat struct {
	Topic            string    `json:"topic"`
	Count            uint64    `json:"count"`
	LastPublished    time.Time `json:"last_published"`
	LastPayloadBytes int       `json:"last_payload_bytes"`
}

// Config holds MQTT publisher configuration.
type Config struct {
	BrokerURL      string
	ClientID       string
	Username       string
	Password       string
	CleanSession   bool
	QoS            byte
	KeepAlive      time.Duration
	ConnectTimeout time.Duration
	ReconnectDelay time.Duration
	MaxReconnect   int
	TLSEnabled     bool
	TLSCertFile    string
	TLSKeyFile     string
	TLSCAFile      string
	BufferSize     int
	PublishTimeout time.Duration
	RetainMessages bool
}

// BufferedMessage represents a message waiting to be published.
type BufferedMessage struct {
	Topic     string
	Payload   []byte
	QoS       byte
	Retained  bool
	Timestamp time.Time
}

// PublisherStats tracks publisher performance metrics.
type PublisherStats struct {
	MessagesPublished atomic.Uint64
	MessagesFailed    atomic.Uint64
	MessagesBuffered  atomic.Uint64
	BytesSent         atomic.Uint64
	ReconnectCount    atomic.Uint64
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BrokerURL:      "tcp://localhost:1883",
		ClientID:       "protocol-gateway",
		CleanSession:   true,
		QoS:            1,
		KeepAlive:      30 * time.Second,
		ConnectTimeout: 10 * time.Second,
		ReconnectDelay: 5 * time.Second,
		MaxReconnect:   -1, // Unlimited
		BufferSize:     10000,
		PublishTimeout: 5 * time.Second,
		RetainMessages: false,
	}
}

// NewPublisher creates a new MQTT publisher.
func NewPublisher(config Config, logger zerolog.Logger, metricsReg *metrics.Registry) (*Publisher, error) {
	// Apply defaults
	if config.BufferSize == 0 {
		config.BufferSize = 10000
	}
	if config.PublishTimeout == 0 {
		config.PublishTimeout = 5 * time.Second
	}
	if config.KeepAlive == 0 {
		config.KeepAlive = 30 * time.Second
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 10 * time.Second
	}
	if config.ReconnectDelay == 0 {
		config.ReconnectDelay = 5 * time.Second
	}

	p := &Publisher{
		config:        config,
		logger:        logger.With().Str("component", "mqtt-publisher").Logger(),
		metrics:       metricsReg,
		messageBuffer: make(chan *BufferedMessage, config.BufferSize),
		done:          make(chan struct{}),
		stats:         &PublisherStats{},
		topicStats:    make(map[string]*TopicStat),
	}

	return p, nil
}

// ActiveTopics returns the most recently published topics, sorted by recency.
// If limit <= 0, a default limit of 200 is used.
func (p *Publisher) ActiveTopics(limit int) []TopicStat {
	if limit <= 0 {
		limit = 200
	}

	p.topicMu.RLock()
	out := make([]TopicStat, 0, len(p.topicStats))
	for _, stat := range p.topicStats {
		out = append(out, *stat)
	}
	p.topicMu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		return out[i].LastPublished.After(out[j].LastPublished)
	})

	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (p *Publisher) recordTopicPublish(topic string, payloadBytes int) {
	now := time.Now()

	p.topicMu.Lock()
	defer p.topicMu.Unlock()

	stat, ok := p.topicStats[topic]
	if !ok {
		// Limit the number of tracked topics to prevent unbounded memory growth
		// If we have too many topics, evict the oldest ones
		const maxTrackedTopics = 10000
		if len(p.topicStats) >= maxTrackedTopics {
			p.evictOldestTopicsLocked(maxTrackedTopics / 10) // Evict 10%
		}
		stat = &TopicStat{Topic: topic}
		p.topicStats[topic] = stat
	}
	stat.Count++
	stat.LastPublished = now
	stat.LastPayloadBytes = payloadBytes
}

// evictOldestTopicsLocked removes the N oldest topics from the stats map.
// Must be called with topicMu held.
func (p *Publisher) evictOldestTopicsLocked(count int) {
	if count <= 0 || len(p.topicStats) == 0 {
		return
	}

	// Build list of topics with their last publish time
	type topicAge struct {
		topic string
		time  time.Time
	}
	topics := make([]topicAge, 0, len(p.topicStats))
	for topic, stat := range p.topicStats {
		topics = append(topics, topicAge{topic: topic, time: stat.LastPublished})
	}

	// Sort by age (oldest first)
	sort.Slice(topics, func(i, j int) bool {
		return topics[i].time.Before(topics[j].time)
	})

	// Delete the oldest entries
	if count > len(topics) {
		count = len(topics)
	}
	for i := 0; i < count; i++ {
		delete(p.topicStats, topics[i].topic)
	}

	p.logger.Debug().Int("evicted", count).Int("remaining", len(p.topicStats)).Msg("Evicted old topic stats")
}

// Connect establishes the connection to the MQTT broker.
func (p *Publisher) Connect(ctx context.Context) error {
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(p.config.BrokerURL)
	opts.SetClientID(p.config.ClientID)
	opts.SetCleanSession(p.config.CleanSession)
	opts.SetKeepAlive(p.config.KeepAlive)
	opts.SetConnectTimeout(p.config.ConnectTimeout)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(p.config.ReconnectDelay)

	if p.config.Username != "" {
		opts.SetUsername(p.config.Username)
		opts.SetPassword(p.config.Password)
	}

	// TLS configuration
	if p.config.TLSEnabled {
		tlsConfig, err := p.createTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to create TLS config: %w", err)
		}
		opts.SetTLSConfig(tlsConfig)
	}

	// Connection handlers
	opts.SetOnConnectHandler(p.onConnect)
	opts.SetConnectionLostHandler(p.onConnectionLost)
	opts.SetReconnectingHandler(p.onReconnecting)

	// Create client
	p.client = pahomqtt.NewClient(opts)

	// Connect with context timeout
	p.logger.Info().Str("broker", p.config.BrokerURL).Msg("Connecting to MQTT broker")

	token := p.client.Connect()

	// Wait for connection with context
	connectDone := make(chan bool, 1)
	go func() {
		connectDone <- token.WaitTimeout(p.config.ConnectTimeout)
	}()

	select {
	case success := <-connectDone:
		if !success {
			return fmt.Errorf("%w: connection timeout", domain.ErrMQTTConnectionFailed)
		}
		if token.Error() != nil {
			return fmt.Errorf("%w: %v", domain.ErrMQTTConnectionFailed, token.Error())
		}
	case <-ctx.Done():
		return fmt.Errorf("%w: %v", domain.ErrMQTTConnectionFailed, ctx.Err())
	}

	// Ensure connected state is set (callback might not have fired yet)
	p.connected.Store(true)

	// Reinitialize done channel for reconnection support
	p.done = make(chan struct{})

	// Start buffer processor
	p.wg.Add(1)
	go p.processBuffer()

	p.logger.Info().Msg("Connected to MQTT broker")
	return nil
}

// Disconnect gracefully disconnects from the MQTT broker.
func (p *Publisher) Disconnect() {
	p.logger.Info().Msg("Disconnecting from MQTT broker")

	// Signal buffer processor to stop (safe close)
	select {
	case <-p.done:
		// Already closed
	default:
		close(p.done)
	}
	p.wg.Wait()

	// Disconnect client
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(1000) // Wait 1 second for pending messages
	}

	p.connected.Store(false)
	p.logger.Info().Msg("Disconnected from MQTT broker")
}

// Publish publishes a data point to the MQTT broker.
func (p *Publisher) Publish(ctx context.Context, dataPoint *domain.DataPoint) error {
	if !p.connected.Load() {
		// Buffer the message for later
		return p.bufferMessage(dataPoint)
	}

	payload, err := dataPoint.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize data point: %w", err)
	}

	return p.publishRaw(ctx, dataPoint.Topic, payload, p.config.QoS, p.config.RetainMessages)
}

// PublishBatch publishes multiple data points efficiently.
func (p *Publisher) PublishBatch(ctx context.Context, dataPoints []*domain.DataPoint) error {
	var lastErr error
	for _, dp := range dataPoints {
		if err := p.Publish(ctx, dp); err != nil {
			lastErr = err
			p.stats.MessagesFailed.Add(1)
		}
	}
	return lastErr
}

// publishRaw publishes raw payload to a topic.
func (p *Publisher) publishRaw(ctx context.Context, topic string, payload []byte, qos byte, retained bool) error {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return domain.ErrMQTTNotConnected
	}

	token := client.Publish(topic, qos, retained, payload)

	// Wait for publish with context
	publishDone := make(chan bool, 1)
	go func() {
		publishDone <- token.WaitTimeout(p.config.PublishTimeout)
	}()

	select {
	case success := <-publishDone:
		if !success {
			p.stats.MessagesFailed.Add(1)
			if p.metrics != nil {
				p.metrics.RecordMQTTPublish(false, p.config.PublishTimeout.Seconds())
			}
			return fmt.Errorf("%w: publish timeout", domain.ErrMQTTPublishFailed)
		}
		if token.Error() != nil {
			p.stats.MessagesFailed.Add(1)
			if p.metrics != nil {
				p.metrics.RecordMQTTPublish(false, 0)
			}
			return fmt.Errorf("%w: %v", domain.ErrMQTTPublishFailed, token.Error())
		}
	case <-ctx.Done():
		p.stats.MessagesFailed.Add(1)
		if p.metrics != nil {
			p.metrics.RecordMQTTPublish(false, p.config.PublishTimeout.Seconds())
		}
		return fmt.Errorf("%w: %v", domain.ErrMQTTPublishFailed, ctx.Err())
	}

	p.stats.MessagesPublished.Add(1)
	p.stats.BytesSent.Add(uint64(len(payload)))
	p.recordTopicPublish(topic, len(payload))

	// Record Prometheus metrics
	if p.metrics != nil {
		p.metrics.RecordMQTTPublish(true, 0) // TODO: measure actual latency
	}

	return nil
}

// bufferMessage adds a message to the buffer for later publishing.
func (p *Publisher) bufferMessage(dataPoint *domain.DataPoint) error {
	payload, err := dataPoint.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize data point: %w", err)
	}

	msg := &BufferedMessage{
		Topic:     dataPoint.Topic,
		Payload:   payload,
		QoS:       p.config.QoS,
		Retained:  p.config.RetainMessages,
		Timestamp: time.Now(),
	}

	select {
	case p.messageBuffer <- msg:
		p.stats.MessagesBuffered.Add(1)
		return nil
	default:
		// Buffer full, drop oldest message
		select {
		case <-p.messageBuffer:
			p.messageBuffer <- msg
			p.logger.Warn().Msg("Buffer full, dropped oldest message")
			return nil
		default:
			return fmt.Errorf("message buffer full")
		}
	}
}

// processBuffer processes buffered messages when connected.
func (p *Publisher) processBuffer() {
	defer p.wg.Done()

	for {
		select {
		case <-p.done:
			// Drain remaining messages
			p.drainBuffer()
			return

		case msg := <-p.messageBuffer:
			if p.connected.Load() {
				ctx, cancel := context.WithTimeout(context.Background(), p.config.PublishTimeout)
				if err := p.publishRaw(ctx, msg.Topic, msg.Payload, msg.QoS, msg.Retained); err != nil {
					p.logger.Warn().Err(err).Str("topic", msg.Topic).Msg("Failed to publish buffered message")
				}
				cancel()
			} else {
				// Re-buffer if not connected
				select {
				case p.messageBuffer <- msg:
				default:
					// Buffer still full, drop message
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// drainBuffer attempts to publish all remaining buffered messages.
func (p *Publisher) drainBuffer() {
	timeout := time.After(5 * time.Second)
	for {
		select {
		case msg := <-p.messageBuffer:
			if p.connected.Load() {
				ctx, cancel := context.WithTimeout(context.Background(), p.config.PublishTimeout)
				if err := p.publishRaw(ctx, msg.Topic, msg.Payload, msg.QoS, msg.Retained); err != nil {
					p.logger.Warn().Err(err).Str("topic", msg.Topic).Msg("Failed to drain buffered message")
				}
				cancel()
			}
		case <-timeout:
			remaining := len(p.messageBuffer)
			if remaining > 0 {
				p.logger.Warn().Int("count", remaining).Msg("Timeout draining buffer, messages dropped")
			}
			return
		default:
			return
		}
	}
}

// createTLSConfig creates TLS configuration for secure connections.
func (p *Publisher) createTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load CA certificate
	if p.config.TLSCAFile != "" {
		caCert, err := os.ReadFile(p.config.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key
	if p.config.TLSCertFile != "" && p.config.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(p.config.TLSCertFile, p.config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// onConnect is called when the client connects to the broker.
func (p *Publisher) onConnect(client pahomqtt.Client) {
	p.connected.Store(true)
	p.reconnecting.Store(false)
	p.logger.Info().Msg("MQTT connection established")

	// Update metrics
	if p.metrics != nil {
		// Record reconnect if this wasn't the initial connection
	}
}

// onConnectionLost is called when the connection is lost.
func (p *Publisher) onConnectionLost(client pahomqtt.Client, err error) {
	p.connected.Store(false)
	p.logger.Warn().Err(err).Msg("MQTT connection lost")
}

// onReconnecting is called when the client is attempting to reconnect.
func (p *Publisher) onReconnecting(client pahomqtt.Client, opts *pahomqtt.ClientOptions) {
	p.reconnecting.Store(true)
	p.stats.ReconnectCount.Add(1)
	p.logger.Info().Msg("Attempting to reconnect to MQTT broker")
}

// IsConnected returns true if the publisher is connected to the broker.
func (p *Publisher) IsConnected() bool {
	return p.connected.Load()
}

// Stats returns publisher statistics.
func (p *Publisher) Stats() PublisherStats {
	return PublisherStats{
		MessagesPublished: p.stats.MessagesPublished,
		MessagesFailed:    p.stats.MessagesFailed,
		MessagesBuffered:  p.stats.MessagesBuffered,
		BytesSent:         p.stats.BytesSent,
		ReconnectCount:    p.stats.ReconnectCount,
	}
}

// BufferSize returns the current number of buffered messages.
func (p *Publisher) BufferSize() int {
	return len(p.messageBuffer)
}

// HealthCheck implements the health.Checker interface.
func (p *Publisher) HealthCheck(ctx context.Context) error {
	if !p.connected.Load() {
		return domain.ErrMQTTNotConnected
	}
	return nil
}

// Client returns the underlying MQTT client.
// This is used by the command handler to subscribe to write commands.
func (p *Publisher) Client() pahomqtt.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client
}
