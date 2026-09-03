package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/auth"
	managerfunctions "supabase-manager/apps/manager/internal/functions"
	"supabase-manager/apps/manager/internal/project"
	"supabase-manager/apps/manager/internal/provisioner"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type logsProvisionerFake struct {
	page contracts.FunctionLogPage
	err  error
}

func (*logsProvisionerFake) DeployFunction(context.Context, string, string, string, io.Reader) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, nil
}
func (*logsProvisionerFake) ListFunctions(context.Context, string) ([]contracts.FunctionSummary, error) {
	return nil, nil
}
func (f *logsProvisionerFake) FunctionLogs(context.Context, string, string, contracts.FunctionLogQuery) (contracts.FunctionLogPage, error) {
	return f.page, f.err
}
func (*logsProvisionerFake) RollbackFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, nil
}
func (*logsProvisionerFake) DeleteFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, nil
}

func logsHandler(t *testing.T, fake *logsProvisionerFake) http.Handler {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	p := contracts.Project{ID: "p-1", Name: "Bee", Slug: "bee", Services: contracts.Services{Functions: true}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), p, contracts.ProjectConfiguration{General: contracts.GeneralConfig{Domain: "bee.example.com"}, Services: p.Services}); err != nil {
		t.Fatal(err)
	}
	projects := project.NewService(database, func() string { return "unused" }, time.Now)
	service := managerfunctions.NewService(nil, nil, nil, fake, time.Now)
	mux := http.NewServeMux()
	RegisterFunctionsRoutes(mux, FunctionsOptions{Service: service, Projects: projects})
	return mux
}

func TestFunctionLogsPublicRouteMappings(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		fake       *logsProvisionerFake
		want       int
		code       string
	}{
		{"success", "/api/projects/p-1/functions/demo/logs", &logsProvisionerFake{page: contracts.FunctionLogPage{Logs: []contracts.FunctionLogRecord{}, Health: contracts.FunctionLogHealth{Status: "healthy"}}}, 200, `"status":"healthy"`},
		{"invalid query", "/api/projects/p-1/functions/demo/logs?limit=nope", &logsProvisionerFake{}, 400, "INVALID_FUNCTION_LOG_QUERY"},
		{"invalid name", "/api/projects/p-1/functions/Bad/logs", &logsProvisionerFake{}, 400, "INVALID_FUNCTION_NAME"},
		{"missing project", "/api/projects/missing/functions/demo/logs", &logsProvisionerFake{}, 404, "PROJECT_NOT_FOUND"},
		{"unknown function", "/api/projects/p-1/functions/demo/logs", &logsProvisionerFake{err: &provisioner.ClientError{Code: "FUNCTION_NOT_FOUND", Status: 404}}, 404, "FUNCTION_NOT_FOUND"},
		{"unavailable", "/api/projects/p-1/functions/demo/logs", &logsProvisionerFake{err: errors.New("/private/secret SQL")}, 502, "FUNCTION_LOGS_UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			logsHandler(t, tc.fake).ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.want || !strings.Contains(response.Body.String(), tc.code) || strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "SQL") {
				t.Fatalf("response=%d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestFunctionLogsRouteRequiresSessionThroughRouter(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	authService := auth.NewService(database, auth.NewPasswordHasher(auth.Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}), strings.NewReader(strings.Repeat("x", 4096)), time.Now)
	handler := NewRouter(RouterOptions{Auth: AuthOptions{Service: authService}, Functions: managerfunctions.NewService(nil, nil, nil, &logsProvisionerFake{}, time.Now), Projects: ProjectOptions{Projects: project.NewService(database, func() string { return "p" }, time.Now)}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/projects/p/functions/demo/logs", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
