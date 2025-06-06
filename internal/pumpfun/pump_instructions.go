// internal/pumpfun/pump_instructions.go
package pumpfun

import (
	"encoding/binary"
	"fmt"
	"github.com/gagliardetto/solana-go"
)

// PumpFunConstants contains all pump.fun program constants
type PumpFunConstants struct {
	ProgramID      solana.PublicKey
	Global         solana.PublicKey
	FeeRecipient   solana.PublicKey
	EventAuthority solana.PublicKey
}

// GetPumpFunConstants returns pump.fun program constants
func GetPumpFunConstants() PumpFunConstants {
	return PumpFunConstants{
		ProgramID:      solana.MustPublicKeyFromBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"),
		Global:         solana.MustPublicKeyFromBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf"),
		FeeRecipient:   solana.MustPublicKeyFromBase58("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM"),
		EventAuthority: solana.MustPublicKeyFromBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1"),
	}
}

// CreatePumpFunAccountMetas creates the standard account array for pump.fun buy/sell instructions
// This function creates the exact same account order for both buy and sell instructions
// according to the pump.fun program IDL specification
func CreatePumpFunAccountMetas(
	tokenEvent *TokenEvent,
	userATA solana.PublicKey,
	userWallet solana.PublicKey,
) []*solana.AccountMeta {
	constants := GetPumpFunConstants()

	// Account order according to pump.fun IDL for both buy and sell instructions:
	// 0: global (read-only)
	// 1: feeRecipient (writable)
	// 2: mint (read-only)
	// 3: bondingCurve (writable)
	// 4: associatedBondingCurve (writable)
	// 5: associatedUser (writable) - user's token account
	// 6: user (writable, signer) - user's wallet
	// 7: systemProgram (read-only)
	// 8: tokenProgram (read-only)
	// 9: rent (read-only) for buy, but we use creator vault for both
	// 10: eventAuthority (read-only)
	// 11: program (read-only)
	return []*solana.AccountMeta{
		{PublicKey: constants.Global, IsWritable: false, IsSigner: false},                 // 0: global
		{PublicKey: constants.FeeRecipient, IsWritable: true, IsSigner: false},            // 1: feeRecipient
		{PublicKey: tokenEvent.Mint, IsWritable: false, IsSigner: false},                  // 2: mint
		{PublicKey: tokenEvent.BondingCurve, IsWritable: true, IsSigner: false},           // 3: bondingCurve
		{PublicKey: tokenEvent.AssociatedBondingCurve, IsWritable: true, IsSigner: false}, // 4: associatedBondingCurve
		{PublicKey: userATA, IsWritable: true, IsSigner: false},                           // 5: associatedUser
		{PublicKey: userWallet, IsWritable: true, IsSigner: true},                         // 6: user
		{PublicKey: solana.SystemProgramID, IsWritable: false, IsSigner: false},           // 7: systemProgram
		{PublicKey: solana.TokenProgramID, IsWritable: false, IsSigner: false},            // 8: tokenProgram
		{PublicKey: tokenEvent.CreatorVault, IsWritable: true, IsSigner: false},           // 9: rent/creatorVault
		{PublicKey: constants.EventAuthority, IsWritable: false, IsSigner: false},         // 10: eventAuthority
		{PublicKey: constants.ProgramID, IsWritable: false, IsSigner: false},              // 11: program
	}
}

// CreatePumpFunBuyInstruction creates a pump.fun buy instruction
func CreatePumpFunBuyInstruction(
	tokenEvent *TokenEvent,
	userATA solana.PublicKey,
	userWallet solana.PublicKey,
	tokenAmount uint64,
	maxSolCost uint64,
) solana.Instruction {
	constants := GetPumpFunConstants()
	accounts := CreatePumpFunAccountMetas(tokenEvent, userATA, userWallet)
	data := createBuyInstructionData(tokenAmount, maxSolCost)

	return solana.NewInstruction(
		constants.ProgramID,
		accounts,
		data,
	)
}

// CreatePumpFunSellInstruction creates a pump.fun sell instruction
func CreatePumpFunSellInstruction(
	tokenEvent *TokenEvent,
	userATA solana.PublicKey,
	userWallet solana.PublicKey,
	tokenAmount uint64,
	minSolOutput uint64,
) solana.Instruction {
	constants := GetPumpFunConstants()
	accounts := CreatePumpFunAccountMetas(tokenEvent, userATA, userWallet)
	data := createSellInstructionData(tokenAmount, minSolOutput)

	return solana.NewInstruction(
		constants.ProgramID,
		accounts,
		data,
	)
}

// createBuyInstructionData creates the buy instruction data
func createBuyInstructionData(tokenAmount uint64, maxSolCost uint64) []byte {
	// Buy instruction discriminator for pump.fun
	discriminator := uint64(16927863322537952870)

	data := make([]byte, 24)
	//// Discriminator (8 bytes)
	//data[0] = 0x66
	//data[1] = 0x06
	//data[2] = 0x3d
	//data[3] = 0x12
	//data[4] = 0x01
	//data[5] = 0xda
	//data[6] = 0xeb
	//data[7] = 0xea
	// Alternative way using binary encoding:
	binary.LittleEndian.PutUint64(data[0:8], discriminator)

	// Token amount (8 bytes)
	data[8] = byte(tokenAmount)
	data[9] = byte(tokenAmount >> 8)
	data[10] = byte(tokenAmount >> 16)
	data[11] = byte(tokenAmount >> 24)
	data[12] = byte(tokenAmount >> 32)
	data[13] = byte(tokenAmount >> 40)
	data[14] = byte(tokenAmount >> 48)
	data[15] = byte(tokenAmount >> 56)

	// Max SOL cost (8 bytes)
	data[16] = byte(maxSolCost)
	data[17] = byte(maxSolCost >> 8)
	data[18] = byte(maxSolCost >> 16)
	data[19] = byte(maxSolCost >> 24)
	data[20] = byte(maxSolCost >> 32)
	data[21] = byte(maxSolCost >> 40)
	data[22] = byte(maxSolCost >> 48)
	data[23] = byte(maxSolCost >> 56)

	return data
}

// createSellInstructionData creates the sell instruction data
func createSellInstructionData(tokenAmount uint64, minSolOutput uint64) []byte {
	// Sell instruction discriminator for pump.fun
	discriminator := uint64(12502976635542562355)

	data := make([]byte, 24)
	// Discriminator (8 bytes)
	//data[0] = 0x33
	//data[1] = 0xe6
	//data[2] = 0x85
	//data[3] = 0xa4
	//data[4] = 0x01
	//data[5] = 0x7f
	//data[6] = 0x83
	//data[7] = 0xad
	// Alternative way using binary encoding:
	binary.LittleEndian.PutUint64(data[0:8], discriminator)

	// Token amount (8 bytes)
	data[8] = byte(tokenAmount)
	data[9] = byte(tokenAmount >> 8)
	data[10] = byte(tokenAmount >> 16)
	data[11] = byte(tokenAmount >> 24)
	data[12] = byte(tokenAmount >> 32)
	data[13] = byte(tokenAmount >> 40)
	data[14] = byte(tokenAmount >> 48)
	data[15] = byte(tokenAmount >> 56)

	// Min SOL output (8 bytes)
	data[16] = byte(minSolOutput)
	data[17] = byte(minSolOutput >> 8)
	data[18] = byte(minSolOutput >> 16)
	data[19] = byte(minSolOutput >> 24)
	data[20] = byte(minSolOutput >> 32)
	data[21] = byte(minSolOutput >> 40)
	data[22] = byte(minSolOutput >> 48)
	data[23] = byte(minSolOutput >> 56)

	return data
}

// ValidatePumpFunAccounts validates that the token event has all required accounts
func ValidatePumpFunAccounts(tokenEvent *TokenEvent) error {
	if tokenEvent.Mint.IsZero() {
		return fmt.Errorf("mint address is zero")
	}

	if tokenEvent.BondingCurve.IsZero() {
		return fmt.Errorf("bonding curve address is zero")
	}

	if tokenEvent.AssociatedBondingCurve.IsZero() {
		return fmt.Errorf("associated bonding curve address is zero")
	}

	if tokenEvent.Creator.IsZero() {
		return fmt.Errorf("creator address is zero")
	}

	if tokenEvent.CreatorVault.IsZero() {
		return fmt.Errorf("creator vault address is zero")
	}

	return nil
}

// GetAccountsDebugInfo returns debug information about the accounts used
func GetAccountsDebugInfo(tokenEvent *TokenEvent, userWallet solana.PublicKey) map[string]string {
	constants := GetPumpFunConstants()

	return map[string]string{
		"program_id":               constants.ProgramID.String(),
		"global":                   constants.Global.String(),
		"fee_recipient":            constants.FeeRecipient.String(),
		"event_authority":          constants.EventAuthority.String(),
		"mint":                     tokenEvent.Mint.String(),
		"bonding_curve":            tokenEvent.BondingCurve.String(),
		"associated_bonding_curve": tokenEvent.AssociatedBondingCurve.String(),
		"creator":                  tokenEvent.Creator.String(),
		"creator_vault":            tokenEvent.CreatorVault.String(),
		"user_wallet":              userWallet.String(),
		"system_program":           solana.SystemProgramID.String(),
		"token_program":            solana.TokenProgramID.String(),
	}
}
