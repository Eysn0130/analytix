package piiauthorization

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ControlledPIIArtifactSchemaVersionV1 = 1
	ControlledPIIArtifactPurposeV1       = "analytix.controlled-pii-artifact/v1"
	ControlledPIIArtifactMediaTypeV1     = "application/vnd.analytix.controlled-case-evidence+json"
	MaxControlledPIIArtifactBytesV1      = 8 << 20
	maxControlledPIIValueBytesV1         = 16 << 10
)

// ControlledPIIArtifactV1 is a protected-delivery contract, never an ordinary
// chat/SSE/history/report projection. ExactValue is copied by the host from a
// verified claim only after current registry validation; no provider or caller
// supplied display value is accepted by the renderer.
type ControlledPIIArtifactV1 struct {
	SchemaVersion                 int                    `json:"schemaVersion"`
	Purpose                       string                 `json:"purpose"`
	MediaType                     string                 `json:"mediaType"`
	Context                       ContextBindingV1       `json:"context"`
	RequesterUserID               string                 `json:"requesterUserId"`
	DisclosurePurpose             string                 `json:"disclosurePurpose"`
	DeliveryScope                 string                 `json:"deliveryScope"`
	ClaimLedgerDigest             string                 `json:"claimLedgerDigest"`
	ProjectionRulesetHash         string                 `json:"projectionRulesetHash"`
	TargetIdentityDigest          string                 `json:"targetIdentityDigest"`
	PreservedControlledFieldCount uint64                 `json:"preservedControlledFieldCount"`
	Fields                        []ControlledPIIFieldV1 `json:"fields"`
	RenderedAt                    string                 `json:"renderedAt"`
}

type ControlledPIIFieldV1 struct {
	PIIClass           string                   `json:"piiClass"`
	ClaimID            string                   `json:"claimId"`
	ClaimRecordDigest  string                   `json:"claimRecordDigest"`
	ClaimType          domainevidence.ClaimType `json:"claimType"`
	FieldName          string                   `json:"fieldName"`
	ExactValue         string                   `json:"exactValue"`
	ValueSHA256        string                   `json:"valueSha256"`
	EvidenceReceiptIDs []string                 `json:"evidenceReceiptIds"`
}

type ControlledPIIArtifactInputV1 struct {
	SecurityContext       domainsecurity.TurnSecurityContext
	ClaimLedgerDigest     string
	ProjectionRulesetHash string
	TargetIdentityDigest  string
	Fields                []ControlledPIIFieldV1
	RenderedAt            time.Time
}

// ControlledPIIArtifactMetadataV1 is the raw-value-free projection used by
// startup trust inventory. ExactValue never crosses the outbound artifact
// adapter during recovery or audit.
type ControlledPIIArtifactMetadataV1 struct {
	SHA256                        string
	ByteLength                    uint64
	MediaType                     string
	Context                       ContextBindingV1
	RequesterUserID               string
	DisclosurePurpose             string
	DeliveryScope                 string
	ClaimLedgerDigest             string
	ProjectionRulesetHash         string
	TargetIdentityDigest          string
	PreservedControlledFieldCount uint64
	FieldBindings                 []FieldBindingV1
	FieldBindingSetDigest         string
	RenderedAt                    string
}

func NewControlledPIIArtifactV1(input ControlledPIIArtifactInputV1) (ControlledPIIArtifactV1, error) {
	contextBinding, err := contextBindingFromSecurityContextV1(input.SecurityContext)
	if err != nil {
		return ControlledPIIArtifactV1{}, err
	}
	fields, err := canonicalControlledPIIFieldsV1(input.Fields)
	if err != nil {
		return ControlledPIIArtifactV1{}, err
	}
	renderedAt := input.RenderedAt.UTC()
	artifact := ControlledPIIArtifactV1{
		SchemaVersion: ControlledPIIArtifactSchemaVersionV1, Purpose: ControlledPIIArtifactPurposeV1,
		MediaType: ControlledPIIArtifactMediaTypeV1, Context: contextBinding, RequesterUserID: input.SecurityContext.UserID,
		DisclosurePurpose: DisclosurePurposeCaseReportV1, DeliveryScope: DeliveryScopeControlledArtifactV1,
		ClaimLedgerDigest: strings.TrimSpace(input.ClaimLedgerDigest), ProjectionRulesetHash: strings.TrimSpace(input.ProjectionRulesetHash),
		TargetIdentityDigest: strings.TrimSpace(input.TargetIdentityDigest), Fields: fields,
		PreservedControlledFieldCount: uint64(len(fields)), RenderedAt: renderedAt.Format(time.RFC3339Nano),
	}
	if err := ValidateControlledPIIArtifactV1(artifact); err != nil {
		return ControlledPIIArtifactV1{}, err
	}
	return artifact, nil
}

func ValidateControlledPIIArtifactV1(artifact ControlledPIIArtifactV1) error {
	renderedAt, timeErr := time.Parse(time.RFC3339Nano, artifact.RenderedAt)
	contextIssuedAt, contextTimeErr := time.Parse(time.RFC3339Nano, artifact.Context.ContextIssuedAt)
	if artifact.SchemaVersion != ControlledPIIArtifactSchemaVersionV1 || artifact.Purpose != ControlledPIIArtifactPurposeV1 ||
		artifact.MediaType != ControlledPIIArtifactMediaTypeV1 || !validContextBindingV1(artifact.Context) ||
		artifact.RequesterUserID != artifact.Context.UserID || artifact.DisclosurePurpose != DisclosurePurposeCaseReportV1 ||
		artifact.DeliveryScope != DeliveryScopeControlledArtifactV1 || !domainsecurity.IsSHA256Hex(artifact.ClaimLedgerDigest) ||
		!domainsecurity.IsSHA256Hex(artifact.ProjectionRulesetHash) || !domainsecurity.IsSHA256Hex(artifact.TargetIdentityDigest) ||
		len(artifact.Fields) == 0 || len(artifact.Fields) > PIIProjectionGrantMaxFieldsV1 ||
		artifact.PreservedControlledFieldCount != uint64(len(artifact.Fields)) || timeErr != nil || contextTimeErr != nil ||
		renderedAt.IsZero() || renderedAt.Before(contextIssuedAt) ||
		renderedAt.UTC().Format(time.RFC3339Nano) != artifact.RenderedAt {
		return errors.New("controlled PII artifact is incomplete")
	}
	canonical, err := canonicalControlledPIIFieldsV1(artifact.Fields)
	if err != nil || !equalControlledPIIFieldsV1(artifact.Fields, canonical) {
		return errors.New("controlled PII artifact fields are not canonical exact bindings")
	}
	return nil
}

func ValidateControlledPIIArtifactForBindingsV1(
	artifact ControlledPIIArtifactV1,
	securityContext domainsecurity.TurnSecurityContext,
	claimLedgerDigest string,
	projectionRulesetHash string,
	targetIdentityDigest string,
) error {
	if ValidateControlledPIIArtifactV1(artifact) != nil {
		return errors.New("controlled PII artifact is invalid")
	}
	expectedContext, err := contextBindingFromSecurityContextV1(securityContext)
	if err != nil || artifact.Context != expectedContext || artifact.RequesterUserID != securityContext.UserID ||
		artifact.ClaimLedgerDigest != strings.TrimSpace(claimLedgerDigest) ||
		artifact.ProjectionRulesetHash != strings.TrimSpace(projectionRulesetHash) ||
		artifact.TargetIdentityDigest != strings.TrimSpace(targetIdentityDigest) {
		return errors.New("controlled PII artifact binding is invalid")
	}
	return nil
}

func ControlledPIIArtifactV1Bytes(artifact ControlledPIIArtifactV1) ([]byte, error) {
	if err := ValidateControlledPIIArtifactV1(artifact); err != nil {
		return nil, err
	}
	body, err := json.Marshal(artifact)
	if err != nil || len(body) > MaxControlledPIIArtifactBytesV1 {
		return nil, errors.New("controlled PII artifact exceeds its protected contract")
	}
	return body, nil
}

func ParseControlledPIIArtifactV1(body []byte) (ControlledPIIArtifactV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxControlledPIIArtifactBytesV1, MaxDepth: 10, MaxTokens: 200_000, MaxStringBytes: maxControlledPIIValueBytesV1,
	}); err != nil {
		return ControlledPIIArtifactV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var artifact ControlledPIIArtifactV1
	if err := decoder.Decode(&artifact); err != nil {
		return ControlledPIIArtifactV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ControlledPIIArtifactV1{}, errors.New("controlled PII artifact contains trailing JSON")
	}
	canonical, err := json.Marshal(artifact)
	if err != nil || !bytes.Equal(body, canonical) {
		return ControlledPIIArtifactV1{}, errors.New("controlled PII artifact is not canonical JSON")
	}
	return artifact, ValidateControlledPIIArtifactV1(artifact)
}

func ControlledPIIArtifactSHA256V1(artifact ControlledPIIArtifactV1) (string, error) {
	body, err := ControlledPIIArtifactV1Bytes(artifact)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(body), nil
}

func ControlledPIIArtifactFieldBindingsV1(artifact ControlledPIIArtifactV1) ([]FieldBindingV1, error) {
	if err := ValidateControlledPIIArtifactV1(artifact); err != nil {
		return nil, err
	}
	bindings := make([]FieldBindingV1, 0, len(artifact.Fields))
	for _, field := range artifact.Fields {
		bindings = append(bindings, FieldBindingV1{
			PIIClass: field.PIIClass, ClaimID: field.ClaimID, ClaimRecordDigest: field.ClaimRecordDigest,
			ClaimType: field.ClaimType, FieldName: field.FieldName, ValueSHA256: field.ValueSHA256,
			EvidenceReceiptIDs: append([]string(nil), field.EvidenceReceiptIDs...),
		})
	}
	return bindings, nil
}

func ControlledPIIArtifactMetadataFromBytesV1(body []byte) (ControlledPIIArtifactMetadataV1, error) {
	artifact, err := ParseControlledPIIArtifactV1(body)
	if err != nil {
		return ControlledPIIArtifactMetadataV1{}, err
	}
	bindings, err := ControlledPIIArtifactFieldBindingsV1(artifact)
	if err != nil {
		return ControlledPIIArtifactMetadataV1{}, err
	}
	return ControlledPIIArtifactMetadataV1{
		SHA256:                        domainsecurity.SHA256Hex(body),
		ByteLength:                    uint64(len(body)),
		MediaType:                     artifact.MediaType,
		Context:                       artifact.Context,
		RequesterUserID:               artifact.RequesterUserID,
		DisclosurePurpose:             artifact.DisclosurePurpose,
		DeliveryScope:                 artifact.DeliveryScope,
		ClaimLedgerDigest:             artifact.ClaimLedgerDigest,
		ProjectionRulesetHash:         artifact.ProjectionRulesetHash,
		TargetIdentityDigest:          artifact.TargetIdentityDigest,
		PreservedControlledFieldCount: artifact.PreservedControlledFieldCount,
		FieldBindings:                 bindings,
		FieldBindingSetDigest:         fieldBindingSetDigestV1(bindings),
		RenderedAt:                    artifact.RenderedAt,
	}, nil
}

// ValidateControlledPIIArtifactMetadataForBindingsV1 proves that a raw-value-
// free metadata projection came from the exact canonical controlled artifact
// authorized for this report. A caller-provided projection class or nonempty
// authorization digest is never sufficient to cross the controlled branch.
func ValidateControlledPIIArtifactMetadataForBindingsV1(
	metadata ControlledPIIArtifactMetadataV1,
	securityContext domainsecurity.TurnSecurityContext,
	claimLedgerDigest string,
	projectionRulesetHash string,
	targetIdentityDigest string,
	artifactSHA256 string,
	artifactByteLength uint64,
	preservedControlledFieldCount uint64,
) error {
	expectedContext, err := contextBindingFromSecurityContextV1(securityContext)
	canonicalBindings, bindingsErr := canonicalFieldBindingsV1(metadata.FieldBindings)
	renderedAt, renderedErr := time.Parse(time.RFC3339Nano, metadata.RenderedAt)
	contextIssuedAt, contextTimeErr := time.Parse(time.RFC3339Nano, expectedContext.ContextIssuedAt)
	if err != nil || bindingsErr != nil || metadata.Context != expectedContext ||
		metadata.RequesterUserID != securityContext.UserID ||
		metadata.MediaType != ControlledPIIArtifactMediaTypeV1 ||
		metadata.DisclosurePurpose != DisclosurePurposeCaseReportV1 ||
		metadata.DeliveryScope != DeliveryScopeControlledArtifactV1 ||
		metadata.ClaimLedgerDigest != strings.TrimSpace(claimLedgerDigest) ||
		metadata.ProjectionRulesetHash != strings.TrimSpace(projectionRulesetHash) ||
		metadata.TargetIdentityDigest != strings.TrimSpace(targetIdentityDigest) ||
		metadata.SHA256 != strings.TrimSpace(artifactSHA256) ||
		metadata.ByteLength == 0 || metadata.ByteLength > MaxControlledPIIArtifactBytesV1 ||
		metadata.ByteLength != artifactByteLength ||
		metadata.PreservedControlledFieldCount == 0 ||
		metadata.PreservedControlledFieldCount != preservedControlledFieldCount ||
		metadata.PreservedControlledFieldCount != uint64(len(metadata.FieldBindings)) ||
		!equalFieldBindingsV1(metadata.FieldBindings, canonicalBindings) ||
		metadata.FieldBindingSetDigest != fieldBindingSetDigestV1(canonicalBindings) ||
		renderedErr != nil || contextTimeErr != nil || renderedAt.IsZero() || renderedAt.Before(contextIssuedAt) ||
		renderedAt.UTC().Format(time.RFC3339Nano) != metadata.RenderedAt {
		return errors.New("controlled PII artifact metadata binding is invalid")
	}
	return nil
}

func validControlledPIIExactValueV1(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > maxControlledPIIValueBytesV1 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == '\u0000' || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func canonicalControlledPIIFieldsV1(values []ControlledPIIFieldV1) ([]ControlledPIIFieldV1, error) {
	if values == nil {
		return nil, errors.New("controlled PII artifact fields are required")
	}
	out := make([]ControlledPIIFieldV1, len(values))
	for index, value := range values {
		value.PIIClass = strings.TrimSpace(value.PIIClass)
		value.ClaimID = strings.TrimSpace(value.ClaimID)
		value.ClaimRecordDigest = strings.TrimSpace(value.ClaimRecordDigest)
		value.FieldName = strings.TrimSpace(value.FieldName)
		value.ValueSHA256 = strings.TrimSpace(value.ValueSHA256)
		value.EvidenceReceiptIDs = canonicalEvidenceIDListV1(value.EvidenceReceiptIDs)
		binding := FieldBindingV1{
			PIIClass: value.PIIClass, ClaimID: value.ClaimID, ClaimRecordDigest: value.ClaimRecordDigest,
			ClaimType: value.ClaimType, FieldName: value.FieldName, ValueSHA256: value.ValueSHA256,
			EvidenceReceiptIDs: append([]string(nil), value.EvidenceReceiptIDs...),
		}
		if !validFieldBindingV1(binding) || !validControlledPIIExactValueV1(value.ExactValue) ||
			domainsecurity.SHA256Hex([]byte(value.ExactValue)) != value.ValueSHA256 {
			return nil, errors.New("controlled PII artifact exact field is invalid")
		}
		out[index] = value
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.PIIClass != right.PIIClass {
			return left.PIIClass < right.PIIClass
		}
		if left.ClaimID != right.ClaimID {
			return left.ClaimID < right.ClaimID
		}
		if left.FieldName != right.FieldName {
			return left.FieldName < right.FieldName
		}
		return left.ValueSHA256 < right.ValueSHA256
	})
	for index := 1; index < len(out); index++ {
		left, right := out[index-1], out[index]
		if left.PIIClass == right.PIIClass && left.ClaimID == right.ClaimID &&
			left.FieldName == right.FieldName && left.ValueSHA256 == right.ValueSHA256 {
			return nil, errors.New("controlled PII artifact field is duplicated")
		}
	}
	return out, nil
}

func equalControlledPIIFieldsV1(left, right []ControlledPIIFieldV1) bool {
	leftBody, _ := json.Marshal(left)
	rightBody, _ := json.Marshal(right)
	return bytes.Equal(leftBody, rightBody)
}
