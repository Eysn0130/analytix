package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	CanonicalEvidenceVersionV2              = 2
	CanonicalEvidencePurposeV2              = "analytix.canonical-evidence/v2"
	SourceFieldBindingVersionV2             = 2
	SourceFieldBindingPurposeV2             = "analytix.source-field-binding/v2"
	SourceFieldBindingCanonicalizerV2       = "analytix.bank-account-text/v2"
	SourceFieldBindingCanonicalFieldAccount = "accountId"
	SourceFieldBindingScalarTextV2          = "text"
)

const (
	sourceFieldBindingDigestDomainV2 = "analytix.source-field-binding/record/v2\x00"
	sourceFieldBindingSetDomainV2    = "analytix.source-field-binding-set/v2\x00"
	maxSourceExactAccountBytesV2     = 256
	maxSourceFieldIdentifierBytesV2  = 4 * 1024
	maxSourceFieldPathBytesV2        = 4 * 1024
)

// SourceFieldBindingV2 is private evidence-registry material. SourceExactValue
// must never be projected through ordinary events, logs, history, reports, or
// model-visible tool results.
type SourceFieldBindingV2 struct {
	SchemaVersion          int       `json:"schemaVersion"`
	Purpose                string    `json:"purpose"`
	FactID                 string    `json:"factId"`
	ClaimType              ClaimType `json:"claimType"`
	CanonicalFieldName     string    `json:"canonicalFieldName"`
	CanonicalEntityID      string    `json:"canonicalEntityId"`
	CanonicalAccountID     string    `json:"canonicalAccountId"`
	SourceRecordID         string    `json:"sourceRecordId"`
	RawArtifactSHA256      string    `json:"rawArtifactSha256"`
	SourceRecordSHA256     string    `json:"sourceRecordSha256"`
	SourceRecordPath       string    `json:"sourceRecordPath"`
	SourceRecordIDPath     string    `json:"sourceRecordIdPath"`
	SourceEntityIDPath     string    `json:"sourceEntityIdPath"`
	SourceFieldPath        string    `json:"sourceFieldPath"`
	SourceScalarKind       string    `json:"sourceScalarKind"`
	SourceExactValue       string    `json:"sourceExactValue"`
	SourceExactValueSHA256 string    `json:"sourceExactValueSha256"`
	Canonicalizer          string    `json:"canonicalizer"`
	BindingDigest          string    `json:"bindingDigest"`
}

// ValidateSourceFieldBindingsAgainstRawResultV2 proves that each private
// source-exact value belongs to the declared source record in the exact MCP
// tools/call result envelope. The raw artifact, record object, record ID, and
// field are independently bound so a pointer into row B cannot borrow row A's
// identifier.
func ValidateSourceFieldBindingsAgainstRawResultV2(
	rawResult json.RawMessage,
	rawSHA256 string,
	material CanonicalEvidenceMaterial,
) error {
	if material.SchemaVersion != CanonicalEvidenceVersionV2 || validateCanonicalEvidenceMaterialV2(material) != nil ||
		domainsecurity.SHA256Hex(rawResult) != strings.TrimSpace(rawSHA256) {
		return errors.New("source field raw result authority is invalid")
	}
	root, err := domainjsonstrict.DecodeValue(rawResult, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil {
		return errors.New("source field raw result is not strict JSON")
	}
	for _, binding := range material.SourceFieldBindings {
		if binding.RawArtifactSHA256 != rawSHA256 {
			return errors.New("source field binding is not bound to the exact raw artifact")
		}
		record, recordOK := resolveSourceJSONPointerV2(root, binding.SourceRecordPath)
		recordObject, objectOK := record.(map[string]any)
		recordBody, recordMarshalErr := json.Marshal(recordObject)
		if !recordOK || !objectOK || recordMarshalErr != nil ||
			domainsecurity.SHA256Hex(recordBody) != binding.SourceRecordSHA256 {
			return errors.New("source field binding record hash does not match the raw artifact")
		}
		recordID, recordIDOK := resolveSourceJSONPointerV2(root, binding.SourceRecordIDPath)
		exactRecordID, recordIDText := recordID.(string)
		if !recordIDOK || !recordIDText || exactRecordID != binding.SourceRecordID {
			return errors.New("source field binding record ID does not match the raw artifact")
		}
		entityID, entityIDOK := resolveSourceJSONPointerV2(root, binding.SourceEntityIDPath)
		exactEntityID, entityIDText := entityID.(string)
		if !entityIDOK || !entityIDText || exactEntityID != binding.CanonicalEntityID {
			return errors.New("source field binding entity ID does not match the raw artifact")
		}
		value, ok := resolveSourceJSONPointerV2(root, binding.SourceFieldPath)
		exact, text := value.(string)
		if !ok || !text || !bytes.Equal([]byte(exact), []byte(binding.SourceExactValue)) {
			return errors.New("source field exact value does not match the raw result")
		}
	}
	return nil
}

type SourceFieldBindingInputV2 struct {
	FactID             string
	ClaimType          ClaimType
	CanonicalEntityID  string
	CanonicalAccountID string
	SourceRecordID     string
	RawArtifactSHA256  string
	SourceRecordSHA256 string
	SourceRecordPath   string
	SourceRecordIDPath string
	SourceEntityIDPath string
	SourceFieldPath    string
	SourceScalarKind   string
	SourceExactValue   string
}

func NewSourceFieldBindingV2(input SourceFieldBindingInputV2) (SourceFieldBindingV2, error) {
	canonicalAccountID, err := canonicalBankAccountTextV2(input.SourceExactValue)
	if err != nil || canonicalAccountID != strings.TrimSpace(input.CanonicalAccountID) {
		return SourceFieldBindingV2{}, errors.New("source field binding account value is invalid")
	}
	binding := SourceFieldBindingV2{
		SchemaVersion: SourceFieldBindingVersionV2, Purpose: SourceFieldBindingPurposeV2,
		FactID: strings.TrimSpace(input.FactID), ClaimType: input.ClaimType,
		CanonicalFieldName: SourceFieldBindingCanonicalFieldAccount,
		CanonicalEntityID:  strings.TrimSpace(input.CanonicalEntityID), CanonicalAccountID: canonicalAccountID,
		SourceRecordID: strings.TrimSpace(input.SourceRecordID), RawArtifactSHA256: strings.TrimSpace(input.RawArtifactSHA256),
		SourceRecordSHA256: strings.TrimSpace(input.SourceRecordSHA256), SourceRecordPath: strings.TrimSpace(input.SourceRecordPath),
		SourceRecordIDPath: strings.TrimSpace(input.SourceRecordIDPath),
		SourceEntityIDPath: strings.TrimSpace(input.SourceEntityIDPath),
		SourceFieldPath:    strings.TrimSpace(input.SourceFieldPath), SourceScalarKind: strings.TrimSpace(input.SourceScalarKind),
		SourceExactValue: input.SourceExactValue, SourceExactValueSHA256: domainsecurity.SHA256Hex([]byte(input.SourceExactValue)),
		Canonicalizer: SourceFieldBindingCanonicalizerV2,
	}
	binding.BindingDigest = sourceFieldBindingDigestV2(binding)
	if err := ValidateSourceFieldBindingV2(binding); err != nil {
		return SourceFieldBindingV2{}, err
	}
	return binding, nil
}

func ValidateSourceFieldBindingV2(binding SourceFieldBindingV2) error {
	canonicalAccountID, canonicalErr := canonicalBankAccountTextV2(binding.SourceExactValue)
	if binding.SchemaVersion != SourceFieldBindingVersionV2 || binding.Purpose != SourceFieldBindingPurposeV2 ||
		!validSourceFieldIdentifierV2(binding.FactID) || !validClaimType(binding.ClaimType) ||
		binding.CanonicalFieldName != SourceFieldBindingCanonicalFieldAccount || !validSourceFieldIdentifierV2(binding.CanonicalEntityID) || canonicalErr != nil ||
		binding.CanonicalAccountID != canonicalAccountID || binding.CanonicalAccountID != strings.TrimSpace(binding.CanonicalAccountID) ||
		!validSourceFieldIdentifierV2(binding.SourceRecordID) ||
		!validSHA256(binding.RawArtifactSHA256) || !validSHA256(binding.SourceRecordSHA256) ||
		!validSourceFieldPathV2(binding.SourceRecordPath) || !validSourceFieldPathV2(binding.SourceRecordIDPath) ||
		!validSourceFieldPathV2(binding.SourceEntityIDPath) || !validSourceFieldPathV2(binding.SourceFieldPath) ||
		!sourceJSONPointerWithinRecordV2(binding.SourceRecordPath, binding.SourceRecordIDPath) ||
		!sourceJSONPointerWithinRecordV2(binding.SourceRecordPath, binding.SourceEntityIDPath) ||
		!sourceJSONPointerWithinRecordV2(binding.SourceRecordPath, binding.SourceFieldPath) ||
		binding.SourceScalarKind != SourceFieldBindingScalarTextV2 || binding.Canonicalizer != SourceFieldBindingCanonicalizerV2 ||
		binding.SourceExactValueSHA256 != domainsecurity.SHA256Hex([]byte(binding.SourceExactValue)) ||
		!validSHA256(binding.BindingDigest) || binding.BindingDigest != sourceFieldBindingDigestV2(binding) {
		return errors.New("source field binding is invalid")
	}
	return nil
}

func CanonicalSourceFieldBindingsV2(bindings []SourceFieldBindingV2) ([]SourceFieldBindingV2, error) {
	if len(bindings) == 0 {
		return nil, errors.New("source field binding set is empty")
	}
	canonical := append([]SourceFieldBindingV2(nil), bindings...)
	for _, binding := range canonical {
		if err := ValidateSourceFieldBindingV2(binding); err != nil {
			return nil, err
		}
	}
	sort.Slice(canonical, func(left, right int) bool {
		return sourceFieldBindingSortKeyV2(canonical[left]) < sourceFieldBindingSortKeyV2(canonical[right])
	})
	for index := 1; index < len(canonical); index++ {
		if sourceFieldBindingIdentityKeyV2(canonical[index-1]) == sourceFieldBindingIdentityKeyV2(canonical[index]) {
			return nil, errors.New("source field binding set contains a duplicate source field")
		}
	}
	return canonical, nil
}

func SourceFieldBindingSetDigestV2(bindings []SourceFieldBindingV2) (string, error) {
	canonical, err := CanonicalSourceFieldBindingsV2(bindings)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(append([]byte(sourceFieldBindingSetDomainV2), body...)), nil
}

func validateCanonicalEvidenceMaterialV2(material CanonicalEvidenceMaterial) error {
	if material.Purpose != CanonicalEvidencePurposeV2 || len(material.SourceFieldBindings) == 0 ||
		!validSHA256(material.SourceFieldBindingSetDigest) {
		return errors.New("canonical evidence V2 source field authority is incomplete")
	}
	for index := 1; index < len(material.Facts); index++ {
		if material.Facts[index-1].FactID >= material.Facts[index].FactID {
			return errors.New("canonical evidence V2 facts are not canonical")
		}
	}
	facts := make(map[string]CanonicalEvidenceFact, len(material.Facts))
	semanticFacts := make(map[string]struct{}, len(material.Facts))
	for _, fact := range material.Facts {
		if !validSourceFieldIdentifierV2(fact.FactID) {
			return errors.New("canonical evidence V2 fact identifier is invalid")
		}
		payload, _ := json.Marshal(fact.NormalizedPayload)
		semanticKey := string(fact.ClaimType) + "\x00" + string(payload)
		if _, exists := semanticFacts[semanticKey]; exists {
			return errors.New("canonical evidence V2 contains duplicate semantic facts")
		}
		semanticFacts[semanticKey] = struct{}{}
		facts[fact.FactID] = fact
	}
	canonicalBindings, err := CanonicalSourceFieldBindingsV2(material.SourceFieldBindings)
	if err != nil || len(canonicalBindings) != len(material.SourceFieldBindings) {
		return errors.New("canonical evidence V2 source field bindings are invalid")
	}
	exactValueByFactField := map[string]string{}
	recordIdentityByID := map[string]string{}
	recordIDByIdentity := map[string]string{}
	for index, binding := range canonicalBindings {
		if binding != material.SourceFieldBindings[index] {
			return errors.New("canonical evidence V2 source field bindings are not canonical")
		}
		fact, found := facts[binding.FactID]
		if !found || fact.ClaimType != binding.ClaimType || fact.NormalizedPayload.AccountID == "" ||
			fact.NormalizedPayload.AccountID != binding.CanonicalAccountID ||
			canonicalFactEntityIDV2(fact.NormalizedPayload) != binding.CanonicalEntityID {
			return errors.New("canonical evidence V2 source field binding does not match its fact")
		}
		factFieldKey := binding.FactID + "\x00" + binding.CanonicalFieldName
		if previous, exists := exactValueByFactField[factFieldKey]; exists && previous != binding.SourceExactValueSHA256 {
			return errors.New("canonical evidence V2 source exact values conflict")
		}
		exactValueByFactField[factFieldKey] = binding.SourceExactValueSHA256
		recordIdentity := strings.Join([]string{
			binding.RawArtifactSHA256, binding.SourceRecordPath, binding.SourceRecordSHA256, binding.SourceRecordIDPath,
		}, "\x00")
		if previous, exists := recordIdentityByID[binding.SourceRecordID]; exists && previous != recordIdentity {
			return errors.New("canonical evidence V2 source record ID maps to conflicting rows")
		}
		if previous, exists := recordIDByIdentity[recordIdentity]; exists && previous != binding.SourceRecordID {
			return errors.New("canonical evidence V2 source row maps to conflicting record IDs")
		}
		recordIdentityByID[binding.SourceRecordID] = recordIdentity
		recordIDByIdentity[recordIdentity] = binding.SourceRecordID
	}
	digest, err := SourceFieldBindingSetDigestV2(canonicalBindings)
	if err != nil || digest != material.SourceFieldBindingSetDigest {
		return errors.New("canonical evidence V2 source field binding set integrity is invalid")
	}
	return nil
}

func sourceFieldBindingDigestV2(binding SourceFieldBindingV2) string {
	binding.BindingDigest = ""
	body, _ := json.Marshal(binding)
	return domainsecurity.SHA256Hex(append([]byte(sourceFieldBindingDigestDomainV2), body...))
}

func sourceFieldBindingSortKeyV2(binding SourceFieldBindingV2) string {
	return strings.Join([]string{
		binding.FactID,
		binding.CanonicalFieldName,
		binding.CanonicalEntityID,
		binding.SourceRecordID,
		binding.RawArtifactSHA256,
		binding.SourceRecordPath,
		binding.SourceRecordIDPath,
		binding.SourceEntityIDPath,
		binding.SourceFieldPath,
		binding.SourceExactValueSHA256,
		binding.BindingDigest,
	}, "\x00")
}

func sourceFieldBindingIdentityKeyV2(binding SourceFieldBindingV2) string {
	return strings.Join([]string{
		binding.FactID,
		binding.CanonicalFieldName,
		binding.CanonicalEntityID,
		binding.SourceRecordID,
		binding.RawArtifactSHA256,
		binding.SourceRecordPath,
		binding.SourceRecordIDPath,
		binding.SourceEntityIDPath,
		binding.SourceFieldPath,
	}, "\x00")
}

func canonicalBankAccountTextV2(value string) (string, error) {
	if value == "" || len(value) > maxSourceExactAccountBytesV2 || !utf8.ValidString(value) || value != strings.TrimSpace(value) {
		return "", errors.New("bank account source value is invalid")
	}
	var canonical strings.Builder
	canonical.Grow(len(value))
	previousSeparator := false
	digitCount := 0
	for index, character := range value {
		switch {
		case character >= '0' && character <= '9':
			canonical.WriteRune(character)
			digitCount++
			previousSeparator = false
		case character == ' ' || character == '-':
			if index == 0 || previousSeparator || digitCount == 0 {
				return "", errors.New("bank account source value is invalid")
			}
			previousSeparator = true
		default:
			return "", errors.New("bank account source value is invalid")
		}
	}
	if previousSeparator || digitCount == 0 {
		return "", errors.New("bank account source value is invalid")
	}
	return canonical.String(), nil
}

func validSourceFieldPathV2(value string) bool {
	if value == "" || len(value) > maxSourceFieldPathBytesV2 || value != strings.TrimSpace(value) ||
		!strings.HasPrefix(value, "/") || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			continue
		}
		if index+1 >= len(value) || (value[index+1] != '0' && value[index+1] != '1') {
			return false
		}
		index++
	}
	return true
}

// Source binding identifiers become map and canonical-order keys. Bounded,
// trim-stable, control-free UTF-8 makes the NUL-delimited internal tuple keys
// unambiguous and prevents attacker-controlled identifiers from inflating the
// registry or changing key boundaries.
func validSourceFieldIdentifierV2(value string) bool {
	if value == "" || len(value) > maxSourceFieldIdentifierBytesV2 || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func sourceJSONPointerWithinRecordV2(recordPath, valuePath string) bool {
	return valuePath != recordPath && strings.HasPrefix(valuePath, recordPath+"/")
}

func canonicalFactEntityIDV2(payload NormalizedClaimPayload) string {
	if payload.EntityID != "" {
		return payload.EntityID
	}
	return payload.SubjectID
}

func resolveSourceJSONPointerV2(root any, pointer string) (any, bool) {
	if !validSourceFieldPathV2(pointer) {
		return nil, false
	}
	current := root
	for _, escaped := range strings.Split(pointer[1:], "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(escaped, "~1", "/"), "~0", "~")
		switch container := current.(type) {
		case map[string]any:
			var found bool
			current, found = container[token]
			if !found {
				return nil, false
			}
		case []any:
			if token == "" || len(token) > 1 && token[0] == '0' {
				return nil, false
			}
			index, err := strconv.ParseUint(token, 10, 31)
			if err != nil || index >= uint64(len(container)) {
				return nil, false
			}
			current = container[index]
		default:
			return nil, false
		}
	}
	return current, true
}
