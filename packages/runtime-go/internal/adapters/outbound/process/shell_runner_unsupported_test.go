//go:build !darwin

package process

import (
	"context"
	"os/exec"
	"testing"

	"analytix.local/runtime-go/internal/ports"
)

func TestShellRunnerUnavailableContainmentNeverStartsOrRetries(t *testing.T) {
	starts := 0
	runner := ShellRunner{startTracked: func(*exec.Cmd) (uintptr, error) {
		starts++
		return 0, nil
	}}
	result := runner.RunShell(context.Background(), ports.ShellRequest{
		Command: "echo synthetic", ProtectedReadDirs: []string{t.TempDir()},
	})
	if !result.StartFailed || result.Error != "process_sandbox_unavailable" || starts != 0 {
		t.Fatal("unsupported containment started or retried an unprotected process")
	}
}
