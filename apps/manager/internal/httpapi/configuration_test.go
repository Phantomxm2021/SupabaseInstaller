package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"supabase-manager/apps/manager/internal/store"
)

func TestConfigurationBusyIsTypedConflict(t *testing.T) {
	response := httptest.NewRecorder()
	h := configurationHandlers{}
	h.handleConfigError(response, store.ErrConfigurationBusy)
	if response.Code != http.StatusConflict {
		t.Fatalf("busy status = %d, want 409", response.Code)
	}
	if strings.Contains(response.Body.String(), "project configuration operation is busy") {
		t.Fatalf("busy response leaked internal sentinel: %s", response.Body.String())
	}
}

func TestDecodeRawRejectsTrailingJSON(t *testing.T) {
	var value map[string]any
	err := decodeRaw([]byte(`{"services":{}} {"ignored":true}`), &value)
	if err == nil {
		t.Fatalf("decodeRaw trailing value error = %v", err)
	}
}
