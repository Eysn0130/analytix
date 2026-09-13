package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	gateprojectionapp "analytix.local/runtime-go/internal/app/gateprojection"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	evidenceregistryv2fixture "analytix.local/runtime-go/internal/testsupport/evidenceregistryv2"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestNewRuntimeServerHandlerFromComponentsPublishesConcreteFactFinalWitnessAsTypedOutput(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	config := RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}
	base := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	workspace := workspacetest.New(t)
	thread, err := base.store.CreateThread(map[string]any{
		"title": "concrete fact-final production composition", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-concrete-fact-final-production"

	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid,
		CaseID:          "case-concrete-fact-final-production",
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("concrete-fact-final-binding-file")),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("concrete-fact-final-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	riskAuthority := newServerTestRiskAuthority()
	risk, err := turnsecurityapp.ResolveRiskPublication(ctx, turnsecurityapp.RiskPolicyResolutionInput{
		Authority: riskAuthority, ThreadID: threadID, WorkspaceRealPath: workspace,
		Binding: observation, RiskIntent: domainsecurity.RiskClassCase, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	finalAuthority, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(t.TempDir(), "final-authority", "ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newProductionConcreteWitnessFixture(t, productionConcreteWitnessInput{
		Root: t.TempDir(), ThreadID: threadID, TurnID: turnID, Observation: observation,
		PublicationPolicy: risk.Publication, RiskBinding: risk.RiskAuthorityBinding,
		Authority: finalAuthority, Now: now,
	})

	caseAuthorityKey, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(t.TempDir(), "case-authority", "ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(t.TempDir(), "case-authority-records"))
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority, err := casethreadapp.NewRegistry(ctx, caseAuthorityKey, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.RegisterRequired(ctx, caseAuthority, fixture.securityContext); err != nil {
		t.Fatal(err)
	}
	privateFinals, err := newServerTestPrivateFinalStore(t, filepath.Join(t.TempDir(), "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(config.DurableTempDir)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(base.store, casReader)
	trustedFinals := gateprojectionapp.NewTrustedFinalProjectionIndexWithReadback(finalAuthority, eventIO.Readback)
	terminalCoordinator := newServerTestTurnTerminalCoordinator(t, finalAuthority, privateFinals)
	finalizer := evidenceapp.NewCasePublicationFinalizerWithHostEvidenceAuthority(
		fixture.registry, fixture.registry, finalAuthority, privateFinals, eventIO,
		terminalCoordinator,
		fixture.host, fixture.dataset, trustedFinals,
	)
	currentCaseAuthority := threadapp.NewCurrentCaseThreadAuthorityValidator(caseAuthority, fixture.dataset, nil)
	publicProjector := threadapp.NewTrustedPublicProjectorWithPrimaryCAS(
		trustedFinals, caseAuthority, currentCaseAuthority, casReader,
	)

	handlerValue, err := NewRuntimeServerHandlerFromComponents(config, RuntimeServerComponents{
		Store: base.store, Memories: base.memories, CaseFinalizer: finalizer,
		CaseThreads: caseAuthority,
		TurnSecurity: turnsecurityapp.WorkspaceSecurityAuthority{
			Identity: testIdentityAuthority(), Observer: fixture.dataset, RiskAuthority: riskAuthority,
			SnapshotAuthorityV2: fixture.dataset,
		},
		Continuations: base.continuations, PendingWork: base.pendingWork, Checkpoints: base.checkpoints,
		PublicProjector: publicProjector, ProviderConfig: base.providerConfig,
		NativeAuthority: base.nativeAuthority, SteeringAuthority: base.steeringAuthority,
		SubagentState: base.subagentState, G6Readiness: base.g6Readiness, SkipMCPConnect: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := handlerValue.(*runtimeServerHandler)
	epochState, err := contextepochapp.BootstrapState(
		threadID, fixture.securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(fixture.securityContext)}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := casethreadapp.CommitRequired(ctx, caseAuthority, fixture.securityContext, epochState, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(fixture.securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord,
		"items": []any{map[string]any{
			"id": "user-concrete-fact-final", "threadId": threadID, "turnId": turnID,
			"kind": "user_message", "role": "user", "status": "completed", "text": "SYNTHETIC_CASE_REQUEST",
		}},
	}, "synthetic-provider", map[string]any{
		"securityState": securityRecord, "contextEpochState": contextepochapp.PublicState(epochState),
	}); err != nil {
		t.Fatal(err)
	}
	if !handler.runtimeControl().RegisterTurnCancel(threadID, turnID, func() {}) {
		t.Fatal("production handler did not register the synthetic turn execution owner")
	}
	defer handler.runtimeControl().UnregisterTurnCancel(threadID, turnID)
	preObservation, err := casReader.ReadAcceptedFinalCASObservation(ctx, threadID, turnID)
	if err != nil || preObservation.HasWinner {
		t.Fatalf("synthetic handler turn already had a public winner before finalization: observation=%#v err=%v", preObservation, err)
	}
	result, err := handler.runtimePublicationAuthority().PersistCurrentCaseCandidate(ctx, evidenceapp.PersistCaseBoundaryInput{
		Store: handler.store, Context: fixture.securityContext, TerminalReason: evidenceapp.TerminalSuccess,
		CaseSlotIntent: evidenceapp.CaseSlotRequestedV1, ThreadID: threadID, TurnID: turnID,
		AcceptedAt: now.Add(5 * time.Minute),
	}, true)
	if err != nil || !result.Persistence.Changed ||
		result.Boundary.Envelope.Variant != domainevidence.EvidenceBackedAnswer ||
		result.Persistence.AcceptedFinal.FactFinalWitnessAdmission == nil ||
		result.Persistence.AcceptedFinal.PublicView == nil {
		t.Fatalf("production handler did not publish a concrete witnessed fact-final: result=%#v err=%v hostErr=%v", result, err, fixture.host.lastErr)
	}
	privateRecord, err := privateFinals.Resolve(ctx, result.Persistence.AcceptedFinal.RecordDigest)
	if err != nil || fixture.registry.VerifyFactFinalWitnessCurrent(ctx, privateRecord) != nil {
		t.Fatalf("production handler fact-final is not current in the same concrete registry: err=%v", err)
	}
	if fixture.coordinator.advanceCalls != 1 {
		t.Fatalf("fact-final issuance advanced or replaced registry authority: advances=%d", fixture.coordinator.advanceCalls)
	}
	recovered, recoverErr := terminalCoordinator.RecoverV1(ctx, turnterminalapp.RestartRecoveryInputV1{
		CompletionStore: handler.store, CASReader: casReader,
		AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{privateRecord},
	})
	if recoverErr != nil || len(recovered.Complete) != 0 || len(recovered.NonExecutableAuditOnly) != 1 ||
		recovered.NonExecutableAuditOnly[0].AcceptedFinal.RecordDigest != privateRecord.AcceptedFinal.RecordDigest {
		t.Fatalf("valid fact-bearing V5 escaped non-executable audit retention: recovered=%#v err=%v", recovered, recoverErr)
	}
	historicalV4 := productionHistoricalWitnessedFactV4(t, ctx, finalAuthority, privateRecord)
	historicalV4WithV2Admission := historicalV4.AcceptedFinal
	historicalV4WithV2Admission.FactFinalWitnessAdmission = privateRecord.AcceptedFinal.FactFinalWitnessAdmission
	if validationErr := domainevidence.ValidateAcceptedFinalRecord(historicalV4WithV2Admission); validationErr == nil ||
		!strings.Contains(validationErr.Error(), "witnessed evidence authority is invalid") {
		t.Fatalf("historical V4 admitted a current V2 fact-final witness: %v", validationErr)
	}
	auditStore := &productionHistoricalAuditPrivateStore{record: historicalV4}
	historicalCoordinator := newServerTestTurnTerminalCoordinator(t, finalAuthority, auditStore)
	seqBeforeHistoricalAudit, err := handler.store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	recovered, recoverErr = historicalCoordinator.RecoverV1(ctx, turnterminalapp.RestartRecoveryInputV1{
		CompletionStore: handler.store, CASReader: casReader,
		AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{historicalV4},
	})
	if recoverErr != nil || len(recovered.Complete) != 0 || len(recovered.NonExecutableAuditOnly) != 1 ||
		recovered.NonExecutableAuditOnly[0].AcceptedFinal.RecordDigest != historicalV4.AcceptedFinal.RecordDigest {
		t.Fatalf("historical V4 fact escaped audit-only retention: recovered=%#v err=%v", recovered, recoverErr)
	}
	seqAfterHistoricalAudit, err := handler.store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if auditStore.writeCalls != 0 || seqAfterHistoricalAudit != seqBeforeHistoricalAudit {
		t.Fatalf("historical V4 audit recovery performed a write: private=%d seq=%d->%d", auditStore.writeCalls, seqBeforeHistoricalAudit, seqAfterHistoricalAudit)
	}
	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publicProjector.ProjectThread(rawThread); err != nil {
		t.Fatalf("production public projector rejected the durable witnessed final: %v", err)
	}

	server := httptest.NewServer(handler)
	defer server.Close()
	publicThread := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK,
	)
	publicBody, err := json.Marshal(publicThread)
	if err != nil {
		t.Fatal(err)
	}
	sseBody := requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0")
	publicTurns := listAny(publicThread["turns"])
	if len(publicTurns) != 1 {
		t.Fatalf("production HTTP projection omitted the witnessed turn: %s", publicBody)
	}
	publicTurn, _ := publicTurns[0].(map[string]any)
	publicView, _ := publicTurn["acceptedFinalView"].(map[string]any)
	if stringField(publicView, "variant") != string(domainevidence.EvidenceBackedAnswer) ||
		acceptedFinalAssistantText(publicTurn) != result.Boundary.Text ||
		!strings.Contains(acceptedFinalAssistantText(publicTurn), "4200000 CNY") {
		t.Fatalf("production HTTP projection omitted the handler-owned typed fact-final: %s", publicBody)
	}
	if !strings.Contains(sseBody, `"variant":"EvidenceBackedAnswer"`) ||
		!strings.Contains(sseBody, result.Persistence.AcceptedFinal.RecordDigest) {
		t.Fatalf("production SSE projection omitted the handler-owned typed fact-final: %s", sseBody)
	}
	if batches := strings.Count(sseBody, `"kind":"accepted_final_batch"`); batches != 1 {
		t.Fatalf("production SSE projection delivered %d accepted-final batches, want exactly one: %s", batches, sseBody)
	}
	expectedView, err := domainevidence.NewAcceptedFinalPublicViewV3WithWitness(
		privateRecord,
		func(candidate domainevidence.PrivateAcceptedFinalRecord) error {
			return fixture.registry.VerifyFactFinalWitnessCurrent(ctx, candidate)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	expectedViewRecord := domainevidence.AcceptedFinalPublicViewRecordV3(expectedView)
	if expectedViewRecord == nil || expectedView.SchemaVersion != domainevidence.AcceptedFinalPublicViewV3Version ||
		expectedView.ReceiptMetadata.Count == 0 || len(expectedView.ReceiptMetadata.Citations) == 0 ||
		expectedView.ReceiptMetadata.SetDigest == "" || expectedView.CheckedScopeDigest == "" {
		t.Fatalf("production generic V3 view omitted accepted masked citation semantics: %#v", expectedView)
	}
	if _, present := publicTurn["acceptedFinal"]; present {
		t.Fatalf("production HTTP turn exposed the private signed accepted-final record: %s", publicBody)
	}
	if !productionSameJSON(publicView, expectedViewRecord) {
		t.Fatalf("production HTTP turn changed the exact generic V3 view:\n got=%#v\nwant=%#v", publicView, expectedViewRecord)
	}
	productionRequireClosedV3Shape(t, "production HTTP turn", publicView)
	productionRequireV3PrivateValueZeroBytes(t, "production HTTP turn", publicView, privateRecord)
	publicItems := listAny(publicTurn["items"])
	if len(publicItems) == 0 {
		t.Fatalf("production HTTP turn omitted its accepted assistant item: %s", publicBody)
	}
	for _, rawItem := range publicItems {
		item, _ := rawItem.(map[string]any)
		if _, present := item["acceptedFinal"]; present {
			t.Fatalf("production HTTP item exposed the private signed accepted-final record: %s", publicBody)
		}
		if itemView, present := item["acceptedFinalView"].(map[string]any); present {
			if !productionSameJSON(itemView, expectedViewRecord) {
				t.Fatalf("production HTTP item changed the exact generic V3 view: %#v", itemView)
			}
		}
	}
	publicBatch := productionAcceptedFinalBatchFromSSE(t, sseBody)
	batchVersion, versionOK := contracts.NumericSeq(publicBatch["schemaVersion"])
	if !versionOK || batchVersion != domainevent.AcceptedFinalDeliveryBatchV2Version ||
		stringField(publicBatch, "purpose") != domainevent.AcceptedFinalDeliveryBatchV2Purpose ||
		strings.Contains(sseBody, domainevent.AcceptedFinalDeliveryBatchV1Purpose) {
		t.Fatalf("production SSE did not emit the explicit current V2 delivery wire: %s", sseBody)
	}
	publicEvents := listAny(publicBatch["events"])
	if len(publicEvents) == 0 {
		t.Fatalf("production SSE batch omitted its projected events: %s", sseBody)
	}
	assistantEvent, _ := publicEvents[0].(map[string]any)
	assistantItem, _ := assistantEvent["item"].(map[string]any)
	if _, present := assistantItem["acceptedFinal"]; present {
		t.Fatalf("production SSE item exposed the private signed accepted-final record: %s", sseBody)
	}
	assistantView, present := assistantItem["acceptedFinalView"].(map[string]any)
	if !present || !productionSameJSON(assistantView, expectedViewRecord) {
		t.Fatalf("production SSE item omitted or changed the exact generic V3 view: %s", sseBody)
	}
	productionRequireClosedV3Shape(t, "production SSE item", assistantView)
	productionRequireV3PrivateValueZeroBytes(t, "production SSE item", assistantView, privateRecord)
	latestHTTPDelivery, _ := publicThread["acceptedFinalDelivery"].(map[string]any)
	allHTTPDeliveries := listAny(publicThread["acceptedFinalDeliveries"])
	if latestHTTPDelivery == nil || len(allHTTPDeliveries) != 1 ||
		!productionSameJSON(latestHTTPDelivery, allHTTPDeliveries[0]) {
		t.Fatalf("production HTTP projection did not bind its visible final to one exact delivery: %s", publicBody)
	}
	if roots := productionRequireExactAcceptedFinalViews(t, "http", publicThread, expectedViewRecord); roots != 4 {
		t.Fatalf("production HTTP projection exposed %d generic V3 views, want turn+item+latest-delivery+per-turn-delivery=4: %s", roots, publicBody)
	}
	if roots := productionRequireExactAcceptedFinalViews(t, "sse", publicBatch, expectedViewRecord); roots != 1 {
		t.Fatalf("production SSE projection exposed %d generic V3 views, want 1: %s", roots, sseBody)
	}
	privateAdmission := privateRecord.AcceptedFinal.FactFinalWitnessAdmission
	if privateAdmission == nil || privateRecord.PublicationSnapshotProof == nil {
		t.Fatal("production private V5 omitted fact-final witness authority")
	}
	for name, body := range map[string]string{"http": string(publicBody), "sse": sseBody} {
		for _, forbidden := range []string{
			`"acceptedFinal":`, `"factFinalWitnessAdmission":`,
			`"publicationSnapshotProof":`, `"publicationSnapshotProofDigest":`,
			`"securityContext":`, `"envelope":`, `"registryHead":`,
			`"publicationIntent":`, `"storeDigest":`,
			`"envelopeDigest":`, `"contextDigest":`, `"contextEpoch":`,
			`"datasetSnapshotId":`, `"caseBindingHash":`, `"publicViewDigest":`,
			`"envelopeIssuedAt":`, `"renderedTextSha256":`,
			"AuthorityRef", "authorityRef",
			`"receiptDigest":`, `"rawSHA256":`,
			`"catalogFingerprint":`,
			`"specFingerprint":`, `"probeDigest":`, "sourceExactValue", "PRIVATE_REVERSE_MAP_SENTINEL",
			"production-concrete-private-raw",
			fixture.securityContext.ContextDigest, fixture.securityContext.CaseBindingHash,
			fixture.securityContext.DatasetSnapshotID,
			fixture.receipt.ReceiptDigest, fixture.receipt.RawSHA256,
			privateAdmission.AdmissionDigest, privateAdmission.WitnessBinding.BindingDigest,
			privateRecord.AcceptedFinal.PublicationSnapshotProofDigest,
			privateRecord.AcceptedFinal.PublicViewDigest,
			privateRecord.StoreDigest,
			privateRecord.RegistryHead.StateDigest,
			"source-production", "row-production", "account-masked-production",
			"synthetic-fixture", "SYNTHETIC_CASE_REQUEST",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s public typed output leaked private fact-final authority %q: %s", name, forbidden, body)
			}
		}
		for _, required := range []string{
			`"schemaVersion":3`,
			`"publicationState":"accepted"`,
			`"acceptedFinalDigest":"` + expectedView.AcceptedFinalDigest + `"`,
			`"checkedScopeDigest":"` + expectedView.CheckedScopeDigest + `"`,
			`"setDigest":"` + expectedView.ReceiptMetadata.SetDigest + `"`,
			`"handle":"` + expectedView.ReceiptMetadata.Citations[0].Handle + `"`,
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s public typed output omitted accepted generic V3 field/value %q: %s", name, required, body)
			}
		}
	}
}

func productionRequireClosedV3Shape(t *testing.T, name string, view map[string]any) {
	t.Helper()
	allowed := []string{
		"schemaVersion", "acceptedFinalDigest", "publicationState", "variant", "terminalReason",
		"blockerCode", "coverageStatus", "checkedScopeDigest", "missingScopeCount", "claimCount",
		"claimTypes", "receiptMetadata", "noHitWording", "acceptedAt",
	}
	if len(view) != len(allowed) {
		t.Fatalf("%s V3 keys are outside the closed allowlist: %#v", name, view)
	}
	for _, field := range allowed {
		if _, present := view[field]; !present {
			t.Fatalf("%s V3 omitted allowlisted field %q: %#v", name, field, view)
		}
	}
	receipt, ok := view["receiptMetadata"].(map[string]any)
	if !ok || len(receipt) != 4 {
		t.Fatalf("%s V3 receipt metadata is not closed: %#v", name, view["receiptMetadata"])
	}
	for _, field := range []string{"projection", "count", "setDigest", "citations"} {
		if _, present := receipt[field]; !present {
			t.Fatalf("%s V3 receipt metadata omitted allowlisted field %q: %#v", name, field, receipt)
		}
	}
	citations, ok := receipt["citations"].([]any)
	if !ok || len(citations) == 0 {
		t.Fatalf("%s V3 omitted masked citation metadata: %#v", name, receipt)
	}
	for index, raw := range citations {
		citation, ok := raw.(map[string]any)
		if !ok || len(citation) != 2 || citation["handle"] == nil || citation["label"] == nil {
			t.Fatalf("%s V3 citation %d is not the closed handle/label shape: %#v", name, index, raw)
		}
	}
	for _, forbidden := range []string{
		"publicViewDigest", "envelopeDigest", "contextDigest", "contextEpoch", "datasetSnapshotId",
		"envelopeIssuedAt", "threadId", "turnId", "renderedTextSha256", "acceptedFinal",
	} {
		if _, present := view[forbidden]; present {
			t.Fatalf("%s V3 exposed forbidden property %q: %#v", name, forbidden, view)
		}
	}
}

func productionRequireV3PrivateValueZeroBytes(
	t *testing.T,
	name string,
	view map[string]any,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
) {
	t.Helper()
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		privateRecord.AcceptedFinal.ThreadID,
		privateRecord.AcceptedFinal.TurnID,
		privateRecord.AcceptedFinal.EnvelopeDigest,
		privateRecord.AcceptedFinal.RenderedTextSHA256,
		privateRecord.AcceptedFinal.PublicViewDigest,
		privateRecord.SecurityContext.ContextDigest,
		privateRecord.SecurityContext.CaseBindingHash,
		privateRecord.SecurityContext.DatasetSnapshotID,
		privateRecord.PrivateRecordDigest,
		privateRecord.StoreDigest,
	} {
		if forbidden != "" && bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("%s V3 exposed private exact-value canary %q: %s", name, forbidden, body)
		}
	}
}

func productionRequireExactAcceptedFinalViews(t *testing.T, name string, value any, expected map[string]any) int {
	t.Helper()
	switch candidate := value.(type) {
	case map[string]any:
		roots := 0
		for field, nested := range candidate {
			if field == "acceptedFinal" {
				t.Fatalf("%s exposed private acceptedFinal: %#v", name, nested)
			}
			if field == "acceptedFinalView" {
				view, ok := nested.(map[string]any)
				if !ok || !productionSameJSON(view, expected) {
					t.Fatalf("%s acceptedFinalView is not the exact generic V3 projection: %#v", name, nested)
				}
				productionRequireClosedV3Shape(t, name, view)
				roots++
				continue
			}
			roots += productionRequireExactAcceptedFinalViews(t, name, nested, expected)
		}
		return roots
	case []any:
		roots := 0
		for _, nested := range candidate {
			roots += productionRequireExactAcceptedFinalViews(t, name, nested, expected)
		}
		return roots
	default:
		return 0
	}
}

func productionSameJSON(left, right any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func productionAcceptedFinalBatchFromSSE(t *testing.T, body string) map[string]any {
	t.Helper()
	for _, frame := range strings.Split(body, "\n\n") {
		if !strings.Contains(frame, "event: accepted_final_batch") {
			continue
		}
		for _, line := range strings.Split(frame, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var batch map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &batch); err != nil {
				t.Fatalf("decode production accepted-final SSE batch: %v", err)
			}
			return batch
		}
	}
	t.Fatalf("production SSE omitted accepted_final_batch: %s", body)
	return nil
}

func productionHistoricalWitnessedFactV4(
	t *testing.T,
	ctx context.Context,
	authority finalauthorityport.Authority,
	current domainevidence.PrivateAcceptedFinalRecord,
) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	if current.AcceptedFinal.FactFinalWitnessAdmission == nil {
		t.Fatal("current fact final omitted its witness admission")
	}
	historicalRendered, err := domainevidence.RenderFinalAnswerAtVersion(
		current.Envelope,
		domainevidence.HistoricalFinalAnswerRendererVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	legacyAdmission := *current.AcceptedFinal.FactFinalWitnessAdmission
	legacyAdmission.SchemaVersion = domainevidence.FactFinalWitnessAdmissionSchemaVersionV1
	legacyAdmission.Purpose = domainevidence.FactFinalWitnessAdmissionPurposeV1
	legacyAdmission.DatasetSnapshotIndexDigest = ""
	legacyAdmission.DatasetSnapshotCount = 0
	legacyAdmission.SelectedDatasetSnapshotIndexDigest = ""
	legacyAdmission.SelectedDatasetSnapshotIndexGeneration = 0
	legacyAdmission.SelectedDatasetSnapshotRecordDigest = ""
	legacyAdmission.DatasetSnapshotManifestDigest = ""
	legacyAdmission.FundsProducerContentID = ""
	legacyAdmission.FundsProducerContentManifestSHA256 = ""
	legacyAdmission.RenderedTextSHA256 = domainsecurity.SHA256Hex([]byte(historicalRendered))
	legacyAdmission.AdmissionDigest = ""
	legacyAdmissionDigestBody, err := json.Marshal(legacyAdmission)
	if err != nil {
		t.Fatal(err)
	}
	legacyAdmission.AdmissionDigest = domainsecurity.SHA256Hex(legacyAdmissionDigestBody)
	privateDigestBody, err := json.Marshal(struct {
		SchemaVersion             int                                         `json:"schemaVersion"`
		SecurityContext           domainsecurity.TurnSecurityContext          `json:"securityContext"`
		Envelope                  domainevidence.FinalAnswerEnvelope          `json:"envelope"`
		RenderedText              string                                      `json:"renderedText"`
		PublicationIntent         domainevidence.TerminalPublicationIntent    `json:"publicationIntent"`
		PublicationSnapshotProof  *domainevidence.PublicationSnapshotProof    `json:"publicationSnapshotProof,omitempty"`
		FactFinalWitnessAdmission *domainevidence.FactFinalWitnessAdmissionV1 `json:"factFinalWitnessAdmission,omitempty"`
	}{
		SchemaVersion:   domainevidence.WitnessedFactPrivateFinalRecordVersion,
		SecurityContext: current.SecurityContext, Envelope: current.Envelope, RenderedText: historicalRendered,
		PublicationIntent: current.PublicationIntent, PublicationSnapshotProof: current.PublicationSnapshotProof,
		FactFinalWitnessAdmission: &legacyAdmission,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateDigest := domainsecurity.SHA256Hex(privateDigestBody)
	accepted := current.AcceptedFinal
	accepted.SchemaVersion = domainevidence.WitnessedFactAcceptedFinalRecordVersion
	accepted.RendererVersion = domainevidence.HistoricalFinalAnswerRendererVersion
	accepted.FinalGateVersion = domainevidence.WitnessedFinalEvidenceGateVersion
	accepted.RenderedTextSHA256 = domainsecurity.SHA256Hex([]byte(historicalRendered))
	accepted.PublicView = nil
	accepted.PublicViewDigest = ""
	accepted.FactFinalWitnessAdmission = &legacyAdmission
	accepted.PrivateRecordDigest = privateDigest
	accepted.AuthoritySignature = ""
	accepted.RecordDigest = ""
	signature, err := authority.Sign(ctx, domainevidence.AcceptedFinalSigningBytes(accepted))
	if err != nil {
		t.Fatal(err)
	}
	accepted.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	acceptedDigestBody, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	accepted.RecordDigest = domainsecurity.SHA256Hex(acceptedDigestBody)
	historical := current
	historical.SchemaVersion = domainevidence.WitnessedFactPrivateFinalRecordVersion
	historical.RenderedText = historicalRendered
	historical.AcceptedFinal = accepted
	historical.PrivateRecordDigest = privateDigest
	historical.StoreDigest = ""
	storeDigestBody, err := json.Marshal(historical)
	if err != nil {
		t.Fatal(err)
	}
	historical.StoreDigest = domainsecurity.SHA256Hex(storeDigestBody)
	if err := domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(historical); err != nil {
		t.Fatalf("historical V4 fact fixture is invalid: %v", err)
	}
	if err := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(historical); err == nil {
		t.Fatal("historical V4 fact regained current publication authority")
	}
	return historical
}

type productionHistoricalAuditPrivateStore struct {
	record     domainevidence.PrivateAcceptedFinalRecord
	writeCalls int
}

func (store *productionHistoricalAuditPrivateStore) PutIfAbsent(context.Context, domainevidence.PrivateAcceptedFinalRecord) error {
	store.writeCalls++
	return errors.New("historical audit private store is read-only")
}

func (store *productionHistoricalAuditPrivateStore) Resolve(context.Context, string) (domainevidence.PrivateAcceptedFinalRecord, error) {
	return store.record, nil
}

func (store *productionHistoricalAuditPrivateStore) List(context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	return []domainevidence.PrivateAcceptedFinalRecord{store.record}, nil
}

func (store *productionHistoricalAuditPrivateStore) HasRecords(context.Context) (bool, error) {
	return true, nil
}

func (store *productionHistoricalAuditPrivateStore) PutDispositionIfAbsent(context.Context, domainevidence.AcceptedFinalDispositionRecord) error {
	store.writeCalls++
	return errors.New("historical audit private store is read-only")
}

func (store *productionHistoricalAuditPrivateStore) ResolveDisposition(context.Context, string) (domainevidence.AcceptedFinalDispositionRecord, error) {
	return domainevidence.AcceptedFinalDispositionRecord{}, errors.New("historical audit disposition is missing")
}

func (store *productionHistoricalAuditPrivateStore) ListDispositions(context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error) {
	return []domainevidence.AcceptedFinalDispositionRecord{}, nil
}

type productionConcreteWitnessInput struct {
	Root              string
	ThreadID          string
	TurnID            string
	Observation       domainsecurity.CaseBindingObservationV1
	PublicationPolicy domainsecurity.TurnPublicationPolicyV1
	RiskBinding       domainsecurity.RiskAuthorityBindingV1
	Authority         finalauthorityport.Authority
	Now               time.Time
}

type productionConcreteWitnessFixture struct {
	securityContext domainsecurity.TurnSecurityContext
	registry        *evidenceregistryapp.Service
	receipt         domainevidence.EvidenceReceipt
	dataset         *productionConcreteDatasetAuthority
	host            *productionConcreteHostAuthority
	coordinator     *productionConcreteCoordinator
}

func newProductionConcreteWitnessFixture(t *testing.T, input productionConcreteWitnessInput) productionConcreteWitnessFixture {
	t.Helper()
	publicKey := input.Authority.PublicKey()
	installationID := domainsecurity.SHA256Hex([]byte("production-concrete-fact-final-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("production-concrete-fact-final-enrollment"))
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: input.Observation, Material: "production-concrete-fact-final",
		InstallationID: installationID, AcceptedAt: input.Now.Add(-time.Minute),
		AuthorityKeyID: input.Authority.KeyID(), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return input.Authority.Sign(context.Background(), message) },
	})
	if err != nil {
		t.Fatal(err)
	}
	datasetIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          domainsecurity.SHA256Hex([]byte("production-concrete-dataset-index")),
		Binding:             resolved.Record.Binding, SnapshotRecordDigest: resolved.Record.RecordDigest,
		AuthorityKeyID: input.Authority.KeyID(), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return input.Authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                 domainsecurity.SHA256Hex([]byte("production-concrete-bundle-genesis")),
		DatasetSnapshotIndexDigest: datasetIndex.IndexDigest, DatasetSnapshotCount: 1,
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              input.Authority.KeyID(), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return input.Authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x78}, ed25519.SeedSize))
	coordinator := &productionConcreteCoordinator{
		bundle: bundle, authority: input.Authority, witnessPrivate: witnessPrivate,
		witnessPublic: witnessPrivate.Public().(ed25519.PublicKey),
		history:       map[string]evidenceauthorityport.ObservationBundle{},
	}
	head, err := coordinator.ObserveFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		Head: head, DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{datasetIndex},
		SelectedIndex: datasetIndex, Snapshot: resolved,
	}
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.ThreadID, TurnID: input.TurnID, WorkspaceRealPath: input.Observation.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: input.Observation.CaseID, CaseBindingHash: input.Observation.CaseBindingHash,
		DatasetSnapshotID: resolved.Record.DatasetSnapshotID, SourceManifestHash: resolved.Record.SourceManifestHash,
		ContextEpoch: 1, IssuedAt: input.Now, PublicationPolicy: input.PublicationPolicy,
		RiskAuthorityBinding: input.RiskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	dataset := &productionConcreteDatasetAuthority{
		coordinator: coordinator, observation: input.Observation, selection: selection,
	}
	registryCAS, err := evidenceregistryv2fixture.New(input.Root)
	if err != nil {
		t.Fatal(err)
	}
	stores := registryCAS.Stores()
	registry, err := evidenceregistryapp.New(evidenceregistryapp.Config{
		InstallationID: installationID, EnrollmentID: enrollmentID, Authority: input.Authority,
		WitnessKeyID: domainsecurity.SHA256Hex(coordinator.witnessPublic), WitnessKey: coordinator.witnessPublic,
		Coordinator: coordinator, WitnessChain: coordinator, Indexes: stores.Indexes, Capsules: stores.Capsules,
		DatasetAuthority: dataset, BindingObserver: dataset,
		Random: bytes.NewReader(bytes.Repeat([]byte{0x79}, 4096)),
		Now:    func() time.Time { return input.Now.Add(6 * time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt := commitProductionConcreteEvidence(t, registry, securityContext, input.Now)
	host := &productionConcreteHostAuthority{
		coordinator: coordinator, observation: input.Observation, selection: selection,
		checkedAt: input.Now.Add(5 * time.Minute),
	}
	return productionConcreteWitnessFixture{
		securityContext: securityContext, registry: registry, receipt: receipt,
		dataset: dataset, host: host, coordinator: coordinator,
	}
}

func commitProductionConcreteEvidence(
	t *testing.T,
	registry *evidenceregistryapp.Service,
	securityContext domainsecurity.TurnSecurityContext,
	now time.Time,
) domainevidence.EvidenceReceipt {
	t.Helper()
	payload := domainevidence.NormalizedClaimPayload{
		SubjectID: "entity-production", EntityID: "entity-production", AccountID: "account-masked-production",
		AmountMinor: "4200000", Currency: "CNY", Direction: "out",
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
	}
	canonicalBody, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-production-concrete", ClaimType: domainevidence.ClaimAmount, NormalizedPayload: payload,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(canonicalBody)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix-fund-analysis", "1.0.0",
		domainsecurity.SHA256Hex([]byte("production-concrete-funds-instance")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	settlementID := domainsecurity.SHA256Hex([]byte("production-concrete-settlement"))
	rawSHA := domainsecurity.SHA256Hex([]byte("production-concrete-private-raw"))
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(settlementID), Context: securityContext,
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("production-concrete-grant")),
		ToolCallID:       toolidentitytest.MustHostToolCallIDV1("production-concrete-fact-final"),
		ServerIdentity:   identity, ServerVersion: "1.0.0", ConnectionEpoch: 3,
		ToolName: "mcp__analytix_funds__query", ArgsHash: domainsecurity.SHA256Hex([]byte("production-concrete-args")),
		ResultHash: domainsecurity.CanonicalJSONHash(canonical), SourceType: "transactions",
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash:         domainsecurity.SHA256Hex([]byte("production-concrete-query")),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{payload.EntityID}, AccountIDs: []string{payload.AccountID},
			Directions: []string{payload.Direction}, StartAt: payload.StartAt, EndAt: payload.EndAt,
			SourceIDs: []string{"source-production"}, FiltersHash: domainsecurity.SHA256Hex([]byte("production-concrete-filters")),
		},
		Granularity: payload.Granularity, Currency: payload.Currency, Timezone: "Asia/Shanghai",
		PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs:        []string{"row-production"}, RawSHA256: rawSHA,
		TransformationLineage: []domainevidence.TransformationLineageStep{{
			StepID: "normalize-production", Transformer: "synthetic-fixture", TransformerVersion: "1.0.0",
			InputHash: rawSHA, OutputHash: domainsecurity.CanonicalJSONHash(canonical),
		}},
		PIIClassification: domainevidence.PIIMasked, IssuedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := registry.CommitPrepared(context.Background(), registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: canonical,
		SettlementProof: domainevidence.EvidenceSettlementProof{
			SettlementID: settlementID, PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("production-concrete-prepared")),
		},
		RegisteredAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

type productionConcreteDatasetAuthority struct {
	coordinator *productionConcreteCoordinator
	observation domainsecurity.CaseBindingObservationV1
	selection   datasetsnapshotport.CurrentSelectionV2
}

func (authority *productionConcreteDatasetAuthority) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if authority == nil || workspace != authority.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("production concrete binding is unavailable")
	}
	return authority.observation, nil
}

func (authority *productionConcreteDatasetAuthority) ReadCurrentBinding(workspace string) (domainsecurity.CaseBinding, error) {
	observation, err := authority.Observe(workspace)
	if err != nil {
		return domainsecurity.CaseBinding{}, err
	}
	return domainsecurity.CaseBinding{
		CaseID: observation.CaseID, WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseBindingHash: observation.CaseBindingHash,
	}, nil
}

func (authority *productionConcreteDatasetAuthority) ResolveWitnessedV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if authority == nil || ctx == nil || ctx.Err() != nil ||
		input.TenantID != domainsecurity.LocalTenantID || input.UserID != domainsecurity.LocalUserID ||
		input.Observation != authority.observation ||
		(input.ExpectedDatasetSnapshotID != "" && input.ExpectedDatasetSnapshotID != authority.selection.Snapshot.Record.DatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.New("production concrete dataset resolution is unavailable")
	}
	return authority.selection.Snapshot, nil
}

func (authority *productionConcreteDatasetAuthority) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if authority == nil || authority.coordinator == nil || ctx == nil || ctx.Err() != nil || callback == nil ||
		input.TenantID != securityContext.TenantID || input.UserID != securityContext.UserID ||
		input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID || input.Observation != authority.observation {
		return errors.New("production concrete current dataset authority is unavailable")
	}
	head, err := authority.coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	selection := authority.selection
	selection.Head = head
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		return err
	}
	capability := &productionConcreteDatasetCapability{
		active: true, ctx: ctx, coordinator: authority.coordinator,
		selection: selection, securityContext: securityContext,
	}
	defer func() { capability.active = false }()
	return callback(selection, capability)
}

type productionConcreteDatasetCapability struct {
	active          bool
	ctx             context.Context
	coordinator     *productionConcreteCoordinator
	selection       datasetsnapshotport.CurrentSelectionV2
	securityContext domainsecurity.TurnSecurityContext
}

func (capability *productionConcreteDatasetCapability) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if capability == nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil ||
		capability.coordinator == nil || use == nil || !reflect.DeepEqual(selection, capability.selection) ||
		!reflect.DeepEqual(securityContext, capability.securityContext) {
		return errors.New("production concrete dataset capability changed")
	}
	before, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !before.HasBundle || before.Bundle.RecordDigest != selection.Head.Bundle.RecordDigest {
		return errors.Join(errors.New("production concrete dataset authority changed before use"), err)
	}
	if err := use(capability.ctx); err != nil {
		return err
	}
	after, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !after.HasBundle || after.Bundle.RecordDigest != selection.Head.Bundle.RecordDigest {
		return errors.Join(errors.New("production concrete dataset authority changed after use"), err)
	}
	return nil
}

type productionConcreteCoordinator struct {
	mu             sync.Mutex
	bundle         domainevidence.EvidenceAuthorityBundleV1
	authority      finalauthorityport.Authority
	witnessPrivate ed25519.PrivateKey
	witnessPublic  ed25519.PublicKey
	history        map[string]evidenceauthorityport.ObservationBundle
	observeCalls   int
	advanceCalls   int
}

func (coordinator *productionConcreteCoordinator) ObserveFresh(context.Context) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.freshHeadLocked("observe")
}

func (coordinator *productionConcreteCoordinator) AdvanceEvidenceRegistry(
	_ context.Context,
	input evidenceauthorityport.RegistryAdvanceInput,
) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	previous := coordinator.bundle
	if input.ExpectedBundleDigest != previous.RecordDigest {
		return evidenceauthorityport.FreshHead{}, errors.New("production concrete witness CAS conflict")
	}
	coordinator.advanceCalls++
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID,
		Generation: previous.Generation + 1, PreviousBundleDigest: previous.RecordDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("production-concrete-mutation:" + input.NextIndexDigest)),
		DatasetSnapshotIndexDigest:  previous.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:        previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: input.NextIndexDigest,
		EvidenceRegistryCount:       previous.EvidenceRegistryCount + 1,
		PublicationIndexDigest:      previous.PublicationIndexDigest,
		PublicationCount:            previous.PublicationCount,
		AuthorityKeyID:              coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	return coordinator.freshHeadLocked("advance")
}

func (coordinator *productionConcreteCoordinator) freshHeadLocked(label string) (evidenceauthorityport.FreshHead, error) {
	coordinator.observeCalls++
	bundle := coordinator.bundle
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("production-concrete-previous-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("production-concrete-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("production-concrete-fence")), MutationID: bundle.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(coordinator.witnessPublic), WitnessPublicKey: coordinator.witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(coordinator.witnessPrivate, message), nil })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte(label + ":" + strconv.Itoa(coordinator.observeCalls) + ":" + bundle.RecordDigest)),
		AuthorityKeyID: coordinator.authority.KeyID(), AuthorityPublicKey: coordinator.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request, checkpoint,
		func(message []byte) ([]byte, error) { return ed25519.Sign(coordinator.witnessPrivate, message), nil },
	)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	head := evidenceauthorityport.FreshHead{
		HasBundle: true, Bundle: bundle, Request: request, Observation: observation,
	}
	coordinator.history[observation.ObservationDigest] = evidenceauthorityport.ObservationBundle{
		Bundle: bundle, Request: request, Observation: observation,
	}
	return head, nil
}

func (coordinator *productionConcreteCoordinator) ResolveWitnessBindingOnFreshChain(
	_ context.Context,
	binding domainevidence.EvidenceAuthorityWitnessBindingV1,
) (evidenceauthorityport.WitnessBindingChainResolution, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if domainevidence.ValidateEvidenceAuthorityWitnessBindingV1(binding) != nil {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.New("production concrete witness binding is invalid")
	}
	historical, found := coordinator.history[binding.ObservationDigest]
	if !found || historical.Bundle.RecordDigest != binding.BundleRecordDigest {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.New("production concrete historical witness is unavailable")
	}
	current, err := coordinator.freshHeadLocked("resolve-witness-binding")
	if err != nil || !current.HasBundle || current.Bundle.RecordDigest != historical.Bundle.RecordDigest {
		return evidenceauthorityport.WitnessBindingChainResolution{}, errors.Join(
			errors.New("production concrete witness binding is no longer current"), err,
		)
	}
	return evidenceauthorityport.WitnessBindingChainResolution{Current: current, Historical: historical}, nil
}

type productionConcreteHostAuthority struct {
	coordinator *productionConcreteCoordinator
	observation domainsecurity.CaseBindingObservationV1
	selection   datasetsnapshotport.CurrentSelectionV2
	checkedAt   time.Time
	lastErr     error
}

func (authority *productionConcreteHostAuthority) WithFreshPublicationSnapshotAuthority(
	ctx context.Context,
	input sourceprobeport.PublicationInput,
	callback func([]domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	if authority == nil || ctx == nil || ctx.Err() != nil || callback == nil || len(input.Requirements) != 1 ||
		input.Binding != authority.observation {
		return errors.New("production concrete host publication authority is unavailable")
	}
	requirement := input.Requirements[0]
	identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(requirement.ServerIdentity)
	if err != nil {
		return err
	}
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: requirement.ServerID, ServerIdentity: requirement.ServerIdentity,
		ConnectionEpoch:    requirement.ConnectionEpoch,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("production-concrete-catalog")),
		SpecFingerprint:    domainsecurity.SHA256Hex([]byte("production-concrete-spec")),
		ThreadID:           input.Context.ThreadID, TurnID: input.Context.TurnID,
		ContextEpoch: input.Context.ContextEpoch, ContextDigest: input.Context.ContextDigest,
		DatasetSnapshotID: input.Context.DatasetSnapshotID, CheckedAt: authority.checkedAt,
		Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: identity.ObservedName,
			ServerVersion: identity.ObservedVersion, CaseID: input.Context.CaseID,
			CaseBindingHash:   input.Context.CaseBindingHash,
			DatasetSnapshotID: input.Context.DatasetSnapshotID,
			Ready:             true, ReadOnly: true, CheckedAt: authority.checkedAt.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return err
	}
	head, err := authority.coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	selection := authority.selection
	selection.Head = head
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		return err
	}
	capability := &productionConcreteHostCapability{
		active: true, ctx: ctx, coordinator: authority.coordinator,
		securityContext: input.Context, probe: probe, selection: selection,
	}
	defer func() { capability.active = false }()
	authority.lastErr = callback([]domainsecurity.VerifiedSourceProbe{probe}, capability)
	return authority.lastErr
}

type productionConcreteHostCapability struct {
	active          bool
	ctx             context.Context
	coordinator     *productionConcreteCoordinator
	securityContext domainsecurity.TurnSecurityContext
	probe           domainsecurity.VerifiedSourceProbe
	selection       datasetsnapshotport.CurrentSelectionV2
}

func (capability *productionConcreteHostCapability) DatasetSelection() (datasetsnapshotport.CurrentSelectionV2, error) {
	if capability == nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errors.New("production concrete host capability is inactive")
	}
	return capability.selection, nil
}

func (capability *productionConcreteHostCapability) UseExact(
	securityContext domainsecurity.TurnSecurityContext,
	probe domainsecurity.VerifiedSourceProbe,
	selection datasetsnapshotport.CurrentSelectionV2,
	use func(context.Context) error,
) error {
	if capability == nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || use == nil ||
		!reflect.DeepEqual(securityContext, capability.securityContext) || !reflect.DeepEqual(probe, capability.probe) ||
		!reflect.DeepEqual(selection, capability.selection) {
		return errors.New("production concrete host capability exact binding changed")
	}
	current, err := capability.coordinator.ObserveFresh(capability.ctx)
	if err != nil || !current.HasBundle || current.Bundle != selection.Head.Bundle {
		return errors.Join(errors.New("production concrete host capability is stale"), err)
	}
	return use(capability.ctx)
}

var _ datasetsnapshotport.AuthorityV2 = (*productionConcreteDatasetAuthority)(nil)
var _ datasetsnapshotport.CurrentAuthorityV2 = (*productionConcreteDatasetAuthority)(nil)
var _ registryport.FactFinalWitnessIssuer = (*evidenceregistryapp.Service)(nil)
