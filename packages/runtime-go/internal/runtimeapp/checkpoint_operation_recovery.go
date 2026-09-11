package runtimeapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	startupapp "analytix.local/runtime-go/internal/app/startup"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	"analytix.local/runtime-go/internal/server"
)

type checkpointRecoveryEventStore interface {
	AllThreadIDs() ([]string, error)
	GetThread(threadID string) (map[string]any, error)
	LoadEventsSince(threadID string, afterSeq int) (server.DurableLoadEventsResult, error)
	ReconcileCheckpointCapturedEventForStartup(draft map[string]any, allowWrite bool) error
}

type checkpointRecoveryCaseAuthority interface {
	IsCaseThread(threadID string) bool
	ContainsContext(domainsecurity.TurnSecurityContext) bool
}

type checkpointOriginalCaseContextObserverV1 interface {
	ObserveOriginalContextForRestartV1(context.Context, domainsecurity.TurnSecurityContext) error
}

type expectedCheckpointCaptureAudit struct {
	threadID        string
	securityContext domainsecurity.TurnSecurityContext
	event           map[string]any
}

type preparedLiveCheckpointOperationRecovery struct {
	authority        checkpointapp.SnapshotAuthority
	service          checkpointapp.OperationService
	store            *checkpointauthority.Store
	present          bool
	restartPreserved *pendingworkapp.ReportRestartScopeV1
}

type liveCheckpointRecoveryDelta struct {
	Managed  startupapp.AuthorizedManagedDeltaV1
	receipts []finalauthority.SecurePrivateCASAdditionReceiptV2
}

func (delta liveCheckpointRecoveryDelta) HasChanges() bool {
	return delta.Managed.HasChanges()
}

// recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1 completes
// only already-authenticated checkpoint transactions. Its before/after reads
// are recovery preflight evidence, never a semantic planning baseline. The
// caller must start its first ReadOnlyPlanningSessionV1 only after this helper
// returns and must retain Verify as a post-apply identity guard.
func recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1(
	ctx context.Context,
	reader startupport.SnapshotReader,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	allowWriteRoots []string,
	observedAt time.Time,
	preserved *pendingworkapp.ReportRestartScopeV1,
) (preparedLiveCheckpointOperationRecovery, liveCheckpointRecoveryDelta, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil || observedAt.IsZero() {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery prebaseline input is invalid")
	}
	before, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	if err := domainstartup.ValidateManagedSnapshotV1(before); err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	present, err := checkpointAuthorityPresentInManagedSnapshot(before)
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	prepared, err := prepareLiveCheckpointOperationRecovery(ctx, dataDir, access, present, allowWriteRoots, preserved)
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	delta, err := prepared.Recover(ctx, observedAt.UTC())
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	if err := prepared.Verify(ctx, delta); err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	after, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	if err := domainstartup.ValidateManagedSnapshotV1(after); err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	if delta.HasChanges() {
		if err := startupapp.ValidateAuthorizedManagedDeltaV1(before, after, delta.Managed); err != nil {
			return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
		}
	} else if before.RootBindingDigest != after.RootBindingDigest ||
		before.RawCaptureDigest != after.RawCaptureDigest || before.SnapshotDigest != after.SnapshotDigest {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery changed managed persistence without an authorized delta")
	}
	if err := prepared.Verify(ctx, delta); err != nil {
		return preparedLiveCheckpointOperationRecovery{}, liveCheckpointRecoveryDelta{}, err
	}
	return prepared, delta, nil
}

// prepareLiveCheckpointOperationRecovery pins and validates the existing
// authority without observing the workspace or writing a terminal. It is used
// only by authenticated prebaseline recovery; it never authorizes mutation
// after the semantic baseline has been sealed.
func prepareLiveCheckpointOperationRecovery(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	expectedPresent bool,
	allowWriteRoots []string,
	preserved *pendingworkapp.ReportRestartScopeV1,
) (preparedLiveCheckpointOperationRecovery, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return preparedLiveCheckpointOperationRecovery{}, err
	}
	if strings.TrimSpace(dataDir) == "" || access == nil {
		return preparedLiveCheckpointOperationRecovery{}, errors.New("live checkpoint operation recovery authority is unavailable")
	}
	if preserved != nil {
		if err := preserved.RevalidatePrimary(ctx); err != nil {
			return preparedLiveCheckpointOperationRecovery{}, err
		}
	}
	store, present, err := checkpointauthority.OpenExistingStoreContext(ctx,
		filepath.Join(dataDir, "private", "checkpoint-authority"), access,
	)
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, err
	}
	if present != expectedPresent {
		return preparedLiveCheckpointOperationRecovery{}, errors.New("checkpoint authority presence changed after managed startup preflight")
	}
	if !present {
		return preparedLiveCheckpointOperationRecovery{}, nil
	}
	authority := checkpointapp.SnapshotAuthority{Store: store}
	mutationAuthority, _, err := filestore.OpenExistingConditionalMutationAuthority(
		filepath.Join(dataDir, "file-mutation-quarantine-v1"),
	)
	if err != nil {
		return preparedLiveCheckpointOperationRecovery{}, err
	}
	observer := filestore.CheckpointOperationObserver{
		AllowWriteRoots: allowWriteRoots, MutationAuthority: mutationAuthority,
	}
	service := checkpointapp.OperationService{Authority: authority, Observer: observer, Recovery: observer}
	if preserved != nil && len(preserved.ThreadIDs()) != 0 {
		service, err = checkpointapp.NewOperationServiceWithRestartPreservationV1(ctx, service, preserved.Contexts())
		if err != nil {
			return preparedLiveCheckpointOperationRecovery{}, err
		}
	}
	return preparedLiveCheckpointOperationRecovery{
		authority:        authority,
		service:          service,
		store:            store,
		present:          true,
		restartPreserved: preserved,
	}, nil
}

func (prepared preparedLiveCheckpointOperationRecovery) Recover(
	ctx context.Context,
	observedAt time.Time,
) (liveCheckpointRecoveryDelta, error) {
	if !prepared.present {
		return liveCheckpointRecoveryDelta{}, nil
	}
	if !prepared.authority.Available() || prepared.service.Observer == nil || prepared.store == nil || observedAt.IsZero() {
		return liveCheckpointRecoveryDelta{}, errors.New("prepared live checkpoint operation recovery is unavailable")
	}
	semanticPlan, err := prepared.service.PrepareRecoverOpen(ctx)
	if err != nil {
		return liveCheckpointRecoveryDelta{}, fmt.Errorf("plan live checkpoint operation recovery: %w", err)
	}
	if prepared.restartPreserved != nil {
		if err := prepared.restartPreserved.RevalidatePrimary(ctx); err != nil {
			return liveCheckpointRecoveryDelta{}, err
		}
	}
	recovered, err := semanticPlan.Apply(ctx, observedAt.UTC())
	if err != nil {
		return liveCheckpointRecoveryDelta{}, fmt.Errorf("recover live checkpoint operation groups: %w", err)
	}
	provisional := make([]finalauthority.SecurePrivateCASAdditionReceiptV2, 0, len(recovered))
	for _, result := range recovered {
		if result.Created {
			if !result.Receipt.CreatedByThisCall || result.Receipt.Finalized {
				return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery commit provenance is invalid")
			}
			provisional = append(provisional, result.Receipt)
		} else if result.Receipt != (finalauthority.SecurePrivateCASAdditionReceiptV2{}) {
			return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery returned a receipt for an existing terminal")
		}
	}
	if len(provisional) != 0 {
		finalized, finalizeErr := prepared.store.FinalizeOperationTerminalAdditions(ctx, provisional)
		if finalizeErr != nil || len(finalized) != len(provisional) {
			return liveCheckpointRecoveryDelta{}, errors.Join(errors.New("finalize checkpoint recovery commit provenance"), finalizeErr)
		}
		byRecord := make(map[string]finalauthority.SecurePrivateCASAdditionReceiptV2, len(finalized))
		for _, receipt := range finalized {
			byRecord[receipt.RecordDigest] = receipt
		}
		for index := range recovered {
			if recovered[index].Created {
				receipt, found := byRecord[recovered[index].Intent.OperationGroupID]
				if !found {
					return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery finalized manifest omitted a created terminal")
				}
				recovered[index].Receipt = receipt
			}
		}
	}
	if _, err := validatedCheckpointOperationInventories(ctx, prepared.authority, prepared.restartPreserved); err != nil {
		return liveCheckpointRecoveryDelta{}, fmt.Errorf("validate live checkpoint operation inventory: %w", err)
	}
	delta, err := checkpointRecoveryAuthorizedDelta(ctx, prepared.store, recovered)
	if err != nil {
		return liveCheckpointRecoveryDelta{}, err
	}
	return delta, nil
}

func (prepared preparedLiveCheckpointOperationRecovery) Verify(
	ctx context.Context,
	delta liveCheckpointRecoveryDelta,
) error {
	if prepared.restartPreserved != nil {
		if err := prepared.restartPreserved.RevalidatePrimary(ctx); err != nil {
			return err
		}
	}
	if !delta.HasChanges() {
		return nil
	}
	if !prepared.present || prepared.store == nil || len(delta.receipts) != len(delta.Managed.AddedFiles) {
		return errors.New("checkpoint recovery addition receipts are unavailable")
	}
	for _, receipt := range delta.receipts {
		if err := prepared.store.VerifyOperationTerminalAddition(ctx, receipt); err != nil {
			return fmt.Errorf("verify checkpoint recovery terminal addition: %w", err)
		}
	}
	return nil
}

func checkpointRecoveryAuthorizedDelta(
	ctx context.Context,
	store *checkpointauthority.Store,
	recovered []checkpointapp.OperationRecoveryResult,
) (liveCheckpointRecoveryDelta, error) {
	if len(recovered) == 0 {
		return liveCheckpointRecoveryDelta{}, nil
	}
	if store == nil {
		return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery terminal store is unavailable")
	}
	const terminalRoot = "data/private/checkpoint-authority/operation-group-terminals-v2"
	delta := liveCheckpointRecoveryDelta{
		Managed: startupapp.AuthorizedManagedDeltaV1{
			AddedFiles: make([]startupapp.AuthorizedManagedFileAdditionV1, 0, len(recovered)),
		},
		receipts: make([]finalauthority.SecurePrivateCASAdditionReceiptV2, 0, len(recovered)),
	}
	shards := map[string]startupapp.AuthorizedManagedDirectoryV1{}
	shardIdentities := map[string]string{}
	var rootDirectory *startupapp.AuthorizedManagedDirectoryV1
	rootIdentity := ""
	for _, result := range recovered {
		if domaincheckpoint.ValidateOperationGroupTerminalForIntentV2(result.Terminal, result.Intent) != nil ||
			!domainsecurity.IsSHA256Hex(result.Intent.OperationGroupID) {
			return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery returned an invalid terminal authority record")
		}
		if !result.Created {
			continue
		}
		body, err := domaincheckpoint.OperationGroupTerminalV2Bytes(result.Terminal, result.Intent)
		if err != nil {
			return liveCheckpointRecoveryDelta{}, err
		}
		receipt := result.Receipt
		if !receipt.CreatedByThisCall || !receipt.Finalized || receipt.BodySHA256 != domainsecurity.SHA256Hex(body) {
			return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery terminal addition receipt is invalid")
		}
		if rootIdentity == "" {
			rootIdentity = receipt.RootIdentityDigest
		} else if rootIdentity != receipt.RootIdentityDigest {
			return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery terminal root identity changed across receipts")
		}
		shard := receipt.ShardName
		shardDirectory := startupapp.AuthorizedManagedDirectoryV1{
			Path: terminalRoot + "/" + shard, Mode: receipt.ShardMetadata.Mode,
			Size: receipt.ShardMetadata.Size, ModTimeUnixNano: receipt.ShardMetadata.ModTimeUnixNano,
		}
		if existing, found := shards[shard]; found &&
			(existing != shardDirectory || shardIdentities[shard] != receipt.ShardIdentityDigest) {
			return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery shard identity changed across receipts")
		}
		shards[shard] = shardDirectory
		shardIdentities[shard] = receipt.ShardIdentityDigest
		if receipt.ShardCreatedByThisCall {
			candidate := startupapp.AuthorizedManagedDirectoryV1{
				Path: terminalRoot, Mode: receipt.RootMetadata.Mode,
				Size: receipt.RootMetadata.Size, ModTimeUnixNano: receipt.RootMetadata.ModTimeUnixNano,
			}
			if rootDirectory != nil && *rootDirectory != candidate {
				return liveCheckpointRecoveryDelta{}, errors.New("checkpoint recovery root metadata changed across receipts")
			}
			rootDirectory = &candidate
		}
		delta.Managed.AddedFiles = append(delta.Managed.AddedFiles, startupapp.AuthorizedManagedFileAdditionV1{
			Path: terminalRoot + "/" + shard + "/" + result.Intent.OperationGroupID + ".json",
			Mode: receipt.RecordMetadata.Mode, Size: receipt.RecordMetadata.Size,
			ModTimeUnixNano: receipt.RecordMetadata.ModTimeUnixNano,
			SHA256:          receipt.BodySHA256, RecordCount: 1,
		})
		delta.receipts = append(delta.receipts, receipt)
	}
	orderedShards := make([]string, 0, len(shards))
	for shard := range shards {
		orderedShards = append(orderedShards, shard)
	}
	sort.Strings(orderedShards)
	if rootDirectory != nil {
		delta.Managed.MutableDirectories = append(delta.Managed.MutableDirectories, *rootDirectory)
	}
	for _, shard := range orderedShards {
		delta.Managed.MutableDirectories = append(delta.Managed.MutableDirectories, shards[shard])
	}
	return delta, nil
}

func checkpointAuthorityPresentInManagedSnapshot(snapshot domainstartup.ManagedSnapshotV1) (bool, error) {
	if domainstartup.ValidateManagedSnapshotV1(snapshot) != nil {
		return false, errors.New("managed startup snapshot is invalid")
	}
	const authorityPath = "data/private/checkpoint-authority"
	found := false
	hasDescendant := false
	for _, entry := range snapshot.Entries {
		switch {
		case entry.Path == authorityPath:
			if entry.Type != domainstartup.ManagedEntryTypeDirectory {
				return false, errors.New("checkpoint authority managed entry is not a directory")
			}
			found = true
		case strings.HasPrefix(entry.Path, authorityPath+"/"):
			hasDescendant = true
		}
	}
	if hasDescendant && !found {
		return false, errors.New("checkpoint authority managed snapshot has orphaned descendants")
	}
	return found, nil
}

// reconcileCheckpointOperationAuditsBeforeActivation never observes a
// workspace and never settles private operation authority. Semantic startup
// calls it with allowWrite=true against the stage. Real activation passes
// false, making the post-generation-check phase persistence-read-only.
func reconcileCheckpointOperationAuditsBeforeActivation(
	ctx context.Context,
	allowWrite bool,
	authority checkpointapp.SnapshotAuthority,
	events checkpointRecoveryEventStore,
	caseAuthority checkpointRecoveryCaseAuthority,
) error {
	return reconcileCheckpointOperationAuditsWithPreservationV1(ctx, allowWrite, authority, events, caseAuthority, runtimeReportRestartPreservationV1{})
}

func reconcileCheckpointOperationAuditsWithPreservationV1(ctx context.Context, allowWrite bool, authority checkpointapp.SnapshotAuthority, events checkpointRecoveryEventStore, caseAuthority checkpointRecoveryCaseAuthority, preserved runtimeReportRestartPreservationV1) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !authority.Available() || events == nil {
		return errors.New("checkpoint operation startup audit reconciliation is unavailable")
	}
	inventories, err := validatedCheckpointOperationInventories(ctx, authority, preserved.report)
	if err != nil {
		return err
	}
	expected := map[string]expectedCheckpointCaptureAudit{}
	originalCaseChecks := []func() error{}
	for _, inventory := range inventories {
		var original map[string]any
		if preserved.report != nil && preserved.report.OwnsThread(inventory.SecurityContext.ThreadID) {
			snapshot, err := preserved.report.ReadPrimaryThreadSnapshotV1(ctx, inventory.SecurityContext.ThreadID)
			if err != nil {
				return err
			}
			original = snapshot.Thread
			check := func() error {
				return validateCheckpointRecoveryThreadWithOriginalObservationV1(ctx, original, caseAuthority, inventory.SecurityContext, true)
			}
			if err := check(); err != nil {
				return err
			}
			originalCaseChecks = append(originalCaseChecks, check)
		} else {
			if err := validateCheckpointRecoveryTurn(events, caseAuthority, inventory.SecurityContext); err != nil {
				return err
			}
		}
		for _, audit := range inventory.Audits {
			if !domaincheckpointref.IsCaptureEventIDV2(audit.CaptureEventID) {
				return errors.New("checkpoint captured audit id is invalid")
			}
			if previous, found := expected[audit.CaptureEventID]; found {
				if previous.threadID != inventory.SecurityContext.ThreadID {
					return errors.New("checkpoint captured audit id crosses threads")
				}
				return errors.New("checkpoint captured audit id is duplicated by private authority")
			}
			expectedEvent := audit.Event
			if original != nil {
				expectedEvent, err = turnapp.SanitizeGenericCaseEventPublication(original, audit.Event)
				if err != nil {
					return err
				}
			}
			expected[audit.CaptureEventID] = expectedCheckpointCaptureAudit{
				threadID: inventory.SecurityContext.ThreadID, securityContext: inventory.SecurityContext, event: expectedEvent,
			}
		}
	}
	// Verify held original rows before any independent audit write. Missing
	// captures convey no authority and remain missing; existing rows must bind
	// exactly to the original legal prefix and cannot be duplicate or foreign.
	if preserved.report != nil {
		for _, threadID := range preserved.report.ThreadIDs() {
			if preserved.durable == nil {
				return errors.New("checkpoint original event preservation is unavailable")
			}
			rows, err := preserved.durable.ReadOriginalEventInventoryV1(ctx, threadID)
			if err != nil {
				return err
			}
			if err := validateCheckpointCaptureRowsV1(threadID, rows, expected, true); err != nil {
				return err
			}
		}
	}
	ids := make([]string, 0, len(expected))
	for id := range expected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, check := range originalCaseChecks {
		if err := check(); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if preserved.report != nil && preserved.report.OwnsThread(expected[id].threadID) {
			continue
		}
		if domainsecurity.TurnSecurityContextIsGeneral(expected[id].securityContext) {
			// Observe an already committed ordinary audit against its exact
			// authenticated draft. Re-projecting it using later case lineage
			// would change its payload digest. This path neither writes nor
			// publishes; current public event projection remains unchanged.
			rows, err := events.LoadEventsSince(expected[id].threadID, 0)
			if err != nil {
				return err
			}
			if len(rows.Diagnostics) != 0 {
				return errors.New("checkpoint captured audit log contains invalid records")
			}
			count, conflict, _ := domaincheckpointref.ExactCapturedEventCount(rows.Events, id, expected[id].event)
			if count == 1 && !conflict {
				continue
			}
		}
		if err := events.ReconcileCheckpointCapturedEventForStartup(expected[id].event, allowWrite); err != nil {
			return fmt.Errorf("reconcile checkpoint captured audit: %w", err)
		}
	}
	if err := validateCheckpointCaptureMarkerInventory(ctx, events, expected, preserved); err != nil {
		return err
	}
	for _, check := range originalCaseChecks {
		if err := check(); err != nil {
			return err
		}
	}
	if preserved.report != nil {
		return preserved.report.RevalidatePrimary(ctx)
	}
	return nil
}

func validatedCheckpointOperationInventories(
	ctx context.Context,
	authority checkpointapp.SnapshotAuthority,
	preserved *pendingworkapp.ReportRestartScopeV1,
) ([]checkpointapp.CapturedOperationInventoryV2, error) {
	states, err := authority.OperationGroups(ctx)
	if err != nil {
		return nil, err
	}
	keys := map[string][2]string{}
	contexts := map[string]domainsecurity.TurnSecurityContext{}
	if preserved != nil {
		if err := preserved.RevalidatePrimary(ctx); err != nil {
			return nil, err
		}
		for _, frozen := range preserved.Contexts() {
			contexts[frozen.ContextDigest] = frozen
		}
	}
	heldContexts := map[string]domainsecurity.TurnSecurityContext{}
	for _, state := range states {
		key := state.Intent.SecurityContext.ThreadID + "\x00" + state.Intent.CheckpointID
		keys[key] = [2]string{state.Intent.SecurityContext.ThreadID, state.Intent.CheckpointID}
		if preserved != nil && preserved.OwnsThread(state.Intent.SecurityContext.ThreadID) {
			frozen, found := contexts[state.Intent.SecurityContext.ContextDigest]
			if !found || frozen != state.Intent.SecurityContext {
				return nil, errors.New("checkpoint preserved capture lost its original context")
			}
			heldContexts[key] = frozen
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	inventories := make([]checkpointapp.CapturedOperationInventoryV2, 0, len(ordered))
	for _, key := range ordered {
		identity := keys[key]
		var inventory checkpointapp.CapturedOperationInventoryV2
		var err error
		if frozen, held := heldContexts[key]; held {
			inventory, err = authority.CapturedOperationInventoryForPreservedContextV1(ctx, frozen, identity[1])
		} else {
			inventory, err = authority.CapturedOperationInventory(ctx, identity[0], identity[1])
		}
		if err != nil {
			return nil, err
		}
		inventories = append(inventories, inventory)
	}
	return inventories, nil
}

func validateCheckpointRecoveryTurn(
	events checkpointRecoveryEventStore,
	caseAuthority checkpointRecoveryCaseAuthority,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	thread, err := events.GetThread(securityContext.ThreadID)
	if err != nil {
		return fmt.Errorf("read checkpoint operation thread authority: %w", err)
	}
	return validateCheckpointRecoveryThread(thread, caseAuthority, securityContext)
}

func validateCheckpointRecoveryThread(thread map[string]any, caseAuthority checkpointRecoveryCaseAuthority, securityContext domainsecurity.TurnSecurityContext) error {
	return validateCheckpointRecoveryThreadWithOriginalObservationV1(context.Background(), thread, caseAuthority, securityContext, false)
}

func validateCheckpointRecoveryThreadWithOriginalObservationV1(ctx context.Context, thread map[string]any, caseAuthority checkpointRecoveryCaseAuthority, securityContext domainsecurity.TurnSecurityContext, original bool) error {
	if thread == nil || exactRecoveryString(thread["id"]) != securityContext.ThreadID {
		return errors.New("checkpoint operation thread authority is missing")
	}
	turns, _ := thread["turns"].([]any)
	found := false
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if turn == nil || exactRecoveryString(turn["id"]) != securityContext.TurnID ||
			exactRecoveryString(turn["threadId"]) != securityContext.ThreadID {
			continue
		}
		frozen, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if parseErr != nil || frozen != securityContext {
			return errors.New("checkpoint operation frozen turn authority does not match durable history")
		}
		found = true
		break
	}
	if !found {
		return errors.New("checkpoint operation frozen turn is missing from durable history")
	}
	caseSensitive := domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext)
	// The private checkpoint inventory authenticates this frozen context, and
	// the durable turn above must match it exactly. A later case turn cannot
	// retroactively turn an earlier GENERAL capture into a case-registry entry.
	// Original/held observations retain their separate registry-owner check.
	if caseAuthority != nil && caseAuthority.IsCaseThread(securityContext.ThreadID) &&
		(original || !domainsecurity.TurnSecurityContextIsGeneral(securityContext)) {
		caseSensitive = true
	}
	if caseSensitive {
		if original {
			if observer, ok := caseAuthority.(checkpointOriginalCaseContextObserverV1); ok {
				return observer.ObserveOriginalContextForRestartV1(ctx, securityContext)
			}
			return errors.New("checkpoint original case context observer is unavailable")
		}
		if caseAuthority == nil || !caseAuthority.ContainsContext(securityContext) {
			return errors.New("checkpoint operation case context lacks committed private authority")
		}
	}
	return nil
}

func validateCheckpointCaptureMarkerInventory(
	ctx context.Context,
	events checkpointRecoveryEventStore,
	expected map[string]expectedCheckpointCaptureAudit,
	preserved runtimeReportRestartPreservationV1,
) error {
	threadIDs, err := events.AllThreadIDs()
	if err != nil {
		return err
	}
	for _, threadID := range threadIDs {
		if preserved.report != nil && preserved.report.OwnsThread(threadID) {
			// All held original IDs, including legacy IDs omitted by the live
			// store, are checked through their original family below.
			continue
		}
		result, err := events.LoadEventsSince(threadID, 0)
		if err != nil {
			return err
		}
		if len(result.Diagnostics) != 0 {
			return errors.New("checkpoint captured audit log contains invalid records")
		}
		if err := validateCheckpointCaptureRowsV1(threadID, result.Events, expected, false); err != nil {
			return err
		}
	}
	if preserved.report != nil {
		for _, threadID := range preserved.report.ThreadIDs() {
			if preserved.durable == nil {
				return errors.New("checkpoint original event preservation is unavailable")
			}
			rows, err := preserved.durable.ReadOriginalEventInventoryV1(ctx, threadID)
			if err != nil {
				return err
			}
			if err := validateCheckpointCaptureRowsV1(threadID, rows, expected, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCheckpointCaptureRowsV1(threadID string, rows []map[string]any, expected map[string]expectedCheckpointCaptureAudit, exact bool) error {
	for _, event := range rows {
		checkpoint, _ := event["checkpoint"].(map[string]any)
		if checkpoint == nil {
			continue
		}
		marker, present := checkpoint["captureEventId"]
		if !present {
			continue
		}
		eventID := exactRecoveryString(marker)
		known, ok := expected[eventID]
		if !ok || known.threadID != threadID || !domaincheckpointref.IsCaptureEventIDV2(eventID) || !domaincheckpointref.CapturedPayloadDigestMatches(checkpoint) {
			return errors.New("checkpoint captured audit history contains an unknown or invalid marker")
		}
	}
	if exact {
		for id, known := range expected {
			if known.threadID != threadID {
				continue
			}
			count, conflict, _ := domaincheckpointref.ExactCapturedEventCount(rows, id, known.event)
			if conflict || count > 1 {
				return errors.New("checkpoint preserved capture conflicts with original authority")
			}
		}
	}
	return nil
}

func exactRecoveryString(value any) string {
	text, _ := value.(string)
	if text == "" || text != strings.TrimSpace(text) {
		return ""
	}
	return text
}
