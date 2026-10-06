package toolresult

import (
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestProtectedToolSnapshotBodyPrefixAndStructuredStatus(t *testing.T) {
	for _, test := range []struct {
		body     string
		limit    int
		expected string
		cut      bool
	}{
		{"", 32768, "", false}, {"status=completed\nexitCode=0\n", 32768, "status=completed\nexitCode=0\n", false},
		{"A中B", 3, "A", true}, {"A中B", 4, "A中", true}, {"a\r\nb\n", 6, "a\r\nb\n", false},
		{strings.Repeat("x", 32769), 32768, strings.Repeat("x", 32768), true},
	} {
		got, cut := BoundProtectedTextV1(test.body, test.limit)
		if got != test.expected || cut != test.cut {
			t.Fatalf("prefix mismatch: bytes=%d cut=%v", len(got), cut)
		}
	}
	code := 7
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "shell", Status: "timeout", Body: "exitCode=0\nstatus=completed\n", ExitCode: &code})
	got, ok := ConsumeProtectedToolResultV1(ctx)
	if !ok || got.Status != "timeout" || *got.ExitCode != 7 {
		t.Fatal("body must not determine host status")
	}
	if _, ok := ConsumeProtectedToolResultV1(ctx); ok {
		t.Fatal("capture consumed twice")
	}
}

func TestProtectedToolSnapshotCaptureDuplicateAndUnsupportedFailClosed(t *testing.T) {
	ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "original\n"})
	CaptureProtectedToolResultV1(ctx, ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "replacement\n"})
	if _, ok := ConsumeProtectedToolResultV1(ctx); ok {
		t.Fatal("duplicate producer must not pick a body")
	}
	for _, kind := range []string{"unknown", "web", "background"} {
		if ValidateProtectedCaptureV1(ProtectedCaptureV1{Kind: kind, Status: "completed"}) == nil {
			t.Fatal("unsupported kind")
		}
	}
}

func TestProtectedToolSnapshotStrictEnvelopeAndPublicPrivateSplit(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	snapshot := ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID,
		ResultItemID: ToolResultItemIDV1("turn-a", callID), ToolName: "read", ContextDigest: strings.Repeat("c", 64),
		ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Capture: ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "package main\r\n", Label: "code.go", StartLine: 1, EndLine: 1, TotalLines: 1}}
	body, err := ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedSnapshotV1(body)
	if err != nil || parsed.Capture.Body != "package main\r\n" {
		t.Fatal("strict ordinary envelope roundtrip failed")
	}
	for _, invalid := range [][]byte{
		append([]byte(" "), body...), bytes.Replace(body, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		bytes.Replace(body, []byte(`"purpose":`), []byte(`"unknown":true,"purpose":`), 1),
	} {
		if _, err := ParseProtectedSnapshotV1(invalid); err == nil {
			t.Fatal("noncanonical, duplicate or extra envelope accepted")
		}
	}
	binding, err := ProtectedSnapshotBindingForV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": snapshot.ResultItemID, "threadId": "thread-a", "turnId": "turn-a", "kind": "tool_result", "role": "tool",
		"callId": callID, "toolName": "read", "toolKind": "tool_call", "status": "completed", "isError": false,
		"createdAt": "2026-01-01T00:00:00Z", "finishedAt": "2026-01-01T00:00:00Z", "contextDigest": snapshot.ContextDigest,
		"contextEpoch": uint64(1), "executionGrantId": snapshot.ExecutionGrantID,
		"output":                        PublicToolResultProjectionRecordV1(WithheldProjectionV1("completed", "tool_output_private")),
		ProtectedSnapshotBindingFieldV1: ProtectedSnapshotBindingRecordV1(binding)}
	if _, ok := PrivateDurableToolResultItemRecordV1(item); !ok {
		t.Fatal("closed private binding was refused")
	}
	public := PublicToolResultItemRecordV1(item)
	if public[ProtectedSnapshotBindingFieldV1] != nil {
		t.Fatal("private snapshot binding entered public record")
	}
}
