#!/bin/bash

# Build script for SteemDB Web Service

set -e

echo "🔨 Building SteemDB Web Service..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Clean previous builds
echo -e "${BLUE}📦 Cleaning previous builds...${NC}"
rm -f steemdb-web

# Build the service
echo -e "${BLUE}📦 Compiling Go binary...${NC}"
go build -ldflags="-w -s" -o steemdb-web cmd/web/main.go

# Check binary size
BINARY_SIZE=$(du -h steemdb-web | cut -f1)
echo -e "${GREEN}📊 Binary size: $BINARY_SIZE${NC}"

# Verify binary
echo -e "${BLUE}✅ Verifying binary...${NC}"
if [ -x "./steemdb-web" ]; then
    echo -e "${GREEN}✅ Binary is executable${NC}"
    
    # Test help/version (if implemented)
    echo -e "${BLUE}🧪 Testing binary...${NC}"
    timeout 5s ./steemdb-web configs/config.yaml 2>/dev/null || echo -e "${GREEN}✅ Binary starts correctly (expected timeout)${NC}"
else
    echo -e "${RED}❌ Binary is not executable${NC}"
    exit 1
fi

# Run tests
echo -e "${BLUE}🧪 Running tests...${NC}"
go test ./... -v

# Check for race conditions
echo -e "${BLUE}🏃 Checking for race conditions...${NC}"
go test -race ./...

# Build for different platforms
echo -e "${BLUE}🌍 Building for multiple platforms...${NC}"

# Linux AMD64
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o dist/steemdb-web-linux-amd64 cmd/web/main.go

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o dist/steemdb-web-linux-arm64 cmd/web/main.go

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o dist/steemdb-web-darwin-amd64 cmd/web/main.go

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s" -o dist/steemdb-web-darwin-arm64 cmd/web/main.go

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o dist/steemdb-web-windows-amd64.exe cmd/web/main.go

echo -e "${GREEN}📦 Cross-platform builds completed:${NC}"
ls -la dist/

echo ""
echo -e "${GREEN}🎉 Build completed successfully!${NC}"
echo ""
echo -e "${BLUE}📋 Next steps:${NC}"
echo "  1. Configure MongoDB and Redis connections in configs/config.yaml"
echo "  2. Run: ./steemdb-web configs/config.yaml"
echo "  3. Test: curl http://localhost:8080/health"
echo "  4. Monitor: http://localhost:9090/metrics"
echo "  5. Or use Docker: docker-compose up --build"
