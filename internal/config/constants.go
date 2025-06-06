package config

import "github.com/mr-tron/base58"

// Solana network constants
const (
	SolanaMainnetRPC = "https://api.mainnet-beta.solana.com"
	SolanaDevnetRPC  = "https://api.devnet.solana.com"

	// WebSocket endpoints
	SolanaMainnetWS = "wss://api.mainnet-beta.solana.com"
	SolanaDevnetWS  = "wss://api.devnet.solana.com"

	// Solana constants
	LamportsPerSol = 1_000_000_000

	// Transaction constants
	MaxRetries        = 3
	RetryDelayMs      = 1000
	ConfirmTimeoutSec = 30
)

// pump.fun program addresses (updated with correct addresses)
var (
	// Main pump.fun program ID (verified)
	PumpFunProgramID = mustDecodeBase58("6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P")

	// Global account for pump.fun (verified)
	PumpFunGlobal = mustDecodeBase58("4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf")

	// Fee recipient (verified)
	PumpFunFeeRecipient = mustDecodeBase58("CebN5WGQ4jvEPvsVU4EoHEpgzq1VV7AbicfhtW4xC9iM")

	// Event authority (verified)
	PumpFunEventAuthority = mustDecodeBase58("Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1")

	// System program
	SystemProgramID = mustDecodeBase58("11111111111111111111111111111111")

	// Token program
	TokenProgramID = mustDecodeBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")

	// Associated Token program
	AssociatedTokenProgramID = mustDecodeBase58("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL")

	// Rent program
	RentProgramID = mustDecodeBase58("SysvarRent111111111111111111111111111111111")

	// Native SOL mint (wrapped SOL)
	NativeSOLMint = mustDecodeBase58("So11111111111111111111111111111111111111112")

	// Compute Budget Program ID
	ComputeBudgetProgramID = mustDecodeBase58("ComputeBudget111111111111111111111111111111")
)

// Trading constants
const (
	// Default slippage in basis points (1% = 100 bp)
	DefaultSlippageBP = 500 // 5%

	// Minimum SOL amount for trades (0.001 SOL)
	MinTradeAmountSOL = 0.0001

	// Maximum SOL amount for trades (1 SOL)
	MaxTradeAmountSOL = 0.1

	// Default buy amount in SOL
	DefaultBuyAmountSOL = 0.01

	// Pump.fun buy/sell instruction discriminators (first 8 bytes)
	BuyInstructionDiscriminator  = "66063d1201daebea" // buy
	SellInstructionDiscriminator = "33e685a4017f83ad" // sell
)

// Compute Budget instruction discriminators
const (
	// SetComputeUnitLimit instruction (instruction 2)
	SetComputeUnitLimitInstruction = 2

	// SetComputeUnitPrice instruction (instruction 3)
	SetComputeUnitPriceInstruction = 3
)

// Helper function to decode base58 addresses and panic on error
// Used for compile-time constant addresses that should never fail
func mustDecodeBase58(addr string) []byte {
	decoded, err := base58.Decode(addr)
	if err != nil {
		panic("Invalid base58 address: " + addr + ", error: " + err.Error())
	}
	return decoded
}

// GetRPCEndpoint returns RPC endpoint based on network
func GetRPCEndpoint(network string) string {
	switch network {
	case "mainnet":
		return SolanaMainnetRPC
	case "devnet":
		return SolanaDevnetRPC
	default:
		return SolanaMainnetRPC
	}
}

// GetWSEndpoint returns WebSocket endpoint based on network
func GetWSEndpoint(network string) string {
	switch network {
	case "mainnet":
		return SolanaMainnetWS
	case "devnet":
		return SolanaDevnetWS
	default:
		return SolanaMainnetWS
	}
}

// ConvertSOLToLamports converts SOL to lamports
func ConvertSOLToLamports(sol float64) uint64 {
	return uint64(sol * LamportsPerSol)
}

// ConvertLamportsToSOL converts lamports to SOL
func ConvertLamportsToSOL(lamports uint64) float64 {
	return float64(lamports) / LamportsPerSol
}

// GetComputeBudgetProgramID returns the Compute Budget Program ID
func GetComputeBudgetProgramID() []byte {
	return ComputeBudgetProgramID
}
