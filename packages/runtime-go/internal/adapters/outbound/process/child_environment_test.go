package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/ports"
	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANALYTIX_PROCESS_CHILD_ENV_HELPER") == "1" {
		if os.Getenv("ANALYTIX_RUNTIME_TOKEN") != "" {
			fmt.Print("leaked")
			os.Exit(91)
		}
		fmt.Print("clean")
		os.Exit(0)
	}
	userconfigtest.Run(m)
}

func TestRuntimeBearerTokenNeverInheritedByCommandProbe(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "host-bearer-secret")
	t.Setenv("ANALYTIX_PROCESS_CHILD_ENV_HELPER", "1")
	result := NewCommandProbe().ProbeCommand(context.Background(), ports.CommandProbeRequest{Binary: os.Args[0]})
	if !result.Found || result.Error != "" || result.Stdout != "clean" {
		t.Fatalf("command probe inherited a host secret: %#v", result)
	}
}

func TestRuntimeBearerTokenNeverInheritedByShell(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "host-bearer-secret")
	var output bytes.Buffer
	command := shellEnvironmentAssertionCommand(filepath.Base(resolveShellCommand("").File))
	result := NewShellRunner().RunShell(context.Background(), ports.ShellRequest{Command: command, Output: &output})
	if result.Error != "" || result.ExitCode != 0 || strings.TrimSpace(output.String()) != "clean" {
		t.Fatalf("shell inherited a host secret: result=%#v output=%q", result, output.String())
	}
}

func shellEnvironmentAssertionCommand(shellBase string) string {
	switch lower := strings.ToLower(shellBase); {
	case strings.Contains(lower, "powershell"), strings.Contains(lower, "pwsh"):
		return `if ($env:ANALYTIX_RUNTIME_TOKEN) { exit 91 } else { Write-Output clean }`
	case strings.Contains(lower, "cmd"):
		return `if defined ANALYTIX_RUNTIME_TOKEN (exit /b 91) else (echo clean)`
	default:
		return `if [ -n "$ANALYTIX_RUNTIME_TOKEN" ]; then exit 91; else printf clean; fi`
	}
}
