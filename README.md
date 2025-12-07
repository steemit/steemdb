# SteemDB

A high-performance, modern blockchain explorer and data synchronization system for the Steem blockchain, built with Go and React.

## 📋 Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Projects](#projects)
- [Quick Start](#quick-start)
- [Deployment](#deployment)
- [Development](#development)
- [Monitoring](#monitoring)
- [Configuration](#configuration)
- [Contributing](#contributing)
- [License](#license)

## 🎯 Overview

SteemDB is a comprehensive blockchain data platform that provides:

- **Real-time Blockchain Synchronization**: High-performance data sync service that processes 200-500 blocks/second
- **RESTful API**: Modern web API for accessing blockchain data
- **WebSocket Support**: Real-time data streaming for live updates
- **Modern Web Interface**: React-based frontend with TypeScript
- **Comprehensive Monitoring**: Prometheus metrics and Grafana dashboards

### Key Features

- ⚡ **High Performance**: 3-5x faster than the original Python implementation
- 🔄 **Real-time Sync**: Continuous blockchain data synchronization
- 📊 **Rich APIs**: RESTful API with WebSocket support
- 🎨 **Modern UI**: React 19 with TypeScript and Tailwind CSS
- 📈 **Monitoring**: Built-in Prometheus metrics and Grafana dashboards
- 🐳 **Containerized**: Full Docker and Docker Compose support
- 🔒 **Reliable**: Comprehensive error handling and automatic recovery

## 🏗 Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      SteemDB System                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐ │
│  │  steemdb-    │    │  steemdb-    │    │  steemdb-    │ │
│  │  sync        │    │  web         │    │  frontend    │ │
│  │              │    │              │    │              │ │
│  │  • Block     │    │  • REST API  │    │  • React UI  │ │
│  │    Sync      │    │  • WebSocket │    │  • TypeScript│ │
│  │  • History   │    │  • Nginx     │    │  • Tailwind  │ │
│  │  • Witnesses │    │  • Go Backend│    │  • Vite      │ │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘ │
│         │                    │                    │         │
│         └────────────────────┼────────────────────┘         │
│                              │                               │
│         ┌────────────────────┴────────────────────┐         │
│         │                                          │         │
│    ┌────▼─────┐                            ┌─────▼────┐    │
│    │ MongoDB  │                            │  Redis   │    │
│    │          │                            │          │    │
│    │ Primary  │                            │  Cache   │    │
│    │ Database │                            │  Layer   │    │
│    └──────────┘                            └──────────┘    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         Monitoring Stack                              │  │
│  │  • Prometheus (Metrics Collection)                   │  │
│  │  • Grafana (Visualization)                           │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Technology Stack

- **Backend Services**: Go 1.23
- **Frontend**: React 19, TypeScript, Vite, Tailwind CSS
- **Database**: MongoDB 6.0 (primary), Redis 7 (cache)
- **Web Framework**: Gin
- **WebSocket**: Gorilla WebSocket
- **Monitoring**: Prometheus, Grafana
- **Containerization**: Docker, Docker Compose
- **Process Management**: Supervisord

## 📦 Projects

### steemdb-sync

High-performance blockchain synchronization service that replaces the original Python services.

**Features:**
- Real-time block synchronization (200-500 blocks/second)
- Account history collection (6-hour intervals)
- Witness monitoring (1-minute intervals)
- 15+ operation type handlers
- Prometheus metrics endpoint

**Documentation**: [steemdb-sync/README.md](steemdb-sync/README.md)

### steemdb-web

Modern web service providing RESTful API and WebSocket support.

**Features:**
- RESTful API for accounts, blocks, witnesses, and statistics
- WebSocket real-time data streaming
- Nginx integration for static file serving
- JWT authentication support
- Health checks and metrics

**Documentation**: [steemdb-web/README.md](steemdb-web/README.md)

### steemdb-frontend

Modern React-based frontend application.

**Features:**
- React 19 with TypeScript
- Tailwind CSS for styling
- Zustand for state management
- TanStack Query for data fetching
- React Router DOM for navigation
- D3.js and Recharts for data visualization

**Documentation**: [steemdb-frontend/README.md](steemdb-frontend/README.md)

## 🚀 Quick Start

### Prerequisites

- **Docker** 20.10+ and **Docker Compose** 2.0+
- **Go** 1.23+ (for development)
- **Node.js** 18+ and **pnpm** 8+ (for frontend development)
- **MongoDB** 6.0+ (included in Docker Compose)
- **Redis** 7+ (included in Docker Compose)

### Using Docker Compose (Recommended)

This is the fastest way to get started with all services running.

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd steemdb
   ```

2. **Configure services** (optional)
   ```bash
   # Edit sync service configuration
   vim steemdb-sync/configs/config.yaml
   
   # Edit web service configuration
   vim steemdb-web/configs/config.yaml
   ```

3. **Start all services**
   ```bash
   docker-compose up -d
   ```

4. **Verify services are running**
   ```bash
   # Check service status
   docker-compose ps
   
   # View logs
   docker-compose logs -f
   
   # Check health endpoints
   curl http://localhost/health
   curl http://localhost:9091/metrics
   ```

5. **Access services**
   - **Frontend**: http://localhost/
   - **API**: http://localhost/api/v1/
   - **WebSocket**: ws://localhost/ws
   - **Prometheus**: http://localhost:9091
   - **Grafana**: http://localhost:3000 (admin/admin123)
   - **MongoDB**: localhost:27017
   - **Redis**: localhost:6379

### Manual Development Setup

#### Sync Service

```bash
cd steemdb-sync
go mod download
go build -o steemdb-sync cmd/sync/main.go
./steemdb-sync configs/config.yaml
```

#### Web Service

```bash
cd steemdb-web
go mod download
go run cmd/web/main.go configs/config.yaml
```

#### Frontend

```bash
cd steemdb-frontend
pnpm install
pnpm run dev
```

## 🐳 Deployment

### Docker Compose Deployment

The project includes a unified `docker-compose.yml` at the root directory that orchestrates all services:

**Available Services:**
- `steemdb-web` - Web API service with Nginx and frontend
- `steemdb-sync` - Blockchain synchronization service (if added to compose)
- `mongo` - MongoDB database
- `redis` - Redis cache
- `prometheus` - Metrics collection
- `grafana` - Visualization dashboards

**Common Commands:**

```bash
# Start all services
docker-compose up -d

# Stop all services
docker-compose down

# View logs
docker-compose logs -f [service-name]

# Restart a service
docker-compose restart [service-name]

# Rebuild and restart
docker-compose up -d --build

# View service status
docker-compose ps

# Stop and remove volumes (⚠️ deletes data)
docker-compose down -v
```

### Service Configuration

#### Configuration Mounting

Both `steemdb-sync` and `steemdb-web` support configuration mounting from the host, allowing you to modify settings without rebuilding images:

- **Sync Service**: `./steemdb-sync/configs:/app/configs`
- **Web Service**: `./steemdb-web/configs:/app/configs`

**To modify configuration:**

1. Edit configuration files on the host:
   ```bash
   # Edit web service config
   vim steemdb-web/configs/config.yaml
   
   # Edit sync service config
   vim steemdb-sync/configs/config.yaml
   ```

2. Restart the service to apply changes:
   ```bash
   docker-compose restart steemdb-web
   # or
   docker-compose restart steemdb-sync
   ```

**For detailed configuration instructions:**
- [steemdb-web/docker/CONFIGURATION.md](steemdb-web/docker/CONFIGURATION.md)
- [steemdb-sync/README.md](steemdb-sync/README.md#configuration)

### Production Deployment

1. **Build production images**
   ```bash
   docker-compose build
   ```

2. **Set environment variables**
   ```bash
   export SERVER_MODE=production
   export MONGODB_URI=mongodb://prod-mongo:27017
   export REDIS_ADDR=prod-redis:6379
   ```

3. **Deploy with production configuration**
   ```bash
   docker-compose -f docker-compose.yml up -d
   ```

4. **Set up reverse proxy** (if needed)
   - Configure Nginx or Traefik for SSL termination
   - Point to `http://localhost:80` for the web service

## 💻 Development

### Project Structure

```
steemdb/
├── docker-compose.yml          # Unified Docker Compose configuration
├── steemdb-sync/               # Blockchain synchronization service
│   ├── cmd/sync/               # Main entry point
│   ├── internal/               # Internal packages
│   │   ├── blockchain/         # Operation processors
│   │   ├── database/           # MongoDB operations
│   │   ├── services/           # Business logic
│   │   └── utils/              # Utilities
│   ├── pkg/steem/              # Steem RPC client
│   ├── configs/                # Configuration files
│   └── monitoring/             # Prometheus configs
├── steemdb-web/                # Web API service
│   ├── cmd/web/                # Main entry point
│   ├── internal/               # Internal packages
│   │   ├── api/                # API handlers
│   │   ├── database/           # Database connections
│   │   ├── models/             # Data models
│   │   ├── services/           # Business logic
│   │   └── websocket/          # WebSocket handlers
│   ├── pkg/                    # Public packages
│   ├── docker/                 # Docker configurations
│   └── configs/                # Configuration files
└── steemdb-frontend/           # React frontend
    ├── src/                    # Source code
    ├── public/                 # Static assets
    └── dist/                   # Build output
```

### Development Workflow

1. **Start development environment**
   ```bash
   # Start dependencies (MongoDB, Redis)
   docker-compose up -d mongo redis
   
   # Run sync service locally
   cd steemdb-sync && go run cmd/sync/main.go configs/config.yaml
   
   # Run web service locally
   cd steemdb-web && go run cmd/web/main.go configs/config.yaml
   
   # Run frontend in dev mode
   cd steemdb-frontend && pnpm run dev
   ```

2. **Run tests**
   ```bash
   # Sync service tests
   cd steemdb-sync && go test ./...
   
   # Web service tests
   cd steemdb-web && go test ./...
   
   # Frontend tests
   cd steemdb-frontend && pnpm test
   ```

3. **Build for production**
   ```bash
   # Build sync service
   cd steemdb-sync && go build -o steemdb-sync cmd/sync/main.go
   
   # Build web service
   cd steemdb-web && go build -o steemdb-web cmd/web/main.go
   
   # Build frontend
   cd steemdb-frontend && pnpm build
   ```

### Adding New Features

#### Adding a New Operation Type (Sync Service)

1. Add handler to `steemdb-sync/internal/blockchain/operation_processor.go`
2. Register handler in `registerHandlers()`
3. Add database model if needed
4. Write tests

#### Adding a New API Endpoint (Web Service)

1. Create handler in `steemdb-web/internal/api/`
2. Add route in `steemdb-web/internal/api/routes.go`
3. Implement service logic in `steemdb-web/internal/services/`
4. Add tests

#### Adding a New Frontend Page

1. Create component in `steemdb-frontend/src/pages/`
2. Add route in `steemdb-frontend/src/App.tsx`
3. Update navigation if needed
4. Add API integration

## 📊 Monitoring

### Prometheus Metrics

Both services expose Prometheus metrics:

- **Sync Service**: `http://localhost:9091/metrics`
- **Web Service**: `http://localhost:9090/metrics` (if exposed)

### Key Metrics

- `steemdb_blocks_processed_total` - Total blocks processed
- `steemdb_operations_processed_total` - Total operations processed
- `steemdb_processing_duration_seconds` - Processing time histograms
- `steemdb_errors_total` - Error counts by type
- `steemdb_http_requests_total` - HTTP request counts
- `steemdb_websocket_connections` - Active WebSocket connections

### Grafana Dashboards

Access Grafana at `http://localhost:3000`:
- Default credentials: `admin/admin123`
- Pre-configured dashboards for sync and web services
- Custom dashboards can be imported from `steemdb-sync/monitoring/`

### Health Checks

All services include health check endpoints:

- **Web Service**: `http://localhost/health` (via Nginx)
- **Sync Service**: `http://localhost:9090/health` (if exposed)
- **MongoDB**: Internal health check via `mongosh`
- **Redis**: Internal health check via `redis-cli ping`

**Check service health:**
```bash
# Web service
curl http://localhost/health

# All services status
docker-compose ps
```

## ⚙️ Configuration

### Environment Variables

Both services support environment variable overrides:

**Sync Service:**
- `MONGODB_URI` - MongoDB connection string
- `REDIS_ADDR` - Redis address
- `STEEM_NODES` - Comma-separated Steem node URLs
- `LOG_LEVEL` - Logging level (debug, info, warn, error)

**Web Service:**
- `SERVER_MODE` - Server mode (development, production)
- `MONGODB_URI` - MongoDB connection string
- `REDIS_ADDR` - Redis address
- `JWT_SECRET` - JWT secret key

### Configuration Files

- **Sync Service**: `steemdb-sync/configs/config.yaml`
- **Web Service**: `steemdb-web/configs/config.yaml`

See individual project READMEs for detailed configuration options.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes with tests
4. Commit your changes (`git commit -m 'Add some amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

### Code Style

- **Go**: Follow standard Go formatting (`go fmt`)
- **TypeScript/React**: Follow ESLint rules
- **Commits**: Use conventional commit messages in English

### Testing Requirements

- All new features must include tests
- Maintain or improve test coverage
- All tests must pass before submitting PR

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Support

- **Documentation**: See individual project READMEs
- **Issues**: [GitHub Issues](https://github.com/steemdb/steemdb/issues)
- **Discussions**: [GitHub Discussions](https://github.com/steemdb/steemdb/discussions)

## 🗺 Roadmap

- [ ] Complete API implementation
- [ ] Advanced caching strategies
- [ ] Horizontal scaling support
- [ ] GraphQL API
- [ ] Mobile API optimizations
- [ ] Enhanced monitoring and alerting
- [ ] Performance optimizations

## 📚 Additional Resources

### Documentation

- [steemdb-sync/README.md](steemdb-sync/README.md) - Sync service documentation
- [steemdb-web/README.md](steemdb-web/README.md) - Web service documentation
- [steemdb-frontend/README.md](steemdb-frontend/README.md) - Frontend documentation
- [steemdb-web/docker/CONFIGURATION.md](steemdb-web/docker/CONFIGURATION.md) - Docker configuration guide

### Project Structure Overview

```
steemdb/
├── README.md                 # This file - main project documentation
├── docker-compose.yml        # Unified Docker Compose configuration
├── steemdb-sync/            # Blockchain synchronization service
├── steemdb-web/             # Web API service (includes frontend)
└── steemdb-frontend/         # React frontend application
```

### Quick Reference

**Service Ports:**
- `80` - Web service (Nginx + Frontend + API)
- `9090` - Web service metrics (if exposed)
- `9091` - Prometheus
- `3000` - Grafana
- `27017` - MongoDB
- `6379` - Redis

**Key Endpoints:**
- Health: `http://localhost/health`
- API: `http://localhost/api/v1/`
- WebSocket: `ws://localhost/ws`
- Metrics: `http://localhost:9091/metrics`
- Grafana: `http://localhost:3000`

---

**Built with ❤️ for the Steem community**

