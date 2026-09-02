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
		"reconcile response":         ReconcileProjectResponse{Error: &APIError{Code: "FAILED", Message: "failed"}, Diagnostic: "safe detail"},
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
	}
}
