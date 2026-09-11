package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"time"

	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

const FinalAnswerEnvelopeVersion = 1

type FinalAnswerVariant string

const (
	EvidenceBackedAnswer    FinalAnswerVariant = "EvidenceBackedAnswer"
	PartialEvidenceAnswer   FinalAnswerVariant = "PartialEvidenceAnswer"
	VerifiedNoHitAnswer     FinalAnswerVariant = "VerifiedNoHitAnswer"
	SourceUnavailableAnswer FinalAnswerVariant = "SourceUnavailableAnswer"
	NeedsEvidenceAnswer     FinalAnswerVariant = "NeedsEvidenceAnswer"
	GeneralGuidanceAnswer   FinalAnswerVariant = "GeneralGuidanceAnswer"
)

func NewAccountFlowQueryScopeRefV1(contextDigest string, queryHash string) (string, error) {
	if !domainsecurity.IsSHA256Hex(contextDigest) || !domainsecurity.IsSHA256Hex(queryHash) {
		return "", errors.New("account-flow query scope input is invalid")
	}
	return "qscope1_" + domainsecurity.SHA256Hex([]byte(contextDigest+"\x00"+queryHash)), nil
}

func ValidateAccountFlowQueryScopeRefV1(reference string) error {
	if !strings.HasPrefix(reference, "qscope1_") ||
		!domainsecurity.IsSHA256Hex(strings.TrimPrefix(reference, "qscope1_")) {
		return errors.New("account-flow query scope reference is invalid")
	}
	return nil
}

const VerifiedNoHitWording = "not_found_in_checked_scope"

const OrdinaryResultOnlyGuidanceCodeV1 = "ordinary_result_only"

const (
	AccountFlowTypedSourceFieldReferenceVersionV1 = 1
	AccountFlowTypedSourceFieldReferencePurposeV1 = "analytix.account-flow-typed-source-field-reference/v1"
	AccountFlowTypedAnswerOutcomeVersionV1        = 1
	AccountFlowTypedAnswerOutcomePurposeV1        = "analytix.account-flow-typed-answer-outcome/v1"

	AccountFlowOutcomeTransportSuccessV1          = "success"
	AccountFlowOutcomeCompletenessCompleteV1      = "complete"
	AccountFlowOutcomeCompletenessIncompleteV1    = "incomplete"
	AccountFlowOutcomeTypedSlotPendingFinalGateV1 = "pending_final_gate"
	AccountFlowOutcomeTypedSlotEligibleV1         = "eligible"
	AccountFlowOutcomeDisplayPendingFinalGateV1   = "pending_final_gate"
	AccountFlowOutcomeDisplayEligibleV1           = "eligible"
	AccountFlowOutcomeDisplayNotRequestedV1       = "not_requested"
	AccountFlowOutcomeQueryScopeHostVerifiedV1    = "host_verified"
	AccountFlowOutcomeQueryScopeEligibleV1        = "eligible"
	AccountFlowOutcomeCurrentnessCurrentV1        = "current"
	AccountFlowOutcomeLineagePendingReceiptV1     = "pending_evidence_receipt"
	AccountFlowOutcomeLineageEligibleV1           = "eligible"
	AccountFlowOutcomeSourceFieldPendingReceiptV1 = "pending_evidence_receipt"
	AccountFlowOutcomeSourceFieldEligibleV1       = "eligible"
)

// AccountFlowTypedSourceFieldReferenceV1 is a provider-safe, value-free
// candidate binding. It names only the closed source-field kind and a digest
// that the host can recompute from the exact query/native-result lineage.
// The native query hash is case-authoritative: its sealed material includes
// case binding, context, snapshot, private subject resolution, and bounds, so
// identical provider-visible aggregates in different cases remain unlinkable.
// Source values, AuthorityEntityRefs, rows, locators, paths, and SQL are never
// part of this shape.
type AccountFlowTypedSourceFieldReferenceV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	BindingRef    string `json:"bindingRef"`
	Field         string `json:"field"`
}

type AccountFlowTypedAnswerGroupV1 struct {
	EvidenceReceiptID    string                                 `json:"evidenceReceiptId"`
	ClaimIDs             []string                               `json:"claimIds"`
	QueryScopeRef        string                                 `json:"queryScopeRef"`
	QueryHash            string                                 `json:"queryHash"`
	ResultHash           string                                 `json:"resultHash"`
	SourceFieldReference AccountFlowTypedSourceFieldReferenceV1 `json:"sourceFieldReference"`
}

// AccountFlowTypedAnswerOutcomeV1 is emitted only by Final Evidence Gate.
// Its independent closed fields prevent receipt, display, or provider prose
// from upgrading one another. Display remains a later local operation even
// when its value-free retained binding is eligible.
type AccountFlowTypedAnswerOutcomeV1 struct {
	SchemaVersion                 int                             `json:"schemaVersion"`
	Purpose                       string                          `json:"purpose"`
	TransportStatus               string                          `json:"transportStatus"`
	AggregateCompleteness         string                          `json:"aggregateCompleteness"`
	EvidenceRowsCompleteness      string                          `json:"evidenceRowsCompleteness"`
	TypedSlotEligibility          string                          `json:"typedSlotEligibility"`
	LocalDisplayAvailability      string                          `json:"localDisplayAvailability"`
	LocalDisplayCompletion        string                          `json:"localDisplayCompletion"`
	FactAnswerAllowed             bool                            `json:"factAnswerAllowed"`
	QueryScopeBindingEligibility  string                          `json:"queryScopeBindingEligibility"`
	CurrentnessEligibility        string                          `json:"currentnessEligibility"`
	LineageEligibility            string                          `json:"lineageEligibility"`
	SourceFieldBindingEligibility string                          `json:"sourceFieldBindingEligibility"`
	Groups                        []AccountFlowTypedAnswerGroupV1 `json:"groups"`
}

func NewAccountFlowTypedSourceFieldReferenceV1(
	queryHash string,
	resultHash string,
	field string,
) (AccountFlowTypedSourceFieldReferenceV1, error) {
	field = strings.TrimSpace(field)
	reference := AccountFlowTypedSourceFieldReferenceV1{
		SchemaVersion: AccountFlowTypedSourceFieldReferenceVersionV1,
		Purpose:       AccountFlowTypedSourceFieldReferencePurposeV1,
		Field:         field,
	}
	if !domainsecurity.IsSHA256Hex(queryHash) || !domainsecurity.IsSHA256Hex(resultHash) ||
		!IsAcceptedSlotSourceFieldV1(field) {
		return AccountFlowTypedSourceFieldReferenceV1{}, errors.New("account-flow typed source field input is invalid")
	}
	reference.BindingRef = "afslot1_" + domainsecurity.SHA256Hex([]byte(
		"analytix.account-flow-typed-source-field-reference/v1\x00"+queryHash+"\x00"+resultHash+"\x00"+field,
	))
	if ValidateAccountFlowTypedSourceFieldReferenceV1(reference, queryHash, resultHash) != nil {
		return AccountFlowTypedSourceFieldReferenceV1{}, errors.New("account-flow typed source field reference is invalid")
	}
	return reference, nil
}

func ValidateAccountFlowTypedSourceFieldReferenceV1(
	reference AccountFlowTypedSourceFieldReferenceV1,
	queryHash string,
	resultHash string,
) error {
	if reference.SchemaVersion != AccountFlowTypedSourceFieldReferenceVersionV1 ||
		reference.Purpose != AccountFlowTypedSourceFieldReferencePurposeV1 ||
		reference.Field != strings.TrimSpace(reference.Field) ||
		!IsAcceptedSlotSourceFieldV1(reference.Field) ||
		!strings.HasPrefix(reference.BindingRef, "afslot1_") ||
		!domainsecurity.IsSHA256Hex(strings.TrimPrefix(reference.BindingRef, "afslot1_")) ||
		!domainsecurity.IsSHA256Hex(queryHash) || !domainsecurity.IsSHA256Hex(resultHash) {
		return errors.New("account-flow typed source field reference is invalid")
	}
	want := "afslot1_" + domainsecurity.SHA256Hex([]byte(
		"analytix.account-flow-typed-source-field-reference/v1\x00"+queryHash+"\x00"+resultHash+"\x00"+reference.Field,
	))
	if reference.BindingRef != want {
		return errors.New("account-flow typed source field reference digest is invalid")
	}
	return nil
}

func NewAccountFlowTypedAnswerOutcomeV1(
	groups []AccountFlowTypedAnswerGroupV1,
) (AccountFlowTypedAnswerOutcomeV1, error) {
	outcome := AccountFlowTypedAnswerOutcomeV1{
		SchemaVersion:                 AccountFlowTypedAnswerOutcomeVersionV1,
		Purpose:                       AccountFlowTypedAnswerOutcomePurposeV1,
		TransportStatus:               AccountFlowOutcomeTransportSuccessV1,
		AggregateCompleteness:         AccountFlowOutcomeCompletenessCompleteV1,
		EvidenceRowsCompleteness:      AccountFlowOutcomeCompletenessCompleteV1,
		TypedSlotEligibility:          AccountFlowOutcomeTypedSlotEligibleV1,
		LocalDisplayAvailability:      AccountFlowOutcomeDisplayEligibleV1,
		LocalDisplayCompletion:        AccountFlowOutcomeDisplayNotRequestedV1,
		FactAnswerAllowed:             true,
		QueryScopeBindingEligibility:  AccountFlowOutcomeQueryScopeEligibleV1,
		CurrentnessEligibility:        AccountFlowOutcomeCurrentnessCurrentV1,
		LineageEligibility:            AccountFlowOutcomeLineageEligibleV1,
		SourceFieldBindingEligibility: AccountFlowOutcomeSourceFieldEligibleV1,
		Groups:                        append([]AccountFlowTypedAnswerGroupV1(nil), groups...),
	}
	for index := range outcome.Groups {
		outcome.Groups[index].ClaimIDs = append([]string(nil), outcome.Groups[index].ClaimIDs...)
	}
	if validateAccountFlowTypedAnswerOutcomeShapeV1(outcome) != nil {
		return AccountFlowTypedAnswerOutcomeV1{}, errors.New("account-flow typed answer outcome is invalid")
	}
	return outcome, nil
}

const (
	CaseSourceUnavailableText           = "当前案件资金分析来源在本轮未通过可用性核验，因此无法核验或发布所请求的案件事实。请重新加载资金分析插件或运行时后重试。"
	LegacyCaseUnverifiedText            = "当前答复未通过案件事实核验，其中的案件事实未予发布。请确认资金分析来源已连接，并重新发起核验。"
	CaseUnverifiedText                  = "本轮未发布任何未经核验的案件事实。若需案件事实，请确认当前资金分析来源和同案证据可用，然后重新发起核验。"
	CaseReportUnavailableText           = "正式报告未发布：当前宿主尚未签发与本轮同案证据、数据快照、逐项结论和隐私投影一致的有效发布回执。可继续进行只读核验和补证准备，但文件路径、可打开、渲染或检查状态均不构成发布证明。"
	AgentSafetyAuthorityUnavailableText = "当前 Agent 安全授权在本轮不可用，因此未执行模型或工具，也未发布任何案件事实。请重启运行时或恢复受控授权服务后重试。"
)

type FinalAnswerEnvelope struct {
	SchemaVersion      int                                `json:"schemaVersion"`
	Variant            FinalAnswerVariant                 `json:"variant"`
	ContextDigest      string                             `json:"contextDigest"`
	ContextEpoch       uint64                             `json:"contextEpoch"`
	DatasetSnapshotID  string                             `json:"datasetSnapshotId"`
	TerminalReason     string                             `json:"terminalReason"`
	OrdinaryResult     *domainordinaryresult.ResultSlotV1 `json:"ordinaryResult,omitempty"`
	AccountFlowOutcome *AccountFlowTypedAnswerOutcomeV1   `json:"accountFlowOutcome,omitempty"`
	Claims             []ClaimRecord                      `json:"claims"`
	EvidenceReceiptIDs []string                           `json:"evidenceReceiptIds"`
	CheckedScope       *EvidenceQueryRange                `json:"checkedScope,omitempty"`
	MissingScope       []string                           `json:"missingScope"`
	Blocker            string                             `json:"blocker"`
	AcquisitionSteps   []string                           `json:"acquisitionSteps"`
	Guidance           []string                           `json:"guidance"`
	NoHitWording       string                             `json:"noHitWording"`
	IssuedAt           string                             `json:"issuedAt"`
	EnvelopeDigest     string                             `json:"envelopeDigest"`
}

type FinalAnswerEnvelopeInput struct {
	Variant            FinalAnswerVariant
	Context            domainsecurity.TurnSecurityContext
	TerminalReason     string
	OrdinaryResult     *domainordinaryresult.ResultSlotV1
	AccountFlowOutcome *AccountFlowTypedAnswerOutcomeV1
	Claims             []ClaimRecord
	EvidenceReceiptIDs []string
	CheckedScope       *EvidenceQueryRange
	MissingScope       []string
	Blocker            string
	AcquisitionSteps   []string
	Guidance           []string
	NoHitWording       string
	IssuedAt           time.Time
}

func NewFinalAnswerEnvelope(input FinalAnswerEnvelopeInput) (FinalAnswerEnvelope, error) {
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(input.Context) != nil {
		return FinalAnswerEnvelope{}, errors.New("final answer context is invalid")
	}
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	envelope := FinalAnswerEnvelope{
		SchemaVersion: FinalAnswerEnvelopeVersion, Variant: input.Variant, ContextDigest: input.Context.ContextDigest,
		ContextEpoch: input.Context.ContextEpoch, DatasetSnapshotID: input.Context.DatasetSnapshotID,
		TerminalReason: strings.TrimSpace(input.TerminalReason), OrdinaryResult: cloneOrdinaryResultSlotV1(input.OrdinaryResult),
		AccountFlowOutcome: cloneAccountFlowTypedAnswerOutcomeV1(input.AccountFlowOutcome),
		Claims:             append([]ClaimRecord(nil), input.Claims...),
		EvidenceReceiptIDs: canonicalEvidenceStrings(input.EvidenceReceiptIDs), CheckedScope: cloneEvidenceQueryRange(input.CheckedScope),
		MissingScope: canonicalEvidenceStrings(input.MissingScope), Blocker: strings.TrimSpace(input.Blocker),
		AcquisitionSteps: canonicalEvidenceStrings(input.AcquisitionSteps), Guidance: canonicalEvidenceStrings(input.Guidance),
		NoHitWording: strings.TrimSpace(input.NoHitWording), IssuedAt: issuedAt.Format(time.RFC3339Nano),
	}
	if envelope.Claims == nil {
		envelope.Claims = []ClaimRecord{}
	}
	if envelope.EvidenceReceiptIDs == nil {
		envelope.EvidenceReceiptIDs = []string{}
	}
	if envelope.MissingScope == nil {
		envelope.MissingScope = []string{}
	}
	if envelope.AcquisitionSteps == nil {
		envelope.AcquisitionSteps = []string{}
	}
	if envelope.Guidance == nil {
		envelope.Guidance = []string{}
	}
	envelope.EnvelopeDigest = finalAnswerEnvelopeDigest(envelope)
	if err := ValidateFinalAnswerEnvelope(envelope); err != nil {
		return FinalAnswerEnvelope{}, err
	}
	return envelope, nil
}

func ParseFinalAnswerEnvelope(raw json.RawMessage) (FinalAnswerEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var envelope FinalAnswerEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return FinalAnswerEnvelope{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return FinalAnswerEnvelope{}, errors.New("final answer envelope contains trailing JSON")
	}
	if err := ValidateFinalAnswerEnvelope(envelope); err != nil {
		return FinalAnswerEnvelope{}, err
	}
	return envelope, nil
}

func ValidateFinalAnswerEnvelope(envelope FinalAnswerEnvelope) error {
	if envelope.SchemaVersion != FinalAnswerEnvelopeVersion || !validFinalAnswerVariant(envelope.Variant) ||
		!validSHA256(envelope.ContextDigest) || envelope.ContextEpoch == 0 || strings.TrimSpace(envelope.DatasetSnapshotID) == "" ||
		!validFinalTerminalReason(envelope.TerminalReason) || envelope.Claims == nil || envelope.EvidenceReceiptIDs == nil ||
		envelope.MissingScope == nil || envelope.AcquisitionSteps == nil || envelope.Guidance == nil ||
		strings.TrimSpace(envelope.IssuedAt) == "" || !validSHA256(envelope.EnvelopeDigest) {
		return errors.New("final answer envelope is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.IssuedAt); err != nil {
		return errors.New("final answer envelope issuedAt is invalid")
	}
	for _, claim := range envelope.Claims {
		if err := ValidateClaimRecord(claim); err != nil {
			return err
		}
	}
	if envelope.OrdinaryResult != nil && domainordinaryresult.ValidateResultSlotV1(*envelope.OrdinaryResult) != nil {
		return errors.New("final answer ordinary result is invalid")
	}
	if envelope.AccountFlowOutcome != nil && validateAccountFlowTypedAnswerOutcomeForEnvelopeV1(*envelope.AccountFlowOutcome, envelope) != nil {
		return errors.New("final answer account-flow typed outcome is invalid")
	}
	switch envelope.Variant {
	case EvidenceBackedAnswer:
		if len(envelope.Claims) == 0 || len(envelope.EvidenceReceiptIDs) == 0 || envelope.CheckedScope == nil || envelope.Blocker != "" ||
			len(envelope.MissingScope) != 0 || len(envelope.Guidance) != 0 || envelope.NoHitWording != "" {
			return errors.New("evidence-backed answer shape is invalid")
		}
		for _, claim := range envelope.Claims {
			if claim.SupportState != ClaimVerified {
				return errors.New("evidence-backed answer contains non-verified claim")
			}
		}
		if !finalAnswerReceiptSetMatchesClaims(envelope) {
			return errors.New("evidence-backed answer receipt set is not closed")
		}
	case PartialEvidenceAnswer:
		if envelope.AccountFlowOutcome != nil {
			return errors.New("partial answer cannot carry account-flow typed eligibility")
		}
		if len(envelope.Claims) == 0 || len(envelope.EvidenceReceiptIDs) == 0 || envelope.CheckedScope == nil ||
			len(envelope.MissingScope) == 0 || envelope.Blocker != "" || envelope.NoHitWording != "" {
			return errors.New("partial evidence answer shape is invalid")
		}
		for _, claim := range envelope.Claims {
			if claim.SupportState != ClaimVerified && claim.SupportState != ClaimPartial {
				return errors.New("partial evidence answer contains unsupported claim")
			}
		}
		if !finalAnswerReceiptSetMatchesClaims(envelope) {
			return errors.New("partial answer receipt set is not closed")
		}
	case VerifiedNoHitAnswer:
		if envelope.AccountFlowOutcome != nil {
			return errors.New("verified no-hit cannot carry account-flow typed eligibility")
		}
		if len(envelope.Claims) != 0 || len(envelope.EvidenceReceiptIDs) == 0 || envelope.CheckedScope == nil ||
			envelope.NoHitWording != VerifiedNoHitWording || envelope.Blocker != "" || len(envelope.MissingScope) != 0 {
			return errors.New("verified no-hit answer shape is invalid")
		}
	case SourceUnavailableAnswer:
		if envelope.AccountFlowOutcome != nil {
			return errors.New("source unavailable cannot carry account-flow typed eligibility")
		}
		if len(envelope.Claims) != 0 || len(envelope.EvidenceReceiptIDs) != 0 || envelope.Blocker == "" ||
			len(envelope.AcquisitionSteps) == 0 || envelope.NoHitWording != "" {
			return errors.New("source unavailable answer shape is invalid")
		}
	case NeedsEvidenceAnswer:
		if envelope.AccountFlowOutcome != nil {
			return errors.New("needs-evidence cannot carry account-flow typed eligibility")
		}
		if len(envelope.Claims) != 0 || len(envelope.EvidenceReceiptIDs) != 0 || len(envelope.MissingScope) == 0 ||
			len(envelope.AcquisitionSteps) == 0 || envelope.NoHitWording != "" {
			return errors.New("needs-evidence answer shape is invalid")
		}
	case GeneralGuidanceAnswer:
		if envelope.AccountFlowOutcome != nil {
			return errors.New("general guidance cannot carry account-flow typed eligibility")
		}
		if len(envelope.Claims) != 0 || len(envelope.EvidenceReceiptIDs) != 0 || envelope.CheckedScope != nil ||
			len(envelope.MissingScope) != 0 || envelope.Blocker != "" || len(envelope.Guidance) == 0 || envelope.NoHitWording != "" {
			return errors.New("general guidance answer shape is invalid")
		}
		ordinaryOnly := len(envelope.Guidance) == 1 && envelope.Guidance[0] == OrdinaryResultOnlyGuidanceCodeV1
		if ordinaryOnly && envelope.OrdinaryResult == nil {
			return errors.New("ordinary-only guidance lacks its exact ordinary result")
		}
		if !ordinaryOnly {
			for _, code := range envelope.Guidance {
				if code == OrdinaryResultOnlyGuidanceCodeV1 {
					return errors.New("ordinary-only guidance is ambiguous")
				}
			}
		}
	}
	if envelope.CheckedScope != nil {
		if err := validateEvidenceQueryRange(*envelope.CheckedScope); err != nil {
			return err
		}
	}
	if finalAnswerEnvelopeDigest(envelope) != envelope.EnvelopeDigest {
		return errors.New("final answer envelope integrity is invalid")
	}
	return nil
}

func validateAccountFlowTypedAnswerOutcomeShapeV1(outcome AccountFlowTypedAnswerOutcomeV1) error {
	if outcome.SchemaVersion != AccountFlowTypedAnswerOutcomeVersionV1 ||
		outcome.Purpose != AccountFlowTypedAnswerOutcomePurposeV1 ||
		outcome.TransportStatus != AccountFlowOutcomeTransportSuccessV1 ||
		outcome.AggregateCompleteness != AccountFlowOutcomeCompletenessCompleteV1 ||
		outcome.EvidenceRowsCompleteness != AccountFlowOutcomeCompletenessCompleteV1 ||
		outcome.TypedSlotEligibility != AccountFlowOutcomeTypedSlotEligibleV1 ||
		outcome.LocalDisplayAvailability != AccountFlowOutcomeDisplayEligibleV1 ||
		outcome.LocalDisplayCompletion != AccountFlowOutcomeDisplayNotRequestedV1 ||
		!outcome.FactAnswerAllowed ||
		outcome.QueryScopeBindingEligibility != AccountFlowOutcomeQueryScopeEligibleV1 ||
		outcome.CurrentnessEligibility != AccountFlowOutcomeCurrentnessCurrentV1 ||
		outcome.LineageEligibility != AccountFlowOutcomeLineageEligibleV1 ||
		outcome.SourceFieldBindingEligibility != AccountFlowOutcomeSourceFieldEligibleV1 ||
		len(outcome.Groups) == 0 {
		return errors.New("account-flow typed answer outcome shape is invalid")
	}
	previousReceiptID := ""
	for _, group := range outcome.Groups {
		if group.EvidenceReceiptID == "" || group.EvidenceReceiptID != strings.TrimSpace(group.EvidenceReceiptID) ||
			(previousReceiptID != "" && previousReceiptID >= group.EvidenceReceiptID) ||
			len(group.ClaimIDs) != 3 || !canonicalStrictStringsV1(group.ClaimIDs) ||
			ValidateAccountFlowQueryScopeRefV1(group.QueryScopeRef) != nil ||
			!domainsecurity.IsSHA256Hex(group.QueryHash) || !domainsecurity.IsSHA256Hex(group.ResultHash) ||
			ValidateAccountFlowTypedSourceFieldReferenceV1(group.SourceFieldReference, group.QueryHash, group.ResultHash) != nil {
			return errors.New("account-flow typed answer group is invalid")
		}
		previousReceiptID = group.EvidenceReceiptID
	}
	return nil
}

func validateAccountFlowTypedAnswerOutcomeForEnvelopeV1(
	outcome AccountFlowTypedAnswerOutcomeV1,
	envelope FinalAnswerEnvelope,
) error {
	if envelope.Variant != EvidenceBackedAnswer || validateAccountFlowTypedAnswerOutcomeShapeV1(outcome) != nil {
		return errors.New("account-flow typed answer outcome is not evidence-backed")
	}
	claimByID := make(map[string]ClaimRecord, len(envelope.Claims))
	for _, claim := range envelope.Claims {
		claimByID[claim.ClaimID] = claim
	}
	coveredClaims := make(map[string]struct{}, len(envelope.Claims))
	groupReceiptIDs := make([]string, 0, len(outcome.Groups))
	for _, group := range outcome.Groups {
		groupReceiptIDs = append(groupReceiptIDs, group.EvidenceReceiptID)
		groupClaims := make([]ClaimRecord, 0, len(group.ClaimIDs))
		for _, claimID := range group.ClaimIDs {
			claim, ok := claimByID[claimID]
			if !ok || len(claim.EvidenceIDs) != 1 || claim.EvidenceIDs[0] != group.EvidenceReceiptID {
				return errors.New("account-flow typed answer group does not bind its exact claim")
			}
			if _, duplicate := coveredClaims[claimID]; duplicate {
				return errors.New("account-flow typed answer group repeats a claim")
			}
			coveredClaims[claimID] = struct{}{}
			groupClaims = append(groupClaims, claim)
		}
		if !validAccountFlowTypedClaimGroupV1(groupClaims, group.QueryScopeRef) {
			return errors.New("account-flow typed answer group is not an exact aggregate claim group")
		}
	}
	if len(coveredClaims) != len(envelope.Claims) ||
		!reflect.DeepEqual(canonicalEvidenceStrings(groupReceiptIDs), envelope.EvidenceReceiptIDs) {
		return errors.New("account-flow typed answer outcome does not cover the exact envelope")
	}
	return nil
}

func validAccountFlowTypedClaimGroupV1(claims []ClaimRecord, queryScopeRef string) bool {
	if len(claims) != 3 {
		return false
	}
	var inflow, outflow, count *ClaimRecord
	for index := range claims {
		claim := &claims[index]
		if claim.SupportState != ClaimVerified || claim.SupportedScope == nil ||
			claim.NormalizedPayload.Granularity != accountFlowAggregateGranularity {
			return false
		}
		switch {
		case claim.ClaimType == ClaimAmount && claim.NormalizedPayload.Direction == "in" && inflow == nil:
			inflow = claim
		case claim.ClaimType == ClaimAmount && claim.NormalizedPayload.Direction == "out" && outflow == nil:
			outflow = claim
		case claim.ClaimType == ClaimCount && claim.NormalizedPayload.Direction == "" && count == nil:
			count = claim
		default:
			return false
		}
	}
	if inflow == nil || outflow == nil || count == nil {
		return false
	}
	if ValidateAccountFlowQueryScopeRefV1(queryScopeRef) != nil ||
		len(inflow.SupportedScope.SourceIDs) != 1 || inflow.SupportedScope.SourceIDs[0] != queryScopeRef {
		return false
	}
	inflowPayload := inflow.NormalizedPayload
	outflowPayload := outflow.NormalizedPayload
	countPayload := count.NormalizedPayload
	return inflowPayload.SubjectID != "" && inflowPayload.SubjectID == outflowPayload.SubjectID &&
		inflowPayload.SubjectID == countPayload.SubjectID && inflowPayload.EntityID == outflowPayload.EntityID &&
		inflowPayload.EntityID == countPayload.EntityID && inflowPayload.AccountID != "" &&
		inflowPayload.AccountID == outflowPayload.AccountID && countPayload.AccountID == "" &&
		inflowPayload.Currency != "" && inflowPayload.Currency == outflowPayload.Currency && countPayload.Currency == "" &&
		inflowPayload.StartAt != "" && inflowPayload.StartAt == outflowPayload.StartAt &&
		inflowPayload.StartAt == countPayload.StartAt && inflowPayload.EndAt != "" &&
		inflowPayload.EndAt == outflowPayload.EndAt && inflowPayload.EndAt == countPayload.EndAt &&
		reflect.DeepEqual(*inflow.SupportedScope, *outflow.SupportedScope) &&
		reflect.DeepEqual(*inflow.SupportedScope, *count.SupportedScope)
}

func cloneAccountFlowTypedAnswerOutcomeV1(
	outcome *AccountFlowTypedAnswerOutcomeV1,
) *AccountFlowTypedAnswerOutcomeV1 {
	if outcome == nil {
		return nil
	}
	cloned := *outcome
	cloned.Groups = append([]AccountFlowTypedAnswerGroupV1(nil), outcome.Groups...)
	for index := range cloned.Groups {
		cloned.Groups[index].ClaimIDs = append([]string(nil), outcome.Groups[index].ClaimIDs...)
	}
	return &cloned
}

func canonicalStrictStringsV1(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func FinalAnswerEnvelopeRecord(envelope FinalAnswerEnvelope) map[string]any {
	body, _ := json.Marshal(envelope)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func validFinalAnswerVariant(variant FinalAnswerVariant) bool {
	switch variant {
	case EvidenceBackedAnswer, PartialEvidenceAnswer, VerifiedNoHitAnswer, SourceUnavailableAnswer, NeedsEvidenceAnswer, GeneralGuidanceAnswer:
		return true
	default:
		return false
	}
}

func validFinalTerminalReason(reason string) bool {
	_, ok := domainterminal.LookupV1(reason)
	return ok
}

func FinalAnswerTerminalStatus(reason string) (string, bool) {
	return domainterminal.StatusForReasonV1(reason)
}

func finalAnswerReceiptSetMatchesClaims(envelope FinalAnswerEnvelope) bool {
	claimReceipts := []string{}
	for _, claim := range envelope.Claims {
		claimReceipts = append(claimReceipts, claim.EvidenceIDs...)
	}
	claimReceipts = canonicalEvidenceStrings(claimReceipts)
	if len(claimReceipts) != len(envelope.EvidenceReceiptIDs) {
		return false
	}
	for index := range claimReceipts {
		if claimReceipts[index] != envelope.EvidenceReceiptIDs[index] {
			return false
		}
	}
	return true
}

func finalAnswerEnvelopeDigest(envelope FinalAnswerEnvelope) string {
	envelope.EnvelopeDigest = ""
	body, _ := json.Marshal(envelope)
	return domainsecurity.SHA256Hex(body)
}

func cloneOrdinaryResultSlotV1(slot *domainordinaryresult.ResultSlotV1) *domainordinaryresult.ResultSlotV1 {
	if slot == nil {
		return nil
	}
	cloned := *slot
	return &cloned
}
