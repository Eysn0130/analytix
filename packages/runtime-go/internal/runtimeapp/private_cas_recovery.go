package runtimeapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	attachmentauthority "analytix.local/runtime-go/internal/adapters/outbound/attachmentauthority"
	authorityadvancefs "analytix.local/runtime-go/internal/adapters/outbound/authorityadvancefs"
	backendgenerationfs "analytix.local/runtime-go/internal/adapters/outbound/backendgenerationfs"
	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	caseentitystore "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	casethreadauthority "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceauthoritystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	piiauthorizationstore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	reportpublicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	threadriskpolicystore "analytix.local/runtime-go/internal/adapters/outbound/threadriskpolicy"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	runtimePrivateCASSemanticValidatorVersionV4         = "analytix.runtime-private-cas-owner-semantics/v4"
	runtimePrivateCASCaseEntityOwnerV1                  = "case-entity"
	runtimePrivateCASCaseEntityBoundaryOnlySemanticsV1  = "analytix.runtime-case-entity-boundary-only-semantics/v1"
	runtimePrivateCASEvidenceRegistryOwnerV1            = domainprivatecas.EvidenceRegistryRecoveryGroupID
	runtimePrivateCASEvidenceRegistryBlockedSemanticsV1 = "analytix.runtime-evidence-registry-blocked-semantics/v1"
)

type runtimePrivateCASOwnerRecovery struct {
	name                    string
	expectedRoots           []string
	standaloneCASRoots      bool
	additionalTopologyCount int
	prepare                 func(context.Context) (runtimePreparedPrivateCASOwnerRecovery, error)
	applyBeforeTransaction  func(context.Context, runtimePreparedPrivateCASOwnerRecovery) (bool, error)
}

type runtimePreparedPrivateCASOwnerRecovery interface {
	finalauthority.PreparedSecurePrivateCASRecoveryAuthorityV3
	ValidateSemantics(context.Context) error
}

type runtimePrivateCASAdditionalSemanticAuthorityV4 interface {
	PrivateCASRecoveryAdditionalSemanticDigestV4() string
}

// runtimeCaseEntityBoundaryRecoveryV1 preserves the authenticated case-entity
// bytes and exact topology when only their domain semantics are unusable. It
// deliberately exposes no case-entity store or funds capability, so damaged
// case-only state cannot disable the permanent ordinary Agent base.
type runtimeCaseEntityBoundaryRecoveryV1 struct {
	plans      []*finalauthority.PreparedSecurePrivateCASRecoveryV1
	topologies []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3
}

// runtimeEvidenceRegistryBoundaryRecoveryV1 retains the exact physically
// bound registry owner while declining all registry semantics. It is a
// host-private recovery participant only: it exposes no registry, witness,
// receipt, repair, or migration capability to the running application.
type runtimeEvidenceRegistryBoundaryRecoveryV1 struct {
	plans      []*finalauthority.PreparedSecurePrivateCASRecoveryV1
	topologies []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3
}

func newRuntimeEvidenceRegistryBoundaryRecoveryV1(
	prepared runtimePreparedPrivateCASOwnerRecovery,
) (*runtimeEvidenceRegistryBoundaryRecoveryV1, error) {
	if prepared == nil {
		return nil, errors.New("runtime evidence registry boundary recovery is unavailable")
	}
	boundary := &runtimeEvidenceRegistryBoundaryRecoveryV1{
		plans: prepared.SecurePrivateCASRecoveryPlansV2(), topologies: prepared.PrivateCASRecoveryTopologiesV3(),
	}
	if len(boundary.plans) != 2 || len(boundary.topologies) != 1 {
		return nil, errors.New("runtime evidence registry boundary recovery is incomplete")
	}
	return boundary, nil
}

func (prepared *runtimeEvidenceRegistryBoundaryRecoveryV1) ValidateSemantics(ctx context.Context) error {
	return prepared.Revalidate(ctx)
}

func (prepared *runtimeEvidenceRegistryBoundaryRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || len(prepared.plans) != 2 || len(prepared.topologies) != 1 {
		return errors.New("runtime evidence registry boundary recovery is invalid")
	}
	for _, plan := range prepared.plans {
		if plan == nil {
			return errors.New("runtime evidence registry boundary recovery contains an invalid leaf")
		}
		if err := plan.Revalidate(ctx); err != nil {
			return err
		}
	}
	for _, topology := range prepared.topologies {
		if topology == nil {
			return errors.New("runtime evidence registry boundary recovery contains an invalid topology")
		}
		if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (prepared *runtimeEvidenceRegistryBoundaryRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil {
		return nil
	}
	return append([]*finalauthority.PreparedSecurePrivateCASRecoveryV1(nil), prepared.plans...)
}

func (prepared *runtimeEvidenceRegistryBoundaryRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil {
		return nil
	}
	return append([]finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3(nil), prepared.topologies...)
}

func (prepared *runtimeEvidenceRegistryBoundaryRecoveryV1) PrivateCASRecoveryAdditionalSemanticDigestV4() string {
	if prepared == nil {
		return ""
	}
	digest := sha256.Sum256([]byte(runtimePrivateCASEvidenceRegistryBlockedSemanticsV1))
	return hex.EncodeToString(digest[:])
}

func (prepared *runtimeEvidenceRegistryBoundaryRecoveryV1) FreezeSecurePrivateCASRecoveryTargetsV4() bool {
	return prepared != nil
}

func newRuntimeCaseEntityBoundaryRecoveryV1(
	prepared runtimePreparedPrivateCASOwnerRecovery,
) (*runtimeCaseEntityBoundaryRecoveryV1, error) {
	if prepared == nil {
		return nil, errors.New("runtime case entity boundary recovery is unavailable")
	}
	boundary := &runtimeCaseEntityBoundaryRecoveryV1{
		plans:      prepared.SecurePrivateCASRecoveryPlansV2(),
		topologies: prepared.PrivateCASRecoveryTopologiesV3(),
	}
	if len(boundary.plans) != 3 || len(boundary.topologies) != 1 {
		return nil, errors.New("runtime case entity boundary recovery is incomplete")
	}
	return boundary, nil
}

func (prepared *runtimeCaseEntityBoundaryRecoveryV1) ValidateSemantics(ctx context.Context) error {
	return prepared.Revalidate(ctx)
}

func (prepared *runtimeCaseEntityBoundaryRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || len(prepared.plans) != 3 || len(prepared.topologies) != 1 {
		return errors.New("runtime case entity boundary recovery is invalid")
	}
	for _, plan := range prepared.plans {
		if plan == nil {
			return errors.New("runtime case entity boundary recovery contains an invalid leaf")
		}
		if err := plan.Revalidate(ctx); err != nil {
			return err
		}
	}
	for _, topology := range prepared.topologies {
		if topology == nil {
			return errors.New("runtime case entity boundary recovery contains an invalid topology")
		}
		if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (prepared *runtimeCaseEntityBoundaryRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil {
		return nil
	}
	return append([]*finalauthority.PreparedSecurePrivateCASRecoveryV1(nil), prepared.plans...)
}

func (prepared *runtimeCaseEntityBoundaryRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil {
		return nil
	}
	return append([]finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3(nil), prepared.topologies...)
}

func (prepared *runtimeCaseEntityBoundaryRecoveryV1) PrivateCASRecoveryAdditionalSemanticDigestV4() string {
	if prepared == nil {
		return ""
	}
	digest := sha256.Sum256([]byte(runtimePrivateCASCaseEntityBoundaryOnlySemanticsV1))
	return hex.EncodeToString(digest[:])
}

func recoverRuntimePrivateCASCreateResidues(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	preservations ...runtimeReportRestartPreservationV1,
) error {
	if access == nil {
		return errors.New("runtime private CAS create-residue recovery authority is unavailable")
	}
	prepared, err := finalauthority.PrepareSecurePrivateCASCreateResidueRecoveryV1(ctx, dataDir, access)
	if err != nil {
		return fmt.Errorf("prepare runtime private CAS create-residue recovery: %w", err)
	}
	rootCatalog, err := runtimePrivateCASRecoveryRootCatalogV4(dataDir)
	if err != nil {
		return err
	}
	probeRoots := make([]string, 0, len(rootCatalog))
	for root := range rootCatalog {
		probeRoots = append(probeRoots, root)
	}
	if err := finalauthority.ValidateNoUnsignedSecurePrivateCASRecoveryPhasesWithPreparedCreateResiduesV4(
		ctx, probeRoots, access, prepared,
	); err != nil {
		return fmt.Errorf("preflight runtime private CAS unsigned recovery phases: %w", err)
	}
	if len(preservations) > 1 {
		return errors.New("runtime private CAS creation preservation is ambiguous")
	}
	apply := prepared.Apply
	if len(preservations) == 1 && (preservations[0].attachments != nil || preservations[0].publication.frozenV1()) {
		preserved := preservations[0]
		core := preserved.core
		if core == nil && preserved.publication != nil {
			core = preserved.publication.core
		}
		if core == nil || core.roots.DataDir != dataDir || core.originalCreates == nil {
			return errors.New("runtime private CAS creation preservation root differs")
		}
		retained, err := core.originalCreates.retainedV1(ctx, preserved)
		if err != nil {
			return err
		}
		var proof *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
		if retained.combined != nil {
			proof = retained.combined
		} else {
			proof = retained.attachment
		}
		_, attachmentRevalidate, err := preserved.prepareAttachmentDirectoryRevalidationV1(ctx, false)
		if err != nil {
			return err
		}
		publicationRevalidate, err := preserved.publication.preparePhysicalRevalidationV1(ctx)
		if err != nil {
			return err
		}
		revalidate := func(ctx context.Context) error {
			var err error
			if attachmentRevalidate != nil {
				err = attachmentRevalidate(ctx)
			}
			if publicationRevalidate != nil {
				err = errors.Join(err, publicationRevalidate(ctx))
			}
			return err
		}
		apply = func(ctx context.Context) error {
			return prepared.ApplyPreservingOriginalCreateResiduesV1(ctx, proof, revalidate)
		}
	}
	if err := apply(ctx); err != nil {
		return fmt.Errorf("apply runtime private CAS create-residue recovery: %w", err)
	}
	return nil
}

func recoverRuntimePrivateCASOrphanTopology(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	preservations ...runtimeReportRestartPreservationV1,
) (resultErr error) {
	if access == nil {
		return errors.New("runtime private CAS orphan-topology recovery authority is unavailable")
	}
	if len(preservations) > 1 {
		return errors.New("runtime private CAS orphan preservation is ambiguous")
	}
	var revalidate func(context.Context) error
	var originalDirectories []string
	attachmentDirectories := false
	var preservation runtimeReportRestartPreservationV1
	if len(preservations) == 1 {
		preserved := preservations[0]
		preservation = preserved
		if preserved.core != nil && preserved.core.roots.DataDir != dataDir {
			return errors.New("runtime private CAS orphan preservation root differs")
		}
		var err error
		originalDirectories, revalidate, err = preserved.prepareAttachmentDirectoryRevalidationV1(ctx, true)
		if err != nil {
			return err
		}
		attachmentDirectories = revalidate != nil
		publicationDirectories, publicationRevalidate, err := preserved.publication.prepareOriginalDirectoriesV1(ctx)
		if err != nil {
			return err
		}
		originalDirectories = append(originalDirectories, publicationDirectories...)
		if publicationRevalidate != nil {
			previousRevalidate := revalidate
			revalidate = func(ctx context.Context) error {
				var err error
				if previousRevalidate != nil {
					err = previousRevalidate(ctx)
				}
				return errors.Join(err, publicationRevalidate(ctx))
			}
		}
	}
	var prepared *finalauthority.PreparedSecurePrivateCASDirectoryRecoveryV1
	var err error
	var creates *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1
	core := preservation.core
	if core == nil && preservation.publication != nil {
		core = preservation.publication.core
	}
	if core != nil && core.originalCreates != nil {
		creates = core.originalCreates.combined
		if creates == nil {
			creates = core.originalCreates.attachment
		}
	}
	if revalidate != nil && creates != nil {
		prepared, err = finalauthority.PrepareSecurePrivateCASOrphanTopologyWithOriginalCreateResiduesV1(ctx, dataDir, access, creates)
	} else {
		prepared, err = finalauthority.PrepareSecurePrivateCASOrphanTopologyRecoveryV1(ctx, dataDir, access)
	}
	if err != nil {
		return fmt.Errorf("prepare runtime private CAS orphan-topology recovery: %w", err)
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return fmt.Errorf("revalidate runtime private CAS orphan-topology recovery: %w", err)
	}
	apply := prepared.Apply
	if revalidate != nil {
		// Native Apply proves its complete exact directory delta. Reopen the
		// semantic observation after recovery exclusion has been released so
		// allowed future-directory cleanup does not reuse stale physical pins.
		defer func() {
			if attachmentDirectories {
				resultErr = errors.Join(resultErr, preservation.validateAttachmentSemanticOperationsV1(ctx, nil, nil, ""))
			}
			if preservation.publication != nil {
				resultErr = errors.Join(resultErr, preservation.publication.ValidateSemanticOperationsV1(ctx, nil, nil, ""))
			}
		}()
		apply = func(ctx context.Context) error {
			return prepared.ApplyPreservingOriginalDirectoriesV1(ctx, originalDirectories, revalidate)
		}
	}
	if err := apply(ctx); err != nil {
		return fmt.Errorf("apply runtime private CAS orphan-topology recovery: %w", err)
	}
	return nil
}

func runtimePrivateCASExpectedRoots(dataDir string, recoveryGroupID string) []string {
	specs := domainprivatecas.RootsForRecoveryGroupV1(recoveryGroupID)
	roots := make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.LayoutKind == domainprivatecas.LayoutDetachedV1 &&
			spec.RelativeCASRoot == "checkpoint-snapshot-quarantine/audit-records" {
			if detached, err := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir); err == nil {
				roots = append(roots, detached)
				continue
			}
		}
		roots = append(roots, filepath.Join(dataDir, "private", filepath.FromSlash(spec.RelativeCASRoot)))
	}
	return roots
}

func runtimePrivateCASOwnerRecoveries(
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
) []runtimePrivateCASOwnerRecovery {
	backendGeneration := filepath.Join(dataDir, "private", "runtime-sidecar-authority-v1", "allocations")
	acceptedFinals := filepath.Join(dataDir, "private", "accepted-finals")
	caseEntities := filepath.Join(dataDir, "private", "case-entity")
	caseThreads := filepath.Join(dataDir, "private", "case-thread-authority")
	continuations := filepath.Join(dataDir, "private", "gate-continuations")
	pendingWork := filepath.Join(dataDir, "private", "pending-work")
	providerCacheTelemetry := filepath.Join(dataDir, "private", "provider-cache-telemetry")
	turnTerminalAuthority := filepath.Join(dataDir, "private", "turn-terminal-authority")
	attachmentAuthority := filepath.Join(dataDir, "private", "attachment-authority")
	authorityAdvance := filepath.Join(dataDir, "private", "authority-advance")
	evidenceAuthority := filepath.Join(dataDir, "private", "evidence-authority")
	evidenceRegistry := filepath.Join(dataDir, "private", "evidence-registry")
	datasetSnapshotAuthority := filepath.Join(dataDir, "private", "dataset-snapshot-authority")
	threadRiskPolicy := filepath.Join(dataDir, "private", "thread-risk-policy")
	checkpointAuthority := filepath.Join(dataDir, "private", "checkpoint-authority")
	piiAuthorization := filepath.Join(dataDir, "private", "pii-authorization")
	reportPublication := filepath.Join(dataDir, "private", "report-publication")
	controlledArtifactAccess := filepath.Join(dataDir, "private", "controlled-artifact-access")
	controlledArtifactAccessV2 := filepath.Join(dataDir, "private", "controlled-artifact-access-v2")
	_, legacyCheckpointAuditErr := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	return []runtimePrivateCASOwnerRecovery{
		{
			name:               "backend-generation",
			expectedRoots:      runtimePrivateCASExpectedRoots(dataDir, "backend-generation"),
			standaloneCASRoots: true,
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return backendgenerationfs.PrepareRecoveryV1(ctx, backendGeneration, access)
			},
		},
		{
			name:          "accepted-finals",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "accepted-finals"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return finalauthority.PreparePrivateStoreRecoveryV1(ctx, acceptedFinals, access)
			},
		},
		{
			name:          "case-entity",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "case-entity"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return caseentitystore.PrepareRecoveryV1(ctx, caseEntities, access)
			},
		},
		{
			name:               "case-thread-authority",
			expectedRoots:      runtimePrivateCASExpectedRoots(dataDir, "case-thread-authority"),
			standaloneCASRoots: true,
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return casethreadauthority.PrepareRecoveryV1(ctx, caseThreads, access)
			},
		},
		{
			name:          "gate-continuations",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "gate-continuations"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return continuationstore.PrepareRecoveryV1(ctx, continuations, access)
			},
		},
		{
			name:          "pending-work",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "pending-work"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return pendingworkstore.PrepareRecoveryV1(ctx, pendingWork, access)
			},
		},
		{
			name:          "provider-cache-telemetry",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "provider-cache-telemetry"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return cachetelemetrystore.PrepareRecoveryV1(ctx, providerCacheTelemetry, access)
			},
		},
		{
			name:          "turn-terminal-authority",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "turn-terminal-authority"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return turnterminalstore.PrepareRecoveryV1(ctx, turnTerminalAuthority, access)
			},
		},
		{
			name:          "attachment-authority",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "attachment-authority"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return attachmentauthority.PrepareRecoveryV1(ctx, attachmentAuthority, access)
			},
		},
		{
			name:          "authority-advance",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "authority-advance"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return authorityadvancefs.PrepareRecoveryV1(ctx, authorityAdvance, access)
			},
		},
		{
			name:          "evidence-authority",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "evidence-authority"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return evidenceauthoritystore.PrepareRecoveryV1(ctx, evidenceAuthority, access)
			},
		},
		{
			name:          "evidence-registry",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "evidence-registry"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return evidenceregistrystore.PrepareRecoveryV2(ctx, evidenceRegistry, access)
			},
		},
		{
			name:          "dataset-snapshot-authority",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "dataset-snapshot-authority"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return datasetsnapshotstore.PrepareRecoveryV2(ctx, datasetSnapshotAuthority, access)
			},
		},
		{
			name:               "thread-risk-policy",
			expectedRoots:      runtimePrivateCASExpectedRoots(dataDir, "thread-risk-policy"),
			standaloneCASRoots: true,
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return threadriskpolicystore.PrepareRecoveryV1(ctx, threadRiskPolicy, access)
			},
		},
		{
			name:          "pii-authorization",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "pii-authorization"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return piiauthorizationstore.PrepareRecoveryV1(ctx, piiAuthorization, access)
			},
		},
		{
			name:          "report-publication",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "report-publication"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return reportpublicationstore.PrepareRecoveryV1(ctx, reportPublication, access)
			},
		},
		{
			name:          "controlled-artifact-access",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "controlled-artifact-access"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return piiauthorizationstore.PrepareAccessRecoveryV1(ctx, controlledArtifactAccess, access)
			},
		},
		{
			name:          "controlled-artifact-access-v2",
			expectedRoots: runtimePrivateCASExpectedRoots(dataDir, "controlled-artifact-access-v2"),
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				return piiauthorizationstore.PrepareAccessRecoveryV2(ctx, controlledArtifactAccessV2, access)
			},
		},
		{
			name:                    "checkpoint-authority",
			expectedRoots:           runtimePrivateCASExpectedRoots(dataDir, "checkpoint-authority"),
			additionalTopologyCount: 1,
			prepare: func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
				if legacyCheckpointAuditErr != nil {
					return nil, fmt.Errorf("resolve legacy checkpoint audit root: %w", legacyCheckpointAuditErr)
				}
				return checkpointauthority.PrepareRecoveryV1(ctx, checkpointAuthority, access)
			},
			applyBeforeTransaction: func(ctx context.Context, prepared runtimePreparedPrivateCASOwnerRecovery) (bool, error) {
				checkpoint, ok := prepared.(*checkpointauthority.PreparedRecoveryV1)
				if !ok {
					return false, errors.New("checkpoint legacy migration authority has an unexpected type")
				}
				return checkpoint.ApplyLegacyCheckpointQuarantineMigrationV1(ctx)
			},
		},
	}
}

func validateRuntimePrivateCASOwnerRootManifest(
	owner runtimePrivateCASOwnerRecovery,
	plans []*finalauthority.PreparedSecurePrivateCASRecoveryV1,
) error {
	actual := make([]string, 0, len(plans))
	for _, plan := range plans {
		if plan == nil || plan.RootPath() == "" {
			return errors.New("runtime private CAS owner returned an invalid recovery root")
		}
		actual = append(actual, plan.RootPath())
	}
	expected := append([]string(nil), owner.expectedRoots...)
	sort.Strings(actual)
	sort.Strings(expected)
	if len(actual) != len(expected) {
		return errors.New("runtime private CAS owner recovery root manifest is incomplete")
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return errors.New("runtime private CAS owner recovery root manifest changed")
		}
		if index > 0 && actual[index] == actual[index-1] {
			return errors.New("runtime private CAS owner recovery root manifest repeats a root")
		}
	}
	return nil
}

func validateRuntimePrivateCASOwnerTopologyManifest(
	owner runtimePrivateCASOwnerRecovery,
	plans []*finalauthority.PreparedSecurePrivateCASRecoveryV1,
	topologies []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3,
) error {
	expectedCount := 1 + owner.additionalTopologyCount
	if owner.standaloneCASRoots {
		expectedCount = 0
	}
	if len(topologies) != expectedCount {
		return errors.New("runtime private CAS owner topology authority manifest is incomplete")
	}
	if owner.standaloneCASRoots {
		return nil
	}
	covered := make([]bool, len(topologies))
	for _, plan := range plans {
		matches := 0
		for index, topology := range topologies {
			if topology == nil || topology.PrivateCASRecoveryTopologyRootV3() == "" {
				return errors.New("runtime private CAS owner topology authority is invalid")
			}
			if filepath.Dir(plan.RootPath()) == topology.PrivateCASRecoveryTopologyRootV3() {
				matches++
				covered[index] = true
			}
		}
		if matches != 1 {
			return errors.New("runtime private CAS owner recovery root is not covered by exactly one parent topology")
		}
	}
	for _, used := range covered {
		if !used {
			return errors.New("runtime private CAS owner topology authority covers no recovery root")
		}
	}
	return nil
}

func prepareAndValidateRuntimePrivateCASOwners(
	ctx context.Context,
	owners []runtimePrivateCASOwnerRecovery,
	required bool,
	installation *finalauthority.AnchoredFileAuthority,
	preservations ...runtimeReportRestartPreservationV1,
) ([]runtimePreparedPrivateCASOwnerRecovery, error) {
	if len(preservations) > 1 {
		return nil, errors.New("runtime private CAS preservation binding is ambiguous")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !required {
		return []runtimePreparedPrivateCASOwnerRecovery{}, nil
	}
	prepared := make([]runtimePreparedPrivateCASOwnerRecovery, 0, len(owners))
	var publication *runtimePublicationSemanticPreservationV1
	if len(preservations) == 1 {
		publication = preservations[0].publication
	}
	var frozen map[string]*runtimeOptionalDomainBoundary
	if publication != nil {
		var err error
		frozen, err = prepareRuntimeFrozenPublicationOwnersV1(ctx, publication.core.roots.DataDir, publication.core.access, installation, publication)
		if err != nil {
			return nil, err
		}
	}
	for _, owner := range owners {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var plan runtimePreparedPrivateCASOwnerRecovery
		var err error
		if boundary := frozen[owner.name]; boundary != nil {
			plan = boundary
		} else if owner.name == "attachment-authority" && len(preservations) == 1 && preservations[0].attachments != nil {
			preserved := preservations[0]
			plan, err = attachmentauthority.PrepareRecoveryPreservingOriginalCreateResiduesV1(ctx, preserved.attachments.createResidues.OwnerRootV1(), preserved.core.access, preserved.attachments.createResidues)
		} else {
			plan, err = owner.prepare(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("prepare runtime private CAS owner %s: %w", owner.name, err)
		}
		prepared = append(prepared, plan)
	}
	for index, plan := range prepared {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := validateRuntimeOptionalDomainSemantics(ctx, plan, installation); err != nil {
			if _, domainOnly := err.(*finalauthority.DomainRecordUnavailableError); domainOnly && runtimeOptionalDomainOwner(owners[index].name) {
				boundary, boundaryErr := newRuntimeOptionalDomainBoundary(owners[index], plan, installation)
				if boundaryErr != nil {
					return nil, boundaryErr
				}
				if boundaryErr := boundary.Revalidate(ctx); boundaryErr != nil {
					return nil, boundaryErr
				}
				prepared[index] = boundary
				continue
			}
			_, domainOnly := err.(*finalauthority.DomainRecordUnavailableError)
			if !domainOnly || (owners[index].name != runtimePrivateCASCaseEntityOwnerV1 &&
				owners[index].name != runtimePrivateCASEvidenceRegistryOwnerV1) {
				return nil, fmt.Errorf("validate runtime private CAS owner %s: %w", owners[index].name, err)
			}
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			var boundary runtimePreparedPrivateCASOwnerRecovery
			var boundaryErr error
			if owners[index].name == runtimePrivateCASCaseEntityOwnerV1 {
				boundary, boundaryErr = newRuntimeCaseEntityBoundaryRecoveryV1(plan)
			} else {
				boundary, boundaryErr = newRuntimeEvidenceRegistryBoundaryRecoveryV1(plan)
			}
			if boundaryErr != nil {
				return nil, fmt.Errorf("prepare runtime private CAS owner %s boundary: %w", owners[index].name, boundaryErr)
			}
			if boundaryErr := boundary.ValidateSemantics(ctx); boundaryErr != nil {
				return nil, fmt.Errorf("validate runtime private CAS owner %s boundary: %w", owners[index].name, boundaryErr)
			}
			prepared[index] = boundary
		}
	}
	if err := isolateRuntimePublicationGraphFailures(ctx, owners, prepared, installation); err != nil {
		return nil, err
	}
	if len(preservations) == 1 {
		for index, owner := range owners {
			if owner.name != "attachment-authority" {
				continue
			}
			bound, err := preservations[0].bindAttachmentPrivateCASPreservationV1(ctx, prepared[index])
			if err != nil {
				return nil, err
			}
			prepared[index] = bound
		}
	}
	for index, plan := range prepared {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := plan.Revalidate(ctx); err != nil {
			return nil, fmt.Errorf("revalidate runtime private CAS owner %s: %w", owners[index].name, err)
		}
	}
	return prepared, nil
}

func runtimePrivateCASRecoveryRootCatalogV4(dataDir string) (map[string]string, error) {
	catalog := make(map[string]string, len(domainprivatecas.RuntimeRootSpecsV1()))
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		root := filepath.Join(dataDir, "private", filepath.FromSlash(spec.RelativeCASRoot))
		if spec.LayoutKind == domainprivatecas.LayoutDetachedV1 &&
			spec.RelativeCASRoot == "checkpoint-snapshot-quarantine/audit-records" {
			var err error
			root, err = persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
			if err != nil {
				return nil, err
			}
		}
		if previous, duplicate := catalog[root]; duplicate && previous != spec.RootID {
			return nil, errors.New("runtime private CAS logical root catalog aliases a host path")
		}
		catalog[root] = spec.RootID
	}
	return catalog, nil
}

func runtimePrivateCASSemanticAuthorityDigestV4(
	ownerName string,
	prepared runtimePreparedPrivateCASOwnerRecovery,
) (string, error) {
	additional := ""
	if authority, ok := prepared.(runtimePrivateCASAdditionalSemanticAuthorityV4); ok {
		additional = authority.PrivateCASRecoveryAdditionalSemanticDigestV4()
		if !domainprivatecas.ValidDigestV1(additional) {
			return "", errors.New("runtime private CAS additional semantic authority is invalid")
		}
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(runtimePrivateCASSemanticValidatorVersionV4))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(ownerName))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(additional))
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func runtimePrivateCASRecoveryParticipantsV4(
	dataDir string,
	owners []runtimePrivateCASOwnerRecovery,
	prepared []runtimePreparedPrivateCASOwnerRecovery,
	required bool,
) ([]finalauthority.PreparedSecurePrivateCASRecoveryParticipantV4, error) {
	if !required {
		if len(prepared) != 0 {
			return nil, errors.New("runtime private CAS unused participant manifest is not empty")
		}
		return []finalauthority.PreparedSecurePrivateCASRecoveryParticipantV4{}, nil
	}
	if len(owners) != len(prepared) {
		return nil, errors.New("runtime private CAS prepared participant count changed")
	}
	catalog, err := runtimePrivateCASRecoveryRootCatalogV4(dataDir)
	if err != nil {
		return nil, err
	}
	participants := make([]finalauthority.PreparedSecurePrivateCASRecoveryParticipantV4, 0, len(prepared))
	seenRoots := make(map[string]struct{}, len(catalog))
	for index, authority := range prepared {
		plans := authority.SecurePrivateCASRecoveryPlansV2()
		roots := make([]finalauthority.SecurePrivateCASRecoveryRootBindingV4, 0, len(plans))
		for _, plan := range plans {
			if plan == nil {
				return nil, errors.New("runtime private CAS V4 participant returned a nil plan")
			}
			rootID, known := catalog[plan.RootPath()]
			if !known {
				return nil, errors.New("runtime private CAS V4 participant returned an unregistered root")
			}
			if _, duplicate := seenRoots[rootID]; duplicate {
				return nil, errors.New("runtime private CAS V4 participant repeats a logical root")
			}
			seenRoots[rootID] = struct{}{}
			roots = append(roots, finalauthority.SecurePrivateCASRecoveryRootBindingV4{
				RootID: rootID, RootPath: plan.RootPath(),
			})
		}
		semanticDigest, err := runtimePrivateCASSemanticAuthorityDigestV4(owners[index].name, authority)
		if err != nil {
			return nil, fmt.Errorf("runtime private CAS owner %s semantic authority: %w", owners[index].name, err)
		}
		participants = append(participants, finalauthority.PreparedSecurePrivateCASRecoveryParticipantV4{
			ParticipantID: owners[index].name, SemanticAuthorityDigest: semanticDigest,
			Authority: authority, Roots: roots,
		})
	}
	if len(seenRoots) != len(catalog) {
		return nil, errors.New("runtime private CAS V4 participant manifest is incomplete")
	}
	return participants, nil
}

// recoverRuntimePrivateCASOwners freezes every exact owner/root manifest and
// validates all owner semantics before any mutation. An active signed journal
// resumes its already-frozen authority without replaying owner migrations. A
// new transaction applies those migrations, prepares every owner again, and
// then enters the installation-signed V4 engine.
func recoverRuntimePrivateCASOwners(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	installation *finalauthority.AnchoredFileAuthority,
	journals ...privatecasport.RecoveryJournalV1,
) error {
	return recoverRuntimePrivateCASOwnersWithPreservationV1(ctx, dataDir, access, installation, runtimeReportRestartPreservationV1{}, journals...)
}

func recoverRuntimePrivateCASOwnersWithPreservationV1(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	installation *finalauthority.AnchoredFileAuthority,
	preservation runtimeReportRestartPreservationV1,
	journals ...privatecasport.RecoveryJournalV1,
) error {
	if access == nil || len(journals) > 1 {
		return errors.New("runtime private CAS recovery authority is unavailable")
	}
	var journal privatecasport.RecoveryJournalV1
	if len(journals) == 1 {
		journal = journals[0]
	} else if lease, ok := access.(*persistencefs.CompositeLease); ok {
		var err error
		journal, err = persistencefs.NewPrivateCASRecoveryJournalV1(lease)
		if err != nil {
			return fmt.Errorf("construct runtime private CAS recovery journal: %w", err)
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := domainprivatecas.ValidateRuntimeDirectorySlotsV1(); err != nil {
		return err
	}
	if journal == nil {
		return errors.New("runtime private CAS signed recovery journal is unavailable after global preflight")
	}
	_, journalErr := journal.Load(ctx)
	resumeSignedRecovery := journalErr == nil
	if journalErr != nil && !errors.Is(journalErr, privatecasport.ErrJournalAbsent) {
		return fmt.Errorf("load runtime private CAS signed recovery journal: %w", journalErr)
	}
	if !resumeSignedRecovery {
		privateStateAbsent, err := runtimePrivateCASRecoveryStateStablyAbsent(ctx, dataDir, access)
		if err != nil {
			return err
		}
		if privateStateAbsent {
			return nil
		}
	}
	owners := runtimePrivateCASOwnerRecoveries(dataDir, access)
	prepared, err := prepareAndValidateRuntimePrivateCASOwners(ctx, owners, true, installation, preservation)
	if err != nil {
		return err
	}
	if !resumeSignedRecovery {
		ownerInventoryChanged := false
		for index, plan := range prepared {
			if err := ctx.Err(); err != nil {
				return err
			}
			if owners[index].applyBeforeTransaction == nil {
				continue
			}
			changed, err := owners[index].applyBeforeTransaction(ctx, plan)
			if err != nil {
				return fmt.Errorf("apply pre-transaction runtime private CAS owner %s: %w", owners[index].name, err)
			}
			ownerInventoryChanged = ownerInventoryChanged || changed
		}
		if ownerInventoryChanged {
			prepared, err = prepareAndValidateRuntimePrivateCASOwners(ctx, owners, true, installation, preservation)
			if err != nil {
				return err
			}
		}
	}
	for index, plan := range prepared {
		ownerLeaves := plan.SecurePrivateCASRecoveryPlansV2()
		if err := validateRuntimePrivateCASOwnerRootManifest(owners[index], ownerLeaves); err != nil {
			return fmt.Errorf("runtime private CAS owner %s manifest: %w", owners[index].name, err)
		}
		if err := validateRuntimePrivateCASOwnerTopologyManifest(
			owners[index], ownerLeaves, plan.PrivateCASRecoveryTopologiesV3(),
		); err != nil {
			return fmt.Errorf("runtime private CAS owner %s topology manifest: %w", owners[index].name, err)
		}
	}
	participants, err := runtimePrivateCASRecoveryParticipantsV4(dataDir, owners, prepared, true)
	if err != nil {
		return err
	}
	if err := finalauthority.ApplyPreparedSecurePrivateCASRecoveryTransactionV4(ctx, participants, journal); err != nil {
		return fmt.Errorf("apply runtime private CAS signed quarantine transaction V4: %w", err)
	}
	return nil
}

func runtimePrivateCASRecoveryStateStablyAbsent(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
) (bool, error) {
	roots := []string{
		filepath.Join(dataDir, "private"),
		filepath.Join(dataDir, persistencefs.LegacyCheckpointSnapshotDirectoryV1),
	}
	for observation := 0; observation < 2; observation++ {
		for _, root := range roots {
			present, err := finalauthority.SecurePrivateCASDirectoryPresentWithAccessAuthorityContext(
				ctx, root, access,
			)
			if err != nil {
				return false, fmt.Errorf("observe runtime private CAS recovery state absence: %w", err)
			}
			if present {
				return false, nil
			}
		}
	}
	return true, nil
}

func withRetiredRuntimePrivateCASOwnerGenerationsForSemanticApply(
	ctx context.Context,
	dataDir string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	installation *finalauthority.AnchoredFileAuthority,
	plan domainstartup.SemanticStartupPlanV1,
	apply func(context.Context) error,
	preservations ...runtimeReportRestartPreservationV1,
) error {
	if access == nil || apply == nil {
		return errors.New("runtime private CAS semantic apply retirement authority is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requiresOwnerPreparation := runtimePrivateCASSemanticPlanMayAffectPrivateCAS(plan)
	owners := runtimePrivateCASOwnerRecoveries(dataDir, access)
	prepared, err := prepareAndValidateRuntimePrivateCASOwners(ctx, owners, requiresOwnerPreparation, installation, preservations...)
	if err != nil {
		return err
	}
	participants, err := runtimePrivateCASRecoveryParticipantsV4(
		dataDir, owners, prepared, requiresOwnerPreparation,
	)
	if err != nil {
		return err
	}
	affectedRootIDs, err := runtimePrivateCASSemanticApplyRootIDs(plan)
	if err != nil {
		return err
	}
	if err := finalauthority.WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
		ctx, participants, affectedRootIDs, apply,
	); err != nil {
		return fmt.Errorf("apply runtime semantic plan after private CAS generation retirement: %w", err)
	}
	return nil
}

// runtimePrivateCASSemanticPlanMayAffectPrivateCAS is deliberately
// conservative and does not replace exact plan validation below. It only
// avoids preparing every private owner when no operation can intersect the
// private CAS subtree. Any disagreement fails closed when the exact root set
// is checked against the empty finalauthority manifest before apply.
func runtimePrivateCASSemanticPlanMayAffectPrivateCAS(plan domainstartup.SemanticStartupPlanV1) bool {
	for _, operation := range plan.Operations {
		for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
			rootLabel := "data/private/" + spec.RelativeCASRoot
			if operation.Path == rootLabel ||
				strings.HasPrefix(operation.Path, rootLabel+"/") ||
				strings.HasPrefix(rootLabel, operation.Path+"/") {
				return true
			}
		}
	}
	return false
}

func runtimePrivateCASSemanticApplyRootIDs(
	plan domainstartup.SemanticStartupPlanV1,
) ([]string, error) {
	if err := domainstartup.ValidateSemanticStartupPlanV1(plan); err != nil {
		return nil, err
	}
	affected := make([]string, 0)
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		rootLabel := "data/private/" + spec.RelativeCASRoot
		for _, operation := range plan.Operations {
			if operation.Path == rootLabel ||
				strings.HasPrefix(operation.Path, rootLabel+"/") ||
				strings.HasPrefix(rootLabel, operation.Path+"/") {
				affected = append(affected, spec.RootID)
				break
			}
		}
	}
	sort.Strings(affected)
	return affected, nil
}
