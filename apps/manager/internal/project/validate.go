package project

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"supabase-manager/internal/contracts"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func ValidateDraft(draft Draft) error {
	var errs []error
	cfg := draft.Configuration
	if name := strings.TrimSpace(draft.Name); name == "" || len(name) > 80 {
		errs = append(errs, FieldError{Field: "name", Message: "must contain 1 to 80 characters"})
	}
	if !slugPattern.MatchString(draft.Slug) {
		errs = append(errs, FieldError{Field: "slug", Message: "must match [a-z0-9-] and be a valid DNS label"})
	}
	if err := NormalizeProjectAddress(draft.Slug, &cfg.General); err != nil {
		errs = append(errs, err)
	}
	if cfg.General.SupabaseVersion == "" || strings.EqualFold(cfg.General.SupabaseVersion, "latest") || strings.EqualFold(cfg.General.SupabaseVersion, "master") {
		errs = append(errs, FieldError{Field: "configuration.general.supabaseVersion", Message: "must be a pinned supported version"})
	}
	if errConfiguration := ValidateConfiguration(cfg); errConfiguration != nil {
		errs = append(errs, errConfiguration)
	}
	return errors.Join(errs...)
}

// NormalizeProjectAddress derives the project hostname from the immutable
// project slug and the administrator-supplied base hostname. Domain is a
// server-owned projection: callers must never be able to select it directly.
func NormalizeProjectAddress(slug string, general *contracts.GeneralConfig) error {
	if !slugPattern.MatchString(slug) {
		return FieldError{Field: "slug", Message: "must match [a-z0-9-] and be a valid DNS label"}
	}
	if general == nil {
		return FieldError{Field: "configuration.general.siteUrl", Message: "is required"}
	}
	baseURL, err := canonicalBaseSiteURL(general.SiteURL)
	if err != nil {
		return FieldError{Field: "configuration.general.siteUrl", Message: err.Error()}
	}
	host := strings.TrimPrefix(baseURL, "https://")
	domain := strings.ToLower(slug) + "." + host
	if !validDomain(domain) {
		return FieldError{Field: "configuration.general.siteUrl", Message: "must contain a DNS hostname suitable for a project subdomain"}
	}
	general.SiteURL = baseURL
	general.Domain = domain
	return nil
}

func canonicalBaseSiteURL(value string) (string, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("must be an absolute http or https URL")
	}
	if parsed.User != nil || parsed.Port() != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("must be a base hostname without a port, path, query, or fragment")
	}
	host := strings.ToLower(parsed.Hostname())
	if !validDomain(host) {
		return "", errors.New("must contain a valid DNS hostname")
	}
	return "https://" + host, nil
}

func validAbsoluteHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func validDomain(value string) bool {
	if value == "" || strings.ContainsAny(value, "/ ") || strings.Contains(value, "://") {
		return false
	}
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	if host == "localhost" || net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if !slugPattern.MatchString(label) {
			return false
		}
	}
	return true
}
