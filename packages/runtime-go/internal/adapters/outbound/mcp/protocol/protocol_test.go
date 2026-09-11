package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func TestParseJSONRPCAndSSEResponses(t *testing.T) {
	result, id, err := ParseJSONRPCResponseWithID([]byte(`{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`))
	if err != nil || !id.Matches(7) || string(result) != `{"ok":true}` {
		t.Fatalf("json-rpc response mismatch result=%s id=%#v err=%v", string(result), id, err)
	}
	sse := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	result, err = ParseSSEJSONRPCResponse(sse, 7)
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("sse response mismatch result=%s err=%v", string(result), err)
	}
	if _, _, err := ParseJSONRPCResponseWithID([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"boom"}}`)); err == nil {
		t.Fatalf("expected json-rpc error")
	}
}

func TestMalformedSSEFrameCannotPrecedeAcceptedResponse(t *testing.T) {
	malformedThenValid := []byte("data: {not-json}\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	if _, err := ParseSSEJSONRPCResponse(malformedThenValid, 7); err == nil {
		t.Fatal("malformed SSE data frame was ignored before a matching response")
	}
	notificationThenValid := []byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	result, err := ParseSSEJSONRPCResponse(notificationThenValid, 7)
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("valid notification before response was rejected: result=%s err=%v", result, err)
	}
}

func TestMalformedNotificationParamsBeforeAcceptedResponse(t *testing.T) {
	stream := []byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":\"not-structured\"}\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	if _, err := ParseSSEJSONRPCResponse(stream, 7); err == nil {
		t.Fatal("notification with scalar params was ignored before a matching response")
	}
}

func TestToolsListChangedRevokesCatalogBeforeNextExecutionGrant(t *testing.T) {
	stream := []byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\",\"params\":{}}\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	if _, err := ParseSSEJSONRPCResponse(stream, 7); err == nil {
		t.Fatal("tools/list_changed was ignored before accepting a response under the stale catalog")
	}
}

func TestServerCancelledRequestCannotPublishLateResult(t *testing.T) {
	stream := []byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/cancelled\",\"params\":{\"requestId\":7}}\n\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	if _, err := ParseSSEJSONRPCResponse(stream, 7); err == nil {
		t.Fatal("server cancellation was ignored before a late result")
	}
}

func TestParseJSONRPCPreservesTypedErrorClassAndData(t *testing.T) {
	for _, test := range []struct {
		code int
		want string
	}{
		{code: -32601, want: "method_not_found"},
		{code: -32602, want: "invalid_params"},
		{code: -32603, want: "internal_error"},
		{code: -32042, want: "server_error"},
	} {
		body := []byte(`{"jsonrpc":"2.0","id":7,"error":{"code":` + strconv.Itoa(test.code) + `,"message":"untrusted account 6217000012345678901","data":{"kind":"fixture"}}}`)
		_, id, err := ParseJSONRPCResponseWithID(body)
		var typed *domainmcp.JSONRPCError
		if !id.Matches(7) || !errors.As(err, &typed) || typed.Code != test.code || typed.Class() != test.want || string(typed.Data) != `{"kind":"fixture"}` {
			t.Fatalf("typed error mismatch id=%#v err=%#v", id, err)
		}
		if strings.Contains(err.Error(), "6217000012345678901") || strings.Contains(err.Error(), "fixture") {
			t.Fatalf("typed Error leaked message/data: %q", err.Error())
		}
	}
}

func TestParseJSONRPCPreservesNullIDConnectionErrors(t *testing.T) {
	for _, code := range []int{-32700, -32600} {
		body := []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":` + strconv.Itoa(code) + `,"message":"invalid request"}}`)
		_, id, err := ParseJSONRPCResponseWithID(body)
		var typed *domainmcp.JSONRPCError
		if !id.Null || !errors.As(err, &typed) || typed.Code != code {
			t.Fatalf("null-id connection error was lost: id=%#v err=%#v", id, err)
		}
	}
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":null,"result":{}}`,
		`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal"}}`,
	} {
		if _, _, err := ParseJSONRPCResponseWithID([]byte(body)); err == nil {
			t.Fatalf("invalid null-id response was accepted: %s", body)
		}
	}
}

func TestJSONRPCNumericMinusOneIsNotNullID(t *testing.T) {
	result, id, err := ParseJSONRPCResponseWithID([]byte(`{"jsonrpc":"2.0","id":-1,"result":{"ok":true}}`))
	if err != nil || id.Null || !id.Matches(-1) || string(result) != `{"ok":true}` {
		t.Fatalf("numeric -1 was confused with null id: result=%s id=%#v err=%v", result, id, err)
	}
}

func TestParseJSONRPCRejectsMalformedOrAmbiguousEnvelopes(t *testing.T) {
	for name, body := range map[string]string{
		"missing version":          `{ "id":1,"result":{} }`,
		"wrong version":            `{"jsonrpc":"1.0","id":1,"result":{}}`,
		"missing result":           `{"jsonrpc":"2.0","id":1}`,
		"both branches":            `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-32603,"message":"x"}}`,
		"unknown field":            `{"jsonrpc":"2.0","id":1,"result":{},"secret":"x"}`,
		"invalid id":               `{"jsonrpc":"2.0","id":"1","result":{}}`,
		"unknown error":            `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"x","secret":"x"}}`,
		"empty message":            `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":" "}}`,
		"trailing json":            `{"jsonrpc":"2.0","id":1,"result":{}} {}`,
		"duplicate id":             `{"jsonrpc":"2.0","id":1,"id":2,"result":{}}`,
		"nested duplicate":         `{"jsonrpc":"2.0","id":1,"result":{"caseId":"a","caseId":"b"}}`,
		"case variant id":          `{"jsonrpc":"2.0","ID":1,"result":{}}`,
		"case variant error":       `{"jsonrpc":"2.0","id":1,"error":{"Code":-32603,"message":"x"}}`,
		"semantic duplicate error": `{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"Code":-32602,"message":"x"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseJSONRPCResponseWithID([]byte(body)); err == nil {
				t.Fatal("malformed JSON-RPC envelope was accepted")
			}
		})
	}
}

func TestParseServerIdentityRequiresExactFieldCasing(t *testing.T) {
	identity, err := ParseServerIdentity(json.RawMessage(`{"protocolVersion":"2025-11-25","serverInfo":{"name":"analytix_funds","version":"1.0.0","description":"Funds evidence server","futureInfo":{}},"capabilities":{"tools":{}},"futureInitializeExtension":{}}`))
	if err != nil || identity.Name != "analytix_funds" || identity.Version != "1.0.0" {
		t.Fatalf("valid identity failed: identity=%#v err=%v", identity, err)
	}
	for _, body := range []string{
		`{"protocolVersion":"2025-11-25","ServerInfo":{"name":"forged"},"capabilities":{"tools":{}}}`,
		`{"protocolVersion":"2025-11-25","serverInfo":{"Name":"forged","version":"1"},"capabilities":{"tools":{}}}`,
		`{"protocolVersion":"2025-11-25","serverInfo":{"name":"real","Name":"forged","version":"1"},"capabilities":{"tools":{}}}`,
	} {
		if _, err := ParseServerIdentity(json.RawMessage(body)); err == nil {
			t.Fatalf("case-variant identity field was accepted: %s", body)
		}
	}
}

func TestParseInitializeRejectsUnsupportedVersionAndMalformedCapabilities(t *testing.T) {
	for _, body := range []string{
		`{"protocolVersion":"2024-11-05","serverInfo":{"name":"server","version":"1"},"capabilities":{"tools":{}}}`,
		`{"protocolVersion":"2025-11-25","serverInfo":{"name":"server","version":"1"}}`,
		`{"protocolVersion":"2025-11-25","serverInfo":{"name":"server","version":"1"},"capabilities":{"tools":true}}`,
		`{"protocolVersion":"2025-11-25","serverInfo":{"name":"server","version":"1"},"capabilities":{"tools":{"listChanged":"yes"}}}`,
		`{"protocolVersion":"2025-11-25","serverInfo":{"name":"server","version":"1"},"capabilities":{"unknown":true}}`,
	} {
		if _, _, err := ParseInitializeResult(json.RawMessage(body), ProtocolVersion); err == nil {
			t.Fatalf("invalid MCP initialize contract was accepted: %s", body)
		}
	}
	identity, capabilities, err := ParseInitializeResult(json.RawMessage(`{"protocolVersion":"2025-11-25","serverInfo":{"name":"server","version":"1"},"capabilities":{"tools":{"listChanged":true},"resources":{"subscribe":false}}}`), ProtocolVersion)
	if err != nil || identity.Name != "server" || !capabilities.Tools || !capabilities.Resources || capabilities.Prompts {
		t.Fatalf("strict initialize result mismatch: identity=%#v capabilities=%#v err=%v", identity, capabilities, err)
	}
	identity, capabilities, err = ParseInitializeResult(json.RawMessage(`{"protocolVersion":"2025-06-18","serverInfo":{"name":"compatible","version":"1"},"capabilities":{"tools":{},"completions":{},"futureExtension":{}}}`), ProtocolVersion)
	if err != nil || identity.ProtocolVersion != "2025-06-18" || !capabilities.Tools {
		t.Fatalf("supported negotiated revision or inert extension capability was rejected: identity=%#v capabilities=%#v err=%v", identity, capabilities, err)
	}
}

func TestLegacyMCPWithoutStructuredOutputCannotBecomeFactSource(t *testing.T) {
	legacyInitialize := json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"legacy","version":"1"},"capabilities":{"tools":{}}}`)
	if _, _, err := ParseInitializeResult(legacyInitialize, ProtocolVersion); err == nil {
		t.Fatal("legacy HTTP+SSE revision was accepted by the Streamable HTTP fact-source profile")
	}
	legacyCatalog := json.RawMessage(`{"tools":[{"name":"query","inputSchema":{"type":"object","properties":{},"additionalProperties":false}}]}`)
	if _, err := ParseTools(legacyCatalog); err == nil {
		t.Fatal("legacy tool without structured output schema became a fact source")
	}
}

func TestParseToolsAcceptsKnownDialectAndRejectsDuplicateSchemaKeys(t *testing.T) {
	tools, err := ParseTools(json.RawMessage(`{"tools":[{"name":"strict","inputSchema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{},"additionalProperties":false},"outputSchema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{},"additionalProperties":false}}]}`))
	if err != nil || len(tools) != 1 {
		t.Fatalf("known schema dialects were rejected: tools=%#v err=%v", tools, err)
	}
	if tools, err := ParseTools(json.RawMessage(`{"tools":[{"name":"ambiguous","inputSchema":{"type":"object","type":"string"},"outputSchema":{"type":"object"}}]}`)); err == nil || len(tools) != 0 {
		t.Fatalf("duplicate schema key passed strict catalog parsing: tools=%#v err=%v", tools, err)
	}
}

func TestParseToolsQuarantinesInvalidSchemaAndKeepsValidSibling(t *testing.T) {
	closed := `{"type":"object","properties":{},"additionalProperties":false}`
	page, err := ParseToolsPage(json.RawMessage(`{"tools":[` +
		`{"name":"valid","inputSchema":` + closed + `,"outputSchema":` + closed + `},` +
		`{"name":"bad_input","inputSchema":{"type":"string"},"outputSchema":` + closed + `},` +
		`{"name":"missing_output","inputSchema":` + closed + `}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tools) != 1 || page.Tools[0].Name != "valid" {
		t.Fatalf("valid sibling was not preserved: %#v", page.Tools)
	}
	want := []ToolContractIssue{
		{Name: "bad_input", Code: ToolContractInvalidInputSchema},
		{Name: "missing_output", Code: ToolContractMissingOutputSchema},
	}
	if !reflect.DeepEqual(page.Quarantined, want) {
		t.Fatalf("quarantine diagnostics mismatch: got=%#v want=%#v", page.Quarantined, want)
	}
	tools, err := ParseTools(json.RawMessage(`{"tools":[` +
		`{"name":"valid","inputSchema":` + closed + `,"outputSchema":` + closed + `},` +
		`{"name":"bad","inputSchema":{"type":"object","additionalProperties":true},"outputSchema":` + closed + `}]}`))
	if err != nil || len(tools) != 1 || tools[0].Name != "valid" {
		t.Fatalf("valid sibling catalog failed: tools=%#v err=%v", tools, err)
	}
}

func TestCollectToolCatalogPagesPreservesClosedQuarantineDiagnostics(t *testing.T) {
	closed := `{"type":"object","properties":{},"additionalProperties":false}`
	catalog, err := CollectToolCatalogPages(context.Background(), func(_ context.Context, params map[string]any) (json.RawMessage, error) {
		if params["cursor"] == nil {
			return json.RawMessage(`{"tools":[` +
				`{"name":"bad_input","inputSchema":{"type":"string"},"outputSchema":` + closed + `},` +
				`{"name":"valid","inputSchema":` + closed + `,"outputSchema":` + closed + `}],"nextCursor":"page-2"}`), nil
		}
		return json.RawMessage(`{"tools":[{"name":"missing_output","inputSchema":` + closed + `}]}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Tools) != 1 || catalog.Tools[0].Name != "valid" {
		t.Fatalf("executable catalog mismatch: %#v", catalog.Tools)
	}
	want := []ToolContractIssue{
		{Name: "bad_input", Code: ToolContractInvalidInputSchema},
		{Name: "missing_output", Code: ToolContractMissingOutputSchema},
	}
	if !reflect.DeepEqual(catalog.Quarantined, want) {
		t.Fatalf("quarantine diagnostics were lost across pages: got=%#v want=%#v", catalog.Quarantined, want)
	}
}

func TestCollectToolCatalogPagesClassifiesContractDamageNotTransportFailure(t *testing.T) {
	_, contractErr := CollectToolCatalogPages(context.Background(), func(context.Context, map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"tools":[{"name":"bad name","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`), nil
	})
	if contractErr == nil || !IsToolCatalogContractError(contractErr) {
		t.Fatalf("structural catalog damage was not classified: %v", contractErr)
	}
	transportErr := errors.New("transport unavailable")
	_, got := CollectToolCatalogPages(context.Background(), func(context.Context, map[string]any) (json.RawMessage, error) {
		return nil, transportErr
	})
	if !errors.Is(got, transportErr) || IsToolCatalogContractError(got) {
		t.Fatalf("transport error was misclassified as catalog damage: %v", got)
	}
}

func TestToolCatalogIdentityAndDuplicatesRemainWholeCatalogFailures(t *testing.T) {
	closed := `{"type":"object","properties":{},"additionalProperties":false}`
	for name, body := range map[string]string{
		"duplicate with invalid sibling": `{"tools":[` +
			`{"name":"same","inputSchema":{"type":"string"},"outputSchema":` + closed + `},` +
			`{"name":"same","inputSchema":` + closed + `,"outputSchema":` + closed + `}]}`,
		"invalid identity with valid sibling": `{"tools":[` +
			`{"name":"bad name","inputSchema":{"type":"string"},"outputSchema":` + closed + `},` +
			`{"name":"valid","inputSchema":` + closed + `,"outputSchema":` + closed + `}]}`,
		"unknown field with valid sibling": `{"tools":[` +
			`{"name":"bad","inputSchema":{"type":"string"},"outputSchema":` + closed + `,"secret":"x"},` +
			`{"name":"valid","inputSchema":` + closed + `,"outputSchema":` + closed + `}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if tools, err := ParseTools(json.RawMessage(body)); err == nil || len(tools) != 0 {
				t.Fatalf("structural catalog damage was partially accepted: tools=%#v err=%v", tools, err)
			}
		})
	}
}

func TestQuarantinedNamesRemainLoadBearingAcrossPages(t *testing.T) {
	closed := `"inputSchema":{"type":"object","properties":{},"additionalProperties":false},"outputSchema":{"type":"object","properties":{},"additionalProperties":false}`
	_, err := CollectToolsPages(context.Background(), func(_ context.Context, params map[string]any) (json.RawMessage, error) {
		if params["cursor"] == nil {
			return json.RawMessage(`{"tools":[{"name":"same","inputSchema":{"type":"string"},"outputSchema":{"type":"object"}}],"nextCursor":"page-2"}`), nil
		}
		return json.RawMessage(`{"tools":[{"name":"same",` + closed + `}]}`), nil
	})
	if err == nil {
		t.Fatal("a quarantined tool identity was forgotten across pages")
	}
}

func TestEveryToolsListPageRequiredForFreshCatalog(t *testing.T) {
	for _, cursor := range []string{`"page-2"`, `""`, `null`} {
		body := json.RawMessage(`{"tools":[],"nextCursor":` + cursor + `}`)
		if _, err := ParseTools(body); err == nil {
			t.Fatalf("present nextCursor was accepted without complete pagination: %s", cursor)
		}
	}
}

func TestToolsListPaginationRequiresCompleteBoundedCatalog(t *testing.T) {
	closed := `"inputSchema":{"type":"object","properties":{},"additionalProperties":false},"outputSchema":{"type":"object","properties":{},"additionalProperties":false}`
	calls := 0
	tools, err := CollectToolsPages(context.Background(), func(_ context.Context, params map[string]any) (json.RawMessage, error) {
		calls++
		switch params["cursor"] {
		case nil:
			return json.RawMessage(`{"tools":[{"name":"zeta",` + closed + `}],"nextCursor":"page-2"}`), nil
		case "page-2":
			return json.RawMessage(`{"tools":[{"name":"alpha",` + closed + `}]}`), nil
		default:
			return nil, errors.New("unexpected cursor")
		}
	})
	if err != nil || calls != 2 || len(tools) != 2 || tools[0].Name != "alpha" || tools[1].Name != "zeta" {
		t.Fatalf("complete paginated catalog mismatch: tools=%#v calls=%d err=%v", tools, calls, err)
	}

	_, err = CollectToolsPages(context.Background(), func(_ context.Context, params map[string]any) (json.RawMessage, error) {
		if params["cursor"] == nil {
			return json.RawMessage(`{"tools":[{"name":"same",` + closed + `}],"nextCursor":"repeat"}`), nil
		}
		return json.RawMessage(`{"tools":[{"name":"same",` + closed + `}]}`), nil
	})
	if err == nil {
		t.Fatal("duplicate tool across catalog pages was accepted")
	}

	_, err = CollectToolsPages(context.Background(), func(_ context.Context, _ map[string]any) (json.RawMessage, error) {
		return json.RawMessage(`{"tools":[],"nextCursor":"repeat"}`), nil
	})
	if err == nil {
		t.Fatal("catalog cursor cycle was accepted")
	}
}

func TestToolCatalogRejectsWhitespaceNormalizedName(t *testing.T) {
	body := json.RawMessage(`{"tools":[{"name":" echo ","inputSchema":{"type":"object","properties":{},"additionalProperties":false},"outputSchema":{"type":"object","properties":{},"additionalProperties":false}}]}`)
	if _, err := ParseTools(body); err == nil {
		t.Fatal("tool catalog silently normalized an advertised execution name")
	}
}

func TestNormalizeToolSchemaClosesNullableObjectsWithoutMutatingConstData(t *testing.T) {
	normalized, err := NormalizeToolOutputSchemaBytes(json.RawMessage(`{
		"type":"object",
		"properties":{
			"record":{"type":["object","null"],"properties":{"name":{"type":"string"}}},
			"literal":{"type":"object","const":{"type":"object"}}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(normalized, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	record := properties["record"].(map[string]any)
	literal := properties["literal"].(map[string]any)
	constValue := literal["const"].(map[string]any)
	if record["additionalProperties"] != false || literal["additionalProperties"] != false {
		t.Fatalf("nullable object schemas were not closed: %s", normalized)
	}
	if _, mutated := constValue["additionalProperties"]; mutated || constValue["type"] != "object" {
		t.Fatalf("const instance data was rewritten as schema: %s", normalized)
	}
}

func TestParseToolsPreservesExactNumericConstraints(t *testing.T) {
	tools, err := ParseTools(json.RawMessage(`{"tools":[{
		"name":"exact",
		"inputSchema":{"type":"object","properties":{"amount":{"type":"integer","minimum":9007199254740993}},"additionalProperties":false},
		"outputSchema":{"type":"object","properties":{},"additionalProperties":false}
	}]}`))
	if err != nil || len(tools) != 1 {
		t.Fatalf("exact catalog failed: tools=%#v err=%v", tools, err)
	}
	if !strings.Contains(string(tools[0].InputSchema), `"minimum":9007199254740993`) {
		t.Fatalf("catalog rounded a security constraint: %s", tools[0].InputSchema)
	}
}

func TestScheduleMCPToolsListContract(t *testing.T) {
	inputSchema := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object", "additionalProperties": false,
		"properties": map[string]any{
			"task_id": map[string]any{"type": "string", "minLength": 1},
			"schedule": map[string]any{"oneOf": []any{
				map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string", "const": "at"}, "atTime": map[string]any{"type": "string", "minLength": 1}}, "required": []string{"kind", "atTime"}, "additionalProperties": false},
				map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string", "const": "interval"}, "everyMinutes": map[string]any{"type": "integer", "minimum": 1, "maximum": 10080}}, "required": []string{"kind", "everyMinutes"}, "additionalProperties": false},
			}},
		},
		"required": []string{"task_id"},
	}
	outputSchema := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object", "additionalProperties": false,
		"properties": map[string]any{
			"status": map[string]any{"type": "string", "enum": []string{"success", "failure"}},
			"task": map[string]any{"anyOf": []any{
				map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"}, "additionalProperties": false},
				map[string]any{"type": "null"},
			}},
		},
		"required": []string{"status", "task"},
	}
	names := []string{
		"claw_schedule_create", "claw_schedule_delete", "claw_schedule_list", "claw_schedule_update",
		"gui_schedule_create", "gui_schedule_delete", "gui_schedule_list", "gui_schedule_update",
	}
	rawTools := make([]map[string]any, 0, len(names))
	for _, name := range names {
		rawTools = append(rawTools, map[string]any{
			"name": name, "inputSchema": inputSchema, "outputSchema": outputSchema,
			"annotations": map[string]any{"readOnlyHint": strings.HasSuffix(name, "_list"), "destructiveHint": strings.HasSuffix(name, "_delete"), "idempotentHint": false, "openWorldHint": false},
			"execution":   map[string]any{"taskSupport": "forbidden"},
		})
	}
	body, err := json.Marshal(map[string]any{"tools": rawTools})
	if err != nil {
		t.Fatal(err)
	}
	tools, err := ParseTools(body)
	if err != nil || len(tools) != 8 {
		t.Fatalf("SDK-shaped Schedule MCP catalog was rejected: tools=%d err=%v", len(tools), err)
	}
	for _, tool := range tools {
		if len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 || tool.ReadOnlyHint != strings.HasSuffix(tool.Name, "_list") || tool.TaskSupport != domainmcp.ToolTaskSupportForbidden {
			t.Fatalf("Schedule MCP authority projection mismatch: %#v", tool)
		}
	}
}

func TestParseToolsPreservesTaskSupport(t *testing.T) {
	closed := `{"type":"object","additionalProperties":false}`
	tools, err := ParseTools(json.RawMessage(`{"tools":[` +
		`{"name":"sync","inputSchema":` + closed + `,"outputSchema":` + closed + `},` +
		`{"name":"optional","inputSchema":` + closed + `,"outputSchema":` + closed + `,"execution":{"taskSupport":"optional"}},` +
		`{"name":"required","inputSchema":` + closed + `,"outputSchema":` + closed + `,"execution":{"taskSupport":"required"}}]}`))
	if err != nil || len(tools) != 3 {
		t.Fatalf("task support catalog was rejected: tools=%#v err=%v", tools, err)
	}
	got := map[string]domainmcp.ToolTaskSupport{}
	for _, tool := range tools {
		got[tool.Name] = tool.TaskSupport
	}
	if got["sync"] != domainmcp.ToolTaskSupportForbidden || got["optional"] != domainmcp.ToolTaskSupportOptional || got["required"] != domainmcp.ToolTaskSupportRequired {
		t.Fatalf("task support was not preserved: %#v", got)
	}
}

func TestMCPToolNameGrammarAcceptsUppercaseDotAnd128Characters(t *testing.T) {
	closed := `{"type":"object","additionalProperties":false}`
	longName := strings.Repeat("A", 128)
	body := `{"tools":[` +
		`{"name":"Lookup.V2__Exact","inputSchema":` + closed + `,"outputSchema":` + closed + `},` +
		`{"name":"` + longName + `","inputSchema":` + closed + `,"outputSchema":` + closed + `}]}`
	tools, err := ParseTools(json.RawMessage(body))
	if err != nil || len(tools) != 2 || tools[0].Name != longName || tools[1].Name != "Lookup.V2__Exact" {
		t.Fatalf("valid SEP-986 names were not preserved exactly: tools=%#v err=%v", tools, err)
	}
	invalid := `{"tools":[{"name":"` + strings.Repeat("a", 129) + `","inputSchema":` + closed + `,"outputSchema":` + closed + `}]}`
	if tools, err := ParseTools(json.RawMessage(invalid)); err == nil || len(tools) != 0 {
		t.Fatalf("overlong MCP tool name was accepted: tools=%#v err=%v", tools, err)
	}
}

func TestToolCallParamsNeverPromotesProviderAuthority(t *testing.T) {
	arguments := map[string]any{
		"query": "needle",
		"_analytix": map[string]any{
			"contextDigest": "ctx-1",
			"grantId":       "grant-1",
		},
		"__analytix":               map[string]any{"forged": true},
		"analytix_runtime_context": map[string]any{"forged": true},
	}
	params := ToolCallParams("lookup", arguments)
	clean, ok := params["arguments"].(map[string]any)
	if !ok || !reflect.DeepEqual(clean, map[string]any{"query": "needle"}) {
		t.Fatalf("provider arguments were not stripped: %#v", params)
	}
	if _, found := params["_meta"]; found {
		t.Fatalf("provider authority was promoted to transport metadata: %#v", params)
	}
	if _, exists := arguments["_analytix"]; !exists {
		t.Fatalf("ToolCallParams mutated its caller input: %#v", arguments)
	}
}

func TestToolCallParamsWithHostContextUsesOnlyTypedEnvelope(t *testing.T) {
	now := time.Date(2026, 7, 11, 1, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("docs", "docs", "1.0.0", domainsecurity.SHA256Hex([]byte("protocol-test-instance")), 3)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: serverIdentity,
		ToolName: "mcp__docs__lookup", ToolCallID: toolidentity.MustHostToolCallIDV1("protocol-host-context"), ConnectionEpoch: 3,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	envelope, err := domainmcp.NewHostContextEnvelope(securityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	params, err := ToolCallParamsWithHostContext("lookup", map[string]any{
		"query": "needle", "_analytix": map[string]any{"caseId": "forged"},
	}, envelope)
	if err != nil {
		t.Fatal(err)
	}
	arguments := params["arguments"].(map[string]any)
	meta := params["_meta"].(map[string]any)
	runtimeContext := meta["analytixRuntimeContext"].(map[string]any)
	if !reflect.DeepEqual(arguments, map[string]any{"query": "needle"}) || runtimeContext["caseId"] != "case-a" ||
		runtimeContext["contextDigest"] != securityContext.ContextDigest || runtimeContext["grantId"] != grant.GrantID {
		t.Fatalf("typed host context params mismatch: %#v", params)
	}
	if _, err := ToolCallParamsWithHostContext("lookup", nil, domainmcp.HostContextEnvelope{}); err == nil {
		t.Fatal("zero host context envelope reached transport metadata")
	}
}

func TestParseCatalogCanonicalizesAndSorts(t *testing.T) {
	tools, err := ParseTools(json.RawMessage(`{"tools":[
		{"name":"z","inputSchema":{"type":"object","required":["b","a"],"properties":{"b":{"type":"string"},"a":{"type":"object","properties":{}}}},"outputSchema":{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}},
		{"name":"a","description":"A","inputSchema":{"type":"object"},"outputSchema":{"type":"object"},"annotations":{"readOnlyHint":true}}
	]}`))
	if err != nil {
		t.Fatalf("parse tools: %v", err)
	}
	if len(tools) != 2 || tools[0].Name != "a" || tools[1].Name != "z" {
		t.Fatalf("tools were not sorted: %#v", tools)
	}
	if !tools[0].ReadOnlyHint {
		t.Fatalf("readOnlyHint was not preserved: %#v", tools[0])
	}
	if got := string(tools[1].InputSchema); got != `{"additionalProperties":false,"properties":{"a":{"additionalProperties":false,"properties":{},"type":"object"},"b":{"type":"string"}},"required":["a","b"],"type":"object"}` {
		t.Fatalf("schema not canonicalized: %s", got)
	}
	if got := string(tools[1].OutputSchema); got != `{"additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"],"type":"object"}` {
		t.Fatalf("output schema not canonicalized: %s", got)
	}

	prompts := ParsePrompts(json.RawMessage(`{"prompts":[{"name":"p","arguments":[{"name":"z"},{"name":"a","required":true}]}]}`))
	if len(prompts) != 1 || prompts[0].Arguments[0].Name != "a" || !prompts[0].Arguments[0].Required {
		t.Fatalf("prompt args mismatch: %#v", prompts)
	}
	resources := ParseResources(json.RawMessage(`{"resources":[{"uri":"z"},{"uri":"a","mimeType":"text/plain"}]}`))
	if len(resources) != 2 || resources[0].URI != "a" || resources[0].MimeType != "text/plain" {
		t.Fatalf("resources mismatch: %#v", resources)
	}
}

func TestParseToolsRejectsMissingNullAliasDuplicateAndOpenEndedSchemas(t *testing.T) {
	validInput := `"inputSchema":{"type":"object"}`
	validOutput := `"outputSchema":{"type":"object"}`
	cases := map[string]string{
		"missing tools":            `{}`,
		"missing schema":           `{"tools":[{"name":"x"}]}`,
		"null input":               `{"tools":[{"name":"x","inputSchema":null,` + validOutput + `}]}`,
		"input alias":              `{"tools":[{"name":"x","input_schema":{"type":"object"},` + validOutput + `}]}`,
		"non-object input":         `{"tools":[{"name":"x","inputSchema":{"type":"string"},` + validOutput + `}]}`,
		"empty input":              `{"tools":[{"name":"x","inputSchema":{},` + validOutput + `}]}`,
		"open input":               `{"tools":[{"name":"x","inputSchema":{"type":"object","additionalProperties":true},` + validOutput + `}]}`,
		"unknown input keyword":    `{"tools":[{"name":"x","inputSchema":{"type":"object","x-unsupported":false},` + validOutput + `}]}`,
		"missing output":           `{"tools":[{"name":"x",` + validInput + `}]}`,
		"null output":              `{"tools":[{"name":"x",` + validInput + `,"outputSchema":null}]}`,
		"output alias":             `{"tools":[{"name":"x",` + validInput + `,"output_schema":{"type":"object"}}]}`,
		"non-object output":        `{"tools":[{"name":"x",` + validInput + `,"outputSchema":{"type":"array"}}]}`,
		"open output":              `{"tools":[{"name":"x",` + validInput + `,"outputSchema":{"type":"object","additionalProperties":true}}]}`,
		"incomplete output array":  `{"tools":[{"name":"x",` + validInput + `,"outputSchema":{"type":"object","properties":{"rows":{"type":"array"}}}}]}`,
		"duplicate name":           `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `},{"name":"x",` + validInput + `,` + validOutput + `}]}`,
		"invalid additional":       `{"tools":[{"name":"x","inputSchema":{"type":"object","additionalProperties":{}},` + validOutput + `}]}`,
		"implicit object property": `{"tools":[{"name":"x","inputSchema":{"type":"object","properties":{"scope":{"properties":{}}}},` + validOutput + `}]}`,
		"untyped numeric keyword":  `{"tools":[{"name":"x","inputSchema":{"type":"object","properties":{"limit":{"minimum":1}}},` + validOutput + `}]}`,
		"case variant catalog":     `{"Tools":[{"name":"x",` + validInput + `,` + validOutput + `}]}`,
		"whitespace null catalog":  `{"tools":  null }`,
		"case variant schema":      `{"tools":[{"name":"x","InputSchema":{"type":"object"},` + validOutput + `}]}`,
		"unknown tool field":       `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `,"secret":true}]}`,
		"paged catalog":            `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `}],"nextCursor":"page-2"}`,
		"empty cursor present":     `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `}],"nextCursor":""}`,
		"null cursor present":      `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `}],"nextCursor":null}`,
		"whitespace tool name":     `{"tools":[{"name":" x ",` + validInput + `,` + validOutput + `}]}`,
		"bad annotation casing":    `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `,"annotations":{"ReadOnlyHint":true}}]}`,
		"bad annotation type":      `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `,"annotations":{"readOnlyHint":"true"}}]}`,
		"bad execution metadata":   `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `,"execution":{"taskSupport":"unknown"}}]}`,
		"bad icon metadata":        `{"tools":[{"name":"x",` + validInput + `,` + validOutput + `,"icons":[{"src":""}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if tools, err := ParseTools(json.RawMessage(body)); err == nil || len(tools) != 0 {
				t.Fatalf("invalid catalog must fail closed tools=%#v err=%v", tools, err)
			}
		})
	}
}

func TestExtractToolResultHandlesTextImageAndStructuredContent(t *testing.T) {
	result := ExtractToolResult(json.RawMessage(`{
		"content":[
			{"type":"text","text":"hello"},
			{"type":"image","mimeType":"image/png","data":"abc"}
		],
		"structuredContent":{"rows":1}
	}`))
	record, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected structured record: %#v", result)
	}
	if record["text"] != "hello" || !reflect.DeepEqual(record["structuredContent"], map[string]any{"rows": json.Number("1")}) {
		t.Fatalf("record mismatch: %#v", record)
	}
	images, _ := record["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("image content missing: %#v", record)
	}
}

func TestExtractLosslessToolResultPreservesExactNumbersAndRawHash(t *testing.T) {
	raw := json.RawMessage(`{"content":[],"structuredContent":{"amount":9007199254740993,"account":"00123456789012345678"}}`)
	result := ExtractLosslessToolResult(raw)
	record, ok := result.Value.(map[string]any)
	if !ok {
		t.Fatalf("lossless result value mismatch: %#v", result.Value)
	}
	structured, _ := record["amount"].(json.Number)
	if structured.String() != "9007199254740993" || !reflect.DeepEqual(result.RawResult, raw) || len(result.RawSHA256) != 64 {
		t.Fatalf("lossless result did not preserve exact evidence bytes/numbers: %#v", result)
	}
}

func TestDecodeLosslessJSONResultPreservesCompleteNativePayload(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"serverName":"analytix_funds","blocker":"","rowCount":2645472}`)
	result := DecodeLosslessJSONResult(raw)
	record, ok := result.Value.(map[string]any)
	if !ok || record["serverName"] != "analytix_funds" || record["blocker"] != "" || record["rowCount"] != json.Number("2645472") {
		t.Fatalf("native result lost fields or exact numbers: %#v", result)
	}
	if !reflect.DeepEqual(result.RawResult, raw) || len(result.RawSHA256) != 64 {
		t.Fatalf("native result lost raw evidence bytes: %#v", result)
	}
}

func TestDecodeLosslessJSONResultNeverProjectsInvalidRawContent(t *testing.T) {
	raw := json.RawMessage(`{"account":"6217000012345678901","account":"forged"}`)
	result := DecodeLosslessJSONResult(raw)
	if result.Value != nil || !reflect.DeepEqual(result.RawResult, raw) || len(result.RawSHA256) != 64 {
		t.Fatalf("invalid raw content became a provider-visible value: %#v", result)
	}
}

func TestMCPMetaPreservedForHostButNeverProviderVisible(t *testing.T) {
	lossless := ExtractLosslessToolResult(json.RawMessage(`{
		"content":[{"type":"text","text":"正式报告未发布"}],
		"isError":true,
		"safeToAnswer":false,
		"semantic_status":"blocked",
		"blocker":{"code":"PUBLICATION_RECEIPT_REQUIRED"},
		"partial_coverage":{"complete":false},
		"evidence_receipts":[],
		"_meta":{"analytix_evidence_ledger":{"status":"unsupported"}}
	}`))
	if lossless.Value != "正式报告未发布" {
		t.Fatalf("provider-safe text projection mismatch: %#v", lossless.Value)
	}
	if !bytes.Contains(lossless.RawResult, []byte(`"_meta"`)) || !bytes.Contains(lossless.RawResult, []byte(`"evidence_receipts"`)) {
		t.Fatalf("host-private raw audit material was lost: %s", lossless.RawResult)
	}
	projected, _ := json.Marshal(lossless.Value)
	for _, forbidden := range []string{"_meta", "evidence_receipts", "safeToAnswer", "PUBLICATION_RECEIPT_REQUIRED"} {
		if bytes.Contains(projected, []byte(forbidden)) {
			t.Fatalf("host-private MCP field %q entered provider projection: %s", forbidden, projected)
		}
	}
}
