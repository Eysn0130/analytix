package casepublication

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"

	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
	caseterminaltest "analytix.local/runtime-go/internal/testsupport/caseturnterminal"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func New(root string) (evidenceapp.CasePublicationFinalizer, error) {
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(root, "authority", "ed25519-v1.json"), false)
	if err != nil {
		return nil, err
	}
	registry, err := evidenceregistry.NewStore(filepath.Join(root, "registry"), authority)
	if err != nil {
		return nil, err
	}
	privateRoot := filepath.Join(root, "accepted-finals")
	privateAccess, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		return nil, err
	}
	privateStore, err := finalauthority.NewPrivateStore(privateRoot, privateAccess)
	if err != nil {
		return nil, err
	}
	eventAuthority := &casePublicationEventAuthority{
		events:     map[appturn.AcceptedFinalCompletionStore][]map[string]any{},
		active:     map[appturn.AcceptedFinalCompletionStore]string{},
		published:  map[string]bool{},
		deliveries: map[string]acceptedfinaleventport.Delivery{},
	}
	eventIO := evidenceapp.FinalPublicationEventIO{
		ReadThread: func(_ context.Context, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			if reader, ok := store.(interface {
				GetThread(string) (map[string]any, error)
			}); ok {
				return reader.GetThread(privateRecord.SecurityContext.ThreadID)
			}
			return nil, errors.New("test case publication thread reader is unavailable")
		},
		ReadCASObservation: func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalCASObservationV1, error) {
			if reader, ok := store.(interface {
				ReadAcceptedFinalCASObservation(context.Context, string, string) (domainevidence.AcceptedFinalCASObservationV1, error)
			}); ok {
				return reader.ReadAcceptedFinalCASObservation(ctx, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
			}
			reader, ok := store.(interface {
				GetThread(string) (map[string]any, error)
			})
			if !ok {
				return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("test case publication CAS reader is unavailable")
			}
			thread, err := reader.GetThread(privateRecord.SecurityContext.ThreadID)
			if err != nil {
				return domainevidence.AcceptedFinalCASObservationV1{}, err
			}
			return testCASObservation(thread, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
		},
		LoadEvents: func(_ context.Context, store appturn.AcceptedFinalCompletionStore, _ string) ([]map[string]any, error) {
			eventAuthority.mu.Lock()
			defer eventAuthority.mu.Unlock()
			out := make([]map[string]any, 0, len(eventAuthority.events[store]))
			for _, event := range eventAuthority.events[store] {
				out = append(out, contracts.CloneMap(event))
			}
			return out, nil
		},
		AppendEvents: func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, bundle []map[string]any) ([]map[string]any, error) {
			var recordedBundle []map[string]any
			var err error
			if deliveryStore, ok := store.(interface {
				AcceptedFinalEventDelivery() acceptedfinaleventport.Delivery
			}); ok {
				delivery := deliveryStore.AcceptedFinalEventDelivery()
				if delivery == nil {
					return nil, errors.New("test case publication delivery adapter is unavailable")
				}
				if len(bundle) > 0 {
					eventAuthority.mu.Lock()
					eventAuthority.deliveries[contracts.StringField(bundle[0], "threadId")] = delivery
					eventAuthority.mu.Unlock()
				}
				recordedBundle, err = delivery.Stage(ctx, bundle)
			} else if atomicStore, ok := store.(interface {
				RecordAcceptedFinalEventBundle([]map[string]any) ([]map[string]any, error)
			}); ok {
				recordedBundle, err = atomicStore.RecordAcceptedFinalEventBundle(bundle)
			} else {
				return nil, errors.New("test case publication store lacks an atomic accepted-final event path")
			}
			if err != nil {
				return nil, err
			}
			eventAuthority.mu.Lock()
			for _, recorded := range recordedBundle {
				eventAuthority.events[store] = append(eventAuthority.events[store], contracts.CloneMap(recorded))
			}
			eventAuthority.mu.Unlock()
			return recordedBundle, nil
		},
		Readback: eventAuthority,
		WithEventReservation: func(
			ctx context.Context,
			store appturn.AcceptedFinalCompletionStore,
			threadID string,
			commitID string,
			work evidenceapp.AcceptedFinalEventReservationWorkV1,
		) error {
			if deliveryStore, ok := store.(interface {
				AcceptedFinalEventDelivery() acceptedfinaleventport.Delivery
			}); ok {
				delivery := deliveryStore.AcceptedFinalEventDelivery()
				if delivery == nil {
					return errors.New("test case publication delivery adapter is unavailable")
				}
				return delivery.WithReservation(ctx, threadID, commitID, work)
			}
			return eventAuthority.withReservation(ctx, store, commitID, work)
		},
		ActivateAndPublishEvents: func(
			ctx context.Context,
			store appturn.AcceptedFinalCompletionStore,
			events []map[string]any,
			seal domainevent.AcceptedFinalDeliverySealV1,
			activate func() error,
		) error {
			if deliveryStore, ok := store.(interface {
				AcceptedFinalEventDelivery() acceptedfinaleventport.Delivery
			}); ok {
				delivery := deliveryStore.AcceptedFinalEventDelivery()
				if delivery == nil {
					return errors.New("test case publication delivery adapter is unavailable")
				}
				return delivery.ActivateAndPublish(ctx, events, seal, activate)
			}
			if len(events) == 0 {
				return errors.New("test case publication manifest is empty")
			}
			if err := eventAuthority.validateReservationContext(
				ctx, store, contracts.StringField(events[0], "publicationCommitId"),
			); err != nil {
				return err
			}
			publicationStore := eventAuthority.eventStore(store)
			prepared, err := publicationStore.PrepareDelivery(events, seal)
			if err != nil {
				return err
			}
			if err := publicationStore.ValidatePreparedDelivery(prepared); err != nil {
				return err
			}
			if err := activate(); err != nil {
				return err
			}
			publicationStore.CommitPreparedDelivery(prepared)
			return nil
		},
	}
	terminalCoordinator, err := caseterminaltest.NewInMemoryCoordinatorV1(authority, privateStore)
	if err != nil {
		return nil, err
	}
	return evidenceapp.NewCasePublicationFinalizerWithAuthority(registry, registry, authority, privateStore, eventIO, terminalCoordinator), nil
}

type casePublicationReservationContextKey struct{}

type casePublicationReservationContext struct {
	store    appturn.AcceptedFinalCompletionStore
	commitID string
}

type casePublicationEventAuthority struct {
	mu         sync.Mutex
	events     map[appturn.AcceptedFinalCompletionStore][]map[string]any
	active     map[appturn.AcceptedFinalCompletionStore]string
	published  map[string]bool
	deliveries map[string]acceptedfinaleventport.Delivery
}

func (authority *casePublicationEventAuthority) withReservation(
	ctx context.Context,
	store appturn.AcceptedFinalCompletionStore,
	commitID string,
	work evidenceapp.AcceptedFinalEventReservationWorkV1,
) error {
	if authority == nil || ctx == nil || store == nil || work == nil || !domainsecurity.IsSHA256Hex(commitID) {
		return errors.New("test case publication reservation is invalid")
	}
	authority.mu.Lock()
	if authority.active[store] != "" {
		authority.mu.Unlock()
		return errors.New("test case publication reservation is already active")
	}
	authority.active[store] = commitID
	authority.mu.Unlock()
	defer func() {
		authority.mu.Lock()
		delete(authority.active, store)
		authority.mu.Unlock()
	}()
	return work(context.WithValue(ctx, casePublicationReservationContextKey{}, casePublicationReservationContext{
		store: store, commitID: commitID,
	}))
}

func (authority *casePublicationEventAuthority) validateReservationContext(
	ctx context.Context,
	store appturn.AcceptedFinalCompletionStore,
	commitID string,
) error {
	reservation, _ := ctx.Value(casePublicationReservationContextKey{}).(casePublicationReservationContext)
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if reservation.store != store || reservation.commitID != commitID || authority.active[store] != commitID {
		return errors.New("test case publication reservation context is invalid")
	}
	return nil
}

func (authority *casePublicationEventAuthority) VerifyReservedTail(ctx context.Context, expected []map[string]any) error {
	if len(expected) == 0 {
		return errors.New("test case publication readback is empty")
	}
	authority.mu.Lock()
	delivery := authority.deliveries[contracts.StringField(expected[0], "threadId")]
	authority.mu.Unlock()
	if delivery != nil {
		return delivery.VerifyReservedTail(ctx, expected)
	}
	reservation, _ := ctx.Value(casePublicationReservationContextKey{}).(casePublicationReservationContext)
	if err := authority.validateReservationContext(
		ctx, reservation.store, contracts.StringField(expected[0], "publicationCommitId"),
	); err != nil {
		return err
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return appturn.ValidateAcceptedFinalDurableReadbackV1(authority.events[reservation.store], expected, true)
}

func (authority *casePublicationEventAuthority) VerifyCommittedManifest(ctx context.Context, expected []map[string]any) error {
	if len(expected) > 0 {
		authority.mu.Lock()
		delivery := authority.deliveries[contracts.StringField(expected[0], "threadId")]
		authority.mu.Unlock()
		if delivery != nil {
			return delivery.VerifyCommittedManifest(ctx, expected)
		}
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	for _, events := range authority.events {
		if appturn.ValidateAcceptedFinalDurableReadbackV1(events, expected, false) == nil {
			return nil
		}
	}
	return errors.New("test committed case publication readback is unavailable")
}

func (authority *casePublicationEventAuthority) eventStore(
	completionStore appturn.AcceptedFinalCompletionStore,
) appturn.AcceptedFinalEventStore {
	return appturn.AcceptedFinalEventStore{
		HighestCommittedTail: func(_ string, expected []map[string]any) (int, error) {
			authority.mu.Lock()
			defer authority.mu.Unlock()
			if err := appturn.ValidateAcceptedFinalDurableReadbackV1(authority.events[completionStore], expected, true); err != nil {
				return 0, err
			}
			seq, ok := contracts.NumericSeq(expected[len(expected)-1]["seq"])
			if !ok {
				return 0, errors.New("test case publication highest sequence is invalid")
			}
			return seq, nil
		},
		PublicationMarked: func(id string) bool {
			authority.mu.Lock()
			defer authority.mu.Unlock()
			return authority.published[id]
		},
		MarkPublication: func(id string) {
			authority.mu.Lock()
			authority.published[id] = true
			authority.mu.Unlock()
		},
		SetHighest:   func(string, int) {},
		PublishBatch: func(string, domainevent.AcceptedFinalDeliveryBatchV2) {},
	}
}

func testCASObservation(thread map[string]any, threadID, turnID string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	var matched map[string]any
	turns, _ := thread["turns"].([]any)
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if textValue(turn["id"]) == turnID {
			if matched != nil {
				return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("test CAS contains duplicate turn identity")
			}
			matched = turn
		}
	}
	if matched == nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("test CAS turn is missing")
	}
	frozen, err := domainsecurity.ParseTurnSecurityContext(matched["securityContext"])
	if err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, err
	}
	current := frozen
	if thread["securityState"] != nil {
		current, err = domainsecurity.ParseTurnSecurityContext(thread["securityState"])
		if err != nil {
			return domainevidence.AcceptedFinalCASObservationV1{}, err
		}
	}
	threadBody, _ := json.Marshal(thread)
	turnBody, _ := json.Marshal(matched)
	observation := domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: threadID, TurnID: turnID, Status: textValue(matched["status"]), FrozenContext: frozen, CurrentContext: current,
		ThreadFileSHA256: domainsecurity.SHA256Hex(threadBody), TurnProjectionSHA256: domainsecurity.SHA256Hex(turnBody),
	}
	if matched["acceptedFinal"] != nil {
		winner, err := domainevidence.ParseAcceptedFinalRecord(matched["acceptedFinal"])
		if err != nil {
			return domainevidence.AcceptedFinalCASObservationV1{}, err
		}
		observation.HasWinner = true
		observation.Winner = winner
	}
	return domainevidence.NewAcceptedFinalCASObservationV1(observation)
}

func textValue(value any) string {
	text, _ := value.(string)
	return text
}
