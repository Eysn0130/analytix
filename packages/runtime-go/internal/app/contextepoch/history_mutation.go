package contextepoch

import (
	"errors"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type HistoryMutationResult struct {
	State           domaincontextepoch.State
	SecurityContext domainsecurity.TurnSecurityContext
}

// PrepareHistoryMutation replaces the accepted security binding with a
// synthetic host turn and advances the shared epoch. It is used only after
// foreground/background effects for the old context have been stopped.
func PrepareHistoryMutation(thread map[string]any, current, target domainsecurity.TurnSecurityContext, at time.Time) (HistoryMutationResult, error) {
	if domainsecurity.ValidateTurnSecurityContext(current) != nil || domainsecurity.ValidateTurnSecurityContext(target) != nil ||
		current.Version != domainsecurity.TurnSecurityContextVersionV2 || target.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		current.ThreadID != target.ThreadID || current.WorkspaceRealPath != target.WorkspaceRealPath ||
		current.TenantID != target.TenantID || current.UserID != target.UserID || !domainsecurity.TurnSecurityContextIsGeneral(current) ||
		!domainsecurity.TurnSecurityContextIsGeneral(target) || current.TurnID == target.TurnID {
		return HistoryMutationResult{}, errors.New("history mutation security context is invalid")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	state, found, err := StateFromThread(thread)
	if err != nil {
		return HistoryMutationResult{}, err
	}
	if !found {
		state, err = BootstrapState(current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{SecurityBindingEntry(current)}, at)
		if err != nil {
			return HistoryMutationResult{}, err
		}
	}
	if state.AcceptedSnapshot.Epoch != current.ContextEpoch || !stateContainsExactSecurityBinding(state, current) {
		return HistoryMutationResult{}, errors.New("history mutation current epoch authority is inconsistent")
	}
	state, _, err = Upsert(state, SecurityBindingEntry(target), false)
	if err != nil {
		return HistoryMutationResult{}, err
	}
	state, changed, err := Upsert(state, domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: "host-history-mutation", Kind: "history-authority",
		Digest: domaincontextepoch.SHA256Hex([]byte(target.ContextDigest)), TrustState: domaincontextepoch.TrustTrusted,
		PromptBoundary: domaincontextepoch.BoundaryStoreOnly, ActivationState: domaincontextepoch.ActivationInactive,
	}, false)
	if err != nil || !changed {
		return HistoryMutationResult{}, errors.Join(errors.New("history mutation authority did not change"), err)
	}
	reconciled, err := Reconcile(ReconcileInput{State: state, Cause: CauseRequestBoundary, At: at})
	if err != nil {
		return HistoryMutationResult{}, err
	}
	if !reconciled.Changed || reconciled.State.AcceptedSnapshot.Epoch <= current.ContextEpoch {
		return HistoryMutationResult{}, errors.New("history mutation did not advance context epoch")
	}
	securityContext, err := securityContextAtEpoch(target, reconciled.State.AcceptedSnapshot.Epoch, at)
	if err != nil {
		return HistoryMutationResult{}, err
	}
	return HistoryMutationResult{State: reconciled.State, SecurityContext: securityContext}, nil
}
