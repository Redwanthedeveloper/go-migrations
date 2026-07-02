package go_migrations_test

import (
	"testing"

	"github.com/Redwanthedeveloper/go-migrations"
)

func TestMigrationNameInitial(t *testing.T) {
	t.Parallel()

	name := go_migrations.MigrationName(go_migrations.ChangeSet{
		CreateTables: []go_migrations.Table{{Name: "plans"}, {Name: "tenants"}},
	}, go_migrations.DatabaseSchema{})
	if name != "initial" {
		t.Fatalf("name = %q, want initial", name)
	}
}

func TestMigrationNameAddColumn(t *testing.T) {
	t.Parallel()

	name := go_migrations.MigrationName(go_migrations.ChangeSet{
		AddColumns: []go_migrations.ColumnChange{{
			Table:  "tenants",
			Column: go_migrations.Column{Name: "email"},
		}},
	}, go_migrations.DatabaseSchema{Tables: []go_migrations.Table{{Name: "tenants"}}})
	if name != "tenant_email" {
		t.Fatalf("name = %q, want tenant_email", name)
	}
}

func TestMigrationNameNewModel(t *testing.T) {
	t.Parallel()

	name := go_migrations.MigrationName(go_migrations.ChangeSet{
		CreateTables: []go_migrations.Table{{Name: "contacts"}},
	}, go_migrations.DatabaseSchema{Tables: []go_migrations.Table{{Name: "tenants"}}})
	if name != "contact" {
		t.Fatalf("name = %q, want contact", name)
	}
}

func TestMigrationNameMultipleChanges(t *testing.T) {
	t.Parallel()

	name := go_migrations.MigrationName(go_migrations.ChangeSet{
		AddColumns: []go_migrations.ColumnChange{
			{Table: "tenants", Column: go_migrations.Column{Name: "email"}},
			{Table: "plans", Column: go_migrations.Column{Name: "code"}},
		},
	}, go_migrations.DatabaseSchema{Tables: []go_migrations.Table{{Name: "tenants"}, {Name: "plans"}}})
	if name != "plan_code_and_more" {
		t.Fatalf("name = %q, want plan_code_and_more", name)
	}
}
