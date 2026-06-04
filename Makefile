BINARY     := geotrace
CMD        := ./cmd/server
GOFLAGS    := -trimpath -ldflags="-s -w"

.PHONY: dev build test lint migrate-up docker-db clean

## dev: run the server with live reload (requires air: go install github.com/air-verse/air@latest)
dev:
	air -c .air.toml

## build: compile a production binary
build:
	go build $(GOFLAGS) -o bin/$(BINARY) $(CMD)

## test: run all tests with race detector
test:
	go test -race -count=1 ./...

## test-store: run only store package tests (no db required for unit tests)
test-store:
	go test -race -count=1 ./internal/store/...

## lint: run golangci-lint (requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	golangci-lint run ./...

## docker-db: start a local Postgres instance for development
docker-db:
	docker run --rm -d \
		--name geotrace-postgres \
		-e POSTGRES_USER=geotrace \
		-e POSTGRES_PASSWORD=password \
		-e POSTGRES_DB=geotrace \
		-p 5432:5432 \
		postgres:16-alpine
	@echo "Postgres ready on localhost:5432"
	@echo "DSN: postgres://geotrace:password@localhost:5432/geotrace?sslmode=disable"

## docker-db-stop: stop the local Postgres instance
docker-db-stop:
	docker stop geotrace-postgres

## clean: remove build artifacts
clean:
	rm -rf bin/

## help: list all targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
