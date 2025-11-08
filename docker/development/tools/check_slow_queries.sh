#!/bin/bash
# Script to check slow query logs

CONTAINER_NAME=${1:-steemdb}  # Default container name, can be passed as parameter
SLOW_THRESHOLD=${2:-5}  # Slow query threshold (seconds), default 5 seconds

echo "=== PHP-FPM Slow Query Logs (requests exceeding 10 seconds) ==="
echo "Searching for slow queries from Docker container stderr:"
echo ""
docker logs $CONTAINER_NAME 2>&1 | grep -i "slow\|timeout\|exceeded" | tail -50

echo ""
echo "=== Recent 100 error logs (may contain timeout information) ==="
docker logs $CONTAINER_NAME 2>&1 | grep -E "504|timeout|Gateway Timeout" | tail -50

echo ""
echo "=== Nginx access logs with response time exceeding ${SLOW_THRESHOLD} seconds ==="
echo "Analyzing rt= field (response time) in logs..."
docker logs $CONTAINER_NAME 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk -v threshold=$SLOW_THRESHOLD '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] > threshold) {
      print $0
    }
  }
}' | tail -50

echo ""
echo "=== Top 20 slowest requests sorted by response time ==="
docker logs $CONTAINER_NAME 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    printf "%.3f %s\n", arr[1], $0
  }
}' | sort -rn | head -20

echo ""
echo "=== Response time distribution statistics ==="
echo "0-1 seconds:"
docker logs $CONTAINER_NAME 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] < 1) count++
  }
} END {print count+0}'

echo "1-5 seconds:"
docker logs $CONTAINER_NAME 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] >= 1 && arr[1] < 5) count++
  }
} END {print count+0}'

echo "5-10 seconds:"
docker logs $CONTAINER_NAME 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] >= 5 && arr[1] < 10) count++
  }
} END {print count+0}'

echo "10+ seconds:"
docker logs $CONTAINER_NAME 2>&1 | grep -E "rt=[0-9]+\.[0-9]+" | awk '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] >= 10) count++
  }
} END {print count+0}'

echo ""
echo "=== Real-time slow query monitoring (Press Ctrl+C to exit) ==="
echo "Monitoring container logs for requests with response time exceeding ${SLOW_THRESHOLD} seconds..."
docker logs -f $CONTAINER_NAME 2>&1 | grep --line-buffered -E "rt=[0-9]+\.[0-9]+" | awk -v threshold=$SLOW_THRESHOLD '{
  if (match($0, /rt=([0-9]+\.[0-9]+)/, arr)) {
    if (arr[1] > threshold) {
      print "[Slow Query] " $0
    }
  }
}'

