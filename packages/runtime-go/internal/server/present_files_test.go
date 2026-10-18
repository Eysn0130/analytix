package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestPresentProducerExactDeclarationAndEightFileCeilingV1(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "result.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{Workspace: workspace, TurnID: "turn-present", SandboxMode: "workspace-write"}
	files := []any{map[string]any{"path": "result.pdf", "description": "  actual description  "}, map[string]any{"path": "result.pdf", "description": ""}, map[string]any{"path": "result.pdf"}}
	for len(files) < 8 {
		files = append(files, map[string]any{"path": "result.pdf"})
	}
	args := map[string]any{"files": files}
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	output, isError := executePresentRuntimeToolV1(ctx, pending, args, nil, nil)
	if isError || !reflect.DeepEqual(output, map[string]any{"turn": pending.TurnID, "files": files}) {
		t.Fatal("producer altered declaration text or native turn identity")
	}
	capture, present, outputError := domaintoolresult.ConsumeProtectedToolObservationV1(ctx)
	if !present || outputError != nil || capture.Present == nil || len(capture.Present.Files) != 8 || capture.Present.Files[1].Description == nil || *capture.Present.Files[1].Description != "" || capture.Present.Files[2].Description != nil || len(capture.PresentTargets) != 8 {
		t.Fatal("typed observation lost order/optional description")
	}
	body, _ := json.Marshal(args)
	if !protectedPresentMatchesArgumentsV1(capture.Present, body) {
		t.Fatal("original call mismatch")
	}
	capture.Present.Files[0].Path = "replacement.pdf"
	if protectedPresentMatchesArgumentsV1(capture.Present, body) {
		t.Fatal("different declaration bound to original call")
	}
	files = append(files, map[string]any{"path": "result.pdf"})
	ctx = domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	if _, isError := executePresentRuntimeToolV1(ctx, pending, map[string]any{"files": files}, nil, nil); !isError {
		t.Fatal("ninth file admitted")
	}
	if _, present, error_ := domaintoolresult.ConsumeProtectedToolObservationV1(ctx); present || error_ == nil {
		t.Fatal("failed declaration produced success capture or lost error")
	}
}

func TestPresentProducerMalformedAndPartialFailureNeverCaptureReceiptV1(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "result.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	pending := runtimePendingToolCall{Workspace: workspace, TurnID: "turn-present", SandboxMode: "workspace-write"}
	for _, args := range []map[string]any{
		{"files": []any{}}, {"files": []any{map[string]any{"path": "  "}}},
		{"files": []any{map[string]any{"path": "result.pdf", "description": nil}}},
		{"files": []any{map[string]any{"path": "result.pdf", "index": 0}}},
		{"files": []any{map[string]any{"path": "result.pdf"}}, "seq": 1},
		{"files": []any{map[string]any{"path": "result.pdf"}, map[string]any{"path": "missing.pdf"}}},
	} {
		ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
		if _, isError := executePresentRuntimeToolV1(ctx, pending, args, nil, nil); !isError {
			t.Fatal("malformed or partial declaration succeeded")
		}
		if _, present, error_ := domaintoolresult.ConsumeProtectedToolObservationV1(ctx); present || error_ == nil {
			t.Fatal("failed declaration produced success capture or lost raw error")
		}
	}
}

func TestPresentSealingRejectsDifferentDeclarationV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "present", map[string]any{"files": []any{map[string]any{"path": "result.pdf", "description": "original"}}})
	if err := os.WriteFile(filepath.Join(pending.Workspace, "result.pdf"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	if _, failed := executePresentRuntimeToolV1(ctx, pending, map[string]any{"files": []any{map[string]any{"path": "result.pdf", "description": "original"}}}, nil, nil); failed {
		t.Fatal("fixture declaration failed")
	}
	capture, present := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !present {
		t.Fatal("producer observation missing")
	}
	*capture.Present.Files[0].Description = "replacement"
	if _, sealed := h.sealProtectedToolSnapshotV1(ctx, pending, capture); sealed {
		t.Fatal("different declaration was sealed under original call arguments")
	}
}
