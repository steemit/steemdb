#!/bin/bash
# Script to stop the cold_ingest test environment

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Stopping Cold Ingest Test Environment ===${NC}\n"

# Stop services
echo -e "${GREEN}Stopping services...${NC}"
docker-compose stop

echo -e "${GREEN}=== Services Stopped ===${NC}"
echo -e "${YELLOW}Note: Containers are stopped but not removed.${NC}"
echo -e "${YELLOW}To remove containers, use: docker-compose down${NC}"
echo -e "${YELLOW}To clean all data, use: ./clean.sh${NC}"
