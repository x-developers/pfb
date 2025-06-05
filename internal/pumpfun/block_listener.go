package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/pkg/utils"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

// BlockListener listens for pump.fun tokens via block subscription
type BlockListener struct {
	wsClient      *client.WSClient
	rpcClient     *client.Client
	config        *ListenerConfig
	tokenChan     chan *TokenEvent
	ctx           context.Context
	cancel        context.CancelFunc
	pdaDerivation *utils.PumpFunPDADerivation
	running       bool
	startTime     time.Time

	// Statistics
	stats               *TokenEventStats
	blocksProcessed     int64
	transactionsScanned int64
	pumpFunTransactions int64

	// Configuration
	pumpFunProgramID solana.PublicKey
}

// Ensure BlockListener implements ListenerInterface
var _ ListenerInterface = (*BlockListener)(nil)

// NewBlockListener creates a new block-based listener
func NewBlockListener(
	wsClient *client.WSClient,
	rpcClient *client.Client,
	cfg *config.Config,
	logger *logger.Logger,
) *BlockListener {
	ctx, cancel := context.WithCancel(context.Background())

	conf := &ListenerConfig{
		Config:      cfg,
		Logger:      logger,
		Context:     ctx,
		BufferSize:  100,
		NetworkType: cfg.Network,
	}

	pumpFunProgramID := solana.PublicKeyFromBytes(config.PumpFunProgramID)

	return &BlockListener{
		wsClient:         wsClient,
		rpcClient:        rpcClient,
		config:           conf,
		tokenChan:        make(chan *TokenEvent, conf.BufferSize),
		ctx:              ctx,
		cancel:           cancel,
		pdaDerivation:    utils.NewPumpFunPDADerivation(),
		running:          false,
		stats:            NewTokenEventStats(),
		startTime:        time.Now(),
		pumpFunProgramID: pumpFunProgramID,
	}
}

// Start starts the block listener
func (bl *BlockListener) Start() error {
	bl.config.Logger.Info("🚀 Starting BlockListener...")

	// Connect WebSocket
	if err := bl.wsClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}
	_, err := bl.wsClient.SubscribeToBlocks(bl.pumpFunProgramID, bl.handleBlockNotification)
	if err != nil {
		return fmt.Errorf("failed to subscribe to blocks: %w", err)
	}

	bl.running = true
	bl.startTime = time.Now()

	bl.config.Logger.WithFields(logrus.Fields{
		"program_id":    bl.pumpFunProgramID.String(),
		"buffer_size":   bl.config.BufferSize,
		"network":       bl.config.NetworkType,
		"listener_type": "blocks",
	}).Info("✅ BlockListener started successfully")

	return nil
}

// Stop stops the block listener
func (bl *BlockListener) Stop() error {
	bl.config.Logger.Info("🛑 Stopping BlockListener...")

	bl.running = false
	bl.cancel()
	close(bl.tokenChan)

	if err := bl.wsClient.Disconnect(); err != nil {
		bl.config.Logger.WithError(err).Error("Error disconnecting WebSocket")
		return err
	}

	bl.config.Logger.Info("✅ BlockListener stopped")
	return nil
}

// GetTokenChannel returns the token event channel
func (bl *BlockListener) GetTokenChannel() <-chan *TokenEvent {
	return bl.tokenChan
}

// GetStats returns listener statistics
func (bl *BlockListener) GetStats() map[string]interface{} {
	baseStats := bl.stats.GetStats()
	baseStats["listener_type"] = "blocks"
	baseStats["is_running"] = bl.running
	baseStats["uptime"] = time.Since(bl.startTime).String()
	baseStats["network"] = bl.config.NetworkType
	baseStats["blocks_processed"] = bl.blocksProcessed
	baseStats["transactions_scanned"] = bl.transactionsScanned
	baseStats["pumpfun_transactions"] = bl.pumpFunTransactions
	baseStats["websocket_stats"] = bl.wsClient.GetConnectionStats()

	if bl.blocksProcessed > 0 {
		baseStats["avg_transactions_per_block"] = float64(bl.transactionsScanned) / float64(bl.blocksProcessed)
		baseStats["pumpfun_tx_percentage"] = float64(bl.pumpFunTransactions) / float64(bl.transactionsScanned) * 100
	}

	return baseStats
}

// GetListenerType returns the listener type
func (bl *BlockListener) GetListenerType() string {
	return "blocks"
}

// IsRunning returns whether the listener is running
func (bl *BlockListener) IsRunning() bool {
	return bl.running
}

// handleBlockNotification handles block notifications from WebSocket
func (bl *BlockListener) handleBlockNotification(data interface{}) error {
	processingStart := time.Now()

	notification, ok := data.(client.BlockNotification)
	if !ok {
		bl.config.Logger.Error("❌ Invalid block notification format")
		return fmt.Errorf("invalid block notification format")
	}

	slot := notification.Result.Value.Slot
	bl.blocksProcessed++

	bl.config.Logger.WithFields(logrus.Fields{
		"slot":      slot,
		"timestamp": processingStart.Format("15:04:05.000"),
	}).Debug("🔍 Processing block notification")

	// Extract block data
	//blockData, ok := notification.Result.Value.Block.(map[string]interface{})
	//if !ok {
	//	bl.config.Logger.WithField("slot", slot).Debug("⚠️ Invalid block data format")
	//	return nil
	//}
	//
	//// Process transactions in the block
	//err := bl.processBlockTransactions(blockData, slot, processingStart)
	//if err != nil {
	//	bl.config.Logger.WithError(err).WithField("slot", slot).Error("❌ Failed to process block transactions")
	//	return err
	//}

	processingTime := time.Since(processingStart)
	bl.config.Logger.WithFields(logrus.Fields{
		"slot":          slot,
		"processing_ms": processingTime.Milliseconds(),
		"blocks_total":  bl.blocksProcessed,
	}).Debug("✅ Block processed")

	return nil
}

// processBlockTransactions processes all transactions in a block
func (bl *BlockListener) processBlockTransactions(blockData map[string]interface{}, slot uint64, discoveryTime time.Time) error {
	transactions, ok := blockData["transactions"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid transactions format in block")
	}

	bl.config.Logger.WithFields(logrus.Fields{
		"slot":     slot,
		"tx_count": len(transactions),
	}).Debug("📦 Processing block transactions")

	for i, txInterface := range transactions {
		bl.transactionsScanned++

		tx, ok := txInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Get transaction data
		txData, ok := tx["transaction"].(map[string]interface{})
		if !ok {
			continue
		}

		// Get message
		message, ok := txData["message"].(map[string]interface{})
		if !ok {
			continue
		}

		// Get instructions
		instructions, ok := message["instructions"].([]interface{})
		if !ok {
			continue
		}

		// Get transaction signature
		signatures, ok := txData["signatures"].([]interface{})
		if !ok || len(signatures) == 0 {
			continue
		}

		signature, ok := signatures[0].(string)
		if !ok {
			continue
		}

		// Check if this transaction involves pump.fun
		if bl.containsPumpFunInstruction(instructions, message) {
			bl.pumpFunTransactions++

			bl.config.Logger.WithFields(logrus.Fields{
				"slot":      slot,
				"signature": signature,
				"tx_index":  i,
			}).Debug("🎯 Found pump.fun transaction in block")

			// Process pump.fun instructions
			err := bl.processPumpFunTransaction(tx, signature, slot, discoveryTime)
			if err != nil {
				bl.config.Logger.WithError(err).WithFields(logrus.Fields{
					"signature": signature,
					"slot":      slot,
				}).Debug("⚠️ Failed to process pump.fun transaction")
			}
		}
	}

	return nil
}

// containsPumpFunInstruction checks if the transaction contains pump.fun instructions
func (bl *BlockListener) containsPumpFunInstruction(instructions []interface{}, message map[string]interface{}) bool {
	// Get account keys
	accountKeys, ok := message["accountKeys"].([]interface{})
	if !ok {
		return false
	}

	// Convert account keys to strings for easier lookup
	accounts := make([]string, len(accountKeys))
	for i, key := range accountKeys {
		if keyStr, ok := key.(string); ok {
			accounts[i] = keyStr
		}
	}

	// Check each instruction
	for _, instrInterface := range instructions {
		instr, ok := instrInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Get program ID index
		programIdxInterface, ok := instr["programIdIndex"]
		if !ok {
			continue
		}

		var programIdx int
		switch v := programIdxInterface.(type) {
		case float64:
			programIdx = int(v)
		case int:
			programIdx = v
		default:
			continue
		}

		// Check if program ID matches pump.fun
		if programIdx < len(accounts) {
			if accounts[programIdx] == bl.pumpFunProgramID.String() {
				return true
			}
		}
	}

	return false
}

// processPumpFunTransaction processes a pump.fun transaction
func (bl *BlockListener) processPumpFunTransaction(tx map[string]interface{}, signature string, slot uint64, discoveryTime time.Time) error {
	// Get block time if available
	var blockTime int64
	if bt, ok := tx["blockTime"]; ok {
		switch v := bt.(type) {
		case float64:
			blockTime = int64(v)
		case int64:
			blockTime = v
		}
	}

	// Extract transaction data
	txData, ok := tx["transaction"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid transaction data")
	}

	message, ok := txData["message"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid message data")
	}

	instructions, ok := message["instructions"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid instructions data")
	}

	accountKeys, ok := message["accountKeys"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid account keys")
	}

	// Convert account keys
	accounts := make([]string, len(accountKeys))
	for i, key := range accountKeys {
		if keyStr, ok := key.(string); ok {
			accounts[i] = keyStr
		}
	}

	// Process each instruction looking for create instructions
	for _, instrInterface := range instructions {
		instr, ok := instrInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is a pump.fun instruction
		programIdxInterface, ok := instr["programIdIndex"]
		if !ok {
			continue
		}

		var programIdx int
		switch v := programIdxInterface.(type) {
		case float64:
			programIdx = int(v)
		case int:
			programIdx = v
		default:
			continue
		}

		if programIdx >= len(accounts) || accounts[programIdx] != bl.pumpFunProgramID.String() {
			continue
		}

		// Get instruction data
		dataInterface, ok := instr["data"]
		if !ok {
			continue
		}

		dataStr, ok := dataInterface.(string)
		if !ok {
			continue
		}

		// Try to extract token information from instruction data
		tokenEvent, err := bl.extractTokenFromInstruction(dataStr, signature, slot, blockTime, discoveryTime, accounts, instr)
		if err != nil {
			bl.config.Logger.WithError(err).Debug("⚠️ Failed to extract token from instruction")
			continue
		}

		if tokenEvent != nil {
			// Send token event
			bl.sendTokenEvent(tokenEvent)
		}
	}

	return nil
}

// extractTokenFromInstruction extracts token information from instruction data
func (bl *BlockListener) extractTokenFromInstruction(
	dataStr string,
	signature string,
	slot uint64,
	blockTime int64,
	discoveryTime time.Time,
	accounts []string,
	instruction map[string]interface{},
) (*TokenEvent, error) {
	// Decode instruction data
	data, err := base58.Decode(dataStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode instruction data: %w", err)
	}

	if len(data) < 8 {
		return nil, fmt.Errorf("instruction data too short")
	}

	// Check discriminator
	discriminator := binary.LittleEndian.Uint64(data[:8])
	if discriminator != CREATE_DISCRIMINATOR {
		return nil, fmt.Errorf("not a create instruction")
	}

	now := time.Now()
	tokenEvent := &TokenEvent{
		Signature:    signature,
		Slot:         slot,
		BlockTime:    blockTime,
		Timestamp:    now,
		DiscoveredAt: discoveryTime,
		Source:       "blocks",
		IsConfirmed:  true, // Block events are confirmed
		Metadata:     make(map[string]interface{}),
	}

	// Store raw data
	tokenEvent.RawData = make([]byte, len(data))
	copy(tokenEvent.RawData, data)

	// Parse instruction data (similar to log listener)
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

	// Parse token data
	if tokenEvent.Name, err = readString(); err != nil {
		return nil, fmt.Errorf("failed to read name: %w", err)
	}
	if tokenEvent.Symbol, err = readString(); err != nil {
		return nil, fmt.Errorf("failed to read symbol: %w", err)
	}
	if tokenEvent.URI, err = readString(); err != nil {
		return nil, fmt.Errorf("failed to read URI: %w", err)
	}
	if tokenEvent.Mint, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read mint: %w", err)
	}
	if tokenEvent.BondingCurve, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read bonding curve: %w", err)
	}
	if tokenEvent.User, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read user: %w", err)
	}
	if tokenEvent.Creator, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read creator: %w", err)
	}

	// Derive additional addresses
	if tokenEvent.CreatorVault, _, err = bl.pdaDerivation.DeriveCreatorVault(tokenEvent.Creator); err != nil {
		return nil, fmt.Errorf("failed to derive creator vault: %w", err)
	}
	if tokenEvent.AssociatedBondingCurve, _, err = bl.pdaDerivation.DeriveAssociatedBondingCurve(tokenEvent.Mint, tokenEvent.BondingCurve); err != nil {
		return nil, fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}

	// Add block-specific metadata
	tokenEvent.SetMetadata("instruction_data", dataStr)
	tokenEvent.SetMetadata("account_keys", accounts)
	tokenEvent.SetMetadata("block_confirmed", true)
	tokenEvent.AddProcessingStep("block_parsed")

	return tokenEvent, nil
}

// sendTokenEvent sends a token event to the channel
func (bl *BlockListener) sendTokenEvent(tokenEvent *TokenEvent) {
	// Calculate processing time
	processingTime := time.Since(tokenEvent.DiscoveredAt)
	tokenEvent.ProcessingDelayMs = processingTime.Milliseconds()

	// Update statistics
	bl.stats.Update(tokenEvent)

	// Enhanced logging
	bl.config.Logger.WithFields(tokenEvent.LogFields()).Info("🎯 New token discovered via blocks")

	// Send to channel
	select {
	case bl.tokenChan <- tokenEvent:
		bl.config.Logger.WithFields(logrus.Fields{
			"mint":        tokenEvent.Mint.String(),
			"channel_lag": time.Since(tokenEvent.DiscoveredAt).Milliseconds(),
		}).Debug("✅ Token sent to processing channel")
	case <-bl.ctx.Done():
		return
	default:
		bl.config.Logger.WithFields(logrus.Fields{
			"mint":   tokenEvent.Mint.String(),
			"age_ms": time.Since(tokenEvent.DiscoveredAt).Milliseconds(),
		}).Warn("⚠️ Token channel full, dropping event")
	}
}
