package thread

import (
	"errors"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	appcontextepoch "analytix.local/runtime-go/internal/app/contextepoch"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
)

type CurrentCaseThreadAuthorityValidator interface {
	CaseThreadAuthority
	ValidateCurrent(string, map[string]any) (domainsecurity.TurnSecurityContext, error)
}

type CurrentCaseThreadAuthority interface {
	CaseThreadAuthority
	ContextTurnIDs(string) []string
}

type currentCaseThreadAuthorityValidator struct {
	authority CurrentCaseThreadAuthority
	bindings  casecontextport.CurrentBindingReader
	snapshots casecontextport.CurrentSnapshotValidator
}

func NewCurrentCaseThreadAuthorityValidator(
	authority CurrentCaseThreadAuthority,
	bindings casecontextport.CurrentBindingReader,
	snapshots casecontextport.CurrentSnapshotValidator,
) CurrentCaseThreadAuthorityValidator {
	return &currentCaseThreadAuthorityValidator{authority: authority, bindings: bindings, snapshots: snapshots}
}

func validateCurrentCaseThreadAuthority(validator CurrentCaseThreadAuthorityValidator, threadID string, thread map[string]any) (domainsecurity.TurnSecurityContext, error) {
	if validator == nil {
		return domainsecurity.TurnSecurityContext{}, errors.New("current case thread authority validator is unavailable")
	}
	return validator.ValidateCurrent(threadID, thread)
}

func (validator *currentCaseThreadAuthorityValidator) IsCaseThread(threadID string) bool {
	return validator != nil && validator.authority != nil && validator.authority.IsCaseThread(strings.TrimSpace(threadID))
}

func (validator *currentCaseThreadAuthorityValidator) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	return validator != nil && validator.authority != nil && validator.authority.ContainsContext(securityContext)
}

func (validator *currentCaseThreadAuthorityValidator) RestartPreservesThreadV1(threadID string) bool {
	return validator != nil && validator.authority != nil && validator.authority.RestartPreservesThreadV1(threadID)
}

// CommittedContext forwards the existing installation-signed context/epoch
// read seam without turning the public projector into a registry writer.
func (validator *currentCaseThreadAuthorityValidator) CommittedContext(
	threadID string,
	turnID string,
) (casethreadapp.CommittedContext, bool) {
	if validator == nil || validator.authority == nil {
		return casethreadapp.CommittedContext{}, false
	}
	committed, ok := validator.authority.(interface {
		CommittedContext(string, string) (casethreadapp.CommittedContext, bool)
	})
	if !ok {
		return casethreadapp.CommittedContext{}, false
	}
	return committed.CommittedContext(strings.TrimSpace(threadID), strings.TrimSpace(turnID))
}

func (validator *currentCaseThreadAuthorityValidator) ValidateCurrent(threadID string, thread map[string]any) (domainsecurity.TurnSecurityContext, error) {
	threadID = strings.TrimSpace(threadID)
	if validator.RestartPreservesThreadV1(threadID) {
		return domainsecurity.TurnSecurityContext{}, casethreadapp.ErrRestartPreserved
	}
	if validator == nil || validator.authority == nil || validator.bindings == nil || thread == nil ||
		threadID == "" || strings.TrimSpace(contracts.StringField(thread, "id")) != threadID || !validator.authority.IsCaseThread(threadID) {
		return domainsecurity.TurnSecurityContext{}, errors.New("current case thread authority is unavailable")
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || securityContext.ThreadID != threadID ||
		!validator.authority.ContainsContext(securityContext) || !currentContextIsAuthorityHighWater(validator.authority, threadID, thread, securityContext) {
		return domainsecurity.TurnSecurityContext{}, errors.New("current case thread security context is not signed by host authority")
	}
	state, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil || !currentEpochMatchesSecurityContext(state, securityContext) {
		return domainsecurity.TurnSecurityContext{}, errors.New("current case thread epoch does not match host authority")
	}
	workspace := strings.TrimSpace(contracts.StringField(thread, "workspace"))
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		observer, ok := validator.bindings.(casecontextport.Observer)
		if !ok {
			return domainsecurity.TurnSecurityContext{}, errors.New("current case boundary observation authority is unavailable")
		}
		observation, observeErr := observer.Observe(workspace)
		if observeErr != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
			observation.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
			observation.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
			return domainsecurity.TurnSecurityContext{}, errors.New("current case boundary observation does not match host authority")
		}
		return securityContext, nil
	}
	if securityContext.CaseID == domainsecurity.UnboundCaseID {
		return domainsecurity.TurnSecurityContext{}, errors.New("current case thread execution context is unbound")
	}
	binding, err := validator.bindings.ReadCurrentBinding(workspace)
	if err != nil || binding.WorkspaceRealPath != securityContext.WorkspaceRealPath || binding.CaseID != securityContext.CaseID ||
		binding.CaseBindingHash != securityContext.CaseBindingHash {
		return domainsecurity.TurnSecurityContext{}, errors.New("current case binding does not match host authority")
	}
	if validator.snapshots != nil {
		if err := validator.snapshots.ValidateCurrentSnapshot(securityContext); err != nil {
			return domainsecurity.TurnSecurityContext{}, errors.New("current dataset snapshot does not match host authority")
		}
	}
	return securityContext, nil
}

func currentContextIsAuthorityHighWater(authority CurrentCaseThreadAuthority, threadID string, thread map[string]any, current domainsecurity.TurnSecurityContext) bool {
	turnIDs := authority.ContextTurnIDs(threadID)
	if len(turnIDs) == 0 {
		return false
	}
	wanted := make(map[string]bool, len(turnIDs))
	for _, turnID := range turnIDs {
		turnID = strings.TrimSpace(turnID)
		if turnID == "" || wanted[turnID] {
			return false
		}
		wanted[turnID] = true
	}
	var latest domainsecurity.TurnSecurityContext
	latestAt := time.Time{}
	latestFound := false
	latestAmbiguous := false
	seen := map[string]bool{}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return false
	}
	for _, value := range turns {
		turn, ok := value.(map[string]any)
		if !ok {
			return false
		}
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if !wanted[turnID] {
			continue
		}
		if seen[turnID] {
			return false
		}
		securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		issuedAt, issuedAtErr := time.Parse(time.RFC3339Nano, securityContext.IssuedAt)
		if err != nil || issuedAtErr != nil || securityContext.ThreadID != threadID || securityContext.TurnID != turnID ||
			!authority.ContainsContext(securityContext) {
			return false
		}
		seen[turnID] = true
		if !latestFound || securityContext.ContextEpoch > latest.ContextEpoch ||
			(securityContext.ContextEpoch == latest.ContextEpoch && issuedAt.After(latestAt)) {
			latest = securityContext
			latestAt = issuedAt
			latestFound = true
			latestAmbiguous = false
		} else if securityContext.ContextEpoch == latest.ContextEpoch && issuedAt.Equal(latestAt) && securityContext != latest {
			latestAmbiguous = true
		}
	}
	return latestFound && !latestAmbiguous && len(seen) == len(wanted) && latest == current
}

func currentEpochMatchesSecurityContext(state domaincontextepoch.State, securityContext domainsecurity.TurnSecurityContext) bool {
	if state.ThreadID != securityContext.ThreadID || state.AcceptedSnapshot.ThreadID != securityContext.ThreadID ||
		state.AcceptedSnapshot.Epoch != securityContext.ContextEpoch ||
		state.AcceptedSnapshot.RegistryDigest != domaincontextepoch.RegistryDigest(state.Registry) {
		return false
	}
	expected := appcontextepoch.SecurityBindingEntry(securityContext)
	registryMatches := false
	for _, entry := range state.Registry {
		if entry.SourceID == appcontextepoch.SecurityBindingSourceID {
			registryMatches = sameSecurityBindingEntry(entry, expected)
			break
		}
	}
	snapshotMatches := false
	for _, source := range state.AcceptedSnapshot.Sources {
		if source.SourceID == appcontextepoch.SecurityBindingSourceID {
			snapshotMatches = source.Kind == expected.Kind && source.Digest == expected.Digest &&
				source.TrustState == expected.TrustState && source.PromptBoundary == expected.PromptBoundary &&
				source.TokenBudget == expected.TokenBudget && source.ActivationState == expected.ActivationState &&
				source.ActivationReason == expected.ActivationReason
			break
		}
	}
	return registryMatches && snapshotMatches
}

func sameSecurityBindingEntry(current, expected domaincontextepoch.SourceEntry) bool {
	return current.SourceID == expected.SourceID && current.Kind == expected.Kind && current.Reference == expected.Reference &&
		current.Digest == expected.Digest && current.TrustState == expected.TrustState && current.PromptBoundary == expected.PromptBoundary &&
		current.TokenBudget == expected.TokenBudget && current.ActivationState == expected.ActivationState &&
		current.ActivationReason == expected.ActivationReason
}
