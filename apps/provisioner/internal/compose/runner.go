package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"supabase-manager/apps/provisioner/internal/redact"
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
type InputExecutor interface {
	RunInput(context.Context, string, []string, []string, []byte) ([]byte, error)
}

type OSExecutor struct{}

func (OSExecutor) RunInput(ctx context.Context, command string, args, env []string, input []byte) ([]byte, error) {
	process := exec.CommandContext(ctx, command, args...)
	process.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	process.Stdin = bytes.NewReader(input)
	return process.CombinedOutput()
}

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

var internalDatabaseRoles = []string{
	"authenticator",
	"pgbouncer",
	"supabase_auth_admin",
	"supabase_functions_admin",
	"supabase_storage_admin",
}

// RotateDatabasePassword changes the database credential for every internal
// service role, not only postgres. Auth, Storage, REST and Supavisor all use
// POSTGRES_PASSWORD, so leaving their roles on the old credential makes a
// seemingly successful rotation break the runtime on its next restart.
func (r *Runner) RotateDatabasePassword(ctx context.Context, project ProjectRef, oldPassword, newPassword string) error {
	if oldPassword == "" || newPassword == "" {
		return fmt.Errorf("database password values are required")
	}
	_ = oldPassword // local postgres socket authentication is used; never put secrets in argv
	return r.setInternalDatabaseRolePasswords(ctx, project, newPassword)
}

// SynchronizeDatabaseRolePasswords makes database startup deterministic. The
// upstream PostgreSQL image creates internal roles during its own bootstrap,
// but an init-script-only password change can be skipped by a partial or
// previously failed bootstrap. Once db is healthy, set every service role from
// the rendered runtime environment before any dependent service is started.
func (r *Runner) SynchronizeDatabaseRolePasswords(ctx context.Context, project ProjectRef) error {
	password, err := requiredDotEnvValue(project.EnvFile, "POSTGRES_PASSWORD")
	if err != nil {
		return err
	}
	return r.setInternalDatabaseRolePasswords(ctx, project, password)
}

func (r *Runner) setInternalDatabaseRolePasswords(ctx context.Context, project ProjectRef, password string) error {
	if password == "" {
		return fmt.Errorf("database password is required")
	}
	// In the official PG17 image `postgres` intentionally is not a superuser.
	// The protected Supabase service roles can only be changed by
	// `supabase_admin` over the local container socket.
	args := append(r.baseArgs(project), "exec", "-T", "db", "psql", "-v", "ON_ERROR_STOP=1", "-U", "supabase_admin", "-d", "postgres")
	if inputRunner, ok := r.executor.(InputExecutor); ok {
		statement := internalRolePasswordStatement(password)
		if output, err := inputRunner.RunInput(ctx, "docker", args, nil, []byte(statement)); err != nil {
			return fmt.Errorf("database role password synchronization failed; output length=%d", len(output))
		}
		return nil
	}
	return errors.New("secure database password input is unavailable")
}

func internalRolePasswordStatement(password string) string {
	quotedPassword := "'" + strings.ReplaceAll(password, "'", "''") + "'"
	roles := make([]string, 0, len(internalDatabaseRoles))
	for _, role := range internalDatabaseRoles {
		roles = append(roles, "'"+strings.ReplaceAll(role, "'", "''")+"'")
	}
	return "SELECT format('ALTER ROLE %I WITH PASSWORD %L;', rolname, " + quotedPassword + ")\n" +
		"FROM pg_roles\n" +
		"WHERE rolname IN (" + strings.Join(roles, ", ") + ")\n" +
		"\\gexec\n"
}

func requiredDotEnvValue(path, key string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("runtime environment file is required to synchronize database roles")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read runtime environment for database role synchronization: %w", err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") {
			unquoted, unquoteErr := strconv.Unquote(value)
			if unquoteErr != nil {
				return "", fmt.Errorf("parse %s in runtime environment: %w", key, unquoteErr)
			}
			value = unquoted
		}
		if value == "" {
			return "", fmt.Errorf("%s is empty in runtime environment", key)
		}
		return value, nil
	}
	return "", fmt.Errorf("%s is missing from runtime environment", key)
}

// composeServices is the closed set emitted by the pinned renderer. Reconcile
// never accepts arbitrary compose arguments from a request.
var composeServices = map[string]struct{}{
	"db": {}, "api-gw": {}, "envoy": {}, "kong": {}, "auth": {}, "auth-templates": {}, "rest": {}, "meta": {},
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
	if err := r.run(ctx, project, "up", "-d", "--wait", "db"); err != nil {
		// A failed `up --wait db` leaves the database container available just
		// long enough to inspect it. Reconcile removes that candidate during
		// rollback, so preserve its redacted tail in the durable operation error.
		logs, logErr := r.Logs(ctx, project, "db", 120)
		if logErr != nil {
			return err
		}
		if detail := boundedComposeDetail(logs); detail != "" {
			return fmt.Errorf("%w; db logs=%s", err, detail)
		}
		return err
	}
	return nil
}

// ResetDatabaseConfig removes only the Compose-managed configuration volume
// before a revision-zero bootstrap. Postgres 15 and 17 store incompatible
// generated configuration in this volume, so resetting only PGDATA is not
// sufficient after a failed PG15-to-PG17 first installation.
func (r *Runner) ResetDatabaseConfig(ctx context.Context, project ProjectRef) error {
	projectName := "supabase-manager-" + project.Slug
	filters := []string{
		"volume", "ls", "-q",
		"--filter", "label=com.docker.compose.project=" + projectName,
		"--filter", "label=com.docker.compose.volume=db-config",
	}
	output, err := r.executor.Run(ctx, "docker", filters, nil)
	if err != nil {
		return fmt.Errorf("list project database configuration volume: %w", err)
	}
	volumes := strings.Fields(string(output))
	if len(volumes) == 0 {
		return nil
	}
	if _, err := r.executor.Run(ctx, "docker", append([]string{"volume", "rm"}, volumes...), nil); err != nil {
		return fmt.Errorf("remove project database configuration volume: %w", err)
	}
	return nil
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
	args := append([]string{"up", "-d", "--force-recreate", "--remove-orphans", "--no-deps"}, services...)
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
	if err := r.run(ctx, project, "down", "--remove-orphans"); err == nil {
		return nil
	} else if fallbackErr := r.removeProjectResources(ctx, project); fallbackErr == nil {
		// Compose can fail before it evaluates the project (for example, when a
		// generated env_file is missing). The scoped label cleanup still removes
		// only this project's containers and networks, allowing a force delete to
		// finish without touching another Compose project.
		return nil
	} else {
		return fmt.Errorf("%w; project-scoped cleanup failed: %v", err, fallbackErr)
	}
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
		detail := boundedComposeDetail(output)
		if detail != "" {
			return fmt.Errorf("compose action failed: %w; output=%s", err, detail)
		}
		return fmt.Errorf("compose action failed: %w", err)
	}
	return nil
}

func boundedComposeDetail(output []byte) string {
	detail := redact.New(nil).String(strings.TrimSpace(string(output)))
	if len(detail) > 4096 {
		// Compose prints image pull progress before its actionable error. Keep
		// the tail so the registry, manifest, or daemon failure survives the
		// bounded operation error and reaches the Manager audit log.
		return "…" + detail[len(detail)-4096:]
	}
	return detail
}

func (r *Runner) removeProjectResources(ctx context.Context, project ProjectRef) error {
	projectName := "supabase-manager-" + project.Slug
	filter := "label=com.docker.compose.project=" + projectName
	output, err := r.executor.Run(ctx, "docker", []string{"ps", "-aq", "--filter", filter}, nil)
	if err != nil {
		return fmt.Errorf("list project containers: %w", err)
	}
	containerIDs := strings.Fields(string(output))
	if len(containerIDs) > 0 {
		args := append([]string{"rm", "-f"}, containerIDs...)
		if _, err := r.executor.Run(ctx, "docker", args, nil); err != nil {
			return fmt.Errorf("remove project containers: %w", err)
		}
	}

	networkOutput, err := r.executor.Run(ctx, "docker", []string{"network", "ls", "-q", "--filter", filter}, nil)
	if err != nil {
		return fmt.Errorf("list project networks: %w", err)
	}
	networkIDs := strings.Fields(string(networkOutput))
	if len(networkIDs) > 0 {
		args := append([]string{"network", "rm"}, networkIDs...)
		if _, err := r.executor.Run(ctx, "docker", args, nil); err != nil {
			return fmt.Errorf("remove project networks: %w", err)
		}
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
