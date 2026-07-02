package go_migrations

import (
	"fmt"
	"sort"
	"strings"
)

// GenerateUp renders upgrade SQL for golang-migrate.
func GenerateUp(changes ChangeSet) string {
	var parts []string

	createTables := sortTablesByDependencies(append([]Table(nil), changes.CreateTables...))
	for _, table := range createTables {
		parts = append(parts, renderCreateTable(table))
	}

	for _, table := range createTables {
		indexes := append([]Index(nil), table.Indexes...)
		sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
		for _, idx := range indexes {
			parts = append(parts, renderCreateIndex(table.Name, idx))
		}
	}

	addColumns := append([]ColumnChange(nil), changes.AddColumns...)
	sort.Slice(addColumns, func(i, j int) bool {
		if addColumns[i].Table == addColumns[j].Table {
			return addColumns[i].Column.Name < addColumns[j].Column.Name
		}
		return addColumns[i].Table < addColumns[j].Table
	})
	for _, change := range addColumns {
		parts = append(parts, fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s;",
			quoteIdent(change.Table),
			renderColumnDefinition(change.Column, false),
		))
	}

	createIndexes := append([]IndexChange(nil), changes.CreateIndexes...)
	sort.Slice(createIndexes, func(i, j int) bool {
		if createIndexes[i].Table == createIndexes[j].Table {
			return createIndexes[i].Index.Name < createIndexes[j].Index.Name
		}
		return createIndexes[i].Table < createIndexes[j].Table
	})
	for _, change := range createIndexes {
		parts = append(parts, renderCreateIndex(change.Table, change.Index))
	}

	return strings.Join(parts, "\n\n") + "\n"
}

// GenerateDown renders downgrade SQL that reverses GenerateUp.
func GenerateDown(changes ChangeSet) string {
	var parts []string

	dropIndexes := append([]IndexChange(nil), changes.CreateIndexes...)
	sort.Slice(dropIndexes, func(i, j int) bool {
		if dropIndexes[i].Table == dropIndexes[j].Table {
			return dropIndexes[i].Index.Name > dropIndexes[j].Index.Name
		}
		return dropIndexes[i].Table > dropIndexes[j].Table
	})
	for _, change := range dropIndexes {
		parts = append(parts, fmt.Sprintf(
			"DROP INDEX IF EXISTS %s;",
			quoteIdent(change.Index.Name),
		))
	}

	dropColumns := append([]ColumnChange(nil), changes.AddColumns...)
	sort.Slice(dropColumns, func(i, j int) bool {
		if dropColumns[i].Table == dropColumns[j].Table {
			return dropColumns[i].Column.Name > dropColumns[j].Column.Name
		}
		return dropColumns[i].Table > dropColumns[j].Table
	})
	for _, change := range dropColumns {
		parts = append(parts, fmt.Sprintf(
			"ALTER TABLE %s DROP COLUMN IF EXISTS %s;",
			quoteIdent(change.Table),
			quoteIdent(change.Column.Name),
		))
	}

	dropTables := reverseTablesByDependencies(append([]Table(nil), changes.CreateTables...))
	for _, table := range dropTables {
		indexes := append([]Index(nil), table.Indexes...)
		sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name > indexes[j].Name })
		for _, idx := range indexes {
			parts = append(parts, fmt.Sprintf(
				"DROP INDEX IF EXISTS %s;",
				quoteIdent(idx.Name),
			))
		}
	}
	for _, table := range dropTables {
		parts = append(parts, fmt.Sprintf("DROP TABLE IF EXISTS %s;", quoteIdent(table.Name)))
	}

	// Reverse of DropTables from up: recreate dropped tables from old schema is not
	// captured here; those require a new makemigrations after restoring models.
	for _, name := range changes.DropTables {
		parts = append(parts, fmt.Sprintf("-- TODO: recreate table %s (removed from models)", name))
	}

	for _, change := range changes.DropColumns {
		parts = append(parts, fmt.Sprintf(
			"-- TODO: re-add column %s.%s (removed from models)",
			change.Table,
			change.Column.Name,
		))
	}

	if len(parts) == 0 {
		return "\n"
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func renderCreateTable(table Table) string {
	columns := append([]Column{}, table.Columns...)
	sort.Slice(columns, func(i, j int) bool { return columns[i].Name < columns[j].Name })

	lines := make([]string, 0, len(columns)+1)
	pkCols := make(map[string]struct{}, len(table.PrimaryKey))
	for _, name := range table.PrimaryKey {
		pkCols[name] = struct{}{}
	}

	for _, col := range columns {
		inPK := len(table.PrimaryKey) > 1
		if _, ok := pkCols[col.Name]; ok {
			inPK = true
		}
		lines = append(lines, "    "+renderColumnDefinition(col, inPK && len(table.PrimaryKey) == 1))
	}

	if len(table.PrimaryKey) > 1 {
		quoted := make([]string, len(table.PrimaryKey))
		for i, name := range table.PrimaryKey {
			quoted[i] = quoteIdent(name)
		}
		lines = append(lines, "    PRIMARY KEY ("+strings.Join(quoted, ", ")+")")
	}

	return fmt.Sprintf(
		"CREATE TABLE %s (\n%s\n);",
		quoteIdent(table.Name),
		strings.Join(lines, ",\n"),
	)
}

func renderColumnDefinition(col Column, inlinePK bool) string {
	parts := []string{quoteIdent(col.Name), strings.ToUpper(col.Type)}

	if inlinePK && col.PrimaryKey {
		parts = append(parts, "PRIMARY KEY")
	}

	if col.NotNull {
		parts = append(parts, "NOT NULL")
	}

	if col.Default != "" {
		parts = append(parts, "DEFAULT "+col.Default)
	}

	if col.References != nil {
		ref := fmt.Sprintf(
			"REFERENCES %s(%s)",
			quoteIdent(col.References.Table),
			quoteIdent(col.References.Column),
		)
		if col.References.OnDelete != "" {
			ref += " ON DELETE " + col.References.OnDelete
		}
		parts = append(parts, ref)
	}

	if col.Unique && !col.PrimaryKey {
		parts = append(parts, "UNIQUE")
	}

	return strings.Join(parts, " ")
}

func renderCreateIndex(table string, idx Index) string {
	cols := make([]string, len(idx.Columns))
	for i, col := range idx.Columns {
		cols[i] = quoteIdent(col)
	}
	kind := "INDEX"
	if idx.Unique {
		kind = "UNIQUE INDEX"
	}
	return fmt.Sprintf(
		"CREATE %s %s ON %s(%s);",
		kind,
		quoteIdent(idx.Name),
		quoteIdent(table),
		strings.Join(cols, ", "),
	)
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func validateSQLIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("identifier is empty")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("invalid identifier %q", name)
	}
	return nil
}

