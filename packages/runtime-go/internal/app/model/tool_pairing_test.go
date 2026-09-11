package model

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestSanitizeToolPairingRepairsDanglingCallAndDropsOrphan(t *testing.T) {
	callID := modelTestHostToolCallID("call_read")
	out := SanitizeToolPairing([]domainmodel.Message{
		{Role: "tool", ToolCallID: "orphan", Content: "drop me"},
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
			ID:        callID,
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"a.txt"`),
		}}},
		{Role: "user", Content: "continue"},
	})
	if len(out) != 3 {
		t.Fatalf("unexpected repaired history length: %#v", out)
	}
	if out[0].Role != "assistant" || string(out[0].ToolCalls[0].Arguments) != `{"path":"a.txt"}` {
		t.Fatalf("assistant tool call should be repaired first: %#v", out)
	}
	if out[1].Role != "tool" || out[1].ToolCallID != callID || out[1].Content != interruptedToolResult {
		t.Fatalf("dangling tool call should get interrupted placeholder result: %#v", out)
	}
	if out[2].Role != "user" || out[2].Content != "continue" {
		t.Fatalf("ordinary post-tool history should be preserved: %#v", out)
	}
}

func TestSanitizeToolPairingNeverSendsLegacyPIICallIdentity(t *testing.T) {
	const account = "6222020202020202020"
	out := SanitizeToolPairing([]domainmodel.Message{
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{
			ID: "provider_call_" + account, Name: "read", Arguments: json.RawMessage(`{}`),
		}}},
		{Role: "tool", ToolCallID: "provider_call_" + account, Content: "legacy result"},
		{Role: "user", Content: "continue"},
	})
	body, _ := json.Marshal(out)
	if len(out) != 1 || out[0].Role != "user" || strings.Contains(string(body), account) || strings.Contains(string(body), "legacy result") {
		t.Fatalf("legacy provider identity crossed request sanitization: %s", body)
	}
}

func TestSanitizeJSONArgumentsRepairsTruncatedValues(t *testing.T) {
	cases := map[string]string{
		`{"path":"a.txt"`:   `{"path":"a.txt"}`,
		`{"items":["a","b"`: `{"items":["a","b"]}`,
		`{"path":"a.txt",`:  `{"path":"a.txt"}`,
		`{"path":`:          `{"path":null}`,
	}
	for raw, want := range cases {
		if got := SanitizeJSONArguments(raw); got != want {
			t.Fatalf("SanitizeJSONArguments(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeSessionMessagesPreservesStandaloneToolMessage(t *testing.T) {
	in := []domainmodel.Message{
		{Role: "system", Content: "sys"},
		{Role: "tool", ToolCallID: "orphan", Content: "saved output"},
	}
	out := NormalizeSessionMessages(in)
	if len(out) != len(in) || &out[0] != &in[0] {
		t.Fatalf("session-safe normalize should keep healthy standalone tool history: %#v", out)
	}
	if out[1].Role != "tool" || out[1].ToolCallID != "orphan" || out[1].Content != "saved output" {
		t.Fatalf("standalone tool message was not preserved: %#v", out)
	}
}
