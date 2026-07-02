package go_migrations

import "sort"

// sortTablesByDependencies orders tables so referenced tables are created first.
func sortTablesByDependencies(tables []Table) []Table {
	if len(tables) <= 1 {
		return tables
	}

	byName := make(map[string]Table, len(tables))
	dependsOn := make(map[string]map[string]struct{}, len(tables))
	for _, table := range tables {
		byName[table.Name] = table
		deps := make(map[string]struct{})
		for _, col := range table.Columns {
			if col.References != nil {
				deps[col.References.Table] = struct{}{}
			}
		}
		dependsOn[table.Name] = deps
	}

	var sorted []Table
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		if visiting[name] {
			return
		}
		visiting[name] = true
		for dep := range dependsOn[name] {
			if _, ok := byName[dep]; ok {
				visit(dep)
			}
		}
		visiting[name] = false
		visited[name] = true
		sorted = append(sorted, byName[name])
	}

	names := make([]string, 0, len(tables))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		visit(name)
	}
	return sorted
}

// reverseTablesByDependencies returns tables in reverse dependency order for drops.
func reverseTablesByDependencies(tables []Table) []Table {
	sorted := sortTablesByDependencies(tables)
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}
	return sorted
}

