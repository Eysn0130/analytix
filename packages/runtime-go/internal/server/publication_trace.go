package server

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var publicationTraceOrigin = time.Now()
var publicationTraceMu sync.Mutex
var publicationTracePending [][]byte
var publicationTraceDraining bool

// One bounded diagnostic per committed terminal, never one write per chunk.
// The single drain worker exits when empty; no idle observer survives a cohort.
// Saturation or a blocked diagnostics pipe drops observations rather than
// delaying terminal publication, ACK, cancellation, or mandatory audit storage.
func recordRuntimePublicationTrace(threadID, turnID string, result apploop.RuntimeAgentLoopResult, validationStartedAt time.Time, timing appturn.PublicationTiming) {
	if !runtimeThreadTraceEnabled() || timing.CommittedAt.IsZero() {
		return
	}
	body := runtimePublicationTrace(threadID, turnID, result, validationStartedAt, timing)
	publicationTraceMu.Lock()
	defer publicationTraceMu.Unlock()
	if len(publicationTracePending) >= 64 {
		return
	}
	publicationTracePending = append(publicationTracePending, body)
	if publicationTraceDraining {
		return
	}
	publicationTraceDraining = true
	go func() {
		for {
			publicationTraceMu.Lock()
			if len(publicationTracePending) == 0 {
				publicationTraceDraining = false
				publicationTracePending = nil
				publicationTraceMu.Unlock()
				return
			}
			next := publicationTracePending[0]
			publicationTracePending[0] = nil
			publicationTracePending = publicationTracePending[1:]
			publicationTraceMu.Unlock()
			_, _ = fmt.Fprintf(os.Stderr, "[analytix-thread-trace] %s\n", next)
		}
	}()
}

func runtimePublicationTrace(threadID, turnID string, result apploop.RuntimeAgentLoopResult, validationStartedAt time.Time, timing appturn.PublicationTiming) []byte {
	data := map[string]any{"pid": os.Getpid(), "timeOrigin": float64(publicationTraceOrigin.UnixMicro()) / 1000}
	for key, at := range map[string]time.Time{
		"providerStartedMs":              result.LastResult.HostTiming.StartedAt,
		"firstProviderContentMs":         result.LastResult.HostTiming.FirstContentAt,
		"lastProviderContentMs":          result.LastResult.HostTiming.LastContentAt,
		"firstProviderTextMs":            result.LastResult.HostTiming.FirstTextAt,
		"lastProviderTextMs":             result.LastResult.HostTiming.LastTextAt,
		"providerFinishBoundaryMs":       result.LastResult.HostTiming.FinishBoundaryAt,
		"providerFinishedMs":             result.LastResult.HostTiming.FinishedAt,
		"lastDependencyMs":               result.LastDependencyAt,
		"candidateProjectionStartedMs":   result.CandidateProjectionStartedAt,
		"candidateProjectionReadyMs":     result.CandidateProjectionReadyAt,
		"publicationValidationStartedMs": validationStartedAt,
		"publicationProjectionReadyMs":   timing.ProjectionReadyAt,
		"publicationCommittedMs":         timing.CommittedAt,
		"sseDeliverableMs":               timing.DeliverableAt,
	} {
		if !at.IsZero() {
			data[key] = float64(at.Sub(publicationTraceOrigin)) / float64(time.Millisecond)
		}
	}
	if !result.LastDependencyAt.IsZero() && !timing.CommittedAt.Before(result.LastDependencyAt) {
		data["publicationGapMs"] = float64(timing.CommittedAt.Sub(result.LastDependencyAt)) / float64(time.Millisecond)
	}
	for i := len(result.LastResult.CacheObservations) - 1; i >= 0; i-- {
		observation := result.LastResult.CacheObservations[i]
		if observation.Validate() == nil {
			data["logicalCallHmac"] = observation.Shape.LogicalCallHMAC
			data["physicalAttempt"] = observation.Shape.Attempt
			break
		}
	}
	body, _ := json.Marshal(map[string]any{
		"name": "thread.terminal.core_committed", "threadId": "ref-" + domainsecurity.SHA256Hex([]byte(threadID)),
		"turnId": "ref-" + domainsecurity.SHA256Hex([]byte(turnID)), "data": data,
	})
	return body
}
