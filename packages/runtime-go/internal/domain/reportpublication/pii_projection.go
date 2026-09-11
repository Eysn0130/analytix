package reportpublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PIIProjectionSchemaVersion = 1
	PIIProjectionPurpose       = "analytix.pii-projection/v1"

	PIIProjectionOrdinaryMasked = "ordinary_masked"
	PIIProjectionControlledFull = "controlled_full"
)

// PIIProjectionV1 contains only counts and digests. Raw account, card,
// identity, phone, or device values must never enter this authority record.
type PIIProjectionV1 struct {
	SchemaVersion                 int    `json:"schemaVersion"`
	Purpose                       string `json:"purpose"`
	ProjectionClass               string `json:"projectionClass"`
	RulesetHash                   string `json:"rulesetHash"`
	ProjectedContentSHA256        string `json:"projectedContentSha256"`
	RestrictedFieldCount          uint64 `json:"restrictedFieldCount"`
	PreservedControlledFieldCount uint64 `json:"preservedControlledFieldCount"`
	AuthorizationAuditDigest      string `json:"authorizationAuditDigest"`
	ProjectionDigest              string `json:"projectionDigest"`
}

type PIIProjectionInputV1 struct {
	ProjectionClass               string
	RulesetHash                   string
	ProjectedContentSHA256        string
	RestrictedFieldCount          uint64
	PreservedControlledFieldCount uint64
	AuthorizationAuditDigest      string
}

func NewPIIProjectionV1(input PIIProjectionInputV1) (PIIProjectionV1, error) {
	projection := PIIProjectionV1{
		SchemaVersion: PIIProjectionSchemaVersion, Purpose: PIIProjectionPurpose,
		ProjectionClass: strings.TrimSpace(input.ProjectionClass), RulesetHash: strings.TrimSpace(input.RulesetHash),
		ProjectedContentSHA256: strings.TrimSpace(input.ProjectedContentSHA256), RestrictedFieldCount: input.RestrictedFieldCount,
		PreservedControlledFieldCount: input.PreservedControlledFieldCount,
		AuthorizationAuditDigest:      strings.TrimSpace(input.AuthorizationAuditDigest),
	}
	projection.ProjectionDigest = piiProjectionDigestV1(projection)
	if err := ValidatePIIProjectionV1(projection); err != nil {
		return PIIProjectionV1{}, err
	}
	return projection, nil
}

func ValidatePIIProjectionV1(projection PIIProjectionV1) error {
	if projection.SchemaVersion != PIIProjectionSchemaVersion || projection.Purpose != PIIProjectionPurpose ||
		!domainsecurity.IsSHA256Hex(projection.RulesetHash) || !domainsecurity.IsSHA256Hex(projection.ProjectedContentSHA256) ||
		!domainsecurity.IsSHA256Hex(projection.ProjectionDigest) {
		return errors.New("PII projection is incomplete")
	}
	switch projection.ProjectionClass {
	case PIIProjectionOrdinaryMasked:
		if projection.AuthorizationAuditDigest != "" || projection.PreservedControlledFieldCount != 0 {
			return errors.New("ordinary PII projection cannot preserve controlled values")
		}
	case PIIProjectionControlledFull:
		if !domainsecurity.IsSHA256Hex(projection.AuthorizationAuditDigest) || projection.PreservedControlledFieldCount == 0 {
			return errors.New("controlled PII projection lacks current authorization authority")
		}
	default:
		return errors.New("PII projection class is invalid")
	}
	if projection.ProjectionDigest != piiProjectionDigestV1(projection) {
		return errors.New("PII projection integrity is invalid")
	}
	return nil
}

func ParsePIIProjectionV1(body []byte) (PIIProjectionV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 << 10, MaxDepth: 4, MaxTokens: 64, MaxStringBytes: 32 << 10,
	}); err != nil {
		return PIIProjectionV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var projection PIIProjectionV1
	if err := decoder.Decode(&projection); err != nil {
		return PIIProjectionV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PIIProjectionV1{}, errors.New("PII projection contains trailing JSON")
	}
	canonical, err := json.Marshal(projection)
	if err != nil || !bytes.Equal(body, canonical) {
		return PIIProjectionV1{}, errors.New("PII projection is not canonically encoded")
	}
	return projection, ValidatePIIProjectionV1(projection)
}

func PIIProjectionV1Bytes(projection PIIProjectionV1) ([]byte, error) {
	if err := ValidatePIIProjectionV1(projection); err != nil {
		return nil, err
	}
	return json.Marshal(projection)
}

func piiProjectionDigestV1(projection PIIProjectionV1) string {
	projection.ProjectionDigest = ""
	body, _ := json.Marshal(projection)
	return domainsecurity.SHA256Hex(append([]byte("analytix.pii-projection/digest/v1\x00"), body...))
}
