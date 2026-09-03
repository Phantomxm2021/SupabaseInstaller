package templates

// Channel identifies how a runtime image manifest was assembled.
type Channel string

const (
	// ChannelOfficial is imported from the official Supabase self-hosted Compose
	// configuration and validated as a complete service set.
	ChannelOfficial Channel = "OFFICIAL"
	// ChannelExperimental is reserved for explicitly opted-in manifests built
	// from newer individual Docker Hub images.
	ChannelExperimental Channel = "EXPERIMENTAL"
)

// RuntimeManifest is the immutable user-visible runtime image set. Template
// snapshot names are implementation details and are deliberately kept apart
// from the manifest ID displayed in the Manager UI.
type RuntimeManifest struct {
	ID           string            `json:"id"`
	Label        string            `json:"label"`
	Channel      Channel           `json:"channel"`
	TemplateRoot string            `json:"-"`
	Images       map[string]string `json:"images"`
}

var manifests = []RuntimeManifest{
	{
		ID:           "official-2026-08-03",
		Label:        "Latest official",
		Channel:      ChannelOfficial,
		TemplateRoot: "self-hosted-v0.8.0",
		Images: map[string]string{
			"studio": "supabase/studio:2026.08.03-sha-022b374",
			"auth":   "supabase/gotrue:v2.189.0",
			"db":     "supabase/postgres:17.6.1.136",
		},
	},
}

var legacyTemplateManifests = map[string]string{
	"self-hosted/v0.8.0": "official-2026-08-03",
}

// LatestOfficial returns the newest bundled official manifest.
func LatestOfficial() RuntimeManifest { return cloneManifest(manifests[0]) }

// SupportedManifests returns a copy of the catalog in newest-first order.
func SupportedManifests() []RuntimeManifest {
	result := make([]RuntimeManifest, len(manifests))
	for index, manifest := range manifests {
		result[index] = cloneManifest(manifest)
	}
	return result
}

// Lookup returns an exact immutable manifest ID only. In particular, it never
// treats mutable Docker tags such as "latest" as an accepted identifier.
func Lookup(id string) (RuntimeManifest, bool) {
	for _, manifest := range manifests {
		if manifest.ID == id {
			return cloneManifest(manifest), true
		}
	}
	return RuntimeManifest{}, false
}

// ResolveLegacy maps the template snapshot IDs persisted by previous Manager
// versions to their imported runtime image manifest.
func ResolveLegacy(id string) (RuntimeManifest, bool) {
	manifestID, ok := legacyTemplateManifests[id]
	if !ok {
		return RuntimeManifest{}, false
	}
	return Lookup(manifestID)
}

func cloneManifest(manifest RuntimeManifest) RuntimeManifest {
	result := manifest
	result.Images = make(map[string]string, len(manifest.Images))
	for name, image := range manifest.Images {
		result.Images[name] = image
	}
	return result
}
