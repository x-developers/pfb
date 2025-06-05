package wallet

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc"
	"pump-fun-bot-go/internal/client"
	"time"

	"pump-fun-bot-go/internal/config"

	"github.com/gagliardetto/solana-go"
	"github.com/sirupsen/logrus"
)

// Wallet represents a Solana wallet
type Wallet struct {
	account   solana.PrivateKey
	publicKey solana.PublicKey
	rpcClient *client.Client
	logger    *logrus.Logger
	config    *config.Config
}

// WalletConfig contains wallet configuration
type WalletConfig struct {
	PrivateKey string
	Network    string
}

// NewWallet creates a new wallet instance from private key
func NewWallet(cfg WalletConfig, rpcClient *client.Client, logger *logrus.Logger, config *config.Config) (*Wallet, error) {
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("private key is required")
	}

	// Parse private key from base58
	privateKey, err := solana.PrivateKeyFromBase58(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	publicKey := privateKey.PublicKey()

	wallet := &Wallet{
		account:   privateKey,
		publicKey: publicKey,
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
func (w *Wallet) GetPublicKey() solana.PublicKey {
	return w.publicKey
}

// GetPublicKeyString returns the wallet's public key as base58 string
func (w *Wallet) GetPublicKeyString() string {
	return w.publicKey.String()
}

// GetAccount returns the wallet's private key for signing transactions
func (w *Wallet) GetAccount() solana.PrivateKey {
	return w.account
}

// GetBalance returns the wallet's SOL balance in lamports
func (w *Wallet) GetBalance(ctx context.Context) (uint64, error) {
	balance, err := w.rpcClient.GetBalance(ctx, w.GetPublicKeyString(), "")
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"balance_lamports": balance.Value,
		"balance_sol":      config.ConvertLamportsToSOL(balance.Value),
	}).Debug("Retrieved wallet balance")

	return balance.Value, nil
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
func (w *Wallet) GetAssociatedTokenAddress(mint solana.PublicKey) (solana.PublicKey, error) {
	ata, _, err := solana.FindAssociatedTokenAddress(w.publicKey, mint)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to find ATA address: %w", err)
	}
	return ata, nil
}

// CreateTransaction creates a new transaction with recent blockhash
func (w *Wallet) CreateTransaction(ctx context.Context, instructions []solana.Instruction) (*solana.Transaction, error) {
	// Get recent blockhash
	recent, err := w.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Create transaction
	tx, err := solana.NewTransaction(
		instructions,
		recent,
		solana.TransactionPayer(w.publicKey),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	_, err = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(w.publicKey) {
				return &w.account
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"blockhash":    recent.String(),
		"instructions": len(instructions),
	}).Debug("Created transaction")

	return tx, nil
}

// SendTransaction sends a transaction to the network
func (w *Wallet) SendTransaction(ctx context.Context, transaction *solana.Transaction) (string, error) {
	// Send transaction
	sig, err := w.rpcClient.SendTransaction(ctx, transaction)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	signature := sig.String()
	w.logger.WithField("signature", signature).Info("Transaction sent")
	return signature, nil
}

// SendAndConfirmTransaction sends a transaction and waits for confirmation
func (w *Wallet) SendAndConfirmTransaction(ctx context.Context, transaction *solana.Transaction) (string, error) {
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

	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			result, err := w.rpcClient.GetSignatureStatus(ctx, sig)
			if err != nil {
				w.logger.WithField("signature", signature).Debug("Error checking signature status, retrying...")
				continue
			}

			if result != nil {
				if result.ConfirmationStatus == rpc.ConfirmationStatusConfirmed || result.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
					return nil // Transaction confirmed
				}
				if result.Err != nil {
					return fmt.Errorf("transaction failed: %v", result.Err)
				}
			}

			// Continue waiting if transaction not confirmed yet
			w.logger.WithField("signature", signature).Debug("Waiting for confirmation...")
		}
	}
}
