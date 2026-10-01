package secretstore

import (
	"context"
	"errors"
	"sync"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

// LegacyKeychainProfile is a validated inventory of ciphertext whose master
// key remains in the old macOS Keychain. It never reads that key. New writes
// use a separate file-authority Store, while old references can only be
// retired by the Registry's already-fenced cleanup transaction.
type LegacyKeychainProfile struct {
	path    string
	records map[portsecretstore.CredentialRef]persistentRecord
}

func (profile *LegacyKeychainProfile) ActivePurposes() map[portsecretstore.CredentialRef]portsecretstore.Purpose {
	out := make(map[portsecretstore.CredentialRef]portsecretstore.Purpose)
	if profile == nil {
		return out
	}
	for ref, record := range profile.records {
		if record.Lifecycle == lifecycleActive {
			out[ref] = portsecretstore.Purpose(record.Purpose)
		}
	}
	return out
}

func (profile *LegacyKeychainProfile) ReentryStore(current *Store) portsecretstore.RegistryStore {
	return &legacyKeychainReentryStore{profile: profile, current: current}
}

type legacyKeychainReentryStore struct {
	profile *LegacyKeychainProfile
	current *Store
	mu      sync.Mutex
}

func (store *legacyKeychainReentryStore) PreparePut(ctx context.Context, purpose portsecretstore.Purpose, secret []byte) (portsecretstore.PreparedCandidate, error) {
	return store.current.PreparePut(ctx, purpose, secret)
}

func (store *legacyKeychainReentryStore) GetForAuthorizedConsumer(ctx context.Context, request portsecretstore.AccessRequest) ([]byte, error) {
	if _, legacy := store.profile.records[request.CredentialRef]; !legacy {
		return store.current.GetForAuthorizedConsumer(ctx, request)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if request.Validate() != nil || ctx == nil || ctx.Err() != nil {
		return nil, portsecretstore.ErrInvalidRequest
	}
	if store.current.authorizer == nil {
		return nil, portsecretstore.ErrUnauthorized
	}
	if err := store.current.authorizer.AuthorizeCredentialAccess(ctx, request); err != nil {
		return nil, portsecretstore.ErrUnauthorized
	}
	record, err := store.legacyRecord(request.CredentialRef, request.Purpose)
	if err != nil {
		return nil, err
	}
	if record.Lifecycle != lifecycleActive {
		return nil, portsecretstore.ErrTombstoned
	}
	return nil, errors.Join(portsecretstore.ErrLegacyReentryRequired, portsecretstore.ErrMasterKeyUnavailable)
}

func (store *legacyKeychainReentryStore) Tombstone(ctx context.Context, ref portsecretstore.CredentialRef, purpose portsecretstore.Purpose) error {
	if _, legacy := store.profile.records[ref]; !legacy {
		return store.current.Tombstone(ctx, ref, purpose)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return portsecretstore.ErrInvalidRequest
	}
	record, err := store.legacyRecord(ref, purpose)
	if err != nil {
		return err
	}
	if record.Lifecycle == lifecycleTombstoned {
		return nil
	}
	document, err := (filePersistence{}).Load(store.profile.path)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	record.Lifecycle = lifecycleTombstoned
	document.Records[string(ref)] = record
	return (filePersistence{}).Commit(store.profile.path, document)
}

func (store *legacyKeychainReentryStore) ExplicitDelete(ctx context.Context, ref portsecretstore.CredentialRef, purpose portsecretstore.Purpose, intent portsecretstore.CredentialMutation) error {
	if _, legacy := store.profile.records[ref]; !legacy {
		return store.current.ExplicitDelete(ctx, ref, purpose, intent)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || intent.Validate() != nil || intent.Kind() != portsecretstore.MutationExplicitDelete {
		return portsecretstore.ErrInvalidRequest
	}
	record, err := store.legacyRecord(ref, purpose)
	if err != nil {
		return err
	}
	if record.Lifecycle != lifecycleTombstoned {
		return portsecretstore.ErrConflict
	}
	document, err := (filePersistence{}).Load(store.profile.path)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	delete(document.Records, string(ref))
	return (filePersistence{}).Commit(store.profile.path, document)
}

func (store *legacyKeychainReentryStore) legacyRecord(ref portsecretstore.CredentialRef, purpose portsecretstore.Purpose) (persistentRecord, error) {
	if portsecretstore.ValidateCredentialRef(ref) != nil || purpose == "" {
		return persistentRecord{}, portsecretstore.ErrInvalidRequest
	}
	if err := verifyLegacyKeychainMarker(store.profile.path); err != nil {
		return persistentRecord{}, portsecretstore.ErrMasterKeyUnavailable
	}
	document, err := (filePersistence{}).Load(store.profile.path)
	if err != nil {
		return persistentRecord{}, portsecretstore.ErrPersistence
	}
	record, ok := document.Records[string(ref)]
	if !ok {
		return persistentRecord{}, portsecretstore.ErrNotFound
	}
	initial := store.profile.records[ref]
	if record.Purpose != string(purpose) || initial.Purpose != record.Purpose ||
		initial.Envelope != record.Envelope ||
		(initial.Lifecycle != lifecycleActive && initial.Lifecycle != lifecycleTombstoned) {
		return persistentRecord{}, portsecretstore.ErrConflict
	}
	if record.Lifecycle != initial.Lifecycle && !(initial.Lifecycle == lifecycleActive && record.Lifecycle == lifecycleTombstoned) {
		return persistentRecord{}, portsecretstore.ErrConflict
	}
	return record, nil
}

var _ portsecretstore.RegistryStore = (*legacyKeychainReentryStore)(nil)
