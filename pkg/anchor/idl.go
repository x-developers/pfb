package anchor

import (
	"encoding/json"
	"fmt"
	"os"
)

// IDL represents an Anchor Interface Definition Language file
type IDL struct {
	Version      string        `json:"version"`
	Name         string        `json:"name"`
	Instructions []Instruction `json:"instructions"`
	Accounts     []Account     `json:"accounts"`
	Types        []Type        `json:"types,omitempty"`
	Events       []Event       `json:"events,omitempty"`
	Errors       []Error       `json:"errors,omitempty"`
	Constants    []Constant    `json:"constants,omitempty"`
}

// Instruction represents an instruction definition
type Instruction struct {
	Name     string       `json:"name"`
	Accounts []IDLAccount `json:"accounts"`
	Args     []Field      `json:"args"`
	Returns  *Field       `json:"returns,omitempty"`
}

// IDLAccount represents an account in instruction context
type IDLAccount struct {
	Name       string   `json:"name"`
	IsMut      bool     `json:"isMut"`
	IsSigner   bool     `json:"isSigner"`
	IsOptional bool     `json:"isOptional,omitempty"`
	Docs       []string `json:"docs,omitempty"`
}

// Account represents an account definition
type Account struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
}

// Type represents a type definition
type Type struct {
	Name string  `json:"name"`
	Type TypeDef `json:"type"`
}

// TypeDef represents the actual type definition
type TypeDef struct {
	Kind   string  `json:"kind"`
	Fields []Field `json:"fields,omitempty"`
}

// Field represents a field in a struct
type Field struct {
	Name string      `json:"name"`
	Type interface{} `json:"type"`
	Docs []string    `json:"docs,omitempty"`
}

// Event represents an event definition
type Event struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

// Error represents an error definition
type Error struct {
	Code int    `json:"code"`
	Name string `json:"name"`
	Msg  string `json:"msg,omitempty"`
}

// Constant represents a constant definition
type Constant struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// LoadIDL loads an IDL file from path
func LoadIDL(path string) (*IDL, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read IDL file: %w", err)
	}

	var idl IDL
	if err := json.Unmarshal(data, &idl); err != nil {
		return nil, fmt.Errorf("failed to parse IDL JSON: %w", err)
	}

	return &idl, nil
}

// GetInstruction returns instruction by name
func (idl *IDL) GetInstruction(name string) (*Instruction, error) {
	for _, inst := range idl.Instructions {
		if inst.Name == name {
			return &inst, nil
		}
	}
	return nil, fmt.Errorf("instruction '%s' not found", name)
}

// GetAccount returns account type by name
func (idl *IDL) GetAccount(name string) (*Account, error) {
	for _, acc := range idl.Accounts {
		if acc.Name == name {
			return &acc, nil
		}
	}
	return nil, fmt.Errorf("account '%s' not found", name)
}

// GetType returns type definition by name
func (idl *IDL) GetType(name string) (*Type, error) {
	for _, t := range idl.Types {
		if t.Name == name {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("type '%s' not found", name)
}

// GetError returns error by code
func (idl *IDL) GetError(code int) (*Error, error) {
	for _, e := range idl.Errors {
		if e.Code == code {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("error with code %d not found", code)
}

// GetInstructionDiscriminator computes discriminator for instruction
func (idl *IDL) GetInstructionDiscriminator(name string) (Discriminator, error) {
	inst, err := idl.GetInstruction(name)
	if err != nil {
		return Discriminator{}, err
	}

	return ComputeInstructionDiscriminator(inst.Name), nil
}

// GetAccountDiscriminator computes discriminator for account
func (idl *IDL) GetAccountDiscriminator(name string) (Discriminator, error) {
	acc, err := idl.GetAccount(name)
	if err != nil {
		return Discriminator{}, err
	}

	return ComputeAccountDiscriminator(acc.Name), nil
}

// ValidateInstruction validates instruction data format
func (idl *IDL) ValidateInstruction(name string, data []byte) error {
	discriminator, err := idl.GetInstructionDiscriminator(name)
	if err != nil {
		return fmt.Errorf("failed to get discriminator: %w", err)
	}

	return ValidateDiscriminator(data, discriminator)
}

// ValidateAccount validates account data format
func (idl *IDL) ValidateAccount(name string, data []byte) error {
	discriminator, err := idl.GetAccountDiscriminator(name)
	if err != nil {
		return fmt.Errorf("failed to get discriminator: %w", err)
	}

	return ValidateDiscriminator(data, discriminator)
}

// PumpFunIDL represents the pump.fun program IDL
var PumpFunIDL = &IDL{
	Version: "0.1.0",
	Name:    "pump_fun",
	Instructions: []Instruction{
		{
			Name: "create",
			Accounts: []IDLAccount{
				{Name: "mint", IsMut: true, IsSigner: true},
				{Name: "mintAuthority", IsMut: false, IsSigner: false},
				{Name: "bondingCurve", IsMut: true, IsSigner: false},
				{Name: "associatedBondingCurve", IsMut: true, IsSigner: false},
				{Name: "global", IsMut: false, IsSigner: false},
				{Name: "mplTokenMetadata", IsMut: false, IsSigner: false},
				{Name: "metadata", IsMut: true, IsSigner: false},
				{Name: "user", IsMut: true, IsSigner: true},
				{Name: "systemProgram", IsMut: false, IsSigner: false},
				{Name: "tokenProgram", IsMut: false, IsSigner: false},
				{Name: "associatedTokenProgram", IsMut: false, IsSigner: false},
				{Name: "rent", IsMut: false, IsSigner: false},
				{Name: "eventAuthority", IsMut: false, IsSigner: false},
				{Name: "program", IsMut: false, IsSigner: false},
			},
			Args: []Field{
				{Name: "name", Type: "string"},
				{Name: "symbol", Type: "string"},
				{Name: "uri", Type: "string"},
			},
		},
		{
			Name: "buy",
			Accounts: []IDLAccount{
				{Name: "global", IsMut: false, IsSigner: false},
				{Name: "feeRecipient", IsMut: true, IsSigner: false},
				{Name: "mint", IsMut: false, IsSigner: false},
				{Name: "bondingCurve", IsMut: true, IsSigner: false},
				{Name: "associatedBondingCurve", IsMut: true, IsSigner: false},
				{Name: "associatedUser", IsMut: true, IsSigner: false},
				{Name: "user", IsMut: true, IsSigner: true},
				{Name: "systemProgram", IsMut: false, IsSigner: false},
				{Name: "tokenProgram", IsMut: false, IsSigner: false},
				{Name: "rent", IsMut: false, IsSigner: false},
				{Name: "eventAuthority", IsMut: false, IsSigner: false},
				{Name: "program", IsMut: false, IsSigner: false},
			},
			Args: []Field{
				{Name: "amount", Type: "u64"},
				{Name: "maxSolCost", Type: "u64"},
			},
		},
		{
			Name: "sell",
			Accounts: []IDLAccount{
				{Name: "global", IsMut: false, IsSigner: false},
				{Name: "feeRecipient", IsMut: true, IsSigner: false},
				{Name: "mint", IsMut: false, IsSigner: false},
				{Name: "bondingCurve", IsMut: true, IsSigner: false},
				{Name: "associatedBondingCurve", IsMut: true, IsSigner: false},
				{Name: "associatedUser", IsMut: true, IsSigner: false},
				{Name: "user", IsMut: true, IsSigner: true},
				{Name: "systemProgram", IsMut: false, IsSigner: false},
				{Name: "tokenProgram", IsMut: false, IsSigner: false},
				{Name: "eventAuthority", IsMut: false, IsSigner: false},
				{Name: "program", IsMut: false, IsSigner: false},
			},
			Args: []Field{
				{Name: "amount", Type: "u64"},
				{Name: "minSolOutput", Type: "u64"},
			},
		},
	},
	Accounts: []Account{
		{
			Name: "BondingCurve",
			Type: Type{
				Name: "BondingCurve",
				Type: TypeDef{
					Kind: "struct",
					Fields: []Field{
						{Name: "virtualTokenReserves", Type: "u64"},
						{Name: "virtualSolReserves", Type: "u64"},
						{Name: "realTokenReserves", Type: "u64"},
						{Name: "realSolReserves", Type: "u64"},
						{Name: "tokenTotalSupply", Type: "u64"},
						{Name: "complete", Type: "bool"},
					},
				},
			},
		},
		{
			Name: "Global",
			Type: Type{
				Name: "Global",
				Type: TypeDef{
					Kind: "struct",
					Fields: []Field{
						{Name: "initialized", Type: "bool"},
						{Name: "authority", Type: "publicKey"},
						{Name: "feeRecipient", Type: "publicKey"},
						{Name: "initialVirtualTokenReserves", Type: "u64"},
						{Name: "initialVirtualSolReserves", Type: "u64"},
						{Name: "initialRealTokenReserves", Type: "u64"},
						{Name: "tokenTotalSupply", Type: "u64"},
						{Name: "feeBasisPoints", Type: "u64"},
					},
				},
			},
		},
	},
	Events: []Event{
		{
			Name: "CreateEvent",
			Fields: []Field{
				{Name: "mint", Type: "publicKey"},
				{Name: "bondingCurve", Type: "publicKey"},
				{Name: "user", Type: "publicKey"},
			},
		},
		{
			Name: "TradeEvent",
			Fields: []Field{
				{Name: "mint", Type: "publicKey"},
				{Name: "solAmount", Type: "u64"},
				{Name: "tokenAmount", Type: "u64"},
				{Name: "isBuy", Type: "bool"},
				{Name: "user", Type: "publicKey"},
				{Name: "timestamp", Type: "i64"},
				{Name: "virtualSolReserves", Type: "u64"},
				{Name: "virtualTokenReserves", Type: "u64"},
			},
		},
	},
}

// GetPumpFunInstruction returns pump.fun specific instruction
func GetPumpFunInstruction(name string) (*Instruction, error) {
	return PumpFunIDL.GetInstruction(name)
}

// GetPumpFunAccount returns pump.fun specific account
func GetPumpFunAccount(name string) (*Account, error) {
	return PumpFunIDL.GetAccount(name)
}

// IsPumpFunInstruction checks if discriminator matches pump.fun instruction
func IsPumpFunInstruction(data []byte, instructionName string) bool {
	discriminator, err := PumpFunIDL.GetInstructionDiscriminator(instructionName)
	if err != nil {
		return false
	}

	return ValidateDiscriminator(data, discriminator) == nil
}
