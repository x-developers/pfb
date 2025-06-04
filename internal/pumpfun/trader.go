// internal/pumpfun/trader.go (Updated with Auto-Sell Integration)
package pumpfun

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/blocto/solana-go-sdk/program/associated_token_account"
	"sync"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

type Trader struct {
	wallet    *wallet.Wallet
	rpcClient *solana.Client
	logger    *logger.Logger
	config    *config.Config

	autoSeller *AutoSeller

	// Pre-computed data for speed
	precomputedInstructions  map[string]types.Instruction
	priorityFeeInstruction   types.Instruction
	computeBudgetInstruction types.Instruction

	// Fast blockhash management
	cachedBlockhash    string
	blockhashTimestamp time.Time
	blockhashMutex     sync.RWMutex

	// Statistics
	totalTrades      int64
	successfulTrades int64
	autoSells        int64
	fastestTrade     time.Duration
	averageTradeTime time.Duration

	// Settings
	skipValidation bool

	// Enhanced statistics for timing validation
	rejectedByTiming int64
	staleTokens      int64
	startTime        time.Time
	lastTradeTime    time.Time
}

func NewTrader(
	wallet *wallet.Wallet,
	rpcClient *solana.Client,
	logger *logger.Logger,
	config *config.Config,
) *Trader {
	trader := &Trader{
		wallet:                  wallet,
		rpcClient:               rpcClient,
		logger:                  logger,
		config:                  config,
		precomputedInstructions: make(map[string]types.Instruction),
		fastestTrade:            time.Hour,
		skipValidation:          config.Strategy.YoloMode,
		startTime:               time.Now(),
	}

	// Initialize auto-seller
	trader.autoSeller = NewAutoSeller(
		wallet,
		rpcClient,
		nil, // Will be set by JitoTrader if used
		logger,
		nil, // Will be set when trade logger is available
		config,
	)

	// Pre-compute common instructions
	trader.precomputeInstructions()

	// Start background blockhash updater
	go trader.blockhashUpdater()

	return trader
}

// SetJitoClient sets the Jito client for auto-seller
func (t *Trader) SetJitoClient(jitoClient *solana.JitoClient) {
	if t.autoSeller != nil {
		t.autoSeller.SetJitoClient(jitoClient)
	}
}

// SetTradeLogger sets the trade logger for auto-seller
func (t *Trader) SetTradeLogger(tradeLogger *logger.TradeLogger) {
	if t.autoSeller != nil {
		t.autoSeller.SetTradeLogger(tradeLogger)
	}
}

// ShouldBuyToken - минимальные проверки для скорости с добавлением проверки возраста токена
func (t *Trader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	// Check if we're respecting the max tokens per hour limit
	if t.config.Strategy.MaxTokensPerHour > 0 {
		hourAgo := time.Now().Add(-time.Hour)
		if t.lastTradeTime.After(hourAgo) && t.successfulTrades >= t.config.Strategy.MaxTokensPerHour {
			return false, "max tokens per hour limit reached"
		}
	}

	// Log the timing analysis for ultra-fast mode
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
		"ultra_fast":             true,
	}).Debug("🕒 Ultra-fast timing analysis for token")

	// Check if token is too old
	if tokenEvent.IsStale(t.config) {
		t.staleTokens++
		reason := fmt.Sprintf("ULTRA-FAST: token too old: %dms (max: %dms)",
			age.Milliseconds(), t.config.Trading.MaxTokenAgeMs)

		t.logger.WithFields(map[string]interface{}{
			"mint":       tokenEvent.Mint.String(),
			"age_ms":     age.Milliseconds(),
			"max_ms":     t.config.Trading.MaxTokenAgeMs,
			"ultra_fast": true,
		}).Debug("⏰ Ultra-fast token rejected - too old")

		return false, reason
	}

	// Check minimum discovery delay
	if !t.skipValidation && tokenEvent.ShouldWaitForDelay(t.config) {
		t.rejectedByTiming++
		waitTime := t.config.GetMinDiscoveryDelay() - timeSinceDiscovery
		reason := fmt.Sprintf("ULTRA-FAST: waiting for discovery delay: need %dms more",
			waitTime.Milliseconds())

		t.logger.WithFields(map[string]interface{}{
			"mint":         tokenEvent.Mint.String(),
			"elapsed_ms":   timeSinceDiscovery.Milliseconds(),
			"required_ms":  t.config.Trading.MinDiscoveryDelayMs,
			"wait_more_ms": waitTime.Milliseconds(),
			"ultra_fast":   true,
		}).Debug("⏱️ Ultra-fast token rejected - waiting for discovery delay")

		return false, reason
	}

	// Если включен skipValidation, пропускаем все остальные проверки
	if t.skipValidation {
		t.logger.WithFields(map[string]interface{}{
			"mint":                tokenEvent.Mint.String(),
			"age_ms":              age.Milliseconds(),
			"discovery_delay_ms":  timeSinceDiscovery.Milliseconds(),
			"processing_delay_ms": tokenEvent.ProcessingDelayMs,
			"ultra_fast":          true,
		}).Debug("✅ Ultra-fast token passed timing validation (skip validation enabled)")

		return true, "ULTRA-FAST: skipping validation (timing checks passed)"
	}

	// Только самые базовые проверки для максимальной скорости
	if tokenEvent.Mint == nil {
		return false, "ULTRA-FAST: invalid mint"
	}

	if tokenEvent.BondingCurve == nil {
		return false, "ULTRA-FAST: invalid bonding curve"
	}

	// Log successful timing validation
	t.logger.WithFields(map[string]interface{}{
		"mint":                tokenEvent.Mint.String(),
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  timeSinceDiscovery.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"ultra_fast":          true,
	}).Debug("✅ Ultra-fast token passed all validation including timing")

	return true, "ULTRA-FAST: basic validation passed with timing checks"
}

// BuyToken - максимально быстрое выполнение покупки с возможностью автопродажи
func (t *Trader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	start := time.Now()

	// Немедленно создаем и отправляем транзакцию
	result, err := t.executeUltraFastBuy(ctx, tokenEvent, start)

	// Обновляем статистику
	t.updateStatistics(start, err == nil && result.Success)

	// Если покупка успешна и включена автопродажа, планируем продажу
	if err == nil && result.Success && t.autoSeller.IsEnabled() {
		t.scheduleAutoSell(tokenEvent, result)
	}

	return result, err
}

// scheduleAutoSell планирует автоматическую продажу после покупки
func (t *Trader) scheduleAutoSell(tokenEvent *TokenEvent, purchaseResult *TradeResult) {
	autoSellRequest := AutoSellRequest{
		TokenEvent:     tokenEvent,
		PurchaseResult: purchaseResult,
		DelayMs:        t.config.Trading.SellDelayMs,
		SellPercentage: t.config.Trading.SellPercentage,
		CloseATA:       t.config.Trading.CloseATAAfterSell,
	}

	t.logger.WithFields(map[string]interface{}{
		"mint":            tokenEvent.Mint.String(),
		"purchase_amount": purchaseResult.AmountSOL,
		"delay_ms":        autoSellRequest.DelayMs,
		"sell_percentage": autoSellRequest.SellPercentage,
		"auto_sell":       true,
		"token_based":     t.config.IsTokenBasedTrading(),
	}).Info("📅 Scheduling auto-sell operation")

	t.autoSeller.ScheduleAutoSell(autoSellRequest)
	t.autoSells++
}

// executeUltraFastBuy - основная логика быстрой покупки с поддержкой токенов
func (t *Trader) executeUltraFastBuy(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {
	instructions := t.createFastInstructionsWithTokenSupport(tokenEvent)

	blockhash := t.getCachedBlockhash()
	if blockhash == "" {
		_blockhash, _ := t.rpcClient.GetLatestBlockhash(ctx)
		blockhash = _blockhash

		//
		//return &TradeResult{
		//	Success:   false,
		//	Error:     fmt.Sprintf("Empty cached blockhash queue. Skipping..."),
		//	TradeTime: time.Since(startTime).Milliseconds(),
		//}, fmt.Errorf("Empty cached blockhash queue. Skipping...")
	}

	transaction, err := types.NewTransaction(types.NewTransactionParam{
		Signers: []types.Account{t.wallet.GetAccount()},
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        t.wallet.GetPublicKey(),
			RecentBlockhash: blockhash,
			Instructions:    instructions,
		}),
	})
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	signature, err := t.wallet.SendTransaction(ctx, transaction)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to send transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	tradeTime := time.Since(startTime)

	var buyAmountSOL float64
	var tokenAmount uint64

	if t.config.IsTokenBasedTrading() {
		tokenAmount = t.config.Trading.BuyAmountTokens
		// Примерная оценка SOL для логирования (можно улучшить)
		buyAmountSOL = float64(tokenAmount) * 0.00001 // Примерная цена
	} else {
		buyAmountSOL = t.config.Trading.BuyAmountSOL
		tokenAmount = 1000000 // Упрощенное значение
	}

	// Calculate delays for enhanced logging
	age := tokenEvent.GetAge()
	discoveryDelay := time.Since(tokenEvent.DiscoveredAt)
	totalDelay := time.Since(tokenEvent.Timestamp)

	t.logger.WithFields(map[string]interface{}{
		"signature":           signature,
		"trade_time":          tradeTime.Milliseconds(),
		"mint":                tokenEvent.Mint.String(),
		"ultra_fast":          true,
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  discoveryDelay.Milliseconds(),
		"total_delay_ms":      totalDelay.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"execution_delay_ms":  tradeTime.Milliseconds(),
		"auto_sell_enabled":   t.autoSeller.IsEnabled(),
		"auto_sell_delay_ms":  t.config.Trading.SellDelayMs,
		"token_based":         t.config.IsTokenBasedTrading(),
		"buy_amount_sol":      buyAmountSOL,
		"buy_amount_tokens":   tokenAmount,
	}).Info("⚡⚡ ULTRA-FAST TRADE EXECUTED")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    buyAmountSOL,
		AmountTokens: tokenAmount,
		Price:        buyAmountSOL / float64(tokenAmount),
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// NEW: createFastInstructionsWithTokenSupport - быстрое создание инструкций с поддержкой токенов
func (t *Trader) createFastInstructionsWithTokenSupport(tokenEvent *TokenEvent) []types.Instruction {
	instructions := make([]types.Instruction, 0, 4)

	// Добавляем pre-computed инструкции
	//if t.config.Trading.PriorityFee > 0 {
	//	instructions = append(instructions, t.priorityFeeInstruction)
	//}
	//instructions = append(instructions, t.computeBudgetInstruction)

	//params := &associated_token_account.CreateIdempotentParam{
	//	Funder: t.wallet.GetPublicKey(),
	//	Owner:  t.wallet.GetPublicKey(),
	//	Mint:   *tokenEvent.Mint,
	//	//AssociatedTokenAccount: common.PublicKey{},
	//}
	ataInstruction := t.createIdempotentAssociatedInstruction(tokenEvent)
	instructions = append(instructions, ataInstruction)

	buyInstruction := t.createBuyInstructionWithTokenSupport(tokenEvent)
	instructions = append(instructions, buyInstruction)

	return instructions
}

func (t *Trader) createIdempotentAssociatedInstruction(tokenEvent *TokenEvent) types.Instruction {
	userATA, _ := t.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)
	params := &associated_token_account.CreateIdempotentParam{
		Funder:                 t.wallet.GetPublicKey(),
		Owner:                  t.wallet.GetPublicKey(),
		Mint:                   *tokenEvent.Mint,
		AssociatedTokenAccount: userATA,
	}

	return associated_token_account.CreateIdempotent(*params)
}

// NEW: createBuyInstructionWithTokenSupport - создание buy инструкции с поддержкой токенов
func (t *Trader) createBuyInstructionWithTokenSupport(tokenEvent *TokenEvent) types.Instruction {
	// Предполагаем что ATA уже существует или будет создан протоколом
	userATA, _ := t.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)

	return types.Instruction{
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
			{PubKey: *tokenEvent.CreatorVault, IsSigner: false, IsWritable: true},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunEventAuthority), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunProgramID), IsSigner: false, IsWritable: false},
		},
		Data: t.createBuyInstructionDataWithTokenSupport(),
	}
}

func (t *Trader) createBuyInstructionDataWithTokenSupport() []byte {
	var tokenAmount uint64
	var maxSolCost uint64

	if t.config.IsTokenBasedTrading() {
		// Используем токены напрямую
		tokenAmount = t.config.Trading.BuyAmountTokens
		// Устанавливаем большое максимальное значение SOL для безопасности
		maxSolCost = config.ConvertSOLToLamports(t.config.Trading.BuyAmountSOL * 2) // 2x от обычного SOL amount как safety margin
	} else {
		// Традиционный метод через SOL
		buyAmountLamports := config.ConvertSOLToLamports(t.config.Trading.BuyAmountSOL)
		tokenAmount = 1000000 // Фиксированное количество токенов
		slippageFactor := 1.0 + float64(t.config.Trading.SlippageBP)/10000.0
		maxSolCost = uint64(float64(buyAmountLamports) * slippageFactor)
	}

	// аналог struct.pack("<Q", 16927863322537952870)
	var expectedDiscriminator bytes.Buffer
	binary.Write(&expectedDiscriminator, binary.LittleEndian, uint64(16927863322537952870))

	// аналог struct.pack("<Q", token_amount_raw)
	var tokenAmountBytes bytes.Buffer
	binary.Write(&tokenAmountBytes, binary.LittleEndian, tokenAmount)

	// аналог struct.pack("<Q", max_amount_lamports)
	var maxAmountBytes bytes.Buffer
	binary.Write(&maxAmountBytes, binary.LittleEndian, maxSolCost)

	// аналог EXPECTED_DISCRIMINATOR + token_amount_bytes + max_amount_bytes
	data := append(expectedDiscriminator.Bytes(), tokenAmountBytes.Bytes()...)
	data = append(data, maxAmountBytes.Bytes()...)

	return data
}

// precomputeInstructions - предварительное вычисление инструкций
func (t *Trader) precomputeInstructions() {
	// Pre-compute priority fee instruction
	t.priorityFeeInstruction = types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      t.createPriorityFeeData(),
	}

	// Pre-compute compute budget instruction
	t.computeBudgetInstruction = types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      t.createComputeBudgetData(),
	}

	t.logger.Info("⚡ Pre-computed instructions for ultra-fast trading")
}

// createFastInstructions - быстрое создание инструкций
func (t *Trader) createFastInstructions(tokenEvent *TokenEvent) []types.Instruction {
	instructions := make([]types.Instruction, 0, 3)

	// Добавляем pre-computed инструкции
	if t.config.Trading.PriorityFee > 0 {
		instructions = append(instructions, t.priorityFeeInstruction)
	}

	instructions = append(instructions, t.computeBudgetInstruction)

	// Создаем buy инструкцию (самая дорогая операция)
	buyInstruction := t.createBuyInstructionFast(tokenEvent)
	instructions = append(instructions, buyInstruction)

	return instructions
}

// createBuyInstructionFast - быстрое создание buy инструкции
func (t *Trader) createBuyInstructionFast(tokenEvent *TokenEvent) types.Instruction {
	// Предполагаем что ATA уже существует или будет создан протоколом
	userATA, _ := t.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)

	return types.Instruction{
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
		Data: t.createBuyInstructionDataFast(),
	}
}

// createBuyInstructionDataFast - предвычисленные данные для buy инструкции
func (t *Trader) createBuyInstructionDataFast() []byte {
	discriminator := []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}

	// Предвычисленные значения для скорости
	buyAmountLamports := config.ConvertSOLToLamports(t.config.Trading.BuyAmountSOL)
	tokenAmount := uint64(1000000) // Фиксированное количество токенов
	slippageFactor := 1.0 + float64(t.config.Trading.SlippageBP)/10000.0
	maxSolCost := uint64(float64(buyAmountLamports) * slippageFactor)

	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], tokenAmount)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	return data
}

// Вспомогательные методы для предвычисленных инструкций
func (t *Trader) createPriorityFeeData() []byte {
	data := make([]byte, 9)
	data[0] = config.SetComputeUnitPriceInstruction
	binary.LittleEndian.PutUint64(data[1:], t.config.Trading.PriorityFee)
	return data
}

func (t *Trader) createComputeBudgetData() []byte {
	data := make([]byte, 5)
	data[0] = config.SetComputeUnitLimitInstruction
	binary.LittleEndian.PutUint32(data[1:], 200000) // Фиксированный лимит
	return data
}

// Кэширование blockhash для скорости
func (t *Trader) getCachedBlockhash() string {
	t.blockhashMutex.RLock()
	defer t.blockhashMutex.RUnlock()

	// Проверяем свежесть (blockhash действителен ~1-2 минуты)
	if time.Since(t.blockhashTimestamp) < 30*time.Second {
		return t.cachedBlockhash
	}

	return ""
}

func (t *Trader) blockhashUpdater() {
	ticker := time.NewTicker(10 * time.Second) // Обновляем каждые 10 секунд
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			blockhash, err := t.rpcClient.GetLatestBlockhash(ctx)
			cancel()

			if err == nil {
				t.blockhashMutex.Lock()
				t.cachedBlockhash = blockhash
				t.blockhashTimestamp = time.Now()
				t.blockhashMutex.Unlock()
			}
		}
	}
}

// Статистика и мониторинг
func (t *Trader) updateStatistics(startTime time.Time, success bool) {
	tradeTime := time.Since(startTime)
	t.lastTradeTime = time.Now()

	t.totalTrades++
	if success {
		t.successfulTrades++
	}

	if tradeTime < t.fastestTrade {
		t.fastestTrade = tradeTime
	}

	// Обновляем среднее время
	if t.totalTrades == 1 {
		t.averageTradeTime = tradeTime
	} else {
		// Экспоненциальное скользящее среднее
		alpha := 0.1
		t.averageTradeTime = time.Duration(float64(t.averageTradeTime)*(1-alpha) + float64(tradeTime)*alpha)
	}
}

// Реализация TraderInterface
func (t *Trader) Stop() {
	uptime := time.Since(t.startTime)
	t.logger.WithFields(map[string]interface{}{
		"total_trades":       t.totalTrades,
		"successful_trades":  t.successfulTrades,
		"auto_sells":         t.autoSells,
		"fastest_trade_ms":   t.fastestTrade.Milliseconds(),
		"average_trade_ms":   t.averageTradeTime.Milliseconds(),
		"rejected_by_timing": t.rejectedByTiming,
		"stale_tokens":       t.staleTokens,
		"uptime":             uptime.String(),
		"ultra_fast":         true,
	}).Info("🛑 Ultra-fast trader stopped")
}

func (t *Trader) GetTradingStats() map[string]interface{} {
	successRate := float64(0)
	if t.totalTrades > 0 {
		successRate = (float64(t.successfulTrades) / float64(t.totalTrades)) * 100
	}

	uptime := time.Since(t.startTime)

	stats := map[string]interface{}{
		"trader_active":     true,
		"mode":              "ultra_fast",
		"trader_type":       "ultra_fast_with_auto_sell",
		"total_trades":      t.totalTrades,
		"successful_trades": t.successfulTrades,
		"auto_sells":        t.autoSells,
		"success_rate":      fmt.Sprintf("%.1f%%", successRate),
		"fastest_trade_ms":  t.fastestTrade.Milliseconds(),
		"average_trade_ms":  t.averageTradeTime.Milliseconds(),
		"skip_validation":   t.skipValidation,
		"cached_blockhash":  t.getCachedBlockhash() != "",
		"uptime_seconds":    uptime.Seconds(),

		// UPDATED: New trading method info
		"token_based_trading": t.config.IsTokenBasedTrading(),
		"buy_amount_sol":      t.config.Trading.BuyAmountSOL,
		"buy_amount_tokens":   t.config.Trading.BuyAmountTokens,
		"use_token_amount":    t.config.Trading.UseTokenAmount,

		"slippage_bp":            t.config.Trading.SlippageBP,
		"rejected_by_timing":     t.rejectedByTiming,
		"stale_tokens":           t.staleTokens,
		"max_token_age_ms":       t.config.Trading.MaxTokenAgeMs,
		"min_discovery_delay_ms": t.config.Trading.MinDiscoveryDelayMs,

		// UPDATED: Auto-sell with milliseconds
		"auto_sell_delay_ms": t.config.Trading.SellDelayMs,
	}

	// Add auto-sell stats
	if t.autoSeller != nil {
		autoSellStats := t.autoSeller.GetStats()
		for k, v := range autoSellStats {
			stats["auto_sell_"+k] = v
		}
	}

	return stats
}

func (t *Trader) GetTraderType() string {
	return "ultra_fast_with_auto_sell"
}

// GetAutoSeller returns the auto-seller instance (for external configuration)
func (t *Trader) GetAutoSeller() *AutoSeller {
	return t.autoSeller
}

// CreateBuyTransaction implements TransactionCapableTrader interface for Jito integration
func (t *Trader) CreateBuyTransaction(ctx context.Context, tokenEvent *TokenEvent) (types.Transaction, error) {
	// Create fast instructions
	instructions := t.createFastInstructions(tokenEvent)

	// Get recent blockhash
	blockhash, err := t.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		// Fallback to cached blockhash
		blockhash = t.getCachedBlockhash()
		if blockhash == "" {
			return types.Transaction{}, fmt.Errorf("failed to get blockhash: %w", err)
		}
	}

	// Create transaction
	transaction, err := types.NewTransaction(types.NewTransactionParam{
		Signers: []types.Account{t.wallet.GetAccount()},
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        t.wallet.GetPublicKey(),
			RecentBlockhash: blockhash,
			Instructions:    instructions,
		}),
	})

	if err != nil {
		return types.Transaction{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	return transaction, nil
}
