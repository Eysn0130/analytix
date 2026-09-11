package evidence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const (
	PreparedEvidenceSettlementVersion = 1
	EvidenceSettlementMarkerVersion   = 1
	EvidenceSettlementPurpose         = "analytix.evidence-settlement/v1"
)

var evidenceSettlementSignatureDomain = []byte("analytix.evidence-settlement-authority/v1\x00")

// PreparedEvidenceSettlement is private crash-recovery material. It is not an
// EvidenceReceipt and cannot authorize a claim until an exact durable tool
// result marker is replayed and the draft is committed to the private registry.
type PreparedEvidenceSettlement struct {
	SchemaVersion               int                                `json:"schemaVersion"`
	Purpose                     string                             `json:"purpose"`
	AuthorityAlgorithm          string                             `json:"authorityAlgorithm"`
	AuthorityKeyID              string                             `json:"authorityKeyId"`
	AuthorityPublicKey          string                             `json:"authorityPublicKey"`
	SettlementID                string                             `json:"settlementId"`
	ReceiptID                   string                             `json:"receiptId"`
	SecurityContext             domainsecurity.TurnSecurityContext `json:"securityContext"`
	ExecutionGrant              domainsecurity.ExecutionGrant      `json:"executionGrant"`
	ActiveGrantRegistrySequence uint64                             `json:"activeGrantRegistrySequence"`
	ActiveGrantRegistryDigest   string                             `json:"activeGrantRegistryDigest"`
	SourceProbe                 domainsecurity.VerifiedSourceProbe `json:"sourceProbe"`
	ToolOutcome                 ToolOutcome                        `json:"toolOutcome"`
	RawResultBase64             string                             `json:"rawResultBase64"`
	RawSHA256                   string                             `json:"rawSha256"`
	CanonicalEvidence           json.RawMessage                    `json:"canonicalEvidence"`
	ReceiptDraft                EvidenceReceipt                    `json:"receiptDraft"`
	QueryHash                   string                             `json:"queryHash"`
	ResultItemID                string                             `json:"resultItemId"`
	// HostAuthority is an optional, signed V2 host-authority witness. A
	// pointer keeps historical V1 JSON/canonical IDs byte-for-byte stable when
	// the field is absent.
	HostAuthority      *PreparedEvidenceHostAuthorityV1 `json:"hostAuthority,omitempty"`
	PreparedAt         string                           `json:"preparedAt"`
	AuthoritySignature string                           `json:"authoritySignature"`
	RecordDigest       string                           `json:"recordDigest"`
}

// PreparedEvidenceHostAuthorityV1 binds an account-flow settlement to the
// exact host case observation and witnessed dataset selection that enclosed
// the one-use private carrier. It is deliberately only a digest/observation;
// the callback-scoped capability itself is never serializable.
type PreparedEvidenceHostAuthorityV1 struct {
	Binding         domainsecurity.CaseBindingObservationV1 `json:"binding"`
	SelectionDigest string                                  `json:"selectionDigest"`
	// Omitted in historical records to preserve their signed bytes and IDs.
	// SelectionDigest remains the original full preparation witness digest.
	SelectionContentDigest string `json:"selectionContentDigest,omitempty"`
}

type PreparedEvidenceSettlementInput struct {
	Context                     domainsecurity.TurnSecurityContext
	Grant                       domainsecurity.ExecutionGrant
	ActiveGrantRegistrySequence uint64
	ActiveGrantRegistryDigest   string
	SourceProbe                 domainsecurity.VerifiedSourceProbe
	ToolOutcome                 ToolOutcome
	RawResult                   []byte
	CanonicalEvidence           json.RawMessage
	ReceiptDraft                EvidenceReceipt
	QueryHash                   string
	ResultItemID                string
	HostAuthority               *PreparedEvidenceHostAuthorityV1
	PreparedAt                  time.Time
	AuthorityKeyID              string
	AuthorityPublicKey          []byte
}

type EvidenceSettlementSignFunc func([]byte) ([]byte, error)

// HostEvidenceSettlementMarker contains no case facts. Only an opaque
// in-process carrier may place it at the top level of a durable tool_result.
type HostEvidenceSettlementMarker struct {
	SchemaVersion        int    `json:"schemaVersion"`
	Purpose              string `json:"purpose"`
	SettlementID         string `json:"settlementId"`
	PreparedRecordDigest string `json:"preparedRecordDigest"`
	ReceiptID            string `json:"receiptId"`
	MarkerDigest         string `json:"markerDigest"`
}

type EvidenceSettlementProof struct {
	SettlementID         string `json:"settlementId"`
	PreparedRecordDigest string `json:"preparedRecordDigest"`
}

// ResolvePreparedSettlementIssue verifies the exact registry issue created
// from one private prepared settlement. Revocation does not erase the issue;
// callers use current membership separately when authorizing a claim.
func ResolvePreparedSettlementIssue(registry EvidenceReceiptRegistry, prepared PreparedEvidenceSettlement) (EvidenceReceipt, bool, error) {
	if ValidateEvidenceReceiptRegistry(registry) != nil || ValidatePreparedEvidenceSettlement(prepared) != nil ||
		!EvidenceReceiptRegistryMatchesContext(registry, prepared.SecurityContext) {
		return EvidenceReceipt{}, false, errors.New("prepared evidence settlement registry context is invalid")
	}
	found := false
	var receipt EvidenceReceipt
	for _, entry := range registry.Entries {
		if entry.Operation != EvidenceRegistryIssue {
			continue
		}
		associated := entry.ReceiptID == prepared.ReceiptID || entry.SettlementID == prepared.SettlementID ||
			entry.PreparedRecordDigest == prepared.RecordDigest
		if !associated {
			continue
		}
		if found || entry.Receipt == nil || entry.ReceiptID != prepared.ReceiptID || entry.SettlementID != prepared.SettlementID ||
			entry.PreparedRecordDigest != prepared.RecordDigest || entry.Receipt.ReceiptDigest != prepared.ReceiptDraft.ReceiptDigest ||
			!bytes.Equal(entry.CanonicalEvidence, prepared.CanonicalEvidence) {
			return EvidenceReceipt{}, false, errors.New("prepared evidence settlement registry issue conflicts with private authority")
		}
		found = true
		receipt = *entry.Receipt
	}
	return receipt, found, nil
}

func ValidateEvidenceSettlementProof(proof EvidenceSettlementProof, receiptID string) error {
	if !validSHA256(proof.SettlementID) || !validSHA256(proof.PreparedRecordDigest) ||
		strings.TrimSpace(receiptID) != EvidenceSettlementReceiptID(proof.SettlementID) {
		return errors.New("evidence settlement registry proof is invalid")
	}
	return nil
}

func ComputeEvidenceSettlementID(input PreparedEvidenceSettlementInput) string {
	draft := input.ReceiptDraft
	draft.ReceiptID = ""
	draft.IssuedAt = ""
	draft.ReceiptDigest = ""
	draft.RegistrySequence = 0
	draft.PreviousRegistryDigest = ""
	draft.RegistryIntegrityProof = ""
	canonical, _ := CanonicalEvidenceBytes(input.CanonicalEvidence)
	body, _ := json.Marshal(struct {
		SchemaVersion               int                                `json:"schemaVersion"`
		Purpose                     string                             `json:"purpose"`
		SecurityContext             domainsecurity.TurnSecurityContext `json:"securityContext"`
		ExecutionGrant              domainsecurity.ExecutionGrant      `json:"executionGrant"`
		ActiveGrantRegistrySequence uint64                             `json:"activeGrantRegistrySequence"`
		ActiveGrantRegistryDigest   string                             `json:"activeGrantRegistryDigest"`
		SourceProbe                 domainsecurity.VerifiedSourceProbe `json:"sourceProbe"`
		ToolOutcome                 ToolOutcome                        `json:"toolOutcome"`
		RawSHA256                   string                             `json:"rawSha256"`
		CanonicalEvidenceHash       string                             `json:"canonicalEvidenceHash"`
		ReceiptDraft                EvidenceReceipt                    `json:"receiptDraft"`
		QueryHash                   string                             `json:"queryHash"`
		ResultItemID                string                             `json:"resultItemId"`
		HostAuthority               *PreparedEvidenceHostAuthorityV1   `json:"hostAuthority,omitempty"`
	}{
		SchemaVersion: PreparedEvidenceSettlementVersion, Purpose: EvidenceSettlementPurpose,
		SecurityContext: input.Context, ExecutionGrant: input.Grant,
		ActiveGrantRegistrySequence: input.ActiveGrantRegistrySequence,
		ActiveGrantRegistryDigest:   strings.TrimSpace(input.ActiveGrantRegistryDigest),
		SourceProbe:                 input.SourceProbe, ToolOutcome: input.ToolOutcome,
		RawSHA256: strings.TrimSpace(input.ReceiptDraft.RawSHA256), CanonicalEvidenceHash: domainsecurity.CanonicalJSONHash(canonical),
		ReceiptDraft: draft, QueryHash: strings.TrimSpace(input.QueryHash), ResultItemID: strings.TrimSpace(input.ResultItemID),
		HostAuthority: input.HostAuthority,
	})
	return domainsecurity.SHA256Hex(body)
}

func EvidenceSettlementReceiptID(settlementID string) string {
	settlementID = strings.TrimSpace(settlementID)
	if !validSHA256(settlementID) {
		return ""
	}
	return "evr_" + settlementID
}

func NewPreparedEvidenceSettlement(input PreparedEvidenceSettlementInput, sign EvidenceSettlementSignFunc) (PreparedEvidenceSettlement, error) {
	if sign == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(input.Grant, input.Context) != nil ||
		!domainmodel.IsHostToolCallIDV1(input.Grant.ToolCallID) ||
		input.ResultItemID != domaintoolresult.ToolResultItemIDV1(input.Context.TurnID, input.Grant.ToolCallID) {
		return PreparedEvidenceSettlement{}, errors.New("evidence settlement signing authority is unavailable")
	}
	preparedAt := input.PreparedAt.UTC()
	if preparedAt.IsZero() {
		preparedAt = time.Now().UTC()
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	if len(publicKey) != ed25519.PublicKeySize || authorityKeyID(publicKey) != strings.TrimSpace(input.AuthorityKeyID) {
		return PreparedEvidenceSettlement{}, errors.New("evidence settlement authority key is invalid")
	}
	canonical, err := CanonicalEvidenceBytes(input.CanonicalEvidence)
	if err != nil {
		return PreparedEvidenceSettlement{}, errors.New("evidence settlement canonical material is invalid")
	}
	input.CanonicalEvidence = canonical
	settlementID := ComputeEvidenceSettlementID(input)
	record := PreparedEvidenceSettlement{
		SchemaVersion: PreparedEvidenceSettlementVersion, Purpose: EvidenceSettlementPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), SettlementID: settlementID,
		ReceiptID: EvidenceSettlementReceiptID(settlementID), SecurityContext: input.Context, ExecutionGrant: input.Grant,
		ActiveGrantRegistrySequence: input.ActiveGrantRegistrySequence,
		ActiveGrantRegistryDigest:   strings.TrimSpace(input.ActiveGrantRegistryDigest), SourceProbe: input.SourceProbe,
		ToolOutcome: input.ToolOutcome, RawResultBase64: base64.RawStdEncoding.EncodeToString(input.RawResult),
		RawSHA256: domainsecurity.SHA256Hex(input.RawResult), CanonicalEvidence: append(json.RawMessage(nil), canonical...),
		ReceiptDraft: input.ReceiptDraft, QueryHash: strings.TrimSpace(input.QueryHash), ResultItemID: strings.TrimSpace(input.ResultItemID),
		HostAuthority: input.HostAuthority,
		PreparedAt:    preparedAt.Format(time.RFC3339Nano),
	}
	signature, err := sign(EvidenceSettlementSigningBytes(record))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return PreparedEvidenceSettlement{}, errors.New("evidence settlement authority signing failed")
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	record.RecordDigest = preparedEvidenceSettlementDigest(record)
	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		return PreparedEvidenceSettlement{}, err
	}
	return record, nil
}

func ParsePreparedEvidenceSettlement(raw []byte) (PreparedEvidenceSettlement, error) {
	var record PreparedEvidenceSettlement
	if err := decodeStrictJSON(raw, &record, "prepared evidence settlement"); err != nil {
		return PreparedEvidenceSettlement{}, err
	}
	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		return PreparedEvidenceSettlement{}, err
	}
	return record, nil
}

func ValidatePreparedEvidenceSettlement(record PreparedEvidenceSettlement) error {
	if record.SchemaVersion != PreparedEvidenceSettlementVersion || record.Purpose != EvidenceSettlementPurpose ||
		record.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm || !validSHA256(record.AuthorityKeyID) ||
		!validSHA256(record.SettlementID) || record.ReceiptID != EvidenceSettlementReceiptID(record.SettlementID) ||
		domainsecurity.ValidateTurnSecurityContext(record.SecurityContext) != nil || domainsecurity.ValidateExecutionGrantForAudit(record.ExecutionGrant) != nil ||
		record.ActiveGrantRegistrySequence == 0 || !validSHA256(record.ActiveGrantRegistryDigest) ||
		domainsecurity.ValidateVerifiedSourceProbe(record.SourceProbe) != nil || ValidateToolOutcome(record.ToolOutcome) != nil ||
		!validSHA256(record.RawSHA256) || !validSHA256(record.QueryHash) || strings.TrimSpace(record.ResultItemID) == "" ||
		!validSHA256(record.RecordDigest) || ValidateEvidenceReceiptDraft(record.ReceiptDraft) != nil {
		return errors.New("prepared evidence settlement is incomplete")
	}
	preparedAt, preparedErr := time.Parse(time.RFC3339Nano, record.PreparedAt)
	grantIssuedAt, grantErr := time.Parse(time.RFC3339Nano, record.ExecutionGrant.IssuedAt)
	grantExpiresAt, expiresErr := time.Parse(time.RFC3339Nano, record.ExecutionGrant.ExpiresAt)
	probeCheckedAt, probeErr := time.Parse(time.RFC3339Nano, record.SourceProbe.CheckedAt)
	outcomeIssuedAt, outcomeErr := time.Parse(time.RFC3339Nano, record.ToolOutcome.IssuedAt)
	if preparedErr != nil || grantErr != nil || expiresErr != nil || probeErr != nil || outcomeErr != nil ||
		probeCheckedAt.After(grantIssuedAt) || grantIssuedAt.After(outcomeIssuedAt) || outcomeIssuedAt.After(preparedAt) || !preparedAt.Before(grantExpiresAt) {
		return errors.New("prepared evidence settlement time authority is invalid")
	}
	rawResult, err := base64.RawStdEncoding.DecodeString(record.RawResultBase64)
	if err != nil || domainsecurity.SHA256Hex(rawResult) != record.RawSHA256 || record.ReceiptDraft.RawSHA256 != record.RawSHA256 {
		return errors.New("prepared evidence settlement raw result is invalid")
	}
	canonical, err := CanonicalEvidenceBytes(record.CanonicalEvidence)
	if err != nil || !bytes.Equal(canonical, record.CanonicalEvidence) ||
		domainsecurity.CanonicalJSONHash(canonical) != record.ReceiptDraft.ResultHash || record.ReceiptDraft.QueryHash != record.QueryHash {
		return errors.New("prepared evidence settlement canonical material is invalid")
	}
	if record.ExecutionGrant.ContextDigest != record.SecurityContext.ContextDigest || record.ExecutionGrant.TurnID != record.SecurityContext.TurnID ||
		record.ToolOutcome.ContextDigest != record.SecurityContext.ContextDigest || record.ToolOutcome.ExecutionGrantID != record.ExecutionGrant.GrantID ||
		record.ToolOutcome.ToolName != record.ExecutionGrant.ToolName || record.ToolOutcome.ToolCallID != record.ExecutionGrant.ToolCallID ||
		record.ToolOutcome.CaseID != record.SecurityContext.CaseID || record.ToolOutcome.ContextEpoch != record.SecurityContext.ContextEpoch ||
		record.ToolOutcome.DatasetSnapshotID != record.SecurityContext.DatasetSnapshotID || record.ToolOutcome.ServerIdentity != record.ExecutionGrant.ServerIdentity ||
		record.SourceProbe.ProbeContextDigest != record.SecurityContext.ContextDigest || record.SourceProbe.ThreadID != record.SecurityContext.ThreadID ||
		record.SourceProbe.TurnID != record.SecurityContext.TurnID || record.SourceProbe.DatasetSnapshotID != record.SecurityContext.DatasetSnapshotID ||
		record.SourceProbe.CaseID != record.SecurityContext.CaseID || record.SourceProbe.ContextEpoch != record.SecurityContext.ContextEpoch ||
		record.SourceProbe.ConnectionEpoch != record.ExecutionGrant.ConnectionEpoch ||
		record.SourceProbe.ServerIdentity != record.ExecutionGrant.ServerIdentity ||
		record.SourceProbe.ServerIdentity != record.ToolOutcome.ServerIdentity ||
		record.ReceiptDraft.ReceiptID != record.ReceiptID || record.ReceiptDraft.ContextDigest != record.SecurityContext.ContextDigest ||
		record.ReceiptDraft.ExecutionGrantID != record.ExecutionGrant.GrantID || record.ReceiptDraft.ToolCallID != record.ExecutionGrant.ToolCallID ||
		record.ReceiptDraft.ToolName != record.ExecutionGrant.ToolName || record.ReceiptDraft.ArgsHash != record.ExecutionGrant.ArgsHash ||
		record.ReceiptDraft.ServerIdentity != record.SourceProbe.ServerIdentity || record.ReceiptDraft.ServerIdentity != record.ExecutionGrant.ServerIdentity ||
		record.ReceiptDraft.ConnectionEpoch != record.SourceProbe.ConnectionEpoch ||
		record.ReceiptDraft.DatasetSnapshotID != record.SecurityContext.DatasetSnapshotID ||
		record.ReceiptDraft.IssuedAt != record.PreparedAt {
		return errors.New("prepared evidence settlement authority binding is invalid")
	}
	if record.HostAuthority != nil {
		if domainsecurity.ValidateCaseBindingObservationV1(record.HostAuthority.Binding) != nil ||
			record.HostAuthority.Binding.State != domainsecurity.CaseBindingStateValid ||
			!validSHA256(record.HostAuthority.SelectionDigest) ||
			(record.HostAuthority.SelectionContentDigest != "" && !validSHA256(record.HostAuthority.SelectionContentDigest)) {
			return errors.New("prepared evidence settlement host authority is invalid")
		}
	}
	identityInput := PreparedEvidenceSettlementInput{
		Context: record.SecurityContext, Grant: record.ExecutionGrant,
		ActiveGrantRegistrySequence: record.ActiveGrantRegistrySequence, ActiveGrantRegistryDigest: record.ActiveGrantRegistryDigest,
		SourceProbe: record.SourceProbe, ToolOutcome: record.ToolOutcome, CanonicalEvidence: record.CanonicalEvidence,
		ReceiptDraft: record.ReceiptDraft, QueryHash: record.QueryHash, ResultItemID: record.ResultItemID, HostAuthority: record.HostAuthority,
	}
	if ComputeEvidenceSettlementID(identityInput) != record.SettlementID {
		return errors.New("prepared evidence settlement identity is invalid")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize || authorityKeyID(publicKey) != record.AuthorityKeyID {
		return errors.New("prepared evidence settlement public key is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	if err != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), EvidenceSettlementSigningBytes(record), signature) {
		return errors.New("prepared evidence settlement signature is invalid")
	}
	if preparedEvidenceSettlementDigest(record) != record.RecordDigest {
		return errors.New("prepared evidence settlement integrity is invalid")
	}
	return nil
}

// ValidatePreparedEvidenceSettlementForHostAuthorityV2 validates the
// callback-scoped host path. It deliberately does not call the historical
// SourceProbeCanAuthorizeFacts gate: a bare DSV2 probe remains insufficient,
// while the caller must separately prove the exact witnessed selection and
// active HostEvidenceCapability lease.
func ValidatePreparedEvidenceSettlementForHostAuthorityV2(
	record PreparedEvidenceSettlement,
	context domainsecurity.TurnSecurityContext,
	currentProbe domainsecurity.VerifiedSourceProbe,
	selectionDigest string,
) error {
	if record.HostAuthority == nil || record.HostAuthority.SelectionDigest != selectionDigest {
		return errors.New("prepared evidence settlement original host selection is unavailable")
	}
	return validatePreparedEvidenceSettlementHostContextV2(record, context, currentProbe, selectionDigest)
}

// ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2 compares signed
// content across witness exchanges. The caller must independently validate the
// full current selection and its active capability. Legacy records retain the
// exact original-selection rule and are never upgraded by this read.
func ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(
	record PreparedEvidenceSettlement,
	context domainsecurity.TurnSecurityContext,
	currentProbe domainsecurity.VerifiedSourceProbe,
	selectionDigest, selectionContentDigest string,
) error {
	if record.HostAuthority == nil {
		return errors.New("prepared evidence settlement host authority is unavailable")
	}
	if record.HostAuthority.SelectionContentDigest == "" {
		return ValidatePreparedEvidenceSettlementForHostAuthorityV2(record, context, currentProbe, selectionDigest)
	}
	if !validSHA256(selectionContentDigest) || record.HostAuthority.SelectionContentDigest != selectionContentDigest {
		return errors.New("prepared evidence settlement host content has changed")
	}
	return validatePreparedEvidenceSettlementHostContextV2(record, context, currentProbe, selectionDigest)
}

func validatePreparedEvidenceSettlementHostContextV2(
	record PreparedEvidenceSettlement,
	context domainsecurity.TurnSecurityContext,
	currentProbe domainsecurity.VerifiedSourceProbe,
	selectionDigest string,
) error {
	if err := ValidatePreparedEvidenceSettlement(record); err != nil ||
		record.HostAuthority == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil ||
		!reflect.DeepEqual(record.SecurityContext, context) ||
		!domainsecurity.SourceProbeEligibleForHostAuthorityV2(record.SourceProbe) ||
		!domainsecurity.SourceProbeEligibleForHostAuthorityV2(currentProbe) ||
		!reflect.DeepEqual(record.SourceProbe, currentProbe) ||
		!domainsecurity.IsSHA256Hex(selectionDigest) ||
		domainsecurity.ValidateCaseBindingObservationV1(record.HostAuthority.Binding) != nil ||
		record.HostAuthority.Binding.State != domainsecurity.CaseBindingStateValid ||
		record.HostAuthority.Binding.WorkspaceRealPath != context.WorkspaceRealPath ||
		record.HostAuthority.Binding.CaseID != context.CaseID ||
		record.HostAuthority.Binding.CaseBindingHash != context.CaseBindingHash ||
		currentProbe.ProbeContextDigest != context.ContextDigest ||
		currentProbe.ConnectionEpoch != record.ExecutionGrant.ConnectionEpoch {
		return errors.New("prepared evidence settlement host authority is unavailable")
	}
	rawResult, err := base64.RawStdEncoding.DecodeString(record.RawResultBase64)
	if err != nil || domainsecurity.SHA256Hex(rawResult) != record.RawSHA256 ||
		ValidateCanonicalEvidenceAgainstReceipt(record.ReceiptDraft, record.CanonicalEvidence) != nil ||
		validatePreparedEvidenceTransformationForExecution(record, rawResult) != nil {
		return errors.New("prepared evidence settlement host source authority is not reproducible")
	}
	material, err := ParseCanonicalEvidenceMaterial(record.CanonicalEvidence)
	if err != nil {
		return errors.New("prepared evidence settlement host canonical authority is not reproducible")
	}
	if material.SchemaVersion == CanonicalEvidenceVersionV2 &&
		ValidateSourceFieldBindingsAgainstRawResultV2(rawResult, record.RawSHA256, material) != nil {
		return errors.New("prepared evidence settlement host source field authority is not reproducible")
	}
	return nil
}

// ValidatePreparedEvidenceSettlementForExecution keeps historical V1
// settlements parseable for audit while preventing restart reconciliation or
// receipt issuance from granting them current execution authority.
func ValidatePreparedEvidenceSettlementForExecution(record PreparedEvidenceSettlement) error {
	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		return err
	}
	rawResult, err := base64.RawStdEncoding.DecodeString(record.RawResultBase64)
	if err != nil || domainsecurity.SHA256Hex(rawResult) != record.RawSHA256 ||
		ValidateCanonicalEvidenceAgainstReceipt(record.ReceiptDraft, record.CanonicalEvidence) != nil ||
		validatePreparedEvidenceTransformationForExecution(record, rawResult) != nil {
		return errors.New("prepared evidence settlement source authority is not reproducible")
	}
	material, err := ParseCanonicalEvidenceMaterial(record.CanonicalEvidence)
	if err != nil {
		return errors.New("prepared evidence settlement canonical authority is not reproducible")
	}
	if material.SchemaVersion == CanonicalEvidenceVersionV2 &&
		ValidateSourceFieldBindingsAgainstRawResultV2(rawResult, record.RawSHA256, material) != nil {
		return errors.New("prepared evidence settlement source field authority is not reproducible")
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(record.ExecutionGrant, record.SecurityContext) != nil ||
		!domainmodel.IsHostToolCallIDV1(record.ExecutionGrant.ToolCallID) ||
		record.ResultItemID != domaintoolresult.ToolResultItemIDV1(record.SecurityContext.TurnID, record.ExecutionGrant.ToolCallID) ||
		!domainsecurity.SourceProbeCanAuthorizeFacts(record.SourceProbe) {
		return errors.New("prepared evidence settlement lacks current V2 execution authority")
	}
	return nil
}

func validatePreparedEvidenceTransformationForExecution(record PreparedEvidenceSettlement, rawResult []byte) error {
	resultHash := domainsecurity.CanonicalJSONHash(record.CanonicalEvidence)
	lineage := record.ReceiptDraft.TransformationLineage
	if domainsecurity.CanonicalJSONHash(rawResult) == resultHash && len(lineage) == 0 {
		return nil
	}
	if len(lineage) == 0 || lineage[0].InputHash != record.RawSHA256 || lineage[len(lineage)-1].OutputHash != resultHash {
		return errors.New("prepared evidence settlement transformation authority is detached")
	}
	for index := 1; index < len(lineage); index++ {
		if lineage[index].InputHash != lineage[index-1].OutputHash {
			return errors.New("prepared evidence settlement transformation authority is broken")
		}
	}
	return nil
}

func EvidenceSettlementSigningBytes(record PreparedEvidenceSettlement) []byte {
	record.AuthoritySignature = ""
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	out := make([]byte, 0, len(evidenceSettlementSignatureDomain)+len(digest))
	out = append(out, evidenceSettlementSignatureDomain...)
	out = append(out, digest[:]...)
	return out
}

func EvidenceSettlementAuthorityMaterial(record PreparedEvidenceSettlement) (string, []byte, []byte, error) {
	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		return "", nil, nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(record.AuthorityPublicKey)
	signature, _ := base64.RawURLEncoding.DecodeString(record.AuthoritySignature)
	return record.AuthorityKeyID, publicKey, signature, nil
}

func NewHostEvidenceSettlementMarker(record PreparedEvidenceSettlement) (HostEvidenceSettlementMarker, error) {
	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		return HostEvidenceSettlementMarker{}, err
	}
	marker := HostEvidenceSettlementMarker{
		SchemaVersion: EvidenceSettlementMarkerVersion, Purpose: EvidenceSettlementPurpose,
		SettlementID: record.SettlementID, PreparedRecordDigest: record.RecordDigest, ReceiptID: record.ReceiptID,
	}
	marker.MarkerDigest = evidenceSettlementMarkerDigest(marker)
	return marker, ValidateHostEvidenceSettlementMarker(marker)
}

func ParseHostEvidenceSettlementMarker(value any) (HostEvidenceSettlementMarker, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return HostEvidenceSettlementMarker{}, err
	}
	var marker HostEvidenceSettlementMarker
	if err := decodeStrictJSON(body, &marker, "host evidence settlement marker"); err != nil {
		return HostEvidenceSettlementMarker{}, err
	}
	if err := ValidateHostEvidenceSettlementMarker(marker); err != nil {
		return HostEvidenceSettlementMarker{}, err
	}
	return marker, nil
}

// HostEvidenceSettlementMarkerRecordV1 returns the closed JSON-shaped form
// accepted by ordinary durable-record inspection. The marker contains only
// opaque binding digests; the prepared settlement and receipt remain private.
func HostEvidenceSettlementMarkerRecordV1(marker HostEvidenceSettlementMarker) (map[string]any, error) {
	if err := ValidateHostEvidenceSettlementMarker(marker); err != nil {
		return nil, err
	}
	return map[string]any{
		"schemaVersion":        marker.SchemaVersion,
		"purpose":              marker.Purpose,
		"settlementId":         marker.SettlementID,
		"preparedRecordDigest": marker.PreparedRecordDigest,
		"receiptId":            marker.ReceiptID,
		"markerDigest":         marker.MarkerDigest,
	}, nil
}

func ValidateHostEvidenceSettlementMarker(marker HostEvidenceSettlementMarker) error {
	if marker.SchemaVersion != EvidenceSettlementMarkerVersion || marker.Purpose != EvidenceSettlementPurpose ||
		!validSHA256(marker.SettlementID) || !validSHA256(marker.PreparedRecordDigest) ||
		marker.ReceiptID != EvidenceSettlementReceiptID(marker.SettlementID) || !validSHA256(marker.MarkerDigest) ||
		evidenceSettlementMarkerDigest(marker) != marker.MarkerDigest {
		return errors.New("host evidence settlement marker is invalid")
	}
	return nil
}

func PreparedEvidenceSettlementBytes(record PreparedEvidenceSettlement) ([]byte, error) {
	if err := ValidatePreparedEvidenceSettlement(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func preparedEvidenceSettlementDigest(record PreparedEvidenceSettlement) string {
	record.RecordDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func evidenceSettlementMarkerDigest(marker HostEvidenceSettlementMarker) string {
	marker.MarkerDigest = ""
	body, _ := json.Marshal(marker)
	return domainsecurity.SHA256Hex(body)
}
