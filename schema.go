package go_migrations

import (
	"encoding/json"
	"sort"
)

// DatabaseSchema describes the PostgreSQL schema used for diffs.
type DatabaseSchema struct {
	Tables []Table `json:"tables"`
}

// Table describes a PostgreSQL table.
type Table struct {
	Name       string   `json:"name"`
	Columns    []Column `json:"columns"`
	Indexes    []Index  `json:"indexes,omitempty"`
	PrimaryKey []string `json:"primary_key,omitempty"`
}

// Column describes a PostgreSQL column.
type Column struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	NotNull    bool        `json:"not_null"`
	Default    string      `json:"default,omitempty"`
	PrimaryKey bool        `json:"primary_key,omitempty"`
	Unique     bool        `json:"unique,omitempty"`
	References *ForeignKey `json:"references,omitempty"`
}

// ForeignKey describes a column reference.
type ForeignKey struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	OnDelete string `json:"on_delete,omitempty"`
}

// Index describes a PostgreSQL index.
type Index struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
}

func (s DatabaseSchema) tableMap() map[string]Table {
	m := make(map[string]Table, len(s.Tables))
	for _, t := range s.Tables {
		m[t.Name] = t
	}
	return m
}

func (t Table) columnMap() map[string]Column {
	m := make(map[string]Column, len(t.Columns))
	for _, c := range t.Columns {
		m[c.Name] = c
	}
	return m
}

func (t Table) indexMap() map[string]Index {
	m := make(map[string]Index, len(t.Indexes))
	for _, idx := range t.Indexes {
		m[idx.Name] = idx
	}
	return m
}

// Normalize sorts tables, columns, and indexes for stable comparison and output.
func (s *DatabaseSchema) Normalize() {
	sort.Slice(s.Tables, func(i, j int) bool { return s.Tables[i].Name < s.Tables[j].Name })
	for i := range s.Tables {
		sort.Slice(s.Tables[i].Columns, func(a, b int) bool {
			return s.Tables[i].Columns[a].Name < s.Tables[i].Columns[b].Name
		})
		sort.Slice(s.Tables[i].Indexes, func(a, b int) bool {
			return s.Tables[i].Indexes[a].Name < s.Tables[i].Indexes[b].Name
		})
		for j := range s.Tables[i].Indexes {
			sort.Strings(s.Tables[i].Indexes[j].Columns)
		}
		sort.Strings(s.Tables[i].PrimaryKey)
	}
}

// Equal reports whether two schemas describe the same database structure.
func (s DatabaseSchema) Equal(other DatabaseSchema) bool {
	a, b := s, other
	a.Normalize()
	b.Normalize()
	aj, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bj, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(aj) == string(bj)
}

