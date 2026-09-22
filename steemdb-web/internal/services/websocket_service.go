package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/steemit/steemdb/web/internal/database"
	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// SteemRPC is the subset of *steem.Client the WebSocket service depends on.
// Kept as an interface so tests can substitute fakes without a live node.
type SteemRPC interface {
	GetDynamicGlobalProperties() (*steem.DynamicGlobalProperties, error)
	GetBlock(blockNum int64) (*steem.Block, error)
	GetActiveWitnesses() ([]string, error)
}

// countFn estimates a collection's document count (the production
// implementation uses EstimatedDocumentCount, matching dashboard_service).
// Abstracted as a function type so tests can inject fakes without MongoDB.
type countFn func(ctx context.Context, collection string) (int64, error)

// Data pump tuning. The props/blocks pump follows the chain (a block every
// 3s), so 1s is the right cadence there. The state counts are slow-moving
// metadata (accounts/comments/witnesses totals over millions of documents);
// per-second precision was never meaningful — legacy live.py had its state
// publish commented out entirely — so they run on a slower 10s cadence that
// matches the dashboard's data freshness. Both are fields so tests can
// shrink them.
const (
	defaultFetchInterval = 1 * time.Second
	defaultStateInterval = 10 * time.Second
	// pumpRestartBackoff throttles the restart of a pump goroutine that
	// panicked on the previous iteration (see runRecoverable).
	pumpRestartBackoff = 5 * time.Second
	// replayQueueCapacity bounds pending per-connection replays. When full,
	// the oldest pending replay is dropped (see enqueueReplay).
	replayQueueCapacity = 128
)

// WebSocketService manages WebSocket connections and real-time data broadcasting
type WebSocketService struct {
	clients    map[*websocket.Conn]*Client
	clientsMux sync.RWMutex

	channels    map[string]map[*websocket.Conn]bool // channel -> connections
	channelsMux sync.RWMutex

	steemClient SteemRPC
	db          *database.MongoDB
	logger      utils.Logger

	upgrader websocket.Upgrader

	// Mentions regex for extracting @username from comments (aligned with old live.py)
	mentionsRegex *regexp.Regexp

	// Broadcasting channels
	broadcast  chan models.WebSocketMessage
	register   chan *Client
	unregister chan *Client

	// Replay pipeline: the hub loop only enqueues new connections
	// (non-blocking); a dedicated worker performs the historical-block RPCs
	// so a slow or hung replay cannot stall the hub or the data pump.
	replayQueue chan *Client

	// Data fetching
	ctx    context.Context
	cancel context.CancelFunc

	// State tracking. Accessed from the pump, the hub and replay worker
	// goroutines, hence atomics (previously a plain int64 read/written
	// across goroutines without synchronization).
	lastBlockNumber    atomic.Int64 // Last head block number (for props updates)
	lastBlockProcessed atomic.Int64 // Last irreversible block processed (for block processing)

	// Pump cadence (see defaultFetchInterval / defaultStateInterval).
	fetchInterval time.Duration
	stateInterval time.Duration

	// estimateDocCount backs the state channel's collection counts.
	estimateDocCount countFn

	// stateCounts caches the last successfully obtained collection counts so
	// a counting error degrades to a stale value instead of broadcasting
	// zeros to every dashboard. stateErrLogged dedupes the "frame skipped"
	// warning across an outage (re-armed by a successful frame).
	stateCounts    map[string]int64
	stateCountsMux sync.Mutex
	stateErrLogged bool

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

	// sendMux guards send-vs-close: the replay worker delivers messages from
	// its own goroutine, so a send racing evict()'s channel close must be
	// serialized (sending on a closed channel panics).
	sendMu   sync.Mutex
	sendOpen bool
}

// deliver sends msg to the client's send channel. It is safe to call from any
// goroutine, including after eviction started: the mutex-guarded sendOpen
// flag prevents sends on a closed channel, and a full buffer drops the
// message (slow-client policy) without blocking the caller.
func (c *Client) deliver(msg models.WebSocketMessage) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if !c.sendOpen {
		return false
	}
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

// evict closes the client's send channel exactly once. Safe to call from both
// the unregister path and slow-client eviction.
func (c *Client) evict() {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.sendOpen {
		c.sendOpen = false
		close(c.send)
	}
}

// NewWebSocketService creates a new WebSocket service
func NewWebSocketService(steemClient SteemRPC, db *database.MongoDB, logger utils.Logger, maxConnections int) *WebSocketService {
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
		replayQueue:    make(chan *Client, replayQueueCapacity),
		ctx:            ctx,
		cancel:         cancel,
		maxConnections: maxConnections,
		mentionsRegex:  regexp.MustCompile(`([@])(\w+)\b`), // Aligned with old live.py
		fetchInterval:  defaultFetchInterval,
		stateInterval:  defaultStateInterval,
		estimateDocCount: func(ctx context.Context, collection string) (int64, error) {
			// EstimatedDocumentCount reads collection metadata instead of
			// scanning; exact counts on multi-million-document collections
			// (account, comment) are a per-tick full-index load the
			// dashboard already rejected — see dashboard_service.go.
			return db.Collection(collection).EstimatedDocumentCount(ctx)
		},
		stateCounts: make(map[string]int64),
	}
}

// Start starts the WebSocket service
func (ws *WebSocketService) Start() {
	// Initialize lastBlockProcessed from current irreversible block
	props, err := ws.steemClient.GetDynamicGlobalProperties()
	if err == nil && props != nil {
		ws.lastBlockProcessed.Store(props.LastIrreversibleBlockNum)
		ws.lastBlockNumber.Store(props.HeadBlockNumber)
	}

	go ws.runRecoverable("hub", ws.run)
	go ws.runRecoverable("replay", ws.runReplayWorker)
	go ws.runRecoverable("data_pump", ws.fetchData)
}

// Stop stops the WebSocket service
func (ws *WebSocketService) Stop() {
	ws.cancel()
}

// runRecoverable keeps fn running for the life of the service. fn is expected
// to return when ws.ctx is done; if fn exits for any other reason — a
// recovered panic inside it, or an unexpected return — it is restarted after
// pumpRestartBackoff. A panic inside a bare goroutine kills the whole web
// process (gin.Recovery only covers HTTP handler goroutines), so every
// long-lived service goroutine goes through here.
func (ws *WebSocketService) runRecoverable(name string, fn func()) {
	for {
		ws.safeRun(name, fn)
		select {
		case <-ws.ctx.Done():
			return
		case <-time.After(pumpRestartBackoff):
			ws.logger.Warn("Service goroutine restarted after failure", utils.String("worker", name))
		}
	}
}

// safeRun invokes fn once, converting a panic into an error log. The panic is
// not rethrown: a single bad block or malformed RPC payload must not take
// down real-time updates for every connected client.
func (ws *WebSocketService) safeRun(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			ws.logger.Error("Panic in WebSocket service goroutine, recovering",
				utils.String("worker", name),
				utils.Any("panic", r),
				utils.String("stack", string(debug.Stack())),
			)
		}
	}()
	fn()
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
		sendOpen:      true,
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

			// Subscribe to default channels (aligned with old live.py).
			// The 10-block history replay is deliberately NOT done here: it
			// needs up to 10 sequential GetBlock RPCs, and running those in
			// the hub loop let a single connection stall the hub, back up
			// the broadcast channel and freeze the data pump. The hub only
			// enqueues; the replay worker delivers asynchronously.
			ws.subscribeClientToDefaults(client)
			ws.enqueueReplay(client)

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

// enqueueReplay hands a newly connected client to the replay worker without
// blocking the hub loop. If the queue is full, the oldest pending replay is
// dropped to make room: replays are a per-connection courtesy (the first 10
// historical blocks), not live data, and the oldest entry is the most likely
// to belong to a connection that has since disconnected. Dropping it costs
// that one client its history feed; blocking or stalling here would cost
// every client the live feed.
func (ws *WebSocketService) enqueueReplay(client *Client) {
	select {
	case ws.replayQueue <- client:
		return
	default:
	}

	// Queue full: evict the oldest pending replay and retry. Only the hub
	// goroutine sends and only the replay worker receives, so after one
	// receive there is guaranteed room for this send.
	select {
	case <-ws.replayQueue:
		ws.logger.Warn("Replay queue full, dropped oldest pending replay")
	default:
	}
	select {
	case ws.replayQueue <- client:
	default:
		// Unreachable given the single-producer/single-consumer queue; kept
		// as a non-blocking safety net.
		ws.logger.Warn("Replay queue full, dropped replay for new client")
	}
}

// runReplayWorker serializes connection replays off the hub loop. A single
// worker bounds the replay RPC load (at most one GetBlock in flight) so a
// connect storm cannot fan out into thousands of concurrent historical
// fetches; queued connections wait, and overflow is shed by enqueueReplay.
func (ws *WebSocketService) runReplayWorker() {
	for {
		select {
		case client := <-ws.replayQueue:
			ws.sendRecentBlocksToClient(client)
		case <-ws.ctx.Done():
			return
		}
	}
}

// fetchData continuously fetches data from the blockchain
func (ws *WebSocketService) fetchData() {
	ticker := time.NewTicker(ws.fetchInterval) // Fetch every 1 second (aligned with old live.py)
	defer ticker.Stop()

	stateTicker := time.NewTicker(ws.stateInterval)
	defer stateTicker.Stop()

	for {
		select {
		case <-ticker.C:
			// Each phase is individually guarded: a panic in one phase
			// (malformed block, nil SDK result) logs an error and the next
			// tick proceeds, instead of killing the pump goroutine.
			ws.safeRun("fetch_props", ws.fetchAndBroadcastProps)
			ws.safeRun("fetch_blocks", ws.fetchAndBroadcastBlocks)

		case <-stateTicker.C:
			ws.safeRun("fetch_state", ws.fetchAndBroadcastState)

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

	// Defense-in-depth nil guard: steemgosdk v0.0.31 (api/v2 GetDynamicGlobalProperties)
	// always returns a non-nil pointer on success, but the SDK is under active
	// development and the client wrapper converts a nil SDK result into (nil, nil) —
	// dereferencing props below would panic the pump.
	if props == nil {
		ws.logger.Error("GetDynamicGlobalProperties returned nil properties without error")
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
	lastBlockNumber := ws.lastBlockNumber.Load()
	if props.HeadBlockNumber != lastBlockNumber {
		ws.lastBlockNumber.Store(props.HeadBlockNumber)
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
	if props == nil {
		// Same nil guard as fetchAndBroadcastProps.
		return
	}

	irreversible := props.LastIrreversibleBlockNum

	// Process all irreversible blocks that haven't been processed yet
	for irreversible > ws.lastBlockProcessed.Load() {
		blockNum := ws.lastBlockProcessed.Add(1)

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

	// Get estimated counts from the database. On error the previously
	// broadcast value is kept: broadcasting a zero tells every dashboard the
	// chain has no accounts/comments, which is strictly worse than a stale
	// count. If a count has never succeeded, this frame is skipped entirely.
	accounts, accountsOK := ws.cachedCount(ctx, "account")
	comments, commentsOK := ws.cachedCount(ctx, "comment")

	// Witness count: database first, then upstream (witness collection may
	// legitimately be empty on a fresh database, which is not an error).
	witnesses, err := ws.estimateDocCount(ctx, "witness")
	witnessesOK := err == nil && witnesses > 0
	if !witnessesOK {
		if activeWitnesses, err := ws.steemClient.GetActiveWitnesses(); err == nil {
			witnesses = int64(len(activeWitnesses))
			witnessesOK = true
		}
	}

	if !accountsOK || !commentsOK || !witnessesOK {
		ws.warnStateUnavailableOnce(accountsOK, commentsOK, witnessesOK)
		return
	}

	stateData := models.StateData{
		LastBlock:  ws.lastBlockNumber.Load(),
		LastUpdate: time.Now(),
		Accounts:   accounts,
		Comments:   comments,
		Witnesses:  witnesses,
	}

	message := models.WebSocketMessage{
		Type:      "state",
		Channel:   "state",
		Data:      stateData,
		Timestamp: time.Now(),
	}

	// A successful frame re-arms the once-per-outage warning.
	ws.stateCountsMux.Lock()
	ws.stateErrLogged = false
	ws.stateCountsMux.Unlock()

	ws.broadcast <- message
}

// cachedCount returns the latest known count for a collection: the fresh
// estimate on success, otherwise the last successfully broadcast value. The
// second return is false only when no value was ever obtained.
func (ws *WebSocketService) cachedCount(ctx context.Context, collection string) (int64, bool) {
	if count, err := ws.estimateDocCount(ctx, collection); err == nil {
		ws.stateCountsMux.Lock()
		ws.stateCounts[collection] = count
		ws.stateCountsMux.Unlock()
		return count, true
	}
	ws.stateCountsMux.Lock()
	count, ok := ws.stateCounts[collection]
	ws.stateCountsMux.Unlock()
	return count, ok
}

// warnStateUnavailableOnce logs the "state frame skipped" condition at most
// once until a state frame succeeds again, so a long Mongo outage does not
// spam the log every interval.
func (ws *WebSocketService) warnStateUnavailableOnce(accounts, comments, witnesses bool) {
	ws.stateCountsMux.Lock()
	first := !ws.stateErrLogged
	ws.stateErrLogged = true
	ws.stateCountsMux.Unlock()
	if first {
		ws.logger.Warn("State counts unavailable, skipping state broadcast (stale or no counts)",
			utils.Bool("accounts", accounts),
			utils.Bool("comments", comments),
			utils.Bool("witnesses", witnesses))
	}
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
			if !client.deliver(message) {
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

// sendRecentBlocksToClient sends the last 10 processed blocks to a newly
// connected client (aligned with old live.py). Runs on the replay worker, not
// the hub loop: the sequential GetBlock RPCs here may take seconds and must
// not stall live broadcasting. Replay messages may interleave with live
// broadcasts on the client's send channel — each message carries its block
// number, and losing strict ordering for the first second of a connection is
// a fair trade for not blocking every other client.
func (ws *WebSocketService) sendRecentBlocksToClient(client *Client) {
	// Send last 10 blocks (aligned with old live.py: for x in range(1, 11))
	lastBlockProcessed := ws.lastBlockProcessed.Load()
	startBlock := lastBlockProcessed - 9
	if startBlock < 1 {
		startBlock = 1
	}

	for blockNum := startBlock; blockNum <= lastBlockProcessed; blockNum++ {
		// A shutdown must not wait out the remaining replay RPCs.
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

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

		if !client.deliver(message) {
			// Client's send buffer is full (slow) or the client is gone.
			ws.logger.Warn("Skipped historical block for client", utils.Int64("block_number", blockNum))
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
