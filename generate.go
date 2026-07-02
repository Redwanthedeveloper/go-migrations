package go_migrations

import (
	"context"
	"fmt"
	"strings"
)

// Options configure migration generation.
type Options struct {
	Models          []any
	ModelsRoot      string
	MigrationsDir   string
	DatabaseURL     string
	MigrationsTable string
	DryRun          bool

	// CurrentSchema overrides database introspection (tests only).
	CurrentSchema *DatabaseSchema
}

// Result describes generated migration output.
type Result struct {
	BaseName string
	UpSQL    string
	DownSQL  string
}

// Generate compares models to the current schema and produces migration SQL.
func Generate(ctx context.Context, opts Options) (Result, error) {
	if opts.MigrationsDir == "" {
		opts.MigrationsDir = "migrations"
	}

	var desired DatabaseSchema
	var err error
	if len(opts.Models) > 0 {
		desired, err = SchemaFromModels(opts.Models)
	} else {
		desired, err = DiscoverSchema(opts.ModelsRoot)
	}
	if err != nil {
		return Result{}, err
	}

	current, err := loadCurrentSchema(ctx, opts)
	if err != nil {
		return Result{}, err
	}

	if desired.Equal(current) {
		return Result{}, ErrNoChanges
	}

	changes := Diff(current, desired)
	if changes.IsEmpty() {
		return Result{}, ErrNoChanges
	}

	upSQL := GenerateUp(changes)
	downSQL := GenerateDown(changes)
	name := MigrationName(changes, current)

	version, err := NextVersion(opts.MigrationsDir)
	if err != nil {
		return Result{}, err
	}
	base := FormatMigrationBase(version, name)

	if opts.DryRun {
		return Result{BaseName: base, UpSQL: upSQL, DownSQL: downSQL}, nil
	}

	if version == 1 && strings.Contains(upSQL, "gen_random_uuid") {
		upSQL = "CREATE EXTENSION IF NOT EXISTS pgcrypto;\n\n" + upSQL
	}

	written, err := WriteMigration(opts.MigrationsDir, version, name, upSQL, downSQL)
	if err != nil {
		return Result{}, err
	}
	base = written

	return Result{
		BaseName: base,
		UpSQL:    upSQL,
		DownSQL:  downSQL,
	}, nil
}

func loadCurrentSchema(ctx context.Context, opts Options) (DatabaseSchema, error) {
	if opts.CurrentSchema != nil {
		return *opts.CurrentSchema, nil
	}

	migrations, err := ListMigrations(opts.MigrationsDir)
	if err != nil {
		return DatabaseSchema{}, err
	}
	// Fresh app: no migration files on disk → empty baseline (like Django with no migrations).
	if len(migrations) == 0 {
		return DatabaseSchema{}, nil
	}

	if opts.DatabaseURL == "" {
		return DatabaseSchema{}, fmt.Errorf("DATABASE_URL is required when migration files already exist")
	}
	table := opts.MigrationsTable
	if table == "" {
		table = DefaultMigrationsTable
	}
	return SchemaFromDatabaseURL(ctx, opts.DatabaseURL, introspectExcludedTables(table)...)
}
