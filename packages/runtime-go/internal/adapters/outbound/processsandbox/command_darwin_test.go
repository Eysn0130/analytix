//go:build darwin

package processsandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	delegatedProcessChildModeEnv    = "ANALYTIX_PROCESS_SANDBOX_DEPUTY_CHILD_V1"
	delegatedProcessChildAddressEnv = "ANALYTIX_PROCESS_SANDBOX_DEPUTY_ADDRESS_V1"
	delegatedProcessChildAllowEnv   = "ANALYTIX_PROCESS_SANDBOX_DEPUTY_ALLOW_V1"
)

func TestCommandContextDeniesProtectedFilesystemAccessAndAllowsOrdinaryWork(t *testing.T) {
	assertProtectedFilesystemAccessAndOrdinaryWork(t, t.TempDir())
}

// Production composition supplies canonical roots. Exercise that contract
// separately from the raw temporary-path case so an OS path alias cannot be
// mistaken for a failure of the canonical policy (or silently ignored).
func TestCommandContextDeniesCanonicalProtectedFilesystemAccess(t *testing.T) {
	raw := t.TempDir()
	root, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("temporary root required canonicalization: %t", root != raw)
	assertProtectedFilesystemAccessAndOrdinaryWork(t, root)
}

func assertProtectedFilesystemAccessAndOrdinaryWork(t *testing.T, root string) {
	t.Helper()
	protectedRoot := filepath.Join(root, `protected-"quoted"-\backslash-(allow default)`)
	secondProtectedRoot := filepath.Join(root, "second-protected")
	ordinaryRoot := filepath.Join(root, "ordinary")
	for _, protected := range []string{protectedRoot, secondProtectedRoot} {
		if err := os.MkdirAll(protected, 0o700); err != nil {
			t.Fatalf("MkdirAll(protected) error = %v", err)
		}
	}
	if err := os.MkdirAll(ordinaryRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll(ordinary) error = %v", err)
	}

	protectedFile := filepath.Join(protectedRoot, "secret.txt")
	if err := os.WriteFile(protectedFile, []byte("protected"), 0o600); err != nil {
		t.Fatalf("WriteFile(protected) error = %v", err)
	}
	secondProtectedFile := filepath.Join(secondProtectedRoot, "secret.txt")
	if err := os.WriteFile(secondProtectedFile, []byte("second-protected"), 0o600); err != nil {
		t.Fatalf("WriteFile(second protected) error = %v", err)
	}
	ordinaryInput := filepath.Join(ordinaryRoot, "input.txt")
	if err := os.WriteFile(ordinaryInput, []byte("ordinary"), 0o600); err != nil {
		t.Fatalf("WriteFile(ordinary) error = %v", err)
	}
	ordinaryOutput := filepath.Join(ordinaryRoot, "output.txt")
	symlink := filepath.Join(ordinaryRoot, "protected-link")
	hardlink := filepath.Join(ordinaryRoot, "protected-hardlink")
	if err := os.Symlink(protectedFile, symlink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	testCases := []struct {
		name       string
		executable string
		args       []string
	}{
		{
			name:       "direct read",
			executable: "/bin/cat",
			args:       []string{protectedFile},
		},
		{
			name:       "second root read",
			executable: "/bin/cat",
			args:       []string{secondProtectedFile},
		},
		{
			name:       "symlink read",
			executable: "/bin/cat",
			args:       []string{symlink},
		},
		{
			name:       "write",
			executable: "/bin/sh",
			args: []string{
				"-c",
				`printf protected > "$1"`,
				"analytix-process-sandbox-test",
				filepath.Join(protectedRoot, "created.txt"),
			},
		},
		{
			name:       "hardlink creation",
			executable: "/bin/ln",
			args:       []string{protectedFile, hardlink},
		},
		{
			name:       "child process inherits policy",
			executable: "/bin/sh",
			args: []string{
				"-c",
				`/bin/cat "$1"`,
				"analytix-process-sandbox-test",
				protectedFile,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			command, err := CommandContext(
				context.Background(),
				testCase.executable,
				testCase.args,
				FilesystemPolicy{DenyRoots: []string{protectedRoot, secondProtectedRoot}},
			)
			if err != nil {
				t.Fatalf("CommandContext() error = %v", err)
			}
			if runErr := command.Run(); runErr == nil {
				t.Fatal("contained command unexpectedly accessed protected root")
			} else if strings.Contains(runErr.Error(), protectedRoot) {
				t.Fatalf("execution error disclosed protected root: %q", runErr)
			}
		})
	}
	if _, err := os.Lstat(hardlink); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("contained command created a hardlink alias outside the protected root: %v", err)
	}

	command, err := CommandContext(
		context.Background(),
		"/bin/cp",
		[]string{ordinaryInput, ordinaryOutput},
		FilesystemPolicy{DenyRoots: []string{protectedRoot, secondProtectedRoot}},
	)
	if err != nil {
		t.Fatalf("CommandContext(ordinary) error = %v", err)
	}
	if runErr := command.Run(); runErr != nil {
		t.Fatalf("ordinary contained command error = %v", runErr)
	}
	content, err := os.ReadFile(ordinaryOutput)
	if err != nil {
		t.Fatalf("ReadFile(ordinary output) error = %v", err)
	}
	if string(content) != "ordinary" {
		t.Fatalf("ordinary output = %q, want ordinary", content)
	}
}

func TestCommandContextDeniesProcessInspection(t *testing.T) {
	command, err := CommandContext(
		context.Background(),
		"/bin/sh",
		[]string{"-c", `/bin/ps -p "$PPID" -wwE >/dev/null 2>&1`},
		FilesystemPolicy{DenyRoots: []string{t.TempDir()}},
	)
	if err != nil {
		t.Fatalf("CommandContext() error = %v", err)
	}
	if runErr := command.Run(); runErr == nil {
		t.Fatal("contained command unexpectedly inspected its host process")
	}
}

func TestDarwinProfileClosesHostDelegationSurfaces(t *testing.T) {
	root := filepath.Join(t.TempDir(), "protected")
	invocation, err := Prepare(
		"/usr/bin/true",
		nil,
		FilesystemPolicy{DenyRoots: []string{root}},
	)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(invocation.Args) < 2 {
		t.Fatalf("prepared invocation has no profile: %#v", invocation.Args)
	}
	profile := invocation.Args[1]
	required := []string{
		"(deny process-info*)\n",
		"(allow process-info* (target self))\n",
		"(deny appleevent-send)\n",
		"(deny job-creation lsopen system-socket)\n",
		"(deny mach-bootstrap mach-lookup mach-register mach-per-user-lookup mach-cross-domain-lookup)\n",
		"(deny mach-task-name mach-task-read mach-task-special-port* mach-priv-host-port)\n",
		"(with-filter (process-path " + quoteSeatbeltString(sandboxStorageProfilerPath) +
			")\n  (allow mach-lookup (global-name " +
			quoteSeatbeltString(sandboxDiskArbitrationService) + ")))\n",
		"(deny ipc-posix* ipc-sysv*)\n",
		"(deny network-outbound (remote unix-socket))\n",
		"(deny network-outbound (remote ip \"localhost:*\"))\n",
		"(allow network-outbound (remote unix-socket (path-literal " +
			quoteSeatbeltString(sandboxDNSUnixSocket) + ")))\n",
	}
	rootRule := "(deny file-read* file-write* process-exec (subpath " +
		quoteSeatbeltString(root) + "))\n"
	rootRuleOffset := strings.Index(profile, rootRule)
	if rootRuleOffset < 0 {
		t.Fatalf("profile lost the protected-root rule: %q", profile)
	}
	for _, rule := range required {
		if occurrences := strings.Count(profile, rule); occurrences != 1 {
			t.Fatalf("profile rule %q occurrences = %d, want 1", rule, occurrences)
		}
		if offset := strings.Index(profile, rule); offset < 0 || offset > rootRuleOffset {
			t.Fatalf("host delegation rule %q was not fixed before filesystem exceptions", rule)
		}
	}
}

func TestCommandContextScopesDiskArbitrationToStorageProfiler(t *testing.T) {
	probe := exec.Command("/usr/sbin/diskutil", "info", "-plist", "/")
	probe.Stdout = io.Discard
	probe.Stderr = io.Discard
	if err := probe.Run(); err != nil {
		t.Fatalf("host diskutil probe is unavailable: %v", err)
	}

	policy := FilesystemPolicy{DenyRoots: []string{t.TempDir()}}
	profiler, err := CommandContext(
		context.Background(),
		sandboxStorageProfilerPath,
		[]string{"SPStorageDataType", "-json", "-detailLevel", "mini", "-timeout", "15"},
		policy,
	)
	if err != nil {
		t.Fatalf("CommandContext(system_profiler) error = %v", err)
	}
	profiler.Stdout = io.Discard
	profiler.Stderr = io.Discard
	if runErr := profiler.Run(); runErr != nil {
		t.Fatalf("contained read-only storage profiler failed: %v", runErr)
	}

	diskutil, err := CommandContext(
		context.Background(),
		"/usr/sbin/diskutil",
		[]string{"info", "-plist", "/"},
		policy,
	)
	if err != nil {
		t.Fatalf("CommandContext(diskutil) error = %v", err)
	}
	diskutil.Stdout = io.Discard
	diskutil.Stderr = io.Discard
	if runErr := diskutil.Run(); runErr == nil {
		t.Fatal("contained diskutil unexpectedly reached Disk Arbitration")
	}
}

func TestCommandContextDeniesLaunchdMachDelegation(t *testing.T) {
	domain := fmt.Sprintf("user/%d", os.Getuid())
	probe := exec.Command("/bin/launchctl", "print", domain)
	probe.Stdout = io.Discard
	probe.Stderr = io.Discard
	if err := probe.Run(); err != nil {
		t.Fatalf("host launchd probe is unavailable: %v", err)
	}

	command, err := CommandContext(
		context.Background(),
		"/bin/launchctl",
		[]string{"print", domain},
		FilesystemPolicy{DenyRoots: []string{t.TempDir()}},
	)
	if err != nil {
		t.Fatalf("CommandContext() error = %v", err)
	}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if runErr := command.Run(); runErr == nil {
		t.Fatal("contained command unexpectedly delegated to its host launchd domain")
	}
}

func TestCommandContextDeniesLocalDeputyTransports(t *testing.T) {
	if mode := os.Getenv(delegatedProcessChildModeEnv); mode != "" {
		connection, err := net.DialTimeout(
			mode,
			os.Getenv(delegatedProcessChildAddressEnv),
			500*time.Millisecond,
		)
		if err == nil {
			_ = connection.Close()
			t.Fatal("contained child reached an unsandboxed local deputy")
		}
		return
	}

	unixRoot, err := os.MkdirTemp("", "analytix-sandbox-deputy-")
	if err != nil {
		t.Fatalf("create short Unix deputy root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(unixRoot) })
	testCases := []struct {
		name    string
		network string
		listen  string
	}{
		{name: "Unix socket", network: "unix", listen: filepath.Join(unixRoot, "d.sock")},
		{name: "local TCP", network: "tcp4", listen: "127.0.0.1:0"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			listener, err := net.Listen(testCase.network, testCase.listen)
			if err != nil {
				t.Fatalf("listen for local deputy: %v", err)
			}
			defer listener.Close()

			command, err := CommandContext(
				context.Background(),
				os.Args[0],
				[]string{"-test.run=^TestCommandContextDeniesLocalDeputyTransports$"},
				FilesystemPolicy{DenyRoots: []string{t.TempDir()}},
			)
			if err != nil {
				t.Fatalf("CommandContext() error = %v", err)
			}
			command.Env = delegatedProcessChildEnvironment(
				testCase.network,
				listener.Addr().String(),
			)
			runErr := command.Run()

			deadline := time.Now().Add(100 * time.Millisecond)
			switch typed := listener.(type) {
			case *net.TCPListener:
				if err := typed.SetDeadline(deadline); err != nil {
					t.Fatalf("set TCP deputy deadline: %v", err)
				}
			case *net.UnixListener:
				if err := typed.SetDeadline(deadline); err != nil {
					t.Fatalf("set Unix deputy deadline: %v", err)
				}
			default:
				t.Fatalf("unexpected local deputy listener %T", listener)
			}
			connection, acceptErr := listener.Accept()
			if connection != nil {
				_ = connection.Close()
			}
			if runErr != nil || acceptErr == nil {
				t.Fatalf(
					"local deputy boundary failed: child=%v accepted=%t",
					runErr,
					acceptErr == nil,
				)
			}
		})
	}
}

func TestCommandContextAllowsOnlyOneHostBoundLoopbackPort(t *testing.T) {
	if os.Getenv(delegatedProcessChildModeEnv) != "" {
		connection, err := net.DialTimeout(
			os.Getenv(delegatedProcessChildModeEnv),
			os.Getenv(delegatedProcessChildAddressEnv),
			500*time.Millisecond,
		)
		if os.Getenv(delegatedProcessChildAllowEnv) == "1" {
			if err != nil {
				t.Fatal("contained child could not reach its exact host-bound port")
			}
			_ = connection.Close()
			return
		}
		if err == nil {
			_ = connection.Close()
			t.Fatal("contained child reached an unbound loopback port")
		}
		return
	}

	allowed, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer allowed.Close()
	denied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Close()
	allowedPort := uint16(allowed.Addr().(*net.TCPAddr).Port)

	allowedCommand, err := CommandContext(
		context.Background(),
		os.Args[0],
		[]string{"-test.run=^TestCommandContextAllowsOnlyOneHostBoundLoopbackPort$"},
		FilesystemPolicy{DenyRoots: []string{t.TempDir()}, AllowLoopbackTCPPort: allowedPort},
	)
	if err != nil {
		t.Fatal(err)
	}
	allowedCommand.Env = append(
		delegatedProcessChildEnvironment("tcp4", allowed.Addr().String()),
		delegatedProcessChildAllowEnv+"=1",
	)
	if err := allowedCommand.Run(); err != nil {
		t.Fatalf("exact host-bound loopback port was unavailable: %v", err)
	}

	deniedCommand, err := CommandContext(
		context.Background(),
		os.Args[0],
		[]string{"-test.run=^TestCommandContextAllowsOnlyOneHostBoundLoopbackPort$"},
		FilesystemPolicy{DenyRoots: []string{t.TempDir()}, AllowLoopbackTCPPort: allowedPort},
	)
	if err != nil {
		t.Fatal(err)
	}
	deniedCommand.Env = delegatedProcessChildEnvironment("tcp4", denied.Addr().String())
	if err := deniedCommand.Run(); err != nil {
		t.Fatalf("child did not prove the adjacent port remained denied: %v", err)
	}
}

func TestContainedOrdinaryCommandLineToolsRetainSelfInspection(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		executable string
		args       []string
	}{
		{name: "shell", executable: "/bin/sh", args: []string{"--version"}},
		{name: "Git", executable: "/usr/bin/git", args: []string{"--version"}},
		{name: "curl", executable: "/usr/bin/curl", args: []string{"--version"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command, err := CommandContext(
				context.Background(),
				testCase.executable,
				testCase.args,
				FilesystemPolicy{DenyRoots: []string{t.TempDir()}},
			)
			if err != nil {
				t.Fatalf("CommandContext() error = %v", err)
			}
			command.Stdout = io.Discard
			command.Stderr = io.Discard
			if runErr := command.Run(); runErr != nil {
				t.Fatalf("ordinary contained tool failed: %v", runErr)
			}
		})
	}
}

func delegatedProcessChildEnvironment(network, address string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	modePrefix := delegatedProcessChildModeEnv + "="
	addressPrefix := delegatedProcessChildAddressEnv + "="
	allowPrefix := delegatedProcessChildAllowEnv + "="
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, modePrefix) || strings.HasPrefix(entry, addressPrefix) ||
			strings.HasPrefix(entry, allowPrefix) {
			continue
		}
		environment = append(environment, entry)
	}
	return append(
		environment,
		modePrefix+network,
		addressPrefix+address,
	)
}

func TestPrepareDarwinWrapsAndDeduplicatesDenyRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "protected")
	secondRoot := filepath.Join(t.TempDir(), "protected-second")
	invocation, err := Prepare(
		"/usr/bin/true",
		[]string{"argument"},
		FilesystemPolicy{DenyRoots: []string{root, secondRoot, root}},
	)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if invocation.Executable != sandboxExecutable {
		t.Fatalf("Executable = %q, want %q", invocation.Executable, sandboxExecutable)
	}
	if len(invocation.Args) != 4 ||
		invocation.Args[0] != "-p" ||
		invocation.Args[2] != "/usr/bin/true" ||
		invocation.Args[3] != "argument" {
		t.Fatalf("Args structure = %#v, want sandbox profile then original invocation", invocation.Args)
	}
	if occurrences := strings.Count(invocation.Args[1], quoteSeatbeltString(root)); occurrences != 1 {
		t.Fatalf("deny root occurrence count = %d, want 1", occurrences)
	}
	if occurrences := strings.Count(invocation.Args[1], quoteSeatbeltString(secondRoot)); occurrences != 1 {
		t.Fatalf("second deny root occurrence count = %d, want 1", occurrences)
	}
	if rules := strings.Count(invocation.Args[1], "(deny file-read* file-write* process-exec (subpath "); rules != 2 {
		t.Fatalf("deny rule count = %d, want one independent rule per root", rules)
	}
}

func TestValidSandboxExecutableFailsClosed(t *testing.T) {
	root := t.TempDir()
	privateMarker := "private-deny-root-marker"
	invalid := filepath.Join(root, privateMarker)
	if err := os.WriteFile(invalid, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if validSandboxExecutable(invalid) {
		t.Fatal("non-executable file unexpectedly accepted as host sandbox")
	}

	_, err := prepareDarwin(
		Invocation{Executable: "/usr/bin/true"},
		[]string{filepath.Join(root, privateMarker)},
		nil,
		0,
		invalid,
	)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("prepareDarwin() error = %v, want ErrUnavailable", err)
	}
	if strings.Contains(err.Error(), privateMarker) {
		t.Fatalf("error disclosed deny root marker: %q", err)
	}
}

func TestAllowReadExecuteRootInsideDenyAncestorIsSpecificAndReadOnly(t *testing.T) {
	userDataRoot := t.TempDir()
	pluginRoot := filepath.Join(userDataRoot, "plugins", "pinned")
	privateRoot := filepath.Join(userDataRoot, "private")
	if err := os.MkdirAll(pluginRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll(plugin) error = %v", err)
	}
	if err := os.MkdirAll(privateRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll(private) error = %v", err)
	}

	entrypoint := filepath.Join(pluginRoot, "entrypoint.sh")
	pluginData := filepath.Join(pluginRoot, "plugin-data.txt")
	privateData := filepath.Join(privateRoot, "private-data.txt")
	if err := os.WriteFile(
		entrypoint,
		[]byte("#!/bin/sh\n/bin/cat \"$1\"\n"),
		0o700,
	); err != nil {
		t.Fatalf("WriteFile(entrypoint) error = %v", err)
	}
	if err := os.WriteFile(pluginData, []byte("plugin"), 0o600); err != nil {
		t.Fatalf("WriteFile(plugin data) error = %v", err)
	}
	if err := os.WriteFile(privateData, []byte("private"), 0o600); err != nil {
		t.Fatalf("WriteFile(private data) error = %v", err)
	}
	policy := FilesystemPolicy{
		DenyRoots:             []string{userDataRoot},
		AllowReadExecuteRoots: []string{pluginRoot, entrypoint},
	}

	allowed, err := CommandContext(
		context.Background(),
		entrypoint,
		[]string{pluginData},
		policy,
	)
	if err != nil {
		t.Fatalf("CommandContext(allowed plugin) error = %v", err)
	}
	output, err := allowed.Output()
	if err != nil {
		t.Fatalf("allowed plugin execution error = %v", err)
	}
	if string(output) != "plugin" {
		t.Fatalf("allowed plugin output = %q, want plugin", output)
	}

	siblingRead, err := CommandContext(
		context.Background(),
		entrypoint,
		[]string{privateData},
		policy,
	)
	if err != nil {
		t.Fatalf("CommandContext(sibling read) error = %v", err)
	}
	if runErr := siblingRead.Run(); runErr == nil {
		t.Fatal("allowed plugin unexpectedly read denied sibling")
	}

	pluginWrite, err := CommandContext(
		context.Background(),
		"/bin/sh",
		[]string{
			"-c",
			`printf changed > "$1"`,
			"analytix-process-sandbox-test",
			pluginData,
		},
		policy,
	)
	if err != nil {
		t.Fatalf("CommandContext(plugin write) error = %v", err)
	}
	if runErr := pluginWrite.Run(); runErr == nil {
		t.Fatal("read/execute exception unexpectedly restored plugin write")
	}
}
