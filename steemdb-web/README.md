# SteemDB Web Service

A modern, high-performance web service for SteemDB blockchain explorer, built with Go and modern web technologies.

## 🚀 Features

### Core API Services
- **Account API**: Account information, history, and statistics
- **Block API**: Block data, transactions, and operations
- **Witness API**: Witness information, voting, and performance metrics
- **Statistics API**: Network statistics and analytics
- **Search API**: Advanced search functionality

### Real-time Features
- **WebSocket Support**: Real-time blockchain data streaming
- **Live Updates**: Real-time block and transaction notifications
- **Event Streaming**: Subscribe to specific blockchain events

### Performance & Reliability
- **High Performance**: Built with Go for optimal performance
- **Caching**: Redis-based caching for fast data retrieval
- **Rate Limiting**: Configurable API rate limiting
- **Health Checks**: Comprehensive health and readiness checks

### Security & Authentication
- **JWT Authentication**: Secure API access with JWT tokens
- **CORS Support**: Configurable CORS for web applications
- **Input Validation**: Comprehensive request validation

## 🛠 Technology Stack

- **Backend**: Go 1.23, Gin Web Framework
- **Database**: MongoDB (primary), Redis (cache)
- **WebSocket**: Gorilla WebSocket
- **Authentication**: JWT tokens
- **Monitoring**: Prometheus metrics
- **Logging**: Structured logging with Zap
- **Configuration**: YAML-based configuration with Viper

## 📁 Project Structure

```
steemdb-web/
├── cmd/web/                # Main application entry point
├── internal/
│   ├── api/               # API handlers and routes
│   ├── database/          # Database connections and operations
│   ├── models/            # Data models and structures
│   ├── services/          # Business logic services
│   ├── middleware/        # HTTP middleware
│   └── websocket/         # WebSocket handlers
├── pkg/
│   ├── auth/              # Authentication utilities
│   └── utils/             # Common utilities and helpers
├── web/
│   ├── src/               # Frontend source code
│   └── public/            # Static assets
├── configs/               # Configuration files
├── scripts/               # Build and deployment scripts
└── docs/                  # Documentation
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
   cp configs/config.yaml configs/local.yaml
   # Edit configs/local.yaml with your settings
   ```

4. **Start the services**
   ```bash
   # Start MongoDB and Redis (using Docker)
   docker-compose up -d mongo redis
   
   # Start the web service
   go run cmd/web/main.go configs/local.yaml
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
- `GET /accounts/:name` - Get account information
- `GET /accounts/:name/history` - Get account history
- `GET /accounts/:name/posts` - Get account posts
- `GET /accounts/:name/votes` - Get account votes

#### Blocks
- `GET /blocks` - Get recent blocks
- `GET /blocks/:number` - Get specific block
- `GET /blocks/:number/operations` - Get block operations

#### Witnesses
- `GET /witnesses` - Get witness list
- `GET /witnesses/:name` - Get witness information
- `GET /witnesses/:name/votes` - Get witness votes

#### Statistics
- `GET /stats/global` - Get global statistics
- `GET /stats/accounts` - Get account statistics
- `GET /stats/witnesses` - Get witness statistics

### WebSocket API

Connect to: `ws://localhost:8080/ws`

#### Subscription Channels
- `blocks` - Real-time block updates
- `operations` - Real-time operation updates
- `accounts:{name}` - Account-specific updates
- `witnesses` - Witness updates

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
    addr: "localhost:6379"
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
└── docker-compose.yml        # Docker Compose configuration
```

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
     -e REDIS_ADDR=redis:6379 \
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
- Readiness endpoint: `GET /ready`
- Metrics endpoint: `GET /metrics` (Prometheus format)

### Logging
- Structured JSON logging in production
- Configurable log levels (debug, info, warn, error)
- Log rotation and archival

### Metrics
- HTTP request metrics
- Database operation metrics
- WebSocket connection metrics
- Custom business metrics

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
export MONGODB_URI=mongodb://prod-mongo:27017
export REDIS_ADDR=prod-redis:6379
export JWT_SECRET=your-production-secret
```

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

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/steemdb/web/issues)
- **Discussions**: [GitHub Discussions](https://github.com/steemdb/web/discussions)

## 🗺 Roadmap

- [ ] Complete API implementation
- [ ] WebSocket real-time features
- [ ] Frontend React application
- [ ] Advanced caching strategies
- [ ] Horizontal scaling support
- [ ] GraphQL API
- [ ] Mobile API optimizations
