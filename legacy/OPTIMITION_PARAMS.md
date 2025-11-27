# MongoDB Performance Optimization Parameters

This document provides optimized configuration parameters for MongoDB running on EC2 m5.2xlarge instances with high aggregation workloads.

## EC2 System Parameters (sysctl.conf)

Add the following settings to `/etc/sysctl.conf` for optimal MongoDB performance:

```conf
# File descriptor limits (supports more connections and file operations)
fs.file-max = 1000000
fs.nr_open = 1000000

# Virtual memory optimization (reduces swap usage, MongoDB relies heavily on memory)
vm.swappiness = 0
vm.dirty_ratio = 10
vm.dirty_background_ratio = 5

# Network optimization (improves concurrent connection handling)
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.tcp_fin_timeout = 30

# Memory mapping limits (required for MongoDB index and data files)
vm.max_map_count = 1048576  # Higher than default to support larger datasets
```

**Apply changes:**
```bash
sudo sysctl -p
```

## MongoDB Configuration

Create the configuration file at `/etc/mongod.conf` and mount it to the container at `/etc/mongod.conf`.

### mongod.conf

```yaml
storage:
  dbPath: /data/db
  engine: wiredTiger  # Default engine, well-suited for aggregation workloads
  wiredTiger:
    engineConfig:
      # Cache size: 50-70% of container memory (16GB for 24GB allocated memory)
      cacheSizeGB: 16
      # Increase concurrent transaction capacity (aggregations may involve transactions)
      maxConcurrentTransactions: 1000
    collectionConfig:
      # Enable zstd compression (higher compression ratio than snappy, acceptable CPU overhead)
      blockCompressor: zstd
    indexConfig:
      # Index prefix compression reduces memory usage
      prefixCompression: true

systemLog:
  destination: file
  path: /var/log/mongodb/mongod.log
  logAppend: true

net:
  port: 27017
  # Increase max connections to handle high-concurrency aggregation requests
  maxIncomingConnections: 10000
  unixDomainSocket:
    enabled: false

processManagement:
  fork: false  # Not needed in Docker containers

setParameter:
  # Enable aggregation pipeline optimizer (automatically optimizes execution order)
  aggregationPipelineOptimizer: true
  # Increase in-memory sort threshold (1GB, reduces disk temporary files)
  internalQueryExecMaxBlockingSortBytes: 1048576000
  # Maximum sort memory usage (2GB)
  internalQueryMaxBlockingSortMemoryUsageBytes: 2147483648
  # Transaction lock timeout (prevents long-term blocking)
  maxTransactionLockRequestTimeoutMillis: 5000

operationProfiling:
  mode: slowOp  # Log slow queries for aggregation optimization
  slowOpThresholdMs: 100  # Log aggregation operations exceeding 100ms
```

## Docker Run Command

Execute the following command to start MongoDB with optimized parameters:

```bash
docker run -d \
  --name mongodb \
  --memory=24G \              # Allocate 24GB memory (reserve 8GB for system and other processes)
  --memory-reservation=20G \   # Minimum reserved memory to prevent system reclaim
  --cpus=6 \                   # Allocate 6 vCPUs (reserve 2 for system and burst load)
  --cpuset-cpus=0-5 \         # Bind CPU cores to reduce scheduling overhead (optional)
  --restart=always \
  -v /etc/mongod.conf:/etc/mongod.conf \
  -v /data/mongodb:/data/db \  # Mount EBS gp3 volume (recommended: 400GB+, 3000+ IOPS)
  -p 27017:27017 \
  mongo:latest \
  mongod --config /etc/mongod.conf
```

### Resource Allocation Notes

- **Memory**: Allocate 24GB to the container, with 16GB for MongoDB WiredTiger cache (cacheSizeGB)
- **CPU**: Allocate 6 out of 8 vCPUs to MongoDB, leaving 2 for system operations
- **Storage**: Use EBS gp3 volumes with high IOPS (3000+) for better aggregation performance
- **Network**: maxIncomingConnections set to 10000 to handle concurrent aggregation requests

### Verification

After starting the container, verify the configuration:

```bash
# Check MongoDB logs
docker logs mongodb

# Connect to MongoDB and verify settings
docker exec -it mongodb mongosh
db.serverStatus().wiredTiger.cache
db.serverStatus().connections
```

## Performance Expectations

With these optimizations, you should see:

- **Reduced CPU usage**: Better query planning and pipeline optimization
- **Improved aggregation performance**: Larger in-memory sort buffers reduce disk spilling
- **Higher concurrency**: Increased connection limits support more simultaneous requests
- **Better memory utilization**: Optimized cache size and compression reduce memory pressure

## Additional Recommendations

1. **Indexes**: Ensure proper indexes are created for frequently queried fields (see `MONGODB_CPU_OPTIMIZATION.md`)
2. **Monitoring**: Set up MongoDB monitoring to track performance metrics
3. **Backup**: Configure regular backups using `mongodump` or MongoDB Atlas backup solutions
4. **Log Rotation**: Configure log rotation to prevent disk space issues