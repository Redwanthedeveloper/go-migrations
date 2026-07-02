package go_migrations

import (
	"fmt"
	"sort"
	"strings"
)

// MigrationName derives a Django-style migration slug from schema changes.
func MigrationName(changes ChangeSet, current DatabaseSchema) string {
	if len(current.Tables) == 0 && len(changes.CreateTables) > 0 {
		return "initial"
	}

	if n := nameForSingleChange(changes); n != "" {
		return n
	}

	if len(changes.AddColumns) > 0 {
		adds := append([]ColumnChange(nil), changes.AddColumns...)
		sort.Slice(adds, func(i, j int) bool {
			if adds[i].Table != adds[j].Table {
				return adds[i].Table < adds[j].Table
			}
			return adds[i].Column.Name < adds[j].Column.Name
		})
		c := adds[0]
		return joinName(modelNameFromTable(c.Table), c.Column.Name, changeSuffix(changes))
	}
	if len(changes.CreateTables) > 0 {
		sort.Slice(changes.CreateTables, func(i, j int) bool {
			return changes.CreateTables[i].Name < changes.CreateTables[j].Name
		})
		return modelNameFromTable(changes.CreateTables[0].Name) + changeSuffix(changes)
	}
	if len(changes.DropColumns) > 0 {
		c := changes.DropColumns[0]
		return joinName("remove", c.Column.Name, modelNameFromTable(c.Table)) + changeSuffix(changes)
	}
	if len(changes.CreateIndexes) > 0 {
		c := changes.CreateIndexes[0]
		return joinName(modelNameFromTable(c.Table), indexFieldName(c.Index)) + changeSuffix(changes)
	}
	if len(changes.DropTables) > 0 {
		sort.Strings(changes.DropTables)
		return joinName("delete", modelNameFromTable(changes.DropTables[0])) + changeSuffix(changes)
	}
	return "auto"
}

func nameForSingleChange(changes ChangeSet) string {
	if changeCount(changes) != 1 {
		return ""
	}

	switch {
	case len(changes.AddColumns) == 1:
		c := changes.AddColumns[0]
		return joinName(modelNameFromTable(c.Table), c.Column.Name)
	case len(changes.CreateTables) == 1:
		return modelNameFromTable(changes.CreateTables[0].Name)
	case len(changes.DropColumns) == 1:
		c := changes.DropColumns[0]
		return joinName("remove", c.Column.Name, modelNameFromTable(c.Table))
	case len(changes.CreateIndexes) == 1:
		c := changes.CreateIndexes[0]
		return joinName(modelNameFromTable(c.Table), indexFieldName(c.Index))
	case len(changes.DropIndexes) == 1:
		c := changes.DropIndexes[0]
		return joinName("remove", indexFieldName(c.Index), modelNameFromTable(c.Table))
	case len(changes.DropTables) == 1:
		return joinName("delete", modelNameFromTable(changes.DropTables[0]))
	default:
		return ""
	}
}

func changeCount(changes ChangeSet) int {
	return len(changes.CreateTables) +
		len(changes.DropTables) +
		len(changes.AddColumns) +
		len(changes.DropColumns) +
		len(changes.CreateIndexes) +
		len(changes.DropIndexes)
}

func changeSuffix(changes ChangeSet) string {
	if changeCount(changes) > 1 {
		return "and_more"
	}
	return ""
}

func joinName(parts ...string) string {
	return strings.Join(parts, "_")
}

func indexFieldName(idx Index) string {
	if len(idx.Columns) == 1 {
		return idx.Columns[0]
	}
	return idx.Name
}

func modelNameFromTable(table string) string {
	if table == "" {
		return "model"
	}
	parts := strings.Split(table, "_")
	last := len(parts) - 1
	parts[last] = singularize(parts[last])
	return strings.Join(parts, "_")
}

func singularize(word string) string {
	if word == "" {
		return word
	}
	if strings.HasSuffix(word, "ies") && len(word) > 3 {
		return word[:len(word)-3] + "y"
	}
	if strings.HasSuffix(word, "ses") && len(word) > 3 {
		return word[:len(word)-2]
	}
	if strings.HasSuffix(word, "s") && len(word) > 1 && !strings.HasSuffix(word, "ss") {
		return word[:len(word)-1]
	}
	return word
}

// FormatMigrationBase returns the full migration base name, e.g. 000001_initial.
func FormatMigrationBase(version int, slug string) string {
	return fmt.Sprintf("%06d_%s", version, sanitizeName(slug))
}
