package control

import (
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestHostGeneralOnlySteerCannotIntroduceCaseOrContextData(t *testing.T) {
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-steer", TurnID: "turn-general-steer", WorkspaceRealPath: "/workspace/general-steer",
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []SteerSecurityAdmissionInput{
		{Context: securityContext, RiskIntent: domainsecurity.RiskClassCase},
		{Context: securityContext, LexicalCaseRisk: true},
		{Context: securityContext, ProtectedCaseData: true},
		{Context: securityContext, AttachmentCount: 1},
		{Context: securityContext, FileRefCount: 1},
	} {
		admission, err := EvaluateSteerSecurityAdmission(input)
		if err != nil || !admission.RequiresNewTurn {
			t.Fatalf("host-general-only steer escaped new-turn admission: input=%#v admission=%#v err=%v", input, admission, err)
		}
	}
}

func TestCaseRiskSteerRequiresNewV2Turn(t *testing.T) {
	general := steeringSecurityContextForTest(t, false)
	for _, test := range []struct {
		name    string
		input   SteerSecurityAdmissionInput
		blocker string
	}{
		{name: "explicit-risk", input: SteerSecurityAdmissionInput{Context: general, RiskIntent: "case"}, blocker: SteerBlockerCaseRiskRaise},
		{name: "lexical-risk", input: SteerSecurityAdmissionInput{Context: general, LexicalCaseRisk: true}, blocker: SteerBlockerCaseRiskRaise},
		{name: "protected-data", input: SteerSecurityAdmissionInput{Context: general, ProtectedCaseData: true}, blocker: SteerBlockerCaseRiskRaise},
		{name: "attachment", input: SteerSecurityAdmissionInput{Context: general, AttachmentCount: 1}, blocker: SteerBlockerContextChangingData},
		{name: "file-reference", input: SteerSecurityAdmissionInput{Context: general, FileRefCount: 1}, blocker: SteerBlockerContextChangingData},
	} {
		t.Run(test.name, func(t *testing.T) {
			admission, err := EvaluateSteerSecurityAdmission(test.input)
			if err != nil || !admission.RequiresNewTurn || admission.BlockerCode != test.blocker {
				t.Fatalf("steer admission = %#v err=%v", admission, err)
			}
		})
	}
}

func TestCaseSteerCanContinueOnlyUnderFrozenCaseContext(t *testing.T) {
	admission, err := EvaluateSteerSecurityAdmission(SteerSecurityAdmissionInput{
		Context: steeringSecurityContextForTest(t, true), RiskIntent: "case", LexicalCaseRisk: true,
	})
	if err != nil || admission.RequiresNewTurn {
		t.Fatalf("same-authority case steer admission = %#v err=%v", admission, err)
	}
}

func TestSteerContextChangingInputAlwaysRequiresNewTurn(t *testing.T) {
	admission, err := EvaluateSteerSecurityAdmission(SteerSecurityAdmissionInput{
		Context: steeringSecurityContextForTest(t, true), AttachmentCount: 1,
	})
	if err != nil || !admission.RequiresNewTurn || admission.BlockerCode != SteerBlockerContextChangingData {
		t.Fatalf("case attachment steer admission = %#v err=%v", admission, err)
	}
}

func TestSteeringLogicalEffectBindingUsesRawHostIntentAndKeepsOrdinaryBase(t *testing.T) {
	caseContext := steeringSecurityContextForTest(t, true)
	for _, test := range []struct {
		name         string
		text         string
		admission    SteerSecurityAdmissionInput
		wantEffect   domainsecurity.LogicalEffect
		wantOrdinary bool
	}{
		{
			name: "ordinary code task", text: "修改代码并运行测试",
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
		{
			name: "case semantic task", text: "请核实案件中的 MAC、亲属关系和投标报价",
			admission: SteerSecurityAdmissionInput{LexicalCaseRisk: true}, wantEffect: domainsecurity.LogicalEffectCaseData,
		},
		{
			name: "funds task", text: "查询当前案件银行账号在指定期间的流入、流出和净额",
			admission: SteerSecurityAdmissionInput{LexicalCaseRisk: true}, wantEffect: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "mixed code and funds task", text: "先修改代码并运行测试，再查询当前案件银行账号的流入和流出",
			admission:  SteerSecurityAdmissionInput{LexicalCaseRisk: true},
			wantEffect: domainsecurity.LogicalEffectFundsData, wantOrdinary: true,
		},
		{
			name: "case-looking local filename", text: "读取 /workspace/资金分析.md 文件并汇总目录结构",
			admission:  SteerSecurityAdmissionInput{RiskIntent: domainsecurity.RiskClassCase, LexicalCaseRisk: true},
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
		{
			name: "bound case short funds follow-up", text: "那净额呢",
			admission: SteerSecurityAdmissionInput{Context: caseContext}, wantEffect: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "bound case software account work", text: "Compare this account parser in the test module.",
			admission:  SteerSecurityAdmissionInput{Context: caseContext},
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := SteeringLogicalEffectBindingV1(test.text, test.admission)
			if got.LogicalEffect != test.wantEffect || got.OrdinaryWork != test.wantOrdinary {
				t.Fatalf("binding=%#v want effect=%q ordinary=%v", got, test.wantEffect, test.wantOrdinary)
			}
		})
	}
}

func TestTaskJobSteerLogicalEffectBindingPreservesContextAndQueueRisk(t *testing.T) {
	caseContext := steeringSecurityContextForTest(t, true)
	for _, test := range []struct {
		name             string
		prompt           string
		frozen           domainsecurity.TurnSecurityContext
		childTurnStarted bool
		parentCaseBound  bool
		want             domainsteering.EntryLogicalEffectBinding
	}{
		{
			name: "queued before child turn", prompt: "那净额呢", parentCaseBound: true,
			want: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
		},
		{
			name: "active child turn", prompt: "分析该账户上月有什么异常", frozen: caseContext,
			childTurnStarted: true, parentCaseBound: true,
			want: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
		},
		{
			name: "ordinary child work", prompt: "Compare this account parser in the test module.", frozen: caseContext,
			childTurnStarted: true, parentCaseBound: true,
			want: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := TaskJobSteerLogicalEffectBindingV1(
				test.prompt, test.frozen, test.childTurnStarted, test.parentCaseBound, false, false,
			)
			if got != test.want {
				t.Fatalf("task-job steer binding = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestStricterTaskJobSteerLogicalEffectBindingCannotLowerRisk(t *testing.T) {
	for _, test := range []struct {
		name    string
		current domainsteering.EntryLogicalEffectBinding
		queued  domainsteering.EntryLogicalEffectBinding
		want    domainsteering.EntryLogicalEffectBinding
	}{
		{
			name:    "queued ordinary cannot lower funds",
			current: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
			queued:  domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
			want:    domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
		},
		{
			name:    "queued funds raises generic case",
			current: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectCaseData, OrdinaryWork: true},
			queued:  domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
			want:    domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
		},
		{
			name:    "ordinary remains ordinary",
			current: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
			queued:  domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
			want:    domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := StricterTaskJobSteerLogicalEffectBindingV1(test.current, test.queued); got != test.want {
				t.Fatalf("merged task-job steer binding = %#v, want %#v", got, test.want)
			}
		})
	}
}

func steeringSecurityContextForTest(t *testing.T, caseRisk bool) domainsecurity.TurnSecurityContext {
	t.Helper()
	at := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	riskClass := domainsecurity.RiskClassGeneral
	disposition := domainsecurity.PublicationDispositionGeneralOutput
	bindingState := domainsecurity.CaseBindingStateMissing
	caseID := domainsecurity.UnboundCaseID
	caseBindingHash := domainsecurity.UnboundCaseBindingHash("/workspace")
	datasetSnapshotID := domainsecurity.NoDatasetSnapshotID
	sourceManifestHash := domainsecurity.EmptySourceManifestHash
	if caseRisk {
		riskClass = domainsecurity.RiskClassCase
		disposition = domainsecurity.PublicationDispositionCaseEvidenceGate
		bindingState = domainsecurity.CaseBindingStateValid
		caseID = "case-1"
		caseBindingHash = domainsecurity.SHA256Hex([]byte("case-binding"))
		datasetSnapshotID = securitycontexttest.DatasetSnapshotID("control-steering-dataset")
		sourceManifestHash = domainsecurity.SHA256Hex([]byte("source-manifest"))
	}
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("risk-policy")), RiskClass: riskClass,
		Disposition: disposition, CaseBindingState: bindingState,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	context, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", TenantID: "tenant-1", UserID: "user-1",
		CaseID: caseID, CaseBindingHash: caseBindingHash, DatasetSnapshotID: datasetSnapshotID,
		SourceManifestHash: sourceManifestHash, ContextEpoch: 1, IssuedAt: at, PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.RiskAuthorityBindingV1{
			SchemaVersion: domainsecurity.RiskAuthorityBindingSchemaVersion, Purpose: domainsecurity.RiskAuthorityBindingPurpose,
			State: domainsecurity.RiskAuthorityBindingStateWitnessed, IndexDigest: domainsecurity.SHA256Hex([]byte("risk-index")),
			Generation: 1, CheckpointDigest: domainsecurity.SHA256Hex([]byte("risk-checkpoint")),
			ObservationDigest: domainsecurity.SHA256Hex([]byte("risk-observation")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(context); err != nil {
		t.Fatalf("test security context: %v", err)
	}
	return context
}

func TestBuildSteeringEntryCreatesStableEntryAndGeneratedID(t *testing.T) {
	now := time.Date(2026, 7, 3, 8, 0, 0, 123, time.UTC)
	fileRef := map[string]any{"path": "/tmp/a.go"}
	clientID, entry, err := BuildSteeringEntry(SteeringEntryInput{
		TurnID:         "turn/one",
		Text:           "  refine scope  ",
		DisplayText:    "Refine scope",
		AttachmentIDs:  []string{"att_1"},
		FileReferences: []any{fileRef},
		EffectBinding: domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectOrdinary,
			OrdinaryWork:  true,
		},
		Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !IsOpaqueClientUserMessageID(clientID) ||
		entry["id"] != domainsteering.EntryIDV1("turn/one", clientID) ||
		entry["clientUserMessageId"] != clientID ||
		entry["text"] != "refine scope" ||
		entry["delivery"] != "steer" ||
		entry["logicalEffect"] != string(domainsecurity.LogicalEffectOrdinary) ||
		entry["ordinaryWork"] != true ||
		entry["admittedAt"] != "2026-07-03T08:00:00.000000123Z" {
		t.Fatalf("steering entry mismatch: %#v", entry)
	}
	if entry["displayText"] != "Refine scope" {
		t.Fatalf("display text should be preserved when distinct: %#v", entry)
	}
	fileRef["path"] = "mutated"
	refs, _ := entry["fileReferences"].([]any)
	ref, _ := refs[0].(map[string]any)
	if ref["path"] != "/tmp/a.go" {
		t.Fatalf("file references should be cloned: %#v", entry)
	}
}

func TestBuildSteeringEntryKeepsExplicitClientID(t *testing.T) {
	const opaqueClientID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	clientID, entry, err := BuildSteeringEntry(SteeringEntryInput{
		TurnID:              "turn-1",
		ClientUserMessageID: " " + opaqueClientID + " ",
		Text:                "more",
		DisplayText:         "more",
		EffectBinding: domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectOrdinary,
			OrdinaryWork:  true,
		},
		Now: time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if clientID != opaqueClientID || entry["id"] != domainsteering.EntryIDV1("turn-1", opaqueClientID) ||
		entry["clientUserMessageId"] != opaqueClientID {
		t.Fatalf("explicit client id mismatch: %q %#v", clientID, entry)
	}
	if _, ok := entry["displayText"]; ok {
		t.Fatalf("display text equal to text should be omitted: %#v", entry)
	}
}

func TestTurnSteeredEventAndAcceptedResponse(t *testing.T) {
	event := BuildTurnSteeredEvent(TurnSteeredEventInput{
		ThreadID:            " thread ",
		TurnID:              " turn ",
		Text:                " more ",
		DisplayText:         " More ",
		ClientUserMessageID: " client ",
	})
	if event["kind"] != "turn_steered" || event["threadId"] != "thread" || event["displayText"] != "More" {
		t.Fatalf("turn steered event mismatch: %#v", event)
	}
	response := SteeringAcceptedResponse(" thread ", " turn ", " client ", float64(7))
	if response["ok"] != true || response["threadId"] != "thread" || response["admittedSeq"] != float64(7) {
		t.Fatalf("accepted response mismatch: %#v", response)
	}
}

func TestBuildSteeringEntryRequiresHostLogicalEffectBinding(t *testing.T) {
	if _, _, err := BuildSteeringEntry(SteeringEntryInput{TurnID: "turn-1", Text: "more"}); err == nil {
		t.Fatal("missing logical effect binding was accepted")
	}
	if _, _, err := BuildSteeringEntry(SteeringEntryInput{
		TurnID: "turn-1", Text: "more",
		EffectBinding: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectOrdinary},
	}); err == nil {
		t.Fatal("ordinary effect without ordinary work was accepted")
	}
}

type steeringStoreStub struct {
	admitErr              error
	recordErr             error
	entry                 map[string]any
	event                 map[string]any
	expectedContextDigest string
}

func (stub *steeringStoreStub) AdmitSteeringEntryForContext(_, _, _, expectedContextDigest string, entry map[string]any) (map[string]any, error) {
	stub.entry = entry
	stub.expectedContextDigest = expectedContextDigest
	if stub.admitErr != nil {
		return nil, stub.admitErr
	}
	admitted := map[string]any{}
	for key, value := range entry {
		admitted[key] = value
	}
	admitted["displayText"] = "Display"
	return admitted, nil
}

func (stub *steeringStoreStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.event = event
	if stub.recordErr != nil {
		return nil, nil, stub.recordErr
	}
	recorded := map[string]any{}
	for key, value := range event {
		recorded[key] = value
	}
	recorded["seq"] = float64(5)
	return recorded, []string{"persist", "publish"}, nil
}

func TestAdmitSteeringTurnPersistsEntryEventAndResponse(t *testing.T) {
	const contextDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const opaqueClientID = "550e8400-e29b-41d4-a716-446655440000"
	store := &steeringStoreStub{}
	response, err := AdmitSteeringTurn(AdmitSteeringInput{
		Store:                 store,
		ExpectedContextDigest: contextDigest,
		EffectBinding: domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectFundsData,
			OrdinaryWork:  true,
		},
		Request: SteerTurnRequest{
			ThreadID:            "thr_1",
			TurnID:              "turn_1",
			Text:                " more ",
			DisplayText:         "Display",
			ClientUserMessageID: opaqueClientID,
			ExpectedTurnID:      "turn_1",
			AttachmentIDs:       []string{"att_1"},
		},
		Now: time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("admit steering: %v", err)
	}
	if store.entry["text"] != "more" || store.entry["clientUserMessageId"] != opaqueClientID ||
		store.entry["logicalEffect"] != string(domainsecurity.LogicalEffectFundsData) || store.entry["ordinaryWork"] != true {
		t.Fatalf("steering entry mismatch: %#v", store.entry)
	}
	if store.expectedContextDigest != contextDigest {
		t.Fatalf("steering context digest mismatch: %q", store.expectedContextDigest)
	}
	if store.event["kind"] != "turn_steered" || store.event["displayText"] != "Display" {
		t.Fatalf("steering event mismatch: %#v", store.event)
	}
	if response["admittedSeq"] != float64(5) || response["clientUserMessageId"] != opaqueClientID {
		t.Fatalf("steering response mismatch: %#v", response)
	}
}

func TestAdmitSteeringTurnRejectsNonOpaqueClientIDBeforePersistence(t *testing.T) {
	store := &steeringStoreStub{}
	_, err := AdmitSteeringTurn(AdmitSteeringInput{
		Store:                 store,
		ExpectedContextDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Request: SteerTurnRequest{
			ThreadID: "thr_1", TurnID: "turn_1", Text: "more",
			ClientUserMessageID: "6222021234567890123",
		},
	})
	if !errors.Is(err, ErrInvalidSteerClientID) || store.entry != nil || store.event != nil {
		t.Fatalf("unsafe client id reached persistence: entry=%#v event=%#v err=%v", store.entry, store.event, err)
	}
}

func TestAdmitSteeringTurnPropagatesStoreErrors(t *testing.T) {
	_, err := AdmitSteeringTurn(AdmitSteeringInput{
		Store:   nil,
		Request: SteerTurnRequest{ThreadID: "thr_1", TurnID: "turn_1", Text: "more"},
	})
	if err == nil || err.Error() != "steering store is required" {
		t.Fatalf("expected missing store error, got %v", err)
	}
	admitErr := errors.New("turn inactive")
	_, err = AdmitSteeringTurn(AdmitSteeringInput{
		Store:                 &steeringStoreStub{admitErr: admitErr},
		ExpectedContextDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EffectBinding: domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
		},
		Request: SteerTurnRequest{ThreadID: "thr_1", TurnID: "turn_1", Text: "more"},
	})
	if !errors.Is(err, admitErr) {
		t.Fatalf("expected admit error, got %v", err)
	}
	recordErr := errors.New("record failed")
	_, err = AdmitSteeringTurn(AdmitSteeringInput{
		Store:                 &steeringStoreStub{recordErr: recordErr},
		ExpectedContextDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EffectBinding: domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
		},
		Request: SteerTurnRequest{ThreadID: "thr_1", TurnID: "turn_1", Text: "more"},
	})
	if !errors.Is(err, recordErr) {
		t.Fatalf("expected record error, got %v", err)
	}
}

func TestAdmitSteeringTurnRequiresFrozenContextDigest(t *testing.T) {
	store := &steeringStoreStub{}
	_, err := AdmitSteeringTurn(AdmitSteeringInput{
		Store:   store,
		Request: SteerTurnRequest{ThreadID: "thr_1", TurnID: "turn_1", Text: "more"},
	})
	if err == nil || err.Error() != "steering context digest is invalid" {
		t.Fatalf("expected context digest rejection, got %v", err)
	}
	if store.entry != nil || store.event != nil {
		t.Fatalf("invalid context must not reach persistence: entry=%#v event=%#v", store.entry, store.event)
	}
}
