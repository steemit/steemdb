package handlers

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestAccountCreateHandler_CreatesStubAndMarksCreator(t *testing.T) {
	m := NewMongoInserter(nil)
	m.BeginBatch(10)
	defer m.EndBatch()

	h := NewAccountCreateHandler(m)
	for _, opType := range []string{"account_create", "account_create_with_delegation", "create_claimed_account"} {
		op := makeOp(opType, 100, map[string]interface{}{
			"creator":          "alice",
			"new_account_name": "bob",
		})
		if err := h.Handle(context.Background(), op, time.Now()); err != nil {
			t.Fatalf("%s Handle: %v", opType, err)
		}
	}

	if _, ok := m.createdAccounts["bob"]; !ok {
		t.Fatal("creation op must queue the new account for stub creation")
	}
	if _, ok := m.dirtyAccounts["alice"]; !ok {
		t.Fatal("creation op must dirty-mark the creator (fee/delegation changed balances)")
	}
}

func TestAccountCreateHandler_SelfCreateSkipsCreatorDirty(t *testing.T) {
	m := NewMongoInserter(nil)
	m.BeginBatch(10)
	defer m.EndBatch()

	h := NewAccountCreateHandler(m)
	op := makeOp("account_create", 100, map[string]interface{}{
		"creator":          "bob",
		"new_account_name": "bob",
	})
	if err := h.Handle(context.Background(), op, time.Now()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(m.dirtyAccounts) != 0 {
		t.Fatalf("self-create must not dirty-mark creator, got %v", m.dirtyAccounts)
	}
}

func TestPowHandler_QueuesWorkerCreate(t *testing.T) {
	m := NewMongoInserter(nil)
	m.BeginBatch(10)
	defer m.EndBatch()

	h := NewPowHandler(m)
	// pow format: top-level worker_account
	op := makeOp("pow", 100, map[string]interface{}{
		"worker_account": "minerone",
		"work":           map[string]interface{}{"worker": "STM..."},
	})
	if err := h.Handle(context.Background(), op, time.Now()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if _, ok := m.createdAccounts["minerone"]; !ok {
		t.Fatal("pow must queue the worker for stub creation (mining created the account)")
	}
}

func TestFollowHandler_NoAccountMarks(t *testing.T) {
	m := NewMongoInserter(nil)
	m.BeginBatch(10)
	defer m.EndBatch()

	h := NewCustomJSONHandler(m)
	op := makeOp("custom_json", 100, map[string]interface{}{
		"id":   "follow",
		"json": `["follow", {"follower": "alice", "following": "ubmit.html", "what": ["blog"]}]`,
	})
	if err := h.Handle(context.Background(), op, time.Now()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(m.dirtyAccounts) != 0 || len(m.createdAccounts) != 0 {
		t.Fatalf("follow must not mark accounts (phantom-stub source), got dirty=%v created=%v",
			m.dirtyAccounts, m.createdAccounts)
	}
}

// TestFlushDirtyNeverUpsertsButCreateDoes pins the account-doc-creation rule
// at the flush layer: dirty marks update existing documents only
// (upsert=false), creation marks may insert the stub (upsert=true).
func TestFlushDirtyNeverUpsertsButCreateDoes(t *testing.T) {
	type mark struct {
		id     interface{}
		upsert bool
	}
	var got []mark
	m := NewMongoInserter(nil)
	m.SetBulkWriteHook(func(ctx context.Context, coll string, models []mongo.WriteModel) error {
		for _, model := range models {
			if um, ok := model.(*mongo.UpdateOneModel); ok {
				filter, _ := um.Filter.(bson.M)
				got = append(got, mark{id: filter["_id"], upsert: um.Upsert != nil && *um.Upsert})
			}
		}
		return nil
	})
	m.BeginBatch(10)
	defer m.EndBatch()

	if err := m.QueueAccountDirty(nil, "existing-acct"); err != nil {
		t.Fatalf("QueueAccountDirty: %v", err)
	}
	if err := m.QueueAccountCreate(nil, "brand-new-acct"); err != nil {
		t.Fatalf("QueueAccountCreate: %v", err)
	}
	if err := m.FlushAll(context.Background()); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}

	upsertByID := map[interface{}]bool{}
	for _, g := range got {
		upsertByID[g.id] = g.upsert
	}
	if up, ok := upsertByID["existing-acct"]; !ok || up {
		t.Errorf("dirty mark upsert = %v (present=%v), want false", up, ok)
	}
	if up, ok := upsertByID["brand-new-acct"]; !ok || !up {
		t.Errorf("create mark upsert = %v (present=%v), want true", up, ok)
	}
}
