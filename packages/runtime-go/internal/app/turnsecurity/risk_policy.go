package turnsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrRiskPolicyInputInvalid = errors.New("turn risk policy resolution input is invalid")

// RiskPolicyAuthority is the host-private authority used to derive, persist,
// and later revalidate the risk floor bound into a turn. Callers provide only
// host-observed signals; they never provide a policy digest or disposition.
type RiskPolicyAuthority interface {
	ResolveOrRaise(context.Context, threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error)
	ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error
}

type RiskPolicyResolutionInput struct {
	Authority            RiskPolicyAuthority
	ThreadID             string
	WorkspaceRealPath    string
	Binding              domainsecurity.CaseBindingObservationV1
	PreviousContext      *domainsecurity.TurnSecurityContext
	RiskIntent           string
	LexicalCaseRisk      bool
	ProtectedCaseData    bool
	ContextChangingInput bool
	TrustedCaseThread    bool
	IssuedAt             time.Time
}

type RiskPolicyResolution struct {
	ThreadPolicy         domainsecurity.ThreadRiskPolicyV1
	RiskAuthorityBinding domainsecurity.RiskAuthorityBindingV1
	Publication          domainsecurity.TurnPublicationPolicyV1
	RequestedRisk        string
	Origin               string
	SignalsDigest        string
	BindingState         string
	PublicationMode      string
}

type riskSignalsV1 struct {
	Version              int    `json:"version"`
	ThreadID             string `json:"threadId"`
	WorkspaceRealPath    string `json:"workspaceRealPath"`
	BindingState         string `json:"bindingState"`
	BindingDigest        string `json:"bindingDigest"`
	RiskIntentCase       bool   `json:"riskIntentCase"`
	LexicalCaseRisk      bool   `json:"lexicalCaseRisk"`
	ProtectedCaseData    bool   `json:"protectedCaseData"`
	ContextChangingInput bool   `json:"contextChangingInput"`
	TrustedCaseThread    bool   `json:"trustedCaseThread"`
	ExistingCaseRisk     bool   `json:"existingCaseRisk"`
}

// ResolveRiskPublication applies the monotonic host risk floor and derives the
// only publication policy shape allowed for the observed binding state.
func ResolveRiskPublication(ctx context.Context, input RiskPolicyResolutionInput) (RiskPolicyResolution, error) {
	if input.Authority == nil || domainsecurity.ValidateCaseBindingObservationV1(input.Binding) != nil ||
		strings.TrimSpace(input.ThreadID) == "" || input.ThreadID != strings.TrimSpace(input.ThreadID) ||
		input.WorkspaceRealPath != strings.TrimSpace(input.WorkspaceRealPath) || input.WorkspaceRealPath == "" ||
		input.Binding.WorkspaceRealPath != input.WorkspaceRealPath || input.IssuedAt.IsZero() {
		return RiskPolicyResolution{}, ErrRiskPolicyInputInvalid
	}
	if input.PreviousContext != nil && (domainsecurity.ValidateTurnSecurityContext(*input.PreviousContext) != nil ||
		input.PreviousContext.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		input.PreviousContext.ThreadID != input.ThreadID || input.PreviousContext.WorkspaceRealPath != input.WorkspaceRealPath) {
		return RiskPolicyResolution{}, ErrRiskPolicyInputInvalid
	}
	intent := strings.TrimSpace(input.RiskIntent)
	if intent != "" && intent != domainsecurity.RiskClassCase {
		return RiskPolicyResolution{}, errors.Join(ErrRiskPolicyInputInvalid, errors.New("turn risk intent may only raise risk to case"))
	}
	existingCaseRisk := input.PreviousContext != nil &&
		domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(*input.PreviousContext)
	markerPresent := bindingPreventsGeneralPublication(input.Binding.State)
	requestedRisk := domainsecurity.RiskClassGeneral
	if existingCaseRisk || input.TrustedCaseThread || intent == domainsecurity.RiskClassCase || input.LexicalCaseRisk || input.ProtectedCaseData || input.ContextChangingInput || markerPresent {
		requestedRisk = domainsecurity.RiskClassCase
	}
	origin := riskPolicyOrigin(requestedRisk, input.Binding.State, input.TrustedCaseThread, intent, input.LexicalCaseRisk, input.ProtectedCaseData, input.ContextChangingInput)
	signals := riskSignalsV1{
		Version: 1, ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath,
		BindingState: input.Binding.State, BindingDigest: input.Binding.ObservationDigest,
		RiskIntentCase: intent == domainsecurity.RiskClassCase, LexicalCaseRisk: input.LexicalCaseRisk,
		ProtectedCaseData:    input.ProtectedCaseData,
		ContextChangingInput: input.ContextChangingInput,
		TrustedCaseThread:    input.TrustedCaseThread, ExistingCaseRisk: existingCaseRisk,
	}
	body, _ := json.Marshal(signals)
	signalsDigest := domainsecurity.SHA256Hex(body)
	previousPolicyDigest := ""
	if input.PreviousContext != nil &&
		input.PreviousContext.RiskAuthorityBinding.State == domainsecurity.RiskAuthorityBindingStateHostPolicy {
		previousPolicyDigest = input.PreviousContext.RiskAuthorityBinding.ThreadPolicyDigest
	}
	head, err := input.Authority.ResolveOrRaise(ctx, threadriskauthorityapp.ResolveOrRaiseInput{
		ThreadID: input.ThreadID, WorkspaceRealPath: input.WorkspaceRealPath, RequestedRisk: requestedRisk,
		BindingObservation: input.Binding, Origin: origin, SignalsDigest: signalsDigest,
		PreviousPolicyDigest: previousPolicyDigest, IssuedAt: input.IssuedAt,
	})
	if err != nil {
		return RiskPolicyResolution{}, err
	}
	threadPolicy := head.Policy
	generalOnlyPolicy := head.GeneralOnlyPolicy
	riskClass := ""
	switch head.RiskAuthorityBinding.State {
	case domainsecurity.RiskAuthorityBindingStateWitnessed:
		if !head.HasIndex || !head.Found || domainsecurity.ValidateThreadRiskPolicyV1(threadPolicy) != nil ||
			head.GeneralOnlyPolicy != (domainsecurity.GeneralOnlyRiskPolicyV1{}) ||
			threadPolicy.ThreadID != input.ThreadID || threadPolicy.WorkspaceRealPath != input.WorkspaceRealPath ||
			(requestedRisk == domainsecurity.RiskClassCase && threadPolicy.RiskClass != domainsecurity.RiskClassCase) ||
			domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
				head.RiskAuthorityBinding, head.Index, head.Request, head.Observation,
			) != nil {
			return RiskPolicyResolution{}, errors.New("host thread risk authority resolution is invalid")
		}
		riskClass = threadPolicy.RiskClass
	case domainsecurity.RiskAuthorityBindingStateHostPolicy:
		if head.HasIndex || !head.Found || domainsecurity.ValidateThreadRiskPolicyV1(threadPolicy) != nil ||
			head.GeneralOnlyPolicy != (domainsecurity.GeneralOnlyRiskPolicyV1{}) ||
			head.Index.IndexDigest != "" || head.Request.RequestDigest != "" || head.Observation.ObservationDigest != "" ||
			requestedRisk != domainsecurity.RiskClassCase || threadPolicy.RiskClass != domainsecurity.RiskClassCase ||
			threadPolicy.ThreadID != input.ThreadID || threadPolicy.WorkspaceRealPath != input.WorkspaceRealPath ||
			domainsecurity.ValidateHostPolicyRiskAuthorityBindingV1(
				head.RiskAuthorityBinding, threadPolicy, input.Binding,
			) != nil {
			return RiskPolicyResolution{}, errors.New("host thread risk authority resolution is invalid")
		}
		riskClass = threadPolicy.RiskClass
	case domainsecurity.RiskAuthorityBindingStateHostGeneralOnly:
		if head.HasIndex || !head.Found || head.Policy != (domainsecurity.ThreadRiskPolicyV1{}) ||
			head.Index.IndexDigest != "" || head.Request.RequestDigest != "" || head.Observation.ObservationDigest != "" ||
			requestedRisk != domainsecurity.RiskClassGeneral || origin != domainsecurity.RiskPolicyOriginGeneralWorkspace ||
			domainsecurity.ValidateGeneralOnlyRiskPolicyV1(generalOnlyPolicy) != nil ||
			generalOnlyPolicy.ThreadID != input.ThreadID || generalOnlyPolicy.WorkspaceRealPath != input.WorkspaceRealPath ||
			generalOnlyPolicy.BindingObservationDigest != input.Binding.ObservationDigest ||
			domainsecurity.ValidateHostGeneralOnlyRiskAuthorityBindingV1(head.RiskAuthorityBinding, generalOnlyPolicy) != nil {
			return RiskPolicyResolution{}, errors.New("host general-only risk authority resolution is invalid")
		}
		riskClass = domainsecurity.RiskClassGeneral
	default:
		return RiskPolicyResolution{}, errors.New("host thread risk authority resolution is invalid")
	}
	bindingState, disposition, blocker := publicationShape(riskClass, input.Binding, input.PreviousContext)
	policyDigest := threadPolicy.PolicyDigest
	if head.RiskAuthorityBinding.State == domainsecurity.RiskAuthorityBindingStateHostGeneralOnly {
		policyDigest = generalOnlyPolicy.PolicyDigest
	}
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: riskClass,
		Disposition: disposition, CaseBindingState: bindingState,
		BindingObservationDigest: input.Binding.ObservationDigest, BlockerCode: blocker,
	})
	if err != nil {
		return RiskPolicyResolution{}, errors.New("host turn publication policy derivation failed")
	}
	if head.RiskAuthorityBinding.State == domainsecurity.RiskAuthorityBindingStateWitnessed ||
		head.RiskAuthorityBinding.State == domainsecurity.RiskAuthorityBindingStateHostPolicy {
		if domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(publication, threadPolicy) != nil {
			return RiskPolicyResolution{}, errors.New("host turn publication policy derivation failed")
		}
	} else if domainsecurity.ValidateTurnPublicationPolicyForGeneralOnlyRiskPolicyV1(publication, generalOnlyPolicy) != nil {
		return RiskPolicyResolution{}, errors.New("host turn publication policy derivation failed")
	}
	return RiskPolicyResolution{
		ThreadPolicy: threadPolicy, RiskAuthorityBinding: head.RiskAuthorityBinding,
		Publication: publication, RequestedRisk: requestedRisk,
		Origin: origin, SignalsDigest: signalsDigest, BindingState: bindingState, PublicationMode: disposition,
	}, nil
}

// bindingPreventsGeneralPublication makes an exact host observation of a
// missing case marker the only binding state that can contribute to a general
// policy. Every other valid observation is negative authority: in particular,
// an unavailable workspace or a legacy not-applicable observation must never
// be interpreted as proof that the workspace is non-case.
func bindingPreventsGeneralPublication(state string) bool {
	return state != domainsecurity.CaseBindingStateMissing
}

func riskPolicyOrigin(risk, bindingState string, trusted bool, intent string, lexical, protectedData, contextChanging bool) string {
	if risk == domainsecurity.RiskClassGeneral {
		return domainsecurity.RiskPolicyOriginGeneralWorkspace
	}
	if bindingState == domainsecurity.CaseBindingStateValid {
		return domainsecurity.RiskPolicyOriginValidCaseBinding
	}
	if bindingPreventsGeneralPublication(bindingState) {
		return domainsecurity.RiskPolicyOriginBindingMarkerPresent
	}
	if trusted {
		return domainsecurity.RiskPolicyOriginSignedCaseLineage
	}
	if intent == domainsecurity.RiskClassCase {
		return domainsecurity.RiskPolicyOriginDesktopCaseEntry
	}
	if lexical {
		return domainsecurity.RiskPolicyOriginLexicalGuard
	}
	if protectedData {
		return domainsecurity.RiskPolicyOriginProtectedDataGuard
	}
	if contextChanging {
		return domainsecurity.RiskPolicyOriginHostInputContextChange
	}
	return domainsecurity.RiskPolicyOriginLegacyMigration
}

func publicationShape(risk string, binding domainsecurity.CaseBindingObservationV1, previous *domainsecurity.TurnSecurityContext) (string, string, string) {
	if risk == domainsecurity.RiskClassGeneral {
		if binding.State != domainsecurity.CaseBindingStateMissing {
			return domainsecurity.CaseBindingStatePolicyCorrupt, domainsecurity.PublicationDispositionCaseBoundaryOnly, domainsecurity.PublicationBlockerCasePolicyCorrupt
		}
		return domainsecurity.CaseBindingStateMissing, domainsecurity.PublicationDispositionGeneralOutput, domainsecurity.PublicationBlockerNone
	}
	state := binding.State
	if state == domainsecurity.CaseBindingStateValid && previousCaseBindingChanged(previous, binding) {
		state = domainsecurity.CaseBindingStateChangedUnaccepted
	}
	if state == domainsecurity.CaseBindingStateValid {
		return state, domainsecurity.PublicationDispositionCaseEvidenceGate, domainsecurity.PublicationBlockerNone
	}
	if blocker := publicationBlockerForState(state); blocker != "" {
		return state, domainsecurity.PublicationDispositionCaseBoundaryOnly, blocker
	}
	return domainsecurity.CaseBindingStatePolicyCorrupt, domainsecurity.PublicationDispositionCaseBoundaryOnly, domainsecurity.PublicationBlockerCasePolicyCorrupt
}

func previousCaseBindingChanged(previous *domainsecurity.TurnSecurityContext, binding domainsecurity.CaseBindingObservationV1) bool {
	if previous == nil || domainsecurity.ValidateTurnSecurityContext(*previous) != nil ||
		!domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(*previous) ||
		previous.PublicationPolicy.Disposition != domainsecurity.PublicationDispositionCaseEvidenceGate {
		return false
	}
	return previous.CaseID != binding.CaseID || previous.CaseBindingHash != binding.CaseBindingHash
}

func publicationBlockerForState(state string) string {
	switch state {
	case domainsecurity.CaseBindingStateMissing:
		return domainsecurity.PublicationBlockerCaseBindingMissing
	case domainsecurity.CaseBindingStateInvalid:
		return domainsecurity.PublicationBlockerCaseBindingInvalid
	case domainsecurity.CaseBindingStateUnreadable:
		return domainsecurity.PublicationBlockerCaseBindingUnreadable
	case domainsecurity.CaseBindingStateUnstable:
		return domainsecurity.PublicationBlockerCaseBindingUnstable
	case domainsecurity.CaseBindingStateChangedUnaccepted:
		return domainsecurity.PublicationBlockerCaseBindingChanged
	case domainsecurity.CaseBindingStateWorkspaceMissing:
		return domainsecurity.PublicationBlockerCaseWorkspaceUnavailable
	case domainsecurity.CaseBindingStatePolicyCorrupt:
		return domainsecurity.PublicationBlockerCasePolicyCorrupt
	default:
		return ""
	}
}
