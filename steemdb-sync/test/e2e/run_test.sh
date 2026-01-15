#!/bin/bash

# Cold Ingest E2E Test Runner
# This script helps run the E2E tests with proper setup
#
# Note: This script only starts MongoDB container if needed.
# The steemd container is automatically started by the Go test code
# (using test/steem-test/run.sh) if it's not already running.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Cold Ingest E2E Test Runner ===${NC}\n"

# Check if MongoDB is running
echo "Checking MongoDB..."
if ! docker ps | grep -q mongo-test; then
    echo -e "${YELLOW}MongoDB test container not found. Starting...${NC}"
    "$SCRIPT_DIR/../scripts/start_mongo.sh"
    sleep 5
fi

# Check if cold_ingest binary exists
BIN_DIR="$(cd "$PROJECT_ROOT/.." && pwd)/bin"
mkdir -p "$BIN_DIR"
echo "Checking cold_ingest binary..."
if [ ! -f "$BIN_DIR/cold_ingest" ]; then
    echo -e "${YELLOW}cold_ingest binary not found. Building...${NC}"
    go build -o "$BIN_DIR/cold_ingest" ./cmd/cold_ingest
fi

# Check if steemd image exists (for full E2E test)
# Note: The steemd container will be automatically started by the Go test code
# using test/steem-test/run.sh script, not by this script
if docker images | grep -q "steemd.*with-ingest"; then
    echo -e "${GREEN}steemd:with-ingest image found${NC}"
    echo -e "${YELLOW}Note: steemd container will be auto-started by test code if not already running${NC}"
    RUN_FULL_E2E=true
else
    echo -e "${YELLOW}steemd:with-ingest image not found. Will skip full E2E test${NC}"
    RUN_FULL_E2E=false
fi

# Set environment variables
export MONGODB_URI="${MONGODB_URI:-mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin}"

# Run tests
echo -e "\n${GREEN}Running E2E tests...${NC}\n"

if [ "$RUN_FULL_E2E" = true ]; then
    echo "Running all E2E tests (including full steemd test)..."
    echo "Note: Full E2E test may take up to 90 minutes (includes steemd replay time)"
    go test -v ./test/e2e/... -timeout 90m "$@"
else
    echo "Running mock plugin test only..."
    go test -v ./test/e2e/... -run TestColdIngestWithMockPlugin -timeout 10m "$@"
fi

echo -e "\n${GREEN}=== E2E Tests Complete ===${NC}"
