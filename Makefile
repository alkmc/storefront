.PHONY: build test integration fmt fix lint deadcode vuln proto up down logs migrate-up migrate-down migrate-status token

build:
	go build ./...

test:
	go test -race ./...

integration:
	go test -race -tags integration ./...

fmt:
	golangci-lint fmt

fix:
	go fix -tags integration ./... && golangci-lint run --fix

lint:
	golangci-lint run && buf lint

deadcode:
	go run golang.org/x/tools/cmd/deadcode@v0.52.0 ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

proto:
	buf generate

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

token:
	@set -a && . ./.env && go run ./cmd/token
