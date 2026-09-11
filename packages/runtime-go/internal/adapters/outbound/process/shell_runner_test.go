package process

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	"analytix.local/runtime-go/internal/ports"
)

var errFakeLookPath = errors.New("not found")

func TestShellRunnerRunsShellCommand(t *testing.T) {
	var output bytes.Buffer
	result := NewShellRunner().RunShell(context.Background(), ports.ShellRequest{
		Command: "echo hello",
		Output:  &output,
	})
	if result.Error != "" || result.ExitCode != 0 || strings.TrimSpace(output.String()) != "hello" {
		t.Fatalf("shell result=%#v output=%q", result, output.String())
	}
}

func TestShellRunnerDeniesProtectedReadAndSymlinkEscape(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	protectedRoot := t.TempDir()
	ordinaryRoot := t.TempDir()
	protectedFile := filepath.Join(protectedRoot, "sentinel.txt")
	if err := os.WriteFile(protectedFile, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ordinaryRoot, "sentinel-link")
	if err := os.Symlink(protectedFile, link); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"/bin/cat " + shellQuoteForTest(protectedFile),
		"/bin/cat " + shellQuoteForTest(link),
	} {
		var output bytes.Buffer
		result := NewShellRunner().RunShell(context.Background(), ports.ShellRequest{
			Command: command, Dir: ordinaryRoot, Output: &output,
			ProtectedReadDirs: []string{protectedRoot},
		})
		if result.StartFailed ||
			result.ExitCode == 0 || result.Error == "" ||
			strings.Contains(output.String(), "must-not-escape") ||
			strings.Contains(result.Error, protectedRoot) {
			t.Fatalf("protected data escaped process sandbox: result=%#v output=%q", result, output.String())
		}
	}
}

func TestShellRunnerProtectedSandboxAllowsOrdinaryWork(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	protectedRoot := t.TempDir()
	ordinaryRoot := t.TempDir()
	outputPath := filepath.Join(ordinaryRoot, "output.txt")
	result := NewShellRunner().RunShell(context.Background(), ports.ShellRequest{
		Command: "printf ordinary > " + shellQuoteForTest(outputPath),
		Dir:     ordinaryRoot, ProtectedReadDirs: []string{protectedRoot},
	})
	if result.StartFailed || result.Error != "" || result.ExitCode != 0 {
		t.Fatalf("ordinary contained shell result=%#v", result)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read ordinary contained output: %v", err)
	}
	if string(output) != "ordinary" {
		t.Fatalf("ordinary contained output=%q, want ordinary", string(output))
	}
}

func TestShellRunnerProtectedSandboxPreservesConfiguredZshSemantics(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("system zsh is unavailable")
	}
	t.Setenv("SHELL", "/bin/zsh")
	ordinaryRoot := t.TempDir()
	outputPath := filepath.Join(ordinaryRoot, "zsh-output.txt")
	result := NewShellRunner().RunShell(context.Background(), ports.ShellRequest{
		Command: "print -r -- contained-zsh > " + shellQuoteForTest(outputPath),
		Dir:     ordinaryRoot, ProtectedReadDirs: []string{t.TempDir()},
	})
	if result.StartFailed || result.Error != "" || result.ExitCode != 0 {
		t.Fatalf("contained zsh result=%#v", result)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil || string(output) != "contained-zsh\n" {
		t.Fatalf("contained shell lost zsh semantics: output=%q err=%v", output, err)
	}
}

func TestShellRunnerInvalidProtectedRootFailsBeforeProcessStart(t *testing.T) {
	starts := 0
	runner := ShellRunner{startTracked: func(*exec.Cmd) (uintptr, error) {
		starts++
		return 0, nil
	}}
	result := runner.RunShell(context.Background(), ports.ShellRequest{
		Command: "true", ProtectedReadDirs: []string{"relative-protected-root"},
	})
	if !result.StartFailed || result.Error != "process_sandbox_deny_root_invalid" || starts != 0 {
		t.Fatalf("invalid protected root did not fail closed before exec: result=%#v starts=%d", result, starts)
	}
}

func TestShellRunnerReportsExitCode(t *testing.T) {
	result := NewShellRunner().RunShell(context.Background(), ports.ShellRequest{
		Command: "exit 7",
	})
	if result.Error == "" || result.ExitCode != 7 || result.StartFailed {
		t.Fatalf("shell exit result=%#v", result)
	}
}

func TestShellRunnerCancelsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result := NewShellRunner().RunShell(ctx, ports.ShellRequest{
		Command:   longRunningShellCommand(),
		KillGrace: 500 * time.Millisecond,
	})
	if !result.Canceled || result.Error == "" {
		t.Fatalf("expected canceled result, got %#v", result)
	}
	if runtime.GOOS == "windows" && strings.Contains(result.Error, "exit status") {
		return
	}
	if !strings.Contains(result.Error, "signal") && !strings.Contains(result.Error, "killed") && !strings.Contains(result.Error, "deadline") {
		t.Fatalf("unexpected cancel error: %#v", result)
	}
}

func TestShellRunnerCancellationDoesNotAcknowledgeStopBeforeWaitReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	runner := ShellRunner{
		startTracked: func(*exec.Cmd) (uintptr, error) { return 0, nil },
		waitTracked: func(*exec.Cmd) error {
			close(waitEntered)
			<-releaseWait
			return context.Canceled
		},
	}
	resultCh := make(chan ports.ShellResult, 1)
	go func() {
		resultCh <- runner.RunShell(ctx, ports.ShellRequest{Command: "ignored", KillGrace: 10 * time.Millisecond})
	}()
	select {
	case <-waitEntered:
	case <-time.After(time.Second):
		t.Fatal("runner did not enter the injected Wait boundary")
	}
	cancel()
	select {
	case result := <-resultCh:
		t.Fatalf("runner falsely acknowledged process exit before Wait returned: %#v", result)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseWait)
	select {
	case result := <-resultCh:
		if !result.Canceled || result.StartFailed || result.Error == "" {
			t.Fatalf("delayed real stop result mismatch: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not return after injected Wait confirmed exit")
	}
}

func TestShellRunnerCanceledBeforeCallHasZeroProcessEffect(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	command := "printf blocked > " + shellQuoteForTest(marker)
	if runtime.GOOS == "windows" {
		command = "Set-Content -LiteralPath '" + strings.ReplaceAll(marker, "'", "''") + "' -Value blocked"
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := NewShellRunner().RunShell(ctx, ports.ShellRequest{Command: command})
	if !result.Canceled || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("pre-canceled shell result is not canceled: %#v", result)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-canceled shell command produced a process side effect: %v", err)
	}
}

func TestShellRunnerCanceledStartBarrierBeforeLastCheckHasZeroProcessEffect(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	command := "printf blocked > " + shellQuoteForTest(marker)
	if runtime.GOOS == "windows" {
		command = "Set-Content -LiteralPath '" + strings.ReplaceAll(marker, "'", "''") + "' -Value blocked"
	}
	barrier := subagentapp.NewBackgroundJobStartBarrier()
	barrier.Cancel()
	starts := 0
	runner := ShellRunner{startTracked: func(*exec.Cmd) (uintptr, error) {
		starts++
		return 0, errors.New("unexpected process start")
	}}
	result := runner.RunShell(context.Background(), ports.ShellRequest{Command: command, StartBarrier: barrier})
	if !result.Canceled || result.StartFailed {
		t.Fatalf("closed start authority was not reported as cancellation: %#v", result)
	}
	if starts != 0 {
		t.Fatalf("closed start authority invoked StartTracked %d times", starts)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed start authority produced a process side effect: %v", err)
	}
}

func TestShellRunnerCancellationWinsAfterLastCheckBeforeStartTracked(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	command := "printf blocked > " + shellQuoteForTest(marker)
	if runtime.GOOS == "windows" {
		command = "Set-Content -LiteralPath '" + strings.ReplaceAll(marker, "'", "''") + "' -Value blocked"
	}
	ctx, cancel := context.WithCancel(context.Background())
	inner := subagentapp.NewBackgroundJobStartBarrier()
	barrier := &afterLastCheckStartBarrier{
		inner: inner, entered: make(chan struct{}), release: make(chan struct{}),
	}
	starts := 0
	runner := ShellRunner{startTracked: func(*exec.Cmd) (uintptr, error) {
		starts++
		return 0, errors.New("unexpected process start")
	}}
	resultCh := make(chan ports.ShellResult, 1)
	go func() {
		resultCh <- runner.RunShell(ctx, ports.ShellRequest{Command: command, StartBarrier: barrier})
	}()
	select {
	case <-barrier.entered:
	case <-time.After(time.Second):
		t.Fatal("shell runner did not reach the last-check/start barrier")
	}
	inner.Cancel()
	cancel()
	close(barrier.release)
	result := <-resultCh
	if !result.Canceled || result.StartFailed {
		t.Fatalf("cancellation winner was not reported without a start failure: %#v", result)
	}
	if starts != 0 {
		t.Fatalf("cancellation winner invoked StartTracked %d times", starts)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("process spawned after cancellation won the final start barrier: %v", err)
	}
}

type afterLastCheckStartBarrier struct {
	inner   *subagentapp.BackgroundJobStartBarrier
	entered chan struct{}
	release chan struct{}
}

func (barrier *afterLastCheckStartBarrier) StartIfActive(ctx context.Context, start func() error) error {
	close(barrier.entered)
	<-barrier.release
	return barrier.inner.StartIfActive(ctx, start)
}

func shellQuoteForTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func TestResolveWindowsShellPrefersPowerShellCore(t *testing.T) {
	shell := resolveShellCommandFor("Write-Output ok", "windows", fakeEnv(map[string]string{
		"ProgramFiles": `C:\Program Files`,
	}), fakeLookPath(map[string]string{
		`C:\Program Files\PowerShell\7\pwsh.exe`: `C:\Program Files\PowerShell\7\pwsh.exe`,
	}))

	if shell.File != `C:\Program Files\PowerShell\7\pwsh.exe` {
		t.Fatalf("expected PowerShell 7, got %#v", shell)
	}
	joined := strings.Join(shell.Args, " ")
	if !strings.Contains(joined, "-EncodedCommand") || strings.Contains(joined, "Write-Output ok") {
		t.Fatalf("expected encoded PowerShell command, got %#v", shell.Args)
	}
}

func TestResolveWindowsShellPrefersPrivatePowerShell(t *testing.T) {
	shell := resolveShellCommandFor("Write-Output ok", "windows", fakeEnv(map[string]string{
		"ANALYTIX_RESOURCES_PATH": `C:\Program Files\Analytix\resources`,
		"ProgramFiles":            `C:\Program Files`,
	}), fakeLookPath(map[string]string{
		`C:\Program Files\Analytix\resources\pwsh\pwsh.exe`: `C:\Program Files\Analytix\resources\pwsh\pwsh.exe`,
		`C:\Program Files\PowerShell\7\pwsh.exe`:            `C:\Program Files\PowerShell\7\pwsh.exe`,
	}))

	if shell.File != `C:\Program Files\Analytix\resources\pwsh\pwsh.exe` {
		t.Fatalf("expected private PowerShell, got %#v", shell)
	}
}

func TestResolveWindowsShellFallsBackToCmdWithoutSh(t *testing.T) {
	shell := resolveShellCommandFor("echo ok", "windows", fakeEnv(map[string]string{
		"COMSPEC": `C:\Windows\System32\cmd.exe`,
	}), fakeLookPath(map[string]string{
		`C:\Windows\System32\cmd.exe`: `C:\Windows\System32\cmd.exe`,
	}))

	if shell.File != `C:\Windows\System32\cmd.exe` {
		t.Fatalf("expected cmd fallback, got %#v", shell)
	}
	if strings.Contains(strings.ToLower(shell.File), "sh") {
		t.Fatalf("windows resolver must not fall back to sh: %#v", shell)
	}
	if got := shell.Args; len(got) != 4 || got[0] != "/d" || got[1] != "/s" || got[2] != "/c" || got[3] != "echo ok" {
		t.Fatalf("unexpected cmd args: %#v", got)
	}
}

func TestEncodePowerShellCommandRoundTrips(t *testing.T) {
	encoded := encodePowerShellCommand("Write-Output 'ok'")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode encoded command: %v", err)
	}
	if len(raw)%2 != 0 {
		t.Fatalf("encoded command should be UTF-16LE bytes")
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2 : i*2+2])
	}
	decoded := string(utf16.Decode(units))
	if !strings.Contains(decoded, "[Console]::OutputEncoding") || !strings.Contains(decoded, "Write-Output 'ok'") {
		t.Fatalf("unexpected decoded PowerShell command: %q", decoded)
	}
	if !strings.Contains(decoded, "$PSDefaultParameterValues['Out-File:Encoding']") ||
		!strings.Contains(decoded, "$PSDefaultParameterValues['Set-Content:Encoding']") ||
		!strings.Contains(decoded, "utf8NoBOM") {
		t.Fatalf("expected UTF-8 file encoding defaults in decoded PowerShell command: %q", decoded)
	}
}

func longRunningShellCommand() string {
	if runtime.GOOS == "windows" {
		return "Start-Sleep -Seconds 5"
	}
	return "sleep 5"
}

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func fakeLookPath(values map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if value, ok := values[name]; ok {
			return value, nil
		}
		return "", errFakeLookPath
	}
}
