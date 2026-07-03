package go_migrations

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm/schema"
)

type modelPackage struct {
	importPath string
	files      []*ast.File
}

type modelType struct {
	name      string
	tableName string
	fields    []modelField
}

type modelField struct {
	name     string
	goType   string
	tag      string
	settings map[string]string
}

// DiscoverSchema loads GORM models from internal/*/model packages under moduleRoot.
func DiscoverSchema(moduleRoot string) (DatabaseSchema, error) {
	root, err := resolveModuleRoot(moduleRoot)
	if err != nil {
		return DatabaseSchema{}, err
	}

	modulePath, err := readModulePath(root)
	if err != nil {
		return DatabaseSchema{}, err
	}

	pkgs, err := loadModelPackagesAST(root, modulePath)
	if err != nil {
		return DatabaseSchema{}, err
	}

	typeRef := buildTypeRefIndex(pkgs)
	tables := make(map[string]Table)

	for _, pkg := range pkgs {
		for _, model := range pkg.models() {
			table, err := tableFromModel(model)
			if err != nil {
				return DatabaseSchema{}, fmt.Errorf("%s.%s: %w", pkg.importPath, model.name, err)
			}
			if _, ok := tables[table.Name]; ok {
				return DatabaseSchema{}, fmt.Errorf("duplicate table %q in %s", table.Name, pkg.importPath)
			}
			tables[table.Name] = table
		}
	}

	attachBelongsToForeignKeys(pkgs, tables, typeRef)

	out := DatabaseSchema{Tables: make([]Table, 0, len(tables))}
	for _, table := range tables {
		out.Tables = append(out.Tables, table)
	}
	out.Normalize()
	return out, nil
}

func resolveModuleRoot(root string) (string, error) {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
		return abs, nil
	}
	return findModuleRoot(abs)
}

func findModuleRoot(from string) (string, error) {
	dir := from
	if !filepath.IsAbs(dir) {
		var err error
		dir, err = filepath.Abs(dir)
		if err != nil {
			return "", err
		}
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", from)
		}
		dir = parent
	}
}

func readModulePath(moduleRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module path not found in go.mod")
}

func loadModelPackagesAST(moduleRoot, modulePath string) ([]*modelPackage, error) {
	dirs, err := filepath.Glob(filepath.Join(moduleRoot, "internal", "*", "model"))
	if err != nil {
		return nil, fmt.Errorf("glob model packages: %w", err)
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no model packages found under %s/internal/*/model", moduleRoot)
	}

	fset := token.NewFileSet()
	pkgs := make([]*modelPackage, 0, len(dirs))
	for _, dir := range dirs {
		rel, err := filepath.Rel(moduleRoot, dir)
		if err != nil {
			return nil, err
		}
		importPath := modulePath + "/" + filepath.ToSlash(rel)

		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}

		pkg := &modelPackage{importPath: importPath}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			if entry.Name() == "doc.go" || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			pkg.files = append(pkg.files, file)
		}
		if len(pkg.files) > 0 {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs, nil
}

func (p *modelPackage) models() []modelType {
	tableNames := tableNamesFromFiles(p.files)
	var models []modelType

	for _, file := range p.files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Name.IsExported() {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				model := modelType{
					name:      typeSpec.Name.Name,
					tableName: tableNames[typeSpec.Name.Name],
					fields:    fieldsFromStruct(file, structType),
				}
				if !model.isGORMModel() {
					continue
				}
				models = append(models, model)
			}
		}
	}
	return models
}

func tableNamesFromFiles(files []*ast.File) map[string]string {
	names := make(map[string]string)
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "TableName" || fn.Body == nil {
				return true
			}
			if len(fn.Recv.List) != 1 {
				return true
			}
			recvType, ok := fn.Recv.List[0].Type.(*ast.Ident)
			if !ok {
				return true
			}
			if len(fn.Body.List) != 1 {
				return true
			}
			ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return true
			}
			lit, ok := ret.Results[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			names[recvType.Name] = strings.Trim(lit.Value, `"`)
			return true
		})
	}
	return names
}

func fieldsFromStruct(file *ast.File, structType *ast.StructType) []modelField {
	var fields []modelField
	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			tag := ""
			if field.Tag != nil {
				tag = field.Tag.Value
			}
			fields = append(fields, modelField{
				name:     name.Name,
				goType:   typeString(file, field.Type),
				tag:      tag,
				settings: parseGormTag(tag),
			})
		}
	}
	return fields
}

func typeString(file *ast.File, expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if t.Obj != nil && t.Obj.Kind == ast.Typ {
			return t.Name
		}
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(file, t.X)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			importPath := importPathForName(file, ident.Name)
			if importPath != "" {
				return importPath + "." + t.Sel.Name
			}
			return ident.Name + "." + t.Sel.Name
		}
		return exprString(expr)
	default:
		return exprString(expr)
	}
}

func importPathForName(file *ast.File, name string) string {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		local := path
		if imp.Name != nil {
			local = imp.Name.Name
		} else {
			parts := strings.Split(path, "/")
			local = parts[len(parts)-1]
		}
		if local == name {
			return path
		}
	}
	return ""
}

func exprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}

func (m modelType) isGORMModel() bool {
	for _, field := range m.fields {
		if len(field.settings) > 0 {
			return true
		}
	}
	return false
}

type typeRef struct {
	importPath string
	typeName   string
	tableName  string
}

type typeRefIndex map[string]typeRef

func buildTypeRefIndex(pkgs []*modelPackage) typeRefIndex {
	index := make(typeRefIndex)
	namer := schema.NamingStrategy{}

	for _, pkg := range pkgs {
		for _, model := range pkg.models() {
			tableName := model.tableName
			if tableName == "" {
				tableName = namer.TableName(model.name)
			}
			key := pkg.importPath + "." + model.name
			index[key] = typeRef{
				importPath: pkg.importPath,
				typeName:   model.name,
				tableName:  tableName,
			}
		}
	}
	return index
}

func tableFromModel(model modelType) (Table, error) {
	namer := schema.NamingStrategy{}
	tableName := model.tableName
	if tableName == "" {
		tableName = namer.TableName(model.name)
	}

	table := Table{Name: tableName}
	indexes := newIndexCollector()
	seq := 0
	for _, field := range model.fields {
		settings := field.settings
		if len(settings) == 0 {
			continue
		}
		if settings["-"] == "-" {
			continue
		}
		if isAssociationField(field, namer) {
			continue
		}

		col := columnFromModelField(field)
		if col.Name == "" {
			continue
		}
		if col.PrimaryKey {
			table.PrimaryKey = append(table.PrimaryKey, col.Name)
		}
		table.Columns = append(table.Columns, col)

		// Fields sharing an index name form a single (possibly composite) index.
		if name, unique, priority, ok := indexFromSettings(settings); ok {
			indexes.add(name, col.Name, unique, priority, seq)
		}
		seq++
	}
	table.Indexes = indexes.build()
	return table, nil
}

// indexColumn tracks a column's placement within a named index. GORM orders
// composite index columns by priority, then by struct field order.
type indexColumn struct {
	name     string
	priority int
	seq      int
}

type indexEntry struct {
	unique  bool
	columns []indexColumn
}

type indexCollector struct {
	order   []string
	entries map[string]*indexEntry
}

func newIndexCollector() *indexCollector {
	return &indexCollector{entries: make(map[string]*indexEntry)}
}

func (c *indexCollector) add(name, column string, unique bool, priority, seq int) {
	entry, ok := c.entries[name]
	if !ok {
		entry = &indexEntry{}
		c.entries[name] = entry
		c.order = append(c.order, name)
	}
	entry.unique = entry.unique || unique
	entry.columns = append(entry.columns, indexColumn{name: column, priority: priority, seq: seq})
}

func (c *indexCollector) build() []Index {
	if len(c.order) == 0 {
		return nil
	}
	out := make([]Index, 0, len(c.order))
	for _, name := range c.order {
		entry := c.entries[name]
		sort.SliceStable(entry.columns, func(i, j int) bool {
			if entry.columns[i].priority != entry.columns[j].priority {
				return entry.columns[i].priority < entry.columns[j].priority
			}
			return entry.columns[i].seq < entry.columns[j].seq
		})
		cols := make([]string, len(entry.columns))
		for i, col := range entry.columns {
			cols[i] = col.name
		}
		out = append(out, Index{Name: name, Columns: cols, Unique: entry.unique})
	}
	return out
}

func columnFromModelField(field modelField) Column {
	namer := schema.NamingStrategy{}
	settings := field.settings

	columnName := settings["COLUMN"]
	if columnName == "" {
		columnName = namer.ColumnName("", field.name)
	}

	col := Column{
		Name:       columnName,
		Type:       postgresTypeFromGoType(field.goType, settings),
		NotNull:    settingEnabled(settings, "NOT NULL", "NOTNULL"),
		PrimaryKey: settingEnabled(settings, "PRIMARYKEY", "PRIMARY_KEY"),
		Unique:     settingEnabled(settings, "UNIQUE"),
	}

	if strings.HasPrefix(field.goType, "*") {
		col.NotNull = false
	}

	if defaultValue, ok := settings["DEFAULT"]; ok {
		col.Default = formatDefaultLiteral(defaultValue, field.goType)
	}
	if col.PrimaryKey {
		col.NotNull = true
	}
	return col
}

func postgresTypeFromGoType(goType string, settings map[string]string) string {
	if t, ok := settings["TYPE"]; ok && t != "" {
		return strings.ToLower(t)
	}

	goType = strings.TrimPrefix(goType, "*")
	switch {
	case strings.HasSuffix(goType, "uuid.UUID"):
		return "uuid"
	case strings.HasSuffix(goType, "datatypes.JSON"):
		return "jsonb"
	case goType == "time.Time" || strings.HasSuffix(goType, "time.Time"):
		return "timestamptz"
	case goType == "bool":
		return "bool"
	case goType == "string":
		return "text"
	case strings.Contains(goType, "int"), strings.Contains(goType, "uint"):
		return "bigint"
	case strings.Contains(goType, "float"):
		return "float"
	default:
		return "text"
	}
}

func formatDefaultLiteral(value, goType string) string {
	if strings.Contains(value, "(") {
		return value
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value
	}
	if goType == "bool" || strings.HasSuffix(strings.TrimPrefix(goType, "*"), "bool") {
		if value == "true" {
			return "true"
		}
		return "false"
	}
	if strings.HasPrefix(goType, "*") {
		return value
	}
	return "'" + value + "'"
}

func parseGormTag(structTag string) map[string]string {
	tag := reflectStructTag(structTag, "gorm")
	if tag == "" {
		return nil
	}
	return schema.ParseTagSetting(tag, ";")
}

func reflectStructTag(structTag, key string) string {
	structTag = strings.Trim(structTag, "`")
	idx := strings.Index(structTag, key+":")
	if idx < 0 {
		return ""
	}

	value := structTag[idx+len(key)+1:]
	if value == "" {
		return ""
	}

	switch value[0] {
	case '"':
		end := strings.Index(value[1:], `"`)
		if end < 0 {
			return ""
		}
		return value[1 : 1+end]
	case '`':
		end := strings.Index(value[1:], "`")
		if end < 0 {
			return ""
		}
		return value[1 : 1+end]
	default:
		if end := strings.IndexByte(value, ' '); end >= 0 {
			return value[:end]
		}
		return value
	}
}

// indexFromSettings extracts the index name, uniqueness, and column priority
// from a field's GORM settings. It understands the `index:name,priority:N` and
// `uniqueIndex:name,priority:N` forms so composite indexes order columns
// correctly. Bare `index`/`uniqueIndex` (no name) are skipped.
func indexFromSettings(settings map[string]string) (name string, unique bool, priority int, ok bool) {
	raw, unique := settings["UNIQUEINDEX"], true
	if raw == "" {
		raw, unique = settings["INDEX"], false
		if raw == "" {
			return "", false, 0, false
		}
	}

	parts := strings.Split(raw, ",")
	name = strings.TrimSpace(parts[0])
	if name == "" {
		return "", false, 0, false
	}
	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		if key, value, found := strings.Cut(opt, ":"); found && strings.EqualFold(key, "priority") {
			if p, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				priority = p
			}
		}
	}
	return name, unique, priority, true
}

func settingEnabled(settings map[string]string, keys ...string) bool {
	for _, key := range keys {
		value, ok := settings[key]
		if !ok {
			continue
		}
		if value == "false" || value == "-" {
			return false
		}
		return true
	}
	return false
}

func isAssociationField(field modelField, namer schema.NamingStrategy) bool {
	foreignKey, ok := field.settings["FOREIGNKEY"]
	if !ok || foreignKey == "" {
		return false
	}
	return namer.ColumnName("", field.name) != namer.ColumnName("", foreignKey)
}

func attachBelongsToForeignKeys(pkgs []*modelPackage, tables map[string]Table, typeRef typeRefIndex) {
	namer := schema.NamingStrategy{}

	for _, pkg := range pkgs {
		for _, model := range pkg.models() {
			ref, ok := typeRef[pkg.importPath+"."+model.name]
			if !ok {
				continue
			}
			table, ok := tables[ref.tableName]
			if !ok {
				continue
			}

			for _, field := range model.fields {
				settings := field.settings
				if len(settings) == 0 {
					continue
				}
				foreignKey := settings["FOREIGNKEY"]
				if foreignKey == "" {
					continue
				}

				targetKey := relationTypeKey(field.goType, typeRef)
				if targetKey == "" {
					continue
				}
				target, ok := typeRef[targetKey]
				if !ok {
					continue
				}

				references := settings["REFERENCES"]
				if references == "" {
					references = "ID"
				}

				onDelete := ""
				if constraint := settings["CONSTRAINT"]; constraint != "" {
					onDelete = strings.ToUpper(schema.ParseTagSetting(constraint, ",")["ONDELETE"])
				}

				fkColumn := namer.ColumnName("", foreignKey)
				refColumn := namer.ColumnName("", references)
				for i, col := range table.Columns {
					if col.Name != fkColumn {
						continue
					}
					table.Columns[i].References = &ForeignKey{
						Table:    target.tableName,
						Column:   refColumn,
						OnDelete: onDelete,
					}
				}
			}
			tables[ref.tableName] = table
		}
	}
}

func relationTypeKey(goType string, typeRef typeRefIndex) string {
	goType = strings.TrimPrefix(goType, "*")
	if strings.Contains(goType, ".") {
		for key := range typeRef {
			if strings.HasSuffix(key, "."+typeNameFromGoType(goType)) {
				return key
			}
		}
	}
	for key, ref := range typeRef {
		if ref.typeName == goType {
			return key
		}
	}
	return ""
}

func typeNameFromGoType(goType string) string {
	if idx := strings.LastIndex(goType, "."); idx >= 0 {
		return goType[idx+1:]
	}
	return goType
}
