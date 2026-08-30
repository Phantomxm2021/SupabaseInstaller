package contracts

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const managedTLSDirectory = "/etc/nginx/ssl"

var managedTLSNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ManagedTLSConfig contains only safe, persistent routing metadata. Certificate
// and private-key PEM contents never belong in project configuration.
type ManagedTLSConfig struct {
	CertificateName string `json:"certificateName"`
	CertificateFile string `json:"certificateFile"`
	PrivateKeyFile  string `json:"privateKeyFile"`
}

// ManagedTLSPaths derives the only certificate paths that a project may use.
// The configured Site URL is the base domain, so its first label identifies the
// certificate scope: https://beegame.studio becomes cloudflare-origin-beegame.
func ManagedTLSPaths(certificateName, siteURL string) (ManagedTLSConfig, error) {
	name := strings.TrimSpace(strings.ToLower(certificateName))
	if !managedTLSNamePattern.MatchString(name) {
		return ManagedTLSConfig{}, fmt.Errorf("certificate name must contain lowercase letters, digits, and hyphens only")
	}
	parsed, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" {
		return ManagedTLSConfig{}, fmt.Errorf("site URL must be an http or https base domain")
	}
	hostname := strings.ToLower(parsed.Hostname())
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 || labels[0] == "" {
		return ManagedTLSConfig{}, fmt.Errorf("site URL must include a base domain")
	}
	label := labels[0]
	if !managedTLSNamePattern.MatchString(label) {
		return ManagedTLSConfig{}, fmt.Errorf("site URL has an invalid base domain label")
	}
	base := name + "-" + label
	return ManagedTLSConfig{
		CertificateName: name,
		CertificateFile: managedTLSDirectory + "/" + base + ".pem",
		PrivateKeyFile:  managedTLSDirectory + "/" + base + ".key",
	}, nil
}
