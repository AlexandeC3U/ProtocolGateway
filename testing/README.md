# Testing Quick Start

Run tests quickly with these commands:

```bash
# Unit tests
make test

# With coverage
make test-cover

# Benchmarks
make bench

# Integration (needs simulators)
docker-compose -f docker-compose.test.yaml up -d
make test-integration
```

See [info.md](info.md) for the complete testing documentation.
