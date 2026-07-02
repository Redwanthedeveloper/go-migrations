package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/Redwanthedeveloper/go-migrations"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	steps := flag.Int("steps", 0, "number of migration steps (0 = all pending for up, 1 for down)")
	dir := flag.String("dir", "migrations", "migrations directory")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL URL")
	migrationsTable := flag.String("migrations-table", go_migrations.DefaultMigrationsTable, "applied migrations tracking table")
	flag.Parse()

	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "database URL is required (set DATABASE_URL or -database-url)")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := go_migrations.Apply(ctx, go_migrations.MigrateOptions{
		DatabaseURL:     *databaseURL,
		MigrationsDir:   *dir,
		MigrationsTable: *migrationsTable,
		Direction:       *direction,
		Steps:           *steps,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate %s: %v\n", *direction, err)
		os.Exit(1)
	}
}
