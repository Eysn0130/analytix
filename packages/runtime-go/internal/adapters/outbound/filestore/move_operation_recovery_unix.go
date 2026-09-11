//go:build darwin || linux

package filestore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
	"golang.org/x/sys/unix"
)

type conditionalMoveRecoveryState string

const (
	conditionalMoveRecoveryNoEffect      conditionalMoveRecoveryState = "no_effect"
	conditionalMoveRecoveryCompleted     conditionalMoveRecoveryState = "completed"
	conditionalMoveRecoveryRollback      conditionalMoveRecoveryState = "rollback_private_pending"
	conditionalMoveRecoveryRollbackStage conditionalMoveRecoveryState = "rollback_private_stage"
	conditionalMoveRecoveryCleanupStage  conditionalMoveRecoveryState = "cleanup_private_stage"
	conditionalMoveRecoveryQuarantined   conditionalMoveRecoveryState = "quarantined"
)

type conditionalMoveRecoveryObservation struct {
	state              conditionalMoveRecoveryState
	journalPresent     bool
	bindingDigest      string
	pendingName        string
	stageReceiptDigest string
	plan               MoveRegularFilePlan
}

type preparedConditionalMoveOperationRecovery struct {
	observer    CheckpointOperationObserver
	intent      domaincheckpoint.OperationGroupIntentV2
	observation conditionalMoveRecoveryObservation
}

func prepareConditionalMoveOperationRecoveryPlatform(
	ctx context.Context,
	observer CheckpointOperationObserver,
	intent domaincheckpoint.OperationGroupIntentV2,
) (checkpointfileport.PreparedOperationRecovery, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	observation, err := observeConditionalMoveOperationRecovery(ctx, observer, intent)
	if err != nil {
		return nil, err
	}
	return &preparedConditionalMoveOperationRecovery{observer: observer, intent: intent, observation: observation}, nil
}

func (prepared *preparedConditionalMoveOperationRecovery) Apply(ctx context.Context) error {
	if prepared == nil {
		return errors.New("prepared conditional move recovery is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := observeConditionalMoveOperationRecovery(ctx, prepared.observer, prepared.intent)
	if err != nil {
		return err
	}
	if !sameConditionalMoveRecoveryObservation(prepared.observation, current) {
		return errors.New("conditional move recovery state changed after semantic planning")
	}
	if current.state == conditionalMoveRecoveryRollbackStage || current.state == conditionalMoveRecoveryCleanupStage ||
		current.state == conditionalMoveRecoveryRollback && current.plan.destinationParentMissing {
		return applyConditionalMoveStagedRecovery(prepared.observer, current)
	}
	if current.state != conditionalMoveRecoveryRollback {
		return nil
	}
	return applyConditionalMovePendingRollback(prepared.observer, current)
}

func observeConditionalMoveOperationRecovery(
	ctx context.Context,
	observer CheckpointOperationObserver,
	intent domaincheckpoint.OperationGroupIntentV2,
) (conditionalMoveRecoveryObservation, error) {
	if err := ctx.Err(); err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	if domaincheckpoint.ValidateOperationGroupIntentV2(intent) != nil || intent.ToolName != "move_file" || !observer.MutationAuthority.Available() {
		return conditionalMoveRecoveryObservation{}, errors.New("conditional move recovery durable authority is invalid")
	}
	sourcePath, destinationPath, sourceIntent, destinationIntent, err := observer.resolveMoveOperationIntent(intent)
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	minimalPlan := MoveRegularFilePlan{operationGroupID: intent.OperationGroupID}
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(observer.MutationAuthority)
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	defer unix.Close(authorityRoot)
	journal, err := openConditionalUnixMoveJournal(authorityRoot, minimalPlan, false)
	if errors.Is(err, unix.ENOENT) {
		state := classifyConditionalMoveRecoveryPublicState(ctx, observer, intent, sourceIntent, destinationIntent)
		return conditionalMoveRecoveryObservation{state: state}, nil
	}
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	defer unix.Close(journal)
	binding, found, err := readConditionalUnixMoveIntent(journal)
	if err != nil || !found {
		return conditionalMoveRecoveryObservation{}, errors.Join(ErrConditionalMutationResidue, err, errors.New("conditional move journal intent is missing or corrupt"))
	}
	plan, err := conditionalMovePlanFromDurableIntent(
		observer.MutationAuthority, intent, sourcePath, destinationPath, sourceIntent, destinationIntent, binding,
	)
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	pending, err := observeConditionalUnixMoveJournal(journal, plan)
	if err != nil {
		return conditionalMoveRecoveryObservation{}, err
	}
	state := classifyConditionalMoveRecoveryPublicState(ctx, observer, intent, sourceIntent, destinationIntent)
	if plan.destinationParentMissing {
		return observeConditionalMoveStagedRecovery(
			ctx, observer, intent, sourceIntent, destinationIntent, plan, journal, pending, state,
		)
	}
	if len(pending) == 0 {
		return conditionalMoveRecoveryObservation{
			state: state, journalPresent: true, bindingDigest: binding.BindingDigest, plan: plan,
		}, nil
	}
	if state != conditionalMoveRecoveryQuarantined ||
		!conditionalMoveRecoveryPathAbsent(ctx, observer, intent, sourceIntent) ||
		!conditionalMoveRecoveryPathAbsent(ctx, observer, intent, destinationIntent) {
		return conditionalMoveRecoveryObservation{}, ErrConditionalMutationResidue
	}
	return conditionalMoveRecoveryObservation{
		state: conditionalMoveRecoveryRollback, journalPresent: true,
		bindingDigest: binding.BindingDigest, pendingName: pending[0].name, plan: plan,
	}, nil
}

func (observer CheckpointOperationObserver) resolveMoveOperationIntent(
	intent domaincheckpoint.OperationGroupIntentV2,
) (string, string, domaincheckpoint.OperationPathV2, domaincheckpoint.OperationPathV2, error) {
	var source domaincheckpoint.OperationPathV2
	var destination domaincheckpoint.OperationPathV2
	for _, item := range intent.Paths {
		switch item.Role {
		case "source":
			source = item
		case "destination":
			destination = item
		}
	}
	if source.Role == "" || destination.Role == "" {
		return "", "", source, destination, errors.New("conditional move recovery intent paths are incomplete")
	}
	sourceAuthority := operationPathAuthorityForRecovery(source)
	destinationAuthority := operationPathAuthorityForRecovery(destination)
	sourcePath, err := observer.resolvePathAuthority(intent.SecurityContext.WorkspaceRealPath, sourceAuthority)
	if err != nil {
		return "", "", source, destination, err
	}
	destinationPath, err := observer.resolvePathAuthority(intent.SecurityContext.WorkspaceRealPath, destinationAuthority)
	if err != nil {
		return "", "", source, destination, err
	}
	return sourcePath, destinationPath, source, destination, nil
}

func operationPathAuthorityForRecovery(item domaincheckpoint.OperationPathV2) checkpointfileport.PathAuthority {
	return checkpointfileport.PathAuthority{
		SchemaVersion: item.PathAuthoritySchemaVersion, Kind: item.AuthorityKind,
		Root: item.AuthorityRoot, RootIdentity: item.AuthorityRootIdentity,
		RootHash: item.AuthorityRootHash, RelativePath: item.RelativePath,
	}
}

func conditionalMovePlanFromDurableIntent(
	authority ConditionalMutationAuthority,
	intent domaincheckpoint.OperationGroupIntentV2,
	sourcePath string,
	destinationPath string,
	source domaincheckpoint.OperationPathV2,
	destination domaincheckpoint.OperationPathV2,
	binding conditionalUnixMoveIntentBinding,
) (MoveRegularFilePlan, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(source.BeforeBytesBase64)
	if err != nil || int64(len(raw)) != source.BeforeSizeBytes || digestAtomicText(raw) != source.BeforeHash {
		return MoveRegularFilePlan{}, errors.New("conditional move durable source snapshot is invalid")
	}
	plan := MoveRegularFilePlan{
		SourcePath: sourcePath, DestinationPath: destinationPath,
		SourceRelativePath: filepath.FromSlash(source.RelativePath), DestinationRelativePath: filepath.FromSlash(destination.RelativePath),
		BytesMoved: source.BeforeSizeBytes, sourceHash: source.BeforeHash, sourceMode: os.FileMode(binding.SourceMode),
		sourceDevice: binding.SourceDevice, sourceInode: binding.SourceInode, sourceRevision: binding.SourceRevision,
		mutationAuthority: authority, operationGroupID: intent.OperationGroupID, operationDigest: intent.IntentDigest,
		sourceAuthority: source.AuthorityRootHash, sourceRelative: source.RelativePath,
		destinationAuthority: destination.AuthorityRootHash, destinationRelative: destination.RelativePath,
		sourceEncoding: source.BeforeEncoding, sourceBytesBase64: source.BeforeBytesBase64, operationBound: true,
	}
	plan = hydrateConditionalUnixMovePlanTopology(plan, binding)
	expected, _, err := conditionalUnixMoveIntentForPlan(plan)
	if err != nil || expected != binding {
		return MoveRegularFilePlan{}, errors.Join(err, errors.New("conditional move journal does not match its durable open intent"))
	}
	return plan, nil
}

func classifyConditionalMoveRecoveryPublicState(
	ctx context.Context,
	observer CheckpointOperationObserver,
	intent domaincheckpoint.OperationGroupIntentV2,
	source domaincheckpoint.OperationPathV2,
	destination domaincheckpoint.OperationPathV2,
) conditionalMoveRecoveryState {
	sourceObserved := observer.ObserveRelative(ctx, intent.SecurityContext.WorkspaceRealPath, operationPathAuthorityForRecovery(source))
	destinationObserved := observer.ObserveRelative(ctx, intent.SecurityContext.WorkspaceRealPath, operationPathAuthorityForRecovery(destination))
	if sourceObserved.ObservationStatus == "exact" && sourceObserved.Existed && sourceObserved.Hash == source.BeforeHash &&
		destinationObserved.ObservationStatus == "exact" && !destinationObserved.Existed {
		return conditionalMoveRecoveryNoEffect
	}
	if sourceObserved.ObservationStatus == "exact" && !sourceObserved.Existed &&
		destinationObserved.ObservationStatus == "exact" && destinationObserved.Existed && destinationObserved.Hash == destination.ExpectedAfterHash {
		return conditionalMoveRecoveryCompleted
	}
	return conditionalMoveRecoveryQuarantined
}

func conditionalMoveRecoveryPathAbsent(
	ctx context.Context,
	observer CheckpointOperationObserver,
	intent domaincheckpoint.OperationGroupIntentV2,
	path domaincheckpoint.OperationPathV2,
) bool {
	observed := observer.ObserveRelative(ctx, intent.SecurityContext.WorkspaceRealPath, operationPathAuthorityForRecovery(path))
	return observed.ObservationStatus == "exact" && !observed.Existed
}

func sameConditionalMoveRecoveryObservation(left, right conditionalMoveRecoveryObservation) bool {
	return left.state == right.state && left.journalPresent == right.journalPresent &&
		left.bindingDigest == right.bindingDigest && left.pendingName == right.pendingName && left.stageReceiptDigest == right.stageReceiptDigest &&
		left.plan.operationGroupID == right.plan.operationGroupID && left.plan.operationDigest == right.plan.operationDigest &&
		left.plan.sourceHash == right.plan.sourceHash && left.plan.sourceDevice == right.plan.sourceDevice &&
		left.plan.sourceInode == right.plan.sourceInode && left.plan.sourceRevision == right.plan.sourceRevision
}

func applyConditionalMovePendingRollback(
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
	binding, found, err := readConditionalUnixMoveIntent(journal)
	if err != nil || !found || binding.BindingDigest != observation.bindingDigest {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	pending, err := observeConditionalUnixMoveJournal(journal, plan)
	if err != nil || len(pending) != 1 || pending[0].name != observation.pendingName {
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
	destinationExists, err := prepareConditionalMoveDestination(plan.DestinationPath)
	if err != nil || destinationExists {
		return errors.Join(ErrConditionalMutationResidue, err)
	}
	if err := renameConditionalUnixNoReplace(journal, pending[0].name, sourceParent, sourceBase); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
		return errors.Join(ErrConditionalMutationIndeterminate, err)
	}
	restored, err := observeConditionalUnixAt(sourceParent, sourceBase)
	if err != nil || !conditionalUnixMatchesMoveInstalled(plan, restored) || restored.identity != pending[0].identity {
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("conditional move source rollback readback failed"))
	}
	remaining, err := observeConditionalUnixMoveJournal(journal, plan)
	if err != nil || len(remaining) != 0 {
		return errors.Join(ErrConditionalMutationIndeterminate, err, errors.New("conditional move private rollback did not settle"))
	}
	destinationExists, err = prepareConditionalMoveDestination(plan.DestinationPath)
	if err != nil || destinationExists {
		return errors.Join(ErrConditionalMutationResidue, err, fmt.Errorf("destination changed during conditional move rollback"))
	}
	return nil
}
