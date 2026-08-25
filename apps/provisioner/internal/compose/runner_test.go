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
	want := []string{"compose", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "up", "-d", "--wait", "db"}
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
