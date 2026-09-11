package steering

import (
	"context"
	"errors"
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

var ErrAuthorityUnavailable = errors.New("steering promotion authority is unavailable")
var ErrContextInvalid = errors.New("steering promotion context is invalid")

type CurrentAuthorityError struct {
	Cause error
}

func (failure CurrentAuthorityError) Error() string { return "steering promotion authority changed" }
func (failure CurrentAuthorityError) Unwrap() error { return failure.Cause }

func ProviderFailureProjection(err error) (message, code string, projected bool) {
	if errors.Is(err, ErrAuthorityUnavailable) {
		return err.Error(), "turn_security_authority_unavailable", true
	}
	if errors.Is(err, ErrContextInvalid) {
		return err.Error(), "turn_security_context_invalid", true
	}
	var currentErr CurrentAuthorityError
	if errors.As(err, &currentErr) {
		return currentErr.Error(), turnsecurityapp.CurrentFailureCode(currentErr.Cause), true
	}
	return "", "", false
}

type Store interface {
	appmodel.SteeringPrefixPromotionStore
	PendingSteeringEntriesForContext(threadID, turnID, expectedContextDigest string) ([]map[string]any, error)
}

type Dependencies struct {
	Store                Store
	Jobs                 subagentapp.TaskJobSteerStore
	AcquireContextEffect func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error)
	ValidateCurrent      func(context.Context, domainsecurity.TurnSecurityContext) error
	VerifyAuthority      func(context.Context, map[string]any, string) error
	RecordEvent          func(map[string]any, string)
}

type EffectAwareDependencies struct {
	Store                Store
	Jobs                 subagentapp.TaskJobSteerStore
	AcquireContextEffect func(context.Context, domainsecurity.TurnSecurityContext, bool) (context.Context, func(), error)
	Authority            turnsecurityapp.WorkspaceSecurityAuthority
	VerifyAuthority      func(context.Context, map[string]any, string) error
	RecordEvent          func(map[string]any, string)
}

type ProviderSteeringBatch struct {
	Messages      []domainmodel.Message
	Prompt        string
	LogicalEffect domainsecurity.LogicalEffect
	OrdinaryWork  bool
}

func PromoteCurrentForProviderEffect(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	_ bool,
	deps EffectAwareDependencies,
) ([]domainmodel.Message, error) {
	batch, err := PromoteCurrentForProviderEffectBatch(ctx, securityContext, deps)
	return batch.Messages, err
}

// PromoteCurrentForProviderEffectBatch always performs steering queue work
// under the ordinary-effect lease. The returned logical effect describes the
// authority required by the selected steering batch; it is not itself that
// authority and must be enforced by the subsequent provider/tool attempt.
func PromoteCurrentForProviderEffectBatch(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	deps EffectAwareDependencies,
) (ProviderSteeringBatch, error) {
	return PromoteCurrentForProviderBatch(ctx, securityContext, Dependencies{
		Store: deps.Store,
		Jobs:  deps.Jobs,
		AcquireContextEffect: func(effectCtx context.Context, current domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			if deps.AcquireContextEffect == nil {
				return effectCtx, nil, ErrAuthorityUnavailable
			}
			return deps.AcquireContextEffect(effectCtx, current, false)
		},
		ValidateCurrent: func(effectCtx context.Context, current domainsecurity.TurnSecurityContext) error {
			return turnsecurityapp.ValidateCurrentForEffect(turnsecurityapp.CurrentValidationInput{
				OperationContext: effectCtx, Identity: deps.Authority.Identity,
				Observer: deps.Authority.Observer, RiskAuthority: deps.Authority.RiskAuthority,
				SnapshotAuthority: deps.Authority.SnapshotAuthority, SnapshotAuthorityV2: deps.Authority.SnapshotAuthorityV2,
				Context: current, Workspace: current.WorkspaceRealPath,
			}, false)
		},
		VerifyAuthority: deps.VerifyAuthority,
		RecordEvent:     deps.RecordEvent,
	})
}

func PromoteCurrentForProvider(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	deps Dependencies,
) ([]domainmodel.Message, error) {
	batch, err := PromoteCurrentForProviderBatch(ctx, securityContext, deps)
	return batch.Messages, err
}

func PromoteCurrentForProviderBatch(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	deps Dependencies,
) (ProviderSteeringBatch, error) {
	if ctx == nil || deps.Store == nil || deps.AcquireContextEffect == nil || deps.ValidateCurrent == nil {
		return ProviderSteeringBatch{}, ErrAuthorityUnavailable
	}
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		return ProviderSteeringBatch{}, ErrContextInvalid
	}
	effectCtx, release, err := deps.AcquireContextEffect(ctx, securityContext)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return ProviderSteeringBatch{}, ErrAuthorityUnavailable
	}
	defer release()
	if err := deps.ValidateCurrent(effectCtx, securityContext); err != nil {
		return ProviderSteeringBatch{}, CurrentAuthorityError{Cause: err}
	}
	pending, err := deps.Store.PendingSteeringEntriesForContext(
		securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
	)
	if err != nil {
		return ProviderSteeringBatch{}, err
	}
	if len(pending) == 0 {
		return ProviderSteeringBatch{}, nil
	}
	selected, expected, batch, err := providerSteeringPrefixV1(pending, securityContext)
	if err != nil {
		return ProviderSteeringBatch{}, err
	}
	if err := subagentapp.ValidateRuntimeTaskJobSteersForPromotion(
		deps.Jobs, selected, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
	); err != nil {
		return ProviderSteeringBatch{}, err
	}
	messages, entries, items, err := appmodel.PromoteSteeringPrefixForProviderWithEntries(
		deps.Store, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest, expected,
	)
	if err != nil {
		return ProviderSteeringBatch{}, err
	}
	if len(entries) != len(selected) || len(items) != len(selected) || len(messages) != len(selected) {
		return ProviderSteeringBatch{}, errors.New("steering promotion did not preserve the selected prefix")
	}
	commits, err := taskJobPromotionCommitsV1(effectCtx, securityContext, entries, items, deps.VerifyAuthority)
	if err != nil {
		return ProviderSteeringBatch{}, err
	}
	if err := subagentapp.RecordRuntimeTaskJobSteersAdmitted(
		subagentapp.TaskJobSteerRuntimeDeps{Jobs: deps.Jobs, RecordEvent: deps.RecordEvent}, commits,
	); err != nil {
		return ProviderSteeringBatch{}, err
	}
	batch.Messages = messages
	return batch, nil
}

func providerSteeringPrefixV1(
	pending []map[string]any,
	securityContext domainsecurity.TurnSecurityContext,
) ([]map[string]any, []domainsteering.PendingEntryExpectationV1, ProviderSteeringBatch, error) {
	selected := make([]map[string]any, 0, len(pending))
	expected := make([]domainsteering.PendingEntryExpectationV1, 0, len(pending))
	prompts := make([]string, 0, len(pending))
	batch := ProviderSteeringBatch{}
	for _, entry := range pending {
		binding, err := steeringEntryLogicalEffectV1(entry, securityContext)
		if err != nil {
			return nil, nil, ProviderSteeringBatch{}, err
		}
		if len(selected) > 0 && binding.LogicalEffect != batch.LogicalEffect {
			break
		}
		entryExpectation, err := domainsteering.NewPendingEntryExpectationV1(entry)
		if err != nil {
			return nil, nil, ProviderSteeringBatch{}, err
		}
		prompt, _ := entry["text"].(string)
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			return nil, nil, ProviderSteeringBatch{}, ErrContextInvalid
		}
		if len(selected) == 0 {
			batch.LogicalEffect = binding.LogicalEffect
		}
		batch.OrdinaryWork = batch.OrdinaryWork || binding.OrdinaryWork
		selected = append(selected, entry)
		expected = append(expected, entryExpectation)
		prompts = append(prompts, prompt)
	}
	batch.Prompt = strings.Join(prompts, "\n\n")
	return selected, expected, batch, nil
}

func steeringEntryLogicalEffectV1(
	entry map[string]any,
	securityContext domainsecurity.TurnSecurityContext,
) (domainsteering.EntryLogicalEffectBinding, error) {
	binding, present, err := domainsteering.LogicalEffectBindingFromEntryV1(entry)
	if err != nil || present {
		return binding, err
	}
	if domainsecurity.TurnSecurityContextIsGeneral(securityContext) {
		return domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectOrdinary,
			OrdinaryWork:  true,
		}, nil
	}
	if domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext) ||
		domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		return domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectCaseData,
			OrdinaryWork:  false,
		}, nil
	}
	return domainsteering.EntryLogicalEffectBinding{}, ErrContextInvalid
}

func taskJobPromotionCommitsV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	entries, items []map[string]any,
	verify func(context.Context, map[string]any, string) error,
) ([]domainsteering.PromotionCommitV1, error) {
	itemByID := make(map[string]map[string]any, len(items))
	for _, item := range items {
		itemID, _ := item["id"].(string)
		if itemID != "" {
			itemByID[itemID] = item
		}
	}
	commits := make([]domainsteering.PromotionCommitV1, 0, len(entries))
	for _, entry := range entries {
		jobID, _ := entry["jobId"].(string)
		if jobID == "" {
			continue
		}
		if verify == nil {
			return nil, ErrAuthorityUnavailable
		}
		itemID, _ := entry["promotedItemId"].(string)
		commit, err := domainsteering.NewPromotionCommitV1(
			securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest,
			entry, itemByID[itemID],
			func(candidate map[string]any, digest string) error { return verify(ctx, candidate, digest) },
		)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)
	}
	return commits, nil
}
