package process

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	childenv "analytix.local/runtime-go/internal/adapters/outbound/childenv"
	processsandbox "analytix.local/runtime-go/internal/adapters/outbound/processsandbox"
	"analytix.local/runtime-go/internal/ports"
)

type CommandProbe struct {
	protectedReadDirs []string
}

func NewCommandProbe(protectedReadDirs ...string) CommandProbe {
	return CommandProbe{
		protectedReadDirs: append([]string(nil), protectedReadDirs...),
	}
}

func (p CommandProbe) ProbeCommand(ctx context.Context, request ports.CommandProbeRequest) ports.CommandProbeResult {
	exe, err := exec.LookPath(request.Binary)
	if err != nil {
		return ports.CommandProbeResult{Found: false, Error: "not found"}
	}
	cmd, err := processsandbox.CommandContext(
		ctx,
		exe,
		request.Args,
		processsandbox.FilesystemPolicy{
			DenyRoots: append([]string(nil), p.protectedReadDirs...),
		},
	)
	if err != nil {
		return ports.CommandProbeResult{Found: true, Error: err.Error()}
	}
	cmd.Env = childenv.Sanitized(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result := ports.CommandProbeResult{
		Found:  true,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		result.Error = err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
		}
	}
	return result
}

func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
