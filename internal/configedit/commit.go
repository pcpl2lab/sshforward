package configedit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/pcpl2lab/sshforward/internal/config"
)

// ErrChanged is returned by Commit when the file no longer matches what the
// form was built from.
var ErrChanged = errors.New("the config file changed while the form was open; nothing was written, run the command again")

// Expected returns before with oldName removed (when non-empty) and s added
// (when non-nil). before is not modified.
// Add: ("", &s). Edit: (name, &s). Rename: (old, &sWithNewName). Delete: (name, nil).
func Expected(before *config.Config, oldName string, s *Service) *config.Config {
	services := make(map[string]config.Service, len(before.Services)+1)
	for name, svc := range before.Services {
		services[name] = svc
	}
	if oldName != "" {
		delete(services, oldName)
	}
	if s != nil {
		services[s.Name] = s.Resolved()
	}
	return &config.Config{Services: services, UpdateCheck: before.UpdateCheck}
}

// ExpectedRename returns before with service oldName moved to newName,
// unchanged. A rename only rewrites the key, so the loaded service is the
// reference, not a re-resolved prefill. before is not modified.
func ExpectedRename(before *config.Config, oldName, newName string) *config.Config {
	services := make(map[string]config.Service, len(before.Services))
	for name, svc := range before.Services {
		services[name] = svc
	}
	if svc, ok := services[oldName]; ok {
		delete(services, oldName)
		services[newName] = svc
	}
	return &config.Config{Services: services, UpdateCheck: before.UpdateCheck}
}

// Commit replaces the config file with updated, but only if the file still
// holds original and updated loads to exactly want. The write goes through a
// temp file in the same directory, so the real file is never half-written.
// A symlinked config is followed, so the link itself survives.
func Commit(path string, original, updated []byte, want *config.Config) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || len(original) != 0 {
			return fmt.Errorf("cannot resolve config path: %w", err)
		}
		resolved = path // new file
	}

	current, err := os.ReadFile(resolved)
	if err != nil && !(errors.Is(err, os.ErrNotExist) && len(original) == 0) {
		return fmt.Errorf("cannot read config: %w", err)
	}
	if !bytes.Equal(current, original) {
		return ErrChanged
	}

	tmp, err := os.CreateTemp(filepath.Dir(resolved), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := writeTemp(tmp, updated); err != nil {
		os.Remove(tmpPath)
		return err
	}

	got, err := config.LoadFromPath(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("result would not load: %w; nothing was written", err)
	}
	if diff := describeDiff(got, want); diff != "" {
		os.Remove(tmpPath)
		return fmt.Errorf("result does not match the preview (%s); nothing was written", diff)
	}

	if err := os.Rename(tmpPath, resolved); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot replace config: %w", err)
	}
	return nil
}

// writeTemp fills the temp file, restricts it to the owner, flushes it to
// disk and closes it, so it can be loaded and renamed afterwards (Windows
// refuses to rename an open file) and a power loss cannot leave an empty
// config behind the rename.
func writeTemp(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot set permissions: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot write temp file: %w", err)
	}
	return nil
}

// describeDiff names what differs between got and want, or returns "".
func describeDiff(got, want *config.Config) string {
	var parts []string
	for _, name := range sortedNames(want.Services) {
		g, ok := got.Services[name]
		switch {
		case !ok:
			parts = append(parts, fmt.Sprintf("service %q missing", name))
		case !reflect.DeepEqual(g, want.Services[name]):
			parts = append(parts, fmt.Sprintf("service %q differs", name))
		}
	}
	for _, name := range sortedNames(got.Services) {
		if _, ok := want.Services[name]; !ok {
			parts = append(parts, fmt.Sprintf("service %q unexpected", name))
		}
	}
	if got.UpdateCheck != want.UpdateCheck {
		parts = append(parts, "update_check differs")
	}
	return strings.Join(parts, ", ")
}

func sortedNames(m map[string]config.Service) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
