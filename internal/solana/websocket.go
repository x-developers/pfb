package solana

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// WSClient represents a WebSocket client for Solana
type WSClient struct {
	url            string
	conn           *websocket.Conn
	logger         *logrus.Logger
	mu             sync.RWMutex
	subscriptions  map[int]*Subscription
	nextID         int
	handlers       map[string]EventHandler
	ctx            context.Context
	cancel         context.CancelFunc
	reconnectDelay time.Duration
}

// Subscription represents a WebSocket subscription
type Subscription struct {
	ID      int
	Method  string
	Params  interface{}
	Handler EventHandler
	Active  bool
}

// EventHandler handles WebSocket events
type EventHandler func(data interface{}) error

// WSMessage represents a WebSocket message
type WSMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// BlockNotification represents a block notification
type BlockNotification struct {
	Subscription int `json:"subscription"`
	Result       struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value struct {
			Slot  uint64                 `json:"slot"`
			Block map[string]interface{} `json:"block"`
			Err   interface{}            `json:"err"`
		} `json:"value"`
	} `json:"result"`
}

// LogsNotification represents a logs notification
type LogsNotification struct {
	Subscription int `json:"subscription"`
	Result       struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value struct {
			Signature string      `json:"signature"`
			Err       interface{} `json:"err"`
			Logs      []string    `json:"logs"`
		} `json:"value"`
	} `json:"result"`
}

// NewWSClient creates a new WebSocket client
func NewWSClient(url string, logger *logrus.Logger) *WSClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &WSClient{
		url:            url,
		logger:         logger,
		subscriptions:  make(map[int]*Subscription),
		handlers:       make(map[string]EventHandler),
		ctx:            ctx,
		cancel:         cancel,
		reconnectDelay: 5 * time.Second,
	}
}

// Connect establishes WebSocket connection
func (ws *WSClient) Connect() error {
	ws.logger.WithField("url", ws.url).Info("🔌 Connecting to Solana WebSocket")

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, resp, err := dialer.Dial(ws.url, nil)
	if err != nil {
		if resp != nil {
			ws.logger.WithFields(logrus.Fields{
				"status": resp.Status,
				"url":    ws.url,
			}).Error("WebSocket connection failed with HTTP response")
		}
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.mu.Unlock()

	ws.logger.WithFields(logrus.Fields{
		"url":    ws.url,
		"status": "connected",
	}).Info("✅ WebSocket connected successfully")

	// Start message handler
	go ws.handleMessages()

	// Start ping handler
	go ws.pingHandler()

	return nil
}

// Disconnect closes the WebSocket connection
func (ws *WSClient) Disconnect() error {
	ws.cancel()

	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.conn != nil {
		err := ws.conn.Close()
		ws.conn = nil
		return err
	}

	return nil
}

// Subscribe subscribes to a WebSocket method
func (ws *WSClient) Subscribe(method string, params interface{}, handler EventHandler) (int, error) {
	ws.mu.Lock()
	id := ws.nextID
	ws.nextID++
	ws.mu.Unlock()

	subscription := &Subscription{
		ID:      id,
		Method:  method,
		Params:  params,
		Handler: handler,
		Active:  false,
	}

	message := WSMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	ws.logger.WithFields(logrus.Fields{
		"method": method,
		"id":     id,
		"params": params,
	}).Info("📡 Sending WebSocket subscription request")

	if err := ws.sendMessage(message); err != nil {
		return 0, fmt.Errorf("failed to send subscription: %w", err)
	}

	ws.mu.Lock()
	ws.subscriptions[id] = subscription
	ws.mu.Unlock()

	ws.logger.WithFields(logrus.Fields{
		"method": method,
		"id":     id,
	}).Info("📝 Subscription request sent, waiting for confirmation")

	return id, nil
}

// Unsubscribe cancels a subscription
func (ws *WSClient) Unsubscribe(id int) error {
	ws.mu.RLock()
	subscription, exists := ws.subscriptions[id]
	ws.mu.RUnlock()

	if !exists {
		return fmt.Errorf("subscription %d not found", id)
	}

	// Get unsubscribe method name
	unsubMethod := getUnsubscribeMethod(subscription.Method)
	if unsubMethod == "" {
		return fmt.Errorf("unknown unsubscribe method for %s", subscription.Method)
	}

	message := WSMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  unsubMethod,
		Params:  []interface{}{id},
	}

	if err := ws.sendMessage(message); err != nil {
		return fmt.Errorf("failed to send unsubscribe: %w", err)
	}

	ws.mu.Lock()
	delete(ws.subscriptions, id)
	ws.mu.Unlock()

	ws.logger.WithField("id", id).Info("Subscription cancelled")
	return nil
}

// SubscribeToBlocks subscribes to block notifications
func (ws *WSClient) SubscribeToBlocks(handler EventHandler) (int, error) {
	params := []interface{}{
		"all",
		map[string]interface{}{
			"commitment":                     "confirmed",
			"encoding":                       "json",
			"transactionDetails":             "full",
			"showRewards":                    false,
			"maxSupportedTransactionVersion": 0,
		},
	}

	return ws.Subscribe("blockSubscribe", params, handler)
}

// SubscribeToLogs subscribes to log notifications
func (ws *WSClient) SubscribeToLogs(programID string, handler EventHandler) (int, error) {
	params := []interface{}{
		map[string]interface{}{
			"mentions": []string{programID},
		},
		map[string]interface{}{
			"commitment": "confirmed",
		},
	}

	return ws.Subscribe("logsSubscribe", params, handler)
}

// sendMessage sends a message over WebSocket
func (ws *WSClient) sendMessage(message WSMessage) error {
	ws.mu.RLock()
	conn := ws.conn
	ws.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("WebSocket not connected")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	ws.logger.WithFields(logrus.Fields{
		"method": message.Method,
		"id":     message.ID,
	}).Debug("Sending WebSocket message")

	return conn.WriteMessage(websocket.TextMessage, data)
}

// handleMessages handles incoming WebSocket messages
func (ws *WSClient) handleMessages() {
	defer ws.logger.Info("Message handler stopped")

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
			ws.mu.RLock()
			conn := ws.conn
			ws.mu.RUnlock()

			if conn == nil {
				ws.logger.Warn("Connection lost, attempting to reconnect...")
				if err := ws.attemptReconnect(); err != nil {
					ws.logger.WithError(err).Error("Reconnection failed")
					time.Sleep(ws.reconnectDelay)
					continue
				}
				continue
			}

			// Set read deadline
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			_, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					ws.logger.WithError(err).Error("WebSocket read error")
				}

				ws.mu.Lock()
				ws.conn = nil
				ws.mu.Unlock()

				continue
			}

			var message WSMessage
			if err := json.Unmarshal(data, &message); err != nil {
				ws.logger.WithError(err).Error("Failed to unmarshal WebSocket message")
				continue
			}

			ws.handleMessage(message)
		}
	}
}

// handleMessage processes a single WebSocket message
func (ws *WSClient) handleMessage(message WSMessage) {
	// Handle subscription confirmations
	if message.ID != nil && message.Result != nil {
		ws.mu.RLock()
		subscription, exists := ws.subscriptions[*message.ID]
		ws.mu.RUnlock()

		if exists && !subscription.Active {
			subscription.Active = true
			ws.logger.WithFields(logrus.Fields{
				"method": subscription.Method,
				"id":     *message.ID,
				"result": message.Result,
			}).Info("✅ WebSocket subscription confirmed")
		}
		return
	}

	// Handle notifications
	if message.Method != "" {
		ws.logger.WithFields(logrus.Fields{
			"method": message.Method,
			"params": message.Params,
		}).Debug("📨 Received WebSocket notification")

		switch message.Method {
		case "blockNotification":
			ws.handleBlockNotification(message.Params)
		case "logsNotification":
			ws.handleLogsNotification(message.Params)
		default:
			ws.logger.WithField("method", message.Method).Debug("❓ Unknown notification method")
		}
	}

	// Handle errors
	if message.Error != nil {
		ws.logger.WithFields(logrus.Fields{
			"code":    message.Error.Code,
			"message": message.Error.Message,
			"data":    message.Error.Data,
		}).Error("❌ WebSocket error received")
	}
}

// handleBlockNotification handles block notifications
func (ws *WSClient) handleBlockNotification(params interface{}) {
	data, err := json.Marshal(params)
	if err != nil {
		ws.logger.WithError(err).Error("Failed to marshal block notification")
		return
	}

	var notification BlockNotification
	if err := json.Unmarshal(data, &notification); err != nil {
		ws.logger.WithError(err).Error("Failed to unmarshal block notification")
		return
	}

	ws.logger.WithFields(logrus.Fields{
		"slot":         notification.Result.Value.Slot,
		"subscription": notification.Subscription,
	}).Debug("Block notification received")

	// Find and call the handler
	ws.mu.RLock()
	for _, subscription := range ws.subscriptions {
		if subscription.Method == "blockSubscribe" && subscription.Handler != nil {
			go func(handler EventHandler) {
				if err := handler(notification); err != nil {
					ws.logger.WithError(err).Error("Block notification handler error")
				}
			}(subscription.Handler)
			break
		}
	}
	ws.mu.RUnlock()
}

// handleLogsNotification handles logs notifications
func (ws *WSClient) handleLogsNotification(params interface{}) {
	data, err := json.Marshal(params)
	if err != nil {
		ws.logger.WithError(err).Error("Failed to marshal logs notification")
		return
	}

	var notification LogsNotification
	if err := json.Unmarshal(data, &notification); err != nil {
		ws.logger.WithError(err).Error("Failed to unmarshal logs notification")
		return
	}

	ws.logger.WithFields(logrus.Fields{
		"signature":    notification.Result.Value.Signature,
		"subscription": notification.Subscription,
		"logs_count":   len(notification.Result.Value.Logs),
	}).Debug("Logs notification received")

	// Find and call the handler
	ws.mu.RLock()
	for _, subscription := range ws.subscriptions {
		if subscription.Method == "logsSubscribe" && subscription.Handler != nil {
			go func(handler EventHandler) {
				if err := handler(notification); err != nil {
					ws.logger.WithError(err).Error("Logs notification handler error")
				}
			}(subscription.Handler)
			break
		}
	}
	ws.mu.RUnlock()
}

// attemptReconnect attempts to reconnect to WebSocket
func (ws *WSClient) attemptReconnect() error {
	ws.logger.Info("Attempting to reconnect WebSocket...")

	if err := ws.Connect(); err != nil {
		return fmt.Errorf("reconnection failed: %w", err)
	}

	// Resubscribe to all active subscriptions
	ws.mu.RLock()
	subscriptions := make([]*Subscription, 0, len(ws.subscriptions))
	for _, sub := range ws.subscriptions {
		subscriptions = append(subscriptions, sub)
	}
	ws.mu.RUnlock()

	for _, sub := range subscriptions {
		if _, err := ws.Subscribe(sub.Method, sub.Params, sub.Handler); err != nil {
			ws.logger.WithError(err).WithField("method", sub.Method).Error("Failed to resubscribe")
		}
	}

	ws.logger.Info("WebSocket reconnected successfully")
	return nil
}

// pingHandler sends periodic ping messages
func (ws *WSClient) pingHandler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			ws.mu.RLock()
			conn := ws.conn
			ws.mu.RUnlock()

			if conn != nil {
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					ws.logger.WithError(err).Debug("Failed to send ping")
				}
			}
		}
	}
}

// getUnsubscribeMethod returns the unsubscribe method name for a subscribe method
func getUnsubscribeMethod(subscribeMethod string) string {
	switch subscribeMethod {
	case "blockSubscribe":
		return "blockUnsubscribe"
	case "logsSubscribe":
		return "logsUnsubscribe"
	case "accountSubscribe":
		return "accountUnsubscribe"
	case "programSubscribe":
		return "programUnsubscribe"
	case "signatureSubscribe":
		return "signatureUnsubscribe"
	case "slotSubscribe":
		return "slotUnsubscribe"
	default:
		return ""
	}
}
