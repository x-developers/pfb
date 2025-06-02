// scripts/test_jito.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
)

func main() {
	fmt.Println("🛡️ Testing Jito MEV Protection")
	fmt.Println("==============================")

	// Create logger
	logConfig := logger.LogConfig{
		Level:  "debug",
		Format: "text",
	}

	testLogger, err := logger.NewLogger(logConfig)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// Determine network and endpoint
	network := "mainnet"
	jitoEndpoint := "https://mainnet.block-engine.jito.wtf/api/v1/bundles"

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "devnet":
			network = "devnet"
			// Note: Jito primarily operates on mainnet
			fmt.Println("⚠️ Warning: Jito primarily operates on mainnet")
		case "frankfurt":
			jitoEndpoint = "https://frankfurt.mainnet.block-engine.jito.wtf/api/v1/bundles"
		case "amsterdam":
			jitoEndpoint = "https://amsterdam.mainnet.block-engine.jito.wtf/api/v1/bundles"
		case "ny":
			jitoEndpoint = "https://ny.mainnet.block-engine.jito.wtf/api/v1/bundles"
		case "tokyo":
			jitoEndpoint = "https://tokyo.mainnet.block-engine.jito.wtf/api/v1/bundles"
		}
	}

	fmt.Printf("Network: %s\n", network)
	fmt.Printf("Jito Endpoint: %s\n", jitoEndpoint)
	fmt.Println()

	// Test 1: Jito Client Connection
	fmt.Println("1️⃣ Testing Jito Client Connection...")
	jitoClient := solana.NewJitoClient(solana.JitoClientConfig{
		Endpoint: jitoEndpoint,
		Timeout:  10 * time.Second,
	}, testLogger.Logger)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test health check
	err = jitoClient.HealthCheck(ctx)
	if err != nil {
		fmt.Printf("❌ Jito health check failed: %v\n", err)
		fmt.Println("This might be normal if:")
		fmt.Println("- Jito service is temporarily unavailable")
		fmt.Println("- Network connectivity issues")
		fmt.Println("- Endpoint requires authentication")
	} else {
		fmt.Println("✅ Jito health check passed!")
	}

	// Test 2: Get Tip Accounts
	fmt.Println("\n2️⃣ Testing Tip Accounts Retrieval...")
	tipAccounts, err := jitoClient.GetTipAccounts(ctx)
	if err != nil {
		fmt.Printf("❌ Failed to get tip accounts: %v\n", err)
	} else {
		fmt.Printf("✅ Retrieved %d tip accounts\n", len(tipAccounts))
		if len(tipAccounts) > 0 {
			fmt.Printf("Sample tip account: %s\n", tipAccounts[0])
		}
	}

	// Test 3: Bundle Cost Estimation
	fmt.Println("\n3️⃣ Testing Bundle Cost Estimation...")
	testCosts := []struct {
		txCount int
		tip     uint64
	}{
		{1, 10000}, // Single transaction, 0.00001 SOL tip
		{2, 15000}, // Two transactions, 0.000015 SOL tip
		{3, 25000}, // Three transactions, 0.000025 SOL tip
	}

	for _, test := range testCosts {
		cost := jitoClient.EstimateBundleCost(test.txCount, test.tip)
		costSOL := float64(cost) / config.LamportsPerSol
		fmt.Printf("📊 %d tx + %d lamports tip = %d total lamports (%.6f SOL)\n",
			test.txCount, test.tip, cost, costSOL)
	}

	// Test 4: Random Tip Account Selection
	fmt.Println("\n4️⃣ Testing Random Tip Account Selection...")
	if len(tipAccounts) > 0 {
		randomTip, err := jitoClient.GetRandomTipAccount(ctx)
		if err != nil {
			fmt.Printf("❌ Failed to get random tip account: %v\n", err)
		} else {
			fmt.Printf("✅ Random tip account: %s\n", randomTip)
		}
	} else {
		fmt.Println("⚠️ No tip accounts available for random selection")
	}

	// Test 5: Performance Test
	fmt.Println("\n5️⃣ Performance Testing...")
	fmt.Println("Testing response times for different operations...")

	operations := []struct {
		name string
		fn   func() error
	}{
		{
			"Health Check",
			func() error {
				return jitoClient.HealthCheck(ctx)
			},
		},
		{
			"Get Tip Accounts",
			func() error {
				_, err := jitoClient.GetTipAccounts(ctx)
				return err
			},
		},
		{
			"Get Random Tip Account",
			func() error {
				_, err := jitoClient.GetRandomTipAccount(ctx)
				return err
			},
		},
	}

	for _, op := range operations {
		start := time.Now()
		err := op.fn()
		duration := time.Since(start)

		status := "✅"
		if err != nil {
			status = "❌"
		}

		fmt.Printf("%s %s: %v\n", status, op.name, duration)
	}

	// Test 6: Bundle Status Simulation
	fmt.Println("\n6️⃣ Testing Bundle Status Queries...")
	// Generate a fake bundle ID for testing the API
	fakeBundleID := "test-bundle-id-" + fmt.Sprintf("%d", time.Now().Unix())

	fmt.Printf("Testing with fake bundle ID: %s\n", fakeBundleID)
	status, err := jitoClient.GetBundleStatus(ctx, fakeBundleID)
	if err != nil {
		fmt.Printf("✅ Expected error for fake bundle ID: %v\n", err)
	} else {
		fmt.Printf("⚠️ Unexpected success for fake bundle ID: %+v\n", status)
	}

	// Test 7: Regional Performance Comparison
	fmt.Println("\n7️⃣ Regional Endpoint Performance Comparison...")
	endpoints := map[string]string{
		"Global":    "https://mainnet.block-engine.jito.wtf/api/v1/bundles",
		"Frankfurt": "https://frankfurt.mainnet.block-engine.jito.wtf/api/v1/bundles",
		"Amsterdam": "https://amsterdam.mainnet.block-engine.jito.wtf/api/v1/bundles",
		"New York":  "https://ny.mainnet.block-engine.jito.wtf/api/v1/bundles",
		"Tokyo":     "https://tokyo.mainnet.block-engine.jito.wtf/api/v1/bundles",
	}

	fmt.Println("Testing response times for different regional endpoints:")

	for region, endpoint := range endpoints {
		testClient := solana.NewJitoClient(solana.JitoClientConfig{
			Endpoint: endpoint,
			Timeout:  5 * time.Second,
		}, testLogger.Logger)

		start := time.Now()
		err := testClient.HealthCheck(ctx)
		duration := time.Since(start)

		status := "✅"
		if err != nil {
			status = "❌"
		}

		fmt.Printf("%s %-10s: %8v (%s)\n", status, region, duration, endpoint)
	}

	// Test 8: Configuration Validation
	fmt.Println("\n8️⃣ Configuration Validation...")

	// Test different tip amounts
	tipAmounts := []uint64{5000, 10000, 15000, 25000, 50000}
	fmt.Println("Recommended tip amounts based on use case:")

	for _, tip := range tipAmounts {
		tipSOL := float64(tip) / config.LamportsPerSol
		var useCase string

		switch {
		case tip <= 5000:
			useCase = "Low congestion / Testing"
		case tip <= 15000:
			useCase = "Normal trading"
		case tip <= 25000:
			useCase = "High-frequency trading"
		case tip <= 50000:
			useCase = "Critical transactions"
		default:
			useCase = "Maximum protection"
		}

		fmt.Printf("💰 %5d lamports (%.6f SOL) - %s\n", tip, tipSOL, useCase)
	}

	// Summary
	fmt.Println("\n📊 Test Summary")
	fmt.Println("================")
	fmt.Printf("Network: %s\n", network)
	fmt.Printf("Primary Endpoint: %s\n", jitoEndpoint)

	if len(tipAccounts) > 0 {
		fmt.Printf("✅ Jito service is accessible\n")
		fmt.Printf("✅ %d tip accounts available\n", len(tipAccounts))
		fmt.Printf("🛡️ MEV protection is ready to use\n")
	} else {
		fmt.Printf("⚠️ Jito service may not be fully accessible\n")
		fmt.Printf("❌ No tip accounts retrieved\n")
		fmt.Printf("🚫 MEV protection may not work properly\n")
	}

	fmt.Println("\n💡 Recommendations:")
	fmt.Println("- Choose the fastest regional endpoint for your location")
	fmt.Println("- Start with 10000-15000 lamports tip for normal trading")
	fmt.Println("- Enable fallback to regular transactions")
	fmt.Println("- Monitor bundle success rates and adjust tips accordingly")
	fmt.Println("- Test on devnet first, then use small amounts on mainnet")

	if len(tipAccounts) > 0 {
		fmt.Println("\n🚀 Ready to use Jito! Try:")
		fmt.Println("./build/pump-fun-bot --network mainnet --jito --jito-tip 15000 --dry-run")
	}

	fmt.Println("\n🛡️ Jito testing completed!")
}
