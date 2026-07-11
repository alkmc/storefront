# storefront

## Table of contents

* [General info](#general-info)
* [Technologies](#technologies)
* [Quickstart](#quickstart)
* [Setup](#setup)
* [API](#api)
* [gRPC API](#grpc-api)
* [Architecture](#architecture)
* [Migrations](#migrations)

## General Info

Product catalog API exposed over REST and gRPC.  
PostgreSQL is the source of truth, with goose migrations embedded in the binary.  
Product reads use cache-aside Redis, concurrent misses are coalesced with singleflight.  
Purchase decrements stock atomically with a conditional UPDATE.  

## Technologies

* Go 1.26
* PostgreSQL 18.x
* Redis 8.8

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
# create a product (stock is optional, defaults to 0)
curl -s -X POST http://localhost:8080/v1/product \
  -H 'Content-Type: application/json' \
  -d '{"name":"widget","stock":10,"price":{"minorAmount":999,"currency":"PLN"}}'

# get a product by id
curl -s http://localhost:8080/v1/product/{id}

# list products (keyset pagination)
curl -s 'http://localhost:8080/v1/product?limit=10'
# next page: pass the nextCursor from the previous response
curl -s 'http://localhost:8080/v1/product?limit=10&cursor={nextCursor}'

# purchase units, decrementing stock atomically
curl -s -X POST http://localhost:8080/v1/product/{id}/purchase \
  -H 'Content-Type: application/json' \
  -d '{"quantity":2}'
```

The list endpoint returns `{"items":[...],"nextCursor":"<id>"}`.  
A missing `nextCursor` means the last page.  

Stock is set at creation and changed only through purchase.  
`PUT` does not accept a `stock` field.  
Purchase returns `{"productId":"<id>","quantity":2,"remainingStock":8}`.  
A `409 Conflict` signals insufficient stock.  
See `api.rest` for the full set of example requests.

## gRPC API

A gRPC transport mirrors the HTTP surface over the same service, on `GRPC_PORT` (default `9090`),
with a registered gRPC health service.  
The contract lives in `api/proto/catalog/v1/product.proto` (Protobuf edition 2024).  
Code is generated with [buf](https://buf.build) into `api/gen` (`make proto`).

Server reflection is off by default.  
Set `GRPC_REFLECTION=true` (dev only) and [grpcurl](https://github.com/fullstorydev/grpcurl) discovers the API without the proto:

```bash
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext localhost:9090 describe catalog.v1.ProductService

# create a product (stock is optional, defaults to 0)
grpcurl -plaintext -d '{"name":"widget","stock":10,"price":{"minorAmount":999,"currency":"PLN"}}' \
  localhost:9090 catalog.v1.ProductService/CreateProduct

# get a product by id
grpcurl -plaintext -d '{"id":"<id>"}' \
  localhost:9090 catalog.v1.ProductService/GetProduct

# list products (keyset pagination)
grpcurl -plaintext -d '{"limit":10}' \
  localhost:9090 catalog.v1.ProductService/ListProducts

# purchase units (FAILED_PRECONDITION on insufficient stock)
grpcurl -plaintext -d '{"id":"<id>","quantity":2}' \
  localhost:9090 catalog.v1.ProductService/PurchaseProduct
```

## Architecture

`cmd/` → `transport/http` and `transport/grpc` → `service` → `store`, with `domain` and `cache` as cross-cutting packages.

## Migrations

Schema changes live in `migrate/migrations/` and are bundled into the binary via `embed.FS`.  
The `cmd/migrate` CLI applies them using [goose](https://github.com/pressly/goose).

```bash
make migrate-up       # apply all pending migrations
make migrate-status   # show applied and pending migrations
make migrate-down     # roll back the last migration (local dev only)
```

The application performs a fail-fast check at startup and refuses to run if the database schema is older than the embedded migrations.
