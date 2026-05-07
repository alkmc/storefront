# restClean

## Table of contents

* [General info](#general-info)
* [Technologies](#technologies)
* [Usage](#usage)
* [Setup](#setup)
* [Migrations](#migrations)

## General Info

This is small simple REST API Web Application which thanks to Dependency Inversion principle shall be:

* easy testable
* independent of frameworks, UI and Databases.

## Technologies

This project is build with Go 1.26.

Other dependencies:

* PostgreSQL 18.x for datastore and testing
* Redis 8.x for cache

## Usage

There are examples of API calls in api.rest file, which can be called directly  
in VS code with Rest Plus plugin, Jetbrain's GoLand or used with CURL / Postman.

## Setup

In order to use PostgreSQL as database, make sure the following environment variables are set:

* PG_HOST
* PG_PORT
* PG_USER
* PG_PASSWORD
* PG_DB

## Migrations

Schema changes live in `internal/migrate/migrations/` and are bundled into the
binary via `embed.FS`. The `cmd/migrate` CLI applies them using
[goose](https://github.com/pressly/goose).

```bash
make migrate-up       # apply all pending migrations
make migrate-status   # show applied and pending migrations
make migrate-down     # roll back the last migration (local dev only)
```

Run `make migrate-up` before deploying a new version of the application.
The application performs a fail-fast check at startup and refuses to run if
the database schema is older than the embedded migrations.

Production is forward-only — `make migrate-down` exists for local development
and is not part of the deployment pipeline.
