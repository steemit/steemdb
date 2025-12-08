#!/bin/bash
# Script to verify steemdb-sync service is working as expected

echo "=========================================="
echo "SteemDB Sync Service Verification"
echo "=========================================="
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 1. Check service status
echo "1. Checking service status..."
STATUS=$(docker compose ps steemdb-sync --format json | jq -r '.[0].Health // "unknown"')
if [ "$STATUS" = "healthy" ]; then
    echo -e "${GREEN}✓ Service is healthy${NC}"
else
    echo -e "${RED}✗ Service status: $STATUS${NC}"
fi
echo ""

# 2. Check for time parsing errors (should be 0)
echo "2. Checking for time parsing errors (last 10 minutes)..."
TIME_ERRORS=$(docker compose logs --since 10m steemdb-sync 2>&1 | grep -iE 'parsing time|unmarshal.*time|time.*error|failed.*time' | wc -l)
if [ "$TIME_ERRORS" -eq 0 ]; then
    echo -e "${GREEN}✓ No time parsing errors found${NC}"
else
    echo -e "${RED}✗ Found $TIME_ERRORS time parsing errors${NC}"
    docker compose logs --since 10m steemdb-sync 2>&1 | grep -iE 'parsing time|unmarshal.*time|time.*error|failed.*time' | head -5
fi
echo ""

# 3. Check service startup
echo "3. Checking service startup..."
STARTUP=$(docker compose logs steemdb-sync 2>&1 | grep -iE 'started successfully|SteemDB Sync Service started' | tail -1)
if [ -n "$STARTUP" ]; then
    echo -e "${GREEN}✓ Service started successfully${NC}"
    echo "  $STARTUP"
else
    echo -e "${YELLOW}⚠ Could not find startup message${NC}"
fi
echo ""

# 4. Check sync statistics
echo "4. Checking sync statistics..."
STATS=$(docker compose logs --tail=200 steemdb-sync 2>&1 | grep -i 'Block sync statistics' | tail -1)
if [ -n "$STATS" ]; then
    echo -e "${GREEN}✓ Sync statistics found:${NC}"
    echo "  $STATS"
else
    echo -e "${YELLOW}⚠ No sync statistics found yet${NC}"
fi
echo ""

# 5. Check for critical errors (excluding duplicate key errors)
echo "5. Checking for critical errors (excluding duplicate keys)..."
CRITICAL_ERRORS=$(docker compose logs --tail=200 steemdb-sync 2>&1 | grep -i error | grep -v 'duplicate key' | grep -v 'multi-key map' | grep -v 'BadValue.*_id index' | wc -l)
if [ "$CRITICAL_ERRORS" -eq 0 ]; then
    echo -e "${GREEN}✓ No critical errors found${NC}"
else
    echo -e "${YELLOW}⚠ Found $CRITICAL_ERRORS critical errors (excluding duplicate keys)${NC}"
    docker compose logs --tail=200 steemdb-sync 2>&1 | grep -i error | grep -v 'duplicate key' | grep -v 'multi-key map' | grep -v 'BadValue.*_id index' | head -5
fi
echo ""

# 6. Check history and witness services
echo "6. Checking history and witness services..."
HISTORY=$(docker compose logs --tail=100 steemdb-sync 2>&1 | grep -iE 'history service|account history' | tail -1)
WITNESS=$(docker compose logs --tail=100 steemdb-sync 2>&1 | grep -iE 'witness service|witness missed' | tail -1)
if [ -n "$HISTORY" ]; then
    echo -e "${GREEN}✓ History service is running${NC}"
else
    echo -e "${YELLOW}⚠ History service status unclear${NC}"
fi
if [ -n "$WITNESS" ]; then
    echo -e "${GREEN}✓ Witness service is running${NC}"
    echo "  $WITNESS"
else
    echo -e "${YELLOW}⚠ Witness service status unclear${NC}"
fi
echo ""

# 7. Check Prometheus metrics (if available)
echo "7. Checking Prometheus metrics..."
if docker compose ps prometheus --format json | jq -e '.[0].State == "running"' > /dev/null 2>&1; then
    METRICS=$(docker compose exec -T steemdb-sync curl -s http://localhost:9091/metrics 2>/dev/null | grep -E 'steemdb_blocks_processed_total|steemdb_operations_processed_total' | head -2)
    if [ -n "$METRICS" ]; then
        echo -e "${GREEN}✓ Metrics available:${NC}"
        echo "$METRICS" | sed 's/^/  /'
    else
        echo -e "${YELLOW}⚠ Metrics endpoint not accessible${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Prometheus not running or metrics not accessible${NC}"
fi
echo ""

# 8. Summary
echo "=========================================="
echo "Summary"
echo "=========================================="
if [ "$STATUS" = "healthy" ] && [ "$TIME_ERRORS" -eq 0 ]; then
    echo -e "${GREEN}✓ Service appears to be working correctly${NC}"
    echo -e "${GREEN}✓ No time parsing errors detected${NC}"
    exit 0
else
    echo -e "${RED}✗ Some issues detected. Please review the output above.${NC}"
    exit 1
fi

