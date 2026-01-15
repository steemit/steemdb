#!/bin/bash

# Script to start steemd container with ingest plugin for E2E testing
# This script is designed to work with the cold_ingest E2E test

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="$SCRIPT_DIR/data"

# Configuration (can be overridden by environment variables)
STEEMD_IMAGE="${STEEMD_IMAGE:-steemd:with-ingest}"
CONTAINER_NAME="${CONTAINER_NAME:-steemd-ingest-test}"
INGEST_ENDPOINT="${INGEST_ENDPOINT:-http://host.docker.internal:8080/ingest/applied_ops}"
INGEST_HTTP_TIMEOUT="${INGEST_HTTP_TIMEOUT:-5000}"
INGEST_QUEUE_SIZE="${INGEST_QUEUE_SIZE:-100000}"
DATA_DIR_MOUNT="${DATA_DIR_MOUNT:-/var/steem}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Starting steemd with ingest plugin ===${NC}"

# Check if Docker image exists
if ! docker images | grep -q "$STEEMD_IMAGE"; then
    echo -e "${RED}Error: Docker image $STEEMD_IMAGE not found${NC}"
    echo "Please build the image first:"
    echo "  cd steem"
    echo "  docker build -f deploy/Dockerfile.ubuntu24.04 -t $STEEMD_IMAGE ."
    exit 1
fi

# Check if container already exists
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        echo -e "${YELLOW}Container $CONTAINER_NAME is already running${NC}"
        exit 0
    else
        echo -e "${YELLOW}Container $CONTAINER_NAME exists but is stopped. Removing...${NC}"
        docker rm "$CONTAINER_NAME" || true
    fi
fi

# Clean up RocksDB files if they exist (optional, for fresh start)
if [ -d "$DATA_DIR/blockchain" ]; then
    echo "Cleaning up RocksDB files for fresh start..."
    sudo rm -rf "$DATA_DIR/blockchain/rocksdb_"* 2>/dev/null || true
fi

# Ensure data directory exists
mkdir -p "$DATA_DIR"

# Start the container
echo -e "${GREEN}Starting container: $CONTAINER_NAME${NC}"
echo "  Image: $STEEMD_IMAGE"
echo "  Ingest endpoint: $INGEST_ENDPOINT"
echo "  Data directory: $DATA_DIR -> $DATA_DIR_MOUNT"

docker run -d --rm \
  --name "$CONTAINER_NAME" \
  --add-host host.docker.internal:host-gateway \
  -v "$DATA_DIR:$DATA_DIR_MOUNT" \
  "$STEEMD_IMAGE" \
  /usr/local/steemd/bin/steemd \
    --replay-blockchain \
    --plugin ingest \
    --ingest-endpoint "$INGEST_ENDPOINT" \
    --ingest-http-timeout "$INGEST_HTTP_TIMEOUT" \
    --ingest-queue-size "$INGEST_QUEUE_SIZE" \
    --data-dir "$DATA_DIR_MOUNT"

echo -e "${GREEN}Container started successfully!${NC}"
echo ""
echo "Useful commands:"
echo "  View logs:    $SCRIPT_DIR/logs.sh"
echo "  Stop:         $SCRIPT_DIR/stop.sh"
echo "  Check status: docker ps | grep $CONTAINER_NAME"
