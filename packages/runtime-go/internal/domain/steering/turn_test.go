package steering

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCurrentExecutableTurnV1RequiresLatestRunningFrozenContext(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace",
		ContextEpoch: 3, IssuedAt: time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := steeringSecurityRecord(t, securityContext)
	validTurn := map[string]any{"id": securityContext.TurnID, "status": "running", "securityContext": record}
	validThread := map[string]any{"turns": []any{validTurn}}
	turns, turn, err := CurrentExecutableTurnV1(validThread, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest)
	if err != nil || len(turns) != 1 || turn["id"] != securityContext.TurnID {
		t.Fatalf("valid frozen turn was rejected: turns=%#v turn=%#v err=%v", turns, turn, err)
	}

	for _, test := range []struct {
		name     string
		thread   map[string]any
		threadID string
		turnID   string
		digest   string
		want     error
	}{
		{name: "missing turn", thread: map[string]any{"turns": []any{}}, threadID: securityContext.ThreadID, turnID: securityContext.TurnID, digest: securityContext.ContextDigest, want: ErrTurnNotFound},
		{name: "not latest", thread: validThread, threadID: securityContext.ThreadID, turnID: "turn-old", digest: securityContext.ContextDigest, want: ErrTurnNotLatest},
		{
			name: "inactive",
			thread: map[string]any{"turns": []any{map[string]any{
				"id": securityContext.TurnID, "status": "completed", "securityContext": record,
			}}},
			threadID: securityContext.ThreadID, turnID: securityContext.TurnID,
			digest: securityContext.ContextDigest, want: ErrTurnInactive,
		},
		{name: "invalid digest", thread: validThread, threadID: securityContext.ThreadID, turnID: securityContext.TurnID, digest: "not-a-digest", want: ErrContextMismatch},
		{name: "wrong thread", thread: validThread, threadID: "thread-elsewhere", turnID: securityContext.TurnID, digest: securityContext.ContextDigest, want: ErrContextMismatch},
		{
			name: "malformed context",
			thread: map[string]any{"turns": []any{map[string]any{
				"id": securityContext.TurnID, "status": "running",
				"securityContext": map[string]any{"contextDigest": securityContext.ContextDigest},
			}}},
			threadID: securityContext.ThreadID, turnID: securityContext.TurnID,
			digest: securityContext.ContextDigest, want: ErrContextMismatch,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := CurrentExecutableTurnV1(test.thread, test.threadID, test.turnID, test.digest); !errors.Is(err, test.want) {
				t.Fatalf("current turn error = %v, want %v", err, test.want)
			}
		})
	}
}

func steeringSecurityRecord(t *testing.T, securityContext domainsecurity.TurnSecurityContext) map[string]any {
	t.Helper()
	body, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{}
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	return record
}
