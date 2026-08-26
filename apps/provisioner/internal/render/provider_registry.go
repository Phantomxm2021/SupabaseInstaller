package render

import (
	"sort"
	"strings"

	"supabase-manager/internal/contracts"
)

type providerDefinition struct {
	Name    string
	Special map[string]string
}

var providerDefinitions = map[string]providerDefinition{
	"apple": {Name: "APPLE"}, "azure": {Name: "AZURE", Special: map[string]string{"tenantURL": "AZURE_TENANT_URL"}},
	"bitbucket": {Name: "BITBUCKET"}, "discord": {Name: "DISCORD"}, "facebook": {Name: "FACEBOOK"},
	"figma": {Name: "FIGMA"}, "github": {Name: "GITHUB", Special: map[string]string{"enterpriseURL": "GITHUB_URL"}},
	"gitlab": {Name: "GITLAB", Special: map[string]string{"url": "GITLAB_URL"}}, "google": {Name: "GOOGLE"},
	"kakao": {Name: "KAKAO"}, "keycloak": {Name: "KEYCLOAK", Special: map[string]string{"realmURL": "KEYCLOAK_URL", "url": "KEYCLOAK_URL"}},
	"linkedin_oidc": {Name: "LINKEDIN_OIDC"}, "notion": {Name: "NOTION"}, "slack_oidc": {Name: "SLACK_OIDC"},
	"snapchat": {Name: "SNAPCHAT"}, "spotify": {Name: "SPOTIFY"}, "twitch": {Name: "TWITCH"}, "twitter": {Name: "TWITTER"},
	"workos": {Name: "WORKOS"}, "zoom": {Name: "ZOOM"},
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
