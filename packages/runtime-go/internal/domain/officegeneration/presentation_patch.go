package officegeneration

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const PresentationMaxDimension = 1000000

var ErrInvalidPresentationPatch = errors.New("presentation_patch_invalid")
var presentationRGB = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// PresentationSelection is the complete ordered page captured by the native
// owner and verified against Core's original. Indices are not durable IDs.
type PresentationSelection struct {
	PageIndex         int                 `json:"pageIndex"`
	TargetShapeIndex  int                 `json:"targetShapeIndex"`
	PageWidth100thMM  int                 `json:"pageWidth100thMm"`
	PageHeight100thMM int                 `json:"pageHeight100thMm"`
	Shapes            []PresentationShape `json:"shapes"`
}

type PresentationShape struct {
	ShapeIndex    int    `json:"shapeIndex"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Text          string `json:"text"`
	X100thMM      int    `json:"x100thMm"`
	Y100thMM      int    `json:"y100thMm"`
	Width100thMM  int    `json:"width100thMm"`
	Height100thMM int    `json:"height100thMm"`
	FillRGB       string `json:"fillRGB"`
}

type PresentationPatch struct {
	Kind          string `json:"kind"`
	X100thMM      *int   `json:"x100thMm,omitempty"`
	Y100thMM      *int   `json:"y100thMm,omitempty"`
	Width100thMM  *int   `json:"width100thMm,omitempty"`
	Height100thMM *int   `json:"height100thMm,omitempty"`
	RGB           string `json:"rgb,omitempty"`
}

type PresentationReview struct {
	Before PresentationSelection `json:"before"`
	After  PresentationSelection `json:"after"`
}

func presentationDecode(body []byte, target any) (map[string]any, error) {
	v, err := jsonstrict.DecodeObject(body, jsonstrict.Options{MaxBytes: 512 << 10, MaxDepth: 6, MaxTokens: 16384, MaxStringBytes: 65536, MaxNumberBytes: 32, MaxAbsExponent: 9})
	if err != nil {
		return nil, ErrInvalidPresentationPatch
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return nil, ErrInvalidPresentationPatch
	}
	return v, nil
}

func ParsePresentationSelection(body []byte) (PresentationSelection, error) {
	var s PresentationSelection
	v, err := presentationDecode(body, &s)
	if err != nil || !workbookPatchKeys(v, "pageIndex targetShapeIndex pageWidth100thMm pageHeight100thMm shapes", "") {
		return s, ErrInvalidPresentationPatch
	}
	shapes, ok := v["shapes"].([]any)
	if !ok {
		return s, ErrInvalidPresentationPatch
	}
	for _, shape := range shapes {
		m, ok := shape.(map[string]any)
		if !ok || !workbookPatchKeys(m, "shapeIndex kind name text x100thMm y100thMm width100thMm height100thMm fillRGB", "") {
			return s, ErrInvalidPresentationPatch
		}
	}
	return s, s.Validate()
}

func ParsePresentationPatch(body []byte) (PresentationPatch, error) {
	var p PresentationPatch
	v, err := presentationDecode(body, &p)
	if err != nil {
		return p, err
	}
	fields := "kind rgb"
	if p.Kind == "shape-geometry" {
		fields = "kind x100thMm y100thMm width100thMm height100thMm"
	}
	if !workbookPatchKeys(v, fields, "") || p.Validate() != nil {
		return p, ErrInvalidPresentationPatch
	}
	return p, nil
}

func (s PresentationSelection) Validate() error {
	if s.PageIndex < 0 || s.PageIndex >= 512 || s.TargetShapeIndex < 0 || s.TargetShapeIndex >= len(s.Shapes) || len(s.Shapes) == 0 || len(s.Shapes) > 64 || s.PageWidth100thMM <= 0 || s.PageWidth100thMM > PresentationMaxDimension || s.PageHeight100thMM <= 0 || s.PageHeight100thMM > PresentationMaxDimension {
		return ErrInvalidPresentationPatch
	}
	total := 0
	for i, shape := range s.Shapes {
		if shape.ShapeIndex != i || (shape.Kind != "rectangle" && shape.Kind != "ellipse" && shape.Kind != "text") || !presentationRGB.MatchString(shape.FillRGB) || !utf8.ValidString(shape.Name+shape.Text) || len(shape.Name) > 4096 || len(shape.Text) > 4096 || strings.ContainsRune(shape.Name+shape.Text, 0) || shape.X100thMM < 0 || shape.Y100thMM < 0 || shape.Width100thMM <= 0 || shape.Height100thMM <= 0 || shape.Width100thMM > s.PageWidth100thMM || shape.Height100thMM > s.PageHeight100thMM || shape.X100thMM > s.PageWidth100thMM-shape.Width100thMM || shape.Y100thMM > s.PageHeight100thMM-shape.Height100thMM {
			return ErrInvalidPresentationPatch
		}
		total += len(shape.Name) + len(shape.Text)
	}
	if total > 65536 {
		return ErrInvalidPresentationPatch
	}
	return nil
}

func (p PresentationPatch) Validate() error {
	if p.Kind == "shape-fill" && presentationRGB.MatchString(p.RGB) && p.X100thMM == nil && p.Y100thMM == nil && p.Width100thMM == nil && p.Height100thMM == nil {
		return nil
	}
	if p.Kind != "shape-geometry" || p.RGB != "" || p.X100thMM == nil || p.Y100thMM == nil || p.Width100thMM == nil || p.Height100thMM == nil {
		return ErrInvalidPresentationPatch
	}
	for _, v := range []*int{p.X100thMM, p.Y100thMM, p.Width100thMM, p.Height100thMM} {
		if *v < 0 || *v > PresentationMaxDimension {
			return ErrInvalidPresentationPatch
		}
	}
	if *p.Width100thMM == 0 || *p.Height100thMM == 0 {
		return ErrInvalidPresentationPatch
	}
	return nil
}

func ApplyPresentationPatch(s PresentationSelection, p PresentationPatch) (PresentationReview, error) {
	r := PresentationReview{Before: s, After: s}
	if s.Validate() != nil || p.Validate() != nil {
		return r, ErrInvalidPresentationPatch
	}
	r.After.Shapes = append([]PresentationShape(nil), s.Shapes...)
	shape := &r.After.Shapes[s.TargetShapeIndex]
	if p.Kind == "shape-fill" {
		shape.FillRGB = p.RGB
	} else {
		shape.X100thMM, shape.Y100thMM = *p.X100thMM, *p.Y100thMM
		shape.Width100thMM, shape.Height100thMM = *p.Width100thMM, *p.Height100thMM
	}
	if r.After.Validate() != nil || reflect.DeepEqual(r.Before, r.After) {
		return r, ErrInvalidPresentationPatch
	}
	return r, nil
}

func ValidatePresentationReview(r PresentationReview) error {
	if r.Before.Validate() != nil || r.After.Validate() != nil {
		return ErrInvalidPresentationPatch
	}
	a, b := r.Before.Shapes[r.Before.TargetShapeIndex], r.After.Shapes[r.After.TargetShapeIndex]
	p := PresentationPatch{Kind: "shape-fill", RGB: b.FillRGB}
	if a.FillRGB == b.FillRGB {
		p = PresentationPatch{Kind: "shape-geometry", X100thMM: &b.X100thMM, Y100thMM: &b.Y100thMM, Width100thMM: &b.Width100thMM, Height100thMM: &b.Height100thMM}
	}
	computed, err := ApplyPresentationPatch(r.Before, p)
	if err != nil || !reflect.DeepEqual(computed, r) {
		return ErrInvalidPresentationPatch
	}
	return nil
}
