// Package site renders the fixed, per-project Nginx virtual-host template.
// It deliberately accepts typed values only; callers never supply raw Nginx.
package site

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ApplyRequest is the complete, typed routing state for one project.
type ApplyRequest struct {
	Slug           string `json:"slug"`
	Domain         string `json:"domain"`
	APIPort        int    `json:"apiPort"`
	StudioPort     int    `json:"studioPort"`
	StudioEnabled  bool   `json:"studioEnabled"`
	StudioUsername string `json:"studioUsername"`
	StudioPassword string `json:"studioPassword"`
}

// TLSPaths is installed by the host operator and is not project-controlled.
type TLSPaths struct {
	CertificateFile    string
	CertificateKeyFile string
	AuthDirectory      string
}

// RenderedSite is a validated Nginx site ready for transactional activation.
type RenderedSite struct {
	AvailableName string
	Contents      string
	AuthDirectory string
	AuthFileName  string
	AuthContents  string
}

// Renderer renders only the fixed proxy configuration used by the manager.
type Renderer struct {
	tls TLSPaths
}

func NewRenderer(tls TLSPaths) Renderer {
	if strings.TrimSpace(tls.AuthDirectory) == "" {
		tls.AuthDirectory = defaultAuthDirectory
	}
	return Renderer{tls: tls}
}

func (r Renderer) RenderApply(request ApplyRequest) (RenderedSite, error) {
	if err := validateRequest(request); err != nil {
		return RenderedSite{}, err
	}
	if err := validateTLSPaths(r.tls); err != nil {
		return RenderedSite{}, err
	}

	authFileName, err := ManagedCredentialName(request.Slug)
	if err != nil {
		return RenderedSite{}, err
	}
	studioLocation := "location / { return 404; }"
	authContents := ""
	if request.StudioEnabled {
		authContents, err = renderHTPasswd(request.StudioUsername, request.StudioPassword)
		if err != nil {
			return RenderedSite{}, err
		}
		studioLocation = fmt.Sprintf("location / {\n        auth_basic \"Supabase Studio\";\n        auth_basic_user_file %s;\n        proxy_pass http://127.0.0.1:%d;\n    }", filepath.Join(r.tls.AuthDirectory, authFileName), request.StudioPort)
	}

	availableName, err := ManagedSiteName(request.Slug)
	if err != nil {
		return RenderedSite{}, err
	}
	return RenderedSite{
		AvailableName: availableName,
		AuthDirectory: r.tls.AuthDirectory,
		AuthFileName:  authFileName,
		AuthContents:  authContents,
		Contents: fmt.Sprintf(`server {
    listen 80;
    listen [::]:80;
    server_name %s;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    server_name %s;

    ssl_certificate %s;
    ssl_certificate_key %s;

    client_max_body_size 100M;
    server_tokens off;
    proxy_http_version 1.1;

    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
    proxy_set_header X-Forwarded-Port $server_port;

    %s

    location /auth { proxy_pass http://127.0.0.1:%d; }
    location /rest { proxy_pass http://127.0.0.1:%d; }
    location /graphql { proxy_pass http://127.0.0.1:%d; }
    location /storage/v1/ {
        proxy_pass http://127.0.0.1:%d;
        proxy_buffering off;
        proxy_request_buffering off;
    }
    location /functions { proxy_pass http://127.0.0.1:%d; }
    location /mcp { proxy_pass http://127.0.0.1:%d; }
    location /sso { proxy_pass http://127.0.0.1:%d; }
    location /realtime/v1/ {
        proxy_pass http://127.0.0.1:%d;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
`, request.Domain, request.Domain, r.tls.CertificateFile, r.tls.CertificateKeyFile,
			studioLocation,
			request.APIPort, request.APIPort, request.APIPort, request.APIPort,
			request.APIPort, request.APIPort, request.APIPort, request.APIPort),
	}, nil
}

func validateRequest(request ApplyRequest) error {
	if _, err := ManagedSiteName(request.Slug); err != nil {
		return err
	}
	if !validHostname(request.Domain) {
		return fmt.Errorf("invalid project domain")
	}
	if !validPort(request.APIPort) {
		return fmt.Errorf("invalid API port")
	}
	if request.StudioEnabled && !validPort(request.StudioPort) {
		return fmt.Errorf("invalid Studio port")
	}
	return nil
}

// ManagedSiteName is the sole mapping from a project slug to a host config.
// The stable file name is intentional: changing a project domain replaces this
// exact file instead of leaving stale virtual hosts behind.
func ManagedSiteName(slug string) (string, error) {
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid project slug")
	}
	return "supabase-manager-" + slug + ".conf", nil
}

// ManagedCredentialName maps a project slug to the separate root-owned
// htpasswd file used only by the Studio location of that same project.
func ManagedCredentialName(slug string) (string, error) {
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid project slug")
	}
	return "supabase-manager-" + slug + ".htpasswd", nil
}

func validateTLSPaths(paths TLSPaths) error {
	if !safeAbsolutePath(paths.CertificateFile) || !safeAbsolutePath(paths.CertificateKeyFile) || !safeAbsolutePath(paths.AuthDirectory) {
		return fmt.Errorf("invalid TLS certificate paths")
	}
	return nil
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func safeAbsolutePath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "\r\n;")
}

func validHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 || strings.ContainsAny(host, "\t\r\n /\\:;{}$") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
}
