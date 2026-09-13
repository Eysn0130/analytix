package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"testing"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestAcceptedFinalStagePublishInterleaveCannotSkipBatch(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "accepted final reservation"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	otherThread, err := store.CreateThread(map[string]any{"title": "independent thread"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	otherThreadID := stringField(otherThread, "id")
	template := acceptedFinalDeliveryBatchForServerTest(t, threadID, 1)
	commitID := template.PublicationCommitID
	delivery := store.AcceptedFinalEventDelivery()

	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	type stagedResult struct {
		events []map[string]any
		err    error
	}
	staged := make(chan stagedResult, 1)
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- delivery.WithReservation(
			context.Background(), threadID, commitID,
			func(reservedContext context.Context) error {
				written, err := delivery.Stage(reservedContext, template.Events)
				staged <- stagedResult{events: written, err: err}
				if err != nil {
					return err
				}
				<-release
				if err := delivery.VerifyReservedTail(reservedContext, written); err != nil {
					return err
				}
				seal, err := acceptedFinalReservationSealForTest(written)
				if err != nil {
					return err
				}
				return delivery.ActivateAndPublish(
					reservedContext, written, seal, func() error { return nil },
				)
			},
		)
	}()

	var written []map[string]any
	select {
	case result := <-staged:
		if result.err != nil {
			t.Fatal(result.err)
		}
		written = result.events
	case <-time.After(time.Second):
		t.Fatal("accepted final staging did not reach the reservation barrier")
	}
	if len(written) != 3 {
		t.Fatalf("staged accepted final event count = %d, want 3", len(written))
	}
	before, err := store.LoadEventsSince(threadID, 0)
	if err != nil || !sameAcceptedFinalReservationJSON(before.Events, written) {
		t.Fatalf("staged durable tail mismatch: read=%#v written=%#v err=%v", before.Events, written, err)
	}

	if _, _, err := store.RecordEvent(map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": "turn-late", "stage": "response_received",
	}); !errors.Is(err, acceptedfinaleventport.ErrPublicationReserved) {
		t.Fatalf("same-thread event crossed accepted-final reservation: %v", err)
	}
	rawSameThread := `{"kind":"pipeline_stage","threadId":"` + threadID + `","turnId":"turn-late","stage":"response_received"}`
	if err := store.AppendRawEventLine(threadID, rawSameThread); !errors.Is(err, acceptedfinaleventport.ErrPublicationReserved) {
		t.Fatalf("raw same-thread append crossed accepted-final reservation: %v", err)
	}
	if _, err := store.RecordGeneralTerminalEventBundle(threadID, "turn-late"); !errors.Is(err, acceptedfinaleventport.ErrPublicationReserved) {
		t.Fatalf("general terminal append crossed accepted-final reservation: %v", err)
	}
	if _, _, err := store.RecordEvent(map[string]any{
		"kind": "pipeline_stage", "threadId": otherThreadID, "turnId": "turn-independent", "stage": "response_received",
	}); err != nil {
		t.Fatalf("reservation blocked an independent thread: %v", err)
	}
	select {
	case event := <-live:
		t.Fatalf("accepted final escaped live before activation: %#v", event)
	default:
	}
	afterRejected, err := store.LoadEventsSince(threadID, 0)
	if err != nil || !sameAcceptedFinalReservationJSON(afterRejected.Events, written) {
		t.Fatalf("rejected interleave changed durable tail: %#v err=%v", afterRejected.Events, err)
	}

	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	var delivered domainevent.AcceptedFinalDeliveryBatchV2
	select {
	case event := <-live:
		delivered, err = domainevent.ParseAcceptedFinalDeliveryBatchV2(event)
		if err != nil || !sameAcceptedFinalReservationJSON(delivered.Events, written) {
			t.Fatalf("live accepted final delivery = %#v, err=%v", event, err)
		}
	case <-time.After(time.Second):
		t.Fatal("accepted final reservation did not publish one closed batch")
	}

	recorded, _, err := store.RecordEvent(map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": "turn-after", "stage": "response_received",
	})
	if err != nil {
		t.Fatal(err)
	}
	nextSeq, ok := contracts.NumericSeq(recorded["seq"])
	if !ok || nextSeq != delivered.LastSeq+1 {
		t.Fatalf("post-final event seq = %v, want %d", recorded["seq"], delivered.LastSeq+1)
	}
	for _, cursor := range []int{delivered.FirstSeq, delivered.LastSeq - 1} {
		replay, err := store.LoadPublicEventsSince(threadID, cursor)
		if err != nil || len(replay.Events) < len(written) || !sameAcceptedFinalReservationJSON(replay.Events[:len(written)], written) {
			t.Fatalf("cursor %d skipped accepted final: events=%#v err=%v", cursor, replay.Events, err)
		}
	}
}

func sameAcceptedFinalReservationJSON(left, right []map[string]any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func acceptedFinalReservationSealForTest(events []map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	threadID := contracts.StringField(events[0], "threadId")
	return domainevent.NewAcceptedFinalDeliverySealForEventsV2(
		events,
		digestServerBatchTest("accepted-final-reservation-disposition-"+threadID),
		digestServerBatchTest("accepted-final-reservation-terminal-"+threadID),
		digestServerBatchBytes(publicKey),
		publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
}
