package compose

import (
	"context"
	"reflect"
	"testing"
)

func TestRunnerUsesArgumentVectorAndFixedProjectDirectory(t *testing.T) {
	executor := &fakeExecutor{}
	runner := NewRunner(executor)
	err := runner.UpDatabase(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err != nil {
		t.Fatalf("UpDatabase() error = %v", err)
	}
	want := []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "up", "-d", "--wait", "db"}
	if executor.command != "docker" || !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("command = %q %#v, want docker %#v", executor.command, executor.args, want)
	}
}

func TestDownRuntimeDoesNotDeleteVolumes(t *testing.T) {
	executor := &fakeExecutor{}
	runner := NewRunner(executor)
	_ = runner.DownRuntime(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	for _, argument := range executor.args {
		if argument == "-v" || argument == "--volumes" {
			t.Fatalf("DownRuntime() included destructive volume argument: %#v", executor.args)
		}
	}
}

func TestRunnerUsesStableProjectDirAndCurrentConfig(t *testing.T) {
	executor := &fakeExecutor{}
	runner := NewRunner(executor)
	project := ProjectRef{Slug: "bee", Dir: "/projects/bee", ComposeFile: "/projects/bee/.manager-runtime/current/docker-compose.yml", EnvFile: "/projects/bee/.manager-runtime/current/.env"}
	if err := runner.UpDatabase(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "--file", project.ComposeFile, "--env-file", project.EnvFile, "--project-directory", project.Dir, "--project-name", "supabase-manager-bee", "up", "-d", "--wait", "db"}
	if !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("command args = %#v, want %#v", executor.args, want)
	}
}

type fakeExecutor struct {
	command string
	args    []string
	env     []string
}

func (executor *fakeExecutor) Run(_ context.Context, command string, args, env []string) ([]byte, error) {
	executor.command = command
	executor.args = append([]string(nil), args...)
	executor.env = append([]string(nil), env...)
	return []byte("ok"), nil
}
