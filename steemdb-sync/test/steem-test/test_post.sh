#!/bin/bash

# Quick test script to verify POST requests work from steemd container
# This helps confirm that the ingest endpoint accepts POST requests correctly

set -e

CONTAINER_NAME="${CONTAINER_NAME:-steemd-ingest-test}"
INGEST_ENDPOINT="${INGEST_ENDPOINT:-http://host.docker.internal:8080/ingest/applied_ops}"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Testing POST request to ingest endpoint...${NC}"
echo "Endpoint: $INGEST_ENDPOINT"
echo ""

# Check and install curl if needed
echo "Checking if curl is available..."
if ! docker exec "$CONTAINER_NAME" which curl > /dev/null 2>&1; then
    echo -e "${YELLOW}curl not found, installing...${NC}"
    docker exec "$CONTAINER_NAME" apt-get update -qq > /dev/null 2>&1
    docker exec "$CONTAINER_NAME" apt-get install -y -qq curl > /dev/null 2>&1
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ curl installed successfully${NC}"
    else
        echo -e "${RED}✗ Failed to install curl${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ curl is available${NC}"
fi
echo ""

# Sample operation JSON (matching the format from steemd ingest plugin)
SAMPLE_JSON='[{
  "block": {
    "num": 1,
    "id": "0000000000000001",
    "timestamp": "2016-03-24T16:05:00"
  },
  "transaction": {
    "id": "test_transaction_id",
    "index": 0
  },
  "operation": {
    "index": 0,
    "type": "transfer",
    "value": {
      "from": "alice",
      "to": "bob",
      "amount": "1.000 STEEM",
      "memo": "test"
    }
  },
  "virtual": false
}]'

echo "Sending POST request..."
echo ""

# Test POST request
RESPONSE=$(docker exec "$CONTAINER_NAME" curl -s -w "\nHTTP_CODE:%{http_code}\nTIME:%{time_total}" \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$SAMPLE_JSON" \
    "$INGEST_ENDPOINT" 2>&1 || echo "ERROR")

# Parse response
HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
TIME_TOTAL=$(echo "$RESPONSE" | grep "TIME:" | cut -d: -f2)
BODY=$(echo "$RESPONSE" | sed '/HTTP_CODE:/d' | sed '/TIME:/d')

echo "Response:"
echo "  HTTP Status: $HTTP_CODE"
echo "  Response Time: ${TIME_TOTAL}s"
echo "  Response Body: $BODY"
echo ""

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "${GREEN}✓ POST request successful!${NC}"
    echo "  The ingest endpoint is working correctly."
    echo "  If steemd is not sending data, the issue is likely:"
    echo "    1. Ingest plugin not loaded in steemd"
    echo "    2. Steemd not started replay yet"
    echo "    3. Ingest plugin configuration issue"
elif [ "$HTTP_CODE" = "ERROR" ] || [ -z "$HTTP_CODE" ]; then
    echo -e "${RED}✗ POST request failed${NC}"
    echo "  Full response: $RESPONSE"
    echo ""
    echo "  Possible issues:"
    echo "    1. Network connectivity problem"
    echo "    2. cold_ingest service not running"
    echo "    3. Firewall blocking POST requests (unlikely if GET works)"
elif [ "$HTTP_CODE" = "400" ]; then
    echo -e "${YELLOW}⚠ Bad Request (400)${NC}"
    echo "  The endpoint received the request but rejected the data format."
    echo "  Response: $BODY"
    echo "  This might indicate a JSON format issue."
elif [ "$HTTP_CODE" = "405" ]; then
    echo -e "${RED}✗ Method Not Allowed (405) for POST${NC}"
    echo "  This is unexpected! POST should be allowed."
    echo "  Check if the endpoint configuration is correct."
else
    echo -e "${YELLOW}⚠ Unexpected HTTP code: $HTTP_CODE${NC}"
    echo "  Response: $BODY"
fi

echo ""
echo "Note: 405 for GET is normal (endpoint only accepts POST)"
echo "      But 405 for POST would indicate a configuration problem."
