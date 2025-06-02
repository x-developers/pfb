package anchor

import (
	"crypto/sha256"
	"fmt"
)

// Discriminator represents an 8-byte instruction discriminator
type Discriminator [8]byte

// String returns hex representation of discriminator
func (d Discriminator) String() string {
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x",
		d[0], d[1], d[2], d[3], d[4], d[5], d[6], d[7])
}

// Bytes returns discriminator as byte slice
func (d Discriminator) Bytes() []byte {
	return d[:]
}

// Equals compares two discriminators
func (d Discriminator) Equals(other Discriminator) bool {
	for i := 0; i < 8; i++ {
		if d[i] != other[i] {
			return false
		}
	}
	return true
}

// ComputeDiscriminator computes 8-byte discriminator for instruction/account
func ComputeDiscriminator(namespace, name string) Discriminator {
	// Anchor discriminator is computed as:
	// sha256(namespace:name)[0:8]
	input := fmt.Sprintf("%s:%s", namespace, name)
	hash := sha256.Sum256([]byte(input))

	var discriminator Discriminator
	copy(discriminator[:], hash[:8])
	return discriminator
}

// ComputeInstructionDiscriminator computes discriminator for instruction
func ComputeInstructionDiscriminator(name string) Discriminator {
	return ComputeDiscriminator("global", name)
}

// ComputeAccountDiscriminator computes discriminator for account
func ComputeAccountDiscriminator(name string) Discriminator {
	return ComputeDiscriminator("account", name)
}

// Predefined pump.fun discriminators
var (
	// Instruction discriminators
	CreateDiscriminator    = ComputeInstructionDiscriminator("create")
	BuyDiscriminator       = ComputeInstructionDiscriminator("buy")
	SellDiscriminator      = ComputeInstructionDiscriminator("sell")
	WithdrawDiscriminator  = ComputeInstructionDiscriminator("withdraw")
	SetParamsDiscriminator = ComputeInstructionDiscriminator("set_params")

	// Account discriminators
	GlobalDiscriminator       = ComputeAccountDiscriminator("Global")
	BondingCurveDiscriminator = ComputeAccountDiscriminator("BondingCurve")

	// Known discriminators map for lookup
	KnownInstructionDiscriminators = map[Discriminator]string{
		CreateDiscriminator:    "create",
		BuyDiscriminator:       "buy",
		SellDiscriminator:      "sell",
		WithdrawDiscriminator:  "withdraw",
		SetParamsDiscriminator: "set_params",
	}

	KnownAccountDiscriminators = map[Discriminator]string{
		GlobalDiscriminator:       "Global",
		BondingCurveDiscriminator: "BondingCurve",
	}
)

// GetInstructionName returns instruction name for discriminator
func GetInstructionName(discriminator Discriminator) string {
	if name, exists := KnownInstructionDiscriminators[discriminator]; exists {
		return name
	}
	return "unknown"
}

// GetAccountName returns account name for discriminator
func GetAccountName(discriminator Discriminator) string {
	if name, exists := KnownAccountDiscriminators[discriminator]; exists {
		return name
	}
	return "unknown"
}

// IsKnownInstruction checks if discriminator is a known instruction
func IsKnownInstruction(discriminator Discriminator) bool {
	_, exists := KnownInstructionDiscriminators[discriminator]
	return exists
}

// IsKnownAccount checks if discriminator is a known account
func IsKnownAccount(discriminator Discriminator) bool {
	_, exists := KnownAccountDiscriminators[discriminator]
	return exists
}

// DiscriminatorFromBytes creates discriminator from byte slice
func DiscriminatorFromBytes(data []byte) (Discriminator, error) {
	if len(data) < 8 {
		return Discriminator{}, fmt.Errorf("data too short for discriminator: need 8 bytes, got %d", len(data))
	}

	var discriminator Discriminator
	copy(discriminator[:], data[:8])
	return discriminator, nil
}

// DiscriminatorFromHex creates discriminator from hex string
func DiscriminatorFromHex(hex string) (Discriminator, error) {
	if len(hex) != 16 {
		return Discriminator{}, fmt.Errorf("invalid hex length: expected 16, got %d", len(hex))
	}

	var discriminator Discriminator
	for i := 0; i < 8; i++ {
		var b byte
		_, err := fmt.Sscanf(hex[i*2:i*2+2], "%02x", &b)
		if err != nil {
			return Discriminator{}, fmt.Errorf("invalid hex at position %d: %w", i, err)
		}
		discriminator[i] = b
	}

	return discriminator, nil
}

// ValidateDiscriminator validates that data starts with expected discriminator
func ValidateDiscriminator(data []byte, expected Discriminator) error {
	if len(data) < 8 {
		return fmt.Errorf("data too short for discriminator validation")
	}

	actual, err := DiscriminatorFromBytes(data)
	if err != nil {
		return fmt.Errorf("failed to extract discriminator: %w", err)
	}

	if !actual.Equals(expected) {
		return fmt.Errorf("discriminator mismatch: expected %s, got %s",
			expected.String(), actual.String())
	}

	return nil
}
