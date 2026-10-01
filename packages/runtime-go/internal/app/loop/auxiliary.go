package loop

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

const InlineCompletionMaxPromptBytes = 64 * 1024
const InlineCompletionMaxOutputBytes = 16 * 1024
const InlineCompletionMaxOutputTokens = 2048
const InlineCompletionTimeout = 30 * time.Second

var (
	ErrInlineCompletionInvalid     = errors.New("inline_completion_invalid_request")
	ErrInlineCompletionBusy        = errors.New("inline_completion_busy")
	ErrInlineCompletionCanceled    = errors.New("inline_completion_canceled")
	ErrInlineCompletionUnavailable = errors.New("inline_completion_unavailable")
	auxiliaryThreadPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	auxiliaryUUIDPattern           = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	auxiliarySessionPattern        = regexp.MustCompile(`^[a-f0-9]{48}$`)
	auxiliaryHashPattern           = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type InlineCompletionDocument struct {
	Path         string `json:"path,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
	ObjectID     string `json:"objectId,omitempty"`
	BaseRevision string `json:"baseRevision,omitempty"`
}

type InlineCompletionRequest struct {
	ThreadID  string                   `json:"threadId"`
	RequestID string                   `json:"requestId"`
	Document  InlineCompletionDocument `json:"document"`
	Prompt    string                   `json:"prompt"`
	Model     string                   `json:"model"`
}

type InlineCompletionResult struct {
	ThreadID     string `json:"threadId"`
	RequestID    string `json:"requestId"`
	ObjectID     string `json:"objectId"`
	BaseRevision string `json:"baseRevision"`
	Text         string `json:"text"`
}

func ValidateInlineCompletionRequest(input InlineCompletionRequest) error {
	doc := input.Document
	validSession := doc.Path == "" && auxiliarySessionPattern.MatchString(doc.SessionID) && auxiliaryHashPattern.MatchString(doc.ObjectID) && auxiliaryHashPattern.MatchString(doc.BaseRevision)
	validPath := doc.SessionID == "" && doc.ObjectID == "" && doc.BaseRevision == "" && strings.TrimSpace(doc.Path) != "" && utf8.ValidString(doc.Path) && len(doc.Path) <= 4096 && !strings.ContainsRune(doc.Path, 0)
	if !auxiliaryThreadPattern.MatchString(input.ThreadID) || !auxiliaryUUIDPattern.MatchString(input.RequestID) || (!validSession && !validPath) ||
		!utf8.ValidString(input.Prompt) || strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > InlineCompletionMaxPromptBytes ||
		!utf8.ValidString(input.Model) || input.Model == "" || len(input.Model) > 128 || input.Model != strings.TrimSpace(input.Model) {
		return ErrInlineCompletionInvalid
	}
	return nil
}

type ProviderTurnCloser interface {
	CloseTurn(context.Context, domainsecurity.TurnSecurityContext, domaincache.ProviderTurnTerminalReasonV1, time.Time) (domaincache.ProviderTurnClosureV1, error)
}

type AuxiliaryProviderInput struct {
	Request         domainmodel.Request
	SecurityContext domainsecurity.TurnSecurityContext
}

type AuxiliaryProviderDependencies struct {
	Provider        ports.ProviderClient
	Closer          ProviderTurnCloser
	Stage           func(domainmodel.PipelineStage) error
	Acquire         func(context.Context, int) (context.Context, func(), error)
	Prepare         func(context.Context, int, domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error)
	ValidateCurrent func(context.Context) error
}

// RunAuxiliaryProvider uses the normal privacy/effect/settlement boundaries. It
// has no history writer, tool executor, pending-work producer or final publisher.
func RunAuxiliaryProvider(ctx context.Context, input AuxiliaryProviderInput, deps AuxiliaryProviderDependencies) (text string, err error) {
	if ctx == nil || deps.Provider == nil || deps.Closer == nil || deps.Stage == nil || deps.Acquire == nil || deps.Prepare == nil || deps.ValidateCurrent == nil ||
		domainsecurity.ValidateTurnSecurityContextForExecution(input.SecurityContext) != nil || !domainsecurity.TurnSecurityContextIsGeneral(input.SecurityContext) {
		return "", ErrInlineCompletionUnavailable
	}
	defer func() {
		reason := domaincache.ProviderTurnTerminalSuccessV1
		if err != nil {
			reason = domaincache.ProviderTurnTerminalProviderFailureV1
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = domaincache.ProviderTurnTerminalTimeoutV1
		} else if ctx.Err() != nil {
			reason = domaincache.ProviderTurnTerminalCancelV1
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		_, closeErr := deps.Closer.CloseTurn(cleanup, input.SecurityContext, reason, time.Now().UTC())
		cancel()
		err = errors.Join(err, closeErr, ctx.Err())
		if err != nil {
			text = ""
		}
	}()
	request := input.Request
	request.Tools = nil
	request.MaxOutputTokens = InlineCompletionMaxOutputTokens
	request.PrivateProviderTelemetry = &domainmodel.ProviderTelemetryBindingV1{
		SecurityContext: input.SecurityContext, UsageSource: domaincache.ProviderUsageSourceTurn,
		Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 1,
	}
	dispatch := providerDispatchGuard{pre: deps.Stage, post: deps.Stage, rejected: deps.Stage, other: deps.Stage}
	request.OnPipelineStage = dispatch.stage
	stream, streamErr := StreamProviderWithRetry(ctx, ProviderStreamInput{
		Provider: deps.Provider, Request: request, SecurityContext: input.SecurityContext, OrdinaryEffect: true,
		MaxAttempts: 1, MaxOutputBytes: InlineCompletionMaxOutputBytes,
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: deps.Acquire, PrepareProviderAttempt: deps.Prepare,
			BeforeProviderInvocation: dispatch.before, AfterProviderAttempt: dispatch.after,
			OnToolCallStart: func(domainmodel.ToolCall) error { return ErrInlineCompletionUnavailable },
		},
	})
	stream, streamErr = dispatch.finish(stream, streamErr)
	if streamErr != nil {
		return "", streamErr
	}
	if !stream.Result.StreamCompleted || stream.PartialToolStarted {
		return "", ErrInlineCompletionUnavailable
	}
	for _, chunk := range stream.Result.Chunks {
		if chunk.Kind == domainmodel.ChunkToolCall || chunk.Kind == domainmodel.ChunkToolCallStart {
			return "", ErrInlineCompletionUnavailable
		}
	}
	if err := deps.ValidateCurrent(ctx); err != nil {
		return "", err
	}
	text = privacyprojectionapp.ProjectOrdinaryText(stream.Text)
	if !utf8.ValidString(text) || len(text) > InlineCompletionMaxOutputBytes {
		return "", ErrInlineCompletionUnavailable
	}
	return text, nil
}
