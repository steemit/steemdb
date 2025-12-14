# SteemDB Sync Service

A high-performance Go service that synchronizes Steem blockchain data to MongoDB, replacing the original Python services (sync, history, witnesses).

## Features

- **Unified Service**: Combines block sync, account history, and witness monitoring in a single Go service
- **High Performance**: 3-5x faster than the original Python implementation
- **Reliable**: Comprehensive error handling and automatic recovery
- **Scalable**: Single goroutine architecture for sequential processing (ensures operation order)
- **Monitored**: Built-in Prometheus metrics and health checks
- **Containerized**: Docker and Docker Compose support

## Architecture

```
SteemDB Sync Service
├── Block Sync Module      # Real-time blockchain synchronization
├── History Module         # Account history data collection
├── Witnesses Module       # Witness monitoring and miss tracking
├── Operation Processor    # Handles 15+ operation types
├── Task Scheduler        # Cron-based job scheduling
└── Metrics Server        # Prometheus metrics endpoint
```

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.23+ (for development)
- MongoDB 6.0+
- Redis 7+

### Using Docker Compose (Recommended)

The sync service is part of the unified Docker Compose setup at the project root.

1. Navigate to the project root:
```bash
cd /path/to/steemdb
```

2. Configure the service (optional):
```bash
# Edit sync service configuration
vim steemdb-sync/configs/config.yaml
```

3. Start all services:
```bash
docker-compose up -d
```

4. Monitor logs:
```bash
# View all logs
docker-compose logs -f

# View sync service logs only
docker-compose logs -f steemdb-sync
```

**Note**: The unified `docker-compose.yml` at the project root orchestrates all services including MongoDB, Redis, Prometheus, and Grafana.

### Manual Setup

1. Install dependencies:
```bash
go mod download
```

2. Configure MongoDB and Redis connections in `configs/config.yaml`

3. Build and run:
```bash
mkdir -p ../bin
go build -o ../bin/steemdb-sync cmd/sync/main.go
../bin/steemdb-sync configs/config.yaml
```

## Configuration

The service is configured via `configs/config.yaml`:

```yaml
server:
  port: 9090
  mode: "production"

steem:
  nodes:
    - "https://api.steemit.com"
  timeout: 30s

mongodb:
  uri: "mongodb://localhost:27017"
  database: "steemdb"
  pool_size: 100

sync:
  batch_size: 50
  workers: 10
  block_interval: 3s

history:
  interval: 6h
  batch_size: 50
  workers: 5

witnesses:
  interval: 1m
  check_interval: 10s

log:
  level: "info"        # debug, info, warn, error
  format: "json"       # json, text
  file: "logs/steemdb-sync.log"  # Log file path (see Logging section below)
  max_size: 100        # Maximum log file size in MB
  max_backups: 5       # Number of backup log files to keep
  max_age: 30          # Maximum age of log files in days
```

### Logging Configuration

The service supports flexible logging configuration with automatic log rotation and graceful error handling.

#### Log File Path Options

**1. Relative Path (Recommended for Development)**
```yaml
log:
  file: "logs/steemdb-sync.log"  # Relative to current working directory
```
- The log directory will be automatically created if it doesn't exist
- Path is resolved relative to the directory where the service is run

**2. Absolute Path (Recommended for Production)**
```yaml
log:
  file: "/var/log/steemdb-sync.log"  # Absolute path
```
- Requires appropriate write permissions
- Useful for system-wide logging locations

**3. Console Output Only**
```yaml
log:
  file: ""  # Empty string disables file logging
```
- All logs will be output to stdout/stderr only
- Useful for containerized deployments where logs are captured by the container runtime

#### Log Rotation

The service automatically rotates log files when they reach the configured size:
- **max_size**: Maximum size per log file (in MB)
- **max_backups**: Number of rotated log files to keep
- **max_age**: Maximum age of log files before deletion (in days)
- Old log files are automatically compressed (`.gz` format)

#### Error Handling

If the service cannot write to the specified log file (e.g., permission denied):
- A warning message is displayed on stderr
- The service automatically falls back to console output only
- The service continues running normally (no crash)

**Example warning:**
```
Warning: cannot write to log file /var/log/steemdb-sync.log: permission denied, using console output only
```

#### Log Format Options

- **json**: Structured JSON format (recommended for production, easier to parse)
- **text**: Human-readable text format (recommended for development)

#### Log Levels

- **debug**: Detailed debugging information
- **info**: General informational messages (default)
- **warn**: Warning messages
- **error**: Error messages only

#### Examples

**Development Configuration:**
```yaml
log:
  level: "debug"
  format: "text"
  file: "logs/steemdb-sync.log"
  max_size: 10
  max_backups: 3
  max_age: 7
```

**Production Configuration:**
```yaml
log:
  level: "info"
  format: "json"
  file: "/var/log/steemdb-sync.log"
  max_size: 500
  max_backups: 10
  max_age: 30
```

**Docker/Container Configuration:**
```yaml
log:
  level: "info"
  format: "json"
  file: ""  # Let Docker capture stdout/stderr
  max_size: 100
  max_backups: 5
  max_age: 30
```

## Services Overview

### Block Sync Service
- **Single goroutine architecture**: Sequential processing ensures operation order correctness
- Synchronizes blockchain data in real-time
- Processes 15+ operation types (votes, comments, transfers, rewards, etc.)
- Handles 200-500 blocks/second
- Automatic error recovery and retry logic
- **Account update marking**: Marks accounts for update instead of calculating balances

### CronTab Service
- **Single goroutine**: Waits for Block Sync to catch up before starting
- **Account Updater**: Batch updates accounts using `condenser_api.get_accounts` (every 6 hours)
- **Stats Updater**: Updates hourly operation statistics
- **30-day Aggregations**: Calculates 30-day aggregated data (daily)

### History Service
- Scans all accounts every 6 hours
- Creates daily account snapshots
- Updates fund history hourly
- Batch processing for efficiency

### Witnesses Service
- Monitors witness status every minute
- Tracks missed blocks in real-time
- Creates witness history snapshots
- Alerts on witness issues

## Monitoring

### Metrics Endpoint
The service exposes Prometheus metrics at `http://localhost:9091/metrics`:

- `steemdb_blocks_processed_total` - Total blocks processed
- `steemdb_operations_processed_total` - Total operations processed
- `steemdb_processing_duration_seconds` - Processing time histograms
- `steemdb_errors_total` - Error counts by type

### Health Check
Health check endpoint: `http://localhost:9090/health`

### Grafana Dashboard
Access Grafana at `http://localhost:3000` (admin/admin123) for visual monitoring.

## Performance

### Benchmarks
| Metric | Python Services | Go Service | Improvement |
|--------|----------------|------------|-------------|
| Blocks/sec | 50-100 | 200-500 | 4-5x |
| Operations/sec | 500-1000 | 2000-5000 | 4-5x |
| Memory Usage | 300-800MB | 100-200MB | 3-4x |
| CPU Usage | 60-80% | 20-40% | 2x |

### Optimization Features
- **Sequential processing**: Single goroutine ensures operation order correctness
- **Denormalized data models**: Avoids JOIN operations for faster queries
- **Account operations index**: Fast account operation history queries
- Batch database operations
- Connection pooling and reuse
- Intelligent retry mechanisms
- Memory-efficient data structures

## Development

### Project Structure
```
steemdb-sync/
├── cmd/sync/           # Main application entry
├── internal/
│   ├── blockchain/     # Operation processors
│   ├── database/       # MongoDB models and operations
│   ├── services/       # Core business logic
│   └── utils/          # Configuration and logging
├── pkg/steem/          # Steem RPC client
├── configs/            # Configuration files
└── monitoring/         # Prometheus configuration
```

### Adding New Operation Types

1. Add handler to `internal/blockchain/operation_processor.go`:
```go
func (p *OperationProcessor) handleNewOperation(ctx context.Context, op *Operation) error {
    // Implementation
}
```

2. Register handler in `registerHandlers()`:
```go
p.handlers["new_operation"] = p.handleNewOperation
```

3. Add database model to `internal/database/models.go`

### Testing

```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...

# Benchmark tests
go test -bench=. ./...
```

## Deployment

### Docker Compose Deployment (Recommended)

The sync service is deployed as part of the unified Docker Compose setup:

```bash
# From project root
cd /path/to/steemdb

# Start all services
docker-compose up -d

# View logs
docker-compose logs -f steemdb-sync

# Restart service
docker-compose restart steemdb-sync
```

### Standalone Docker Deployment

1. Build production image:
```bash
cd steemdb-sync
docker build -t steemdb/sync:latest .
```

2. Run container:
```bash
docker run -d \
  -p 9091:9091 \
  -v $(pwd)/configs:/app/configs \
  -e MONGODB_URI=mongodb://mongo:27017 \
  -e REDIS_URI=redis://redis:6379 \
  --name steemdb-sync \
  steemdb/sync:latest
```

### Environment Variables
- `MONGODB_URI` - MongoDB connection string
- `REDIS_URI` - Redis connection URI (format: `redis://[password@]host:port[/db]` or `host:port`)
- `STEEM_NODES` - Comma-separated Steem node URLs
- `LOG_LEVEL` - Logging level (debug, info, warn, error)

**Note**: Log file path and other log settings (format, rotation, etc.) are configured in `config.yaml` under the `log` section, not via environment variables. See the [Logging Configuration](#logging-configuration) section for details.

## Migration from Python Services

This Go service replaces the following Python services:
- `docker/sync/sync.py` → Block Sync Module
- `docker/history/history.py` → History Module  
- `docker/witnesses/witnesses.py` → Witnesses Module
- `docker/live/live.py` → Integrated into Web service

### Migration Steps

1. Stop existing Python services
2. Backup MongoDB data
3. Deploy Go sync service
4. Verify data consistency
5. Monitor performance metrics

## Troubleshooting

### Common Issues

**High Memory Usage**
- Reduce `batch_size` in config
- Check for memory leaks in logs

**Slow Synchronization**
- Use faster Steem nodes
- Optimize MongoDB indexes
- Note: Single goroutine architecture ensures correctness over raw speed

**Connection Errors**
- Verify Steem node availability
- Check MongoDB/Redis connectivity
- Review firewall settings

**Log File Permission Errors**
- Check log directory permissions: `ls -la logs/`
- Ensure the service user has write access to the log directory
- The service will automatically fall back to console output if file writing fails
- For production, use absolute paths with proper permissions: `/var/log/steemdb-sync.log`
- Fix permissions: `chmod 755 logs/` or `chown user:group logs/`

### Logs Analysis

**Docker Deployment:**
```bash
# View real-time logs
docker-compose logs -f steemdb-sync

# Search for errors
docker-compose logs steemdb-sync | grep ERROR

# Check specific service logs
docker-compose logs steemdb-sync | grep "Block sync"
```

**Standalone Deployment:**
```bash
# View log file (if configured)
tail -f logs/steemdb-sync.log

# Search for errors
grep ERROR logs/steemdb-sync.log

# View rotated log files
ls -lh logs/steemdb-sync*.log*

# Monitor real-time logs (JSON format)
tail -f logs/steemdb-sync.log | jq '.'

# Filter by log level
grep '"level":"error"' logs/steemdb-sync.log | jq '.'
```

**Log File Permissions:**
If you encounter permission errors when writing to log files:
1. Ensure the log directory exists and is writable
2. Check file ownership: `ls -la logs/`
3. Fix permissions: `chmod 755 logs/` or `chown user:group logs/`
4. The service will automatically fall back to console output if file writing fails

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Support

- GitHub Issues: Report bugs and feature requests
- Documentation: See `/docs` directory
- Monitoring: Use Grafana dashboards for operational insights
