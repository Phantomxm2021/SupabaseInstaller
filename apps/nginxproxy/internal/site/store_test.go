package site

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreApplyAtomicallyActivatesSite(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	runner := &recordingRunner{}
	store := NewStore(available, enabled, runner)
	rendered := RenderedSite{AvailableName: "supabase-manager-bee.conf", Contents: "new nginx config"}

	if err := store.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(available, rendered.AvailableName))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), rendered.Contents; got != want {
		t.Fatalf("available contents = %q, want %q", got, want)
	}
	target, err := os.Readlink(filepath.Join(enabled, rendered.AvailableName))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := target, filepath.Join(available, rendered.AvailableName); got != want {
		t.Fatalf("enabled target = %q, want %q", got, want)
	}
	if got, want := runner.calls, []string{"test", "reload"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runner calls = %#v, want %#v", got, want)
	}
}

func TestStoreApplyRestoresPreviousSiteWhenReloadFails(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	name := "supabase-manager-bee.conf"
	availablePath := filepath.Join(available, name)
	if err := os.WriteFile(availablePath, []byte("old nginx config"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(availablePath, filepath.Join(enabled, name)); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{reloadErrors: []error{errors.New("reload failed")}}
	store := NewStore(available, enabled, runner)
	err := store.Apply(context.Background(), RenderedSite{AvailableName: name, Contents: "new nginx config"})
	if err == nil {
		t.Fatal("Apply() error = nil, want reload error")
	}

	contents, readErr := os.ReadFile(availablePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := string(contents), "old nginx config"; got != want {
		t.Fatalf("available contents after rollback = %q, want %q", got, want)
	}
	if got, want := runner.calls, []string{"test", "reload", "test", "reload"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runner calls = %#v, want %#v", got, want)
	}
}

func TestStoreRemoveRestoresPreviousSiteWhenValidationFails(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	name := "supabase-manager-bee.conf"
	availablePath := filepath.Join(available, name)
	if err := os.WriteFile(availablePath, []byte("old nginx config"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(availablePath, filepath.Join(enabled, name)); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{testErrors: []error{errors.New("nginx -t failed")}}
	store := NewStore(available, enabled, runner)
	err := store.Remove(context.Background(), name)
	if err == nil {
		t.Fatal("Remove() error = nil, want validation error")
	}

	if _, err := os.Stat(availablePath); err != nil {
		t.Fatalf("available config after rollback: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(enabled, name)); err != nil {
		t.Fatalf("enabled link after rollback: %v", err)
	}
	if got, want := runner.calls, []string{"test", "test", "reload"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runner calls = %#v, want %#v", got, want)
	}
}

type recordingRunner struct {
	calls        []string
	testErrors   []error
	reloadErrors []error
}

func (r *recordingRunner) Test(context.Context) error {
	r.calls = append(r.calls, "test")
	return r.next(&r.testErrors)
}

func (r *recordingRunner) Reload(context.Context) error {
	r.calls = append(r.calls, "reload")
	return r.next(&r.reloadErrors)
}

func (r *recordingRunner) next(errors *[]error) error {
	if len(*errors) == 0 {
		return nil
	}
	err := (*errors)[0]
	*errors = (*errors)[1:]
	return err
}
