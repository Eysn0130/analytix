package providerregistry

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestManagerConnectCommitsKeyFreeRegistryAfterCandidateIsDurable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	candidateRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("B", 43))
	secretMarker := []byte("synthetic-provider-secret-marker")
	events := make([]string, 0, 16)

	registry := &recordingRegistryStore{
		state: domainregistry.Registry{
			Version:      domainregistry.FormatVersion,
			Incarnation:  registryIncarnation,
			Providers:    map[string]domainregistry.Provider{},
			Transactions: map[string]domainregistry.Transaction{},
		},
		events: &events,
	}
	secrets := &recordingSecretStore{
		candidateRef: candidateRef,
		secret:       bytes.Clone(secretMarker),
		events:       &events,
	}
	faults := FaultRecorderFunc(func(point FaultPoint) error {
		events = append(events, "fault:"+string(point))
		return nil
	})
	manager, err := NewManagerWithFaultRecorder(registry, secrets, faults)
	if err != nil {
		t.Fatalf("NewManagerWithFaultRecorder() error = %v", err)
	}
	credential, err := secretstoreport.SetCredential(secretMarker)
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}

	committed, err := manager.Connect(ctx, ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision:    0,
			RegistryIncarnation: registryIncarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID:             "provider-alpha",
			Kind:           "openai-compatible",
			Endpoint:       "https://provider.invalid/v1",
			Proxy:          "",
			Models:         []string{"model-alpha"},
			MediaModels:    []string{"media-alpha"},
			SelectedModel:  "model-alpha",
			SelectedMedia:  "media-alpha",
			SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
		Credential:        credential,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if committed.CredentialRef != string(candidateRef) || committed.CredentialPurpose != "provider-api-key" {
		t.Fatalf("committed credential authority = (%q, %q), want synthetic candidate ref and purpose", committed.CredentialRef, committed.CredentialPurpose)
	}
	if committed.ID != "provider-alpha" || committed.Revision != 1 || committed.Generation != 1 {
		t.Fatalf("committed Provider = %#v", committed)
	}
	if bytes.Contains(registry.committedBytes, secretMarker) {
		t.Fatal("committed Registry bytes contain the synthetic secret marker")
	}

	wantOrder := []string{
		"registry:prepared",
		"secret:candidate-durable",
		"registry:winner-committed",
		"secret:authorized-readback-verified",
	}
	last := -1
	for _, want := range wantOrder {
		index := indexOfEvent(events, want)
		if index < 0 {
			t.Fatalf("events are missing %q: %v", want, events)
		}
		if index <= last {
			t.Fatalf("events are out of order for %q: %v", want, events)
		}
		last = index
	}
	if !secrets.readbackAuthorized {
		t.Fatal("Manager did not use the authorized internal consumer readback seam")
	}
}

type recordingRegistryStore struct {
	state          domainregistry.Registry
	events         *[]string
	committedBytes []byte
}

func (store *recordingRegistryStore) WithExclusive(
	ctx context.Context,
	use func(registryport.Transaction) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(&recordingRegistryTransaction{store: store})
}

type recordingRegistryTransaction struct {
	store *recordingRegistryStore
}

func (transaction *recordingRegistryTransaction) Load(context.Context) (domainregistry.Registry, error) {
	return transaction.store.state.Clone(), nil
}

func (transaction *recordingRegistryTransaction) Commit(_ context.Context, state domainregistry.Registry) error {
	if err := state.Validate(); err != nil {
		return err
	}
	for _, pending := range state.Transactions {
		switch pending.Phase {
		case domainregistry.PhasePrepared:
			*transaction.store.events = append(*transaction.store.events, "registry:prepared")
		case domainregistry.PhaseCandidateDurable:
			*transaction.store.events = append(*transaction.store.events, "registry:candidate-durable-recorded")
		case domainregistry.PhaseMetadataCommitted:
			*transaction.store.events = append(*transaction.store.events, "registry:winner-committed")
		}
	}
	committed, err := domainregistry.Marshal(state)
	if err != nil {
		return err
	}
	transaction.store.committedBytes = committed
	transaction.store.state = state.Clone()
	return nil
}

type recordingSecretStore struct {
	candidateRef       secretstoreport.CredentialRef
	secret             []byte
	events             *[]string
	candidateDurable   bool
	readbackAuthorized bool
}

func (store *recordingSecretStore) PreparePut(
	_ context.Context,
	purpose secretstoreport.Purpose,
	secret []byte,
) (secretstoreport.PreparedCandidate, error) {
	if purpose != secretstoreport.Purpose("provider-api-key") || !bytes.Equal(secret, store.secret) {
		return nil, secretstoreport.ErrInvalidRequest
	}
	return &recordingPreparedCandidate{store: store}, nil
}

func (store *recordingSecretStore) GetForAuthorizedConsumer(
	_ context.Context,
	request secretstoreport.AccessRequest,
) ([]byte, error) {
	if !store.candidateDurable || request.CredentialRef != store.candidateRef ||
		request.Purpose != secretstoreport.Purpose("provider-api-key") ||
		(request.Consumer != RegistryReadbackConsumer && request.Consumer != ProviderExecutionConsumer) {
		return nil, secretstoreport.ErrUnauthorized
	}
	if request.Consumer == RegistryReadbackConsumer {
		store.readbackAuthorized = true
		*store.events = append(*store.events, "secret:authorized-readback-verified")
	}
	return bytes.Clone(store.secret), nil
}

func (*recordingSecretStore) Tombstone(
	context.Context,
	secretstoreport.CredentialRef,
	secretstoreport.Purpose,
) error {
	return errors.New("unexpected tombstone during first connect")
}

func (*recordingSecretStore) ExplicitDelete(
	context.Context,
	secretstoreport.CredentialRef,
	secretstoreport.Purpose,
	secretstoreport.CredentialMutation,
) error {
	return errors.New("unexpected explicit delete during first connect")
}

type recordingPreparedCandidate struct {
	store *recordingSecretStore
}

func (candidate *recordingPreparedCandidate) CredentialRef() secretstoreport.CredentialRef {
	return candidate.store.candidateRef
}

func (candidate *recordingPreparedCandidate) Commit(context.Context) error {
	if candidate.store.candidateDurable {
		return secretstoreport.ErrConflict
	}
	candidate.store.candidateDurable = true
	*candidate.store.events = append(*candidate.store.events, "secret:candidate-durable")
	return nil
}

func (candidate *recordingPreparedCandidate) Abort() {
	if !candidate.store.candidateDurable {
		candidate.store.secret = nil
	}
}

func indexOfEvent(events []string, want string) int {
	for index, event := range events {
		if reflect.DeepEqual(event, want) {
			return index
		}
	}
	return -1
}

// Physical metadata helpers are test-only fixture checks; production reads live in the filesystem adapter.
func legacyMigrationFileInfoUintField(info os.FileInfo, fieldName string) (uint64, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	value := reflect.ValueOf(info.Sys())
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName(fieldName)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() < 0 {
			return 0, false
		}
		return uint64(field.Int()), true
	default:
		return 0, false
	}
}

func legacyMigrationRegularSingleLinkFileInfo(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	links, ok := legacyMigrationFileInfoUintField(info, "Nlink")
	if !ok {
		links, ok = legacyMigrationFileInfoUintField(info, "NumberOfLinks")
	}
	return ok && links == 1
}

func TestManagerMissingLegacySourceReaderFailsClosed(t *testing.T) {
	t.Parallel()
	manager, err := NewManager(newMemoryRegistryStore(), newMemorySecretStore())
	if err != nil {
		t.Fatal(err)
	}
	challenge := legacyMigrationSourceAuthorityPrefix + strings.Repeat("A", 43)
	command := legacyMigrationSourceAuthorityChallenge{Operation: LegacyMigrationSourceAuthorityOperationFinalize}
	manager.sourceAuthorityChallenges = map[string]legacyMigrationSourceAuthorityChallenge{challenge: command}
	intent, generation, err := manager.consumeLegacyMigrationSourceAuthority(command, LegacyMigrationSourceAuthorityProof{
		Challenge: challenge, SourcePath: t.TempDir(), SourceDevice: "1", SourceInode: "1",
		LockOwnerToken: "01234567-89ab-4000-8000-0123456789ab",
	}, []byte(`{"synthetic":"source"}`))
	if !errors.Is(err, registryport.ErrInvalidRequest) || intent != "" || generation != "" {
		t.Fatal("source authority must fail closed without an explicitly composed reader")
	}
	if _, exists := manager.sourceAuthorityChallenges[challenge]; !exists {
		t.Fatal("missing dependency consumed a challenge before input validation")
	}
}
