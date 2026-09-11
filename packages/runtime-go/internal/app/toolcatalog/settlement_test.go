package toolcatalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func toolCatalogTestHostCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.tool-catalog-test-call/v1\x00" + seed))
	id, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return id
}

func TestSettleToolResultBuildsDurableItemEventAndModelReply(t *testing.T) {
	callID := toolCatalogTestHostCallID("read-1")
	records, err := SettleToolResult(ToolResultInput{
		ThreadID:   "thr_1",
		TurnID:     "turn_1",
		CreatedAt:  "2026-01-01T00:00:00Z",
		FinishedAt: "2026-01-01T00:00:01Z",
		Call: domainmodel.ToolCall{
			ID:        callID,
			Name:      "read",
			Arguments: json.RawMessage(`{"path":"README.md"}`),
		},
		Projection: BuildPublicToolResultProjectionV1("read", map[string]any{"content": "hello"}, false),
		IsError:    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if records.Status != "completed" {
		t.Fatalf("status mismatch: %#v", records)
	}
	if records.ResultItemID != ToolResultItemID("turn_1", callID) {
		t.Fatalf("result item id mismatch: %#v", records)
	}
	if records.ResultItem["toolKind"] != "tool_call" || records.ResultItem["isError"] != false {
		t.Fatalf("result item mismatch: %#v", records.ResultItem)
	}
	for _, key := range []string{"contextDigest", "contextEpoch", "executionGrantId"} {
		if _, ok := records.ResultItem[key]; ok {
			t.Fatalf("authority-free result emitted %s: %#v", key, records.ResultItem)
		}
		if _, ok := records.Event[key]; ok {
			t.Fatalf("authority-free event emitted %s: %#v", key, records.Event)
		}
	}
	if records.Event["kind"] != "tool_call_finished" || records.Event["itemId"] != records.ResultItemID {
		t.Fatalf("event mismatch: %#v", records.Event)
	}
	projection, ok := records.ResultItem["output"].(map[string]any)
	if !ok || projection["schemaVersion"] != float64(1) || projection["projectionKind"] != "host_status" || projection["privatePayloadWithheld"] != true {
		t.Fatalf("public projection mismatch: %#v", records.ResultItem["output"])
	}
	encoded, _ := json.Marshal(records.ResultItem)
	if string(encoded) == "" || !json.Valid(encoded) || bytes.Contains(encoded, []byte("hello")) {
		t.Fatalf("raw tool payload reached durable result: %s", encoded)
	}
	if records.ModelReply.Role != "tool" || records.ModelReply.Name != "read" || records.ModelReply.ToolCallID != callID {
		t.Fatalf("model reply mismatch: %#v", records.ModelReply)
	}
	message := ToolResultMessage(domainmodel.ToolCall{ID: callID, Name: "read"}, "hello")
	if message.Content != "hello" || message.ToolCallID != callID {
		t.Fatalf("tool result message mismatch: %#v", message)
	}
}

func TestSettleToolResultDoesNotSilentlyDowngradePartialAuthority(t *testing.T) {
	records, err := SettleToolResult(ToolResultInput{
		ThreadID: "thr", TurnID: "turn", Call: domainmodel.ToolCall{ID: toolCatalogTestHostCallID("partial-authority"), Name: "read"},
		Projection:    BuildPublicToolResultProjectionV1("read", map[string]any{"content": "private"}, false),
		ContextDigest: "partial-digest",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"contextDigest", "contextEpoch", "executionGrantId"} {
		if _, ok := records.ResultItem[key]; !ok {
			t.Fatalf("partial authority was silently downgraded by omitting %s: %#v", key, records.ResultItem)
		}
		if _, ok := records.Event[key]; !ok {
			t.Fatalf("partial event authority was silently downgraded by omitting %s: %#v", key, records.Event)
		}
	}
}

func TestSettleToolResultMarksErrorsFailed(t *testing.T) {
	records, err := SettleToolResult(ToolResultInput{
		ThreadID:   "thr",
		TurnID:     "turn",
		Call:       domainmodel.ToolCall{ID: toolCatalogTestHostCallID("write-failed"), Name: "write"},
		Projection: BuildPublicToolResultProjectionV1("write", map[string]any{"code": "failed"}, true),
		IsError:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if records.Status != "failed" || records.ResultItem["status"] != "failed" || records.ResultItem["toolKind"] != "file_change" {
		t.Fatalf("failed result mismatch: %#v", records)
	}
}

func TestSettleToolResultRejectsInvalidIdentityProjectionAndLifecycleWithoutRecords(t *testing.T) {
	validCall := domainmodel.ToolCall{ID: toolCatalogTestHostCallID("closed-validation"), Name: "read"}
	invalidProjection := BuildPublicToolResultProjectionV1(validCall.Name, nil, false)
	invalidProjection.MessageKey = "provider supplied lifecycle"
	for name, input := range map[string]ToolResultInput{
		"missing thread": {
			TurnID: "turn", Call: validCall,
			Projection: BuildPublicToolResultProjectionV1(validCall.Name, nil, false),
		},
		"missing turn": {
			ThreadID: "thread", Call: validCall,
			Projection: BuildPublicToolResultProjectionV1(validCall.Name, nil, false),
		},
		"missing tool name": {
			ThreadID: "thread", TurnID: "turn", Call: domainmodel.ToolCall{ID: validCall.ID},
			Projection: BuildPublicToolResultProjectionV1(validCall.Name, nil, false),
		},
		"provider call id": {
			ThreadID: "thread", TurnID: "turn", Call: domainmodel.ToolCall{ID: "provider_call_6222020202020202020", Name: validCall.Name},
			Projection: BuildPublicToolResultProjectionV1(validCall.Name, nil, false),
		},
		"invalid projection": {
			ThreadID: "thread", TurnID: "turn", Call: validCall, Projection: invalidProjection,
		},
		"error completed": {
			ThreadID: "thread", TurnID: "turn", Call: validCall,
			Projection: BuildPublicToolResultProjectionV1(validCall.Name, nil, false), IsError: true,
		},
		"success failed": {
			ThreadID: "thread", TurnID: "turn", Call: validCall,
			Projection: BuildPublicToolResultProjectionV1(validCall.Name, nil, true),
		},
		"forged unknown": {
			ThreadID: "thread", TurnID: "turn", Call: validCall,
			Projection: domaintoolresult.LegacyWithheldProjectionV1(), IsError: true,
		},
		"case projection without private outcome": {
			ThreadID: "thread", TurnID: "turn", Call: validCall,
			Projection: domaintoolresult.PublicToolResultProjectionV1{
				SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionCaseSourceStatus,
				Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "case_source_private", Status: "completed",
				Code: "case_source_result_private", PrivatePayloadWithheld: true,
			},
		},
		"invalid evidence marker": {
			ThreadID: "thread", TurnID: "turn", Call: validCall,
			Projection:         BuildPublicToolResultProjectionV1(validCall.Name, nil, false),
			EvidenceSettlement: &domainevidence.HostEvidenceSettlementMarker{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			records, err := SettleToolResult(input)
			if !errors.Is(err, ErrToolResultSettlementInvalid) || !reflect.DeepEqual(records, ToolResultRecords{}) {
				t.Fatalf("records=%#v err=%v", records, err)
			}
		})
	}
}

func TestSettleCaseSourceUsesPrivateOutcomeAuthorityAndEmitsClosedPublicShape(t *testing.T) {
	toolName := "mcp__analytix-fund-analysis__query_transactions"
	call := domainmodel.ToolCall{
		ID: toolCatalogTestHostCallID("private-case-settlement"), Name: toolName,
		Arguments: json.RawMessage(`{"account":"PRIVATE_REVERSE_MAP_SENTINEL"}`),
	}
	contextDigest := domainsecurity.SHA256Hex([]byte("private-case-context"))
	grantID := domainsecurity.SHA256Hex([]byte("private-case-grant"))
	serverIdentity := toolCatalogTestMCPIdentity(t, "analytix-fund-analysis", 7)
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: toolName, ToolCallID: call.ID, ContextDigest: contextDigest, ExecutionGrantID: grantID,
		CaseID: "case-private", ContextEpoch: 9, DatasetSnapshotID: "snapshot-private", ServerIdentity: serverIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess,
		Data: map[string]any{
			"account": "6222020202020202020", "AuthorityRef": "PRIVATE_AUTHORITY_REF",
			"path": "/private/case.sqlite", "sql": "SELECT * FROM private_case", "providerBody": "PRIVATE_PROVIDER_BODY",
		},
		IssuedAt: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
	})
	projection := BuildPublicToolResultProjectionV1(toolName, map[string]any{
		"code": "case_source_result_private", "executed": true, "isError": false,
	}, false)
	valid := ToolResultInput{
		ThreadID: "thread-private", TurnID: "turn-private", Call: call, Projection: projection, IsError: false,
		ContextDigest: contextDigest, ContextEpoch: outcome.ContextEpoch, ExecutionGrantID: grantID,
		CaseID: outcome.CaseID, DatasetSnapshotID: outcome.DatasetSnapshotID, ServerIdentity: outcome.ServerIdentity,
		PrivateCaseOutcome: &outcome,
	}
	records, err := SettleToolResult(valid)
	if err != nil {
		t.Fatal(err)
	}
	proof, proofErr := domaintoolresult.ParseCaseSourceBindingProofV1(
		records.ResultItem[domaintoolresult.CaseSourceBindingProofFieldV1],
	)
	eventItem, _ := records.Event["item"].(map[string]any)
	eventProof, eventProofErr := domaintoolresult.ParseCaseSourceBindingProofV1(
		eventItem[domaintoolresult.CaseSourceBindingProofFieldV1],
	)
	if proofErr != nil || eventProofErr != nil || proof != eventProof {
		t.Fatalf("settlement did not bind the private durable item/event: proof=%#v event=%#v errs=%v/%v", proof, eventProof, proofErr, eventProofErr)
	}
	publicItem := domaintoolresult.PublicToolResultItemRecordV1(records.ResultItem)
	publicEvent := map[string]any{"kind": "tool_call_finished", "item": publicItem}
	for name, value := range map[string]any{"result": records.ResultItem["output"], "event": publicEvent, "history": publicItem} {
		body, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, forbidden := range []string{
			"caseOutcome", domaintoolresult.CaseSourceBindingProofFieldV1, proof.Digest,
			contextDigest, grantID, outcome.CaseID, outcome.DatasetSnapshotID,
			outcome.ServerIdentity, "6222020202020202020", "PRIVATE_AUTHORITY_REF", "/private/case.sqlite",
			"SELECT * FROM private_case", "PRIVATE_PROVIDER_BODY", "PRIVATE_REVERSE_MAP_SENTINEL",
		} {
			if bytes.Contains(body, []byte(forbidden)) {
				t.Fatalf("%s public projection leaked %q: %s", name, forbidden, body)
			}
		}
	}
	parsed, parseErr := domaintoolresult.ParsePublicToolResultProjectionV1(records.ResultItem["output"])
	if parseErr != nil || parsed.ProjectionKind != domaintoolresult.ProjectionCaseSourceStatus || parsed.MessageKey != "case_source_private" {
		t.Fatalf("closed case projection mismatch: projection=%#v err=%v", parsed, parseErr)
	}

	mutations := map[string]func(*ToolResultInput){
		"tool name":       func(input *ToolResultInput) { input.Call.Name = "mcp__analytix-fund-analysis__other" },
		"call id":         func(input *ToolResultInput) { input.Call.ID = toolCatalogTestHostCallID("wrong-private-case") },
		"context digest":  func(input *ToolResultInput) { input.ContextDigest = domainsecurity.SHA256Hex([]byte("wrong-context")) },
		"context epoch":   func(input *ToolResultInput) { input.ContextEpoch++ },
		"grant":           func(input *ToolResultInput) { input.ExecutionGrantID = domainsecurity.SHA256Hex([]byte("wrong-grant")) },
		"case":            func(input *ToolResultInput) { input.CaseID = "wrong-case" },
		"snapshot":        func(input *ToolResultInput) { input.DatasetSnapshotID = "wrong-snapshot" },
		"server":          func(input *ToolResultInput) { input.ServerIdentity = "host:builtin" },
		"isError":         func(input *ToolResultInput) { input.IsError = true },
		"missing outcome": func(input *ToolResultInput) { input.PrivateCaseOutcome = nil },
		"safe-to-answer": func(input *ToolResultInput) {
			cloned := outcome
			cloned.SafeToAnswer = true
			input.PrivateCaseOutcome = &cloned
		},
		"receipt": func(input *ToolResultInput) {
			cloned := outcome
			cloned.EvidenceReceiptIDs = []string{"forged"}
			input.PrivateCaseOutcome = &cloned
		},
		"transport-semantic": func(input *ToolResultInput) {
			cloned := outcome
			cloned.TransportStatus = domainevidence.TransportFailure
			input.PrivateCaseOutcome = &cloned
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if records, err := SettleToolResult(input); !errors.Is(err, ErrToolResultSettlementInvalid) || !reflect.DeepEqual(records, ToolResultRecords{}) {
				t.Fatalf("private authority mismatch settled: records=%#v err=%v", records, err)
			}
		})
	}
}

func TestSettleToolResultKeepsSemanticTerminalStatusInsideClosedProjection(t *testing.T) {
	call := domainmodel.ToolCall{ID: toolCatalogTestHostCallID("closed-terminal"), Name: "read"}
	for name, projection := range map[string]domaintoolresult.PublicToolResultProjectionV1{
		"blocked":   domaintoolresult.WithheldProjectionV1("blocked", "tool_output_private"),
		"cancelled": domaintoolresult.WithheldProjectionV1("cancelled", "tool_output_private"),
		"unknown":   domaintoolresult.OutcomeUnknownAfterRestartProjectionV1(),
	} {
		t.Run(name, func(t *testing.T) {
			records, err := SettleToolResult(ToolResultInput{
				ThreadID: "thread", TurnID: "turn", Call: call, Projection: projection, IsError: true,
			})
			output, parseErr := domaintoolresult.ParsePublicToolResultProjectionV1(records.ResultItem["output"])
			if err != nil || parseErr != nil || records.Status != "failed" || records.ResultItem["status"] != "failed" || output.Status != name {
				t.Fatalf("records=%#v err=%v", records, err)
			}
		})
	}
}
