package checkpoint

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type BeginOperationGroupAuthorityInput struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	ExecutionGrant              domainsecurity.ExecutionGrant
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	ToolName                    string
	ArgumentsJSON               []byte
	Paths                       []domaincheckpoint.OperationPathInputV2
	CreatedAt                   time.Time
}

type CapturedOperationAuditV2 struct {
	CaptureEventID string
	Event          map[string]any
}

type CapturedOperationInventoryV2 struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	Audits                      []CapturedOperationAuditV2
}

func (authority SnapshotAuthority) BeginOperationGroup(
	ctx context.Context,
	input BeginOperationGroupAuthorityInput,
) (domaincheckpoint.OperationGroupIntentV2, *domaincheckpoint.OperationGroupTerminalV2, bool, error) {
	if authority.Store == nil {
		return domaincheckpoint.OperationGroupIntentV2{}, nil, false, errors.New("checkpoint operation group authority is unavailable")
	}
	return authority.Store.BeginOperationGroup(ctx, domaincheckpoint.OperationGroupIntentInputV2{
		SecurityContext: input.SecurityContext, ExecutionGrant: input.ExecutionGrant,
		CheckpointID: input.CheckpointID, SourceWorkspaceCheckpointID: input.SourceWorkspaceCheckpointID,
		ToolName: input.ToolName, ArgumentsJSON: input.ArgumentsJSON, Paths: input.Paths, CreatedAt: input.CreatedAt,
	})
}

func (authority SnapshotAuthority) SettleOperationGroup(
	ctx context.Context,
	operationGroupID string,
	status string,
	reasonCode string,
	observed []domaincheckpoint.ObservedOperationPathV2,
	settledAt time.Time,
) (domaincheckpoint.OperationGroupTerminalV2, error) {
	if authority.Store == nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, errors.New("checkpoint operation group authority is unavailable")
	}
	return authority.Store.SettleOperationGroup(ctx, operationGroupID, status, reasonCode, observed, settledAt)
}

func (authority SnapshotAuthority) SettleOperationGroupForRecovery(
	ctx context.Context,
	operationGroupID string,
	status string,
	reasonCode string,
	observed []domaincheckpoint.ObservedOperationPathV2,
	settledAt time.Time,
) (domaincheckpoint.OperationGroupTerminalV2, privatecasport.AdditionReceiptV2, bool, error) {
	store, ok := authority.Store.(checkpointport.RecoveryStore)
	if !ok || store == nil {
		return domaincheckpoint.OperationGroupTerminalV2{}, privatecasport.AdditionReceiptV2{}, false, errors.New("checkpoint operation recovery authority is unavailable")
	}
	return store.SettleOperationGroupForRecovery(ctx, operationGroupID, status, reasonCode, observed, settledAt)
}

func (authority SnapshotAuthority) OpenOperationGroups(ctx context.Context) ([]domaincheckpoint.OperationGroupIntentV2, error) {
	if authority.Store == nil {
		return nil, errors.New("checkpoint operation group authority is unavailable")
	}
	return authority.Store.OpenOperationGroups(ctx)
}

func (authority SnapshotAuthority) OperationGroups(ctx context.Context) ([]domaincheckpoint.OperationGroupStateV2, error) {
	if authority.Store == nil {
		return nil, errors.New("checkpoint operation group authority is unavailable")
	}
	return authority.Store.ListOperationGroups(ctx)
}

// CapturedOperationInventory materializes one deterministic audit event for
// every completed operation prefix. A later operation on the same checkpoint
// therefore cannot be hidden by an older checkpoint_captured event.
func (authority SnapshotAuthority) CapturedOperationInventory(
	ctx context.Context,
	threadID string,
	checkpointID string,
) (CapturedOperationInventoryV2, error) {
	return authority.capturedOperationInventory(ctx, threadID, checkpointID, nil)
}

// CapturedOperationInventoryForPreservedContextV1 validates the full original
// group sequence but observes audits only from its already settled prefix.
// Open or blocked suffixes remain part of the validated inventory; this view
// neither settles them nor authorizes a new capture event.
func (authority SnapshotAuthority) CapturedOperationInventoryForPreservedContextV1(ctx context.Context, frozen domainsecurity.TurnSecurityContext, checkpointID string) (CapturedOperationInventoryV2, error) {
	if domainsecurity.ValidateTurnSecurityContext(frozen) != nil || frozen.Version != domainsecurity.TurnSecurityContextVersionV2 {
		return CapturedOperationInventoryV2{}, errors.New("checkpoint preserved capture context is invalid")
	}
	return authority.capturedOperationInventory(ctx, frozen.ThreadID, checkpointID, &frozen)
}

func (authority SnapshotAuthority) capturedOperationInventory(ctx context.Context, threadID, checkpointID string, preserved *domainsecurity.TurnSecurityContext) (CapturedOperationInventoryV2, error) {
	if authority.Store == nil {
		return CapturedOperationInventoryV2{}, errors.New("checkpoint operation group authority is unavailable")
	}
	threadID = strings.TrimSpace(threadID)
	checkpointID = strings.TrimSpace(checkpointID)
	states, err := authority.Store.ListOperationGroups(ctx)
	if err != nil {
		return CapturedOperationInventoryV2{}, err
	}
	matched := make([]domaincheckpoint.OperationGroupStateV2, 0)
	for _, state := range states {
		if state.Intent.SecurityContext.ThreadID == threadID && state.Intent.CheckpointID == checkpointID {
			matched = append(matched, state)
		}
	}
	if len(matched) == 0 {
		return CapturedOperationInventoryV2{}, checkpointport.ErrNotFound
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Intent.OperationOrdinal < matched[j].Intent.OperationOrdinal
	})
	if err := validateOperationGroupSequenceV1(matched); err != nil {
		return CapturedOperationInventoryV2{}, err
	}
	first := matched[0].Intent
	end := len(matched)
	if preserved == nil {
		if _, _, err := domaincheckpoint.NewOperationGroupCaptureFrontierV2(matched); err != nil {
			return CapturedOperationInventoryV2{}, err
		}
	} else {
		if first.SecurityContext != *preserved {
			return CapturedOperationInventoryV2{}, errors.New("checkpoint preserved capture lost its original context")
		}
		for index, state := range matched {
			if state.Terminal == nil || (state.Terminal.Status != "completed" && state.Terminal.Status != "no_effect") {
				end = index
				break
			}
		}
	}
	inventory := CapturedOperationInventoryV2{
		SecurityContext: first.SecurityContext, CheckpointID: first.CheckpointID,
		SourceWorkspaceCheckpointID: first.SourceWorkspaceCheckpointID,
		Audits:                      make([]CapturedOperationAuditV2, 0, end),
	}
	for index, state := range matched[:end] {
		if state.Terminal == nil || state.Terminal.Status != "completed" {
			continue
		}
		prefix := matched[:index+1]
		frontier, hasCompleted, frontierErr := domaincheckpoint.NewOperationGroupCaptureFrontierV2(prefix)
		if frontierErr != nil || !hasCompleted {
			return CapturedOperationInventoryV2{}, errors.Join(errors.New("checkpoint operation capture prefix is invalid"), frontierErr)
		}
		materialized := make([]domaincheckpoint.MaterializedOperationGroupV2, 0, frontier.CompletedOperationGroupCount)
		for _, prefixState := range prefix {
			if prefixState.Terminal != nil && prefixState.Terminal.Status == "completed" {
				materialized = append(materialized, domaincheckpoint.MaterializedOperationGroupV2{
					Intent: prefixState.Intent, Terminal: *prefixState.Terminal,
				})
			}
		}
		records, recordErr := operationGroupsToRecords(materialized)
		if recordErr != nil {
			return CapturedOperationInventoryV2{}, recordErr
		}
		event, ok := BuildCapturedCheckpointEvent(CapturedCheckpointInput{
			ThreadID: frontier.SecurityContext.ThreadID, TurnID: frontier.SecurityContext.TurnID,
			CheckpointID: frontier.CheckpointID, SourceWorkspaceCheckpointID: frontier.SourceWorkspaceCheckpointID,
			WorkspaceFallback: frontier.SecurityContext.WorkspaceRealPath, CreatedAtFallback: frontier.CreatedAt,
			Records: records, CaptureEventID: frontier.CaptureEventID,
		})
		if !ok {
			return CapturedOperationInventoryV2{}, errors.New("checkpoint operation captured audit is invalid")
		}
		checkpoint, _ := event["checkpoint"].(map[string]any)
		if checkpoint == nil || !domaincheckpointref.CapturedPayloadDigestMatches(checkpoint) {
			return CapturedOperationInventoryV2{}, errors.New("checkpoint operation captured audit digest is invalid")
		}
		inventory.Audits = append(inventory.Audits, CapturedOperationAuditV2{
			CaptureEventID: frontier.CaptureEventID, Event: event,
		})
	}
	return inventory, nil
}

// Callers sort a complete checkpoint sequence before using this pure check.
// Unlike a capture frontier, valid original open groups supply no new event.
func validateOperationGroupSequenceV1(states []domaincheckpoint.OperationGroupStateV2) error {
	if len(states) == 0 {
		return errors.New("checkpoint operation sequence is empty")
	}
	first := states[0].Intent
	for index, state := range states {
		intent := state.Intent
		if domaincheckpoint.ValidateOperationGroupIntentV2(intent) != nil || intent.OperationOrdinal != uint64(index+1) ||
			intent.SecurityContext != first.SecurityContext || intent.CheckpointID != first.CheckpointID || intent.SourceWorkspaceCheckpointID != first.SourceWorkspaceCheckpointID ||
			(state.Terminal != nil && domaincheckpoint.ValidateOperationGroupTerminalForIntentV2(*state.Terminal, intent) != nil) {
			return errors.New("checkpoint original operation sequence is invalid")
		}
	}
	return nil
}
