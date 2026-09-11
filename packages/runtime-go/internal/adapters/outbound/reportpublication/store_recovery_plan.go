package reportpublication

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type PreparedRecoveryV1 struct {
	owner     *finalauthorityadapter.PreparedSecurePrivateCASOwnerRecoveryV1
	validated bool
}

type preparedArtifactMaterialV1 struct {
	reportSHA256 string
	byteLength   uint64
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("report publication prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("report publication prepared recovery root is invalid")
	}
	owner, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, originalPublicationLeavesV1(), access,
	)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{owner: owner}, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("report publication recovery plan is invalid")
	}
	err := prepared.owner.ValidateDomainSemantics(ctx, func(visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor) error {
		return prepared.validateDomainSemantics(ctx, visit, visitMaterials, nil)
	})
	prepared.validated = err == nil
	return err
}

func (prepared *PreparedRecoveryV1) validateDomainSemantics(ctx context.Context, visit finalauthorityadapter.PrivateCASDomainVisitor, visitMaterials finalauthorityadapter.PrivateCASDomainMaterialVisitor, checkInstallation func(string, string) error) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	attempts := map[string]domainpublication.PublicationAttemptV1{}
	receipts := map[string]domainpublication.PublicationReceiptV1{}
	commits := map[string]domainpublication.PublicationCommitReceiptV1{}
	selections := map[string]domainpublication.PublicationCommitSelectionV1{}
	decisions := map[string]domainpublication.ReportDeliveryDecisionV1{}
	grantSettlements := map[string]domainpublication.ReportGrantSettlementV1{}
	stageCompletions := map[string]domainpublication.ReportStageCompletionV1{}
	deliveryOutcomes := map[string]domainpublication.ReportDeliveryOutcomeV1{}
	indexes := map[string]domainpublication.PublicationIndexV1{}
	ledgers := map[string]domainpublication.ClaimLedgerV1{}
	projections := map[string]domainpublication.PIIProjectionV1{}
	inspections := map[string]domainpublication.RenderInspectionV1{}
	artifacts := map[string]preparedArtifactMaterialV1{}
	if err := visit("attempts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		attempt, err := domainpublication.ParsePublicationAttemptV1(file.Body)
		canonical, canonicalErr := domainpublication.PublicationAttemptV1Bytes(attempt)
		if err != nil || canonicalErr != nil || attempt.AttemptID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt attempt"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(attempt.AuthorityKeyID, attempt.AuthorityPublicKey); err != nil {
				return err
			}
		}
		if _, duplicate := attempts[attempt.AttemptID]; duplicate {
			return errors.New("report publication recovery contains a duplicate attempt")
		}
		attempts[attempt.AttemptID] = attempt
		return nil
	}); err != nil {
		return err
	}
	if err := visit("receipts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domainpublication.ParsePublicationReceiptV1(file.Body)
		canonical, canonicalErr := domainpublication.PublicationReceiptV1Bytes(receipt)
		if err != nil || canonicalErr != nil || receipt.RecordDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt receipt"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(receipt.AuthorityKeyID, receipt.AuthorityPublicKey); err != nil {
				return err
			}
		}
		receipts[file.Digest] = receipt
		return nil
	}); err != nil {
		return err
	}
	if err := visit("commit-receipts", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		commit, err := domainpublication.ParsePublicationCommitReceiptV1(file.Body)
		canonical, canonicalErr := domainpublication.PublicationCommitReceiptV1Bytes(commit)
		if err != nil || canonicalErr != nil || commit.RecordDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt commit receipt"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(commit.AuthorityKeyID, commit.AuthorityPublicKey); err != nil {
				return err
			}
		}
		commits[file.Digest] = commit
		return nil
	}); err != nil {
		return err
	}
	if err := visit("commit-selections", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		selection, err := domainpublication.ParsePublicationCommitSelectionV1(file.Body)
		canonical, canonicalErr := domainpublication.PublicationCommitSelectionV1Bytes(selection)
		if err != nil || canonicalErr != nil || selection.SelectionID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt commit selection"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(selection.AuthorityKeyID, selection.AuthorityPublicKey); err != nil {
				return err
			}
		}
		if _, duplicate := selections[selection.SelectionID]; duplicate {
			return errors.New("report publication recovery contains a duplicate commit selection")
		}
		selections[selection.SelectionID] = selection
		return nil
	}); err != nil {
		return err
	}
	if err := visit("delivery-decisions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		decision, err := domainpublication.ParseReportDeliveryDecisionV1(file.Body)
		canonical, canonicalErr := domainpublication.ReportDeliveryDecisionV1Bytes(decision)
		if err != nil || canonicalErr != nil || decision.DecisionID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt delivery decision"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(decision.AuthorityKeyID, decision.AuthorityPublicKey); err != nil {
				return err
			}
		}
		if _, duplicate := decisions[decision.DecisionID]; duplicate {
			return errors.New("report publication recovery contains a duplicate delivery decision")
		}
		decisions[decision.DecisionID] = decision
		return nil
	}); err != nil {
		return err
	}
	if err := visit("grant-settlements", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		settlement, err := domainpublication.ParseReportGrantSettlementV1(file.Body)
		canonical, canonicalErr := domainpublication.ReportGrantSettlementV1Bytes(settlement)
		if err != nil || canonicalErr != nil || settlement.SettlementID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt grant settlement"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(settlement.AuthorityKeyID, settlement.AuthorityPublicKey); err != nil {
				return err
			}
		}
		if _, duplicate := grantSettlements[settlement.SettlementID]; duplicate {
			return errors.New("report publication recovery contains a duplicate grant settlement")
		}
		grantSettlements[settlement.SettlementID] = settlement
		return nil
	}); err != nil {
		return err
	}
	if err := visit("stage-completions", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		completion, err := domainpublication.ParseReportStageCompletionV1(file.Body)
		canonical, canonicalErr := domainpublication.ReportStageCompletionV1Bytes(completion)
		if err != nil || canonicalErr != nil || completion.CompletionID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt stage completion"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(completion.AuthorityKeyID, completion.AuthorityPublicKey); err != nil {
				return err
			}
		}
		if _, duplicate := stageCompletions[completion.CompletionID]; duplicate {
			return errors.New("report publication recovery contains a duplicate stage completion")
		}
		stageCompletions[completion.CompletionID] = completion
		return nil
	}); err != nil {
		return err
	}
	if err := visit("delivery-projections", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		outcome, err := domainpublication.ParseReportDeliveryOutcomeV1(file.Body)
		canonical, canonicalErr := domainpublication.ReportDeliveryOutcomeV1Bytes(outcome)
		deliveryID := domainpublication.ReportDeliveryOutcomeID(outcome)
		if err != nil || canonicalErr != nil || deliveryID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt delivery outcome"), err, canonicalErr)
		}
		if checkInstallation != nil {
			var keyID, publicKey string
			if outcome.Projection != nil {
				keyID, publicKey = outcome.Projection.AuthorityKeyID, outcome.Projection.AuthorityPublicKey
			} else {
				keyID, publicKey = outcome.Rejection.AuthorityKeyID, outcome.Rejection.AuthorityPublicKey
			}
			if err := checkInstallation(keyID, publicKey); err != nil {
				return err
			}
		}
		if _, duplicate := deliveryOutcomes[deliveryID]; duplicate {
			return errors.New("report publication recovery contains a duplicate delivery outcome")
		}
		deliveryOutcomes[deliveryID] = outcome
		return nil
	}); err != nil {
		return err
	}
	if err := visit("indexes", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		index, err := domainpublication.ParsePublicationIndexV1(file.Body)
		canonical, canonicalErr := domainpublication.PublicationIndexV1Bytes(index)
		if err != nil || canonicalErr != nil || index.IndexDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt index"), err, canonicalErr)
		}
		if checkInstallation != nil {
			if err := checkInstallation(index.AuthorityKeyID, index.AuthorityPublicKey); err != nil {
				return err
			}
		}
		indexes[file.Digest] = index
		return nil
	}); err != nil {
		return err
	}
	if err := visit("claim-ledgers", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		ledger, err := domainpublication.ParseClaimLedgerV1(file.Body)
		canonical, canonicalErr := domainpublication.ClaimLedgerV1Bytes(ledger)
		if err != nil || canonicalErr != nil || ledger.LedgerDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt claim ledger"), err, canonicalErr)
		}
		ledgers[file.Digest] = ledger
		return nil
	}); err != nil {
		return err
	}
	if err := visit("pii-projections", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		projection, err := domainpublication.ParsePIIProjectionV1(file.Body)
		canonical, canonicalErr := domainpublication.PIIProjectionV1Bytes(projection)
		if err != nil || canonicalErr != nil || projection.ProjectionDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt PII projection"), err, canonicalErr)
		}
		projections[file.Digest] = projection
		return nil
	}); err != nil {
		return err
	}
	if err := visit("render-inspections", func(file finalauthorityadapter.SecurePrivateCASFile) error {
		inspection, err := domainpublication.ParseRenderInspectionV1(file.Body)
		canonical, canonicalErr := domainpublication.RenderInspectionV1Bytes(inspection)
		if err != nil || canonicalErr != nil || inspection.InspectionDigest != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("report publication recovery contains a corrupt render inspection"), err, canonicalErr)
		}
		inspections[file.Digest] = inspection
		return nil
	}); err != nil {
		return err
	}
	if err := visitMaterials(
		"artifacts",
		func(material finalauthorityadapter.SecurePrivateCASPreparedMaterialV1) error {
			if !domainsecurity.IsSHA256Hex(material.Digest) ||
				!domainsecurity.IsSHA256Hex(material.BodySHA256) ||
				material.ByteLength == 0 || material.ByteLength > maxPublishedArtifactBytes {
				return errors.New("report publication recovery contains an invalid protected artifact")
			}
			artifacts[material.Digest] = preparedArtifactMaterialV1{
				reportSHA256: material.BodySHA256,
				byteLength:   material.ByteLength,
			}
			return nil
		},
	); err != nil {
		return err
	}
	for _, receipt := range receipts {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		ledger, ledgerOK := ledgers[receipt.ClaimLedgerDigest]
		projection, projectionOK := projections[receipt.PIIProjectionDigest]
		inspection, inspectionOK := inspections[receipt.RenderInspectionDigest]
		artifact, artifactOK := artifacts[receipt.TargetIdentityDigest]
		if !ledgerOK || !projectionOK || !inspectionOK || !artifactOK ||
			domainpublication.ValidatePublicationReceiptMaterialsV1(receipt, ledger, projection, inspection) != nil ||
			receipt.ReportSHA256 != artifact.reportSHA256 || receipt.ReportByteLength != artifact.byteLength {
			return errors.New("report publication recovery receipt lost exact material or artifact authority")
		}
	}
	for _, index := range indexes {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		receipt, found := receipts[index.ReceiptRecordDigest]
		if !found || domainpublication.ValidatePublicationIndexReceiptV1(index, receipt) != nil {
			return errors.New("report publication recovery index lost its exact receipt")
		}
	}
	selectionByCandidate := make(map[string]domainpublication.PublicationCommitSelectionV1, len(selections))
	for _, selection := range selections {
		attempt, attemptOK := attempts[selection.AttemptID]
		receipt, receiptOK := receipts[selection.CandidateRecordDigest]
		index, indexOK := indexes[selection.PublicationIndexDigest]
		if !attemptOK || !receiptOK || !indexOK ||
			domainpublication.ValidatePublicationCommitSelectionMaterialsV1(
				selection, attempt, receipt, index, selection.AuthorityAdvanceSettlementDigest,
			) != nil {
			return errors.New("report publication recovery selection lost its exact attempt materials")
		}
		if _, duplicate := selectionByCandidate[selection.CandidateRecordDigest]; duplicate {
			return errors.New("report publication recovery contains multiple selections for one candidate")
		}
		selectionByCandidate[selection.CandidateRecordDigest] = selection
	}
	commitByCandidate := make(map[string]domainpublication.PublicationCommitReceiptV1, len(commits))
	for _, commit := range commits {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		receipt, receiptOK := receipts[commit.CandidateRecordDigest]
		index, indexOK := indexes[commit.PublicationIndexDigest]
		selection, selectionOK := selectionByCandidate[commit.CandidateRecordDigest]
		if !receiptOK || !indexOK || !selectionOK ||
			domainpublication.ValidatePublicationCommitReceiptMaterialsV1(commit, receipt, index) != nil ||
			domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil {
			return errors.New("report publication recovery commit lost its candidate or witnessed index")
		}
		if _, duplicate := commitByCandidate[commit.CandidateRecordDigest]; duplicate {
			return errors.New("report publication recovery contains multiple commits for one candidate")
		}
		commitByCandidate[commit.CandidateRecordDigest] = commit
	}
	for _, decision := range decisions {
		attempt, attemptOK := attempts[decision.AttemptID]
		receipt, receiptOK := receipts[decision.CandidateRecordDigest]
		index, indexOK := indexes[decision.PublicationIndexDigest]
		selection, selectionOK := selections[decision.CommitSelectionID]
		commit, commitOK := commits[decision.CommitRecordDigest]
		ledger, ledgerOK := ledgers[decision.ClaimLedgerDigest]
		projection, projectionOK := projections[decision.PIIProjectionDigest]
		inspection, inspectionOK := inspections[decision.RenderInspectionDigest]
		if !attemptOK || !receiptOK || !indexOK || !selectionOK || !commitOK || !ledgerOK || !projectionOK || !inspectionOK ||
			domainpublication.ValidateReportDeliveryDecisionDurableGraphV1(
				decision, attempt, receipt, index, selection, commit, ledger, projection, inspection,
			) != nil {
			return errors.New("report publication recovery delivery decision lost its exact durable material graph")
		}
	}
	settlementByDecision := make(map[string]domainpublication.ReportGrantSettlementV1, len(grantSettlements))
	for _, settlement := range grantSettlements {
		decision, found := decisions[settlement.DecisionID]
		if !found || domainpublication.ValidateReportGrantSettlementDecisionV1(settlement, decision) != nil {
			return errors.New("report publication recovery grant settlement lost its admitted decision")
		}
		if _, duplicate := settlementByDecision[settlement.DecisionID]; duplicate {
			return errors.New("report publication recovery contains multiple grant settlements for one decision")
		}
		settlementByDecision[settlement.DecisionID] = settlement
	}
	completionByDecision := make(map[string]domainpublication.ReportStageCompletionV1, len(stageCompletions))
	for _, completion := range stageCompletions {
		decision, decisionFound := decisions[completion.DecisionID]
		settlement, settlementFound := grantSettlements[completion.GrantSettlementID]
		if !decisionFound || !settlementFound ||
			domainpublication.ValidateReportStageCompletionDecisionSettlementV1(completion, decision, settlement) != nil {
			return errors.New("report publication recovery stage completion lost its decision or grant settlement")
		}
		if _, duplicate := completionByDecision[completion.DecisionID]; duplicate {
			return errors.New("report publication recovery contains multiple stage completions for one decision")
		}
		completionByDecision[completion.DecisionID] = completion
	}
	outcomeByCompletion := make(map[string]domainpublication.ReportDeliveryOutcomeV1, len(deliveryOutcomes))
	for _, outcome := range deliveryOutcomes {
		completionID := domainpublication.ReportDeliveryOutcomeCompletionID(outcome)
		completion, completionFound := stageCompletions[completionID]
		if !completionFound || domainpublication.ValidateReportDeliveryOutcomeCompletionV1(outcome, completion) != nil {
			return errors.New("report publication recovery delivery outcome lost its exact completion")
		}
		decision, decisionFound := decisions[completion.DecisionID]
		if !decisionFound || completion.DecisionID != decision.DecisionID {
			return errors.New("report publication recovery delivery outcome lost its exact decision")
		}
		if outcome.Kind == domainpublication.ReportDeliveryOutcomeProjectedV1 {
			projection := *outcome.Projection
			if projection.ReportVariant != decision.ReportVariant || projection.ClaimLedgerDigest != decision.ClaimLedgerDigest ||
				projection.ClaimCount != decision.ClaimCount || projection.PIIProjectionDigest != decision.PIIProjectionDigest ||
				projection.PIIProjectionClass != decision.PIIProjectionClass ||
				projection.AuthorizationAuditDigest != decision.AuthorizationAuditDigest ||
				projection.RenderInspectionDigest != decision.RenderInspectionDigest || projection.ReportSHA256 != decision.ReportSHA256 ||
				projection.ReportByteLength != decision.ReportByteLength || projection.MediaType != decision.MediaType ||
				projection.TargetIdentityDigest != decision.TargetIdentityDigest {
				return errors.New("report publication recovery projected outcome lost its exact decision")
			}
		}
		if _, duplicate := outcomeByCompletion[completionID]; duplicate {
			return errors.New("report publication recovery contains multiple delivery outcomes for one completion")
		}
		outcomeByCompletion[completionID] = outcome
	}
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("report publication recovery plan is invalid")
	}
	return prepared.owner.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.owner}
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.owner == nil {
		return nil
	}
	return prepared.owner.SecurePrivateCASRecoveryPlansV2()
}

func (prepared *PreparedRecoveryV1) ValidateInstallation(ctx context.Context, authority *finalauthorityadapter.AnchoredFileAuthority) error {
	if prepared == nil || prepared.owner == nil {
		return errors.New("private domain recovery plan is unavailable")
	}
	err := prepared.owner.ValidateDomainInstallation(ctx, authority, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		return prepared.validateDomainSemantics(ctx, visit, materials, check)
	})
	prepared.validated = err == nil
	return err
}

func originalPublicationLeavesV1() []finalauthorityadapter.SecurePrivateCASOwnerLeafV1 {
	return []finalauthorityadapter.SecurePrivateCASOwnerLeafV1{
		{Name: "attempts", MaxBytes: maxPublicationAttemptBytes},
		{Name: "receipts", MaxBytes: maxPublicationReceiptBytes},
		{Name: "commit-receipts", MaxBytes: maxPublicationCommitBytes},
		{Name: "commit-selections", MaxBytes: maxPublicationSelectionBytes},
		{Name: "delivery-decisions", MaxBytes: maxPublicationDecisionBytes},
		{Name: "grant-settlements", MaxBytes: maxReportGrantSettlementBytes},
		{Name: "stage-completions", MaxBytes: maxReportStageCompletionBytes},
		{Name: "delivery-projections", MaxBytes: maxReportDeliveryOutcomeBytes},
		{Name: "indexes", MaxBytes: maxPublicationIndexBytes},
		{Name: "claim-ledgers", MaxBytes: maxPublicationLedgerBytes},
		{Name: "pii-projections", MaxBytes: maxPublicationProjectionBytes},
		{Name: "render-inspections", MaxBytes: maxPublicationProjectionBytes},
		{Name: "artifacts", MaxBytes: maxPublishedArtifactBytes},
	}
}

// PrepareOriginalObservationV1 observes all raw entries without opening a writable store.
func PrepareOriginalObservationV1(ctx context.Context, root string, access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority, originals ...*finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*finalauthorityadapter.OriginalFixedOwnerObservationV1, error) {
	return finalauthorityadapter.PrepareOriginalFixedOwnerObservationV1(ctx, root, originalPublicationLeavesV1(), access, originals...)
}

// ValidateOriginalEntriesV1 checks a complete original or signed endpoint with the existing domain rules.
func ValidateOriginalEntriesV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1, authority *finalauthorityadapter.AnchoredFileAuthority, localCheck func(string, string) error, creates *finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) error {
	return finalauthorityadapter.ValidateOriginalDomainEntriesV1(ctx, files, originalPublicationLeavesV1(), authority, localCheck, creates, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		return (&PreparedRecoveryV1{}).validateDomainSemantics(ctx, visit, materials, check)
	})
}
