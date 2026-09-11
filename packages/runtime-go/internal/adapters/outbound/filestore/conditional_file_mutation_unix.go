//go:build darwin || linux

package filestore

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const conditionalUnixRecoveryEntryLimit = 4096

const (
	conditionalUnixMoveIntentFile     = "intent-v1.json"
	conditionalUnixMoveIntentPurpose  = "analytix.conditional-move-intent/v1"
	conditionalUnixMoveIntentMaxBytes = 2 * 1024 * 1024
	conditionalUnixMoveStageFile      = "stage-authority-v1.json"
	conditionalUnixMoveStagePurpose   = "analytix.conditional-move-stage-authority/v1"
	conditionalUnixMoveStageMaxBytes  = 64 * 1024
)

type conditionalUnixObservation struct {
	exists   bool
	hash     string
	size     int64
	mode     os.FileMode
	identity atomicUnixIdentity
	stat     unix.Stat_t
}

type conditionalUnixRecoveryEntry struct {
	name        string
	settled     bool
	hash        string
	identity    atomicUnixIdentity
	observation conditionalUnixObservation
}

type conditionalUnixMoveIntentBinding struct {
	SchemaVersion             int    `json:"schemaVersion"`
	Purpose                   string `json:"purpose"`
	OperationGroupID          string `json:"operationGroupId"`
	OperationIntentDigest     string `json:"operationIntentDigest"`
	SourceAuthorityHash       string `json:"sourceAuthorityHash"`
	SourceRelativePath        string `json:"sourceRelativePath"`
	DestinationAuthorityHash  string `json:"destinationAuthorityHash"`
	DestinationRelativePath   string `json:"destinationRelativePath"`
	SourceHash                string `json:"sourceHash"`
	SourceSizeBytes           int64  `json:"sourceSizeBytes"`
	SourceEncoding            string `json:"sourceEncoding"`
	SourceBytesBase64         string `json:"sourceBytesBase64"`
	SourceMode                uint32 `json:"sourceMode"`
	SourceDevice              uint64 `json:"sourceDevice"`
	SourceInode               uint64 `json:"sourceInode"`
	SourceRevision            string `json:"sourceRevision"`
	AuthorityDevice           uint64 `json:"authorityDevice"`
	AuthorityInode            uint64 `json:"authorityInode"`
	DestinationParentMissing  bool   `json:"destinationParentMissing"`
	DestinationExistingParent string `json:"destinationExistingParent,omitempty"`
	DestinationExistingDevice uint64 `json:"destinationExistingDevice,omitempty"`
	DestinationExistingInode  uint64 `json:"destinationExistingInode,omitempty"`
	DestinationExistingMode   uint32 `json:"destinationExistingMode,omitempty"`
	DestinationTopMissing     string `json:"destinationTopMissing,omitempty"`
	DestinationRelativeTail   string `json:"destinationRelativeTail,omitempty"`
	DestinationStageName      string `json:"destinationStageName,omitempty"`
	BindingDigest             string `json:"bindingDigest"`
}

type conditionalUnixMoveStageAuthority struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Purpose               string `json:"purpose"`
	OperationGroupID      string `json:"operationGroupId"`
	OperationIntentDigest string `json:"operationIntentDigest"`
	BindingDigest         string `json:"bindingDigest"`
	StageName             string `json:"stageName"`
	StageDevice           uint64 `json:"stageDevice"`
	StageInode            uint64 `json:"stageInode"`
	StageMode             uint32 `json:"stageMode"`
	ExistingParentDevice  uint64 `json:"existingParentDevice"`
	ExistingParentInode   uint64 `json:"existingParentInode"`
	TopMissing            string `json:"topMissing"`
	RelativeTail          string `json:"relativeTail"`
	ReceiptDigest         string `json:"receiptDigest"`
}

func prepareConditionalMoveSourcePlatform(path string) (conditionalMoveSourceObservation, error) {
	parent, base, missing, err := openAtomicUnixParent(path, false)
	if err != nil {
		return conditionalMoveSourceObservation{}, err
	}
	if missing {
		return conditionalMoveSourceObservation{}, os.ErrNotExist
	}
	defer unix.Close(parent)
	observation, err := observeConditionalUnixAt(parent, base)
	if err != nil {
		return conditionalMoveSourceObservation{}, err
	}
	if !observation.exists {
		return conditionalMoveSourceObservation{}, os.ErrNotExist
	}
	return conditionalMoveSourceObservation{
		Hash: observation.hash, Size: observation.size, Mode: observation.mode,
		Device: observation.identity.dev, Inode: observation.identity.ino, Revision: conditionalUnixRevision(observation.stat),
	}, nil
}

func openConditionalMutationAuthorityPlatform(root string) (ConditionalMutationAuthority, error) {
	canonicalRoot, err := canonicalAtomicUnixSystemAlias(root)
	if err != nil {
		return ConditionalMutationAuthority{}, err
	}
	root = canonicalRoot
	fd, err := openConditionalUnixAuthorityRoot(root)
	if err != nil {
		return ConditionalMutationAuthority{}, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return ConditionalMutationAuthority{}, err
	}
	return ConditionalMutationAuthority{
		root: root, device: uint64(stat.Dev), inode: uint64(stat.Ino), available: true,
	}, nil
}

func openExistingConditionalMutationAuthorityPlatform(root string) (ConditionalMutationAuthority, bool, error) {
	canonicalRoot, err := canonicalAtomicUnixSystemAlias(root)
	if err != nil {
		return ConditionalMutationAuthority{}, false, err
	}
	parent, base, missing, err := openAtomicUnixParent(canonicalRoot, false)
	if err != nil {
		return ConditionalMutationAuthority{}, false, err
	}
	if missing {
		return ConditionalMutationAuthority{}, false, nil
	}
	defer unix.Close(parent)
	fd, err := openExistingConditionalUnixDirectory(parent, base)
	if errors.Is(err, unix.ENOENT) {
		return ConditionalMutationAuthority{}, false, nil
	}
	if err != nil {
		return ConditionalMutationAuthority{}, false, err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return ConditionalMutationAuthority{}, false, err
	}
	return ConditionalMutationAuthority{
		root: canonicalRoot, device: uint64(stat.Dev), inode: uint64(stat.Ino), available: true,
	}, true, nil
}

func prepareConditionalMoveDestinationPlatform(path string) (bool, error) {
	parent, base, missing, err := openAtomicUnixParent(path, false)
	if err != nil {
		return false, err
	}
	if missing {
		return false, nil
	}
	defer unix.Close(parent)
	observation, err := observeConditionalUnixAt(parent, base)
	if err != nil {
		return false, err
	}
	return observation.exists, nil
}

func applyConditionalMovePlatform(plan MoveRegularFilePlan) error {
	if !plan.mutationAuthority.Available() {
		return fmt.Errorf("%w: move requires a host-private mutation authority", ErrAtomicTextUnsupportedPlatform)
	}
	sourceParent, sourceBase, missing, err := openAtomicUnixParent(plan.SourcePath, false)
	if err != nil {
		return err
	}
	if missing {
		return fmt.Errorf("%w: move source parent is absent", ErrConditionalMutationResidue)
	}
	defer unix.Close(sourceParent)
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(plan.mutationAuthority)
	if err != nil {
		return err
	}
	defer unix.Close(authorityRoot)
	if same, sameErr := conditionalUnixSameDevice(sourceParent, authorityRoot); sameErr != nil || !same {
		return errors.Join(sameErr, fmt.Errorf("%w: move source and authority are on different filesystems", ErrAtomicTextUnsupportedPlatform))
	}
	journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, true)
	if err != nil {
		return err
	}
	defer unix.Close(journal)
	plan, err = ensureConditionalUnixMoveIntent(journal, plan)
	if err != nil {
		return err
	}
	if plan.destinationExistingDevice != plan.mutationAuthority.device || plan.testHooks != nil && plan.testHooks.ForceCrossDevice {
		return fmt.Errorf("%w: cross-filesystem move is unavailable", ErrAtomicTextUnsupportedPlatform)
	}
	destinationMissing := plan.destinationParentMissing
	pendingEntries, err := observeConditionalUnixMoveJournal(journal, plan)
	if err != nil {
		return err
	}
	if destinationMissing {
		return applyConditionalUnixStagedDirectoryMove(plan, sourceParent, sourceBase, authorityRoot, journal, pendingEntries)
	}
	var destinationParent int = -1
	var destinationBase string
	var createdDirectory conditionalUnixCreatedDirectory
	openDestination := func() error {
		if destinationParent >= 0 {
			return nil
		}
		var openErr error
		destinationParent, destinationBase, createdDirectory, openErr = openConditionalUnixDestinationParent(plan.DestinationPath, destinationMissing)
		if openErr != nil {
			return openErr
		}
		return nil
	}
	defer func() {
		if destinationParent >= 0 {
			_ = unix.Close(destinationParent)
		}
		createdDirectory.close()
	}()
	if !destinationMissing {
		if err := openDestination(); err != nil {
			return err
		}
	}

	initial, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil {
		return err
	}
	destination := conditionalUnixObservation{}
	if destinationParent >= 0 {
		destination, err = observeConditionalUnixAt(destinationParent, destinationBase)
		if err != nil {
			return err
		}
	}
	if len(pendingEntries) == 0 && !initial.exists && conditionalUnixMatchesMoveInstalled(plan, destination) {
		return syncConditionalUnixParents(destinationParent, journal)
	}
	if len(pendingEntries) == 1 {
		if initial.exists || destination.exists {
			return ErrConditionalMutationResidue
		}
		if err := openDestination(); err != nil {
			return err
		}
		err := installConditionalUnixMoveDestination(plan, sourceParent, sourceBase, destinationParent, destinationBase, journal, pendingEntries[0])
		return finalizeConditionalUnixCreatedDirectory(authorityRoot, destinationParent, destinationBase, createdDirectory, plan, err)
	}
	if !initial.exists {
		return fmt.Errorf("%w: move source is absent without an exact private pending entry", ErrConditionalMutationResidue)
	}
	if err := validateConditionalUnixMoveSource(plan, initial); err != nil {
		return err
	}
	if destination.exists {
		return fmt.Errorf("%w: move destination appeared after preparation", ErrAtomicTextBeforeDrift)
	}
	if plan.testHooks != nil && plan.testHooks.AfterInitialValidation != nil {
		plan.testHooks.AfterInitialValidation()
	}
	current, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil {
		return err
	}
	if err := validateConditionalUnixMoveSource(plan, current); err != nil || current.identity != initial.identity {
		return errors.Join(err, fmt.Errorf("%w: move source identity changed", ErrAtomicTextBeforeDrift))
	}
	if destination.exists {
		return fmt.Errorf("%w: move destination changed before rename", ErrAtomicTextBeforeDrift)
	}
	if plan.testHooks != nil && plan.testHooks.BeforeRename != nil {
		plan.testHooks.BeforeRename()
	}
	pendingName, err := conditionalUnixMovePendingName(plan)
	if err != nil {
		return err
	}
	if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, pendingName); err != nil {
		return err
	}
	if plan.testHooks != nil && plan.testHooks.AfterSourceRenameBeforeVerify != nil {
		plan.testHooks.AfterSourceRenameBeforeVerify()
	}
	rollbackPending := func() error {
		if err := renameConditionalUnixNoReplace(journal, pendingName, sourceParent, sourceBase); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		return nil
	}
	pending, err := observeConditionalUnixAt(journal, pendingName)
	if err != nil || !conditionalUnixMatchesMovePrivate(plan, pending) {
		return errors.Join(ErrAtomicTextBeforeDrift, err, rollbackPending())
	}
	if plan.testHooks != nil && plan.testHooks.AfterSourceQuarantine != nil {
		plan.testHooks.AfterSourceQuarantine()
	}
	sourceAfter, sourceErr := observeConditionalUnixAt(sourceParent, sourceBase)
	if sourceErr != nil || sourceAfter.exists {
		return errors.Join(ErrConditionalMutationResidue, sourceErr)
	}
	if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
		rollbackErr := rollbackPending()
		if rollbackErr != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err, rollbackErr)
		}
		return err
	}
	if err := openDestination(); err != nil {
		rollbackErr := rollbackPending()
		return errors.Join(err, rollbackErr)
	}
	err = installConditionalUnixMoveDestination(plan, sourceParent, sourceBase, destinationParent, destinationBase, journal, conditionalUnixRecoveryEntry{
		name: pendingName, hash: plan.sourceHash, identity: initial.identity, observation: pending,
	})
	return finalizeConditionalUnixCreatedDirectory(authorityRoot, destinationParent, destinationBase, createdDirectory, plan, err)
}

func installConditionalUnixMoveDestination(
	plan MoveRegularFilePlan,
	sourceParent int,
	sourceBase string,
	destinationParent int,
	destinationBase string,
	journal int,
	pending conditionalUnixRecoveryEntry,
) error {
	source, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || source.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	destination, err := observeConditionalUnixAt(destinationParent, destinationBase)
	if err != nil || destination.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	private, err := observeConditionalUnixAt(journal, pending.name)
	if err != nil || !conditionalUnixMatchesMovePrivate(plan, private) || private.identity != pending.identity {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if plan.testHooks != nil && plan.testHooks.BeforeDestinationInstall != nil {
		plan.testHooks.BeforeDestinationInstall()
	}
	source, err = observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || source.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	destination, err = observeConditionalUnixAt(destinationParent, destinationBase)
	if err != nil || destination.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	private, err = observeConditionalUnixAt(journal, pending.name)
	if err != nil || !conditionalUnixMatchesMovePrivate(plan, private) || private.identity != pending.identity {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if err := renameConditionalUnixNoReplace(journal, pending.name, destinationParent, destinationBase); err != nil {
		return err
	}
	installed, observeErr := observeConditionalUnixAt(destinationParent, destinationBase)
	if observeErr != nil || !conditionalUnixMatchesMoveInstalled(plan, installed) {
		rollbackErr := renameConditionalUnixNoReplace(destinationParent, destinationBase, journal, pending.name)
		if rollbackErr == nil {
			rollbackErr = syncConditionalUnixParents(destinationParent, journal)
		}
		return errors.Join(ErrConditionalMutationIndeterminate, observeErr, errors.New("move destination exact readback failed"), rollbackErr)
	}
	if plan.testHooks != nil && plan.testHooks.AfterRename != nil {
		if hookErr := plan.testHooks.AfterRename(); hookErr != nil {
			return errors.Join(ErrConditionalMutationResidue, hookErr)
		}
	}
	source, err = observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || source.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if err := syncConditionalUnixParents(sourceParent, destinationParent); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	if err := unix.Fsync(journal); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	installed, err = observeConditionalUnixAt(destinationParent, destinationBase)
	if err != nil || !conditionalUnixMatchesMoveInstalled(plan, installed) {
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("move destination durable readback failed"))
	}
	return nil
}

type conditionalUnixCreatedDirectory struct {
	parent   int
	name     string
	identity atomicUnixIdentity
}

func (created *conditionalUnixCreatedDirectory) close() {
	if created != nil && created.parent >= 0 {
		_ = unix.Close(created.parent)
		created.parent = -1
	}
}

func freezeConditionalUnixMoveDestinationTopology(plan MoveRegularFilePlan) (MoveRegularFilePlan, error) {
	clean, err := canonicalAtomicUnixSystemAlias(plan.DestinationPath)
	if err != nil {
		return MoveRegularFilePlan{}, err
	}
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return MoveRegularFilePlan{}, err
	}
	defer unix.Close(current)
	components := strings.Split(strings.TrimPrefix(filepath.Dir(clean), string(filepath.Separator)), string(filepath.Separator))
	currentPath := string(filepath.Separator)
	for index, component := range components {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			return MoveRegularFilePlan{}, fmt.Errorf("%w: destination parent traversal is invalid", ErrAtomicTextUnsafePath)
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			var stat unix.Stat_t
			if err := unix.Fstat(current, &stat); err != nil {
				return MoveRegularFilePlan{}, err
			}
			tail := append([]string(nil), components[index+1:]...)
			tail = append(tail, filepath.Base(clean))
			stageName, err := conditionalUnixRandomName(".move-stage-v1-")
			if err != nil {
				return MoveRegularFilePlan{}, err
			}
			plan.destinationParentMissing = true
			plan.destinationExistingParent = filepath.Clean(currentPath)
			plan.destinationExistingDevice = uint64(stat.Dev)
			plan.destinationExistingInode = uint64(stat.Ino)
			plan.destinationExistingMode = os.FileMode(stat.Mode & 0o777)
			plan.destinationTopMissing = component
			plan.destinationRelativeTail = filepath.ToSlash(filepath.Join(tail...))
			plan.destinationStageName = stageName
			return plan, nil
		}
		if openErr != nil {
			return MoveRegularFilePlan{}, fmt.Errorf("%w: destination parent is unsafe: %v", ErrAtomicTextUnsafePath, openErr)
		}
		_ = unix.Close(current)
		current = next
		currentPath = filepath.Join(currentPath, component)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(current, &stat); err != nil {
		return MoveRegularFilePlan{}, err
	}
	plan.destinationParentMissing = false
	plan.destinationExistingParent = filepath.Clean(filepath.Dir(clean))
	plan.destinationExistingDevice = uint64(stat.Dev)
	plan.destinationExistingInode = uint64(stat.Ino)
	plan.destinationExistingMode = os.FileMode(stat.Mode & 0o777)
	return plan, nil
}

func hydrateConditionalUnixMovePlanTopology(plan MoveRegularFilePlan, binding conditionalUnixMoveIntentBinding) MoveRegularFilePlan {
	plan.destinationParentMissing = binding.DestinationParentMissing
	plan.destinationExistingParent = binding.DestinationExistingParent
	plan.destinationExistingDevice = binding.DestinationExistingDevice
	plan.destinationExistingInode = binding.DestinationExistingInode
	plan.destinationExistingMode = os.FileMode(binding.DestinationExistingMode)
	plan.destinationTopMissing = binding.DestinationTopMissing
	plan.destinationRelativeTail = binding.DestinationRelativeTail
	plan.destinationStageName = binding.DestinationStageName
	return plan
}

func conditionalUnixSafeComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value &&
		!strings.ContainsRune(value, filepath.Separator) && !strings.ContainsRune(value, '\x00')
}

func conditionalUnixSafeRelativeTail(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func openConditionalUnixDestinationParent(path string, create bool) (int, string, conditionalUnixCreatedDirectory, error) {
	created := conditionalUnixCreatedDirectory{parent: -1}
	clean, err := canonicalAtomicUnixSystemAlias(path)
	if err != nil {
		return -1, "", created, err
	}
	base := filepath.Base(clean)
	if base == "" || base == "." || base == ".." {
		return -1, "", created, fmt.Errorf("%w: destination name is invalid", ErrAtomicTextUnsafePath)
	}
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, "", created, err
	}
	components := strings.Split(strings.TrimPrefix(filepath.Dir(clean), string(filepath.Separator)), string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			_ = unix.Close(current)
			created.close()
			return -1, "", created, fmt.Errorf("%w: destination parent traversal is invalid", ErrAtomicTextUnsafePath)
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && create {
			if mkdirErr := unix.Mkdirat(current, component, 0o755); mkdirErr != nil {
				_ = unix.Close(current)
				created.close()
				return -1, "", created, mkdirErr
			}
			if syncErr := unix.Fsync(current); syncErr != nil {
				_ = unix.Close(current)
				created.close()
				return -1, "", created, syncErr
			}
			next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if openErr == nil && created.parent < 0 {
				parentCopy, duplicateErr := unix.Dup(current)
				if duplicateErr != nil {
					_ = unix.Close(next)
					_ = unix.Close(current)
					return -1, "", created, duplicateErr
				}
				var stat unix.Stat_t
				if statErr := unix.Fstat(next, &stat); statErr != nil {
					_ = unix.Close(parentCopy)
					_ = unix.Close(next)
					_ = unix.Close(current)
					return -1, "", created, statErr
				}
				created = conditionalUnixCreatedDirectory{
					parent: parentCopy, name: component,
					identity: atomicUnixIdentity{dev: uint64(stat.Dev), ino: uint64(stat.Ino)},
				}
			}
		}
		if openErr != nil {
			_ = unix.Close(current)
			created.close()
			return -1, "", created, fmt.Errorf("%w: open destination parent: %v", ErrAtomicTextUnsafePath, openErr)
		}
		_ = unix.Close(current)
		current = next
	}
	return current, base, created, nil
}

func finalizeConditionalUnixCreatedDirectory(
	authorityRoot int,
	destinationParent int,
	destinationBase string,
	created conditionalUnixCreatedDirectory,
	plan MoveRegularFilePlan,
	mutationErr error,
) error {
	if mutationErr == nil || created.parent < 0 {
		return mutationErr
	}
	destination, observeErr := observeConditionalUnixAt(destinationParent, destinationBase)
	if observeErr == nil && conditionalUnixMatchesMoveInstalled(plan, destination) {
		return mutationErr
	}
	residues, err := openOrCreateConditionalUnixDirectory(authorityRoot, "move-directory-residues-v1")
	if err != nil {
		return errors.Join(mutationErr, ErrConditionalMutationIndeterminate, err)
	}
	defer unix.Close(residues)
	residueName, err := conditionalUnixRandomName(".move-directory-v1-")
	if err != nil {
		return errors.Join(mutationErr, ErrConditionalMutationIndeterminate, err)
	}
	var linked unix.Stat_t
	if err := unix.Fstatat(created.parent, created.name, &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		uint64(linked.Dev) != created.identity.dev || uint64(linked.Ino) != created.identity.ino || linked.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.Join(mutationErr, ErrConditionalMutationIndeterminate, err, errors.New("created destination directory identity changed"))
	}
	if err := renameConditionalUnixNoReplace(created.parent, created.name, residues, residueName); err != nil {
		return errors.Join(mutationErr, ErrConditionalMutationIndeterminate, err)
	}
	if err := syncConditionalUnixParents(created.parent, residues); err != nil {
		return errors.Join(mutationErr, ErrConditionalMutationIndeterminate, err)
	}
	if err := unix.Fstatat(residues, residueName, &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		uint64(linked.Dev) != created.identity.dev || uint64(linked.Ino) != created.identity.ino || linked.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.Join(mutationErr, ErrConditionalMutationIndeterminate, err, errors.New("created destination directory quarantine readback failed"))
	}
	return mutationErr
}

func openConditionalUnixBoundAuthorityRoot(authority ConditionalMutationAuthority) (int, error) {
	root, err := openConditionalUnixAuthorityRoot(authority.root)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil || uint64(stat.Dev) != authority.device || uint64(stat.Ino) != authority.inode {
		_ = unix.Close(root)
		return -1, errors.Join(err, fmt.Errorf("%w: conditional mutation authority identity changed", ErrAtomicTextBeforeDrift))
	}
	return root, nil
}

func openConditionalUnixMoveJournal(authorityRoot int, plan MoveRegularFilePlan, create bool) (int, error) {
	if len(plan.operationGroupID) != 64 {
		return -1, errors.New("conditional move operation id is invalid")
	}
	openDirectory := openExistingConditionalUnixDirectory
	if create {
		openDirectory = openOrCreateConditionalUnixDirectory
	}
	moves, err := openDirectory(authorityRoot, "moves-v1")
	if err != nil {
		return -1, err
	}
	shardName := plan.operationGroupID[:2]
	bindingName := plan.operationGroupID
	shard, err := openDirectory(moves, shardName)
	_ = unix.Close(moves)
	if err != nil {
		return -1, err
	}
	binding, err := openDirectory(shard, bindingName)
	_ = unix.Close(shard)
	return binding, err
}

func openExistingConditionalUnixDirectory(parent int, name string) (int, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return -1, errors.Join(err, fmt.Errorf("%w: conditional mutation journal directory authority is unsafe", ErrAtomicTextUnsafePath))
	}
	return fd, nil
}

func ensureConditionalUnixMoveIntent(journal int, plan MoveRegularFilePlan) (MoveRegularFilePlan, error) {
	current, found, err := readConditionalUnixMoveIntent(journal)
	if err != nil {
		return MoveRegularFilePlan{}, errors.Join(ErrConditionalMutationResidue, err)
	}
	if found {
		plan = hydrateConditionalUnixMovePlanTopology(plan, current)
		expected, _, expectedErr := conditionalUnixMoveIntentForPlan(plan)
		if expectedErr != nil {
			return MoveRegularFilePlan{}, expectedErr
		}
		if current != expected {
			return MoveRegularFilePlan{}, errors.New("conditional move journal intent conflicts with durable operation authority")
		}
		return plan, nil
	}
	plan, err = freezeConditionalUnixMoveDestinationTopology(plan)
	if err != nil {
		return MoveRegularFilePlan{}, err
	}
	expected, body, err := conditionalUnixMoveIntentForPlan(plan)
	if err != nil {
		return MoveRegularFilePlan{}, err
	}
	fd, err := unix.Openat(
		journal, conditionalUnixMoveIntentFile,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return MoveRegularFilePlan{}, err
	}
	file := os.NewFile(uintptr(fd), conditionalUnixMoveIntentFile)
	if file == nil {
		_ = unix.Close(fd)
		return MoveRegularFilePlan{}, errors.New("conditional move intent file handle is invalid")
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return MoveRegularFilePlan{}, errors.Join(ErrConditionalMutationIndeterminate, writeErr, syncErr, closeErr)
	}
	if err := unix.Fsync(journal); err != nil {
		return MoveRegularFilePlan{}, errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	current, found, err = readConditionalUnixMoveIntent(journal)
	if err != nil || !found || current != expected {
		return MoveRegularFilePlan{}, errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("conditional move intent durable readback failed"))
	}
	return plan, nil
}

func conditionalUnixMoveIntentForPlan(plan MoveRegularFilePlan) (conditionalUnixMoveIntentBinding, []byte, error) {
	record := conditionalUnixMoveIntentBinding{
		SchemaVersion: 1, Purpose: conditionalUnixMoveIntentPurpose,
		OperationGroupID: plan.operationGroupID, OperationIntentDigest: plan.operationDigest,
		SourceAuthorityHash: plan.sourceAuthority, SourceRelativePath: plan.sourceRelative,
		DestinationAuthorityHash: plan.destinationAuthority, DestinationRelativePath: plan.destinationRelative,
		SourceHash: plan.sourceHash, SourceSizeBytes: plan.BytesMoved, SourceEncoding: plan.sourceEncoding,
		SourceBytesBase64: plan.sourceBytesBase64, SourceMode: uint32(plan.sourceMode.Perm()),
		SourceDevice: plan.sourceDevice, SourceInode: plan.sourceInode, SourceRevision: plan.sourceRevision,
		AuthorityDevice: plan.mutationAuthority.device, AuthorityInode: plan.mutationAuthority.inode,
		DestinationParentMissing:  plan.destinationParentMissing,
		DestinationExistingParent: plan.destinationExistingParent,
		DestinationExistingDevice: plan.destinationExistingDevice,
		DestinationExistingInode:  plan.destinationExistingInode,
		DestinationExistingMode:   uint32(plan.destinationExistingMode.Perm()),
		DestinationTopMissing:     plan.destinationTopMissing,
		DestinationRelativeTail:   plan.destinationRelativeTail,
		DestinationStageName:      plan.destinationStageName,
	}
	record.BindingDigest = conditionalUnixMoveIntentDigest(record)
	body, err := json.Marshal(record)
	if err != nil || len(body) == 0 || len(body) > conditionalUnixMoveIntentMaxBytes || !validConditionalUnixMoveIntent(record) {
		return conditionalUnixMoveIntentBinding{}, nil, errors.Join(err, errors.New("conditional move intent binding is invalid"))
	}
	return record, body, nil
}

func conditionalUnixMoveIntentDigest(record conditionalUnixMoveIntentBinding) string {
	record.BindingDigest = ""
	body, _ := json.Marshal(record)
	return digestAtomicText(body)
}

func validConditionalUnixMoveIntent(record conditionalUnixMoveIntentBinding) bool {
	raw, err := base64.StdEncoding.Strict().DecodeString(record.SourceBytesBase64)
	return record.SchemaVersion == 1 && record.Purpose == conditionalUnixMoveIntentPurpose &&
		conditionalUnixSHA256(record.OperationGroupID) && conditionalUnixSHA256(record.OperationIntentDigest) &&
		conditionalUnixSHA256(record.SourceAuthorityHash) && conditionalUnixSHA256(record.DestinationAuthorityHash) &&
		record.SourceRelativePath != "" && record.DestinationRelativePath != "" &&
		conditionalUnixSHA256(record.SourceHash) && record.SourceSizeBytes >= 0 && err == nil &&
		int64(len(raw)) == record.SourceSizeBytes && digestAtomicText(raw) == record.SourceHash &&
		record.SourceEncoding != "" && record.SourceMode <= 0o777 && record.SourceDevice != 0 && record.SourceInode != 0 &&
		record.SourceRevision != "" && record.AuthorityDevice != 0 && record.AuthorityInode != 0 &&
		validConditionalUnixMoveDestinationTopology(record) &&
		conditionalUnixSHA256(record.BindingDigest) && record.BindingDigest == conditionalUnixMoveIntentDigest(record)
}

func validConditionalUnixMoveDestinationTopology(record conditionalUnixMoveIntentBinding) bool {
	if record.DestinationExistingParent == "" || !filepath.IsAbs(record.DestinationExistingParent) ||
		record.DestinationExistingDevice == 0 || record.DestinationExistingInode == 0 || record.DestinationExistingMode > 0o777 {
		return false
	}
	if !record.DestinationParentMissing {
		return record.DestinationTopMissing == "" && record.DestinationRelativeTail == "" && record.DestinationStageName == ""
	}
	return conditionalUnixSafeComponent(record.DestinationTopMissing) &&
		conditionalUnixSafeRelativeTail(record.DestinationRelativeTail) && strings.HasPrefix(record.DestinationStageName, ".move-stage-v1-") &&
		len(record.DestinationStageName) <= 128 && !strings.ContainsRune(record.DestinationStageName, filepath.Separator)
}

func conditionalUnixSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func readConditionalUnixMoveIntent(journal int) (conditionalUnixMoveIntentBinding, bool, error) {
	fd, err := unix.Openat(journal, conditionalUnixMoveIntentFile, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return conditionalUnixMoveIntentBinding{}, false, nil
	}
	if err != nil {
		return conditionalUnixMoveIntentBinding{}, false, err
	}
	file := os.NewFile(uintptr(fd), conditionalUnixMoveIntentFile)
	if file == nil {
		_ = unix.Close(fd)
		return conditionalUnixMoveIntentBinding{}, false, errors.New("conditional move intent handle is invalid")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 ||
		before.Uid != uint32(os.Geteuid()) || before.Mode&0o077 != 0 || before.Size <= 0 || before.Size > conditionalUnixMoveIntentMaxBytes {
		return conditionalUnixMoveIntentBinding{}, false, errors.Join(err, errors.New("conditional move intent file authority is unsafe"))
	}
	body, readErr := io.ReadAll(io.LimitReader(file, conditionalUnixMoveIntentMaxBytes+1))
	var after unix.Stat_t
	var linked unix.Stat_t
	statErr := unix.Fstat(fd, &after)
	linkErr := unix.Fstatat(journal, conditionalUnixMoveIntentFile, &linked, unix.AT_SYMLINK_NOFOLLOW)
	if readErr != nil || statErr != nil || linkErr != nil || len(body) == 0 || len(body) > conditionalUnixMoveIntentMaxBytes ||
		int64(len(body)) != after.Size || !sameConditionalUnixStableStat(before, after) || !sameConditionalUnixStableStat(after, linked) {
		return conditionalUnixMoveIntentBinding{}, false, errors.Join(readErr, statErr, linkErr, errors.New("conditional move intent changed while read"))
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record conditionalUnixMoveIntentBinding
	if err := decoder.Decode(&record); err != nil {
		return conditionalUnixMoveIntentBinding{}, false, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return conditionalUnixMoveIntentBinding{}, false, errors.New("conditional move intent contains trailing JSON")
	}
	if !validConditionalUnixMoveIntent(record) {
		return conditionalUnixMoveIntentBinding{}, false, errors.New("conditional move intent integrity is invalid")
	}
	return record, true, nil
}

func observeConditionalUnixMoveJournal(journal int, plan MoveRegularFilePlan) ([]conditionalUnixRecoveryEntry, error) {
	duplicate, err := unix.Openat(journal, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(duplicate), "conditional-move-journal")
	if directory == nil {
		_ = unix.Close(duplicate)
		return nil, errors.New("conditional move journal handle is invalid")
	}
	defer directory.Close()
	names, err := directory.Readdirnames(conditionalUnixRecoveryEntryLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > conditionalUnixRecoveryEntryLimit {
		return nil, errors.New("conditional move recovery entry bound exceeded")
	}
	entries := make([]conditionalUnixRecoveryEntry, 0, 1)
	for _, name := range names {
		if name == conditionalUnixMoveIntentFile || plan.destinationParentMissing &&
			(name == conditionalUnixMoveStageFile || name == plan.destinationStageName) {
			continue
		}
		digest, identity, ok := parseConditionalUnixMovePendingName(name)
		if !ok || digest != plan.sourceHash || identity.dev != plan.sourceDevice || identity.ino != plan.sourceInode {
			return nil, ErrConditionalMutationResidue
		}
		observation, observeErr := observeConditionalUnixAt(journal, name)
		if observeErr != nil || !conditionalUnixMatchesMovePrivate(plan, observation) || observation.identity != identity {
			return nil, errors.Join(ErrConditionalMutationResidue, observeErr)
		}
		entries = append(entries, conditionalUnixRecoveryEntry{
			name: name, hash: digest, identity: identity, observation: observation,
		})
	}
	if len(entries) > 1 {
		return nil, ErrConditionalMutationResidue
	}
	return entries, nil
}

func conditionalUnixMovePendingName(plan MoveRegularFilePlan) (string, error) {
	nonce, err := conditionalUnixRandomName("")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(".move-pending-v1-%s-%016x-%016x-%s", plan.sourceHash, plan.sourceDevice, plan.sourceInode, nonce), nil
}

func parseConditionalUnixMovePendingName(name string) (string, atomicUnixIdentity, bool) {
	const prefix = ".move-pending-v1-"
	if !strings.HasPrefix(name, prefix) {
		return "", atomicUnixIdentity{}, false
	}
	parts := strings.Split(strings.TrimPrefix(name, prefix), "-")
	if len(parts) != 4 || len(parts[0]) != sha256.Size*2 || len(parts[1]) != 16 || len(parts[2]) != 16 || len(parts[3]) != 32 {
		return "", atomicUnixIdentity{}, false
	}
	if _, err := hex.DecodeString(parts[0]); err != nil {
		return "", atomicUnixIdentity{}, false
	}
	device, deviceErr := strconv.ParseUint(parts[1], 16, 64)
	inode, inodeErr := strconv.ParseUint(parts[2], 16, 64)
	if _, err := hex.DecodeString(parts[3]); deviceErr != nil || inodeErr != nil || err != nil || device == 0 || inode == 0 {
		return "", atomicUnixIdentity{}, false
	}
	return parts[0], atomicUnixIdentity{dev: device, ino: inode}, true
}

func conditionalUnixMatchesMovePrivate(plan MoveRegularFilePlan, observation conditionalUnixObservation) bool {
	return conditionalUnixMatchesMoveContent(plan, observation) && observation.identity.dev == plan.sourceDevice && observation.identity.ino == plan.sourceInode
}

func conditionalUnixMatchesMoveInstalled(plan MoveRegularFilePlan, observation conditionalUnixObservation) bool {
	return conditionalUnixMatchesMovePrivate(plan, observation)
}

func conditionalUnixRevision(stat unix.Stat_t) string {
	return fmt.Sprintf("%d:%d:%d:%d:%d:%d:%d", stat.Dev, stat.Ino, stat.Size, stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec)
}

func conditionalDeleteExactPlatform(path, expectedHash string, authority ConditionalMutationAuthority, hooks *atomicTextTestHooks) error {
	parent, base, missing, err := openAtomicUnixParent(path, false)
	if err != nil {
		return err
	}
	if missing {
		return fmt.Errorf("%w: conditional delete target parent is absent", ErrAtomicTextBeforeDrift)
	}
	defer unix.Close(parent)
	journal, err := openConditionalUnixJournalDirectory(authority, path)
	if err != nil {
		return err
	}
	defer unix.Close(journal)
	if same, err := conditionalUnixSameDevice(parent, journal); err != nil || !same {
		return errors.Join(err, fmt.Errorf("%w: conditional delete journal must share the target filesystem", ErrAtomicTextUnsupportedPlatform))
	}

	initial, err := observeConditionalUnixAt(parent, base)
	if err != nil {
		return err
	}
	recovery, err := observeConditionalUnixRecovery(journal, expectedHash)
	if err != nil {
		return err
	}
	if !initial.exists {
		return recoverConditionalUnixCommittedDelete(parent, journal, recovery, expectedHash)
	}
	// A target that reappears after an exact pending/settled delete is an ABA
	// state, not a new authorization. Without a distinct host operation id the
	// old journal cannot authorize deleting the replacement, even when its bytes
	// happen to have the same hash.
	if len(recovery.pending) != 0 || len(recovery.settled) != 0 {
		return ErrConditionalMutationResidue
	}
	if initial.hash != expectedHash {
		return fmt.Errorf("%w: conditional delete content hash changed", ErrAtomicTextBeforeDrift)
	}
	if hooks != nil && hooks.AfterInitialValidation != nil {
		hooks.AfterInitialValidation()
	}
	current, err := observeConditionalUnixAt(parent, base)
	if err != nil {
		return err
	}
	if !sameConditionalUnixObservation(initial, current) || current.hash != expectedHash {
		return fmt.Errorf("%w: conditional delete identity changed", ErrAtomicTextBeforeDrift)
	}
	if err := atomicTextBeforeReplace(hooks); err != nil {
		return err
	}
	pending, err := conditionalUnixJournalName(".pending-v1-", current)
	if err != nil {
		return err
	}
	if err := renameConditionalUnixNoReplace(parent, base, journal, pending); err != nil {
		return err
	}
	rollback := func() error {
		if err := renameConditionalUnixNoReplace(journal, pending, parent, base); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		if err := syncConditionalUnixParents(parent, journal); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		restored, observeErr := observeConditionalUnixAt(parent, base)
		if observeErr != nil || !sameConditionalUnixObservation(current, restored) {
			return errors.Join(ErrConditionalMutationIndeterminate, observeErr, errors.New("conditional delete rollback verification failed"))
		}
		return nil
	}
	quarantined, err := observeConditionalUnixAt(journal, pending)
	if err != nil || !sameConditionalUnixObservation(current, quarantined) || quarantined.hash != expectedHash {
		return errors.Join(ErrAtomicTextBeforeDrift, err, rollback())
	}
	if hooks != nil && hooks.AfterDeleteQuarantine != nil {
		if hookErr := hooks.AfterDeleteQuarantine(); hookErr != nil {
			return errors.Join(hookErr, rollback())
		}
	}
	if err := syncConditionalUnixParents(parent, journal); err != nil {
		rollbackErr := rollback()
		if rollbackErr != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err, rollbackErr)
		}
		return err
	}
	settled := strings.Replace(pending, ".pending-v1-", ".settled-v1-", 1)
	if err := renameConditionalUnixNoReplace(journal, pending, journal, settled); err != nil {
		return errors.Join(ErrConditionalMutationResidue, errors.New("conditional delete committed with an unsettled recovery journal"), err)
	}
	if err := unix.Fsync(journal); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, errors.New("conditional delete settlement sync failed"), err)
	}
	settledObservation, err := observeConditionalUnixAt(journal, settled)
	if err != nil || !sameConditionalUnixObservation(current, settledObservation) {
		return errors.Join(ErrConditionalMutationResidue, err, errors.New("conditional delete settled journal readback failed"))
	}
	after, err := observeConditionalUnixAt(parent, base)
	if err != nil || after.exists {
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("conditional delete target reappeared after commit"))
	}
	return nil
}

type conditionalUnixRecoveryObservation struct {
	pending []conditionalUnixRecoveryEntry
	settled []conditionalUnixRecoveryEntry
}

func observeConditionalUnixRecovery(journal int, expectedHash string) (conditionalUnixRecoveryObservation, error) {
	duplicate, err := unix.Openat(journal, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return conditionalUnixRecoveryObservation{}, err
	}
	directory := os.NewFile(uintptr(duplicate), "conditional-file-mutation-journal")
	if directory == nil {
		_ = unix.Close(duplicate)
		return conditionalUnixRecoveryObservation{}, errors.New("conditional mutation journal handle is invalid")
	}
	defer directory.Close()
	names, err := directory.Readdirnames(conditionalUnixRecoveryEntryLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return conditionalUnixRecoveryObservation{}, err
	}
	if len(names) > conditionalUnixRecoveryEntryLimit {
		return conditionalUnixRecoveryObservation{}, errors.New("conditional mutation recovery entry bound exceeded")
	}
	result := conditionalUnixRecoveryObservation{}
	for _, name := range names {
		settled, digest, identity, ok := parseConditionalUnixJournalName(name)
		if !ok || digest != expectedHash {
			return conditionalUnixRecoveryObservation{}, ErrConditionalMutationResidue
		}
		observation, observeErr := observeConditionalUnixAt(journal, name)
		if observeErr != nil || !observation.exists || observation.hash != digest || observation.identity != identity {
			return conditionalUnixRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, observeErr)
		}
		entry := conditionalUnixRecoveryEntry{name: name, settled: settled, hash: digest, identity: identity, observation: observation}
		if settled {
			result.settled = append(result.settled, entry)
		} else {
			result.pending = append(result.pending, entry)
		}
	}
	return result, nil
}

func recoverConditionalUnixCommittedDelete(parent, journal int, recovery conditionalUnixRecoveryObservation, expectedHash string) error {
	if len(recovery.pending)+len(recovery.settled) > 1 {
		return ErrConditionalMutationResidue
	}
	if len(recovery.pending) == 1 {
		entry := recovery.pending[0]
		settled := strings.Replace(entry.name, ".pending-v1-", ".settled-v1-", 1)
		if err := renameConditionalUnixNoReplace(journal, entry.name, journal, settled); err != nil {
			return errors.Join(ErrConditionalMutationResidue, err)
		}
		if err := syncConditionalUnixParents(parent, journal); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		settledObservation, err := observeConditionalUnixAt(journal, settled)
		if err != nil || settledObservation.hash != expectedHash || settledObservation.identity != entry.identity {
			return errors.Join(ErrConditionalMutationResidue, err, errors.New("conditional delete recovery settlement readback failed"))
		}
		return nil
	}
	if len(recovery.settled) == 1 {
		if err := syncConditionalUnixParents(parent, journal); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		return nil
	}
	return fmt.Errorf("%w: conditional delete target is absent without an exact journal entry for %s", ErrAtomicTextBeforeDrift, expectedHash)
}

func openConditionalUnixJournalDirectory(authority ConditionalMutationAuthority, targetPath string) (int, error) {
	root := filepath.Clean(authority.root)
	targetPath = filepath.Clean(targetPath)
	if relative, err := filepath.Rel(root, targetPath); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return -1, fmt.Errorf("%w: mutation journal cannot contain its target", ErrAtomicTextUnsafePath)
	}
	rootFD, err := openConditionalUnixAuthorityRoot(root)
	if err != nil {
		return -1, err
	}
	var rootStat unix.Stat_t
	if err := unix.Fstat(rootFD, &rootStat); err != nil || uint64(rootStat.Dev) != authority.device || uint64(rootStat.Ino) != authority.inode {
		_ = unix.Close(rootFD)
		return -1, errors.Join(err, fmt.Errorf("%w: conditional mutation authority identity changed", ErrAtomicTextBeforeDrift))
	}
	digest := sha256.Sum256([]byte(targetPath))
	shard := hex.EncodeToString(digest[:1])
	target := hex.EncodeToString(digest[:])
	shardFD, err := openOrCreateConditionalUnixDirectory(rootFD, shard)
	_ = unix.Close(rootFD)
	if err != nil {
		return -1, err
	}
	targetFD, err := openOrCreateConditionalUnixDirectory(shardFD, target)
	_ = unix.Close(shardFD)
	return targetFD, err
}

func openConditionalUnixAuthorityRoot(root string) (int, error) {
	parent, base, missing, err := openAtomicUnixParent(root, true)
	if err != nil {
		return -1, err
	}
	if missing {
		return -1, os.ErrNotExist
	}
	defer unix.Close(parent)
	return openOrCreateConditionalUnixDirectory(parent, base)
}

func openOrCreateConditionalUnixDirectory(parent int, name string) (int, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		if err := unix.Mkdirat(parent, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
			return -1, err
		}
		if err := unix.Fsync(parent); err != nil {
			return -1, err
		}
		fd, err = unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return -1, fmt.Errorf("%w: open conditional mutation journal directory: %v", ErrAtomicTextUnsafePath, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		_ = unix.Close(fd)
		return -1, errors.Join(err, fmt.Errorf("%w: conditional mutation journal directory authority is unsafe", ErrAtomicTextUnsafePath))
	}
	return fd, nil
}

func observeConditionalUnixAt(parent int, base string) (conditionalUnixObservation, error) {
	fd, err := unix.Openat(parent, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return conditionalUnixObservation{}, nil
	}
	if err != nil {
		return conditionalUnixObservation{}, fmt.Errorf("%w: open conditional mutation target: %v", ErrAtomicTextUnsafePath, err)
	}
	file := os.NewFile(uintptr(fd), base)
	if file == nil {
		_ = unix.Close(fd)
		return conditionalUnixObservation{}, errors.New("conditional mutation target handle is invalid")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return conditionalUnixObservation{}, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG {
		return conditionalUnixObservation{}, fmt.Errorf("%w: conditional mutation target is not a regular file", ErrAtomicTextUnsafePath)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return conditionalUnixObservation{}, err
	}
	return finishConditionalUnixObservation(parent, base, fd, before, hasher)
}

func finishConditionalUnixObservation(parent int, base string, fd int, before unix.Stat_t, hasher hash.Hash) (conditionalUnixObservation, error) {
	var after unix.Stat_t
	var linked unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return conditionalUnixObservation{}, err
	}
	if err := unix.Fstatat(parent, base, &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return conditionalUnixObservation{}, fmt.Errorf("%w: conditional mutation target changed while read", ErrAtomicTextBeforeDrift)
	}
	if !sameConditionalUnixStableStat(before, after) || !sameConditionalUnixStableStat(after, linked) {
		return conditionalUnixObservation{}, fmt.Errorf("%w: conditional mutation target changed while read", ErrAtomicTextBeforeDrift)
	}
	return conditionalUnixObservation{
		exists: true, hash: hex.EncodeToString(hasher.Sum(nil)), size: after.Size,
		mode: os.FileMode(after.Mode & 0o777), identity: atomicUnixIdentity{dev: uint64(after.Dev), ino: uint64(after.Ino)}, stat: after,
	}, nil
}

func validateConditionalUnixMoveSource(plan MoveRegularFilePlan, observation conditionalUnixObservation) error {
	if !conditionalUnixMatchesMovePlan(plan, observation) {
		return fmt.Errorf("%w: conditional move source differs from its prepared authority", ErrAtomicTextBeforeDrift)
	}
	return nil
}

func conditionalUnixMatchesMovePlan(plan MoveRegularFilePlan, observation conditionalUnixObservation) bool {
	return conditionalUnixMatchesMoveContent(plan, observation) &&
		observation.identity.dev == plan.sourceDevice && observation.identity.ino == plan.sourceInode &&
		conditionalUnixRevision(observation.stat) == plan.sourceRevision
}

func conditionalUnixMatchesMoveContent(plan MoveRegularFilePlan, observation conditionalUnixObservation) bool {
	return observation.exists && observation.hash == plan.sourceHash && observation.size == plan.BytesMoved &&
		observation.mode.Perm() == plan.sourceMode.Perm()
}

func sameConditionalUnixObservation(left, right conditionalUnixObservation) bool {
	return left.exists == right.exists && (!left.exists || left.identity == right.identity && left.hash == right.hash && left.size == right.size && left.mode.Perm() == right.mode.Perm())
}

func conditionalUnixSameDevice(left, right int) (bool, error) {
	var leftStat unix.Stat_t
	var rightStat unix.Stat_t
	if err := unix.Fstat(left, &leftStat); err != nil {
		return false, err
	}
	if err := unix.Fstat(right, &rightStat); err != nil {
		return false, err
	}
	return leftStat.Dev == rightStat.Dev, nil
}

func syncConditionalUnixParents(left, right int) error {
	if err := unix.Fsync(left); err != nil {
		return err
	}
	var leftStat unix.Stat_t
	var rightStat unix.Stat_t
	if unix.Fstat(left, &leftStat) == nil && unix.Fstat(right, &rightStat) == nil && leftStat.Dev == rightStat.Dev && leftStat.Ino == rightStat.Ino {
		return nil
	}
	return unix.Fsync(right)
}

func conditionalUnixRandomName(prefix string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(nonce), nil
}

func conditionalUnixJournalName(prefix string, observation conditionalUnixObservation) (string, error) {
	nonce, err := conditionalUnixRandomName("")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s-%016x-%016x-%s", prefix, observation.hash, observation.identity.dev, observation.identity.ino, nonce), nil
}

func parseConditionalUnixJournalName(name string) (bool, string, atomicUnixIdentity, bool) {
	settled := false
	prefix := ".pending-v1-"
	if strings.HasPrefix(name, ".settled-v1-") {
		settled = true
		prefix = ".settled-v1-"
	}
	if !strings.HasPrefix(name, prefix) {
		return false, "", atomicUnixIdentity{}, false
	}
	parts := strings.Split(strings.TrimPrefix(name, prefix), "-")
	if len(parts) != 4 || len(parts[0]) != sha256.Size*2 || len(parts[1]) != 16 || len(parts[2]) != 16 || len(parts[3]) != 32 {
		return false, "", atomicUnixIdentity{}, false
	}
	if _, err := hex.DecodeString(parts[0]); err != nil {
		return false, "", atomicUnixIdentity{}, false
	}
	dev, devErr := strconv.ParseUint(parts[1], 16, 64)
	ino, inoErr := strconv.ParseUint(parts[2], 16, 64)
	if _, err := hex.DecodeString(parts[3]); devErr != nil || inoErr != nil || err != nil || dev == 0 || ino == 0 {
		return false, "", atomicUnixIdentity{}, false
	}
	return settled, parts[0], atomicUnixIdentity{dev: dev, ino: ino}, true
}
