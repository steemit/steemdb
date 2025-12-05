# Frontend build stage
FROM node:20-alpine AS frontend-builder

# Install pnpm
RUN npm install -g pnpm

# Set working directory
WORKDIR /app

# Copy frontend package files
COPY steemdb-frontend/package.json steemdb-frontend/pnpm-lock.yaml ./

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy frontend source code
COPY steemdb-frontend/ ./

# Build frontend (set NODE_ENV=production for production build)
ENV NODE_ENV=production
RUN pnpm build

# Go build stage
FROM golang:1.23-alpine AS go-builder

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY steemdb-web/go.mod steemdb-web/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY steemdb-web/ .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o steemdb-web cmd/web/main.go

# Final stage
FROM alpine:latest

# Install required packages
RUN apk --no-cache add \
    ca-certificates \
    nginx \
    supervisor \
    wget

# Create directories
RUN mkdir -p /app /var/log/nginx /var/log/steemdb-web /var/log/supervisor \
    && mkdir -p /etc/nginx/conf.d /etc/supervisor/conf.d

# Create non-root user
RUN addgroup -g 1001 -S steemdb && \
    adduser -u 1001 -S steemdb -G steemdb

# Set working directory
WORKDIR /app

# Copy Go binary from builder stage
COPY --from=go-builder /app/steemdb-web .

# Copy configuration files
COPY --from=go-builder /app/configs ./configs

# Copy frontend build artifacts
COPY --from=frontend-builder /app/dist /usr/share/nginx/html

# Copy Nginx configuration
COPY steemdb-web/docker/nginx/nginx.conf /etc/nginx/nginx.conf

# Copy Supervisord configuration
COPY steemdb-web/docker/supervisor/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Create log directories and set permissions
RUN chown -R steemdb:steemdb /app /var/log/steemdb-web && \
    chown -R nginx:nginx /var/log/nginx /usr/share/nginx/html && \
    chmod -R 755 /usr/share/nginx/html

# Expose port 80 (Nginx)
EXPOSE 80

# Health check (check Nginx)
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost/health || exit 1

# Run supervisord
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
