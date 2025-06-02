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

// JitoTrader wraps trading operations with MEV protection via Jito
type JitoTrader struct {
	trader     TraderInterface
	jitoClient *solana.JitoClient
	wallet     *wallet.Wallet
	logger     *logger.Logger
	config     *config.Config
	enabled    bool
}

// NewJitoTrader creates a new Jito-enhanced trader
func NewJitoTrader(
	baseTrader TraderInterface,
	jitoClient *solana.JitoClient,
	wallet *wallet.Wallet,
	logger *logger.Logger,
	config *config.Config,
) *JitoTrader {
	return &JitoTrader{
		trader:     baseTrader,
		jitoClient: jitoClient,
		wallet:     wallet,
		logger:     logger,
		config:     config,
		enabled:    config.JITO.Enabled && config.JITO.UseForTrading,
	}
}

// ShouldBuyToken delegates to base trader
func (jt *JitoTrader) ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string) {
	return jt.trader.ShouldBuyToken(ctx, tokenEvent)
}

// BuyToken executes buy with optional Jito MEV protection
func (jt *JitoTrader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	startTime := time.Now()

	// If Jito is not enabled or not configured for trading, use base trader
	if !jt.enabled {
		return jt.trader.BuyToken(ctx, tokenEvent)
	}

	jt.logger.WithFields(map[string]interface{}{
		"mint":         tokenEvent.Mint.String(),
		"jito_enabled": true,
		"trader_type":  jt.trader.GetTraderType(),
	}).Info("🛡️ Executing Jito-protected buy transaction")

	// Create the buy transaction
	buyTransaction, err := jt.createBuyTransaction(ctx, tokenEvent)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("failed to create buy transaction: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	// Create tip transaction if configured
	var tipTransaction string
	if jt.config.JITO.TipAmount > 0 {
		tipTransaction, err = jt.createTipTransaction(ctx)
		if err != nil {
			jt.logger.WithError(err).Warn("Failed to create tip transaction, continuing without tip")
		}
	}

	// Send via Jito bundle
	bundleID, err := jt.sendJitoBundle(ctx, buyTransaction, tipTransaction)
	if err != nil {
		// Fallback to regular transaction if Jito fails
		jt.logger.WithError(err).Warn("Jito bundle failed, falling back to regular transaction")
		return jt.fallbackToRegularTransaction(ctx, tokenEvent, startTime)
	}

	// Monitor bundle status
	signature, err := jt.waitForBundleConfirmation(ctx, bundleID)
	if err != nil {
		return &TradeResult{
			Success:   false,
			Error:     fmt.Sprintf("bundle confirmation failed: %v", err),
			TradeTime: time.Since(startTime).Milliseconds(),
		}, err
	}

	tradeTime := time.Since(startTime)
	buyAmountSOL := jt.config.Trading.BuyAmountSOL

	jt.logger.WithFields(map[string]interface{}{
		"signature":      signature,
		"bundle_id":      bundleID,
		"trade_time":     tradeTime.Milliseconds(),
		"amount_sol":     buyAmountSOL,
		"jito_protected": true,
	}).Info("🛡️ Jito-protected buy transaction successful")

	return &TradeResult{
		Success:      true,
		Signature:    signature,
		AmountSOL:    buyAmountSOL,
		AmountTokens: 1000000, // This would be calculated properly
		Price:        buyAmountSOL / 1000000,
		TradeTime:    tradeTime.Milliseconds(),
	}, nil
}

// createBuyTransaction creates a buy transaction without sending it
func (jt *JitoTrader) createBuyTransaction(ctx context.Context, tokenEvent *TokenEvent) (string, error) {
	// Get the buy instructions from the base trader
	var instructions []types.Instruction

	// Add priority fee if configured
	if jt.config.Trading.PriorityFee > 0 {
		priorityFeeInstruction := jt.createPriorityFeeInstruction()
		instructions = append(instructions, priorityFeeInstruction)
	}

	// Create ATA instruction if needed
	ataInstruction, err := jt.createATAInstructionIfNeeded(ctx, *tokenEvent.Mint)
	if err != nil {
		return "", fmt.Errorf("failed to create ATA instruction: %w", err)
	}
	if ataInstruction != nil {
		instructions = append(instructions, *ataInstruction)
	}

	// Create buy instruction
	buyInstruction, err := jt.createBuyInstruction(tokenEvent)
	if err != nil {
		return "", fmt.Errorf("failed to create buy instruction: %w", err)
	}
	instructions = append(instructions, buyInstruction)

	// Create transaction
	transaction, err := jt.wallet.CreateTransaction(ctx, instructions)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	// Serialize transaction to base64
	txBytes, err := transaction.Serialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	return base64.StdEncoding.EncodeToString(txBytes), nil
}

// createTipTransaction creates a tip transaction for MEV protection
func (jt *JitoTrader) createTipTransaction(ctx context.Context) (string, error) {
	// Get a random tip account
	tipAccount, err := jt.jitoClient.GetRandomTipAccount(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get tip account: %w", err)
	}

	// Parse tip account address
	tipAccountPubkey := common.PublicKeyFromString(tipAccount)

	// Create tip instruction (transfer SOL to tip account)
	tipInstruction := types.Instruction{
		ProgramID: common.SystemProgramID,
		Accounts: []types.AccountMeta{
			{PubKey: jt.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},
			{PubKey: tipAccountPubkey, IsSigner: false, IsWritable: true},
		},
		Data: jt.createTransferInstructionData(jt.config.JITO.TipAmount),
	}

	// Create tip transaction
	tipTx, err := jt.wallet.CreateTransaction(ctx, []types.Instruction{tipInstruction})
	if err != nil {
		return "", fmt.Errorf("failed to create tip transaction: %w", err)
	}

	// Serialize tip transaction
	tipBytes, err := tipTx.Serialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize tip transaction: %w", err)
	}

	return base64.StdEncoding.EncodeToString(tipBytes), nil
}

// sendJitoBundle sends transactions via Jito bundle
func (jt *JitoTrader) sendJitoBundle(ctx context.Context, buyTx, tipTx string) (string, error) {
	var transactions []string

	// Add buy transaction
	transactions = append(transactions, buyTx)

	// Add tip transaction if provided
	if tipTx != "" {
		transactions = append(transactions, tipTx)
	}

	// Estimate bundle cost for logging
	estimatedCost := jt.jitoClient.EstimateBundleCost(len(transactions), jt.config.JITO.TipAmount)

	jt.logger.WithFields(map[string]interface{}{
		"transactions":   len(transactions),
		"tip_amount":     jt.config.JITO.TipAmount,
		"estimated_cost": estimatedCost,
	}).Info("📦 Sending Jito bundle")

	// Send bundle
	bundleID, err := jt.jitoClient.SendBundle(ctx, transactions)
	if err != nil {
		return "", fmt.Errorf("failed to send Jito bundle: %w", err)
	}

	return bundleID, nil
}

// waitForBundleConfirmation waits for bundle confirmation and extracts signature
func (jt *JitoTrader) waitForBundleConfirmation(ctx context.Context, bundleID string) (string, error) {
	// Create timeout context for confirmation
	confirmCtx, cancel := context.WithTimeout(ctx, time.Duration(jt.config.JITO.ConfirmTimeout)*time.Second)
	defer cancel()

	jt.logger.WithField("bundle_id", bundleID).Info("⏳ Waiting for Jito bundle confirmation")

	// Wait for confirmation
	err := jt.jitoClient.ConfirmBundle(confirmCtx, bundleID)
	if err != nil {
		return "", fmt.Errorf("bundle confirmation failed: %w", err)
	}

	// Get bundle status to extract transaction signatures
	status, err := jt.jitoClient.GetBundleStatus(ctx, bundleID)
	if err != nil {
		return "", fmt.Errorf("failed to get bundle status: %w", err)
	}

	// Return the first transaction signature (buy transaction)
	if len(status.Transactions) > 0 {
		return status.Transactions[0].Signature, nil
	}

	return "", fmt.Errorf("no transaction signatures found in bundle")
}

// fallbackToRegularTransaction falls back to regular transaction if Jito fails
func (jt *JitoTrader) fallbackToRegularTransaction(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {
	jt.logger.Warn("🔄 Falling back to regular transaction")

	// Use the base trader for fallback
	result, err := jt.trader.BuyToken(ctx, tokenEvent)
	if err != nil {
		return result, err
	}

	// Update the trade time to include the Jito attempt time
	if result != nil {
		result.TradeTime = time.Since(startTime).Milliseconds()
	}

	return result, nil
}

// Helper methods

func (jt *JitoTrader) createPriorityFeeInstruction() types.Instruction {
	data := make([]byte, 9)
	data[0] = config.SetComputeUnitPriceInstruction
	binary.LittleEndian.PutUint64(data[1:], jt.config.Trading.PriorityFee)

	return types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.GetComputeBudgetProgramID()),
		Accounts:  []types.AccountMeta{},
		Data:      data,
	}
}

func (jt *JitoTrader) createATAInstructionIfNeeded(ctx context.Context, mint common.PublicKey) (*types.Instruction, error) {
	ata, err := jt.wallet.GetAssociatedTokenAddress(mint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive ATA: %w", err)
	}

	// Check if ATA already exists (simplified check)
	// In a full implementation, you'd check the account exists via RPC

	// Create ATA instruction
	instruction := types.Instruction{
		ProgramID: common.SPLAssociatedTokenAccountProgramID,
		Accounts: []types.AccountMeta{
			{PubKey: jt.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},   // Payer
			{PubKey: ata, IsSigner: false, IsWritable: true},                       // Associated token account
			{PubKey: jt.wallet.GetPublicKey(), IsSigner: false, IsWritable: false}, // Owner
			{PubKey: mint, IsSigner: false, IsWritable: false},                     // Mint
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},   // System program
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},    // Token program
		},
		Data: []byte{}, // ATA creation instruction has no data
	}

	return &instruction, nil
}

func (jt *JitoTrader) createBuyInstruction(tokenEvent *TokenEvent) (types.Instruction, error) {
	userATA, err := jt.wallet.GetAssociatedTokenAddress(*tokenEvent.Mint)
	if err != nil {
		return types.Instruction{}, err
	}

	instruction := types.Instruction{
		ProgramID: common.PublicKeyFromBytes(config.PumpFunProgramID),
		Accounts: []types.AccountMeta{
			{PubKey: common.PublicKeyFromBytes(config.PumpFunGlobal), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunFeeRecipient), IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.Mint, IsSigner: false, IsWritable: false},
			{PubKey: *tokenEvent.BondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: *tokenEvent.AssociatedBondingCurve, IsSigner: false, IsWritable: true},
			{PubKey: userATA, IsSigner: false, IsWritable: true},
			{PubKey: jt.wallet.GetPublicKey(), IsSigner: true, IsWritable: true},
			{PubKey: common.SystemProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.TokenProgramID, IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.RentProgramID), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunEventAuthority), IsSigner: false, IsWritable: false},
			{PubKey: common.PublicKeyFromBytes(config.PumpFunProgramID), IsSigner: false, IsWritable: false},
		},
		Data: jt.createBuyInstructionData(),
	}

	return instruction, nil
}

func (jt *JitoTrader) createBuyInstructionData() []byte {
	// Pump.fun buy instruction discriminator
	discriminator := []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}

	buyAmountLamports := config.ConvertSOLToLamports(jt.config.Trading.BuyAmountSOL)
	tokenAmount := uint64(1000000) // This should be calculated based on bonding curve

	// Apply slippage protection
	slippageFactor := 1.0 + float64(jt.config.Trading.SlippageBP)/10000.0
	maxSolCost := uint64(float64(buyAmountLamports) * slippageFactor)

	data := make([]byte, 24)
	copy(data[0:8], discriminator)
	binary.LittleEndian.PutUint64(data[8:16], tokenAmount)
	binary.LittleEndian.PutUint64(data[16:24], maxSolCost)

	return data
}

func (jt *JitoTrader) createTransferInstructionData(amount uint64) []byte {
	// System program transfer instruction data
	// Instruction type (2 for Transfer) + amount (8 bytes)
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[0:4], 2) // Transfer instruction
	binary.LittleEndian.PutUint64(data[4:12], amount)

	return data
}

// Implement remaining TraderInterface methods by delegating to base trader

func (jt *JitoTrader) Stop() {
	jt.trader.Stop()
}

func (jt *JitoTrader) GetTradingStats() map[string]interface{} {
	stats := jt.trader.GetTradingStats()
	stats["jito_enabled"] = jt.enabled
	stats["jito_tip_amount"] = jt.config.JITO.TipAmount
	return stats
}

func (jt *JitoTrader) GetTraderType() string {
	baseType := jt.trader.GetTraderType()
	if jt.enabled {
		return baseType + "_jito_protected"
	}
	return baseType
}

// JitoExtremeFastTrader combines extreme fast mode with Jito protection
type JitoExtremeFastTrader struct {
	*JitoTrader
	extremeFastTrader *ExtremeFastTrader
}

// NewJitoExtremeFastTrader creates Jito-protected extreme fast trader
func NewJitoExtremeFastTrader(
	extremeFastTrader *ExtremeFastTrader,
	jitoClient *solana.JitoClient,
	wallet *wallet.Wallet,
	logger *logger.Logger,
	config *config.Config,
) *JitoExtremeFastTrader {
	jitoTrader := NewJitoTrader(extremeFastTrader, jitoClient, wallet, logger, config)

	return &JitoExtremeFastTrader{
		JitoTrader:        jitoTrader,
		extremeFastTrader: extremeFastTrader,
	}
}

// BuyToken uses extreme fast mode with optional Jito protection
func (jeft *JitoExtremeFastTrader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	// Check if we should use Jito protection for extreme fast trades
	if jeft.enabled && jeft.config.ExtremeFast.UseJito {
		jeft.logger.WithField("mint", tokenEvent.Mint.String()).Info("⚡🛡️ Using Extreme Fast + Jito protection")
		return jeft.JitoTrader.BuyToken(ctx, tokenEvent)
	}

	// Use regular extreme fast mode
	return jeft.extremeFastTrader.BuyToken(ctx, tokenEvent)
}

func (jeft *JitoExtremeFastTrader) GetTraderType() string {
	if jeft.enabled && jeft.config.ExtremeFast.UseJito {
		return "extreme_fast_jito_protected"
	}
	return "extreme_fast"
}

func (jeft *JitoExtremeFastTrader) GetTradingStats() map[string]interface{} {
	stats := jeft.extremeFastTrader.GetTradingStats()
	stats["jito_enabled"] = jeft.enabled
	stats["jito_for_extreme_fast"] = jeft.config.ExtremeFast.UseJito
	return stats
}
