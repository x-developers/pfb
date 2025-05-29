package wallet

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/pkg/utils"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

// Wallet represents a Solana wallet
type Wallet struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	rpcClient  *solana.Client
	logger     *logrus.Logger
	config     *config.Config
}

// WalletConfig contains wallet configuration
type WalletConfig struct {
	PrivateKey string
	Network    string
}

// NewWallet creates a new wallet instance
func NewWallet(cfg WalletConfig, rpcClient *solana.Client, logger *logrus.Logger, config *config.Config) (*Wallet, error) {
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("private key is required")
	}

	// Decode private key from base58
	privateKeyBytes, err := base58.Decode(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key format: %w", err)
	}

	if len(privateKeyBytes) != 64 {
		return nil, fmt.Errorf("invalid private key length: expected 64 bytes, got %d", len(privateKeyBytes))
	}

	// Create ed25519 private key
	privateKey := ed25519.PrivateKey(privateKeyBytes)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	wallet := &Wallet{
		privateKey: privateKey,
		publicKey:  publicKey,
		rpcClient:  rpcClient,
		logger:     logger,
		config:     config,
	}

	logger.WithFields(logrus.Fields{
		"public_key": base58.Encode(publicKey),
		"network":    cfg.Network,
	}).Info("Wallet initialized")

	return wallet, nil
}

// GetPublicKey returns the wallet's public key as bytes
func (w *Wallet) GetPublicKey() []byte {
	return w.publicKey
}

// GetPublicKeyString returns the wallet's public key as base58 string
func (w *Wallet) GetPublicKeyString() string {
	return base58.Encode(w.publicKey)
}

// GetPrivateKey returns the wallet's private key (use with caution)
func (w *Wallet) GetPrivateKey() ed25519.PrivateKey {
	return w.privateKey
}

// Sign signs data with the wallet's private key
func (w *Wallet) Sign(data []byte) []byte {
	return ed25519.Sign(w.privateKey, data)
}

// GetBalance returns the wallet's SOL balance in lamports
func (w *Wallet) GetBalance(ctx context.Context) (uint64, error) {
	balance, err := w.rpcClient.GetBalance(ctx, w.GetPublicKeyString())
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"balance_lamports": balance,
		"balance_sol":      utils.ConvertLamportsToSOL(balance),
	}).Debug("Retrieved wallet balance")

	return balance, nil
}

// GetBalanceSOL returns the wallet's SOL balance as float64
func (w *Wallet) GetBalanceSOL(ctx context.Context) (float64, error) {
	balance, err := w.GetBalance(ctx)
	if err != nil {
		return 0, err
	}
	return utils.ConvertLamportsToSOL(balance), nil
}

// ValidateBalance checks if wallet has sufficient balance for a transaction
func (w *Wallet) ValidateBalance(ctx context.Context, requiredLamports uint64) error {
	balance, err := w.GetBalance(ctx)
	if err != nil {
		return fmt.Errorf("failed to check balance: %w", err)
	}

	if balance < requiredLamports {
		return fmt.Errorf("insufficient balance: have %d lamports (%.6f SOL), need %d lamports (%.6f SOL)",
			balance, utils.ConvertLamportsToSOL(balance),
			requiredLamports, utils.ConvertLamportsToSOL(requiredLamports))
	}

	return nil
}

// ValidateBalanceSOL checks if wallet has sufficient SOL balance
func (w *Wallet) ValidateBalanceSOL(ctx context.Context, requiredSOL float64) error {
	requiredLamports := utils.ConvertSOLToLamports(requiredSOL)
	return w.ValidateBalance(ctx, requiredLamports)
}

// SolanaTransaction represents a Solana transaction in the wire format
type SolanaTransaction struct {
	Signatures [][]byte
	Message    *SolanaMessage
	serialized []byte
}

// SolanaMessage represents a Solana transaction message
type SolanaMessage struct {
	Header          MessageHeader
	AccountKeys     [][]byte
	RecentBlockhash [32]byte
	Instructions    []CompiledInstruction
}

// MessageHeader represents the transaction message header
type MessageHeader struct {
	NumRequiredSignatures       uint8
	NumReadonlySignedAccounts   uint8
	NumReadonlyUnsignedAccounts uint8
}

// CompiledInstruction represents a compiled instruction
type CompiledInstruction struct {
	ProgramIdIndex uint8
	Accounts       []uint8
	Data           []byte
}

// CreateTransaction creates a new Solana transaction
func (w *Wallet) CreateTransaction(ctx context.Context, instructions []CompiledInstruction, additionalSigners [][]byte) (*SolanaTransaction, error) {
	// Get recent blockhash
	blockhash, err := w.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Decode blockhash
	blockhashBytes, err := base58.Decode(blockhash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode blockhash: %w", err)
	}

	var blockhashArray [32]byte
	copy(blockhashArray[:], blockhashBytes)

	// Collect unique account keys
	accountKeys := [][]byte{w.publicKey} // Payer is always first
	accountKeyMap := make(map[string]int)
	accountKeyMap[string(w.publicKey)] = 0

	// Add additional signers
	numSigners := 1 // Payer
	for _, signer := range additionalSigners {
		signerStr := string(signer)
		if _, exists := accountKeyMap[signerStr]; !exists {
			accountKeys = append(accountKeys, signer)
			accountKeyMap[signerStr] = len(accountKeys) - 1
			numSigners++
		}
	}

	// Add program accounts and instruction accounts from instructions
	for _, inst := range instructions {
		// Add program account
		programKey := make([]byte, 32) // Placeholder - would need actual program keys
		programStr := string(programKey)
		if _, exists := accountKeyMap[programStr]; !exists {
			accountKeys = append(accountKeys, programKey)
			accountKeyMap[programStr] = len(accountKeys) - 1
		}

		// Update program ID index
		inst.ProgramIdIndex = uint8(accountKeyMap[programStr])
	}

	// Create message header
	header := MessageHeader{
		NumRequiredSignatures:       uint8(numSigners),
		NumReadonlySignedAccounts:   0,
		NumReadonlyUnsignedAccounts: uint8(len(accountKeys) - numSigners),
	}

	// Create message
	message := &SolanaMessage{
		Header:          header,
		AccountKeys:     accountKeys,
		RecentBlockhash: blockhashArray,
		Instructions:    instructions,
	}

	// Create transaction
	transaction := &SolanaTransaction{
		Signatures: make([][]byte, numSigners),
		Message:    message,
	}

	w.logger.WithFields(logrus.Fields{
		"blockhash":    blockhash,
		"instructions": len(instructions),
		"signers":      numSigners,
		"accounts":     len(accountKeys),
	}).Debug("Created transaction")

	return transaction, nil
}

// SignTransaction signs a transaction with the wallet's private key
func (w *Wallet) SignTransaction(transaction *SolanaTransaction) error {
	// Serialize message for signing
	messageBytes, err := w.serializeMessage(transaction.Message)
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Sign with wallet's private key (payer signature goes first)
	signature := ed25519.Sign(w.privateKey, messageBytes)
	transaction.Signatures[0] = signature

	w.logger.WithField("signature", base58.Encode(signature)).Debug("Signed transaction")

	return nil
}

// SendTransaction sends a signed transaction to the network
func (w *Wallet) SendTransaction(ctx context.Context, transaction *SolanaTransaction) (string, error) {
	// Serialize transaction for network transmission
	serializedTx, err := w.serializeTransaction(transaction)
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	// Encode as base64 for RPC
	encodedTx := base64.StdEncoding.EncodeToString(serializedTx)

	// Send transaction
	signature, err := w.rpcClient.SendTransaction(ctx, encodedTx)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	w.logger.WithField("signature", signature).Info("Transaction sent")

	return signature, nil
}

// SendAndConfirmTransaction sends a transaction and waits for confirmation
func (w *Wallet) SendAndConfirmTransaction(ctx context.Context, transaction *SolanaTransaction) (string, error) {
	// Send transaction
	signature, err := w.SendTransaction(ctx, transaction)
	if err != nil {
		return "", err
	}

	// Wait for confirmation with timeout
	confirmCtx, cancel := context.WithTimeout(ctx, time.Duration(w.config.Advanced.ConfirmTimeoutSec)*time.Second)
	defer cancel()

	err = w.WaitForConfirmation(confirmCtx, signature)
	if err != nil {
		return signature, fmt.Errorf("transaction sent but confirmation failed: %w", err)
	}

	w.logger.WithField("signature", signature).Info("Transaction confirmed")

	return signature, nil
}

// WaitForConfirmation waits for transaction confirmation
func (w *Wallet) WaitForConfirmation(ctx context.Context, signature string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			err := w.rpcClient.ConfirmTransaction(ctx, signature)
			if err == nil {
				return nil // Transaction confirmed
			}
			// Continue waiting if transaction not confirmed yet
			w.logger.WithField("signature", signature).Debug("Waiting for confirmation...")
		}
	}
}

// serializeMessage serializes a transaction message for signing
func (w *Wallet) serializeMessage(message *SolanaMessage) ([]byte, error) {
	var buf []byte

	// Add header
	buf = append(buf, message.Header.NumRequiredSignatures)
	buf = append(buf, message.Header.NumReadonlySignedAccounts)
	buf = append(buf, message.Header.NumReadonlyUnsignedAccounts)

	// Add account keys
	buf = append(buf, encodeLength(len(message.AccountKeys))...)
	for _, key := range message.AccountKeys {
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid account key length: %d", len(key))
		}
		buf = append(buf, key...)
	}

	// Add recent blockhash
	buf = append(buf, message.RecentBlockhash[:]...)

	// Add instructions
	buf = append(buf, encodeLength(len(message.Instructions))...)
	for _, inst := range message.Instructions {
		buf = append(buf, inst.ProgramIdIndex)
		buf = append(buf, encodeLength(len(inst.Accounts))...)
		buf = append(buf, inst.Accounts...)
		buf = append(buf, encodeLength(len(inst.Data))...)
		buf = append(buf, inst.Data...)
	}

	return buf, nil
}

// serializeTransaction serializes a complete transaction
func (w *Wallet) serializeTransaction(transaction *SolanaTransaction) ([]byte, error) {
	if len(transaction.Signatures) == 0 {
		return nil, fmt.Errorf("transaction must have at least one signature")
	}

	var buf []byte

	// Add signatures
	buf = append(buf, encodeLength(len(transaction.Signatures))...)
	for _, sig := range transaction.Signatures {
		if len(sig) != 64 {
			return nil, fmt.Errorf("invalid signature length: %d", len(sig))
		}
		buf = append(buf, sig...)
	}

	// Add message
	messageBytes, err := w.serializeMessage(transaction.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}
	buf = append(buf, messageBytes...)

	return buf, nil
}

// encodeLength encodes length in compact-u16 format
func encodeLength(length int) []byte {
	if length < 0x80 {
		return []byte{byte(length)}
	} else if length < 0x4000 {
		return []byte{
			byte(length&0x7f) | 0x80,
			byte(length >> 7),
		}
	} else if length < 0x200000 {
		return []byte{
			byte(length&0x7f) | 0x80,
			byte((length>>7)&0x7f) | 0x80,
			byte(length >> 14),
		}
	}
	// For very large lengths, use 4 bytes
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(length)|0x80808080)
	return buf
}

// EstimateTransactionFee estimates the fee for a transaction
func (w *Wallet) EstimateTransactionFee(instructions []CompiledInstruction) uint64 {
	// Base fee per signature (5000 lamports)
	baseFee := uint64(5000)

	// Additional fee per instruction (very small)
	instructionFee := uint64(len(instructions)) * 100

	// Priority fee from config
	priorityFee := w.config.Trading.PriorityFee

	totalFee := baseFee + instructionFee + priorityFee

	w.logger.WithFields(logrus.Fields{
		"base_fee":        baseFee,
		"instruction_fee": instructionFee,
		"priority_fee":    priorityFee,
		"total_fee":       totalFee,
	}).Debug("Estimated transaction fee")

	return totalFee
}

// GetNetworkInfo returns network information
func (w *Wallet) GetNetworkInfo(ctx context.Context) (map[string]interface{}, error) {
	slot, err := w.rpcClient.GetSlot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current slot: %w", err)
	}

	blockhash, err := w.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	balance, err := w.GetBalance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	info := map[string]interface{}{
		"slot":             slot,
		"blockhash":        blockhash,
		"network":          w.config.Network,
		"public_key":       w.GetPublicKeyString(),
		"balance_lamports": balance,
		"balance_sol":      utils.ConvertLamportsToSOL(balance),
	}

	return info, nil
}

// ValidateAddress validates a Solana address
func (w *Wallet) ValidateAddress(address string) error {
	if !utils.IsValidSolanaAddress(address) {
		return fmt.Errorf("invalid Solana address: %s", address)
	}
	return nil
}
