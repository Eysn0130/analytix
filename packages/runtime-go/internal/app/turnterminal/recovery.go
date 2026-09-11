package turnterminal

import (
	"context"
	"errors"
	"reflect"
	"sort"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
)

type RestartRecoveryInputV1 struct {
	CompletionStore appturn.AcceptedFinalCompletionStore
	CASReader       finalauthorityport.AcceptedFinalCASReader
	// PrivateInventory contains current executable authority and current losing
	// contenders. AuditOnlyPrivateInventory is a disjoint, non-executable
	// inventory that must be verified for closure but can never be resumed,
	// backfilled, repaired, completed, or projected. Their union must equal the
	// durable private-final store exactly.
	PrivateInventory          []domainevidence.PrivateAcceptedFinalRecord
	AuditOnlyPrivateInventory []domainevidence.PrivateAcceptedFinalRecord
	Candidates                []domainevidence.PrivateAcceptedFinalRecord
}

type RestartRecoveryResultV1 struct {
	Complete               []CommitResultV1
	LegacyQuarantined      []domainevidence.PrivateAcceptedFinalRecord
	AuditOnly              []domainevidence.PrivateAcceptedFinalRecord
	NonExecutableAuditOnly []domainevidence.PrivateAcceptedFinalRecord
	ProviderAuditOnly      []domaincachetelemetry.ProviderTurnClosureV1
	// Preserved retains original held records without granting terminal,
	// quarantine, audit-only, replay or publication authority.
	Preserved []domainevidence.PrivateAcceptedFinalRecord
}

type restartRecoveryActionV1 uint8

const (
	restartRecoveryResumeV1 restartRecoveryActionV1 = iota + 1
	restartRecoveryCompleteV1
	restartRecoveryLegacyQuarantineV1
)

type restartRecoveryCandidateV1 struct {
	action              restartRecoveryActionV1
	privateFinal        domainevidence.PrivateAcceptedFinalRecord
	observation         domainevidence.AcceptedFinalCASObservationV1
	intent              domainturnterminal.TurnTerminalIntentV1
	providerClosure     domaincachetelemetry.ProviderTurnClosureV1
	acceptedDisposition domainevidence.AcceptedFinalDispositionRecord
	terminalDisposition domainturnterminal.TurnTerminalDispositionV1
}

type restartRecoveryInventoryV1 struct {
	knownByDigest           map[string]domainevidence.PrivateAcceptedFinalRecord
	auditOnlyByDigest       map[string]domainevidence.PrivateAcceptedFinalRecord
	intentByContext         map[string]domainturnterminal.TurnTerminalIntentV1
	intentByAcceptedFinal   map[string]domainturnterminal.TurnTerminalIntentV1
	closureByID             map[string]domaincachetelemetry.ProviderTurnClosureV1
	acceptedByFinal         map[string]domainevidence.AcceptedFinalDispositionRecord
	terminalByContext       map[string]domainturnterminal.TurnTerminalDispositionV1
	terminalByAcceptedFinal map[string]domainturnterminal.TurnTerminalDispositionV1
}

// RecoverV1 resumes only physically valid prefixes of I -> C -> P -> Daf ->
// Dt. A public winner without a pre-existing intent is legacy audit material;
// it is quarantined without backfilling a synthetic earlier chain.
func (coordinator *Coordinator) RecoverV1(ctx context.Context, input RestartRecoveryInputV1) (RestartRecoveryResultV1, error) {
	if coordinator == nil || coordinator.authority == nil || coordinator.privateFinals == nil || coordinator.terminals == nil ||
		coordinator.providerTurns == nil || input.CompletionStore == nil || input.CASReader == nil {
		return RestartRecoveryResultV1{}, errors.New("turn terminal restart recovery authority is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	preservedScope := coordinator.beginRestartRecoveryV1()
	candidates := append([]domainevidence.PrivateAcceptedFinalRecord(nil), input.Candidates...)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].SecurityContext.ContextDigest == candidates[j].SecurityContext.ContextDigest {
			return candidates[i].AcceptedFinal.RecordDigest < candidates[j].AcceptedFinal.RecordDigest
		}
		return candidates[i].SecurityContext.ContextDigest < candidates[j].SecurityContext.ContextDigest
	})
	for index, candidate := range candidates {
		if domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(candidate) != nil {
			return RestartRecoveryResultV1{}, errors.New("turn terminal restart candidate is invalid")
		}
		if index > 0 && candidates[index-1].SecurityContext.ContextDigest == candidate.SecurityContext.ContextDigest {
			return RestartRecoveryResultV1{}, errors.New("turn terminal restart candidates duplicate one frozen context")
		}
	}
	known := append([]domainevidence.PrivateAcceptedFinalRecord(nil), input.PrivateInventory...)
	if len(known) == 0 {
		known = append(known, candidates...)
	}
	nonExecutableAudit := append([]domainevidence.PrivateAcceptedFinalRecord(nil), input.AuditOnlyPrivateInventory...)
	preservedObservations := map[string]domainevidence.AcceptedFinalCASObservationV1{}
	preservedRecords := []domainevidence.PrivateAcceptedFinalRecord{}
	for _, records := range [][]domainevidence.PrivateAcceptedFinalRecord{known, nonExecutableAudit} {
		for _, record := range records {
			if err := preservedScope.validateContext(record.SecurityContext); err != nil {
				return RestartRecoveryResultV1{}, err
			}
			if preservedScope.ownsThread(record.SecurityContext.ThreadID) {
				observation, err := input.CASReader.ReadAcceptedFinalCASObservation(ctx, record.SecurityContext.ThreadID, record.SecurityContext.TurnID)
				if err != nil || !terminalObservationMatchesFrozenContextV1(observation, record.SecurityContext) {
					return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal preserved primary is invalid"), err)
				}
				preservedObservations[record.AcceptedFinal.RecordDigest] = observation
				preservedRecords = append(preservedRecords, record)
			}
		}
	}
	inventory, err := coordinator.preflightRestartRecoveryInventoryV1(
		ctx, known, nonExecutableAudit, candidates,
	)
	if err != nil {
		return RestartRecoveryResultV1{}, err
	}
	candidateByDigest := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		candidateByDigest[candidate.AcceptedFinal.RecordDigest] = true
	}
	auditOnly := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(known)-len(candidates))
	deferredAudit := make([]domainevidence.PrivateAcceptedFinalRecord, 0)
	consumedAccepted := make(map[string]bool, len(inventory.acceptedByFinal))
	for _, record := range known {
		if candidateByDigest[record.AcceptedFinal.RecordDigest] {
			continue
		}
		observation, readErr := input.CASReader.ReadAcceptedFinalCASObservation(
			ctx, record.SecurityContext.ThreadID, record.SecurityContext.TurnID,
		)
		if readErr != nil || !terminalObservationMatchesFrozenContextV1(observation, record.SecurityContext) {
			return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal audit-only private final is not an exact losing contender"), readErr)
		}
		if !observation.HasWinner {
			owner, ownerFound := inventory.intentByContext[record.SecurityContext.ContextDigest]
			if !ownerFound || owner.AcceptedFinalDigest == record.AcceptedFinal.RecordDigest ||
				!candidateByDigest[owner.AcceptedFinalDigest] || !terminalObservationMatchesContextV1(observation, record.SecurityContext) {
				return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only private final is ambiguous before public CAS recovery")
			}
			deferredAudit = append(deferredAudit, record)
		} else if observation.Winner.RecordDigest == record.AcceptedFinal.RecordDigest {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only private final is the public winner")
		}
		if intent, found := inventory.intentByAcceptedFinal[record.AcceptedFinal.RecordDigest]; found && intent.IntentID != "" {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only contender owns a resumable intent")
		}
		if disposition, found := inventory.terminalByAcceptedFinal[record.AcceptedFinal.RecordDigest]; found && disposition.DispositionID != "" {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only contender owns a terminal disposition")
		}
		if disposition, found := inventory.acceptedByFinal[record.AcceptedFinal.RecordDigest]; found {
			if err := coordinator.verifyAuditOnlyAcceptedDispositionV1(ctx, record, observation, disposition); err != nil {
				return RestartRecoveryResultV1{}, err
			}
			consumedAccepted[record.AcceptedFinal.RecordDigest] = true
		}
		auditOnly = append(auditOnly, record)
	}
	sort.Slice(auditOnly, func(i, j int) bool {
		return auditOnly[i].AcceptedFinal.RecordDigest < auditOnly[j].AcceptedFinal.RecordDigest
	})

	consumedIntents := make(map[string]bool, len(inventory.intentByContext))
	consumedClosures := make(map[string]bool, len(inventory.closureByID))
	consumedTerminal := make(map[string]bool, len(inventory.terminalByContext))
	sort.Slice(nonExecutableAudit, func(i, j int) bool {
		return nonExecutableAudit[i].AcceptedFinal.RecordDigest < nonExecutableAudit[j].AcceptedFinal.RecordDigest
	})
	nonExecutableObservations := make(map[string]domainevidence.AcceptedFinalCASObservationV1, len(nonExecutableAudit))
	for _, record := range nonExecutableAudit {
		if err := ctx.Err(); err != nil {
			return RestartRecoveryResultV1{}, err
		}
		digest := record.AcceptedFinal.RecordDigest
		manifestDigest := terminalEventManifestDigestV1(record)
		if manifestDigest == "" {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only publication manifest is unavailable")
		}
		observation, readErr := input.CASReader.ReadAcceptedFinalCASObservation(
			ctx, record.SecurityContext.ThreadID, record.SecurityContext.TurnID,
		)
		if readErr != nil || !terminalObservationMatchesFrozenContextV1(observation, record.SecurityContext) {
			return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal audit-only CAS observation is invalid"), readErr)
		}
		nonExecutableObservations[digest] = observation

		intent, intentFound := inventory.intentByAcceptedFinal[digest]
		if intentFound {
			if err := coordinator.verifyIntentAuditV1(ctx, intent, record, manifestDigest); err != nil {
				return RestartRecoveryResultV1{}, err
			}
			consumedIntents[record.SecurityContext.ContextDigest] = true
		}
		closure, closureFound, closureErr := coordinator.providerTurns.ObserveTurnClosureV1(ctx, record.SecurityContext)
		if closureErr != nil {
			return RestartRecoveryResultV1{}, closureErr
		}
		if closureFound && intentFound {
			inventoried, found := inventory.closureByID[closure.ClosureID]
			if !found || !reflect.DeepEqual(inventoried, closure) || coordinator.verifyProviderClosureV1(ctx, intent, closure) != nil {
				return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only provider closure is detached")
			}
			consumedClosures[closure.ClosureID] = true
		}
		acceptedDisposition, acceptedFound := inventory.acceptedByFinal[digest]
		terminalDisposition, terminalFound := inventory.terminalByAcceptedFinal[digest]

		winnerIsRecord := observation.HasWinner && observation.Winner.RecordDigest == digest
		if winnerIsRecord && intentFound && !closureFound {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only public winner predates provider closure")
		}
		if !observation.HasWinner && (acceptedFound || terminalFound) {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only disposition predates public CAS")
		}
		if terminalFound && (!intentFound || !closureFound || !acceptedFound) {
			return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only terminal disposition lacks its complete prior chain")
		}
		if acceptedFound {
			switch acceptedDisposition.State {
			case domainevidence.AcceptedFinalCommitted:
				if !winnerIsRecord || coordinator.verifyAuditCommittedAcceptedDispositionV1(
					ctx, record, observation, acceptedDisposition,
				) != nil {
					return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only committed disposition is detached")
				}
				if intentFound && (acceptedDisposition.DecidedAt != closure.ClosedAt ||
					acceptedDisposition.AuthorityKeyID != closure.AuthorityKeyID ||
					acceptedDisposition.AuthorityPublicKey != closure.AuthorityPublicKey) {
					return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only committed disposition does not follow provider closure")
				}
			case domainevidence.AcceptedFinalExplicitlyNotCommitted:
				if winnerIsRecord || coordinator.verifyAuditOnlyAcceptedDispositionV1(
					ctx, record, observation, acceptedDisposition,
				) != nil || terminalFound {
					return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only losing disposition is detached")
				}
			default:
				return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only disposition state is invalid")
			}
			consumedAccepted[digest] = true
		}
		if terminalFound {
			if acceptedDisposition.State != domainevidence.AcceptedFinalCommitted ||
				coordinator.verifyTerminalDispositionV1(ctx, terminalDisposition, intent, closure, acceptedDisposition) != nil {
				return RestartRecoveryResultV1{}, errors.New("turn terminal audit-only terminal disposition is detached")
			}
			consumedTerminal[record.SecurityContext.ContextDigest] = true
		}
	}

	plans := make([]restartRecoveryCandidateV1, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return RestartRecoveryResultV1{}, err
		}
		observation, err := input.CASReader.ReadAcceptedFinalCASObservation(
			ctx, candidate.SecurityContext.ThreadID, candidate.SecurityContext.TurnID,
		)
		if err != nil || !terminalObservationMatchesFrozenContextV1(observation, candidate.SecurityContext) {
			return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal restart CAS observation is invalid"), err)
		}
		contextDigest := candidate.SecurityContext.ContextDigest
		intent, intentFound := inventory.intentByContext[contextDigest]
		persistedIntent, intentErr := coordinator.terminals.ReadIntent(ctx, contextDigest)
		if intentFound {
			if intentErr != nil || !reflect.DeepEqual(persistedIntent, intent) || intent.AcceptedFinalDigest != candidate.AcceptedFinal.RecordDigest {
				return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal restart intent inventory changed or targets another final"), intentErr)
			}
			consumedIntents[contextDigest] = true
		} else if intentErr != nil && !errors.Is(intentErr, turnterminalstoreport.ErrNotFound) {
			return RestartRecoveryResultV1{}, intentErr
		} else if intentErr == nil {
			return RestartRecoveryResultV1{}, errors.New("turn terminal restart intent appeared after preflight")
		}
		closure, closureFound, err := coordinator.providerTurns.ObserveTurnClosureV1(ctx, candidate.SecurityContext)
		if err != nil {
			return RestartRecoveryResultV1{}, err
		}
		if closureFound {
			inventoried, found := inventory.closureByID[closure.ClosureID]
			if !found || !reflect.DeepEqual(inventoried, closure) {
				return RestartRecoveryResultV1{}, errors.New("turn terminal provider closure inventory changed after preflight")
			}
			consumedClosures[closure.ClosureID] = true
		}
		acceptedDisposition, acceptedDispositionFound := inventory.acceptedByFinal[candidate.AcceptedFinal.RecordDigest]
		if acceptedDispositionFound {
			consumedAccepted[candidate.AcceptedFinal.RecordDigest] = true
		}
		terminalDisposition, terminalDispositionFound := inventory.terminalByContext[contextDigest]
		persistedTerminal, terminalDispositionErr := coordinator.terminals.ReadDisposition(ctx, contextDigest)
		if terminalDispositionFound {
			if terminalDispositionErr != nil || !reflect.DeepEqual(persistedTerminal, terminalDisposition) ||
				terminalDisposition.AcceptedFinalDigest != candidate.AcceptedFinal.RecordDigest {
				return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal disposition inventory changed or targets another final"), terminalDispositionErr)
			}
			consumedTerminal[contextDigest] = true
		} else if terminalDispositionErr != nil && !errors.Is(terminalDispositionErr, turnterminalstoreport.ErrNotFound) {
			return RestartRecoveryResultV1{}, terminalDispositionErr
		} else if terminalDispositionErr == nil {
			return RestartRecoveryResultV1{}, errors.New("turn terminal disposition appeared after preflight")
		}

		if observation.HasWinner {
			if observation.Winner.RecordDigest != candidate.AcceptedFinal.RecordDigest {
				return RestartRecoveryResultV1{}, errors.New("turn terminal restart candidate is not the public winner")
			}
			if !intentFound {
				if closureFound || acceptedDispositionFound || terminalDispositionFound {
					return RestartRecoveryResultV1{}, errors.New("turn terminal legacy winner has detached later authority")
				}
				plans = append(plans, restartRecoveryCandidateV1{
					action: restartRecoveryLegacyQuarantineV1, privateFinal: candidate, observation: observation,
				})
				continue
			}
			if !closureFound {
				return RestartRecoveryResultV1{}, errors.New("turn terminal public winner lacks its prior provider closure")
			}
		} else {
			if !turnTerminalCASStatusIsActiveV1(observation.Status) {
				return RestartRecoveryResultV1{}, errors.New("turn terminal restart candidate has no active public CAS")
			}
			if closureFound && !intentFound {
				return RestartRecoveryResultV1{}, errors.New("turn terminal provider closure predates its intent")
			}
			if acceptedDispositionFound || terminalDispositionFound {
				return RestartRecoveryResultV1{}, errors.New("turn terminal disposition predates public commit")
			}
		}

		if intentFound {
			if err := coordinator.verifyIntentV1(ctx, intent, candidate, terminalEventManifestDigestV1(candidate), nil); err != nil {
				return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal restart intent is inconsistent"), err)
			}
			if closureFound {
				if err := coordinator.verifyProviderClosureV1(ctx, intent, closure); err != nil {
					return RestartRecoveryResultV1{}, err
				}
			}
		}
		if acceptedDispositionFound {
			if !observation.HasWinner || !closureFound || coordinator.verifyAcceptedFinalDispositionV1(
				ctx, candidate, terminalEventManifestDigestV1(candidate), observation, closure, acceptedDisposition, nil,
			) != nil {
				return RestartRecoveryResultV1{}, errors.New("turn terminal restart accepted-final disposition is not an exact V2 commit")
			}
		}
		if terminalDispositionFound {
			if !intentFound || !closureFound || !acceptedDispositionFound ||
				coordinator.verifyTerminalDispositionV1(ctx, terminalDisposition, intent, closure, acceptedDisposition) != nil {
				return RestartRecoveryResultV1{}, errors.New("turn terminal restart disposition is detached")
			}
		}
		complete := observation.HasWinner && intentFound && closureFound && acceptedDispositionFound && terminalDispositionFound
		if complete {
			plans = append(plans, restartRecoveryCandidateV1{
				action: restartRecoveryCompleteV1, privateFinal: candidate, observation: observation,
				intent: intent, providerClosure: closure, acceptedDisposition: acceptedDisposition,
				terminalDisposition: terminalDisposition,
			})
			continue
		}
		if !preservedScope.ownsThread(candidate.SecurityContext.ThreadID) && !terminalObservationMatchesContextV1(observation, candidate.SecurityContext) {
			return RestartRecoveryResultV1{}, errors.New("turn terminal incomplete historical prefix cannot resume after context advance")
		}
		plans = append(plans, restartRecoveryCandidateV1{
			action: restartRecoveryResumeV1, privateFinal: candidate, observation: observation,
			intent: intent, providerClosure: closure, acceptedDisposition: acceptedDisposition,
			terminalDisposition: terminalDisposition,
		})
	}
	if len(consumedIntents) != len(inventory.intentByContext) ||
		len(consumedAccepted) != len(inventory.acceptedByFinal) || len(consumedTerminal) != len(inventory.terminalByContext) {
		return RestartRecoveryResultV1{}, errors.New("turn terminal restart authority inventory contains an unclassified record")
	}
	providerAuditOnly := make([]domaincachetelemetry.ProviderTurnClosureV1, 0, len(inventory.closureByID)-len(consumedClosures))
	for closureID, closure := range inventory.closureByID {
		if !consumedClosures[closureID] {
			providerAuditOnly = append(providerAuditOnly, closure)
		}
	}
	sort.Slice(providerAuditOnly, func(i, j int) bool { return providerAuditOnly[i].ClosureID < providerAuditOnly[j].ClosureID })
	revalidated, err := coordinator.preflightRestartRecoveryInventoryV1(
		ctx, known, nonExecutableAudit, candidates,
	)
	if err != nil || !reflect.DeepEqual(revalidated, inventory) {
		return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal restart authority inventory changed after planning"), err)
	}
	for _, record := range nonExecutableAudit {
		observed, readErr := input.CASReader.ReadAcceptedFinalCASObservation(
			ctx, record.SecurityContext.ThreadID, record.SecurityContext.TurnID,
		)
		if readErr != nil || !reflect.DeepEqual(observed, nonExecutableObservations[record.AcceptedFinal.RecordDigest]) {
			return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal audit-only CAS inventory changed after planning"), readErr)
		}
	}

	result := RestartRecoveryResultV1{
		Complete: []CommitResultV1{}, LegacyQuarantined: []domainevidence.PrivateAcceptedFinalRecord{}, AuditOnly: auditOnly,
		NonExecutableAuditOnly: append([]domainevidence.PrivateAcceptedFinalRecord(nil), nonExecutableAudit...),
		ProviderAuditOnly:      providerAuditOnly,
		Preserved:              preservedRecords,
	}
	withoutPreserved := func(records []domainevidence.PrivateAcceptedFinalRecord) []domainevidence.PrivateAcceptedFinalRecord {
		out := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(records))
		for _, record := range records {
			if !preservedScope.ownsThread(record.SecurityContext.ThreadID) {
				out = append(out, record)
			}
		}
		return out
	}
	result.AuditOnly = withoutPreserved(result.AuditOnly)
	result.NonExecutableAuditOnly = withoutPreserved(result.NonExecutableAuditOnly)
	revalidatePreserved := func() error {
		for _, record := range preservedRecords {
			observed, err := input.CASReader.ReadAcceptedFinalCASObservation(ctx, record.SecurityContext.ThreadID, record.SecurityContext.TurnID)
			if err != nil || !reflect.DeepEqual(observed, preservedObservations[record.AcceptedFinal.RecordDigest]) {
				return errors.Join(errors.New("turn terminal preserved primary changed"), err)
			}
		}
		return ctx.Err()
	}
	if err := revalidatePreserved(); err != nil {
		return RestartRecoveryResultV1{}, err
	}
	for _, plan := range plans {
		if preservedScope.ownsThread(plan.privateFinal.SecurityContext.ThreadID) {
			continue
		}
		switch plan.action {
		case restartRecoveryLegacyQuarantineV1:
			result.LegacyQuarantined = append(result.LegacyQuarantined, plan.privateFinal)
			continue
		case restartRecoveryCompleteV1:
			publication, err := appturn.BuildAcceptedFinalPublicationPlan(
				plan.privateFinal.AcceptedFinal, plan.privateFinal.RenderedText, plan.privateFinal.PublicationIntent,
			)
			if err != nil {
				return RestartRecoveryResultV1{}, err
			}
			result.Complete = append(result.Complete, CommitResultV1{
				Persistence: appturn.PersistAcceptedFinalResult{
					CompletionRecord: publication.Completion, AcceptedFinal: plan.privateFinal.AcceptedFinal,
					Publication: publication, Changed: false, Status: plan.observation.Status,
				},
				Intent: plan.intent, ProviderClosure: plan.providerClosure, PublicObservation: plan.observation,
				AcceptedFinalDisposition: plan.acceptedDisposition, TerminalDisposition: plan.terminalDisposition,
			})
			continue
		case restartRecoveryResumeV1:
		default:
			return RestartRecoveryResultV1{}, errors.New("turn terminal restart recovery plan is invalid")
		}
		committed, err := coordinator.CommitV1(ctx, CommitInputV1{
			CompletionStore: input.CompletionStore, CASReader: input.CASReader, PrivateFinal: plan.privateFinal,
		})
		if err != nil {
			return RestartRecoveryResultV1{}, err
		}
		result.Complete = append(result.Complete, committed)
	}
	for _, record := range deferredAudit {
		if preservedScope.ownsThread(record.SecurityContext.ThreadID) {
			continue // Its original pre-CAS relationship remains held, not a new loser.
		}
		observation, err := input.CASReader.ReadAcceptedFinalCASObservation(
			ctx, record.SecurityContext.ThreadID, record.SecurityContext.TurnID,
		)
		if err != nil || !terminalObservationMatchesFrozenContextV1(observation, record.SecurityContext) ||
			!observation.HasWinner || observation.Winner.RecordDigest == record.AcceptedFinal.RecordDigest {
			return RestartRecoveryResultV1{}, errors.Join(errors.New("turn terminal deferred audit contender did not become an exact loser"), err)
		}
	}
	if err := revalidatePreserved(); err != nil {
		return RestartRecoveryResultV1{}, err
	}
	return result, nil
}

func (coordinator *Coordinator) preflightRestartRecoveryInventoryV1(
	ctx context.Context,
	known []domainevidence.PrivateAcceptedFinalRecord,
	auditOnly []domainevidence.PrivateAcceptedFinalRecord,
	candidates []domainevidence.PrivateAcceptedFinalRecord,
) (restartRecoveryInventoryV1, error) {
	inventory := restartRecoveryInventoryV1{
		knownByDigest:           make(map[string]domainevidence.PrivateAcceptedFinalRecord, len(known)+len(auditOnly)),
		auditOnlyByDigest:       make(map[string]domainevidence.PrivateAcceptedFinalRecord, len(auditOnly)),
		intentByContext:         map[string]domainturnterminal.TurnTerminalIntentV1{},
		intentByAcceptedFinal:   map[string]domainturnterminal.TurnTerminalIntentV1{},
		closureByID:             map[string]domaincachetelemetry.ProviderTurnClosureV1{},
		acceptedByFinal:         map[string]domainevidence.AcceptedFinalDispositionRecord{},
		terminalByContext:       map[string]domainturnterminal.TurnTerminalDispositionV1{},
		terminalByAcceptedFinal: map[string]domainturnterminal.TurnTerminalDispositionV1{},
	}
	for _, record := range known {
		if domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(record) != nil ||
			coordinator.verifyAcceptedFinalTrustedV1(ctx, record.AcceptedFinal) != nil {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal private inventory contains an invalid record")
		}
		digest := record.AcceptedFinal.RecordDigest
		if _, duplicate := inventory.knownByDigest[digest]; duplicate {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal private inventory contains a duplicate record")
		}
		inventory.knownByDigest[digest] = record
	}
	for _, record := range auditOnly {
		publicationErr := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(record)
		currentExecutableBoundary := record.SchemaVersion == domainevidence.PrivateAcceptedFinalRecordVersion &&
			record.AcceptedFinal.SchemaVersion == domainevidence.AcceptedFinalRecordVersion && publicationErr == nil
		if domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(record) != nil ||
			currentExecutableBoundary ||
			coordinator.verifyAcceptedFinalTrustedV1(ctx, record.AcceptedFinal) != nil {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal audit-only private inventory contains an invalid or executable record")
		}
		digest := record.AcceptedFinal.RecordDigest
		if _, duplicate := inventory.knownByDigest[digest]; duplicate {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal private inventories overlap or contain a duplicate record")
		}
		inventory.knownByDigest[digest] = record
		inventory.auditOnlyByDigest[digest] = record
	}
	persisted, err := coordinator.privateFinals.List(ctx)
	if err != nil {
		return restartRecoveryInventoryV1{}, err
	}
	if len(persisted) != len(inventory.knownByDigest) {
		return restartRecoveryInventoryV1{}, errors.New("turn terminal caller inventory omits or invents a private final")
	}
	for _, record := range persisted {
		knownRecord, found := inventory.knownByDigest[record.AcceptedFinal.RecordDigest]
		if !found || !reflect.DeepEqual(knownRecord, record) {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal caller inventory differs from durable private authority")
		}
	}
	for _, candidate := range candidates {
		knownRecord, found := inventory.knownByDigest[candidate.AcceptedFinal.RecordDigest]
		_, auditOnlyCandidate := inventory.auditOnlyByDigest[candidate.AcceptedFinal.RecordDigest]
		if !found || auditOnlyCandidate || !reflect.DeepEqual(knownRecord, candidate) {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal restart candidate is outside the exact private inventory")
		}
	}
	if err := coordinator.terminals.VisitIntents(ctx, func(intent domainturnterminal.TurnTerminalIntentV1) error {
		record, found := inventory.knownByDigest[intent.AcceptedFinalDigest]
		if !found {
			return errors.New("turn terminal intent is orphaned from private final authority")
		}
		if _, duplicate := inventory.intentByContext[intent.SecurityContext.ContextDigest]; duplicate {
			return errors.New("turn terminal intent inventory duplicates a frozen context")
		}
		if _, duplicate := inventory.intentByAcceptedFinal[intent.AcceptedFinalDigest]; duplicate {
			return errors.New("turn terminal intent inventory duplicates an accepted final")
		}
		manifestDigest := terminalEventManifestDigestV1(record)
		if _, auditOnlyRecord := inventory.auditOnlyByDigest[intent.AcceptedFinalDigest]; auditOnlyRecord {
			if err := coordinator.verifyIntentAuditV1(ctx, intent, record, manifestDigest); err != nil {
				return err
			}
		} else if err := coordinator.verifyIntentV1(ctx, intent, record, manifestDigest, nil); err != nil {
			return err
		}
		inventory.intentByContext[intent.SecurityContext.ContextDigest] = intent
		inventory.intentByAcceptedFinal[intent.AcceptedFinalDigest] = intent
		return nil
	}); err != nil {
		return restartRecoveryInventoryV1{}, err
	}
	if err := coordinator.providerTurns.VisitTurnClosuresV1(ctx, func(closure domaincachetelemetry.ProviderTurnClosureV1) error {
		if err := coordinator.verifyProviderClosureTrustedInventoryV1(ctx, closure); err != nil {
			return err
		}
		if _, duplicate := inventory.closureByID[closure.ClosureID]; duplicate {
			return errors.New("turn terminal provider closure inventory duplicates an identity")
		}
		for _, existing := range inventory.closureByID {
			if existing.TurnBindingHMAC == closure.TurnBindingHMAC {
				return errors.New("turn terminal provider closure inventory duplicates a turn binding")
			}
		}
		inventory.closureByID[closure.ClosureID] = closure
		return nil
	}); err != nil {
		return restartRecoveryInventoryV1{}, err
	}
	dispositions, err := coordinator.privateFinals.ListDispositions(ctx)
	if err != nil {
		return restartRecoveryInventoryV1{}, err
	}
	for _, disposition := range dispositions {
		record, found := inventory.knownByDigest[disposition.AcceptedFinalDigest]
		if !found || disposition.PrivateRecordDigest != record.PrivateRecordDigest ||
			disposition.ThreadID != record.SecurityContext.ThreadID || disposition.TurnID != record.SecurityContext.TurnID {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal accepted-final disposition is orphaned")
		}
		if _, duplicate := inventory.acceptedByFinal[disposition.AcceptedFinalDigest]; duplicate {
			return restartRecoveryInventoryV1{}, errors.New("turn terminal restart accepted-final disposition is duplicated")
		}
		if err := coordinator.verifyAcceptedDispositionTrustedInventoryV1(ctx, disposition); err != nil {
			return restartRecoveryInventoryV1{}, err
		}
		inventory.acceptedByFinal[disposition.AcceptedFinalDigest] = disposition
	}
	if err := coordinator.terminals.VisitDispositions(ctx, func(disposition domainturnterminal.TurnTerminalDispositionV1) error {
		if _, found := inventory.knownByDigest[disposition.AcceptedFinalDigest]; !found {
			return errors.New("turn terminal disposition is orphaned from private final authority")
		}
		if _, duplicate := inventory.terminalByContext[disposition.ContextDigest]; duplicate {
			return errors.New("turn terminal disposition inventory duplicates a frozen context")
		}
		if _, duplicate := inventory.terminalByAcceptedFinal[disposition.AcceptedFinalDigest]; duplicate {
			return errors.New("turn terminal disposition inventory duplicates an accepted final")
		}
		if err := coordinator.verifyTerminalDispositionTrustedInventoryV1(ctx, disposition); err != nil {
			return err
		}
		inventory.terminalByContext[disposition.ContextDigest] = disposition
		inventory.terminalByAcceptedFinal[disposition.AcceptedFinalDigest] = disposition
		return nil
	}); err != nil {
		return restartRecoveryInventoryV1{}, err
	}
	return inventory, nil
}

func (coordinator *Coordinator) verifyProviderClosureTrustedInventoryV1(
	ctx context.Context,
	closure domaincachetelemetry.ProviderTurnClosureV1,
) error {
	keyID, publicKey, signature, err := domaincachetelemetry.ProviderTurnClosureV1AuthorityMaterial(closure)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domaincachetelemetry.ProviderTurnClosureV1SigningBytes(closure), signature) != nil {
		return errors.New("turn terminal provider closure inventory is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyAcceptedDispositionTrustedInventoryV1(
	ctx context.Context,
	disposition domainevidence.AcceptedFinalDispositionRecord,
) error {
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature) != nil {
		return errors.New("turn terminal accepted-final disposition inventory is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyTerminalDispositionTrustedInventoryV1(
	ctx context.Context,
	disposition domainturnterminal.TurnTerminalDispositionV1,
) error {
	keyID, publicKey, signature, err := domainturnterminal.TurnTerminalDispositionV1AuthorityMaterial(disposition)
	if err != nil || coordinator.authority.VerifyTrusted(ctx, keyID, publicKey,
		domainturnterminal.TurnTerminalDispositionV1SigningBytes(disposition), signature) != nil {
		return errors.New("turn terminal disposition inventory is not installation-trusted")
	}
	return nil
}

func (coordinator *Coordinator) verifyAuditOnlyAcceptedDispositionV1(
	ctx context.Context,
	record domainevidence.PrivateAcceptedFinalRecord,
	observation domainevidence.AcceptedFinalCASObservationV1,
	disposition domainevidence.AcceptedFinalDispositionRecord,
) error {
	publication, err := buildRestartRecoveryPublicationPlanV1(record)
	if err != nil || disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		disposition.State != domainevidence.AcceptedFinalExplicitlyNotCommitted ||
		disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionDifferentPublicWinner ||
		disposition.AcceptedFinalDigest != record.AcceptedFinal.RecordDigest ||
		disposition.PrivateRecordDigest != record.PrivateRecordDigest ||
		disposition.ThreadID != record.SecurityContext.ThreadID || disposition.TurnID != record.SecurityContext.TurnID ||
		disposition.EventManifestDigest != publication.EventManifestDigest ||
		disposition.TurnCASDigest != observation.TurnProjectionSHA256 ||
		disposition.WinnerDigest != observation.Winner.RecordDigest ||
		coordinator.verifyAcceptedDispositionTrustedInventoryV1(ctx, disposition) != nil {
		return errors.New("turn terminal audit-only disposition is not an exact losing CAS record")
	}
	return nil
}

func (coordinator *Coordinator) verifyAuditCommittedAcceptedDispositionV1(
	ctx context.Context,
	record domainevidence.PrivateAcceptedFinalRecord,
	observation domainevidence.AcceptedFinalCASObservationV1,
	disposition domainevidence.AcceptedFinalDispositionRecord,
) error {
	publication, err := buildRestartRecoveryPublicationPlanV1(record)
	if err != nil || domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(record) != nil ||
		domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil ||
		disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		disposition.State != domainevidence.AcceptedFinalCommitted ||
		disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		!observation.HasWinner || observation.Winner.RecordDigest != record.AcceptedFinal.RecordDigest ||
		disposition.AcceptedFinalDigest != record.AcceptedFinal.RecordDigest ||
		disposition.PrivateRecordDigest != record.PrivateRecordDigest ||
		disposition.ThreadID != record.SecurityContext.ThreadID || disposition.TurnID != record.SecurityContext.TurnID ||
		disposition.EventManifestDigest != publication.EventManifestDigest ||
		disposition.TurnCASDigest != observation.TurnProjectionSHA256 ||
		disposition.WinnerDigest != record.AcceptedFinal.RecordDigest ||
		coordinator.verifyAcceptedDispositionTrustedInventoryV1(ctx, disposition) != nil {
		return errors.New("turn terminal audit-only committed disposition is not an exact public CAS record")
	}
	return nil
}

func terminalEventManifestDigestV1(privateFinal domainevidence.PrivateAcceptedFinalRecord) string {
	publication, err := buildRestartRecoveryPublicationPlanV1(privateFinal)
	if err != nil {
		return ""
	}
	return publication.EventManifestDigest
}

// buildRestartRecoveryPublicationPlanV1 is byte reconstruction only. Callers
// must separately hold live publication authority before using its result for
// mutation; audit-only records use it solely to verify existing immutable
// manifests and dispositions.
func buildRestartRecoveryPublicationPlanV1(privateFinal domainevidence.PrivateAcceptedFinalRecord) (appturn.AcceptedFinalPublicationPlan, error) {
	if privateFinal.SchemaVersion == domainevidence.PrivateAcceptedFinalRecordVersion &&
		privateFinal.AcceptedFinal.SchemaVersion == domainevidence.AcceptedFinalRecordVersion {
		return appturn.BuildAcceptedFinalPublicationPlan(
			privateFinal.AcceptedFinal, privateFinal.RenderedText, privateFinal.PublicationIntent,
		)
	}
	return appturn.BuildAcceptedFinalAuditPublicationPlan(
		privateFinal.AcceptedFinal, privateFinal.RenderedText, privateFinal.PublicationIntent,
	)
}
