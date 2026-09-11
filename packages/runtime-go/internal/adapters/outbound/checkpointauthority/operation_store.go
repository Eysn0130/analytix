package checkpointauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
)

func (store *Store) BeginOperationGroup(
	ctx context.Context,
	input domaincheckpoint.OperationGroupIntentInputV2,
) (domaincheckpoint.OperationGroupIntentV2, *domaincheckpoint.OperationGroupTerminalV2, bool, error) {
	if store == nil || store.operationIntentCAS == nil || store.operationTerminalCAS == nil {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, errors.New("checkpoint operation group authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intents, err := store.listOperationIntents(ctx)
	if err != nil {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, err
	}
	var maximum uint64
	for _, existing := range intents {
		if existing.SecurityContext.ThreadID != input.SecurityContext.ThreadID || existing.CheckpointID != strings.TrimSpace(input.CheckpointID) {
			continue
		}
		if existing.SecurityContext != input.SecurityContext || existing.SourceWorkspaceCheckpointID != strings.TrimSpace(input.SourceWorkspaceCheckpointID) {
			return domaincheckpoint.OperationGroupIntentV2{}, nil, false, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint operation group crosses its frozen authority"))
		}
		if existing.OperationOrdinal > maximum {
			maximum = existing.OperationOrdinal
		}
	}
	input.OperationOrdinal = maximum + 1
	candidate, err := domaincheckpoint.NewOperationGroupIntentV2(input)
	if err != nil {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, err
	}
	if existing, found, readErr := store.readOperationIntent(ctx, candidate.OperationGroupID); readErr != nil {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, readErr
	} else if found {
		if !operationIntentInputMatches(existing, input) {
			return domaincheckpoint.OperationGroupIntentV2{}, nil, false, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint operation group identity was reused with different paths or state"))
		}
		terminal, terminalFound, terminalErr := store.readOperationTerminal(ctx, existing)
		if terminalErr != nil {
			return domaincheckpoint.OperationGroupIntentV2{}, nil, false, terminalErr
		}
		if terminalFound {
			return existing, &terminal, true, nil
		}
		return existing, nil, true, nil
	}
	body, err := domaincheckpoint.OperationGroupIntentV2Bytes(candidate)
	if err != nil {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, err
	}
	if writeErr := putExact(ctx, store.operationIntentCAS, candidate.OperationGroupID, body); writeErr != nil {
		written, found, readErr := store.readOperationIntent(ctx, candidate.OperationGroupID)
		writtenBody, bodyErr := domaincheckpoint.OperationGroupIntentV2Bytes(written)
		if found && readErr == nil && bodyErr == nil && bytes.Equal(writtenBody, body) {
			return written, nil, false, nil
		}
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, errors.Join(writeErr, readErr, bodyErr)
	}
	written, found, err := store.readOperationIntent(ctx, candidate.OperationGroupID)
	if err != nil || !found || written.IntentDigest != candidate.IntentDigest {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint operation group intent readback failed"), err)
	}
	return written, nil, false, nil
}

func (store *Store) SettleOperationGroup(
	ctx context.Context,
	operationGroupID string,
	status string,
	reasonCode string,
	observed []domaincheckpoint.ObservedOperationPathV2,
	settledAt time.Time,
) (domaincheckpoint.OperationGroupTerminalV2, error) {
	terminal, _, _, err := store.settleOperationGroup(ctx, operationGroupID, status, reasonCode, observed, settledAt, false)
	return terminal, err
}

func (store *Store) SettleOperationGroupForRecovery(
	ctx context.Context,
	operationGroupID string,
	status string,
	reasonCode string,
	observed []domaincheckpoint.ObservedOperationPathV2,
	settledAt time.Time,
) (domaincheckpoint.OperationGroupTerminalV2, finalauthority.SecurePrivateCASAdditionReceiptV2, bool, error) {
	return store.settleOperationGroup(ctx, operationGroupID, status, reasonCode, observed, settledAt, true)
}

func (store *Store) settleOperationGroup(
	ctx context.Context,
	operationGroupID string,
	status string,
	reasonCode string,
	observed []domaincheckpoint.ObservedOperationPathV2,
	settledAt time.Time,
	issueRecoveryReceipt bool,
) (domaincheckpoint.OperationGroupTerminalV2, finalauthority.SecurePrivateCASAdditionReceiptV2, bool, error) {
	if store == nil || store.operationIntentCAS == nil || store.operationTerminalCAS == nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, errors.New("checkpoint operation group authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intent, found, err := store.readOperationIntent(ctx, strings.TrimSpace(operationGroupID))
	if err != nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, err
	}
	if !found {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, checkpointport.ErrNotFound
	}
	candidate, err := domaincheckpoint.NewOperationGroupTerminalV2(domaincheckpoint.OperationGroupTerminalInputV2{
		Intent: intent, Status: status, ReasonCode: reasonCode, ObservedPaths: observed, SettledAt: settledAt,
	})
	if err != nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, err
	}
	if existing, found, err := store.readOperationTerminal(ctx, intent); err != nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, err
	} else if found {
		if domaincheckpoint.TerminalSemanticEqual(existing, candidate) {
			return existing, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, nil
		}
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, errors.Join(checkpointport.ErrConflict, errors.New("checkpoint operation group already has a different terminal"))
	}
	body, err := domaincheckpoint.OperationGroupTerminalV2Bytes(candidate, intent)
	if err != nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, err
	}
	receipt := finalauthority.SecurePrivateCASAdditionReceiptV2{}
	writeErr := error(nil)
	if issueRecoveryReceipt {
		receipt, writeErr = store.operationTerminalCAS.PutIfAbsentWithAdditionReceipt(ctx, intent.OperationGroupID, body)
	} else {
		writeErr = putExact(ctx, store.operationTerminalCAS, intent.OperationGroupID, body)
	}
	if writeErr != nil {
		written, found, readErr := store.readOperationTerminal(ctx, intent)
		if issueRecoveryReceipt && errors.Is(writeErr, os.ErrExist) && found && readErr == nil && domaincheckpoint.TerminalSemanticEqual(written, candidate) {
			return written, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, nil
		}
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, errors.Join(writeErr, readErr)
	}
	written, found, err := store.readOperationTerminal(ctx, intent)
	if err != nil || !found || written.TerminalDigest != candidate.TerminalDigest {
		return domaincheckpoint.OperationGroupTerminalV2{}, finalauthority.SecurePrivateCASAdditionReceiptV2{}, false, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint operation group terminal readback failed"), err)
	}
	return written, receipt, issueRecoveryReceipt, nil
}

func (store *Store) VerifyOperationTerminalAddition(
	ctx context.Context,
	receipt finalauthority.SecurePrivateCASAdditionReceiptV2,
) error {
	if store == nil || store.operationTerminalCAS == nil {
		return errors.New("checkpoint operation terminal authority is unavailable")
	}
	return store.operationTerminalCAS.VerifyCommittedAddition(ctx, receipt)
}

func (store *Store) FinalizeOperationTerminalAdditions(
	ctx context.Context,
	provisional []finalauthority.SecurePrivateCASAdditionReceiptV2,
) ([]finalauthority.SecurePrivateCASAdditionReceiptV2, error) {
	if store == nil || store.operationTerminalCAS == nil {
		return nil, errors.New("checkpoint operation terminal authority is unavailable")
	}
	return store.operationTerminalCAS.FinalizeCommittedAdditions(ctx, provisional)
}

func (store *Store) ResolveOperationGroups(ctx context.Context, threadID, checkpointID string) ([]domaincheckpoint.MaterializedOperationGroupV2, error) {
	if store == nil || store.operationIntentCAS == nil || store.operationTerminalCAS == nil {
		return nil, errors.New("checkpoint operation group authority is unavailable")
	}
	threadID = strings.TrimSpace(threadID)
	checkpointID = strings.TrimSpace(checkpointID)
	if threadID == "" || checkpointID == "" {
		return nil, errors.New("checkpoint operation group lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intents, err := store.listOperationIntents(ctx)
	if err != nil {
		return nil, err
	}
	matched := make([]domaincheckpoint.OperationGroupIntentV2, 0)
	var frozenContext string
	var sourceCheckpointID string
	for _, intent := range intents {
		if intent.SecurityContext.ThreadID != threadID || intent.CheckpointID != checkpointID {
			continue
		}
		if frozenContext == "" {
			frozenContext = intent.SecurityContext.ContextDigest
			sourceCheckpointID = intent.SourceWorkspaceCheckpointID
		} else if frozenContext != intent.SecurityContext.ContextDigest || sourceCheckpointID != intent.SourceWorkspaceCheckpointID {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint operation groups cross frozen authority"))
		}
		matched = append(matched, intent)
	}
	if len(matched) == 0 {
		return nil, checkpointport.ErrNotFound
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].OperationOrdinal < matched[j].OperationOrdinal })
	resolved := make([]domaincheckpoint.MaterializedOperationGroupV2, 0, len(matched))
	for index, intent := range matched {
		if intent.OperationOrdinal != uint64(index+1) {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint operation group ordinals are not contiguous"))
		}
		terminal, found, err := store.readOperationTerminal(ctx, intent)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.Join(checkpointport.ErrIncomplete, errors.New("checkpoint operation group has no terminal"))
		}
		switch terminal.Status {
		case "completed":
			resolved = append(resolved, domaincheckpoint.MaterializedOperationGroupV2{Intent: intent, Terminal: terminal})
		case "no_effect":
		case "quarantined":
			return nil, errors.Join(checkpointport.ErrQuarantined, errors.New("checkpoint operation group filesystem state is quarantined"))
		default:
			return nil, checkpointport.ErrCorrupt
		}
	}
	return resolved, nil
}

func (store *Store) OpenOperationGroups(ctx context.Context) ([]domaincheckpoint.OperationGroupIntentV2, error) {
	if store == nil || store.operationIntentCAS == nil || store.operationTerminalCAS == nil {
		return nil, errors.New("checkpoint operation group authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	intents, err := store.listOperationIntents(ctx)
	if err != nil {
		return nil, err
	}
	open := make([]domaincheckpoint.OperationGroupIntentV2, 0)
	for _, intent := range intents {
		_, found, err := store.readOperationTerminal(ctx, intent)
		if err != nil {
			return nil, err
		}
		if !found {
			open = append(open, intent)
		}
	}
	sort.Slice(open, func(i, j int) bool {
		if open[i].SecurityContext.ThreadID != open[j].SecurityContext.ThreadID {
			return open[i].SecurityContext.ThreadID < open[j].SecurityContext.ThreadID
		}
		if open[i].CheckpointID != open[j].CheckpointID {
			return open[i].CheckpointID < open[j].CheckpointID
		}
		return open[i].OperationOrdinal < open[j].OperationOrdinal
	})
	return open, nil
}

func (store *Store) ListOperationGroups(ctx context.Context) ([]domaincheckpoint.OperationGroupStateV2, error) {
	if store == nil || store.operationIntentCAS == nil || store.operationTerminalCAS == nil {
		return nil, errors.New("checkpoint operation group authority is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.validateOperationInventory(ctx); err != nil {
		return nil, err
	}
	intents, err := store.listOperationIntents(ctx)
	if err != nil {
		return nil, err
	}
	states := make([]domaincheckpoint.OperationGroupStateV2, 0, len(intents))
	for _, intent := range intents {
		terminal, found, err := store.readOperationTerminal(ctx, intent)
		if err != nil {
			return nil, err
		}
		state := domaincheckpoint.OperationGroupStateV2{Intent: intent}
		if found {
			terminalCopy := terminal
			state.Terminal = &terminalCopy
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool {
		if states[i].Intent.SecurityContext.ThreadID != states[j].Intent.SecurityContext.ThreadID {
			return states[i].Intent.SecurityContext.ThreadID < states[j].Intent.SecurityContext.ThreadID
		}
		if states[i].Intent.CheckpointID != states[j].Intent.CheckpointID {
			return states[i].Intent.CheckpointID < states[j].Intent.CheckpointID
		}
		return states[i].Intent.OperationOrdinal < states[j].Intent.OperationOrdinal
	})
	return states, nil
}

func (store *Store) validateOperationInventory(ctx context.Context) error {
	if store == nil || store.operationIntentCAS == nil || store.operationTerminalCAS == nil {
		return errors.New("checkpoint operation group authority is unavailable")
	}
	intents, err := store.listOperationIntents(ctx)
	if err != nil {
		return err
	}
	byID := make(map[string]domaincheckpoint.OperationGroupIntentV2, len(intents))
	for _, intent := range intents {
		if _, duplicate := byID[intent.OperationGroupID]; duplicate {
			return checkpointport.ErrCorrupt
		}
		byID[intent.OperationGroupID] = intent
		if _, _, err := store.readOperationTerminal(ctx, intent); err != nil {
			return err
		}
	}
	return store.operationTerminalCAS.Visit(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		intent, found := byID[file.Digest]
		if !found {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("orphan checkpoint operation terminal"))
		}
		terminal, err := domaincheckpoint.ParseOperationGroupTerminalV2(file.Body, intent)
		if err != nil || terminal.OperationGroupID != file.Digest {
			return errors.Join(checkpointport.ErrCorrupt, err)
		}
		return nil
	})
}

func (store *Store) listOperationIntents(ctx context.Context) ([]domaincheckpoint.OperationGroupIntentV2, error) {
	files, err := store.operationIntentCAS.List(ctx)
	if err != nil {
		return nil, err
	}
	intents := make([]domaincheckpoint.OperationGroupIntentV2, 0, len(files))
	for _, file := range files {
		intent, err := domaincheckpoint.ParseOperationGroupIntentV2(file.Body)
		if err != nil || intent.OperationGroupID != file.Digest {
			return nil, errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint operation group intent filename is invalid"), err)
		}
		intents = append(intents, intent)
	}
	return intents, nil
}

func (store *Store) readOperationIntent(ctx context.Context, groupID string) (domaincheckpoint.OperationGroupIntentV2, bool, error) {
	body, err := store.operationIntentCAS.Read(ctx, groupID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincheckpoint.OperationGroupIntentV2{}, false, nil
	}
	if err != nil {
		return domaincheckpoint.OperationGroupIntentV2{}, false, err
	}
	intent, err := domaincheckpoint.ParseOperationGroupIntentV2(body)
	if err != nil || intent.OperationGroupID != groupID {
		return domaincheckpoint.OperationGroupIntentV2{}, false, errors.Join(checkpointport.ErrCorrupt, err)
	}
	return intent, true, nil
}

func (store *Store) readOperationTerminal(ctx context.Context, intent domaincheckpoint.OperationGroupIntentV2) (domaincheckpoint.OperationGroupTerminalV2, bool, error) {
	body, err := store.operationTerminalCAS.Read(ctx, intent.OperationGroupID)
	if errors.Is(err, os.ErrNotExist) {
		return domaincheckpoint.OperationGroupTerminalV2{}, false, nil
	}
	if err != nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, false, err
	}
	terminal, err := domaincheckpoint.ParseOperationGroupTerminalV2(body, intent)
	return terminal, err == nil, err
}

func operationIntentInputMatches(existing domaincheckpoint.OperationGroupIntentV2, input domaincheckpoint.OperationGroupIntentInputV2) bool {
	createdAt, err := time.Parse(time.RFC3339Nano, existing.CreatedAt)
	if err != nil {
		return false
	}
	input.OperationOrdinal = existing.OperationOrdinal
	input.CreatedAt = createdAt
	candidate, err := domaincheckpoint.NewOperationGroupIntentV2(input)
	return err == nil && candidate.IntentDigest == existing.IntentDigest
}
