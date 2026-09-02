package contracts

import (
	"encoding/json"
	"testing"
)

func TestProjectSecretsAuthKeyFieldsJSON(t *testing.T) {
	s := ProjectSecrets{SupabasePublishableKey: "pub", SupabaseSecretKey: "sec", AnonKeyAsymmetric: "anon", ServiceRoleKeyAsymmetric: "service", JWTKeys: "keys", JWTJWKS: "jwks"}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"supabasePublishableKey", "supabaseSecretKey", "anonKeyAsymmetric", "serviceRoleKeyAsymmetric", "jwtKeys", "jwtJwks"} {
		if _, ok := got[k]; ok {
			t.Errorf("wire-unsafe auth field %s exposed", k)
		}
	}
}
