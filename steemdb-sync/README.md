# SteemDB Sync

Steem blockchain data synchronization service for SteemDB explorer.

## Architecture

This service implements a three-phase synchronization architecture:

1. **Cold Start**: Uses steemd replay + plugin for fast historical data ingestion
2. **Live Sync**: Uses RPC to continuously sync new blocks
3. **Repair**: Uses RPC to fix missing blocks

## Components

- `cmd/cold_ingest`: HTTP service that receives operations from steemd plugin
- `cmd/live_sync`: RPC-based live block synchronization service
- `cmd/repair`: Tool for repairing missing blocks

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

# Run repair tool
./repair -config configs/config.yaml
```

## Dependencies

- **steemgosdk**: For Steem RPC communication
- **steemutil**: For Steem protocol structures (operations, blocks, etc.)
