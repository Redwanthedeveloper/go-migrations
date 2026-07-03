// Command go_migrations is an Alembic-style migration CLI for Go projects.
//
// Usage:
//
//	go_migrations revision --autogenerate -m "message"   # generate from GORM models
//	go_migrations revision -m "message"                  # empty, hand-edited revision
//	go_migrations upgrade head                            # apply all pending
//	go_migrations upgrade +1                              # apply next revision
//	go_migrations downgrade -1                            # roll back one revision
//	go_migrations downgrade base                          # roll back everything
//	go_migrations current                                 # show current revision
//	go_migrations history                                 # list revisions
//	go_migrations heads                                   # show head revision(s)
//	go_migrations stamp head                              # set revision without running SQL
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	go_migrations "github.com/Redwanthedeveloper/go-migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "revision":
		err = cmdRevision(ctx, args)
	case "upgrade":
		err = cmdUpgrade(ctx, args)
	case "downgrade":
		err = cmdDowngrade(ctx, args)
	case "stamp":
		err = cmdStamp(ctx, args)
	case "current":
		err = cmdCurrent(ctx, args)
	case "history":
		err = cmdHistory(args)
	case "heads":
		err = cmdHeads(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "go_migrations %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

type globalFlags struct {
	dir         *string
	databaseURL *string
	table       *string
}

func addGlobalFlags(fs *flag.FlagSet) globalFlags {
	return globalFlags{
		dir:         fs.String("dir", "migrations", "migrations directory"),
		databaseURL: fs.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL URL (or set DATABASE_URL)"),
		table:       fs.String("migrations-table", go_migrations.DefaultMigrationsTable, "revision tracking table"),
	}
}

func (g globalFlags) migrateOptions() go_migrations.MigrateOptions {
	return go_migrations.MigrateOptions{
		DatabaseURL:     *g.databaseURL,
		MigrationsDir:   *g.dir,
		MigrationsTable: *g.table,
		Logf:            func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	}
}

func cmdRevision(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("revision", flag.ExitOnError)
	g := addGlobalFlags(fs)
	message := fs.String("m", "", "revision message")
	autogenerate := fs.Bool("autogenerate", false, "diff GORM models to populate the revision")
	models := fs.String("models", ".", "module root for model discovery")
	dryRun := fs.Bool("dry-run", false, "print the revision without writing a file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	result, err := go_migrations.Generate(ctx, go_migrations.Options{
		ModelsRoot:      *models,
		MigrationsDir:   *g.dir,
		DatabaseURL:     *g.databaseURL,
		MigrationsTable: *g.table,
		Message:         *message,
		Empty:           !*autogenerate,
		DryRun:          *dryRun,
	})
	if err != nil {
		if errors.Is(err, go_migrations.ErrNoChanges) {
			fmt.Println("No changes detected.")
			return nil
		}
		return err
	}

	if *dryRun {
		fmt.Printf("-- revision: %s (down_revision: %s) --\n", result.Revision, downLabel(result.DownRevision))
		fmt.Printf("-- message: %s\n", result.Message)
		fmt.Println("-- up --")
		fmt.Print(result.UpSQL)
		fmt.Println("\n-- down --")
		fmt.Print(result.DownSQL)
		fmt.Println()
		return nil
	}

	fmt.Printf("Generated revision %s (%s)\n", result.Revision, result.Message)
	fmt.Printf("  %s\n", result.Path)
	return nil
}

func cmdUpgrade(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	g := addGlobalFlags(fs)
	flagArgs, target := splitTarget(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if target == "" {
		target = "head"
	}
	return go_migrations.Upgrade(ctx, g.migrateOptions(), target)
}

func cmdDowngrade(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("downgrade", flag.ExitOnError)
	g := addGlobalFlags(fs)
	flagArgs, target := splitTarget(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("a target is required (base, -N, or a revision)")
	}
	return go_migrations.Downgrade(ctx, g.migrateOptions(), target)
}

func cmdStamp(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("stamp", flag.ExitOnError)
	g := addGlobalFlags(fs)
	flagArgs, target := splitTarget(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("a target is required (head, base, or a revision)")
	}
	return go_migrations.Stamp(ctx, g.migrateOptions(), target)
}

// splitTarget separates the single positional target from flag arguments so the
// target may appear before or after flags (e.g. "upgrade head -dir x").
// All global flags take a value, so a token following a value-flag is its value.
// A relative target ("+1"/"-1") is recognized even though it starts with "-".
func splitTarget(args []string) (flagArgs []string, target string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if isRelativeTarget(arg) {
			if target == "" {
				target = arg
			}
			continue
		}
		if len(arg) > 0 && arg[0] == '-' {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		if target == "" {
			target = arg
		}
	}
	return flagArgs, target
}

func isRelativeTarget(arg string) bool {
	if len(arg) < 2 || (arg[0] != '+' && arg[0] != '-') {
		return false
	}
	for _, r := range arg[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func cmdCurrent(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("current", flag.ExitOnError)
	g := addGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	current, err := go_migrations.CurrentRevision(ctx, g.migrateOptions())
	if err != nil {
		return err
	}
	if current == "" {
		fmt.Println("(base) - no revisions applied")
		return nil
	}
	head, err := go_migrations.HeadRevision(*g.dir)
	if err == nil && head == current {
		fmt.Printf("%s (head)\n", current)
		return nil
	}
	fmt.Println(current)
	return nil
}

func cmdHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	g := addGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	ordered, err := go_migrations.OrderedRevisions(*g.dir)
	if err != nil {
		return err
	}
	if len(ordered) == 0 {
		fmt.Println("(no revisions)")
		return nil
	}
	for i := len(ordered) - 1; i >= 0; i-- {
		rev := ordered[i]
		head := ""
		if i == len(ordered)-1 {
			head = " (head)"
		}
		fmt.Printf("%s -> %s%s, %s\n", downLabel(rev.DownRevision), rev.ID, head, rev.Message)
	}
	return nil
}

func cmdHeads(args []string) error {
	fs := flag.NewFlagSet("heads", flag.ExitOnError)
	g := addGlobalFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	heads, err := go_migrations.Heads(*g.dir)
	if err != nil {
		return err
	}
	if len(heads) == 0 {
		fmt.Println("(no revisions)")
		return nil
	}
	for _, head := range heads {
		fmt.Printf("%s (head)\n", head)
	}
	return nil
}

func downLabel(down string) string {
	if down == "" {
		return "<base>"
	}
	return down
}

func usage() {
	fmt.Fprint(os.Stderr, `go_migrations - Alembic-style database migrations for Go

Usage:
  go_migrations <command> [flags]

Commands:
  revision   Create a new revision (--autogenerate to diff GORM models)
  upgrade    Apply revisions forward (head, +N, or a revision id)
  downgrade  Roll revisions back (base, -N, or a revision id)
  current    Show the currently-applied revision
  history    List revisions from head to base
  heads      Show head revision(s)
  stamp      Set the recorded revision without running SQL

Global flags (all commands):
  -dir               migrations directory (default "migrations")
  -database-url      PostgreSQL URL (or set DATABASE_URL)
  -migrations-table  revision tracking table (default "go_migrations_version")

Examples:
  go_migrations revision --autogenerate -m "create users"
  go_migrations upgrade head
  go_migrations downgrade -1
  go_migrations current
`)
}
