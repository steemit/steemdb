# Steemd Test Container Scripts

This directory contains helper scripts to manage a `steemd` Docker container with the ingest plugin for E2E testing.

## Prerequisites

1. **Docker image**: Build the `steemd:with-ingest` image:
   ```bash
   cd steem
   docker build -f deploy/Dockerfile.ubuntu24.04 -t steemd:with-ingest .
   ```

2. **Blockchain data**: Place your `block_log` and `block_log.index` files in the `data/` directory:
   ```bash
   # Example structure:
   data/
   ├── blockchain/
   │   ├── block_log
   │   └── block_log.index
   └── config.ini
   ```

## Scripts

### `run.sh` - Start steemd with ingest plugin

Starts a `steemd` container with the ingest plugin enabled. The container will:
- Replay the blockchain from `block_log`
- Send operations to the ingest endpoint via HTTP POST
- Use the data directory mounted from `./data`

**Usage:**
```bash
./run.sh
```

**Environment variables** (optional):
- `STEEMD_IMAGE`: Docker image name (default: `steemd:with-ingest`)
- `CONTAINER_NAME`: Container name (default: `steemd-ingest-test`)
- `INGEST_ENDPOINT`: HTTP endpoint for ingest service (default: `http://host.docker.internal:8080/ingest/applied_op`)
- `INGEST_HTTP_TIMEOUT`: HTTP timeout in milliseconds (default: `5000`)
- `INGEST_QUEUE_SIZE`: Maximum queue size (default: `100000`)

**Example:**
```bash
INGEST_ENDPOINT=http://host.docker.internal:8080/ingest/applied_op ./run.sh
```

### `run-dry.sh` - Start steemd in dry-run mode

Starts `steemd` in dry-run mode, which logs operations to files instead of sending HTTP requests. Useful for testing without a running ingest service.

**Usage:**
```bash
./run-dry.sh
```

Operations will be logged to `data/ingest/` directory.

### `stop.sh` - Stop the container

Stops and removes the `steemd` container.

**Usage:**
```bash
./stop.sh
```

**Environment variables** (optional):
- `CONTAINER_NAME`: Container name (default: `steemd-ingest-test`)

### `logs.sh` - View container logs

Shows the container logs. By default, it follows the logs (use Ctrl+C to exit).

**Usage:**
```bash
./logs.sh
```

**Environment variables** (optional):
- `CONTAINER_NAME`: Container name (default: `steemd-ingest-test`)
- `LOG_LINES`: Number of lines to show (default: `100`)
- `FOLLOW`: Whether to follow logs (default: `true`)

**Examples:**
```bash
# Show last 50 lines without following
LOG_LINES=50 FOLLOW=false ./logs.sh

# Show last 200 lines and follow
LOG_LINES=200 ./logs.sh
```

## Integration with E2E Tests

The E2E test (`test/e2e/cold_ingest_test.go`) will automatically detect if these scripts are available and provide helpful instructions if the container is not running.

**Typical workflow:**

1. Start MongoDB test container:
   ```bash
   cd steemdb-sync/test
   ./scripts/start_mongo.sh
   ```

2. Start `cold_ingest` service (in one terminal):
   ```bash
   cd steemdb-sync
   ../bin/cold_ingest -config configs/config.yaml
   ```

3. Start `steemd` container (in another terminal):
   ```bash
   cd steemdb-sync/test/steem-test
   ./run.sh
   ```

4. Run E2E test:
   ```bash
   cd steemdb-sync
   go test -v ./test/e2e/... -run TestColdIngestE2E
   ```

## Data Directory Structure

```
data/
├── blockchain/          # Blockchain data (block_log, block_log.index, etc.)
│   └── rocksdb_*/       # RocksDB databases (auto-generated, can be cleaned)
├── config.ini          # Steemd configuration
├── database.cfg        # Database configuration
├── ingest/             # Dry-run mode operation logs
│   └── ingest_*.jsonl
└── logs/               # Steemd logs
    └── p2p/
```

## Troubleshooting

### Container won't start

- Check if the Docker image exists: `docker images | grep steemd:with-ingest`
- Check if port 8080 is available (for ingest endpoint)
- Verify `block_log` file exists in `data/blockchain/`

### Container starts but no operations received

- Verify `cold_ingest` service is running and accessible
- Check container logs: `./logs.sh`
- Verify ingest endpoint URL is correct (should be `http://host.docker.internal:8080/ingest/applied_op`)

### Container is stuck

- Check if `block_log.index` is being rebuilt (this can take time)
- View logs: `./logs.sh`
- Check if there are any errors in the logs

### Clean start

To start fresh (removes RocksDB databases):
```bash
./stop.sh
sudo rm -rf data/blockchain/rocksdb_*
./run.sh
```

## Debugging Scripts

### `check_connection.sh` - Test network connectivity

Tests if the steemd container can connect to the ingest endpoint:
- DNS resolution for `host.docker.internal`
- HTTP connectivity (GET and POST requests)
- Checks steemd logs for ingest plugin status

**Usage:**
```bash
./check_connection.sh
```

**Note:** The script will automatically install `curl` in the container if it's not available.

### `test_post.sh` - Test POST request

Sends a sample POST request to verify the ingest endpoint accepts POST requests correctly.

**Usage:**
```bash
./test_post.sh
```

**Note:** The script will automatically install `curl` in the container if it's not available.

### `diagnose_queue.sh` - Diagnose queue full issues

Comprehensive diagnosis tool for when the ingest queue is full and operations are being dropped:
- Checks if cold_ingest is running
- Looks for HTTP errors in steemd logs
- Counts queue full messages
- Tests POST requests from container
- Provides recommendations

**Usage:**
```bash
# Run once
./diagnose_queue.sh

# Run continuously (useful during E2E tests)
watch -n 5 ./diagnose_queue.sh
```

**When to use:** 
- If you see "Ingest queue full, dropping operation" messages in steemd logs
- During E2E tests to monitor ingest process in real-time
- To debug why operations are not being processed

**Note:** This script can be run in parallel with E2E tests. It will gracefully handle cases where the container hasn't started yet.

## Notes

- The scripts use `--rm` flag, so containers are automatically removed when stopped
- Data in `data/` directory persists between container runs
- The container uses `host.docker.internal` to access services on the host machine
- For Linux, `--add-host host.docker.internal:host-gateway` is required (already included in scripts)
- The `steemd:with-ingest` image does not include `curl` by default. Debugging scripts (`check_connection.sh`, `test_post.sh`) will automatically install it when needed
- To manually install curl in the container:
  ```bash
  docker exec steemd-ingest-test apt-get update
  docker exec steemd-ingest-test apt-get install -y curl
  ```