package site

import "testing"

func TestAPR1CryptMatchesOpenSSLReference(t *testing.T) {
	if got, want := apr1Crypt("password", "salt"), "$apr1$salt$Xxd1irWT9ycqoYxGFn4cb."; got != want {
		t.Fatalf("apr1Crypt() = %q, want %q", got, want)
	}
}

func TestRenderHTPasswdRejectsUnsafeUsernames(t *testing.T) {
	if _, err := renderHTPasswd("operator\nadmin", "password"); err == nil {
		t.Fatal("renderHTPasswd() error = nil, want unsafe username rejection")
	}
}
