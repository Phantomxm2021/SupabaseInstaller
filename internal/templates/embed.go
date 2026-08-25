package templates

import (
	"embed"
	"fmt"
)

//go:embed all:self-hosted-v0.8.0
var runtime embed.FS

func DockerCompose() []byte {
	data, err := runtime.ReadFile("self-hosted-v0.8.0/docker-compose.yml")
	if err != nil {
		panic(fmt.Sprintf("embedded official Supabase Compose is unavailable: %v", err))
	}
	return data
}

func Files() embed.FS {
	return runtime
}
