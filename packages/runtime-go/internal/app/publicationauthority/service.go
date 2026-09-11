package publicationauthority

import (
	"context"
	"errors"
	"sync"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type CaseLineage interface {
	IsCaseThread(string) bool
}

type TerminalArbitrator interface {
	AcquireCandidateTerminal(context.Context, string, string, string) (func(), error)
	AcquireHostTerminal(context.Context, string, string, string) (func(), error)
}

type loopTerminalArbitrator interface {
	AcquireCandidateTerminalForLoop(context.Context, string, string, string) (context.Context, func(), error)
}

type Dependencies struct {
	AcquireContextEffect appturn.AcquireGeneralPublicationEffectFunc
	ValidateCurrent      appturn.ValidateCurrentGeneralPublicationFunc
	AcquireContextEffectForAuthority func(context.Context, domainsecurity.TurnSecurityContext, bool) (context.Context, func(), error)
	ValidateCurrentForAuthority      func(context.Context, domainsecurity.TurnSecurityContext, bool) error
	CaseLineage          CaseLineage
	TerminalArbitrator   TerminalArbitrator
	CaseFinalizer        evidenceapp.CasePublicationFinalizer
	HostContext          func(context.Context) (context.Context, context.CancelFunc)
}

type Service struct {
	deps Dependencies
}

func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

func (s *Service) AcquireCurrentCandidateTerminalForLoop(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	caseDataEffect bool,
) (context.Context, func(), error) {
	if s == nil || ctx == nil || s.deps.TerminalArbitrator == nil {
		return nil, nil, errors.New("loop candidate terminal authority is unavailable")
	}
	if domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext) {
		if err := s.ValidateCaseTerminalLineage(securityContext); err != nil {
			return nil, nil, err
		}
	} else if err := s.ValidateGeneralTerminalLineage(securityContext); err != nil {
		return nil, nil, err
	}
	effectContext, releaseEffect, err := s.acquireContextEffect(ctx, securityContext, caseDataEffect)
	if err != nil || effectContext == nil || releaseEffect == nil {
		if releaseEffect != nil {
			releaseEffect()
		}
		if err == nil {
			err = errors.New("loop candidate publication effect lease is invalid")
		}
		return nil, nil, err
	}
	if err := s.validateCurrent(effectContext, securityContext, caseDataEffect); err != nil {
		releaseEffect()
		return nil, nil, errors.Join(errors.New("loop candidate publication authority changed"), err)
	}
	arbitrator, ok := s.deps.TerminalArbitrator.(loopTerminalArbitrator)
	if !ok {
		releaseEffect()
		return nil, nil, errors.New("loop candidate terminal arbitrator is unavailable")
	}
	terminalContext, releaseTerminal, err := arbitrator.AcquireCandidateTerminalForLoop(
		effectContext, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
	)
	if err != nil || terminalContext == nil || releaseTerminal == nil {
		if releaseTerminal != nil {
			releaseTerminal()
		}
		releaseEffect()
		if err == nil {
			err = errors.New("loop candidate terminal claim is invalid")
		}
		return nil, nil, err
	}
	var once sync.Once
	return terminalContext, func() {
		once.Do(func() {
			releaseTerminal()
			releaseEffect()
		})
	}, nil
}

func (s *Service) CommitCurrentGeneralCompletion(
	ctx context.Context,
	input appturn.CommitCompletionInput,
) (appturn.CommitCompletionResult, error) {
	return appturn.CommitCurrentGeneralCompletion(ctx, s.currentGeneralAuthority(), input)
}

func (s *Service) FinalizeCurrentGeneralAfterLoop(ctx context.Context, input appturn.FinalizeAfterLoopInput) error {
	return appturn.FinalizeCurrentGeneralAfterLoop(ctx, s.currentGeneralAuthority(), input)
}

func (s *Service) WithCurrentGeneralFixedTerminal(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	persist func(context.Context) error,
) error {
	if s == nil || ctx == nil || persist == nil || s.deps.TerminalArbitrator == nil {
		return errors.New("fixed general terminal authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ValidateGeneralTerminalLineage(securityContext); err != nil {
		return err
	}
	release, err := s.deps.TerminalArbitrator.AcquireHostTerminal(
		ctx, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
	)
	if err != nil || release == nil {
		if err == nil {
			err = errors.New("fixed general terminal permit is invalid")
		}
		return errors.Join(errors.New("fixed general terminal arbitration failed"), err)
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	return persist(ctx)
}

func (s *Service) ValidateGeneralTerminalLineage(securityContext domainsecurity.TurnSecurityContext) error {
	if s == nil || !domainsecurity.TurnSecurityContextIsGeneral(securityContext) {
		return errors.New("general terminal context is invalid")
	}
	if s.deps.CaseLineage == nil {
		return errors.New("case lineage registry is unavailable")
	}
	if s.deps.CaseLineage.IsCaseThread(securityContext.ThreadID) {
		return errors.New("signed case lineage cannot use a general terminal")
	}
	return nil
}

func (s *Service) ValidateCaseTerminalLineage(securityContext domainsecurity.TurnSecurityContext) error {
	if s == nil || s.deps.CaseLineage == nil {
		return errors.New("case terminal lineage authority is unavailable")
	}
	if !domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext) {
		if s.deps.CaseLineage.IsCaseThread(securityContext.ThreadID) {
			return errors.New("signed case lineage cannot use a general terminal context")
		}
		return errors.New("case terminal context is invalid")
	}
	if !s.deps.CaseLineage.IsCaseThread(securityContext.ThreadID) {
		return errors.New("case terminal context is missing signed case lineage")
	}
	return nil
}

func (s *Service) PersistCurrentCaseCandidate(
	operationContext context.Context,
	input evidenceapp.PersistCaseBoundaryInput,
	caseDataEffects ...bool,
) (evidenceapp.PersistCaseBoundaryResult, error) {
	if s == nil || operationContext == nil ||
		s.deps.TerminalArbitrator == nil || s.deps.CaseFinalizer == nil || s.deps.HostContext == nil {
		return evidenceapp.PersistCaseBoundaryResult{}, errors.New("case candidate operation context is unavailable")
	}
	if err := s.ValidateCaseTerminalLineage(input.Context); err != nil {
		return evidenceapp.PersistCaseBoundaryResult{}, err
	}
	caseDataEffect := true
	if len(caseDataEffects) > 1 {
		return evidenceapp.PersistCaseBoundaryResult{}, errors.New("case candidate effect binding is invalid")
	}
	if len(caseDataEffects) == 1 {
		caseDataEffect = caseDataEffects[0]
	}
	effectContext, releaseEffect, err := s.acquireContextEffect(operationContext, input.Context, caseDataEffect)
	if err != nil || effectContext == nil || releaseEffect == nil {
		if err == nil {
			err = errors.New("case candidate context effect lease is invalid")
		}
		return evidenceapp.PersistCaseBoundaryResult{}, errors.Join(errors.New("case candidate publication authority changed"), err)
	}
	defer releaseEffect()
	release, err := s.deps.TerminalArbitrator.AcquireCandidateTerminal(
		effectContext, input.Context.ThreadID, input.Context.TurnID, input.Context.ContextDigest,
	)
	if err != nil || release == nil {
		if err == nil {
			err = errors.New("case candidate terminal permit is invalid")
		}
		return evidenceapp.PersistCaseBoundaryResult{}, errors.Join(errors.New("case candidate terminal arbitration failed"), err)
	}
	defer release()
	hostContext, cancel := s.deps.HostContext(effectContext)
	if hostContext == nil || cancel == nil {
		if cancel != nil {
			cancel()
		}
		return evidenceapp.PersistCaseBoundaryResult{}, errors.New("case candidate host context is unavailable")
	}
	defer cancel()
	return evidenceapp.PersistCaseTerminalBoundary(hostContext, s.deps.CaseFinalizer, input)
}

func (s *Service) PersistCurrentCaseFixed(
	ctx context.Context,
	input evidenceapp.PersistCaseBoundaryInput,
) (evidenceapp.PersistCaseBoundaryResult, error) {
	if s == nil || ctx == nil || s.deps.TerminalArbitrator == nil || s.deps.CaseFinalizer == nil {
		return evidenceapp.PersistCaseBoundaryResult{}, errors.New("case fixed terminal context is unavailable")
	}
	if err := s.ValidateCaseTerminalLineage(input.Context); err != nil {
		return evidenceapp.PersistCaseBoundaryResult{}, err
	}
	release, err := s.deps.TerminalArbitrator.AcquireHostTerminal(
		ctx, input.Context.ThreadID, input.Context.TurnID, input.Context.ContextDigest,
	)
	if err != nil || release == nil {
		if err == nil {
			err = errors.New("case fixed terminal permit is invalid")
		}
		return evidenceapp.PersistCaseBoundaryResult{}, errors.Join(errors.New("case fixed terminal arbitration failed"), err)
	}
	defer release()
	return evidenceapp.PersistCaseTerminalBoundary(ctx, s.deps.CaseFinalizer, input)
}

func (s *Service) currentGeneralAuthority() appturn.CurrentGeneralPublicationAuthority {
	if s == nil {
		return appturn.CurrentGeneralPublicationAuthority{}
	}
	return appturn.CurrentGeneralPublicationAuthority{
		AcquireContextEffect: func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return s.acquireContextEffect(ctx, securityContext, false)
		},
		ValidateCurrent: func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			return s.validateCurrent(ctx, securityContext, false)
		},
		CaseLineageContains: func(threadID string) (bool, error) {
			if s.deps.CaseLineage == nil {
				return false, errors.New("case lineage registry is unavailable")
			}
			return s.deps.CaseLineage.IsCaseThread(threadID), nil
		},
		AcquireTerminalCAS: func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (func(), error) {
			if s.deps.TerminalArbitrator == nil {
				return nil, errors.New("terminal arbitrator is unavailable")
			}
			return s.deps.TerminalArbitrator.AcquireCandidateTerminal(
				ctx, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
			)
		},
	}
}

func (s *Service) acquireContextEffect(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	caseDataEffect bool,
) (context.Context, func(), error) {
	if s == nil {
		return nil, nil, errors.New("publication context effect authority is unavailable")
	}
	if s.deps.AcquireContextEffectForAuthority != nil {
		return s.deps.AcquireContextEffectForAuthority(ctx, securityContext, caseDataEffect)
	}
	if s.deps.AcquireContextEffect == nil {
		return nil, nil, errors.New("publication context effect authority is unavailable")
	}
	return s.deps.AcquireContextEffect(ctx, securityContext)
}

func (s *Service) validateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	caseDataEffect bool,
) error {
	if s == nil {
		return errors.New("publication currentness authority is unavailable")
	}
	if s.deps.ValidateCurrentForAuthority != nil {
		return s.deps.ValidateCurrentForAuthority(ctx, securityContext, caseDataEffect)
	}
	if s.deps.ValidateCurrent == nil {
		return errors.New("publication currentness authority is unavailable")
	}
	return s.deps.ValidateCurrent(ctx, securityContext)
}
