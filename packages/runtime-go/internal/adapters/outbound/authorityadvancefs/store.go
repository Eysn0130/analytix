package authorityadvancefs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/authorityadvance"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type Store struct {
	intents     *finalauthorityadapter.SecurePrivateCAS
	settlements *finalauthorityadapter.SecurePrivateCAS
}

var _ storeport.IntentStore = (*Store)(nil)
var _ storeport.SettlementStore = (*Store)(nil)
var _ storeport.IntentInventoryStore = (*Store)(nil)
var _ storeport.SettlementInventoryStore = (*Store)(nil)

func NewStore(root string, access privatecasport.AccessAuthority) (*Store, error) {
	if root == "" || root != strings.TrimSpace(root) || !filepath.IsAbs(root) || filepath.Clean(root) != root || access == nil {
		return nil, errors.New("authority advance journal root is invalid")
	}
	intents, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "v2", "intents"), domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2, access,
	)
	if err != nil {
		return nil, err
	}
	settlements, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(root, "v2", "settlements"), domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2, access,
	)
	if err != nil {
		return nil, err
	}
	return &Store{intents: intents, settlements: settlements}, nil
}

func (store *Store) PutIntentIfAbsent(
	ctx context.Context,
	intent domainauthority.MonotonicAdvanceIntentV2,
) error {
	if store == nil || store.intents == nil {
		return errors.New("authority advance intent store is unavailable")
	}
	body, err := domainauthority.MonotonicAdvanceIntentV2Bytes(intent)
	if err != nil || len(body) == 0 || len(body) > domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2 {
		return errors.New("authority advance intent is invalid")
	}
	return putExactV2(ctx, store.intents, intent.MutationID, body, func(candidate []byte) error {
		parsed, err := domainauthority.ParseMonotonicAdvanceIntentV2(candidate)
		if err != nil || parsed.MutationID != intent.MutationID {
			return errors.New("authority advance intent readback is invalid")
		}
		return nil
	})
}

func (store *Store) ResolveIntent(
	ctx context.Context,
	mutationID string,
) (domainauthority.MonotonicAdvanceIntentV2, error) {
	if store == nil || store.intents == nil || !canonicalDigestV1(mutationID) {
		return domainauthority.MonotonicAdvanceIntentV2{}, errors.New("authority advance intent lookup is invalid")
	}
	body, err := store.intents.Read(ctx, mutationID)
	if err != nil {
		return domainauthority.MonotonicAdvanceIntentV2{}, classifyJournalReadError(err)
	}
	intent, err := domainauthority.ParseMonotonicAdvanceIntentV2(body)
	if err != nil || intent.MutationID != mutationID {
		return domainauthority.MonotonicAdvanceIntentV2{}, errors.Join(
			storeport.ErrCorrupt, errors.New("authority advance intent content address is invalid"), err,
		)
	}
	return intent, nil
}

func (store *Store) VisitIntents(
	ctx context.Context,
	visit func(domainauthority.MonotonicAdvanceIntentV2) error,
) error {
	if store == nil || store.intents == nil || ctx == nil || visit == nil {
		return storeport.ErrUnavailable
	}
	return store.intents.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		intent, err := domainauthority.ParseMonotonicAdvanceIntentV2(file.Body)
		if err != nil || intent.MutationID != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("authority advance intent inventory is corrupt"), err)
		}
		return visit(intent)
	})
}

func (store *Store) PutSettlementIfAbsent(
	ctx context.Context,
	settlement domainauthority.MonotonicAdvanceSettlementV2,
) error {
	if store == nil || store.settlements == nil {
		return errors.New("authority advance settlement store is unavailable")
	}
	body, err := domainauthority.MonotonicAdvanceSettlementV2Bytes(settlement)
	if err != nil || len(body) == 0 || len(body) > domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2 {
		return errors.New("authority advance settlement is invalid")
	}
	return putExactV2(ctx, store.settlements, settlement.MutationID, body, func(candidate []byte) error {
		parsed, err := domainauthority.ParseMonotonicAdvanceSettlementV2(candidate)
		if err != nil || parsed.MutationID != settlement.MutationID {
			return errors.New("authority advance settlement readback is invalid")
		}
		return nil
	})
}

func (store *Store) ResolveSettlement(
	ctx context.Context,
	mutationID string,
) (domainauthority.MonotonicAdvanceSettlementV2, error) {
	if store == nil || store.settlements == nil || !canonicalDigestV1(mutationID) {
		return domainauthority.MonotonicAdvanceSettlementV2{}, errors.New("authority advance settlement lookup is invalid")
	}
	body, err := store.settlements.Read(ctx, mutationID)
	if err != nil {
		return domainauthority.MonotonicAdvanceSettlementV2{}, classifyJournalReadError(err)
	}
	settlement, err := domainauthority.ParseMonotonicAdvanceSettlementV2(body)
	if err != nil || settlement.MutationID != mutationID {
		return domainauthority.MonotonicAdvanceSettlementV2{}, errors.Join(
			storeport.ErrCorrupt, errors.New("authority advance settlement content address is invalid"), err,
		)
	}
	return settlement, nil
}

func (store *Store) VisitSettlements(
	ctx context.Context,
	visit func(domainauthority.MonotonicAdvanceSettlementV2) error,
) error {
	if store == nil || store.settlements == nil || ctx == nil || visit == nil {
		return storeport.ErrUnavailable
	}
	return store.settlements.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		settlement, err := domainauthority.ParseMonotonicAdvanceSettlementV2(file.Body)
		if err != nil || settlement.MutationID != file.Digest {
			return errors.Join(storeport.ErrCorrupt, errors.New("authority advance settlement inventory is corrupt"), err)
		}
		return visit(settlement)
	})
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil || store.intents == nil || store.settlements == nil || ctx == nil {
		return false, storeport.ErrUnavailable
	}
	found := errors.New("authority advance journal record found")
	visit := func(inventory *finalauthorityadapter.SecurePrivateCAS) (bool, error) {
		err := inventory.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
		if errors.Is(err, found) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	}
	if present, err := visit(store.intents); err != nil || present {
		return present, err
	}
	if present, err := visit(store.settlements); err != nil || present {
		return present, err
	}
	return false, nil
}

func putExactV2(
	ctx context.Context,
	store *finalauthorityadapter.SecurePrivateCAS,
	key string,
	body []byte,
	validate func([]byte) error,
) error {
	if current, err := store.Read(ctx, key); err == nil {
		return validateExactRecordV2(current, body, validate)
	} else if !errors.Is(err, os.ErrNotExist) {
		return classifyJournalReadError(err)
	}
	if err := store.PutIfAbsent(ctx, key, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return errors.Join(storeport.ErrUnavailable, err)
		}
		current, readErr := store.Read(ctx, key)
		if readErr != nil {
			return errors.Join(storeport.ErrUnavailable, readErr)
		}
		return validateExactRecordV2(current, body, validate)
	}
	written, err := store.Read(ctx, key)
	if err != nil {
		return errors.Join(storeport.ErrUnavailable, err)
	}
	if !bytes.Equal(written, body) || validate(written) != nil {
		return errors.Join(storeport.ErrCorrupt, errors.New("authority advance journal write readback failed"))
	}
	return nil
}

func classifyJournalReadError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return errors.Join(storeport.ErrNotFound, err)
	}
	if errors.Is(err, finalauthorityadapter.ErrSecurePrivateCASIntegrity) {
		return errors.Join(storeport.ErrCorrupt, err)
	}
	return errors.Join(storeport.ErrUnavailable, err)
}

func validateExactRecordV2(current, expected []byte, validate func([]byte) error) error {
	if err := validate(current); err != nil {
		return errors.Join(storeport.ErrCorrupt, err)
	}
	if !bytes.Equal(current, expected) {
		return errors.Join(storeport.ErrConflict, errors.New("authority advance journal access has different bytes"))
	}
	return nil
}

func canonicalDigestV1(value string) bool {
	return value == strings.TrimSpace(value) && domainsecurity.IsSHA256Hex(value)
}
