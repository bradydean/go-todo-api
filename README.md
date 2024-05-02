# go-todo-api

A simple todo API written in Go showcasing the use of JWT authentication and PostgreSQL.

## tools used

- [echo](https://github.com/labstack/echo): web framework
- [go-jwt-middleware](https://github.com/auth0/go-jwt-middleware): JWT middleware by Auth0
- [jet](https://github.com/go-jet/jet): Type-safe SQL builder and code generator
- [pgx](https://github.com/jackc/pgx): PostgreSQL driver and toolkit
- [migrate](https://github.com/golang-migrate/migrate): Database migration tool

## env vars

A `.env` file also works.

```bash
$ export DATABASE_URL="postgres://user:password@localhost:5432/dbname?sslmode=disable"
$ export AUTH0_DOMAIN="example.auth0.com"
$ export AUTH0_AUDIENCE="https://example.auth0.com/api/v2/
```

## migrate database

```bash
$ go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate -path ./migrations -database ${DATABASE_URL} up
```

## generate jet files

```bash
$ go run ./internal/scripts/jet/main.go
```

## build and run

```bash
$ go build -o todo-api main.go
$ ./todo-api
```
