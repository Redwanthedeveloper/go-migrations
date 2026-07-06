package go_migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Revision is a single migration file, Alembic-style: each revision carries its
// own identifier and a pointer to the revision it descends from (down_revision).
type Revision struct {
	ID           string    // this revision's identifier
	DownRevision string    // parent revision id ("" for the base revision)
	Message      string    // human-readable slug
	CreateDate   time.Time // when the revision file was generated
	Path         string    // file path on disk
	UpSQL        string    // SQL executed on upgrade
	DownSQL      string    // SQL executed on downgrade
}

const (
	markerUp   = "-- migrate:up"
	markerDown = "-- migrate:down"
)

// LoadRevisions parses every revision file in dir, ordered base to head.
func LoadRevisions(dir string) ([]Revision, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	var revisions []Revision
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		rev, err := ParseRevisionFile(path)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, rev)
	}
	return OrderRevisions(revisions)
}

// ParseRevisionFile reads and parses a single revision file.
func ParseRevisionFile(path string) (Revision, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Revision{}, fmt.Errorf("read %s: %w", path, err)
	}
	rev, err := parseRevisionContent(data)
	if err != nil {
		return Revision{}, fmt.Errorf("%s: %w", path, err)
	}
	rev.Path = path
	return rev, nil
}

func parseRevisionContent(data []byte) (Revision, error) {
	var rev Revision
	section := "" // "", "up" or "down"
	var up, down []string

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == markerUp:
			section = "up"
		case trimmed == markerDown:
			section = "down"
		case section == "" && strings.HasPrefix(trimmed, "-- revision:"):
			rev.ID = strings.TrimSpace(strings.TrimPrefix(trimmed, "-- revision:"))
		case section == "" && strings.HasPrefix(trimmed, "-- down_revision:"):
			rev.DownRevision = strings.TrimSpace(strings.TrimPrefix(trimmed, "-- down_revision:"))
		case section == "" && strings.HasPrefix(trimmed, "-- create_date:"):
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(strings.TrimPrefix(trimmed, "-- create_date:"))); err == nil {
				rev.CreateDate = t
			}
		case section == "" && strings.HasPrefix(trimmed, "-- message:"):
			rev.Message = strings.TrimSpace(strings.TrimPrefix(trimmed, "-- message:"))
		case section == "up":
			up = append(up, line)
		case section == "down":
			down = append(down, line)
		}
	}

	if rev.ID == "" {
		return Revision{}, fmt.Errorf("missing %q header", "-- revision:")
	}
	rev.UpSQL = strings.TrimSpace(strings.Join(up, "\n"))
	rev.DownSQL = strings.TrimSpace(strings.Join(down, "\n"))
	return rev, nil
}

// OrderRevisions returns revisions ordered from base to head by walking the
// down_revision chain. Branching (multiple heads) is not supported.
func OrderRevisions(revisions []Revision) ([]Revision, error) {
	if len(revisions) == 0 {
		return nil, nil
	}

	byID := make(map[string]Revision, len(revisions))
	for _, rev := range revisions {
		if _, dup := byID[rev.ID]; dup {
			return nil, fmt.Errorf("duplicate revision id %q", rev.ID)
		}
		byID[rev.ID] = rev
	}

	children := make(map[string][]string)
	var bases []string
	for _, rev := range revisions {
		if rev.DownRevision == "" {
			bases = append(bases, rev.ID)
			continue
		}
		if _, ok := byID[rev.DownRevision]; !ok {
			return nil, fmt.Errorf("revision %q references unknown down_revision %q", rev.ID, rev.DownRevision)
		}
		children[rev.DownRevision] = append(children[rev.DownRevision], rev.ID)
	}

	switch len(bases) {
	case 0:
		return nil, fmt.Errorf("no base revision found (cyclic history?)")
	case 1:
	default:
		sort.Strings(bases)
		return nil, fmt.Errorf("multiple base revisions: %s", strings.Join(bases, ", "))
	}

	ordered := make([]Revision, 0, len(revisions))
	current := bases[0]
	for {
		ordered = append(ordered, byID[current])
		next := children[current]
		if len(next) == 0 {
			break
		}
		if len(next) > 1 {
			sort.Strings(next)
			return nil, fmt.Errorf("multiple heads descend from %q: %s (branching not supported)", current, strings.Join(next, ", "))
		}
		current = next[0]
	}

	if len(ordered) != len(revisions) {
		return nil, fmt.Errorf("revision history is disconnected or cyclic")
	}
	return ordered, nil
}

// OrderedRevisions loads revisions from dir (base to head).
func OrderedRevisions(dir string) ([]Revision, error) {
	return LoadRevisions(dir)
}

// Heads returns every revision that has no descendant.
func Heads(dir string) ([]string, error) {
	revisions, err := LoadRevisions(dir)
	if err != nil {
		return nil, err
	}
	return headIDs(revisions), nil
}

func headIDs(revisions []Revision) []string {
	hasChild := make(map[string]bool, len(revisions))
	for _, rev := range revisions {
		if rev.DownRevision != "" {
			hasChild[rev.DownRevision] = true
		}
	}
	var heads []string
	for _, rev := range revisions {
		if !hasChild[rev.ID] {
			heads = append(heads, rev.ID)
		}
	}
	sort.Strings(heads)
	return heads
}

// HeadRevision returns the single head revision id, or "" when there are no
// revisions. It errors when the history has multiple heads.
func HeadRevision(dir string) (string, error) {
	revisions, err := LoadRevisions(dir)
	if err != nil {
		return "", err
	}
	if len(revisions) == 0 {
		return "", nil
	}
	heads := headIDs(revisions)
	if len(heads) == 0 {
		return "", fmt.Errorf("no head revision found (cyclic history?)")
	}
	if len(heads) > 1 {
		return "", fmt.Errorf("multiple heads: %s", strings.Join(heads, ", "))
	}
	return heads[0], nil
}
