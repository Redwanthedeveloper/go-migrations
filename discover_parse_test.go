package go_migrations

import "testing"

func TestParseGormTagLimitsField(t *testing.T) {
	t.Parallel()

	settings := parseGormTag(`gorm:"type:jsonb;not null;default:'{}'"`)
	if len(settings) == 0 {
		t.Fatal("settings is empty")
	}
	if !settingEnabled(settings, "NOT NULL", "NOTNULL") {
		t.Fatalf("NOT NULL not enabled: %#v", settings)
	}
	if settings["DEFAULT"] != "'{}'" {
		t.Fatalf("DEFAULT = %q, want '{}'", settings["DEFAULT"])
	}
}

func TestParseGormTagSlugField(t *testing.T) {
	t.Parallel()

	settings := parseGormTag(`gorm:"type:text;not null;uniqueIndex:idx_tenants_slug"`)
	if len(settings) == 0 {
		t.Fatal("settings is empty")
	}
	if indexNameFromSettings(settings) != "idx_tenants_slug" {
		t.Fatalf("index = %q, settings = %#v", indexNameFromSettings(settings), settings)
	}
}
