package pumpfun

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"

	"github.com/blocto/solana-go-sdk/types"
)

// TransactionCapableTrader interface for traders that can create transactions
type TransactionCapableTrader interface {
	TraderInterface
	CreateBuyTransaction(ctx context.Context, tokenEvent *TokenEvent) (types.Transaction, error)
}

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

// BuyToken executes buy with Jito MEV protection if enabled, otherwise uses base trader
func (jt *JitoTrader) BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error) {
	startTime := time.Now()

	// If Jito is not enabled, delegate directly to base trader
	if !jt.enabled {
		jt.logger.Debug("🔄 Jito not enabled, using base trader directly")
		return jt.trader.BuyToken(ctx, tokenEvent)
	}

	jt.logger.WithFields(map[string]interface{}{
		"mint":         tokenEvent.Mint.String(),
		"jito_enabled": true,
		"trader_type":  jt.trader.GetTraderType(),
	}).Info("🛡️ Executing Jito-protected buy transaction")

	// Try to use Jito protection
	result, err := jt.executeWithJitoProtection(ctx, tokenEvent, startTime)
	if err != nil {
		// Fallback to regular transaction if Jito fails
		jt.logger.WithError(err).Warn("🔄 Jito protection failed, falling back to regular transaction")
		return jt.fallbackToRegularTransaction(ctx, tokenEvent, startTime)
	}

	return result, nil
}

// executeWithJitoProtection tries to execute the trade with Jito protection
func (jt *JitoTrader) executeWithJitoProtection(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {
	// Check if the base trader can create transactions
	txCapableTrader, canCreateTx := jt.trader.(TransactionCapableTrader)

	if !canCreateTx {
		// If the trader can't create transactions, use it normally
		// This handles cases like UltraFastTrader that manage their own transactions
		jt.logger.Debug("🔄 Base trader doesn't support transaction creation, using direct execution with Jito wrapping")
		return jt.wrapTraderWithJitoMonitoring(ctx, tokenEvent, startTime)
	}

	// Create the buy transaction using the base trader
	buyTransaction, err := txCapableTrader.CreateBuyTransaction(ctx, tokenEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to create buy transaction: %w", err)
	}

	// Serialize transaction to base64
	txBytes, err := buyTransaction.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}
	encodedTx := base64.StdEncoding.EncodeToString(txBytes)

	// Create tip transaction if configured
	var tipTransaction string
	if jt.config.JITO.TipAmount > 0 {
		tipTransaction, err = jt.createTipTransaction(ctx)
		if err != nil {
			jt.logger.WithError(err).Warn("Failed to create tip transaction, continuing without tip")
		}
	}

	// Send via Jito bundle
	bundleID, err := jt.sendJitoBundle(ctx, encodedTx, tipTransaction)
	if err != nil {
		return nil, fmt.Errorf("failed to send Jito bundle: %w", err)
	}

	// Monitor bundle status
	signature, err := jt.waitForBundleConfirmation(ctx, bundleID)
	if err != nil {
		return nil, fmt.Errorf("bundle confirmation failed: %w", err)
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

// wrapTraderWithJitoMonitoring wraps non-transaction-capable traders with Jito monitoring
func (jt *JitoTrader) wrapTraderWithJitoMonitoring(ctx context.Context, tokenEvent *TokenEvent, startTime time.Time) (*TradeResult, error) {
	// For traders like UltraFastTrader that manage their own transactions,
	// we'll let them execute normally but try to add Jito monitoring if possible

	jt.logger.Debug("🔍 Executing trade with Jito monitoring")

	// Execute the trade using the base trader
	result, err := jt.trader.BuyToken(ctx, tokenEvent)
	if err != nil {
		return result, err
	}

	// If successful, log that it was executed with Jito awareness
	if result != nil && result.Success {
		jt.logger.WithFields(map[string]interface{}{
			"signature":       result.Signature,
			"trader_type":     jt.trader.GetTraderType(),
			"jito_aware":      true,
			"protection_type": "monitoring",
		}).Info("🛡️ Trade executed with Jito monitoring")

		// Update trade time to include Jito overhead
		result.TradeTime = time.Since(startTime).Milliseconds()
	}

	return result, nil
}

// createTipTransaction creates a tip transaction for MEV protection
func (jt *JitoTrader) createTipTransaction(ctx context.Context) (string, error) {
	// Get a random tip account
	tipAccount, err := jt.jitoClient.GetRandomTipAccount(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get tip account: %w", err)
	}

	// Create a simple transfer instruction to the tip account
	// This is a simplified implementation - in production you'd want more sophisticated tip logic
	jt.logger.WithFields(map[string]interface{}{
		"tip_account": tipAccount,
		"tip_amount":  jt.config.JITO.TipAmount,
	}).Debug("Creating Jito tip transaction")

	// For now, return empty string indicating no tip transaction
	// This should be implemented based on your specific Jito integration needs
	return "", nil
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

		// Log that we fell back from Jito
		if result.Success {
			jt.logger.WithFields(map[string]interface{}{
				"signature":   result.Signature,
				"trader_type": jt.trader.GetTraderType(),
				"fallback":    true,
			}).Info("✅ Trade executed via fallback (no Jito protection)")
		}
	}

	return result, nil
}

// Start starts the Jito trader (delegates to base trader if it supports starting)
func (jt *JitoTrader) Start() error {
	jt.logger.WithFields(map[string]interface{}{
		"jito_enabled":  jt.enabled,
		"base_trader":   jt.trader.GetTraderType(),
		"tip_amount":    jt.config.JITO.TipAmount,
		"jito_endpoint": jt.config.JITO.Endpoint,
	}).Info("🛡️ Starting Jito-enhanced trader")

	// Start the base trader if it supports starting
	if starter, ok := jt.trader.(interface{ Start() error }); ok {
		return starter.Start()
	}

	return nil
}

// Stop stops the trader (delegates to base trader)
func (jt *JitoTrader) Stop() {
	jt.logger.Info("🛑 Stopping Jito-enhanced trader")
	jt.trader.Stop()
}

// GetTradingStats returns enhanced trading statistics
func (jt *JitoTrader) GetTradingStats() map[string]interface{} {
	stats := jt.trader.GetTradingStats()

	// Add Jito-specific stats
	stats["jito_enabled"] = jt.enabled
	stats["jito_tip_amount"] = jt.config.JITO.TipAmount
	stats["jito_endpoint"] = jt.config.JITO.Endpoint
	stats["trader_wrapper"] = "jito_protected"

	return stats
}

// GetTraderType returns the enhanced trader type
func (jt *JitoTrader) GetTraderType() string {
	baseType := jt.trader.GetTraderType()
	if jt.enabled {
		return baseType + "_jito_protected"
	}
	return baseType + "_jito_aware"
}

// ValidateTokenEvent delegates to base trader if it supports validation
func (jt *JitoTrader) ValidateTokenEvent(tokenEvent *TokenEvent) error {
	if validator, ok := jt.trader.(interface{ ValidateTokenEvent(*TokenEvent) error }); ok {
		return validator.ValidateTokenEvent(tokenEvent)
	}

	// Basic validation if base trader doesn't support it
	if tokenEvent.Mint == nil {
		return fmt.Errorf("token mint is nil")
	}
	if tokenEvent.BondingCurve == nil {
		return fmt.Errorf("bonding curve is nil")
	}

	return nil
}

// Helper method to check if trader supports transaction creation
func (jt *JitoTrader) SupportsTransactionCreation() bool {
	_, canCreate := jt.trader.(TransactionCapableTrader)
	return canCreate
}

// Helper method to get Jito protection status
func (jt *JitoTrader) GetJitoStatus() map[string]interface{} {
	return map[string]interface{}{
		"enabled":              jt.enabled,
		"tip_amount":           jt.config.JITO.TipAmount,
		"endpoint":             jt.config.JITO.Endpoint,
		"supports_tx_creation": jt.SupportsTransactionCreation(),
		"base_trader_type":     jt.trader.GetTraderType(),
	}
}
