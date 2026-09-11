package reportpublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	RenderInspectionSchemaVersion = 1
	RenderInspectionPurpose       = "analytix.render-inspection/v1"
)

type RenderInspectionV1 struct {
	SchemaVersion    int      `json:"schemaVersion"`
	Purpose          string   `json:"purpose"`
	Renderer         string   `json:"renderer"`
	RendererVersion  string   `json:"rendererVersion"`
	ReportSHA256     string   `json:"reportSha256"`
	ReportByteLength uint64   `json:"reportByteLength"`
	MediaType        string   `json:"mediaType"`
	Passed           bool     `json:"passed"`
	IssueCodes       []string `json:"issueCodes"`
	InspectedAt      string   `json:"inspectedAt"`
	InspectionDigest string   `json:"inspectionDigest"`
}

type RenderInspectionInputV1 struct {
	Renderer         string
	RendererVersion  string
	ReportSHA256     string
	ReportByteLength uint64
	MediaType        string
	Passed           bool
	IssueCodes       []string
	InspectedAt      time.Time
}

func NewRenderInspectionV1(input RenderInspectionInputV1) (RenderInspectionV1, error) {
	inspectedAt := input.InspectedAt.UTC()
	if inspectedAt.IsZero() {
		inspectedAt = time.Now().UTC()
	}
	inspection := RenderInspectionV1{
		SchemaVersion: RenderInspectionSchemaVersion, Purpose: RenderInspectionPurpose,
		Renderer: strings.TrimSpace(input.Renderer), RendererVersion: strings.TrimSpace(input.RendererVersion),
		ReportSHA256: strings.TrimSpace(input.ReportSHA256), ReportByteLength: input.ReportByteLength,
		MediaType: strings.TrimSpace(input.MediaType), Passed: input.Passed, IssueCodes: canonicalIssueCodes(input.IssueCodes),
		InspectedAt: inspectedAt.Format(time.RFC3339Nano),
	}
	if inspection.IssueCodes == nil {
		inspection.IssueCodes = []string{}
	}
	inspection.InspectionDigest = renderInspectionDigestV1(inspection)
	if err := ValidateRenderInspectionV1(inspection); err != nil {
		return RenderInspectionV1{}, err
	}
	return inspection, nil
}

func ValidateRenderInspectionV1(inspection RenderInspectionV1) error {
	inspectedAt, timeErr := time.Parse(time.RFC3339Nano, inspection.InspectedAt)
	if inspection.SchemaVersion != RenderInspectionSchemaVersion || inspection.Purpose != RenderInspectionPurpose ||
		strings.TrimSpace(inspection.Renderer) == "" || strings.TrimSpace(inspection.RendererVersion) == "" ||
		!domainsecurity.IsSHA256Hex(inspection.ReportSHA256) || inspection.ReportByteLength == 0 || strings.TrimSpace(inspection.MediaType) == "" ||
		inspection.IssueCodes == nil || timeErr != nil || inspectedAt.IsZero() || inspectedAt.UTC().Format(time.RFC3339Nano) != inspection.InspectedAt ||
		!domainsecurity.IsSHA256Hex(inspection.InspectionDigest) {
		return errors.New("render inspection is incomplete")
	}
	if !canonicalTextSlice(inspection.IssueCodes) || inspection.Passed != (len(inspection.IssueCodes) == 0) {
		return errors.New("render inspection result is inconsistent")
	}
	if inspection.InspectionDigest != renderInspectionDigestV1(inspection) {
		return errors.New("render inspection integrity is invalid")
	}
	return nil
}

func ParseRenderInspectionV1(body []byte) (RenderInspectionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 128 << 10, MaxDepth: 4, MaxTokens: 256, MaxStringBytes: 32 << 10,
	}); err != nil {
		return RenderInspectionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var inspection RenderInspectionV1
	if err := decoder.Decode(&inspection); err != nil {
		return RenderInspectionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return RenderInspectionV1{}, errors.New("render inspection contains trailing JSON")
	}
	canonical, err := json.Marshal(inspection)
	if err != nil || !bytes.Equal(body, canonical) {
		return RenderInspectionV1{}, errors.New("render inspection is not canonically encoded")
	}
	return inspection, ValidateRenderInspectionV1(inspection)
}

func RenderInspectionV1Bytes(inspection RenderInspectionV1) ([]byte, error) {
	if err := ValidateRenderInspectionV1(inspection); err != nil {
		return nil, err
	}
	return json.Marshal(inspection)
}

func canonicalIssueCodes(values []string) []string {
	if values == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func canonicalTextSlice(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || index > 0 && value <= values[index-1] {
			return false
		}
	}
	return true
}

func renderInspectionDigestV1(inspection RenderInspectionV1) string {
	inspection.InspectionDigest = ""
	body, _ := json.Marshal(inspection)
	return domainsecurity.SHA256Hex(append([]byte("analytix.render-inspection/digest/v1\x00"), body...))
}
