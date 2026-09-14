// Package canvas owns bounded, data-only scenes and image transformations.
// References and digests are content identifiers, never provenance or permission.
package canvas

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	MaxSceneBytes = 4 << 20
	MaxNodes      = 256
	MaxEdges      = 1024
	MaxOperations = 128
	MaxCoordinate = 1000000
)

var ErrInvalid = errors.New("canvas_invalid_data")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
var attributeKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type SourceRef struct {
	SourceID string `json:"sourceId"`
	Locator  string `json:"locator,omitempty"`
	Note     string `json:"note,omitempty"`
}
type Node struct {
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Attributes map[string]any `json:"attributes"`
	Sources    []SourceRef    `json:"sources"`
	Assumption bool           `json:"assumption"`
}
type Edge struct {
	ID         string         `json:"id"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Label      string         `json:"label"`
	Relation   string         `json:"relation"`
	Attributes map[string]any `json:"attributes"`
	Sources    []SourceRef    `json:"sources"`
	Assumption bool           `json:"assumption"`
}
type Facts struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Layout struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type Style struct {
	Fill        string `json:"fill"`
	Stroke      string `json:"stroke"`
	StrokeWidth int    `json:"strokeWidth"`
	Dash        string `json:"dash"`
	Shape       string `json:"shape"`
}
type NodeView struct {
	ID           string `json:"id"`
	Layout       Layout `json:"layout"`
	DisplayLabel string `json:"displayLabel"`
	Style        Style  `json:"style"`
}
type EdgeView struct {
	ID           string  `json:"id"`
	Points       []Point `json:"points"`
	DisplayLabel string  `json:"displayLabel"`
	Style        Style   `json:"style"`
}
type Presentation struct {
	Nodes []NodeView `json:"nodes"`
	Edges []EdgeView `json:"edges"`
}
type Scene struct {
	SchemaVersion int          `json:"schemaVersion"`
	Facts         Facts        `json:"facts"`
	Presentation  Presentation `json:"presentation"`
}

type Operation struct {
	Kind         string  `json:"kind"`
	ID           string  `json:"id"`
	Target       string  `json:"target,omitempty"`
	Layout       *Layout `json:"layout,omitempty"`
	Points       []Point `json:"points,omitempty"`
	DisplayLabel *string `json:"displayLabel,omitempty"`
	Style        *Style  `json:"style,omitempty"`
}
type Change struct {
	Kind      string          `json:"kind"`
	Target    string          `json:"target"`
	ID        string          `json:"id"`
	Field     string          `json:"field"`
	FactLabel string          `json:"factLabel"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
}
type Result struct {
	Scene       Scene    `json:"scene"`
	FactsDigest string   `json:"factsDigest"`
	Diff        []Change `json:"diff"`
}

// Required fields must be present even when their legitimate value is 0/false.
// Unlike encoding/json alone this also rejects explicit null for typed fields.
func shape(t reflect.Type, value any) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Interface {
		return nil
	}
	if value == nil {
		return ErrInvalid
	}
	switch t.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return ErrInvalid
		}
		allowed := map[string]bool{}
		for i := 0; i < t.NumField(); i++ {
			allowed[strings.Split(t.Field(i).Tag.Get("json"), ",")[0]] = true
		}
		for key := range object {
			if !allowed[key] {
				return ErrInvalid
			}
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			v, exists := object[tag[0]]
			if !exists {
				if len(tag) > 1 && tag[1] == "omitempty" {
					continue
				}
				return ErrInvalid
			}
			if err := shape(f.Type, v); err != nil {
				return err
			}
		}
	case reflect.Slice:
		list, ok := value.([]any)
		if !ok {
			return ErrInvalid
		}
		for _, v := range list {
			if err := shape(t.Elem(), v); err != nil {
				return err
			}
		}
	case reflect.Map:
		object, ok := value.(map[string]any)
		if !ok {
			return ErrInvalid
		}
		for _, v := range object {
			if err := shape(t.Elem(), v); err != nil {
				return err
			}
		}
	}
	return nil
}
func decode(raw []byte, target any, max int) error {
	if jsonstrict.Validate(raw, jsonstrict.Options{MaxBytes: max, MaxDepth: 12, MaxTokens: 1 << 19, MaxStringBytes: 4096}) != nil {
		return ErrInvalid
	}
	var generic any
	if json.Unmarshal(raw, &generic) != nil || shape(reflect.TypeOf(target).Elem(), generic) != nil {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if decoder.Decode(target) != nil {
		return ErrInvalid
	}
	return nil
}
func boundedText(s string, max int) bool {
	return utf8.ValidString(s) && len(s) <= max && !strings.ContainsRune(s, 0)
}
func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= MaxCoordinate
}
func validLayout(v Layout) bool {
	return finite(v.X) && finite(v.Y) && finite(v.Width) && finite(v.Height) && v.Width > 0 && v.Height > 0 && math.Abs(v.X+v.Width) <= MaxCoordinate && math.Abs(v.Y+v.Height) <= MaxCoordinate
}
func validPoints(points []Point) bool {
	if len(points) > 32 {
		return false
	}
	for _, p := range points {
		if !finite(p.X) || !finite(p.Y) {
			return false
		}
	}
	return len(points) == 0 || len(points) >= 2
}
func validStyle(s Style, target string) bool {
	if !colorPattern.MatchString(s.Fill) || !colorPattern.MatchString(s.Stroke) || s.StrokeWidth < 1 || s.StrokeWidth > 16 || (s.Dash != "solid" && s.Dash != "dashed") {
		return false
	}
	if target == "edge" {
		return s.Shape == "line"
	}
	return s.Shape == "rectangle" || s.Shape == "rounded" || s.Shape == "ellipse"
}
func normalizeAttributes(attributes map[string]any) bool {
	if attributes == nil || len(attributes) > 32 {
		return false
	}
	for key, value := range attributes {
		if !attributeKey.MatchString(key) {
			return false
		}
		switch value := value.(type) {
		case nil, bool:
		case string:
			if !boundedText(value, 2048) {
				return false
			}
		case json.Number:
			number, err := strconv.ParseFloat(string(value), 64)
			if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || math.Abs(number) > 9007199254740991 {
				return false
			}
			if number == 0 {
				number = 0
			}
			attributes[key] = number
		case float64:
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 9007199254740991 {
				return false
			}
			if value == 0 {
				attributes[key] = float64(0)
			}
		default:
			return false
		}
	}
	return true
}
func validSources(refs []SourceRef) bool {
	if refs == nil || len(refs) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		key := ref.SourceID + "\x00" + ref.Locator
		if !idPattern.MatchString(ref.SourceID) || !boundedText(ref.Locator, 1024) || !boundedText(ref.Note, 2048) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}
func validateScene(s Scene) error {
	if s.SchemaVersion != 1 || s.Facts.Nodes == nil || s.Facts.Edges == nil || s.Presentation.Nodes == nil || s.Presentation.Edges == nil || len(s.Facts.Nodes) > MaxNodes || len(s.Facts.Edges) > MaxEdges || len(s.Presentation.Nodes) != len(s.Facts.Nodes) || len(s.Presentation.Edges) != len(s.Facts.Edges) {
		return ErrInvalid
	}
	nodes, edges := map[string]bool{}, map[string]bool{}
	for _, n := range s.Facts.Nodes {
		if !idPattern.MatchString(n.ID) || nodes[n.ID] || !boundedText(n.Label, 1024) || !normalizeAttributes(n.Attributes) || !validSources(n.Sources) {
			return ErrInvalid
		}
		nodes[n.ID] = true
	}
	for _, e := range s.Facts.Edges {
		if !idPattern.MatchString(e.ID) || edges[e.ID] || nodes[e.ID] || !nodes[e.From] || !nodes[e.To] || !boundedText(e.Label, 1024) || !boundedText(e.Relation, 128) || e.Relation == "" || !normalizeAttributes(e.Attributes) || !validSources(e.Sources) {
			return ErrInvalid
		}
		edges[e.ID] = true
	}
	seen := map[string]bool{}
	for _, n := range s.Presentation.Nodes {
		if !nodes[n.ID] || seen[n.ID] || !validLayout(n.Layout) || !boundedText(n.DisplayLabel, 1024) || !validStyle(n.Style, "node") {
			return ErrInvalid
		}
		seen[n.ID] = true
	}
	for _, e := range s.Presentation.Edges {
		if !edges[e.ID] || seen[e.ID] || e.Points == nil || !validPoints(e.Points) || !boundedText(e.DisplayLabel, 1024) || !validStyle(e.Style, "edge") {
			return ErrInvalid
		}
		seen[e.ID] = true
	}
	return nil
}
func ParseScene(raw []byte) (Scene, error) {
	var s Scene
	if decode(raw, &s, MaxSceneBytes) != nil || validateScene(s) != nil {
		return Scene{}, ErrInvalid
	}
	return s, nil
}
func cloneScene(s Scene) (Scene, error) {
	if len(s.Facts.Nodes) > MaxNodes || len(s.Facts.Edges) > MaxEdges || len(s.Presentation.Nodes) > MaxNodes || len(s.Presentation.Edges) > MaxEdges {
		return Scene{}, ErrInvalid
	}
	// encoding/json repairs invalid UTF-8 in Go strings. Reject before encoding
	// so a typed caller cannot silently change factual text or reference notes.
	validFactText := func(label string, attributes map[string]any, sources []SourceRef) bool {
		if !boundedText(label, 1024) || !validSources(sources) || len(attributes) > 32 {
			return false
		}
		for key, value := range attributes {
			if !utf8.ValidString(key) {
				return false
			}
			if text, ok := value.(string); ok && !boundedText(text, 2048) {
				return false
			}
		}
		return true
	}
	for _, n := range s.Facts.Nodes {
		if !validFactText(n.Label, n.Attributes, n.Sources) {
			return Scene{}, ErrInvalid
		}
	}
	for _, e := range s.Facts.Edges {
		if !validFactText(e.Label, e.Attributes, e.Sources) || !boundedText(e.Relation, 128) {
			return Scene{}, ErrInvalid
		}
	}
	for _, n := range s.Presentation.Nodes {
		if !boundedText(n.DisplayLabel, 1024) {
			return Scene{}, ErrInvalid
		}
	}
	for _, e := range s.Presentation.Edges {
		if !boundedText(e.DisplayLabel, 1024) {
			return Scene{}, ErrInvalid
		}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return Scene{}, ErrInvalid
	}
	return ParseScene(raw)
}
func ValidateScene(s Scene) error { _, err := cloneScene(s); return err }
func sortedSources(s []SourceRef) []SourceRef {
	r := append([]SourceRef{}, s...)
	sort.Slice(r, func(i, j int) bool {
		a, b := r[i], r[j]
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		return a.Locator < b.Locator
	})
	return r
}
func factDigest(s Scene) string {
	facts := Facts{Nodes: append([]Node{}, s.Facts.Nodes...), Edges: append([]Edge{}, s.Facts.Edges...)}
	sort.Slice(facts.Nodes, func(i, j int) bool { return facts.Nodes[i].ID < facts.Nodes[j].ID })
	sort.Slice(facts.Edges, func(i, j int) bool { return facts.Edges[i].ID < facts.Edges[j].ID })
	for i := range facts.Nodes {
		facts.Nodes[i].Sources = sortedSources(facts.Nodes[i].Sources)
	}
	for i := range facts.Edges {
		facts.Edges[i].Sources = sortedSources(facts.Edges[i].Sources)
	}
	raw, _ := json.Marshal(facts)
	h := sha256.New()
	h.Write([]byte("AnalytixCanvasFactsV1\x00"))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

// FactsDigest is independent of input node/edge/reference order and all layout.
// Scalar numbers use JSON float64 semantics; exact decimals belong in strings.
func FactsDigest(s Scene) (string, error) {
	normalized, err := cloneScene(s)
	if err != nil {
		return "", err
	}
	return factDigest(normalized), nil
}
func validOperation(op Operation) bool {
	if !idPattern.MatchString(op.ID) {
		return false
	}
	switch op.Kind {
	case "set-node-layout":
		return op.Target == "" && op.Layout != nil && validLayout(*op.Layout) && op.Points == nil && op.DisplayLabel == nil && op.Style == nil
	case "set-edge-route":
		return op.Target == "" && op.Layout == nil && len(op.Points) >= 2 && validPoints(op.Points) && op.DisplayLabel == nil && op.Style == nil
	case "set-display-label":
		return (op.Target == "node" || op.Target == "edge") && op.Layout == nil && op.Points == nil && op.DisplayLabel != nil && boundedText(*op.DisplayLabel, 1024) && op.Style == nil
	case "set-style":
		return (op.Target == "node" || op.Target == "edge") && op.Layout == nil && op.Points == nil && op.DisplayLabel == nil && op.Style != nil && validStyle(*op.Style, op.Target)
	}
	return false
}
func ParseOperations(raw []byte) ([]Operation, error) {
	var ops []Operation
	if decode(raw, &ops, 1<<20) != nil || len(ops) < 1 || len(ops) > MaxOperations {
		return nil, ErrInvalid
	}
	// Reject irrelevant keys even when supplied with their zero value.
	var objects []map[string]json.RawMessage
	json.Unmarshal(raw, &objects)
	seen := map[string]bool{}
	for i, op := range ops {
		expected := 3
		if op.Kind == "set-display-label" || op.Kind == "set-style" {
			expected = 4
		}
		if len(objects[i]) != expected || !validOperation(op) {
			return nil, ErrInvalid
		}
		key := op.Kind + "/" + op.ID
		if seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
	}
	return ops, nil
}
func Apply(s Scene, operations []Operation) (Result, error) {
	if len(operations) < 1 || len(operations) > MaxOperations {
		return Result{}, ErrInvalid
	}
	candidate, err := cloneScene(s)
	if err != nil {
		return Result{}, err
	}
	for _, op := range operations {
		if op.DisplayLabel != nil && !boundedText(*op.DisplayLabel, 1024) {
			return Result{}, ErrInvalid
		}
	}
	raw, err := json.Marshal(operations)
	if err != nil {
		return Result{}, ErrInvalid
	}
	ops, err := ParseOperations(raw)
	if err != nil {
		return Result{}, err
	}
	beforeFacts := factDigest(candidate)
	diff := []Change{}
	for _, op := range ops {
		target := op.Target
		if target == "" {
			target = "node"
			if op.Kind == "set-edge-route" {
				target = "edge"
			}
		}
		var n *NodeView
		var e *EdgeView
		factLabel := ""
		if target == "node" {
			for i := range candidate.Presentation.Nodes {
				if candidate.Presentation.Nodes[i].ID == op.ID {
					n = &candidate.Presentation.Nodes[i]
				}
			}
			for _, v := range candidate.Facts.Nodes {
				if v.ID == op.ID {
					factLabel = v.Label
				}
			}
		} else {
			for i := range candidate.Presentation.Edges {
				if candidate.Presentation.Edges[i].ID == op.ID {
					e = &candidate.Presentation.Edges[i]
				}
			}
			for _, v := range candidate.Facts.Edges {
				if v.ID == op.ID {
					factLabel = v.Label
				}
			}
		}
		if n == nil && e == nil {
			return Result{}, ErrInvalid
		}
		var before, after any
		field := ""
		switch op.Kind {
		case "set-node-layout":
			before, after, field = n.Layout, *op.Layout, "layout"
			n.Layout = *op.Layout
		case "set-edge-route":
			before, after, field = e.Points, op.Points, "points"
			e.Points = append([]Point{}, op.Points...)
		case "set-display-label":
			field = "displayLabel"
			after = *op.DisplayLabel
			if n != nil {
				before = n.DisplayLabel
				n.DisplayLabel = *op.DisplayLabel
			} else {
				before = e.DisplayLabel
				e.DisplayLabel = *op.DisplayLabel
			}
		case "set-style":
			field = "style"
			after = *op.Style
			if n != nil {
				before = n.Style
				n.Style = *op.Style
			} else {
				before = e.Style
				e.Style = *op.Style
			}
		}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		if !bytes.Equal(b, a) {
			diff = append(diff, Change{op.Kind, target, op.ID, field, factLabel, b, a})
		}
	}
	candidate, err = cloneScene(candidate)
	if err != nil || factDigest(candidate) != beforeFacts {
		return Result{}, ErrInvalid
	}
	return Result{candidate, beforeFacts, diff}, nil
}

func privateBytesDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
