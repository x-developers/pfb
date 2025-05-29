package pumpfun

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

// TokenEvent represents a new token creation event
type TokenEvent struct {
	Signature              string    `json:"signature"`
	Slot                   uint64    `json:"slot"`
	BlockTime              int64     `json:"block_time"`
	Mint                   string    `json:"mint"`
	BondingCurve           string    `json:"bonding_curve"`
	AssociatedBondingCurve string    `json:"associated_bonding_curve"`
	Creator                string    `json:"creator"`
	Name                   string    `json:"name"`
	Symbol                 string    `json:"symbol"`
	URI                    string    `json:"uri"`
	InitialPrice           float64   `json:"initial_price"`
	Timestamp              time.Time `json:"timestamp"`
}

// Listener listens for new pump.fun tokens
type Listener struct {
	wsClient  *solana.WSClient
	rpcClient *solana.Client
	logger    *logger.Logger
	config    *config.Config
	tokenChan chan *TokenEvent
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewListener creates a new pump.fun listener
func NewListener(wsClient *solana.WSClient, rpcClient *solana.Client, logger *logger.Logger, cfg *config.Config) *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	return &Listener{
		wsClient:  wsClient,
		rpcClient: rpcClient,
		logger:    logger,
		config:    cfg,
		tokenChan: make(chan *TokenEvent, 100),
		ctx:       ctx,
		cancel:    cancel,
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

	l.logger.WithField("program_id", pumpFunProgramID).Info("Subscribed to pump.fun logs")

	// Start token event processor
	go l.processTokenEvents()

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

// handleLogsNotification handles logs notifications from WebSocket
func (l *Listener) handleLogsNotification(data interface{}) error {
	notification, ok := data.(solana.LogsNotification)
	if !ok {
		return fmt.Errorf("invalid logs notification format")
	}

	// Skip if transaction failed
	if notification.Result.Value.Err != nil {
		return nil
	}

	signature := notification.Result.Value.Signature
	logs := notification.Result.Value.Logs
	slot := notification.Result.Context.Slot

	l.logger.WithFields(logrus.Fields{
		"signature": signature,
		"slot":      slot,
		"logs":      len(logs),
	}).Debug("Processing logs notification")

	// Check if this is a create instruction
	if !l.isCreateInstruction(logs) {
		return nil // Not a token creation, skip
	}

	// Process the transaction to extract token information
	go l.processTransaction(signature, slot)

	return nil
}

// isCreateInstruction checks if logs contain a create instruction
func (l *Listener) isCreateInstruction(logs []string) bool {
	for _, log := range logs {
		// Look for pump.fun create instruction patterns
		if strings.Contains(log, "Program log: Instruction: Create") ||
			strings.Contains(log, "create") ||
			strings.Contains(log, "Create") {
			return true
		}
	}
	return false
}

// processTransaction processes a transaction to extract token information
func (l *Listener) processTransaction(signature string, slot uint64) {
	l.logger.WithField("signature", signature).Debug("Processing transaction")

	// Add delay to ensure transaction is available
	time.Sleep(2 * time.Second)

	ctx, cancel := context.WithTimeout(l.ctx, 30*time.Second)
	defer cancel()

	// Get transaction details
	tx, err := l.rpcClient.GetTransaction(ctx, signature)
	if err != nil {
		l.logger.WithError(err).WithField("signature", signature).Error("Failed to get transaction")
		return
	}

	// Extract token information from transaction
	tokenEvent, err := l.extractTokenEvent(tx, signature, slot)
	if err != nil {
		l.logger.WithError(err).WithField("signature", signature).Error("Failed to extract token event")
		return
	}

	if tokenEvent == nil {
		return // Not a token creation transaction
	}

	l.logger.LogTokenDiscovered(tokenEvent.Mint, tokenEvent.Creator, tokenEvent.Name, tokenEvent.Symbol)

	// Send token event to channel
	select {
	case l.tokenChan <- tokenEvent:
		l.logger.WithFields(logrus.Fields{
			"mint":    tokenEvent.Mint,
			"creator": tokenEvent.Creator,
			"name":    tokenEvent.Name,
			"symbol":  tokenEvent.Symbol,
		}).Info("New token discovered")
	case <-l.ctx.Done():
		return
	default:
		l.logger.Warn("Token channel full, dropping event")
	}
}

// extractTokenEvent extracts token creation information from transaction
func (l *Listener) extractTokenEvent(tx *solana.TransactionResponse, signature string, slot uint64) (*TokenEvent, error) {
	if tx == nil || tx.Transaction == nil {
		return nil, fmt.Errorf("invalid transaction data")
	}

	// Get transaction message
	message, ok := tx.Transaction["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid transaction message")
	}

	// Get instructions
	instructions, ok := message["instructions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid transaction instructions")
	}

	// Get account keys
	accountKeys, ok := message["accountKeys"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid account keys")
	}

	// Look for pump.fun create instruction
	for _, inst := range instructions {
		instruction, ok := inst.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is a pump.fun instruction
		programIdIndex, ok := instruction["programIdIndex"].(float64)
		if !ok {
			continue
		}

		if int(programIdIndex) >= len(accountKeys) {
			continue
		}

		programId, ok := accountKeys[int(programIdIndex)].(string)
		if !ok {
			continue
		}

		// Check if this is the pump.fun program
		pumpFunProgramID := base58.Encode(config.PumpFunProgramID)
		if programId != pumpFunProgramID {
			continue
		}

		// This is a pump.fun instruction, extract token data
		return l.extractTokenFromInstruction(instruction, accountKeys, tx, signature, slot)
	}

	return nil, nil // No pump.fun create instruction found
}

// extractTokenFromInstruction extracts token information from pump.fun instruction
func (l *Listener) extractTokenFromInstruction(instruction map[string]interface{}, accountKeys []interface{}, tx *solana.TransactionResponse, signature string, slot uint64) (*TokenEvent, error) {
	// Get instruction data
	dataStr, ok := instruction["data"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid instruction data")
	}

	// Decode base58 instruction data
	data, err := base58.Decode(dataStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode instruction data: %w", err)
	}

	// Check if this is a create instruction (first 8 bytes should match discriminator)
	if len(data) < 8 {
		return nil, fmt.Errorf("instruction data too short")
	}

	// For now, we'll extract account addresses from the instruction accounts
	// In a full implementation, we would decode the instruction data properly

	accounts, ok := instruction["accounts"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid instruction accounts")
	}

	if len(accounts) < 3 {
		return nil, fmt.Errorf("insufficient accounts for create instruction")
	}

	// Extract key account addresses
	var mint, creator, bondingCurve string

	// Mint is typically the first account
	if mintIndex, ok := accounts[0].(float64); ok && int(mintIndex) < len(accountKeys) {
		if mintAddr, ok := accountKeys[int(mintIndex)].(string); ok {
			mint = mintAddr
		}
	}

	// Creator/payer is typically one of the early accounts
	if creatorIndex, ok := accounts[1].(float64); ok && int(creatorIndex) < len(accountKeys) {
		if creatorAddr, ok := accountKeys[int(creatorIndex)].(string); ok {
			creator = creatorAddr
		}
	}

	// Look for bonding curve in the accounts
	for i, acc := range accounts {
		if accIndex, ok := acc.(float64); ok && int(accIndex) < len(accountKeys) {
			if addr, ok := accountKeys[int(accIndex)].(string); ok {
				// Simple heuristic: bonding curve addresses are usually longer/different pattern
				// In production, we'd have proper account identification
				if i >= 2 && bondingCurve == "" {
					bondingCurve = addr
					break
				}
			}
		}
	}

	if mint == "" || creator == "" {
		return nil, fmt.Errorf("could not extract required addresses")
	}

	// Create token event with basic information
	tokenEvent := &TokenEvent{
		Signature:    signature,
		Slot:         slot,
		Mint:         mint,
		Creator:      creator,
		BondingCurve: bondingCurve,
		Timestamp:    time.Now(),
	}

	// Get block time if available
	if tx.BlockTime != nil {
		tokenEvent.BlockTime = *tx.BlockTime
	}

	// Try to get token metadata (name, symbol, URI)
	go l.enrichTokenMetadata(tokenEvent)

	return tokenEvent, nil
}

// enrichTokenMetadata tries to get additional token metadata
func (l *Listener) enrichTokenMetadata(event *TokenEvent) {
	ctx, cancel := context.WithTimeout(l.ctx, 10*time.Second)
	defer cancel()

	// Try to get mint account info to extract metadata
	accountInfo, err := l.rpcClient.GetAccountInfo(ctx, event.Mint)
	if err != nil {
		l.logger.WithError(err).WithField("mint", event.Mint).Debug("Failed to get mint account info")
		return
	}

	if accountInfo == nil || len(accountInfo.Data) == 0 {
		return
	}

	// Decode mint account data
	// For now, we'll use placeholder values
	// In production, we'd properly decode the mint account and metadata
	event.Name = "Unknown Token"
	event.Symbol = "UNK"
	event.URI = ""
	event.InitialPrice = 0.0

	l.logger.WithFields(logrus.Fields{
		"mint":   event.Mint,
		"name":   event.Name,
		"symbol": event.Symbol,
	}).Debug("Enriched token metadata")
}

// processTokenEvents processes token events from the channel
func (l *Listener) processTokenEvents() {
	l.logger.Info("Started token event processor")
	defer l.logger.Info("Token event processor stopped")

	for {
		select {
		case <-l.ctx.Done():
			return
		case event, ok := <-l.tokenChan:
			if !ok {
				return
			}

			l.logger.WithFields(logrus.Fields{
				"mint":      event.Mint,
				"creator":   event.Creator,
				"signature": event.Signature,
			}).Info("Processing token event")

			// Here we would pass the event to the trading logic
			// For now, just log it
			l.logTokenEvent(event)
		}
	}
}

// logTokenEvent logs a token event
func (l *Listener) logTokenEvent(event *TokenEvent) {
	l.logger.WithFields(logrus.Fields{
		"event":         "new_token",
		"mint":          event.Mint,
		"creator":       event.Creator,
		"name":          event.Name,
		"symbol":        event.Symbol,
		"signature":     event.Signature,
		"slot":          event.Slot,
		"block_time":    event.BlockTime,
		"bonding_curve": event.BondingCurve,
		"timestamp":     event.Timestamp,
	}).Info("New pump.fun token detected")
}

// Helper function to convert hex string to bytes
func hexToBytes(hexStr string) ([]byte, error) {
	// Remove 0x prefix if present
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = hexStr[2:]
	}
	return hex.DecodeString(hexStr)
}

// Helper function to convert base64 string to bytes
func base64ToBytes(b64Str string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64Str)
}
