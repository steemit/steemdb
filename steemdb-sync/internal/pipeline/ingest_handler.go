package pipeline

import (
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/model"
)

// IngestHandler handles HTTP requests for operation ingestion
type IngestHandler struct {
	batcher *Batcher
}

// NewIngestHandler creates a new ingest handler
func NewIngestHandler(batcher *Batcher) *IngestHandler {
	return &IngestHandler{
		batcher: batcher,
	}
}

// OperationJSON represents the JSON format from plugin
type OperationJSON struct {
	Block      BlockJSON      `json:"block"`
	Transaction TransactionJSON `json:"transaction"`
	Operation  OperationDataJSON `json:"operation"`
	Virtual    bool           `json:"virtual"`
}

type BlockJSON struct {
	Num       uint32 `json:"num"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
}

type TransactionJSON struct {
	ID    *string `json:"id"`
	Index int32   `json:"index"`
}

type OperationDataJSON struct {
	Index int32                  `json:"index"`
	Type  string                 `json:"type"`
	Value map[string]interface{} `json:"value"`
}

// HandleAppliedOp handles POST /ingest/applied_op
func (h *IngestHandler) HandleAppliedOp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var opJSON OperationJSON
	if err := json.NewDecoder(r.Body).Decode(&opJSON); err != nil {
		http.Error(w, errors.Wrap(err, "failed to decode JSON").Error(), http.StatusBadRequest)
		return
	}

	// Convert to internal Operation model
	op, err := h.convertToOperation(&opJSON)
	if err != nil {
		http.Error(w, errors.Wrap(err, "failed to convert operation").Error(), http.StatusBadRequest)
		return
	}

	// Add to batcher (non-blocking enqueue)
	if err := h.batcher.AddOperation(op); err != nil {
		http.Error(w, errors.Wrap(err, "failed to enqueue operation").Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// convertToOperation converts OperationJSON to model.Operation
func (h *IngestHandler) convertToOperation(opJSON *OperationJSON) (*model.Operation, error) {
	// Generate operation ID
	trxIndex := opJSON.Transaction.Index
	opIndex := opJSON.Operation.Index
	opID := model.OperationID(opJSON.Block.Num, trxIndex, opIndex)

	// Get transaction ID
	trxID := ""
	if opJSON.Transaction.ID != nil {
		trxID = *opJSON.Transaction.ID
	}

	op := &model.Operation{
		ID:       opID,
		BlockNum: opJSON.Block.Num,
		TrxID:    trxID,
		TrxIndex: trxIndex,
		OpIndex:  opIndex,
		OpType:   opJSON.Operation.Type,
		OpValue:  opJSON.Operation.Value,
		Virtual:  opJSON.Virtual,
		Source:   "plugin",
	}

	return op, nil
}
