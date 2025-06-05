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

type LogListener struct {
	wsClient      *client.WSClient
	rpcClient     *client.Client
	config        *ListenerConfig
	tokenChan     chan *TokenEvent
	ctx           context.Context
	cancel        context.CancelFunc
	pdaDerivation *utils.PumpFunPDADerivation
	running       bool
	startTime     time.Time

	// Statistics for monitoring
	stats *TokenEventStats
}

// Ensure LogListener implements ListenerInterface
var _ ListenerInterface = (*LogListener)(nil)

// NewLogListener creates a new pump.fun log listener
func NewLogListener(wsClient *client.WSClient, rpcClient *client.Client, cfg *config.Config, logger *logger.Logger) *LogListener {
	ctx, cancel := context.WithCancel(context.Background())

	conf := &ListenerConfig{
		Config:      cfg,
		Logger:      logger,
		Context:     ctx,
		BufferSize:  100,
		NetworkType: cfg.Network,
	}

	return &LogListener{
		wsClient:      wsClient,
		rpcClient:     rpcClient,
		config:        conf,
		tokenChan:     make(chan *TokenEvent, conf.BufferSize),
		ctx:           ctx,
		cancel:        cancel,
		pdaDerivation: utils.NewPumpFunPDADerivation(),
		running:       false,
		stats:         NewTokenEventStats(),
		startTime:     time.Now(),
	}
}

// Start starts the log listener
func (l *LogListener) Start() error {
	l.config.Logger.Info("🚀 Starting LogListener...")

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

	l.running = true
	l.startTime = time.Now()

	l.config.Logger.WithFields(logrus.Fields{
		"program_id":    pumpFunProgramID,
		"buffer_size":   l.config.BufferSize,
		"network":       l.config.NetworkType,
		"listener_type": "logs",
	}).Info("✅ LogListener started successfully")

	return nil
}

// Stop stops the log listener
func (l *LogListener) Stop() error {
	l.config.Logger.Info("🛑 Stopping LogListener...")

	l.running = false
	l.cancel()
	close(l.tokenChan)

	if err := l.wsClient.Disconnect(); err != nil {
		l.config.Logger.WithError(err).Error("Error disconnecting WebSocket")
		return err
	}

	l.config.Logger.Info("✅ LogListener stopped")
	return nil
}

// GetTokenChannel returns the token event channel
func (l *LogListener) GetTokenChannel() <-chan *TokenEvent {
	return l.tokenChan
}

// GetStats returns listener statistics
func (l *LogListener) GetStats() map[string]interface{} {
	baseStats := l.stats.GetStats()
	baseStats["listener_type"] = "logs"
	baseStats["is_running"] = l.running
	baseStats["uptime"] = time.Since(l.startTime).String()
	baseStats["network"] = l.config.NetworkType
	baseStats["websocket_stats"] = l.wsClient.GetConnectionStats()

	return baseStats
}

// GetListenerType returns the listener type
func (l *LogListener) GetListenerType() string {
	return "logs"
}

// IsRunning returns whether the listener is running
func (l *LogListener) IsRunning() bool {
	return l.running
}

// handleLogsNotification handles logs notifications from WebSocket with enhanced processing and timing
func (l *LogListener) handleLogsNotification(data interface{}) error {
	processingStart := time.Now() // Start timing immediately

	notification, ok := data.(client.LogsNotification)
	if !ok {
		return fmt.Errorf("invalid logs notification format")
	}

	// Skip if transaction failed
	if notification.Result.Value.Err != nil {
		l.config.Logger.WithField("signature", notification.Result.Value.Signature).Debug("Skipping failed transaction")
		return nil
	}

	signature := notification.Result.Value.Signature
	logs := notification.Result.Value.Logs
	slot := notification.Result.Context.Slot

	l.config.Logger.WithFields(logrus.Fields{
		"signature":  signature,
		"slot":       slot,
		"logs_count": len(logs),
		"timestamp":  processingStart.Format("15:04:05.000"),
	}).Debug("🔍 Processing logs notification")

	// Process program logs to extract token information
	tokenInfo, err := l.processProgramLogs(logs, signature, slot, processingStart)
	if err != nil {
		l.config.Logger.WithError(err).WithField("signature", signature).Debug("Failed to process program logs")
		return nil
	}

	if tokenInfo == nil {
		l.config.Logger.WithField("signature", signature).Debug("No token creation found in logs")
		return nil
	}

	// Calculate processing time
	processingTime := time.Since(processingStart)
	processingMs := processingTime.Milliseconds()
	tokenInfo.ProcessingDelayMs = processingMs

	// Set source
	tokenInfo.Source = "logs"

	// Update statistics
	l.stats.Update(tokenInfo)

	// Enhanced logging with timing information
	l.config.Logger.WithFields(tokenInfo.LogFields()).Info("🎯 New token discovered via logs")

	// Send token event to channel
	select {
	case l.tokenChan <- tokenInfo:
		l.config.Logger.WithFields(logrus.Fields{
			"mint":        tokenInfo.Mint.String(),
			"channel_lag": time.Since(tokenInfo.DiscoveredAt).Milliseconds(),
		}).Debug("✅ Token sent to processing channel")
	case <-l.ctx.Done():
		return nil
	default:
		l.config.Logger.WithFields(logrus.Fields{
			"mint":   tokenInfo.Mint.String(),
			"age_ms": time.Since(tokenInfo.DiscoveredAt).Milliseconds(),
		}).Warn("⚠️ Token channel full, dropping event")
	}

	return nil
}

// processProgramLogs processes program logs to extract token creation information with timing
func (l *LogListener) processProgramLogs(logs []string, signature string, slot uint64, discoveryTime time.Time) (*TokenEvent, error) {
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
func (l *LogListener) extractTokenFromData(dataStr string, signature string, slot uint64, discoveryTime time.Time) (*TokenEvent, error) {
	now := time.Now()
	tokenEvent := &TokenEvent{
		Signature:    signature,
		Slot:         slot,
		Timestamp:    now,           // Current timestamp
		DiscoveredAt: discoveryTime, // When we started processing this event
		Source:       "logs",
		Metadata:     make(map[string]interface{}),
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

	// Store raw data for debugging
	tokenEvent.RawData = make([]byte, len(data))
	copy(tokenEvent.RawData, data)

	// Add logs-specific metadata
	tokenEvent.SetMetadata("log_data", dataStr)
	tokenEvent.SetMetadata("logs_source", true)
	tokenEvent.AddProcessingStep("logs_parsed")

	return tokenEvent, nil
}
