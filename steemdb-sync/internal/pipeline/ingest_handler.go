package pipeline

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/steemit/steemdb-sync/internal/model"
)

// IngestHandler handles HTTP requests for operation ingestion
type IngestHandler struct {
	batcher      *Batcher
	requestCount uint64 // Atomic counter for total requests received
	lastLogBlock uint32 // Last block number we logged (for periodic logging)
}

// NewIngestHandler creates a new ingest handler
func NewIngestHandler(batcher *Batcher) *IngestHandler {
	return &IngestHandler{
		batcher: batcher,
	}
}

// OperationJSON represents the JSON format from plugin
type OperationJSON struct {
	Block       BlockJSON         `json:"block"`
	Transaction TransactionJSON   `json:"transaction"`
	Operation   OperationDataJSON `json:"operation"`
	Virtual     bool              `json:"virtual"`
	BlockOnly   bool              `json:"block_only,omitempty"` // Indicates this is a block-only record (no operations)
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

	// Increment request counter
	requestNum := atomic.AddUint64(&h.requestCount, 1)

	var opJSON OperationJSON
	if err := json.NewDecoder(r.Body).Decode(&opJSON); err != nil {
		http.Error(w, errors.Wrap(err, "failed to decode JSON").Error(), http.StatusBadRequest)
		return
	}

	// Parse block timestamp
	// Try RFC3339 first, then try the format used in jsonl file (2006-01-02T15:04:05)
	blockTimestamp, err := time.Parse(time.RFC3339, opJSON.Block.Timestamp)
	if err != nil {
		// Try alternative format (without timezone, e.g., "2016-03-24T16:05:00")
		blockTimestamp, err = time.Parse("2006-01-02T15:04:05", opJSON.Block.Timestamp)
		if err != nil {
			// Fallback to current time if both formats fail
			blockTimestamp = time.Now()
		}
	}

	// Store block info
	h.batcher.AddBlockInfo(opJSON.Block.Num, opJSON.Block.ID, blockTimestamp)

	// Handle block-only records (blocks without operations)
	if opJSON.BlockOnly {
		// Block-only records: synchronously flush blocks to MongoDB (ACK mechanism)
		if err := h.batcher.FlushOperationsAndBlocks(r.Context(), nil); err != nil {
			log.Printf("[IngestHandler] Failed to flush block-only record to MongoDB: %v", err)
			http.Error(w, errors.Wrap(err, "failed to flush block to database").Error(), http.StatusInternalServerError)
			return
		}
		// Log first few requests and every 1000th request for debugging
		if requestNum <= 3 || requestNum%1000 == 0 {
			log.Printf("[IngestHandler] Received block-only request #%d: block=%d (no operations)",
				requestNum, opJSON.Block.Num)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	// Log first few requests and every 1000th request for debugging
	if requestNum <= 3 || requestNum%1000 == 0 {
		log.Printf("[IngestHandler] Received request #%d: block=%d, op_type=%s, trx_index=%d, op_index=%d",
			requestNum, opJSON.Block.Num, opJSON.Operation.Type, opJSON.Transaction.Index, opJSON.Operation.Index)
	}

	// Convert to internal Operation model
	op, err := h.convertToOperation(&opJSON)
	if err != nil {
		http.Error(w, errors.Wrap(err, "failed to convert operation").Error(), http.StatusBadRequest)
		return
	}

	// Synchronously flush operation and blocks to MongoDB (ACK mechanism)
	// Only return 200 if data is successfully written
	if err := h.batcher.FlushOperationsAndBlocks(r.Context(), []*model.Operation{op}); err != nil {
		log.Printf("[IngestHandler] Failed to flush operation to MongoDB: %v", err)
		http.Error(w, errors.Wrap(err, "failed to flush to database").Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// BatchResponse represents the response for batch operations
type BatchResponse struct {
	Status    string       `json:"status"`
	Processed int          `json:"processed"`
	Errors    []BatchError `json:"errors"`
}

// BatchError represents an error for a specific operation in a batch
type BatchError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// HandleAppliedOps handles POST /ingest/applied_ops (batch operations)
func (h *IngestHandler) HandleAppliedOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Increment request counter
	requestNum := atomic.AddUint64(&h.requestCount, 1)

	var operations []OperationJSON
	if err := json.NewDecoder(r.Body).Decode(&operations); err != nil {
		http.Error(w, errors.Wrap(err, "failed to decode JSON array").Error(), http.StatusBadRequest)
		return
	}

	if len(operations) == 0 {
		http.Error(w, "empty operations array", http.StatusBadRequest)
		return
	}

	// Log batch requests for debugging
	if requestNum <= 3 || requestNum%100 == 0 {
		log.Printf("[IngestHandler] Received batch request #%d: %d operations", requestNum, len(operations))
	}

	// Process batch and collect operations
	processed := 0
	batchErrors := []BatchError{}
	opsToFlush := make([]*model.Operation, 0, len(operations))

	for i, opJSON := range operations {
		// Parse block timestamp
		// Try RFC3339 first, then try the format used in jsonl file (2006-01-02T15:04:05)
		blockTimestamp, err := time.Parse(time.RFC3339, opJSON.Block.Timestamp)
		if err != nil {
			// Try alternative format (without timezone, e.g., "2016-03-24T16:05:00")
			blockTimestamp, err = time.Parse("2006-01-02T15:04:05", opJSON.Block.Timestamp)
			if err != nil {
				// Fallback to current time if both formats fail
				blockTimestamp = time.Now()
			}
		}

		// Store block info
		h.batcher.AddBlockInfo(opJSON.Block.Num, opJSON.Block.ID, blockTimestamp)

		// Handle block-only records (blocks without operations)
		if opJSON.BlockOnly {
			// Block-only records: only store block info, no operation to add
			processed++
			continue
		}

		// Convert to internal Operation model
		op, err := h.convertToOperation(&opJSON)
		if err != nil {
			batchErrors = append(batchErrors, BatchError{
				Index: i,
				Error: errors.Wrap(err, "failed to convert operation").Error(),
			})
			continue
		}

		// Collect operation for synchronous flush (ACK mechanism)
		opsToFlush = append(opsToFlush, op)
		processed++
	}

	// Synchronously flush operations and blocks to MongoDB (ACK mechanism)
	// Only return 200 if data is successfully written
	if len(opsToFlush) > 0 {
		if err := h.batcher.FlushOperationsAndBlocks(r.Context(), opsToFlush); err != nil {
			log.Printf("[IngestHandler] Failed to flush batch to MongoDB: %v", err)
			http.Error(w, errors.Wrap(err, "failed to flush to database").Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Even if no operations, flush any unwritten blocks (block-only blocks)
		if err := h.batcher.FlushOperationsAndBlocks(r.Context(), nil); err != nil {
			log.Printf("[IngestHandler] Failed to flush blocks to MongoDB: %v", err)
			http.Error(w, errors.Wrap(err, "failed to flush blocks to database").Error(), http.StatusInternalServerError)
			return
		}
	}

	// Build response (only if flush succeeded)
	status := "ok"
	if len(batchErrors) > 0 {
		status = "partial"
	}

	response := BatchResponse{
		Status:    status,
		Processed: processed,
		Errors:    batchErrors,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[IngestHandler] Failed to encode batch response: %v", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
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
