# Cold Ingest Docker Compose Test Environment

Complete local testing environment for `cold_ingest` service using Docker Compose.

## Overview

This directory provides a complete Docker Compose setup for testing the `cold_ingest` service locally. It includes:

- **MongoDB**: Database for storing blockchain data
- **cold_ingest**: HTTP service that receives operations from steemd plugin
- **steemd**: Steem blockchain node with ingest plugin (optional)

## Prerequisites

1. **Docker** and **Docker Compose** installed
2. **steemd Docker image** with ingest plugin (optional, for full E2E test):
   ```bash
   cd /path/to/steem
   docker build -f deploy/Dockerfile.ubuntu24.04 -t steemd:with-ingest .
   ```
3. **Blockchain data** (optional, for steemd replay):
   - Place `block_log` file in `steemd_data/blockchain/` directory
   - Or mount your existing blockchain data directory

## Quick Start

### 1. Initial Setup

```bash
cd test/docker-compose

# Copy environment template
cp .env.example .env

# Copy config template
mkdir -p configs
cp configs/config.yaml.template configs/config.yaml

# Edit config.yaml and set target_height (e.g., 1000 for quick test)
# Edit .env if you need to change default values
```

### 2. Start Services

```bash
# Make scripts executable
chmod +x *.sh

# Start all services
./start.sh
```

The script will:
- Check prerequisites
- Create necessary directories
- Start MongoDB, cold_ingest, and steemd services
- Wait for services to be healthy
- Display status and useful commands

### 3. Monitor Services

```bash
# View all logs
docker-compose logs -f

# View specific service logs
docker-compose logs -f cold-ingest
docker-compose logs -f steemd
docker-compose logs -f mongo

# View metrics
curl http://localhost:9090/metrics

# Check service status
docker-compose ps
```

### 4. Verify Environment

```bash
# Verify all services are working correctly
./verify.sh
```

This script checks:
- Service status
- MongoDB health
- cold_ingest service availability
- Metrics endpoint
- Network connectivity

### 5. Stop Services

```bash
# Stop services (containers remain)
./stop.sh

# Or use docker-compose directly
docker-compose stop
```

### 6. Clean All Data

```bash
# Remove containers, volumes, and data
./clean.sh
```

This will:
- Stop and remove all containers
- Remove all volumes (MongoDB data, steemd data)
- Clean log files
- Reset to a clean state

## Configuration

### Environment Variables (.env)

```bash
# MongoDB settings
MONGO_USERNAME=admin
MONGO_PASSWORD=123456
MONGO_DATABASE=steemdb_test
MONGO_PORT=27017

# Cold Ingest service
INGEST_PORT=8080
METRICS_PORT=9090

# Steemd settings
STEEMD_IMAGE=steemd:with-ingest
INGEST_ENDPOINT=http://cold-ingest:8080/ingest/applied_ops
INGEST_HTTP_TIMEOUT=5000
INGEST_QUEUE_SIZE=100000
INGEST_BATCH_SIZE=100
INGEST_BATCH_TIMEOUT=1000
# Stop replay at specific block number (0 = no limit, replay all blocks)
# Useful for testing with a limited block range
STOP_REPLAY_AT_BLOCK=0
```

### Service Configuration (configs/config.yaml)

Key settings:

```yaml
cold_start:
  target_height: 1000      # Target block height (0 = no limit)
  safety_margin: 5          # Safety margin in blocks

batch:
  size: 1000               # Operations per batch
  flush_interval: "1s"      # Batch flush interval

ingest:
  listen_addr: ":8080"      # HTTP server address
  queue_size: 100000        # Max queue size
```

## Services

### MongoDB

- **Container**: `cold-ingest-mongo`
- **Port**: `27017` (configurable via `MONGO_PORT`)
- **Data**: Persisted in `mongo_data` volume
- **Health Check**: Automatic ping check

**Connection String**:
```
mongodb://admin:123456@localhost:27017/steemdb_test?authSource=admin
```

### Cold Ingest Service

- **Container**: `cold-ingest-service`
- **Ports**: 
  - `8080` - HTTP API
  - `9090` - Metrics endpoint
- **Health Check**: Metrics endpoint check
- **Logs**: `./logs/` directory

**Endpoints**:
- `POST /ingest/applied_op` - Single operation
- `POST /ingest/applied_ops` - Batch operations
- `GET /metrics` - Prometheus metrics

### Steemd Node

- **Container**: `cold-ingest-steemd`
- **Image**: `steemd:with-ingest` (configurable)
- **Data**: Persisted in `steemd_data` volume
- **Logs**: `./steemd-logs/` directory

**Configuration**:
- `STOP_REPLAY_AT_BLOCK`: Stop replay at specific block number (0 = no limit)
  - Set in `.env` file: `STOP_REPLAY_AT_BLOCK=1000` to stop at block 1000
  - Useful for quick testing with limited block range
  - Works independently of `cold_start.target_height` in config.yaml

**Note**: Steemd will automatically exit after replay completes (if `target_height` is set in config.yaml or `STOP_REPLAY_AT_BLOCK` is set in .env).

## Testing Workflow

### 1. Basic Test (MongoDB + cold_ingest only)

```bash
# Start only MongoDB and cold_ingest
docker-compose up -d mongo cold-ingest

# Test with mock data or manual HTTP requests
curl -X POST http://localhost:8080/ingest/applied_op \
  -H "Content-Type: application/json" \
  -d '{"block": {"num": 1, "id": "..."}, ...}'
```

### 2. Full E2E Test (with steemd)

```bash
# Start all services
./start.sh

# Monitor progress
docker-compose logs -f steemd cold-ingest

# Check MongoDB data
docker-compose exec mongo mongo steemdb_test \
  --username admin --password 123456 --authenticationDatabase admin \
  --eval "db.operations.count()"
```

### 3. Verify Data

```bash
# Connect to MongoDB
docker-compose exec mongo mongo steemdb_test \
  --username admin --password 123456 --authenticationDatabase admin

# Check collections
> show collections
> db.operations.count()
> db.blocks.count()
> db.meta.findOne()

# Query operations
> db.operations.find().limit(5).pretty()
> db.blocks.find().sort({block_num: -1}).limit(5).pretty()
```

## Troubleshooting

### Services Not Starting

```bash
# Check logs
docker-compose logs

# Check service status
docker-compose ps

# Restart services
docker-compose restart
```

### MongoDB Connection Issues

```bash
# Check MongoDB health
docker-compose exec mongo mongo --eval "db.adminCommand('ping')" \
  --username admin --password 123456 --authenticationDatabase admin

# Check network connectivity
docker-compose exec cold-ingest ping mongo
```

### Cold Ingest Not Receiving Data

```bash
# Check if service is listening
curl http://localhost:8080/metrics

# Check steemd logs
docker-compose logs steemd

# Verify endpoint configuration
docker-compose exec steemd env | grep INGEST
```

### Port Conflicts

If ports are already in use, modify `.env`:

```bash
MONGO_PORT=27018
INGEST_PORT=8081
METRICS_PORT=9091
```

## Data Persistence

### Volumes

- `mongo_data`: MongoDB database files
- `steemd_data`: Steemd blockchain data and state
- `ingest_data`: Cold ingest service data (if any)

### Clean Start

To start completely fresh:

```bash
./clean.sh
./start.sh
```

## Advanced Usage

### Custom steemd Image

```bash
# Build custom image
cd /path/to/steem
docker build -f deploy/Dockerfile.ubuntu24.04 -t my-steemd:custom .

# Update .env
STEEMD_IMAGE=my-steemd:custom
```

### Mount Custom Blockchain Data

Edit `docker-compose.yml`:

```yaml
steemd:
  volumes:
    - /path/to/your/blockchain:/var/steem/blockchain:ro
    - steemd_data:/var/steem
```

### Run Without steemd

```bash
# Start only MongoDB and cold_ingest
docker-compose up -d mongo cold-ingest

# Test with manual HTTP requests or mock data
```

### Development Mode

For development, you can mount source code:

```yaml
cold-ingest:
  volumes:
    - ../../:/app/src:ro
    - ./configs:/app/configs:ro
```

Then rebuild and restart:

```bash
docker-compose build cold-ingest
docker-compose up -d cold-ingest
```

## Scripts

- **start.sh**: Start all services and wait for health checks
- **stop.sh**: Stop all services (containers remain)
- **clean.sh**: Remove all containers, volumes, and data

## Network

All services are connected via `cold-ingest-network` bridge network.

- MongoDB: `mongo` (hostname)
- Cold Ingest: `cold-ingest` (hostname)
- Steemd: `steemd` (hostname)

## Monitoring

### Metrics Endpoint

```bash
# View Prometheus metrics
curl http://localhost:9090/metrics

# Key metrics:
# - ingest_operations_total
# - ingest_batches_total
# - ingest_queue_size
# - ingest_tps
```

### Logs

```bash
# Follow all logs
docker-compose logs -f

# Follow specific service
docker-compose logs -f cold-ingest

# View last 100 lines
docker-compose logs --tail=100 cold-ingest
```

## Cleanup

### Stop and Remove Everything

```bash
./clean.sh
```

### Keep Data, Remove Containers Only

```bash
docker-compose down
```

### Remove Only Volumes

```bash
docker-compose down -v
```

## Notes

- **First Run**: MongoDB initialization may take 30-60 seconds
- **Steemd Replay**: Can take hours depending on blockchain size and target_height
- **Resource Usage**: Ensure sufficient disk space for MongoDB and steemd data
- **Network**: Services communicate via Docker network, not localhost

## Support

For issues or questions:
1. Check logs: `docker-compose logs`
2. Check service status: `docker-compose ps`
3. Verify configuration: Review `.env` and `configs/config.yaml`
4. Check network: `docker network inspect cold-ingest-network`
