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

- **steemgosdk**: For Steem RPC communication
- **steemutil**: For Steem protocol structures (operations, blocks, etc.)
- **prometheus/client_golang**: For Prometheus metrics