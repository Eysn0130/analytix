package runtimeapp

import (
	"context"
	"sort"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type factRecoveryObservationV1 struct {
	Candidates, Admitted, Held int
	LastRejectedPhase          string
}
type factRecoveryObservationKeyV1 struct{}

// Whole-inventory signatures, terminal topology and event manifests have
// already passed startup preflight. Current domain authority can still be
// unavailable; retain that thread's originals without blocking ordinary Core.
func restoreRuntimeFactTerminalsV1(ctx context.Context, index *gateprojection.TrustedFinalProjectionIndex, candidates []turnterminalapp.FactTerminalCandidateV1, registry any, validate func(context.Context, domainevidence.PrivateAcceptedFinalRecord) error, load func(context.Context, domainevidence.PrivateAcceptedFinalRecord, appturn.FactFinalMutationAuthority) ([]map[string]any, error)) {
	report := factRecoveryObservationV1{Candidates: len(candidates)}
	defer func() {
		if observe, ok := ctx.Value(factRecoveryObservationKeyV1{}).(func(factRecoveryObservationV1)); ok {
			observe(report)
		}
	}()
	witness, ok := registry.(registryport.RecoveredFactFinalWitnessIssuer)
	if !ok {
		report.Held = len(candidates)
		return
	}
	batches := map[string][]gateprojection.TerminalCompleteFinalAuthorityV1{}
	for _, candidate := range candidates {
		complete := candidate.Terminal
		original := candidate.PrivateFinal
		batches[original.SecurityContext.ThreadID] = append(batches[original.SecurityContext.ThreadID], gateprojection.TerminalCompleteFinalAuthorityV1{
			PrivateFinal: original, Intent: complete.Intent, ProviderClosure: complete.ProviderClosure, PublicObservation: complete.PublicObservation,
			AcceptedFinalDisposition: complete.AcceptedFinalDisposition, TerminalDisposition: complete.TerminalDisposition,
		})
	}
	threads := make([]string, 0, len(batches))
	for id := range batches {
		threads = append(threads, id)
	}
	sort.Strings(threads)
	for _, id := range threads {
		batch := batches[id]
		if err := index.RestoreFactTerminalBatch(ctx, batch, witness, validate, load); err != nil {
			report.Held += len(batch)
			report.LastRejectedPhase = gateprojection.FactRecoveryFailurePhase(err)
		} else {
			report.Admitted += len(batch)
		}
	}
}
