package evidence

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	CanonicalEvidenceVersionV3               = 3
	CanonicalEvidencePurposeV3               = "analytix.canonical-evidence/v3"
	AcceptedSlotSourceBindingSchemaVersionV1 = 1
	AcceptedSlotSourceBindingPurposeV1       = "analytix.accepted-slot-source-binding/v1"
	AcceptedSlotSourceFieldAccountV1         = "account"
	AcceptedSlotSourceFieldCardV1            = "card"
)

func AcceptedSlotSourceFieldForModelEntityAliasV1(alias string) (string, error) {
	entityType, _, err := domaincaseentity.ParseModelEntityAliasV1(alias)
	if err != nil {
		return "", errors.New("accepted slot source alias is invalid")
	}
	switch entityType {
	case domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1:
		return AcceptedSlotSourceFieldAccountV1, nil
	case domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1:
		return AcceptedSlotSourceFieldCardV1, nil
	default:
		return "", errors.New("accepted slot source alias field is unsupported")
	}
}

func IsAcceptedSlotSourceFieldV1(field string) bool {
	return field == AcceptedSlotSourceFieldAccountV1 || field == AcceptedSlotSourceFieldCardV1
}

// AcceptedSlotSourceBindingV1 is private evidence-registry material. It binds
// one accepted entity slot to an already witnessed source-row locator without
// persisting the source-exact value. The locator and AuthorityEntityRef remain
// private and must never enter ordinary history, HTTP/SSE, logs, or renderer
// state; local display consumes them only through a retained DSV2 callback.
type AcceptedSlotSourceBindingV1 struct {
	SchemaVersion   int      `json:"schemaVersion"`
	Purpose         string   `json:"purpose"`
	FactIDs         []string `json:"factIds"`
	EntityReference string   `json:"entityReference"`
	SourceRecordID  string   `json:"sourceRecordId"`
	SourceFileID    string   `json:"sourceFileId"`
	SourceRowNumber uint64   `json:"sourceRowNumber"`
	Field           string   `json:"field"`
	BindingDigest   string   `json:"bindingDigest"`
}

type AcceptedSlotSourceBindingInputV1 struct {
	FactIDs         []string
	EntityReference string
	SourceRecordID  string
	SourceFileID    string
	SourceRowNumber uint64
	Field           string
}

func NewAcceptedSlotSourceBindingV1(
	input AcceptedSlotSourceBindingInputV1,
) (AcceptedSlotSourceBindingV1, error) {
	binding := AcceptedSlotSourceBindingV1{
		SchemaVersion:   AcceptedSlotSourceBindingSchemaVersionV1,
		Purpose:         AcceptedSlotSourceBindingPurposeV1,
		FactIDs:         canonicalEvidenceStrings(input.FactIDs),
		EntityReference: strings.TrimSpace(input.EntityReference),
		SourceRecordID:  strings.TrimSpace(input.SourceRecordID),
		SourceFileID:    strings.TrimSpace(input.SourceFileID),
		SourceRowNumber: input.SourceRowNumber,
		Field:           strings.TrimSpace(input.Field),
	}
	binding.BindingDigest = acceptedSlotSourceBindingDigestV1(binding)
	if err := ValidateAcceptedSlotSourceBindingV1(binding); err != nil {
		return AcceptedSlotSourceBindingV1{}, err
	}
	return binding, nil
}

func ValidateAcceptedSlotSourceBindingV1(binding AcceptedSlotSourceBindingV1) error {
	if binding.SchemaVersion != AcceptedSlotSourceBindingSchemaVersionV1 ||
		binding.Purpose != AcceptedSlotSourceBindingPurposeV1 ||
		len(binding.FactIDs) == 0 || !canonicalAcceptedSlotFactIDsV1(binding.FactIDs) ||
		domaincaseentity.ValidateReferenceV1(binding.EntityReference) != nil ||
		!strings.HasPrefix(binding.SourceRecordID, SourceRowRecordIDPrefixV1) ||
		!domainsecurity.IsSHA256Hex(strings.TrimPrefix(binding.SourceRecordID, SourceRowRecordIDPrefixV1)) ||
		!validRawArtifactHostSourceFileIDV1(binding.SourceFileID) ||
		binding.SourceRowNumber == 0 || binding.SourceRowNumber > maxSourceRowJSONIntegerV1 ||
		!IsAcceptedSlotSourceFieldV1(binding.Field) ||
		!domainsecurity.IsSHA256Hex(binding.BindingDigest) ||
		binding.BindingDigest != acceptedSlotSourceBindingDigestV1(binding) {
		return errors.New("accepted slot source binding is invalid")
	}
	return nil
}

func CanonicalAcceptedSlotSourceBindingsV1(
	bindings []AcceptedSlotSourceBindingV1,
) ([]AcceptedSlotSourceBindingV1, error) {
	if len(bindings) == 0 {
		return nil, errors.New("accepted slot source binding set is empty")
	}
	canonical := append([]AcceptedSlotSourceBindingV1(nil), bindings...)
	for _, binding := range canonical {
		if err := ValidateAcceptedSlotSourceBindingV1(binding); err != nil {
			return nil, err
		}
	}
	sort.Slice(canonical, func(left, right int) bool {
		return acceptedSlotSourceBindingSortKeyV1(canonical[left]) <
			acceptedSlotSourceBindingSortKeyV1(canonical[right])
	})
	for index := 1; index < len(canonical); index++ {
		if acceptedSlotSourceBindingIdentityKeyV1(canonical[index-1]) ==
			acceptedSlotSourceBindingIdentityKeyV1(canonical[index]) {
			return nil, errors.New("accepted slot source binding set contains a duplicate source field")
		}
	}
	return canonical, nil
}

func AcceptedSlotSourceBindingSetDigestV1(bindings []AcceptedSlotSourceBindingV1) (string, error) {
	canonical, err := CanonicalAcceptedSlotSourceBindingsV1(bindings)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(append([]byte("analytix.accepted-slot-source-binding-set/v1\x00"), body...)), nil
}

func validateCanonicalEvidenceMaterialV3(material CanonicalEvidenceMaterial) error {
	if material.Purpose != CanonicalEvidencePurposeV3 ||
		len(material.AcceptedSlotSourceBindings) == 0 ||
		!domainsecurity.IsSHA256Hex(material.AcceptedSlotSourceBindingSetDigest) ||
		len(material.SourceFieldBindings) != 0 || material.SourceFieldBindingSetDigest != "" {
		return errors.New("canonical evidence V3 accepted slot lineage is incomplete")
	}
	canonical, err := CanonicalAcceptedSlotSourceBindingsV1(material.AcceptedSlotSourceBindings)
	if err != nil || len(canonical) != len(material.AcceptedSlotSourceBindings) {
		return errors.New("canonical evidence V3 accepted slot lineage is invalid")
	}
	for index := range canonical {
		if !reflect.DeepEqual(canonical[index], material.AcceptedSlotSourceBindings[index]) {
			return errors.New("canonical evidence V3 accepted slot lineage is not canonical")
		}
	}
	digest, err := AcceptedSlotSourceBindingSetDigestV1(canonical)
	if err != nil || digest != material.AcceptedSlotSourceBindingSetDigest {
		return errors.New("canonical evidence V3 accepted slot lineage digest is invalid")
	}
	facts := make(map[string]CanonicalEvidenceFact, len(material.Facts))
	factIDs := make([]string, 0, len(material.Facts))
	for _, fact := range material.Facts {
		facts[fact.FactID] = fact
		factIDs = append(factIDs, fact.FactID)
	}
	factIDs = canonicalEvidenceStrings(factIDs)
	for _, binding := range canonical {
		if !reflect.DeepEqual(binding.FactIDs, factIDs) {
			return errors.New("canonical evidence V3 accepted slot lineage does not bind the exact fact set")
		}
		for _, factID := range binding.FactIDs {
			fact, found := facts[factID]
			if !found || !claimPayloadContainsExactEntityReferenceV1(fact.NormalizedPayload, binding.EntityReference) {
				return errors.New("canonical evidence V3 accepted slot lineage has no matching fact")
			}
		}
	}
	return nil
}

func claimPayloadContainsExactEntityReferenceV1(payload NormalizedClaimPayload, reference string) bool {
	return payload.SubjectID == reference || payload.EntityID == reference ||
		payload.AccountID == reference || payload.CounterpartyID == reference
}

func acceptedSlotSourceBindingDigestV1(binding AcceptedSlotSourceBindingV1) string {
	copy := binding
	copy.BindingDigest = ""
	body, _ := json.Marshal(copy)
	return domainsecurity.SHA256Hex(append([]byte("analytix.accepted-slot-source-binding/record/v1\x00"), body...))
}

func acceptedSlotSourceBindingSortKeyV1(binding AcceptedSlotSourceBindingV1) string {
	return strings.Join([]string{
		binding.EntityReference,
		binding.SourceFileID,
		strconv.FormatUint(binding.SourceRowNumber, 10),
		binding.Field,
		binding.SourceRecordID,
	}, "\x00")
}

func acceptedSlotSourceBindingIdentityKeyV1(binding AcceptedSlotSourceBindingV1) string {
	return strings.Join([]string{
		binding.EntityReference,
		binding.SourceFileID,
		strconv.FormatUint(binding.SourceRowNumber, 10),
		binding.Field,
	}, "\x00")
}

func canonicalAcceptedSlotFactIDsV1(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) ||
			index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}
