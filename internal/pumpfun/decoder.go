package pumpfun

import (
	"encoding/binary"
	"fmt"
)

// Pump.fun instruction discriminators
var (
	CreateDiscriminator = []byte{0x24, 0x30, 0x20, 0x95, 0x3a, 0xc8, 0x52, 0xb3}
	BuyDiscriminator    = []byte{0x66, 0x06, 0x3d, 0x12, 0x01, 0xda, 0xeb, 0xea}
	SellDiscriminator   = []byte{0x33, 0xe6, 0x85, 0xa4, 0x01, 0x7f, 0x83, 0xad}
)

// InstructionType represents pump.fun instruction types
type InstructionType int

const (
	UnknownInstruction InstructionType = iota
	CreateInstruction
	BuyInstruction
	SellInstruction
)

// CreateInstructionData represents create instruction data
type CreateInstructionData struct {
	Name   string
	Symbol string
	URI    string
}

// BuyInstructionData represents buy instruction data
type BuyInstructionData struct {
	Amount     uint64
	MaxSolCost uint64
}

// SellInstructionData represents sell instruction data
type SellInstructionData struct {
	Amount       uint64
	MinSolOutput uint64
}

// GetInstructionType determines instruction type from data
func GetInstructionType(data []byte) InstructionType {
	if len(data) < 8 {
		return UnknownInstruction
	}

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

// DecodeCreateInstruction decodes a create instruction
func DecodeCreateInstruction(data []byte) (*CreateInstructionData, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("instruction data too short")
	}

	if !bytesEqual(data[:8], CreateDiscriminator) {
		return nil, fmt.Errorf("invalid create instruction discriminator")
	}

	// Skip discriminator
	offset := 8

	// Decode name
	name, newOffset, err := decodeString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to decode name: %w", err)
	}
	offset = newOffset

	// Decode symbol
	symbol, newOffset, err := decodeString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to decode symbol: %w", err)
	}
	offset = newOffset

	// Decode URI
	uri, _, err := decodeString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to decode URI: %w", err)
	}

	return &CreateInstructionData{
		Name:   name,
		Symbol: symbol,
		URI:    uri,
	}, nil
}

// DecodeBuyInstruction decodes a buy instruction
func DecodeBuyInstruction(data []byte) (*BuyInstructionData, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("buy instruction data too short")
	}

	if !bytesEqual(data[:8], BuyDiscriminator) {
		return nil, fmt.Errorf("invalid buy instruction discriminator")
	}

	amount := binary.LittleEndian.Uint64(data[8:16])
	maxSolCost := binary.LittleEndian.Uint64(data[16:24])

	return &BuyInstructionData{
		Amount:     amount,
		MaxSolCost: maxSolCost,
	}, nil
}

// DecodeSellInstruction decodes a sell instruction
func DecodeSellInstruction(data []byte) (*SellInstructionData, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("sell instruction data too short")
	}

	if !bytesEqual(data[:8], SellDiscriminator) {
		return nil, fmt.Errorf("invalid sell instruction discriminator")
	}

	amount := binary.LittleEndian.Uint64(data[8:16])
	minSolOutput := binary.LittleEndian.Uint64(data[16:24])

	return &SellInstructionData{
		Amount:       amount,
		MinSolOutput: minSolOutput,
	}, nil
}

// Helper functions

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

// decodeString decodes a length-prefixed string
func decodeString(data []byte, offset int) (string, int, error) {
	if offset+4 > len(data) {
		return "", 0, fmt.Errorf("insufficient data for string length")
	}

	length := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	if offset+int(length) > len(data) {
		return "", 0, fmt.Errorf("insufficient data for string")
	}

	str := string(data[offset : offset+int(length)])
	offset += int(length)

	return str, offset, nil
}

// IsValidCreateInstruction checks if data is a valid create instruction
func IsValidCreateInstruction(data []byte) bool {
	return GetInstructionType(data) == CreateInstruction
}

// IsValidBuyInstruction checks if data is a valid buy instruction
func IsValidBuyInstruction(data []byte) bool {
	return GetInstructionType(data) == BuyInstruction
}

// IsValidSellInstruction checks if data is a valid sell instruction
func IsValidSellInstruction(data []byte) bool {
	return GetInstructionType(data) == SellInstruction
}

// GetInstructionTypeName returns string name of instruction type
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
