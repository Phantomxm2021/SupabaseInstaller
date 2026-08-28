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
		Slug:          "bee-game",
		Domain:        "bee.example.com",
		APIPort:       18001,
		StudioPort:    18002,
		StudioEnabled: true,
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
