package pumpfun

import (
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"

	"github.com/gagliardetto/solana-go"
)

// TokenEvent represents a new token creation event with timing information
type TokenEvent struct {
	Signature              string           `json:"signature"`
	Slot                   uint64           `json:"slot"`
	BlockTime              int64            `json:"block_time"`
	Mint                   solana.PublicKey `json:"mint"`
	BondingCurve           solana.PublicKey `json:"bonding_curve"`
	AssociatedBondingCurve solana.PublicKey `json:"associated_bonding_curve"`
	Creator                solana.PublicKey `json:"creator"`
	CreatorVault           solana.PublicKey `json:"creator_vault"`
	User                   solana.PublicKey `json:"user"`
	Name                   string           `json:"name"`
	Symbol                 string           `json:"symbol"`
	URI                    string           `json:"uri"`
	InitialPrice           float64          `json:"initial_price"`
	Timestamp              time.Time        `json:"timestamp"`           // Время обнаружения токена
	DiscoveredAt           time.Time        `json:"discovered_at"`       // Время когда токен был обработан
	ProcessingDelayMs      int64            `json:"processing_delay_ms"` // Задержка обработки в мс

	// Additional metadata
	Source      string                 `json:"source"`       // "logs" or "blocks"
	RawData     []byte                 `json:"raw_data"`     // Raw instruction/transaction data
	Metadata    map[string]interface{} `json:"metadata"`     // Additional metadata
	IsConfirmed bool                   `json:"is_confirmed"` // Whether the transaction is confirmed
}

// GetAge returns the age of the token since discovery
func (te *TokenEvent) GetAge() time.Duration {
	return time.Since(te.DiscoveredAt)
}

// GetTimeSinceBlockTime returns time since block time
func (te *TokenEvent) GetTimeSinceBlockTime() time.Duration {
	if te.BlockTime == 0 {
		return time.Duration(0)
	}
	blockTimeStamp := time.Unix(te.BlockTime, 0)
	return time.Since(blockTimeStamp)
}

// IsStale checks if token is too old based on configuration
func (te *TokenEvent) IsStale(cfg *config.Config) bool {
	age := te.GetAge()
	return !cfg.IsTokenAgeValid(age)
}

// ShouldWaitForDelay checks if we should wait more before trading
func (te *TokenEvent) ShouldWaitForDelay(cfg *config.Config) bool {
	timeSinceDiscovery := time.Since(te.DiscoveredAt)
	return !cfg.IsDiscoveryDelayValid(timeSinceDiscovery)
}

// GetAgeMs returns age in milliseconds
func (te *TokenEvent) GetAgeMs() int64 {
	return te.GetAge().Milliseconds()
}

// GetTimeSinceBlockTimeMs returns time since block time in milliseconds
func (te *TokenEvent) GetTimeSinceBlockTimeMs() int64 {
	return te.GetTimeSinceBlockTime().Milliseconds()
}

// IsVeryFresh checks if token is very fresh (less than 1 second old)
func (te *TokenEvent) IsVeryFresh() bool {
	return te.GetAge() < time.Second
}

// IsFresh checks if token is fresh (less than 5 seconds old)
func (te *TokenEvent) IsFresh() bool {
	return te.GetAge() < 5*time.Second
}

// SetMetadata sets additional metadata
func (te *TokenEvent) SetMetadata(key string, value interface{}) {
	if te.Metadata == nil {
		te.Metadata = make(map[string]interface{})
	}
	te.Metadata[key] = value
}

// GetMetadata gets metadata value
func (te *TokenEvent) GetMetadata(key string) (interface{}, bool) {
	if te.Metadata == nil {
		return nil, false
	}
	value, exists := te.Metadata[key]
	return value, exists
}

// AddProcessingStep adds a processing step timestamp to metadata
func (te *TokenEvent) AddProcessingStep(step string) {
	te.SetMetadata("step_"+step, time.Now())
}

// GetProcessingStep gets processing step timestamp
func (te *TokenEvent) GetProcessingStep(step string) (time.Time, bool) {
	if value, exists := te.GetMetadata("step_" + step); exists {
		if timestamp, ok := value.(time.Time); ok {
			return timestamp, true
		}
	}
	return time.Time{}, false
}

// String returns a string representation of the token event
func (te *TokenEvent) String() string {
	return fmt.Sprintf("Token{Mint: %s, Name: %s, Symbol: %s, Creator: %s, Age: %dms, Source: %s}",
		te.Mint.String()[:8]+"...",
		te.Name,
		te.Symbol,
		te.Creator.String()[:8]+"...",
		te.GetAgeMs(),
		te.Source,
	)
}

// LogFields returns structured log fields for this token event
func (te *TokenEvent) LogFields() map[string]interface{} {
	return map[string]interface{}{
		"signature":                te.Signature,
		"slot":                     te.Slot,
		"block_time":               te.BlockTime,
		"mint":                     te.Mint.String(),
		"bonding_curve":            te.BondingCurve.String(),
		"associated_bonding_curve": te.AssociatedBondingCurve.String(),
		"creator":                  te.Creator.String(),
		"creator_vault":            te.CreatorVault.String(),
		"user":                     te.User.String(),
		"name":                     te.Name,
		"symbol":                   te.Symbol,
		"uri":                      te.URI,
		"initial_price":            te.InitialPrice,
		"timestamp":                te.Timestamp,
		"discovered_at":            te.DiscoveredAt,
		"processing_delay_ms":      te.ProcessingDelayMs,
		"age_ms":                   te.GetAgeMs(),
		"time_since_block_time_ms": te.GetTimeSinceBlockTimeMs(),
		"source":                   te.Source,
		"is_confirmed":             te.IsConfirmed,
		"is_fresh":                 te.IsFresh(),
		"is_very_fresh":            te.IsVeryFresh(),
	}
}

// TokenEventStats represents statistics about token events
type TokenEventStats struct {
	TotalEvents         int64            `json:"total_events"`
	EventsBySource      map[string]int64 `json:"events_by_source"`
	AverageProcessingMs float64          `json:"average_processing_ms"`
	FastestProcessingMs int64            `json:"fastest_processing_ms"`
	SlowestProcessingMs int64            `json:"slowest_processing_ms"`
	FreshEventsCount    int64            `json:"fresh_events_count"`
	StaleEventsCount    int64            `json:"stale_events_count"`
	LastEventTime       time.Time        `json:"last_event_time"`
}

// NewTokenEventStats creates new token event statistics
func NewTokenEventStats() *TokenEventStats {
	return &TokenEventStats{
		EventsBySource:      make(map[string]int64),
		FastestProcessingMs: 999999,
	}
}

// Update updates statistics with a new token event
func (stats *TokenEventStats) Update(event *TokenEvent) {
	stats.TotalEvents++
	stats.EventsBySource[event.Source]++
	stats.LastEventTime = event.DiscoveredAt

	// Update processing time statistics
	if event.ProcessingDelayMs < stats.FastestProcessingMs {
		stats.FastestProcessingMs = event.ProcessingDelayMs
	}

	if event.ProcessingDelayMs > stats.SlowestProcessingMs {
		stats.SlowestProcessingMs = event.ProcessingDelayMs
	}

	// Update average processing time (exponential moving average)
	if stats.TotalEvents == 1 {
		stats.AverageProcessingMs = float64(event.ProcessingDelayMs)
	} else {
		alpha := 0.1
		stats.AverageProcessingMs = stats.AverageProcessingMs*(1-alpha) + float64(event.ProcessingDelayMs)*alpha
	}

	// Update freshness counters
	if event.IsFresh() {
		stats.FreshEventsCount++
	} else {
		stats.StaleEventsCount++
	}
}

// GetStats returns the current statistics as a map
func (stats *TokenEventStats) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_events":          stats.TotalEvents,
		"events_by_source":      stats.EventsBySource,
		"average_processing_ms": stats.AverageProcessingMs,
		"fastest_processing_ms": stats.FastestProcessingMs,
		"slowest_processing_ms": stats.SlowestProcessingMs,
		"fresh_events_count":    stats.FreshEventsCount,
		"stale_events_count":    stats.StaleEventsCount,
		"last_event_time":       stats.LastEventTime,
		"fresh_percentage":      float64(stats.FreshEventsCount) / float64(stats.TotalEvents) * 100,
	}
}
