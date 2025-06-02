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

// ShouldBuyToken - минимальные проверки для скорости
func (uft *UltraFastTrader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	if uft.skipValidation {
		return true, "ULTRA-FAST: skipping validation"
	}

	// Только самые базовые проверки
	if tokenEvent.Mint == nil {
		return false, "invalid mint"
	}

	return true, "basic validation passed"
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

	uft.logger.WithFields(map[string]interface{}{
		"signature":  signature,
		"trade_time": tradeTime.Milliseconds(),
		"mint":       tokenEvent.Mint.String(),
		"ultra_fast": true,
	}).Info("⚡ ULTRA-FAST TRADE EXECUTED")

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
	uft.logger.WithFields(map[string]interface{}{
		"total_trades":      uft.totalTrades,
		"successful_trades": uft.successfulTrades,
		"fastest_trade_ms":  uft.fastestTrade.Milliseconds(),
		"average_trade_ms":  uft.averageTradeTime.Milliseconds(),
	}).Info("🛑 Ultra-fast trader stopped")
}

func (uft *UltraFastTrader) GetTradingStats() map[string]interface{} {
	successRate := float64(0)
	if uft.totalTrades > 0 {
		successRate = (float64(uft.successfulTrades) / float64(uft.totalTrades)) * 100
	}

	return map[string]interface{}{
		"trader_type":       "ultra_fast",
		"total_trades":      uft.totalTrades,
		"successful_trades": uft.successfulTrades,
		"success_rate":      fmt.Sprintf("%.1f%%", successRate),
		"fastest_trade_ms":  uft.fastestTrade.Milliseconds(),
		"average_trade_ms":  uft.averageTradeTime.Milliseconds(),
		"skip_validation":   uft.skipValidation,
		"cached_blockhash":  uft.getCachedBlockhash() != "",
	}
}

func (uft *UltraFastTrader) GetTraderType() string {
	return "ultra_fast"
}
