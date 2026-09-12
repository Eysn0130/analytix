package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	electronlegacytask "analytix.local/runtime-go/internal/adapters/outbound/electronlegacytask"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	appstartup "analytix.local/runtime-go/internal/app/startup"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"analytix.local/runtime-go/internal/jobs"
)

// RunDesktopPrivateHistoryMigrationV2 is the desktop activation barrier for
// the independently owned Electron background-task file. When that owner has
// no work, the command performs no Go-owned semantic pass: the immediately
// following runtime startup remains the sole owner of the managed fixed point
// and must finish it before opening its listener. When Electron recovery or a
// new retirement is required, both owners still share one composite lease and
// the caller cannot activate the desktop between them.
func RunDesktopPrivateHistoryMigrationV2(
	ctx context.Context,
	dataDir string,
	durableDir string,
	userDataDir string,
) (resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	roots, err := persistencefs.ResolveRootSet(dataDir, durableDir)
	if err != nil {
		return err
	}
	separateRoots, err := persistencefs.ResolveSeparateOwnerRoots(roots, userDataDir)
	if err != nil || len(separateRoots) != 1 {
		return errors.New("legacy Electron owner root resolution failed")
	}
	lease, err := persistencefs.AcquireCompositeLeaseWithStartupUserDataAndSeparateOwnerRoots(
		roots, separateRoots[0], separateRoots[0],
	)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lease.Close()) }()
	electronPreAuthority := &electronJournalAuthorityPreflightOwnerV2{
		lease: lease, root: separateRoots[0],
	}
	if _, err := electronPreAuthority.ObserveJournalAuthorityPreflightV1(ctx); err != nil {
		return err
	}
	if !electronPreAuthority.TargetPresent() && !electronPreAuthority.JournalPresent() {
		return electronPreAuthority.ValidateJournalAuthorityPreflightV1(
			ctx, electronPreAuthority.observation.Digest(),
		)
	}
	if electronPreAuthority.JournalPresent() {
		if err := electronPreAuthority.ValidateJournalAuthorityPreflightV1(
			ctx, electronPreAuthority.observation.Digest(),
		); err != nil {
			return err
		}
		if _, present, err := lease.OpenExistingJournalAuthorityV1(); err != nil || !present {
			return errors.New("legacy Electron recovery journal has no existing installation authority")
		}
		store, err := electronlegacytask.NewStore(lease, separateRoots[0])
		if err != nil {
			return err
		}
		observation, err := store.Observe(ctx)
		if err != nil {
			return err
		}
		if observation.Retired() {
			if err := store.ValidateObservation(ctx, observation); err != nil {
				return err
			}
			return electronPreAuthority.ValidateJournalAuthorityPreflightV1(
				ctx, electronPreAuthority.observation.Digest(),
			)
		}
	}
	childRunRoot := filepath.Join(roots.DataDir, "child-runs")
	managed := &managedSnapshotFixedPointOwnerV1{
		reader: persistencefs.NewStartupSnapshotReader(roots),
	}
	childRuns := &childRunFixedPointOwnerV1{
		root: childRunRoot,
	}
	authorityPreflight, err := persistencefs.PrepareJournalAuthorityBootstrapV1(
		ctx, lease, managed, childRuns, electronPreAuthority,
	)
	if err != nil {
		return err
	}
	_, existingAuthority, err := authorityPreflight.BindExistingV1(ctx)
	if err != nil {
		return err
	}
	managedSemanticInput, err := managed.HasSemanticInputV1()
	if err != nil {
		return err
	}
	if electronPreAuthority.JournalPresent() && !existingAuthority {
		return errors.New("legacy Electron recovery journal has no existing installation authority")
	}
	if !existingAuthority {
		if !electronPreAuthority.TargetPresent() && !childRuns.HasEntries() && !managedSemanticInput {
			return nil
		}
	}
	// Reject an invalid retired TypeScript lineage before the first startup
	// authority namespace directory can be created. The semantic callback
	// repeats this witness after its normal fixed-point preparation.
	if err := validateRuntimeRetiredTypeScriptAuditLineageBeforeMutationV1(ctx, roots); err != nil {
		return err
	}
	if !existingAuthority {
		if info, err := os.Lstat(separateRoots[0]); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("Electron userData authority root is unavailable")
		}
		if _, err := authorityPreflight.CreateV1(ctx); err != nil {
			return err
		}
	}
	store, err := electronlegacytask.NewStore(lease, separateRoots[0])
	if err != nil {
		return err
	}
	electron := &electronRetirementOwnerV1{store: store}
	return appstartup.RunCompositeSemanticThenRetirementV2(
		ctx, []appstartup.FixedPointOwnerV1{managed, childRuns}, electron,
		func(ctx context.Context) error {
			migrationCtx := withRuntimeRetiredTypeScriptAuditMigrationV1(ctx)
			if err := RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE(migrationCtx, lease); err != nil {
				return err
			}
			childInventory, err := jobs.BuildChildRunInventoryV1(childRunRoot)
			if err != nil || childInventory.ManifestSHA256 == "" {
				return errors.New("child-run inventory failed")
			}
			if !managedSemanticInput && (!childInventory.RootExists || len(childInventory.Entries) == 0) {
				return nil
			}
			err = RunRuntimeSemanticStartupMigrationWithPersistenceLeaseContextE(migrationCtx, Config{
				RuntimeToken:   "desktop-private-history-migration-v2",
				DataDir:        roots.DataDir,
				DurableTempDir: roots.DurableDir,
				UserDataDir:    separateRoots[0],
				Host:           "127.0.0.1",
			}, lease)
			return err
		},
	)
}

type electronJournalAuthorityPreflightOwnerV2 struct {
	lease       *persistencefs.CompositeLease
	root        string
	observation electronlegacytask.PreAuthorityObservationV2
	observed    bool
}

func (owner *electronJournalAuthorityPreflightOwnerV2) ObserveJournalAuthorityPreflightV1(
	ctx context.Context,
) (string, error) {
	if owner == nil {
		return "", errors.New("legacy Electron pre-authority owner is unavailable")
	}
	observation, err := electronlegacytask.ObserveBeforeJournalAuthorityV2(ctx, owner.lease, owner.root)
	if err != nil || observation.Digest() == "" {
		return "", errors.New("legacy Electron pre-authority observation failed")
	}
	owner.observation = observation
	owner.observed = true
	return observation.Digest(), nil
}

func (owner *electronJournalAuthorityPreflightOwnerV2) ValidateJournalAuthorityPreflightV1(
	ctx context.Context,
	expected string,
) error {
	if owner == nil || !owner.observed || expected == "" || expected != owner.observation.Digest() {
		return errors.New("legacy Electron pre-authority binding is invalid")
	}
	return electronlegacytask.ValidateBeforeJournalAuthorityV2(
		ctx, owner.lease, owner.root, owner.observation,
	)
}

func (owner *electronJournalAuthorityPreflightOwnerV2) JournalPresent() bool {
	return owner != nil && owner.observed && owner.observation.JournalPresent()
}

func (owner *electronJournalAuthorityPreflightOwnerV2) TargetPresent() bool {
	return owner != nil && owner.observed && owner.observation.TargetPresent()
}

type managedSnapshotFixedPointOwnerV1 struct {
	reader   persistencefs.StartupSnapshotReader
	snapshot domainstartup.ManagedSnapshotV1
	observed bool
}

func (owner *managedSnapshotFixedPointOwnerV1) Observe(ctx context.Context) (appstartup.OwnerObservationV1, error) {
	snapshot, err := owner.reader.CaptureManagedSnapshotV1(ctx)
	if err != nil || domainstartup.ValidateManagedSnapshotV1(snapshot) != nil {
		return appstartup.OwnerObservationV1{}, errors.New("managed startup inventory failed")
	}
	owner.snapshot = snapshot
	owner.observed = true
	return appstartup.OwnerObservationV1{Digest: snapshot.SnapshotDigest, Retired: true}, nil
}

func (owner *managedSnapshotFixedPointOwnerV1) HasSemanticInputV1() (bool, error) {
	if owner == nil || !owner.observed {
		return false, errors.New("managed startup inventory was not observed")
	}
	return domainstartup.ManagedSnapshotHasSemanticInputV1(owner.snapshot)
}

func (owner *managedSnapshotFixedPointOwnerV1) ValidateObservation(
	ctx context.Context,
	expected appstartup.OwnerObservationV1,
) error {
	current, err := owner.Observe(ctx)
	if err != nil || current.Digest != expected.Digest {
		return errors.New("managed startup inventory changed")
	}
	return nil
}

func (owner *managedSnapshotFixedPointOwnerV1) ObserveJournalAuthorityPreflightV1(
	ctx context.Context,
) (string, error) {
	observation, err := owner.Observe(ctx)
	return observation.Digest, err
}

func (owner *managedSnapshotFixedPointOwnerV1) ValidateJournalAuthorityPreflightV1(
	ctx context.Context,
	expected string,
) error {
	current, err := owner.Observe(ctx)
	if err != nil || expected == "" || current.Digest != expected {
		return errors.New("managed startup inventory changed before journal authority bootstrap")
	}
	return nil
}

type childRunFixedPointOwnerV1 struct {
	root      string
	inventory jobs.ChildRunInventoryV1
	observed  bool
}

func (owner *childRunFixedPointOwnerV1) Observe(context.Context) (appstartup.OwnerObservationV1, error) {
	inventory, err := jobs.BuildChildRunInventoryV1(owner.root)
	if err != nil || inventory.ManifestSHA256 == "" {
		return appstartup.OwnerObservationV1{}, errors.New("child-run inventory failed")
	}
	owner.inventory = inventory
	owner.observed = true
	return appstartup.OwnerObservationV1{Digest: inventory.ManifestSHA256, Retired: true}, nil
}

func (owner *childRunFixedPointOwnerV1) ValidateObservation(
	_ context.Context,
	expected appstartup.OwnerObservationV1,
) error {
	if !owner.observed || expected.Digest == "" || expected.Digest != owner.inventory.ManifestSHA256 {
		return errors.New("child-run inventory binding is invalid")
	}
	return jobs.ValidateChildRunInventoryV1(owner.root, owner.inventory)
}

func (owner *childRunFixedPointOwnerV1) ObserveJournalAuthorityPreflightV1(
	ctx context.Context,
) (string, error) {
	observation, err := owner.Observe(ctx)
	return observation.Digest, err
}

func (owner *childRunFixedPointOwnerV1) ValidateJournalAuthorityPreflightV1(
	ctx context.Context,
	expected string,
) error {
	current, err := owner.Observe(ctx)
	if err != nil || expected == "" || current.Digest != expected {
		return errors.New("child-run inventory changed before journal authority bootstrap")
	}
	return nil
}

func (owner *childRunFixedPointOwnerV1) HasEntries() bool {
	return owner != nil && owner.observed && owner.inventory.RootExists && len(owner.inventory.Entries) > 0
}

type electronRetirementOwnerV1 struct {
	store       *electronlegacytask.Store
	observation electronlegacytask.ObservationV1
	observed    bool
}

func (owner *electronRetirementOwnerV1) Observe(ctx context.Context) (appstartup.OwnerObservationV1, error) {
	observation, err := owner.store.Observe(ctx)
	if err != nil || observation.Digest() == "" {
		return appstartup.OwnerObservationV1{}, errors.New("legacy Electron owner observation failed")
	}
	owner.observation = observation
	owner.observed = true
	return appstartup.OwnerObservationV1{
		Digest:           observation.Digest(),
		RecoveryRequired: observation.RecoveryRequired(),
		Retired:          observation.Retired(),
	}, nil
}

func (owner *electronRetirementOwnerV1) ValidateObservation(
	ctx context.Context,
	expected appstartup.OwnerObservationV1,
) error {
	if !owner.observed || expected.Digest == "" || expected.Digest != owner.observation.Digest() ||
		expected.RecoveryRequired != owner.observation.RecoveryRequired() ||
		expected.Retired != owner.observation.Retired() {
		return errors.New("legacy Electron owner observation binding is invalid")
	}
	return owner.store.ValidateObservation(ctx, owner.observation)
}

func (owner *electronRetirementOwnerV1) Recover(ctx context.Context) error {
	return owner.store.Recover(ctx)
}

func (owner *electronRetirementOwnerV1) Prepare(
	ctx context.Context,
) (appstartup.PreparedMonotonicRetirementV1, error) {
	return owner.store.Prepare(ctx)
}
