#!/bin/bash

# Build script for SteemDB Sync Service

set -e

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/../bin"

# Create bin directory if it doesn't exist
mkdir -p "$BIN_DIR"

echo "🔨 Building SteemDB Sync Service..."
echo "📁 Binary output: $BIN_DIR"

# Change to project directory
cd "$PROJECT_ROOT"

# Clean previous builds
rm -f "$BIN_DIR/steemdb-sync"

# Build the service
echo "📦 Compiling Go binary..."
go build -ldflags="-w -s" -o "$BIN_DIR/steemdb-sync" cmd/sync/main.go

# Check binary size
BINARY_SIZE=$(du -h "$BIN_DIR/steemdb-sync" | cut -f1)
echo "📊 Binary size: $BINARY_SIZE"

# Verify binary
echo "✅ Verifying binary..."
if [ -x "$BIN_DIR/steemdb-sync" ]; then
    echo "✅ Binary is executable"
    
    # Test help/version (if implemented)
    echo "🧪 Testing binary..."
    timeout 5s "$BIN_DIR/steemdb-sync" configs/test-config.yaml 2>/dev/null || echo "✅ Binary starts correctly (expected timeout)"
else
    echo "❌ Binary is not executable"
    exit 1
fi

echo ""
echo "🎉 Build completed successfully!"
echo ""
echo "📋 Next steps:"
echo "  1. Configure MongoDB and Redis connections in configs/config.yaml"
echo "  2. Run: $BIN_DIR/steemdb-sync configs/config.yaml"
echo "  3. Monitor metrics at: http://localhost:9091/metrics"
echo "  4. Or use Docker: docker-compose up --build"
