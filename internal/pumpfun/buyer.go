// internal/pumpfun/buyer.go
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
	"github.com/gagliardetto/solana-go/programs/associated-token-account"
)

// Buyer handles token purchase operations
type Buyer struct {
	wallet    *wallet.Wallet
	rpcClient *client.Client
	logger    *logger.Logger
	config    *config.Config

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
	return &Buyer{
		wallet:         wallet,
		rpcClient:      rpcClient,
		logger:         logger,
		config:         config,
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

	// Create ATA if needed
	ataAddress, err := b.ensureAssociatedTokenAccount(ctx, request.TokenEvent.Mint)
	if err != nil {
		return b.createErrorResult(startTime, fmt.Sprintf("failed to ensure ATA: %v", err)), err
	}

	// Create and send buy transaction
	result, err := b.executeBuyTransaction(ctx, request, ataAddress)
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

// ensureAssociatedTokenAccount creates ATA if it doesn't exist
func (b *Buyer) ensureAssociatedTokenAccount(ctx context.Context, mint solana.PublicKey) (solana.PublicKey, error) {
	ataAddress, err := b.wallet.GetAssociatedTokenAddress(mint)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get ATA address: %w", err)
	}

	// Check if ATA already exists
	_, err = b.rpcClient.GetAccountInfo(ctx, ataAddress.String())
	if err == nil {
		// ATA already exists
		b.logger.WithField("ata_address", ataAddress.String()).Debug("ATA already exists")
		return ataAddress, nil
	}

	// Create ATA
	b.logger.WithField("ata_address", ataAddress.String()).Debug("Creating Associated Token Account")

	err = b.createATA(ctx, mint, ataAddress)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to create ATA: %w", err)
	}

	return ataAddress, nil
}

// createATA creates an Associated Token Account
func (b *Buyer) createATA(ctx context.Context, mint, ataAddress solana.PublicKey) error {
	instruction := associatedtokenaccount.NewCreateInstruction(
		b.wallet.GetPublicKey(), // payer
		b.wallet.GetPublicKey(), // wallet
		mint,                    // mint
	).Build()

	// Get recent blockhash
	blockhash, err := b.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create and send transaction
	transaction, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		blockhash,
		solana.TransactionPayer(b.wallet.GetPublicKey()),
	)
	if err != nil {
		return fmt.Errorf("failed to create ATA transaction: %w", err)
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
		return fmt.Errorf("failed to sign ATA transaction: %w", err)
	}

	// Send transaction
	_, err = b.rpcClient.SendAndConfirmTransaction(ctx, transaction)
	if err != nil {
		return fmt.Errorf("failed to send ATA transaction: %w", err)
	}

	b.logger.WithField("ata_address", ataAddress.String()).Info("✅ ATA created successfully")
	return nil
}

// executeBuyTransaction creates and executes the buy transaction
func (b *Buyer) executeBuyTransaction(ctx context.Context, request BuyRequest, ataAddress solana.PublicKey) (*BuyResult, error) {
	// Create buy instruction
	buyInstruction := b.createBuyInstruction(request, ataAddress)

	// Get recent blockhash
	blockhash, err := b.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create transaction
	transaction, err := solana.NewTransaction(
		[]solana.Instruction{buyInstruction},
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
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
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

// createBuyInstruction creates the pump.fun buy instruction
func (b *Buyer) createBuyInstruction(request BuyRequest, userATA solana.PublicKey) solana.Instruction {
	// Get pump.fun program constants
	pumpFunProgram := solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")
	pumpFunGlobal := solana.MustPublicKeyFromBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf")
	pumpFunFeeRecipient := solana.MustPublicKeyFromBase58("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM")
	pumpFunEventAuthority := solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	// Create accounts array
	accounts := []*solana.AccountMeta{
		{PublicKey: pumpFunGlobal, IsWritable: false, IsSigner: false},
		{PublicKey: pumpFunFeeRecipient, IsWritable: true, IsSigner: false},
		{PublicKey: request.TokenEvent.Mint, IsWritable: false, IsSigner: false},
		{PublicKey: request.TokenEvent.BondingCurve, IsWritable: true, IsSigner: false},
		{PublicKey: request.TokenEvent.AssociatedBondingCurve, IsWritable: true, IsSigner: false},
		{PublicKey: userATA, IsWritable: true, IsSigner: false},
		{PublicKey: b.wallet.GetPublicKey(), IsWritable: true, IsSigner: true},
		{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},
		{PublicKey: request.TokenEvent.CreatorVault, IsWritable: true, IsSigner: false},
		{PublicKey: pumpFunEventAuthority, IsWritable: false, IsSigner: false},
		{PublicKey: pumpFunProgram, IsWritable: false, IsSigner: false},
	}

	// Create instruction data
	data := b.createBuyInstructionData(request)

	return solana.NewInstruction(
		pumpFunProgram,
		accounts,
		data,
	)
}

// createBuyInstructionData creates the buy instruction data
func (b *Buyer) createBuyInstructionData(request BuyRequest) []byte {
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

	// Buy instruction discriminator for pump.fun
	discriminator := uint64(16927863322537952870)

	data := make([]byte, 24)
	binary.LittleEndian.PutUint64(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], tokenAmount)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	return data
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

	// Create buy instruction
	buyInstruction := b.createBuyInstruction(request, ataAddress)

	// Get recent blockhash
	blockhash, err := b.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create transaction
	transaction, err := solana.NewTransaction(
		[]solana.Instruction{buyInstruction},
		blockhash,
		solana.TransactionPayer(b.wallet.GetPublicKey()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return transaction, nil
}

// EstimateBuyCost estimates the cost of a buy operation
func (b *Buyer) EstimateBuyCost(request BuyRequest) (uint64, error) {
	if request.UseTokenAmount {
		// For token-based trading, estimate SOL cost
		estimatedPrice := 0.00001 // Very rough estimate
		estimatedSOL := float64(request.AmountTokens) * estimatedPrice
		slippageFactor := 1.0 + float64(request.MaxSlippage)/10000.0
		return config.ConvertSOLToLamports(estimatedSOL * slippageFactor), nil
	} else {
		// For SOL-based trading, use the SOL amount directly
		slippageFactor := 1.0 + float64(request.MaxSlippage)/10000.0
		return config.ConvertSOLToLamports(request.AmountSOL * slippageFactor), nil
	}
}
