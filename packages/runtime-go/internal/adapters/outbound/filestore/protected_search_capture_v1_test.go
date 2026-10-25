package filestore_test

import (
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	snapshotfs "analytix.local/runtime-go/internal/adapters/outbound/toolresultsnapshot"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type searchFixtureAuthorityV1 struct {
	value   snapshotport.AuthorizedV1
	calls   int
	revoked bool
}

func (a *searchFixtureAuthorityV1) AcquireCurrent(context.Context, snapshotport.SelectorV1) (snapshotport.AuthorizedV1, func(), error) {
	return a.value, func() {}, nil
}
func (a *searchFixtureAuthorityV1) ValidateCurrent(context.Context, snapshotport.AuthorizedV1) error {
	a.calls++
	if a.revoked && a.calls == 2 {
		return snapshotport.ErrUnavailable
	}
	return nil
}
func syntheticSearchRootV1(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}
func invokeSearchV1(t *testing.T, tool, root string, args map[string]any) domaintoolresult.ProtectedCaptureV1 {
	t.Helper()
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	input := filestore.WorkspaceSearchToolInput{Context: ctx, Workspace: root, Args: args, SandboxMode: "workspace-write"}
	var failed bool
	if tool == "grep" {
		_, failed = filestore.ExecuteGrepTool(input)
	} else {
		_, failed = filestore.ExecuteGlobTool(input)
	}
	if failed {
		t.Fatal("synthetic producer failed")
	}
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !ok {
		t.Fatal("actual producer emitted no protected capture")
	}
	return capture
}
func TestActualSearchProducerCanonicalSnapshotAuthorizedDisplayV1(t *testing.T) {
	root := syntheticSearchRootV1(t, "before\nSOURCE_MATCH\nafter\n")
	capture := invokeSearchV1(t, "grep", root, map[string]any{"pattern": "SOURCE", "context": 1})
	match := capture.Search.Files[0].Matches[0]
	if match.LineNumber != 2 || match.Line != "SOURCE_MATCH" || len(match.ContextBefore) != 1 || match.ContextAfter[0] != "after" || capture.Search.Total == nil || *capture.Search.Total != 1 {
		t.Fatal("producer truth lost")
	}
	if capture.Search.Files[0].Path != filepath.Join(root, "source.txt") {
		t.Fatal("display alias became source truth")
	}
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	call := "call_host_" + strings.Repeat("b", 64)
	snapshot := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: root,
		ThreadID: "thread", TurnID: "turn", CallID: call, ResultItemID: domaintoolresult.ToolResultItemIDV1("turn", call), ToolName: "grep", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Capture: capture}
	body, err := domaintoolresult.ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	storeRoot := filepath.Join(t.TempDir(), "tool-result-snapshots-v1")
	access, err := privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := snapshotfs.NewStore(ctx, storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := store.Put(ctx, snapshot)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	prepared, err := snapshotfs.PrepareRecoveryV1(ctx, storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		t.Fatal("Search snapshot failed existing recovery semantics", err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	store, err = snapshotfs.NewStore(ctx, storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	recovered, err := store.Read(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	recoveredBytes, err := domaintoolresult.ProtectedSnapshotBytesV1(recovered)
	if err != nil || string(recoveredBytes) != string(body) {
		t.Fatal("restarted store changed the sealed producer observation", err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("MUTATED_SOURCE"), 0600); err != nil {
		t.Fatal(err)
	}
	selector := snapshotport.SelectorV1{ThreadID: "thread", TurnID: "turn", CallID: call, ResultItemID: snapshot.ResultItemID}
	for _, revoked := range []bool{false, true} {
		a := &searchFixtureAuthorityV1{revoked: revoked, value: snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: root, ToolName: "grep", ContextDigest: snapshot.ContextDigest, ContextEpoch: 1, ExecutionGrantID: snapshot.ExecutionGrantID, CaseBindingHash: snapshot.CaseBindingHash, Binding: binding}}
		service := snapshotapp.Service{Store: store, Authority: a}
		display, err := service.Read(context.Background(), selector)
		if revoked {
			if err == nil || display.Capture.Body != "" {
				t.Fatal("revocation released body")
			}
			continue
		}
		if err != nil || display.Capture.Body != capture.Body || display.SnapshotDigest != binding.SnapshotDigest {
			t.Fatal("snapshot was not recovered exactly", err)
		}
		wire, err := json.Marshal(display)
		if err != nil || !strings.Contains(string(wire), "SOURCE_MATCH") {
			t.Fatal("actual DTO lost source match")
		}
	}
}
func TestActualSearchKeepsOriginalLimitAndSeparateCaptureTruncationV1(t *testing.T) {
	root := syntheticSearchRootV1(t, "SOURCE_ONE\nSOURCE_TWO\n")
	capture := invokeSearchV1(t, "grep", root, map[string]any{"pattern": "SOURCE", "limit": 1})
	if capture.Search.Total != nil || !capture.Search.Truncated || capture.Truncated || len(capture.Search.Files[0].Matches) != 1 || strings.Contains(capture.Body, "SOURCE_TWO") {
		t.Fatal("limit was expanded or fabricated a full total")
	}
	for _, file := range []string{"second.txt", "third.txt"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	glob := invokeSearchV1(t, "glob", root, map[string]any{"pattern": "*.txt", "limit": 1})
	if glob.Search.Total != nil || !glob.Search.Truncated || len(glob.Search.Paths) != 1 {
		t.Fatal("glob budget changed")
	}
	largeRoot := syntheticSearchRootV1(t, "SOURCE_ONE\nSOURCE_"+strings.Repeat("x", 33000)+"\n")
	clipped := invokeSearchV1(t, "grep", largeRoot, map[string]any{"pattern": "SOURCE"})
	if clipped.Search.Truncated || !clipped.Truncated || clipped.Search.Total == nil || *clipped.Search.Total != 2 || len(clipped.Search.Files[0].Matches) != 1 {
		t.Fatal("capture byte limit confused with producer limit")
	}
	if len(clipped.Body) > domaintoolresult.ProtectedSnapshotBodyLimitV1 {
		t.Fatal("capture budget widened")
	}
}
func TestSearchValidationFailureUsesRealProtectedGenericOutputV1(t *testing.T) {
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	_, failed := filestore.ExecuteGrepTool(filestore.WorkspaceSearchToolInput{Context: ctx, Workspace: t.TempDir(), Args: map[string]any{"pattern": "["}, SandboxMode: "workspace-write"})
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !failed || !ok || !capture.Search.ToolIsError || capture.Status != "failed" || !strings.Contains(capture.Body, "invalid_pattern") {
		t.Fatal("actual failure became a search card")
	}
}

func TestSearchIncompleteBinaryTailAndEscapingBudgetAreHonestV1(t *testing.T) {
	root := syntheticSearchRootV1(t, "SOURCE_FIRST\n"+strings.Repeat("x", 9000)+"\x00\nSOURCE_UNSCANNED\n")
	capture := invokeSearchV1(t, "grep", root, map[string]any{"pattern": "SOURCE"})
	if capture.Search.Total != nil || !capture.Search.Truncated || strings.Contains(capture.Body, "UNSCANNED") {
		t.Fatal("early binary tail fabricated scan completeness")
	}
	escaped := syntheticSearchRootV1(t, "SOURCE_"+strings.Repeat("\x01", 22000)+"\n")
	clipped := invokeSearchV1(t, "grep", escaped, map[string]any{"pattern": "SOURCE"})
	if !clipped.Truncated || clipped.Search.Total == nil || *clipped.Search.Total != 1 || len(clipped.Search.Files) != 0 {
		t.Fatal("escaped envelope lost its honest clipped snapshot")
	}
}
