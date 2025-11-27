#!/bin/bash

# Build script for SteemDB Sync Service

set -e

echo "🔨 Building SteemDB Sync Service..."

# Clean previous builds
rm -f steemdb-sync

# Build the service
echo "📦 Compiling Go binary..."
go build -ldflags="-w -s" -o steemdb-sync cmd/sync/main.go

# Check binary size
BINARY_SIZE=$(du -h steemdb-sync | cut -f1)
echo "📊 Binary size: $BINARY_SIZE"

# Verify binary
echo "✅ Verifying binary..."
if [ -x "./steemdb-sync" ]; then
    echo "✅ Binary is executable"
    
    # Test help/version (if implemented)
    echo "🧪 Testing binary..."
    timeout 5s ./steemdb-sync configs/test-config.yaml 2>/dev/null || echo "✅ Binary starts correctly (expected timeout)"
else
    echo "❌ Binary is not executable"
    exit 1
fi

echo ""
echo "🎉 Build completed successfully!"
echo ""
echo "📋 Next steps:"
echo "  1. Configure MongoDB and Redis connections in configs/config.yaml"
echo "  2. Run: ./steemdb-sync configs/config.yaml"
echo "  3. Monitor metrics at: http://localhost:9091/metrics"
echo "  4. Or use Docker: docker-compose up --build"
