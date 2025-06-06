package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"log"
	"math"
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
	Mint              solana.PublicKey       `json:"mint"`
	ATAAddress        solana.PublicKey       `json:"ata_address"`
	Creator           solana.PublicKey       `json:"creator"`
	Balance           uint64                 `json:"balance"`
	UIAmount          float64                `json:"ui_amount"`
	Decimals          uint8                  `json:"decimals"`
	BondingCurve      solana.PublicKey       `json:"bonding_curve,omitempty"`
	AssocBondingCurve solana.PublicKey       `json:"assoc_bonding_curve,omitempty"`
	IsPumpFunToken    bool                   `json:"is_pump_fun_token"`
	MarketStats       map[string]interface{} `json:"market_stats,omitempty"`
}

// SellResult represents the result of selling a token with curve analysis
type SellResult struct {
	TokenInfo   TokenInfo `json:"token_info"`
	SellTx      string    `json:"sell_tx,omitempty"`
	CloseTx     string    `json:"close_tx,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	SOLReceived float64   `json:"sol_received,omitempty"`
	ATAClosed   bool      `json:"ata_closed"`

	// NEW: Curve-related data
	CurveCalculation *pumpfun.CurveCalculationResult `json:"curve_calculation,omitempty"`
	OptimalAmount    uint64                          `json:"optimal_amount,omitempty"`
	PriceImpact      float64                         `json:"price_impact,omitempty"`
	SlippageActual   float64                         `json:"slippage_actual,omitempty"`
	MarketCondition  string                          `json:"market_condition,omitempty"`
	SellStrategy     string                          `json:"sell_strategy,omitempty"`
	CurveStats       map[string]interface{}          `json:"curve_stats,omitempty"`
}

// WalletTokenManager manages all token operations with curve analysis
type WalletTokenManager struct {
	wallet       *wallet.Wallet
	rpcClient    *client.Client
	logger       *logger.Logger
	config       *config.Config
	curveManager *pumpfun.CurveManager // NEW: Curve manager integration
	ctx          context.Context

	// Rate limiting
	lastRequestTime time.Time
	requestDelay    time.Duration

	// Performance tracking
	totalProcessingTime time.Duration
	marketAnalysisTime  time.Duration
	transactionTime     time.Duration
}

func main() {
	fmt.Println("🎯 Advanced Wallet Token Manager - Curve-Aware Selling & Analysis")
	fmt.Println("================================================================")

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  go run sell_all_tokens.go <command> [options]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  list                    - List all tokens with curve analysis")
		fmt.Println("  analyze <mint>          - Deep curve analysis for specific token")
		fmt.Println("  sell-all [strategy]     - Sell all pump.fun tokens with strategy")
		fmt.Println("  sell <mint> [strategy]  - Sell specific token with strategy")
		fmt.Println("  simulate <mint>         - Simulate different sell amounts")
		fmt.Println("  close-empty-atas        - Close empty ATA accounts")
		fmt.Println("  emergency-close-all     - Emergency: close all ATAs (⚠️ will lose tokens!)")
		fmt.Println("")
		fmt.Println("Sell Strategies:")
		fmt.Println("  optimal     - Use curve analysis for optimal timing (default)")
		fmt.Println("  immediate   - Sell immediately regardless of market conditions")
		fmt.Println("  conservative - Conservative sell with minimal price impact")
		fmt.Println("  percentage  - Sell specified percentage (25%, 50%, 75%, 100%)")
		fmt.Println("")
		fmt.Println("Environment variables required:")
		fmt.Println("  PUMPBOT_PRIVATE_KEY    - Your wallet private key")
		fmt.Println("  PUMPBOT_NETWORK        - Network (mainnet/devnet)")
		fmt.Println("")
		fmt.Println("Optional environment variables:")
		fmt.Println("  PUMPBOT_REQUEST_DELAY_MS     - Delay between requests (default: 500ms)")
		fmt.Println("  PUMPBOT_MAX_PRICE_IMPACT     - Max acceptable price impact % (default: 5.0)")
		fmt.Println("  PUMPBOT_MIN_LIQUIDITY_SOL    - Min liquidity required (default: 1.0)")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  go run sell_all_tokens.go list")
		fmt.Println("  go run sell_all_tokens.go analyze 4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R")
		fmt.Println("  go run sell_all_tokens.go sell-all optimal")
		fmt.Println("  go run sell_all_tokens.go sell 4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R conservative")
		fmt.Println("  go run sell_all_tokens.go simulate 4k3Dyjzvzp8eMZWUXbBCjEvwSkkk59S5iCNLY3QrkX6R")
		os.Exit(1)
	}

	command := os.Args[1]
	strategy := "optimal"
	if len(os.Args) > 2 {
		strategy = os.Args[2]
	}

	// Initialize components
	manager, err := initializeManager()
	if err != nil {
		log.Fatalf("Failed to initialize manager: %v", err)
	}

	// Execute command
	switch command {
	case "list":
		err = manager.listTokensWithCurveAnalysis()
	case "analyze":
		if len(os.Args) < 3 {
			log.Fatal("Please provide mint address: go run sell_all_tokens.go analyze <mint_address>")
		}
		mintAddress := os.Args[2]
		err = manager.analyzeCurveConditions(mintAddress)
	case "sell-all":
		err = manager.sellAllTokensWithStrategy(strategy)
	case "sell":
		if len(os.Args) < 3 {
			log.Fatal("Please provide mint address: go run sell_all_tokens.go sell <mint_address> [strategy]")
		}
		mintAddress := os.Args[2]
		err = manager.sellSpecificTokenWithStrategy(mintAddress, strategy)
	case "simulate":
		if len(os.Args) < 3 {
			log.Fatal("Please provide mint address: go run sell_all_tokens.go simulate <mint_address>")
		}
		mintAddress := os.Args[2]
		err = manager.simulateSellScenarios(mintAddress)
	case "close-empty-atas":
		err = manager.closeEmptyATAs()
	case "emergency-close-all":
		fmt.Println("⚠️ WARNING: This will close ALL ATA accounts!")
		fmt.Println("⚠️ You will LOSE all tokens in these accounts!")
		fmt.Print("Type 'YES' to continue: ")
		var confirmation string
		_, err := fmt.Scanln(&confirmation)
		if err != nil {
			return
		}
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

	// Initialize config with curve-friendly settings
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

	// Configure request delay
	requestDelay := 500 * time.Millisecond // Default 500ms
	if delayStr := os.Getenv("PUMPBOT_REQUEST_DELAY_MS"); delayStr != "" {
		if delayMs, err := time.ParseDuration(delayStr + "ms"); err == nil {
			requestDelay = delayMs
		}
	}

	fmt.Printf("⏱️ Request delay: %v\n", requestDelay)
	fmt.Printf("🎯 Curve-aware selling enabled\n")

	return &WalletTokenManager{
		wallet:       walletInstance,
		rpcClient:    rpcClient,
		logger:       log,
		config:       cfg,
		curveManager: pumpfun.NewCurveManager(rpcClient, log), // Initialize curve manager
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

// NEW: Enhanced listing with curve analysis
func (wtm *WalletTokenManager) listTokensWithCurveAnalysis() error {
	fmt.Println("📋 Listing all tokens with curve analysis...")

	tokens, err := wtm.getAllTokensWithCurveData()
	if err != nil {
		return fmt.Errorf("failed to get tokens: %w", err)
	}

	if len(tokens) == 0 {
		fmt.Println("💭 No tokens found in wallet")
		return nil
	}

	fmt.Printf("Found %d tokens:\n\n", len(tokens))

	pumpFunCount := 0
	totalValue := 0.0

	for i, _token := range tokens {
		fmt.Printf("Token #%d:\n", i+1)
		fmt.Printf("  🪙 Mint: %s\n", _token.Mint.String())
		fmt.Printf("  📍 ATA Address: %s\n", _token.ATAAddress.String())
		fmt.Printf("  💰 Balance: %v tokens (%.6f UI)\n", _token.Balance, _token.UIAmount)
		fmt.Printf("  🔢 Decimals: %d\n", _token.Decimals)
		fmt.Printf("  🎯 Pump.fun Token: %v\n", _token.IsPumpFunToken)

		if _token.IsPumpFunToken {
			pumpFunCount++
			fmt.Printf("  📈 Bonding Curve: %s\n", _token.BondingCurve.String())
			fmt.Printf("  🔗 Assoc Bonding Curve: %s\n", _token.AssocBondingCurve.String())

			// Display market stats if available
			if _token.MarketStats != nil {
				fmt.Printf("  📊 Market Analysis:\n")
				if price, ok := _token.MarketStats["current_price_sol"].(float64); ok {
					fmt.Printf("    💱 Current Price: %.9f SOL\n", price)
				}
				if marketCap, ok := _token.MarketStats["market_cap_sol"].(float64); ok {
					fmt.Printf("    🏪 Market Cap: %.3f SOL\n", marketCap)
				}
				if liquidity, ok := _token.MarketStats["liquidity_sol"].(float64); ok {
					fmt.Printf("    🌊 Liquidity: %.3f SOL\n", liquidity)
				}
				if progress, ok := _token.MarketStats["curve_progress_percent"].(float64); ok {
					fmt.Printf("    📈 Curve Progress: %.1f%%\n", progress)
				}
				if complete, ok := _token.MarketStats["curve_complete"].(bool); ok && complete {
					fmt.Printf("    ✅ Curve Status: COMPLETED\n")
				}

				// Calculate estimated value
				if price, ok := _token.MarketStats["current_price_sol"].(float64); ok {
					estimatedValue := price * _token.UIAmount
					totalValue += estimatedValue
					fmt.Printf("    💵 Estimated Value: %.6f SOL\n", estimatedValue)
				}
			}
		}

		fmt.Println("")
	}

	// Enhanced summary with market analysis
	fmt.Printf("📊 Portfolio Summary:\n")
	fmt.Printf("  Total tokens: %d\n", len(tokens))
	fmt.Printf("  Pump.fun tokens: %d\n", pumpFunCount)
	fmt.Printf("  Other tokens: %d\n", len(tokens)-pumpFunCount)
	fmt.Printf("  Estimated total value: %.6f SOL\n", totalValue)

	// Show SOL balance
	balance, err := wtm.wallet.GetBalanceSOL(wtm.ctx)
	if err == nil {
		fmt.Printf("  SOL balance: %.6f SOL\n", balance)
		fmt.Printf("  Total portfolio: %.6f SOL\n", balance+totalValue)
	}

	return nil
}

// NEW: Deep curve analysis for specific token
func (wtm *WalletTokenManager) analyzeCurveConditions(mintAddress string) error {
	fmt.Printf("🎯 Deep Curve Analysis for Token: %s\n", mintAddress)
	fmt.Println("=====================================")

	mint, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return fmt.Errorf("invalid mint address: %w", err)
	}

	// Get token info
	token, err := wtm.getTokenInfo(mint)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if !token.IsPumpFunToken {
		return fmt.Errorf("token is not a pump.fun token")
	}

	fmt.Printf("📊 Basic Token Information:\n")
	fmt.Printf("  Balance: %v tokens (%.6f UI)\n", token.Balance, token.UIAmount)
	fmt.Printf("  Decimals: %d\n", token.Decimals)
	fmt.Println("")

	// Get comprehensive market stats
	fmt.Printf("📈 Market Analysis:\n")
	marketStats, err := wtm.curveManager.GetMarketStats(wtm.ctx, token.BondingCurve)
	if err != nil {
		return fmt.Errorf("failed to get market stats: %w", err)
	}

	for key, value := range marketStats {
		switch key {
		case "current_price_sol":
			fmt.Printf("  💱 Current Price: %.9f SOL\n", value)
		case "market_cap_sol":
			fmt.Printf("  🏪 Market Cap: %.3f SOL\n", value)
		case "liquidity_sol":
			fmt.Printf("  🌊 Liquidity: %.3f SOL\n", value)
		case "curve_progress_percent":
			fmt.Printf("  📈 Curve Progress: %.1f%%\n", value)
		case "curve_complete":
			if complete, ok := value.(bool); ok && complete {
				fmt.Printf("  ✅ Curve Status: COMPLETED\n")
			} else {
				fmt.Printf("  🔄 Curve Status: ACTIVE\n")
			}
		}
	}
	fmt.Println("")

	// Simulate different sell scenarios if we have balance
	if token.Balance > 0 {
		fmt.Printf("🎲 Sell Impact Simulation:\n")
		sellSimulations, err := wtm.curveManager.SimulateSellImpact(wtm.ctx, token.BondingCurve, token.Balance)
		if err != nil {
			fmt.Printf("  ❌ Failed to simulate sell impact: %v\n", err)
		} else {
			for percentage, result := range sellSimulations {
				fmt.Printf("  %s sell:\n", percentage)
				fmt.Printf("    💰 SOL received: %.6f SOL\n", config.ConvertLamportsToSOL(result.AmountOut))
				fmt.Printf("    💸 Fee paid: %.6f SOL\n", config.ConvertLamportsToSOL(result.Fee))
				fmt.Printf("    📊 Price impact: %.2f%%\n", result.PriceImpact)
				fmt.Printf("    💱 Effective price: %.9f SOL per token\n", result.PricePerToken/config.LamportsPerSol)
			}
		}
		fmt.Println("")

		// Recommend optimal strategy
		fmt.Printf("💡 Strategy Recommendation:\n")
		liquidity, _ := marketStats["liquidity_sol"].(float64)
		progress, _ := marketStats["curve_progress_percent"].(float64)

		if liquidity < 1.0 {
			fmt.Printf("  ⚠️ LOW LIQUIDITY: Consider waiting for more liquidity or selling smaller amounts\n")
		} else if progress > 80.0 {
			fmt.Printf("  🚀 HIGH PROGRESS: Curve near completion, consider selling before graduation\n")
		} else if liquidity > 10.0 && progress < 50.0 {
			fmt.Printf("  ✅ GOOD CONDITIONS: Healthy liquidity and early curve stage\n")
		} else {
			fmt.Printf("  📊 MODERATE CONDITIONS: Standard market conditions\n")
		}
	}

	return nil
}

// NEW: Simulate different sell scenarios
func (wtm *WalletTokenManager) simulateSellScenarios(mintAddress string) error {
	fmt.Printf("🎲 Simulating Sell Scenarios for: %s\n", mintAddress)
	fmt.Println("=====================================")

	mint, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return fmt.Errorf("invalid mint address: %w", err)
	}

	token, err := wtm.getTokenInfo(mint)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if !token.IsPumpFunToken {
		return fmt.Errorf("token is not a pump.fun token")
	}

	if token.Balance == 0 {
		fmt.Println("💭 Token balance is zero, nothing to simulate")
		return nil
	}

	// Comprehensive simulation
	fmt.Printf("Token: %.6f tokens\n", token.UIAmount)
	fmt.Println("")

	sellAmounts := []struct {
		name       string
		percentage float64
	}{
		{"Conservative", 10},
		{"Quarter", 25},
		{"Half", 50},
		{"Majority", 75},
		{"Complete", 100},
	}

	bestScenario := ""
	bestSOLPerToken := 0.0
	worstImpact := 0.0

	for _, scenario := range sellAmounts {
		sellAmount := uint64(float64(token.Balance) * (scenario.percentage / 100.0))

		result, err := wtm.curveManager.CalculateSellReturn(wtm.ctx, token.BondingCurve, sellAmount)
		if err != nil {
			fmt.Printf("❌ %s (%d%%): Failed to calculate - %v\n", scenario.name, int(scenario.percentage), err)
			continue
		}

		solReceived := config.ConvertLamportsToSOL(result.AmountOut)
		fee := config.ConvertLamportsToSOL(result.Fee)
		solPerToken := solReceived / (float64(sellAmount) / math.Pow(10, float64(token.Decimals)))

		fmt.Printf("📊 %s Sell (%d%%):\n", scenario.name, int(scenario.percentage))
		fmt.Printf("  🪙 Tokens to sell: %.6f\n", float64(sellAmount)/math.Pow(10, float64(token.Decimals)))
		fmt.Printf("  💰 SOL received: %.6f SOL\n", solReceived)
		fmt.Printf("  💸 Fee: %.6f SOL\n", fee)
		fmt.Printf("  📊 Price impact: %.2f%%\n", result.PriceImpact)
		fmt.Printf("  💱 SOL per token: %.9f\n", solPerToken)

		// Track best scenario
		if solPerToken > bestSOLPerToken {
			bestSOLPerToken = solPerToken
			bestScenario = scenario.name
		}
		if result.PriceImpact > worstImpact {
			worstImpact = result.PriceImpact
		}

		fmt.Println("")
	}

	// Summary and recommendations
	fmt.Printf("💡 Simulation Summary:\n")
	fmt.Printf("  🏆 Best price efficiency: %s (%.9f SOL per token)\n", bestScenario, bestSOLPerToken)
	fmt.Printf("  📊 Highest price impact: %.2f%%\n", worstImpact)

	if worstImpact > 10.0 {
		fmt.Printf("  ⚠️ WARNING: High price impact detected. Consider smaller sell amounts.\n")
	} else if worstImpact < 2.0 {
		fmt.Printf("  ✅ GOOD: Low price impact across all scenarios.\n")
	} else {
		fmt.Printf("  📊 MODERATE: Reasonable price impact levels.\n")
	}

	return nil
}

// NEW: Enhanced sell with strategy support
func (wtm *WalletTokenManager) sellAllTokensWithStrategy(strategy string) error {
	fmt.Printf("💰 Selling all pump.fun tokens with '%s' strategy...\n", strategy)

	tokens, err := wtm.getAllTokensWithCurveData()
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

	fmt.Printf("Found %d pump.fun tokens to sell using '%s' strategy\n\n", len(sellableTokens), strategy)

	results := make([]SellResult, 0)
	totalSOLReceived := 0.0

	for i, token := range sellableTokens {
		fmt.Printf("Selling token %d/%d: %s\n", i+1, len(sellableTokens), token.Mint.String())

		result := wtm.sellTokenWithStrategy(token, strategy)
		results = append(results, result)

		if result.Success {
			totalSOLReceived += result.SOLReceived
			fmt.Printf("✅ Success: %.6f SOL received", result.SOLReceived)
			if result.CurveCalculation != nil {
				fmt.Printf(" (%.2f%% impact)", result.CurveCalculation.PriceImpact)
			}
			fmt.Println()
		} else {
			fmt.Printf("❌ Failed: %s\n", result.Error)
		}

		fmt.Println("")

		// Longer delay between sell transactions to avoid rate limits
		time.Sleep(3 * time.Second)
	}

	// Enhanced summary with curve analysis
	fmt.Printf("📊 Final Summary:\n")
	fmt.Printf("  Strategy used: %s\n", strategy)
	fmt.Printf("  Tokens processed: %d\n", len(results))

	successCount := 0
	totalPriceImpact := 0.0
	for _, result := range results {
		if result.Success {
			successCount++
			if result.CurveCalculation != nil {
				totalPriceImpact += result.CurveCalculation.PriceImpact
			}
		}
	}

	fmt.Printf("  Successful sales: %d\n", successCount)
	fmt.Printf("  Failed sales: %d\n", len(results)-successCount)
	fmt.Printf("  Total SOL received: %.6f SOL\n", totalSOLReceived)
	if successCount > 0 {
		fmt.Printf("  Average price impact: %.2f%%\n", totalPriceImpact/float64(successCount))
	}

	// Save enhanced results to file
	enhancedResults := map[string]interface{}{
		"timestamp":            time.Now(),
		"strategy":             strategy,
		"total_tokens":         len(results),
		"successful_sales":     successCount,
		"total_sol_received":   totalSOLReceived,
		"average_price_impact": totalPriceImpact / float64(successCount),
		"results":              results,
	}

	resultsJSON, _ := json.MarshalIndent(enhancedResults, "", "  ")
	filename := fmt.Sprintf("sell_results_curve_%s_%d.json", strategy, time.Now().Unix())
	os.WriteFile(filename, resultsJSON, 0644)
	fmt.Printf("  Enhanced results saved to: %s\n", filename)

	return nil
}

// NEW: Sell specific token with strategy
func (wtm *WalletTokenManager) sellSpecificTokenWithStrategy(mintAddress, strategy string) error {
	fmt.Printf("💰 Selling token %s with '%s' strategy\n", mintAddress, strategy)

	mint, err := solana.PublicKeyFromBase58(mintAddress)
	if err != nil {
		return fmt.Errorf("invalid mint address: %w", err)
	}

	// Get token info with curve data
	tokenInfo, err := wtm.getTokenInfoWithCurveData(mint)
	if err != nil {
		return fmt.Errorf("failed to get token info: %w", err)
	}

	if tokenInfo.Balance == 0 {
		fmt.Println("💭 Token balance is zero, nothing to sell")
		return nil
	}

	if !tokenInfo.IsPumpFunToken {
		return fmt.Errorf("token is not a pump.fun token, cannot sell using pump.fun contract")
	}

	fmt.Printf("Token balance: %v tokens (%.6f UI)\n", tokenInfo.Balance, tokenInfo.UIAmount)

	result := wtm.sellTokenWithStrategy(tokenInfo, strategy)

	if result.Success {
		fmt.Printf("✅ Successfully sold token using '%s' strategy!\n", strategy)
		fmt.Printf("  SOL received: %.6f SOL\n", result.SOLReceived)
		fmt.Printf("  Sell transaction: %s\n", result.SellTx)
		if result.CurveCalculation != nil {
			fmt.Printf("  Price impact: %.2f%%\n", result.CurveCalculation.PriceImpact)
			fmt.Printf("  Curve fee: %.6f SOL\n", config.ConvertLamportsToSOL(result.CurveCalculation.Fee))
		}
		if result.ATAClosed {
			fmt.Printf("  ATA close transaction: %s\n", result.CloseTx)
		}
	} else {
		fmt.Printf("❌ Failed to sell token: %s\n", result.Error)
	}

	return nil
}

// NEW: Enhanced token selling with curve-aware strategies
func (wtm *WalletTokenManager) sellTokenWithStrategy(token TokenInfo, strategy string) SellResult {
	start := time.Now()

	result := SellResult{
		TokenInfo:    token,
		Success:      false,
		SellStrategy: strategy,
	}

	if !token.IsPumpFunToken {
		result.Error = "not a pump.fun token"
		return result
	}

	if token.Balance == 0 {
		result.Error = "zero balance"
		return result
	}

	wtm.logger.WithFields(logrus.Fields{
		"mint":     token.Mint,
		"strategy": strategy,
		"balance":  token.UIAmount,
	}).Info("🎯 Executing curve-aware token sell")

	// Analyze market conditions first
	marketAnalysisStart := time.Now()
	marketStats, err := wtm.curveManager.GetMarketStats(wtm.ctx, token.BondingCurve)
	if err != nil {
		result.Error = fmt.Sprintf("failed to analyze market conditions: %v", err)
		return result
	}
	wtm.marketAnalysisTime += time.Since(marketAnalysisStart)

	result.CurveStats = marketStats

	// Determine sell amount based on strategy
	sellAmount, sellPercentage, err := wtm.calculateSellAmountByStrategy(token, strategy, marketStats)
	if err != nil {
		result.Error = fmt.Sprintf("failed to calculate sell amount: %v", err)
		return result
	}

	result.OptimalAmount = sellAmount

	// Get curve calculation for the sell
	curveCalc, err := wtm.curveManager.CalculateSellReturn(wtm.ctx, token.BondingCurve, sellAmount)
	if err != nil {
		result.Error = fmt.Sprintf("failed to calculate curve return: %v", err)
		return result
	}

	result.CurveCalculation = curveCalc
	result.PriceImpact = curveCalc.PriceImpact

	// Validate sell based on strategy
	if !wtm.validateSellByStrategy(strategy, curveCalc, marketStats) {
		result.Error = fmt.Sprintf("sell validation failed for strategy '%s': price impact %.2f%% too high", strategy, curveCalc.PriceImpact)
		result.MarketCondition = "unfavorable"
		return result
	}

	result.MarketCondition = "favorable"

	// Execute the sell transaction
	transactionStart := time.Now()
	success := wtm.executeCurveAwareSell(token, sellAmount, curveCalc, &result)
	wtm.transactionTime += time.Since(transactionStart)

	if success {
		result.Success = true
		result.SOLReceived = config.ConvertLamportsToSOL(curveCalc.AmountOut)

		// Try to close ATA if selling 100%
		if sellPercentage >= 100.0 {
			closeSignature, err := wtm.closeATA(token.ATAAddress)
			if err != nil {
				wtm.logger.WithError(err).Debug("Failed to close ATA after sell")
			} else {
				result.CloseTx = closeSignature
				result.ATAClosed = true
			}
		}
	}

	wtm.totalProcessingTime += time.Since(start)
	return result
}

// NEW: Calculate sell amount based on strategy
func (wtm *WalletTokenManager) calculateSellAmountByStrategy(token TokenInfo, strategy string, marketStats map[string]interface{}) (uint64, float64, error) {
	switch strategy {
	case "immediate":
		return token.Balance, 100.0, nil // Sell everything immediately

	case "conservative":
		// Sell smaller amount to minimize price impact
		liquidity, _ := marketStats["liquidity_sol"].(float64)
		if liquidity < 2.0 {
			return uint64(float64(token.Balance) * 0.25), 25.0, nil // 25% if low liquidity
		}
		return uint64(float64(token.Balance) * 0.50), 50.0, nil // 50% normally

	case "optimal":
		// Use curve manager to find optimal amount
		optimalAmount, _, err := wtm.curveManager.EstimateOptimalSellAmount(wtm.ctx, token.BondingCurve, token.Balance, 100.0)
		if err != nil {
			return token.Balance, 100.0, nil // Fallback to full amount
		}
		percentage := (float64(optimalAmount) / float64(token.Balance)) * 100.0
		return optimalAmount, percentage, nil

	case "percentage":
		// Default to 75% for percentage strategy
		return uint64(float64(token.Balance) * 0.75), 75.0, nil

	default:
		return token.Balance, 100.0, fmt.Errorf("unknown strategy: %s", strategy)
	}
}

// NEW: Validate sell based on strategy requirements
func (wtm *WalletTokenManager) validateSellByStrategy(strategy string, curveCalc *pumpfun.CurveCalculationResult, marketStats map[string]interface{}) bool {
	maxPriceImpact := 5.0 // Default 5%
	if envImpact := os.Getenv("PUMPBOT_MAX_PRICE_IMPACT"); envImpact != "" {
		_, err := fmt.Sscanf(envImpact, "%f", &maxPriceImpact)
		if err != nil {
			return false
		}
	}

	switch strategy {
	case "immediate":
		return true // Always execute regardless of conditions

	case "conservative":
		return curveCalc.PriceImpact <= 2.0 // Very strict price impact limit

	case "optimal":
		liquidity, _ := marketStats["liquidity_sol"].(float64)
		minLiquidity := 1.0
		if envLiq := os.Getenv("PUMPBOT_MIN_LIQUIDITY_SOL"); envLiq != "" {
			_, err := fmt.Sscanf(envLiq, "%f", &minLiquidity)
			if err != nil {
				return false
			}
		}

		return curveCalc.PriceImpact <= maxPriceImpact && liquidity >= minLiquidity

	default:
		return curveCalc.PriceImpact <= maxPriceImpact
	}
}

// NEW: Execute curve-aware sell with enhanced validation
func (wtm *WalletTokenManager) executeCurveAwareSell(token TokenInfo, sellAmount uint64, curveCalc *pumpfun.CurveCalculationResult, result *SellResult) bool {
	// Create TokenEvent for the sell instruction
	tokenEvent := &pumpfun.TokenEvent{
		Mint:                   token.Mint,
		BondingCurve:           token.BondingCurve,
		AssociatedBondingCurve: token.AssocBondingCurve,
	}

	// Get bonding curve account data to extract the creator
	bondingCurveInfo, err := wtm.rpcClient.GetAccountInfo(wtm.ctx, token.BondingCurve.String())
	if err != nil {
		result.Error = fmt.Sprintf("failed to get bonding curve info: %v", err)
		return false
	}

	if bondingCurveInfo == nil || bondingCurveInfo.Value == nil || len(bondingCurveInfo.Value.Data.GetBinary()) < 40 {
		result.Error = "invalid bonding curve data"
		return false
	}

	// Derive creator vault PDA using creator public key
	pumpFunProgram := solana.PublicKeyFromBytes(config.PumpFunProgramID)
	creatorVaultSeeds := [][]byte{
		[]byte("creator-vault"),
		token.Creator.Bytes(),
	}
	creatorVault, _, err := solana.FindProgramAddress(creatorVaultSeeds, pumpFunProgram)
	if err != nil {
		result.Error = fmt.Sprintf("failed to derive creator vault: %v", err)
		return false
	}
	tokenEvent.CreatorVault = creatorVault
	// Use curve calculation for minimum SOL output with slippage protection
	slippageFactor := 1.0 - float64(wtm.config.Trading.SlippageBP)/10000.0
	minSolOutput := uint64(float64(curveCalc.AmountOut) * slippageFactor)

	wtm.logger.WithFields(logrus.Fields{
		"sell_amount":    sellAmount,
		"expected_sol":   curveCalc.AmountOut,
		"min_sol_output": minSolOutput,
		"price_impact":   curveCalc.PriceImpact,
		"curve_fee":      curveCalc.Fee,
	}).Info("📊 Executing curve-optimized sell")

	// Create sell instruction
	sellInstruction := pumpfun.CreatePumpFunSellInstruction(
		tokenEvent,
		token.ATAAddress,
		wtm.wallet.GetPublicKey(),
		sellAmount,
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
		return false
	}

	// Create and sign transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{sellInstruction},
		blockhash,
		solana.TransactionPayer(wtm.wallet.GetPublicKey()),
	)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create transaction: %v", err)
		return false
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
		return false
	}

	// Send transaction with rate limiting
	var signature solana.Signature
	err = wtm.rateLimitedRequest(func() error {
		signature, err = wtm.rpcClient.SendAndConfirmTransaction(wtm.ctx, tx)
		return err
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to send transaction: %v", err)
		return false
	}

	result.SellTx = signature.String()
	return true
}

// Enhanced methods that include curve analysis
func (wtm *WalletTokenManager) getAllTokensWithCurveData() ([]TokenInfo, error) {
	fmt.Println("🔍 Getting token accounts with curve analysis (with rate limiting)...")

	tokens, err := wtm.getAllTokens()
	if err != nil {
		return nil, err
	}

	// Add curve analysis for pump.fun tokens
	for i := range tokens {
		if tokens[i].IsPumpFunToken {
			marketStats, err := wtm.curveManager.GetMarketStats(wtm.ctx, tokens[i].BondingCurve)
			if err != nil {
				wtm.logger.WithError(err).Debug("Failed to get market stats for token")
			} else {
				tokens[i].MarketStats = marketStats
			}

			// Small delay to avoid overwhelming the RPC
			time.Sleep(wtm.requestDelay / 2)
		}
	}

	return tokens, nil
}

func (wtm *WalletTokenManager) getTokenInfoWithCurveData(mint solana.PublicKey) (TokenInfo, error) {
	tokenInfo, err := wtm.getTokenInfo(mint)
	if err != nil {
		return TokenInfo{}, err
	}

	if tokenInfo.IsPumpFunToken {
		marketStats, err := wtm.curveManager.GetMarketStats(wtm.ctx, tokenInfo.BondingCurve)
		if err != nil {
			wtm.logger.WithError(err).Debug("Failed to get market stats for tokenInfo")
		} else {
			tokenInfo.MarketStats = marketStats
		}
	}

	return tokenInfo, nil
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

				curveData := bondingCurveInfo.Value.Data.GetBinary()
				creatorOffset := 8 + 8 + 8 + 8 + 8 + 8 + 1 // Skip to creator field
				if len(curveData) >= creatorOffset+32 {
					tokenInfo.Creator = solana.PublicKeyFromBytes(curveData[creatorOffset : creatorOffset+32])
				} else {
					// Fallback: try different offset or use a default approach
					// Sometimes the structure might be different, so we try another common offset
					if len(curveData) >= 64 {
						tokenInfo.Creator = solana.PublicKeyFromBytes(curveData[32:64])
					}
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
