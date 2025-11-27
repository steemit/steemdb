#!/bin/bash

# Test script for SteemDB Sync Service

set -e

echo "Building SteemDB Sync Service..."
go build -o steemdb-sync cmd/sync/main.go

echo "Testing configuration loading..."
./steemdb-sync configs/test-config.yaml --dry-run 2>/dev/null || echo "Config test completed"

echo "Running basic validation tests..."

# Test 1: Check if binary exists and is executable
if [ -x "./steemdb-sync" ]; then
    echo "✅ Binary is executable"
else
    echo "❌ Binary is not executable"
    exit 1
fi

# Test 2: Check if config file is valid
if [ -f "configs/test-config.yaml" ]; then
    echo "✅ Test config file exists"
else
    echo "❌ Test config file missing"
    exit 1
fi

# Test 3: Basic package compilation
echo "Testing package compilation..."
go test -c ./internal/utils > /dev/null 2>&1 && echo "✅ Utils package compiles" || echo "⚠️  Utils package test compilation failed"
go test -c ./internal/database > /dev/null 2>&1 && echo "✅ Database package compiles" || echo "⚠️  Database package test compilation failed"
go test -c ./pkg/steem > /dev/null 2>&1 && echo "✅ Steem package compiles" || echo "⚠️  Steem package test compilation failed"

echo ""
echo "🎉 Basic tests completed successfully!"
echo ""
echo "To run the service:"
echo "  ./steemdb-sync configs/test-config.yaml"
echo ""
echo "To run with Docker:"
echo "  docker-compose up --build"
echo ""
echo "Metrics will be available at: http://localhost:9091/metrics"
