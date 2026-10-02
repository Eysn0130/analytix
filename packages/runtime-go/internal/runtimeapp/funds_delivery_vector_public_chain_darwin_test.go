//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	"analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Real Go import/DSV2/Host/native/Provider/Final Gate/local display/reopen. The
// native generation is bound to the selected input's sealed source identity;
// package admission and Provider are synthetic, so this is not installation QA.
func TestFundsDeliveryVectorProductionPublicChain(t *testing.T) {
	source := deliveryCNYCSV(t)
	sourceHash := domainsecurity.SHA256Hex(source)
	reference := b1ReferenceFlowFromCSVInWindow(t, source, "2026-09-")
	referenceRows := deliveryReferenceTransactions(t, source)
	t.Logf("separately derived Go CNY source sha256=%s", sourceHash)
	var mu sync.Mutex
	calls, semantics := 0, 0
	correctSemantics := 0
	firstContextBytes, secondContextBytes := 0, 0
	firstRequestBytes, secondRequestBytes, firstInputBytes, secondInputBytes := 0, 0, 0, 0
	var requestBodies [][]byte
	var requestInputBytes []int
	var requestBodyComplete []bool
	providerResults := make(map[int]domainnative.AccountFlowProviderSemanticResultV1)
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		requestSizes := make([]int, len(requestBodies))
		inputMeasured := make([]bool, len(requestBodies))
		allRequestBytes, allInputBytes := 0, 0
		for index, body := range requestBodies {
			requestSizes[index] = len(body)
			allRequestBytes += len(body)
			if requestInputBytes[index] >= 0 {
				inputMeasured[index] = true
				allInputBytes += requestInputBytes[index]
			}
		}
		if len(requestSizes) != calls || len(requestInputBytes) != calls || len(requestBodyComplete) != calls {
			t.Error("complete Provider dispatch capture is missing requests")
		}
		t.Logf("all received Provider dispatches=%d; body_complete=%v; request_json_bytes=%v total=%d; messages_tools_json_bytes=%v measured=%v measured_total=%d", calls, requestBodyComplete, requestSizes, allRequestBytes, requestInputBytes, inputMeasured, allInputBytes)
	}()
	var firstRoles, secondRoles map[string]int
	secondHasCurrentClaim := false
	var firstClaimDigests []string
	model := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
		mu.Lock()
		calls++
		call := calls
		requestBodies = append(requestBodies, bytes.Clone(body))
		requestInputBytes = append(requestInputBytes, -1)
		requestBodyComplete = append(requestBodyComplete, err == nil && len(body) <= 2<<20)
		mu.Unlock()
		t.Logf("received Provider dispatch=%d request_json_bytes=%d body_complete=%t", call, len(body), err == nil && len(body) <= 2<<20)
		if err != nil || len(body) > 2<<20 || r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("unexpected loopback Provider request")
			w.WriteHeader(400)
			return
		}
		deliveryAssertPublic(t, body)
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil {
			t.Error("invalid complete Provider request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		inputBytes, inputErr := json.Marshal(map[string]json.RawMessage{"messages": fields["messages"], "tools": fields["tools"]})
		if inputErr != nil {
			t.Error("invalid complete model input")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requestInputBytes[call-1] = len(inputBytes)
		t.Logf("normalized Provider dispatch=%d messages_tools_json_bytes=%d measured=true", call, len(inputBytes))
		if call == 1 || call == 3 {
			var request struct {
				Messages []struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				} `json:"messages"`
			}
			if json.Unmarshal(body, &request) != nil {
				t.Error("invalid two-thread Provider request")
			}
			roles := make(map[string]int)
			contextBlocks := 0
			for _, message := range request.Messages {
				roles[message.Role]++
				if message.Role != "system" && message.Role != "developer" && message.Role != "user" && message.Role != "assistant" && message.Role != "tool" {
					t.Error("unknown Provider message role")
				}
				semanticBody, found, blockErr := deliveryCaseSemanticBody(message.Content)
				if !found {
					continue
				}
				contextBlocks++
				if blockErr != nil {
					t.Error(blockErr)
					continue
				}
				if message.Role != "user" {
					t.Error("case context attached to an unexpected role")
				}
				var semantic struct {
					Entities []struct {
						Alias string `json:"alias"`
					} `json:"entities"`
					Longitudinal caseentityapp.ProviderIngressLongitudinalStateV1 `json:"longitudinal"`
				}
				if json.Unmarshal(semanticBody, &semantic) != nil || len(semantic.Entities) == 0 {
					t.Error("invalid case semantic JSON")
					continue
				}
				subjects := 0
				for _, entity := range semantic.Entities {
					if entity.Alias == "acct:1" {
						subjects++
					}
				}
				if subjects != 1 {
					t.Error("case semantic context did not bind exactly one selected subject")
				}
				if call == 1 {
					firstContextBytes += len(message.Content)
				} else {
					secondContextBytes += len(message.Content)
					matched := 0
					for _, digest := range firstClaimDigests {
						for _, item := range semantic.Longitudinal.Items {
							if item.Digest == digest && item.Kind == caseentityapp.ProviderIngressLongitudinalCurrentVerifiedFactV1 &&
								item.Currentness == "current" && item.InvestigationState == "confirmed" && item.EvidenceReferenceCount == 1 {
								matched++
							}
						}
					}
					secondHasCurrentClaim = len(firstClaimDigests) == 3 && matched == 3
				}
			}
			if contextBlocks != 1 {
				t.Error("Provider request did not contain exactly one final case semantic block")
			}
			if roles["user"] == 0 {
				t.Error("Provider request has no user input")
			}
			if call == 1 {
				firstRequestBytes, firstInputBytes, firstRoles = len(body), len(inputBytes), roles
			} else {
				secondRequestBytes, secondInputBytes, secondRoles = len(body), len(inputBytes), roles
			}
		}
		mu.Unlock()
		data := deliveryProviderSemantics(t, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(data) > 0 {
			correct := len(data) == 1 && data[0].InflowMinor == reference.inflowMinor && data[0].OutflowMinor == reference.outflowMinor &&
				data[0].NetMinor == reference.netMinor && data[0].Currency == reference.currency && data[0].MinorUnitScale == 2 &&
				data[0].SubjectAlias == "acct:1" && data[0].Timezone == "Z" &&
				data[0].StartInclusive == "2026-09-01T00:00:00.000000Z" && data[0].EndInclusive == "2026-09-30T23:59:59.999999Z" &&
				data[0].TransactionCount == uint64(reference.transactionCount) && data[0].AggregateComplete &&
				data[0].EvidenceRowsComplete && data[0].EvidenceTransactionCount == uint64(reference.transactionCount) &&
				len(data[0].Transactions) == reference.transactionCount && deliveryTransactionsMatch(referenceRows, data[0].Transactions)
			if !correct {
				t.Error("final serialized Provider facts differ from independent exact CSV arithmetic")
			}
			mu.Lock()
			semantics++
			if len(data) == 1 {
				providerResults[call] = data[0]
			}
			if correct {
				correctSemantics++
			}
			mu.Unlock()
			runtimeOptionalLifecycleModelResponseV1(w, "", nil, "合成账户流水分析完成。")
			return
		}
		if call > 4 {
			t.Error("unexpected repeat without native semantics")
			runtimeOptionalLifecycleModelResponseV1(w, "", nil, "停止合成验证。")
			return
		}
		runtimeOptionalLifecycleModelResponseV1(w, rev14AccountFlowToolName, map[string]any{"subject_alias": "acct:1", "start_inclusive": "2026-09-01T00:00:00.000000Z", "end_inclusive": "2026-09-30T23:59:59.999999Z", "evidence_row_limit": 100}, "")
	})
	runB1ProductionPublicChain(t, source, model, func(config Config, root, workspace, sourcePath, authorityID string, sourceEvents func() []domainplugincapability.FundsSourceReadDecisionEventV1, driver b1PublicChainDriver) {
		client := &http.Client{Timeout: 45 * time.Second}
		request := func(method, path string, body map[string]any, status int) map[string]any {
			t.Helper()
			return b1PublicChainRequest(t, client, driver.baseURL(), method, path, body, status)
		}
		staged := request(http.MethodPost, "/v1/local-display/funds-import/stage", map[string]any{"workspaceRoot": workspace, "sourcePath": sourcePath}, http.StatusOK)
		items, _ := staged["items"].([]any)
		if len(items) != 1 {
			t.Fatal("derived vector did not stage one actual source")
		}
		confirmed := request(http.MethodPost, "/v1/local-display/funds-import/confirm", map[string]any{"selector": items[0].(map[string]any)["selector"]}, http.StatusOK)
		importedRows, _ := confirmed["sourceRowCount"].(float64)
		if importedRows != 9 {
			t.Fatal("actual native importer lost source rows")
		}
		thread := request(http.MethodPost, "/v1/threads", map[string]any{"title": "Synthetic funds delivery", "workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
		threadID := contracts.StringField(thread, "id")
		started := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "分析银行账号 " + rev14PrivateAccount + " 在二〇二六年九月一日至九月三十日的收支。", "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
		turnID := contracts.StringField(started, "turnId")
		driver.waitTurn(threadID, turnID, "delivery-vector")
		turn := b1WaitTerminal(t, client, driver.baseURL(), threadID, turnID)
		public, _ := json.Marshal(turn)
		deliveryAssertPublic(t, public)
		final, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
		if err != nil || turn["status"] != "completed" || final.Variant != domainevidence.EvidenceBackedAnswer {
			t.Fatalf("derived vector terminal did not bind actual native evidence: status=%v variant=%v blocker=%v", turn["status"], final.Variant, final.BlockerCode)
		}
		prepared := b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared"))
		if len(prepared) != 1 {
			t.Fatal("expected one actual prepared native evidence settlement")
		}
		for _, body := range prepared {
			record, err := domainevidence.ParsePreparedEvidenceSettlement(body)
			if err != nil || record.AuthorityKeyID != authorityID || record.HostAuthority == nil {
				t.Fatal("missing signed native preparation")
			}
		}
		display := func() {
			out := request(http.MethodPost, "/v1/local-display/accepted-slot-display", map[string]any{"kind": "accepted_slot_display", "threadId": threadID, "turnId": turnID, "acceptedFinalDigest": final.AcceptedFinalDigest, "displayMode": "full"}, http.StatusOK)
			slots, _ := out["slots"].([]any)
			if len(slots) != 1 || slots[0].(map[string]any)["displayValue"] != rev14PrivateAccount {
				t.Fatal("protected local display did not retain exact derived source account")
			}
		}
		mu.Lock()
		beforeCalls := calls
		beforeSemantics := semantics
		mu.Unlock()
		if beforeCalls != 2 || beforeSemantics != 1 {
			t.Fatalf("actual Provider sequence calls=%d semantics=%d", beforeCalls, beforeSemantics)
		}
		display()
		for _, body := range b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "accepted-finals", "records")) {
			record, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
			if err != nil {
				t.Fatal(err)
			}
			if record.SecurityContext.ThreadID == threadID && record.SecurityContext.TurnID == turnID {
				mu.Lock()
				for _, claim := range record.Envelope.Claims {
					firstClaimDigests = append(firstClaimDigests, claim.RecordDigest)
				}
				mu.Unlock()
			}
		}
		// Independent B selects the same account in the same project. Its first
		// request must receive A's typed currentness/evidence metadata, while its
		// numerical answer still requires a fresh authorized native query.
		secondThread := request(http.MethodPost, "/v1/threads", map[string]any{"title": "Synthetic longitudinal B", "workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
		secondID := contracts.StringField(secondThread, "id")
		secondStart := request(http.MethodPost, "/v1/threads/"+secondID+"/turns", map[string]any{"prompt": "继续核对银行账号 " + rev14PrivateAccount + " 在二〇二六年九月的收支。", "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
		secondTurnID := contracts.StringField(secondStart, "turnId")
		driver.waitTurn(secondID, secondTurnID, "longitudinal-b")
		secondTurn := b1WaitTerminal(t, client, driver.baseURL(), secondID, secondTurnID)
		secondFinal, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(secondTurn["acceptedFinalView"])
		if err != nil || secondTurn["status"] != "completed" || secondFinal.Variant != domainevidence.EvidenceBackedAnswer || secondFinal.ReceiptMetadata.Count != 1 {
			t.Fatal("independent B did not finalize its current native evidence")
		}
		mu.Lock()
		beforeCalls, beforeSemantics = calls, semantics
		bHasCurrentClaim, aBytes, bBytes := secondHasCurrentClaim, firstContextBytes, secondContextBytes
		aRequest, bRequest, aInput, bInput, correct := firstRequestBytes, secondRequestBytes, firstInputBytes, secondInputBytes, correctSemantics
		aRoles, bRoles := firstRoles, secondRoles
		mu.Unlock()
		if secondTurn["factHistoryState"] != nil {
			t.Fatal("new B final was not current before OwnerClose")
		}
		if beforeCalls != 4 || beforeSemantics != 2 {
			t.Fatalf("two-thread Provider sequence calls=%d semantics=%d", beforeCalls, beforeSemantics)
		}
		if !bHasCurrentClaim {
			t.Error("independent raw B request omitted A's current verified claim and evidence binding")
		}
		if aBytes == 0 || bBytes == 0 {
			t.Error("two-thread serialized Provider context measurement is missing")
		}
		var nativePreparations []domainevidence.PreparedEvidenceSettlement
		for _, body := range b1PrivateCASRecords(t, filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared")) {
			record, err := domainevidence.ParsePreparedEvidenceSettlement(body)
			if err != nil {
				t.Fatal(err)
			}
			b1AssertPreparedFlowValuesWithBindings(t, record, reference, reference.transactionCount)
			nativePreparations = append(nativePreparations, record)
			providerCall := 0
			if record.SecurityContext.ThreadID == threadID {
				providerCall = 2
				b1AssertActualFinal(t, config.DataDir, record, final.AcceptedFinalDigest, true)
			} else if record.SecurityContext.ThreadID == secondID {
				providerCall = 4
				b1AssertActualFinal(t, config.DataDir, record, secondFinal.AcceptedFinalDigest, true)
			} else {
				t.Fatal("native evidence came from another thread")
			}
			mu.Lock()
			semantic, observed := providerResults[providerCall]
			mu.Unlock()
			if !observed || semantic.QueryHash != record.QueryHash || len(record.ReceiptDraft.TransformationLineage) != 2 ||
				semantic.ResultHash != record.ReceiptDraft.TransformationLineage[0].OutputHash {
				t.Fatal("Provider semantics did not bind their own thread's signed native query and result")
			}
			material, err := domainevidence.ParseCanonicalEvidenceMaterial(record.CanonicalEvidence)
			if err != nil || len(record.ReceiptDraft.SourceRecordIDs) != reference.transactionCount {
				t.Fatal("native receipt did not bind the complete selected row set")
			}
			semanticRows := deliveryEvidenceRows(semantic.Transactions)
			boundRows := make(map[string]bool)
			entity := material.AcceptedSlotSourceBindings[0].EntityReference
			fileID := material.AcceptedSlotSourceBindings[0].SourceFileID
			for _, binding := range material.AcceptedSlotSourceBindings {
				if _, exists := semanticRows[binding.SourceRecordID]; !exists || boundRows[binding.SourceRecordID] ||
					binding.EntityReference != entity || binding.SourceFileID != fileID || binding.SourceRowNumber == 0 ||
					binding.Field != domainevidence.AcceptedSlotSourceFieldAccountV1 || len(binding.FactIDs) != 3 {
					t.Fatal("native row lineage did not match the selected subject and exact Provider evidence references")
				}
				boundRows[binding.SourceRecordID] = true
			}
			receiptRows := make(map[string]bool)
			for _, ref := range record.ReceiptDraft.SourceRecordIDs {
				if !boundRows[ref] || receiptRows[ref] {
					t.Fatal("native receipt and canonical row lineage selected different source records")
				}
				receiptRows[ref] = true
			}
		}
		if len(nativePreparations) != 2 || nativePreparations[0].SecurityContext.ThreadID == nativePreparations[1].SecurityContext.ThreadID ||
			nativePreparations[0].SecurityContext.DatasetSnapshotID != nativePreparations[1].SecurityContext.DatasetSnapshotID ||
			nativePreparations[0].SecurityContext.CaseBindingHash != nativePreparations[1].SecurityContext.CaseBindingHash ||
			nativePreparations[0].ReceiptID == nativePreparations[1].ReceiptID {
			t.Fatal("A/B evidence scopes or receipts were rebound")
		}
		firstMaterial, _ := domainevidence.ParseCanonicalEvidenceMaterial(nativePreparations[0].CanonicalEvidence)
		secondMaterial, _ := domainevidence.ParseCanonicalEvidenceMaterial(nativePreparations[1].CanonicalEvidence)
		if firstMaterial.AcceptedSlotSourceBindings[0].EntityReference != secondMaterial.AcceptedSlotSourceBindings[0].EntityReference {
			t.Fatal("A/B selected different source entities")
		}
		secondBindings := make(map[string]domainevidence.AcceptedSlotSourceBindingV1)
		for _, binding := range secondMaterial.AcceptedSlotSourceBindings {
			secondBindings[binding.SourceRecordID] = binding
		}
		for _, binding := range firstMaterial.AcceptedSlotSourceBindings {
			other, found := secondBindings[binding.SourceRecordID]
			if !found || binding.EntityReference != other.EntityReference || binding.SourceFileID != other.SourceFileID ||
				binding.SourceRowNumber != other.SourceRowNumber || binding.Field != other.Field {
				t.Fatal("same immutable snapshot rebound a canonical source-row locator in B")
			}
		}
		mu.Lock()
		aRows := deliveryEvidenceRows(providerResults[2].Transactions)
		bRows := deliveryEvidenceRows(providerResults[4].Transactions)
		mu.Unlock()
		if len(aRows) != reference.transactionCount || len(bRows) != len(aRows) {
			t.Fatal("A/B evidence reference cardinality differs from the independently selected rows")
		}
		for ref, row := range aRows {
			if bRows[ref] != row {
				t.Fatal("same immutable snapshot rebound a transaction evidence reference or its values in B")
			}
		}
		t.Logf("two-thread offline: request_json_bytes A=%d B=%d; model_messages_tools_json_bytes A=%d B=%d; roles A=%v B=%v; context_message_bytes A=%d B=%d; provider_requests_with_native_semantics=%d; signed_native_preparations=%d; exact_native_semantic_requests=%d/%d; current A metadata in B=%t; SQL count unmeasured", aRequest, bRequest, aInput, bInput, aRoles, bRoles, aBytes, bBytes, beforeSemantics, len(nativePreparations), correct, beforeSemantics, bHasCurrentClaim)
		t.Logf("independent CNY reference inflow_minor=%s outflow_minor=%s net_minor=%s transactions=%d; observed native imported_rows=%.0f", reference.inflowMinor, reference.outflowMinor, reference.netMinor, reference.transactionCount, importedRows)
		request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
		t.Log("fresh thread-detail hydration passed before reopen")
		driver.reopen()
		restored := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
		restoredTurn := packagedSourceUnavailableHydrationTurnV1(restored, turnID)
		restoredFinal, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(restoredTurn["acceptedFinalView"])
		if err != nil || restoredFinal.AcceptedFinalDigest != final.AcceptedFinalDigest || restoredTurn["factHistoryState"] != nil {
			t.Fatal("reopen lost committed final")
		}
		restoredBody, _ := json.Marshal(restored)
		deliveryAssertPublic(t, restoredBody)
		display()
		mu.Lock()
		unchanged := calls == beforeCalls
		mu.Unlock()
		if !unchanged {
			t.Fatal("display/reopen reissued Provider work")
		}
		currentSource, err := os.ReadFile(sourcePath)
		if err != nil || domainsecurity.SHA256Hex(currentSource) != sourceHash {
			t.Fatal("analysis changed original CSV")
		}
		if sourceEvents == nil || len(sourceEvents()) == 0 {
			t.Fatal("actual Host source lifecycle was not observed")
		}
		// Close the actual admitted Owner, then try to reuse the prior case and
		// facts. This is native-source unavailability, not persisted DSV2 revoke.
		preparedRoot := filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared")
		preparedBeforeFailure := startupWholeTreeDigest(t, preparedRoot)
		if driver.closeNativeSource == nil || driver.closeNativeSource() != nil {
			t.Fatal("actual native-source failure prerequisite could not be established")
		}
		blockedStart := request(http.MethodPost, "/v1/threads/"+secondID+"/turns", map[string]any{"prompt": "继续核对银行账号 " + rev14PrivateAccount + " 在二〇二六年九月的收支。", "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
		blockedTurnID := contracts.StringField(blockedStart, "turnId")
		driver.waitTurn(secondID, blockedTurnID, "native-source-unavailable")
		publicHydrationStarted := time.Now()
		t.Log("post-Owner-close public terminal hydration started")
		b1WaitTerminal(t, client, driver.baseURL(), secondID, blockedTurnID)
		t.Logf("post-Owner-close public terminal hydration_ms=%d", time.Since(publicHydrationStarted).Milliseconds())
		blockedDetail := request(http.MethodGet, "/v1/threads/"+secondID, nil, http.StatusOK)
		blockedTurn := packagedSourceUnavailableHydrationTurnV1(blockedDetail, blockedTurnID)
		originalBContext := runtimeWitnessedRegistryFrozenContextV2(t, config.ProductionDurableRoot, secondID, secondTurnID)
		blockedBContext := runtimeWitnessedRegistryFrozenContextV2(t, config.ProductionDurableRoot, secondID, blockedTurnID)
		b1AssertOwnerCloseRetainedScope(t, originalBContext, blockedBContext)
		retainedB := b1RecoveryExpectedFinal{ThreadID: secondID, TurnID: secondTurnID, FinalDigest: secondFinal.AcceptedFinalDigest, HistoryState: "retained_snapshot"}
		if b1RecoveryFinalReadbackFailure(retainedB, blockedDetail) != "" {
			t.Fatal("OwnerClose did not preserve B's exact original final as retained history")
		}

		blockedBody, err := json.Marshal(blockedTurn)
		if err != nil {
			t.Fatal("native-source failure terminal could not be serialized")
		}
		deliveryAssertPublic(t, blockedBody)
		if blockedTurn["status"] != "completed" || !bytes.Contains(blockedBody, []byte(domainevidence.CaseSourceUnavailableText)) {
			t.Fatal("native-source failure did not complete with the expected Host-owned boundary")
		}
		if view := blockedTurn["acceptedFinalView"]; view != nil {
			blockedFinal, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(view)
			if err != nil || blockedFinal.Variant == domainevidence.EvidenceBackedAnswer || blockedFinal.Variant == domainevidence.PartialEvidenceAnswer {
				t.Fatal("unavailable native source returned a malformed or authoritative numeric final")
			}
		}
		mu.Lock()
		unchanged = calls == beforeCalls
		mu.Unlock()
		if !unchanged || startupWholeTreeDigest(t, preparedRoot) != preparedBeforeFailure {
			t.Fatal("unavailable native source issued Provider work or new evidence")
		}
		t.Log("actual native Owner closed: prior-fact retry produced zero Provider dispatches and zero new preparations; persisted DSV2 revocation remains unmeasured")
		if !t.Failed() {
			t.Log("derived vector: real import -> DuckDB -> Go authority -> final Provider semantic -> Final Gate -> protected local display -> reopen passed")
		}
		driver.assertNewProcess(rev14PrivateAccount,
			b1RecoveryExpectedFinal{ThreadID: threadID, TurnID: turnID, FinalDigest: final.AcceptedFinalDigest},
			retainedB,
		)
		mu.Lock()
		unchanged = calls == beforeCalls
		mu.Unlock()
		if !unchanged {
			t.Fatal("fresh process recovery reissued Provider work")
		}
	})
}

// Original finals keep their frozen identity; a new Host policy can advance
// the same source's epoch. Eligibility is not admission: public readback and
// fresh-process exact-ref display still perform their real authorization checks.
func b1AssertOwnerCloseRetainedScope(t *testing.T, original, current domainsecurity.TurnSecurityContext) {
	t.Helper()
	if current.ThreadID != original.ThreadID || current.WorkspaceRealPath != original.WorkspaceRealPath ||
		current.TenantID != original.TenantID || current.UserID != original.UserID || current.CaseID != original.CaseID ||
		current.CaseBindingHash != original.CaseBindingHash || current.DatasetSnapshotID != original.DatasetSnapshotID ||
		current.SourceManifestHash != original.SourceManifestHash || current.ContextEpoch <= original.ContextEpoch ||
		!domainsecurity.TurnSecurityContextUsesHostRiskPolicy(original) || !domainsecurity.TurnSecurityContextUsesHostRiskPolicy(current) ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(current) ||
		current.PublicationPolicy.ThreadRiskPolicyDigest == original.PublicationPolicy.ThreadRiskPolicyDigest {
		t.Fatal("same-source authorized higher-epoch history scope was not established")
	}
	t.Logf("native history scope=original-principal-workspace-case-preserved same_snapshot=true same_manifest=true host_policy_advanced=true original_epoch=%d current_epoch=%d", original.ContextEpoch, current.ContextEpoch)
}

// Single-thread causal diagnostic: the original current expectation is
// compared before/after a real SourceUnavailable turn.
// This does not substitute for the complete A/B fresh-process acceptance chain.
func TestFundsDeliveryOwnerCloseHistoryReadback(t *testing.T) {
	source := deliveryCNYCSV(t)
	reference := b1ReferenceFlowFromCSVInWindow(t, source, "2026-09-")
	rows := deliveryReferenceTransactions(t, source)
	var mu sync.Mutex
	calls, semantics := 0, 0
	model := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if err != nil || len(body) > 2<<20 || r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("unexpected loopback Provider request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		deliveryAssertPublic(t, body)
		w.Header().Set("Content-Type", "text/event-stream")
		data := deliveryProviderSemantics(t, body)
		if len(data) != 0 {
			if len(data) != 1 || data[0].InflowMinor != reference.inflowMinor || data[0].OutflowMinor != reference.outflowMinor || data[0].NetMinor != reference.netMinor || !deliveryTransactionsMatch(rows, data[0].Transactions) {
				t.Error("native diagnostic differs from independent CSV truth")
			}
			mu.Lock()
			semantics++
			mu.Unlock()
			runtimeOptionalLifecycleModelResponseV1(w, "", nil, "合成账户流水分析完成。")
			return
		}
		if call != 1 {
			t.Error("unexpected repeated native request")
		}
		runtimeOptionalLifecycleModelResponseV1(w, rev14AccountFlowToolName, map[string]any{"subject_alias": "acct:1", "start_inclusive": "2026-09-01T00:00:00.000000Z", "end_inclusive": "2026-09-30T23:59:59.999999Z", "evidence_row_limit": 100}, "")
	})
	runB1ProductionPublicChain(t, source, model, func(config Config, root, workspace, sourcePath, authorityID string, sourceEvents func() []domainplugincapability.FundsSourceReadDecisionEventV1, driver b1PublicChainDriver) {
		client := &http.Client{Timeout: 45 * time.Second}
		request := func(method, path string, body map[string]any, status int) map[string]any {
			return b1PublicChainRequest(t, client, driver.baseURL(), method, path, body, status)
		}
		staged := request(http.MethodPost, "/v1/local-display/funds-import/stage", map[string]any{"workspaceRoot": workspace, "sourcePath": sourcePath}, http.StatusOK)
		items, _ := staged["items"].([]any)
		if len(items) != 1 {
			t.Fatal("diagnostic did not stage one source")
		}
		confirmed := request(http.MethodPost, "/v1/local-display/funds-import/confirm", map[string]any{"selector": items[0].(map[string]any)["selector"]}, http.StatusOK)
		if confirmed["sourceRowCount"] != float64(9) {
			t.Fatal("diagnostic import lost rows")
		}
		thread := request(http.MethodPost, "/v1/threads", map[string]any{"title": "Synthetic source-unavailable history", "workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
		threadID := contracts.StringField(thread, "id")
		prompt := "分析银行账号 " + rev14PrivateAccount + " 在二〇二六年九月一日至九月三十日的收支。"
		start := func() string {
			started := request(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": prompt, "mode": "agent", "async": true, "approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
			turnID := contracts.StringField(started, "turnId")
			driver.waitTurn(threadID, turnID, "owner-close-history")
			return turnID
		}
		turnID := start()
		turn := b1WaitTerminal(t, client, driver.baseURL(), threadID, turnID)
		final, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
		if err != nil || turn["status"] != "completed" || final.Variant != domainevidence.EvidenceBackedAnswer {
			t.Fatal("diagnostic did not acquire a numeric final")
		}
		expected := b1RecoveryExpectedFinal{ThreadID: threadID, TurnID: turnID, FinalDigest: final.AcceptedFinalDigest}
		if b1RecoveryFinalReadbackFailure(expected, request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)) != "" {
			t.Fatal("pre-failure current final was invalid")
		}
		// Read the private durable contexts locally; never serialize them into logs.
		readContext := func(turnID string) domainsecurity.TurnSecurityContext {
			return runtimeWitnessedRegistryFrozenContextV2(t, config.ProductionDurableRoot, threadID, turnID)
		}
		original := readContext(turnID)
		preparedRoot := filepath.Join(config.DataDir, "private", "evidence-settlements", "prepared")
		beforePrepared := startupWholeTreeDigest(t, preparedRoot)
		mu.Lock()
		beforeCalls, beforeSemantics := calls, semantics
		mu.Unlock()
		if beforeCalls != 2 || beforeSemantics != 1 {
			t.Fatal("diagnostic did not use one fresh native query")
		}
		if driver.closeNativeSource == nil || driver.closeNativeSource() != nil {
			t.Fatal("diagnostic actual OwnerClose failed")
		}
		blockedID := start()
		blockedTurn := b1WaitTerminal(t, client, driver.baseURL(), threadID, blockedID)
		blockedBody, err := json.Marshal(blockedTurn)
		if err != nil {
			t.Fatal("diagnostic terminal could not be serialized")
		}
		deliveryAssertPublic(t, blockedBody)
		if blockedTurn["status"] != "completed" || !bytes.Contains(blockedBody, []byte(domainevidence.CaseSourceUnavailableText)) {
			t.Fatal("diagnostic did not complete SourceUnavailable")
		}
		if value := blockedTurn["acceptedFinalView"]; value != nil {
			view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(value)
			if err != nil || view.Variant == domainevidence.EvidenceBackedAnswer || view.Variant == domainevidence.PartialEvidenceAnswer {
				t.Fatal("unavailable source generated authoritative numeric final")
			}
		}
		current := readContext(blockedID)
		b1AssertOwnerCloseRetainedScope(t, original, current)
		mu.Lock()
		unchanged := calls == beforeCalls && semantics == beforeSemantics
		mu.Unlock()
		if !unchanged || startupWholeTreeDigest(t, preparedRoot) != beforePrepared {
			t.Fatal("OwnerClose reissued Provider or evidence work")
		}
		checkHistory := func(ordinal int) {
			detail := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
			public, err := json.Marshal(detail)
			if err != nil {
				t.Fatal("diagnostic history could not be serialized")
			}
			deliveryAssertPublic(t, public)
			failure := b1RecoveryFinalReadbackFailure(expected, detail)
			t.Logf("native diagnostic readback_failure=%s ordinal=%d", failure, ordinal)
			if failure != "final-history-label-mismatch" || packagedSourceUnavailableHydrationTurnV1(detail, turnID)["factHistoryState"] != "retained_snapshot" {
				t.Fatal("same-source higher-epoch retained-label hypothesis was falsified")
			}
		}
		checkHistory(1)
		driver.reopen()
		if readContext(turnID) != original || readContext(blockedID) != current {
			t.Fatal("reopen changed original frozen contexts")
		}
		checkHistory(2)
	})
}

// Static system instructions mention the tag inline. An actual Host block
// occupies its own lines, is unique, and closes the message with valid JSON.
// Recognition precedes the caller's user-role check.
func deliveryCaseSemanticBody(content string) ([]byte, bool, error) {
	const start = "<analytix_host_verified_case_entity_semantics>"
	const end = "</analytix_host_verified_case_entity_semantics>"
	if !strings.Contains("\n"+content, "\n"+start+"\n") {
		return nil, false, nil
	}
	_, tail, _ := strings.Cut(content, start)
	body, suffix, closed := strings.Cut(tail, end)
	canonical := []byte(strings.TrimSpace(body))
	if strings.Count(content, start) != 1 || strings.Count(content, end) != 1 || !closed ||
		strings.TrimSpace(suffix) != "" || !json.Valid(canonical) {
		return nil, true, errors.New("case semantic context is not one final JSON block")
	}
	return canonical, true, nil
}

func TestFundsDeliveryCaseSemanticBlockRecognition(t *testing.T) {
	const start = "<analytix_host_verified_case_entity_semantics>"
	const end = "</analytix_host_verified_case_entity_semantics>"
	const body = `{"entities":[{"alias":"acct:1"}]}`
	valid := "request\n\n" + start + "\n" + body + "\n" + end
	for _, test := range []struct {
		name, content  string
		found, invalid bool
	}{
		{"system instruction", "A trusted semantic record is only the final " + start + " block appended to a provider user message.", false, false},
		{"final block", valid, true, false},
		{"duplicate block", valid + "\n" + start + "\n" + body + "\n" + end, true, true},
		{"trailing text", valid + "\nuntrusted tail", true, true},
		{"invalid JSON", start + "\nnot JSON\n" + end, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, found, err := deliveryCaseSemanticBody(test.content)
			if found != test.found || (err != nil) != test.invalid || (found && err == nil && string(got) != body) {
				t.Fatal("case semantic block classification changed")
			}
		})
	}
}

type deliveryTransactionValue struct {
	occurredAt, direction, amountMinor, currency string
	minorUnitScale                               uint8
}

// Compare every selected row, including multiplicity, independently of DuckDB
// and the model. This checks public transaction values and stable opaque refs;
// it does not substitute for an authenticated DSV2 material-graph resolution.
func deliveryReferenceTransactions(t *testing.T, source []byte) map[deliveryTransactionValue]int {
	t.Helper()
	rows, err := csv.NewReader(bytes.NewReader(source)).ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatal("invalid independent transaction CSV")
	}
	columns := make(map[string]int)
	for index, name := range rows[0] {
		columns[name] = index
	}
	for _, name := range []string{"交易账号", "交易时间", "交易金额", "收付标志", "交易币种"} {
		if _, ok := columns[name]; !ok {
			t.Fatal("independent transaction CSV is missing a column")
		}
	}
	values := make(map[deliveryTransactionValue]int)
	for _, row := range rows[1:] {
		if row[columns["交易账号"]] != rev14PrivateAccount || !strings.HasPrefix(row[columns["交易时间"]], "2026-09-") {
			continue
		}
		instant, err := time.Parse("2006-01-02 15:04:05.999999", row[columns["交易时间"]])
		minor, valid := b1ReferenceMinorUnits(row[columns["交易金额"]])
		if err != nil || !valid || row[columns["交易币种"]] != "CNY" {
			t.Fatal("invalid independently selected transaction")
		}
		direction := domainnative.AccountFlowDirectionInflowV1
		if row[columns["收付标志"]] == "出" {
			direction = domainnative.AccountFlowDirectionOutflowV1
		} else if row[columns["收付标志"]] != "进" {
			t.Fatal("unknown independent transaction direction")
		}
		// Public flow rows carry direction plus exact magnitude; the imported
		// source's signed decimal remains separate from that public contract.
		values[deliveryTransactionValue{instant.Format(time.RFC3339Nano), direction, minor.Abs(minor).String(), "CNY", 2}]++
	}
	return values
}

func deliveryTransactionsMatch(expected map[deliveryTransactionValue]int, rows []domainnative.AccountFlowProviderSemanticTransactionV1) bool {
	observed := make(map[deliveryTransactionValue]int)
	refs := make(map[string]bool)
	for _, row := range rows {
		instant, err := time.Parse(time.RFC3339Nano, row.OccurredAt)
		if err != nil || !strings.HasPrefix(row.EvidenceRef, "srow1_") || !domainsecurity.IsSHA256Hex(strings.TrimPrefix(row.EvidenceRef, "srow1_")) || refs[row.EvidenceRef] {
			return false
		}
		refs[row.EvidenceRef] = true
		observed[deliveryTransactionValue{instant.UTC().Format(time.RFC3339Nano), row.Direction, row.AmountMinor, row.Currency, row.MinorUnitScale}]++
	}
	if len(expected) != len(observed) {
		return false
	}
	for value, count := range expected {
		if observed[value] != count {
			return false
		}
	}
	return true
}

func deliveryEvidenceRows(rows []domainnative.AccountFlowProviderSemanticTransactionV1) map[string]deliveryTransactionValue {
	values := make(map[string]deliveryTransactionValue, len(rows))
	for _, row := range rows {
		values[row.EvidenceRef] = deliveryTransactionValue{row.OccurredAt, row.Direction, row.AmountMinor, row.Currency, row.MinorUnitScale}
	}
	return values
}

func deliveryCNYCSV(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("../../../../tools/analysis_compute/tests/fixtures/core-funds-20260921/funds-facts.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if domainsecurity.SHA256Hex(body) != "6894ac103a21260de3260aba39a49c15749ee2ec1db52842f5764abc6fc3cc82" {
		t.Fatal("original vector changed")
	}
	header, err := csv.NewReader(bytes.NewReader(rev14AccountFlowCSV())).Read()
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.UseCRLF = true
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var record map[string]any
		if json.Unmarshal(line, &record) != nil {
			t.Fatal("invalid vector")
		}
		field := func(key string) string { value, _ := record[key].(string); return value }
		if field("case_id") != "SYNTH_CASE_A" || field("snapshot_id") != "SYNTH_SNAPSHOT_A1" || field("currency") != "CNY" || field("row_id") == "A011" || field("row_id") == "A001_DUP" {
			continue
		}
		row := make([]string, len(header))
		row[1] = rev14PrivateAccount
		if field("subject_key") != "SYNTH_SUBJECT_1" {
			row[1] = "6222021234567891"
		}
		row[2] = field("raw_subject_name")
		row[4] = strings.ReplaceAll(strings.TrimSuffix(field("recorded_at"), "Z"), "T", " ")
		row[5] = field("amount")
		if strings.Contains(row[5], ".") {
			row[5] = strings.TrimSuffix(strings.TrimRight(row[5], "0"), ".")
		}
		row[7] = "进"
		if field("direction") == "out" {
			row[7] = "出"
		}
		row[8] = field("counterparty_key")
		row[10] = field("raw_counterparty_name")
		row[14] = "CNY"
		row[24] = field("source_record_id")
		row[31] = field("raw_memo")
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		t.Fatal(writer.Error())
	}
	return buffer.Bytes()
}

func deliveryAssertPublic(t *testing.T, body []byte) bool {
	t.Helper()
	clean := true
	for _, sentinel := range []string{rev14PrivateAccount, "6222021234567891", "SYNTH_PII_", "SYNTH_UNTRUSTED_MEMO_", "SYNTH_COUNTERPARTY_", "SELECT ", "baseline.csv"} {
		if bytes.Contains(body, []byte(sentinel)) {
			clean = false
			t.Error("private vector content reached generic or Provider output")
		}
	}
	return clean
}

func deliveryProviderSemantics(t *testing.T, body []byte) []domainnative.AccountFlowProviderSemanticResultV1 {
	t.Helper()
	var root any
	if json.Unmarshal(body, &root) != nil {
		t.Error("invalid serialized Provider JSON")
		return nil
	}
	var found []domainnative.AccountFlowProviderSemanticResultV1
	var visit func(any)
	visit = func(value any) {
		switch value := value.(type) {
		case string:
			var decoded any
			if json.Unmarshal([]byte(value), &decoded) == nil {
				visit(decoded)
			}
		case []any:
			for _, child := range value {
				visit(child)
			}
		case map[string]any:
			if value["purpose"] == domainnative.AccountFlowProviderModelPurposeV1 {
				canonical, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(value)
				if err != nil {
					t.Error("invalid typed Provider semantics")
					return
				}
				var output domainnative.AccountFlowProviderModelOutputV1
				if json.Unmarshal(canonical, &output) != nil {
					t.Error("invalid canonical Provider semantics")
					return
				}
				found = append(found, output.Data)
				return
			}
			for _, child := range value {
				visit(child)
			}
		}
	}
	visit(root)
	return found
}

func TestFundsDeliveryDiagnosticKeepsFiniteThreadFailureCodes(t *testing.T) {
	for _, code := range []string{"public_projection_pending", "accepted_final_hydration_unavailable"} {
		got := b1PublicChainRequestFailure(http.MethodGet, "/v1/threads/private-synthetic-identity", 503, 200, map[string]any{"code": code, "message": "SYNTH_PRIVATE_ERROR"})
		if !strings.HasSuffix(got, "code="+code) || strings.Contains(got, "private-synthetic-identity") || strings.Contains(got, "SYNTH_PRIVATE_ERROR") {
			t.Fatal("finite thread failure code was hidden or private content was exposed")
		}
	}
	got := b1PublicChainRequestFailure(http.MethodGet, "/v1/threads/private-synthetic-identity", 503, 200, map[string]any{"code": "SYNTH_PRIVATE_ERROR"})
	if !strings.HasSuffix(got, "code=unavailable") {
		t.Fatal("unknown error code escaped diagnostics")
	}
}
