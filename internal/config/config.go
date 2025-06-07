package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	// Network settings
	Network   string `mapstructure:"network" yaml:"network"`
	RPCUrl    string `mapstructure:"rpc_url" yaml:"rpc_url"`
	WSUrl     string `mapstructure:"ws_url" yaml:"ws_url"`
	RPCAPIKey string `mapstructure:"rpc_api_key" yaml:"rpc_api_key"`

	Listener ListenerConfig `mapstructure:"listener" yaml:"listener"`

	// Wallet settings
	PrivateKey string `mapstructure:"private_key" yaml:"private_key"`

	// Trading settings
	Trading TradingConfig `mapstructure:"trading" yaml:"trading"`

	// Strategy settings
	Strategy StrategyConfig `mapstructure:"strategy" yaml:"strategy"`

	// Logging settings
	Logging LoggingConfig `mapstructure:"logging" yaml:"logging"`

	// Advanced settings
	Advanced AdvancedConfig `mapstructure:"advanced" yaml:"advanced"`

	// Ultra Fast Mode settings
	UltraFast UltraFastConfig `mapstructure:"ultra_fast" yaml:"ultra_fast"`

	// Jito settings
	Jito JitoConfig `mapstructure:"jito" yaml:"jito"`
}

type ListenerType string

const (
	LogsListenerType   ListenerType = "logs"
	BlocksListenerType ListenerType = "blocks"
	MultiListenerType  ListenerType = "multi"
)

// JitoConfig contains Jito-related settings
type JitoConfig struct {
	Enabled        bool   `mapstructure:"enabled" yaml:"enabled"`
	BlockEngineURL string `mapstructure:"block_engine_url" yaml:"block_engine_url"`
	APIKey         string `mapstructure:"api_key" yaml:"api_key"`
	Timeout        int    `mapstructure:"timeout_ms" yaml:"timeout_ms"`

	// Advanced settings
	FallbackEnabled     bool `mapstructure:"fallback_enabled" yaml:"fallback_enabled"`
	MaxBundleSize       int  `mapstructure:"max_bundle_size" yaml:"max_bundle_size"`
	BundleRetryAttempts int  `mapstructure:"bundle_retry_attempts" yaml:"bundle_retry_attempts"`

	// MEV Protection
	TipLamports    uint64  `mapstructure:"tip_lamports" yaml:"tip_lamports"`
	TipPercent     float64 `mapstructure:"tip_percent" yaml:"tip_percent"`
	UseRandomTip   bool    `mapstructure:"use_random_tip" yaml:"use_random_tip"`
	MinTipLamports uint64  `mapstructure:"min_tip_lamports" yaml:"min_tip_lamports"`
	MaxTipLamports uint64  `mapstructure:"max_tip_lamports" yaml:"max_tip_lamports"`
}

// ListenerConfig contains listener-specific configuration
type ListenerConfig struct {
	// Listener type selection
	Type ListenerType `mapstructure:"type" yaml:"type"`

	// Buffer size for token events
	BufferSize int `mapstructure:"buffer_size" yaml:"buffer_size"`

	// Multi-listener configuration
	EnableLogListener   bool `mapstructure:"enable_log_listener" yaml:"enable_log_listener"`
	EnableBlockListener bool `mapstructure:"enable_block_listener" yaml:"enable_block_listener"`

	// Performance settings
	ParallelProcessing bool `mapstructure:"parallel_processing" yaml:"parallel_processing"`
	WorkerCount        int  `mapstructure:"worker_count" yaml:"worker_count"`

	// Filtering options
	EnableDuplicateFilter bool `mapstructure:"enable_duplicate_filter" yaml:"enable_duplicate_filter"`
	DuplicateFilterTTL    int  `mapstructure:"duplicate_filter_ttl" yaml:"duplicate_filter_ttl"` // seconds

	// Debug options
	LogRawEvents bool `mapstructure:"log_raw_events" yaml:"log_raw_events"`
	LogTiming    bool `mapstructure:"log_timing" yaml:"log_timing"`
}

// TradingConfig contains trading-related settings
type TradingConfig struct {
	BuyAmountSOL    float64 `mapstructure:"buy_amount_sol" yaml:"buy_amount_sol"`
	BuyAmountTokens uint64  `mapstructure:"buy_amount_tokens" yaml:"buy_amount_tokens"`
	UseTokenAmount  bool    `mapstructure:"use_token_amount" yaml:"use_token_amount"`
	SlippageBP      int     `mapstructure:"slippage_bp" yaml:"slippage_bp"`
	MaxGasPrice     uint64  `mapstructure:"max_gas_price" yaml:"max_gas_price"`
	PriorityFee     uint64  `mapstructure:"priority_fee" yaml:"priority_fee"`

	// Auto-sell settings
	AutoSell          bool    `mapstructure:"auto_sell" yaml:"auto_sell"`
	SellDelayMs       int64   `mapstructure:"sell_delay_ms" yaml:"sell_delay_ms"` // CHANGED: now in milliseconds
	SellPercentage    float64 `mapstructure:"sell_percentage" yaml:"sell_percentage"`
	CloseATAAfterSell bool    `mapstructure:"close_ata_after_sell" yaml:"close_ata_after_sell"`

	// Existing fields
	TakeProfitPercent   float64 `mapstructure:"take_profit_percent" yaml:"take_profit_percent"`
	StopLossPercent     float64 `mapstructure:"stop_loss_percent" yaml:"stop_loss_percent"`
	MaxTokenAgeMs       int64   `mapstructure:"max_token_age_ms" yaml:"max_token_age_ms"`
	MinDiscoveryDelayMs int64   `mapstructure:"min_discovery_delay_ms" yaml:"min_discovery_delay_ms"`
}

// StrategyConfig contains strategy-related settings
type StrategyConfig struct {
	Type             string   `mapstructure:"type" yaml:"type"`
	FilterByCreator  bool     `mapstructure:"filter_by_creator" yaml:"filter_by_creator"`
	AllowedCreators  []string `mapstructure:"allowed_creators" yaml:"allowed_creators"`
	FilterByName     bool     `mapstructure:"filter_by_name" yaml:"filter_by_name"`
	NamePatterns     []string `mapstructure:"name_patterns" yaml:"name_patterns"`
	MinLiquiditySOL  float64  `mapstructure:"min_liquidity_sol" yaml:"min_liquidity_sol"`
	MaxTokensPerHour int64    `mapstructure:"max_tokens_per_hour" yaml:"max_tokens_per_hour"`
	YoloMode         bool     `mapstructure:"yolo_mode" yaml:"yolo_mode"`
	HoldOnly         bool     `mapstructure:"hold_only" yaml:"hold_only"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level       string `mapstructure:"level" yaml:"level"`
	Format      string `mapstructure:"format" yaml:"format"`
	LogToFile   bool   `mapstructure:"log_to_file" yaml:"log_to_file"`
	LogFilePath string `mapstructure:"log_file_path" yaml:"log_file_path"`
	TradeLogDir string `mapstructure:"trade_log_dir" yaml:"trade_log_dir"`
}

// AdvancedConfig contains advanced settings
type AdvancedConfig struct {
	MaxRetries        int  `mapstructure:"max_retries" yaml:"max_retries"`
	RetryDelayMs      int  `mapstructure:"retry_delay_ms" yaml:"retry_delay_ms"`
	ConfirmTimeoutSec int  `mapstructure:"confirm_timeout_sec" yaml:"confirm_timeout_sec"`
	EnableMetrics     bool `mapstructure:"enable_metrics" yaml:"enable_metrics"`
	MetricsPort       int  `mapstructure:"metrics_port" yaml:"metrics_port"`
}

// UltraFastConfig contains ultra-fast mode settings for maximum speed
type UltraFastConfig struct {
	// Core ultra-fast settings
	Enabled        bool `mapstructure:"enabled" yaml:"enabled"`
	SkipValidation bool `mapstructure:"skip_validation" yaml:"skip_validation"`
	NoConfirmation bool `mapstructure:"no_confirmation" yaml:"no_confirmation"`
	FireAndForget  bool `mapstructure:"fire_and_forget" yaml:"fire_and_forget"`

	// Parallel processing
	ParallelWorkers int `mapstructure:"parallel_workers" yaml:"parallel_workers"`
	TokenQueueSize  int `mapstructure:"token_queue_size" yaml:"token_queue_size"`

	// Caching and pre-computation
	CacheBlockhash           bool `mapstructure:"cache_blockhash" yaml:"cache_blockhash"`
	BlockhashRefreshInterval int  `mapstructure:"blockhash_refresh_interval" yaml:"blockhash_refresh_interval"`
	PrecomputeInstructions   bool `mapstructure:"precompute_instructions" yaml:"precompute_instructions"`
	ReuseTransactionBuffers  bool `mapstructure:"reuse_buffers" yaml:"reuse_buffers"`

	// Network optimizations
	MultiplexRPCEndpoints bool     `mapstructure:"multiplex_rpc" yaml:"multiplex_rpc"`
	RPCEndpoints          []string `mapstructure:"rpc_endpoints" yaml:"rpc_endpoints"`
	UseFastestRPC         bool     `mapstructure:"use_fastest_rpc" yaml:"use_fastest_rpc"`
	RPCTimeout            int      `mapstructure:"rpc_timeout_ms" yaml:"rpc_timeout_ms"`

	// Performance monitoring
	BenchmarkMode bool `mapstructure:"benchmark_mode" yaml:"benchmark_mode"`
	LogLatency    bool `mapstructure:"log_latency" yaml:"log_latency"`
	ProfileMemory bool `mapstructure:"profile_memory" yaml:"profile_memory"`

	// Risk management (ultra-fast specific)
	MaxTokensPerSecond int     `mapstructure:"max_tokens_per_second" yaml:"max_tokens_per_second"`
	EmergencyStopLoss  float64 `mapstructure:"emergency_stop_loss" yaml:"emergency_stop_loss"`
	FixedBuyAmount     bool    `mapstructure:"fixed_buy_amount" yaml:"fixed_buy_amount"`

	// Advanced optimizations
	SkipATACreation    bool `mapstructure:"skip_ata_creation" yaml:"skip_ata_creation"`
	AssumeATAExists    bool `mapstructure:"assume_ata_exists" yaml:"assume_ata_exists"`
	ImmediateExecution bool `mapstructure:"immediate_execution" yaml:"immediate_execution"`
	PriorityOverSafety bool `mapstructure:"priority_over_safety" yaml:"priority_over_safety"`
}

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string, envPath string) (*Config, error) {
	config := &Config{}

	// First, load .env file if specified or default locations
	if err := loadEnvFile(envPath); err != nil {
		fmt.Printf("Warning: Failed to load .env file: %v\n", err)
	}

	// Set default values
	setDefaults()
	setListenerDefaults()
	setDefaultsUltraFast()
	setJitoDefaults()

	// Set config file path
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		// Look for config in current directory and common config directories
		viper.SetConfigName("bot")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath("$HOME/.pump-fun-bot")
		viper.AddConfigPath("/etc/pump-fun-bot/")
	}

	// Enable reading from environment variables
	viper.AutomaticEnv()
	viper.SetEnvPrefix("PUMPBOT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Manually bind environment variables that viper might miss
	bindEnvVariables()
	bindListenerEnvVariables()
	bindUltraFastEnvVariables()
	bindJitoEnvVariables()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, continue with defaults and env vars
		fmt.Printf("Config file not found, using environment variables and defaults\n")
	} else {
		fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
	}

	// Process environment variable substitution in config values
	err := processEnvSubstitution()
	if err != nil {
		return nil, fmt.Errorf("failed to process environment variables: %w", err)
	}

	// Override with direct environment variable reads to ensure they're applied
	overrideWithEnvVars()

	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate and post-process config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	if err := validateListenerConfig(config); err != nil {
		return nil, fmt.Errorf("listener validation failed: %w", err)
	}

	return config, nil
}

// Rest of the methods remain the same but remove all Jito-related functionality...

// Updated bindEnvVariables without Jito
func bindEnvVariables() {
	// Top-level variables
	viper.BindEnv("network", "PUMPBOT_NETWORK")
	viper.BindEnv("rpc_url", "PUMPBOT_RPC_URL")
	viper.BindEnv("ws_url", "PUMPBOT_WS_URL")
	viper.BindEnv("rpc_api_key", "PUMPBOT_RPC_API_KEY")
	viper.BindEnv("private_key", "PUMPBOT_PRIVATE_KEY")

	viper.BindEnv("trading.buy_amount_sol", "PUMPBOT_TRADING_BUY_AMOUNT_SOL")
	viper.BindEnv("trading.buy_amount_tokens", "PUMPBOT_TRADING_BUY_AMOUNT_TOKENS")
	viper.BindEnv("trading.use_token_amount", "PUMPBOT_TRADING_USE_TOKEN_AMOUNT")
	viper.BindEnv("trading.slippage_bp", "PUMPBOT_TRADING_SLIPPAGE_BP")
	viper.BindEnv("trading.priority_fee", "PUMPBOT_TRADING_PRIORITY_FEE")
	viper.BindEnv("trading.auto_sell", "PUMPBOT_TRADING_AUTO_SELL")
	viper.BindEnv("trading.sell_delay_ms", "PUMPBOT_TRADING_SELL_DELAY_MS")
	viper.BindEnv("trading.sell_percentage", "PUMPBOT_TRADING_SELL_PERCENTAGE")
	viper.BindEnv("trading.close_ata_after_sell", "PUMPBOT_TRADING_CLOSE_ATA_AFTER_SELL")

	viper.BindEnv("trading.max_token_age_ms", "PUMPBOT_TRADING_MAX_TOKEN_AGE_MS")
	viper.BindEnv("trading.min_discovery_delay_ms", "PUMPBOT_TRADING_MIN_DISCOVERY_DELAY_MS")

	// Strategy variables
	viper.BindEnv("strategy.type", "PUMPBOT_STRATEGY_TYPE")
	viper.BindEnv("strategy.yolo_mode", "PUMPBOT_STRATEGY_YOLO_MODE")
	viper.BindEnv("strategy.hold_only", "PUMPBOT_STRATEGY_HOLD_ONLY")
	viper.BindEnv("strategy.max_tokens_per_hour", "PUMPBOT_STRATEGY_MAX_TOKENS_PER_HOUR")

	// Logging variables
	viper.BindEnv("logging.level", "PUMPBOT_LOGGING_LEVEL")
	viper.BindEnv("logging.format", "PUMPBOT_LOGGING_FORMAT")
	viper.BindEnv("logging.log_to_file", "PUMPBOT_LOGGING_LOG_TO_FILE")
}

// Rest of the functions remain the same, just remove all Jito-related parts...
// (I'll include key functions but remove all Jito references)

// setDefaults sets default configuration values
func setDefaults() {
	// Network defaults
	viper.SetDefault("network", "mainnet")
	viper.SetDefault("rpc_url", "")
	viper.SetDefault("ws_url", "")

	viper.SetDefault("trading.buy_amount_sol", DefaultBuyAmountSOL)
	viper.SetDefault("trading.buy_amount_tokens", 1000000)
	viper.SetDefault("trading.use_token_amount", true)
	viper.SetDefault("trading.slippage_bp", DefaultSlippageBP)
	viper.SetDefault("trading.max_gas_price", 0)
	viper.SetDefault("trading.priority_fee", 0)

	viper.SetDefault("trading.auto_sell", true)
	viper.SetDefault("trading.sell_delay_ms", 1000)
	viper.SetDefault("trading.sell_percentage", 100.0)
	viper.SetDefault("trading.close_ata_after_sell", true)

	viper.SetDefault("trading.take_profit_percent", 50.0)
	viper.SetDefault("trading.stop_loss_percent", -20.0)
	viper.SetDefault("trading.max_token_age_ms", 5000)
	viper.SetDefault("trading.min_discovery_delay_ms", 100)

	// Strategy defaults
	viper.SetDefault("strategy.type", "sniper")
	viper.SetDefault("strategy.filter_by_creator", false)
	viper.SetDefault("strategy.filter_by_name", false)
	viper.SetDefault("strategy.min_liquidity_sol", 0.0)
	viper.SetDefault("strategy.max_tokens_per_hour", 10)
	viper.SetDefault("strategy.yolo_mode", false)
	viper.SetDefault("strategy.hold_only", false)

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "text")
	viper.SetDefault("logging.log_to_file", false)
	viper.SetDefault("logging.log_file_path", "logs/bot.log")
	viper.SetDefault("logging.trade_log_dir", "trades")

	// Advanced defaults
	viper.SetDefault("advanced.max_retries", MaxRetries)
	viper.SetDefault("advanced.retry_delay_ms", RetryDelayMs)
	viper.SetDefault("advanced.confirm_timeout_sec", ConfirmTimeoutSec)
	viper.SetDefault("advanced.enable_metrics", false)
	viper.SetDefault("advanced.metrics_port", 8080)
}

// All other functions remain the same, just remove Jito references...
// I'll skip showing all the boilerplate but you should remove any references to:
// - JitoConfig
// - Jito-related environment variables
// - Jito-related validation
// - Jito-related defaults

// Helper functions for environment variables
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return defaultValue
}

// Additional helper methods for config (add all the missing functions here)
// IsUltraFastModeEnabled, GetEffectiveParallelWorkers, etc...
// (Copy from the original file but remove Jito-related code)

func loadEnvFile(envPath string) error {
	var envFiles []string

	// If specific path provided, use it first
	if envPath != "" {
		envFiles = append(envFiles, envPath)
	}

	// Add default .env file locations
	envFiles = append(envFiles, []string{
		".env",
		"./.env",
		"configs/.env",
	}...)

	var envFile string
	for _, file := range envFiles {
		if _, err := os.Stat(file); err == nil {
			envFile = file
			break
		}
	}

	if envFile == "" {
		if envPath != "" {
			return fmt.Errorf("specified .env file not found: %s", envPath)
		}
		return fmt.Errorf(".env file not found in any of the expected locations: %v", envFiles)
	}

	fmt.Printf("Loading .env file from: %s\n", envFile)

	file, err := os.Open(envFile)
	if err != nil {
		return fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	loadedCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE format
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Remove quotes if present
				if len(value) >= 2 {
					if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
						(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
						value = value[1 : len(value)-1]
					}
				}

				// Set environment variable
				if err := os.Setenv(key, value); err == nil {
					loadedCount++
					// Only show debug info for non-sensitive variables
					if strings.Contains(key, "URL") || strings.Contains(key, "NETWORK") ||
						strings.Contains(key, "LEVEL") || strings.Contains(key, "ENABLED") {
						fmt.Printf("Loaded from .env: %s=%s\n", key, value)
					} else if strings.Contains(key, "KEY") {
						fmt.Printf("Loaded from .env: %s=[HIDDEN]\n", key)
					} else {
						fmt.Printf("Loaded from .env: %s=%s\n", key, value)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading .env file: %w", err)
	}

	fmt.Printf("Successfully loaded %d environment variables from %s\n", loadedCount, envFile)
	return nil
}

// Add all the missing helper methods (processEnvSubstitution, overrideWithEnvVars, etc.)
// but remove Jito references

func processEnvSubstitution() error {
	// Get all configuration keys
	allKeys := viper.AllKeys()

	for _, key := range allKeys {
		value := viper.GetString(key)

		// Process environment variable substitution
		processedValue := expandEnvVars(value)

		// Set the processed value back
		viper.Set(key, processedValue)
	}

	return nil
}

func expandEnvVars(value string) string {
	if !strings.Contains(value, "${") {
		return value
	}

	// Find all ${VAR:-default} patterns
	result := value
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}

		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		// Extract the variable expression
		expr := result[start+2 : end]

		var varName, defaultValue string
		if strings.Contains(expr, ":-") {
			parts := strings.SplitN(expr, ":-", 2)
			varName = parts[0]
			defaultValue = parts[1]
		} else {
			varName = expr
			defaultValue = ""
		}

		// Get environment variable value
		envValue := os.Getenv(varName)
		if envValue == "" {
			envValue = defaultValue
		}

		// Replace the expression with the resolved value
		result = result[:start] + envValue + result[end+1:]
	}

	return result
}

func overrideWithEnvVars() {
	// Direct environment variable overrides (without Jito)
	if envVal := os.Getenv("PUMPBOT_NETWORK"); envVal != "" {
		viper.Set("network", envVal)
		fmt.Printf("ENV Override - Network: %s\n", envVal)
	}
	if envVal := os.Getenv("PUMPBOT_RPC_URL"); envVal != "" {
		viper.Set("rpc_url", envVal)
		fmt.Printf("ENV Override - RPC URL: %s\n", envVal)
	}
	if envVal := os.Getenv("PUMPBOT_WS_URL"); envVal != "" {
		viper.Set("ws_url", envVal)
		fmt.Printf("ENV Override - WS URL: %s\n", envVal)
	}
	if envVal := os.Getenv("PUMPBOT_PRIVATE_KEY"); envVal != "" {
		viper.Set("private_key", envVal)
		fmt.Printf("ENV Override - Private Key: [SET]\n")
	}

	// Trading overrides
	if envVal := os.Getenv("PUMPBOT_TRADING_BUY_AMOUNT_SOL"); envVal != "" {
		if val, err := strconv.ParseFloat(envVal, 64); err == nil {
			viper.Set("trading.buy_amount_sol", val)
			fmt.Printf("ENV Override - Buy Amount: %f SOL\n", val)
		}
	}
}

func validateConfig(config *Config) error {
	// Set RPC and WS URLs if not provided
	if config.RPCUrl == "" {
		config.RPCUrl = GetRPCEndpoint(config.Network)
	}
	if config.WSUrl == "" {
		config.WSUrl = GetWSEndpoint(config.Network)
	}

	// Validate private key
	if config.PrivateKey == "" {
		return fmt.Errorf("private_key is required")
	}

	// Validate trading amounts
	if config.Trading.BuyAmountSOL < MinTradeAmountSOL {
		return fmt.Errorf("buy_amount_sol must be at least %f", MinTradeAmountSOL)
	}
	if config.Trading.BuyAmountSOL > MaxTradeAmountSOL {
		return fmt.Errorf("buy_amount_sol must not exceed %f", MaxTradeAmountSOL)
	}

	// Validate slippage
	if config.Trading.SlippageBP < 10 || config.Trading.SlippageBP > 5000 {
		return fmt.Errorf("slippage_bp must be between 10 and 5000 (0.1%% to 50%%)")
	}

	// Create log directories if they don't exist
	if config.Logging.LogToFile {
		logDir := filepath.Dir(config.Logging.LogFilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory %s: %w", logDir, err)
		}
	}

	if err := os.MkdirAll(config.Logging.TradeLogDir, 0755); err != nil {
		return fmt.Errorf("failed to create trade log directory %s: %w", config.Logging.TradeLogDir, err)
	}

	return nil
}

// Add all missing helper methods from the original config.go...
func setListenerDefaults() {
	viper.SetDefault("listener.type", "logs")
	viper.SetDefault("listener.buffer_size", 100)
	viper.SetDefault("listener.enable_log_listener", true)
	viper.SetDefault("listener.enable_block_listener", false)
}

func setDefaultsUltraFast() {
	viper.SetDefault("ultra_fast.enabled", true)
	viper.SetDefault("ultra_fast.skip_validation", false)
	viper.SetDefault("ultra_fast.parallel_workers", 1)
	viper.SetDefault("ultra_fast.token_queue_size", 100)
}

func bindListenerEnvVariables() {
	viper.BindEnv("listener.type", "PUMPBOT_LISTENER_TYPE")
	viper.BindEnv("listener.buffer_size", "PUMPBOT_LISTENER_BUFFER_SIZE")
}

func bindUltraFastEnvVariables() {
	viper.BindEnv("ultra_fast.enabled", "PUMPBOT_ULTRA_FAST_ENABLED")
	viper.BindEnv("ultra_fast.skip_validation", "PUMPBOT_ULTRA_FAST_SKIP_VALIDATION")
	viper.BindEnv("ultra_fast.parallel_workers", "PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS")
}

func validateListenerConfig(config *Config) error {
	return nil
}

func GetConfigFromEnv(envPath string) *Config {
	fmt.Printf("Loading configuration from environment variables only...\n")

	// Load .env file first
	if err := loadEnvFile(envPath); err != nil {
		fmt.Printf("Warning: Failed to load .env file: %v\n", err)
	}

	config := &Config{
		Network:    getEnvString("PUMPBOT_NETWORK", "mainnet"),
		RPCUrl:     getEnvString("PUMPBOT_RPC_URL", ""),
		WSUrl:      getEnvString("PUMPBOT_WS_URL", ""),
		RPCAPIKey:  getEnvString("PUMPBOT_RPC_API_KEY", ""),
		PrivateKey: getEnvString("PUMPBOT_PRIVATE_KEY", ""),
		Trading: TradingConfig{
			BuyAmountSOL:        getEnvFloat("PUMPBOT_TRADING_BUY_AMOUNT_SOL", DefaultBuyAmountSOL),
			SlippageBP:          getEnvInt("PUMPBOT_TRADING_SLIPPAGE_BP", DefaultSlippageBP),
			AutoSell:            getEnvBool("PUMPBOT_TRADING_AUTO_SELL", false),
			SellDelayMs:         getEnvInt64("PUMPBOT_TRADING_SELL_DELAY_MS", 1000),
			SellPercentage:      getEnvFloat("PUMPBOT_TRADING_SELL_PERCENTAGE", 100.0),
			TakeProfitPercent:   getEnvFloat("PUMPBOT_TRADING_TAKE_PROFIT_PERCENT", 50.0),
			StopLossPercent:     getEnvFloat("PUMPBOT_TRADING_STOP_LOSS_PERCENT", -20.0),
			PriorityFee:         uint64(getEnvInt("PUMPBOT_TRADING_PRIORITY_FEE", 0)),
			MaxGasPrice:         uint64(getEnvInt("PUMPBOT_TRADING_MAX_GAS_PRICE", 0)),
			MaxTokenAgeMs:       getEnvInt64("PUMPBOT_TRADING_MAX_TOKEN_AGE_MS", 5000),
			MinDiscoveryDelayMs: getEnvInt64("PUMPBOT_TRADING_MIN_DISCOVERY_DELAY_MS", 100),
		},
		Strategy: StrategyConfig{
			Type:             getEnvString("PUMPBOT_STRATEGY_TYPE", "sniper"),
			FilterByCreator:  getEnvBool("PUMPBOT_STRATEGY_FILTER_BY_CREATOR", false),
			FilterByName:     getEnvBool("PUMPBOT_STRATEGY_FILTER_BY_NAME", false),
			MinLiquiditySOL:  getEnvFloat("PUMPBOT_STRATEGY_MIN_LIQUIDITY_SOL", 0.0),
			MaxTokensPerHour: getEnvInt64("PUMPBOT_STRATEGY_MAX_TOKENS_PER_HOUR", 10),
			YoloMode:         getEnvBool("PUMPBOT_STRATEGY_YOLO_MODE", false),
			HoldOnly:         getEnvBool("PUMPBOT_STRATEGY_HOLD_ONLY", false),
		},
		Logging: LoggingConfig{
			Level:       getEnvString("PUMPBOT_LOGGING_LEVEL", "info"),
			Format:      getEnvString("PUMPBOT_LOGGING_FORMAT", "text"),
			LogToFile:   getEnvBool("PUMPBOT_LOGGING_LOG_TO_FILE", false),
			LogFilePath: getEnvString("PUMPBOT_LOGGING_LOG_FILE_PATH", "logs/bot.log"),
			TradeLogDir: getEnvString("PUMPBOT_LOGGING_TRADE_LOG_DIR", "trades"),
		},
		Advanced: AdvancedConfig{
			MaxRetries:        getEnvInt("PUMPBOT_ADVANCED_MAX_RETRIES", MaxRetries),
			RetryDelayMs:      getEnvInt("PUMPBOT_ADVANCED_RETRY_DELAY_MS", RetryDelayMs),
			ConfirmTimeoutSec: getEnvInt("PUMPBOT_ADVANCED_CONFIRM_TIMEOUT_SEC", ConfirmTimeoutSec),
			EnableMetrics:     getEnvBool("PUMPBOT_ADVANCED_ENABLE_METRICS", false),
			MetricsPort:       getEnvInt("PUMPBOT_ADVANCED_METRICS_PORT", 8080),
		},
	}

	// Set URLs if not provided via environment
	if config.RPCUrl == "" {
		config.RPCUrl = GetRPCEndpoint(config.Network)
	}

	if config.WSUrl == "" {
		config.WSUrl = GetWSEndpoint(config.Network)
	}

	config.Jito = JitoConfig{
		Enabled:             getEnvBool("PUMPBOT_JITO_ENABLED", true),
		BlockEngineURL:      getEnvString("PUMPBOT_JITO_BLOCK_ENGINE_URL", ""),
		APIKey:              getEnvString("PUMPBOT_JITO_API_KEY", ""),
		Timeout:             getEnvInt("PUMPBOT_JITO_TIMEOUT_MS", 30000),
		FallbackEnabled:     getEnvBool("PUMPBOT_JITO_FALLBACK_ENABLED", true),
		MaxBundleSize:       getEnvInt("PUMPBOT_JITO_MAX_BUNDLE_SIZE", 5),
		BundleRetryAttempts: getEnvInt("PUMPBOT_JITO_BUNDLE_RETRY_ATTEMPTS", 3),
		TipLamports:         uint64(getEnvInt64("PUMPBOT_JITO_TIP_LAMPORTS", 10000)),
		TipPercent:          getEnvFloat("PUMPBOT_JITO_TIP_PERCENT", 0.0),
		UseRandomTip:        getEnvBool("PUMPBOT_JITO_USE_RANDOM_TIP", false),
		MinTipLamports:      uint64(getEnvInt64("PUMPBOT_JITO_MIN_TIP_LAMPORTS", 1000)),
		MaxTipLamports:      uint64(getEnvInt64("PUMPBOT_JITO_MAX_TIP_LAMPORTS", 100000)),
	}

	return config
}

// Add to setDefaults function
func setJitoDefaults() {
	viper.SetDefault("jito.enabled", false)
	viper.SetDefault("jito.block_engine_url", "")
	viper.SetDefault("jito.timeout_ms", 30000)
	viper.SetDefault("jito.fallback_enabled", true)
	viper.SetDefault("jito.max_bundle_size", 5)
	viper.SetDefault("jito.bundle_retry_attempts", 3)
	viper.SetDefault("jito.tip_lamports", 10000)
	viper.SetDefault("jito.tip_percent", 0.0)
	viper.SetDefault("jito.use_random_tip", false)
	viper.SetDefault("jito.min_tip_lamports", 1000)
	viper.SetDefault("jito.max_tip_lamports", 100000)
}

// Add to bindEnvVariables function
func bindJitoEnvVariables() {
	viper.BindEnv("jito.enabled", "PUMPBOT_JITO_ENABLED")
	viper.BindEnv("jito.block_engine_url", "PUMPBOT_JITO_BLOCK_ENGINE_URL")
	viper.BindEnv("jito.api_key", "PUMPBOT_JITO_API_KEY")
	viper.BindEnv("jito.timeout_ms", "PUMPBOT_JITO_TIMEOUT_MS")
	viper.BindEnv("jito.fallback_enabled", "PUMPBOT_JITO_FALLBACK_ENABLED")
	viper.BindEnv("jito.max_bundle_size", "PUMPBOT_JITO_MAX_BUNDLE_SIZE")
	viper.BindEnv("jito.bundle_retry_attempts", "PUMPBOT_JITO_BUNDLE_RETRY_ATTEMPTS")
	viper.BindEnv("jito.tip_lamports", "PUMPBOT_JITO_TIP_LAMPORTS")
	viper.BindEnv("jito.tip_percent", "PUMPBOT_JITO_TIP_PERCENT")
	viper.BindEnv("jito.use_random_tip", "PUMPBOT_JITO_USE_RANDOM_TIP")
	viper.BindEnv("jito.min_tip_lamports", "PUMPBOT_JITO_MIN_TIP_LAMPORTS")
	viper.BindEnv("jito.max_tip_lamports", "PUMPBOT_JITO_MAX_TIP_LAMPORTS")
}

// Add helper methods to Config
func (c *Config) IsJitoEnabled() bool {
	return c.Jito.Enabled && c.Jito.BlockEngineURL != ""
}

func (c *Config) GetJitoBlockEngineURL() string {
	if c.Jito.BlockEngineURL != "" {
		return c.Jito.BlockEngineURL
	}

	// Default based on network
	switch c.Network {
	case "mainnet":
		return "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
	case "devnet":
		return "https://devnet.block-engine.jito.wtf/api/v1/bundles"
	default:
		return "https://mainnet.block-engine.jito.wtf/api/v1/bundles"
	}
}

func (c *Config) GetJitoTimeout() time.Duration {
	if c.Jito.Timeout > 0 {
		return time.Duration(c.Jito.Timeout) * time.Millisecond
	}
	return 30 * time.Second
}

// Add to validateConfig function
func validateJitoConfig(config *Config) error {
	if !config.Jito.Enabled {
		return nil // Skip validation if Jito is disabled
	}

	if config.Jito.BlockEngineURL == "" {
		return fmt.Errorf("jito.block_engine_url is required when Jito is enabled")
	}

	if config.Jito.Timeout < 1000 {
		return fmt.Errorf("jito.timeout_ms must be at least 1000ms")
	}

	if config.Jito.MaxBundleSize < 1 || config.Jito.MaxBundleSize > 10 {
		return fmt.Errorf("jito.max_bundle_size must be between 1 and 10")
	}

	if config.Jito.UseRandomTip {
		if config.Jito.MinTipLamports >= config.Jito.MaxTipLamports {
			return fmt.Errorf("jito.min_tip_lamports must be less than max_tip_lamports")
		}
	}

	return nil
}

// Add other helper methods...
func (c *Config) IsUltraFastModeEnabled() bool {
	return c.UltraFast.Enabled
}

func (c *Config) GetEffectiveParallelWorkers() int {
	if c.UltraFast.ParallelWorkers < 1 {
		return 1
	}
	if c.UltraFast.ParallelWorkers > 10 {
		return 10
	}
	return c.UltraFast.ParallelWorkers
}

func (c *Config) IsTokenBasedTrading() bool {
	return c.Trading.UseTokenAmount
}

func (c *Config) GetListenerType() ListenerType {
	return c.Listener.Type
}

func (c *Config) GetListenerBufferSize() int {
	if c.Listener.BufferSize > 0 {
		return c.Listener.BufferSize
	}
	return 100
}

func (c *Config) ShouldUseLogListener() bool {
	return c.Listener.Type == LogsListenerType
}

func (c *Config) ShouldUseBlockListener() bool {
	return c.Listener.Type == BlocksListenerType
}

func (c *Config) ShouldFilterDuplicates() bool {
	return c.Listener.EnableDuplicateFilter
}

func (c *Config) GetDuplicateFilterTTL() time.Duration {
	return time.Duration(c.Listener.DuplicateFilterTTL) * time.Second
}

func (c *Config) GetMaxTokenAge() time.Duration {
	if c.Trading.MaxTokenAgeMs <= 0 {
		return time.Duration(0)
	}
	return time.Duration(c.Trading.MaxTokenAgeMs) * time.Millisecond
}

func (c *Config) GetMinDiscoveryDelay() time.Duration {
	if c.Trading.MinDiscoveryDelayMs <= 0 {
		return time.Duration(0)
	}
	return time.Duration(c.Trading.MinDiscoveryDelayMs) * time.Millisecond
}

func (c *Config) IsTokenAgeValid(tokenAge time.Duration) bool {
	maxAge := c.GetMaxTokenAge()
	if maxAge == 0 {
		return true
	}
	return tokenAge <= maxAge
}

func (c *Config) IsDiscoveryDelayValid(timeSinceDiscovery time.Duration) bool {
	minDelay := c.GetMinDiscoveryDelay()
	if minDelay == 0 {
		return true
	}
	return timeSinceDiscovery >= minDelay
}
