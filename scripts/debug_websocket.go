package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"

	"github.com/mr-tron/base58"
)

func main() {
	fmt.Println("🔍 Debugging WebSocket Connection to Solana")
	fmt.Println("==========================================")

	// Create logger
	logConfig := logger.LogConfig{
		Level:  "debug",
		Format: "text",
	}

	testLogger, err := logger.NewLogger(logConfig)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	// Determine network
	network := "devnet"
	if len(os.Args) > 1 && os.Args[1] == "mainnet" {
		network = "mainnet"
	}

	wsUrl := config.GetWSEndpoint(network)
	rpcUrl := config.GetRPCEndpoint(network)

	fmt.Printf("Network: %s\n", network)
	fmt.Printf("WebSocket URL: %s\n", wsUrl)
	fmt.Printf("RPC URL: %s\n", rpcUrl)
	fmt.Println()

	// Test RPC connection first
	fmt.Println("1️⃣ Testing RPC Connection...")
	rpcClient := client.NewClient(client.ClientConfig{
		Endpoint: rpcUrl,
		Timeout:  30 * time.Second,
	}, testLogger.Logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	slot, err := rpcClient.GetSlot(ctx)
	cancel()

	if err != nil {
		log.Fatalf("❌ RPC connection failed: %v", err)
	}

	fmt.Printf("✅ RPC connection successful! Current slot: %d\n\n", slot)

	// Test WebSocket connection
	fmt.Println("2️⃣ Testing WebSocket Connection...")
	wsClient := client.NewWSClient(wsUrl, testLogger.Logger)

	err = wsClient.Connect()
	if err != nil {
		log.Fatalf("❌ WebSocket connection failed: %v", err)
	}

	fmt.Println("✅ WebSocket connected successfully!")
	fmt.Println()

	// Test pump.fun program logs subscription
	fmt.Println("3️⃣ Testing pump.fun Program Logs Subscription...")
	pumpFunProgramID := base58.Encode(config.PumpFunProgramID)
	fmt.Printf("pump.fun Program ID: %s\n", pumpFunProgramID)

	// Counter for received notifications
	notificationCount := 0
	logsCount := 0

	// Create handler for logs
	logHandler := func(data interface{}) error {
		notificationCount++

		notification, ok := data.(client.LogsNotification)
		if !ok {
			fmt.Printf("❌ Invalid notification format: %T\n", data)
			return fmt.Errorf("invalid notification format")
		}

		signature := notification.Result.Value.Signature
		logs := notification.Result.Value.Logs
		slot := notification.Result.Context.Slot
		hasError := notification.Result.Value.Err != nil

		logsCount += len(logs)

		fmt.Printf("📨 Notification #%d: Signature=%s, Slot=%d, Logs=%d, Error=%v\n",
			notificationCount, signature[:8]+"...", slot, len(logs), hasError)

		// Check if any log mentions pump.fun or create
		for _, logEntry := range logs {
			if contains(logEntry, []string{"create", "Create", "pump", "Program log"}) {
				fmt.Printf("  🎯 Interesting log: %s\n", logEntry)
			}
		}

		return nil
	}

	subscriptionID, err := wsClient.SubscribeToLogs(pumpFunProgramID, logHandler)
	if err != nil {
		log.Fatalf("❌ Failed to subscribe to logs: %v", err)
	}

	fmt.Printf("✅ Subscribed to pump.fun logs! Subscription ID: %d\n", subscriptionID)
	fmt.Println()

	// Wait for notifications
	fmt.Println("4️⃣ Listening for notifications... (Press Ctrl+C to stop)")
	fmt.Println("⏳ Waiting for pump.fun transactions...")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Periodic status updates
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	start := time.Now()

	for {
		select {
		case <-sigChan:
			fmt.Println("\n🛑 Shutting down...")

			elapsed := time.Since(start)
			fmt.Printf("📊 Statistics:\n")
			fmt.Printf("  - Running time: %v\n", elapsed)
			fmt.Printf("  - Total notifications: %d\n", notificationCount)
			fmt.Printf("  - Total log entries: %d\n", logsCount)
			fmt.Printf("  - Rate: %.2f notifications/min\n", float64(notificationCount)/(elapsed.Minutes()))

			wsClient.Disconnect()
			return

		case <-ticker.C:
			elapsed := time.Since(start)
			rate := float64(notificationCount) / elapsed.Minutes()
			fmt.Printf("📊 Status: %d notifications in %v (%.2f/min)\n",
				notificationCount, elapsed.Truncate(time.Second), rate)

			if notificationCount == 0 {
				fmt.Println("⚠️  No notifications received yet. This could mean:")
				fmt.Println("   - Low activity on pump.fun")
				fmt.Println("   - WebSocket subscription not working")
				fmt.Println("   - Wrong program ID")
			}
		}
	}
}

// Helper function to check if string contains any of the given substrings
func contains(s string, substrings []string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
