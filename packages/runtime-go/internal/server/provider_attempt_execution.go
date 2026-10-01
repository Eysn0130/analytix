package server

import (
	"context"
	"errors"
	"strings"

	attachmentpipelineapp "analytix.local/runtime-go/internal/app/attachmentpipeline"
	apploop "analytix.local/runtime-go/internal/app/loop"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type runtimeProviderAttemptExecution struct {
	ResolveIntent func(context.Context) (apploop.RuntimeProviderIntent, error)
	CurrentIntent func(context.Context) (apploop.RuntimeProviderIntent, error)
	Prepare       func(context.Context, int, domainmodel.Request, string, string, uint64) (domainmodel.Request, apploop.ProviderAttemptSettlement, error)
}

// Shared by normal turns and auxiliary producers: intent is key-free, credentials
// materialize only under the exact effect lease, and settlement rechecks authority.
func (h *runtimeServerHandler) runtimeProviderAttempts(input runtimeAgentLoopInput) runtimeProviderAttemptExecution {
	executionInput := runtimeProviderExecutionInputV1(input)
	_, requireIntentMatch := h.providerExecution.(ProviderExecutionIntentResolver)
	resolveProviderIntent := func(intentCtx context.Context) (apploop.RuntimeProviderIntent, error) {
		if !requireIntentMatch {
			config := input.ProviderConfig
			config.APIKey = ""
			return apploop.RuntimeProviderIntent{
				ProviderConfig: config, ProviderID: input.ProviderID, Model: input.Model, Effort: input.Effort,
			}, nil
		}
		resolved, err := h.resolveRuntimeTurnIntent(intentCtx, executionInput)
		if err != nil {
			return apploop.RuntimeProviderIntent{}, err
		}
		return apploop.RuntimeProviderIntent{
			ProviderConfig: resolved.Config, ProviderID: resolved.ProviderID, Model: resolved.Model, Effort: resolved.Effort,
		}, nil
	}
	var currentProviderIntentResolver func(context.Context) (apploop.RuntimeProviderIntent, error)
	if requireIntentMatch {
		currentProviderIntentResolver = resolveProviderIntent
	}
	prepareProviderAttempt := func(
		attemptCtx context.Context,
		attempt int,
		request domainmodel.Request,
		promptRoute string,
		toolManifestHash string,
		providerCallSequence uint64,
	) (domainmodel.Request, apploop.ProviderAttemptSettlement, error) {
		if err := h.requireRuntimeTurnExecutionCurrentness(); err != nil {
			return domainmodel.Request{}, nil, err
		}
		resolved, err := h.resolveRuntimeTurnExecution(attemptCtx, executionInput)
		if err != nil {
			return domainmodel.Request{}, nil, err
		}
		if strings.TrimSpace(resolved.Config.APIKey) == "" ||
			(requireIntentMatch && !runtimeProviderExecutionMatchesIntentV1(resolved.Config, request)) {
			resolved.Config.APIKey = ""
			return domainmodel.Request{}, nil, errors.New("provider execution authority changed before effect")
		}
		request = bindRuntimeProviderExecutionV1(request, resolved)
		request.PrivateProviderProxyAuthority = requireIntentMatch
		authority := resolved.Authority
		beforeCurrentness := request.PrivateProviderCurrentnessBeforeSend
		request.PrivateProviderCurrentnessBeforeSend = func(physicalAttempt int) error {
			if beforeCurrentness != nil {
				if err := beforeCurrentness(physicalAttempt); err != nil {
					return err
				}
			}
			return h.validateRuntimeTurnExecutionCurrent(attemptCtx, authority)
		}
		resolved.Config.APIKey = ""
		var attachmentSettle apploop.ProviderAttemptSettlement
		if !input.Attachments.Empty() {
			attachmentPipeline := h.runtimeAttachmentPipeline()
			prepared, prepareErr := attachmentPipeline.PrepareProviderAttempt(attemptCtx, attachmentpipelineapp.ProviderAttemptInput{
				SecurityContext: input.SecurityContext, Plan: input.Attachments, Request: request,
				Primary: resolved.Config, PrimaryProvider: resolved.ProviderID, PrimaryModel: resolved.Model,
				PromptRoute: promptRoute, ToolManifestHash: toolManifestHash,
				Sequence: providerCallSequence, Attempt: attempt,
			})
			if prepareErr != nil {
				return domainmodel.Request{}, nil, prepareErr
			}
			token := prepared.Token
			request = prepared.Request
			attachmentSettle = func(settleCtx context.Context, status, reason string) error {
				return attachmentPipeline.SettleProviderAttempt(settleCtx, token, status, reason)
			}
		}
		settle := func(settleCtx context.Context, status, reason string) error {
			currentErr := h.validateRuntimeTurnExecutionCurrent(settleCtx, authority)
			if currentErr != nil {
				if attachmentSettle != nil {
					return errors.Join(currentErr, attachmentSettle(settleCtx, "failed", "provider_authority_changed"))
				}
				return currentErr
			}
			if attachmentSettle != nil {
				return attachmentSettle(settleCtx, status, reason)
			}
			return nil
		}
		return request, settle, nil
	}
	return runtimeProviderAttemptExecution{ResolveIntent: resolveProviderIntent, CurrentIntent: currentProviderIntentResolver, Prepare: prepareProviderAttempt}
}
