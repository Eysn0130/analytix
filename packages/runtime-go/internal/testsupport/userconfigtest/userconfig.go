package userconfigtest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const isolatedRootEnvironment = "ANALYTIX_GO_TEST_USER_CONFIG_ROOT"

// Run executes one Go test package with an isolated per-process user-config
// root so startup-authority tests cannot mutate a developer's real profile.
func Run(m *testing.M) {
	if inherited := os.Getenv(isolatedRootEnvironment); inherited != "" {
		if !validInheritedRoot(inherited) {
			fmt.Fprintln(os.Stderr, "analytix inherited test user-config isolation is invalid")
			os.Exit(2)
		}
		os.Exit(m.Run())
	}
	isolation, err := newIsolation()
	if err != nil {
		fmt.Fprintln(os.Stderr, "analytix test user-config isolation failed")
		os.Exit(2)
	}
	for name, value := range isolation.environment {
		if err := os.Setenv(name, value); err != nil {
			_ = isolation.cleanup()
			fmt.Fprintln(os.Stderr, "analytix test user-config isolation failed")
			os.Exit(2)
		}
	}

	code := m.Run()
	if err := isolation.cleanup(); err != nil && code == 0 {
		fmt.Fprintln(os.Stderr, "analytix test user-config cleanup failed")
		code = 2
	}
	os.Exit(code)
}

type isolation struct {
	base        string
	environment map[string]string
}

func newIsolation() (*isolation, error) {
	originalHome := os.Getenv("HOME")
	originalUserCache, _ := os.UserCacheDir()
	// macOS commonly supplies /var/... as TMPDIR while its real ancestor is
	// /private/var. Fixtures must use canonical paths before strict no-symlink
	// production opens; do not relax those opens to accommodate a test alias.
	temporaryParent, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return nil, err
	}
	base, err := os.MkdirTemp(temporaryParent, "analytix-go-test-user-config-")
	if err != nil {
		return nil, err
	}
	removeOnError := true
	defer func() {
		if removeOnError {
			_ = os.RemoveAll(base)
		}
	}()
	home := filepath.Join(base, "home")
	config := filepath.Join(base, "config")
	temporary := filepath.Join(base, "tmp")
	if err := os.Mkdir(home, 0o700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(config, 0o700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(temporary, 0o700); err != nil {
		return nil, err
	}
	environment := map[string]string{
		isolatedRootEnvironment: base,
		"HOME":                  home,
		"XDG_CONFIG_HOME":       config,
		"APPDATA":               config,
		"LOCALAPPDATA":          config,
		"USERPROFILE":           home,
		"TMPDIR":                temporary,
		"TMP":                   temporary,
		"TEMP":                  temporary,
		"GOTMPDIR":              temporary,
	}
	// Some root-package tests launch `go run` to exercise the real command.
	// Preserve the caller's Go caches before HOME changes so the nested Go tool
	// cannot populate the disposable user-config root or race its cleanup.
	if os.Getenv("GOPATH") == "" && originalHome != "" {
		environment["GOPATH"] = filepath.Join(originalHome, "go")
	}
	if os.Getenv("GOMODCACHE") == "" && originalHome != "" {
		environment["GOMODCACHE"] = filepath.Join(originalHome, "go", "pkg", "mod")
	}
	if os.Getenv("GOCACHE") == "" && originalUserCache != "" {
		environment["GOCACHE"] = filepath.Join(originalUserCache, "go-build")
	}
	if os.Getenv("GOTELEMETRY") == "" {
		environment["GOTELEMETRY"] = "off"
	}
	removeOnError = false
	return &isolation{base: base, environment: environment}, nil
}

func (i *isolation) cleanup() error {
	if i == nil || i.base == "" {
		return nil
	}
	return os.RemoveAll(i.base)
}

// CreateChildEnvironment creates one isolated user-config root for a tracked
// test child. The caller owns cleanup and must call it after the complete child
// process tree has exited, including failure and cancellation paths.
func CreateChildEnvironment(base []string) ([]string, func() error, error) {
	isolation, err := newIsolation()
	if err != nil {
		return nil, nil, err
	}
	overrides := isolation.environment
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := overrides[name]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+overrides[name])
	}
	return result, isolation.cleanup, nil
}

func validInheritedRoot(base string) bool {
	clean := filepath.Clean(base)
	temporary := filepath.Join(clean, "tmp")
	if filepath.Clean(os.TempDir()) != temporary {
		return false
	}
	expectedPaths := map[string]string{
		"HOME":            filepath.Join(clean, "home"),
		"USERPROFILE":     filepath.Join(clean, "home"),
		"XDG_CONFIG_HOME": filepath.Join(clean, "config"),
		"APPDATA":         filepath.Join(clean, "config"),
		"LOCALAPPDATA":    filepath.Join(clean, "config"),
	}
	for name, expected := range expectedPaths {
		if filepath.Clean(os.Getenv(name)) != expected {
			return false
		}
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GOTMPDIR"} {
		if filepath.Clean(os.Getenv(name)) != temporary {
			return false
		}
	}
	for _, path := range []string{clean, filepath.Join(clean, "home"), filepath.Join(clean, "config"), temporary} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return false
		}
	}
	return true
}

func pathWithinTemporaryRoot(value string) bool {
	clean := filepath.Clean(value)
	temporary, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(temporary, clean)
	return err == nil && filepath.IsAbs(clean) && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
