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
var ErrProviderOutputByteBudgetExceeded = errors.New("provider output byte budget exceeded")

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
	// OnProcessFrame receives only ephemeral native framing or private drafts. Failure of
	// this optional observer cannot change Provider outcomes or continuation.
	OnProcessFrame           func(domainmodel.ProcessFrameV1)
	OnTextDelta              func(string) error
	OnTextChunk              func(domainmodel.Chunk) error
	OnToolCallStart          func(domainmodel.ToolCall) error
	OnProviderRetrying       func(attempt int, maxAttempts int, cause error) error
	OnProviderRetryLifecycle func(domainmodel.ProviderRetryV1, int, error) error
}

type ProviderAttemptSettlement func(context.Context, string, string) error

type ProviderStreamInput struct {
	Provider            ports.ProviderClient
	Request             domainmodel.Request
	SecurityContext     domainsecurity.TurnSecurityContext
	HostEntitySelection HostCaseEntitySelectionV1
	// OrdinaryEffect is host-owned. Its zero value preserves the strict
	// case-data projection used by existing case callers.
	OrdinaryEffect bool
	MaxAttempts    int
	// MaxOutputBytes bounds text, reasoning and tool material before buffering; zero preserves the existing unlimited byte policy.
	MaxOutputBytes        int
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

// ProviderFailureDiagnosticV1 returns only closed diagnostic labels. It does
// not inspect error text, request data or failure Details, and grants no retry
// or recovery authority. Only the existing attempt markers identify an exact
// pre-transport callback; a privacy sentinel identifies its semantic boundary.
func ProviderFailureDiagnosticV1(err error) (stage, cause string) {
	var acquire providerAttemptAcquireError
	if errors.As(err, &acquire) {
		return "acquire", providerFailureCauseV1(acquire.Err)
	}
	var prepare providerAttemptPrepareError
	if errors.As(err, &prepare) {
		return "prepare", providerFailureCauseV1(prepare.Err)
	}
	if cause, ok := providerPrivacyFailureCauseV1(err); ok {
		return "privacy_projection", cause
	}
	return "unknown", providerFailureCauseV1(err)
}

func providerPrivacyFailureCauseV1(err error) (string, bool) {
	for _, candidate := range []struct {
		err   error
		cause string
	}{
		{privacyprojectionapp.ErrProviderPrivacyAuthorityUnavailable, "provider_privacy_authority_unavailable"},
		{privacyprojectionapp.ErrUninspectableProviderPart, "uninspectable_provider_part"},
		{privacyprojectionapp.ErrProviderSchemaContainsPII, "provider_schema_contains_pii"},
		{privacyprojectionapp.ErrOrdinaryProviderContainsCaseEffect, "ordinary_provider_contains_case_effect"},
		{privacyprojectionapp.ErrProviderLogicalEffectInvalid, "provider_logical_effect_invalid"},
	} {
		if errors.Is(err, candidate.err) {
			return candidate.cause, true
		}
	}
	return "", false
}

func providerFailureCauseV1(err error) string {
	if cause, ok := providerPrivacyFailureCauseV1(err); ok {
		return cause
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline_exceeded"
	}
	var failure TurnFailureError
	var failurePointer *TurnFailureError
	code := ""
	if errors.As(err, &failure) {
		code = failure.Code
	} else if errors.As(err, &failurePointer) && failurePointer != nil {
		code = failurePointer.Code
	}
	switch code {
	case "turn_security_authority_unavailable", "turn_security_identity_mismatch",
		"turn_security_workspace_mismatch", "turn_security_case_binding_mismatch",
		"turn_security_dataset_snapshot_mismatch", "turn_security_risk_policy_mismatch",
		"turn_security_context_invalid", "source_probe_unavailable", "tool_source_unavailable":
		return code
	default:
		return "unknown"
	}
}

func StreamProviderWithRetry(ctx context.Context, input ProviderStreamInput) (ProviderStreamOutput, error) {
	if err := domainmodel.ValidateReasoningEffortV1(input.Request.ReasoningEffort); err != nil {
		return ProviderStreamOutput{}, err
	}
	if input.MaxOutputBytes < 0 {
		return ProviderStreamOutput{}, ErrProviderOutputByteBudgetExceeded
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
	estimate  appmodel.TextTokenEstimatorV1
}

func newProviderOutputTokenBudgetV1(maxTokens int) *providerOutputTokenBudgetV1 {
	return &providerOutputTokenBudgetV1{maxTokens: maxTokens}
}

func (budget *providerOutputTokenBudgetV1) Observe(chunk domainmodel.Chunk) error {
	if budget == nil || budget.maxTokens <= 0 {
		return nil
	}
	budget.observed = true
	budget.estimate.WriteString(chunk.DiscardedToolMaterial)
	switch chunk.Kind {
	case domainmodel.ChunkReasoning, domainmodel.ChunkText:
		budget.estimate.WriteString(chunk.Text)
	case domainmodel.ChunkToolCallStart, domainmodel.ChunkToolCall:
		budget.estimate.WriteString(chunk.ToolCall.Name)
		budget.estimate.WriteString(string(chunk.ToolCall.Arguments))
	case domainmodel.ChunkUsage:
		if providerReportedOutputTokensV1(chunk.Usage) > budget.maxTokens {
			return ErrProviderOutputTokenBudgetExceeded
		}
	}
	if budget.estimate.Tokens() > budget.maxTokens {
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

func streamProviderWithRetry(ctx context.Context, input ProviderStreamInput) (output ProviderStreamOutput, retErr error) {
	var pendingRetry *domainmodel.ProviderRetryV1
	notifyRetry := func(retry domainmodel.ProviderRetryV1, max int, cause error) error {
		if input.Callbacks.OnProviderRetryLifecycle != nil {
			if err := input.Callbacks.OnProviderRetryLifecycle(retry, max, cause); err != nil {
				return err
			}
		}
		if retry.Phase == "scheduled" && input.Callbacks.OnProviderRetrying != nil {
			target := retry.OuterAttempt
			if retry.Source == "transport_reconnect" {
				target = retry.PhysicalAttempt
			}
			if err := input.Callbacks.OnProviderRetrying(target, max, cause); err != nil {
				// The new observer already accepted scheduled. A legacy admission
				// callback can still reject before invocation; close that observation.
				if input.Callbacks.OnProviderRetryLifecycle != nil {
					retry.Phase, retry.At = "cancelled", time.Now().UTC()
					retry.DelayMs, retry.DeadlineUnixMs = nil, nil
					retry.CancelReason = domainmodel.ProviderRetryCancelReasonV1(ctx.Err(), err)
					return errors.Join(err, input.Callbacks.OnProviderRetryLifecycle(retry, max, err))
				}
				return err
			}
		}
		return nil
	}
	defer func() {
		if pendingRetry == nil {
			return
		}
		retry := *pendingRetry
		retry.Phase, retry.At = "cancelled", time.Now().UTC()
		retry.DelayMs, retry.DeadlineUnixMs = nil, nil
		retry.CancelReason = domainmodel.ProviderRetryCancelReasonV1(ctx.Err(), retErr)
		if err := notifyRetry(retry, input.MaxAttempts, retErr); err != nil {
			output = sealFailedProviderStreamOutput(output)
			retErr = errors.Join(retErr, ProviderStreamCallbackError{Err: err})
		}
	}()
	if input.Provider == nil {
		return ProviderStreamOutput{}, errors.New("provider client is not configured")
	}
	providerRequestToolManifestHash := toolcatalogapp.ToolSchemaHash(input.Request.Tools)
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultProviderStreamMaxAttempts
	}
	input.MaxAttempts = maxAttempts
	var err error
	var cacheObservations []domaincache.ProviderCallObservationV1
	var processAttemptCount uint64
	if binding := input.Request.PrivateProviderTelemetry; binding != nil && binding.StepStartedAt.IsZero() {
		copyBinding := *binding
		copyBinding.StepStartedAt = time.Now().UTC()
		input.Request.PrivateProviderTelemetry = &copyBinding
	}
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
		var reasoningBuffer strings.Builder
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
		outputBytes := 0
		var outputBytesErr error
		providerStartedAt := time.Now()
		hostTiming := domainmodel.ProviderTiming{StartedAt: providerStartedAt}
		firstTokenLatencyMs := int64(-1)
		firstReasoningLatencyMs := int64(-1)
		firstRawTextLatencyMs := int64(-1)
		markFirstToken := func(observedAt time.Time) {
			if firstTokenLatencyMs < 0 {
				firstTokenLatencyMs = providerFirstTokenLatencyMillis(providerStartedAt, observedAt, time.Now())
			}
		}
		processAttemptBase := processAttemptCount
		processLocalAttempt := uint64(1)
		processAttemptOpened := false
		processAttemptTerminal := false
		acceptProcessFrame := func(frame domainmodel.ProcessFrameV1) {
			if frame.Source != "" && frame.Source != "assistant_draft" && frame.Source != "provider_reasoning" && frame.Source != "tool_preparation" {
				return
			}
			if frame.ReasoningForm != "" && (frame.ReasoningForm != "summary" || frame.Source != "" && frame.Source != "provider_reasoning") {
				return
			}
			localAttempt := frame.Attempt
			if localAttempt == 0 {
				localAttempt = processLocalAttempt
			}
			if localAttempt < processLocalAttempt {
				return
			}
			if localAttempt > processLocalAttempt {
				processLocalAttempt = localAttempt
				processAttemptOpened = false
				processAttemptTerminal = false
			}
			frame.Attempt = processAttemptBase + processLocalAttempt
			frame.ProjectionSecrets = []string{request.APIKey}
			if frame.Attempt > processAttemptCount {
				processAttemptCount = frame.Attempt
			}
			if !processAttemptOpened {
				if frame.Kind == "terminal" {
					return
				}
				processAttemptOpened = true
				notifyProcessDisplayV1(input.Callbacks.OnProcessFrame, domainmodel.ProcessFrameV1{Kind: "reset", Attempt: frame.Attempt, ProjectionSecrets: []string{request.APIKey}})
			}
			if frame.Kind == "reset" || processAttemptTerminal {
				return
			}
			switch frame.Kind {
			case "start", "delta", "end":
				if frame.Source == "" && frame.NativeIndex < 0 {
					return
				}
				if frame.Source == "tool_preparation" {
					if frame.ToolPreparation == nil || (frame.Kind != "start" && frame.Kind != "delta" && frame.Kind != "end") || frame.ToolPreparation.Index < 0 || uint64(frame.ToolPreparation.Index) > 1<<53-1 || len(frame.ToolPreparation.ToolName) > 1024 || len(frame.ToolPreparation.Arguments) > 1024*1024 {
						return
					}
					preparation := *frame.ToolPreparation
					if frame.Kind == "start" {
						if preparation.ToolName != "" || preparation.Arguments != "" || frame.Text != "" {
							return
						}
						// A slot observation never mints a call. The actual named
						// delta/start/final still share the one existing issuer.
						preparation.CallID = ""
					} else {
						issued, err := toolCallIDs.issue(preparation.CallID)
						if err != nil || issued == "" {
							return
						}
						preparation.CallID = issued
					}
					frame.ToolPreparation = &preparation
					frame.NativeIndex, frame.NativeID, frame.Text = 0, "", ""
				} else if frame.Source == "provider_reasoning" {
					if frame.HostSegmentOrdinal == 0 || frame.HostSegmentOrdinal > 128 {
						return
					}
					frame.NativeIndex, frame.NativeID = 0, ""
					if frame.Kind != "delta" {
						frame.Text = ""
					}
				} else if frame.Source == "assistant_draft" {
					if frame.Kind != "delta" || (frame.ContentClassification != "provider_text" && frame.ContentClassification != "unclassified_inline") {
						return
					}
					frame.NativeIndex, frame.NativeID = 0, ""
				} else if frame.Kind != "delta" {
					frame.Text = ""
				}
				if len(frame.Text) > 1024*1024 {
					// A display capture bound must not truncate a native block into
					// an apparently complete body or alter provider continuation.
					frame = domainmodel.ProcessFrameV1{Kind: "terminal", Attempt: frame.Attempt, ProjectionSecrets: []string{request.APIKey}}
					processAttemptTerminal = true
				}
			case "terminal":
				frame.NativeIndex, frame.NativeID, frame.Text = 0, "", ""
				frame.ReasoningForm = ""
				processAttemptTerminal = true
			default:
				return
			}
			notifyProcessDisplayV1(input.Callbacks.OnProcessFrame, frame)
		}
		request.OnChunk = func(chunk domainmodel.Chunk) error {
			// Lifecycle settlement carries no Provider bytes. Cancellation cannot
			// suppress its observation; ordinary output remains fenced below.
			if chunk.Kind == domainmodel.ChunkRetrying && chunk.Retry != nil && chunk.Retry.Phase != "scheduled" {
				if chunk.Retry.Phase != "started" && chunk.Retry.Phase != "cancelled" {
					return ProviderStreamCallbackError{Err: errInvalidPipelineTelemetry}
				}
				if err := notifyRetry(*chunk.Retry, chunk.RetryMax, nil); err != nil {
					return ProviderStreamCallbackError{Err: err}
				}
				return nil
			}
			if err := attemptCtx.Err(); err != nil {
				return ProviderStreamCallbackError{Err: err}
			}
			if chunk.ProcessFrame != nil {
				acceptProcessFrame(*chunk.ProcessFrame)
				if chunk.Kind == domainmodel.ChunkReasoning && chunk.Text == "" && chunk.Signature == "" {
					return nil
				}
			}
			if input.MaxOutputBytes > 0 {
				size := providerChunkBytes(chunk)
				if size > input.MaxOutputBytes-outputBytes {
					outputBytesErr = ErrProviderOutputByteBudgetExceeded
				}
				if outputBytesErr != nil {
					return ProviderStreamCallbackError{Err: outputBytesErr}
				}
				outputBytes += size
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
				chunk.Trace.LoopChunkCallbackAt = time.Now()
			}
			first, last := chunk.Trace.ProviderRawSSEChunkAt, chunk.Trace.ProviderLastRawSSEChunkAt
			if first.IsZero() {
				first = chunk.Trace.LoopChunkCallbackAt
			}
			if last.IsZero() {
				last = first
			}
			if first.Before(providerStartedAt) || first.After(chunk.Trace.LoopChunkCallbackAt) {
				first = chunk.Trace.LoopChunkCallbackAt
			}
			if last.Before(first) || last.After(chunk.Trace.LoopChunkCallbackAt) {
				last = chunk.Trace.LoopChunkCallbackAt
			}
			if (chunk.Kind == domainmodel.ChunkText || chunk.Kind == domainmodel.ChunkReasoning) && chunk.Text != "" {
				if hostTiming.FirstContentAt.IsZero() || first.Before(hostTiming.FirstContentAt) {
					hostTiming.FirstContentAt = first
				}
				if last.After(hostTiming.LastContentAt) {
					hostTiming.LastContentAt = last
				}
				if chunk.Kind == domainmodel.ChunkText {
					if hostTiming.FirstTextAt.IsZero() {
						hostTiming.FirstTextAt = first
					}
					hostTiming.LastTextAt = last
				}
			}
			if chunk.Kind == domainmodel.ChunkDone {
				hostTiming.FinishBoundaryAt = chunk.Trace.LoopChunkCallbackAt
			}
			switch chunk.Kind {
			case domainmodel.ChunkReasoning:
				if chunk.Text != "" {
					markFirstToken(chunk.Trace.ProviderRawSSEChunkAt)
					if firstReasoningLatencyMs < 0 {
						firstReasoningLatencyMs = providerFirstTokenLatencyMillis(providerStartedAt, chunk.Trace.ProviderRawSSEChunkAt, time.Now())
					}
					attemptOutput.StreamedDeltas = true
					reasoningBuffer.WriteString(chunk.Text)
				}
				if chunk.Signature != "" {
					attemptOutput.ReasoningSignature = chunk.Signature
				}
			case domainmodel.ChunkText:
				if chunk.Text != "" {
					markFirstToken(chunk.Trace.ProviderRawSSEChunkAt)
					if firstRawTextLatencyMs < 0 {
						firstRawTextLatencyMs = providerFirstTokenLatencyMillis(providerStartedAt, chunk.Trace.ProviderRawSSEChunkAt, time.Now())
					}
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
				acceptProcessFrame(domainmodel.ProcessFrameV1{Kind: "terminal", Attempt: processLocalAttempt})
				nextProcessAttempt := processLocalAttempt + 1
				if chunk.RetryAttempt > 0 && uint64(chunk.RetryAttempt) > nextProcessAttempt {
					nextProcessAttempt = uint64(chunk.RetryAttempt)
				}
				acceptProcessFrame(domainmodel.ProcessFrameV1{Kind: "reset", Attempt: nextProcessAttempt})
				// A transport reconnect starts a new physical provider attempt.
				// Nothing buffered from the failed attempt has crossed the public
				// callback boundary, so discard both its text candidate and its
				// private reasoning before accepting replacement bytes.
				attemptOutput.Text = ""
				reasoningBuffer.Reset()
				// Discarded bytes are not a dependency of the replacement candidate.
				hostTiming = domainmodel.ProviderTiming{StartedAt: providerStartedAt}
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
				if chunk.Retry != nil {
					if err := notifyRetry(*chunk.Retry, retryMax, errors.New(message)); err != nil {
						return ProviderStreamCallbackError{Err: err}
					}
				} else if input.Callbacks.OnProviderRetrying != nil {
					// Legacy chunks retain their scheduled-only behavior without inventing phase.
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
		invocationErr := ctx.Err()
		if invocationErr == nil {
			invocationErr = attemptCtx.Err()
		}
		if invocationErr == nil && pendingRetry != nil {
			retry := *pendingRetry
			retry.Phase, retry.At = "started", time.Now().UTC()
			retry.DelayMs, retry.DeadlineUnixMs = nil, nil
			// A failed recording cannot establish either phase durably. Preserve the
			// consumer's unresolved observation and prevent the invocation.
			pendingRetry = nil
			invocationErr = notifyRetry(retry, maxAttempts, nil)
		}
		if invocationErr != nil {
			var settleErr error
			if settleAttempt != nil {
				settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(attemptCtx), 15*time.Second)
				status, reason := "failed", "provider_invocation_rejected"
				if ctx.Err() != nil || attemptCtx.Err() != nil {
					status, reason = "cancelled", "provider_attempt_cancelled"
				}
				settleErr = settleAttempt(settleCtx, status, reason)
				cancelSettle()
			}
			request = domainmodel.Request{}
			releaseAttempt()
			return output, ProviderStreamCallbackError{Err: errors.Join(invocationErr, settleErr)}
		}
		// Reserve the first physical attempt even if this provider has no native
		// display framing. Actual reconnect ordinals are supplied by the client.
		processAttemptCount = processAttemptBase + 1
		result, streamErr := input.Provider.Stream(attemptCtx, request)
		acceptProcessFrame(domainmodel.ProcessFrameV1{Kind: "terminal", Attempt: processLocalAttempt})
		hostTiming.FinishedAt = time.Now()
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
		if input.MaxOutputBytes > 0 {
			resultBytes := 0
			for _, chunk := range result.Chunks {
				size := providerChunkBytes(chunk)
				if size > input.MaxOutputBytes-resultBytes {
					outputBytesErr = ErrProviderOutputByteBudgetExceeded
					break
				}
				resultBytes += size
			}
			if outputBytesErr != nil {
				result.Chunks = nil
				streamErr = errors.Join(streamErr, outputBytesErr)
			}
		}
		nativeTiming := result.HostTiming
		result = appmodel.NormalizeProviderResult(request, result)
		if nativeTiming.FirstContentPresenceKnown {
			hostTiming.FirstContentPresenceKnown = true
			hostTiming.FirstContentAt = time.Time{}
			firstTokenLatencyMs = -1
			// Preserve explicit absence through normalization and cancellation;
			// display callbacks and control frames cannot fill it in.
			if !nativeTiming.FirstContentAt.IsZero() && !nativeTiming.FirstContentAt.Before(providerStartedAt) && !nativeTiming.FirstContentAt.After(time.Now()) {
				hostTiming.FirstContentAt = nativeTiming.FirstContentAt
				firstTokenLatencyMs = nativeTiming.FirstContentAt.Sub(providerStartedAt).Milliseconds()
				result.FirstTokenLatencyMs, result.HasFirstTokenLatency = firstTokenLatencyMs, true
			}
			result.HostTiming = hostTiming
		}
		if budgetErr := outputBudget.ValidateResult(result); budgetErr != nil {
			streamErr = errors.Join(streamErr, budgetErr)
		}
		if streamErr == nil && providerResultOutputCapped(result) {
			// A capped response can legitimately end with an unfinished tool
			// block. Drop it before requiring an executable transcript; callback
			// identity errors and provider/context errors still take precedence.
			kept := make([]domainmodel.Chunk, 0, len(result.Chunks))
			for _, chunk := range result.Chunks {
				if chunk.Kind != domainmodel.ChunkToolCallStart && chunk.Kind != domainmodel.ChunkToolCall {
					chunk.ProcessFrame = nil
					chunk.DiscardedToolMaterial = ""
					kept = append(kept, chunk)
				}
			}
			result.Chunks = kept
		} else {
			normalizedResult, identityErr := toolCallIDs.normalizeResult(result)
			if identityErr != nil {
				result.Chunks = nil
				streamErr = errors.Join(streamErr, identityErr)
			} else {
				result = normalizedResult
			}
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
		result.HostTiming = hostTiming
		result.DurationMs = time.Since(providerStartedAt).Milliseconds()
		result.HasDuration = true
		if firstTokenLatencyMs >= 0 {
			result.FirstTokenLatencyMs = firstTokenLatencyMs
			result.HasFirstTokenLatency = true
		}
		if firstReasoningLatencyMs >= 0 {
			result.FirstReasoningLatencyMs = firstReasoningLatencyMs
			result.HasFirstReasoningLatency = true
		}
		if firstRawTextLatencyMs >= 0 {
			result.FirstRawTextLatencyMs = firstRawTextLatencyMs
			result.HasFirstRawTextLatency = true
		}
		attemptOutput.Reasoning = reasoningBuffer.String()
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
		// Failed settlement and the retry predicate above precede scheduling.
		// Authority is rechecked inside the next effect lease after this wait.
		delay := ProviderRetryDelay(err)
		if delay < 0 {
			delay = 0
		}
		frozenAt := time.Now()
		at := frozenAt.UTC()
		deadline := frozenAt.Add(delay)
		delayMs, deadlineMs := delay.Milliseconds(), deadline.UnixMilli()
		sequence := input.ContinuationAuthority.Sequence
		if input.Request.PrivateProviderTelemetry != nil {
			sequence = input.Request.PrivateProviderTelemetry.LogicalSequence
		}
		retry := domainmodel.ProviderRetryV1{SchemaVersion: 1, Source: "provider_retry", Phase: "scheduled", LogicalSequence: sequence, OuterAttempt: attempt + 1, DelayMs: &delayMs, DeadlineUnixMs: &deadlineMs, At: at}
		timer := time.NewTimer(time.Until(deadline))
		if retryErr := notifyRetry(retry, maxAttempts, err); retryErr != nil {
			timer.Stop()
			return output, ProviderStreamCallbackError{Err: retryErr}
		}
		pendingRetry = &retry
		select {
		case <-ctx.Done():
			timer.Stop()
			return sealFailedProviderStreamOutput(output), ctx.Err()
		case <-timer.C:
		}
		timer.Stop()
	}
	return sealFailedProviderStreamOutput(output), err
}

type providerOutputStartedError interface {
	ProviderOutputStarted() bool
}

func notifyProcessDisplayV1(callback func(domainmodel.ProcessFrameV1), frame domainmodel.ProcessFrameV1) {
	if callback == nil {
		return
	}
	if frame.ToolPreparation != nil {
		preparation := *frame.ToolPreparation
		frame.ToolPreparation = &preparation
	}
	defer func() { _ = recover() }()
	callback(frame)
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
		normalized.Chunks[index].ProcessFrame = nil
		normalized.Chunks[index].DiscardedToolMaterial = ""
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

func providerChunkBytes(chunk domainmodel.Chunk) int {
	return len(chunk.Text) + len(chunk.Signature) + len(chunk.ToolCall.ID) + len(chunk.ToolCall.Name) + len(chunk.ToolCall.Arguments) + len(chunk.DiscardedToolMaterial)
}
