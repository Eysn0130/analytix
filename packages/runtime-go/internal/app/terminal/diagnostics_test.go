package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/ports"
)

func TestCommandOutputLineRedactsHomeAndTruncates(t *testing.T) {
	home := "/Users/analytix-test"
	input := "\x1b[31m" + home + "/project\nsecond line"
	got := CommandOutputLine(input, home)
	if strings.Contains(got, home) {
		t.Fatalf("home directory was not redacted: %q", got)
	}
	if strings.Contains(got, "\n") || strings.Contains(got, "\x1b") {
		t.Fatalf("output should be one sanitized line: %q", got)
	}
	long := strings.Repeat("x", 300)
	got = CommandOutputLine(long, home)
	if len([]rune(got)) > 240 || !strings.HasSuffix(got, "...") {
		t.Fatalf("long output not truncated: len=%d output=%q", len([]rune(got)), got)
	}
}

func TestCommandDiagnosticsForSortsFoundBeforeMissing(t *testing.T) {
	probe := fakeCommandProbe{
		"go": ports.CommandProbeResult{Found: true, Stdout: "go version go1.99.0 test\n"},
	}
	diagnostics := CommandDiagnosticsFor([]string{
		"definitely-missing-analytix-binary --version",
		"go version",
	}, 2*time.Second, probe, "")
	if len(diagnostics) != 2 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	first, ok := diagnostics[0].(map[string]any)
	if !ok || first["binary"] != "go" || first["found"] != true {
		t.Fatalf("found command should sort first: %#v", diagnostics)
	}
	second, ok := diagnostics[1].(map[string]any)
	if !ok || second["found"] != false || second["error"] != "not found" {
		t.Fatalf("missing command should report not found: %#v", diagnostics)
	}
}

type fakeCommandProbe map[string]ports.CommandProbeResult

func (f fakeCommandProbe) ProbeCommand(_ context.Context, request ports.CommandProbeRequest) ports.CommandProbeResult {
	if result, ok := f[request.Binary]; ok {
		return result
	}
	return ports.CommandProbeResult{Found: false, Error: "not found"}
}
