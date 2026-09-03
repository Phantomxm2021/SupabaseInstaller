// Package authkeys creates the opt-in asymmetric API key material used by the
// self-hosted Supabase stack.
package authkeys

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
)

const opaqueProjectRef = "supabase-self-hosted"

// Bundle is an all-or-nothing set of opt-in API keys and signing keys.
type Bundle struct {
	SupabasePublishableKey   string `json:"supabasePublishableKey"`
	SupabaseSecretKey        string `json:"supabaseSecretKey"`
	AnonKeyAsymmetric        string `json:"anonKeyAsymmetric"`
	ServiceRoleKeyAsymmetric string `json:"serviceRoleKeyAsymmetric"`
	JWTKeys                  string `json:"jwtKeys"`
	JWTJWKS                  string `json:"jwtJwks"`
}

type jwk struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid,omitempty"`
	Use string   `json:"use,omitempty"`
	Ops []string `json:"key_ops,omitempty"`
	Alg string   `json:"alg,omitempty"`
	Ext bool     `json:"ext,omitempty"`
	Crv string   `json:"crv,omitempty"`
	X   string   `json:"x,omitempty"`
	Y   string   `json:"y,omitempty"`
	D   string   `json:"d,omitempty"`
	K   string   `json:"k,omitempty"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}
type jwtClaims struct {
	Role string `json:"role"`
	Iss  string `json:"iss"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

var b64 = base64.RawURLEncoding

func Generate(r io.Reader, legacyJWTSecret string) (Bundle, error) {
	var out Bundle
	if r == nil {
		return out, errors.New("authkeys: nil random reader")
	}
	if len(legacyJWTSecret) < 32 {
		return out, errors.New("authkeys: legacy JWT secret must be at least 32 bytes")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), r)
	if err != nil {
		return out, fmt.Errorf("authkeys: generate P-256 key: %w", err)
	}
	rawKid := make([]byte, 16)
	if _, err := io.ReadFull(r, rawKid); err != nil {
		return out, err
	}
	kid := b64.EncodeToString(rawKid)
	priv := jwk{Kty: "EC", Kid: kid, Use: "sig", Ops: []string{"sign", "verify"}, Alg: "ES256", Ext: true, Crv: "P-256", X: b64.EncodeToString(pad32(key.PublicKey.X.Bytes())), Y: b64.EncodeToString(pad32(key.PublicKey.Y.Bytes())), D: b64.EncodeToString(pad32(key.D.Bytes()))}
	pub := priv
	pub.Ops = []string{"verify"}
	pub.D = ""
	oct := jwk{Kty: "oct", K: b64.EncodeToString([]byte(legacyJWTSecret)), Alg: "HS256"}
	privateJSON, _ := json.Marshal([]jwk{priv, oct})
	publicJSON, _ := json.Marshal(jwks{Keys: []jwk{pub, oct}})
	out.JWTKeys, out.JWTJWKS = string(privateJSON), string(publicJSON)
	if out.SupabasePublishableKey, err = opaqueKey(r, "sb_publishable_"); err != nil {
		return Bundle{}, err
	}
	if out.SupabaseSecretKey, err = opaqueKey(r, "sb_secret_"); err != nil {
		return Bundle{}, err
	}
	if out.AnonKeyAsymmetric, err = signRole(key, kid, "anon", r); err != nil {
		return Bundle{}, err
	}
	if out.ServiceRoleKeyAsymmetric, err = signRole(key, kid, "service_role", r); err != nil {
		return Bundle{}, err
	}
	return out, out.Validate(legacyJWTSecret)
}

func opaqueKey(r io.Reader, prefix string) (string, error) {
	b := make([]byte, 17)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	random := b64.EncodeToString(b)[:22]
	intermediate := prefix + random
	h := sha256.Sum256([]byte(opaqueProjectRef + "|" + intermediate))
	return intermediate + "_" + b64.EncodeToString(h[:])[:8], nil
}

func signRole(key *ecdsa.PrivateKey, kid, role string, r io.Reader) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "typ": "JWT", "kid": kid})
	now := time.Now().Unix()
	payload, _ := json.Marshal(map[string]interface{}{"role": role, "iss": "supabase", "iat": now, "exp": now + 5*365*24*3600})
	input := b64.EncodeToString(header) + "." + b64.EncodeToString(payload)
	h := sha256.Sum256([]byte(input))
	sig, err := ecdsa.SignASN1(r, key, h[:])
	if err != nil {
		return "", err
	}
	// ES256 JWT signatures are the fixed-width IEEE-P1363 (r||s) form.
	raw := make([]byte, 64)
	var parsed struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &parsed); err != nil || parsed.R == nil || parsed.S == nil {
		return "", errors.New("authkeys: invalid ECDSA signature")
	}
	rv, sv := parsed.R.Bytes(), parsed.S.Bytes()
	copy(raw[32-len(rv):32], rv)
	copy(raw[64-len(sv):], sv)
	return input + "." + b64.EncodeToString(raw), nil
}

func ValidateBundle(b Bundle, legacyJWTSecret string) error { return b.Validate(legacyJWTSecret) }
func (b Bundle) Validate(legacyJWTSecret string) error {
	if len(legacyJWTSecret) < 32 {
		return errors.New("authkeys: invalid legacy JWT secret")
	}
	if !validOpaque(b.SupabasePublishableKey, "sb_publishable_") || !validOpaque(b.SupabaseSecretKey, "sb_secret_") {
		return errors.New("authkeys: invalid opaque key")
	}
	priv, err := parsePrivateKeys([]byte(b.JWTKeys))
	pub, err2 := parsePublicKeys([]byte(b.JWTJWKS))
	if err != nil || err2 != nil || len(priv) != 2 || len(pub.Keys) != 2 {
		return errors.New("authkeys: invalid JWK sets")
	}
	if priv[0].Kty != "EC" || priv[0].D == "" || pub.Keys[0].Kty != "EC" || pub.Keys[0].D != "" || priv[0].Kid == "" || priv[0].Kid != pub.Keys[0].Kid || priv[0].X != pub.Keys[0].X || priv[0].Y != pub.Keys[0].Y {
		return errors.New("authkeys: invalid EC key visibility")
	}
	if priv[0].Alg != "ES256" || pub.Keys[0].Alg != "ES256" || priv[0].Crv != "P-256" || pub.Keys[0].Crv != "P-256" || priv[0].Use != "sig" || pub.Keys[0].Use != "sig" {
		return errors.New("authkeys: invalid EC key parameters")
	}
	d, dx := b64.DecodeString(priv[0].D)
	x, xx := b64.DecodeString(priv[0].X)
	y, xy := b64.DecodeString(priv[0].Y)
	if dx != nil || xx != nil || xy != nil || len(d) != 32 || len(x) != 32 || len(y) != 32 {
		return errors.New("authkeys: malformed EC key material")
	}
	derivedX, derivedY := elliptic.P256().ScalarBaseMult(d)
	if derivedX == nil || derivedY == nil || derivedX.Cmp(new(big.Int).SetBytes(x)) != 0 || derivedY.Cmp(new(big.Int).SetBytes(y)) != 0 {
		return errors.New("authkeys: private/public EC key mismatch")
	}
	if new(big.Int).SetBytes(d).Sign() <= 0 || new(big.Int).SetBytes(d).Cmp(elliptic.P256().Params().N) >= 0 || !elliptic.P256().IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {
		return errors.New("authkeys: invalid EC scalar or point")
	}
	if priv[1].Kty != "oct" || pub.Keys[1].Kty != "oct" || subtle.ConstantTimeCompare([]byte(priv[1].K), []byte(b64.EncodeToString([]byte(legacyJWTSecret)))) != 1 || priv[1].K != pub.Keys[1].K {
		return errors.New("authkeys: legacy key mismatch")
	}
	if err := verifyRole(b.AnonKeyAsymmetric, pub.Keys[0], "anon"); err != nil {
		return err
	}
	return verifyRole(b.ServiceRoleKeyAsymmetric, pub.Keys[0], "service_role")
}

func validOpaque(v, prefix string) bool {
	if !strings.HasPrefix(v, prefix) {
		return false
	}
	x := strings.TrimPrefix(v, prefix)
	sep := 22
	if len(x) != 31 || x[sep] != '_' {
		return false
	}
	h := sha256.Sum256([]byte(opaqueProjectRef + "|" + prefix + x[:sep]))
	return subtle.ConstantTimeCompare([]byte(x[sep+1:]), []byte(b64.EncodeToString(h[:])[:8])) == 1
}

func pad32(v []byte) []byte {
	out := make([]byte, 32)
	if len(v) > 32 {
		return v[len(v)-32:]
	}
	copy(out[32-len(v):], v)
	return out
}
func parsePrivateKeys(raw []byte) ([]jwk, error) {
	if err := checkFieldSets(raw, true); err != nil {
		return nil, err
	}
	var k []jwk
	if err := strictJSON(raw, &k); err != nil {
		return nil, err
	}
	if len(k) != 2 || !canonicalEC(k[0], true) || !canonicalOct(k[1]) {
		return nil, errors.New("authkeys: noncanonical private JWK set")
	}
	return k, nil
}
func parsePublicKeys(raw []byte) (jwks, error) {
	if err := checkFieldSets(raw, false); err != nil {
		return jwks{}, err
	}
	var k jwks
	if err := strictJSON(raw, &k); err != nil {
		return k, err
	}
	if len(k.Keys) != 2 || !canonicalEC(k.Keys[0], false) || !canonicalOct(k.Keys[1]) {
		return k, errors.New("authkeys: noncanonical public JWK set")
	}
	return k, nil
}
func checkFieldSets(raw []byte, private bool) error {
	var vals []json.RawMessage
	if private {
		if err := json.Unmarshal(raw, &vals); err != nil {
			return err
		}
	} else {
		var o struct {
			Keys []json.RawMessage `json:"keys"`
		}
		if err := json.Unmarshal(raw, &o); err != nil {
			return err
		}
		vals = o.Keys
	}
	if len(vals) != 2 {
		return errors.New("wrong key count")
	}
	sets := []map[string]bool{{"kty": true, "kid": true, "use": true, "key_ops": true, "alg": true, "ext": true, "crv": true, "x": true, "y": true}, {"kty": true, "k": true, "alg": true}}
	if !private {
		sets[0] = map[string]bool{"kty": true, "kid": true, "use": true, "key_ops": true, "alg": true, "ext": true, "crv": true, "x": true, "y": true}
	}
	for i := range vals {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(vals[i], &m); err != nil {
			return err
		}
		expected := sets[i]
		if i == 0 && private {
			expected["d"] = true
		}
		if len(m) != len(expected) {
			return errors.New("noncanonical fields")
		}
		for k := range expected {
			if _, ok := m[k]; !ok {
				return errors.New("missing canonical field")
			}
		}
	}
	return nil
}
func strictJSON(raw []byte, dst interface{}) error {
	dup, err := duplicateJSONKey(raw)
	if err != nil || dup {
		return errors.New("authkeys: duplicate or malformed JSON fields")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("authkeys: trailing JSON")
	}
	return nil
}

func duplicateJSONKey(raw []byte) (bool, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dup, err := scanJSONValue(dec)
	if err != nil {
		return false, err
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return false, errors.New("trailing JSON")
	}
	return dup, nil
}
func scanJSONValue(dec *json.Decoder) (bool, error) {
	tok, err := dec.Token()
	if err != nil {
		return false, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return false, nil
	}
	if d == '[' {
		for dec.More() {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return false, err
			}
			dup, err := duplicateJSONKey(raw)
			if err != nil {
				return false, err
			}
			if dup {
				return true, nil
			}
		}
		_, err = dec.Token()
		return false, err
	}
	if d != '{' {
		return false, errors.New("invalid JSON delimiter")
	}
	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return false, err
		}
		ks, ok := key.(string)
		if !ok {
			return false, errors.New("invalid object key")
		}
		if seen[ks] {
			return true, nil
		}
		seen[ks] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return false, err
		}
		dup, err := duplicateJSONKey(raw)
		if err != nil {
			return false, err
		}
		if dup {
			return true, nil
		}
	}
	_, err = dec.Token()
	return false, err
}
func canonicalEC(k jwk, private bool) bool {
	opsOK := (private && len(k.Ops) == 2 && k.Ops[0] == "sign" && k.Ops[1] == "verify") || (!private && len(k.Ops) == 1 && k.Ops[0] == "verify")
	return k.Kty == "EC" && k.Kid != "" && k.Use == "sig" && k.Alg == "ES256" && k.Ext && k.Crv == "P-256" && opsOK && k.X != "" && k.Y != "" && (private == (k.D != ""))
}
func canonicalOct(k jwk) bool {
	return k.Kty == "oct" && k.Alg == "HS256" && k.K != "" && k.Kid == "" && k.Use == "" && len(k.Ops) == 0 && !k.Ext && k.Crv == "" && k.X == "" && k.Y == "" && k.D == ""
}

func verifyRole(token string, key jwk, role string) error {
	p := strings.Split(token, ".")
	if len(p) != 3 {
		return errors.New("authkeys: malformed JWT")
	}
	var h jwtHeader
	var c jwtClaims
	hb, err := b64.DecodeString(p[0])
	if err != nil {
		return errors.New("authkeys: malformed JWT header")
	}
	cb, err := b64.DecodeString(p[1])
	if err != nil {
		return errors.New("authkeys: malformed JWT claims")
	}
	if strictJSON(hb, &h) != nil || strictJSON(cb, &c) != nil || !exactFields(hb, "alg", "typ", "kid") || !exactFields(cb, "role", "iss", "iat", "exp") {
		return errors.New("authkeys: malformed JWT JSON")
	}
	if h.Alg != "ES256" || h.Typ != "JWT" || h.Kid != key.Kid || c.Role != role || c.Iss != "supabase" || c.Iat <= 0 || c.Exp <= c.Iat {
		return errors.New("authkeys: JWT claims mismatch")
	}
	sig, err := b64.DecodeString(p[2])
	if err != nil || len(sig) != 64 {
		return errors.New("authkeys: malformed JWT signature")
	}
	x, err := b64.DecodeString(key.X)
	if err != nil {
		return errors.New("authkeys: malformed public key")
	}
	y, err := b64.DecodeString(key.Y)
	if err != nil {
		return errors.New("authkeys: malformed public key")
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	px := new(big.Int).SetBytes(x)
	py := new(big.Int).SetBytes(y)
	hh := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if !ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: px, Y: py}, hh[:], r, s) {
		return errors.New("authkeys: invalid JWT signature")
	}
	return nil
}

func exactFields(raw []byte, expected ...string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || len(m) != len(expected) {
		return false
	}
	for _, k := range expected {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}
