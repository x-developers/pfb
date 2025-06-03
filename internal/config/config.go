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

	// JITO settings
	JITO JitoConfig `mapstructure:"jito" yaml:"jito"`

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

	// Extreme Fast Mode settings
	ExtremeFast ExtremeFastConfig `mapstructure:"extreme_fast" yaml:"extreme_fast"`

	// Ultra Fast Mode settings
	UltraFast UltraFastConfig `mapstructure:"ultra_fast" yaml:"ultra_fast"`
}

// TradingConfig contains trading-related settings
type TradingConfig struct {
	BuyAmountSOL        float64 `mapstructure:"buy_amount_sol" yaml:"buy_amount_sol"`
	SlippageBP          int     `mapstructure:"slippage_bp" yaml:"slippage_bp"`
	MaxGasPrice         uint64  `mapstructure:"max_gas_price" yaml:"max_gas_price"`
	PriorityFee         uint64  `mapstructure:"priority_fee" yaml:"priority_fee"`
	AutoSell            bool    `mapstructure:"auto_sell" yaml:"auto_sell"`
	SellDelaySeconds    int     `mapstructure:"sell_delay_seconds" yaml:"sell_delay_seconds"`
	SellPercentage      float64 `mapstructure:"sell_percentage" yaml:"sell_percentage"`
	TakeProfitPercent   float64 `mapstructure:"take_profit_percent" yaml:"take_profit_percent"`
	StopLossPercent     float64 `mapstructure:"stop_loss_percent" yaml:"stop_loss_percent"`
	MaxTokenAgeMs       int64   `mapstructure:"max_token_age_ms" yaml:"max_token_age_ms"`             // NEW: максимальный возраст токена в мс
	MinDiscoveryDelayMs int64   `mapstructure:"min_discovery_delay_ms" yaml:"min_discovery_delay_ms"` // NEW: минимальная задержка после обнаружения
}

// JitoConfig contains JITO-related settings
type JitoConfig struct {
	Enabled        bool   `mapstructure:"enabled" yaml:"enabled"`
	Endpoint       string `mapstructure:"endpoint" yaml:"endpoint"`
	APIKey         string `mapstructure:"api_key" yaml:"api_key"`
	TipAmount      uint64 `mapstructure:"tip_amount" yaml:"tip_amount"`           // Tip amount in lamports
	UseForTrading  bool   `mapstructure:"use_for_trading" yaml:"use_for_trading"` // Use JITO for buy/sell transactions
	ConfirmTimeout int    `mapstructure:"confirm_timeout" yaml:"confirm_timeout"` // Bundle confirmation timeout in seconds
	MaxBundleSize  int    `mapstructure:"max_bundle_size" yaml:"max_bundle_size"` // Maximum transactions per bundle
}

// StrategyConfig contains strategy-related settings
type StrategyConfig struct {
	Type             string   `mapstructure:"type" yaml:"type"`
	FilterByCreator  bool     `mapstructure:"filter_by_creator" yaml:"filter_by_creator"`
	AllowedCreators  []string `mapstructure:"allowed_creators" yaml:"allowed_creators"`
	FilterByName     bool     `mapstructure:"filter_by_name" yaml:"filter_by_name"`
	NamePatterns     []string `mapstructure:"name_patterns" yaml:"name_patterns"`
	MinLiquiditySOL  float64  `mapstructure:"min_liquidity_sol" yaml:"min_liquidity_sol"`
	MaxTokensPerHour int      `mapstructure:"max_tokens_per_hour" yaml:"max_tokens_per_hour"`
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

// ExtremeFastConfig contains configuration for extreme fast mode
type ExtremeFastConfig struct {
	Enabled                  bool   `mapstructure:"enabled" yaml:"enabled"`
	PriorityFee              uint64 `mapstructure:"priority_fee" yaml:"priority_fee"`                             // Micro-lamports per compute unit
	ComputeUnitLimit         uint32 `mapstructure:"compute_unit_limit" yaml:"compute_unit_limit"`                 // Max compute units
	ComputeUnitPrice         uint64 `mapstructure:"compute_unit_price" yaml:"compute_unit_price"`                 // Micro-lamports per compute unit
	MaxPrecomputedTx         int    `mapstructure:"max_precomputed_tx" yaml:"max_precomputed_tx"`                 // Number of precomputed transactions to maintain
	RefreshBlockhashInterval int    `mapstructure:"refresh_blockhash_interval" yaml:"refresh_blockhash_interval"` // Seconds between blockhash refresh
	UseJito                  bool   `mapstructure:"use_jito" yaml:"use_jito"`                                     // Use Jito MEV protection
	JitoTipAmount            uint64 `mapstructure:"jito_tip_amount" yaml:"jito_tip_amount"`                       // Jito tip in lamports
	MaxSlippageBP            int    `mapstructure:"max_slippage_bp" yaml:"max_slippage_bp"`                       // Maximum slippage in basis points
	TargetConfirmationTime   int    `mapstructure:"target_confirmation_time" yaml:"target_confirmation_time"`     // Target confirmation time in seconds
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
	setDefaultsUltraFast()

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
	bindUltraFastEnvVariables()

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

	// Debug: Print some key values to verify substitution
	fmt.Printf("Debug - Network: %s\n", viper.GetString("network"))
	fmt.Printf("Debug - RPC URL: %s\n", viper.GetString("rpc_url"))
	fmt.Printf("Debug - WS URL: %s\n", viper.GetString("ws_url"))
	fmt.Printf("Debug - Extreme Fast Enabled: %v\n", viper.GetBool("extreme_fast.enabled"))
	fmt.Printf("Debug - Buy Amount: %v\n", viper.GetFloat64("trading.buy_amount_sol"))

	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate and post-process config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Validate and post-process config
	if err := validateUltraFastConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// loadEnvFile loads environment variables from .env file
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

// bindEnvVariables manually binds environment variables that viper might miss
func bindEnvVariables() {
	// Top-level variables
	viper.BindEnv("network", "PUMPBOT_NETWORK")
	viper.BindEnv("rpc_url", "PUMPBOT_RPC_URL")
	viper.BindEnv("ws_url", "PUMPBOT_WS_URL")
	viper.BindEnv("rpc_api_key", "PUMPBOT_RPC_API_KEY")
	viper.BindEnv("private_key", "PUMPBOT_PRIVATE_KEY")

	// Trading variables
	viper.BindEnv("trading.buy_amount_sol", "PUMPBOT_TRADING_BUY_AMOUNT_SOL")
	viper.BindEnv("trading.slippage_bp", "PUMPBOT_TRADING_SLIPPAGE_BP")
	viper.BindEnv("trading.priority_fee", "PUMPBOT_TRADING_PRIORITY_FEE")
	viper.BindEnv("trading.auto_sell", "PUMPBOT_TRADING_AUTO_SELL")
	viper.BindEnv("trading.sell_delay_seconds", "PUMPBOT_TRADING_SELL_DELAY_SECONDS")
	viper.BindEnv("trading.max_token_age_ms", "PUMPBOT_TRADING_MAX_TOKEN_AGE_MS")
	viper.BindEnv("trading.min_discovery_delay_ms", "PUMPBOT_TRADING_MIN_DISCOVERY_DELAY_MS")

	// Strategy variables
	viper.BindEnv("strategy.type", "PUMPBOT_STRATEGY_TYPE")
	viper.BindEnv("strategy.yolo_mode", "PUMPBOT_STRATEGY_YOLO_MODE")
	viper.BindEnv("strategy.hold_only", "PUMPBOT_STRATEGY_HOLD_ONLY")
	viper.BindEnv("strategy.max_tokens_per_hour", "PUMPBOT_STRATEGY_MAX_TOKENS_PER_HOUR")

	// Extreme Fast variables
	viper.BindEnv("extreme_fast.enabled", "PUMPBOT_EXTREME_FAST_ENABLED")
	viper.BindEnv("extreme_fast.priority_fee", "PUMPBOT_EXTREME_FAST_PRIORITY_FEE")
	viper.BindEnv("extreme_fast.compute_unit_limit", "PUMPBOT_EXTREME_FAST_COMPUTE_LIMIT")
	viper.BindEnv("extreme_fast.compute_unit_price", "PUMPBOT_EXTREME_FAST_COMPUTE_PRICE")
	viper.BindEnv("extreme_fast.max_precomputed_tx", "PUMPBOT_EXTREME_FAST_MAX_PRECOMPUTED")
	viper.BindEnv("extreme_fast.refresh_blockhash_interval", "PUMPBOT_EXTREME_FAST_BLOCKHASH_INTERVAL")
	viper.BindEnv("extreme_fast.max_slippage_bp", "PUMPBOT_EXTREME_FAST_MAX_SLIPPAGE")

	// JITO variables
	viper.BindEnv("jito.enabled", "PUMPBOT_JITO_ENABLED")
	viper.BindEnv("jito.endpoint", "PUMPBOT_JITO_ENDPOINT")
	viper.BindEnv("jito.api_key", "PUMPBOT_JITO_API_KEY")
	viper.BindEnv("jito.tip_amount", "PUMPBOT_JITO_TIP_AMOUNT")
	viper.BindEnv("jito.use_for_trading", "PUMPBOT_JITO_USE_FOR_TRADING")
	viper.BindEnv("jito.confirm_timeout", "PUMPBOT_JITO_CONFIRM_TIMEOUT")
	viper.BindEnv("jito.max_bundle_size", "PUMPBOT_JITO_MAX_BUNDLE_SIZE")

	// Logging variables
	viper.BindEnv("logging.level", "PUMPBOT_LOGGING_LEVEL")
	viper.BindEnv("logging.format", "PUMPBOT_LOGGING_FORMAT")
	viper.BindEnv("logging.log_to_file", "PUMPBOT_LOGGING_LOG_TO_FILE")
}

// overrideWithEnvVars directly reads environment variables to ensure they override everything
func overrideWithEnvVars() {
	// Direct environment variable overrides
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
	if envVal := os.Getenv("PUMPBOT_RPC_API_KEY"); envVal != "" {
		viper.Set("rpc_api_key", envVal)
		fmt.Printf("ENV Override - API Key: [SET]\n")
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
	if envVal := os.Getenv("PUMPBOT_TRADING_SLIPPAGE_BP"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil {
			viper.Set("trading.slippage_bp", val)
			fmt.Printf("ENV Override - Slippage: %d BP\n", val)
		}
	}
	if envVal := os.Getenv("PUMPBOT_TRADING_MAX_TOKEN_AGE_MS"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			viper.Set("trading.max_token_age_ms", val)
			fmt.Printf("ENV Override - Max Token Age: %d ms\n", val)
		}
	}
	if envVal := os.Getenv("PUMPBOT_TRADING_MIN_DISCOVERY_DELAY_MS"); envVal != "" {
		if val, err := strconv.ParseInt(envVal, 10, 64); err == nil {
			viper.Set("trading.min_discovery_delay_ms", val)
			fmt.Printf("ENV Override - Min Discovery Delay: %d ms\n", val)
		}
	}

	// Strategy overrides
	if envVal := os.Getenv("PUMPBOT_STRATEGY_YOLO_MODE"); envVal != "" {
		val := envVal == "true" || envVal == "1"
		viper.Set("strategy.yolo_mode", val)
		fmt.Printf("ENV Override - YOLO Mode: %v\n", val)
	}
	if envVal := os.Getenv("PUMPBOT_STRATEGY_HOLD_ONLY"); envVal != "" {
		val := envVal == "true" || envVal == "1"
		viper.Set("strategy.hold_only", val)
		fmt.Printf("ENV Override - Hold Only: %v\n", val)
	}

	// Extreme Fast overrides
	if envVal := os.Getenv("PUMPBOT_EXTREME_FAST_ENABLED"); envVal != "" {
		val := envVal == "true" || envVal == "1"
		viper.Set("extreme_fast.enabled", val)
		fmt.Printf("ENV Override - Extreme Fast: %v\n", val)
	}
	if envVal := os.Getenv("PUMPBOT_EXTREME_FAST_PRIORITY_FEE"); envVal != "" {
		if val, err := strconv.ParseUint(envVal, 10, 64); err == nil {
			viper.Set("extreme_fast.priority_fee", val)
			fmt.Printf("ENV Override - Extreme Fast Priority Fee: %d\n", val)
		}
	}

	// JITO overrides
	if envVal := os.Getenv("PUMPBOT_JITO_ENABLED"); envVal != "" {
		val := envVal == "true" || envVal == "1"
		viper.Set("jito.enabled", val)
		fmt.Printf("ENV Override - JITO Enabled: %v\n", val)
	}
	if envVal := os.Getenv("PUMPBOT_JITO_ENDPOINT"); envVal != "" {
		viper.Set("jito.endpoint", envVal)
		fmt.Printf("ENV Override - JITO Endpoint: %s\n", envVal)
	}
	if envVal := os.Getenv("PUMPBOT_JITO_API_KEY"); envVal != "" {
		viper.Set("jito.api_key", envVal)
		fmt.Printf("ENV Override - JITO API Key: [SET]\n")
	}
	if envVal := os.Getenv("PUMPBOT_JITO_TIP_AMOUNT"); envVal != "" {
		if val, err := strconv.ParseUint(envVal, 10, 64); err == nil {
			viper.Set("jito.tip_amount", val)
			fmt.Printf("ENV Override - JITO Tip Amount: %d\n", val)
		}
	}
	if envVal := os.Getenv("PUMPBOT_JITO_USE_FOR_TRADING"); envVal != "" {
		val := envVal == "true" || envVal == "1"
		viper.Set("jito.use_for_trading", val)
		fmt.Printf("ENV Override - JITO Use For Trading: %v\n", val)
	}

	// Logging overrides
	if envVal := os.Getenv("PUMPBOT_LOGGING_LEVEL"); envVal != "" {
		viper.Set("logging.level", envVal)
		fmt.Printf("ENV Override - Log Level: %s\n", envVal)
	}
}

// processEnvSubstitution processes ${VAR:-default} substitution in config values
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

// expandEnvVars expands environment variables in the format ${VAR:-default}
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

// setDefaults sets default configuration values
func setDefaults() {
	// Network defaults
	viper.SetDefault("network", "mainnet")
	viper.SetDefault("rpc_url", "")
	viper.SetDefault("ws_url", "")

	viper.SetDefault("trading.buy_amount_sol", DefaultBuyAmountSOL)
	viper.SetDefault("trading.slippage_bp", DefaultSlippageBP)
	viper.SetDefault("trading.max_gas_price", 0)
	viper.SetDefault("trading.priority_fee", 0)
	viper.SetDefault("trading.auto_sell", false)
	viper.SetDefault("trading.sell_delay_seconds", 30)
	viper.SetDefault("trading.sell_percentage", 100.0)
	viper.SetDefault("trading.take_profit_percent", 50.0)
	viper.SetDefault("trading.stop_loss_percent", -20.0)
	viper.SetDefault("trading.max_token_age_ms", 5000)      // NEW: 5 seconds max age
	viper.SetDefault("trading.min_discovery_delay_ms", 100) // NEW: 100ms minimum delay

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

	// JITO defaults
	viper.SetDefault("jito.enabled", false)
	viper.SetDefault("jito.endpoint", "https://mainnet.block-engine.jito.wtf/api/v1/bundles")
	viper.SetDefault("jito.api_key", "")
	viper.SetDefault("jito.tip_amount", 10000)
	viper.SetDefault("jito.use_for_trading", false)
	viper.SetDefault("jito.confirm_timeout", 30)
	viper.SetDefault("jito.max_bundle_size", 5)

	// Extreme Fast Mode defaults
	viper.SetDefault("extreme_fast.enabled", false)
	viper.SetDefault("extreme_fast.priority_fee", 100000)
	viper.SetDefault("extreme_fast.compute_unit_limit", 400000)
	viper.SetDefault("extreme_fast.compute_unit_price", 1000)
	viper.SetDefault("extreme_fast.max_precomputed_tx", 50)
	viper.SetDefault("extreme_fast.refresh_blockhash_interval", 5)
	viper.SetDefault("extreme_fast.use_jito", false)
	viper.SetDefault("extreme_fast.jito_tip_amount", 10000)
	viper.SetDefault("extreme_fast.max_slippage_bp", 2000)
	viper.SetDefault("extreme_fast.target_confirmation_time", 4)
}

// validateConfig validates the configuration
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

	// Validate strategy
	if config.Strategy.Type != "sniper" && config.Strategy.Type != "holder" {
		return fmt.Errorf("strategy.type must be 'sniper' or 'holder'")
	}

	// Validate timing settings
	if config.Trading.MaxTokenAgeMs < 0 {
		return fmt.Errorf("max_token_age_ms must be non-negative")
	}
	if config.Trading.MinDiscoveryDelayMs < 0 {
		return fmt.Errorf("min_discovery_delay_ms must be non-negative")
	}
	if config.Trading.MaxTokenAgeMs > 0 && config.Trading.MinDiscoveryDelayMs > config.Trading.MaxTokenAgeMs {
		return fmt.Errorf("min_discovery_delay_ms (%d) cannot be greater than max_token_age_ms (%d)",
			config.Trading.MinDiscoveryDelayMs, config.Trading.MaxTokenAgeMs)
	}

	// Validate extreme fast mode settings
	if config.ExtremeFast.Enabled {
		if config.ExtremeFast.MaxPrecomputedTx < 1 || config.ExtremeFast.MaxPrecomputedTx > 200 {
			return fmt.Errorf("extreme_fast.max_precomputed_tx must be between 1 and 200")
		}
		if config.ExtremeFast.RefreshBlockhashInterval < 1 || config.ExtremeFast.RefreshBlockhashInterval > 60 {
			return fmt.Errorf("extreme_fast.refresh_blockhash_interval must be between 1 and 60 seconds")
		}
		if config.ExtremeFast.MaxSlippageBP < 100 || config.ExtremeFast.MaxSlippageBP > 10000 {
			return fmt.Errorf("extreme_fast.max_slippage_bp must be between 100 and 10000 (1%% to 100%%)")
		}
		if config.ExtremeFast.ComputeUnitLimit < 10000 || config.ExtremeFast.ComputeUnitLimit > 1400000 {
			return fmt.Errorf("extreme_fast.compute_unit_limit must be between 10000 and 1400000")
		}
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

// GetConfigFromEnv loads configuration from environment variables only
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
			SellDelaySeconds:    getEnvInt("PUMPBOT_TRADING_SELL_DELAY_SECONDS", 30),
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
			MaxTokensPerHour: getEnvInt("PUMPBOT_STRATEGY_MAX_TOKENS_PER_HOUR", 10),
			YoloMode:         getEnvBool("PUMPBOT_STRATEGY_YOLO_MODE", false),
			HoldOnly:         getEnvBool("PUMPBOT_STRATEGY_HOLD_ONLY", false),
		},
		JITO: JitoConfig{
			Enabled:        getEnvBool("PUMPBOT_JITO_ENABLED", false),
			Endpoint:       getEnvString("PUMPBOT_JITO_ENDPOINT", "https://mainnet.block-engine.jito.wtf/api/v1/bundles"),
			APIKey:         getEnvString("PUMPBOT_JITO_API_KEY", ""),
			TipAmount:      uint64(getEnvInt("PUMPBOT_JITO_TIP_AMOUNT", 10000)),
			UseForTrading:  getEnvBool("PUMPBOT_JITO_USE_FOR_TRADING", false),
			ConfirmTimeout: getEnvInt("PUMPBOT_JITO_CONFIRM_TIMEOUT", 30),
			MaxBundleSize:  getEnvInt("PUMPBOT_JITO_MAX_BUNDLE_SIZE", 5),
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
		ExtremeFast: ExtremeFastConfig{
			Enabled:                  getEnvBool("PUMPBOT_EXTREME_FAST_ENABLED", false),
			PriorityFee:              uint64(getEnvInt("PUMPBOT_EXTREME_FAST_PRIORITY_FEE", 100000)),
			ComputeUnitLimit:         uint32(getEnvInt("PUMPBOT_EXTREME_FAST_COMPUTE_LIMIT", 400000)),
			ComputeUnitPrice:         uint64(getEnvInt("PUMPBOT_EXTREME_FAST_COMPUTE_PRICE", 1000)),
			MaxPrecomputedTx:         getEnvInt("PUMPBOT_EXTREME_FAST_MAX_PRECOMPUTED", 50),
			RefreshBlockhashInterval: getEnvInt("PUMPBOT_EXTREME_FAST_BLOCKHASH_INTERVAL", 5),
			UseJito:                  getEnvBool("PUMPBOT_EXTREME_FAST_USE_JITO", false),
			JitoTipAmount:            uint64(getEnvInt("PUMPBOT_EXTREME_FAST_JITO_TIP", 10000)),
			MaxSlippageBP:            getEnvInt("PUMPBOT_EXTREME_FAST_MAX_SLIPPAGE", 2000),
			TargetConfirmationTime:   getEnvInt("PUMPBOT_EXTREME_FAST_TARGET_CONFIRM", 4),
		},
	}

	// Set URLs if not provided via environment
	if config.RPCUrl == "" {
		config.RPCUrl = GetRPCEndpoint(config.Network)
		fmt.Printf("Using default RPC URL for %s: %s\n", config.Network, config.RPCUrl)
	} else {
		fmt.Printf("Using custom RPC URL: %s\n", config.RPCUrl)
	}

	if config.WSUrl == "" {
		config.WSUrl = GetWSEndpoint(config.Network)
		fmt.Printf("Using default WebSocket URL for %s: %s\n", config.Network, config.WSUrl)
	} else {
		fmt.Printf("Using custom WebSocket URL: %s\n", config.WSUrl)
	}

	return config
}

// Extreme Fast Mode helper methods
func (c *Config) IsExtremeFastModeEnabled() bool {
	return c.ExtremeFast.Enabled
}

func (c *Config) GetPriorityFee() uint64 {
	if c.IsExtremeFastModeEnabled() {
		return c.ExtremeFast.PriorityFee
	}
	return c.Trading.PriorityFee
}

func (c *Config) GetComputeUnitLimit() uint32 {
	if c.IsExtremeFastModeEnabled() {
		return c.ExtremeFast.ComputeUnitLimit
	}
	return 200000 // Default
}

func (c *Config) GetComputeUnitPrice() uint64 {
	if c.IsExtremeFastModeEnabled() {
		return c.ExtremeFast.ComputeUnitPrice
	}
	return 1000000 // Default from original config
}

func (c *Config) GetEffectiveSlippageBP() int {
	if c.IsExtremeFastModeEnabled() {
		return c.ExtremeFast.MaxSlippageBP
	}
	return c.Trading.SlippageBP
}

// IsUltraFastModeEnabled returns true if ultra-fast mode is enabled
func (c *Config) IsUltraFastModeEnabled() bool {
	return c.UltraFast.Enabled
}

// GetEffectiveParallelWorkers returns the number of workers to use
func (c *Config) GetEffectiveParallelWorkers() int {
	if c.UltraFast.ParallelWorkers < 1 {
		return 1
	}
	if c.UltraFast.ParallelWorkers > 10 {
		return 10 // Safety limit
	}
	return c.UltraFast.ParallelWorkers
}

// ShouldSkipValidation returns true if validation should be skipped
func (c *Config) ShouldSkipValidation() bool {
	return c.UltraFast.Enabled && (c.UltraFast.SkipValidation || c.Strategy.YoloMode)
}

// GetEffectiveRPCTimeout returns RPC timeout in milliseconds
func (c *Config) GetEffectiveRPCTimeout() time.Duration {
	if c.UltraFast.RPCTimeout > 0 {
		return time.Duration(c.UltraFast.RPCTimeout) * time.Millisecond
	}
	return 5000 * time.Millisecond // Default 5 seconds
}

// NEW: Token timing validation methods
func (c *Config) GetMaxTokenAge() time.Duration {
	if c.Trading.MaxTokenAgeMs <= 0 {
		return time.Duration(0) // No age limit
	}
	return time.Duration(c.Trading.MaxTokenAgeMs) * time.Millisecond
}

func (c *Config) GetMinDiscoveryDelay() time.Duration {
	if c.Trading.MinDiscoveryDelayMs <= 0 {
		return time.Duration(0) // No minimum delay
	}
	return time.Duration(c.Trading.MinDiscoveryDelayMs) * time.Millisecond
}

func (c *Config) IsTokenAgeValid(tokenAge time.Duration) bool {
	maxAge := c.GetMaxTokenAge()
	if maxAge == 0 {
		return true // No age limit
	}
	return tokenAge <= maxAge
}

func (c *Config) IsDiscoveryDelayValid(timeSinceDiscovery time.Duration) bool {
	minDelay := c.GetMinDiscoveryDelay()
	if minDelay == 0 {
		return true // No minimum delay
	}
	return timeSinceDiscovery >= minDelay
}

// Add to setDefaults function:
func setDefaultsUltraFast() {
	// Ultra Fast Mode defaults
	viper.SetDefault("ultra_fast.enabled", true)
	viper.SetDefault("ultra_fast.skip_validation", false)
	viper.SetDefault("ultra_fast.no_confirmation", false)
	viper.SetDefault("ultra_fast.fire_and_forget", false)
	viper.SetDefault("ultra_fast.parallel_workers", 1)
	viper.SetDefault("ultra_fast.token_queue_size", 100)
	viper.SetDefault("ultra_fast.cache_blockhash", true)
	viper.SetDefault("ultra_fast.blockhash_refresh_interval", 10)
	viper.SetDefault("ultra_fast.precompute_instructions", true)
	viper.SetDefault("ultra_fast.reuse_buffers", true)
	viper.SetDefault("ultra_fast.multiplex_rpc", false)
	viper.SetDefault("ultra_fast.use_fastest_rpc", false)
	viper.SetDefault("ultra_fast.rpc_timeout_ms", 3000)
	viper.SetDefault("ultra_fast.benchmark_mode", false)
	viper.SetDefault("ultra_fast.log_latency", false)
	viper.SetDefault("ultra_fast.profile_memory", false)
	viper.SetDefault("ultra_fast.max_tokens_per_second", 10)
	viper.SetDefault("ultra_fast.emergency_stop_loss", -50.0)
	viper.SetDefault("ultra_fast.fixed_buy_amount", true)
	viper.SetDefault("ultra_fast.skip_ata_creation", false)
	viper.SetDefault("ultra_fast.assume_ata_exists", false)
	viper.SetDefault("ultra_fast.immediate_execution", true)
	viper.SetDefault("ultra_fast.priority_over_safety", false)
}

// Add environment variable bindings:
func bindUltraFastEnvVariables() {
	// Ultra Fast variables
	viper.BindEnv("ultra_fast.enabled", "PUMPBOT_ULTRA_FAST_ENABLED")
	viper.BindEnv("ultra_fast.skip_validation", "PUMPBOT_ULTRA_FAST_SKIP_VALIDATION")
	viper.BindEnv("ultra_fast.no_confirmation", "PUMPBOT_ULTRA_FAST_NO_CONFIRMATION")
	viper.BindEnv("ultra_fast.fire_and_forget", "PUMPBOT_ULTRA_FAST_FIRE_AND_FORGET")
	viper.BindEnv("ultra_fast.parallel_workers", "PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS")
	viper.BindEnv("ultra_fast.token_queue_size", "PUMPBOT_ULTRA_FAST_TOKEN_QUEUE_SIZE")
	viper.BindEnv("ultra_fast.cache_blockhash", "PUMPBOT_ULTRA_FAST_CACHE_BLOCKHASH")
	viper.BindEnv("ultra_fast.blockhash_refresh_interval", "PUMPBOT_ULTRA_FAST_BLOCKHASH_INTERVAL")
	viper.BindEnv("ultra_fast.precompute_instructions", "PUMPBOT_ULTRA_FAST_PRECOMPUTE_INSTRUCTIONS")
	viper.BindEnv("ultra_fast.rpc_timeout_ms", "PUMPBOT_ULTRA_FAST_RPC_TIMEOUT_MS")
	viper.BindEnv("ultra_fast.benchmark_mode", "PUMPBOT_ULTRA_FAST_BENCHMARK_MODE")
	viper.BindEnv("ultra_fast.log_latency", "PUMPBOT_ULTRA_FAST_LOG_LATENCY")
	viper.BindEnv("ultra_fast.max_tokens_per_second", "PUMPBOT_ULTRA_FAST_MAX_TOKENS_PER_SECOND")
	viper.BindEnv("ultra_fast.immediate_execution", "PUMPBOT_ULTRA_FAST_IMMEDIATE_EXECUTION")
}

// Validation for ultra-fast config
func validateUltraFastConfig(config *Config) error {
	if config.UltraFast.Enabled {
		// Safety checks for ultra-fast mode
		if config.UltraFast.ParallelWorkers > 10 {
			return fmt.Errorf("ultra_fast.parallel_workers must not exceed 10 (got %d)", config.UltraFast.ParallelWorkers)
		}

		if config.UltraFast.MaxTokensPerSecond > 100 {
			return fmt.Errorf("ultra_fast.max_tokens_per_second must not exceed 100 (got %d)", config.UltraFast.MaxTokensPerSecond)
		}

		if config.UltraFast.PriorityOverSafety {
			// Warn about extremely dangerous mode
			fmt.Println("⚠️ WARNING: Priority over safety mode enabled - this is extremely risky!")
		}

		if config.UltraFast.FireAndForget && !config.UltraFast.NoConfirmation {
			// Auto-enable no confirmation for fire and forget
			config.UltraFast.NoConfirmation = true
		}

		// Ensure minimum safety measures in ultra-fast mode
		if config.Trading.BuyAmountSOL > 1.0 && config.UltraFast.SkipValidation {
			return fmt.Errorf("buy amount too high for ultra-fast mode with skip validation: %.3f SOL", config.Trading.BuyAmountSOL)
		}
	}

	return nil
}

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
