package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"analytix.local/runtime-go/internal/jobs"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
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
	if jobSnapshot.HasLegacyTypeScript {
		return nil, pendingworkapp.ErrChildProducerInventoryIncomplete
	}
	primaries, threads, err := readRuntimeOriginalPrimaryInventoryV1(ctx, roots, snapshot)
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
