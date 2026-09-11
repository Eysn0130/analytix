package job

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ChildCompletionReceiptSchemaVersionV1 = 1
	ChildCompletionReceiptPurposeV1       = "analytix.child-completion-receipt/v1"
	ChildCompletionReceiptAlgorithmV1     = "Ed25519"

	maxChildCompletionReceiptBytes = 256 * 1024
	maxChildCompletionTextBytes    = 32 * 1024
)

var childCompletionReceiptSignatureDomainV1 = []byte("analytix.child-completion-receipt-signature/v1\x00")
var childCompletionReceiptDigestDomainV1 = []byte("analytix.child-completion-receipt-digest/v1\x00")

// ChildCompletionContextRefV1 records the exact authority identity of one
// frozen context. It intentionally does not copy prompts, model output,
// reasoning, evidence payloads, PII projections, or other private material.
type ChildCompletionContextRefV1 struct {
	ContextVersion     int    `json:"contextVersion"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextDigest      string `json:"contextDigest"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	WorkspaceRealPath  string `json:"workspaceRealPath"`
	TenantID           string `json:"tenantId"`
	UserID             string `json:"userId"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
}

// ChildCompletionReceiptV1 proves that a specific durable child run reached a
// host-accepted final under an exact parent/child context pair. The receipt is
// not output, evidence, fact, or parent-goal authority. Production use must
// additionally anchor its self-contained signature to the trusted host key.
type ChildCompletionReceiptV1 struct {
	SchemaVersion               int                               `json:"schemaVersion"`
	Purpose                     string                            `json:"purpose"`
	ChildRunID                  string                            `json:"childRunId"`
	SecurityBindingDigest       string                            `json:"securityBindingDigest"`
	ParentExecutionGrantID      string                            `json:"parentExecutionGrantId"`
	ParentToolCallID            string                            `json:"parentToolCallId"`
	ParentContext               ChildCompletionContextRefV1       `json:"parentContext"`
	ChildContext                ChildCompletionContextRefV1       `json:"childContext"`
	AcceptedFinalDigest         string                            `json:"acceptedFinalDigest"`
	PrivateRecordDigest         string                            `json:"privateRecordDigest"`
	EnvelopeDigest              string                            `json:"envelopeDigest"`
	FinalVariant                domainevidence.FinalAnswerVariant `json:"finalVariant"`
	TerminalReason              string                            `json:"terminalReason"`
	CanContinueParent           bool                              `json:"canContinueParent"`
	OutputWithheld              bool                              `json:"outputWithheld"`
	CanReadOutput               bool                              `json:"canReadOutput"`
	FactAnswerAllowed           bool                              `json:"factAnswerAllowed"`
	EvidenceAuthority           bool                              `json:"evidenceAuthority"`
	ParentGoalCompletionAllowed bool                              `json:"parentGoalCompletionAllowed"`
	IssuedAt                    string                            `json:"issuedAt"`
	AuthorityAlgorithm          string                            `json:"authorityAlgorithm"`
	AuthorityKeyID              string                            `json:"authorityKeyId"`
	AuthorityPublicKey          string                            `json:"authorityPublicKey"`
	AuthoritySignature          string                            `json:"authoritySignature"`
	ReceiptDigest               string                            `json:"receiptDigest"`
}

type ChildCompletionReceiptInputV1 struct {
	ChildRunID         string
	SecurityBinding    *SecurityBinding
	ParentContext      domainsecurity.TurnSecurityContext
	ChildContext       domainsecurity.TurnSecurityContext
	AcceptedFinal      domainevidence.AcceptedFinalRecord
	CanContinueParent  bool
	IssuedAt           time.Time
	AuthorityKeyID     string
	AuthorityPublicKey []byte
}

type ChildCompletionReceiptSignFuncV1 func([]byte) ([]byte, error)

func (receipt *ChildCompletionReceiptV1) UnmarshalJSON(raw []byte) error {
	if receipt == nil {
		return errors.New("child completion receipt target is nil")
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxChildCompletionReceiptBytes, MaxDepth: 8, MaxTokens: 256,
		MaxStringBytes: maxChildCompletionTextBytes,
	}); err != nil {
		return err
	}
	type wireReceipt ChildCompletionReceiptV1
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var decoded wireReceipt
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("child completion receipt contains trailing JSON")
	}
	candidate := ChildCompletionReceiptV1(decoded)
	if err := ValidateChildCompletionReceiptV1(candidate); err != nil {
		return err
	}
	*receipt = candidate
	return nil
}

func NewChildCompletionReceiptV1(input ChildCompletionReceiptInputV1, sign ChildCompletionReceiptSignFuncV1) (ChildCompletionReceiptV1, error) {
	if sign == nil || domainsecurity.ValidateTurnSecurityContextForExecution(input.ParentContext) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(input.ChildContext) != nil ||
		ValidateSecurityBinding(input.SecurityBinding) != nil || !SecurityBindingMatchesContext(input.SecurityBinding, input.ParentContext) ||
		domainevidence.ValidateAcceptedFinalRecord(input.AcceptedFinal) != nil ||
		!acceptedFinalMatchesChildCompletionContext(input.AcceptedFinal, input.ChildContext) {
		return ChildCompletionReceiptV1{}, errors.New("child completion receipt authority input is invalid")
	}
	parentContext, err := NewChildCompletionContextRefV1(input.ParentContext)
	if err != nil {
		return ChildCompletionReceiptV1{}, err
	}
	childContext, err := NewChildCompletionContextRefV1(input.ChildContext)
	if err != nil || !ChildCompletionContextRefsShareCaseScopeV1(parentContext, childContext) || parentContext.ThreadID == childContext.ThreadID {
		return ChildCompletionReceiptV1{}, errors.New("child completion contexts do not share exact case scope")
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	acceptedAt, _ := time.Parse(time.RFC3339Nano, input.AcceptedFinal.AcceptedAt)
	if issuedAt.Before(acceptedAt) {
		return ChildCompletionReceiptV1{}, errors.New("child completion receipt predates accepted final")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	receipt := ChildCompletionReceiptV1{
		SchemaVersion: ChildCompletionReceiptSchemaVersionV1,
		Purpose:       ChildCompletionReceiptPurposeV1,
		ChildRunID:    strings.TrimSpace(input.ChildRunID), SecurityBindingDigest: input.SecurityBinding.BindingDigest,
		ParentExecutionGrantID: input.SecurityBinding.ParentExecutionGrantID, ParentToolCallID: input.SecurityBinding.ParentToolCallID,
		ParentContext: parentContext, ChildContext: childContext,
		AcceptedFinalDigest: input.AcceptedFinal.RecordDigest, PrivateRecordDigest: input.AcceptedFinal.PrivateRecordDigest,
		EnvelopeDigest: input.AcceptedFinal.EnvelopeDigest, FinalVariant: input.AcceptedFinal.Variant,
		TerminalReason: input.AcceptedFinal.TerminalReason, CanContinueParent: input.CanContinueParent,
		OutputWithheld: true, CanReadOutput: false, FactAnswerAllowed: false, EvidenceAuthority: false,
		ParentGoalCompletionAllowed: false, IssuedAt: issuedAt.Format(time.RFC3339Nano),
		AuthorityAlgorithm: ChildCompletionReceiptAlgorithmV1, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if !validChildCompletionRunID(receipt.ChildRunID) || len(publicKey) != ed25519.PublicKeySize ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return ChildCompletionReceiptV1{}, errors.New("child completion receipt signing authority is invalid")
	}
	signature, err := sign(ChildCompletionReceiptV1SigningBytes(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ChildCompletionReceiptV1{}, errors.New("child completion receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.ReceiptDigest = childCompletionReceiptDigestV1(receipt)
	if err := ValidateChildCompletionReceiptForBindingsV1(receipt, input.ParentContext, input.ChildContext, input.SecurityBinding, input.AcceptedFinal); err != nil {
		return ChildCompletionReceiptV1{}, err
	}
	return receipt, nil
}

func NewChildCompletionContextRefV1(context domainsecurity.TurnSecurityContext) (ChildCompletionContextRefV1, error) {
	if err := domainsecurity.ValidateTurnSecurityContext(context); err != nil || context.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return ChildCompletionContextRefV1{}, errors.New("child completion context is invalid")
	}
	reference := ChildCompletionContextRefV1{
		ContextVersion: context.Version, ThreadID: context.ThreadID, TurnID: context.TurnID,
		ContextDigest: context.ContextDigest, ContextEpoch: context.ContextEpoch,
		WorkspaceRealPath: context.WorkspaceRealPath, TenantID: context.TenantID, UserID: context.UserID,
		CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash,
		DatasetSnapshotID: context.DatasetSnapshotID, SourceManifestHash: context.SourceManifestHash,
	}
	return reference, ValidateChildCompletionContextRefV1(reference)
}

func ValidateChildCompletionContextRefV1(reference ChildCompletionContextRefV1) error {
	if reference.ContextVersion != domainsecurity.TurnSecurityContextVersionV2 ||
		!childCompletionCanonicalText(reference.ThreadID) || !childCompletionCanonicalText(reference.TurnID) ||
		!childCompletionCanonicalSHA256(reference.ContextDigest) || reference.ContextEpoch == 0 ||
		!childCompletionCanonicalText(reference.WorkspaceRealPath) || !childCompletionCanonicalText(reference.TenantID) ||
		!childCompletionCanonicalText(reference.UserID) || !childCompletionCanonicalText(reference.CaseID) ||
		reference.CaseID == domainsecurity.UnboundCaseID || !childCompletionCanonicalSHA256(reference.CaseBindingHash) ||
		!childCompletionCanonicalText(reference.DatasetSnapshotID) || reference.DatasetSnapshotID == domainsecurity.NoDatasetSnapshotID ||
		strings.HasPrefix(reference.DatasetSnapshotID, domainsecurity.UnresolvedSnapshotMark) ||
		!childCompletionCanonicalSHA256(reference.SourceManifestHash) {
		return errors.New("child completion context reference is invalid")
	}
	return nil
}

func ChildCompletionContextRefV1MatchesContext(reference ChildCompletionContextRefV1, context domainsecurity.TurnSecurityContext) bool {
	expected, err := NewChildCompletionContextRefV1(context)
	return err == nil && reference == expected
}

// ChildCompletionContextRefsShareCaseScopeV1 compares only shared workspace
// authority. Context epochs are deliberately not compared because they are
// thread-local; each side is instead bound to its own exact epoch and digest.
func ChildCompletionContextRefsShareCaseScopeV1(parent, child ChildCompletionContextRefV1) bool {
	if ValidateChildCompletionContextRefV1(parent) != nil || ValidateChildCompletionContextRefV1(child) != nil {
		return false
	}
	return parent.WorkspaceRealPath == child.WorkspaceRealPath && parent.TenantID == child.TenantID && parent.UserID == child.UserID &&
		parent.CaseID == child.CaseID && parent.CaseBindingHash == child.CaseBindingHash &&
		parent.DatasetSnapshotID == child.DatasetSnapshotID && parent.SourceManifestHash == child.SourceManifestHash
}

func ValidateChildCompletionReceiptV1(receipt ChildCompletionReceiptV1) error {
	if receipt.SchemaVersion != ChildCompletionReceiptSchemaVersionV1 || receipt.Purpose != ChildCompletionReceiptPurposeV1 ||
		!validChildCompletionRunID(receipt.ChildRunID) || !childCompletionCanonicalSHA256(receipt.SecurityBindingDigest) ||
		!childCompletionCanonicalSHA256(receipt.ParentExecutionGrantID) || !childCompletionCanonicalText(receipt.ParentToolCallID) ||
		ValidateChildCompletionContextRefV1(receipt.ParentContext) != nil || ValidateChildCompletionContextRefV1(receipt.ChildContext) != nil ||
		!ChildCompletionContextRefsShareCaseScopeV1(receipt.ParentContext, receipt.ChildContext) ||
		receipt.ParentContext.ThreadID == receipt.ChildContext.ThreadID ||
		!childCompletionCanonicalSHA256(receipt.AcceptedFinalDigest) || !childCompletionCanonicalSHA256(receipt.PrivateRecordDigest) ||
		!childCompletionCanonicalSHA256(receipt.EnvelopeDigest) || !validChildCompletionFinalVariant(receipt.FinalVariant) ||
		!validChildCompletionTerminalReason(receipt.TerminalReason) ||
		(receipt.CanContinueParent && !childCompletionFinalVariantCanContinue(receipt.FinalVariant)) ||
		!receipt.OutputWithheld || receipt.CanReadOutput ||
		receipt.FactAnswerAllowed || receipt.EvidenceAuthority || receipt.ParentGoalCompletionAllowed ||
		receipt.AuthorityAlgorithm != ChildCompletionReceiptAlgorithmV1 || !childCompletionCanonicalSHA256(receipt.AuthorityKeyID) ||
		!childCompletionCanonicalSHA256(receipt.ReceiptDigest) {
		return errors.New("child completion receipt is incomplete")
	}
	encoded, encodedErr := json.Marshal(receipt)
	if encodedErr != nil || len(encoded) > maxChildCompletionReceiptBytes {
		return errors.New("child completion receipt exceeds its canonical bound")
	}
	issuedAt, issuedAtErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if issuedAtErr != nil || issuedAt.IsZero() || issuedAt.UTC().Format(time.RFC3339Nano) != receipt.IssuedAt {
		return errors.New("child completion receipt issuedAt is invalid")
	}
	publicKey, publicKeyErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicKeyErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != receipt.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != receipt.AuthoritySignature ||
		receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ChildCompletionReceiptV1SigningBytes(receipt), signature) {
		return errors.New("child completion receipt signature is invalid")
	}
	if childCompletionReceiptDigestV1(receipt) != receipt.ReceiptDigest {
		return errors.New("child completion receipt digest is invalid")
	}
	return nil
}

// ValidateChildCompletionReceiptForBindingsV1 binds an otherwise authentic
// receipt to the exact live domain records. Trusted-key verification remains
// an application boundary and must precede continuation in production.
func ValidateChildCompletionReceiptForBindingsV1(
	receipt ChildCompletionReceiptV1,
	parentContext domainsecurity.TurnSecurityContext,
	childContext domainsecurity.TurnSecurityContext,
	securityBinding *SecurityBinding,
	acceptedFinal domainevidence.AcceptedFinalRecord,
) error {
	if err := ValidateChildCompletionReceiptV1(receipt); err != nil {
		return err
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(parentContext) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(childContext) != nil ||
		!ChildCompletionContextRefV1MatchesContext(receipt.ParentContext, parentContext) ||
		!ChildCompletionContextRefV1MatchesContext(receipt.ChildContext, childContext) ||
		ValidateSecurityBinding(securityBinding) != nil || !SecurityBindingMatchesContext(securityBinding, parentContext) ||
		receipt.SecurityBindingDigest != securityBinding.BindingDigest ||
		receipt.ParentExecutionGrantID != securityBinding.ParentExecutionGrantID || receipt.ParentToolCallID != securityBinding.ParentToolCallID ||
		domainevidence.ValidateAcceptedFinalRecord(acceptedFinal) != nil || !acceptedFinalMatchesChildCompletionContext(acceptedFinal, childContext) ||
		receipt.AcceptedFinalDigest != acceptedFinal.RecordDigest || receipt.PrivateRecordDigest != acceptedFinal.PrivateRecordDigest ||
		receipt.EnvelopeDigest != acceptedFinal.EnvelopeDigest || receipt.FinalVariant != acceptedFinal.Variant ||
		receipt.TerminalReason != acceptedFinal.TerminalReason {
		return errors.New("child completion receipt binding is invalid")
	}
	acceptedAt, _ := time.Parse(time.RFC3339Nano, acceptedFinal.AcceptedAt)
	issuedAt, _ := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if issuedAt.Before(acceptedAt) {
		return errors.New("child completion receipt predates accepted final")
	}
	return nil
}

func ParseChildCompletionReceiptV1(raw []byte) (ChildCompletionReceiptV1, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxChildCompletionReceiptBytes, MaxDepth: 8, MaxTokens: 256,
		MaxStringBytes: maxChildCompletionTextBytes,
	}); err != nil {
		return ChildCompletionReceiptV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var receipt ChildCompletionReceiptV1
	if err := decoder.Decode(&receipt); err != nil {
		return ChildCompletionReceiptV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ChildCompletionReceiptV1{}, errors.New("child completion receipt contains trailing JSON")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ChildCompletionReceiptV1{}, errors.New("child completion receipt is not canonically encoded")
	}
	return receipt, ValidateChildCompletionReceiptV1(receipt)
}

func ChildCompletionReceiptV1Bytes(receipt ChildCompletionReceiptV1) ([]byte, error) {
	if err := ValidateChildCompletionReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func ChildCompletionReceiptV1SigningBytes(receipt ChildCompletionReceiptV1) []byte {
	receipt.AuthoritySignature = ""
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), childCompletionReceiptSignatureDomainV1...)
	return append(out, digest[:]...)
}

func ChildCompletionReceiptV1AuthorityMaterial(receipt ChildCompletionReceiptV1) (string, []byte, []byte, error) {
	if err := ValidateChildCompletionReceiptV1(receipt); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	return receipt.AuthorityKeyID, publicKey, signature, nil
}

func CloneChildCompletionReceiptV1(receipt *ChildCompletionReceiptV1) *ChildCompletionReceiptV1 {
	if receipt == nil {
		return nil
	}
	clone := *receipt
	return &clone
}

func acceptedFinalMatchesChildCompletionContext(record domainevidence.AcceptedFinalRecord, context domainsecurity.TurnSecurityContext) bool {
	return domainevidence.ValidateAcceptedFinalForCurrentWriteV1(record) == nil &&
		record.ThreadID == context.ThreadID && record.TurnID == context.TurnID && record.ContextDigest == context.ContextDigest &&
		record.ContextEpoch == context.ContextEpoch && record.DatasetSnapshotID == context.DatasetSnapshotID
}

func childCompletionReceiptDigestV1(receipt ChildCompletionReceiptV1) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	payload := append([]byte(nil), childCompletionReceiptDigestDomainV1...)
	return domainsecurity.SHA256Hex(append(payload, body...))
}

func childCompletionCanonicalSHA256(value string) bool {
	return value == strings.ToLower(value) && domainsecurity.IsSHA256Hex(value)
}

func childCompletionCanonicalText(value string) bool {
	if value == "" || len(value) > maxChildCompletionTextBytes || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validChildCompletionFinalVariant(variant domainevidence.FinalAnswerVariant) bool {
	switch variant {
	case domainevidence.EvidenceBackedAnswer, domainevidence.PartialEvidenceAnswer, domainevidence.VerifiedNoHitAnswer,
		domainevidence.SourceUnavailableAnswer, domainevidence.NeedsEvidenceAnswer, domainevidence.GeneralGuidanceAnswer:
		return true
	default:
		return false
	}
}

func validChildCompletionTerminalReason(reason string) bool {
	if !childCompletionCanonicalText(reason) {
		return false
	}
	_, ok := domainevidence.FinalAnswerTerminalStatus(reason)
	return ok
}

func childCompletionFinalVariantCanContinue(variant domainevidence.FinalAnswerVariant) bool {
	switch variant {
	case domainevidence.SourceUnavailableAnswer, domainevidence.NeedsEvidenceAnswer, domainevidence.GeneralGuidanceAnswer:
		return true
	default:
		return false
	}
}

func validChildCompletionRunID(value string) bool {
	if !childCompletionCanonicalText(value) || !strings.HasPrefix(value, "job-") {
		return false
	}
	sequence, err := strconv.Atoi(strings.TrimPrefix(value, "job-"))
	return err == nil && sequence > 0 && value == fmt.Sprintf("job-%d", sequence)
}
