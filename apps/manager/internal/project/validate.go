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
	draft = NormalizeDraft(draft)
	var errs []error
	if name := strings.TrimSpace(draft.Name); name == "" || len(name) > 80 {
		errs = append(errs, FieldError{Field: "name", Message: "must contain 1 to 80 characters"})
	}
	if !slugPattern.MatchString(draft.Slug) {
		errs = append(errs, FieldError{Field: "slug", Message: "must match [a-z0-9-] and be a valid DNS label"})
	}
	if !validDomain(draft.Domain) {
		errs = append(errs, FieldError{Field: "domain", Message: "must be a hostname without a scheme or path"})
	}
	if !validAbsoluteHTTPURL(draft.SiteURL) {
		errs = append(errs, FieldError{Field: "siteUrl", Message: "must be an absolute http or https URL"})
	}
	if draft.SupabaseVersion == "" || strings.EqualFold(draft.SupabaseVersion, "latest") || strings.EqualFold(draft.SupabaseVersion, "master") {
		errs = append(errs, FieldError{Field: "supabaseVersion", Message: "must be a pinned supported version"})
	}
	errServices := validateServices(draft.Services)
	if errServices != nil {
		errs = append(errs, errServices)
	}
	if errConfiguration := ValidateConfiguration(draft.Configuration); errConfiguration != nil {
		errs = append(errs, errConfiguration)
	}
	return errors.Join(errs...)
}

func validateServices(services Services) error {
	var errs []error
	if !services.Database {
		errs = append(errs, FieldError{Field: "services.database", Message: "PostgreSQL is required"})
	}
	if services.Studio && !services.PostgresMeta {
		errs = append(errs, FieldError{Field: "services.postgresMeta", Message: "postgres-meta is required by Studio"})
	}
	if (services.Auth || services.REST || services.Studio || services.Realtime || services.Storage) && !services.Gateway {
		errs = append(errs, FieldError{Field: "services.gateway", Message: "API Gateway is required by enabled public services"})
	}
	if services.Imgproxy && !services.Storage {
		errs = append(errs, FieldError{Field: "services.imgproxy", Message: "Image Transformation requires Storage"})
	}
	if services.Logs != services.Vector {
		errs = append(errs, FieldError{Field: "services.vector", Message: "Logs and Vector must be enabled together"})
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
