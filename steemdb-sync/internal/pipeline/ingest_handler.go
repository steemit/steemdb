package pipeline

import (
	"encoding/json"
	"io"
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

	// Read body into memory so we can try multiple decode strategies
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, errors.Wrap(err, "failed to read request body").Error(), http.StatusBadRequest)
		return
	}

	// Try to decode as array first
	var operations []OperationJSON
	if err := json.Unmarshal(bodyBytes, &operations); err != nil {
		// If array decode fails, try as single object
		var singleOp OperationJSON
		if err2 := json.Unmarshal(bodyBytes, &singleOp); err2 != nil {
			http.Error(w, errors.Wrap(err, "failed to decode JSON array or object").Error(), http.StatusBadRequest)
			return
		}
		// Successfully decoded as single object, wrap in array
		operations = []OperationJSON{singleOp}
	}

	if len(operations) == 0 {
		http.Error(w, "empty operations array", http.StatusBadRequest)
		return
	}

	// Debug log for batches containing blocks < 1000
	hasLowBlocks := false
	for _, opJSON := range operations {
		if opJSON.Block.Num < 1000 {
			hasLowBlocks = true
			break
		}
	}
	if hasLowBlocks {
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

		// Debug log for blocks < 1000
		if opJSON.Block.Num < 1000 {
			log.Printf("[IngestHandler] [DEBUG] Batch request #%d: Processing op[%d]: block=%d, op_type=%s, trx_index=%d, op_index=%d, block_only=%v",
				requestNum, i, opJSON.Block.Num, opJSON.Operation.Type, opJSON.Transaction.Index, opJSON.Operation.Index, opJSON.BlockOnly)
		}

		// Handle block-only records (blocks without operations)
		if opJSON.BlockOnly {
			// Block-only records: only store block info, no operation to add
			processed++
			continue
		}

		// Convert to internal Operation model
		op, err := h.convertToOperation(&opJSON)
		if err != nil {
			if opJSON.Block.Num < 1000 {
			}
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
	if hasLowBlocks {
	}
	if len(opsToFlush) > 0 {
		if err := h.batcher.FlushOperationsAndBlocks(r.Context(), opsToFlush); err != nil {
			log.Printf("[IngestHandler] Failed to flush batch to MongoDB: %v", err)
			if hasLowBlocks {
			}
			http.Error(w, errors.Wrap(err, "failed to flush to database").Error(), http.StatusInternalServerError)
			return
		}
		if hasLowBlocks {
		}
	} else {
		// Even if no operations, flush any unwritten blocks (block-only blocks)
		if hasLowBlocks {
		}
		if err := h.batcher.FlushOperationsAndBlocks(r.Context(), nil); err != nil {
			log.Printf("[IngestHandler] Failed to flush blocks to MongoDB: %v", err)
			if hasLowBlocks {
			}
			http.Error(w, errors.Wrap(err, "failed to flush blocks to database").Error(), http.StatusInternalServerError)
			return
		}
		if hasLowBlocks {
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
