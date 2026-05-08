.PHONY: build run test test-race testcontainers testcontainers-race fmt vet deadcode lint check tools docker-build up down logs migrate-up migrate-down migrate-status

build:
	go build ./cmd/server ./cmd/migrate

run:
	go run ./cmd/server

test:
	go test ./...

test-race:
	go test -race ./...

testcontainers:
	go test -tags integration ./internal/repository

testcontainers-race:
	go test -race -tags integration ./internal/repository

fmt:
	go fmt ./...

vet:
	go vet ./...

deadcode:
	go run golang.org/x/tools/cmd/deadcode@latest ./...

lint:
	golangci-lint run

check:
	govulncheck ./...

tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

docker-build:
	docker build -t restclean:dev .

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f

migrate-up:
	docker compose run --rm migrate up

migrate-down:
	docker compose run --rm migrate down

migrate-status:
	docker compose run --rm migrate status
