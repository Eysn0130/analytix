package evidence

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ClaimProposalVersion     = 1
	ClaimRecordVersion       = 1
	CanonicalEvidenceVersion = 1
)

type ClaimType string

const (
	ClaimAmount                ClaimType = "amount"
	ClaimCount                 ClaimType = "count"
	ClaimAccount               ClaimType = "account"
	ClaimEntity                ClaimType = "entity"
	ClaimDirection             ClaimType = "direction"
	ClaimDateRange             ClaimType = "date_range"
	ClaimRelationship          ClaimType = "relationship"
	ClaimQuote                 ClaimType = "quote"
	ClaimDeviceIdentifier      ClaimType = "device_identifier"
	ClaimOwnership             ClaimType = "ownership"
	ClaimControl               ClaimType = "control"
	ClaimAddress               ClaimType = "address"
	ClaimChange                ClaimType = "change"
	ClaimBidCertificate        ClaimType = "bid_certificate"
	ClaimBidEditMetadata       ClaimType = "bid_edit_metadata"
	ClaimLegalCharacterization ClaimType = "legal_characterization"
)

type ClaimSupportState string

const (
	ClaimVerified   ClaimSupportState = "verified"
	ClaimPartial    ClaimSupportState = "partial"
	ClaimUnresolved ClaimSupportState = "unresolved"
	ClaimRefuted    ClaimSupportState = "refuted"
)

type NormalizedClaimPayload struct {
	SubjectID             string `json:"subjectId"`
	EntityID              string `json:"entityId"`
	AccountID             string `json:"accountId"`
	CounterpartyID        string `json:"counterpartyId"`
	AmountMinor           string `json:"amountMinor"`
	Count                 string `json:"count"`
	Currency              string `json:"currency"`
	Direction             string `json:"direction"`
	StartAt               string `json:"startAt"`
	EndAt                 string `json:"endAt"`
	RelationshipType      string `json:"relationshipType"`
	Quote                 string `json:"quote"`
	DeviceKind            string `json:"deviceKind"`
	DeviceIdentifier      string `json:"deviceIdentifier"`
	AttributeName         string `json:"attributeName"`
	AttributeValue        string `json:"attributeValue"`
	LegalCharacterization string `json:"legalCharacterization"`
	Granularity           string `json:"granularity"`
}

type ClaimProposal struct {
	SchemaVersion      int                    `json:"schemaVersion"`
	ProposalID         string                 `json:"proposalId"`
	ClaimType          ClaimType              `json:"claimType"`
	NormalizedPayload  NormalizedClaimPayload `json:"normalizedPayload"`
	EvidenceIDs        []string               `json:"evidenceIds"`
	CounterEvidenceIDs []string               `json:"counterEvidenceIds"`
}

type CanonicalEvidenceFact struct {
	FactID            string                 `json:"factId"`
	ClaimType         ClaimType              `json:"claimType"`
	NormalizedPayload NormalizedClaimPayload `json:"normalizedPayload"`
}

type CanonicalEvidenceMaterial struct {
	SchemaVersion                      int                           `json:"schemaVersion"`
	Purpose                            string                        `json:"purpose,omitempty"`
	Facts                              []CanonicalEvidenceFact       `json:"facts"`
	SourceFieldBindings                []SourceFieldBindingV2        `json:"sourceFieldBindings,omitempty"`
	SourceFieldBindingSetDigest        string                        `json:"sourceFieldBindingSetDigest,omitempty"`
	AcceptedSlotSourceBindings         []AcceptedSlotSourceBindingV1 `json:"acceptedSlotSourceBindings,omitempty"`
	AcceptedSlotSourceBindingSetDigest string                        `json:"acceptedSlotSourceBindingSetDigest,omitempty"`
}

type ClaimRecord struct {
	SchemaVersion       int                    `json:"schemaVersion"`
	ClaimID             string                 `json:"claimId"`
	ClaimType           ClaimType              `json:"claimType"`
	NormalizedPayload   NormalizedClaimPayload `json:"normalizedPayload"`
	SupportState        ClaimSupportState      `json:"supportState"`
	EvidenceIDs         []string               `json:"evidenceIds"`
	CounterEvidenceIDs  []string               `json:"counterEvidenceIds"`
	SupportedScope      *EvidenceQueryRange    `json:"supportedScope,omitempty"`
	AllowedWording      []string               `json:"allowedWording"`
	ProhibitedUpgrades  []string               `json:"prohibitedUpgrades"`
	RequiresHumanReview bool                   `json:"requiresHumanReview"`
	VerifierReceiptID   string                 `json:"verifierReceiptId"`
	VerificationReason  string                 `json:"verificationReason"`
	VerifiedAt          string                 `json:"verifiedAt"`
	RecordDigest        string                 `json:"recordDigest"`
}

type ClaimRecordInput struct {
	ClaimID             string
	Proposal            ClaimProposal
	SupportState        ClaimSupportState
	EvidenceIDs         []string
	CounterEvidenceIDs  []string
	SupportedScope      *EvidenceQueryRange
	AllowedWording      []string
	ProhibitedUpgrades  []string
	RequiresHumanReview bool
	VerifierReceiptID   string
	VerificationReason  string
	VerifiedAt          time.Time
}

func ParseClaimProposal(raw json.RawMessage) (ClaimProposal, error) {
	var proposal ClaimProposal
	if err := strictEvidenceDecode(raw, &proposal); err != nil {
		return ClaimProposal{}, err
	}
	return NormalizeClaimProposal(proposal)
}

func NormalizeClaimProposal(proposal ClaimProposal) (ClaimProposal, error) {
	proposal.ProposalID = strings.TrimSpace(proposal.ProposalID)
	proposal.EvidenceIDs = canonicalEvidenceStrings(proposal.EvidenceIDs)
	proposal.CounterEvidenceIDs = canonicalEvidenceStrings(proposal.CounterEvidenceIDs)
	if proposal.EvidenceIDs == nil {
		proposal.EvidenceIDs = []string{}
	}
	if proposal.CounterEvidenceIDs == nil {
		proposal.CounterEvidenceIDs = []string{}
	}
	payload, err := normalizeClaimPayload(proposal.ClaimType, proposal.NormalizedPayload)
	if err != nil {
		return ClaimProposal{}, err
	}
	proposal.NormalizedPayload = payload
	if proposal.SchemaVersion != ClaimProposalVersion || proposal.ProposalID == "" || !validClaimType(proposal.ClaimType) {
		return ClaimProposal{}, errors.New("claim proposal is incomplete")
	}
	return proposal, nil
}

func ParseCanonicalEvidenceMaterial(raw json.RawMessage) (CanonicalEvidenceMaterial, error) {
	var material CanonicalEvidenceMaterial
	if err := strictEvidenceDecode(raw, &material); err != nil {
		return CanonicalEvidenceMaterial{}, err
	}
	if (material.SchemaVersion != CanonicalEvidenceVersion && material.SchemaVersion != CanonicalEvidenceVersionV2 &&
		material.SchemaVersion != CanonicalEvidenceVersionV3) || material.Facts == nil {
		return CanonicalEvidenceMaterial{}, errors.New("canonical evidence material is incomplete")
	}
	seen := map[string]bool{}
	for index := range material.Facts {
		fact := &material.Facts[index]
		originalFactID := fact.FactID
		fact.FactID = strings.TrimSpace(fact.FactID)
		payload, err := normalizeClaimPayload(fact.ClaimType, fact.NormalizedPayload)
		if err != nil || fact.FactID == "" || seen[fact.FactID] ||
			(material.SchemaVersion != CanonicalEvidenceVersion && originalFactID != fact.FactID) {
			return CanonicalEvidenceMaterial{}, errors.New("canonical evidence fact is invalid")
		}
		seen[fact.FactID] = true
		fact.NormalizedPayload = payload
	}
	if material.SchemaVersion == CanonicalEvidenceVersion {
		if material.Purpose != "" || len(material.SourceFieldBindings) != 0 || material.SourceFieldBindingSetDigest != "" ||
			len(material.AcceptedSlotSourceBindings) != 0 || material.AcceptedSlotSourceBindingSetDigest != "" {
			return CanonicalEvidenceMaterial{}, errors.New("canonical evidence V1 cannot carry source field bindings")
		}
		return material, nil
	}
	if material.SchemaVersion == CanonicalEvidenceVersionV2 {
		if len(material.AcceptedSlotSourceBindings) != 0 || material.AcceptedSlotSourceBindingSetDigest != "" {
			return CanonicalEvidenceMaterial{}, errors.New("canonical evidence V2 cannot carry accepted slot source bindings")
		}
		if err := validateCanonicalEvidenceMaterialV2(material); err != nil {
			return CanonicalEvidenceMaterial{}, err
		}
		return material, nil
	}
	if err := validateCanonicalEvidenceMaterialV3(material); err != nil {
		return CanonicalEvidenceMaterial{}, err
	}
	return material, nil
}

func NewClaimRecord(input ClaimRecordInput) (ClaimRecord, error) {
	proposal, err := NormalizeClaimProposal(input.Proposal)
	if err != nil {
		return ClaimRecord{}, err
	}
	verifiedAt := input.VerifiedAt.UTC()
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}
	record := ClaimRecord{
		SchemaVersion: ClaimRecordVersion, ClaimID: strings.TrimSpace(input.ClaimID), ClaimType: proposal.ClaimType,
		NormalizedPayload: proposal.NormalizedPayload, SupportState: input.SupportState,
		EvidenceIDs: canonicalEvidenceStrings(input.EvidenceIDs), CounterEvidenceIDs: canonicalEvidenceStrings(input.CounterEvidenceIDs),
		SupportedScope: cloneEvidenceQueryRange(input.SupportedScope), AllowedWording: canonicalEvidenceStrings(input.AllowedWording),
		ProhibitedUpgrades: canonicalEvidenceStrings(input.ProhibitedUpgrades), RequiresHumanReview: input.RequiresHumanReview,
		VerifierReceiptID: strings.TrimSpace(input.VerifierReceiptID), VerificationReason: strings.TrimSpace(input.VerificationReason),
		VerifiedAt: verifiedAt.Format(time.RFC3339Nano),
	}
	if record.EvidenceIDs == nil {
		record.EvidenceIDs = []string{}
	}
	if record.CounterEvidenceIDs == nil {
		record.CounterEvidenceIDs = []string{}
	}
	if record.AllowedWording == nil {
		record.AllowedWording = []string{}
	}
	if record.ProhibitedUpgrades == nil {
		record.ProhibitedUpgrades = []string{}
	}
	record.RecordDigest = claimRecordDigest(record)
	if err := ValidateClaimRecord(record); err != nil {
		return ClaimRecord{}, err
	}
	return record, nil
}

func ParseClaimRecord(value any) (ClaimRecord, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ClaimRecord{}, err
	}
	var record ClaimRecord
	if err := strictEvidenceDecode(body, &record); err != nil {
		return ClaimRecord{}, err
	}
	if err := ValidateClaimRecord(record); err != nil {
		return ClaimRecord{}, err
	}
	return record, nil
}

func ValidateClaimRecord(record ClaimRecord) error {
	if record.SchemaVersion != ClaimRecordVersion || strings.TrimSpace(record.ClaimID) == "" || !validClaimType(record.ClaimType) ||
		record.EvidenceIDs == nil || record.CounterEvidenceIDs == nil || record.AllowedWording == nil || record.ProhibitedUpgrades == nil ||
		strings.TrimSpace(record.VerificationReason) == "" || strings.TrimSpace(record.VerifiedAt) == "" || !validSHA256(record.RecordDigest) {
		return errors.New("claim record is incomplete")
	}
	if _, err := normalizeClaimPayload(record.ClaimType, record.NormalizedPayload); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, record.VerifiedAt); err != nil {
		return errors.New("claim record verifiedAt is invalid")
	}
	switch record.SupportState {
	case ClaimVerified, ClaimPartial:
		if len(record.EvidenceIDs) == 0 || record.SupportedScope == nil || strings.TrimSpace(record.VerifierReceiptID) == "" || len(record.AllowedWording) == 0 {
			return errors.New("supported claim record lacks verifier authority")
		}
		if record.VerifierReceiptID != VerifierReceiptDigest(record.ClaimID, record.ClaimType, record.NormalizedPayload, record.EvidenceIDs, record.CounterEvidenceIDs, record.SupportState) {
			return errors.New("supported claim verifier receipt is invalid")
		}
		if err := validateEvidenceQueryRange(*record.SupportedScope); err != nil {
			return err
		}
	case ClaimRefuted:
		if len(record.CounterEvidenceIDs) == 0 || strings.TrimSpace(record.VerifierReceiptID) == "" || len(record.AllowedWording) != 0 {
			return errors.New("refuted claim record is invalid")
		}
		if record.VerifierReceiptID != VerifierReceiptDigest(record.ClaimID, record.ClaimType, record.NormalizedPayload, record.EvidenceIDs, record.CounterEvidenceIDs, record.SupportState) {
			return errors.New("refuted claim verifier receipt is invalid")
		}
	case ClaimUnresolved:
		if record.SupportedScope != nil || record.VerifierReceiptID != "" || len(record.AllowedWording) != 0 {
			return errors.New("unresolved claim record cannot carry support authority")
		}
	default:
		return errors.New("claim support state is invalid")
	}
	if claimRecordDigest(record) != record.RecordDigest {
		return errors.New("claim record integrity is invalid")
	}
	return nil
}

func SourceTypeSupportsClaim(sourceType string, claimType ClaimType) bool {
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	capabilities := map[string]map[ClaimType]bool{
		"transactions": {
			ClaimAmount: true, ClaimCount: true, ClaimAccount: true, ClaimDirection: true, ClaimDateRange: true,
		},
		SourceTypeTransactionDatasetInventory: {
			ClaimCount: true,
		},
		"corporate": {
			ClaimOwnership: true, ClaimControl: true, ClaimAddress: true, ClaimChange: true,
		},
		"bids": {
			ClaimQuote: true, ClaimBidCertificate: true, ClaimBidEditMetadata: true,
		},
		"device_logs": {
			ClaimDeviceIdentifier: true,
		},
		"relationship_material": {
			ClaimRelationship: true,
		},
	}
	return capabilities[sourceType][claimType]
}

func ClaimPayloadExactlyMatches(left NormalizedClaimPayload, right NormalizedClaimPayload) bool {
	return left == right
}

func normalizeClaimPayload(claimType ClaimType, payload NormalizedClaimPayload) (NormalizedClaimPayload, error) {
	payload.SubjectID = strings.TrimSpace(payload.SubjectID)
	payload.EntityID = strings.TrimSpace(payload.EntityID)
	payload.AccountID = strings.TrimSpace(payload.AccountID)
	payload.CounterpartyID = strings.TrimSpace(payload.CounterpartyID)
	payload.AmountMinor = strings.TrimSpace(payload.AmountMinor)
	payload.Count = strings.TrimSpace(payload.Count)
	payload.Currency = strings.ToUpper(strings.TrimSpace(payload.Currency))
	payload.Direction = strings.ToLower(strings.TrimSpace(payload.Direction))
	payload.StartAt = canonicalClaimTime(payload.StartAt)
	payload.EndAt = canonicalClaimTime(payload.EndAt)
	payload.RelationshipType = strings.ToLower(strings.TrimSpace(payload.RelationshipType))
	payload.Quote = strings.TrimSpace(strings.ReplaceAll(payload.Quote, "\r\n", "\n"))
	payload.DeviceKind = strings.ToLower(strings.TrimSpace(payload.DeviceKind))
	payload.DeviceIdentifier = normalizeDeviceIdentifier(payload.DeviceKind, payload.DeviceIdentifier)
	payload.AttributeName = strings.ToLower(strings.TrimSpace(payload.AttributeName))
	payload.AttributeValue = strings.TrimSpace(payload.AttributeValue)
	payload.LegalCharacterization = strings.TrimSpace(payload.LegalCharacterization)
	payload.Granularity = strings.ToLower(strings.TrimSpace(payload.Granularity))
	if payload.SubjectID == "" || !validClaimType(claimType) {
		return NormalizedClaimPayload{}, errors.New("claim payload subject or type is invalid")
	}
	switch claimType {
	case ClaimAmount, ClaimCount, ClaimAccount, ClaimDirection, ClaimDateRange, ClaimRelationship,
		ClaimQuote, ClaimDeviceIdentifier, ClaimLegalCharacterization:
		if payload.EntityID != "" && payload.EntityID != payload.SubjectID {
			return NormalizedClaimPayload{}, errors.New("rendered claim subject must equal its evidence entity")
		}
	}
	if payload.AmountMinor != "" && !canonicalUnsignedInteger(payload.AmountMinor) {
		return NormalizedClaimPayload{}, errors.New("claim amountMinor is invalid")
	}
	if payload.Count != "" && !canonicalUnsignedInteger(payload.Count) {
		return NormalizedClaimPayload{}, errors.New("claim count is invalid")
	}
	if (payload.StartAt == "") != (payload.EndAt == "") {
		return NormalizedClaimPayload{}, errors.New("claim date range is incomplete")
	}
	if payload.StartAt != "" {
		start, startErr := time.Parse(time.RFC3339Nano, payload.StartAt)
		end, endErr := time.Parse(time.RFC3339Nano, payload.EndAt)
		if startErr != nil || endErr != nil || end.Before(start) {
			return NormalizedClaimPayload{}, errors.New("claim date range is invalid")
		}
	}
	if err := validateClaimPayloadDiscriminant(claimType, payload); err != nil {
		return NormalizedClaimPayload{}, err
	}
	switch claimType {
	case ClaimAmount:
		if payload.EntityID == "" || payload.AccountID == "" || payload.AmountMinor == "" || payload.Currency == "" ||
			(payload.Direction != "in" && payload.Direction != "out") || payload.StartAt == "" || payload.Granularity == "" {
			return NormalizedClaimPayload{}, errors.New("amount claim requires entity, account, amount, currency, direction, date range, and granularity")
		}
	case ClaimCount:
		if payload.EntityID == "" || payload.Count == "" || payload.Granularity == "" ||
			(payload.Granularity != "dataset_table_rows" && payload.StartAt == "") {
			return NormalizedClaimPayload{}, errors.New("count claim requires entity, count, date range, and granularity")
		}
	case ClaimAccount:
		if payload.AccountID == "" {
			return NormalizedClaimPayload{}, errors.New("account claim requires accountId")
		}
	case ClaimEntity:
		if payload.EntityID == "" {
			return NormalizedClaimPayload{}, errors.New("entity claim requires entityId")
		}
	case ClaimDirection:
		if payload.Direction != "in" && payload.Direction != "out" {
			return NormalizedClaimPayload{}, errors.New("direction claim requires in or out")
		}
	case ClaimDateRange:
		if payload.StartAt == "" {
			return NormalizedClaimPayload{}, errors.New("date range claim requires startAt and endAt")
		}
	case ClaimRelationship:
		if payload.CounterpartyID == "" || !allowedRelationshipType(payload.RelationshipType) {
			return NormalizedClaimPayload{}, errors.New("relationship claim requires counterparty and relationship type")
		}
	case ClaimQuote:
		if payload.Quote == "" {
			return NormalizedClaimPayload{}, errors.New("quote claim requires exact quote")
		}
	case ClaimDeviceIdentifier:
		if (payload.DeviceKind != "mac" && payload.DeviceKind != "ip" && payload.DeviceKind != "device_id") || payload.DeviceIdentifier == "" ||
			(payload.DeviceKind == "mac" && !validMAC(payload.DeviceIdentifier)) || (payload.DeviceKind == "ip" && net.ParseIP(payload.DeviceIdentifier) == nil) {
			return NormalizedClaimPayload{}, errors.New("device claim requires kind and identifier")
		}
	case ClaimOwnership, ClaimControl, ClaimAddress, ClaimChange, ClaimBidCertificate, ClaimBidEditMetadata:
		if payload.EntityID == "" || !allowedAttributeName(claimType, payload.AttributeName) || payload.AttributeValue == "" {
			return NormalizedClaimPayload{}, errors.New("attribute claim requires entity, name, and value")
		}
	case ClaimLegalCharacterization:
		if payload.LegalCharacterization == "" {
			return NormalizedClaimPayload{}, errors.New("legal characterization claim requires a characterization")
		}
	}
	return payload, nil
}

func validateClaimPayloadDiscriminant(claimType ClaimType, payload NormalizedClaimPayload) error {
	allowed := allowedClaimPayloadFields(claimType)
	fields := map[string]string{
		"subjectId": payload.SubjectID, "entityId": payload.EntityID, "accountId": payload.AccountID,
		"counterpartyId": payload.CounterpartyID, "amountMinor": payload.AmountMinor, "count": payload.Count,
		"currency": payload.Currency, "direction": payload.Direction, "startAt": payload.StartAt, "endAt": payload.EndAt,
		"relationshipType": payload.RelationshipType, "quote": payload.Quote, "deviceKind": payload.DeviceKind,
		"deviceIdentifier": payload.DeviceIdentifier, "attributeName": payload.AttributeName, "attributeValue": payload.AttributeValue,
		"legalCharacterization": payload.LegalCharacterization, "granularity": payload.Granularity,
	}
	for name, value := range fields {
		if value != "" && !allowed[name] {
			return errors.New("claim payload contains a field outside its discriminated variant")
		}
	}
	return nil
}

func allowedClaimPayloadFields(claimType ClaimType) map[string]bool {
	fields := map[string]bool{"subjectId": true, "entityId": true, "granularity": true}
	for _, name := range map[ClaimType][]string{
		ClaimAmount:                {"accountId", "amountMinor", "currency", "direction", "startAt", "endAt"},
		ClaimCount:                 {"count", "startAt", "endAt"},
		ClaimAccount:               {"accountId"},
		ClaimEntity:                {},
		ClaimDirection:             {"accountId", "direction", "startAt", "endAt"},
		ClaimDateRange:             {"accountId", "direction", "startAt", "endAt"},
		ClaimRelationship:          {"counterpartyId", "relationshipType"},
		ClaimQuote:                 {"quote"},
		ClaimDeviceIdentifier:      {"deviceKind", "deviceIdentifier"},
		ClaimOwnership:             {"attributeName", "attributeValue"},
		ClaimControl:               {"attributeName", "attributeValue"},
		ClaimAddress:               {"attributeName", "attributeValue"},
		ClaimChange:                {"attributeName", "attributeValue"},
		ClaimBidCertificate:        {"attributeName", "attributeValue"},
		ClaimBidEditMetadata:       {"attributeName", "attributeValue"},
		ClaimLegalCharacterization: {"legalCharacterization"},
	}[claimType] {
		fields[name] = true
	}
	return fields
}

func allowedRelationshipType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "family", "employment", "ownership", "control", "representation", "contact", "transaction_counterparty", "shared_device", "shared_address":
		return true
	default:
		return false
	}
}

func allowedAttributeName(claimType ClaimType, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	allowed := map[ClaimType]map[string]bool{
		ClaimOwnership:       {"shareholder": true, "ownership_percentage": true, "beneficial_owner": true},
		ClaimControl:         {"controller": true, "control_type": true},
		ClaimAddress:         {"registered_address": true, "business_address": true},
		ClaimChange:          {"change_type": true, "changed_field": true, "changed_at": true},
		ClaimBidCertificate:  {"certificate_number": true, "certificate_type": true, "certificate_status": true},
		ClaimBidEditMetadata: {"document_author": true, "editor": true, "edited_at": true, "software": true, "revision_id": true},
	}
	return allowed[claimType][value]
}

func validClaimType(claimType ClaimType) bool {
	switch claimType {
	case ClaimAmount, ClaimCount, ClaimAccount, ClaimEntity, ClaimDirection, ClaimDateRange, ClaimRelationship, ClaimQuote,
		ClaimDeviceIdentifier, ClaimOwnership, ClaimControl, ClaimAddress, ClaimChange, ClaimBidCertificate,
		ClaimBidEditMetadata, ClaimLegalCharacterization:
		return true
	default:
		return false
	}
}

func canonicalUnsignedInteger(value string) bool {
	matched, _ := regexp.MatchString(`^(0|[1-9][0-9]*)$`, value)
	return matched
}

func canonicalClaimTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func normalizeDeviceIdentifier(kind string, value string) string {
	value = strings.TrimSpace(value)
	switch kind {
	case "mac":
		return strings.ToUpper(strings.ReplaceAll(value, "-", ":"))
	case "ip":
		if parsed := net.ParseIP(value); parsed != nil {
			return parsed.String()
		}
	}
	return value
}

func validMAC(value string) bool {
	hardware, err := net.ParseMAC(strings.TrimSpace(value))
	return err == nil && len(hardware) == 6
}

func cloneEvidenceQueryRange(scope *EvidenceQueryRange) *EvidenceQueryRange {
	if scope == nil {
		return nil
	}
	cloned := normalizeEvidenceQueryRange(*scope)
	return &cloned
}

func strictEvidenceDecode(raw []byte, target any) error {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func claimRecordDigest(record ClaimRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func VerifierReceiptDigest(claimID string, claimType ClaimType, payload NormalizedClaimPayload, evidenceIDs []string, counterEvidenceIDs []string, support ClaimSupportState) string {
	evidenceIDs = append([]string(nil), evidenceIDs...)
	counterEvidenceIDs = append([]string(nil), counterEvidenceIDs...)
	sort.Strings(evidenceIDs)
	sort.Strings(counterEvidenceIDs)
	body, _ := json.Marshal(struct {
		ClaimID            string                 `json:"claimId"`
		ClaimType          ClaimType              `json:"claimType"`
		Payload            NormalizedClaimPayload `json:"payload"`
		EvidenceIDs        []string               `json:"evidenceIds"`
		CounterEvidenceIDs []string               `json:"counterEvidenceIds"`
		Support            ClaimSupportState      `json:"support"`
	}{strings.TrimSpace(claimID), claimType, payload, evidenceIDs, counterEvidenceIDs, support})
	return domainsecurity.SHA256Hex(body)
}
