# Docker Configuration Directory

This directory contains all Docker-related configuration files for the `steemdb-web` service.

## Directory Structure

```
docker/
├── nginx/
│   └── nginx.conf          # Nginx configuration for static file serving and reverse proxy
├── supervisor/
│   └── supervisord.conf    # Supervisord configuration for process management
└── README.md               # This file
```

## Configuration Files

### Nginx Configuration (`nginx/nginx.conf`)

- Static file serving for frontend build artifacts
- Reverse proxy for API endpoints (`/api/*`)
- WebSocket proxy (`/ws`)
- SPA routing support
- Gzip compression
- Static file caching
- Security headers

### Supervisord Configuration (`supervisor/supervisord.conf`)

- Manages Nginx process (priority 100, starts first)
- Manages Go backend process (priority 200, starts after Nginx)
- Log rotation and size limits
- Auto-restart on failure

## Application Configuration

The application configuration (`config.yaml`) is **not** stored in this directory. It should be mounted from the host system at runtime.

See the main README.md for configuration instructions.

