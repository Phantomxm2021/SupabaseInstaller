package compose

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if !containsArgument(executor.args, "supabase_admin") {
		t.Fatalf("role synchronization must use the official superuser: %v", executor.args)
	}
	if !strings.Contains(string(executor.input), "new-sentinel") {
		t.Fatal("new password was not supplied through controlled SQL input")
	}
}

func containsArgument(arguments []string, expected string) bool {
	for _, argument := range arguments {
		if argument == expected {
			return true
		}
	}
	return false
}

func TestRotateDatabasePasswordRequiresControlledInputExecutor(t *testing.T) {
	if err := NewRunner(&fakeExecutor{}).RotateDatabasePassword(context.Background(), ProjectRef{Slug: "bee", Dir: "/tmp/project"}, "old", "new"); err == nil || !strings.Contains(err.Error(), "secure database password input") {
		t.Fatalf("rotation without controlled input error = %v", err)
	}
}

func TestSynchronizeDatabaseRolePasswordsUsesRenderedDatabasePassword(t *testing.T) {
	directory := t.TempDir()
	envFile := filepath.Join(directory, ".env")
	if err := os.WriteFile(envFile, []byte("POSTGRES_PASSWORD=runtime-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &inputCaptureExecutor{}
	runner := NewRunner(executor)
	project := ProjectRef{Slug: "bee", Dir: directory, ComposeFile: filepath.Join(directory, "docker-compose.yml"), EnvFile: envFile}
	if err := runner.SynchronizeDatabaseRolePasswords(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	for _, arg := range executor.args {
		if strings.Contains(arg, "runtime-secret") {
			t.Fatalf("secret leaked into argv: %v", executor.args)
		}
	}
	input := string(executor.input)
	for _, role := range []string{"authenticator", "pgbouncer", "supabase_auth_admin", "supabase_functions_admin", "supabase_storage_admin"} {
		if !strings.Contains(input, role) {
			t.Fatalf("password synchronization omitted role %q: %s", role, input)
		}
	}
	if !strings.Contains(input, "runtime-secret") {
		t.Fatal("rendered database password was not supplied through controlled SQL input")
	}
}

func TestVerifyDatabaseBootstrapRejectsAnIncompleteOfficialBootstrap(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{{
		output: []byte("schema:auth:supabase_admin\nfunction:auth.email:supabase_auth_admin\nfunction:auth.role:supabase_auth_admin\nfunction:auth.uid:supabase_admin\n"),
	}, {
		output: []byte("/docker-entrypoint-initdb.d/migrate.sh: migration sequence was not executed"),
	}}}
	err := NewRunner(executor).VerifyDatabaseBootstrap(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err == nil || !strings.Contains(err.Error(), "auth.uid") || !strings.Contains(err.Error(), "supabase_auth_admin") || !strings.Contains(err.Error(), "migration sequence was not executed") {
		t.Fatalf("VerifyDatabaseBootstrap() error = %v, want auth.uid ownership violation", err)
	}
	want := []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "exec", "-T", "db", "psql", "-v", "ON_ERROR_STOP=1", "-U", "supabase_admin", "-d", "postgres", "-At", "-c"}
	if len(executor.calls) != 2 || len(executor.calls[0]) != len(want)+1 || !reflect.DeepEqual(executor.calls[0][:len(want)], want) {
		t.Fatalf("bootstrap verification args = %#v, want %#v", executor.calls, want)
	}
	if got := executor.calls[1]; !reflect.DeepEqual(got, []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "logs", "--no-color", "--tail", "240", "db"}) {
		t.Fatalf("bootstrap diagnostics args = %#v", got)
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

func TestStorageObjectCountUsesFixedSafeQuery(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{{output: []byte("0\n")}}}
	runner := NewRunner(executor)
	got, err := runner.StorageObjectCount(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err != nil || got != 0 {
		t.Fatalf("StorageObjectCount() = %d, %v; want 0, nil", got, err)
	}
	want := []string{"compose", "--file", "/projects/bee/docker-compose.yml", "--project-directory", "/projects/bee", "--project-name", "supabase-manager-bee", "exec", "-T", "db", "psql", "-v", "ON_ERROR_STOP=1", "-U", "supabase_admin", "-d", "postgres", "-At", "-c", "SELECT count(*) FROM storage.objects;"}
	if !reflect.DeepEqual(executor.calls[0], want) {
		t.Fatalf("query args = %#v, want %#v", executor.calls[0], want)
	}
}

func TestStorageObjectCountFailsClosedOnMalformedOutput(t *testing.T) {
	for _, output := range []string{"", "1\n2\n", "-1\n", "not-a-count\n", "1 extra\n"} {
		t.Run(strings.TrimSpace(output), func(t *testing.T) {
			executor := &sequenceExecutor{results: []executorResult{{output: []byte(output)}}}
			if got, err := NewRunner(executor).StorageObjectCount(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"}); err == nil || got != 0 {
				t.Fatalf("StorageObjectCount() = %d, %v; want parse error", got, err)
			}
		})
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

func TestResetDatabaseConfigRemovesOnlyScopedVolume(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{
		{output: []byte("supabase-manager-bee_db-config\n")},
		{},
	}}
	runner := NewRunner(executor)
	if err := runner.ResetDatabaseConfig(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"}); err != nil {
		t.Fatalf("ResetDatabaseConfig() error = %v", err)
	}
	if len(executor.calls) != 2 {
		t.Fatalf("calls = %#v, want scoped lookup and removal", executor.calls)
	}
	wantLookup := []string{"volume", "ls", "-q", "--filter", "label=com.docker.compose.project=supabase-manager-bee", "--filter", "label=com.docker.compose.volume=db-config"}
	if !reflect.DeepEqual(executor.calls[0], wantLookup) {
		t.Fatalf("volume lookup args = %#v, want %#v", executor.calls[0], wantLookup)
	}
	wantRemove := []string{"volume", "rm", "supabase-manager-bee_db-config"}
	if !reflect.DeepEqual(executor.calls[1], wantRemove) {
		t.Fatalf("volume removal args = %#v, want %#v", executor.calls[1], wantRemove)
	}
}

func TestResetDatabaseConfigUsesServerTerminologyForVolumeFailures(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{{err: errors.New("docker unavailable")}}}
	err := NewRunner(executor).ResetDatabaseConfig(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err == nil || !strings.Contains(err.Error(), "list server database configuration volume") {
		t.Fatalf("ResetDatabaseConfig() error = %v, want server terminology", err)
	}
}

func TestDownRuntimeUsesServerTerminologyForFallbackFailures(t *testing.T) {
	executor := &sequenceExecutor{results: []executorResult{
		{err: errors.New("compose failed")},
		{err: errors.New("docker unavailable")},
	}}
	err := NewRunner(executor).DownRuntime(context.Background(), ProjectRef{Slug: "bee", Dir: "/projects/bee"})
	if err == nil || !strings.Contains(err.Error(), "server-scoped cleanup failed") || !strings.Contains(err.Error(), "list server containers") {
		t.Fatalf("DownRuntime() error = %v, want server terminology", err)
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
