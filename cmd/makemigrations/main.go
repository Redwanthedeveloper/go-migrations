package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/Redwanthedeveloper/go-migrations"
)

func main() {
	dir := flag.String("dir", "migrations", "migrations output directory")
	modelsRoot := flag.String("models", ".", "module root to discover GORM models from")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "PostgreSQL URL (required when migrations already exist)")
	migrationsTable := flag.String("migrations-table", go_migrations.DefaultMigrationsTable, "applied migrations tracking table")
	dryRun := flag.Bool("dry-run", false, "print SQL without writing files")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	result, err := go_migrations.Generate(ctx, go_migrations.Options{
		ModelsRoot:      *modelsRoot,
		MigrationsDir:   *dir,
		DatabaseURL:     *databaseURL,
		MigrationsTable: *migrationsTable,
		DryRun:          *dryRun,
	})
	if err != nil {
		if errors.Is(err, go_migrations.ErrNoChanges) {
			fmt.Println("No changes detected.")
			return
		}
		fmt.Fprintf(os.Stderr, "makemigrations: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Printf("-- %s --\n", result.BaseName)
		fmt.Println("-- up --")
		fmt.Print(result.UpSQL)
		fmt.Println("-- down --")
		fmt.Print(result.DownSQL)
		return
	}

	fmt.Printf("Created migration %s\n", result.BaseName)
	fmt.Printf("  %s.up.sql\n", result.BaseName)
	fmt.Printf("  %s.down.sql\n", result.BaseName)
}
