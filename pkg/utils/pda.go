package utils

import (
	"github.com/gagliardetto/solana-go"
	"pump-fun-bot-go/internal/config"
)

type PumpFunPDADerivation struct {
}

func NewPumpFunPDADerivation() *PumpFunPDADerivation {
	return &PumpFunPDADerivation{}
}

func (p *PumpFunPDADerivation) DeriveCreatorVault(creator solana.PublicKey) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("creator-vault"),
		creator.Bytes(),
	}

	programID := solana.PublicKeyFromBytes(config.PumpFunProgramID)
	data, nonce, err := solana.FindProgramAddress(seeds, programID)

	return data, nonce, err
}

// DeriveBondingCurve derives the bonding curve PDA using the mint
func (p *PumpFunPDADerivation) DeriveBondingCurve(mint solana.PublicKey) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("bonding-curve"),
		mint.Bytes(),
	}

	programID := solana.PublicKeyFromBytes(config.PumpFunProgramID)
	pda, nonce, err := solana.FindProgramAddress(seeds, programID)

	return pda, nonce, err
}

// DeriveAssociatedBondingCurve derives the associated bonding curve token account
func (p *PumpFunPDADerivation) DeriveAssociatedBondingCurve(bondingCurve, mint solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindAssociatedTokenAddress(bondingCurve, mint)
}
