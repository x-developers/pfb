// internal/pumpfun/curve_manager.go
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"

	"github.com/gagliardetto/solana-go"
	"github.com/sirupsen/logrus"
)

// BondingCurveState represents the state of a bonding curve
type BondingCurveState struct {
	VirtualTokenReserves uint64 `json:"virtual_token_reserves"`
	VirtualSolReserves   uint64 `json:"virtual_sol_reserves"`
	RealTokenReserves    uint64 `json:"real_token_reserves"`
	RealSolReserves      uint64 `json:"real_sol_reserves"`
	TokenTotalSupply     uint64 `json:"token_total_supply"`
	Complete             bool   `json:"complete"`
}

// CurveManager handles all bonding curve operations
type CurveManager struct {
	rpcClient *client.Client
	logger    *logger.Logger

	// Constants based on pump.fun implementation
	virtualTokenReserves uint64
	virtualSolReserves   uint64
	feeRateBasisPoints   uint64
}

// CurveCalculationResult represents the result of curve calculations
type CurveCalculationResult struct {
	AmountOut            uint64  `json:"amount_out"`
	Fee                  uint64  `json:"fee"`
	PricePerToken        float64 `json:"price_per_token"`
	PriceImpact          float64 `json:"price_impact"`
	SlippageToleranceMet bool    `json:"slippage_tolerance_met"`
}

// NewCurveManager creates a new curve manager instance
func NewCurveManager(rpcClient *client.Client, logger *logger.Logger) *CurveManager {
	return &CurveManager{
		rpcClient: rpcClient,
		logger:    logger,
		// Default pump.fun constants
		virtualTokenReserves: 1073000000000000, // 1.073B tokens
		virtualSolReserves:   30000000000,      // 30 SOL in lamports
		feeRateBasisPoints:   100,              // 1% fee
	}
}

// GetBondingCurveState fetches the current state of a bonding curve
func (cm *CurveManager) GetBondingCurveState(ctx context.Context, bondingCurveAddress solana.PublicKey) (*BondingCurveState, error) {
	accountInfo, err := cm.rpcClient.GetAccountInfo(ctx, bondingCurveAddress.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get bonding curve account: %w", err)
	}

	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	data := accountInfo.Value.Data.GetBinary()
	if len(data) < 64 {
		return nil, fmt.Errorf("invalid bonding curve data length: %d", len(data))
	}

	state := &BondingCurveState{}

	// Skip 8-byte discriminator
	offset := 8

	state.VirtualTokenReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	state.VirtualSolReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	state.RealTokenReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	state.RealSolReserves = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	state.TokenTotalSupply = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	if len(data) > offset {
		state.Complete = data[offset] != 0
	}

	cm.logger.WithFields(logrus.Fields{}).Debug("Retrieved bonding curve state")

	return state, nil
}

// CalculateSellReturn calculates how much SOL will be received for selling tokens
func (cm *CurveManager) CalculateSellReturn(
	ctx context.Context,
	bondingCurveAddress solana.PublicKey,
	tokenAmount uint64,
) (*CurveCalculationResult, error) {
	state, err := cm.GetBondingCurveState(ctx, bondingCurveAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get curve state: %w", err)
	}

	if state.Complete {
		return nil, fmt.Errorf("bonding curve is complete, cannot sell through curve")
	}

	return cm.calculateSellReturnFromState(state, tokenAmount), nil
}

// calculateSellReturnFromState calculates sell return from existing state
func (cm *CurveManager) calculateSellReturnFromState(state *BondingCurveState, tokenAmount uint64) *CurveCalculationResult {
	if tokenAmount == 0 {
		return &CurveCalculationResult{}
	}

	// Use virtual reserves for calculation
	virtualTokenReserves := float64(state.VirtualTokenReserves)
	virtualSolReserves := float64(state.VirtualSolReserves)
	tokenAmountFloat := float64(tokenAmount)

	// Calculate price before trade
	priceBefore := virtualSolReserves / virtualTokenReserves

	// Calculate constant product (k = x * y)
	k := virtualTokenReserves * virtualSolReserves

	// After selling tokens, token reserves increase
	newTokenReserves := virtualTokenReserves + tokenAmountFloat

	// New SOL reserves based on constant product
	newSolReserves := k / newTokenReserves

	// SOL to be received (before fees)
	solReceived := virtualSolReserves - newSolReserves

	if solReceived <= 0 {
		return &CurveCalculationResult{
			AmountOut:            0,
			Fee:                  0,
			PricePerToken:        priceBefore,
			PriceImpact:          0,
			SlippageToleranceMet: false,
		}
	}

	// Calculate fee (typically 1% on pump.fun)
	fee := solReceived * float64(cm.feeRateBasisPoints) / 10000
	solAfterFee := solReceived - fee

	// Calculate price after trade
	priceAfter := newSolReserves / newTokenReserves

	// Calculate price impact
	priceImpact := math.Abs((priceAfter-priceBefore)/priceBefore) * 100

	// Calculate effective price per token
	effectivePricePerToken := solAfterFee / tokenAmountFloat

	result := &CurveCalculationResult{
		AmountOut:            uint64(solAfterFee),
		Fee:                  uint64(fee),
		PricePerToken:        effectivePricePerToken,
		PriceImpact:          priceImpact,
		SlippageToleranceMet: true, // Will be checked against tolerance later
	}

	cm.logger.WithFields(logrus.Fields{
		"token_amount":         tokenAmount,
		"sol_received_gross":   uint64(solReceived),
		"fee":                  uint64(fee),
		"sol_received_net":     uint64(solAfterFee),
		"price_per_token":      effectivePricePerToken,
		"price_impact_percent": priceImpact,
		"price_before":         priceBefore,
		"price_after":          priceAfter,
	}).Debug("Calculated sell return")

	return result
}

// CalculateBuyAmount calculates how many tokens can be bought with SOL
func (cm *CurveManager) CalculateBuyAmount(
	ctx context.Context,
	bondingCurveAddress solana.PublicKey,
	solAmount uint64,
) (*CurveCalculationResult, error) {
	state, err := cm.GetBondingCurveState(ctx, bondingCurveAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get curve state: %w", err)
	}

	if state.Complete {
		return nil, fmt.Errorf("bonding curve is complete, cannot buy through curve")
	}

	return cm.calculateBuyAmountFromState(state, solAmount), nil
}

// calculateBuyAmountFromState calculates buy amount from existing state
func (cm *CurveManager) calculateBuyAmountFromState(state *BondingCurveState, solAmount uint64) *CurveCalculationResult {
	if solAmount == 0 {
		return &CurveCalculationResult{}
	}

	// Use virtual reserves for calculation
	virtualTokenReserves := float64(state.VirtualTokenReserves)
	virtualSolReserves := float64(state.VirtualSolReserves)
	solAmountFloat := float64(solAmount)

	// Calculate fee first (SOL amount after fee)
	fee := solAmountFloat * float64(cm.feeRateBasisPoints) / 10000
	solAfterFee := solAmountFloat - fee

	// Calculate price before trade
	priceBefore := virtualSolReserves / virtualTokenReserves

	// Calculate constant product (k = x * y)
	k := virtualTokenReserves * virtualSolReserves

	// After buying, SOL reserves increase by the amount after fee
	newSolReserves := virtualSolReserves + solAfterFee

	// New token reserves based on constant product
	newTokenReserves := k / newSolReserves

	// Tokens to be received
	tokensReceived := virtualTokenReserves - newTokenReserves

	if tokensReceived <= 0 {
		return &CurveCalculationResult{
			AmountOut:            0,
			Fee:                  uint64(fee),
			PricePerToken:        priceBefore,
			PriceImpact:          0,
			SlippageToleranceMet: false,
		}
	}

	// Calculate price after trade
	priceAfter := newSolReserves / newTokenReserves

	// Calculate price impact
	priceImpact := math.Abs((priceAfter-priceBefore)/priceBefore) * 100

	// Calculate effective price per token
	effectivePricePerToken := solAmountFloat / tokensReceived

	result := &CurveCalculationResult{
		AmountOut:            uint64(tokensReceived),
		Fee:                  uint64(fee),
		PricePerToken:        effectivePricePerToken,
		PriceImpact:          priceImpact,
		SlippageToleranceMet: true,
	}

	cm.logger.WithFields(logrus.Fields{
		"sol_amount":           solAmount,
		"fee":                  uint64(fee),
		"sol_after_fee":        uint64(solAfterFee),
		"tokens_received":      uint64(tokensReceived),
		"price_per_token":      effectivePricePerToken,
		"price_impact_percent": priceImpact,
		"price_before":         priceBefore,
		"price_after":          priceAfter,
	}).Debug("Calculated buy amount")

	return result
}

// CheckSlippageTolerance checks if the calculated result meets slippage tolerance
func (cm *CurveManager) CheckSlippageTolerance(
	result *CurveCalculationResult,
	expectedAmountOut uint64,
	slippageToleranceBP int,
) bool {
	if expectedAmountOut == 0 {
		return true
	}

	// Calculate minimum acceptable amount (accounting for slippage)
	slippageFactor := float64(slippageToleranceBP) / 10000
	minAcceptableAmount := float64(expectedAmountOut) * (1 - slippageFactor)

	result.SlippageToleranceMet = float64(result.AmountOut) >= minAcceptableAmount

	cm.logger.WithFields(logrus.Fields{
		"expected_amount":       expectedAmountOut,
		"actual_amount":         result.AmountOut,
		"min_acceptable":        uint64(minAcceptableAmount),
		"slippage_tolerance_bp": slippageToleranceBP,
		"tolerance_met":         result.SlippageToleranceMet,
	}).Debug("Checked slippage tolerance")

	return result.SlippageToleranceMet
}

// GetCurrentPrice returns the current price per token
func (cm *CurveManager) GetCurrentPrice(ctx context.Context, bondingCurveAddress solana.PublicKey) (float64, error) {
	state, err := cm.GetBondingCurveState(ctx, bondingCurveAddress)
	if err != nil {
		return 0, fmt.Errorf("failed to get curve state: %w", err)
	}

	if state.VirtualTokenReserves == 0 {
		return 0, fmt.Errorf("invalid token reserves")
	}

	price := float64(state.VirtualSolReserves) / float64(state.VirtualTokenReserves)

	cm.logger.WithFields(logrus.Fields{
		"bonding_curve": bondingCurveAddress.String(),
		"price":         price,
		"price_sol":     price / config.LamportsPerSol,
	}).Debug("Current token price")

	return price, nil
}

// EstimateOptimalSellAmount estimates the optimal amount to sell based on available balance
func (cm *CurveManager) EstimateOptimalSellAmount(
	ctx context.Context,
	bondingCurveAddress solana.PublicKey,
	tokenBalance uint64,
	sellPercentage float64,
) (uint64, *CurveCalculationResult, error) {
	if sellPercentage <= 0 || sellPercentage > 100 {
		return 0, nil, fmt.Errorf("invalid sell percentage: %.2f", sellPercentage)
	}

	// Calculate amount to sell based on percentage
	sellAmount := uint64(float64(tokenBalance) * (sellPercentage / 100.0))

	if sellAmount == 0 {
		return 0, nil, fmt.Errorf("calculated sell amount is zero")
	}

	// Get sell calculation
	result, err := cm.CalculateSellReturn(ctx, bondingCurveAddress, sellAmount)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to calculate sell return: %w", err)
	}

	cm.logger.WithFields(logrus.Fields{
		"token_balance":   tokenBalance,
		"sell_percentage": sellPercentage,
		"sell_amount":     sellAmount,
		"estimated_sol":   result.AmountOut,
		"fee":             result.Fee,
		"price_impact":    result.PriceImpact,
	}).Info("Estimated optimal sell amount")

	return sellAmount, result, nil
}

// GetMarketStats returns current market statistics for the token
func (cm *CurveManager) GetMarketStats(ctx context.Context, bondingCurveAddress solana.PublicKey) (map[string]interface{}, error) {
	state, err := cm.GetBondingCurveState(ctx, bondingCurveAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get curve state: %w", err)
	}

	currentPrice := float64(0)
	if state.VirtualTokenReserves > 0 {
		currentPrice = float64(state.VirtualSolReserves) / float64(state.VirtualTokenReserves)
	}

	marketCap := currentPrice * float64(state.TokenTotalSupply)
	liquidity := float64(state.RealSolReserves)

	stats := map[string]interface{}{
		"current_price_lamports": currentPrice,
		"current_price_sol":      currentPrice / config.LamportsPerSol,
		"market_cap_lamports":    uint64(marketCap),
		"market_cap_sol":         marketCap / config.LamportsPerSol,
		"liquidity_lamports":     state.RealSolReserves,
		"liquidity_sol":          liquidity / config.LamportsPerSol,
		"virtual_token_reserves": state.VirtualTokenReserves,
		"virtual_sol_reserves":   state.VirtualSolReserves,
		"real_token_reserves":    state.RealTokenReserves,
		"real_sol_reserves":      state.RealSolReserves,
		"token_total_supply":     state.TokenTotalSupply,
		"curve_complete":         state.Complete,
		"curve_progress_percent": float64(state.RealSolReserves) / float64(state.VirtualSolReserves) * 100,
	}

	cm.logger.WithFields(stats).Debug("Market statistics")

	return stats, nil
}

// SimulateSellImpact simulates the impact of different sell amounts
func (cm *CurveManager) SimulateSellImpact(
	ctx context.Context,
	bondingCurveAddress solana.PublicKey,
	tokenBalance uint64,
) (map[string]*CurveCalculationResult, error) {
	sellPercentages := []float64{10, 25, 50, 75, 100}
	results := make(map[string]*CurveCalculationResult)

	state, err := cm.GetBondingCurveState(ctx, bondingCurveAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get curve state: %w", err)
	}

	for _, percentage := range sellPercentages {
		sellAmount := uint64(float64(tokenBalance) * (percentage / 100.0))
		if sellAmount > 0 {
			result := cm.calculateSellReturnFromState(state, sellAmount)
			results[fmt.Sprintf("%.0f%%", percentage)] = result
		}
	}

	cm.logger.WithFields(logrus.Fields{
		"token_balance":     tokenBalance,
		"simulations_count": len(results),
	}).Debug("Simulated sell impact for different percentages")

	return results, nil
}

// ValidateSellTransaction validates a sell transaction before execution
func (cm *CurveManager) ValidateSellTransaction(
	ctx context.Context,
	bondingCurveAddress solana.PublicKey,
	tokenAmount uint64,
	minSolOutput uint64,
	slippageToleranceBP int,
) error {
	// Get current calculation
	result, err := cm.CalculateSellReturn(ctx, bondingCurveAddress, tokenAmount)
	if err != nil {
		return fmt.Errorf("failed to calculate sell return: %w", err)
	}

	// Check if we'll receive enough SOL
	if result.AmountOut < minSolOutput {
		return fmt.Errorf("insufficient SOL output: expected %d, calculated %d",
			minSolOutput, result.AmountOut)
	}

	// Check slippage tolerance
	if !cm.CheckSlippageTolerance(result, minSolOutput, slippageToleranceBP) {
		return fmt.Errorf("slippage tolerance exceeded: %.2f%% impact, max allowed: %.2f%%",
			result.PriceImpact, float64(slippageToleranceBP)/100)
	}

	// Check for excessive price impact
	if result.PriceImpact > 10.0 { // 10% price impact warning
		cm.logger.WithField("price_impact", result.PriceImpact).
			Warn("High price impact detected for sell transaction")
	}

	cm.logger.WithFields(logrus.Fields{
		"token_amount":   tokenAmount,
		"min_sol_output": minSolOutput,
		"calculated_sol": result.AmountOut,
		"price_impact":   result.PriceImpact,
		"validation":     "passed",
	}).Debug("Sell transaction validation passed")

	return nil
}
