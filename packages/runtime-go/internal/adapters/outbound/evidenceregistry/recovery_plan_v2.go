package evidenceregistry

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	maxRegistryOwnerEntries = 64
	maxRegistrySiblingBytes = 64 << 20
)

// PreparedRecoveryV2 binds the V2 CAS leaves and the complete registry parent
// topology. Legacy V1 siblings remain frozen blockers: recovery never scans,
// upgrades, deletes, or interprets them as current V2 authority.
type PreparedRecoveryV2 struct {
	root               string
	topology           *finalauthorityadapter.PreparedSecurePrivateCASOwnerTopologyV1
	indexes            *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1
	capsules           *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1
	incompleteV2       bool
	unknownInventory   bool
	legacyLockBlocked  bool
	legacyV1Allowed    bool
	witnessedV2Allowed bool
	freshImportShape   bool
	authorityEmpty     bool
	validated          bool
}

func PrepareRecoveryV2(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV2, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("evidence registry V2 prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("evidence registry V2 prepared recovery root is invalid")
	}
	indexes, err := finalauthorityadapter.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, filepath.Join(absolute, domainprivatecas.EvidenceRegistryIndexesLeafV2), maxEvidenceRegistryAuthorityIndexV2Bytes, access,
	)
	if err != nil {
		return nil, err
	}
	capsules, err := finalauthorityadapter.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, filepath.Join(absolute, domainprivatecas.EvidenceRegistryCapsulesLeafV2), maxEvidenceRegistryAuthorityCapsuleV2Bytes, access,
	)
	if err != nil {
		return nil, err
	}
	topology, entries, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(
		ctx, absolute,
		[]string{domainprivatecas.EvidenceRegistryIndexesLeafV2, domainprivatecas.EvidenceRegistryCapsulesLeafV2},
		maxRegistryOwnerEntries, maxRegistrySiblingBytes, access,
	)
	if err != nil {
		return nil, err
	}
	knownNames := []string{
		domainprivatecas.EvidenceRegistryIndexesLeafV2, domainprivatecas.EvidenceRegistryCapsulesLeafV2,
		domainprivatecas.EvidenceRegistryLegacyAuthorityIndexV1, domainprivatecas.EvidenceRegistryLegacyLockV1,
		domainprivatecas.EvidenceRegistryLegacyCapsulesLeafV1, domainprivatecas.EvidenceRegistryLegacyProjectionsV1,
	}
	seen := make(map[string]bool, len(entries))
	unknownInventory := false
	legacyLockBlocked := false
	for _, entry := range entries {
		name := entry.Name
		for _, known := range knownNames {
			if strings.EqualFold(name, known) && name != known {
				return nil, errors.New("evidence registry physical inventory aliases a reserved entry by case")
			}
		}
		seen[name] = true
		switch name {
		case domainprivatecas.EvidenceRegistryIndexesLeafV2, domainprivatecas.EvidenceRegistryCapsulesLeafV2:
			// The leaf recovery plans independently bind and validate CAS kind.
		case domainprivatecas.EvidenceRegistryLegacyCapsulesLeafV1, domainprivatecas.EvidenceRegistryLegacyProjectionsV1:
			if !entry.Directory {
				unknownInventory = true
			}
		case domainprivatecas.EvidenceRegistryLegacyAuthorityIndexV1, domainprivatecas.EvidenceRegistryLegacyLockV1:
			if entry.Directory {
				unknownInventory = true
			}
		default:
			unknownInventory = true
		}
		if name == domainprivatecas.EvidenceRegistryLegacyLockV1 && entry.ByteLength != 0 {
			legacyLockBlocked = true
		}
	}
	legacyDataCount := 0
	for _, name := range []string{
		domainprivatecas.EvidenceRegistryLegacyAuthorityIndexV1,
		domainprivatecas.EvidenceRegistryLegacyCapsulesLeafV1,
		domainprivatecas.EvidenceRegistryLegacyProjectionsV1,
	} {
		if seen[name] {
			legacyDataCount++
		}
	}
	legacyV1Allowed := !unknownInventory && !legacyLockBlocked && !indexes.Present() && !capsules.Present() &&
		legacyDataCount == 3 && seen[domainprivatecas.EvidenceRegistryLegacyLockV1]
	witnessedV2Allowed := !unknownInventory && !legacyLockBlocked &&
		indexes.Present() == capsules.Present() && legacyDataCount == 0
	return &PreparedRecoveryV2{
		root: absolute, topology: topology, indexes: indexes, capsules: capsules,
		incompleteV2: indexes.Present() != capsules.Present(), unknownInventory: unknownInventory,
		legacyLockBlocked: legacyLockBlocked, legacyV1Allowed: legacyV1Allowed,
		witnessedV2Allowed: witnessedV2Allowed,
		freshImportShape:   witnessedV2Allowed && !seen[domainprivatecas.EvidenceRegistryLegacyLockV1],
	}, nil
}

func (prepared *PreparedRecoveryV2) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil || prepared.indexes == nil || prepared.capsules == nil {
		return errors.New("evidence registry V2 recovery plan is invalid")
	}
	prepared.validated = false
	err := finalauthorityadapter.ValidatePreparedPrivateCASDomainSemanticsV1(ctx,
		map[string]*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1{
			domainprivatecas.EvidenceRegistryIndexesLeafV2:  prepared.indexes,
			domainprivatecas.EvidenceRegistryCapsulesLeafV2: prepared.capsules,
		}, prepared.RevalidatePhysicalV2, func(visit finalauthorityadapter.PrivateCASDomainVisitor) error {
			if prepared.incompleteV2 || prepared.unknownInventory || prepared.legacyLockBlocked {
				return errors.New("evidence registry authority is semantically unavailable")
			}
			indexes := make(map[string]domainevidence.EvidenceRegistryAuthorityIndexV2)
			capsules := make(map[string]domainevidence.EvidenceRegistryAuthorityCapsule)
			if err := visit(domainprivatecas.EvidenceRegistryIndexesLeafV2, func(file finalauthorityadapter.SecurePrivateCASFile) error {
				index, err := domainevidence.ParseEvidenceRegistryAuthorityIndexV2(file.Body)
				if err != nil || index.IndexDigest != file.Digest {
					return errors.Join(errors.New("evidence registry V2 recovery contains a corrupt index"), err)
				}
				indexes[file.Digest] = index
				return nil
			}); err != nil {
				return err
			}
			if err := visit(domainprivatecas.EvidenceRegistryCapsulesLeafV2, func(file finalauthorityadapter.SecurePrivateCASFile) error {
				capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(file.Body)
				if err != nil || capsule.RecordDigest != file.Digest ||
					domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(capsule.SecurityContext) != nil {
					return errors.Join(errors.New("evidence registry V2 recovery contains a corrupt capsule"), err)
				}
				capsules[file.Digest] = capsule
				return nil
			}); err != nil {
				return err
			}
			for _, index := range indexes {
				capsule, found := capsules[index.Entry.CapsuleRecordDigest]
				if !found || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(index, capsule) {
					return errors.New("evidence registry V2 recovery index lost its exact capsule")
				}
				if index.Generation == 1 {
					if index.PreviousIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
						return errors.New("evidence registry V2 recovery genesis changed")
					}
					continue
				}
				previous, found := indexes[index.PreviousIndexDigest]
				if !found || domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(previous, index) != nil {
					return errors.New("evidence registry V2 recovery index lost its exact predecessor")
				}
			}
			lineages := make(map[string][]domainevidence.EvidenceRegistryAuthorityIndexV2)
			for _, index := range indexes {
				identity := index.Entry.ThreadID + "\x00" + index.Entry.TurnID
				lineages[identity] = append(lineages[identity], index)
			}
			for _, lineage := range lineages {
				sort.Slice(lineage, func(i, j int) bool { return lineage[i].Generation < lineage[j].Generation })
				if lineage[0].Entry.RegistrySequence != 1 {
					return errors.New("evidence registry V2 recovery identity does not start at sequence one")
				}
				for position := 1; position < len(lineage); position++ {
					capsule := capsules[lineage[position].Entry.CapsuleRecordDigest]
					if domainevidence.ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(
						lineage[position-1].Entry, lineage[position].Entry, capsule,
					) != nil {
						return errors.New("evidence registry V2 recovery identity extension changed")
					}
				}
			}
			prepared.authorityEmpty = len(indexes) == 0 && len(capsules) == 0
			return nil
		})
	if err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

// LegacyV1ActivationAllowed reports only a host-private activation decision
// after the complete owner has been physically bound. It is not registry
// authority and does not inspect, upgrade, or repair any legacy record.
func (prepared *PreparedRecoveryV2) LegacyV1ActivationAllowed() bool {
	return prepared != nil && prepared.topology != nil && prepared.legacyV1Allowed
}

func (prepared *PreparedRecoveryV2) AuthorityKnownEmptyV2() bool {
	return prepared != nil && prepared.validated && prepared.authorityEmpty
}

// HasStateV2 reports the physically bound owner-container presence. Any empty,
// partial, legacy, unknown-safe, or current inventory is state; only a proven
// absent owner returns false. Unsafe inventory fails during preparation.
func (prepared *PreparedRecoveryV2) HasStateV2() bool {
	return prepared != nil && prepared.topology != nil && prepared.topology.PresentV1()
}

// WitnessedV2ActivationAllowed reports whether the handle-bound complete
// inventory contains a semantically valid, non-empty current V2 pair plus an
// optional empty legacy lock. Absent and generation-zero inventories remain an
// unavailable optional capability and are never initialized by observation.
func (prepared *PreparedRecoveryV2) WitnessedV2ActivationAllowed() bool {
	return prepared != nil && prepared.validated && prepared.witnessedV2Allowed &&
		prepared.indexes.Present() && prepared.capsules.Present() && !prepared.authorityEmpty
}

// FreshImportInventoryV2 is only a physical/semantic prerequisite for an
// explicit Host import. It grants no activation authority: the caller must
// also prove the freshly authenticated registry child is canonical zero and
// revalidate the exact topology before writing. Startup must not use it to
// activate or initialize a registry.
func (prepared *PreparedRecoveryV2) FreshImportInventoryV2() bool {
	return prepared != nil && prepared.validated && prepared.freshImportShape && prepared.authorityEmpty
}

func (prepared *PreparedRecoveryV2) Revalidate(ctx context.Context) error {
	if prepared == nil || !prepared.validated {
		return errors.New("evidence registry V2 recovery plan is not validated")
	}
	return prepared.RevalidatePhysicalV2(ctx)
}

// RevalidatePhysicalV2 repeats only the handle-bound topology and CAS identity
// checks. Runtime composition uses it to preserve an optional semantic blocker
// without treating the blocked bytes as an empty or usable registry.
func (prepared *PreparedRecoveryV2) RevalidatePhysicalV2(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil || prepared.indexes == nil || prepared.capsules == nil {
		return errors.New("evidence registry V2 physical recovery plan is invalid")
	}
	if err := prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return err
	}
	if err := prepared.indexes.Revalidate(ctx); err != nil {
		return err
	}
	return prepared.capsules.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV2) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.topology == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.topology}
}

func (prepared *PreparedRecoveryV2) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.indexes == nil || prepared.capsules == nil {
		return nil
	}
	return []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1{prepared.indexes, prepared.capsules}
}
