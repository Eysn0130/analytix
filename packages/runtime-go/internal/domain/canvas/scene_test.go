package canvas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func fixtureScene() Scene {
	nodeStyle := Style{"#ffffff", "#102030", 2, "solid", "rounded"}
	edgeStyle := Style{"#ffffff", "#102030", 1, "dashed", "line"}
	return Scene{1, Facts{
		[]Node{{"n2", "Concept", map[string]any{}, []SourceRef{}, true}, {"n1", "事实 <>&\u2028", map[string]any{"z": nil, "amount": 0.25, "active": true}, []SourceRef{{"s1", "😀", "astral"}, {"s1", "\ue000", "BMP"}, {"s2", "p:2", ""}}, false}},
		[]Edge{{"e2", "n1", "n2", "Assumed", "hypothesis", map[string]any{}, []SourceRef{}, true}, {"e1", "n1", "n2", "Transfer", "transfer", map[string]any{"currency": "CNY"}, []SourceRef{{"s3", "row:7", "source reference only"}}, false}},
	}, Presentation{[]NodeView{{"n1", Layout{0, 0, 100, 60}, "Original display", nodeStyle}, {"n2", Layout{200, 50, 100, 60}, "Concept", nodeStyle}}, []EdgeView{{"e1", []Point{}, "Transfer", edgeStyle}, {"e2", []Point{{0, 0}, {200, 50}}, "Assumed", edgeStyle}}}}
}
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSceneRoundTripEditsPreserveCompleteFacts(t *testing.T) {
	s, err := ParseScene(mustJSON(t, fixtureScene()))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		s.Facts.Nodes[0].Sources = append(s.Facts.Nodes[0].Sources, SourceRef{SourceID: fmt.Sprintf("evidence-%d", i), Locator: fmt.Sprintf("row:%d", i), Note: "retained"})
	}
	original := mustJSON(t, s)
	digest, err := FactsDigest(s)
	if err != nil {
		t.Fatal(err)
	}
	label := "Display only"
	style := Style{"#e0e0e0", "#000000", 3, "solid", "ellipse"}
	ops := []Operation{{Kind: "set-node-layout", ID: "n1", Layout: &Layout{20, 30, 120, 80}}, {Kind: "set-edge-route", ID: "e1", Points: []Point{{20, 30}, {100, 70}, {200, 50}}}, {Kind: "set-display-label", ID: "e1", Target: "edge", DisplayLabel: &label}, {Kind: "set-style", ID: "n2", Target: "node", Style: &style}}
	result, err := Apply(s, ops)
	if err != nil {
		t.Fatal(err)
	}
	if result.FactsDigest != digest || !reflect.DeepEqual(result.Scene.Facts, s.Facts) || len(result.Diff) != 4 {
		t.Fatalf("facts or diff changed: %+v", result.Diff)
	}
	if result.Diff[2].FactLabel != "Transfer" || string(result.Diff[2].Before) != `"Transfer"` || string(result.Diff[2].After) != `"Display only"` {
		t.Fatal("display diff lost original fact label")
	}
	if !bytes.Equal(original, mustJSON(t, s)) {
		t.Fatal("mutated caller scene")
	}
	if _, err := ParseScene(mustJSON(t, result.Scene)); err != nil {
		t.Fatal(err)
	}
	ops[1].Points[0].X = 999
	if result.Scene.Presentation.Edges[0].Points[0].X != 20 {
		t.Fatal("retained operation pointer")
	}
	// Parallel edges retain independent IDs even when their endpoints coincide.
	if result.Scene.Presentation.Edges[1].DisplayLabel != "Assumed" {
		t.Fatal("changed parallel edge")
	}
}

func TestFactsDigestSharedVectorAndOrderIndependence(t *testing.T) {
	s := fixtureScene()
	got, err := FactsDigest(s)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "c5a9d7aea6eef9439c40c12d224510bcba06eac4db2298732f09b5c543116bc2"
	if got != expected {
		t.Fatalf("cross-language vector: got %s", got)
	}
	s.Facts.Nodes[0], s.Facts.Nodes[1] = s.Facts.Nodes[1], s.Facts.Nodes[0]
	s.Facts.Edges[0], s.Facts.Edges[1] = s.Facts.Edges[1], s.Facts.Edges[0]
	refs := s.Facts.Nodes[0].Sources
	refs[0], refs[2] = refs[2], refs[0]
	s.Presentation.Nodes[0].DisplayLabel = "new display"
	s.Presentation.Nodes[0].Layout.X = 999
	reordered, err := FactsDigest(s)
	if err != nil || reordered != got {
		t.Fatal("digest depends on order/presentation", err)
	}
	s.Facts.Nodes[0].Label = "Changed fact"
	changed, err := FactsDigest(s)
	if err != nil || changed == got {
		t.Fatal("digest ignores factual label")
	}
}

func TestSceneRejectsAmbiguityAndIncompleteFacts(t *testing.T) {
	raw := string(mustJSON(t, fixtureScene()))
	for name, input := range map[string]string{
		"duplicate":       strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		"unknown":         strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"owner":"fake"`, 1),
		"case alias":      strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"SchemaVersion":1`, 1),
		"missing false":   strings.Replace(raw, `,"assumption":false`, "", 1),
		"null array":      strings.Replace(raw, `"sources":[]`, `"sources":null`, 1),
		"null attributes": strings.Replace(raw, `"attributes":{}`, `"attributes":null`, 1),
		"surrogate":       strings.Replace(raw, `"Concept"`, `"\ud800"`, 1),
		"trailing":        raw + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseScene([]byte(input)); err == nil {
				t.Fatal("accepted malformed scene")
			}
		})
	}
	changes := map[string]func(*Scene){
		"duplicate node":       func(s *Scene) { s.Facts.Nodes[0].ID = "n1" },
		"duplicate edge":       func(s *Scene) { s.Facts.Edges[0].ID = "e1" },
		"unknown endpoint":     func(s *Scene) { s.Facts.Edges[0].To = "missing" },
		"missing presentation": func(s *Scene) { s.Presentation.Edges = s.Presentation.Edges[:1] },
		"unbounded position":   func(s *Scene) { s.Presentation.Nodes[0].Layout.X = MaxCoordinate },
		"unsafe style":         func(s *Scene) { s.Presentation.Nodes[0].Style.Fill = `url(file:///private)` },
		"nested fact":          func(s *Scene) { s.Facts.Nodes[0].Attributes["nested"] = map[string]any{"x": 1} },
		"source duplicate": func(s *Scene) {
			s.Facts.Nodes[1].Sources = append(s.Facts.Nodes[1].Sources, s.Facts.Nodes[1].Sources[0])
		},
		"oversized fact": func(s *Scene) { s.Facts.Nodes[0].Label = strings.Repeat("x", 1025) },
		"too many nodes": func(s *Scene) { s.Facts.Nodes = make([]Node, MaxNodes+1) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			s := fixtureScene()
			change(&s)
			if ValidateScene(s) == nil {
				t.Fatal("accepted invalid scene")
			}
		})
	}
}

func TestOperationsRejectUnknownDuplicateAndPartialChanges(t *testing.T) {
	bad := []string{
		`[]`,
		`[{"kind":"set-node-layout","id":"n1","layout":{"x":0,"y":0,"width":10}}]`,
		`[{"kind":"set-node-layout","id":"n1","layout":{"x":0,"y":0,"width":10,"height":10},"target":""}]`,
		`[{"kind":"set-display-label","id":"n1","target":"node","displayLabel":"x","label":"alter fact"}]`,
		`[{"kind":"set-display-label","id":"n1","target":"node","displayLabel":"x","ID":"n2"}]`,
		`[{"kind":"set-display-label","id":"n1","target":"node","displayLabel":"x"},{"kind":"set-display-label","id":"n1","target":"node","displayLabel":"y"}]`,
		`[{"kind":"set-edge-route","id":"e1","points":[{"x":0,"y":0}]}]`,
		`[{"kind":"delete-node","id":"n1"}]`,
	}
	for _, raw := range bad {
		if _, err := ParseOperations([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	label := ""
	result, err := Apply(fixtureScene(), []Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &label}})
	if err != nil || result.Scene.Presentation.Nodes[0].DisplayLabel != "" {
		t.Fatal("cannot clear display label", err)
	}
	if _, err := Apply(fixtureScene(), []Operation{{Kind: "set-display-label", ID: "missing", Target: "node", DisplayLabel: &label}}); err == nil {
		t.Fatal("accepted unknown ID")
	}
}

func TestTypedSceneDoesNotRepairInvalidFactText(t *testing.T) {
	s := fixtureScene()
	s.Facts.Nodes[0].Label = string([]byte{0xff})
	if ValidateScene(s) == nil {
		t.Fatal("silently repaired invalid factual UTF-8")
	}
	bad := string([]byte{0xff})
	if _, err := Apply(fixtureScene(), []Operation{{Kind: "set-display-label", ID: "n1", Target: "node", DisplayLabel: &bad}}); err == nil {
		t.Fatal("silently repaired invalid display UTF-8")
	}
}
