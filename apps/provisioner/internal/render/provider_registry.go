package render

import (
	"sort"
	"strings"

	"supabase-manager/internal/contracts"
)

type providerDefinition struct {
	Name    string
	Special map[string]string
	Fields  map[string]string
}

var commonProviderFields = func(name string) map[string]string {
	return map[string]string{
		"skipNonceChecks":        name + "_SKIP_NONCE_CHECK",
		"allowUsersWithoutEmail": name + "_EMAIL_OPTIONAL",
	}
}

var providerDefinitions = map[string]providerDefinition{
	"apple": {Name: "APPLE", Fields: commonProviderFields("APPLE")}, "azure": {Name: "AZURE", Special: map[string]string{"tenantUrl": "AZURE_URL"}, Fields: commonProviderFields("AZURE")},
	"bitbucket": {Name: "BITBUCKET", Fields: commonProviderFields("BITBUCKET")}, "discord": {Name: "DISCORD", Fields: commonProviderFields("DISCORD")}, "facebook": {Name: "FACEBOOK", Fields: commonProviderFields("FACEBOOK")},
	"figma": {Name: "FIGMA", Fields: commonProviderFields("FIGMA")}, "github": {Name: "GITHUB", Special: map[string]string{"enterpriseUrl": "GITHUB_URL"}, Fields: commonProviderFields("GITHUB")},
	"gitlab": {Name: "GITLAB", Special: map[string]string{"selfHostedUrl": "GITLAB_URL"}, Fields: commonProviderFields("GITLAB")}, "google": {Name: "GOOGLE", Fields: commonProviderFields("GOOGLE")},
	"kakao": {Name: "KAKAO", Fields: commonProviderFields("KAKAO")}, "keycloak": {Name: "KEYCLOAK", Special: map[string]string{"realmUrl": "KEYCLOAK_URL"}, Fields: commonProviderFields("KEYCLOAK")},
	"linkedin_oidc": {Name: "LINKEDIN_OIDC", Fields: commonProviderFields("LINKEDIN_OIDC")}, "notion": {Name: "NOTION", Fields: commonProviderFields("NOTION")}, "slack_oidc": {Name: "SLACK_OIDC", Fields: commonProviderFields("SLACK_OIDC")},
	"snapchat": {Name: "SNAPCHAT", Fields: commonProviderFields("SNAPCHAT")}, "spotify": {Name: "SPOTIFY", Fields: commonProviderFields("SPOTIFY")}, "twitch": {Name: "TWITCH", Fields: commonProviderFields("TWITCH")}, "twitter": {Name: "TWITTER", Fields: commonProviderFields("TWITTER")},
	"workos": {Name: "WORKOS", Fields: commonProviderFields("WORKOS")}, "zoom": {Name: "ZOOM", Fields: commonProviderFields("ZOOM")},
}

func providerDefinitionFor(name string) providerDefinition {
	if definition, ok := providerDefinitions[strings.ToLower(name)]; ok {
		return definition
	}
	return providerDefinition{Name: strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name))}
}

func configuredProviderNames(oauth map[string]contracts.OAuthProviderConfig) []string {
	names := make([]string, 0, len(oauth))
	for name := range oauth {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
