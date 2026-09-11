package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"sync"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

type PersistenceLease = persistencefs.CompositeLease

// PreparedRuntimeServerStartup proves that semantic persistence planning,
// simulation, journal apply, and final readback completed before the caller
// activates listeners or other externally observable runtime surfaces.
type PreparedRuntimeServerStartup struct {
	mu                        sync.Mutex
	lease                     *persistencefs.CompositeLease
	roots                     persistencefs.RootSet
	configurationDigest       string
	managedSnapshotDigest     string
	generationDigest          string
	semanticValidation        *persistencefs.StartupSemanticValidationAttemptV1
	originalCreates           *runtimeOriginalCreateStartupV1
	finalHistoryQualification *runtimeFinalHistoryQualificationV1
	activated                 bool
}

func PrepareRuntimeServerStartupWithPersistenceLeaseE(config Config, lease *persistencefs.CompositeLease) (*PreparedRuntimeServerStartup, error) {
	return PrepareRuntimeServerStartupWithPersistenceLeaseContextE(context.Background(), config, lease)
}

func PrepareRuntimeServerStartupWithPersistenceLeaseContextE(
	ctx context.Context,
	config Config,
	lease *persistencefs.CompositeLease,
) (*PreparedRuntimeServerStartup, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	semanticValidation := persistencefs.NewStartupSemanticValidationAttemptV1()
	ctx = semanticValidation.WithContext(ctx)
	config = normalizeConfig(config)
	if err := validateRuntimeProtectedRootTopology(config); err != nil {
		return nil, err
	}
	if err := validateRuntimeAuthentication(config); err != nil {
		return nil, err
	}
	currentRoots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		return nil, err
	}
	frozenRoots, held := lease.FrozenRoots()
	if !held || currentRoots != frozenRoots {
		return nil, errors.New("runtime persistence roots changed before startup preparation")
	}
	if err := lease.ValidateStartupUserData(config.UserDataDir); err != nil {
		return nil, err
	}
	rootAuthority, held := lease.FrozenAuthority()
	if !held {
		return nil, errors.New("runtime persistence root authority changed before startup preparation")
	}
	originalCreates, err := prepareRuntimeOriginalCreateStartupV1(ctx, currentRoots, lease)
	if err != nil {
		return nil, err
	}
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, originalCreates.plainProofV1())
	journalAuthority, err := ensureRuntimeJournalAuthorityAfterManagedPreflight(ctx, currentRoots, lease, originalCreates)
	if err != nil {
		return nil, err
	}
	runtimeInfoDataDir := config.DataDir
	config = applyRuntimePersistenceRoots(config, currentRoots)
	var planOutput runtimeStartupPlanOutputV1
	if _, err := newRuntimeServerHandlerWithRootsModeE(
		ctx, config, currentRoots, rootAuthority, journalAuthority, lease, true, false, "", runtimeInfoDataDir, &planOutput, false, nil, originalCreates,
	); err != nil {
		return nil, err
	}
	configurationDigest := planOutput.configurationDigest
	if configurationDigest == "" || planOutput.originalCreates == nil || planOutput.finalHistoryQualification == nil {
		return nil, errors.New("runtime startup preparation omitted its configuration authority")
	}
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, planOutput.originalCreates.plainProofV1())
	finalSnapshot, err := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(currentRoots, planOutput.originalCreates.proofV1()).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		return nil, err
	}
	generationDigest, err := runtimeStartupGenerationDigest(configurationDigest, finalSnapshot)
	if err != nil {
		return nil, err
	}
	return &PreparedRuntimeServerStartup{
		lease: lease, roots: currentRoots, configurationDigest: configurationDigest,
		managedSnapshotDigest: finalSnapshot.SnapshotDigest, generationDigest: generationDigest,
		semanticValidation: semanticValidation, originalCreates: planOutput.originalCreates,
		finalHistoryQualification: planOutput.finalHistoryQualification,
	}, nil
}

// RunRuntimeSemanticStartupMigrationWithPersistenceLeaseContextE executes the
// canonical semantic persistence migration without returning an activation
// capability. It is used by pre-activation maintenance barriers that must
// never be able to start a listener, provider, MCP server, or host process.
func RunRuntimeSemanticStartupMigrationWithPersistenceLeaseContextE(
	ctx context.Context,
	config Config,
	lease *persistencefs.CompositeLease,
) error {
	_, err := PrepareRuntimeServerStartupWithPersistenceLeaseContextE(ctx, config, lease)
	return err
}

// RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE
// settles only an already-authenticated semantic transaction. It does not load
// provider/MCP configuration, create a new semantic plan, or return an
// activation capability.
func RecoverAuthenticatedExistingSemanticStartupWithPersistenceLeaseContextE(
	ctx context.Context,
	lease *persistencefs.CompositeLease,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if lease == nil {
		return errors.New("runtime semantic recovery lease is unavailable")
	}
	roots, held := lease.FrozenRoots()
	if !held {
		return errors.New("runtime semantic recovery roots changed")
	}
	rootAuthority, held := lease.FrozenAuthority()
	if !held {
		return errors.New("runtime semantic recovery root authority changed")
	}
	journalAuthority, held := lease.FrozenJournalAuthority()
	if !held {
		var err error
		journalAuthority, held, err = lease.OpenExistingJournalAuthorityV1()
		if err != nil {
			return err
		}
		if !held {
			return nil
		}
	}
	originalCreates, err := prepareRuntimeOriginalCreateStartupV1(ctx, roots, lease)
	if err != nil {
		return err
	}
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, originalCreates.plainProofV1())
	if err := rejectUnqualifiedFinalHistoryRecoveryV1(ctx, roots, originalCreates); err != nil {
		return err
	}
	core, err := prepareRuntimeChildIdentityStartupV1(ctx, roots, rootAuthority, lease, nil, originalCreates)
	if err != nil {
		return err
	}
	preserved, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, core, lease, nil, nil)
	if err != nil {
		return err
	}
	retained, err := originalCreates.retainedV1(ctx, preserved)
	if err != nil {
		return err
	}
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, retained.plainProofV1())
	// This entry settles an existing program without running create cleanup.
	// Its still-present original directories remain part of the signed state,
	// even when no report scope requires retention across ordinary startup.
	return persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(
		roots, rootAuthority, journalAuthority, preserved, originalCreates.proofV1(),
	).RecoverAuthenticatedExisting(ctx)
}

func ensureRuntimeJournalAuthorityAfterManagedPreflight(
	ctx context.Context,
	roots persistencefs.RootSet,
	lease *persistencefs.CompositeLease,
	originalCreates *runtimeOriginalCreateStartupV1,
) (*persistencefs.JournalNamespaceAuthority, error) {
	if err := originalCreates.revalidateV1(ctx, roots); err != nil {
		return nil, err
	}
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, originalCreates.plainProofV1())
	// Transitional single-owner startup leases still bootstrap their journal
	// authority at acquisition so authenticated recovery can classify existing
	// private-CAS residues before the strict managed baseline. Composite-owner
	// desktop leases never take this branch and require a prepared owner proof.
	if authority, held := lease.FrozenJournalAuthority(); held {
		return authority, nil
	}
	owner := &managedSnapshotFixedPointOwnerV1{
		reader: persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots, originalCreates.proofV1()),
	}
	prepared, err := persistencefs.PrepareJournalAuthorityBootstrapV1(ctx, lease, owner)
	if err != nil {
		return nil, err
	}
	authority, present, err := prepared.BindExistingV1(ctx)
	if err != nil {
		return nil, err
	}
	if present {
		return authority, nil
	}
	return prepared.CreateV1(ctx)
}

func (prepared *PreparedRuntimeServerStartup) Activate(config Config) (http.Handler, error) {
	return prepared.ActivateContext(context.Background(), config)
}

func (prepared *PreparedRuntimeServerStartup) ActivateContext(ctx context.Context, config Config) (http.Handler, error) {
	if prepared == nil {
		return nil, errors.New("runtime startup preparation is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prepared.semanticValidation == nil {
		return nil, errors.New("runtime startup semantic validation attempt is unavailable")
	}
	ctx = prepared.semanticValidation.WithContext(ctx)
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.activated {
		return nil, errors.New("runtime startup preparation was already activated")
	}
	config = normalizeConfig(config)
	if err := validateRuntimeProtectedRootTopology(config); err != nil {
		return nil, err
	}
	if err := validateRuntimeAuthentication(config); err != nil {
		return nil, err
	}
	currentRoots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		return nil, err
	}
	frozenRoots, held := prepared.lease.FrozenRoots()
	if !held || currentRoots != prepared.roots || frozenRoots != prepared.roots {
		return nil, errors.New("runtime persistence roots changed after startup preparation")
	}
	if err := prepared.lease.ValidateStartupUserData(config.UserDataDir); err != nil {
		return nil, err
	}
	rootAuthority, held := prepared.lease.FrozenAuthority()
	if !held {
		return nil, errors.New("runtime persistence root authority changed after startup preparation")
	}
	journalAuthority, held := prepared.lease.FrozenJournalAuthority()
	if !held {
		return nil, errors.New("runtime startup journal authority changed after startup preparation")
	}
	activation, err := prepared.lease.BeginActivation()
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() { activation.Complete(succeeded) }()
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, prepared.originalCreates.plainProofV1())
	if err := prepared.originalCreates.revalidateV1(ctx, prepared.roots); err != nil {
		return nil, err
	}
	currentSnapshot, err := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(prepared.roots, prepared.originalCreates.proofV1()).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		return nil, err
	}
	currentGenerationDigest, err := runtimeStartupGenerationDigest(prepared.configurationDigest, currentSnapshot)
	if err != nil {
		return nil, err
	}
	if currentSnapshot.SnapshotDigest != prepared.managedSnapshotDigest || currentGenerationDigest != prepared.generationDigest {
		return nil, errors.New("managed persistence changed after startup preparation")
	}
	runtimeInfoDataDir := config.DataDir
	config = applyRuntimePersistenceRoots(config, prepared.roots)
	if prepared.finalHistoryQualification == nil {
		return nil, errors.New("runtime startup Original final history qualification is unavailable")
	}
	ctx = context.WithValue(ctx, runtimeFinalHistoryQualificationKeyV1{}, prepared.finalHistoryQualification)
	handler, err := newRuntimeServerHandlerWithRootsModeE(
		ctx, config, prepared.roots, rootAuthority, journalAuthority, prepared.lease, false, false, prepared.configurationDigest, runtimeInfoDataDir, nil, true, nil, prepared.originalCreates,
	)
	if err != nil {
		return nil, err
	}
	prepared.activated = true
	succeeded = true
	return handler, nil
}
