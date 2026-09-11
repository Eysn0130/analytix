package reportpublication

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authoritystoreport "analytix.local/runtime-go/internal/ports/authorityadvance"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

type RestartPreflightConfigV1 struct {
	Pending          pendingworkapp.TrustedInventoryV1
	Attempts         publicationport.AttemptInventoryStore
	Receipts         restartReceiptStoreV1
	Indexes          restartIndexStoreV1
	Commits          publicationport.CommitReceiptInventoryStore
	Selections       publicationport.CommitSelectionInventoryStore
	Decisions        publicationport.DeliveryDecisionInventoryStore
	GrantSettlements publicationport.ReportGrantSettlementInventoryStore
	StageCompletions publicationport.ReportStageCompletionInventoryStore
	DeliveryOutcomes publicationport.DeliveryOutcomeInventoryStore
	Ledgers          restartClaimLedgerResolverV1
	Projections      restartPIIProjectionResolverV1
	Inspections      restartRenderInspectionResolverV1
	Threads          restartThreadReaderV1
	Intents          authoritystoreport.IntentInventoryStore
	Settlements      authoritystoreport.SettlementInventoryStore
	Authority        finalauthorityport.Authority
}

type restartReceiptStoreV1 interface {
	publicationport.ReceiptResolver
	publicationport.ReceiptInventoryStore
}

type restartIndexStoreV1 interface {
	publicationport.IndexResolver
	publicationport.IndexInventoryStore
}

type restartClaimLedgerResolverV1 interface {
	Resolve(context.Context, string) (domainpublication.ClaimLedgerV1, error)
}

type restartPIIProjectionResolverV1 interface {
	Resolve(context.Context, string) (domainpublication.PIIProjectionV1, error)
}

type restartRenderInspectionResolverV1 interface {
	Resolve(context.Context, string) (domainpublication.RenderInspectionV1, error)
}

type RestartAttemptStateV1 string

const (
	RestartAttemptReservedV1             RestartAttemptStateV1 = "reserved"
	RestartAttemptCandidateDurableV1     RestartAttemptStateV1 = "candidate_durable"
	RestartAttemptMaterialsDurableV1     RestartAttemptStateV1 = "materials_durable"
	RestartAttemptIntentDurableV1        RestartAttemptStateV1 = "intent_durable"
	RestartAttemptCommittedSettlementV1  RestartAttemptStateV1 = "committed_settlement"
	RestartAttemptCommitSelectionV1      RestartAttemptStateV1 = "commit_selection_durable"
	RestartAttemptSupersededSettlementV1 RestartAttemptStateV1 = "superseded_settlement"
	RestartAttemptCommitReceiptV1        RestartAttemptStateV1 = "commit_receipt_durable"
	RestartAttemptDeliveryDecisionV1     RestartAttemptStateV1 = "delivery_decision_durable"
	RestartAttemptGrantSettlementV1      RestartAttemptStateV1 = "grant_settlement_durable"
	RestartAttemptStageDispositionV1     RestartAttemptStateV1 = "stage_disposition_completed"
	RestartAttemptStageCompletionV1      RestartAttemptStateV1 = "stage_completion_durable"
	RestartAttemptDeliveryProjectionV1   RestartAttemptStateV1 = "delivery_projection_durable"
	RestartAttemptDeliveryRejectionV1    RestartAttemptStateV1 = "delivery_rejection_durable"
	RestartAttemptDeliveryBlockedV1      RestartAttemptStateV1 = "delivery_blocked"
	RestartAttemptAbortedV1              RestartAttemptStateV1 = "aborted_before_witness"
)

type RestartAttemptV1 struct {
	Attempt         domainpublication.PublicationAttemptV1
	Stage           domainpendingwork.PendingWorkReceiptV1
	State           RestartAttemptStateV1
	Candidate       *domainpublication.PublicationReceiptV1
	Index           *domainpublication.PublicationIndexV1
	Intent          *domainauthority.MonotonicAdvanceIntentV2
	Settlement      *domainauthority.MonotonicAdvanceSettlementV2
	Commit          *domainpublication.PublicationCommitReceiptV1
	Selection       *domainpublication.PublicationCommitSelectionV1
	Decision        *domainpublication.ReportDeliveryDecisionV1
	GrantSettlement *domainpublication.ReportGrantSettlementV1
	StageCompletion *domainpublication.ReportStageCompletionV1
	DeliveryOutcome *domainpublication.ReportDeliveryOutcomeV1
	Disposition     *domainpendingwork.PendingWorkDispositionV1
}

type RestartPlanV1 struct {
	Attempts   []RestartAttemptV1
	sealDigest string
}

// PreflightRestartV1 reads and validates the complete cross-owner outbox graph
// without performing recovery, witness calls, delivery, or pending-work
// mutation. Missing suffix records are valid crash cuts; mismatched or orphan
// suffix records are integrity failures.
func PreflightRestartV1(ctx context.Context, config RestartPreflightConfigV1) (RestartPlanV1, error) {
	if ctx == nil || config.Attempts == nil || config.Receipts == nil || config.Indexes == nil ||
		config.Commits == nil || config.Selections == nil || config.Decisions == nil || config.GrantSettlements == nil || config.StageCompletions == nil || config.DeliveryOutcomes == nil ||
		config.Ledgers == nil || config.Projections == nil || config.Inspections == nil || config.Threads == nil ||
		config.Intents == nil || config.Settlements == nil || config.Authority == nil {
		return RestartPlanV1{}, errors.New("report publication restart preflight dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return RestartPlanV1{}, err
	}
	coreValidator, err := newAttemptCoreValidatorV1(config.Pending, config.Authority)
	if err != nil {
		return RestartPlanV1{}, err
	}
	receiptByDigest := map[string]domainpublication.PublicationReceiptV1{}
	if err := config.Receipts.VisitReceipts(ctx, func(receipt domainpublication.PublicationReceiptV1) error {
		if _, duplicate := receiptByDigest[receipt.RecordDigest]; duplicate {
			return errors.New("report publication restart inventory repeats a receipt")
		}
		receiptByDigest[receipt.RecordDigest] = receipt
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	indexByDigest := map[string]domainpublication.PublicationIndexV1{}
	if err := config.Indexes.VisitIndexes(ctx, func(index domainpublication.PublicationIndexV1) error {
		if _, duplicate := indexByDigest[index.IndexDigest]; duplicate {
			return errors.New("report publication restart inventory repeats an index")
		}
		indexByDigest[index.IndexDigest] = index
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	allIntentByMutation := map[string]domainauthority.MonotonicAdvanceIntentV2{}
	intentByMutation := map[string]domainauthority.MonotonicAdvanceIntentV2{}
	if err := config.Intents.VisitIntents(ctx, func(intent domainauthority.MonotonicAdvanceIntentV2) error {
		if _, duplicate := allIntentByMutation[intent.MutationID]; duplicate {
			return errors.New("report publication restart inventory repeats an intent")
		}
		allIntentByMutation[intent.MutationID] = intent
		if intent.Root == domainauthority.AdvanceRootPublicationV2 {
			intentByMutation[intent.MutationID] = intent
		}
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	allSettlementByMutation := map[string]domainauthority.MonotonicAdvanceSettlementV2{}
	settlementByMutation := map[string]domainauthority.MonotonicAdvanceSettlementV2{}
	if err := config.Settlements.VisitSettlements(ctx, func(settlement domainauthority.MonotonicAdvanceSettlementV2) error {
		intent, found := allIntentByMutation[settlement.MutationID]
		if !found {
			return errors.New("report publication restart settlement is orphaned")
		}
		if _, duplicate := allSettlementByMutation[settlement.MutationID]; duplicate {
			return errors.New("report publication restart inventory repeats a settlement")
		}
		if domainauthority.ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent) != nil {
			return errors.New("report publication restart settlement is mismatched")
		}
		allSettlementByMutation[settlement.MutationID] = settlement
		_, publication := intentByMutation[settlement.MutationID]
		if !publication {
			return nil
		}
		settlementByMutation[settlement.MutationID] = settlement
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	commitByCandidate := map[string]domainpublication.PublicationCommitReceiptV1{}
	if err := config.Commits.VisitCommitReceipts(ctx, func(commit domainpublication.PublicationCommitReceiptV1) error {
		if _, duplicate := commitByCandidate[commit.CandidateRecordDigest]; duplicate {
			return errors.New("report publication restart inventory repeats a candidate commit")
		}
		commitByCandidate[commit.CandidateRecordDigest] = commit
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	selectionByAttempt := map[string]domainpublication.PublicationCommitSelectionV1{}
	if err := config.Selections.VisitCommitSelections(ctx, func(selection domainpublication.PublicationCommitSelectionV1) error {
		if _, duplicate := selectionByAttempt[selection.AttemptID]; duplicate {
			return errors.New("report publication restart inventory repeats an attempt selection")
		}
		selectionByAttempt[selection.AttemptID] = selection
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	decisionByAttempt := map[string]domainpublication.ReportDeliveryDecisionV1{}
	decisionByCommit := map[string]domainpublication.ReportDeliveryDecisionV1{}
	decisionByID := map[string]domainpublication.ReportDeliveryDecisionV1{}
	if err := config.Decisions.VisitDeliveryDecisions(ctx, func(decision domainpublication.ReportDeliveryDecisionV1) error {
		if _, duplicate := decisionByID[decision.DecisionID]; duplicate {
			return errors.New("report publication restart inventory repeats a delivery decision")
		}
		if _, duplicate := decisionByAttempt[decision.AttemptID]; duplicate {
			return errors.New("report publication restart inventory repeats an attempt decision")
		}
		if _, duplicate := decisionByCommit[decision.CommitRecordDigest]; duplicate {
			return errors.New("report publication restart inventory repeats a commit decision")
		}
		decisionByID[decision.DecisionID] = decision
		decisionByAttempt[decision.AttemptID] = decision
		decisionByCommit[decision.CommitRecordDigest] = decision
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	grantSettlementByDecision := map[string]domainpublication.ReportGrantSettlementV1{}
	grantSettlementByID := map[string]domainpublication.ReportGrantSettlementV1{}
	if err := config.GrantSettlements.VisitGrantSettlements(ctx, func(settlement domainpublication.ReportGrantSettlementV1) error {
		if _, duplicate := grantSettlementByID[settlement.SettlementID]; duplicate {
			return errors.New("report publication restart inventory repeats a grant settlement")
		}
		if _, duplicate := grantSettlementByDecision[settlement.DecisionID]; duplicate {
			return errors.New("report publication restart inventory repeats a decision grant settlement")
		}
		grantSettlementByID[settlement.SettlementID] = settlement
		grantSettlementByDecision[settlement.DecisionID] = settlement
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	stageCompletionByDecision := map[string]domainpublication.ReportStageCompletionV1{}
	stageCompletionByID := map[string]domainpublication.ReportStageCompletionV1{}
	if err := config.StageCompletions.VisitStageCompletions(ctx, func(completion domainpublication.ReportStageCompletionV1) error {
		if _, duplicate := stageCompletionByID[completion.CompletionID]; duplicate {
			return errors.New("report publication restart inventory repeats a stage completion")
		}
		if _, duplicate := stageCompletionByDecision[completion.DecisionID]; duplicate {
			return errors.New("report publication restart inventory repeats a decision stage completion")
		}
		stageCompletionByID[completion.CompletionID] = completion
		stageCompletionByDecision[completion.DecisionID] = completion
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	deliveryOutcomeByCompletion := map[string]domainpublication.ReportDeliveryOutcomeV1{}
	deliveryOutcomeByID := map[string]domainpublication.ReportDeliveryOutcomeV1{}
	if err := config.DeliveryOutcomes.VisitDeliveryOutcomes(ctx, func(outcome domainpublication.ReportDeliveryOutcomeV1) error {
		deliveryID := domainpublication.ReportDeliveryOutcomeID(outcome)
		completionID := domainpublication.ReportDeliveryOutcomeCompletionID(outcome)
		if _, duplicate := deliveryOutcomeByID[deliveryID]; duplicate {
			return errors.New("report publication restart inventory repeats a delivery outcome")
		}
		if _, duplicate := deliveryOutcomeByCompletion[completionID]; duplicate {
			return errors.New("report publication restart inventory repeats a completion delivery outcome")
		}
		deliveryOutcomeByID[deliveryID] = outcome
		deliveryOutcomeByCompletion[completionID] = outcome
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	plan := RestartPlanV1{Attempts: make([]RestartAttemptV1, 0)}
	usedReceipts := map[string]bool{}
	usedIndexes := map[string]bool{}
	usedIntents := map[string]bool{}
	usedSettlements := map[string]bool{}
	usedCommits := map[string]bool{}
	usedSelections := map[string]bool{}
	usedDecisions := map[string]bool{}
	usedGrantSettlements := map[string]bool{}
	usedStageCompletions := map[string]bool{}
	usedDeliveryOutcomes := map[string]bool{}
	var attempts []domainpublication.PublicationAttemptV1
	if err := config.Attempts.VisitAttempts(ctx, func(attempt domainpublication.PublicationAttemptV1) error {
		attempts = append(attempts, attempt)
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	// Durable material and thread reads cannot nest inside the attempts CAS visit.
	validateAttempt := func(attempt domainpublication.PublicationAttemptV1) error {
		stage, err := coreValidator.validate(attempt)
		if err != nil {
			return err
		}
		entry := RestartAttemptV1{Attempt: attempt, Stage: stage, State: RestartAttemptReservedV1}

		candidate, candidateFound := receiptByDigest[attempt.CandidateRecordDigest]
		index, indexFound := indexByDigest[attempt.PublicationIndexDigest]
		if candidateFound {
			entry.Candidate = &candidate
			usedReceipts[candidate.RecordDigest] = true
		}
		if indexFound {
			entry.Index = &index
			usedIndexes[index.IndexDigest] = true
		}
		if indexFound && !candidateFound {
			return errors.New("report publication restart index precedes its candidate")
		}
		if candidateFound && !indexFound {
			if domainpublication.ValidatePublicationAttemptCandidateV1(attempt, stage, candidate) != nil {
				return errors.New("report publication restart candidate is mismatched")
			}
			entry.State = RestartAttemptCandidateDurableV1
		}
		if candidateFound && indexFound {
			if domainpublication.ValidatePublicationAttemptGraphV1(attempt, stage, candidate, index) != nil {
				return errors.New("report publication restart attempt material graph is mismatched")
			}
			entry.State = RestartAttemptMaterialsDurableV1
		}

		intent, intentFound := intentByMutation[attempt.AuthorityAdvanceMutationID]
		if intentFound {
			if !candidateFound || !indexFound || intent.RecordDigest != attempt.AuthorityAdvanceIntentDigest ||
				intent.MutationID != attempt.AuthorityAdvanceMutationID || intent.Root != domainauthority.AdvanceRootPublicationV2 ||
				intent.Transition.EvidenceBundle == nil ||
				intent.Transition.EvidenceBundle.PreviousBundle.RecordDigest != attempt.ExpectedEvidenceBundleDigest ||
				intent.Transition.EvidenceBundle.NextBundle.RecordDigest != attempt.NextEvidenceBundleDigest {
				return errors.New("report publication restart intent is mismatched or precedes materials")
			}
			entry.Intent = &intent
			usedIntents[intent.MutationID] = true
			entry.State = RestartAttemptIntentDurableV1
		}

		settlement, settlementFound := settlementByMutation[attempt.AuthorityAdvanceMutationID]
		if settlementFound {
			if !intentFound || domainauthority.ValidateMonotonicAdvanceSettlementIntentHeaderV2(settlement, intent) != nil {
				return errors.New("report publication restart settlement is orphaned or mismatched")
			}
			entry.Settlement = &settlement
			usedSettlements[settlement.MutationID] = true
			switch settlement.Kind {
			case domainauthority.MonotonicAdvanceSettlementCommittedV2:
				if domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, nil) != nil {
					return errors.New("report publication restart committed settlement is invalid")
				}
				entry.State = RestartAttemptCommittedSettlementV1
			case domainauthority.MonotonicAdvanceSettlementSupersededV2:
				committedRange, rangeErr := restartCommittedRangeV2(settlement, allIntentByMutation, allSettlementByMutation)
				if rangeErr != nil || domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(settlement, intent, committedRange) != nil {
					return errors.New("report publication restart superseded settlement range is invalid")
				}
				entry.State = RestartAttemptSupersededSettlementV1
			default:
				return errors.New("report publication restart settlement kind is unknown")
			}
		}

		selection, selectionFound := selectionByAttempt[attempt.AttemptID]
		if selectionFound {
			if !candidateFound || !indexFound || !settlementFound ||
				settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 ||
				domainpublication.ValidatePublicationCommitSelectionMaterialsV1(
					selection, attempt, candidate, index, settlement.RecordDigest,
				) != nil {
				return errors.New("report publication restart selection is not backed by the exact committed attempt")
			}
			entry.Selection = &selection
			usedSelections[selection.SelectionID] = true
			entry.State = RestartAttemptCommitSelectionV1
		}

		if commit, found := commitByCandidate[attempt.CandidateRecordDigest]; found {
			if !candidateFound || !selectionFound || !settlementFound || settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 ||
				domainpublication.ValidatePublicationCommitReceiptMaterialsV1(commit, candidate, index) != nil ||
				domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil ||
				commit.CommittedEvidenceBundleDigest != attempt.NextEvidenceBundleDigest {
				return errors.New("report publication restart commit is not backed by the exact committed attempt")
			}
			entry.Commit = &commit
			usedCommits[commit.RecordDigest] = true
			entry.State = RestartAttemptCommitReceiptV1
		}
		if decision, found := decisionByAttempt[attempt.AttemptID]; found {
			if entry.Commit == nil || entry.Selection == nil || entry.Candidate == nil || entry.Index == nil ||
				decision.CommitRecordDigest != entry.Commit.RecordDigest {
				return errors.New("report publication restart decision precedes or mismatches its commit")
			}
			ledger, ledgerErr := config.Ledgers.Resolve(ctx, decision.ClaimLedgerDigest)
			projection, projectionErr := config.Projections.Resolve(ctx, decision.PIIProjectionDigest)
			inspection, inspectionErr := config.Inspections.Resolve(ctx, decision.RenderInspectionDigest)
			if ledgerErr != nil || projectionErr != nil || inspectionErr != nil ||
				domainpublication.ValidateReportDeliveryDecisionDurableGraphV1(
					decision, attempt, *entry.Candidate, *entry.Index, *entry.Selection, *entry.Commit,
					ledger, projection, inspection,
				) != nil {
				return errors.New("report publication restart delivery decision lost its exact durable graph")
			}
			entry.Decision = &decision
			usedDecisions[decision.DecisionID] = true
			entry.State = RestartAttemptDeliveryDecisionV1
			if grantSettlement, found := grantSettlementByDecision[decision.DecisionID]; found {
				if validateRestartGrantSettlementFromThreadV1(ctx, grantSettlement, decision, config.Threads) != nil {
					return errors.New("report publication restart grant settlement lost its exact durable result transition")
				}
				entry.GrantSettlement = &grantSettlement
				usedGrantSettlements[grantSettlement.SettlementID] = true
				entry.State = RestartAttemptGrantSettlementV1
			}
		}
		if disposition, found := config.Pending.Dispositions[attempt.ReportStageWorkID]; found {
			value := disposition
			entry.Disposition = &value
			switch disposition.Status {
			case domainpendingwork.StatusCompleted:
				if entry.Decision == nil || entry.GrantSettlement == nil ||
					domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, stage) != nil ||
					validateRestartSuccessfulReportResultV1(ctx, *entry.GrantSettlement, *entry.Decision, config.Threads) != nil {
					return errors.New("report publication restart completed stage lacks an exact successful grant settlement")
				}
				entry.State = RestartAttemptStageDispositionV1
				if completion, completionFound := stageCompletionByDecision[entry.Decision.DecisionID]; completionFound {
					if domainpublication.ValidateReportStageCompletionGraphV1(
						completion,
						domainpublication.ReportStageCompletionInputV1{
							Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
							StageReceipt: stage, StageDisposition: disposition,
							AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
						},
					) != nil {
						return errors.New("report publication restart stage completion lost its exact completed-stage graph")
					}
					entry.StageCompletion = &completion
					usedStageCompletions[completion.CompletionID] = true
					entry.State = RestartAttemptStageCompletionV1
					if outcome, outcomeFound := deliveryOutcomeByCompletion[completion.CompletionID]; outcomeFound {
						if err := validateRestartDeliveryOutcomeGraphV1(
							outcome, *entry.Decision, *entry.GrantSettlement, stage, disposition, completion,
							config.Authority.KeyID(), config.Authority.PublicKey(),
						); err != nil {
							return errors.New("report publication restart delivery outcome lost its exact completed-stage graph")
						}
						entry.DeliveryOutcome = &outcome
						usedDeliveryOutcomes[domainpublication.ReportDeliveryOutcomeID(outcome)] = true
						if outcome.Kind == domainpublication.ReportDeliveryOutcomeProjectedV1 {
							entry.State = RestartAttemptDeliveryProjectionV1
						} else {
							entry.State = RestartAttemptDeliveryRejectionV1
						}
					}
				}
			case domainpendingwork.StatusFailed, domainpendingwork.StatusCancelled, domainpendingwork.StatusExpired,
				domainpendingwork.StatusRejected, domainpendingwork.StatusRestartInvalid, domainpendingwork.StatusStaleContext,
				domainpendingwork.StatusOutcomeUnknown:
				if entry.Intent == nil && entry.Settlement == nil && entry.Selection == nil && entry.Commit == nil && entry.Decision == nil && entry.GrantSettlement == nil {
					entry.State = RestartAttemptAbortedV1
				} else {
					entry.State = RestartAttemptDeliveryBlockedV1
				}
			default:
				return errors.New("report publication restart stage disposition is not safely terminal")
			}
		}
		plan.Attempts = append(plan.Attempts, entry)
		return nil
	}
	for _, attempt := range attempts {
		if err := ctx.Err(); err != nil {
			return RestartPlanV1{}, err
		}
		if err := validateAttempt(attempt); err != nil {
			return RestartPlanV1{}, err
		}
	}
	if len(usedDecisions) != len(decisionByID) {
		return RestartPlanV1{}, errors.New("report publication restart inventory contains an orphan delivery decision")
	}
	if len(usedGrantSettlements) != len(grantSettlementByID) {
		return RestartPlanV1{}, errors.New("report publication restart inventory contains an orphan grant settlement")
	}
	if len(usedStageCompletions) != len(stageCompletionByID) {
		return RestartPlanV1{}, errors.New("report publication restart inventory contains an orphan stage completion")
	}
	if len(usedDeliveryOutcomes) != len(deliveryOutcomeByID) {
		return RestartPlanV1{}, errors.New("report publication restart inventory contains an orphan delivery outcome")
	}
	if len(usedReceipts) != len(receiptByDigest) || len(usedIndexes) != len(indexByDigest) ||
		len(usedIntents) != len(intentByMutation) || len(usedSettlements) != len(settlementByMutation) ||
		len(usedSelections) != len(selectionByAttempt) || len(usedCommits) != len(commitByCandidate) {
		return RestartPlanV1{}, errors.New("report publication restart inventory contains an orphan record")
	}
	sort.Slice(plan.Attempts, func(i, j int) bool {
		return plan.Attempts[i].Attempt.AttemptID < plan.Attempts[j].Attempt.AttemptID
	})
	plan.sealDigest = restartPlanDigestV1(plan)
	return snapshotRestartPlanV1(plan)
}

func validateRestartDeliveryOutcomeGraphV1(
	outcome domainpublication.ReportDeliveryOutcomeV1,
	decision domainpublication.ReportDeliveryDecisionV1,
	grantSettlement domainpublication.ReportGrantSettlementV1,
	stage domainpendingwork.PendingWorkReceiptV1,
	disposition domainpendingwork.PendingWorkDispositionV1,
	completion domainpublication.ReportStageCompletionV1,
	authorityKeyID string,
	authorityPublicKey []byte,
) error {
	if domainpublication.ValidateReportDeliveryOutcomeCompletionV1(outcome, completion) != nil {
		return errors.New("report delivery outcome completion binding is invalid")
	}
	switch outcome.Kind {
	case domainpublication.ReportDeliveryOutcomeProjectedV1:
		return domainpublication.ValidateReportDeliveryProjectionGraphV1(
			*outcome.Projection,
			domainpublication.ReportDeliveryProjectionInputV1{
				Decision: decision, GrantSettlement: grantSettlement,
				StageReceipt: stage, StageDisposition: disposition, StageCompletion: completion,
				AuthorityKeyID: authorityKeyID, AuthorityPublicKey: authorityPublicKey,
			},
		)
	case domainpublication.ReportDeliveryOutcomeRejectedV1:
		rejection := *outcome.Rejection
		return domainpublication.ValidateReportDeliveryRejectionGraphV1(
			rejection,
			domainpublication.ReportDeliveryRejectionInputV1{
				Decision: decision, GrantSettlement: grantSettlement,
				StageReceipt: stage, StageDisposition: disposition, StageCompletion: completion,
				ReasonCode:                    rejection.ReasonCode,
				WitnessObservationDigest:      rejection.WitnessObservationDigest,
				EvidenceAuthorityBundleDigest: rejection.EvidenceAuthorityBundleDigest,
				EvidenceRegistryIndexDigest:   rejection.EvidenceRegistryIndexDigest,
				EvidenceRegistrySequence:      rejection.EvidenceRegistrySequence,
				EvidenceRegistryStateDigest:   rejection.EvidenceRegistryStateDigest,
				AuthorityKeyID:                authorityKeyID, AuthorityPublicKey: authorityPublicKey,
			},
		)
	default:
		return errors.New("report delivery outcome kind is invalid")
	}
}

func restartCommittedRangeV2(
	settlement domainauthority.MonotonicAdvanceSettlementV2,
	intents map[string]domainauthority.MonotonicAdvanceIntentV2,
	settlements map[string]domainauthority.MonotonicAdvanceSettlementV2,
) ([]domainauthority.MonotonicAdvanceCommittedStepV2, error) {
	if settlement.Superseded == nil || len(settlement.Superseded.RangeReferences) == 0 {
		return nil, errors.New("report publication restart superseded range is absent")
	}
	steps := make([]domainauthority.MonotonicAdvanceCommittedStepV2, 0, len(settlement.Superseded.RangeReferences))
	for _, reference := range settlement.Superseded.RangeReferences {
		intent, intentFound := intents[reference.MutationID]
		committed, settlementFound := settlements[reference.MutationID]
		if !intentFound || !settlementFound || committed.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 ||
			intent.Root != reference.Root || intent.RecordDigest != reference.IntentDigest || intent.RequestDigest != reference.RequestDigest ||
			committed.RecordDigest != reference.SettlementDigest || committed.Committed == nil ||
			committed.Committed.Receipt.ReceiptDigest != reference.ReceiptDigest ||
			intent.PreviousCheckpoint.CheckpointDigest != reference.PreviousCheckpointDigest ||
			committed.Committed.Receipt.Checkpoint.CheckpointDigest != reference.NextCheckpointDigest {
			return nil, errors.New("report publication restart superseded range reference is unresolved")
		}
		steps = append(steps, domainauthority.MonotonicAdvanceCommittedStepV2{Intent: intent, Settlement: committed})
	}
	return steps, nil
}

func restartPlanDigestV1(plan RestartPlanV1) string {
	body, err := restartPlanBodyV1(plan)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(append([]byte("analytix.report-publication-restart-plan/v1\x00"), body...))
}

func restartPlanBodyV1(plan RestartPlanV1) ([]byte, error) {
	plan.sealDigest = ""
	return json.Marshal(plan)
}

// snapshotRestartPlanV1 derives and verifies the seal from one immutable byte
// snapshot, then parses only those same bytes. No caller-owned slice or nested
// pointer survives into apply, so a post-preflight alias cannot mutate the
// effect graph after seal verification.
func snapshotRestartPlanV1(plan RestartPlanV1) (RestartPlanV1, error) {
	seal := plan.sealDigest
	body, err := restartPlanBodyV1(plan)
	if err != nil || !domainsecurity.IsSHA256Hex(seal) ||
		seal != domainsecurity.SHA256Hex(append([]byte("analytix.report-publication-restart-plan/v1\x00"), body...)) {
		return RestartPlanV1{}, ErrPublicationIntegrity
	}
	var snapshot RestartPlanV1
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return RestartPlanV1{}, ErrPublicationIntegrity
	}
	snapshot.sealDigest = seal
	return snapshot, nil
}
