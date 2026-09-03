package functionlogs

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"supabase-manager/internal/diagnostic"
)

const maxMessageBytes = 10 * 1024

var (
	projectSecretNames = exactNameSet(
		"AUTHORIZATION",
		"POSTGRES_PASSWORD", "JWT_SECRET", "ANON_KEY", "SERVICE_ROLE_KEY",
		"SUPABASE_PUBLISHABLE_KEY", "SUPABASE_SECRET_KEY", "ANON_KEY_ASYMMETRIC", "SERVICE_ROLE_KEY_ASYMMETRIC", "JWT_KEYS", "JWT_JWKS",
		"DASHBOARD_USERNAME", "DASHBOARD_PASSWORD", "SECRET_KEY_BASE", "VAULT_ENC_KEY", "PG_META_CRYPTO_KEY", "REALTIME_DB_ENC_KEY",
		"OPENAI_API_KEY", "SMTP_USER", "SMTP_PASS", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "PHONE_SECRET",
		"LOGFLARE_PUBLIC_ACCESS_TOKEN", "LOGFLARE_PRIVATE_ACCESS_TOKEN", "S3_PROTOCOL_ACCESS_KEY_ID", "S3_PROTOCOL_ACCESS_KEY_SECRET",
		"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD",
		"APPLE_SECRET", "AZURE_SECRET", "BITBUCKET_SECRET", "DISCORD_SECRET", "FACEBOOK_SECRET", "FIGMA_SECRET",
		"GITHUB_SECRET", "GITLAB_SECRET", "GOOGLE_SECRET", "KAKAO_SECRET", "KEYCLOAK_SECRET", "LINKEDIN_OIDC_SECRET",
		"NOTION_SECRET", "SLACK_OIDC_SECRET", "SNAPCHAT_SECRET", "SPOTIFY_SECRET", "TWITCH_SECRET", "TWITTER_SECRET",
		"WORKOS_SECRET", "ZOOM_SECRET",
	)
	messageCredentials = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(["']?\b(?:api[_-]?key|apikey|password|passwd|pwd|secret|token|access[_-]?key(?:[_-]?id)?|secret[_-]?key|private[_-]?key|client[_-]?secret|jwt[_-]?secret|service[_-]?role[_-]?key|supabase[_-]?secret[_-]?key|postgres[_-]?password|smtp[_-]?password|aws[_-]?secret[_-]?access[_-]?key|vault[_-]?enc[_-]?key|secret[_-]?key[_-]?base)\b["']?\s*(?:=|:)\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`),
		regexp.MustCompile(`(?i)(\bauthorization\s*:\s*[^\s,;]+\s+)[^\s,;]+`),
		regexp.MustCompile(`(?i)(\bbearer\s+)[^\s,;]+`),
	}
)

type Redactor struct{ known []string }

func LoadRedactor(projectEnvPath, functionsEnvPath string) (*Redactor, error) {
	known, err := readDotenvValues(projectEnvPath, func(name string) bool { return projectSecretNames[name] })
	if err != nil {
		return nil, err
	}
	functionValues, err := readDotenvValues(functionsEnvPath, func(string) bool { return true })
	if err != nil {
		return nil, err
	}
	known = append(known, functionValues...)
	sort.SliceStable(known, func(i, j int) bool { return len(known[i]) > len(known[j]) })
	return &Redactor{known: known}, nil
}

func exactNameSet(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func readDotenvValues(path string, include func(string) bool) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var values []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "export "))
		if !ok || name == "" || !include(name) {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if value != "" {
			values = append(values, value)
		}
	}
	return values, scanner.Err()
}

func (r *Redactor) SanitizeMessage(input string) (string, bool) {
	input = strings.ToValidUTF8(input, string(utf8.RuneError))
	for _, value := range r.known {
		input = strings.ReplaceAll(input, value, "[REDACTED]")
	}
	for _, pattern := range messageCredentials {
		input = pattern.ReplaceAllString(input, `${1}[REDACTED]`)
	}
	var sanitized strings.Builder
	for len(input) > 0 {
		end := len(input)
		if end > 3072 {
			end = 3072
			for !utf8.RuneStart(input[end]) {
				end--
			}
		}
		sanitized.WriteString(diagnostic.Sanitize(input[:end], r.known))
		input = input[end:]
	}
	message := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, sanitized.String())
	if len(message) <= maxMessageBytes {
		return message, false
	}
	message = message[:maxMessageBytes]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message, true
}
