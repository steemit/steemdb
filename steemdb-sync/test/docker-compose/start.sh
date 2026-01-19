#!/bin/bash
# Script to start the cold_ingest test environment

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Starting Cold Ingest Test Environment ===${NC}\n"

# Check if .env file exists
if [ ! -f .env ]; then
    echo -e "${YELLOW}Warning: .env file not found. Creating from env.example...${NC}"
    if [ -f env.example ]; then
        cp env.example .env
        echo -e "${GREEN}Created .env file. Please review and adjust if needed.${NC}"
    else
        echo -e "${RED}Error: env.example not found. Please create .env manually.${NC}"
        exit 1
    fi
fi

# Check if config.yaml exists
if [ ! -f configs/config.yaml ]; then
    echo -e "${YELLOW}Warning: configs/config.yaml not found. Creating from template...${NC}"
    if [ -f configs/config.yaml.template ]; then
        mkdir -p configs
        cp configs/config.yaml.template configs/config.yaml
        echo -e "${GREEN}Created config.yaml. Please review and adjust target_height if needed.${NC}"
    else
        echo -e "${RED}Error: config.yaml.template not found.${NC}"
        exit 1
    fi
fi

# Create necessary directories
mkdir -p logs steemd-logs

# Check if steemd image exists
source .env
if ! docker images | grep -q "${STEEMD_IMAGE:-steemd:with-ingest}"; then
    echo -e "${YELLOW}Warning: Docker image ${STEEMD_IMAGE:-steemd:with-ingest} not found.${NC}"
    echo -e "${YELLOW}Please build the steemd image first:${NC}"
    echo -e "${BLUE}  cd /path/to/steem${NC}"
    echo -e "${BLUE}  docker build -f deploy/Dockerfile.ubuntu24.04 -t ${STEEMD_IMAGE:-steemd:with-ingest} .${NC}"
    echo ""
    read -p "Continue without steemd? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Start services
echo -e "${GREEN}Starting services with docker-compose...${NC}"
docker-compose up -d

# Wait for services to be healthy
echo -e "${GREEN}Waiting for services to be ready...${NC}"
echo -e "${BLUE}Checking MongoDB...${NC}"
timeout=60
elapsed=0
while [ $elapsed -lt $timeout ]; do
    if docker-compose exec -T mongo mongo --quiet --eval "db.adminCommand('ping')" --username "${MONGO_USERNAME:-admin}" --password "${MONGO_PASSWORD:-123456}" --authenticationDatabase admin > /dev/null 2>&1; then
        echo -e "${GREEN}MongoDB is ready!${NC}"
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
    echo -n "."
done
echo ""

if [ $elapsed -ge $timeout ]; then
    echo -e "${RED}MongoDB failed to start within ${timeout} seconds${NC}"
    docker-compose logs mongo
    exit 1
fi

echo -e "${BLUE}Checking cold_ingest service...${NC}"
timeout=30
elapsed=0
while [ $elapsed -lt $timeout ]; do
    if docker-compose exec -T cold-ingest wget --quiet --tries=1 --spider http://localhost:8080/metrics > /dev/null 2>&1; then
        echo -e "${GREEN}cold_ingest service is ready!${NC}"
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
    echo -n "."
done
echo ""

if [ $elapsed -ge $timeout ]; then
    echo -e "${YELLOW}cold_ingest service may not be ready yet. Check logs:${NC}"
    echo -e "${BLUE}  docker-compose logs cold-ingest${NC}"
fi

# Show status
echo -e "\n${GREEN}=== Services Status ===${NC}"
docker-compose ps

echo -e "\n${GREEN}=== Useful Commands ===${NC}"
echo -e "${BLUE}View logs:${NC}"
echo -e "  docker-compose logs -f [service_name]"
echo -e "  docker-compose logs -f cold-ingest"
echo -e "  docker-compose logs -f steemd"
echo -e "  docker-compose logs -f mongo"
echo ""
echo -e "${BLUE}View metrics:${NC}"
echo -e "  curl http://localhost:${METRICS_PORT:-9090}/metrics"
echo ""
echo -e "${BLUE}Stop services:${NC}"
echo -e "  ./stop.sh"
echo ""
echo -e "${BLUE}Clean data:${NC}"
echo -e "  ./clean.sh"
echo ""
echo -e "${GREEN}=== Test Environment Started ===${NC}"
