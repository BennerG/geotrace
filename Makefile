.PHONY: dev dev-go dev-web build test vet lint

dev:
	docker-compose up -d --build
	@echo "Waiting for postgres..."
	@sleep 3
	@$(MAKE) -j2 dev-go dev-web

dev-go:
	go run ./cmd/server 

dev-web:
	@cd web && npm run dev

build:
	go build -o geotrace ./cmd/server

test:
	go test ./... -race

vet:
	go vet ./...

lint:
	golangci-lint run ./...
