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

- Listens to `applied_operation` signals during blockchain replay
- Serializes operations to JSON format
- Sends operation data to the Go ingest service via HTTP POST
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
```

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
   ./cold_ingest -config configs/config.yaml
   ```

2. Run steemd replay with the ingest plugin:
   ```bash
   steemd --replay-blockchain \
          --plugin ingest \
          --ingest-endpoint http://localhost:8080/ingest/applied_op \
          --ingest-http-timeout 5000 \
          --ingest-queue-size 100000
   ```

The plugin will automatically send all operations to the ingest service during replay.

## Configuration

See `configs/config.yaml` for configuration examples.

## Development

```bash
# Build all components
go build ./cmd/...

# Run cold ingest
./cold_ingest -config configs/config.yaml

# Run live sync
./live_sync -config configs/config.yaml

# Run repair tool (scan and repair missing blocks)
./repair -config configs/config.yaml

# Repair tool with options
./repair -config configs/config.yaml -start 1000 -end 2000  # Repair specific range
./repair -config configs/config.yaml -dry-run               # Scan only, don't repair
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