package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

// Characterize the concrete fallback conflict independently of the runtime's
// error classifier. A deadline after a committed boundary cannot supersede its
// private preparation, public winner or terminal event delivery.
func TestRuntimeDeadlineFailurePreservesCommittedSourceUnavailable(t *testing.T) {
	fixture := newConcreteRegistryFinalizerFixture(t)
	ctx := context.Background()
	committed, err := fixture.finalizer.PersistBoundary(ctx, PersistCaseBoundaryInput{
		Store: fixture.store, Context: fixture.securityContext,
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		TerminalReason: TerminalSourceUnavailable, SourceUnavailable: true,
		AcceptedAt: fixture.now,
	})
	if err != nil || committed.Boundary.Envelope.Variant != domainevidence.SourceUnavailableAnswer || fixture.store.status != "completed" {
		t.Fatal("fixture did not commit the original source-unavailable winner")
	}
	beforeRecords, err := fixture.privateStore.List(ctx)
	if err != nil || len(beforeRecords) != 1 {
		t.Fatal("fixture lacks exactly one authentic private preparation")
	}
	before, err := json.Marshal([]any{beforeRecords, fixture.privateStore.dispositions, fixture.store.status, fixture.store.items, fixture.store.fields, fixture.store.events})
	if err != nil {
		t.Fatal(err)
	}
	finishCalls, putCalls := fixture.store.finishCalls, fixture.privateStore.putCalls
	finalizeCalls := 0
	err = PersistRuntimeFailure(ctx, PersistRuntimeFailureInput{
		Context:  fixture.securityContext,
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		Cause: context.DeadlineExceeded, At: fixture.now.Add(time.Second),
		FinalizeCase: func(ctx context.Context, input PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error) {
			finalizeCalls++
			if ctx.Err() != nil || input.TerminalReason != TerminalTimeout || input.SourceUnavailable {
				t.Fatal("fallback lost its independent live timeout boundary")
			}
			input.Store = fixture.store
			return fixture.finalizer.PersistBoundary(ctx, input)
		},
	})
	if err == nil || err.Error() != "case terminal already has another private preparation" || finalizeCalls != 1 {
		t.Fatal("deadline fallback did not reach the exact same-turn private admission conflict")
	}
	afterRecords, err := fixture.privateStore.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal([]any{afterRecords, fixture.privateStore.dispositions, fixture.store.status, fixture.store.items, fixture.store.fields, fixture.store.events})
	if err != nil || !bytes.Equal(before, after) || fixture.store.finishCalls != finishCalls || fixture.privateStore.putCalls != putCalls {
		t.Fatal("deadline fallback changed preparation, winner or event delivery")
	}
}
