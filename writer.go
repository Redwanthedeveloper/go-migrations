package go_migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var migrationFilePattern = regexp.MustCompile(`^(\d+)_.+\.(up|down)\.sql$`)

// NextVersion returns the next sequential migration version based on files in dir.
func NextVersion(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	maxVersion := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if len(matches) < 2 {
			continue
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		if version > maxVersion {
			maxVersion = version
		}
	}
	return maxVersion + 1, nil
}

// WriteMigration writes paired up/down SQL files.
func WriteMigration(dir string, version int, name, upSQL, downSQL string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create migrations dir %s: %w", dir, err)
	}

	base := fmt.Sprintf("%06d_%s", version, sanitizeName(name))
	upPath := filepath.Join(dir, base+".up.sql")
	downPath := filepath.Join(dir, base+".down.sql")

	if err := os.WriteFile(upPath, []byte(upSQL), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", upPath, err)
	}
	if err := os.WriteFile(downPath, []byte(downSQL), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", downPath, err)
	}
	return base, nil
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

