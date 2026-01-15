# SteemDB Sync

Steem blockchain data synchronization service for SteemDB explorer.

## Architecture

This service implements a three-phase synchronization architecture:

1. **Cold Start**: Uses steemd replay + plugin for fast historical data ingestion
2. **Live Sync**: Uses RPC to continuously sync new blocks
3. **Repair**: Uses RPC to fix missing blocks

## Components

- `cmd/cold_ingest`: HTTP service that receives operations from steemd plugin during cold start
- `cmd/live_sync`: RPC-based live block synchronization service
- `cmd/repair`: Tool for scanning and repairing missing blocks

## Steemd Ingest Plugin

The cold start phase uses a custom C++ plugin (`ingest_plugin`) for the `steemd` node. This plugin:

- Listens to `applied_operation` and `applied_block` signals during blockchain replay
- Serializes operations to JSON format
- Sends operation data to the Go ingest service via HTTP POST (single or batch)
- **Sends block-only records** for blocks without operations (ensures all blocks are recorded)
- **Automatic retry mechanism**: Retries failed HTTP requests with 3-second delay (up to 5 attempts)
- **Blocking queue**: Blocks replay when queue is full to protect the API endpoint
- Runs asynchronously to avoid blocking steemd replay

### Plugin Location

The ingest plugin is located in the `steem` repository:
- Path: `steem/libraries/plugins/ingest/`
- Header: `include/steem/plugins/ingest/ingest_plugin.hpp`
- Implementation: `ingest_plugin.cpp`
- Build config: `CMakeLists.txt`

### Plugin Configuration

The plugin accepts the following command-line options:

```bash
--ingest-endpoint <url>        # HTTP endpoint for ingest service (default: http://localhost:8080/ingest/applied_op)
--ingest-http-timeout <ms>      # HTTP request timeout in milliseconds (default: 5000)
--ingest-queue-size <size>      # Maximum queue size for pending operations (default: 100000)
--ingest-batch-size <size>       # Number of operations to batch before sending (default: 100, 1 = disable batching)
--ingest-batch-timeout <ms>      # Max milliseconds to wait before sending a batch (default: 100)
--ingest-dry-run                 # Dry run mode: write to file instead of sending HTTP (default: false)
```

### Plugin Features

#### Batch Sending
- **Automatic batching**: Operations are collected and sent in batches to improve throughput
- **Batch endpoint**: When `batch_size > 1`, uses `/ingest/applied_ops` endpoint
- **Single endpoint**: When `batch_size = 1`, uses `/ingest/applied_op` endpoint (backward compatible)
- **Configurable batch size**: Adjust `--ingest-batch-size` to balance latency and throughput

#### Block-Only Records
- **Automatic detection**: Plugin detects blocks without operations
- **Block-only JSON**: Sends special records with `"block_only": true` marker
- **Complete block coverage**: Ensures all blocks (including empty ones) are recorded in MongoDB

#### Retry Mechanism
- **Automatic retry**: Failed HTTP requests are automatically retried
- **Retry delay**: 3 seconds between retry attempts
- **Max retries**: Up to 5 retry attempts before dropping
- **Retry queue**: Failed batches are queued separately and retried asynchronously

#### Reliability Features
- **Blocking queue**: When queue is full, replay blocks until space is available (protects API endpoint)
- **Connection pooling**: Reuses TCP connections for better performance
- **ACK mechanism**: Service only returns 200 after data is successfully written to MongoDB
- **Error handling**: Comprehensive error logging and graceful degradation

### Building steemd with Ingest Plugin

To build `steemd` with the ingest plugin enabled:

```bash
cd steem
docker build -f deploy/Dockerfile.ubuntu24.04 -t steemd:ingest .
```

Or using CMake directly:

```bash
cd steem
mkdir build && cd build
cmake .. -DCMAKE_BUILD_TYPE=Release
make -j$(nproc)
```

### Running steemd with Ingest Plugin

1. Start the Go ingest service first:
   ```bash
   ../bin/cold_ingest -config configs/config.yaml
   ```

2. Run steemd replay with the ingest plugin:
   ```bash
   steemd --replay-blockchain \
          --plugin ingest \
          --ingest-endpoint http://localhost:8080/ingest/applied_ops \
          --ingest-http-timeout 5000 \
          --ingest-queue-size 100000 \
          --ingest-batch-size 100 \
          --ingest-batch-timeout 100
   ```

The plugin will automatically send all operations (and block-only records) to the ingest service during replay.

### Dry Run Mode

For testing without a running ingest service, use dry run mode:

```bash
steemd --replay-blockchain \
       --plugin ingest \
       --ingest-dry-run \
       --data-dir /var/steem
```

Operations will be written to `{data-dir}/ingest/ingest_YYYYMMDD_HHMMSS_mmm.jsonl` files in JSON Lines format.

## Configuration

See `configs/config.yaml` for configuration examples.

## Development

### Building

All binaries are built to `steemdb/bin/` directory:

```bash
# Build all components
mkdir -p ../bin
go build -o ../bin/cold_ingest ./cmd/cold_ingest
go build -o ../bin/live_sync ./cmd/live_sync
go build -o ../bin/repair ./cmd/repair

# Or build all at once
go build -o ../bin/cold_ingest ./cmd/cold_ingest && \
go build -o ../bin/live_sync ./cmd/live_sync && \
go build -o ../bin/repair ./cmd/repair
```

### Running

```bash
# Run cold ingest (from steemdb-sync directory)
../bin/cold_ingest -config configs/config.yaml

# Run live sync
../bin/live_sync -config configs/config.yaml

# Run repair tool (scan and repair missing blocks)
../bin/repair -config configs/config.yaml

# Repair tool with options
../bin/repair -config configs/config.yaml -start 1000 -end 2000  # Repair specific range
../bin/repair -config configs/config.yaml -dry-run               # Scan only, don't repair
```

## Metrics

The service exposes Prometheus metrics at `/metrics` endpoint:

- **Ingest metrics**: `steemdb_sync_ingest_ops_total`, `steemdb_sync_ingest_ops_per_second`
- **MongoDB metrics**: `steemdb_sync_mongo_write_duration_seconds`, `steemdb_sync_mongo_write_total`
- **RPC metrics**: `steemdb_sync_rpc_latency_seconds`, `steemdb_sync_rpc_total`
- **Batch metrics**: `steemdb_sync_batch_size`, `steemdb_sync_batch_flush_duration_seconds`
- **Queue metrics**: `steemdb_sync_queue_size`
- **Block metrics**: `steemdb_sync_current_block`

For `cold_ingest`, metrics are available at the same HTTP server (default: `:8080/metrics`).
For `live_sync`, metrics are available on port `:9091/metrics`.

## Dependencies

### Go Dependencies

- **steemgosdk**: For Steem RPC communication
- **steemutil**: For Steem protocol structures (operations, blocks, etc.)
- **prometheus/client_golang**: For Prometheus metrics

### C++ Plugin Dependencies (for steemd)

The ingest plugin requires:
- **Boost.Beast**: For HTTP client functionality
- **Boost.Signals2**: For signal/slot connections
- **FC (Fungible Core)**: For serialization and variants
- **steem_chain**: Steem chain library
- **steem_protocol**: Steem protocol library

## Development Status

### Completed ✅

- [x] Go ingest service (`cmd/cold_ingest`)
- [x] Configuration module (`internal/config`)
- [x] Data models (`internal/model`)
- [x] MongoDB access layer (`internal/mongo`)
- [x] Pipeline processing (`internal/pipeline`)
- [x] **Steemd ingest plugin** (`steem/libraries/plugins/ingest/`)
  - [x] Operation serialization and HTTP sending
  - [x] Batch sending with configurable batch size
  - [x] Block-only records for empty blocks
  - [x] Automatic retry mechanism (3s delay, max 5 attempts)
  - [x] Blocking queue to protect API endpoint
  - [x] Connection pooling for performance
  - [x] Dry run mode for testing
- [x] **Service reliability features**
  - [x] ACK mechanism (only return 200 after successful MongoDB write)
  - [x] Synchronous flush for guaranteed data persistence
  - [x] Block-only block handling
- [x] Unit tests for core modules
- [x] Prometheus metrics integration

### In Progress 🚧

- [ ] Live sync service (`cmd/live_sync`)
- [ ] Repair tool (`cmd/repair`)
- [ ] Integration tests
- [ ] Performance testing

### Planned 📋

- [ ] End-to-end testing with steemd replay
- [ ] Performance optimization
- [ ] Documentation and examples