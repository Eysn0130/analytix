package officegeneration

import (
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"unicode/utf16"
	"unicode/utf8"
)

var ErrInvalidPresentation = errors.New("invalid presentation generation specification")
var presentationID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var presentationColor = regexp.MustCompile(`^[A-Fa-f0-9]{6}$`)

type Presentation struct {
	Slides []PresentationSlide `json:"slides"`
}
type PresentationSlide struct {
	ID         string               `json:"id"`
	Background string               `json:"background,omitempty"`
	Objects    []PresentationObject `json:"objects"`
}
type PresentationObject struct {
	ID         string               `json:"id"`
	Kind       string               `json:"kind"`
	X          float64              `json:"x"`
	Y          float64              `json:"y"`
	W          float64              `json:"w"`
	H          float64              `json:"h"`
	Text       *string              `json:"text,omitempty"`
	FontSize   *float64             `json:"fontSize,omitempty"`
	Bold       *bool                `json:"bold,omitempty"`
	Color      *string              `json:"color,omitempty"`
	Align      *string              `json:"align,omitempty"`
	Shape      *string              `json:"shape,omitempty"`
	Fill       *string              `json:"fill,omitempty"`
	LineColor  *string              `json:"lineColor,omitempty"`
	LineWidth  *float64             `json:"lineWidth,omitempty"`
	ChartType  *string              `json:"chartType,omitempty"`
	Title      *string              `json:"title,omitempty"`
	Categories []string             `json:"categories,omitempty"`
	Series     []PresentationSeries `json:"series,omitempty"`
	ImageID    *string              `json:"imageId,omitempty"`
}
type PresentationSeries struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

func ParsePresentation(body []byte) (Presentation, error) {
	raw, rawErr := domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{MaxBytes: 4 << 20, MaxDepth: 16, MaxTokens: 100000, MaxStringBytes: 1 << 20})
	if rawErr != nil || !presentationNoNull(raw) || !presentationKeys(raw, "slides") {
		return Presentation{}, ErrInvalidPresentation
	}
	slides, ok := raw["slides"].([]any)
	if !ok {
		return Presentation{}, ErrInvalidPresentation
	}
	for _, item := range slides {
		slide, ok := item.(map[string]any)
		if !ok || !presentationKeys(slide, "id", "background", "objects") {
			return Presentation{}, ErrInvalidPresentation
		}
		if color, present := slide["background"]; present {
			text, ok := color.(string)
			if !ok || !presentationColor.MatchString(text) {
				return Presentation{}, ErrInvalidPresentation
			}
		}
		objects, ok := slide["objects"].([]any)
		if !ok {
			return Presentation{}, ErrInvalidPresentation
		}
		for _, item := range objects {
			object, ok := item.(map[string]any)
			if !ok || !presentationKeys(object, "id", "kind", "x", "y", "w", "h", "text", "fontSize", "bold", "color", "align", "shape", "fill", "lineColor", "lineWidth", "chartType", "title", "categories", "series", "imageId") {
				return Presentation{}, ErrInvalidPresentation
			}
			for _, key := range []string{"id", "kind", "x", "y", "w", "h"} {
				if _, exists := object[key]; !exists {
					return Presentation{}, ErrInvalidPresentation
				}
			}
			if list, exists := object["series"]; exists {
				series, ok := list.([]any)
				if !ok {
					return Presentation{}, ErrInvalidPresentation
				}
				for _, item := range series {
					entry, ok := item.(map[string]any)
					if !ok || !presentationKeys(entry, "name", "values") {
						return Presentation{}, ErrInvalidPresentation
					}
					if _, ok := entry["name"]; !ok {
						return Presentation{}, ErrInvalidPresentation
					}
				}
			}
		}
	}
	var value Presentation
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if len(body) > 4<<20 || decoder.Decode(&value) != nil {
		return Presentation{}, ErrInvalidPresentation
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || value.Validate(nil, "") != nil {
		return Presentation{}, ErrInvalidPresentation
	}
	return value, nil
}

func presentationNoNull(value any) bool {
	if value == nil {
		return false
	}
	switch x := value.(type) {
	case map[string]any:
		for _, v := range x {
			if !presentationNoNull(v) {
				return false
			}
		}
	case []any:
		for _, v := range x {
			if !presentationNoNull(v) {
				return false
			}
		}
	}
	return true
}

// Validate applies the same finite object contract as the fixed Node codec.
// A nil image inventory validates the structure only; Core supplies the actual
// inventory before encoding so unresolved image references cannot be delivered.
func (p Presentation) Validate(images map[string]bool, title string) error {
	if len(p.Slides) < 1 || len(p.Slides) > 50 {
		return ErrInvalidPresentation
	}
	textBudget := 0
	addText := func(s string, limit int) bool {
		if !presentationXMLText(s) {
			return false
		}
		n := len(utf16.Encode([]rune(s)))
		textBudget += n
		return n <= limit && textBudget <= 100000
	}
	if !addText(title, 256) {
		return ErrInvalidPresentation
	}
	seen := map[string]bool{}
	objects := 0
	claim := func(id string) bool {
		if !presentationID.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	for _, slide := range p.Slides {
		if !claim(slide.ID) || len(slide.Objects) < 1 || len(slide.Objects) > 100 || (slide.Background != "" && !presentationColor.MatchString(slide.Background)) {
			return ErrInvalidPresentation
		}
		objects += len(slide.Objects)
		if objects > 1000 {
			return ErrInvalidPresentation
		}
		for _, o := range slide.Objects {
			if !claim(o.ID) || !presentationFinite(o.X) || !presentationFinite(o.Y) || !presentationFinite(o.W) || !presentationFinite(o.H) || o.X < 0 || o.Y < 0 || o.W < 0 || o.H < 0 || o.X+o.W > 13.333333 || o.Y+o.H > 7.5 {
				return ErrInvalidPresentation
			}
			line := o.Kind == "shape" && o.Shape != nil && *o.Shape == "line"
			if (line && o.W == 0 && o.H == 0) || (!line && (o.W == 0 || o.H == 0)) {
				return ErrInvalidPresentation
			}
			switch o.Kind {
			case "text":
				if o.Text == nil || !addText(*o.Text, 6000) || o.Shape != nil || o.Fill != nil || o.LineColor != nil || o.LineWidth != nil || o.ChartType != nil || o.Title != nil || o.Categories != nil || o.Series != nil || o.ImageID != nil {
					return ErrInvalidPresentation
				}
				if o.FontSize != nil && (!presentationFinite(*o.FontSize) || *o.FontSize < 10 || *o.FontSize > 60) {
					return ErrInvalidPresentation
				}
				if o.Color != nil && !presentationColor.MatchString(*o.Color) {
					return ErrInvalidPresentation
				}
				if o.Align != nil && *o.Align != "left" && *o.Align != "center" && *o.Align != "right" {
					return ErrInvalidPresentation
				}
			case "shape":
				if o.Shape == nil || (*o.Shape != "rect" && *o.Shape != "ellipse" && *o.Shape != "line") || o.Text != nil || o.FontSize != nil || o.Bold != nil || o.Color != nil || o.Align != nil || o.ChartType != nil || o.Title != nil || o.Categories != nil || o.Series != nil || o.ImageID != nil {
					return ErrInvalidPresentation
				}
				if (o.Fill != nil && !presentationColor.MatchString(*o.Fill)) || (o.LineColor != nil && !presentationColor.MatchString(*o.LineColor)) || (o.LineWidth != nil && (!presentationFinite(*o.LineWidth) || *o.LineWidth < 0 || *o.LineWidth > 20)) {
					return ErrInvalidPresentation
				}
			case "chart":
				if o.ChartType == nil || (*o.ChartType != "bar" && *o.ChartType != "line" && *o.ChartType != "pie") || o.Text != nil || o.FontSize != nil || o.Bold != nil || o.Color != nil || o.Align != nil || o.Shape != nil || o.Fill != nil || o.LineColor != nil || o.LineWidth != nil || o.ImageID != nil || len(o.Categories) < 1 || len(o.Categories) > 100 || len(o.Series) < 1 || len(o.Series) > 8 {
					return ErrInvalidPresentation
				}
				if o.Title != nil && !addText(*o.Title, 256) {
					return ErrInvalidPresentation
				}
				for _, category := range o.Categories {
					if !addText(category, 6000) {
						return ErrInvalidPresentation
					}
				}
				for _, series := range o.Series {
					if !addText(series.Name, 6000) || len(series.Values) != len(o.Categories) {
						return ErrInvalidPresentation
					}
					for _, number := range series.Values {
						if !presentationFinite(number) {
							return ErrInvalidPresentation
						}
					}
				}
			case "image":
				if o.ImageID == nil || !presentationID.MatchString(*o.ImageID) || (images != nil && !images[*o.ImageID]) || o.Text != nil || o.FontSize != nil || o.Bold != nil || o.Color != nil || o.Align != nil || o.Shape != nil || o.Fill != nil || o.LineColor != nil || o.LineWidth != nil || o.ChartType != nil || o.Title != nil || o.Categories != nil || o.Series != nil {
					return ErrInvalidPresentation
				}
			default:
				return ErrInvalidPresentation
			}
		}
	}
	return nil
}

func presentationFinite(n float64) bool { return !math.IsInf(n, 0) && !math.IsNaN(n) }
func presentationXMLText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0xfffe || r == 0xffff {
			return false
		}
	}
	return true
}

func presentationKeys(value map[string]any, allowed ...string) bool {
	for key := range value {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
