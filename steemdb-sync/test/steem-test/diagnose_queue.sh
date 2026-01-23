#!/bin/bash

# Script to diagnose why ingest queue is full
# This helps identify if cold_ingest is receiving data or if there are HTTP errors
#
# Usage:
#   ./diagnose_queue.sh                    # Run once
#   watch -n 5 ./diagnose_queue.sh        # Run every 5 seconds (useful during E2E tests)
#
# This script can be run in parallel with E2E tests to monitor the ingest process

set -e

CONTAINER_NAME="${CONTAINER_NAME:-steemd-ingest-test}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}=== Diagnosing Ingest Queue Issue ===${NC}"
echo ""

# Check 1: Is cold_ingest running?
echo -e "${YELLOW}Check 1: Is cold_ingest service running?${NC}"
if curl -s http://localhost:8080/ingest/applied_ops > /dev/null 2>&1; then
    echo -e "${GREEN}✓ cold_ingest is accessible on localhost:8080${NC}"
    # Try to get metrics if available
    if curl -s http://localhost:8080/metrics > /dev/null 2>&1; then
        METRICS=$(curl -s http://localhost:8080/metrics 2>/dev/null | grep -E "steemdb_sync_ingest_ops_total|steemdb_sync_ingest_ops_per_second" || true)
        if [ -n "$METRICS" ]; then
            echo "  Metrics available:"
            echo "$METRICS" | head -n 3 | sed 's/^/    /'
        fi
    fi
else
    echo -e "${RED}✗ cold_ingest is NOT accessible on localhost:8080${NC}"
    echo "  This is likely the root cause!"
    echo ""
    echo "  Note: If you're running E2E tests, cold_ingest should be started by the test."
    echo "        If it's not running, check the test output."
    echo ""
    echo "  To start manually:"
    echo "    cd steemdb-sync"
    echo "    ../bin/cold_ingest -config configs/config.yaml"
    # Don't exit if running in watch mode - just report the issue
    if [ -z "$WATCH_MODE" ]; then
        exit 1
    fi
fi
echo ""

# Check 2: Check if steemd container exists
echo -e "${YELLOW}Check 2: Checking steemd container status...${NC}"
if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}⚠ Container $CONTAINER_NAME does not exist${NC}"
    echo "  This is normal if E2E test hasn't started steemd yet."
    echo "  Waiting for container to appear..."
    exit 0
fi

if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}⚠ Container $CONTAINER_NAME exists but is not running${NC}"
    echo "  Container may be starting or stopped."
    exit 0
fi

echo -e "${GREEN}✓ Container $CONTAINER_NAME is running${NC}"
echo ""

# Check 3: Check steemd logs for HTTP errors
echo -e "${YELLOW}Check 3: Checking steemd logs for HTTP errors...${NC}"
LOGS=$(docker logs --tail 100 "$CONTAINER_NAME" 2>&1)

HTTP_ERRORS=$(echo "$LOGS" | grep -i "HTTP.*error\|HTTP.*send.*error" || true)
if [ -n "$HTTP_ERRORS" ]; then
    echo -e "${RED}✗ Found HTTP errors in logs:${NC}"
    echo "$HTTP_ERRORS" | head -n 10
    echo ""
    echo "  This indicates network or connection problems."
else
    echo -e "${GREEN}✓ No HTTP errors found in logs${NC}"
fi
echo ""

# Check 4: Count queue full messages
echo -e "${YELLOW}Check 4: Counting 'queue full' messages...${NC}"
QUEUE_FULL_COUNT=$(echo "$LOGS" | grep -c "Ingest queue full" || echo "0")
if [ "$QUEUE_FULL_COUNT" -gt 0 ]; then
    echo -e "${RED}✗ Found $QUEUE_FULL_COUNT 'queue full' messages${NC}"
    echo "  This means operations are being dropped!"
    echo ""
    echo "  Recent queue full messages:"
    echo "$LOGS" | grep "Ingest queue full" | tail -n 5
else
    echo -e "${GREEN}✓ No 'queue full' messages found${NC}"
fi
echo ""

# Check 5: Check if ingest plugin is sending data
echo -e "${YELLOW}Check 5: Checking if ingest plugin is active...${NC}"
if echo "$LOGS" | grep -qi "ingest.*plugin\|send_operation"; then
    echo -e "${GREEN}✓ Ingest plugin appears to be active${NC}"
    echo "  Recent ingest activity:"
    echo "$LOGS" | grep -i "ingest\|send_operation" | tail -n 3
else
    echo -e "${YELLOW}⚠ No ingest plugin activity found in logs${NC}"
fi
echo ""

# Check 6: Test POST request from container
echo -e "${YELLOW}Check 6: Testing POST request from container...${NC}"
if ! docker exec "$CONTAINER_NAME" which curl > /dev/null 2>&1; then
    echo "Installing curl in container..."
    docker exec "$CONTAINER_NAME" apt-get update -qq > /dev/null 2>&1
    docker exec "$CONTAINER_NAME" apt-get install -y -qq curl > /dev/null 2>&1
fi

SAMPLE_JSON='[{"block":{"num":1,"id":"0000000000000001","timestamp":"2016-03-24T16:05:00"},"transaction":{"id":"test","index":0},"operation":{"index":0,"type":"transfer","value":{}},"virtual":false}]'
RESPONSE=$(docker exec "$CONTAINER_NAME" curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "$SAMPLE_JSON" \
    "http://host.docker.internal:8080/ingest/applied_ops" 2>&1 || echo "ERROR")
HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ POST request successful from container${NC}"
    echo "  Network connectivity is working."
elif [ "$HTTP_CODE" = "ERROR" ] || [ -z "$HTTP_CODE" ]; then
    echo -e "${RED}✗ POST request failed from container${NC}"
    echo "  This is a critical issue!"
    echo "  Response: $RESPONSE"
else
    echo -e "${YELLOW}⚠ Unexpected HTTP code: $HTTP_CODE${NC}"
fi
echo ""

# Summary and recommendations
echo -e "${YELLOW}=== Summary and Recommendations ===${NC}"
echo ""

if [ "$QUEUE_FULL_COUNT" -gt 0 ]; then
    echo -e "${RED}Problem: Queue is full, operations are being dropped${NC}"
    echo ""
    echo "Possible causes:"
    echo "  1. cold_ingest is not running or not processing requests"
    echo "  2. cold_ingest is too slow to process requests"
    echo "  3. Network issues causing HTTP requests to fail"
    echo "  4. Queue size (100000) is too small for the replay speed"
    echo ""
    echo "Solutions:"
    echo "  1. Ensure cold_ingest is running:"
    echo "     cd steemdb-sync"
    echo "     ../bin/cold_ingest -config configs/config.yaml"
    echo ""
    echo "  2. Check cold_ingest logs for errors"
    echo ""
    echo "  3. Increase queue size (if needed):"
    echo "     INGEST_QUEUE_SIZE=500000 ./run.sh"
    echo ""
    echo "  4. Check if cold_ingest is processing data:"
    echo "     - Check MongoDB for new operations"
    echo "     - Check cold_ingest logs for '[IngestHandler] Received request' messages"
else
    echo -e "${GREEN}No immediate issues detected${NC}"
    echo "  Queue is not full, operations should be processing normally."
fi
