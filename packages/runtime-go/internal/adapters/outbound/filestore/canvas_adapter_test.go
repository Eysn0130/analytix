//go:build darwin || linux

package filestore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	canvasapp "analytix.local/runtime-go/internal/app/canvasediting"
	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	identity "analytix.local/runtime-go/internal/domain/identity"
	host "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func TestCanvasAdapterFiniteReviewAndAuthority(t *testing.T) {
	ctx := context.Background()
	before, _ := canvasEditingBytes(t, "canvas")
	files, workspace, path := canvasEditingFixture(t, "canvas", before)
	principal, _ := identity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	authority := &officeTestIdentity{principal: principal}
	service := canvasapp.New(authority, map[string]canvasapp.Objects{"canvas": objectapp.New(authority, files)})
	if err := service.BindHost(officeRecoveryTestProjector{authority}, func(_ context.Context, _, _ string, validate func() error) (func(), error) {
		return func() {}, validate()
	}); err != nil {
		t.Fatal(err)
	}
	ready := true
	adapter := canvasapp.NewAdapter(service, func(context.Context) bool { return ready })
	binding := host.Binding{PackageID: "analytix-canvas"}
	call := func(op string, input any) map[string]json.RawMessage {
		t.Helper()
		raw, e := json.Marshal(input)
		if e != nil {
			t.Fatal(e)
		}
		result, e := adapter.Invoke(ctx, host.Call{Binding: binding, Principal: principal, ContributionID: "workspace-editor", Operation: op, Input: raw})
		if e != nil {
			t.Fatal(op, e)
		}
		var out map[string]json.RawMessage
		if e = json.Unmarshal(result.Output, &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	opened := call("open-object", map[string]any{"threadId": "thread_main", "object": map[string]any{"workspace": workspace, "path": path}, "kind": "canvas"})
	var doc canvasapp.Document
	if string(opened["ok"]) != "true" || json.Unmarshal(opened["document"], &doc) != nil || doc.SessionID == "" {
		t.Fatal("open failed", opened)
	}
	input := map[string]any{"sessionId": doc.SessionID, "threadId": doc.ThreadID, "baseRevision": doc.Revision, "selectedIds": []string{"n1"}, "operations": []any{map[string]any{"kind": "set-display-label", "target": "node", "id": "n1", "displayLabel": "Reviewed label"}}}
	for _, extra := range []string{"content", "path", "command"} {
		input[extra] = "untrusted"
		if out := call("propose-scene", input); string(out["ok"]) != "false" || string(out["code"]) != `"invalid_request"` {
			t.Fatal("extra field admitted", extra, out)
		}
		delete(input, extra)
	}
	input["operations"] = []any{map[string]any{"kind": "set-node-layout", "id": "n1", "layout": map[string]any{"y": 0, "width": 100, "height": 100}}}
	if out := call("propose-scene", input); string(out["ok"]) != "false" {
		t.Fatal("missing x treated as zero", out)
	}
	input["operations"] = []any{map[string]any{"kind": "set-display-label", "target": "node", "id": "n1", "displayLabel": "Reviewed label"}}
	out := call("propose-scene", input)
	var proposal canvasapp.Proposal
	if string(out["ok"]) != "true" || json.Unmarshal(out["proposal"], &proposal) != nil || proposal.ID == "" {
		t.Fatal("proposal failed", out)
	}
	officeEditingAssertBytes(t, path, before)
	request := map[string]any{"sessionId": doc.SessionID, "threadId": doc.ThreadID, "proposalId": proposal.ID}
	request["threadId"] = "other-thread"
	if out = call("proposal-apply", request); string(out["ok"]) != "false" {
		t.Fatal("wrong thread applied", out)
	}
	officeEditingAssertBytes(t, path, before)
	request["threadId"] = doc.ThreadID
	if out = call("proposal-apply", request); string(out["ok"]) != "true" {
		t.Fatal("apply failed", out)
	}
	if out = call("object-recovery", map[string]any{"sessionId": doc.SessionID, "threadId": doc.ThreadID}); string(out["ok"]) != "true" || !strings.Contains(string(out["recovery"]), "Reviewed label") {
		t.Fatal("review not retained", out)
	}
	for _, raw := range []string{`{"sessionId":"` + doc.SessionID + `","threadId":"thread_main","threadId":"other"}`, `{"sessionId":"` + doc.SessionID + `","threadId":"thread_main","__proto__":{}}`} {
		result, e := adapter.Invoke(ctx, host.Call{Binding: binding, Principal: principal, ContributionID: "workspace-editor", Operation: "read-object", Input: json.RawMessage(raw)})
		if e != nil || !strings.Contains(string(result.Output), `"invalid_request"`) {
			t.Fatal("malformed accepted", e)
		}
	}
	ready = false
	state, e := adapter.Readiness(ctx, binding)
	if e != nil || state.Available {
		t.Fatal("revoked still ready", state, e)
	}
	if _, e = adapter.Invoke(ctx, host.Call{Binding: binding, Principal: principal, ContributionID: "workspace-editor", Operation: "read-object", Input: json.RawMessage(`{}`)}); e == nil {
		t.Fatal("revoked package invoked")
	}
	if _, e = adapter.Readiness(ctx, host.Binding{PackageID: "analytix-documents"}); e == nil {
		t.Fatal("wrong binding")
	}
}
