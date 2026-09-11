package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/ports"
)

func TestCommandProbeReportsMissingBinary(t *testing.T) {
	result := NewCommandProbe().ProbeCommand(context.Background(), ports.CommandProbeRequest{
		Binary: "definitely-missing-analytix-process-probe",
	})
	if result.Found || result.Error != "not found" {
		t.Fatalf("missing binary result = %#v", result)
	}
}

func TestCommandProbeRunsAvailableCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := NewCommandProbe().ProbeCommand(ctx, ports.CommandProbeRequest{
		Binary: "go",
		Args:   []string{"version"},
	})
	if !result.Found || result.Error != "" || !strings.Contains(result.Stdout, "go version") {
		t.Fatalf("go version probe result = %#v", result)
	}
}

func TestCommandProbeProtectedRootsDenyReadsAndAllowOrdinaryCommands(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	protectedRoot := t.TempDir()
	protectedFile := filepath.Join(protectedRoot, "sentinel.txt")
	if err := os.WriteFile(protectedFile, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	ordinaryFile := filepath.Join(t.TempDir(), "ordinary.txt")
	if err := os.WriteFile(ordinaryFile, []byte("ordinary"), 0o600); err != nil {
		t.Fatal(err)
	}

	probe := NewCommandProbe(protectedRoot)
	blocked := probe.ProbeCommand(context.Background(), ports.CommandProbeRequest{
		Binary: "/bin/cat",
		Args:   []string{protectedFile},
	})
	if !blocked.Found || blocked.Error == "" || strings.Contains(blocked.Stdout, "must-not-escape") {
		t.Fatalf("protected command access escaped: %#v", blocked)
	}

	ordinary := probe.ProbeCommand(context.Background(), ports.CommandProbeRequest{
		Binary: "/bin/cat",
		Args:   []string{ordinaryFile},
	})
	if !ordinary.Found || ordinary.Error != "" || ordinary.Stdout != "ordinary" {
		t.Fatalf("ordinary contained command failed: %#v", ordinary)
	}
}
