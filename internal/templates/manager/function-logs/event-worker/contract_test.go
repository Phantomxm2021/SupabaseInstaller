package eventworker

import (
	"os"
	"path/filepath"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestPinnedEventFixturesCarryExactFunctionAttribution(t *testing.T) {
	for _, name := range []string{"log-event.json", "uncaught-exception.json", "boot-event.json"} {
		raw, err := os.ReadFile(filepath.Join("fixtures", name))
		if err != nil {
			t.Fatal(err)
		}

		event, err := contracts.ParseEdgeRuntimeEvent(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if event.FunctionName == "" || event.EventID == "" {
			t.Fatalf("%s lacks exact attribution: %#v", name, event)
		}
	}
}
