# Integration Tests

This directory contains integration tests for the PostgreSQL cache backend using Testcontainers.

## Prerequisites

- Docker installed and running
- Go 1.21 or later

## Running Integration Tests

Integration tests are separated from unit tests using build tags. They require Docker to be running.

### Run all integration tests

```bash
go test -v -tags=integration ./postgresql/...
```

### Run specific integration test

```bash
go test -v -tags=integration -run TestPostgreSQLCacheIntegration ./postgresql/...
```

### Run CockroachDB tests only

```bash
go test -v -tags=integration -run TestCockroachDBCacheIntegration ./postgresql/...
```

## What is Tested

### PostgreSQL Tests

- Connection pool management
- Cache operations (Get, Set, Delete)
- Concurrent access scenarios
- Table creation and cleanup
- Key prefix functionality

### CockroachDB Tests

- Compatibility with CockroachDB
- UPSERT behavior verification
- Distributed transaction handling
- Concurrent write scenarios

## Test Containers

The integration tests use Testcontainers to spin up:

- **PostgreSQL 16 Alpine** - Lightweight PostgreSQL container
- **CockroachDB v24.2.0** - Distributed SQL database

Containers are automatically:

- Started before tests
- Cleaned up after tests
- Isolated per test run

## Test Structure

```
postgresql/
├── postgresql_integration_test.go  # Integration tests with testcontainers
├── postgresql_test.go              # Unit tests (local DB required)
├── postgresql_bench_test.go        # Benchmark tests
└── test_common.go                  # Shared test constants
```

## Environment Variables

No environment variables are required. The tests use ephemeral containers that are automatically provisioned and torn down.

## Troubleshooting

### Docker not running

```
Error: Cannot connect to the Docker daemon
```

**Solution**: Start Docker Desktop or Docker daemon

### Port already in use

Testcontainers automatically assigns random ports, so port conflicts should not occur.

### Slow tests

Integration tests are slower than unit tests because they:

- Pull Docker images (first run only)
- Start containers
- Wait for database readiness
- Run full test suite

**Tip**: Run unit tests during development and integration tests before commits.

## CI/CD Integration

Integration tests can be run in CI/CD pipelines that support Docker:

```yaml
# Example GitHub Actions workflow
- name: Run Integration Tests
  run: go test -v -tags=integration ./postgresql/...
```

## Test Coverage

Integration tests complement unit tests by testing:

- Real database interactions
- Container startup/shutdown
- Network communication
- Actual SQL query execution
- Transaction isolation

## Performance

Typical run times:

- PostgreSQL tests: ~15-30 seconds (first run may take longer due to image pull)
- CockroachDB tests: ~30-45 seconds
- Total: ~1-2 minutes

## Further Reading

- [Testcontainers Go Documentation](https://golang.testcontainers.org/)
- [PostgreSQL Docker Image](https://hub.docker.com/_/postgres)
- [CockroachDB Docker Image](https://hub.docker.com/r/cockroachdb/cockroach)
