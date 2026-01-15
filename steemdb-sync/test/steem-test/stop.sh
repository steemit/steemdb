#!/bin/bash

# Script to stop steemd container

set -e

# Configuration (can be overridden by environment variables)
CONTAINER_NAME="${CONTAINER_NAME:-steemd-ingest-test}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Stopping steemd container ===${NC}"

# Check if container exists
if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}Container $CONTAINER_NAME does not exist${NC}"
    exit 0
fi

# Check if container is running
if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Stopping container: $CONTAINER_NAME"
    docker stop -t 10 "$CONTAINER_NAME" || {
        echo -e "${RED}Failed to stop container gracefully, forcing removal...${NC}"
        docker rm -f "$CONTAINER_NAME" || true
    }
    echo -e "${GREEN}Container stopped successfully${NC}"
else
    echo -e "${YELLOW}Container $CONTAINER_NAME is not running${NC}"
    # Try to remove if it exists but is stopped
    docker rm "$CONTAINER_NAME" 2>/dev/null || true
fi
