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

func TestReconcileRunnerUsesFixedComposeArgumentVectors(t *testing.T) {
	executor := &fakeExecutor{}
	runner := NewRunner(executor)
	project := ProjectRef{Slug: "bee", Dir: "/projects/bee"}
	if err := runner.Validate(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "config", "--quiet"}
	if !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("Validate args = %#v, want %#v", executor.args, want)
	}
	if err := runner.UpSelected(context.Background(), project, "auth", "rest"); err != nil {
		t.Fatal(err)
	}
	want = []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "up", "-d", "--remove-orphans", "auth", "rest"}
	if !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("UpSelected args = %#v, want %#v", executor.args, want)
	}
	if err := runner.Recreate(context.Background(), project, "auth"); err != nil {
		t.Fatal(err)
	}
	want = []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "up", "-d", "--force-recreate", "--remove-orphans", "auth"}
	if !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("Recreate args = %#v, want %#v", executor.args, want)
	}
	if err := runner.RemoveStopped(context.Background(), project, "realtime"); err != nil {
		t.Fatal(err)
	}
	want = []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "rm", "-s", "-f", "realtime"}
	if !reflect.DeepEqual(executor.args, want) {
		t.Fatalf("RemoveStopped args = %#v, want %#v", executor.args, want)
	}
}

func TestReconcileRunnerRejectsUnknownServiceNames(t *testing.T) {
	runner := NewRunner(&fakeExecutor{})
	err := runner.UpSelected(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"}, "--project-directory")
	if err == nil {
		t.Fatal("UpSelected accepted an unsafe service name")
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
