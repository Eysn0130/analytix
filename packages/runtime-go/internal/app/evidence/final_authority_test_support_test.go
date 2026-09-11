package evidence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
	"strings"
	"sync"

	appturn "analytix.local/runtime-go/internal/app/turn"
	appturnterminal "analytix.local/runtime-go/internal/app/turnterminal"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	caseterminaltest "analytix.local/runtime-go/internal/testsupport/caseturnterminal"
)

type lockedMemoryEvidenceRegistry struct {
	*memoryEvidenceRegistry
	mu sync.Mutex
}

func (registry *lockedMemoryEvidenceRegistry) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, callback func(domainevidence.EvidenceReceiptRegistry) error) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	snapshot, err := registry.current(securityContext)
	if err != nil {
		return err
	}
	return callback(snapshot)
}

func (registry *lockedMemoryEvidenceRegistry) ReplayAt(_ context.Context, securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	current, err := registry.current(securityContext)
	if err != nil || sequence > current.Sequence {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("test registry sequence is unavailable")
	}
	snapshot, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	for _, entry := range current.Entries[:sequence] {
		snapshot, err = domainevidence.ApplyEvidenceRegistryEntry(snapshot, entry)
		if err != nil {
			return domainevidence.EvidenceReceiptRegistry{}, err
		}
	}
	return snapshot, nil
}

type memoryFinalAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newMemoryFinalAuthority(seedByte byte) *memoryFinalAuthority {
	seed := bytes.Repeat([]byte{seedByte}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &memoryFinalAuthority{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (authority *memoryFinalAuthority) KeyID() string { return authority.keyID }
func (authority *memoryFinalAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *memoryFinalAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *memoryFinalAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("test final authority mismatch")
	}
	return nil
}

type memoryPrivateFinalStore struct {
	mu           sync.Mutex
	records      map[string]domainevidence.PrivateAcceptedFinalRecord
	dispositions map[string]domainevidence.AcceptedFinalDispositionRecord
	putErr       error
	putCalls     int
}

func (store *memoryPrivateFinalStore) PutIfAbsent(_ context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	if store.putErr != nil {
		return store.putErr
	}
	if domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil {
		return errors.New("test private record is invalid")
	}
	if current, ok := store.records[record.AcceptedFinal.RecordDigest]; ok && !reflect.DeepEqual(current, record) {
		return errors.New("test private record conflicts")
	}
	store.records[record.AcceptedFinal.RecordDigest] = record
	return nil
}

func (store *memoryPrivateFinalStore) Resolve(_ context.Context, digest string) (domainevidence.PrivateAcceptedFinalRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[strings.TrimSpace(digest)]
	if !ok {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("test private record is missing")
	}
	return record, nil
}

func (store *memoryPrivateFinalStore) List(context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	records := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}
	return records, nil
}

func (store *memoryPrivateFinalStore) VisitAcceptedFinals(ctx context.Context, visit func(domainevidence.PrivateAcceptedFinalRecord) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, record := range store.records {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryPrivateFinalStore) HasRecords(context.Context) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.records) > 0 || len(store.dispositions) > 0, nil
}

func (store *memoryPrivateFinalStore) PutDispositionIfAbsent(_ context.Context, record domainevidence.AcceptedFinalDispositionRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if domainevidence.ValidateAcceptedFinalDispositionRecord(record) != nil {
		return errors.New("test disposition is invalid")
	}
	if store.dispositions == nil {
		store.dispositions = map[string]domainevidence.AcceptedFinalDispositionRecord{}
	}
	if current, ok := store.dispositions[record.AcceptedFinalDigest]; ok && !reflect.DeepEqual(current, record) {
		return errors.New("test disposition conflicts")
	}
	store.dispositions[record.AcceptedFinalDigest] = record
	return nil
}

func (store *memoryPrivateFinalStore) ResolveDisposition(_ context.Context, digest string) (domainevidence.AcceptedFinalDispositionRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.dispositions[strings.TrimSpace(digest)]
	if !ok {
		return domainevidence.AcceptedFinalDispositionRecord{}, errors.New("test disposition is missing")
	}
	return record, nil
}

func (store *memoryPrivateFinalStore) ListDispositions(context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	records := make([]domainevidence.AcceptedFinalDispositionRecord, 0, len(store.dispositions))
	for _, record := range store.dispositions {
		records = append(records, record)
	}
	return records, nil
}

func (store *memoryPrivateFinalStore) VisitDispositions(ctx context.Context, visit func(domainevidence.AcceptedFinalDispositionRecord) error) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, record := range store.dispositions {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return nil
}

func newTestFinalPublicationEventIO() FinalPublicationEventIO {
	authority := &testFinalEventReadbackAuthority{
		events:    map[appturn.AcceptedFinalCompletionStore][]map[string]any{},
		active:    map[appturn.AcceptedFinalCompletionStore]string{},
		published: map[string]bool{},
	}
	return FinalPublicationEventIO{
		ReadThread: func(_ context.Context, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			if reader, ok := store.(interface {
				GetThread(string) (map[string]any, error)
			}); ok {
				return reader.GetThread(privateRecord.SecurityContext.ThreadID)
			}
			if reader, ok := store.(interface {
				FinalPublicationThread(domainsecurity.TurnSecurityContext) (map[string]any, error)
			}); ok {
				return reader.FinalPublicationThread(privateRecord.SecurityContext)
			}
			return nil, errors.New("test publication thread reader is unavailable")
		},
		ReadCASObservation: func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalCASObservationV1, error) {
			reader, ok := store.(interface {
				FinalPublicationThread(domainsecurity.TurnSecurityContext) (map[string]any, error)
			})
			if !ok {
				return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("test publication CAS reader is unavailable")
			}
			thread, err := reader.FinalPublicationThread(privateRecord.SecurityContext)
			if err != nil {
				return domainevidence.AcceptedFinalCASObservationV1{}, err
			}
			return acceptedFinalCASObservationForTest(ctx, thread, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
		},
		LoadEvents: func(_ context.Context, store appturn.AcceptedFinalCompletionStore, _ string) ([]map[string]any, error) {
			authority.mu.Lock()
			defer authority.mu.Unlock()
			out := make([]map[string]any, 0, len(authority.events[store]))
			for _, event := range authority.events[store] {
				out = append(out, contracts.CloneMap(event))
			}
			return out, nil
		},
		AppendEvents: func(_ context.Context, store appturn.AcceptedFinalCompletionStore, bundle []map[string]any) ([]map[string]any, error) {
			atomicStore, ok := store.(interface {
				RecordAcceptedFinalEventBundle([]map[string]any) ([]map[string]any, error)
			})
			if !ok {
				return nil, errors.New("test publication store lacks an atomic accepted-final event path")
			}
			recordedBundle, err := atomicStore.RecordAcceptedFinalEventBundle(bundle)
			if err != nil {
				return nil, err
			}
			authority.mu.Lock()
			for _, recorded := range recordedBundle {
				authority.events[store] = append(authority.events[store], contracts.CloneMap(recorded))
			}
			authority.mu.Unlock()
			return recordedBundle, nil
		},
		Readback: authority,
		WithEventReservation: func(
			ctx context.Context,
			store appturn.AcceptedFinalCompletionStore,
			_ string,
			commitID string,
			work AcceptedFinalEventReservationWorkV1,
		) error {
			return authority.withReservation(ctx, store, commitID, work)
		},
		ActivateAndPublishEvents: func(
			ctx context.Context,
			store appturn.AcceptedFinalCompletionStore,
			events []map[string]any,
			seal domainevent.AcceptedFinalDeliverySealV1,
			activate func() error,
		) error {
			if len(events) == 0 {
				return errors.New("test publication manifest is empty")
			}
			if err := authority.validateReservationContext(ctx, store, contracts.StringField(events[0], "publicationCommitId")); err != nil {
				return err
			}
			eventStore := authority.eventStore(store)
			prepared, err := eventStore.PrepareDelivery(events, seal)
			if err != nil {
				return err
			}
			if err := eventStore.ValidatePreparedDelivery(prepared); err != nil {
				return err
			}
			if err := activate(); err != nil {
				return err
			}
			eventStore.CommitPreparedDelivery(prepared)
			return nil
		},
	}
}

type testFinalEventReservationContextKey struct{}

type testFinalEventReservationContext struct {
	store    appturn.AcceptedFinalCompletionStore
	commitID string
}

type testFinalEventReadbackAuthority struct {
	mu        sync.Mutex
	events    map[appturn.AcceptedFinalCompletionStore][]map[string]any
	active    map[appturn.AcceptedFinalCompletionStore]string
	published map[string]bool
}

func (authority *testFinalEventReadbackAuthority) withReservation(
	ctx context.Context,
	store appturn.AcceptedFinalCompletionStore,
	commitID string,
	work AcceptedFinalEventReservationWorkV1,
) error {
	if authority == nil || ctx == nil || store == nil || work == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(commitID)) {
		return errors.New("test accepted final reservation is invalid")
	}
	authority.mu.Lock()
	if authority.active[store] != "" {
		authority.mu.Unlock()
		return errors.New("test accepted final reservation is already active")
	}
	authority.active[store] = commitID
	authority.mu.Unlock()
	defer func() {
		authority.mu.Lock()
		delete(authority.active, store)
		authority.mu.Unlock()
	}()
	return work(context.WithValue(ctx, testFinalEventReservationContextKey{}, testFinalEventReservationContext{
		store: store, commitID: commitID,
	}))
}

func (authority *testFinalEventReadbackAuthority) validateReservationContext(
	ctx context.Context,
	store appturn.AcceptedFinalCompletionStore,
	commitID string,
) error {
	reservation, _ := ctx.Value(testFinalEventReservationContextKey{}).(testFinalEventReservationContext)
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if reservation.store != store || reservation.commitID != commitID || authority.active[store] != commitID {
		return errors.New("test accepted final reservation context is invalid")
	}
	return nil
}

func (authority *testFinalEventReadbackAuthority) VerifyReservedTail(ctx context.Context, expected []map[string]any) error {
	if len(expected) == 0 {
		return errors.New("test accepted final readback is empty")
	}
	reservation, _ := ctx.Value(testFinalEventReservationContextKey{}).(testFinalEventReservationContext)
	commitID := contracts.StringField(expected[0], "publicationCommitId")
	if err := authority.validateReservationContext(ctx, reservation.store, commitID); err != nil {
		return err
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return appturn.ValidateAcceptedFinalDurableReadbackV1(authority.events[reservation.store], expected, true)
}

func (authority *testFinalEventReadbackAuthority) VerifyCommittedManifest(_ context.Context, expected []map[string]any) error {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	for _, events := range authority.events {
		if appturn.ValidateAcceptedFinalDurableReadbackV1(events, expected, false) == nil {
			return nil
		}
	}
	return errors.New("test committed accepted final readback is unavailable")
}

func (authority *testFinalEventReadbackAuthority) eventStore(
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
				return 0, errors.New("test accepted final highest sequence is invalid")
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

func newTestCasePublicationFinalizer() (CasePublicationFinalizer, *lockedMemoryEvidenceRegistry, *memoryFinalAuthority, *memoryPrivateFinalStore) {
	registry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	privateStore := &memoryPrivateFinalStore{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{}}
	authority := newMemoryFinalAuthority(1)
	return NewCasePublicationFinalizerWithAuthority(registry, registry, authority, privateStore, newTestFinalPublicationEventIO(), newTestTurnTerminalCoordinator(authority, privateStore)), registry, authority, privateStore
}

func newTestTurnTerminalCoordinator(authority *memoryFinalAuthority, privateStore *memoryPrivateFinalStore) *appturnterminal.Coordinator {
	coordinator, err := caseterminaltest.NewInMemoryCoordinatorV1(authority, privateStore)
	if err != nil {
		panic(err)
	}
	return coordinator
}
