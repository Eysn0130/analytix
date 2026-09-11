package security

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	TurnSecurityContextVersionV1 = 1
	TurnSecurityContextVersionV2 = 2
	ExecutionGrantVersion        = 1

	// ContractVersion is retained for source compatibility with existing grant
	// call sites. TurnSecurityContext has its own independently versioned
	// contract and new execution authority requires V2.
	ContractVersion = ExecutionGrantVersion
)

const (
	LocalTenantID          = "local"
	LocalUserID            = "local"
	UnboundCaseID          = "unbound"
	NoDatasetSnapshotID    = "none"
	UnresolvedSnapshotMark = "unresolved:"
)

var EmptySourceManifestHash = SHA256Hex([]byte("[]"))

type CaseBinding struct {
	Version           int
	CaseID            string
	WorkspaceRealPath string
	BindingPath       string
	BindingSHA256     string
	CaseBindingHash   string
}

type TurnSecurityContext struct {
	Version              int                     `json:"version"`
	ThreadID             string                  `json:"threadId"`
	TurnID               string                  `json:"turnId"`
	WorkspaceRealPath    string                  `json:"workspaceRealPath"`
	TenantID             string                  `json:"tenantId"`
	UserID               string                  `json:"userId"`
	CaseID               string                  `json:"caseId"`
	CaseBindingHash      string                  `json:"caseBindingHash"`
	DatasetSnapshotID    string                  `json:"datasetSnapshotId"`
	SourceManifestHash   string                  `json:"sourceManifestHash"`
	ContextEpoch         uint64                  `json:"contextEpoch"`
	IssuedAt             string                  `json:"issuedAt"`
	ContextDigest        string                  `json:"contextDigest"`
	PublicationPolicy    TurnPublicationPolicyV1 `json:"-"`
	RiskAuthorityBinding RiskAuthorityBindingV1  `json:"-"`
}

type ExecutionGrant struct {
	Version         int    `json:"version"`
	GrantID         string `json:"grantId"`
	TurnID          string `json:"turnId"`
	ContextDigest   string `json:"contextDigest"`
	Provider        string `json:"provider"`
	ServerIdentity  string `json:"serverIdentity"`
	ToolName        string `json:"toolName"`
	ToolCallID      string `json:"toolCallId"`
	ConnectionEpoch uint64 `json:"connectionEpoch"`
	ArgsHash        string `json:"argsHash"`
	SchemaHash      string `json:"schemaHash"`
	ScopeHash       string `json:"scopeHash"`
	ReadOnly        bool   `json:"readOnly"`
	ApprovalState   string `json:"approvalState"`
	IssuedAt        string `json:"issuedAt"`
	ExpiresAt       string `json:"expiresAt"`
}

type TurnSecurityContextInput struct {
	ThreadID             string
	TurnID               string
	WorkspaceRealPath    string
	TenantID             string
	UserID               string
	CaseID               string
	CaseBindingHash      string
	DatasetSnapshotID    string
	SourceManifestHash   string
	ContextEpoch         uint64
	IssuedAt             time.Time
	PublicationPolicy    TurnPublicationPolicyV1
	RiskAuthorityBinding RiskAuthorityBindingV1
}

type ExecutionGrantInput struct {
	Context         TurnSecurityContext
	Provider        string
	ServerIdentity  string
	ToolName        string
	ToolCallID      string
	ConnectionEpoch uint64
	ArgsHash        string
	SchemaHash      string
	ScopeHash       string
	ReadOnly        bool
	ApprovalState   string
	IssuedAt        time.Time
	ExpiresAt       time.Time
}

// NewTurnSecurityContext constructs the legacy V1 audit contract. It remains
// temporarily available so unmigrated consumers compile, but its output is
// rejected by ValidateTurnSecurityContextForExecution. New execution paths
// must call NewTurnSecurityContextV2 with an explicit publication policy.
func NewTurnSecurityContext(input TurnSecurityContextInput) TurnSecurityContext {
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	epoch := input.ContextEpoch
	if epoch == 0 {
		epoch = 1
	}
	workspaceRealPath := strings.TrimSpace(input.WorkspaceRealPath)
	tenantID := strings.TrimSpace(input.TenantID)
	if tenantID == "" {
		tenantID = LocalTenantID
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		userID = LocalUserID
	}
	caseID := strings.TrimSpace(input.CaseID)
	caseBindingHash := strings.TrimSpace(input.CaseBindingHash)
	if caseID == "" {
		caseID = UnboundCaseID
		caseBindingHash = UnboundCaseBindingHash(workspaceRealPath)
	}
	datasetSnapshotID := strings.TrimSpace(input.DatasetSnapshotID)
	if datasetSnapshotID == "" {
		if caseID == UnboundCaseID {
			datasetSnapshotID = NoDatasetSnapshotID
		} else {
			datasetSnapshotID = UnresolvedSnapshotMark + caseBindingHash
		}
	}
	sourceManifestHash := strings.TrimSpace(input.SourceManifestHash)
	if sourceManifestHash == "" {
		sourceManifestHash = EmptySourceManifestHash
	}
	context := TurnSecurityContext{
		Version:            TurnSecurityContextVersionV1,
		ThreadID:           strings.TrimSpace(input.ThreadID),
		TurnID:             strings.TrimSpace(input.TurnID),
		WorkspaceRealPath:  workspaceRealPath,
		TenantID:           tenantID,
		UserID:             userID,
		CaseID:             caseID,
		CaseBindingHash:    caseBindingHash,
		DatasetSnapshotID:  datasetSnapshotID,
		SourceManifestHash: sourceManifestHash,
		ContextEpoch:       epoch,
		IssuedAt:           issuedAt.Format(time.RFC3339Nano),
	}
	context.ContextDigest = hashContract(context)
	return context
}

// NewTurnSecurityContextV2 requires explicit host publication and risk
// authority bindings. It does not infer a general policy from missing fields,
// because that would turn absence into authority.
func NewTurnSecurityContextV2(input TurnSecurityContextInput) (TurnSecurityContext, error) {
	if ValidateTurnPublicationPolicyV1(input.PublicationPolicy) != nil {
		return TurnSecurityContext{}, errors.New("turn security context publication policy is invalid")
	}
	if ValidateRiskAuthorityBindingV1(input.RiskAuthorityBinding) != nil {
		return TurnSecurityContext{}, errors.New("turn security context risk authority binding is invalid")
	}
	if input.IssuedAt.IsZero() || input.ContextEpoch == 0 || strings.TrimSpace(input.ThreadID) == "" ||
		strings.TrimSpace(input.TurnID) == "" || strings.TrimSpace(input.WorkspaceRealPath) == "" ||
		strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.UserID) == "" {
		return TurnSecurityContext{}, errors.New("turn security context V2 input is incomplete")
	}
	context := TurnSecurityContext{
		Version:              TurnSecurityContextVersionV2,
		ThreadID:             strings.TrimSpace(input.ThreadID),
		TurnID:               strings.TrimSpace(input.TurnID),
		WorkspaceRealPath:    strings.TrimSpace(input.WorkspaceRealPath),
		TenantID:             strings.TrimSpace(input.TenantID),
		UserID:               strings.TrimSpace(input.UserID),
		CaseID:               strings.TrimSpace(input.CaseID),
		CaseBindingHash:      strings.TrimSpace(input.CaseBindingHash),
		DatasetSnapshotID:    strings.TrimSpace(input.DatasetSnapshotID),
		SourceManifestHash:   strings.TrimSpace(input.SourceManifestHash),
		ContextEpoch:         input.ContextEpoch,
		IssuedAt:             input.IssuedAt.UTC().Format(time.RFC3339Nano),
		PublicationPolicy:    input.PublicationPolicy,
		RiskAuthorityBinding: input.RiskAuthorityBinding,
	}
	context.ContextDigest = hashContract(context)
	if err := ValidateTurnSecurityContext(context); err != nil {
		return TurnSecurityContext{}, err
	}
	return context, nil
}

func NewExecutionGrant(input ExecutionGrantInput) ExecutionGrant {
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	expiresAt := input.ExpiresAt.UTC()
	if expiresAt.IsZero() || !expiresAt.After(issuedAt) {
		expiresAt = issuedAt.Add(15 * time.Minute)
	}
	grant := ExecutionGrant{
		Version:         ExecutionGrantVersion,
		TurnID:          strings.TrimSpace(input.Context.TurnID),
		ContextDigest:   strings.TrimSpace(input.Context.ContextDigest),
		Provider:        strings.TrimSpace(input.Provider),
		ServerIdentity:  strings.TrimSpace(input.ServerIdentity),
		ToolName:        strings.TrimSpace(input.ToolName),
		ToolCallID:      strings.TrimSpace(input.ToolCallID),
		ConnectionEpoch: input.ConnectionEpoch,
		ArgsHash:        strings.TrimSpace(input.ArgsHash),
		SchemaHash:      strings.TrimSpace(input.SchemaHash),
		ScopeHash:       strings.TrimSpace(input.ScopeHash),
		ReadOnly:        input.ReadOnly,
		ApprovalState:   strings.TrimSpace(input.ApprovalState),
		IssuedAt:        issuedAt.Format(time.RFC3339Nano),
		ExpiresAt:       expiresAt.Format(time.RFC3339Nano),
	}
	grant.GrantID = hashContract(grant)
	return grant
}

func ParseTurnSecurityContext(value any) (TurnSecurityContext, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return TurnSecurityContext{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var context TurnSecurityContext
	if err := decoder.Decode(&context); err != nil {
		return TurnSecurityContext{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return TurnSecurityContext{}, errors.New("turn security context contains trailing JSON")
	}
	if err := ValidateTurnSecurityContext(context); err != nil {
		return TurnSecurityContext{}, err
	}
	return context, nil
}

func ParseExecutionGrant(value any) (ExecutionGrant, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ExecutionGrant{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var grant ExecutionGrant
	if err := decoder.Decode(&grant); err != nil {
		return ExecutionGrant{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ExecutionGrant{}, errors.New("execution grant contains trailing JSON")
	}
	if err := ValidateExecutionGrant(grant); err != nil {
		return ExecutionGrant{}, err
	}
	return grant, nil
}

func ValidateExecutionGrant(grant ExecutionGrant) error {
	if err := validateExecutionGrantForAudit(grant); err != nil {
		return err
	}
	if !IsHostToolCallIDV1(grant.ToolCallID) {
		return errors.New("execution grant tool-call identity is invalid")
	}
	return nil
}

// ValidateExecutionGrantForAudit validates the immutable integrity and typed
// bindings of a historical grant without restoring its authority to execute.
// Provider-originated call IDs are accepted only here so signed legacy records
// remain inspectable during migration; every execution boundary must use
// ValidateExecutionGrant or ValidateExecutionGrantForContext instead.
func ValidateExecutionGrantForAudit(grant ExecutionGrant) error {
	return validateExecutionGrantForAudit(grant)
}

func validateExecutionGrantForAudit(grant ExecutionGrant) error {
	if grant.Version != ExecutionGrantVersion || !isSHA256Hex(grant.GrantID) || grant.GrantID != executionGrantHash(grant) {
		return errors.New("execution grant integrity is invalid")
	}
	if strings.TrimSpace(grant.TurnID) == "" || !isSHA256Hex(grant.ContextDigest) || strings.TrimSpace(grant.Provider) == "" ||
		strings.TrimSpace(grant.ServerIdentity) == "" || strings.TrimSpace(grant.ToolName) == "" || strings.TrimSpace(grant.ToolCallID) == "" ||
		!isSHA256Hex(grant.ArgsHash) || !isSHA256Hex(grant.SchemaHash) || !isSHA256Hex(grant.ScopeHash) {
		return errors.New("execution grant is incomplete")
	}
	if len(grant.ToolCallID) > 512 || grant.ToolCallID != strings.TrimSpace(grant.ToolCallID) || !utf8.ValidString(grant.ToolCallID) {
		return errors.New("execution grant audit tool-call identity is invalid")
	}
	for _, char := range grant.ToolCallID {
		if char < 0x20 || char == 0x7f {
			return errors.New("execution grant audit tool-call identity is invalid")
		}
	}
	switch grant.ApprovalState {
	case "not_required", "pending", "approved":
	default:
		return errors.New("execution grant approval state is invalid")
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if issuedErr != nil || expiresErr != nil || !expiresAt.After(issuedAt) {
		return errors.New("execution grant time range is invalid")
	}
	if grant.ServerIdentity == "host:builtin" {
		if grant.ConnectionEpoch != 0 {
			return errors.New("execution grant host connection epoch is invalid")
		}
	} else if identity, err := ParseVerifiedMCPServerIdentity(grant.ServerIdentity); err != nil ||
		!VerifiedMCPServerIdentityCanAuthorizeFacts(identity) ||
		identity.ConnectionEpoch != grant.ConnectionEpoch {
		return errors.New("execution grant MCP server identity is invalid")
	}
	return nil
}

// ValidateExecutionGrantForContext is the execution-boundary validator. A
// syntactically valid V1 grant cannot turn a legacy audit context into current
// execution authority, and a grant cannot be replayed against another turn or
// context digest.
func ValidateExecutionGrantForContext(grant ExecutionGrant, context TurnSecurityContext) error {
	if err := ValidateTurnSecurityContextForExecution(context); err != nil {
		return err
	}
	return validateExecutionGrantContextBinding(grant, context)
}

// ValidateExecutionGrantForOrdinaryContext binds an ordinary-effect grant to
// the same exact turn without requiring case-fact execution authority.
// Callers must still classify the concrete tool before choosing this helper.
func ValidateExecutionGrantForOrdinaryContext(grant ExecutionGrant, context TurnSecurityContext) error {
	if err := ValidateTurnSecurityContextForOrdinaryEffect(context); err != nil {
		return err
	}
	return validateExecutionGrantContextBinding(grant, context)
}

func validateExecutionGrantContextBinding(grant ExecutionGrant, context TurnSecurityContext) error {
	if err := ValidateExecutionGrant(grant); err != nil {
		return err
	}
	if grant.TurnID != context.TurnID || grant.ContextDigest != context.ContextDigest {
		return errors.New("execution grant context binding is invalid")
	}
	return nil
}

func ValidateTurnSecurityContext(context TurnSecurityContext) error {
	if context.Version != TurnSecurityContextVersionV1 && context.Version != TurnSecurityContextVersionV2 {
		return errors.New("turn security context version is invalid")
	}
	if strings.TrimSpace(context.ThreadID) == "" || strings.TrimSpace(context.TurnID) == "" ||
		strings.TrimSpace(context.WorkspaceRealPath) == "" || strings.TrimSpace(context.TenantID) == "" || strings.TrimSpace(context.UserID) == "" ||
		strings.TrimSpace(context.CaseID) == "" || !isSHA256Hex(context.CaseBindingHash) || strings.TrimSpace(context.DatasetSnapshotID) == "" ||
		!isSHA256Hex(context.SourceManifestHash) || context.ContextEpoch == 0 || strings.TrimSpace(context.IssuedAt) == "" ||
		strings.TrimSpace(context.ContextDigest) == "" {
		return errors.New("turn security context is incomplete")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, context.IssuedAt)
	if err != nil {
		return errors.New("turn security context issuedAt is invalid")
	}
	if context.Version == TurnSecurityContextVersionV1 {
		if context.PublicationPolicy != (TurnPublicationPolicyV1{}) || context.RiskAuthorityBinding != (RiskAuthorityBindingV1{}) {
			return errors.New("turn security context V1 contains V2 authority fields")
		}
	} else {
		if issuedAt.UTC().Format(time.RFC3339Nano) != context.IssuedAt {
			return errors.New("turn security context V2 issuedAt is not canonical UTC")
		}
		if !turnSecurityContextV2StringsAreCanonical(context) {
			return errors.New("turn security context V2 fields are not canonical")
		}
		if err := validateTurnSecurityContextV2Policy(context); err != nil {
			return err
		}
	}
	expected := context
	expected.ContextDigest = ""
	if context.ContextDigest != hashContract(expected) {
		return errors.New("turn security context integrity is invalid")
	}
	return nil
}

func turnSecurityContextV2StringsAreCanonical(context TurnSecurityContext) bool {
	for _, value := range []string{
		context.ThreadID, context.TurnID, context.WorkspaceRealPath, context.TenantID, context.UserID,
		context.CaseID, context.CaseBindingHash, context.DatasetSnapshotID, context.SourceManifestHash,
		context.IssuedAt, context.ContextDigest,
	} {
		if value != strings.TrimSpace(value) {
			return false
		}
	}
	return true
}

// ValidateTurnSecurityContextForExecution is the sole domain validator for a
// new provider or tool execution. Legacy V1 contexts remain parseable for
// audit/migration, while boundary-only V2 contexts may be finalized by the
// host but cannot authorize provider or tool work. This structural domain
// check does not replace the app's required current registry membership and
// current-run witness observation validation for RiskAuthorityBinding.
func ValidateTurnSecurityContextForExecution(context TurnSecurityContext) error {
	if err := ValidateTurnSecurityContext(context); err != nil {
		return err
	}
	if context.Version != TurnSecurityContextVersionV2 {
		return errors.New("turn security context V1 is audit-only")
	}
	if !TurnPublicationAllowsExecution(context.PublicationPolicy) {
		return errors.New("turn security context is boundary-only")
	}
	if context.RiskAuthorityBinding.State != RiskAuthorityBindingStateWitnessed &&
		context.RiskAuthorityBinding.State != RiskAuthorityBindingStateHostPolicy &&
		!(context.RiskAuthorityBinding.State == RiskAuthorityBindingStateHostGeneralOnly && TurnSecurityContextIsGeneral(context)) {
		return errors.New("turn security context risk authority is quarantined")
	}
	return nil
}

// ValidateTurnSecurityContextForOrdinaryEffect is the structural admission
// rule for the permanent ordinary Agent capability base. It deliberately does
// not turn a boundary-only context into case-data or case-publication
// authority: it permits that context only when either the independently
// witnessed risk authority or the exact host general-only policy remains
// present. Audit-only V1 and quarantined risk authority remain
// non-executable.
func ValidateTurnSecurityContextForOrdinaryEffect(context TurnSecurityContext) error {
	if err := ValidateTurnSecurityContext(context); err != nil {
		return err
	}
	if context.Version != TurnSecurityContextVersionV2 {
		return errors.New("turn security context V1 is audit-only")
	}
	if TurnSecurityContextIsBoundaryOnly(context) {
		if context.RiskAuthorityBinding.State != RiskAuthorityBindingStateWitnessed &&
			context.RiskAuthorityBinding.State != RiskAuthorityBindingStateHostPolicy &&
			context.RiskAuthorityBinding.State != RiskAuthorityBindingStateHostGeneralOnly {
			return errors.New("turn security context risk authority is quarantined")
		}
		return nil
	}
	return ValidateTurnSecurityContextForExecution(context)
}

// ValidateTurnSecurityContextForCasePublication is the structural admission
// rule for a newly persisted case final. It permits both evidence-capable and
// boundary-only V2 contexts, but never lets an audit-only V1 context re-enter
// the live publication path.
func ValidateTurnSecurityContextForCasePublication(context TurnSecurityContext) error {
	if err := ValidateTurnSecurityContext(context); err != nil {
		return err
	}
	if context.Version != TurnSecurityContextVersionV2 {
		return errors.New("turn security context V1 is audit-only")
	}
	if !TurnPublicationRequiresFinalEvidenceGate(context.PublicationPolicy) {
		return errors.New("turn security context does not authorize case publication")
	}
	return nil
}

// ValidateTurnSecurityContextForCaseFactPublication is the structural rule
// for claims, receipts, verified no-hit results, and publication proofs. A
// boundary-only V2 context may persist a fixed host boundary but can never
// acquire fact authority.
func ValidateTurnSecurityContextForCaseFactPublication(context TurnSecurityContext) error {
	if err := ValidateTurnSecurityContextForCasePublication(context); err != nil {
		return err
	}
	if !TurnSecurityContextAllowsCaseEvidence(context) {
		return errors.New("turn security context does not authorize case facts")
	}
	return nil
}

func TurnSecurityContextRequiresFinalEvidenceGate(context TurnSecurityContext) bool {
	return ValidateTurnSecurityContext(context) == nil && context.Version == TurnSecurityContextVersionV2 &&
		TurnPublicationRequiresFinalEvidenceGate(context.PublicationPolicy)
}

// TurnSecurityContextIsCaseSensitive is the common conservative classifier
// for publication, persistence, replay, compaction, and recovery boundaries.
// Current V2 boundary-only contexts intentionally carry the unbound case
// sentinel, while historical V1 case contexts carry a concrete case id. Both
// require case protections; neither may fall through an ordinary output path.
func TurnSecurityContextIsCaseSensitive(context TurnSecurityContext) bool {
	if ValidateTurnSecurityContext(context) != nil {
		return false
	}
	return TurnSecurityContextRequiresFinalEvidenceGate(context) || context.CaseID != UnboundCaseID
}

func TurnSecurityContextIsGeneral(context TurnSecurityContext) bool {
	return ValidateTurnSecurityContext(context) == nil && context.Version == TurnSecurityContextVersionV2 &&
		context.PublicationPolicy.Disposition == PublicationDispositionGeneralOutput
}

// TurnSecurityContextUsesHostGeneralOnlyRisk identifies the deliberately
// non-elevating production fallback. It can authorize the permanent ordinary
// Agent tool base for a currently non-case workspace, but never case evidence,
// protected snapshots, or controlled publication.
func TurnSecurityContextUsesHostGeneralOnlyRisk(context TurnSecurityContext) bool {
	return ValidateTurnSecurityContext(context) == nil && context.Version == TurnSecurityContextVersionV2 &&
		context.PublicationPolicy.Disposition == PublicationDispositionGeneralOutput &&
		context.RiskAuthorityBinding.State == RiskAuthorityBindingStateHostGeneralOnly
}

// TurnSecurityContextUsesHostRiskPolicy identifies the first-stage
// installation-signed case-risk variant. It must still be revalidated by the
// host at every concrete effect and carries no independent monotonic-witness
// claim.
func TurnSecurityContextUsesHostRiskPolicy(context TurnSecurityContext) bool {
	return ValidateTurnSecurityContext(context) == nil && context.Version == TurnSecurityContextVersionV2 &&
		context.RiskAuthorityBinding.State == RiskAuthorityBindingStateHostPolicy
}

func TurnSecurityContextIsBoundaryOnly(context TurnSecurityContext) bool {
	return ValidateTurnSecurityContext(context) == nil && context.Version == TurnSecurityContextVersionV2 &&
		context.PublicationPolicy.Disposition == PublicationDispositionCaseBoundaryOnly
}

func TurnSecurityContextAllowsCaseEvidence(context TurnSecurityContext) bool {
	return ValidateTurnSecurityContextForExecution(context) == nil &&
		TurnPublicationAllowsCaseEvidence(context.PublicationPolicy)
}

func validateTurnSecurityContextV2Policy(context TurnSecurityContext) error {
	if err := ValidateTurnPublicationPolicyV1(context.PublicationPolicy); err != nil {
		return errors.New("turn security context publication policy is invalid")
	}
	if err := ValidateRiskAuthorityBindingV1(context.RiskAuthorityBinding); err != nil {
		return errors.New("turn security context risk authority binding is invalid")
	}
	unboundHash := UnboundCaseBindingHash(context.WorkspaceRealPath)
	switch context.PublicationPolicy.Disposition {
	case PublicationDispositionGeneralOutput:
		if context.RiskAuthorityBinding.State != RiskAuthorityBindingStateWitnessed &&
			context.RiskAuthorityBinding.State != RiskAuthorityBindingStateHostGeneralOnly {
			return errors.New("general turn security context requires current risk authority")
		}
		if context.CaseID != UnboundCaseID || context.CaseBindingHash != unboundHash || context.DatasetSnapshotID != NoDatasetSnapshotID ||
			context.SourceManifestHash != EmptySourceManifestHash {
			return errors.New("general turn security context case binding is invalid")
		}
		if context.RiskAuthorityBinding.State == RiskAuthorityBindingStateHostGeneralOnly {
			policy, err := NewGeneralOnlyRiskPolicyV1(
				context.ThreadID, context.WorkspaceRealPath, context.PublicationPolicy.BindingObservationDigest,
			)
			if err != nil || ValidateHostGeneralOnlyRiskAuthorityBindingV1(context.RiskAuthorityBinding, policy) != nil ||
				ValidateTurnPublicationPolicyForGeneralOnlyRiskPolicyV1(context.PublicationPolicy, policy) != nil {
				return errors.New("general turn security context host authority is invalid")
			}
		}
	case PublicationDispositionCaseEvidenceGate:
		if context.RiskAuthorityBinding.State != RiskAuthorityBindingStateWitnessed &&
			context.RiskAuthorityBinding.State != RiskAuthorityBindingStateHostPolicy {
			return errors.New("case evidence turn security context requires current risk authority")
		}
		if context.CaseID == UnboundCaseID || context.CaseBindingHash == unboundHash || !IsDatasetSnapshotIDV2Syntax(context.DatasetSnapshotID) {
			return errors.New("case evidence turn security context binding is invalid")
		}
	case PublicationDispositionCaseBoundaryOnly:
		if context.CaseID != UnboundCaseID || context.CaseBindingHash != unboundHash || context.DatasetSnapshotID != NoDatasetSnapshotID ||
			context.SourceManifestHash != EmptySourceManifestHash {
			return errors.New("case boundary turn security context binding is invalid")
		}
	default:
		return errors.New("turn security context publication disposition is invalid")
	}
	return nil
}

type turnSecurityContextV1Wire struct {
	Version            int    `json:"version"`
	ThreadID           string `json:"threadId"`
	TurnID             string `json:"turnId"`
	WorkspaceRealPath  string `json:"workspaceRealPath"`
	TenantID           string `json:"tenantId"`
	UserID             string `json:"userId"`
	CaseID             string `json:"caseId"`
	CaseBindingHash    string `json:"caseBindingHash"`
	DatasetSnapshotID  string `json:"datasetSnapshotId"`
	SourceManifestHash string `json:"sourceManifestHash"`
	ContextEpoch       uint64 `json:"contextEpoch"`
	IssuedAt           string `json:"issuedAt"`
	ContextDigest      string `json:"contextDigest"`
}

type turnSecurityContextV2Wire struct {
	turnSecurityContextV1Wire
	PublicationPolicy    TurnPublicationPolicyV1 `json:"publicationPolicy"`
	RiskAuthorityBinding RiskAuthorityBindingV1  `json:"riskAuthorityBinding"`
}

func (context TurnSecurityContext) MarshalJSON() ([]byte, error) {
	base := turnSecurityContextV1Wire{
		Version: context.Version, ThreadID: context.ThreadID, TurnID: context.TurnID,
		WorkspaceRealPath: context.WorkspaceRealPath, TenantID: context.TenantID, UserID: context.UserID,
		CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash, DatasetSnapshotID: context.DatasetSnapshotID,
		SourceManifestHash: context.SourceManifestHash, ContextEpoch: context.ContextEpoch, IssuedAt: context.IssuedAt,
		ContextDigest: context.ContextDigest,
	}
	if context.Version == TurnSecurityContextVersionV1 {
		return json.Marshal(base)
	}
	return json.Marshal(turnSecurityContextV2Wire{
		turnSecurityContextV1Wire: base, PublicationPolicy: context.PublicationPolicy,
		RiskAuthorityBinding: context.RiskAuthorityBinding,
	})
}

func (context *TurnSecurityContext) UnmarshalJSON(body []byte) error {
	if context == nil {
		return errors.New("turn security context target is nil")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 128 * 1024, MaxDepth: 24, MaxTokens: 4_000, MaxStringBytes: 16 * 1024,
	}); err != nil {
		return err
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return err
	}
	switch header.Version {
	case TurnSecurityContextVersionV1:
		var wire turnSecurityContextV1Wire
		if err := decodeTurnSecurityContextWire(body, &wire); err != nil {
			return err
		}
		*context = turnSecurityContextFromV1Wire(wire)
	case TurnSecurityContextVersionV2:
		var wire turnSecurityContextV2Wire
		if err := decodeTurnSecurityContextWire(body, &wire); err != nil {
			return err
		}
		*context = turnSecurityContextFromV1Wire(wire.turnSecurityContextV1Wire)
		context.PublicationPolicy = wire.PublicationPolicy
		context.RiskAuthorityBinding = wire.RiskAuthorityBinding
	default:
		return errors.New("turn security context version is invalid")
	}
	return nil
}

func decodeTurnSecurityContextWire(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("turn security context contains trailing JSON")
	}
	return nil
}

func turnSecurityContextFromV1Wire(wire turnSecurityContextV1Wire) TurnSecurityContext {
	return TurnSecurityContext{
		Version: wire.Version, ThreadID: wire.ThreadID, TurnID: wire.TurnID, WorkspaceRealPath: wire.WorkspaceRealPath,
		TenantID: wire.TenantID, UserID: wire.UserID, CaseID: wire.CaseID, CaseBindingHash: wire.CaseBindingHash,
		DatasetSnapshotID: wire.DatasetSnapshotID, SourceManifestHash: wire.SourceManifestHash,
		ContextEpoch: wire.ContextEpoch, IssuedAt: wire.IssuedAt, ContextDigest: wire.ContextDigest,
	}
}

func SHA256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func IsSHA256Hex(value string) bool {
	return isSHA256Hex(value)
}

func UnboundCaseBindingHash(workspaceRealPath string) string {
	return SHA256Hex([]byte("case-binding:none\x00" + strings.TrimSpace(workspaceRealPath)))
}

func executionGrantHash(grant ExecutionGrant) string {
	grant.GrantID = ""
	return hashContract(grant)
}

// DecodeCanonicalJSONValue applies the exact input limits and strict decoder
// used by execution-grant argument authority.
func DecodeCanonicalJSONValue(raw []byte) (any, error) {
	return domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
}

// DecodeCanonicalJSONObject is the shared strict argument decoder for host
// tool execution and the side-effect identity derived from that execution.
func DecodeCanonicalJSONObject(raw []byte) (map[string]any, error) {
	value, err := DecodeCanonicalJSONValue(raw)
	if err != nil {
		return nil, err
	}
	_, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("canonical JSON value must be an object")
	}
	// Host builtin parsers historically consume encoding/json values, including
	// float64 numbers. Decode only after strict validation so execution and the
	// semantic side-effect projection see the same concrete argument types.
	object := map[string]any{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	return object, nil
}

// CanonicalJSONHash binds an execution grant to the exact JSON value supplied
// by the provider while treating insignificant JSON whitespace as equivalent.
// Invalid or trailing JSON fails closed.
func CanonicalJSONHash(raw []byte) string {
	value, err := DecodeCanonicalJSONValue(raw)
	if err != nil {
		return ""
	}
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func hashContract(value any) string {
	body, _ := json.Marshal(value)
	return SHA256Hex(body)
}

func isSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
