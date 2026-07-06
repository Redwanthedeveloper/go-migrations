package go_migrations

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// newRevisionID returns a 12-character hex identifier, matching Alembic's style.
func newRevisionID() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate revision id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// renderRevisionFile renders a single Alembic-style revision file.
func renderRevisionFile(rev Revision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-- revision: %s\n", rev.ID)
	fmt.Fprintf(&b, "-- down_revision: %s\n", rev.DownRevision)
	fmt.Fprintf(&b, "-- create_date: %s\n", rev.CreateDate.Format(time.RFC3339))
	fmt.Fprintf(&b, "-- message: %s\n", rev.Message)

	b.WriteString("\n" + markerUp + "\n")
	if sql := strings.TrimSpace(rev.UpSQL); sql != "" {
		b.WriteString(sql + "\n")
	}
	b.WriteString("\n" + markerDown + "\n")
	if sql := strings.TrimSpace(rev.DownSQL); sql != "" {
		b.WriteString(sql + "\n")
	}
	return b.String()
}

// WriteRevision writes a single revision file named <seq>_<id>_<slug>.sql.
// The sequence prefix keeps migrations in apply order when listed by filename.
func WriteRevision(dir string, rev Revision) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create migrations dir %s: %w", dir, err)
	}

	base, err := revisionFileBase(dir, rev)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, base+".sql")
	if err := os.WriteFile(path, []byte(renderRevisionFile(rev)), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func revisionFileBase(dir string, rev Revision) (string, error) {
	ordered, err := OrderedRevisions(dir)
	if err != nil {
		return "", err
	}
	seq := len(ordered) + 1
	return fmt.Sprintf("%06d_%s_%s", seq, rev.ID, sanitizeName(rev.Message)), nil
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(name, "")
	if name == "" {
		return "migration"
	}
	return name
}
