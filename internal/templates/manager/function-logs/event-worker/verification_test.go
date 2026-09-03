package eventworker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedImageVerificationScriptIsBoundedAndChecksFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	script, err := os.ReadFile(filepath.Join(root, "scripts", "verify-edge-event-worker.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	for _, fragment := range []string{
		"supabase/edge-runtime:v1.74.0",
		"2781daf92394db91f7e94129cc3d04ec474ad16a8fe64b3fbeef6e7d557ab120",
		"--event-worker /event-worker",
		"FUNCTION_LOG_VERIFY_FIXTURES",
		"FUNCTION_LOG_FIXTURE_RECORDS=",
		"sleep 0.25",
		"run_bounded",
		"FUNCTION_LOG_EVENT_MANAGER_INERT",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("verification script missing %q", fragment)
		}
	}
	if strings.Contains(text, "|| true") {
		t.Fatal("verification script suppresses Docker operation failures")
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(makefile), "verify-edge-event-worker:") {
		t.Fatal("Makefile target verify-edge-event-worker is missing")
	}
}
