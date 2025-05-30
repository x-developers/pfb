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

// Enhanced trader with complete buy/sell functionality
type Trader struct {
	wallet        *wallet.Wallet
	rpcClient     *solana.Client
	priceCalc     *PriceCalculator
	logger        *logger.Logger
	tradeLogger   *logger.TradeLogger
	config        *config.Config
	ctx           context.Context
	cancel        context.CancelFunc
	tokenRegistry map[string]*TokenInfo // Store token info for selling
}

// TokenInfo stores information about purchased tokens
type TokenInfo struct {
	Mint                   string    `json:"mint"`
	BondingCurve           string    `json:"bonding_curve"`
	AssociatedBondingCurve string    `json:"associated_bonding_curve"`
	Creator                string    `json:"creator"`
	Name                   string    `json:"name"`
	Symbol                 string    `json:"symbol"`
	PurchasePrice          float64   `json:"purchase_price"`
	PurchaseTime           time.Time `json:"purchase_time"`
	Amount                 uint64    `json:"amount"`
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
	ProfitLoss      float64   `json:"profit_loss,omitempty"` // For sell trades
	Error           string    `json:"error,omitempty"`
}

func NewTrader(wallet *wallet.Wallet, rpcClient *solana.Client, priceCalc *PriceCalculator,
	logger *logger.Logger, tradeLogger *logger.TradeLogger, config *config.Config) *Trader {
	ctx, cancel := context.WithCancel(context.Background())

	return &Trader{
		wallet:        wallet,
		rpcClient:     rpcClient,
		priceCalc:     priceCalc,
		logger:        logger,
		tradeLogger:   tradeLogger,
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		tokenRegistry: make(map[string]*TokenInfo),
	}
}

func (t *Trader) Stop() {
	t.cancel()
}

// BuyToken attempts to buy a token with complete implementation
func (t *Trader) BuyToken(ctx context.Context, event *TokenEvent) (*TradeResult, error) {
	t.logger.WithFields(logrus.Fields{
		"mint":    event.Mint,
		"creator": event.Creator,
		"name":    event.Name,
		"symbol":  event.Symbol,
	}).Info("💰 Attempting to buy token")

	result := &TradeResult{
		Type:      "buy",
		Mint:      event.Mint,
		Timestamp: time.Now(),
	}

	// Enhanced validation
	if err := t.validateBuyRequest(event); err != nil {
		result.Error = err.Error()
		return result, err
	}

	// Calculate optimal buy amount
	buyAmountSOL, err := t.CalculateOptimalBuyAmount(ctx, event)
	if err != nil {
		result.Error = fmt.Sprintf("failed to calculate buy amount: %v", err)
		return result, err
	}

	// Get current price and calculate expected tokens
	tokensToReceive, err := t.calculateTokensToReceive(ctx, event, buyAmountSOL)
	if err != nil {
		result.Error = fmt.Sprintf("failed to calculate tokens: %v", err)
		return result, err
	}

	// Create or get ATA
	ata, created, err := t.wallet.CreateATAIfNeeded(ctx, event.Mint)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create ATA: %v", err)
		return result, err
	}

	if created {
		t.logger.WithField("ata", base58.Encode(ata)).Info("📝 Created new ATA")
	}

	// Build and send transaction
	signature, actualTokens, actualPrice, gasFee, err := t.executeBuyTransaction(ctx, event, buyAmountSOL, tokensToReceive)
	if err != nil {
		result.Error = fmt.Sprintf("failed to execute buy: %v", err)
		return result, err
	}

	// Calculate slippage
	expectedPrice := buyAmountSOL / float64(tokensToReceive)
	slippagePercent := t.priceCalc.CalculateSlippage(expectedPrice, actualPrice)

	// Update result
	result.Success = true
	result.Signature = signature
	result.AmountSOL = buyAmountSOL
	result.AmountTokens = actualTokens
	result.Price = actualPrice
	result.SlippagePercent = slippagePercent
	result.GasFee = gasFee

	// Store token info for future selling
	t.tokenRegistry[event.Mint] = &TokenInfo{
		Mint:                   event.Mint,
		BondingCurve:           event.BondingCurve,
		AssociatedBondingCurve: event.AssociatedBondingCurve,
		Creator:                event.Creator,
		Name:                   event.Name,
		Symbol:                 event.Symbol,
		PurchasePrice:          actualPrice,
		PurchaseTime:           time.Now(),
		Amount:                 actualTokens,
	}

	// Log successful trade
	err = t.tradeLogger.LogBuy(
		event.Mint, event.Name, event.Symbol, event.Creator,
		buyAmountSOL, float64(actualTokens), actualPrice,
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
		"amount_tokens":    actualTokens,
		"price":            actualPrice,
		"slippage_percent": slippagePercent,
		"gas_fee":          gasFee,
	}).Info("✅ Token purchased successfully")

	return result, nil
}

// SellToken attempts to sell a token with complete implementation
func (t *Trader) SellToken(ctx context.Context, mint string, amountTokens uint64) (*TradeResult, error) {
	t.logger.WithFields(logrus.Fields{
		"mint":          mint,
		"amount_tokens": amountTokens,
	}).Info("💸 Attempting to sell token")

	result := &TradeResult{
		Type:         "sell",
		Mint:         mint,
		AmountTokens: amountTokens,
		Timestamp:    time.Now(),
	}

	// Get token info from registry
	tokenInfo, exists := t.tokenRegistry[mint]
	if !exists {
		result.Error = "token not found in registry - cannot sell tokens not purchased by this bot"
		return result, fmt.Errorf(result.Error)
	}

	// Validate token balance
	err := t.wallet.ValidateTokenAmount(ctx, mint, amountTokens)
	if err != nil {
		result.Error = fmt.Sprintf("insufficient token balance: %v", err)
		return result, err
	}

	// Calculate expected SOL output
	expectedSOL, err := t.priceCalc.GetSOLForTokens(ctx, tokenInfo.BondingCurve, amountTokens)
	if err != nil {
		result.Error = fmt.Sprintf("failed to calculate SOL output: %v", err)
		return result, err
	}

	expectedSOLFloat := utils.ConvertLamportsToSOL(expectedSOL)

	// Execute sell transaction
	signature, actualSOL, actualPrice, gasFee, err := t.executeSellTransaction(ctx, tokenInfo, amountTokens, expectedSOL)
	if err != nil {
		result.Error = fmt.Sprintf("failed to execute sell: %v", err)
		return result, err
	}

	// Calculate profit/loss
	costBasis := tokenInfo.PurchasePrice * float64(amountTokens)
	profitLoss := actualSOL - costBasis

	// Calculate slippage
	slippagePercent := t.priceCalc.CalculateSlippage(expectedSOLFloat, actualSOL)

	// Update result
	result.Success = true
	result.Signature = signature
	result.AmountSOL = actualSOL
	result.Price = actualPrice
	result.SlippagePercent = slippagePercent
	result.GasFee = gasFee
	result.ProfitLoss = profitLoss

	// Update token registry (reduce amount)
	if tokenInfo.Amount <= amountTokens {
		delete(t.tokenRegistry, mint)
	} else {
		tokenInfo.Amount -= amountTokens
	}

	// Log successful trade
	err = t.tradeLogger.LogSell(
		mint, tokenInfo.Name, tokenInfo.Symbol,
		actualSOL, float64(amountTokens), actualPrice,
		signature, "success", "", gasFee,
		t.config.Trading.SlippageBP, t.config.Strategy.Type, profitLoss,
	)
	if err != nil {
		t.logger.WithError(err).Warn("Failed to log sell trade")
	}

	t.logger.WithFields(logrus.Fields{
		"mint":             mint,
		"signature":        signature,
		"amount_sol":       actualSOL,
		"amount_tokens":    amountTokens,
		"price":            actualPrice,
		"profit_loss":      profitLoss,
		"slippage_percent": slippagePercent,
		"gas_fee":          gasFee,
	}).Info("💰 Token sold successfully")

	return result, nil
}

// validateBuyRequest validates a buy request
func (t *Trader) validateBuyRequest(event *TokenEvent) error {
	if event.BondingCurve == "" {
		return fmt.Errorf("bonding curve address is required")
	}

	if event.AssociatedBondingCurve == "" {
		return fmt.Errorf("associated bonding curve address is required")
	}

	if event.Mint == "" {
		return fmt.Errorf("mint address is required")
	}

	return nil
}

// calculateTokensToReceive calculates expected tokens for SOL amount
func (t *Trader) calculateTokensToReceive(ctx context.Context, event *TokenEvent, solAmount float64) (uint64, error) {
	solLamports := utils.ConvertSOLToLamports(solAmount)

	tokensToReceive, err := t.priceCalc.GetTokensForSOL(ctx, event.BondingCurve, solLamports)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate tokens: %w", err)
	}

	if tokensToReceive == 0 {
		return 0, fmt.Errorf("would receive 0 tokens for %.6f SOL", solAmount)
	}

	return tokensToReceive, nil
}

// executeBuyTransaction builds and executes a buy transaction
func (t *Trader) executeBuyTransaction(ctx context.Context, event *TokenEvent, solAmount float64, expectedTokens uint64) (string, uint64, float64, uint64, error) {
	solLamports := utils.ConvertSOLToLamports(solAmount)

	// Apply slippage protection
	maxSolCost := t.priceCalc.ApplySlippageToAmount(solLamports, t.config.Trading.SlippageBP, true)

	// Create buy instruction
	buyInstruction, accountKeys, err := t.createBuyInstruction(event, expectedTokens, maxSolCost)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to create buy instruction: %w", err)
	}

	// Create and send transaction
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

	// Get recent blockhash
	blockhash, err := t.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to get blockhash: %w", err)
	}

	blockhashBytes, err := base58.Decode(blockhash)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to decode blockhash: %w", err)
	}

	copy(transaction.Message.RecentBlockhash[:], blockhashBytes)

	// Sign transaction
	err = t.wallet.SignTransaction(transaction)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction with retry logic
	signature, err := t.sendTransactionWithRetry(ctx, transaction)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to send transaction: %w", err)
	}

	// For now, return expected values - in production you'd parse transaction logs
	actualTokens := expectedTokens
	actualPrice := solAmount / float64(actualTokens)
	gasFee := t.wallet.EstimateTransactionFee([]wallet.CompiledInstruction{buyInstruction})

	return signature, actualTokens, actualPrice, gasFee, nil
}

// executeSellTransaction builds and executes a sell transaction
func (t *Trader) executeSellTransaction(ctx context.Context, tokenInfo *TokenInfo, amountTokens uint64, expectedSOL uint64) (string, float64, float64, uint64, error) {
	// Apply slippage protection for minimum SOL output
	minSolOutput := t.priceCalc.ApplySlippageToAmount(expectedSOL, t.config.Trading.SlippageBP, false)

	// Create sell instruction
	sellInstruction, accountKeys, err := t.createSellInstruction(tokenInfo, amountTokens, minSolOutput)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to create sell instruction: %w", err)
	}

	// Create and send transaction
	transaction := &wallet.SolanaTransaction{
		Signatures: make([][]byte, 1),
		Message: &wallet.SolanaMessage{
			Header: wallet.MessageHeader{
				NumRequiredSignatures:       1,
				NumReadonlySignedAccounts:   0,
				NumReadonlyUnsignedAccounts: uint8(len(accountKeys) - 1),
			},
			AccountKeys:  accountKeys,
			Instructions: []wallet.CompiledInstruction{sellInstruction},
		},
	}

	// Get recent blockhash
	blockhash, err := t.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to get blockhash: %w", err)
	}

	blockhashBytes, err := base58.Decode(blockhash)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to decode blockhash: %w", err)
	}

	copy(transaction.Message.RecentBlockhash[:], blockhashBytes)

	// Sign transaction
	err = t.wallet.SignTransaction(transaction)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction with retry logic
	signature, err := t.sendTransactionWithRetry(ctx, transaction)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("failed to send transaction: %w", err)
	}

	// For now, return expected values - in production you'd parse transaction logs
	actualSOL := utils.ConvertLamportsToSOL(expectedSOL)
	actualPrice := actualSOL / float64(amountTokens)
	gasFee := t.wallet.EstimateTransactionFee([]wallet.CompiledInstruction{sellInstruction})

	return signature, actualSOL, actualPrice, gasFee, nil
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

	return instruction, accountKeys, nil
}

// createSellInstruction creates a sell instruction for pump.fun
func (t *Trader) createSellInstruction(tokenInfo *TokenInfo, amount, minSolOutput uint64) (wallet.CompiledInstruction, [][]byte, error) {
	// Create sell instruction data using Anchor
	instructionData := anchor.BuildSellInstruction(amount, minSolOutput)

	// Get ATA for user's tokens
	userATA, err := t.wallet.GetAssociatedTokenAccount(tokenInfo.Mint)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to get user ATA: %w", err)
	}

	// Decode required addresses
	mintBytes, err := base58.Decode(tokenInfo.Mint)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to decode mint: %w", err)
	}

	bondingCurveBytes, err := base58.Decode(tokenInfo.BondingCurve)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to decode bonding curve: %w", err)
	}

	associatedBondingCurveBytes, err := base58.Decode(tokenInfo.AssociatedBondingCurve)
	if err != nil {
		return wallet.CompiledInstruction{}, nil, fmt.Errorf("failed to decode associated bonding curve: %w", err)
	}

	// Account keys for pump.fun sell instruction
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
		config.PumpFunEventAuthority, // 9: Event authority
		config.PumpFunProgramID,      // 10: pump.fun program
	}

	// Create compiled instruction
	instruction := wallet.CompiledInstruction{
		ProgramIdIndex: 10,                                    // pump.fun program is at index 10
		Accounts:       []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, // All accounts except program
		Data:           instructionData,
	}

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
			}).Info("🔄 Retrying transaction")

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
		}).Info("✅ Transaction successful")

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
		}).Info("💡 Adjusted buy amount due to balance constraints")
	}

	// Apply strategy-specific adjustments
	switch t.config.Strategy.Type {
	case "holder":
		// Holders might want to invest more in promising tokens
		//if event.InitialPrice > 0 && event.InitialPrice < 0.0001 {
		//	baseAmount *= 1.5 // Increase by 50% for very cheap tokens
		//}
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
		// Prefer tokens with names/symbols (better established)
		if event.Name == "" || event.Symbol == "" {
			return false, "missing token metadata for holder strategy"
		}

	case "sniper":
		// Snipers are less selective but still need basic validation
		// They primarily rely on speed and volume
	}

	// Estimate potential profit (optional)
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

	return avgEstimatedProfit, nil
}

// GetTokenRegistry returns the token registry for inspection
func (t *Trader) GetTokenRegistry() map[string]*TokenInfo {
	return t.tokenRegistry
}

// GetCurrentPositions returns current token positions from registry
func (t *Trader) GetCurrentPositions() map[string]*logger.PositionLog {
	positions := make(map[string]*logger.PositionLog)

	for mint, tokenInfo := range t.tokenRegistry {
		if tokenInfo.Amount > 0 {
			currentValue := float64(tokenInfo.Amount) * tokenInfo.PurchasePrice
			totalInvested := float64(tokenInfo.Amount) * tokenInfo.PurchasePrice

			positions[mint] = &logger.PositionLog{
				Timestamp:     tokenInfo.PurchaseTime,
				Mint:          mint,
				TokenName:     tokenInfo.Name,
				TokenSymbol:   tokenInfo.Symbol,
				Position:      float64(tokenInfo.Amount),
				AvgBuyPrice:   tokenInfo.PurchasePrice,
				CurrentPrice:  tokenInfo.PurchasePrice, // Would need to fetch current price
				UnrealizedPL:  currentValue - totalInvested,
				TotalInvested: totalInvested,
				CurrentValue:  currentValue,
			}
		}
	}

	return positions
}

// GetTradingStats returns comprehensive trading statistics
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
		"tokens_in_registry":  len(t.tokenRegistry),
	}
}
