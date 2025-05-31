package utils

import (
	"github.com/blocto/solana-go-sdk/common"
	"pump-fun-bot-go/internal/config"
)

type PumpFunPDADerivation struct {
}

func NewPumpFunPDADerivation() *PumpFunPDADerivation {
	return &PumpFunPDADerivation{}
}

func (p *PumpFunPDADerivation) DeriveAssociatedBondingCurve(mint common.PublicKey, bondingCurve common.PublicKey) (*common.PublicKey, uint8, error) {
	seeds := [][]byte{
		bondingCurve.Bytes(),
		common.TokenProgramID.Bytes(),
		mint.Bytes(),
	}

	data, nonce, err := common.FindProgramAddress(seeds, common.SPLAssociatedTokenAccountProgramID)

	return &data, nonce, err
}

func (p *PumpFunPDADerivation) DeriveCreatorVault(creator common.PublicKey) (*common.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("creator-vault"),
		common.TokenProgramID.Bytes(),
		creator.Bytes(),
	}

	data, nonce, err := common.FindProgramAddress(seeds, common.PublicKeyFromBytes(config.PumpFunProgramID))

	return &data, nonce, err
}
