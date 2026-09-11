package historymigration

import "testing"

func TestProjectAuthorityFreeSidecarItemPreservesCanonicalLegacyItemIdentity(t *testing.T) {
	item := map[string]any{
		"id": "legacy-call-item", "threadId": "source-thread", "turnId": "turn-1",
		"kind": "tool_call", "status": "completed", "toolName": "read_file",
		"callId": "legacy-provider-call", "arguments": map[string]any{"path": "private"},
		"contextDigest": "untrusted-context", "executionGrantId": "untrusted-grant",
	}

	projected, keep := ProjectAuthorityFreeSidecarItem("target-thread", item, "2026-07-23T00:00:00Z")
	if !keep || projected == nil {
		t.Fatal("canonical legacy item was dropped")
	}
	if projected["id"] != "legacy-call-item" || projected["threadId"] != "target-thread" {
		t.Fatalf("legacy rendering identity was not preserved: %#v", projected)
	}
	if projected["contextDigest"] != nil || projected["executionGrantId"] != nil {
		t.Fatalf("legacy execution authority survived projection: %#v", projected)
	}
	arguments, _ := projected["arguments"].(map[string]any)
	if arguments == nil || arguments["messageKey"] != "tool_arguments_withheld" {
		t.Fatalf("legacy tool arguments were not closed: %#v", projected)
	}
}

func TestProjectAuthorityFreeSidecarItemDoesNotPreserveInvalidLegacyItemIdentity(t *testing.T) {
	projected, keep := ProjectAuthorityFreeSidecarItem("target-thread", map[string]any{
		"id": "../invalid", "turnId": "turn-1", "kind": "tool_call", "callId": "legacy-provider-call",
	}, "2026-07-23T00:00:00Z")
	if !keep || projected == nil {
		t.Fatal("closed legacy item projection was unexpectedly dropped")
	}
	if projected["id"] != nil {
		t.Fatalf("invalid legacy item identity survived projection: %#v", projected)
	}
}
