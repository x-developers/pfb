package pumpfun

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
