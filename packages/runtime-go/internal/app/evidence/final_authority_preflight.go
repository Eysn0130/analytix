package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type AcceptedFinalPublicReader interface {
	AllThreadIDs() ([]string, error)
	GetThread(string) (map[string]any, error)
}

// The root may provide a record-bound reader over authenticated Original
// history. It supplies content for boundary finals only, never current
// membership or fact publication authority.
type originalAcceptedFinalReplayV1 interface {
	ReplayOriginalAcceptedFinalV1(context.Context, domainevidence.PrivateAcceptedFinalRecord) (domainevidence.EvidenceReceiptRegistry, error)
}

type caseCompactionAuthorityReaderV1 interface {
	ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
}

func validateCaseCompactionReaderAuthorityV1(
	reader AcceptedFinalPublicReader,
	threadID string,
	thread map[string]any,
	turn map[string]any,
) error {
	validator, ok := reader.(caseCompactionAuthorityReaderV1)
	if !ok {
		return errors.New("case compaction committed authority reader is unavailable")
	}
	return validator.ValidateCaseCompactionAuthorityTurnV1(threadID, thread, turn)
}

func HasPublicAcceptedFinalAuthority(reader AcceptedFinalPublicReader) (bool, error) {
	if reader == nil {
		return false, errors.New("accepted final public reader is unavailable")
	}
	threadIDs, err := reader.AllThreadIDs()
	if err != nil {
		return false, err
	}
	for _, threadID := range threadIDs {
		thread, err := reader.GetThread(threadID)
		if err != nil {
			return false, err
		}
		for _, turn := range authorityTurns(thread) {
			for _, value := range authorityRecordValues(turn) {
				if _, present := authoritySchemaVersion(value); present {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

type FinalAuthorityInventory struct {
	Committed              []domainevidence.PrivateAcceptedFinalRecord
	CommittedDispositions  []domainevidence.AcceptedFinalDispositionRecord
	NotCommitted           []domainevidence.PrivateAcceptedFinalRecord
	AuditOnlyPublicWinners []domainevidence.PrivateAcceptedFinalRecord
	AuditOnlyNotCommitted  []domainevidence.PrivateAcceptedFinalRecord
	PublicCommitRepairs    []AcceptedFinalPublicCommitRepair
}

func PreflightFinalAuthority(ctx context.Context, reader AcceptedFinalPublicReader, casReader authorityport.AcceptedFinalCASReader, historical registryport.HistoricalReplay, authority authorityport.Authority, privateStore authorityport.PrivateFinalInventoryStore) error {
	_, err := PreflightFinalAuthorityInventory(ctx, reader, casReader, historical, authority, privateStore)
	return err
}

func PreflightFinalAuthorityInventory(ctx context.Context, reader AcceptedFinalPublicReader, casReader authorityport.AcceptedFinalCASReader, historical registryport.HistoricalReplay, authority authorityport.Authority, privateStore authorityport.PrivateFinalInventoryStore) (FinalAuthorityInventory, error) {
	if reader == nil || casReader == nil || historical == nil || authority == nil || privateStore == nil {
		return FinalAuthorityInventory{}, errors.New("final authority preflight dependencies are unavailable")
	}
	privateRecords := make([]domainevidence.PrivateAcceptedFinalRecord, 0)
	privateByDigest := map[string]domainevidence.PrivateAcceptedFinalRecord{}
	privateByTurn := map[string]map[string]bool{}
	registryUnavailable := isCaseEvidenceAuthorityUnavailableV1(historical)
	if err := privateStore.VisitAcceptedFinals(ctx, func(record domainevidence.PrivateAcceptedFinalRecord) error {
		var err error
		_, originalBoundary := historical.(originalAcceptedFinalReplayV1)
		originalBoundary = originalBoundary && record.RegistryHead.Sequence > 0 &&
			!domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Envelope.Variant) && record.AcceptedFinal.FactFinalWitnessAdmission == nil
		if registryUnavailable && !originalBoundary {
			err = verifyTrustedPrivateFinalInstallationV1(ctx, authority, record)
		} else {
			err = verifyTrustedPrivateFinal(ctx, historical, authority, record)
		}
		if err != nil {
			return err
		}
		digest := record.AcceptedFinal.RecordDigest
		if _, duplicate := privateByDigest[digest]; duplicate {
			return errors.New("duplicate private accepted final authority record")
		}
		privateRecords = append(privateRecords, record)
		privateByDigest[digest] = record
		turnKey := authorityTurnKey(record.SecurityContext.ThreadID, record.SecurityContext.TurnID)
		if privateByTurn[turnKey] == nil {
			privateByTurn[turnKey] = map[string]bool{}
		}
		privateByTurn[turnKey][digest] = true
		return nil
	}); err != nil {
		return FinalAuthorityInventory{}, err
	}
	sort.Slice(privateRecords, func(left, right int) bool {
		return privateRecords[left].AcceptedFinal.RecordDigest < privateRecords[right].AcceptedFinal.RecordDigest
	})
	dispositionByDigest := map[string]domainevidence.AcceptedFinalDispositionRecord{}
	if err := privateStore.VisitDispositions(ctx, func(disposition domainevidence.AcceptedFinalDispositionRecord) error {
		if _, duplicate := dispositionByDigest[disposition.AcceptedFinalDigest]; duplicate {
			return errors.New("duplicate accepted final disposition authority record")
		}
		privateRecord, ok := privateByDigest[disposition.AcceptedFinalDigest]
		if !ok || disposition.PrivateRecordDigest != privateRecord.PrivateRecordDigest ||
			disposition.ThreadID != privateRecord.SecurityContext.ThreadID || disposition.TurnID != privateRecord.SecurityContext.TurnID {
			return errors.New("accepted final disposition is detached from its private authority")
		}
		keyID, publicKey, signature, err := domainevidence.AcceptedFinalDispositionAuthorityMaterial(disposition)
		if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalDispositionSigningBytes(disposition), signature) != nil {
			return errors.New("accepted final disposition is not signed by the trusted installation authority")
		}
		plan, err := buildAcceptedFinalPreflightPlan(privateRecord)
		if err != nil || plan.EventManifestDigest != disposition.EventManifestDigest {
			return errors.New("accepted final disposition event manifest is inconsistent")
		}
		dispositionByDigest[disposition.AcceptedFinalDigest] = disposition
		return nil
	}); err != nil {
		return FinalAuthorityInventory{}, err
	}
	publicDigests, observations, err := validatePublicFinalAuthority(ctx, reader, casReader, privateByDigest, privateByTurn)
	if err != nil {
		return FinalAuthorityInventory{}, err
	}
	inventory := FinalAuthorityInventory{
		Committed: []domainevidence.PrivateAcceptedFinalRecord{}, CommittedDispositions: []domainevidence.AcceptedFinalDispositionRecord{},
		NotCommitted: []domainevidence.PrivateAcceptedFinalRecord{}, AuditOnlyPublicWinners: []domainevidence.PrivateAcceptedFinalRecord{},
		AuditOnlyNotCommitted: []domainevidence.PrivateAcceptedFinalRecord{}, PublicCommitRepairs: []AcceptedFinalPublicCommitRepair{},
	}
	preparedTurns := map[string]bool{}
	for _, privateRecord := range privateRecords {
		digest := privateRecord.AcceptedFinal.RecordDigest
		turnKey := authorityTurnKey(privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
		observation, cached := observations[turnKey]
		var observationErr error
		if !cached {
			observation, observationErr = casReader.ReadAcceptedFinalCASObservation(
				ctx, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID,
			)
			if observationErr == nil {
				observations[turnKey] = observation
			}
		}
		if observationErr != nil || domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil {
			return FinalAuthorityInventory{}, errors.New("private accepted final lacks an exact primary CAS observation")
		}
		disposition, dispositionFound := dispositionByDigest[digest]
		state := disposition.State
		auditOnlyRecord := privateRecord.SchemaVersion != domainevidence.PrivateAcceptedFinalRecordVersion ||
			domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateRecord.Envelope)
		if !dispositionFound {
			var classifiedState domainevidence.AcceptedFinalDispositionState
			var repair *AcceptedFinalPublicCommitRepair
			var classifyErr error
			if auditOnlyRecord {
				classifiedState, classifyErr = classifyAuditOnlyPrivateFinal(privateRecord, observation)
			} else {
				classifiedState, repair, classifyErr = classifyUnresolvedPrivateFinal(privateRecord, observation)
			}
			if classifyErr != nil {
				return FinalAuthorityInventory{}, classifyErr
			}
			if repair != nil {
				if preparedTurns[turnKey] {
					return FinalAuthorityInventory{}, errors.New("multiple unresolved accepted finals target the same active turn")
				}
				preparedTurns[turnKey] = true
				inventory.PublicCommitRepairs = append(inventory.PublicCommitRepairs, *repair)
				continue
			}
			state = classifiedState
		}
		if dispositionFound && disposition.SchemaVersion == domainevidence.AcceptedFinalDispositionRecordV2 &&
			(disposition.TurnCASDigest != observation.TurnProjectionSHA256 || !observation.HasWinner || disposition.WinnerDigest != observation.Winner.RecordDigest) {
			return FinalAuthorityInventory{}, errors.New("accepted final disposition V2 does not match the exact surviving CAS")
		}
		switch state {
		case domainevidence.AcceptedFinalCommitted:
			if !publicDigests[digest] || !observation.HasWinner || observation.Winner.RecordDigest != digest {
				return FinalAuthorityInventory{}, errors.New("committed accepted final is missing its public authority")
			}
			if auditOnlyRecord {
				inventory.AuditOnlyPublicWinners = append(inventory.AuditOnlyPublicWinners, privateRecord)
			} else {
				inventory.Committed = append(inventory.Committed, privateRecord)
			}
			if dispositionFound && !auditOnlyRecord {
				inventory.CommittedDispositions = append(inventory.CommittedDispositions, disposition)
			}
		case domainevidence.AcceptedFinalExplicitlyNotCommitted:
			if publicDigests[digest] || observation.HasWinner && observation.Winner.RecordDigest == digest {
				return FinalAuthorityInventory{}, errors.New("explicitly uncommitted accepted final has public authority")
			}
			if auditOnlyRecord {
				inventory.AuditOnlyNotCommitted = append(inventory.AuditOnlyNotCommitted, privateRecord)
			} else {
				inventory.NotCommitted = append(inventory.NotCommitted, privateRecord)
			}
		default:
			return FinalAuthorityInventory{}, errors.New("accepted final disposition state is unknown")
		}
	}
	return inventory, nil
}

func validatePublicFinalAuthority(
	ctx context.Context,
	reader AcceptedFinalPublicReader,
	casReader authorityport.AcceptedFinalCASReader,
	privateByPublicDigest map[string]domainevidence.PrivateAcceptedFinalRecord,
	privateByTurn map[string]map[string]bool,
) (map[string]bool, map[string]domainevidence.AcceptedFinalCASObservationV1, error) {
	threadIDs, err := reader.AllThreadIDs()
	if err != nil {
		return nil, nil, err
	}
	seenPublic := map[string]bool{}
	observations := map[string]domainevidence.AcceptedFinalCASObservationV1{}
	for _, threadID := range threadIDs {
		thread, err := reader.GetThread(threadID)
		if err != nil {
			return nil, nil, err
		}
		for _, turn := range authorityTurns(thread) {
			value := turn["acceptedFinal"]
			version, present := authoritySchemaVersion(value)
			turnID := strings.TrimSpace(authorityString(turn, "id"))
			privateDigestsAtTurn := privateByTurn[authorityTurnKey(threadID, turnID)]
			if strings.TrimSpace(authorityString(turn, "caseHistoryProjection")) == "compaction_authority_v1" {
				if err := validateCaseCompactionReaderAuthorityV1(reader, strings.TrimSpace(threadID), thread, turn); err != nil {
					return nil, nil, errors.Join(errors.New("case compaction authority marker is invalid"), err)
				}
				if err := validateAcceptedFinalItems(turn, nil); err != nil {
					return nil, nil, err
				}
				continue
			}
			caseBound, caseContextErr := authorityCaseBoundTurn(threadID, turn)
			if caseContextErr != nil {
				return nil, nil, caseContextErr
			}
			if caseBound && authorityTerminalStatus(authorityString(turn, "status")) && !present {
				return nil, nil, errors.New("terminal case turn is missing accepted-final history")
			}
			if !present {
				if err := validateAcceptedFinalItems(turn, nil); err != nil {
					return nil, nil, err
				}
				continue
			}
			if version == domainevidence.LegacyAcceptedFinalRecordVersion {
				return nil, nil, errors.New("legacy accepted final requires a trusted signed cutover inventory")
			}
			if !replayableAcceptedFinalAuthorityVersion(version) {
				return nil, nil, errors.New("public accepted final has an unknown authority version")
			}
			publicRecord, err := domainevidence.ParseAcceptedFinalRecord(value)
			if err != nil {
				return nil, nil, err
			}
			observation, err := casReader.ReadAcceptedFinalCASObservation(ctx, threadID, turnID)
			if err != nil || domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil || !observation.HasWinner ||
				!reflect.DeepEqual(observation.Winner, publicRecord) {
				return nil, nil, errors.New("public accepted final is not present in the exact primary CAS record")
			}
			observations[authorityTurnKey(threadID, turnID)] = observation
			if seenPublic[publicRecord.RecordDigest] || (len(privateDigestsAtTurn) != 0 && !privateDigestsAtTurn[publicRecord.RecordDigest]) {
				return nil, nil, errors.New("public accepted final authority is duplicated")
			}
			seenPublic[publicRecord.RecordDigest] = true
			privateRecord, ok := privateByPublicDigest[publicRecord.RecordDigest]
			if !ok || !reflect.DeepEqual(privateRecord.AcceptedFinal, publicRecord) {
				return nil, nil, errors.New("public accepted final is missing its private authority record")
			}
			securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			if err != nil || !reflect.DeepEqual(securityContext, privateRecord.SecurityContext) ||
				publicRecord.ThreadID != strings.TrimSpace(threadID) || publicRecord.TurnID != strings.TrimSpace(authorityString(turn, "id")) {
				return nil, nil, errors.New("public accepted final does not match its frozen turn context")
			}
			expectedStatus, ok := domainevidence.FinalAnswerTerminalStatus(publicRecord.TerminalReason)
			if !ok || strings.TrimSpace(authorityString(turn, "status")) != expectedStatus {
				return nil, nil, errors.New("public accepted final terminal status is inconsistent")
			}
			if err := validateAcceptedFinalItems(turn, &publicRecord); err != nil {
				return nil, nil, err
			}
			if err := validateAcceptedFinalPublicationItems(turn, privateRecord); err != nil {
				return nil, nil, err
			}
		}
	}
	return seenPublic, observations, nil
}

func validateAcceptedFinalPublicationItems(turn map[string]any, privateRecord domainevidence.PrivateAcceptedFinalRecord) error {
	plan, err := buildAcceptedFinalPreflightPlan(privateRecord)
	if err != nil {
		return err
	}
	expected := make(map[string]map[string]any, len(plan.TurnItems))
	for _, item := range plan.TurnItems {
		expected[authorityString(item, "id")] = item
	}
	seen := map[string]bool{}
	items, _ := turn["items"].([]any)
	for _, value := range items {
		item, _ := value.(map[string]any)
		itemID := authorityString(item, "id")
		expectedItem, isExpected := expected[itemID]
		owned := isExpected || item["acceptedFinal"] != nil || authorityString(item, "acceptedFinalDigest") == privateRecord.AcceptedFinal.RecordDigest
		if !owned {
			continue
		}
		if !isExpected || seen[itemID] || canonicalAuthorityDigest(item) != canonicalAuthorityDigest(expectedItem) {
			return errors.New("accepted final public turn item manifest is inconsistent")
		}
		seen[itemID] = true
	}
	if len(seen) != len(expected) {
		return errors.New("accepted final public turn item manifest is incomplete")
	}
	if discard, required := plan.TurnFields["discard"]; required && turn["discard"] != discard {
		return errors.New("accepted final public turn discard state is inconsistent")
	}
	expectedView, requiresView := plan.TurnFields["acceptedFinalView"]
	if requiresView {
		if canonicalAuthorityDigest(turn["acceptedFinalView"]) != canonicalAuthorityDigest(expectedView) {
			return errors.New("accepted final public turn view is inconsistent")
		}
	} else if turn["acceptedFinalView"] != nil {
		return errors.New("historical accepted final carries an unsigned public view")
	}
	return nil
}

func buildAcceptedFinalPreflightPlan(record domainevidence.PrivateAcceptedFinalRecord) (appturn.AcceptedFinalPublicationPlan, error) {
	if record.SchemaVersion == domainevidence.PrivateAcceptedFinalRecordVersion &&
		record.AcceptedFinal.SchemaVersion == domainevidence.AcceptedFinalRecordVersion {
		return appturn.BuildAcceptedFinalPublicationPlan(record.AcceptedFinal, record.RenderedText, record.PublicationIntent)
	}
	return appturn.BuildAcceptedFinalAuditPublicationPlan(record.AcceptedFinal, record.RenderedText, record.PublicationIntent)
}

func canonicalAuthorityDigest(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return domainsecurity.SHA256Hex(body)
}

func verifyTrustedPrivateFinal(ctx context.Context, historical registryport.HistoricalReplay, authority authorityport.Authority, record domainevidence.PrivateAcceptedFinalRecord) error {
	if ctx == nil {
		return errors.New("private accepted final verification context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyTrustedPrivateFinalInstallationV1(ctx, authority, record); err != nil {
		return err
	}
	var snapshot domainevidence.EvidenceReceiptRegistry
	var err error
	if record.RegistryHead.Sequence == 0 {
		// Sequence zero has one context-bound historical prefix. Verify it
		// against the signed full head below; this says nothing about the
		// current registry and is never a fallback after a failed replay.
		snapshot, err = domainevidence.NewEvidenceReceiptRegistry(record.SecurityContext)
	} else if original, ok := historical.(originalAcceptedFinalReplayV1); ok &&
		!domainevidence.FinalAnswerVariantRequiresPublicationSnapshotProof(record.Envelope.Variant) && record.AcceptedFinal.FactFinalWitnessAdmission == nil {
		snapshot, err = original.ReplayOriginalAcceptedFinalV1(ctx, record)
	} else {
		snapshot, err = historical.ReplayAt(ctx, record.SecurityContext, record.RegistryHead.Sequence)
	}
	if err != nil {
		return err
	}
	head, err := domainevidence.NewEvidenceRegistryHead(snapshot)
	if err != nil || !reflect.DeepEqual(head, record.RegistryHead) {
		return errors.New("private accepted final registry head is unavailable or inconsistent")
	}
	if err := domainevidence.ValidatePublicationSnapshotProofAgainstRegistry(record.PublicationSnapshotProof, record.SecurityContext, record.Envelope, snapshot); err != nil {
		return err
	}
	if err := verifyEnvelopeAtRegistrySnapshot(ctx, record, snapshot); err != nil {
		return err
	}
	return ctx.Err()
}

func verifyTrustedPrivateFinalInstallationV1(ctx context.Context, authority authorityport.Authority, record domainevidence.PrivateAcceptedFinalRecord) error {
	if err := domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(record); err != nil {
		return err
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record.AcceptedFinal)
	if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(record.AcceptedFinal), signature) != nil {
		return errors.New("private accepted final is not signed by the trusted installation authority")
	}
	return nil
}

func classifyAuditOnlyPrivateFinal(
	record domainevidence.PrivateAcceptedFinalRecord,
	observation domainevidence.AcceptedFinalCASObservationV1,
) (domainevidence.AcceptedFinalDispositionState, error) {
	if domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(record) != nil ||
		domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil ||
		observation.ThreadID != record.SecurityContext.ThreadID || observation.TurnID != record.SecurityContext.TurnID ||
		!reflect.DeepEqual(observation.FrozenContext, record.SecurityContext) {
		return "", errors.New("audit-only accepted final primary CAS observation is invalid")
	}
	if observation.HasWinner && observation.Winner.RecordDigest == record.AcceptedFinal.RecordDigest {
		return domainevidence.AcceptedFinalCommitted, nil
	}
	return domainevidence.AcceptedFinalExplicitlyNotCommitted, nil
}

func verifyEnvelopeAtRegistrySnapshot(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, snapshot domainevidence.EvidenceReceiptRegistry) error {
	issuedAt, err := time.Parse(time.RFC3339Nano, record.Envelope.IssuedAt)
	if err != nil {
		return err
	}
	input := FinalGateInput{
		Context: record.SecurityContext, TerminalReason: TerminalReason(record.Envelope.TerminalReason), Claims: record.Envelope.Claims,
		OrdinaryResult: record.Envelope.OrdinaryResult,
		Blocker:        record.Envelope.Blocker, CheckedScope: record.Envelope.CheckedScope, MissingScope: record.Envelope.MissingScope,
		AcquisitionSteps: record.Envelope.AcquisitionSteps, GeneralGuidance: record.Envelope.Guidance, IssuedAt: issuedAt,
	}
	switch record.Envelope.Variant {
	case domainevidence.SourceUnavailableAnswer:
		input.SourceUnavailable = true
	case domainevidence.VerifiedNoHitAnswer:
		input.NoHitReceiptIDs = record.Envelope.EvidenceReceiptIDs
	}
	verified, err := (FinalEvidenceGate{Registry: newSnapshotRegistry(snapshot)}).Finalize(ctx, input)
	if err != nil || verified.EnvelopeDigest != record.Envelope.EnvelopeDigest {
		return errors.New("private accepted final cannot be reproduced from its sealed registry snapshot")
	}
	rendered, err := RenderFinalAnswer(verified)
	if err != nil || rendered != record.RenderedText {
		return errors.New("private accepted final deterministic rendering is inconsistent")
	}
	return nil
}

func validateAcceptedFinalItems(turn map[string]any, expected *domainevidence.AcceptedFinalRecord) error {
	matched := 0
	items, _ := turn["items"].([]any)
	for _, itemValue := range items {
		item, _ := itemValue.(map[string]any)
		value := item["acceptedFinal"]
		version, present := authoritySchemaVersion(value)
		kind := strings.TrimSpace(authorityString(item, "kind"))
		if !present {
			if expected != nil && kind == "assistant_text" {
				return errors.New("accepted final turn contains an unsigned assistant item")
			}
			continue
		}
		if version == domainevidence.LegacyAcceptedFinalRecordVersion {
			return errors.New("legacy accepted final item appears outside a validated legacy turn")
		}
		if !replayableAcceptedFinalAuthorityVersion(version) {
			return errors.New("accepted final item has an unknown authority version")
		}
		record, err := domainevidence.ParseAcceptedFinalRecord(value)
		text, textOK := item["text"].(string)
		if err != nil || expected == nil || kind != "assistant_text" || authorityString(item, "role") != "assistant" ||
			!reflect.DeepEqual(record, *expected) || authorityString(item, "threadId") != expected.ThreadID ||
			authorityString(item, "turnId") != expected.TurnID || !textOK || domainsecurity.SHA256Hex([]byte(text)) != expected.RenderedTextSHA256 {
			return errors.New("accepted final item is detached from turn authority")
		}
		matched++
	}
	if expected != nil && matched != 1 {
		return fmt.Errorf("accepted final turn requires exactly one public item, found %d", matched)
	}
	return nil
}

func replayableAcceptedFinalAuthorityVersion(version int) bool {
	return version == domainevidence.AcceptedFinalRecordVersion ||
		version == domainevidence.BoundaryAcceptedFinalRecordVersion ||
		version == domainevidence.WitnessedFactAcceptedFinalRecordVersion ||
		version == domainevidence.PreviousAcceptedFinalRecordVersion
}

func authorityTurns(thread map[string]any) []map[string]any {
	values, _ := thread["turns"].([]any)
	turns := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if turn, ok := value.(map[string]any); ok {
			turns = append(turns, turn)
		}
	}
	return turns
}

func authorityRecordValues(turn map[string]any) []any {
	values := []any{turn["acceptedFinal"]}
	items, _ := turn["items"].([]any)
	for _, itemValue := range items {
		if item, ok := itemValue.(map[string]any); ok {
			values = append(values, item["acceptedFinal"])
		}
	}
	return values
}

func authoritySchemaVersion(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	record, ok := value.(map[string]any)
	if !ok || record == nil {
		return 0, true
	}
	switch version := record["schemaVersion"].(type) {
	case int:
		return version, true
	case int64:
		return int(version), true
	case float64:
		parsed := int(version)
		if version != float64(parsed) {
			return 0, true
		}
		return parsed, true
	case json.Number:
		parsed, err := version.Int64()
		if err != nil {
			return 0, true
		}
		return int(parsed), true
	default:
		return 0, true
	}
}

func authorityTurnKey(threadID, turnID string) string {
	return strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(turnID)
}

func authorityCaseBoundTurn(threadID string, turn map[string]any) (bool, error) {
	value := turn["securityContext"]
	if value == nil {
		return false, nil
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(value)
	if err != nil {
		if !authorityTerminalStatus(authorityString(turn, "status")) {
			return false, nil
		}
		return false, errors.New("turn contains an invalid frozen security context")
	}
	if securityContext.ThreadID != strings.TrimSpace(threadID) || securityContext.TurnID != strings.TrimSpace(authorityString(turn, "id")) {
		return false, errors.New("turn identity contradicts its frozen security context")
	}
	return domainsecurity.TurnSecurityContextIsCaseSensitive(securityContext), nil
}

func authorityTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "aborted":
		return true
	default:
		return false
	}
}

func authorityString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
