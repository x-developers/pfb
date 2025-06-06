package pumpfun

import (
	"context"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/wallet"
)

// Trader orchestrates token trading operations
type Trader struct {
	buyer       *Buyer
	seller      *Seller
	wallet      *wallet.Wallet
	rpcClient   *client.Client
	logger      *logger.Logger
	tradeLogger *logger.TradeLogger
	config      *config.Config

	// Statistics
	totalTrades      int64
	successfulTrades int64
	rejectedByTiming int64
	staleTokens      int64
	startTime        time.Time
	lastTradeTime    time.Time

	// Settings
	skipValidation bool
}

// NewTrader creates a new trader instance
func NewTrader(
	wallet *wallet.Wallet,
	rpcClient *client.Client,
	logger *logger.Logger,
	config *config.Config,
) *Trader {
	trader := &Trader{
		wallet:         wallet,
		rpcClient:      rpcClient,
		logger:         logger,
		config:         config,
		skipValidation: config.Strategy.YoloMode,
		startTime:      time.Now(),
	}

	// Initialize buyer
	trader.buyer = NewBuyer(wallet, rpcClient, logger, config)

	// Initialize seller
	trader.seller = NewSeller(wallet, rpcClient, logger, nil, config)

	return trader
}

// SetTradeLogger sets the trade logger for both buyer and seller
func (t *Trader) SetTradeLogger(tradeLogger *logger.TradeLogger) {
	t.tradeLogger = tradeLogger
	t.seller.SetTradeLogger(tradeLogger)
}

// ShouldBuyToken determines if we should buy a token with timing validation
func (t *Trader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	// Check trading limits
	if t.config.Strategy.MaxTokensPerHour > 0 {
		hourAgo := time.Now().Add(-time.Hour)
		if t.lastTradeTime.After(hourAgo) && t.successfulTrades >= t.config.Strategy.MaxTokensPerHour {
			return false, "max tokens per hour limit reached"
		}
	}

	// Timing analysis
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

	// Check if token is too old
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

	// Check minimum discovery delay
	if !t.skipValidation && tokenEvent.ShouldWaitForDelay(t.config) {
		t.rejectedByTiming++
		waitTime := t.config.GetMinDiscoveryDelay() - timeSinceDiscovery
		reason := fmt.Sprintf("waiting for discovery delay: need %dms more",
			waitTime.Milliseconds())

		t.logger.WithFields(map[string]interface{}{
			"mint":         tokenEvent.Mint.String(),
			"elapsed_ms":   timeSinceDiscovery.Milliseconds(),
			"required_ms":  t.config.Trading.MinDiscoveryDelayMs,
			"wait_more_ms": waitTime.Milliseconds(),
		}).Debug("⏱️ Token rejected - waiting for discovery delay")

		return false, reason
	}

	// Skip validation if enabled
	if t.skipValidation {
		t.logger.WithFields(map[string]interface{}{
			"mint":                tokenEvent.Mint.String(),
			"age_ms":              age.Milliseconds(),
			"discovery_delay_ms":  timeSinceDiscovery.Milliseconds(),
			"processing_delay_ms": tokenEvent.ProcessingDelayMs,
		}).Debug("✅ Token passed timing validation (skip validation enabled)")

		return true, "skipping validation (timing checks passed)"
	}

	// Basic validation
	if tokenEvent.Mint.IsZero() {
		return false, "invalid mint"
	}

	if tokenEvent.BondingCurve.IsZero() {
		return false, "invalid bonding curve"
	}

	t.logger.WithFields(map[string]interface{}{
		"mint":                tokenEvent.Mint.String(),
		"age_ms":              age.Milliseconds(),
		"discovery_delay_ms":  timeSinceDiscovery.Milliseconds(),
		"processing_delay_ms": tokenEvent.ProcessingDelayMs,
	}).Debug("✅ Token passed all validation including timing")

	return true, "basic validation passed with timing checks"
}

// BuyToken executes a token purchase and schedules auto-sell if enabled
func (t *Trader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	start := time.Now()

	// Create buy request
	buyRequest := BuyRequest{
		TokenEvent:     tokenEvent,
		UseTokenAmount: t.config.IsTokenBasedTrading(),
		MaxSlippage:    t.config.Trading.SlippageBP,
	}

	if t.config.IsTokenBasedTrading() {
		buyRequest.AmountTokens = t.config.Trading.BuyAmountTokens
		buyRequest.AmountSOL = float64(t.config.Trading.BuyAmountTokens) * 0.00001 // Estimate for logging
	} else {
		buyRequest.AmountSOL = t.config.Trading.BuyAmountSOL
		buyRequest.AmountTokens = 1000000 // Default token amount
	}

	// Execute buy
	buyResult, err := t.buyer.Buy(ctx, buyRequest)
	if err != nil {
		t.updateStatistics(start, false)
		return t.convertBuyResultToTradeResult(buyResult), err
	}

	// Update statistics
	t.updateStatistics(start, buyResult.Success)

	// Schedule auto-sell if enabled and purchase was successful
	if buyResult.Success && t.seller.IsEnabled() {
		t.scheduleAutoSell(tokenEvent, buyResult)
	}

	return t.convertBuyResultToTradeResult(buyResult), nil
}

// scheduleAutoSell schedules automatic selling after purchase
func (t *Trader) scheduleAutoSell(tokenEvent *TokenEvent, buyResult *BuyResult) {
	sellRequest := SellRequest{
		TokenEvent:     tokenEvent,
		PurchaseResult: buyResult,
		DelayMs:        t.config.Trading.SellDelayMs,
		SellPercentage: t.config.Trading.SellPercentage,
		CloseATA:       t.config.Trading.CloseATAAfterSell,
	}

	t.logger.WithFields(map[string]interface{}{
		"mint":            tokenEvent.Mint.String(),
		"purchase_amount": buyResult.AmountSOL,
		"delay_ms":        sellRequest.DelayMs,
		"sell_percentage": sellRequest.SellPercentage,
		"auto_sell":       true,
		"token_based":     t.config.IsTokenBasedTrading(),
	}).Info("📅 Scheduling auto-sell operation")

	t.seller.ScheduleSell(sellRequest)
}

// convertBuyResultToTradeResult converts BuyResult to TradeResult
func (t *Trader) convertBuyResultToTradeResult(buyResult *BuyResult) *TradeResult {
	if buyResult == nil {
		return &TradeResult{
			Success: false,
			Error:   "buy result is nil",
		}
	}

	return &TradeResult{
		Success:      buyResult.Success,
		Signature:    buyResult.Signature,
		AmountSOL:    buyResult.AmountSOL,
		AmountTokens: buyResult.AmountTokens,
		Price:        buyResult.Price,
		Error:        buyResult.Error,
		TradeTime:    buyResult.BuyTime.Milliseconds(),
	}
}

// updateStatistics updates trader statistics
func (t *Trader) updateStatistics(startTime time.Time, success bool) {
	t.lastTradeTime = time.Now()
	t.totalTrades++

	if success {
		t.successfulTrades++
	}
}

// Start starts the trader (if needed)
func (t *Trader) Start() error {
	t.logger.WithFields(map[string]interface{}{
		"trader_type":       t.GetTraderType(),
		"skip_validation":   t.skipValidation,
		"auto_sell_enabled": t.seller.IsEnabled(),
		"token_based":       t.config.IsTokenBasedTrading(),
		"buy_amount_sol":    t.config.Trading.BuyAmountSOL,
		"buy_amount_tokens": t.config.Trading.BuyAmountTokens,
		"sell_delay_ms":     t.config.Trading.SellDelayMs,
	}).Info("🚀 Starting trader")

	return nil
}

// Stop stops the trader and logs final statistics
func (t *Trader) Stop() {
	uptime := time.Since(t.startTime)

	t.logger.WithFields(map[string]interface{}{
		"total_trades":       t.totalTrades,
		"successful_trades":  t.successfulTrades,
		"rejected_by_timing": t.rejectedByTiming,
		"stale_tokens":       t.staleTokens,
		"uptime":             uptime.String(),
		"trader_type":        t.GetTraderType(),
	}).Info("🛑 Trader stopped")
}

// GetTradingStats returns comprehensive trading statistics
func (t *Trader) GetTradingStats() map[string]interface{} {
	successRate := float64(0)
	if t.totalTrades > 0 {
		successRate = (float64(t.successfulTrades) / float64(t.totalTrades)) * 100
	}

	uptime := time.Since(t.startTime)

	stats := map[string]interface{}{
		"trader_active":      true,
		"trader_type":        t.GetTraderType(),
		"total_trades":       t.totalTrades,
		"successful_trades":  t.successfulTrades,
		"success_rate":       fmt.Sprintf("%.1f%%", successRate),
		"skip_validation":    t.skipValidation,
		"uptime_seconds":     uptime.Seconds(),
		"rejected_by_timing": t.rejectedByTiming,
		"stale_tokens":       t.staleTokens,

		// Trading configuration
		"token_based_trading":    t.config.IsTokenBasedTrading(),
		"buy_amount_sol":         t.config.Trading.BuyAmountSOL,
		"buy_amount_tokens":      t.config.Trading.BuyAmountTokens,
		"slippage_bp":            t.config.Trading.SlippageBP,
		"max_token_age_ms":       t.config.Trading.MaxTokenAgeMs,
		"min_discovery_delay_ms": t.config.Trading.MinDiscoveryDelayMs,

		// Auto-sell configuration
		"auto_sell_enabled":    t.seller.IsEnabled(),
		"sell_delay_ms":        t.config.Trading.SellDelayMs,
		"sell_percentage":      t.config.Trading.SellPercentage,
		"close_ata_after_sell": t.config.Trading.CloseATAAfterSell,
	}

	// Add buyer stats
	buyerStats := t.buyer.GetStats()
	for k, v := range buyerStats {
		stats["buyer_"+k] = v
	}

	// Add seller stats
	sellerStats := t.seller.GetStats()
	for k, v := range sellerStats {
		stats["seller_"+k] = v
	}

	return stats
}

// GetTraderType returns the trader type
func (t *Trader) GetTraderType() string {
	traderType := "standard"

	if t.config.UltraFast.Enabled {
		traderType = "ultra_fast"
	}

	if t.seller.IsEnabled() {
		traderType += "_with_auto_sell"
	}

	if t.config.IsTokenBasedTrading() {
		traderType += "_token_based"
	} else {
		traderType += "_sol_based"
	}

	return traderType
}
