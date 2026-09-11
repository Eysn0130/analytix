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

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	ThreadRiskPolicySchemaVersion = 1
	ThreadRiskPolicyPurpose       = "analytix.thread-risk-policy/v1"
	ThreadRiskPolicyAlgorithm     = "Ed25519"

	TurnPublicationPolicySchemaVersion = 1
	TurnPublicationPolicyPurpose       = "analytix.turn-publication-policy/v1"
)

const (
	RiskClassGeneral = "general"
	RiskClassCase    = "case"
)

const (
	RiskPolicyOriginGeneralWorkspace       = "general_workspace"
	RiskPolicyOriginDesktopCaseEntry       = "desktop_case_entry"
	RiskPolicyOriginBindingMarkerPresent   = "binding_marker_present"
	RiskPolicyOriginValidCaseBinding       = "valid_case_binding"
	RiskPolicyOriginSignedCaseLineage      = "signed_case_lineage"
	RiskPolicyOriginTrustedCaseFinal       = "trusted_case_final"
	RiskPolicyOriginLegacyMigration        = "legacy_migration"
	RiskPolicyOriginLexicalGuard           = "lexical_guard"
	RiskPolicyOriginProtectedDataGuard     = "protected_data_guard"
	RiskPolicyOriginHostInputContextChange = "host_input_context_change"
)

const (
	PublicationDispositionGeneralOutput    = "general_output"
	PublicationDispositionCaseEvidenceGate = "case_evidence_gate"
	PublicationDispositionCaseBoundaryOnly = "case_boundary_only"
)

const (
	CaseBindingStateNotApplicable     = "not_applicable"
	CaseBindingStateValid             = "valid"
	CaseBindingStateMissing           = "missing"
	CaseBindingStateInvalid           = "invalid"
	CaseBindingStateUnreadable        = "unreadable"
	CaseBindingStateUnstable          = "unstable"
	CaseBindingStateChangedUnaccepted = "changed_unaccepted"
	CaseBindingStateWorkspaceMissing  = "workspace_unavailable"
	CaseBindingStatePolicyCorrupt     = "policy_corrupt"
)

const (
	PublicationBlockerNone                       = "none"
	PublicationBlockerCaseBindingMissing         = "case_binding_missing"
	PublicationBlockerCaseBindingInvalid         = "case_binding_invalid"
	PublicationBlockerCaseBindingUnreadable      = "case_binding_unreadable"
	PublicationBlockerCaseBindingUnstable        = "case_binding_unstable"
	PublicationBlockerCaseBindingChanged         = "case_binding_changed"
	PublicationBlockerCaseWorkspaceUnavailable   = "case_workspace_unavailable"
	PublicationBlockerCasePolicyCorrupt          = "case_policy_corrupt"
	PublicationBlockerRiskAuthorityUnavailable   = "risk_authority_unavailable"
	PublicationBlockerRiskAuthorityIndeterminate = "risk_authority_indeterminate"
	PublicationBlockerRiskAuthorityInconsistent  = "risk_authority_inconsistent"
	PublicationBlockerDatasetSnapshotUnavailable = "dataset_snapshot_unavailable"
	PublicationBlockerDatasetSnapshotCorrupt     = "dataset_snapshot_corrupt"
	PublicationBlockerDatasetSnapshotMismatch    = "dataset_snapshot_mismatch"
	PublicationBlockerDatasetSnapshotStale       = "dataset_snapshot_stale"
)

var threadRiskPolicySignatureDomain = []byte("analytix.thread-risk-policy/v1\x00")

type ThreadRiskPolicyV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	ThreadID                string `json:"threadId"`
	WorkspaceRealPath       string `json:"workspaceRealPath"`
	RiskClass               string `json:"riskClass"`
	Origin                  string `json:"origin"`
	SignalsDigest           string `json:"signalsDigest"`
	PredecessorPolicyDigest string `json:"predecessorPolicyDigest,omitempty"`
	IssuedAt                string `json:"issuedAt"`
	AuthorityAlgorithm      string `json:"authorityAlgorithm"`
	AuthorityKeyID          string `json:"authorityKeyId"`
	AuthorityPublicKey      string `json:"authorityPublicKey"`
	AuthoritySignature      string `json:"authoritySignature"`
	PolicyDigest            string `json:"policyDigest"`
}

type ThreadRiskPolicyInputV1 struct {
	ThreadID                string
	WorkspaceRealPath       string
	RiskClass               string
	Origin                  string
	SignalsDigest           string
	PredecessorPolicyDigest string
	IssuedAt                time.Time
	AuthorityKeyID          string
	AuthorityPublicKey      []byte
}

type TurnPublicationPolicyV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	ThreadRiskPolicyDigest   string `json:"threadRiskPolicyDigest"`
	RiskClass                string `json:"riskClass"`
	Disposition              string `json:"disposition"`
	CaseBindingState         string `json:"caseBindingState"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
	BlockerCode              string `json:"blockerCode"`
	PolicyDigest             string `json:"policyDigest"`
}

type TurnPublicationPolicyInputV1 struct {
	ThreadRiskPolicyDigest   string
	RiskClass                string
	Disposition              string
	CaseBindingState         string
	BindingObservationDigest string
	BlockerCode              string
}

type RiskPolicySignFunc func([]byte) ([]byte, error)

func NewThreadRiskPolicyV1(input ThreadRiskPolicyInputV1, sign RiskPolicySignFunc) (ThreadRiskPolicyV1, error) {
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		return ThreadRiskPolicyV1{}, errors.New("thread risk policy issuedAt is required")
	}
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	policy := ThreadRiskPolicyV1{
		SchemaVersion: ThreadRiskPolicySchemaVersion, Purpose: ThreadRiskPolicyPurpose,
		ThreadID: strings.TrimSpace(input.ThreadID), WorkspaceRealPath: strings.TrimSpace(input.WorkspaceRealPath),
		RiskClass: strings.TrimSpace(input.RiskClass), Origin: strings.TrimSpace(input.Origin),
		SignalsDigest: strings.TrimSpace(input.SignalsDigest), PredecessorPolicyDigest: strings.TrimSpace(input.PredecessorPolicyDigest),
		IssuedAt: issuedAt.Format(time.RFC3339Nano), AuthorityAlgorithm: ThreadRiskPolicyAlgorithm,
		AuthorityKeyID: strings.TrimSpace(input.AuthorityKeyID), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || policy.AuthorityKeyID != SHA256Hex(publicKey) {
		return ThreadRiskPolicyV1{}, errors.New("thread risk policy signing authority is invalid")
	}
	if err := validateThreadRiskPolicyUnsigned(policy); err != nil {
		return ThreadRiskPolicyV1{}, err
	}
	signature, err := sign(ThreadRiskPolicySigningBytesV1(policy))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ThreadRiskPolicyV1{}, errors.New("thread risk policy signing failed")
	}
	policy.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	policy.PolicyDigest = threadRiskPolicyDigestV1(policy)
	if err := ValidateThreadRiskPolicyV1(policy); err != nil {
		return ThreadRiskPolicyV1{}, err
	}
	return policy, nil
}

func ValidateThreadRiskPolicyV1(policy ThreadRiskPolicyV1) error {
	if err := validateThreadRiskPolicyUnsigned(policy); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(policy.PolicyDigest) || policy.PolicyDigest != threadRiskPolicyDigestV1(policy) {
		return errors.New("thread risk policy digest is invalid")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(policy.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(policy.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != policy.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != policy.AuthoritySignature ||
		policy.AuthorityKeyID != SHA256Hex(publicKey) || !ed25519.Verify(ed25519.PublicKey(publicKey), ThreadRiskPolicySigningBytesV1(policy), signature) {
		return errors.New("thread risk policy authority is invalid")
	}
	return nil
}

// ValidateThreadRiskPolicyV1ForInstallation anchors the self-contained
// signature material to the authority trusted by this installation. Merely
// carrying a valid, attacker-generated Ed25519 key is not installation
// authority.
func ValidateThreadRiskPolicyV1ForInstallation(policy ThreadRiskPolicyV1, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidateThreadRiskPolicyV1(policy); err != nil {
		return err
	}
	authorityKeyID = strings.TrimSpace(authorityKeyID)
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if len(authorityPublicKey) != ed25519.PublicKeySize || authorityKeyID != SHA256Hex(authorityPublicKey) ||
		policy.AuthorityKeyID != authorityKeyID ||
		policy.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("thread risk policy installation authority mismatch")
	}
	return nil
}

func ValidateThreadRiskPolicyTransitionV1(previous, next ThreadRiskPolicyV1) error {
	if ValidateThreadRiskPolicyV1(previous) != nil || ValidateThreadRiskPolicyV1(next) != nil {
		return errors.New("thread risk policy transition authority is invalid")
	}
	if next.PredecessorPolicyDigest != previous.PolicyDigest || next.ThreadID != previous.ThreadID ||
		next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey {
		return errors.New("thread risk policy transition lineage is invalid")
	}
	previousIssuedAt, _ := time.Parse(time.RFC3339Nano, previous.IssuedAt)
	nextIssuedAt, _ := time.Parse(time.RFC3339Nano, next.IssuedAt)
	if nextIssuedAt.Before(previousIssuedAt) {
		return errors.New("thread risk policy transition time is invalid")
	}
	if previous.RiskClass == RiskClassCase && next.RiskClass != RiskClassCase {
		return errors.New("thread risk policy cannot downgrade case risk")
	}
	return nil
}

func ParseThreadRiskPolicyV1(body []byte) (ThreadRiskPolicyV1, error) {
	var policy ThreadRiskPolicyV1
	if err := decodeStrictRiskContract(body, &policy); err != nil {
		return ThreadRiskPolicyV1{}, err
	}
	return policy, ValidateThreadRiskPolicyV1(policy)
}

func ThreadRiskPolicyV1Bytes(policy ThreadRiskPolicyV1) ([]byte, error) {
	if err := ValidateThreadRiskPolicyV1(policy); err != nil {
		return nil, err
	}
	return json.Marshal(policy)
}

func ThreadRiskPolicySigningBytesV1(policy ThreadRiskPolicyV1) []byte {
	policy.AuthoritySignature = ""
	policy.PolicyDigest = ""
	body, _ := json.Marshal(policy)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), threadRiskPolicySignatureDomain...)
	return append(out, digest[:]...)
}

func NewTurnPublicationPolicyV1(input TurnPublicationPolicyInputV1) (TurnPublicationPolicyV1, error) {
	policy := TurnPublicationPolicyV1{
		SchemaVersion: TurnPublicationPolicySchemaVersion, Purpose: TurnPublicationPolicyPurpose,
		ThreadRiskPolicyDigest: strings.TrimSpace(input.ThreadRiskPolicyDigest), RiskClass: strings.TrimSpace(input.RiskClass),
		Disposition: strings.TrimSpace(input.Disposition), CaseBindingState: strings.TrimSpace(input.CaseBindingState),
		BindingObservationDigest: strings.TrimSpace(input.BindingObservationDigest), BlockerCode: strings.TrimSpace(input.BlockerCode),
	}
	policy.PolicyDigest = turnPublicationPolicyDigestV1(policy)
	if err := ValidateTurnPublicationPolicyV1(policy); err != nil {
		return TurnPublicationPolicyV1{}, err
	}
	return policy, nil
}

func ValidateTurnPublicationPolicyV1(policy TurnPublicationPolicyV1) error {
	if policy.SchemaVersion != TurnPublicationPolicySchemaVersion || policy.Purpose != TurnPublicationPolicyPurpose ||
		!isCanonicalSHA256Hex(policy.ThreadRiskPolicyDigest) || !isCanonicalSHA256Hex(policy.BindingObservationDigest) ||
		!isCanonicalSHA256Hex(policy.PolicyDigest) || policy.PolicyDigest != turnPublicationPolicyDigestV1(policy) {
		return errors.New("turn publication policy integrity is invalid")
	}
	switch policy.RiskClass {
	case RiskClassGeneral:
		if policy.Disposition != PublicationDispositionGeneralOutput || policy.CaseBindingState != CaseBindingStateMissing ||
			policy.BlockerCode != PublicationBlockerNone {
			return errors.New("general turn publication policy shape is invalid")
		}
	case RiskClassCase:
		switch policy.Disposition {
		case PublicationDispositionCaseEvidenceGate:
			if policy.CaseBindingState != CaseBindingStateValid || policy.BlockerCode != PublicationBlockerNone {
				return errors.New("case evidence publication policy shape is invalid")
			}
		case PublicationDispositionCaseBoundaryOnly:
			if !validCaseBoundaryBlocker(policy.CaseBindingState, policy.BlockerCode) {
				return errors.New("case boundary publication policy shape is invalid")
			}
		default:
			return errors.New("case turn publication disposition is invalid")
		}
	default:
		return errors.New("turn publication risk class is invalid")
	}
	return nil
}

// ValidateTurnPublicationPolicyForThreadRiskPolicyV1 proves that a per-turn
// publication decision is derived from the exact signed thread policy rather
// than an arbitrary non-empty digest or a mismatched risk class.
func ValidateTurnPublicationPolicyForThreadRiskPolicyV1(publication TurnPublicationPolicyV1, thread ThreadRiskPolicyV1) error {
	if err := ValidateTurnPublicationPolicyV1(publication); err != nil {
		return err
	}
	if err := ValidateThreadRiskPolicyV1(thread); err != nil {
		return err
	}
	if publication.ThreadRiskPolicyDigest != thread.PolicyDigest || publication.RiskClass != thread.RiskClass {
		return errors.New("turn publication policy thread risk binding is invalid")
	}
	if publication.Disposition == PublicationDispositionGeneralOutput && thread.Origin != RiskPolicyOriginGeneralWorkspace {
		return errors.New("general publication policy requires current general-workspace origin")
	}
	return nil
}

func ParseTurnPublicationPolicyV1(value any) (TurnPublicationPolicyV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return TurnPublicationPolicyV1{}, err
	}
	var policy TurnPublicationPolicyV1
	if err := decodeStrictRiskContract(body, &policy); err != nil {
		return TurnPublicationPolicyV1{}, err
	}
	return policy, ValidateTurnPublicationPolicyV1(policy)
}

func TurnPublicationPolicyV1Bytes(policy TurnPublicationPolicyV1) ([]byte, error) {
	if err := ValidateTurnPublicationPolicyV1(policy); err != nil {
		return nil, err
	}
	return json.Marshal(policy)
}

func TurnPublicationRequiresFinalEvidenceGate(policy TurnPublicationPolicyV1) bool {
	return ValidateTurnPublicationPolicyV1(policy) == nil && policy.RiskClass == RiskClassCase
}

func TurnPublicationAllowsCaseEvidence(policy TurnPublicationPolicyV1) bool {
	return ValidateTurnPublicationPolicyV1(policy) == nil && policy.RiskClass == RiskClassCase &&
		policy.Disposition == PublicationDispositionCaseEvidenceGate
}

func TurnPublicationAllowsExecution(policy TurnPublicationPolicyV1) bool {
	return ValidateTurnPublicationPolicyV1(policy) == nil && policy.Disposition != PublicationDispositionCaseBoundaryOnly
}

func validateThreadRiskPolicyUnsigned(policy ThreadRiskPolicyV1) error {
	if policy.SchemaVersion != ThreadRiskPolicySchemaVersion || policy.Purpose != ThreadRiskPolicyPurpose ||
		strings.TrimSpace(policy.ThreadID) == "" || strings.TrimSpace(policy.ThreadID) != policy.ThreadID ||
		strings.TrimSpace(policy.WorkspaceRealPath) == "" || strings.TrimSpace(policy.WorkspaceRealPath) != policy.WorkspaceRealPath ||
		!isCanonicalSHA256Hex(policy.SignalsDigest) ||
		(policy.PredecessorPolicyDigest != "" && !isCanonicalSHA256Hex(policy.PredecessorPolicyDigest)) ||
		policy.AuthorityAlgorithm != ThreadRiskPolicyAlgorithm || !isCanonicalSHA256Hex(policy.AuthorityKeyID) {
		return errors.New("thread risk policy is incomplete")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, policy.IssuedAt)
	if err != nil || issuedAt.UTC().Format(time.RFC3339Nano) != policy.IssuedAt {
		return errors.New("thread risk policy issuedAt is invalid")
	}
	if !validRiskPolicyOrigin(policy.Origin) {
		return errors.New("thread risk policy origin is invalid")
	}
	switch policy.RiskClass {
	case RiskClassGeneral:
		if policy.Origin != RiskPolicyOriginGeneralWorkspace && policy.Origin != RiskPolicyOriginLegacyMigration {
			return errors.New("general thread risk policy origin is invalid")
		}
	case RiskClassCase:
		if policy.Origin == RiskPolicyOriginGeneralWorkspace {
			return errors.New("case thread risk policy origin is invalid")
		}
	default:
		return errors.New("thread risk class is invalid")
	}
	return nil
}

func isCanonicalSHA256Hex(value string) bool {
	return value == strings.TrimSpace(value) && IsSHA256Hex(value)
}

func validRiskPolicyOrigin(origin string) bool {
	switch origin {
	case RiskPolicyOriginGeneralWorkspace, RiskPolicyOriginDesktopCaseEntry, RiskPolicyOriginBindingMarkerPresent,
		RiskPolicyOriginValidCaseBinding, RiskPolicyOriginSignedCaseLineage, RiskPolicyOriginTrustedCaseFinal,
		RiskPolicyOriginLegacyMigration, RiskPolicyOriginLexicalGuard, RiskPolicyOriginProtectedDataGuard,
		RiskPolicyOriginHostInputContextChange:
		return true
	default:
		return false
	}
}

func expectedPublicationBlocker(bindingState string) string {
	switch bindingState {
	case CaseBindingStateMissing:
		return PublicationBlockerCaseBindingMissing
	case CaseBindingStateInvalid:
		return PublicationBlockerCaseBindingInvalid
	case CaseBindingStateUnreadable:
		return PublicationBlockerCaseBindingUnreadable
	case CaseBindingStateUnstable:
		return PublicationBlockerCaseBindingUnstable
	case CaseBindingStateChangedUnaccepted:
		return PublicationBlockerCaseBindingChanged
	case CaseBindingStateWorkspaceMissing:
		return PublicationBlockerCaseWorkspaceUnavailable
	case CaseBindingStatePolicyCorrupt:
		return PublicationBlockerCasePolicyCorrupt
	default:
		return ""
	}
}

func validCaseBoundaryBlocker(bindingState, blocker string) bool {
	if expected := expectedPublicationBlocker(bindingState); expected != "" && blocker == expected {
		return true
	}
	switch blocker {
	case PublicationBlockerRiskAuthorityUnavailable,
		PublicationBlockerRiskAuthorityIndeterminate,
		PublicationBlockerRiskAuthorityInconsistent:
		return bindingState == CaseBindingStateValid || expectedPublicationBlocker(bindingState) != ""
	case PublicationBlockerDatasetSnapshotUnavailable,
		PublicationBlockerDatasetSnapshotCorrupt,
		PublicationBlockerDatasetSnapshotMismatch,
		PublicationBlockerDatasetSnapshotStale:
		return bindingState == CaseBindingStateValid
	default:
		return false
	}
}

func threadRiskPolicyDigestV1(policy ThreadRiskPolicyV1) string {
	policy.PolicyDigest = ""
	body, _ := json.Marshal(policy)
	return SHA256Hex(body)
}

func turnPublicationPolicyDigestV1(policy TurnPublicationPolicyV1) string {
	policy.PolicyDigest = ""
	body, _ := json.Marshal(policy)
	return SHA256Hex(body)
}

func decodeStrictRiskContract(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 * 1024, MaxDepth: 16, MaxTokens: 2_000, MaxStringBytes: 8 * 1024,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("risk policy contains trailing JSON")
	}
	return nil
}
