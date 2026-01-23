#!/bin/bash

# Script to check if steemd container can connect to cold_ingest service
# This helps diagnose network connectivity issues

set -e

CONTAINER_NAME="${CONTAINER_NAME:-steemd-ingest-test}"
INGEST_ENDPOINT="${INGEST_ENDPOINT:-http://host.docker.internal:8080/ingest/applied_ops}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Checking steemd container connectivity ===${NC}"

# Check if container exists
if ! docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${RED}Error: Container $CONTAINER_NAME does not exist${NC}"
    exit 1
fi

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${YELLOW}Warning: Container $CONTAINER_NAME is not running${NC}"
    exit 1
fi

echo "Container: $CONTAINER_NAME"
echo "Target endpoint: $INGEST_ENDPOINT"
echo ""

# Test 1: Check if curl is available in container
echo -e "${YELLOW}Test 1: Checking if curl is available...${NC}"
if docker exec "$CONTAINER_NAME" which curl > /dev/null 2>&1; then
    echo -e "${GREEN}✓ curl is available${NC}"
else
    echo -e "${YELLOW}curl is not available, installing...${NC}"
    if docker exec "$CONTAINER_NAME" apt-get update -qq > /dev/null 2>&1 && \
       docker exec "$CONTAINER_NAME" apt-get install -y -qq curl > /dev/null 2>&1; then
        echo -e "${GREEN}✓ curl installed successfully${NC}"
    else
        echo -e "${RED}✗ Failed to install curl${NC}"
        echo "  You may need to install it manually:"
        echo "    docker exec $CONTAINER_NAME apt-get update"
        echo "    docker exec $CONTAINER_NAME apt-get install -y curl"
        exit 1
    fi
fi

# Test 2: Test DNS resolution
echo ""
echo -e "${YELLOW}Test 2: Testing DNS resolution...${NC}"
if docker exec "$CONTAINER_NAME" getent hosts host.docker.internal > /dev/null 2>&1; then
    HOST_IP=$(docker exec "$CONTAINER_NAME" getent hosts host.docker.internal | awk '{print $1}')
    echo -e "${GREEN}✓ host.docker.internal resolves to: $HOST_IP${NC}"
else
    echo -e "${RED}✗ host.docker.internal cannot be resolved${NC}"
    echo "  This might be a Linux Docker issue. Try using the host's IP address instead."
fi

# Test 3: Test HTTP connectivity (GET request - should return Method Not Allowed)
echo ""
echo -e "${YELLOW}Test 3: Testing HTTP connectivity (GET request)...${NC}"
RESPONSE=$(docker exec "$CONTAINER_NAME" curl -s -w "\n%{http_code}" "$INGEST_ENDPOINT" 2>&1 || echo "ERROR")
HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)
BODY=$(echo "$RESPONSE" | head -n -1)

if [ "$HTTP_CODE" = "405" ]; then
    echo -e "${GREEN}✓ HTTP connection successful (405 Method Not Allowed is expected for GET)${NC}"
    echo "  This means:"
    echo "    - Network connection is working"
    echo "    - The endpoint is reachable"
    echo "    - The server is responding"
    echo "    - 405 is NOT a permission issue, it just means GET is not allowed (POST is required)"
    echo ""
    echo "  HTTP Status Codes:"
    echo "    - 401 Unauthorized = Authentication required"
    echo "    - 403 Forbidden = Permission denied"
    echo "    - 405 Method Not Allowed = Wrong HTTP method (GET vs POST)"
elif [ "$HTTP_CODE" = "ERROR" ] || [ -z "$HTTP_CODE" ]; then
    echo -e "${RED}✗ HTTP connection failed${NC}"
    echo "  Response: $RESPONSE"
    echo ""
    echo "  Possible issues:"
    echo "  1. cold_ingest service is not running on host"
    echo "  2. Network connectivity problem (host.docker.internal not working)"
    echo "  3. Firewall blocking the connection"
else
    echo -e "${YELLOW}⚠ Unexpected HTTP code: $HTTP_CODE${NC}"
    echo "  Response body: $BODY"
fi

# Test 4: Test POST request with sample data
echo ""
echo -e "${YELLOW}Test 4: Testing POST request with sample data...${NC}"
SAMPLE_JSON='[{"block":{"num":1,"id":"0000000000000001","timestamp":"2016-03-24T16:05:00"},"transaction":{"id":"test_trx","index":0},"operation":{"index":0,"type":"transfer","value":{}},"virtual":false}]'
RESPONSE=$(docker exec "$CONTAINER_NAME" curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "$SAMPLE_JSON" \
    "$INGEST_ENDPOINT" 2>&1 || echo "ERROR")
HTTP_CODE=$(echo "$RESPONSE" | tail -n 1)
BODY=$(echo "$RESPONSE" | head -n -1)

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ POST request successful (200 OK)${NC}"
    echo "  The ingest endpoint is working correctly!"
elif [ "$HTTP_CODE" = "ERROR" ] || [ -z "$HTTP_CODE" ]; then
    echo -e "${RED}✗ POST request failed${NC}"
    echo "  Response: $RESPONSE"
else
    echo -e "${YELLOW}⚠ Unexpected HTTP code: $HTTP_CODE${NC}"
    echo "  Response body: $BODY"
fi

# Test 5: Check steemd logs for ingest plugin status
echo ""
echo -e "${YELLOW}Test 5: Checking steemd logs for ingest plugin...${NC}"
LOGS=$(docker logs --tail 100 "$CONTAINER_NAME" 2>&1)

if echo "$LOGS" | grep -qi "ingest.*plugin"; then
    echo -e "${GREEN}✓ Ingest plugin found in logs${NC}"
    echo "$LOGS" | grep -i "ingest.*plugin" | head -n 3
else
    echo -e "${RED}✗ No ingest plugin messages found in logs${NC}"
    echo "  The ingest plugin might not be loaded."
fi

if echo "$LOGS" | grep -qi "HTTP.*error\|HTTP.*send.*error"; then
    echo -e "${RED}✗ HTTP errors found in logs:${NC}"
    echo "$LOGS" | grep -i "HTTP.*error\|HTTP.*send.*error" | head -n 5
fi

# Check for queue full messages
QUEUE_FULL_COUNT=$(echo "$LOGS" | grep -c "Ingest queue full" || echo "0")
if [ "$QUEUE_FULL_COUNT" -gt 0 ]; then
    echo -e "${RED}✗ WARNING: Found $QUEUE_FULL_COUNT 'queue full' messages${NC}"
    echo "  Operations are being dropped! This usually means:"
    echo "    1. cold_ingest is not running or not processing requests"
    echo "    2. cold_ingest is too slow"
    echo "    3. Network issues"
    echo ""
    echo "  Run ./diagnose_queue.sh for detailed diagnosis"
    echo "$LOGS" | grep "Ingest queue full" | tail -n 3
fi

if echo "$LOGS" | grep -qi "replay"; then
    echo -e "${GREEN}✓ Replay appears to be in progress${NC}"
    echo "$LOGS" | grep -i "replay" | tail -n 2
fi

echo ""
echo -e "${GREEN}=== Diagnostic complete ===${NC}"
