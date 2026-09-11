package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	evidenceregistryadapter "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

func TestSteerThreadReadFailureUsesFixedPublicProjection(t *testing.T) {
	const pathSentinel = "customer-pii-13900000057-steer-thread-read"
	durableRoot := filepath.Join(t.TempDir(), pathSentinel)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "steer-read-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "steer-read-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Steer read projection", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_13900000058"
	before, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	threadPath := handler.store.threadPath(threadID)
	if err := os.Remove(threadPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(threadPath, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := handler.steerRuntimeTurn(context.Background(), controlapp.SteerTurnRequest{
		ThreadID: threadID, TurnID: turnID, ExpectedTurnID: turnID, Text: "ordinary follow-up",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 500 || stringField(result.Body, "code") != "internal_error" ||
		stringField(result.Body, "message") != "thread state is unavailable" {
		t.Fatalf("steer thread-read failure result = %#v", result)
	}
	body, err := json.Marshal(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{pathSentinel, threadID, turnID, "is a directory"} {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("steer thread-read failure leaked %q: %s", sentinel, body)
		}
	}
	after, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("steer thread-read failure changed event sequence: before=%#v after=%#v", before.Events, after.Events)
	}
}

func TestSteeringLogicalEffectBindingUsesRawHostIntentAndKeepsOrdinaryBase(t *testing.T) {
	caseContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-steer-effect", TurnID: "turn-steer-effect", WorkspaceRealPath: t.TempDir(),
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	for _, test := range []struct {
		name         string
		text         string
		admission    controlapp.SteerSecurityAdmissionInput
		wantEffect   domainsecurity.LogicalEffect
		wantOrdinary bool
	}{
		{
			name: "ordinary code task", text: "修改代码并运行测试",
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
		{
			name: "case semantic task", text: "请核实案件中的 MAC、亲属关系和投标报价",
			admission:  controlapp.SteerSecurityAdmissionInput{LexicalCaseRisk: true},
			wantEffect: domainsecurity.LogicalEffectCaseData,
		},
		{
			name: "funds task", text: "查询当前案件银行账号在指定期间的流入、流出和净额",
			admission:  controlapp.SteerSecurityAdmissionInput{LexicalCaseRisk: true},
			wantEffect: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "mixed code and funds task", text: "先修改代码并运行测试，再查询当前案件银行账号的流入和流出",
			admission:  controlapp.SteerSecurityAdmissionInput{LexicalCaseRisk: true},
			wantEffect: domainsecurity.LogicalEffectFundsData, wantOrdinary: true,
		},
		{
			name: "case-looking local filename", text: "读取 /workspace/资金分析.md 文件并汇总目录结构",
			admission:  controlapp.SteerSecurityAdmissionInput{RiskIntent: domainsecurity.RiskClassCase, LexicalCaseRisk: true},
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
		{
			name: "bound case deictic account follow-up", text: "分析该账户上月有什么异常",
			admission:  controlapp.SteerSecurityAdmissionInput{Context: caseContext},
			wantEffect: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "bound case short funds follow-up", text: "那净额呢",
			admission:  controlapp.SteerSecurityAdmissionInput{Context: caseContext},
			wantEffect: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "bound case terse outflow follow-up", text: "再看流出",
			admission:  controlapp.SteerSecurityAdmissionInput{Context: caseContext},
			wantEffect: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "bound case software account work", text: "Compare this account parser in the test module.",
			admission:  controlapp.SteerSecurityAdmissionInput{Context: caseContext},
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := steeringLogicalEffectBinding(test.text, test.admission)
			if got.LogicalEffect != test.wantEffect || got.OrdinaryWork != test.wantOrdinary {
				t.Fatalf("binding=%#v want effect=%q ordinary=%v", got, test.wantEffect, test.wantOrdinary)
			}
		})
	}

	const account = "6222021234567890123"
	raw := "请查询银行账号 " + account + " 在指定期间的流入、流出和净额"
	binding := steeringLogicalEffectBinding(raw, controlapp.SteerSecurityAdmissionInput{
		LexicalCaseRisk: true, ProtectedCaseData: true,
	})
	projected, _, _ := privacyprojectionapp.ProjectSteeringContent(raw, "", nil)
	if binding != (domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData}) ||
		strings.Contains(projected, account) {
		t.Fatalf("raw host intent or privacy projection mismatch: binding=%#v projected=%q", binding, projected)
	}
}

func TestHostClassifiesNonFundAndDisplayCaseRiskBeforeProvider(t *testing.T) {
	for _, test := range []struct {
		name           string
		prompt         string
		displayText    string
		fileReferences []any
		forbidden      string
	}{
		{name: "non-fund-case-facts", prompt: "请核实案件中的 MAC、亲属关系和投标报价"},
		{name: "display-text", prompt: "Please continue.", displayText: "请形成经侦案件行贿与利益输送报告"},
		{name: "file-reference", prompt: "Summarize the supplied file.", fileReferences: []any{map[string]any{"path": "/workspace/bank-ledger.csv", "kind": "file"}}},
		{name: "restricted-account", prompt: "请核实账号 6222020000000000000 的资金流水", forbidden: "6222020000000000000"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			durableRoot := t.TempDir()
			handler := NewRuntimeServerHandler(RuntimeServerConfig{
				RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
				ProviderID: "risk-classifier-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
				Model: "risk-classifier-model", EndpointFormat: "chat_completions",
			}).(*runtimeServerHandler)
			configureServerGeneralExecution(t, handler)
			configureSteerTestCaseAuthorities(t, handler, durableRoot)
			provider := &admissionFailureCountingProvider{}
			handler.provider = provider
			thread, err := handler.store.CreateThread(map[string]any{
				"title": "risk classifier", "workspace": workspace,
				"providerId": "risk-classifier-provider", "model": "risk-classifier-model",
			}, workspace)
			if err != nil {
				t.Fatal(err)
			}
			threadID := stringField(thread, "id")
			started, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
				Prompt: test.prompt, DisplayText: test.displayText, FileReferences: test.fileReferences,
			})
			if err != nil {
				t.Fatal(err)
			}
			if provider.calls.Load() != 0 {
				t.Fatal("host-classified unbound case request reached provider")
			}
			reloaded, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			var turn map[string]any
			for _, value := range listAny(reloaded["turns"]) {
				candidate, _ := value.(map[string]any)
				if stringField(candidate, "id") == stringField(started, "turnId") {
					turn = candidate
					break
				}
			}
			if turn == nil {
				t.Fatalf("started turn is missing: %#v", reloaded["turns"])
			}
			if test.forbidden != "" {
				threadBody, _ := json.Marshal(reloaded)
				if strings.Contains(string(threadBody), test.forbidden) || !strings.Contains(string(threadBody), "[ACCOUNT]") {
					t.Fatalf("restricted PII crossed durable turn boundary: %s", threadBody)
				}
				replay, replayErr := handler.store.LoadEventsSince(threadID, 0)
				if replayErr != nil {
					t.Fatal(replayErr)
				}
				eventBody, _ := json.Marshal(replay.Events)
				if strings.Contains(string(eventBody), test.forbidden) || !strings.Contains(string(eventBody), "[ACCOUNT]") {
					t.Fatalf("restricted PII crossed durable event boundary: %s", eventBody)
				}
			}
			securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
				!domainsecurity.TurnSecurityContextRequiresFinalEvidenceGate(securityContext) {
				t.Fatalf("case request did not freeze a boundary-only case V2 context: context=%#v err=%v", securityContext, err)
			}
		})
	}
}

func configureSteerTestCaseAuthorities(t *testing.T, handler *runtimeServerHandler, durableRoot string) {
	t.Helper()
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	keyRoot := filepath.Join(privateRoot, "case-key")
	if err := os.Mkdir(keyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	signer, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(keyRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newServerTestCaseThreadStore(t, filepath.Join(privateRoot, "case-authority-records"))
	if err != nil {
		t.Fatal(err)
	}
	authority, err := casethreadapp.NewRegistry(context.Background(), signer, store)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseThreads = authority
	handler.store.SetCaseThreadAuthority(authority)

	finalKeyRoot := filepath.Join(privateRoot, "final-key")
	if err := os.Mkdir(finalKeyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	finalSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(finalKeyRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	evidenceRegistry, err := evidenceregistryadapter.NewStore(filepath.Join(privateRoot, "evidence-registry"), finalSigner)
	if err != nil {
		t.Fatal(err)
	}
	privateFinals, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseFinalizer = evidenceapp.NewCasePublicationFinalizerWithAuthority(
		evidenceRegistry, evidenceRegistry, finalSigner, privateFinals,
		durableAcceptedFinalEventIO(handler.store, casReader), newServerTestTurnTerminalCoordinator(t, finalSigner, privateFinals),
	)
}

func TestHighRiskSteerCannotEnterFrozenGeneralTurn(t *testing.T) {
	for _, test := range []struct {
		name        string
		request     controlapp.SteerTurnRequest
		wantBlocker string
	}{
		{
			name: "explicit-case-risk",
			request: controlapp.SteerTurnRequest{
				Text: "EXPLICIT_CASE_STEER_MUST_NOT_PERSIST_481902", RiskIntent: "case",
			},
			wantBlocker: controlapp.SteerBlockerCaseRiskRaise,
		},
		{
			name: "host-classified-case-risk",
			request: controlapp.SteerTurnRequest{
				Text: "请分析银行账号与资金流水 LEXICAL_CASE_STEER_MUST_NOT_PERSIST_731604",
			},
			wantBlocker: controlapp.SteerBlockerCaseRiskRaise,
		},
		{
			name: "display-text-case-risk",
			request: controlapp.SteerTurnRequest{
				Text:        "DISPLAY_CASE_STEER_MUST_NOT_PERSIST_193750",
				DisplayText: "请分析案件银行账号与金额",
			},
			wantBlocker: controlapp.SteerBlockerCaseRiskRaise,
		},
		{
			name: "attachment",
			request: controlapp.SteerTurnRequest{
				Text: "ATTACHMENT_STEER_MUST_NOT_PERSIST_509137", AttachmentIDs: []string{"att-new-context"},
			},
			wantBlocker: controlapp.SteerBlockerContextChangingData,
		},
		{
			name: "file-reference",
			request: controlapp.SteerTurnRequest{
				Text:           "FILE_REFERENCE_STEER_MUST_NOT_PERSIST_842615",
				FileReferences: []any{map[string]any{"path": "/workspace/new.csv", "kind": "file"}},
			},
			wantBlocker: controlapp.SteerBlockerContextChangingData,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			handler := NewRuntimeServerHandler(RuntimeServerConfig{
				RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
				ProviderID: "steer-security-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
				Model: "steer-security-model", EndpointFormat: "chat_completions",
			}).(*runtimeServerHandler)
			configureServerGeneralExecution(t, handler)
			provider := &ignoredCancellationProvider{
				entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}),
				text: "AUTHORIZED_ORDINARY_RESULT_MUST_PUBLISH_264809",
			}
			handler.provider = provider
			released := false
			defer func() {
				if !released {
					close(provider.release)
				}
			}()

			thread, err := handler.store.CreateThread(map[string]any{
				"title": "steer security", "workspace": workspace,
				"providerId": "steer-security-provider", "model": "steer-security-model",
			}, workspace)
			if err != nil {
				t.Fatal(err)
			}
			threadID := stringField(thread, "id")
			started, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
				Prompt: "Summarize this generic note.", Async: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			turnID := stringField(started, "turnId")
			if turnID == "" {
				t.Fatalf("start response lost turn id: %#v", started)
			}
			select {
			case <-provider.entered:
			case <-time.After(foregroundProviderEntryTimeout):
				t.Fatal("provider did not enter before steer admission")
			}

			request := test.request
			request.ThreadID = threadID
			request.TurnID = turnID
			request.ExpectedTurnID = turnID
			result, err := handler.steerRuntimeTurn(context.Background(), request)
			if err != nil || result.StatusCode != 409 || stringField(result.Body, "code") != "new_turn_required" ||
				stringField(result.Body, "blockerCode") != test.wantBlocker {
				t.Fatalf("high-risk steer result = %#v err=%v", result, err)
			}
			select {
			case <-provider.cancelled:
				t.Fatal("rejected high-risk steer cancelled the already-authorized ordinary turn")
			default:
			}

			reloaded, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			threadBody, _ := json.Marshal(reloaded)
			if strings.Contains(string(threadBody), request.Text) {
				t.Fatalf("rejected steer entered durable thread state: %s", threadBody)
			}
			events, err := handler.store.LoadEventsSince(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events.Events {
				eventBody, _ := json.Marshal(event)
				if stringField(event, "kind") == "turn_steered" || strings.Contains(string(eventBody), request.Text) {
					t.Fatalf("rejected steer entered durable events: %#v", event)
				}
			}

			close(provider.release)
			released = true
			waitContext, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelWait()
			if err := handler.runtimeControl().WaitForThreadTurns(waitContext, threadID); err != nil {
				t.Fatalf("authorized ordinary continuation did not settle: %v", err)
			}
			settled, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			settledBody, _ := json.Marshal(settled)
			if strings.Contains(string(settledBody), request.Text) || !strings.Contains(string(settledBody), provider.text) {
				t.Fatalf("rejected steer entered history or authorized ordinary result was lost: %s", settledBody)
			}
		})
	}
}
