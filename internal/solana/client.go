package solana

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mr-tron/base58"
	"github.com/sirupsen/logrus"
)

// Client represents a Solana RPC client
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	logger     *logrus.Logger
}

// ClientConfig contains configuration for Solana client
type ClientConfig struct {
	Endpoint string
	APIKey   string
	Timeout  time.Duration
}

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface
func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// AccountInfo represents Solana account information
type AccountInfo struct {
	Data       []string `json:"data"`
	Executable bool     `json:"executable"`
	Lamports   uint64   `json:"lamports"`
	Owner      string   `json:"owner"`
	//RentEpoch  uint64   `json:"rentEpoch"`
}

func (ai *AccountInfo) GetOwnerKey() string {
	base64Data := ai.Data[0]
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		panic(err)
	}

	owner := decoded[32:64]
	ownerBase58 := base58.Encode(owner)
	fmt.Println("Owner pubkey (base58):", ownerBase58)

	return ownerBase58
}

// AccountInfoResponse represents the response for getAccountInfo
type AccountInfoResponse struct {
	Context struct {
		Slot uint64 `json:"slot"`
	} `json:"context"`
	Value *AccountInfo `json:"value"`
}

// TransactionResponse represents transaction response
type TransactionResponse struct {
	Slot        uint64                 `json:"slot"`
	Transaction map[string]interface{} `json:"transaction"`
	Meta        *TransactionMeta       `json:"meta"`
	BlockTime   *int64                 `json:"blockTime"`
	Version     interface{}            `json:"version"`
}

// TransactionMeta contains transaction metadata
type TransactionMeta struct {
	Err               interface{}   `json:"err"`
	Fee               uint64        `json:"fee"`
	PreBalances       []uint64      `json:"preBalances"`
	PostBalances      []uint64      `json:"postBalances"`
	InnerInstructions []interface{} `json:"innerInstructions"`
	LogMessages       []string      `json:"logMessages"`
	PreTokenBalances  []interface{} `json:"preTokenBalances"`
	PostTokenBalances []interface{} `json:"postTokenBalances"`
	Rewards           []interface{} `json:"rewards"`
}

// BlockResponse represents block response
type BlockResponse struct {
	BlockHeight       *uint64               `json:"blockHeight"`
	BlockTime         *int64                `json:"blockTime"`
	Blockhash         string                `json:"blockhash"`
	ParentSlot        uint64                `json:"parentSlot"`
	PreviousBlockhash string                `json:"previousBlockhash"`
	Transactions      []TransactionResponse `json:"transactions"`
}

// NewClient creates a new Solana RPC client
func NewClient(config ClientConfig, logger *logrus.Logger) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		endpoint: config.Endpoint,
		apiKey:   config.APIKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
	}
}

// makeRequest makes a JSON-RPC request to Solana
func (c *Client) makeRequest(ctx context.Context, method string, params interface{}) (*RPCResponse, error) {
	request := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	c.logger.WithFields(logrus.Fields{
		"method":   method,
		"endpoint": c.endpoint,
	}).Debug("Making RPC request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(responseBody))
	}

	var rpcResponse RPCResponse
	if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResponse.Error != nil {
		return nil, rpcResponse.Error
	}

	return &rpcResponse, nil
}

// GetAccountInfo gets account information
func (c *Client) GetAccountInfo(ctx context.Context, address string) (*AccountInfo, error) {
	params := []interface{}{
		address,
		map[string]interface{}{
			"encoding": "base64",
		},
	}

	resp, err := c.makeRequest(ctx, "getAccountInfo", params)
	if err != nil {
		return nil, fmt.Errorf("getAccountInfo failed: %w", err)
	}

	var accountResponse AccountInfoResponse
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &accountResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal balance: %w", err)
	}

	if accountResponse.Value == nil {
		return nil, fmt.Errorf("Empty accountResponse")
	}

	return accountResponse.Value, nil
}

// GetTransaction gets transaction information
func (c *Client) GetTransaction(ctx context.Context, signature string) (*TransactionResponse, error) {
	params := []interface{}{
		signature,
		map[string]interface{}{
			"encoding":                       "json",
			"maxSupportedTransactionVersion": 0,
		},
	}

	resp, err := c.makeRequest(ctx, "getTransaction", params)
	if err != nil {
		return nil, fmt.Errorf("getTransaction failed: %w", err)
	}

	var transaction TransactionResponse
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &transaction); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}

	return &transaction, nil
}

// GetBlock gets block information
func (c *Client) GetBlock(ctx context.Context, slot uint64) (*BlockResponse, error) {
	params := []interface{}{
		slot,
		map[string]interface{}{
			"encoding":                       "json",
			"transactionDetails":             "full",
			"maxSupportedTransactionVersion": 0,
		},
	}

	resp, err := c.makeRequest(ctx, "getBlock", params)
	if err != nil {
		return nil, fmt.Errorf("getBlock failed: %w", err)
	}

	var block BlockResponse
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &block, nil
}

// GetLatestBlockhash gets the latest blockhash
func (c *Client) GetLatestBlockhash(ctx context.Context) (string, error) {
	params := []interface{}{}
	resp, err := c.makeRequest(ctx, "getLatestBlockhash", params)
	if err != nil {
		return "", fmt.Errorf("getLatestBlockhash failed: %w", err)
	}

	type blockhashResponse struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value struct {
			Blockhash     string `json:"blockhash"`
			LastValidSlot uint64 `json:"lastValidBlockHeight"`
		} `json:"value"`
	}

	var bhResp blockhashResponse
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &bhResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal blockhash: %w", err)
	}

	return bhResp.Value.Blockhash, nil
}

// SendTransaction sends a transaction to the network
func (c *Client) SendTransaction(ctx context.Context, transaction string) (string, error) {
	params := []interface{}{
		transaction,
		map[string]interface{}{
			"encoding": "base64",
		},
	}

	resp, err := c.makeRequest(ctx, "sendTransaction", params)
	if err != nil {
		return "", fmt.Errorf("sendTransaction failed: %w", err)
	}

	signature, ok := resp.Result.(string)
	if !ok {
		return "", fmt.Errorf("invalid response format for sendTransaction")
	}

	return signature, nil
}

// ConfirmTransaction confirms a transaction
func (c *Client) ConfirmTransaction(ctx context.Context, signature string) error {
	params := []interface{}{
		signature,
		map[string]interface{}{
			"commitment": "confirmed",
		},
	}

	resp, err := c.makeRequest(ctx, "getSignatureStatus", params)
	if err != nil {
		return fmt.Errorf("getSignatureStatus failed: %w", err)
	}

	type signatureStatus struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value []struct {
			Slot               uint64      `json:"slot"`
			Confirmations      *int        `json:"confirmations"`
			Err                interface{} `json:"err"`
			ConfirmationStatus string      `json:"confirmationStatus"`
		} `json:"value"`
	}

	var status signatureStatus
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &status); err != nil {
		return fmt.Errorf("failed to unmarshal signature status: %w", err)
	}

	if len(status.Value) == 0 || status.Value[0].Err != nil {
		return fmt.Errorf("transaction failed or not found")
	}

	return nil
}

// GetBalance gets account balance in lamports
func (c *Client) GetBalance(ctx context.Context, address string) (uint64, error) {
	params := []interface{}{address}

	resp, err := c.makeRequest(ctx, "getBalance", params)
	if err != nil {
		return 0, fmt.Errorf("getBalance failed: %w", err)
	}

	type balanceResponse struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value uint64 `json:"value"`
	}

	var balResp balanceResponse
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &balResp); err != nil {
		return 0, fmt.Errorf("failed to unmarshal balance: %w", err)
	}

	return balResp.Value, nil
}

// GetSlot gets current slot
func (c *Client) GetSlot(ctx context.Context) (uint64, error) {
	resp, err := c.makeRequest(ctx, "getSlot", nil)
	if err != nil {
		return 0, fmt.Errorf("getSlot failed: %w", err)
	}

	slot, ok := resp.Result.(float64)
	if !ok {
		return 0, fmt.Errorf("invalid response format for getSlot")
	}

	return uint64(slot), nil
}
