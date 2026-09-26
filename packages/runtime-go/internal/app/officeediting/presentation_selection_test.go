package officeediting

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	office "analytix.local/runtime-go/internal/domain/officegeneration"
)

type presentationEditingStub struct {
	*fakeNativeEditing
	selected office.PresentationSelection
}

func (f *presentationEditingStub) ValidateNativePresentation(_ context.Context, id, base string, s office.PresentationSelection, p *office.PresentationPatch) (*office.PresentationReview, error) {
	if id != f.document.SessionID || base != f.document.Revision || !reflect.DeepEqual(s, f.selected) {
		return nil, editingapp.ErrDraftStale
	}
	if p == nil {
		return nil, nil
	}
	r, err := office.ApplyPresentationPatch(s, *p)
	return &r, err
}
func nativePresentationFixture(t *testing.T) (*Adapter, *fakeEditing, *nativeTestProjector, office.PresentationSelection) {
	t.Helper()
	fake := &fakeEditing{document: editingapp.Opened{SessionID: strings.Repeat("b", 48), ObjectID: strings.Repeat("d", 64), Path: "/workspace/report.pptx", Revision: strings.Repeat("c", 64), Content: "synthetic"}}
	s := office.PresentationSelection{PageWidth100thMM: 28000, PageHeight100thMM: 15750, Shapes: []office.PresentationShape{{Kind: "rectangle", Name: "Target", Text: "Synthetic", X100thMM: 1000, Y100thMM: 1000, Width100thMM: 4000, Height100thMM: 2000, FillRGB: "#ffffff"}}}
	projector := &nativeTestProjector{}
	a := New("pptx", &presentationEditingStub{fakeNativeEditing: &fakeNativeEditing{fakeEditing: fake}, selected: s}, func(context.Context) bool { return true })
	if e := a.BindSelectionHost(projector, func(_ context.Context, _, _ string, read func() error) (func(), error) {
		if e := read(); e != nil {
			return nil, e
		}
		return func() {}, nil
	}); e != nil {
		t.Fatal(e)
	}
	if out := nativeInvoke(t, a, "open-object", map[string]any{"object": map[string]any{"workspace": "/workspace", "path": fake.document.Path}}); out["ok"] != true {
		t.Fatal(out)
	}
	return a, fake, projector, s
}
func TestNativePresentationScopesAndProposalsAreTypedAndProjected(t *testing.T) {
	a, fake, projector, s := nativePresentationFixture(t)
	fields := captureFields(fake, true)
	fields["text"] = "Synthetic"
	fields["presentation"] = s
	for _, bad := range []any{nil, map[string]any{"pageIndex": 0}} {
		fields["presentation"] = bad
		if out := nativeInvoke(t, a, "capture-selection", fields); out["ok"] == true {
			t.Fatal("bad capture admitted")
		}
	}
	fields["presentation"] = s
	fields["workbook"] = map[string]any{}
	if nativeInvoke(t, a, "capture-selection", fields)["ok"] == true {
		t.Fatal("mixed capture admitted")
	}
	delete(fields, "workbook")
	private := s
	private.Shapes = append([]office.PresentationShape{}, s.Shapes...)
	private.Shapes[0].Text = "PRIVATE"
	fields["presentation"] = private
	if nativeInvoke(t, a, "capture-selection", fields)["ok"] == true {
		t.Fatal("forged snapshot admitted")
	}
	fields["presentation"] = s
	out := nativeInvoke(t, a, "capture-selection", fields)
	if out["ok"] != true {
		t.Fatal(out)
	}
	scope := out["scope"].(map[string]any)
	read := nativeInvoke(t, a, "model-selection-read", modelFields(scope))
	if read["ok"] != true {
		t.Fatal(read)
	}
	readBody, _ := json.Marshal(read["result"].(map[string]any)["presentation"])
	if strings.Contains(string(readBody), "Synthetic") || strings.Contains(string(readBody), "Target") || strings.Contains(string(readBody), `"name"`) || strings.Contains(string(readBody), `"text"`) {
		t.Fatal("model read exposed original shape name/text", string(readBody))
	}
	input := modelFields(scope)
	input["operationId"] = "presentation_propose_01"
	input["presentation"] = map[string]any{"kind": "shape-fill", "rgb": "#123456"}
	for _, bad := range []any{nil, map[string]any{"kind": "chart-data"}, map[string]any{"kind": "shape-fill", "rgb": "#ffffff"}, map[string]any{"kind": "shape-fill", "rgb": "#123456", "shapeIndex": 0}} {
		input["presentation"] = bad
		if nativeInvoke(t, a, "model-selection-propose", input)["ok"] == true {
			t.Fatal("bad proposal admitted", bad)
		}
	}
	input["presentation"] = map[string]any{"kind": "shape-fill", "rgb": "#123456"}
	input["parts"] = []any{}
	if nativeInvoke(t, a, "model-selection-propose", input)["ok"] == true {
		t.Fatal("mixed proposal admitted")
	}
	delete(input, "parts")
	out = nativeInvoke(t, a, "model-selection-propose", input)
	if out["ok"] != true {
		t.Fatal(out)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "beforeText") || strings.Contains(string(encoded), "pageWidth100thMm") {
		t.Fatal("review leaked into model proposal")
	}
	input["presentation"] = map[string]any{"kind": "shape-fill", "rgb": "#234567"}
	if nativeInvoke(t, a, "model-selection-propose", input)["ok"] == true {
		t.Fatal("operation identity reused")
	}
	projector.invalid = true
	if nativeInvoke(t, a, "model-selection-read", modelFields(scope))["ok"] == true {
		t.Fatal("revoked authority read")
	}
}

func TestNativePresentationRetainsProtectedTextProposalRoute(t *testing.T) {
	a, fake, _, s := nativePresentationFixture(t)
	s.Shapes[0].Name = "PRIVATE"
	s.Shapes[0].Text = "Before PRIVATE after"
	stub := a.service.(*presentationEditingStub)
	stub.selected = s
	fields := captureFields(fake, true)
	fields["presentation"] = s
	out := nativeInvoke(t, a, "capture-selection", fields)
	if out["ok"] != true {
		t.Fatal(out)
	}
	scope := out["scope"].(map[string]any)
	read := nativeInvoke(t, a, "model-selection-read", modelFields(scope))
	encoded, _ := json.Marshal(read)
	if read["ok"] != true || strings.Contains(string(encoded), "PRIVATE") || !strings.Contains(string(encoded), "protectedRef") {
		t.Fatal("selected text lost its protected projection", string(encoded))
	}
	proposal := proposed(t, a, scope)
	accepted := nativeInvoke(t, a, "proposal-accept", decisionFields(scope, proposal))
	if accepted["ok"] != true {
		t.Fatal(accepted)
	}
	replacement := accepted["replacement"].(map[string]any)
	if replacement["text"] != "Updated PRIVATE after" || replacement["presentation"] != nil || stub.draft.Presentation != nil || stub.draft.Workbook != nil {
		t.Fatal("text proposal acquired a typed review", accepted, stub.draft)
	}
}
