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

// UltraFastTrader - максимальная скорость с минимальными проверками
type UltraFastTrader struct {
	wallet    *wallet.Wallet
	rpcClient *solana.Client
	logger    *logger.Logger
	config    *config.Config

	// Pre-computed data for speed
	precomputedInstructions  map[string]types.Instruction // Кэш инструкций
	priorityFeeInstruction   types.Instruction
	computeBudgetInstruction types.Instruction

	// Fast blockhash management
	cachedBlockhash    string
	blockhashTimestamp time.Time
	blockhashMutex     sync.RWMutex

	// Statistics
	totalTrades      int64
	successfulTrades int64
	fastestTrade     time.Duration
	averageTradeTime time.Duration

	// Ultra-fast mode settings
	skipValidation  bool
	preSignedTxPool []*PreSignedTransaction
	poolMutex       sync.Mutex

	// NEW: Enhanced statistics for timing validation
	rejectedByTiming int64 // Количество токенов отклоненных по времени
	staleTokens      int64 // Количество устаревших токенов
	startTime        time.Time
}

type PreSignedTransaction struct {
	Transaction types.Transaction
	CreatedAt   time.Time
	Blockhash   string
	IsValid     bool
}

func NewUltraFastTrader(
	wallet *wallet.Wallet,
	rpcClient *solana.Client,
	logger *logger.Logger,
	config *config.Config,
) *UltraFastTrader {
	trader := &UltraFastTrader{
		wallet:                  wallet,
		rpcClient:               rpcClient,
		logger:                  logger,
		config:                  config,
		precomputedInstructions: make(map[string]types.Instruction),
		fastestTrade:            time.Hour,                // Initialize with large value
		skipValidation:          config.Strategy.YoloMode, // Skip all validations in YOLO mode
		startTime:               time.Now(),
	}

	// Pre-compute common instructions
	trader.precomputeInstructions()

	// Start background blockhash updater
	go trader.blockhashUpdater()

	return trader
}

// precomputeInstructions - предварительное вычисление инструкций
func (uft *UltraFastTrader) precomputeInstructions() {
	// Pre-compute priority fee instruction
	uft.priorityFeeInstruction = types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      uft.createPriorityFeeData(),
	}

	// Pre-compute compute budget instruction
	uft.computeBudgetInstruction = types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      uft.createComputeBudgetData(),
	}

	uft.logger.Info("⚡ Pre-computed instructions for ultra-fast trading")
}

// ShouldBuyToken - минимальные проверки для скорости с добавлением проверки возраста токена
func (uft *UltraFastTrader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	// Log the timing analysis for ultra-fast mode
	age := tokenEvent.GetAge()
	timeSinceDiscovery := time.Since(tokenEvent.DiscoveredAt)

	uft.logger.WithFields(map[string]interface{}{
		"mint":                   tokenEvent.Mint.String(),
		"discovered_at":          tokenEvent.DiscoveredAt.Format("15:04:05.000"),
		"age_ms":                 age.Milliseconds(),
		"time_since_discovery":   timeSinceDiscovery.Milliseconds(),
		"processing_delay_ms":    tokenEvent.ProcessingDelayMs,
		"max_age_ms":             uft.config.Trading.MaxTokenAgeMs,
		"min_discovery_delay_ms": uft.config.Trading.MinDiscoveryDelayMs,
		"ultra_fast":             true,
	}).Debug("🕒 Ultra-fast timing analysis for token")

	// NEW: Check if token is too old (даже в ultra-fast режиме)
	if tokenEvent.IsStale(uft.config) {
		uft.staleTokens++
		reason := fmt.Sprintf("ULTRA-FAST: token too old: %dms (max: %dms)",
			age.Milliseconds(), uft.config.Trading.MaxTokenAgeMs)

		uft.logger.WithFields(map[string]interface{}{
			"mint":       tokenEvent.Mint.String(),
			"age_ms":     age.Milliseconds(),
			"max_ms":     uft.config.Trading.MaxTokenAgeMs,
			"ultra_fast": true,
		}).Debug("⏰ Ultra-fast token rejected - too old")

		return false, reason
	}

	//	NEW: Check minimum discovery delay (только если не skip validation)
	if !uft.skipValidation && tokenEvent.ShouldWaitForDelay(uft.config) {
		uft.rejectedByTiming++
		waitTime := uft.config.GetMinDiscoveryDelay() - timeSinceDiscovery
		reason := fmt.Sprintf("ULTRA-FAST: waiting for discovery delay: need %dms more",
			waitTime.Milliseconds())

		uft.logger.WithFields(map[string]interface{}{
			"mint":         tokenEvent.Mint.String(),
			"elapsed_ms":   timeSinceDiscovery.Milliseconds(),
			"required_ms":  uft.config.Trading.MinDiscoveryDelayMs,
			"wait_more_ms": waitTime.Milliseconds(),
			"ultra_fast":   true,
		}).Debug("⏱️ Ultra-fast token rejected - waiting for discovery delay")

		return false, reason
	}

	// Если включен skipValidation, пропускаем все остальные проверки
	if uft.skipValidation {
		uft.logger.WithFields(map[string]interface{}{
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
	uft.logger.WithFields(map[string]interface{}{
		"mint":                tokenEvent.Mint.String(),
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  timeSinceDiscovery.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"ultra_fast":          true,
	}).Debug("✅ Ultra-fast token passed all validation including timing")

	return true, "ULTRA-FAST: basic validation passed with timing checks"
}

// BuyToken - максимально быстрое выполнение покупки
func (uft *UltraFastTrader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	start := time.Now()

	// Немедленно создаем и отправляем транзакцию
	result, err := uft.executeUltraFastBuy(ctx, tokenEvent, start)

	// Обновляем статистику
	uft.updateStatistics(start, err == nil && result.Success)

	return result, err
}

// executeUltraFastBuy - основная логика быстрой покупки
func (uft *UltraFastTrader) executeUltraFastBuy(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {
	// 1. Быстрое создание инструкций (без RPC вызовов)
	instructions := uft.createFastInstructions(tokenEvent)

	// 2. Получение кэшированного blockhash
	blockhash := uft.getCachedBlockhash()
	if blockhash == "" {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("Empty cached blockhash queue. Skipping..."),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, fmt.Errorf("Empty cached blockhash queue. Skipping...")
	}

	// 3. Создание транзакции
	transaction, err := types.NewTransaction(types.NewTransactionParam{
		Signers: []types.Account{uft.wallet.GetAccount()},
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        uft.wallet.GetPublicKey(),
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

	// 4. Немедленная отправка (без подтверждения)
	signature, err := uft.wallet.SendTransaction(ctx, transaction)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to send transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	tradeTime := time.Since(startTime)
	buyAmountSOL := uft.config.Trading.BuyAmountSOL

	// Calculate delays for enhanced logging
	age := tokenEvent.GetAge()
	discoveryDelay := time.Since(tokenEvent.DiscoveredAt)
	totalDelay := time.Since(tokenEvent.Timestamp)

	uft.logger.WithFields(map[string]interface{}{
		"signature":           signature,
		"trade_time":          tradeTime.Milliseconds(),
		"mint":                tokenEvent.Mint.String(),
		"ultra_fast":          true,
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  discoveryDelay.Milliseconds(),
		"total_delay_ms":      totalDelay.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		"execution_delay_ms":  tradeTime.Milliseconds(),
	}).Info("⚡⚡ ULTRA-FAST TRADE EXECUTED")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    buyAmountSOL,
		AmountTokens: 1000000, // Упрощенное значение
		Price:        buyAmountSOL / 1000000,
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// createFastInstructions - быстрое создание инструкций
func (uft *UltraFastTrader) createFastInstructions(tokenEvent *TokenEvent) []types.Instruction {
	instructions := make([]types.Instruction, 0, 3)

	// Добавляем pre-computed инструкции
	if uft.config.Trading.PriorityFee > 0 {
		instructions = append(instructions, uft.priorityFeeInstruction)
	}

	instructions = append(instructions, uft.computeBudgetInstruction)

	// Создаем buy инструкцию (самая дорогая операция)
	buyInstruction := uft.createBuyInstructionFast(tokenEvent)
	instructions = append(instructions, buyInstruction)

	return instructions
}

// createBuyInstructionFast - быстрое создание buy инструкции
func (uft *UltraFastTrader) createBuyInstructionFast(tokenEvent *TokenEvent) types.Instruction {
	// Предполагаем что ATA уже существует или будет создан протоколом
	userATA, _ := uft.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.PumpFunProgramID),
		Accounts: []types.AccountMeta{
			{PubKey: common.PublicKeyFromBytes(config.PumpFunGlobal), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunFeeRecipient), IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.Mint, IsSigner: false, IsWritable: false},
			{PubKey: *tokenEvent.BondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: userATA, IsSigner: false, IsWritable: true},
			{PubKey: uft.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.RentProgramID), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunEventAuthority), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunProgramID), IsSigner: false, IsWritable: false},
		},
		Data: uft.createBuyInstructionDataFast(),
	}
}

// createBuyInstructionDataFast - предвычисленные данные для buy инструкции
func (uft *UltraFastTrader) createBuyInstructionDataFast() []byte {
	discriminator := []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}

	// Предвычисленные значения для скорости
	buyAmountLamports := config.ConvertSOLToLamports(uft.config.Trading.BuyAmountSOL)
	tokenAmount := uint64(1000000) // Фиксированное количество токенов
	slippageFactor := 1.0 + float64(uft.config.Trading.SlippageBP)/10000.0
	maxSolCost := uint64(float64(buyAmountLamports) * slippageFactor)

	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], tokenAmount)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	return data
}

// Вспомогательные методы для предвычисленных инструкций
func (uft *UltraFastTrader) createPriorityFeeData() []byte {
	data := make([]byte, 9)
	data[0] = config.SetComputeUnitPriceInstruction
	binary.LittleEndian.PutUint64(data[1:], uft.config.Trading.PriorityFee)
	return data
}

func (uft *UltraFastTrader) createComputeBudgetData() []byte {
	data := make([]byte, 5)
	data[0] = config.SetComputeUnitLimitInstruction
	binary.LittleEndian.PutUint32(data[1:], 200000) // Фиксированный лимит
	return data
}

// Кэширование blockhash для скорости
func (uft *UltraFastTrader) getCachedBlockhash() string {
	uft.blockhashMutex.RLock()
	defer uft.blockhashMutex.RUnlock()

	// Проверяем свежесть (blockhash действителен ~1-2 минуты)
	if time.Since(uft.blockhashTimestamp) < 30*time.Second {
		return uft.cachedBlockhash
	}

	return ""
}

func (uft *UltraFastTrader) blockhashUpdater() {
	ticker := time.NewTicker(10 * time.Second) // Обновляем каждые 10 секунд
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			blockhash, err := uft.rpcClient.GetLatestBlockhash(ctx)
			cancel()

			if err == nil {
				uft.blockhashMutex.Lock()
				uft.cachedBlockhash = blockhash
				uft.blockhashTimestamp = time.Now()
				uft.blockhashMutex.Unlock()
			}
		}
	}
}

// Статистика и мониторинг
func (uft *UltraFastTrader) updateStatistics(startTime time.Time, success bool) {
	tradeTime := time.Since(startTime)

	uft.totalTrades++
	if success {
		uft.successfulTrades++
	}

	if tradeTime < uft.fastestTrade {
		uft.fastestTrade = tradeTime
	}

	// Обновляем среднее время
	if uft.totalTrades == 1 {
		uft.averageTradeTime = tradeTime
	} else {
		// Экспоненциальное скользящее среднее
		alpha := 0.1
		uft.averageTradeTime = time.Duration(float64(uft.averageTradeTime)*(1-alpha) + float64(tradeTime)*alpha)
	}
}

// Реализация TraderInterface
func (uft *UltraFastTrader) Stop() {
	uptime := time.Since(uft.startTime)
	uft.logger.WithFields(map[string]interface{}{
		"total_trades":       uft.totalTrades,
		"successful_trades":  uft.successfulTrades,
		"fastest_trade_ms":   uft.fastestTrade.Milliseconds(),
		"average_trade_ms":   uft.averageTradeTime.Milliseconds(),
		"rejected_by_timing": uft.rejectedByTiming,
		"stale_tokens":       uft.staleTokens,
		"uptime":             uptime.String(),
		"ultra_fast":         true,
	}).Info("🛑 Ultra-fast trader stopped")
}

func (uft *UltraFastTrader) GetTradingStats() map[string]interface{} {
	successRate := float64(0)
	if uft.totalTrades > 0 {
		successRate = (float64(uft.successfulTrades) / float64(uft.totalTrades)) * 100
	}

	uptime := time.Since(uft.startTime)
	lastTradeAgo := float64(0)
	// For ultra-fast trader, we don't track lastTradeTime to keep it lightweight

	return map[string]interface{}{
		"trader_active":          true,
		"mode":                   "ultra_fast",
		"trader_type":            "ultra_fast",
		"total_trades":           uft.totalTrades,
		"successful_trades":      uft.successfulTrades,
		"success_rate":           fmt.Sprintf("%.1f%%", successRate),
		"fastest_trade_ms":       uft.fastestTrade.Milliseconds(),
		"average_trade_ms":       uft.averageTradeTime.Milliseconds(),
		"skip_validation":        uft.skipValidation,
		"cached_blockhash":       uft.getCachedBlockhash() != "",
		"uptime_seconds":         uptime.Seconds(),
		"last_trade_ago":         lastTradeAgo,
		"buy_amount_sol":         uft.config.Trading.BuyAmountSOL,
		"slippage_bp":            uft.config.Trading.SlippageBP,
		"rejected_by_timing":     uft.rejectedByTiming,
		"stale_tokens":           uft.staleTokens,
		"max_token_age_ms":       uft.config.Trading.MaxTokenAgeMs,
		"min_discovery_delay_ms": uft.config.Trading.MinDiscoveryDelayMs,
	}
}

func (uft *UltraFastTrader) GetTraderType() string {
	return "ultra_fast"
}
