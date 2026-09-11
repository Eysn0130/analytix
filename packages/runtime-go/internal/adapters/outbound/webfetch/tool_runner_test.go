package webfetch

import (
	"testing"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
)

func TestExecuteToolRejectsDisabledConfig(t *testing.T) {
	output, isError := ExecuteTool(ToolInput{
		Config: runtimeinfoapp.WebConfig{},
		Args:   map[string]any{"url": "https://example.com"},
	})
	if !isError {
		t.Fatalf("disabled web_fetch should fail: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["code"] != "provider_unavailable" {
		t.Fatalf("disabled output mismatch: %#v", output)
	}
}

func TestExecuteToolValidatesRequestBeforeFetching(t *testing.T) {
	output, isError := ExecuteTool(ToolInput{
		Config: runtimeinfoapp.WebConfig{Enabled: true, FetchEnabled: true},
		Args:   map[string]any{"url": "file:///tmp/nope"},
	})
	if !isError {
		t.Fatalf("invalid URL should fail: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["code"] != "policy_blocked" {
		t.Fatalf("invalid URL output mismatch: %#v", output)
	}
}
