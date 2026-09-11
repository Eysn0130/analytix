package privatecastopology

import (
	"errors"
	"path"
	"sort"
	"strings"
)

// DirectorySlotV1 is one fixed runtime directory relative to DataDir. A slot
// is a path topology fact only; it grants no filesystem or deletion authority.
type DirectorySlotV1 struct {
	RelativePath       string
	ParentRelativePath string
	Component          string
	CASRoot            bool
}

type OwnerDirectoryGroupV1 struct {
	RecoveryGroupID           string
	OwnerRelativePath         string
	LeafComponents            []string
	MigrationLeafComponents   []string
	FrozenDirectoryComponents []string
	RegularSiblingComponents  []string
	DeferOrphanRecovery       bool
}

const (
	EvidenceRegistryRecoveryGroupID        = "evidence-registry"
	EvidenceRegistryIndexesLeafV2          = "indexes"
	EvidenceRegistryCapsulesLeafV2         = "capsules"
	EvidenceRegistryLegacyAuthorityIndexV1 = ".registry-authority-index.json"
	EvidenceRegistryLegacyCapsulesLeafV1   = ".registry-capsules"
	EvidenceRegistryLegacyProjectionsV1    = ".registry-projections"
	EvidenceRegistryLegacyLockV1           = ".registry.lock"
)

func FixedDirectorySlotsV1() []DirectorySlotV1 {
	casRoots := make(map[string]struct{}, len(runtimeRootSpecsV1))
	paths := map[string]struct{}{"private": {}}
	for _, spec := range runtimeRootSpecsV1 {
		current := "private"
		for _, component := range strings.Split(spec.RelativeCASRoot, "/") {
			current = path.Join(current, component)
			paths[current] = struct{}{}
		}
		casRoots[path.Join("private", spec.RelativeCASRoot)] = struct{}{}
	}
	slots := make([]DirectorySlotV1, 0, len(paths))
	for relative := range paths {
		parent := path.Dir(relative)
		if parent == "" {
			parent = "."
		}
		_, casRoot := casRoots[relative]
		slots = append(slots, DirectorySlotV1{
			RelativePath: relative, ParentRelativePath: parent,
			Component: path.Base(relative), CASRoot: casRoot,
		})
	}
	sort.Slice(slots, func(left int, right int) bool {
		leftDepth := strings.Count(slots[left].RelativePath, "/")
		rightDepth := strings.Count(slots[right].RelativePath, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return slots[left].RelativePath < slots[right].RelativePath
	})
	return slots
}

func FixedDirectoryParentPathsV1() []string {
	parents := make(map[string]struct{})
	for _, slot := range FixedDirectorySlotsV1() {
		parents[slot.ParentRelativePath] = struct{}{}
	}
	return sortedPrivateCASPathSetV1(parents)
}

func ShardParentPathsV1() []string {
	parents := make(map[string]struct{}, len(runtimeRootSpecsV1))
	for _, spec := range runtimeRootSpecsV1 {
		parents[path.Join("private", spec.RelativeCASRoot)] = struct{}{}
	}
	return sortedPrivateCASPathSetV1(parents)
}

func CreateRecoveryScanParentPathsV1() []string {
	parents := make(map[string]struct{})
	for _, parent := range FixedDirectoryParentPathsV1() {
		parents[parent] = struct{}{}
	}
	for _, parent := range ShardParentPathsV1() {
		parents[parent] = struct{}{}
	}
	return sortedPrivateCASPathSetV1(parents)
}

func CanonicalShardComponentsV1() []string {
	shards := make([]string, 0, 256)
	for high := 0; high < 16; high++ {
		for low := 0; low < 16; low++ {
			shards = append(shards, string([]byte{lowerHexDigit(high), lowerHexDigit(low)}))
		}
	}
	return shards
}

func MaximumCreateResidueCandidateLocationsV1() int {
	return len(FixedDirectorySlotsV1()) + len(ShardParentPathsV1())*len(CanonicalShardComponentsV1())
}

func RecoverableOwnerDirectoryGroupsV1() []OwnerDirectoryGroupV1 {
	byGroup := make(map[string]*OwnerDirectoryGroupV1)
	order := make([]string, 0)
	for _, spec := range runtimeRootSpecsV1 {
		if spec.LayoutKind != LayoutOwnerLeafV1 {
			continue
		}
		components := strings.Split(spec.RelativeCASRoot, "/")
		if len(components) != 2 {
			continue
		}
		group := byGroup[spec.RecoveryGroupID]
		if group == nil {
			group = &OwnerDirectoryGroupV1{
				RecoveryGroupID:   spec.RecoveryGroupID,
				OwnerRelativePath: path.Join("private", components[0]),
				LeafComponents:    make([]string, 0),
			}
			byGroup[spec.RecoveryGroupID] = group
			order = append(order, spec.RecoveryGroupID)
		}
		group.LeafComponents = append(group.LeafComponents, components[1])
	}
	sort.Strings(order)
	result := make([]OwnerDirectoryGroupV1, 0, len(order))
	for _, groupID := range order {
		group := byGroup[groupID]
		sort.Strings(group.LeafComponents)
		result = append(result, OwnerDirectoryGroupV1{
			RecoveryGroupID:           group.RecoveryGroupID,
			OwnerRelativePath:         group.OwnerRelativePath,
			LeafComponents:            append([]string(nil), group.LeafComponents...),
			MigrationLeafComponents:   migrationLeafComponentsV1(group.RecoveryGroupID),
			FrozenDirectoryComponents: frozenDirectoryComponentsV1(group.RecoveryGroupID),
			RegularSiblingComponents:  regularSiblingComponentsV1(group.RecoveryGroupID),
			DeferOrphanRecovery:       group.RecoveryGroupID == EvidenceRegistryRecoveryGroupID,
		})
	}
	return result
}

func ValidateRuntimeDirectorySlotsV1() error {
	if err := ValidateRuntimeRootSpecsV1(); err != nil {
		return err
	}
	slots := FixedDirectorySlotsV1()
	fixedParents := FixedDirectoryParentPathsV1()
	shardParents := ShardParentPathsV1()
	scanParents := CreateRecoveryScanParentPathsV1()
	shards := CanonicalShardComponentsV1()
	ownerGroups := RecoverableOwnerDirectoryGroupsV1()
	if len(slots) != 76 || len(fixedParents) != 21 || len(shardParents) != 56 ||
		len(scanParents) != 77 || len(shards) != 256 || len(ownerGroups) != 15 ||
		MaximumCreateResidueCandidateLocationsV1() != 14_412 {
		return errors.New("runtime private CAS directory topology count changed")
	}
	seenPaths := make(map[string]DirectorySlotV1, len(slots))
	foldedPaths := make(map[string]string, len(slots))
	casRoots := 0
	topLevel := 0
	for _, slot := range slots {
		if slot.RelativePath == "" || slot.RelativePath != path.Clean(slot.RelativePath) ||
			path.IsAbs(slot.RelativePath) || strings.Contains(slot.RelativePath, `\`) ||
			slot.ParentRelativePath != path.Dir(slot.RelativePath) ||
			slot.Component == "" || slot.Component != path.Base(slot.RelativePath) ||
			strings.ContainsAny(slot.Component, `/\`) {
			return errors.New("runtime private CAS directory slot is invalid")
		}
		if _, duplicate := seenPaths[slot.RelativePath]; duplicate {
			return errors.New("runtime private CAS directory slot is duplicated")
		}
		seenPaths[slot.RelativePath] = slot
		folded := strings.ToLower(slot.RelativePath)
		if previous, alias := foldedPaths[folded]; alias && previous != slot.RelativePath {
			return errors.New("runtime private CAS directory slot aliases by case")
		}
		foldedPaths[folded] = slot.RelativePath
		if slot.CASRoot {
			casRoots++
		}
		if strings.Count(slot.RelativePath, "/") == 1 {
			topLevel++
		}
	}
	if casRoots != 56 || topLevel != 20 {
		return errors.New("runtime private CAS directory root classification changed")
	}
	seenOwners := make(map[string]struct{}, len(ownerGroups))
	migrationLeaves := 0
	frozenDirectories := 0
	regularSiblings := 0
	deferredOrphanOwners := 0
	for _, group := range ownerGroups {
		if group.RecoveryGroupID == "" || group.OwnerRelativePath == "" ||
			len(group.LeafComponents) == 0 {
			return errors.New("runtime private CAS recoverable owner group is invalid")
		}
		if _, duplicate := seenOwners[group.OwnerRelativePath]; duplicate {
			return errors.New("runtime private CAS recoverable owner path is duplicated")
		}
		seenOwners[group.OwnerRelativePath] = struct{}{}
		if group.DeferOrphanRecovery {
			deferredOrphanOwners++
			if group.RecoveryGroupID != EvidenceRegistryRecoveryGroupID {
				return errors.New("runtime private CAS deferred orphan owner is invalid")
			}
		}
		for _, leaf := range group.LeafComponents {
			relative := path.Join(group.OwnerRelativePath, leaf)
			slot, found := seenPaths[relative]
			if !found || !slot.CASRoot {
				return errors.New("runtime private CAS recoverable owner leaf is not a CAS root")
			}
		}
		for _, leaf := range group.MigrationLeafComponents {
			migrationLeaves++
			if group.RecoveryGroupID != "gate-continuations" ||
				(leaf != "receipts" && leaf != "dispositions") {
				return errors.New("runtime private CAS migration leaf is invalid")
			}
			for _, current := range group.LeafComponents {
				if strings.EqualFold(leaf, current) {
					return errors.New("runtime private CAS migration leaf aliases a current leaf")
				}
			}
		}
		for _, sibling := range group.RegularSiblingComponents {
			regularSiblings++
			if group.RecoveryGroupID != EvidenceRegistryRecoveryGroupID ||
				(sibling != EvidenceRegistryLegacyAuthorityIndexV1 && sibling != EvidenceRegistryLegacyLockV1) {
				return errors.New("runtime private CAS regular sibling is invalid")
			}
			for _, current := range group.LeafComponents {
				if strings.EqualFold(sibling, current) {
					return errors.New("runtime private CAS regular sibling aliases a current leaf")
				}
			}
		}
		for _, directory := range group.FrozenDirectoryComponents {
			frozenDirectories++
			if group.RecoveryGroupID != EvidenceRegistryRecoveryGroupID ||
				(directory != EvidenceRegistryLegacyCapsulesLeafV1 && directory != EvidenceRegistryLegacyProjectionsV1) {
				return errors.New("runtime private CAS frozen directory is invalid")
			}
			for _, current := range group.LeafComponents {
				if strings.EqualFold(directory, current) {
					return errors.New("runtime private CAS frozen directory aliases a current leaf")
				}
			}
		}
	}
	if migrationLeaves != 2 || frozenDirectories != 2 || regularSiblings != 2 || deferredOrphanOwners != 1 {
		return errors.New("runtime private CAS migration or frozen sibling count changed")
	}
	for _, spec := range runtimeRootSpecsV1 {
		relative := path.Join("private", spec.RelativeCASRoot)
		slot, found := seenPaths[relative]
		if !found || !slot.CASRoot {
			return errors.New("runtime private CAS root is absent from fixed directory slots")
		}
	}
	for leftIndex, left := range runtimeRootSpecsV1 {
		for rightIndex, right := range runtimeRootSpecsV1 {
			if leftIndex != rightIndex &&
				strings.HasPrefix(right.RelativeCASRoot, left.RelativeCASRoot+"/") {
				return errors.New("runtime private CAS roots overlap by path prefix")
			}
		}
	}
	residuesByParent := make(map[string]map[string]string)
	for _, slot := range slots {
		names := residuesByParent[slot.ParentRelativePath]
		if names == nil {
			names = make(map[string]string)
			residuesByParent[slot.ParentRelativePath] = names
		}
		residue := CreateDirectoryResidueNameV1(slot.Component)
		if previous, collision := names[residue]; collision && previous != slot.Component {
			return errors.New("runtime private CAS fixed directory residues collide")
		}
		names[residue] = slot.Component
	}
	shardResidues := make(map[string]string, len(shards))
	for _, shard := range shards {
		if !ValidShardV1(shard) {
			return errors.New("runtime private CAS shard component is invalid")
		}
		residue := CreateDirectoryResidueNameV1(shard)
		if previous, collision := shardResidues[residue]; collision && previous != shard {
			return errors.New("runtime private CAS shard residues collide")
		}
		shardResidues[residue] = shard
	}
	return nil
}

func migrationLeafComponentsV1(recoveryGroupID string) []string {
	if recoveryGroupID != "gate-continuations" {
		return nil
	}
	return []string{"dispositions", "receipts"}
}

func regularSiblingComponentsV1(recoveryGroupID string) []string {
	if recoveryGroupID != "evidence-registry" {
		return nil
	}
	return []string{EvidenceRegistryLegacyAuthorityIndexV1, EvidenceRegistryLegacyLockV1}
}

func frozenDirectoryComponentsV1(recoveryGroupID string) []string {
	if recoveryGroupID != "evidence-registry" {
		return nil
	}
	return []string{EvidenceRegistryLegacyCapsulesLeafV1, EvidenceRegistryLegacyProjectionsV1}
}

func sortedPrivateCASPathSetV1(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
