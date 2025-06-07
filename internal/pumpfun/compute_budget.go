package pumpfun

import (
	"encoding/binary"
	"github.com/gagliardetto/solana-go"
	"pump-fun-bot-go/internal/config"
)

// ComputeBudgetInstruction types
const (
	RequestUnitsInstruction        uint8 = 0 // Deprecated
	RequestHeapFrameInstruction    uint8 = 1
	SetComputeUnitLimitInstruction uint8 = 2
	SetComputeUnitPriceInstruction uint8 = 3
)

// ComputeBudgetConfig contains compute budget settings
type ComputeBudgetConfig struct {
	ComputeUnitLimit uint32 // Maximum compute units for the transaction
	ComputeUnitPrice uint64 // Priority fee in micro-lamports per compute unit
}

// CreateSetComputeUnitLimitInstruction creates a compute unit limit instruction
func CreateSetComputeUnitLimitInstruction(units uint32) solana.Instruction {
	data := make([]byte, 5) // 1 byte instruction + 4 bytes for units
	data[0] = SetComputeUnitLimitInstruction
	binary.LittleEndian.PutUint32(data[1:5], units)

	return solana.NewInstruction(
		solana.PublicKeyFromBytes(config.ComputeBudgetProgramID),
		[]*solana.AccountMeta{}, // No accounts required
		data,
	)
}

// CreateSetComputeUnitPriceInstruction creates a compute unit price instruction for priority fees
func CreateSetComputeUnitPriceInstruction(microLamports uint64) solana.Instruction {
	data := make([]byte, 9) // 1 byte instruction + 8 bytes for price
	data[0] = SetComputeUnitPriceInstruction
	binary.LittleEndian.PutUint64(data[1:9], microLamports)

	return solana.NewInstruction(
		solana.PublicKeyFromBytes(config.ComputeBudgetProgramID),
		[]*solana.AccountMeta{}, // No accounts required
		data,
	)
}

// CalculatePriorityFee calculates priority fee based on config and network conditions
func CalculatePriorityFee(cfg *config.Config) uint64 {
	baseFee := cfg.Trading.PriorityFee

	// Apply multipliers based on trading mode
	if cfg.UltraFast.Enabled {
		if cfg.UltraFast.SkipValidation {
			// Ultra-fast mode with skip validation - higher priority
			return baseFee * 2
		}
		// Standard ultra-fast mode
		return baseFee + (baseFee / 2) // 1.5x
	}

	// YOLO mode gets higher priority
	if cfg.Strategy.YoloMode {
		return baseFee + (baseFee / 4) // 1.25x
	}

	// Token-based trading gets slight priority boost
	if cfg.IsTokenBasedTrading() {
		return baseFee + (baseFee / 10) // 1.1x
	}

	return baseFee
}

// GetRecommendedComputeUnitLimit returns recommended compute unit limit based on operation type
func GetRecommendedComputeUnitLimit(operationType string) uint32 {
	switch operationType {
	case "buy":
		return 250000 // Standard buy operation
	case "sell":
		return 180000 // Sell operations typically use less
	default:
		return 200000 // Default
	}
}

// EstimateTransactionFee estimates the total transaction fee including priority fee
func EstimateTransactionFee(priorityFee uint64, computeUnits uint32) uint64 {
	// Base transaction fee (standard Solana fee)
	baseFee := uint64(5000) // 5000 lamports

	// Priority fee calculation: priority_fee * compute_units / 1_000_000
	priorityFeeTotal := priorityFee * uint64(computeUnits) / 1_000_000

	return baseFee + priorityFeeTotal
}

// ComputeBudgetHelper provides utility functions for compute budget management
type ComputeBudgetHelper struct {
	config *config.Config
}

// NewComputeBudgetHelper creates a new compute budget helper
func NewComputeBudgetHelper(cfg *config.Config) *ComputeBudgetHelper {
	return &ComputeBudgetHelper{
		config: cfg,
	}
}

// GetInstructionsForOperation returns appropriate compute budget instructions for a specific operation
func (cbh *ComputeBudgetHelper) GetInstructionsForOperation(operationType string, customPriorityFee ...uint64) []solana.Instruction {
	// Determine priority fee
	priorityFee := cbh.config.Trading.PriorityFee
	if len(customPriorityFee) > 0 && customPriorityFee[0] > 0 {
		priorityFee = customPriorityFee[0]
	}

	// Get compute unit limit for operation
	computeUnitLimit := GetRecommendedComputeUnitLimit(operationType)

	var instructions []solana.Instruction

	// Always add compute unit limit for better success rates
	instructions = append(instructions, CreateSetComputeUnitLimitInstruction(computeUnitLimit))

	// Add priority fee if specified
	if priorityFee > 0 {
		instructions = append(instructions, CreateSetComputeUnitPriceInstruction(priorityFee))
	}

	return instructions
}

// GetEstimatedFee returns estimated transaction fee for an operation
func (cbh *ComputeBudgetHelper) GetEstimatedFee(operationType string) uint64 {
	priorityFee := CalculatePriorityFee(cbh.config)
	computeUnitLimit := GetRecommendedComputeUnitLimit(operationType)
	return EstimateTransactionFee(priorityFee, computeUnitLimit)
}

// GetOptimalPriorityFee returns optimal priority fee based on network conditions and urgency
func (cbh *ComputeBudgetHelper) GetOptimalPriorityFee(urgency string) uint64 {
	baseFee := cbh.config.Trading.PriorityFee

	switch urgency {
	case "low":
		return baseFee / 2 // 50% of base fee
	case "normal":
		return baseFee // Standard fee
	case "high":
		return baseFee * 2 // 2x base fee
	case "urgent":
		return baseFee * 5 // 5x base fee for urgent transactions
	case "extreme":
		return baseFee * 10 // 10x base fee for extreme urgency
	default:
		return baseFee
	}
}

// LogComputeBudgetInfo logs compute budget information for debugging
func (cbh *ComputeBudgetHelper) LogComputeBudgetInfo(operationType string, priorityFee uint64) map[string]interface{} {
	computeUnitLimit := GetRecommendedComputeUnitLimit(operationType)
	estimatedFee := EstimateTransactionFee(priorityFee, computeUnitLimit)

	return map[string]interface{}{
		"operation_type":     operationType,
		"compute_unit_limit": computeUnitLimit,
		"priority_fee":       priorityFee,
		"estimated_fee":      estimatedFee,
		"estimated_fee_sol":  config.ConvertLamportsToSOL(estimatedFee),
		"has_priority_fee":   priorityFee > 0,
		"ultra_fast_mode":    cbh.config.UltraFast.Enabled,
		"yolo_mode":          cbh.config.Strategy.YoloMode,
	}
}
