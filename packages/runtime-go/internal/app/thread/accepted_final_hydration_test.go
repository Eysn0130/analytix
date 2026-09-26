package thread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type acceptedFinalHydrationMultiReadbackV1 struct {
	expected [][]map[string]any
}

func (readback acceptedFinalHydrationMultiReadbackV1) matches(events []map[string]any) bool {
	for _, expected := range readback.expected {
		if reflect.DeepEqual(expected, events) {
			return true
		}
	}
	return false
}

func (readback acceptedFinalHydrationMultiReadbackV1) VerifyReservedTail(_ context.Context, events []map[string]any) error {
	if !readback.matches(events) {
		return errors.New("accepted-final hydration reserved tail is detached")
	}
	return nil
}

func (readback acceptedFinalHydrationMultiReadbackV1) VerifyCommittedManifest(_ context.Context, events []map[string]any) error {
	if !readback.matches(events) {
		return errors.New("accepted-final hydration committed manifest is detached")
	}
	return nil
}

type acceptedFinalHydrationProjectionOverrideV1 struct {
	PublicProjector
	AcceptedFinalDeliveryAuthority
	visibleForCall func(int) bool
	calls          int
}

type acceptedFinalHydrationBoundProbeV1 struct {
	projectCalls int
	sealCalls    int
}

func (probe *acceptedFinalHydrationBoundProbeV1) ProjectThread(value map[string]any) (map[string]any, error) {
	probe.projectCalls++
	return contracts.CloneMap(value), nil
}

func (probe *acceptedFinalHydrationBoundProbeV1) ProjectEvent(
	string, map[string]any, map[string]any,
) (map[string]any, bool, error) {
	probe.projectCalls++
	return nil, false, errors.New("delivery bound probe reached event projection")
}

func (probe *acceptedFinalHydrationBoundProbeV1) SealAcceptedFinalDelivery(
	context.Context, []map[string]any,
) (domainevent.AcceptedFinalDeliverySealV1, error) {
	probe.sealCalls++
	return domainevent.AcceptedFinalDeliverySealV1{}, errors.New("delivery bound probe reached signer")
}

func (*acceptedFinalHydrationBoundProbeV1) ValidateAcceptedFinalDelivery(
	context.Context, domainevent.AcceptedFinalDeliveryBatchV2,
) error {
	return errors.New("delivery bound probe reached validator")
}

func (projector *acceptedFinalHydrationProjectionOverrideV1) ProjectEvent(
	routeThreadID string,
	thread map[string]any,
	event map[string]any,
) (map[string]any, bool, error) {
	call := projector.calls
	projector.calls++
	if projector.visibleForCall != nil && !projector.visibleForCall(call) {
		return nil, false, nil
	}
	return projector.PublicProjector.ProjectEvent(routeThreadID, thread, event)
}

func TestBuildLatestAcceptedFinalHydrationUsesExactDurableCurrentAuthority(t *testing.T) {
	authority := newProjectionTestAuthority(91)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration", "turn-hydration", "case-hydration", "snapshot-hydration", 4,
	))
	events := sequencedAcceptedFinalHydrationEvents(t, fixture)
	readback := &projectionDeliveryReadbackStub{expected: events, durable: true}
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	projector := NewTrustedPublicProjectorWithPrimaryCAS(
		index, nil, nil, fixture.casReader(t, fixture.securityContext),
	)
	projected, err := projector.ProjectThread(fixture.thread())
	if err != nil {
		t.Fatal(err)
	}
	projected["latestSeq"] = float64(len(events))

	delivery, present, err := BuildLatestAcceptedFinalHydrationV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: fixture.securityContext.ThreadID,
		SnapshotLatestSeq: len(events), AuthorityThread: fixture.thread(), ProjectedThread: projected,
		DurableEvents: events, Projector: projector,
	})
	if err != nil || !present || delivery == nil {
		t.Fatalf("current durable accepted final was not hydrated: present=%t delivery=%#v err=%v", present, delivery, err)
	}
	batch, err := domainevent.ParseAcceptedFinalDeliveryBatchV2(delivery)
	if err != nil {
		t.Fatal(err)
	}
	if batch.ThreadID != fixture.securityContext.ThreadID ||
		batch.TurnID != fixture.securityContext.TurnID ||
		batch.PublicationCommitID != fixture.privateRecord.AcceptedFinal.RecordDigest ||
		batch.FirstSeq != 1 || batch.LastSeq != len(events) ||
		batch.PublicationAuthority.AuthorityKeyID != authority.KeyID() {
		t.Fatalf("hydrated delivery is detached from current authority: %#v", batch)
	}
	if err := projector.ValidateAcceptedFinalDelivery(context.Background(), batch); err != nil {
		t.Fatalf("hydrated delivery failed current validation: %v", err)
	}
}

func TestBuildAcceptedFinalHydrationProjectionSealsEveryVisibleTurnIndependently(t *testing.T) {
	authority := newProjectionTestAuthority(97)
	first := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-multiple", "turn-hydration-first", "case-hydration-multiple",
		"snapshot-hydration-multiple", 8,
	))
	second := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-multiple", "turn-hydration-second", "case-hydration-multiple",
		"snapshot-hydration-multiple", 8,
	))
	firstEvents := sequencedAcceptedFinalHydrationEvents(t, first)
	secondEvents := sequencedAcceptedFinalHydrationEventsFrom(t, second, len(firstEvents)+1)
	readback := acceptedFinalHydrationMultiReadbackV1{expected: [][]map[string]any{firstEvents, secondEvents}}
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	if err := index.SeedTerminalComplete(context.Background(), []gateprojection.TerminalCompleteFinalAuthorityV1{
		first.terminalAuthority, second.terminalAuthority,
	}); err != nil {
		t.Fatal(err)
	}
	authorityThread := first.thread()
	authorityThread["securityState"] = publicProjectionSecurityRecord(second.securityContext)
	authorityThread["turns"] = append(
		authorityThread["turns"].([]any),
		trustedProjectionRawTurn(second.securityContext, second.privateRecord.AcceptedFinal, second.plan),
	)
	firstReader := first.casReader(t, second.securityContext)
	secondReader := second.casReader(t, second.securityContext)
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, projectionCASReaderStub{
		observations: map[string]domainevidence.AcceptedFinalCASObservationV1{
			first.securityContext.TurnID:  firstReader.observations[first.securityContext.TurnID],
			second.securityContext.TurnID: secondReader.observations[second.securityContext.TurnID],
		},
	})
	projectedThread, err := projector.ProjectThread(authorityThread)
	if err != nil {
		t.Fatal(err)
	}
	events := append(append([]map[string]any{}, firstEvents...), secondEvents...)
	projectedThread["latestSeq"] = float64(len(events))
	input := AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: first.securityContext.ThreadID,
		SnapshotLatestSeq: len(events), AuthorityThread: authorityThread,
		ProjectedThread: projectedThread, DurableEvents: events, Projector: projector,
	}
	signCallsBefore := authority.signCalls.Load()
	projection, present, err := BuildAcceptedFinalHydrationProjectionV1(input)
	if err != nil || !present || len(projection.Deliveries) != 2 {
		t.Fatalf("two visible finals did not receive independent delivery authority: projection=%#v present=%t err=%v", projection, present, err)
	}
	for index, expected := range []trustedProjectionFixture{first, second} {
		batch, parseErr := domainevent.ParseAcceptedFinalDeliveryBatchV2(projection.Deliveries[index])
		if parseErr != nil || batch.ThreadID != expected.securityContext.ThreadID ||
			batch.TurnID != expected.securityContext.TurnID ||
			batch.PublicationCommitID != expected.privateRecord.AcceptedFinal.RecordDigest ||
			projector.ValidateAcceptedFinalDelivery(context.Background(), batch) != nil {
			t.Fatalf("delivery %d is detached from its exact final: batch=%#v err=%v", index, batch, parseErr)
		}
	}
	if !reflect.DeepEqual(projection.Latest, projection.Deliveries[1]) {
		t.Fatalf("singular latest delivery is not the exact final bounded projection: %#v", projection)
	}
	if got := authority.signCalls.Load() - signCallsBefore; got != 2 {
		t.Fatalf("visible finals used %d delivery signatures, want exactly one per turn", got)
	}

	hostile := []struct {
		name   string
		mutate func(AcceptedFinalHydrationInputV1)
	}{
		{name: "old view", mutate: func(candidate AcceptedFinalHydrationInputV1) {
			turn := candidate.ProjectedThread["turns"].([]any)[0].(map[string]any)
			turn["acceptedFinalView"].(map[string]any)["acceptedFinalDigest"] = second.privateRecord.AcceptedFinal.RecordDigest
		}},
		{name: "new item", mutate: func(candidate AcceptedFinalHydrationInputV1) {
			turn := candidate.ProjectedThread["turns"].([]any)[1].(map[string]any)
			for _, value := range turn["items"].([]any) {
				item := value.(map[string]any)
				if _, ok := item["acceptedFinalView"]; ok {
					item["text"] = "DETACHED_PUBLIC_ITEM"
				}
			}
		}},
		{name: "old sequence", mutate: func(candidate AcceptedFinalHydrationInputV1) {
			candidate.DurableEvents[0]["seq"] = float64(2)
		}},
		{name: "new commit", mutate: func(candidate AcceptedFinalHydrationInputV1) {
			candidate.DurableEvents[len(firstEvents)]["publicationCommitId"] = first.privateRecord.AcceptedFinal.RecordDigest
		}},
	}
	for _, test := range hostile {
		t.Run(test.name, func(t *testing.T) {
			candidate := input
			candidate.ProjectedThread = contracts.CloneMap(input.ProjectedThread)
			candidate.DurableEvents = cloneAcceptedFinalHydrationEventsV1(input.DurableEvents)
			test.mutate(candidate)
			signCallsBefore := authority.signCalls.Load()
			if got, ok, gotErr := BuildAcceptedFinalHydrationProjectionV1(candidate); gotErr == nil || ok || len(got.Deliveries) != 0 {
				t.Fatalf("hostile multi-final mutation received delivery authority: projection=%#v present=%t err=%v", got, ok, gotErr)
			}
			if authority.signCalls.Load() != signCallsBefore {
				t.Fatalf("hostile multi-final mutation reached the delivery signer")
			}
		})
	}
	// This tests the operation boundary with the existing signed two-delivery
	// fixture. The production collector separately requires an exact fact proof.
	for _, mode := range []string{"valid", "revoked-before", "revoked-after", "cancel-after"} {
		t.Run("retained-operation-"+mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			candidate := input
			candidate.Context = ctx
			calls := 0
			local := *projector
			local.retainedFactVerifier = func(original domainevidence.PrivateAcceptedFinalRecord, current domainsecurity.TurnSecurityContext) error {
				calls++
				if !reflect.DeepEqual(original, first.privateRecord) || !reflect.DeepEqual(current, second.securityContext) {
					t.Fatal("retained operation changed original or current permission binding")
				}
				if mode == "revoked-before" || (mode == "revoked-after" && calls == 2) {
					return errors.New("revoked")
				}
				if mode == "cancel-after" && calls == 2 {
					cancel()
				}
				return nil
			}
			before := authority.signCalls.Load()
			got, present, err := buildRetainedAcceptedFinalHydrationV1(candidate, &local, map[string]bool{}, []retainedFactAdmissionV1{{original: first.privateRecord, current: second.securityContext}})
			if mode == "valid" {
				if err != nil || !present || len(got.Deliveries) != 2 || calls != 2 {
					t.Fatalf("bounded retained operation: present=%t deliveries=%d checks=%d err=%v", present, len(got.Deliveries), calls, err)
				}
			} else {
				if err == nil || present || got.Latest != nil || len(got.Deliveries) != 0 {
					t.Fatal("revoked retained operation exposed a partial batch")
				}
				if mode == "revoked-before" && (calls != 1 || authority.signCalls.Load() != before) {
					t.Fatal("rejected operation reached signing")
				}
				if mode != "revoked-before" && calls != 2 {
					t.Fatal("final operation revalidation was omitted")
				}
			}
		})
	}

}

func TestBuildAcceptedFinalHydrationProjectionCapsEveryVisibleTurnBeforeSigning(t *testing.T) {
	projectedThread := map[string]any{
		"id":    "thread-hydration-cap",
		"turns": make([]any, domainevent.AcceptedFinalDeliveryGroupLimitV1+1),
	}
	turns := projectedThread["turns"].([]any)
	for index := range turns {
		turns[index] = map[string]any{"acceptedFinalView": map[string]any{"schemaVersion": float64(3)}}
	}
	bounded := contracts.CloneMap(projectedThread)
	bounded["turns"] = bounded["turns"].([]any)[:domainevent.AcceptedFinalDeliveryGroupLimitV1]
	if err := validateAcceptedFinalHydrationDeliveryBoundV1(bounded); err != nil {
		t.Fatalf("256 accepted-final groups exceeded the exact public bound: %v", err)
	}
	if err := validateAcceptedFinalHydrationDeliveryBoundV1(projectedThread); err == nil {
		t.Fatal("257 accepted-final groups passed the exact public bound")
	}

	probe := &acceptedFinalHydrationBoundProbeV1{}
	overflow, present, err := BuildAcceptedFinalHydrationProjectionV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: "thread-hydration-cap",
		AuthorityThread: map[string]any{"id": "thread-hydration-cap", "turns": []any{}},
		ProjectedThread: projectedThread, Projector: probe,
	})
	if err == nil || present || overflow.Latest != nil || len(overflow.Deliveries) != 0 ||
		probe.projectCalls != 0 || probe.sealCalls != 0 {
		t.Fatalf("257 accepted-final groups did not fail before projection/signing: projection=%#v present=%t projections=%d signs=%d err=%v",
			overflow, present, probe.projectCalls, probe.sealCalls, err)
	}
}

func TestBuildAcceptedFinalHydrationProjectionCapsEveryDurableGroupBeforeProjectionOrSigning(t *testing.T) {
	authority := newProjectionTestAuthority(99)
	const threadID = "thread-hydration-durable-cap"
	events := make([]map[string]any, 0, (domainevent.AcceptedFinalDeliveryGroupLimitV1+1)*4)
	for index := 0; index <= domainevent.AcceptedFinalDeliveryGroupLimitV1; index++ {
		fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
			threadID,
			fmt.Sprintf("turn-hydration-durable-cap-%d", index+1),
			fmt.Sprintf("case-hydration-durable-cap-%d", index+1),
			fmt.Sprintf("snapshot-hydration-durable-cap-%d", index+1),
			1,
		))
		events = append(events, sequencedAcceptedFinalHydrationEventsFrom(t, fixture, len(events)+1)...)
	}
	probe := &acceptedFinalHydrationBoundProbeV1{}
	thread := map[string]any{"id": threadID, "turns": []any{}}
	projection, present, err := BuildAcceptedFinalHydrationProjectionV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: threadID,
		SnapshotLatestSeq: len(events), AuthorityThread: thread, ProjectedThread: thread,
		DurableEvents: events, Projector: probe,
	})
	if err == nil || present || projection.Latest != nil || len(projection.Deliveries) != 0 ||
		probe.projectCalls != 0 || probe.sealCalls != 0 {
		t.Fatalf("257 durable accepted-final groups did not fail before projection/signing: projection=%#v present=%t projections=%d signs=%d err=%v",
			projection, present, probe.projectCalls, probe.sealCalls, err)
	}
}

func TestBuildAcceptedFinalHydrationProjectionPreservesHistoricalV1BesideCurrentV2(t *testing.T) {
	authority := newProjectionTestAuthority(98)
	historical := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-mixed-wire", "turn-hydration-historical", "case-hydration-mixed-wire",
		"snapshot-hydration-mixed-wire", 9,
	))
	current := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-mixed-wire", "turn-hydration-current", "case-hydration-mixed-wire",
		"snapshot-hydration-mixed-wire", 9,
	))
	historicalEvents := sequencedHistoricalAcceptedFinalHydrationEventsV1(t, historical, 1)
	currentEvents := sequencedAcceptedFinalHydrationEventsFrom(t, current, len(historicalEvents)+1)
	readback := acceptedFinalHydrationMultiReadbackV1{expected: [][]map[string]any{historicalEvents, currentEvents}}
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	if err := index.SeedTerminalComplete(context.Background(), []gateprojection.TerminalCompleteFinalAuthorityV1{
		historical.terminalAuthority, current.terminalAuthority,
	}); err != nil {
		t.Fatal(err)
	}
	authorityThread := historical.thread()
	authorityThread["securityState"] = publicProjectionSecurityRecord(current.securityContext)
	authorityThread["turns"] = append(
		authorityThread["turns"].([]any),
		trustedProjectionRawTurn(current.securityContext, current.privateRecord.AcceptedFinal, current.plan),
	)
	historicalReader := historical.casReader(t, current.securityContext)
	currentReader := current.casReader(t, current.securityContext)
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, projectionCASReaderStub{
		observations: map[string]domainevidence.AcceptedFinalCASObservationV1{
			historical.securityContext.TurnID: historicalReader.observations[historical.securityContext.TurnID],
			current.securityContext.TurnID:    currentReader.observations[current.securityContext.TurnID],
		},
	})
	projectedAll, err := projector.ProjectThread(authorityThread)
	if err != nil {
		t.Fatal(err)
	}
	projectedCurrent := contracts.CloneMap(projectedAll)
	projectedTurns := projectedCurrent["turns"].([]any)
	projectedCurrent["turns"] = []any{contracts.CloneValue(projectedTurns[1])}
	events := append(cloneAcceptedFinalHydrationEventsV1(historicalEvents), currentEvents...)
	before, _ := json.Marshal(events)
	manifests, latestSeq, err := durableAcceptedFinalManifestsV1(historical.securityContext.ThreadID, events)
	if err != nil || latestSeq != len(events) || len(manifests) != 2 ||
		manifests[0].WireSchemaVersion != domainevent.AcceptedFinalDeliveryBatchV1Version ||
		manifests[0].WirePurpose != domainevent.AcceptedFinalDeliveryBatchV1Purpose ||
		manifests[1].WireSchemaVersion != domainevent.AcceptedFinalDeliveryBatchV2Version ||
		manifests[1].WirePurpose != domainevent.AcceptedFinalDeliveryBatchV2Purpose {
		t.Fatalf("mixed durable wire families were not classified exactly: manifests=%#v latest=%d err=%v", manifests, latestSeq, err)
	}
	signCallsBefore := authority.signCalls.Load()
	projection, present, err := BuildAcceptedFinalHydrationProjectionV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: historical.securityContext.ThreadID,
		SnapshotLatestSeq: len(events), AuthorityThread: authorityThread,
		ProjectedThread: projectedCurrent, DurableEvents: events, Projector: projector,
	})
	if err != nil || !present || len(projection.Deliveries) != 1 {
		t.Fatalf("current V2 beside historical V1 was not hydrated exactly: projection=%#v present=%t err=%v", projection, present, err)
	}
	batch, err := domainevent.ParseAcceptedFinalDeliveryBatchV2(projection.Deliveries[0])
	if err != nil || batch.TurnID != current.securityContext.TurnID ||
		batch.Purpose != domainevent.AcceptedFinalDeliveryBatchV2Purpose {
		t.Fatalf("mixed hydration rewrote or detached the current V2 batch: batch=%#v err=%v", batch, err)
	}
	after, _ := json.Marshal(events)
	if !reflect.DeepEqual(before, after) || authority.signCalls.Load()-signCallsBefore != 1 {
		t.Fatalf("historical V1 bytes were mutated or re-signed: before=%s after=%s signDelta=%d", before, after, authority.signCalls.Load()-signCallsBefore)
	}

	if got, ok, gotErr := BuildAcceptedFinalHydrationProjectionV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: historical.securityContext.ThreadID,
		SnapshotLatestSeq: len(events), AuthorityThread: authorityThread,
		ProjectedThread: projectedAll, DurableEvents: events, Projector: projector,
	}); gotErr == nil || ok || len(got.Deliveries) != 0 {
		t.Fatalf("historical V1 acquired a current public slot: projection=%#v present=%t err=%v", got, ok, gotErr)
	}

	forgedEvents := cloneAcceptedFinalHydrationEventsV1(events)
	forgedItem := forgedEvents[0]["item"].(map[string]any)
	forgedRecord := forgedItem["acceptedFinal"].(map[string]any)
	forgedRecord["authoritySignature"] = "forged-historical-signature"
	delete(forgedEvents[0], "publicationPayloadDigest")
	forgedEvents[0]["publicationPayloadDigest"] = appturn.AcceptedFinalPublicationPayloadDigest(forgedEvents[0])
	if got, ok, gotErr := BuildAcceptedFinalHydrationProjectionV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: historical.securityContext.ThreadID,
		SnapshotLatestSeq: len(forgedEvents), AuthorityThread: authorityThread,
		ProjectedThread: projectedCurrent, DurableEvents: forgedEvents, Projector: projector,
	}); gotErr == nil || ok || len(got.Deliveries) != 0 {
		t.Fatalf("forged historical V1 survived audit validation: projection=%#v present=%t err=%v", got, ok, gotErr)
	}
}

func TestBuildLatestAcceptedFinalHydrationTreatsOnlyAuthorityVerifiedAllInvisibleManifestAsAbsent(t *testing.T) {
	authority := newProjectionTestAuthority(93)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-hidden", "turn-hydration-hidden", "case-hydration-hidden",
		"snapshot-hydration-hidden", 6,
	))
	events := sequencedAcceptedFinalHydrationEvents(t, fixture)
	readback := &projectionDeliveryReadbackStub{expected: events, durable: true}
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	trusted := NewTrustedPublicProjectorWithPrimaryCAS(
		index, nil, nil, fixture.casReader(t, fixture.securityContext),
	)
	withoutReference := map[string]any{
		"id": fixture.securityContext.ThreadID,
		"turns": []any{map[string]any{
			"id": fixture.securityContext.TurnID, "threadId": fixture.securityContext.ThreadID,
			"items": []any{},
		}},
	}
	input := AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: fixture.securityContext.ThreadID,
		SnapshotLatestSeq: len(events), AuthorityThread: fixture.thread(), ProjectedThread: withoutReference,
		DurableEvents: events,
	}

	allHidden := &acceptedFinalHydrationProjectionOverrideV1{
		PublicProjector: trusted, AcceptedFinalDeliveryAuthority: trusted,
		visibleForCall: func(int) bool { return false },
	}
	input.Projector = allHidden
	signCallsBefore := authority.signCalls.Load()
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(input); err != nil || present || delivery != nil {
		t.Fatalf("authority-verified all-invisible manifest was not absent: present=%t delivery=%#v err=%v", present, delivery, err)
	}
	if allHidden.calls != len(events) {
		t.Fatalf("all-invisible manifest projected %d events, want %d", allHidden.calls, len(events))
	}
	if got := authority.signCalls.Load() - signCallsBefore; got != 0 {
		t.Fatalf("all-invisible manifest triggered %d delivery signatures", got)
	}

	mixed := &acceptedFinalHydrationProjectionOverrideV1{
		PublicProjector: trusted, AcceptedFinalDeliveryAuthority: trusted,
		visibleForCall: func(call int) bool { return call == 0 },
	}
	input.Projector = mixed
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(input); err == nil || present || delivery != nil {
		t.Fatalf("mixed-visibility manifest did not fail closed: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	input.Projector = trusted
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(input); err == nil || present || delivery != nil {
		t.Fatalf("visible manifest without an exact public slot did not fail closed: present=%t delivery=%#v err=%v", present, delivery, err)
	}
}

func TestBuildLatestAcceptedFinalHydrationFailsClosedOnTornMissingDuplicateAndRevokedState(t *testing.T) {
	authority := newProjectionTestAuthority(92)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-reject", "turn-hydration-reject", "case-hydration-reject",
		"snapshot-hydration-reject", 5,
	))
	events := sequencedAcceptedFinalHydrationEvents(t, fixture)
	readback := &projectionDeliveryReadbackStub{expected: events, durable: true}
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	projector := NewTrustedPublicProjectorWithPrimaryCAS(
		index, nil, nil, fixture.casReader(t, fixture.securityContext),
	)
	projected, err := projector.ProjectThread(fixture.thread())
	if err != nil {
		t.Fatal(err)
	}

	base := AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: fixture.securityContext.ThreadID,
		SnapshotLatestSeq: len(events), AuthorityThread: fixture.thread(), ProjectedThread: projected,
		DurableEvents: events, Projector: projector,
	}
	torn := base
	torn.SnapshotLatestSeq--
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(torn); !errors.Is(err, ErrPublicProjectionPending) || present || delivery != nil {
		t.Fatalf("torn frontier produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}
	ahead := base
	ahead.SnapshotLatestSeq++
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(ahead); err == nil || errors.Is(err, ErrPublicProjectionPending) || present || delivery != nil {
		t.Fatalf("missing durable events were treated as pending: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	missing := base
	missing.SnapshotLatestSeq = 0
	missing.DurableEvents = nil
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(missing); err == nil || errors.Is(err, ErrPublicProjectionPending) || present || delivery != nil {
		t.Fatalf("missing manifest produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	duplicate := base
	duplicate.DurableEvents = append([]map[string]any{}, events...)
	for index, event := range events {
		cloned := contracts.CloneMap(event)
		cloned["seq"] = float64(len(events) + index + 1)
		duplicate.DurableEvents = append(duplicate.DurableEvents, cloned)
	}
	duplicate.SnapshotLatestSeq = len(duplicate.DurableEvents)
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(duplicate); err == nil || present || delivery != nil {
		t.Fatalf("duplicate manifest produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	wrongTurnThread := base
	wrongTurnThread.ProjectedThread = contracts.CloneMap(projected)
	wrongTurnThread.ProjectedThread["turns"].([]any)[0].(map[string]any)["threadId"] = "thread-foreign"
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(wrongTurnThread); err == nil || present || delivery != nil {
		t.Fatalf("foreign turn thread produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	missingView := base
	missingView.ProjectedThread = contracts.CloneMap(projected)
	delete(missingView.ProjectedThread["turns"].([]any)[0].(map[string]any), "acceptedFinalView")
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(missingView); err == nil || present || delivery != nil {
		t.Fatalf("accepted final without its public view produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	missingItemView := base
	missingItemView.ProjectedThread = contracts.CloneMap(projected)
	items := missingItemView.ProjectedThread["turns"].([]any)[0].(map[string]any)["items"].([]any)
	for _, value := range items {
		item := value.(map[string]any)
		if _, present := item["acceptedFinalView"]; present {
			delete(item, "acceptedFinalView")
		}
	}
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(missingItemView); err == nil || present || delivery != nil {
		t.Fatalf("accepted final item without its public view produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	standalonePublicView := base
	standalonePublicView.AuthorityThread = contracts.CloneMap(projected)
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(standalonePublicView); err == nil || present || delivery != nil {
		t.Fatalf("standalone public V3 acquired hydration authority: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	missingPrivateRecord := base
	missingPrivateRecord.AuthorityThread = contracts.CloneMap(base.AuthorityThread)
	privateTurn := missingPrivateRecord.AuthorityThread["turns"].([]any)[0].(map[string]any)
	delete(privateTurn, "acceptedFinal")
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(missingPrivateRecord); err == nil || present || delivery != nil {
		t.Fatalf("public V3 without the host-private V5 record acquired hydration authority: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	mismatchedCoverage := base
	mismatchedCoverage.ProjectedThread = contracts.CloneMap(projected)
	mismatchedCoverage.ProjectedThread["turns"].([]any)[0].(map[string]any)["acceptedFinalView"].(map[string]any)["coverageStatus"] = "complete"
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(mismatchedCoverage); err == nil || present || delivery != nil {
		t.Fatalf("accepted final public view with variant-detached coverage produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}

	revokedIndex := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	revoked := base
	revoked.Projector = NewTrustedPublicProjectorWithPrimaryCAS(
		revokedIndex, nil, nil, fixture.casReader(t, fixture.securityContext),
	)
	if delivery, present, err := BuildLatestAcceptedFinalHydrationV1(revoked); err == nil || present || delivery != nil {
		t.Fatalf("revoked authority produced hydration: present=%t delivery=%#v err=%v", present, delivery, err)
	}
}

func TestLatestDurableAcceptedFinalManifestRejectsNonExactFrontierAndPartialMarkersV1(t *testing.T) {
	authority := newProjectionTestAuthority(95)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-hydration-frontier", "turn-hydration-frontier", "case-hydration-frontier",
		"snapshot-hydration-frontier", 2,
	))
	events := sequencedAcceptedFinalHydrationEvents(t, fixture)

	gap := make([]map[string]any, len(events))
	for index, event := range events {
		gap[index] = contracts.CloneMap(event)
		gap[index]["seq"] = float64(index + 2)
	}
	if manifest, present, latest, err := latestDurableAcceptedFinalManifestV1(fixture.securityContext.ThreadID, gap); err == nil || present || manifest != nil || latest != 0 {
		t.Fatalf("non-exact full replay frontier was accepted: present=%t latest=%d manifest=%#v err=%v", present, latest, manifest, err)
	}

	partial := make([]map[string]any, 0, len(events)+1)
	for _, event := range events {
		partial = append(partial, contracts.CloneMap(event))
	}
	partial = append(partial, map[string]any{
		"kind": "turn_completed", "threadId": fixture.securityContext.ThreadID,
		"turnId": fixture.securityContext.TurnID, "seq": float64(len(events) + 1),
		"acceptedFinalDigest": fixture.privateRecord.AcceptedFinal.RecordDigest,
	})
	if manifest, present, latest, err := latestDurableAcceptedFinalManifestV1(fixture.securityContext.ThreadID, partial); err == nil || present || manifest != nil || latest != 0 {
		t.Fatalf("partial accepted-final marker was accepted: present=%t latest=%d manifest=%#v err=%v", present, latest, manifest, err)
	}
}

func TestBuildLatestAcceptedFinalHydrationLeavesOrdinarySnapshotUnchanged(t *testing.T) {
	thread := map[string]any{
		"id": "thread-ordinary-hydration",
		"turns": []any{map[string]any{
			"id": "turn-ordinary-hydration", "threadId": "thread-ordinary-hydration",
			"items": []any{},
		}},
	}
	event := map[string]any{
		"kind": "turn_started", "threadId": "thread-ordinary-hydration",
		"turnId": "turn-ordinary-hydration", "seq": float64(1),
	}
	delivery, present, err := BuildLatestAcceptedFinalHydrationV1(AcceptedFinalHydrationInputV1{
		Context: context.Background(), RouteThreadID: "thread-ordinary-hydration",
		SnapshotLatestSeq: 1, AuthorityThread: thread, ProjectedThread: thread,
		DurableEvents: []map[string]any{event}, Projector: NewTrustedPublicProjector(nil),
	})
	if err != nil || present || delivery != nil {
		t.Fatalf("ordinary snapshot acquired accepted-final authority: present=%t delivery=%#v err=%v", present, delivery, err)
	}
}

func sequencedAcceptedFinalHydrationEvents(
	t *testing.T,
	fixture trustedProjectionFixture,
) []map[string]any {
	return sequencedAcceptedFinalHydrationEventsFrom(t, fixture, 1)
}

func sequencedAcceptedFinalHydrationEventsFrom(
	t *testing.T,
	fixture trustedProjectionFixture,
	firstSeq int,
) []map[string]any {
	t.Helper()
	events := make([]map[string]any, 0, len(fixture.plan.Events))
	for index, planned := range fixture.plan.Events {
		event := contracts.CloneMap(planned.Draft)
		event["seq"] = float64(firstSeq + index)
		events = append(events, event)
	}
	if err := domainevent.ValidateAcceptedFinalDeliveryEventsV2(events); err != nil {
		t.Fatalf("accepted-final hydration fixture is invalid: %v", err)
	}
	return events
}

func sequencedHistoricalAcceptedFinalHydrationEventsV1(
	t *testing.T,
	fixture trustedProjectionFixture,
	firstSeq int,
) []map[string]any {
	t.Helper()
	events := sequencedAcceptedFinalHydrationEventsFrom(t, fixture, firstSeq)
	privateAssistant := contracts.CloneMap(fixture.plan.Completion.AssistantItem)
	events[0]["item"] = privateAssistant
	events[0]["itemId"] = contracts.StringField(privateAssistant, "id")
	delete(events[0], "publicationPayloadDigest")
	events[0]["publicationPayloadDigest"] = appturn.AcceptedFinalPublicationPayloadDigest(events[0])
	if err := domainevidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1(events); err != nil {
		t.Fatalf("historical accepted-final hydration fixture is invalid: %v", err)
	}
	return events
}

func cloneAcceptedFinalHydrationEventsV1(events []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, contracts.CloneMap(event))
	}
	return cloned
}
