package go_migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var migrationUpPattern = regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)

// Migration describes a versioned migration on disk.
type Migration struct {
	Version int
	Name    string
	Dir     string
}

// BaseName returns the migration identifier, e.g. 000001_init.
func (m Migration) BaseName() string {
	return fmt.Sprintf("%06d_%s", m.Version, m.Name)
}

func (m Migration) upPath() string   { return filepath.Join(m.Dir, m.BaseName()+".up.sql") }
func (m Migration) downPath() string { return filepath.Join(m.Dir, m.BaseName()+".down.sql") }

// ListMigrations returns migrations sorted by version ascending.
func ListMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	byVersion := make(map[int]Migration)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationUpPattern.FindStringSubmatch(entry.Name())
		if len(matches) < 3 {
			continue
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		byVersion[version] = Migration{
			Version: version,
			Name:    matches[2],
			Dir:     dir,
		}
	}

	migrations := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		migrations = append(migrations, migration)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}
