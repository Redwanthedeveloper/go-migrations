package go_migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boomdevs/go_migrations"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func fixturesRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "fixtures")
}

func TestListMigrations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000001_init.up.sql"), []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	migrations, err := go_migrations.ListMigrations(dir)
	if err != nil {
		t.Fatalf("ListMigrations() error = %v", err)
	}
	if len(migrations) != 1 || migrations[0].BaseName() != "000001_init" {
		t.Fatalf("migrations = %#v", migrations)
	}
}

func TestGenerateUsesDiscoveryByDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	empty := go_migrations.DatabaseSchema{}

	result, err := go_migrations.Generate(context.Background(), go_migrations.Options{
		ModelsRoot:    fixturesRoot(t),
		MigrationsDir: dir,
		CurrentSchema: &empty,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !strings.Contains(result.UpSQL, `CREATE TABLE "plans"`) {
		t.Fatalf("up SQL missing plans table:\n%s", result.UpSQL)
	}
	if result.BaseName != "000001_initial" {
		t.Fatalf("BaseName = %q, want 000001_initial", result.BaseName)
	}
}

func TestDiscoverSchemaPlatformModels(t *testing.T) {
	t.Parallel()

	schema, err := go_migrations.DiscoverSchema(fixturesRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSchema() error = %v", err)
	}

	tableNames := make(map[string]struct{})
	for _, table := range schema.Tables {
		tableNames[table.Name] = struct{}{}
	}

	for _, name := range []string{"plans", "tenants", "tenant_modules"} {
		if _, ok := tableNames[name]; !ok {
			t.Fatalf("missing table %q in %#v", name, schema.Tables)
		}
	}
}

func TestDiscoverSchemaMatchesSchemaFromModels(t *testing.T) {
	t.Parallel()

	discovered, err := go_migrations.DiscoverSchema(fixturesRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSchema() error = %v", err)
	}

	fromModels, err := go_migrations.SchemaFromModels([]any{
		&fixturePlan{},
		&fixtureTenant{},
		&fixtureTenantModule{},
	})
	if err != nil {
		t.Fatalf("SchemaFromModels() error = %v", err)
	}

	if !discovered.Equal(fromModels) {
		t.Fatalf("discovered schema does not match SchemaFromModels\ndiscovered: %#v\nfrom models: %#v", discovered, fromModels)
	}
}

func TestDiscoverSchemaTenantSlugIndex(t *testing.T) {
	t.Parallel()

	schema, err := go_migrations.DiscoverSchema(fixturesRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSchema() error = %v", err)
	}

	for _, table := range schema.Tables {
		if table.Name != "tenants" {
			continue
		}
		for _, idx := range table.Indexes {
			if idx.Name == "idx_tenants_slug" {
				return
			}
		}
		t.Fatalf("tenants indexes = %#v, want idx_tenants_slug", table.Indexes)
	}
	t.Fatal("tenants table not found")
}

func TestDiscoverSchemaTenantModuleForeignKey(t *testing.T) {
	t.Parallel()

	schema, err := go_migrations.DiscoverSchema(fixturesRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSchema() error = %v", err)
	}

	for _, table := range schema.Tables {
		if table.Name != "tenant_modules" {
			continue
		}
		for _, col := range table.Columns {
			if col.Name != "tenant_id" {
				continue
			}
			if col.References == nil {
				t.Fatal("tenant_id missing foreign key reference")
			}
			if col.References.Table != "tenants" || col.References.Column != "id" {
				t.Fatalf("tenant_id reference = %#v, want tenants(id)", col.References)
			}
			if col.References.OnDelete != "CASCADE" {
				t.Fatalf("tenant_id on delete = %q, want CASCADE", col.References.OnDelete)
			}
			return
		}
	}
	t.Fatal("tenant_modules.tenant_id not found")
}

type fixturePlan struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string         `gorm:"type:text;not null"`
	Limits    datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time      `gorm:"type:timestamptz;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"type:timestamptz;autoUpdateTime"`
}

func (fixturePlan) TableName() string { return "plans" }

type fixtureTenant struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string     `gorm:"type:text;not null"`
	Slug      string     `gorm:"type:text;not null;uniqueIndex:idx_tenants_slug"`
	PlanID    *uuid.UUID `gorm:"type:uuid"`
	Status    string     `gorm:"type:text;not null;default:'active'"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:now()"`

	Plan *fixturePlan `gorm:"foreignKey:PlanID;references:ID"`
}

func (fixtureTenant) TableName() string { return "tenants" }

type fixtureTenantModule struct {
	TenantID  uuid.UUID      `gorm:"type:uuid;primaryKey"`
	ModuleKey string         `gorm:"type:text;primaryKey"`
	Enabled   bool           `gorm:"not null;default:false"`
	Config    datatypes.JSON `gorm:"type:jsonb;default:'{}'"`

	Tenant fixtureTenant `gorm:"foreignKey:TenantID;references:ID;constraint:OnDelete:CASCADE"`
}

func (fixtureTenantModule) TableName() string { return "tenant_modules" }
