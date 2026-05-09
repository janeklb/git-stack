package app

import "testing"

func TestNormalizeTemplateScopeDefaultsToRepo(t *testing.T) {
	t.Parallel()

	scope, err := normalizeTemplateScope("")
	if err != nil {
		t.Fatalf("normalizeTemplateScope returned error: %v", err)
	}
	if scope != templateScopeRepo {
		t.Fatalf("expected default repo scope, got %q", scope)
	}
}

func TestNormalizeTemplateScopeRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	if _, err := normalizeTemplateScope("team"); err == nil {
		t.Fatal("expected invalid template scope to fail")
	}
}
