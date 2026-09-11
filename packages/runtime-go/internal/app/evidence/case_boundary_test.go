package evidence

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type caseTerminalStoreStub struct {
	status      string
	items       []map[string]any
	fields      map[string]any
	events      []map[string]any
	unchanged   bool
	finishErr   error
	finishCalls int
	finishHook  func()
}

func turnSecurityContextRecord(securityContext domainsecurity.TurnSecurityContext) map[string]any {
	body, _ := json.Marshal(securityContext)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func (stub *caseTerminalStoreStub) FinishTurnIfActiveWithItemsAndFields(_, _ string, status string, items []map[string]any, fields map[string]any) (bool, string, error) {
	stub.finishCalls++
	if stub.finishHook != nil {
		stub.finishHook()
	}
	stub.status = status
	if !stub.unchanged {
		stub.items, stub.fields = items, fields
	}
	return !stub.unchanged, status, stub.finishErr
}

func (stub *caseTerminalStoreStub) FinishTurnIfActiveWithAcceptedFinalAuthority(
	threadID, turnID, status string,
	items []map[string]any,
	fields map[string]any,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
) (bool, string, error) {
	if _, err := appturn.ValidateAcceptedFinalCASAuthority(
		threadID, turnID, status, items, fields, privateFinal, factAuthority,
	); err != nil {
		return false, "", err
	}
	return stub.FinishTurnIfActiveWithItemsAndFields(threadID, turnID, status, items, fields)
}

func (stub *caseTerminalStoreStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, nil, nil
}

func (stub *caseTerminalStoreStub) RecordAcceptedFinalEventBundle(bundle []map[string]any) ([]map[string]any, error) {
	recorded := make([]map[string]any, 0, len(bundle))
	for index, event := range bundle {
		item := contracts.CloneMap(event)
		item["seq"] = float64(len(stub.events) + index + 1)
		recorded = append(recorded, item)
	}
	stub.events = append(stub.events, recorded...)
	return recorded, nil
}

func (stub *caseTerminalStoreStub) FinalPublicationThread(securityContext domainsecurity.TurnSecurityContext) (map[string]any, error) {
	items := make([]any, 0, len(stub.items))
	for _, item := range stub.items {
		items = append(items, item)
	}
	status := stub.status
	if strings.TrimSpace(status) == "" {
		status = "running"
	}
	turn := map[string]any{
		"id": securityContext.TurnID, "status": status, "securityContext": turnSecurityContextRecord(securityContext), "items": items,
	}
	for key, value := range stub.fields {
		turn[key] = contracts.CloneValue(value)
	}
	contextRecord := turnSecurityContextRecord(securityContext)
	return map[string]any{"id": securityContext.ThreadID, "securityState": contextRecord, "turns": []any{turn}}, nil
}

func TestCaseFundUnavailableReplacesFabricatedFinal(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	fabricated := "五家公司交易 2,645,472 条，账户 6222020000000000000，MAC 00:11:22:33:44:55。"
	result, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSourceUnavailable, SourceUnavailable: true, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || result.Envelope.Variant != domainevidence.SourceUnavailableAnswer {
		t.Fatalf("source-unavailable boundary mismatch: result=%#v err=%v", result, err)
	}
	for _, forbidden := range []string{fabricated, "2,645,472", "6222020000000000000", "00:11:22:33:44:55"} {
		if strings.Contains(result.Text, forbidden) {
			t.Fatalf("provider fabrication survived the host boundary: %q", result.Text)
		}
	}
}

func TestOrdinaryOnlyCaseLineageDoesNotCreateACaseResultSlot(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	slot, err := domainordinaryresult.NewResultSlotV1("The ordinary read completed.")
	if err != nil {
		t.Fatal(err)
	}
	result, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
		CaseSlotIntent: CaseSlotNotRequestedV1, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || result.Envelope.Variant != domainevidence.GeneralGuidanceAnswer ||
		result.Text != slot.Text || strings.Contains(result.Text, "案件事实") || strings.Contains(result.Text, "资金分析") {
		t.Fatalf("ordinary-only case lineage acquired a case result slot: result=%#v err=%v", result, err)
	}
	if _, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSuccess, OrdinaryResult: &slot,
		IssuedAt: evidenceIssuerTime(),
	}); err == nil {
		t.Fatal("ordinary result without a host-classified case-slot intent reached publication")
	}
}

func TestRiskAuthorityUnavailableUsesAgentSafetyBoundary(t *testing.T) {
	const workspace = "/workspace/risk-authority-unavailable"
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("risk-authority-unavailable-policy")),
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:       domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"risk-authority-unavailable-binding",
		)),
		BlockerCode: domainsecurity.PublicationBlockerRiskAuthorityUnavailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-risk-authority-unavailable", TurnID: "turn-risk-authority-unavailable",
		WorkspaceRealPath: workspace, TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: evidenceIssuerTime(), PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) {
		t.Fatalf("risk-authority boundary context is invalid: context=%#v err=%v", securityContext, err)
	}

	result, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: securityContext, TerminalReason: TerminalSuccess,
		PublicationBlocker: domainsecurity.PublicationBlockerRiskAuthorityUnavailable,
		IssuedAt:           evidenceIssuerTime(),
	})
	if err != nil || result.Envelope.Variant != domainevidence.GeneralGuidanceAnswer ||
		result.Text != domainevidence.AgentSafetyAuthorityUnavailableText ||
		strings.Contains(result.Text, "资金分析来源") {
		t.Fatalf("risk-authority boundary was misreported as a data-source failure: result=%#v err=%v", result, err)
	}
}

func TestPersistCaseBoundaryInputHasNoProviderResultOrArbitraryMapChannel(t *testing.T) {
	inputType := reflect.TypeOf(PersistCaseBoundaryInput{})
	for _, forbidden := range []string{"Result", "CacheDiagnostics", "AssistantText", "Chunks", "ToolCalls", "Reasoning"} {
		if _, ok := inputType.FieldByName(forbidden); ok {
			t.Fatalf("case publication input exposes forbidden provider channel %q", forbidden)
		}
	}
	telemetry, ok := inputType.FieldByName("Telemetry")
	if !ok || telemetry.Type != reflect.TypeOf(appusage.TerminalTelemetryV1{}) {
		t.Fatalf("case publication telemetry is not the closed V1 projection: %#v", telemetry)
	}
	for index := 0; index < inputType.NumField(); index++ {
		field := inputType.Field(index)
		if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Slice {
			t.Fatalf("case publication input exposes open collection field %s (%s)", field.Name, field.Type)
		}
	}
}

func TestCaseBoundaryReportRequiresPublicationReceipt(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	result, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalReportFallback, ReportRequested: true, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || result.Envelope.Variant != domainevidence.NeedsEvidenceAnswer || len(result.Envelope.MissingScope) != 1 || result.Envelope.MissingScope[0] != "publication_receipt" {
		t.Fatalf("report boundary mismatch: result=%#v err=%v", result, err)
	}
}

func TestSourceUnavailablePrecedesReportPublicationReceiptGap(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	result, err := FinalizeCaseBoundary(context.Background(), nil, CaseBoundaryInput{
		Context: input.Context, TerminalReason: TerminalSuccess, SourceUnavailable: true,
		ReportRequested: true, PublicationBlocker: domainsecurity.PublicationBlockerCaseBindingMissing,
		IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || result.Envelope.Variant != domainevidence.SourceUnavailableAnswer ||
		result.Envelope.Blocker != "current_case_source_unavailable" {
		t.Fatalf("source-unavailable report boundary was downgraded: result=%#v err=%v", result, err)
	}
}

func TestEveryTerminalPathUsesFinalEvidenceGate(t *testing.T) {
	_, fixture := evidenceIssuerFixture(t)
	known := map[TerminalReason]bool{}
	for _, row := range ProductionTerminalReasonOwnershipV1() {
		if known[row.Reason] {
			t.Fatalf("duplicate terminal ownership row: %s", row.Reason)
		}
		known[row.Reason] = true
		t.Run(string(row.Reason), func(t *testing.T) {
			store := &caseTerminalStoreStub{}
			finalizer, _, _, _ := newTestCasePublicationFinalizer()
			result, err := PersistCaseTerminalBoundary(context.Background(), finalizer, PersistCaseBoundaryInput{
				Store: store, Context: fixture.Context, TerminalReason: row.Reason,
				SourceUnavailable: row.Reason == TerminalSourceUnavailable,
				ThreadID:          fixture.Context.ThreadID, TurnID: fixture.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
			})
			if row.Class != TerminalEmissionTurnV1 {
				if err == nil || store.finishCalls != 0 || result.Persistence.Changed {
					t.Fatalf("non-turn lifecycle reason entered case terminal emitter: row=%#v result=%#v err=%v", row, result, err)
				}
				return
			}
			if len(row.Owners) == 0 {
				t.Fatalf("production turn reason has no owner: %#v", row)
			}
			expectedStatus, ok := domainevidence.FinalAnswerTerminalStatus(string(row.Reason))
			if !ok || err != nil || store.status != expectedStatus || store.fields["acceptedFinal"] == nil || !result.Persistence.Changed {
				t.Fatalf("terminal path bypassed gate: reason=%s status=%q items=%#v fields=%#v err=%v", row.Reason, store.status, store.items, store.fields, err)
			}
			if len(store.items) == 0 || store.items[0]["acceptedFinal"] == nil || len(store.events) == 0 || store.events[len(store.events)-1]["kind"] != "turn_"+expectedStatus {
				t.Fatalf("terminal path lacks accepted projection: reason=%s items=%#v events=%#v", row.Reason, store.items, store.events)
			}
		})
	}
	for _, reason := range AllTerminalReasons() {
		if !known[reason] {
			t.Fatalf("closed terminal reason lacks production ownership classification: %s", reason)
		}
	}
	if len(known) != len(AllTerminalReasons()) {
		t.Fatalf("ownership table contains reasons outside the closed vocabulary: table=%d reasons=%d", len(known), len(AllTerminalReasons()))
	}
}
