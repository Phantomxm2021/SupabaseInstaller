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
	auth := t.TempDir()
	runner := &recordingRunner{}
	store := NewStore(available, enabled, auth, runner)
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

func TestStoreApplyWritesRootOnlyStudioCredentialsAndRemoveDeletesThem(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	auth := t.TempDir()
	store := NewStore(available, enabled, auth, &recordingRunner{})
	rendered := RenderedSite{
		AvailableName: "supabase-manager-studio.conf",
		Contents:      "new nginx config",
		AuthDirectory: auth,
		AuthFileName:  "supabase-manager-studio.htpasswd",
		AuthContents:  "operator:$apr1$salt$Xxd1irWT9ycqoYxGFn4cb.\n",
	}
	if err := store.Apply(context.Background(), rendered); err != nil {
		t.Fatal(err)
	}

	credentialPath := filepath.Join(auth, rendered.AuthFileName)
	info, err := os.Stat(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("credential mode = %#o, want %#o", got, want)
	}
	contents, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), rendered.AuthContents; got != want {
		t.Fatalf("credential content = %q, want %q", got, want)
	}

	if err := store.Remove(context.Background(), rendered.AvailableName); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(credentialPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credential after remove error = %v, want not exist", err)
	}
}

func TestStoreApplyRestoresPreviousSiteWhenReloadFails(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	auth := t.TempDir()
	name := "supabase-manager-bee.conf"
	availablePath := filepath.Join(available, name)
	if err := os.WriteFile(availablePath, []byte("old nginx config"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(availablePath, filepath.Join(enabled, name)); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{reloadErrors: []error{errors.New("reload failed")}}
	store := NewStore(available, enabled, auth, runner)
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

func TestStoreApplyRestoresPreviousStudioCredentialsWhenReloadFails(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	auth := t.TempDir()
	name := "supabase-manager-studio.conf"
	authName := "supabase-manager-studio.htpasswd"
	availablePath := filepath.Join(available, name)
	credentialPath := filepath.Join(auth, authName)
	if err := os.WriteFile(availablePath, []byte("old nginx config"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialPath, []byte("operator:$apr1$old$oldhash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(availablePath, filepath.Join(enabled, name)); err != nil {
		t.Fatal(err)
	}

	store := NewStore(available, enabled, auth, &recordingRunner{reloadErrors: []error{errors.New("reload failed")}})
	err := store.Apply(context.Background(), RenderedSite{
		AvailableName: name, Contents: "new nginx config", AuthDirectory: auth,
		AuthFileName: authName, AuthContents: "operator:$apr1$new$newhash\n",
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want reload error")
	}
	contents, readErr := os.ReadFile(credentialPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := string(contents), "operator:$apr1$old$oldhash\n"; got != want {
		t.Fatalf("credential after rollback = %q, want %q", got, want)
	}
}

func TestStoreRemoveRestoresPreviousSiteWhenValidationFails(t *testing.T) {
	available := t.TempDir()
	enabled := t.TempDir()
	auth := t.TempDir()
	name := "supabase-manager-bee.conf"
	availablePath := filepath.Join(available, name)
	if err := os.WriteFile(availablePath, []byte("old nginx config"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(availablePath, filepath.Join(enabled, name)); err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{testErrors: []error{errors.New("nginx -t failed")}}
	store := NewStore(available, enabled, auth, runner)
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
