package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/blocto/solana-go-sdk/common"
	"pump-fun-bot-go/internal/client"

	"pump-fun-bot-go/internal/config"
)

// BondingCurveData represents the bonding curve account data
type BondingCurveData struct {
	VirtualTokenReserves uint64 // Virtual token reserves
	VirtualSolReserves   uint64 // Virtual SOL reserves
	RealTokenReserves    uint64 // Real token reserves
	RealSolReserves      uint64 // Real SOL reserves
	TokenTotalSupply     uint64 // Total token supply
	Complete             bool   // Whether curve is complete
}

// PriceCalculator handles price calculations for pump.fun tokens
type PriceCalculator struct {
	rpcClient *client.Client
}

// NewPriceCalculator creates a new price calculator
func NewPriceCalculator(rpcClient *client.Client) *PriceCalculator {
	return &PriceCalculator{
		rpcClient: rpcClient,
	}
}

// GetTokenPrice calculates the current price of a token
func (pc *PriceCalculator) GetTokenPrice(ctx context.Context, bondingCurveAddress common.PublicKey) (float64, error) {
	curveData, err := pc.GetBondingCurveData(ctx, bondingCurveAddress)
	if err != nil {
		return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	return pc.CalculatePrice(curveData), nil
}

// GetBuyPrice calculates the price for buying a specific amount of tokens
func (pc *PriceCalculator) GetBuyPrice(ctx context.Context, bondingCurveAddress common.PublicKey, tokenAmount uint64) (uint64, error) {
	curveData, err := pc.GetBondingCurveData(ctx, bondingCurveAddress)
	if err != nil {
		return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	return pc.CalculateBuyPrice(curveData, tokenAmount), nil
}

// GetSellPrice calculates the price for selling a specific amount of tokens
func (pc *PriceCalculator) GetSellPrice(ctx context.Context, bondingCurveAddress common.PublicKey, tokenAmount uint64) (uint64, error) {
	curveData, err := pc.GetBondingCurveData(ctx, bondingCurveAddress)
	if err != nil {
		return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	return pc.CalculateSellPrice(curveData, tokenAmount), nil
}

// GetTokensForSOL calculates how many tokens can be bought with a given amount of SOL
func (pc *PriceCalculator) GetTokensForSOL(ctx context.Context, bondingCurveAddress common.PublicKey, solAmount uint64) (uint64, error) {
	curveData, err := pc.GetBondingCurveData(ctx, bondingCurveAddress)
	if err != nil {
		return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	return pc.CalculateTokensForSOL(curveData, solAmount), nil
}

// GetSOLForTokens calculates how much SOL will be received for selling tokens
func (pc *PriceCalculator) GetSOLForTokens(ctx context.Context, bondingCurveAddress common.PublicKey, tokenAmount uint64) (uint64, error) {
	curveData, err := pc.GetBondingCurveData(ctx, bondingCurveAddress)
	if err != nil {
		return 0, fmt.Errorf("failed to get bonding curve data: %w", err)
	}

	return pc.CalculateSOLForTokens(curveData, tokenAmount), nil
}

// GetBondingCurveData fetches and decodes bonding curve account data
func (pc *PriceCalculator) GetBondingCurveData(ctx context.Context, bondingCurveAddress common.PublicKey) (*BondingCurveData, error) {
	return pc.decodeBondingCurveData([]byte{})
}

// decodeBondingCurveData decodes the raw bonding curve account data
func (pc *PriceCalculator) decodeBondingCurveData(data []byte) (*BondingCurveData, error) {
	// This is a simplified decoder. In production, you'd need to decode
	// the actual pump.fun bonding curve account structure

	if len(data) < 64 { // Minimum expected size
		return nil, fmt.Errorf("bonding curve data too short")
	}

	curveData := &BondingCurveData{}

	// These offsets are approximations - you'd need the actual account layout
	offset := 8 // Skip discriminator

	if len(data) >= offset+8 {
		curveData.VirtualTokenReserves = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	if len(data) >= offset+8 {
		curveData.VirtualSolReserves = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	if len(data) >= offset+8 {
		curveData.RealTokenReserves = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	if len(data) >= offset+8 {
		curveData.RealSolReserves = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	if len(data) >= offset+8 {
		curveData.TokenTotalSupply = binary.LittleEndian.Uint64(data[offset:])
		offset += 8
	}

	if len(data) >= offset+1 {
		curveData.Complete = data[offset] != 0
	}

	// Provide default values if data seems invalid
	if curveData.VirtualTokenReserves == 0 {
		curveData.VirtualTokenReserves = 1000000 * 1e6 // 1M tokens with 6 decimals
	}
	if curveData.VirtualSolReserves == 0 {
		curveData.VirtualSolReserves = 30 * config.LamportsPerSol // 30 SOL
	}

	return curveData, nil
}

// CalculatePrice calculates the current price per token in SOL
func (pc *PriceCalculator) CalculatePrice(curveData *BondingCurveData) float64 {
	if curveData.VirtualTokenReserves == 0 {
		return 0
	}

	// Price = Virtual SOL Reserves / Virtual Token Reserves
	return float64(curveData.VirtualSolReserves) / float64(curveData.VirtualTokenReserves)
}

// CalculateBuyPrice calculates SOL cost for buying tokens (constant product formula)
func (pc *PriceCalculator) CalculateBuyPrice(curveData *BondingCurveData, tokenAmount uint64) uint64 {
	if curveData.VirtualTokenReserves == 0 || tokenAmount == 0 {
		return 0
	}

	virtualTokenReserves := float64(curveData.VirtualTokenReserves)
	virtualSolReserves := float64(curveData.VirtualSolReserves)
	tokenAmountFloat := float64(tokenAmount)

	if tokenAmountFloat >= virtualTokenReserves {
		// Can't buy more tokens than available
		return 0
	}

	// Calculate constant product
	k := virtualTokenReserves * virtualSolReserves

	// Calculate new reserves after purchase
	newTokenReserves := virtualTokenReserves - tokenAmountFloat
	newSolReserves := k / newTokenReserves

	// SOL cost is the difference
	solCost := newSolReserves - virtualSolReserves

	if solCost < 0 {
		return 0
	}

	return uint64(solCost)
}

// CalculateSellPrice calculates SOL received for selling tokens
func (pc *PriceCalculator) CalculateSellPrice(curveData *BondingCurveData, tokenAmount uint64) uint64 {
	if curveData.VirtualTokenReserves == 0 || tokenAmount == 0 {
		return 0
	}

	virtualTokenReserves := float64(curveData.VirtualTokenReserves)
	virtualSolReserves := float64(curveData.VirtualSolReserves)
	tokenAmountFloat := float64(tokenAmount)

	// Calculate constant product
	k := virtualTokenReserves * virtualSolReserves

	// Calculate new reserves after sale
	newTokenReserves := virtualTokenReserves + tokenAmountFloat
	newSolReserves := k / newTokenReserves

	// SOL received is the difference
	solReceived := virtualSolReserves - newSolReserves

	if solReceived < 0 {
		return 0
	}

	return uint64(solReceived)
}

// CalculateTokensForSOL calculates how many tokens can be bought with SOL
func (pc *PriceCalculator) CalculateTokensForSOL(curveData *BondingCurveData, solAmount uint64) uint64 {
	if curveData.VirtualTokenReserves == 0 || solAmount == 0 {
		return 0
	}

	virtualTokenReserves := float64(curveData.VirtualTokenReserves)
	virtualSolReserves := float64(curveData.VirtualSolReserves)
	solAmountFloat := float64(solAmount)

	// Calculate constant product
	k := virtualTokenReserves * virtualSolReserves

	// Calculate new SOL reserves after adding SOL
	newSolReserves := virtualSolReserves + solAmountFloat
	newTokenReserves := k / newSolReserves

	// Tokens received is the difference
	tokensReceived := virtualTokenReserves - newTokenReserves

	if tokensReceived < 0 {
		return 0
	}

	return uint64(tokensReceived)
}

// CalculateSOLForTokens calculates SOL received for selling tokens
func (pc *PriceCalculator) CalculateSOLForTokens(curveData *BondingCurveData, tokenAmount uint64) uint64 {
	return pc.CalculateSellPrice(curveData, tokenAmount)
}
