package logger

import (
	"encoding/hex"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger represents the application logger
type Logger struct {
	*logrus.Logger
	config LogConfig
}

// LogConfig contains logger configuration
type LogConfig struct {
	Level       string
	Format      string // "json" or "text"
	LogToFile   bool
	LogFilePath string
	TradeLogDir string
}

// NewLogger creates a new logger instance
func NewLogger(config LogConfig) (*Logger, error) {
	log := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %s: %w", config.Level, err)
	}
	log.SetLevel(level)

	// Always output to stdout first
	log.SetOutput(os.Stdout)

	// Set log format based on configuration
	switch strings.ToLower(config.Format) {
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime:  "timestamp",
				logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg:   "message",
			},
		})
	case "text":
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05.000",
			ForceColors:     true,
			DisableQuote:    true,
		})
	default:
		// Default to a custom text format with clear timestamp
		log.SetFormatter(&CustomFormatter{})
	}

	// Create trade log directory if specified
	if config.TradeLogDir != "" {
		if err := os.MkdirAll(config.TradeLogDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create trade log directory %s: %w", config.TradeLogDir, err)
		}
	}

	// Optionally also log to file (in addition to stdout)
	if config.LogToFile && config.LogFilePath != "" {
		// Create log directory if it doesn't exist
		logDir := filepath.Dir(config.LogFilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
		}

		// Note: For dual output (stdout + file), you'd typically use a MultiWriter
		// For now, we prioritize stdout output as requested
	}

	return &Logger{
		Logger: log,
		config: config,
	}, nil
}

// CustomFormatter provides a clean, timestamped format for console output
type CustomFormatter struct{}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("2006-01-02 15:04:05.000")
	level := strings.ToUpper(entry.Level.String())

	// Color coding for different log levels
	var levelColor string
	switch entry.Level {
	case logrus.DebugLevel:
		levelColor = "\033[36m" // Cyan
	case logrus.InfoLevel:
		levelColor = "\033[32m" // Green
	case logrus.WarnLevel:
		levelColor = "\033[33m" // Yellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		levelColor = "\033[31m" // Red
	default:
		levelColor = "\033[0m" // Reset
	}

	resetColor := "\033[0m"

	// Build the log message
	msg := fmt.Sprintf("%s [%s%s%s] %s",
		timestamp,
		levelColor,
		level,
		resetColor,
		entry.Message)

	// Add fields if present
	if len(entry.Data) > 0 {
		msg += " |"
		for key, value := range entry.Data {
			msg += fmt.Sprintf(" %s=%v", key, value)
		}
	}

	msg += "\n"
	return []byte(msg), nil
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *logrus.Entry {
	return l.Logger.WithField(key, value)
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields logrus.Fields) *logrus.Entry {
	return l.Logger.WithFields(fields)
}

// WithError adds an error field to the logger
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// Trading-specific logging methods with timestamps

// LogTokenDiscovered logs when a new token is discovered
func (l *Logger) LogTokenDiscovered(mint, creator, name, symbol string) {
	l.WithFields(logrus.Fields{
		"event":     "token_discovered",
		"mint":      mint,
		"creator":   creator,
		"name":      name,
		"symbol":    symbol,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("🔍 New token discovered")
}

// LogTradeAttempt logs when a trade attempt is made
func (l *Logger) LogTradeAttempt(tradeType, mint string, amount float64, signature string) {
	l.WithFields(logrus.Fields{
		"event":     "trade_attempt",
		"type":      tradeType,
		"mint":      mint,
		"amount":    amount,
		"signature": signature,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("💰 Trade attempt initiated")
}

// LogTradeSuccess logs when a trade is successful
func (l *Logger) LogTradeSuccess(tradeType, mint string, amount float64, signature string, price float64) {
	l.WithFields(logrus.Fields{
		"event":     "trade_success",
		"type":      tradeType,
		"mint":      mint,
		"amount":    amount,
		"signature": signature,
		"price":     price,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("✅ Trade successful")
}

// LogTradeError logs when a trade fails
func (l *Logger) LogTradeError(tradeType, mint string, amount float64, err error) {
	l.WithFields(logrus.Fields{
		"event":     "trade_error",
		"type":      tradeType,
		"mint":      mint,
		"amount":    amount,
		"timestamp": time.Now().Format(time.RFC3339),
	}).WithError(err).Error("❌ Trade failed")
}

// LogFilterMatch logs when a token matches filters
func (l *Logger) LogFilterMatch(mint string, filterType, filterValue string) {
	l.WithFields(logrus.Fields{
		"event":        "filter_match",
		"mint":         mint,
		"filter_type":  filterType,
		"filter_value": filterValue,
		"timestamp":    time.Now().Format(time.RFC3339),
	}).Info("✓ Token matches filter")
}

// LogFilterReject logs when a token is rejected by filters
func (l *Logger) LogFilterReject(mint string, filterType, reason string) {
	l.WithFields(logrus.Fields{
		"event":       "filter_reject",
		"mint":        mint,
		"filter_type": filterType,
		"reason":      reason,
		"timestamp":   time.Now().Format(time.RFC3339),
	}).Info("✗ Token rejected by filter")
}

// LogWebSocketEvent logs WebSocket events
func (l *Logger) LogWebSocketEvent(eventType string, data interface{}) {
	l.WithFields(logrus.Fields{
		"event":     "websocket_" + eventType,
		"data":      data,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Debug("🔌 WebSocket event")
}

// LogPerformanceMetric logs performance metrics
func (l *Logger) LogPerformanceMetric(metric string, value interface{}, unit string) {
	l.WithFields(logrus.Fields{
		"event":     "performance_metric",
		"metric":    metric,
		"value":     value,
		"unit":      unit,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("📊 Performance metric")
}

// LogError logs general errors with context
func (l *Logger) LogError(component, operation string, err error, fields logrus.Fields) {
	logFields := logrus.Fields{
		"event":     "error",
		"component": component,
		"operation": operation,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// Merge additional fields
	for k, v := range fields {
		logFields[k] = v
	}

	l.WithFields(logFields).WithError(err).Error("💥 Component error")
}

// LogStartup logs application startup information
func (l *Logger) LogStartup(version, network, rpcUrl string, config interface{}) {
	l.WithFields(logrus.Fields{
		"event":     "startup",
		"version":   version,
		"network":   network,
		"rpc_url":   rpcUrl,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("🚀 Bot starting up")
}

// LogShutdown logs application shutdown information
func (l *Logger) LogShutdown(reason string) {
	l.WithFields(logrus.Fields{
		"event":     "shutdown",
		"reason":    reason,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("🛑 Bot shutting down")
}

// Context-aware logging methods

// WithComponent returns a logger with component context
func (l *Logger) WithComponent(component string) *logrus.Entry {
	return l.WithField("component", component)
}

// WithTransaction returns a logger with transaction context
func (l *Logger) WithTransaction(signature string) *logrus.Entry {
	return l.WithField("transaction", signature)
}

// WithToken returns a logger with token context
func (l *Logger) WithToken(mint string) *logrus.Entry {
	return l.WithField("mint", mint)
}

// Performance logging

// LogLatency logs operation latency
func (l *Logger) LogLatency(operation string, duration time.Duration) {
	l.WithFields(logrus.Fields{
		"event":     "latency",
		"operation": operation,
		"duration":  duration.Milliseconds(),
		"unit":      "ms",
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("⏱️ Operation latency")
}

// LogThroughput logs throughput metrics
func (l *Logger) LogThroughput(operation string, count int, duration time.Duration) {
	rate := float64(count) / duration.Seconds()
	l.WithFields(logrus.Fields{
		"event":     "throughput",
		"operation": operation,
		"count":     count,
		"duration":  duration.Seconds(),
		"rate":      rate,
		"unit":      "ops/sec",
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("📈 Operation throughput")
}

// Utility logging methods

// LogConnection logs connection status
func (l *Logger) LogConnection(service, status string, details interface{}) {
	l.WithFields(logrus.Fields{
		"event":     "connection",
		"service":   service,
		"status":    status,
		"details":   details,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("🔗 Connection status")
}

// LogBalance logs wallet balance information
func (l *Logger) LogBalance(balanceSOL float64, balanceLamports uint64) {
	l.WithFields(logrus.Fields{
		"event":            "balance_check",
		"balance_sol":      balanceSOL,
		"balance_lamports": balanceLamports,
		"timestamp":        time.Now().Format(time.RFC3339),
	}).Info("💰 Wallet balance")
}

// LogTransaction logs transaction information
func (l *Logger) LogTransaction(signature, status string, fee uint64) {
	l.WithFields(logrus.Fields{
		"event":     "transaction",
		"signature": signature,
		"status":    status,
		"fee":       fee,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("📋 Transaction status")
}

func (l *Logger) LogInstruction(ix solana.Instruction) {
	fmt.Println("----- Associated Token Account Instruction -----")
	fmt.Printf("ProgramID: %s\n", ix.ProgramID().String())
	fmt.Printf("Accounts:\n")
	for i, acc := range ix.Accounts() {
		fmt.Printf("  %d. %s (isSigner: %v, isWritable: %v)\n", i, acc.PublicKey, acc.IsSigner, acc.IsWritable)
	}
	data, err := ix.Data()
	if err != nil {
		fmt.Printf("Data: <error: %v>\n", err)
	} else if len(data) > 0 {
		fmt.Printf("Data: %s\n", hex.EncodeToString(data))
	} else {
		fmt.Println("Data: <none>")
	}
	fmt.Println("-----------------------------------------------")
}
