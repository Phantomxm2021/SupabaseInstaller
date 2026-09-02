package authkeys

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateAndValidateBundle(t *testing.T) {
	b, err := Generate(strings.NewReader(strings.Repeat("x", 4096)), "legacy-secret-that-is-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Validate("legacy-secret-that-is-long-enough"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.SupabasePublishableKey, "sb_publishable_") || !strings.HasPrefix(b.SupabaseSecretKey, "sb_secret_") {
		t.Fatalf("opaque prefixes missing")
	}
}

func TestBundleRejectsWrongSecretAndPartial(t *testing.T) {
	b, err := Generate(strings.NewReader(strings.Repeat("y", 4096)), "legacy-secret-that-is-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Validate("wrong-secret-that-is-long-enough"); err == nil {
		t.Fatal("wrong secret accepted")
	}
	b.JWTJWKS = "{}"
	if err := b.Validate("legacy-secret-that-is-long-enough"); err == nil {
		t.Fatal("partial bundle accepted")
	}
}

func TestBundleJWKVisibilityAndTamperRejection(t *testing.T) {
	b, err := Generate(strings.NewReader(strings.Repeat("z", 8192)), "legacy-secret-that-is-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	var private []map[string]interface{}
	var public struct {
		Keys []map[string]interface{} `json:"keys"`
	}
	if err := json.Unmarshal([]byte(b.JWTKeys), &private); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(b.JWTJWKS), &public); err != nil {
		t.Fatal(err)
	}
	if _, ok := private[0]["d"]; !ok {
		t.Fatal("private JWK missing d")
	}
	if _, ok := public.Keys[0]["d"]; ok {
		t.Fatal("public JWKS leaked d")
	}
	private[0]["d"] = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	mut, _ := json.Marshal(private)
	b.JWTKeys = string(mut)
	if err := b.Validate("legacy-secret-that-is-long-enough"); err == nil {
		t.Fatal("tampered private key accepted")
	}
}

func TestPad32LeadingZero(t *testing.T) {
	got := pad32([]byte{1, 2, 3})
	if len(got) != 32 || got[29] != 1 || got[30] != 2 || got[31] != 3 {
		t.Fatalf("pad32 produced incorrect fixed-width value")
	}
}

func TestRoleTokenRejectsLooseClaims(t *testing.T) {
	b, err := Generate(strings.NewReader(strings.Repeat("q", 8192)), "legacy-secret-that-is-long-enough")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(b.AnonKeyAsymmetric, ".")
	// Replacing the signed payload is sufficient: validation must reject the
	// malformed/untyped claims before signature verification.
	parts[1] = "e30"
	b.AnonKeyAsymmetric = strings.Join(parts, ".")
	if err := b.Validate("legacy-secret-that-is-long-enough"); err == nil {
		t.Fatal("loose JWT claims accepted")
	}
}
