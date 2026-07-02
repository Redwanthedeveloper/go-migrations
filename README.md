# go_migrations

Django-style database migrations for Go projects using [GORM](https://gorm.io) models and PostgreSQL.

`go_migrations` compares your GORM struct definitions to the current database schema, generates versioned `.up.sql` / `.down.sql` files, and applies them with a simple migration runner.

## Features

- Auto-discover GORM models from `internal/*/model/` packages
- Django-style auto-generated migration names (`000001_initial`, `000002_tenant_email`, …)
- SQL up/down migration files on disk
- Applied migrations tracked in a PostgreSQL table (default: `go_migrations`)
- Fresh-app support: first migration works with an empty `migrations/` folder (no DB required)
- Live database introspection when migration files already exist

## Install

```bash
go get github.com/Redwanthedeveloper/go-migrations@v0.1.0
```

Or use the CLI tools:

```bash
go install github.com/Redwanthedeveloper/go-migrations/cmd/makemigrations@latest
go install github.com/Redwanthedeveloper/go-migrations/cmd/migrate@latest
```

## Quick start

1. Define GORM models under `internal/{domain}/model/`.
2. Generate migrations:

```bash
makemigrations -models . -dir migrations
```

3. Apply migrations:

```bash
export DATABASE_URL=postgres://user:pass@localhost:5432/mydb?sslmode=disable
migrate -dir migrations -database-url "$DATABASE_URL"
```

## CLI reference

### makemigrations

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-models` | `.` | Module root for model discovery |
| `-dir` | `migrations` | Output directory |
| `-database-url` | `$DATABASE_URL` | PostgreSQL URL (required when migrations exist) |
| `-migrations-table` | `go_migrations` | Table used to track applied migrations |
| `-dry-run` | `false` | Print SQL without writing files |

### migrate

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-direction` | `up` | `up` or `down` |
| `-steps` | `0` | Steps to run (0 = all pending for up, 1 for down) |
| `-dir` | `migrations` | Migrations directory |
| `-database-url` | `$DATABASE_URL` | PostgreSQL URL (required) |
| `-migrations-table` | `go_migrations` | Applied migrations table |

## Library usage

```go
import "github.com/Redwanthedeveloper/go-migrations"

result, err := go_migrations.Generate(ctx, go_migrations.Options{
    ModelsRoot:    ".",
    MigrationsDir: "migrations",
    DatabaseURL:   os.Getenv("DATABASE_URL"),
})

err = go_migrations.Apply(ctx, go_migrations.MigrateOptions{
    DatabaseURL:   os.Getenv("DATABASE_URL"),
    MigrationsDir: "migrations",
    Direction:     "up",
})
```

## Auto-naming

| Change | Example |
| ------ | ------- |
| First migration | `000001_initial` |
| New model | `000002_contact` |
| Add column | `000003_tenant_email` |
| Multiple changes | `000004_plan_code_and_more` |

## Publishing

Published at [github.com/Redwanthedeveloper/go-migrations](https://github.com/Redwanthedeveloper/go-migrations).

**Module path:** `github.com/Redwanthedeveloper/go-migrations`

To release a new version:

```bash
git tag v0.1.1
git push origin v0.1.1
```

Go module proxy will index the tag automatically.

## Requirements

- Go 1.22+
- PostgreSQL
- GORM-tagged structs

## License

MIT — see [LICENSE](LICENSE).
