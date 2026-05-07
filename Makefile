.PHONY: build test test-race testcontainers testcontainers-race fmt vet deadcode lint check migrate-up migrate-down migrate-status

build:
	go build ./cmd/inventor ./cmd/migrate

test:
	go test ./cmd/inventor ./cmd/migrate ./internal/cache ./internal/config ./internal/entity ./internal/httpapi ./internal/migrate ./internal/service

test-race:
	go test -race ./cmd/inventor ./cmd/migrate ./internal/cache ./internal/config ./internal/entity ./internal/httpapi ./internal/migrate ./internal/service

testcontainers:
	go test ./internal/repository

testcontainers-race:
	go test -race ./internal/repository

fmt:
	go fmt ./...

vet:
	go vet ./...

deadcode:
	go run golang.org/x/tools/cmd/deadcode@latest ./...

lint:
	golangci-lint run

check:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status
