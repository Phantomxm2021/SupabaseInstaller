package site

import (
	"strings"
	"testing"
)

func TestRenderApplyUsesStableSlugFileAndCurrentDomain(t *testing.T) {
	renderer := NewRenderer(TLSPaths{
		CertificateFile:    "/etc/nginx/ssl/cloudflare-origin.pem",
		CertificateKeyFile: "/etc/nginx/ssl/cloudflare-origin.key",
	})

	rendered, err := renderer.RenderApply(ApplyRequest{
		Slug:           "bee-game",
		Domain:         "bee.example.com",
		APIPort:        18001,
		StudioPort:     18002,
		StudioEnabled:  true,
		StudioUsername: "operator",
		StudioPassword: "operator-password",
	})
	if err != nil {
		t.Fatalf("RenderApply() error = %v", err)
	}

	if got, want := rendered.AvailableName, "supabase-manager-bee-game.conf"; got != want {
		t.Fatalf("AvailableName = %q, want %q", got, want)
	}

	for _, fragment := range []string{
		"server_name bee.example.com;",
		"proxy_pass http://127.0.0.1:18001;",
		"proxy_pass http://127.0.0.1:18002;",
		"ssl_certificate /etc/nginx/ssl/cloudflare-origin.pem;",
		"ssl_certificate_key /etc/nginx/ssl/cloudflare-origin.key;",
		"location /realtime/v1/",
	} {
		if !strings.Contains(rendered.Contents, fragment) {
			t.Errorf("rendered config does not contain %q:\n%s", fragment, rendered.Contents)
		}
	}
}

func TestRenderApplyProtectsOnlyStudioRoot(t *testing.T) {
	renderer := NewRenderer(TLSPaths{
		CertificateFile:    "/etc/nginx/ssl/cloudflare-origin.pem",
		CertificateKeyFile: "/etc/nginx/ssl/cloudflare-origin.key",
	})

	rendered, err := renderer.RenderApply(ApplyRequest{
		Slug: "studio", Domain: "studio.example.com", APIPort: 18001,
		StudioPort: 18002, StudioEnabled: true, StudioUsername: "operator", StudioPassword: "operator-password",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`auth_basic "Supabase Studio";`,
		"auth_basic_user_file /etc/supabase-manager/nginx-auth/supabase-manager-studio.htpasswd;",
		`proxy_set_header Authorization "";`,
		"proxy_pass http://127.0.0.1:18002;",
	} {
		if !strings.Contains(rendered.Contents, want) {
			t.Fatalf("rendered Studio root is missing %q:\n%s", want, rendered.Contents)
		}
	}
	for _, apiRoute := range []string{"location /auth", "location /rest", "location /storage/v1/", "location /realtime/v1/"} {
		begin := strings.Index(rendered.Contents, apiRoute)
		if begin < 0 {
			t.Fatalf("missing API route %q", apiRoute)
		}
		end := strings.Index(rendered.Contents[begin:], "}\n")
		if end < 0 {
			t.Fatalf("cannot find end of API route %q", apiRoute)
		}
		if strings.Contains(rendered.Contents[begin:begin+end], "auth_basic") {
			t.Fatalf("API route %q must not require Studio credentials", apiRoute)
		}
	}
}

func TestRenderApplyRejectsUnsafeInputs(t *testing.T) {
	renderer := NewRenderer(TLSPaths{
		CertificateFile:    "/etc/nginx/ssl/cloudflare-origin.pem",
		CertificateKeyFile: "/etc/nginx/ssl/cloudflare-origin.key",
	})

	for name, request := range map[string]ApplyRequest{
		"path traversal slug": {
			Slug: "../etc", Domain: "bee.example.com", APIPort: 18001,
		},
		"nginx injection domain": {
			Slug: "bee", Domain: "bee.example.com; include /tmp/pwn;", APIPort: 18001,
		},
		"invalid api port": {
			Slug: "bee", Domain: "bee.example.com", APIPort: 0,
		},
		"enabled studio without port": {
			Slug: "bee", Domain: "bee.example.com", APIPort: 18001, StudioEnabled: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := renderer.RenderApply(request); err == nil {
				t.Fatal("RenderApply() error = nil, want validation error")
			}
		})
	}
}

func TestValidationUsesServerTerminology(t *testing.T) {
	renderer := NewRenderer(TLSPaths{
		CertificateFile:    "/etc/nginx/ssl/cloudflare-origin.pem",
		CertificateKeyFile: "/etc/nginx/ssl/cloudflare-origin.key",
	})
	if _, err := renderer.RenderApply(ApplyRequest{Slug: "bee", Domain: "invalid domain", APIPort: 18001}); err == nil || !strings.Contains(err.Error(), "invalid server domain") {
		t.Fatalf("RenderApply() error = %v, want server terminology", err)
	}
	for _, name := range []func(string) (string, error){ManagedSiteName, ManagedCredentialName} {
		if _, err := name("../escape"); err == nil || !strings.Contains(err.Error(), "invalid server slug") {
			t.Fatalf("managed name error = %v, want server terminology", err)
		}
	}
}
