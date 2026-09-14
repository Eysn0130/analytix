package officeediting

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type nativeTestProjector struct {
	invalid bool
	seen    []editingapp.ScopeAuthority
}

func (p *nativeTestProjector) ValidateCurrent(_ context.Context, in editingapp.ScopeAuthority) error {
	p.seen = append(p.seen, in)
	if p.invalid {
		return editingapp.ErrProjection
	}
	return nil
}
func (p *nativeTestProjector) AuthorizeAndProject(ctx context.Context, in editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	if err := p.ValidateCurrent(ctx, in.ScopeAuthority); err != nil {
		return nil, err
	}
	if start := strings.Index(in.Text, "PRIVATE"); start >= 0 {
		return []editingapp.ProtectedRange{{StartByte: start, EndByte: start + 7}}, nil
	}
	return editingapp.ProtectedSelectionRanges(in.Text)
}
func selectionFixture(t *testing.T) (*Adapter, *fakeEditing, *nativeTestProjector, *int) {
	t.Helper()
	fake := &fakeEditing{document: editingapp.Opened{SessionID: strings.Repeat("b", 48), ObjectID: strings.Repeat("d", 64), Path: "/workspace/report.docx", Revision: strings.Repeat("c", 64), Content: "private-base64"}}
	projector := &nativeTestProjector{}
	leases := new(int)
	adapter := New("docx", &fakeNativeEditing{fakeEditing: fake}, func(context.Context) bool { return true })
	if err := adapter.BindSelectionHost(projector, func(_ context.Context, id, path string, read func() error) (func(), error) {
		if id != fake.document.SessionID || path != fake.document.Path {
			t.Fatal("lease binding")
		}
		if err := read(); err != nil {
			return nil, err
		}
		*leases++
		return func() { *leases-- }, nil
	}); err != nil {
		t.Fatal(err)
	}
	if out := invoke(t, adapter, validCall(t, "open-object", goodInput(t, "open-object"))); out["ok"] != true {
		t.Fatal(out)
	}
	return adapter, fake, projector, leases
}
func captureFields(fake *fakeEditing, editable bool) map[string]any {
	return map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread_main", "selectionToken": "native_selection_01", "changeSequence": 3, "baseRevision": fake.document.Revision, "text": "Before PRIVATE after", "editable": editable}
}
func nativeInvoke(t *testing.T, a *Adapter, operation string, input any) map[string]any {
	t.Helper()
	call := validCall(t, operation, requestBody(t, input))
	call.Binding.PackageID = a.packageID
	return invoke(t, a, call)
}
func captured(t *testing.T, a *Adapter, fake *fakeEditing, editable bool) map[string]any {
	t.Helper()
	out := nativeInvoke(t, a, "capture-selection", captureFields(fake, editable))
	if out["ok"] != true {
		t.Fatal(out)
	}
	return out["scope"].(map[string]any)
}
func modelFields(scope map[string]any) map[string]any {
	return map[string]any{"scopeId": scope["scopeId"], "threadId": scope["threadId"]}
}
func proposed(t *testing.T, a *Adapter, scope map[string]any) map[string]any {
	t.Helper()
	fields := modelFields(scope)
	fields["operationId"] = "propose_0001"
	fields["parts"] = scope["parts"]
	out := nativeInvoke(t, a, "model-selection-propose", fields)
	if out["ok"] != true {
		t.Fatal(out)
	}
	return out["result"].(map[string]any)
}
func decisionFields(scope, proposal map[string]any) map[string]any {
	return map[string]any{"sessionId": scope["sessionId"], "scopeId": scope["scopeId"], "proposalId": proposal["proposalId"], "operationId": "approve_0001", "selectionToken": scope["selectionToken"], "changeSequence": scope["changeSequence"], "baseRevision": scope["baseRevision"]}
}

func TestNativeSelectionEditableUTF16LimitAndDiscussionCapture(t *testing.T) {
	for _, text := range []string{strings.Repeat("x", 4096), strings.Repeat("é", 4096), strings.Repeat("😀", 2048)} {
		a, fake, _, _ := selectionFixture(t)
		fields := captureFields(fake, true)
		fields["text"] = text
		if out := nativeInvoke(t, a, "capture-selection", fields); out["ok"] != true {
			t.Fatal("exact UTF-16 limit rejected", out)
		}
		fields["text"] = text + "x"
		if out := nativeInvoke(t, a, "capture-selection", fields); out["code"] != "proposal_invalid" {
			t.Fatal("oversized editable capture accepted", out)
		}
		fields["editable"] = false
		if out := nativeInvoke(t, a, "capture-selection", fields); out["ok"] != true {
			t.Fatal("discussion capture inherited replacement limit", out)
		}
	}
}

func TestNativeSelectionProposalUTF16LimitBeforeApproval(t *testing.T) {
	for _, text := range []string{strings.Repeat("x", 4096), strings.Repeat("é", 4096), strings.Repeat("😀", 2048)} {
		a, fake, _, _ := selectionFixture(t)
		capture := captureFields(fake, true)
		capture["text"] = "Original"
		captured := nativeInvoke(t, a, "capture-selection", capture)["scope"].(map[string]any)
		fields := modelFields(captured)
		fields["operationId"] = "proposal_limit_01"
		fields["parts"] = []editingapp.PatchPart{{Kind: "literal", Text: text + "x"}}
		if out := nativeInvoke(t, a, "model-selection-propose", fields); out["code"] != "proposal_invalid" {
			t.Fatal("oversized proposal was created", out)
		}
		fields["parts"] = []editingapp.PatchPart{{Kind: "literal", Text: text}}
		out := nativeInvoke(t, a, "model-selection-propose", fields)
		if out["ok"] != true {
			t.Fatal("exact UTF-16 proposal rejected", out)
		}
		proposal := out["result"].(map[string]any)
		scope := a.scopes[captured["scopeId"].(string)]
		stored := scope.proposals[proposal["proposalId"].(string)]
		// Admission also protects against a retained candidate exceeding the
		// native protocol: approval must not advance its status or consume the op.
		stored.Parts[0].Text += "x"
		decision := decisionFields(captured, proposal)
		if result := nativeInvoke(t, a, "proposal-accept", decision); result["code"] != "proposal_invalid" || stored.Status != "proposed" || stored.decisionOperation != "" {
			t.Fatal("oversized replacement became approved", result)
		}
		stored.Parts[0].Text = text
		if result := nativeInvoke(t, a, "proposal-accept", decision); result["ok"] != true || result["replacement"].(map[string]any)["text"] != text {
			t.Fatal("exact UTF-16 replacement failed", result)
		}
	}
}

func TestNativeSelectionReplacementCountsPrivateSpansInUTF16Limit(t *testing.T) {
	ref := "protected_" + strings.Repeat("a", 48)
	scope := &nativeScope{Editable: true, protected: []editingapp.PatchPart{{Kind: "protected", ProtectedRef: ref, Text: "😀"}}}
	parts := []editingapp.PatchPart{{Kind: "literal", Text: strings.Repeat("x", 4094)}, {Kind: "protected", ProtectedRef: ref}}
	if _, err := renderNativeProposal(scope, parts); err != nil {
		t.Fatal("protected span at exact boundary rejected", err)
	}
	parts[0].Text += "x"
	if _, err := renderNativeProposal(scope, parts); !errors.Is(err, editingapp.ErrProposal) {
		t.Fatal("private span was omitted from UTF-16 count", err)
	}
}

func TestNativeSelectionProposalProtectedReplacementAndReplay(t *testing.T) {
	a, fake, projector, leases := selectionFixture(t)
	scope := captured(t, a, fake, true)
	if *leases != 1 {
		t.Fatal("missing managed lease")
	}
	if strings.Contains(requestBody(t, scope), "PRIVATE") || a.sessions[fake.document.SessionID].document.Content != "" {
		t.Fatal("raw native bytes retained in projection")
	}
	read := nativeInvoke(t, a, "model-selection-read", modelFields(scope))
	if read["ok"] != true || strings.Contains(requestBody(t, read), "selectionToken") || strings.Contains(requestBody(t, read), "sessionId") {
		t.Fatal(read)
	}
	proposal := proposed(t, a, scope)
	fields := decisionFields(scope, proposal)
	result := nativeInvoke(t, a, "proposal-accept", fields)
	if result["ok"] != true || result["replacement"].(map[string]any)["text"] != "Before PRIVATE after" {
		t.Fatal(result)
	}
	replay := nativeInvoke(t, a, "proposal-accept", fields)
	if !reflect.DeepEqual(result, replay) {
		t.Fatal("accept replay changed")
	}
	if got := proposed(t, a, scope); got["status"] != "approved" {
		t.Fatal("create replay lost approved status")
	}
	fields["operationId"] = "approve_0002"
	if out := nativeInvoke(t, a, "proposal-accept", fields); out["code"] != "proposal_invalid" {
		t.Fatal(out)
	}
	for _, call := range fake.calls {
		if call == "commit" || call == "native-commit" {
			t.Fatal("proposal persisted native file")
		}
	}
	if len(projector.seen) == 0 || projector.seen[0].ThreadID != "thread_main" || projector.seen[0].ObjectID != fake.document.ObjectID {
		t.Fatal("missing thread/object authority")
	}
	if out := nativeInvoke(t, a, "close-object", map[string]any{"sessionId": scope["sessionId"]}); out["ok"] != true || *leases != 0 {
		t.Fatal("lease not released on session close")
	}
}

func TestNativeSelectionRefusesStaleAndForgedTargets(t *testing.T) {
	for _, field := range []string{"selectionToken", "changeSequence", "baseRevision"} {
		t.Run(field, func(t *testing.T) {
			a, fake, _, _ := selectionFixture(t)
			scope := captured(t, a, fake, true)
			proposal := proposed(t, a, scope)
			fields := decisionFields(scope, proposal)
			if field == "changeSequence" {
				fields[field] = 4
			} else {
				fields[field] = "different_target"
			}
			if out := nativeInvoke(t, a, "proposal-accept", fields); out["code"] != "draft_stale" {
				t.Fatal(out)
			}
		})
	}
	a, fake, p, _ := selectionFixture(t)
	scope := captured(t, a, fake, true)
	fields := modelFields(scope)
	fields["threadId"] = "another_thread"
	if out := nativeInvoke(t, a, "model-selection-read", fields); out["code"] != "scope_invalid" {
		t.Fatal(out)
	}
	call := validCall(t, "model-selection-read", requestBody(t, modelFields(scope)))
	call.Binding.GenerationID = strings.Repeat("e", 64)
	if out := invoke(t, a, call); out["code"] != "scope_invalid" {
		t.Fatal(out)
	}
	p.invalid = true
	if out := nativeInvoke(t, a, "model-selection-read", modelFields(scope)); out["code"] != "projection_unavailable" {
		t.Fatal(out)
	}
	p.invalid = false
	fake.document.Revision = strings.Repeat("e", 64)
	if out := nativeInvoke(t, a, "model-selection-read", modelFields(scope)); out["code"] != "draft_stale" {
		t.Fatal(out)
	}
}

func TestNativeSelectionRejectRevokeAndDiscussionOnly(t *testing.T) {
	a, fake, _, _ := selectionFixture(t)
	scope := captured(t, a, fake, true)
	proposal := proposed(t, a, scope)
	reject := map[string]any{"sessionId": scope["sessionId"], "scopeId": scope["scopeId"], "proposalId": proposal["proposalId"], "operationId": "reject_0001"}
	for i := 0; i < 2; i++ {
		if out := nativeInvoke(t, a, "proposal-reject", reject); out["ok"] != true || out["proposal"].(map[string]any)["status"] != "rejected" {
			t.Fatal(out)
		}
	}
	if out := nativeInvoke(t, a, "selection-read", map[string]any{"sessionId": scope["sessionId"], "scopeId": scope["scopeId"]}); out["ok"] != true {
		t.Fatal("reject changed scope")
	}
	if out := nativeInvoke(t, a, "selection-revoke", map[string]any{"sessionId": scope["sessionId"], "scopeId": scope["scopeId"]}); out["ok"] != true {
		t.Fatal(out)
	}
	if out := nativeInvoke(t, a, "model-selection-read", modelFields(scope)); out["code"] != "scope_invalid" {
		t.Fatal(out)
	}
	if out := nativeInvoke(t, a, "proposal-read", map[string]any{"sessionId": scope["sessionId"], "scopeId": scope["scopeId"]}); out["ok"] != true || len(out["proposals"].([]any)) != 1 {
		t.Fatal("revocation lost safe proposal summary", out)
	}
	if a.scopes[scope["scopeId"].(string)].protected != nil {
		t.Fatal("revocation retained protected text")
	}
	scope = captured(t, a, fake, false)
	fields := modelFields(scope)
	fields["operationId"] = "propose_0002"
	fields["parts"] = scope["parts"]
	if out := nativeInvoke(t, a, "model-selection-propose", fields); out["code"] != "scope_invalid" {
		t.Fatal("discussion scope mutated", out)
	}
}

func TestNativeSelectionProtectedReferencesAndNoPublicBody(t *testing.T) {
	a, fake, _, _ := selectionFixture(t)
	scope := captured(t, a, fake, true)
	original := scope["parts"].([]any)
	for _, parts := range []any{[]any{}, []any{original[0], original[2]}, []any{original[1], original[1]}, []any{map[string]any{"kind": "literal", "text": "PRIVATE"}, original[1]}, []any{map[string]any{"kind": "literal", "text": "PERSON_1"}, original[1]}, []any{map[string]any{"kind": "protected", "protectedRef": "protected_" + strings.Repeat("f", 48)}}} {
		fields := modelFields(scope)
		fields["operationId"] = "propose_0001"
		fields["parts"] = parts
		if out := nativeInvoke(t, a, "model-selection-propose", fields); out["code"] != "protected_span_invalid" {
			t.Fatal(out)
		}
	}
	publicArgs := map[string]any{"scopeId": scope["scopeId"], "operationId": "propose_0001", "parts": original}
	if err := domainevent.ValidatePublicRecord(map[string]any{"kind": "tool_call", "arguments": publicArgs}); err != nil {
		t.Fatal("projected arguments fail public boundary", err)
	}
	public := toolcatalogapp.BuildPublicToolResultProjectionV1("native_selection_read", scope, false)
	body, _ := json.Marshal(public)
	if strings.Contains(string(body), "Before") || strings.Contains(string(body), "PRIVATE") || strings.Contains(string(body), "selectionToken") {
		t.Fatal("scope body reached public result")
	}
	if _, err := decodeNativeParts([]any{map[string]any{"kind": "literal", "text": "safe", "path": "/private"}}); !errors.Is(err, fileport.ErrInvalidInput) {
		t.Fatal("unknown patch selector admitted")
	}
}

var _ adapterport.Adapter = (*Adapter)(nil)
