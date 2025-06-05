// internal/pumpfun/trader.go (Updated with Auto-Sell Integration)
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"pump-fun-bot-go/internal/client"
	"sync"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/wallet"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/associated-token-account"
)

type Trader struct {
	wallet    *wallet.Wallet
	rpcClient *client.Client
	logger    *logger.Logger
	config    *config.Config

	autoSeller *AutoSeller

	// Pre-computed data for speed
	precomputedInstructions  map[string]solana.Instruction
	priorityFeeInstruction   solana.Instruction
	computeBudgetInstruction solana.Instruction

	// Fast blockhash management
	cachedBlockhash    solana.Hash
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
	rpcClient *client.Client,
	logger *logger.Logger,
	config *config.Config,
) *Trader {
	trader := &Trader{
		wallet:                  wallet,
		rpcClient:               rpcClient,
		logger:                  logger,
		config:                  config,
		precomputedInstructions: make(map[string]solana.Instruction),
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
func (t *Trader) SetJitoClient(jitoClient *client.JitoClient) {
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
	if tokenEvent.Mint.IsZero() {
		return false, "ULTRA-FAST: invalid mint"
	}

	if tokenEvent.BondingCurve.IsZero() {
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

func (t *Trader) executeUltraFastBuy(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {

	t.rpcClient.CreateATA(t.wallet.GetPublicKey(), t.wallet.GetAccount(), tokenEvent.Mint)

	//t.logger.LogTokenEventDiscovery(tokenEvent)
	instructions := t.createFastInstructionsWithTokenSupport(tokenEvent)

	blockhash := t.getCachedBlockhash()
	if blockhash.IsZero() {
		recent, _ := t.rpcClient.GetLatestBlockhash(ctx)
		blockhash = recent

		//
		//return &TradeResult{
		//	Success:   false,
		//	Error:     fmt.Sprintf("Empty cached blockhash queue. Skipping..."),
		//	TradeTime: time.Since(startTime).Milliseconds(),
		//}, fmt.Errorf("Empty cached blockhash queue. Skipping...")
	}

	transaction, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(t.wallet.GetPublicKey()),
	)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	// Sign transaction

	_, err = transaction.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if t.wallet.GetPublicKey().Equals(key) {
				account := t.wallet.GetAccount()
				return &account
			}
			return nil
		},
	)

	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to sign transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	signature, err := t.rpcClient.SendAndConfirmTransaction(ctx, transaction)
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
func (t *Trader) createFastInstructionsWithTokenSupport(tokenEvent *TokenEvent) []solana.Instruction {
	instructions := make([]solana.Instruction, 0, 4)

	// Create ATA if needed
	ataInstruction := t.createIdempotentAssociatedInstruction(tokenEvent)
	instructions = append(instructions, ataInstruction)

	buyInstruction := t.createBuyInstructionWithTokenSupport(tokenEvent)
	instructions = append(instructions, buyInstruction)

	return instructions
}

func (t *Trader) createIdempotentAssociatedInstruction(tokenEvent *TokenEvent) solana.Instruction {
	return associatedtokenaccount.NewCreateInstruction(
		t.wallet.GetPublicKey(), // payer
		t.wallet.GetPublicKey(), // wallet
		tokenEvent.Mint,         // mint
	).Build()
}

// NEW: createBuyInstructionWithTokenSupport - создание buy инструкции с поддержкой токенов
func (t *Trader) createBuyInstructionWithTokenSupport(tokenEvent *TokenEvent) solana.Instruction {
	// Get ATA address
	userATA, _ := t.wallet.GetAssociatedTokenAddress(tokenEvent.Mint)

	// Get pump.fun program constants
	pumpFunProgram := solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	pumpFunGlobal := solana.MustPublicKeyFromBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf")
	pumpFunFeeRecipient := solana.MustPublicKeyFromBase58("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM")
	pumpFunEventAuthority := solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	accounts := []*solana.AccountMeta{
		{PublicKey: pumpFunGlobal, IsWritable: false, IsSigner: false},
		{PublicKey: pumpFunFeeRecipient, IsWritable: true, IsSigner: false},
		{PublicKey: tokenEvent.Mint, IsWritable: false, IsSigner: false},
		{PublicKey: tokenEvent.BondingCurve, IsWritable: true, IsSigner: false},
		{PublicKey: tokenEvent.AssociatedBondingCurve, IsWritable: true, IsSigner: false},
		{PublicKey: userATA, IsWritable: true, IsSigner: false},
		{PublicKey: t.wallet.GetPublicKey(), IsWritable: true, IsSigner: true},
		{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: tokenEvent.CreatorVault, IsWritable: true, IsSigner: false},
		{PublicKey: pumpFunEventAuthority, IsWritable: false, IsSigner: false},
		{PublicKey: pumpFunProgram, IsWritable: false, IsSigner: false},
	}

	data := t.createBuyInstructionDataWithTokenSupport()

	return solana.NewInstruction(
		pumpFunProgram,
		accounts,
		data,
	)
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

	// Buy instruction discriminator for pump.fun
	discriminator := uint64(16927863322537952870)

	data := make([]byte, 24)
	binary.LittleEndian.PutUint64(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], tokenAmount)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	return data
}

// precomputeInstructions - предварительное вычисление инструкций
func (t *Trader) precomputeInstructions() {
	t.logger.Info("⚡ Pre-computed instructions for ultra-fast trading")
}

// Кэширование blockhash для скорости
func (t *Trader) getCachedBlockhash() solana.Hash {
	t.blockhashMutex.RLock()
	defer t.blockhashMutex.RUnlock()

	// Проверяем свежесть (blockhash действителен ~1-2 минуты)
	if time.Since(t.blockhashTimestamp) < 30*time.Second {
		return t.cachedBlockhash
	}

	return solana.Hash{}
}

func (t *Trader) blockhashUpdater() {
	ticker := time.NewTicker(10 * time.Second) // Обновляем каждые 10 секунд
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			recent, err := t.rpcClient.GetLatestBlockhash(ctx)
			cancel()

			if err == nil {
				t.blockhashMutex.Lock()
				t.cachedBlockhash = recent
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
		"cached_blockhash":  !t.getCachedBlockhash().IsZero(),
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
