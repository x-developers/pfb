package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"
	"pump-fun-bot-go/pkg/utils"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

// Trader handles pump.fun trading operations
type Trader struct {
	wallet      *wallet.Wallet
	rpcClient   *solana.Client
	priceCalc   *PriceCalculator
	logger      *logger.Logger
	tradeLogger *logger.TradeLogger
	config      *config.Config

	// Trading statistics
	totalTrades      int
	successfulTrades int
	lastTradeTime    time.Time
	startTime        time.Time
	rejectedByTiming int // NEW: количество токенов отклоненных по времени
	staleTokens      int // NEW: количество устаревших токенов
}

// Ensure Trader implements TraderInterface
var _ TraderInterface = (*Trader)(nil)

// NewTrader creates a new trader instance
func NewTrader(
	wallet *wallet.Wallet,
	rpcClient *solana.Client,
	priceCalc *PriceCalculator,
	logger *logger.Logger,
	tradeLogger *logger.TradeLogger,
	config *config.Config,
) *Trader {
	return &Trader{
		wallet:      wallet,
		rpcClient:   rpcClient,
		priceCalc:   priceCalc,
		logger:      logger,
		tradeLogger: tradeLogger,
		config:      config,
		startTime:   time.Now(),
	}
}

// ShouldBuyToken determines if we should buy a token with timing validation
func (t *Trader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	// Log the timing analysis
	age := tokenEvent.GetAge()
	timeSinceDiscovery := time.Since(tokenEvent.DiscoveredAt)

	t.logger.WithFields(map[string]interface{}{
		"mint":                   tokenEvent.Mint.String(),
		"discovered_at":          tokenEvent.DiscoveredAt.Format("15:04:05.000"),
		"age_ms":                 age.Milliseconds(),
		"time_since_discovery":   timeSinceDiscovery.Milliseconds(),
		"processing_delay_ms":    tokenEvent.ProcessingDelayMs,
		"max_age_ms":             t.config.Trading.MaxTokenAgeMs,
		"min_discovery_delay_ms": t.config.Trading.MinDiscoveryDelayMs,
	}).Debug("🕒 Timing analysis for token")

	// NEW: Check if token is too old
	if tokenEvent.IsStale(t.config) {
		t.staleTokens++
		reason := fmt.Sprintf("token too old: %dms (max: %dms)",
			age.Milliseconds(), t.config.Trading.MaxTokenAgeMs)

		t.logger.WithFields(map[string]interface{}{
			"mint":   tokenEvent.Mint.String(),
			"age_ms": age.Milliseconds(),
			"max_ms": t.config.Trading.MaxTokenAgeMs,
		}).Debug("⏰ Token rejected - too old")

		return false, reason
	}

	// NEW: Check if we should wait for minimum discovery delay
	//if tokenEvent.ShouldWaitForDelay(t.config) {
	//	t.rejectedByTiming++
	//	waitTime := t.config.GetMinDiscoveryDelay() - timeSinceDiscovery
	//	reason := fmt.Sprintf("waiting for discovery delay: need %dms more",
	//		waitTime.Milliseconds())
	//
	//	t.logger.WithFields(map[string]interface{}{
	//		"mint":         tokenEvent.Mint.String(),
	//		"elapsed_ms":   timeSinceDiscovery.Milliseconds(),
	//		"required_ms":  t.config.Trading.MinDiscoveryDelayMs,
	//		"wait_more_ms": waitTime.Milliseconds(),
	//	}).Debug("⏱️ Token rejected - waiting for discovery delay")
	//
	//	return false, reason
	//}

	// Check wallet balance
	balance, err := t.wallet.GetBalanceSOL(ctx)
	if err != nil {
		return false, "failed to check balance"
	}

	requiredAmount := t.config.Trading.BuyAmountSOL + 0.001 // Add buffer for fees
	if balance < requiredAmount {
		return false, fmt.Sprintf("insufficient balance: %.6f SOL (need %.6f)", balance, requiredAmount)
	}

	// Check if we're respecting the max tokens per hour limit
	if t.config.Strategy.MaxTokensPerHour > 0 {
		hourAgo := time.Now().Add(-time.Hour)
		if t.lastTradeTime.After(hourAgo) && t.totalTrades >= t.config.Strategy.MaxTokensPerHour {
			return false, "max tokens per hour limit reached"
		}
	}

	// Log successful timing validation
	t.logger.WithFields(map[string]interface{}{
		"mint":                tokenEvent.Mint.String(),
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  timeSinceDiscovery.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
	}).Debug("✅ Token passed timing validation")

	return true, "all conditions met including timing"
}

// BuyToken executes a buy transaction with enhanced timing logging
func (t *Trader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	startTime := time.Now()
	buyAmountSOL := t.config.Trading.BuyAmountSOL
	buyAmountLamports := utils.ConvertSOLToLamports(buyAmountSOL)

	// Calculate delays for enhanced logging
	age := tokenEvent.GetAge()
	discoveryDelay := time.Since(tokenEvent.DiscoveredAt)
	totalDelay := time.Since(tokenEvent.Timestamp)

	t.logger.WithFields(map[string]interface{}{
		"mint":                tokenEvent.Mint.String(),
		"name":                tokenEvent.Name,
		"symbol":              tokenEvent.Symbol,
		"amount_sol":          buyAmountSOL,
		"trader":              "normal",
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  discoveryDelay.Milliseconds(),
		"total_delay_ms":      totalDelay.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"execution_time":      startTime.Format("15:04:05.000"),
		"discovered_at":       tokenEvent.DiscoveredAt.Format("15:04:05.000"),
	}).Info("🛒 Executing buy transaction")

	// Calculate expected tokens received (simplified calculation)
	tokensReceived := uint64(1000000) // This would be calculated from actual bonding curve

	// Apply slippage protection
	slippageFactor := 1.0 + float64(t.config.Trading.SlippageBP)/10000.0
	maxSolCost := uint64(float64(buyAmountLamports) * slippageFactor)

	// Create instructions list
	var instructions []types.Instruction

	// Add priority fee instruction if configured
	if t.config.Trading.PriorityFee > 0 {
		priorityFeeInstruction := t.createPriorityFeeInstruction()
		instructions = append(instructions, priorityFeeInstruction)
	}

	// Create ATA if needed
	ataInstruction, err := t.createATAInstructionIfNeeded(ctx, *tokenEvent.Mint)
	if err != nil {
		t.totalTrades++
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create ATA instruction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}
	if ataInstruction != nil {
		instructions = append(instructions, *ataInstruction)
	}

	// Create buy instruction
	buyInstruction, err := t.createBuyInstruction(tokenEvent, tokensReceived, maxSolCost)
	if err != nil {
		t.totalTrades++
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create buy instruction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}
	instructions = append(instructions, buyInstruction)

	// Create and send transaction
	transaction, err := t.wallet.CreateTransaction(ctx, instructions)
	if err != nil {
		t.totalTrades++
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	signature, err := t.wallet.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		t.totalTrades++
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to send transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	// Update statistics
	t.totalTrades++
	t.successfulTrades++
	t.lastTradeTime = time.Now()
	tradeTime := time.Since(startTime)

	// Calculate price
	price := float64(buyAmountLamports) / float64(tokensReceived)

	// Enhanced success logging with all timing information
	t.logger.WithFields(map[string]interface{}{
		"signature":           signature,
		"trade_time":          tradeTime.Milliseconds(),
		"amount_sol":          buyAmountSOL,
		"tokens":              tokensReceived,
		"price":               price,
		"trader_type":         "normal",
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  discoveryDelay.Milliseconds(),
		"total_delay_ms":      totalDelay.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"execution_delay_ms":  tradeTime.Milliseconds(),
	}).Info("✅ Buy transaction successful")

	// Log trade to file
	err = t.tradeLogger.LogBuy(
		tokenEvent.Mint.String(),
		tokenEvent.Name,
		tokenEvent.Symbol,
		tokenEvent.Creator.String(),
		buyAmountSOL,
		float64(tokensReceived),
		price,
		signature,
		"success",
		"",
		5000, // Estimated fee in lamports
		t.config.Trading.SlippageBP,
		t.config.Strategy.Type,
	)
	if err != nil {
		t.logger.WithError(err).Error("Failed to log trade to file")
	}

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    buyAmountSOL,
		AmountTokens: tokensReceived,
		Price:        price,
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// SellToken executes a sell transaction
func (t *Trader) SellToken(ctx context.Context, mint common.PublicKey, amount uint64) (*TradeResult, error) {
	startTime := time.Now()

	t.logger.WithFields(map[string]interface{}{
		"mint":   mint.String(),
		"amount": amount,
	}).Info("🔄 Executing sell transaction")

	// For now, return not implemented
	// In full implementation, this would:
	// 1. Get token balance
	// 2. Calculate expected SOL output
	// 3. Create sell instruction
	// 4. Send transaction
	// 5. Log results

	return &TradeResult{
		Success:   false,
		Error:     "sell functionality not yet implemented",
		TradeTime: time.Since(startTime).Milliseconds(),
	}, fmt.Errorf("sell functionality not yet implemented")
}

// GetTradingStats returns enhanced trading statistics with timing info
func (t *Trader) GetTradingStats() map[string]interface{} {
	successRate := float64(0)
	if t.totalTrades > 0 {
		successRate = (float64(t.successfulTrades) / float64(t.totalTrades)) * 100
	}

	uptime := time.Since(t.startTime)
	lastTradeAgo := float64(0)
	if !t.lastTradeTime.IsZero() {
		lastTradeAgo = time.Since(t.lastTradeTime).Seconds()
	}

	return map[string]interface{}{
		"trader_active":          true,
		"mode":                   "normal",
		"trader_type":            t.GetTraderType(),
		"total_trades":           t.totalTrades,
		"successful_trades":      t.successfulTrades,
		"success_rate":           fmt.Sprintf("%.1f%%", successRate),
		"uptime_seconds":         uptime.Seconds(),
		"last_trade_ago":         lastTradeAgo,
		"buy_amount_sol":         t.config.Trading.BuyAmountSOL,
		"slippage_bp":            t.config.Trading.SlippageBP,
		"rejected_by_timing":     t.rejectedByTiming,
		"stale_tokens":           t.staleTokens,
		"max_token_age_ms":       t.config.Trading.MaxTokenAgeMs,
		"min_discovery_delay_ms": t.config.Trading.MinDiscoveryDelayMs,
	}
}

// GetTraderType returns the type of trader (implements TraderInterface)
func (t *Trader) GetTraderType() string {
	return "normal"
}

// Stop stops the trader (implements TraderInterface)
func (t *Trader) Stop() {
	uptime := time.Since(t.startTime)
	t.logger.WithFields(map[string]interface{}{
		"total_trades":       t.totalTrades,
		"successful_trades":  t.successfulTrades,
		"rejected_by_timing": t.rejectedByTiming,
		"stale_tokens":       t.staleTokens,
		"uptime":             uptime.String(),
	}).Info("🛑 Normal trader stopped")
}

// createPriorityFeeInstruction creates a priority fee instruction for faster processing
func (t *Trader) createPriorityFeeInstruction() types.Instruction {
	data := make([]byte, 9)
	data[0] = config.SetComputeUnitPriceInstruction // SetComputeUnitPrice instruction
	binary.LittleEndian.PutUint64(data[1:], t.config.Trading.PriorityFee)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      data,
	}
}

// createBuyInstruction creates a pump.fun buy instruction
func (t *Trader) createBuyInstruction(tokenEvent *TokenEvent, amount, maxSolCost uint64) (types.Instruction, error) {
	userATA, err := t.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)
	if err != nil {
		return types.Instruction{}, fmt.Errorf("failed to get user ATA: %w", err)
	}

	instruction := types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.PumpFunProgramID),
		Accounts: []types.AccountMeta{
			{PubKey: common.PublicKeyFromBytes(config.PumpFunGlobal), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunFeeRecipient), IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.Mint, IsSigner: false, IsWritable: false},
			{PubKey: *tokenEvent.BondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: userATA, IsSigner: false, IsWritable: true},
			{PubKey: t.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.RentProgramID), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunEventAuthority), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunProgramID), IsSigner: false, IsWritable: false},
		},
		Data: t.createBuyInstructionData(amount, maxSolCost),
	}

	return instruction, nil
}

// createATAInstructionIfNeeded creates Associated Token Account instruction if needed
func (t *Trader) createATAInstructionIfNeeded(ctx context.Context, mint common.PublicKey) (*types.Instruction, error) {
	ata, err := t.wallet.GetAssociatedTokenAddress(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive ATA: %w", err)
	}

	// Create ATA instruction
	instruction := types.Instruction{
		ProgramID: common.SPLAssociatedTokenAccountProgramID,
		Accounts: []types.AccountMeta{
			{PubKey: t.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},   // Payer
			{PubKey: ata, IsSigner: false, IsWritable: true},                      // Associated token account
			{PubKey: t.wallet.GetPublicKey(), IsSigner: false, IsWritable: false}, // Owner
			{PubKey: mint, IsSigner: false, IsWritable: false},                    // Mint
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},  // System program
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},   // Token program
		},
		Data: []byte{}, // ATA creation instruction has no data
	}

	t.logger.WithField("ata", ata.String()).Debug("Creating ATA instruction")
	return &instruction, nil
}

// createBuyInstructionData creates the instruction data for pump.fun buy
func (t *Trader) createBuyInstructionData(amount, maxSolCost uint64) []byte {
	// Pump.fun buy instruction discriminator
	discriminator := []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}

	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], amount)      // Token amount to buy
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost) // Max SOL to spend

	return data
}

// createSellInstructionData creates the instruction data for pump.fun sell
func (t *Trader) createSellInstructionData(amount, minSolOutput uint64) []byte {
	// Pump.fun sell instruction discriminator
	discriminator := []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}

	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], amount)        // Token amount to sell
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput) // Min SOL to receive

	return data
}

// Helper methods for trading operations

// CalculateTokensExpected calculates expected tokens from SOL amount (simplified)
func (t *Trader) CalculateTokensExpected(ctx context.Context, bondingCurve common.PublicKey, solAmount uint64) (uint64, error) {
	// In a real implementation, this would:
	// 1. Fetch bonding curve data
	// 2. Calculate based on curve formula
	// 3. Account for fees and slippage

	// For now, return a simplified calculation
	return solAmount * 1000000, nil // 1 SOL = 1M tokens (example rate)
}

// EstimateTransactionFee estimates the transaction fee
func (t *Trader) EstimateTransactionFee() uint64 {
	baseFee := uint64(5000) // Base transaction fee

	if t.config.Trading.PriorityFee > 0 {
		// Add priority fee costs
		computeUnits := uint64(200000) // Estimated compute units for buy transaction
		priorityFeeCost := (t.config.Trading.PriorityFee * computeUnits) / 1000000
		return baseFee + priorityFeeCost
	}

	return baseFee
}

// ValidateTokenEvent validates token event data before trading
func (t *Trader) ValidateTokenEvent(tokenEvent *TokenEvent) error {
	if tokenEvent.Mint == nil {
		return fmt.Errorf("token mint is nil")
	}

	if tokenEvent.BondingCurve == nil {
		return fmt.Errorf("bonding curve is nil")
	}

	if tokenEvent.AssociatedBondingCurve == nil {
		return fmt.Errorf("associated bonding curve is nil")
	}

	if tokenEvent.Creator == nil {
		return fmt.Errorf("creator is nil")
	}

	if tokenEvent.Name == "" {
		return fmt.Errorf("token name is empty")
	}

	if tokenEvent.Symbol == "" {
		return fmt.Errorf("token symbol is empty")
	}

	return nil
}

// LogTradeAttempt logs when a trade attempt is made with timing information
func (t *Trader) LogTradeAttempt(tokenEvent *TokenEvent, tradeType string) {
	age := tokenEvent.GetAge()
	discoveryDelay := time.Since(tokenEvent.DiscoveredAt)

	t.logger.WithFields(map[string]interface{}{
		"event":               "trade_attempt",
		"type":                tradeType,
		"mint":                tokenEvent.Mint.String(),
		"name":                tokenEvent.Name,
		"symbol":              tokenEvent.Symbol,
		"creator":             tokenEvent.Creator.String(),
		"trader":              "normal",
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  discoveryDelay.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"timestamp":           time.Now().Format(time.RFC3339),
	}).Info("💰 Trade attempt initiated")
}
