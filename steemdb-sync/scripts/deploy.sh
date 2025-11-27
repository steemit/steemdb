#!/bin/bash

# Deployment script for SteemDB Sync Service

set -e

ENVIRONMENT=${1:-development}
CONFIG_FILE="configs/${ENVIRONMENT}.yaml"

echo "🚀 Deploying SteemDB Sync Service (Environment: $ENVIRONMENT)"

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "❌ Configuration file not found: $CONFIG_FILE"
    echo "Available configs:"
    ls -la configs/
    exit 1
fi

echo "📋 Using configuration: $CONFIG_FILE"

# Build Docker image
echo "🐳 Building Docker image..."
docker build -t steemdb/sync:${ENVIRONMENT} .

# Stop existing containers
echo "🛑 Stopping existing containers..."
docker-compose down || true

# Start services
echo "🚀 Starting services..."
if [ "$ENVIRONMENT" = "production" ]; then
    # Production deployment
    docker-compose -f docker-compose.yml up -d
else
    # Development deployment
    docker-compose up -d
fi

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 10

# Health check
echo "🏥 Performing health checks..."

# Check if sync service is running
if docker-compose ps steemdb-sync | grep -q "Up"; then
    echo "✅ Sync service is running"
else
    echo "❌ Sync service is not running"
    docker-compose logs steemdb-sync
    exit 1
fi

# Check MongoDB connection
if docker-compose exec -T mongo mongosh --eval "db.adminCommand('ping')" > /dev/null 2>&1; then
    echo "✅ MongoDB is accessible"
else
    echo "❌ MongoDB is not accessible"
fi

# Check Redis connection
if docker-compose exec -T redis redis-cli ping > /dev/null 2>&1; then
    echo "✅ Redis is accessible"
else
    echo "❌ Redis is not accessible"
fi

# Check metrics endpoint
sleep 5
if curl -s http://localhost:9091/metrics > /dev/null; then
    echo "✅ Metrics endpoint is accessible"
else
    echo "⚠️  Metrics endpoint is not accessible (may take time to start)"
fi

echo ""
echo "🎉 Deployment completed!"
echo ""
echo "📊 Monitoring URLs:"
echo "  - Metrics: http://localhost:9091/metrics"
echo "  - Prometheus: http://localhost:9090"
echo "  - Grafana: http://localhost:3000 (admin/admin123)"
echo "  - MongoDB Express: http://localhost:8081 (admin/admin123)"
echo ""
echo "📝 Useful commands:"
echo "  - View logs: docker-compose logs -f steemdb-sync"
echo "  - Check status: docker-compose ps"
echo "  - Stop services: docker-compose down"
echo "  - Restart sync: docker-compose restart steemdb-sync"
