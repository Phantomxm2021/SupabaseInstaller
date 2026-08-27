package compose

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type inputCaptureExecutor struct {
	args  []string
	input []byte
}

func (e *inputCaptureExecutor) Run(_ context.Context, _ string, args, _ []string) ([]byte, error) {
	e.args = args
	return nil, nil
}
func (e *inputCaptureExecutor) RunInput(_ context.Context, _ string, args, _ []string, input []byte) ([]byte, error) {
	e.args = args
	e.input = input
	return nil, nil
}

func TestRotateDatabasePasswordKeepsSecretsOutOfArgv(t *testing.T) {
	executor := &inputCaptureExecutor{}
	runner := NewRunner(executor)
	if err := runner.RotateDatabasePassword(context.Background(), ProjectRef{Slug: "bee", Dir: "/tmp/project", ComposeFile: "/tmp/project/current.yml"}, "old-sentinel", "new-sentinel"); err != nil {
		t.Fatal(err)
	}
	for _, arg := range executor.args {
		if strings.Contains(arg, "sentinel") {
			t.Fatalf("secret leaked into argv: %v", executor.args)
		}
	}
	if !strings.Contains(string(executor.input), "new-sentinel") {
		t.Fatal("new password was not supplied through controlled SQL input")
	}
}

func TestRotateDatabasePasswordRequiresControlledInputExecutor(t *testing.T) {
	if err := NewRunner(&fakeExecutor{}).RotateDatabasePassword(context.Background(), ProjectRef{Slug: "bee", Dir: "/tmp/project"}, "old", "new"); err == nil || !strings.Contains(err.Error(), "secure database password input") {
		t.Fatalf("rotation without controlled input error = %v", err)
	}
}

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

func TestDownRuntimeFallsBackToProjectScopedContainerCleanup(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{
		{output: []byte("compose failed"), err: errors.New("compose config failed")},
		{output: []byte("container-a\ncontainer-b\n")},
		{},
		{output: []byte("network-a\n")},
		{},
	}}
	runner := NewRunner(executor)
	if err := runner.DownRuntime(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"}); err != nil {
		t.Fatalf("DownRuntime() error = %v", err)
	}
	if len(executor.calls) != 5 {
		t.Fatalf("fallback calls = %#v", executor.calls)
	}
	wantPS := []string{"ps", "-aq", "--filter", "label=com.docker.compose.project=supabase-manager-bee"}
	if !reflect.DeepEqual(executor.calls[1], wantPS) {
		t.Fatalf("project lookup args = %#v, want %#v", executor.calls[1], wantPS)
	}
	wantRM := []string{"rm", "-f", "container-a", "container-b"}
	if !reflect.DeepEqual(executor.calls[2], wantRM) {
		t.Fatalf("container cleanup args = %#v, want %#v", executor.calls[2], wantRM)
	}
	wantNetworks := []string{"network", "ls", "-q", "--filter", "label=com.docker.compose.project=supabase-manager-bee"}
	if !reflect.DeepEqual(executor.calls[3], wantNetworks) {
		t.Fatalf("network lookup args = %#v, want %#v", executor.calls[3], wantNetworks)
	}
	wantNetworkRM := []string{"network", "rm", "network-a"}
	if !reflect.DeepEqual(executor.calls[4], wantNetworkRM) {
		t.Fatalf("network cleanup args = %#v, want %#v", executor.calls[4], wantNetworkRM)
	}
}

func TestDownRuntimeTreatsMissingProjectContainersAsAlreadyRemoved(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{
		{output: []byte("compose failed"), err: errors.New("compose config failed")},
		{},
		{},
	}}
	runner := NewRunner(executor)
	if err := runner.DownRuntime(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"}); err != nil {
		t.Fatalf("DownRuntime() error = %v", err)
	}
	if len(executor.calls) != 3 {
		t.Fatalf("calls = %#v", executor.calls)
	}
}

func TestRunnerIncludesRedactedComposeOutputInError(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{{
		output: []byte("FATAL: POSTGRES_PASSWORD=super-secret\nmissing env file"),
		err:    errors.New("exit status 1"),
	}}}
	err := NewRunner(executor).Validate(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err == nil || !strings.Contains(err.Error(), "missing env file") || strings.Contains(err.Error(), "super-secret") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunnerKeepsComposeFailureOutputTail(t *testing.T) {
	output := strings.Repeat("pull progress\n", 500) + "final registry error: manifest unknown"
	executor := &sequenceExecutor{results: []executorResult{{
		output: []byte(output),
		err:    errors.New("exit status 1"),
	}}}
	err := NewRunner(executor).Validate(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err == nil || !strings.Contains(err.Error(), "final registry error: manifest unknown") {
		t.Fatalf("error = %v, want final Compose failure detail", err)
	}
}

func TestUpDatabaseIncludesDatabaseLogsBeforeRollback(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{
		{output: []byte("container db is unhealthy"), err: errors.New("exit status 1")},
		{output: []byte("postgres startup fatal: incompatible data directory")},
	}}
	err := NewRunner(executor).UpDatabase(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err == nil || !strings.Contains(err.Error(), "postgres startup fatal: incompatible data directory") {
		t.Fatalf("error = %v, want database startup log", err)
	}
	if len(executor.calls) != 2 {
		t.Fatalf("calls = %#v, want database up and logs", executor.calls)
	}
	want := []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "logs", "--no-color", "--tail", "120", "db"}
	if !reflect.DeepEqual(executor.calls[1], want) {
		t.Fatalf("database log args = %#v, want %#v", executor.calls[1], want)
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
	want = []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "up", "-d", "--force-recreate", "--remove-orphans", "--no-deps", "auth"}
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

type executorResult struct {
	output []byte
	err    error
}

type sequenceExecutor struct {
	results []executorResult
	calls   [][]string
}

func (executor *sequenceExecutor) Run(_ context.Context, command string, args, _ []string) ([]byte, error) {
	executor.calls = append(executor.calls, append([]string(nil), args...))
	if len(executor.results) == 0 {
		return nil, nil
	}
	result := executor.results[0]
	executor.results = executor.results[1:]
	_ = command
	return result.output, result.err
}

func (executor *fakeExecutor) Run(_ context.Context, command string, args, env []string) ([]byte, error) {
	executor.command = command
	executor.args = append([]string(nil), args...)
	executor.env = append([]string(nil), env...)
	return []byte("ok"), nil
}
