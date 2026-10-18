package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

func TestBuildCompactionReplacesOldTurnsWithSummary(t *testing.T) {
	turns := []any{
		map[string]any{"id": "turn_1", "items": []any{
			map[string]any{"id": "user_1", "kind": "user_message", "text": "hello world"},
			map[string]any{"id": "reasoning_1", "kind": "assistant_reasoning", "text": "private scratchpad"},
			map[string]any{"id": "err_1", "kind": "error", "message": "skip"},
		}},
		map[string]any{"id": "turn_2", "items": []any{
			map[string]any{"id": "tool_1", "kind": "tool_call", "toolName": "read_file", "reasoningContent": "private tool reasoning"},
		}},
		map[string]any{"id": "turn_3", "items": []any{map[string]any{"id": "keep_1", "kind": "assistant_text"}}},
		map[string]any{"id": "turn_4", "items": []any{map[string]any{"id": "keep_2", "kind": "assistant_text"}}},
	}
	plan := BuildCompaction(CompactionInput{
		ThreadID: "thr/one",
		Model:    "gpt-compatible",
		Turns:    turns,
		Reason:   "manual",
		Stamp:    123,
		Now:      "2026-07-03T00:00:00Z",
	})
	if !plan.Changed {
		t.Fatal("expected compaction to change the turn list")
	}
	if plan.Result.ThreadID != "thr/one" || plan.Result.CompactedTurns != 2 || plan.Result.RemainingTurns != 3 {
		t.Fatalf("result mismatch: %#v", plan.Result)
	}
	if plan.Result.SourceDigest == "" || plan.Result.DigestMarker != "sha256:"+plan.Result.SourceDigest[:12] {
		t.Fatalf("digest mismatch: %#v", plan.Result)
	}
	if len(plan.Result.SourceItemIDs) != 1 || plan.Result.SourceItemIDs[0] != "user_1" {
		t.Fatalf("source ids mismatch: %#v", plan.Result.SourceItemIDs)
	}
	compactTurn, _ := plan.NextTurns[0].(map[string]any)
	if compactTurn["id"] != "turn_thr_one_compaction_123" || compactTurn["model"] != "gpt-compatible" {
		t.Fatalf("compact turn mismatch: %#v", compactTurn)
	}
	items, _ := compactTurn["items"].([]any)
	summaryItem, _ := items[0].(map[string]any)
	if summaryItem["schemaVersion"] != float64(3) || summaryItem["reasoningExcluded"] != true ||
		summaryItem["assistantProseExcluded"] != true || summaryItem["toolPayloadsExcluded"] != true ||
		summaryItem["caseFactsExcluded"] != true || summaryItem["providerHistoryProjectionVersion"] != float64(1) {
		t.Fatalf("compaction reasoning exclusion proof missing: %#v", summaryItem)
	}
	if proof, _ := summaryItem["reasoningExclusionProof"].(string); proof == "" || proof != domainevent.ReasoningExclusionProof(summaryItem) {
		t.Fatalf("compaction reasoning exclusion proof is invalid: %#v", summaryItem)
	}
	if summaryItem["summary"] != GeneralCompactionSummaryTextV3 || strings.Contains(summaryItem["summary"].(string), "hello world") ||
		strings.Contains(summaryItem["summary"].(string), "skip") || strings.Contains(summaryItem["summary"].(string), "private scratchpad") {
		t.Fatalf("summary mismatch: %#v", summaryItem["summary"])
	}
}

func TestCompactionSourceIDsExcludeLegacyProviderIdentity(t *testing.T) {
	const account = "6222020202020202020"
	items := compactionSourceItems([]any{
		map[string]any{
			"id": "item_tool_turn-provider_call_" + account, "kind": "tool_call", "toolName": "read",
			"turnId": "turn-legacy", "callId": "provider_call_" + account,
		},
		map[string]any{
			"id": "item_result_turn-provider_call_" + account, "kind": "tool_result", "toolName": "read",
			"turnId": "turn-legacy", "callId": "provider_call_" + account, "isError": true,
			"output": map[string]any{"account": account},
		},
	})
	body, _ := json.Marshal(items)
	if strings.Contains(string(body), account) || len(compactionSourceItemIDs(items)) != 0 {
		t.Fatalf("legacy provider identity entered compaction source: %s ids=%#v", body, compactionSourceItemIDs(items))
	}
}

func TestCompactionSourceDropsLegacyAssistantProcessDraft(t *testing.T) {
	const sentinel = "LEGACY_COMPACTION_DRAFT_SENTINEL"
	items := compactionSourceItems([]any{map[string]any{
		"id": "turn_1", "items": []any{
			map[string]any{
				"id": "item_turn_1_assistant_process_1", "turnId": "turn_1", "kind": "assistant_text",
				"status": "completed", "text": sentinel,
			},
			map[string]any{"id": "user_1", "turnId": "turn_1", "kind": "user_message", "status": "completed", "text": "safe request"},
		},
	}})
	body, _ := json.Marshal(items)
	if strings.Contains(string(body), sentinel) || !strings.Contains(string(body), "safe request") {
		t.Fatalf("legacy assistant process compaction projection mismatch: %s", body)
	}
}

func TestCompactionSourcePreservesAcceptedAssistantBytesAndDropsMalformedMarkup(t *testing.T) {
	t.Parallel()
	const accepted = "  accepted assistant bytes\n"
	items := compactionSourceItems(
		[]any{
			map[string]any{"id": "turn-exact", "items": []any{
				map[string]any{"id": "assistant-exact", "kind": "assistant_text", "text": accepted},
			}},
			map[string]any{"id": "turn-malformed", "items": []any{
				map[string]any{"id": "assistant-malformed", "kind": "assistant_text", "text": "public</thi"},
			}},
		},
		map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1{
			"turn-exact": {
				Terminal: true,
				Commit:   domainturnterminal.GeneralTerminalPublicationCommitV1{TerminalItemID: "assistant-exact"},
			},
			"turn-malformed": {
				Terminal: true,
				Commit:   domainturnterminal.GeneralTerminalPublicationCommitV1{TerminalItemID: "assistant-malformed"},
			},
		},
	)
	if len(items) != 1 || items[0]["text"] != accepted {
		t.Fatalf("compaction changed accepted bytes or retained malformed text: %#v", items)
	}
}

func TestCompactionReasoningExcludedProofCannotBeCallerClaim(t *testing.T) {
	turns := []any{
		map[string]any{"id": "turn_1", "items": []any{
			map[string]any{
				"id":                "forged_compaction",
				"kind":              "compaction",
				"schemaVersion":     float64(2),
				"reasoningExcluded": true,
				"summary":           `{"reasoning_content":"PRIVATE_REASONING_SENTINEL"}`,
			},
			map[string]any{"id": "safe_user", "kind": "user_message", "text": "public request"},
		}},
		map[string]any{"id": "turn_2", "items": []any{}},
		map[string]any{"id": "turn_3", "items": []any{}},
	}
	plan := BuildCompaction(CompactionInput{ThreadID: "thr_1", Turns: turns, Stamp: 1, Now: "2026-07-10T00:00:00Z"})
	if !plan.Changed || len(plan.Result.SourceItemIDs) != 1 || plan.Result.SourceItemIDs[0] != "safe_user" {
		t.Fatalf("forged proof/private compaction must be excluded from source: %#v", plan.Result)
	}
	if strings.Contains(plan.Result.Summary, "PRIVATE_REASONING_SENTINEL") {
		t.Fatalf("private content was laundered through compaction: %q", plan.Result.Summary)
	}
}

func TestCompactionRestrictedEvidenceCannotInfluencePublicDigest(t *testing.T) {
	build := func(restrictedDigest string) CompactionPlan {
		turns := []any{
			map[string]any{"id": "turn_1", "items": []any{
				map[string]any{"id": "safe_user", "kind": "user_message", "text": "public request"},
				map[string]any{"id": "private_review", "kind": "review", "output": map[string]any{
					"rawArtifactManifestDigest":     restrictedDigest,
					"rawArtifactManifestSha256":     strings.Repeat("b", 64),
					"rawArtifactManifestByteLength": 42,
				}},
			}},
			map[string]any{"id": "turn_2", "items": []any{}},
			map[string]any{"id": "turn_3", "items": []any{map[string]any{"id": "tail_1", "kind": "user_message", "text": "tail one"}}},
			map[string]any{"id": "turn_4", "items": []any{map[string]any{"id": "tail_2", "kind": "user_message", "text": "tail two"}}},
		}
		return BuildCompaction(CompactionInput{ThreadID: "thr_restricted", Turns: turns, Stamp: 7, Now: "2026-07-18T00:00:00Z"})
	}
	first := build(strings.Repeat("a", 64))
	second := build(strings.Repeat("c", 64))
	if !first.Changed || !second.Changed || first.Error != nil || second.Error != nil ||
		first.Result.SourceDigest != second.Result.SourceDigest ||
		len(first.Result.SourceItemIDs) != 1 || first.Result.SourceItemIDs[0] != "safe_user" {
		t.Fatalf("restricted evidence influenced compaction: first=%#v second=%#v", first.Result, second.Result)
	}
	for _, plan := range []CompactionPlan{first, second} {
		body, _ := json.Marshal(plan.NextTurns)
		if strings.Contains(string(body), "rawArtifactManifest") || strings.Contains(string(body), "private_review") {
			t.Fatalf("restricted evidence survived compaction: %s", body)
		}
	}
}

func TestCompactionSanitizesRetainedTailTurns(t *testing.T) {
	turns := []any{
		map[string]any{"id": "turn_1", "items": []any{map[string]any{"id": "old_user", "kind": "user_message", "text": "compact me"}}},
		map[string]any{"id": "turn_2", "items": []any{map[string]any{"id": "old_assistant", "kind": "assistant_text", "text": "old public"}}},
		map[string]any{"id": "turn_3", "reasoningContent": "PRIVATE_TURN", "items": []any{
			map[string]any{"id": "tail_reasoning", "kind": "assistant_reasoning", "text": "PRIVATE_ITEM"},
			map[string]any{"id": "tail_assistant", "kind": "assistant_text", "text": "<think>PRIVATE_INLINE</think>tail public"},
		}},
		map[string]any{"id": "turn_4", "items": []any{map[string]any{"id": "tail_user", "kind": "user_message", "text": "literal <think>user example</think>"}}},
	}
	plan := BuildCompaction(CompactionInput{ThreadID: "thr_1", Turns: turns, Stamp: 2, Now: "2026-07-10T00:00:00Z"})
	data, err := json.Marshal(plan.NextTurns)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, forbidden := range []string{"PRIVATE_TURN", "PRIVATE_ITEM", "PRIVATE_INLINE", "reasoningContent", "assistant_reasoning"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("retained compaction tail leaked %q: %s", forbidden, serialized)
		}
	}
	if strings.Contains(serialized, "tail public") || !strings.Contains(serialized, "literal") || !strings.Contains(serialized, "user example") {
		t.Fatalf("unbound assistant text was retained or user text was lost: %s", serialized)
	}
}

func TestCompactionTailDropsOnlyPrivateProtocolToolPairs(t *testing.T) {
	privateCallID := "call_host_" + strings.Repeat("a", 64)
	ordinaryCallID := "call_host_" + strings.Repeat("b", 64)
	projection := domaintoolresult.PublicToolResultProjectionRecordV1(
		domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
	)
	toolPair := func(turnID, callID string, private bool) []any {
		call := map[string]any{
			"id": domaintoolcall.ToolCallItemIDV1(turnID, callID), "turnId": turnID, "threadId": "thread-compact",
			"kind": "tool_call", "status": "completed", "toolName": "read_file", "callId": callID,
			"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
		}
		result := map[string]any{
			"id": domaintoolresult.ToolResultItemIDV1(turnID, callID), "turnId": turnID, "threadId": "thread-compact",
			"kind": "tool_result", "status": "completed", "toolName": "read_file", "callId": callID,
			"isError": false, "output": projection,
		}
		if private {
			result["privateProtocolObserved"] = true
		}
		return []any{call, result}
	}
	privateItems := []any{map[string]any{
		"id": "tail-user-private", "turnId": "turn-tail-private", "threadId": "thread-compact",
		"kind": "user_message", "status": "completed", "text": "private tail",
	}}
	privateItems = append(privateItems, toolPair("turn-tail-private", privateCallID, true)...)
	ordinaryItems := []any{map[string]any{
		"id": "tail-user-ordinary", "turnId": "turn-tail-ordinary", "threadId": "thread-compact",
		"kind": "user_message", "status": "completed", "text": "ordinary tail",
	}}
	ordinaryItems = append(ordinaryItems, toolPair("turn-tail-ordinary", ordinaryCallID, false)...)
	plan := BuildCompaction(CompactionInput{
		ThreadID: "thread-compact", Stamp: 3, Now: "2026-07-31T00:00:00Z",
		Turns: []any{
			map[string]any{"id": "turn-old-1", "items": []any{map[string]any{"id": "old-user-1", "kind": "user_message", "text": "old one"}}},
			map[string]any{"id": "turn-old-2", "items": []any{map[string]any{"id": "old-user-2", "kind": "user_message", "text": "old two"}}},
			map[string]any{"id": "turn-tail-private", "threadId": "thread-compact", "status": "completed", "items": privateItems},
			map[string]any{"id": "turn-tail-ordinary", "threadId": "thread-compact", "status": "completed", "items": ordinaryItems},
		},
	})
	if !plan.Changed || plan.Error != nil {
		t.Fatalf("compaction failed: %#v", plan)
	}
	body, _ := json.Marshal(plan.NextTurns)
	if strings.Contains(string(body), privateCallID) || strings.Contains(string(body), "privateProtocolObserved") {
		t.Fatalf("compaction retained attempt-private tool pair or marker: %s", body)
	}
	if !strings.Contains(string(body), ordinaryCallID) {
		t.Fatalf("compaction dropped ordinary native tool pair: %s", body)
	}
	history := appmodel.ProviderHistoryMessagesFromThread(map[string]any{"turns": plan.NextTurns})
	for _, message := range history {
		for _, call := range message.ToolCalls {
			if call.ID == privateCallID {
				t.Fatalf("private protocol tool pair re-entered compacted provider history: %#v", history)
			}
		}
	}
}

func TestBuildCompactionNoopsWhenTooShortOrNoSourceItems(t *testing.T) {
	short := BuildCompaction(CompactionInput{ThreadID: "thr_1", Turns: []any{map[string]any{}}})
	if short.Changed || short.Result.RemainingTurns != 1 {
		t.Fatalf("short compaction mismatch: %#v", short)
	}
	empty := BuildCompaction(CompactionInput{
		ThreadID: "thr_1",
		Turns: []any{
			map[string]any{"items": []any{map[string]any{"kind": "error"}}},
			map[string]any{"items": []any{}},
			map[string]any{"items": []any{}},
		},
	})
	if empty.Changed || empty.Result.RemainingTurns != 3 {
		t.Fatalf("empty source compaction mismatch: %#v", empty)
	}
}

func TestBuildCompactionDoesNotRecompactItsOwnSummaryLoop(t *testing.T) {
	turns := []any{
		map[string]any{"id": "turn_1", "items": []any{map[string]any{"id": "old_user", "kind": "user_message", "text": "compact me"}}},
		map[string]any{"id": "turn_2", "items": []any{map[string]any{"id": "old_assistant", "kind": "assistant_text", "text": "old public"}}},
		map[string]any{"id": "turn_3", "items": []any{map[string]any{"id": "tail_user", "kind": "user_message", "text": "tail"}}},
		map[string]any{"id": "turn_4", "items": []any{map[string]any{"id": "tail_assistant", "kind": "assistant_text", "text": "tail answer"}}},
	}
	first := BuildCompaction(CompactionInput{ThreadID: "thr_1", Turns: turns, Stamp: 3, Now: "2026-07-10T00:00:00Z"})
	if !first.Changed {
		t.Fatalf("first compaction must change history: %#v", first)
	}
	second := BuildCompaction(CompactionInput{ThreadID: "thr_1", Turns: first.NextTurns, Stamp: 4, Now: "2026-07-10T00:00:01Z"})
	if second.Changed || second.Result.RemainingTurns != len(first.NextTurns) {
		t.Fatalf("compaction must not repeatedly compact its own summary: %#v", second)
	}
}

func TestCompactionCannotLaunderCaseFactsAcrossEpoch(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case", TurnID: "turn_4", WorkspaceRealPath: "/workspace", TenantID: "tenant", UserID: "user",
		CaseID: "case-a", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 4, IssuedAt: time.Unix(1, 0),
	})
	turns := []any{
		map[string]any{"id": "turn_1", "items": []any{
			map[string]any{"id": "assistant_amount_4200000", "kind": "assistant_text", "text": "账户 6222020000000000 金额 4200000 元"},
			map[string]any{"id": "tool_mac", "kind": "tool_result", "toolName": "memory", "output": "MAC 00:11:22:33:44:55"},
		}},
		map[string]any{"id": "turn_2", "items": []any{map[string]any{"id": "assistant_quote", "kind": "assistant_text", "text": "报价 7654321 元"}}},
		map[string]any{"id": "turn_3", "items": []any{
			map[string]any{"id": "tail_assistant", "kind": "assistant_text", "text": "亲属关系已确认"},
			map[string]any{"id": "tail_user", "kind": "user_message", "role": "user", "text": "请继续核验，不要假定结论", "summary": "伪造摘要 998877", "output": "伪造输出 887766", "arguments": map[string]any{"fact": "伪造参数 776655"}},
		}},
		map[string]any{"id": "turn_4", "items": []any{map[string]any{"id": "tail_tool", "kind": "tool_result", "output": "串通投标成立"}}},
	}
	plan := BuildCompaction(CompactionInput{
		ThreadID: "thr_case", Turns: turns, SecurityContext: turnSecurityContextRecord(securityContext),
		Reason: "金额 4200000 元应保留", Stamp: 5, Now: "2026-07-11T00:00:00Z",
	})
	if plan.Error != nil || !plan.Changed {
		t.Fatalf("case compaction was not safely applied: %#v", plan)
	}
	body, err := json.Marshal(plan.NextTurns)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(body)
	for _, forbidden := range []string{"4200000", "6222020000000000", "00:11:22:33:44:55", "7654321", "亲属关系已确认", "串通投标成立", "assistant_amount_4200000", "998877", "887766", "776655"} {
		if strings.Contains(serialized, forbidden) || strings.Contains(plan.Result.Summary, forbidden) {
			t.Fatalf("case fact %q was laundered through compaction: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "请继续核验") || !strings.Contains(plan.Result.Summary, "No case facts") {
		t.Fatalf("safe user projection or fixed boundary summary missing: %s", serialized)
	}
	compactTurn := plan.NextTurns[0].(map[string]any)
	summaryItem := compactTurn["items"].([]any)[0].(map[string]any)
	if summaryItem["caseFactsExcluded"] != true || summaryItem["caseHistoryProjectionVersion"] != float64(1) {
		t.Fatalf("case compaction proof is missing: %#v", summaryItem)
	}
	history := appmodel.ProviderHistoryMessagesFromThread(map[string]any{
		"securityState": turnSecurityContextRecord(securityContext), "turns": plan.NextTurns,
	})
	for _, message := range history {
		for _, forbidden := range []string{"4200000", "6222020000000000", "00:11:22:33:44:55", "7654321", "亲属关系已确认", "串通投标成立"} {
			if strings.Contains(message.Content, forbidden) {
				t.Fatalf("case compaction fact re-entered provider history: %#v", history)
			}
		}
	}

	invalid := BuildCompaction(CompactionInput{ThreadID: "thr_case", Turns: turns, SecurityContext: map[string]any{"threadId": "thr_case"}})
	if invalid.Error == nil || invalid.Changed {
		t.Fatalf("invalid case authority did not fail closed: %#v", invalid)
	}
}

func TestCompactionAcceptedFinalMarkerCannotFallBackToGeneralSummary(t *testing.T) {
	turns := []any{
		map[string]any{"id": "turn_1", "acceptedFinal": map[string]any{"schemaVersion": float64(1)}, "items": []any{
			map[string]any{"id": "forged", "kind": "assistant_text", "text": "账户 6222020000000000 金额 4200000 元"},
		}},
		map[string]any{"id": "turn_2", "items": []any{}},
		map[string]any{"id": "turn_3", "items": []any{}},
	}
	plan := BuildCompaction(CompactionInput{ThreadID: "thr_legacy_case", Turns: turns, Stamp: 7, Now: "2026-07-11T00:00:00Z"})
	if plan.Error == nil || plan.Changed {
		t.Fatalf("accepted-final authority was removed without a trusted archive: %#v", plan)
	}
}

func TestCompactionCannotRemoveEvidenceSettlementAuthority(t *testing.T) {
	turns := []any{
		map[string]any{"id": "turn_1", "items": []any{map[string]any{
			"id": "result_1", "kind": "tool_result", "hostEvidenceSettlement": map[string]any{"settlementId": "settlement-a"},
		}}},
		map[string]any{"id": "turn_2", "items": []any{}},
		map[string]any{"id": "turn_3", "items": []any{}},
	}
	plan := BuildCompaction(CompactionInput{ThreadID: "thr_case", Turns: turns, Stamp: 8, Now: "2026-07-11T00:00:00Z"})
	if plan.Error == nil || plan.Changed {
		t.Fatalf("evidence settlement marker was compacted without an authority archive: %#v", plan)
	}
}

func TestCompactionTailTelemetryScanViewPreservesExactAuthorityV1(t *testing.T) {
	thread, commit, authorities := compactionTelemetryTailFixtureV1(t)
	turn := publicationFixtureTurn(thread)
	if err := domainevent.ValidatePublicRecord(turn); !errors.Is(err, domainevent.ErrRawFailurePersistence) {
		t.Fatalf("generic nested outbox scanner no longer reproduces the boundary: %v", err)
	}
	before, err := json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	beforeOutbox, _ := json.Marshal(turn["generalTerminalPublication"])
	beforeBinding, _ := json.Marshal(turn["generalTerminalCASBinding"])
	beforeItems, _ := json.Marshal(turn["items"])
	value, ok := sanitizeCompactionTailTurn(turn, authorities)
	projected, _ := value.(map[string]any)
	if !ok || projected == nil {
		t.Fatal("validated telemetry outbox was not retained in the compaction tail")
	}
	afterOutbox, _ := json.Marshal(projected["generalTerminalPublication"])
	afterBinding, _ := json.Marshal(projected["generalTerminalCASBinding"])
	afterItems, _ := json.Marshal(projected["items"])
	if !bytes.Equal(beforeOutbox, afterOutbox) || !bytes.Equal(beforeBinding, afterBinding) || !bytes.Equal(beforeItems, afterItems) {
		t.Fatal("compaction scan view changed retained tail items, binding or outbox bytes")
	}
	minimal := map[string]any{"id": commit.ThreadID, "turns": []any{projected}}
	if err := ValidateGeneralTerminalPublicationCommitForThreadV1(minimal, commit.TurnID, commit); err != nil {
		t.Fatalf("retained telemetry outbox is not reconstructable: %v", err)
	}
	after, err := json.Marshal(thread)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("compaction validation changed its original thread payload")
	}
}

func TestCompactionTailTelemetryScanViewRejectsInvalidAuthorityV1(t *testing.T) {
	rewriteUsage := func(t *testing.T, thread map[string]any, mutate func(map[string]any)) {
		t.Helper()
		turn := publicationFixtureTurn(thread)
		commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
		if err != nil {
			t.Fatal(err)
		}
		frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if err != nil {
			t.Fatal(err)
		}
		binding, err := domainturnterminal.ParseGeneralTerminalCASBindingV1(turn["generalTerminalCASBinding"])
		if err != nil {
			t.Fatal(err)
		}
		usage := contracts.CloneMap(commit.UsageEvent)
		mutate(usage)
		itemEvent := AssistantItemCompletedEvent(publicationFixtureAssistant(thread))
		itemEvent["timestamp"] = commit.CommittedAt
		// The domain constructor preserves this legacy open telemetry shape.
		// The existing app authority validator must reject it before qualification.
		replacement, err := domainturnterminal.NewGeneralTerminalPublicationCommitV1(frozen, binding, commit.CommittedAt,
			[]domainturnterminal.GeneralTerminalPublicationDraftV1{
				{Slot: "terminal-item", Draft: itemEvent},
				{Slot: "usage", Draft: usage},
				{Slot: "terminal", Draft: contracts.CloneMap(commit.TerminalEvent)},
			},
		)
		if err != nil {
			t.Fatal("could not prepare the legacy open telemetry rejection fixture")
		}
		turn["generalTerminalPublication"] = domainturnterminal.GeneralTerminalPublicationCommitV1Map(replacement)
	}
	for name, mutate := range map[string]func(*testing.T, map[string]any){
		"unknown outbox field": func(_ *testing.T, thread map[string]any) {
			publicationFixtureTurn(thread)["generalTerminalPublication"].(map[string]any)["unexpected"] = true
		},
		"raw usage field": func(t *testing.T, thread map[string]any) {
			rewriteUsage(t, thread, func(usage map[string]any) {
				usage["cacheDiagnostics"].(map[string]any)["outputPreview"] = "PRIVATE_PROVIDER_SENTINEL"
			})
		},
		"raw usage error": func(t *testing.T, thread map[string]any) {
			rewriteUsage(t, thread, func(usage map[string]any) {
				usage["cacheDiagnostics"].(map[string]any)["providerAttemptTelemetryError"] = "PRIVATE_PROVIDER_FAILURE"
			})
		},
		"missing binding": func(_ *testing.T, thread map[string]any) {
			delete(publicationFixtureTurn(thread), "generalTerminalCASBinding")
		},
		"detached binding": func(t *testing.T, thread map[string]any) {
			turn := publicationFixtureTurn(thread)
			frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			if err != nil {
				t.Fatal(err)
			}
			binding, err := domainturnterminal.NewGeneralTerminalCASBindingV1(frozen, "other public assistant text")
			if err != nil {
				t.Fatal(err)
			}
			turn["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
		},
		"wrong commit digest": func(_ *testing.T, thread map[string]any) {
			publicationFixtureTurn(thread)["generalTerminalPublication"].(map[string]any)["commitDigest"] = strings.Repeat("b", 64)
		},
		"wrong terminal item binding": func(_ *testing.T, thread map[string]any) {
			publicationFixtureAssistant(thread)["generalTerminalCASBinding"].(map[string]any)["bindingDigest"] = strings.Repeat("b", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			thread, _, authorities := compactionTelemetryTailFixtureV1(t)
			mutate(t, thread)
			before, err := json.Marshal(thread)
			if err != nil {
				t.Fatal(err)
			}
			if value, ok := sanitizeCompactionTailTurn(publicationFixtureTurn(thread), authorities); ok || value != nil {
				t.Fatal("invalid outbox authority received compaction scan-view qualification")
			}
			after, err := json.Marshal(thread)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("compaction rejection changed its original thread payload")
			}
		})
	}
}

func compactionTelemetryTailFixtureV1(t *testing.T) (map[string]any, domainturnterminal.GeneralTerminalPublicationCommitV1, map[string]domainturnterminal.GeneralTerminalProjectionAuthorityV1) {
	t.Helper()
	prepared, err := PrepareTerminalCASMutation(generalTerminalTelemetryCASInputV1(t))
	if err != nil || !prepared.Applied {
		t.Fatalf("exact active telemetry fixture could not prepare the compaction tail: %v", err)
	}
	thread := prepared.Thread
	turn := publicationFixtureTurn(thread)
	turn["items"] = append(turn["items"].([]any), map[string]any{
		"id": "user-telemetry-tail", "turnId": stringField(turn, "id"), "threadId": stringField(thread, "id"),
		"kind": "user_message", "role": "user", "status": "completed", "text": "  retained public user text\n",
	})
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive)
	authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
	if err != nil || !authorities[commit.TurnID].Terminal {
		t.Fatalf("telemetry fixture has no exact compaction authority: %v", err)
	}
	return thread, commit, authorities
}
