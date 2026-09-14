// Package documentgeneration owns typed document generation requests.
package documentgeneration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	officedomain "analytix.local/runtime-go/internal/domain/officegeneration"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

var ErrInvalidRequest = errors.New("invalid document generation request")
var imageID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type Request struct {
	Path         string            `json:"path"`
	Kind         string            `json:"kind"`
	Markdown     string            `json:"markdown,omitempty"`
	Title        string            `json:"title,omitempty"`
	Images       []codecport.Image `json:"images,omitempty"`
	Workbook     json.RawMessage   `json:"workbook,omitempty"`
	Presentation json.RawMessage   `json:"presentation,omitempty"`
}

func ParseRequest(args map[string]any) (Request, error) {
	allowed := map[string]bool{"path": true, "kind": true, "markdown": true, "title": true, "images": true, "workbook": true, "presentation": true}
	for key := range args {
		if !allowed[key] {
			return Request{}, ErrInvalidRequest
		}
	}
	for _, key := range []string{"title", "images", "workbook", "presentation"} {
		if value, present := args[key]; present && value == nil {
			return Request{}, ErrInvalidRequest
		}
	}
	body, err := json.Marshal(args)
	if err != nil || len(body) > 4<<20 {
		return Request{}, ErrInvalidRequest
	}
	// Match the execution-grant argument boundary before invoking the codec.
	// The data-only codec may accept larger private assets; model tool arguments
	// must remain within the existing canonical JSON authority budget.
	if _, err := domainsecurity.DecodeCanonicalJSONValue(body); err != nil {
		return Request{}, ErrInvalidRequest
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Path == "" || request.Path != strings.TrimSpace(request.Path) ||
		len(request.Path) > 4096 || strings.ContainsRune(request.Path, 0) || strings.ToLower(filepath.Ext(request.Path)) != "."+request.Kind ||
		!utf8.ValidString(request.Title) || len(utf16.Encode([]rune(request.Title))) > 256 || len(request.Images) > 24 {
		return Request{}, ErrInvalidRequest
	}
	switch request.Kind {
	case "docx":
		if !utf8.ValidString(request.Markdown) || len(request.Markdown) > 1<<20 || strings.TrimSpace(request.Markdown) == "" || request.Workbook != nil || request.Presentation != nil {
			return Request{}, ErrInvalidRequest
		}
	case "xlsx":
		if _, present := args["markdown"]; present {
			return Request{}, ErrInvalidRequest
		}
		if _, present := args["images"]; present {
			return Request{}, ErrInvalidRequest
		}
		if request.Presentation != nil {
			return Request{}, ErrInvalidRequest
		}
		if _, err := officedomain.ParseWorkbook(request.Workbook); err != nil {
			return Request{}, ErrInvalidRequest
		}
	case "pptx":
		if _, present := args["markdown"]; present {
			return Request{}, ErrInvalidRequest
		}
		if request.Workbook != nil {
			return Request{}, ErrInvalidRequest
		}
	default:
		return Request{}, ErrInvalidRequest
	}
	seen, total := map[string]bool{}, 0
	imageSizes := map[string]int{}
	for _, image := range request.Images {
		if !imageID.MatchString(image.ID) || seen[image.ID] || len(image.DataBase64) > base64.StdEncoding.EncodedLen(8<<20) {
			return Request{}, ErrInvalidRequest
		}
		switch image.Type {
		case "png", "jpg", "gif", "bmp":
		default:
			return Request{}, ErrInvalidRequest
		}
		data, err := base64.StdEncoding.Strict().DecodeString(image.DataBase64)
		if err != nil || len(data) == 0 || len(data) > 8<<20 || base64.StdEncoding.EncodeToString(data) != image.DataBase64 {
			return Request{}, ErrInvalidRequest
		}
		total += len(data)
		if total > 24<<20 {
			return Request{}, ErrInvalidRequest
		}
		seen[image.ID] = true
		imageSizes[image.ID] = len(data)
	}
	if request.Kind == "pptx" {
		presentation, err := officedomain.ParsePresentation(request.Presentation)
		if err != nil || presentation.Validate(seen, request.Title) != nil {
			return Request{}, ErrInvalidRequest
		}
		embeddedBytes := 0
		for _, slide := range presentation.Slides {
			for _, object := range slide.Objects {
				if object.ImageID != nil {
					embeddedBytes += imageSizes[*object.ImageID]
					if embeddedBytes > 24<<20 {
						return Request{}, ErrInvalidRequest
					}
				}
			}
		}
	}
	return request, nil
}

func (r Request) CodecInput() codecport.Input {
	return codecport.Input{SchemaVersion: 1, Kind: r.Kind, Markdown: r.Markdown, Title: r.Title, Images: r.Images, Workbook: r.Workbook, Presentation: r.Presentation}
}
