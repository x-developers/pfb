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
)

// Buyer handles token purchase operations
type Buyer struct {
	wallet    *wallet.Wallet
	rpcClient *client.Client
	logger    *logger.Logger
	config    *config.Config
	compute   *ComputeBudgetHelper

	// Statistics
	totalBuys      int64
	successfulBuys int64
	totalSpent     float64
	fastestBuy     time.Duration
	averageBuyTime time.Duration
}

// BuyRequest represents a token purchase request
type BuyRequest struct {
	TokenEvent     *TokenEvent
	AmountSOL      float64
	AmountTokens   uint64
	UseTokenAmount bool
	MaxSlippage    int
}

// BuyResult represents the result of a buy operation
type BuyResult struct {
	Success      bool
	Signature    solana.Signature
	AmountSOL    float64
	AmountTokens uint64
	Price        float64
	Error        string
	BuyTime      time.Duration
}

// NewBuyer creates a new buyer instance
func NewBuyer(
	wallet *wallet.Wallet,
	rpcClient *client.Client,
	logger *logger.Logger,
	config *config.Config,
) *Buyer {
	compute := NewComputeBudgetHelper(config)
	return &Buyer{
		wallet:         wallet,
		rpcClient:      rpcClient,
		logger:         logger,
		config:         config,
		compute:        compute,
		fastestBuy:     time.Hour,
		totalSpent:     0.0,
		totalBuys:      0,
		successfulBuys: 0,
	}
}

// Buy executes a token purchase
func (b *Buyer) Buy(ctx context.Context, request BuyRequest) (*BuyResult, error) {
	startTime := time.Now()

	b.logger.WithFields(map[string]interface{}{
		"mint":             request.TokenEvent.Mint.String(),
		"name":             request.TokenEvent.Name,
		"symbol":           request.TokenEvent.Symbol,
		"amount_sol":       request.AmountSOL,
		"amount_tokens":    request.AmountTokens,
		"use_token_amount": request.UseTokenAmount,
	}).Info("🛒 Executing token purchase")

	// Validate request
	if err := b.validateRequest(request); err != nil {
		return b.createErrorResult(startTime, fmt.Sprintf("invalid request: %v", err)), err
	}

	// Create and send buy transaction
	result, err := b.executeBuyTransaction(ctx, request)
	if err != nil {
		return b.createErrorResult(startTime, fmt.Sprintf("transaction failed: %v", err)), err
	}

	// Update statistics
	buyTime := time.Since(startTime)
	b.updateStatistics(buyTime, request.AmountSOL, result.Success)

	// Update result with timing
	result.BuyTime = buyTime

	if result.Success {
		b.logger.WithFields(map[string]interface{}{
			"signature":     result.Signature.String(),
			"buy_time_ms":   buyTime.Milliseconds(),
			"amount_sol":    result.AmountSOL,
			"amount_tokens": result.AmountTokens,
			"price":         result.Price,
		}).Info("✅ Token purchase successful")
	}

	return result, err
}

// validateRequest validates the buy request
func (b *Buyer) validateRequest(request BuyRequest) error {
	if request.TokenEvent == nil {
		return fmt.Errorf("token event is nil")
	}

	if request.TokenEvent.Mint.IsZero() {
		return fmt.Errorf("invalid mint address")
	}

	if request.UseTokenAmount && request.AmountTokens == 0 {
		return fmt.Errorf("token amount must be greater than 0")
	}

	if !request.UseTokenAmount && request.AmountSOL <= 0 {
		return fmt.Errorf("SOL amount must be greater than 0")
	}

	if request.MaxSlippage < 0 || request.MaxSlippage > 10000 {
		return fmt.Errorf("invalid slippage: %d (must be 0-10000 basis points)", request.MaxSlippage)
	}

	return nil
}

// executeBuyTransaction creates and executes the buy transaction
func (b *Buyer) executeBuyTransaction(ctx context.Context, request BuyRequest) (*BuyResult, error) {
	// Create transaction
	transaction, err := b.CreateBuyTransaction(ctx, request)

	//fmt.Printf(transaction.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}
	// Send transaction
	signature, err := b.rpcClient.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Calculate price
	price := 0.0
	if request.AmountTokens > 0 {
		price = request.AmountSOL / float64(request.AmountTokens)
	}

	return &BuyResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    request.AmountSOL,
		AmountTokens: request.AmountTokens,
		Price:        price,
	}, nil
}

func (b *Buyer) createFeeInstructions() []solana.Instruction {
	return b.compute.GetInstructionsForOperation("buy")
}

func (b *Buyer) createAssociatedAccountInstruction(mint solana.PublicKey) solana.Instruction {
	return CreateAssociatedAccountInstruction(mint, b.wallet.GetPublicKey())
}

// createBuyInstruction creates the pump.fun buy instruction
func (b *Buyer) createBuyInstruction(request BuyRequest, userATA solana.PublicKey) solana.Instruction {
	// Calculate amounts
	var tokenAmount uint64
	var maxSolCost uint64

	if request.UseTokenAmount {
		// Token-based trading
		tokenAmount = request.AmountTokens
		// Set a safe maximum SOL cost with slippage
		baseSOL := request.AmountSOL
		if baseSOL == 0 {
			// Estimate SOL cost if not provided
			baseSOL = float64(request.AmountTokens) * 0.00001 // Rough estimate
		}
		slippageFactor := 1.0 + float64(request.MaxSlippage)/10000.0
		maxSolCost = config.ConvertSOLToLamports(baseSOL * slippageFactor)
	} else {
		// SOL-based trading
		tokenAmount = request.AmountTokens
		if tokenAmount == 0 {
			tokenAmount = 1000000 // Default token amount
		}

		// Apply slippage to max SOL cost
		slippageFactor := 1.0 + float64(request.MaxSlippage)/10000.0
		maxSolCost = config.ConvertSOLToLamports(request.AmountSOL * slippageFactor)
	}

	// Use the shared pump.fun instruction creation function
	return CreatePumpFunBuyInstruction(
		request.TokenEvent,
		userATA,
		b.wallet.GetPublicKey(),
		tokenAmount,
		maxSolCost,
	)
}

// createErrorResult creates an error result
func (b *Buyer) createErrorResult(startTime time.Time, errorMsg string) *BuyResult {
	buyTime := time.Since(startTime)
	b.updateStatistics(buyTime, 0, false)

	return &BuyResult{
		Success: false,
		Error:   errorMsg,
		BuyTime: buyTime,
	}
}

// updateStatistics updates buyer statistics
func (b *Buyer) updateStatistics(buyTime time.Duration, amountSOL float64, success bool) {
	b.totalBuys++

	if success {
		b.successfulBuys++
		b.totalSpent += amountSOL
	}

	if buyTime < b.fastestBuy {
		b.fastestBuy = buyTime
	}

	// Update average buy time (exponential moving average)
	if b.totalBuys == 1 {
		b.averageBuyTime = buyTime
	} else {
		alpha := 0.1
		b.averageBuyTime = time.Duration(float64(b.averageBuyTime)*(1-alpha) + float64(buyTime)*alpha)
	}
}

// GetStats returns buyer statistics
func (b *Buyer) GetStats() map[string]interface{} {
	successRate := float64(0)
	if b.totalBuys > 0 {
		successRate = (float64(b.successfulBuys) / float64(b.totalBuys)) * 100
	}

	return map[string]interface{}{
		"total_buys":      b.totalBuys,
		"successful_buys": b.successfulBuys,
		"success_rate":    fmt.Sprintf("%.1f%%", successRate),
		"total_spent":     b.totalSpent,
		"fastest_buy_ms":  b.fastestBuy.Milliseconds(),
		"average_buy_ms":  b.averageBuyTime.Milliseconds(),
	}
}

// CreateBuyTransaction creates a buy transaction without executing it
func (b *Buyer) CreateBuyTransaction(ctx context.Context, request BuyRequest) (*solana.Transaction, error) {
	// Validate request
	if err := b.validateRequest(request); err != nil {
		return nil, fmt.Errorf("invalid request: %v", err)
	}

	// Get ATA address (assume it exists or will be created separately)
	ataAddress, err := b.wallet.GetAssociatedTokenAddress(request.TokenEvent.Mint)
	if err != nil {
		return nil, fmt.Errorf("failed to get ATA address: %w", err)
	}

	var instructions []solana.Instruction
	instructions = append(instructions, b.createAssociatedAccountInstruction(request.TokenEvent.Mint))
	instructions = append(instructions, b.createBuyInstruction(request, ataAddress))
	instructions = append(instructions, b.createFeeInstructions()...)

	// Get recent blockhash
	blockhash, err := b.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create transaction
	transaction, err := solana.NewTransaction(
		instructions,
		blockhash,
		solana.TransactionPayer(b.wallet.GetPublicKey()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	_, err = transaction.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if b.wallet.GetPublicKey().Equals(key) {
				account := b.wallet.GetAccount()
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
