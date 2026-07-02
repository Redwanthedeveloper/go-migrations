package go_migrations

import (
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm/schema"
)

// SchemaFromModels builds a database schema from the provided GORM models.
func SchemaFromModels(models []any) (DatabaseSchema, error) {
	cache := &sync.Map{}
	namer := schema.NamingStrategy{}

	tables := make(map[string]Table)
	for _, model := range models {
		parsed, err := schema.Parse(model, cache, namer)
		if err != nil {
			return DatabaseSchema{}, fmt.Errorf("parse model %T: %w", model, err)
		}

		table := Table{Name: parsed.Table}
		pk := make([]string, 0, len(parsed.PrimaryFields))
		for _, field := range parsed.PrimaryFields {
			pk = append(pk, field.DBName)
		}
		table.PrimaryKey = pk

		for _, field := range parsed.Fields {
			if field.IgnoreMigration || field.DBName == "" {
				continue
			}
			col := columnFromField(field)
			table.Columns = append(table.Columns, col)
		}

		for _, idx := range parsed.ParseIndexes() {
			if idx.Name == "" {
				continue
			}
			columns := make([]string, 0, len(idx.Fields))
			for _, field := range idx.Fields {
				columns = append(columns, field.DBName)
			}
			table.Indexes = append(table.Indexes, Index{
				Name:    idx.Name,
				Columns: columns,
				Unique:  idx.Class == "UNIQUE",
			})
		}

		tables[table.Name] = table
	}

	// Attach foreign keys declared via BelongsTo associations.
	for _, model := range models {
		parsed, err := schema.Parse(model, cache, namer)
		if err != nil {
			return DatabaseSchema{}, fmt.Errorf("parse model %T: %w", model, err)
		}
		for _, rel := range parsed.Relationships.Relations {
			if rel.Type != schema.BelongsTo {
				continue
			}

			onDelete := ""
			if constraint := rel.ParseConstraint(); constraint != nil && constraint.OnDelete != "" {
				onDelete = strings.ToUpper(constraint.OnDelete)
			}

			for _, ref := range rel.References {
				if ref.ForeignKey == nil || ref.PrimaryKey == nil || rel.FieldSchema == nil {
					continue
				}

				foreignTable, ok := tables[parsed.Table]
				if !ok {
					continue
				}
				for i, col := range foreignTable.Columns {
					if col.Name != ref.ForeignKey.DBName {
						continue
					}
					foreignTable.Columns[i].References = &ForeignKey{
						Table:    rel.FieldSchema.Table,
						Column:   ref.PrimaryKey.DBName,
						OnDelete: onDelete,
					}
				}
				tables[parsed.Table] = foreignTable
			}
		}
	}

	out := DatabaseSchema{Tables: make([]Table, 0, len(tables))}
	for _, table := range tables {
		out.Tables = append(out.Tables, table)
	}
	out.Normalize()
	return out, nil
}

func columnFromField(field *schema.Field) Column {
	col := Column{
		Name:       field.DBName,
		Type:       postgresType(field),
		NotNull:    field.NotNull,
		PrimaryKey: field.PrimaryKey,
		Unique:     field.Unique,
	}

	if field.HasDefaultValue {
		col.Default = formatFieldDefault(field)
	}
	if col.PrimaryKey {
		col.NotNull = true
	}
	return col
}

func postgresType(field *schema.Field) string {
	if field.DataType != "" && field.DataType != schema.DataType("decimal") {
		return strings.ToLower(string(field.DataType))
	}

	switch field.FieldType.Kind() {
	case 1: // bool
		return "boolean"
	case 2, 3, 4, 5, 6: // int types
		return "bigint"
	case 8: // string
		return "text"
	default:
		typeName := field.FieldType.String()
		switch {
		case strings.Contains(typeName, "uuid.UUID"):
			return "uuid"
		case strings.Contains(typeName, "time.Time"):
			return "timestamptz"
		case strings.Contains(typeName, "datatypes.JSON"):
			return "jsonb"
		default:
			if field.DataType != "" {
				return strings.ToLower(string(field.DataType))
			}
			return "text"
		}
	}
}

func formatFieldDefault(field *schema.Field) string {
	if field.DefaultValueInterface != nil {
		switch v := field.DefaultValueInterface.(type) {
		case bool:
			if v {
				return "true"
			}
			return "false"
		}
	}
	return formatDefault(field.DefaultValue)
}

func formatDefault(value interface{}) string {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") {
			return v
		}
		if strings.Contains(v, "(") {
			return v
		}
		return "'" + v + "'"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v)
	case float32, float64:
		return fmt.Sprint(v)
	default:
		return fmt.Sprint(v)
	}
}

