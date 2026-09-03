package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"supabase-manager/apps/provisioner/internal/officialtemplate"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/render"

	"gopkg.in/yaml.v3"
)

type fixtureTemplateSource struct{ snapshot officialtemplate.Snapshot }

func (s fixtureTemplateSource) Resolve(context.Context, string, bool) (officialtemplate.Snapshot, error) {
	return s.snapshot, nil
}

func init() {
	testTemplateSourceFactory = func(*projectfs.Root) templateSource {
		files := map[string][]byte{}
		root := filepath.Join("..", "..", "..", "..", "internal", "templates", "self-hosted-v0.8.0")
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files[filepath.ToSlash(relative)] = data
			return nil
		})
		digest := sha256.Sum256(files["docker-compose.yml"])
		return fixtureTemplateSource{snapshot: officialtemplate.Snapshot{Ref: "self-hosted/v0.8.0", SHA256: hex.EncodeToString(digest[:]), Files: files}}
	}
}

func TestRuntimeTemplateFixtureIsComplete(t *testing.T) {
	snapshot, err := fixtureTemplateSnapshot()
	if err != nil || len(snapshot.Compose()) == 0 || len(snapshot.EnvExample()) == 0 {
		t.Fatalf("fixture = %#v, %v", snapshot, err)
	}
}

func fixtureTemplateSnapshot() (officialtemplate.Snapshot, error) {
	return testTemplateSourceFactory(nil).Resolve(context.Background(), "self-hosted/latest", false)
}

func fixtureRenderInput(t *testing.T, input render.Input) render.Input {
	t.Helper()
	snapshot, err := fixtureTemplateSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	compose, err := render.LoadOfficialCompose(input.Configuration, snapshot.Files)
	if err != nil {
		t.Fatal(err)
	}
	input.TemplateCompose, err = yaml.Marshal(compose)
	if err != nil {
		t.Fatal(err)
	}
	input.TemplateEnv, input.TemplateFiles = snapshot.EnvExample(), snapshot.Files
	return input
}
