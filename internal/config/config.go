package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	// Network settings
	Network   string `mapstructure:"network" yaml:"network"`
	RPCUrl    string `mapstructure:"rpc_url" yaml:"rpc_url"`
	WSUrl     string `mapstructure:"ws_url" yaml:"ws_url"`
	RPCAPIKey string `mapstructure:"rpc_api_key" yaml:"rpc_api_key"`

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
}

// TradingConfig contains trading-related settings
type TradingConfig struct {
	BuyAmountSOL      float64 `mapstructure:"buy_amount_sol" yaml:"buy_amount_sol"`
	SlippageBP        int     `mapstructure:"slippage_bp" yaml:"slippage_bp"`
	MaxGasPrice       uint64  `mapstructure:"max_gas_price" yaml:"max_gas_price"`
	PriorityFee       uint64  `mapstructure:"priority_fee" yaml:"priority_fee"`
	AutoSell          bool    `mapstructure:"auto_sell" yaml:"auto_sell"`
	SellDelaySeconds  int     `mapstructure:"sell_delay_seconds" yaml:"sell_delay_seconds"`
	SellPercentage    float64 `mapstructure:"sell_percentage" yaml:"sell_percentage"`
	TakeProfitPercent float64 `mapstructure:"take_profit_percent" yaml:"take_profit_percent"`
	StopLossPercent   float64 `mapstructure:"stop_loss_percent" yaml:"stop_loss_percent"`
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

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{}

	// Set default values
	setDefaults()

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

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, continue with defaults and env vars
	}

	// Unmarshal config
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate and post-process config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Network defaults
	viper.SetDefault("network", "mainnet")
	viper.SetDefault("rpc_url", "")
	viper.SetDefault("ws_url", "")

	// Trading defaults
	viper.SetDefault("trading.buy_amount_sol", DefaultBuyAmountSOL)
	viper.SetDefault("trading.slippage_bp", DefaultSlippageBP)
	viper.SetDefault("trading.max_gas_price", 0)
	viper.SetDefault("trading.priority_fee", 0)
	viper.SetDefault("trading.auto_sell", false)
	viper.SetDefault("trading.sell_delay_seconds", 30)
	viper.SetDefault("trading.sell_percentage", 100.0)
	viper.SetDefault("trading.take_profit_percent", 50.0)
	viper.SetDefault("trading.stop_loss_percent", -20.0)

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
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.log_to_file", true)
	viper.SetDefault("logging.log_file_path", "logs/bot.log")
	viper.SetDefault("logging.trade_log_dir", "trades")

	// Advanced defaults
	viper.SetDefault("advanced.max_retries", MaxRetries)
	viper.SetDefault("advanced.retry_delay_ms", RetryDelayMs)
	viper.SetDefault("advanced.confirm_timeout_sec", ConfirmTimeoutSec)
	viper.SetDefault("advanced.enable_metrics", false)
	viper.SetDefault("advanced.metrics_port", 8080)
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
func GetConfigFromEnv() *Config {
	config := &Config{
		Network:    getEnvString("PUMPBOT_NETWORK", "mainnet"),
		RPCUrl:     getEnvString("PUMPBOT_RPC_URL", ""),
		WSUrl:      getEnvString("PUMPBOT_WS_URL", ""),
		RPCAPIKey:  getEnvString("PUMPBOT_RPC_API_KEY", ""),
		PrivateKey: getEnvString("PUMPBOT_PRIVATE_KEY", ""),
		Trading: TradingConfig{
			BuyAmountSOL:      getEnvFloat("PUMPBOT_TRADING_BUY_AMOUNT_SOL", DefaultBuyAmountSOL),
			SlippageBP:        getEnvInt("PUMPBOT_TRADING_SLIPPAGE_BP", DefaultSlippageBP),
			AutoSell:          getEnvBool("PUMPBOT_TRADING_AUTO_SELL", false),
			SellDelaySeconds:  getEnvInt("PUMPBOT_TRADING_SELL_DELAY_SECONDS", 30),
			SellPercentage:    getEnvFloat("PUMPBOT_TRADING_SELL_PERCENTAGE", 100.0),
			TakeProfitPercent: getEnvFloat("PUMPBOT_TRADING_TAKE_PROFIT_PERCENT", 50.0),
			StopLossPercent:   getEnvFloat("PUMPBOT_TRADING_STOP_LOSS_PERCENT", -20.0),
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
		Logging: LoggingConfig{
			Level:       getEnvString("PUMPBOT_LOGGING_LEVEL", "info"),
			Format:      getEnvString("PUMPBOT_LOGGING_FORMAT", "json"),
			LogToFile:   getEnvBool("PUMPBOT_LOGGING_LOG_TO_FILE", true),
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

	// Set URLs if not provided
	if config.RPCUrl == "" {
		config.RPCUrl = GetRPCEndpoint(config.Network)
	}
	if config.WSUrl == "" {
		config.WSUrl = GetWSEndpoint(config.Network)
	}

	return config
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
		if intValue := parseInt(value); intValue != 0 {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue := parseFloat(value); floatValue != 0 {
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

// Simple parsing functions (for brevity - in production use strconv)
func parseInt(s string) int {
	// Implementation would use strconv.Atoi
	return 0
}

func parseFloat(s string) float64 {
	// Implementation would use strconv.ParseFloat
	return 0.0
}
