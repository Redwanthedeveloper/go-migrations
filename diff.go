package go_migrations

// ChangeSet holds ordered schema operations for a migration.
type ChangeSet struct {
	CreateTables  []Table
	DropTables    []string
	AddColumns    []ColumnChange
	DropColumns   []ColumnChange
	CreateIndexes []IndexChange
	DropIndexes   []IndexChange
}

// ColumnChange targets a single column on a table.
type ColumnChange struct {
	Table  string
	Column Column
}

// IndexChange targets a single index on a table.
type IndexChange struct {
	Table string
	Index Index
}

// Diff computes migration operations from old to new schema.
func Diff(old, new DatabaseSchema) ChangeSet {
	oldTables := old.tableMap()
	newTables := new.tableMap()

	var changes ChangeSet

	for name, table := range newTables {
		prev, ok := oldTables[name]
		if !ok {
			changes.CreateTables = append(changes.CreateTables, table)
			continue
		}

		oldCols := prev.columnMap()
		newCols := table.columnMap()
		for colName, col := range newCols {
			if _, exists := oldCols[colName]; !exists {
				changes.AddColumns = append(changes.AddColumns, ColumnChange{Table: name, Column: col})
			}
		}
		for colName := range oldCols {
			if _, exists := newCols[colName]; !exists {
				changes.DropColumns = append(changes.DropColumns, ColumnChange{Table: name, Column: oldCols[colName]})
			}
		}

		oldIdx := prev.indexMap()
		newIdx := table.indexMap()
		for idxName, idx := range newIdx {
			if _, exists := oldIdx[idxName]; !exists {
				changes.CreateIndexes = append(changes.CreateIndexes, IndexChange{Table: name, Index: idx})
			}
		}
		for idxName, idx := range oldIdx {
			if _, exists := newIdx[idxName]; !exists {
				changes.DropIndexes = append(changes.DropIndexes, IndexChange{Table: name, Index: idx})
			}
		}
	}

	for name := range oldTables {
		if _, ok := newTables[name]; !ok {
			changes.DropTables = append(changes.DropTables, name)
		}
	}

	return changes
}

// IsEmpty reports whether the change set would produce SQL.
func (c ChangeSet) IsEmpty() bool {
	return len(c.CreateTables) == 0 &&
		len(c.DropTables) == 0 &&
		len(c.AddColumns) == 0 &&
		len(c.DropColumns) == 0 &&
		len(c.CreateIndexes) == 0 &&
		len(c.DropIndexes) == 0
}

