// Package diagnostic sanitizes operational failure details before they cross a
// process or API boundary.
package diagnostic

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"supabase-manager/internal/contracts"
)

const (
	marker         = "[REDACTED]"
	maxOutputBytes = 4 * 1024
)

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(["']?\b(?:api[_-]?key|apikey|password|passwd|pwd|secret|token|access[_-]?key(?:[_-]?id)?|secret[_-]?key|private[_-]?key|client[_-]?secret|jwt[_-]?secret|service[_-]?role[_-]?key|supabase[_-]?secret[_-]?key|postgres[_-]?password|smtp[_-]?password|aws[_-]?secret[_-]?access[_-]?key|vault[_-]?enc[_-]?key|secret[_-]?key[_-]?base)\b["']?\s*(?:=|:)\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`),
	regexp.MustCompile(`(?i)(\bauthorization\s*:\s*[^\s,;]+\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(\bbearer\s+)[^\s,;]+`),
}

// ConfigurationSecretValues returns every plaintext SecretInput value carried
// by a complete project configuration.
func ConfigurationSecretValues(configuration contracts.ProjectConfiguration) []string {
	values := []string{
		configuration.General.StudioPassword.Value,
		configuration.Auth.Phone.Secret.Value,
		configuration.Auth.SMTP.Password.Value,
		configuration.Storage.SecretAccessKey.Value,
	}
	for _, provider := range configuration.Auth.OAuth {
		values = append(values, provider.Secret.Value)
	}
	for _, variable := range configuration.Functions.Variables {
		values = append(values, variable.Value.Value)
	}
	return values
}

// Sanitize returns a bounded, single-line diagnostic with supplied secret
// values and common credential forms replaced by a stable marker.
func Sanitize(input string, knownValues []string) string {
	values := knownSecrets(knownValues)
	output := input
	for _, value := range values {
		output = strings.ReplaceAll(output, value, marker)
	}
	for _, pattern := range credentialPatterns {
		output = pattern.ReplaceAllString(output, `${1}`+marker)
	}
	output = strings.Map(flattenControl, output)
	return truncate(output)
}

func knownSecrets(values []string) []string {
	known := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			known = append(known, value)
		}
	}
	sort.SliceStable(known, func(left, right int) bool {
		return len(known[left]) > len(known[right])
	})
	return known
}

func flattenControl(character rune) rune {
	if unicode.IsControl(character) {
		return ' '
	}
	return character
}

func truncate(input string) string {
	if len(input) <= maxOutputBytes {
		return input
	}
	output := input[:maxOutputBytes]
	for !utf8.ValidString(output) {
		output = output[:len(output)-1]
	}
	for prefixLength := 1; prefixLength < len(marker); prefixLength++ {
		if strings.HasSuffix(output, marker[:prefixLength]) {
			return output[:len(output)-prefixLength]
		}
	}
	return output
}
