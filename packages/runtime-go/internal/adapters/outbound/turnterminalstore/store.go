package turnterminalstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
)

const maxRecordBytes = 256 * 1024

type Store struct {
	mu             sync.Mutex
	root           string
	intents        string
	dispositions   string
	intentCAS      *finalauthorityadapter.SecurePrivateCAS
	dispositionCAS *finalauthorityadapter.SecurePrivateCAS
}

type inventoryV1 struct {
	intents      map[string]domainturnterminal.TurnTerminalIntentV1
	dispositions map[string]domainturnterminal.TurnTerminalDispositionV1
}

func NewStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	return NewStoreContext(context.Background(), root, access)
}

func NewStoreContext(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	if access == nil {
		return nil, errors.New("turn terminal authority access is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("turn terminal authority root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("turn terminal authority root is invalid")
	}
	intents := filepath.Join(absolute, "intents")
	intentCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, intents, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	dispositions := filepath.Join(absolute, "dispositions")
	dispositionCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, dispositions, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	store := &Store{
		root: absolute, intents: intents, dispositions: dispositions,
		intentCAS: intentCAS, dispositionCAS: dispositionCAS,
	}
	store.mu.Lock()
	_, err = store.loadInventoryLocked(ctx)
	store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func PreflightRecovery(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority) error {
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	return prepared.Revalidate(ctx)
}

func (store *Store) PutIntentIfAbsent(ctx context.Context, intent domainturnterminal.TurnTerminalIntentV1) error {
	if store == nil || domainturnterminal.ValidateTurnTerminalIntentV1(intent) != nil {
		return errors.New("turn terminal intent is invalid")
	}
	body, err := domainturnterminal.TurnTerminalIntentV1Bytes(intent)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return errors.New("turn terminal intent exceeds storage limit")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	key := intent.SecurityContext.ContextDigest
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return err
	}
	if existing, found := inventory.intents[key]; found {
		existingBody, _ := domainturnterminal.TurnTerminalIntentV1Bytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("turn terminal context already has another intent")
	}
	if _, found := inventory.dispositions[key]; found {
		return errors.New("turn terminal context has a disposition without its intent")
	}
	if err := store.intentCAS.PutIfAbsent(ctx, key, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readIntentLocked(ctx, key)
		if readErr != nil {
			return readErr
		}
		existingBody, bodyErr := domainturnterminal.TurnTerminalIntentV1Bytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("turn terminal context concurrently acquired another intent")
	}
	written, err := store.readIntentLocked(ctx, key)
	if err != nil || written.IntentID != intent.IntentID {
		return errors.New("turn terminal intent write verification failed")
	}
	if _, err := store.loadInventoryLocked(ctx); err != nil {
		return fmt.Errorf("turn terminal intent inventory verification failed: %w", err)
	}
	return nil
}

func (store *Store) ReadIntent(ctx context.Context, contextDigest string) (domainturnterminal.TurnTerminalIntentV1, error) {
	if store == nil || !validDigest(contextDigest) {
		return domainturnterminal.TurnTerminalIntentV1{}, errors.New("turn terminal context identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domainturnterminal.TurnTerminalIntentV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domainturnterminal.TurnTerminalIntentV1{}, err
	}
	intent, found := inventory.intents[contextDigest]
	if !found {
		return domainturnterminal.TurnTerminalIntentV1{}, turnterminalstoreport.ErrNotFound
	}
	return intent, nil
}

func (store *Store) VisitIntents(ctx context.Context, visit func(domainturnterminal.TurnTerminalIntentV1) error) error {
	if store == nil || visit == nil {
		return errors.New("turn terminal intent visitor is required")
	}
	intents, err := store.snapshotIntents(ctx)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		if err := visit(intent); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) PutDispositionIfAbsent(ctx context.Context, disposition domainturnterminal.TurnTerminalDispositionV1) error {
	if store == nil || domainturnterminal.ValidateTurnTerminalDispositionV1(disposition) != nil {
		return errors.New("turn terminal disposition is invalid")
	}
	body, err := domainturnterminal.TurnTerminalDispositionV1Bytes(disposition)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return errors.New("turn terminal disposition exceeds storage limit")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	key := disposition.ContextDigest
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return err
	}
	intent, found := inventory.intents[key]
	if !found || domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent) != nil {
		return errors.New("turn terminal disposition has no exact intent authority")
	}
	if existing, found := inventory.dispositions[key]; found {
		existingBody, _ := domainturnterminal.TurnTerminalDispositionV1Bytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("turn terminal context was already closed differently")
	}
	if err := store.dispositionCAS.PutIfAbsent(ctx, key, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readDispositionLocked(ctx, key)
		if readErr != nil {
			return readErr
		}
		existingBody, bodyErr := domainturnterminal.TurnTerminalDispositionV1Bytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("turn terminal context was concurrently closed differently")
	}
	written, err := store.readDispositionLocked(ctx, key)
	if err != nil || written.DispositionID != disposition.DispositionID {
		return errors.New("turn terminal disposition write verification failed")
	}
	if _, err := store.loadInventoryLocked(ctx); err != nil {
		return fmt.Errorf("turn terminal disposition inventory verification failed: %w", err)
	}
	return nil
}

func (store *Store) ReadDisposition(ctx context.Context, contextDigest string) (domainturnterminal.TurnTerminalDispositionV1, error) {
	if store == nil || !validDigest(contextDigest) {
		return domainturnterminal.TurnTerminalDispositionV1{}, errors.New("turn terminal context identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, err
	}
	disposition, found := inventory.dispositions[contextDigest]
	if !found {
		return domainturnterminal.TurnTerminalDispositionV1{}, turnterminalstoreport.ErrNotFound
	}
	return disposition, nil
}

func (store *Store) VisitDispositions(ctx context.Context, visit func(domainturnterminal.TurnTerminalDispositionV1) error) error {
	if store == nil || visit == nil {
		return errors.New("turn terminal disposition visitor is required")
	}
	dispositions, err := store.snapshotDispositions(ctx)
	if err != nil {
		return err
	}
	for _, disposition := range dispositions {
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, errors.New("turn terminal authority store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return false, err
	}
	return len(inventory.intents)+len(inventory.dispositions) > 0, nil
}

func (store *Store) snapshotIntents(ctx context.Context) ([]domainturnterminal.TurnTerminalIntentV1, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	keys := sortedKeys(inventory.intents)
	out := make([]domainturnterminal.TurnTerminalIntentV1, 0, len(keys))
	for _, key := range keys {
		out = append(out, inventory.intents[key])
	}
	return out, nil
}

func (store *Store) snapshotDispositions(ctx context.Context) ([]domainturnterminal.TurnTerminalDispositionV1, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	keys := sortedKeys(inventory.dispositions)
	out := make([]domainturnterminal.TurnTerminalDispositionV1, 0, len(keys))
	for _, key := range keys {
		out = append(out, inventory.dispositions[key])
	}
	return out, nil
}

func (store *Store) loadInventoryLocked(ctx context.Context) (inventoryV1, error) {
	if err := contextError(ctx); err != nil {
		return inventoryV1{}, err
	}
	inventory := inventoryV1{
		intents:      map[string]domainturnterminal.TurnTerminalIntentV1{},
		dispositions: map[string]domainturnterminal.TurnTerminalDispositionV1{},
	}
	intentFiles, err := store.intentCAS.List(ctx)
	if err != nil {
		return inventoryV1{}, err
	}
	for _, file := range intentFiles {
		intent, err := domainturnterminal.ParseTurnTerminalIntentV1(file.Body)
		if err != nil || intent.SecurityContext.ContextDigest != file.Digest {
			return inventoryV1{}, errors.Join(errors.New("turn terminal intent content address is corrupt"), err)
		}
		if _, duplicate := inventory.intents[file.Digest]; duplicate {
			return inventoryV1{}, errors.New("turn terminal intent inventory contains a duplicate context")
		}
		inventory.intents[file.Digest] = intent
	}
	dispositionFiles, err := store.dispositionCAS.List(ctx)
	if err != nil {
		return inventoryV1{}, err
	}
	for _, file := range dispositionFiles {
		disposition, err := domainturnterminal.ParseTurnTerminalDispositionV1(file.Body)
		intent, found := inventory.intents[file.Digest]
		if err != nil || disposition.ContextDigest != file.Digest || !found ||
			domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(disposition, intent) != nil {
			return inventoryV1{}, errors.Join(errors.New("turn terminal disposition lost its exact intent authority"), err)
		}
		if _, duplicate := inventory.dispositions[file.Digest]; duplicate {
			return inventoryV1{}, errors.New("turn terminal disposition inventory contains a duplicate context")
		}
		inventory.dispositions[file.Digest] = disposition
	}
	return inventory, nil
}

func (store *Store) readIntentLocked(ctx context.Context, contextDigest string) (domainturnterminal.TurnTerminalIntentV1, error) {
	body, err := store.intentCAS.Read(ctx, contextDigest)
	if err != nil {
		return domainturnterminal.TurnTerminalIntentV1{}, err
	}
	intent, err := domainturnterminal.ParseTurnTerminalIntentV1(body)
	if err != nil {
		return domainturnterminal.TurnTerminalIntentV1{}, fmt.Errorf("read turn terminal intent: %w", err)
	}
	if intent.SecurityContext.ContextDigest != contextDigest {
		return domainturnterminal.TurnTerminalIntentV1{}, errors.New("turn terminal intent content address is corrupt")
	}
	return intent, nil
}

func (store *Store) readDispositionLocked(ctx context.Context, contextDigest string) (domainturnterminal.TurnTerminalDispositionV1, error) {
	body, err := store.dispositionCAS.Read(ctx, contextDigest)
	if err != nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, err
	}
	disposition, err := domainturnterminal.ParseTurnTerminalDispositionV1(body)
	if err != nil {
		return domainturnterminal.TurnTerminalDispositionV1{}, fmt.Errorf("read turn terminal disposition: %w", err)
	}
	if disposition.ContextDigest != contextDigest {
		return domainturnterminal.TurnTerminalDispositionV1{}, errors.New("turn terminal disposition content address is corrupt")
	}
	return disposition, nil
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
