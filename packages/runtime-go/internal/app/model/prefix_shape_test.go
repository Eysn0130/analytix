package model

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestCapturePrefixShapeCanonicalizesToolsAndSeparatesSourceDiagnostics(t *testing.T) {
	baseTools := []domainmodel.ToolSchema{{
		Name:        "b_tool",
		Description: "B",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"number"}},"required":["z","a"]}`),
	}, {
		Name:        "a_tool",
		Description: "A",
		Parameters:  json.RawMessage(`{"properties":{"q":{"type":["string","null"],"enum":["beta","alpha"]}},"type":"object"}`),
		Source:      "mcp",
	}}
	left := CapturePrefixShape(domainmodel.Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Model:          "deepseek-chat",
		Route:          "agent",
		SystemPrompt:   "stable system",
		Tools:          baseTools,
	})
	reordered := []domainmodel.ToolSchema{{
		Name:        "a_tool",
		Description: "A",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"q":{"enum":["alpha","beta"],"type":["null","string"]}}}`),
		Source:      "skill",
	}, {
		Name:        "b_tool",
		Description: "B",
		Parameters:  json.RawMessage(`{"required":["a","z"],"properties":{"a":{"type":"number"},"z":{"type":"string"}},"type":"object"}`),
	}}
	right := CapturePrefixShape(domainmodel.Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Model:          "deepseek-chat",
		Route:          "agent",
		SystemPrompt:   "stable system",
		Tools:          reordered,
	})

	if left.ToolsHash != right.ToolsHash || left.PrefixHash != right.PrefixHash {
		t.Fatalf("tool order/schema key reorder should keep provider-visible hashes stable: %#v != %#v", left, right)
	}
	if left.ToolSourcesHash == right.ToolSourcesHash {
		t.Fatalf("source-only drift should be isolated to source diagnostics: %#v == %#v", left, right)
	}
	if !domainmodel.SameStringSlice(left.ToolSourceIDs, []string{"builtin", "mcp"}) ||
		!domainmodel.SameStringSlice(right.ToolSourceIDs, []string{"builtin", "skill"}) {
		t.Fatalf("unexpected sorted source ids: left=%#v right=%#v", left.ToolSourceIDs, right.ToolSourceIDs)
	}
	if left.Route != "agent" || left.ProviderID != "deepseek" || left.Model != "deepseek-chat" {
		t.Fatalf("prefix shape should preserve execution identity: %#v", left)
	}
	for name, digest := range map[string]string{
		"systemHash": left.SystemHash, "toolsHash": left.ToolsHash, "prefixHash": left.PrefixHash,
		"prefixItemsHash": left.PrefixItemsHash, "toolSourcesHash": left.ToolSourcesHash,
	} {
		assertFullLowerSHA256(t, name, digest)
	}
}

func TestCapturePrefixShapeIgnoresDynamicTailButTracksCompletedHistory(t *testing.T) {
	base := CapturePrefixShape(domainmodel.Request{
		SystemPrompt: "stable system",
		Messages: []domainmodel.Message{
			{Role: "system", Content: "stable system"},
			{Role: "user", Content: "read note.txt"},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "call_read", Name: "read_file", Arguments: json.RawMessage(`{"path":"note.txt"}`)}}},
			{Role: "tool", ToolCallID: "call_read", Content: "alpha"},
			{Role: "assistant", Content: "note says alpha"},
			{Role: "user", Content: "selected text at 2026-06-26T01:00:00Z"},
		},
	})
	tailChanged := CapturePrefixShape(domainmodel.Request{
		SystemPrompt: "stable system",
		Messages: []domainmodel.Message{
			{Role: "system", Content: "stable system"},
			{Role: "user", Content: "read note.txt"},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "call_read", Name: "read_file", Arguments: json.RawMessage(`{"path":"note.txt"}`)}}},
			{Role: "tool", ToolCallID: "call_read", Content: "alpha"},
			{Role: "assistant", Content: "note says alpha"},
			{Role: "user", Content: "selected text at 2026-06-26T01:01:00Z"},
		},
	})
	historyChanged := CapturePrefixShape(domainmodel.Request{
		SystemPrompt: "stable system",
		Messages: []domainmodel.Message{
			{Role: "system", Content: "stable system"},
			{Role: "user", Content: "read note.txt"},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{{ID: "call_read", Name: "read_file", Arguments: json.RawMessage(`{"path":"note.txt"}`)}}},
			{Role: "tool", ToolCallID: "call_read", Content: "beta"},
			{Role: "assistant", Content: "note says beta"},
			{Role: "user", Content: "selected text at 2026-06-26T01:00:00Z"},
		},
	})

	if base.PrefixItemsHash == "" {
		t.Fatalf("completed history prefix hash should be populated: %#v", base)
	}
	if base.PrefixHash != tailChanged.PrefixHash || base.PrefixItemsHash != tailChanged.PrefixItemsHash {
		t.Fatalf("trailing dynamic user/system tail must not perturb stable prefix shape: %#v != %#v", base, tailChanged)
	}
	if base.PrefixItemsHash == historyChanged.PrefixItemsHash {
		t.Fatalf("completed history changes must perturb prefixItemsHash: %#v == %#v", base, historyChanged)
	}
	if base.PrefixHash != historyChanged.PrefixHash {
		t.Fatalf("history diagnostics should stay separate from provider-visible system/tool prefix: %#v != %#v", base, historyChanged)
	}
}

func TestStablePrefixMessageItemsHashMediaAndRepairToolArguments(t *testing.T) {
	items := stablePrefixMessageItems([]domainmodel.Message{{
		Role: "user",
		Parts: []domainmodel.MessagePart{{
			Type:      "input_image",
			MediaType: "image/png",
			ImageURL:  "https://example.test/raw-image.png?secret=token",
			Data:      "base64-secret-data",
		}},
	}, {
		Role: "assistant",
		ToolCalls: []domainmodel.ToolCall{{
			ID:        "call_read",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"b":2,"a":1`),
		}},
	}})
	raw := string(mustJSON(t, items))
	if strings.Contains(raw, "base64-secret-data") || strings.Contains(raw, "raw-image.png") || strings.Contains(raw, "secret=token") {
		t.Fatalf("stable prefix items should hash media payloads instead of retaining raw values: %s", raw)
	}
	for _, required := range []string{`"type":"image"`, `"mediaType":"image/png"`, `"imageUrlHash"`, `"dataHash"`, `"dataBytes":18`, `"arguments":{"a":1,"b":2}`} {
		if !strings.Contains(raw, required) {
			t.Fatalf("stable prefix items missing %s in %s", required, raw)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertFullLowerSHA256(t *testing.T, name string, digest string) {
	t.Helper()
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 || len(digest) != 64 || strings.ToLower(digest) != digest {
		t.Fatalf("%s must be a full lowercase SHA-256 digest, got %q", name, digest)
	}
}
