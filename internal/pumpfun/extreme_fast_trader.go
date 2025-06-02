package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

// PrecomputedTransaction represents a pre-built transaction ready for execution
type PrecomputedTransaction struct {
	Transaction      types.Transaction
	CreatedAt        time.Time
	Blockhash        string
	IsValid          bool
	ComputeUnitPrice uint64
	ComputeUnitLimit uint32
	PriorityFee      uint64
}

// ExtremeFastTrader handles extreme fast mode trading with precomputed transactions
type ExtremeFastTrader struct {
	wallet      *wallet.Wallet
	rpcClient   *solana.Client
	priceCalc   *PriceCalculator
	logger      *logger.Logger
	tradeLogger *logger.TradeLogger
	config      *config.Config

	// Extreme fast mode specific
	precomputedTxPool  []*PrecomputedTransaction
	poolMutex          sync.RWMutex
	blockhashCache     string
	blockhashUpdatedAt time.Time
	blockhashMutex     sync.RWMutex

	// Fixed token amount for speed (no calculations)
	fixedTokenAmount uint64
	fixedMaxSOLCost  uint64

	// Performance tracking
	lastTradeTime    time.Time
	fastestTradeTime time.Duration
	totalTrades      int
	successfulTrades int
	startTime        time.Time

	// Background workers
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Additional metrics
	poolMisses     int
	fallbackTrades int
}

// Ensure ExtremeFastTrader implements TraderInterface
var _ TraderInterface = (*ExtremeFastTrader)(nil)

// NewExtremeFastTrader creates a new extreme fast mode trader
func NewExtremeFastTrader(
	wallet *wallet.Wallet,
	rpcClient *solana.Client,
	priceCalc *PriceCalculator,
	logger *logger.Logger,
	tradeLogger *logger.TradeLogger,
	config *config.Config,
) *ExtremeFastTrader {
	ctx, cancel := context.WithCancel(context.Background())

	trader := &ExtremeFastTrader{
		wallet:            wallet,
		rpcClient:         rpcClient,
		priceCalc:         priceCalc,
		logger:            logger,
		tradeLogger:       tradeLogger,
		config:            config,
		precomputedTxPool: make([]*PrecomputedTransaction, 0, config.ExtremeFast.MaxPrecomputedTx),
		ctx:               ctx,
		cancel:            cancel,
		fastestTradeTime:  time.Hour, // Initialize with large value
		startTime:         time.Now(),
	}

	return trader
}

// Start initializes extreme fast mode components
func (eft *ExtremeFastTrader) Start() error {
	if !eft.config.IsExtremeFastModeEnabled() {
		return fmt.Errorf("extreme fast mode is not enabled")
	}

	// Pre-calculate fixed amounts for speed
	eft.calculateFixedAmounts()

	eft.logger.WithFields(map[string]interface{}{
		"priority_fee":       eft.config.ExtremeFast.PriorityFee,
		"compute_unit_limit": eft.config.ExtremeFast.ComputeUnitLimit,
		"compute_unit_price": eft.config.ExtremeFast.ComputeUnitPrice,
		"max_precomputed":    eft.config.ExtremeFast.MaxPrecomputedTx,
		"fixed_token_amount": eft.fixedTokenAmount,
		"fixed_max_sol_cost": eft.fixedMaxSOLCost,
	}).Info("🚀 Starting Extreme Fast Mode Trader")

	// Initialize blockhash cache
	if err := eft.refreshBlockhash(); err != nil {
		return fmt.Errorf("failed to initialize blockhash: %w", err)
	}

	// Start background workers
	eft.wg.Add(2)
	go eft.blockhashRefreshWorker()
	go eft.precomputedTxManager()

	// Wait a moment for initial pool to be created
	time.Sleep(100 * time.Millisecond)

	eft.logger.Info("⚡ Extreme Fast Mode activated - ready for lightning-fast trades!")
	return nil
}

// calculateFixedAmounts pre-calculates fixed amounts for maximum speed
func (eft *ExtremeFastTrader) calculateFixedAmounts() {
	// Fixed token amount based on configuration (e.g., 1M tokens)
	eft.fixedTokenAmount = uint64(eft.config.Trading.BuyAmountSOL * 1000000 * 1e6) // Assuming 6 decimals

	// Calculate max SOL cost with slippage
	buyAmountLamports := config.ConvertSOLToLamports(eft.config.Trading.BuyAmountSOL)
	slippageFactor := 1.0 + float64(eft.config.ExtremeFast.MaxSlippageBP)/10000.0
	eft.fixedMaxSOLCost = uint64(float64(buyAmountLamports) * slippageFactor)

	eft.logger.WithFields(map[string]interface{}{
		"fixed_token_amount": eft.fixedTokenAmount,
		"fixed_max_sol_cost": eft.fixedMaxSOLCost,
		"buy_amount_sol":     eft.config.Trading.BuyAmountSOL,
	}).Info("💎 Fixed amounts calculated for maximum speed")
}

// ShouldBuyToken determines if we should buy a token (ULTRA-FAST VERSION)
func (eft *ExtremeFastTrader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	// Skip all checks for maximum speed - just verify pool availability
	eft.poolMutex.RLock()
	poolSize := len(eft.precomputedTxPool)
	eft.poolMutex.RUnlock()

	if poolSize == 0 {
		eft.poolMisses++
		return false, "no precomputed transactions available"
	}

	// Skip balance check, bonding curve check, and other validations for speed
	return true, fmt.Sprintf("ULTRA-FAST MODE - pool: %d", poolSize)
}

// BuyToken executes an extremely fast buy operation (OPTIMIZED VERSION)
func (eft *ExtremeFastTrader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	startTime := time.Now()

	// Get precomputed transaction immediately
	precomputedTx := eft.getPrecomputedTransaction()
	if precomputedTx == nil {
		eft.poolMisses++
		return eft.fallbackBuy(ctx, tokenEvent, startTime)
	}

	// Use pre-calculated fixed amounts for speed
	customizedTx, err := eft.createOptimizedTransaction(precomputedTx, tokenEvent)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     err.Error(),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	// Send transaction with minimal retries for speed
	signature, err := eft.sendTransactionUltraFast(ctx, customizedTx)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     err.Error(),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	tradeTime := time.Since(startTime)

	// Update fastest trade time
	if tradeTime < eft.fastestTradeTime {
		eft.fastestTradeTime = tradeTime
	}

	buyAmountSOL := eft.config.Trading.BuyAmountSOL
	tokensReceived := eft.fixedTokenAmount
	price := float64(eft.fixedMaxSOLCost) / float64(tokensReceived)

	// Update statistics
	eft.totalTrades++
	eft.successfulTrades++
	eft.lastTradeTime = time.Now()

	// Log successful extreme fast trade
	eft.logger.WithFields(map[string]interface{}{
		"signature":   signature,
		"trade_time":  tradeTime.Milliseconds(),
		"mint":        tokenEvent.Mint.String(),
		"amount_sol":  buyAmountSOL,
		"tokens":      tokensReceived,
		"price":       price,
		"trader_type": "extreme_fast",
	}).Info("⚡ ULTRA-FAST TRADE SUCCESS")

	// Async logging to avoid blocking
	go eft.logTradeAsync(tokenEvent, buyAmountSOL, tokensReceived, price, signature, tradeTime)

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    buyAmountSOL,
		AmountTokens: tokensReceived,
		Price:        price,
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// createOptimizedTransaction creates transaction with pre-calculated values
func (eft *ExtremeFastTrader) createOptimizedTransaction(precomputed *PrecomputedTransaction, tokenEvent *TokenEvent) (types.Transaction, error) {
	// Build instructions efficiently
	instructions := make([]types.Instruction, 0, 4)

	// Add priority fee instruction
	instructions = append(instructions, eft.createPriorityFeeInstruction())

	// Add compute budget instruction
	instructions = append(instructions, eft.createComputeBudgetInstruction())

	// Create optimized buy instruction with fixed amounts
	buyInstruction, err := eft.createOptimizedBuyInstruction(tokenEvent)
	if err != nil {
		return types.Transaction{}, err
	}
	instructions = append(instructions, buyInstruction)

	// Create transaction
	transaction, err := types.NewTransaction(types.NewTransactionParam{
		Signers: []types.Account{eft.wallet.GetAccount()},
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        eft.wallet.GetPublicKey(),
			RecentBlockhash: precomputed.Blockhash,
			Instructions:    instructions,
		}),
	})

	if err != nil {
		return types.Transaction{}, fmt.Errorf("failed to create optimized transaction: %w", err)
	}

	return transaction, nil
}

// createOptimizedBuyInstruction creates buy instruction with pre-calculated amounts
func (eft *ExtremeFastTrader) createOptimizedBuyInstruction(tokenEvent *TokenEvent) (types.Instruction, error) {
	// Use pre-calculated ATA (assuming it exists or will be created by protocol)
	userATA, err := eft.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)
	if err != nil {
		return types.Instruction{}, err
	}

	// Build instruction with pre-calculated amounts
	instruction := types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.PumpFunProgramID),
		Accounts: []types.AccountMeta{
			{PubKey: common.PublicKeyFromBytes(config.PumpFunGlobal), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunFeeRecipient), IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.Mint, IsSigner: false, IsWritable: false},
			{PubKey: *tokenEvent.BondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: userATA, IsSigner: false, IsWritable: true},
			{PubKey: eft.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.RentProgramID), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunEventAuthority), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunProgramID), IsSigner: false, IsWritable: false},
		},
		Data: eft.createOptimizedBuyInstructionData(),
	}

	return instruction, nil
}

// createOptimizedBuyInstructionData creates buy instruction data with fixed amounts
func (eft *ExtremeFastTrader) createOptimizedBuyInstructionData() []byte {
	// Buy discriminator
	discriminator := []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}

	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], eft.fixedTokenAmount)
	binary.LittleEndian.PutUint64(data[16:24], eft.fixedMaxSOLCost)

	return data
}

// sendTransactionUltraFast sends transaction with minimal overhead
func (eft *ExtremeFastTrader) sendTransactionUltraFast(ctx context.Context, transaction types.Transaction) (string, error) {
	// Single attempt for maximum speed - no retries
	signature, err := eft.wallet.SendTransaction(ctx, transaction)
	if err != nil {
		return "", fmt.Errorf("ultra-fast send failed: %w", err)
	}

	return signature, nil
}

// logTradeAsync logs trade information asynchronously to avoid blocking
func (eft *ExtremeFastTrader) logTradeAsync(tokenEvent *TokenEvent, amountSOL float64, tokensReceived uint64, price float64, signature string, tradeTime time.Duration) {
	err := eft.tradeLogger.LogBuy(
		tokenEvent.Mint.String(),
		tokenEvent.Name,
		tokenEvent.Symbol,
		tokenEvent.Creator.String(),
		amountSOL,
		float64(tokensReceived),
		price,
		signature,
		"success",
		"",
		uint64(tradeTime.Milliseconds()),
		eft.config.ExtremeFast.MaxSlippageBP,
		"extreme_fast",
	)
	if err != nil {
		eft.logger.WithError(err).Error("Failed to log extreme fast trade")
	}
}

// maintainPrecomputedPool maintains the pool more aggressively
func (eft *ExtremeFastTrader) maintainPrecomputedPool() {
	eft.poolMutex.Lock()
	defer eft.poolMutex.Unlock()

	// Remove expired transactions more aggressively (shorter TTL for freshness)
	validTxs := make([]*PrecomputedTransaction, 0, len(eft.precomputedTxPool))
	for _, tx := range eft.precomputedTxPool {
		if time.Since(tx.CreatedAt) < 10*time.Second && tx.IsValid { // Shorter TTL
			validTxs = append(validTxs, tx)
		}
	}
	eft.precomputedTxPool = validTxs

	// Create new transactions more aggressively
	needed := eft.config.ExtremeFast.MaxPrecomputedTx - len(eft.precomputedTxPool)
	for i := 0; i < needed; i++ {
		if tx := eft.createPrecomputedTransaction(); tx != nil {
			eft.precomputedTxPool = append(eft.precomputedTxPool, tx)
		}
	}
}

// precomputedTxManager runs more frequently for faster response
func (eft *ExtremeFastTrader) precomputedTxManager() {
	defer eft.wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond) // More frequent updates
	defer ticker.Stop()

	// Initial population
	eft.maintainPrecomputedPool()

	for {
		select {
		case <-eft.ctx.Done():
			return
		case <-ticker.C:
			eft.maintainPrecomputedPool()
		}
	}
}

// fallbackBuy executes a faster fallback when pool is empty
func (eft *ExtremeFastTrader) fallbackBuy(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {
	eft.fallbackTrades++

	// Fast fallback - create transaction immediately without precomputation
	var instructions []types.Instruction

	// Add priority fee
	instructions = append(instructions, eft.createPriorityFeeInstruction())

	// Add compute budget
	instructions = append(instructions, eft.createComputeBudgetInstruction())

	// Add buy instruction
	buyInstruction, err := eft.createOptimizedBuyInstruction(tokenEvent)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("fallback instruction failed: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}
	instructions = append(instructions, buyInstruction)

	// Create and send transaction
	transaction, err := eft.wallet.CreateTransaction(ctx, instructions)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("fallback transaction failed: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	signature, err := eft.wallet.SendTransaction(ctx, transaction)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("fallback send failed: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	tradeTime := time.Since(startTime)
	buyAmountSOL := eft.config.Trading.BuyAmountSOL

	eft.totalTrades++
	eft.successfulTrades++
	eft.lastTradeTime = time.Now()

	eft.logger.WithFields(map[string]interface{}{
		"signature":  signature,
		"trade_time": tradeTime.Milliseconds(),
		"mode":       "fallback",
	}).Info("⚡ FALLBACK TRADE SUCCESS")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    buyAmountSOL,
		AmountTokens: eft.fixedTokenAmount,
		Price:        float64(eft.fixedMaxSOLCost) / float64(eft.fixedTokenAmount),
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// GetTradingStats returns enhanced trading statistics
func (eft *ExtremeFastTrader) GetTradingStats() map[string]interface{} {
	successRate := float64(0)
	if eft.totalTrades > 0 {
		successRate = (float64(eft.successfulTrades) / float64(eft.totalTrades)) * 100
	}

	eft.poolMutex.RLock()
	poolSize := len(eft.precomputedTxPool)
	eft.poolMutex.RUnlock()

	uptime := time.Since(eft.startTime)
	lastTradeAgo := float64(0)
	if !eft.lastTradeTime.IsZero() {
		lastTradeAgo = time.Since(eft.lastTradeTime).Seconds()
	}

	return map[string]interface{}{
		"trader_active":      true,
		"mode":               "extreme_fast_optimized",
		"trader_type":        eft.GetTraderType(),
		"total_trades":       eft.totalTrades,
		"successful_trades":  eft.successfulTrades,
		"success_rate":       fmt.Sprintf("%.1f%%", successRate),
		"fastest_trade_ms":   eft.fastestTradeTime.Milliseconds(),
		"pool_size":          poolSize,
		"pool_misses":        eft.poolMisses,
		"fallback_trades":    eft.fallbackTrades,
		"last_trade_ago":     lastTradeAgo,
		"uptime_seconds":     uptime.Seconds(),
		"fixed_token_amount": eft.fixedTokenAmount,
		"fixed_max_sol_cost": eft.fixedMaxSOLCost,
		"trades_per_minute":  float64(eft.totalTrades) / uptime.Minutes(),
	}
}

// Implement remaining interface methods...
func (eft *ExtremeFastTrader) GetTraderType() string {
	return "extreme_fast_optimized"
}

func (eft *ExtremeFastTrader) Stop() {
	eft.logger.Info("🛑 Stopping Extreme Fast Mode Trader")
	eft.cancel()
	eft.wg.Wait()

	stats := eft.GetTradingStats()
	eft.logger.WithFields(stats).Info("📊 Final Extreme Fast Mode Statistics")
	eft.logger.Info("✅ Extreme Fast Mode Trader shutdown complete")
}

// Helper methods from original implementation...
func (eft *ExtremeFastTrader) createPriorityFeeInstruction() types.Instruction {
	data := make([]byte, 9)
	data[0] = config.SetComputeUnitPriceInstruction
	binary.LittleEndian.PutUint64(data[1:], eft.config.ExtremeFast.ComputeUnitPrice)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      data,
	}
}

func (eft *ExtremeFastTrader) createComputeBudgetInstruction() types.Instruction {
	data := make([]byte, 5)
	data[0] = config.SetComputeUnitLimitInstruction
	binary.LittleEndian.PutUint32(data[1:], eft.config.ExtremeFast.ComputeUnitLimit)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      data,
	}
}

func (eft *ExtremeFastTrader) refreshBlockhash() error {
	ctx, cancel := context.WithTimeout(eft.ctx, 3*time.Second) // Shorter timeout
	defer cancel()

	blockhash, err := eft.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	eft.blockhashMutex.Lock()
	eft.blockhashCache = blockhash
	eft.blockhashUpdatedAt = time.Now()
	eft.blockhashMutex.Unlock()

	return nil
}

func (eft *ExtremeFastTrader) blockhashRefreshWorker() {
	defer eft.wg.Done()

	ticker := time.NewTicker(3 * time.Second) // More frequent refresh
	defer ticker.Stop()

	for {
		select {
		case <-eft.ctx.Done():
			return
		case <-ticker.C:
			if err := eft.refreshBlockhash(); err != nil {
				eft.logger.WithError(err).Error("❌ Failed to refresh blockhash")
			}
		}
	}
}

func (eft *ExtremeFastTrader) createPrecomputedTransaction() *PrecomputedTransaction {
	eft.blockhashMutex.RLock()
	blockhash := eft.blockhashCache
	eft.blockhashMutex.RUnlock()

	if blockhash == "" {
		return nil
	}

	// Create minimal base transaction template
	var instructions []types.Instruction
	instructions = append(instructions, eft.createPriorityFeeInstruction())
	instructions = append(instructions, eft.createComputeBudgetInstruction())

	transaction, err := types.NewTransaction(types.NewTransactionParam{
		Signers: []types.Account{eft.wallet.GetAccount()},
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        eft.wallet.GetPublicKey(),
			RecentBlockhash: blockhash,
			Instructions:    instructions,
		}),
	})

	if err != nil {
		return nil
	}

	return &PrecomputedTransaction{
		Transaction:      transaction,
		CreatedAt:        time.Now(),
		Blockhash:        blockhash,
		IsValid:          true,
		ComputeUnitPrice: eft.config.ExtremeFast.ComputeUnitPrice,
		ComputeUnitLimit: eft.config.ExtremeFast.ComputeUnitLimit,
		PriorityFee:      eft.config.ExtremeFast.PriorityFee,
	}
}

func (eft *ExtremeFastTrader) getPrecomputedTransaction() *PrecomputedTransaction {
	eft.poolMutex.Lock()
	defer eft.poolMutex.Unlock()

	if len(eft.precomputedTxPool) == 0 {
		return nil
	}

	// Get the newest transaction
	tx := eft.precomputedTxPool[len(eft.precomputedTxPool)-1]
	eft.precomputedTxPool = eft.precomputedTxPool[:len(eft.precomputedTxPool)-1]
	return tx
}

func (eft *ExtremeFastTrader) ValidateTokenEvent(tokenEvent *TokenEvent) error {
	// Minimal validation for speed
	if tokenEvent.Mint == nil || tokenEvent.BondingCurve == nil {
		return fmt.Errorf("invalid token event")
	}
	return nil
}
