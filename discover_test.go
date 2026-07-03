package go_migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Redwanthedeveloper/go-migrations"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func fixturesRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "fixtures")
}

func TestLoadRevisions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "-- revision: abc123\n-- down_revision:\n-- create_date: 2026-07-03T12:00:00Z\n-- message: init\n\n" +
		"-- migrate:up\nCREATE TABLE t (id int);\n\n-- migrate:down\nDROP TABLE t;\n"
	if err := os.WriteFile(filepath.Join(dir, "abc123_init.sql"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	revisions, err := go_migrations.LoadRevisions(dir)
	if err != nil {
		t.Fatalf("LoadRevisions() error = %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("revisions = %#v", revisions)
	}
	rev := revisions[0]
	if rev.ID != "abc123" || rev.DownRevision != "" || rev.Message != "init" {
		t.Fatalf("rev header = %#v", rev)
	}
	if rev.UpSQL != "CREATE TABLE t (id int);" {
		t.Fatalf("up SQL = %q", rev.UpSQL)
	}
	if rev.DownSQL != "DROP TABLE t;" {
		t.Fatalf("down SQL = %q", rev.DownSQL)
	}
}

func TestOrderRevisionsFollowsDownRevision(t *testing.T) {
	t.Parallel()

	revisions := []go_migrations.Revision{
		{ID: "c", DownRevision: "b"},
		{ID: "a", DownRevision: ""},
		{ID: "b", DownRevision: "a"},
	}
	ordered, err := go_migrations.OrderRevisions(revisions)
	if err != nil {
		t.Fatalf("OrderRevisions() error = %v", err)
	}
	got := []string{ordered[0].ID, ordered[1].ID, ordered[2].ID}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("order = %v, want [a b c]", got)
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
	if result.Message != "initial" {
		t.Fatalf("Message = %q, want initial", result.Message)
	}
	if result.Revision == "" || result.DownRevision != "" {
		t.Fatalf("first revision = %q, down_revision = %q", result.Revision, result.DownRevision)
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
		&fixtureOAuthAccount{},
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

type fixtureOAuthAccount struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index:idx_oauth_accounts_user_id"`
	Provider       string    `gorm:"type:text;not null;uniqueIndex:idx_oauth_accounts_provider_user"`
	ProviderUserID string    `gorm:"type:text;not null;uniqueIndex:idx_oauth_accounts_provider_user"`
	CreatedAt      time.Time `gorm:"type:timestamptz;not null;default:now()"`
}

func (fixtureOAuthAccount) TableName() string { return "oauth_accounts" }

func TestDiscoverSchemaCompositeUniqueIndex(t *testing.T) {
	t.Parallel()

	schema, err := go_migrations.DiscoverSchema(fixturesRoot(t))
	if err != nil {
		t.Fatalf("DiscoverSchema() error = %v", err)
	}

	for _, table := range schema.Tables {
		if table.Name != "oauth_accounts" {
			continue
		}
		for _, idx := range table.Indexes {
			if idx.Name != "idx_oauth_accounts_provider_user" {
				continue
			}
			if !idx.Unique {
				t.Fatalf("index %q should be unique", idx.Name)
			}
			// Both columns must live under one index, ordered by struct position.
			want := []string{"provider", "provider_user_id"}
			if len(idx.Columns) != len(want) {
				t.Fatalf("index columns = %v, want %v", idx.Columns, want)
			}
			for i, col := range want {
				if idx.Columns[i] != col {
					t.Fatalf("index columns = %v, want %v", idx.Columns, want)
				}
			}
			return
		}
		t.Fatalf("oauth_accounts indexes = %#v, want composite idx_oauth_accounts_provider_user", table.Indexes)
	}
	t.Fatal("oauth_accounts table not found")
}
