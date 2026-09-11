package mcp

import (
	"encoding/json"
	"testing"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestValidateToolResultOutputRequiresExactStructuredContent(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","properties":{"amount":{"type":"integer"}},"required":["amount"],"additionalProperties":false}}},"required":["rows"],"additionalProperties":false}`)
	valid := json.RawMessage(`{"content":[{"type":"text","text":"ok"}],"structuredContent":{"rows":[{"amount":9007199254740993}]}}`)
	if err := ValidateToolResultOutput(losslessResultForValidation(valid), schema); err != nil {
		t.Fatalf("valid exact result was rejected: %v", err)
	}
	for name, raw := range map[string]json.RawMessage{
		"missing structured": json.RawMessage(`{"content":[]}`),
		"wrong type":         json.RawMessage(`{"content":[],"structuredContent":{"rows":"none"}}`),
		"unknown field":      json.RawMessage(`{"content":[],"structuredContent":{"rows":[],"secret":true}}`),
		"duplicate envelope": json.RawMessage(`{"content":[],"structuredContent":{"rows":[]},"structuredContent":{"rows":[{"amount":1}]}}`),
		"duplicate output":   json.RawMessage(`{"content":[],"structuredContent":{"rows":[],"rows":[{"amount":1}]}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateToolResultOutput(losslessResultForValidation(raw), schema); err == nil {
				t.Fatal("invalid result was accepted")
			}
		})
	}
	forged := losslessResultForValidation(valid)
	forged.RawSHA256 = domainsecurity.SHA256Hex([]byte("different"))
	if err := ValidateToolResultOutput(forged, schema); err == nil {
		t.Fatal("forged raw result hash was accepted")
	}
}

func TestParseCallToolResultAcceptsStandardMetaAndLastModified(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	raw := json.RawMessage(`{
		"content":[
			{
				"type":"text",
				"text":"source result",
				"annotations":{"audience":["assistant"],"priority":0.75,"lastModified":"2026-07-11T01:02Z"},
				"_meta":{"source_record_id":"row-1"}
			},
			{
				"type":"resource",
				"resource":{"uri":"case://evidence/row-1","mimeType":"application/json","text":"{}","_meta":{"sha256":"untrusted-source-value"}},
				"_meta":{"transport_hint":"untrusted"}
			},
			{
				"type":"resource_link",
				"uri":"case://evidence/row-1",
				"name":"row-1",
				"icons":[{"src":"data:image/png;base64,cG5n","mimeType":"image/png","sizes":["16x16"],"theme":"dark"}]
			}
		],
		"structuredContent":{"ok":true},
		"_meta":{"progressToken":"untrusted-token","io.modelcontextprotocol/related-task":{"taskId":"task-1"}}
	}`)
	if err := ValidateToolResultOutput(losslessResultForValidation(raw), schema); err != nil {
		t.Fatalf("standard CallToolResult metadata was rejected: %v", err)
	}
}

func TestParseCallToolResultAcceptsResourceLinkIcons(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	raw := json.RawMessage(`{"content":[{"type":"resource_link","uri":"case://evidence/row-1","name":"row-1","icons":[{"src":"https://example.invalid/icon.png","mimeType":"image/png","sizes":["16x16","any"],"theme":"light"}]}],"structuredContent":{"ok":true}}`)
	if err := ValidateToolResultOutput(losslessResultForValidation(raw), schema); err != nil {
		t.Fatalf("standard resource_link icons were rejected: %v", err)
	}
}

func TestCallToolErrorMayOmitStructuredContent(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	raw := json.RawMessage(`{"content":[{"type":"text","text":"source rejected the query"}],"isError":true}`)
	observation, err := InspectToolResultOutput(losslessResultForValidation(raw), schema)
	if err != nil {
		t.Fatalf("standard MCP tool error without structuredContent was rejected: %v", err)
	}
	if !observation.IsError || observation.SemanticStatus != "failure" {
		t.Fatalf("tool-reported error was not preserved as semantic failure: %#v", observation)
	}
}

func TestCallToolResultRequiresContent(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	for name, raw := range map[string]json.RawMessage{
		"missing": json.RawMessage(`{"structuredContent":{"ok":true}}`),
		"null":    json.RawMessage(`{"content":null,"structuredContent":{"ok":true}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateToolResultOutput(losslessResultForValidation(raw), schema); err == nil {
				t.Fatal("CallToolResult without a content array was accepted")
			}
		})
	}
}

func TestInvalidStructuredContentCannotEraseRemoteIsError(t *testing.T) {
	raw := json.RawMessage(`{
		"content":[],
		"isError":true,
		"blocker":"SOURCE_FAILED",
		"_meta":{"analytix_tool_outcome":{"safeToAnswer":false}},
		"structuredContent":[]
	}`)
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{"rows":{"type":"array","items":{"type":"object","additionalProperties":false}}},
		"required":["rows"],
		"additionalProperties":false
	}`)
	observation, err := InspectToolResultOutput(losslessResultForValidation(raw), schema)
	if err == nil {
		t.Fatal("invalid structured content was accepted")
	}
	if !observation.IsError || observation.SemanticStatus != "failure" || observation.Blocker != "SOURCE_FAILED" ||
		observation.ReportedSafeToAnswer == nil || *observation.ReportedSafeToAnswer {
		t.Fatalf("schema rejection erased exact negative remote assertions: %#v", observation)
	}
}

func TestValidateToolResultOutputRejectsInvalidOuterEnvelope(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	for name, raw := range map[string]json.RawMessage{
		"unknown field":              json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"unexpected":true}`),
		"case variant":               json.RawMessage(`{"content":[],"StructuredContent":{"ok":true}}`),
		"string isError":             json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"isError":"false"}`),
		"array meta":                 json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"_meta":[]}`),
		"fractional progress token":  json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"_meta":{"progressToken":1.5}}`),
		"invalid related task":       json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"_meta":{"io.modelcontextprotocol/related-task":{"id":"task-1"}}}`),
		"object content":             json.RawMessage(`{"structuredContent":{"ok":true},"content":{}}`),
		"content missing type":       json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"text":"ok"}]}`),
		"content coerced text":       json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"type":"text","text":123}]}`),
		"content extra field":        json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"type":"text","text":"ok","secret":true}]}`),
		"content unknown type":       json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"type":"video","data":"x"}]}`),
		"content malformed image":    json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"type":"image","data":"x"}]}`),
		"content malformed resource": json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"type":"resource","resource":{"uri":"file:///x","text":"a","blob":"b"}}]}`),
		"resource icon theme":        json.RawMessage(`{"structuredContent":{"ok":true},"content":[{"type":"resource_link","uri":"case://x","name":"x","icons":[{"src":"icon.png","theme":"system"}]}]}`),
		"string safeToAnswer":        json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"safeToAnswer":"true"}`),
		"object evidence receipts":   json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"evidenceReceipts":{}}`),
		"array semantic status":      json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"semanticStatus":[]}`),
		"null isError":               json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"isError":null}`),
		"null semantic status":       json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"semanticStatus":null}`),
		"null evidence receipts":     json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"evidenceReceipts":null}`),
		"scalar evidence receipt":    json.RawMessage(`{"content":[],"structuredContent":{"ok":true},"evidenceReceipts":["fake"]}`),
		"null content":               json.RawMessage(`{"structuredContent":{"ok":true},"content":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateToolResultOutput(losslessResultForValidation(raw), schema); err == nil {
				t.Fatal("invalid outer MCP result envelope was accepted")
			}
		})
	}
}

func TestMalformedBase64MCPContentRejected(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	for name, raw := range map[string]json.RawMessage{
		"image alphabet": json.RawMessage(`{"content":[{"type":"image","mimeType":"image/png","data":"%%%"}],"structuredContent":{"ok":true}}`),
		"image mime":     json.RawMessage(`{"content":[{"type":"image","mimeType":"audio/mpeg","data":"cG5n"}],"structuredContent":{"ok":true}}`),
		"audio padding":  json.RawMessage(`{"content":[{"type":"audio","mimeType":"audio/mpeg","data":"YQ==="}],"structuredContent":{"ok":true}}`),
		"resource blob":  json.RawMessage(`{"content":[{"type":"resource","resource":{"uri":"file:///evidence.bin","blob":"not-base64"}}],"structuredContent":{"ok":true}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateToolResultOutput(losslessResultForValidation(raw), schema); err == nil {
				t.Fatal("malformed base64 MCP content was accepted")
			}
		})
	}
	valid := json.RawMessage(`{"content":[{"type":"image","mimeType":"image/png","data":"cG5n"}],"structuredContent":{"ok":true}}`)
	if err := ValidateToolResultOutput(losslessResultForValidation(valid), schema); err != nil {
		t.Fatalf("valid bounded base64 MCP content was rejected: %v", err)
	}
}

func TestInspectToolResultOutputOnlyAllowsMonotonicDowngrade(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"semanticStatus":{"type":"string"},
			"isError":{"type":"boolean"},
			"blocker":{"type":"string"},
			"partialCoverage":{"type":"object","properties":{"paginationComplete":{"type":"boolean"}},"additionalProperties":false},
			"data":{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object","additionalProperties":false}},"truncated":{"type":"boolean"}},"additionalProperties":false}
		},
		"additionalProperties":false
	}`)
	tests := []struct {
		name        string
		raw         json.RawMessage
		status      string
		isError     bool
		wantBlock   bool
		wantPartial bool
	}{
		{
			name:   "structured failure wins over meta success",
			raw:    json.RawMessage(`{"content":[],"structuredContent":{"semanticStatus":"failure","isError":true},"_meta":{"analytix_tool_outcome":{"semanticStatus":"success","safeToAnswer":true}}}`),
			status: "failure", isError: true,
		},
		{
			name:   "blocker cannot be upgraded",
			raw:    json.RawMessage(`{"content":[],"structuredContent":{"semanticStatus":"success","blocker":"SOURCE_INCOMPLETE"},"_meta":{"analytix_tool_outcome":{"semanticStatus":"success"}}}`),
			status: "blocked", isError: true, wantBlock: true,
		},
		{
			name:   "safeToAnswer false cannot be upgraded",
			raw:    json.RawMessage(`{"content":[],"structuredContent":{"semanticStatus":"success"},"safeToAnswer":false}`),
			status: "blocked", isError: true,
		},
		{
			name:   "partial coverage cannot be upgraded",
			raw:    json.RawMessage(`{"content":[],"structuredContent":{"semanticStatus":"success","partialCoverage":{"paginationComplete":false}},"_meta":{"analytix_tool_outcome":{"semanticStatus":"success"}}}`),
			status: "partial", wantPartial: true,
		},
		{
			name:   "nested truncation cannot be upgraded",
			raw:    json.RawMessage(`{"content":[],"structuredContent":{"semanticStatus":"success","data":{"rows":[],"truncated":true}},"_meta":{"analytix_tool_outcome":{"semanticStatus":"success"}}}`),
			status: "partial", wantPartial: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation, err := InspectToolResultOutput(losslessResultForValidation(test.raw), schema)
			if err != nil {
				t.Fatal(err)
			}
			if observation.SemanticStatus != test.status || observation.IsError != test.isError || (observation.Blocker != "") != test.wantBlock || (len(observation.PartialCoverage) > 0) != test.wantPartial {
				t.Fatalf("non-monotonic observation: %#v", observation)
			}
		})
	}
	malformedMeta := json.RawMessage(`{"content":[],"structuredContent":{"semanticStatus":"success"},"_meta":{"analytix_tool_outcome":{"partialCoverage":[]}}}`)
	if _, err := InspectToolResultOutput(losslessResultForValidation(malformedMeta), schema); err == nil {
		t.Fatal("malformed analytix tool outcome metadata was accepted")
	}
}

func TestInspectToolResultOutputPreservesExactTopLevelAssertionsForHostAudit(t *testing.T) {
	raw := json.RawMessage(`{
		"content":[],
		"structuredContent":{"ok":true},
		"safeToAnswer":false,
		"semanticStatus":"blocked",
		"blocker":{"code":"SOURCE_NOT_READY"},
		"partialCoverage":{"coverageStatus":"partial"},
		"evidenceReceipts":[{"receiptId":"remote-candidate"}],
		"_meta":{
			"analytix_evidence_ledger":{"status":"unsupported"},
			"analytix_tool_outcome":{
				"reportedSemanticStatus":"success",
				"caseId":"case-remote",
				"contextEpoch":99,
				"datasetSnapshotId":"snapshot-remote",
				"serverIdentity":"server-remote"
			}
		}
	}`)
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`)
	lossless := losslessResultForValidation(raw)
	observation, err := InspectToolResultOutput(lossless, schema)
	if err != nil {
		t.Fatal(err)
	}
	if observation.RawSHA256 != lossless.RawSHA256 || observation.ReportedSafeToAnswer == nil || *observation.ReportedSafeToAnswer ||
		observation.SemanticStatus != "blocked" || !observation.IsError || observation.Blocker != "SOURCE_NOT_READY" ||
		observation.ReportedSemanticStatus != "success" || observation.ReportedCaseID != "case-remote" ||
		observation.ReportedContextEpoch != 99 || observation.ReportedDatasetSnapshotID != "snapshot-remote" ||
		observation.ReportedServerIdentity != "server-remote" || len(observation.CandidateEvidenceReceipts) != 1 {
		t.Fatalf("exact top-level MCP assertions were lost: %#v", observation)
	}
	ledger, _ := observation.UntrustedMeta["analytix_evidence_ledger"].(map[string]any)
	if ledger["status"] != "unsupported" {
		t.Fatalf("exact MCP _meta was lost from the host-private observation: %#v", observation.UntrustedMeta)
	}
}

func losslessResultForValidation(raw json.RawMessage) domainmcp.LosslessToolResult {
	return domainmcp.LosslessToolResult{RawResult: raw, RawSHA256: domainsecurity.SHA256Hex(raw)}
}
