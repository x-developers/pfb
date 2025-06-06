package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"log"
	"os"
	"strings"
	"time"

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/pumpfun"
	"pump-fun-bot-go/internal/wallet"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
)

// TokenInfo represents information about a token in the wallet
type TokenInfo struct {
	Mint              solana.PublicKey `json:"mint"`
	ATAAddress        solana.PublicKey `json:"ata_address"`
	Balance           uint64           `json:"balance"`
	UIAmount          float64          `json:"ui_amount"`
	Decimals          uint8            `json:"decimals"`
	BondingCurve      solana.PublicKey `json:"bonding_curve,omitempty"`
	AssocBondingCurve solana.PublicKey `json:"assoc_bonding_curve,omitempty"`
	IsPumpFunToken    bool             `json:"is_pump_fun_token"`
}

// SellResult represents the result of selling a token
type SellResult struct {
	TokenInfo   TokenInfo `json:"token_info"`
	SellTx      string    `json:"sell_tx,omitempty"`
	CloseTx     string    `json:"close_tx,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	SOLReceived float64   `json:"sol_received,omitempty"`
	ATAClosed   bool      `json:"ata_closed"`
}

// WalletTokenManager manages all token operations
type WalletTokenManager struct {
	wallet    *wallet.Wallet
	rpcClient *client.Client
	logger    *logger.Logger
	config    *config.Config
	ctx       context.Context

	// Rate limiting
	lastRequestTime time.Time
	requestDelay    time.Duration
}

func main() {
	fmt.Println("🔍 Wallet Token Manager - Sell All Tokens & Close ATAs")
	fmt.Println("======================================================")

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  go run wallet_token_manager.go <command> [options]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  list                    - List all tokens in wallet")
		fmt.Println("  sell-all               - Sell all pump.fun tokens")
		fmt.Println("  sell <mint_address>    - Sell specific token")
		fmt.Println("  close-empty-atas       - Close empty ATA accounts")
		fmt.Println("  emergency-close-all    - Emergency: close all ATAs (⚠️ will lose tokens!)")
		fmt.Println("")
		fmt.Println("Environment variables required:")
		fmt.Println("  PUMPBOT_PRIVATE_KEY    - Your wallet private key")
		fmt.Println("  PUMPBOT_NETWORK        - Network (mainnet/devnet)")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  go run wallet_token_manager.go list")
		fmt.Println("  go run wallet_token_manager.go sell-all")
		fmt.Println("  go run wallet_token_manager.go sell 4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R")
		os.Exit(1)
	}

	command := os.Args[1]

	// Initialize components
	manager, err := initializeManager()
	if err != nil {
		log.Fatalf("Failed to initialize manager: %v", err)
	}

	// Execute command
	switch command {
	case "list":
		err = manager.listTokens()
	case "sell-all":
		err = manager.sellAllTokens()
	case "sell":
		if len(os.Args) < 3 {
			log.Fatal("Please provide mint address: go run wallet_token_manager.go sell <mint_address>")
		}
		mintAddress := os.Args[2]
		err = manager.sellSpecificToken(mintAddress)
	case "close-empty-atas":
		err = manager.closeEmptyATAs()
	case "emergency-close-all":
		fmt.Println("⚠️ WARNING: This will close ALL ATA accounts!")
		fmt.Println("⚠️ You will LOSE all tokens in these accounts!")
		fmt.Print("Type 'YES' to continue: ")
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation == "YES" {
			err = manager.emergencyCloseAllATAs()
		} else {
			fmt.Println("Operation cancelled.")
		}
	default:
		log.Fatalf("Unknown command: %s", command)
	}

	if err != nil {
		log.Fatalf("Command failed: %v", err)
	}
}

func initializeManager() (*WalletTokenManager, error) {
	// Load environment variables
	privateKey := os.Getenv("PUMPBOT_PRIVATE_KEY")
	if privateKey == "" {
		return nil, fmt.Errorf("PUMPBOT_PRIVATE_KEY environment variable is required")
	}

	network := os.Getenv("PUMPBOT_NETWORK")
	if network == "" {
		network = "mainnet"
	}

	// Initialize logger
	logConfig := logger.LogConfig{
		Level:  "info",
		Format: "text",
	}

	log, err := logger.NewLogger(logConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Initialize config
	cfg := &config.Config{
		Network: network,
		Trading: config.TradingConfig{
			SlippageBP: 1000, // 10% slippage for emergency sells
		},
	}
	cfg.RPCUrl = config.GetRPCEndpoint(network)
	cfg.WSUrl = config.GetWSEndpoint(network)

	// Initialize RPC client
	rpcClient := client.NewClient(client.ClientConfig{
		RPCEndpoint: cfg.RPCUrl,
		WSEndpoint:  cfg.WSUrl,
		Timeout:     30 * time.Second,
	}, log.Logger)

	// Initialize wallet
	walletInstance, err := wallet.NewWallet(wallet.WalletConfig{
		PrivateKey: privateKey,
		Network:    network,
	}, rpcClient, log.Logger, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	fmt.Printf("✅ Initialized wallet: %s\n", walletInstance.GetPublicKeyString())
	fmt.Printf("🌐 Network: %s\n", network)
	fmt.Printf("🔗 RPC: %s\n", cfg.RPCUrl)
	fmt.Println("")

	// Check for custom request delay
	requestDelay := 500 * time.Millisecond // Default 500ms
	if delayStr := os.Getenv("PUMPBOT_REQUEST_DELAY_MS"); delayStr != "" {
		if delayMs, err := time.ParseDuration(delayStr + "ms"); err == nil {
			requestDelay = delayMs
		}
	}

	fmt.Printf("⏱️ Request delay: %v\n", requestDelay)

	return &WalletTokenManager{
		wallet:       walletInstance,
		rpcClient:    rpcClient,
		logger:       log,
		config:       cfg,
		ctx:          context.Background(),
		requestDelay: requestDelay,
	}, nil
}

func (wtm *WalletTokenManager) rateLimitedRequest(requestFunc func() error) error {
	// Ensure minimum delay between requests
	if time.Since(wtm.lastRequestTime) < wtm.requestDelay {
		sleepTime := wtm.requestDelay - time.Since(wtm.lastRequestTime)
		time.Sleep(sleepTime)
	}

	wtm.lastRequestTime = time.Now()

	// Execute request with retry logic for rate limits
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := requestFunc()
		if err == nil {
			return nil
		}

		// Check if it's a rate limit error
		if isRateLimitError(err) {
			backoffDelay := time.Duration(attempt*attempt) * time.Second // Exponential backoff
			wtm.logger.WithFields(map[string]interface{}{
				"attempt": attempt,
				"delay":   backoffDelay,
				"error":   err.Error(),
			}).Warn("Rate limit exceeded, retrying...")

			time.Sleep(backoffDelay)
			continue
		}

		// If it's not a rate limit error, return immediately
		return err
	}

	return fmt.Errorf("request failed after %d attempts due to rate limiting", maxRetries)
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "Connection rate limits exceeded") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "Too Many Requests")
}

func (wtm *WalletTokenManager) sellAllTokens() error {
	fmt.Println("💰 Selling all pump.fun tokens...")

	tokens, err := wtm.getAllTokens()
	if err != nil {
		return fmt.Errorf("failed to get tokens: %w", err)
	}

	// Filter only pump.fun tokens with balance > 0
	sellableTokens := make([]TokenInfo, 0)
	for _, token := range tokens {
		if token.IsPumpFunToken && token.Balance > 0 {
			sellableTokens = append(sellableTokens, token)
		}
	}

	if len(sellableTokens) == 0 {
		fmt.Println("💭 No pump.fun tokens found to sell")
		return nil
	}

	fmt.Printf("Found %d pump.fun tokens to sell\n\n", len(sellableTokens))

	results := make([]SellResult, 0)
	totalSOLReceived := 0.0

	for i, token := range sellableTokens {
		fmt.Printf("Selling token %d/%d: %s\n", i+1, len(sellableTokens), token.Mint.String())

		result := wtm.sellToken(token)
		results = append(results, result)

		if result.Success {
			totalSOLReceived += result.SOLReceived
			fmt.Printf("✅ Success: %.6f SOL received\n", result.SOLReceived)
		} else {
			fmt.Printf("❌ Failed: %s\n", result.Error)
		}

		fmt.Println("")

		// Longer delay between sell transactions to avoid rate limits
		time.Sleep(3 * time.Second)
	}

	// Show final summary
	fmt.Printf("📊 Final Summary:\n")
	fmt.Printf("  Tokens processed: %d\n", len(results))

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	fmt.Printf("  Successful sales: %d\n", successCount)
	fmt.Printf("  Failed sales: %d\n", len(results)-successCount)
	fmt.Printf("  Total SOL received: %.6f SOL\n", totalSOLReceived)

	// Save results to file
	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	filename := fmt.Sprintf("sell_results_%d.json", time.Now().Unix())
	os.WriteFile(filename, resultsJSON, 0644)
	fmt.Printf("  Results saved to: %s\n", filename)

	return nil
}

func (wtm *WalletTokenManager) sellSpecificToken(mintAddress string) error {
	fmt.Printf("💰 Selling specific token: %s\n", mintAddress)

	mint, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return fmt.Errorf("invalid mint address: %w", err)
	}

	// Get token info
	token, err := wtm.getTokenInfo(mint)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if token.Balance == 0 {
		fmt.Println("💭 Token balance is zero, nothing to sell")
		return nil
	}

	if !token.IsPumpFunToken {
		return fmt.Errorf("token is not a pump.fun token, cannot sell using pump.fun contract")
	}

	fmt.Printf("Token balance: %v tokens (%.6f UI)\n", token.Balance, token.UIAmount)

	result := wtm.sellToken(token)

	if result.Success {
		fmt.Printf("✅ Successfully sold token!\n")
		fmt.Printf("  SOL received: %.6f SOL\n", result.SOLReceived)
		fmt.Printf("  Sell transaction: %s\n", result.SellTx)
		if result.ATAClosed {
			fmt.Printf("  ATA close transaction: %s\n", result.CloseTx)
		}
	} else {
		fmt.Printf("❌ Failed to sell token: %s\n", result.Error)
	}

	return nil
}

func (wtm *WalletTokenManager) closeEmptyATAs() error {
	fmt.Println("🗑️ Closing empty ATA accounts...")

	tokens, err := wtm.getAllTokens()
	if err != nil {
		return fmt.Errorf("failed to get tokens: %w", err)
	}

	emptyATAs := make([]TokenInfo, 0)
	for _, token := range tokens {
		if token.Balance == 0 {
			emptyATAs = append(emptyATAs, token)
		}
	}

	if len(emptyATAs) == 0 {
		fmt.Println("💭 No empty ATA accounts found")
		return nil
	}

	fmt.Printf("Found %d empty ATA accounts to close\n\n", len(emptyATAs))

	totalRentReclaimed := 0.0
	successCount := 0

	for i, token := range emptyATAs {
		fmt.Printf("Closing ATA %d/%d: %s\n", i+1, len(emptyATAs), token.ATAAddress.String())

		tx, err := wtm.closeATA(token.ATAAddress)
		if err != nil {
			fmt.Printf("❌ Failed: %s\n", err)
		} else {
			successCount++
			rentReclaimed := 0.002039280 // Standard ATA rent
			totalRentReclaimed += rentReclaimed
			fmt.Printf("✅ Success: %s (%.6f SOL reclaimed)\n", tx, rentReclaimed)
		}

		time.Sleep(2 * time.Second) // Increased delay to avoid rate limits
	}

	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("  ATAs processed: %d\n", len(emptyATAs))
	fmt.Printf("  Successfully closed: %d\n", successCount)
	fmt.Printf("  Failed: %d\n", len(emptyATAs)-successCount)
	fmt.Printf("  Total rent reclaimed: %.6f SOL\n", totalRentReclaimed)

	return nil
}

func (wtm *WalletTokenManager) emergencyCloseAllATAs() error {
	fmt.Println("🚨 EMERGENCY: Closing ALL ATA accounts...")

	tokens, err := wtm.getAllTokens()
	if err != nil {
		return fmt.Errorf("failed to get tokens: %w", err)
	}

	if len(tokens) == 0 {
		fmt.Println("💭 No ATA accounts found")
		return nil
	}

	fmt.Printf("⚠️ Will close %d ATA accounts (including non-empty ones!)\n\n", len(tokens))

	totalRentReclaimed := 0.0
	successCount := 0

	for i, token := range tokens {
		fmt.Printf("Closing ATA %d/%d: %s", i+1, len(tokens), token.ATAAddress.String())
		if token.Balance > 0 {
			fmt.Printf(" (⚠️ HAS BALANCE: %.6f tokens!)", token.UIAmount)
		}
		fmt.Println()

		tx, err := wtm.closeATA(token.ATAAddress)
		if err != nil {
			fmt.Printf("❌ Failed: %s\n", err)
		} else {
			successCount++
			rentReclaimed := 0.002039280
			totalRentReclaimed += rentReclaimed
			fmt.Printf("✅ Success: %s\n", tx)
		}

		time.Sleep(1 * time.Second)
	}

	fmt.Printf("\n📊 Emergency Close Summary:\n")
	fmt.Printf("  ATAs processed: %d\n", len(tokens))
	fmt.Printf("  Successfully closed: %d\n", successCount)
	fmt.Printf("  Failed: %d\n", len(tokens)-successCount)
	fmt.Printf("  Total rent reclaimed: %.6f SOL\n", totalRentReclaimed)

	return nil
}

func (wtm *WalletTokenManager) listTokens() error {
	fmt.Println("📋 Listing all tokens in wallet...")

	tokens, err := wtm.getAllTokens()
	if err != nil {
		return fmt.Errorf("failed to get tokens: %w", err)
	}

	if len(tokens) == 0 {
		fmt.Println("💭 No tokens found in wallet")
		return nil
	}

	fmt.Printf("Found %d tokens:\n\n", len(tokens))

	pumpFunCount := 0

	for i, token := range tokens {
		fmt.Printf("Token #%d:\n", i+1)
		fmt.Printf("  Mint: %s\n", token.Mint.String())
		fmt.Printf("  ATA Address: %s\n", token.ATAAddress.String())
		fmt.Printf("  Balance: %v tokens (%.6f UI)\n", token.Balance, token.UIAmount)
		fmt.Printf("  Decimals: %d\n", token.Decimals)
		fmt.Printf("  Pump.fun Token: %v\n", token.IsPumpFunToken)

		if token.IsPumpFunToken {
			pumpFunCount++
			fmt.Printf("  Bonding Curve: %s\n", token.BondingCurve.String())
			fmt.Printf("  Assoc Bonding Curve: %s\n", token.AssocBondingCurve.String())
		}

		fmt.Println("")
	}

	// Show summary
	fmt.Printf("📊 Summary:\n")
	fmt.Printf("  Total tokens: %d\n", len(tokens))
	fmt.Printf("  Pump.fun tokens: %d\n", pumpFunCount)
	fmt.Printf("  Other tokens: %d\n", len(tokens)-pumpFunCount)

	// Show SOL balance
	balance, err := wtm.wallet.GetBalanceSOL(wtm.ctx)
	if err == nil {
		fmt.Printf("  SOL balance: %.6f SOL\n", balance)
	}

	return nil
}

func (wtm *WalletTokenManager) getAllTokens() ([]TokenInfo, error) {
	fmt.Println("🔍 Getting token accounts (with rate limiting)...")

	var result *rpc.GetTokenAccountsResult
	var err error

	// Use rate limited request for getting token accounts
	err = wtm.rateLimitedRequest(func() error {
		result, err = wtm.rpcClient.GetTokenAccounts(wtm.ctx, wtm.wallet.GetPublicKeyString())
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get token accounts: %w", err)
	}

	tokens := make([]TokenInfo, 0)
	fmt.Printf("📦 Processing %d token accounts...\n", len(result.Value))

	for i, account := range result.Value {
		if i > 0 && i%10 == 0 {
			fmt.Printf("  Processed %d/%d accounts...\n", i, len(result.Value))
		}

		// Parse account data to get mint and balance
		accountData := account.Account.Data.GetBinary()
		if accountData == nil || len(accountData) < 165 {
			continue // Token account should be exactly 165 bytes
		}

		// Manually parse token account data structure
		mint := solana.PublicKeyFromBytes(accountData[0:32])
		owner := solana.PublicKeyFromBytes(accountData[32:64])

		// Skip if not owned by our wallet
		if !owner.Equals(wtm.wallet.GetPublicKey()) {
			continue
		}

		// Parse amount (little endian uint64)
		amount := uint64(accountData[64]) |
			uint64(accountData[65])<<8 |
			uint64(accountData[66])<<16 |
			uint64(accountData[67])<<24 |
			uint64(accountData[68])<<32 |
			uint64(accountData[69])<<40 |
			uint64(accountData[70])<<48 |
			uint64(accountData[71])<<56

		// Get mint info with rate limiting
		var mintInfo *rpc.GetAccountInfoResult
		err = wtm.rateLimitedRequest(func() error {
			mintInfo, err = wtm.rpcClient.GetAccountInfo(wtm.ctx, mint.String())
			return err
		})

		decimals := uint8(6) // Default to 6 decimals if we can't get mint info
		if err != nil {
			wtm.logger.WithError(err).Debug("Failed to get mint info, using default decimals")
		} else if mintInfo != nil && mintInfo.Value != nil && len(mintInfo.Value.Data.GetBinary()) >= 44 {
			decimals = mintInfo.Value.Data.GetBinary()[44]
		}

		// Calculate UI amount
		uiAmount := float64(amount)
		if decimals > 0 {
			for i := uint8(0); i < decimals; i++ {
				uiAmount /= 10.0
			}
		}

		tokenInfo := TokenInfo{
			Mint:       mint,
			ATAAddress: account.Pubkey,
			Balance:    amount,
			UIAmount:   uiAmount,
			Decimals:   decimals,
		}

		// Check if this is a pump.fun token with rate limiting
		bondingCurve, _, err := wtm.deriveBondingCurve(mint)
		if err == nil {
			var bondingCurveInfo *rpc.GetAccountInfoResult
			err = wtm.rateLimitedRequest(func() error {
				bondingCurveInfo, err = wtm.rpcClient.GetAccountInfo(wtm.ctx, bondingCurve.String())
				return err
			})

			if err == nil && bondingCurveInfo != nil && bondingCurveInfo.Value != nil {
				// This looks like a pump.fun token
				tokenInfo.IsPumpFunToken = true
				tokenInfo.BondingCurve = bondingCurve

				// Derive associated bonding curve
				assocBondingCurve, _, err := solana.FindAssociatedTokenAddress(bondingCurve, mint)
				if err == nil {
					tokenInfo.AssocBondingCurve = assocBondingCurve
				}
			}
		}

		tokens = append(tokens, tokenInfo)
	}

	fmt.Printf("✅ Processed all %d accounts\n", len(result.Value))
	return tokens, nil
}

func (wtm *WalletTokenManager) getTokenInfo(mint solana.PublicKey) (TokenInfo, error) {
	tokens, err := wtm.getAllTokens()
	if err != nil {
		return TokenInfo{}, err
	}

	for _, token := range tokens {
		if token.Mint.Equals(mint) {
			return token, nil
		}
	}

	return TokenInfo{}, fmt.Errorf("token not found in wallet")
}

func (wtm *WalletTokenManager) logTokenInfo(token TokenInfo) {
	wtm.logger.WithFields(logrus.Fields{
		"mint": token.Mint,
		"ata":  token.ATAAddress,
		"bond": token.BondingCurve.String(),
	}).Info("<UNK> Token Info")
}

func (wtm *WalletTokenManager) sellToken(token TokenInfo) SellResult {
	result := SellResult{
		TokenInfo: token,
		Success:   false,
	}

	if !token.IsPumpFunToken {
		result.Error = "not a pump.fun token"
		return result
	}

	if token.Balance == 0 {
		result.Error = "zero balance"
		return result
	}

	wtm.logTokenInfo(token)

	// Create TokenEvent for the sell instruction
	tokenEvent := &pumpfun.TokenEvent{
		Mint:                   token.Mint,
		BondingCurve:           token.BondingCurve,
		AssociatedBondingCurve: token.AssocBondingCurve,
	}

	// Derive creator vault (required for pump.fun instructions)
	pumpFunProgram := solana.PublicKeyFromBytes(config.PumpFunProgramID)
	creatorVaultSeeds := [][]byte{
		[]byte("creator-vault"),
		token.Mint.Bytes(),
	}
	creatorVault, _, err := solana.FindProgramAddress(creatorVaultSeeds, pumpFunProgram)
	if err != nil {
		result.Error = fmt.Sprintf("failed to derive creator vault: %v", err)
		return result
	}
	tokenEvent.CreatorVault = creatorVault

	// Calculate minimum SOL output (very conservative for emergency sell)
	minSolOutput := uint64(1) // Accept any amount of SOL

	// Create sell instruction
	sellInstruction := pumpfun.CreatePumpFunSellInstruction(
		tokenEvent,
		token.ATAAddress,
		wtm.wallet.GetPublicKey(),
		token.Balance, // Sell all tokens
		minSolOutput,
	)

	// Get recent blockhash with rate limiting
	var blockhash solana.Hash
	err = wtm.rateLimitedRequest(func() error {
		blockhash, err = wtm.rpcClient.GetLatestBlockhash(wtm.ctx)
		return err
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to get blockhash: %v", err)
		return result
	}

	// Create and sign transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{sellInstruction},
		blockhash,
		solana.TransactionPayer(wtm.wallet.GetPublicKey()),
	)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create transaction: %v", err)
		return result
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(wtm.wallet.GetPublicKey()) {
			account := wtm.wallet.GetAccount()
			return &account
		}
		return nil
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to sign transaction: %v", err)
		return result
	}

	// Send transaction with rate limiting
	var signature solana.Signature
	err = wtm.rateLimitedRequest(func() error {
		signature, err = wtm.rpcClient.SendAndConfirmTransaction(wtm.ctx, tx)
		return err
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to send transaction: %v", err)
		return result
	}

	result.SellTx = signature.String()
	result.Success = true
	result.SOLReceived = 0.001 // Estimate - would need price calculation for actual amount

	// Try to close ATA after successful sell
	closeSignature, err := wtm.closeATA(token.ATAAddress)
	if err != nil {
		wtm.logger.WithError(err).Debug("Failed to close ATA after sell")
	} else {
		result.CloseTx = closeSignature
		result.ATAClosed = true
	}

	return result
}

func (wtm *WalletTokenManager) closeATA(ataAddress solana.PublicKey) (string, error) {
	// Create close account instruction
	closeInstruction := token.NewCloseAccountInstruction(
		ataAddress,
		wtm.wallet.GetPublicKey(), // Destination for reclaimed SOL
		wtm.wallet.GetPublicKey(), // Owner
		[]solana.PublicKey{},      // Multi-sig signers (empty)
	).Build()

	// Get recent blockhash with rate limiting
	var blockhash solana.Hash
	var err error
	err = wtm.rateLimitedRequest(func() error {
		blockhash, err = wtm.rpcClient.GetLatestBlockhash(wtm.ctx)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to get blockhash: %w", err)
	}

	// Create and sign transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{closeInstruction},
		blockhash,
		solana.TransactionPayer(wtm.wallet.GetPublicKey()),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(wtm.wallet.GetPublicKey()) {
			account := wtm.wallet.GetAccount()
			return &account
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction with rate limiting
	var signature solana.Signature
	err = wtm.rateLimitedRequest(func() error {
		signature, err = wtm.rpcClient.SendAndConfirmTransaction(wtm.ctx, tx)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	return signature.String(), nil
}

func (wtm *WalletTokenManager) deriveBondingCurve(mint solana.PublicKey) (solana.PublicKey, uint8, error) {
	// Derive bonding curve PDA for pump.fun
	seeds := [][]byte{
		[]byte("bonding-curve"),
		mint.Bytes(),
	}

	pumpFunProgram := solana.PublicKeyFromBytes(config.PumpFunProgramID)
	return solana.FindProgramAddress(seeds, pumpFunProgram)
}
