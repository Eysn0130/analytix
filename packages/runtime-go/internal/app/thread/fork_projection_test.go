package thread

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestGoalDerivedThreadsDoNotReuseSourceBoundGoalHistory(t *testing.T) {
	goal := map[string]any{"threadId": "source", "id": "goal-source", "revision": 1, "objective": "Recorded source objective", "status": "active", "maxGoalRounds": 8, "roundsStarted": 1, "tokensUsed": 0, "timeUsedSeconds": 0, "createdAt": "2026-10-11T00:00:00Z", "updatedAt": "2026-10-11T00:00:00Z"}
	source := map[string]any{"id": "source", "title": "Source", "status": "idle", "turns": []any{}, "goal": goal,
		"goalHistory": []any{map[string]any{"id": "goal-record", "operation": "round_started", "createdAt": "2026-10-11T00:00:00Z", "ref": map[string]any{"id": "goal-source", "revision": 1}, "goal": contracts.CloneMap(goal)}}}
	before, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectPublicThread(source); err != nil {
		t.Fatal("source fixture is not a valid public thread", err)
	}
	for _, kind := range []string{"fork", "resume"} {
		t.Run(kind, func(t *testing.T) {
			var derived map[string]any
			var err error
			if kind == "fork" {
				derived, err = BuildFork(ForkInput{Source: source, ForkID: "derived", ParentThreadID: "source", Now: "2026-10-11T00:01:00Z"})
			} else {
				derived, err = BuildResume(ResumeInput{Source: source, ThreadID: "derived", SessionID: "source", Now: "2026-10-11T00:01:00Z"})
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ProjectPublicThread(derived); err != nil {
				t.Fatal("derived public thread rejected source-bound Goal history", err)
			}
			for _, field := range []string{"goal", "goalHistory"} {
				if _, present := derived[field]; present {
					t.Fatal("derived thread reused source Goal authority", field)
				}
			}
			after, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("source recorded history was mutated")
			}
		})
	}
}

func TestForkQuestionPreservesHostIdentityAndMultiSelectWithoutAuthority(t *testing.T) {
	inputID := "input_" + strings.Repeat("a", 31) + "9999999999999" + strings.Repeat("b", 20)
	for _, status := range []string{"pending", "continued", "submitted", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			source := map[string]any{"id": "item_" + inputID, "kind": "user_input", "threadId": "source", "turnId": "turn", "inputId": inputID, "status": status,
				"continuationReceiptId": strings.Repeat("c", 64), "securityContext": map[string]any{"private": true}, "executionGrant": map[string]any{"private": true},
				"questions": []any{map[string]any{"id": inputID + "_1", "question": "Synthetic question", "multiSelect": true,
					"options": []any{map[string]any{"id": inputID + "_1_option_1", "label": "One", "description": ""}}}}}
			before := contracts.CloneMap(source)
			cloned := CloneItemForThread(source, "child", "2026-10-09T00:00:00Z")
			want := status
			if status == "pending" {
				want = "cancelled"
			}
			if cloned["inputId"] != inputID || cloned["status"] != want || cloned["threadId"] != "child" {
				t.Fatal("history identity or inert status changed")
			}
			question := cloned["questions"].([]any)[0].(map[string]any)
			if question["id"] != inputID+"_1" || question["multiSelect"] != true || question["options"].([]any)[0].(map[string]any)["id"] != inputID+"_1_option_1" {
				t.Fatal("original question identity or selection semantics lost")
			}
			for _, field := range []string{"continuationReceiptId", "securityContext", "executionGrant"} {
				if _, exists := cloned[field]; exists {
					t.Fatal("fork retained execution authority")
				}
			}
			if domainprivacy.ValidatePublicValue(cloned) != nil || domainprivacy.ValidatePublicSourceProse(cloned) != nil {
				t.Fatal("legitimate Host identity became prose PII")
			}
			cloned["prompt"] = "account 6222020000000000000"
			if domainprivacy.ValidatePublicValue(cloned) == nil {
				t.Fatal("question identity exception admitted ordinary account prose")
			}
			if !reflect.DeepEqual(source, before) {
				t.Fatal("clone mutated original source")
			}
		})
	}
}

func TestBuildForkClonesThreadHistoryAndLineage(t *testing.T) {
	firstContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_source", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	secondContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_source", TurnID: "turn_2", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(2, 0),
	})
	source := map[string]any{
		"id":                "thr_source",
		"title":             "Source thread",
		"status":            "running",
		"securityState":     secondContext,
		"contextEpochState": map[string]any{"threadId": "thr_source"},
		"goal":              map[string]any{"text": "old goal"},
		"todos": map[string]any{"threadId": "thr_source", "items": []any{
			map[string]any{"id": "todo_failed", "content": "Run tool", "status": "failed", "statusReasonCode": "tool_failed"},
		}},
		"turns": []any{
			map[string]any{
				"id":              "turn_1",
				"threadId":        "thr_source",
				"status":          "completed",
				"securityContext": firstContext,
				"items": []any{
					map[string]any{"kind": "user_message", "threadId": "thr_source", "attachmentIds": []any{"att_1", "att_1"}},
					map[string]any{"kind": "assistant_message", "threadId": "thr_source"},
				},
			},
			map[string]any{
				"id":              "turn_2",
				"threadId":        "thr_source",
				"status":          "running",
				"securityContext": secondContext,
				"items": []any{
					map[string]any{"kind": "user_message", "threadId": "thr_source", "attachmentIds": []any{"att_2"}},
					map[string]any{"kind": "approval", "status": "pending", "threadId": "thr_source"},
				},
			},
		},
	}
	fork, err := BuildFork(ForkInput{
		Source:         source,
		ForkID:         "thr_fork",
		ParentThreadID: "thr_source",
		Now:            "2026-07-03T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("build fork: %v", err)
	}
	if fork["id"] != "thr_fork" || fork["title"] != "Source thread fork" || fork["relation"] != "fork" {
		t.Fatalf("unexpected fork identity: %#v", fork)
	}
	if fork["goal"] != nil {
		t.Fatalf("fork must not inherit active goal: %#v", fork["goal"])
	}
	if fork["securityState"] != nil || fork["contextEpochState"] != nil {
		t.Fatalf("fork must re-establish thread-scoped security and epoch state: %#v", fork)
	}
	if got := fork["forkedFromMessageCount"]; got != float64(2) {
		t.Fatalf("fork message count mismatch: %#v", got)
	}
	if got := fork["forkedFromTurnCount"]; got != float64(2) {
		t.Fatalf("fork turn count mismatch: %#v", got)
	}
	todos, _ := fork["todos"].(map[string]any)
	if todos["threadId"] != "thr_fork" || todos["updatedAt"] != "2026-07-03T00:00:00Z" {
		t.Fatalf("fork todos were not rebound: %#v", todos)
	}
	forkedTodo := todos["items"].([]any)[0].(map[string]any)
	if forkedTodo["status"] != "failed" || forkedTodo["statusReasonCode"] != "tool_failed" {
		t.Fatalf("fork lost terminal todo lifecycle: %#v", forkedTodo)
	}
	turns, _ := fork["turns"].([]any)
	second, _ := turns[1].(map[string]any)
	if second["status"] != "completed" || second["finishedAt"] != "2026-07-03T00:00:00Z" {
		t.Fatalf("running fork turn should be completed: %#v", second)
	}
	items, _ := second["items"].([]any)
	approval, _ := items[1].(map[string]any)
	if approval["status"] != "expired" || approval["threadId"] != "thr_fork" {
		t.Fatalf("pending approval should expire on fork: %#v", approval)
	}
}

func TestBuildSideForkKeepsOnlyUserItemFromRunningTurn(t *testing.T) {
	source := map[string]any{
		"title": "Parent",
		"turns": []any{
			map[string]any{
				"id":       "turn_1",
				"status":   "running",
				"threadId": "thr_parent",
				"items": []any{
					map[string]any{"kind": "user_message", "threadId": "thr_parent", "attachmentIds": []any{"att_side"}},
					map[string]any{"kind": "tool_call", "status": "running", "threadId": "thr_parent"},
				},
			},
		},
	}
	fork, err := BuildFork(ForkInput{
		Source:         source,
		ForkID:         "thr_side",
		ParentThreadID: "thr_parent",
		Now:            "2026-07-03T00:00:00Z",
		Relation:       "side",
	})
	if err != nil {
		t.Fatalf("build side fork: %v", err)
	}
	turns, _ := fork["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	if turn["status"] != "aborted" {
		t.Fatalf("side fork should abort running source turn: %#v", turn)
	}
	items, _ := turn["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("side fork should keep only the user item, got %#v", items)
	}
	user, _ := items[0].(map[string]any)
	if user["threadId"] != "thr_side" {
		t.Fatalf("side fork user item should be rebound: %#v", user)
	}
}

func TestBuildForkStopsAtRequestedTurn(t *testing.T) {
	source := map[string]any{
		"title": "Parent",
		"todos": map[string]any{
			"items": []any{"drop when fork is not latest"},
		},
		"turns": []any{
			map[string]any{"id": "turn_1", "items": []any{map[string]any{"kind": "user_message"}}},
			map[string]any{"id": "turn_2", "items": []any{map[string]any{"kind": "user_message"}}},
		},
	}
	fork, err := BuildFork(ForkInput{
		Source:         source,
		ForkID:         "thr_fork",
		ParentThreadID: "thr_parent",
		TurnID:         "turn_1",
	})
	if err != nil {
		t.Fatalf("build bounded fork: %v", err)
	}
	turns, _ := fork["turns"].([]any)
	if len(turns) != 1 {
		t.Fatalf("expected one cloned turn, got %#v", turns)
	}
	if _, ok := fork["todos"]; ok {
		t.Fatalf("bounded fork should not inherit todos from later state: %#v", fork["todos"])
	}
	_, err = BuildFork(ForkInput{Source: source, ForkID: "thr_missing", TurnID: "missing"})
	if !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("expected ErrTurnNotFound, got %v", err)
	}
}

func TestForkAndResumeDropOnlyPrivateProtocolToolPairs(t *testing.T) {
	privateCallID := "call_host_" + strings.Repeat("a", 64)
	ordinaryCallID := "call_host_" + strings.Repeat("b", 64)
	projection := domaintoolresult.PublicToolResultProjectionRecordV1(
		domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
	)
	toolPair := func(callID string, private bool) []any {
		call := map[string]any{
			"id": domaintoolcall.ToolCallItemIDV1("turn-tools", callID), "turnId": "turn-tools", "threadId": "thread-source",
			"kind": "tool_call", "status": "completed", "toolName": "read_file", "callId": callID,
			"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
		}
		result := map[string]any{
			"id": domaintoolresult.ToolResultItemIDV1("turn-tools", callID), "turnId": "turn-tools", "threadId": "thread-source",
			"kind": "tool_result", "status": "completed", "toolName": "read_file", "callId": callID,
			"isError": false, "output": projection,
		}
		if private {
			result["privateProtocolObserved"] = true
		}
		return []any{call, result}
	}
	items := []any{map[string]any{
		"id": "user-tools", "turnId": "turn-tools", "threadId": "thread-source",
		"kind": "user_message", "status": "completed", "text": "inspect",
	}}
	items = append(items, toolPair(privateCallID, true)...)
	items = append(items, toolPair(ordinaryCallID, false)...)
	source := map[string]any{
		"id": "thread-source", "title": "Source", "turns": []any{map[string]any{
			"id": "turn-tools", "threadId": "thread-source", "status": "completed", "items": items,
		}},
	}
	fork, err := BuildFork(ForkInput{
		Source: source, ForkID: "thread-fork", ParentThreadID: "thread-source", Now: "2026-07-31T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	resume, err := BuildResume(ResumeInput{
		Source: source, ThreadID: "thread-resume", SessionID: "thread-source", Now: "2026-07-31T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, derived := range map[string]map[string]any{"fork": fork, "resume": resume} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(derived)
			if strings.Contains(string(body), privateCallID) || strings.Contains(string(body), "privateProtocolObserved") {
				t.Fatalf("derived thread retained attempt-private tool pair or marker: %s", body)
			}
			if !strings.Contains(string(body), ordinaryCallID) {
				t.Fatalf("derived thread dropped ordinary native tool pair: %s", body)
			}
		})
	}
}

func TestBuildResumeRebindsRuntimeThreadState(t *testing.T) {
	unbound := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_session", TurnID: "turn_1", WorkspaceRealPath: "/old", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	source := map[string]any{
		"title":             "Old session",
		"workspace":         "/old",
		"model":             "old-model",
		"mode":              "plan",
		"goal":              map[string]any{"text": "do not carry"},
		"securityState":     unbound,
		"contextEpochState": map[string]any{"threadId": "thr_session"},
		"todos": map[string]any{"threadId": "thr_session", "items": []any{
			map[string]any{"id": "todo_canceled", "content": "Canceled", "status": "canceled", "statusReasonCode": "user_canceled"},
		}},
		"turns": []any{
			map[string]any{
				"id":              "turn_1",
				"threadId":        "thr_session",
				"status":          "running",
				"securityContext": unbound,
				"items": []any{
					map[string]any{"kind": "user_message", "threadId": "thr_session"},
					map[string]any{"kind": "user_input", "status": "pending", "threadId": "thr_session"},
				},
			},
		},
	}
	resumed, err := BuildResume(ResumeInput{
		Source:    source,
		ThreadID:  "thr_resume",
		SessionID: "thr_session",
		Now:       "2026-07-03T00:00:00Z",
		Workspace: "/new",
		Model:     "new-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resumed["id"] != "thr_resume" || resumed["title"] != "Old session resumed" {
		t.Fatalf("unexpected resume identity: %#v", resumed)
	}
	if resumed["workspace"] != "/new" || resumed["model"] != "new-model" || resumed["mode"] != "plan" {
		t.Fatalf("unexpected resume overrides: %#v", resumed)
	}
	if resumed["relation"] != "primary" || resumed["forkedFromThreadId"] != "thr_session" {
		t.Fatalf("unexpected resume lineage: %#v", resumed)
	}
	if resumed["securityState"] != nil || resumed["contextEpochState"] != nil {
		t.Fatalf("resume must re-establish thread-scoped security and epoch state: %#v", resumed)
	}
	if _, ok := resumed["goal"]; ok {
		t.Fatalf("resume must not inherit active goal: %#v", resumed["goal"])
	}
	todos, _ := resumed["todos"].(map[string]any)
	if todos["threadId"] != "thr_resume" {
		t.Fatalf("resume todos were not rebound: %#v", todos)
	}
	resumedTodo := todos["items"].([]any)[0].(map[string]any)
	if resumedTodo["status"] != "canceled" || resumedTodo["statusReasonCode"] != "user_canceled" {
		t.Fatalf("resume lost terminal todo lifecycle: %#v", resumedTodo)
	}
	turns, _ := resumed["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	userInput, _ := items[1].(map[string]any)
	if turn["status"] != "completed" || userInput["status"] != "cancelled" {
		t.Fatalf("resume should settle in-flight state: turn=%#v item=%#v", turn, userInput)
	}
}

func TestOrdinaryForkAndResumeStripExecutionAuthorityButPreserveClosedToolPair(t *testing.T) {
	now := time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC)
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_source", TurnID: "turn_tool", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: now,
	})
	args := []byte(`{"path":"SOURCE_ARGUMENT_SENTINEL"}`)
	sourceCallID := threadTestHostToolCallID("call_source")
	caseCallID := threadTestHostToolCallID("call_case")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: sourceCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash(args), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	if err := json.Unmarshal(grantBody, &grantRecord); err != nil {
		t.Fatal(err)
	}
	hostProjection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionHostStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "tool_completed", Status: "completed", Code: "tool_completed",
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
	}
	caseProjection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionCaseSourceStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "case_source_private", Status: "completed", Code: "case_source_result_private",
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
	}
	caseToolName := "mcp__analytix-fund-analysis__query_transactions"
	caseGrantID := domainsecurity.SHA256Hex([]byte("fork-case-source-grant"))
	caseProof, err := domaintoolresult.NewCaseSourceBindingProofV1(domaintoolresult.CaseSourceBindingInputV1{
		ToolName: caseToolName, ToolCallID: caseCallID, ContextDigest: securityContext.ContextDigest,
		ContextEpoch: securityContext.ContextEpoch, ExecutionGrantID: caseGrantID,
		Projection: caseProjection, IsError: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := map[string]any{
		"id": "thr_source", "title": "Ordinary source", "pendingApprovalIds": []any{"APPROVAL_HANDLE_SENTINEL"},
		"pendingUserInputIds": []any{"INPUT_HANDLE_SENTINEL"},
		"turns": []any{map[string]any{
			"id": "turn_tool", "threadId": "thr_source", "status": "completed", "securityContext": securityContext,
			"items": []any{
				map[string]any{
					"id": "call_item", "kind": "tool_call", "role": "assistant", "status": "completed", "threadId": "thr_source", "turnId": "turn_tool",
					"toolName": "read", "callId": sourceCallID, "arguments": map[string]any{"path": "SOURCE_ARGUMENT_SENTINEL"},
					"createdAt": now.Format(time.RFC3339Nano), "finishedAt": now.Add(time.Second).Format(time.RFC3339Nano),
					"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
					"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
				},
				map[string]any{
					"id": "transition_item", "kind": "execution_grant_transition", "threadId": "thr_source", "turnId": "turn_tool",
					"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
					"executionGrantId": grant.GrantID, "parentGrantId": "PARENT_GRANT_SENTINEL", "executionGrant": grantRecord,
				},
				map[string]any{
					"id": "result_item", "kind": "tool_result", "role": "tool", "status": "completed", "threadId": "thr_source", "turnId": "turn_tool",
					"toolName": "read", "callId": sourceCallID, "isError": false,
					"createdAt": now.Add(time.Second).Format(time.RFC3339Nano), "finishedAt": now.Add(time.Second).Format(time.RFC3339Nano),
					"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
					"executionGrantId": grant.GrantID, "hostEvidenceSettlement": map[string]any{"id": "SETTLEMENT_SENTINEL"},
					"output": domaintoolresult.PublicToolResultProjectionRecordV1(hostProjection),
				},
				map[string]any{
					"id": domaintoolresult.ToolResultItemIDV1("turn_tool", caseCallID), "kind": "tool_result", "role": "tool", "status": "completed", "threadId": "thr_source", "turnId": "turn_tool",
					"toolName": caseToolName, "callId": caseCallID, "isError": false,
					"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": caseGrantID,
					domaintoolresult.CaseSourceBindingProofFieldV1: domaintoolresult.CaseSourceBindingProofRecordV1(caseProof),
					"output": domaintoolresult.PublicToolResultProjectionRecordV1(caseProjection),
				},
				map[string]any{
					"id": "approval_item", "kind": "approval", "status": "pending", "threadId": "thr_source", "turnId": "turn_tool",
					"approvalId": "APPROVAL_HANDLE_SENTINEL", "continuationReceiptId": "CONTINUATION_RECEIPT_SENTINEL",
				},
				map[string]any{
					"id": "input_item", "kind": "user_input", "status": "pending", "threadId": "thr_source", "turnId": "turn_tool",
					"inputId": "INPUT_HANDLE_SENTINEL", "continuationReceiptId": "CONTINUATION_RECEIPT_SENTINEL",
				},
			},
		}},
	}
	corrupt := contracts.CloneMap(source)
	corrupt["generalTerminalPublicationArchive"] = map[string]any{"archiveDigest": "GENERAL_ARCHIVE_SENTINEL"}
	if fork, err := BuildFork(ForkInput{Source: corrupt, ForkID: "thr_rejected", ParentThreadID: "thr_source", Now: now.Format(time.RFC3339Nano)}); err == nil || fork != nil {
		t.Fatalf("fork stripped rather than rejected corrupt terminal authority: fork=%#v err=%v", fork, err)
	}
	derived := map[string]map[string]any{}
	fork, err := BuildFork(ForkInput{Source: source, ForkID: "thr_fork", ParentThreadID: "thr_source", Now: now.Add(time.Hour).Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	derived["fork"] = fork
	resume, err := BuildResume(ResumeInput{Source: source, ThreadID: "thr_resume", SessionID: "thr_source", Now: now.Add(time.Hour).Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	derived["resume"] = resume

	for name, projected := range derived {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(projected)
			for _, sentinel := range []string{
				"SOURCE_ARGUMENT_SENTINEL", grant.GrantID, securityContext.ContextDigest, "PARENT_GRANT_SENTINEL",
				"SETTLEMENT_SENTINEL", "APPROVAL_HANDLE_SENTINEL", "INPUT_HANDLE_SENTINEL", "CONTINUATION_RECEIPT_SENTINEL",
				"GENERAL_OUTBOX_SENTINEL", "GENERAL_ARCHIVE_SENTINEL", "GENERAL_BINDING_SENTINEL", "GENERAL_EVENT_SENTINEL",
			} {
				if strings.Contains(string(body), sentinel) {
					t.Fatalf("derived history retained source execution authority %q: %s", sentinel, body)
				}
			}
			turn := projected["turns"].([]any)[0].(map[string]any)
			if turn["securityContext"] != nil || turn["pendingApprovalIds"] != nil || turn["pendingUserInputIds"] != nil {
				t.Fatalf("derived turn retained frozen or pending authority: %#v", turn)
			}
			items := turn["items"].([]any)
			if len(items) != 5 {
				t.Fatalf("grant transition was not dropped: %#v", items)
			}
			call := items[0].(map[string]any)
			if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(call["arguments"]); err != nil {
				t.Fatalf("derived tool call did not preserve a closed pairing placeholder: %v", err)
			}
			result := items[1].(map[string]any)
			projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
			if err != nil || projection.ProjectionKind != domaintoolresult.ProjectionHostStatus {
				t.Fatalf("derived tool result lost its closed host status: projection=%#v err=%v", projection, err)
			}
			caseResult := items[2].(map[string]any)
			caseStatus, err := domaintoolresult.ParsePublicToolResultProjectionV1(caseResult["output"])
			if err != nil || caseStatus.ProjectionKind != domaintoolresult.ProjectionWithheld {
				t.Fatalf("derived case source status retained source authority: projection=%#v err=%v", caseStatus, err)
			}
			if items[3].(map[string]any)["status"] != "expired" || items[4].(map[string]any)["status"] != "cancelled" {
				t.Fatalf("derived pending gates were not terminalized: %#v", items)
			}
		})
	}
}

func TestForkAndResumeCannotRebindGeneralTerminalPublication(t *testing.T) {
	source, _, sentinel := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	source["title"] = "Canonical source"
	for name, derived := range map[string]func() (map[string]any, error){
		"fork": func() (map[string]any, error) {
			return BuildFork(ForkInput{
				Source: source, ForkID: "thread-derived-fork", ParentThreadID: "thread-public-terminal", Now: "2026-07-15T01:00:00Z",
			})
		},
		"resume": func() (map[string]any, error) {
			return BuildResume(ResumeInput{
				Source: source, ThreadID: "thread-derived-resume", SessionID: "thread-public-terminal", Now: "2026-07-15T01:00:00Z",
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			thread, err := derived()
			body, _ := json.Marshal(thread)
			if err != nil || strings.Contains(string(body), sentinel) || strings.Contains(string(body), "generalTerminal") ||
				strings.Contains(string(body), "assistant_text") {
				t.Fatalf("derived thread rebound source terminal authority: body=%s err=%v", body, err)
			}
		})
	}
}

func TestCaseForkAndResumeDoNotRebindAcceptedFinalAuthority(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case", TurnID: "turn_case", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	source := map[string]any{
		"id": "thr_case", "title": "Case source", "workspace": "/workspace",
		"turns": []any{map[string]any{
			"id": "turn_case", "threadId": "thr_case", "status": "completed", "securityContext": securityContext,
			"acceptedFinal": map[string]any{"schemaVersion": 2, "recordDigest": "signed-old-thread"},
			"items": []any{
				map[string]any{"kind": "user_message", "threadId": "thr_case", "turnId": "turn_case", "text": "核验案件"},
				map[string]any{"kind": "assistant_text", "threadId": "thr_case", "turnId": "turn_case", "text": "案件事实", "acceptedFinal": map[string]any{"schemaVersion": 2}},
				map[string]any{"kind": "tool_result", "threadId": "thr_case", "turnId": "turn_case", "output": "raw case data", "executionGrantId": "grant-a", "hostEvidenceSettlement": map[string]any{"settlementId": "SETTLEMENT_AUTHORITY_SENTINEL"}},
			},
		}},
	}
	for name, derive := range map[string]func() error{
		"fork": func() error {
			_, err := BuildFork(ForkInput{Source: source, ForkID: "thr_fork", ParentThreadID: "thr_case", Now: "2026-07-11T00:00:00Z"})
			return err
		},
		"resume": func() error {
			_, err := BuildResume(ResumeInput{Source: source, ThreadID: "thr_resume", SessionID: "thr_case", Now: "2026-07-11T00:00:00Z"})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := derive(); !errors.Is(err, ErrCaseDerivationAdmissionAuthorityRequired) {
				t.Fatalf("%s did not fail closed before case authority could be rebound: %v", name, err)
			}
		})
	}
	for name, derive := range map[string]func() (map[string]any, error){
		"authorized fork": func() (map[string]any, error) {
			return BuildFork(AuthorizeCaseForkV1(ForkInput{
				Source: source, ForkID: "thr_authorized_fork", ParentThreadID: "thr_case", Now: "2026-07-11T00:00:00Z",
			}))
		},
		"authorized resume": func() (map[string]any, error) {
			return BuildResume(AuthorizeCaseResumeV1(ResumeInput{
				Source: source, ThreadID: "thr_authorized_resume", SessionID: "thr_case", Now: "2026-07-11T00:00:00Z",
			}))
		},
	} {
		t.Run(name, func(t *testing.T) {
			thread, err := derive()
			body, _ := json.Marshal(thread)
			for _, forbidden := range []string{"案件事实", "raw case data", "acceptedFinal", "securityContext", "executionGrantId", "SETTLEMENT_AUTHORITY_SENTINEL"} {
				if strings.Contains(string(body), forbidden) {
					t.Fatalf("authorized case derivation retained %q: %s", forbidden, body)
				}
			}
			if err != nil || !strings.Contains(string(body), "核验案件") ||
				stringField(thread, "historyAuthority") != CaseBoundaryOnlyHistoryAuthority {
				t.Fatalf("authorized case derivation did not preserve only the isolated user shell: body=%s err=%v", body, err)
			}
		})
	}
	for name, derive := range map[string]func() error{
		"authorized fork wrong parent": func() error {
			_, err := BuildFork(AuthorizeCaseForkV1(ForkInput{
				Source: source, ForkID: "thr_wrong_parent", ParentThreadID: "thr_other", Now: "2026-07-11T00:00:00Z",
			}))
			return err
		},
		"authorized side fork": func() error {
			_, err := BuildFork(AuthorizeCaseForkV1(ForkInput{
				Source: source, ForkID: "thr_side", ParentThreadID: "thr_case", Relation: "side", Now: "2026-07-11T00:00:00Z",
			}))
			return err
		},
		"authorized resume cross workspace": func() error {
			_, err := BuildResume(AuthorizeCaseResumeV1(ResumeInput{
				Source: source, ThreadID: "thr_cross_workspace", SessionID: "thr_case", Workspace: "/other", Now: "2026-07-11T00:00:00Z",
			}))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := derive(); !errors.Is(err, ErrCaseDerivationAdmissionAuthorityRequired) {
				t.Fatalf("case derivation admission mismatch was accepted: %v", err)
			}
		})
	}
}

func TestCaseForkAndResumeCannotLaunderLegacyFactsOrTodos(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case", TurnID: "turn_legacy", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	source := map[string]any{
		"id": "thr_case", "title": "Case source", "securityState": securityContext,
		"preview": "PREVIEW_AMOUNT_SENTINEL_4200000",
		"todos":   map[string]any{"items": []any{map[string]any{"content": "TODO_QUOTE_SENTINEL_7654321"}}},
		"turns": []any{map[string]any{
			"id": "turn_legacy", "threadId": "thr_case", "status": "completed",
			"items": []any{
				map[string]any{"id": "user", "kind": "user_message", "text": "核验案件", "summary": "NESTED_RELATION_SENTINEL"},
				map[string]any{"id": "assistant", "kind": "assistant_text", "text": "ASSISTANT_AMOUNT_SENTINEL_4200000"},
				map[string]any{"id": "tool", "kind": "tool_result", "output": "TOOL_ACCOUNT_SENTINEL_622202"},
			},
		}},
	}
	for name, derive := range map[string]func() error{
		"fork": func() error {
			_, err := BuildFork(ForkInput{Source: source, ForkID: "thr_fork", ParentThreadID: "thr_case", Now: "2026-07-11T00:00:00Z"})
			return err
		},
		"resume": func() error {
			_, err := BuildResume(ResumeInput{Source: source, ThreadID: "thr_resume", SessionID: "thr_case", Now: "2026-07-11T00:00:00Z"})
			return err
		},
	} {
		if err := derive(); !errors.Is(err, ErrCaseDerivationAdmissionAuthorityRequired) {
			t.Fatalf("%s could launder legacy case content: %v", name, err)
		}
	}
}

func TestCaseDerivationDoesNotInheritCompactionAuthority(t *testing.T) {
	for _, projection := range []string{"authority_only_v1", "user_only_untrusted_v1", "compaction_authority_v1"} {
		for _, operation := range []string{"fork", "resume"} {
			t.Run(operation+"/"+projection, func(t *testing.T) {
				source := map[string]any{
					"id": "source", "historyAuthority": CaseBoundaryOnlyHistoryAuthority,
					"turns": []any{map[string]any{
						"id": "turn", "threadId": "source", "status": "completed",
						"caseHistoryProjection": projection, "items": []any{},
						"acceptedFinalView": map[string]any{"sourceOnly": true},
					}},
				}
				var derived map[string]any
				var err error
				if operation == "fork" {
					derived, err = BuildFork(AuthorizeCaseForkV1(ForkInput{
						Source: source, ForkID: "target", ParentThreadID: "source", Now: "2026-07-11T00:00:00Z",
					}))
				} else {
					derived, err = BuildResume(AuthorizeCaseResumeV1(ResumeInput{
						Source: source, ThreadID: "target", SessionID: "source", Now: "2026-07-11T00:00:00Z",
					}))
				}
				if err != nil {
					t.Fatal(err)
				}
				turn := derived["turns"].([]any)[0].(map[string]any)
				if _, present := turn["caseHistoryProjection"]; present {
					t.Fatal("new derived user shell retained source compaction authority")
				}
				if _, present := turn["acceptedFinalView"]; present {
					t.Fatal("new derived user shell retained source accepted final authority")
				}
				if err := ValidateCaseDerivedHistoryTurnV1("target", turn); err != nil {
					t.Fatal(err)
				}
				if source["turns"].([]any)[0].(map[string]any)["caseHistoryProjection"] != projection {
					t.Fatal("derivation changed original history")
				}
				if source["turns"].([]any)[0].(map[string]any)["acceptedFinalView"] == nil {
					t.Fatal("derivation removed original accepted final view")
				}
			})
		}
	}
}

func TestForkPreservesSealedConversationSummaryWithoutExecutionAuthority(t *testing.T) {
	facts, err := domainthread.SealCompactionSummaryFactsV1(domainthread.CompactionSummaryFactsV1{CompactionID: "compaction_source", Summary: "The user requested continuing the recorded task.", SourceDigest: strings.Repeat("a", 64), SourceItemIDs: []string{"item_source"}, ShadowedItemCount: 1, ReplacedTokens: 50, FinishedAt: "2026-10-09T00:00:01Z"})
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": facts.CompactionID, "compactionId": facts.CompactionID, "threadId": "source", "turnId": "turn_source", "kind": "compaction", "role": "system", "status": "completed", "auto": false,
		"createdAt": "2026-10-09T00:00:00Z", "finishedAt": facts.FinishedAt, "schemaVersion": 5, "providerHistoryProjectionVersion": 3, "summaryKind": "conversation", "summary": facts.Summary, "summaryDigest": facts.SummaryDigest,
		"checkpointDigest": facts.CheckpointDigest, "sourceDigest": facts.SourceDigest, "sourceItemIds": []any{"item_source"}, "shadowedItemCount": 1, "replacedTokens": 50, "tokenCountKind": "estimated", "tokenEstimatorVersion": 1,
		"reasoningExcluded": true, "caseFactsExcluded": true, "toolPayloadsExcluded": true, "pinnedConstraints": []any{"user: preserve recent turns"}, "digestMarker": "sha256:" + facts.SourceDigest[:12]}
	item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
	before := contracts.CloneMap(item)
	cloned := CloneItemForThread(item, "child", "2026-10-10T00:00:00Z")
	if !domainevent.ValidGeneralCompactionProviderHistoryItemV5(cloned) || cloned["threadId"] != "child" || cloned["summary"] != facts.Summary || cloned["checkpointDigest"] != facts.CheckpointDigest || cloned["compactionId"] != facts.CompactionID || !reflect.DeepEqual(before, contracts.CloneMap(item)) {
		t.Fatal("fork lost or invented summary facts")
	}
	for _, key := range []string{"securityContext", "contextEpochSnapshot", "contextDigest", "executionGrant"} {
		if _, ok := cloned[key]; ok {
			t.Fatal("summary clone gained execution authority")
		}
	}
	item["summary"] = "tampered summary"
	if CloneItemForThread(item, "child", "2026-10-10T00:00:00Z") != nil {
		t.Fatal("tampered historical summary revived")
	}
}
