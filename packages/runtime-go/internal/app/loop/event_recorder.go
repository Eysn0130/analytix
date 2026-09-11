package loop

import (
	"errors"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/ports"
)

type RuntimeEventRecorder struct {
	events       ports.EventRecorder
	traceEnabled bool
}

type atomicRuntimeEventBatchRecorder interface {
	RecordEventsAtomic([]map[string]any) ([]map[string]any, []string, error)
}

func NewRuntimeEventRecorder(events ports.EventRecorder) *RuntimeEventRecorder {
	return &RuntimeEventRecorder{events: events}
}

func NewRuntimeEventRecorderWithTrace(events ports.EventRecorder, traceEnabled bool) *RuntimeEventRecorder {
	return &RuntimeEventRecorder{events: events, traceEnabled: traceEnabled}
}

func (r *RuntimeEventRecorder) AssistantTextDelta(threadID, turnID, text string) error {
	return r.AssistantTextDeltaWithTrace(threadID, turnID, text, domainmodel.ChunkTrace{})
}

func (r *RuntimeEventRecorder) AssistantTextDeltaWithTrace(threadID, turnID, text string, chunkTrace domainmodel.ChunkTrace) error {
	now := time.Now().UTC()
	return r.record(BuildAssistantDeltaEvent(AssistantDeltaEventInput{
		ThreadID:  threadID,
		TurnID:    turnID,
		EventKind: "assistant_text_delta",
		ItemKind:  "assistant_text",
		Text:      text,
		CreatedAt: now.Format(time.RFC3339Nano),
		Trace:     r.assistantDeltaTrace(chunkTrace, now),
	}))
}

func (r *RuntimeEventRecorder) PipelineStage(threadID, turnID string, stage string, details map[string]any, at time.Time, trace map[string]any) error {
	event, err := r.pipelineStageEvent(threadID, turnID, domainmodel.PipelineStage{
		Stage:   stage,
		At:      at,
		Details: details,
		Trace:   trace,
	})
	if err != nil {
		return err
	}
	return r.record(event)
}

// PipelineStageAtomic persists one transport-critical stage through the
// recorder's atomic durable path. It deliberately has no per-event fallback:
// provider transport must not begin when the configured store cannot prove
// the pre-send marker was committed.
func (r *RuntimeEventRecorder) PipelineStageAtomic(threadID, turnID string, stage domainmodel.PipelineStage) error {
	return r.PipelineStagesAtomic(threadID, turnID, []domainmodel.PipelineStage{stage})
}

// PipelineStagesAtomic persists transport-critical stages as one all-or-none
// bundle. It deliberately has no per-event fallback: a provider transport
// cannot begin unless every preceding startup stage and its pre-send marker
// share the same committed event-log frontier.
func (r *RuntimeEventRecorder) PipelineStagesAtomic(threadID, turnID string, stages []domainmodel.PipelineStage) error {
	if r == nil || r.events == nil {
		return errors.New("runtime loop event recorder is not configured")
	}
	if len(stages) == 0 {
		return nil
	}
	batch, ok := r.events.(atomicRuntimeEventBatchRecorder)
	if !ok {
		return errors.New("runtime loop atomic event recorder is not configured")
	}
	events := make([]map[string]any, 0, len(stages))
	for _, stage := range stages {
		event, err := r.pipelineStageEvent(threadID, turnID, stage)
		if err != nil {
			return err
		}
		events = append(events, event)
	}
	_, _, err := batch.RecordEventsAtomic(events)
	return err
}

func (r *RuntimeEventRecorder) PipelineStages(threadID, turnID string, stages []domainmodel.PipelineStage) error {
	if r == nil || r.events == nil {
		return errors.New("runtime loop event recorder is not configured")
	}
	if len(stages) == 0 {
		return nil
	}
	events := make([]map[string]any, 0, len(stages))
	for _, stage := range stages {
		event, err := r.pipelineStageEvent(threadID, turnID, stage)
		if err != nil {
			return err
		}
		events = append(events, event)
	}
	if batch, ok := r.events.(atomicRuntimeEventBatchRecorder); ok {
		_, _, err := batch.RecordEventsAtomic(events)
		return err
	}
	for _, event := range events {
		if err := r.record(event); err != nil {
			return err
		}
	}
	return nil
}

func (r *RuntimeEventRecorder) pipelineStageEvent(threadID, turnID string, stage domainmodel.PipelineStage) (map[string]any, error) {
	at := stage.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	projectedDetails, projectedTrace, err := projectPipelineTelemetry(stage.Stage, stage.Details, stage.Trace, r.traceEnabled)
	if err != nil {
		return nil, err
	}
	return BuildPipelineStageEvent(PipelineStageEventInput{
		ThreadID:  threadID,
		TurnID:    turnID,
		Stage:     stage.Stage,
		Details:   projectedDetails,
		Timestamp: at.UTC().Format(time.RFC3339Nano),
		Trace:     projectedTrace,
	}), nil
}

func (r *RuntimeEventRecorder) ToolCallPartial(threadID, turnID string, call domainmodel.ToolCall) error {
	return r.record(BuildToolCallPartialEvent(threadID, turnID, call.ID, call.Name))
}

func (r *RuntimeEventRecorder) ProviderRetrying(threadID, turnID string, attempt int, maxAttempts int, cause error) error {
	return r.record(BuildProviderRetryingEvent(ProviderRetryingEventInput{
		ThreadID:           threadID,
		TurnID:             turnID,
		Attempt:            attempt,
		MaxAttempts:        maxAttempts,
		Message:            SanitizeProviderRetryMessage(cause),
		ProviderDiagnostic: ProviderErrorDiagnostic(cause),
	}))
}

func (r *RuntimeEventRecorder) StreamInterruptedRecovery(threadID, turnID string, attempt int, maxAttempts int, partialToolStarted bool, cause error) error {
	return r.record(BuildStreamInterruptedRecoveryEvent(StreamInterruptedRecoveryEventInput{
		ThreadID:           threadID,
		TurnID:             turnID,
		Attempt:            attempt,
		MaxAttempts:        maxAttempts,
		PartialToolStarted: partialToolStarted,
		Message:            SanitizeProviderRetryMessage(cause),
		ProviderDiagnostic: ProviderErrorDiagnostic(cause),
	}))
}

func (r *RuntimeEventRecorder) StepLimitFinalAnswerRecovery(threadID, turnID string, maxSteps int) error {
	return r.record(BuildStepLimitFinalAnswerRecoveryEvent(threadID, turnID, maxSteps))
}

func (r *RuntimeEventRecorder) ProviderError(threadID, turnID string, cause error) error {
	return r.record(BuildProviderErrorEvent(threadID, turnID, PublicFailureForError(cause), ProviderErrorDiagnostic(cause)))
}

func (r *RuntimeEventRecorder) EmptyFinalRecovery(threadID, turnID string, attempt int, maxAttempts int) error {
	return r.record(BuildEmptyFinalRecoveryEvent(EmptyFinalRecoveryEventInput{
		ThreadID:    threadID,
		TurnID:      turnID,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
	}))
}

func (r *RuntimeEventRecorder) EmptyFinalRecoveryExhausted(threadID, turnID string, attempt int, maxAttempts int) error {
	return r.record(BuildEmptyFinalRecoveryExhaustedEvent(EmptyFinalRecoveryEventInput{
		ThreadID:    threadID,
		TurnID:      turnID,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
	}))
}

func (r *RuntimeEventRecorder) LoopGuard(threadID, turnID, toolName string, count int, guardKind string) error {
	return r.record(BuildLoopGuardEvent(threadID, turnID, toolName, count, guardKind))
}

func (r *RuntimeEventRecorder) record(event map[string]any) error {
	if r == nil || r.events == nil {
		return errors.New("runtime loop event recorder is not configured")
	}
	_, _, err := r.events.RecordEvent(event)
	return err
}

func (r *RuntimeEventRecorder) assistantDeltaTrace(chunkTrace domainmodel.ChunkTrace, recordStartedAt time.Time) map[string]any {
	if !r.traceEnabled {
		return nil
	}
	trace := map[string]any{
		"event_record_started_at": UnixMillis(recordStartedAt),
		"event_recorded_at":       UnixMillis(recordStartedAt),
	}
	if !chunkTrace.ProviderRequestSentAt.IsZero() {
		trace["provider_request_sent_at"] = UnixMillis(chunkTrace.ProviderRequestSentAt)
	}
	if !chunkTrace.ProviderResponseHeadersAt.IsZero() {
		trace["provider_response_headers_at"] = UnixMillis(chunkTrace.ProviderResponseHeadersAt)
	}
	if !chunkTrace.ProviderRawSSEChunkAt.IsZero() {
		trace["provider_raw_sse_chunk_at"] = UnixMillis(chunkTrace.ProviderRawSSEChunkAt)
	}
	if !chunkTrace.ProviderChunkParsedAt.IsZero() {
		trace["provider_chunk_parsed_at"] = UnixMillis(chunkTrace.ProviderChunkParsedAt)
	}
	if !chunkTrace.LoopChunkCallbackAt.IsZero() {
		trace["loop_chunk_callback_at"] = UnixMillis(chunkTrace.LoopChunkCallbackAt)
	}
	return trace
}
