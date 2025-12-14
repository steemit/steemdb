# SteemDB Sync - Cold Start Validation and Performance Testing Guide

## Overview

This guide provides instructions for validating the cold start process and testing performance of the SteemDB Sync service.

## Quick Start

### Option A: Using Docker Test Environment (Recommended)

The easiest way to run tests is using the Docker test environment, which automatically creates and cleans up a temporary MongoDB instance:

```bash
cd steemdb-sync

# Run all tests with Docker
./scripts/run_tests_with_docker.sh

# Or run specific tests
./scripts/run_tests_with_docker.sh validation   # Cold start validation only
./scripts/run_tests_with_docker.sh performance # Performance tests only
```

This will:
- ✅ Automatically start a temporary MongoDB container
- ✅ Run the tests
- ✅ Clean up containers and volumes when done

### Option B: Using Existing MongoDB

If you have MongoDB running locally, you can run tests directly:

```bash
cd steemdb-sync
./scripts/cold_start_validation.sh
```

### 1. Cold Start Validation

Before starting the sync service for the first time, run the validation script:

**With Docker (recommended):**
```bash
./scripts/run_tests_with_docker.sh validation
```

**Without Docker:**
```bash
./scripts/cold_start_validation.sh
```

This script checks:
- ✅ MongoDB connection
- ✅ Sync binary exists
- ✅ Configuration file exists
- ✅ Initial database state (ready for cold start)
- ✅ Database indexes (will be created automatically)

### 2. Start Sync Service

After validation passes, start the sync service:

```bash
./bin/steemdb-sync ./configs/config.yaml
```

Monitor the logs to verify:
- Service starts from block 1
- Indexes are created automatically
- Blocks are being processed

### 3. Performance Testing

After the service has processed some blocks (recommended: 1000+ blocks), run performance tests:

**With Docker (recommended):**
```bash
./scripts/run_tests_with_docker.sh performance
```

**Without Docker:**
```bash
# Shell-based performance test
./scripts/performance_test.sh

# Or use the Go-based performance test tool
go build -o bin/validate ./cmd/validate
./bin/validate ./configs/config.yaml
```

## Docker Test Environment

### Overview

The Docker test environment provides an isolated MongoDB instance for testing, ensuring:
- ✅ No interference with existing databases
- ✅ Consistent test environment
- ✅ Automatic cleanup after tests

### Configuration

The test environment uses `docker-compose.test.yml` which:
- Creates a MongoDB 7.0 container
- Uses port 27018 (to avoid conflicts with main MongoDB on 27017)
- Creates a named volume for data (automatically removed on cleanup)
- Sets up health checks

### Usage

**Automatic (recommended):**
```bash
# Run all tests
./scripts/run_tests_with_docker.sh

# Run specific test
./scripts/run_tests_with_docker.sh validation
./scripts/run_tests_with_docker.sh performance
```

**Manual:**
```bash
# Start test environment
docker-compose -f docker-compose.test.yml up -d

# Run tests with USE_DOCKER=true
USE_DOCKER=true ./scripts/cold_start_validation.sh
USE_DOCKER=true ./scripts/performance_test.sh

# Cleanup
docker-compose -f docker-compose.test.yml down -v
```

### Environment Variables

When using Docker test environment, scripts automatically use:
- `MONGODB_URI=mongodb://localhost:27018` (test port)
- `MONGODB_DB=steemdb`

You can override these if needed:
```bash
USE_DOCKER=true MONGODB_URI=mongodb://localhost:27018 ./scripts/cold_start_validation.sh
```

### Cleanup

The test scripts automatically clean up Docker containers and volumes on exit. If you need to manually clean up:

```bash
# Stop and remove containers and volumes
docker-compose -f docker-compose.test.yml down -v

# Remove orphaned containers (if any)
docker ps -a | grep steemdb-sync-test | awk '{print $1}' | xargs docker rm -f

# Remove test volumes (if any)
docker volume ls | grep steemdb-sync-test | awk '{print $2}' | xargs docker volume rm
```

## Detailed Instructions

### Cold Start Validation

The cold start validation script (`scripts/cold_start_validation.sh`) ensures the system is ready for a fresh start.

**Prerequisites:**
- MongoDB running and accessible
- Sync binary built (`./bin/steemdb-sync`)
- Configuration file exists (`./configs/config.yaml`)

**Environment Variables:**
```bash
export MONGODB_URI="mongodb://localhost:27017"
export MONGODB_DB="steemdb"
export SYNC_BINARY="./bin/steemdb-sync"
export CONFIG_FILE="./configs/config.yaml"
```

**Expected Output:**
```
✓ MongoDB Connection: Connected successfully
✓ Sync Binary: Found at ./bin/steemdb-sync
✓ Config File: Found at ./configs/config.yaml
✓ Initial State Check: No height record found (cold start ready)
⚠ Indexes: No indexes found (will be created on first run)
```

### Performance Testing

#### Shell Script (`scripts/performance_test.sh`)

Tests query performance using MongoDB shell commands.

**Tests:**
- Block query by number (target: < 10ms)
- Latest blocks query
- Account query by name (target: < 50ms)
- Account operations query (target: < 100ms)
- Database statistics
- Index verification

**Usage:**
```bash
export MONGODB_URI="mongodb://localhost:27017"
export MONGODB_DB="steemdb"
export METRICS_URL="http://localhost:9091/metrics"

./scripts/performance_test.sh
```

#### Go Tool (`cmd/validate/main.go`)

More comprehensive performance testing using Go.

**Tests:**
- Block query by number
- Latest blocks query
- Account query by name
- Account operations query
- Operations query by type
- Comments query by author

**Usage:**
```bash
# Build the tool
go build -o bin/validate ./cmd/validate

# Run tests
./bin/validate ./configs/config.yaml
```

**Output:**
```
✓ Block Query (by number): 2.5ms (target: < 10ms)
✓ Latest Blocks Query: 15ms
✓ Account Query (by name): 8ms (target: < 50ms)
✓ Account Operations Query: 45ms (target: < 100ms)
...
```

## Performance Targets

Based on the architecture plan, the following performance targets should be met:

| Metric | Target | Description |
|--------|--------|-------------|
| Block Processing | 500+ blocks/sec | Blocks processed per second |
| Operation Processing | 5000+ ops/sec | Operations processed per second |
| Block Query | < 10ms | Query block by number |
| Account Query | < 50ms | Query account by name |
| Account History | < 100ms | Query account operations (100 records) |
| Latest Blocks | < 50ms | Query latest 20 blocks |

## Monitoring

### Prometheus Metrics

Monitor performance metrics at:
```
http://localhost:9091/metrics
```

**Key Metrics:**
- `steemdb_blocks_processed_total` - Total blocks processed
- `steemdb_operations_processed_total` - Total operations processed
- `steemdb_current_block` - Current block number
- `steemdb_processing_duration_seconds` - Processing time
- `steemdb_batch_write_duration_seconds` - Batch write time

### Health Checks

- Health endpoint: `http://localhost:9091/health`
- Readiness endpoint: `http://localhost:9091/ready`

## Troubleshooting

### Validation Fails

**MongoDB Connection Failed:**
```bash
# Check if MongoDB is running
docker-compose ps mongo

# Check connection string
echo $MONGODB_URI
```

**Binary Not Found:**
```bash
# Build the binary
go build -o bin/steemdb-sync ./cmd/sync
```

### Performance Targets Not Met

**Slow Queries:**
1. Check if indexes are created:
   ```bash
   mongosh $MONGODB_URI/$MONGODB_DB --eval "db.blocks.getIndexes()"
   ```

2. Verify MongoDB resources (CPU, memory, disk I/O)

3. Check query execution plans:
   ```bash
   mongosh $MONGODB_URI/$MONGODB_DB --eval "db.blocks.find({number: 1}).explain('executionStats')"
   ```

**Low Processing Speed:**
1. Check sync service logs for errors
2. Verify Steem RPC nodes are responsive
3. Check MongoDB write performance
4. Monitor system resources (CPU, memory, network)

## Next Steps

After successful validation and performance testing:

1. ✅ Cold start validation passed
2. ✅ Performance tests passed
3. ✅ System ready for production
4. 📊 Set up Grafana dashboards
5. 🔔 Configure alerting rules
6. 📝 Document operational runbooks

## Additional Resources

- [Architecture Plan](../.cursor/plans/steemdb_sync_架构重构与查询优化.plan.md)
- [Progress Analysis](../.cursor/plans/steemdb_sync_进度分析.md)
- [Scripts README](./scripts/README.md)
