//go:build windows

package finalauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"unsafe"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/windows"
)

type privateCASCreateResidueWindowsStableIdentity struct {
	id             privateWindowsFileIDInfo
	attributes     uint32
	securitySHA256 [32]byte
}

type privateCASCreateResidueWindowsExactIdentity struct {
	stable privateCASCreateResidueWindowsStableIdentity
	object privateWindowsObjectIdentity
}

type privateCASCreateResidueWindowsParent struct {
	relative string
	identity privateCASCreateResidueWindowsExactIdentity
}

type privateCASCreateResidueWindowsCandidate struct {
	parentRelative string
	name           string
	component      string
	kind           uint8
	identity       privateCASCreateResidueWindowsExactIdentity
}

const (
	privateCASCreateResidueWindowsCandidateTemporary uint8 = iota + 1
	privateCASCreateResidueWindowsCandidateEmptyShard
	privateCASCreateResidueWindowsCandidateEmptyLeaf
	privateCASCreateResidueWindowsCandidateEmptyOwner
)

type privateCASCreateResiduePreparedPlan struct {
	dataDirPresent bool
	dataDir        privateCASCreateResidueWindowsExactIdentity
	parents        []privateCASCreateResidueWindowsParent
	residues       []privateCASCreateResidueWindowsCandidate
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
	dataDir, present, err := privateCASWindowsOpenCreateRecoveryDataDir(binding)
	if err != nil || !present {
		return privateCASCreateResiduePreparedPlan{dataDirPresent: present}, err
	}
	defer windows.CloseHandle(dataDir)
	dataIdentity, err := privateCASWindowsCreateRecoveryExactIdentity(
		dataDir,
		binding.RootIdentity.VolumeSerial,
	)
	if err != nil {
		return privateCASCreateResiduePreparedPlan{}, err
	}
	plan := privateCASCreateResiduePreparedPlan{
		dataDirPresent: true,
		dataDir:        dataIdentity,
		parents:        make([]privateCASCreateResidueWindowsParent, 0, 51),
		residues:       make([]privateCASCreateResidueWindowsCandidate, 0),
	}
	for _, spec := range privateCASCreateResidueParentSpecsV1() {
		inOwner := ownerScope == "" || spec.relativePath == ownerScope || strings.HasPrefix(spec.relativePath, ownerScope+"/")
		if !inOwner && spec.relativePath != "." && !strings.HasPrefix(ownerScope, spec.relativePath+"/") {
			continue
		}
		if err := privateCASContextError(ctx); err != nil {
			return privateCASCreateResiduePreparedPlan{}, err
		}
		parent, parentPresent, err := privateCASWindowsOpenCreateRecoveryParent(
			dataDir,
			spec.relativePath,
			binding.RootIdentity.VolumeSerial,
		)
		if err != nil {
			return privateCASCreateResiduePreparedPlan{}, err
		}
		if !parentPresent {
			continue
		}
		parentIdentity, err := privateCASWindowsCreateRecoveryExactIdentity(
			parent,
			binding.RootIdentity.VolumeSerial,
		)
		if err != nil {
			_ = windows.CloseHandle(parent)
			return privateCASCreateResiduePreparedPlan{}, err
		}
		plan.parents = append(plan.parents, privateCASCreateResidueWindowsParent{
			relative: spec.relativePath,
			identity: parentIdentity,
		})
		// Freeze ancestor identity without classifying unrelated recovery state.
		if !inOwner {
			if err := windows.CloseHandle(parent); err != nil {
				return privateCASCreateResiduePreparedPlan{}, err
			}
			continue
		}
		residues, scanErr := privateCASWindowsObserveCreateRecoveryParent(
			ctx,
			parent,
			spec,
			binding.RootIdentity.VolumeSerial,
			mode,
			privateCASOriginalCreateNamesInParentV1(original, spec.relativePath),
		)
		closeErr := windows.CloseHandle(parent)
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
			residues, err := privateCASWindowsObserveRecoverablePartialOwner(
				ctx,
				dataDir,
				owner,
				binding.RootIdentity.VolumeSerial,
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

func privateCASWindowsOpenCreateRecoveryDataDir(
	binding privatecasport.RootBinding,
) (windows.Handle, bool, error) {
	if binding.RootIdentity.Kind != privatecasport.DirectoryIdentityWindows ||
		binding.RootIdentity.Device != 0 || binding.RootIdentity.Inode != 0 ||
		binding.RootIdentity.VolumeSerial == 0 || binding.RootIdentity.FileID == [16]byte{} {
		return 0, false, errors.New("private CAS Windows create-residue binding is invalid")
	}
	components, err := privateCASWindowsRelativeComponents(binding.RelativePath)
	if err != nil || len(components) == 0 || components[len(components)-1] != "private" {
		return 0, false, errors.New("private CAS Windows create-residue binding does not target the private root")
	}
	current, err := privateWindowsOpenAbsoluteDirectory(binding.RootPath, false)
	if err != nil {
		return 0, false, err
	}
	rootIdentity, err := privateWindowsValidateAuthorityObject(current, true, 0)
	if err != nil || rootIdentity.id.VolumeSerialNumber != binding.RootIdentity.VolumeSerial ||
		rootIdentity.id.FileID != binding.RootIdentity.FileID {
		_ = windows.CloseHandle(current)
		return 0, false, errors.New("private CAS Windows create-residue frozen root changed")
	}
	for _, component := range components[:len(components)-1] {
		next, present, err := privateCASWindowsOpenExactBoundDirectory(
			current,
			component,
			binding.RootIdentity.VolumeSerial,
		)
		_ = windows.CloseHandle(current)
		if err != nil || !present {
			return 0, false, err
		}
		current = next
	}
	return current, true, nil
}

func privateCASWindowsOpenCreateRecoveryParent(
	dataDir windows.Handle,
	relative string,
	volume uint64,
) (windows.Handle, bool, error) {
	current, err := privateCASWindowsDuplicateDirectory(dataDir)
	if err != nil {
		return 0, false, err
	}
	if relative == "." {
		return current, true, nil
	}
	for _, component := range strings.Split(relative, "/") {
		next, present, err := privateCASWindowsOpenExactBoundDirectory(current, component, volume)
		_ = windows.CloseHandle(current)
		if err != nil || !present {
			return 0, false, err
		}
		current = next
	}
	return current, true, nil
}

func privateCASWindowsDuplicateDirectory(handle windows.Handle) (windows.Handle, error) {
	return privateWindowsReopenFile(
		handle,
		privateWindowsObserveDirectoryAccess,
		privateWindowsShare,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
	)
}

func privateCASWindowsOpenExactBoundDirectory(
	parent windows.Handle,
	component string,
	volume uint64,
) (windows.Handle, bool, error) {
	return privateCASWindowsOpenExactBoundDirectoryWithAccess(
		parent,
		component,
		volume,
		privateWindowsMutateDirectoryAccess,
	)
}

func privateCASWindowsOpenExactBoundDirectoryWithAccess(
	parent windows.Handle,
	component string,
	volume uint64,
	access uint32,
) (windows.Handle, bool, error) {
	if access != privateWindowsMutateDirectoryAccess &&
		access != privateWindowsObserveDirectoryAccess {
		return 0, false, errors.New("private CAS Windows exact-directory access is invalid")
	}
	entries, err := privateCASWindowsReadDirBounded(
		parent,
		maxPrivateCASCreateRecoveryDirectoryEntries,
	)
	if err != nil {
		return 0, false, err
	}
	actual := ""
	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), component) {
			continue
		}
		if actual != "" {
			return 0, false, errors.New("private CAS Windows create-residue path aliases a component")
		}
		actual = entry.Name()
	}
	if actual == "" {
		return 0, false, nil
	}
	if actual != component {
		return 0, false, errors.New("private CAS Windows create-residue path aliases a component by case")
	}
	next, err := privateWindowsOpenRelative(
		parent,
		component,
		access,
		windows.FILE_OPEN,
		true,
	)
	if err != nil {
		return 0, false, err
	}
	if _, err := privateCASWindowsCreateRecoveryExactIdentity(next, volume); err != nil {
		_ = windows.CloseHandle(next)
		return 0, false, err
	}
	if err := privateWindowsVerifyRelativeDirectoryIdentity(parent, component, next); err != nil {
		_ = windows.CloseHandle(next)
		return 0, false, err
	}
	return next, true, nil
}

func privateCASWindowsOpenDeletionGuard(
	parent windows.Handle,
	residue privateCASCreateResidueWindowsCandidate,
	volume uint64,
) (windows.Handle, error) {
	anchor, present, err := privateCASWindowsOpenExactBoundDirectoryWithAccess(
		parent,
		residue.name,
		volume,
		privateWindowsObserveDirectoryAccess,
	)
	if err != nil || !present {
		return 0, errors.Join(
			errors.New("private CAS Windows create residue changed in the final deletion window"),
			err,
		)
	}
	anchorIdentity, identityErr := privateCASWindowsCreateRecoveryExactIdentity(anchor, volume)
	anchorEmpty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(anchor)
	if identityErr != nil ||
		!privateCASWindowsCreateRecoveryCandidateIdentityMatches(residue, anchorIdentity) ||
		emptyErr != nil || !anchorEmpty {
		_ = windows.CloseHandle(anchor)
		return 0, errors.Join(
			errors.New("private CAS Windows create residue failed deletion-window revalidation"),
			identityErr,
			emptyErr,
		)
	}
	guard, guardErr := privateWindowsReopenFile(
		anchor,
		privateWindowsMutateDirectoryAccess,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
	)
	if guardErr != nil {
		_ = windows.CloseHandle(anchor)
		return 0, errors.Join(
			errors.New("private CAS Windows create residue could not acquire a delete-share guard"),
			guardErr,
		)
	}
	guardIdentity, guardIdentityErr := privateCASWindowsCreateRecoveryExactIdentity(guard, volume)
	guardEmpty, guardEmptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(guard)
	nameErr := privateWindowsVerifyRelativeDirectoryIdentity(parent, residue.name, guard)
	anchorCloseErr := windows.CloseHandle(anchor)
	if guardIdentityErr != nil ||
		!privateCASWindowsCreateRecoveryCandidateIdentityMatches(residue, guardIdentity) ||
		guardEmptyErr != nil || !guardEmpty ||
		nameErr != nil || anchorCloseErr != nil {
		_ = windows.CloseHandle(guard)
		return 0, errors.Join(
			errors.New("private CAS Windows create residue delete-share guard is invalid"),
			guardIdentityErr,
			guardEmptyErr,
			nameErr,
			anchorCloseErr,
		)
	}
	return guard, nil
}

func privateCASWindowsObserveCreateRecoveryParent(
	ctx context.Context,
	parent windows.Handle,
	spec privateCASCreateResidueParentSpec,
	volume uint64,
	mode privateCASDirectoryRecoveryModeV1,
	originalNames map[string]bool,
) ([]privateCASCreateResidueWindowsCandidate, error) {
	entries, err := privateCASWindowsReadDirBounded(
		parent,
		maxPrivateCASCreateRecoveryDirectoryEntries,
	)
	if err != nil {
		return nil, err
	}
	fixedByFold := make(map[string]string, len(spec.fixedComponents))
	for component := range spec.fixedComponents {
		fixedByFold[strings.ToLower(component)] = component
	}
	residues := make([]privateCASCreateResidueWindowsCandidate, 0)
	for _, entry := range entries {
		if err := privateCASContextError(ctx); err != nil {
			return nil, err
		}
		name := entry.Name()
		if component, known := fixedByFold[strings.ToLower(name)]; known {
			if name != component {
				return nil, errors.New("private CAS Windows fixed directory aliases its canonical component")
			}
			child, err := privateCASWindowsOpenObservedCreateRecoveryDirectory(parent, name, volume)
			if err != nil {
				return nil, errors.Join(errors.New("private CAS Windows fixed directory is unsafe"), err)
			}
			if err := windows.CloseHandle(child); err != nil {
				return nil, err
			}
			continue
		}
		if spec.shardParent {
			folded := strings.ToLower(name)
			if domainprivatecas.ValidShardV1(folded) {
				if name != folded {
					return nil, errors.New("private CAS Windows shard aliases its canonical component")
				}
				child, err := privateCASWindowsOpenObservedCreateRecoveryDirectory(parent, name, volume)
				if err != nil {
					return nil, errors.Join(errors.New("private CAS Windows shard directory is unsafe"), err)
				}
				identity, identityErr := privateCASWindowsCreateRecoveryExactIdentity(child, volume)
				empty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(child)
				closeErr := windows.CloseHandle(child)
				if identityErr != nil || emptyErr != nil || closeErr != nil {
					return nil, errors.Join(
						errors.New("private CAS Windows shard directory could not be classified"),
						identityErr,
						emptyErr,
						closeErr,
					)
				}
				if empty && mode == privateCASDirectoryRecoveryOrphanTopologyV1 && !spec.deferOrphanRecovery {
					residues = append(residues, privateCASCreateResidueWindowsCandidate{
						parentRelative: spec.relativePath,
						name:           name,
						component:      name,
						kind:           privateCASCreateResidueWindowsCandidateEmptyShard,
						identity:       identity,
					})
				}
				continue
			}
		}
		if !domainprivatecas.LooksLikeCreateDirectoryResidueNameV1(name) {
			continue
		}
		component, expected := spec.residueComponents[name]
		if !expected {
			return nil, errors.New("private CAS Windows create-residue name is unknown or case-aliased")
		}
		if mode != privateCASDirectoryRecoveryCreateResiduesV1 && !originalNames[name] {
			return nil, errors.New("private CAS Windows create residue appeared after pre-journal recovery")
		}
		residue, err := privateCASWindowsOpenObservedCreateRecoveryDirectory(parent, name, volume)
		if err != nil {
			return nil, errors.Join(errors.New("private CAS Windows create residue is unsafe"), err)
		}
		identity, identityErr := privateCASWindowsCreateRecoveryExactIdentity(residue, volume)
		empty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(residue)
		closeErr := windows.CloseHandle(residue)
		if identityErr != nil || emptyErr != nil || !empty || closeErr != nil {
			return nil, errors.Join(
				errors.New("private CAS Windows create residue is non-empty or unsafe"),
				identityErr,
				emptyErr,
				closeErr,
			)
		}
		residues = append(residues, privateCASCreateResidueWindowsCandidate{
			parentRelative: spec.relativePath,
			name:           name,
			component:      component,
			kind:           privateCASCreateResidueWindowsCandidateTemporary,
			identity:       identity,
		})
	}
	return residues, nil
}

func privateCASWindowsObserveRecoverablePartialOwner(
	ctx context.Context,
	dataDir windows.Handle,
	group domainprivatecas.OwnerDirectoryGroupV1,
	volume uint64,
	original *PreparedSecurePrivateCASOriginalCreateResiduesV1,
) ([]privateCASCreateResidueWindowsCandidate, error) {
	owner, present, err := privateCASWindowsOpenCreateRecoveryParent(
		dataDir,
		group.OwnerRelativePath,
		volume,
	)
	if err != nil || !present {
		return nil, err
	}
	defer windows.CloseHandle(owner)
	ownerIdentity, err := privateCASWindowsCreateRecoveryExactIdentity(owner, volume)
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
	entries, err := privateCASWindowsReadDirBounded(
		owner,
		len(group.LeafComponents)+len(group.MigrationLeafComponents)+
			len(group.FrozenDirectoryComponents)+len(group.RegularSiblingComponents)+len(originalNames),
	)
	if err != nil {
		return nil, errors.Join(errors.New("private CAS Windows partial owner inventory exceeds its exact leaves"), err)
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
			return nil, errors.New("private CAS Windows partial owner contains an unknown or case-aliased leaf")
		}
		frozenPresent = true
	}
	if migrationPresent || regularPresent || frozenPresent {
		return nil, nil
	}
	if len(presentLeaves) == len(group.LeafComponents) {
		return nil, nil
	}
	sort.Strings(presentLeaves)
	residues := make([]privateCASCreateResidueWindowsCandidate, 0, len(presentLeaves)+1)
	for _, leafName := range presentLeaves {
		leaf, present, err := privateCASWindowsOpenExactBoundDirectory(owner, leafName, volume)
		if err != nil || !present {
			return nil, errors.Join(errors.New("private CAS Windows partial owner leaf changed"), err)
		}
		leafIdentity, identityErr := privateCASWindowsCreateRecoveryExactIdentity(leaf, volume)
		originalShards := privateCASOriginalCreateNamesInParentV1(original, path.Join(group.OwnerRelativePath, leafName))
		leafEntries, inventoryErr := privateCASWindowsReadDirBounded(leaf, maxSecurePrivateCASShards+len(originalShards))
		if identityErr != nil || inventoryErr != nil {
			_ = windows.CloseHandle(leaf)
			return nil, errors.Join(
				errors.New("private CAS Windows partial owner leaf is unsafe"),
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
				_ = windows.CloseHandle(leaf)
				return nil, errors.New("private CAS Windows partial owner leaf contains non-canonical state")
			}
			shard, present, err := privateCASWindowsOpenExactBoundDirectory(leaf, name, volume)
			if err != nil || !present {
				_ = windows.CloseHandle(leaf)
				return nil, errors.Join(errors.New("private CAS Windows partial owner shard changed"), err)
			}
			empty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(shard)
			closeErr := windows.CloseHandle(shard)
			if emptyErr != nil || !empty || closeErr != nil {
				_ = windows.CloseHandle(leaf)
				return nil, errors.Join(
					errors.New("private CAS Windows populated partial owner cannot be rolled back"),
					emptyErr,
					closeErr,
				)
			}
		}
		if err := windows.CloseHandle(leaf); err != nil {
			return nil, err
		}
		residues = append(residues, privateCASCreateResidueWindowsCandidate{
			parentRelative: group.OwnerRelativePath,
			name:           leafName,
			component:      leafName,
			kind:           privateCASCreateResidueWindowsCandidateEmptyLeaf,
			identity:       leafIdentity,
		})
	}
	residues = append(residues, privateCASCreateResidueWindowsCandidate{
		parentRelative: path.Dir(group.OwnerRelativePath),
		name:           path.Base(group.OwnerRelativePath),
		component:      path.Base(group.OwnerRelativePath),
		kind:           privateCASCreateResidueWindowsCandidateEmptyOwner,
		identity:       ownerIdentity,
	})
	return residues, nil
}

func privateCASWindowsOpenObservedCreateRecoveryDirectory(
	parent windows.Handle,
	name string,
	volume uint64,
) (windows.Handle, error) {
	handle, err := privateWindowsOpenRelative(
		parent,
		name,
		privateWindowsMutateDirectoryAccess,
		windows.FILE_OPEN,
		true,
	)
	if err != nil {
		return 0, err
	}
	if _, err := privateCASWindowsCreateRecoveryExactIdentity(handle, volume); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	return handle, nil
}

func privateCASWindowsCreateRecoveryDirectoryEmpty(handle windows.Handle) (bool, error) {
	duplicate, err := privateCASWindowsDuplicateDirectory(handle)
	if err != nil {
		return false, err
	}
	directory := os.NewFile(uintptr(duplicate), "private-cas-windows-create-recovery-empty-directory")
	if directory == nil {
		_ = windows.CloseHandle(duplicate)
		return false, errors.New("private CAS Windows create-residue directory handle is invalid")
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

func privateCASWindowsCreateRecoveryExactIdentity(
	handle windows.Handle,
	volume uint64,
) (privateCASCreateResidueWindowsExactIdentity, error) {
	object, err := privateWindowsValidateAuthorityObject(handle, true, 0)
	if err != nil || object.id.VolumeSerialNumber != volume {
		return privateCASCreateResidueWindowsExactIdentity{}, errors.Join(
			errors.New("private CAS Windows create-residue directory authority is unsafe"),
			err,
		)
	}
	securitySHA256, err := privateCASWindowsSecurityDescriptorDigest(handle)
	if err != nil {
		return privateCASCreateResidueWindowsExactIdentity{}, err
	}
	return privateCASCreateResidueWindowsExactIdentity{
		stable: privateCASCreateResidueWindowsStableIdentity{
			id:             object.id,
			attributes:     object.attributes,
			securitySHA256: securitySHA256,
		},
		object: object,
	}, nil
}

func privateCASWindowsSecurityDescriptorDigest(handle windows.Handle) ([32]byte, error) {
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil || descriptor == nil {
		return [32]byte{}, errors.New("private CAS Windows create-residue security descriptor is unavailable")
	}
	length := descriptor.Length()
	if length == 0 || length > 64<<10 {
		return [32]byte{}, errors.New("private CAS Windows create-residue security descriptor length is invalid")
	}
	body := unsafe.Slice((*byte)(unsafe.Pointer(descriptor)), int(length))
	return sha256.Sum256(body), nil
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
		return errors.Join(errors.New("private CAS Windows create-residue preflight changed before deletion"), err)
	}
	if err := privateCASCreateResidueRecoveryTestCut("after_global_preflight", -1); err != nil {
		return err
	}
	dataDir, present, err := privateCASWindowsOpenCreateRecoveryDataDir(binding)
	if err != nil || present != prepared.dataDirPresent {
		return errors.Join(errors.New("private CAS Windows create-residue data root changed before deletion"), err)
	}
	if !present {
		return nil
	}
	defer windows.CloseHandle(dataDir)
	dataIdentity, err := privateCASWindowsCreateRecoveryExactIdentity(
		dataDir,
		binding.RootIdentity.VolumeSerial,
	)
	if err != nil || dataIdentity != prepared.dataDir {
		return errors.Join(errors.New("private CAS Windows create-residue data root identity changed"), err)
	}
	residues := make([]privateCASCreateResidueWindowsCandidate, 0, len(prepared.residues))
	retained := make([]privateCASCreateResidueWindowsCandidate, 0)
	for _, candidate := range prepared.residues {
		if privateCASDirectoryCandidatePreservedV1(candidate.parentRelative, candidate.name, preservedOwners) {
			retained = append(retained, candidate)
		} else {
			residues = append(residues, candidate)
		}
	}
	applied := prepared
	applied.residues = append([]privateCASCreateResidueWindowsCandidate(nil), residues...)
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
	parentIdentities := make(map[string]privateCASCreateResidueWindowsStableIdentity, len(prepared.parents))
	for _, parent := range prepared.parents {
		parentIdentities[parent.relative] = parent.identity.stable
	}
	for index, residue := range residues {
		if err := privateCASContextError(ctx); err != nil {
			return err
		}
		parent, parentPresent, err := privateCASWindowsOpenCreateRecoveryParent(
			dataDir,
			residue.parentRelative,
			binding.RootIdentity.VolumeSerial,
		)
		if err != nil || !parentPresent {
			return errors.Join(errors.New("private CAS Windows create-residue parent disappeared"), err)
		}
		parentIdentity, identityErr := privateCASWindowsCreateRecoveryExactIdentity(
			parent,
			binding.RootIdentity.VolumeSerial,
		)
		if identityErr != nil || parentIdentity.stable != parentIdentities[residue.parentRelative] {
			_ = windows.CloseHandle(parent)
			return errors.Join(errors.New("private CAS Windows create-residue parent identity changed"), identityErr)
		}
		candidate, candidatePresent, openErr := privateCASWindowsOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.VolumeSerial,
		)
		if openErr != nil || !candidatePresent {
			_ = windows.CloseHandle(parent)
			return errors.Join(errors.New("private CAS Windows create residue disappeared"), openErr)
		}
		candidateIdentity, candidateIdentityErr := privateCASWindowsCreateRecoveryExactIdentity(
			candidate,
			binding.RootIdentity.VolumeSerial,
		)
		empty, emptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(candidate)
		if candidateIdentityErr != nil ||
			!privateCASWindowsCreateRecoveryCandidateIdentityMatches(residue, candidateIdentity) ||
			emptyErr != nil || !empty {
			_ = windows.CloseHandle(candidate)
			_ = windows.CloseHandle(parent)
			return errors.Join(
				errors.New("private CAS Windows create residue changed before deletion"),
				candidateIdentityErr,
				emptyErr,
			)
		}
		if err := privateCASCreateResidueRecoveryTestCut("before_delete", index); err != nil {
			_ = windows.CloseHandle(candidate)
			_ = windows.CloseHandle(parent)
			return err
		}
		verification, verificationPresent, verifyErr := privateCASWindowsOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.VolumeSerial,
		)
		if verifyErr != nil || !verificationPresent {
			_ = windows.CloseHandle(candidate)
			_ = windows.CloseHandle(parent)
			return errors.Join(errors.New("private CAS Windows create residue changed at deletion"), verifyErr)
		}
		verificationIdentity, verificationIdentityErr := privateCASWindowsCreateRecoveryExactIdentity(
			verification,
			binding.RootIdentity.VolumeSerial,
		)
		verificationEmpty, verificationEmptyErr := privateCASWindowsCreateRecoveryDirectoryEmpty(verification)
		verificationCloseErr := windows.CloseHandle(verification)
		if verificationIdentityErr != nil ||
			!privateCASWindowsCreateRecoveryCandidateIdentityMatches(residue, verificationIdentity) ||
			verificationEmptyErr != nil || !verificationEmpty || verificationCloseErr != nil {
			_ = windows.CloseHandle(candidate)
			_ = windows.CloseHandle(parent)
			return errors.Join(
				errors.New("private CAS Windows create residue failed final identity validation"),
				verificationIdentityErr,
				verificationEmptyErr,
				verificationCloseErr,
			)
		}
		candidateCloseErr := windows.CloseHandle(candidate)
		if candidateCloseErr != nil {
			_ = windows.CloseHandle(parent)
			return errors.Join(
				errors.New("private CAS Windows create residue candidate could not be released before its delete guard"),
				candidateCloseErr,
			)
		}
		if err := privateCASCreateResidueRecoveryTestCut("after_final_validation_before_delete", index); err != nil {
			_ = windows.CloseHandle(parent)
			return err
		}
		guard, guardErr := privateCASWindowsOpenDeletionGuard(
			parent,
			residue,
			binding.RootIdentity.VolumeSerial,
		)
		if guardErr != nil {
			_ = windows.CloseHandle(parent)
			return guardErr
		}
		deleteErr := privateWindowsDeleteHandle(guard)
		stillNamed, stillPresent, absenceErr := privateCASWindowsOpenExactBoundDirectory(
			parent,
			residue.name,
			binding.RootIdentity.VolumeSerial,
		)
		if stillPresent {
			_ = windows.CloseHandle(stillNamed)
		}
		if absenceErr == nil && stillPresent {
			absenceErr = errors.New("private CAS Windows deleted create-residue name is still present")
		}
		syncErr := privateWindowsSyncDirectory(parent)
		guardCloseErr := windows.CloseHandle(guard)
		parentCloseErr := windows.CloseHandle(parent)
		if deleteErr != nil || absenceErr != nil || syncErr != nil ||
			guardCloseErr != nil || parentCloseErr != nil {
			return errors.Join(deleteErr, absenceErr, syncErr, guardCloseErr, parentCloseErr)
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
		!privateCASWindowsRetainedDirectoryCandidatesEqualV1(retained, second.residues) ||
		!privateCASWindowsCreateRecoveryPostTopologyEqual(applied, second) {
		return errors.Join(errors.New("private CAS Windows directory cleanup did not produce the exact retained topology"), err)
	}
	return nil
}

func privateCASWindowsCreateRecoveryParentTopologyEqual(
	left privateCASCreateResiduePreparedPlan,
	right privateCASCreateResiduePreparedPlan,
) bool {
	return left.dataDirPresent == right.dataDirPresent &&
		left.dataDir == right.dataDir &&
		slices.Equal(left.parents, right.parents)
}

func privateCASWindowsCreateRecoveryPostTopologyEqual(
	left privateCASCreateResiduePreparedPlan,
	right privateCASCreateResiduePreparedPlan,
) bool {
	if left.dataDirPresent != right.dataDirPresent ||
		left.dataDir.stable != right.dataDir.stable {
		return false
	}
	removed := make(map[string]struct{})
	for _, candidate := range left.residues {
		if candidate.kind != privateCASCreateResidueWindowsCandidateEmptyLeaf &&
			candidate.kind != privateCASCreateResidueWindowsCandidateEmptyOwner {
			continue
		}
		removed[path.Join(candidate.parentRelative, candidate.name)] = struct{}{}
	}
	expected := make([]privateCASCreateResidueWindowsParent, 0, len(left.parents))
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
			expected[index].identity.stable != right.parents[index].identity.stable {
			return false
		}
	}
	return true
}

func privateCASWindowsCreateRecoveryCandidateIdentityMatches(
	prepared privateCASCreateResidueWindowsCandidate,
	current privateCASCreateResidueWindowsExactIdentity,
) bool {
	switch prepared.kind {
	case privateCASCreateResidueWindowsCandidateEmptyLeaf,
		privateCASCreateResidueWindowsCandidateEmptyOwner:
		return prepared.identity.stable == current.stable
	default:
		return prepared.identity == current
	}
}

func privateCASCreateResiduePreparedPlansEqual(
	left privateCASCreateResiduePreparedPlan,
	right privateCASCreateResiduePreparedPlan,
) bool {
	return privateCASWindowsCreateRecoveryParentTopologyEqual(left, right) &&
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
		case privateCASCreateResidueWindowsCandidateEmptyOwner:
			if relative != owner {
				return false
			}
			count++
		case privateCASCreateResidueWindowsCandidateEmptyLeaf, privateCASCreateResidueWindowsCandidateEmptyShard:
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
		if residue.kind == privateCASCreateResidueWindowsCandidateTemporary && selected {
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
	if original.dataDirPresent != current.dataDirPresent || original.dataDir.stable != current.dataDir.stable || !slices.Equal(original.residues, current.residues) {
		return false
	}
	for _, parent := range original.parents {
		found := false
		for _, now := range current.parents {
			if parent.relative == now.relative {
				if parent.identity.stable != now.identity.stable {
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
	expected := make([]privateCASCreateResidueWindowsCandidate, 0)
	for _, residue := range original.residues {
		if residue.parentRelative == leaf {
			expected = append(expected, privateCASCreateResidueWindowsCandidate{name: residue.name, identity: residue.identity})
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
func privateCASWindowsRetainedDirectoryCandidatesEqualV1(expected, current []privateCASCreateResidueWindowsCandidate) bool {
	if len(expected) != len(current) {
		return false
	}
	for index, candidate := range expected {
		now := current[index]
		if candidate.parentRelative != now.parentRelative || candidate.name != now.name || candidate.component != now.component || candidate.kind != now.kind || !privateCASWindowsCreateRecoveryCandidateIdentityMatches(candidate, now.identity) {
			return false
		}
	}
	return true
}

func privateCASOriginalCreateFingerprintMaterialV1(plan privateCASCreateResiduePreparedPlan) string {
	var material strings.Builder
	fmt.Fprintf(&material, "%t\n%#v\n", plan.dataDirPresent, plan.dataDir.stable)
	for _, parent := range plan.parents {
		fmt.Fprintf(&material, "%q\n%#v\n", parent.relative, parent.identity.stable)
	}
	fmt.Fprintf(&material, "%#v\n", plan.residues)
	return material.String()
}

func privateCASOriginalCreateDirectoryStatesV1(plan privateCASCreateResiduePreparedPlan) []privatecasport.OriginalCreateDirectoryV1 {
	states := make([]privatecasport.OriginalCreateDirectoryV1, 0, len(plan.residues))
	for _, residue := range plan.residues {
		mode := uint32(os.ModeDir) | 0o777
		if residue.identity.stable.attributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
			mode = uint32(os.ModeDir) | 0o555
		}
		states = append(states, privatecasport.OriginalCreateDirectoryV1{RelativePath: path.Join(residue.parentRelative, residue.name), Mode: mode})
	}
	return states
}
