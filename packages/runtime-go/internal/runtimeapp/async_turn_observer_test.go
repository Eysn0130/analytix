package runtimeapp

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"analytix.local/runtime-go/internal/server"
)

// Identities stay only in this cell-owned memory. Reports contain controlled
// fixture phase names and closed server observation fields, never raw errors.
type runtimeAsyncTurnGuardV1 struct {
	mu      sync.Mutex
	fault   string
	records map[string]*runtimeAsyncTurnRecordV1
}

type runtimeAsyncTurnRecordV1 struct {
	ordinal int
	phase   string
	starts  int
	ends    int
	result  server.AsyncTurnObservationV1
}

func newRuntimeAsyncTurnGuardV1(fault string) *runtimeAsyncTurnGuardV1 {
	return &runtimeAsyncTurnGuardV1{fault: fault, records: map[string]*runtimeAsyncTurnRecordV1{}}
}

func (guard *runtimeAsyncTurnGuardV1) record(threadID, turnID string) *runtimeAsyncTurnRecordV1 {
	key := threadID + "\x00" + turnID
	record := guard.records[key]
	if record == nil {
		record = &runtimeAsyncTurnRecordV1{ordinal: len(guard.records) + 1, phase: "internal"}
		guard.records[key] = record
	}
	return record
}

func (guard *runtimeAsyncTurnGuardV1) expect(threadID, turnID, phase string) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.record(threadID, turnID).phase = phase
}

func (guard *runtimeAsyncTurnGuardV1) observe(observation server.AsyncTurnObservationV1) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	record := guard.record(observation.ThreadID, observation.TurnID)
	if observation.Stage == "started" {
		record.starts++
	} else if observation.Stage == "finished" {
		record.ends++
		record.result = observation
	} else {
		record.starts = -1
	}
}

func (guard *runtimeAsyncTurnGuardV1) failures() ([]string, int) {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	rows := make([]*runtimeAsyncTurnRecordV1, 0, len(guard.records))
	for _, record := range guard.records {
		rows = append(rows, record)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ordinal < rows[j].ordinal })
	var failures []string
	for _, record := range rows {
		result := record.result
		if record.starts != 1 || record.ends != 1 || result.CompletionErrorClass != "none" || result.FailureRecordErrorClass != "none" || result.TerminalStatus != "completed" {
			failures = append(failures, fmt.Sprintf("fault=%s op=%d phase=%s starts=%d ends=%d completion_phase=%s completion_error=%s failure_record_error=%s terminal_status=%s completion_detail=%s failure_record_detail=%s", guard.fault, record.ordinal, record.phase, record.starts, record.ends, result.CompletionPhase, result.CompletionErrorClass, result.FailureRecordErrorClass, result.TerminalStatus, result.CompletionDetailClass, result.FailureRecordDetailClass))
		}
	}
	if len(rows) == 0 {
		failures = append(failures, "no asynchronous operation was observed")
	}
	return failures, len(rows)
}

func (guard *runtimeAsyncTurnGuardV1) assertDrained(t *testing.T) {
	t.Helper()
	failures, operations := guard.failures()
	for _, failure := range failures {
		t.Errorf("R131 async terminal guard: %s", failure)
	}
	if len(failures) != 0 {
		t.FailNow()
	}
	t.Logf("R131 async terminal guard fault=%s complete_operations=%d failures=0", guard.fault, operations)
}

func TestRuntimeAsyncTerminalGuardRejectsLateOrMissingFailure(t *testing.T) {
	for _, mode := range []string{"complete", "missing_start", "missing_finish", "late_execution_failure", "failure_record_failed", "duplicate_finish", "nonterminal"} {
		t.Run(mode, func(t *testing.T) {
			guard := newRuntimeAsyncTurnGuardV1("synthetic")
			guard.expect("private-thread", "private-turn", "ordinary")
			row := server.AsyncTurnObservationV1{ThreadID: "private-thread", TurnID: "private-turn", Stage: "started"}
			if mode != "missing_start" {
				guard.observe(row)
			}
			row.Stage, row.CompletionPhase = "finished", "general_candidate_commit"
			row.CompletionErrorClass, row.FailureRecordErrorClass, row.TerminalStatus = "none", "none", "completed"
			if mode == "late_execution_failure" || mode == "failure_record_failed" {
				row.CompletionErrorClass = "unclassified"
			}
			if mode == "failure_record_failed" {
				row.FailureRecordErrorClass = "unclassified"
			}
			if mode == "nonterminal" {
				row.TerminalStatus = "running"
			}
			if mode != "missing_finish" {
				guard.observe(row)
			}
			if mode == "duplicate_finish" {
				guard.observe(row)
			}
			failures, count := guard.failures()
			if count != 1 || (len(failures) == 0) != (mode == "complete") {
				t.Fatal("guard failed to distinguish complete async success from late or missing failure")
			}
		})
	}
}
