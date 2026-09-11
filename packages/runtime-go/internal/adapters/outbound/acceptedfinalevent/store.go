package acceptedfinalevent

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
)

type Dependencies struct {
	Owner                      sync.Locker
	EventLog                   *eventlog.Store
	ReadThreadOwnerLocked      func(string) (map[string]any, error)
	NormalizeThreadOwnerLocked func(string, map[string]any) (map[string]any, error)
	NextSeqOwnerLocked         func(string) (int, error)
	AppendEventsOwnerLocked    func(string, []map[string]any, bool) error
	SetHighestOwnerLocked      func(string, int)
	ValidThreadID              func(string) bool
	PublishBatchOwnerLocked    func(string, domainevent.AcceptedFinalDeliveryBatchV2)
}

type reservationTokenV1 struct {
	generation uint64
}

type reservationV1 struct {
	threadID   string
	commitID   string
	generation uint64
	token      *reservationTokenV1
	durable    bool
	committed  bool
}

type reservationContextKeyV1 struct{}

// Store owns the complete accepted-final event delivery state machine. Owner
// is deliberately supplied by the durable event adapter so this store and all
// ordinary event writers serialize on one mutex.
type Store struct {
	deps         Dependencies
	reservations map[string]*reservationV1
	generation   uint64
}

func NewStore(deps Dependencies) (*Store, error) {
	if deps.Owner == nil || deps.EventLog == nil || deps.ReadThreadOwnerLocked == nil ||
		deps.NormalizeThreadOwnerLocked == nil || deps.NextSeqOwnerLocked == nil ||
		deps.AppendEventsOwnerLocked == nil || deps.SetHighestOwnerLocked == nil ||
		deps.ValidThreadID == nil || deps.PublishBatchOwnerLocked == nil {
		return nil, errors.New("accepted final event delivery dependencies are unavailable")
	}
	return &Store{deps: deps, reservations: map[string]*reservationV1{}}, nil
}

// PublicationReservedOwnerLocked is only valid while Dependencies.Owner is
// held. It lets ordinary writers fail closed without acquiring a second lock.
func (store *Store) PublicationReservedOwnerLocked(threadID string) bool {
	return store != nil && store.reservations != nil && store.reservations[strings.TrimSpace(threadID)] != nil
}

// Stage persists a complete manifest atomically but deliberately does not
// expose it to live subscribers before host-issued durable readback.
func (store *Store) Stage(ctx context.Context, drafts []map[string]any) ([]map[string]any, error) {
	if store == nil {
		return nil, errors.New("accepted final event delivery is unavailable")
	}
	store.deps.Owner.Lock()
	defer store.deps.Owner.Unlock()

	threadID, commitID, err := store.manifestIdentity(drafts)
	if err != nil {
		return nil, err
	}
	reserved := store.PublicationReservedOwnerLocked(threadID)
	if reserved {
		if _, err := store.reservationForContextOwnerLocked(ctx, threadID, commitID); err != nil {
			return nil, err
		}
	}
	publicationStore := store.publicationStoreOwnerLocked()
	publicationStore.PersistAtomic = func(actualThreadID string, events []map[string]any) error {
		if actualThreadID != threadID {
			return errors.New("accepted final event staging thread changed")
		}
		var reservation *reservationV1
		if reserved {
			var reservationErr error
			reservation, reservationErr = store.reservationForContextOwnerLocked(ctx, threadID, commitID)
			if reservationErr != nil {
				return reservationErr
			}
		}
		persistErr := store.deps.AppendEventsOwnerLocked(threadID, events, true)
		if reservation != nil && (persistErr == nil || eventlog.AtomicAppendCommitted(persistErr)) {
			reservation.durable = true
		}
		return persistErr
	}
	events, err := publicationStore.Stage(drafts)
	if errors.Is(err, appturn.ErrAcceptedFinalEventThreadNotFound) {
		err = os.ErrNotExist
	}
	if err != nil || !reserved {
		return events, err
	}
	reservation, err := store.reservationForContextOwnerLocked(ctx, threadID, commitID)
	if err != nil {
		return nil, err
	}
	reservation.durable = true
	return events, nil
}

// WithReservation prevents every ordinary same-thread writer from acquiring
// a sequence between durable staging and one-batch live publication. The
// opaque token is carried only in the derived context.
func (store *Store) WithReservation(
	ctx context.Context,
	threadID, commitID string,
	work acceptedfinaleventport.ReservationWork,
) (err error) {
	threadID = strings.TrimSpace(threadID)
	commitID = strings.TrimSpace(commitID)
	if store == nil || ctx == nil || work == nil || threadID == "" || !store.deps.ValidThreadID(threadID) ||
		!domainsecurity.IsSHA256Hex(commitID) || ctx.Err() != nil {
		return errors.New("accepted final event reservation identity is invalid")
	}

	store.deps.Owner.Lock()
	reservation := store.reservations[threadID]
	if reservation != nil && (reservation.commitID != commitID || reservation.token != nil || reservation.committed) {
		store.deps.Owner.Unlock()
		return acceptedfinaleventport.ErrPublicationReserved
	}
	store.generation++
	token := &reservationTokenV1{generation: store.generation}
	if reservation == nil {
		reservation = &reservationV1{threadID: threadID, commitID: commitID}
		store.reservations[threadID] = reservation
	}
	reservation.generation = token.generation
	reservation.token = token
	reservation.committed = false
	store.deps.Owner.Unlock()

	reservedContext := context.WithValue(ctx, reservationContextKeyV1{}, token)
	defer func() {
		panicValue := recover()
		store.deps.Owner.Lock()
		current := store.reservations[threadID]
		committed := current != nil && current.token == token && current.committed
		switch {
		case committed:
			delete(store.reservations, threadID)
		case current != nil && current.token == token && (panicValue != nil || current.durable):
			// Preserve the durable suffix reservation while allowing the same
			// commit to retry with a new opaque token.
			current.token = nil
		case current != nil && current.token == token:
			delete(store.reservations, threadID)
		}
		store.deps.Owner.Unlock()
		if panicValue != nil {
			panic(panicValue)
		}
		if err == nil && !committed {
			err = errors.New("accepted final event reservation ended without publication")
		}
	}()
	err = work(reservedContext)
	return err
}

// VerifyReservedTail is the pre-signing durable proof for a staged accepted
// final and requires the matching reservation context.
func (store *Store) VerifyReservedTail(ctx context.Context, expected []map[string]any) error {
	if store == nil {
		return errors.New("accepted final event delivery is unavailable")
	}
	threadID, commitID, err := store.manifestIdentity(expected)
	if err != nil {
		return err
	}
	store.deps.Owner.Lock()
	defer store.deps.Owner.Unlock()
	reservation, err := store.reservationForContextOwnerLocked(ctx, threadID, commitID)
	if err != nil {
		return err
	}
	loaded, err := store.deps.EventLog.LoadSince(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return errors.Join(err, errors.New("accepted final reserved readback is not replayable"))
	}
	if err := appturn.ValidateAcceptedFinalDurableReadbackV1(loaded.Events, expected, true); err != nil {
		return err
	}
	reservation.durable = true
	return nil
}

// VerifyCommittedManifest proves an exact unique durable commit group after
// activation; an older group need not remain the physical log tail.
func (store *Store) VerifyCommittedManifest(ctx context.Context, expected []map[string]any) error {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return errors.New("accepted final committed readback context is invalid")
	}
	threadID, _, err := store.manifestIdentity(expected)
	if err != nil {
		return err
	}
	store.deps.Owner.Lock()
	defer store.deps.Owner.Unlock()
	loaded, err := store.deps.EventLog.LoadSince(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return errors.Join(err, errors.New("accepted final committed readback is not replayable"))
	}
	return appturn.ValidateAcceptedFinalDurableReadbackV1(loaded.Events, expected, false)
}

// ActivateAndPublish performs every fallible delivery check before projection
// activation, then commits publication markers and the live batch without an
// error path while the same-thread reservation remains installed.
func (store *Store) ActivateAndPublish(
	ctx context.Context,
	events []map[string]any,
	deliverySeal domainevent.AcceptedFinalDeliverySealV1,
	activate func() error,
) error {
	if store == nil || activate == nil || ctx == nil || ctx.Err() != nil {
		return errors.New("accepted final event activation is unavailable")
	}
	threadID, commitID, err := store.manifestIdentity(events)
	if err != nil {
		return err
	}
	store.deps.Owner.Lock()
	defer store.deps.Owner.Unlock()
	reservation, err := store.reservationForContextOwnerLocked(ctx, threadID, commitID)
	if err != nil {
		return err
	}
	publicationStore := store.publicationStoreOwnerLocked()
	prepared, err := publicationStore.PrepareDelivery(events, deliverySeal)
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
	reservation.committed = true
	return nil
}

func (store *Store) reservationForContextOwnerLocked(
	ctx context.Context,
	threadID, commitID string,
) (*reservationV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("accepted final event reservation context is invalid")
	}
	reservation := store.reservations[strings.TrimSpace(threadID)]
	token, _ := ctx.Value(reservationContextKeyV1{}).(*reservationTokenV1)
	if reservation == nil || token == nil || reservation.token != token ||
		reservation.threadID != strings.TrimSpace(threadID) || reservation.commitID != strings.TrimSpace(commitID) ||
		reservation.generation != token.generation || reservation.committed {
		return nil, acceptedfinaleventport.ErrPublicationReserved
	}
	return reservation, nil
}

func (store *Store) manifestIdentity(events []map[string]any) (string, string, error) {
	if len(events) == 0 {
		return "", "", errors.New("accepted final event reservation manifest is empty")
	}
	threadID := strings.TrimSpace(contracts.StringField(events[0], "threadId"))
	commitID := strings.TrimSpace(contracts.StringField(events[0], "publicationCommitId"))
	if threadID == "" || !store.deps.ValidThreadID(threadID) || !domainsecurity.IsSHA256Hex(commitID) {
		return "", "", errors.New("accepted final event reservation manifest identity is invalid")
	}
	for _, event := range events {
		if strings.TrimSpace(contracts.StringField(event, "threadId")) != threadID ||
			strings.TrimSpace(contracts.StringField(event, "publicationCommitId")) != commitID {
			return "", "", errors.New("accepted final event reservation manifest binding is inconsistent")
		}
	}
	return threadID, commitID, nil
}

func (store *Store) publicationStoreOwnerLocked() appturn.AcceptedFinalEventStore {
	return appturn.AcceptedFinalEventStore{
		ReadThread: store.deps.ReadThreadOwnerLocked, NormalizeThread: store.deps.NormalizeThreadOwnerLocked,
		NextSeq: store.deps.NextSeqOwnerLocked,
		PersistAtomic: func(threadID string, events []map[string]any) error {
			return store.deps.AppendEventsOwnerLocked(threadID, events, true)
		},
		SetHighest: store.deps.SetHighestOwnerLocked, ValidThreadID: store.deps.ValidThreadID,
		HighestCommittedTail: store.deps.EventLog.HighestSeqAfterCommittedTail,
		PublicationMarked: func(id string) bool {
			return store.deps.EventLog.PublicationMarked(eventlog.PublicationNamespaceAcceptedFinal, id)
		},
		MarkPublication: func(id string) {
			store.deps.EventLog.MarkPublication(eventlog.PublicationNamespaceAcceptedFinal, id)
		},
		PublishBatch: store.deps.PublishBatchOwnerLocked,
	}
}

var _ acceptedfinaleventport.Delivery = (*Store)(nil)
