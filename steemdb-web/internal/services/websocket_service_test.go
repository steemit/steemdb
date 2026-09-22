package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/steemit/steemdb/web/internal/models"
	"github.com/steemit/steemdb/web/pkg/steem"
	"github.com/steemit/steemdb/web/pkg/utils"
)

// fakeSteemRPC stands in for *steem.Client. Behavior hooks are plain
// functions so each test shapes the chain interaction it needs.
type fakeSteemRPC struct {
	mu sync.Mutex

	propsFn func(call int) (*steem.DynamicGlobalProperties, error)
	blockFn func(blockNum int64) (*steem.Block, error)

	propsCalls     int
	witnesses      []string
	witnessesError error
}

func (f *fakeSteemRPC) GetDynamicGlobalProperties() (*steem.DynamicGlobalProperties, error) {
	f.mu.Lock()
	f.propsCalls++
	call := f.propsCalls
	fn := f.propsFn
	f.mu.Unlock()
	if fn == nil {
		return &steem.DynamicGlobalProperties{HeadBlockNumber: 1000, LastIrreversibleBlockNum: 1000}, nil
	}
	return fn(call)
}

func (f *fakeSteemRPC) GetBlock(blockNum int64) (*steem.Block, error) {
	f.mu.Lock()
	fn := f.blockFn
	f.mu.Unlock()
	if fn == nil {
		return &steem.Block{Number: blockNum}, nil
	}
	return fn(blockNum)
}

func (f *fakeSteemRPC) GetActiveWitnesses() ([]string, error) {
	if f.witnessesError != nil {
		return nil, f.witnessesError
	}
	return f.witnesses, nil
}

func wsTestLogger() utils.Logger {
	logger, err := utils.NewLogger(utils.LogConfig{Level: "error"})
	if err != nil {
		panic(err)
	}
	return logger
}

// newTestService builds a WebSocketService wired to fakes. db is nil: every
// database access goes through estimateDocCount, which the test overrides.
func newTestService(rpc SteemRPC) *WebSocketService {
	ws := NewWebSocketService(rpc, nil, wsTestLogger(), 1000)
	ws.fetchInterval = 10 * time.Millisecond
	ws.stateInterval = 10 * time.Millisecond
	return ws
}

// drainBroadcast returns all messages currently buffered in ws.broadcast.
func drainBroadcast(ws *WebSocketService) []models.WebSocketMessage {
	var got []models.WebSocketMessage
	for {
		select {
		case msg := <-ws.broadcast:
			got = append(got, msg)
		default:
			return got
		}
	}
}

func stateOf(t *testing.T, msg models.WebSocketMessage) models.StateData {
	t.Helper()
	data, ok := msg.Data.(models.StateData)
	if !ok {
		t.Fatalf("message data is %T, want models.StateData", msg.Data)
	}
	return data
}

// --- state channel: estimated counts, no zero on error, lower frequency ---

func TestFetchAndBroadcastStateSkipsFrameWhenCountsNeverSucceeded(t *testing.T) {
	ws := newTestService(&fakeSteemRPC{witnesses: make([]string, 20)})
	ws.estimateDocCount = func(ctx context.Context, collection string) (int64, error) {
		return 0, errors.New("count failed")
	}

	ws.fetchAndBroadcastState()

	if got := drainBroadcast(ws); len(got) != 0 {
		t.Fatalf("broadcast %d messages with failing counts, want 0 (never broadcast zeros)", len(got))
	}
}

func TestFetchAndBroadcastStateKeepsLastCountsOnError(t *testing.T) {
	fake := &fakeSteemRPC{witnesses: make([]string, 21)}
	ws := newTestService(fake)

	countCalls := 0
	ws.estimateDocCount = func(ctx context.Context, collection string) (int64, error) {
		countCalls++
		if countCalls <= 3 { // first frame: all three succeed
			switch collection {
			case "account":
				return 100, nil
			case "comment":
				return 200, nil
			case "witness":
				return 30, nil
			}
		}
		// Subsequent frames fail (Mongo outage) except witness, which stays 0
		// to force the RPC fallback path.
		if collection == "witness" {
			return 0, nil
		}
		return 0, errors.New("count failed")
	}

	// Frame 1: everything fresh.
	ws.fetchAndBroadcastState()
	msgs := drainBroadcast(ws)
	if len(msgs) != 1 {
		t.Fatalf("frame 1 broadcast %d messages, want 1", len(msgs))
	}
	first := stateOf(t, msgs[0])
	if first.Accounts != 100 || first.Comments != 200 || first.Witnesses != 30 {
		t.Fatalf("frame 1 counts = %d/%d/%d, want 100/200/30", first.Accounts, first.Comments, first.Witnesses)
	}

	// Frame 2: account/comment counting fails; the previous values must be
	// reused (not zeros); witness count of 0 falls back to active witnesses.
	ws.fetchAndBroadcastState()
	msgs = drainBroadcast(ws)
	if len(msgs) != 1 {
		t.Fatalf("frame 2 broadcast %d messages, want 1 (keep last values, do not broadcast zeros)", len(msgs))
	}
	second := stateOf(t, msgs[0])
	if second.Accounts != 100 || second.Comments != 200 {
		t.Fatalf("frame 2 counts = %d/%d, want stale 100/200 (not zeros)", second.Accounts, second.Comments)
	}
	if second.Witnesses != 21 {
		t.Fatalf("frame 2 witnesses = %d, want 21 from RPC fallback", second.Witnesses)
	}
}

func TestStateBroadcastsSlowerThanFetchTicker(t *testing.T) {
	head := int64(1000)
	fake := &fakeSteemRPC{
		propsFn: func(call int) (*steem.DynamicGlobalProperties, error) {
			// Increasing head so a props message is emitted every tick; LIB
			// tracks head so no per-block fetching happens.
			head += 1
			return &steem.DynamicGlobalProperties{HeadBlockNumber: head, LastIrreversibleBlockNum: head}, nil
		},
		witnesses: make([]string, 20),
	}
	ws := newTestService(fake)
	ws.fetchInterval = 10 * time.Millisecond
	ws.stateInterval = 150 * time.Millisecond
	ws.estimateDocCount = func(ctx context.Context, collection string) (int64, error) {
		return 1, nil
	}

	done := make(chan struct{})
	go func() {
		ws.fetchData()
		close(done)
	}()

	props, states := 0, 0
	deadline := time.After(600 * time.Millisecond)
collect:
	for {
		select {
		case msg := <-ws.broadcast:
			if msg.Channel == "props" {
				props++
			}
			if msg.Channel == "state" {
				states++
			}
		case <-deadline:
			break collect
		}
	}
	ws.Stop()
	<-done

	// ~60 fetch ticks vs ~4 state ticks in 600ms. The exact numbers are
	// timing-sensitive; the invariant is that state is strictly throttled
	// relative to the fetch ticker, not per-tick.
	if states == 0 {
		t.Fatal("no state messages broadcast")
	}
	if props <= states*3 {
		t.Fatalf("props=%d, states=%d — state is not throttled relative to the fetch ticker", props, states)
	}
}

// --- data pump panic recovery ---

func TestSafeRunRecoversPanic(t *testing.T) {
	ws := newTestService(&fakeSteemRPC{})

	panicked := make(chan struct{})
	go func() {
		defer close(panicked)
		ws.safeRun("boom", func() {
			panic("injected panic")
		})
	}()

	select {
	case <-panicked:
	case <-time.After(2 * time.Second):
		t.Fatal("safeRun did not return after panic")
	}
}

func TestFetchDataSurvivesPanics(t *testing.T) {
	var propsCalls atomic.Int32
	fake := &fakeSteemRPC{
		propsFn: func(call int) (*steem.DynamicGlobalProperties, error) {
			n := propsCalls.Add(1)
			if n <= 2 {
				// Panic the first two ticks (both props and blocks phases
				// call this, so a couple of panics exercise recovery).
				panic("injected props panic")
			}
			head := int64(1000 + n)
			return &steem.DynamicGlobalProperties{HeadBlockNumber: head, LastIrreversibleBlockNum: 1000}, nil
		},
		witnesses: make([]string, 20),
	}
	ws := newTestService(fake)

	ws.estimateDocCount = func(ctx context.Context, collection string) (int64, error) {
		if collection == "witness" {
			return 0, nil
		}
		if collection == "account" {
			// Panic one state tick as well.
			panic("injected count panic")
		}
		return 7, nil
	}

	done := make(chan struct{})
	go func() {
		ws.fetchData()
		close(done)
	}()

	// The pump must keep producing after the injected panics; if a panic
	// escaped, the whole test process would die before this deadline.
	sawProps, sawState := false, false
	deadline := time.After(2 * time.Second)
collect:
	for {
		select {
		case msg := <-ws.broadcast:
			if msg.Channel == "props" {
				sawProps = true
			}
			if msg.Channel == "state" {
				sawState = true
			}
			if sawProps && sawState {
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	ws.Stop()
	<-done

	if !sawProps {
		t.Fatal("no props broadcast after panics — pump did not survive")
	}
}

// --- replay: async, off the hub loop ---

// dialWS connects a real WebSocket client to a test server exposing
// ws.HandleWebSocket.
func dialWS(t *testing.T, s *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(s.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	return conn
}

func readMessage(t *testing.T, conn *websocket.Conn) (map[string]interface{}, bool) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, false
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unreadable message: %v", err)
	}
	return msg, true
}

// TestHubNotBlockedBySlowReplay proves the hub loop keeps serving live
// broadcasts while a newly connected client's 10-block replay is stuck in a
// hung GetBlock RPC. Before the async replay this was a synchronous
// head-of-line block: the hub froze, the broadcast buffer filled and the
// data pump stalled.
func TestHubNotBlockedBySlowReplay(t *testing.T) {
	blockReleased := make(chan struct{})
	var blockCalls atomic.Int32
	fake := &fakeSteemRPC{
		propsFn: func(call int) (*steem.DynamicGlobalProperties, error) {
			return &steem.DynamicGlobalProperties{HeadBlockNumber: 5000, LastIrreversibleBlockNum: 5000}, nil
		},
		blockFn: func(blockNum int64) (*steem.Block, error) {
			blockCalls.Add(1)
			// First client's replay hangs here until the test releases it.
			<-blockReleased
			return &steem.Block{Number: blockNum}, nil
		},
		witnesses: make([]string, 20),
	}
	ws := newTestService(fake)
	ws.lastBlockProcessed.Store(100)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws.HandleWebSocket(w, r)
	}))
	defer server.Close()

	hubDone := make(chan struct{})
	replayDone := make(chan struct{})
	go func() {
		ws.run()
		close(hubDone)
	}()
	go func() {
		ws.runReplayWorker()
		close(replayDone)
	}()

	// Client A connects; its replay immediately hangs in GetBlock.
	connA := dialWS(t, server)

	// Give the hub a moment to register A and the worker to pick up (and
	// get stuck in) its replay.
	time.Sleep(150 * time.Millisecond)
	if blockCalls.Load() == 0 {
		t.Fatal("replay worker never called GetBlock")
	}

	// Client B connects while A's replay is stuck. B must be registered and
	// receive a live broadcast promptly — this is the assertion that the hub
	// loop is not blocked by the slow replay.
	connB := dialWS(t, server)
	time.Sleep(150 * time.Millisecond)

	propsMsg := models.WebSocketMessage{
		Type:      "props",
		Channel:   "props",
		Data:      models.PropsData{HeadBlockNumber: 6000},
		Timestamp: time.Now(),
	}
	select {
	case ws.broadcast <- propsMsg:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast channel full — hub stuck?")
	}

	msg, ok := readMessage(t, connB)
	if !ok {
		t.Fatal("client B received nothing — hub blocked by client A's replay")
	}
	if msg["type"] != "props" {
		t.Fatalf("client B first message type = %v, want props", msg["type"])
	}

	// Release A's replay and confirm A eventually gets its replayed blocks.
	close(blockReleased)
	replayed := 0
	deadline := time.Now().Add(5 * time.Second)
	for replayed < 10 && time.Now().Before(deadline) {
		_ = connA.SetReadDeadline(deadline)
		_, raw, err := connA.ReadMessage()
		if err != nil {
			t.Fatalf("client A read error after %d replayed blocks: %v", replayed, err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unreadable message: %v", err)
		}
		if m["type"] == "block" {
			replayed++
		}
	}
	if replayed < 10 {
		t.Fatalf("client A received %d replayed blocks, want 10", replayed)
	}

	// Shutdown: close conns first so readPumps' unregister is handled by the
	// still-running hub, then stop the service.
	_ = connA.Close()
	_ = connB.Close()
	time.Sleep(150 * time.Millisecond)
	ws.Stop()
	select {
	case <-hubDone:
	case <-time.After(2 * time.Second):
		t.Fatal("hub did not exit after Stop()")
	}
	select {
	case <-replayDone:
	case <-time.After(2 * time.Second):
		t.Fatal("replay worker did not exit after Stop()")
	}
}

// --- replay queue overflow: drop oldest, never block the hub ---

func TestEnqueueReplayDropsOldestWhenQueueFull(t *testing.T) {
	ws := newTestService(&fakeSteemRPC{})

	// Fill the queue to capacity without a worker draining it.
	clients := make([]*Client, replayQueueCapacity+1)
	for i := range clients {
		clients[i] = &Client{
			conn:          nil,
			send:          make(chan models.WebSocketMessage, 1),
			service:       ws,
			subscriptions: make(map[string]bool),
			sendOpen:      true,
		}
	}
	for _, c := range clients[:replayQueueCapacity] {
		ws.replayQueue <- c
	}

	// Enqueueing one more must not block and must drop the oldest entry.
	done := make(chan struct{})
	go func() {
		ws.enqueueReplay(clients[replayQueueCapacity])
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueueReplay blocked on a full queue")
	}

	if got := len(ws.replayQueue); got != replayQueueCapacity {
		t.Fatalf("queue length = %d, want %d", got, replayQueueCapacity)
	}
	// The oldest (clients[0]) must have been dropped; the queue now holds
	// clients[1..] plus the new client at the tail.
	head := <-ws.replayQueue
	if head != clients[1] {
		t.Fatal("oldest pending replay was not the dropped one")
	}
	tail := <-ws.replayQueue
	for i := 2; i < replayQueueCapacity; i++ {
		tail = <-ws.replayQueue
	}
	if tail != clients[replayQueueCapacity] {
		t.Fatal("new client's replay was not enqueued after dropping the oldest")
	}
}

// --- deliver/evict race safety (replay worker vs hub eviction) ---

func TestDeliverAfterEvictDoesNotPanic(t *testing.T) {
	ws := newTestService(&fakeSteemRPC{})
	client := &Client{
		send:          make(chan models.WebSocketMessage, 1),
		service:       ws,
		subscriptions: make(map[string]bool),
		sendOpen:      true,
	}
	client.evict()

	if client.deliver(models.WebSocketMessage{Type: "block"}) {
		t.Fatal("deliver after evict succeeded, want false")
	}

	// Hammer deliver concurrently with evict to catch send-on-closed-channel
	// races under -race.
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				client.deliver(models.WebSocketMessage{Type: "block"})
			}
		}
	}()
	go func() {
		defer wg.Done()
		client.evict()
		close(stop)
	}()
	wg.Wait()
}
