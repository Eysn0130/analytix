package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// verifiedPrivateCASCommitWitnessV4 is the package-private deletion
// capability. It can be issued only after a fresh journal load, full session
// authority validation, and installation-key signature verification.
type verifiedPrivateCASCommitWitnessV4 struct {
	transactionID  string
	witnessDigest  string
	commitTargetID string
}

func verifyPrivateCASCommitWitnessV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
	session domainprivatecas.RecoveryJournalSessionV1,
) (verifiedPrivateCASCommitWitnessV4, error) {
	if journal == nil || session.State != domainprivatecas.RecoveryJournalSessionCommitWitnessedV1 ||
		session.Manifest == nil || session.CommitWitness == nil {
		return verifiedPrivateCASCommitWitnessV4{}, errors.New("private CAS recovery deletion witness is unavailable")
	}
	if err := validatePreparedPrivateCASRecoverySessionAuthorityV4(prepared, session); err != nil {
		return verifiedPrivateCASCommitWitnessV4{}, err
	}
	signingBytes, err := domainprivatecas.RecoveryJournalCommitWitnessSigningBytesV1(*session.CommitWitness)
	if err != nil {
		return verifiedPrivateCASCommitWitnessV4{}, err
	}
	if err := journal.Verify(
		ctx,
		session.CommitWitness.AuthorityKeyID,
		signingBytes,
		session.CommitWitness.AuthoritySignature,
	); err != nil {
		return verifiedPrivateCASCommitWitnessV4{}, errors.Join(
			errors.New("private CAS recovery deletion witness signature is invalid"), err,
		)
	}
	return verifiedPrivateCASCommitWitnessV4{
		transactionID: session.Manifest.TransactionID, witnessDigest: session.CommitWitness.WitnessDigest,
		commitTargetID: session.CommitWitness.CommitTargetID,
	}, nil
}

// ValidateNoUnsignedSecurePrivateCASRecoveryPhasesV4 is a read-only upgrade
// guard used before any startup cleanup. Plain write residues may enter a new
// signed manifest after full semantic preflight; legacy staged/committed names
// have no external witness and can never bootstrap deletion authority.
func ValidateNoUnsignedSecurePrivateCASRecoveryPhasesV4(
	ctx context.Context,
	roots []string,
	access SecurePrivateCASRecoveryAccessAuthority,
) error {
	return validateNoUnsignedSecurePrivateCASRecoveryPhasesV4(ctx, roots, access, false)
}

// ValidateNoUnsignedSecurePrivateCASRecoveryPhasesWithPreparedCreateResiduesV4
// permits only the exact empty shard-create directories already frozen by the
// supplied global create-residue plan. Transaction phases are still rejected
// before that plan may delete anything, preventing partial cleanup when an
// unsigned legacy marker exists elsewhere in the runtime inventory.
func ValidateNoUnsignedSecurePrivateCASRecoveryPhasesWithPreparedCreateResiduesV4(
	ctx context.Context,
	roots []string,
	access SecurePrivateCASRecoveryAccessAuthority,
	preparedCreateResidues *PreparedSecurePrivateCASDirectoryRecoveryV1,
) error {
	if access == nil || preparedCreateResidues == nil ||
		preparedCreateResidues.mode != privateCASDirectoryRecoveryCreateResiduesV1 {
		return errors.New("private CAS prepared create-residue probe is invalid")
	}
	if err := preparedCreateResidues.Revalidate(ctx); err != nil {
		return err
	}
	if err := withExistingPrivateCASAccess(
		ctx,
		access,
		preparedCreateResidues.requestedPrivateRoot,
		func(binding privatecasport.RootBinding) error {
			if binding != preparedCreateResidues.binding {
				return errors.New("private CAS prepared create-residue access authority changed")
			}
			return nil
		},
	); err != nil {
		return err
	}
	if err := validateNoUnsignedSecurePrivateCASRecoveryPhasesV4(ctx, roots, access, true); err != nil {
		return err
	}
	return preparedCreateResidues.Revalidate(ctx)
}

func validateNoUnsignedSecurePrivateCASRecoveryPhasesV4(
	ctx context.Context,
	roots []string,
	access SecurePrivateCASRecoveryAccessAuthority,
	allowPreparedCreateResidues bool,
) error {
	if access == nil || len(roots) == 0 || len(roots) > domainprivatecas.MaxRecoveryJournalPlansV1 {
		return errors.New("private CAS unsigned recovery phase probe is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	canonical := append([]string(nil), roots...)
	sort.Slice(canonical, func(left, right int) bool {
		return strings.ToLower(canonical[left]) < strings.ToLower(canonical[right])
	})
	for index, root := range canonical {
		if root == "" || root != strings.TrimSpace(root) || filepath.Clean(root) != root ||
			index > 0 && strings.EqualFold(root, canonical[index-1]) {
			return errors.New("private CAS unsigned recovery phase roots are invalid")
		}
		state, err := observePrivateCASRecoveryTransactionStateV4(
			ctx, root, access, allowPreparedCreateResidues,
		)
		if err != nil {
			return err
		}
		if state.staged != 0 || state.committed != 0 || state.transactionID != "" {
			return errors.New("private CAS staged or committed recovery residue has no signed journal")
		}
	}
	return nil
}

func observePrivateCASRecoveryTransactionStateV4(
	ctx context.Context,
	root string,
	access SecurePrivateCASRecoveryAccessAuthority,
	allowPreparedCreateResidues bool,
) (privateCASRecoveryTransactionState, error) {
	requestedRoot, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return privateCASRecoveryTransactionState{}, err
	}
	err = withExistingPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || !present {
			return err
		}
		return securePrivateCASValidateNoUnsignedRecoveryPhasesV4(
			ctx, authority, maxPrivateAcceptedFinalBytes, allowPreparedCreateResidues,
		)
	})
	return privateCASRecoveryTransactionState{}, err
}

// ApplyPreparedSecurePrivateCASRecoveryTransactionV4 is the only recovery
// engine whose deletion authority survives a process crash. The external,
// installation-signed commit witness is the unique no-rollback point; an
// internal CAS commit filename is only a rename phase and never sufficient to
// authorize cleanup on its own.
func ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
	ctx context.Context,
	participants []PreparedSecurePrivateCASRecoveryParticipantV4,
	journal privatecasport.RecoveryJournalV1,
) error {
	if journal == nil {
		return errors.New("private CAS recovery V4 journal authority is required")
	}
	if len(participants) == 0 {
		return errors.New("private CAS recovery V4 participant manifest is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := privateCASRecoveryExclusion.acquireRecovery(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseRecovery()
	prepared, err := prepareSecurePrivateCASRecoveryAuthoritySetV4(ctx, participants)
	if err != nil {
		return err
	}
	return applyPreparedSecurePrivateCASRecoveryTransactionV4(ctx, prepared, journal)
}

// WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1 retires
// live in-process generations only after the caller has built and validated an
// exact semantic startup plan, then applies that plan while the recovery
// exclusion remains held. It is intentionally separate from no-target
// recovery preflight: a read-only recovery check must not revoke authorities,
// while an imminent atomic semantic replacement must prevent stale
// generations from attaching between retirement and apply.
func WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(
	ctx context.Context,
	participants []PreparedSecurePrivateCASRecoveryParticipantV4,
	affectedRootIDs []string,
	apply func(context.Context) error,
) error {
	if apply == nil {
		return errors.New("private CAS semantic apply callback is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := privateCASRecoveryExclusion.acquireRecovery(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseRecovery()
	prepared, err := prepareSecurePrivateCASRecoveryAuthoritySetV4(ctx, participants)
	if err != nil {
		return err
	}
	if len(prepared.currentTargets()) != 0 {
		return errors.New("private CAS semantic apply retirement found unresolved recovery targets")
	}
	if err := revokePreparedPrivateCASRecoveryGenerationSubsetV4(
		ctx, prepared, affectedRootIDs,
	); err != nil {
		return err
	}
	return privateCASRecoveryExclusion.withSemanticObservationV1(ctx, apply)
}

func applyPreparedSecurePrivateCASRecoveryTransactionV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
) error {
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	session, err := journal.Load(ctx)
	if errors.Is(err, privatecasport.ErrJournalAbsent) {
		return beginPreparedPrivateCASRecoveryTransactionV4(ctx, prepared, journal)
	}
	if err != nil {
		return fmt.Errorf("load signed private CAS recovery journal: %w", err)
	}
	if err := validatePreparedPrivateCASRecoverySessionAuthorityV4(prepared, session); err != nil {
		return err
	}
	switch session.State {
	case domainprivatecas.RecoveryJournalSessionPreparedV1:
		return continuePreparedPrivateCASRecoveryManifestV4(ctx, prepared, journal, session)
	case domainprivatecas.RecoveryJournalSessionManifestedV1:
		return continuePreparedPrivateCASRecoveryBeforeWitnessV4(ctx, prepared, journal, session)
	case domainprivatecas.RecoveryJournalSessionCommitWitnessedV1:
		return continueCommittedPrivateCASRecoveryV4(ctx, prepared, journal, session)
	case domainprivatecas.RecoveryJournalSessionCompletedV1:
		return retireCompletedPrivateCASRecoveryV4(ctx, prepared, journal, session)
	default:
		return errors.New("signed private CAS recovery journal has an unknown state")
	}
}

func beginPreparedPrivateCASRecoveryTransactionV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
) error {
	targets := prepared.currentTargets()
	if len(targets) == 0 {
		// A read-only no-target preflight is not a recovery generation. Keep
		// sibling authorities live so a later startup validation failure has
		// zero in-process authority side effects.
		return prepared.revalidate(ctx)
	}
	if err := validateUnsignedPrivateCASRecoveryTargetsV4(targets); err != nil {
		return err
	}
	chunks, err := privateCASRecoveryTargetChunksV4(targets)
	if err != nil {
		return err
	}
	preparation, err := journal.BeginPreparationAfterValidatedPreflight(ctx, privatecasport.RecoveryPreparationRequestV1{
		AuthoritySetDigest: prepared.authoritySetDigest,
		ParticipantCount:   uint32(len(prepared.participants)),
		PlanCount:          uint32(len(prepared.plans)),
		TopologyCount:      preparedPrivateCASTopologyCountV4(prepared),
		PreparedAt:         time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("persist signed private CAS recovery preparation: %w", err)
	}
	if err := validatePreparedPrivateCASRecoveryPreparationAuthorityV4(prepared, preparation); err != nil {
		return err
	}
	session := domainprivatecas.RecoveryJournalSessionV1{
		State: domainprivatecas.RecoveryJournalSessionPreparedV1, Preparation: preparation,
	}
	return persistPreparedPrivateCASRecoveryManifestV4(ctx, prepared, journal, session, chunks)
}

func continuePreparedPrivateCASRecoveryManifestV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
	session domainprivatecas.RecoveryJournalSessionV1,
) error {
	targets := prepared.currentTargets()
	if len(targets) == 0 {
		return errors.New("signed private CAS recovery preparation lost its complete target set")
	}
	if err := validateUnsignedPrivateCASRecoveryTargetsV4(targets); err != nil {
		return err
	}
	chunks, err := privateCASRecoveryTargetChunksV4(targets)
	if err != nil {
		return err
	}
	return persistPreparedPrivateCASRecoveryManifestV4(ctx, prepared, journal, session, chunks)
}

func persistPreparedPrivateCASRecoveryManifestV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
	session domainprivatecas.RecoveryJournalSessionV1,
	chunks []domainprivatecas.RecoveryTargetChunkV1,
) error {
	if err := validateRecoveryChunkPrefixReplayV4(session.Chunks, chunks); err != nil {
		return err
	}
	for index := len(session.Chunks); index < len(chunks); index++ {
		if err := journal.PutTargetChunkIfAbsent(ctx, chunks[index]); err != nil {
			return fmt.Errorf("persist private CAS recovery target chunk %d: %w", index, err)
		}
	}
	manifest, err := domainprivatecas.NewRecoveryJournalManifestDraftV1(domainprivatecas.RecoveryJournalManifestInputV1{
		Preparation: session.Preparation, Chunks: chunks, ManifestedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	manifest, err = sealPrivateCASRecoveryManifestV4(ctx, journal, manifest)
	if err != nil {
		return err
	}
	if err := journal.PutManifestIfAbsent(ctx, manifest); err != nil {
		return fmt.Errorf("persist signed private CAS recovery manifest: %w", err)
	}
	reloaded, err := journal.Load(ctx)
	if err != nil {
		return fmt.Errorf("reload signed private CAS recovery manifest: %w", err)
	}
	if reloaded.State != domainprivatecas.RecoveryJournalSessionManifestedV1 || reloaded.Manifest == nil ||
		reloaded.Manifest.ManifestDigest != manifest.ManifestDigest {
		return errors.New("signed private CAS recovery manifest readback is not exact")
	}
	if err := validatePreparedPrivateCASRecoverySessionAuthorityV4(prepared, reloaded); err != nil {
		return err
	}
	return continuePreparedPrivateCASRecoveryBeforeWitnessV4(ctx, prepared, journal, reloaded)
}

func continuePreparedPrivateCASRecoveryBeforeWitnessV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
	session domainprivatecas.RecoveryJournalSessionV1,
) error {
	if session.Manifest == nil {
		return errors.New("private CAS recovery manifested session omitted its manifest")
	}
	manifest := *session.Manifest
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	current := prepared.currentTargets()
	if err := validatePrivateCASRecoveryTargetsAgainstManifestV4(current, session.Chunks, manifest.TransactionID, false, ""); err != nil {
		return err
	}
	if privateCASRecoveryTargetsHaveTransactionPhaseV4(current) {
		if err := rollbackPreparedPrivateCASRecoveryTransactionV4(ctx, prepared, len(prepared.v3.plans)-1); err != nil {
			return errors.Join(errors.New("rollback pre-witness private CAS recovery phase: signed witness was not issued"), err)
		}
		if err := prepared.revalidate(ctx); err != nil {
			return err
		}
		current = prepared.currentTargets()
		if err := validatePrivateCASRecoveryTargetsAgainstManifestV4(current, session.Chunks, manifest.TransactionID, false, ""); err != nil {
			return err
		}
		if err := validateUnsignedPrivateCASRecoveryTargetsV4(current); err != nil {
			return err
		}
	}
	for index, plan := range prepared.v3.plans {
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if prepared.plans[index].recoveryTargetsFrozen {
			continue
		}
		if err := privateCASRecoveryTransactionTestCut("before_v4_plan_stage", index); err != nil {
			return err
		}
		if err := plan.stageTransaction(ctx, manifest.TransactionID); err != nil {
			return err
		}
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if err := privateCASRecoveryTransactionTestCut("after_v4_plan_stage", index); err != nil {
			return err
		}
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	current = prepared.currentTargets()
	if err := validatePrivateCASRecoveryTargetsAgainstManifestV4(current, session.Chunks, manifest.TransactionID, false, ""); err != nil {
		return err
	}
	markedPlan := -1
	for index, plan := range prepared.v3.plans {
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if prepared.plans[index].recoveryTargetsFrozen {
			continue
		}
		marked, err := plan.markTransactionCommit(ctx, manifest.TransactionID)
		if err != nil {
			// Whether the rename, sync, or readback failed is deliberately not
			// interpreted here. No deletion is legal until a fresh, signed
			// witness load proves the external no-rollback point.
			return err
		}
		if marked {
			markedPlan = index
			break
		}
	}
	if markedPlan < 0 {
		return errors.New("private CAS recovery V4 could not create its internal marker")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	current = prepared.currentTargets()
	commitTargetID, err := privateCASRecoveryCommitTargetIDV4(current, manifest.TransactionID)
	if err != nil {
		return err
	}
	if err := privateCASRecoveryTransactionTestCut("after_v4_internal_marker", markedPlan); err != nil {
		return err
	}
	witness, err := domainprivatecas.NewRecoveryJournalCommitWitnessDraftV1(domainprivatecas.RecoveryJournalCommitWitnessInputV1{
		Manifest: manifest, Chunks: session.Chunks, CommitTargetID: commitTargetID, WitnessedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	witness, err = sealPrivateCASRecoveryCommitWitnessV4(ctx, journal, witness)
	if err != nil {
		return err
	}
	if err := journal.PutCommitWitnessIfAbsent(ctx, witness); err != nil {
		// Never roll back here. The exclusive write may have reached durable
		// storage even when sync/readback returned an error.
		return fmt.Errorf("persist private CAS recovery commit witness: %w", err)
	}
	if err := privateCASRecoveryTransactionTestCut("after_v4_commit_witness", markedPlan); err != nil {
		return err
	}
	reloaded, err := journal.Load(ctx)
	if err != nil {
		return fmt.Errorf("reload private CAS recovery commit witness: %w", err)
	}
	if reloaded.State != domainprivatecas.RecoveryJournalSessionCommitWitnessedV1 || reloaded.CommitWitness == nil ||
		reloaded.CommitWitness.WitnessDigest != witness.WitnessDigest {
		return errors.New("private CAS recovery commit witness readback is not exact")
	}
	if err := validatePreparedPrivateCASRecoverySessionAuthorityV4(prepared, reloaded); err != nil {
		return err
	}
	return continueCommittedPrivateCASRecoveryV4(ctx, prepared, journal, reloaded)
}

func continueCommittedPrivateCASRecoveryV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
	session domainprivatecas.RecoveryJournalSessionV1,
) error {
	if session.Manifest == nil || session.CommitWitness == nil {
		return errors.New("private CAS recovery committed session omitted its signed authority")
	}
	deletionWitness, err := verifyPrivateCASCommitWitnessV4(ctx, prepared, journal, session)
	if err != nil {
		return err
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	current := prepared.currentTargets()
	if err := validatePrivateCASRecoveryTargetsAgainstManifestV4(
		current, session.Chunks, session.Manifest.TransactionID, true, session.CommitWitness.CommitTargetID,
	); err != nil {
		return err
	}
	if len(current) > 0 {
		if err := commitPreparedPrivateCASRecoveryAuthoritySetWithWitnessV4(ctx, prepared, deletionWitness); err != nil {
			return err
		}
		if err := prepared.revalidate(ctx); err != nil {
			return err
		}
		current = prepared.currentTargets()
	}
	if len(current) != 0 {
		return errors.New("private CAS recovery commit left signed targets behind")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	if err := privateCASRecoveryTransactionTestCut("after_v4_cleanup", -1); err != nil {
		return err
	}
	finalInventoryDigest := privateCASRecoveryFinalInventoryDigestV4(prepared.authoritySetDigest)
	receipt, err := domainprivatecas.NewRecoveryJournalCompletionDraftV1(domainprivatecas.RecoveryJournalCompletionInputV1{
		Manifest: *session.Manifest, CommitWitness: *session.CommitWitness,
		FinalInventoryDigest: finalInventoryDigest, CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	receipt, err = sealPrivateCASRecoveryCompletionV4(ctx, journal, receipt)
	if err != nil {
		return err
	}
	if err := journal.PutCompletionReceiptIfAbsent(ctx, receipt); err != nil {
		return fmt.Errorf("persist private CAS recovery completion receipt: %w", err)
	}
	if err := privateCASRecoveryTransactionTestCut("after_v4_completion_receipt", -1); err != nil {
		return err
	}
	reloaded, err := journal.Load(ctx)
	if err != nil {
		return fmt.Errorf("reload private CAS recovery completion receipt: %w", err)
	}
	if reloaded.State != domainprivatecas.RecoveryJournalSessionCompletedV1 || reloaded.CompletionReceipt == nil ||
		reloaded.CompletionReceipt.ReceiptDigest != receipt.ReceiptDigest {
		return errors.New("private CAS recovery completion readback is not exact")
	}
	return retireCompletedPrivateCASRecoveryV4(ctx, prepared, journal, reloaded)
}

func retireCompletedPrivateCASRecoveryV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	journal privatecasport.RecoveryJournalV1,
	session domainprivatecas.RecoveryJournalSessionV1,
) error {
	if session.Manifest == nil || session.CompletionReceipt == nil {
		return errors.New("completed private CAS recovery session omitted its receipt")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	if len(prepared.currentTargets()) != 0 {
		return errors.New("completed private CAS recovery session regained a residue")
	}
	finalInventoryDigest := privateCASRecoveryFinalInventoryDigestV4(prepared.authoritySetDigest)
	if session.CompletionReceipt.FinalInventoryDigest != finalInventoryDigest {
		return errors.New("private CAS recovery completion no longer matches final inventory")
	}
	if err := journal.Retire(ctx, privatecasport.RecoveryRetirementRequestV1{
		TransactionID:           session.Manifest.TransactionID,
		CompletionReceiptDigest: session.CompletionReceipt.ReceiptDigest,
	}); err != nil {
		return fmt.Errorf("retire completed private CAS recovery journal: %w", err)
	}
	return revokePreparedPrivateCASRecoveryGenerationsV4(ctx, prepared)
}

func validatePreparedPrivateCASRecoverySessionAuthorityV4(
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	session domainprivatecas.RecoveryJournalSessionV1,
) error {
	if err := domainprivatecas.ValidateRecoveryJournalSessionV1(session); err != nil {
		return err
	}
	return validatePreparedPrivateCASRecoveryPreparationAuthorityV4(prepared, session.Preparation)
}

func validatePreparedPrivateCASRecoveryPreparationAuthorityV4(
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
) error {
	if prepared == nil || preparation.AuthoritySetDigest != prepared.authoritySetDigest ||
		preparation.ParticipantCount != uint32(len(prepared.participants)) ||
		preparation.PlanCount != uint32(len(prepared.plans)) ||
		preparation.TopologyCount != preparedPrivateCASTopologyCountV4(prepared) {
		return errors.New("signed private CAS recovery journal belongs to a different authority set")
	}
	return nil
}

func preparedPrivateCASTopologyCountV4(prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4) uint32 {
	var count uint32
	if prepared == nil {
		return 0
	}
	for _, participant := range prepared.participants {
		count += uint32(len(participant.topologyDigests))
	}
	return count
}

func privateCASRecoveryTargetChunksV4(
	targets []privateCASRecoveryTargetObservationV4,
) ([]domainprivatecas.RecoveryTargetChunkV1, error) {
	entries := make([]domainprivatecas.RecoveryTargetEntryV1, 0, len(targets))
	for _, target := range targets {
		entries = append(entries, target.entry)
	}
	return domainprivatecas.BuildRecoveryTargetChunksV1(entries)
}

func validateUnsignedPrivateCASRecoveryTargetsV4(targets []privateCASRecoveryTargetObservationV4) error {
	for _, target := range targets {
		if target.phase != privateCASRecoveryQuarantinePlain || target.transactionID != "" {
			return errors.New("unsigned private CAS staged or committed recovery residue is not trusted")
		}
	}
	return nil
}

func validatePrivateCASRecoveryTargetsAgainstManifestV4(
	current []privateCASRecoveryTargetObservationV4,
	chunks []domainprivatecas.RecoveryTargetChunkV1,
	transactionID string,
	allowCommittedSubset bool,
	commitTargetID string,
) error {
	authorized := make(map[string]domainprivatecas.RecoveryTargetEntryV1)
	for _, chunk := range chunks {
		for _, entry := range chunk.Entries {
			authorized[entry.TargetID] = entry
		}
	}
	if !allowCommittedSubset && len(current) != len(authorized) {
		return errors.New("private CAS pre-witness recovery target set is not complete")
	}
	committed := 0
	commitPresent := false
	for _, target := range current {
		expected, ok := authorized[target.entry.TargetID]
		if !ok || expected != target.entry {
			return errors.New("private CAS recovery contains an unknown or mismatched signed target")
		}
		switch target.phase {
		case privateCASRecoveryQuarantinePlain:
			if allowCommittedSubset || target.transactionID != "" {
				return errors.New("private CAS committed recovery regressed to an unsigned plain residue")
			}
		case privateCASRecoveryQuarantineStaged:
			if target.transactionID != transactionID {
				return errors.New("private CAS recovery staged target belongs to another transaction")
			}
		case privateCASRecoveryQuarantineCommitted:
			if target.transactionID != transactionID {
				return errors.New("private CAS recovery internal marker belongs to another transaction")
			}
			committed++
			commitPresent = commitPresent || target.entry.TargetID == commitTargetID
		default:
			return errors.New("private CAS recovery target phase is invalid")
		}
	}
	if committed > 1 {
		return errors.New("private CAS recovery contains multiple internal markers")
	}
	if allowCommittedSubset && len(current) > 0 && (committed != 1 || !commitPresent) {
		return errors.New("signed private CAS recovery commit marker is missing or mismatched")
	}
	return nil
}

func privateCASRecoveryTargetsHaveTransactionPhaseV4(targets []privateCASRecoveryTargetObservationV4) bool {
	for _, target := range targets {
		if target.phase != privateCASRecoveryQuarantinePlain || target.transactionID != "" {
			return true
		}
	}
	return false
}

func privateCASRecoveryCommitTargetIDV4(
	targets []privateCASRecoveryTargetObservationV4,
	transactionID string,
) (string, error) {
	commitTargetID := ""
	for _, target := range targets {
		if target.phase != privateCASRecoveryQuarantineCommitted {
			continue
		}
		if target.transactionID != transactionID || commitTargetID != "" {
			return "", errors.New("private CAS recovery internal marker is ambiguous")
		}
		commitTargetID = target.entry.TargetID
	}
	if commitTargetID == "" {
		return "", errors.New("private CAS recovery internal marker is absent")
	}
	return commitTargetID, nil
}

func validateRecoveryChunkPrefixReplayV4(
	persisted []domainprivatecas.RecoveryTargetChunkV1,
	expected []domainprivatecas.RecoveryTargetChunkV1,
) error {
	if len(persisted) > len(expected) {
		return errors.New("private CAS recovery journal contains extra target chunks")
	}
	for index := range persisted {
		left, leftErr := domainprivatecas.RecoveryTargetChunkV1Bytes(persisted[index])
		right, rightErr := domainprivatecas.RecoveryTargetChunkV1Bytes(expected[index])
		if leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			return errors.New("private CAS recovery target chunk prefix conflicts with current authority")
		}
	}
	return nil
}

func sealPrivateCASRecoveryManifestV4(
	ctx context.Context,
	journal privatecasport.RecoveryJournalV1,
	draft domainprivatecas.RecoveryJournalManifestV1,
) (domainprivatecas.RecoveryJournalManifestV1, error) {
	body, err := domainprivatecas.RecoveryJournalManifestSigningBytesV1(draft)
	if err != nil {
		return domainprivatecas.RecoveryJournalManifestV1{}, err
	}
	signature, err := journal.Sign(ctx, body)
	if err != nil {
		return domainprivatecas.RecoveryJournalManifestV1{}, err
	}
	return domainprivatecas.SealRecoveryJournalManifestV1(draft, signature)
}

func sealPrivateCASRecoveryCommitWitnessV4(
	ctx context.Context,
	journal privatecasport.RecoveryJournalV1,
	draft domainprivatecas.RecoveryJournalCommitWitnessV1,
) (domainprivatecas.RecoveryJournalCommitWitnessV1, error) {
	body, err := domainprivatecas.RecoveryJournalCommitWitnessSigningBytesV1(draft)
	if err != nil {
		return domainprivatecas.RecoveryJournalCommitWitnessV1{}, err
	}
	signature, err := journal.Sign(ctx, body)
	if err != nil {
		return domainprivatecas.RecoveryJournalCommitWitnessV1{}, err
	}
	return domainprivatecas.SealRecoveryJournalCommitWitnessV1(draft, signature)
}

func sealPrivateCASRecoveryCompletionV4(
	ctx context.Context,
	journal privatecasport.RecoveryJournalV1,
	draft domainprivatecas.RecoveryJournalCompletionReceiptV1,
) (domainprivatecas.RecoveryJournalCompletionReceiptV1, error) {
	body, err := domainprivatecas.RecoveryJournalCompletionSigningBytesV1(draft)
	if err != nil {
		return domainprivatecas.RecoveryJournalCompletionReceiptV1{}, err
	}
	signature, err := journal.Sign(ctx, body)
	if err != nil {
		return domainprivatecas.RecoveryJournalCompletionReceiptV1{}, err
	}
	return domainprivatecas.SealRecoveryJournalCompletionV1(draft, signature)
}

func revokePreparedPrivateCASRecoveryGenerationsV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
) error {
	rootIDs := make([]string, 0, len(prepared.plans))
	for _, plan := range prepared.plans {
		rootIDs = append(rootIDs, plan.rootID)
	}
	return revokePreparedPrivateCASRecoveryGenerationSubsetV4(ctx, prepared, rootIDs)
}

func revokePreparedPrivateCASRecoveryGenerationSubsetV4(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV4,
	rootIDs []string,
) error {
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	if len(prepared.currentTargets()) != 0 {
		return errors.New("private CAS generation retirement found unresolved recovery targets")
	}
	requested := make(map[string]struct{}, len(rootIDs))
	available := make(map[string]struct{}, len(prepared.plans))
	for _, binding := range prepared.plans {
		available[binding.rootID] = struct{}{}
	}
	for _, rootID := range rootIDs {
		if rootID == "" || rootID != strings.TrimSpace(rootID) {
			return errors.New("private CAS generation retirement root id is invalid")
		}
		if _, duplicate := requested[rootID]; duplicate {
			return errors.New("private CAS generation retirement repeats a root id")
		}
		if _, known := available[rootID]; !known {
			return errors.New("private CAS generation retirement contains an unknown root id")
		}
		requested[rootID] = struct{}{}
	}
	matched := 0
	for index, binding := range prepared.plans {
		if _, affected := requested[binding.rootID]; !affected {
			continue
		}
		matched++
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if binding.recoveryTargetsFrozen {
			continue
		}
		plan := prepared.v3.plans[index]
		if err := revokePrivateCASRootGeneration(plan.binding, plan.rootPath); err != nil {
			return err
		}
		if err := prepared.v3.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
	}
	if matched != len(requested) {
		return errors.New("private CAS generation retirement root selection changed")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	if len(prepared.currentTargets()) != 0 {
		return errors.New("private CAS generation retirement gained a recovery target")
	}
	return nil
}
