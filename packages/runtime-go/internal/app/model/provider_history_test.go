package model

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestProviderHistoryFromThreadDropsLegacyDuplicateAndMissingToolResults(t *testing.T) {
	const account = "6222020202020202020"
	thread := map[string]any{
		"turns": []any{
			map[string]any{
				"items": []any{
					map[string]any{"kind": "user_message", "text": "inspect files"},
					map[string]any{"kind": "tool_call", "callId": "provider_call_" + account, "toolName": "read_file", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
					map[string]any{"kind": "tool_result", "callId": "provider_call_" + account, "toolName": "read_file", "output": "FILE-RESULT"},
					map[string]any{"kind": "tool_call", "callId": "dup", "toolName": "grep", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
					map[string]any{"kind": "tool_result", "callId": "dup", "toolName": "grep", "output": "GREP-RESULT"},
					map[string]any{"kind": "tool_call", "toolName": "search", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
					map[string]any{"kind": "tool_result", "toolName": "search", "output": "SEARCH-RESULT"},
				},
			},
		},
	}

	messages := ProviderHistoryFromThread(thread)
	body, _ := json.Marshal(messages)
	if len(messages) != 1 || messages[0].Role != "user" || strings.Contains(string(body), account) ||
		strings.Contains(string(body), "FILE-RESULT") || strings.Contains(string(body), "GREP-RESULT") || strings.Contains(string(body), "SEARCH-RESULT") {
		t.Fatalf("legacy tool identities or results entered provider history: %s", body)
	}
}

func TestProviderHistorySkipsDiscardedAbortedTurnAndKeepsCompaction(t *testing.T) {
	compaction := providerHistoryCompactionV3()
	thread := map[string]any{
		"turns": []any{
			map[string]any{
				"status":  "aborted",
				"discard": true,
				"items": []any{
					map[string]any{"kind": "user_message", "text": "discard me"},
				},
			},
			map[string]any{
				"id": "turn-kept",
				"items": []any{
					compaction,
					map[string]any{"id": "item_turn-kept_assistant", "turnId": "turn-kept", "kind": "assistant_text", "status": "completed", "text": "UNBOUND_ASSISTANT_SENTINEL"},
				},
			},
		},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 1 {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if !strings.Contains(messages[0].Content, domainevent.GeneralCompactionSummaryTextV3) {
		t.Fatalf("compaction not retained: %#v", messages)
	}
	if strings.Contains(messages[0].Content, "discard me") || strings.Contains(messages[0].Content, "UNBOUND_ASSISTANT_SENTINEL") {
		t.Fatalf("discarded aborted turn leaked into history: %#v", messages)
	}
}

func TestProviderHistoryRejectsForgedReasoningExcludedClaim(t *testing.T) {
	thread := map[string]any{
		"turns": []any{map[string]any{
			"id": "turn-forged-proof",
			"items": []any{
				map[string]any{
					"kind":                    "compaction",
					"summary":                 `{"reasoning_content":"PRIVATE_REASONING_SENTINEL"}`,
					"schemaVersion":           float64(2),
					"reasoningExcluded":       true,
					"reasoningExclusionProof": "sha256:forged",
				},
				map[string]any{"id": "item_turn-forged-proof_assistant", "turnId": "turn-forged-proof", "kind": "assistant_text", "status": "completed", "text": "public answer"},
			},
		}},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 0 {
		t.Fatalf("caller-claimed reasoning proof must not enter provider history: %#v", messages)
	}
}

func TestProviderHistoryRejectsLegacyCompactionWithoutReasoningExclusionProof(t *testing.T) {
	thread := map[string]any{
		"turns": []any{map[string]any{
			"id": "turn-legacy-compaction",
			"items": []any{
				map[string]any{"kind": "compaction", "summary": "assistant_reasoning: PRIVATE_REASONING_SENTINEL"},
				map[string]any{"id": "item_turn-legacy-compaction_assistant", "turnId": "turn-legacy-compaction", "kind": "assistant_text", "status": "completed", "text": "public answer"},
			},
		}},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 0 {
		t.Fatalf("legacy compaction must not enter provider history: %#v", messages)
	}
}

func providerHistoryCompactionV3() map[string]any {
	sourceDigest := domainsecurity.SHA256Hex([]byte("provider-history-compaction-source"))
	item := map[string]any{
		"id": "compaction-provider-history", "turnId": "turn-kept", "threadId": "thread-provider-history",
		"role": "system", "status": "completed", "createdAt": "2026-07-15T00:00:00Z", "finishedAt": "2026-07-15T00:00:00Z",
		"kind": "compaction", "summary": domainevent.GeneralCompactionSummaryTextV3, "replacedTokens": float64(4), "auto": false,
		"pinnedConstraints": []any{"user: preserve recent turns"}, "sourceDigest": sourceDigest,
		"digestMarker": "sha256:" + sourceDigest[:12], "sourceItemIds": []any{"user-old"}, "schemaVersion": float64(3),
		"reasoningExcluded": true, "assistantProseExcluded": true, "toolPayloadsExcluded": true, "caseFactsExcluded": true,
		"providerHistoryProjectionVersion": float64(1),
	}
	item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
	return item
}

func TestProviderHistoryRequiresCanonicalGeneralTerminalPublication(t *testing.T) {
	thread, sentinel := providerHistoryCanonicalGeneralTerminal(t)
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 1 || messages[0].Role != "assistant" || messages[0].Content != sentinel {
		t.Fatalf("canonical general terminal answer was not replayed: %#v", messages)
	}

	withoutArchive := contracts.CloneMap(thread)
	delete(withoutArchive, domainturnterminal.GeneralTerminalPublicationArchiveFieldV1)
	if messages := ProviderHistoryMessagesFromThread(withoutArchive); len(messages) != 0 {
		t.Fatalf("terminal answer without root archive entered provider history: %#v", messages)
	}

	tampered := contracts.CloneMap(thread)
	tamperedTurn := tampered["turns"].([]any)[0].(map[string]any)
	tamperedTurn["items"].([]any)[0].(map[string]any)["text"] = sentinel + "_TAMPERED"
	if messages := ProviderHistoryMessagesFromThread(tampered); len(messages) != 0 {
		t.Fatalf("digest-mismatched terminal answer entered provider history: %#v", messages)
	}
}

func providerHistoryCanonicalGeneralTerminal(t *testing.T) (map[string]any, string) {
	t.Helper()
	context, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-provider-terminal", TurnID: "turn-provider-terminal", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-15T00:00:00Z"
	sentinel := domainevent.GeneralTerminalCompletedBoundaryTextV1
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(context, "success", "completed", sentinel)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": "item-turn-provider-terminal", "threadId": context.ThreadID, "turnId": context.TurnID,
		"role": "assistant", "status": "completed", "createdAt": committedAt, "finishedAt": committedAt,
		"kind": "assistant_text", "text": sentinel,
		"generalTerminalCASBinding": domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
	}
	itemEvent := map[string]any{
		"kind": "item_completed", "threadId": context.ThreadID, "turnId": context.TurnID,
		"itemId": item["id"], "item": contracts.CloneMap(item), "timestamp": committedAt,
	}
	usageEvent := map[string]any{
		"kind": "usage", "threadId": context.ThreadID, "turnId": context.TurnID, "model": "gpt-5",
		"usage": map[string]any{}, "cacheDiagnostics": map[string]any{}, "timestamp": committedAt, "usageFinalStatus": "completed",
	}
	terminalEvent := map[string]any{
		"kind": "turn_completed", "threadId": context.ThreadID, "turnId": context.TurnID, "status": "completed",
		"timestamp": committedAt, "terminalReason": "success", "generalTerminalCASBindingDigest": binding.BindingDigest,
	}
	commit, err := domainturnterminal.NewGeneralTerminalPublicationCommitV1(
		context, binding, committedAt, []domainturnterminal.GeneralTerminalPublicationDraftV1{
			{Slot: "terminal-item", Draft: itemEvent}, {Slot: "usage", Draft: usageEvent}, {Slot: "terminal", Draft: terminalEvent},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := providerHistorySecurityRecord(context)
	return map[string]any{
		"id": context.ThreadID, "securityState": securityRecord,
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{map[string]any{
			"id": context.TurnID, "threadId": context.ThreadID, "status": "completed", "finishedAt": committedAt,
			"securityContext": securityRecord, "items": []any{item},
			"generalTerminalCASBinding":  domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
			"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
		}},
	}, sentinel
}

func TestProviderHistoryDropsLegacyAssistantProcessDraft(t *testing.T) {
	thread := map[string]any{
		"turns": []any{map[string]any{
			"id": "turn-legacy-process",
			"items": []any{
				map[string]any{
					"id": "item_turn-legacy-process_assistant_process_1", "turnId": "turn-legacy-process",
					"kind": "assistant_text", "status": "completed", "text": "<think>PRIVATE_REASONING_SENTINEL</think>public answer",
				},
			},
		}},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 0 {
		t.Fatalf("legacy assistant process draft entered provider history: %#v", messages)
	}
}

func TestProviderHistoryDropsUnboundSteeringAndSanitizesToolResultImages(t *testing.T) {
	turnID := "turn-steering-tool"
	callItem, resultItem := providerHistoryTestToolPairV1(turnID, "call-1", "screenshot", map[string]any{
		"image_url": "data:image/png;base64,AAAA",
		"nested":    []any{map[string]any{"data_base64": "BBBB"}},
	})
	callItem["reasoningContent"] = "need visual state"
	thread := map[string]any{
		"turns": []any{
			map[string]any{
				"id": turnID,
				"items": []any{
					map[string]any{"kind": "user_message", "delivery": "steer", "text": "  please narrow scope\n", "createdAt": "2026-01-02T03:04:05Z"},
					callItem,
					resultItem,
				},
			},
		},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(messages[0].Parts) != 0 || strings.Contains(messages[0].Content, "need visual state") {
		t.Fatalf("persisted reasoning must not enter provider history: %#v", messages[0])
	}
	if strings.Contains(messages[1].Content, "data:image/") || strings.Contains(messages[1].Content, "BBBB") {
		t.Fatalf("image bytes leaked to provider history: %s", messages[1].Content)
	}
	if !strings.Contains(messages[1].Content, `"messageKey":"legacy_output_withheld"`) {
		t.Fatalf("legacy image result was not withheld: %s", messages[1].Content)
	}
}

func TestProviderHistoryRevalidatesPrivateCaseSourceBindingProof(t *testing.T) {
	turnID := "turn-case-source-provider-history"
	callID := modelTestHostToolCallID("case-source-provider-history")
	toolName := "mcp__analytix-fund-analysis__query_transactions"
	contextDigest := domainsecurity.SHA256Hex([]byte("case-source-provider-context"))
	grantID := domainsecurity.SHA256Hex([]byte("case-source-provider-grant"))
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionCaseSourceStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "case_source_private", Status: "completed",
		Code: "case_source_result_private", PrivatePayloadWithheld: true,
	}
	proof, err := domaintoolresult.NewCaseSourceBindingProofV1(domaintoolresult.CaseSourceBindingInputV1{
		ToolName: toolName, ToolCallID: callID, ContextDigest: contextDigest, ContextEpoch: 7,
		ExecutionGrantID: grantID, Projection: projection, IsError: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(turnID, callID), "turnId": turnID,
		"kind": "tool_call", "status": "completed", "callId": callID, "toolName": toolName,
		"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
	}
	result := map[string]any{
		"id": domaintoolresult.ToolResultItemIDV1(turnID, callID), "threadId": "thread-case-source-provider-history", "turnId": turnID,
		"kind": "tool_result", "role": "tool", "status": "completed", "callId": callID, "toolName": toolName,
		"toolKind": "tool_call", "isError": false, "contextDigest": contextDigest, "contextEpoch": uint64(7),
		"executionGrantId": grantID, domaintoolresult.CaseSourceBindingProofFieldV1: domaintoolresult.CaseSourceBindingProofRecordV1(proof),
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(projection),
	}
	providerThread := func(resultItem map[string]any) map[string]any {
		return map[string]any{"turns": []any{map[string]any{
			"id": turnID, "items": []any{contracts.CloneMap(call), resultItem},
		}}}
	}
	messages := ProviderHistoryFromThread(providerThread(contracts.CloneMap(result)))
	canonicalBody, _ := json.Marshal(messages)
	if len(messages) != 2 || !strings.Contains(messages[1].Content, `"projectionKind":"case_source_status"`) ||
		strings.Contains(string(canonicalBody), domaintoolresult.CaseSourceBindingProofFieldV1) ||
		strings.Contains(string(canonicalBody), proof.Digest) {
		t.Fatalf("canonical case-source provider history was not metadata-only: %s", canonicalBody)
	}

	mutations := map[string]func(map[string]any){
		"missing proof": func(value map[string]any) { delete(value, domaintoolresult.CaseSourceBindingProofFieldV1) },
		"tool rebound":  func(value map[string]any) { value["toolName"] = "mcp__analytix-fund-analysis__other" },
		"call rebound":  func(value map[string]any) { value["callId"] = modelTestHostToolCallID("case-source-rebound") },
		"digest rebound": func(value map[string]any) {
			value["contextDigest"] = domainsecurity.SHA256Hex([]byte("rebound-context"))
		},
		"epoch rebound": func(value map[string]any) { value["contextEpoch"] = uint64(8) },
		"grant rebound": func(value map[string]any) {
			value["executionGrantId"] = domainsecurity.SHA256Hex([]byte("rebound-grant"))
		},
		"status rebound": func(value map[string]any) { value["status"] = "failed" },
		"error rebound":  func(value map[string]any) { value["isError"] = true },
		"proof rebound": func(value map[string]any) {
			value[domaintoolresult.CaseSourceBindingProofFieldV1].(map[string]any)["digest"] = domainsecurity.SHA256Hex([]byte("rebound-proof"))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			rebound := contracts.CloneMap(result)
			mutate(rebound)
			messages := ProviderHistoryFromThread(providerThread(rebound))
			body, _ := json.Marshal(messages)
			caseStatusPresent := false
			for _, message := range messages {
				caseStatusPresent = caseStatusPresent || strings.Contains(message.Content, `"projectionKind":"case_source_status"`)
			}
			if caseStatusPresent ||
				strings.Contains(string(body), domaintoolresult.CaseSourceBindingProofFieldV1) || strings.Contains(string(body), proof.Digest) {
				t.Fatalf("rebound case-source result entered provider history: %s", body)
			}
			if len(messages) == 2 && !strings.Contains(messages[1].Content, `"messageKey":"legacy_output_withheld"`) &&
				!strings.Contains(messages[1].Content, "[no result:") {
				t.Fatalf("paired rebound result was not withheld: %#v", messages)
			}
		})
	}
}

func TestProviderHistoryReplaysOnlyContextBoundOrdinarySteering(t *testing.T) {
	authority := newProviderHistorySteeringAuthority(t)
	prior, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-steer-history", TurnID: "turn-prior", WorkspaceRealPath: "/workspace",
		TenantID: "tenant", UserID: "user", SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")),
		ContextEpoch: 4, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: prior.ThreadID, TurnID: "turn-current", WorkspaceRealPath: prior.WorkspaceRealPath,
		TenantID: prior.TenantID, UserID: prior.UserID, SourceManifestHash: prior.SourceManifestHash,
		ContextEpoch: prior.ContextEpoch, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	clientID := "client-history-steer"
	entryID := domainsteering.EntryIDV1(prior.TurnID, clientID)
	pending, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": "please narrow scope",
		"admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer",
	}, prior.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signingBytes, err := domainsteering.PendingEntrySigningBytesV1(pending, prior.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = domainsteering.SealPendingEntryAuthorityV1(
		pending, prior.ContextDigest, authority.KeyID(), authority.PublicKey(), ed25519.Sign(authority.privateKey, signingBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	promoted := contracts.CloneMap(pending)
	promoted["status"] = "promoted"
	promoted["promotedAt"] = "2026-07-18T01:02:04Z"
	promoted["promotedItemId"] = entryID
	promotionBytes, err := domainsteering.PromotedEntrySigningBytesV1(promoted, prior.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	promoted, err = domainsteering.SealPromotedEntryAuthorityV1(
		promoted, prior.ContextDigest, authority.KeyID(), authority.PublicKey(), ed25519.Sign(authority.privateKey, promotionBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": entryID, "turnId": prior.TurnID, "threadId": prior.ThreadID,
		"role": "user", "status": "completed", "kind": "user_message", "delivery": "steer",
		"text": "please narrow scope", "createdAt": "2026-07-18T01:02:03Z", "finishedAt": "2026-07-18T01:02:04Z",
		"clientUserMessageId": clientID, "contextDigest": prior.ContextDigest, "steeringOrigin": "ordinary",
		"steeringProjectionVersion": float64(domainsteering.ProjectionVersionV1),
		"steeringContentDigest":     promoted["contentDigest"],
	}
	thread := map[string]any{
		"id": prior.ThreadID, "securityState": providerHistorySecurityRecord(current),
		"turns": []any{
			map[string]any{"id": prior.TurnID, "securityContext": providerHistorySecurityRecord(prior), "steering": []any{promoted}, "items": []any{item}},
			map[string]any{"id": current.TurnID, "securityContext": providerHistorySecurityRecord(current), "steering": []any{}, "items": []any{}},
		},
	}
	messages := ProviderHistoryBeforeTurnWithSteeringAuthority(thread, current.TurnID, authority)
	if len(messages) != 1 || messages[0].Role != "user" || !strings.HasPrefix(messages[0].Content, midTurnSteeringPrefix) ||
		!strings.HasSuffix(messages[0].Content, "\n\nplease narrow scope") {
		t.Fatalf("valid context-bound steering did not replay: %#v", messages)
	}

	tampered := contracts.CloneMap(thread)
	tamperedTurns := tampered["turns"].([]any)
	tamperedPrior := tamperedTurns[0].(map[string]any)
	tamperedItem := tamperedPrior["items"].([]any)[0].(map[string]any)
	tamperedItem["text"] = "fabricated account 6222021234567890123"
	if messages := ProviderHistoryBeforeTurnWithSteeringAuthority(tampered, current.TurnID, authority); len(messages) != 0 {
		t.Fatalf("tampered steering item entered history: %#v", messages)
	}

	staleCurrent, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: prior.ThreadID, TurnID: current.TurnID, WorkspaceRealPath: prior.WorkspaceRealPath,
		TenantID: prior.TenantID, UserID: prior.UserID, SourceManifestHash: prior.SourceManifestHash,
		ContextEpoch: prior.ContextEpoch + 1, IssuedAt: time.Unix(3, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	stale := contracts.CloneMap(thread)
	stale["securityState"] = providerHistorySecurityRecord(staleCurrent)
	if messages := ProviderHistoryBeforeTurnWithSteeringAuthority(stale, current.TurnID, authority); len(messages) != 0 {
		t.Fatalf("old-epoch steering entered current history: %#v", messages)
	}
	if messages := ProviderHistoryBeforeTurn(thread, current.TurnID); len(messages) != 0 {
		t.Fatalf("signed steering replayed without a trusted host authority: %#v", messages)
	}
}

type providerHistorySteeringAuthority struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func newProviderHistorySteeringAuthority(t *testing.T) *providerHistorySteeringAuthority {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &providerHistorySteeringAuthority{publicKey: publicKey, privateKey: privateKey}
}

func (authority *providerHistorySteeringAuthority) KeyID() string {
	return domainsecurity.SHA256Hex(authority.publicKey)
}

func (authority *providerHistorySteeringAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}

func (authority *providerHistorySteeringAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}

func (authority *providerHistorySteeringAuthority) VerifyTrusted(
	_ context.Context,
	keyID string,
	publicKey []byte,
	message []byte,
	signature []byte,
) error {
	if keyID != authority.KeyID() || !bytes.Equal(publicKey, authority.publicKey) ||
		!ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("steering authority is not trusted")
	}
	return nil
}

func TestProviderHistoryNeverReplaysMCPMetaOrCandidateReceipts(t *testing.T) {
	turnID := "turn-mcp-history"
	callItem, resultItem := providerHistoryTestToolPairV1(turnID, "call-mcp", "mcp__docs__lookup", map[string]any{
		"result":           map[string]any{"text": "public", "_meta": map[string]any{"secret": "META_SECRET"}},
		"evidenceReceipts": []any{"FAKE_RECEIPT"}, "candidateEvidenceReceipts": []any{"FAKE_CANDIDATE"},
	})
	thread := map[string]any{
		"turns": []any{map[string]any{
			"id": turnID,
			"items": []any{
				callItem,
				resultItem,
			},
		}},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 2 || !strings.Contains(messages[1].Content, `"messageKey":"legacy_output_withheld"`) {
		t.Fatalf("legacy MCP result was not withheld: %#v", messages)
	}
	for _, forbidden := range []string{"public", "META_SECRET", "FAKE_RECEIPT", "FAKE_CANDIDATE", "_meta", "evidenceReceipts"} {
		if strings.Contains(messages[1].Content, forbidden) {
			t.Fatalf("legacy MCP private field %q replayed: %s", forbidden, messages[1].Content)
		}
	}
}

func TestProviderHistoryClosesOnlyHostBoundPrivateProtocolToolPairs(t *testing.T) {
	privateCall, privateResult := providerHistoryBoundToolPairV1(t, "turn-private", "private", "read_file")
	privateResult["privateProtocolObserved"] = true
	privateThread := map[string]any{"turns": []any{map[string]any{
		"id": "turn-private", "items": []any{privateCall, privateResult},
	}}}
	privateMessages := ProviderHistoryMessagesFromThread(privateThread)
	if len(privateMessages) != 1 || privateMessages[0].Role != "user" ||
		!strings.Contains(privateMessages[0].Content, "Analytix host-selected prior tool activity") ||
		strings.Contains(privateMessages[0].Content, `"role":"tool"`) {
		t.Fatalf("private tool pair was not closed into semantic history: %#v", privateMessages)
	}

	ordinaryCall, ordinaryResult := providerHistoryBoundToolPairV1(t, "turn-ordinary", "ordinary", "read_file")
	ordinaryThread := map[string]any{"turns": []any{map[string]any{
		"id": "turn-ordinary", "items": []any{ordinaryCall, ordinaryResult},
	}}}
	ordinaryMessages := ProviderHistoryMessagesFromThread(ordinaryThread)
	if len(ordinaryMessages) != 2 || ordinaryMessages[0].Role != "assistant" ||
		len(ordinaryMessages[0].ToolCalls) != 1 || ordinaryMessages[1].Role != "tool" {
		t.Fatalf("ordinary Anthropic-compatible tool pair lost native history: %#v", ordinaryMessages)
	}

	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"false marker": func(_ map[string]any, result map[string]any) {
			result["privateProtocolObserved"] = false
		},
		"wrong marker type": func(_ map[string]any, result map[string]any) {
			result["privateProtocolObserved"] = "true"
		},
		"grant mismatch": func(_ map[string]any, result map[string]any) {
			result["executionGrantId"] = strings.Repeat("f", 64)
		},
		"context mismatch": func(_ map[string]any, result map[string]any) {
			result["contextDigest"] = strings.Repeat("e", 64)
		},
		"epoch mismatch": func(_ map[string]any, result map[string]any) {
			result["contextEpoch"] = float64(99)
		},
		"missing call grant": func(call map[string]any, _ map[string]any) {
			delete(call, "executionGrant")
		},
	} {
		t.Run(name, func(t *testing.T) {
			call, result := providerHistoryBoundToolPairV1(t, "turn-malformed", name, "read_file")
			result["privateProtocolObserved"] = true
			mutate(call, result)
			thread := map[string]any{"turns": []any{map[string]any{
				"id": "turn-malformed", "items": []any{call, result},
			}}}
			if messages := ProviderHistoryMessagesFromThread(thread); len(messages) != 0 {
				t.Fatalf("malformed private marker retained native tool wire: %#v", messages)
			}
		})
	}
}

func TestProviderHistoryDropsUnsafeRawIdentityEvenWhenLegacyItemIDsAreMissing(t *testing.T) {
	account := "6222020202020202020"
	callID := "provider_call_" + account
	thread := map[string]any{"turns": []any{map[string]any{
		"id": "turn-legacy-empty-id",
		"items": []any{
			map[string]any{
				"id": "", "turnId": "turn-legacy-empty-id", "kind": "tool_call", "callId": callID,
				"toolName": "mcp__docs__lookup", "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
			},
			map[string]any{
				"id": "", "turnId": "turn-legacy-empty-id", "kind": "tool_result", "callId": callID,
				"toolName": "mcp__docs__lookup", "output": map[string]any{"text": account},
			},
		},
	}}}
	messages := ProviderHistoryMessagesFromThread(thread)
	body, _ := json.Marshal(messages)
	if len(messages) != 0 || strings.Contains(string(body), account) {
		t.Fatalf("unsafe legacy provider identity was replayed: %s", body)
	}
}

func providerHistoryTestToolPairV1(turnID, seed, toolName string, output any) (map[string]any, map[string]any) {
	callID := modelTestHostToolCallID(seed)
	return map[string]any{
			"id": domaintoolcall.ToolCallItemIDV1(turnID, callID), "turnId": turnID,
			"kind": "tool_call", "callId": callID, "toolName": toolName,
			"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
		}, map[string]any{
			"id": domaintoolresult.ToolResultItemIDV1(turnID, callID), "turnId": turnID,
			"kind": "tool_result", "callId": callID, "toolName": toolName, "output": output,
		}
}

func providerHistoryBoundToolPairV1(t *testing.T, turnID, seed, toolName string) (map[string]any, map[string]any) {
	t.Helper()
	call, result := providerHistoryTestToolPairV1(
		turnID,
		seed,
		toolName,
		domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private")),
	)
	context, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-provider-private", TurnID: turnID, WorkspaceRealPath: "/workspace",
		ContextEpoch: 7, IssuedAt: time.Unix(7, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	callID := providerHistoryStringField(call, "callId")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider-private", ServerIdentity: "host:builtin",
		ToolName: toolName, ToolCallID: callID,
		ArgsHash:   domainsecurity.CanonicalJSONHash([]byte(`{}`)),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly:   true, ApprovalState: "not_required",
		IssuedAt: time.Unix(7, 0), ExpiresAt: time.Unix(7, 0).Add(time.Minute),
	})
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	for _, item := range []map[string]any{call, result} {
		item["contextDigest"] = context.ContextDigest
		item["contextEpoch"] = float64(context.ContextEpoch)
		item["executionGrantId"] = grant.GrantID
	}
	call["executionGrant"] = grantRecord
	return call, result
}

func TestSteeringProviderContentIgnoresDynamicTimestamps(t *testing.T) {
	first := SteeringProviderContent("please narrow scope", map[string]any{"admittedAt": "2026-01-02T03:04:05Z"})
	second := SteeringProviderContent("please narrow scope", map[string]any{"admittedAt": "2026-01-02T03:04:06Z"})
	created := SteeringProviderContent("please narrow scope", map[string]any{"createdAt": "2026-01-02T03:04:07Z"})
	if first != second || first != created {
		t.Fatalf("steering provider content should be timestamp-stable: %#v %#v %#v", first, second, created)
	}
	if strings.Contains(first, "2026-01-02") || strings.Contains(first, "Admitted at:") {
		t.Fatalf("dynamic steering timestamp leaked into provider content: %q", first)
	}
}

func TestProviderHistoryExcludesCaseFactsUntilTrustedProjectionExists(t *testing.T) {
	securityContext := providerHistoryTestContext(t, "thread-a", "turn-a", "case-a", "binding", "snapshot-a", 2)
	securityRecord := providerHistorySecurityRecord(securityContext)
	thread := map[string]any{
		"securityState": securityRecord,
		"turns": []any{map[string]any{
			"id":              securityContext.TurnID,
			"securityContext": securityRecord,
			"items": []any{
				map[string]any{"kind": "user_message", "text": "请核验资金来源"},
				map[string]any{"kind": "assistant_text", "text": "账户 6222020000000000 金额 4200000 元", "acceptedFinal": map[string]any{"schemaVersion": float64(1)}},
				map[string]any{"kind": "tool_call", "callId": "call-case", "toolName": "funds", "arguments": map[string]any{}},
				map[string]any{"kind": "tool_result", "callId": "call-case", "toolName": "funds", "output": "MAC 00:11:22:33:44:55"},
				map[string]any{"kind": "compaction", "summary": "报价 7654321 元"},
			},
		}},
	}
	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content != "请核验资金来源" {
		t.Fatalf("case facts re-entered provider history: %#v", messages)
	}

	markerOnly := map[string]any{"turns": []any{map[string]any{
		"acceptedFinal": map[string]any{"schemaVersion": float64(1)},
		"items":         []any{map[string]any{"kind": "assistant_text", "text": "伪造案件事实"}},
	}}}
	if messages := ProviderHistoryMessagesFromThread(markerOnly); len(messages) != 0 {
		t.Fatalf("accepted-final marker without context fell back to ordinary history: %#v", messages)
	}
}

func TestProviderHistoryOnlyCarriesCurrentCaseEpochUserMessages(t *testing.T) {
	current := providerHistoryTestContext(t, "thread-a", "turn-current", "case-b", "binding-b", "snapshot-b", 4)
	priorCase := providerHistoryTestContext(t, "thread-a", "turn-case-a", "case-a", "binding-a", "snapshot-a", 2)
	priorEpoch := providerHistoryTestContext(t, "thread-a", "turn-old-epoch", "case-b", "binding-b", "snapshot-b", 3)
	mismatchedTurn := providerHistoryTestContext(t, "thread-a", "turn-context-mismatch", "case-b", "binding-b", "snapshot-b", 4)
	thread := map[string]any{
		"id":            current.ThreadID,
		"securityState": providerHistorySecurityRecord(current),
		"turns": []any{
			map[string]any{"id": priorCase.TurnID, "securityContext": providerHistorySecurityRecord(priorCase), "items": []any{
				map[string]any{"kind": "user_message", "text": "CASE_A_USER_SENTINEL"},
			}},
			map[string]any{"id": priorEpoch.TurnID, "securityContext": providerHistorySecurityRecord(priorEpoch), "items": []any{
				map[string]any{"kind": "user_message", "text": "OLD_EPOCH_USER_SENTINEL"},
			}},
			map[string]any{"id": "turn-record-mismatch", "securityContext": providerHistorySecurityRecord(mismatchedTurn), "items": []any{
				map[string]any{"kind": "user_message", "text": "MISMATCHED_TURN_USER_SENTINEL"},
			}},
			map[string]any{"id": current.TurnID, "securityContext": providerHistorySecurityRecord(current), "items": []any{
				map[string]any{"kind": "user_message", "text": "CURRENT_CASE_USER_SENTINEL"},
			}},
		},
	}

	messages := ProviderHistoryMessagesFromThread(thread)
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content != "CURRENT_CASE_USER_SENTINEL" {
		t.Fatalf("foreign case, stale epoch, or mismatched turn user text entered provider history: %#v", messages)
	}
}

func TestProviderHistoryBeforeFirstCaseTurnUsesFrozenContextAndExcludesActivePrompt(t *testing.T) {
	current := providerHistoryTestContext(t, "thread-a", "turn-case", "case-a", "binding-a", "snapshot-a", 2)
	unbound := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-before-case", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	thread := map[string]any{
		"id": "thread-a", "securityState": providerHistorySecurityRecord(current),
		"turns": []any{
			map[string]any{"id": unbound.TurnID, "securityContext": providerHistorySecurityRecord(unbound), "items": []any{
				map[string]any{"kind": "user_message", "text": "OLD_UNBOUND_USER_SENTINEL"},
				map[string]any{"kind": "assistant_text", "text": "OLD_AMOUNT_SENTINEL_4200000"},
				map[string]any{"kind": "tool_call", "callId": "old", "toolName": "funds", "arguments": map[string]any{"account": "622202"}},
				map[string]any{"kind": "tool_result", "callId": "old", "toolName": "funds", "output": "MAC_SENTINEL"},
				map[string]any{"kind": "compaction", "summary": "QUOTE_SENTINEL"},
			}},
			map[string]any{"id": current.TurnID, "securityContext": providerHistorySecurityRecord(current), "items": []any{
				map[string]any{"kind": "user_message", "text": "ACTIVE_CASE_PROMPT_SENTINEL"},
			}},
		},
	}
	messages := ProviderHistoryBeforeTurn(thread, current.TurnID)
	if len(messages) != 0 {
		t.Fatalf("stale pre-case history or active prompt entered first case request: %#v", messages)
	}
}

func TestProviderHistoryQuarantinesAuditOnlyV1CaseUserText(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", CaseID: "case-v1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-v1")), DatasetSnapshotID: "snapshot-v1",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-v1")), ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	record := providerHistorySecurityRecord(legacy)
	thread := map[string]any{"securityState": record, "turns": []any{map[string]any{
		"id": legacy.TurnID, "securityContext": record,
		"items": []any{map[string]any{"kind": "user_message", "text": "AUDIT_ONLY_CASE_PROMPT"}},
	}}}
	if messages := ProviderHistoryMessagesFromThread(thread); len(messages) != 0 {
		t.Fatalf("audit-only V1 case text entered live provider history: %#v", messages)
	}
}

func providerHistoryTestContext(t *testing.T, threadID, turnID, caseID, binding, snapshot string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace", TenantID: "tenant", UserID: "user",
		CaseID: caseID, CaseBindingHash: domainsecurity.SHA256Hex([]byte(binding)), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID(snapshot),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: epoch, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func providerHistorySecurityRecord(securityContext domainsecurity.TurnSecurityContext) map[string]any {
	body, _ := json.Marshal(securityContext)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
