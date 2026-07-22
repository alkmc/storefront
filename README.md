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
Only readers populate the cache, and only under a compare-and-set guard, so a reader
that raced a writer loses instead of restoring stale data.  
Writers just tombstone the key.  
Missing products are cached too, under a shorter TTL that bounds memory against scans.  
Purchase decrements stock atomically with a conditional UPDATE.  
Writes emit domain events through a transactional outbox relayed to RabbitMQ.  

## Technologies

* Go 1.26
* PostgreSQL 18.x
* Redis 8.8
* RabbitMQ 4.3 (AMQP 1.0)

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
curl -s -X POST http://localhost:8080/v1/products \
  -H 'Content-Type: application/json' \
  -d '{"name":"widget","stock":10,"price":{"minorAmount":999,"currency":"PLN"}}'

# get a product by id
curl -s http://localhost:8080/v1/products/{id}

# list products (keyset pagination)
curl -s 'http://localhost:8080/v1/products?limit=10'
# next page: pass the nextCursor from the previous response
curl -s 'http://localhost:8080/v1/products?limit=10&cursor={nextCursor}'

# create an order, decrementing stock atomically (requires a bearer token)
TOKEN=$(make -s token)
curl -s -X POST http://localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"productId":"{id}","quantity":2}'

# list your own orders (keyset pagination, newest first)
curl -s -H "Authorization: Bearer $TOKEN" 'http://localhost:8080/v1/orders?limit=10'

# get one of your orders by id
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/v1/orders/{id}
```

The list endpoints return `{"items":[...],"nextCursor":"<id>"}`.  
A missing `nextCursor` means the last page.  

Stock is set at creation and changed only through purchase.  
`PUT` does not accept a `stock` field.  
`POST /v1/orders` answers `201 Created` with a `Location` header and returns the order with its
`remainingStock`: `{"id":"<id>","productId":"<id>","quantity":2,"unitPrice":{"minorAmount":999,"currency":"PLN"},"remainingStock":8,"createdAt":"<ts>"}`.  
A `409 Conflict` signals insufficient stock.  
See `api.rest` for the full set of example requests.

### Auth & orders

The `/v1/orders*` endpoints require a bearer token, catalog reads stay public.  
Tokens are HS256 JWTs verified against `AUTH_JWT_SECRET`,
`make token` prints a dev token.  
Verification uses [golang-jwt](https://github.com/golang-jwt/jwt) pinned to HS256 through its `alg`
allowlist, with `exp` required, and the test suite keeps the adversarial cases (`alg: none`,
RS256 key confusion) as configuration regression guards.  
There is intentionally no signup, refresh, or IdP integration, the showcase is object-level
authorization, not identity management.  
Both transports deny by default: a route or RPC serves anonymous callers only when it is
explicitly registered as public, gRPC infrastructure (health, reflection) is excepted, and
application streams are rejected outright until one consciously opts in.  

Every purchase records an order owned by the token's user, with the unit price snapshotted at
purchase time.  
Listings return only your own rows because ownership is enforced in the SQL query, not filtered
in the handler, and a foreign order id answers `404`, so its existence stays hidden.  
`DELETE /v1/products/{id}` answers `409 Conflict` once a product has orders, backed by a
foreign key with `ON DELETE RESTRICT`.

## gRPC API

A gRPC transport mirrors the HTTP surface over the same service, on `GRPC_PORT` (default `9090`),
with a registered gRPC health service.  
The contract lives in `api/proto` as two packages, `catalog.v1` and `order.v1` (Protobuf edition 2024),
mirroring the aggregate split of the HTTP paths.  
Code is generated with [buf](https://buf.build) into `api/gen` (`make proto`).  
The same bearer token guards the same operations: `order.v1.OrderService`
requires an `authorization` metadata entry, catalog reads stay public.

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

# create an order (FAILED_PRECONDITION on insufficient stock, requires a bearer token)
grpcurl -plaintext -H "authorization: Bearer $(make -s token)" \
  -d '{"productId":"<id>","quantity":2}' \
  localhost:9090 order.v1.OrderService/CreateOrder

# list your own orders (newest first)
grpcurl -plaintext -H "authorization: Bearer $(make -s token)" \
  -d '{"limit":10}' localhost:9090 order.v1.OrderService/ListOrders

# get one of your orders by id
grpcurl -plaintext -H "authorization: Bearer $(make -s token)" \
  -d '{"id":"<id>"}' localhost:9090 order.v1.OrderService/GetOrder
```

## Architecture

`cmd/` → `transport/http` and `transport/grpc` → `service` → `store`, with `domain` and `cache` as cross-cutting packages.

### Outbox & events

Every write (create, update, delete, purchase) inserts a domain event into the `outbox` table
in the same transaction as the data change, so there is no dual-write.  
A relay claims due rows with `FOR UPDATE SKIP LOCKED` (safe with multiple instances, no leader election),
publishes them to the `storefront.product` topic exchange over AMQP 1.0, and deletes the published rows in the
same transaction.  
New events wake the relay via `LISTEN/NOTIFY`, interval polling is only a fallback.

Delivery is at-least-once end-to-end: consumers deduplicate by `eventId` (UUIDv7) and can discard
stale updates by `version`, since retries and multiple relays give no ordering guarantee.  
Transient broker failures never consume retry attempts, events wait in Postgres for as long as an outage
lasts.  
Only permanent rejections (poison messages) count toward `OUTBOX_MAX_ATTEMPTS` and end up in `outbox_dead` together with their last error.

## Migrations

Schema changes live in `migrate/migrations/` and are bundled into the binary via `embed.FS`.  
The `cmd/migrate` CLI applies them using [goose](https://github.com/pressly/goose).

```bash
make migrate-up       # apply all pending migrations
make migrate-status   # show applied and pending migrations
make migrate-down     # roll back the last migration (local dev only)
```

The application performs a fail-fast check at startup and refuses to run if the database schema is older than the embedded migrations.
