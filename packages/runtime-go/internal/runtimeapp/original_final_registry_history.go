package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"

	evidenceauthoritystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// This reader supplies historical content only. Its live delegate remains the
// owner of current membership and fact-final authority; a failed Original
// observation never falls back to that delegate.
type runtimeOriginalFinalReplayV1 struct {
	registryport.HistoricalReplay
	frozen *runtimeOriginalFinalHistoryV1
}

func runtimeFinalHistoricalReaderV1(live registryport.HistoricalReplay, core *runtimeChildIdentityStartupV1, preserved runtimeReportRestartPreservationV1) registryport.HistoricalReplay {
	if preserved.finalHistory == nil {
		// No new historical qualification is acquired after recovery.
		return live
	}
	return &runtimeOriginalFinalReplayV1{HistoricalReplay: live, frozen: preserved.finalHistory}
}

func (reader *runtimeOriginalFinalReplayV1) CaseEvidenceAuthorityUnavailableV1() bool {
	marker, ok := reader.HistoricalReplay.(interface{ CaseEvidenceAuthorityUnavailableV1() bool })
	return ok && marker.CaseEvidenceAuthorityUnavailableV1()
}

func (reader *runtimeOriginalFinalReplayV1) ReplayOriginalAcceptedFinalV1(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord) (snapshot domainevidence.EvidenceReceiptRegistry, resultErr error) {
	if ctx == nil || reader == nil || reader.frozen == nil ||
		record.RegistryHead.Sequence == 0 || domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Envelope.Variant) || record.AcceptedFinal.FactFinalWitnessAdmission != nil {
		return snapshot, errors.New("original boundary final history is unavailable")
	}
	if err := reader.frozen.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return snapshot, err
	}
	snapshot, resultErr = replayRuntimeOriginalBoundaryPrefixV1(ctx, record, reader.frozen.registry, reader.frozen.evidence)
	resultErr = errors.Join(resultErr, reader.frozen.ValidateSemanticOperationsV1(ctx, nil, nil, ""))
	if resultErr != nil {
		snapshot = domainevidence.EvidenceReceiptRegistry{}
	}
	return snapshot, resultErr
}

type runtimeOriginalFinalGraphsV1 struct {
	registry                        *registryport.OriginalHistoryV2
	evidence                        evidenceauthoritystore.OriginalGraphV1
	registryOriginal, registryFinal runtimeOriginalSemanticFilesV1
	evidenceOriginal, evidenceFinal runtimeOriginalSemanticFilesV1
	contexts                        []domainsecurity.TurnSecurityContext
	revalidate                      func(context.Context) error
}

func observeRuntimeOriginalFinalGraphsV1(ctx context.Context, core *runtimeChildIdentityStartupV1, preserved runtimeReportRestartPreservationV1) (result runtimeOriginalFinalGraphsV1, resultErr error) {
	if ctx == nil || core == nil || preserved.report == nil {
		return result, errors.New("original final graph dependencies are unavailable")
	}
	if err := errors.Join(ctx.Err(), core.revalidateKey(ctx), core.originalRegistryTrust.Revalidate(ctx)); err != nil {
		return result, err
	}
	var saved runtimeOriginalSemanticFilesV1
	if preserved.registry != nil {
		saved = preserved.registry.recoveryBefore
	}
	registryFiles, registryFinal, contexts, registryObservation, err := readRuntimeRegistrySemanticInventoryV1(ctx, core, preserved.report, saved)
	if err != nil {
		return result, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, registryObservation.Revalidate(ctx), core.revalidateKey(ctx), core.originalRegistryTrust.Revalidate(ctx), ctx.Err())
		if resultErr != nil {
			result = runtimeOriginalFinalGraphsV1{}
		}
	}()
	originalDomain, err := observeRuntimeOriginalRegistryDomainV1(ctx, core, registryFiles, contexts, true)
	if err != nil {
		return result, err
	}
	if originalDomain.unavailable {
		return result, nil
	}
	originals, finals, associatedContexts, associatedObservation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, core, preserved.report, preserved.associated)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, associatedObservation.Revalidate(ctx)) }()
	if !sameRuntimeSettlementContextsV1(contexts, associatedContexts) {
		return result, errors.New("original boundary history owners have different primary contexts")
	}
	if err := errors.Join(registryObservation.Revalidate(ctx), associatedObservation.Revalidate(ctx)); err != nil {
		return result, err
	}
	registry, err := parseRuntimeOriginalRegistryInventoryV1(ctx, core, registryFiles, contexts)
	if err != nil {
		return result, err
	}
	if registry.v2 == nil || registry.unavailable || len(registry.legacy) != 0 {
		return result, nil
	}
	for _, operation := range registryObservation.journal.OperationsV1() {
		if name, owned := registrySemanticRelativeV1(operation); owned {
			if err := registryFinal.applyV1(operation, name, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
				return registryObservation.journal.ReadAfterV1(ctx, operation)
			}); err != nil {
				return result, err
			}
		}
	}
	trust := core.originalRegistryTrust
	parseEvidence := func(files runtimeOriginalSemanticFilesV1) (evidenceauthoritystore.OriginalGraphV1, error) {
		projection := trust.projection
		return evidenceauthoritystore.ParseOriginalGraphV1(ctx, files, projection.InstallationID, projection.Enrollment.EnrollmentID, projection.Enrollment.WitnessKeyID, trust.witnessKey, core.verification)
	}
	graph, err := parseEvidence(originals["evidence-authority"])
	if err != nil {
		return result, err
	}
	if _, err := parseEvidence(finals["evidence-authority"]); err != nil {
		return result, err
	}
	return runtimeOriginalFinalGraphsV1{registry: registry.v2, evidence: graph, registryOriginal: registryFiles, registryFinal: registryFinal, evidenceOriginal: originals["evidence-authority"], evidenceFinal: finals["evidence-authority"], contexts: contexts, revalidate: func(ctx context.Context) error {
		return errors.Join(registryObservation.Revalidate(ctx), associatedObservation.Revalidate(ctx), core.revalidateKey(ctx), core.originalRegistryTrust.Revalidate(ctx), ctx.Err())
	}}, nil
}

// Both graphs have already been authenticated as one complete Original
// observation. A signed local sibling or standalone capsule cannot select
// itself: only ancestry rooted in a stored, independently enrolled witness
// exchange can support the exact head signed into this boundary final.
func replayRuntimeOriginalBoundaryPrefixV1(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, history *registryport.OriginalHistoryV2, graph evidenceauthoritystore.OriginalGraphV1) (domainevidence.EvidenceReceiptRegistry, error) {
	empty := domainevidence.EvidenceReceiptRegistry{}
	if ctx == nil || history == nil || history.Indexes == nil || history.Capsules == nil || graph.Bundles == nil || graph.Observations == nil ||
		record.RegistryHead.Sequence == 0 || domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Envelope.Variant) || record.AcceptedFinal.FactFinalWitnessAdmission != nil {
		return empty, errors.New("original boundary prefix dependencies are unavailable")
	}
	var selected domainevidence.EvidenceReceiptRegistry
	var selectedBytes []byte
	for _, stored := range graph.Observations {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		bundle, found := graph.Bundles[stored.Bundle.RecordDigest]
		if !found || !reflect.DeepEqual(bundle, stored.Bundle) {
			return empty, errors.New("original boundary witness lost its exact bundle")
		}
		if bundle.EvidenceRegistryCount == 0 {
			continue
		}
		index, found := history.Indexes[bundle.EvidenceRegistryIndexDigest]
		if !found || domainevidence.ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(index, bundle.EvidenceRegistryIndexDigest, bundle.EvidenceRegistryCount) != nil {
			return empty, errors.New("original boundary witness lost its exact registry root")
		}
		for {
			if err := ctx.Err(); err != nil {
				return empty, err
			}
			capsule, found := history.Capsules[index.Entry.CapsuleRecordDigest]
			if !found || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(index, capsule) {
				return empty, errors.New("original boundary ancestry lost its exact capsule")
			}
			if reflect.DeepEqual(capsule.SecurityContext, record.SecurityContext) && capsule.Registry.Sequence >= record.RegistryHead.Sequence {
				prefix, err := domainevidence.NewEvidenceReceiptRegistry(record.SecurityContext)
				if err != nil {
					return empty, err
				}
				for _, entry := range capsule.Registry.Entries[:record.RegistryHead.Sequence] {
					prefix, err = domainevidence.ApplyEvidenceRegistryEntry(prefix, entry)
					if err != nil {
						return empty, err
					}
				}
				head, err := domainevidence.NewEvidenceRegistryHead(prefix)
				if err != nil {
					return empty, err
				}
				if reflect.DeepEqual(head, record.RegistryHead) {
					body, err := json.Marshal(prefix)
					if err != nil {
						return empty, err
					}
					if selectedBytes != nil && !bytes.Equal(selectedBytes, body) {
						return empty, errors.New("original boundary head has inconsistent historical content")
					}
					selected, selectedBytes = prefix, body
				}
			}
			if index.Generation == 1 {
				// Root and every transition have their full native domain grammar.
				break
			}
			previous, found := history.Indexes[index.PreviousIndexDigest]
			if !found || domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(previous, index) != nil {
				return empty, errors.New("original boundary registry ancestry is incomplete")
			}
			index = previous
		}
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if selectedBytes == nil {
		return empty, errors.New("original boundary head has no witnessed historical prefix")
	}
	return selected, nil
}
