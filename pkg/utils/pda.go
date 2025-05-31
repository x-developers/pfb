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

func (p *PumpFunPDADerivation) DeriveAssociatedBondingCurve(mint string, bondingCurve string) (common.PublicKey, uint8, error) {
	seeds := [][]byte{
		common.PublicKeyFromString(bondingCurve).Bytes(),
		common.TokenProgramID.Bytes(),
		common.PublicKeyFromString(mint).Bytes(),
	}

	return common.FindProgramAddress(seeds, common.SPLAssociatedTokenAccountProgramID)
}

func (p *PumpFunPDADerivation) DeriveCreatorVault(creator string) (common.PublicKey, uint8, error) {
	seeds := [][]byte{
		[]byte("creator-vault"),
		common.TokenProgramID.Bytes(),
		common.PublicKeyFromString(creator).Bytes(),
	}

	return common.FindProgramAddress(seeds, common.PublicKeyFromBytes(config.PumpFunProgramID))
}
