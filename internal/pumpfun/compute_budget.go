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

// GetRecommendedComputeUnitLimit returns recommended compute unit limit based on operation type
func GetRecommendedComputeUnitLimit(operationType string) uint32 {
	switch operationType {
	case "buy":
		return 400000 // Standard buy operation
	case "sell":
		return 180000 // Sell operations typically use less
	default:
		return 200000 // Default
	}
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
func (cbh *ComputeBudgetHelper) GetInstructionsForOperation(operationType string) []solana.Instruction {
	// Determine priority fee
	priorityFee := cbh.config.Trading.PriorityFee

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
