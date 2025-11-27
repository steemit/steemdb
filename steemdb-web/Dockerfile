# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o steemdb-web cmd/web/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1001 -S steemdb && \
    adduser -u 1001 -S steemdb -G steemdb

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/steemdb-web .

# Copy configuration files
COPY --from=builder /app/configs ./configs

# Change ownership
RUN chown -R steemdb:steemdb /app

# Switch to non-root user
USER steemdb

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./steemdb-web", "configs/config.yaml"]
