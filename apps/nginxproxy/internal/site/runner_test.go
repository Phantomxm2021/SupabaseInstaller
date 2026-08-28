package site

import (
	"context"
	"reflect"
	"testing"
)

func TestSystemRunnerUsesFixedNginxCommands(t *testing.T) {
	executor := &recordingExecutor{}
	runner := NewSystemRunner(executor, "/usr/sbin/nginx", "/bin/systemctl")

	if err := runner.Test(context.Background()); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if err := runner.Reload(context.Background()); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if got, want := executor.calls, []commandCall{
		{command: "/usr/sbin/nginx", arguments: []string{"-t"}},
		{command: "/bin/systemctl", arguments: []string{"reload", "nginx"}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

type recordingExecutor struct {
	calls []commandCall
}

type commandCall struct {
	command   string
	arguments []string
}

func (r *recordingExecutor) Run(_ context.Context, command string, arguments ...string) ([]byte, error) {
	r.calls = append(r.calls, commandCall{command: command, arguments: arguments})
	return nil, nil
}
