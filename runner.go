package go_migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// DefaultMigrationsTable is the applied-migrations tracking table when none is set.
const DefaultMigrationsTable = "go_migrations"

// MigrateOptions configure database migration execution.
type MigrateOptions struct {
	DatabaseURL     string
	MigrationsDir   string
	MigrationsTable string
	Direction       string // up or down
	Steps           int    // 0 = all pending (up) or one step (down default in CLI)
}

// Apply runs pending migrations and records them in the migrations table (Django-style).
func Apply(ctx context.Context, opts MigrateOptions) error {
	if opts.DatabaseURL == "" {
		return fmt.Errorf("database URL is required")
	}
	if opts.MigrationsDir == "" {
		opts.MigrationsDir = "migrations"
	}
	table, err := normalizeMigrationsTable(opts.MigrationsTable)
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", opts.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if err := ensureMigrationsTable(ctx, db, table); err != nil {
		return err
	}

	all, err := ListMigrations(opts.MigrationsDir)
	if err != nil {
		return err
	}
	applied, err := appliedMigrations(ctx, db, table)
	if err != nil {
		return err
	}

	switch opts.Direction {
	case "up", "":
		return applyUp(ctx, db, table, all, applied, opts.Steps)
	case "down":
		steps := opts.Steps
		if steps == 0 {
			steps = 1
		}
		return applyDown(ctx, db, table, all, applied, steps)
	default:
		return fmt.Errorf("unknown direction %q", opts.Direction)
	}
}

func normalizeMigrationsTable(name string) (string, error) {
	if name == "" {
		name = DefaultMigrationsTable
	}
	if err := validateSQLIdentifier(name); err != nil {
		return "", fmt.Errorf("migrations table: %w", err)
	}
	return name, nil
}

func ensureMigrationsTable(ctx context.Context, db *sql.DB, table string) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id         BIGSERIAL PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`, quoteIdent(table))
	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("create %s table: %w", table, err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	query := fmt.Sprintf(`SELECT name FROM %s ORDER BY id`, quoteIdent(table))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func applyUp(ctx context.Context, db *sql.DB, table string, all []Migration, applied []string, steps int) error {
	appliedSet := make(map[string]struct{}, len(applied))
	for _, name := range applied {
		appliedSet[name] = struct{}{}
	}

	var pending []Migration
	for _, migration := range all {
		if _, ok := appliedSet[migration.BaseName()]; !ok {
			pending = append(pending, migration)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if steps > 0 && steps < len(pending) {
		pending = pending[:steps]
	}

	for _, migration := range pending {
		if err := runMigrationUp(ctx, db, table, migration); err != nil {
			return err
		}
	}
	return nil
}

func applyDown(ctx context.Context, db *sql.DB, table string, all []Migration, applied []string, steps int) error {
	if len(applied) == 0 {
		return nil
	}

	byName := make(map[string]Migration, len(all))
	for _, migration := range all {
		byName[migration.BaseName()] = migration
	}

	for range steps {
		if len(applied) == 0 {
			return nil
		}
		name := applied[len(applied)-1]
		migration, ok := byName[name]
		if !ok {
			return fmt.Errorf("applied migration %q not found on disk", name)
		}
		if err := runMigrationDown(ctx, db, table, migration); err != nil {
			return err
		}
		applied = applied[:len(applied)-1]
	}
	return nil
}

func runMigrationUp(ctx context.Context, db *sql.DB, table string, migration Migration) error {
	sqlBytes, err := os.ReadFile(migration.upPath())
	if err != nil {
		return fmt.Errorf("read %s: %w", migration.upPath(), err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("apply %s: %w", migration.BaseName(), err)
	}
	insert := fmt.Sprintf(`INSERT INTO %s (name) VALUES ($1)`, quoteIdent(table))
	if _, err := tx.ExecContext(ctx, insert, migration.BaseName()); err != nil {
		return fmt.Errorf("record %s: %w", migration.BaseName(), err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", migration.BaseName(), err)
	}
	return nil
}

func runMigrationDown(ctx context.Context, db *sql.DB, table string, migration Migration) error {
	sqlBytes, err := os.ReadFile(migration.downPath())
	if err != nil {
		return fmt.Errorf("read %s: %w", migration.downPath(), err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("rollback %s: %w", migration.BaseName(), err)
	}
	deleteSQL := fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, quoteIdent(table))
	result, err := tx.ExecContext(ctx, deleteSQL, migration.BaseName())
	if err != nil {
		return fmt.Errorf("unrecord %s: %w", migration.BaseName(), err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected %s: %w", migration.BaseName(), err)
	}
	if rows == 0 {
		return fmt.Errorf("migration %q was not recorded as applied", migration.BaseName())
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", migration.BaseName(), err)
	}
	return nil
}

// ErrNoMigrations is returned when there is nothing to apply.
var ErrNoMigrations = errors.New("no migrations to apply")
