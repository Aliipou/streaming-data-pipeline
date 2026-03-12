.PHONY: run-producer run-processor run-api test up down

run-producer:
	go run ./cmd/producer

run-processor:
	go run ./cmd/processor

run-api:
	go run ./cmd/api

test:
	go test ./... -race -count=1

up:
	docker compose -f deployments/docker-compose.yml up -d

down:
	docker compose -f deployments/docker-compose.yml down
