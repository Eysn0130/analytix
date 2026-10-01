package checkpointauthority

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
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const maxCheckpointAuthorityCASBytes = domaincheckpoint.MaxSnapshotAuthorityRecordBytes

type Store struct {
	mu                   sync.Mutex
	root                 string
	intentCAS            *finalauthority.SecurePrivateCAS
	completionCAS        *finalauthority.SecurePrivateCAS
	dispositionCAS       *finalauthority.SecurePrivateCAS
	operationIntentCAS   *finalauthority.SecurePrivateCAS
	operationTerminalCAS *finalauthority.SecurePrivateCAS
}

var _ checkpointport.Store = (*Store)(nil)

func NewStoreContext(ctx context.Context, root string, access privatecasport.AccessAuthority) (*Store, error) {
	absolute, err := checkpointAuthorityRoot(root, access)
	if err != nil {
		return nil, err
	}
	intentCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "snapshot-intents"), maxCheckpointAuthorityCASBytes, access,
	)
	if err != nil {
		return nil, err
	}
	completionCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "snapshot-completions"), maxCheckpointAuthorityCASBytes, access,
	)
	if err != nil {
		return nil, err
	}
	dispositionCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "snapshot-dispositions"), maxCheckpointAuthorityCASBytes, access,
	)
	if err != nil {
		return nil, err
	}
	operationIntentCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "operation-group-intents-v2"), domaincheckpoint.MaxOperationGroupIntentRecordBytes, access,
	)
	if err != nil {
		return nil, err
	}
	operationTerminalCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx, filepath.Join(absolute, "operation-group-terminals-v2"), maxCheckpointAuthorityCASBytes, access,
	)
	if err != nil {
		return nil, err
	}
	store := &Store{
		root: absolute, intentCAS: intentCAS, completionCAS: completionCAS, dispositionCAS: dispositionCAS,
		operationIntentCAS: operationIntentCAS, operationTerminalCAS: operationTerminalCAS,
	}
	if err := store.validateInventory(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// OpenExistingStoreContext opens the complete current checkpoint-authority
// layout without creating any CAS root. A wholly absent layout is an optional
// authority with no live recovery work. Any partial layout is corrupt rather
// than an invitation to synthesize the missing authority stores.
func OpenExistingStoreContext(
	ctx context.Context,
	root string,
	access privatecasport.AccessAuthority,
) (*Store, bool, error) {
	absolute, err := checkpointAuthorityRoot(root, access)
	if err != nil {
		return nil, false, err
	}
	layoutPresent, err := currentStoreTopology(ctx, absolute, access)
	if err != nil {
		return nil, false, err
	}
	if !layoutPresent {
		return nil, false, nil
	}
	names := []string{
		"snapshot-intents",
		"snapshot-completions",
		"snapshot-dispositions",
		"operation-group-intents-v2",
		"operation-group-terminals-v2",
	}
	stores := make([]*finalauthority.SecurePrivateCAS, len(names))
	present := make([]bool, len(names))
	presentCount := 0
	for index, name := range names {
		maxBytes := maxCheckpointAuthorityCASBytes
		if name == "operation-group-intents-v2" {
			maxBytes = domaincheckpoint.MaxOperationGroupIntentRecordBytes
		}
		stores[index], present[index], err = finalauthority.OpenExistingSecurePrivateCASWithAccessAuthorityContext(
			ctx, filepath.Join(absolute, name), maxBytes, access,
		)
		if err != nil {
			return nil, false, err
		}
		if present[index] {
			presentCount++
		}
	}
	if presentCount != len(names) {
		missing := make([]string, 0, len(names)-presentCount)
		for index, found := range present {
			if !found {
				missing = append(missing, names[index])
			}
		}
		if len(missing) == 0 {
			missing = append(missing, "all current CAS roots")
		}
		return nil, false, errors.Join(
			checkpointport.ErrCorrupt,
			fmt.Errorf("checkpoint authority layout is incomplete: missing %s", strings.Join(missing, ", ")),
		)
	}
	store := &Store{
		root: absolute, intentCAS: stores[0], completionCAS: stores[1], dispositionCAS: stores[2],
		operationIntentCAS: stores[3], operationTerminalCAS: stores[4],
	}
	if err := store.validateInventory(ctx); err != nil {
		return nil, false, err
	}
	return store, true, nil
}

func PreflightRecovery(ctx context.Context, root string, access privatecasport.RecoveryAccessAuthority) error {
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		return err
	}
	return prepared.Revalidate(ctx)
}

func currentStoreTopology(
	ctx context.Context,
	absolute string,
	access privatecasport.AccessAuthority,
) (bool, error) {
	rootPresent, err := finalauthority.SecurePrivateCASDirectoryPresentWithAccessAuthorityContext(ctx, absolute, access)
	if err != nil {
		return false, err
	}
	names := []string{
		"snapshot-intents", "snapshot-completions", "snapshot-dispositions",
		"operation-group-intents-v2", "operation-group-terminals-v2",
	}
	presentCount := 0
	missing := make([]string, 0, len(names))
	for _, name := range names {
		present, err := finalauthority.SecurePrivateCASDirectoryPresentWithAccessAuthorityContext(
			ctx, filepath.Join(absolute, name), access,
		)
		if err != nil {
			return false, err
		}
		if present {
			presentCount++
		} else {
			missing = append(missing, name)
		}
	}
	if !rootPresent && presentCount == 0 {
		return false, nil
	}
	if !rootPresent || presentCount != len(names) {
		if len(missing) == 0 {
			missing = append(missing, "checkpoint-authority container")
		}
		return false, errors.Join(
			checkpointport.ErrCorrupt,
			fmt.Errorf("checkpoint authority layout is incomplete: missing %s", strings.Join(missing, ", ")),
		)
	}
	return true, nil
}

func visitRecoveryRoots(
	ctx context.Context,
	root string,
	access privatecasport.AccessAuthority,
	visit func(context.Context, string, int, finalauthority.SecurePrivateCASAccessAuthority) error,
) error {
	absolute, err := checkpointAuthorityRoot(root, access)
	if err != nil {
		return err
	}
	for _, name := range []string{"snapshot-intents", "snapshot-completions", "snapshot-dispositions", "operation-group-intents-v2", "operation-group-terminals-v2"} {
		if err := visit(ctx, filepath.Join(absolute, name), maxCheckpointAuthorityCASBytes, access); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) BeginSnapshot(
	ctx context.Context,
	input domaincheckpoint.SnapshotIntentInputV1,
) (domaincheckpoint.SnapshotIntentV1, error) {
	if store == nil || store.intentCAS == nil {
		return domaincheckpoint.SnapshotIntentV1{}, errors.New("checkpoint snapshot authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intents, err := store.listIntents(ctx)
	if err != nil {
		return domaincheckpoint.SnapshotIntentV1{}, err
	}
	var maximum uint64
	for _, existing := range intents {
		if existing.SecurityContext.ThreadID != input.SecurityContext.ThreadID || existing.CheckpointID != strings.TrimSpace(input.CheckpointID) {
			continue
		}
		if existing.SecurityContext.ContextDigest != input.SecurityContext.ContextDigest ||
			existing.SecurityContext.TurnID != input.SecurityContext.TurnID ||
			existing.SecurityContext.WorkspaceRealPath != input.SecurityContext.WorkspaceRealPath {
			return domaincheckpoint.SnapshotIntentV1{}, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint id crosses a frozen turn security context"))
		}
		if existing.MutationOrdinal > maximum {
			maximum = existing.MutationOrdinal
		}
		if existing.ExecutionGrant.GrantID == input.ExecutionGrant.GrantID {
			if existing.ExecutionGrant.ToolCallID != input.ExecutionGrant.ToolCallID {
				return domaincheckpoint.SnapshotIntentV1{}, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint execution grant identity is inconsistent"))
			}
			if existing.RelativePath == strings.TrimSpace(input.RelativePath) {
				return existing, nil
			}
		}
	}
	input.MutationOrdinal = maximum + 1
	record, err := domaincheckpoint.NewSnapshotIntentV1(input)
	if err != nil {
		return domaincheckpoint.SnapshotIntentV1{}, err
	}
	body, err := domaincheckpoint.SnapshotIntentV1Bytes(record)
	if err != nil {
		return domaincheckpoint.SnapshotIntentV1{}, err
	}
	if writeErr := putExact(ctx, store.intentCAS, record.SnapshotIntentID, body); writeErr != nil {
		written, readErr := store.readIntent(ctx, record.SnapshotIntentID)
		writtenBody, bodyErr := domaincheckpoint.SnapshotIntentV1Bytes(written)
		if readErr == nil && bodyErr == nil && bytes.Equal(writtenBody, body) {
			return written, nil
		}
		return domaincheckpoint.SnapshotIntentV1{}, errors.Join(writeErr, readErr, bodyErr)
	}
	written, err := store.readIntent(ctx, record.SnapshotIntentID)
	if err != nil || written.SnapshotIntentID != record.SnapshotIntentID {
		return domaincheckpoint.SnapshotIntentV1{}, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot intent readback failed"), err)
	}
	return written, nil
}

func (store *Store) CompleteSnapshot(
	ctx context.Context,
	intentID string,
	afterExisted bool,
	afterHash string,
	completedAt time.Time,
) (domaincheckpoint.SnapshotCompletionV1, error) {
	if store == nil || store.completionCAS == nil {
		return domaincheckpoint.SnapshotCompletionV1{}, errors.New("checkpoint snapshot authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, err := store.readIntent(ctx, strings.TrimSpace(intentID))
	if err != nil {
		return domaincheckpoint.SnapshotCompletionV1{}, err
	}
	if _, found, err := store.readDisposition(ctx, intent); err != nil {
		return domaincheckpoint.SnapshotCompletionV1{}, err
	} else if found {
		return domaincheckpoint.SnapshotCompletionV1{}, errors.Join(checkpointport.ErrConflict, errors.New("aborted checkpoint snapshot cannot be completed"))
	}
	candidate, err := domaincheckpoint.NewSnapshotCompletionV1(domaincheckpoint.SnapshotCompletionInputV1{
		Intent: intent, AfterExisted: afterExisted, AfterHash: afterHash, CompletedAt: completedAt,
	})
	if err != nil {
		return domaincheckpoint.SnapshotCompletionV1{}, err
	}
	body, err := domaincheckpoint.SnapshotCompletionV1Bytes(candidate, intent)
	if err != nil {
		return domaincheckpoint.SnapshotCompletionV1{}, err
	}
	if existing, found, err := store.readCompletion(ctx, intent); err != nil {
		return domaincheckpoint.SnapshotCompletionV1{}, err
	} else if found {
		existingBody, _ := domaincheckpoint.SnapshotCompletionV1Bytes(existing, intent)
		if bytes.Equal(existingBody, body) {
			return existing, nil
		}
		return domaincheckpoint.SnapshotCompletionV1{}, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint snapshot already has a different completion"))
	}
	if writeErr := putExact(ctx, store.completionCAS, intent.SnapshotIntentID, body); writeErr != nil {
		written, found, readErr := store.readCompletion(ctx, intent)
		writtenBody, bodyErr := domaincheckpoint.SnapshotCompletionV1Bytes(written, intent)
		if found && readErr == nil && bodyErr == nil && bytes.Equal(writtenBody, body) {
			return written, nil
		}
		return domaincheckpoint.SnapshotCompletionV1{}, errors.Join(writeErr, readErr, bodyErr)
	}
	written, found, err := store.readCompletion(ctx, intent)
	if err != nil || !found || written.CompletionDigest != candidate.CompletionDigest {
		return domaincheckpoint.SnapshotCompletionV1{}, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot completion readback failed"), err)
	}
	return written, nil
}

func (store *Store) AbortSnapshot(
	ctx context.Context,
	intentID string,
	reasonCode string,
	closedAt time.Time,
) (domaincheckpoint.SnapshotDispositionV1, error) {
	if store == nil || store.dispositionCAS == nil {
		return domaincheckpoint.SnapshotDispositionV1{}, errors.New("checkpoint snapshot authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, err := store.readIntent(ctx, strings.TrimSpace(intentID))
	if err != nil {
		return domaincheckpoint.SnapshotDispositionV1{}, err
	}
	if _, found, err := store.readCompletion(ctx, intent); err != nil {
		return domaincheckpoint.SnapshotDispositionV1{}, err
	} else if found {
		return domaincheckpoint.SnapshotDispositionV1{}, errors.Join(checkpointport.ErrConflict, errors.New("completed checkpoint snapshot cannot be aborted"))
	}
	candidate, err := domaincheckpoint.NewSnapshotDispositionV1(domaincheckpoint.SnapshotDispositionInputV1{
		Intent: intent, ReasonCode: reasonCode, ClosedAt: closedAt,
	})
	if err != nil {
		return domaincheckpoint.SnapshotDispositionV1{}, err
	}
	body, err := domaincheckpoint.SnapshotDispositionV1Bytes(candidate, intent)
	if err != nil {
		return domaincheckpoint.SnapshotDispositionV1{}, err
	}
	if existing, found, err := store.readDisposition(ctx, intent); err != nil {
		return domaincheckpoint.SnapshotDispositionV1{}, err
	} else if found {
		existingBody, _ := domaincheckpoint.SnapshotDispositionV1Bytes(existing, intent)
		if bytes.Equal(existingBody, body) {
			return existing, nil
		}
		return domaincheckpoint.SnapshotDispositionV1{}, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint snapshot already has a different disposition"))
	}
	if writeErr := putExact(ctx, store.dispositionCAS, intent.SnapshotIntentID, body); writeErr != nil {
		written, found, readErr := store.readDisposition(ctx, intent)
		writtenBody, bodyErr := domaincheckpoint.SnapshotDispositionV1Bytes(written, intent)
		if found && readErr == nil && bodyErr == nil && bytes.Equal(writtenBody, body) {
			return written, nil
		}
		return domaincheckpoint.SnapshotDispositionV1{}, errors.Join(writeErr, readErr, bodyErr)
	}
	written, found, err := store.readDisposition(ctx, intent)
	if err != nil || !found || written.DispositionDigest != candidate.DispositionDigest {
		return domaincheckpoint.SnapshotDispositionV1{}, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot disposition readback failed"), err)
	}
	return written, nil
}

func (store *Store) ResolveCheckpoint(
	ctx context.Context,
	threadID string,
	checkpointID string,
) ([]domaincheckpoint.MaterializedSnapshotV1, error) {
	if store == nil || store.intentCAS == nil {
		return nil, errors.New("checkpoint snapshot authority is unavailable")
	}
	threadID = strings.TrimSpace(threadID)
	checkpointID = strings.TrimSpace(checkpointID)
	if threadID == "" || checkpointID == "" {
		return nil, errors.New("checkpoint snapshot lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intents, err := store.listIntents(ctx)
	if err != nil {
		return nil, err
	}
	matched := make([]domaincheckpoint.SnapshotIntentV1, 0)
	contextDigest := ""
	for _, intent := range intents {
		if intent.SecurityContext.ThreadID != threadID || intent.CheckpointID != checkpointID {
			continue
		}
		if contextDigest == "" {
			contextDigest = intent.SecurityContext.ContextDigest
		} else if contextDigest != intent.SecurityContext.ContextDigest {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot inventory crosses security contexts"))
		}
		matched = append(matched, intent)
	}
	if len(matched) == 0 {
		return nil, checkpointport.ErrNotFound
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].MutationOrdinal < matched[j].MutationOrdinal })
	resolved := make([]domaincheckpoint.MaterializedSnapshotV1, 0, len(matched))
	for index, intent := range matched {
		if intent.MutationOrdinal != uint64(index+1) {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot mutation ordinals are not contiguous"))
		}
		completion, completed, completionErr := store.readCompletion(ctx, intent)
		disposition, disposed, dispositionErr := store.readDisposition(ctx, intent)
		if completionErr != nil || dispositionErr != nil {
			return nil, errors.Join(checkpointport.ErrCorrupt, completionErr, dispositionErr)
		}
		if completed && disposed {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot has both completion and disposition"))
		}
		if disposed {
			if domaincheckpoint.ValidateSnapshotDispositionForIntentV1(disposition, intent) != nil {
				return nil, checkpointport.ErrCorrupt
			}
			continue
		}
		if !completed {
			return nil, errors.Join(checkpointport.ErrIncomplete, errors.New("checkpoint snapshot intent has no terminal authority"))
		}
		resolved = append(resolved, domaincheckpoint.MaterializedSnapshotV1{Intent: intent, Completion: completion})
	}
	return resolved, nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil || store.intentCAS == nil || store.operationIntentCAS == nil {
		return false, errors.New("checkpoint snapshot authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	count := 0
	err := store.operationIntentCAS.Visit(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		count++
		intent, parseErr := domaincheckpoint.ParseOperationGroupIntentV2(file.Body)
		if parseErr != nil || intent.OperationGroupID != file.Digest {
			return errors.Join(checkpointport.ErrCorrupt, parseErr)
		}
		return nil
	})
	if err != nil || count > 0 {
		return count > 0, err
	}
	err = store.intentCAS.Visit(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		count++
		if count > 1 {
			return nil
		}
		intent, err := domaincheckpoint.ParseSnapshotIntentV1(file.Body)
		if err != nil || intent.SnapshotIntentID != file.Digest {
			return errors.Join(checkpointport.ErrCorrupt, err)
		}
		return nil
	})
	return count > 0, err
}

func (store *Store) validateInventory(ctx context.Context) error {
	if err := store.validateOperationInventory(ctx); err != nil {
		return err
	}
	intents, err := store.listIntents(ctx)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		completion, completed, err := store.readCompletion(ctx, intent)
		if err != nil {
			return err
		}
		disposition, disposed, err := store.readDisposition(ctx, intent)
		if err != nil {
			return err
		}
		if completed && disposed {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot has conflicting terminal records"))
		}
		if completed && domaincheckpoint.ValidateSnapshotCompletionForIntentV1(completion, intent) != nil {
			return checkpointport.ErrCorrupt
		}
		if disposed && domaincheckpoint.ValidateSnapshotDispositionForIntentV1(disposition, intent) != nil {
			return checkpointport.ErrCorrupt
		}
	}
	return nil
}

func (store *Store) listIntents(ctx context.Context) ([]domaincheckpoint.SnapshotIntentV1, error) {
	files, err := store.intentCAS.List(ctx)
	if err != nil {
		return nil, err
	}
	intents := make([]domaincheckpoint.SnapshotIntentV1, 0, len(files))
	for _, file := range files {
		intent, err := domaincheckpoint.ParseSnapshotIntentV1(file.Body)
		if err != nil || intent.SnapshotIntentID != file.Digest {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint snapshot intent filename is invalid"), err)
		}
		intents = append(intents, intent)
	}
	return intents, nil
}

func (store *Store) readIntent(ctx context.Context, intentID string) (domaincheckpoint.SnapshotIntentV1, error) {
	body, err := store.intentCAS.Read(ctx, intentID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincheckpoint.SnapshotIntentV1{}, checkpointport.ErrNotFound
	}
	if err != nil {
		return domaincheckpoint.SnapshotIntentV1{}, err
	}
	intent, err := domaincheckpoint.ParseSnapshotIntentV1(body)
	if err != nil || intent.SnapshotIntentID != intentID {
		return domaincheckpoint.SnapshotIntentV1{}, errors.Join(checkpointport.ErrCorrupt, err)
	}
	return intent, nil
}

func (store *Store) readCompletion(ctx context.Context, intent domaincheckpoint.SnapshotIntentV1) (domaincheckpoint.SnapshotCompletionV1, bool, error) {
	body, err := store.completionCAS.Read(ctx, intent.SnapshotIntentID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincheckpoint.SnapshotCompletionV1{}, false, nil
	}
	if err != nil {
		return domaincheckpoint.SnapshotCompletionV1{}, false, err
	}
	record, err := domaincheckpoint.ParseSnapshotCompletionV1(body, intent)
	return record, err == nil, err
}

func (store *Store) readDisposition(ctx context.Context, intent domaincheckpoint.SnapshotIntentV1) (domaincheckpoint.SnapshotDispositionV1, bool, error) {
	body, err := store.dispositionCAS.Read(ctx, intent.SnapshotIntentID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincheckpoint.SnapshotDispositionV1{}, false, nil
	}
	if err != nil {
		return domaincheckpoint.SnapshotDispositionV1{}, false, err
	}
	record, err := domaincheckpoint.ParseSnapshotDispositionV1(body, intent)
	return record, err == nil, err
}

func putExact(ctx context.Context, store *finalauthority.SecurePrivateCAS, key string, body []byte) error {
	if err := store.PutIfAbsent(ctx, key, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.Read(ctx, key)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, body) {
			return checkpointport.ErrConflict
		}
	}
	written, err := store.Read(ctx, key)
	if err != nil || !bytes.Equal(written, body) {
		return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint authority write verification failed"), err)
	}
	return nil
}

func checkpointAuthorityRoot(root string, access privatecasport.AccessAuthority) (string, error) {
	if access == nil || root == "" || root != strings.TrimSpace(root) {
		return "", errors.New("checkpoint authority root or access authority is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return "", errors.New("checkpoint authority root is invalid")
	}
	return absolute, nil
}
