package redact

import (
	"regexp"
	"sort"
	"strings"
)

const marker = "[REDACTED]"

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(apikey\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)((?:JWT_SECRET|SERVICE_ROLE_KEY|SUPABASE_SECRET_KEY|POSTGRES_PASSWORD|SMTP_PASSWORD|AWS_SECRET_ACCESS_KEY|CLIENT_SECRET|SECRET_KEY_BASE|VAULT_ENC_KEY)\s*=\s*)[^\s]+`),
}

type Redactor struct {
	known []string
}

func New(knownValues []string) *Redactor {
	values := make([]string, 0, len(knownValues))
	for _, value := range knownValues {
		if len(value) >= 4 {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(left, right int) bool { return len(values[left]) > len(values[right]) })
	return &Redactor{known: values}
}

func (r *Redactor) String(input string) string {
	output := input
	for _, value := range r.known {
		output = strings.ReplaceAll(output, value, marker)
	}
	for _, pattern := range credentialPatterns {
		output = pattern.ReplaceAllString(output, `${1}`+marker)
	}
	return output
}
