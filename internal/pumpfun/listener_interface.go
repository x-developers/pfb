package pumpfun

import (
	"context"
	"strings"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
)

// ListenerInterface defines the interface for token event listeners
type ListenerInterface interface {
	// Start starts the listener
	Start() error

	// Stop stops the listener
	Stop() error

	// GetTokenChannel returns the channel for receiving token events
	GetTokenChannel() <-chan *TokenEvent

	// GetStats returns listener statistics
	GetStats() map[string]interface{}

	// GetListenerType returns the type of listener
	GetListenerType() string

	// IsRunning returns whether the listener is currently running
	IsRunning() bool
}

// ListenerConfig contains common configuration for all listeners
type ListenerConfig struct {
	Config      *config.Config
	Logger      *logger.Logger
	Context     context.Context
	BufferSize  int
	NetworkType string // "mainnet" or "devnet"
}

// DefaultListenerConfig returns default listener configuration
func DefaultListenerConfig(cfg *config.Config, logger *logger.Logger) *ListenerConfig {
	return &ListenerConfig{
		Config:      cfg,
		Logger:      logger,
		Context:     context.Background(),
		BufferSize:  100,
		NetworkType: cfg.Network,
	}
}

// ListenerStats represents common statistics for all listeners
type ListenerStats struct {
	ListenerType        string  `json:"listener_type"`
	IsRunning           bool    `json:"is_running"`
	TokensDiscovered    int64   `json:"tokens_discovered"`
	TokensProcessed     int64   `json:"tokens_processed"`
	AverageLatencyMs    float64 `json:"average_latency_ms"`
	FastestProcessingMs int64   `json:"fastest_processing_ms"`
	SlowestProcessingMs int64   `json:"slowest_processing_ms"`
	ErrorCount          int64   `json:"error_count"`
	LastTokenTime       string  `json:"last_token_time"`
	UpTime              string  `json:"uptime"`
}

// MultiListener combines multiple listeners for comprehensive token discovery
type MultiListener struct {
	listeners []ListenerInterface
	config    *ListenerConfig
	tokenChan chan *TokenEvent
	running   bool
}

// NewMultiListener creates a new multi-listener that combines different listener types
func NewMultiListener(config *ListenerConfig, listeners ...ListenerInterface) *MultiListener {
	return &MultiListener{
		listeners: listeners,
		config:    config,
		tokenChan: make(chan *TokenEvent, config.BufferSize*len(listeners)),
		running:   false,
	}
}

// Start starts all listeners
func (ml *MultiListener) Start() error {
	ml.running = true

	// Start all listeners
	for _, listener := range ml.listeners {
		if err := listener.Start(); err != nil {
			ml.config.Logger.WithError(err).Error("Failed to start listener")
			continue
		}
	}

	// Start token event aggregation
	go ml.aggregateTokenEvents()

	ml.config.Logger.WithField("listener_count", len(ml.listeners)).Info("🚀 MultiListener started")
	return nil
}

// Stop stops all listeners
func (ml *MultiListener) Stop() error {
	ml.running = false

	for _, listener := range ml.listeners {
		if err := listener.Stop(); err != nil {
			ml.config.Logger.WithError(err).Error("Failed to stop listener")
		}
	}

	close(ml.tokenChan)
	ml.config.Logger.Info("🛑 MultiListener stopped")
	return nil
}

// GetTokenChannel returns the aggregated token channel
func (ml *MultiListener) GetTokenChannel() <-chan *TokenEvent {
	return ml.tokenChan
}

// GetStats returns combined statistics from all listeners
func (ml *MultiListener) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"listener_type":  "multi",
		"is_running":     ml.running,
		"listener_count": len(ml.listeners),
		"listeners":      make(map[string]interface{}),
	}

	totalTokens := int64(0)
	totalProcessed := int64(0)

	for _, listener := range ml.listeners {
		listenerStats := listener.GetStats()
		stats["listeners"].(map[string]interface{})[listener.GetListenerType()] = listenerStats

		// Aggregate totals
		if tokens, ok := listenerStats["tokens_discovered"].(int64); ok {
			totalTokens += tokens
		}
		if processed, ok := listenerStats["tokens_processed"].(int64); ok {
			totalProcessed += processed
		}
	}

	stats["total_tokens_discovered"] = totalTokens
	stats["total_tokens_processed"] = totalProcessed

	return stats
}

// GetListenerType returns the listener type
func (ml *MultiListener) GetListenerType() string {
	return "multi"
}

// IsRunning returns whether the multi-listener is running
func (ml *MultiListener) IsRunning() bool {
	return ml.running
}

// aggregateTokenEvents aggregates token events from all listeners
func (ml *MultiListener) aggregateTokenEvents() {
	seenTokens := make(map[string]bool) // Deduplication by mint address

	for ml.running {
		for _, listener := range ml.listeners {
			select {
			case token, ok := <-listener.GetTokenChannel():
				if !ok {
					continue
				}

				// Deduplicate tokens by mint address
				mintKey := token.Mint.String()
				if seenTokens[mintKey] {
					ml.config.Logger.WithField("mint", mintKey).Debug("🔄 Duplicate token filtered")
					continue
				}

				seenTokens[mintKey] = true

				// Add source information
				token.SetMetadata("original_source", token.Source)
				token.SetMetadata("listener_type", listener.GetListenerType())

				select {
				case ml.tokenChan <- token:
					ml.config.Logger.WithFields(token.LogFields()).Debug("📦 Token aggregated from " + listener.GetListenerType())
				default:
					ml.config.Logger.Warn("⚠️ Token channel full, dropping event")
				}
			default:
				// Non-blocking, continue to next listener
			}
		}
	}
}

// FilteredListener wraps another listener with filtering capabilities
type FilteredListener struct {
	baseListener ListenerInterface
	config       *ListenerConfig
	tokenChan    chan *TokenEvent
	filters      []TokenFilter
	running      bool
}

// TokenFilter defines a filter function for token events
type TokenFilter func(*TokenEvent) bool

// NewFilteredListener creates a filtered listener
func NewFilteredListener(baseListener ListenerInterface, config *ListenerConfig, filters ...TokenFilter) *FilteredListener {
	return &FilteredListener{
		baseListener: baseListener,
		config:       config,
		tokenChan:    make(chan *TokenEvent, config.BufferSize),
		filters:      filters,
		running:      false,
	}
}

// Start starts the filtered listener
func (fl *FilteredListener) Start() error {
	fl.running = true

	// Start base listener
	if err := fl.baseListener.Start(); err != nil {
		return err
	}

	// Start filtering
	go fl.filterTokenEvents()

	fl.config.Logger.WithField("filter_count", len(fl.filters)).Info("🔍 FilteredListener started")
	return nil
}

// Stop stops the filtered listener
func (fl *FilteredListener) Stop() error {
	fl.running = false

	if err := fl.baseListener.Stop(); err != nil {
		return err
	}

	close(fl.tokenChan)
	fl.config.Logger.Info("🛑 FilteredListener stopped")
	return nil
}

// GetTokenChannel returns the filtered token channel
func (fl *FilteredListener) GetTokenChannel() <-chan *TokenEvent {
	return fl.tokenChan
}

// GetStats returns filtered listener statistics
func (fl *FilteredListener) GetStats() map[string]interface{} {
	baseStats := fl.baseListener.GetStats()
	baseStats["listener_type"] = "filtered_" + fl.baseListener.GetListenerType()
	baseStats["filter_count"] = len(fl.filters)
	baseStats["is_running"] = fl.running
	return baseStats
}

// GetListenerType returns the listener type
func (fl *FilteredListener) GetListenerType() string {
	return "filtered_" + fl.baseListener.GetListenerType()
}

// IsRunning returns whether the filtered listener is running
func (fl *FilteredListener) IsRunning() bool {
	return fl.running
}

// filterTokenEvents filters token events based on configured filters
func (fl *FilteredListener) filterTokenEvents() {
	for fl.running {
		select {
		case token, ok := <-fl.baseListener.GetTokenChannel():
			if !ok {
				return
			}

			// Apply all filters
			passed := true
			for _, filter := range fl.filters {
				if !filter(token) {
					passed = false
					break
				}
			}

			if passed {
				select {
				case fl.tokenChan <- token:
					fl.config.Logger.WithFields(token.LogFields()).Debug("✅ Token passed filters")
				default:
					fl.config.Logger.Warn("⚠️ Filtered token channel full, dropping event")
				}
			} else {
				fl.config.Logger.WithFields(token.LogFields()).Debug("🚫 Token filtered out")
			}
		}
	}
}

// Common filter functions

// NameFilter creates a filter that matches token names containing specified patterns
func NameFilter(patterns []string) TokenFilter {
	return func(token *TokenEvent) bool {
		if len(patterns) == 0 {
			return true
		}

		tokenName := strings.ToLower(token.Name)
		for _, pattern := range patterns {
			if strings.Contains(tokenName, strings.ToLower(pattern)) {
				return true
			}
		}
		return false
	}
}

// CreatorFilter creates a filter that matches specific creator addresses
func CreatorFilter(creators []string) TokenFilter {
	creatorMap := make(map[string]bool)
	for _, creator := range creators {
		creatorMap[creator] = true
	}

	return func(token *TokenEvent) bool {
		if len(creatorMap) == 0 {
			return true
		}
		return creatorMap[token.Creator.String()]
	}
}

// FreshnessFilter creates a filter that only allows fresh tokens
func FreshnessFilter(maxAge time.Duration) TokenFilter {
	return func(token *TokenEvent) bool {
		return token.GetAge() <= maxAge
	}
}

// ConfirmedFilter creates a filter that only allows confirmed transactions
func ConfirmedFilter() TokenFilter {
	return func(token *TokenEvent) bool {
		return token.IsConfirmed
	}
}

// SourceFilter creates a filter that only allows tokens from specific sources
func SourceFilter(allowedSources []string) TokenFilter {
	sourceMap := make(map[string]bool)
	for _, source := range allowedSources {
		sourceMap[source] = true
	}

	return func(token *TokenEvent) bool {
		if len(sourceMap) == 0 {
			return true
		}
		return sourceMap[token.Source]
	}
}
