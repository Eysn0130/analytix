package turnstart

import (
	"context"
	"errors"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type DurableStore interface {
	ReadThreadStartBaseline(string) (map[string]any, string, error)
	AppendTurnToThreadIfBaseline(string, map[string]any, string, map[string]any, string, string) error
	GetThread(string) (map[string]any, error)
}

type PreparedCommitInput struct {
	Context               context.Context
	HostContext           context.Context
	Store                 DurableStore
	CaseAuthority         casethreadapp.Authority
	Transition            *subagentapp.SecurityContextTransition
	Principal             domainidentity.PrincipalV1
	Reader                turnsecurityapp.WorkspaceReader
	SecurityAuthority     turnsecurityapp.WorkspaceSecurityAuthority
	Source                any
	InitialBaselineDigest string
	// PostBarrierThread is the exact baseline read by the parent-thread caller
	// after it acquired the transition writer. CommitPreparedTurn validates the
	// paired digest and identity, while the store append still performs the final
	// durable CAS. Child turns leave reuse disabled and retain the internal read.
	PostBarrierThread        map[string]any
	PostBarrierDigest        string
	ReusePostBarrierBaseline bool
	ThreadID                 string
	TurnID                   string
	Workspace                string
	ProviderID               string
	Records                  SecurityRecords
	RecordFactory            func(map[string]any, SecurityPreparation) (SecurityRecords, string, error)
	IssuedAt                 time.Time
	CommitAt                 time.Time
	Timeout                  time.Duration
	// PrepareWorkspaceBeforeFreeze is a host-only, workspace-scoped preparation
	// step. CommitPreparedTurn invokes it while the transition writer is held,
	// after the durable baseline and case quarantine checks, and before any
	// binding observation or TurnSecurityContext freeze. The callback receives
	// the exact canonical workspace concurrency identity acquired by the
	// transition and must not create or modify case authority material.
	PrepareWorkspaceBeforeFreeze func(context.Context, string) error
	// ValidateFrozen runs while the transition writer is held, after the live
	// workspace/case/snapshot freeze and before any case authority registration
	// or durable turn append. It is used for exact delegated-child revalidation.
	ValidateFrozen func(context.Context, domainsecurity.TurnSecurityContext) error
}

type CommitResult struct {
	Thread      map[string]any
	Preparation SecurityPreparation
	Records     SecurityRecords
	Appended    bool
}

// CommitPreparedTurn keeps the transition writer across authoritative baseline
// validation, final freeze/probe, exact old-job cancellation, durable baseline
// CAS, readback, and committed case authority record. The caller retains the
// transition after return so any post-append terminal closure can run before
// releasing it.
func CommitPreparedTurn(input PreparedCommitInput) (CommitResult, error) {
	if input.Context == nil || input.HostContext == nil || input.Store == nil || input.Transition == nil || input.Reader == nil {
		return CommitResult{}, errors.New("turn start commit dependencies are unavailable")
	}
	if err := turnsecurityapp.ValidateResolvedPrincipal(input.Context, input.SecurityAuthority.Identity, input.Principal); err != nil {
		return CommitResult{}, err
	}
	thread, baselineDigest, err := preparedCommitBaseline(input)
	if err != nil {
		return CommitResult{}, err
	}
	if err := ValidateReadback(thread, input.ThreadID, input.Workspace, baselineDigest, input.InitialBaselineDigest); err != nil {
		return CommitResult{}, err
	}
	if err := ValidateUnoccupiedTurnIDV1(thread, input.TurnID); err != nil {
		return CommitResult{}, err
	}
	if err := casethreadapp.RequireExecutable(input.CaseAuthority, input.ThreadID); err != nil {
		return CommitResult{}, err
	}
	if err := prepareWorkspaceBeforeFreeze(input); err != nil {
		return CommitResult{}, err
	}
	securityRecords := input.Records
	if input.RecordFactory != nil {
		securityRecords = SecurityRecords{Turn: map[string]any{}, TurnStartedEvent: map[string]any{}, ThreadPatch: map[string]any{}}
	}
	frozenHooks := make([]func(context.Context, domainsecurity.TurnSecurityContext) error, 0, 2)
	if input.ValidateFrozen != nil {
		frozenHooks = append(frozenHooks, input.ValidateFrozen)
	}
	frozenHooks = append(frozenHooks,
		func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			return casethreadapp.RegistrationHook(input.CaseAuthority)(ctx, securityContext)
		},
	)
	preparation, err := PrepareSecurityTransition(
		input.Context, input.Transition, input.Principal, input.Reader, input.SecurityAuthority, input.Source,
		thread, input.ThreadID, input.TurnID, input.Workspace,
		input.IssuedAt, securityRecords, input.Timeout, frozenHooks...,
	)
	if err != nil {
		return CommitResult{}, err
	}
	providerID := input.ProviderID
	commitRecords := input.Records
	if input.RecordFactory != nil {
		commitRecords, providerID, err = input.RecordFactory(thread, preparation)
		if err != nil {
			return CommitResult{Preparation: preparation}, err
		}
		if commitRecords.Turn == nil || commitRecords.TurnStartedEvent == nil || commitRecords.ThreadPatch == nil {
			return CommitResult{Preparation: preparation}, errors.New("turn start record factory returned incomplete records")
		}
		turnsecurityapp.AttachStartRecords(commitRecords.Turn, commitRecords.TurnStartedEvent, commitRecords.ThreadPatch, preparation.SecurityContext)
		contextepochapp.AttachStartRecords(commitRecords.Turn, commitRecords.TurnStartedEvent, commitRecords.ThreadPatch, preparation.EpochState)
	}
	result := CommitResult{Preparation: preparation, Records: commitRecords}
	if err := input.Store.AppendTurnToThreadIfBaseline(
		input.ThreadID, commitRecords.Turn, providerID, commitRecords.ThreadPatch, input.Workspace, baselineDigest,
	); err != nil {
		return result, err
	}
	result.Appended = true
	thread, err = input.Store.GetThread(input.ThreadID)
	if err != nil {
		return result, err
	}
	if thread == nil {
		return result, errors.New("started turn durable readback is unavailable")
	}
	frozen, err := appturn.FrozenSecurityContextForTurn(thread, input.TurnID)
	if err != nil || frozen != preparation.SecurityContext {
		return result, errors.New("started turn durable security context readback is inconsistent")
	}
	if err := casethreadapp.CommitRequired(
		input.HostContext, input.CaseAuthority, preparation.SecurityContext, preparation.EpochState, input.CommitAt,
	); err != nil {
		return result, err
	}
	result.Thread = thread
	return result, nil
}

func preparedCommitBaseline(input PreparedCommitInput) (map[string]any, string, error) {
	if !input.ReusePostBarrierBaseline {
		return input.Store.ReadThreadStartBaseline(input.ThreadID)
	}
	if input.PostBarrierThread == nil || input.PostBarrierDigest == "" {
		return nil, "", ErrBaselineConflict
	}
	return input.PostBarrierThread, input.PostBarrierDigest, nil
}

func prepareWorkspaceBeforeFreeze(input PreparedCommitInput) error {
	if input.PrepareWorkspaceBeforeFreeze == nil {
		return nil
	}
	if input.Context == nil || input.Transition == nil || input.Reader == nil {
		return errors.New("turn start workspace preparation dependencies are unavailable")
	}
	expectedWorkspace, err := input.Transition.ScopeWorkspaceRealPath()
	if err != nil {
		return err
	}
	currentWorkspace, err := input.Reader.WorkspaceRealPath(input.Workspace)
	if err != nil || currentWorkspace != expectedWorkspace {
		return errors.New("turn start workspace changed before preparation")
	}
	if err := input.PrepareWorkspaceBeforeFreeze(input.Context, expectedWorkspace); err != nil {
		return err
	}
	currentWorkspace, err = input.Reader.WorkspaceRealPath(input.Workspace)
	if err != nil || currentWorkspace != expectedWorkspace {
		return errors.New("turn start workspace changed during preparation")
	}
	return nil
}
