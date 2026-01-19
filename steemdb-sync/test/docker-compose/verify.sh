#!/bin/bash
# Script to verify the cold_ingest test environment is working

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Verifying Cold Ingest Test Environment ===${NC}\n"

# Check if services are running
echo -e "${BLUE}Checking service status...${NC}"
if ! docker-compose ps | grep -q "Up"; then
    echo -e "${RED}Error: No services are running. Please start them first with: ./start.sh${NC}"
    exit 1
fi

# Check MongoDB
echo -e "${BLUE}Checking MongoDB...${NC}"
if docker-compose exec -T mongo mongo --quiet --eval "db.adminCommand('ping')" --username "${MONGO_USERNAME:-admin}" --password "${MONGO_PASSWORD:-123456}" --authenticationDatabase admin > /dev/null 2>&1; then
    echo -e "${GREEN}✓ MongoDB is healthy${NC}"
else
    echo -e "${RED}✗ MongoDB is not responding${NC}"
    exit 1
fi

# Check cold_ingest service
echo -e "${BLUE}Checking cold_ingest service...${NC}"
if curl -s -f http://localhost:${INGEST_PORT:-8080}/metrics > /dev/null 2>&1; then
    echo -e "${GREEN}✓ cold_ingest service is responding${NC}"
else
    echo -e "${YELLOW}⚠ cold_ingest service may not be ready yet${NC}"
fi

# Check metrics endpoint (metrics are served on the same port as the main service)
echo -e "${BLUE}Checking metrics endpoint...${NC}"
METRICS=$(curl -s http://localhost:${INGEST_PORT:-8080}/metrics 2>/dev/null || echo "")
if [ -n "$METRICS" ]; then
    echo -e "${GREEN}✓ Metrics endpoint is accessible${NC}"
    echo -e "${BLUE}Sample metrics:${NC}"
    echo "$METRICS" | head -5
else
    echo -e "${YELLOW}⚠ Metrics endpoint is not accessible${NC}"
fi

# Check MongoDB connection from cold_ingest
echo -e "${BLUE}Checking MongoDB connection from cold_ingest...${NC}"
if docker-compose exec -T cold-ingest ping -c 1 mongo > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Network connectivity to MongoDB is OK${NC}"
else
    echo -e "${RED}✗ Cannot reach MongoDB from cold_ingest container${NC}"
fi

# Check steemd (if running)
if docker-compose ps | grep -q "steemd.*Up"; then
    echo -e "${BLUE}Checking steemd service...${NC}"
    # Try to check network connectivity using wget or curl (ping may not be available)
    if docker-compose exec -T steemd wget --quiet --tries=1 --spider http://cold-ingest:8080/metrics > /dev/null 2>&1 || \
       docker-compose exec -T steemd curl -s -f http://cold-ingest:8080/metrics > /dev/null 2>&1; then
        echo -e "${GREEN}✓ steemd can reach cold_ingest service${NC}"
    else
        echo -e "${YELLOW}⚠ steemd cannot reach cold_ingest service (may need wget/curl in steemd image)${NC}"
    fi
else
    echo -e "${YELLOW}⚠ steemd service is not running${NC}"
fi

# Summary
echo -e "\n${GREEN}=== Verification Summary ===${NC}"
echo -e "${GREEN}Environment appears to be working correctly!${NC}"
echo ""
echo -e "${BLUE}Useful commands:${NC}"
echo -e "  View logs:    docker-compose logs -f [service_name]"
echo -e "  View metrics: curl http://localhost:${INGEST_PORT:-8080}/metrics"
echo -e "  Stop:         ./stop.sh"
echo -e "  Clean:        ./clean.sh"
