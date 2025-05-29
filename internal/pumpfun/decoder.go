package pumpfun

import (
	"encoding/binary"
	"fmt"
	"pump-fun-bot-go/pkg/anchor"
)

// Legacy discriminators for backward compatibility
var (
	// Legacy pump.fun instruction discriminators (keeping for compatibility)
	CreateDiscriminator = []byte{0x24, 0x30, 0x20, 0x95, 0x3a, 0xc8, 0x52, 0xb3} // create
	BuyDiscriminator    = []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea} // buy
	SellDiscriminator   = []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad} // sell
)

// CreateInstructionData represents the data for a create instruction (legacy)
type CreateInstructionData struct {
	Discriminator [8]byte
	Name          string
	Symbol        string
	URI           string
}

// BuyInstructionData represents the data for a buy instruction (legacy)
type BuyInstructionData struct {
	Discriminator [8]byte
	Amount        uint64
	MaxSolCost    uint64
}

// SellInstructionData represents the data for a sell instruction (legacy)
type SellInstructionData struct {
	Discriminator [8]byte
	Amount        uint64
	MinSolOutput  uint64
}

// InstructionType represents the type of pump.fun instruction
type InstructionType int

const (
	UnknownInstruction InstructionType = iota
	CreateInstruction
	BuyInstruction
	SellInstruction
)

// GetInstructionType determines the instruction type from data
// Now uses Anchor discriminators as primary method
func GetInstructionType(data []byte) InstructionType {
	if len(data) < 8 {
		return UnknownInstruction
	}

	// First try Anchor discriminators
	instructionName, err := anchor.IdentifyInstruction(data)
	if err == nil {
		switch instructionName {
		case "create":
			return CreateInstruction
		case "buy":
			return BuyInstruction
		case "sell":
			return SellInstruction
		}
	}

	// Fallback to legacy discriminators
	discriminator := data[:8]

	if bytesEqual(discriminator, CreateDiscriminator) {
		return CreateInstruction
	}
	if bytesEqual(discriminator, BuyDiscriminator) {
		return BuyInstruction
	}
	if bytesEqual(discriminator, SellDiscriminator) {
		return SellInstruction
	}

	return UnknownInstruction
}

// DecodeCreateInstruction decodes a create instruction (legacy fallback)
func DecodeCreateInstruction(data []byte) (*CreateInstructionData, error) {
	// First try Anchor decoding
	anchorInst, err := anchor.DecodeCreateInstruction(data)
	if err == nil {
		return &CreateInstructionData{
			Name:   anchorInst.Name,
			Symbol: anchorInst.Symbol,
			URI:    anchorInst.URI,
		}, nil
	}

	// Fallback to legacy decoding
	if len(data) < 8 {
		return nil, fmt.Errorf("instruction data too short")
	}

	if !bytesEqual(data[:8], CreateDiscriminator) {
		return nil, fmt.Errorf("invalid create instruction discriminator")
	}

	instruction := &CreateInstructionData{}
	copy(instruction.Discriminator[:], data[:8])

	// Skip discriminator
	offset := 8

	// Decode name (length-prefixed string)
	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for name length")
	}

	nameLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	if len(data) < offset+int(nameLen) {
		return nil, fmt.Errorf("insufficient data for name")
	}

	instruction.Name = string(data[offset : offset+int(nameLen)])
	offset += int(nameLen)

	// Decode symbol (length-prefixed string)
	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for symbol length")
	}

	symbolLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	if len(data) < offset+int(symbolLen) {
		return nil, fmt.Errorf("insufficient data for symbol")
	}

	instruction.Symbol = string(data[offset : offset+int(symbolLen)])
	offset += int(symbolLen)

	// Decode URI (length-prefixed string)
	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for URI length")
	}

	uriLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	if len(data) < offset+int(uriLen) {
		return nil, fmt.Errorf("insufficient data for URI")
	}

	instruction.URI = string(data[offset : offset+int(uriLen)])

	return instruction, nil
}

// DecodeBuyInstruction decodes a buy instruction (legacy fallback)
func DecodeBuyInstruction(data []byte) (*BuyInstructionData, error) {
	// First try Anchor decoding
	anchorInst, err := anchor.DecodeBuyInstruction(data)
	if err == nil {
		return &BuyInstructionData{
			Amount:     anchorInst.Amount,
			MaxSolCost: anchorInst.MaxSolCost,
		}, nil
	}

	// Fallback to legacy decoding
	if len(data) < 24 { // 8 bytes discriminator + 8 bytes amount + 8 bytes max_sol_cost
		return nil, fmt.Errorf("buy instruction data too short")
	}

	if !bytesEqual(data[:8], BuyDiscriminator) {
		return nil, fmt.Errorf("invalid buy instruction discriminator")
	}

	instruction := &BuyInstructionData{}
	copy(instruction.Discriminator[:], data[:8])

	// Decode amount (8 bytes, little endian)
	instruction.Amount = binary.LittleEndian.Uint64(data[8:16])

	// Decode max_sol_cost (8 bytes, little endian)
	instruction.MaxSolCost = binary.LittleEndian.Uint64(data[16:24])

	return instruction, nil
}

// DecodeSellInstruction decodes a sell instruction (legacy fallback)
func DecodeSellInstruction(data []byte) (*SellInstructionData, error) {
	// First try Anchor decoding
	anchorInst, err := anchor.DecodeSellInstruction(data)
	if err == nil {
		return &SellInstructionData{
			Amount:       anchorInst.Amount,
			MinSolOutput: anchorInst.MinSolOutput,
		}, nil
	}

	// Fallback to legacy decoding
	if len(data) < 24 { // 8 bytes discriminator + 8 bytes amount + 8 bytes min_sol_output
		return nil, fmt.Errorf("sell instruction data too short")
	}

	if !bytesEqual(data[:8], SellDiscriminator) {
		return nil, fmt.Errorf("invalid sell instruction discriminator")
	}

	instruction := &SellInstructionData{}
	copy(instruction.Discriminator[:], data[:8])

	// Decode amount (8 bytes, little endian)
	instruction.Amount = binary.LittleEndian.Uint64(data[8:16])

	// Decode min_sol_output (8 bytes, little endian)
	instruction.MinSolOutput = binary.LittleEndian.Uint64(data[16:24])

	return instruction, nil
}

// CreateBuyInstructionData creates buy instruction data using Anchor
func CreateBuyInstructionData(amount, maxSolCost uint64) []byte {
	return anchor.BuildBuyInstruction(amount, maxSolCost)
}

// CreateSellInstructionData creates sell instruction data using Anchor
func CreateSellInstructionData(amount, minSolOutput uint64) []byte {
	return anchor.BuildSellInstruction(amount, minSolOutput)
}

// bytesEqual compares two byte slices
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Helper functions for instruction validation (now using Anchor)

// IsValidCreateInstruction checks if data represents a valid create instruction
func IsValidCreateInstruction(data []byte) bool {
	return anchor.IsCreateInstruction(data) || GetInstructionType(data) == CreateInstruction
}

// IsValidBuyInstruction checks if data represents a valid buy instruction
func IsValidBuyInstruction(data []byte) bool {
	return anchor.IsBuyInstruction(data) || GetInstructionType(data) == BuyInstruction
}

// IsValidSellInstruction checks if data represents a valid sell instruction
func IsValidSellInstruction(data []byte) bool {
	return anchor.IsSellInstruction(data) || GetInstructionType(data) == SellInstruction
}

// GetInstructionTypeName returns the string name of an instruction type
func GetInstructionTypeName(instType InstructionType) string {
	switch instType {
	case CreateInstruction:
		return "create"
	case BuyInstruction:
		return "buy"
	case SellInstruction:
		return "sell"
	default:
		return "unknown"
	}
}
