package compose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type ProjectRef struct {
	Slug string
	Dir  string
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
	args := append([]string{"up", "-d", "--force-recreate", "--wait"}, services...)
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
	return r.executor.Run(ctx, "docker", args, []string{"COMPOSE_FILE=docker-compose.yml"})
}

func (r *Runner) run(ctx context.Context, project ProjectRef, action ...string) error {
	args := append(r.baseArgs(project), action...)
	output, err := r.executor.Run(ctx, "docker", args, []string{"COMPOSE_FILE=docker-compose.yml"})
	if err != nil {
		return fmt.Errorf("compose action failed: %w; output length=%d", err, len(output))
	}
	return nil
}

func (r *Runner) baseArgs(project ProjectRef) []string {
	return []string{"compose", "--project-directory", project.Dir, "--project-name", "supabase-manager-" + project.Slug}
}
