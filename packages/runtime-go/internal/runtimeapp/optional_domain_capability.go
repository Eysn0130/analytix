package runtimeapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

var errRuntimeReportRestartReconciliationRequired = errors.New("report publication restart reconciliation is required before runtime activation")
var errRuntimeOptionalDomainInstallationRequired = errors.New("unavailable signed domain requires independent installation authority")

func runtimeOptionalDomainOwner(name string) bool {
	switch name {
	case "evidence-authority", "dataset-snapshot-authority", "pii-authorization",
		"report-publication", "controlled-artifact-access", "controlled-artifact-access-v2":
		return true
	default:
		return false
	}
}

type runtimeOptionalDomainBoundary struct {
	name           string
	installation   *finalauthority.AnchoredFileAuthority
	plans          []*finalauthority.PreparedSecurePrivateCASRecoveryV1
	topologies     []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3
	original       *finalauthority.FrozenOriginalFixedOwnerRecoveryV1
	reportDeferred bool
}

func newRuntimeOptionalDomainBoundary(owner runtimePrivateCASOwnerRecovery, prepared runtimePreparedPrivateCASOwnerRecovery, installation *finalauthority.AnchoredFileAuthority) (*runtimeOptionalDomainBoundary, error) {
	if !runtimeOptionalDomainOwner(owner.name) || prepared == nil {
		return nil, errors.New("optional domain recovery owner is invalid")
	}
	if boundary, ok := prepared.(*runtimeOptionalDomainBoundary); ok {
		if boundary.name != owner.name || boundary.installation != installation {
			return nil, errors.New("optional domain original boundary binding changed")
		}
		return boundary, nil
	}
	boundary := &runtimeOptionalDomainBoundary{
		name: owner.name, installation: installation, plans: prepared.SecurePrivateCASRecoveryPlansV2(),
		topologies: prepared.PrivateCASRecoveryTopologiesV3(),
	}
	boundary.original, _ = prepared.(*finalauthority.FrozenOriginalFixedOwnerRecoveryV1)
	if len(boundary.plans) != len(owner.expectedRoots) || len(boundary.plans) == 0 || len(boundary.topologies) != 1 {
		return nil, errors.New("optional domain recovery boundary is incomplete")
	}
	return boundary, nil
}

func (boundary *runtimeOptionalDomainBoundary) ValidateSemantics(ctx context.Context) error {
	return boundary.Revalidate(ctx)
}

func (boundary *runtimeOptionalDomainBoundary) Revalidate(ctx context.Context) error {
	if boundary == nil || !runtimeOptionalDomainOwner(boundary.name) || len(boundary.plans) == 0 || len(boundary.topologies) != 1 {
		return errors.New("optional domain recovery boundary is invalid")
	}
	if boundary.installation != nil {
		if err := boundary.installation.ValidateCurrentInstallation(ctx); err != nil {
			return err
		}
	}
	if boundary.original != nil {
		if err := boundary.original.Revalidate(ctx); err != nil {
			return err
		}
		if boundary.installation != nil {
			return boundary.installation.ValidateCurrentInstallation(ctx)
		}
		return nil
	}
	for _, plan := range boundary.plans {
		if plan == nil {
			return errors.New("optional domain recovery leaf is unavailable")
		}
		if err := plan.Revalidate(ctx); err != nil {
			return err
		}
	}
	for _, topology := range boundary.topologies {
		if topology == nil {
			return errors.New("optional domain recovery topology is unavailable")
		}
		if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
			return err
		}
	}
	if boundary.installation != nil {
		return boundary.installation.ValidateCurrentInstallation(ctx)
	}
	return nil
}

func (boundary *runtimeOptionalDomainBoundary) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	return append([]*finalauthority.PreparedSecurePrivateCASRecoveryV1(nil), boundary.plans...)
}

func (boundary *runtimeOptionalDomainBoundary) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return append([]finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3(nil), boundary.topologies...)
}

func (boundary *runtimeOptionalDomainBoundary) PrivateCASRecoveryAdditionalSemanticDigestV4() string {
	reason := "domain-unavailable"
	if boundary.reportDeferred {
		reason = "report-restart-deferred"
	}
	digest := sha256.Sum256([]byte(runtimePrivateCASSemanticValidatorVersionV4 + "\x00" + reason + "\x00" + boundary.name))
	return hex.EncodeToString(digest[:])
}

func (boundary *runtimeOptionalDomainBoundary) FreezeSecurePrivateCASRecoveryTargetsV4() bool {
	return true
}

type runtimeOptionalDomainCapability struct {
	available  bool
	hasRecords bool
	prepared   runtimePreparedPrivateCASOwnerRecovery
}

func prepareRuntimeOptionalDomainCapabilities(ctx context.Context, dataDir string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority, originals ...*runtimePublicationSemanticPreservationV1) (map[string]runtimeOptionalDomainCapability, error) {
	return prepareRuntimeOptionalDomainCapabilitiesForOwnersV1(ctx, dataDir, access, installation, runtimeOptionalDomainOwner, originals...)
}

func prepareRuntimeOptionalDomainCapabilitiesForOwnersV1(ctx context.Context, dataDir string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority, includes func(string) bool, originals ...*runtimePublicationSemanticPreservationV1) (map[string]runtimeOptionalDomainCapability, error) {
	frozen, err := prepareRuntimeFrozenPublicationOwnersV1(ctx, dataDir, access, installation, originals...)
	if err != nil {
		return nil, err
	}
	capabilities := make(map[string]runtimeOptionalDomainCapability)
	var owners []runtimePrivateCASOwnerRecovery
	var plans []runtimePreparedPrivateCASOwnerRecovery
	for _, owner := range runtimePrivateCASOwnerRecoveries(dataDir, access) {
		if !includes(owner.name) {
			continue
		}
		var prepared runtimePreparedPrivateCASOwnerRecovery
		available := true
		if boundary := frozen[owner.name]; boundary != nil {
			prepared, available = boundary, false
		} else {
			prepared, err = owner.prepare(ctx)
		}
		if err != nil {
			return nil, err
		}
		if err := validateRuntimeOptionalDomainSemantics(ctx, prepared, installation); err != nil {
			// Only this exact producer result is degradable. Joined or wrapped
			// physical/control errors never inherit domain-unavailable authority.
			if _, domainOnly := err.(*finalauthority.DomainRecordUnavailableError); !domainOnly {
				return nil, fmt.Errorf("validate optional domain %s: %w", owner.name, err)
			}
			boundary, err := newRuntimeOptionalDomainBoundary(owner, prepared, installation)
			if err != nil {
				return nil, err
			}
			prepared, available = boundary, false
		}
		if err := prepared.Revalidate(ctx); err != nil {
			return nil, err
		}
		hasRecords, err := runtimePreparedPrivateCASHasRecordsV1(ctx, prepared)
		if err != nil {
			return nil, err
		}
		capabilities[owner.name] = runtimeOptionalDomainCapability{available: available, hasRecords: hasRecords, prepared: prepared}
		owners = append(owners, owner)
		plans = append(plans, prepared)
	}
	if err := isolateRuntimePublicationGraphFailures(ctx, owners, plans, installation); err != nil {
		return nil, err
	}
	for index, owner := range owners {
		if _, unavailable := plans[index].(*runtimeOptionalDomainBoundary); unavailable {
			capability := capabilities[owner.name]
			capability.available, capability.prepared = false, plans[index]
			capabilities[owner.name] = capability
		}
	}
	return capabilities, nil
}

func loadRuntimeOptionalDomainInstallation(ctx context.Context, config Config, roots persistencefs.RootSet, rootAuthority *persistencefs.RootAuthority) (*finalauthority.AnchoredFileAuthority, error) {
	enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentConfigV2(ctx, config)
	if err != nil || !configured {
		return nil, err
	}
	anchor, err := finalauthority.NewExistingFileAuthorityAnchor(rootAuthority, enrolled.projection.InstallationAuthorityKeyID, enrolled.projection.InstallationAuthorityPublicKey)
	if err != nil {
		return nil, err
	}
	return anchor.Open(filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"))
}

func validateRuntimeOptionalDomainSemantics(ctx context.Context, prepared runtimePreparedPrivateCASOwnerRecovery, installation *finalauthority.AnchoredFileAuthority) error {
	if domain, ok := prepared.(interface {
		ValidateInstallation(context.Context, *finalauthority.AnchoredFileAuthority) error
	}); ok {
		if installation != nil {
			return domain.ValidateInstallation(ctx, installation)
		}
		err := prepared.ValidateSemantics(ctx)
		if _, domainOnly := err.(*finalauthority.DomainRecordUnavailableError); domainOnly {
			// Self-consistent local key bytes cannot prove the original
			// installation identity when its signed records are unusable.
			// Keep this global before any dependency-closure suppression can
			// hide another record's installation mismatch.
			return errors.Join(errRuntimeOptionalDomainInstallationRequired, err)
		}
		return err
	}
	return prepared.ValidateSemantics(ctx)
}
