package go_migrations

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Options configure revision generation.
type Options struct {
	Models          []any
	ModelsRoot      string
	MigrationsDir   string
	DatabaseURL     string
	MigrationsTable string
	Message         string // -m, human-readable revision message
	Empty           bool   // create an empty (hand-editable) revision, no model diff
	DryRun          bool

	// CurrentSchema overrides database introspection (tests only).
	CurrentSchema *DatabaseSchema
}

// Result describes a generated revision.
type Result struct {
	Revision     string
	DownRevision string
	Message      string
	Path         string
	UpSQL        string
	DownSQL      string
}

// Generate creates a new revision that descends from the current head. With
// Empty set it writes a blank up/down stub; otherwise it diffs models against
// the current schema (Alembic --autogenerate).
func Generate(ctx context.Context, opts Options) (Result, error) {
	if opts.MigrationsDir == "" {
		opts.MigrationsDir = "migrations"
	}

	downRevision, err := HeadRevision(opts.MigrationsDir)
	if err != nil {
		return Result{}, err
	}

	var upSQL, downSQL, message string
	if opts.Empty {
		message = opts.Message
		if message == "" {
			message = "revision"
		}
	} else {
		upSQL, downSQL, message, err = autogenerate(ctx, opts, downRevision)
		if err != nil {
			return Result{}, err
		}
	}

	revID, err := newRevisionID()
	if err != nil {
		return Result{}, err
	}
	rev := Revision{
		ID:           revID,
		DownRevision: downRevision,
		Message:      message,
		CreateDate:   time.Now().UTC(),
		UpSQL:        upSQL,
		DownSQL:      downSQL,
	}

	result := Result{
		Revision:     rev.ID,
		DownRevision: rev.DownRevision,
		Message:      rev.Message,
		UpSQL:        rev.UpSQL,
		DownSQL:      rev.DownSQL,
	}
	if opts.DryRun {
		return result, nil
	}

	path, err := WriteRevision(opts.MigrationsDir, rev)
	if err != nil {
		return Result{}, err
	}
	result.Path = path
	return result, nil
}

func autogenerate(ctx context.Context, opts Options, downRevision string) (upSQL, downSQL, message string, err error) {
	var desired DatabaseSchema
	if len(opts.Models) > 0 {
		desired, err = SchemaFromModels(opts.Models)
	} else {
		desired, err = DiscoverSchema(opts.ModelsRoot)
	}
	if err != nil {
		return "", "", "", err
	}

	current, err := loadCurrentSchema(ctx, opts)
	if err != nil {
		return "", "", "", err
	}
	if desired.Equal(current) {
		return "", "", "", ErrNoChanges
	}

	changes := Diff(current, desired)
	if changes.IsEmpty() {
		return "", "", "", ErrNoChanges
	}

	upSQL = GenerateUp(changes)
	downSQL = GenerateDown(changes)
	message = opts.Message
	if message == "" {
		message = MigrationName(changes, current)
	}

	// First revision needs pgcrypto for gen_random_uuid() defaults.
	if downRevision == "" && strings.Contains(upSQL, "gen_random_uuid") {
		upSQL = "CREATE EXTENSION IF NOT EXISTS pgcrypto;\n\n" + upSQL
	}
	return upSQL, downSQL, message, nil
}

func loadCurrentSchema(ctx context.Context, opts Options) (DatabaseSchema, error) {
	if opts.CurrentSchema != nil {
		return *opts.CurrentSchema, nil
	}

	revisions, err := LoadRevisions(opts.MigrationsDir)
	if err != nil {
		return DatabaseSchema{}, err
	}
	// Fresh app: no revisions on disk → empty baseline.
	if len(revisions) == 0 {
		return DatabaseSchema{}, nil
	}

	if opts.DatabaseURL == "" {
		return DatabaseSchema{}, fmt.Errorf("DATABASE_URL is required when revisions already exist")
	}
	table := opts.MigrationsTable
	if table == "" {
		table = DefaultMigrationsTable
	}
	return SchemaFromDatabaseURL(ctx, opts.DatabaseURL, introspectExcludedTables(table)...)
}
