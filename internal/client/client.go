package client

import (
	"context"
	"fmt"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	confirm "github.com/gagliardetto/solana-go/rpc/sendAndConfirmTransaction"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/sirupsen/logrus"
)

// Client represents a Solana RPC client wrapper with blockhash caching
type Client struct {
	client   *rpc.Client
	wsClient *ws.Client
	logger   *logrus.Logger

	// Blockhash caching
	cachedBlockhash    solana.Hash
	blockhashTimestamp time.Time
	blockhashMutex     sync.RWMutex
	blockhashTTL       time.Duration
}

// ClientConfig contains configuration for Solana client
type ClientConfig struct {
	RPCEndpoint string
	WSEndpoint  string
	APIKey      string
	Timeout     time.Duration
}

// NewClient creates a new Solana RPC client with blockhash caching
func NewClient(config ClientConfig, logger *logrus.Logger) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	var rpcClient *rpc.Client
	if config.APIKey != "" {
		// Create client with API key authentication
		rpcClient = rpc.NewWithHeaders(config.RPCEndpoint, map[string]string{
			"Authorization": "Bearer " + config.APIKey,
		})
	} else {
		rpcClient = rpc.NewWithHeaders(config.RPCEndpoint, map[string]string{
			"User-Agent":   "python-httpx/0.28.1",
			"Content-Type": "application/json",
			"Connection":   "keep-alive",
		})
	}

	wsClient, _ := ws.Connect(context.Background(), config.WSEndpoint)

	client := &Client{
		client:       rpcClient,
		wsClient:     wsClient,
		logger:       logger,
		blockhashTTL: 30 * time.Second, // Blockhash is valid for ~60-90 seconds, cache for 30
	}

	// Start blockhash updater in background
	go client.blockhashUpdater()

	return client
}

// GetLatestBlockhash returns cached blockhash or fetches new one
func (c *Client) GetLatestBlockhash(ctx context.Context) (solana.Hash, error) {
	// Try to use cached blockhash first
	if cachedHash := c.getCachedBlockhash(); !cachedHash.IsZero() {
		return cachedHash, nil
	}

	// Fetch new blockhash
	result, err := c.client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solana.Hash{}, fmt.Errorf("getLatestBlockhash failed: %w", err)
	}

	// Cache the new blockhash
	c.cacheBlockhash(result.Value.Blockhash)

	return result.Value.Blockhash, nil
}

// getCachedBlockhash returns cached blockhash if it's still valid
func (c *Client) getCachedBlockhash() solana.Hash {
	c.blockhashMutex.RLock()
	defer c.blockhashMutex.RUnlock()

	// Check if cache is still valid
	if time.Since(c.blockhashTimestamp) < c.blockhashTTL && !c.cachedBlockhash.IsZero() {
		return c.cachedBlockhash
	}

	return solana.Hash{}
}

// cacheBlockhash stores blockhash in cache
func (c *Client) cacheBlockhash(blockhash solana.Hash) {
	c.blockhashMutex.Lock()
	defer c.blockhashMutex.Unlock()

	c.cachedBlockhash = blockhash
	c.blockhashTimestamp = time.Now()

	c.logger.Debug("🔗 Blockhash cached")
}

// blockhashUpdater periodically updates the cached blockhash
func (c *Client) blockhashUpdater() {
	ticker := time.NewTicker(15 * time.Second) // Update every 15 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

			result, err := c.client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
			if err == nil {
				c.cacheBlockhash(result.Value.Blockhash)
				c.logger.Debug("🔄 Blockhash updated in background")
			} else {
				c.logger.WithError(err).Debug("Failed to update blockhash in background")
			}

			cancel()
		}
	}
}

// GetBlockhashCacheInfo returns information about blockhash cache
func (c *Client) GetBlockhashCacheInfo() map[string]interface{} {
	c.blockhashMutex.RLock()
	defer c.blockhashMutex.RUnlock()

	return map[string]interface{}{
		"cached":      !c.cachedBlockhash.IsZero(),
		"cached_at":   c.blockhashTimestamp,
		"age_seconds": time.Since(c.blockhashTimestamp).Seconds(),
		"ttl_seconds": c.blockhashTTL.Seconds(),
		"is_valid":    time.Since(c.blockhashTimestamp) < c.blockhashTTL,
		"blockhash":   c.cachedBlockhash.String(),
	}
}

// Rest of the client methods remain the same...

// GetAccountInfo gets account information
func (c *Client) GetAccountInfo(ctx context.Context, address string) (*rpc.GetAccountInfoResult, error) {
	pubkey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	result, err := c.client.GetAccountInfo(ctx, pubkey)
	if err != nil {
		return nil, fmt.Errorf("getAccountInfo failed: %w", err)
	}

	return result, nil
}

// GetTokenBalance gets token account balance
func (c *Client) GetTokenBalance(ctx context.Context, address string) (uint64, error) {
	pubkey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return 0, fmt.Errorf("invalid address: %w", err)
	}

	result, err := c.client.GetTokenAccountBalance(ctx, pubkey, rpc.CommitmentFinalized)
	if err != nil {
		return 0, fmt.Errorf("failed to get token balance: %w", err)
	}

	if result == nil || result.Value == nil {
		return 0, fmt.Errorf("token account not found")
	}

	return uint64(*result.Value.UiAmount), nil
}

// GetTransaction gets transaction information
func (c *Client) GetTransaction(ctx context.Context, signature string) (*rpc.GetTransactionResult, error) {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	result, err := c.client.GetTransaction(
		ctx,
		sig,
		&rpc.GetTransactionOpts{
			Encoding:                       solana.EncodingJSON,
			MaxSupportedTransactionVersion: &[]uint64{0}[0],
		},
	)
	if err != nil {
		return nil, fmt.Errorf("getTransaction failed: %w", err)
	}

	return result, nil
}

// GetBlock gets block information
func (c *Client) GetBlock(ctx context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	result, err := c.client.GetBlock(ctx, slot)
	if err != nil {
		return nil, fmt.Errorf("getBlock failed: %w", err)
	}

	return result, nil
}

// SendAndConfirmTransaction sends a transaction and confirms it
func (c *Client) SendAndConfirmTransaction(ctx context.Context, transaction *solana.Transaction) (solana.Signature, error) {
	sig, err := confirm.SendAndConfirmTransaction(
		ctx,
		c.client,
		c.wsClient,
		transaction,
	)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("failed to send transaction: %w", err)
	}

	return sig, nil
}

// CreateATA creates an Associated Token Account
func (c *Client) CreateATA(pub solana.PublicKey, priv solana.PrivateKey, mintAddress solana.PublicKey) (*solana.PublicKey, error) {
	// Find ATA address
	ataAddress, _, err := solana.FindAssociatedTokenAddress(
		pub,
		mintAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find ATA address: %w", err)
	}

	// Check if ATA already exists
	_, err = c.client.GetAccountInfo(context.TODO(), ataAddress)
	if err == nil {
		// ATA already exists
		c.logger.WithField("ata_address", ataAddress.String()).Debug("ATA already exists")
		return &ataAddress, nil
	}

	// Create instruction for ATA creation
	instruction := associatedtokenaccount.NewCreateInstruction(
		pub,         // payer
		pub,         // wallet
		mintAddress, // mint
	).Build()

	// Get latest blockhash (will use cache if available)
	recent, err := c.GetLatestBlockhash(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Create transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recent,
		solana.TransactionPayer(pub),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Sign transaction
	_, err = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if pub.Equals(key) {
				acc := priv
				return &acc
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	sig, err := confirm.SendAndConfirmTransaction(
		context.TODO(),
		c.client,
		c.wsClient,
		tx,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	c.logger.WithFields(map[string]interface{}{
		"signature":   sig.String(),
		"ata_address": ataAddress.String(),
	}).Info("✅ ATA created successfully")

	return &ataAddress, nil
}

// SendTransaction sends a transaction to the network
func (c *Client) SendTransaction(ctx context.Context, transaction *solana.Transaction) (solana.Signature, error) {
	sig, err := c.client.SendTransaction(ctx, transaction)
	if err != nil {
		return solana.Signature{}, fmt.Errorf("sendTransaction failed: %w", err)
	}

	return sig, nil
}

// ConfirmTransaction confirms a transaction
func (c *Client) ConfirmTransaction(ctx context.Context, signature string) error {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	result, err := c.GetSignatureStatus(ctx, sig)
	if err != nil {
		return fmt.Errorf("getSignatureStatus failed: %w", err)
	}

	if result == nil {
		return fmt.Errorf("transaction not found")
	}

	if result.Err != nil {
		return fmt.Errorf("transaction failed: %v", result.Err)
	}

	if result.ConfirmationStatus != rpc.ConfirmationStatusConfirmed && result.ConfirmationStatus != rpc.ConfirmationStatusFinalized {
		return fmt.Errorf("transaction not confirmed, status: %s", result.ConfirmationStatus)
	}

	return nil
}

// GetBalance gets account balance in lamports
func (c *Client) GetBalance(ctx context.Context, address string, commitment rpc.CommitmentType) (*rpc.GetBalanceResult, error) {
	pubkey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	result, err := c.client.GetBalance(ctx, pubkey, commitment)
	if err != nil {
		return nil, fmt.Errorf("getBalance failed: %w", err)
	}

	return result, nil
}

// GetSlot gets current slot
func (c *Client) GetSlot(ctx context.Context) (uint64, error) {
	result, err := c.client.GetSlot(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("getSlot failed: %w", err)
	}

	return result, nil
}

// GetSignatureStatuses gets signature statuses
func (c *Client) GetSignatureStatuses(ctx context.Context, signatures []solana.Signature) (*rpc.GetSignatureStatusesResult, error) {
	result, err := c.client.GetSignatureStatuses(ctx, true, signatures...)
	if err != nil {
		return nil, fmt.Errorf("getSignatureStatuses failed: %w", err)
	}

	return result, nil
}

// GetSignatureStatus gets single signature status (convenience method)
func (c *Client) GetSignatureStatus(ctx context.Context, signature solana.Signature) (*rpc.SignatureStatusesResult, error) {
	result, err := c.GetSignatureStatuses(ctx, []solana.Signature{signature})
	if err != nil {
		return nil, err
	}

	if result == nil || len(result.Value) == 0 {
		return nil, fmt.Errorf("signature not found")
	}

	return result.Value[0], nil
}
