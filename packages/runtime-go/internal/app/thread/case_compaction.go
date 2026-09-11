package thread

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func (s *Service) prepareCompactionForCurrentThread(
	ctx context.Context,
	thread map[string]any,
	threadID, reason string,
	at time.Time,
	auto bool,
	prior *CaseCompactionAuthorization,
) (PreparedCompaction, *CaseCompactionAuthorization, error) {
	if err := s.requireRestartWritableV1(threadID); err != nil {
		return PreparedCompaction{}, nil, err
	}
	if strings.EqualFold(strings.TrimSpace(contracts.StringField(thread, "status")), "running") {
		return PreparedCompaction{}, nil, ErrThreadRunning
	}
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil {
		return PreparedCompaction{}, nil, err
	}
	knownCase := s.caseThreads != nil && s.caseThreads.IsCaseThread(threadID)
	if !caseSensitive && !knownCase {
		if prior != nil {
			return PreparedCompaction{}, nil, errors.New("case compaction authority changed before preparation")
		}
		prepared, err := prepareCompaction(thread, threadID, reason, at, auto, nil)
		return prepared, nil, err
	}
	if !knownCase {
		return PreparedCompaction{}, nil, ErrCaseCompactionRequiresTrustedArchive
	}
	current, authorityTurnIDs, err := s.validateCurrentCaseCompactionAuthority(ctx, thread, threadID)
	if err != nil {
		return PreparedCompaction{}, nil, errors.Join(ErrCaseCompactionRequiresTrustedArchive, err)
	}
	continuation, continuationErr := turnapp.BuildTaskContinuationSnapshotV1(thread)
	if continuationErr != nil {
		return PreparedCompaction{}, nil, errors.Join(ErrCaseCompactionRequiresTrustedArchive, continuationErr)
	}
	inherited, err := ActiveInheritedCompactionRecordV1(ctx, thread, s.caseCompactionAuthority, s.repository)
	if err != nil {
		return PreparedCompaction{}, nil, err
	}
	continuationAuthorization := CaseCompactionAuthorization{
		Continuation: continuation, SourceContextDigest: current.ContextDigest,
		AuthorityTurnIDs: authorityTurnIDs, ActiveInheritedHistory: inherited,
	}
	if prior != nil &&
		(!reflect.DeepEqual(authorityTurnIDs, canonicalCompactionAuthorityTurnIDs(prior.AuthorityTurnIDs)) ||
			prior.SourceContextDigest != current.ContextDigest ||
			continuation.StateDigest != prior.Continuation.StateDigest || !reflect.DeepEqual(inherited, prior.ActiveInheritedHistory)) {
		return PreparedCompaction{}, nil, ErrCompactionBaselineConflict
	}
	prepared, err := PrepareCaseCompaction(
		thread, threadID, reason, at, auto, continuationAuthorization,
	)
	if err != nil {
		return PreparedCompaction{}, nil, err
	}
	return prepared, &continuationAuthorization, nil
}

func (s *Service) validateCurrentCaseCompactionAuthority(
	ctx context.Context,
	thread map[string]any,
	threadID string,
) (domainsecurity.TurnSecurityContext, []string, error) {
	authority := s.caseCompactionAuthority
	if authority == nil || !authority.IsCaseThread(threadID) || !authority.CanExecute(threadID) {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("committed case thread authority is unavailable")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err == nil && current.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("turn security context V1 is audit-only")
	}
	if err != nil || current.ThreadID != threadID ||
		domainsecurity.ValidateTurnSecurityContextForCasePublication(current) != nil {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("current case compaction context is invalid")
	}
	state, err := domaincontextepoch.ParseState(thread["contextEpochState"])
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("current case compaction epoch is invalid")
	}
	committedCurrent, found := authority.CommittedContext(threadID, current.TurnID)
	if !found || committedCurrent.SecurityContext != current || !reflect.DeepEqual(committedCurrent.EpochState, state) {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("current case compaction context is not committed")
	}
	committedTurnIDs := casethreadapp.CommittedTurnIDs(authority, threadID)
	turnIDs := canonicalCompactionAuthorityTurnIDs(committedTurnIDs)
	if len(turnIDs) == 0 || len(turnIDs) != len(committedTurnIDs) {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("case compaction authority inventory is invalid")
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return domainsecurity.TurnSecurityContext{}, nil, errors.New("case compaction durable turns are invalid")
	}
	durable := make(map[string]map[string]any, len(turns))
	for _, raw := range turns {
		turn, structured := raw.(map[string]any)
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if !structured || turnID == "" || durable[turnID] != nil {
			return domainsecurity.TurnSecurityContext{}, nil, errors.New("case compaction durable turn identity is invalid")
		}
		durable[turnID] = turn
	}
	for _, turnID := range turnIDs {
		turn := durable[turnID]
		frozen, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		committed, committedFound := authority.CommittedContext(threadID, turnID)
		if turn == nil || parseErr != nil || !committedFound || frozen != committed.SecurityContext ||
			frozen.ThreadID != threadID || frozen.TurnID != turnID {
			return domainsecurity.TurnSecurityContext{}, nil, errors.New("case compaction would orphan committed turn authority")
		}
	}
	validation := turnsecurityapp.CurrentValidationInput{
		OperationContext: ctx, Identity: s.workspaceSecurity.Identity,
		Observer: s.workspaceSecurity.Observer, RiskAuthority: s.workspaceSecurity.RiskAuthority,
		SnapshotAuthority:   s.workspaceSecurity.SnapshotAuthority,
		SnapshotAuthorityV2: s.workspaceSecurity.SnapshotAuthorityV2,
		Context:             current, Workspace: strings.TrimSpace(contracts.StringField(thread, "workspace")),
	}
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(current) {
		// This is a host-only archive transition: it cannot authorize a
		// provider, tool, case fact, or arbitrary publication. The signed case
		// archive and fixed public marker are revalidated separately.
		err = turnsecurityapp.ValidateCurrentHostBoundary(validation)
	} else {
		err = turnsecurityapp.ValidateCurrent(validation)
	}
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, nil, err
	}
	return current, turnIDs, nil
}

func (s *Service) revalidateCaseCompactionImmediatelyBeforeCommit(
	ctx context.Context,
	thread map[string]any,
	prepared PreparedCompaction,
) error {
	if !prepared.CaseBound {
		return nil
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || current.ContextDigest != prepared.CaseSourceContextDigest {
		return ErrCompactionBaselineConflict
	}
	validation := turnsecurityapp.CurrentValidationInput{
		OperationContext: ctx, Identity: s.workspaceSecurity.Identity,
		Observer: s.workspaceSecurity.Observer, RiskAuthority: s.workspaceSecurity.RiskAuthority,
		SnapshotAuthority:   s.workspaceSecurity.SnapshotAuthority,
		SnapshotAuthorityV2: s.workspaceSecurity.SnapshotAuthorityV2,
		Context:             current, Workspace: strings.TrimSpace(contracts.StringField(thread, "workspace")),
	}
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(current) {
		// See validateCurrentCaseCompactionAuthority: the boundary validator is
		// used only for the signed host compaction transaction.
		return turnsecurityapp.ValidateCurrentHostBoundary(validation)
	}
	return turnsecurityapp.ValidateCurrent(validation)
}
