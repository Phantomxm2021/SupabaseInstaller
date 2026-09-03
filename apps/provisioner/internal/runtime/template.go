package runtime

import (
	"fmt"

	"supabase-manager/apps/provisioner/internal/render"

	"gopkg.in/yaml.v3"
)

func (backend *Backend) renderStoredTemplate(slug string, input render.Input) (render.OutputFiles, error) {
	if input.ProvisionerImageRef == "" {
		input.ProvisionerImageRef = backend.provisionerImageRef
	}
	files, err := backend.projectFS.OfficialTemplateFiles(slug)
	if err != nil {
		return render.OutputFiles{}, fmt.Errorf("load official template snapshot: %w", err)
	}
	compose, err := render.LoadOfficialCompose(input.Configuration, files)
	if err != nil {
		return render.OutputFiles{}, err
	}
	input.TemplateCompose, err = yaml.Marshal(compose)
	if err != nil {
		return render.OutputFiles{}, fmt.Errorf("encode official template snapshot: %w", err)
	}
	input.TemplateEnv, input.TemplateFiles = files[".env.example"], files
	return render.Project(input)
}
