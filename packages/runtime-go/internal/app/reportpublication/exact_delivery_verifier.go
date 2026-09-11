package reportpublication

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// exactDeliveryGraphV1 is a raw-value-free snapshot of one exact
// durable publication suffix. Artifact bytes are verified and zeroed while
// resolving the graph and can never escape through this value.
type exactDeliveryGraphV1 struct {
	Projection          domainpublication.ReportDeliveryProjectionV1
	Completion          domainpublication.ReportStageCompletionV1
	Decision            domainpublication.ReportDeliveryDecisionV1
	GrantSettlement     domainpublication.ReportGrantSettlementV1
	StageReceipt        domainpendingwork.PendingWorkReceiptV1
	StageDisposition    domainpendingwork.PendingWorkDispositionV1
	Attempt             domainpublication.PublicationAttemptV1
	Candidate           domainpublication.PublicationReceiptV1
	Index               domainpublication.PublicationIndexV1
	Intent              domainauthority.MonotonicAdvanceIntentV2
	AuthoritySettlement domainauthority.MonotonicAdvanceSettlementV2
	Selection           domainpublication.PublicationCommitSelectionV1
	Commit              domainpublication.PublicationCommitReceiptV1
	PreviousBundle      domainevidence.EvidenceAuthorityBundleV1
	CommittedBundle     domainevidence.EvidenceAuthorityBundleV1
	Observation         evidenceauthorityport.ObservationBundle
	Ledger              domainpublication.ClaimLedgerV1
	PIIProjection       domainpublication.PIIProjectionV1
	Inspection          domainpublication.RenderInspectionV1
	ControlledMetadata  *domainpii.ControlledPIIArtifactMetadataV1
}

type exactProjectedDeliverySelectorV1 struct {
	CurrentSecurityContext  *domainsecurity.TurnSecurityContext
	DeliveryID              string
	OutcomeRecordDigest     string
	PublicationCommitDigest string
	ThreadID                string
	TurnID                  string
	ContextDigest           string
	CaseBindingHash         string
	ContextEpoch            uint64
	DatasetSnapshotID       string
	SourceManifestHash      string
}

type exactPublicationCommitResolverConfigV1 struct {
	InstallationID string
	EnrollmentID   string
	Authority      finalauthorityport.Verifier
	WitnessKeyID   string
	WitnessKey     []byte
	Bundles        evidenceauthorityport.BundleResolver
	Observations   evidenceauthorityport.ObservationResolver
}

func (authority *ProjectedDeliveryAuthorityV1) resolveExactProjectedGraphV1(
	ctx context.Context,
	selector exactProjectedDeliverySelectorV1,
	projection domainpublication.ReportDeliveryProjectionV1,
) (exactDeliveryGraphV1, error) {
	config := authority.config
	if projection.DeliveryID != selector.DeliveryID || projection.RecordDigest != selector.OutcomeRecordDigest ||
		projection.InstallationID != config.InstallationID || projection.EnrollmentID != config.EnrollmentID ||
		projection.CommitRecordDigest != selector.PublicationCommitDigest ||
		projection.ContextDigest != selector.ContextDigest ||
		projection.ThreadID != selector.ThreadID || projection.TurnID != selector.TurnID ||
		projection.CaseBindingHash != selector.CaseBindingHash ||
		projection.ContextEpoch != selector.ContextEpoch ||
		projection.DatasetSnapshotID != selector.DatasetSnapshotID ||
		projection.SourceManifestHash != selector.SourceManifestHash {
		return exactDeliveryGraphV1{}, ErrProjectedDeliveryIntegrity
	}
	completion, err := config.StageCompletions.Resolve(ctx, projection.CompletionID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	graph, err := authority.resolveExactCompletedGraphV1(ctx, selector, completion)
	if err != nil {
		return exactDeliveryGraphV1{}, err
	}
	if domainpublication.ValidateReportDeliveryProjectionGraphV1(
		projection,
		domainpublication.ReportDeliveryProjectionInputV1{
			Decision: graph.Decision, GrantSettlement: graph.GrantSettlement, StageReceipt: graph.StageReceipt,
			StageDisposition: graph.StageDisposition, StageCompletion: graph.Completion,
			AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
		},
	) != nil {
		return exactDeliveryGraphV1{}, ErrProjectedDeliveryIntegrity
	}
	graph.Projection = projection
	return graph, nil
}

// Both historical outcomes share the exact completed graph. A rejection is
// never converted into a projection to reach this verifier.
func (authority *ProjectedDeliveryAuthorityV1) resolveExactCompletedGraphV1(
	ctx context.Context,
	selector exactProjectedDeliverySelectorV1,
	completion domainpublication.ReportStageCompletionV1,
) (exactDeliveryGraphV1, error) {
	config := authority.config
	if domainpublication.ValidateReportStageCompletionV1(completion) != nil ||
		completion.InstallationID != config.InstallationID || completion.EnrollmentID != config.EnrollmentID ||
		completion.CommitRecordDigest != selector.PublicationCommitDigest {
		return exactDeliveryGraphV1{}, ErrProjectedDeliveryIntegrity
	}
	decision, err := config.Decisions.Resolve(ctx, completion.DecisionID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	securityContext := decision.Context
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		securityContext.ThreadID != selector.ThreadID || securityContext.TurnID != selector.TurnID ||
		securityContext.ContextDigest != selector.ContextDigest || securityContext.CaseBindingHash != selector.CaseBindingHash ||
		securityContext.ContextEpoch != selector.ContextEpoch || securityContext.DatasetSnapshotID != selector.DatasetSnapshotID ||
		securityContext.SourceManifestHash != selector.SourceManifestHash ||
		selector.CurrentSecurityContext != nil && securityContext != *selector.CurrentSecurityContext {
		return exactDeliveryGraphV1{}, ErrProjectedDeliveryIntegrity
	}
	grantSettlement, err := config.GrantSettlements.Resolve(ctx, completion.GrantSettlementID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	terminal, err := config.Pending.ResolveTrustedCompletedReportStageV1(ctx, completion.ReportStageWorkID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	stage, disposition := terminal.Receipt, terminal.Disposition
	attempt, err := config.Attempts.Resolve(ctx, decision.AttemptID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	candidate, err := config.Receipts.Resolve(ctx, decision.CandidateRecordDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	index, err := config.Indexes.Resolve(ctx, decision.PublicationIndexDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	selection, err := config.Selections.Resolve(ctx, decision.CommitSelectionID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	commit, err := config.Commits.Resolve(ctx, decision.CommitRecordDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	intent, err := config.Intents.ResolveIntent(ctx, attempt.AuthorityAdvanceMutationID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	authoritySettlement, err := config.Settlements.ResolveSettlement(ctx, attempt.AuthorityAdvanceMutationID)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	ledger, err := config.Ledgers.Resolve(ctx, decision.ClaimLedgerDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	piiProjection, err := config.Projections.Resolve(ctx, decision.PIIProjectionDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	inspection, err := config.Inspections.Resolve(ctx, decision.RenderInspectionDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	commitInput, previous, committed, observation, err := resolveExactPublicationCommitInputV1(
		ctx, selection, attempt, candidate, index, authoritySettlement.RecordDigest,
		exactPublicationCommitConfigFromProjectedDeliveryV1(config),
	)
	if err != nil || domainpublication.ValidatePublicationCommitReceiptExactV1(commit, commitInput) != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	artifact, err := config.Artifacts.ResolveExact(ctx, candidate.TargetIdentityDigest)
	if err != nil {
		return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(err)
	}
	defer clear(artifact)
	var controlledMetadata *domainpii.ControlledPIIArtifactMetadataV1
	if piiProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull {
		parsed, parseErr := domainpii.ControlledPIIArtifactMetadataFromBytesV1(artifact)
		stored, storeErr := config.ControlledMetadata.ResolveControlledMetadata(ctx, candidate.TargetIdentityDigest)
		if parseErr != nil || storeErr != nil || !reflect.DeepEqual(parsed, stored) ||
			domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
				parsed, securityContext, ledger.LedgerDigest, piiProjection.RulesetHash,
				candidate.TargetIdentityDigest, candidate.ReportSHA256, candidate.ReportByteLength,
				piiProjection.PreservedControlledFieldCount,
			) != nil || piiProjection.RestrictedFieldCount != piiProjection.PreservedControlledFieldCount {
			return exactDeliveryGraphV1{}, projectedDeliveryIntegrityV1(errors.Join(parseErr, storeErr))
		}
		value := parsed
		controlledMetadata = &value
	}
	publicKey := config.Authority.PublicKey()
	encodedPublicKey := base64.RawURLEncoding.EncodeToString(publicKey)
	if commit.RecordDigest != selector.PublicationCommitDigest ||
		domainpublication.ValidatePublicationAttemptForInstallationV1(
			attempt, config.InstallationID, config.EnrollmentID, config.Authority.KeyID(), publicKey,
		) != nil ||
		domainpublication.ValidatePublicationAttemptGraphV1(attempt, stage, candidate, index) != nil ||
		intent.InstallationID != config.InstallationID || intent.EnrollmentID != config.EnrollmentID ||
		intent.AuthorityKeyID != config.Authority.KeyID() || intent.AuthorityPublicKey != encodedPublicKey ||
		intent.RecordDigest != attempt.AuthorityAdvanceIntentDigest || intent.MutationID != attempt.AuthorityAdvanceMutationID ||
		intent.Root != domainauthority.AdvanceRootPublicationV2 || intent.Transition.EvidenceBundle == nil ||
		!reflect.DeepEqual(intent.Transition.EvidenceBundle.PreviousBundle, previous) ||
		!reflect.DeepEqual(intent.Transition.EvidenceBundle.NextBundle, committed) ||
		authoritySettlement.AuthorityKeyID != config.Authority.KeyID() ||
		authoritySettlement.AuthorityPublicKey != encodedPublicKey ||
		authoritySettlement.RecordDigest != decision.AuthorityAdvanceSettlementDigest ||
		authoritySettlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 ||
		domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(authoritySettlement, intent, nil) != nil ||
		domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil ||
		domainpublication.ValidatePublicationCommitReceiptMaterialsV1(commit, candidate, index) != nil ||
		domainpublication.ValidateReportDeliveryDecisionDurableGraphV1(
			decision, attempt, candidate, index, selection, commit, ledger, piiProjection, inspection,
		) != nil ||
		domainpublication.ValidateReportGrantSettlementDecisionV1(grantSettlement, decision) != nil ||
		domainpublication.ValidateReportGrantSettlementGraphV1(
			grantSettlement,
			domainpublication.ReportGrantSettlementInputV1{
				Decision: decision, Grant: terminal.Grant,
				ActiveRegistry: terminal.ActiveRegistry, SettledRegistry: terminal.SettledRegistry,
				ResultItemID: terminal.ResultItemID, ResultItemDigest: terminal.ResultItemDigest,
				SettledAt: terminal.SettledAt, AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: publicKey,
			},
		) != nil ||
		domainpublication.ValidateReportStageCompletionGraphV1(
			completion,
			domainpublication.ReportStageCompletionInputV1{
				Decision: decision, GrantSettlement: grantSettlement, StageReceipt: stage, StageDisposition: disposition,
				AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: publicKey,
			},
		) != nil ||
		validateRestartSuccessfulReportResultItemV1(terminal.ResultItem, decision) != nil ||
		uint64(len(artifact)) != candidate.ReportByteLength || domainsecurity.SHA256Hex(artifact) != candidate.ReportSHA256 ||
		domainpublication.ValidatePublicationReceiptMaterialsV1(candidate, ledger, piiProjection, inspection) != nil {
		return exactDeliveryGraphV1{}, ErrProjectedDeliveryIntegrity
	}
	return exactDeliveryGraphV1{
		Completion: completion, Decision: decision, GrantSettlement: grantSettlement,
		StageReceipt: stage, StageDisposition: disposition, Attempt: attempt, Candidate: candidate, Index: index,
		Intent: intent, AuthoritySettlement: authoritySettlement, Selection: selection, Commit: commit,
		PreviousBundle: previous, CommittedBundle: committed, Observation: observation,
		Ledger: ledger, PIIProjection: piiProjection, Inspection: inspection, ControlledMetadata: controlledMetadata,
	}, nil
}

func (authority *ProjectedDeliveryAuthorityV1) resolveCurrentProjectedWithinSnapshotV1(
	ctx context.Context,
	selector publicationport.ProjectedDeliverySelectorV1,
	use func(domainpublication.ReportDeliveryProjectionV1) error,
) (domainpublication.ReportDeliveryProjectionV1, error) {
	var resolved domainpublication.ReportDeliveryProjectionV1
	err := withWitnessedSnapshotAuthorityV2(ctx, authority.config.Evidence, selector.SecurityContext, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		outcome, err := authority.config.DeliveryOutcomes.ResolveOutcome(ctx, selector.DeliveryID)
		if errors.Is(err, publicationport.ErrNotFound) {
			return ErrProjectedDeliveryNotFound
		}
		if err != nil || domainpublication.ValidateReportDeliveryOutcomeV1(outcome) != nil ||
			domainpublication.ReportDeliveryOutcomeID(outcome) != selector.DeliveryID ||
			domainpublication.ReportDeliveryOutcomeRecordDigest(outcome) != selector.OutcomeRecordDigest {
			return projectedDeliveryIntegrityV1(err)
		}
		if err := verifyTrustedDeliveryOutcomeV1(ctx, authority.config.Authority, outcome); err != nil {
			return projectedDeliveryIntegrityV1(err)
		}
		if outcome.Kind == domainpublication.ReportDeliveryOutcomeRejectedV1 {
			return ErrProjectedDeliveryRejected
		}
		if outcome.Kind != domainpublication.ReportDeliveryOutcomeProjectedV1 || outcome.Projection == nil {
			return ErrProjectedDeliveryIntegrity
		}
		projection := *outcome.Projection
		graph, err := authority.resolveExactProjectedGraphV1(ctx, exactProjectedDeliverySelectorFromCurrentV1(selector), projection)
		if err != nil {
			return err
		}
		if snapshot.Context != selector.SecurityContext ||
			validateExactFreshPublicationHeadV1(snapshot.Head, authority.config) != nil ||
			domainevidence.ValidateEvidenceRegistryAuthorityIndexForInstallationV2(
				snapshot.RootIndex, authority.config.InstallationID, authority.config.EnrollmentID,
				authority.config.Authority.KeyID(), authority.config.Authority.PublicKey(),
			) != nil ||
			domainevidence.ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(
				snapshot.RootIndex, snapshot.Head.Bundle.EvidenceRegistryIndexDigest,
				snapshot.Head.Bundle.EvidenceRegistryCount,
			) != nil {
			return ErrProjectedDeliveryIntegrity
		}
		ancestor, err := exactPublicationBundleIsAncestorV1(
			ctx, snapshot.Head.Bundle, graph.CommittedBundle, authority.config,
		)
		if err != nil || !ancestor {
			return projectedDeliveryIntegrityV1(err)
		}
		allowControlled := graph.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull
		if err := validateLedgerSnapshotV1(
			ctx, snapshot, graph.Decision.ReportVariant, graph.Ledger, allowControlled,
		); err != nil {
			return projectedDeliveryIntegrityV1(err)
		}
		if allowControlled {
			if graph.ControlledMetadata == nil || authority.config.PIIAuthority.ValidateControlledArtifactCurrentWithinSnapshot(
				ctx, selector.SecurityContext, graph.PIIProjection, *graph.ControlledMetadata, snapshot, capability,
			) != nil {
				return ErrProjectedDeliveryIntegrity
			}
		}
		stable, err := authority.config.DeliveryOutcomes.ResolveOutcome(ctx, selector.DeliveryID)
		if err != nil || !reflect.DeepEqual(stable, outcome) {
			return projectedDeliveryIntegrityV1(err)
		}
		if use != nil {
			useErr := use(projection)
			stableAfterUse, err := authority.config.DeliveryOutcomes.ResolveOutcome(ctx, selector.DeliveryID)
			if err != nil || !reflect.DeepEqual(stableAfterUse, outcome) {
				return projectedDeliveryIntegrityV1(err)
			}
			if useErr != nil {
				return &projectedDeliveryUseErrorV1{cause: useErr}
			}
		}
		resolved = projection
		return nil
	})
	if err != nil {
		return domainpublication.ReportDeliveryProjectionV1{}, err
	}
	return resolved, nil
}

func resolveExactPublicationCommitInputV1(
	ctx context.Context,
	selection domainpublication.PublicationCommitSelectionV1,
	attempt domainpublication.PublicationAttemptV1,
	candidate domainpublication.PublicationReceiptV1,
	index domainpublication.PublicationIndexV1,
	settlementDigest string,
	config exactPublicationCommitResolverConfigV1,
) (
	domainpublication.PublicationCommitReceiptInputV1,
	domainevidence.EvidenceAuthorityBundleV1,
	domainevidence.EvidenceAuthorityBundleV1,
	evidenceauthorityport.ObservationBundle,
	error,
) {
	emptyInput := domainpublication.PublicationCommitReceiptInputV1{}
	if domainpublication.ValidatePublicationCommitSelectionMaterialsV1(
		selection, attempt, candidate, index, settlementDigest,
	) != nil {
		return emptyInput, domainevidence.EvidenceAuthorityBundleV1{}, domainevidence.EvidenceAuthorityBundleV1{}, evidenceauthorityport.ObservationBundle{}, ErrProjectedDeliveryIntegrity
	}
	keyID, publicKey, signature, err := domainpublication.PublicationCommitSelectionAuthorityMaterialV1(selection)
	if err != nil || keyID != config.Authority.KeyID() || !reflect.DeepEqual(publicKey, config.Authority.PublicKey()) ||
		config.Authority.VerifyTrusted(
			ctx, keyID, publicKey, domainpublication.PublicationCommitSelectionSigningBytesV1(selection), signature,
		) != nil {
		return emptyInput, domainevidence.EvidenceAuthorityBundleV1{}, domainevidence.EvidenceAuthorityBundleV1{}, evidenceauthorityport.ObservationBundle{}, ErrProjectedDeliveryIntegrity
	}
	previous, err := config.Bundles.Resolve(ctx, selection.PreviousEvidenceBundleDigest)
	if err != nil || previous.RecordDigest != selection.PreviousEvidenceBundleDigest {
		return emptyInput, domainevidence.EvidenceAuthorityBundleV1{}, domainevidence.EvidenceAuthorityBundleV1{}, evidenceauthorityport.ObservationBundle{}, projectedDeliveryIntegrityV1(err)
	}
	committed, err := config.Bundles.Resolve(ctx, selection.CommittedEvidenceBundleDigest)
	if err != nil || committed.RecordDigest != selection.CommittedEvidenceBundleDigest ||
		domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, committed) != nil {
		return emptyInput, domainevidence.EvidenceAuthorityBundleV1{}, domainevidence.EvidenceAuthorityBundleV1{}, evidenceauthorityport.ObservationBundle{}, projectedDeliveryIntegrityV1(err)
	}
	observation, err := config.Observations.Resolve(ctx, selection.ObservationDigest)
	if err != nil || !reflect.DeepEqual(observation.Bundle, committed) ||
		observation.Request.RequestDigest != selection.ObserveRequestDigest ||
		observation.Observation.ObservationDigest != selection.ObservationDigest {
		return emptyInput, domainevidence.EvidenceAuthorityBundleV1{}, domainevidence.EvidenceAuthorityBundleV1{}, evidenceauthorityport.ObservationBundle{}, projectedDeliveryIntegrityV1(err)
	}
	input := domainpublication.PublicationCommitReceiptInputV1{
		PreviousBundle: previous, CommittedBundle: committed,
		ObserveRequest: observation.Request, Observation: observation.Observation,
		Candidate: candidate, Index: index,
		InstallationID: config.InstallationID, EnrollmentID: config.EnrollmentID,
		AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
		WitnessKeyID: config.WitnessKeyID, WitnessPublicKey: config.WitnessKey,
	}
	if domainpublication.ValidatePublicationCommitSelectionExactV1(
		selection,
		domainpublication.PublicationCommitSelectionInputV1{
			Attempt: attempt, SettlementDigest: settlementDigest, CommitInput: input,
		},
	) != nil {
		return emptyInput, domainevidence.EvidenceAuthorityBundleV1{}, domainevidence.EvidenceAuthorityBundleV1{}, evidenceauthorityport.ObservationBundle{}, ErrProjectedDeliveryIntegrity
	}
	return input, previous, committed, observation, nil
}

func exactPublicationCommitConfigFromProjectedDeliveryV1(
	config ProjectedDeliveryAuthorityConfigV1,
) exactPublicationCommitResolverConfigV1 {
	return exactPublicationCommitResolverConfigV1{
		InstallationID: config.InstallationID, EnrollmentID: config.EnrollmentID,
		Authority: config.Authority, WitnessKeyID: config.WitnessKeyID, WitnessKey: config.WitnessKey,
		Bundles: config.Bundles, Observations: config.Observations,
	}
}

func exactProjectedDeliverySelectorFromCurrentV1(
	selector publicationport.ProjectedDeliverySelectorV1,
) exactProjectedDeliverySelectorV1 {
	securityContext := selector.SecurityContext
	return exactProjectedDeliverySelectorV1{
		CurrentSecurityContext: &securityContext,
		DeliveryID:             selector.DeliveryID, OutcomeRecordDigest: selector.OutcomeRecordDigest,
		PublicationCommitDigest: selector.PublicationCommitDigest,
		ThreadID:                securityContext.ThreadID, TurnID: securityContext.TurnID,
		ContextDigest: securityContext.ContextDigest, CaseBindingHash: securityContext.CaseBindingHash,
		ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		SourceManifestHash: securityContext.SourceManifestHash,
	}
}

func exactProjectedDeliverySelectorFromHistoricalV1(
	selector publicationport.HistoricalProjectedDeliverySelectorV1,
) exactProjectedDeliverySelectorV1 {
	return exactProjectedDeliverySelectorV1{
		DeliveryID: selector.DeliveryID, OutcomeRecordDigest: selector.OutcomeRecordDigest,
		PublicationCommitDigest: selector.PublicationCommitDigest,
		ThreadID:                selector.ThreadID, TurnID: selector.TurnID, ContextDigest: selector.ContextDigest,
		CaseBindingHash: selector.CaseBindingHash, ContextEpoch: selector.ContextEpoch,
		DatasetSnapshotID: selector.DatasetSnapshotID, SourceManifestHash: selector.SourceManifestHash,
	}
}

func validateExactFreshPublicationHeadV1(
	head evidenceauthorityport.FreshHead,
	config ProjectedDeliveryAuthorityConfigV1,
) error {
	if !head.HasBundle || domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
		head.Bundle, config.InstallationID, config.EnrollmentID,
		config.Authority.KeyID(), config.Authority.PublicKey(),
	) != nil || domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(head.Bundle, head.Observation.Checkpoint) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			head.Observation, head.Request, config.InstallationID, config.Authority.KeyID(), config.Authority.PublicKey(),
			config.EnrollmentID, config.WitnessKeyID, config.WitnessKey,
		) != nil {
		return ErrProjectedDeliveryIntegrity
	}
	return nil
}

func exactPublicationBundleIsAncestorV1(
	ctx context.Context,
	current domainevidence.EvidenceAuthorityBundleV1,
	ancestor domainevidence.EvidenceAuthorityBundleV1,
	config ProjectedDeliveryAuthorityConfigV1,
) (bool, error) {
	if current.Generation < ancestor.Generation {
		return false, nil
	}
	cursor := current
	for depth := 0; cursor.Generation > ancestor.Generation; depth++ {
		if depth >= maxPublicationIndexDepthV1 {
			return false, ErrProjectedDeliveryIntegrity
		}
		previous, err := config.Bundles.Resolve(ctx, cursor.PreviousBundleDigest)
		if err != nil || previous.RecordDigest != cursor.PreviousBundleDigest ||
			domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
				previous, config.InstallationID, config.EnrollmentID,
				config.Authority.KeyID(), config.Authority.PublicKey(),
			) != nil || domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, cursor) != nil {
			return false, projectedDeliveryIntegrityV1(err)
		}
		cursor = previous
	}
	return reflect.DeepEqual(cursor, ancestor), nil
}

func projectedDeliveryIntegrityV1(err error) error {
	if err == nil {
		return ErrProjectedDeliveryIntegrity
	}
	return errors.Join(ErrProjectedDeliveryIntegrity, err)
}
