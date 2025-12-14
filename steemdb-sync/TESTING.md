# Testing Guide - Temporary Files and Cleanup

## Testing with Docker (Recommended)

The easiest and cleanest way to run tests is using the Docker test environment:

```bash
# Run all tests with automatic Docker setup/cleanup
./scripts/run_tests_with_docker.sh

# Or run specific tests
./scripts/run_tests_with_docker.sh validation
./scripts/run_tests_with_docker.sh performance
```

This automatically:
- ✅ Creates a temporary MongoDB container
- ✅ Runs the tests
- ✅ Cleans up containers and volumes when done

## Temporary Files Generated During Testing

When running cold start validation and performance tests, the following temporary files and directories may be created:

### Test Temporary Directory

- **Location**: `test-tmp/` (in project root)
- **Contents**: 
  - `block_query_time.txt` - Block query performance metrics
  - `latest_blocks_time.txt` - Latest blocks query metrics
  - `account_query_time.txt` - Account query performance metrics
  - `account_ops_query_time.txt` - Account operations query metrics

### Docker Test Environment

- **Container**: `steemdb-sync-test-mongo` (automatically removed)
- **Volume**: `steemdb-sync_mongodb-test-data` (automatically removed with `-v` flag)
- **Port**: `27018` (to avoid conflicts with main MongoDB)

### Other Test Artifacts

- **Test binaries**: `bin/validate` (performance test tool)
- **Test logs**: Any log files generated during testing
- **Test database dumps**: If database dumps are created for testing

## Git Ignore Configuration

All test-related temporary files are automatically ignored by `.gitignore`:

```gitignore
# Test and validation artifacts
test-tmp/
test_data/
validation_output/
cold_start_test/
performance_test_results/
test_db/
*.dump
*.bson

# Test binaries
bin/validate
bin/validate.exe
test_validate

# Docker test volumes
docker-test-data/
```

## Cleanup

### Automatic Cleanup

Both test scripts automatically clean up:

1. **Temporary files**: Removed on script exit
2. **Docker containers**: Removed when `USE_DOCKER=true` and script exits
3. **Docker volumes**: Removed with `docker-compose down -v`

### Manual Cleanup

If you need to manually clean up test artifacts:

```bash
# Remove test temporary directory
rm -rf test-tmp/

# Remove test binaries
rm -f bin/validate bin/validate.exe

# Remove test logs
rm -f *.log test*.log

# Clean up Docker test environment
docker-compose -f docker-compose.test.yml down -v

# Remove orphaned containers (if any)
docker ps -a | grep steemdb-sync-test | awk '{print $1}' | xargs docker rm -f

# Remove test volumes (if any)
docker volume ls | grep steemdb-sync-test | awk '{print $2}' | xargs docker volume rm
```

### Before Committing

Before committing changes, ensure no test artifacts are included:

```bash
# Check for untracked test files
git status

# If test-tmp/ or other test artifacts appear, they should be ignored
# If they're not ignored, check .gitignore configuration
```

## Best Practices

1. **Use Docker test environment**: This ensures complete isolation and automatic cleanup
   ```bash
   ./scripts/run_tests_with_docker.sh
   ```

2. **Always run tests in project directory**: This ensures temporary files are created in the correct location and properly ignored.

3. **Check git status before committing**: 
   ```bash
   git status
   ```
   Verify that test artifacts are not being tracked.

4. **Use test-tmp/ directory**: All test scripts use `test-tmp/` for temporary files, which is already in `.gitignore`.

5. **Clean up after testing**: While scripts auto-cleanup, you can manually remove `test-tmp/` if needed.

## Running Tests Safely

### With Docker (Recommended)

```bash
# 1. Verify Docker is running
docker ps

# 2. Run tests (automatic setup and cleanup)
./scripts/run_tests_with_docker.sh

# 3. Check git status (should not show test artifacts)
git status
```

### Without Docker

```bash
# 1. Verify .gitignore is up to date
cat .gitignore | grep test

# 2. Run validation
./scripts/cold_start_validation.sh

# 3. Run performance tests
./scripts/performance_test.sh

# 4. Check git status (should not show test artifacts)
git status

# 5. Clean up if needed
rm -rf test-tmp/
```

## Docker Test Environment Details

### Configuration

The test environment is defined in `docker-compose.test.yml`:

- **MongoDB Image**: `mongo:7.0`
- **Port**: `27018` (to avoid conflicts)
- **Volume**: Named volume (automatically removed)
- **Network**: Isolated bridge network

### Manual Docker Operations

If you need to manually manage the test environment:

```bash
# Start test MongoDB
docker-compose -f docker-compose.test.yml up -d

# Check status
docker-compose -f docker-compose.test.yml ps

# View logs
docker-compose -f docker-compose.test.yml logs -f

# Stop and remove (with volumes)
docker-compose -f docker-compose.test.yml down -v
```

### Environment Variables

When using Docker, scripts automatically set:
- `USE_DOCKER=true`
- `MONGODB_URI=mongodb://localhost:27018`
- `DOCKER_COMPOSE_FILE=./docker-compose.test.yml`

You can override these:
```bash
USE_DOCKER=true MONGODB_URI=mongodb://localhost:27019 ./scripts/cold_start_validation.sh
```

## Troubleshooting

### Test files appearing in git status

If test artifacts appear in `git status`:

1. Check `.gitignore` includes the patterns:
   ```bash
   grep -E "(test-tmp|test_data|validation)" .gitignore
   ```

2. If files were already tracked, remove them:
   ```bash
   git rm --cached test-tmp/*.txt
   git rm --cached bin/validate
   ```

3. Verify `.gitignore` is in the correct location (project root or steemdb-sync/)

### Docker issues

**Port already in use:**
```bash
# Check what's using port 27018
lsof -i :27018

# Or use a different port
MONGODB_URI=mongodb://localhost:27019 USE_DOCKER=true ./scripts/cold_start_validation.sh
```

**Container not starting:**
```bash
# Check Docker logs
docker-compose -f docker-compose.test.yml logs

# Check Docker is running
docker ps
```

**Cleanup not working:**
```bash
# Force cleanup
docker-compose -f docker-compose.test.yml down -v --remove-orphans

# Remove manually if needed
docker rm -f steemdb-sync-test-mongo
docker volume rm steemdb-sync_mongodb-test-data
```

### Temporary files in wrong location

If scripts create files in `/tmp/` instead of `test-tmp/`:

1. Check script uses `$TEST_TMP_DIR` variable
2. Verify `TEST_TMP_DIR` is set to project-relative path
3. Update script if needed
