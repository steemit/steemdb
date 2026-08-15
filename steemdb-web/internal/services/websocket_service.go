package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
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

	// Mentions regex for extracting @username from comments (aligned with old live.py)
	mentionsRegex *regexp.Regexp

	// Broadcasting channels
	broadcast  chan models.WebSocketMessage
	register   chan *Client
	unregister chan *Client

	// Data fetching
	ctx    context.Context
	cancel context.CancelFunc

	// State tracking
	lastBlockNumber    int64 // Last head block number (for props updates)
	lastBlockProcessed int64 // Last irreversible block processed (for block processing)
	lastPropsUpdate    time.Time

	// Connection cap (websocket.max_connections; default 1000)
	maxConnections int
}

// Client represents a WebSocket client connection
type Client struct {
	conn          *websocket.Conn
	send          chan models.WebSocketMessage
	service       *WebSocketService
	subscriptions map[string]bool // subscribed channels
	userAccount   string          // for @username subscriptions
	closeOnce     sync.Once       // guards against double-closing send
}

// evict closes the client's send channel exactly once. Safe to call from both
// the unregister path and slow-client eviction.
func (c *Client) evict() {
	c.closeOnce.Do(func() { close(c.send) })
}

// NewWebSocketService creates a new WebSocket service
func NewWebSocketService(steemClient *steem.Client, db *database.MongoDB, logger utils.Logger, maxConnections int) *WebSocketService {
	ctx, cancel := context.WithCancel(context.Background())

	if maxConnections <= 0 {
		maxConnections = 1000
	}

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
		broadcast:      make(chan models.WebSocketMessage, 1024),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		ctx:            ctx,
		cancel:         cancel,
		maxConnections: maxConnections,
		mentionsRegex:  regexp.MustCompile(`([@])(\w+)\b`), // Aligned with old live.py
	}
}

// Start starts the WebSocket service
func (ws *WebSocketService) Start() {
	// Initialize lastBlockProcessed from current irreversible block
	props, err := ws.steemClient.GetDynamicGlobalProperties()
	if err == nil && props != nil {
		ws.lastBlockProcessed = props.LastIrreversibleBlockNum
		ws.lastBlockNumber = props.HeadBlockNumber
	}

	go ws.run()
	go ws.fetchData()
}

// Stop stops the WebSocket service
func (ws *WebSocketService) Stop() {
	ws.cancel()
}

// HandleWebSocket handles WebSocket connections
func (ws *WebSocketService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Enforce the connection cap before upgrading
	ws.clientsMux.RLock()
	atCapacity := len(ws.clients) >= ws.maxConnections
	ws.clientsMux.RUnlock()
	if atCapacity {
		http.Error(w, "too many WebSocket connections", http.StatusServiceUnavailable)
		return
	}

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

			// Subscribe to default channels and send recent blocks (aligned with old live.py)
			ws.subscribeClientToDefaults(client)
			ws.sendRecentBlocksToClient(client)

		case client := <-ws.unregister:
			ws.clientsMux.Lock()
			if _, ok := ws.clients[client.conn]; ok {
				delete(ws.clients, client.conn)
				client.evict()

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
	ticker := time.NewTicker(1 * time.Second) // Fetch every 1 second (aligned with old live.py)
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

	// Calculate steem_per_mvests (aligned with old live.py)
	steemPerMVests := float64(0)
	if props.TotalVestingFundSteem != "" && props.TotalVestingShares != "" {
		// Parse amounts (format: "123.456 STEEM" or "123.456")
		// Extract numeric part before space
		var totalVestingFundStr, totalVestingSharesStr string
		if idx := len(props.TotalVestingFundSteem); idx > 0 {
			// Find space or use whole string
			for i, r := range props.TotalVestingFundSteem {
				if r == ' ' {
					totalVestingFundStr = props.TotalVestingFundSteem[:i]
					break
				}
			}
			if totalVestingFundStr == "" {
				totalVestingFundStr = props.TotalVestingFundSteem
			}
		}
		if idx := len(props.TotalVestingShares); idx > 0 {
			for i, r := range props.TotalVestingShares {
				if r == ' ' {
					totalVestingSharesStr = props.TotalVestingShares[:i]
					break
				}
			}
			if totalVestingSharesStr == "" {
				totalVestingSharesStr = props.TotalVestingShares
			}
		}

		var totalVestingFund, totalVestingShares float64
		if _, err := fmt.Sscanf(totalVestingFundStr, "%f", &totalVestingFund); err == nil {
			if _, err := fmt.Sscanf(totalVestingSharesStr, "%f", &totalVestingShares); err == nil {
				if totalVestingShares > 0 {
					steemPerMVests = (totalVestingFund / totalVestingShares) * 1000000
					// Round to 3 decimal places (aligned with old live.py: math.floor(... * 1000) / 1000)
					steemPerMVests = float64(int64(steemPerMVests*1000)) / 1000
				}
			}
		}
	}

	// Calculate reversible_blocks
	reversibleBlocks := props.HeadBlockNumber - props.LastIrreversibleBlockNum

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
		SteemPerMVests:               steemPerMVests,
		ReversibleBlocks:             reversibleBlocks,
	}

	// Update lastBlockNumber for props change detection
	if props.HeadBlockNumber != ws.lastBlockNumber {
		ws.lastBlockNumber = props.HeadBlockNumber
		message := models.WebSocketMessage{
			Type:      "props",
			Channel:   "props",
			Data:      propsData,
			Timestamp: time.Now(),
		}
		ws.broadcast <- message
	}
}

// fetchAndBroadcastBlocks fetches and broadcasts new blocks
// Aligned with old live.py: processes only irreversible blocks
func (ws *WebSocketService) fetchAndBroadcastBlocks() {
	props, err := ws.steemClient.GetDynamicGlobalProperties()
	if err != nil {
		return
	}

	irreversible := props.LastIrreversibleBlockNum

	// Process all irreversible blocks that haven't been processed yet
	for irreversible > ws.lastBlockProcessed {
		ws.lastBlockProcessed++
		blockNum := ws.lastBlockProcessed

		block, err := ws.steemClient.GetBlock(blockNum)
		if err != nil {
			ws.logger.Error("Failed to fetch block", utils.Int64("block_number", blockNum), utils.Error(err))
			continue
		}

		// Extract accounts and count operations (aligned with old live.py)
		accountsSet := make(map[string]bool)
		opTypes := make([]string, 0)
		opCount := 0

		for _, tx := range block.Transactions {
			for _, op := range tx.Operations {
				opCount++
				opTypes = append(opTypes, op.Type)

				// Extract accounts from operation
				relatedAccounts := ws.extractAccountsFromOperation(op)
				for _, account := range relatedAccounts {
					accountsSet[account] = true
				}
			}
		}

		// Count operation types (aligned with old live.py)
		opCounts := make(map[string]int)
		for _, opType := range opTypes {
			opCounts[opType]++
		}

		// Convert accounts set to slice
		accounts := make([]string, 0, len(accountsSet))
		for account := range accountsSet {
			accounts = append(accounts, account)
		}

		blockData := models.BlockData{
			Number:       blockNum,
			Timestamp:    block.Timestamp,
			Witness:      block.Witness,
			Transactions: len(block.Transactions),
			Operations:   opCount,
			Accounts:     accounts,
			OpCounts:     opCounts,
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

// processBlockOperations processes operations for account notifications and
// the global operation feed channel
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

			// Broadcast to the global operation feed channel (consumed by the
			// live feed page)
			ws.broadcast <- models.WebSocketMessage{
				Type:      "operation",
				Channel:   "operation",
				Data:      opData,
				Timestamp: time.Now(),
			}

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
// Aligned with old live.py getRelatedAccounts method
func (ws *WebSocketService) extractAccountsFromOperation(op steem.Operation) []string {
	accountsSet := make(map[string]bool)

	// Convert operation value to map for easier access
	opMap, ok := op.Value.(map[string]interface{})
	if !ok {
		return []string{}
	}

	// Operation type to field mapping (aligned with old live.py fieldMap)
	fieldMap := map[string][]string{
		"account_create":        {},
		"account_update":        {},
		"account_witness_vote":  {"account", "witness"},
		"author_reward":         {"author"},
		"comment":               {"author", "parent_author"},
		"convert":               {},
		"curation_reward":       {"curator"},
		"custom_json":           {},
		"feed_publish":          {},
		"fill_order":            {},
		"fill_vesting_withdraw": {},
		"limit_order_cancel":    {},
		"limit_order_create":    {},
		"pow2":                  {},
		"transfer":              {"from", "to"},
		"transfer_to_vesting":   {"from", "to"},
		"vote":                  {"author", "voter"},
	}

	// Extract accounts based on operation type
	if fields, exists := fieldMap[op.Type]; exists {
		for _, field := range fields {
			if value, exists := opMap[field]; exists {
				if valueStr, ok := value.(string); ok && valueStr != "" {
					accountsSet[valueStr] = true
				}
			}
		}
	}

	// Extract mentions from comment body (aligned with old live.py)
	if op.Type == "comment" {
		if body, exists := opMap["body"]; exists {
			if bodyStr, ok := body.(string); ok {
				matches := ws.mentionsRegex.FindAllStringSubmatch(bodyStr, -1)
				for _, match := range matches {
					if len(match) >= 3 {
						accountsSet[match[2]] = true // match[2] is the username without @
					}
				}
			}
		}
	}

	// Convert set to slice
	accounts := make([]string, 0, len(accountsSet))
	for account := range accountsSet {
		accounts = append(accounts, account)
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

	// Send message to all clients in this channel; collect slow clients so the
	// eviction (map delete + channel close) happens under a write lock, never
	// inside the read lock.
	ws.clientsMux.RLock()
	slow := make([]*Client, 0)
	for conn := range clientsCopy {
		if client, exists := ws.clients[conn]; exists {
			select {
			case client.send <- message:
			default:
				slow = append(slow, client)
			}
		}
	}
	ws.clientsMux.RUnlock()

	if len(slow) > 0 {
		ws.clientsMux.Lock()
		for _, client := range slow {
			if _, exists := ws.clients[client.conn]; exists {
				delete(ws.clients, client.conn)
				client.evict()
			}
		}
		ws.clientsMux.Unlock()
	}
}

// subscribeClientToDefaults subscribes a new client to default channels (aligned with old live.py)
func (ws *WebSocketService) subscribeClientToDefaults(client *Client) {
	ws.channelsMux.Lock()
	defer ws.channelsMux.Unlock()

	defaultChannels := []string{"blocks", "props", "state"}
	for _, channel := range defaultChannels {
		if ws.channels[channel] == nil {
			ws.channels[channel] = make(map[*websocket.Conn]bool)
		}
		ws.channels[channel][client.conn] = true
		client.subscriptions[channel] = true
	}
}

// sendRecentBlocksToClient sends the last 10 processed blocks to a newly connected client (aligned with old live.py)
func (ws *WebSocketService) sendRecentBlocksToClient(client *Client) {
	// Send last 10 blocks (aligned with old live.py: for x in range(1, 11))
	startBlock := ws.lastBlockProcessed - 9
	if startBlock < 1 {
		startBlock = 1
	}

	for blockNum := startBlock; blockNum <= ws.lastBlockProcessed; blockNum++ {
		block, err := ws.steemClient.GetBlock(blockNum)
		if err != nil {
			ws.logger.Warn("Failed to fetch block for client history", utils.Int64("block_number", blockNum), utils.Error(err))
			continue
		}

		// Extract accounts and count operations (same logic as fetchAndBroadcastBlocks)
		accountsSet := make(map[string]bool)
		opTypes := make([]string, 0)
		opCount := 0

		for _, tx := range block.Transactions {
			for _, op := range tx.Operations {
				opCount++
				opTypes = append(opTypes, op.Type)

				relatedAccounts := ws.extractAccountsFromOperation(op)
				for _, account := range relatedAccounts {
					accountsSet[account] = true
				}
			}
		}

		// Count operation types
		opCounts := make(map[string]int)
		for _, opType := range opTypes {
			opCounts[opType]++
		}

		// Convert accounts set to slice
		accounts := make([]string, 0, len(accountsSet))
		for account := range accountsSet {
			accounts = append(accounts, account)
		}

		blockData := models.BlockData{
			Number:       blockNum,
			Timestamp:    block.Timestamp,
			Witness:      block.Witness,
			Transactions: len(block.Transactions),
			Operations:   opCount,
			Accounts:     accounts,
			OpCounts:     opCounts,
		}

		message := models.WebSocketMessage{
			Type:      "block",
			Channel:   "blocks",
			Data:      blockData,
			Timestamp: time.Now(),
		}

		// Send to client (non-blocking)
		select {
		case client.send <- message:
		default:
			// Client's send channel is full, skip this block
			ws.logger.Warn("Client send channel full, skipping historical block", utils.Int64("block_number", blockNum))
		}
	}
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
