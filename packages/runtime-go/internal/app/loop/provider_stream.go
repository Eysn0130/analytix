package loop

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"io"
	"strings"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

const DefaultProviderStreamMaxAttempts = 2

var ErrProviderOutputTokenBudgetExceeded = errors.New("provider output token budget exceeded")

type ProviderStreamCallbacks struct {
	AcquireProviderAttempt func(context.Context, int) (context.Context, func(), error)
	BeforeProviderAttempt  func(attempt int) error
	// BeforeProviderInvocation runs after host preparation and privacy
	// projection, immediately before Provider.Stream receives the request.
	BeforeProviderInvocation func(attempt int) error
	// AfterProviderAttempt runs exactly once after Provider.Stream returns. It
	// lets the host validate the transport's durable dispatch disposition before
	// retry or publication decisions consume the attempt result.
	AfterProviderAttempt func(attempt int, err error) error
	// PrepareProviderAttempt runs only after the exact effect lease and current
	// continuation authority are valid. It may materialize private request data
	// for this one physical attempt and returns a mandatory durable settlement
	// callback. The callback must finish before the request is discarded and
	// the effect lease is released.
	PrepareProviderAttempt func(context.Context, int, domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error)
	OnTextDelta            func(string) error
	OnTextChunk            func(domainmodel.Chunk) error
	OnToolCallStart        func(domainmodel.ToolCall) error
	OnProviderRetrying     func(attempt int, maxAttempts int, cause error) error
}

type ProviderAttemptSettlement func(context.Context, string, string) error

type ProviderStreamInput struct {
	Provider            ports.ProviderClient
	Request             domainmodel.Request
	SecurityContext     domainsecurity.TurnSecurityContext
	HostEntitySelection HostCaseEntitySelectionV1
	// OrdinaryEffect is host-owned. Its zero value preserves the strict
	// case-data projection used by existing case callers.
	OrdinaryEffect        bool
	MaxAttempts           int
	Callbacks             ProviderStreamCallbacks
	ContinuationAuthority ProviderContinuationAuthorityInput
	toolCallIDRandom      io.Reader
}

type ProviderContinuationAuthorityInput struct {
	Service          *pendingworkapp.Service
	SecurityContext  domainsecurity.TurnSecurityContext
	References       []domainsecurity.SettledToolReference
	ProviderConfig   domainmodel.TurnConfig
	PromptRoute      string
	ToolManifestHash string
	Sequence         uint64
}

type ProviderStreamOutput struct {
	Result             domainmodel.Result
	Text               string
	Reasoning          string
	ReasoningSignature string
	StreamedDeltas     bool
	PartialTextStarted bool
	PartialToolStarted bool
}

type ProviderStreamCallbackError struct {
	Err error
}

type providerPipelineContractError struct {
	reason string
}

type providerDispatchStateErrorV1 interface {
	ProviderDispatchStateV1() domaincache.ProviderDispatchStateV1
}

func providerDispatchStateFromErrorV1(err error) (domaincache.ProviderDispatchStateV1, bool) {
	var disposition providerDispatchStateErrorV1
	if err == nil || !errors.As(err, &disposition) {
		return "", false
	}
	switch state := disposition.ProviderDispatchStateV1(); state {
	case domaincache.ProviderDispatchStateNotSent, domaincache.ProviderDispatchStateSent,
		domaincache.ProviderDispatchStateIndeterminate:
		return state, true
	default:
		return "", false
	}
}

func (err providerPipelineContractError) Error() string {
	if strings.TrimSpace(err.reason) == "" {
		return "provider durable pipeline contract is invalid"
	}
	return err.reason
}

func (providerPipelineContractError) ProviderRetryForbidden() bool { return true }

func (err ProviderStreamCallbackError) Error() string {
	if err.Err == nil {
		return "provider stream callback failed"
	}
	return err.Err.Error()
}

func (err ProviderStreamCallbackError) Unwrap() error {
	return err.Err
}

// providerAttemptAcquireError marks the one callback edge that is guaranteed
// to fail before the provider transport receives this physical attempt. The
// runner may use that fact to continue an independent ordinary lane for a
// closed allowlist of protected-capability currentness failures; no later
// callback or settlement failure receives this marker.
type providerAttemptAcquireError struct {
	Err error
}

func (err providerAttemptAcquireError) Error() string {
	if err.Err == nil {
		return "provider attempt authority acquisition failed"
	}
	return err.Err.Error()
}

func (err providerAttemptAcquireError) Unwrap() error {
	return err.Err
}

// providerAttemptPrepareError marks a private-materialization failure that is
// guaranteed to happen before the provider transport receives the physical
// attempt. The runner may inspect only a closed set of typed currentness
// failures and may never downgrade an unknown preparation error.
type providerAttemptPrepareError struct {
	Err error
}

func (err providerAttemptPrepareError) Error() string {
	if err.Err == nil {
		return "provider attempt private materialization failed"
	}
	return err.Err.Error()
}

func (err providerAttemptPrepareError) Unwrap() error {
	return err.Err
}

func StreamProviderWithRetry(ctx context.Context, input ProviderStreamInput) (ProviderStreamOutput, error) {
	if err := domainmodel.ValidateReasoningEffortV1(input.Request.ReasoningEffort); err != nil {
		return ProviderStreamOutput{}, err
	}
	if input.Request.MaxOutputTokens < 0 {
		return ProviderStreamOutput{}, ErrProviderOutputTokenBudgetExceeded
	}
	authority := input.ContinuationAuthority
	if len(authority.References) > 0 {
		if err := domainmodel.ValidateReasoningEffortV1(authority.ProviderConfig.ReasoningEffort); err != nil {
			return ProviderStreamOutput{}, err
		}
	}
	if len(authority.References) == 0 {
		return streamProviderWithRetry(ctx, input)
	}
	lease, err := authority.Service.BeginProviderContinuation(ctx, pendingworkapp.ProviderContinuationRequest{
		SecurityContext: authority.SecurityContext, References: authority.References, Request: input.Request,
		ProviderConfig: authority.ProviderConfig, PromptRoute: authority.PromptRoute, ToolManifestHash: authority.ToolManifestHash,
		Sequence: authority.Sequence, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		return ProviderStreamOutput{}, err
	}
	expectedRequest := pendingworkapp.ProviderContinuationRequest{
		SecurityContext: authority.SecurityContext, References: authority.References, Request: input.Request,
		ProviderConfig: authority.ProviderConfig, PromptRoute: authority.PromptRoute, ToolManifestHash: authority.ToolManifestHash,
		Sequence: authority.Sequence,
	}
	verifyRequest := func() error {
		return authority.Service.VerifyProviderContinuationRequest(ctx, lease, expectedRequest, time.Now().UTC())
	}
	beforeAttempt := input.Callbacks.BeforeProviderAttempt
	input.Callbacks.BeforeProviderAttempt = func(attempt int) error {
		if err := verifyRequest(); err != nil {
			return err
		}
		if beforeAttempt != nil {
			return beforeAttempt(attempt)
		}
		return nil
	}
	beforeSend := input.Request.BeforeSend
	input.Request.BeforeSend = func(attempt int) error {
		if err := verifyRequest(); err != nil {
			return ProviderStreamCallbackError{Err: err}
		}
		if beforeSend != nil {
			return beforeSend(attempt)
		}
		return nil
	}
	output, streamErr := streamProviderWithRetry(ctx, input)
	status, reason := domainpendingwork.StatusCompleted, "provider_completed"
	if streamErr != nil {
		status, reason = domainpendingwork.StatusFailed, "provider_failed"
	}
	if ctx.Err() != nil {
		status, reason = domainpendingwork.StatusCancelled, "provider_cancelled"
	}
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	_, closeErr := authority.Service.CloseProviderContinuationLease(closeCtx, lease, status, reason, time.Now().UTC())
	cancel()
	if closeErr != nil {
		return sealFailedProviderStreamOutput(output), errors.Join(streamErr, closeErr)
	}
	return output, streamErr
}

type providerOutputTokenBudgetV1 struct {
	maxTokens int
	observed  bool
	fragments strings.Builder
}

func newProviderOutputTokenBudgetV1(maxTokens int) *providerOutputTokenBudgetV1 {
	return &providerOutputTokenBudgetV1{maxTokens: maxTokens}
}

func (budget *providerOutputTokenBudgetV1) Observe(chunk domainmodel.Chunk) error {
	if budget == nil || budget.maxTokens <= 0 {
		return nil
	}
	budget.observed = true
	switch chunk.Kind {
	case domainmodel.ChunkReasoning, domainmodel.ChunkText:
		budget.fragments.WriteString(chunk.Text)
	case domainmodel.ChunkToolCallStart, domainmodel.ChunkToolCall:
		budget.fragments.WriteString(chunk.ToolCall.Name)
		budget.fragments.Write(chunk.ToolCall.Arguments)
	case domainmodel.ChunkUsage:
		if providerReportedOutputTokensV1(chunk.Usage) > budget.maxTokens {
			return ErrProviderOutputTokenBudgetExceeded
		}
	}
	if appmodel.EstimateTextTokensV1(budget.fragments.String()) > budget.maxTokens {
		return ErrProviderOutputTokenBudgetExceeded
	}
	return nil
}

func (budget *providerOutputTokenBudgetV1) ValidateResult(result domainmodel.Result) error {
	if budget == nil || budget.maxTokens <= 0 {
		return nil
	}
	if providerReportedOutputTokensV1(result.Usage) > budget.maxTokens {
		return ErrProviderOutputTokenBudgetExceeded
	}
	if budget.observed {
		return nil
	}
	for _, chunk := range result.Chunks {
		if err := budget.Observe(chunk); err != nil {
			return err
		}
	}
	return nil
}

func providerReportedOutputTokensV1(usage domainmodel.Usage) int {
	reported := usage.CompletionTokens
	if usage.ReasoningTokens > reported {
		reported = usage.ReasoningTokens
	}
	if output := usage.TotalTokens - usage.PromptTokens; output > reported {
		reported = output
	}
	return reported
}

func streamProviderWithRetry(ctx context.Context, input ProviderStreamInput) (ProviderStreamOutput, error) {
	if input.Provider == nil {
		return ProviderStreamOutput{}, errors.New("provider client is not configured")
	}
	providerRequestToolManifestHash := toolcatalogapp.ToolSchemaHash(input.Request.Tools)
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultProviderStreamMaxAttempts
	}
	var output ProviderStreamOutput
	var err error
	var cacheObservations []domaincache.ProviderCallObservationV1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return output, err
		}
		attemptCtx := ctx
		releaseAttempt := func() {}
		if input.Callbacks.AcquireProviderAttempt != nil {
			leasedCtx, release, leaseErr := input.Callbacks.AcquireProviderAttempt(ctx, attempt)
			if leaseErr != nil || leasedCtx == nil || release == nil {
				if leaseErr == nil {
					leaseErr = errors.New("provider attempt effect authority is unavailable")
				}
				return output, ProviderStreamCallbackError{Err: providerAttemptAcquireError{Err: leaseErr}}
			}
			attemptCtx = leasedCtx
			releaseAttempt = release
		}
		attemptOutput := ProviderStreamOutput{}
		publicTextDecoder := domainreasoningmarkup.NewDecoder()
		var reasoningMarkupErr error
		request := input.Request
		request.Messages = appmodel.SanitizeToolPairing(request.Messages)
		if request.PrivateProviderTelemetry != nil {
			binding := *request.PrivateProviderTelemetry
			binding.OrdinaryEffect = input.OrdinaryEffect
			binding.OuterAttempt = uint32(attempt)
			request.PrivateProviderTelemetry = &binding
		}
		if input.Callbacks.BeforeProviderAttempt != nil {
			if callbackErr := input.Callbacks.BeforeProviderAttempt(attempt); callbackErr != nil {
				releaseAttempt()
				return output, ProviderStreamCallbackError{Err: callbackErr}
			}
		}
		settleAttempt := ProviderAttemptSettlement(nil)
		if input.Callbacks.PrepareProviderAttempt != nil {
			prepared, settle, prepareErr := input.Callbacks.PrepareProviderAttempt(attemptCtx, attempt, request)
			if prepareErr != nil || settle == nil {
				request = domainmodel.Request{}
				releaseAttempt()
				if prepareErr == nil {
					prepareErr = errors.New("provider attempt private materialization settlement is unavailable")
				}
				return output, ProviderStreamCallbackError{Err: providerAttemptPrepareError{Err: prepareErr}}
			}
			request = prepared
			settleAttempt = settle
			reasoningErr := domainmodel.ValidateReasoningEffortV1(request.ReasoningEffort)
			if reasoningErr != nil || request.ReasoningEffort != input.Request.ReasoningEffort ||
				request.MaxOutputTokens != input.Request.MaxOutputTokens {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				settleErr := settleAttempt(settleCtx, "failed", "provider_request_authority_invalid")
				cancelSettle()
				settleAttempt = nil
				releaseAttempt()
				if reasoningErr == nil {
					if request.ReasoningEffort != input.Request.ReasoningEffort {
						reasoningErr = domainmodel.ErrInvalidReasoningEffort
					} else {
						reasoningErr = ErrProviderOutputTokenBudgetExceeded
					}
				}
				return output, errors.Join(reasoningErr, settleErr)
			}
			if request.PrivateProviderTelemetry != nil {
				binding := *request.PrivateProviderTelemetry
				binding.OrdinaryEffect = input.OrdinaryEffect
				binding.OuterAttempt = uint32(attempt)
				request.PrivateProviderTelemetry = &binding
			}
		}
		request.PrivateProviderReferenceBinding = nil
		projectedRequest, projectionErr := privacyprojectionapp.ProjectProviderRequestForEffect(
			input.SecurityContext,
			request,
			input.OrdinaryEffect,
		)
		if projectionErr != nil {
			var settleErr error
			if settleAttempt != nil {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				settleErr = settleAttempt(settleCtx, "failed", "privacy_projection_failed")
				cancelSettle()
				settleAttempt = nil
			}
			request = domainmodel.Request{}
			releaseAttempt()
			return output, ProviderStreamCallbackError{Err: errors.Join(projectionErr, settleErr)}
		}
		request = projectedRequest
		if toolcatalogapp.ToolSchemaHash(request.Tools) != providerRequestToolManifestHash {
			var settleErr error
			if settleAttempt != nil {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				settleErr = settleAttempt(settleCtx, "failed", "provider_request_tool_manifest_mismatch")
				cancelSettle()
				settleAttempt = nil
			}
			request = domainmodel.Request{}
			releaseAttempt()
			return output, ProviderStreamCallbackError{Err: errors.Join(
				providerPipelineContractError{reason: "provider request tool manifest changed after attempt preparation"},
				settleErr,
			)}
		}
		thresholds := appmodel.ResolveContextThresholdsV1(input.ContinuationAuthority.ProviderConfig.ContextWindowTokens)
		projectedRequestTokens := appmodel.EstimateProviderRequestTokensV1(request)
		if projectedRequestTokens >= thresholds.HardThresholdTokens {
			var diagnosticErr error
			if request.OnPipelineStage != nil {
				diagnosticErr = request.OnPipelineStage(domainmodel.PipelineStage{
					Stage: "provider_admission_rejected",
					At:    time.Now().UTC(),
					Details: map[string]any{
						"reasonCode":             "context_window_hard_limit",
						"projectedRequestTokens": float64(projectedRequestTokens),
						"hardThresholdTokens":    float64(thresholds.HardThresholdTokens),
						"providerAttemptCount":   float64(0),
					},
				})
			}
			var settleErr error
			if settleAttempt != nil {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				settleErr = settleAttempt(settleCtx, "failed", "context_window_hard_limit")
				cancelSettle()
				settleAttempt = nil
			}
			request = domainmodel.Request{}
			releaseAttempt()
			return output, errors.Join(
				domainfailure.NewError("context_window_hard_limit", nil), diagnosticErr, settleErr,
			)
		}
		toolCallIDs := newProviderToolCallIDIssuerV1(input.toolCallIDRandom)
		outputBudget := newProviderOutputTokenBudgetV1(request.MaxOutputTokens)
		providerStartedAt := time.Now()
		firstTokenLatencyMs := int64(-1)
		markFirstToken := func(observedAt time.Time) {
			if firstTokenLatencyMs < 0 {
				firstTokenLatencyMs = providerFirstTokenLatencyMillis(providerStartedAt, observedAt, time.Now())
			}
		}
		request.OnChunk = func(chunk domainmodel.Chunk) error {
			if err := attemptCtx.Err(); err != nil {
				return ProviderStreamCallbackError{Err: err}
			}
			normalizedChunk, identityErr := toolCallIDs.normalizeCallbackChunk(chunk)
			if identityErr != nil {
				return ProviderStreamCallbackError{Err: identityErr}
			}
			chunk = normalizedChunk
			if err := outputBudget.Observe(chunk); err != nil {
				return ProviderStreamCallbackError{Err: err}
			}
			if chunk.Trace.LoopChunkCallbackAt.IsZero() {
				chunk.Trace.LoopChunkCallbackAt = time.Now().UTC()
			}
			switch chunk.Kind {
			case domainmodel.ChunkReasoning:
				if chunk.Text != "" {
					markFirstToken(chunk.Trace.ProviderRawSSEChunkAt)
					attemptOutput.StreamedDeltas = true
					attemptOutput.Reasoning += chunk.Text
				}
				if chunk.Signature != "" {
					attemptOutput.ReasoningSignature = chunk.Signature
				}
			case domainmodel.ChunkText:
				if chunk.Text != "" {
					markFirstToken(chunk.Trace.ProviderRawSSEChunkAt)
					attemptOutput.StreamedDeltas = true
					attemptOutput.PartialTextStarted = true
					if reasoningMarkupErr == nil {
						if err := publicTextDecoder.Push(chunk.Text); err != nil {
							reasoningMarkupErr = domainreasoningmarkup.NewProtocolError(err)
							return reasoningMarkupErr
						}
					}
				}
			case domainmodel.ChunkToolCallStart:
				markFirstToken(chunk.Trace.ProviderRawSSEChunkAt)
				attemptOutput.StreamedDeltas = true
				attemptOutput.PartialToolStarted = true
				if chunk.ToolCall.ID != "" && chunk.ToolCall.Name != "" {
					if input.Callbacks.OnToolCallStart != nil {
						if err := input.Callbacks.OnToolCallStart(chunk.ToolCall); err != nil {
							return ProviderStreamCallbackError{Err: err}
						}
					}
				}
			case domainmodel.ChunkRetrying:
				if attemptOutput.PartialTextStarted || attemptOutput.PartialToolStarted {
					return providerReconnectAfterOutputError{}
				}
				// A transport reconnect starts a new physical provider attempt.
				// Nothing buffered from the failed attempt has crossed the public
				// callback boundary, so discard both its text candidate and its
				// private reasoning before accepting replacement bytes.
				attemptOutput.Text = ""
				attemptOutput.Reasoning = ""
				attemptOutput.ReasoningSignature = ""
				attemptOutput.StreamedDeltas = false
				publicTextDecoder = domainreasoningmarkup.NewDecoder()
				reasoningMarkupErr = nil
				retryAttempt := chunk.RetryAttempt
				if retryAttempt <= 0 {
					retryAttempt = 1
				}
				retryMax := chunk.RetryMax
				if retryMax <= 0 {
					retryMax = retryAttempt
				}
				message := chunk.Text
				if message == "" {
					message = "provider stream retrying before first output"
				}
				if input.Callbacks.OnProviderRetrying != nil {
					if err := input.Callbacks.OnProviderRetrying(retryAttempt, retryMax, errors.New(message)); err != nil {
						return ProviderStreamCallbackError{Err: err}
					}
				}
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			var settleErr error
			if settleAttempt != nil {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				settleErr = settleAttempt(settleCtx, "cancelled", "provider_attempt_cancelled")
				cancelSettle()
				settleAttempt = nil
			}
			request = domainmodel.Request{}
			releaseAttempt()
			return output, errors.Join(err, settleErr)
		}
		if attemptErr := attemptCtx.Err(); attemptErr != nil {
			var settleErr error
			if settleAttempt != nil {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				settleErr = settleAttempt(settleCtx, "cancelled", "provider_attempt_cancelled")
				cancelSettle()
				settleAttempt = nil
			}
			request = domainmodel.Request{}
			releaseAttempt()
			return output, ProviderStreamCallbackError{Err: errors.Join(attemptErr, settleErr)}
		}
		if input.Callbacks.BeforeProviderInvocation != nil {
			if callbackErr := input.Callbacks.BeforeProviderInvocation(attempt); callbackErr != nil {
				var settleErr error
				if settleAttempt != nil {
					settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
					settleErr = settleAttempt(settleCtx, "failed", "provider_invocation_rejected")
					cancelSettle()
					settleAttempt = nil
				}
				request = domainmodel.Request{}
				releaseAttempt()
				return output, ProviderStreamCallbackError{Err: errors.Join(callbackErr, settleErr)}
			}
		}
		result, streamErr := input.Provider.Stream(attemptCtx, request)
		if input.Callbacks.AfterProviderAttempt != nil {
			if callbackErr := input.Callbacks.AfterProviderAttempt(attempt, streamErr); callbackErr != nil {
				streamErr = errors.Join(streamErr, ProviderStreamCallbackError{Err: callbackErr})
			}
		}
		typedRetryUnsafe := false
		if observation, observed := providerOutputObservationV1(streamErr); observed {
			typedRetryUnsafe = observation.RetryUnsafeV1()
			if observation.IncludesV1(domainmodel.ProviderOutputObservationPublicTextV1) {
				attemptOutput.PartialTextStarted = true
			}
			if observation.IncludesV1(domainmodel.ProviderOutputObservationToolStartedV1) {
				attemptOutput.PartialToolStarted = true
			}
			if typedRetryUnsafe {
				attemptOutput.StreamedDeltas = true
			}
		} else {
			if providerOutputStarted(streamErr) {
				attemptOutput.StreamedDeltas = true
				attemptOutput.PartialTextStarted = true
			}
		}
		result = appmodel.NormalizeProviderResult(request, result)
		if budgetErr := outputBudget.ValidateResult(result); budgetErr != nil {
			streamErr = errors.Join(streamErr, budgetErr)
		}
		normalizedResult, identityErr := toolCallIDs.normalizeResult(result)
		if identityErr != nil {
			result.Chunks = nil
			streamErr = errors.Join(streamErr, identityErr)
		} else {
			result = normalizedResult
		}
		cacheObservations = append(cacheObservations, result.CacheObservations...)
		result.CacheObservations = append([]domaincache.ProviderCallObservationV1(nil), cacheObservations...)
		if attemptErr := attemptCtx.Err(); attemptErr != nil {
			streamErr = errors.Join(streamErr, ProviderStreamCallbackError{Err: attemptErr})
		}
		publicText := ""
		if reasoningMarkupErr != nil {
			streamErr = errors.Join(streamErr, reasoningMarkupErr)
		} else if streamErr == nil {
			outcome, markupErr := publicTextDecoder.Finish()
			if markupErr != nil {
				streamErr = domainreasoningmarkup.NewProtocolError(markupErr)
			} else {
				publicText = outcome.PublicText
			}
		}
		if settleAttempt != nil {
			status, reason := "consumed", "provider_attempt_completed"
			if streamErr != nil {
				status, reason = "failed", "provider_attempt_failed"
			}
			if attemptCtx.Err() != nil || ctx.Err() != nil {
				status, reason = "cancelled", "provider_attempt_cancelled"
			}
			settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
			settleErr := settleAttempt(settleCtx, status, reason)
			cancelSettle()
			settleAttempt = nil
			request = domainmodel.Request{}
			if settleErr != nil {
				releaseAttempt()
				return output, errors.Join(streamErr, ProviderStreamCallbackError{Err: settleErr})
			}
		}
		request = domainmodel.Request{}
		if err := ctx.Err(); err != nil {
			releaseAttempt()
			cancelledResult := result
			cancelledResult.Chunks = nil
			cancelledResult.CacheObservations = append([]domaincache.ProviderCallObservationV1(nil), result.CacheObservations...)
			return ProviderStreamOutput{Result: cancelledResult}, errors.Join(streamErr, err)
		}
		if attemptErr := attemptCtx.Err(); attemptErr != nil {
			releaseAttempt()
			cancelledResult := result
			cancelledResult.Chunks = nil
			cancelledResult.CacheObservations = append([]domaincache.ProviderCallObservationV1(nil), result.CacheObservations...)
			return ProviderStreamOutput{Result: cancelledResult}, errors.Join(streamErr, ProviderStreamCallbackError{Err: attemptErr})
		}
		if streamErr == nil && publicText != "" {
			attemptOutput.Text = publicText
			if input.Callbacks.OnTextChunk != nil {
				if callbackErr := input.Callbacks.OnTextChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: publicText}); callbackErr != nil {
					streamErr = ProviderStreamCallbackError{Err: callbackErr}
				}
			} else if input.Callbacks.OnTextDelta != nil {
				if callbackErr := input.Callbacks.OnTextDelta(publicText); callbackErr != nil {
					streamErr = ProviderStreamCallbackError{Err: callbackErr}
				}
			}
		}
		if err := ctx.Err(); err != nil {
			releaseAttempt()
			cancelledResult := result
			cancelledResult.Chunks = nil
			cancelledResult.CacheObservations = append([]domaincache.ProviderCallObservationV1(nil), result.CacheObservations...)
			return ProviderStreamOutput{Result: cancelledResult}, errors.Join(streamErr, err)
		}
		if attemptErr := attemptCtx.Err(); attemptErr != nil {
			releaseAttempt()
			cancelledResult := result
			cancelledResult.Chunks = nil
			cancelledResult.CacheObservations = append([]domaincache.ProviderCallObservationV1(nil), result.CacheObservations...)
			return ProviderStreamOutput{Result: cancelledResult}, errors.Join(streamErr, ProviderStreamCallbackError{Err: attemptErr})
		}
		releaseAttempt()
		result.DurationMs = time.Since(providerStartedAt).Milliseconds()
		result.HasDuration = true
		if firstTokenLatencyMs >= 0 {
			result.FirstTokenLatencyMs = firstTokenLatencyMs
			result.HasFirstTokenLatency = true
		} else if ProviderResultHasVisibleOutput(result) {
			result.FirstTokenLatencyMs = result.DurationMs
			result.HasFirstTokenLatency = true
		}
		attemptOutput.Result = result
		output = attemptOutput
		err = streamErr
		if err == nil {
			return output, nil
		}
		// Private reasoning is never observable outside the provider attempt and
		// therefore does not make a retry ambiguous.  Only public draft text or
		// a started tool call can have crossed an irreversible callback boundary.
		// A failed reasoning-only attempt is discarded in full before retrying.
		retryUnsafeOutputStarted := typedRetryUnsafe || attemptOutput.PartialTextStarted || attemptOutput.PartialToolStarted
		if retryUnsafeOutputStarted || attempt >= maxAttempts || !ProviderErrorLooksRetryable(err) {
			return sealFailedProviderStreamOutput(output), err
		}
		output = sealFailedProviderStreamOutput(output)
		if input.Callbacks.OnProviderRetrying != nil {
			if retryErr := input.Callbacks.OnProviderRetrying(attempt+1, maxAttempts, err); retryErr != nil {
				return output, ProviderStreamCallbackError{Err: retryErr}
			}
		}
		if delay := ProviderRetryDelay(err); delay > 0 {
			select {
			case <-ctx.Done():
				return sealFailedProviderStreamOutput(output), ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return sealFailedProviderStreamOutput(output), err
}

type providerOutputStartedError interface {
	ProviderOutputStarted() bool
}

type providerOutputObservationError interface {
	ProviderOutputObservationV1() domainmodel.ProviderOutputObservationV1
}

func providerOutputObservationV1(err error) (domainmodel.ProviderOutputObservationV1, bool) {
	var observed providerOutputObservationError
	if errors.As(err, &observed) {
		return observed.ProviderOutputObservationV1(), true
	}
	return domainmodel.ProviderOutputObservationNoneV1, false
}

func providerOutputStarted(err error) bool {
	var observed providerOutputStartedError
	return errors.As(err, &observed) && observed.ProviderOutputStarted()
}

type providerReconnectAfterOutputError struct{}

func (providerReconnectAfterOutputError) Error() string {
	return "provider stream reconnect was rejected after output started"
}

func (providerReconnectAfterOutputError) ProviderRetryForbidden() bool { return true }

const maxProviderToolCallIDCollisionsV1 = 4

var (
	errProviderToolCallIdentityUnavailableV1 = errors.New("provider tool-call host identity is unavailable")
	errProviderToolCallTranscriptMismatchV1  = errors.New("provider tool-call transcript identity is inconsistent")
)

// providerToolCallIDIssuerV1 exists for one physical provider attempt. Raw
// provider identifiers never leave this boundary: every distinct raw value is
// mapped to fresh host entropy, while callback and final chunks from the same
// attempt resolve through the same in-memory map.
type providerToolCallIDIssuerV1 struct {
	random         io.Reader
	issuedByRaw    map[string]string
	issued         map[string]struct{}
	callbackStarts map[string]string
	callbackFinals map[string]string
}

func newProviderToolCallIDIssuerV1(random io.Reader) *providerToolCallIDIssuerV1 {
	if random == nil {
		random = cryptorand.Reader
	}
	return &providerToolCallIDIssuerV1{
		random:         random,
		issuedByRaw:    map[string]string{},
		issued:         map[string]struct{}{},
		callbackStarts: map[string]string{},
		callbackFinals: map[string]string{},
	}
}

func (issuer *providerToolCallIDIssuerV1) issue(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if issued := issuer.issuedByRaw[raw]; issued != "" {
		return issued, nil
	}
	for attempt := 0; attempt < maxProviderToolCallIDCollisionsV1; attempt++ {
		entropy := make([]byte, domainmodel.HostToolCallIDEntropyBytesV1)
		if _, err := io.ReadFull(issuer.random, entropy); err != nil {
			return "", errProviderToolCallIdentityUnavailableV1
		}
		issued, err := domainmodel.NewHostToolCallIDV1(entropy)
		if err != nil {
			return "", errProviderToolCallIdentityUnavailableV1
		}
		if _, collision := issuer.issued[issued]; collision {
			continue
		}
		issuer.issuedByRaw[raw] = issued
		issuer.issued[issued] = struct{}{}
		return issued, nil
	}
	return "", errProviderToolCallIdentityUnavailableV1
}

func (issuer *providerToolCallIDIssuerV1) normalizeCallbackChunk(chunk domainmodel.Chunk) (domainmodel.Chunk, error) {
	switch chunk.Kind {
	case domainmodel.ChunkToolCallStart, domainmodel.ChunkToolCall:
		raw := chunk.ToolCall.ID
		issued, err := issuer.issue(raw)
		if err != nil {
			return domainmodel.Chunk{}, err
		}
		if chunk.Kind == domainmodel.ChunkToolCallStart {
			if err := observeProviderToolCallIdentityV1(issuer.callbackStarts, raw, chunk.ToolCall.Name, false); err != nil {
				return domainmodel.Chunk{}, err
			}
		} else {
			if err := observeProviderToolCallIdentityV1(issuer.callbackFinals, raw, chunk.ToolCall.Name, true); err != nil {
				return domainmodel.Chunk{}, err
			}
			if len(issuer.callbackStarts) > 0 && !providerToolCallIdentityMatchesV1(issuer.callbackStarts, raw, chunk.ToolCall.Name) {
				return domainmodel.Chunk{}, errProviderToolCallTranscriptMismatchV1
			}
		}
		chunk.ToolCall.ID = issued
	}
	return chunk, nil
}

func (issuer *providerToolCallIDIssuerV1) normalizeResult(result domainmodel.Result) (domainmodel.Result, error) {
	if len(result.Chunks) == 0 {
		if len(issuer.callbackStarts) != 0 || len(issuer.callbackFinals) != 0 {
			return domainmodel.Result{}, errProviderToolCallTranscriptMismatchV1
		}
		return result, nil
	}
	normalized := result
	normalized.Chunks = append([]domainmodel.Chunk(nil), result.Chunks...)
	resultStarts := map[string]string{}
	resultFinals := map[string]string{}
	for index, chunk := range result.Chunks {
		switch chunk.Kind {
		case domainmodel.ChunkToolCallStart, domainmodel.ChunkToolCall:
			raw := chunk.ToolCall.ID
			target := resultStarts
			rejectDuplicate := false
			if chunk.Kind == domainmodel.ChunkToolCall {
				target = resultFinals
				rejectDuplicate = true
			}
			if err := observeProviderToolCallIdentityV1(target, raw, chunk.ToolCall.Name, rejectDuplicate); err != nil {
				return domainmodel.Result{}, err
			}
			issued, err := issuer.issue(raw)
			if err != nil {
				return domainmodel.Result{}, err
			}
			normalized.Chunks[index].ToolCall.ID = issued
		}
	}
	if len(issuer.callbackStarts) > 0 && len(resultStarts) > 0 && !sameProviderToolCallIdentitiesV1(issuer.callbackStarts, resultStarts) {
		return domainmodel.Result{}, errProviderToolCallTranscriptMismatchV1
	}
	expectedStarts := issuer.callbackStarts
	if len(expectedStarts) == 0 {
		expectedStarts = resultStarts
	}
	if len(expectedStarts) > 0 && !sameProviderToolCallIdentitiesV1(expectedStarts, resultFinals) {
		return domainmodel.Result{}, errProviderToolCallTranscriptMismatchV1
	}
	if len(issuer.callbackFinals) > 0 && !sameProviderToolCallIdentitiesV1(issuer.callbackFinals, resultFinals) {
		return domainmodel.Result{}, errProviderToolCallTranscriptMismatchV1
	}
	return normalized, nil
}

func observeProviderToolCallIdentityV1(target map[string]string, raw, name string, rejectDuplicate bool) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if existing, found := target[raw]; found {
		if rejectDuplicate || strings.TrimSpace(existing) != strings.TrimSpace(name) {
			return errProviderToolCallTranscriptMismatchV1
		}
		return nil
	}
	target[raw] = name
	return nil
}

func providerToolCallIdentityMatchesV1(expected map[string]string, raw, name string) bool {
	expectedName, found := expected[raw]
	return found && strings.TrimSpace(expectedName) == strings.TrimSpace(name)
}

func sameProviderToolCallIdentitiesV1(expected, actual map[string]string) bool {
	if len(expected) != len(actual) {
		return false
	}
	for raw, name := range actual {
		if !providerToolCallIdentityMatchesV1(expected, raw, name) {
			return false
		}
	}
	return true
}

func sealFailedProviderStreamOutput(output ProviderStreamOutput) ProviderStreamOutput {
	output.Result.Chunks = nil
	output.Text = ""
	output.Reasoning = ""
	output.ReasoningSignature = ""
	return output
}

func providerFirstTokenLatencyMillis(startedAt time.Time, observedAt time.Time, now time.Time) int64 {
	if startedAt.IsZero() {
		return 0
	}
	if now.Before(startedAt) {
		now = startedAt
	}
	if observedAt.IsZero() || observedAt.Before(startedAt) || observedAt.After(now) {
		observedAt = now
	}
	return observedAt.Sub(startedAt).Milliseconds()
}
