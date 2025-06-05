// internal/logger/token_event.go
package logger

import (
	"fmt"
	"github.com/gagliardetto/solana-go"
	"time"

	"github.com/sirupsen/logrus"
)

// TokenEventDetails represents detailed information about a token event for logging
type TokenEventDetails struct {
	// Basic token information
	Signature              string  `json:"signature"`
	Slot                   uint64  `json:"slot"`
	BlockTime              int64   `json:"block_time"`
	Mint                   string  `json:"mint"`
	BondingCurve           string  `json:"bonding_curve"`
	AssociatedBondingCurve string  `json:"associated_bonding_curve"`
	Creator                string  `json:"creator"`
	CreatorVault           string  `json:"creator_vault"`
	User                   string  `json:"user"`
	Name                   string  `json:"name"`
	Symbol                 string  `json:"symbol"`
	URI                    string  `json:"uri"`
	InitialPrice           float64 `json:"initial_price"`

	// Timing information
	Timestamp         time.Time `json:"timestamp"`
	DiscoveredAt      time.Time `json:"discovered_at"`
	ProcessingDelayMs int64     `json:"processing_delay_ms"`

	// Calculated timing metrics
	AgeMs                int64 `json:"age_ms"`
	TimeSinceBlockTimeMs int64 `json:"time_since_block_time_ms"`
	DiscoveryLatencyMs   int64 `json:"discovery_latency_ms"`

	// Additional context
	EventType string `json:"event_type"`
	Source    string `json:"source"`
}

// LogTokenEvent logs a comprehensive TokenEvent with all timing and metadata information
func (l *Logger) LogTokenEvent(tokenEvent interface{}, eventType string, additionalFields map[string]interface{}) {
	// Convert the token event to our internal structure
	details := l.extractTokenEventDetails(tokenEvent, eventType)

	// Build the base log fields
	logFields := logrus.Fields{
		"event":                    "token_discovered",
		"event_type":               details.EventType,
		"signature":                details.Signature,
		"slot":                     details.Slot,
		"block_time":               details.BlockTime,
		"mint":                     details.Mint,
		"bonding_curve":            details.BondingCurve,
		"associated_bonding_curve": details.AssociatedBondingCurve,
		"creator":                  details.Creator,
		"creator_vault":            details.CreatorVault,
		"user":                     details.User,
		"name":                     details.Name,
		"symbol":                   details.Symbol,
		"uri":                      details.URI,
		"initial_price":            details.InitialPrice,
		"timestamp":                details.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"discovered_at":            details.DiscoveredAt.Format("2006-01-02T15:04:05.000Z07:00"),
		"processing_delay_ms":      details.ProcessingDelayMs,
		"age_ms":                   details.AgeMs,
		"time_since_block_time_ms": details.TimeSinceBlockTimeMs,
		"discovery_latency_ms":     details.DiscoveryLatencyMs,
		"source":                   details.Source,
	}

	// Add any additional fields provided
	for key, value := range additionalFields {
		logFields[key] = value
	}

	// Determine log level based on timing and importance
	logLevel := l.determineTokenEventLogLevel(details)

	// Create log message with enhanced formatting
	message := l.formatTokenEventMessage(details)

	// Log based on determined level
	switch logLevel {
	case logrus.DebugLevel:
		l.WithFields(logFields).Debug(message)
	case logrus.InfoLevel:
		l.WithFields(logFields).Info(message)
	case logrus.WarnLevel:
		l.WithFields(logFields).Warn(message)
	case logrus.ErrorLevel:
		l.WithFields(logFields).Error(message)
	default:
		l.WithFields(logFields).Info(message)
	}
}

// LogTokenEventWithTiming logs a token event with specific focus on timing metrics
func (l *Logger) LogTokenEventWithTiming(tokenEvent interface{}, processingStart time.Time, additionalContext map[string]interface{}) {
	details := l.extractTokenEventDetails(tokenEvent, "timing_analysis")

	// Calculate additional timing metrics
	now := time.Now()
	totalProcessingTime := now.Sub(processingStart)

	timingFields := logrus.Fields{
		"event":                "token_timing_analysis",
		"mint":                 details.Mint,
		"name":                 details.Name,
		"symbol":               details.Symbol,
		"processing_start":     processingStart.Format("15:04:05.000"),
		"processing_end":       now.Format("15:04:05.000"),
		"total_processing_ms":  totalProcessingTime.Milliseconds(),
		"discovery_delay_ms":   details.ProcessingDelayMs,
		"age_ms":               details.AgeMs,
		"discovery_latency_ms": details.DiscoveryLatencyMs,
		"timestamp":            details.Timestamp.Format("15:04:05.000"),
		"discovered_at":        details.DiscoveredAt.Format("15:04:05.000"),
	}

	// Add context fields
	for key, value := range additionalContext {
		timingFields[key] = value
	}

	// Determine if timing is concerning
	isFast := totalProcessingTime.Milliseconds() < 100
	isSlow := totalProcessingTime.Milliseconds() > 1000
	isStale := details.AgeMs > 5000

	timingFields["is_fast_processing"] = isFast
	timingFields["is_slow_processing"] = isSlow
	timingFields["is_stale_token"] = isStale

	var message string
	var logLevel logrus.Level

	switch {
	case isStale:
		message = fmt.Sprintf("⏰ STALE TOKEN: %s (%s) - Age: %dms, Processing: %dms",
			details.Name, details.Symbol, details.AgeMs, totalProcessingTime.Milliseconds())
		logLevel = logrus.WarnLevel
	case isFast:
		message = fmt.Sprintf("⚡ FAST PROCESSING: %s (%s) - Processing: %dms, Age: %dms",
			details.Name, details.Symbol, totalProcessingTime.Milliseconds(), details.AgeMs)
		logLevel = logrus.InfoLevel
	case isSlow:
		message = fmt.Sprintf("🐌 SLOW PROCESSING: %s (%s) - Processing: %dms, Age: %dms",
			details.Name, details.Symbol, totalProcessingTime.Milliseconds(), details.AgeMs)
		logLevel = logrus.WarnLevel
	default:
		message = fmt.Sprintf("⏱️ TIMING: %s (%s) - Processing: %dms, Age: %dms",
			details.Name, details.Symbol, totalProcessingTime.Milliseconds(), details.AgeMs)
		logLevel = logrus.DebugLevel
	}

	// Log with appropriate level
	switch logLevel {
	case logrus.DebugLevel:
		l.WithFields(timingFields).Debug(message)
	case logrus.InfoLevel:
		l.WithFields(timingFields).Info(message)
	case logrus.WarnLevel:
		l.WithFields(timingFields).Warn(message)
	case logrus.ErrorLevel:
		l.WithFields(timingFields).Error(message)
	}
}

// LogTokenEventSummary logs a summary of token events for performance monitoring
func (l *Logger) LogTokenEventSummary(stats map[string]interface{}) {
	summaryFields := logrus.Fields{
		"event": "token_events_summary",
	}

	// Add all provided stats
	for key, value := range stats {
		summaryFields[key] = value
	}

	// Extract key metrics for message
	tokensDiscovered, _ := stats["tokens_discovered"].(int64)
	tokensProcessed, _ := stats["tokens_processed"].(int64)
	averageLatency, _ := stats["average_processing_ms"].(float64)
	fastestProcessing, _ := stats["fastest_processing_ms"].(int64)

	message := fmt.Sprintf("📊 TOKEN EVENTS SUMMARY: Discovered: %d, Processed: %d, Avg Latency: %.1fms, Fastest: %dms",
		tokensDiscovered, tokensProcessed, averageLatency, fastestProcessing)

	l.WithFields(summaryFields).Info(message)
}

// LogTokenEventError logs errors related to token event processing
func (l *Logger) LogTokenEventError(tokenEvent interface{}, operation string, err error, context map[string]interface{}) {
	details := l.extractTokenEventDetails(tokenEvent, "error")

	errorFields := logrus.Fields{
		"event":     "token_event_error",
		"operation": operation,
		"mint":      details.Mint,
		"name":      details.Name,
		"symbol":    details.Symbol,
		"signature": details.Signature,
		"error":     err.Error(),
	}

	// Add context fields
	for key, value := range context {
		errorFields[key] = value
	}

	message := fmt.Sprintf("❌ TOKEN EVENT ERROR: %s failed for %s (%s) - %v",
		operation, details.Name, details.Symbol, err)

	l.WithFields(errorFields).WithError(err).Error(message)
}

// extractTokenEventDetails extracts details from a token event interface
func (l *Logger) extractTokenEventDetails(tokenEvent interface{}, eventType string) *TokenEventDetails {
	details := &TokenEventDetails{
		EventType: eventType,
		Source:    "pump_fun",
		Timestamp: time.Now(),
	}

	// Type assertion to handle different token event types
	// This assumes the tokenEvent has similar fields to the TokenEvent struct
	// You might need to adjust this based on your actual TokenEvent structure

	if te, ok := tokenEvent.(*TokenEventStruct); ok {
		details.Signature = te.Signature
		details.Slot = te.Slot
		details.BlockTime = te.BlockTime
		details.Name = te.Name
		details.Symbol = te.Symbol
		details.URI = te.URI
		details.InitialPrice = te.InitialPrice
		details.Timestamp = te.Timestamp
		details.DiscoveredAt = te.DiscoveredAt
		details.ProcessingDelayMs = te.ProcessingDelayMs

		// Convert public keys to strings
		if te.Mint != nil {
			details.Mint = te.Mint.String()
		}
		if te.BondingCurve != nil {
			details.BondingCurve = te.BondingCurve.String()
		}
		if te.AssociatedBondingCurve != nil {
			details.AssociatedBondingCurve = te.AssociatedBondingCurve.String()
		}
		if te.Creator != nil {
			details.Creator = te.Creator.String()
		}
		if te.CreatorVault != nil {
			details.CreatorVault = te.CreatorVault.String()
		}
		if te.User != nil {
			details.User = te.User.String()
		}

		// Calculate timing metrics
		now := time.Now()
		details.AgeMs = now.Sub(te.DiscoveredAt).Milliseconds()
		details.DiscoveryLatencyMs = te.DiscoveredAt.Sub(te.Timestamp).Milliseconds()

		if te.BlockTime > 0 {
			blockTime := time.Unix(te.BlockTime, 0)
			details.TimeSinceBlockTimeMs = now.Sub(blockTime).Milliseconds()
		}
	}

	return details
}

// determineTokenEventLogLevel determines appropriate log level based on event characteristics
func (l *Logger) determineTokenEventLogLevel(details *TokenEventDetails) logrus.Level {
	// High priority tokens (fast processing, new tokens)
	if details.ProcessingDelayMs < 50 && details.AgeMs < 1000 {
		return logrus.InfoLevel
	}

	// Stale or slow processing tokens
	if details.AgeMs > 5000 || details.ProcessingDelayMs > 1000 {
		return logrus.WarnLevel
	}

	// Normal tokens
	if details.ProcessingDelayMs < 500 {
		return logrus.InfoLevel
	}

	// Default to debug for less important events
	return logrus.DebugLevel
}

// formatTokenEventMessage creates a formatted message for token events
func (l *Logger) formatTokenEventMessage(details *TokenEventDetails) string {
	// Create emoji indicator based on timing
	var indicator string
	switch {
	case details.ProcessingDelayMs < 50:
		indicator = "⚡"
	case details.ProcessingDelayMs < 200:
		indicator = "🚀"
	case details.ProcessingDelayMs < 500:
		indicator = "🎯"
	case details.ProcessingDelayMs < 1000:
		indicator = "⏳"
	default:
		indicator = "🐌"
	}

	// Add stale indicator if token is old
	if details.AgeMs > 5000 {
		indicator = "⏰" + indicator
	}

	return fmt.Sprintf("%s TOKEN DISCOVERED: %s (%s) | Mint: %s | Processing: %dms | Age: %dms | Creator: %s",
		indicator,
		details.Name,
		details.Symbol,
		details.Mint,
		details.ProcessingDelayMs,
		details.AgeMs,
		details.Creator)
}

// TokenEventStruct represents the structure we expect from TokenEvent
// This should match your actual TokenEvent struct from internal/pumpfun/listener.go
type TokenEventStruct struct {
	Signature              string
	Slot                   uint64
	BlockTime              int64
	Mint                   *solana.PublicKey
	BondingCurve           *solana.PublicKey
	AssociatedBondingCurve *solana.PublicKey
	Creator                *solana.PublicKey
	CreatorVault           *solana.PublicKey
	User                   *solana.PublicKey
	Name                   string
	Symbol                 string
	URI                    string
	InitialPrice           float64
	Timestamp              time.Time
	DiscoveredAt           time.Time
	ProcessingDelayMs      int64
}

// Helper functions for specific token event logging scenarios

// LogTokenEventDiscovery logs when a new token is first discovered
func (l *Logger) LogTokenEventDiscovery(tokenEvent interface{}) {
	l.LogTokenEvent(tokenEvent, "discovery", map[string]interface{}{
		"discovery_source": "websocket",
		"discovery_method": "logs_subscription",
	})
}

// LogTokenEventFiltered logs when a token passes or fails filters
func (l *Logger) LogTokenEventFiltered(tokenEvent interface{}, filterType string, filterResult bool, reason string) {
	l.LogTokenEvent(tokenEvent, "filtered", map[string]interface{}{
		"filter_type":   filterType,
		"filter_passed": filterResult,
		"filter_reason": reason,
	})
}

// LogTokenEventTrade logs when a token event leads to a trade
func (l *Logger) LogTokenEventTrade(tokenEvent interface{}, tradeType string, tradeResult map[string]interface{}) {
	additionalFields := map[string]interface{}{
		"trade_type": tradeType,
	}

	// Merge trade result fields
	for key, value := range tradeResult {
		additionalFields["trade_"+key] = value
	}

	l.LogTokenEvent(tokenEvent, "trade", additionalFields)
}

// LogTokenEventRejected logs when a token event is rejected for trading
func (l *Logger) LogTokenEventRejected(tokenEvent interface{}, rejectionReason string, context map[string]interface{}) {
	additionalFields := map[string]interface{}{
		"rejection_reason": rejectionReason,
		"rejected_at":      time.Now().Format("15:04:05.000"),
	}

	// Merge context fields
	for key, value := range context {
		additionalFields[key] = value
	}

	l.LogTokenEvent(tokenEvent, "rejected", additionalFields)
}
