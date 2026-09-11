package evidence

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	settlementport "analytix.local/runtime-go/internal/ports/evidencesettlement"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const accountFlowEvidenceSourceFileIDV1 = "0123456789abcdefabcd"

func TestOriginalOutcomeAcceptsAccountFlowPartial(t *testing.T) {
	_, _, input := toolEvidenceServiceFixture(t, "2")
	input.Call.Name = "mcp__analytix_funds__analyze_account_flows"
	input.Grant.ToolName = input.Call.Name
	input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticPartial,
		PartialCoverage: map[string]any{"coverageStatus": "partial"}, IssuedAt: evidenceIssuerTime(),
	})
	pending := appmodel.PendingToolCall{Call: input.Call, SecurityContext: input.Context, ExecutionGrant: input.Grant}
	record := map[string]any{"executed": true, "toolOutcome": domainevidence.ToolOutcomeRecord(input.Outcome)}
	outcome, eligible, err := originalOutcomeForEvidence(record, false, pending)
	if err != nil || !eligible || outcome.SemanticStatus != domainevidence.SemanticPartial {
		t.Fatalf("account-flow partial outcome was not eligible: outcome=%#v eligible=%v err=%v", outcome, eligible, err)
	}
}

func TestAccountFlowNormalizerProducesPIIFreeCanonicalMaterial(t *testing.T) {
	_, reader, input := toolEvidenceServiceFixture(t, "1")
	input.Call.Name = fundsAccountFlowCanonicalTool
	input.Grant.ToolName = input.Call.Name
	input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticPartial,
		PartialCoverage: map[string]any{"coverageStatus": "partial"}, IssuedAt: evidenceIssuerTime(),
	})
	input.Binding, _ = domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: input.Context.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid,
		CaseID: input.Context.CaseID, BindingSHA256: domainsecurity.SHA256Hex([]byte("account-binding")),
		CaseBindingHash: input.Context.CaseBindingHash,
	})
	const subject = "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	semantic := domainnative.AccountFlowProviderSemanticResultV1{
		SubjectAlias: "acct:1", StartInclusive: "2026-01-01T00:00:00.000000Z", EndInclusive: "2026-01-31T23:59:59.000000Z",
		Timezone: "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: "100", OutflowMinor: "0", NetMinor: "100", TransactionCount: 1,
		EvidenceTransactionCount: 1, EvidenceRowLimit: 1, AggregateComplete: true, EvidenceRowsComplete: true,
		CounterpartySemanticsComplete: false,
		Currentness:                   domainnative.AccountFlowProviderCurrentnessCurrentV1,
		Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{State: domainnative.AccountFlowCoveragePartialV1,
			Gaps: []string{domainnative.AccountFlowGapCounterpartyResolutionV1}, NormalizedSnapshotRows: 1,
			AcceptedSnapshotRows: 1, ObservedMatchingRows: 1},
		QueryHash: domainsecurity.SHA256Hex([]byte("account-flow-query")), ResultHash: domainsecurity.SHA256Hex([]byte("account-flow-result")),
		Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{{
			EvidenceRef:  "srow1_" + domainsecurity.SHA256Hex([]byte("account-flow-row")),
			Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
			OccurredAt:   "2026-01-05T10:30:00.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1,
			AmountMinor: "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		}},
	}
	setAccountFlowProviderOutcomeForEvidenceTestV1(t, &semantic)
	capture := &accountFlowEvidenceCapture{}
	if err := capture.consumeSummary(subject, input.Context.DatasetSnapshotID, input.Context.ContextEpoch, input.Context.ContextDigest,
		input.Context.CaseBindingHash, semantic.StartInclusive, semantic.EndInclusive, semantic.Timezone, semantic.Currency,
		semantic.MinorUnitScale, semantic.InflowMinor, semantic.OutflowMinor, semantic.NetMinor, semantic.TransactionCount,
		semantic.AggregateComplete, semantic.EvidenceRowsComplete, semantic.EvidenceTransactionCount, semantic.Coverage,
		semantic.QueryHash, semantic.ResultHash); err != nil {
		t.Fatal(err)
	}
	if err := capture.consumeRow(0, subject, semantic.Transactions[0].EvidenceRef, accountFlowEvidenceSourceFileIDV1, 7,
		semantic.Transactions[0].OccurredAt, semantic.Transactions[0].Direction, semantic.Transactions[0].AmountMinor,
		semantic.Transactions[0].Currency, semantic.Transactions[0].MinorUnitScale); err != nil {
		t.Fatal(err)
	}
	raw := losslessEvidenceCandidate(t, map[string]any{"structuredContent": map[string]any{"safe": true}, "hostOnly": "/private/case.duckdb"})
	material, err := normalizeAccountFlowEvidence(input, reader.probe, raw, semantic, capture)
	if err != nil {
		t.Fatalf("account-flow normalizer rejected safe semantic material: %v", err)
	}
	canonicalText := string(material.CanonicalEvidence)
	if strings.Contains(canonicalText, "/private/case.duckdb") ||
		!strings.Contains(canonicalText, accountFlowEvidenceSourceFileIDV1) ||
		!strings.Contains(canonicalText, `"sourceRowNumber":7`) {
		t.Fatalf("canonical account-flow material did not retain only its private opaque source lineage: %s", canonicalText)
	}
	parsed, err := domainevidence.ParseCanonicalEvidenceMaterial(material.CanonicalEvidence)
	if err != nil || len(parsed.Facts) != 3 || material.SourceType != "transactions" || material.PIIClassification != domainevidence.PIINone ||
		material.PaginationCompleteness != domainevidence.PaginationComplete || material.Granularity != accountFlowReceiptGranularity ||
		len(material.TransformationLineage) != 2 || material.TransformationLineage[0].StepID != accountFlowNativeBindStepIDV1 ||
		material.TransformationLineage[0].InputHash != raw.RawSHA256 || material.TransformationLineage[0].OutputHash != semantic.ResultHash ||
		material.TransformationLineage[1].StepID != accountFlowNormalizeStepIDV1 || material.TransformationLineage[1].InputHash != semantic.ResultHash ||
		material.TransformationLineage[1].OutputHash != domainsecurity.CanonicalJSONHash(material.CanonicalEvidence) {
		t.Fatalf("account-flow canonical material is incomplete: parsed=%#v material=%#v err=%v", parsed, material, err)
	}
	if len(parsed.AcceptedSlotSourceBindings) != 1 ||
		parsed.AcceptedSlotSourceBindings[0].EntityReference != subject ||
		parsed.AcceptedSlotSourceBindings[0].SourceRecordID != semantic.Transactions[0].EvidenceRef ||
		parsed.AcceptedSlotSourceBindings[0].Field != domainevidence.AcceptedSlotSourceFieldAccountV1 ||
		len(parsed.AcceptedSlotSourceBindings[0].FactIDs) != len(parsed.Facts) {
		t.Fatalf("account-flow source lineage did not bind its exact entity/fact/record set: %#v", parsed.AcceptedSlotSourceBindings)
	}
	boundFacts := make(map[string]bool, len(parsed.AcceptedSlotSourceBindings[0].FactIDs))
	for _, factID := range parsed.AcceptedSlotSourceBindings[0].FactIDs {
		boundFacts[factID] = true
	}
	for _, fact := range parsed.Facts {
		if !boundFacts[fact.FactID] || fact.NormalizedPayload.SubjectID != subject ||
			fact.NormalizedPayload.EntityID != subject {
			t.Fatalf("account-flow source lineage omitted or altered fact %q", fact.FactID)
		}
	}
	var aggregateIn, aggregateOut, aggregateCount bool
	for _, fact := range parsed.Facts {
		switch {
		case strings.HasPrefix(fact.FactID, accountFlowAggregateFactPrefix) && fact.ClaimType == domainevidence.ClaimAmount && fact.NormalizedPayload.Direction == "in":
			aggregateIn = fact.NormalizedPayload.AmountMinor == "100" && fact.NormalizedPayload.Granularity == accountFlowReceiptGranularity
		case strings.HasPrefix(fact.FactID, accountFlowAggregateFactPrefix) && fact.ClaimType == domainevidence.ClaimAmount && fact.NormalizedPayload.Direction == "out":
			aggregateOut = fact.NormalizedPayload.AmountMinor == "0" && fact.NormalizedPayload.Granularity == accountFlowReceiptGranularity
		case strings.HasPrefix(fact.FactID, accountFlowAggregateFactPrefix) && fact.ClaimType == domainevidence.ClaimCount:
			aggregateCount = fact.NormalizedPayload.Count == "1" && fact.NormalizedPayload.Granularity == accountFlowReceiptGranularity
		}
	}
	if !aggregateIn || !aggregateOut || !aggregateCount {
		t.Fatalf("account-flow facts did not retain row support and zero-direction aggregates: %#v", parsed.Facts)
	}
	detached := *capture
	detached.rows = append([]accountFlowCapturedRow(nil), capture.rows...)
	detached.rows[0].amountMinor = "101"
	if _, err := normalizeAccountFlowEvidence(input, reader.probe, raw, semantic, &detached); err == nil {
		t.Fatal("provider-safe account-flow semantics detached from the host-private evidence row")
	}
	detached = *capture
	detached.summary.inflowMinor = "101"
	if _, err := normalizeAccountFlowEvidence(input, reader.probe, raw, semantic, &detached); err == nil {
		t.Fatal("account-flow row and aggregate mismatch was not rejected")
	}
}

func TestAccountFlowNormalizerKeepsAggregateClaimsPartialWhenEvidenceRowsTruncate(t *testing.T) {
	_, reader, input := toolEvidenceServiceFixture(t, "1")
	input.Call.Name = fundsAccountFlowCanonicalTool
	input.Grant.ToolName = input.Call.Name
	input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticPartial,
		PartialCoverage: map[string]any{"coverageStatus": "partial"}, IssuedAt: evidenceIssuerTime(),
	})
	input.Binding, _ = domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: input.Context.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid,
		CaseID: input.Context.CaseID, BindingSHA256: domainsecurity.SHA256Hex([]byte("account-binding-partial")),
		CaseBindingHash: input.Context.CaseBindingHash,
	})
	const subject = "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	row := domainnative.AccountFlowProviderSemanticTransactionV1{
		EvidenceRef:  "srow1_" + domainsecurity.SHA256Hex([]byte("account-flow-partial-row")),
		Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
		OccurredAt:   "2026-01-05T10:30:00.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1,
		AmountMinor: "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
	}
	semantic := domainnative.AccountFlowProviderSemanticResultV1{
		SubjectAlias: "acct:1", StartInclusive: "2026-01-01T00:00:00.000000Z", EndInclusive: "2026-01-31T23:59:59.000000Z",
		Timezone: "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: "300", OutflowMinor: "0", NetMinor: "300", TransactionCount: 2,
		EvidenceTransactionCount: 1, EvidenceRowLimit: 1, AggregateComplete: true, EvidenceRowsComplete: false,
		CounterpartySemanticsComplete: false,
		Currentness:                   domainnative.AccountFlowProviderCurrentnessCurrentV1,
		Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{State: domainnative.AccountFlowCoveragePartialV1,
			Gaps: []string{domainnative.AccountFlowGapEvidenceRowLimitV1, domainnative.AccountFlowGapCounterpartyResolutionV1}, NormalizedSnapshotRows: 2,
			AcceptedSnapshotRows: 2, ObservedMatchingRows: 2},
		QueryHash: domainsecurity.SHA256Hex([]byte("account-flow-partial-query")), ResultHash: domainsecurity.SHA256Hex([]byte("account-flow-partial-result")),
		Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{row},
	}
	setAccountFlowProviderOutcomeForEvidenceTestV1(t, &semantic)
	if semantic.Outcome.AggregateCompleteness != domainevidence.AccountFlowOutcomeCompletenessCompleteV1 ||
		semantic.Outcome.EvidenceRowsCompleteness != domainevidence.AccountFlowOutcomeCompletenessIncompleteV1 ||
		semantic.Outcome.FactAnswerAllowed {
		t.Fatalf("partial pagination upgraded an independent account-flow outcome axis: %#v", semantic.Outcome)
	}
	capture := &accountFlowEvidenceCapture{}
	if err := capture.consumeSummary(subject, input.Context.DatasetSnapshotID, input.Context.ContextEpoch, input.Context.ContextDigest,
		input.Context.CaseBindingHash, semantic.StartInclusive, semantic.EndInclusive, semantic.Timezone, semantic.Currency,
		semantic.MinorUnitScale, semantic.InflowMinor, semantic.OutflowMinor, semantic.NetMinor, semantic.TransactionCount,
		semantic.AggregateComplete, semantic.EvidenceRowsComplete, semantic.EvidenceTransactionCount, semantic.Coverage,
		semantic.QueryHash, semantic.ResultHash); err != nil {
		t.Fatal(err)
	}
	if err := capture.consumeRow(0, subject, row.EvidenceRef, accountFlowEvidenceSourceFileIDV1, 7, row.OccurredAt, row.Direction,
		row.AmountMinor, row.Currency, row.MinorUnitScale); err != nil {
		t.Fatal(err)
	}
	raw := losslessEvidenceCandidate(t, map[string]any{"structuredContent": map[string]any{"safe": true}})
	material, err := normalizeAccountFlowEvidence(input, reader.probe, raw, semantic, capture)
	if err != nil || material.PaginationCompleteness != domainevidence.PaginationPartial {
		t.Fatalf("truncated account-flow evidence did not remain partial: material=%#v err=%v", material, err)
	}
	parsed, err := domainevidence.ParseCanonicalEvidenceMaterial(material.CanonicalEvidence)
	if err != nil || len(parsed.Facts) != 3 {
		t.Fatalf("partial account-flow aggregate facts missing: facts=%#v err=%v", parsed.Facts, err)
	}
}

func TestAccountFlowHostAuthoritySettlementExactlyOnce(t *testing.T) {
	for _, test := range []struct {
		alias, field string
	}{
		{alias: "acct:1", field: domainevidence.AcceptedSlotSourceFieldAccountV1},
		{alias: "card:1", field: domainevidence.AcceptedSlotSourceFieldCardV1},
	} {
		t.Run(test.field, func(t *testing.T) {
			testAccountFlowHostAuthoritySettlementExactlyOnceV1(t, test.alias, test.field, false)
		})
	}
}

func TestAccountFlowHostAuthoritySettlementSurvivesFreshWitnessExchange(t *testing.T) {
	testAccountFlowHostAuthoritySettlementExactlyOnceV1(t, "acct:1", domainevidence.AcceptedSlotSourceFieldAccountV1, true)
}

func testAccountFlowHostAuthoritySettlementExactlyOnceV1(t *testing.T, subjectAlias, expectedField string, renewWitness bool) {
	service, reader, input := toolEvidenceServiceFixture(t, "1")
	input.Call.Name = fundsAccountFlowCanonicalTool
	input.Call.Arguments = json.RawMessage(`{"subject_alias":"` + subjectAlias + `","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.000000Z","evidence_row_limit":1}`)
	input.Grant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: input.Context, Provider: "provider-a", ServerIdentity: input.Grant.ServerIdentity, ToolName: input.Call.Name,
		ToolCallID: input.Call.ID, ConnectionEpoch: reader.probe.ConnectionEpoch, ArgsHash: domainsecurity.CanonicalJSONHash(input.Call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema-account-flow")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope-account-flow")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: evidenceIssuerTime(),
	})
	var err error
	input.GrantRegistry, err = domainsecurity.RegisterExecutionGrant(domainsecurity.NewExecutionGrantRegistry(input.Context.ThreadID), input.Context.ThreadID, input.Grant, evidenceIssuerTime())
	if err != nil {
		t.Fatal(err)
	}
	input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticPartial,
		PartialCoverage: map[string]any{"coverageStatus": "partial"}, IssuedAt: evidenceIssuerTime().Add(time.Second),
	})
	input.Binding, err = domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: input.Context.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid, CaseID: input.Context.CaseID,
		BindingSHA256: domainsecurity.SHA256Hex([]byte("account-flow-binding")), CaseBindingHash: input.Context.CaseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	const subject = "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	semantic := domainnative.AccountFlowProviderSemanticResultV1{
		SubjectAlias: subjectAlias, StartInclusive: "2026-01-01T00:00:00.000000Z", EndInclusive: "2026-01-31T23:59:59.000000Z",
		Timezone: "Z", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		InflowMinor: "100", OutflowMinor: "0", NetMinor: "100", TransactionCount: 1, EvidenceTransactionCount: 1,
		EvidenceRowLimit: 1, AggregateComplete: true, EvidenceRowsComplete: true, CounterpartySemanticsComplete: false,
		Currentness: domainnative.AccountFlowProviderCurrentnessCurrentV1,
		Coverage: domainnative.AccountFlowProviderSemanticCoverageV1{State: domainnative.AccountFlowCoveragePartialV1,
			Gaps: []string{domainnative.AccountFlowGapCounterpartyResolutionV1}, NormalizedSnapshotRows: 1, AcceptedSnapshotRows: 1, ObservedMatchingRows: 1},
		QueryHash: domainsecurity.SHA256Hex([]byte("account-flow-query")), ResultHash: domainsecurity.SHA256Hex([]byte("account-flow-result")),
		Transactions: []domainnative.AccountFlowProviderSemanticTransactionV1{{
			EvidenceRef: "srow1_" + domainsecurity.SHA256Hex([]byte("account-flow-row")), Counterparty: domainnative.AccountFlowProviderCounterpartyV1{Status: domainnative.AccountFlowCounterpartyUnresolvedV1},
			OccurredAt: "2026-01-05T10:30:00.000000Z", Direction: domainnative.AccountFlowDirectionInflowV1, AmountMinor: "100", Currency: "CNY", MinorUnitScale: domainnative.AccountFlowMinorUnitScaleV1,
		}},
	}
	setAccountFlowProviderOutcomeForEvidenceTestV1(t, &semantic)
	selectionBinding := domainsecurity.DatasetSnapshotBindingKeyV1{TenantID: input.Context.TenantID, UserID: input.Context.UserID,
		WorkspaceRealPath: input.Context.WorkspaceRealPath, CaseID: input.Context.CaseID, CaseBindingHash: input.Context.CaseBindingHash,
		BindingObservationDigest: input.Context.PublicationPolicy.BindingObservationDigest}
	selection, err := datasetsnapshotv2fixture.CurrentSelectionForContextV2(input.Context, selectionBinding)
	if err != nil {
		t.Fatal(err)
	}
	raw := losslessEvidenceCandidate(t, map[string]any{"structuredContent": map[string]any{"safe": true}, "path": "/private/case.duckdb"})
	source := &accountFlowHostSourceStub{probe: reader.probe, capability: &accountFlowCapabilityStub{selection: selection}, semantic: semantic, subjectRef: subject, snapshotID: input.Context.DatasetSnapshotID}
	service.Reader = source
	service.Issuer.SettlementStore = &hostMemoryEvidenceSettlementStore{}
	input.RawResult = raw
	prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input)
	if err != nil || !eligible || prepared.Authority.Marker.ReceiptID == "" {
		t.Fatalf("account-flow host authority did not prepare settlement: prepared=%#v eligible=%v err=%v", prepared, eligible, err)
	}
	if source.consumeCall != 1 || !source.consumed {
		t.Fatalf("account-flow carrier was not consumed exactly once: calls=%d consumed=%v", source.consumeCall, source.consumed)
	}
	publicOutput, marshalErr := json.Marshal(prepared.Output)
	for _, forbidden := range []string{
		accountFlowEvidenceSourceFileIDV1,
		`"sourceRowNumber"`,
		semantic.Transactions[0].EvidenceRef,
		subject,
		"/private/case.duckdb",
		"authorityEntityRef",
	} {
		if marshalErr != nil || strings.Contains(string(publicOutput), forbidden) {
			t.Fatalf("provider-visible tool output leaked private accepted-slot lineage %q: %s", forbidden, publicOutput)
		}
	}
	if prepared.Authority.Record.HostAuthority == nil || domainevidence.ValidatePreparedEvidenceSettlementForExecution(prepared.Authority.Record) == nil {
		t.Fatalf("host settlement unexpectedly acquired bare execution authority: %#v", prepared.Authority.Record.HostAuthority)
	}
	if err := source.ConsumeHostFundsAccountFlowEvidenceV1(raw, nil, nil); err == nil {
		t.Fatal("account-flow carrier remained reusable after settlement")
	}
	thread := toolEvidenceDurableThread(input, prepared.Authority.Marker, evidenceIssuerTime().Add(3*time.Second))
	threadBody, marshalErr := json.Marshal(thread)
	for _, forbidden := range []string{
		accountFlowEvidenceSourceFileIDV1,
		`"sourceRowNumber"`,
		semantic.Transactions[0].EvidenceRef,
		subject,
		"/private/case.duckdb",
		"authorityEntityRef",
	} {
		if marshalErr != nil || strings.Contains(string(threadBody), forbidden) {
			t.Fatalf("ordinary durable history leaked private accepted-slot lineage %q: %s", forbidden, threadBody)
		}
	}
	turn := thread["turns"].([]any)[0].(map[string]any)
	callItem := turn["items"].([]any)[0].(map[string]any)
	var accountArguments map[string]any
	if err := json.Unmarshal(input.Call.Arguments, &accountArguments); err != nil {
		t.Fatal(err)
	}
	callItem["arguments"] = accountArguments
	if renewWitness {
		// A fresh capability can select the identical authority bundle and DSV2
		// graph after a new witness exchange. This stub models only that exchange;
		// the production public-chain test supplies the real signed witness.
		selection := source.capability.selection
		selection.Head.Request.ChallengeNonce = strings.Repeat("f", 64)
		selection.Head.Observation.ChallengeNonce = strings.Repeat("f", 64)
		selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
		if err != nil || selection.SelectionDigest == source.capability.selection.SelectionDigest {
			t.Fatal("fresh witness exchange did not change its exact audit digest")
		}
		source.capability.selection = selection
		testAccountFlowFreshContentGuardsV2(t, service, source, input, prepared, thread)
	}
	receipt, err := service.CommitCurrentToolEvidence(context.Background(), CommitToolEvidenceInput{
		Context: input.Context, Marker: prepared.Authority.Marker, Thread: thread,
	})
	if err != nil || receipt.ReceiptID == "" {
		t.Fatalf("account-flow host settlement did not commit receipt: receipt=%#v err=%v", receipt, err)
	}
	receiptBody, _ := json.Marshal(receipt)
	if strings.Contains(string(receiptBody), "/private/case.duckdb") {
		t.Fatal("account-flow receipt leaked private source path")
	}
	canonicalText := string(prepared.Authority.Record.CanonicalEvidence)
	if strings.Contains(canonicalText, "/private/case.duckdb") ||
		!strings.Contains(canonicalText, accountFlowEvidenceSourceFileIDV1) ||
		!strings.Contains(canonicalText, `"sourceRowNumber":1`) {
		t.Fatalf("host settlement canonical evidence did not retain only its private opaque source lineage: %s", canonicalText)
	}
	canonicalMaterial, err := domainevidence.ParseCanonicalEvidenceMaterial(prepared.Authority.Record.CanonicalEvidence)
	if err != nil || len(canonicalMaterial.Facts) != 3 || len(canonicalMaterial.AcceptedSlotSourceBindings) != 1 ||
		canonicalMaterial.AcceptedSlotSourceBindings[0].Field != expectedField ||
		len(canonicalMaterial.AcceptedSlotSourceBindings[0].FactIDs) != 3 {
		t.Fatalf("%s settlement lost its exact three-fact source field binding: material=%#v err=%v",
			subjectAlias, canonicalMaterial, err)
	}
	claims := make([]domainevidence.ClaimRecord, len(canonicalMaterial.Facts))
	for index, fact := range canonicalMaterial.Facts {
		proposal := domainevidence.ClaimProposal{
			SchemaVersion:     domainevidence.ClaimProposalVersion,
			ProposalID:        "proposal-account-flow-" + strconv.Itoa(index+1),
			ClaimType:         fact.ClaimType,
			NormalizedPayload: fact.NormalizedPayload,
			EvidenceIDs:       []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{},
		}
		claimID := "claim-account-flow-" + strconv.Itoa(index+1)
		claim, verifyErr := (ClaimVerifier{
			Registry:    service.Issuer.Registry,
			IDGenerator: func() (string, error) { return claimID, nil },
			Now:         evidenceIssuerTime,
		}).Verify(context.Background(), input.Context, proposal)
		if verifyErr != nil || claim.SupportState != domainevidence.ClaimVerified || len(claim.EvidenceIDs) != 1 ||
			claim.EvidenceIDs[0] != receipt.ReceiptID || claim.VerifierReceiptID == "" {
			t.Fatalf("account-flow receipt did not produce verified claim %d: claim=%#v err=%v", index, claim, verifyErr)
		}
		claims[index] = claim
	}
	incompleteEnvelope, err := (FinalEvidenceGate{Registry: service.Issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess,
		Claims: claims[:1], IssuedAt: evidenceIssuerTime().Add(4 * time.Second),
	})
	if err != nil || incompleteEnvelope.AccountFlowOutcome != nil {
		t.Fatalf("incomplete account-flow group acquired typed eligibility: envelope=%#v err=%v", incompleteEnvelope, err)
	}
	if slots, slotsErr := domainevidence.BuildAcceptedEntitySlotBindingsV1(incompleteEnvelope); slotsErr != nil || len(slots) != 0 {
		t.Fatalf("incomplete account-flow group acquired a display slot: slots=%#v err=%v", slots, slotsErr)
	}
	envelope, err := (FinalEvidenceGate{Registry: service.Issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess,
		Claims: claims, IssuedAt: evidenceIssuerTime().Add(5 * time.Second),
	})
	if err != nil || envelope.Variant != domainevidence.EvidenceBackedAnswer || len(envelope.Claims) != 3 ||
		len(envelope.EvidenceReceiptIDs) != 1 || envelope.EvidenceReceiptIDs[0] != receipt.ReceiptID ||
		envelope.AccountFlowOutcome == nil || !envelope.AccountFlowOutcome.FactAnswerAllowed ||
		envelope.AccountFlowOutcome.TypedSlotEligibility != domainevidence.AccountFlowOutcomeTypedSlotEligibleV1 ||
		envelope.AccountFlowOutcome.LocalDisplayCompletion != domainevidence.AccountFlowOutcomeDisplayNotRequestedV1 ||
		len(envelope.AccountFlowOutcome.Groups) != 1 ||
		envelope.AccountFlowOutcome.Groups[0].SourceFieldReference != semantic.Outcome.SourceFieldReference {
		t.Fatalf("complete account-flow group did not acquire exact typed eligibility: envelope=%#v err=%v", envelope, err)
	}
	if slots, slotsErr := domainevidence.BuildAcceptedEntitySlotBindingsV1(envelope); slotsErr != nil || len(slots) != 1 {
		t.Fatalf("complete account-flow group did not derive one typed slot: slots=%#v err=%v", slots, slotsErr)
	}
	for name, mutate := range map[string]func(*domainevidence.AccountFlowTypedAnswerOutcomeV1){
		"fact authority downgrade": func(outcome *domainevidence.AccountFlowTypedAnswerOutcomeV1) {
			outcome.FactAnswerAllowed = false
		},
		"display completion upgrade": func(outcome *domainevidence.AccountFlowTypedAnswerOutcomeV1) {
			outcome.LocalDisplayCompletion = "completed"
		},
		"cross-query source binding": func(outcome *domainevidence.AccountFlowTypedAnswerOutcomeV1) {
			outcome.Groups[0].SourceFieldReference.BindingRef = "afslot1_" + strings.Repeat("f", 64)
		},
	} {
		t.Run("final outcome rejects "+name, func(t *testing.T) {
			tampered := *envelope.AccountFlowOutcome
			tampered.Groups = append([]domainevidence.AccountFlowTypedAnswerGroupV1(nil), envelope.AccountFlowOutcome.Groups...)
			mutate(&tampered)
			if _, constructErr := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.EvidenceBackedAnswer, Context: input.Context, TerminalReason: string(TerminalSuccess),
				AccountFlowOutcome: &tampered, Claims: envelope.Claims, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs,
				CheckedScope: envelope.CheckedScope, IssuedAt: evidenceIssuerTime().Add(6 * time.Second),
			}); constructErr == nil {
				t.Fatal("tampered Final Gate typed outcome was accepted")
			}
		})
	}
	envelopeBody, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var hostileEnvelope map[string]any
	if err := json.Unmarshal(envelopeBody, &hostileEnvelope); err != nil {
		t.Fatal(err)
	}
	hostileEnvelope["accountFlowOutcome"].(map[string]any)["sourceExactValue"] = "6222021234567890123"
	hostileBody, err := json.Marshal(hostileEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, parseErr := domainevidence.ParseFinalAnswerEnvelope(hostileBody); parseErr == nil {
		t.Fatal("Final Gate typed outcome accepted an unknown source-exact value field")
	}
	wrongScope := *envelope.CheckedScope
	wrongScope.FiltersHash = domainsecurity.SHA256Hex([]byte("wrong-account-flow-scope"))
	wrongScopeEnvelope, wrongScopeErr := (FinalEvidenceGate{Registry: service.Issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess,
		Claims: claims, CheckedScope: &wrongScope, RequestedScope: &wrongScope,
		IssuedAt: evidenceIssuerTime().Add(6 * time.Second),
	})
	if wrongScopeErr != nil || wrongScopeEnvelope.Variant != domainevidence.PartialEvidenceAnswer ||
		wrongScopeEnvelope.AccountFlowOutcome != nil {
		t.Fatalf("wrong query scope upgraded account-flow typed eligibility: envelope=%#v err=%v", wrongScopeEnvelope, wrongScopeErr)
	}
	staleContext := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, WorkspaceRealPath: input.Context.WorkspaceRealPath,
		CaseID: input.Context.CaseID, CaseBindingHash: input.Context.CaseBindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("account-flow-stale"),
		SourceManifestHash: input.Context.SourceManifestHash, ContextEpoch: input.Context.ContextEpoch + 1,
		IssuedAt: evidenceIssuerTime().Add(7 * time.Second),
	})
	staleEnvelope, staleErr := (FinalEvidenceGate{Registry: service.Issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: staleContext, TerminalReason: TerminalSuccess,
		Claims: claims, IssuedAt: evidenceIssuerTime().Add(8 * time.Second),
	})
	if staleErr != nil || staleEnvelope.Variant != domainevidence.NeedsEvidenceAnswer ||
		staleEnvelope.AccountFlowOutcome != nil || len(staleEnvelope.Claims) != 0 {
		t.Fatalf("stale snapshot/epoch upgraded account-flow typed eligibility: envelope=%#v err=%v", staleEnvelope, staleErr)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil || !strings.Contains(rendered, "〔账户槽位 1〕") || strings.Contains(rendered, subject) {
		t.Fatalf("eligible account-flow group did not render one value-free entity slot: rendered=%q err=%v", rendered, err)
	}
	for _, forbidden := range []string{
		accountFlowEvidenceSourceFileIDV1,
		`"sourceRowNumber"`,
		`"acceptedSlotSourceBindings"`,
		semantic.Transactions[0].EvidenceRef,
		"/private/case.duckdb",
		"authorityEntityRef",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered final leaked private V3 lineage %q: %s", forbidden, rendered)
		}
	}
}

type lockedEvidenceReaderStub struct {
	context domainsecurity.TurnSecurityContext
	probe   domainsecurity.VerifiedSourceProbe
	raw     domainmcp.LosslessToolResult
	err     error
	calls   int
}

func testAccountFlowFreshContentGuardsV2(t *testing.T, service ToolEvidenceService, source *accountFlowHostSourceStub, input PrepareToolEvidenceInput, prepared PreparedToolEvidence, thread map[string]any) {
	t.Helper()
	baseSelection, baseProbe := source.capability.selection, source.probe
	for name, mutate := range map[string]func(*datasetsnapshotport.CurrentSelectionV2){
		"bundle generation": func(s *datasetsnapshotport.CurrentSelectionV2) { s.Head.Bundle.Generation++ },
		"registry child": func(s *datasetsnapshotport.CurrentSelectionV2) {
			s.Head.Bundle.EvidenceRegistryIndexDigest = strings.Repeat("d", 64)
		},
		"publication child": func(s *datasetsnapshotport.CurrentSelectionV2) { s.Head.Bundle.PublicationCount++ },
		"bundle absent":     func(s *datasetsnapshotport.CurrentSelectionV2) { s.Head.HasBundle = !s.Head.HasBundle },
		"index path": func(s *datasetsnapshotport.CurrentSelectionV2) {
			s.DatasetIndexPath = append([]domainsecurity.DatasetSnapshotIndexV1(nil), s.DatasetIndexPath...)
			s.DatasetIndexPath = append(s.DatasetIndexPath, domainsecurity.DatasetSnapshotIndexV1{IndexDigest: strings.Repeat("d", 64)})
		},
		"selected index": func(s *datasetsnapshotport.CurrentSelectionV2) { s.SelectedIndex.IndexDigest = strings.Repeat("d", 64) },
		"snapshot":       func(s *datasetsnapshotport.CurrentSelectionV2) { s.Snapshot.Record.DatasetSnapshotID += "drift" },
	} {
		t.Run("fresh content rejects "+name, func(t *testing.T) {
			selection := baseSelection
			mutate(&selection)
			selection.SelectionDigest, _ = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
			source.capability.selection = selection
			defer func() { source.capability.selection = baseSelection }()
			receipt, err := service.CommitCurrentToolEvidence(context.Background(), CommitToolEvidenceInput{Context: input.Context, Marker: prepared.Authority.Marker, Thread: thread})
			if err == nil || receipt.ReceiptID != "" {
				t.Fatal("changed complete content acquired a receipt")
			}
		})
	}
	for name, mutate := range map[string]func(*domainsecurity.VerifiedSourceProbe){
		"probe": func(p *domainsecurity.VerifiedSourceProbe) {
			p.CheckedAt = evidenceIssuerTime().Add(time.Minute).Format(time.RFC3339Nano)
		},
		"connection epoch": func(p *domainsecurity.VerifiedSourceProbe) { p.ConnectionEpoch++ },
		"case":             func(p *domainsecurity.VerifiedSourceProbe) { p.CaseID = "other-case" },
	} {
		t.Run("fresh content rejects "+name, func(t *testing.T) {
			mutate(&source.probe)
			defer func() { source.probe = baseProbe }()
			receipt, err := service.CommitCurrentToolEvidence(context.Background(), CommitToolEvidenceInput{Context: input.Context, Marker: prepared.Authority.Marker, Thread: thread})
			if err == nil || receipt.ReceiptID != "" {
				t.Fatal("changed probe acquired a receipt")
			}
		})
	}
	for _, capability := range []sourceprobeport.HostEvidenceCapability{nil, &accountFlowCapabilityStub{selection: baseSelection}, &accountFlowCapabilityStub{active: true}} {
		if _, err := validatePreparedHostAuthoritySelection(prepared.Authority.Record, input.Context, source.probe, capability); err == nil {
			t.Fatal("missing, expired or syntax-only capability acquired current authority")
		}
	}
	changedContext := input.Context
	changedContext.ContextEpoch++
	source.capability.active = true
	defer func() { source.capability.active = false }()
	if _, err := validatePreparedHostAuthoritySelection(prepared.Authority.Record, changedContext, source.probe, source.capability); err == nil {
		t.Fatal("changed full context retained current authority")
	}
	if validateNewHostAuthorityContent("", source.capability) == nil || validateNewHostAuthorityContent("bad", source.capability) == nil {
		t.Fatal("new preparation admitted a missing or malformed content binding")
	}
	// Reconstruct an old signed synthetic record without changing a stored
	// record. Historical inventory/read compatibility never grants freshness.
	record := prepared.Authority.Record
	host := *record.HostAuthority
	host.SelectionContentDigest = ""
	when, _ := time.Parse(time.RFC3339Nano, record.PreparedAt)
	r := record.ReceiptDraft
	draftInput := domainevidence.EvidenceReceiptInput{
		Context: record.SecurityContext, ExecutionGrantID: r.ExecutionGrantID, ToolCallID: r.ToolCallID,
		ServerIdentity: r.ServerIdentity, ServerVersion: r.ServerVersion, ConnectionEpoch: r.ConnectionEpoch,
		ToolName: r.ToolName, ArgsHash: r.ArgsHash, ResultHash: r.ResultHash, SourceType: r.SourceType,
		DatasetSnapshotID: r.DatasetSnapshotID, QueryHash: r.QueryHash, QueryRange: r.QueryRange,
		Granularity: r.Granularity, Currency: r.Currency, Timezone: r.Timezone, PaginationCompleteness: r.PaginationCompleteness,
		SourceRecordIDs: r.SourceRecordIDs, RawSHA256: r.RawSHA256, TransformationLineage: r.TransformationLineage,
		PIIClassification: r.PIIClassification, IssuedAt: when,
	}
	oldRaw, err := base64.RawStdEncoding.DecodeString(record.RawResultBase64)
	if err != nil {
		t.Fatal(err)
	}
	oldInput := domainevidence.PreparedEvidenceSettlementInput{
		Context: record.SecurityContext, Grant: record.ExecutionGrant, ActiveGrantRegistrySequence: record.ActiveGrantRegistrySequence,
		ActiveGrantRegistryDigest: record.ActiveGrantRegistryDigest, SourceProbe: record.SourceProbe, ToolOutcome: record.ToolOutcome,
		RawResult: oldRaw, CanonicalEvidence: record.CanonicalEvidence, ReceiptDraft: r,
		QueryHash: record.QueryHash, ResultItemID: record.ResultItemID, HostAuthority: &host, PreparedAt: when,
		AuthorityKeyID: service.Issuer.Authority.KeyID(), AuthorityPublicKey: service.Issuer.Authority.PublicKey(),
	}
	draftInput.ReceiptID = domainevidence.EvidenceSettlementReceiptID(domainevidence.ComputeEvidenceSettlementID(oldInput))
	oldInput.ReceiptDraft, err = domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := domainevidence.NewPreparedEvidenceSettlement(oldInput, func(message []byte) ([]byte, error) {
		return service.Issuer.Authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	content := record.HostAuthority.SelectionContentDigest
	if err := domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(legacy, input.Context, source.probe, host.SelectionDigest, content); err != nil {
		t.Fatalf("historical exact selection lost its existing validation: %v", err)
	}
	if err := domainevidence.ValidatePreparedEvidenceSettlementForCurrentHostAuthorityV2(legacy, input.Context, source.probe, baseSelection.SelectionDigest, content); err == nil {
		t.Fatal("old record gained cross-observation authority")
	}
	if legacy.HostAuthority.SelectionContentDigest != "" {
		t.Fatal("historical read synthesized content authority")
	}
	legacyBytes, err := domainevidence.PreparedEvidenceSettlementBytes(legacy)
	if err != nil {
		t.Fatal(err)
	}
	oldIssuer := service.Issuer
	oldIssuer.Registry = &memoryEvidenceRegistry{}
	oldIssuer.SettlementStore = &hostMemoryEvidenceSettlementStore{memoryEvidenceSettlementStore: memoryEvidenceSettlementStore{
		records: map[string]domainevidence.PreparedEvidenceSettlement{legacy.SettlementID: legacy},
	}}
	legacyMarker, err := domainevidence.NewHostEvidenceSettlementMarker(legacy)
	if err != nil {
		t.Fatal(err)
	}
	oldThread := toolEvidenceDurableThread(input, legacyMarker, evidenceIssuerTime().Add(3*time.Second))
	oldTurn := oldThread["turns"].([]any)[0].(map[string]any)
	oldCall := oldTurn["items"].([]any)[0].(map[string]any)
	var args map[string]any
	if err := json.Unmarshal(input.Call.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	oldCall["arguments"] = args
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{input.Context.ThreadID: oldThread}}
	inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, oldIssuer)
	if err != nil || len(inventory.Pending) != 0 || len(inventory.Quarantined) != 1 {
		t.Fatalf("old host pending record lost audit-only restart classification: %v", err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, oldIssuer, inventory); err != nil {
		t.Fatal(err)
	}
	after, err := oldIssuer.SettlementStore.ResolvePrepared(context.Background(), legacy.SettlementID)
	afterBytes, _ := domainevidence.PreparedEvidenceSettlementBytes(after)
	if err != nil || !bytes.Equal(legacyBytes, afterBytes) || oldIssuer.Registry.(*memoryEvidenceRegistry).commitCalls != 0 {
		t.Fatal("historical reconciliation changed signed bytes or issued a receipt")
	}
}

type accountFlowCapabilityStub struct {
	selection datasetsnapshotport.CurrentSelectionV2
	active    bool
}

func (stub *accountFlowCapabilityStub) DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error) {
	if stub == nil || !stub.active {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("host capability is inactive")
	}
	return stub.selection, nil
}

func (stub *accountFlowCapabilityStub) UseExact(
	_ domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	callback func(context.Context) error,
) error {
	if stub == nil || !stub.active || callback == nil || selection.SelectionDigest != stub.selection.SelectionDigest {
		return errors.New("host capability exact selection mismatch")
	}
	return callback(context.Background())
}

type accountFlowHostSourceStub struct {
	probe       domainsecurity.VerifiedSourceProbe
	capability  *accountFlowCapabilityStub
	semantic    domainnative.AccountFlowProviderSemanticResultV1
	subjectRef  string
	snapshotID  string
	consumed    bool
	discarded   int
	consumeCall int
}

// This existing in-memory issuer fixture checks signed binding propagation.
// Actual CAS provenance is tested with the composed runtime owners.
func (stub *accountFlowCapabilityStub) UseExactRegistryCommit(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	_ domainevidence.PreparedEvidenceSettlement,
	_ domainevidence.HostEvidenceSettlementMarker,
	callback func(context.Context) error,
) error {
	return stub.UseExact(securityContext, probe, selection, callback)
}

func setAccountFlowProviderOutcomeForEvidenceTestV1(
	t *testing.T,
	semantic *domainnative.AccountFlowProviderSemanticResultV1,
) {
	t.Helper()
	if semantic == nil {
		t.Fatal("account-flow semantic fixture is nil")
	}
	outcome, err := domainnative.NewAccountFlowProviderOutcomeV1(
		semantic.SubjectAlias,
		semantic.AggregateComplete,
		semantic.EvidenceRowsComplete,
		semantic.QueryHash,
		semantic.ResultHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	semantic.Outcome = outcome
}

func (stub *accountFlowHostSourceStub) WithCurrentEvidenceRead(
	context.Context, sourceprobeport.EvidenceReadInput, func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error,
) error {
	return errors.New("legacy evidence read path unavailable")
}

func (stub *accountFlowHostSourceStub) WithCurrentProbeAuthority(
	_ context.Context, _ sourceprobeport.CurrentInput,
	callback func(domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if callback == nil || stub.capability == nil {
		return errors.New("host authority callback unavailable")
	}
	stub.capability.active = true
	defer func() { stub.capability.active = false }()
	return callback(stub.probe, stub.capability)
}

func (stub *accountFlowHostSourceStub) HostFundsAccountFlowProviderSemanticV1(
	result domainmcp.LosslessToolResult,
) (domainnative.AccountFlowProviderSemanticResultV1, bool) {
	return stub.semantic, domainmcp.ValidLosslessToolResult(result) && !stub.consumed
}

func (stub *accountFlowHostSourceStub) ConsumeHostFundsAccountFlowEvidenceV1(
	result domainmcp.LosslessToolResult,
	consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
	consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
) error {
	if stub.consumed || !domainmcp.ValidLosslessToolResult(result) || consumeSummary == nil || consumeRow == nil {
		stub.discarded++
		return errors.New("account-flow carrier is not active")
	}
	stub.consumeCall++
	stub.consumed = true
	if err := consumeSummary(stub.subjectRef, stub.snapshotID, stub.probe.ContextEpoch, stub.probe.ProbeContextDigest,
		stub.probe.CaseBindingHash, stub.semantic.StartInclusive, stub.semantic.EndInclusive, stub.semantic.Timezone, stub.semantic.Currency,
		stub.semantic.MinorUnitScale, stub.semantic.InflowMinor, stub.semantic.OutflowMinor, stub.semantic.NetMinor, stub.semantic.TransactionCount,
		stub.semantic.AggregateComplete, stub.semantic.EvidenceRowsComplete, stub.semantic.EvidenceTransactionCount, stub.semantic.Coverage,
		stub.semantic.QueryHash, stub.semantic.ResultHash); err != nil {
		return err
	}
	for index, row := range stub.semantic.Transactions {
		if err := consumeRow(index, stub.subjectRef, row.EvidenceRef, accountFlowEvidenceSourceFileIDV1, uint64(index+1), row.OccurredAt,
			row.Direction, row.AmountMinor, row.Currency, row.MinorUnitScale); err != nil {
			return err
		}
	}
	return nil
}

func (stub *accountFlowHostSourceStub) DiscardHostFundsAccountFlowEvidenceV1(domainmcp.LosslessToolResult) {
	stub.discarded++
}

type hostMemoryEvidenceSettlementStore struct {
	memoryEvidenceSettlementStore
	hostCalls int
}

func (store *hostMemoryEvidenceSettlementStore) PutPreparedIfAbsentWithHostAuthority(
	ctx context.Context, record domainevidence.PreparedEvidenceSettlement, input settlementport.HostAuthorityInput,
) error {
	store.hostCalls++
	if input.Capability == nil || record.HostAuthority == nil ||
		domainevidence.ValidatePreparedEvidenceSettlementForHostAuthorityV2(record, input.Context, input.CurrentProbe, input.SelectionDigest) != nil {
		return errors.New("host settlement authority invalid")
	}
	selection, err := input.Capability.DatasetSelection()
	if err != nil || selection.SelectionDigest != input.SelectionDigest {
		return errors.New("host settlement selection invalid")
	}
	if store.records == nil {
		store.records = map[string]domainevidence.PreparedEvidenceSettlement{}
	}
	if existing, found := store.records[record.SettlementID]; found && existing.RecordDigest != record.RecordDigest {
		return errors.New("host settlement conflicts")
	}
	store.records[record.SettlementID] = record
	return nil
}

type publicationSnapshotLeaseStub struct {
	calls   int
	active  bool
	err     error
	corrupt bool
}

func (stub *publicationSnapshotLeaseStub) WithFreshPublicationSnapshot(_ context.Context, input sourceprobeport.PublicationInput, callback func([]domainsecurity.VerifiedSourceProbe) error) error {
	stub.calls++
	if stub.err != nil {
		return stub.err
	}
	probes := []domainsecurity.VerifiedSourceProbe{}
	seen := map[string]bool{}
	for _, requirement := range input.Requirements {
		if seen[requirement.ServerID] {
			continue
		}
		seen[requirement.ServerID] = true
		snapshotID := input.Context.DatasetSnapshotID
		if stub.corrupt {
			snapshotID = securitycontexttest.DatasetSnapshotID("publication-mismatch")
		}
		probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
			ServerID:        requirement.ServerID,
			ServerIdentity:  requirement.ServerIdentity,
			ConnectionEpoch: requirement.ConnectionEpoch, CatalogFingerprint: domainsecurity.SHA256Hex([]byte("publication-catalog")),
			SpecFingerprint: domainsecurity.SHA256Hex([]byte("publication-spec")), ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID,
			ContextEpoch: input.Context.ContextEpoch, ContextDigest: input.Context.ContextDigest, DatasetSnapshotID: snapshotID,
			CheckedAt: evidenceIssuerTime().Add(4 * time.Second),
			Response: domainsecurity.SourceProbeResponse{
				Version: domainsecurity.SourceProbeVersion, ServerName: requirement.ServerID, ServerVersion: requirement.ServerVersion,
				CaseID: input.Context.CaseID, CaseBindingHash: input.Context.CaseBindingHash, DatasetSnapshotID: snapshotID,
				Ready: true, ReadOnly: true, CheckedAt: evidenceIssuerTime().Add(4 * time.Second).Format(time.RFC3339Nano),
			},
		})
		if err != nil {
			return err
		}
		probes = append(probes, probe)
	}
	stub.active = true
	defer func() { stub.active = false }()
	return callback(probes)
}

func (reader *lockedEvidenceReaderStub) WithCurrentEvidenceRead(_ context.Context, input sourceprobeport.EvidenceReadInput, callback func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error) error {
	reader.calls++
	if reader.err != nil {
		return reader.err
	}
	if input.Context != reader.probeContext() || input.Grant.ContextDigest != input.Context.ContextDigest {
		return errors.New("locked evidence reader received mismatched host authority")
	}
	return callback(reader.probe, reader.raw)
}

func (reader *lockedEvidenceReaderStub) probeContext() domainsecurity.TurnSecurityContext {
	return reader.context
}

func TestPublicationSnapshotRejectsLegacyProbeDespiteVerifiedObservedName(t *testing.T) {
	now := evidenceIssuerTime()
	securityContext := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-publication-name", TurnID: "turn-publication-name", WorkspaceRealPath: "/workspace", CaseID: "case-publication-name",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-publication-name")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("publication-name"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-publication-name")), ContextEpoch: 3, IssuedAt: now,
	})
	identity := evidenceTestVerifiedMCPIdentity(t, "analytix_funds", "funds-evidence-kernel", "0.16.16", 7)
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: identity, ConnectionEpoch: 7,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog-publication-name")), SpecFingerprint: domainsecurity.SHA256Hex([]byte("spec-publication-name")),
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID, CheckedAt: now, Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: "funds-evidence-kernel", ServerVersion: "0.16.16",
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: securityContext.DatasetSnapshotID,
			Ready: true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	requirement := sourceprobeport.PublicationSourceRequirement{
		ReceiptID: "receipt-publication-name", ServerID: "analytix_funds", ServerIdentity: identity,
		ServerVersion: "0.16.16", ConnectionEpoch: 7, ToolName: fundsCountEvidenceCanonicalTool,
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
	}
	if publicationSnapshotProbesMatch(securityContext, []sourceprobeport.PublicationSourceRequirement{requirement}, []domainsecurity.VerifiedSourceProbe{probe}) {
		t.Fatal("legacy snapshot probe supported a publication proof")
	}
	if !publicationSnapshotProbesMatchWithAuthority(
		securityContext,
		[]sourceprobeport.PublicationSourceRequirement{requirement},
		[]domainsecurity.VerifiedSourceProbe{probe},
		domainsecurity.SourceProbeEligibleForHostAuthorityV2,
	) {
		t.Fatal("host-authorized DSV2 probe did not support publication proof assembly")
	}
	requirement.ServerIdentity = evidenceTestVerifiedMCPIdentity(t, "analytix_funds", "funds-evidence-kernel", "0.16.16", 7) + "x"
	if publicationSnapshotProbesMatch(securityContext, []sourceprobeport.PublicationSourceRequirement{requirement}, []domainsecurity.VerifiedSourceProbe{probe}) {
		t.Fatal("publication accepted a non-canonical or mismatched verified identity")
	}
	if publicationSnapshotProbesMatchWithAuthority(
		securityContext,
		[]sourceprobeport.PublicationSourceRequirement{requirement},
		[]domainsecurity.VerifiedSourceProbe{probe},
		domainsecurity.SourceProbeEligibleForHostAuthorityV2,
	) {
		t.Fatal("host-authorized publication accepted a mismatched verified identity")
	}
}

func TestLegacyCountCaseRowsAndEvidenceReadNeverMintReceipt(t *testing.T) {
	service, reader, input := toolEvidenceServiceFixture(t, "2645472")
	prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input)
	if err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable || !eligible {
		t.Fatalf("legacy count evidence did not fail closed: prepared=%#v eligible=%v err=%v", prepared, eligible, err)
	}
	if reader.calls != 0 || prepared.Authority.Marker.SettlementID != "" || len(prepared.Output) != 0 {
		t.Fatalf("legacy count reached evidenceRead or produced settlement state: calls=%d prepared=%#v", reader.calls, prepared)
	}
	registry := service.Issuer.Registry.(*memoryEvidenceRegistry)
	if registry.initialized && registry.registry.Sequence != 0 {
		t.Fatalf("legacy count minted registry evidence: %#v", registry.registry)
	}
}

func TestFundsCountToolEvidenceRunsPrepareDurableCommitAndRegistryReadback(t *testing.T) {
	service, reader, input := toolEvidenceServiceFixture(t, "2645472")
	prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input)
	if err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable || !eligible {
		t.Fatalf("legacy count evidence escaped snapshot quarantine: prepared=%#v eligible=%v err=%v", prepared, eligible, err)
	}
	if reader.calls != 0 || prepared.Authority.Marker.SettlementID != "" || len(prepared.Output) != 0 {
		t.Fatalf("legacy count crossed evidence read or produced private authority: calls=%d prepared=%#v", reader.calls, prepared)
	}

	// A historical opaque marker remains audit-readable, but public tool
	// projection must strip it and cannot turn it into current evidence.
	_, _, _, historicalMarker := legacyPreparedSettlementFixture(t)
	privateOutput := map[string]any{
		"executed": true,
		domainmcp.HostEvidenceSettlementCarrierKey: domainmcp.HostEvidenceSettlementCarrier{Marker: historicalMarker},
	}
	publicOutput := toolcatalogapp.PersistableToolOutputForExecution(input.Call, input.Context, input.Grant, privateOutput)
	publicBody, _ := json.Marshal(publicOutput)
	if bytes.Contains(publicBody, []byte(historicalMarker.SettlementID)) || bytes.Contains(publicBody, []byte("2645472")) {
		t.Fatalf("historical private marker entered public output: %s", publicBody)
	}
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID,
		CreatedAt:  evidenceIssuerTime().Add(3 * time.Second).Format(time.RFC3339Nano),
		FinishedAt: evidenceIssuerTime().Add(3 * time.Second).Format(time.RFC3339Nano),
		Call:       input.Call, Projection: toolcatalogapp.BuildPublicToolResultProjectionV1(input.Call.Name, publicOutput, false),
		ContextDigest: input.Context.ContextDigest, ContextEpoch: input.Context.ContextEpoch,
		ExecutionGrantID: input.Grant.GrantID, EvidenceSettlement: &historicalMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if records.ResultItem["hostEvidenceSettlement"] == nil || records.Event["item"].(map[string]any)["hostEvidenceSettlement"] != nil {
		t.Fatalf("historical settlement marker must remain durable-thread-only: result=%#v event=%#v", records.ResultItem, records.Event)
	}

	registry := service.Issuer.Registry.(*memoryEvidenceRegistry)
	boundary, err := FinalizeCaseBoundary(context.Background(), registry, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSuccess, IssuedAt: evidenceIssuerTime().Add(4 * time.Second),
	})
	if err != nil || boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer ||
		bytes.Contains([]byte(boundary.Text), []byte("2645472")) {
		t.Fatalf("legacy count reached deterministic fact publication: boundary=%#v err=%v", boundary, err)
	}
	reportBoundary, err := FinalizeCaseBoundary(context.Background(), registry, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSuccess, ReportRequested: true,
		IssuedAt: evidenceIssuerTime().Add(4 * time.Second),
	})
	if err != nil || reportBoundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer ||
		bytes.Contains([]byte(reportBoundary.Text), []byte("2645472")) {
		t.Fatalf("legacy count reached report publication: boundary=%#v err=%v", reportBoundary, err)
	}
	thread := toolEvidenceDurableThread(input, historicalMarker, evidenceIssuerTime().Add(3*time.Second))
	if receipt, err := service.CommitCurrentToolEvidence(context.Background(), CommitToolEvidenceInput{
		Context: input.Context, Marker: historicalMarker, Thread: thread,
	}); err == nil || receipt.ReceiptID != "" {
		t.Fatalf("historical marker minted current evidence: receipt=%#v err=%v", receipt, err)
	}
	if registry.commitCalls != 0 || (registry.initialized && registry.registry.Sequence != 0) {
		t.Fatalf("legacy count mutated registry: calls=%d registry=%#v", registry.commitCalls, registry.registry)
	}
}
func TestFundsCountToolEvidenceRejectsUntrustedCandidateFields(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"snapshot mismatch": func(record map[string]any) {
			record["datasetSnapshotId"] = securitycontexttest.DatasetSnapshotID("other")
		},
		"numeric count":   func(record map[string]any) { record["rowCount"] = float64(2) },
		"leading zero":    func(record map[string]any) { record["rowCount"] = "02" },
		"extra authority": func(record map[string]any) { record["safeToAnswer"] = true },
		"partial":         func(record map[string]any) { record["paginationComplete"] = false },
	} {
		t.Run(name, func(t *testing.T) {
			service, reader, input := toolEvidenceServiceFixture(t, "2")
			record := reader.raw.Value.(map[string]any)
			mutate(record)
			reader.raw = losslessEvidenceCandidate(t, record)
			if prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input); err == nil || !eligible || prepared.Authority.Record.ReceiptID != "" {
				t.Fatalf("untrusted candidate prepared evidence: prepared=%#v eligible=%v err=%v", prepared, eligible, err)
			}
		})
	}
}

func TestFailedToolOutcomeCannotBeUpgradedByEvidenceRead(t *testing.T) {
	for name, state := range map[string]struct {
		transport domainevidence.TransportStatus
		semantic  domainevidence.SemanticStatus
	}{
		"semantic failure":  {domainevidence.TransportSuccess, domainevidence.SemanticFailure},
		"transport failure": {domainevidence.TransportFailure, domainevidence.SemanticFailure},
		"timeout":           {domainevidence.TransportTimeout, domainevidence.SemanticTimeout},
		"cancel":            {domainevidence.TransportCancelled, domainevidence.SemanticCancelled},
	} {
		t.Run(name, func(t *testing.T) {
			service, reader, input := toolEvidenceServiceFixture(t, "2")
			input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
				ToolName: input.Call.Name, ToolCallID: input.Call.ID, ContextDigest: input.Context.ContextDigest,
				ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
				DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
				TransportStatus: state.transport, SemanticStatus: state.semantic, IsError: true, IssuedAt: evidenceIssuerTime(),
			})
			if prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input); err == nil || !eligible || reader.calls != 0 || prepared.Authority.Record.ReceiptID != "" {
				t.Fatalf("negative outcome reached evidence read: prepared=%#v eligible=%v calls=%d err=%v", prepared, eligible, reader.calls, err)
			}
		})
	}
}

func TestSourceSafeToAnswerFalseCannotIssueReceipt(t *testing.T) {
	service, reader, input := toolEvidenceServiceFixture(t, "2")
	reportedSafe := false
	input.Outcome = domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ContextDigest: input.Context.ContextDigest,
		ExecutionGrantID: input.Grant.GrantID, CaseID: input.Context.CaseID, ContextEpoch: input.Context.ContextEpoch,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, ServerIdentity: input.Grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess,
		ReportedSafeToAnswer: &reportedSafe, IssuedAt: evidenceIssuerTime(),
	})
	if _, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input); err == nil || !eligible || reader.calls != 0 {
		t.Fatalf("safeToAnswer=false reached evidence read: eligible=%v calls=%d err=%v", eligible, reader.calls, err)
	}
}

func TestFundsEvidenceRejectsDuplicateRawFieldsAndInvalidUnicode(t *testing.T) {
	for name, mutate := range map[string]func([]byte) []byte{
		"duplicate": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"rowCount":"2"`), []byte(`"rowCount":"1","rowCount":"2"`), 1)
		},
		"invalid utf8": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"rowCount":"2"`), []byte{'"', 'r', 'o', 'w', 'C', 'o', 'u', 'n', 't', '"', ':', '"', '2', 0xff, '"'}, 1)
		},
		"unpaired surrogate": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte(`"rowCount":"2"`), []byte(`"rowCount":"2","note":"\uD800"`), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			service, reader, input := toolEvidenceServiceFixture(t, "2")
			raw := mutate(append([]byte(nil), reader.raw.RawResult...))
			reader.raw = domainmcp.LosslessToolResult{Value: reader.raw.Value, RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw)}
			if prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input); err == nil || !eligible || prepared.Authority.Record.ReceiptID != "" {
				t.Fatalf("ambiguous raw evidence was prepared: prepared=%#v eligible=%v err=%v", prepared, eligible, err)
			}
		})
	}
}

func toolEvidenceServiceFixture(t *testing.T, count string) (ToolEvidenceService, *lockedEvidenceReaderStub, PrepareToolEvidenceInput) {
	t.Helper()
	now := evidenceIssuerTime()
	snapshotID := securitycontexttest.DatasetSnapshotID("dataset-count")
	securityContext := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-count", TurnID: "turn-count", WorkspaceRealPath: "/workspace", CaseID: "case-count",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-count")), DatasetSnapshotID: snapshotID,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-count")), ContextEpoch: 7, IssuedAt: now,
	})
	arguments := json.RawMessage(`{"table_name":"analysis_txn_detail_idx"}`)
	call := domainmodel.ToolCall{ID: evidenceTestHostToolCallID(t, "count"), Name: fundsCountEvidenceCanonicalTool, Arguments: arguments}
	serverIdentity := evidenceTestVerifiedMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 7)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: serverIdentity,
		ToolName: call.Name, ToolCallID: call.ID, ConnectionEpoch: 7, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema-count")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope-count")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	grantRegistry, err := domainsecurity.RegisterExecutionGrant(domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: call.Name, ToolCallID: call.ID, ContextDigest: securityContext.ContextDigest, ExecutionGrantID: grant.GrantID,
		CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ServerIdentity: grant.ServerIdentity, TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess,
		IssuedAt: now.Add(time.Second),
	})
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: serverIdentity, ConnectionEpoch: 7,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog-count")), SpecFingerprint: domainsecurity.SHA256Hex([]byte("spec-count")),
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID, CheckedAt: now, Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: fundsCountEvidenceServerVersion,
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: snapshotID,
			Ready: true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := map[string]any{
		"schemaVersion": float64(1), "purpose": fundsCountEvidencePurpose, "serverName": "analytix_funds",
		"serverVersion": fundsCountEvidenceServerVersion, "toolName": "count_case_rows", "caseId": securityContext.CaseID,
		"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
		"datasetSnapshotId": snapshotID, "snapshotContract": "analytix_duckdb_dataset_snapshot_v1",
		"tableName": fundsCountEvidenceTable, "noFilter": true, "rowCount": count, "paginationComplete": true, "readOnly": true,
	}
	reader := &lockedEvidenceReaderStub{context: securityContext, probe: probe, raw: losslessEvidenceCandidate(t, candidate)}
	issuer := Issuer{
		Registry: &memoryEvidenceRegistry{}, SettlementStore: &memoryEvidenceSettlementStore{}, Authority: newMemoryFinalAuthority(81),
		Now: func() time.Time { return now.Add(2 * time.Second) },
	}
	return ToolEvidenceService{Issuer: issuer, Reader: reader, Now: func() time.Time { return now.Add(time.Second) }}, reader, PrepareToolEvidenceInput{
		Context: securityContext, Grant: grant, GrantRegistry: grantRegistry, Call: call, Outcome: outcome,
		ResultItemID: toolcatalogapp.ToolResultItemID(securityContext.TurnID, call.ID),
	}
}

func losslessEvidenceCandidate(t *testing.T, value map[string]any) domainmcp.LosslessToolResult {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return domainmcp.LosslessToolResult{Value: decoded, RawResult: body, RawSHA256: domainsecurity.SHA256Hex(body)}
}

func toolEvidenceDurableThread(input PrepareToolEvidenceInput, marker domainevidence.HostEvidenceSettlementMarker, settledAt time.Time) map[string]any {
	callItem := map[string]any{
		"id": "item_tool_count", "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": input.Context.ThreadID, "turnId": input.Context.TurnID, "toolName": input.Call.Name, "callId": input.Call.ID,
		"arguments": map[string]any{"table_name": fundsCountEvidenceTable}, "executionGrant": strictTestRecord(input.Grant),
		"executionGrantId": input.Grant.GrantID, "contextDigest": input.Context.ContextDigest,
		"contextEpoch": float64(input.Context.ContextEpoch), "createdAt": input.Grant.IssuedAt,
	}
	resultItem := map[string]any{
		"id": input.ResultItemID, "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": input.Context.ThreadID, "turnId": input.Context.TurnID, "toolName": input.Call.Name, "callId": input.Call.ID,
		"executionGrantId": input.Grant.GrantID, "contextDigest": input.Context.ContextDigest,
		"contextEpoch": float64(input.Context.ContextEpoch), "createdAt": settledAt.Format(time.RFC3339Nano),
		"finishedAt": settledAt.Format(time.RFC3339Nano), "isError": false, "hostEvidenceSettlement": marker,
	}
	return map[string]any{
		"id": input.Context.ThreadID,
		"turns": []any{map[string]any{
			"id": input.Context.TurnID, "threadId": input.Context.ThreadID, "status": "completed",
			"securityContext": strictTestRecord(input.Context), "items": []any{callItem, resultItem},
		}},
	}
}
