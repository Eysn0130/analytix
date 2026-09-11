package secretstore

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

type fixedMasterKeyProvider struct {
	key []byte
}

func (p fixedMasterKeyProvider) LoadOrCreate(context.Context) ([]byte, error) {
	return bytes.Clone(p.key), nil
}

type allowCredentialConsumer struct{}

func (allowCredentialConsumer) AuthorizeCredentialAccess(context.Context, portsecretstore.AccessRequest) error {
	return nil
}

type credentialAuthorizerFunc func(context.Context, portsecretstore.AccessRequest) error

func (authorize credentialAuthorizerFunc) AuthorizeCredentialAccess(ctx context.Context, request portsecretstore.AccessRequest) error {
	return authorize(ctx, request)
}

type countingMasterKeyProvider struct {
	mu       sync.Mutex
	key      []byte
	err      error
	accesses int
}

func (provider *countingMasterKeyProvider) LoadOrCreate(context.Context) ([]byte, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.accesses++
	return bytes.Clone(provider.key), provider.err
}

func (provider *countingMasterKeyProvider) accessCount() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.accesses
}

type countingPersistence struct {
	mu      sync.Mutex
	loads   int
	commits int
	loaded  persistentDocument
}

func (persistence *countingPersistence) Load(string) (persistentDocument, error) {
	persistence.mu.Lock()
	defer persistence.mu.Unlock()
	persistence.loads++
	return persistence.loaded, nil
}

func (persistence *countingPersistence) Commit(string, persistentDocument) error {
	persistence.mu.Lock()
	defer persistence.mu.Unlock()
	persistence.commits++
	return nil
}

func (persistence *countingPersistence) loadCount() int {
	persistence.mu.Lock()
	defer persistence.mu.Unlock()
	return persistence.loads
}

type failingCommitPersistence struct {
	delegate persistenceBackend
}

func (persistence failingCommitPersistence) Load(path string) (persistentDocument, error) {
	return persistence.delegate.Load(path)
}

func (failingCommitPersistence) Commit(string, persistentDocument) error {
	return errors.New("synthetic path and secret must not escape")
}

func TestStorePersistsVersionedAEADEnvelopeWithPurposeBoundAAD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	key := bytes.Repeat([]byte{0x5a}, 32)
	marker := []byte("synthetic-provider-secret-marker")
	purpose := portsecretstore.Purpose("provider-api-key")
	storePath := filepath.Join(t.TempDir(), "credentials.enc.json")

	store, err := newStore(storePath, fixedMasterKeyProvider{key: key}, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("newStore() error = %v", err)
	}
	ref, err := store.Put(ctx, purpose, marker)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if ref == "" {
		t.Fatal("Put() returned an empty credential reference")
	}
	if err := portsecretstore.ValidateCredentialRef(ref); err != nil {
		t.Fatalf("Put() returned an invalid opaque credential reference: %v", err)
	}
	if strings.Contains(string(ref), string(marker)) || strings.Contains(string(ref), string(purpose)) {
		t.Fatal("opaque credential reference contains secret or purpose material")
	}

	committed, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if bytes.Contains(committed, marker) {
		t.Fatal("committed store contains the synthetic plaintext marker")
	}

	var document struct {
		Version int `json:"version"`
		Records map[string]struct {
			Purpose   string `json:"purpose"`
			Lifecycle string `json:"lifecycle"`
			Envelope  struct {
				Version    int    `json:"version"`
				Nonce      string `json:"nonce"`
				Ciphertext string `json:"ciphertext"`
			} `json:"envelope"`
		} `json:"records"`
	}
	if err := json.Unmarshal(committed, &document); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if document.Version != persistentFormatVersion {
		t.Fatalf("document version = %d, want %d", document.Version, persistentFormatVersion)
	}
	record, ok := document.Records[string(ref)]
	if !ok {
		t.Fatal("committed store is missing the opaque credential reference")
	}
	if record.Purpose != string(purpose) {
		t.Fatalf("record purpose = %q, want normalized purpose", record.Purpose)
	}
	if record.Lifecycle != lifecycleActive {
		t.Fatalf("record lifecycle = %q, want %q", record.Lifecycle, lifecycleActive)
	}
	if record.Envelope.Version != envelopeVersion {
		t.Fatalf("envelope version = %d, want %d", record.Envelope.Version, envelopeVersion)
	}

	nonce, err := base64.RawStdEncoding.DecodeString(record.Envelope.Nonce)
	if err != nil {
		t.Fatalf("DecodeString(nonce) error = %v", err)
	}
	if len(nonce) != 12 {
		t.Fatalf("nonce length = %d, want 12", len(nonce))
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(record.Envelope.Ciphertext)
	if err != nil {
		t.Fatalf("DecodeString(ciphertext) error = %v", err)
	}
	if len(ciphertext) <= len(marker) {
		t.Fatalf("ciphertext length = %d, want plaintext plus authentication tag", len(ciphertext))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM() error = %v", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, envelopeAdditionalData(envelopeVersion, ref, purpose))
	if err != nil {
		t.Fatalf("Open(correct AAD) error = %v", err)
	}
	if !bytes.Equal(plaintext, marker) {
		t.Fatal("decrypted plaintext does not match the synthetic marker")
	}
	if _, err := aead.Open(nil, nonce, ciphertext, envelopeAdditionalData(envelopeVersion, ref, portsecretstore.Purpose("oauth-token"))); err == nil {
		t.Fatal("Open(wrong purpose AAD) unexpectedly succeeded")
	}
}

func TestStoreRoundTripUsesUniqueNoncesAndAuthorizedReads(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	key := bytes.Repeat([]byte{0x39}, masterKeySize)
	purpose := portsecretstore.Purpose("provider-api-key")
	marker := []byte("synthetic-round-trip-marker")
	provider := &countingMasterKeyProvider{key: key}
	store, err := newStore(filepath.Join(t.TempDir(), "credentials.enc.json"), provider, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("newStore() error = %v", err)
	}
	firstRef, err := store.Put(ctx, purpose, marker)
	if err != nil {
		t.Fatalf("first Put() error = %v", err)
	}
	secondRef, err := store.Put(ctx, purpose, marker)
	if err != nil {
		t.Fatalf("second Put() error = %v", err)
	}
	if firstRef == secondRef {
		t.Fatal("two puts returned the same opaque credential reference")
	}

	document, err := (filePersistence{}).Load(store.path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.Records[string(firstRef)].Envelope.Nonce == document.Records[string(secondRef)].Envelope.Nonce {
		t.Fatal("two encryptions reused an AES-GCM nonce")
	}
	plaintext, err := store.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
		CredentialRef: firstRef,
		Purpose:       purpose,
		Consumer:      "runtime-provider",
	})
	if err != nil {
		t.Fatalf("GetForAuthorizedConsumer() error = %v", err)
	}
	defer clearBytes(plaintext)
	if !bytes.Equal(plaintext, marker) {
		t.Fatal("authorized read did not return the synthetic marker")
	}
}

func TestUnauthorizedReadStopsBeforePersistenceAndMasterKeyAccess(t *testing.T) {
	t.Parallel()

	request := portsecretstore.AccessRequest{
		CredentialRef: portsecretstore.CredentialRef("cred_" + strings.Repeat("A", 43)),
		Purpose:       portsecretstore.Purpose("provider-api-key"),
		Consumer:      "runtime-provider",
	}
	for _, testCase := range []struct {
		name       string
		authorizer portsecretstore.ConsumerAuthorizer
	}{
		{name: "missing authorizer"},
		{
			name: "denied authorizer",
			authorizer: credentialAuthorizerFunc(func(context.Context, portsecretstore.AccessRequest) error {
				return errors.New("synthetic-provider-secret-marker")
			}),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			provider := &countingMasterKeyProvider{key: bytes.Repeat([]byte{0x41}, masterKeySize)}
			persistence := &countingPersistence{loaded: newPersistentDocument()}
			store, err := newStoreWithDependencies(
				filepath.Join(t.TempDir(), "credentials.enc.json"),
				provider,
				testCase.authorizer,
				persistence,
				bytes.NewReader(bytes.Repeat([]byte{0x77}, 128)),
			)
			if err != nil {
				t.Fatalf("newStoreWithDependencies() error = %v", err)
			}
			_, err = store.GetForAuthorizedConsumer(context.Background(), request)
			if !errors.Is(err, portsecretstore.ErrUnauthorized) {
				t.Fatalf("GetForAuthorizedConsumer() error = %v, want unauthorized", err)
			}
			if persistence.loadCount() != 0 {
				t.Fatalf("persistence loads = %d, want 0", persistence.loadCount())
			}
			if provider.accessCount() != 0 {
				t.Fatalf("master-key accesses = %d, want 0", provider.accessCount())
			}
			assertRedactedError(t, err, "synthetic-provider-secret-marker", string(request.CredentialRef), string(request.Purpose))
		})
	}
}

func TestStoreFailsClosedForAADTamperUnknownVersionAndWrongKey(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		mutate  func(t *testing.T, path string, ref portsecretstore.CredentialRef, document persistentDocument)
		key     []byte
		purpose portsecretstore.Purpose
		want    error
	}{
		{
			name: "purpose AAD mismatch",
			mutate: func(t *testing.T, path string, ref portsecretstore.CredentialRef, document persistentDocument) {
				record := document.Records[string(ref)]
				record.Purpose = "oauth-token"
				document.Records[string(ref)] = record
				if err := (filePersistence{}).Commit(path, document); err != nil {
					t.Fatalf("Commit() error = %v", err)
				}
			},
			purpose: portsecretstore.Purpose("oauth-token"),
			want:    portsecretstore.ErrCryptographicFailure,
		},
		{
			name: "ciphertext tamper",
			mutate: func(t *testing.T, path string, ref portsecretstore.CredentialRef, document persistentDocument) {
				record := document.Records[string(ref)]
				ciphertext, err := base64.RawStdEncoding.DecodeString(record.Envelope.Ciphertext)
				if err != nil {
					t.Fatalf("DecodeString() error = %v", err)
				}
				ciphertext[len(ciphertext)-1] ^= 0xff
				record.Envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(ciphertext)
				clearBytes(ciphertext)
				document.Records[string(ref)] = record
				if err := (filePersistence{}).Commit(path, document); err != nil {
					t.Fatalf("Commit() error = %v", err)
				}
			},
			purpose: portsecretstore.Purpose("provider-api-key"),
			want:    portsecretstore.ErrCryptographicFailure,
		},
		{
			name: "unknown envelope version",
			mutate: func(t *testing.T, path string, ref portsecretstore.CredentialRef, document persistentDocument) {
				record := document.Records[string(ref)]
				record.Envelope.Version = envelopeVersion + 1
				document.Records[string(ref)] = record
				content, err := json.Marshal(document)
				if err != nil {
					t.Fatalf("Marshal() error = %v", err)
				}
				if err := writePrivateFileAtomically(path, content, nil); err != nil {
					t.Fatalf("writePrivateFileAtomically() error = %v", err)
				}
			},
			purpose: portsecretstore.Purpose("provider-api-key"),
			want:    portsecretstore.ErrPersistence,
		},
		{
			name:    "wrong master key",
			mutate:  func(*testing.T, string, portsecretstore.CredentialRef, persistentDocument) {},
			key:     bytes.Repeat([]byte{0x28}, masterKeySize),
			purpose: portsecretstore.Purpose("provider-api-key"),
			want:    portsecretstore.ErrCryptographicFailure,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "credentials.enc.json")
			originalKey := bytes.Repeat([]byte{0x27}, masterKeySize)
			store, err := newStore(path, fixedMasterKeyProvider{key: originalKey}, allowCredentialConsumer{})
			if err != nil {
				t.Fatalf("newStore() error = %v", err)
			}
			marker := []byte("synthetic-tamper-marker")
			ref, err := store.Put(ctx, portsecretstore.Purpose("provider-api-key"), marker)
			if err != nil {
				t.Fatalf("Put() error = %v", err)
			}
			document, err := (filePersistence{}).Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			testCase.mutate(t, path, ref, document)
			readKey := testCase.key
			if readKey == nil {
				readKey = originalKey
			}
			reader, err := newStore(path, fixedMasterKeyProvider{key: readKey}, allowCredentialConsumer{})
			if err != nil {
				t.Fatalf("newStore(reader) error = %v", err)
			}
			_, err = reader.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
				CredentialRef: ref,
				Purpose:       testCase.purpose,
				Consumer:      "runtime-provider",
			})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("GetForAuthorizedConsumer() error = %v, want %v", err, testCase.want)
			}
			assertRedactedError(t, err, string(marker), string(ref), string(testCase.purpose), path)
		})
	}
}

func TestStoreFailsClosedWhenCredentialReferenceAADChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "credentials.enc.json")
	key := bytes.Repeat([]byte{0x36}, masterKeySize)
	purpose := portsecretstore.Purpose("provider-api-key")
	store, err := newStore(path, fixedMasterKeyProvider{key: key}, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("newStore() error = %v", err)
	}
	ref, err := store.Put(ctx, purpose, []byte("synthetic-reference-aad-marker"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	document, err := (filePersistence{}).Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	replacementRef := portsecretstore.CredentialRef("cred_" + strings.Repeat("Z", 43))
	document.Records[string(replacementRef)] = document.Records[string(ref)]
	delete(document.Records, string(ref))
	if err := (filePersistence{}).Commit(path, document); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	_, err = store.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
		CredentialRef: replacementRef,
		Purpose:       purpose,
		Consumer:      "runtime-provider",
	})
	if !errors.Is(err, portsecretstore.ErrCryptographicFailure) {
		t.Fatalf("GetForAuthorizedConsumer() error = %v, want cryptographic failure", err)
	}
}

func TestStoreMutationAndDurableDeletionSemantics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "credentials.enc.json")
	purpose := portsecretstore.Purpose("provider-api-key")
	provider := &countingMasterKeyProvider{key: bytes.Repeat([]byte{0x62}, masterKeySize)}
	store, err := newStore(path, provider, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("newStore() error = %v", err)
	}
	ref, err := store.Put(ctx, purpose, []byte("synthetic-original-marker"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	originalBytes := mustReadFile(t, path)
	if err := store.Replace(ctx, ref, purpose, portsecretstore.KeepCredential()); err != nil {
		t.Fatalf("Replace(keep) error = %v", err)
	}
	if afterKeep := mustReadFile(t, path); !bytes.Equal(originalBytes, afterKeep) {
		t.Fatal("keep mutation changed committed bytes")
	}
	if err := store.Replace(ctx, ref, purpose, portsecretstore.CredentialMutation{}); !errors.Is(err, portsecretstore.ErrInvalidRequest) {
		t.Fatalf("Replace(zero mutation) error = %v, want invalid request", err)
	}
	if _, err := portsecretstore.SetCredential(nil); !errors.Is(err, portsecretstore.ErrInvalidRequest) {
		t.Fatalf("SetCredential(empty) error = %v, want invalid request", err)
	}
	if err := store.ExplicitDelete(ctx, ref, purpose, portsecretstore.ExplicitlyDeleteCredential()); !errors.Is(err, portsecretstore.ErrConflict) {
		t.Fatalf("ExplicitDelete(live) error = %v, want conflict", err)
	}
	if afterRejectedDelete := mustReadFile(t, path); !bytes.Equal(originalBytes, afterRejectedDelete) {
		t.Fatal("rejected live delete changed committed bytes")
	}

	replacement, err := portsecretstore.SetCredential([]byte("synthetic-replacement-marker"))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	if err := store.Replace(ctx, ref, purpose, replacement); err != nil {
		t.Fatalf("Replace(set) error = %v", err)
	}
	accessesBeforeTombstone := provider.accessCount()
	if err := store.Tombstone(ctx, ref, purpose); err != nil {
		t.Fatalf("Tombstone() error = %v", err)
	}
	accessesAfterTombstone := provider.accessCount()
	if accessesAfterTombstone != accessesBeforeTombstone+1 {
		t.Fatal("Tombstone() did not authenticate the committed envelopes exactly once")
	}
	document, err := (filePersistence{}).Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.Records[string(ref)].Lifecycle != lifecycleTombstoned {
		t.Fatal("tombstone lifecycle was not committed durably")
	}
	_, err = store.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
		CredentialRef: ref,
		Purpose:       purpose,
		Consumer:      "runtime-provider",
	})
	if !errors.Is(err, portsecretstore.ErrTombstoned) {
		t.Fatalf("GetForAuthorizedConsumer(tombstoned) error = %v, want tombstoned", err)
	}
	if provider.accessCount() != accessesAfterTombstone {
		t.Fatal("tombstoned read unexpectedly accessed the master key")
	}
	if err := store.Replace(ctx, ref, purpose, replacement); !errors.Is(err, portsecretstore.ErrConflict) {
		t.Fatalf("Replace(tombstoned) error = %v, want conflict", err)
	}
	if err := store.ExplicitDelete(ctx, ref, purpose, portsecretstore.ExplicitlyDeleteCredential()); err != nil {
		t.Fatalf("ExplicitDelete() error = %v", err)
	}
	document, err = (filePersistence{}).Load(path)
	if err != nil {
		t.Fatalf("Load(after delete) error = %v", err)
	}
	if _, exists := document.Records[string(ref)]; exists {
		t.Fatal("explicit delete retained the tombstoned record")
	}
}

func TestStoreFailuresPreservePriorCommittedBytes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	purpose := portsecretstore.Purpose("provider-api-key")
	for _, testCase := range []struct {
		name      string
		provider  masterKeyProvider
		backend   func() persistenceBackend
		wantError error
	}{
		{
			name:     "atomic persistence failure",
			provider: fixedMasterKeyProvider{key: bytes.Repeat([]byte{0x74}, masterKeySize)},
			backend: func() persistenceBackend {
				return failingCommitPersistence{delegate: filePersistence{}}
			},
			wantError: portsecretstore.ErrPersistence,
		},
		{
			name: "unreadable master key",
			provider: &countingMasterKeyProvider{
				err: errors.New("synthetic key bytes and path must not escape"),
			},
			backend:   func() persistenceBackend { return filePersistence{} },
			wantError: portsecretstore.ErrMasterKeyUnavailable,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.enc.json")
			key := bytes.Repeat([]byte{0x74}, masterKeySize)
			writer, err := newStore(path, fixedMasterKeyProvider{key: key}, allowCredentialConsumer{})
			if err != nil {
				t.Fatalf("newStore(writer) error = %v", err)
			}
			ref, err := writer.Put(ctx, purpose, []byte("synthetic-preserved-marker"))
			if err != nil {
				t.Fatalf("Put() error = %v", err)
			}
			before := mustReadFile(t, path)
			store, err := newStoreWithDependencies(path, testCase.provider, allowCredentialConsumer{}, testCase.backend(), bytes.NewReader(bytes.Repeat([]byte{0x81}, 128)))
			if err != nil {
				t.Fatalf("newStoreWithDependencies() error = %v", err)
			}
			mutation, err := portsecretstore.SetCredential([]byte("synthetic-candidate-marker"))
			if err != nil {
				t.Fatalf("SetCredential() error = %v", err)
			}
			err = store.Replace(ctx, ref, purpose, mutation)
			if !errors.Is(err, testCase.wantError) {
				t.Fatalf("Replace() error = %v, want %v", err, testCase.wantError)
			}
			after := mustReadFile(t, path)
			if !bytes.Equal(before, after) {
				t.Fatal("failed replace changed prior committed bytes")
			}
			assertRedactedError(t, err, "synthetic-preserved-marker", "synthetic-candidate-marker", string(ref), path)
		})
	}
}

func TestStoreMutationsAuthenticateAllCommittedEnvelopesBeforeWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	purpose := portsecretstore.Purpose("provider-api-key")
	key := bytes.Repeat([]byte{0xc1}, masterKeySize)
	wrongKey := bytes.Repeat([]byte{0xc2}, masterKeySize)
	for _, testCase := range []struct {
		name    string
		prepare func(t *testing.T, store *Store, path string, firstRef, secondRef portsecretstore.CredentialRef)
		mutate  func(t *testing.T, store *Store, path string, firstRef, secondRef portsecretstore.CredentialRef) error
	}{
		{
			name: "put rejects wrong key for existing envelope",
			mutate: func(t *testing.T, _ *Store, path string, _, _ portsecretstore.CredentialRef) error {
				store, err := newStore(path, fixedMasterKeyProvider{key: wrongKey}, allowCredentialConsumer{})
				if err != nil {
					t.Fatalf("newStore(wrong key) error = %v", err)
				}
				_, err = store.Put(ctx, purpose, []byte("synthetic-new-put-marker"))
				return err
			},
		},
		{
			name: "replace rejects tampered sibling envelope",
			prepare: func(t *testing.T, _ *Store, path string, firstRef, _ portsecretstore.CredentialRef) {
				tamperPersistentEnvelope(t, path, firstRef)
			},
			mutate: func(t *testing.T, store *Store, _ string, _, secondRef portsecretstore.CredentialRef) error {
				mutation, err := portsecretstore.SetCredential([]byte("synthetic-new-replace-marker"))
				if err != nil {
					t.Fatalf("SetCredential() error = %v", err)
				}
				return store.Replace(ctx, secondRef, purpose, mutation)
			},
		},
		{
			name: "tombstone rejects tampered target envelope",
			prepare: func(t *testing.T, _ *Store, path string, firstRef, _ portsecretstore.CredentialRef) {
				tamperPersistentEnvelope(t, path, firstRef)
			},
			mutate: func(_ *testing.T, store *Store, _ string, firstRef, _ portsecretstore.CredentialRef) error {
				return store.Tombstone(ctx, firstRef, purpose)
			},
		},
		{
			name: "explicit delete rejects tampered tombstoned envelope",
			prepare: func(t *testing.T, store *Store, path string, firstRef, _ portsecretstore.CredentialRef) {
				if err := store.Tombstone(ctx, firstRef, purpose); err != nil {
					t.Fatalf("Tombstone(setup) error = %v", err)
				}
				tamperPersistentEnvelope(t, path, firstRef)
			},
			mutate: func(_ *testing.T, store *Store, _ string, firstRef, _ portsecretstore.CredentialRef) error {
				return store.ExplicitDelete(ctx, firstRef, purpose, portsecretstore.ExplicitlyDeleteCredential())
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.enc.json")
			store, err := newStore(path, fixedMasterKeyProvider{key: key}, allowCredentialConsumer{})
			if err != nil {
				t.Fatalf("newStore() error = %v", err)
			}
			firstRef, err := store.Put(ctx, purpose, []byte("synthetic-first-committed-marker"))
			if err != nil {
				t.Fatalf("first Put() error = %v", err)
			}
			secondRef, err := store.Put(ctx, purpose, []byte("synthetic-second-committed-marker"))
			if err != nil {
				t.Fatalf("second Put() error = %v", err)
			}
			if testCase.prepare != nil {
				testCase.prepare(t, store, path, firstRef, secondRef)
			}
			beforeRejectedCommit := mustReadFile(t, path)
			err = testCase.mutate(t, store, path, firstRef, secondRef)
			if !errors.Is(err, portsecretstore.ErrCryptographicFailure) {
				t.Fatalf("mutation error = %v, want cryptographic failure", err)
			}
			afterRejectedCommit := mustReadFile(t, path)
			if !bytes.Equal(beforeRejectedCommit, afterRejectedCommit) {
				t.Fatal("rejected mutation changed prior committed bytes")
			}
			assertRedactedError(t, err, string(firstRef), string(secondRef), path, "synthetic")
		})
	}
}

func tamperPersistentEnvelope(t *testing.T, path string, ref portsecretstore.CredentialRef) {
	t.Helper()
	document, err := (filePersistence{}).Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	record := document.Records[string(ref)]
	ciphertext, err := base64.RawStdEncoding.DecodeString(record.Envelope.Ciphertext)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	record.Envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(ciphertext)
	clearBytes(ciphertext)
	document.Records[string(ref)] = record
	if err := (filePersistence{}).Commit(path, document); err != nil {
		t.Fatalf("Commit(tamper) error = %v", err)
	}
}

func TestStoreCloseFailsClosed(t *testing.T) {
	t.Parallel()

	store, err := newStore(
		filepath.Join(t.TempDir(), "credentials.enc.json"),
		fixedMasterKeyProvider{key: bytes.Repeat([]byte{0x18}, masterKeySize)},
		allowCredentialConsumer{},
	)
	if err != nil {
		t.Fatalf("newStore() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := store.Put(context.Background(), portsecretstore.Purpose("provider-api-key"), []byte("synthetic-marker")); !errors.Is(err, portsecretstore.ErrClosed) {
		t.Fatalf("Put(after Close) error = %v, want closed", err)
	}
}

func TestPreparedCandidateIsNotDurableUntilExplicitCommitAndIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "credentials.enc.json")
	purpose := portsecretstore.Purpose("provider-api-key")
	marker := []byte("synthetic-prepared-candidate-marker")
	store, err := newStore(path, fixedMasterKeyProvider{key: bytes.Repeat([]byte{0xd4}, masterKeySize)}, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("newStore() error = %v", err)
	}
	candidate, err := store.PreparePut(ctx, purpose, marker)
	if err != nil {
		t.Fatalf("PreparePut() error = %v", err)
	}
	if err := portsecretstore.ValidateCredentialRef(candidate.CredentialRef()); err != nil {
		t.Fatalf("prepared reference is invalid: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("PreparePut() committed encrypted bytes before the Registry could name the candidate")
	}
	if err := candidate.Commit(ctx); err != nil {
		t.Fatalf("candidate Commit() error = %v", err)
	}
	if err := candidate.Commit(ctx); err != nil {
		t.Fatalf("candidate repeat Commit() error = %v", err)
	}
	candidate.Abort()
	plaintext, err := store.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
		CredentialRef: candidate.CredentialRef(), Purpose: purpose, Consumer: "provider-registry-manager",
	})
	if err != nil {
		t.Fatalf("GetForAuthorizedConsumer() error = %v", err)
	}
	defer clearBytes(plaintext)
	if !bytes.Equal(plaintext, marker) {
		t.Fatal("committed prepared candidate readback differs from the synthetic marker")
	}
	restarted, err := newStore(path, fixedMasterKeyProvider{key: bytes.Repeat([]byte{0xd4}, masterKeySize)}, allowCredentialConsumer{})
	if err != nil {
		t.Fatalf("newStore(restarted) error = %v", err)
	}
	restartedPlaintext, err := restarted.GetForAuthorizedConsumer(ctx, portsecretstore.AccessRequest{
		CredentialRef: candidate.CredentialRef(), Purpose: purpose, Consumer: "provider-registry-manager",
	})
	if err != nil {
		t.Fatalf("restarted GetForAuthorizedConsumer() error = %v", err)
	}
	defer clearBytes(restartedPlaintext)
	if !bytes.Equal(restartedPlaintext, marker) {
		t.Fatal("fresh Secret Store instance did not recover the prepared candidate")
	}
	committed := mustReadFile(t, path)
	if bytes.Contains(committed, marker) {
		t.Fatal("prepared candidate committed plaintext bytes")
	}
}

func TestPreparedCandidateAbortAndCommitFailureLeaveNoUntrackedSecret(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	purpose := portsecretstore.Purpose("provider-api-key")
	for _, testCase := range []struct {
		name        string
		persistence persistenceBackend
		want        error
	}{
		{name: "abort before commit", persistence: filePersistence{}},
		{name: "atomic commit failure", persistence: failingCommitPersistence{delegate: filePersistence{}}, want: portsecretstore.ErrPersistence},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.enc.json")
			store, err := newStoreWithDependencies(
				path,
				fixedMasterKeyProvider{key: bytes.Repeat([]byte{0xe4}, masterKeySize)},
				allowCredentialConsumer{}, testCase.persistence,
				bytes.NewReader(bytes.Repeat([]byte{0x71}, 128)),
			)
			if err != nil {
				t.Fatalf("newStoreWithDependencies() error = %v", err)
			}
			candidate, err := store.PreparePut(ctx, purpose, []byte("synthetic-aborted-candidate-marker"))
			if err != nil {
				t.Fatalf("PreparePut() error = %v", err)
			}
			if testCase.want == nil {
				candidate.Abort()
				if err := candidate.Commit(ctx); !errors.Is(err, portsecretstore.ErrConflict) {
					t.Fatalf("Commit(after Abort) error = %v, want conflict", err)
				}
			} else if err := candidate.Commit(ctx); !errors.Is(err, testCase.want) {
				t.Fatalf("candidate Commit() error = %v, want %v", err, testCase.want)
			}
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("aborted or failed candidate created an untracked committed store")
			}
		})
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return content
}

func assertRedactedError(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a redacted error")
	}
	for _, value := range forbidden {
		if value != "" && strings.Contains(err.Error(), value) {
			t.Fatalf("error contains forbidden material: %q", err.Error())
		}
	}
}
