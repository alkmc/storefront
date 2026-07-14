.PHONY: build run test test-race testcontainers testcontainers-race fmt vet deadcode lint check verify proto proto-lint up down logs migrate-up migrate-down migrate-status

build:
	go build ./cmd/server ./cmd/migrate

run:
	go run ./cmd/server

test:
	go test ./...

test-race:
	go test -race ./...

testcontainers:
	go test -tags integration ./...

testcontainers-race:
	go test -race -tags integration ./...

fmt:
	gofumpt -l -w .

vet:
	go vet ./...

deadcode:
	go run golang.org/x/tools/cmd/deadcode@v0.48.0 ./...

lint:
	golangci-lint run

check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

verify:
	go mod verify

proto:
	buf generate

proto-lint:
	buf lint

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
