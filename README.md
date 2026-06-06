# restClean

## Table of contents

* [General info](#general-info)
* [Technologies](#technologies)
* [Quickstart](#quickstart)
* [Setup](#setup)
* [API](#api)
* [Architecture](#architecture)
* [Migrations](#migrations)

## General Info

Small REST API built with clean architecture principles.  

## Technologies

* Go 1.26
* PostgreSQL 18.x
* Redis 8.x

## Quickstart

```bash
cp .env.example .env
# fill in required values in .env
make migrate-up
make up
```

## Setup

Copy `.env.example` to `.env` and fill in the required values.  
All available variables with their defaults are documented in `.env.example`.

## API

```bash
# create a product
curl -s -X POST http://localhost:7000/product \
  -H 'Content-Type: application/json' \
  -d '{"name":"widget","price":{"minorAmount":999,"currency":"PLN"}}'

# get a product by id
curl -s http://localhost:7000/product/{id}
```

See `api.rest` for the full set of example requests.

## Architecture

`cmd/` → `httpapi` → `service` → `repository`, with `cache` and `entity` as cross-cutting packages.

## Migrations

Schema changes live in `internal/migrate/migrations/` and are bundled into the binary via `embed.FS`.  
The `cmd/migrate` CLI applies them using [goose](https://github.com/pressly/goose).

```bash
make migrate-up       # apply all pending migrations
make migrate-status   # show applied and pending migrations
make migrate-down     # roll back the last migration (local dev only)
```

The application performs a fail-fast check at startup and refuses to run if the database schema is older than the embedded migrations.
