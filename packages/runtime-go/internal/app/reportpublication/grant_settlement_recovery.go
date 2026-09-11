package reportpublication

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type restartThreadReaderV1 interface {
	GetThread(string) (map[string]any, error)
}

// validateRestartGrantSettlementFromThreadV1 reconstructs the exact active
// and settled execution-grant registries from the durable thread. The signed
// settlement record never substitutes for result membership in that thread.
func validateRestartGrantSettlementFromThreadV1(
	ctx context.Context,
	settlement domainpublication.ReportGrantSettlementV1,
	decision domainpublication.ReportDeliveryDecisionV1,
	threads restartThreadReaderV1,
) error {
	_, err := resolveRestartGrantSettlementFromThreadV1(ctx, settlement, decision, threads)
	return err
}

func resolveRestartGrantSettlementFromThreadV1(
	ctx context.Context,
	settlement domainpublication.ReportGrantSettlementV1,
	decision domainpublication.ReportDeliveryDecisionV1,
	threads restartThreadReaderV1,
) (map[string]any, error) {
	if ctx == nil || threads == nil || ctx.Err() != nil ||
		domainpublication.ValidateReportGrantSettlementDecisionV1(settlement, decision) != nil {
		return nil, ErrPublicationIntegrity
	}
	thread, err := threads.GetThread(settlement.ThreadID)
	if err != nil || thread == nil {
		return nil, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	registry, err := executiongrantapp.RegistryFromThread(settlement.ThreadID, thread, settlement.TurnID)
	if err != nil {
		return nil, ErrPublicationIntegrity
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, settlement.GrantID)
	if !found || entry.Grant.ToolCallID != settlement.ToolCallID || entry.Grant.ToolName != settlement.ToolName {
		return nil, ErrPublicationIntegrity
	}
	durable, err := executiongrantapp.DurableSettlementFromThread(
		settlement.ThreadID, thread, settlement.TurnID, settlement.ResultItemID, entry.Grant,
	)
	resultItem, marshalErr := json.Marshal(durable.ResultItem)
	if err != nil || marshalErr != nil ||
		strings.TrimSpace(settlement.ResultItemDigest) != domainsecurity.CanonicalJSONHash(resultItem) {
		return nil, ErrPublicationIntegrity
	}
	keyID, publicKey, _, err := domainpublication.ReportGrantSettlementAuthorityMaterialV1(settlement)
	if err != nil || domainpublication.ValidateReportGrantSettlementGraphV1(
		settlement,
		domainpublication.ReportGrantSettlementInputV1{
			Decision: decision, Grant: entry.Grant,
			ActiveRegistry: durable.ActiveRegistry, SettledRegistry: durable.SettledRegistry,
			ResultItemID: settlement.ResultItemID, ResultItemDigest: settlement.ResultItemDigest,
			SettledAt: durable.SettledAt, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
		},
	) != nil {
		return nil, ErrPublicationIntegrity
	}
	return durable.ResultItem, nil
}

func validateRestartSuccessfulReportResultV1(
	ctx context.Context,
	settlement domainpublication.ReportGrantSettlementV1,
	decision domainpublication.ReportDeliveryDecisionV1,
	threads restartThreadReaderV1,
) error {
	item, err := resolveRestartGrantSettlementFromThreadV1(ctx, settlement, decision, threads)
	if err != nil {
		return err
	}
	return validateRestartSuccessfulReportResultItemV1(item, decision)
}

func validateRestartSuccessfulReportResultItemV1(
	item map[string]any,
	decision domainpublication.ReportDeliveryDecisionV1,
) error {
	if domaintoolresult.ValidatePrivateAdmittedReportResultItemV1(
		item, decision.DecisionID, decision.RecordDigest,
	) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func stringFieldV1(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
