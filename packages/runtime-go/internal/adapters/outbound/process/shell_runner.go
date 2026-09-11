package process

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"

	childenv "analytix.local/runtime-go/internal/adapters/outbound/childenv"
	processsandbox "analytix.local/runtime-go/internal/adapters/outbound/processsandbox"
	"analytix.local/runtime-go/internal/ports"
	proc "analytix.local/runtime-go/internal/proc"
)

type ShellRunner struct {
	startTracked func(*exec.Cmd) (uintptr, error)
	waitTracked  func(*exec.Cmd) error
}

func NewShellRunner() ShellRunner {
	return ShellRunner{startTracked: proc.StartTracked, waitTracked: func(cmd *exec.Cmd) error { return cmd.Wait() }}
}

func (runner ShellRunner) RunShell(ctx context.Context, request ports.ShellRequest) ports.ShellResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ports.ShellResult{ExitCode: -1, Error: err.Error(), Canceled: true}
	}
	shell := resolveShellCommand(request.Command)
	invocation, prepareErr := processsandbox.Prepare(
		shell.File,
		shell.Args,
		processsandbox.FilesystemPolicy{
			DenyRoots: append([]string(nil), request.ProtectedReadDirs...),
		},
	)
	if prepareErr != nil {
		return ports.ShellResult{
			ExitCode: -1, Error: prepareErr.Error(), StartFailed: true,
		}
	}
	cmd := exec.Command(invocation.Executable, invocation.Args...)
	cmd.Dir = request.Dir
	cmd.Env = childenv.Sanitized(os.Environ())
	if request.Output != nil {
		cmd.Stdout = request.Output
		cmd.Stderr = request.Output
	}
	if err := ctx.Err(); err != nil {
		return ports.ShellResult{ExitCode: -1, Error: err.Error(), Canceled: true}
	}
	var job uintptr
	start := func() error {
		var startErr error
		startTracked := runner.startTracked
		if startTracked == nil {
			startTracked = proc.StartTracked
		}
		job, startErr = startTracked(cmd)
		return startErr
	}
	var startErr error
	if request.StartBarrier != nil {
		startErr = request.StartBarrier.StartIfActive(ctx, start)
	} else {
		startErr = start()
	}
	if startErr != nil {
		if ctx.Err() != nil || errors.Is(startErr, context.Canceled) || errors.Is(startErr, context.DeadlineExceeded) {
			return ports.ShellResult{ExitCode: -1, Error: startErr.Error(), Canceled: true}
		}
		return ports.ShellResult{ExitCode: -1, Error: startErr.Error(), StartFailed: true}
	}
	defer proc.ReapTracked(cmd, job)

	waitCh := make(chan error, 1)
	go func() {
		waitTracked := runner.waitTracked
		if waitTracked == nil {
			waitTracked = func(cmd *exec.Cmd) error { return cmd.Wait() }
		}
		waitCh <- waitTracked(cmd)
	}()
	var err error
	canceled := false
	select {
	case err = <-waitCh:
	case <-ctx.Done():
		proc.KillTracked(cmd, job)
		canceled = true
		grace := request.KillGrace
		if grace <= 0 {
			grace = 2 * time.Second
		}
		select {
		case err = <-waitCh:
		case <-time.After(grace):
			// A kill request is not proof that the process tree exited. Repeat the
			// hard kill after the grace period, then keep ownership until Wait
			// confirms exit; the surrounding job/shutdown timeout remains the
			// fail-closed bound seen by callers.
			proc.KillTracked(cmd, job)
			err = <-waitCh
		}
	}
	result := ports.ShellResult{ExitCode: 0, Canceled: canceled}
	if err != nil {
		result.ExitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Error = err.Error()
	}
	return result
}

type shellCommand struct {
	File string
	Args []string
}

type shellCandidate struct {
	Kind  string
	Names []string
}

func resolveShellCommand(command string) shellCommand {
	return resolveShellCommandFor(command, runtime.GOOS, os.Getenv, exec.LookPath)
}

func resolveShellCommandFor(
	command string,
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
) shellCommand {
	if goos == "windows" {
		return resolveWindowsShellCommand(command, getenv, lookPath)
	}
	return resolvePosixShellCommand(command, getenv, lookPath)
}

func resolveWindowsShellCommand(
	command string,
	getenv func(string) string,
	lookPath func(string) (string, error),
) shellCommand {
	candidates := []shellCandidate{
		{Kind: "powershell", Names: windowsPrivatePowerShellCandidates(getenv)},
		{Kind: "powershell", Names: windowsPowerShellCoreCandidates(getenv)},
		{Kind: "powershell", Names: windowsPowerShellCandidates(getenv)},
		{Kind: "bash", Names: windowsGitBashCandidates(getenv)},
		{Kind: "cmd", Names: []string{strings.TrimSpace(getenv("COMSPEC")), "cmd.exe"}},
	}
	for _, candidate := range candidates {
		if file := firstResolvedShell(candidate.Names, lookPath); file != "" {
			return shellCommandForKind(candidate.Kind, file, command)
		}
	}
	return shellCommandForKind("cmd", "cmd.exe", command)
}

func resolvePosixShellCommand(
	command string,
	getenv func(string) string,
	lookPath func(string) (string, error),
) shellCommand {
	candidates := []shellCandidate{
		{Kind: "bash", Names: []string{strings.TrimSpace(getenv("SHELL")), "/bin/bash", "bash", "/bin/zsh", "zsh", "/bin/sh", "sh"}},
	}
	for _, candidate := range candidates {
		if file := firstResolvedShell(candidate.Names, lookPath); file != "" {
			return shellCommandForKind(candidate.Kind, file, command)
		}
	}
	return shellCommandForKind("sh", "sh", command)
}

func shellCommandForKind(kind string, file string, command string) shellCommand {
	switch kind {
	case "powershell":
		return shellCommand{
			File: file,
			Args: []string{
				"-NoLogo",
				"-NoProfile",
				"-NonInteractive",
				"-InputFormat",
				"Text",
				"-OutputFormat",
				"Text",
				"-ExecutionPolicy",
				"Bypass",
				"-EncodedCommand",
				encodePowerShellCommand(command),
			},
		}
	case "cmd":
		return shellCommand{File: file, Args: []string{"/d", "/s", "/c", command}}
	default:
		return shellCommand{File: file, Args: []string{"-lc", command}}
	}
}

func firstResolvedShell(names []string, lookPath func(string) (string, error)) string {
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		resolved, err := lookPath(name)
		if err == nil && strings.TrimSpace(resolved) != "" {
			return resolved
		}
	}
	return ""
}

func windowsPrivatePowerShellCandidates(getenv func(string) string) []string {
	resourcesPath := strings.TrimSpace(getenv("ANALYTIX_RESOURCES_PATH"))
	appRoot := strings.TrimSpace(getenv("ANALYTIX_APP_ROOT"))
	candidates := []string{
		strings.TrimSpace(getenv("ANALYTIX_PRIVATE_PWSH_PATH")),
		joinWindowsPath(resourcesPath, "pwsh", "pwsh.exe"),
		joinWindowsPath(appRoot, "..", "pwsh", "pwsh.exe"),
	}
	if executable, err := os.Executable(); err == nil && strings.TrimSpace(executable) != "" {
		candidates = append(
			candidates,
			filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "..", "pwsh", "pwsh.exe")),
		)
	}
	return candidates
}

func windowsPowerShellCoreCandidates(getenv func(string) string) []string {
	return []string{
		joinWindowsPath(getenv("ProgramFiles"), "PowerShell", "7", "pwsh.exe"),
		joinWindowsPath(getenv("ProgramW6432"), "PowerShell", "7", "pwsh.exe"),
		"pwsh.exe",
	}
}

func windowsPowerShellCandidates(getenv func(string) string) []string {
	return []string{
		joinWindowsPath(getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
		joinWindowsPath(getenv("WINDIR"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
		"powershell.exe",
	}
}

func windowsGitBashCandidates(getenv func(string) string) []string {
	return []string{
		joinWindowsPath(getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		joinWindowsPath(getenv("ProgramW6432"), "Git", "bin", "bash.exe"),
		joinWindowsPath(getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
		"bash.exe",
	}
}

func joinWindowsPath(root string, segments ...string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	out := strings.TrimRight(root, `\/`)
	for _, segment := range segments {
		segment = strings.Trim(segment, `\/`)
		if segment == "" {
			continue
		}
		out += `\` + segment
	}
	return out
}

func encodePowerShellCommand(command string) string {
	preamble := "[Console]::InputEncoding = [System.Text.Encoding]::UTF8; " +
		"[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " +
		"$OutputEncoding = [System.Text.Encoding]::UTF8; " +
		"$analytixUtf8Encoding = if ($PSVersionTable.PSVersion.Major -ge 7) { 'utf8NoBOM' } else { 'utf8' }; " +
		"$PSDefaultParameterValues['Out-File:Encoding'] = $analytixUtf8Encoding; " +
		"$PSDefaultParameterValues['Set-Content:Encoding'] = $analytixUtf8Encoding; " +
		"$ProgressPreference = 'SilentlyContinue'; " +
		"$VerbosePreference = 'SilentlyContinue'; " +
		"$InformationPreference = 'SilentlyContinue'; "
	encoded := utf16.Encode([]rune(preamble + command))
	bytes := make([]byte, 0, len(encoded)*2)
	for _, unit := range encoded {
		bytes = append(bytes, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
