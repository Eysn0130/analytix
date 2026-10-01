package officegeneration

import (
	"encoding/json"
	"strings"
	"testing"
)

func presentationTestSelection() PresentationSelection {
	return PresentationSelection{PageWidth100thMM: 28000, PageHeight100thMM: 15750, Shapes: []PresentationShape{{Kind: "rectangle", Name: "Target", Text: "Synthetic", X100thMM: 1000, Y100thMM: 1000, Width100thMM: 4000, Height100thMM: 2000, FillRGB: "#ffffff"}, {ShapeIndex: 1, Kind: "ellipse", Name: "Neighbor", X100thMM: 6000, Y100thMM: 1000, Width100thMM: 3000, Height100thMM: 2000, FillRGB: "#3478a0"}}}
}
func TestPresentationPatchStrictFiniteVariants(t *testing.T) {
	s := presentationTestSelection()
	body, _ := json.Marshal(s)
	if _, err := ParsePresentationSelection(body); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"kind":"shape-fill","rgb":"#123456"}`, `{"kind":"shape-geometry","x100thMm":0,"y100thMm":0,"width100thMm":4000,"height100thMm":2000}`} {
		p, err := ParsePresentationPatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		r, err := ApplyPresentationPatch(s, p)
		if err != nil || ValidatePresentationReview(r) != nil {
			t.Fatal(err)
		}
		if s.Shapes[0].FillRGB != "#ffffff" || s.Shapes[0].X100thMM != 1000 {
			t.Fatal("input mutated")
		}
		if _, err = ApplyPresentationPatch(r.After, p); err == nil {
			t.Fatal("no-op admitted")
		}
		r.After.Shapes[1].Name = "changed"
		if ValidatePresentationReview(r) == nil {
			t.Fatal("neighbor edit admitted")
		}
	}
	for _, raw := range []string{`null`, `{"kind":"chart-data","rgb":"#123456"}`, `{"kind":"shape-fill","rgb":"#ABCDEF"}`, `{"kind":"shape-fill","rgb":"#123456","targetShapeIndex":1}`, `{"kind":"shape-fill","rgb":"#123456","x100thMm":null}`, `{"kind":"shape-fill","rgb":"#123456","rgb":"#234567"}`, `{"kind":"shape-geometry","x100thMm":0.5,"y100thMm":0,"width100thMm":1,"height100thMm":1}`, `{"kind":"shape-geometry","x100thMm":0,"y100thMm":0,"width100thMm":0,"height100thMm":1}`} {
		if _, err := ParsePresentationPatch([]byte(raw)); err == nil {
			t.Fatalf("invalid patch admitted: %s", raw)
		}
	}
	for _, mutate := range []func(*PresentationSelection){func(s *PresentationSelection) { s.Shapes = s.Shapes[:1]; s.TargetShapeIndex = 1 }, func(s *PresentationSelection) { s.Shapes[0].ShapeIndex = 1 }, func(s *PresentationSelection) { s.Shapes[0].X100thMM = 28000 }, func(s *PresentationSelection) { s.Shapes[0].Kind = "chart" }, func(s *PresentationSelection) { s.Shapes[0].Text = strings.Repeat("x", 4097) }} {
		bad := presentationTestSelection()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid snapshot admitted")
		}
	}
	for _, raw := range []string{strings.Replace(string(body), `"pageIndex":0`, `"pageIndex":null`, 1), strings.Replace(string(body), `"fillRGB":"#ffffff"`, `"fillRGB":null`, 1)} {
		if _, err := ParsePresentationSelection([]byte(raw)); err == nil {
			t.Fatal("null admitted")
		}
	}
}
