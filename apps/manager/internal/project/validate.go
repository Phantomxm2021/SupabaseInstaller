package project

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
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
	if !validDomain(cfg.General.Domain) {
		errs = append(errs, FieldError{Field: "configuration.general.domain", Message: "must be a hostname without a scheme or path"})
	}
	if !validAbsoluteHTTPURL(cfg.General.SiteURL) {
		errs = append(errs, FieldError{Field: "configuration.general.siteUrl", Message: "must be an absolute http or https URL"})
	}
	if cfg.General.SupabaseVersion == "" || strings.EqualFold(cfg.General.SupabaseVersion, "latest") || strings.EqualFold(cfg.General.SupabaseVersion, "master") {
		errs = append(errs, FieldError{Field: "configuration.general.supabaseVersion", Message: "must be a pinned supported version"})
	}
	if errConfiguration := ValidateConfiguration(cfg); errConfiguration != nil {
		errs = append(errs, errConfiguration)
	}
	return errors.Join(errs...)
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
