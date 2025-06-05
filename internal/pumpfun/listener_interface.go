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
	defer fl.config.Logger.Debug("🛑 Filter goroutine stopped")

	fl.config.Logger.Info("🔍 Starting token filtering goroutine")

	for fl.running {
		select {
		case token, ok := <-fl.baseListener.GetTokenChannel():
			if !ok {
				fl.config.Logger.Info("📬 Base listener channel closed, stopping filter")
				return
			}

			fl.config.Logger.WithFields(map[string]interface{}{
				"mint":   token.Mint.String(),
				"name":   token.Name,
				"symbol": token.Symbol,
				"source": token.Source,
			}).Debug("🔍 Received token for filtering")

			// Apply all filters
			passed := true
			for i, filter := range fl.filters {
				if !filter(token) {
					fl.config.Logger.WithFields(map[string]interface{}{
						"mint":      token.Mint.String(),
						"filter_id": i,
						"name":      token.Name,
						"symbol":    token.Symbol,
					}).Debug("🚫 Token filtered out")
					passed = false
					break
				}
			}

			if passed {
				select {
				case fl.tokenChan <- token:
					fl.config.Logger.WithFields(map[string]interface{}{
						"mint":   token.Mint.String(),
						"name":   token.Name,
						"symbol": token.Symbol,
					}).Info("✅ Token passed all filters")
				default:
					fl.config.Logger.WithFields(map[string]interface{}{
						"mint":   token.Mint.String(),
						"age_ms": token.GetAgeMs(),
					}).Warn("⚠️ Filtered token channel full, dropping event")
				}
			}

		case <-time.After(5 * time.Second):
			// Периодически логируем что мы живы
			fl.config.Logger.Debug("🔍 Filter goroutine is alive, waiting for tokens...")
			if !fl.running {
				fl.config.Logger.Debug("🛑 Filter goroutine stopping due to running=false")
				return
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
