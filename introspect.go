package go_migrations

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// SchemaFromDatabaseURL introspects the live PostgreSQL schema.
func SchemaFromDatabaseURL(ctx context.Context, databaseURL string, excludeTables ...string) (DatabaseSchema, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return DatabaseSchema{}, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return DatabaseSchema{}, fmt.Errorf("ping database: %w", err)
	}

	return SchemaFromDatabase(ctx, db, excludeTables...)
}

// SchemaFromDatabase introspects public tables from a connected database.
func SchemaFromDatabase(ctx context.Context, db *sql.DB, excludeTables ...string) (DatabaseSchema, error) {
	tables, err := listTables(ctx, db, excludeTables)
	if err != nil {
		return DatabaseSchema{}, err
	}

	schema := DatabaseSchema{Tables: make([]Table, 0, len(tables))}
	for _, tableName := range tables {
		table, err := introspectTable(ctx, db, tableName)
		if err != nil {
			return DatabaseSchema{}, err
		}
		schema.Tables = append(schema.Tables, table)
	}
	schema.Normalize()
	return schema, nil
}

func listTables(ctx context.Context, db *sql.DB, excludeTables []string) ([]string, error) {
	excluded := introspectExcludedTables(excludeTables...)
	placeholders := make([]string, len(excluded))
	args := make([]any, len(excluded))
	for i, name := range excluded {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = name
	}
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'`
	if len(excluded) > 0 {
		query += fmt.Sprintf("\n\t\t  AND table_name NOT IN (%s)", strings.Join(placeholders, ", "))
	}
	query += "\n\t\tORDER BY table_name"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func introspectTable(ctx context.Context, db *sql.DB, tableName string) (Table, error) {
	table := Table{Name: tableName}

	columns, err := introspectColumns(ctx, db, tableName)
	if err != nil {
		return Table{}, err
	}
	table.Columns = columns

	pk, err := introspectPrimaryKey(ctx, db, tableName)
	if err != nil {
		return Table{}, err
	}
	table.PrimaryKey = pk
	for i, col := range table.Columns {
		for _, pkCol := range pk {
			if col.Name == pkCol {
				table.Columns[i].PrimaryKey = true
				table.Columns[i].NotNull = true
			}
		}
	}

	indexes, err := introspectIndexes(ctx, db, tableName)
	if err != nil {
		return Table{}, err
	}
	table.Indexes = indexes

	return table, nil
}

func introspectColumns(ctx context.Context, db *sql.DB, tableName string) ([]Column, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, udt_name, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position`, tableName)
	if err != nil {
		return nil, fmt.Errorf("list columns for %s: %w", tableName, err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var name, udtName, nullable string
		var defaultValue sql.NullString
		if err := rows.Scan(&name, &udtName, &nullable, &defaultValue); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		col := Column{
			Name:    name,
			Type:    mapPostgresType(udtName),
			NotNull: nullable == "NO",
		}
		if defaultValue.Valid {
			col.Default = normalizeDefault(defaultValue.String)
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fks, err := introspectForeignKeys(ctx, db, tableName)
	if err != nil {
		return nil, err
	}
	for i, col := range columns {
		if fk, ok := fks[col.Name]; ok {
			columns[i].References = &fk
		}
	}
	return columns, nil
}

func introspectForeignKeys(ctx context.Context, db *sql.DB, tableName string) (map[string]ForeignKey, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			kcu.column_name,
			ccu.table_name AS foreign_table,
			ccu.column_name AS foreign_column,
			rc.delete_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		JOIN information_schema.referential_constraints rc
			ON rc.constraint_name = tc.constraint_name
			AND rc.constraint_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema = 'public'
		  AND tc.table_name = $1`, tableName)
	if err != nil {
		return nil, fmt.Errorf("list foreign keys for %s: %w", tableName, err)
	}
	defer rows.Close()

	fks := make(map[string]ForeignKey)
	for rows.Next() {
		var column, foreignTable, foreignColumn, deleteRule string
		if err := rows.Scan(&column, &foreignTable, &foreignColumn, &deleteRule); err != nil {
			return nil, fmt.Errorf("scan foreign key: %w", err)
		}
		fk := ForeignKey{Table: foreignTable, Column: foreignColumn}
		if deleteRule != "" && deleteRule != "NO ACTION" {
			fk.OnDelete = strings.ToUpper(deleteRule)
		}
		fks[column] = fk
	}
	return fks, rows.Err()
}

func introspectPrimaryKey(ctx context.Context, db *sql.DB, tableName string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
		  AND tc.table_schema = 'public'
		  AND tc.table_name = $1
		ORDER BY kcu.ordinal_position`, tableName)
	if err != nil {
		return nil, fmt.Errorf("list primary key for %s: %w", tableName, err)
	}
	defer rows.Close()

	var pk []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, fmt.Errorf("scan primary key: %w", err)
		}
		pk = append(pk, column)
	}
	return pk, rows.Err()
}

func introspectIndexes(ctx context.Context, db *sql.DB, tableName string) ([]Index, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			i.relname AS index_name,
			a.attname AS column_name,
			ix.indisunique
		FROM pg_class t
		JOIN pg_index ix ON t.oid = ix.indrelid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = 'public'
		  AND t.relname = $1
		  AND NOT ix.indisprimary
		ORDER BY i.relname, array_position(ix.indkey, a.attnum)`, tableName)
	if err != nil {
		return nil, fmt.Errorf("list indexes for %s: %w", tableName, err)
	}
	defer rows.Close()

	byName := make(map[string]*Index)
	var order []string
	for rows.Next() {
		var indexName, columnName string
		var unique bool
		if err := rows.Scan(&indexName, &columnName, &unique); err != nil {
			return nil, fmt.Errorf("scan index: %w", err)
		}
		idx, ok := byName[indexName]
		if !ok {
			idx = &Index{Name: indexName, Unique: unique}
			byName[indexName] = idx
			order = append(order, indexName)
		}
		idx.Columns = append(idx.Columns, columnName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	indexes := make([]Index, 0, len(order))
	for _, name := range order {
		indexes = append(indexes, *byName[name])
	}
	return indexes, nil
}

func mapPostgresType(udtName string) string {
	switch udtName {
	case "uuid":
		return "uuid"
	case "text", "varchar", "bpchar":
		return "text"
	case "jsonb":
		return "jsonb"
	case "bool":
		return "bool"
	case "timestamptz":
		return "timestamptz"
	case "int2", "int4", "int8":
		return "bigint"
	case "float4", "float8":
		return "float"
	default:
		return udtName
	}
}

func normalizeDefault(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'::text") {
		return "'" + strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'::text") + "'"
	}
	if strings.HasSuffix(value, "::text") {
		inner := strings.TrimSuffix(value, "::text")
		if strings.HasPrefix(inner, "'") {
			return inner
		}
	}
	if strings.Contains(value, "::") {
		parts := strings.Split(value, "::")
		return parts[0]
	}
	return value
}

func introspectExcludedTables(extra ...string) []string {
	seen := map[string]struct{}{
		DefaultMigrationsTable: {},
		"schema_migrations":    {},
		"django_migrations":    {},
	}
	for _, name := range extra {
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
