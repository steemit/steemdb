# Configuration Guide

This document explains how to configure `steemdb-web` service from outside the Docker container.

## Configuration File Location

The application configuration file (`config.yaml`) is mounted from the host system into the container at `/app/configs/config.yaml`.

## Default Configuration

The default configuration file is located at:
```
steemdb-web/configs/config.yaml
```

## Mounting Configuration

### Using Docker Compose (Recommended)

The `docker-compose.yml` file already includes a volume mount:

```yaml
volumes:
  - ./configs:/app/configs
```

This mounts the `steemdb-web/configs/` directory from the host to `/app/configs` in the container.

### Using Docker Run

If running the container directly with `docker run`, use the `-v` flag:

```bash
docker run -v /path/to/configs:/app/configs steemdb-web
```

### Custom Configuration Location

To use a custom configuration directory:

1. **Update docker-compose.yml:**

```yaml
volumes:
  - /custom/path/to/configs:/app/configs
```

2. **Or use environment-specific config:**

```yaml
volumes:
  - ./configs/production.yaml:/app/configs/config.yaml
```

## Configuration File Structure

The configuration file uses YAML format. Key sections:

```yaml
server:
  port: 8080
  host: "127.0.0.1"  # Must be 127.0.0.1 (Nginx proxies external requests)
  mode: "production"  # or "development"

database:
  mongodb:
    uri: "mongodb://mongo:27017"
    database: "steemdb"
  redis:
    uri: "redis://redis:6379"

# ... other sections
```

## Environment Variables

You can also override configuration using environment variables in `docker-compose.yml`:

```yaml
environment:
  - SERVER_MODE=production
  - MONGODB_URI=mongodb://mongo:27017
  - REDIS_URI=redis://redis:6379
```

Note: The Go application uses Viper for configuration, which supports environment variable overrides. Check `pkg/utils/config.go` for supported environment variable names.

## Configuration Updates

### Hot Reload

The application does **not** support hot reload of configuration. To apply changes:

1. Edit `configs/config.yaml` on the host system
2. Restart the container:

```bash
docker-compose restart steemdb-web
```

### Verification

After updating configuration, verify it's loaded correctly:

```bash
# Check logs
docker-compose logs steemdb-web

# Check health endpoint
curl http://localhost/health
```

## Production Configuration

For production deployments:

1. **Create production config:**

```bash
cp configs/config.yaml configs/production.yaml
```

2. **Update production.yaml with production values:**
   - Set `server.mode: "production"`
   - Update database URIs
   - Set secure JWT secrets
   - Configure proper logging

3. **Mount production config:**

```yaml
volumes:
  - ./configs/production.yaml:/app/configs/config.yaml
```

## Security Notes

- Never commit sensitive configuration (passwords, secrets) to version control
- Use environment variables or secrets management for sensitive values
- The `configs/` directory should have appropriate file permissions (e.g., `chmod 600`)

## Troubleshooting

### Configuration Not Loading

1. Check volume mount:
```bash
docker-compose exec steemdb-web ls -la /app/configs
```

2. Check file permissions:
```bash
docker-compose exec steemdb-web cat /app/configs/config.yaml
```

3. Check application logs:
```bash
docker-compose logs steemdb-web | grep -i config
```

### Configuration Syntax Errors

Validate YAML syntax:
```bash
# Using Python
python3 -c "import yaml; yaml.safe_load(open('configs/config.yaml'))"

# Or using yq
yq eval '.' configs/config.yaml
```

