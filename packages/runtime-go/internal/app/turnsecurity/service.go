package turnsecurity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

// WorkspaceReader remains only as a source-compatible audit/migration
// boundary while turn-start callers are moved to WorkspaceSecurityAuthority.
// It must not be used to infer a missing binding as an unbound general turn.
type WorkspaceReader interface {
	ReadOptional(string) (domainsecurity.CaseBinding, bool, error)
	WorkspaceRealPath(string) (string, error)
}

// WorkspaceSecurityAuthority contains only host-owned authority. The caller
// can raise risk with RiskIntent, but cannot supply a case id, binding hash,
// publication disposition, policy digest, or blocker code.
type WorkspaceSecurityAuthority struct {
	Identity      identityport.Authority
	Observer      casecontextport.Observer
	RiskAuthority RiskPolicyAuthority
	// SnapshotAuthority is the audit-only V1 compatibility surface. A V1
	// record can never authorize case facts for a newly frozen context.
	SnapshotAuthority    datasetsnapshotport.Authority
	SnapshotAuthorityV2  datasetsnapshotport.AuthorityV2
	RiskIntent           string
	LexicalCaseRisk      bool
	ProtectedCaseData    bool
	ContextChangingInput bool
	TrustedCaseThread    bool
}

func CurrentFailureCode(err error) string {
	if err != nil {
		switch strings.TrimSpace(err.Error()) {
		case "turn_security_identity_mismatch", "turn_security_workspace_mismatch", "turn_security_case_binding_mismatch", "turn_security_dataset_snapshot_mismatch", "turn_security_risk_policy_mismatch":
			return strings.TrimSpace(err.Error())
		}
	}
	return "turn_security_context_invalid"
}

type WorkspaceFreezeInput struct {
	Context   context.Context
	Authority WorkspaceSecurityAuthority
	// Reader is retained so unmigrated callers compile, but is deliberately
	// ignored. Observer and RiskAuthority are both mandatory.
	Reader    WorkspaceReader
	Thread    map[string]any
	ThreadID  string
	TurnID    string
	Workspace string
	// Principal is resolved once from host-only identity authority before any
	// writer, case observation, dataset snapshot, provider, or effect boundary.
	// HTTP, IPC, model, MCP, and tool payloads cannot select it.
	Principal domainidentity.PrincipalV1
	// DatasetSnapshotID and VerifiedProbe are retained for source compatibility
	// and ignored. A snapshot may enter V2 only through SnapshotAuthority;
	// the later live probe can verify, but never select or replace it.
	DatasetSnapshotID string
	VerifiedProbe     *domainsecurity.VerifiedSourceProbe
	SourceDiagnostics []any
	IssuedAt          time.Time
}

type CurrentValidationInput struct {
	OperationContext context.Context
	Identity         identityport.Authority
	Observer         casecontextport.Observer
	RiskAuthority    RiskPolicyAuthority
	// SnapshotAuthority is retained for V1 audit compatibility only. Current
	// case execution requires an exact V2 resolution from SnapshotAuthorityV2.
	SnapshotAuthority   datasetsnapshotport.Authority
	SnapshotAuthorityV2 datasetsnapshotport.AuthorityV2
	// Reader is retained only for source compatibility. It is not authority.
	Reader            WorkspaceReader
	Context           domainsecurity.TurnSecurityContext
	Workspace         string
	SourceDiagnostics []any
}

type SourceDiagnosticsProvider interface {
	ServerDiagnostics() []any
}

type ContextualSourceDiagnosticsProvider interface {
	ServerDiagnosticsForSecurityContext(domainsecurity.TurnSecurityContext) []any
}

func PublicRecord(context domainsecurity.TurnSecurityContext) map[string]any {
	body, _ := json.Marshal(context)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func FreezeWorkspace(input WorkspaceFreezeInput) (domainsecurity.TurnSecurityContext, error) {
	return freezeWorkspace(input, true)
}

// ResolveCurrentPrincipal resolves and immediately revalidates the host-only
// identity authority. Callers retain this exact value through writer
// admission, case binding, dataset snapshot selection, TSC freeze, and commit.
func ResolveCurrentPrincipal(ctx context.Context, authority identityport.Authority) (domainidentity.PrincipalV1, error) {
	if ctx == nil || authority == nil {
		return domainidentity.PrincipalV1{}, errors.New("turn_security_identity_mismatch")
	}
	principal, err := authority.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		authority.ValidateCurrent(ctx, principal) != nil {
		return domainidentity.PrincipalV1{}, errors.New("turn_security_identity_mismatch")
	}
	return principal, nil
}

// ValidateResolvedPrincipal proves that a previously resolved principal is
// still current without allowing any request surface to replace it.
func ValidateResolvedPrincipal(ctx context.Context, authority identityport.Authority, principal domainidentity.PrincipalV1) error {
	if ctx == nil || authority == nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		authority.ValidateCurrent(ctx, principal) != nil {
		return errors.New("turn_security_identity_mismatch")
	}
	return nil
}

// FreezeWorkspaceRebindTarget derives a new general-workspace context without
// treating the old workspace context as an in-place continuation. The risk
// authority still owns the monotonic thread floor; this helper only prevents
// the previous workspace identity from being misread as the target identity.
func FreezeWorkspaceRebindTarget(input WorkspaceFreezeInput, previous domainsecurity.TurnSecurityContext) (domainsecurity.TurnSecurityContext, error) {
	if domainsecurity.ValidateTurnSecurityContextForExecution(previous) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(previous) ||
		previous.ThreadID != strings.TrimSpace(input.ThreadID) {
		return domainsecurity.TurnSecurityContext{}, errors.New("workspace rebind previous authority is invalid")
	}
	if previous.TenantID != input.Principal.TenantID || previous.UserID != input.Principal.UserID {
		return domainsecurity.TurnSecurityContext{}, errors.New("turn_security_identity_mismatch")
	}
	target, err := freezeWorkspace(input, false)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if !domainsecurity.TurnSecurityContextIsGeneral(target) || target.WorkspaceRealPath == previous.WorkspaceRealPath {
		return domainsecurity.TurnSecurityContext{}, errors.New("workspace rebind target authority is invalid")
	}
	return target, nil
}

func freezeWorkspace(input WorkspaceFreezeInput, useThreadPrevious bool) (domainsecurity.TurnSecurityContext, error) {
	if input.Context == nil || input.Authority.Observer == nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("turn security V2 case binding observer is required")
	}
	if input.IssuedAt.IsZero() || strings.TrimSpace(input.ThreadID) == "" || strings.TrimSpace(input.TurnID) == "" {
		return domainsecurity.TurnSecurityContext{}, errors.New("turn security V2 freeze input is incomplete")
	}
	if err := ValidateResolvedPrincipal(input.Context, input.Authority.Identity, input.Principal); err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	observation, err := input.Authority.Observer.Observe(input.Workspace)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, fmt.Errorf("observe turn security case binding: %w", err)
	}
	if err := domainsecurity.ValidateCaseBindingObservationV1(observation); err != nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("turn security case binding observation is invalid")
	}
	workspaceRealPath := observation.WorkspaceRealPath
	previous := domainsecurity.TurnSecurityContext{}
	found := false
	if useThreadPrevious {
		previous, found, err = LatestContext(input.Thread)
		if err != nil {
			return domainsecurity.TurnSecurityContext{}, err
		}
	}
	if found {
		if previous.Version != domainsecurity.TurnSecurityContextVersionV2 {
			return domainsecurity.TurnSecurityContext{}, errors.New("turn security context V1 is audit-only")
		}
		if previous.TenantID != input.Principal.TenantID || previous.UserID != input.Principal.UserID {
			return domainsecurity.TurnSecurityContext{}, errors.New("turn_security_identity_mismatch")
		}
	}
	resolution := RiskPolicyResolution{}
	resolutionErr := error(threadriskauthorityapp.ErrUnavailable)
	if input.Authority.RiskAuthority != nil {
		resolution, resolutionErr = ResolveRiskPublication(input.Context, RiskPolicyResolutionInput{
			Authority: input.Authority.RiskAuthority, ThreadID: strings.TrimSpace(input.ThreadID), WorkspaceRealPath: workspaceRealPath,
			Binding: observation, PreviousContext: optionalPreviousContext(previous, found), RiskIntent: input.Authority.RiskIntent,
			LexicalCaseRisk: input.Authority.LexicalCaseRisk, ProtectedCaseData: input.Authority.ProtectedCaseData,
			ContextChangingInput: input.Authority.ContextChangingInput,
			TrustedCaseThread:    input.Authority.TrustedCaseThread, IssuedAt: input.IssuedAt,
		})
	}
	epoch, err := acceptedContextEpoch(input.Thread, input.ThreadID)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if resolutionErr != nil {
		if errors.Is(resolutionErr, ErrRiskPolicyInputInvalid) {
			return domainsecurity.TurnSecurityContext{}, resolutionErr
		}
		return quarantinedTurnSecurityContext(input, observation, epoch, riskAuthorityBlocker(resolutionErr))
	}
	publication := resolution.Publication
	riskBinding := resolution.RiskAuthorityBinding
	caseID := domainsecurity.UnboundCaseID
	bindingHash := domainsecurity.UnboundCaseBindingHash(workspaceRealPath)
	datasetSnapshotID := domainsecurity.NoDatasetSnapshotID
	sourceManifestHash := domainsecurity.EmptySourceManifestHash
	if publication.Disposition == domainsecurity.PublicationDispositionCaseEvidenceGate {
		resolved, resolveErr := resolveDatasetSnapshotV2(
			input.Context, input.Authority.SnapshotAuthorityV2, observation,
			input.Principal.TenantID, input.Principal.UserID, "",
		)
		if resolveErr != nil {
			publication, err = datasetBoundaryPublication(publication, resolveErr)
			if err != nil {
				return domainsecurity.TurnSecurityContext{}, err
			}
		} else {
			caseID = resolved.Record.Binding.CaseID
			bindingHash = resolved.Record.Binding.CaseBindingHash
			datasetSnapshotID = resolved.Record.DatasetSnapshotID
			sourceManifestHash = resolved.Record.SourceManifestHash
		}
	}
	if err := ValidateResolvedPrincipal(input.Context, input.Authority.Identity, input.Principal); err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	frozen, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: strings.TrimSpace(input.ThreadID), TurnID: strings.TrimSpace(input.TurnID), WorkspaceRealPath: workspaceRealPath,
		TenantID: input.Principal.TenantID, UserID: input.Principal.UserID, CaseID: caseID,
		CaseBindingHash: bindingHash, DatasetSnapshotID: datasetSnapshotID, SourceManifestHash: sourceManifestHash,
		ContextEpoch: epoch, IssuedAt: input.IssuedAt, PublicationPolicy: publication,
		RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(frozen) == nil {
		if err := ValidateCurrentRiskAuthority(input.Context, input.Authority.RiskAuthority, frozen); err != nil {
			return domainsecurity.TurnSecurityContext{}, fmt.Errorf("validate frozen turn risk authority: %w", err)
		}
	}
	return frozen, nil
}

// FreezeAndAttachStart is the fail-closed compatibility surface for callers
// that have not yet been migrated to host V2 authority. It intentionally does
// not infer a general policy from a legacy WorkspaceReader.
func FreezeAndAttachStart(ctx context.Context, reader WorkspaceReader, source any, thread map[string]any, threadID string, turnID string, workspace string, issuedAt time.Time, turn map[string]any, turnStartedEvent map[string]any, threadPatch map[string]any, frozenHooks ...func(context.Context, domainsecurity.TurnSecurityContext) error) (domainsecurity.TurnSecurityContext, error) {
	return domainsecurity.TurnSecurityContext{}, errors.New("turn security V2 observer and risk policy authority are required")
}

func FreezeAndAttachStartV2(ctx context.Context, authority WorkspaceSecurityAuthority, source any, thread map[string]any, threadID string, turnID string, workspace string, issuedAt time.Time, turn map[string]any, turnStartedEvent map[string]any, threadPatch map[string]any, frozenHooks ...func(context.Context, domainsecurity.TurnSecurityContext) error) (domainsecurity.TurnSecurityContext, error) {
	result, err := FreezeAndAttachStartV2Result(
		ctx, authority, source, thread, threadID, turnID, workspace, issuedAt,
		turn, turnStartedEvent, threadPatch, frozenHooks...,
	)
	return result.SecurityContext, err
}

type FreezeStartResult struct {
	SecurityContext domainsecurity.TurnSecurityContext
	SourceProbe     domainsecurity.VerifiedSourceProbe
	SourceReady     bool
	SourceError     error
}

// FreezeAndAttachStartV2Result freezes exactly one TSC from host authority.
// A later live probe can only confirm the already selected snapshot. Probe
// failure is returned as readiness state so the caller can take the fixed
// source-unavailable terminal path without invoking a provider.
func FreezeAndAttachStartV2Result(ctx context.Context, authority WorkspaceSecurityAuthority, source any, thread map[string]any, threadID string, turnID string, workspace string, issuedAt time.Time, turn map[string]any, turnStartedEvent map[string]any, threadPatch map[string]any, frozenHooks ...func(context.Context, domainsecurity.TurnSecurityContext) error) (FreezeStartResult, error) {
	principal, err := ResolveCurrentPrincipal(ctx, authority.Identity)
	if err != nil {
		return FreezeStartResult{}, err
	}
	frozen, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: ctx, Authority: authority, Thread: thread, ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: principal, IssuedAt: issuedAt,
	})
	if err != nil {
		return FreezeStartResult{}, err
	}
	result := FreezeStartResult{SecurityContext: frozen, SourceReady: !domainsecurity.TurnSecurityContextAllowsCaseEvidence(frozen)}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(frozen) {
		for _, hook := range frozenHooks {
			if hook != nil {
				if err := hook(ctx, frozen); err != nil {
					return FreezeStartResult{}, err
				}
			}
		}
		binding, bindingErr := observeCurrentCaseBinding(authority.Observer, workspace, frozen)
		discovery := domainsecurity.VerifiedSourceProbe{}
		probeErr := bindingErr
		if bindingErr == nil {
			discovery, probeErr = ProbeCurrentCaseSource(ctx, source, frozen, binding)
		}
		result.SourceProbe = discovery
		result.SourceError = probeErr
		result.SourceReady = probeErr == nil && SourceDiscoveryMatchesContext(discovery, frozen)
		if probeErr == nil && !result.SourceReady {
			result.SourceError = errors.New("current case source probe does not match frozen host authority")
		}
	}
	AttachStartRecords(turn, turnStartedEvent, threadPatch, frozen)
	return result, nil
}

func ProbeCurrentCaseSource(
	ctx context.Context,
	source any,
	securityContext domainsecurity.TurnSecurityContext,
	binding domainsecurity.CaseBindingObservationV1,
) (domainsecurity.VerifiedSourceProbe, error) {
	prober, ok := source.(sourceprobeport.Prober)
	if !ok || prober == nil || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) ||
		validateCurrentCaseBindingForSource(binding, securityContext) != nil {
		return domainsecurity.VerifiedSourceProbe{}, errors.New("current case source probe authority is unavailable")
	}
	return prober.ProbeCaseSource(ctx, sourceprobeport.Input{
		ServerID: "analytix_funds", Context: securityContext, Binding: binding,
		WorkspaceRealPath: securityContext.WorkspaceRealPath, ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch:      securityContext.ContextEpoch, ContextDigest: securityContext.ContextDigest,
	})
}

func observeCurrentCaseBinding(
	observer casecontextport.Observer,
	workspace string,
	securityContext domainsecurity.TurnSecurityContext,
) (domainsecurity.CaseBindingObservationV1, error) {
	if observer == nil {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("current case binding observer is unavailable")
	}
	binding, err := observer.Observe(workspace)
	if err != nil || validateCurrentCaseBindingForSource(binding, securityContext) != nil {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("current case binding changed before source probe")
	}
	return binding, nil
}

func validateCurrentCaseBindingForSource(
	binding domainsecurity.CaseBindingObservationV1,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainsecurity.ValidateCaseBindingObservationV1(binding) != nil ||
		binding.State != domainsecurity.CaseBindingStateValid ||
		binding.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		binding.CaseID != securityContext.CaseID ||
		binding.CaseBindingHash != securityContext.CaseBindingHash ||
		binding.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
		return errors.New("current case binding does not match frozen source authority")
	}
	return nil
}

// SourceDiscoveryMatchesContext is the single exact comparison used after a
// current-run source probe. A structurally valid probe for another case,
// binding, turn, epoch, context digest, or dataset is never readiness proof.
func SourceDiscoveryMatchesContext(probe domainsecurity.VerifiedSourceProbe, context domainsecurity.TurnSecurityContext) bool {
	return domainsecurity.ValidateVerifiedSourceProbe(probe) == nil && probe.ThreadID == context.ThreadID && probe.TurnID == context.TurnID &&
		probe.ContextEpoch == context.ContextEpoch && probe.ProbeContextDigest == context.ContextDigest && probe.CaseID == context.CaseID &&
		probe.CaseBindingHash == context.CaseBindingHash && probe.DatasetSnapshotID == context.DatasetSnapshotID
}

func ValidateCurrent(input CurrentValidationInput) error {
	return validateCurrent(input, true, false)
}

// ValidateCurrentInsideExactDatasetCapability revalidates the current
// principal, risk authority, workspace and case binding while an existing
// CurrentSelectionCapabilityV2 exact-use callback already owns DSV2
// currentness. It must never be used outside that callback and deliberately
// performs no second dataset inventory/material resolution.
func ValidateCurrentInsideExactDatasetCapability(input CurrentValidationInput) error {
	return validateCurrent(input, false, true)
}

// ValidateCurrentForEffect keeps the currentness choice at the authority
// owner: case-data effects require the exact live DSV2, while ordinary effects
// retain the same identity, risk, workspace, and case-binding checks without
// making that dataset a global Agent prerequisite.
func ValidateCurrentForEffect(input CurrentValidationInput, caseDataEffect bool) error {
	return validateCurrent(input, caseDataEffect, caseDataEffect)
}

// ValidateCurrentOrdinaryEffect revalidates the same TSCV2 identity, risk
// authority, workspace, and case binding as ValidateCurrent, but does not make
// the current dataset snapshot a prerequisite for an ordinary provider or
// tool effect. Protected case-data effects must continue to use
// ValidateCurrent.
func ValidateCurrentOrdinaryEffect(input CurrentValidationInput) error {
	return ValidateCurrentForEffect(input, false)
}

// ValidateCurrentHostBoundary verifies that a fixed boundary-only response is
// still bound to the same principal, workspace, and host observation. It does
// not make the context executable and must never be used to authorize a
// provider, tool, case fact, or publication effect.
func ValidateCurrentHostBoundary(input CurrentValidationInput) error {
	if domainsecurity.ValidateTurnSecurityContext(input.Context) != nil ||
		input.Context.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		!domainsecurity.TurnSecurityContextIsBoundaryOnly(input.Context) {
		return errors.New("turn_security_risk_policy_mismatch")
	}
	operationContext := input.OperationContext
	if operationContext == nil {
		operationContext = context.Background()
	}
	principal, err := ResolveCurrentPrincipal(operationContext, input.Identity)
	if err != nil || principal.TenantID != input.Context.TenantID || principal.UserID != input.Context.UserID {
		return errors.New("turn_security_identity_mismatch")
	}
	if input.Observer == nil {
		return errors.New("turn security V2 observer and risk policy authority are required")
	}
	observation, err := input.Observer.Observe(input.Workspace)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.WorkspaceRealPath != input.Context.WorkspaceRealPath {
		return errors.New("turn_security_workspace_mismatch")
	}
	if observation.ObservationDigest != input.Context.PublicationPolicy.BindingObservationDigest {
		return errors.New("turn_security_case_binding_mismatch")
	}
	return nil
}

func validateCurrent(input CurrentValidationInput, requireDatasetSnapshot bool, requireCaseFact bool) error {
	if err := domainsecurity.ValidateTurnSecurityContext(input.Context); err != nil {
		return err
	}
	if input.Context.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return errors.New("turn security context V1 is audit-only")
	}
	if requireCaseFact {
		if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
			return errors.New("turn_security_risk_policy_mismatch")
		}
	} else {
		if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(input.Context); err != nil {
			return err
		}
	}
	operationContext := input.OperationContext
	if operationContext == nil {
		operationContext = context.Background()
	}
	principal, err := ResolveCurrentPrincipal(operationContext, input.Identity)
	if err != nil || principal.TenantID != input.Context.TenantID || principal.UserID != input.Context.UserID {
		return errors.New("turn_security_identity_mismatch")
	}
	if input.Observer == nil {
		return errors.New("turn security V2 observer and risk policy authority are required")
	}
	if !requireDatasetSnapshot {
		if input.RiskAuthority == nil || input.RiskAuthority.ValidateCurrent(operationContext, input.Context) != nil {
			return errors.New("turn_security_risk_policy_mismatch")
		}
	} else if domainsecurity.ValidateTurnSecurityContextForExecution(input.Context) == nil {
		if input.RiskAuthority == nil || ValidateCurrentRiskAuthority(operationContext, input.RiskAuthority, input.Context) != nil {
			return errors.New("turn_security_risk_policy_mismatch")
		}
	}
	observation, err := input.Observer.Observe(input.Workspace)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.WorkspaceRealPath != input.Context.WorkspaceRealPath {
		return errors.New("turn_security_workspace_mismatch")
	}
	if observation.ObservationDigest != input.Context.PublicationPolicy.BindingObservationDigest {
		return errors.New("turn_security_case_binding_mismatch")
	}
	if domainsecurity.TurnSecurityContextAllowsCaseEvidence(input.Context) &&
		(observation.State != domainsecurity.CaseBindingStateValid || observation.CaseID != input.Context.CaseID ||
			observation.CaseBindingHash != input.Context.CaseBindingHash) {
		return errors.New("turn_security_case_binding_mismatch")
	}
	if requireDatasetSnapshot && domainsecurity.TurnSecurityContextAllowsCaseEvidence(input.Context) {
		resolved, resolveErr := resolveDatasetSnapshotV2(
			operationContext, input.SnapshotAuthorityV2, observation, input.Context.TenantID, input.Context.UserID,
			input.Context.DatasetSnapshotID,
		)
		if resolveErr != nil || resolved.Record.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
			resolved.Record.SourceManifestHash != input.Context.SourceManifestHash ||
			resolved.Manifest.SourceManifestHash != input.Context.SourceManifestHash {
			return errors.New("turn_security_dataset_snapshot_mismatch")
		}
	}
	return nil
}

// ValidateCurrentRiskAuthority rejects a syntactically and historically valid
// policy once a newer signed head exists. Historical resolution is sufficient
// for audit, never for current execution, settlement, or mutation authority.
func ValidateCurrentRiskAuthority(ctx context.Context, authority RiskPolicyAuthority, securityContext domainsecurity.TurnSecurityContext) error {
	if authority == nil || securityContext.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil {
		return errors.New("turn security current risk authority is invalid")
	}
	if ctx == nil {
		return errors.New("turn security current risk authority context is required")
	}
	return authority.ValidateCurrent(ctx, securityContext)
}

func LatestContext(thread map[string]any) (domainsecurity.TurnSecurityContext, bool, error) {
	if thread == nil || thread["securityState"] == nil {
		return domainsecurity.TurnSecurityContext{}, false, nil
	}
	context, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, false, fmt.Errorf("invalid persisted turn security state: %w", err)
	}
	return context, true, nil
}

func quarantinedTurnSecurityContext(input WorkspaceFreezeInput, observation domainsecurity.CaseBindingObservationV1, epoch uint64, blocker string) (domainsecurity.TurnSecurityContext, error) {
	if input.Context != nil && input.Context.Err() != nil {
		return domainsecurity.TurnSecurityContext{}, input.Context.Err()
	}
	state := observation.State
	if publicationBlockerForState(state) == "" && state != domainsecurity.CaseBindingStateValid {
		state = domainsecurity.CaseBindingStatePolicyCorrupt
	}
	referenceBytes, _ := json.Marshal(struct {
		Purpose           string `json:"purpose"`
		ThreadID          string `json:"threadId"`
		WorkspaceRealPath string `json:"workspaceRealPath"`
		ObservationDigest string `json:"observationDigest"`
		Blocker           string `json:"blocker"`
	}{
		Purpose: "analytix.quarantined-risk-reference/v1", ThreadID: strings.TrimSpace(input.ThreadID),
		WorkspaceRealPath: observation.WorkspaceRealPath, ObservationDigest: observation.ObservationDigest, Blocker: blocker,
	})
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex(referenceBytes), RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly, CaseBindingState: state,
		BindingObservationDigest: observation.ObservationDigest, BlockerCode: blocker,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: strings.TrimSpace(input.ThreadID), TurnID: strings.TrimSpace(input.TurnID),
		WorkspaceRealPath: observation.WorkspaceRealPath, TenantID: input.Principal.TenantID, UserID: input.Principal.UserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(observation.WorkspaceRealPath),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: epoch, IssuedAt: input.IssuedAt, PublicationPolicy: publication,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
}

func riskAuthorityBlocker(err error) string {
	switch {
	case errors.Is(err, monotonicheadport.ErrIndeterminate),
		errors.Is(err, threadriskauthorityapp.ErrAdvanceNotCommitted):
		return domainsecurity.PublicationBlockerRiskAuthorityIndeterminate
	case errors.Is(err, monotonicheadport.ErrUnavailable),
		errors.Is(err, monotonicheadport.ErrNotEnrolled),
		errors.Is(err, threadriskauthorityapp.ErrUnavailable),
		errors.Is(err, threadriskauthorityapp.ErrGeneralOnlyDenied):
		return domainsecurity.PublicationBlockerRiskAuthorityUnavailable
	default:
		return domainsecurity.PublicationBlockerRiskAuthorityInconsistent
	}
}

// resolveDatasetSnapshot is retained only for audit/migration callers. It is
// deliberately not used by FreezeWorkspace or ValidateCurrent: a witnessed V1
// record has no exact producer-content graph and therefore cannot authorize a
// case fact in a newly executing turn.
func resolveDatasetSnapshot(ctx context.Context, authority datasetsnapshotport.Authority, observation domainsecurity.CaseBindingObservationV1, tenantID, userID string) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if authority == nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrUnavailable
	}
	record, err := authority.ResolveWitnessed(ctx, datasetsnapshotport.ResolveInput{
		TenantID: strings.TrimSpace(tenantID), UserID: strings.TrimSpace(userID), Observation: observation,
	})
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	if domainsecurity.ValidateDatasetSnapshotAuthorityRecordV1(record) != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrCorrupt
	}
	if domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV1(record, observation) != nil ||
		record.TenantID != strings.TrimSpace(tenantID) || record.UserID != strings.TrimSpace(userID) {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrMismatch
	}
	return record, nil
}

func resolveDatasetSnapshotV2(
	ctx context.Context,
	authority datasetsnapshotport.AuthorityV2,
	observation domainsecurity.CaseBindingObservationV1,
	tenantID, userID, expectedDatasetSnapshotID string,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if authority == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	tenantID = strings.TrimSpace(tenantID)
	userID = strings.TrimSpace(userID)
	resolved, err := authority.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID: tenantID, UserID: userID, Observation: observation,
		ExpectedDatasetSnapshotID: strings.TrimSpace(expectedDatasetSnapshotID),
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if datasetsnapshotport.ValidateResolvedSnapshotV2(resolved) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrCorrupt
	}
	if domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV2(
		resolved.Record, tenantID, userID, observation,
	) != nil || resolved.Manifest.Binding != resolved.Record.Binding {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	if expectedDatasetSnapshotID != "" && resolved.Record.DatasetSnapshotID != strings.TrimSpace(expectedDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return resolved, nil
}

func datasetBoundaryPublication(current domainsecurity.TurnPublicationPolicyV1, cause error) (domainsecurity.TurnPublicationPolicyV1, error) {
	blocker := domainsecurity.PublicationBlockerDatasetSnapshotCorrupt
	switch {
	case errors.Is(cause, datasetsnapshotport.ErrUnavailable):
		blocker = domainsecurity.PublicationBlockerDatasetSnapshotUnavailable
	case errors.Is(cause, datasetsnapshotport.ErrMismatch):
		blocker = domainsecurity.PublicationBlockerDatasetSnapshotMismatch
	case errors.Is(cause, datasetsnapshotport.ErrStale):
		blocker = domainsecurity.PublicationBlockerDatasetSnapshotStale
	case errors.Is(cause, datasetsnapshotport.ErrCorrupt):
		blocker = domainsecurity.PublicationBlockerDatasetSnapshotCorrupt
	}
	return domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: current.ThreadRiskPolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: current.BindingObservationDigest, BlockerCode: blocker,
	})
}

func acceptedContextEpoch(thread map[string]any, threadID string) (uint64, error) {
	threadID = strings.TrimSpace(threadID)
	if thread != nil && thread["contextEpochState"] != nil {
		state, err := domaincontextepoch.ParseState(thread["contextEpochState"])
		if err != nil {
			return 0, fmt.Errorf("invalid persisted context epoch state: %w", err)
		}
		if state.ThreadID != threadID {
			return 0, errors.New("context epoch state thread mismatch")
		}
		return state.AcceptedSnapshot.Epoch, nil
	}
	previous, found, err := LatestContext(thread)
	if err != nil {
		return 0, err
	}
	if found {
		if previous.ThreadID != threadID {
			return 0, errors.New("turn security state thread mismatch")
		}
		return previous.ContextEpoch, nil
	}
	return 1, nil
}

func AttachStartRecords(turn map[string]any, turnStartedEvent map[string]any, threadPatch map[string]any, context domainsecurity.TurnSecurityContext) {
	record := PublicRecord(context)
	turn["securityContext"] = cloneRecord(record)
	turnStartedEvent["securityContext"] = cloneRecord(record)
	threadPatch["securityState"] = cloneRecord(record)
}

func optionalPreviousContext(context domainsecurity.TurnSecurityContext, found bool) *domainsecurity.TurnSecurityContext {
	if !found {
		return nil
	}
	return &context
}

func SourceManifestHash(diagnostics []any) string {
	return domainsecurity.SourceManifestHash(diagnostics)
}

func SourceDiagnosticsForContext(source any, context domainsecurity.TurnSecurityContext) []any {
	if contextual, ok := source.(ContextualSourceDiagnosticsProvider); ok && contextual != nil {
		return contextual.ServerDiagnosticsForSecurityContext(context)
	}
	diagnostics, _ := source.(SourceDiagnosticsProvider)
	return SourceDiagnostics(diagnostics)
}

func SourceDiagnostics(source SourceDiagnosticsProvider) []any {
	if source == nil {
		return nil
	}
	return source.ServerDiagnostics()
}

func ErrorDetails(err error) map[string]any {
	code := strings.TrimSpace(err.Error())
	if !strings.HasPrefix(code, "turn_security_") {
		code = "turn_security_context_invalid"
	}
	return map[string]any{
		"code":  code,
		"error": "turn security context no longer matches the active workspace, case binding, or source manifest",
	}
}

func cloneRecord(input map[string]any) map[string]any {
	body, _ := json.Marshal(input)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return out
}
