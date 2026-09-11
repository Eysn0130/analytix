package evidence

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// EvidenceSettlementInventory is a complete startup classification. Prepared
// records without a durable marker remain non-authoritative; Pending contains
// the only repairable state: exact prepared authority plus exact durable
// marker, with no registry issue yet.
type EvidenceSettlementInventory struct {
	Pending     []EvidenceSettlementReconciliationPlan
	Committed   []domainevidence.PreparedEvidenceSettlement
	Abandoned   []domainevidence.PreparedEvidenceSettlement
	Quarantined []domainevidence.PreparedEvidenceSettlement
	Preserved   []domainevidence.PreparedEvidenceSettlement
	preserved   *PreservedEvidenceSettlementInventoryV1
}

type EvidenceSettlementReconciliationPlan struct {
	prepared domainevidence.PreparedEvidenceSettlement
	marker   domainevidence.HostEvidenceSettlementMarker
	settled  time.Time
}

type settlementMarkerAuthority struct {
	marker    domainevidence.HostEvidenceSettlementMarker
	authority executiongrantapp.DurableSettlementAuthority
	order     int
}

type historicalV1EvidenceRegistryAuthority interface {
	EvidenceRegistryHistoricalV1Authority() bool
}

// HasPublicEvidenceSettlementAuthority reports field presence, not validity.
// Its only purpose is preventing unsafe installation-key recreation before
// the trusted preflight can validate the complete state.
func HasPublicEvidenceSettlementAuthority(reader AcceptedFinalPublicReader) (bool, error) {
	if reader == nil {
		return false, errors.New("evidence settlement public reader is unavailable")
	}
	threadIDs, err := strictEvidenceSettlementThreadIDs(reader)
	if err != nil {
		return false, err
	}
	for _, threadID := range threadIDs {
		thread, err := reader.GetThread(threadID)
		if err != nil || thread == nil || strings.TrimSpace(authorityString(thread, "id")) != threadID {
			return false, errors.New("evidence settlement thread inventory is invalid")
		}
		turns, err := strictSettlementTurns(thread)
		if err != nil {
			return false, err
		}
		for _, turn := range turns {
			items, err := strictSettlementItems(turn)
			if err != nil {
				return false, fmt.Errorf("evidence settlement item inventory thread=%s turn=%s: %w",
					settlementAuthorityIDDiagnostic(threadID), settlementAuthorityIDDiagnostic(authorityString(turn, "id")), err)
			}
			for _, item := range items {
				if _, present := item["hostEvidenceSettlement"]; present {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// PreflightEvidenceSettlementInventory validates every prepared record,
// durable marker, and registry issue before returning any repair plan.
func PreflightEvidenceSettlementInventory(ctx context.Context, reader AcceptedFinalPublicReader, issuer Issuer) (EvidenceSettlementInventory, error) {
	return preflightEvidenceSettlementInventoryV1(ctx, reader, issuer, nil, nil)
}

func preflightEvidenceSettlementInventoryV1(ctx context.Context, reader AcceptedFinalPublicReader, issuer Issuer, preserved *PreservedEvidenceSettlementInventoryV1, registrySource any) (EvidenceSettlementInventory, error) {
	return inspectEvidenceSettlementInventoryV1(ctx, reader, issuer, preserved, registrySource, nil)
}

func inspectEvidenceSettlementInventoryV1(ctx context.Context, reader AcceptedFinalPublicReader, issuer Issuer, preserved *PreservedEvidenceSettlementInventoryV1, registrySource any, audit *originalEvidenceSettlementAuditV1) (EvidenceSettlementInventory, error) {
	inventory := EvidenceSettlementInventory{
		Pending: []EvidenceSettlementReconciliationPlan{}, Committed: []domainevidence.PreparedEvidenceSettlement{},
		Abandoned: []domainevidence.PreparedEvidenceSettlement{}, Quarantined: []domainevidence.PreparedEvidenceSettlement{},
	}
	if ctx == nil {
		return EvidenceSettlementInventory{}, errors.New("evidence settlement context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return EvidenceSettlementInventory{}, err
	}
	held := preservedEvidenceSettlementThreadsV1(preserved)
	originalUnavailable := preserved != nil && preserved.RegistryUnavailable != nil
	if originalUnavailable && (!domainsecurity.IsSHA256Hex(preserved.RegistryUnavailable.InventoryDigest) ||
		preserved.HistoricalRegistryV1 || preserved.RegistryHistoryV2 != nil || len(preserved.Registries) != 0 || !isCaseEvidenceAuthorityUnavailableV1(issuer.Registry)) {
		return EvidenceSettlementInventory{}, errors.New("unavailable original registry is not isolated from live authority")
	}
	if registrySource != nil && preserved == nil {
		return EvidenceSettlementInventory{}, errors.New("original registry source lacks preservation observation")
	}
	historyContexts := map[string]bool{}
	if preserved != nil && preserved.RegistryHistoryV2 != nil {
		for _, capsule := range preserved.RegistryHistoryV2.Capsules {
			historyContexts[capsule.SecurityContext.ContextDigest] = true
		}
	}
	registryInventory, ok := issuer.Registry.(registryport.Inventory)
	if reader == nil || issuer.Authority == nil || audit == nil && (!ok || issuer.SettlementStore == nil) {
		return EvidenceSettlementInventory{}, errors.New("evidence settlement reconciliation dependencies are unavailable")
	}
	var preparedRecords []domainevidence.PreparedEvidenceSettlement
	var err error
	if audit != nil {
		preparedRecords = audit.prepared
	} else {
		preparedRecords, err = issuer.SettlementStore.ListPrepared(ctx)
	}
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	preparedByID := make(map[string]domainevidence.PreparedEvidenceSettlement, len(preparedRecords))
	preparedByReceipt := make(map[string]string, len(preparedRecords))
	quarantinedPrepared := make(map[string]bool, len(preparedRecords))
	for _, prepared := range preparedRecords {
		if domainevidence.ValidatePreparedEvidenceSettlement(prepared) != nil {
			return EvidenceSettlementInventory{}, errors.New("evidence settlement inventory contains an invalid prepared record")
		}
		keyID, publicKey, signature, materialErr := domainevidence.EvidenceSettlementAuthorityMaterial(prepared)
		if materialErr != nil {
			return EvidenceSettlementInventory{}, materialErr
		}
		if err := issuer.Authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.EvidenceSettlementSigningBytes(prepared), signature); err != nil {
			return EvidenceSettlementInventory{}, errors.Join(errors.New("evidence settlement inventory contains an untrusted prepared record"), err)
		}
		if prepared.HostAuthority != nil {
			// Check the stored witness as historical data. This supplies no
			// current capability and does not permit Commit.
			if err := domainevidence.ValidatePreparedEvidenceSettlementForHostAuthorityV2(prepared, prepared.SecurityContext, prepared.SourceProbe, prepared.HostAuthority.SelectionDigest); err != nil {
				return EvidenceSettlementInventory{}, err
			}
		}
		if _, duplicate := preparedByID[prepared.SettlementID]; duplicate || preparedByReceipt[prepared.ReceiptID] != "" {
			return EvidenceSettlementInventory{}, errors.New("evidence settlement inventory contains duplicate prepared authority")
		}
		preparedByID[prepared.SettlementID] = prepared
		preparedByReceipt[prepared.ReceiptID] = prepared.SettlementID
		if domainevidence.ValidatePreparedEvidenceSettlementForExecution(prepared) != nil {
			quarantinedPrepared[prepared.SettlementID] = true
		}
	}

	threadIDs, err := strictEvidenceSettlementThreadIDs(reader)
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	contexts := []domainsecurity.TurnSecurityContext{}
	contextByDigest := map[string]domainsecurity.TurnSecurityContext{}
	quarantinedContext := map[string]bool{}
	markers := map[string]settlementMarkerAuthority{}
	markerOrderByContext := map[string][]string{}
	for _, threadID := range threadIDs {
		auditOnly := false
		if quarantine, ok := reader.(interface{ QuarantinedThread(string) bool }); ok {
			auditOnly = quarantine.QuarantinedThread(threadID)
		}
		thread, err := reader.GetThread(threadID)
		if err != nil || thread == nil || strings.TrimSpace(authorityString(thread, "id")) != threadID {
			return EvidenceSettlementInventory{}, errors.New("evidence settlement thread inventory is invalid")
		}
		turns, err := strictSettlementTurns(thread)
		if err != nil {
			return EvidenceSettlementInventory{}, err
		}
		for _, turn := range turns {
			turnID := strings.TrimSpace(authorityString(turn, "id"))
			var securityContext domainsecurity.TurnSecurityContext
			if value, present := turn["securityContext"]; present && value != nil {
				securityContext, err = domainsecurity.ParseTurnSecurityContext(value)
				if err != nil || securityContext.ThreadID != threadID || securityContext.TurnID != turnID {
					return EvidenceSettlementInventory{}, errors.New("evidence settlement turn context is invalid")
				}
				if _, duplicate := contextByDigest[securityContext.ContextDigest]; duplicate {
					return EvidenceSettlementInventory{}, errors.New("evidence settlement turn context is duplicated")
				}
				contextByDigest[securityContext.ContextDigest] = securityContext
				contexts = append(contexts, securityContext)
				if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
					quarantinedContext[securityContext.ContextDigest] = true
				}
			}
			items, err := strictSettlementItems(turn)
			if err != nil {
				return EvidenceSettlementInventory{}, fmt.Errorf("evidence settlement item inventory thread=%s turn=%s: %w",
					settlementAuthorityIDDiagnostic(threadID), settlementAuthorityIDDiagnostic(turnID), err)
			}
			for itemOrder, item := range items {
				value, present := item["hostEvidenceSettlement"]
				if !present {
					continue
				}
				marker, parseErr := domainevidence.ParseHostEvidenceSettlementMarker(value)
				prepared, preparedOK := preparedByID[marker.SettlementID]
				if parseErr != nil || !preparedOK || securityContext.ContextDigest == "" || !reflect.DeepEqual(securityContext, prepared.SecurityContext) {
					return EvidenceSettlementInventory{}, errors.New("evidence settlement durable marker is orphaned or invalid")
				}
				expected, markerErr := domainevidence.NewHostEvidenceSettlementMarker(prepared)
				if markerErr != nil || marker != expected || markers[marker.SettlementID].marker.SettlementID != "" {
					return EvidenceSettlementInventory{}, errors.New("evidence settlement durable marker is duplicated or conflicting")
				}
				if audit != nil || originalUnavailable || quarantinedPrepared[prepared.SettlementID] || historyContexts[securityContext.ContextDigest] {
					if audit != nil || originalUnavailable || held[threadID] || auditOnly || historyContexts[securityContext.ContextDigest] {
						if err := validateHistoricalSettlementPrefixV1(prepared, items); err != nil {
							return EvidenceSettlementInventory{}, err
						}
						settledAt, err := time.Parse(time.RFC3339Nano, settlementStringField(item, "finishedAt"))
						if err != nil {
							return EvidenceSettlementInventory{}, err
						}
						if err := validateEvidenceSettlementResultV1(prepared, securityContext, marker, item, settledAt); err != nil {
							return EvidenceSettlementInventory{}, err
						}
					}
					markers[marker.SettlementID] = settlementMarkerAuthority{marker: marker, order: itemOrder}
					markerOrderByContext[securityContext.ContextDigest] = append(markerOrderByContext[securityContext.ContextDigest], marker.SettlementID)
					continue
				}
				authority, authorityErr := executiongrantapp.DurableSettlementFromThread(threadID, thread, turnID, prepared.ResultItemID, prepared.ExecutionGrant)
				if authorityErr != nil {
					return EvidenceSettlementInventory{}, authorityErr
				}
				commitInput := CommitEvidenceInput{Context: securityContext, Marker: marker, Authority: authority}
				if held[threadID] || auditOnly {
					if err := validateEvidenceSettlementDurableAuthorityV1(prepared, commitInput); err != nil {
						return EvidenceSettlementInventory{}, err
					}
				} else if _, validateErr := issuer.validateCommitAuthority(ctx, commitInput); validateErr != nil {
					return EvidenceSettlementInventory{}, validateErr
				}
				markers[marker.SettlementID] = settlementMarkerAuthority{marker: marker, authority: authority, order: itemOrder}
				markerOrderByContext[securityContext.ContextDigest] = append(markerOrderByContext[securityContext.ContextDigest], marker.SettlementID)
			}
		}
	}
	if audit != nil {
		for _, prepared := range preparedRecords {
			if prepared.SecurityContext != contextByDigest[prepared.SecurityContext.ContextDigest] {
				return EvidenceSettlementInventory{}, errors.New("original settlement prepared context is absent or conflicting")
			}
		}
		// Audit has no registry or store capability and returns no classification
		// or repair plan, including for marker-before-capsule crash prefixes.
		err := validateOriginalSettlementRegistryHistoryV2(ctx, issuer, audit.history, contextByDigest, preparedByID, markers, markerOrderByContext)
		return EvidenceSettlementInventory{}, errors.Join(err, ctx.Err())
	}
	registryUnavailable := isCaseEvidenceAuthorityUnavailableV1(issuer.Registry)
	if registryUnavailable && preserved == nil {
		inventory.Quarantined = append(inventory.Quarantined, preparedRecords...)
		sort.Slice(inventory.Quarantined, func(i, j int) bool {
			return inventory.Quarantined[i].SettlementID < inventory.Quarantined[j].SettlementID
		})
		return inventory, nil
	}

	var registryRecords []registryport.InventoryRecord
	var history *registryport.OriginalHistoryV2
	var unavailable *registryport.UnavailableOriginalInventoryV1
	switch source := registrySource.(type) {
	case EvidenceSettlementRestartRegistryObservationV1:
		var observed registryport.OriginalObservationV1
		observed, err = source.ObserveRestartEvidenceRegistryObservationV1(ctx, contexts)
		registryRecords, history, unavailable = observed.Legacy, observed.HistoryV2, observed.Unavailable
	case EvidenceSettlementRestartRegistryInventoryV2:
		registryRecords, history, err = source.ObserveRestartEvidenceRegistryInventoryV2(ctx, contexts)
	case EvidenceSettlementRestartRegistryInventoryV1:
		registryRecords, err = source.ObserveRestartEvidenceRegistryInventoryV1(ctx, contexts)
	case nil:
		if registryUnavailable {
			// A verified original inventory remains readable history even when
			// the live registry cannot provide a current issuance capability.
			registryRecords = preserved.Registries
		} else {
			registryRecords, err = registryInventory.ListRegistries(ctx, contexts)
		}
	default:
		return EvidenceSettlementInventory{}, errors.New("original registry source is invalid")
	}
	if err != nil {
		return EvidenceSettlementInventory{}, err
	}
	if preserved != nil && !reflect.DeepEqual(history, preserved.RegistryHistoryV2) {
		return EvidenceSettlementInventory{}, errors.New("original registry V2 history changed or is unavailable")
	}
	if unavailable != nil && (len(registryRecords) != 0 || history != nil) ||
		preserved != nil && !reflect.DeepEqual(unavailable, preserved.RegistryUnavailable) {
		return EvidenceSettlementInventory{}, errors.New("original registry unavailable observation changed or is incomplete")
	}
	if err := validateOriginalSettlementRegistryHistoryV2(ctx, issuer, history, contextByDigest, preparedByID, markers, markerOrderByContext); err != nil {
		return EvidenceSettlementInventory{}, err
	}
	if err := validatePreservedEvidenceSettlementRecordsV1(preserved, preparedRecords, registryRecords); err != nil {
		return EvidenceSettlementInventory{}, err
	}
	issues := map[string]bool{}
	registryContexts := map[string]bool{}
	historicalV1, _ := issuer.Registry.(historicalV1EvidenceRegistryAuthority)
	retainHistoricalV1 := historicalV1 != nil && historicalV1.EvidenceRegistryHistoricalV1Authority()
	if registryUnavailable || registrySource != nil {
		retainHistoricalV1 = preserved.HistoricalRegistryV1
	}
	for _, record := range registryRecords {
		contextDigest := record.Context.ContextDigest
		if frozen, known := contextByDigest[contextDigest]; !known || !reflect.DeepEqual(frozen, record.Context) || registryContexts[contextDigest] {
			return EvidenceSettlementInventory{}, errors.New("evidence settlement registry inventory context is duplicated or unknown")
		}
		registryContexts[contextDigest] = true
		if historyContexts[contextDigest] {
			return EvidenceSettlementInventory{}, errors.New("original V2 history overlaps current registry inventory")
		}
		issueOrder := []string{}
		for _, entry := range record.Registry.Entries {
			if entry.Operation != domainevidence.EvidenceRegistryIssue {
				continue
			}
			prepared, preparedOK := preparedByID[entry.SettlementID]
			if issues[entry.SettlementID] {
				return EvidenceSettlementInventory{}, errors.New("evidence settlement registry issue is orphaned or duplicated")
			}
			if !preparedOK {
				if !retainHistoricalV1 {
					return EvidenceSettlementInventory{}, errors.New("evidence settlement registry issue is orphaned or duplicated")
				}
				// The V1 Store already validated this issue through its signed
				// authority chain and exact durable turn context. It remains a
				// historical committed receipt, never a repair candidate and
				// never an input to witnessed V2 issuance.
				issues[entry.SettlementID] = true
				continue
			}
			if _, markerOK := markers[entry.SettlementID]; !markerOK && !quarantinedPrepared[entry.SettlementID] {
				return EvidenceSettlementInventory{}, errors.New("evidence settlement registry issue lacks durable marker authority")
			}
			if _, found, resolveErr := domainevidence.ResolvePreparedSettlementIssue(record.Registry, prepared); resolveErr != nil || !found {
				return EvidenceSettlementInventory{}, errors.New("evidence settlement registry issue conflicts with prepared authority")
			}
			issues[entry.SettlementID] = true
			if quarantinedPrepared[entry.SettlementID] {
				continue
			}
			issueOrder = append(issueOrder, entry.SettlementID)
		}
		if quarantinedContext[contextDigest] {
			continue
		}
		expectedOrder := markerOrderByContext[contextDigest]
		if len(issueOrder) > len(expectedOrder) {
			return EvidenceSettlementInventory{}, errors.New("evidence settlement registry issue order exceeds durable markers")
		}
		for index := range issueOrder {
			if issueOrder[index] != expectedOrder[index] {
				return EvidenceSettlementInventory{}, errors.New("evidence settlement registry contains a non-prefix issue gap")
			}
		}
		if record.Registry.Sequence == 0 && len(expectedOrder) == 0 {
			return EvidenceSettlementInventory{}, errors.New("empty evidence registry is detached from a recoverable settlement")
		}
	}

	for _, prepared := range preparedRecords {
		if held[prepared.SecurityContext.ThreadID] {
			inventory.Preserved = append(inventory.Preserved, prepared)
			continue
		}
		if registryUnavailable || quarantinedPrepared[prepared.SettlementID] || historyContexts[prepared.SecurityContext.ContextDigest] {
			inventory.Quarantined = append(inventory.Quarantined, prepared)
			continue
		}
		if quarantine, ok := reader.(interface{ QuarantinedThread(string) bool }); ok && quarantine.QuarantinedThread(prepared.SecurityContext.ThreadID) {
			inventory.Quarantined = append(inventory.Quarantined, prepared)
			continue
		}
		markerAuthority, hasMarker := markers[prepared.SettlementID]
		hasIssue := issues[prepared.SettlementID]
		switch {
		case !hasMarker && !hasIssue:
			inventory.Abandoned = append(inventory.Abandoned, prepared)
		case hasMarker && hasIssue:
			inventory.Committed = append(inventory.Committed, prepared)
		case hasMarker && !hasIssue:
			inventory.Pending = append(inventory.Pending, EvidenceSettlementReconciliationPlan{
				prepared: prepared, marker: markerAuthority.marker, settled: markerAuthority.authority.SettledAt,
			})
		default:
			return EvidenceSettlementInventory{}, errors.New("evidence settlement inventory contains an orphan registry issue")
		}
	}
	sort.Slice(inventory.Pending, func(i, j int) bool {
		left := markers[inventory.Pending[i].prepared.SettlementID]
		right := markers[inventory.Pending[j].prepared.SettlementID]
		if inventory.Pending[i].prepared.SecurityContext.ThreadID != inventory.Pending[j].prepared.SecurityContext.ThreadID {
			return inventory.Pending[i].prepared.SecurityContext.ThreadID < inventory.Pending[j].prepared.SecurityContext.ThreadID
		}
		if inventory.Pending[i].prepared.SecurityContext.TurnID != inventory.Pending[j].prepared.SecurityContext.TurnID {
			return inventory.Pending[i].prepared.SecurityContext.TurnID < inventory.Pending[j].prepared.SecurityContext.TurnID
		}
		if left.order != right.order {
			return left.order < right.order
		}
		return inventory.Pending[i].prepared.SettlementID < inventory.Pending[j].prepared.SettlementID
	})
	inventory.preserved = preserved
	return inventory, ctx.Err()
}

// ApplyEvidenceSettlementReconciliationInventory repeats the full dry run
// before the first write, re-derives each durable prefix immediately before
// commit, and requires a zero-pending fixed point after all appends.
func ApplyEvidenceSettlementReconciliationInventory(ctx context.Context, reader AcceptedFinalPublicReader, issuer Issuer, expected EvidenceSettlementInventory) error {
	return ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(ctx, reader, issuer, expected, nil)
}

func ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(ctx context.Context, reader AcceptedFinalPublicReader, issuer Issuer, expected EvidenceSettlementInventory, observer EvidenceSettlementRestartPreservationV1) error {
	current, err := PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, reader, issuer, observer)
	if err != nil {
		return err
	}
	if settlementPlanDigest(current.Pending) != settlementPlanDigest(expected.Pending) || !reflect.DeepEqual(current.preserved, expected.preserved) || !reflect.DeepEqual(current.Preserved, expected.Preserved) {
		return errors.New("evidence settlement reconciliation inventory changed after preflight")
	}
	guardedIssuer := issuer
	guardedIssuer.beforeSettlementCommitV1 = func(ctx context.Context) error {
		if issuer.beforeSettlementCommitV1 != nil {
			if err := issuer.beforeSettlementCommitV1(ctx); err != nil {
				return err
			}
		}
		observed, err := PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, reader, issuer, observer)
		if err != nil || !reflect.DeepEqual(observed.preserved, current.preserved) || !reflect.DeepEqual(observed.Preserved, current.Preserved) {
			return errors.Join(errors.New("original settlement changed before registry commit"), err)
		}
		return nil
	}
	for _, plan := range current.Pending {
		thread, err := reader.GetThread(plan.prepared.SecurityContext.ThreadID)
		if err != nil || thread == nil {
			return errors.New("evidence settlement reconciliation thread disappeared")
		}
		authority, err := executiongrantapp.DurableSettlementFromThread(
			plan.prepared.SecurityContext.ThreadID, thread, plan.prepared.SecurityContext.TurnID,
			plan.prepared.ResultItemID, plan.prepared.ExecutionGrant,
		)
		if err != nil || !authority.SettledAt.Equal(plan.settled) {
			return errors.New("evidence settlement reconciliation authority changed")
		}
		if _, err := guardedIssuer.Commit(ctx, CommitEvidenceInput{Context: plan.prepared.SecurityContext, Marker: plan.marker, Authority: authority}); err != nil {
			return err
		}
	}
	verified, err := PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, reader, issuer, observer)
	if err != nil {
		return err
	}
	if len(verified.Pending) != 0 || len(verified.Committed) != len(current.Committed)+len(current.Pending) ||
		len(verified.Abandoned) != len(current.Abandoned) || len(verified.Quarantined) != len(current.Quarantined) ||
		!reflect.DeepEqual(verified.preserved, current.preserved) || !reflect.DeepEqual(verified.Preserved, current.Preserved) {
		return errors.New("evidence settlement reconciliation did not reach a fixed point")
	}
	return nil
}

func strictEvidenceSettlementThreadIDs(reader AcceptedFinalPublicReader) ([]string, error) {
	threadIDs, err := reader.AllThreadIDs()
	if err != nil {
		return nil, err
	}
	sort.Strings(threadIDs)
	for index, threadID := range threadIDs {
		if strings.TrimSpace(threadID) == "" || (index > 0 && threadIDs[index-1] == threadID) {
			return nil, errors.New("evidence settlement thread inventory contains an invalid or duplicate id")
		}
	}
	return threadIDs, nil
}

func strictSettlementTurns(thread map[string]any) ([]map[string]any, error) {
	values, ok := thread["turns"].([]any)
	if !ok || values == nil {
		return nil, errors.New("evidence settlement thread turns are invalid")
	}
	turns := make([]map[string]any, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		turn, ok := value.(map[string]any)
		turnID := strings.TrimSpace(authorityString(turn, "id"))
		if !ok || turn == nil || turnID == "" || seen[turnID] {
			return nil, errors.New("evidence settlement thread contains an invalid or duplicate turn")
		}
		seen[turnID] = true
		turns = append(turns, turn)
	}
	return turns, nil
}

func strictSettlementItems(turn map[string]any) ([]map[string]any, error) {
	value, present := turn["items"]
	if !present || value == nil {
		return []map[string]any{}, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, errors.New("evidence settlement turn items are invalid")
	}
	items := make([]map[string]any, 0, len(values))
	seen := map[string]bool{}
	for index, value := range values {
		item, ok := value.(map[string]any)
		itemID := strings.TrimSpace(authorityString(item, "id"))
		if !ok || item == nil {
			return nil, fmt.Errorf("evidence settlement turn contains an invalid or duplicate item code=type index=%d id=empty", index)
		}
		if itemID == "" {
			return nil, fmt.Errorf("evidence settlement turn contains an invalid or duplicate item code=missing_id index=%d id=empty", index)
		}
		if seen[itemID] {
			return nil, fmt.Errorf("evidence settlement turn contains an invalid or duplicate item code=duplicate_id index=%d id=%s",
				index, settlementAuthorityIDDiagnostic(itemID))
		}
		seen[itemID] = true
		items = append(items, item)
	}
	return items, nil
}

func settlementAuthorityIDDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	return domainsecurity.SHA256Hex([]byte(value))[:16]
}

func settlementPlanDigest(plans []EvidenceSettlementReconciliationPlan) string {
	parts := make([]string, 0, len(plans))
	for _, plan := range plans {
		parts = append(parts, plan.prepared.SettlementID+"\x00"+plan.marker.MarkerDigest+"\x00"+plan.settled.UTC().Format(time.RFC3339Nano))
	}
	return domainsecurity.SHA256Hex([]byte(strings.Join(parts, "\x00")))
}
