package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TradeLog represents a trade log entry
type TradeLog struct {
	Timestamp    time.Time `json:"timestamp"`
	TradeType    string    `json:"trade_type"`              // "buy" or "sell"
	Mint         string    `json:"mint"`                    // Token mint address
	TokenName    string    `json:"token_name"`              // Token name
	TokenSymbol  string    `json:"token_symbol"`            // Token symbol
	Creator      string    `json:"creator"`                 // Token creator address
	AmountSOL    float64   `json:"amount_sol"`              // Amount in SOL
	AmountTokens float64   `json:"amount_tokens"`           // Amount in tokens
	Price        float64   `json:"price"`                   // Price per token in SOL
	Signature    string    `json:"signature"`               // Transaction signature
	Status       string    `json:"status"`                  // "success", "failed", "pending"
	ErrorMessage string    `json:"error_message,omitempty"` // Error if failed
	GasFee       uint64    `json:"gas_fee"`                 // Gas fee in lamports
	SlippageBP   int       `json:"slippage_bp"`             // Slippage in basis points
	Strategy     string    `json:"strategy"`                // Strategy used
	ProfitLoss   float64   `json:"profit_loss,omitempty"`   // P&L for sell trades
}

// PositionLog represents a position tracking entry
type PositionLog struct {
	Timestamp     time.Time `json:"timestamp"`
	Mint          string    `json:"mint"`
	TokenName     string    `json:"token_name"`
	TokenSymbol   string    `json:"token_symbol"`
	Position      float64   `json:"position"`       // Token amount held
	AvgBuyPrice   float64   `json:"avg_buy_price"`  // Average buy price
	CurrentPrice  float64   `json:"current_price"`  // Current price
	UnrealizedPL  float64   `json:"unrealized_pl"`  // Unrealized P&L
	TotalInvested float64   `json:"total_invested"` // Total SOL invested
	CurrentValue  float64   `json:"current_value"`  // Current value in SOL
}

// TradeLogger handles trade-specific logging
type TradeLogger struct {
	baseDir   string
	logger    *Logger
	positions map[string]*PositionLog // mint -> position
}

// NewTradeLogger creates a new trade logger
func NewTradeLogger(baseDir string, logger *Logger) (*TradeLogger, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create trade log directory: %w", err)
	}

	return &TradeLogger{
		baseDir:   baseDir,
		logger:    logger,
		positions: make(map[string]*PositionLog),
	}, nil
}

// LogTrade logs a trade to both structured logs and trade files
func (tl *TradeLogger) LogTrade(trade TradeLog) error {
	// Log to main logger
	tl.logger.WithFields(map[string]interface{}{
		"event":         "trade_logged",
		"trade_type":    trade.TradeType,
		"mint":          trade.Mint,
		"token_name":    trade.TokenName,
		"amount_sol":    trade.AmountSOL,
		"amount_tokens": trade.AmountTokens,
		"price":         trade.Price,
		"signature":     trade.Signature,
		"status":        trade.Status,
	}).Info("Trade logged")

	// Write to daily trade file
	filename := fmt.Sprintf("trades_%s.jsonl", time.Now().Format("2006-01-02"))
	filepath := filepath.Join(tl.baseDir, filename)

	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open trade log file: %w", err)
	}
	defer file.Close()

	// Write trade as JSON line
	tradeBytes, err := json.Marshal(trade)
	if err != nil {
		return fmt.Errorf("failed to marshal trade: %w", err)
	}

	if _, err := file.Write(append(tradeBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write trade to file: %w", err)
	}

	// Update position tracking
	tl.updatePosition(trade)

	return nil
}

// LogBuy logs a buy trade
func (tl *TradeLogger) LogBuy(mint, tokenName, tokenSymbol, creator string,
	amountSOL, amountTokens, price float64, signature string,
	status string, errorMsg string, gasFee uint64, slippageBP int, strategy string) error {

	trade := TradeLog{
		Timestamp:    time.Now(),
		TradeType:    "buy",
		Mint:         mint,
		TokenName:    tokenName,
		TokenSymbol:  tokenSymbol,
		Creator:      creator,
		AmountSOL:    amountSOL,
		AmountTokens: amountTokens,
		Price:        price,
		Signature:    signature,
		Status:       status,
		ErrorMessage: errorMsg,
		GasFee:       gasFee,
		SlippageBP:   slippageBP,
		Strategy:     strategy,
	}

	return tl.LogTrade(trade)
}

// LogSell logs a sell trade
func (tl *TradeLogger) LogSell(mint, tokenName, tokenSymbol string,
	amountSOL, amountTokens, price float64, signature string,
	status string, errorMsg string, gasFee uint64, slippageBP int,
	strategy string, profitLoss float64) error {

	trade := TradeLog{
		Timestamp:    time.Now(),
		TradeType:    "sell",
		Mint:         mint,
		TokenName:    tokenName,
		TokenSymbol:  tokenSymbol,
		AmountSOL:    amountSOL,
		AmountTokens: amountTokens,
		Price:        price,
		Signature:    signature,
		Status:       status,
		ErrorMessage: errorMsg,
		GasFee:       gasFee,
		SlippageBP:   slippageBP,
		Strategy:     strategy,
		ProfitLoss:   profitLoss,
	}

	return tl.LogTrade(trade)
}

// updatePosition updates position tracking based on trade
func (tl *TradeLogger) updatePosition(trade TradeLog) {
	position, exists := tl.positions[trade.Mint]
	if !exists {
		position = &PositionLog{
			Mint:        trade.Mint,
			TokenName:   trade.TokenName,
			TokenSymbol: trade.TokenSymbol,
		}
		tl.positions[trade.Mint] = position
	}

	position.Timestamp = trade.Timestamp

	switch trade.TradeType {
	case "buy":
		if trade.Status == "success" {
			// Update average buy price
			totalValue := position.Position*position.AvgBuyPrice + trade.AmountTokens*trade.Price
			position.Position += trade.AmountTokens
			if position.Position > 0 {
				position.AvgBuyPrice = totalValue / position.Position
			}
			position.TotalInvested += trade.AmountSOL
		}
	case "sell":
		if trade.Status == "success" {
			position.Position -= trade.AmountTokens
			if position.Position < 0 {
				position.Position = 0 // Prevent negative positions
			}
			position.TotalInvested -= trade.AmountTokens * position.AvgBuyPrice
			if position.TotalInvested < 0 {
				position.TotalInvested = 0
			}
		}
	}

	// Calculate current value and unrealized P&L
	position.CurrentPrice = trade.Price
	position.CurrentValue = position.Position * position.CurrentPrice
	position.UnrealizedPL = position.CurrentValue - position.TotalInvested
}

// LogPosition logs current position to positions file
func (tl *TradeLogger) LogPosition(mint string) error {
	position, exists := tl.positions[mint]
	if !exists {
		return fmt.Errorf("position not found for mint: %s", mint)
	}

	filename := fmt.Sprintf("positions_%s.jsonl", time.Now().Format("2006-01-02"))
	filepath := filepath.Join(tl.baseDir, filename)

	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open position log file: %w", err)
	}
	defer file.Close()

	positionBytes, err := json.Marshal(position)
	if err != nil {
		return fmt.Errorf("failed to marshal position: %w", err)
	}

	if _, err := file.Write(append(positionBytes, '\n')); err != nil {
		return fmt.Errorf("failed to write position to file: %w", err)
	}

	return nil
}

// GetPosition returns current position for a token
func (tl *TradeLogger) GetPosition(mint string) (*PositionLog, bool) {
	position, exists := tl.positions[mint]
	return position, exists
}

// GetAllPositions returns all current positions
func (tl *TradeLogger) GetAllPositions() map[string]*PositionLog {
	return tl.positions
}

// LogDailySummary creates a daily trading summary
func (tl *TradeLogger) LogDailySummary() error {
	summary := struct {
		Date            string                  `json:"date"`
		Timestamp       time.Time               `json:"timestamp"`
		TotalTrades     int                     `json:"total_trades"`
		TotalBuys       int                     `json:"total_buys"`
		TotalSells      int                     `json:"total_sells"`
		TotalVolume     float64                 `json:"total_volume_sol"`
		TotalProfitLoss float64                 `json:"total_profit_loss"`
		ActivePositions int                     `json:"active_positions"`
		Positions       map[string]*PositionLog `json:"positions"`
	}{
		Date:      time.Now().Format("2006-01-02"),
		Timestamp: time.Now(),
		Positions: tl.positions,
	}

	// Calculate summary statistics
	totalPL := 0.0
	activePositions := 0
	for _, position := range tl.positions {
		if position.Position > 0 {
			activePositions++
		}
		totalPL += position.UnrealizedPL
	}

	summary.ActivePositions = activePositions
	summary.TotalProfitLoss = totalPL

	filename := fmt.Sprintf("summary_%s.json", time.Now().Format("2006-01-02"))
	filepath := filepath.Join(tl.baseDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create summary file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	tl.logger.WithFields(map[string]interface{}{
		"event":            "daily_summary",
		"active_positions": activePositions,
		"total_pl":         totalPL,
	}).Info("Daily summary logged")

	return nil
}
