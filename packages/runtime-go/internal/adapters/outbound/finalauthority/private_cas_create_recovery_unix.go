//go:build darwin || linux

package finalauthority

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

type privateCASCreateResidueUnixDirectoryIdentity struct {
	device uint64
	inode  uint64
	mode   uint32
	uid    uint32
	gid    uint32
}

type privateCASCreateResidueUnixExactIdentity struct {
	directory privateCASCreateResidueUnixDirectoryIdentity
	links     uint64
	size      int64
	times     [32]byte
}

type privateCASCreateResidueUnixParent struct {
	relative string
	identity privateCASCreateResidueUnixExactIdentity
}

type privateCASCreateResidueUnixCandidate struct {
	parentRelative string
	name           string
	component      string
	kind           uint8
	identity       privateCASCreateResidueUnixExactIdentity
}

const (
	privateCASCreateResidueUnixCandidateTemporary uint8 = iota + 1
	privateCASCreateResidueUnixCandidateEmptyShard
	privateCASCreateResidueUnixCandidateEmptyLeaf
	privateCASCreateResidueUnixCandidateEmptyOwner
)

type privateCASCreateResiduePreparedPlan struct {
	dataDirPresent bool
	dataDir        privateCASCreateResidueUnixExactIdentity
	parents        []privateCASCreateResidueUnixParent
	residues       []privateCASCreateResidueUnixCandidate
}

func securePrivateCASObserveCreateResidueRecovery(
	ctx context.Context,
	binding privatecasport.RootBinding,
	mode privateCASDirectoryRecoveryModeV1,
	ownerScope string,
	originals ...*PreparedSecurePrivateCASOriginalCreateResiduesV1,
) (privateCASCreateResiduePreparedPlan, error) {
	if err := privateCASContextError(ctx); err != nil {
		return privateCASCreateResiduePreparedPlan{}, err
	}
	original, err := privateCASOriginalDirectoryRecoveryProofV1(mode, originals)
	if err != nil {
		return privateCASCreateResiduePreparedPlan{}, err
	}
	if original != nil && (original.prepared.binding != binding || ownerScope != "") {
		return privateCASCreateResiduePreparedPlan{}, errors.New("original orphan directory binding differs")
	}
	dataDir, present, err := privateCASUnixOpenCreateRecoveryDataDir(binding)
	if err != nil || !present {
		return privateCASCreateResiduePreparedPlan{dataDirPresent: present}, err
	}
	defer unix.Close(dataDir)
	dataIdentity, err := privateCASUnixCreateRecoveryDataDirExactIdentity(dataDir, binding.RootIdentity.Device)
	if err != nil {
		return privateCASCreateResiduePreparedPlan{}, err
	}
	plan := privateCASCreateResiduePreparedPlan{
		dataDirPresent: true,
		dataDir:        dataIdentity,
		parents:        make([]privateCASCreateResidueUnixParent, 0, 51),
		residues:       make([]privateCASCreateResidueUnixCandidate, 0),
	}
	for _, spec := range privateCASCreateResidueParentSpecsV1() {
		inOwner := ownerScope == "" || spec.relativePath == ownerScope || strings.HasPrefix(spec.relativePath, ownerScope+"/")
		if !inOwner && spec.relativePath != "." && !strings.HasPrefix(ownerScope, spec.relativePath+"/") {
			continue
		}
		if err := privateCASContextError(ctx); err != nil {
			return privateCASCreateResiduePreparedPlan{}, err
		}
		parent, parentPresent, err := privateCASUnixOpenCreateRecoveryParent(
			dataDir,
			spec.relativePath,
			binding.RootIdentity.Device,
		)
		if err != nil {
			return privateCASCreateResiduePreparedPlan{}, err
		}
		if !parentPresent {
			continue
		}
		parentIdentity, err := privateCASUnixCreateRecoveryExactIdentity(parent, binding.RootIdentity.Device)
		if spec.relativePath == "." {
			parentIdentity, err = privateCASUnixCreateRecoveryDataDirExactIdentity(
				parent,
				binding.RootIdentity.Device,
			)
		}
		if err != nil {
			_ = unix.Close(parent)
			return privateCASCreateResiduePreparedPlan{}, err
		}
		plan.parents = append(plan.parents, privateCASCreateResidueUnixParent{
			relative: spec.relativePath,
			identity: parentIdentity,
		})
		// Freeze ancestor identity without classifying unrelated recovery state.
		if !inOwner {
			if err := unix.Close(parent); err != nil {
				return privateCASCreateResiduePreparedPlan{}, err
			}
			continue
		}
		residues, scanErr := privateCASUnixObserveCreateRecoveryParent(
			ctx,
			parent,
			spec,
			binding.RootIdentity.Device,
			mode,
			privateCASOriginalCreateNamesInParentV1(original, spec.relativePath),
		)
		closeErr := unix.Close(parent)
		if scanErr != nil || closeErr != nil {
			return privateCASCreateResiduePreparedPlan{}, errors.Join(scanErr, closeErr)
		}
		plan.residues = append(plan.residues, residues...)
	}
	if mode == privateCASDirectoryRecoveryOrphanTopologyV1 {
		for _, owner := range domainprivatecas.RecoverableOwnerDirectoryGroupsV1() {
			if ownerScope != "" && owner.OwnerRelativePath != ownerScope {
				continue
			}
			residues, err := privateCASUnixObserveRecoverablePartialOwner(
				ctx,
				dataDir,
				owner,
				binding.RootIdentity.Device,
				original,
			)
			if err != nil {
				return privateCASCreateResiduePreparedPlan{}, err
			}
			plan.residues = append(plan.residues, residues...)
		}
	}
	sort.Slice(plan.residues, func(left int, right int) bool {
		if plan.residues[left].parentRelative != plan.residues[right].parentRelative {
			return plan.residues[left].parentRelative < plan.residues[right].parentRelative
		}
		return plan.residues[left].name < plan.residues[right].name
	})
	if original != nil && !privateCASOriginalCreateResiduePlansEqualV1(original.plan, original.projectV1(plan)) {
		return privateCASCreateResiduePreparedPlan{}, errors.New("original orphan creation residue projection changed")
	}
	return plan, nil
}

func privateCASUnixOpenCreateRecoveryDataDir(
	binding privatecasport.RootBinding,
) (int, bool, error) {
	if binding.RootIdentity.Kind != privatecasport.DirectoryIdentityUnix ||
		binding.RootIdentity.Device == 0 || binding.RootIdentity.Inode == 0 ||
		binding.RootIdentity.VolumeSerial != 0 || binding.RootIdentity.FileID != [16]byte{} {
		return -1, false, errors.New("private CAS Unix create-residue binding is invalid")
	}
	components, err := privateCASUnixRelativeComponents(binding.RelativePath)
	if err != nil || len(components) == 0 || components[len(components)-1] != "private" {
		return -1, false, errors.New("private CAS Unix create-residue binding does not target the private root")
	}
	current, err := securePrivateOpenAbsoluteDirectory(binding.RootPath, false)
	if err != nil {
		return -1, false, err
	}
	var rootStat unix.Stat_t
	if err := unix.Fstat(current, &rootStat); err != nil ||
		!privateCASUnixFrozenRootSafe(rootStat, uint32(os.Geteuid())) ||
		!existingPrivateAuthorityExtendedSecuritySafe(current) ||
		uint64(rootStat.Dev) != binding.RootIdentity.Device ||
		rootStat.Ino != binding.RootIdentity.Inode {
		_ = unix.Close(current)
		return -1, false, errors.New("private CAS Unix create-residue frozen root changed")
	}
	for _, component := range components[:len(components)-1] {
		next, present, err := privateCASUnixOpenExactBoundDirectory(
			current,
			component,
			binding.RootIdentity.Device,
		)
		_ = unix.Close(current)
		if err != nil || !present {
			return -1, false, err
		}
		current = next
	}
	return current, true, nil
}

func privateCASUnixOpenCreateRecoveryParent(
	dataDir int,
	relative string,
	device uint64,
) (int, bool, error) {
	current, err := unix.Dup(dataDir)
	if err != nil {
		return -1, false, err
	}
	if relative == "." {
		return current, true, nil
	}
	for _, component := range strings.Split(relative, "/") {
		next, present, err := privateCASUnixOpenExactBoundDirectory(current, component, device)
		_ = unix.Close(current)
		if err != nil || !present {
			return -1, false, err
		}
		current = next
	}
	return current, true, nil
}

func privateCASUnixOpenExactBoundDirectory(
	parent int,
	component string,
	device uint64,
) (int, bool, error) {
	actual := ""
	count := 0
	err := privateCASUnixWalkDir(parent, func(entry os.DirEntry) error {
		count++
		if count > maxPrivateCASCreateRecoveryDirectoryEntries {
			return errors.New("private CAS Unix create-residue directory entry bound exceeded")
		}
		if !strings.EqualFold(entry.Name(), component) {
			return nil
		}
		if actual != "" {
			return errors.New("private CAS Unix create-residue path aliases a component")
		}
		actual = entry.Name()
		return nil
	})
	if err != nil {
		return -1, false, err
	}
	if actual == "" {
		return -1, false, nil
	}
	if actual != component {
		return -1, false, errors.New("private CAS Unix create-residue path aliases a component by case")
	}
	next, err := unix.Openat(
		parent,
		component,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return -1, false, err
	}
	if _, err := privateCASUnixCreateRecoveryDirectoryIdentity(next, device); err != nil {
		_ = unix.Close(next)
		return -1, false, err
	}
	return next, true, nil
}

func privateCASUnixObserveCreateRecoveryParent(
	ctx context.Context,
	parent int,
	spec privateCASCreateResidueParentSpec,
	device uint64,
	mode privateCASDirectoryRecoveryModeV1,
	originalNames map[string]bool,
) ([]privateCASCreateResidueUnixCandidate, error) {
	fixedByFold := make(map[string]string, len(spec.fixedComponents))
	for component := range spec.fixedComponents {
		fixedByFold[strings.ToLower(component)] = component
	}
	residues := make([]privateCASCreateResidueUnixCandidate, 0)
	count := 0
	err := privateCASUnixWalkDir(parent, func(entry os.DirEntry) error {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		count++
		if count > maxPrivateCASCreateRecoveryDirectoryEntries {
			return errors.New("private CAS Unix create-residue directory entry bound exceeded")
		}
		name := entry.Name()
		if component, known := fixedByFold[strings.ToLower(name)]; known {
			if name != component {
				return errors.New("private CAS Unix fixed directory aliases its canonical component")
			}
			child, err := privateCASUnixOpenObservedCreateRecoveryDirectory(parent, name, device)
			if err != nil {
				return errors.Join(errors.New("private CAS Unix fixed directory is unsafe"), err)
			}
			return unix.Close(child)
		}
		if spec.shardParent {
			folded := strings.ToLower(name)
			if domainprivatecas.ValidShardV1(folded) {
				if name != folded {
					return errors.New("private CAS Unix shard aliases its canonical component")
				}
				child, err := privateCASUnixOpenObservedCreateRecoveryDirectory(parent, name, device)
				if err != nil {
					return errors.Join(errors.New("private CAS Unix shard directory is unsafe"), err)
				}
				identity, identityErr := privateCASUnixCreateRecoveryExactIdentity(child, device)
				empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(child)
				closeErr := unix.Close(child)
				if identityErr != nil || emptyErr != nil || closeErr != nil {
					return errors.Join(
						errors.New("private CAS Unix shard directory could not be classified"),
						identityErr,
						emptyErr,
						closeErr,
					)
				}
				if empty && mode == privateCASDirectoryRecoveryOrphanTopologyV1 && !spec.deferOrphanRecovery {
					residues = append(residues, privateCASCreateResidueUnixCandidate{
						parentRelative: spec.relativePath,
						name:           name,
						component:      name,
						kind:           privateCASCreateResidueUnixCandidateEmptyShard,
						identity:       identity,
					})
				}
				return nil
			}
		}
		if !domainprivatecas.LooksLikeCreateDirectoryResidueNameV1(name) {
			return nil
		}
		component, expected := spec.residueComponents[name]
		if !expected {
			return errors.New("private CAS Unix create-residue name is unknown or case-aliased")
		}
		if mode != privateCASDirectoryRecoveryCreateResiduesV1 && !originalNames[name] {
			return errors.New("private CAS Unix create residue appeared after pre-journal recovery")
		}
		residue, err := privateCASUnixOpenObservedCreateRecoveryDirectory(parent, name, device)
		if err != nil {
			return errors.Join(errors.New("private CAS Unix create residue is unsafe"), err)
		}
		identity, identityErr := privateCASUnixCreateRecoveryExactIdentity(residue, device)
		empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(residue)
		closeErr := unix.Close(residue)
		if identityErr != nil || emptyErr != nil || !empty || closeErr != nil {
			return errors.Join(
				errors.New("private CAS Unix create residue is non-empty or unsafe"),
				identityErr,
				emptyErr,
				closeErr,
			)
		}
		residues = append(residues, privateCASCreateResidueUnixCandidate{
			parentRelative: spec.relativePath,
			name:           name,
			component:      component,
			kind:           privateCASCreateResidueUnixCandidateTemporary,
			identity:       identity,
		})
		return nil
	})
	return residues, err
}

func privateCASUnixObserveRecoverablePartialOwner(
	ctx context.Context,
	dataDir int,
	group domainprivatecas.OwnerDirectoryGroupV1,
	device uint64,
	original *PreparedSecurePrivateCASOriginalCreateResiduesV1,
) ([]privateCASCreateResidueUnixCandidate, error) {
	owner, present, err := privateCASUnixOpenCreateRecoveryParent(
		dataDir,
		group.OwnerRelativePath,
		device,
	)
	if err != nil || !present {
		return nil, err
	}
	defer unix.Close(owner)
	ownerIdentity, err := privateCASUnixCreateRecoveryExactIdentity(owner, device)
	if err != nil {
		return nil, err
	}
	// The evidence-registry owner has an owner-specific handle-bound inventory
	// that freezes both current and legacy/unknown physical state. Generic
	// orphan rollback must not delete or interpret any part of that optional
	// capability before its semantic boundary is classified.
	if group.DeferOrphanRecovery {
		return nil, nil
	}
	originalNames := privateCASOriginalCreateNamesInParentV1(original, group.OwnerRelativePath)
	entries, err := privateCASUnixReadDirBounded(
		owner,
		len(group.LeafComponents)+len(group.MigrationLeafComponents)+
			len(group.FrozenDirectoryComponents)+len(group.RegularSiblingComponents)+len(originalNames),
	)
	if err != nil {
		return nil, errors.Join(errors.New("private CAS Unix partial owner inventory exceeds its exact leaves"), err)
	}
	expectedByFold := make(map[string]string, len(group.LeafComponents))
	for _, leaf := range group.LeafComponents {
		expectedByFold[strings.ToLower(leaf)] = leaf
	}
	migrationByFold := make(map[string]string, len(group.MigrationLeafComponents))
	for _, leaf := range group.MigrationLeafComponents {
		migrationByFold[strings.ToLower(leaf)] = leaf
	}
	regularByFold := make(map[string]string, len(group.RegularSiblingComponents))
	for _, sibling := range group.RegularSiblingComponents {
		regularByFold[strings.ToLower(sibling)] = sibling
	}
	frozenByFold := make(map[string]string, len(group.FrozenDirectoryComponents))
	for _, directory := range group.FrozenDirectoryComponents {
		frozenByFold[strings.ToLower(directory)] = directory
	}
	presentLeaves := make([]string, 0, len(entries))
	migrationPresent := false
	regularPresent := false
	frozenPresent := false
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return nil, err
		}
		if originalNames[entry.Name()] && entry.IsDir() {
			continue
		}
		folded := strings.ToLower(entry.Name())
		expected, known := expectedByFold[folded]
		if known && entry.Name() == expected {
			presentLeaves = append(presentLeaves, expected)
			continue
		}
		migration, knownMigration := migrationByFold[folded]
		if knownMigration && entry.Name() == migration {
			migrationPresent = true
			continue
		}
		regular, knownRegular := regularByFold[folded]
		if knownRegular && entry.Name() == regular {
			regularPresent = true
			continue
		}
		frozen, knownFrozen := frozenByFold[folded]
		if !knownFrozen || entry.Name() != frozen {
			return nil, errors.New("private CAS Unix partial owner contains an unknown or case-aliased leaf")
		}
		frozenPresent = true
	}
	// Reserved legacy components are immutable input to their owner-specific
	// semantic startup path. Orphan recovery must neither delete nor reinterpret
	// them; the owner validates the exact body/path inventory before activation.
	if migrationPresent || regularPresent || frozenPresent {
		return nil, nil
	}
	if len(presentLeaves) == len(group.LeafComponents) {
		return nil, nil
	}
	sort.Strings(presentLeaves)
	residues := make([]privateCASCreateResidueUnixCandidate, 0, len(presentLeaves)+1)
	for _, leafName := range presentLeaves {
		leaf, present, err := privateCASUnixOpenExactBoundDirectory(owner, leafName, device)
		if err != nil || !present {
			return nil, errors.Join(errors.New("private CAS Unix partial owner leaf changed"), err)
		}
		leafIdentity, identityErr := privateCASUnixCreateRecoveryExactIdentity(leaf, device)
		originalShards := privateCASOriginalCreateNamesInParentV1(original, path.Join(group.OwnerRelativePath, leafName))
		leafEntries, inventoryErr := privateCASUnixReadDirBounded(leaf, maxSecurePrivateCASShards+len(originalShards))
		if identityErr != nil || inventoryErr != nil {
			_ = unix.Close(leaf)
			return nil, errors.Join(
				errors.New("private CAS Unix partial owner leaf is unsafe"),
				identityErr,
				inventoryErr,
			)
		}
		for _, shardEntry := range leafEntries {
			name := shardEntry.Name()
			if originalShards[name] && shardEntry.IsDir() {
				continue
			}
			if !shardEntry.IsDir() || !domainprivatecas.ValidShardV1(name) {
				_ = unix.Close(leaf)
				return nil, errors.New("private CAS Unix partial owner leaf contains non-canonical state")
			}
			shard, present, err := privateCASUnixOpenExactBoundDirectory(leaf, name, device)
			if err != nil || !present {
				_ = unix.Close(leaf)
				return nil, errors.Join(errors.New("private CAS Unix partial owner shard changed"), err)
			}
			empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(shard)
			closeErr := unix.Close(shard)
			if emptyErr != nil || !empty || closeErr != nil {
				_ = unix.Close(leaf)
				return nil, errors.Join(
					errors.New("private CAS Unix populated partial owner cannot be rolled back"),
					emptyErr,
					closeErr,
				)
			}
		}
		if err := unix.Close(leaf); err != nil {
			return nil, err
		}
		residues = append(residues, privateCASCreateResidueUnixCandidate{
			parentRelative: group.OwnerRelativePath,
			name:           leafName,
			component:      leafName,
			kind:           privateCASCreateResidueUnixCandidateEmptyLeaf,
			identity:       leafIdentity,
		})
	}
	residues = append(residues, privateCASCreateResidueUnixCandidate{
		parentRelative: path.Dir(group.OwnerRelativePath),
		name:           path.Base(group.OwnerRelativePath),
		component:      path.Base(group.OwnerRelativePath),
		kind:           privateCASCreateResidueUnixCandidateEmptyOwner,
		identity:       ownerIdentity,
	})
	return residues, nil
}

func privateCASUnixOpenObservedCreateRecoveryDirectory(
	parent int,
	name string,
	device uint64,
) (int, error) {
	fd, err := unix.Openat(
		parent,
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return -1, err
	}
	if _, err := privateCASUnixCreateRecoveryDirectoryIdentity(fd, device); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func privateCASUnixCreateRecoveryDirectoryEmpty(fd int) (bool, error) {
	duplicate, err := unix.Openat(
		fd,
		".",
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return false, err
	}
	directory := os.NewFile(uintptr(duplicate), "private-cas-create-recovery-empty-directory")
	if directory == nil {
		_ = unix.Close(duplicate)
		return false, errors.New("private CAS Unix create-residue directory handle is invalid")
	}
	entries, readErr := directory.ReadDir(1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return false, closeErr
	}
	return len(entries) == 0 && errors.Is(readErr, io.EOF), nil
}

func privateCASUnixCreateRecoveryDirectoryIdentity(
	fd int,
	device uint64,
) (privateCASCreateResidueUnixDirectoryIdentity, error) {
	return privateCASUnixCreateRecoveryDirectoryIdentityWithPolicy(fd, device, false)
}

func privateCASUnixCreateRecoveryDataDirIdentity(
	fd int,
	device uint64,
) (privateCASCreateResidueUnixDirectoryIdentity, error) {
	return privateCASUnixCreateRecoveryDirectoryIdentityWithPolicy(fd, device, true)
}

func privateCASUnixCreateRecoveryDirectoryIdentityWithPolicy(
	fd int,
	device uint64,
	allowFrozenRootMode bool,
) (privateCASCreateResidueUnixDirectoryIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return privateCASCreateResidueUnixDirectoryIdentity{}, errors.Join(
			errors.New("private CAS Unix create-residue directory metadata is unavailable"),
			err,
		)
	}
	safeMode := existingPrivateAuthorityRootSafe(stat, uint32(os.Geteuid()))
	if allowFrozenRootMode {
		safeMode = privateCASUnixFrozenRootSafe(stat, uint32(os.Geteuid()))
	}
	if !safeMode || stat.Mode&0o7000 != 0 || stat.Nlink == 0 {
		return privateCASCreateResidueUnixDirectoryIdentity{}, fmt.Errorf(
			"private CAS Unix create-residue directory type, owner, mode, or links are unsafe: mode=%#o uid=%d links=%d",
			stat.Mode,
			stat.Uid,
			stat.Nlink,
		)
	}
	if !existingPrivateAuthorityExtendedSecuritySafe(fd) {
		return privateCASCreateResidueUnixDirectoryIdentity{}, errors.New("private CAS Unix create-residue directory ACL is unsafe")
	}
	if uint64(stat.Dev) != device {
		return privateCASCreateResidueUnixDirectoryIdentity{}, errors.New("private CAS Unix create-residue directory crossed its frozen device")
	}
	return privateCASCreateResidueUnixDirectoryIdentity{
		device: uint64(stat.Dev),
		inode:  stat.Ino,
		mode:   uint32(stat.Mode),
		uid:    stat.Uid,
		gid:    stat.Gid,
	}, nil
}

func privateCASUnixCreateRecoveryExactIdentity(
	fd int,
	device uint64,
) (privateCASCreateResidueUnixExactIdentity, error) {
	directory, err := privateCASUnixCreateRecoveryDirectoryIdentity(fd, device)
	if err != nil {
		return privateCASCreateResidueUnixExactIdentity{}, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return privateCASCreateResidueUnixExactIdentity{}, err
	}
	return privateCASCreateResidueUnixExactIdentity{
		directory: directory,
		links:     uint64(stat.Nlink),
		size:      stat.Size,
		times:     privateCASUnixStatTimes(stat),
	}, nil
}

func privateCASUnixCreateRecoveryDataDirExactIdentity(
	fd int,
	device uint64,
) (privateCASCreateResidueUnixExactIdentity, error) {
	directory, err := privateCASUnixCreateRecoveryDataDirIdentity(fd, device)
	if err != nil {
		return privateCASCreateResidueUnixExactIdentity{}, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return privateCASCreateResidueUnixExactIdentity{}, err
	}
	return privateCASCreateResidueUnixExactIdentity{
		directory: directory,
		links:     uint64(stat.Nlink),
		size:      stat.Size,
		times:     privateCASUnixStatTimes(stat),
	}, nil
}

func securePrivateCASApplyCreateResidueRecovery(
	ctx context.Context,
	binding privatecasport.RootBinding,
	mode privateCASDirectoryRecoveryModeV1,
	prepared privateCASCreateResiduePreparedPlan,
	preservedOwners []string,
	originals ...*PreparedSecurePrivateCASOriginalCreateResiduesV1,
) error {
	current, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, mode, "", originals...)
	if err != nil || !privateCASCreateResiduePreparedPlansEqual(current, prepared) {
		return errors.Join(errors.New("private CAS Unix create-residue preflight changed before deletion"), err)
	}
	if err := privateCASCreateResidueRecoveryTestCut("after_global_preflight", -1); err != nil {
		return err
	}
	dataDir, present, err := privateCASUnixOpenCreateRecoveryDataDir(binding)
	if err != nil || present != prepared.dataDirPresent {
		return errors.Join(errors.New("private CAS Unix create-residue data root changed before deletion"), err)
	}
	if !present {
		return nil
	}
	defer unix.Close(dataDir)
	dataIdentity, err := privateCASUnixCreateRecoveryDataDirExactIdentity(dataDir, binding.RootIdentity.Device)
	if err != nil || dataIdentity != prepared.dataDir {
		return errors.Join(errors.New("private CAS Unix create-residue data root identity changed"), err)
	}
	residues := make([]privateCASCreateResidueUnixCandidate, 0, len(prepared.residues))
	retained := make([]privateCASCreateResidueUnixCandidate, 0)
	for _, candidate := range prepared.residues {
		if privateCASDirectoryCandidatePreservedV1(candidate.parentRelative, candidate.name, preservedOwners) {
			retained = append(retained, candidate)
		} else {
			residues = append(residues, candidate)
		}
	}
	applied := prepared
	applied.residues = append([]privateCASCreateResidueUnixCandidate(nil), residues...)
	sort.Slice(residues, func(left int, right int) bool {
		leftDepth := privateCASCreateResiduePathDepth(residues[left].parentRelative)
		rightDepth := privateCASCreateResiduePathDepth(residues[right].parentRelative)
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		if residues[left].parentRelative != residues[right].parentRelative {
			return residues[left].parentRelative < residues[right].parentRelative
		}
		return residues[left].name < residues[right].name
	})
	parentIdentities := make(map[string]privateCASCreateResidueUnixDirectoryIdentity, len(prepared.parents))
	for _, parent := range prepared.parents {
		parentIdentities[parent.relative] = parent.identity.directory
	}
	for index, residue := range residues {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		parent, parentPresent, err := privateCASUnixOpenCreateRecoveryParent(
			dataDir,
			residue.parentRelative,
			binding.RootIdentity.Device,
		)
		if err != nil || !parentPresent {
			return errors.Join(errors.New("private CAS Unix create-residue parent disappeared"), err)
		}
		parentIdentity, identityErr := privateCASUnixCreateRecoveryDirectoryIdentity(parent, binding.RootIdentity.Device)
		if residue.parentRelative == "." {
			parentIdentity, identityErr = privateCASUnixCreateRecoveryDataDirIdentity(
				parent,
				binding.RootIdentity.Device,
			)
		}
		if identityErr != nil || parentIdentity != parentIdentities[residue.parentRelative] {
			_ = unix.Close(parent)
			return errors.Join(errors.New("private CAS Unix create-residue parent identity changed"), identityErr)
		}
		candidate, candidatePresent, openErr := privateCASUnixOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.Device,
		)
		if openErr != nil || !candidatePresent {
			_ = unix.Close(parent)
			return errors.Join(errors.New("private CAS Unix create residue disappeared"), openErr)
		}
		candidateIdentity, candidateIdentityErr := privateCASUnixCreateRecoveryExactIdentity(
			candidate,
			binding.RootIdentity.Device,
		)
		empty, emptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(candidate)
		if candidateIdentityErr != nil ||
			!privateCASUnixCreateRecoveryCandidateIdentityMatches(residue, candidateIdentity) ||
			emptyErr != nil || !empty {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return errors.Join(
				errors.New("private CAS Unix create residue changed before deletion"),
				candidateIdentityErr,
				emptyErr,
			)
		}
		if err := privateCASCreateResidueRecoveryTestCut("before_delete", index); err != nil {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return err
		}
		verification, verificationPresent, verifyErr := privateCASUnixOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.Device,
		)
		if verifyErr != nil || !verificationPresent {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return errors.Join(errors.New("private CAS Unix create residue changed at deletion"), verifyErr)
		}
		verificationIdentity, verificationIdentityErr := privateCASUnixCreateRecoveryExactIdentity(
			verification,
			binding.RootIdentity.Device,
		)
		verificationEmpty, verificationEmptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(verification)
		verificationCloseErr := unix.Close(verification)
		if verificationIdentityErr != nil ||
			!privateCASUnixCreateRecoveryCandidateIdentityMatches(residue, verificationIdentity) ||
			verificationEmptyErr != nil || !verificationEmpty || verificationCloseErr != nil {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return errors.Join(
				errors.New("private CAS Unix create residue failed final identity validation"),
				verificationIdentityErr,
				verificationEmptyErr,
				verificationCloseErr,
			)
		}
		if err := privateCASCreateResidueRecoveryTestCut("after_final_validation_before_delete", index); err != nil {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return err
		}
		finalName, finalNamePresent, finalNameErr := privateCASUnixOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.Device,
		)
		if finalNameErr != nil || !finalNamePresent {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return errors.Join(errors.New("private CAS Unix create residue changed in the final deletion window"), finalNameErr)
		}
		finalNameIdentity, finalNameIdentityErr := privateCASUnixCreateRecoveryExactIdentity(
			finalName,
			binding.RootIdentity.Device,
		)
		finalNameEmpty, finalNameEmptyErr := privateCASUnixCreateRecoveryDirectoryEmpty(finalName)
		finalNameCloseErr := unix.Close(finalName)
		if finalNameIdentityErr != nil ||
			!privateCASUnixCreateRecoveryCandidateIdentityMatches(residue, finalNameIdentity) ||
			finalNameEmptyErr != nil || !finalNameEmpty || finalNameCloseErr != nil {
			_ = unix.Close(candidate)
			_ = unix.Close(parent)
			return errors.Join(
				errors.New("private CAS Unix create residue failed deletion-window revalidation"),
				finalNameIdentityErr,
				finalNameEmptyErr,
				finalNameCloseErr,
			)
		}
		deleteErr := unix.Unlinkat(parent, residue.name, unix.AT_REMOVEDIR)
		var heldStat unix.Stat_t
		heldStatErr := unix.Fstat(candidate, &heldStat)
		stillNamed, stillPresent, absenceErr := privateCASUnixOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.Device,
		)
		if stillPresent {
			_ = unix.Close(stillNamed)
		}
		if absenceErr == nil && stillPresent {
			absenceErr = errors.New("private CAS Unix deleted create-residue name is still present")
		}
		candidateCloseErr := unix.Close(candidate)
		syncErr := unix.Fsync(parent)
		parentCloseErr := unix.Close(parent)
		if deleteErr != nil || heldStatErr != nil ||
			uint64(heldStat.Dev) != residue.identity.directory.device ||
			heldStat.Ino != residue.identity.directory.inode ||
			absenceErr != nil || candidateCloseErr != nil ||
			syncErr != nil || parentCloseErr != nil {
			return errors.Join(
				deleteErr,
				heldStatErr,
				absenceErr,
				candidateCloseErr,
				syncErr,
				parentCloseErr,
				errors.New("private CAS Unix create residue deletion did not preserve its pinned identity"),
			)
		}
		if err := privateCASCreateResidueRecoveryTestCut("after_delete", index); err != nil {
			return err
		}
	}
	first, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, mode, "", originals...)
	if err != nil {
		return err
	}
	second, err := securePrivateCASObserveCreateResidueRecovery(ctx, binding, mode, "", originals...)
	if err != nil || !privateCASCreateResiduePreparedPlansEqual(first, second) ||
		!privateCASUnixRetainedDirectoryCandidatesEqualV1(retained, second.residues) ||
		!privateCASUnixCreateRecoveryPostTopologyEqual(applied, second) {
		return errors.Join(errors.New("private CAS Unix directory cleanup did not produce the exact retained topology"), err)
	}
	return nil
}

func privateCASUnixCreateRecoveryParentTopologyEqual(
	left privateCASCreateResiduePreparedPlan,
	right privateCASCreateResiduePreparedPlan,
) bool {
	return left.dataDirPresent == right.dataDirPresent &&
		left.dataDir == right.dataDir &&
		slices.Equal(left.parents, right.parents)
}

func privateCASUnixCreateRecoveryPostTopologyEqual(
	left privateCASCreateResiduePreparedPlan,
	right privateCASCreateResiduePreparedPlan,
) bool {
	if left.dataDirPresent != right.dataDirPresent ||
		left.dataDir.directory != right.dataDir.directory {
		return false
	}
	removed := make(map[string]struct{})
	for _, candidate := range left.residues {
		if candidate.kind != privateCASCreateResidueUnixCandidateEmptyLeaf &&
			candidate.kind != privateCASCreateResidueUnixCandidateEmptyOwner {
			continue
		}
		removed[path.Join(candidate.parentRelative, candidate.name)] = struct{}{}
	}
	expected := make([]privateCASCreateResidueUnixParent, 0, len(left.parents))
	for _, parent := range left.parents {
		if _, wasRemoved := removed[parent.relative]; !wasRemoved {
			expected = append(expected, parent)
		}
	}
	if len(expected) != len(right.parents) {
		return false
	}
	for index := range expected {
		if expected[index].relative != right.parents[index].relative ||
			expected[index].identity.directory != right.parents[index].identity.directory {
			return false
		}
	}
	return true
}

func privateCASUnixCreateRecoveryCandidateIdentityMatches(
	prepared privateCASCreateResidueUnixCandidate,
	current privateCASCreateResidueUnixExactIdentity,
) bool {
	switch prepared.kind {
	case privateCASCreateResidueUnixCandidateEmptyLeaf,
		privateCASCreateResidueUnixCandidateEmptyOwner:
		return prepared.identity.directory == current.directory
	default:
		return prepared.identity == current
	}
}

func privateCASCreateResiduePreparedPlansEqual(
	left privateCASCreateResiduePreparedPlan,
	right privateCASCreateResiduePreparedPlan,
) bool {
	return privateCASUnixCreateRecoveryParentTopologyEqual(left, right) &&
		slices.Equal(left.residues, right.residues)
}

func privateCASCreateResiduePreparedPlanCount(plan privateCASCreateResiduePreparedPlan) int {
	return len(plan.residues)
}

// An empty shard or a complete empty owner cannot prove a proper partial owner.
func privateCASPreparedPlanProvesEmptyPartialOwnerV1(plan privateCASCreateResiduePreparedPlan, owner string) bool {
	count := 0
	for _, candidate := range plan.residues {
		relative := path.Join(candidate.parentRelative, candidate.name)
		switch candidate.kind {
		case privateCASCreateResidueUnixCandidateEmptyOwner:
			if relative != owner {
				return false
			}
			count++
		case privateCASCreateResidueUnixCandidateEmptyLeaf, privateCASCreateResidueUnixCandidateEmptyShard:
			if !strings.HasPrefix(relative, owner+"/") {
				return false
			}
		default:
			return false
		}
	}
	return count == 1
}

// Canonical parents may gain independently authorized contents. Original
// ancestor identities and the exact empty residue objects remain immutable.
func privateCASOriginalCreateResidueProjectionV1(plan privateCASCreateResiduePreparedPlan, owner string) privateCASCreateResiduePreparedPlan {
	return privateCASOriginalCreateResidueProjectionForOwnersV1(plan, []string{owner})
}

func privateCASOriginalCreateResidueProjectionForOwnersV1(plan privateCASCreateResiduePreparedPlan, owners []string) privateCASCreateResiduePreparedPlan {
	projected := privateCASCreateResiduePreparedPlan{dataDirPresent: plan.dataDirPresent, dataDir: plan.dataDir}
	for _, parent := range plan.parents {
		selected := false
		for _, owner := range owners {
			selected = selected || privateCASCreationParentInOwnerV1(parent.relative, owner)
		}
		if selected {
			projected.parents = append(projected.parents, parent)
		}
	}
	for _, residue := range plan.residues {
		selected := false
		for _, owner := range owners {
			selected = selected || privateCASCreationTargetInOwnerV1(residue.parentRelative, residue.component, owner)
		}
		if residue.kind == privateCASCreateResidueUnixCandidateTemporary && selected {
			projected.residues = append(projected.residues, residue)
		}
	}
	return projected
}

func privateCASOriginalCreateResiduePathsV1(plan privateCASCreateResiduePreparedPlan) []string {
	paths := make([]string, 0, len(plan.residues))
	for _, residue := range plan.residues {
		paths = append(paths, path.Join(residue.parentRelative, residue.name))
	}
	return paths
}

func privateCASOriginalCreateResiduePlansEqualV1(original, current privateCASCreateResiduePreparedPlan) bool {
	if original.dataDirPresent != current.dataDirPresent || original.dataDir.directory != current.dataDir.directory || !slices.Equal(original.residues, current.residues) {
		return false
	}
	for _, parent := range original.parents {
		found := false
		for _, now := range current.parents {
			if parent.relative == now.relative {
				if parent.identity.directory != now.identity.directory {
					return false
				}
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func privateCASOriginalCreateLeafMatchesV1(original privateCASCreateResiduePreparedPlan, leaf string, observed privateCASPreparedRecoveryPlan) bool {
	expected := make([]privateCASCreateResidueUnixCandidate, 0)
	for _, residue := range original.residues {
		if residue.parentRelative == leaf {
			expected = append(expected, privateCASCreateResidueUnixCandidate{name: residue.name, identity: residue.identity})
		}
	}
	return slices.Equal(expected, observed.originalCreateResidues)
}

func privateCASOriginalCreateTargetShardV1(plan privateCASCreateResiduePreparedPlan, leaf, shard string) bool {
	for _, residue := range plan.residues {
		if residue.parentRelative == leaf && residue.component == shard && residue.name == domainprivatecas.CreateDirectoryResidueNameV1(shard) {
			return true
		}
	}
	return false
}

// Retained canonical ancestors may change child counts and timestamps when
// independent descendants are removed. Residues and retained shards stay exact.
func privateCASUnixRetainedDirectoryCandidatesEqualV1(expected, current []privateCASCreateResidueUnixCandidate) bool {
	if len(expected) != len(current) {
		return false
	}
	for index, candidate := range expected {
		now := current[index]
		if candidate.parentRelative != now.parentRelative || candidate.name != now.name || candidate.component != now.component || candidate.kind != now.kind || !privateCASUnixCreateRecoveryCandidateIdentityMatches(candidate, now.identity) {
			return false
		}
	}
	return true
}

func privateCASOriginalCreateFingerprintMaterialV1(plan privateCASCreateResiduePreparedPlan) string {
	var material strings.Builder
	fmt.Fprintf(&material, "%t\n%#v\n", plan.dataDirPresent, plan.dataDir.directory)
	for _, parent := range plan.parents {
		fmt.Fprintf(&material, "%q\n%#v\n", parent.relative, parent.identity.directory)
	}
	fmt.Fprintf(&material, "%#v\n", plan.residues)
	return material.String()
}

func privateCASOriginalCreateDirectoryStatesV1(plan privateCASCreateResiduePreparedPlan) []privatecasport.OriginalCreateDirectoryV1 {
	states := make([]privatecasport.OriginalCreateDirectoryV1, 0, len(plan.residues))
	for _, residue := range plan.residues {
		mode := uint32(os.ModeDir) | (residue.identity.directory.mode & 0o777)
		states = append(states, privatecasport.OriginalCreateDirectoryV1{RelativePath: path.Join(residue.parentRelative, residue.name), Mode: mode})
	}
	return states
}
