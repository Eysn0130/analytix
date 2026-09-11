package turn

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type AcquireGeneralPublicationEffectFunc func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) (context.Context, func(), error)

type ValidateCurrentGeneralPublicationFunc func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) error

type AcquireGeneralTerminalPermitFunc func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) (func(), error)

type CurrentGeneralPublicationAuthority struct {
	AcquireContextEffect AcquireGeneralPublicationEffectFunc
	ValidateCurrent      ValidateCurrentGeneralPublicationFunc
	CaseLineageContains  func(string) (bool, error)
	AcquireTerminalCAS   AcquireGeneralTerminalPermitFunc
}

// WithCurrentGeneralPublicationAuthority holds the context-effect lease
// across the final authority check and atomic general-terminal CAS. The signed
// risk head, exact missing binding observation, and absence of signed case
// lineage are host authority; provider text never contributes to admission.
func WithCurrentGeneralPublicationAuthority(
	ctx context.Context,
	authority CurrentGeneralPublicationAuthority,
	securityContext domainsecurity.TurnSecurityContext,
	publish func(context.Context) error,
) error {
	if ctx == nil || authority.AcquireContextEffect == nil || authority.ValidateCurrent == nil ||
		authority.CaseLineageContains == nil || authority.AcquireTerminalCAS == nil || publish == nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(securityContext) {
		return errors.New("general publication authority is unavailable")
	}
	effectCtx, release, err := authority.AcquireContextEffect(ctx, securityContext)
	if err != nil || effectCtx == nil || release == nil {
		if err == nil {
			err = errors.New("general publication effect lease is invalid")
		}
		return err
	}
	defer release()
	if err := authority.ValidateCurrent(effectCtx, securityContext); err != nil {
		return errors.Join(errors.New("general publication authority changed"), err)
	}
	contains, err := authority.CaseLineageContains(securityContext.ThreadID)
	if err != nil {
		return errors.Join(errors.New("case lineage authority is unavailable"), err)
	}
	if contains {
		return errors.New("signed case lineage cannot use general publication")
	}
	if err := effectCtx.Err(); err != nil {
		return err
	}
	releaseTerminal, err := authority.AcquireTerminalCAS(effectCtx, securityContext)
	if err != nil || releaseTerminal == nil {
		if err == nil {
			err = errors.New("general terminal permit is invalid")
		}
		return errors.Join(errors.New("general terminal arbitration failed"), err)
	}
	defer releaseTerminal()
	if err := effectCtx.Err(); err != nil {
		return err
	}
	return publish(effectCtx)
}

func CommitCurrentGeneralCompletion(
	ctx context.Context,
	authority CurrentGeneralPublicationAuthority,
	input CommitCompletionInput,
) (CommitCompletionResult, error) {
	result := CommitCompletionResult{}
	err := WithCurrentGeneralPublicationAuthority(ctx, authority, input.SecurityContext, func(context.Context) error {
		var commitErr error
		result, commitErr = CommitCompletedTurn(input)
		return commitErr
	})
	return result, err
}

func FinalizeCurrentGeneralAfterLoop(
	ctx context.Context,
	authority CurrentGeneralPublicationAuthority,
	input FinalizeAfterLoopInput,
) error {
	return WithCurrentGeneralPublicationAuthority(ctx, authority, input.SecurityContext, func(context.Context) error {
		return FinalizeAfterLoop(input)
	})
}
