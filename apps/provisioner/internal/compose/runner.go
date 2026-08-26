package compose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ProjectRef struct {
	Slug        string
	Dir         string
	ComposeFile string
	EnvFile     string
}

type Executor interface {
	Run(ctx context.Context, command string, args, env []string) ([]byte, error)
}

type OSExecutor struct{}

func (OSExecutor) Run(ctx context.Context, command string, args, env []string) ([]byte, error) {
	process := exec.CommandContext(ctx, command, args...)
	process.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	output, err := process.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s failed: %w", command, err)
	}
	return output, nil
}

type Runner struct {
	executor Executor
}

// RotateDatabasePassword changes the postgres role using fixed argv and psql
// variables. Neither password is interpolated into a shell command or output.
func (r *Runner) RotateDatabasePassword(ctx context.Context, project ProjectRef, oldPassword, newPassword string) error {
	if oldPassword == "" || newPassword == "" {
		return fmt.Errorf("database password values are required")
	}
	args := append(r.baseArgs(project), "exec", "-T", "-e", "PGPASSWORD="+oldPassword, "db", "psql", "-U", "postgres", "-d", "postgres", "-v", "new_password="+newPassword, "-c", "ALTER ROLE postgres PASSWORD :'new_password'")
	output, err := r.executor.Run(ctx, "docker", args, nil)
	if err != nil {
		return fmt.Errorf("database password update failed; output length=%d", len(output))
	}
	return nil
}

// composeServices is the closed set emitted by the pinned renderer. Reconcile
// never accepts arbitrary compose arguments from a request.
var composeServices = map[string]struct{}{
	"db": {}, "api-gw": {}, "envoy": {}, "kong": {}, "auth": {}, "rest": {}, "meta": {},
	"studio": {}, "realtime": {}, "storage": {}, "imgproxy": {}, "functions": {},
	"supavisor": {}, "db-config": {}, "analytics": {}, "logflare": {}, "vector": {}, "deno-cache": {}, "caddy": {},
}

func NewRunner(executor Executor) *Runner {
	return &Runner{executor: executor}
}

func (r *Runner) Pull(ctx context.Context, project ProjectRef) error {
	return r.run(ctx, project, "pull")
}

func (r *Runner) UpDatabase(ctx context.Context, project ProjectRef) error {
	return r.run(ctx, project, "up", "-d", "--wait", "db")
}

func (r *Runner) UpServices(ctx context.Context, project ProjectRef, services ...string) error {
	args := append([]string{"up", "-d", "--wait"}, services...)
	return r.run(ctx, project, args...)
}

func (r *Runner) Stop(ctx context.Context, project ProjectRef) error {
	return r.run(ctx, project, "stop")
}

func (r *Runner) Restart(ctx context.Context, project ProjectRef, services ...string) error {
	args := append([]string{"restart"}, services...)
	return r.run(ctx, project, args...)
}

func (r *Runner) Recreate(ctx context.Context, project ProjectRef, services ...string) error {
	if err := validateServices(services); err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}
	args := append([]string{"up", "-d", "--force-recreate", "--remove-orphans"}, services...)
	return r.run(ctx, project, args...)
}

// Validate validates a candidate Compose model without changing containers.
func (r *Runner) Validate(ctx context.Context, project ProjectRef) error {
	return r.run(ctx, project, "config", "--quiet")
}

// UpSelected starts only renderer-selected services.
func (r *Runner) UpSelected(ctx context.Context, project ProjectRef, services ...string) error {
	if err := validateServices(services); err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}
	args := append([]string{"up", "-d", "--remove-orphans"}, services...)
	return r.run(ctx, project, args...)
}

// RemoveStopped removes disabled containers while preserving all volumes.
func (r *Runner) RemoveStopped(ctx context.Context, project ProjectRef, services ...string) error {
	if err := validateServices(services); err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}
	args := append([]string{"rm", "-s", "-f"}, services...)
	return r.run(ctx, project, args...)
}

func (r *Runner) DownRuntime(ctx context.Context, project ProjectRef) error {
	return r.run(ctx, project, "down", "--remove-orphans")
}

func (r *Runner) RemoveTemporary(ctx context.Context, project ProjectRef) error {
	return r.DownRuntime(ctx, project)
}

func (r *Runner) Logs(ctx context.Context, project ProjectRef, service string, lines int) ([]byte, error) {
	args := r.baseArgs(project)
	args = append(args, "logs", "--no-color", "--tail", fmt.Sprintf("%d", lines))
	if service != "" {
		args = append(args, service)
	}
	return r.executor.Run(ctx, "docker", args, nil)
}

func (r *Runner) run(ctx context.Context, project ProjectRef, action ...string) error {
	args := append(r.baseArgs(project), action...)
	output, err := r.executor.Run(ctx, "docker", args, nil)
	if err != nil {
		return fmt.Errorf("compose action failed: %w; output length=%d", err, len(output))
	}
	return nil
}

func (r *Runner) baseArgs(project ProjectRef) []string {
	composeFile := project.ComposeFile
	if composeFile == "" {
		composeFile = filepath.Join(project.Dir, "docker-compose.yml")
	}
	args := []string{"compose", "--file", composeFile}
	if project.EnvFile != "" {
		args = append(args, "--env-file", project.EnvFile)
	}
	return append(args, "--project-directory", project.Dir, "--project-name", "supabase-manager-"+project.Slug)
}

func validateServices(services []string) error {
	for _, service := range services {
		if _, ok := composeServices[service]; !ok {
			return fmt.Errorf("unsupported compose service %q", service)
		}
	}
	return nil
}
