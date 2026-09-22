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

	"analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Real Go import/DSV2/Host/native/Provider/Final Gate/local display/reopen. The
// native generation is the explicitly admitted historical one used by B1;
// package admission and Provider are synthetic, so this is not installation QA.
func TestFundsDeliveryVectorProductionPublicChain(t *testing.T) {
	source := deliveryCNYCSV(t)
	sourceHash := domainsecurity.SHA256Hex(source)
	t.Logf("separately derived Go CNY source sha256=%s", sourceHash)
	var mu sync.Mutex
	calls, semantics := 0, 0
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
		mu.Unlock()
		data := deliveryProviderSemantics(t, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(data) > 0 {
			if len(data) != 1 || data[0].InflowMinor != "1301001" || data[0].OutflowMinor != "120060" || data[0].TransactionCount != 7 || !data[0].AggregateComplete || !data[0].EvidenceRowsComplete || data[0].EvidenceTransactionCount != 7 {
				t.Error("final serialized Provider facts differ from the hand-authored vector")
			}
			mu.Lock()
			semantics++
			mu.Unlock()
			runtimeOptionalLifecycleModelResponseV1(w, "", nil, "合成账户流水分析完成。")
			return
		}
		if call > 2 {
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
