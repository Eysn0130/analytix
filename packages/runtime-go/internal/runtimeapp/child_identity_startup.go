package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	"analytix.local/runtime-go/internal/jobs"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	runtimeserver "analytix.local/runtime-go/internal/server"
)

type runtimeChildIdentityStartupV1 struct {
	roots                 persistencefs.RootSet
	snapshot              persistencefs.RawSnapshot
	jobs                  jobs.ChildRunInventoryV1
	jobRecords            []domainjob.Record
	pending               *pendingworkstore.PreparedRecoveryV1
	emptyPending          *finalauthority.EmptyPartialOwnerProofV1
	revalidateKey         func(context.Context) error
	floors                domainpendingwork.ChildIdentityFloorsV1
	pendingInventory      pendingworkapp.TrustedInventoryV1
	pendingSemantic       *runtimePendingSemanticObservationV1
	verification          authorityport.Authority
	access                finalauthority.SecurePrivateCASRecoveryAccessAuthority
	primaries             *runtimeOriginalPrimaryInventoryV1
	originalCreates       *runtimeOriginalCreateStartupV1
	originalRegistryTrust *runtimeOriginalRegistryTrustV2
}

// runtimeRetiredTypeScriptAuditMigrationContextKeyV1 is an in-process
// capability for the desktop migration barrier. A retired TypeScript record
// remains outside the current child-producer admission model; the migration
// may carry its complete inventory forward only long enough for the later
// exact lineage witness and semantic projection to retire it. Ordinary startup
// never receives this capability and continues to refuse the incomplete
// producer inventory.
type runtimeRetiredTypeScriptAuditMigrationContextKeyV1 struct{}

type runtimeRetiredTypeScriptAuditMigrationCapabilityV1 struct{}

func withRuntimeRetiredTypeScriptAuditMigrationV1(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeRetiredTypeScriptAuditMigrationContextKeyV1{}, runtimeRetiredTypeScriptAuditMigrationCapabilityV1{})
}

func allowsRuntimeRetiredTypeScriptAuditMigrationV1(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(runtimeRetiredTypeScriptAuditMigrationContextKeyV1{}).(runtimeRetiredTypeScriptAuditMigrationCapabilityV1)
	return ok
}

type runtimeSemanticStageAccessMarkerV1 interface {
	IsSemanticStagePrivateCASAccessAuthority() bool
}

func isRuntimeSemanticStageAccessV1(access finalauthority.SecurePrivateCASRecoveryAccessAuthority) bool {
	marker, ok := access.(runtimeSemanticStageAccessMarkerV1)
	return ok && marker.IsSemanticStagePrivateCASAccessAuthority()
}

// runtimeRetiredTypeScriptAuditParentIDsV1 derives the only parent IDs that
// the retired audit inventory may temporarily tolerate from the same exact
// source inventory that the semantic child-run witness verifies. The second
// observation closes the small gap between the witness' private observation
// and this planning value; a changed source inventory fails closed.
func runtimeRetiredTypeScriptAuditParentIDsV1(ctx context.Context, roots persistencefs.RootSet) (map[string]struct{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	childRunRoot := filepath.Join(roots.DataDir, "child-runs")
	observation, err := jobs.BuildLegacyTypeScriptLineageObservationV1(childRunRoot)
	if err != nil || len(observation.Entries) == 0 {
		return nil, errors.Join(pendingworkapp.ErrChildProducerInventoryIncomplete, err)
	}
	if _, err := newFrozenLegacyTypeScriptChildLineageWitnessV1(
		childRunRoot, newLegacyTypeScriptRawThreadReaderV1(roots.DurableDir),
	); err != nil {
		return nil, err
	}
	current, err := jobs.BuildLegacyTypeScriptLineageObservationV1(childRunRoot)
	if err != nil {
		return nil, err
	}
	if current.InventoryManifestSHA256 != observation.InventoryManifestSHA256 || len(current.Entries) != len(observation.Entries) {
		return nil, errors.New("legacy TypeScript child-run inventory changed during lineage witness")
	}
	parentIDs := make(map[string]struct{}, len(observation.Entries))
	for _, entry := range observation.Entries {
		parentID := strings.TrimSpace(entry.Lineage.ParentThreadID)
		if !domainthread.IsCanonicalRecordID(parentID) {
			return nil, pendingworkapp.ErrChildProducerInventoryIncomplete
		}
		parentIDs[parentID] = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return parentIDs, nil
}

// validateRuntimeRetiredTypeScriptAuditLineageBeforeMutationV1 performs the
// same complete legacy lineage witness before the desktop migration can
// create its first startup-authority namespace entry. The semantic stage
// repeats the identity-bound observation; this early read only prevents an
// invalid retired source from leaving preflight housekeeping behind.
func validateRuntimeRetiredTypeScriptAuditLineageBeforeMutationV1(
	ctx context.Context,
	roots persistencefs.RootSet,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	jobSnapshot, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(roots.DataDir, "child-runs"))
	if err != nil {
		return err
	}
	if !jobSnapshot.HasLegacyTypeScript {
		return nil
	}
	_, err = runtimeRetiredTypeScriptAuditParentIDsV1(ctx, roots)
	return err
}

// materializeRuntimeRetiredAuditPrimariesV1 runs only against semantic-stage
// roots. It uses the normal durable recovery/upsert sanitation path, so a
// metadata-only parent becomes a real stage primary captured by the signed
// semantic plan before any ordinary startup can observe it.
func materializeRuntimeRetiredAuditPrimariesV1(
	ctx context.Context,
	roots persistencefs.RootSet,
	snapshot persistencefs.RawSnapshot,
	parentIDs map[string]struct{},
) (persistencefs.RawSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(parentIDs) == 0 {
		return snapshot, nil
	}
	files := make(map[string]persistencefs.EntryRecord, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		files[entry.Path] = entry
	}
	ids := make([]string, 0, len(parentIDs))
	for id := range parentIDs {
		if !domainthread.IsCanonicalRecordID(id) {
			return persistencefs.RawSnapshot{}, pendingworkapp.ErrChildProducerInventoryIncomplete
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	store, err := runtimeserver.NewRuntimeEventSessionStoreForSemanticStartup(runtimeserver.RuntimeServerConfig{
		ProductionDurableRoot: roots.DurableDir,
	})
	if err != nil {
		return persistencefs.RawSnapshot{}, err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return persistencefs.RawSnapshot{}, err
		}
		threadPath := "durable/threads/" + id
		directory, exists := files[threadPath]
		if !exists || directory.Type != "directory" {
			return persistencefs.RawSnapshot{}, pendingworkapp.ErrChildProducerInventoryIncomplete
		}
		primary, exists := files[threadPath+"/thread.json"]
		if exists {
			if primary.Type != "file" {
				return persistencefs.RawSnapshot{}, pendingworkapp.ErrChildProducerInventoryIncomplete
			}
			continue
		}
		thread, err := store.GetThreadForAuthorityRepair(id)
		if err != nil {
			return persistencefs.RawSnapshot{}, err
		}
		if thread == nil || runtimeappStringFieldV1(thread, "id") != id {
			return persistencefs.RawSnapshot{}, pendingworkapp.ErrChildProducerInventoryIncomplete
		}
		if err := store.ReplaceThreadForAuthorityRepair(id, thread); err != nil {
			return persistencefs.RawSnapshot{}, err
		}
	}
	return persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, roots)
}

// prepareRuntimeChildIdentityStartupV1 runs before recovery, constructors,
// legacy projection, and seed. Its inventory burns identities only: missing
// child jobs or process start capabilities are never reconstructed.
func prepareRuntimeChildIdentityStartupV1(ctx context.Context, roots persistencefs.RootSet, rootAuthority *persistencefs.RootAuthority, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority, originals ...*runtimeOriginalCreateStartupV1) (*runtimeChildIdentityStartupV1, error) {
	if len(originals) > 1 {
		return nil, errors.New("runtime original creation observation is ambiguous")
	}
	var original *runtimeOriginalCreateStartupV1
	if len(originals) == 1 {
		original = originals[0]
		if err := original.revalidateV1(ctx, roots); err != nil {
			return nil, err
		}
	}
	snapshot, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, roots)
	if err != nil {
		return nil, err
	}
	semanticJournal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, roots, original.proofV1())
	if err != nil {
		return nil, err
	}
	needsPendingObservation, err := pendingSemanticJournalNeedsObservationV1(semanticJournal)
	if err != nil {
		return nil, err
	}
	emptyPending, err := finalauthority.PrepareEmptyPartialOwnerProofV1(ctx, roots.DataDir, "pending-work", access)
	if err != nil {
		return nil, err
	}
	var pending *pendingworkstore.PreparedRecoveryV1
	var receipts []domainpendingwork.PendingWorkReceiptV1
	var dispositions []domainpendingwork.PendingWorkDispositionV1
	if emptyPending == nil {
		pending, err = pendingworkstore.PrepareRecoveryV1(ctx, filepath.Join(roots.DataDir, "private", "pending-work"), access)
		if err != nil {
			return nil, err
		}
		if semanticJournal == nil {
			receipts, dispositions, err = pending.SnapshotInventory(ctx)
		} else {
			receipts, dispositions, err = pending.SnapshotCanonicalRecordsV1(ctx)
		}
		if err != nil {
			return nil, err
		}
	}
	var trusted pendingworkapp.TrustedInventoryV1
	var revalidateKey func(context.Context) error
	var verification authorityport.Authority
	var pendingSemantic *runtimePendingSemanticObservationV1
	if len(receipts) != 0 || len(dispositions) != 0 || needsPendingObservation {
		if installation == nil {
			localKey, err := finalauthority.OpenExistingFileVerificationV1(rootAuthority)
			if err != nil {
				return nil, err
			}
			verification, revalidateKey = localKey, localKey.Revalidate
		} else {
			verification, revalidateKey = installation, installation.ValidateCurrentInstallation
		}
		if semanticJournal == nil {
			trusted, err = pendingworkapp.VerifyTrustedSnapshotV1(ctx, receipts, dispositions, verification)
		} else {
			pendingSemantic, err = prepareRuntimePendingSemanticObservationV1(ctx, receipts, dispositions, verification, semanticJournal)
			if err == nil {
				trusted = pendingSemantic.original
			}
		}
		if err != nil {
			return nil, err
		}
	}
	jobSnapshot, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(roots.DataDir, "child-runs"))
	if err != nil {
		return nil, err
	}
	if jobSnapshot.HasLegacyTypeScript && !allowsRuntimeRetiredTypeScriptAuditMigrationV1(ctx) {
		return nil, pendingworkapp.ErrChildProducerInventoryIncomplete
	}
	var retiredParentIDs map[string]struct{}
	if jobSnapshot.HasLegacyTypeScript && allowsRuntimeRetiredTypeScriptAuditMigrationV1(ctx) {
		retiredParentIDs, err = runtimeRetiredTypeScriptAuditParentIDsV1(ctx, roots)
		if err != nil {
			return nil, err
		}
		if isRuntimeSemanticStageAccessV1(access) {
			snapshot, err = materializeRuntimeRetiredAuditPrimariesV1(ctx, roots, snapshot, retiredParentIDs)
			if err != nil {
				return nil, err
			}
		}
	}
	var primaries *runtimeOriginalPrimaryInventoryV1
	var threads []map[string]any
	if len(retiredParentIDs) != 0 && allowsRuntimeRetiredTypeScriptAuditMigrationV1(ctx) && !isRuntimeSemanticStageAccessV1(access) {
		primaries, threads, err = readRuntimeOriginalPrimaryInventoryForRetiredAuditV1(ctx, roots, snapshot, retiredParentIDs)
	} else {
		primaries, threads, err = readRuntimeOriginalPrimaryInventoryV1(ctx, roots, snapshot)
	}
	if err != nil {
		return nil, err
	}
	allocation := trusted
	if pendingSemantic != nil {
		if err := validateRuntimePendingSemanticScopeV1(ctx, pendingSemantic, verification, primaries, &runtimeReportInheritedHistoryV1{core: &runtimeChildIdentityStartupV1{
			roots: roots, access: access, verification: verification, revalidateKey: revalidateKey,
			pendingInventory: trusted, pendingSemantic: pendingSemantic, jobRecords: jobSnapshot.Records, originalCreates: original,
		}}); err != nil {
			return nil, err
		}
		allocation = pendingSemantic.allocation
	}
	floors, err := pendingworkapp.DeriveChildIdentityFloorsV1(allocation, jobSnapshot.Records, threads)
	if err != nil {
		return nil, err
	}
	// Residue/artifact bytes cannot become records, but their exact producer
	// names still occupy the job namespace until authenticated recovery.
	for _, entry := range jobSnapshot.Inventory.Entries {
		if err := floors.Observe(entry.JobID, "", ""); err != nil {
			return nil, err
		}
	}
	prepared := &runtimeChildIdentityStartupV1{roots: roots, snapshot: snapshot, jobs: jobSnapshot.Inventory, jobRecords: jobSnapshot.Records, pending: pending, emptyPending: emptyPending, revalidateKey: revalidateKey, floors: floors, pendingInventory: trusted, pendingSemantic: pendingSemantic, verification: verification, access: access, primaries: primaries, originalCreates: original}
	if err := prepared.revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *runtimeChildIdentityStartupV1) revalidate(ctx context.Context) error {
	if prepared == nil || (prepared.pending == nil) == (prepared.emptyPending == nil) {
		return errors.New("Core child identity startup snapshot is unavailable")
	}
	if prepared.originalCreates != nil {
		if err := prepared.originalCreates.revalidateV1(ctx, prepared.roots); err != nil {
			return err
		}
	}
	if prepared.revalidateKey != nil {
		if err := prepared.revalidateKey(ctx); err != nil {
			return err
		}
	}
	if prepared.originalRegistryTrust != nil {
		if err := prepared.originalRegistryTrust.Revalidate(ctx); err != nil {
			return err
		}
	}
	if prepared.pendingSemantic != nil {
		if err := prepared.pendingSemantic.journal.Revalidate(ctx); err != nil {
			return err
		}
	}
	if prepared.emptyPending != nil {
		if err := prepared.emptyPending.Revalidate(ctx); err != nil {
			return err
		}
	} else {
		if err := prepared.pending.Revalidate(ctx); err != nil {
			return err
		}
	}
	if err := jobs.ValidateChildRunInventoryV1(filepath.Join(prepared.roots.DataDir, "child-runs"), prepared.jobs); err != nil {
		return err
	}
	current, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, prepared.roots)
	if err != nil {
		return err
	}
	if current.SHA256 != prepared.snapshot.SHA256 || current.FileCount != prepared.snapshot.FileCount || len(current.Entries) != len(prepared.snapshot.Entries) {
		return errors.New("Core child identity denominator changed before startup effects")
	}
	return nil
}
