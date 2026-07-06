# go_migrations

Alembic-style database migrations for Go projects using [GORM](https://gorm.io) models and PostgreSQL.

`go_migrations` compares your GORM struct definitions to the current database schema, generates versioned revision files linked by `down_revision` (exactly like [Alembic](https://alembic.sqlalchemy.org)), and applies them with `upgrade` / `downgrade`.

## Features

- Auto-discover GORM models from `internal/*/model/` packages
- Alembic-style revisions: each file has a hash `revision` id and a `down_revision` pointer forming a history chain
- Single `.sql` file per revision with `-- migrate:up` / `-- migrate:down` sections
- Current revision tracked in a one-row PostgreSQL table (`version_num`), like Alembic's `alembic_version`
- Relative targets: `upgrade +2`, `downgrade -1`, plus `head` / `base` / revision ids (or prefixes)
- Fresh-app support: the first `revision --autogenerate` works with an empty `migrations/` folder (no DB required)
- Live database introspection when revisions already exist

## Install

```bash
go install github.com/Redwanthedeveloper/go-migrations/cmd/go_migrations@latest
```

Or use it as a library:

```bash
go get github.com/Redwanthedeveloper/go-migrations@latest
```

## Quick start

1. Define GORM models under `internal/{domain}/model/`.
2. Generate a revision from your models:

```bash
go_migrations revision --autogenerate -m "create users"
```

3. Apply it:

```bash
export DATABASE_URL=postgres://user:pass@localhost:5432/mydb?sslmode=disable
go_migrations upgrade head
```

## CLI reference

Commands mirror Alembic; only the program name differs.

| Alembic | go_migrations | Description |
| ------- | ------------- | ----------- |
| `alembic revision --autogenerate -m "msg"` | `go_migrations revision --autogenerate -m "msg"` | Diff GORM models → revision file |
| `alembic revision -m "msg"` | `go_migrations revision -m "msg"` | Empty, hand-edited revision |
| `alembic upgrade head` | `go_migrations upgrade head` | Apply all pending revisions |
| `alembic upgrade +2` | `go_migrations upgrade +2` | Apply the next N revisions |
| `alembic upgrade <rev>` | `go_migrations upgrade <rev>` | Upgrade up to a revision |
| `alembic downgrade base` | `go_migrations downgrade base` | Roll back every revision |
| `alembic downgrade -1` | `go_migrations downgrade -1` | Roll back N revisions |
| `alembic downgrade <rev>` | `go_migrations downgrade <rev>` | Downgrade back to a revision |
| `alembic current` | `go_migrations current` | Show the current revision |
| `alembic history` | `go_migrations history` | List revisions (head → base) |
| `alembic heads` | `go_migrations heads` | Show head revision(s) |
| `alembic stamp head` | `go_migrations stamp head` | Set the recorded revision without running SQL |

### Global flags

Available on every command:

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `-dir` | `migrations` | Revisions directory |
| `-database-url` | `$DATABASE_URL` | PostgreSQL URL (required for DB commands) |
| `-migrations-table` | `go_migrations_version` | One-row revision tracking table |

`revision` adds: `-m` (message), `--autogenerate` (diff models), `-models` (module root, default `.`), `-dry-run`.

## Revision file format

Each revision is a single SQL file named `<seq>_<revision>_<slug>.sql` (for example `000001_9106914b8a65_create_users.sql`). The six-digit sequence prefix keeps files in apply order when listed by name; the revision id in the header is still the canonical identifier.

```sql
-- revision: 9106914b8a65
-- down_revision: 
-- create_date: 2026-07-03T12:00:00Z
-- message: create users

-- migrate:up
CREATE TABLE users (...);

-- migrate:down
DROP TABLE users;
```

Ordering comes from the `down_revision` chain, not the filename — exactly like Alembic. The base revision has an empty `down_revision`.

## Library usage

```go
import go_migrations "github.com/Redwanthedeveloper/go-migrations"

// Generate a revision from GORM models (autogenerate).
result, err := go_migrations.Generate(ctx, go_migrations.Options{
    ModelsRoot:    ".",
    MigrationsDir: "migrations",
    DatabaseURL:   os.Getenv("DATABASE_URL"),
    Message:       "create users",
})

// Apply / roll back.
opts := go_migrations.MigrateOptions{
    DatabaseURL:   os.Getenv("DATABASE_URL"),
    MigrationsDir: "migrations",
}
err = go_migrations.Upgrade(ctx, opts, "head")
err = go_migrations.Downgrade(ctx, opts, "-1")
err = go_migrations.Stamp(ctx, opts, "head")
current, err := go_migrations.CurrentRevision(ctx, opts)
```

## Auto-naming

When `-m` is omitted, `--autogenerate` derives a slug from the detected changes:

| Change | Example message |
| ------ | --------------- |
| First revision | `initial` |
| New model | `contact` |
| Add column | `tenant_email` |
| Multiple changes | `plan_code_and_more` |

## Requirements

- Go 1.22+
- PostgreSQL
- GORM-tagged structs

## Publishing

Published at [github.com/Redwanthedeveloper/go-migrations](https://github.com/Redwanthedeveloper/go-migrations).

To release a new version:

```bash
git tag v0.2.0
git push origin v0.2.0
```

## License

MIT — see [LICENSE](LICENSE).
