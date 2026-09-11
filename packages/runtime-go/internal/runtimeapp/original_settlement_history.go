package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type runtimeOriginalSettlementHistoryV1 struct {
	prepared  runtimeOriginalSemanticFilesV1
	primaries runtimeReportSemanticPrimariesV1
	markers   map[string]string
}

func runtimeSettlementMarkerBytesV1(primaries runtimeReportSemanticPrimariesV1) (map[string]string, error) {
	markers := map[string]string{}
	for _, primary := range primaries {
		turns, _ := primary.Thread["turns"].([]any)
		for _, value := range turns {
			turn, _ := value.(map[string]any)
			items, _ := turn["items"].([]any)
			for _, value := range items {
				item, _ := value.(map[string]any)
				value, present := item["hostEvidenceSettlement"]
				if !present {
					continue
				}
				marker, err := domainevidence.ParseHostEvidenceSettlementMarker(value)
				if err != nil || markers[marker.SettlementID] != "" {
					return nil, errors.Join(errors.New("original settlement marker identity is invalid"), err)
				}
				body, err := json.Marshal(item)
				if err != nil {
					return nil, err
				}
				markers[marker.SettlementID] = string(body)
			}
		}
	}
	return markers, nil
}

func prepareRuntimeOriginalSettlementHistoryV1(ctx context.Context, frozen *runtimeOriginalFinalHistoryV1, graphs runtimeOriginalFinalGraphsV1) (*runtimeOriginalSettlementHistoryV1, error) {
	support := &runtimeOriginalSettlementHistoryV1{}
	files, primaries, err := support.validateV1(ctx, frozen, graphs, graphs.registry, nil, nil, "")
	if err != nil {
		return nil, err
	}
	markers, err := runtimeSettlementMarkerBytesV1(primaries)
	if err != nil {
		return nil, err
	}
	support.prepared, support.primaries, support.markers = runtimeReportCommittedFilesV1(files), primaries, markers
	return support, nil
}

func (support *runtimeOriginalSettlementHistoryV1) validateV1(ctx context.Context, frozen *runtimeOriginalFinalHistoryV1, graphs runtimeOriginalFinalGraphsV1, finalRegistry *registryport.OriginalHistoryV2, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWrite string) (_ runtimeOriginalSemanticFilesV1, _ runtimeReportSemanticPrimariesV1, resultErr error) {
	prepared, candidate, contexts, observation, err := readRuntimeSettlementSemanticInventoryV1(ctx, frozen.core, frozen.preserved.report, support.prepared)
	if err != nil {
		return nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx)) }()
	if !sameRuntimeSettlementContextsV1(contexts, graphs.contexts) {
		return nil, nil, errors.New("original settlement history context denominator changed")
	}
	apply := func(ops []domainstartup.SemanticStartupOperationV1, read func(domainstartup.SemanticStartupOperationV1) ([]byte, error), omitted string) error {
		for _, operation := range ops {
			if omitted != "" && operation.OperationID == omitted {
				continue
			}
			if err := validateRuntimeOriginalSemanticAncestorV1(runtimeSettlementSemanticRootV1, operation); err != nil {
				return err
			}
			if name, owned := settlementSemanticRelativeV1(operation); owned {
				if err := candidate.applyV1(operation, name, read); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := apply(observation.journal.OperationsV1(), func(op domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		return observation.journal.ReadAfterV1(ctx, op)
	}, ""); err != nil {
		return nil, nil, err
	}
	if err := apply(operations, readAfter, noWrite); err != nil {
		return nil, nil, err
	}
	primaries, finalPrimaries, revalidate, err := observeRuntimeSettlementPrimariesV1(ctx, frozen.core, support.primaries, operations, readAfter, noWrite)
	if err != nil {
		return nil, nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, revalidate(ctx)) }()
	for _, endpoint := range []struct {
		files     runtimeOriginalSemanticFilesV1
		primaries runtimeReportSemanticPrimariesV1
		registry  *registryport.OriginalHistoryV2
	}{{prepared, primaries, graphs.registry}, {candidate, finalPrimaries, finalRegistry}} {
		records, err := endpoint.files.verifySettlementsV1(ctx, frozen.core, contexts)
		if err != nil {
			return nil, nil, err
		}
		all := make([]domainevidence.PreparedEvidenceSettlement, 0, len(records))
		for _, record := range records {
			all = append(all, record)
		}
		if err := evidenceapp.ValidateOriginalEvidenceSettlementInventoryV1(ctx, runtimeSettlementPrimaryReaderV1{ctx, endpoint.primaries}, frozen.core.verification, all, endpoint.registry); err != nil {
			return nil, nil, err
		}
		for name, expected := range support.prepared {
			if !reflect.DeepEqual(endpoint.files[name], expected) {
				return nil, nil, errors.New("semantic candidate changed original settlement prepared material")
			}
		}
		markers, err := runtimeSettlementMarkerBytesV1(endpoint.primaries)
		if err != nil {
			return nil, nil, err
		}
		for id, expected := range support.markers {
			if markers[id] != expected {
				return nil, nil, errors.New("semantic candidate changed original settlement Core marker result")
			}
		}
	}
	return prepared, primaries, nil
}

type runtimeOriginalSettlementObserverV1 struct {
	frozen *runtimeOriginalFinalHistoryV1
}

func (observer runtimeOriginalSettlementObserverV1) ObserveOriginalEvidenceSettlementInventoryV1(ctx context.Context) (_ evidenceapp.PreservedEvidenceSettlementInventoryV1, resultErr error) {
	empty := evidenceapp.PreservedEvidenceSettlementInventoryV1{}
	frozen := observer.frozen
	if frozen == nil || frozen.settlementHistory == nil {
		return empty, errors.New("original settlement history is unqualified")
	}
	if err := frozen.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return empty, err
	}
	graphs, err := observeRuntimeOriginalFinalGraphsV1(ctx, frozen.core, frozen.preserved)
	if err != nil {
		return empty, err
	}
	defer func() { resultErr = errors.Join(resultErr, graphs.revalidate(ctx)) }()
	finalRegistry, err := parseRuntimeOriginalRegistryInventoryV1(ctx, frozen.core, graphs.registryFinal, graphs.contexts)
	if err != nil {
		return empty, err
	}
	_, primaries, err := frozen.settlementHistory.validateV1(ctx, frozen, graphs, finalRegistry.v2, nil, nil, "")
	if err != nil {
		return empty, err
	}
	original := evidenceapp.PreservedEvidenceSettlementInventoryV1{RegistryHistoryV2: graphs.registry}
	for _, primary := range primaries {
		original.AdditionalPrimaries = append(original.AdditionalPrimaries, primary)
	}
	sort.Slice(original.AdditionalPrimaries, func(i, j int) bool {
		return original.AdditionalPrimaries[i].ThreadID < original.AdditionalPrimaries[j].ThreadID
	})
	return original, nil
}

func (observer runtimeOriginalSettlementObserverV1) ObserveRestartEvidenceRegistryObservationV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) (registryport.OriginalObservationV1, error) {
	original, err := observer.ObserveOriginalEvidenceSettlementInventoryV1(ctx)
	if err != nil {
		return registryport.OriginalObservationV1{}, err
	}
	observed, revalidate, err := runtimeRegistryContextsV1(ctx, observer.frozen.core)
	if err != nil {
		return registryport.OriginalObservationV1{}, err
	}
	if !sameRuntimeSettlementContextsV1(contexts, observed) {
		return registryport.OriginalObservationV1{}, errors.New("original settlement reader primary denominator changed")
	}
	if err := revalidate(ctx); err != nil {
		return registryport.OriginalObservationV1{}, err
	}
	return registryport.OriginalObservationV1{HistoryV2: original.RegistryHistoryV2}, nil
}
