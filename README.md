# restClean

## Table of contents

* [General info](#general-info)
* [Technologies](#technologies)
* [Usage](#usage)
* [Setup](#setup)

## General Info

This is small simple rest API Web Application which thanks to Dependency Inversion principle shall be:

-easy testable

Independent of:

-frameworks

-UI

-Databases.

## Technologies

This project is build with Go 1.18.

Other dependencies:

PostgreSQL / SQLite3 for datastore and testing

Redis for cache

## Usage

There are examples of API calls in api.rest file, which can be called Directly in VS code with Rest Plus plugin, Jetbrain's GoLand or used with CURL / Postman.

## Setup

In order to use PostgreSQL as database, make sure the following environment variables are set:

"PG_HOST",

"PG_PORT",

"PG_USER",

"PG_PASSWORD",

"PG_DB",
