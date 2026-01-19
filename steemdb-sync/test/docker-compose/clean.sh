#!/bin/bash
# Script to clean all data from the cold_ingest test environment

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${RED}=== Cleaning Cold Ingest Test Environment ===${NC}\n"

# Confirm action
echo -e "${YELLOW}This will:${NC}"
echo -e "  - Stop and remove all containers"
echo -e "  - Remove all volumes (MongoDB data, steemd data, etc.)"
echo -e "  - Remove all networks"
echo -e "  - Clean log files"
echo -e "  - Clean RocksDB files (steemd state database)"
echo ""
read -p "Are you sure you want to continue? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${GREEN}Cancelled.${NC}"
    exit 0
fi

# Stop and remove containers
echo -e "${GREEN}Stopping and removing containers...${NC}"
docker-compose down -v

# Remove volumes (if any remain)
echo -e "${GREEN}Removing volumes...${NC}"
docker volume ls | grep -E "cold-ingest|steemdb-sync" | awk '{print $2}' | xargs -r docker volume rm || true

# Clean log files
echo -e "${GREEN}Cleaning log files...${NC}"
rm -rf logs/* steemd-logs/* 2>/dev/null || true

# Clean temporary files
echo -e "${GREEN}Cleaning temporary files...${NC}"
find . -name "*.log" -type f -delete 2>/dev/null || true

# Clean RocksDB files (steemd state database)
# This matches the cleanup logic in test/steem-test/run.sh
STEEM_DATA_DIR="../steem-test/data"
if [ -d "$STEEM_DATA_DIR/blockchain" ]; then
    echo -e "${GREEN}Cleaning RocksDB files...${NC}"
    if sudo rm -rf "$STEEM_DATA_DIR/blockchain/rocksdb_"* 2>/dev/null; then
        echo -e "${GREEN}RocksDB files cleaned${NC}"
    else
        echo -e "${YELLOW}No RocksDB files to clean or cleanup failed (this is OK)${NC}"
    fi
fi

echo -e "\n${GREEN}=== Cleanup Complete ===${NC}"
echo -e "${YELLOW}All data has been removed. You can start fresh with: ./start.sh${NC}"
