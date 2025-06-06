package pumpfun

import (
	"context"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/wallet"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
)

// Seller handles automatic selling of tokens after purchase with curve management
type Seller struct {
	wallet       *wallet.Wallet
	rpcClient    *client.Client
	logger       *logger.Logger
	tradeLogger  *logger.TradeLogger
	config       *config.Config
	curveManager *CurveManager // NEW: Curve manager integration
	enabled      bool

	// Statistics
	totalSells      int64
	successfulSells int64
	totalReceived   float64
	fastestSell     time.Duration
	averageSellTime time.Duration

	// Curve-related stats
	totalPriceImpact   float64
	averagePriceImpact float64
	highImpactSells    int64
	slippageViolations int64
}

// SellRequest represents a request to sell tokens with curve awareness
type SellRequest struct {
	TokenEvent     *TokenEvent
	PurchaseResult *BuyResult
	DelayMs        int64
	SellPercentage float64
	CloseATA       bool

	// NEW: Curve-specific parameters
	MaxSlippagePercent float64 // Maximum acceptable slippage
	MaxPriceImpact     float64 // Maximum acceptable price impact
	UseOptimalSizing   bool    // Use curve manager for optimal sizing
	ValidateBeforeSell bool    // Validate transaction before execution
}

// SellResult represents the result of a sell operation with curve data
type SellResult struct {
	SellResult  *TradeResult
	CloseResult *ATACloseResult
	TotalTime   time.Duration
	Success     bool
	Error       string

	// NEW: Curve-related results
	CurveCalculation *CurveCalculationResult
	MarketStats      map[string]interface{}
	PriceImpact      float64
	SlippageActual   float64
	OptimalAmount    uint64
}

// ATACloseResult represents the result of closing an ATA
type ATACloseResult struct {
	Success           bool
	Signature         solana.Signature
	ReclaimedLamports uint64
	Error             string
}

// NewSeller creates a new seller instance with curve manager
func NewSeller(
	wallet *wallet.Wallet,
	rpcClient *client.Client,
	logger *logger.Logger,
	tradeLogger *logger.TradeLogger,
	config *config.Config,
) *Seller {
	return &Seller{
		wallet:       wallet,
		rpcClient:    rpcClient,
		logger:       logger,
		tradeLogger:  tradeLogger,
		config:       config,
		curveManager: NewCurveManager(rpcClient, logger), // Initialize curve manager
		enabled:      config.Trading.AutoSell,
		fastestSell:  time.Hour,
	}
}

// ScheduleSell schedules an automatic sell operation with curve analysis
func (s *Seller) ScheduleSell(request SellRequest) {
	if !s.enabled {
		s.logger.Debug("Auto-sell is disabled, skipping")
		return
	}

	// Set default curve parameters if not specified
	if request.MaxSlippagePercent == 0 {
		request.MaxSlippagePercent = float64(s.config.Trading.SlippageBP) / 100.0
	}
	if request.MaxPriceImpact == 0 {
		request.MaxPriceImpact = 5.0 // Default 5% max price impact
	}
	if !request.UseOptimalSizing {
		request.UseOptimalSizing = true // Enable by default
	}
	if !request.ValidateBeforeSell {
		request.ValidateBeforeSell = true // Enable by default
	}

	// Start sell operation in goroutine to not block main thread
	go s.executeSell(request)
}

// executeSell executes the sell operation with curve analysis and millisecond timing
func (s *Seller) executeSell(request SellRequest) {
	startTime := time.Now()

	s.logger.WithFields(map[string]interface{}{
		"mint":                 request.TokenEvent.Mint.String(),
		"delay_ms":             request.DelayMs,
		"sell_percentage":      request.SellPercentage,
		"close_ata":            request.CloseATA,
		"max_slippage":         request.MaxSlippagePercent,
		"max_price_impact":     request.MaxPriceImpact,
		"use_optimal_sizing":   request.UseOptimalSizing,
		"validate_before_sell": request.ValidateBeforeSell,
	}).Info("🕒 Scheduling curve-aware sell operation")

	// Apply delay if specified
	if request.DelayMs > 0 {
		delayDuration := time.Duration(request.DelayMs) * time.Millisecond
		s.logger.WithFields(map[string]interface{}{
			"delay_ms":       request.DelayMs,
			"delay_duration": delayDuration,
		}).Info("⏳ Waiting before sell...")
		time.Sleep(delayDuration)
	}

	// Execute the sell operation with curve analysis
	result := s.performCurveAwareSell(request, startTime)

	// Log the result
	s.logSellResult(request, result)

	// Log to trade logger if available
	if s.tradeLogger != nil {
		s.logToTradeLogger(request, result)
	}
}

// performCurveAwareSell performs the actual sell operation with curve analysis
func (s *Seller) performCurveAwareSell(request SellRequest, startTime time.Time) *SellResult {
	ctx := context.Background()

	result := &SellResult{
		Success: false,
	}

	// Step 1: Get current market stats
	s.logger.Info("📊 Analyzing bonding curve state...")
	marketStats, err := s.curveManager.GetMarketStats(ctx, request.TokenEvent.BondingCurve)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get market stats: %v", err)
		result.TotalTime = time.Since(startTime)
		return result
	}
	result.MarketStats = marketStats

	s.logger.WithFields(marketStats).Info("📈 Current market statistics")

	// Step 2: Calculate current token balance (estimated)
	var tokenBalance uint64
	if s.config.IsTokenBasedTrading() {
		tokenBalance = s.config.Trading.BuyAmountTokens
	} else {
		// Estimate token balance from SOL amount
		tokenBalance = uint64(s.config.Trading.BuyAmountSOL * 100000) // Rough conversion
	}

	// Step 3: Determine optimal sell amount
	var sellAmount uint64
	if request.UseOptimalSizing {
		s.logger.Info("🎯 Calculating optimal sell amount using curve analysis...")
		optimalAmount, curveCalc, err := s.curveManager.EstimateOptimalSellAmount(
			ctx,
			request.TokenEvent.BondingCurve,
			tokenBalance,
			request.SellPercentage,
		)
		if err != nil {
			result.Error = fmt.Sprintf("failed to calculate optimal sell amount: %v", err)
			result.TotalTime = time.Since(startTime)
			return result
		}

		sellAmount = optimalAmount
		result.CurveCalculation = curveCalc
		result.OptimalAmount = optimalAmount
		result.PriceImpact = curveCalc.PriceImpact

		s.logger.WithFields(map[string]interface{}{
			"token_balance":   tokenBalance,
			"sell_percentage": request.SellPercentage,
			"optimal_amount":  optimalAmount,
			"estimated_sol":   curveCalc.AmountOut,
			"price_impact":    curveCalc.PriceImpact,
			"fee":             curveCalc.Fee,
		}).Info("💡 Optimal sell amount calculated")

		// Check if price impact is acceptable
		if curveCalc.PriceImpact > request.MaxPriceImpact {
			s.highImpactSells++
			s.logger.WithFields(map[string]interface{}{
				"price_impact": curveCalc.PriceImpact,
				"max_allowed":  request.MaxPriceImpact,
			}).Warn("⚠️ High price impact detected, proceeding with caution")
		}
	} else {
		// Use fixed sell amount based on configuration
		if s.config.IsTokenBasedTrading() {
			sellAmount = uint64(float64(s.config.Trading.BuyAmountTokens) * (request.SellPercentage / 100.0))
		} else {
			estimatedTokens := uint64(s.config.Trading.BuyAmountSOL * 100000)
			sellAmount = uint64(float64(estimatedTokens) * (request.SellPercentage / 100.0))
		}

		s.logger.WithFields(map[string]interface{}{
			"sell_amount":     sellAmount,
			"sell_percentage": request.SellPercentage,
			"token_based":     s.config.IsTokenBasedTrading(),
		}).Info("💰 Using fixed sell amount")
	}

	if sellAmount == 0 {
		result.Error = "calculated sell amount is zero"
		result.TotalTime = time.Since(startTime)
		return result
	}

	// Step 4: Validate transaction before execution
	if request.ValidateBeforeSell {
		s.logger.Info("✅ Validating sell transaction...")
		minSolOutput := uint64(0)
		if result.CurveCalculation != nil {
			// Use calculated amount minus slippage tolerance
			slippageFactor := 1.0 - (request.MaxSlippagePercent / 100.0)
			minSolOutput = uint64(float64(result.CurveCalculation.AmountOut) * slippageFactor)
		}

		err = s.curveManager.ValidateSellTransaction(
			ctx,
			request.TokenEvent.BondingCurve,
			sellAmount,
			minSolOutput,
			s.config.Trading.SlippageBP,
		)
		if err != nil {
			s.slippageViolations++
			result.Error = fmt.Sprintf("sell validation failed: %v", err)
			result.TotalTime = time.Since(startTime)
			return result
		}

		s.logger.Info("✅ Sell transaction validation passed")
	}

	// Step 5: Execute sell transaction
	sellResult, err := s.executeSellTransaction(ctx, request.TokenEvent, sellAmount)
	if err != nil {
		result.Error = fmt.Sprintf("sell transaction failed: %v", err)
		result.TotalTime = time.Since(startTime)
		return result
	}

	result.SellResult = sellResult
	result.Success = sellResult != nil && sellResult.Success

	// Step 6: Update curve-related statistics
	if result.CurveCalculation != nil {
		s.updateCurveStatistics(result.CurveCalculation.PriceImpact)
	}

	// Step 7: Close ATA if requested and selling 100%
	if result.Success && request.SellPercentage >= 100.0 {
		s.logger.Info("🗑️ Closing ATA account...")
		ataAddress, err := s.wallet.GetAssociatedTokenAddress(request.TokenEvent.Mint)
		if err != nil {
			s.logger.WithError(err).Warn("Failed to get ATA address for closing")
		} else {
			closeResult := s.closeATA(ctx, ataAddress)
			result.CloseResult = closeResult
		}
	}

	result.TotalTime = time.Since(startTime)

	// Step 8: Log final curve analysis
	if result.Success && result.CurveCalculation != nil {
		s.logger.WithFields(map[string]interface{}{
			"signature":       sellResult.Signature,
			"tokens_sold":     sellAmount,
			"sol_received":    sellResult.AmountSOL,
			"price_impact":    result.CurveCalculation.PriceImpact,
			"fee_paid":        result.CurveCalculation.Fee,
			"effective_price": result.CurveCalculation.PricePerToken,
			"total_time_ms":   result.TotalTime.Milliseconds(),
		}).Info("🎯 Curve-aware sell completed successfully")
	}

	return result
}

// executeSellTransaction executes the sell transaction (enhanced with curve data)
func (s *Seller) executeSellTransaction(
	ctx context.Context,
	tokenEvent *TokenEvent,
	sellAmount uint64,
) (*TradeResult, error) {
	startTime := time.Now()

	s.logger.WithFields(map[string]interface{}{
		"mint":        tokenEvent.Mint.String(),
		"sell_amount": sellAmount,
		"name":        tokenEvent.Name,
		"symbol":      tokenEvent.Symbol,
	}).Info("🛒 Executing curve-optimized token sale")

	// Get expected SOL amount from curve
	curveResult, err := s.curveManager.CalculateSellReturn(ctx, tokenEvent.BondingCurve, sellAmount)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get curve calculation, using estimate")
	}

	// Create and send sell transaction
	transaction, err := s.CreateSellTransaction(ctx, tokenEvent, sellAmount)
	if err != nil {
		return nil, fmt.Errorf("failed to create sell transaction: %w", err)
	}

	// Send transaction
	signature, err := s.rpcClient.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to send sell transaction: %w", err)
	}

	// Use curve calculation if available, otherwise estimate
	var estimatedSOL float64
	var price float64

	if curveResult != nil && curveResult.AmountOut > 0 {
		estimatedSOL = config.ConvertLamportsToSOL(curveResult.AmountOut)
		price = curveResult.PricePerToken / config.LamportsPerSol

		s.logger.WithFields(map[string]interface{}{
			"curve_sol_amount": estimatedSOL,
			"curve_fee":        config.ConvertLamportsToSOL(curveResult.Fee),
			"price_impact":     curveResult.PriceImpact,
		}).Info("📊 Using curve calculation for sell result")
	} else {
		// Fallback to estimation
		estimatedSOL = s.estimateSOLReceived(sellAmount)
		if sellAmount > 0 {
			price = estimatedSOL / float64(sellAmount)
		}
	}

	sellTime := time.Since(startTime)

	// Update statistics
	s.updateStatistics(sellTime, estimatedSOL, true)

	s.logger.WithFields(map[string]interface{}{
		"signature":      signature.String(),
		"sell_time_ms":   sellTime.Milliseconds(),
		"sell_amount":    sellAmount,
		"estimated_sol":  estimatedSOL,
		"price":          price,
		"curve_enhanced": curveResult != nil,
	}).Info("✅ Curve-enhanced token sale successful")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    estimatedSOL,
		AmountTokens: sellAmount,
		Price:        price,
		TradeTime:    sellTime.Milliseconds(),
	}, nil
}

// CreateSellTransaction creates a sell transaction without executing it (enhanced with curve validation)
func (s *Seller) CreateSellTransaction(ctx context.Context, tokenEvent *TokenEvent, sellAmount uint64) (*solana.Transaction, error) {
	// Get ATA address for the token
	ataAddress, err := s.wallet.GetAssociatedTokenAddress(tokenEvent.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get ATA address: %w", err)
	}

	// Calculate minimum SOL output using curve manager
	var minSolOutput uint64
	curveResult, err := s.curveManager.CalculateSellReturn(ctx, tokenEvent.BondingCurve, sellAmount)
	if err != nil {
		s.logger.WithError(err).Warn("Failed to get curve calculation, using fallback")
		// Fallback calculation
		estimatedSOL := s.estimateSOLReceived(sellAmount)
		slippageFactor := 1.0 - float64(s.config.Trading.SlippageBP)/10000.0
		minSolOutput = config.ConvertSOLToLamports(estimatedSOL * slippageFactor)
	} else {
		// Apply slippage to curve calculation
		slippageFactor := 1.0 - float64(s.config.Trading.SlippageBP)/10000.0
		minSolOutput = uint64(float64(curveResult.AmountOut) * slippageFactor)

		s.logger.WithFields(map[string]interface{}{
			"curve_amount_out": curveResult.AmountOut,
			"slippage_bp":      s.config.Trading.SlippageBP,
			"min_sol_output":   minSolOutput,
			"price_impact":     curveResult.PriceImpact,
		}).Debug("Using curve-calculated minimum SOL output")
	}

	// Create sell instruction with curve-optimized parameters
	sellInstruction := s.createSellInstruction(tokenEvent, sellAmount, ataAddress, minSolOutput)

	// Get recent blockhash
	blockhash, err := s.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create transaction
	transaction, err := solana.NewTransaction(
		[]solana.Instruction{sellInstruction},
		blockhash,
		solana.TransactionPayer(s.wallet.GetPublicKey()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	_, err = transaction.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if s.wallet.GetPublicKey().Equals(key) {
				account := s.wallet.GetAccount()
				return &account
			}
			return nil
		},
	)

	if err != nil {
		return transaction, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return transaction, nil
}

// createSellInstruction creates a pump.fun sell instruction with curve-optimized parameters
func (s *Seller) createSellInstruction(
	tokenEvent *TokenEvent,
	sellAmount uint64,
	ataAddress solana.PublicKey,
	minSolOutput uint64,
) solana.Instruction {
	// Use the shared pump.fun instruction creation function
	return CreatePumpFunSellInstruction(
		tokenEvent,
		ataAddress,
		s.wallet.GetPublicKey(),
		sellAmount,
		minSolOutput,
	)
}

// closeATA closes the Associated Token Account and reclaims rent
func (s *Seller) closeATA(ctx context.Context, ataAddress solana.PublicKey) *ATACloseResult {
	result := &ATACloseResult{
		Success: false,
	}

	s.logger.WithField("ata_address", ataAddress.String()).Info("🗑️ Closing ATA account")

	// Create close account instruction
	closeInstruction := token.NewCloseAccountInstruction(
		ataAddress,              // Account to close
		s.wallet.GetPublicKey(), // Destination for lamports
		s.wallet.GetPublicKey(), // Owner/authority
		[]solana.PublicKey{},    // Multisig signers (empty for single signer)
	).Build()

	// Get recent blockhash
	blockhash, err := s.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get latest blockhash: %v", err)
		return result
	}

	// Create transaction
	transaction, err := solana.NewTransaction(
		[]solana.Instruction{closeInstruction},
		blockhash,
		solana.TransactionPayer(s.wallet.GetPublicKey()),
	)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create transaction: %v", err)
		return result
	}

	// Sign transaction
	_, err = transaction.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if s.wallet.GetPublicKey().Equals(key) {
				account := s.wallet.GetAccount()
				return &account
			}
			return nil
		},
	)
	if err != nil {
		result.Error = fmt.Sprintf("failed to sign transaction: %v", err)
		return result
	}

	// Send transaction
	signature, err := s.rpcClient.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		result.Error = fmt.Sprintf("failed to close ATA: %v", err)
		s.logger.WithError(err).Error("❌ Failed to close ATA")
		return result
	}

	// Estimate reclaimed lamports (typical ATA rent is ~2039280 lamports)
	reclaimedLamports := uint64(2039280) // Standard ATA rent amount

	result.Success = true
	result.Signature = signature
	result.ReclaimedLamports = reclaimedLamports

	s.logger.WithFields(map[string]interface{}{
		"signature":          signature,
		"reclaimed_lamports": reclaimedLamports,
		"reclaimed_sol":      config.ConvertLamportsToSOL(reclaimedLamports),
	}).Info("✅ ATA closed successfully")

	return result
}

// estimateSOLReceived estimates SOL received for selling tokens (fallback method)
func (s *Seller) estimateSOLReceived(tokenAmount uint64) float64 {
	// Simplified estimation - in production you'd query the bonding curve
	basePrice := 0.00001 // Very rough estimate
	return float64(tokenAmount) * basePrice
}

// updateStatistics updates seller statistics
func (s *Seller) updateStatistics(sellTime time.Duration, amountSOL float64, success bool) {
	s.totalSells++

	if success {
		s.successfulSells++
		s.totalReceived += amountSOL
	}

	if sellTime < s.fastestSell {
		s.fastestSell = sellTime
	}

	// Update average sell time (exponential moving average)
	if s.totalSells == 1 {
		s.averageSellTime = sellTime
	} else {
		alpha := 0.1
		s.averageSellTime = time.Duration(float64(s.averageSellTime)*(1-alpha) + float64(sellTime)*alpha)
	}
}

// updateCurveStatistics updates curve-related statistics
func (s *Seller) updateCurveStatistics(priceImpact float64) {
	s.totalPriceImpact += priceImpact

	// Update average price impact
	if s.totalSells == 1 {
		s.averagePriceImpact = priceImpact
	} else {
		alpha := 0.1
		s.averagePriceImpact = s.averagePriceImpact*(1-alpha) + priceImpact*alpha
	}

	// Track high impact sells (> 5%)
	if priceImpact > 5.0 {
		s.highImpactSells++
	}
}

// logSellResult logs the sell operation result with curve data
func (s *Seller) logSellResult(request SellRequest, result *SellResult) {
	fields := map[string]interface{}{
		"mint":           request.TokenEvent.Mint.String(),
		"success":        result.Success,
		"total_time":     result.TotalTime.Milliseconds(),
		"auto_sell":      true,
		"curve_enhanced": result.CurveCalculation != nil,
	}

	if result.SellResult != nil {
		fields["sell_signature"] = result.SellResult.Signature
		fields["sell_amount_tokens"] = result.SellResult.AmountTokens
		fields["sell_amount_sol"] = result.SellResult.AmountSOL
	}

	if result.CurveCalculation != nil {
		fields["price_impact"] = result.CurveCalculation.PriceImpact
		fields["curve_fee"] = result.CurveCalculation.Fee
		fields["optimal_amount"] = result.OptimalAmount
	}

	if result.CloseResult != nil {
		fields["ata_closed"] = result.CloseResult.Success
		fields["close_signature"] = result.CloseResult.Signature
		fields["reclaimed_lamports"] = result.CloseResult.ReclaimedLamports
	}

	if result.Error != "" {
		fields["error"] = result.Error
		s.logger.WithFields(fields).Error("❌ Curve-aware sell failed")
	} else {
		s.logger.WithFields(fields).Info("✅ Curve-aware sell completed successfully")
	}
}

// logToTradeLogger logs to the trade logger with curve data
func (s *Seller) logToTradeLogger(request SellRequest, result *SellResult) {
	if result.SellResult == nil || s.tradeLogger == nil {
		return
	}

	status := "success"
	errorMsg := ""
	if !result.Success {
		status = "failed"
		errorMsg = result.Error
	}

	// Calculate profit/loss
	profitLoss := 0.0
	if request.PurchaseResult != nil {
		profitLoss = result.SellResult.AmountSOL - request.PurchaseResult.AmountSOL
	}

	strategy := "curve_aware_auto_sell"
	if result.CurveCalculation != nil {
		strategy = fmt.Sprintf("curve_aware_auto_sell_impact_%.2f", result.CurveCalculation.PriceImpact)
	}

	err := s.tradeLogger.LogSell(
		request.TokenEvent.Mint.String(),
		request.TokenEvent.Name,
		request.TokenEvent.Symbol,
		result.SellResult.AmountSOL,
		float64(result.SellResult.AmountTokens),
		result.SellResult.Price,
		result.SellResult.Signature.String(),
		status,
		errorMsg,
		5000, // Estimated gas fee
		s.config.Trading.SlippageBP,
		strategy,
		profitLoss,
	)

	if err != nil {
		s.logger.WithError(err).Error("Failed to log sell to trade logger")
	}
}

// Public interface methods

// IsEnabled returns whether auto-sell is enabled
func (s *Seller) IsEnabled() bool {
	return s.enabled
}

// SetEnabled enables or disables auto-sell
func (s *Seller) SetEnabled(enabled bool) {
	s.enabled = enabled
	s.logger.WithField("enabled", enabled).Info("🔄 Auto-sell setting changed")
}

// SetTradeLogger sets the trade logger
func (s *Seller) SetTradeLogger(tradeLogger *logger.TradeLogger) {
	s.tradeLogger = tradeLogger
}

// UpdateConfig updates the seller configuration
func (s *Seller) UpdateConfig(config *config.Config) {
	s.config = config
	s.enabled = config.Trading.AutoSell

	s.logger.WithFields(map[string]interface{}{
		"auto_sell_enabled": s.enabled,
		"sell_delay_ms":     config.Trading.SellDelayMs,
		"sell_percentage":   config.Trading.SellPercentage,
		"token_based":       config.IsTokenBasedTrading(),
	}).Info("🔄 Seller configuration updated")
}

// GetStats returns seller statistics including curve data
func (s *Seller) GetStats() map[string]interface{} {
	successRate := float64(0)
	if s.totalSells > 0 {
		successRate = (float64(s.successfulSells) / float64(s.totalSells)) * 100
	}

	stats := map[string]interface{}{
		"enabled":          s.enabled,
		"total_sells":      s.totalSells,
		"successful_sells": s.successfulSells,
		"success_rate":     fmt.Sprintf("%.1f%%", successRate),
		"total_received":   s.totalReceived,
		"fastest_sell_ms":  s.fastestSell.Milliseconds(),
		"average_sell_ms":  s.averageSellTime.Milliseconds(),
		"sell_delay_ms":    s.config.Trading.SellDelayMs,
		"sell_percentage":  s.config.Trading.SellPercentage,

		// NEW: Curve-related statistics
		"curve_enhanced":       true,
		"average_price_impact": s.averagePriceImpact,
		"high_impact_sells":    s.highImpactSells,
		"slippage_violations":  s.slippageViolations,
		"total_price_impact":   s.totalPriceImpact,
	}

	return stats
}
