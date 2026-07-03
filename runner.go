package go_migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// DefaultMigrationsTable tracks the currently-applied revision, Alembic-style:
// it holds at most one row (version_num) pointing at the current head.
const DefaultMigrationsTable = "go_migrations_version"

// MigrateOptions configure database migration execution.
type MigrateOptions struct {
	DatabaseURL     string
	MigrationsDir   string
	MigrationsTable string

	// Logf, when set, receives human-readable progress lines.
	Logf func(format string, args ...any)
}

func (o MigrateOptions) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// Upgrade runs revisions forward until target is reached.
// target may be "head", a relative step ("+2"), or a revision id (or prefix).
func Upgrade(ctx context.Context, opts MigrateOptions, target string) error {
	return run(ctx, opts, func(ctx context.Context, db *sql.DB, table string, ordered []Revision, currentIdx int) error {
		targetIdx, err := resolveUpgradeTarget(ordered, currentIdx, target)
		if err != nil {
			return err
		}
		if targetIdx <= currentIdx {
			opts.logf("Already at or ahead of target %q; nothing to upgrade.", target)
			return nil
		}
		for i := currentIdx + 1; i <= targetIdx; i++ {
			rev := ordered[i]
			if err := runRevision(ctx, db, table, rev.UpSQL, rev.ID); err != nil {
				return fmt.Errorf("upgrade %s: %w", rev.ID, err)
			}
			opts.logf("Running upgrade %s -> %s, %s", displayDown(rev.DownRevision), rev.ID, rev.Message)
		}
		return nil
	})
}

// Downgrade rolls revisions back until target is reached.
// target may be "base", a relative step ("-1"), or a revision id (or prefix).
func Downgrade(ctx context.Context, opts MigrateOptions, target string) error {
	return run(ctx, opts, func(ctx context.Context, db *sql.DB, table string, ordered []Revision, currentIdx int) error {
		targetIdx, err := resolveDowngradeTarget(ordered, currentIdx, target)
		if err != nil {
			return err
		}
		if targetIdx >= currentIdx {
			opts.logf("Already at or below target %q; nothing to downgrade.", target)
			return nil
		}
		for i := currentIdx; i > targetIdx; i-- {
			rev := ordered[i]
			if err := runRevision(ctx, db, table, rev.DownSQL, rev.DownRevision); err != nil {
				return fmt.Errorf("downgrade %s: %w", rev.ID, err)
			}
			opts.logf("Running downgrade %s -> %s, %s", rev.ID, displayDown(rev.DownRevision), rev.Message)
		}
		return nil
	})
}

// Stamp sets the recorded revision to target without running any SQL.
func Stamp(ctx context.Context, opts MigrateOptions, target string) error {
	return run(ctx, opts, func(ctx context.Context, db *sql.DB, table string, ordered []Revision, currentIdx int) error {
		var revID string
		switch target {
		case "base":
			revID = ""
		case "head":
			if len(ordered) == 0 {
				return fmt.Errorf("no revisions to stamp")
			}
			revID = ordered[len(ordered)-1].ID
		default:
			idx, err := indexByID(ordered, target)
			if err != nil {
				return err
			}
			revID = ordered[idx].ID
		}
		if err := setCurrentRevision(ctx, db, table, revID); err != nil {
			return err
		}
		opts.logf("Stamped revision to %s", displayDown(revID))
		return nil
	})
}

// CurrentRevision returns the currently-applied revision id ("" when none).
func CurrentRevision(ctx context.Context, opts MigrateOptions) (string, error) {
	table, err := normalizeMigrationsTable(opts.MigrationsTable)
	if err != nil {
		return "", err
	}
	db, err := openDatabase(ctx, opts.DatabaseURL)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if err := ensureVersionTable(ctx, db, table); err != nil {
		return "", err
	}
	return currentRevision(ctx, db, table)
}

// run wires up shared setup (db, table, ordered graph, current position) for the
// mutating commands.
func run(ctx context.Context, opts MigrateOptions, fn func(context.Context, *sql.DB, string, []Revision, int) error) error {
	if opts.MigrationsDir == "" {
		opts.MigrationsDir = "migrations"
	}
	table, err := normalizeMigrationsTable(opts.MigrationsTable)
	if err != nil {
		return err
	}
	ordered, err := OrderedRevisions(opts.MigrationsDir)
	if err != nil {
		return err
	}

	db, err := openDatabase(ctx, opts.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureVersionTable(ctx, db, table); err != nil {
		return err
	}
	current, err := currentRevision(ctx, db, table)
	if err != nil {
		return err
	}
	currentIdx := -1
	if current != "" {
		idx, err := indexByID(ordered, current)
		if err != nil {
			return fmt.Errorf("recorded revision %q not found on disk: %w", current, err)
		}
		currentIdx = idx
	}
	return fn(ctx, db, table, ordered, currentIdx)
}

func openDatabase(ctx context.Context, databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
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

func ensureVersionTable(ctx context.Context, db *sql.DB, table string) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (version_num VARCHAR(32) NOT NULL)`, quoteIdent(table))
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create %s table: %w", table, err)
	}
	return nil
}

func currentRevision(ctx context.Context, db *sql.DB, table string) (string, error) {
	query := fmt.Sprintf(`SELECT version_num FROM %s LIMIT 1`, quoteIdent(table))
	var version string
	switch err := db.QueryRowContext(ctx, query).Scan(&version); err {
	case nil:
		return version, nil
	case sql.ErrNoRows:
		return "", nil
	default:
		return "", fmt.Errorf("read current revision: %w", err)
	}
}

// setCurrentRevision replaces the tracked revision. revID "" clears it (base).
func setCurrentRevision(ctx context.Context, db *sql.DB, table, revID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := writeRevisionPointer(ctx, tx, table, revID); err != nil {
		return err
	}
	return tx.Commit()
}

func writeRevisionPointer(ctx context.Context, tx *sql.Tx, table, revID string) error {
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s`, quoteIdent(table))); err != nil {
		return fmt.Errorf("clear revision pointer: %w", err)
	}
	if revID == "" {
		return nil
	}
	insert := fmt.Sprintf(`INSERT INTO %s (version_num) VALUES ($1)`, quoteIdent(table))
	if _, err := tx.ExecContext(ctx, insert, revID); err != nil {
		return fmt.Errorf("record revision %s: %w", revID, err)
	}
	return nil
}

// runRevision executes migrationSQL and moves the recorded pointer to newRev in
// a single transaction. Empty SQL (e.g. hand-written stubs) is skipped.
func runRevision(ctx context.Context, db *sql.DB, table, migrationSQL, newRev string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if strings.TrimSpace(migrationSQL) != "" {
		if _, err := tx.ExecContext(ctx, migrationSQL); err != nil {
			return err
		}
	}
	if err := writeRevisionPointer(ctx, tx, table, newRev); err != nil {
		return err
	}
	return tx.Commit()
}

func resolveUpgradeTarget(ordered []Revision, currentIdx int, target string) (int, error) {
	target = strings.TrimSpace(target)
	switch {
	case target == "" || target == "head":
		return len(ordered) - 1, nil
	case strings.HasPrefix(target, "+"):
		n, err := strconv.Atoi(target[1:])
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid relative target %q", target)
		}
		idx := currentIdx + n
		if idx >= len(ordered) {
			return 0, fmt.Errorf("relative target %q goes past head", target)
		}
		return idx, nil
	default:
		return indexByID(ordered, target)
	}
}

func resolveDowngradeTarget(ordered []Revision, currentIdx int, target string) (int, error) {
	target = strings.TrimSpace(target)
	switch {
	case target == "base":
		return -1, nil
	case strings.HasPrefix(target, "-"):
		n, err := strconv.Atoi(target[1:])
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid relative target %q", target)
		}
		idx := currentIdx - n
		if idx < -1 {
			return 0, fmt.Errorf("relative target %q goes past base", target)
		}
		return idx, nil
	case target == "" || target == "head":
		return 0, fmt.Errorf("downgrade requires a target (base, -N, or a revision)")
	default:
		return indexByID(ordered, target)
	}
}

// indexByID resolves a full revision id or an unambiguous prefix to its index.
func indexByID(ordered []Revision, id string) (int, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, fmt.Errorf("empty revision id")
	}
	exact := -1
	prefix := -1
	prefixCount := 0
	for i, rev := range ordered {
		if rev.ID == id {
			exact = i
		}
		if strings.HasPrefix(rev.ID, id) {
			prefix = i
			prefixCount++
		}
	}
	if exact >= 0 {
		return exact, nil
	}
	switch prefixCount {
	case 1:
		return prefix, nil
	case 0:
		return 0, fmt.Errorf("revision %q not found", id)
	default:
		return 0, fmt.Errorf("revision %q is ambiguous", id)
	}
}

func displayDown(revID string) string {
	if revID == "" {
		return "<base>"
	}
	return revID
}
