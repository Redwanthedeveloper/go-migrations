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
	name, unique, _, ok := indexFromSettings(settings)
	if !ok || name != "idx_tenants_slug" || !unique {
		t.Fatalf("index = %q (unique=%v, ok=%v), settings = %#v", name, unique, ok, settings)
	}
}

func TestIndexFromSettingsComposite(t *testing.T) {
	t.Parallel()

	provider := parseGormTag(`gorm:"type:text;not null;uniqueIndex:idx_oauth_accounts_provider_user"`)
	providerUser := parseGormTag(`gorm:"type:text;not null;uniqueIndex:idx_oauth_accounts_provider_user,priority:2"`)

	name1, unique1, prio1, ok1 := indexFromSettings(provider)
	if !ok1 || name1 != "idx_oauth_accounts_provider_user" || !unique1 || prio1 != 0 {
		t.Fatalf("provider index = %q (unique=%v, prio=%d, ok=%v)", name1, unique1, prio1, ok1)
	}
	name2, _, prio2, ok2 := indexFromSettings(providerUser)
	if !ok2 || name2 != "idx_oauth_accounts_provider_user" || prio2 != 2 {
		t.Fatalf("provider_user index = %q (prio=%d, ok=%v)", name2, prio2, ok2)
	}
}
