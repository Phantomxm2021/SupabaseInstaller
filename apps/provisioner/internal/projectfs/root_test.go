package projectfs

import "testing"

func TestProjectPathRejectsTraversalAndAbsoluteInput(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, slug := range []string{"../escape", "/tmp/escape", "bee/../../escape", "Bee", "bee_api"} {
		if _, err := root.ProjectPath(slug); err == nil {
			t.Errorf("ProjectPath(%q) succeeded, want rejection", slug)
		}
	}
}

func TestProjectPathReturnsContainedDirectory(t *testing.T) {
	base := t.TempDir()
	root, _ := New(base)
	path, err := root.ProjectPath("bee-2")
	if err != nil {
		t.Fatalf("ProjectPath() error = %v", err)
	}
	if path != base+"/bee-2" {
		t.Fatalf("ProjectPath() = %q, want %q", path, base+"/bee-2")
	}
}
