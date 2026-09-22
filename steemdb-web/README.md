# SteemDB Web Service

A modern, high-performance web service for SteemDB blockchain explorer, built with Go and modern web technologies.

## 🚀 Features

### Core API Services
- **Account API**: Account information, history, statistics, and top accounts
- **Block API**: Block data with RPC-enriched details and virtual operations
- **Post/Comment API**: Posts list, detail, replies, votes, and reblogs
- **Labs API**: PowerUp, PowerDown, Rshares, Curation/Author leaderboards, Flags, Clients, Benefactors, Pending
- **Witness API**: Witness information and voting
- **Statistics API**: Network statistics and charts
- **Search API**: Search across blocks, transactions, and accounts
- **Legacy API**: 302 redirects from old `/api/*` paths to their v1 counterparts

### Real-time Features
- **WebSocket Support**: Real-time blockchain data streaming
- **Live Updates**: Real-time block and transaction notifications
- **Event Streaming**: Subscribe to specific blockchain events

### Performance & Reliability
- **High Performance**: Built with Go for optimal performance
- **Health Checks**: `/health` and `/ready` endpoints (readiness pings both MongoDB and Redis)
- **Graceful Shutdown**: Proper signal handling and connection draining

### Security
- **CORS Support**: Configurable CORS for web applications
- **Loopback by Default**: The Go server binds to `127.0.0.1`; Nginx is the only exposed listener

> Note: Redis is connected and used for the readiness probe only — response
> caching, rate limiting, and JWT authentication are **not implemented**.
> The corresponding `api.rate_limit`, `cache`, `metrics`, and `auth` config
> sections are parsed but currently have no consumers.

## 🛠 Technology Stack

- **Backend**: Go 1.23, Gin Web Framework
- **Database**: MongoDB (primary, read-only view of sync-written data), Redis (readiness probe)
- **WebSocket**: Gorilla WebSocket
- **Steem RPC**: [steemgosdk](https://github.com/steemit/steemgosdk)-based client with multi-node failover
- **Logging**: Structured logging with Zap
- **Configuration**: YAML-based configuration with Viper

## 📁 Project Structure

```
steemdb-web/
├── cmd/web/                # Main application entry point
├── internal/
│   ├── api/               # API handlers and routes (routes.go)
│   ├── database/          # MongoDB/Redis connections only; index authority lives in steemdb-sync
│   ├── models/            # Data models and structures
│   └── services/          # Business logic services (incl. websocket_service.go)
├── pkg/
│   ├── steem/             # Steem RPC client (steemgosdk-based, multi-node failover)
│   └── utils/             # Common utilities (config, logging)
├── web/                   # Static dev tools (websocket-test.html)
├── configs/               # Configuration files
├── docker/                # Docker configuration (nginx, supervisor)
└── scripts/               # Build and test scripts
```

## 🚀 Quick Start

### Prerequisites

- Go 1.23 or later
- MongoDB 4.4 or later
- Redis 6.0 or later
- Node.js 18+ (for frontend development)

### Installation

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd steemdb-web
   ```

2. **Install Go dependencies**
   ```bash
   go mod download
   ```

3. **Configure the application**
   ```bash
   # Edit configs/config.yaml with your settings
   ```

4. **Start the services**
   ```bash
   # Start MongoDB and Redis (from the repository root — the compose file lives there)
   docker-compose up -d mongo redis
   
   # Start the web service (from steemdb-web/)
   go run cmd/web/main.go configs/config.yaml
   ```

5. **Verify the installation**
   ```bash
   curl http://localhost:8080/health
   curl http://localhost:8080/ready
   ```

## 📖 API Documentation

### Health Endpoints

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (includes database connectivity)

### API v1 Endpoints

Base URL: `/api/v1`

#### Accounts
- `GET /accounts` - List accounts (page/page_size, sort_by/sort_order; limit/sort/order aliases also accepted; optional `search` name filter)
- `GET /accounts/search?q=` - Search accounts by name prefix
- `GET /accounts/stats` - Account statistics
- `GET /accounts/top?criteria=` - Top accounts (reputation/vests/balance/posts)
- `GET /accounts/:name` - Get account information
- `GET /accounts/:name/history` - Get account operation history

#### Blocks
- `GET /blocks` - List blocks (page/page_size, sort_by/sort_order; limit/sort/order aliases)
- `GET /blocks/latest` - Get latest blocks
- `GET /blocks/stats` - Block statistics (placeholder data)
- `GET /blocks/:number` - Get specific block (headers enriched from the steem RPC for cold-ingested blocks)
- `GET /blocks/:number/virtual-ops` - Get the virtual operations of a block (always served from the steem RPC; virtual ops are not persisted locally)

#### Posts
- `GET /posts` - List top-level posts (page/page_size, sort_by/sort_order; limit/sort/order aliases)
- `GET /posts/daily` - Posts by date/tag
- `GET /posts/:author/:permlink` - Get post detail
- `GET /posts/:author/:permlink/replies` - Get post replies
- `GET /posts/:author/:permlink/votes` - Get post votes
- `GET /posts/:author/:permlink/reblogs` - Get post reblogs

#### Labs
- `GET /labs` - Labs index
- `GET /labs/powerup` / `GET /labs/powerdown` - Vesting deposits/withdrawals
- `GET /labs/rshares` / `GET /labs/curation` / `GET /labs/author` - Content reward leaderboards (date, grouping)
- `GET /labs/flags` - Downvote activity
- `GET /labs/clients` - Client app usage snapshot
- `GET /labs/benefactors` - Benefactor rewards by day
- `GET /labs/pending` - Posts approaching payout

#### Witnesses
- `GET /witnesses` - Get witness list (page/limit, sort/order)
- `GET /witnesses/top` - Get top witnesses
- `GET /witnesses/:username` - Get witness information

#### Statistics & Misc
- `GET /stats/global` - Get global statistics
- `GET /stats/props` - Get dynamic global properties (from the steem RPC)
- `GET /dashboard` - Dashboard aggregate (local/ upstream with per-probe degradation)
- `GET /search?q=&type=` - Global search
- `GET /charts/accounts/growth` / `GET /charts/blocks/production` / `GET /charts/transactions/volume` / `GET /charts/witnesses/voting` - Chart data
- `GET /operations/stats` - Operation type statistics (currently returns placeholder data)
- `GET /status`, `GET /health` - Service status

#### Legacy API
- `GET /api/{supply,props,percentage,rshares,downvotes,topwitnesses,rewards,curation,powerup,steem}` - 302 redirects to the v1 equivalents
- `GET /api/token` - Plain-text supply figures

### WebSocket API

Connect to: `ws://localhost:8080/ws` (or `ws://localhost/ws` through Nginx)

#### Subscription Channels
- `blocks` - Real-time block updates (default subscription)
- `props` - Dynamic global properties (default subscription)
- `state` - Collection counts / chain state (default subscription)
- `operation` - Global operation feed (subscribe explicitly)
- `@{account}` - Account-specific updates when the account is mentioned in an operation

New clients are automatically subscribed to `blocks`, `props`, and `state`
and receive a replay of the last 10 irreversible blocks (aligned with legacy
`live.py`).

## ⚙️ Configuration

The application uses YAML configuration files. Key configuration sections:

### Server Configuration
```yaml
server:
  port: 8080
  host: "127.0.0.1"
  mode: "development"
  read_timeout: 30s
  write_timeout: 30s
```

### Database Configuration
```yaml
database:
  mongodb:
    uri: "mongodb://localhost:27017"
    database: "steemdb"
    pool_size: 100
  redis:
    uri: "redis://localhost:6379"
    db: 0
    pool_size: 100
```

### API Configuration
```yaml
api:
  rate_limit:
    enabled: true
    requests_per_minute: 100
  cors:
    enabled: true
    allowed_origins: ["http://localhost:3000"]
```

> Note: the `api.rate_limit` and `cache` sections are parsed but currently
> have no consumers — rate limiting and Redis response caching are not
> implemented.

## 🐳 Docker Deployment

### Directory Structure

Docker-related configuration files are organized in the `docker/` directory:

```
steemdb-web/
├── docker/                    # Docker configuration files
│   ├── nginx/                 # Nginx configuration
│   ├── supervisor/            # Supervisord configuration
│   └── CONFIGURATION.md       # Configuration guide
├── configs/                   # Application configuration (mounted at runtime)
│   └── config.yaml           # Default configuration
└── scripts/                   # Build and test scripts
```

(Docker Compose orchestration lives at the repository root, not in this
directory — see the root `docker-compose.yml` and
`docker-compose.production.yml`.)

### Configuration Mounting

The application configuration (`config.yaml`) is mounted from the host system into the container. This allows you to modify configuration without rebuilding the image.

**Default mount location:**
- Host: `./configs/config.yaml`
- Container: `/app/configs/config.yaml`

**To use a custom configuration:**

1. Edit `configs/config.yaml` on the host system
2. The changes will be available in the container (restart required for changes to take effect)

For detailed configuration instructions, see [docker/CONFIGURATION.md](docker/CONFIGURATION.md).

### Using Docker Compose

The web service is part of the unified Docker Compose setup at the project root.

1. **Navigate to project root**
   ```bash
   cd /path/to/steemdb
   ```

2. **Configure the application** (optional)
   ```bash
   # Edit configuration file
   vim steemdb-web/configs/config.yaml
   ```

3. **Build and start all services**
   ```bash
   docker-compose up --build -d
   ```

4. **View logs**
   ```bash
   # View all logs
   docker-compose logs -f
   
   # View web service logs only
   docker-compose logs -f steemdb-web
   ```

5. **Restart after configuration changes**
   ```bash
   docker-compose restart steemdb-web
   ```

**Note**: The unified `docker-compose.yml` at the project root orchestrates all services including MongoDB, Redis, Prometheus, and Grafana.

### Using Docker

1. **Build the image**
   ```bash
   # From project root
   docker build -f steemdb-web/Dockerfile -t steemdb-web .
   ```

2. **Run the container**
   ```bash
   docker run -d \
     -p 80:80 \
     -v $(pwd)/steemdb-web/configs:/app/configs \
     -e MONGODB_URI=mongodb://mongo:27017 \
     -e REDIS_URI=redis://redis:6379 \
     --name steemdb-web \
     steemdb-web
   ```

### Access Points

- Frontend: `http://localhost/`
- API: `http://localhost/api/v1/...`
- WebSocket: `ws://localhost/ws`
- Health Check: `http://localhost/health`

## 📊 Monitoring

### Health Checks
- Health endpoint: `GET /health`
- Readiness endpoint: `GET /ready` (pings MongoDB and Redis)

The web service itself does **not** expose a `/metrics` endpoint; the
`metrics.*` config keys are currently unused. Prometheus in the compose
stacks scrapes the sync services (`live-sync` `:9091`, `processor` `:9092`,
`steemdb-refresher` `:9093`), not this service.

### Logging
- Structured JSON logging in production
- Configurable log levels (debug, info, warn, error)
- Log rotation and archival

## 🧪 Testing

### Run Tests
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/api/...
```

### Load Testing
```bash
# Install hey (HTTP load testing tool)
go install github.com/rakyll/hey@latest

# Test API endpoints
hey -n 1000 -c 10 http://localhost:8080/api/v1/blocks
```

## 🚀 Production Deployment

### Environment Variables
```bash
export SERVER_MODE=production
export MONGODB_URI=mongodb://prod-mongo:27017     # alias of DATABASE_MONGODB_URI
export REDIS_URI=redis://prod-redis:6379          # alias of DATABASE_REDIS_URI
```

Other bound variables (via Viper's env replacer): `SERVER_PORT`,
`SERVER_HOST`, `AUTH_JWT_SECRET` (note: the auth feature itself is not
implemented yet).

### Systemd Service
```ini
[Unit]
Description=SteemDB Web Service
After=network.target

[Service]
Type=simple
User=steemdb
WorkingDirectory=/opt/steemdb-web
ExecStart=/opt/steemdb-web/steemdb-web configs/production.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Nginx Configuration
```nginx
upstream steemdb_web {
    server 127.0.0.1:8080;
    server 127.0.0.1:8081;
    server 127.0.0.1:8082;
}

server {
    listen 80;
    server_name api.steemdb.com;
    
    location / {
        proxy_pass http://steemdb_web;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
    
    location /ws {
        proxy_pass http://steemdb_web;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🆘 Support

- **Documentation**: [README.md](README.md), [docker/CONFIGURATION.md](docker/CONFIGURATION.md)
- **Issues**: [GitHub Issues](https://github.com/steemit/steemdb/issues)
- **Discussions**: [GitHub Discussions](https://github.com/steemit/steemdb/discussions)

## 🗺 Roadmap

- [x] Core API implementation (accounts, blocks, posts, labs, witnesses, stats, search, charts)
- [x] WebSocket real-time features
- [x] Frontend React application (see `steemdb-frontend/`)
- [x] Legacy API compatibility (302 redirects)
- [ ] Response caching and rate limiting (config exists, not wired)
- [ ] Horizontal scaling support
- [ ] GraphQL API
- [ ] Mobile API optimizations
