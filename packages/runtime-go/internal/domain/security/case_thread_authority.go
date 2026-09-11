package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	CaseThreadAuthorityRecordVersion = 1
	CaseThreadAuthorityPurpose       = "analytix.case-thread-authority/v1"
	CaseThreadAuthorityAlgorithm     = "Ed25519"
)

var caseThreadAuthoritySignatureDomain = []byte("analytix.case-thread-authority/v1\x00")

type CaseThreadAuthorityRecord struct {
	SchemaVersion          int                              `json:"schemaVersion"`
	AuthorityPurpose       string                           `json:"authorityPurpose"`
	AuthorityAlgorithm     string                           `json:"authorityAlgorithm"`
	AuthorityKeyID         string                           `json:"authorityKeyId"`
	AuthorityPublicKey     string                           `json:"authorityPublicKey"`
	SecurityContext        *TurnSecurityContext             `json:"securityContext,omitempty"`
	CommittedTurnState     *CommittedTurnState              `json:"committedTurnState,omitempty"`
	ThreadID               string                           `json:"threadId,omitempty"`
	ParentThreadID         string                           `json:"parentThreadId,omitempty"`
	ParentRecordDigest     string                           `json:"parentRecordDigest,omitempty"`
	Derivation             string                           `json:"derivation,omitempty"`
	ActiveInheritedHistory *ActiveInheritedHistoryBindingV1 `json:"activeInheritedHistory,omitempty"`
	AuthoritySignature     string                           `json:"authoritySignature"`
	RecordDigest           string                           `json:"recordDigest"`
}

type CommittedTurnState struct {
	ContextEpochState domaincontextepoch.State `json:"contextEpochState"`
	CommittedAt       string                   `json:"committedAt"`
}

func NewCaseThreadAuthorityRecord(context TurnSecurityContext, keyID string, publicKey []byte, sign func([]byte) ([]byte, error)) (CaseThreadAuthorityRecord, error) {
	keyID = strings.TrimSpace(keyID)
	publicKey = append([]byte(nil), publicKey...)
	if sign == nil || context.Version != TurnSecurityContextVersionV2 || !TurnSecurityContextRequiresFinalEvidenceGate(context) ||
		len(publicKey) != ed25519.PublicKeySize || SHA256Hex(publicKey) != keyID {
		return CaseThreadAuthorityRecord{}, errors.New("case thread authority input is invalid")
	}
	record := CaseThreadAuthorityRecord{
		SchemaVersion: CaseThreadAuthorityRecordVersion, AuthorityPurpose: CaseThreadAuthorityPurpose,
		AuthorityAlgorithm: CaseThreadAuthorityAlgorithm, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), SecurityContext: &context,
	}
	return signCaseThreadAuthorityRecord(record, sign)
}

func NewCommittedTurnContextAuthorityRecord(context TurnSecurityContext, state domaincontextepoch.State, committedAt time.Time, keyID string, publicKey []byte, sign func([]byte) ([]byte, error)) (CaseThreadAuthorityRecord, error) {
	keyID = strings.TrimSpace(keyID)
	publicKey = append([]byte(nil), publicKey...)
	if committedAt.IsZero() {
		committedAt = time.Now().UTC()
	}
	clonedState, stateErr := domaincontextepoch.ParseState(state)
	if sign == nil || stateErr != nil || context.Version != TurnSecurityContextVersionV2 || !TurnSecurityContextRequiresFinalEvidenceGate(context) ||
		clonedState.ThreadID != context.ThreadID || clonedState.AcceptedSnapshot.Epoch != context.ContextEpoch ||
		len(publicKey) != ed25519.PublicKeySize || SHA256Hex(publicKey) != keyID {
		return CaseThreadAuthorityRecord{}, errors.New("committed turn context authority input is invalid")
	}
	record := CaseThreadAuthorityRecord{
		SchemaVersion: CaseThreadAuthorityRecordVersion, AuthorityPurpose: CaseThreadAuthorityPurpose,
		AuthorityAlgorithm: CaseThreadAuthorityAlgorithm, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), SecurityContext: &context,
		CommittedTurnState: &CommittedTurnState{ContextEpochState: clonedState, CommittedAt: committedAt.UTC().Format(time.RFC3339Nano)},
	}
	return signCaseThreadAuthorityRecord(record, sign)
}

func NewCaseThreadLineageAuthorityRecord(threadID, parentThreadID, parentRecordDigest, derivation, keyID string, publicKey []byte, sign func([]byte) ([]byte, error)) (CaseThreadAuthorityRecord, error) {
	return newCaseThreadLineageAuthorityRecord(threadID, parentThreadID, parentRecordDigest, derivation, nil, keyID, publicKey, sign)
}

func NewCaseThreadLineageAuthorityRecordWithInheritedHistoryV1(threadID, parentThreadID, parentRecordDigest, derivation string, binding ActiveInheritedHistoryBindingV1, keyID string, publicKey []byte, sign func([]byte) ([]byte, error)) (CaseThreadAuthorityRecord, error) {
	if ValidateActiveInheritedHistoryBindingV1(binding) != nil || binding.SourceThreadID != parentThreadID ||
		binding.TargetThreadID != threadID || binding.SourceAuthorityRecordDigest != parentRecordDigest || binding.Derivation != derivation {
		return CaseThreadAuthorityRecord{}, errors.New("case thread inherited history authority input is invalid")
	}
	return newCaseThreadLineageAuthorityRecord(threadID, parentThreadID, parentRecordDigest, derivation, &binding, keyID, publicKey, sign)
}

func newCaseThreadLineageAuthorityRecord(threadID, parentThreadID, parentRecordDigest, derivation string, binding *ActiveInheritedHistoryBindingV1, keyID string, publicKey []byte, sign func([]byte) ([]byte, error)) (CaseThreadAuthorityRecord, error) {
	threadID = strings.TrimSpace(threadID)
	parentThreadID = strings.TrimSpace(parentThreadID)
	parentRecordDigest = strings.TrimSpace(parentRecordDigest)
	derivation = strings.TrimSpace(derivation)
	keyID = strings.TrimSpace(keyID)
	publicKey = append([]byte(nil), publicKey...)
	if sign == nil || threadID == "" || parentThreadID == "" || threadID == parentThreadID ||
		!IsSHA256Hex(parentRecordDigest) || (derivation != "fork" && derivation != "resume") ||
		len(publicKey) != ed25519.PublicKeySize || SHA256Hex(publicKey) != keyID {
		return CaseThreadAuthorityRecord{}, errors.New("case thread lineage authority input is invalid")
	}
	record := CaseThreadAuthorityRecord{
		SchemaVersion: CaseThreadAuthorityRecordVersion, AuthorityPurpose: CaseThreadAuthorityPurpose,
		AuthorityAlgorithm: CaseThreadAuthorityAlgorithm, AuthorityKeyID: keyID,
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), ThreadID: threadID,
		ParentThreadID: parentThreadID, ParentRecordDigest: parentRecordDigest, Derivation: derivation,
		ActiveInheritedHistory: CloneActiveInheritedHistoryBindingV1(binding),
	}
	return signCaseThreadAuthorityRecord(record, sign)
}

func signCaseThreadAuthorityRecord(record CaseThreadAuthorityRecord, sign func([]byte) ([]byte, error)) (CaseThreadAuthorityRecord, error) {
	signature, err := sign(CaseThreadAuthoritySigningBytes(record))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return CaseThreadAuthorityRecord{}, errors.New("case thread authority signing failed")
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	record.RecordDigest = caseThreadAuthorityRecordDigest(record)
	if err := ValidateCaseThreadAuthorityRecord(record); err != nil {
		return CaseThreadAuthorityRecord{}, err
	}
	return record, nil
}

func ValidateCaseThreadAuthorityRecord(record CaseThreadAuthorityRecord) error {
	contextRecord := record.SecurityContext != nil
	lineageRecord := strings.TrimSpace(record.ThreadID) != "" || strings.TrimSpace(record.ParentThreadID) != "" ||
		strings.TrimSpace(record.ParentRecordDigest) != "" || strings.TrimSpace(record.Derivation) != ""
	if record.SchemaVersion != CaseThreadAuthorityRecordVersion || record.AuthorityPurpose != CaseThreadAuthorityPurpose ||
		record.AuthorityAlgorithm != CaseThreadAuthorityAlgorithm || !IsSHA256Hex(record.AuthorityKeyID) ||
		!IsSHA256Hex(record.RecordDigest) || contextRecord == lineageRecord || (!contextRecord && record.CommittedTurnState != nil) {
		return errors.New("case thread authority record is incomplete")
	}
	if contextRecord {
		if record.ActiveInheritedHistory != nil {
			return errors.New("case thread context cannot carry inherited history authority")
		}
		if !caseThreadAuthorityContextIsAuditable(*record.SecurityContext) {
			return errors.New("case thread authority context is invalid")
		}
		if record.CommittedTurnState != nil {
			state := record.CommittedTurnState.ContextEpochState
			committedAt, err := time.Parse(time.RFC3339Nano, record.CommittedTurnState.CommittedAt)
			if err != nil || committedAt.UTC().Format(time.RFC3339Nano) != record.CommittedTurnState.CommittedAt ||
				domaincontextepoch.ValidateState(state) != nil || state.ThreadID != record.SecurityContext.ThreadID ||
				state.AcceptedSnapshot.Epoch != record.SecurityContext.ContextEpoch {
				return errors.New("committed turn context authority state is invalid")
			}
		}
	} else if strings.TrimSpace(record.ThreadID) == "" || strings.TrimSpace(record.ParentThreadID) == "" ||
		strings.TrimSpace(record.ThreadID) == strings.TrimSpace(record.ParentThreadID) || !IsSHA256Hex(record.ParentRecordDigest) ||
		(record.Derivation != "fork" && record.Derivation != "resume") {
		return errors.New("case thread authority lineage is invalid")
	}
	if binding := record.ActiveInheritedHistory; binding != nil {
		if ValidateActiveInheritedHistoryBindingV1(*binding) != nil || binding.SourceThreadID != record.ParentThreadID ||
			binding.TargetThreadID != record.ThreadID || binding.SourceAuthorityRecordDigest != record.ParentRecordDigest || binding.Derivation != record.Derivation {
			return errors.New("case thread inherited history authority binding is invalid")
		}
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize || SHA256Hex(publicKey) != record.AuthorityKeyID {
		return errors.New("case thread authority public key is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	if err != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), CaseThreadAuthoritySigningBytes(record), signature) {
		return errors.New("case thread authority signature is invalid")
	}
	if caseThreadAuthorityRecordDigest(record) != record.RecordDigest {
		return errors.New("case thread authority integrity is invalid")
	}
	return nil
}

func caseThreadAuthorityContextIsAuditable(context TurnSecurityContext) bool {
	if ValidateTurnSecurityContext(context) != nil {
		return false
	}
	if context.Version == TurnSecurityContextVersionV1 {
		return TurnSecurityContextIsCaseSensitive(context)
	}
	return TurnSecurityContextRequiresFinalEvidenceGate(context)
}

func CaseThreadAuthorityThreadID(record CaseThreadAuthorityRecord) string {
	if record.SecurityContext != nil {
		return strings.TrimSpace(record.SecurityContext.ThreadID)
	}
	return strings.TrimSpace(record.ThreadID)
}

func CaseThreadAuthorityIsLineage(record CaseThreadAuthorityRecord) bool {
	return record.SecurityContext == nil
}

func CaseThreadAuthorityIsCommittedContext(record CaseThreadAuthorityRecord) bool {
	return record.SecurityContext != nil && record.CommittedTurnState != nil
}

// CaseThreadAuthorityCanAuthorizeExecution separates legacy audit parsing from
// current execution authority. V1 context records remain inspectable during
// migration but cannot authorize a provider, tool, receipt, or publication.
func CaseThreadAuthorityCanAuthorizeExecution(record CaseThreadAuthorityRecord) bool {
	return ValidateCaseThreadAuthorityRecord(record) == nil && record.SecurityContext != nil &&
		ValidateTurnSecurityContextForExecution(*record.SecurityContext) == nil &&
		TurnSecurityContextAllowsCaseEvidence(*record.SecurityContext)
}

func ParseCaseThreadAuthorityRecord(raw []byte) (CaseThreadAuthorityRecord, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{RequireObject: true}); err != nil {
		return CaseThreadAuthorityRecord{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record CaseThreadAuthorityRecord
	if err := decoder.Decode(&record); err != nil {
		return CaseThreadAuthorityRecord{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CaseThreadAuthorityRecord{}, errors.New("case thread authority contains trailing JSON")
	}
	if err := ValidateCaseThreadAuthorityRecord(record); err != nil {
		return CaseThreadAuthorityRecord{}, err
	}
	return record, nil
}

func CaseThreadAuthorityRecordBytes(record CaseThreadAuthorityRecord) ([]byte, error) {
	if err := ValidateCaseThreadAuthorityRecord(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func CaseThreadAuthoritySigningBytes(record CaseThreadAuthorityRecord) []byte {
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	out := make([]byte, 0, len(caseThreadAuthoritySignatureDomain)+len(digest))
	out = append(out, caseThreadAuthoritySignatureDomain...)
	return append(out, digest[:]...)
}

func CaseThreadAuthorityMaterial(record CaseThreadAuthorityRecord) (string, []byte, []byte, error) {
	if err := ValidateCaseThreadAuthorityRecord(record); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	return record.AuthorityKeyID, publicKey, signature, nil
}

func caseThreadAuthorityRecordDigest(record CaseThreadAuthorityRecord) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return SHA256Hex(body)
}
