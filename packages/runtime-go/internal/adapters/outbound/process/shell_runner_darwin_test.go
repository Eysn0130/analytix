//go:build darwin

package process

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestCachePrivateTreeEnforcesEntryPolicy(t *testing.T) {
	candidate := cachePrivateTreeFunction(t, repositoryScriptPath(t, "use-analytix-cache.sh"))
	var baseline string
	if path := os.Getenv("ANALYTIX_CACHE_STAT_BASELINE_HELPER"); path != "" {
		baseline = cachePrivateTreeFunction(t, path)
	}
	cases := []struct {
		name, mode, mismatch string
		fileMode             os.FileMode
		kind                 string
		reject               bool
	}{
		{name: "empty", mode: "output"},
		{name: "hidden whitespace output", mode: "output", fileMode: 0600},
		{name: "executable output", mode: "output", fileMode: 0700},
		{name: "output hardlink", mode: "output", fileMode: 0600, kind: "hardlink"},
		{name: "hidden whitespace evidence", mode: "evidence", fileMode: 0600},
		{name: "evidence hardlink", mode: "evidence", fileMode: 0600, kind: "hardlink"},
		{name: "hidden whitespace native binary", mode: "native", fileMode: 0700},
		{name: "native metadata", mode: "native", fileMode: 0600, kind: "metadata"},
		{name: "native lock metadata", mode: "native", fileMode: 0600, kind: "lock"},
		{name: "retired native lock metadata", mode: "native", fileMode: 0600, kind: "retired"},
		{name: "released native lock metadata", mode: "native", fileMode: 0600, kind: "released"},
		{name: "symlink", mode: "output", fileMode: 0600, kind: "symlink", reject: true},
		{name: "dangling symlink", mode: "output", kind: "dangling", reject: true},
		{name: "special file", mode: "output", kind: "fifo", reject: true},
		{name: "wrong directory mode", mode: "output", kind: "directory", reject: true},
		{name: "wrong output mode", mode: "output", fileMode: 0640, reject: true},
		{name: "wrong evidence mode", mode: "evidence", fileMode: 0700, reject: true},
		{name: "wrong native binary mode", mode: "native", fileMode: 0600, reject: true},
		{name: "wrong native metadata mode", mode: "native", fileMode: 0700, kind: "metadata", reject: true},
		{name: "native hardlink", mode: "native", fileMode: 0700, kind: "hardlink", reject: true},
		{name: "owner mismatch", mode: "output", fileMode: 0600, mismatch: "owner", reject: true},
		{name: "device mismatch", mode: "output", fileMode: 0600, mismatch: "device", reject: true},
		{name: "noncanonical entry", mode: "output", fileMode: 0600, kind: "noncanonical", reject: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := cachePrivateTreeFixtureRoot(t)
			entry := filepath.Join(root, ".hidden directory with spaces", ".file with\twhitespace\nand newline")
			switch tc.kind {
			case "metadata":
				entry = filepath.Join(root, "analytix-native-development-build.json")
			case "lock", "retired", "released":
				suffix := ""
				if tc.kind != "lock" {
					suffix = "." + tc.kind
				}
				entry = filepath.Join(root, "component.build.lock"+suffix, "owner.json")
			}
			if tc.fileMode != 0 {
				writeCachePrivateTreeFixture(t, entry, tc.fileMode)
			}
			var err error
			switch tc.kind {
			case "symlink", "dangling":
				err = os.Symlink(entry, filepath.Join(root, ".symlink with spaces"))
			case "fifo":
				err = syscall.Mkfifo(filepath.Join(root, ".special file"), 0600)
			case "directory":
				path := filepath.Join(root, ".unsafe directory")
				if err = os.Mkdir(path, 0700); err == nil {
					err = os.Chmod(path, 0750)
				}
			case "hardlink":
				err = os.Link(entry, filepath.Join(root, ".second link"))
			case "noncanonical":
				err = os.Mkdir(filepath.Join(root, "sub"), 0700)
				root += "/sub/.."
			}
			if err != nil {
				t.Fatalf("prepare private-tree fixture: %v", err)
			}
			result := runCachePrivateTreeFixture(t, candidate, root, tc.mode, tc.mismatch, nil)
			if (result.exit != 0) != tc.reject {
				t.Fatalf("candidate exit=%d, want rejection=%t", result.exit, tc.reject)
			}
			if baseline != "" {
				before := runCachePrivateTreeFixture(t, baseline, root, tc.mode, tc.mismatch, nil)
				if before.exit != result.exit {
					t.Fatalf("baseline/candidate rejection drift: before=%d after=%d", before.exit, result.exit)
				}
			}
		})
	}
}

func TestCachePrivateTreeStatRecordFailsClosed(t *testing.T) {
	candidate := cachePrivateTreeFunction(t, repositoryScriptPath(t, "use-analytix-cache.sh"))
	root := cachePrivateTreeFixtureRoot(t)
	writeCachePrivateTreeFixture(t, filepath.Join(root, ".native binary"), 0700)
	device, uid := cachePrivateTreeFixtureIdentity(t, root)
	valid := device + ":" + uid + ":700:1"
	cases := []struct {
		name, record string
		status       int
	}{
		{name: "empty"},
		{name: "truncated", record: device + ":" + uid + ":700"},
		{name: "extra field", record: valid + ":0"},
		{name: "nonnumeric device", record: "device:" + uid + ":700:1"},
		{name: "signed owner", record: device + ":+" + uid + ":700:1"},
		{name: "invalid octal", record: device + ":" + uid + ":780:1"},
		{name: "empty field", record: device + "::700:1"},
		{name: "whitespace", record: " " + valid},
		{name: "embedded newline", record: valid + "\n" + valid},
		{name: "nonnumeric links", record: device + ":" + uid + ":700:links"},
		{name: "failed stat with valid output", record: valid, status: 1},
	}
	// Fault injection is confined to this extracted test function. Policy/parity
	// fixtures above and the call-count fixture below use the real /usr/bin/stat.
	candidate = strings.ReplaceAll(candidate, "/usr/bin/stat", "analytix_cache_test_stat")
	control := cachePrivateTreeStatFault{record: valid}
	if result := runCachePrivateTreeFixture(t, candidate, root, "native", "", &control); result.exit != 0 {
		t.Fatalf("valid stat-record control failed: exit=%d", result.exit)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fault := cachePrivateTreeStatFault{record: tc.record, status: tc.status}
			if result := runCachePrivateTreeFixture(t, candidate, root, "native", "", &fault); result.exit == 0 {
				t.Fatal("malformed or failed stat record unexpectedly passed")
			}
		})
	}
}

func TestCachePrivateTreeUsesOneStatPerEntry(t *testing.T) {
	candidate := cachePrivateTreeFunction(t, repositoryScriptPath(t, "use-analytix-cache.sh"))
	root := cachePrivateTreeFixtureRoot(t)
	for _, tree := range []string{"bin", "evidence", "native-components-development"} {
		for index := 0; index < 12; index++ {
			mode := os.FileMode(0600)
			if tree == "native-components-development" || tree == "bin" && index%2 == 0 {
				mode = 0700
			}
			writeCachePrivateTreeFixture(t, filepath.Join(root, tree, ".hidden directory", fmt.Sprintf(".file %02d\twith\nwhitespace", index)), mode)
		}
	}
	nativeRoot := filepath.Join(root, "native-components-development")
	writeCachePrivateTreeFixture(t, filepath.Join(nativeRoot, "analytix-native-development-build.json"), 0600)
	for _, suffix := range []string{"", ".retired", ".released"} {
		writeCachePrivateTreeFixture(t, filepath.Join(nativeRoot, "component.build.lock"+suffix, "owner.json"), 0600)
	}
	entries, nativeFiles := 0, 0
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && filepath.Dir(path) != root {
			entries++
			if strings.HasPrefix(path, nativeRoot+string(filepath.Separator)) && entry.Type().IsRegular() {
				nativeFiles++
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("count bounded fixture entries: %v", err)
	}
	if entries != 46 {
		t.Fatalf("bounded fixture entry count drifted: %d", entries)
	}
	// When supplied for a delivery comparison, run the untouched baseline once
	// before the candidate on the same quiescent fixture; normal CI needs no Git history.
	if path := os.Getenv("ANALYTIX_CACHE_STAT_BASELINE_HELPER"); path != "" {
		baseline := cachePrivateTreeFunction(t, path)
		before := runCachePrivateTreeFixture(t, baseline, root, "all", "", nil)
		if before.exit != 0 || before.statCalls != 3*entries+nativeFiles {
			t.Fatalf("baseline fixture exit=%d stat_calls=%d, expected=%d", before.exit, before.statCalls, 3*entries+nativeFiles)
		}
		t.Logf("baseline entries=%d stat_calls=%d wall_ms=%.3f exit=%d", entries, before.statCalls, float64(before.wall)/float64(time.Millisecond), before.exit)
	}
	after := runCachePrivateTreeFixture(t, candidate, root, "all", "", nil)
	if after.exit != 0 || after.statCalls != entries {
		t.Fatalf("candidate fixture exit=%d stat_calls=%d, expected=%d", after.exit, after.statCalls, entries)
	}
	t.Logf("candidate entries=%d stat_calls=%d wall_ms=%.3f exit=%d", entries, after.statCalls, float64(after.wall)/float64(time.Millisecond), after.exit)
}

func cachePrivateTreeFunction(t *testing.T, helperPath string) string {
	t.Helper()
	body, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatalf("read cache helper: %v", err)
	}
	start := strings.Index(string(body), "analytix_cache_validate_private_tree() {\n")
	if start < 0 {
		t.Fatal("private-tree validator is absent")
	}
	end := strings.Index(string(body)[start:], "\n}\n")
	if end < 0 {
		t.Fatal("private-tree validator is incomplete")
	}
	return string(body)[start : start+end+2]
}

func cachePrivateTreeFixtureRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve private-tree fixture: %v", err)
	}
	return root
}

func writeCachePrivateTreeFixture(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("create private fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("synthetic\n"), 0600); err != nil {
		t.Fatalf("create private fixture file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set private fixture file mode: %v", err)
	}
}

func cachePrivateTreeFixtureIdentity(t *testing.T, root string) (string, string) {
	t.Helper()
	info, err := os.Stat(root)
	if err != nil {
		t.Fatalf("inspect private fixture: %v", err)
	}
	return fmt.Sprint(info.Sys().(*syscall.Stat_t).Dev), fmt.Sprint(os.Geteuid())
}

type cachePrivateTreeStatFault struct {
	record string
	status int
}

type cachePrivateTreeResult struct {
	exit, statCalls int
	wall            time.Duration
}

func runCachePrivateTreeFixture(t *testing.T, function, root, mode, mismatch string, fault *cachePrivateTreeStatFault) cachePrivateTreeResult {
	t.Helper()
	device, uid := cachePrivateTreeFixtureIdentity(t, root)
	if mismatch == "owner" {
		uid = fmt.Sprint(os.Geteuid() + 1)
	} else if mismatch == "device" {
		device += "0"
	}
	script := `emulate -L zsh
unsetopt xtrace
analytix_cache_mount_device_id="$3"
analytix_cache_effective_uid="$4"
analytix_cache_fail() { print -r -- "$1" >&2 }
` + function + `
TRAPDEBUG() {
  if [[ "$ZSH_DEBUG_CMD" == '/usr/bin/stat -f '* ]]; then
    print -r -- '__analytix_stat_call__' >&2
  fi
}
setopt DEBUG_BEFORE_CMD
`
	record, statStatus := "", "0"
	if fault != nil {
		record, statStatus = fault.record, fmt.Sprint(fault.status)
		script += `analytix_cache_test_stat_output="$5"
analytix_cache_test_stat_status="$6"
analytix_cache_test_stat() { print -r -- "$analytix_cache_test_stat_output"; return "$analytix_cache_test_stat_status" }
`
	}
	script += `if [ "$2" = 'all' ]; then
  analytix_cache_validate_private_tree "$1/bin" 'output' &&
    analytix_cache_validate_private_tree "$1/evidence" 'evidence' &&
    analytix_cache_validate_private_tree "$1/native-components-development" 'native'
else
  analytix_cache_validate_private_tree "$1" "$2"
fi`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/zsh", "-f", "-c", script, "analytix-private-tree-test", root, mode, device, uid, record, statStatus)
	var stderr bytes.Buffer
	command.Stdout, command.Stderr = io.Discard, &stderr
	started := time.Now()
	err := command.Run()
	result := cachePrivateTreeResult{wall: time.Since(started)}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if line == "__analytix_stat_call__" {
			result.statCalls++
		}
	}
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || ctx.Err() != nil {
			t.Fatalf("run private-tree validator: %v", err)
		}
		result.exit = exitError.ExitCode()
	}
	return result
}
