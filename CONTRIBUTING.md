# Contributing to streaming-data-pipeline

## Setup

```bash
git clone https://github.com/Aliipou/streaming-data-pipeline.git
cd streaming-data-pipeline
go mod download
docker compose up -d  # Start Kafka and Redis
```

## Running Tests

```bash
go test ./... -v -race
```

## Code Style

- `gofmt` required
- `golangci-lint run` must pass
- Exported symbols need GoDoc comments
- Use `context.Context` for all I/O operations

## Commit Messages

`feat:`, `fix:`, `docs:`, `test:`, `perf:`, `chore:`
