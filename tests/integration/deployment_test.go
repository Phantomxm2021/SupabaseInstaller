package integration

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type composeConfig struct {
	Services map[string]struct {
		Ports   []string `yaml:"ports"`
		Volumes []string `yaml:"volumes"`
	} `yaml:"services"`
}

func TestComposeExposesOnlyManagerAndMountsSocketOnlyIntoProvisioner(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/docker-compose.yml")
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	var config composeConfig
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parse compose: %v", err)
	}
	manager := config.Services["manager"]
	provisioner := config.Services["provisioner"]
	if len(manager.Ports) != 1 || manager.Ports[0] != "${MANAGER_PORT:-8080}:8080" {
		t.Fatalf("manager ports = %#v", manager.Ports)
	}
	if len(provisioner.Ports) != 0 {
		t.Fatalf("provisioner ports = %#v, want none", provisioner.Ports)
	}
	if strings.Contains(strings.Join(manager.Volumes, " "), "docker.sock") {
		t.Fatal("manager must not mount Docker socket")
	}
	if !strings.Contains(strings.Join(provisioner.Volumes, " "), "docker.sock") {
		t.Fatal("provisioner must mount Docker socket")
	}
}
