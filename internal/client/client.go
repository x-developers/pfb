package client

import (
	"context"
	"fmt"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	confirm "github.com/gagliardetto/solana-go/rpc/sendAndConfirmTransaction"
	"github.com/gagliardetto/solana-go/rpc/ws"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/sirupsen/logrus"
)

// Client represents a Solana RPC client wrapper
type Client struct {
	client   *rpc.Client
	wsClient *ws.Client
	logger   *logrus.Logger
}

// ClientConfig contains configuration for Solana client
type ClientConfig struct {
	RPCEndpoint string
	WSEndpoint  string
	APIKey      string
	Timeout     time.Duration
}

// NewClient creates a new Solana RPC client
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

		//rpcClient = rpc.New(config.RPCEndpoint)
	}

	wsClient, _ := ws.Connect(context.Background(), config.WSEndpoint)

	return &Client{
		client:   rpcClient,
		wsClient: wsClient,
		logger:   logger,
	}
}

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

func (c *Client) GetLatestBlockhash(ctx context.Context) (solana.Hash, error) {
	result, err := c.client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return solana.Hash{}, fmt.Errorf("getLatestBlockhash failed: %w", err)
	}

	return result.Value.Blockhash, nil
}

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

func (c *Client) CreateATA(pub solana.PublicKey, priv solana.PrivateKey, mintAddress solana.PublicKey) (*solana.PublicKey, error) {
	// Находим адрес ATA
	ataAddress, _, err := solana.FindAssociatedTokenAddress(
		pub,
		mintAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find ATA address: %w", err)
	}

	// Проверяем, существует ли уже ATA
	//_, err = aw.client.GetAccountInfo(context.TODO(), ataAddress)
	//if err == nil {
	//	// ATA уже существует
	//	fmt.Printf("ATA already exists: %s\n", ataAddress.String())
	//	aw.ataAccounts[mintAddress.String()] = ataAddress
	//	return &ataAddress, nil
	//}

	// Создаем инструкцию для создания ATA
	instruction := associatedtokenaccount.NewCreateInstruction(
		pub,         // payer
		pub,         // wallet
		mintAddress, // mint
	).Build()

	// Получаем последний blockhash
	recent, err := c.client.GetLatestBlockhash(context.TODO(), rpc.CommitmentFinalized)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recent.Value.Blockhash,
		solana.TransactionPayer(pub),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// Подписываем транзакцию
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

	// Отправляем транзакцию
	sig, err := confirm.SendAndConfirmTransaction(
		context.TODO(),
		c.client,
		c.wsClient,
		tx,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	fmt.Printf("ATA created successfully! Transaction: %s\n", sig.String())
	fmt.Printf("ATA Address: %s\n", ataAddress.String())

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
