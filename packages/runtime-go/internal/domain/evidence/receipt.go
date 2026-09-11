package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const EvidenceReceiptVersion = 1

const SourceTypeTransactionDatasetInventory = "transaction_dataset_inventory"

type PaginationCompleteness string

const (
	PaginationComplete PaginationCompleteness = "complete"
	PaginationPartial  PaginationCompleteness = "partial"
)

type PIIClassification string

const (
	PIINone       PIIClassification = "none"
	PIIMasked     PIIClassification = "masked"
	PIIRestricted PIIClassification = "restricted"
	PIIControlled PIIClassification = "controlled"
)

type EvidenceQueryRange struct {
	EntityIDs   []string `json:"entityIds"`
	AccountIDs  []string `json:"accountIds"`
	Directions  []string `json:"directions"`
	StartAt     string   `json:"startAt"`
	EndAt       string   `json:"endAt"`
	SourceIDs   []string `json:"sourceIds"`
	FiltersHash string   `json:"filtersHash"`
}

type TransformationLineageStep struct {
	StepID             string `json:"stepId"`
	Transformer        string `json:"transformer"`
	TransformerVersion string `json:"transformerVersion"`
	InputHash          string `json:"inputHash"`
	OutputHash         string `json:"outputHash"`
}

// EvidenceReceipt is host-issued authority. The registry fields are sealed by
// the private append-only registry; an unregistered draft is never evidence.
type EvidenceReceipt struct {
	SchemaVersion          int                         `json:"schemaVersion"`
	ReceiptID              string                      `json:"receiptId"`
	ThreadID               string                      `json:"threadId"`
	TurnID                 string                      `json:"turnId"`
	CaseID                 string                      `json:"caseId"`
	CaseBindingHash        string                      `json:"caseBindingHash"`
	ContextEpoch           uint64                      `json:"contextEpoch"`
	ContextDigest          string                      `json:"contextDigest"`
	ExecutionGrantID       string                      `json:"executionGrantId"`
	ToolCallID             string                      `json:"toolCallId"`
	ServerIdentity         string                      `json:"serverIdentity"`
	ServerVersion          string                      `json:"serverVersion"`
	ConnectionEpoch        uint64                      `json:"connectionEpoch"`
	ToolName               string                      `json:"toolName"`
	ArgsHash               string                      `json:"argsHash"`
	ResultHash             string                      `json:"resultHash"`
	SourceType             string                      `json:"sourceType"`
	DatasetSnapshotID      string                      `json:"datasetSnapshotId"`
	QueryHash              string                      `json:"queryHash"`
	QueryRange             EvidenceQueryRange          `json:"range"`
	Granularity            string                      `json:"granularity"`
	Currency               string                      `json:"currency"`
	Timezone               string                      `json:"timezone"`
	PaginationCompleteness PaginationCompleteness      `json:"paginationCompleteness"`
	SourceRecordIDs        []string                    `json:"sourceRecordIds"`
	RawSHA256              string                      `json:"rawSha256"`
	TransformationLineage  []TransformationLineageStep `json:"transformationLineage"`
	PIIClassification      PIIClassification           `json:"piiClassification"`
	IssuedAt               string                      `json:"issuedAt"`
	ReceiptDigest          string                      `json:"receiptDigest"`
	RegistrySequence       uint64                      `json:"registrySequence"`
	PreviousRegistryDigest string                      `json:"previousRegistryDigest"`
	RegistryIntegrityProof string                      `json:"registryIntegrityProof"`
}

type EvidenceReceiptInput struct {
	ReceiptID              string
	Context                domainsecurity.TurnSecurityContext
	ExecutionGrantID       string
	ToolCallID             string
	ServerIdentity         string
	ServerVersion          string
	ConnectionEpoch        uint64
	ToolName               string
	ArgsHash               string
	ResultHash             string
	SourceType             string
	DatasetSnapshotID      string
	QueryHash              string
	QueryRange             EvidenceQueryRange
	Granularity            string
	Currency               string
	Timezone               string
	PaginationCompleteness PaginationCompleteness
	SourceRecordIDs        []string
	RawSHA256              string
	TransformationLineage  []TransformationLineageStep
	PIIClassification      PIIClassification
	IssuedAt               time.Time
}

func NewEvidenceReceiptDraft(input EvidenceReceiptInput) (EvidenceReceipt, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		!domainmodel.IsHostToolCallIDV1(input.ToolCallID) {
		return EvidenceReceipt{}, errors.New("evidence receipt draft requires current V2 case fact authority")
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	receipt := EvidenceReceipt{
		SchemaVersion: EvidenceReceiptVersion, ReceiptID: strings.TrimSpace(input.ReceiptID),
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, CaseID: input.Context.CaseID,
		CaseBindingHash: input.Context.CaseBindingHash, ContextEpoch: input.Context.ContextEpoch, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: strings.TrimSpace(input.ExecutionGrantID), ToolCallID: strings.TrimSpace(input.ToolCallID),
		ServerIdentity: strings.TrimSpace(input.ServerIdentity), ServerVersion: strings.TrimSpace(input.ServerVersion),
		ConnectionEpoch: input.ConnectionEpoch, ToolName: strings.TrimSpace(input.ToolName), ArgsHash: strings.TrimSpace(input.ArgsHash),
		ResultHash: strings.TrimSpace(input.ResultHash), SourceType: strings.TrimSpace(input.SourceType),
		DatasetSnapshotID: strings.TrimSpace(input.DatasetSnapshotID), QueryHash: strings.TrimSpace(input.QueryHash),
		QueryRange: normalizeEvidenceQueryRange(input.QueryRange), Granularity: strings.TrimSpace(input.Granularity),
		Currency: strings.ToUpper(strings.TrimSpace(input.Currency)), Timezone: strings.TrimSpace(input.Timezone),
		PaginationCompleteness: input.PaginationCompleteness, SourceRecordIDs: canonicalEvidenceStrings(input.SourceRecordIDs),
		RawSHA256: strings.TrimSpace(input.RawSHA256), TransformationLineage: cloneTransformationLineage(input.TransformationLineage),
		PIIClassification: input.PIIClassification, IssuedAt: issuedAt.Format(time.RFC3339Nano),
	}
	if receipt.SourceRecordIDs == nil {
		receipt.SourceRecordIDs = []string{}
	}
	if receipt.TransformationLineage == nil {
		receipt.TransformationLineage = []TransformationLineageStep{}
	}
	receipt.ReceiptDigest = evidenceReceiptDigest(receipt)
	if err := ValidateEvidenceReceiptDraft(receipt); err != nil {
		return EvidenceReceipt{}, err
	}
	return receipt, nil
}

func ParseEvidenceReceipt(value any) (EvidenceReceipt, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return EvidenceReceipt{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var receipt EvidenceReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return EvidenceReceipt{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EvidenceReceipt{}, errors.New("evidence receipt contains trailing JSON")
	}
	if err := ValidateEvidenceReceipt(receipt); err != nil {
		return EvidenceReceipt{}, err
	}
	return receipt, nil
}

func ValidateEvidenceReceiptDraft(receipt EvidenceReceipt) error {
	if err := validateEvidenceReceiptCore(receipt); err != nil {
		return err
	}
	if receipt.RegistrySequence != 0 || receipt.PreviousRegistryDigest != "" || receipt.RegistryIntegrityProof != "" {
		return errors.New("evidence receipt draft already contains registry authority")
	}
	return nil
}

func ValidateEvidenceReceipt(receipt EvidenceReceipt) error {
	if err := validateEvidenceReceiptCore(receipt); err != nil {
		return err
	}
	if receipt.RegistrySequence == 0 || !validSHA256(receipt.PreviousRegistryDigest) || !validSHA256(receipt.RegistryIntegrityProof) ||
		receipt.RegistryIntegrityProof != evidenceReceiptRegistryProof(receipt) {
		return errors.New("evidence receipt registry proof is invalid")
	}
	return nil
}

func EvidenceReceiptRecord(receipt EvidenceReceipt) map[string]any {
	body, _ := json.Marshal(receipt)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func CanonicalEvidenceBytes(raw json.RawMessage) ([]byte, error) {
	value, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func ValidateCanonicalEvidenceAgainstReceipt(receipt EvidenceReceipt, raw json.RawMessage) error {
	canonical, err := CanonicalEvidenceBytes(raw)
	if err != nil || domainsecurity.CanonicalJSONHash(canonical) != receipt.ResultHash {
		return errors.New("canonical evidence does not match receipt result hash")
	}
	material, err := ParseCanonicalEvidenceMaterial(canonical)
	if err != nil || (len(material.Facts) > 0) != (len(receipt.SourceRecordIDs) > 0) {
		return errors.New("canonical evidence facts do not match source record lineage")
	}
	for _, fact := range material.Facts {
		if !SourceTypeSupportsClaim(receipt.SourceType, fact.ClaimType) ||
			!EvidenceQueryRangeSupportsClaimPayload(receipt.QueryRange, fact.NormalizedPayload) ||
			!EvidenceReceiptMetadataSupportsClaim(receipt, fact.ClaimType, fact.NormalizedPayload) {
			return errors.New("canonical evidence fact exceeds receipt capability, scope, or metadata")
		}
	}
	if material.SchemaVersion == CanonicalEvidenceVersionV2 {
		if receipt.PIIClassification != PIIRestricted && receipt.PIIClassification != PIIControlled {
			return errors.New("canonical evidence V2 source fields require restricted PII authority")
		}
		for _, binding := range material.SourceFieldBindings {
			if !containsEvidenceValue(receipt.SourceRecordIDs, binding.SourceRecordID) ||
				binding.RawArtifactSHA256 != receipt.RawSHA256 {
				return errors.New("canonical evidence V2 source field exceeds receipt record lineage")
			}
		}
	}
	if material.SchemaVersion == CanonicalEvidenceVersionV3 {
		boundRecords := make(map[string]struct{}, len(material.AcceptedSlotSourceBindings))
		for _, binding := range material.AcceptedSlotSourceBindings {
			if !containsEvidenceValue(receipt.SourceRecordIDs, binding.SourceRecordID) ||
				(!containsEvidenceValue(receipt.QueryRange.EntityIDs, binding.EntityReference) &&
					!containsEvidenceValue(receipt.QueryRange.AccountIDs, binding.EntityReference)) {
				return errors.New("canonical evidence V3 accepted slot lineage exceeds receipt scope")
			}
			boundRecords[binding.SourceRecordID] = struct{}{}
		}
		if len(boundRecords) != len(receipt.SourceRecordIDs) {
			return errors.New("canonical evidence V3 accepted slot lineage does not cover the receipt records")
		}
	}
	return nil
}

func EvidenceQueryRangeSupportsClaimPayload(scope EvidenceQueryRange, payload NormalizedClaimPayload) bool {
	entityID := payload.EntityID
	if entityID == "" {
		entityID = payload.SubjectID
	}
	if entityID != "" && (len(scope.EntityIDs) == 0 || !containsEvidenceValue(scope.EntityIDs, entityID)) {
		return false
	}
	if payload.AccountID != "" && (len(scope.AccountIDs) == 0 || !containsEvidenceValue(scope.AccountIDs, payload.AccountID)) {
		return false
	}
	if payload.Direction != "" && (len(scope.Directions) == 0 || !containsEvidenceValue(scope.Directions, payload.Direction)) {
		return false
	}
	if payload.StartAt != "" && (scope.StartAt != payload.StartAt || scope.EndAt != payload.EndAt) {
		return false
	}
	return true
}

func EvidenceReceiptMetadataSupportsClaim(receipt EvidenceReceipt, claimType ClaimType, payload NormalizedClaimPayload) bool {
	if payload.Granularity != "" && strings.TrimSpace(receipt.Granularity) != payload.Granularity {
		return false
	}
	return claimType != ClaimAmount || strings.ToUpper(strings.TrimSpace(receipt.Currency)) == payload.Currency
}

func containsEvidenceValue(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func validateEvidenceReceiptCore(receipt EvidenceReceipt) error {
	if receipt.SchemaVersion != EvidenceReceiptVersion || strings.TrimSpace(receipt.ReceiptID) == "" ||
		strings.TrimSpace(receipt.ThreadID) == "" || strings.TrimSpace(receipt.TurnID) == "" || strings.TrimSpace(receipt.CaseID) == "" ||
		!validSHA256(receipt.CaseBindingHash) || receipt.ContextEpoch == 0 || !validSHA256(receipt.ContextDigest) ||
		!validSHA256(receipt.ExecutionGrantID) || strings.TrimSpace(receipt.ToolCallID) == "" || strings.TrimSpace(receipt.ServerIdentity) == "" ||
		strings.TrimSpace(receipt.ServerVersion) == "" || receipt.ConnectionEpoch == 0 || strings.TrimSpace(receipt.ToolName) == "" ||
		!validSHA256(receipt.ArgsHash) || !validSHA256(receipt.ResultHash) || strings.TrimSpace(receipt.SourceType) == "" ||
		strings.TrimSpace(receipt.DatasetSnapshotID) == "" || !validSHA256(receipt.QueryHash) || strings.TrimSpace(receipt.Granularity) == "" ||
		strings.TrimSpace(receipt.Timezone) == "" || receipt.SourceRecordIDs == nil || !validSHA256(receipt.RawSHA256) ||
		receipt.TransformationLineage == nil || strings.TrimSpace(receipt.IssuedAt) == "" || !validSHA256(receipt.ReceiptDigest) {
		return errors.New("evidence receipt is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.IssuedAt); err != nil {
		return errors.New("evidence receipt issuedAt is invalid")
	}
	serverID, _, toolIdentityOK := domainmcpname.Parse(receipt.ToolName)
	identity, identityErr := domainsecurity.ParseVerifiedMCPServerIdentity(receipt.ServerIdentity)
	if !toolIdentityOK || identityErr != nil || !domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(identity) || identity.ServerID != serverID || identity.ObservedVersion != receipt.ServerVersion ||
		identity.ConnectionEpoch != receipt.ConnectionEpoch {
		return errors.New("evidence receipt server identity is invalid")
	}
	if err := validateEvidenceQueryRange(receipt.QueryRange); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(receipt.SourceType), "transactions") &&
		(strings.TrimSpace(receipt.Currency) == "" || len(receipt.QueryRange.EntityIDs) == 0 || len(receipt.QueryRange.Directions) == 0 ||
			receipt.QueryRange.StartAt == "" || receipt.QueryRange.EndAt == "") {
		return errors.New("transaction evidence receipt scope is incomplete")
	}
	if strings.EqualFold(strings.TrimSpace(receipt.SourceType), SourceTypeTransactionDatasetInventory) &&
		(receipt.Granularity != "dataset_table_rows" || receipt.Currency != "" || len(receipt.QueryRange.EntityIDs) != 1 ||
			len(receipt.QueryRange.AccountIDs) != 0 || len(receipt.QueryRange.Directions) != 0 || receipt.QueryRange.StartAt != "" ||
			receipt.QueryRange.EndAt != "" || receipt.PaginationCompleteness != PaginationComplete) {
		return errors.New("transaction dataset inventory evidence receipt scope is invalid")
	}
	for index, step := range receipt.TransformationLineage {
		if strings.TrimSpace(step.StepID) == "" || strings.TrimSpace(step.Transformer) == "" || strings.TrimSpace(step.TransformerVersion) == "" ||
			!validSHA256(step.InputHash) || !validSHA256(step.OutputHash) {
			return errors.New("evidence receipt transformation lineage is invalid")
		}
		if index > 0 && step.InputHash != receipt.TransformationLineage[index-1].OutputHash {
			return errors.New("evidence receipt transformation lineage chain is invalid")
		}
	}
	switch receipt.PaginationCompleteness {
	case PaginationComplete, PaginationPartial:
	default:
		return errors.New("evidence receipt pagination completeness is invalid")
	}
	switch receipt.PIIClassification {
	case PIINone, PIIMasked, PIIRestricted, PIIControlled:
	default:
		return errors.New("evidence receipt PII classification is invalid")
	}
	if evidenceReceiptDigest(receipt) != receipt.ReceiptDigest {
		return errors.New("evidence receipt integrity is invalid")
	}
	return nil
}

func validateEvidenceQueryRange(scope EvidenceQueryRange) error {
	if scope.EntityIDs == nil || scope.AccountIDs == nil || scope.Directions == nil || scope.SourceIDs == nil || len(scope.SourceIDs) == 0 || !validSHA256(scope.FiltersHash) {
		return errors.New("evidence receipt query range is incomplete")
	}
	if (scope.StartAt == "") != (scope.EndAt == "") {
		return errors.New("evidence receipt query time range is incomplete")
	}
	if scope.StartAt != "" {
		start, startErr := time.Parse(time.RFC3339Nano, scope.StartAt)
		end, endErr := time.Parse(time.RFC3339Nano, scope.EndAt)
		if startErr != nil || endErr != nil || end.Before(start) {
			return errors.New("evidence receipt query time range is invalid")
		}
	}
	return nil
}

func normalizeEvidenceQueryRange(scope EvidenceQueryRange) EvidenceQueryRange {
	scope.EntityIDs = canonicalEvidenceStrings(scope.EntityIDs)
	scope.AccountIDs = canonicalEvidenceStrings(scope.AccountIDs)
	scope.Directions = canonicalEvidenceStrings(scope.Directions)
	scope.SourceIDs = canonicalEvidenceStrings(scope.SourceIDs)
	if scope.EntityIDs == nil {
		scope.EntityIDs = []string{}
	}
	if scope.AccountIDs == nil {
		scope.AccountIDs = []string{}
	}
	if scope.Directions == nil {
		scope.Directions = []string{}
	}
	if scope.SourceIDs == nil {
		scope.SourceIDs = []string{}
	}
	scope.StartAt = strings.TrimSpace(scope.StartAt)
	scope.EndAt = strings.TrimSpace(scope.EndAt)
	scope.FiltersHash = strings.TrimSpace(scope.FiltersHash)
	return scope
}

func canonicalEvidenceStrings(values []string) []string {
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

func cloneTransformationLineage(values []TransformationLineageStep) []TransformationLineageStep {
	if values == nil {
		return nil
	}
	out := append([]TransformationLineageStep(nil), values...)
	for index := range out {
		out[index].StepID = strings.TrimSpace(out[index].StepID)
		out[index].Transformer = strings.TrimSpace(out[index].Transformer)
		out[index].TransformerVersion = strings.TrimSpace(out[index].TransformerVersion)
		out[index].InputHash = strings.TrimSpace(out[index].InputHash)
		out[index].OutputHash = strings.TrimSpace(out[index].OutputHash)
	}
	return out
}

func evidenceReceiptDigest(receipt EvidenceReceipt) string {
	receipt.ReceiptDigest = ""
	receipt.RegistrySequence = 0
	receipt.PreviousRegistryDigest = ""
	receipt.RegistryIntegrityProof = ""
	body, _ := json.Marshal(receipt)
	return domainsecurity.SHA256Hex(body)
}

func evidenceReceiptRegistryProof(receipt EvidenceReceipt) string {
	body, _ := json.Marshal(struct {
		SchemaVersion          int    `json:"schemaVersion"`
		ReceiptID              string `json:"receiptId"`
		ReceiptDigest          string `json:"receiptDigest"`
		RegistrySequence       uint64 `json:"registrySequence"`
		PreviousRegistryDigest string `json:"previousRegistryDigest"`
		ContextDigest          string `json:"contextDigest"`
		DatasetSnapshotID      string `json:"datasetSnapshotId"`
	}{
		SchemaVersion: receipt.SchemaVersion, ReceiptID: receipt.ReceiptID, ReceiptDigest: receipt.ReceiptDigest,
		RegistrySequence: receipt.RegistrySequence, PreviousRegistryDigest: receipt.PreviousRegistryDigest,
		ContextDigest: receipt.ContextDigest, DatasetSnapshotID: receipt.DatasetSnapshotID,
	})
	return domainsecurity.SHA256Hex(body)
}
