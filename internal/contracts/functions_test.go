package contracts

import "testing"

func TestValidateFunctionName(t *testing.T) {
	for _, name := range []string{"hello", "stripe-webhook", "x1"} {
		if err := ValidateFunctionName(name); err != nil {
			t.Fatalf("ValidateFunctionName(%q) error = %v", name, err)
		}
	}
	for _, name := range []string{"", "Main", "main", "../escape", "-leading", "trailing-"} {
		if err := ValidateFunctionName(name); err == nil {
			t.Fatalf("ValidateFunctionName(%q) succeeded, want rejection", name)
		}
	}
}
