package functionlogs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLoadRedactorUsesAllowListedProjectAndAllFunctionSecrets(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, ".env")
	functions := filepath.Join(dir, ".env.functions")
	projectText := "POSTGRES_PASSWORD=db-sentinel\nJWT_SECRET=jwt-sentinel\nSERVICE_ROLE_KEY=role-sentinel\nDASHBOARD_PASSWORD=dash-sentinel\nSMTP_PASS=smtp-sentinel\nAWS_SECRET_ACCESS_KEY=storage-sentinel\nGOOGLE_SECRET=oauth-sentinel\nANON_KEY=anon-sentinel\nPUBLIC_TOKEN_LABEL=public-token-label\nDATABASE_DISPLAY_NAME=database-display\nSECRETARY_NAME=secretary\nMONKEY_API_KEY_HINT=monkey-hint\nPUBLIC_VALUE=must-remain\n"
	if err := os.WriteFile(project, []byte(projectText), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(functions, []byte("WHATEVER_NAME=function-sentinel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	redactor, err := LoadRedactor(project, functions)
	if err != nil {
		t.Fatal(err)
	}
	message, truncated := redactor.SanitizeMessage("Authorization: Bearer auth-sentinel api_key=key-sentinel " + projectText + " WHATEVER_NAME=function-sentinel\x00\n")
	if truncated {
		t.Fatal("unexpected truncation")
	}
	for _, sentinel := range []string{"auth-sentinel", "key-sentinel", "db-sentinel", "jwt-sentinel", "role-sentinel", "dash-sentinel", "smtp-sentinel", "storage-sentinel", "oauth-sentinel", "anon-sentinel", "function-sentinel"} {
		if strings.Contains(message, sentinel) {
			t.Fatalf("secret %q remained in %q", sentinel, message)
		}
	}
	if !strings.Contains(message, "must-remain") {
		t.Fatalf("non-secret project value was redacted: %q", message)
	}
	for _, nonSecret := range []string{"public-token-label", "database-display", "secretary", "monkey-hint"} {
		if !strings.Contains(message, nonSecret) {
			t.Fatalf("unrelated project value %q was loaded as a secret: %q", nonSecret, message)
		}
	}
	if strings.ContainsAny(message, "\x00\n\r") {
		t.Fatalf("control characters remained: %q", message)
	}
}

func TestSanitizeMessageNormalizesInvalidUTF8AndCapsAt10KiB(t *testing.T) {
	redactor, err := LoadRedactor("", "")
	if err != nil {
		t.Fatal(err)
	}
	input := string([]byte{0xff, 'x'}) + strings.Repeat("界", 5000)
	message, truncated := redactor.SanitizeMessage(input)
	if !truncated {
		t.Fatal("expected truncation")
	}
	if len(message) > 10*1024 {
		t.Fatalf("message length = %d", len(message))
	}
	if !utf8.ValidString(message) {
		t.Fatal("message is invalid UTF-8")
	}
	if !strings.ContainsRune(message, utf8.RuneError) {
		t.Fatal("invalid byte was not replaced")
	}
}

func TestSanitizeMessageRedactsCredentialSpanningInternalChunkBoundary(t *testing.T) {
	redactor, err := LoadRedactor("", "")
	if err != nil {
		t.Fatal(err)
	}
	sentinel := "boundary-secret"
	message, _ := redactor.SanitizeMessage(strings.Repeat("x", 3068) + " api_key=" + sentinel)
	if strings.Contains(message, sentinel) {
		t.Fatalf("boundary credential remained in output")
	}
}
