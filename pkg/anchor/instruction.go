package anchor

import (
	"encoding/binary"
	"fmt"
)

// InstructionBuilder helps build Anchor instructions
type InstructionBuilder struct {
	discriminator Discriminator
	data          []byte
}

// NewInstructionBuilder creates a new instruction builder
func NewInstructionBuilder(instructionName string) *InstructionBuilder {
	discriminator := ComputeInstructionDiscriminator(instructionName)
	return &InstructionBuilder{
		discriminator: discriminator,
		data:          discriminator.Bytes(),
	}
}

// AddU8 adds a u8 value to instruction data
func (ib *InstructionBuilder) AddU8(value uint8) *InstructionBuilder {
	ib.data = append(ib.data, value)
	return ib
}

// AddU16 adds a u16 value to instruction data (little endian)
func (ib *InstructionBuilder) AddU16(value uint16) *InstructionBuilder {
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, value)
	ib.data = append(ib.data, buf...)
	return ib
}

// AddU32 adds a u32 value to instruction data (little endian)
func (ib *InstructionBuilder) AddU32(value uint32) *InstructionBuilder {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, value)
	ib.data = append(ib.data, buf...)
	return ib
}

// AddU64 adds a u64 value to instruction data (little endian)
func (ib *InstructionBuilder) AddU64(value uint64) *InstructionBuilder {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, value)
	ib.data = append(ib.data, buf...)
	return ib
}

// AddString adds a string to instruction data (length-prefixed)
func (ib *InstructionBuilder) AddString(value string) *InstructionBuilder {
	data := []byte(value)
	// Add length as u32
	ib.AddU32(uint32(len(data)))
	// Add string data
	ib.data = append(ib.data, data...)
	return ib
}

// AddBytes adds raw bytes to instruction data
func (ib *InstructionBuilder) AddBytes(value []byte) *InstructionBuilder {
	ib.data = append(ib.data, value...)
	return ib
}

// AddBool adds a boolean to instruction data
func (ib *InstructionBuilder) AddBool(value bool) *InstructionBuilder {
	if value {
		ib.data = append(ib.data, 1)
	} else {
		ib.data = append(ib.data, 0)
	}
	return ib
}

// Build returns the final instruction data
func (ib *InstructionBuilder) Build() []byte {
	return ib.data
}

// GetDiscriminator returns the instruction discriminator
func (ib *InstructionBuilder) GetDiscriminator() Discriminator {
	return ib.discriminator
}

// InstructionDecoder helps decode Anchor instructions
type InstructionDecoder struct {
	data   []byte
	offset int
}

// NewInstructionDecoder creates a new instruction decoder
func NewInstructionDecoder(data []byte) *InstructionDecoder {
	return &InstructionDecoder{
		data:   data,
		offset: 8, // Skip discriminator
	}
}

// GetDiscriminator returns the instruction discriminator
func (id *InstructionDecoder) GetDiscriminator() (Discriminator, error) {
	return DiscriminatorFromBytes(id.data)
}

// ReadU8 reads a u8 value from instruction data
func (id *InstructionDecoder) ReadU8() (uint8, error) {
	if id.offset+1 > len(id.data) {
		return 0, fmt.Errorf("not enough data to read u8")
	}
	value := id.data[id.offset]
	id.offset++
	return value, nil
}

// ReadU16 reads a u16 value from instruction data (little endian)
func (id *InstructionDecoder) ReadU16() (uint16, error) {
	if id.offset+2 > len(id.data) {
		return 0, fmt.Errorf("not enough data to read u16")
	}
	value := binary.LittleEndian.Uint16(id.data[id.offset:])
	id.offset += 2
	return value, nil
}

// ReadU32 reads a u32 value from instruction data (little endian)
func (id *InstructionDecoder) ReadU32() (uint32, error) {
	if id.offset+4 > len(id.data) {
		return 0, fmt.Errorf("not enough data to read u32")
	}
	value := binary.LittleEndian.Uint32(id.data[id.offset:])
	id.offset += 4
	return value, nil
}

// ReadU64 reads a u64 value from instruction data (little endian)
func (id *InstructionDecoder) ReadU64() (uint64, error) {
	if id.offset+8 > len(id.data) {
		return 0, fmt.Errorf("not enough data to read u64")
	}
	value := binary.LittleEndian.Uint64(id.data[id.offset:])
	id.offset += 8
	return value, nil
}

// ReadString reads a string from instruction data (length-prefixed)
func (id *InstructionDecoder) ReadString() (string, error) {
	// Read length
	length, err := id.ReadU32()
	if err != nil {
		return "", fmt.Errorf("failed to read string length: %w", err)
	}

	// Read string data
	if id.offset+int(length) > len(id.data) {
		return "", fmt.Errorf("not enough data to read string of length %d", length)
	}

	value := string(id.data[id.offset : id.offset+int(length)])
	id.offset += int(length)
	return value, nil
}

// ReadBytes reads raw bytes from instruction data
func (id *InstructionDecoder) ReadBytes(length int) ([]byte, error) {
	if id.offset+length > len(id.data) {
		return nil, fmt.Errorf("not enough data to read %d bytes", length)
	}

	value := make([]byte, length)
	copy(value, id.data[id.offset:id.offset+length])
	id.offset += length
	return value, nil
}

// ReadBool reads a boolean from instruction data
func (id *InstructionDecoder) ReadBool() (bool, error) {
	value, err := id.ReadU8()
	if err != nil {
		return false, err
	}
	return value != 0, nil
}

// HasMoreData checks if there's more data to read
func (id *InstructionDecoder) HasMoreData() bool {
	return id.offset < len(id.data)
}

// Remaining returns remaining bytes count
func (id *InstructionDecoder) Remaining() int {
	return len(id.data) - id.offset
}

// PumpFun specific instruction builders

// CreateInstruction represents a pump.fun create instruction
type CreateInstruction struct {
	Name   string
	Symbol string
	URI    string
}

// BuildCreateInstruction builds a pump.fun create instruction
func BuildCreateInstruction(name, symbol, uri string) []byte {
	return NewInstructionBuilder("create").
		AddString(name).
		AddString(symbol).
		AddString(uri).
		Build()
}

// DecodeCreateInstruction decodes a pump.fun create instruction
func DecodeCreateInstruction(data []byte) (*CreateInstruction, error) {
	decoder := NewInstructionDecoder(data)

	// Validate discriminator
	discriminator, err := decoder.GetDiscriminator()
	if err != nil {
		return nil, fmt.Errorf("failed to get discriminator: %w", err)
	}

	expectedDiscriminator := ComputeInstructionDiscriminator("create")
	if !discriminator.Equals(expectedDiscriminator) {
		return nil, fmt.Errorf("invalid discriminator for create instruction")
	}

	// Decode fields
	name, err := decoder.ReadString()
	if err != nil {
		return nil, fmt.Errorf("failed to read name: %w", err)
	}

	symbol, err := decoder.ReadString()
	if err != nil {
		return nil, fmt.Errorf("failed to read symbol: %w", err)
	}

	uri, err := decoder.ReadString()
	if err != nil {
		return nil, fmt.Errorf("failed to read URI: %w", err)
	}

	return &CreateInstruction{
		Name:   name,
		Symbol: symbol,
		URI:    uri,
	}, nil
}

// BuyInstruction represents a pump.fun buy instruction
type BuyInstruction struct {
	Amount     uint64
	MaxSolCost uint64
}

// BuildBuyInstruction builds a pump.fun buy instruction
func BuildBuyInstruction(amount, maxSolCost uint64) []byte {
	return NewInstructionBuilder("buy").
		AddU64(amount).
		AddU64(maxSolCost).
		Build()
}

// DecodeBuyInstruction decodes a pump.fun buy instruction
func DecodeBuyInstruction(data []byte) (*BuyInstruction, error) {
	decoder := NewInstructionDecoder(data)

	// Validate discriminator
	discriminator, err := decoder.GetDiscriminator()
	if err != nil {
		return nil, fmt.Errorf("failed to get discriminator: %w", err)
	}

	expectedDiscriminator := ComputeInstructionDiscriminator("buy")
	if !discriminator.Equals(expectedDiscriminator) {
		return nil, fmt.Errorf("invalid discriminator for buy instruction")
	}

	// Decode fields
	amount, err := decoder.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("failed to read amount: %w", err)
	}

	maxSolCost, err := decoder.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("failed to read maxSolCost: %w", err)
	}

	return &BuyInstruction{
		Amount:     amount,
		MaxSolCost: maxSolCost,
	}, nil
}

// SellInstruction represents a pump.fun sell instruction
type SellInstruction struct {
	Amount       uint64
	MinSolOutput uint64
}

// BuildSellInstruction builds a pump.fun sell instruction
func BuildSellInstruction(amount, minSolOutput uint64) []byte {
	return NewInstructionBuilder("sell").
		AddU64(amount).
		AddU64(minSolOutput).
		Build()
}

// DecodeSellInstruction decodes a pump.fun sell instruction
func DecodeSellInstruction(data []byte) (*SellInstruction, error) {
	decoder := NewInstructionDecoder(data)

	// Validate discriminator
	discriminator, err := decoder.GetDiscriminator()
	if err != nil {
		return nil, fmt.Errorf("failed to get discriminator: %w", err)
	}

	expectedDiscriminator := ComputeInstructionDiscriminator("sell")
	if !discriminator.Equals(expectedDiscriminator) {
		return nil, fmt.Errorf("invalid discriminator for sell instruction")
	}

	// Decode fields
	amount, err := decoder.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("failed to read amount: %w", err)
	}

	minSolOutput, err := decoder.ReadU64()
	if err != nil {
		return nil, fmt.Errorf("failed to read minSolOutput: %w", err)
	}

	return &SellInstruction{
		Amount:       amount,
		MinSolOutput: minSolOutput,
	}, nil
}

// Helper functions for instruction validation and identification

// IdentifyInstruction identifies the type of pump.fun instruction
func IdentifyInstruction(data []byte) (string, error) {
	if len(data) < 8 {
		return "", fmt.Errorf("data too short for instruction identification")
	}

	discriminator, err := DiscriminatorFromBytes(data)
	if err != nil {
		return "", fmt.Errorf("failed to extract discriminator: %w", err)
	}

	// Check against known pump.fun instructions
	createDiscriminator := ComputeInstructionDiscriminator("create")
	if discriminator.Equals(createDiscriminator) {
		return "create", nil
	}

	buyDiscriminator := ComputeInstructionDiscriminator("buy")
	if discriminator.Equals(buyDiscriminator) {
		return "buy", nil
	}

	sellDiscriminator := ComputeInstructionDiscriminator("sell")
	if discriminator.Equals(sellDiscriminator) {
		return "sell", nil
	}

	return "unknown", nil
}

// IsCreateInstruction checks if data represents a create instruction
func IsCreateInstruction(data []byte) bool {
	instructionType, err := IdentifyInstruction(data)
	return err == nil && instructionType == "create"
}

// IsBuyInstruction checks if data represents a buy instruction
func IsBuyInstruction(data []byte) bool {
	instructionType, err := IdentifyInstruction(data)
	return err == nil && instructionType == "buy"
}

// IsSellInstruction checks if data represents a sell instruction
func IsSellInstruction(data []byte) bool {
	instructionType, err := IdentifyInstruction(data)
	return err == nil && instructionType == "sell"
}
