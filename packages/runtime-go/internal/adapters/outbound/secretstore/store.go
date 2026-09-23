package secretstore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"path/filepath"
	"sync"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

type Store struct {
	path        string
	masterKeys  masterKeyProvider
	authorizer  portsecretstore.ConsumerAuthorizer
	persistence persistenceBackend
	random      io.Reader

	mu     sync.Mutex
	closed bool
}

// Options carries main-private startup authority. It is never derived from a
// public request or persisted settings.
type Options struct {
	// DevelopmentFileAuthority explicitly selects the existing protected-local
	// fallback. The runtime admits it only in compiled source-development mode.
	DevelopmentFileAuthority     bool
	DarwinKeychainDBPath         string
	DarwinKeychainBindingDigest  string
	DarwinKeychainSecurityDigest string
	// DarwinKeychainAuthorityStorePath is the final canonical store identity.
	// Semantic planning may write the same marker through a temporary stage path.
	DarwinKeychainAuthorityStorePath string
}

func (options Options) empty() bool {
	return options.DarwinKeychainDBPath == "" &&
		options.DarwinKeychainBindingDigest == "" &&
		options.DarwinKeychainSecurityDigest == "" &&
		options.DarwinKeychainAuthorityStorePath == ""
}

var _ portsecretstore.RegistryStore = (*Store)(nil)

func New(path string, authorizer portsecretstore.ConsumerAuthorizer) (*Store, error) {
	return NewWithOptions(path, authorizer, Options{})
}

func NewWithOptions(path string, authorizer portsecretstore.ConsumerAuthorizer, options Options) (*Store, error) {
	provider, err := defaultMasterKeyProvider(path, options)
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return newStore(path, provider, authorizer)
}

func newStore(path string, provider masterKeyProvider, authorizer portsecretstore.ConsumerAuthorizer) (*Store, error) {
	return newStoreWithDependencies(path, provider, authorizer, filePersistence{}, rand.Reader)
}

func newStoreWithDependencies(
	path string,
	provider masterKeyProvider,
	authorizer portsecretstore.ConsumerAuthorizer,
	persistence persistenceBackend,
	random io.Reader,
) (*Store, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || provider == nil || persistence == nil || random == nil {
		return nil, portsecretstore.ErrInvalidRequest
	}
	return &Store{
		path:        path,
		masterKeys:  provider,
		authorizer:  authorizer,
		persistence: persistence,
		random:      random,
	}, nil
}

func (store *Store) Close() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.closed = true
	return nil
}

func (store *Store) Put(ctx context.Context, rawPurpose portsecretstore.Purpose, secret []byte) (portsecretstore.CredentialRef, error) {
	candidate, err := store.PreparePut(ctx, rawPurpose, secret)
	if err != nil {
		return "", err
	}
	defer candidate.Abort()
	if err := candidate.Commit(ctx); err != nil {
		return "", err
	}
	return candidate.CredentialRef(), nil
}

func (store *Store) PreparePut(
	ctx context.Context,
	rawPurpose portsecretstore.Purpose,
	secret []byte,
) (portsecretstore.PreparedCandidate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.requireOpen(); err != nil {
		return nil, err
	}
	purpose, err := normalizeExactPurpose(rawPurpose)
	if err != nil || len(secret) == 0 || len(secret) > 1<<20 {
		return nil, portsecretstore.ErrInvalidRequest
	}
	document, err := store.persistence.Load(store.path)
	if err != nil {
		return nil, portsecretstore.ErrPersistence
	}
	key, err := store.authenticateDocument(ctx, document)
	if err != nil {
		return nil, err
	}
	if key == nil {
		key, err = loadMasterKey(ctx, store.masterKeys)
		if err != nil {
			return nil, err
		}
	}
	defer clearBytes(key)
	ref, err := store.newCredentialRef(document)
	if err != nil {
		return nil, portsecretstore.ErrCryptographicFailure
	}
	envelope, err := encryptCredential(store.random, key, ref, purpose, secret)
	if err != nil {
		return nil, portsecretstore.ErrCryptographicFailure
	}
	return &preparedCandidate{
		store: store,
		ref:   ref,
		record: persistentRecord{
			Purpose: string(purpose), Lifecycle: lifecycleActive, Envelope: envelope,
		},
	}, nil
}

type preparedCandidate struct {
	store  *Store
	ref    portsecretstore.CredentialRef
	record persistentRecord

	mu        sync.Mutex
	committed bool
	aborted   bool
}

var _ portsecretstore.PreparedCandidate = (*preparedCandidate)(nil)

func (candidate *preparedCandidate) CredentialRef() portsecretstore.CredentialRef {
	if candidate == nil {
		return ""
	}
	return candidate.ref
}

func (candidate *preparedCandidate) Commit(ctx context.Context) error {
	if candidate == nil || candidate.store == nil {
		return portsecretstore.ErrInvalidRequest
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if candidate.aborted {
		return portsecretstore.ErrConflict
	}
	if candidate.committed {
		return nil
	}
	if err := candidate.store.commitPreparedCandidate(ctx, candidate.ref, candidate.record); err != nil {
		return err
	}
	candidate.committed = true
	return nil
}

func (candidate *preparedCandidate) Abort() {
	if candidate == nil {
		return
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if candidate.committed {
		return
	}
	candidate.aborted = true
	candidate.record = persistentRecord{}
}

func (store *Store) commitPreparedCandidate(
	ctx context.Context,
	ref portsecretstore.CredentialRef,
	record persistentRecord,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.requireOpen(); err != nil {
		return err
	}
	if portsecretstore.ValidateCredentialRef(ref) != nil || record.Purpose == "" ||
		record.Lifecycle != lifecycleActive || record.Envelope.Version != envelopeVersion {
		return portsecretstore.ErrInvalidRequest
	}
	document, err := store.persistence.Load(store.path)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	key, err := store.authenticateDocument(ctx, document)
	clearBytes(key)
	if err != nil {
		return err
	}
	if existing, ok := document.Records[string(ref)]; ok {
		if existing == record {
			return nil
		}
		return portsecretstore.ErrConflict
	}
	document.Records[string(ref)] = record
	if err := store.persistence.Commit(store.path, document); err != nil {
		return portsecretstore.ErrPersistence
	}
	return nil
}

func (store *Store) GetForAuthorizedConsumer(ctx context.Context, request portsecretstore.AccessRequest) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.requireOpen(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, portsecretstore.ErrInvalidRequest
	}
	if store.authorizer == nil {
		return nil, portsecretstore.ErrUnauthorized
	}
	if err := store.authorizer.AuthorizeCredentialAccess(ctx, request); err != nil {
		return nil, portsecretstore.ErrUnauthorized
	}
	document, err := store.persistence.Load(store.path)
	if err != nil {
		return nil, portsecretstore.ErrPersistence
	}
	record, ok := document.Records[string(request.CredentialRef)]
	if !ok {
		return nil, portsecretstore.ErrNotFound
	}
	if record.Purpose != string(request.Purpose) {
		return nil, portsecretstore.ErrUnauthorized
	}
	if record.Lifecycle == lifecycleTombstoned {
		return nil, portsecretstore.ErrTombstoned
	}
	key, err := loadMasterKey(ctx, store.masterKeys)
	if err != nil {
		return nil, err
	}
	defer clearBytes(key)
	plaintext, err := decryptCredential(key, request.CredentialRef, request.Purpose, record.Envelope)
	if err != nil {
		return nil, portsecretstore.ErrCryptographicFailure
	}
	return plaintext, nil
}

func (store *Store) Replace(
	ctx context.Context,
	ref portsecretstore.CredentialRef,
	rawPurpose portsecretstore.Purpose,
	mutation portsecretstore.CredentialMutation,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.requireOpen(); err != nil {
		return err
	}
	purpose, err := normalizeExactPurpose(rawPurpose)
	if err != nil || portsecretstore.ValidateCredentialRef(ref) != nil || mutation.Validate() != nil {
		return portsecretstore.ErrInvalidRequest
	}
	if mutation.Kind() == portsecretstore.MutationKeep {
		return nil
	}
	if mutation.Kind() != portsecretstore.MutationSet {
		return portsecretstore.ErrInvalidRequest
	}
	secret := mutation.Secret()
	defer clearBytes(secret)
	document, err := store.persistence.Load(store.path)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	record, ok := document.Records[string(ref)]
	if !ok {
		return portsecretstore.ErrNotFound
	}
	if record.Purpose != string(purpose) || record.Lifecycle != lifecycleActive {
		return portsecretstore.ErrConflict
	}
	key, err := store.authenticateDocument(ctx, document)
	if err != nil {
		return err
	}
	defer clearBytes(key)
	envelope, err := encryptCredential(store.random, key, ref, purpose, secret)
	if err != nil {
		return portsecretstore.ErrCryptographicFailure
	}
	record.Envelope = envelope
	document.Records[string(ref)] = record
	if err := store.persistence.Commit(store.path, document); err != nil {
		return portsecretstore.ErrPersistence
	}
	return nil
}

func (store *Store) Tombstone(ctx context.Context, ref portsecretstore.CredentialRef, rawPurpose portsecretstore.Purpose) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.requireOpen(); err != nil {
		return err
	}
	purpose, err := normalizeExactPurpose(rawPurpose)
	if err != nil || portsecretstore.ValidateCredentialRef(ref) != nil {
		return portsecretstore.ErrInvalidRequest
	}
	document, err := store.persistence.Load(store.path)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	record, ok := document.Records[string(ref)]
	if !ok {
		return portsecretstore.ErrNotFound
	}
	if record.Purpose != string(purpose) {
		return portsecretstore.ErrConflict
	}
	if record.Lifecycle == lifecycleTombstoned {
		return nil
	}
	key, err := store.authenticateDocument(ctx, document)
	clearBytes(key)
	if err != nil {
		return err
	}
	record.Lifecycle = lifecycleTombstoned
	document.Records[string(ref)] = record
	if err := store.persistence.Commit(store.path, document); err != nil {
		return portsecretstore.ErrPersistence
	}
	return nil
}

func (store *Store) ExplicitDelete(
	ctx context.Context,
	ref portsecretstore.CredentialRef,
	rawPurpose portsecretstore.Purpose,
	intent portsecretstore.CredentialMutation,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.requireOpen(); err != nil {
		return err
	}
	purpose, err := normalizeExactPurpose(rawPurpose)
	if err != nil || portsecretstore.ValidateCredentialRef(ref) != nil || intent.Validate() != nil || intent.Kind() != portsecretstore.MutationExplicitDelete {
		return portsecretstore.ErrInvalidRequest
	}
	document, err := store.persistence.Load(store.path)
	if err != nil {
		return portsecretstore.ErrPersistence
	}
	record, ok := document.Records[string(ref)]
	if !ok {
		return portsecretstore.ErrNotFound
	}
	if record.Purpose != string(purpose) || record.Lifecycle != lifecycleTombstoned {
		return portsecretstore.ErrConflict
	}
	key, err := store.authenticateDocument(ctx, document)
	clearBytes(key)
	if err != nil {
		return err
	}
	delete(document.Records, string(ref))
	if err := store.persistence.Commit(store.path, document); err != nil {
		return portsecretstore.ErrPersistence
	}
	return nil
}

func (store *Store) requireOpen() error {
	if store.closed {
		return portsecretstore.ErrClosed
	}
	return nil
}

func (store *Store) authenticateDocument(ctx context.Context, document persistentDocument) ([]byte, error) {
	if len(document.Records) == 0 {
		return nil, nil
	}
	key, err := loadMasterKey(ctx, store.masterKeys)
	if err != nil {
		return nil, err
	}
	for rawRef, record := range document.Records {
		ref := portsecretstore.CredentialRef(rawRef)
		purpose := portsecretstore.Purpose(record.Purpose)
		plaintext, decryptErr := decryptCredential(key, ref, purpose, record.Envelope)
		clearBytes(plaintext)
		if decryptErr != nil {
			clearBytes(key)
			return nil, portsecretstore.ErrCryptographicFailure
		}
	}
	return key, nil
}

func (store *Store) newCredentialRef(document persistentDocument) (portsecretstore.CredentialRef, error) {
	for attempt := 0; attempt < 4; attempt++ {
		random := make([]byte, 32)
		if _, err := io.ReadFull(store.random, random); err != nil {
			clearBytes(random)
			return "", portsecretstore.ErrCryptographicFailure
		}
		encoded := base64.RawURLEncoding.EncodeToString(random)
		clearBytes(random)
		ref := portsecretstore.CredentialRef("cred_" + encoded)
		if _, exists := document.Records[string(ref)]; !exists {
			return ref, nil
		}
	}
	return "", portsecretstore.ErrCryptographicFailure
}

func normalizeExactPurpose(raw portsecretstore.Purpose) (portsecretstore.Purpose, error) {
	normalized, err := portsecretstore.NormalizePurpose(string(raw))
	if err != nil || normalized != raw {
		return "", portsecretstore.ErrInvalidRequest
	}
	return normalized, nil
}
