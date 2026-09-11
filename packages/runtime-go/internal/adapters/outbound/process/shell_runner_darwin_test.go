//go:build darwin

package process

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/ports"
)

func TestShellRunnerScopesStorageMetadataDeputyToSystemProfiler(t *testing.T) {
	runner := NewShellRunner()
	policy := []string{t.TempDir()}
	profiler := runner.RunShell(context.Background(), ports.ShellRequest{
		Command:           "LC_ALL=C /usr/sbin/system_profiler SPStorageDataType -json -detailLevel mini -timeout 15 >/dev/null",
		Dir:               t.TempDir(),
		Output:            io.Discard,
		ProtectedReadDirs: policy,
	})
	if profiler.StartFailed || profiler.Canceled || profiler.Error != "" || profiler.ExitCode != 0 {
		t.Fatalf("nested read-only storage profiler failed: %#v", profiler)
	}

	diskutil := runner.RunShell(context.Background(), ports.ShellRequest{
		Command:           "/usr/sbin/diskutil info -plist / >/dev/null",
		Dir:               t.TempDir(),
		Output:            io.Discard,
		ProtectedReadDirs: policy,
	})
	if diskutil.StartFailed || diskutil.Canceled || diskutil.Error == "" || diskutil.ExitCode == 0 {
		t.Fatalf("nested diskutil unexpectedly reached Disk Arbitration: %#v", diskutil)
	}
}

func TestShellRunnerRunsConfiguredCachePreflightInsideContainment(t *testing.T) {
	const expectedCacheRoot = "/Volumes/AnalytixCache/development-v3"
	if os.Getenv("ANALYTIX_DEV_CACHE_ROOT") != expectedCacheRoot {
		t.Skip("requires the configured Analytix cache environment")
	}

	helperPath := repositoryScriptPath(t, "use-analytix-cache.sh")
	helperInfo, err := os.Lstat(helperPath)
	if err != nil {
		t.Fatalf("inspect cache helper: %v", err)
	}
	if !helperInfo.Mode().IsRegular() || helperInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("cache helper is not a regular repository file: %s", helperPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result := NewShellRunner().RunShell(ctx, ports.ShellRequest{
		Command: ". " + shellQuoteForTest(helperPath) +
			" && test \"$ANALYTIX_DEV_CACHE_ROOT\" = " + shellQuoteForTest(expectedCacheRoot),
		Dir:               t.TempDir(),
		Output:            io.Discard,
		ProtectedReadDirs: []string{t.TempDir()},
	})
	if result.StartFailed || result.Canceled || result.Error != "" || result.ExitCode != 0 {
		t.Fatalf("contained cache preflight failed: %#v", result)
	}
}

func TestShellRunnerCachePreflightDoesNotTraceStorageSnapshot(t *testing.T) {
	const expectedCacheRoot = "/Volumes/AnalytixCache/development-v3"
	if os.Getenv("ANALYTIX_DEV_CACHE_ROOT") != expectedCacheRoot {
		t.Skip("requires the configured Analytix cache environment")
	}

	helperPath := repositoryScriptPath(t, "use-analytix-cache.sh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var output bytes.Buffer
	result := NewShellRunner().RunShell(ctx, ports.ShellRequest{
		Command: "set -x; . " + shellQuoteForTest(helperPath) +
			"; analytix_helper_status=$?; set +x; test $analytix_helper_status = 0 && test \"$ANALYTIX_DEV_CACHE_ROOT\" = " +
			shellQuoteForTest(expectedCacheRoot),
		Dir:               t.TempDir(),
		Output:            &output,
		ProtectedReadDirs: []string{t.TempDir()},
	})
	outputHash := sha256.Sum256(output.Bytes())
	if result.StartFailed || result.Canceled || result.Error != "" || result.ExitCode != 0 {
		t.Fatalf(
			"xtrace-contained cache preflight failed: result=%#v outputBytes=%d outputSha256=%x",
			result,
			output.Len(),
			outputHash,
		)
	}
	for _, privateMarker := range []string{"SPStorageDataType", "physical_drive", "volume_uuid"} {
		if bytes.Contains(output.Bytes(), []byte(privateMarker)) {
			t.Fatalf(
				"xtrace exposed storage metadata marker %q: outputBytes=%d outputSha256=%x",
				privateMarker,
				output.Len(),
				outputHash,
			)
		}
	}
}

func TestCacheStorageMetadataParserFailsClosed(t *testing.T) {
	libraryPath := repositoryScriptPath(t, "analytix-cache-storage.zsh")
	validFixture := `{"SPStorageDataType":[{"mount_point":"/Volumes/Other"},{"mount_point":"/Volumes/AnalytixCache","bsd_name":"disk9s1","file_system":"apfs"}]}`
	validScript := `index=$(analytix_cache_storage_index /Volumes/AnalytixCache) && ` +
		`[ "$index" = 1 ] && ` +
		`[ "$(analytix_cache_storage_field "$index" bsd_name)" = disk9s1 ] && ` +
		`filesystem=$(analytix_cache_storage_field "$index" file_system) && ` +
		`[ "${filesystem:u}" = APFS ]`
	if result := runCacheStorageFixture(t, libraryPath, validFixture, validScript); result != 0 {
		t.Fatalf("valid storage metadata with an unnamed preceding item failed: exit=%d", result)
	}

	oversizedItems := make([]map[string]string, 129)
	for index := range oversizedItems {
		oversizedItems[index] = map[string]string{"mount_point": "/Volumes/Other"}
	}
	oversizedFixture, err := json.Marshal(map[string]any{"SPStorageDataType": oversizedItems})
	if err != nil {
		t.Fatalf("encode oversized fixture: %v", err)
	}
	failureCases := []struct {
		name    string
		fixture string
		script  string
	}{
		{
			name:    "duplicate mount",
			fixture: `{"SPStorageDataType":[{"mount_point":"/Volumes/AnalytixCache"},{"mount_point":"/Volumes/AnalytixCache"}]}`,
			script:  `analytix_cache_storage_index /Volumes/AnalytixCache >/dev/null`,
		},
		{
			name:    "missing required field",
			fixture: `{"SPStorageDataType":[{"mount_point":"/Volumes/AnalytixCache"}]}`,
			script:  `analytix_cache_storage_field 0 bsd_name >/dev/null 2>&1`,
		},
		{
			name:    "missing storage array",
			fixture: `{}`,
			script:  `analytix_cache_storage_index /Volumes/AnalytixCache >/dev/null`,
		},
		{
			name:    "malformed storage json",
			fixture: `{"SPStorageDataType":[`,
			script:  `analytix_cache_storage_index /Volumes/AnalytixCache >/dev/null`,
		},
		{
			name:    "scalar storage value",
			fixture: `{"SPStorageDataType":"not-an-array"}`,
			script:  `analytix_cache_storage_index /Volumes/AnalytixCache >/dev/null`,
		},
		{
			name:    "empty storage array",
			fixture: `{"SPStorageDataType":[]}`,
			script:  `analytix_cache_storage_index /Volumes/AnalytixCache >/dev/null`,
		},
		{
			name:    "oversized storage array",
			fixture: string(oversizedFixture),
			script:  `analytix_cache_storage_index /Volumes/AnalytixCache >/dev/null`,
		},
	}
	for _, testCase := range failureCases {
		t.Run(testCase.name, func(t *testing.T) {
			if result := runCacheStorageFixture(t, libraryPath, testCase.fixture, testCase.script); result == 0 {
				t.Fatal("unsafe storage metadata unexpectedly passed")
			}
		})
	}
}

func repositoryScriptPath(t *testing.T, name string) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve package directory: %v", err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(packageDir, "../../../../../.."))
	return filepath.Join(repositoryRoot, "scripts", name)
}

func runCacheStorageFixture(t *testing.T, libraryPath string, fixture string, script string) int {
	t.Helper()
	command := exec.Command("/bin/zsh", "-c", `. "$1" || exit 64
analytix_cache_storage_info=$(/bin/cat)
eval "$2"`, "analytix-cache-storage-test", libraryPath, script)
	command.Stdin = bytes.NewBufferString(fixture)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run storage metadata parser: %v", err)
		}
		return exitError.ExitCode()
	}
	return 0
}
