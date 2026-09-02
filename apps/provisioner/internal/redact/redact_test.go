package redact

import (
	"strings"
	"testing"
)

func TestRedactorRemovesKnownValuesAndCredentialAssignments(t *testing.T) {
	redactor := New([]string{"actual-secret"})
	got := redactor.String("Authorization: Bearer actual-secret POSTGRES_PASSWORD=hunter2 apikey: abc123")
	for _, forbidden := range []string{"actual-secret", "hunter2", "abc123"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted log contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redacted log = %q, want marker", got)
	}
}

func TestRedactorStillRedactsExistingSupabaseSecretAssignments(t *testing.T) {
	got := New(nil).String("SUPABASE_SECRET_KEY=secret-value")
	if strings.Contains(got, "secret-value") {
		t.Fatalf("redacted log contains a Supabase secret: %s", got)
	}
}
