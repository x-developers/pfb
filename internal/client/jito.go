package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// JitoClient represents a JITO RPC client for MEV-protected transactions
type JitoClient struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	logger     *logrus.Logger
}

// JitoClientConfig contains configuration for JITO client
type JitoClientConfig struct {
	Endpoint string
	APIKey   string
	Timeout  time.Duration
}

// JitoBundleRequest represents a JITO bundle submission request
type JitoBundleRequest struct {
	JSONRPC string                    `json:"jsonrpc"`
	ID      int                       `json:"id"`
	Method  string                    `json:"method"`
	Params  []JitoBundleRequestParams `json:"params"`
}

// JitoBundleRequestParams represents parameters for JITO bundle request
type JitoBundleRequestParams struct {
	EncodedTransactions []string `json:"encodedTransactions"`
}

// JitoBundleResponse represents JITO bundle submission response
type JitoBundleResponse struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      int               `json:"id"`
	Result  string            `json:"result,omitempty"` // Bundle UUID
	Error   *jsonrpc.RPCError `json:"error,omitempty"`
}

// JitoTipRequest represents a tip transaction request
type JitoTipRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Method  string                 `json:"method"`
	Params  []JitoTipRequestParams `json:"params"`
}

// JitoTipRequestParams represents parameters for tip request
type JitoTipRequestParams struct {
	Accounts        []string          `json:"accounts"`
	Instructions    []JitoInstruction `json:"instructions"`
	RecentBlockhash string            `json:"recentBlockhash"`
}

// JitoInstruction represents a JITO instruction
type JitoInstruction struct {
	ProgramID string            `json:"programId"`
	Accounts  []JitoAccountMeta `json:"accounts"`
	Data      string            `json:"data"`
}

// JitoAccountMeta represents account metadata for JITO
type JitoAccountMeta struct {
	Pubkey     string `json:"pubkey"`
	IsSigner   bool   `json:"isSigner"`
	IsWritable bool   `json:"isWritable"`
}

// JitoTipAccountsResponse represents tip accounts response
type JitoTipAccountsResponse struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      int               `json:"id"`
	Result  []string          `json:"result,omitempty"`
	Error   *jsonrpc.RPCError `json:"error,omitempty"`
}

// JitoBundleStatusRequest represents bundle status request
type JitoBundleStatusRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      int      `json:"id"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
}

// JitoBundleStatusResponse represents bundle status response
type JitoBundleStatusResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Result  JitoBundleStatusResult `json:"result,omitempty"`
	Error   *jsonrpc.RPCError      `json:"error,omitempty"`
}

// JitoBundleStatusResult represents bundle status result
type JitoBundleStatusResult struct {
	Context JitoContext      `json:"context"`
	Value   JitoBundleStatus `json:"value"`
}

// JitoContext represents JITO context
type JitoContext struct {
	Slot uint64 `json:"slot"`
}

// JitoBundleStatus represents bundle status
type JitoBundleStatus struct {
	BundleID           string                  `json:"bundle_id"`
	Transactions       []JitoTransactionStatus `json:"transactions"`
	Slot               uint64                  `json:"slot"`
	ConfirmationStatus string                  `json:"confirmation_status"`
	Err                interface{}             `json:"err"`
}

// JitoTransactionStatus represents transaction status within bundle
type JitoTransactionStatus struct {
	Signature string      `json:"signature"`
	Err       interface{} `json:"err"`
}

// NewJitoClient creates a new JITO RPC client
func NewJitoClient(config JitoClientConfig, logger *logrus.Logger) *JitoClient {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &JitoClient{
		endpoint: config.Endpoint,
		apiKey:   config.APIKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: logger,
	}
}

// makeJitoRequest makes a JSON-RPC request to JITO
func (jc *JitoClient) makeJitoRequest(ctx context.Context, method string, params interface{}) (*json.RawMessage, error) {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", jc.endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if jc.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+jc.apiKey)
	}

	jc.logger.WithFields(logrus.Fields{
		"method":   method,
		"endpoint": jc.endpoint,
	}).Debug("Making JITO RPC request")

	resp, err := jc.httpClient.Do(req)
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

	var rpcResponse struct {
		JSONRPC string            `json:"jsonrpc"`
		ID      int               `json:"id"`
		Result  *json.RawMessage  `json:"result,omitempty"`
		Error   *jsonrpc.RPCError `json:"error,omitempty"`
	}

	if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResponse.Error != nil {
		return nil, rpcResponse.Error
	}

	return rpcResponse.Result, nil
}

// GetTipAccounts gets JITO tip accounts
func (jc *JitoClient) GetTipAccounts(ctx context.Context) ([]string, error) {
	result, err := jc.makeJitoRequest(ctx, "getTipAccounts", []interface{}{})
	if err != nil {
		return nil, fmt.Errorf("getTipAccounts failed: %w", err)
	}

	var tipAccounts []string
	if err := json.Unmarshal(*result, &tipAccounts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tip accounts: %w", err)
	}

	jc.logger.WithField("tip_accounts_count", len(tipAccounts)).Debug("Retrieved JITO tip accounts")
	return tipAccounts, nil
}

// SendBundle sends a bundle of transactions to JITO
func (jc *JitoClient) SendBundle(ctx context.Context, encodedTransactions []string) (string, error) {
	params := []interface{}{encodedTransactions}

	result, err := jc.makeJitoRequest(ctx, "sendBundle", params)
	if err != nil {
		return "", fmt.Errorf("sendBundle failed: %w", err)
	}

	var bundleID string
	if err := json.Unmarshal(*result, &bundleID); err != nil {
		return "", fmt.Errorf("failed to unmarshal bundle ID: %w", err)
	}

	jc.logger.WithFields(logrus.Fields{
		"bundle_id":    bundleID,
		"transactions": len(encodedTransactions),
	}).Info("JITO bundle sent successfully")

	return bundleID, nil
}

// SendTransaction sends a single transaction via JITO bundle
func (jc *JitoClient) SendTransaction(ctx context.Context, encodedTransaction string) (string, error) {
	return jc.SendBundle(ctx, []string{encodedTransaction})
}

// SendTransactionWithTip sends a transaction with tip via JITO bundle
func (jc *JitoClient) SendTransactionWithTip(ctx context.Context, encodedTransaction string, tipTransaction string) (string, error) {
	transactions := []string{encodedTransaction}
	if tipTransaction != "" {
		transactions = append(transactions, tipTransaction)
	}

	return jc.SendBundle(ctx, transactions)
}

// GetBundleStatus gets the status of a JITO bundle
func (jc *JitoClient) GetBundleStatus(ctx context.Context, bundleID string) (*JitoBundleStatus, error) {
	params := []interface{}{bundleID}

	result, err := jc.makeJitoRequest(ctx, "getBundleStatuses", params)
	if err != nil {
		return nil, fmt.Errorf("getBundleStatuses failed: %w", err)
	}

	var bundleStatuses struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value []JitoBundleStatus `json:"value"`
	}

	if err := json.Unmarshal(*result, &bundleStatuses); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bundle status: %w", err)
	}

	if len(bundleStatuses.Value) == 0 {
		return nil, fmt.Errorf("bundle status not found for ID: %s", bundleID)
	}

	status := &bundleStatuses.Value[0]
	jc.logger.WithFields(logrus.Fields{
		"bundle_id":           status.BundleID,
		"confirmation_status": status.ConfirmationStatus,
		"slot":                status.Slot,
		"transactions_count":  len(status.Transactions),
	}).Debug("Retrieved JITO bundle status")

	return status, nil
}

// ConfirmBundle waits for bundle confirmation
func (jc *JitoClient) ConfirmBundle(ctx context.Context, bundleID string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := jc.GetBundleStatus(ctx, bundleID)
			if err != nil {
				jc.logger.WithError(err).Debug("Failed to get bundle status")
				continue
			}

			if status.Err != nil {
				return fmt.Errorf("bundle failed: %v", status.Err)
			}

			if status.ConfirmationStatus == "confirmed" || status.ConfirmationStatus == "finalized" {
				jc.logger.WithFields(logrus.Fields{
					"bundle_id": bundleID,
					"status":    status.ConfirmationStatus,
					"slot":      status.Slot,
				}).Info("JITO bundle confirmed")
				return nil
			}

			jc.logger.WithFields(logrus.Fields{
				"bundle_id": bundleID,
				"status":    status.ConfirmationStatus,
			}).Debug("Waiting for bundle confirmation...")
		}
	}
}

// SendAndConfirmBundle sends a bundle and waits for confirmation
func (jc *JitoClient) SendAndConfirmBundle(ctx context.Context, encodedTransactions []string) (string, error) {
	bundleID, err := jc.SendBundle(ctx, encodedTransactions)
	if err != nil {
		return "", err
	}

	// Wait for confirmation with timeout
	confirmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err = jc.ConfirmBundle(confirmCtx, bundleID)
	if err != nil {
		return bundleID, fmt.Errorf("bundle sent but confirmation failed: %w", err)
	}

	return bundleID, nil
}

// SendAndConfirmTransaction sends a single transaction and waits for confirmation
func (jc *JitoClient) SendAndConfirmTransaction(ctx context.Context, encodedTransaction string) (string, error) {
	return jc.SendAndConfirmBundle(ctx, []string{encodedTransaction})
}

// CreateTipTransaction creates a tip transaction for MEV protection
func (jc *JitoClient) CreateTipTransaction(ctx context.Context, tipAccount string, tipAmount uint64, recentBlockhash string, feePayer string) (string, error) {
	// This is a simplified version - in a real implementation, you'd need to
	// construct the actual tip transaction with proper instructions

	// For now, return empty string indicating no tip transaction
	// This should be implemented based on JITO's tip transaction format
	jc.logger.WithFields(logrus.Fields{
		"tip_account": tipAccount,
		"tip_amount":  tipAmount,
		"fee_payer":   feePayer,
	}).Debug("Creating JITO tip transaction")

	// TODO: Implement actual tip transaction creation
	return "", nil
}

// GetRandomTipAccount returns a random tip account from available accounts
func (jc *JitoClient) GetRandomTipAccount(ctx context.Context) (string, error) {
	tipAccounts, err := jc.GetTipAccounts(ctx)
	if err != nil {
		return "", err
	}

	if len(tipAccounts) == 0 {
		return "", fmt.Errorf("no tip accounts available")
	}

	// Return first account for now - in production you might want to randomize
	return tipAccounts[0], nil
}

// HealthCheck checks if JITO service is healthy
func (jc *JitoClient) HealthCheck(ctx context.Context) error {
	_, err := jc.GetTipAccounts(ctx)
	if err != nil {
		return fmt.Errorf("JITO health check failed: %w", err)
	}

	jc.logger.Debug("JITO health check passed")
	return nil
}

// GetInflightBundleStatuses gets statuses of inflight bundles
func (jc *JitoClient) GetInflightBundleStatuses(ctx context.Context, bundleIDs []string) ([]JitoBundleStatus, error) {
	params := []interface{}{bundleIDs}

	result, err := jc.makeJitoRequest(ctx, "getInflightBundleStatuses", params)
	if err != nil {
		return nil, fmt.Errorf("getInflightBundleStatuses failed: %w", err)
	}

	var response struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value []JitoBundleStatus `json:"value"`
	}

	if err := json.Unmarshal(*result, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inflight bundle statuses: %w", err)
	}

	jc.logger.WithFields(logrus.Fields{
		"requested_bundles": len(bundleIDs),
		"returned_statuses": len(response.Value),
		"slot":              response.Context.Slot,
	}).Debug("Retrieved inflight bundle statuses")

	return response.Value, nil
}

// EstimateBundleCost estimates the cost of sending a bundle (for planning purposes)
func (jc *JitoClient) EstimateBundleCost(transactionCount int, tipAmount uint64) uint64 {
	// Base cost per transaction (estimated)
	baseCostPerTx := uint64(5000) // 5000 lamports base cost

	// Total base cost
	totalBaseCost := baseCostPerTx * uint64(transactionCount)

	// Add tip amount
	totalCost := totalBaseCost + tipAmount

	jc.logger.WithFields(logrus.Fields{
		"transaction_count": transactionCount,
		"base_cost":         totalBaseCost,
		"tip_amount":        tipAmount,
		"total_cost":        totalCost,
	}).Debug("Estimated JITO bundle cost")

	return totalCost
}
