package contextepoch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	contextsourceport "analytix.local/runtime-go/internal/ports/contextsource"
)

const SecurityBindingSourceID = "turn-security-binding"

type PrepareTurnInput struct {
	Thread          map[string]any
	SecurityContext domainsecurity.TurnSecurityContext
	At              time.Time
}

type PrepareTurnResult struct {
	State           domaincontextepoch.State
	SecurityContext domainsecurity.TurnSecurityContext
	Changed         bool
	Initialized     bool
}

type PrepareStartRecordsInput struct {
	Thread           map[string]any
	SecurityContext  domainsecurity.TurnSecurityContext
	At               time.Time
	Turn             map[string]any
	TurnStartedEvent map[string]any
	ThreadPatch      map[string]any
	Reader           contextsourceport.Reader
	Source           any
}

type PrepareStartRecordsResult struct {
	State           domaincontextepoch.State
	SecurityContext domainsecurity.TurnSecurityContext
	ProviderContext domaincontextepoch.ProviderContext
}

func PrepareAndAttachStartRecords(ctx context.Context, input PrepareStartRecordsInput) (PrepareStartRecordsResult, error) {
	prepared, err := PrepareTurn(PrepareTurnInput{Thread: input.Thread, SecurityContext: input.SecurityContext, At: input.At})
	if err != nil {
		return PrepareStartRecordsResult{}, err
	}
	turnsecurityapp.AttachStartRecords(input.Turn, input.TurnStartedEvent, input.ThreadPatch, prepared.SecurityContext)
	AttachStartRecords(input.Turn, input.TurnStartedEvent, input.ThreadPatch, prepared.State)
	providerContext, err := BuildProviderContext(ctx, prepared.State, input.Reader)
	if err != nil {
		return PrepareStartRecordsResult{}, err
	}
	if len(providerContext.Unavailable) > 0 {
		unavailable := make([]any, 0, len(providerContext.Unavailable))
		for _, source := range providerContext.Unavailable {
			unavailable = append(unavailable, map[string]any{"sourceId": source.SourceID, "reason": source.Reason})
		}
		input.Turn["contextSourceUnavailable"] = unavailable
		input.TurnStartedEvent["contextSourceUnavailable"] = cloneValue(unavailable)
	}
	return PrepareStartRecordsResult{State: prepared.State, SecurityContext: prepared.SecurityContext, ProviderContext: providerContext}, nil
}

func PrepareTurn(input PrepareTurnInput) (PrepareTurnResult, error) {
	if err := domainsecurity.ValidateTurnSecurityContext(input.SecurityContext); err != nil {
		return PrepareTurnResult{}, err
	}
	if input.SecurityContext.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return PrepareTurnResult{}, errors.New("turn security context V1 is audit-only")
	}
	at := input.At
	if at.IsZero() {
		parsed, err := time.Parse(time.RFC3339Nano, input.SecurityContext.IssuedAt)
		if err != nil {
			return PrepareTurnResult{}, err
		}
		at = parsed
	}
	entry := SecurityBindingEntry(input.SecurityContext)
	value := any(nil)
	if input.Thread != nil {
		value = input.Thread["contextEpochState"]
	}
	var state domaincontextepoch.State
	initialized := value == nil
	changed := false
	reconcileCurrent := !initialized
	if initialized {
		previous, hasPrevious, err := turnsecurityapp.LatestContext(input.Thread)
		if err != nil {
			return PrepareTurnResult{}, err
		}
		if hasPrevious {
			if previous.Version != domainsecurity.TurnSecurityContextVersionV2 {
				return PrepareTurnResult{}, errors.New("persisted turn security context V1 is audit-only")
			}
			if previous.ThreadID != input.SecurityContext.ThreadID {
				return PrepareTurnResult{}, errors.New("turn security state thread mismatch")
			}
			state, err = BootstrapState(input.SecurityContext.ThreadID, previous.ContextEpoch, []domaincontextepoch.SourceEntry{SecurityBindingEntry(previous)}, at)
			reconcileCurrent = true
		} else {
			state, err = BootstrapState(input.SecurityContext.ThreadID, input.SecurityContext.ContextEpoch, []domaincontextepoch.SourceEntry{entry}, at)
		}
		if err != nil {
			return PrepareTurnResult{}, err
		}
	} else {
		var err error
		state, _, err = ParseOrDefault(value, input.SecurityContext.ThreadID, input.SecurityContext.ContextEpoch, at)
		if err != nil {
			return PrepareTurnResult{}, err
		}
	}
	if reconcileCurrent {
		if input.SecurityContext.ContextEpoch != state.AcceptedSnapshot.Epoch {
			return PrepareTurnResult{}, errors.New("turn security context epoch does not match the accepted context epoch")
		}
		var err error
		state, _, err = Upsert(state, entry, false)
		if err != nil {
			return PrepareTurnResult{}, err
		}
		reconciled, err := Reconcile(ReconcileInput{State: state, Cause: CauseRequestBoundary, At: at})
		if err != nil {
			return PrepareTurnResult{}, err
		}
		state = reconciled.State
		changed = reconciled.Changed
	}
	securityContext, err := securityContextAtEpoch(input.SecurityContext, state.AcceptedSnapshot.Epoch, at)
	if err != nil {
		return PrepareTurnResult{}, err
	}
	return PrepareTurnResult{
		State: state, SecurityContext: securityContext, Changed: changed, Initialized: initialized,
	}, nil
}

func SecurityBindingEntry(context domainsecurity.TurnSecurityContext) domaincontextepoch.SourceEntry {
	bound := struct {
		WorkspaceRealPath       string `json:"workspaceRealPath"`
		TenantID                string `json:"tenantId"`
		UserID                  string `json:"userId"`
		CaseID                  string `json:"caseId"`
		CaseBindingHash         string `json:"caseBindingHash"`
		DatasetSnapshotID       string `json:"datasetSnapshotId"`
		SourceManifestHash      string `json:"sourceManifestHash"`
		ThreadRiskPolicyDigest  string `json:"threadRiskPolicyDigest"`
		PublicationPolicyDigest string `json:"publicationPolicyDigest"`
		RiskClass               string `json:"riskClass"`
		PublicationDisposition  string `json:"publicationDisposition"`
		CaseBindingState        string `json:"caseBindingState"`
		BindingObservation      string `json:"bindingObservationDigest"`
		PublicationBlocker      string `json:"publicationBlocker"`
		RiskAuthorityState      string `json:"riskAuthorityState"`
	}{
		WorkspaceRealPath: strings.TrimSpace(context.WorkspaceRealPath),
		TenantID:          strings.TrimSpace(context.TenantID), UserID: strings.TrimSpace(context.UserID),
		CaseID: strings.TrimSpace(context.CaseID), CaseBindingHash: strings.TrimSpace(context.CaseBindingHash),
		DatasetSnapshotID: strings.TrimSpace(context.DatasetSnapshotID), SourceManifestHash: strings.TrimSpace(context.SourceManifestHash),
		ThreadRiskPolicyDigest:  strings.TrimSpace(context.PublicationPolicy.ThreadRiskPolicyDigest),
		PublicationPolicyDigest: strings.TrimSpace(context.PublicationPolicy.PolicyDigest),
		RiskClass:               strings.TrimSpace(context.PublicationPolicy.RiskClass),
		PublicationDisposition:  strings.TrimSpace(context.PublicationPolicy.Disposition),
		CaseBindingState:        strings.TrimSpace(context.PublicationPolicy.CaseBindingState),
		BindingObservation:      strings.TrimSpace(context.PublicationPolicy.BindingObservationDigest),
		PublicationBlocker:      strings.TrimSpace(context.PublicationPolicy.BlockerCode),
		RiskAuthorityState:      strings.TrimSpace(context.RiskAuthorityBinding.State),
	}
	body, _ := json.Marshal(bound)
	return domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: SecurityBindingSourceID, Kind: "security-context",
		Digest: domaincontextepoch.SHA256Hex(body), Sequence: 1, TrustState: domaincontextepoch.TrustTrusted,
		PromptBoundary: domaincontextepoch.BoundaryStoreOnly, ActivationState: domaincontextepoch.ActivationInactive,
	}
}

func AttachStartRecords(turn map[string]any, turnStartedEvent map[string]any, threadPatch map[string]any, state domaincontextepoch.State) {
	threadPatch["contextEpochState"] = PublicState(state)
	snapshot := PublicSnapshot(state.AcceptedSnapshot)
	turn["contextEpochSnapshot"] = cloneMap(snapshot)
	turnStartedEvent["contextEpochSnapshot"] = cloneMap(snapshot)
}

func StateFromThread(thread map[string]any) (domaincontextepoch.State, bool, error) {
	if thread == nil || thread["contextEpochState"] == nil {
		return domaincontextepoch.State{}, false, nil
	}
	state, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil {
		return domaincontextepoch.State{}, false, errors.New("invalid persisted context epoch state: " + err.Error())
	}
	return state, true, nil
}

func securityContextAtEpoch(context domainsecurity.TurnSecurityContext, epoch uint64, _ time.Time) (domainsecurity.TurnSecurityContext, error) {
	if context.Version != domainsecurity.TurnSecurityContextVersionV2 || domainsecurity.ValidateTurnSecurityContext(context) != nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("turn security context V1 is audit-only")
	}
	issuedAt, _ := time.Parse(time.RFC3339Nano, context.IssuedAt)
	return domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: context.ThreadID, TurnID: context.TurnID, WorkspaceRealPath: context.WorkspaceRealPath,
		TenantID: context.TenantID, UserID: context.UserID, CaseID: context.CaseID, CaseBindingHash: context.CaseBindingHash,
		DatasetSnapshotID: context.DatasetSnapshotID, SourceManifestHash: context.SourceManifestHash, ContextEpoch: epoch, IssuedAt: issuedAt,
		PublicationPolicy: context.PublicationPolicy, RiskAuthorityBinding: context.RiskAuthorityBinding,
	})
}

func cloneMap(input map[string]any) map[string]any {
	body, _ := json.Marshal(input)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return out
}

func cloneValue(input any) any {
	body, _ := json.Marshal(input)
	var out any
	_ = json.Unmarshal(body, &out)
	return out
}
