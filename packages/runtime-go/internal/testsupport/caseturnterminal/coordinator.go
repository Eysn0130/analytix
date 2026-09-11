package caseturnterminal

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"

	appturnterminal "analytix.local/runtime-go/internal/app/turnterminal"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
)

// NewInMemoryCoordinatorV1 supplies the complete terminal-authority chain to
// tests without making a production constructor silently bypass provider
// closure or terminal dispositions.
func NewInMemoryCoordinatorV1(authority finalauthorityport.Authority, privateFinals finalauthorityport.PrivateFinalStore) (*appturnterminal.Coordinator, error) {
	if authority == nil || privateFinals == nil {
		return nil, errors.New("test turn terminal authority is incomplete")
	}
	return appturnterminal.NewCoordinator(
		authority,
		privateFinals,
		&memoryTerminalStoreV1{
			intents:      map[string]domainturnterminal.TurnTerminalIntentV1{},
			dispositions: map[string]domainturnterminal.TurnTerminalDispositionV1{},
		},
		&memoryProviderCloserV1{
			authority: authority,
			closures:  map[string]domaincachetelemetry.ProviderTurnClosureV1{},
		},
	)
}

type memoryTerminalStoreV1 struct {
	mu           sync.Mutex
	intents      map[string]domainturnterminal.TurnTerminalIntentV1
	dispositions map[string]domainturnterminal.TurnTerminalDispositionV1
}

func (store *memoryTerminalStoreV1) PutIntentIfAbsent(_ context.Context, intent domainturnterminal.TurnTerminalIntentV1) error {
	if err := domainturnterminal.ValidateTurnTerminalIntentV1(intent); err != nil {
		return err
	}
	key := intent.SecurityContext.ContextDigest
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, exists := store.intents[key]; exists && !reflect.DeepEqual(current, intent) {
		return errors.New("test terminal intent conflicts")
	}
	store.intents[key] = intent
	return nil
}

func (store *memoryTerminalStoreV1) ReadIntent(_ context.Context, contextDigest string) (domainturnterminal.TurnTerminalIntentV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, exists := store.intents[contextDigest]
	if !exists {
		return domainturnterminal.TurnTerminalIntentV1{}, turnterminalstoreport.ErrNotFound
	}
	return intent, nil
}

func (store *memoryTerminalStoreV1) VisitIntents(_ context.Context, visit func(domainturnterminal.TurnTerminalIntentV1) error) error {
	store.mu.Lock()
	values := make([]domainturnterminal.TurnTerminalIntentV1, 0, len(store.intents))
	for _, intent := range store.intents {
		values = append(values, intent)
	}
	store.mu.Unlock()
	for _, intent := range values {
		if err := visit(intent); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryTerminalStoreV1) PutDispositionIfAbsent(_ context.Context, disposition domainturnterminal.TurnTerminalDispositionV1) error {
	if err := domainturnterminal.ValidateTurnTerminalDispositionV1(disposition); err != nil {
		return err
	}
	key := disposition.ContextDigest
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, exists := store.intents[key]
	if !exists || domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent) != nil {
		return errors.New("test terminal disposition lacks its exact intent")
	}
	if current, exists := store.dispositions[key]; exists && !reflect.DeepEqual(current, disposition) {
		return errors.New("test terminal disposition conflicts")
	}
	store.dispositions[key] = disposition
	return nil
}

func (store *memoryTerminalStoreV1) ReadDisposition(_ context.Context, contextDigest string) (domainturnterminal.TurnTerminalDispositionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	disposition, exists := store.dispositions[contextDigest]
	if !exists {
		return domainturnterminal.TurnTerminalDispositionV1{}, turnterminalstoreport.ErrNotFound
	}
	return disposition, nil
}

func (store *memoryTerminalStoreV1) VisitDispositions(_ context.Context, visit func(domainturnterminal.TurnTerminalDispositionV1) error) error {
	store.mu.Lock()
	values := make([]domainturnterminal.TurnTerminalDispositionV1, 0, len(store.dispositions))
	for _, disposition := range store.dispositions {
		values = append(values, disposition)
	}
	store.mu.Unlock()
	for _, disposition := range values {
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryTerminalStoreV1) HasRecords(context.Context) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.intents) != 0 || len(store.dispositions) != 0, nil
}

type memoryProviderCloserV1 struct {
	mu        sync.Mutex
	authority finalauthorityport.Authority
	closures  map[string]domaincachetelemetry.ProviderTurnClosureV1
}

func (closer *memoryProviderCloserV1) CloseTurn(ctx context.Context, securityContext domainsecurity.TurnSecurityContext,
	reason domaincachetelemetry.ProviderTurnTerminalReasonV1, closedAt time.Time) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	if err := domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	if err := domaincachetelemetry.ValidateProviderTurnTerminalReasonV1(reason); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	key := securityContext.ContextDigest
	closer.mu.Lock()
	defer closer.mu.Unlock()
	if current, exists := closer.closures[key]; exists {
		if current.TerminalReasonCode != reason {
			return domaincachetelemetry.ProviderTurnClosureV1{}, errors.New("test provider closure reason conflicts")
		}
		return current, nil
	}
	closure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC:    domainsecurity.SHA256Hex([]byte("test-provider-turn:" + key)),
		TerminalReasonCode: reason, ClosedAt: closedAt,
		AuthorityKeyID: closer.authority.KeyID(), AuthorityPublicKey: closer.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return closer.authority.Sign(ctx, message) })
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	closer.closures[key] = closure
	return closure, nil
}

func (closer *memoryProviderCloserV1) ObserveTurnClosureV1(_ context.Context, securityContext domainsecurity.TurnSecurityContext) (domaincachetelemetry.ProviderTurnClosureV1, bool, error) {
	if err := domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, false, err
	}
	closer.mu.Lock()
	defer closer.mu.Unlock()
	closure, found := closer.closures[securityContext.ContextDigest]
	return closure, found, nil
}

func (closer *memoryProviderCloserV1) VisitTurnClosuresV1(_ context.Context, visit func(domaincachetelemetry.ProviderTurnClosureV1) error) error {
	if visit == nil {
		return errors.New("test provider closure visitor is unavailable")
	}
	closer.mu.Lock()
	values := make([]domaincachetelemetry.ProviderTurnClosureV1, 0, len(closer.closures))
	for _, closure := range closer.closures {
		values = append(values, closure)
	}
	closer.mu.Unlock()
	sort.Slice(values, func(i, j int) bool { return values[i].ClosureID < values[j].ClosureID })
	for _, closure := range values {
		if err := visit(closure); err != nil {
			return err
		}
	}
	return nil
}
