package contextepoch

import (
	"errors"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type WorkspaceRebindResult struct {
	State           domaincontextepoch.State
	SecurityContext domainsecurity.TurnSecurityContext
}

// PrepareWorkspaceRebind advances the accepted epoch even when no provider
// turn is running. The current security binding must be the accepted registry
// high-water; the target binding then becomes a new monotonic snapshot.
func PrepareWorkspaceRebind(thread map[string]any, current, target domainsecurity.TurnSecurityContext, at time.Time) (WorkspaceRebindResult, error) {
	if domainsecurity.ValidateTurnSecurityContext(current) != nil || domainsecurity.ValidateTurnSecurityContext(target) != nil ||
		current.Version != domainsecurity.TurnSecurityContextVersionV2 || target.Version != domainsecurity.TurnSecurityContextVersionV2 ||
		current.ThreadID != target.ThreadID || current.TenantID != target.TenantID || current.UserID != target.UserID ||
		current.WorkspaceRealPath == target.WorkspaceRealPath || !domainsecurity.TurnSecurityContextIsGeneral(current) ||
		!domainsecurity.TurnSecurityContextIsGeneral(target) {
		return WorkspaceRebindResult{}, errors.New("workspace rebind security context is invalid")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	state, found, err := StateFromThread(thread)
	if err != nil {
		return WorkspaceRebindResult{}, err
	}
	if !found {
		state, err = BootstrapState(current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{SecurityBindingEntry(current)}, at)
		if err != nil {
			return WorkspaceRebindResult{}, err
		}
	}
	if state.AcceptedSnapshot.Epoch != current.ContextEpoch || !stateContainsExactSecurityBinding(state, current) {
		return WorkspaceRebindResult{}, errors.New("workspace rebind current epoch authority is inconsistent")
	}
	state, changed, err := Upsert(state, SecurityBindingEntry(target), false)
	if err != nil {
		return WorkspaceRebindResult{}, err
	}
	if !changed {
		return WorkspaceRebindResult{}, errors.New("workspace rebind did not change the security binding")
	}
	reconciled, err := Reconcile(ReconcileInput{State: state, Cause: CauseWorkspaceRebind, At: at})
	if err != nil {
		return WorkspaceRebindResult{}, err
	}
	if !reconciled.Changed || reconciled.State.AcceptedSnapshot.Epoch <= current.ContextEpoch {
		return WorkspaceRebindResult{}, errors.New("workspace rebind did not advance context epoch")
	}
	securityContext, err := securityContextAtEpoch(target, reconciled.State.AcceptedSnapshot.Epoch, at)
	if err != nil {
		return WorkspaceRebindResult{}, err
	}
	return WorkspaceRebindResult{State: reconciled.State, SecurityContext: securityContext}, nil
}

func stateContainsExactSecurityBinding(state domaincontextepoch.State, current domainsecurity.TurnSecurityContext) bool {
	expected := SecurityBindingEntry(current)
	for _, entry := range state.Registry {
		if entry.SourceID == SecurityBindingSourceID {
			return sameSourceConfiguration(entry, expected)
		}
	}
	return false
}
