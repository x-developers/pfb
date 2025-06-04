package config

import "github.com/mr-tron/base58"

// Solana network constants
const (
	SolanaMainnetRPC = "https://api.mainnet-beta.solana.com"
	SolanaDevnetRPC  = "https://api.devnet.solana.com"

	// WebSocket endpoints
	SolanaMainnetWS = "wss://api.mainnet-beta.solana.com"
	SolanaDevnetWS  = "wss://api.devnet.solana.com"

	// Jito RPC endpoints
	JitoMainnetRPC = "https://mainnet.block-engine.jito.wtf/api/v1/transactions"
	JitoDevnetRPC  = "https://devnet.block-engine.jito.wtf/api/v1/transactions"

	// Jito bundle endpoints
	JitoMainnetBundle = "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
	JitoDevnetBundle  = "https://devnet.block-engine.jito.wtf/api/v1/bundles"

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

	// Jito tip accounts for mainnet
	JitoTipAccounts = [][]byte{
		mustDecodeBase58("96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5"),
		mustDecodeBase58("HFqU5x63VTqvQss8hp11i4wVV8bD44PvwucfZ2bU7gRe"),
		mustDecodeBase58("Cw8CFyM9FkoMi7K7Crf6HNQqf4uEMzpKw6QNghXLvLkY"),
		mustDecodeBase58("ADaUMid9yfUytqMBgopwjb2DTLSokTSzL1zt6iGPaS49"),
		mustDecodeBase58("DfXygSm4jCyNCybVYYK6DwvWqjKee8pbDmJGcLWNDXjh"),
		mustDecodeBase58("ADuUkR4vqLUMWXxW9gh6D6L8pMSawimctcNZ5pGwDcEt"),
		mustDecodeBase58("DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL"),
		mustDecodeBase58("3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6jT"),
	}
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

// GetJitoRPCEndpoint returns Jito RPC endpoint based on network
func GetJitoRPCEndpoint(network string) string {
	switch network {
	case "mainnet":
		return JitoMainnetRPC
	case "devnet":
		return JitoDevnetRPC
	default:
		return JitoMainnetRPC
	}
}

// GetJitoBundleEndpoint returns Jito bundle endpoint based on network
func GetJitoBundleEndpoint(network string) string {
	switch network {
	case "mainnet":
		return JitoMainnetBundle
	case "devnet":
		return JitoDevnetBundle
	default:
		return JitoMainnetBundle
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

// GetRandomJitoTipAccount returns a random Jito tip account for MEV protection
func GetRandomJitoTipAccount() []byte {
	if len(JitoTipAccounts) == 0 {
		return nil
	}
	// Simple rotation - in production you might want to use crypto/rand
	return JitoTipAccounts[0] // For now, use first account
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
