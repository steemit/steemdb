# Batch Operation Ingest API Specification

## API Overview

**Endpoint**: `POST /ingest/applied_ops`  
**Content-Type**: `application/json`  
**Purpose**: Receive batch operation data from steemd ingest plugin

---

## 1. Request Format

### 1.1 HTTP Request

```http
POST /ingest/applied_ops HTTP/1.1
Host: localhost:8080
Content-Type: application/json
User-Agent: steemd-ingest-plugin/1.0
Connection: keep-alive

[operation1, operation2, ..., operationN]
```

### 1.2 Request Body Format

**Request body is a JSON array**, where each element is an operation object (format is fully compatible with single endpoint `/ingest/applied_op`).

```json
[
  {
    "block": {
      "num": 123456,
      "id": "0001e240...",
      "timestamp": "2023-01-01T00:00:00"
    },
    "transaction": {
      "id": "abcd1234...",
      "index": 2
    },
    "operation": {
      "index": 0,
      "type": "transfer",
      "value": {
        "from": "alice",
        "to": "bob",
        "amount": "1.000 STEEM",
        "memo": "test"
      }
    },
    "virtual": false
  },
  {
    "block": {
      "num": 123456,
      "id": "0001e240...",
      "timestamp": "2023-01-01T00:00:00"
    },
    "transaction": {
      "id": null,
      "index": -1
    },
    "operation": {
      "index": 0,
      "type": "author_reward",
      "value": {
        "author": "alice",
        "permlink": "test-post",
        "sbd_payout": "0.000 SBD",
        "steem_payout": "1.000 STEEM"
      }
    },
    "virtual": true
  }
]
```

### 1.3 Operation Object Format (Same as Single Endpoint)

Each operation object contains the following fields:

| Field | Type | Required | Description |
|--------|------|-----------|-------------|
| `block` | object | ✅ | Block information |
| `block.num` | number | ✅ | Block number (>= 1) |
| `block.id` | string | ✅ | Block ID (hex string) |
| `block.timestamp` | string | ✅ | Block timestamp (ISO-8601 UTC) |
| `transaction` | object | ✅ | Transaction information |
| `transaction.id` | string \| null | ✅ | Transaction ID (hex string), null for virtual op |
| `transaction.index` | number | ✅ | Transaction index in block (starting from 0), -1 for virtual op |
| `operation` | object | ✅ | Operation information |
| `operation.index` | number | ✅ | Operation index in transaction (starting from 0) |
| `operation.type` | string | ✅ | Operation type (snake_case, e.g., `transfer`, `vote`, `comment`) |
| `operation.value` | object | ✅ | Operation data (JSON object, fields match steemd C++ struct) |
| `virtual` | boolean | ✅ | Whether it is a virtual operation |
| `block_only` | boolean | ⭕ | Whether it's a block-only record (block without operations), optional field |

**Note**:
- `block_only` field only appears in block-only records (value is `true`)
- Block-only records ensure that all blocks (including those without operations) are recorded
- In block-only records, `transaction.id` is `null`, `transaction.index` is `-1`, and `operation` field is empty

### 1.4 Block-Only Record Format

For blocks without operations, the plugin sends block-only records:

```json
{
  "block": {
    "num": 123456,
    "id": "0001e240...",
    "timestamp": "2023-01-01T00:00:00"
  },
  "transaction": {
    "id": null,
    "index": -1
  },
  "operation": {
    "index": -1,
    "type": "",
    "value": null
  },
  "virtual": false,
  "block_only": true
}
```

---

## 2. Response Format

### 2.1 Success Response

**HTTP 200 OK**

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "ok",
  "processed": 100,
  "errors": []
}
```

**Response Field Description**:

| Field | Type | Description |
|--------|------|-------------|
| `status` | string | Status: `"ok"` or `"partial"` |
| `processed` | number | Number of successfully processed operations |
| `errors` | array | Error list (if there are partial failures) |

### 2.2 Partial Success Response

When some operations fail to process, return `"partial"` status:

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "partial",
  "processed": 95,
  "errors": [
    {
      "index": 5,
      "error": "invalid operation format"
    },
    {
      "index": 42,
      "error": "duplicate operation"
    }
  ]
}
```

**Error Object Format**:

| Field | Type | Description |
|--------|------|-------------|
| `index` | number | Index of the failed operation in the request array (starting from 0) |
| `error` | string | Error description |

### 2.3 Error Response

#### 2.3.1 Request Format Error (400 Bad Request)

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": "invalid request format",
  "message": "request body must be a JSON array"
}
```

#### 2.3.2 Server Error (500 Internal Server Error)

**Important**: The server implements ACK mechanism, only returns 200 after data is successfully written to MongoDB. If write fails, returns 500 error:

```http
HTTP/1.1 500 Internal Server Error
Content-Type: text/plain; charset=utf-8

failed to flush to database: <error details>
```

**Client Behavior**:
- When receiving 500 error, the client automatically puts the batch into retry queue
- Retry mechanism: retry after 3 seconds, up to 5 retry attempts
- If all 5 retries fail, the batch is dropped and error is logged

---

## 3. Processing Logic Requirements

### 3.1 Batch Processing

1. **Receive Array**: The interface must receive a JSON array as request body
2. **Process Individually**: Process each operation in array order
3. **Idempotency**: Each operation's processing must be idempotent (repeated processing won't cause side effects)
4. **Synchronous Write (ACK Mechanism)**: 
   - **Required**: Data must be synchronously written to MongoDB before returning 200
   - **Failure Handling**: If write fails, return 500 error, client will automatically retry
   - **Block-only Processing**: Block-only records also need to be synchronously written to blocks collection

### 3.1.1 Block-Only Record Processing

- **Detection**: If the operation object contains `"block_only": true`, only write block information, do not write operation
- **Synchronous Write**: Block-only records also need to be synchronously written to MongoDB before returning 200
- **Completeness**: Ensure all blocks (including those without operations) are recorded

### 3.2 Error Handling

1. **Partial Failure**: If some operations fail, continue processing other operations
2. **Error Logging**: Record the index and error reason for failed operations
3. **Response Return**: Return processing results and error list in response

### 3.3 Performance Requirements

1. **Throughput**: Must support processing thousands of operations per second
2. **Latency**: Batch processing latency should be as low as possible (suggested < 100ms)
3. **Batch Size**: Support batch sizes from 1 to 1000+ operations

---

## 4. Compatibility with Single Endpoint

### 4.1 Endpoint Comparison

| Feature | `/ingest/applied_op` | `/ingest/applied_ops` |
|---------|---------------------|----------------------|
| Request Body | Single operation object | Operation object array |
| Content-Type | `application/json` | `application/json` |
| Processing | Individual processing | Batch processing |
| Performance | Lower (one operation per request) | Higher (multiple operations per request) |

### 4.2 Data Format Consistency

**Important**: The operation object formats received by both endpoints must be completely consistent to ensure:
- Same field structure
- Same data types
- Same validation rules

---

## 5. Implementation Suggestions

### 5.1 Go Implementation Example

```go
// HandleAppliedOps handles POST /ingest/applied_ops
func (h *IngestHandler) HandleAppliedOps(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var operations []OperationJSON
    if err := json.NewDecoder(r.Body).Decode(&operations); err != nil {
        http.Error(w, fmt.Sprintf("failed to decode JSON array: %v", err), http.StatusBadRequest)
        return
    }

    if len(operations) == 0 {
        http.Error(w, "empty operations array", http.StatusBadRequest)
        return
    }

    // Process batch
    processed := 0
    errors := []BatchError{}
    
    for i, opJSON := range operations {
        op, err := h.convertToOperation(&opJSON)
        if err != nil {
            errors = append(errors, BatchError{
                Index: i,
                Error: err.Error(),
            })
            continue
        }

        // Parse block timestamp
        blockTimestamp, err := time.Parse(time.RFC3339, opJSON.Block.Timestamp)
        if err != nil {
            blockTimestamp, err = time.Parse("2006-01-02T15:04:05", opJSON.Block.Timestamp)
            if err != nil {
                blockTimestamp = time.Now()
            }
        }

        // Store block info
        h.batcher.AddBlockInfo(opJSON.Block.Num, opJSON.Block.ID, blockTimestamp)

        // Add operation to batcher
        if err := h.batcher.AddOperation(op); err != nil {
            errors = append(errors, BatchError{
                Index: i,
                Error: err.Error(),
            })
            continue
        }

        processed++
    }

    // Build response
    status := "ok"
    if len(errors) > 0 {
        status = "partial"
    }

    response := BatchResponse{
        Status:    status,
        Processed: processed,
        Errors:    errors,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

type BatchResponse struct {
    Status    string       `json:"status"`
    Processed int          `json:"processed"`
    Errors    []BatchError `json:"errors"`
}

type BatchError struct {
    Index int    `json:"index"`
    Error string `json:"error"`
}
```

### 5.2 Route Registration

```go
// Add batch endpoint during route registration
router.HandleFunc("/ingest/applied_ops", handler.HandleAppliedOps).Methods("POST")
```

---

## 6. Testing Requirements

### 6.1 Unit Tests

1. **Empty Array**: Test handling of empty array
2. **Single Operation**: Test case where array has only one operation
3. **Multiple Operations**: Test batch processing of multiple operations
4. **Mixed Operations**: Test mixing of real operations and virtual operations
5. **Error Handling**: Test case where some operations fail
6. **Format Validation**: Test handling of invalid JSON format

### 6.2 Integration Tests

1. **Performance Test**: Test batch processing throughput
2. **Stress Test**: Test large number of concurrent requests
3. **Compatibility Test**: Ensure data format consistency with single endpoint

---

## 7. Monitoring Metrics

It is recommended to add the following monitoring metrics:

1. **Batch Request Count**: `steemdb_sync_batch_requests_total`
2. **Batch Operation Count**: `steemdb_sync_batch_operations_total`
3. **Batch Processing Latency**: `steemdb_sync_batch_duration_seconds`
4. **Batch Size Distribution**: `steemdb_sync_batch_size`
5. **Batch Error Count**: `steemdb_sync_batch_errors_total`

---

## 8. Considerations

1. **Idempotency**: Must guarantee idempotency of operations, repeated processing won't produce duplicate data
2. **Ordering**: Batch processing should try to maintain operation order (if possible)
3. **Error Recovery**: When some operations fail, detailed error information should be logged for subsequent fixes
4. **Performance Optimization**: Use batch writes to MongoDB for better performance
5. **Connection Reuse**: Client uses keep-alive connections to reduce connection overhead
6. **ACK Mechanism**: Server must ensure data is successfully written to MongoDB before returning 200, return 500 on failure
7. **Block-only Records**: Must handle records with `block_only: true` to ensure all blocks are recorded
8. **Client Retry**: Client will automatically retry failed requests (3-second delay, up to 5 times), no manual handling required

---

## 9. Example Requests

### 9.1 cURL Example

```bash
curl -X POST http://localhost:8080/ingest/applied_ops \
  -H "Content-Type: application/json" \
  -d '[
    {
      "block": {
        "num": 123456,
        "id": "0001e240...",
        "timestamp": "2023-01-01T00:00:00"
      },
      "transaction": {
        "id": "abcd1234...",
        "index": 0
      },
      "operation": {
        "index": 0,
        "type": "transfer",
        "value": {
          "from": "alice",
          "to": "bob",
          "amount": "1.000 STEEM"
        }
      },
      "virtual": false
    }
  ]'
```

### 9.2 Batch Request Example (100 operations)

```bash
# Generate JSON array containing 100 operations
# Send to batch endpoint
curl -X POST http://localhost:8080/ingest/applied_ops \
  -H "Content-Type: application/json" \
  --data-binary @batch_100_ops.json
```

---

## 10. Version Compatibility

- **Current Version**: v1.0
- **Backward Compatible**: Fully compatible with `/ingest/applied_op` endpoint
- **Future Extensions**: Could consider adding compression support (gzip) to improve transmission efficiency

---

## 11. Reference Documents

- Single endpoint documentation: Refer to `HandleAppliedOp` implementation
- Operation format definition: Refer to "Operation JSON Protocol Definition" in `steemdb-sync/.cursor/plans/steemdb-sync.md`
- Existing implementation: `steemdb-sync/internal/pipeline/ingest_handler.go`
