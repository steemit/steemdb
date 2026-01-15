#!/bin/bash

# Script to view steemd container logs

set -e

# Configuration (can be overridden by environment variables)
CONTAINER_NAME="${CONTAINER_NAME:-steemd-ingest-test}"
LOG_LINES="${LOG_LINES:-100}"
FOLLOW="${FOLLOW:-true}"

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if container exists
if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${RED}Error: Container $CONTAINER_NAME does not exist${NC}"
    exit 1
fi

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}Warning: Container $CONTAINER_NAME is not running${NC}"
    echo "Showing last logs from stopped container..."
    docker logs --tail "$LOG_LINES" "$CONTAINER_NAME"
    exit 0
fi

# Show logs
if [ "$FOLLOW" = "true" ]; then
    echo "Following logs for container: $CONTAINER_NAME (Ctrl+C to exit)"
    docker logs -f --tail "$LOG_LINES" "$CONTAINER_NAME"
else
    docker logs --tail "$LOG_LINES" "$CONTAINER_NAME"
fi
