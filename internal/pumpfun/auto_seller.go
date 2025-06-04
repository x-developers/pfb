// internal/pumpfun/auto_seller.go
package pumpfun

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
)

// AutoSeller handles automatic selling of tokens after purchase
type AutoSeller struct {
	wallet      *wallet.Wallet
	rpcClient   *solana.Client
	jitoClient  *solana.JitoClient
	logger      *logger.Logger
	tradeLogger *logger.TradeLogger
	config      *config.Config
	enabled     bool
	useJito     bool
}

// AutoSellRequest represents a request to auto-sell a token
type AutoSellRequest struct {
	TokenEvent     *TokenEvent
	PurchaseResult *TradeResult
	DelayMs        int64
	SellPercentage float64
	CloseATA       bool
}

// AutoSellResult represents the result of an auto-sell operation
type AutoSellResult struct {
	SellResult  *TradeResult
	CloseResult *ATACloseResult
	TotalTime   time.Duration
	Success     bool
	Error       string
}

// ATACloseResult represents the result of closing an ATA
type ATACloseResult struct {
	Success           bool
	Signature         string
	ReclaimedLamports uint64
	Error             string
}

// NewAutoSeller creates a new auto-seller instance
func NewAutoSeller(
	wallet *wallet.Wallet,
	rpcClient *solana.Client,
	jitoClient *solana.JitoClient,
	logger *logger.Logger,
	tradeLogger *logger.TradeLogger,
	config *config.Config,
) *AutoSeller {
	return &AutoSeller{
		wallet:      wallet,
		rpcClient:   rpcClient,
		jitoClient:  jitoClient,
		logger:      logger,
		tradeLogger: tradeLogger,
		config:      config,
		enabled:     config.Trading.AutoSell,
		useJito:     config.JITO.Enabled && config.JITO.UseForTrading,
	}
}

// ScheduleAutoSell schedules an automatic sell operation
func (as *AutoSeller) ScheduleAutoSell(request AutoSellRequest) {
	if !as.enabled {
		as.logger.Debug("Auto-sell is disabled, skipping")
		return
	}

	// Start auto-sell in goroutine to not block main thread
	go as.executeAutoSell(request)
}

// executeAutoSell executes the auto-sell operation with timing
func (as *AutoSeller) executeAutoSell(request AutoSellRequest) {
	startTime := time.Now()

	as.logger.WithFields(map[string]interface{}{
		"mint":            request.TokenEvent.Mint.String(),
		"delay_ms":        request.DelayMs,
		"sell_percentage": request.SellPercentage,
		"close_ata":       request.CloseATA,
		"use_jito":        as.useJito,
	}).Info("🕒 Scheduling auto-sell operation")

	// Apply delay if specified
	if request.DelayMs > 0 {
		delayDuration := time.Duration(request.DelayMs) * time.Millisecond
		as.logger.WithField("delay", delayDuration).Info("⏳ Waiting before auto-sell...")
		time.Sleep(delayDuration)
	}

	// Execute the sell operation
	result := as.performAutoSell(request, startTime)

	// Log the result
	as.logAutoSellResult(request, result)

	// Log to trade logger if available
	if as.tradeLogger != nil {
		as.logToTradeLogger(request, result)
	}
}

// performAutoSell performs the actual auto-sell operation
func (as *AutoSeller) performAutoSell(request AutoSellRequest, startTime time.Time) *AutoSellResult {
	ctx := context.Background()

	result := &AutoSellResult{
		Success: false,
	}

	// Step 1: Get ATA address for the token
	ataAddress, err := as.wallet.GetAssociatedTokenAddress(*request.TokenEvent.Mint)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get ATA address: %v", err)
		result.TotalTime = time.Since(startTime)
		return result
	}

	as.logger.WithField("ata_address", ataAddress.String()).Debug("🏦 ATA address obtained")

	// Step 2: Get token balance to determine how much to sell
	tokenBalance, err := as.getTokenBalance(ctx, ataAddress)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get token balance: %v", err)
		result.TotalTime = time.Since(startTime)
		return result
	}

	if tokenBalance == 0 {
		result.Error = "no tokens to sell"
		result.TotalTime = time.Since(startTime)
		return result
	}

	// Calculate amount to sell based on percentage
	sellAmount := uint64(float64(tokenBalance) * (request.SellPercentage / 100.0))
	if sellAmount == 0 {
		sellAmount = tokenBalance // Sell all if calculation results in 0
	}

	as.logger.WithFields(map[string]interface{}{
		"token_balance":   tokenBalance,
		"sell_amount":     sellAmount,
		"sell_percentage": request.SellPercentage,
	}).Info("💰 Calculated sell amount")

	// Step 3: Execute sell transaction
	sellResult, err := as.executeSellTransaction(ctx, request.TokenEvent, sellAmount, ataAddress)
	if err != nil {
		result.Error = fmt.Sprintf("sell transaction failed: %v", err)
		result.TotalTime = time.Since(startTime)
		return result
	}

	result.SellResult = sellResult

	// Step 4: Close ATA if requested and all tokens were sold
	if request.CloseATA && sellAmount == tokenBalance {
		as.logger.Info("🗑️ Closing ATA account...")
		closeResult := as.closeATA(ctx, ataAddress, *request.TokenEvent.Mint)
		result.CloseResult = closeResult
	}

	result.Success = sellResult != nil && sellResult.Success
	result.TotalTime = time.Since(startTime)

	return result
}

// getTokenBalance gets the token balance for the ATA
func (as *AutoSeller) getTokenBalance(ctx context.Context, ataAddress common.PublicKey) (uint64, error) {
	accountInfo, err := as.rpcClient.GetAccountInfo(ctx, ataAddress.String())
	if err != nil {
		return 0, fmt.Errorf("failed to get account info: %w", err)
	}

	if accountInfo == nil {
		return 0, fmt.Errorf("ATA account not found")
	}

	// Parse token account data to get balance
	// Token account structure: https://docs.solana.com/developing/programming-model/accounts#token-accounts
	// Offset 64: amount (u64)
	if len(accountInfo.Data) < 1 || len(accountInfo.Data[0]) < 72 {
		return 0, fmt.Errorf("invalid token account data")
	}

	// Decode base64 data
	data := accountInfo.Data[0]
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return 0, fmt.Errorf("failed to decode account data: %w", err)
	}

	if len(decodedData) < 72 {
		return 0, fmt.Errorf("token account data too short")
	}

	// Extract amount (little-endian u64 at offset 64)
	amount := binary.LittleEndian.Uint64(decodedData[64:72])
	return amount, nil
}

// executeSellTransaction executes the sell transaction
func (as *AutoSeller) executeSellTransaction(
	ctx context.Context,
	tokenEvent *TokenEvent,
	sellAmount uint64,
	ataAddress common.PublicKey,
) (*TradeResult, error) {

	startTime := time.Now()

	// Create sell instruction
	sellInstruction := as.createSellInstruction(tokenEvent, sellAmount, ataAddress)

	// Create transaction with instructions
	instructions := []types.Instruction{}

	// Add priority fee if configured
	if as.config.Trading.PriorityFee > 0 {
		priorityFeeInstruction := as.createPriorityFeeInstruction()
		instructions = append(instructions, priorityFeeInstruction)
	}

	// Add compute budget instruction
	computeBudgetInstruction := as.createComputeBudgetInstruction()
	instructions = append(instructions, computeBudgetInstruction)

	// Add sell instruction
	instructions = append(instructions, sellInstruction)

	// Execute transaction based on configuration
	var signature string
	var err error

	if as.useJito && as.jitoClient != nil {
		signature, err = as.executeSellWithJito(ctx, instructions)
	} else {
		signature, err = as.executeSellRegular(ctx, instructions)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to execute sell transaction: %w", err)
	}

	tradeTime := time.Since(startTime)

	// Estimate received SOL (simplified calculation)
	estimatedSOL := as.estimateSOLReceived(sellAmount)

	as.logger.WithFields(map[string]interface{}{
		"signature":     signature,
		"sell_amount":   sellAmount,
		"estimated_sol": estimatedSOL,
		"trade_time_ms": tradeTime.Milliseconds(),
		"auto_sell":     true,
	}).Info("✅ Auto-sell transaction executed")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    estimatedSOL,
		AmountTokens: sellAmount,
		Price:        estimatedSOL / float64(sellAmount),
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// createSellInstruction creates a pump.fun sell instruction
func (as *AutoSeller) createSellInstruction(
	tokenEvent *TokenEvent,
	sellAmount uint64,
	ataAddress common.PublicKey,
) types.Instruction {

	// Calculate minimum SOL output with slippage
	estimatedSOL := as.estimateSOLReceived(sellAmount)
	slippageFactor := 1.0 - float64(as.config.Trading.SlippageBP)/10000.0
	minSolOutput := uint64(estimatedSOL * slippageFactor * config.LamportsPerSol)

	// Create sell instruction data
	discriminator := []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad} // sell discriminator
	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], sellAmount)
	binary.LittleEndian.PutUint64(data[16:24], minSolOutput)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.PumpFunProgramID),
		Accounts: []types.AccountMeta{
			{PubKey: common.PublicKeyFromBytes(config.PumpFunGlobal), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunFeeRecipient), IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.Mint, IsSigner: false, IsWritable: false},
			{PubKey: *tokenEvent.BondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: ataAddress, IsSigner: false, IsWritable: true},
			{PubKey: as.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunEventAuthority), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunProgramID), IsSigner: false, IsWritable: false},
		},
		Data: data,
	}
}

// closeATA closes the Associated Token Account and reclaims rent
func (as *AutoSeller) closeATA(ctx context.Context, ataAddress common.PublicKey, mint common.PublicKey) *ATACloseResult {
	result := &ATACloseResult{
		Success: false,
	}

	as.logger.WithField("ata_address", ataAddress.String()).Info("🗑️ Closing ATA account")

	// Create close account instruction
	closeInstruction := types.Instruction{
		ProgramID: common.TokenProgramID,
		Accounts: []types.AccountMeta{
			{PubKey: ataAddress, IsSigner: false, IsWritable: true},               // Account to close
			{PubKey: as.wallet.GetPublicKey(), IsSigner: false, IsWritable: true}, // Destination for lamports
			{PubKey: as.wallet.GetPublicKey(), IsSigner: true, IsWritable: false}, // Owner/authority
		},
		Data: []byte{9}, // CloseAccount instruction discriminator
	}

	// Execute close transaction
	instructions := []types.Instruction{closeInstruction}

	var signature string
	var err error

	if as.useJito && as.jitoClient != nil {
		signature, err = as.executeSellWithJito(ctx, instructions)
	} else {
		signature, err = as.executeSellRegular(ctx, instructions)
	}

	if err != nil {
		result.Error = fmt.Sprintf("failed to close ATA: %v", err)
		as.logger.WithError(err).Error("❌ Failed to close ATA")
		return result
	}

	// Estimate reclaimed lamports (typical ATA rent is ~2039280 lamports)
	reclaimedLamports := uint64(2039280) // Standard ATA rent amount

	result.Success = true
	result.Signature = signature
	result.ReclaimedLamports = reclaimedLamports

	as.logger.WithFields(map[string]interface{}{
		"signature":          signature,
		"reclaimed_lamports": reclaimedLamports,
		"reclaimed_sol":      config.ConvertLamportsToSOL(reclaimedLamports),
	}).Info("✅ ATA closed successfully")

	return result
}

// Helper functions for transaction execution

func (as *AutoSeller) executeSellWithJito(ctx context.Context, instructions []types.Instruction) (string, error) {
	// Create transaction
	transaction, err := as.wallet.CreateTransaction(ctx, instructions)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	// Serialize transaction
	txBytes, err := transaction.Serialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	// Encode as base64 for Jito
	encodedTx := base64.StdEncoding.EncodeToString(txBytes)

	// Send via Jito bundle
	bundleID, err := as.jitoClient.SendBundle(ctx, []string{encodedTx})
	if err != nil {
		return "", fmt.Errorf("failed to send Jito bundle: %w", err)
	}

	as.logger.WithField("bundle_id", bundleID).Info("📦 Jito bundle sent for auto-sell")

	// Wait for confirmation
	confirmCtx, cancel := context.WithTimeout(ctx, time.Duration(as.config.JITO.ConfirmTimeout)*time.Second)
	defer cancel()

	err = as.jitoClient.ConfirmBundle(confirmCtx, bundleID)
	if err != nil {
		return "", fmt.Errorf("bundle confirmation failed: %w", err)
	}

	// Get bundle status to extract signature
	status, err := as.jitoClient.GetBundleStatus(ctx, bundleID)
	if err != nil {
		return "", fmt.Errorf("failed to get bundle status: %w", err)
	}

	if len(status.Transactions) > 0 {
		return status.Transactions[0].Signature, nil
	}

	return bundleID, nil // Return bundle ID if no transaction signature available
}

func (as *AutoSeller) executeSellRegular(ctx context.Context, instructions []types.Instruction) (string, error) {
	// Create and send transaction
	transaction, err := as.wallet.CreateTransaction(ctx, instructions)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	signature, err := as.wallet.SendTransaction(ctx, transaction)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	// Wait for confirmation if not in fire-and-forget mode
	if !as.config.UltraFast.FireAndForget {
		confirmCtx, cancel := context.WithTimeout(ctx, time.Duration(as.config.Advanced.ConfirmTimeoutSec)*time.Second)
		defer cancel()

		err = as.wallet.WaitForConfirmation(confirmCtx, signature)
		if err != nil {
			as.logger.WithError(err).Warn("⚠️ Transaction sent but confirmation failed")
			// Return signature even if confirmation failed
		}
	}

	return signature, nil
}

// Enhanced configuration for auto-sell
func (as *AutoSeller) UpdateConfig(config *config.Config) {
	as.config = config
	as.enabled = config.Trading.AutoSell
	as.useJito = config.JITO.Enabled && config.JITO.UseForTrading

	as.logger.WithFields(map[string]interface{}{
		"auto_sell_enabled": as.enabled,
		"use_jito":          as.useJito,
		"sell_delay_sec":    config.Trading.SellDelaySeconds,
		"sell_percentage":   config.Trading.SellPercentage,
	}).Info("🔄 Auto-sell configuration updated")
}

// SetJitoClient sets the Jito client for the auto-seller
func (as *AutoSeller) SetJitoClient(jitoClient *solana.JitoClient) {
	as.jitoClient = jitoClient
	as.useJito = as.config.JITO.Enabled && as.config.JITO.UseForTrading
}

// SetTradeLogger sets the trade logger for the auto-seller
func (as *AutoSeller) SetTradeLogger(tradeLogger *logger.TradeLogger) {
	as.tradeLogger = tradeLogger
}

// Helper functions for instructions and calculations

func (as *AutoSeller) createPriorityFeeInstruction() types.Instruction {
	data := make([]byte, 9)
	data[0] = config.SetComputeUnitPriceInstruction
	binary.LittleEndian.PutUint64(data[1:], as.config.Trading.PriorityFee)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      data,
	}
}

func (as *AutoSeller) createComputeBudgetInstruction() types.Instruction {
	data := make([]byte, 5)
	data[0] = config.SetComputeUnitLimitInstruction
	binary.LittleEndian.PutUint32(data[1:], 200000) // Fixed compute limit

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      data,
	}
}

func (as *AutoSeller) estimateSOLReceived(tokenAmount uint64) float64 {
	// Simplified estimation - in production you'd query the bonding curve
	// This is a rough estimate based on typical pump.fun pricing
	basePrice := 0.00001 // Very rough estimate
	return float64(tokenAmount) * basePrice
}

// Logging functions

func (as *AutoSeller) logAutoSellResult(request AutoSellRequest, result *AutoSellResult) {
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
		as.logger.WithFields(fields).Error("❌ Auto-sell failed")
	} else {
		as.logger.WithFields(fields).Info("✅ Auto-sell completed successfully")
	}
}

func (as *AutoSeller) logToTradeLogger(request AutoSellRequest, result *AutoSellResult) {
	if result.SellResult == nil {
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

	err := as.tradeLogger.LogSell(
		request.TokenEvent.Mint.String(),
		request.TokenEvent.Name,
		request.TokenEvent.Symbol,
		result.SellResult.AmountSOL,
		float64(result.SellResult.AmountTokens), // Convert uint64 to float64
		result.SellResult.Price,
		result.SellResult.Signature,
		status,
		errorMsg,
		5000, // Estimated gas fee
		as.config.Trading.SlippageBP,
		"auto_sell",
		profitLoss,
	)

	if err != nil {
		as.logger.WithError(err).Error("Failed to log auto-sell to trade logger")
	}
}

// Public interface methods

// IsEnabled returns whether auto-sell is enabled
func (as *AutoSeller) IsEnabled() bool {
	return as.enabled
}

// SetEnabled enables or disables auto-sell
func (as *AutoSeller) SetEnabled(enabled bool) {
	as.enabled = enabled
	as.logger.WithField("enabled", enabled).Info("🔄 Auto-sell setting changed")
}

// GetStats returns auto-sell statistics
func (as *AutoSeller) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":         as.enabled,
		"use_jito":        as.useJito,
		"sell_delay":      as.config.Trading.SellDelaySeconds,
		"sell_percentage": as.config.Trading.SellPercentage,
	}
}
