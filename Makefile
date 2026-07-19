.PHONY: build run test integration fmt deadcode lint check proto proto-lint up down logs migrate-up migrate-down migrate-status

build:
	go build ./cmd/storefront ./cmd/migrate

run:
	go run ./cmd/storefront

test:
	go test -race ./...

integration:
	go test -race -tags integration ./...

fmt:
	gofumpt -l -w .

deadcode:
	go run golang.org/x/tools/cmd/deadcode@v0.48.0 ./...

lint:
	golangci-lint run

check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

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
