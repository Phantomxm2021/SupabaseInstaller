package project

import (
	"errors"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestDefaultConfiguration(t *testing.T) {
	got := DefaultConfiguration(contracts.PresetLightweight)
	if !got.Services.Database || !got.Services.Auth || got.Services.Storage || got.Auth.SMTP.Enabled {
		t.Fatalf("unexpected Lightweight defaults: %#v", got)
	}
	if !got.Auth.Email.Enabled || got.Auth.Phone.Enabled || got.Auth.AnonymousSignIn {
		t.Fatalf("unexpected Auth defaults: %#v", got.Auth)
	}
}

func TestValidateConfigurationDependencies(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contracts.ProjectConfiguration)
		field  string
	}{
		{"database mandatory", func(c *contracts.ProjectConfiguration) { c.Services.Database = false }, "services.database"},
		{"studio requires meta", func(c *contracts.ProjectConfiguration) { c.Services.PostgresMeta = false }, "services.postgresMeta"},
		{"imgproxy requires storage", func(c *contracts.ProjectConfiguration) { c.Services.Imgproxy = true }, "services.imgproxy"},
		{"vector follows logs", func(c *contracts.ProjectConfiguration) { c.Services.Vector = true }, "services.vector"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			tc.mutate(&cfg)
			err := ValidateConfiguration(cfg)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
				t.Fatalf("expected field error for %s, got %v", tc.field, err)
			}
		})
	}
}
