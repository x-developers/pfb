package client

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// WSClient represents a WebSocket client for Solana with enhanced debugging
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

	// Debug counters
	messagesReceived int
	messagesSent     int
	reconnectCount   int
	lastActivity     time.Time
}

// Enhanced subscription tracking
type Subscription struct {
	ID          int
	Method      string
	Params      interface{}
	Handler     EventHandler
	Active      bool
	Created     time.Time
	LastMessage time.Time
}

// EventHandler handles WebSocket events
type EventHandler func(data interface{}) error

// Enhanced WebSocket message with debugging
type WSMessage struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      *int              `json:"id,omitempty"`
	Method  string            `json:"method,omitempty"`
	Params  interface{}       `json:"params,omitempty"`
	Result  interface{}       `json:"result,omitempty"`
	Error   *jsonrpc.RPCError `json:"error,omitempty"`
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

// NewWSClient creates a new WebSocket client with enhanced debugging
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
		lastActivity:   time.Now(),
	}
}

// Connect establishes WebSocket connection with enhanced debugging
func (ws *WSClient) Connect() error {
	ws.logger.WithField("url", ws.url).Info("🔌 Connecting to Solana WebSocket...")

	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	// Add headers for better connection
	headers := make(map[string][]string)
	//headers["User-Agent"] = []string{"PumpFunBot/1.0"}
	//headers["Origin"] = []string{"https://pump.fun"}

	conn, resp, err := dialer.Dial(ws.url, headers)
	if err != nil {
		if resp != nil {
			ws.logger.WithFields(logrus.Fields{
				"status":      resp.Status,
				"status_code": resp.StatusCode,
				"url":         ws.url,
			}).Error("❌ WebSocket connection failed")
		}
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.lastActivity = time.Now()
	ws.mu.Unlock()

	ws.logger.WithFields(logrus.Fields{
		"url":    ws.url,
		"status": "connected",
	}).Info("✅ WebSocket connected successfully")

	// Set connection parameters
	conn.SetReadLimit(1024 * 1024) // 1MB read limit
	conn.SetPongHandler(func(string) error {
		ws.logger.Debug("🏓 Received pong")
		ws.lastActivity = time.Now()
		return nil
	})

	// Start message handlers
	go ws.handleMessages()
	go ws.pingHandler()
	go ws.connectionMonitor()

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

// Subscribe subscribes to a WebSocket method with enhanced tracking
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
		Created: time.Now(),
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
	}).Info("📝 Subscription request sent, waiting for confirmation...")

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

	ws.logger.WithField("id", id).Info("🗑️ Subscription cancelled")
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

// SubscribeToLogs subscribes to log notifications with enhanced parameters
func (ws *WSClient) SubscribeToLogs(programID string, handler EventHandler) (int, error) {
	var params []interface{}

	if programID == "all" {
		// Subscribe to all logs
		params = []interface{}{
			"all",
			map[string]interface{}{
				"commitment": "processed",
			},
		}
	} else {
		// Subscribe to specific program
		params = []interface{}{
			map[string]interface{}{
				"mentions": []string{programID},
			},
			map[string]interface{}{
				"commitment": "processed",
			},
		}
	}

	return ws.Subscribe("logsSubscribe", params, handler)
}

// sendMessage sends a message over WebSocket with debugging
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
		"method":       message.Method,
		"id":           message.ID,
		"message_size": len(data),
	}).Debug("📤 Sending WebSocket message")

	err = conn.WriteMessage(websocket.TextMessage, data)
	if err == nil {
		ws.messagesSent++
		ws.lastActivity = time.Now()
	}

	return err
}

// handleMessages handles incoming WebSocket messages with enhanced debugging
func (ws *WSClient) handleMessages() {
	defer ws.logger.Info("🛑 Message handler stopped")

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
			ws.mu.RLock()
			conn := ws.conn
			ws.mu.RUnlock()

			if conn == nil {
				ws.logger.Warn("⚠️ Connection lost, attempting to reconnect...")
				if err := ws.attemptReconnect(); err != nil {
					ws.logger.WithError(err).Error("❌ Reconnection failed")
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
					ws.logger.WithError(err).Error("❌ WebSocket read error")
				}

				ws.mu.Lock()
				ws.conn = nil
				ws.mu.Unlock()

				continue
			}

			ws.messagesReceived++
			ws.lastActivity = time.Now()

			var message WSMessage
			if err := json.Unmarshal(data, &message); err != nil {
				ws.logger.WithError(err).WithField("data", string(data[:min(len(data), 200)])).Error("❌ Failed to unmarshal WebSocket message")
				continue
			}

			ws.handleMessage(message)
		}
	}
}

// handleMessage processes a single WebSocket message with enhanced debugging
func (ws *WSClient) handleMessage(message WSMessage) {
	// Handle subscription confirmations
	if message.ID != nil && message.Result != nil {
		ws.mu.RLock()
		subscription, exists := ws.subscriptions[*message.ID]
		ws.mu.RUnlock()

		if exists && !subscription.Active {
			subscription.Active = true
			subscription.LastMessage = time.Now()

			ws.logger.WithFields(logrus.Fields{
				"method": subscription.Method,
				"id":     *message.ID,
				"result": message.Result,
			}).Info("✅ WebSocket subscription confirmed")
		}
		return
	}

	// Handle error responses
	if message.Error != nil {
		ws.logger.WithFields(logrus.Fields{
			"code":    message.Error.Code,
			"message": message.Error.Message,
			"data":    message.Error.Data,
		}).Error("❌ WebSocket error received")
		return
	}

	// Handle notifications
	if message.Method != "" {
		ws.logger.WithFields(logrus.Fields{
			"method": message.Method,
		}).Debug("📨 Received WebSocket notification")

		switch message.Method {
		case "blockNotification":
			ws.handleBlockNotification(message.Params)
		case "logsNotification":
			ws.handleLogsNotification(message.Params)
		default:
			ws.logger.WithFields(logrus.Fields{
				"method": message.Method,
				"params": message.Params,
			}).Debug("❓ Unknown notification method")
		}
	}
}

// handleBlockNotification handles block notifications
func (ws *WSClient) handleBlockNotification(params interface{}) {
	data, err := json.Marshal(params)
	if err != nil {
		ws.logger.WithError(err).Error("❌ Failed to marshal block notification")
		return
	}

	var notification BlockNotification
	if err := json.Unmarshal(data, &notification); err != nil {
		ws.logger.WithError(err).Error("❌ Failed to unmarshal block notification")
		return
	}

	ws.logger.WithFields(logrus.Fields{
		"slot":         notification.Result.Value.Slot,
		"subscription": notification.Subscription,
	}).Debug("📦 Block notification received")

	// Find and call the handler
	ws.mu.RLock()
	for _, subscription := range ws.subscriptions {
		if subscription.Method == "blockSubscribe" && subscription.Handler != nil {
			subscription.LastMessage = time.Now()
			go func(handler EventHandler) {
				if err := handler(notification); err != nil {
					ws.logger.WithError(err).Error("❌ Block notification handler error")
				}
			}(subscription.Handler)
			break
		}
	}
	ws.mu.RUnlock()
}

// handleLogsNotification handles logs notifications with enhanced debugging
func (ws *WSClient) handleLogsNotification(params interface{}) {
	data, err := json.Marshal(params)
	if err != nil {
		ws.logger.WithError(err).Error("❌ Failed to marshal logs notification")
		return
	}

	var notification LogsNotification
	if err := json.Unmarshal(data, &notification); err != nil {
		ws.logger.WithError(err).Error("❌ Failed to unmarshal logs notification")
		return
	}

	ws.logger.WithFields(logrus.Fields{
		"signature":    notification.Result.Value.Signature,
		"subscription": notification.Subscription,
		"logs_count":   len(notification.Result.Value.Logs),
		"slot":         notification.Result.Context.Slot,
		"has_error":    notification.Result.Value.Err != nil,
	}).Debug("📋 Logs notification received")

	// Debug: Log some of the actual log messages
	if ws.logger.Level == logrus.DebugLevel {
		for i, log := range notification.Result.Value.Logs {
			if i < 3 { // Only log first 3 to avoid spam
				ws.logger.Debugf("  Log[%d]: %s", i, log)
			}
		}
		if len(notification.Result.Value.Logs) > 3 {
			ws.logger.Debugf("  ... and %d more logs", len(notification.Result.Value.Logs)-3)
		}
	}

	// Find and call the handler
	ws.mu.RLock()
	handlerCalled := false
	for _, subscription := range ws.subscriptions {
		if subscription.Method == "logsSubscribe" && subscription.Handler != nil {
			subscription.LastMessage = time.Now()
			handlerCalled = true

			go func(handler EventHandler, subID int) {
				if err := handler(notification); err != nil {
					ws.logger.WithError(err).WithField("subscription_id", subID).Error("❌ Logs notification handler error")
				}
			}(subscription.Handler, subscription.ID)
		}
	}
	ws.mu.RUnlock()

	if !handlerCalled {
		ws.logger.Warn("⚠️ No handler found for logs notification")
	}
}

// attemptReconnect attempts to reconnect to WebSocket with enhanced retry logic
func (ws *WSClient) attemptReconnect() error {
	ws.reconnectCount++
	ws.logger.WithField("attempt", ws.reconnectCount).Info("🔄 Attempting to reconnect WebSocket...")

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

	resubscribeCount := 0
	for _, sub := range subscriptions {
		if _, err := ws.Subscribe(sub.Method, sub.Params, sub.Handler); err != nil {
			ws.logger.WithError(err).WithField("method", sub.Method).Error("❌ Failed to resubscribe")
		} else {
			resubscribeCount++
		}
	}

	ws.logger.WithFields(logrus.Fields{
		"reconnect_count":     ws.reconnectCount,
		"resubscribed":        resubscribeCount,
		"total_subscriptions": len(subscriptions),
	}).Info("✅ WebSocket reconnected successfully")

	return nil
}

// pingHandler sends periodic ping messages with enhanced monitoring
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
			lastActivity := ws.lastActivity
			ws.mu.RUnlock()

			if conn != nil {
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					ws.logger.WithError(err).Debug("❌ Failed to send ping")
				} else {
					ws.logger.Debug("🏓 Sent ping")
				}

				// Check for connection staleness
				if time.Since(lastActivity) > 2*time.Minute {
					ws.logger.WithField("last_activity", lastActivity).Warn("⚠️ Connection appears stale - no activity for 2+ minutes")
				}
			}
		}
	}
}

// connectionMonitor monitors connection health and provides statistics
func (ws *WSClient) connectionMonitor() {
	ticker := time.NewTicker(60 * time.Second) // Report every minute
	defer ticker.Stop()

	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			ws.mu.RLock()
			activeSubscriptions := 0
			for _, sub := range ws.subscriptions {
				if sub.Active {
					activeSubscriptions++
				}
			}

			stats := map[string]interface{}{
				"messages_received":    ws.messagesReceived,
				"messages_sent":        ws.messagesSent,
				"active_subscriptions": activeSubscriptions,
				"total_subscriptions":  len(ws.subscriptions),
				"reconnect_count":      ws.reconnectCount,
				"last_activity":        ws.lastActivity,
				"connection_active":    ws.conn != nil,
			}
			ws.mu.RUnlock()

			ws.logger.WithFields(stats).Info("📊 WebSocket Connection Statistics")

			// Health checks
			if ws.messagesReceived == 0 && time.Since(ws.lastActivity) > 5*time.Minute {
				ws.logger.Error("🚨 WebSocket health check failed - no messages received in 5+ minutes")
			}

			if activeSubscriptions == 0 {
				ws.logger.Warn("⚠️ No active subscriptions - may not receive notifications")
			}
		}
	}
}

// GetConnectionStats returns current connection statistics
func (ws *WSClient) GetConnectionStats() map[string]interface{} {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	activeSubscriptions := 0
	subscriptionDetails := make(map[string]interface{})

	for id, sub := range ws.subscriptions {
		if sub.Active {
			activeSubscriptions++
		}
		subscriptionDetails[fmt.Sprintf("sub_%d", id)] = map[string]interface{}{
			"method":       sub.Method,
			"active":       sub.Active,
			"created":      sub.Created,
			"last_message": sub.LastMessage,
		}
	}

	return map[string]interface{}{
		"messages_received":    ws.messagesReceived,
		"messages_sent":        ws.messagesSent,
		"active_subscriptions": activeSubscriptions,
		"total_subscriptions":  len(ws.subscriptions),
		"reconnect_count":      ws.reconnectCount,
		"last_activity":        ws.lastActivity,
		"connection_active":    ws.conn != nil,
		"subscriptions":        subscriptionDetails,
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

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
