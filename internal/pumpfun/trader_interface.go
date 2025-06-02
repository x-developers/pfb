// internal/pumpfun/trader_interface.go
package pumpfun

import (
	"context"
)

// TraderInterface defines the common interface for all trader implementations
type TraderInterface interface {
	// ShouldBuyToken determines if we should buy a token
	ShouldBuyToken(ctx context.Context, tokenEvent *TokenEvent) (bool, string)

	// BuyToken executes a buy transaction
	BuyToken(ctx context.Context, tokenEvent *TokenEvent) (*TradeResult, error)

	// Stop stops the trader
	Stop()

	// GetTradingStats returns trading statistics
	GetTradingStats() map[string]interface{}

	// GetTraderType returns the type of trader ("normal", "extreme_fast")
	GetTraderType() string
}

// TradeResult represents the result of a trade (already defined, keeping for reference)
type TradeResult struct {
	Success      bool
	Signature    string
	AmountSOL    float64
	AmountTokens uint64
	Price        float64
	Error        string
	TradeTime    int64 // Add trade time in milliseconds
}
