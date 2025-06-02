package wallet

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/solana"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/sirupsen/logrus"
)

// Wallet represents a Solana wallet
type Wallet struct {
	account   types.Account
	rpcClient *solana.Client
	logger    *logrus.Logger
	config    *config.Config
}

// WalletConfig contains wallet configuration
type WalletConfig struct {
	PrivateKey string
	Network    string
}

// NewWallet creates a new wallet instance from private key
func NewWallet(cfg WalletConfig, rpcClient *solana.Client, logger *logrus.Logger, config *config.Config) (*Wallet, error) {
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("private key is required")
	}

	// Create account from base58 private key
	account, err := types.AccountFromBase58(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	wallet := &Wallet{
		account:   account,
		rpcClient: rpcClient,
		logger:    logger,
		config:    config,
	}

	logger.WithFields(logrus.Fields{
		"public_key": wallet.GetPublicKeyString(),
		"network":    cfg.Network,
	}).Info("Wallet initialized")

	return wallet, nil
}

// GetPublicKey returns the wallet's public key
func (w *Wallet) GetPublicKey() common.PublicKey {
	return w.account.PublicKey
}

// GetPublicKeyString returns the wallet's public key as base58 string
func (w *Wallet) GetPublicKeyString() string {
	return w.account.PublicKey.String()
}

// GetAccount returns the wallet's account for signing transactions
func (w *Wallet) GetAccount() types.Account {
	return w.account
}

// GetBalance returns the wallet's SOL balance in lamports
func (w *Wallet) GetBalance(ctx context.Context) (uint64, error) {
	balance, err := w.rpcClient.GetBalance(ctx, w.GetPublicKeyString())
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"balance_lamports": balance,
		"balance_sol":      config.ConvertLamportsToSOL(balance),
	}).Debug("Retrieved wallet balance")

	return balance, nil
}

// GetBalanceSOL returns the wallet's SOL balance as float64
func (w *Wallet) GetBalanceSOL(ctx context.Context) (float64, error) {
	balance, err := w.GetBalance(ctx)
	if err != nil {
		return 0, err
	}
	return config.ConvertLamportsToSOL(balance), nil
}

// GetAssociatedTokenAddress returns the ATA address for given mint (no RPC call)
func (w *Wallet) GetAssociatedTokenAddress(mint common.PublicKey) (common.PublicKey, error) {
	ata, _, err := common.FindAssociatedTokenAddress(w.account.PublicKey, mint)
	if err != nil {
		return common.PublicKey{}, fmt.Errorf("failed to find ATA address: %w", err)
	}
	return ata, nil
}

// CreateTransaction creates a new transaction with recent blockhash
func (w *Wallet) CreateTransaction(ctx context.Context, instructions []types.Instruction) (types.Transaction, error) {
	// Get recent blockhash
	blockhash, err := w.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return types.Transaction{}, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Create transaction
	transaction, err := types.NewTransaction(types.NewTransactionParam{
		Signers: []types.Account{w.account},
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        w.account.PublicKey,
			RecentBlockhash: blockhash,
			Instructions:    instructions,
		}),
	})
	if err != nil {
		return types.Transaction{}, fmt.Errorf("failed to create transaction: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"blockhash":    blockhash,
		"instructions": len(instructions),
	}).Debug("Created transaction")

	return transaction, nil
}

// SendTransaction sends a transaction to the network
func (w *Wallet) SendTransaction(ctx context.Context, transaction types.Transaction) (string, error) {
	// Serialize transaction to base64
	txBytes, err := transaction.Serialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	// Encode as base64 for RPC
	encodedTx := base64.StdEncoding.EncodeToString(txBytes)

	// Send transaction
	signature, err := w.rpcClient.SendTransaction(ctx, encodedTx)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	w.logger.WithField("signature", signature).Info("Transaction sent")
	return signature, nil
}

// SendAndConfirmTransaction sends a transaction and waits for confirmation
func (w *Wallet) SendAndConfirmTransaction(ctx context.Context, transaction types.Transaction) (string, error) {
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
