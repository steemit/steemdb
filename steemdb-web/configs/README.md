# Configuration Directory

This directory contains application configuration files for `steemdb-web`.

## Default Configuration

- `config.yaml` - Default configuration file (used in development)

## Configuration Mounting

When running in Docker, this directory is mounted into the container at `/app/configs`.

### Docker Compose

The `docker-compose.yml` file includes:

```yaml
volumes:
  - ./configs:/app/configs
```

This means:
- Changes to files in this directory on the host are immediately available in the container
- **Note**: Application restart is required for configuration changes to take effect

### Custom Configuration

To use a different configuration file:

1. Create a new config file (e.g., `production.yaml`)
2. Update `docker-compose.yml`:

```yaml
volumes:
  - ./configs/production.yaml:/app/configs/config.yaml
```

Or mount a different directory:

```yaml
volumes:
  - /path/to/your/configs:/app/configs
```

## Configuration Structure

See `config.yaml` for the complete configuration structure. Key sections:

- `server` - Server settings (port, host, mode)
- `database` - MongoDB and Redis connection settings
- `api` - API configuration (rate limiting, CORS)
- `websocket` - WebSocket settings
- `steem` - Steem blockchain node configuration
- `cache` - Redis cache settings
- `log` - Logging configuration
- `metrics` - Prometheus metrics configuration

## Environment Variables

Configuration values can be overridden using environment variables. See `docker/CONFIGURATION.md` for details.

## Security

⚠️ **Important**: Never commit sensitive values (passwords, secrets, API keys) to version control.

For production:
- Use environment variables
- Use secrets management systems
- Set appropriate file permissions: `chmod 600 configs/production.yaml`

