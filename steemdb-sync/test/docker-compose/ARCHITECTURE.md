# Cold Ingest Docker Compose Test Architecture

## Overview

This Docker Compose setup provides a complete local testing environment for the `cold_ingest` service, simulating the production architecture in a containerized environment.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Compose Network                    │
│                  (cold-ingest-network)                       │
│                                                               │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────┐  │
│  │   MongoDB    │      │ cold_ingest  │      │  steemd   │  │
│  │              │◄─────┤              │◄─────┤          │  │
│  │  Port: 27017 │      │  Port: 8080  │      │  Plugin  │  │
│  │              │      │  Port: 9090  │      │          │  │
│  └──────────────┘      └──────────────┘      └──────────┘  │
│         ▲                      ▲                            │
│         │                      │                            │
│         └──────────────────────┘                            │
│              (Data Flow)                                      │
└─────────────────────────────────────────────────────────────┘
         │                      │                    │
         │                      │                    │
    ┌────▼────┐            ┌────▼────┐         ┌────▼────┐
    │  Host   │            │  Host   │         │  Host   │
    │  Port   │            │  Port   │         │  Data   │
    │  27017  │            │  8080   │         │  Volumes │
    └─────────┘            └─────────┘         └─────────┘
```

## Components

### 1. MongoDB Service

**Container**: `cold-ingest-mongo`  
**Image**: `mongo:4.4`  
**Purpose**: Stores blockchain operations and blocks

**Configuration**:
- Port: `27017` (configurable via `MONGO_PORT`)
- Authentication: Username/password (default: `admin/123456`)
- Database: `steemdb_test` (configurable via `MONGO_DATABASE`)
- Data Persistence: `mongo_data` volume

**Health Check**:
- Automatic ping check every 10 seconds
- 30-second startup grace period

### 2. Cold Ingest Service

**Container**: `cold-ingest-service`  
**Image**: Built from `Dockerfile.cold-ingest`  
**Purpose**: HTTP service that receives operations from steemd plugin

**Configuration**:
- HTTP API Port: `8080` (configurable via `INGEST_PORT`)
- Metrics Port: `9090` (configurable via `METRICS_PORT`)
- Config File: `/app/configs/config.yaml`
- MongoDB URI: Set via `MONGO_URI` environment variable

**Endpoints**:
- `POST /ingest/applied_op` - Single operation endpoint
- `POST /ingest/applied_ops` - Batch operations endpoint
- `GET /metrics` - Prometheus metrics

**Health Check**:
- Metrics endpoint check every 10 seconds
- 10-second startup grace period

### 3. Steemd Node (Optional)

**Container**: `cold-ingest-steemd`  
**Image**: `steemd:with-ingest` (configurable)  
**Purpose**: Steem blockchain node with ingest plugin

**Configuration**:
- Ingest Endpoint: `http://cold-ingest:8080/ingest/applied_ops`
- HTTP Timeout: `5000ms` (configurable)
- Queue Size: `100000` (configurable)
- Batch Size: `100` (configurable)
- Batch Timeout: `1000ms` (configurable)
- Data Directory: `/var/steem` (persisted in `steemd_data` volume)

**Behavior**:
- Runs `--replay-blockchain` mode
- Sends operations to `cold_ingest` via HTTP
- Exits automatically after replay completes (if `target_height` is set)

## Data Flow

### 1. Cold Start Workflow

```
steemd (replay)
    │
    ├─► applied_operation signal
    │   └─► ingest_plugin
    │       └─► Serialize to JSON
    │           └─► HTTP POST to cold_ingest
    │               └─► /ingest/applied_ops
    │                   └─► IngestHandler
    │                       └─► Batcher
    │                           └─► MongoDB BulkWrite
    │
    └─► applied_block signal (if no operations)
        └─► Block-only record
            └─► Same flow as above
```

### 2. Batch Processing

```
Operations Queue
    │
    ├─► Batch Size Threshold (1000 ops)
    │   └─► Flush to MongoDB
    │
    └─► Time Threshold (1 second)
        └─► Flush to MongoDB
```

### 3. ACK Mechanism

```
Client (steemd plugin)
    │
    ├─► HTTP POST /ingest/applied_ops
    │   └─► Server (cold_ingest)
    │       ├─► Decode JSON
    │       ├─► Add to batcher
    │       ├─► FlushOperationsAndBlocks (synchronous)
    │       │   └─► MongoDB write
    │       │       └─► Success?
    │       │           ├─► Yes: Return 200 OK
    │       │           └─► No: Return 500 Error
    │       │
    │       └─► Client receives response
    │           ├─► 200 OK: Continue
    │           └─► 500 Error: Retry (3s delay, max 5 attempts)
```

## Network Architecture

### Docker Network

All services are connected via a bridge network (`cold-ingest-network`):

- **Service Discovery**: Services can reach each other by container name
  - MongoDB: `mongo`
  - Cold Ingest: `cold-ingest`
  - Steemd: `steemd`

- **Port Mapping**: Host ports are mapped to container ports
  - MongoDB: `27017:27017`
  - Cold Ingest: `8080:8080`, `9090:9090`
  - Steemd: No external ports (internal only)

### Service Dependencies

```
mongo (no dependencies)
    │
    └─► cold-ingest (depends on: mongo health)
        │
        └─► steemd (depends on: cold-ingest health)
```

## Data Persistence

### Volumes

1. **mongo_data**: MongoDB database files
   - Location: Docker volume
   - Persists: Database, collections, indexes

2. **steemd_data**: Steemd blockchain data
   - Location: Docker volume
   - Persists: block_log, state, RocksDB

3. **ingest_data**: Cold ingest service data (if any)
   - Location: Docker volume
   - Currently unused, reserved for future use

### Host Mounts

1. **configs/**: Configuration files
   - Mounted as read-only
   - Contains `config.yaml`

2. **logs/**: Application logs
   - Mounted as read-write
   - Contains cold_ingest logs

3. **steemd-logs/**: Steemd logs
   - Mounted as read-write
   - Contains steemd application logs

## Configuration Management

### Environment Variables (.env)

Loaded by docker-compose and passed to containers:

```bash
# MongoDB
MONGO_USERNAME=admin
MONGO_PASSWORD=123456
MONGO_DATABASE=steemdb_test
MONGO_PORT=27017

# Cold Ingest
INGEST_PORT=8080
METRICS_PORT=9090

# Steemd
STEEMD_IMAGE=steemd:with-ingest
INGEST_ENDPOINT=http://cold-ingest:8080/ingest/applied_ops
INGEST_HTTP_TIMEOUT=5000
INGEST_QUEUE_SIZE=100000
INGEST_BATCH_SIZE=100
INGEST_BATCH_TIMEOUT=1000
```

### Service Configuration (configs/config.yaml)

Loaded by cold_ingest service:

```yaml
mongo:
  uri: "mongodb://admin:123456@mongo:27017/steemdb_test?authSource=admin"
  # Overridden by MONGO_URI environment variable

cold_start:
  target_height: 1000  # 0 = no limit
  safety_margin: 5

batch:
  size: 1000
  flush_interval: "1s"

ingest:
  listen_addr: ":8080"
  queue_size: 100000
```

**Note**: Environment variables override config file values (see `config.go`).

## Health Checks

### MongoDB

```bash
mongo --eval "db.adminCommand('ping')" \
  --username admin --password 123456 \
  --authenticationDatabase admin
```

- Interval: 10 seconds
- Timeout: 5 seconds
- Retries: 5
- Start Period: 30 seconds

### Cold Ingest

```bash
wget --quiet --tries=1 --spider http://localhost:8080/metrics
```

- Interval: 10 seconds
- Timeout: 5 seconds
- Retries: 5
- Start Period: 10 seconds

## Startup Sequence

1. **MongoDB starts** (no dependencies)
   - Initializes database
   - Waits for health check to pass

2. **Cold Ingest starts** (waits for MongoDB)
   - Builds Docker image (if needed)
   - Connects to MongoDB
   - Starts HTTP server
   - Waits for health check to pass

3. **Steemd starts** (waits for Cold Ingest)
   - Connects to cold_ingest endpoint
   - Starts blockchain replay
   - Sends operations via HTTP

## Shutdown Sequence

1. **Stop signal** (Ctrl+C or `docker-compose stop`)
   - Services receive SIGTERM

2. **Graceful shutdown**:
   - Cold Ingest: Flushes remaining batches, closes HTTP server
   - Steemd: Stops replay, closes connections
   - MongoDB: Closes connections, flushes data

3. **Container removal** (if using `docker-compose down`)
   - Containers are stopped and removed
   - Volumes remain (unless using `-v` flag)

## Cleanup

### Stop Only

```bash
./stop.sh
# or
docker-compose stop
```

- Containers are stopped but not removed
- Volumes remain intact
- Data is preserved

### Full Cleanup

```bash
./clean.sh
# or
docker-compose down -v
```

- Containers are stopped and removed
- Volumes are removed
- All data is deleted
- Log files are cleaned

## Testing Scenarios

### 1. Basic Test (MongoDB + cold_ingest)

```bash
docker-compose up -d mongo cold-ingest
```

Test with manual HTTP requests or mock data.

### 2. Full E2E Test (All Services)

```bash
./start.sh
```

Complete workflow with steemd replay.

### 3. Development Test

```bash
# Start only MongoDB
docker-compose up -d mongo

# Run cold_ingest locally (outside Docker)
go run ./cmd/cold_ingest -config test/docker-compose/configs/config.yaml
```

## Monitoring

### Metrics

Access Prometheus metrics:

```bash
curl http://localhost:9090/metrics
```

Key metrics:
- `ingest_operations_total` - Total operations received
- `ingest_batches_total` - Total batches processed
- `ingest_queue_size` - Current queue size
- `ingest_tps` - Operations per second

### Logs

View service logs:

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f cold-ingest
docker-compose logs -f steemd
docker-compose logs -f mongo
```

### Database

Query MongoDB:

```bash
docker-compose exec mongo mongo steemdb_test \
  --username admin --password 123456 \
  --authenticationDatabase admin

> db.operations.count()
> db.blocks.count()
> db.meta.findOne()
```

## Troubleshooting

### Service Not Starting

1. Check logs: `docker-compose logs [service]`
2. Check health: `docker-compose ps`
3. Verify network: `docker network inspect cold-ingest-network`
4. Check ports: `netstat -tuln | grep [port]`

### Connection Issues

1. Verify service names (use container names, not localhost)
2. Check network connectivity: `docker-compose exec [service] ping [target]`
3. Verify environment variables: `docker-compose exec [service] env`

### Data Issues

1. Check MongoDB: `docker-compose exec mongo mongo ...`
2. Verify volumes: `docker volume ls`
3. Check data persistence: Restart services and verify data remains

## Best Practices

1. **Use volumes for data persistence**: Don't rely on container filesystem
2. **Set appropriate timeouts**: Adjust based on your hardware
3. **Monitor resource usage**: Use `docker stats` to monitor CPU/memory
4. **Clean up regularly**: Use `./clean.sh` to reset test environment
5. **Use health checks**: Wait for services to be healthy before testing
6. **Check logs first**: Most issues are visible in logs

## Security Notes

⚠️ **This setup is for testing only!**

- Default passwords are weak (`admin/123456`)
- Services are exposed on host ports
- No TLS/SSL encryption
- No firewall rules

For production, use:
- Strong passwords
- TLS certificates
- Firewall rules
- Network isolation
- Secrets management
