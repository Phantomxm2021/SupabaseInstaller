package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticFieldsOmitWhenUnset(t *testing.T) {
	responses := map[string]any{
		"error envelope":             ErrorEnvelope{Error: APIError{Code: "FAILED", Message: "failed"}},
		"reconcile response":         ReconcileProjectResponse{},
		"rotation response":          RotateDatabasePasswordResponse{},
		"function deployment result": FunctionDeploymentResult{},
		"managed TLS response":       StageManagedTLSResponse{},
	}
	for name, response := range responses {
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if strings.Contains(string(body), `"diagnostic"`) {
			t.Fatalf("%s unexpectedly included a diagnostic: %s", name, body)
		}
	}
}

func TestDiagnosticFieldsMarshalOnFailedOutcomes(t *testing.T) {
	responses := map[string]any{
		"error envelope":             ErrorEnvelope{Error: APIError{Code: "FAILED", Message: "failed"}, Diagnostic: "safe detail"},
		"reconcile response":         ReconcileProjectResponse{Error: &APIError{Code: "FAILED", Message: "failed"}, Diagnostic: "safe detail", DiagnosticVersion: DiagnosticVersionCompleteRedaction},
		"rotation response":          RotateDatabasePasswordResponse{Error: &APIError{Code: "FAILED", Message: "failed"}, Diagnostic: "safe detail", DiagnosticVersion: DiagnosticVersionCompleteRedaction},
		"function deployment result": FunctionDeploymentResult{Error: &APIError{Code: "FAILED", Message: "failed"}, Diagnostic: "safe detail"},
		"managed TLS response":       StageManagedTLSResponse{Diagnostic: "safe detail"},
	}
	for name, response := range responses {
		body, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if !strings.Contains(string(body), `"diagnostic":"safe detail"`) {
			t.Fatalf("%s did not include diagnostic: %s", name, body)
		}
		if (name == "reconcile response" || name == "rotation response") && !strings.Contains(string(body), `"diagnosticVersion":1`) {
			t.Fatalf("%s did not include diagnostic version: %s", name, body)
		}
	}
}

func TestDiagnosticVersionSupportIsExact(t *testing.T) {
	if !SupportsDiagnosticVersion(DiagnosticVersionCompleteRedaction) || SupportsDiagnosticVersion(0) || SupportsDiagnosticVersion(DiagnosticVersionCompleteRedaction+1) {
		t.Fatalf("unexpected diagnostic version support")
	}
}
