package site

import (
	"context"
	"fmt"
	"os/exec"
)

// Executor supports a narrow, fixed command surface for the host agent.
type Executor interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

// SystemRunner validates then reloads Nginx using absolute configured binaries.
type SystemRunner struct {
	executor        Executor
	nginxBinary     string
	systemctlBinary string
}

func NewSystemRunner(executor Executor, nginxBinary, systemctlBinary string) SystemRunner {
	return SystemRunner{
		executor:        executor,
		nginxBinary:     nginxBinary,
		systemctlBinary: systemctlBinary,
	}
}

func (r SystemRunner) Test(ctx context.Context) error {
	output, err := r.executor.Run(ctx, r.nginxBinary, "-t")
	if err != nil {
		return fmt.Errorf("nginx -t: %w: %s", err, output)
	}
	return nil
}

func (r SystemRunner) Reload(ctx context.Context) error {
	output, err := r.executor.Run(ctx, r.systemctlBinary, "reload", "nginx")
	if err != nil {
		return fmt.Errorf("systemctl reload nginx: %w: %s", err, output)
	}
	return nil
}

// OSExecutor is used only by the native host service.
type OSExecutor struct{}

func (OSExecutor) Run(ctx context.Context, command string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, command, arguments...).CombinedOutput()
}
