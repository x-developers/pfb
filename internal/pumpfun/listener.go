package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/blocto/solana-go-sdk/common"
	"strings"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/pkg/utils"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

const CREATE_DISCRIMINATOR uint64 = 8530921459188068891

// TokenEvent represents a new token creation event
type TokenEvent struct {
	Signature              string            `json:"signature"`
	Slot                   uint64            `json:"slot"`
	BlockTime              int64             `json:"block_time"`
	Mint                   *common.PublicKey `json:"mint"`
	BondingCurve           *common.PublicKey `json:"bonding_curve"`
	AssociatedBondingCurve *common.PublicKey `json:"associated_bonding_curve"`
	Creator                *common.PublicKey `json:"creator"`
	CreatorVault           *common.PublicKey `json:"creator_vault"`
	User                   *common.PublicKey `json:"user"`
	Name                   string            `json:"name"`
	Symbol                 string            `json:"symbol"`
	URI                    string            `json:"uri"`
	InitialPrice           float64           `json:"initial_price"`
	Timestamp              time.Time         `json:"timestamp"`
}

// Listener listens for new pump.fun tokens
type Listener struct {
	wsClient      *solana.WSClient
	rpcClient     *solana.Client
	logger        *logger.Logger
	config        *config.Config
	tokenChan     chan *TokenEvent
	ctx           context.Context
	cancel        context.CancelFunc
	pdaDerivation *utils.PumpFunPDADerivation
}

// NewListener creates a new pump.fun listener
func NewListener(wsClient *solana.WSClient, rpcClient *solana.Client, logger *logger.Logger, cfg *config.Config) *Listener {
	ctx, cancel := context.WithCancel(context.Background())

	return &Listener{
		wsClient:      wsClient,
		rpcClient:     rpcClient,
		logger:        logger,
		config:        cfg,
		tokenChan:     make(chan *TokenEvent, 100),
		ctx:           ctx,
		cancel:        cancel,
		pdaDerivation: utils.NewPumpFunPDADerivation(),
	}
}

// Start starts the listener
func (l *Listener) Start() error {
	l.logger.Info("Starting pump.fun token listener...")

	// Connect WebSocket
	if err := l.wsClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	// Subscribe to pump.fun program logs
	pumpFunProgramID := base58.Encode(config.PumpFunProgramID)

	_, err := l.wsClient.SubscribeToLogs(pumpFunProgramID, l.handleLogsNotification)
	if err != nil {
		return fmt.Errorf("failed to subscribe to logs: %w", err)
	}

	return nil
}

// Stop stops the listener
func (l *Listener) Stop() error {
	l.cancel()
	close(l.tokenChan)
	return l.wsClient.Disconnect()
}

// GetTokenChannel returns the token event channel
func (l *Listener) GetTokenChannel() <-chan *TokenEvent {
	return l.tokenChan
}

// handleLogsNotification handles logs notifications from WebSocket with enhanced processing
func (l *Listener) handleLogsNotification(data interface{}) error {
	notification, ok := data.(solana.LogsNotification)
	if !ok {
		return fmt.Errorf("invalid logs notification format")
	}

	// Skip if transaction failed
	if notification.Result.Value.Err != nil {
		l.logger.WithField("signature", notification.Result.Value.Signature).Debug("Skipping failed transaction")
		return nil
	}

	signature := notification.Result.Value.Signature
	logs := notification.Result.Value.Logs
	slot := notification.Result.Context.Slot

	l.logger.WithFields(logrus.Fields{
		"signature":  signature,
		"slot":       slot,
		"logs_count": len(logs),
	}).Debug("Processing logs notification")

	// Process program logs to extract token information
	tokenInfo, err := l.processProgramLogs(logs, signature, slot)
	if err != nil {
		l.logger.WithError(err).WithField("signature", signature).Debug("Failed to process program logs")
		return nil
	}

	if tokenInfo == nil {
		l.logger.WithField("signature", signature).Debug("No token creation found in logs")
		return nil
	}

	// Send token event to channel
	select {
	case l.tokenChan <- tokenInfo:
		l.logger.LogTokenDiscovered(tokenInfo.Mint.String(), tokenInfo.Creator.String(), tokenInfo.Name, tokenInfo.Symbol)
	case <-l.ctx.Done():
		return nil
	default:
		l.logger.Warn("Token channel full, dropping event")
	}

	return nil
}

// processProgramLogs processes program logs to extract token creation information
func (l *Listener) processProgramLogs(logs []string, signature string, slot uint64) (*TokenEvent, error) {
	var processLogs bool

	// Check if this is a token creation
	for _, log := range logs {
		if strings.Contains(log, "Program log: Instruction: Create") {
			processLogs = true
			break
		}
	}

	// Skip swaps as the first condition may pass them
	for _, log := range logs {
		if strings.Contains(log, "Program log: Instruction: CreateTokenAccount") {
			processLogs = false
			break
		}
	}

	if !processLogs {
		return nil, nil
	}

	for _, log := range logs {
		// Check for pump.fun program log patterns
		if strings.Contains(log, "Program data: ") {
			logContent := strings.TrimPrefix(log, "Program data: ")
			tokenEvent, err := l.extractTokenFromData(logContent, signature, slot)
			if err == nil {
				return tokenEvent, err
			}
		}
	}

	return nil, nil
}

func (l *Listener) extractTokenFromData(dataStr string, signature string, slot uint64) (*TokenEvent, error) {
	tokenEvent := &TokenEvent{
		Signature: signature,
		Slot:      slot,
		Timestamp: time.Now(),
	}

	data, err := utils.DecodeDataString(dataStr)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("too short for discriminator")
	}
	discriminator := binary.LittleEndian.Uint64(data[:8])
	if discriminator != CREATE_DISCRIMINATOR {
		return nil, fmt.Errorf("not a CreateInstruction discriminator: %x", discriminator)
	}
	offset := 8
	readString := func() (string, error) {
		if offset+4 > len(data) {
			return "", fmt.Errorf("not enough data for string length")
		}
		l := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		if offset+int(l) > len(data) {
			return "", fmt.Errorf("not enough data for string value")
		}
		val := string(data[offset : offset+int(l)])
		offset += int(l)
		return val, nil
	}
	readPubKey := func() (*common.PublicKey, error) {
		if offset+32 > len(data) {
			return nil, fmt.Errorf("not enough data for pubkey")
		}
		val := common.PublicKeyFromBytes(data[offset : offset+32])
		offset += 32
		return &val, nil
	}

	if tokenEvent.Name, err = readString(); err != nil {
		return nil, err
	}
	if tokenEvent.Symbol, err = readString(); err != nil {
		return nil, err
	}
	if tokenEvent.URI, err = readString(); err != nil {
		return nil, err
	}
	if tokenEvent.Mint, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.BondingCurve, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.User, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.Creator, err = readPubKey(); err != nil {
		return nil, err
	}
	if tokenEvent.CreatorVault, _, err = l.pdaDerivation.DeriveCreatorVault(*tokenEvent.Creator); err != nil {
		return nil, err
	}
	if tokenEvent.AssociatedBondingCurve, _, err = l.pdaDerivation.DeriveAssociatedBondingCurve(*tokenEvent.Mint, *tokenEvent.BondingCurve); err != nil {
		return nil, err
	}

	return tokenEvent, nil
}
