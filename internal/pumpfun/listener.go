package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"pump-fun-bot-go/internal/client"
	"strings"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/pkg/utils"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

const CREATE_DISCRIMINATOR uint64 = 8530921459188068891

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
	Timestamp              time.Time        `json:"timestamp"`           // NEW: Время обнаружения токена
	DiscoveredAt           time.Time        `json:"discovered_at"`       // NEW: Время когда токен был обработан
	ProcessingDelayMs      int64            `json:"processing_delay_ms"` // NEW: Задержка обработки в мс
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

// Listener listens for new pump.fun tokens
type Listener struct {
	wsClient      *client.WSClient
	rpcClient     *client.Client
	logger        *logger.Logger
	config        *config.Config
	tokenChan     chan *TokenEvent
	ctx           context.Context
	cancel        context.CancelFunc
	pdaDerivation *utils.PumpFunPDADerivation

	// Statistics for monitoring
	tokensDiscovered    int64
	tokensProcessed     int64
	averageProcessingMs float64
	fastestProcessingMs int64
	slowestProcessingMs int64
}

// NewListener creates a new pump.fun listener
func NewListener(wsClient *client.WSClient, rpcClient *client.Client, logger *logger.Logger, cfg *config.Config) *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	return &Listener{
		wsClient:            wsClient,
		rpcClient:           rpcClient,
		logger:              logger,
		config:              cfg,
		tokenChan:           make(chan *TokenEvent, 100),
		ctx:                 ctx,
		cancel:              cancel,
		pdaDerivation:       utils.NewPumpFunPDADerivation(),
		fastestProcessingMs: 999999,
	}
}

// Start starts the listener
func (l *Listener) Start() error {
	l.logger.Info("Starting pump.fun token listener...")

	// Connect WebSocket
	if err := l.wsClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	// Subscribe to pump.fun program logs
	pumpFunProgramID := base58.Encode(config.PumpFunProgramID)

	_, err := l.wsClient.SubscribeToLogs(pumpFunProgramID, l.handleLogsNotification)
	if err != nil {
		return fmt.Errorf("failed to subscribe to logs: %w", err)
	}

	return nil
}

// Stop stops the listener
func (l *Listener) Stop() error {
	l.cancel()
	close(l.tokenChan)
	return l.wsClient.Disconnect()
}

// GetTokenChannel returns the token event channel
func (l *Listener) GetTokenChannel() <-chan *TokenEvent {
	return l.tokenChan
}

// GetStats returns listener statistics
func (l *Listener) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"tokens_discovered":     l.tokensDiscovered,
		"tokens_processed":      l.tokensProcessed,
		"average_processing_ms": l.averageProcessingMs,
		"fastest_processing_ms": l.fastestProcessingMs,
		"slowest_processing_ms": l.slowestProcessingMs,
	}
}

// handleLogsNotification handles logs notifications from WebSocket with enhanced processing and timing
func (l *Listener) handleLogsNotification(data interface{}) error {
	processingStart := time.Now() // Start timing immediately

	notification, ok := data.(client.LogsNotification)
	if !ok {
		return fmt.Errorf("invalid logs notification format")
	}

	// Skip if transaction failed
	if notification.Result.Value.Err != nil {
		l.logger.WithField("signature", notification.Result.Value.Signature).Debug("Skipping failed transaction")
		return nil
	}

	signature := notification.Result.Value.Signature
	logs := notification.Result.Value.Logs
	slot := notification.Result.Context.Slot

	l.logger.WithFields(logrus.Fields{
		"signature":  signature,
		"slot":       slot,
		"logs_count": len(logs),
		"timestamp":  processingStart.Format("15:04:05.000"),
	}).Debug("🔍 Processing logs notification")

	// Process program logs to extract token information
	tokenInfo, err := l.processProgramLogs(logs, signature, slot, processingStart)
	if err != nil {
		l.logger.WithError(err).WithField("signature", signature).Debug("Failed to process program logs")
		return nil
	}

	if tokenInfo == nil {
		l.logger.WithField("signature", signature).Debug("No token creation found in logs")
		return nil
	}

	// Calculate processing time
	processingTime := time.Since(processingStart)
	processingMs := processingTime.Milliseconds()
	tokenInfo.ProcessingDelayMs = processingMs

	// Update statistics
	l.updateProcessingStats(processingMs)

	// Enhanced logging with timing information
	l.logger.WithFields(logrus.Fields{
		"mint":                     tokenInfo.Mint.String(),
		"name":                     tokenInfo.Name,
		"symbol":                   tokenInfo.Symbol,
		"creator":                  tokenInfo.Creator.String(),
		"creator_vault":            tokenInfo.CreatorVault.String(),
		"bonding_curve":            tokenInfo.BondingCurve.String(),
		"associated_bonding_curve": tokenInfo.AssociatedBondingCurve.String(),
		"processing_ms":            processingMs,
		"discovered_at":            tokenInfo.DiscoveredAt.Format("15:04:05.000"),
		"timestamp":                tokenInfo.Timestamp.Format("15:04:05.000"),
	}).Info("🔍 New token discovered")

	// Send token event to channel
	select {
	case l.tokenChan <- tokenInfo:
		l.tokensProcessed++
		l.logger.WithFields(logrus.Fields{
			"mint":        tokenInfo.Mint.String(),
			"channel_lag": time.Since(tokenInfo.DiscoveredAt).Milliseconds(),
		}).Debug("✅ Token sent to processing channel")
	case <-l.ctx.Done():
		return nil
	default:
		l.logger.WithFields(logrus.Fields{
			"mint":   tokenInfo.Mint.String(),
			"age_ms": time.Since(tokenInfo.DiscoveredAt).Milliseconds(),
		}).Warn("⚠️ Token channel full, dropping event")
	}

	return nil
}

// updateProcessingStats updates processing statistics
func (l *Listener) updateProcessingStats(processingMs int64) {
	l.tokensDiscovered++

	// Update fastest processing time
	if processingMs < l.fastestProcessingMs {
		l.fastestProcessingMs = processingMs
	}

	// Update slowest processing time
	if processingMs > l.slowestProcessingMs {
		l.slowestProcessingMs = processingMs
	}

	// Update average processing time (exponential moving average)
	if l.tokensDiscovered == 1 {
		l.averageProcessingMs = float64(processingMs)
	} else {
		alpha := 0.1 // Smoothing factor
		l.averageProcessingMs = l.averageProcessingMs*(1-alpha) + float64(processingMs)*alpha
	}
}

// processProgramLogs processes program logs to extract token creation information with timing
func (l *Listener) processProgramLogs(logs []string, signature string, slot uint64, discoveryTime time.Time) (*TokenEvent, error) {
	var processLogs bool

	// Check if this is a token creation
	for _, log := range logs {
		if strings.Contains(log, "Program log: Instruction: Create") {
			processLogs = true
			break
		}
	}

	// Skip swaps as the first condition may pass them
	for _, log := range logs {
		if strings.Contains(log, "Program log: Instruction: CreateTokenAccount") {
			processLogs = false
			break
		}
	}

	if !processLogs {
		return nil, nil
	}

	for _, log := range logs {
		// Check for pump.fun program log patterns
		if strings.Contains(log, "Program data: ") {
			logContent := strings.TrimPrefix(log, "Program data: ")
			tokenEvent, err := l.extractTokenFromData(logContent, signature, slot, discoveryTime)
			if err == nil {
				return tokenEvent, err
			}
		}
	}

	return nil, nil
}

// extractTokenFromData extracts token information from program data with timing
func (l *Listener) extractTokenFromData(dataStr string, signature string, slot uint64, discoveryTime time.Time) (*TokenEvent, error) {
	now := time.Now()
	tokenEvent := &TokenEvent{
		Signature:    signature,
		Slot:         slot,
		Timestamp:    now,           // Current timestamp
		DiscoveredAt: discoveryTime, // When we started processing this event
	}

	data, err := utils.DecodeDataString(dataStr)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("too short for discriminator")
	}
	discriminator := binary.LittleEndian.Uint64(data[:8])
	if discriminator != CREATE_DISCRIMINATOR {
		return nil, fmt.Errorf("not a CreateInstruction discriminator: %x", discriminator)
	}
	offset := 8
	readString := func() (string, error) {
		if offset+4 > len(data) {
			return "", fmt.Errorf("not enough data for string length")
		}
		l := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		if offset+int(l) > len(data) {
			return "", fmt.Errorf("not enough data for string value")
		}
		val := string(data[offset : offset+int(l)])
		offset += int(l)
		return val, nil
	}
	readPubKey := func() (solana.PublicKey, error) {
		if offset+32 > len(data) {
			return solana.PublicKey{}, fmt.Errorf("not enough data for pubkey")
		}
		val := solana.PublicKeyFromBytes(data[offset : offset+32])
		offset += 32
		return val, nil
	}

	if tokenEvent.Name, err = readString(); err != nil {
		return nil, err
	}
	if tokenEvent.Symbol, err = readString(); err != nil {
		return nil, err
	}
	if tokenEvent.URI, err = readString(); err != nil {
		return nil, err
	}
	if tokenEvent.Mint, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.BondingCurve, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.User, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.Creator, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.CreatorVault, _, err = l.pdaDerivation.DeriveCreatorVault(tokenEvent.Creator); err != nil {
		return nil, err
	}
	if tokenEvent.AssociatedBondingCurve, _, err = l.pdaDerivation.DeriveAssociatedBondingCurve(tokenEvent.Mint, tokenEvent.BondingCurve); err != nil {
		return nil, err
	}

	return tokenEvent, nil
}
