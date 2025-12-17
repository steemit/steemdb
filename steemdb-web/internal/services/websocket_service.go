package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/gorilla/websocket"
	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// WebSocketService manages WebSocket connections and real-time data broadcasting
type WebSocketService struct {
	clients    map[*websocket.Conn]*Client
	clientsMux sync.RWMutex

	channels    map[string]map[*websocket.Conn]bool // channel -> connections
	channelsMux sync.RWMutex

	steemClient *steem.Client
	db          *database.MongoDB
	logger      utils.Logger

	upgrader websocket.Upgrader

	// Broadcasting channels
	broadcast  chan models.WebSocketMessage
	register   chan *Client
	unregister chan *Client

	// Data fetching
	ctx    context.Context
	cancel context.CancelFunc

	// State tracking
	lastBlockNumber int64
	lastPropsUpdate time.Time
}

// Client represents a WebSocket client connection
type Client struct {
	conn          *websocket.Conn
	send          chan models.WebSocketMessage
	service       *WebSocketService
	subscriptions map[string]bool // subscribed channels
	userAccount   string          // for @username subscriptions
}

// NewWebSocketService creates a new WebSocket service
func NewWebSocketService(steemClient *steem.Client, db *database.MongoDB, logger utils.Logger) *WebSocketService {
	ctx, cancel := context.WithCancel(context.Background())

	return &WebSocketService{
		clients:     make(map[*websocket.Conn]*Client),
		channels:    make(map[string]map[*websocket.Conn]bool),
		steemClient: steemClient,
		db:          db,
		logger:      logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now - should be configured in production
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		broadcast:  make(chan models.WebSocketMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start starts the WebSocket service
func (ws *WebSocketService) Start() {
	go ws.run()
	go ws.fetchData()
}

// Stop stops the WebSocket service
func (ws *WebSocketService) Stop() {
	ws.cancel()
}

// HandleWebSocket handles WebSocket connections
func (ws *WebSocketService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		ws.logger.Error("Failed to upgrade WebSocket connection", utils.Error(err))
		return
	}

	client := &Client{
		conn:          conn,
		send:          make(chan models.WebSocketMessage, 256),
		service:       ws,
		subscriptions: make(map[string]bool),
	}

	ws.register <- client

	// Start goroutines for this client
	go client.writePump()
	go client.readPump()
}

// run handles the main WebSocket service loop
func (ws *WebSocketService) run() {
	for {
		select {
		case client := <-ws.register:
			ws.clientsMux.Lock()
			ws.clients[client.conn] = client
			ws.clientsMux.Unlock()
			ws.logger.Info("Client connected", utils.String("remote_addr", client.conn.RemoteAddr().String()))

		case client := <-ws.unregister:
			ws.clientsMux.Lock()
			if _, ok := ws.clients[client.conn]; ok {
				delete(ws.clients, client.conn)
				close(client.send)

				// Remove from all channels
				ws.channelsMux.Lock()
				for channel := range client.subscriptions {
					if clients, exists := ws.channels[channel]; exists {
						delete(clients, client.conn)
						if len(clients) == 0 {
							delete(ws.channels, channel)
						}
					}
				}
				ws.channelsMux.Unlock()
			}
			ws.clientsMux.Unlock()
			ws.logger.Info("Client disconnected", utils.String("remote_addr", client.conn.RemoteAddr().String()))

		case message := <-ws.broadcast:
			ws.broadcastToChannel(message.Channel, message)

		case <-ws.ctx.Done():
			return
		}
	}
}

// fetchData continuously fetches data from the blockchain
func (ws *WebSocketService) fetchData() {
	ticker := time.NewTicker(3 * time.Second) // Fetch every 3 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ws.fetchAndBroadcastProps()
			ws.fetchAndBroadcastBlocks()
			ws.fetchAndBroadcastState()

		case <-ws.ctx.Done():
			return
		}
	}
}

// fetchAndBroadcastProps fetches and broadcasts blockchain properties
func (ws *WebSocketService) fetchAndBroadcastProps() {
	props, err := ws.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		ws.logger.Error("Failed to fetch dynamic global properties", utils.Error(err))
		return
	}

	propsData := models.PropsData{
		HeadBlockNumber:              props.HeadBlockNumber,
		HeadBlockID:                  props.HeadBlockID,
		Time:                         props.Time,
		CurrentWitness:               props.CurrentWitness,
		TotalPow:                     props.TotalPow,
		NumPowWitnesses:              props.NumPowWitnesses,
		VirtualSupply:                props.VirtualSupply,
		CurrentSupply:                props.CurrentSupply,
		ConfidentialSupply:           props.ConfidentialSupply,
		CurrentSBDSupply:             props.CurrentSBDSupply,
		ConfidentialSBDSupply:        props.ConfidentialSBDSupply,
		TotalVestingFundSteem:        props.TotalVestingFundSteem,
		TotalVestingShares:           props.TotalVestingShares,
		TotalRewardFundSteem:         props.TotalRewardFundSteem,
		TotalRewardShares2:           props.TotalRewardShares2,
		PendingRewardedVestingShares: props.PendingRewardedVestingShares,
		PendingRewardedVestingSteem:  props.PendingRewardedVestingSteem,
		SBDInterestRate:              props.SBDInterestRate,
		SBDPrintRate:                 props.SBDPrintRate,
		MaximumBlockSize:             props.MaximumBlockSize,
		CurrentAslot:                 props.CurrentAslot,
		RecentSlotsFilled:            props.RecentSlotsFilled,
		ParticipationCount:           props.ParticipationCount,
		LastIrreversibleBlockNum:     props.LastIrreversibleBlockNum,
		VotePowerReserveRate:         props.VotePowerReserveRate,
	}

	message := models.WebSocketMessage{
		Type:      "props",
		Channel:   "props",
		Data:      propsData,
		Timestamp: time.Now(),
	}

	ws.broadcast <- message
}

// fetchAndBroadcastBlocks fetches and broadcasts new blocks
func (ws *WebSocketService) fetchAndBroadcastBlocks() {
	props, err := ws.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		return
	}

	currentBlock := props.HeadBlockNumber
	if currentBlock <= ws.lastBlockNumber {
		return
	}

	// Broadcast new blocks
	for blockNum := ws.lastBlockNumber + 1; blockNum <= currentBlock; blockNum++ {
		block, err := ws.steemClient.GetBlock(blockNum)
		if err != nil {
			ws.logger.Error("Failed to fetch block", utils.Int64("block_number", blockNum), utils.Error(err))
			continue
		}

		blockData := models.BlockData{
			Number:       blockNum,
			Timestamp:    block.Timestamp,
			Witness:      block.Witness,
			Transactions: len(block.Transactions),
			Operations:   ws.countOperations(block),
		}

		message := models.WebSocketMessage{
			Type:      "block",
			Channel:   "blocks",
			Data:      blockData,
			Timestamp: time.Now(),
		}

		ws.broadcast <- message

		// Process operations for account notifications
		ws.processBlockOperations(block, blockNum)
	}

	ws.lastBlockNumber = currentBlock
}

// fetchAndBroadcastState fetches and broadcasts global state
func (ws *WebSocketService) fetchAndBroadcastState() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get account count from database
	accountCount := int64(0)
	accountCollection := ws.db.Collection("account")
	if count, err := accountCollection.CountDocuments(ctx, bson.M{}); err == nil {
		accountCount = count
	}

	// Get comment count from database
	commentCount := int64(0)
	commentCollection := ws.db.Collection("comment")
	if count, err := commentCollection.CountDocuments(ctx, bson.M{}); err == nil {
		commentCount = count
	}

	// Get witness count - try database first, then upstream
	witnessCount := int64(0)
	witnessCollection := ws.db.Collection("witness")
	if count, err := witnessCollection.CountDocuments(ctx, bson.M{}); err == nil && count > 0 {
		witnessCount = count
	} else {
		// Fallback to upstream if database doesn't have witness data
		if activeWitnesses, err := ws.steemClient.GetActiveWitnesses(); err == nil {
			witnessCount = int64(len(activeWitnesses))
		}
	}

	stateData := models.StateData{
		LastBlock:  ws.lastBlockNumber,
		LastUpdate: time.Now(),
		Accounts:   accountCount,
		Comments:   commentCount,
		Witnesses:  witnessCount,
	}

	message := models.WebSocketMessage{
		Type:      "state",
		Channel:   "state",
		Data:      stateData,
		Timestamp: time.Now(),
	}

	ws.broadcast <- message
}

// processBlockOperations processes operations for account notifications
func (ws *WebSocketService) processBlockOperations(block *steem.Block, blockNum int64) {
	for _, tx := range block.Transactions {
		for _, op := range tx.Operations {
			opData := models.OperationData{
				Type:      op.Type,
				Block:     blockNum,
				Timestamp: block.Timestamp,
				Data:      op.Value,
			}

			// Extract affected accounts based on operation type
			accounts := ws.extractAccountsFromOperation(op)
			opData.Accounts = accounts

			// Broadcast to account-specific channels
			for _, account := range accounts {
				channel := fmt.Sprintf("@%s", account)
				message := models.WebSocketMessage{
					Type:      "operation",
					Channel:   channel,
					Data:      opData,
					Timestamp: time.Now(),
				}
				ws.broadcast <- message
			}
		}
	}
}

// extractAccountsFromOperation extracts account names from operations
func (ws *WebSocketService) extractAccountsFromOperation(op steem.Operation) []string {
	accounts := make([]string, 0)

	// Convert operation value to map for easier access
	if opMap, ok := op.Value.(map[string]interface{}); ok {
		// Common account fields
		if author, exists := opMap["author"]; exists {
			if authorStr, ok := author.(string); ok {
				accounts = append(accounts, authorStr)
			}
		}
		if from, exists := opMap["from"]; exists {
			if fromStr, ok := from.(string); ok {
				accounts = append(accounts, fromStr)
			}
		}
		if to, exists := opMap["to"]; exists {
			if toStr, ok := to.(string); ok {
				accounts = append(accounts, toStr)
			}
		}
		if voter, exists := opMap["voter"]; exists {
			if voterStr, ok := voter.(string); ok {
				accounts = append(accounts, voterStr)
			}
		}
		if account, exists := opMap["account"]; exists {
			if accountStr, ok := account.(string); ok {
				accounts = append(accounts, accountStr)
			}
		}
	}

	return accounts
}

// countOperations counts total operations in a block
func (ws *WebSocketService) countOperations(block *steem.Block) int {
	count := 0
	for _, tx := range block.Transactions {
		count += len(tx.Operations)
	}
	return count
}

// broadcastToChannel broadcasts a message to all clients subscribed to a channel
func (ws *WebSocketService) broadcastToChannel(channel string, message models.WebSocketMessage) {
	ws.channelsMux.RLock()
	clients, exists := ws.channels[channel]
	if !exists {
		ws.channelsMux.RUnlock()
		return
	}

	// Create a copy of the clients map to avoid holding the lock too long
	clientsCopy := make(map[*websocket.Conn]bool)
	for conn, active := range clients {
		clientsCopy[conn] = active
	}
	ws.channelsMux.RUnlock()

	// Send message to all clients in this channel
	ws.clientsMux.RLock()
	for conn := range clientsCopy {
		if client, exists := ws.clients[conn]; exists {
			select {
			case client.send <- message:
			default:
				// Client's send channel is full, close it
				close(client.send)
				delete(ws.clients, conn)
			}
		}
	}
	ws.clientsMux.RUnlock()
}

// Client methods

// readPump handles reading messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.service.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle subscription requests
		var subReq models.SubscriptionRequest
		if err := json.Unmarshal(message, &subReq); err == nil {
			c.handleSubscription(subReq)
		}
	}
}

// writePump handles writing messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleSubscription handles subscription/unsubscription requests
func (c *Client) handleSubscription(req models.SubscriptionRequest) {
	c.service.channelsMux.Lock()
	defer c.service.channelsMux.Unlock()

	switch req.Action {
	case "subscribe":
		// Add client to channel
		if c.service.channels[req.Channel] == nil {
			c.service.channels[req.Channel] = make(map[*websocket.Conn]bool)
		}
		c.service.channels[req.Channel][c.conn] = true
		c.subscriptions[req.Channel] = true

		c.service.logger.Info("Client subscribed to channel",
			utils.String("channel", req.Channel),
			utils.String("remote_addr", c.conn.RemoteAddr().String()))

	case "unsubscribe":
		// Remove client from channel
		if clients, exists := c.service.channels[req.Channel]; exists {
			delete(clients, c.conn)
			if len(clients) == 0 {
				delete(c.service.channels, req.Channel)
			}
		}
		delete(c.subscriptions, req.Channel)

		c.service.logger.Info("Client unsubscribed from channel",
			utils.String("channel", req.Channel),
			utils.String("remote_addr", c.conn.RemoteAddr().String()))
	}
}
