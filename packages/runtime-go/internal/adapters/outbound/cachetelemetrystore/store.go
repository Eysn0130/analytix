package cachetelemetrystore

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
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	cachetelemetrystoreport "analytix.local/runtime-go/internal/ports/cachetelemetrystore"
)

const maxRecordBytes = 1024 * 1024

type Store struct {
	mu             sync.Mutex
	root           string
	attempts       string
	settlements    string
	turnClosures   string
	attemptCAS     *finalauthorityadapter.SecurePrivateCAS
	settlementCAS  *finalauthorityadapter.SecurePrivateCAS
	turnClosureCAS *finalauthorityadapter.SecurePrivateCAS
}

type inventoryV1 struct {
	intents     map[string]domaincachetelemetry.ProviderAttemptIntentV1
	settlements map[string]domaincachetelemetry.ProviderAttemptSettlementV1
	closures    map[string]domaincachetelemetry.ProviderTurnClosureV1
	turnIntents map[string][]domaincachetelemetry.ProviderAttemptIntentV1
	turnSettles map[string][]domaincachetelemetry.ProviderAttemptSettlementV1
}

func NewStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	return NewStoreContext(context.Background(), root, access)
}

func NewStoreContext(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*Store, error) {
	if access == nil {
		return nil, errors.New("provider cache telemetry access authority is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("provider cache telemetry root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("provider cache telemetry root is invalid")
	}
	attempts := filepath.Join(absolute, "attempts")
	attemptCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, attempts, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	settlements := filepath.Join(absolute, "settlements")
	settlementCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, settlements, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	turnClosures := filepath.Join(absolute, "turn-closures")
	turnClosureCAS, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, turnClosures, maxRecordBytes, access)
	if err != nil {
		return nil, err
	}
	store := &Store{
		root: absolute, attempts: attempts, settlements: settlements, turnClosures: turnClosures,
		attemptCAS: attemptCAS, settlementCAS: settlementCAS, turnClosureCAS: turnClosureCAS,
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

func (store *Store) PutIntentIfAbsent(ctx context.Context, intent domaincachetelemetry.ProviderAttemptIntentV1) error {
	_, err := store.CommitIntentIfAbsent(ctx, intent)
	return err
}

// CommitIntentIfAbsent returns the exact semantic value whose canonical bytes
// the secure CAS committed and read back. The post-commit cross-leaf scan still
// guards the complete ledger; callers do not need a second record read.
func (store *Store) CommitIntentIfAbsent(ctx context.Context, intent domaincachetelemetry.ProviderAttemptIntentV1) (domaincachetelemetry.ProviderAttemptIntentV1, error) {
	if store == nil || domaincachetelemetry.ValidateProviderAttemptIntentV1(intent) != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry intent is invalid")
	}
	body, err := domaincachetelemetry.ProviderAttemptIntentV1Bytes(intent)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry intent exceeds storage limit")
	}
	if err := contextError(ctx); err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, err
	}
	if existing, found := inventory.intents[intent.IntentID]; found {
		existingBody, _ := domaincachetelemetry.ProviderAttemptIntentV1Bytes(existing)
		if bytes.Equal(existingBody, body) {
			return existing, nil
		}
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry intent conflicts with existing authority")
	}
	if _, closed := inventory.closures[intent.TurnBindingHMAC]; closed {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry turn is already closed")
	}
	candidateIntents := append(append([]domaincachetelemetry.ProviderAttemptIntentV1(nil), inventory.turnIntents[intent.TurnBindingHMAC]...), intent)
	if err := domaincachetelemetry.ValidateProviderLedgerOpenInventoryV1(intent.TurnBindingHMAC, candidateIntents, inventory.turnSettles[intent.TurnBindingHMAC]); err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, fmt.Errorf("provider cache telemetry intent would corrupt open inventory: %w", err)
	}
	if err := store.attemptCAS.PutIfAbsent(ctx, intent.IntentID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return domaincachetelemetry.ProviderAttemptIntentV1{}, err
		}
		existing, readErr := store.readIntentLocked(ctx, intent.IntentID)
		if readErr != nil {
			return domaincachetelemetry.ProviderAttemptIntentV1{}, readErr
		}
		existingBody, bodyErr := domaincachetelemetry.ProviderAttemptIntentV1Bytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return existing, nil
		}
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry intent conflicts with concurrently created authority")
	}
	if _, err := store.loadInventoryLocked(ctx); err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, fmt.Errorf("provider cache telemetry intent inventory verification failed: %w", err)
	}
	return intent, nil
}

func (store *Store) ReadIntent(ctx context.Context, intentID string) (domaincachetelemetry.ProviderAttemptIntentV1, error) {
	if store == nil || !validDigest(intentID) {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry intent identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, err
	}
	intent, found := inventory.intents[intentID]
	if !found {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, cachetelemetrystoreport.ErrNotFound
	}
	return intent, nil
}

func (store *Store) VisitIntents(ctx context.Context, visit func(domaincachetelemetry.ProviderAttemptIntentV1) error) error {
	if store == nil || visit == nil {
		return errors.New("provider cache telemetry intent visitor is required")
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

// VisitInventory exposes one exact, cross-leaf inventory observation. The
// three visitors run after the store lock is released, but every value comes
// from the same fully validated CAS scan rather than three independently
// timed observations.
func (store *Store) VisitInventory(
	ctx context.Context,
	visitIntent func(domaincachetelemetry.ProviderAttemptIntentV1) error,
	visitSettlement func(domaincachetelemetry.ProviderAttemptSettlementV1) error,
	visitClosure func(domaincachetelemetry.ProviderTurnClosureV1) error,
) error {
	if store == nil || visitIntent == nil || visitSettlement == nil || visitClosure == nil {
		return errors.New("provider cache telemetry inventory visitors are required")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	store.mu.Lock()
	inventory, err := store.loadInventoryLocked(ctx)
	store.mu.Unlock()
	if err != nil {
		return err
	}
	intents := make([]domaincachetelemetry.ProviderAttemptIntentV1, 0, len(inventory.intents))
	for _, value := range inventory.intents {
		intents = append(intents, value)
	}
	settlements := make([]domaincachetelemetry.ProviderAttemptSettlementV1, 0, len(inventory.settlements))
	for _, value := range inventory.settlements {
		settlements = append(settlements, value)
	}
	closures := make([]domaincachetelemetry.ProviderTurnClosureV1, 0, len(inventory.closures))
	for _, value := range inventory.closures {
		closures = append(closures, value)
	}
	sort.Slice(intents, func(i, j int) bool { return intents[i].IntentID < intents[j].IntentID })
	sort.Slice(settlements, func(i, j int) bool { return settlements[i].IntentID < settlements[j].IntentID })
	sort.Slice(closures, func(i, j int) bool { return closures[i].TurnBindingHMAC < closures[j].TurnBindingHMAC })
	for _, value := range intents {
		if err := visitIntent(value); err != nil {
			return err
		}
	}
	for _, value := range settlements {
		if err := visitSettlement(value); err != nil {
			return err
		}
	}
	for _, value := range closures {
		if err := visitClosure(value); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) PutSettlementIfAbsent(ctx context.Context, settlement domaincachetelemetry.ProviderAttemptSettlementV1) error {
	_, err := store.CommitSettlementIfAbsent(ctx, settlement)
	return err
}

// CommitSettlementIfAbsent returns the settlement covered by the secure CAS
// commit's exact read-back while retaining the complete cross-leaf post-check.
func (store *Store) CommitSettlementIfAbsent(ctx context.Context, settlement domaincachetelemetry.ProviderAttemptSettlementV1) (domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	if store == nil || domaincachetelemetry.ValidateProviderAttemptSettlementV1(settlement) != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry settlement is invalid")
	}
	body, err := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(settlement)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry settlement exceeds storage limit")
	}
	if err := contextError(ctx); err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
	}
	if existing, found := inventory.settlements[settlement.IntentID]; found {
		existingBody, _ := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(existing)
		if bytes.Equal(existingBody, body) {
			return existing, nil
		}
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry intent is already settled")
	}
	if _, closed := inventory.closures[settlement.TurnBindingHMAC]; closed {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry turn is already closed")
	}
	intent, found := inventory.intents[settlement.IntentID]
	if !found || domaincachetelemetry.ValidateProviderAttemptSettlementForIntentV1(settlement, intent) != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry settlement has no exact intent authority")
	}
	candidateSettlements := append(append([]domaincachetelemetry.ProviderAttemptSettlementV1(nil), inventory.turnSettles[settlement.TurnBindingHMAC]...), settlement)
	if err := domaincachetelemetry.ValidateProviderLedgerOpenInventoryV1(settlement.TurnBindingHMAC, inventory.turnIntents[settlement.TurnBindingHMAC], candidateSettlements); err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, fmt.Errorf("provider cache telemetry settlement would corrupt open inventory: %w", err)
	}
	if err := store.settlementCAS.PutIfAbsent(ctx, settlement.IntentID, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
		}
		existing, readErr := store.readSettlementLocked(ctx, settlement.IntentID)
		if readErr != nil {
			return domaincachetelemetry.ProviderAttemptSettlementV1{}, readErr
		}
		existingBody, bodyErr := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(existing)
		if bodyErr == nil && bytes.Equal(existingBody, body) {
			return existing, nil
		}
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry intent was concurrently settled differently")
	}
	if _, err := store.loadInventoryLocked(ctx); err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, fmt.Errorf("provider cache telemetry settlement inventory verification failed: %w", err)
	}
	return settlement, nil
}

func (store *Store) ReadSettlement(ctx context.Context, intentID string) (domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	if store == nil || !validDigest(intentID) {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry settlement identity is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
	}
	settlement, found := inventory.settlements[intentID]
	if !found {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, cachetelemetrystoreport.ErrNotFound
	}
	return settlement, nil
}

func (store *Store) VisitSettlements(ctx context.Context, visit func(domaincachetelemetry.ProviderAttemptSettlementV1) error) error {
	if store == nil || visit == nil {
		return errors.New("provider cache telemetry settlement visitor is required")
	}
	settlements, err := store.snapshotSettlements(ctx)
	if err != nil {
		return err
	}
	for _, settlement := range settlements {
		if err := visit(settlement); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) PutClosureIfAbsent(ctx context.Context, closure domaincachetelemetry.ProviderTurnClosureV1) error {
	persisted, err := store.CommitClosureIfAbsent(ctx, closure)
	if err != nil {
		return err
	}
	persistedBody, persistedErr := domaincachetelemetry.ProviderTurnClosureV1Bytes(persisted)
	closureBody, closureErr := domaincachetelemetry.ProviderTurnClosureV1Bytes(closure)
	if persistedErr != nil || closureErr != nil || !bytes.Equal(persistedBody, closureBody) {
		return errors.New("provider cache telemetry turn is already closed differently")
	}
	return nil
}

// CommitClosureIfAbsent returns the exact closure in the fully validated
// cross-leaf ledger. A concurrent valid closure is returned to let the service
// preserve same-reason idempotency while still verifying installation authority.
func (store *Store) CommitClosureIfAbsent(ctx context.Context, closure domaincachetelemetry.ProviderTurnClosureV1) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	if store == nil || domaincachetelemetry.ValidateProviderTurnClosureV1(closure) != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, errors.New("provider cache telemetry closure is invalid")
	}
	body, err := domaincachetelemetry.ProviderTurnClosureV1Bytes(closure)
	if err != nil || len(body) == 0 || len(body) > maxRecordBytes {
		return domaincachetelemetry.ProviderTurnClosureV1{}, errors.New("provider cache telemetry closure exceeds storage limit")
	}
	if err := contextError(ctx); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	if existing, found := inventory.closures[closure.TurnBindingHMAC]; found {
		return existing, nil
	}
	if err := domaincachetelemetry.ValidateProviderTurnClosureForInventoryV1(
		closure, inventory.turnIntents[closure.TurnBindingHMAC], inventory.turnSettles[closure.TurnBindingHMAC],
	); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, fmt.Errorf("provider cache telemetry closure does not match live inventory: %w", err)
	}
	if err := store.turnClosureCAS.PutIfAbsent(ctx, closure.TurnBindingHMAC, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return domaincachetelemetry.ProviderTurnClosureV1{}, err
		}
	}
	verifiedInventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, fmt.Errorf("provider cache telemetry closure inventory verification failed: %w", err)
	}
	persisted, found := verifiedInventory.closures[closure.TurnBindingHMAC]
	if !found {
		return domaincachetelemetry.ProviderTurnClosureV1{}, errors.New("provider cache telemetry closure write verification failed")
	}
	return persisted, nil
}

func (store *Store) ReadClosure(ctx context.Context, turnBindingHMAC string) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	if store == nil || !validDigest(turnBindingHMAC) {
		return domaincachetelemetry.ProviderTurnClosureV1{}, errors.New("provider cache telemetry turn binding is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadInventoryLocked(ctx)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	closure, found := inventory.closures[turnBindingHMAC]
	if !found {
		return domaincachetelemetry.ProviderTurnClosureV1{}, cachetelemetrystoreport.ErrNotFound
	}
	return closure, nil
}

func (store *Store) VisitClosures(ctx context.Context, visit func(domaincachetelemetry.ProviderTurnClosureV1) error) error {
	if store == nil || visit == nil {
		return errors.New("provider cache telemetry closure visitor is required")
	}
	closures, err := store.snapshotClosures(ctx)
	if err != nil {
		return err
	}
	for _, closure := range closures {
		if err := visit(closure); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, errors.New("provider cache telemetry store is unavailable")
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
	return len(inventory.intents)+len(inventory.settlements)+len(inventory.closures) > 0, nil
}

func (store *Store) loadInventoryLocked(ctx context.Context) (inventoryV1, error) {
	attemptFiles, err := store.attemptCAS.List(ctx)
	if err != nil {
		return inventoryV1{}, err
	}
	settlementFiles, err := store.settlementCAS.List(ctx)
	if err != nil {
		return inventoryV1{}, err
	}
	closureFiles, err := store.turnClosureCAS.List(ctx)
	if err != nil {
		return inventoryV1{}, err
	}
	return validateInventoryFilesV1(attemptFiles, settlementFiles, closureFiles)
}

func validateInventoryFilesV1(attemptFiles, settlementFiles, closureFiles []finalauthorityadapter.SecurePrivateCASFile) (inventoryV1, error) {
	inventory := inventoryV1{
		intents:     make(map[string]domaincachetelemetry.ProviderAttemptIntentV1),
		settlements: make(map[string]domaincachetelemetry.ProviderAttemptSettlementV1),
		closures:    make(map[string]domaincachetelemetry.ProviderTurnClosureV1),
		turnIntents: make(map[string][]domaincachetelemetry.ProviderAttemptIntentV1),
		turnSettles: make(map[string][]domaincachetelemetry.ProviderAttemptSettlementV1),
	}
	for _, file := range attemptFiles {
		intent, err := domaincachetelemetry.ParseProviderAttemptIntentV1(file.Body)
		canonical, canonicalErr := domaincachetelemetry.ProviderAttemptIntentV1Bytes(intent)
		if err != nil || canonicalErr != nil || intent.IntentID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return inventoryV1{}, errors.Join(errors.New("provider cache telemetry intent is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if _, duplicate := inventory.intents[intent.IntentID]; duplicate {
			return inventoryV1{}, errors.New("provider cache telemetry inventory contains a duplicate intent")
		}
		inventory.intents[intent.IntentID] = intent
		inventory.turnIntents[intent.TurnBindingHMAC] = append(inventory.turnIntents[intent.TurnBindingHMAC], intent)
	}
	for _, file := range settlementFiles {
		settlement, err := domaincachetelemetry.ParseProviderAttemptSettlementV1(file.Body)
		canonical, canonicalErr := domaincachetelemetry.ProviderAttemptSettlementV1Bytes(settlement)
		intent, found := inventory.intents[file.Digest]
		if err != nil || canonicalErr != nil || settlement.IntentID != file.Digest || !bytes.Equal(canonical, file.Body) ||
			!found || domaincachetelemetry.ValidateProviderAttemptSettlementForIntentV1(settlement, intent) != nil {
			return inventoryV1{}, errors.Join(errors.New("provider cache telemetry settlement lost its exact intent authority"), err, canonicalErr)
		}
		if _, duplicate := inventory.settlements[settlement.IntentID]; duplicate {
			return inventoryV1{}, errors.New("provider cache telemetry inventory contains a duplicate settlement")
		}
		inventory.settlements[settlement.IntentID] = settlement
		inventory.turnSettles[settlement.TurnBindingHMAC] = append(inventory.turnSettles[settlement.TurnBindingHMAC], settlement)
	}
	for _, file := range closureFiles {
		closure, err := domaincachetelemetry.ParseProviderTurnClosureV1(file.Body)
		canonical, canonicalErr := domaincachetelemetry.ProviderTurnClosureV1Bytes(closure)
		if err != nil || canonicalErr != nil || closure.TurnBindingHMAC != file.Digest || !bytes.Equal(canonical, file.Body) {
			return inventoryV1{}, errors.Join(errors.New("provider cache telemetry closure is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		if _, duplicate := inventory.closures[closure.TurnBindingHMAC]; duplicate {
			return inventoryV1{}, errors.New("provider cache telemetry inventory contains a duplicate turn closure")
		}
		inventory.closures[closure.TurnBindingHMAC] = closure
	}
	turns := make(map[string]struct{})
	for turn := range inventory.turnIntents {
		turns[turn] = struct{}{}
	}
	for turn := range inventory.turnSettles {
		turns[turn] = struct{}{}
	}
	for turn := range inventory.closures {
		turns[turn] = struct{}{}
	}
	for turn := range turns {
		if closure, closed := inventory.closures[turn]; closed {
			if err := domaincachetelemetry.ValidateProviderTurnClosureForInventoryV1(closure, inventory.turnIntents[turn], inventory.turnSettles[turn]); err != nil {
				return inventoryV1{}, fmt.Errorf("provider cache telemetry closed inventory is corrupt: %w", err)
			}
		} else if err := domaincachetelemetry.ValidateProviderLedgerOpenInventoryV1(turn, inventory.turnIntents[turn], inventory.turnSettles[turn]); err != nil {
			return inventoryV1{}, fmt.Errorf("provider cache telemetry open inventory is corrupt: %w", err)
		}
	}
	return inventory, nil
}

func (store *Store) snapshotIntents(ctx context.Context) ([]domaincachetelemetry.ProviderAttemptIntentV1, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	inventory, err := store.loadInventoryLocked(ctx)
	store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	values := make([]domaincachetelemetry.ProviderAttemptIntentV1, 0, len(inventory.intents))
	for _, value := range inventory.intents {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].IntentID < values[j].IntentID })
	return values, nil
}

func (store *Store) snapshotSettlements(ctx context.Context) ([]domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	inventory, err := store.loadInventoryLocked(ctx)
	store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	values := make([]domaincachetelemetry.ProviderAttemptSettlementV1, 0, len(inventory.settlements))
	for _, value := range inventory.settlements {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].IntentID < values[j].IntentID })
	return values, nil
}

func (store *Store) snapshotClosures(ctx context.Context) ([]domaincachetelemetry.ProviderTurnClosureV1, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	inventory, err := store.loadInventoryLocked(ctx)
	store.mu.Unlock()
	if err != nil {
		return nil, err
	}
	values := make([]domaincachetelemetry.ProviderTurnClosureV1, 0, len(inventory.closures))
	for _, value := range inventory.closures {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].TurnBindingHMAC < values[j].TurnBindingHMAC })
	return values, nil
}

func (store *Store) readIntentLocked(ctx context.Context, intentID string) (domaincachetelemetry.ProviderAttemptIntentV1, error) {
	body, err := store.attemptCAS.Read(ctx, intentID)
	if err != nil {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, err
	}
	intent, err := domaincachetelemetry.ParseProviderAttemptIntentV1(body)
	if err != nil || intent.IntentID != intentID {
		return domaincachetelemetry.ProviderAttemptIntentV1{}, errors.New("provider cache telemetry intent content address is corrupt")
	}
	return intent, nil
}

func (store *Store) readSettlementLocked(ctx context.Context, intentID string) (domaincachetelemetry.ProviderAttemptSettlementV1, error) {
	body, err := store.settlementCAS.Read(ctx, intentID)
	if err != nil {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, err
	}
	settlement, err := domaincachetelemetry.ParseProviderAttemptSettlementV1(body)
	if err != nil || settlement.IntentID != intentID {
		return domaincachetelemetry.ProviderAttemptSettlementV1{}, errors.New("provider cache telemetry settlement content address is corrupt")
	}
	return settlement, nil
}

func (store *Store) readClosureLocked(ctx context.Context, turnBinding string) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	body, err := store.turnClosureCAS.Read(ctx, turnBinding)
	if err != nil {
		return domaincachetelemetry.ProviderTurnClosureV1{}, err
	}
	closure, err := domaincachetelemetry.ParseProviderTurnClosureV1(body)
	if err != nil || closure.TurnBindingHMAC != turnBinding {
		return domaincachetelemetry.ProviderTurnClosureV1{}, errors.New("provider cache telemetry closure content address is corrupt")
	}
	return closure, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("provider cache telemetry context is required")
	}
	return ctx.Err()
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
