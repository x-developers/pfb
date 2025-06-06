package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/wallet"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
)

// Seller handles automatic selling of tokens after purchase
type Seller struct {
	wallet      *wallet.Wallet
	rpcClient   *client.Client
	logger      *logger.Logger
	tradeLogger *logger.TradeLogger
	config      *config.Config
	enabled     bool

	// Statistics
	totalSells      int64
	successfulSells int64
	totalReceived   float64
	fastestSell     time.Duration
	averageSellTime time.Duration
}

// SellRequest represents a request to sell tokens
type SellRequest struct {
	TokenEvent     *TokenEvent
	PurchaseResult *BuyResult
	DelayMs        int64
	SellPercentage float64
	CloseATA       bool
}

// SellResult represents the result of a sell operation
type SellResult struct {
	SellResult  *TradeResult
	CloseResult *ATACloseResult
	TotalTime   time.Duration
	Success     bool
	Error       string
}

// ATACloseResult represents the result of closing an ATA
type ATACloseResult struct {
	Success           bool
	Signature         solana.Signature
	ReclaimedLamports uint64
	Error             string
}

// NewSeller creates a new seller instance
func NewSeller(
	wallet *wallet.Wallet,
	rpcClient *client.Client,
	logger *logger.Logger,
	tradeLogger *logger.TradeLogger,
	config *config.Config,
) *Seller {
	return &Seller{
		wallet:      wallet,
		rpcClient:   rpcClient,
		logger:      logger,
		tradeLogger: tradeLogger,
		config:      config,
		enabled:     config.Trading.AutoSell,
		fastestSell: time.Hour,
	}
}

// ScheduleSell schedules an automatic sell operation
func (s *Seller) ScheduleSell(request SellRequest) {
	if !s.enabled {
		s.logger.Debug("Auto-sell is disabled, skipping")
		return
	}

	// Start sell operation in goroutine to not block main thread
	go s.executeSell(request)
}

// executeSell executes the sell operation with millisecond timing
func (s *Seller) executeSell(request SellRequest) {
	startTime := time.Now()

	s.logger.WithFields(map[string]interface{}{
		"mint":            request.TokenEvent.Mint.String(),
		"delay_ms":        request.DelayMs,
		"sell_percentage": request.SellPercentage,
		"close_ata":       request.CloseATA,
	}).Info("🕒 Scheduling sell operation")

	// Apply delay if specified
	if request.DelayMs > 0 {
		delayDuration := time.Duration(request.DelayMs) * time.Millisecond
		s.logger.WithFields(map[string]interface{}{
			"delay_ms":       request.DelayMs,
			"delay_duration": delayDuration,
		}).Info("⏳ Waiting before sell...")
		time.Sleep(delayDuration)
	}

	// Execute the sell operation
	result := s.performSell(request, startTime)

	// Log the result
	s.logSellResult(request, result)

	// Log to trade logger if available
	if s.tradeLogger != nil {
		s.logToTradeLogger(request, result)
	}
}

// performSell performs the actual sell operation
func (s *Seller) performSell(request SellRequest, startTime time.Time) *SellResult {
	ctx := context.Background()

	result := &SellResult{
		Success: false,
	}

	// Calculate fixed sell amount based on configuration
	var sellAmount uint64
	if s.config.IsTokenBasedTrading() {
		// Use the same fixed amount that was purchased
		sellAmount = s.config.Trading.BuyAmountTokens

		// Apply sell percentage if less than 100%
		if request.SellPercentage < 100.0 {
			sellAmount = uint64(float64(sellAmount) * (request.SellPercentage / 100.0))
		}
	} else {
		// For SOL-based trading, estimate token amount from SOL amount
		// This is a rough estimation - in practice you'd query the actual balance
		estimatedTokens := uint64(s.config.Trading.BuyAmountSOL * 100000) // Rough conversion
		sellAmount = uint64(float64(estimatedTokens) * (request.SellPercentage / 100.0))
	}

	if sellAmount == 0 {
		result.Error = "calculated sell amount is zero"
		result.TotalTime = time.Since(startTime)
		return result
	}

	s.logger.WithFields(map[string]interface{}{
		"sell_amount":     sellAmount,
		"sell_percentage": request.SellPercentage,
		"token_based":     s.config.IsTokenBasedTrading(),
	}).Info("💰 Using fixed sell amount")

	// Execute sell transaction

	sellResult, err := s.executeSellTransaction(ctx, request.TokenEvent, sellAmount)
	if err != nil {
		result.Error = fmt.Sprintf("sell transaction failed: %v", err)
		result.TotalTime = time.Since(startTime)
		return result
	}

	result.SellResult = sellResult
	result.Success = sellResult != nil && sellResult.Success

	// Close ATA if requested and selling 100%
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

	return result
}

// executeSellTransaction executes the sell transaction (similar to executeBuyTransaction)
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
	}).Info("🛒 Executing token sale")

	// Create and send sell transaction
	transaction, err := s.CreateSellTransaction(ctx, tokenEvent, sellAmount)
	fmt.Println(transaction.String())

	if err != nil {
		return nil, fmt.Errorf("failed to create sell transaction: %w", err)
	}

	// Send transaction
	signature, err := s.rpcClient.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to send sell transaction: %w", err)
	}

	// Calculate estimated SOL received (simplified calculation)
	estimatedSOL := s.estimateSOLReceived(sellAmount)

	// Calculate price
	price := 0.0
	if sellAmount > 0 {
		price = estimatedSOL / float64(sellAmount)
	}

	sellTime := time.Since(startTime)

	// Update statistics
	s.updateStatistics(sellTime, estimatedSOL, true)

	s.logger.WithFields(map[string]interface{}{
		"signature":     signature.String(),
		"sell_time_ms":  sellTime.Milliseconds(),
		"sell_amount":   sellAmount,
		"estimated_sol": estimatedSOL,
		"price":         price,
	}).Info("✅ Token sale successful")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    estimatedSOL,
		AmountTokens: sellAmount,
		Price:        price,
		TradeTime:    sellTime.Milliseconds(),
	}, nil
}

// CreateSellTransaction creates a sell transaction without executing it
func (s *Seller) CreateSellTransaction(ctx context.Context, tokenEvent *TokenEvent, sellAmount uint64) (*solana.Transaction, error) {
	// Get ATA address for the token
	ataAddress, err := s.wallet.GetAssociatedTokenAddress(tokenEvent.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get ATA address: %w", err)
	}

	// Create sell instruction
	sellInstruction := s.createSellInstruction(tokenEvent, sellAmount, ataAddress)
	s.logger.LogInstruction(sellInstruction)
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

	fmt.Printf(transaction.String())

	return transaction, nil
}

// createSellInstruction creates a pump.fun sell instruction
func (s *Seller) createSellInstruction(
	tokenEvent *TokenEvent,
	sellAmount uint64,
	ataAddress solana.PublicKey,
) solana.Instruction {
	// Calculate minimum SOL output with slippage
	estimatedSOL := s.estimateSOLReceived(sellAmount)
	slippageFactor := 1.0 - float64(s.config.Trading.SlippageBP)/10000.0
	minSolOutput := uint64(estimatedSOL * slippageFactor * config.LamportsPerSol)

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
		{PublicKey: ataAddress, IsWritable: true, IsSigner: false},
		{PublicKey: s.wallet.GetPublicKey(), IsWritable: true, IsSigner: true},
		{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: tokenEvent.CreatorVault, IsWritable: true, IsSigner: false},

		{PublicKey: pumpFunEventAuthority, IsWritable: false, IsSigner: false},
		{PublicKey: pumpFunProgram, IsWritable: false, IsSigner: false},
	}

	// Create instruction data
	data := s.createSellInstructionData(sellAmount, minSolOutput)

	return solana.NewInstruction(
		pumpFunProgram,
		accounts,
		data,
	)
}

// createSellInstructionData creates the sell instruction data
func (s *Seller) createSellInstructionData(sellAmount uint64, minSolOutput uint64) []byte {
	// Sell instruction discriminator for pump.fun
	discriminator := uint64(12502976635542562355) // sell discriminator from decoder.go

	data := make([]byte, 24)
	binary.LittleEndian.PutUint64(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], sellAmount)
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput)

	return data
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

// estimateSOLReceived estimates SOL received for selling tokens
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

// logSellResult logs the sell operation result
func (s *Seller) logSellResult(request SellRequest, result *SellResult) {
	fields := map[string]interface{}{
		"mint":       request.TokenEvent.Mint.String(),
		"success":    result.Success,
		"total_time": result.TotalTime.Milliseconds(),
		"auto_sell":  true,
	}

	if result.SellResult != nil {
		fields["sell_signature"] = result.SellResult.Signature
		fields["sell_amount_tokens"] = result.SellResult.AmountTokens
		fields["sell_amount_sol"] = result.SellResult.AmountSOL
	}

	if result.CloseResult != nil {
		fields["ata_closed"] = result.CloseResult.Success
		fields["close_signature"] = result.CloseResult.Signature
		fields["reclaimed_lamports"] = result.CloseResult.ReclaimedLamports
	}

	if result.Error != "" {
		fields["error"] = result.Error
		s.logger.WithFields(fields).Error("❌ Sell failed")
	} else {
		s.logger.WithFields(fields).Info("✅ Sell completed successfully")
	}
}

// logToTradeLogger logs to the trade logger
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
		"auto_sell",
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

// GetStats returns seller statistics
func (s *Seller) GetStats() map[string]interface{} {
	successRate := float64(0)
	if s.totalSells > 0 {
		successRate = (float64(s.successfulSells) / float64(s.totalSells)) * 100
	}

	return map[string]interface{}{
		"enabled":          s.enabled,
		"total_sells":      s.totalSells,
		"successful_sells": s.successfulSells,
		"success_rate":     fmt.Sprintf("%.1f%%", successRate),
		"total_received":   s.totalReceived,
		"fastest_sell_ms":  s.fastestSell.Milliseconds(),
		"average_sell_ms":  s.averageSellTime.Milliseconds(),
		"sell_delay_ms":    s.config.Trading.SellDelayMs,
		"sell_percentage":  s.config.Trading.SellPercentage,
	}
}
