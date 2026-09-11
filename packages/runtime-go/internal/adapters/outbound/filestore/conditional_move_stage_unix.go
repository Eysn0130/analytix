//go:build darwin || linux

package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	"golang.org/x/sys/unix"
)

func observeConditionalMoveStagedRecovery(
	ctx context.Context,
	observer CheckpointOperationObserver,
	intent domaincheckpoint.OperationGroupIntentV2,
	sourceIntent domaincheckpoint.OperationPathV2,
	destinationIntent domaincheckpoint.OperationPathV2,
	plan MoveRegularFilePlan,
	journal int,
	pending []conditionalUnixRecoveryEntry,
	publicState conditionalMoveRecoveryState,
) (conditionalMoveRecoveryObservation, error) {
	receipt, found, err := readConditionalUnixMoveStageAuthority(journal)
	if err != nil {
		return conditionalMoveRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, err)
	}
	if !found {
		var linked unix.Stat_t
		stageErr := unix.Fstatat(journal, plan.destinationStageName, &linked, unix.AT_SYMLINK_NOFOLLOW)
		if stageErr == nil || !errors.Is(stageErr, unix.ENOENT) || len(pending) != 0 {
			return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
		}
		return conditionalMoveRecoveryObservation{
			state: publicState, journalPresent: true,
			bindingDigest: conditionalUnixMoveBindingDigestForPlan(plan), plan: plan,
		}, nil
	}
	if !validConditionalUnixMoveStageAuthority(receipt, plan) {
		return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
	}
	stage, err := openExistingConditionalUnixDirectory(journal, plan.destinationStageName)
	if errors.Is(err, unix.ENOENT) {
		if len(pending) != 0 {
			return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
		}
		if publicState == conditionalMoveRecoveryNoEffect {
			present, residueErr := conditionalUnixMoveStageResiduePresent(observer.MutationAuthority, plan, receipt)
			if residueErr != nil || !present {
				return conditionalMoveRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, residueErr)
			}
			return conditionalMoveRecoveryObservation{
				state: conditionalMoveRecoveryCleanupStage, journalPresent: true,
				bindingDigest: conditionalUnixMoveBindingDigestForPlan(plan), stageReceiptDigest: receipt.ReceiptDigest, plan: plan,
			}, nil
		}
		if publicState != conditionalMoveRecoveryCompleted {
			return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
		}
		publicParent, parentErr := openConditionalUnixFrozenDestinationParent(plan)
		if parentErr != nil {
			return conditionalMoveRecoveryObservation{}, parentErr
		}
		publicTop, topErr := observeConditionalUnixDirectoryAt(publicParent, plan.destinationTopMissing)
		_ = unix.Close(publicParent)
		if topErr != nil || !publicTop.exists || publicTop.identity != (atomicUnixIdentity{dev: receipt.StageDevice, ino: receipt.StageInode}) {
			publicTop.close()
			return conditionalMoveRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, topErr)
		}
		if treeErr := validateConditionalUnixMoveStageTree(publicTop.fd, plan, true); treeErr != nil {
			publicTop.close()
			return conditionalMoveRecoveryObservation{}, treeErr
		}
		publicTop.close()
		return conditionalMoveRecoveryObservation{
			state: conditionalMoveRecoveryCompleted, journalPresent: true,
			bindingDigest: conditionalUnixMoveBindingDigestForPlan(plan), stageReceiptDigest: receipt.ReceiptDigest, plan: plan,
		}, nil
	}
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	defer unix.Close(stage)
	var stageStat unix.Stat_t
	if err := unix.Fstat(stage, &stageStat); err != nil || uint64(stageStat.Dev) != receipt.StageDevice || uint64(stageStat.Ino) != receipt.StageInode {
		return conditionalMoveRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, err)
	}
	publicParent, err := openConditionalUnixFrozenDestinationParent(plan)
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	publicTop, topErr := observeConditionalUnixDirectoryAt(publicParent, plan.destinationTopMissing)
	_ = unix.Close(publicParent)
	if topErr != nil || publicTop.exists {
		publicTop.close()
		return conditionalMoveRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, topErr)
	}
	publicTop.close()
	leafParent, leafBase, leafParentErr := openConditionalUnixMoveStageLeafParent(stage, plan.destinationRelativeTail, false)
	leaf := conditionalUnixObservation{}
	if leafParentErr == nil {
		leaf, err = observeConditionalUnixAt(leafParent, leafBase)
		_ = unix.Close(leafParent)
		if err != nil {
			return conditionalMoveRecoveryObservation{}, err
		}
	} else if !errors.Is(leafParentErr, unix.ENOENT) {
		return conditionalMoveRecoveryObservation{}, leafParentErr
	}
	base := conditionalMoveRecoveryObservation{
		journalPresent: true, bindingDigest: conditionalUnixMoveBindingDigestForPlan(plan),
		stageReceiptDigest: receipt.ReceiptDigest, plan: plan,
	}
	if publicState == conditionalMoveRecoveryNoEffect && len(pending) == 0 && !leaf.exists {
		base.state = conditionalMoveRecoveryCleanupStage
		return base, nil
	}
	publicSourceAbsent := conditionalMoveRecoveryPathAbsent(ctx, observer, intent, sourceIntent)
	publicDestinationAbsent := conditionalMoveRecoveryPathAbsent(ctx, observer, intent, destinationIntent)
	if !publicSourceAbsent || !publicDestinationAbsent {
		return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
	}
	if len(pending) == 1 && !leaf.exists {
		base.state = conditionalMoveRecoveryRollback
		base.pendingName = pending[0].name
		return base, nil
	}
	if len(pending) == 0 && conditionalUnixMatchesMoveInstalled(plan, leaf) {
		base.state = conditionalMoveRecoveryRollbackStage
		return base, nil
	}
	return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
}

func applyConditionalMoveStagedRecovery(
	observer CheckpointOperationObserver,
	observation conditionalMoveRecoveryObservation,
) error {
	plan := observation.plan
	if observation.state == conditionalMoveRecoveryRollback {
		if err := applyConditionalMovePendingRollback(observer, observation); err != nil {
			return err
		}
	} else if observation.state == conditionalMoveRecoveryRollbackStage {
		if err := rollbackConditionalUnixMoveStageLeaf(observer, observation); err != nil {
			return err
		}
	}
	return quarantineConditionalUnixMoveStage(observer.MutationAuthority, plan, observation.stageReceiptDigest)
}

func rollbackConditionalUnixMoveStageLeaf(
	observer CheckpointOperationObserver,
	observation conditionalMoveRecoveryObservation,
) error {
	plan := observation.plan
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(observer.MutationAuthority)
	if err != nil {
		return err
	}
	defer unix.Close(authorityRoot)
	journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, false)
	if err != nil {
		return err
	}
	defer unix.Close(journal)
	receipt, found, err := readConditionalUnixMoveStageAuthority(journal)
	if err != nil || !found || receipt.ReceiptDigest != observation.stageReceiptDigest || !validConditionalUnixMoveStageAuthority(receipt, plan) {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	stage, err := openExistingConditionalUnixDirectory(journal, plan.destinationStageName)
	if err != nil {
		return err
	}
	defer unix.Close(stage)
	var stageStat unix.Stat_t
	if err := unix.Fstat(stage, &stageStat); err != nil || uint64(stageStat.Dev) != receipt.StageDevice || uint64(stageStat.Ino) != receipt.StageInode {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	leafParent, leafBase, err := openConditionalUnixMoveStageLeafParent(stage, plan.destinationRelativeTail, false)
	if err != nil {
		return err
	}
	defer unix.Close(leafParent)
	leaf, err := observeConditionalUnixAt(leafParent, leafBase)
	if err != nil || !conditionalUnixMatchesMoveInstalled(plan, leaf) {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	sourceParent, sourceBase, missing, err := openAtomicUnixParent(plan.SourcePath, false)
	if err != nil || missing {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	defer unix.Close(sourceParent)
	source, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || source.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if err := renameConditionalUnixNoReplace(leafParent, leafBase, sourceParent, sourceBase); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	if err := syncConditionalUnixParents(leafParent, sourceParent); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	restored, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || !conditionalUnixMatchesMoveInstalled(plan, restored) || restored.identity != leaf.identity {
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("conditional move stage rollback readback failed"))
	}
	return nil
}

func quarantineConditionalUnixMoveStage(
	authority ConditionalMutationAuthority,
	plan MoveRegularFilePlan,
	receiptDigest string,
) error {
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(authority)
	if err != nil {
		return err
	}
	defer unix.Close(authorityRoot)
	journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, false)
	if err != nil {
		return err
	}
	defer unix.Close(journal)
	receipt, found, err := readConditionalUnixMoveStageAuthority(journal)
	if err != nil || !found || receipt.ReceiptDigest != receiptDigest || !validConditionalUnixMoveStageAuthority(receipt, plan) {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	residues, err := openOrCreateConditionalUnixDirectory(authorityRoot, "move-stage-residues-v1")
	if err != nil {
		return err
	}
	defer unix.Close(residues)
	residueName := conditionalUnixMoveStageResidueName(plan)
	stage, err := openExistingConditionalUnixDirectory(journal, plan.destinationStageName)
	if errors.Is(err, unix.ENOENT) {
		quarantined, residueErr := observeConditionalUnixDirectoryAt(residues, residueName)
		if residueErr != nil || !quarantined.exists || quarantined.identity != (atomicUnixIdentity{dev: receipt.StageDevice, ino: receipt.StageInode}) {
			quarantined.close()
			return errors.Join(ErrConditionalMutationResidue, residueErr)
		}
		quarantined.close()
		return syncConditionalUnixParents(journal, residues)
	}
	if err != nil {
		return err
	}
	var stat unix.Stat_t
	statErr := unix.Fstat(stage, &stat)
	_ = unix.Close(stage)
	if statErr != nil || uint64(stat.Dev) != receipt.StageDevice || uint64(stat.Ino) != receipt.StageInode {
		return errors.Join(ErrConditionalMutationResidue, statErr)
	}
	if err := renameConditionalUnixNoReplace(journal, plan.destinationStageName, residues, residueName); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	if err := syncConditionalUnixParents(journal, residues); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	quarantined, err := observeConditionalUnixDirectoryAt(residues, residueName)
	if err != nil || !quarantined.exists || quarantined.identity != (atomicUnixIdentity{dev: receipt.StageDevice, ino: receipt.StageInode}) {
		quarantined.close()
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	quarantined.close()
	return nil
}

func conditionalUnixMoveStageResidueName(plan MoveRegularFilePlan) string {
	return ".move-stage-residue-v1-" + plan.operationGroupID
}

func conditionalUnixMoveStageResiduePresent(
	authority ConditionalMutationAuthority,
	plan MoveRegularFilePlan,
	receipt conditionalUnixMoveStageAuthority,
) (bool, error) {
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(authority)
	if err != nil {
		return false, err
	}
	defer unix.Close(authorityRoot)
	residues, err := openExistingConditionalUnixDirectory(authorityRoot, "move-stage-residues-v1")
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer unix.Close(residues)
	entry, err := observeConditionalUnixDirectoryAt(residues, conditionalUnixMoveStageResidueName(plan))
	if err != nil {
		return false, err
	}
	defer entry.close()
	return entry.exists && entry.identity == (atomicUnixIdentity{dev: receipt.StageDevice, ino: receipt.StageInode}), nil
}

func applyConditionalUnixStagedDirectoryMove(
	plan MoveRegularFilePlan,
	sourceParent int,
	sourceBase string,
	authorityRoot int,
	journal int,
	pending []conditionalUnixRecoveryEntry,
) error {
	stage, stageAuthority, err := ensureConditionalUnixMoveStage(journal, plan)
	if err != nil {
		return err
	}
	if stage >= 0 {
		defer unix.Close(stage)
	}
	publicParent, err := openConditionalUnixFrozenDestinationParent(plan)
	if err != nil {
		return err
	}
	defer unix.Close(publicParent)
	publicTop, err := observeConditionalUnixDirectoryAt(publicParent, plan.destinationTopMissing)
	if err != nil {
		return err
	}
	if stage < 0 {
		if !publicTop.exists || publicTop.identity != (atomicUnixIdentity{dev: stageAuthority.StageDevice, ino: stageAuthority.StageInode}) {
			return ErrConditionalMutationResidue
		}
		if err := validateConditionalUnixMoveStageTree(publicTop.fd, plan, true); err != nil {
			publicTop.close()
			return err
		}
		publicTop.close()
		source, sourceErr := observeConditionalUnixAt(sourceParent, sourceBase)
		if sourceErr != nil || source.exists || len(pending) != 0 {
			return errors.Join(ErrConditionalMutationResidue, sourceErr)
		}
		return syncConditionalUnixParents(publicParent, journal)
	}
	defer publicTop.close()
	if publicTop.exists {
		return ErrConditionalMutationResidue
	}
	leafParent, leafBase, err := openConditionalUnixMoveStageLeafParent(stage, plan.destinationRelativeTail, true)
	if err != nil {
		return err
	}
	defer unix.Close(leafParent)
	leaf, err := observeConditionalUnixAt(leafParent, leafBase)
	if err != nil {
		return err
	}
	source, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil {
		return err
	}
	if source.exists {
		if len(pending) != 0 || leaf.exists || validateConditionalUnixMoveSource(plan, source) != nil {
			return ErrConditionalMutationResidue
		}
		if err := validateConditionalUnixMoveStageTree(stage, plan, false); err != nil {
			return err
		}
		if plan.testHooks != nil && plan.testHooks.AfterInitialValidation != nil {
			plan.testHooks.AfterInitialValidation()
		}
		current, currentErr := observeConditionalUnixAt(sourceParent, sourceBase)
		if currentErr != nil || validateConditionalUnixMoveSource(plan, current) != nil || current.identity != source.identity {
			return errors.Join(ErrAtomicTextBeforeDrift, currentErr)
		}
		if plan.testHooks != nil && plan.testHooks.BeforeRename != nil {
			plan.testHooks.BeforeRename()
		}
		pendingName, nameErr := conditionalUnixMovePendingName(plan)
		if nameErr != nil {
			return nameErr
		}
		if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, pendingName); err != nil {
			return err
		}
		if plan.testHooks != nil && plan.testHooks.AfterSourceRenameBeforeVerify != nil {
			plan.testHooks.AfterSourceRenameBeforeVerify()
		}
		private, privateErr := observeConditionalUnixAt(journal, pendingName)
		if privateErr != nil || !conditionalUnixMatchesMovePrivate(plan, private) || private.identity != source.identity {
			rollbackErr := renameConditionalUnixNoReplace(journal, pendingName, sourceParent, sourceBase)
			return errors.Join(ErrConditionalMutationIndeterminate, privateErr, rollbackErr)
		}
		if plan.testHooks != nil && plan.testHooks.AfterSourceQuarantine != nil {
			plan.testHooks.AfterSourceQuarantine()
		}
		sourceAfter, sourceErr := observeConditionalUnixAt(sourceParent, sourceBase)
		if sourceErr != nil || sourceAfter.exists {
			return errors.Join(ErrConditionalMutationResidue, sourceErr)
		}
		source = conditionalUnixObservation{}
		if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		pending = []conditionalUnixRecoveryEntry{{name: pendingName, identity: private.identity, observation: private}}
	}
	if leaf.exists {
		if len(pending) != 0 || source.exists || !conditionalUnixMatchesMoveInstalled(plan, leaf) {
			return ErrConditionalMutationResidue
		}
	} else {
		if len(pending) != 1 || source.exists || !conditionalUnixMatchesMovePrivate(plan, pending[0].observation) {
			return ErrConditionalMutationResidue
		}
		if err := renameConditionalUnixNoReplace(journal, pending[0].name, leafParent, leafBase); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		if err := syncConditionalUnixParents(journal, leafParent); err != nil {
			return errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		leaf, err = observeConditionalUnixAt(leafParent, leafBase)
		if err != nil || !conditionalUnixMatchesMoveInstalled(plan, leaf) || leaf.identity != pending[0].identity {
			return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("private move stage leaf readback failed"))
		}
		pending = nil
	}
	if err := fsyncConditionalUnixRegularAt(leafParent, leafBase); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	if err := validateConditionalUnixMoveStageTree(stage, plan, true); err != nil {
		return err
	}
	if err := unix.Fsync(stage); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	if plan.testHooks != nil && plan.testHooks.BeforeDestinationInstall != nil {
		plan.testHooks.BeforeDestinationInstall()
	}
	source, err = observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || source.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	publicTop.close()
	publicTop, err = observeConditionalUnixDirectoryAt(publicParent, plan.destinationTopMissing)
	if err != nil || publicTop.exists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if err := validateConditionalUnixMoveStageTree(stage, plan, true); err != nil {
		return err
	}
	if err := renameConditionalUnixNoReplace(journal, plan.destinationStageName, publicParent, plan.destinationTopMissing); err != nil {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if err := syncConditionalUnixParents(journal, publicParent); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	installed, err := observeConditionalUnixDirectoryAt(publicParent, plan.destinationTopMissing)
	if err != nil || !installed.exists || installed.identity != (atomicUnixIdentity{dev: stageAuthority.StageDevice, ino: stageAuthority.StageInode}) {
		installed.close()
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("published move stage identity readback failed"))
	}
	if err := validateConditionalUnixMoveStageTree(installed.fd, plan, true); err != nil {
		installed.close()
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	installed.close()
	if plan.testHooks != nil && plan.testHooks.AfterRename != nil {
		if hookErr := plan.testHooks.AfterRename(); hookErr != nil {
			return errors.Join(ErrConditionalMutationResidue, hookErr)
		}
	}
	return nil
}

type conditionalUnixDirectoryObservation struct {
	exists   bool
	fd       int
	identity atomicUnixIdentity
}

func (observation *conditionalUnixDirectoryObservation) close() {
	if observation != nil && observation.fd >= 0 {
		_ = unix.Close(observation.fd)
		observation.fd = -1
	}
}

func observeConditionalUnixDirectoryAt(parent int, name string) (conditionalUnixDirectoryObservation, error) {
	result := conditionalUnixDirectoryObservation{fd: -1}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(fd)
		return result, errors.Join(err, errors.New("conditional move directory is unsafe"))
	}
	result.exists = true
	result.fd = fd
	result.identity = atomicUnixIdentity{dev: uint64(stat.Dev), ino: uint64(stat.Ino)}
	return result, nil
}

func openConditionalUnixFrozenDestinationParent(plan MoveRegularFilePlan) (int, error) {
	parent, _, missing, err := openAtomicUnixParent(filepath.Join(plan.destinationExistingParent, ".analytix-move-parent-probe"), false)
	if err != nil || missing {
		return -1, errors.Join(ErrConditionalMutationResidue, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(parent, &stat); err != nil || uint64(stat.Dev) != plan.destinationExistingDevice ||
		uint64(stat.Ino) != plan.destinationExistingInode || os.FileMode(stat.Mode&0o777) != plan.destinationExistingMode.Perm() {
		_ = unix.Close(parent)
		return -1, errors.Join(ErrConditionalMutationResidue, err, errors.New("destination existing parent authority changed"))
	}
	return parent, nil
}

func ensureConditionalUnixMoveStage(journal int, plan MoveRegularFilePlan) (int, conditionalUnixMoveStageAuthority, error) {
	receipt, found, err := readConditionalUnixMoveStageAuthority(journal)
	if err != nil {
		return -1, conditionalUnixMoveStageAuthority{}, errors.Join(ErrConditionalMutationResidue, err)
	}
	if !found {
		if err := unix.Mkdirat(journal, plan.destinationStageName, 0o700); err != nil {
			return -1, conditionalUnixMoveStageAuthority{}, err
		}
		if err := unix.Fsync(journal); err != nil {
			return -1, conditionalUnixMoveStageAuthority{}, errors.Join(ErrConditionalMutationIndeterminate, err)
		}
		stage, err := openExistingConditionalUnixDirectory(journal, plan.destinationStageName)
		if err != nil {
			return -1, conditionalUnixMoveStageAuthority{}, err
		}
		var stat unix.Stat_t
		if err := unix.Fstat(stage, &stat); err != nil {
			_ = unix.Close(stage)
			return -1, conditionalUnixMoveStageAuthority{}, err
		}
		receipt = conditionalUnixMoveStageAuthority{
			SchemaVersion: 1, Purpose: conditionalUnixMoveStagePurpose,
			OperationGroupID: plan.operationGroupID, OperationIntentDigest: plan.operationDigest,
			BindingDigest: conditionalUnixMoveBindingDigestForPlan(plan), StageName: plan.destinationStageName,
			StageDevice: uint64(stat.Dev), StageInode: uint64(stat.Ino), StageMode: uint32(stat.Mode & 0o777),
			ExistingParentDevice: plan.destinationExistingDevice, ExistingParentInode: plan.destinationExistingInode,
			TopMissing: plan.destinationTopMissing, RelativeTail: plan.destinationRelativeTail,
		}
		receipt.ReceiptDigest = conditionalUnixMoveStageDigest(receipt)
		if !validConditionalUnixMoveStageAuthority(receipt, plan) {
			_ = unix.Close(stage)
			return -1, conditionalUnixMoveStageAuthority{}, errors.New("conditional move stage authority is invalid")
		}
		if err := writeConditionalUnixMoveStageAuthority(journal, receipt); err != nil {
			_ = unix.Close(stage)
			return -1, conditionalUnixMoveStageAuthority{}, err
		}
		return stage, receipt, nil
	}
	if !validConditionalUnixMoveStageAuthority(receipt, plan) {
		return -1, conditionalUnixMoveStageAuthority{}, ErrConditionalMutationResidue
	}
	stage, err := openExistingConditionalUnixDirectory(journal, receipt.StageName)
	if errors.Is(err, unix.ENOENT) {
		return -1, receipt, nil
	}
	if err != nil {
		return -1, conditionalUnixMoveStageAuthority{}, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(stage, &stat); err != nil || uint64(stat.Dev) != receipt.StageDevice || uint64(stat.Ino) != receipt.StageInode || uint32(stat.Mode&0o777) != receipt.StageMode {
		_ = unix.Close(stage)
		return -1, conditionalUnixMoveStageAuthority{}, errors.Join(ErrConditionalMutationResidue, err)
	}
	return stage, receipt, nil
}

func conditionalUnixMoveBindingDigestForPlan(plan MoveRegularFilePlan) string {
	record, _, err := conditionalUnixMoveIntentForPlan(plan)
	if err != nil {
		return ""
	}
	return record.BindingDigest
}

func conditionalUnixMoveStageDigest(receipt conditionalUnixMoveStageAuthority) string {
	receipt.ReceiptDigest = ""
	body, _ := json.Marshal(receipt)
	return digestAtomicText(body)
}

func validConditionalUnixMoveStageAuthority(receipt conditionalUnixMoveStageAuthority, plan MoveRegularFilePlan) bool {
	return receipt.SchemaVersion == 1 && receipt.Purpose == conditionalUnixMoveStagePurpose &&
		receipt.OperationGroupID == plan.operationGroupID && receipt.OperationIntentDigest == plan.operationDigest &&
		receipt.BindingDigest == conditionalUnixMoveBindingDigestForPlan(plan) && receipt.StageName == plan.destinationStageName &&
		receipt.StageDevice != 0 && receipt.StageInode != 0 && receipt.StageMode == 0o700 &&
		receipt.ExistingParentDevice == plan.destinationExistingDevice && receipt.ExistingParentInode == plan.destinationExistingInode &&
		receipt.TopMissing == plan.destinationTopMissing && receipt.RelativeTail == plan.destinationRelativeTail &&
		conditionalUnixSHA256(receipt.ReceiptDigest) && receipt.ReceiptDigest == conditionalUnixMoveStageDigest(receipt)
}

func writeConditionalUnixMoveStageAuthority(journal int, receipt conditionalUnixMoveStageAuthority) error {
	body, err := json.Marshal(receipt)
	if err != nil || len(body) == 0 || len(body) > conditionalUnixMoveStageMaxBytes {
		return errors.Join(err, errors.New("conditional move stage authority body is invalid"))
	}
	fd, err := unix.Openat(journal, conditionalUnixMoveStageFile, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), conditionalUnixMoveStageFile)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("conditional move stage authority handle is invalid")
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, writeErr, syncErr, closeErr)
	}
	if err := unix.Fsync(journal); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	readback, found, err := readConditionalUnixMoveStageAuthority(journal)
	if err != nil || !found || readback != receipt {
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("conditional move stage authority readback failed"))
	}
	return nil
}

func readConditionalUnixMoveStageAuthority(journal int) (conditionalUnixMoveStageAuthority, bool, error) {
	fd, err := unix.Openat(journal, conditionalUnixMoveStageFile, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return conditionalUnixMoveStageAuthority{}, false, nil
	}
	if err != nil {
		return conditionalUnixMoveStageAuthority{}, false, err
	}
	file := os.NewFile(uintptr(fd), conditionalUnixMoveStageFile)
	if file == nil {
		_ = unix.Close(fd)
		return conditionalUnixMoveStageAuthority{}, false, errors.New("conditional move stage authority handle is invalid")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 ||
		before.Uid != uint32(os.Geteuid()) || before.Mode&0o077 != 0 || before.Size <= 0 || before.Size > conditionalUnixMoveStageMaxBytes {
		return conditionalUnixMoveStageAuthority{}, false, errors.Join(err, errors.New("conditional move stage authority file is unsafe"))
	}
	body, readErr := io.ReadAll(io.LimitReader(file, conditionalUnixMoveStageMaxBytes+1))
	var after unix.Stat_t
	var linked unix.Stat_t
	statErr := unix.Fstat(fd, &after)
	linkErr := unix.Fstatat(journal, conditionalUnixMoveStageFile, &linked, unix.AT_SYMLINK_NOFOLLOW)
	if readErr != nil || statErr != nil || linkErr != nil || int64(len(body)) != after.Size ||
		!sameConditionalUnixStableStat(before, after) || !sameConditionalUnixStableStat(after, linked) {
		return conditionalUnixMoveStageAuthority{}, false, errors.Join(readErr, statErr, linkErr)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var receipt conditionalUnixMoveStageAuthority
	if err := decoder.Decode(&receipt); err != nil {
		return conditionalUnixMoveStageAuthority{}, false, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return conditionalUnixMoveStageAuthority{}, false, errors.New("conditional move stage authority contains trailing JSON")
	}
	return receipt, true, nil
}

func openConditionalUnixMoveStageLeafParent(stage int, tail string, create bool) (int, string, error) {
	components := strings.Split(filepath.FromSlash(tail), string(filepath.Separator))
	if len(components) == 0 || !conditionalUnixSafeComponent(components[len(components)-1]) {
		return -1, "", errors.New("conditional move stage tail is invalid")
	}
	current, err := unix.Dup(stage)
	if err != nil {
		return -1, "", err
	}
	for _, component := range components[:len(components)-1] {
		if !conditionalUnixSafeComponent(component) {
			_ = unix.Close(current)
			return -1, "", errors.New("conditional move stage component is invalid")
		}
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) && create {
			if err := unix.Mkdirat(current, component, 0o700); err != nil {
				_ = unix.Close(current)
				return -1, "", err
			}
			if err := unix.Fsync(current); err != nil {
				_ = unix.Close(current)
				return -1, "", errors.Join(ErrConditionalMutationIndeterminate, err)
			}
			next, openErr = unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, "", openErr
		}
		var stat unix.Stat_t
		if err := unix.Fstat(next, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
			_ = unix.Close(next)
			_ = unix.Close(current)
			return -1, "", errors.Join(err, errors.New("conditional move stage directory authority is unsafe"))
		}
		_ = unix.Close(current)
		current = next
	}
	return current, components[len(components)-1], nil
}

func validateConditionalUnixMoveStageTree(stage int, plan MoveRegularFilePlan, requireLeaf bool) error {
	components := strings.Split(filepath.FromSlash(plan.destinationRelativeTail), string(filepath.Separator))
	current, err := unix.Dup(stage)
	if err != nil {
		return err
	}
	for index, component := range components {
		names, err := conditionalUnixDirectoryNames(current, 2)
		if err != nil {
			_ = unix.Close(current)
			return err
		}
		last := index == len(components)-1
		if last && !requireLeaf && len(names) == 0 {
			_ = unix.Close(current)
			return nil
		}
		if len(names) != 1 || names[0] != component {
			_ = unix.Close(current)
			return ErrConditionalMutationResidue
		}
		if last {
			leaf, err := observeConditionalUnixAt(current, component)
			_ = unix.Close(current)
			if err != nil || !conditionalUnixMatchesMoveInstalled(plan, leaf) {
				return errors.Join(ErrConditionalMutationResidue, err)
			}
			return nil
		}
		next, err := openExistingConditionalUnixDirectory(current, component)
		if err != nil {
			_ = unix.Close(current)
			return err
		}
		_ = unix.Close(current)
		current = next
	}
	_ = unix.Close(current)
	return ErrConditionalMutationResidue
}

func conditionalUnixDirectoryNames(directory int, limit int) ([]string, error) {
	duplicate, err := unix.Openat(directory, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "conditional-move-directory")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, errors.New("conditional move directory handle is invalid")
	}
	defer file.Close()
	names, err := file.Readdirnames(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > limit {
		return nil, ErrConditionalMutationResidue
	}
	sort.Strings(names)
	return names, nil
}

func fsyncConditionalUnixRegularAt(parent int, name string) error {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}
