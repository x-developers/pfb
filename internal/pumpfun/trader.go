package pumpfun

import (
	"context"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"
	"pump-fun-bot-go/pkg/anchor"
	"pump-fun-bot-go/pkg/utils"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

// Trader handles pump.fun trading operations
type Trader struct {
	wallet      *wallet.Wallet
	rpcClient   *solana.Client
	priceCalc   *PriceCalculator
	logger      *logger.Logger
	tradeLogger *logger.TradeLogger
	config      *config.Config
	ctx         context.Context
	cancel      context.CancelFunc
}

// TradeResult represents the result of a trade operation
type TradeResult struct {
	Success         bool      `json:"success"`
	Signature       string    `json:"signature"`
	Type            string    `json:"type"` // "buy" or "sell"
	Mint            string    `json:"mint"`
	AmountSOL       float64   `json:"amount_sol"`
	AmountTokens    uint64    `json:"amount_tokens"`
	Price           float64   `json:"price"`
	SlippagePercent float64   `json:"slippage_percent"`
	GasFee          uint64    `json:"gas_fee"`
	Timestamp       time.Time `json:"timestamp"`
	Error           string    `json:"error,omitempty"`
}

// NewTrader creates a new trader instance
func NewTrader(wallet *wallet.Wallet, rpcClient *solana.Client, priceCalc *PriceCalculator, logger *logger.Logger, tradeLogger *logger.TradeLogger, config *config.Config) *Trader {
	ctx, cancel := context.WithCancel(context.Background())

	return &Trader{
		wallet:      wallet,
		rpcClient:   rpcClient,
		priceCalc:   priceCalc,
		logger:      logger,
		tradeLogger: tradeLogger,
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Stop stops the trader
func (t *Trader) Stop() {
	t.cancel()
}

// BuyToken attempts to buy a token
func (t *Trader) BuyToken(ctx context.Context, event *TokenEvent) (*TradeResult, error) {
	t.logger.WithFields(logrus.Fields{
		"mint":    event.Mint,
		"creator": event.Creator,
		"name":    event.Name,
		"symbol":  event.Symbol,
	}).Info("Attempting to buy token")

	result := &TradeResult{
		Type:      "buy",
		Mint:      event.Mint,
		Timestamp: time.Now(),
	}

	// Validate input
	if event.BondingCurve == "" {
		result.Error = "bonding curve address is required"
		return result, fmt.Errorf(result.Error)
	}

	if event.AssociatedBondingCurve == "" {
		result.Error = "associated bonding curve address is required"
		return result, fmt.Errorf(result.Error)
	}

	// Calculate buy amount
	buyAmountSOL := t.config.Trading.BuyAmountSOL
	buyAmountLamports := utils.ConvertSOLToLamports(buyAmountSOL)

	// Check wallet balance (include extra for fees and ATA creation)
	requiredBalance := buyAmountSOL + 0.015 // +0.015 SOL for fees and ATA creation
	err := t.wallet.ValidateBalanceSOL(ctx, requiredBalance)
	if err != nil {
		result.Error = fmt.Sprintf("insufficient balance: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	// Get current price and calculate tokens to receive
	tokensToReceive, err := t.priceCalc.GetTokensForSOL(ctx, event.BondingCurve, buyAmountLamports)
	if err != nil {
		result.Error = fmt.Sprintf("failed to calculate tokens: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	if tokensToReceive == 0 {
		result.Error = "would receive 0 tokens"
		return result, fmt.Errorf(result.Error)
	}

	// Calculate price with slippage protection
	maxSolCost := t.priceCalc.ApplySlippageToAmount(buyAmountLamports, t.config.Trading.SlippageBP, true)

	// Create or get ATA for the token
	ata, created, err := t.wallet.CreateATAIfNeeded(ctx, event.Mint)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create ATA: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	if created {
		t.logger.WithField("ata", base58.Encode(ata)).Info("Created new ATA for token")
	}

	// Create buy instruction
	buyInstruction, accountKeys, err := t.createBuyInstruction(event, tokensToReceive, maxSolCost)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create buy instruction: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	// Create transaction
	transaction := &wallet.SolanaTransaction{
		Signatures: make([][]byte, 1),
		Message: &wallet.SolanaMessage{
			Header: wallet.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: uint8(len(accountKeys) - 1),
			},
			AccountKeys:  accountKeys,
			Instructions: []wallet.CompiledInstruction{buyInstruction},
		},
	}

	// Get and set recent blockhash
	blockhash, err := t.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get blockhash: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	blockhashBytes, err := base58.Decode(blockhash)
	if err != nil {
		result.Error = fmt.Sprintf("failed to decode blockhash: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	copy(transaction.Message.RecentBlockhash[:], blockhashBytes)

	// Sign transaction
	err = t.wallet.SignTransaction(transaction)
	if err != nil {
		result.Error = fmt.Sprintf("failed to sign transaction: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	// Send transaction with retry logic
	signature, err := t.sendTransactionWithRetry(ctx, transaction)
	if err != nil {
		result.Error = fmt.Sprintf("failed to send transaction: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	// Calculate actual price and slippage
	actualPrice := float64(buyAmountLamports) / float64(tokensToReceive)

	// Get bonding curve data for more accurate price calculation
	curveData, err := t.priceCalc.GetBondingCurveData(ctx, event.BondingCurve)
	if err != nil {
		t.logger.WithError(err).Debug("Failed to get bonding curve data for slippage calculation")
	}

	var slippagePercent float64
	if curveData != nil {
		estimatedPrice := t.priceCalc.CalculatePrice(curveData)
		slippagePercent = t.priceCalc.CalculateSlippage(estimatedPrice, actualPrice)
	}

	// Estimate gas fee
	gasFee := t.wallet.EstimateTransactionFee([]wallet.CompiledInstruction{buyInstruction})

	// Update result
	result.Success = true
	result.Signature = signature
	result.AmountSOL = buyAmountSOL
	result.AmountTokens = tokensToReceive
	result.Price = actualPrice
	result.SlippagePercent = slippagePercent
	result.GasFee = gasFee

	// Log trade
	err = t.tradeLogger.LogBuy(
		event.Mint, event.Name, event.Symbol, event.Creator,
		buyAmountSOL, float64(tokensToReceive), actualPrice,
		signature, "success", "", gasFee,
		t.config.Trading.SlippageBP, t.config.Strategy.Type,
	)
	if err != nil {
		t.logger.WithError(err).Warn("Failed to log buy trade")
	}

	t.logger.WithFields(logrus.Fields{
		"mint":             event.Mint,
		"signature":        signature,
		"amount_sol":       buyAmountSOL,
		"amount_tokens":    tokensToReceive,
		"price":            actualPrice,
		"slippage_percent": slippagePercent,
		"gas_fee":          gasFee,
	}).Info("Successfully bought token")

	return result, nil
}

// SellToken attempts to sell a token
func (t *Trader) SellToken(ctx context.Context, mint string, amountTokens uint64) (*TradeResult, error) {
	t.logger.WithFields(logrus.Fields{
		"mint":          mint,
		"amount_tokens": amountTokens,
	}).Info("Attempting to sell token")

	result := &TradeResult{
		Type:         "sell",
		Mint:         mint,
		AmountTokens: amountTokens,
		Timestamp:    time.Now(),
	}

	// Validate token balance
	err := t.wallet.ValidateTokenAmount(ctx, mint, amountTokens)
	if err != nil {
		result.Error = fmt.Sprintf("insufficient token balance: %v", err)
		return result, fmt.Errorf(result.Error)
	}

	// TODO: For full sell implementation, we would need to:
	// 1. Store bonding curve addresses when buying tokens
	// 2. Create sell instruction with proper accounts
	// 3. Calculate minimum SOL output with slippage protection
	// 4. Send and confirm transaction

	result.Error = "sell functionality requires bonding curve address storage - will be implemented in next phase"
	return result, fmt.Errorf(result.Error)
}

// createBuyInstruction creates a buy instruction for pump.fun
func (t *Trader) createBuyInstruction(event *TokenEvent, amount, maxSolCost uint64) (wallet.CompiledInstruction, [][]byte, error) {
	// Create buy instruction data using Anchor
	instructionData := anchor.BuildBuyInstruction(amount, maxSolCost)

	// Get ATA for user's tokens
	userATA, err := t.wallet.GetAssociatedTokenAccount(event.Mint)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to get user ATA: %w", err)
	}

	// Decode required addresses
	mintBytes, err := base58.Decode(event.Mint)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to decode mint: %w", err)
	}

	bondingCurveBytes, err := base58.Decode(event.BondingCurve)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to decode bonding curve: %w", err)
	}

	associatedBondingCurveBytes, err := base58.Decode(event.AssociatedBondingCurve)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to decode associated bonding curve: %w", err)
	}

	// Account keys for pump.fun buy instruction (order is important!)
	accountKeys := [][]byte{
		config.PumpFunGlobal,         // 0: Global state account
		config.PumpFunFeeRecipient,   // 1: Fee recipient account
		mintBytes,                    // 2: Token mint account
		bondingCurveBytes,            // 3: Bonding curve account
		associatedBondingCurveBytes,  // 4: Associated bonding curve account
		userATA,                      // 5: User's associated token account
		t.wallet.GetPublicKey(),      // 6: User (payer) account
		config.SystemProgramID,       // 7: System program
		config.TokenProgramID,        // 8: Token program
		config.RentProgramID,         // 9: Rent program
		config.PumpFunEventAuthority, // 10: Event authority
		config.PumpFunProgramID,      // 11: pump.fun program
	}

	// Create compiled instruction
	instruction := wallet.CompiledInstruction{
		ProgramIdIndex: 11,                                        // pump.fun program is at index 11
		Accounts:       []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, // All accounts except program
		Data:           instructionData,
	}

	t.logger.WithFields(logrus.Fields{
		"mint":         event.Mint,
		"amount":       amount,
		"max_sol_cost": maxSolCost,
		"accounts":     len(accountKeys),
		"data_length":  len(instructionData),
	}).Debug("Created buy instruction")

	return instruction, accountKeys, nil
}

// sendTransactionWithRetry sends a transaction with retry logic and exponential backoff
func (t *Trader) sendTransactionWithRetry(ctx context.Context, transaction *wallet.SolanaTransaction) (string, error) {
	maxRetries := t.config.Advanced.MaxRetries
	retryDelay := time.Duration(t.config.Advanced.RetryDelayMs) * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			t.logger.WithFields(logrus.Fields{
				"attempt":     attempt,
				"max_retries": maxRetries,
			}).Info("Retrying transaction")

			// Calculate exponential backoff delay
			backoffDuration := retryDelay * time.Duration(1<<(attempt-1))
			if backoffDuration > 30*time.Second {
				backoffDuration = 30 * time.Second // Cap at 30 seconds
			}

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoffDuration):
			}

			// Get fresh blockhash for retry
			blockhash, err := t.rpcClient.GetLatestBlockhash(ctx)
			if err != nil {
				lastErr = fmt.Errorf("failed to get new blockhash: %w", err)
				continue
			}

			blockhashBytes, err := base58.Decode(blockhash)
			if err != nil {
				lastErr = fmt.Errorf("failed to decode new blockhash: %w", err)
				continue
			}

			copy(transaction.Message.RecentBlockhash[:], blockhashBytes)

			// Re-sign transaction with new blockhash
			err = t.wallet.SignTransaction(transaction)
			if err != nil {
				lastErr = fmt.Errorf("failed to re-sign transaction: %w", err)
				continue
			}
		}

		// Attempt to send and confirm transaction
		signature, err := t.wallet.SendAndConfirmTransaction(ctx, transaction)
		if err != nil {
			lastErr = err
			t.logger.WithError(err).WithFields(logrus.Fields{
				"attempt":   attempt,
				"signature": signature,
			}).Warn("Transaction attempt failed")
			continue
		}

		t.logger.WithFields(logrus.Fields{
			"signature": signature,
			"attempt":   attempt,
		}).Info("Transaction successful")

		return signature, nil
	}

	return "", fmt.Errorf("transaction failed after %d attempts: %w", maxRetries, lastErr)
}

// CalculateOptimalBuyAmount calculates optimal buy amount based on strategy and wallet balance
func (t *Trader) CalculateOptimalBuyAmount(ctx context.Context, event *TokenEvent) (float64, error) {
	baseAmount := t.config.Trading.BuyAmountSOL

	// Get current wallet balance
	balance, err := t.wallet.GetBalanceSOL(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get wallet balance: %w", err)
	}

	// Reserve SOL for fees and future transactions
	reserveAmount := 0.02 // Reserve 0.02 SOL
	maxSpendable := balance - reserveAmount

	if maxSpendable <= 0 {
		return 0, fmt.Errorf("insufficient balance: have %.6f SOL, need at least %.6f SOL", balance, reserveAmount)
	}

	// Don't spend more than available
	if baseAmount > maxSpendable {
		baseAmount = maxSpendable
		t.logger.WithFields(logrus.Fields{
			"original_amount": t.config.Trading.BuyAmountSOL,
			"adjusted_amount": baseAmount,
			"balance":         balance,
		}).Info("Adjusted buy amount due to balance constraints")
	}

	// Apply strategy-specific adjustments
	switch t.config.Strategy.Type {
	case "holder":
		// Holders might want to invest more in promising tokens
		if event.InitialPrice > 0 && event.InitialPrice < 0.0001 {
			baseAmount *= 1.5 // Increase by 50% for very cheap tokens
		}
	case "sniper":
		// Snipers prefer smaller, quicker trades
		baseAmount *= 0.8 // Reduce by 20% for faster execution
	}

	// Ensure within configured limits
	if baseAmount < config.MinTradeAmountSOL {
		baseAmount = config.MinTradeAmountSOL
	}
	if baseAmount > config.MaxTradeAmountSOL {
		baseAmount = config.MaxTradeAmountSOL
	}

	// Final check - don't exceed spendable amount
	if baseAmount > maxSpendable {
		baseAmount = maxSpendable
	}

	return baseAmount, nil
}

// EstimateProfitLoss estimates potential profit/loss for a trade
func (t *Trader) EstimateProfitLoss(ctx context.Context, event *TokenEvent, buyAmountSOL float64) (float64, error) {
	if event.BondingCurve == "" {
		return 0, fmt.Errorf("bonding curve address required")
	}

	// Calculate tokens we would receive
	buyAmountLamports := utils.ConvertSOLToLamports(buyAmountSOL)
	tokensToReceive, err := t.priceCalc.GetTokensForSOL(ctx, event.BondingCurve, buyAmountLamports)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate tokens: %w", err)
	}

	if tokensToReceive == 0 {
		return 0, fmt.Errorf("would receive 0 tokens")
	}

	// Get current bonding curve data
	curveData, err := t.priceCalc.GetBondingCurveData(ctx, event.BondingCurve)
	if err != nil {
		// Use default curve data if we can't fetch it
		curveData = &BondingCurveData{
			VirtualTokenReserves: 1000000 * 1e6,
			VirtualSolReserves:   30 * config.LamportsPerSol,
		}
	}

	currentPrice := t.priceCalc.CalculatePrice(curveData)

	// Estimate future scenarios
	scenarios := []struct {
		name        string
		priceChange float64
	}{
		{"conservative", 1.1}, // 10% increase
		{"moderate", 1.25},    // 25% increase
		{"optimistic", 1.5},   // 50% increase
	}

	totalEstimatedProfit := 0.0
	for _, scenario := range scenarios {
		futurePrice := currentPrice * scenario.priceChange
		futureValue := float64(tokensToReceive) * futurePrice
		profit := futureValue - buyAmountSOL
		totalEstimatedProfit += profit
	}

	// Return average of scenarios
	avgEstimatedProfit := totalEstimatedProfit / float64(len(scenarios))

	t.logger.WithFields(logrus.Fields{
		"buy_amount":           buyAmountSOL,
		"tokens_to_receive":    tokensToReceive,
		"current_price":        currentPrice,
		"avg_estimated_profit": avgEstimatedProfit,
	}).Debug("Estimated P&L")

	return avgEstimatedProfit, nil
}

// ShouldBuyToken determines if a token should be bought based on strategy and conditions
func (t *Trader) ShouldBuyToken(ctx context.Context, event *TokenEvent) (bool, string) {
	// Basic validation
	if event.BondingCurve == "" {
		return false, "no bonding curve address"
	}

	if event.AssociatedBondingCurve == "" {
		return false, "no associated bonding curve address"
	}

	// Check wallet balance
	requiredBalance := t.config.Trading.BuyAmountSOL + 0.015 // +fees and ATA
	err := t.wallet.ValidateBalanceSOL(ctx, requiredBalance)
	if err != nil {
		return false, fmt.Sprintf("insufficient balance: %v", err)
	}

	// Strategy-specific validation
	switch t.config.Strategy.Type {
	case "holder":
		// Holders are more selective and prefer tokens with potential
		if event.InitialPrice == 0 {
			return false, "no initial price available for holder strategy"
		}

		// Avoid expensive tokens
		if event.InitialPrice > 0.001 {
			return false, "initial price too high for holder strategy"
		}

		// Prefer tokens with names/symbols (better established)
		if event.Name == "" || event.Symbol == "" {
			return false, "missing token metadata for holder strategy"
		}

	case "sniper":
		// Snipers are less selective but still need basic validation
		// They primarily rely on speed and volume
	}

	// Estimate potential profit
	estimatedProfit, err := t.EstimateProfitLoss(ctx, event, t.config.Trading.BuyAmountSOL)
	if err != nil {
		t.logger.WithError(err).Debug("Failed to estimate P&L")
		// Don't fail on P&L estimation error, just log it
	} else if estimatedProfit <= 0 {
		return false, fmt.Sprintf("negative estimated profit: %.6f SOL", estimatedProfit)
	}

	// All checks passed
	return true, "passed all validation checks"
}

// GetTradeHistory returns recent trade history
func (t *Trader) GetTradeHistory() []TradeResult {
	// This would integrate with the trade logger to return recent trades
	// For now, return empty slice - full implementation would query trade logs
	return []TradeResult{}
}

// GetCurrentPositions returns current token positions
func (t *Trader) GetCurrentPositions() map[string]*logger.PositionLog {
	return t.tradeLogger.GetAllPositions()
}

// GetWalletInfo returns comprehensive wallet information
func (t *Trader) GetWalletInfo(ctx context.Context) (map[string]interface{}, error) {
	return t.wallet.GetNetworkInfo(ctx)
}

// GetTradingStats returns trading statistics
func (t *Trader) GetTradingStats() map[string]interface{} {
	positions := t.GetCurrentPositions()

	totalPositions := len(positions)
	totalValue := 0.0
	totalInvested := 0.0
	totalPnL := 0.0

	for _, position := range positions {
		if position.Position > 0 {
			totalValue += position.CurrentValue
			totalInvested += position.TotalInvested
			totalPnL += position.UnrealizedPL
		}
	}

	return map[string]interface{}{
		"total_positions":     totalPositions,
		"total_value_sol":     totalValue,
		"total_invested_sol":  totalInvested,
		"total_unrealized_pl": totalPnL,
		"active_positions":    totalPositions,
	}
}
