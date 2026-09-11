package loop

import (
	"context"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ProviderEffectAcquirer func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	bool,
) (context.Context, func(), error)

func AcquireProviderAttemptEffect(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	caseDataEffect bool,
	acquire ProviderEffectAcquirer,
	validate func(context.Context) error,
) (context.Context, func(), error) {
	if acquire == nil || validate == nil {
		return ctx, nil, providerAuthorityFailure(
			"provider authority is unavailable",
			"turn_security_authority_unavailable",
		)
	}
	effectCtx, release, err := acquire(ctx, securityContext, caseDataEffect)
	if err != nil {
		return ctx, nil, err
	}
	if effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return ctx, nil, providerAuthorityFailure(
			"provider authority is unavailable",
			"turn_security_authority_unavailable",
		)
	}
	if err := validate(effectCtx); err != nil {
		release()
		return ctx, nil, err
	}
	return effectCtx, release, nil
}

func AcquirePendingToolEffect(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	acquire ProviderEffectAcquirer,
) (context.Context, func(), error) {
	if acquire == nil {
		return ctx, nil, providerAuthorityFailure(
			"tool effect authority is unavailable",
			"turn_security_authority_unavailable",
		)
	}
	if err := executiongrantapp.ValidateExecutionGrantForCall(
		pending.SecurityContext,
		pending.ExecutionGrant,
		pending.Call,
	); err != nil {
		return ctx, nil, err
	}
	return acquire(
		ctx,
		pending.SecurityContext,
		executiongrantapp.CallUsesCaseDataAuthority(pending.Call),
	)
}
