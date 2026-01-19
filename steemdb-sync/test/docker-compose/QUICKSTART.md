# Quick Start Guide

## 5-Minute Setup

### 1. Prerequisites

```bash
# Ensure Docker and Docker Compose are installed
docker --version
docker-compose --version
```

### 2. Initial Setup

```bash
cd test/docker-compose

# Copy environment template
cp env.example .env

# Copy config template
mkdir -p configs
cp configs/config.yaml.template configs/config.yaml

# Edit config.yaml and set target_height (e.g., 1000 for quick test)
# Optional: Edit .env if you need to change default values
```

### 3. Start Services

```bash
# Make scripts executable (if not already)
chmod +x *.sh

# Start all services
./start.sh
```

### 4. Verify

```bash
# Check service status
./verify.sh

# Or manually check
docker-compose ps
curl http://localhost:9090/metrics
```

### 5. Monitor

```bash
# View logs
docker-compose logs -f cold-ingest

# View metrics
curl http://localhost:9090/metrics | grep ingest
```

### 6. Stop and Clean

```bash
# Stop services
./stop.sh

# Clean all data
./clean.sh
```

## Common Tasks

### Start Only MongoDB and cold_ingest

```bash
docker-compose up -d mongo cold-ingest
```

### Start All Services (including steemd)

```bash
./start.sh
```

### View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f cold-ingest
docker-compose logs -f steemd
docker-compose logs -f mongo
```

### Check MongoDB Data

```bash
docker-compose exec mongo mongo steemdb_test \
  --username admin --password 123456 \
  --authenticationDatabase admin

> db.operations.count()
> db.blocks.count()
> db.operations.find().limit(5).pretty()
```

### Restart a Service

```bash
docker-compose restart cold-ingest
```

### Rebuild cold_ingest Image

```bash
docker-compose build cold-ingest
docker-compose up -d cold-ingest
```

## Troubleshooting

### Services Won't Start

```bash
# Check logs
docker-compose logs

# Check port conflicts
netstat -tuln | grep -E "27017|8080|9090"

# Check Docker resources
docker stats
```

### MongoDB Connection Issues

```bash
# Test MongoDB connection
docker-compose exec mongo mongo --eval "db.adminCommand('ping')" \
  --username admin --password 123456 --authenticationDatabase admin

# Check network
docker-compose exec cold-ingest ping mongo
```

### Cold Ingest Not Receiving Data

```bash
# Check if service is running
curl http://localhost:8080/metrics

# Check steemd logs
docker-compose logs steemd

# Verify endpoint configuration
docker-compose exec steemd env | grep INGEST
```

## Next Steps

- Read [README.md](README.md) for detailed documentation
- Read [ARCHITECTURE.md](ARCHITECTURE.md) for architecture details
- Customize configuration in `configs/config.yaml`
- Adjust environment variables in `.env`
