#!/bin/bash
# Script to check steemdb-sync synchronization speed

echo "=========================================="
echo "SteemDB Sync Speed Monitor"
echo "=========================================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get latest statistics from logs
echo -e "${BLUE}1. Current Sync Statistics${NC}"
echo "----------------------------------------"
STATS=$(docker compose logs --tail=500 steemdb-sync 2>&1 | grep 'Block sync statistics' | tail -1)
if [ -n "$STATS" ]; then
    # Extract values using jq if available, otherwise use grep/sed
    if command -v jq &> /dev/null; then
        BLOCKS_PROCESSED=$(echo "$STATS" | jq -r '.blocks_processed // 0')
        OPS_PROCESSED=$(echo "$STATS" | jq -r '.operations_processed // 0')
        BLOCKS_PER_SEC=$(echo "$STATS" | jq -r '.blocks_per_second // 0')
        OPS_PER_SEC=$(echo "$STATS" | jq -r '.ops_per_second // 0')
        LAST_BLOCK=$(echo "$STATS" | jq -r '.last_block // 0')
        ERRORS=$(echo "$STATS" | jq -r '.errors // 0')
        TIMESTAMP=$(echo "$STATS" | jq -r '.timestamp // "unknown"')
    else
        BLOCKS_PROCESSED=$(echo "$STATS" | grep -oE '"blocks_processed":[0-9]+' | cut -d: -f2)
        OPS_PROCESSED=$(echo "$STATS" | grep -oE '"operations_processed":[0-9]+' | cut -d: -f2)
        BLOCKS_PER_SEC=$(echo "$STATS" | grep -oE '"blocks_per_second":[0-9.]+' | cut -d: -f2)
        OPS_PER_SEC=$(echo "$STATS" | grep -oE '"ops_per_second":[0-9.]+' | cut -d: -f2)
        LAST_BLOCK=$(echo "$STATS" | grep -oE '"last_block":[0-9]+' | cut -d: -f2)
        ERRORS=$(echo "$STATS" | grep -oE '"errors":[0-9]+' | cut -d: -f2)
        TIMESTAMP=$(echo "$STATS" | grep -oE '"timestamp":"[^"]+"' | cut -d'"' -f4)
    fi
    
    echo "Timestamp:     $TIMESTAMP"
    echo "Last Block:    $LAST_BLOCK"
    echo "Blocks:        $BLOCKS_PROCESSED processed"
    echo "Operations:    $OPS_PROCESSED processed"
    echo "Speed:         $BLOCKS_PER_SEC blocks/sec, $OPS_PER_SEC ops/sec"
    echo "Errors:        $ERRORS"
else
    echo -e "${YELLOW}No statistics found${NC}"
fi
echo ""

# Get latest block from database
echo -e "${BLUE}2. Database Status${NC}"
echo "----------------------------------------"
LATEST_DB_BLOCK=$(docker compose exec -T mongo mongosh steemdb --quiet --eval 'db.block_30d.find().sort({_id: -1}).limit(1).forEach(doc => print(doc._id))' 2>/dev/null | tail -1)
if [ -n "$LATEST_DB_BLOCK" ] && [ "$LATEST_DB_BLOCK" != "undefined" ]; then
    echo "Latest block in DB: $LATEST_DB_BLOCK"
    if [ -n "$LAST_BLOCK" ] && [ "$LAST_BLOCK" != "0" ]; then
        DIFF=$((LATEST_DB_BLOCK - LAST_BLOCK))
        if [ "$DIFF" -gt 0 ]; then
            echo -e "${YELLOW}⚠ Gap detected: DB has $DIFF more blocks than service reports${NC}"
        elif [ "$DIFF" -lt 0 ]; then
            echo -e "${YELLOW}⚠ Service reports higher block number than DB${NC}"
        else
            echo -e "${GREEN}✓ Block numbers match${NC}"
        fi
    fi
else
    echo -e "${YELLOW}Could not retrieve latest block from database${NC}"
fi
echo ""

# Calculate speed from recent statistics
echo -e "${BLUE}3. Speed Analysis (Last 10 Statistics)${NC}"
echo "----------------------------------------"
RECENT_STATS=$(docker compose logs --tail=2000 steemdb-sync 2>&1 | grep 'Block sync statistics' | tail -10)
if [ -n "$RECENT_STATS" ]; then
    echo "Recent blocks_per_second values:"
    echo "$RECENT_STATS" | grep -oE '"blocks_per_second":[0-9.]+' | cut -d: -f2 | while read speed; do
        if [ "$speed" = "0" ] || [ -z "$speed" ]; then
            echo -e "  ${YELLOW}$speed${NC}"
        else
            echo -e "  ${GREEN}$speed${NC}"
        fi
    done
    echo ""
    echo "Recent ops_per_second values:"
    echo "$RECENT_STATS" | grep -oE '"ops_per_second":[0-9.]+' | cut -d: -f2 | while read speed; do
        if [ "$speed" = "0" ] || [ -z "$speed" ]; then
            echo -e "  ${YELLOW}$speed${NC}"
        else
            echo -e "  ${GREEN}$speed${NC}"
        fi
    done
else
    echo -e "${YELLOW}No recent statistics found${NC}"
fi
echo ""

# Check for duplicate key errors (normal when syncing existing blocks)
echo -e "${BLUE}4. Error Analysis${NC}"
echo "----------------------------------------"
DUPLICATE_ERRORS=$(docker compose logs --tail=500 steemdb-sync 2>&1 | grep -c 'duplicate key')
OTHER_ERRORS=$(docker compose logs --tail=500 steemdb-sync 2>&1 | grep -i error | grep -v 'duplicate key' | grep -v 'multi-key map' | grep -v 'BadValue' | grep -v 'curation_rewards' | wc -l)
echo "Duplicate key errors (normal): $DUPLICATE_ERRORS"
echo "Other errors: $OTHER_ERRORS"
if [ "$OTHER_ERRORS" -gt 0 ]; then
    echo -e "${YELLOW}⚠ Found $OTHER_ERRORS non-duplicate errors${NC}"
fi
echo ""

# Check if service is processing new blocks
echo -e "${BLUE}5. Recent Activity${NC}"
echo "----------------------------------------"
RECENT_BLOCKS=$(docker compose logs --since 2m steemdb-sync 2>&1 | grep -oE 'block":[0-9]+' | cut -d: -f2 | sort -u | tail -5)
if [ -n "$RECENT_BLOCKS" ]; then
    echo "Recently processed blocks:"
    echo "$RECENT_BLOCKS" | while read block; do
        echo "  Block $block"
    done
else
    echo -e "${YELLOW}No recent block processing activity${NC}"
fi
echo ""

# Summary
echo "=========================================="
echo "Summary"
echo "=========================================="
if [ "$BLOCKS_PER_SEC" != "0" ] && [ -n "$BLOCKS_PER_SEC" ]; then
    echo -e "${GREEN}✓ Service is processing blocks at $BLOCKS_PER_SEC blocks/sec${NC}"
elif [ "$DUPLICATE_ERRORS" -gt 0 ]; then
    echo -e "${YELLOW}⚠ Service appears to be syncing existing blocks (duplicate key errors)${NC}"
    echo -e "${YELLOW}  This is normal if blocks already exist in the database${NC}"
else
    echo -e "${YELLOW}⚠ No active block processing detected${NC}"
fi

