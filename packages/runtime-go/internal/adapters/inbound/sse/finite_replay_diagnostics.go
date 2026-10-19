package sse

import (
	"encoding/json"
	"io"
	"sync/atomic"
	"time"
)

// FiniteReplayPhase names operations before the finite replay response is
// written. It deliberately carries no request identity or event content.
type FiniteReplayPhase string

const (
	ReplayValidateThread      FiniteReplayPhase = "validate_thread"
	ReplayPublicPreflight     FiniteReplayPhase = "public_preflight"
	ReplayHighestSeq          FiniteReplayPhase = "highest_seq"
	ReplayLoadEvents          FiniteReplayPhase = "load_events"
	ReplayTerminalGroups      FiniteReplayPhase = "terminal_groups"
	ReplayAcceptedFinalGroups FiniteReplayPhase = "accepted_final_groups"
	ReplayProjectPublic       FiniteReplayPhase = "project_public"
	ReplayResponseWrite       FiniteReplayPhase = "response_write"
)

const FiniteReplayDiagnosticPrefix = "ANALYTIX_FINITE_REPLAY_DIAGNOSTIC "

var finiteReplayRequestCount atomic.Uint64

type FiniteReplayDiagnostics struct {
	writer       io.Writer
	startedAt    time.Time
	requestCount uint64
}

// NewFiniteReplayDiagnostics is used only by the explicitly enabled finite
// GET diagnostic route. A nil observer does no clock, I/O or projection work.
func NewFiniteReplayDiagnostics(writer io.Writer) *FiniteReplayDiagnostics {
	if writer == nil {
		return nil
	}
	d := &FiniteReplayDiagnostics{writer: writer, startedAt: time.Now(), requestCount: finiteReplayRequestCount.Add(1)}
	d.emit("request_start", d.startedAt, 0)
	return d
}

func (d *FiniteReplayDiagnostics) Begin(phase FiniteReplayPhase) func(int) {
	if d == nil {
		return ignoreFiniteReplayCount
	}
	switch phase {
	case ReplayValidateThread, ReplayPublicPreflight, ReplayHighestSeq, ReplayLoadEvents,
		ReplayTerminalGroups, ReplayAcceptedFinalGroups, ReplayProjectPublic, ReplayResponseWrite:
	default:
		return ignoreFiniteReplayCount
	}
	startedAt := time.Now()
	d.emit(string(phase)+"_start", startedAt, 0)
	return func(count int) { d.emit(string(phase)+"_end", startedAt, count) }
}

func ignoreFiniteReplayCount(int) {}

func (d *FiniteReplayDiagnostics) Finish(cancelled bool) {
	if d == nil {
		return
	}
	phase := "request_end"
	if cancelled {
		phase = "request_cancelled"
	}
	d.emit(phase, d.startedAt, 0)
}

func (d *FiniteReplayDiagnostics) emit(phase string, phaseStartedAt time.Time, count int) {
	// Optional diagnostics must not change delivery or error handling, even if
	// their writer fails. Only this fixed, value-free record reaches stderr.
	defer func() { _ = recover() }()
	now := time.Now()
	data, err := json.Marshal(struct {
		Phase          string `json:"phase"`
		ElapsedMs      int64  `json:"elapsedMs"`
		PhaseElapsedMs int64  `json:"phaseElapsedMs"`
		RequestCount   uint64 `json:"requestCount"`
		Count          int    `json:"count"`
	}{phase, now.Sub(d.startedAt).Milliseconds(), now.Sub(phaseStartedAt).Milliseconds(), d.requestCount, max(0, count)})
	if err == nil {
		// One bounded write keeps concurrent request records from interleaving.
		_, _ = d.writer.Write(append(append([]byte(FiniteReplayDiagnosticPrefix), data...), '\n'))
	}
}
