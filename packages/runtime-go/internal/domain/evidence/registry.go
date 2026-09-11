package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const EvidenceReceiptRegistryVersion = 2

type EvidenceRegistryOperation string

const (
	EvidenceRegistryIssue  EvidenceRegistryOperation = "issue"
	EvidenceRegistryRevoke EvidenceRegistryOperation = "revoke"
)

type EvidenceReceiptRegistryEntry struct {
	SchemaVersion          int                       `json:"schemaVersion"`
	Sequence               uint64                    `json:"sequence"`
	ThreadID               string                    `json:"threadId"`
	TurnID                 string                    `json:"turnId"`
	ContextDigest          string                    `json:"contextDigest"`
	DatasetSnapshotID      string                    `json:"datasetSnapshotId"`
	Operation              EvidenceRegistryOperation `json:"operation"`
	ReceiptID              string                    `json:"receiptId"`
	SettlementID           string                    `json:"settlementId,omitempty"`
	PreparedRecordDigest   string                    `json:"preparedRecordDigest,omitempty"`
	Receipt                *EvidenceReceipt          `json:"receipt,omitempty"`
	CanonicalEvidence      json.RawMessage           `json:"canonicalEvidence,omitempty"`
	ReasonCode             string                    `json:"reasonCode,omitempty"`
	RegisteredAt           string                    `json:"registeredAt"`
	PreviousRegistryDigest string                    `json:"previousRegistryDigest"`
	EntryDigest            string                    `json:"entryDigest"`
}

type EvidenceReceiptRegistry struct {
	SchemaVersion     int                            `json:"schemaVersion"`
	ThreadID          string                         `json:"threadId"`
	TurnID            string                         `json:"turnId"`
	CaseID            string                         `json:"caseId"`
	CaseBindingHash   string                         `json:"caseBindingHash"`
	ContextEpoch      uint64                         `json:"contextEpoch"`
	ContextDigest     string                         `json:"contextDigest"`
	DatasetSnapshotID string                         `json:"datasetSnapshotId"`
	Sequence          uint64                         `json:"sequence"`
	Entries           []EvidenceReceiptRegistryEntry `json:"entries"`
	StateDigest       string                         `json:"stateDigest"`
}

type RegisteredEvidence struct {
	Receipt           EvidenceReceipt `json:"receipt"`
	CanonicalEvidence json.RawMessage `json:"canonicalEvidence"`
	Revoked           bool            `json:"revoked"`
}

func NewEvidenceReceiptRegistry(context domainsecurity.TurnSecurityContext) (EvidenceReceiptRegistry, error) {
	if err := domainsecurity.ValidateTurnSecurityContext(context); err != nil {
		return EvidenceReceiptRegistry{}, errors.New("evidence registry context is invalid")
	}
	registry := EvidenceReceiptRegistry{
		SchemaVersion: EvidenceReceiptRegistryVersion, ThreadID: context.ThreadID, TurnID: context.TurnID,
		CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash, ContextEpoch: context.ContextEpoch,
		ContextDigest: context.ContextDigest, DatasetSnapshotID: context.DatasetSnapshotID, Entries: []EvidenceReceiptRegistryEntry{},
	}
	registry.StateDigest = evidenceRegistryStateDigest(registry)
	return registry, ValidateEvidenceReceiptRegistry(registry)
}

func ParseEvidenceReceiptRegistry(value any) (EvidenceReceiptRegistry, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return EvidenceReceiptRegistry{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var registry EvidenceReceiptRegistry
	if err := decoder.Decode(&registry); err != nil {
		return EvidenceReceiptRegistry{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EvidenceReceiptRegistry{}, errors.New("evidence registry contains trailing JSON")
	}
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return EvidenceReceiptRegistry{}, err
	}
	return registry, nil
}

func ParseEvidenceReceiptRegistryEntry(raw []byte) (EvidenceReceiptRegistryEntry, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var entry EvidenceReceiptRegistryEntry
	if err := decoder.Decode(&entry); err != nil {
		return EvidenceReceiptRegistryEntry{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EvidenceReceiptRegistryEntry{}, errors.New("evidence registry entry contains trailing JSON")
	}
	return entry, nil
}

func ValidateEvidenceReceiptRegistry(registry EvidenceReceiptRegistry) error {
	if registry.SchemaVersion != EvidenceReceiptRegistryVersion || strings.TrimSpace(registry.ThreadID) == "" ||
		strings.TrimSpace(registry.TurnID) == "" || strings.TrimSpace(registry.CaseID) == "" || !validSHA256(registry.CaseBindingHash) ||
		registry.ContextEpoch == 0 || !validSHA256(registry.ContextDigest) || strings.TrimSpace(registry.DatasetSnapshotID) == "" ||
		registry.Entries == nil || !validSHA256(registry.StateDigest) {
		return errors.New("evidence registry identity is invalid")
	}
	current := emptyEvidenceRegistryLike(registry)
	for _, entry := range registry.Entries {
		if err := validateEvidenceRegistryEntryForState(current, entry); err != nil {
			return err
		}
		current = appendEvidenceRegistryEntryUnchecked(current, entry)
	}
	if registry.Sequence != uint64(len(registry.Entries)) || current.Sequence != registry.Sequence || current.StateDigest != registry.StateDigest {
		return errors.New("evidence registry sequence or integrity is invalid")
	}
	return nil
}

func RegisterEvidenceReceipt(registry EvidenceReceiptRegistry, draft EvidenceReceipt, canonicalEvidence json.RawMessage, proof EvidenceSettlementProof, at time.Time) (EvidenceReceiptRegistry, EvidenceReceipt, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, err
	}
	if err := ValidateEvidenceReceiptDraft(draft); err != nil || !receiptMatchesRegistry(draft, registry) {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, errors.New("evidence receipt authority does not match registry")
	}
	if err := ValidateEvidenceSettlementProof(proof, draft.ReceiptID); err != nil {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, err
	}
	if _, found := evidenceReceiptByID(registry, draft.ReceiptID); found {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, errors.New("evidence receipt id is already registered")
	}
	material, err := CanonicalEvidenceBytes(canonicalEvidence)
	if err != nil || domainsecurity.CanonicalJSONHash(material) != draft.ResultHash {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, errors.New("evidence receipt material hash is invalid")
	}
	if err := ValidateCanonicalEvidenceAgainstReceipt(draft, material); err != nil {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, err
	}
	sealed := draft
	sealed.RegistrySequence = registry.Sequence + 1
	sealed.PreviousRegistryDigest = registry.StateDigest
	sealed.RegistryIntegrityProof = evidenceReceiptRegistryProof(sealed)
	if err := ValidateEvidenceReceipt(sealed); err != nil {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, err
	}
	stamp := evidenceRegistryTime(at)
	entry := EvidenceReceiptRegistryEntry{
		SchemaVersion: EvidenceReceiptRegistryVersion, Sequence: sealed.RegistrySequence,
		ThreadID: registry.ThreadID, TurnID: registry.TurnID, ContextDigest: registry.ContextDigest,
		DatasetSnapshotID: registry.DatasetSnapshotID, Operation: EvidenceRegistryIssue, ReceiptID: sealed.ReceiptID,
		SettlementID: proof.SettlementID, PreparedRecordDigest: proof.PreparedRecordDigest,
		Receipt: &sealed, CanonicalEvidence: append(json.RawMessage(nil), material...), RegisteredAt: stamp,
		PreviousRegistryDigest: registry.StateDigest,
	}
	entry.EntryDigest = evidenceRegistryEntryDigest(entry)
	next, err := ApplyEvidenceRegistryEntry(registry, entry)
	if err != nil {
		return EvidenceReceiptRegistry{}, EvidenceReceipt{}, err
	}
	return next, sealed, nil
}

func RevokeEvidenceReceipt(registry EvidenceReceiptRegistry, receiptID string, reasonCode string, at time.Time) (EvidenceReceiptRegistry, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return EvidenceReceiptRegistry{}, err
	}
	receiptID = strings.TrimSpace(receiptID)
	registered, found := evidenceReceiptByID(registry, receiptID)
	if !found || registered.Revoked || strings.TrimSpace(reasonCode) == "" {
		return EvidenceReceiptRegistry{}, errors.New("evidence receipt is not revocable")
	}
	entry := EvidenceReceiptRegistryEntry{
		SchemaVersion: EvidenceReceiptRegistryVersion, Sequence: registry.Sequence + 1,
		ThreadID: registry.ThreadID, TurnID: registry.TurnID, ContextDigest: registry.ContextDigest,
		DatasetSnapshotID: registry.DatasetSnapshotID, Operation: EvidenceRegistryRevoke, ReceiptID: receiptID,
		ReasonCode: strings.TrimSpace(reasonCode), RegisteredAt: evidenceRegistryTime(at), PreviousRegistryDigest: registry.StateDigest,
	}
	entry.EntryDigest = evidenceRegistryEntryDigest(entry)
	return ApplyEvidenceRegistryEntry(registry, entry)
}

func MatchEvidenceReceiptRegistration(registry EvidenceReceiptRegistry, draft EvidenceReceipt, canonicalEvidence json.RawMessage, proof EvidenceSettlementProof) (EvidenceReceipt, bool, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return EvidenceReceipt{}, false, err
	}
	if err := ValidateEvidenceReceiptDraft(draft); err != nil || !receiptMatchesRegistry(draft, registry) {
		return EvidenceReceipt{}, false, errors.New("evidence receipt retry authority does not match registry")
	}
	if err := ValidateEvidenceSettlementProof(proof, draft.ReceiptID); err != nil {
		return EvidenceReceipt{}, false, err
	}
	material, err := CanonicalEvidenceBytes(canonicalEvidence)
	if err != nil || domainsecurity.CanonicalJSONHash(material) != draft.ResultHash {
		return EvidenceReceipt{}, false, errors.New("evidence receipt retry material hash is invalid")
	}
	registered, found := evidenceReceiptByID(registry, draft.ReceiptID)
	if !found {
		return EvidenceReceipt{}, false, nil
	}
	if registered.Revoked {
		return EvidenceReceipt{}, false, errors.New("evidence receipt retry targets a revoked receipt")
	}
	var issue EvidenceReceiptRegistryEntry
	for _, entry := range registry.Entries {
		if entry.Operation == EvidenceRegistryIssue && entry.ReceiptID == draft.ReceiptID {
			issue = entry
			break
		}
	}
	if issue.Receipt == nil || issue.SettlementID != proof.SettlementID || issue.PreparedRecordDigest != proof.PreparedRecordDigest ||
		!bytes.Equal(issue.CanonicalEvidence, material) {
		return EvidenceReceipt{}, false, errors.New("evidence receipt retry conflicts with registered evidence")
	}
	expected := draft
	expected.RegistrySequence = issue.Sequence
	expected.PreviousRegistryDigest = issue.PreviousRegistryDigest
	expected.RegistryIntegrityProof = evidenceReceiptRegistryProof(expected)
	expectedBody, _ := json.Marshal(expected)
	registeredBody, _ := json.Marshal(issue.Receipt)
	if ValidateEvidenceReceipt(expected) != nil || !bytes.Equal(expectedBody, registeredBody) {
		return EvidenceReceipt{}, false, errors.New("evidence receipt retry conflicts with registered receipt authority")
	}
	return *issue.Receipt, true, nil
}

func MatchEvidenceReceiptRevocation(registry EvidenceReceiptRegistry, receiptID, reasonCode string) (bool, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return false, err
	}
	receiptID = strings.TrimSpace(receiptID)
	reasonCode = strings.TrimSpace(reasonCode)
	if receiptID == "" || reasonCode == "" {
		return false, errors.New("evidence receipt revocation retry is invalid")
	}
	for _, entry := range registry.Entries {
		if entry.Operation != EvidenceRegistryRevoke || entry.ReceiptID != receiptID {
			continue
		}
		if entry.ReasonCode != reasonCode {
			return false, errors.New("evidence receipt revocation retry conflicts with the registered reason")
		}
		return true, nil
	}
	return false, nil
}

func ApplyEvidenceRegistryEntry(registry EvidenceReceiptRegistry, entry EvidenceReceiptRegistryEntry) (EvidenceReceiptRegistry, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil {
		return EvidenceReceiptRegistry{}, err
	}
	if err := validateEvidenceRegistryEntryForState(registry, entry); err != nil {
		return EvidenceReceiptRegistry{}, err
	}
	next := appendEvidenceRegistryEntryUnchecked(registry, entry)
	return next, ValidateEvidenceReceiptRegistry(next)
}

func VerifyEvidenceReceiptMembership(registry EvidenceReceiptRegistry, context domainsecurity.TurnSecurityContext, receiptID string) (RegisteredEvidence, error) {
	if err := ValidateEvidenceReceiptRegistry(registry); err != nil || domainsecurity.ValidateTurnSecurityContext(context) != nil ||
		!evidenceRegistryMatchesContext(registry, context) {
		return RegisteredEvidence{}, errors.New("evidence registry context is invalid")
	}
	registered, found := evidenceReceiptByID(registry, strings.TrimSpace(receiptID))
	if !found || registered.Revoked || !receiptMatchesRegistry(registered.Receipt, registry) || ValidateEvidenceReceipt(registered.Receipt) != nil {
		return RegisteredEvidence{}, errors.New("evidence receipt registry membership is invalid")
	}
	registered.CanonicalEvidence = append(json.RawMessage(nil), registered.CanonicalEvidence...)
	return registered, nil
}

func EvidenceReceiptRegistryRecord(registry EvidenceReceiptRegistry) map[string]any {
	body, _ := json.Marshal(registry)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func EvidenceReceiptRegistryEntryRecord(entry EvidenceReceiptRegistryEntry) map[string]any {
	body, _ := json.Marshal(entry)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func EvidenceReceiptRegistryMatchesContext(registry EvidenceReceiptRegistry, context domainsecurity.TurnSecurityContext) bool {
	return ValidateEvidenceReceiptRegistry(registry) == nil && domainsecurity.ValidateTurnSecurityContext(context) == nil &&
		evidenceRegistryMatchesContext(registry, context)
}

func validateEvidenceRegistryEntryForState(registry EvidenceReceiptRegistry, entry EvidenceReceiptRegistryEntry) error {
	if entry.SchemaVersion != EvidenceReceiptRegistryVersion || entry.Sequence != registry.Sequence+1 ||
		entry.ThreadID != registry.ThreadID || entry.TurnID != registry.TurnID || entry.ContextDigest != registry.ContextDigest ||
		entry.DatasetSnapshotID != registry.DatasetSnapshotID || strings.TrimSpace(entry.ReceiptID) == "" ||
		entry.PreviousRegistryDigest != registry.StateDigest || strings.TrimSpace(entry.RegisteredAt) == "" || !validSHA256(entry.EntryDigest) {
		return errors.New("evidence registry entry identity or sequence is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, entry.RegisteredAt); err != nil {
		return errors.New("evidence registry entry time is invalid")
	}
	if evidenceRegistryEntryDigest(entry) != entry.EntryDigest {
		return errors.New("evidence registry entry integrity is invalid")
	}
	switch entry.Operation {
	case EvidenceRegistryIssue:
		if entry.Receipt == nil || strings.TrimSpace(entry.ReasonCode) != "" || len(entry.CanonicalEvidence) == 0 ||
			ValidateEvidenceSettlementProof(EvidenceSettlementProof{SettlementID: entry.SettlementID, PreparedRecordDigest: entry.PreparedRecordDigest}, entry.ReceiptID) != nil ||
			entry.ReceiptID != entry.Receipt.ReceiptID || entry.Receipt.RegistrySequence != entry.Sequence ||
			entry.Receipt.PreviousRegistryDigest != entry.PreviousRegistryDigest || !receiptMatchesRegistry(*entry.Receipt, registry) ||
			ValidateEvidenceReceipt(*entry.Receipt) != nil {
			return errors.New("evidence registry issue entry is invalid")
		}
		material, err := CanonicalEvidenceBytes(entry.CanonicalEvidence)
		if err != nil || !bytes.Equal(material, entry.CanonicalEvidence) || domainsecurity.CanonicalJSONHash(material) != entry.Receipt.ResultHash {
			return errors.New("evidence registry canonical material is invalid")
		}
		if err := ValidateCanonicalEvidenceAgainstReceipt(*entry.Receipt, material); err != nil {
			return err
		}
		if _, found := evidenceReceiptByID(registry, entry.ReceiptID); found {
			return errors.New("evidence registry duplicate receipt is invalid")
		}
	case EvidenceRegistryRevoke:
		registered, found := evidenceReceiptByID(registry, entry.ReceiptID)
		if entry.Receipt != nil || len(entry.CanonicalEvidence) != 0 || entry.SettlementID != "" || entry.PreparedRecordDigest != "" ||
			strings.TrimSpace(entry.ReasonCode) == "" || !found || registered.Revoked {
			return errors.New("evidence registry revocation entry is invalid")
		}
	default:
		return errors.New("evidence registry operation is invalid")
	}
	return nil
}

func evidenceReceiptByID(registry EvidenceReceiptRegistry, receiptID string) (RegisteredEvidence, bool) {
	receiptID = strings.TrimSpace(receiptID)
	var registered RegisteredEvidence
	found := false
	for _, entry := range registry.Entries {
		if entry.ReceiptID != receiptID {
			continue
		}
		switch entry.Operation {
		case EvidenceRegistryIssue:
			if entry.Receipt == nil || found {
				return RegisteredEvidence{}, false
			}
			registered = RegisteredEvidence{Receipt: *entry.Receipt, CanonicalEvidence: append(json.RawMessage(nil), entry.CanonicalEvidence...)}
			found = true
		case EvidenceRegistryRevoke:
			if !found || registered.Revoked {
				return RegisteredEvidence{}, false
			}
			registered.Revoked = true
		}
	}
	return registered, found
}

func appendEvidenceRegistryEntryUnchecked(registry EvidenceReceiptRegistry, entry EvidenceReceiptRegistryEntry) EvidenceReceiptRegistry {
	registry.Entries = append(append([]EvidenceReceiptRegistryEntry(nil), registry.Entries...), cloneEvidenceRegistryEntry(entry))
	registry.Sequence = entry.Sequence
	registry.StateDigest = evidenceRegistryStateDigest(registry)
	return registry
}

func emptyEvidenceRegistryLike(registry EvidenceReceiptRegistry) EvidenceReceiptRegistry {
	empty := registry
	empty.Sequence = 0
	empty.Entries = []EvidenceReceiptRegistryEntry{}
	empty.StateDigest = evidenceRegistryStateDigest(empty)
	return empty
}

func evidenceRegistryMatchesContext(registry EvidenceReceiptRegistry, context domainsecurity.TurnSecurityContext) bool {
	return registry.ThreadID == context.ThreadID && registry.TurnID == context.TurnID && registry.CaseID == context.CaseID &&
		registry.CaseBindingHash == context.CaseBindingHash && registry.ContextEpoch == context.ContextEpoch &&
		registry.ContextDigest == context.ContextDigest && registry.DatasetSnapshotID == context.DatasetSnapshotID
}

func receiptMatchesRegistry(receipt EvidenceReceipt, registry EvidenceReceiptRegistry) bool {
	return receipt.ThreadID == registry.ThreadID && receipt.TurnID == registry.TurnID && receipt.CaseID == registry.CaseID &&
		receipt.CaseBindingHash == registry.CaseBindingHash && receipt.ContextEpoch == registry.ContextEpoch &&
		receipt.ContextDigest == registry.ContextDigest && receipt.DatasetSnapshotID == registry.DatasetSnapshotID
}

func cloneEvidenceRegistryEntry(entry EvidenceReceiptRegistryEntry) EvidenceReceiptRegistryEntry {
	entry.CanonicalEvidence = append(json.RawMessage(nil), entry.CanonicalEvidence...)
	if entry.Receipt != nil {
		receipt := *entry.Receipt
		entry.Receipt = &receipt
	}
	return entry
}

func evidenceRegistryEntryDigest(entry EvidenceReceiptRegistryEntry) string {
	entry.EntryDigest = ""
	body, _ := json.Marshal(entry)
	return domainsecurity.SHA256Hex(body)
}

func evidenceRegistryStateDigest(registry EvidenceReceiptRegistry) string {
	registry.StateDigest = ""
	body, _ := json.Marshal(registry)
	return domainsecurity.SHA256Hex(body)
}

func evidenceRegistryTime(at time.Time) string {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return at.UTC().Format(time.RFC3339Nano)
}
