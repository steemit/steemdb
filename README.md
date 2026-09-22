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

- **RESTful API**: Modern web API for accessing blockchain data
- **WebSocket Support**: Real-time data streaming for live updates
- **Modern Web Interface**: React-based frontend with TypeScript
- **Comprehensive Monitoring**: Prometheus metrics and Grafana dashboards

### Key Features

- ⚡ **High Performance**: Modern Go implementation
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
│  ┌──────────────┐    ┌──────────────┐ │
│  │  steemdb-    │    │  steemdb-    │ │
│  │  web         │    │  frontend    │ │
│  │              │    │              │ │
│  │  • REST API  │    │  • React UI  │ │
│  │  • WebSocket │    │  • TypeScript│ │
│  │  • Nginx     │    │  • Tailwind  │ │
│  │  • Go Backend│    │  • Vite      │ │
│  └──────┬───────┘    └──────┬───────┘ │
│         │                    │                    │         │
│         └────────────────────┘                    │
│                                                    │
│         ┌────────────────────┐                   │
│         │                                          │         │
│    ┌────▼─────┐                            ┌─────▼────┐    │
│    │ MongoDB  │                            │  Redis   │    │
│    │          │                            │          │    │
│    │ Primary  │                            │ Readiness│    │
│    │ Database │                            │  probe   │    │
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

- **Backend Services**: Go (1.23 for steemdb-web, 1.25 for steemdb-sync)
- **Frontend**: React 19, TypeScript, Vite, Tailwind CSS
- **Database**: MongoDB (6.0 in `docker-compose.production.yml`, 4.4 in the dev `docker-compose.yml`), Redis 7 (readiness probe; caching not implemented yet)
- **Web Framework**: Gin
- **WebSocket**: Gorilla WebSocket
- **Monitoring**: Prometheus, Grafana (scrape the sync services)
- **Containerization**: Docker, Docker Compose
- **Process Management**: Supervisord

## 📦 Projects

### steemdb-web

Modern web service providing RESTful API and WebSocket support, with full legacy API compatibility.

**Features:**
- RESTful API for accounts, blocks, witnesses, posts, and statistics
- Posts API: List, detail, replies, votes, and reblogs
- Labs API: PowerUp, PowerDown, Rshares, Curation/Author leaderboards, Flags, Clients, Benefactors, Pending posts
- Legacy API compatibility: 302 redirects for backward compatibility
- WebSocket real-time data streaming (aligned with legacy live.py functionality)
- Nginx integration for static file serving
- Health checks (`/health`, `/ready`)

**Documentation**: [steemdb-web/README.md](steemdb-web/README.md)

### steemdb-frontend

Modern React-based frontend application with full feature parity to legacy system.

**Features:**
- React 19 with TypeScript
- Tailwind CSS for styling
- Zustand for state management
- TanStack Query for data fetching
- React Router DOM for navigation
- Recharts for data visualization
- **Posts Pages**: List, detail, replies, votes, and reblogs
- **Labs Pages**: PowerUp, PowerDown, Rshares, Curation, Author, Flags, Clients, Benefactors, Pending
- **Dashboard**: Network performance, reward pool, and global properties
- **Accounts**: Account details, history, and statistics
- **Witnesses**: Witness information and voting

**Documentation**: [steemdb-frontend/README.md](steemdb-frontend/README.md)

## 🚀 Quick Start

### Prerequisites

- **Docker** 20.10+ and **Docker Compose** 2.0+
- **Go** 1.23+ (for development)
- **Node.js** 18+ and **pnpm** 8+ (for frontend development)
- **MongoDB** (included via Docker Compose: 4.4 in the dev stack, 6.0 in the production stack)
- **Redis** 7+ (included via Docker Compose)

### Using Docker Compose (Recommended)

This is the fastest way to get started with all services running.

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd steemdb
   ```

2. **Configure services** (optional)
   ```bash
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
   ```

5. **Access services**
   - **Frontend**: http://localhost/
   - **API**: http://localhost/api/v1/
   - **WebSocket**: ws://localhost/ws
   - **MongoDB**: localhost:27017
   - **Redis**: localhost:6379

### Manual Development Setup

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

Two compose files cover two different purposes — pick deliberately:

- **`docker-compose.yml`** (dev/demo): web + refresher + mongo/redis + monitoring.
  It does **not** run the data writers (processor/live_sync), so Posts/Labs/Accounts
  pages stay empty until a writer fills the derived collections.
- **`docker-compose.production.yml`** (production, single-box all-in-one): the six
  resident services — steemdb-web, processor, live-sync, steemdb-refresher, mongo,
  redis — plus prometheus (real scrape config) and grafana. See
  [Production Topology](#production-topology) below.

**Available Services (`docker-compose.yml`, dev):**
- `steemdb-web` - Web API service with Nginx and frontend
- `steemdb-refresher` - witness/stats/funds/clients snapshots
- `mongo` - MongoDB database
- `redis` - Redis cache
- `prometheus` / `grafana` - monitoring (scrapes the refresher metrics)

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

`steemdb-web` supports configuration mounting from the host, allowing you to modify settings without rebuilding images:

- **Web Service**: `./steemdb-web/configs:/app/configs`

**To modify configuration:**

1. Edit configuration files on the host:
   ```bash
   # Edit web service config
   vim steemdb-web/configs/config.yaml
   ```

2. Restart the service to apply changes:
   ```bash
   docker-compose restart steemdb-web
   ```

**For detailed configuration instructions:**
- [steemdb-web/docker/CONFIGURATION.md](steemdb-web/docker/CONFIGURATION.md)

### Production Topology

The production stack is the single-box, all-in-one topology described in
`docs/AI-driver/01-architecture-overview.md`. It is codified in
**`docker-compose.production.yml`**:

| Service | Role | Metrics |
|---|---|---|
| `steemdb-web` | REST API + WebSocket + Nginx + SPA (only published port) | - |
| `processor` | `operations` -> derived collections (the only writer of posts/votes/...) | `:9092/metrics` |
| `live-sync` | RPC catch-up + chain-head follow (started via the `live` profile) | `:9091/metrics` |
| `steemdb-refresher` | witness/stats/funds/clients snapshots | `:9093/metrics` |
| `mongo` | raw + derived data (sync side is the writer & index authority) | - |
| `redis` | cache | - |

Shared-Mongo contract: the `steemdb` database is written by the sync side only;
`steemdb-web` connects to the same database **read-only**. The collection/field
contract is documented in `docs/AI-driver/02-data-model.md`. Mongo and Redis are
not published to the host (compose network only).

**Cold start & steady state** (cold_ingest/steemd/repair are tools, not resident
services — details in the header of `docker-compose.production.yml`):

> **Data handoff is a manual step.** The cold-start stack
> (`steemdb-sync/test/docker-compose/`) runs its **own** Mongo (mongo:4.4,
> auth-enabled, database `steemdb_test`, separate volume), while the production
> compose runs a different mongo:6.0 on a fresh **empty** volume. Nothing
> copies the replayed data between the two. Skip the handoff in step 1 below
> and the processor starts against an empty database while live-sync would
> re-fetch the entire chain from block 1 over RPC — the months-long refetch
> the cold replay exists to avoid.

1. **Replay the chain with the cold-start stack** (`steemdb-sync/test/docker-compose/`:
   steemd ingest plugin pushes ops to cold_ingest; both retire when the replay
   finishes), writing into the **production** Mongo via one of:

   - **Path A (recommended; matches the original design — the replay writes
     production directly).** Bring up the production mongo alone first:

     ```bash
     docker compose -f docker-compose.production.yml up -d mongo
     ```

     Then run the cold-start stack with its writers repointed at that mongo
     via a local compose override (not shipped — the shipped test compose is a
     test fixture): set `cold_ingest`'s `MONGO_URI` to the production mongo
     with database **`steemdb`** (not `steemdb_test`). Either attach the
     `cold-ingest` container to the production compose network (default name
     `steemdb_steemdb-network`; check `docker network ls`) and use
     `mongodb://mongo:27017/steemdb`, or publish the production mongo on
     loopback (the commented-out `127.0.0.1:27017` port in its service block)
     and point the URI at the host. Add credentials if mongo auth is enabled.
     Set `MONGO_DATABASE=steemdb` in the cold stack's `.env` as well — it
     builds the cold stack's `MONGO_URI` from that variable, and an explicit
     `MONGO_DATABASE` wins over a URI's database name, so both must agree on
     `steemdb`.
     Only the receiver (`cold-ingest`) and `steemd` are needed for the replay —
     do **not** also run the cold stack's processor/live-sync against the
     production database (the production processor does the catch-up; a second
     processor would race it on the same status cursor).
     *Trade-off:* zero-copy and no version boundary to cross. Mounting the
     production `mongo_data` volume into the cold stack's mongo:4.4 instead is
     also possible but drags 4.4→6.0 in-place-upgrade and
     auth-initialization caveats with it — prefer repointing the URI.

   - **Path B (fallback — replay into `steemdb_test`, then dump/restore).**
     Let the replay land in the cold stack's own database, stop its writers,
     then move the data (logical dump/restore is the supported way across
     4.4 → 6.0 — never copy data files between major versions; mongo:6.0
     images no longer bundle the tools, so use the
     `mongodb/mongodb-database-tools` image or a host install):

     ```bash
     # Source URI (auth) is built from MONGO_USERNAME/MONGO_PASSWORD/
     # MONGO_DATABASE in the cold stack's .env (steemdb-sync/test/docker-compose/).
     mongodump --uri="mongodb://<user>:<pass>@<cold-mongo-host>:27017/steemdb_test?authSource=admin" \
       --archive=steemdb.archive
     mongorestore --uri="mongodb://<prod-mongo-host>:27017" \
       --nsFrom=steemdb_test --nsTo=steemdb --archive=steemdb.archive
     ```

     *Trade-off:* slower and needs disk headroom for the archive, but leaves
     both stacks untouched.

2. **Verify the handoff in the production Mongo** (database `steemdb`; N = the
   replayed target height — counts must line up before anything else starts):

   ```js
   db.blocks.countDocuments()                       // == N (blocks are numbered 1..N)
   db.operations.countDocuments()                   // == ops cold_ingest reported
   db.meta.findOne({_id: "sync_state"}).max_block   // == N
   ```

   Also confirm **zero block gaps** over the range (the `repair` binary can
   scan for gaps), then start the production stack **without** the live
   profile — processor catches up over the replayed operations.

3. Hand off to live sync: `docker compose -f docker-compose.production.yml --profile live up -d live-sync`
   (resumes from `meta.max_block` / highest block, then follows the chain head).

4. `repair` is an ad-hoc maintenance binary built into the steemdb-sync image
   (run via `docker compose ... run --rm processor /app/repair ...`).

**Setup:**

```bash
cp .env.production.example .env   # set GRAFANA_ADMIN_PASSWORD (required) etc.
docker compose -f docker-compose.production.yml up -d --build
```

### Production Deployment

1. **Build production images**
   ```bash
   docker compose -f docker-compose.production.yml build
   ```

2. **Set environment variables** (via `.env`, see `.env.production.example`)
   ```bash
   # Required
   GRAFANA_ADMIN_PASSWORD=<secret>
   # Common overrides
   RPC_ENDPOINT=https://api.steemit.com
   MONGO_URI=mongodb://mongo:27017/steemdb        # sync-side writers
   WEB_MONGODB_URI=mongodb://mongo:27017/         # web (db name from its config)
   ```

3. **Deploy with the production compose**
   ```bash
   docker compose -f docker-compose.production.yml up -d
   ```

4. **Set up reverse proxy** (if needed)
   - Configure Nginx or Traefik for SSL termination
   - Point to `http://localhost:80` (or `${WEB_PORT}`) for the web service

## 💻 Development

### Project Structure

```
steemdb/
├── docker-compose.yml          # Dev/demo Docker Compose configuration
├── docker-compose.production.yml # Production topology (single-box all-in-one)
├── steemdb-web/                # Web API service
│   ├── cmd/web/                # Main entry point
│   ├── internal/               # Internal packages
│   │   ├── api/                # API handlers and routes
│   │   ├── database/           # Database connections
│   │   ├── models/             # Data models
│   │   └── services/           # Business logic (incl. WebSocket service)
│   ├── pkg/                    # Public packages (steem RPC client, utils)
│   ├── docker/                 # Docker configurations (nginx, supervisor)
│   └── configs/                # Configuration files
├── steemdb-sync/               # Data sync service
│   ├── cmd/                    # cold_ingest / live_sync / processor / refresher / repair
│   ├── internal/               # config, metrics, model, mongo, pipeline, processor, refresher, rpc, checker
│   └── configs/                # Configuration files
├── steemdb-frontend/           # React frontend
│   ├── src/                    # Source code
│   └── public/                 # Static assets
└── legacy/                     # Legacy PHP application (reference only)
```

### Development Workflow

1. **Start development environment**
   ```bash
   # Start dependencies (MongoDB, Redis)
   docker-compose up -d mongo redis
   
   # Run web service locally
   cd steemdb-web && go run cmd/web/main.go configs/config.yaml
   
   # Run frontend in dev mode
   cd steemdb-frontend && pnpm run dev
   ```

2. **Run tests**
   ```bash
   # Web service tests
   cd steemdb-web && go test ./...
   
   # Sync service tests
   cd steemdb-sync && go test ./...
   
   # Frontend: lint + type-aware build (no test framework is configured yet)
   cd steemdb-frontend && pnpm run lint && pnpm run build
   ```

3. **Build for production**
   ```bash
   # Build web service
   cd steemdb-web && go build -o steemdb-web cmd/web/main.go
   
   # Build frontend
   cd steemdb-frontend && pnpm build
   ```

### Adding New Features

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

Prometheus is configured with real scrape targets (targets are verified against
code, not guessed):

- **Sync services** (production compose, `monitoring/prometheus.yml`):
  - `live-sync` — `http://live-sync:9091/metrics`
  - `processor` — `http://processor:9092/metrics`
  - `steemdb-refresher` — `http://steemdb-refresher:9093/metrics`
- **Dev compose** (`monitoring/prometheus.dev.yml`): scrapes `steemdb-refresher:9093`
  (the only resident sync service there).
- **steemdb-web is not scraped**: it currently starts no metrics server (the
  `metrics.*` config keys are unused defaults). Do not add a web scrape target
  until the web side actually serves `/metrics`.

### Key Metrics (steemdb-sync services)

- `steemdb_sync_current_block` - processed/followed block height
- `steemdb_sync_processor_window_duration_seconds` / `_blocks` / `_ops` - processor window breakdown
- `steemdb_sync_mongo_write_duration_seconds` / `steemdb_sync_mongo_write_total` - Mongo write path
- `steemdb_sync_rpc_latency_seconds` / `steemdb_sync_rpc_total` - Steem RPC calls

### Grafana Dashboards

Grafana is not published to the host by default (reach it via SSH tunnel or
reverse proxy). In the production compose the admin password comes from
`GRAFANA_ADMIN_PASSWORD` in `.env` — there is no baked-in default. Datasource
provisioning is not included; add the prometheus datasource manually.

### Health Checks

All services include health check endpoints:

- **Web Service**: `http://localhost/health` (via Nginx)
- **MongoDB**: Internal health check via mongo shell ping (`mongosh` in the production stack, legacy `mongo` in the 4.4 dev stack)
- **Redis**: Internal health check via `redis-cli ping`
- **Sync services**: health checks via their `/metrics` endpoints (production compose)

**Check service health:**
```bash
# Web service
curl http://localhost/health

# All services status
docker-compose ps
```

## ⚙️ Configuration

### Environment Variables

Environment variables are the ones actually bound in code
(`steemdb-web/pkg/utils/config.go`, `steemdb-sync/internal/config/config.go`):

**Web Service** (viper `BindEnv` + `AutomaticEnv` with `.`->`_`):
- `SERVER_MODE` - Server mode (development, production)
- `DATABASE_MONGODB_URI` (alias `MONGODB_URI`) - MongoDB connection string
- `DATABASE_REDIS_URI` (alias `REDIS_URI`) - Redis connection string
- `AUTH_JWT_SECRET` - JWT secret key (overrides `auth.jwt_secret`)

**Sync services** (processor / live_sync / refresher, `loadFromEnv`):
- `MONGO_URI` - MongoDB connection string (database name parsed from the URI)
- `RPC_ENDPOINT` - Steem RPC endpoint
- `LOG_LEVEL`, `PROCESSOR_WINDOW_SIZE`, `PROCESSOR_BUFFER_LIMIT`,
  `LIVE_SYNC_CHUNK_SIZE`, `LIVE_SYNC_FOLLOW_THRESHOLD`, `LIVE_SYNC_RPC_CONCURRENCY` - tuning

See `.env.production.example` for the variables consumed by
`docker-compose.production.yml`.

### Configuration Files

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
- **Issues**: [GitHub Issues](https://github.com/steemit/steemdb/issues)
- **Discussions**: [GitHub Discussions](https://github.com/steemit/steemdb/discussions)

## 🗺 Roadmap

### Completed ✅
- [x] Complete API implementation (Accounts, Blocks, Witnesses, Posts, Labs)
- [x] Legacy API compatibility (302 redirects)
- [x] WebSocket real-time streaming (aligned with legacy live.py)
- [x] Frontend pages (Posts, Labs, Dashboard, Accounts, Witnesses)
- [x] Docker and Docker Compose deployment
- [x] Environment variable configuration support

### In Progress 🚧
- [ ] Advanced caching strategies
- [ ] Performance optimizations

### Planned 📋
- [ ] Horizontal scaling support
- [ ] GraphQL API
- [ ] Mobile API optimizations
- [ ] Enhanced monitoring and alerting

## 📚 Additional Resources

### Documentation

- [steemdb-web/README.md](steemdb-web/README.md) - Web service documentation
- [steemdb-sync/README.md](steemdb-sync/README.md) - Sync service documentation
- [steemdb-frontend/README.md](steemdb-frontend/README.md) - Frontend documentation
- [steemdb-web/docker/CONFIGURATION.md](steemdb-web/docker/CONFIGURATION.md) - Docker configuration guide

### Project Structure Overview

```
steemdb/
├── README.md                       # This file - main project documentation
├── docker-compose.yml              # Dev/demo Docker Compose configuration
├── docker-compose.production.yml   # Production topology
├── steemdb-web/                    # Web API service (serves the built frontend)
├── steemdb-sync/                   # Data sync service (cold ingest, live sync, processor, refresher, repair)
├── steemdb-frontend/               # React frontend application
└── legacy/                         # Legacy PHP application (reference only)
```

### Quick Reference

**Service Ports:**
- `80` - Web service (Nginx + Frontend + API) — the only published port in production
- `27017` - MongoDB (compose network only, not published)
- `6379` - Redis (compose network only, not published)
- `9091/9092/9093` - live_sync / processor / refresher metrics (compose network only)

**Key Endpoints:**
- Health: `http://localhost/health`
- API: `http://localhost/api/v1/`
- WebSocket: `ws://localhost/ws`

---

**Built with ❤️ for the Steem community**

---

**Note**: This project refactoring was driven by [@ety001](https://github.com/ety001).

