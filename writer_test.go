package go_migrations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Redwanthedeveloper/go-migrations"
)

func TestWriteRevisionUsesSequentialFilenames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(id, down, message string) {
		t.Helper()
		path, err := go_migrations.WriteRevision(dir, go_migrations.Revision{
			ID:           id,
			DownRevision: down,
			Message:      message,
			CreateDate:   time.Now().UTC(),
			UpSQL:        "SELECT 1;",
			DownSQL:      "SELECT 1;",
		})
		if err != nil {
			t.Fatalf("WriteRevision(%s) error = %v", id, err)
		}
		if !strings.HasPrefix(filepath.Base(path), "00000") {
			t.Fatalf("path = %q, want sequential prefix", path)
		}
	}

	write("aaa111", "", "init")
	write("bbb222", "aaa111", "second")
	write("ccc333", "bbb222", "third")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("files = %d, want 3", len(entries))
	}

	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	wantPrefixes := []string{"000001_", "000002_", "000003_"}
	for i, prefix := range wantPrefixes {
		if !strings.HasPrefix(names[i], prefix) {
			t.Fatalf("names = %v, want sequential prefixes %v", names, wantPrefixes)
		}
	}
}

func TestLoadRevisionsReturnsChainOrderRegardlessOfFilenameOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := []struct {
		name    string
		id      string
		down    string
		message string
	}{
		{"zzz999_third.sql", "ccc333", "bbb222", "third"},
		{"aaa111_init.sql", "aaa111", "", "init"},
		{"mmm555_second.sql", "bbb222", "aaa111", "second"},
	}
	for _, file := range files {
		content := "-- revision: " + file.id + "\n" +
			"-- down_revision: " + file.down + "\n" +
			"-- create_date: 2026-07-06T12:00:00Z\n" +
			"-- message: " + file.message + "\n\n" +
			"-- migrate:up\nSELECT 1;\n\n-- migrate:down\nSELECT 1;\n"
		if err := os.WriteFile(filepath.Join(dir, file.name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	revisions, err := go_migrations.LoadRevisions(dir)
	if err != nil {
		t.Fatalf("LoadRevisions() error = %v", err)
	}
	got := []string{revisions[0].ID, revisions[1].ID, revisions[2].ID}
	want := []string{"aaa111", "bbb222", "ccc333"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("order = %v, want %v", got, want)
	}
}
