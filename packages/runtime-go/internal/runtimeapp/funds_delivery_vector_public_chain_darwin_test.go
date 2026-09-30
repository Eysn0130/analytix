//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
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
	t.Logf("separately derived Go CNY source sha256=%s", sourceHash)
	var mu sync.Mutex
	calls, semantics := 0, 0
	correctSemantics := 0
	firstContextBytes, secondContextBytes := 0, 0
	firstRequestBytes, secondRequestBytes, firstInputBytes, secondInputBytes := 0, 0, 0, 0
	var firstRoles, secondRoles map[string]int
	secondHasCurrentClaim := false
	var firstClaimDigests []string
	model := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil || r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("unexpected loopback Provider request")
			w.WriteHeader(400)
			return
		}
		deliveryAssertPublic(t, body)
		mu.Lock()
		calls++
		call := calls
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
			var fields map[string]json.RawMessage
			if json.Unmarshal(body, &fields) != nil {
				t.Error("invalid complete Provider request")
			}
			inputBytes, err := json.Marshal(map[string]json.RawMessage{"messages": fields["messages"], "tools": fields["tools"]})
			if err != nil {
				t.Error("invalid complete model input")
			}
			roles := make(map[string]int)
			for _, message := range request.Messages {
				roles[message.Role]++
				if message.Role != "system" && message.Role != "developer" && message.Role != "user" && message.Role != "assistant" && message.Role != "tool" {
					t.Error("unknown Provider message role")
				}
				if !strings.Contains(message.Content, "<analytix_host_verified_case_entity_semantics>") {
					continue
				}
				if message.Role != "user" {
					t.Error("case context attached to an unexpected role")
				}
				if call == 1 {
					firstContextBytes += len(message.Content)
				} else {
					secondContextBytes += len(message.Content)
					_, tail, _ := strings.Cut(message.Content, "<analytix_host_verified_case_entity_semantics>")
					semanticBody, _, _ := strings.Cut(tail, "</analytix_host_verified_case_entity_semantics>")
					var semantic struct {
						Longitudinal caseentityapp.ProviderIngressLongitudinalStateV1 `json:"longitudinal"`
					}
					if json.Unmarshal([]byte(strings.TrimSpace(semanticBody)), &semantic) != nil {
						t.Error("invalid case semantic JSON")
					}
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
				data[0].TransactionCount == uint64(reference.transactionCount) && data[0].AggregateComplete &&
				data[0].EvidenceRowsComplete && data[0].EvidenceTransactionCount == uint64(reference.transactionCount)
			if !correct {
				t.Error("final serialized Provider facts differ from independent exact CSV arithmetic")
			}
			mu.Lock()
			semantics++
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
		if confirmed["sourceRowCount"] != float64(9) {
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
			b1AssertPreparedFlowValues(t, record, reference)
			nativePreparations = append(nativePreparations, record)
			if record.SecurityContext.ThreadID == threadID {
				b1AssertActualFinal(t, config.DataDir, record, final.AcceptedFinalDigest, true)
			} else if record.SecurityContext.ThreadID == secondID {
				b1AssertActualFinal(t, config.DataDir, record, secondFinal.AcceptedFinalDigest, true)
			} else {
				t.Fatal("native evidence came from another thread")
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
		t.Logf("two-thread offline: request_json_bytes A=%d B=%d; model_messages_tools_json_bytes A=%d B=%d; roles A=%v B=%v; context_message_bytes A=%d B=%d; provider_requests_with_native_semantics=%d; signed_native_preparations=%d; exact_native_semantic_requests=%d/%d; current A metadata in B=%t; SQL count unmeasured", aRequest, bRequest, aInput, bInput, aRoles, bRoles, aBytes, bBytes, beforeSemantics, len(nativePreparations), correct, beforeSemantics, bHasCurrentClaim)
		request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
		t.Log("fresh thread-detail hydration passed before reopen")
		driver.reopen()
		restored := request(http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
		restoredTurn := packagedSourceUnavailableHydrationTurnV1(restored, turnID)
		restoredFinal, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(restoredTurn["acceptedFinalView"])
		if err != nil || restoredFinal.AcceptedFinalDigest != final.AcceptedFinalDigest {
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
		t.Log("derived vector: real import -> DuckDB -> Go authority -> final Provider semantic -> Final Gate -> protected local display -> reopen passed")
		driver.assertNewProcess(threadID, turnID, final.AcceptedFinalDigest, rev14PrivateAccount)
		mu.Lock()
		unchanged = calls == beforeCalls
		mu.Unlock()
		if !unchanged {
			t.Fatal("fresh process recovery reissued Provider work")
		}
	})
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

func deliveryAssertPublic(t *testing.T, body []byte) {
	t.Helper()
	for _, sentinel := range []string{rev14PrivateAccount, "6222021234567891", "SYNTH_PII_", "SYNTH_UNTRUSTED_MEMO_", "SYNTH_COUNTERPARTY_", "SELECT ", "baseline.csv"} {
		if bytes.Contains(body, []byte(sentinel)) {
			t.Error("private vector content reached generic or Provider output")
		}
	}
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
