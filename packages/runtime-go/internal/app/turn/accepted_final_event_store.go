package turn

import (
	"errors"
	"reflect"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
)

var ErrAcceptedFinalEventThreadNotFound = errors.New("accepted final event thread was not found")

// AcceptedFinalEventStore owns the stage/readback-to-live transition while a
// concrete adapter holds its per-thread persistence lock.
type AcceptedFinalEventStore struct {
	ReadThread           func(string) (map[string]any, error)
	NormalizeThread      func(string, map[string]any) (map[string]any, error)
	NextSeq              func(string) (int, error)
	PersistAtomic        func(string, []map[string]any) error
	SetHighest           func(string, int)
	ValidThreadID        func(string) bool
	HighestCommittedTail func(string, []map[string]any) (int, error)
	PublicationMarked    func(string) bool
	MarkPublication      func(string)
	PublishBatch         func(string, domainevent.AcceptedFinalDeliveryBatchV2)
}

// preparedAcceptedFinalDeliveryV1 contains every fallible result needed for
// live publication. Its fields are private so callers can only pass through a
// value produced and revalidated by AcceptedFinalEventStore.
type preparedAcceptedFinalDeliveryV1 struct {
	threadID            string
	publicationCommitID string
	highest             int
	publicationIDs      []string
	batch               domainevent.AcceptedFinalDeliveryBatchV2
	alreadyPublished    bool
}

func (store AcceptedFinalEventStore) Stage(drafts []map[string]any) ([]map[string]any, error) {
	if len(drafts) == 0 || store.ReadThread == nil || store.NormalizeThread == nil || store.NextSeq == nil ||
		store.PersistAtomic == nil || store.SetHighest == nil || store.ValidThreadID == nil {
		return nil, errors.New("accepted final event bundle store is unavailable")
	}
	threadID := strings.TrimSpace(contracts.StringField(drafts[0], "threadId"))
	if threadID == "" || !store.ValidThreadID(threadID) {
		return nil, errors.New("accepted final event bundle identity is invalid")
	}
	thread, err := store.ReadThread(threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, ErrAcceptedFinalEventThreadNotFound
	}
	if strings.TrimSpace(contracts.StringField(thread, "id")) != threadID {
		return nil, errors.New("accepted final event thread identity is invalid")
	}
	thread, err = store.NormalizeThread(threadID, thread)
	if err != nil {
		return nil, err
	}
	nextSeq, err := store.NextSeq(threadID)
	if err != nil {
		return nil, err
	}
	events, err := PrepareAcceptedFinalEventBundle(thread, drafts, nextSeq)
	if err != nil {
		return nil, err
	}
	if err := store.PersistAtomic(threadID, events); err != nil {
		return nil, err
	}
	store.SetHighest(threadID, nextSeq+len(events)-1)
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, contracts.CloneMap(event))
	}
	return out, nil
}

func (store AcceptedFinalEventStore) PrepareDelivery(
	events []map[string]any,
	deliverySeal domainevent.AcceptedFinalDeliverySealV1,
) (preparedAcceptedFinalDeliveryV1, error) {
	if len(events) == 0 || store.HighestCommittedTail == nil || store.PublicationMarked == nil ||
		store.MarkPublication == nil || store.SetHighest == nil || store.PublishBatch == nil {
		return preparedAcceptedFinalDeliveryV1{}, errors.New("accepted final live event store is unavailable")
	}
	threadID := strings.TrimSpace(contracts.StringField(events[0], "threadId"))
	batch, err := domainevent.NewAcceptedFinalDeliveryBatchV2(events, deliverySeal)
	if err != nil || threadID == "" {
		return preparedAcceptedFinalDeliveryV1{}, errors.Join(err, errors.New("accepted final live event identity is invalid"))
	}
	highest, err := store.HighestCommittedTail(threadID, events)
	if err != nil {
		return preparedAcceptedFinalDeliveryV1{}, err
	}
	alreadyPublished := 0
	publicationIDs := make([]string, 0, len(events))
	for _, event := range events {
		publicationID := contracts.StringField(event, "publicationEventId")
		if contracts.StringField(event, "threadId") != threadID ||
			contracts.StringField(event, "publicationCommitId") != batch.PublicationCommitID || publicationID == "" {
			return preparedAcceptedFinalDeliveryV1{}, errors.New("accepted final live event manifest is invalid")
		}
		publicationIDs = append(publicationIDs, publicationID)
		if store.PublicationMarked(publicationID) {
			alreadyPublished++
		}
	}
	if alreadyPublished != 0 && alreadyPublished != len(events) {
		return preparedAcceptedFinalDeliveryV1{}, errors.New("accepted final live event bundle is partially published")
	}
	return preparedAcceptedFinalDeliveryV1{
		threadID: threadID, publicationCommitID: batch.PublicationCommitID, highest: highest,
		publicationIDs: publicationIDs, batch: batch, alreadyPublished: alreadyPublished == len(events),
	}, nil
}

// ValidatePreparedDelivery repeats every fallible check immediately before
// projection activation. The reservation owner calls this while ordinary
// same-thread writers remain excluded.
func (store AcceptedFinalEventStore) ValidatePreparedDelivery(prepared preparedAcceptedFinalDeliveryV1) error {
	current, err := store.PrepareDelivery(prepared.batch.Events, prepared.batch.PublicationAuthority)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, prepared) {
		return errors.New("accepted final prepared delivery changed before activation")
	}
	return nil
}

// CommitPreparedDelivery performs no I/O and has no error path. Callers must
// validate the prepared value under their publication reservation immediately
// before invoking it.
func (store AcceptedFinalEventStore) CommitPreparedDelivery(prepared preparedAcceptedFinalDeliveryV1) {
	if prepared.alreadyPublished {
		return
	}
	store.SetHighest(prepared.threadID, prepared.highest)
	for _, publicationID := range prepared.publicationIDs {
		store.MarkPublication(publicationID)
	}
	store.PublishBatch(prepared.threadID, prepared.batch)
}
