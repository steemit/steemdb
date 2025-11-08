# Slow Query Log Guide

## Log Configuration

### 1. PHP-FPM Slow Query Logs
- **Configuration Location**: `conf/www.conf`
- **Slow Query Threshold**: `request_slowlog_timeout = 10s` (requests exceeding 10 seconds will be logged)
- **Log Output**: `/dev/stderr` (output through Docker container stderr)

### 2. Nginx Access Logs (Enhanced)
- **Configuration Location**: `conf/nginx.conf`
- **Log Format**: `detailed` (includes response time information)
- **Fields Included**:
  - `rt=$request_time` - Total response time (seconds)
  - `uct=$upstream_connect_time` - Upstream connection time
  - `uht=$upstream_header_time` - Upstream response header time
  - `urt=$upstream_response_time` - Upstream response time
- **Log Output**: `/dev/stdout` (output through Docker container stdout)

## Methods to View Slow Query Logs

### Method 1: Using the Check Script (Recommended)

```bash
# Basic usage (using default container name and 5 second threshold)
./docker/development/scripts/check_slow_queries.sh

# Specify container name
./docker/development/scripts/check_slow_queries.sh your-container-name

# Specify container name and slow query threshold (seconds)
./docker/development/scripts/check_slow_queries.sh your-container-name 3
```

The script will display:
- PHP-FPM slow query logs
- 504 timeout errors
- Requests with response time exceeding threshold
- Top 20 slowest requests (sorted by response time)
- Response time distribution statistics

### Method 2: Direct Docker Commands

#### View PHP-FPM Slow Queries (exceeding 10 seconds)
```bash
docker logs <container-name> 2>&1 | grep -i "slow\|timeout\|exceeded"
```

#### View 504 Timeout Errors
```bash
docker logs <container-name> 2>&1 | grep -E "504|timeout|Gateway Timeout"
```

#### View Requests with Response Time Exceeding 5 Seconds
```bash
docker logs <container-name> 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] > 5) print $0
  }
}'
```

#### View Top 20 Slowest Requests
```bash
docker logs <container-name> 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    printf "%.3f %s\n", arr[1], $0
  }
}' | sort -rn | head -20
```

#### Real-time Slow Query Monitoring
```bash
# Monitor all logs
docker logs -f <container-name> 2>&1

# Monitor only slow queries (response time > 5 seconds)
docker logs -f <container-name> 2>&1 | grep --line-buffered -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] > 5) print "[Slow Query] " $0
  }
}'
```

### Method 3: Analyze Slow Queries for Specific URLs

```bash
# Find slow requests for specific path
docker logs <container-name> 2>&1 | grep "/api/slow-endpoint" | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] > 3) print arr[1] " seconds: " $0
  }
}'
```

## Log Format Examples

### Nginx Access Log (Enhanced Format)
```
[Web] 192.168.1.100 - - [25/Dec/2024:10:30:45 +0000] "GET /api/endpoint HTTP/1.1" 200 1234 "-" "Mozilla/5.0" "-" rt=2.345 uct="0.001" uht="0.002" urt="2.340"
```

Field Descriptions:
- `rt=2.345` - Total response time 2.345 seconds
- `uct="0.001"` - Time to connect to PHP-FPM 0.001 seconds
- `uht="0.002"` - Time to receive PHP-FPM response header 0.002 seconds
- `urt="2.340"` - PHP-FPM processing time 2.340 seconds

### PHP-FPM Slow Query Log
```
[25-Dec-2024 10:30:45] WARNING: [pool www] child 1234, script '/var/www/html/public/index.php' (request: "GET /api/endpoint") executing too slow (10.234 sec), logging
```

## Optimization Recommendations

1. **If many slow queries are found**:
   - Check if MongoDB queries are using indexes
   - Optimize complex aggregation operations
   - Consider adding caching

2. **If `urt` (upstream response time) is long**:
   - Indicates slow PHP processing, need to optimize PHP code or database queries

3. **If `uct` (connection time) is long**:
   - May indicate PHP-FPM process pool is full, consider increasing `pm.max_children`

4. **Adjust Slow Query Threshold**:
   - Modify `request_slowlog_timeout` value in `conf/www.conf`
   - Currently set to 10 seconds, can be adjusted as needed

## FAQ

### Q: Why can't I see slow query logs?
A: Check the following:
1. Confirm `request_slowlog_timeout` is set correctly
2. Confirm requests actually exceed the threshold
3. Check Docker log output: `docker logs <container-name> 2>&1`

### Q: How to adjust slow query threshold?
A: Edit `conf/www.conf`, modify the `request_slowlog_timeout` value, then restart the container.

### Q: Too many logs, how to view only recent ones?
A: Use the `tail` command:
```bash
docker logs <container-name> 2>&1 | tail -100 | grep -i slow
```
