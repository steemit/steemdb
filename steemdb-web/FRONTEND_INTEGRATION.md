# Frontend Integration Implementation

## Overview

This document describes the implementation of integrating `steemdb-frontend` into the `steemdb-web` Docker image using Nginx + Supervisord architecture.

## Architecture

- **Nginx**: Serves static frontend files and acts as reverse proxy for API and WebSocket
- **Go Backend**: Provides API endpoints and WebSocket service (listens on 127.0.0.1:8080)
- **Supervisord**: Manages both Nginx and Go processes

## Implementation Details

### 1. Dockerfile Changes

- Added frontend build stage using Node.js 20 and pnpm
- Added Nginx installation in final stage
- Added Supervisord installation in final stage
- Multi-stage build to optimize image size
- Frontend build artifacts copied to `/usr/share/nginx/html`
- Nginx and Supervisord configurations copied to appropriate locations

### 2. Nginx Configuration (`docker/nginx/nginx.conf`)

- Static file serving from `/usr/share/nginx/html`
- API reverse proxy: `/api/*` -> `http://127.0.0.1:8080`
- WebSocket proxy: `/ws` -> `http://127.0.0.1:8080`
- SPA routing support: `try_files $uri $uri/ /index.html`
- Gzip compression enabled (level 6)
- Static asset caching (1 year for assets, 1 hour for HTML)
- Security headers (X-Frame-Options, X-Content-Type-Options, X-XSS-Protection, etc.)
- Health check endpoint proxied to Go backend

### 3. Supervisord Configuration (`docker/supervisor/supervisord.conf`)

- Nginx process management (priority 100, starts first)
- Go backend process management (priority 200, starts after Nginx)
- Log rotation and size limits configured
- Auto-restart enabled for both processes

### 4. Go Service Changes

- Removed static file serving code (`router.Static` and root redirect)
- Service listens on `127.0.0.1:8080` (internal only)
- Health check endpoints (`/health`, `/ready`) retained for monitoring

### 5. Frontend Configuration

- Updated `vite.config.ts` to use relative paths in production (`/api` instead of `http://localhost:8080/api`)
- WebSocket uses relative protocol in production (uses `window.location.host`)
- Development mode still uses absolute URLs for local development

### 6. Docker Compose Changes

- Build context changed to `..` to access both `steemdb-web` and `steemdb-frontend`
- Port mapping updated: `80:80` (Nginx) instead of `8080:8080` (Go)
- Health check updated to check Nginx endpoint (`http://localhost/health`)
- Removed separate Nginx service (now integrated)

## File Structure

```
steemdb-web/
├── Dockerfile                    # Multi-stage build with frontend, Go, Nginx, Supervisord
├── docker/                       # Docker-related configuration files
│   ├── nginx/
│   │   └── nginx.conf            # Nginx configuration
│   ├── supervisor/
│   │   └── supervisord.conf     # Supervisord configuration
│   ├── README.md                 # Docker configuration overview
│   └── CONFIGURATION.md         # Configuration mounting guide
├── configs/                      # Application configuration (mounted at runtime)
│   ├── config.yaml              # Default configuration
│   └── README.md                # Configuration directory guide
├── cmd/web/main.go              # Go service (static file serving removed)
└── docker-compose.yml           # Docker Compose configuration
```

## Build and Deployment

### Build Command

```bash
cd /path/to/steemdb
docker-compose -f steemdb-web/docker-compose.yml build
```

### Run Command

```bash
docker-compose -f steemdb-web/docker-compose.yml up -d
```

### Access Points

- Frontend: `http://localhost/`
- API: `http://localhost/api/v1/...`
- WebSocket: `ws://localhost/ws`
- Health Check: `http://localhost/health`

## Testing Checklist

- [x] Docker image builds successfully
- [x] Nginx serves static files correctly
- [x] API reverse proxy works
- [x] WebSocket proxy works
- [x] SPA routing works (all routes fallback to index.html)
- [x] Gzip compression enabled
- [x] Static file caching configured
- [x] Security headers present
- [x] Supervisord manages both processes
- [x] Health checks work

## Notes

- Go service is only accessible internally (127.0.0.1:8080)
- All external traffic goes through Nginx (port 80)
- Frontend uses relative paths in production for API and WebSocket
- Development mode still uses absolute URLs for local development

