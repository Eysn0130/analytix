package caseentity

import (
	"context"
	"errors"
	"reflect"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type AppendPersistedEvidenceReceiptInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	Receipt         domainevidence.EvidenceReceipt
}

func AppendPersistedEvidenceReceiptV1(
	ctx context.Context,
	service *Service,
	input AppendPersistedEvidenceReceiptInputV1,
) error {
	receipt := input.Receipt
	securityContext := input.SecurityContext
	if domainevidence.ValidateEvidenceReceipt(receipt) != nil ||
		receipt.ThreadID != securityContext.ThreadID || receipt.TurnID != securityContext.TurnID ||
		receipt.CaseID != securityContext.CaseID || receipt.CaseBindingHash != securityContext.CaseBindingHash ||
		receipt.ContextEpoch != securityContext.ContextEpoch || receipt.ContextDigest != securityContext.ContextDigest ||
		receipt.DatasetSnapshotID != securityContext.DatasetSnapshotID {
		return errors.New("case longitudinal evidence receipt is invalid")
	}
	if service == nil {
		return ErrPrivateStateNotFound
	}
	return service.AppendCaseLongitudinalOwnerStateV1(ctx, AppendCaseLongitudinalOwnerStateInputV1{
		SecurityContext: securityContext,
		Evidence: []CaseLongitudinalEvidenceDigestV1{{
			EvidenceReference: receipt.ReceiptID,
			EvidenceDigest:    receipt.ReceiptDigest,
		}},
	})
}

type FinalizedCaseThreadReaderV1 interface {
	GetThread(string) (map[string]any, error)
}

type FinalizedCaseClaimsV1 interface {
	CaseLongitudinalClaimsV1() ([]domainevidence.ClaimRecord, bool)
}

type finalizedCaseAcceptedSlotsV1 interface {
	UseCaseLongitudinalAcceptedSlotsV1(func(
		domainsecurity.TurnSecurityContext,
		string,
		string,
		string,
		domainevidence.AcceptedEntitySlotBindingV1,
	) error) (bool, error)
}

type caseLongitudinalFinalizedSlotV1 struct {
	securityContext     domainsecurity.TurnSecurityContext
	acceptedFinalDigest string
	dispositionDigest   string
	finalGateVersion    string
	slot                domainevidence.AcceptedEntitySlotBindingV1
}

func AppendFinalizedCaseLongitudinalStateV1(
	ctx context.Context,
	service *Service,
	reader FinalizedCaseThreadReaderV1,
	threadID string,
	securityContext domainsecurity.TurnSecurityContext,
	finalized FinalizedCaseClaimsV1,
) error {
	if service == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return nil
	}
	ownerInput, err := finalizedCaseLongitudinalOwnerInputV1(reader, threadID, securityContext, finalized)
	if err != nil {
		return err
	}
	err = service.AppendCaseLongitudinalOwnerStateV1(ctx, ownerInput)
	if errors.Is(err, ErrPrivateStateNotFound) {
		return nil
	}
	return err
}

func finalizedCaseLongitudinalOwnerInputV1(
	reader FinalizedCaseThreadReaderV1,
	threadID string,
	securityContext domainsecurity.TurnSecurityContext,
	finalized FinalizedCaseClaimsV1,
) (AppendCaseLongitudinalOwnerStateInputV1, error) {
	if finalized == nil {
		return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case finalization owner state is unavailable")
	}
	claimsInput, ok := finalized.CaseLongitudinalClaimsV1()
	if !ok {
		return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case finalization owner state is unavailable")
	}
	if reader == nil {
		return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case continuation digest owner is unavailable")
	}
	thread, err := reader.GetThread(threadID)
	if err != nil {
		return AppendCaseLongitudinalOwnerStateInputV1{}, err
	}
	continuation, err := appturn.BuildTaskContinuationSnapshotV1(thread)
	if err != nil || !domainsecurity.IsSHA256Hex(continuation.StateDigest) {
		return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case continuation digest owner is unavailable")
	}
	claims := make([]CaseLongitudinalClaimDigestV1, 0, len(claimsInput))
	claimsByReference := make(map[string]domainevidence.ClaimRecord, len(claimsInput))
	for _, claim := range claimsInput {
		if domainevidence.ValidateClaimRecord(claim) != nil {
			return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case claim digest owner is unavailable")
		}
		investigationState := domaincaseentity.InvestigationOpenV1
		switch claim.SupportState {
		case domainevidence.ClaimVerified:
			investigationState = domaincaseentity.InvestigationConfirmedV1
		case domainevidence.ClaimRefuted:
			investigationState = domaincaseentity.InvestigationRejectedV1
		case domainevidence.ClaimPartial, domainevidence.ClaimUnresolved:
		default:
			return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case claim currentness is unavailable")
		}
		claims = append(claims, CaseLongitudinalClaimDigestV1{
			ClaimReference: claim.ClaimID, ClaimDigest: claim.RecordDigest,
			ClaimType:                 string(claim.ClaimType),
			InvestigationState:        investigationState,
			EvidenceReferences:        append([]string(nil), claim.EvidenceIDs...),
			CounterEvidenceReferences: append([]string(nil), claim.CounterEvidenceIDs...),
		})
		if _, duplicate := claimsByReference[claim.ClaimID]; duplicate {
			return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case claim digest owner is unavailable")
		}
		claimsByReference[claim.ClaimID] = claim
	}
	var finalizedSlots []caseLongitudinalFinalizedSlotV1
	if owner, ok := finalized.(finalizedCaseAcceptedSlotsV1); ok {
		_, slotErr := owner.UseCaseLongitudinalAcceptedSlotsV1(func(
			finalContext domainsecurity.TurnSecurityContext,
			acceptedFinalDigest string,
			dispositionDigest string,
			finalGateVersion string,
			slot domainevidence.AcceptedEntitySlotBindingV1,
		) error {
			if !reflect.DeepEqual(finalContext, securityContext) ||
				!domainsecurity.IsSHA256Hex(acceptedFinalDigest) ||
				!domainsecurity.IsSHA256Hex(dispositionDigest) ||
				finalGateVersion != domainevidence.FinalEvidenceGateVersion ||
				len(slot.ClaimIDs) == 0 || len(slot.ReceiptIDs) == 0 {
				return errors.New("case accepted slot finalization owner is unavailable")
			}
			claimSet := make(map[string]struct{}, len(slot.ClaimIDs))
			receiptSet := make(map[string]struct{}, len(slot.ReceiptIDs))
			for _, claimID := range slot.ClaimIDs {
				claim, found := claimsByReference[claimID]
				if !found || (claim.SupportState != domainevidence.ClaimVerified && claim.SupportState != domainevidence.ClaimPartial) {
					return errors.New("case accepted slot claim owner is unavailable")
				}
				if _, duplicate := claimSet[claimID]; duplicate {
					return errors.New("case accepted slot claim owner is unavailable")
				}
				claimSet[claimID] = struct{}{}
				for _, receiptID := range claim.EvidenceIDs {
					receiptSet[receiptID] = struct{}{}
				}
			}
			if len(receiptSet) != len(slot.ReceiptIDs) {
				return errors.New("case accepted slot evidence owner is unavailable")
			}
			for _, receiptID := range slot.ReceiptIDs {
				if _, found := receiptSet[receiptID]; !found {
					return errors.New("case accepted slot evidence owner is unavailable")
				}
			}
			referenceCalls := 0
			if err := slot.UseReferenceV1(func(domaincaseentity.ReferenceV1) error {
				referenceCalls++
				return nil
			}); err != nil || referenceCalls != 1 {
				return errors.New("case accepted slot entity owner is unavailable")
			}
			finalizedSlots = append(finalizedSlots, caseLongitudinalFinalizedSlotV1{
				securityContext: finalContext, acceptedFinalDigest: acceptedFinalDigest,
				dispositionDigest: dispositionDigest, finalGateVersion: finalGateVersion, slot: slot,
			})
			return nil
		})
		if slotErr != nil {
			return AppendCaseLongitudinalOwnerStateInputV1{}, errors.New("case accepted slot finalization owner is unavailable")
		}
	}
	return AppendCaseLongitudinalOwnerStateInputV1{
		SecurityContext: securityContext, Claims: claims,
		ContinuationDigest: continuation.StateDigest,
		finalizedSlots:     finalizedSlots,
	}, nil
}
