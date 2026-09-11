package pendingwork

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SchemaVersion = 1

	ReceiptPurpose     = "analytix.pending-work-receipt/v1"
	DispositionPurpose = "analytix.pending-work-disposition/v1"
	AuthorityAlgorithm = "Ed25519"

	KindToolBatch            = "tool_batch"
	KindApprovedToolDispatch = "approved_tool_dispatch"
	KindSideEffectIntent     = "side_effect_intent"
	KindProviderContinuation = "provider_continuation"
	KindReportStage          = "report_stage"

	StatusCompleted      = "completed"
	StatusCancelled      = "cancelled"
	StatusFailed         = "failed"
	StatusExpired        = "expired"
	StatusRejected       = "rejected"
	StatusRestartInvalid = "restart_invalid"
	StatusStaleContext   = "stale_context"
	StatusOutcomeUnknown = "outcome_unknown"

	maxRecordBytes        = 1024 * 1024
	maxGrantMembers       = 512
	maxOpaqueTextBytes    = 4096
	workIDDomain          = "analytix/pending-work-id/v1\x00"
	receiptIDDomain       = "analytix/pending-work-receipt-id/v1\x00"
	receiptSigningDomain  = "analytix/pending-work-receipt-signature/v1\x00"
	dispositionIDDomain   = "analytix/pending-work-disposition-id/v1\x00"
	dispositionSignDomain = "analytix/pending-work-disposition-signature/v1\x00"
)

var reasonCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,95}$`)

// ContextBindingV1 is the minimum private authority needed to reject stale or
// cross-case work. It deliberately excludes workspace paths, tenant/user IDs,
// raw case IDs, prompts, model content, PII, and reasoning.
type ContextBindingV1 struct {
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	ContextDigest      string `json:"contextDigest"`
	CaseBindingHash    string `json:"caseBindingHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
}

// GrantMemberV1 binds one ordered work member to the exact durable grant
// registry entry. Provider continuations additionally bind the durable result
// item that settled the member grant.
type GrantMemberV1 struct {
	Ordinal             uint32 `json:"ordinal"`
	GrantID             string `json:"grantId"`
	RegistrySequence    uint64 `json:"registrySequence"`
	RegistryEntryDigest string `json:"registryEntryDigest"`
	ResultItemID        string `json:"resultItemId,omitempty"`
	ResultItemDigest    string `json:"resultItemDigest,omitempty"`
}

// PendingWorkReceiptV1 is private host authority. It contains only opaque
// identities and hashes; raw provider payloads, prompts, PII, tool results, and
// reasoning never belong in this contract.
type PendingWorkReceiptV1 struct {
	SchemaVersion         int              `json:"schemaVersion"`
	Purpose               string           `json:"purpose"`
	WorkID                string           `json:"workId"`
	ReceiptID             string           `json:"receiptId"`
	Kind                  string           `json:"kind"`
	Context               ContextBindingV1 `json:"context"`
	GrantRegistrySequence uint64           `json:"grantRegistrySequence"`
	GrantRegistryDigest   string           `json:"grantRegistryDigest"`
	GrantMembers          []GrantMemberV1  `json:"grantMembers"`
	PayloadHash           string           `json:"payloadHash"`
	RouteHash             string           `json:"routeHash"`
	IssuedAt              string           `json:"issuedAt"`
	ExpiresAt             string           `json:"expiresAt"`
	AuthorityAlgorithm    string           `json:"authorityAlgorithm"`
	AuthorityKeyID        string           `json:"authorityKeyId"`
	AuthorityPublicKey    string           `json:"authorityPublicKey"`
	AuthoritySignature    string           `json:"authoritySignature"`
	ChildProducer         *ChildProducerV1 `json:"childProducer,omitempty"`
}

type ReceiptInputV1 struct {
	Kind                  string
	SecurityContext       domainsecurity.TurnSecurityContext
	GrantRegistrySequence uint64
	GrantRegistryDigest   string
	GrantMembers          []GrantMemberV1
	PayloadHash           string
	RouteHash             string
	IssuedAt              time.Time
	ExpiresAt             time.Time
	AuthorityKeyID        string
	AuthorityPublicKey    []byte
	ChildProducer         *ChildProducerV1
}

type PendingWorkDispositionV1 struct {
	SchemaVersion      int    `json:"schemaVersion"`
	Purpose            string `json:"purpose"`
	DispositionID      string `json:"dispositionId"`
	WorkID             string `json:"workId"`
	ReceiptID          string `json:"receiptId"`
	Kind               string `json:"kind"`
	ContextDigest      string `json:"contextDigest"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	GrantMembersDigest string `json:"grantMembersDigest"`
	PayloadHash        string `json:"payloadHash"`
	RouteHash          string `json:"routeHash"`
	Status             string `json:"status"`
	ReasonCode         string `json:"reasonCode"`
	DisposedAt         string `json:"disposedAt"`
	AuthorityAlgorithm string `json:"authorityAlgorithm"`
	AuthorityKeyID     string `json:"authorityKeyId"`
	AuthorityPublicKey string `json:"authorityPublicKey"`
	AuthoritySignature string `json:"authoritySignature"`
}

type SignFunc func([]byte) ([]byte, error)

func NewPendingWorkReceiptV1(input ReceiptInputV1, sign SignFunc) (PendingWorkReceiptV1, error) {
	if err := validateReceiptChildProducer(input.Kind, input.ChildProducer); err != nil {
		return PendingWorkReceiptV1{}, err
	}
	contextBinding, err := ContextBindingFromExecutionContextV1(input.SecurityContext)
	if err != nil {
		return PendingWorkReceiptV1{}, err
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() || !expiresAt.After(issuedAt) {
		return PendingWorkReceiptV1{}, errors.New("pending work expiry must follow issuance")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	receipt := PendingWorkReceiptV1{
		SchemaVersion: SchemaVersion, Purpose: ReceiptPurpose,
		Kind: strings.TrimSpace(input.Kind), Context: contextBinding,
		GrantRegistrySequence: input.GrantRegistrySequence,
		GrantRegistryDigest:   strings.TrimSpace(input.GrantRegistryDigest),
		GrantMembers:          sealGrantMembers(input.GrantMembers),
		PayloadHash:           strings.TrimSpace(input.PayloadHash),
		RouteHash:             strings.TrimSpace(input.RouteHash),
		IssuedAt:              issuedAt.Format(time.RFC3339Nano),
		ExpiresAt:             expiresAt.Format(time.RFC3339Nano),
		AuthorityAlgorithm:    AuthorityAlgorithm,
		AuthorityKeyID:        strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:    base64.RawURLEncoding.EncodeToString(publicKey),
		ChildProducer:         CloneChildProducerV1(input.ChildProducer),
	}
	receipt.WorkID = computeWorkID(receipt)
	receipt.ReceiptID = computeReceiptID(receipt)
	bounded := receipt
	bounded.AuthoritySignature = strings.Repeat("A", base64.RawURLEncoding.EncodedLen(ed25519.SignatureSize))
	boundedBytes, err := json.Marshal(bounded)
	if err != nil || validateRecordJSON(boundedBytes) != nil {
		return PendingWorkReceiptV1{}, errors.New("pending work receipt exceeds record bounds")
	}
	if len(publicKey) != ed25519.PublicKeySize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) || sign == nil {
		return PendingWorkReceiptV1{}, errors.New("pending work receipt authority is invalid")
	}
	signature, err := sign(PendingWorkReceiptV1SigningBytes(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PendingWorkReceiptV1{}, errors.New("pending work receipt signing failed")
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidatePendingWorkReceiptV1(receipt); err != nil {
		return PendingWorkReceiptV1{}, err
	}
	return receipt, nil
}

func NewPendingWorkDispositionV1(receipt PendingWorkReceiptV1, status, reasonCode string, disposedAt time.Time, keyID string, publicKey []byte, sign SignFunc) (PendingWorkDispositionV1, error) {
	if err := ValidatePendingWorkReceiptV1(receipt); err != nil {
		return PendingWorkDispositionV1{}, err
	}
	disposedAt = disposedAt.UTC()
	if disposedAt.IsZero() {
		disposedAt = time.Now().UTC()
	}
	publicKey = append([]byte(nil), publicKey...)
	disposition := PendingWorkDispositionV1{
		SchemaVersion: SchemaVersion, Purpose: DispositionPurpose,
		WorkID: receipt.WorkID, ReceiptID: receipt.ReceiptID, Kind: receipt.Kind,
		ContextDigest: receipt.Context.ContextDigest, ContextEpoch: receipt.Context.ContextEpoch,
		DatasetSnapshotID:  receipt.Context.DatasetSnapshotID,
		GrantMembersDigest: grantMembersDigest(receipt.GrantMembers),
		PayloadHash:        receipt.PayloadHash, RouteHash: receipt.RouteHash,
		Status: strings.TrimSpace(status), ReasonCode: strings.TrimSpace(reasonCode),
		DisposedAt: disposedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: AuthorityAlgorithm,
		AuthorityKeyID: strings.TrimSpace(keyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	disposition.DispositionID = computeDispositionID(disposition)
	if len(publicKey) != ed25519.PublicKeySize || disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) || sign == nil {
		return PendingWorkDispositionV1{}, errors.New("pending work disposition authority is invalid")
	}
	signature, err := sign(PendingWorkDispositionV1SigningBytes(disposition))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PendingWorkDispositionV1{}, errors.New("pending work disposition signing failed")
	}
	disposition.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := ValidatePendingWorkDispositionForReceiptV1(disposition, receipt); err != nil {
		return PendingWorkDispositionV1{}, err
	}
	return disposition, nil
}

func ContextBindingFromSecurityContextV1(context domainsecurity.TurnSecurityContext) (ContextBindingV1, error) {
	if err := domainsecurity.ValidateTurnSecurityContext(context); err != nil {
		return ContextBindingV1{}, errors.New("pending work security context is invalid")
	}
	binding := ContextBindingV1{
		ThreadID: strings.TrimSpace(context.ThreadID), TurnID: strings.TrimSpace(context.TurnID),
		ContextDigest: strings.TrimSpace(context.ContextDigest), CaseBindingHash: strings.TrimSpace(context.CaseBindingHash),
		ContextEpoch: context.ContextEpoch, DatasetSnapshotID: strings.TrimSpace(context.DatasetSnapshotID),
		SourceManifestHash: strings.TrimSpace(context.SourceManifestHash),
	}
	return binding, ValidateContextBindingV1(binding)
}

// ContextBindingFromExecutionContextV1 is the live receipt-record boundary.
// Per-call grant admission decides whether a member is ordinary or protected;
// the receipt itself only records opaque grant membership. Keeping this
// converter at the ordinary-effect floor lets an ordinary pending operation
// survive a case-data boundary without authorizing a protected grant. The
// generic converter remains available for audit parsing and deterministic
// migration of historical V1 records.
func ContextBindingFromExecutionContextV1(context domainsecurity.TurnSecurityContext) (ContextBindingV1, error) {
	if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(context); err != nil {
		return ContextBindingV1{}, errors.New("pending work requires current V2 ordinary-effect authority")
	}
	return ContextBindingFromSecurityContextV1(context)
}

func ValidateContextBindingV1(binding ContextBindingV1) error {
	if !validOpaqueText(binding.ThreadID) || !validOpaqueText(binding.TurnID) ||
		!domainsecurity.IsSHA256Hex(binding.ContextDigest) || !domainsecurity.IsSHA256Hex(binding.CaseBindingHash) ||
		binding.ContextEpoch == 0 || !validOpaqueText(binding.DatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(binding.SourceManifestHash) {
		return errors.New("pending work context binding is invalid")
	}
	return nil
}

func ValidatePendingWorkReceiptV1(receipt PendingWorkReceiptV1) error {
	body, err := json.Marshal(receipt)
	if err != nil || validateRecordJSON(body) != nil {
		return errors.New("pending work receipt exceeds record bounds")
	}
	if err := validateReceiptChildProducer(receipt.Kind, receipt.ChildProducer); err != nil {
		return err
	}
	if receipt.SchemaVersion != SchemaVersion || receipt.Purpose != ReceiptPurpose || !validKind(receipt.Kind) ||
		ValidateContextBindingV1(receipt.Context) != nil || receipt.GrantRegistrySequence == 0 ||
		!domainsecurity.IsSHA256Hex(receipt.GrantRegistryDigest) || !domainsecurity.IsSHA256Hex(receipt.PayloadHash) ||
		!domainsecurity.IsSHA256Hex(receipt.RouteHash) || !domainsecurity.IsSHA256Hex(receipt.WorkID) ||
		!domainsecurity.IsSHA256Hex(receipt.ReceiptID) || receipt.AuthorityAlgorithm != AuthorityAlgorithm ||
		!domainsecurity.IsSHA256Hex(receipt.AuthorityKeyID) {
		return errors.New("pending work receipt identity is invalid")
	}
	if err := validateGrantMembers(receipt.Kind, receipt.GrantRegistrySequence, receipt.GrantMembers); err != nil {
		return err
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, receipt.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || !expiresAt.After(issuedAt) {
		return errors.New("pending work receipt time range is invalid")
	}
	if receipt.WorkID != computeWorkID(receipt) || receipt.ReceiptID != computeReceiptID(receipt) {
		return errors.New("pending work receipt integrity is invalid")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize || receipt.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PendingWorkReceiptV1SigningBytes(receipt), signature) {
		return errors.New("pending work receipt authority material is invalid")
	}
	return nil
}

func ValidatePendingWorkDispositionV1(disposition PendingWorkDispositionV1) error {
	if disposition.SchemaVersion != SchemaVersion || disposition.Purpose != DispositionPurpose ||
		!domainsecurity.IsSHA256Hex(disposition.DispositionID) || !domainsecurity.IsSHA256Hex(disposition.WorkID) ||
		!domainsecurity.IsSHA256Hex(disposition.ReceiptID) || !validKind(disposition.Kind) ||
		!domainsecurity.IsSHA256Hex(disposition.ContextDigest) || disposition.ContextEpoch == 0 ||
		!validOpaqueText(disposition.DatasetSnapshotID) || !domainsecurity.IsSHA256Hex(disposition.GrantMembersDigest) ||
		!domainsecurity.IsSHA256Hex(disposition.PayloadHash) || !domainsecurity.IsSHA256Hex(disposition.RouteHash) ||
		!validStatus(disposition.Status) || !reasonCodePattern.MatchString(disposition.ReasonCode) ||
		disposition.AuthorityAlgorithm != AuthorityAlgorithm || !domainsecurity.IsSHA256Hex(disposition.AuthorityKeyID) {
		return errors.New("pending work disposition identity is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, disposition.DisposedAt); err != nil {
		return errors.New("pending work disposition time is invalid")
	}
	if disposition.DispositionID != computeDispositionID(disposition) {
		return errors.New("pending work disposition integrity is invalid")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize ||
		len(signature) != ed25519.SignatureSize || disposition.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), PendingWorkDispositionV1SigningBytes(disposition), signature) {
		return errors.New("pending work disposition authority material is invalid")
	}
	return nil
}

func ValidatePendingWorkDispositionForReceiptV1(disposition PendingWorkDispositionV1, receipt PendingWorkReceiptV1) error {
	if err := ValidatePendingWorkReceiptV1(receipt); err != nil {
		return err
	}
	if err := ValidatePendingWorkDispositionV1(disposition); err != nil {
		return err
	}
	if disposition.WorkID != receipt.WorkID || disposition.ReceiptID != receipt.ReceiptID || disposition.Kind != receipt.Kind ||
		disposition.ContextDigest != receipt.Context.ContextDigest || disposition.ContextEpoch != receipt.Context.ContextEpoch ||
		disposition.DatasetSnapshotID != receipt.Context.DatasetSnapshotID ||
		disposition.GrantMembersDigest != grantMembersDigest(receipt.GrantMembers) ||
		disposition.PayloadHash != receipt.PayloadHash || disposition.RouteHash != receipt.RouteHash ||
		disposition.AuthorityKeyID != receipt.AuthorityKeyID || disposition.AuthorityPublicKey != receipt.AuthorityPublicKey {
		return errors.New("pending work disposition does not bind the exact receipt")
	}
	if disposition.Status == StatusOutcomeUnknown {
		validUnknown := ((receipt.Kind == KindApprovedToolDispatch || receipt.Kind == KindSideEffectIntent) && disposition.ReasonCode == "tool_outcome_unknown_after_restart") ||
			(receipt.Kind == KindReportStage && disposition.ReasonCode == "report_stage_outcome_unknown_after_restart")
		if !validUnknown {
			return errors.New("pending work outcome-unknown disposition is invalid")
		}
	} else if disposition.ReasonCode == "tool_outcome_unknown_after_restart" || disposition.ReasonCode == "report_stage_outcome_unknown_after_restart" {
		return errors.New("pending work outcome-unknown reason is invalid")
	}
	if receipt.Kind == KindApprovedToolDispatch && disposition.Status == StatusCompleted && disposition.ReasonCode != "tool_outcome_durable" {
		return errors.New("approved tool dispatch completion reason is invalid")
	}
	if receipt.Kind == KindSideEffectIntent && disposition.Status == StatusCompleted && disposition.ReasonCode != "tool_outcome_durable" {
		return errors.New("side effect intent completion reason is invalid")
	}
	if receipt.Kind == KindReportStage && disposition.Status == StatusCompleted && disposition.ReasonCode != "report_stage_completed" {
		return errors.New("report stage completion reason is invalid")
	}
	issuedAt, _ := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	disposedAt, _ := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if disposedAt.Before(issuedAt) {
		return errors.New("pending work disposition predates its receipt")
	}
	return nil
}

func ParsePendingWorkReceiptV1(body []byte) (PendingWorkReceiptV1, error) {
	var receipt PendingWorkReceiptV1
	if err := decodeStrict(body, &receipt); err != nil {
		return PendingWorkReceiptV1{}, err
	}
	return receipt, ValidatePendingWorkReceiptV1(receipt)
}

func ParsePendingWorkDispositionV1(body []byte) (PendingWorkDispositionV1, error) {
	var disposition PendingWorkDispositionV1
	if err := decodeStrict(body, &disposition); err != nil {
		return PendingWorkDispositionV1{}, err
	}
	return disposition, ValidatePendingWorkDispositionV1(disposition)
}

func PendingWorkReceiptV1Bytes(receipt PendingWorkReceiptV1) ([]byte, error) {
	if err := ValidatePendingWorkReceiptV1(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

func PendingWorkDispositionV1Bytes(disposition PendingWorkDispositionV1) ([]byte, error) {
	if err := ValidatePendingWorkDispositionV1(disposition); err != nil {
		return nil, err
	}
	return json.Marshal(disposition)
}

func PendingWorkReceiptV1SigningBytes(receipt PendingWorkReceiptV1) []byte {
	value := receipt
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return append([]byte(receiptSigningDomain), body...)
}

func PendingWorkDispositionV1SigningBytes(disposition PendingWorkDispositionV1) []byte {
	value := disposition
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return append([]byte(dispositionSignDomain), body...)
}

func PendingWorkReceiptV1AuthorityMaterial(receipt PendingWorkReceiptV1) (string, []byte, []byte, error) {
	if err := ValidatePendingWorkReceiptV1(receipt); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(receipt.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(receipt.AuthoritySignature)
	return receipt.AuthorityKeyID, publicKey, signature, nil
}

func PendingWorkDispositionV1AuthorityMaterial(disposition PendingWorkDispositionV1) (string, []byte, []byte, error) {
	if err := ValidatePendingWorkDispositionV1(disposition); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(disposition.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(disposition.AuthoritySignature)
	return disposition.AuthorityKeyID, publicKey, signature, nil
}

func ComputePendingWorkIDV1(input ReceiptInputV1) (string, error) {
	if err := validateReceiptChildProducer(input.Kind, input.ChildProducer); err != nil {
		return "", err
	}
	contextBinding, err := ContextBindingFromExecutionContextV1(input.SecurityContext)
	if err != nil {
		return "", err
	}
	receipt := PendingWorkReceiptV1{
		SchemaVersion: SchemaVersion, Purpose: ReceiptPurpose, Kind: strings.TrimSpace(input.Kind), Context: contextBinding,
		GrantRegistrySequence: input.GrantRegistrySequence, GrantRegistryDigest: strings.TrimSpace(input.GrantRegistryDigest),
		GrantMembers: sealGrantMembers(input.GrantMembers), PayloadHash: strings.TrimSpace(input.PayloadHash), RouteHash: strings.TrimSpace(input.RouteHash),
	}
	if !validKind(receipt.Kind) || receipt.GrantRegistrySequence == 0 || !domainsecurity.IsSHA256Hex(receipt.GrantRegistryDigest) ||
		!domainsecurity.IsSHA256Hex(receipt.PayloadHash) || !domainsecurity.IsSHA256Hex(receipt.RouteHash) {
		return "", errors.New("pending work identity input is invalid")
	}
	if err := validateGrantMembers(receipt.Kind, receipt.GrantRegistrySequence, receipt.GrantMembers); err != nil {
		return "", err
	}
	return computeWorkID(receipt), nil
}

func computeWorkID(receipt PendingWorkReceiptV1) string {
	// A side-effect intent identifies the semantic operation for one frozen
	// turn. Grant/call ids, registry growth, approval transitions, and route
	// changes cannot create a second execution identity.
	if receipt.Kind == KindSideEffectIntent {
		value := struct {
			SchemaVersion int              `json:"schemaVersion"`
			Purpose       string           `json:"purpose"`
			Kind          string           `json:"kind"`
			Context       ContextBindingV1 `json:"context"`
			PayloadHash   string           `json:"payloadHash"`
		}{receipt.SchemaVersion, receipt.Purpose, receipt.Kind, receipt.Context, receipt.PayloadHash}
		body, _ := json.Marshal(value)
		return domainsecurity.SHA256Hex(append([]byte(workIDDomain), body...))
	}
	// Approved writable dispatch is an effect identity, not a snapshot identity.
	// Its one grant member already binds the stable registry entry that approved
	// the exact call. Excluding the later-growing registry head prevents an
	// unrelated append from minting a second WorkID for the same side effect.
	if receipt.Kind == KindApprovedToolDispatch {
		value := struct {
			SchemaVersion int              `json:"schemaVersion"`
			Purpose       string           `json:"purpose"`
			Kind          string           `json:"kind"`
			Context       ContextBindingV1 `json:"context"`
			GrantMembers  []GrantMemberV1  `json:"grantMembers"`
			PayloadHash   string           `json:"payloadHash"`
			RouteHash     string           `json:"routeHash"`
		}{receipt.SchemaVersion, receipt.Purpose, receipt.Kind, receipt.Context,
			receipt.GrantMembers, receipt.PayloadHash, receipt.RouteHash}
		body, _ := json.Marshal(value)
		return domainsecurity.SHA256Hex(append([]byte(workIDDomain), body...))
	}
	if receipt.Kind == KindReportStage {
		value := struct {
			SchemaVersion int              `json:"schemaVersion"`
			Purpose       string           `json:"purpose"`
			Kind          string           `json:"kind"`
			Context       ContextBindingV1 `json:"context"`
			GrantMembers  []GrantMemberV1  `json:"grantMembers"`
			RouteHash     string           `json:"routeHash"`
		}{receipt.SchemaVersion, receipt.Purpose, receipt.Kind, receipt.Context, receipt.GrantMembers, receipt.RouteHash}
		body, _ := json.Marshal(value)
		return domainsecurity.SHA256Hex(append([]byte(workIDDomain), body...))
	}
	value := struct {
		SchemaVersion         int              `json:"schemaVersion"`
		Purpose               string           `json:"purpose"`
		Kind                  string           `json:"kind"`
		Context               ContextBindingV1 `json:"context"`
		GrantRegistrySequence uint64           `json:"grantRegistrySequence"`
		GrantRegistryDigest   string           `json:"grantRegistryDigest"`
		GrantMembers          []GrantMemberV1  `json:"grantMembers"`
		PayloadHash           string           `json:"payloadHash"`
		RouteHash             string           `json:"routeHash"`
	}{receipt.SchemaVersion, receipt.Purpose, receipt.Kind, receipt.Context, receipt.GrantRegistrySequence,
		receipt.GrantRegistryDigest, receipt.GrantMembers, receipt.PayloadHash, receipt.RouteHash}
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(workIDDomain), body...))
}

func computeReceiptID(receipt PendingWorkReceiptV1) string {
	value := receipt
	value.ReceiptID = ""
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(receiptIDDomain), body...))
}

func computeDispositionID(disposition PendingWorkDispositionV1) string {
	value := disposition
	value.DispositionID = ""
	value.AuthoritySignature = ""
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(append([]byte(dispositionIDDomain), body...))
}

func grantMembersDigest(members []GrantMemberV1) string {
	body, _ := json.Marshal(members)
	return domainsecurity.SHA256Hex(body)
}

func sealGrantMembers(members []GrantMemberV1) []GrantMemberV1 {
	sealed := append([]GrantMemberV1(nil), members...)
	for index := range sealed {
		sealed[index].GrantID = strings.TrimSpace(sealed[index].GrantID)
		sealed[index].RegistryEntryDigest = strings.TrimSpace(sealed[index].RegistryEntryDigest)
		sealed[index].ResultItemID = strings.TrimSpace(sealed[index].ResultItemID)
		sealed[index].ResultItemDigest = strings.TrimSpace(sealed[index].ResultItemDigest)
	}
	return sealed
}

func validateGrantMembers(kind string, registrySequence uint64, members []GrantMemberV1) error {
	if len(members) == 0 || len(members) > maxGrantMembers {
		return errors.New("pending work grant member count is invalid")
	}
	if (kind == KindApprovedToolDispatch || kind == KindSideEffectIntent) && len(members) != 1 {
		return errors.New("side effect dispatch must bind exactly one grant")
	}
	seen := map[string]bool{}
	var previousRegistrySequence uint64
	for index, member := range members {
		if member.Ordinal != uint32(index+1) || !domainsecurity.IsSHA256Hex(member.GrantID) ||
			member.RegistrySequence == 0 || member.RegistrySequence > registrySequence ||
			!domainsecurity.IsSHA256Hex(member.RegistryEntryDigest) || seen[member.GrantID] {
			return errors.New("pending work ordered grant member is invalid")
		}
		if index > 0 && member.RegistrySequence <= previousRegistrySequence {
			return errors.New("pending work grant registry order is invalid")
		}
		seen[member.GrantID] = true
		previousRegistrySequence = member.RegistrySequence
		if kind == KindProviderContinuation {
			if !validOpaqueText(member.ResultItemID) || !domainsecurity.IsSHA256Hex(member.ResultItemDigest) {
				return errors.New("provider continuation member lacks an exact durable result item")
			}
		} else if member.ResultItemID != "" || member.ResultItemDigest != "" {
			return errors.New("non-provider pending work contains a result item")
		}
	}
	return nil
}

func decodeStrict(body []byte, target any) error {
	if err := validateRecordJSON(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("pending work record contains trailing JSON")
	}
	return nil
}

func validateRecordJSON(body []byte) error {
	return domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxRecordBytes, MaxDepth: 32, MaxTokens: 20_000, MaxStringBytes: maxOpaqueTextBytes,
	})
}

func validKind(value string) bool {
	switch value {
	case KindToolBatch, KindApprovedToolDispatch, KindSideEffectIntent, KindProviderContinuation, KindReportStage:
		return true
	default:
		return false
	}
}

func validStatus(value string) bool {
	switch value {
	case StatusCompleted, StatusCancelled, StatusFailed, StatusExpired, StatusRejected, StatusRestartInvalid, StatusStaleContext, StatusOutcomeUnknown:
		return true
	default:
		return false
	}
}

func validOpaqueText(value string) bool {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > maxOpaqueTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
