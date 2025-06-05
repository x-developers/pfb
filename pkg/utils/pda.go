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

func (p *PumpFunPDADerivation) DeriveAssociatedBondingCurve(mint solana.PublicKey, bondingCurve solana.PublicKey) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{
		bondingCurve.Bytes(),
		//solana.TokenProgramID.Bytes(),
		mint.Bytes(),
	}

	data, nonce, err := solana.FindProgramAddress(seeds, solana.SPLAssociatedTokenAccountProgramID)

	return data, nonce, err
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
