package protocol

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	domainjsonschema "analytix.local/runtime-go/internal/domain/jsonschema"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainmcpprotocol "analytix.local/runtime-go/internal/domain/mcpprotocol"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const MaxMCPJSONBytes = 4 * 1024 * 1024
const maxMCPJSONTokens = 200_000
const maxMCPJSONStringBytes = 1024 * 1024
const ProtocolVersion = domainmcpprotocol.PreferredVersion

const (
	MaxToolsListPages       = 32
	MaxToolsListEntries     = 2048
	MaxToolsListCursorBytes = 4096
)

type ToolsPage struct {
	Tools       []domainmcp.ToolSpec
	Quarantined []ToolContractIssue
	NextCursor  string
	HasNext     bool
}

// ToolCatalog is the validated executable catalog plus the minimum closed
// diagnostic needed to explain why individual schema-invalid siblings were
// excluded. Quarantined entries never carry their untrusted schemas forward.
type ToolCatalog struct {
	Tools       []domainmcp.ToolSpec
	Quarantined []ToolContractIssue
}

// ToolCatalogContractError distinguishes a structurally invalid catalog from
// a transport outage. Callers may retain a non-executable cache hint only for
// the latter; a damaged live catalog must fail closed instead of being hidden
// behind stale schemas.
type ToolCatalogContractError struct {
	cause error
}

func (err *ToolCatalogContractError) Error() string {
	if err == nil || err.cause == nil {
		return "mcp tools catalog contract failure"
	}
	return err.cause.Error()
}

func (err *ToolCatalogContractError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func IsToolCatalogContractError(err error) bool {
	var contractErr *ToolCatalogContractError
	return errors.As(err, &contractErr)
}

func toolCatalogContractError(err error) error {
	if err == nil || IsToolCatalogContractError(err) {
		return err
	}
	return &ToolCatalogContractError{cause: err}
}

type ToolContractIssueCode string

const (
	ToolContractMissingInputSchema  ToolContractIssueCode = "missing_input_schema"
	ToolContractInvalidInputSchema  ToolContractIssueCode = "invalid_input_schema"
	ToolContractMissingOutputSchema ToolContractIssueCode = "missing_output_schema"
	ToolContractInvalidOutputSchema ToolContractIssueCode = "invalid_output_schema"
)

type ToolContractIssue struct {
	Name string
	Code ToolContractIssueCode
}

type toolCatalogCandidate struct {
	tool          domainmcp.ToolSpec
	inputSchema   any
	inputPresent  bool
	outputSchema  any
	outputPresent bool
}

type ToolsPageRequester func(context.Context, map[string]any) (json.RawMessage, error)

type ResponseID struct {
	Number int
	Null   bool
}

type JSONRPCStreamFrameKind uint8

const (
	JSONRPCStreamMatchingResult JSONRPCStreamFrameKind = iota + 1
	JSONRPCStreamMatchingError
	JSONRPCStreamIgnorableNotification
)

type JSONRPCStreamFrame struct {
	Kind                JSONRPCStreamFrameKind
	Result              json.RawMessage
	Err                 error
	InvalidatesIdentity bool
}

func (id ResponseID) Matches(expected int) bool {
	return !id.Null && id.Number == expected
}

func MarshalBoundedJSONRPC(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal MCP JSON-RPC payload: %w", err)
	}
	if len(body) > MaxMCPJSONBytes {
		return nil, errors.New("MCP JSON-RPC payload exceeds limit")
	}
	return body, nil
}

func strictMCPJSONOptions(requireObject bool) domainjsonstrict.Options {
	return domainjsonstrict.Options{
		RequireObject: requireObject,
		MaxBytes:      MaxMCPJSONBytes, MaxTokens: maxMCPJSONTokens, MaxStringBytes: maxMCPJSONStringBytes,
	}
}

type Capabilities struct {
	Tools     bool
	Prompts   bool
	Resources bool
}

// ToolCallParams is the authority-free transport path. Reserved provider keys
// are always removed and can never be promoted into the outer MCP _meta field.
func ToolCallParams(name string, arguments map[string]any) map[string]any {
	cleanArguments := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cleanArguments[key] = value
	}
	for _, key := range domainmcp.ProviderAuthorityArgumentKeys() {
		delete(cleanArguments, key)
	}
	return map[string]any{"name": name, "arguments": cleanArguments}
}

// ToolCallParamsWithHostContext is the only tools/call encoder that may emit
// Analytix authority metadata. The unforgeable in-process envelope is created
// from a validated TurnSecurityContext and ExecutionGrant, never from tool
// arguments or server annotations.
func ToolCallParamsWithHostContext(name string, arguments map[string]any, envelope domainmcp.HostContextEnvelope) (map[string]any, error) {
	runtimeContext, err := envelope.RuntimeContextRecord()
	if err != nil {
		return nil, err
	}
	params := ToolCallParams(name, arguments)
	params["_meta"] = map[string]any{"analytixRuntimeContext": runtimeContext}
	return params, nil
}

func ParseInitializeResult(result json.RawMessage, requestedVersion string) (domainmcp.ServerIdentity, Capabilities, error) {
	payload, err := decodeOpenRawObject(result, "mcp initialize result",
		"protocolVersion", "capabilities", "serverInfo", "instructions", "_meta")
	if err != nil {
		return domainmcp.ServerIdentity{}, Capabilities{}, err
	}
	if !domainmcpprotocol.SupportedVersion(requestedVersion) {
		return domainmcp.ServerIdentity{}, Capabilities{}, errors.New("mcp requested protocol version is unsupported")
	}
	serverInfo, err := decodeOpenRawObject(payload["serverInfo"], "mcp serverInfo",
		"name", "version", "title", "description", "icons", "websiteUrl")
	if err != nil {
		return domainmcp.ServerIdentity{}, Capabilities{}, err
	}
	var protocolVersion, name, version string
	if raw := payload["protocolVersion"]; len(raw) == 0 || json.Unmarshal(raw, &protocolVersion) != nil || !domainmcpprotocol.SupportedVersion(protocolVersion) {
		return domainmcp.ServerIdentity{}, Capabilities{}, errors.New("mcp initialize result has unsupported protocolVersion")
	}
	if json.Unmarshal(serverInfo["name"], &name) != nil || strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
		return domainmcp.ServerIdentity{}, Capabilities{}, errors.New("mcp initialize result has invalid serverInfo.name")
	}
	if raw := serverInfo["version"]; len(raw) == 0 || json.Unmarshal(raw, &version) != nil || strings.TrimSpace(version) == "" || strings.TrimSpace(version) != version {
		return domainmcp.ServerIdentity{}, Capabilities{}, errors.New("mcp initialize result has invalid serverInfo.version")
	}
	identity := domainmcp.ServerIdentity{
		ProtocolVersion: protocolVersion,
		Name:            name,
		Version:         version,
	}
	for _, key := range []string{"title", "description", "websiteUrl"} {
		if raw := serverInfo[key]; len(raw) > 0 {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return domainmcp.ServerIdentity{}, Capabilities{}, fmt.Errorf("mcp serverInfo.%s must be a string", key)
			}
		}
	}
	if raw := payload["instructions"]; len(raw) > 0 {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return domainmcp.ServerIdentity{}, Capabilities{}, errors.New("mcp initialize instructions must be a string")
		}
	}
	capabilities, err := parseStrictServerCapabilities(payload["capabilities"])
	if err != nil {
		return domainmcp.ServerIdentity{}, Capabilities{}, err
	}
	return identity, capabilities, nil
}

func ParseServerIdentity(result json.RawMessage) (domainmcp.ServerIdentity, error) {
	identity, _, err := ParseInitializeResult(result, ProtocolVersion)
	return identity, err
}

func ParseJSONRPCResponse(data []byte) (json.RawMessage, error) {
	result, _, err := ParseJSONRPCResponseWithID(data)
	return result, err
}

func ParseSSEJSONRPCResponse(data []byte, expectedID int) (json.RawMessage, error) {
	limit := len(data) + 1
	return ReadSSEJSONRPCResponse(bytes.NewReader(data), expectedID, limit, 1024)
}

func ReadSSEJSONRPCResponse(reader io.Reader, expectedID int, maxBytes int, maxEvents int) (json.RawMessage, error) {
	if reader == nil || maxBytes <= 0 || maxEvents <= 0 {
		return nil, errors.New("mcp sse stream limits are invalid")
	}
	var event strings.Builder
	events := 0
	totalBytes := 0
	flush := func() (json.RawMessage, bool, error) {
		if event.Len() == 0 {
			return nil, false, nil
		}
		events++
		if events > maxEvents {
			return nil, true, errors.New("mcp sse stream exceeds event limit")
		}
		payload := event.String()
		event.Reset()
		frame, err := ClassifyJSONRPCStreamFrame([]byte(payload), expectedID)
		if err != nil {
			return nil, true, err
		}
		switch frame.Kind {
		case JSONRPCStreamIgnorableNotification:
			return nil, false, nil
		case JSONRPCStreamMatchingError:
			return nil, true, frame.Err
		case JSONRPCStreamMatchingResult:
			return frame.Result, true, nil
		default:
			return nil, true, errors.New("mcp sse stream frame classification is invalid")
		}
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxBytes)
	for scanner.Scan() {
		rawLine := scanner.Text()
		totalBytes += len(rawLine) + 1
		if totalBytes > maxBytes {
			return nil, errors.New("mcp sse stream exceeds byte limit")
		}
		line := strings.TrimSuffix(rawLine, "\r")
		if line == "" {
			if result, matched, err := flush(); matched || err != nil {
				return result, err
			}
			continue
		}
		value, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		if event.Len() > 0 {
			event.WriteByte('\n')
		}
		event.WriteString(strings.TrimPrefix(value, " "))
		if event.Len() > maxBytes {
			return nil, errors.New("mcp sse event exceeds byte limit")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("mcp sse stream read failed")
	}
	if result, matched, err := flush(); matched || err != nil {
		return result, err
	}
	return nil, fmt.Errorf("mcp sse stream ended without response to id %d", expectedID)
}

func ClassifyJSONRPCStreamFrame(payload []byte, expectedID int) (JSONRPCStreamFrame, error) {
	result, id, err := ParseJSONRPCResponseWithID(payload)
	if err == nil {
		if !id.Matches(expectedID) {
			return JSONRPCStreamFrame{}, errors.New("mcp stream response id does not match request id")
		}
		return JSONRPCStreamFrame{Kind: JSONRPCStreamMatchingResult, Result: result}, nil
	}
	ignorableNotification, notificationErr := classifyJSONRPCNotification(payload)
	if notificationErr != nil {
		return JSONRPCStreamFrame{}, notificationErr
	}
	if ignorableNotification {
		return JSONRPCStreamFrame{Kind: JSONRPCStreamIgnorableNotification}, nil
	}
	var rpcError *domainmcp.JSONRPCError
	if errors.As(err, &rpcError) {
		if id.Matches(expectedID) {
			return JSONRPCStreamFrame{Kind: JSONRPCStreamMatchingError, Err: err}, nil
		}
		if id.Null && (rpcError.Code == -32700 || rpcError.Code == -32600) {
			return JSONRPCStreamFrame{Kind: JSONRPCStreamMatchingError, Err: err, InvalidatesIdentity: true}, nil
		}
	}
	return JSONRPCStreamFrame{}, errors.New("mcp stream contains an invalid or unrelated JSON-RPC frame")
}

func classifyJSONRPCNotification(payload []byte) (bool, error) {
	object, err := decodeExactRawObject(payload, "mcp sse notification", "jsonrpc", "method", "params")
	if err != nil {
		return false, nil
	}
	var version, method string
	if json.Unmarshal(object["jsonrpc"], &version) != nil || version != "2.0" ||
		json.Unmarshal(object["method"], &method) != nil || strings.TrimSpace(method) == "" || strings.TrimSpace(method) != method {
		return false, nil
	}
	if params := object["params"]; len(params) > 0 {
		value, err := domainjsonstrict.DecodeValue(params, strictMCPJSONOptions(false))
		if err != nil {
			return false, nil
		}
		switch value.(type) {
		case map[string]any, []any:
		default:
			return false, nil
		}
	}
	switch method {
	case "notifications/progress", "notifications/message":
		return true, nil
	default:
		return false, errors.New("mcp sse stream contains an authority-affecting notification")
	}
}

func ParseJSONRPCResponseWithID(data []byte) (json.RawMessage, ResponseID, error) {
	response, err := decodeExactRawObject(data, "mcp jsonrpc response", "jsonrpc", "id", "result", "error")
	if err != nil {
		return nil, ResponseID{}, err
	}
	var version string
	if err := json.Unmarshal(response["jsonrpc"], &version); err != nil || version != "2.0" {
		return nil, ResponseID{}, errors.New("mcp jsonrpc response has invalid jsonrpc version")
	}
	result, hasResult := response["result"]
	rawError, hasError := response["error"]
	if hasResult == hasError {
		return nil, ResponseID{}, errors.New("mcp jsonrpc response must contain exactly one of result or error")
	}
	rawID, hasID := response["id"]
	if !hasID {
		return nil, ResponseID{}, errors.New("mcp jsonrpc response is missing id")
	}
	id := ResponseID{Null: bytes.Equal(bytes.TrimSpace(rawID), []byte("null"))}
	if !id.Null {
		if err := json.Unmarshal(rawID, &id.Number); err != nil {
			return nil, ResponseID{}, errors.New("mcp jsonrpc response has invalid id")
		}
	}
	if hasResult {
		if id.Null {
			return nil, id, errors.New("mcp jsonrpc result cannot use a null id")
		}
		return append(json.RawMessage(nil), result...), id, nil
	}
	rpcError, err := decodeExactRawObject(rawError, "mcp jsonrpc error", "code", "message", "data")
	if err != nil {
		return nil, id, err
	}
	var code int
	var message string
	if json.Unmarshal(rpcError["code"], &code) != nil {
		return nil, id, errors.New("mcp jsonrpc error has invalid code")
	}
	if json.Unmarshal(rpcError["message"], &message) != nil || strings.TrimSpace(message) == "" {
		return nil, id, errors.New("mcp jsonrpc error has invalid message")
	}
	if id.Null && code != -32700 && code != -32600 {
		return nil, id, errors.New("mcp jsonrpc null-id error has invalid class")
	}
	return nil, id, &domainmcp.JSONRPCError{
		Code: code, Message: message, Data: append(json.RawMessage(nil), rpcError["data"]...),
	}
}

func isConnectionLevelJSONRPCError(err error) bool {
	var rpcError *domainmcp.JSONRPCError
	return errors.As(err, &rpcError) && (rpcError.Code == -32700 || rpcError.Code == -32600)
}

func ParseServerCapabilities(result json.RawMessage) Capabilities {
	_, capabilities, err := ParseInitializeResult(result, ProtocolVersion)
	if err != nil {
		return Capabilities{}
	}
	return capabilities
}

func parseStrictServerCapabilities(raw json.RawMessage) (Capabilities, error) {
	capabilityFields, err := domainjsonstrict.DecodeRawObject(raw, strictMCPJSONOptions(true))
	if err != nil {
		return Capabilities{}, errors.New("mcp server capabilities must be a strict object")
	}
	capabilities := Capabilities{}
	for key, allowed := range map[string][]string{
		"logging": {}, "prompts": {"listChanged"}, "resources": {"subscribe", "listChanged"}, "tools": {"listChanged"},
	} {
		value := capabilityFields[key]
		if len(value) == 0 {
			continue
		}
		object, err := decodeExactRawObject(value, "mcp "+key+" capability", allowed...)
		if err != nil {
			return Capabilities{}, err
		}
		for field, rawValue := range object {
			var enabled bool
			if json.Unmarshal(rawValue, &enabled) != nil {
				return Capabilities{}, fmt.Errorf("mcp %s capability %s must be boolean", key, field)
			}
		}
		switch key {
		case "tools":
			capabilities.Tools = true
		case "prompts":
			capabilities.Prompts = true
		case "resources":
			capabilities.Resources = true
		}
	}
	if rawExperimental := capabilityFields["experimental"]; len(rawExperimental) > 0 {
		if _, err := domainjsonstrict.DecodeRawObject(rawExperimental, strictMCPJSONOptions(true)); err != nil {
			return Capabilities{}, errors.New("mcp experimental capability must be an object")
		}
	}
	for key, value := range capabilityFields {
		switch key {
		case "experimental", "logging", "prompts", "resources", "tools":
			continue
		default:
			if _, err := domainjsonstrict.DecodeRawObject(value, strictMCPJSONOptions(true)); err != nil {
				return Capabilities{}, fmt.Errorf("mcp unhandled %s capability must be an object", key)
			}
		}
	}
	return capabilities, nil
}

func ParseTools(result json.RawMessage) ([]domainmcp.ToolSpec, error) {
	page, err := ParseToolsPage(result)
	if err != nil {
		return nil, err
	}
	if page.HasNext {
		return nil, errors.New("mcp tools catalog pagination is incomplete")
	}
	if len(page.Tools) == 0 && len(page.Quarantined) > 0 {
		return nil, errors.New("mcp tools catalog contains no valid tool contracts")
	}
	return page.Tools, nil
}

func ParseToolsPage(result json.RawMessage) (ToolsPage, error) {
	payload, err := decodeExactRawObject(result, "mcp tools catalog", "tools", "nextCursor", "_meta")
	if err != nil {
		return ToolsPage{}, err
	}
	page := ToolsPage{}
	if rawMeta, hasMeta := payload["_meta"]; hasMeta {
		if _, err := domainjsonstrict.DecodeRawObject(rawMeta, strictMCPJSONOptions(true)); err != nil {
			return ToolsPage{}, errors.New("mcp tools catalog _meta must be an object")
		}
	}
	if rawCursor, hasCursor := payload["nextCursor"]; hasCursor {
		var cursor string
		if json.Unmarshal(rawCursor, &cursor) != nil || cursor == "" || strings.TrimSpace(cursor) != cursor || len([]byte(cursor)) > MaxToolsListCursorBytes {
			return ToolsPage{}, errors.New("mcp tools catalog nextCursor is invalid")
		}
		page.NextCursor = cursor
		page.HasNext = true
	}
	rawCatalog := payload["tools"]
	trimmedCatalog := bytes.TrimSpace(rawCatalog)
	if len(trimmedCatalog) == 0 || bytes.Equal(trimmedCatalog, []byte("null")) || trimmedCatalog[0] != '[' {
		return ToolsPage{}, errors.New("mcp tools catalog is missing tools")
	}
	var rawTools []map[string]any
	toolsDecoder := json.NewDecoder(bytes.NewReader(rawCatalog))
	toolsDecoder.UseNumber()
	if err := toolsDecoder.Decode(&rawTools); err != nil {
		return ToolsPage{}, fmt.Errorf("decode mcp tools list: %w", err)
	}
	if len(rawTools) > MaxToolsListEntries {
		return ToolsPage{}, errors.New("mcp tools catalog exceeds the entry limit")
	}
	candidates := make([]toolCatalogCandidate, 0, len(rawTools))
	seen := map[string]struct{}{}
	for index, raw := range rawTools {
		if !hasOnlyExactKeys(raw, "name", "title", "description", "inputSchema", "outputSchema", "annotations", "icons", "execution", "_meta") {
			return ToolsPage{}, fmt.Errorf("mcp tool %d contains an unknown field", index)
		}
		nameValue, ok := raw["name"].(string)
		if !ok {
			return ToolsPage{}, fmt.Errorf("mcp tool %d has invalid name", index)
		}
		name := strings.TrimSpace(nameValue)
		if name == "" || name != nameValue || domainmcpname.ValidateToolName(name) != nil {
			return ToolsPage{}, fmt.Errorf("mcp tool %d has empty name", index)
		}
		if _, duplicate := seen[name]; duplicate {
			return ToolsPage{}, fmt.Errorf("mcp tool %q is duplicated", name)
		}
		seen[name] = struct{}{}
		if value, exists := raw["title"]; exists {
			if _, ok := value.(string); !ok {
				return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid title", name)
			}
		}
		description := ""
		if value, exists := raw["description"]; exists {
			text, ok := value.(string)
			if !ok {
				return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid description", name)
			}
			description = strings.TrimSpace(text)
		}
		tool := domainmcp.ToolSpec{
			Name:        name,
			Description: description,
			TaskSupport: domainmcp.ToolTaskSupportForbidden,
		}
		if annotationsValue, exists := raw["annotations"]; exists && annotationsValue != nil {
			annotations, ok := annotationsValue.(map[string]any)
			if !ok || !hasOnlyExactKeys(annotations, "title", "readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint") {
				return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid annotations", name)
			}
			for _, key := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
				if value, exists := annotations[key]; exists {
					if _, ok := value.(bool); !ok {
						return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid annotations", name)
					}
				}
			}
			if value, exists := annotations["title"]; exists {
				if _, ok := value.(string); !ok {
					return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid annotations", name)
				}
			}
			tool.ReadOnlyHint, _ = annotations["readOnlyHint"].(bool)
		}
		if executionValue, exists := raw["execution"]; exists && executionValue != nil {
			execution, ok := executionValue.(map[string]any)
			if !ok || !hasOnlyExactKeys(execution, "taskSupport") {
				return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid execution metadata", name)
			}
			if value, exists := execution["taskSupport"]; exists {
				taskSupport, ok := value.(string)
				if !ok || (taskSupport != "forbidden" && taskSupport != "optional" && taskSupport != "required") {
					return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid execution metadata", name)
				}
				tool.TaskSupport = domainmcp.ToolTaskSupport(taskSupport)
			}
		}
		if meta, exists := raw["_meta"]; exists && meta != nil {
			if _, ok := meta.(map[string]any); !ok {
				return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid _meta", name)
			}
		}
		if icons, exists := raw["icons"]; exists && icons != nil {
			if !validToolIcons(icons) {
				return ToolsPage{}, fmt.Errorf("mcp tool %q has invalid icons", name)
			}
		}
		inputSchema, inputPresent := raw["inputSchema"]
		outputSchema, outputPresent := raw["outputSchema"]
		candidates = append(candidates, toolCatalogCandidate{
			tool:        tool,
			inputSchema: inputSchema, inputPresent: inputPresent && inputSchema != nil,
			outputSchema: outputSchema, outputPresent: outputPresent && outputSchema != nil,
		})
	}
	tools := make([]domainmcp.ToolSpec, 0, len(candidates))
	quarantined := make([]ToolContractIssue, 0)
	for _, candidate := range candidates {
		if !candidate.inputPresent {
			quarantined = append(quarantined, ToolContractIssue{Name: candidate.tool.Name, Code: ToolContractMissingInputSchema})
			continue
		}
		inputSchemaBytes, err := NormalizeToolInputSchema(candidate.inputSchema)
		if err != nil {
			quarantined = append(quarantined, ToolContractIssue{Name: candidate.tool.Name, Code: ToolContractInvalidInputSchema})
			continue
		}
		if !candidate.outputPresent {
			quarantined = append(quarantined, ToolContractIssue{Name: candidate.tool.Name, Code: ToolContractMissingOutputSchema})
			continue
		}
		outputSchemaBytes, err := NormalizeToolOutputSchema(candidate.outputSchema)
		if err != nil {
			quarantined = append(quarantined, ToolContractIssue{Name: candidate.tool.Name, Code: ToolContractInvalidOutputSchema})
			continue
		}
		candidate.tool.InputSchema = json.RawMessage(CanonicalJSONSchema(inputSchemaBytes))
		candidate.tool.OutputSchema = json.RawMessage(CanonicalJSONSchema(outputSchemaBytes))
		tools = append(tools, candidate.tool)
	}
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	sort.SliceStable(quarantined, func(i, j int) bool {
		if quarantined[i].Name == quarantined[j].Name {
			return quarantined[i].Code < quarantined[j].Code
		}
		return quarantined[i].Name < quarantined[j].Name
	})
	page.Tools = tools
	page.Quarantined = quarantined
	return page, nil
}

func CollectToolsPages(ctx context.Context, request ToolsPageRequester) ([]domainmcp.ToolSpec, error) {
	catalog, err := CollectToolCatalogPages(ctx, request)
	if err != nil {
		return nil, err
	}
	return catalog.Tools, nil
}

func CollectToolCatalogPages(ctx context.Context, request ToolsPageRequester) (ToolCatalog, error) {
	if request == nil {
		return ToolCatalog{}, errors.New("mcp tools page requester is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	all := make([]domainmcp.ToolSpec, 0)
	allQuarantined := make([]ToolContractIssue, 0)
	seenTools := map[string]bool{}
	seenCursors := map[string]bool{}
	totalEntries := 0
	params := map[string]any{}
	for pageIndex := 0; pageIndex < MaxToolsListPages; pageIndex++ {
		if err := ctx.Err(); err != nil {
			return ToolCatalog{}, err
		}
		raw, err := request(ctx, params)
		if err != nil {
			return ToolCatalog{}, err
		}
		page, err := ParseToolsPage(raw)
		if err != nil {
			return ToolCatalog{}, toolCatalogContractError(err)
		}
		pageEntries := len(page.Tools) + len(page.Quarantined)
		totalEntries += pageEntries
		if totalEntries > MaxToolsListEntries {
			return ToolCatalog{}, toolCatalogContractError(errors.New("mcp tools catalog exceeds the entry limit"))
		}
		for _, tool := range page.Tools {
			if seenTools[tool.Name] {
				return ToolCatalog{}, toolCatalogContractError(errors.New("mcp tools catalog repeats a tool across pages"))
			}
			seenTools[tool.Name] = true
			all = append(all, tool)
		}
		for _, issue := range page.Quarantined {
			if seenTools[issue.Name] {
				return ToolCatalog{}, toolCatalogContractError(errors.New("mcp tools catalog repeats a tool across pages"))
			}
			seenTools[issue.Name] = true
			allQuarantined = append(allQuarantined, issue)
		}
		if !page.HasNext {
			if len(all) == 0 && len(allQuarantined) > 0 {
				return ToolCatalog{}, toolCatalogContractError(errors.New("mcp tools catalog contains no valid tool contracts"))
			}
			sort.SliceStable(all, func(i, j int) bool { return all[i].Name < all[j].Name })
			sort.SliceStable(allQuarantined, func(i, j int) bool {
				if allQuarantined[i].Name == allQuarantined[j].Name {
					return allQuarantined[i].Code < allQuarantined[j].Code
				}
				return allQuarantined[i].Name < allQuarantined[j].Name
			})
			return ToolCatalog{Tools: all, Quarantined: allQuarantined}, nil
		}
		if seenCursors[page.NextCursor] {
			return ToolCatalog{}, toolCatalogContractError(errors.New("mcp tools catalog cursor cycle detected"))
		}
		seenCursors[page.NextCursor] = true
		params = map[string]any{"cursor": page.NextCursor}
	}
	return ToolCatalog{}, toolCatalogContractError(errors.New("mcp tools catalog exceeds the page limit"))
}

func validToolIcons(value any) bool {
	icons, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range icons {
		icon, ok := value.(map[string]any)
		if !ok || !hasOnlyExactKeys(icon, "src", "mimeType", "sizes", "theme") {
			return false
		}
		src, ok := icon["src"].(string)
		if !ok || strings.TrimSpace(src) == "" {
			return false
		}
		if mimeType, exists := icon["mimeType"]; exists {
			if _, ok := mimeType.(string); !ok {
				return false
			}
		}
		if sizesValue, exists := icon["sizes"]; exists {
			sizes, ok := sizesValue.([]any)
			if !ok {
				return false
			}
			for _, size := range sizes {
				if _, ok := size.(string); !ok {
					return false
				}
			}
		}
		if themeValue, exists := icon["theme"]; exists {
			theme, ok := themeValue.(string)
			if !ok || (theme != "light" && theme != "dark") {
				return false
			}
		}
	}
	return true
}

func decodeExactRawObject(body []byte, label string, allowed ...string) (map[string]json.RawMessage, error) {
	object, err := domainjsonstrict.DecodeRawObject(body, strictMCPJSONOptions(true))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	allowlist := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowlist[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowlist[key]; !ok {
			return nil, fmt.Errorf("%s contains an unknown field", label)
		}
	}
	return object, nil
}

func decodeOpenRawObject(body []byte, label string, known ...string) (map[string]json.RawMessage, error) {
	object, err := domainjsonstrict.DecodeRawObject(body, strictMCPJSONOptions(true))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	knownExact := make(map[string]bool, len(known))
	for _, key := range known {
		knownExact[key] = true
	}
	for key := range object {
		if knownExact[key] {
			continue
		}
		for _, canonical := range known {
			if strings.EqualFold(key, canonical) {
				return nil, fmt.Errorf("%s contains a case-confusable field", label)
			}
		}
	}
	return object, nil
}

func hasOnlyExactKeys(object map[string]any, allowed ...string) bool {
	allowlist := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowlist[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowlist[key]; !ok {
			return false
		}
	}
	return true
}

func NormalizeToolInputSchema(value any) (json.RawMessage, error) {
	return normalizeToolObjectSchema(value)
}

func NormalizeToolOutputSchema(value any) (json.RawMessage, error) {
	return normalizeToolObjectSchema(value)
}

func normalizeToolObjectSchema(value any) (json.RawMessage, error) {
	root, ok := value.(map[string]any)
	if !ok || len(root) == 0 {
		return nil, errors.New("schema root must be a non-empty object")
	}
	rootType, ok := root["type"].(string)
	if !ok || strings.TrimSpace(rootType) != "object" {
		return nil, errors.New("schema root type must be object")
	}
	if err := domainjsonschema.ValidateDefinition(root, domainjsonschema.DefinitionOptions{
		RequireObjectRoot: true, RequireClosedObjects: true, RequireCompleteCollections: true,
	}); err != nil {
		return nil, err
	}
	normalized, err := closeExternalObjectSchemas(root)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(CanonicalJSONSchema(body)), nil
}

func NormalizeToolInputSchemaBytes(body json.RawMessage) (json.RawMessage, error) {
	return normalizeToolObjectSchemaBytes(body)
}

func NormalizeToolOutputSchemaBytes(body json.RawMessage) (json.RawMessage, error) {
	return normalizeToolObjectSchemaBytes(body)
}

func normalizeToolObjectSchemaBytes(body json.RawMessage) (json.RawMessage, error) {
	if len(body) == 0 || string(body) == "null" {
		return nil, errors.New("schema is missing")
	}
	value, err := domainjsonstrict.DecodeValue(body, strictMCPJSONOptions(true))
	if err != nil {
		return nil, err
	}
	return normalizeToolObjectSchema(value)
}

func closeExternalObjectSchemas(value any) (any, error) {
	if boolean, ok := value.(bool); ok {
		return boolean, nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("schema node must be an object")
	}
	out := make(map[string]any, len(schema)+1)
	for key, child := range schema {
		switch key {
		case "properties", "patternProperties", "dependentSchemas", "$defs", "definitions":
			properties, ok := child.(map[string]any)
			if !ok {
				return nil, errors.New("properties must be an object")
			}
			normalizedProperties := make(map[string]any, len(properties))
			for name, rawProperty := range properties {
				normalized, err := closeExternalObjectSchemas(rawProperty)
				if err != nil {
					return nil, err
				}
				normalizedProperties[name] = normalized
			}
			out[key] = normalizedProperties
		case "items", "additionalProperties", "unevaluatedProperties", "contains", "propertyNames", "not", "if", "then", "else", "contentSchema", "additionalItems", "unevaluatedItems":
			if _, isSchema := child.(map[string]any); !isSchema {
				out[key] = child
				continue
			}
			normalized, err := closeExternalObjectSchemas(child)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		case "anyOf", "oneOf", "allOf", "prefixItems":
			branches, ok := child.([]any)
			if !ok {
				return nil, fmt.Errorf("%s must be an array", key)
			}
			normalizedBranches := make([]any, 0, len(branches))
			for _, branch := range branches {
				normalized, err := closeExternalObjectSchemas(branch)
				if err != nil {
					return nil, err
				}
				normalizedBranches = append(normalizedBranches, normalized)
			}
			out[key] = normalizedBranches
		case "dependencies":
			dependencies, ok := child.(map[string]any)
			if !ok {
				return nil, errors.New("dependencies must be an object")
			}
			normalizedDependencies := make(map[string]any, len(dependencies))
			for name, dependency := range dependencies {
				if _, isList := dependency.([]any); isList {
					normalizedDependencies[name] = dependency
					continue
				}
				normalized, err := closeExternalObjectSchemas(dependency)
				if err != nil {
					return nil, err
				}
				normalizedDependencies[name] = normalized
			}
			out[key] = normalizedDependencies
		default:
			// enum/const/default/examples are instance data, not nested schema
			// definitions. They must remain byte-semantically unchanged.
			out[key] = child
		}
	}
	if schemaAllowsObject(schema["type"]) {
		if additional, exists := schema["additionalProperties"]; !exists {
			out["additionalProperties"] = false
		} else if closed, ok := additional.(bool); !ok || closed {
			if ok {
				return nil, errors.New("additionalProperties must be false")
			}
			return nil, errors.New("additionalProperties must be boolean")
		}
	}
	return out, nil
}

func schemaAllowsObject(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == "object"
	case []any:
		for _, item := range typed {
			if name, ok := item.(string); ok && strings.TrimSpace(name) == "object" {
				return true
			}
		}
	}
	return false
}

func ParsePrompts(result json.RawMessage) []domainmcp.PromptSpec {
	var payload struct {
		Prompts []map[string]any `json:"prompts"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil
	}
	prompts := make([]domainmcp.PromptSpec, 0, len(payload.Prompts))
	for _, raw := range payload.Prompts {
		name := stringAny(raw["name"])
		if name == "" {
			continue
		}
		prompt := domainmcp.PromptSpec{
			Name:        name,
			Description: stringAny(raw["description"]),
		}
		for _, item := range listAny(raw["arguments"]) {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			argName := stringAny(record["name"])
			if argName == "" {
				continue
			}
			prompt.Arguments = append(prompt.Arguments, domainmcp.PromptArgumentSpec{
				Name:        argName,
				Description: stringAny(record["description"]),
				Required:    boolAny(record["required"]),
			})
		}
		sort.SliceStable(prompt.Arguments, func(i, j int) bool { return prompt.Arguments[i].Name < prompt.Arguments[j].Name })
		prompts = append(prompts, prompt)
	}
	sort.SliceStable(prompts, func(i, j int) bool { return prompts[i].Name < prompts[j].Name })
	return prompts
}

func ParseResources(result json.RawMessage) []domainmcp.ResourceSpec {
	var payload struct {
		Resources []map[string]any `json:"resources"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil
	}
	resources := make([]domainmcp.ResourceSpec, 0, len(payload.Resources))
	for _, raw := range payload.Resources {
		uri := stringAny(raw["uri"])
		if uri == "" {
			continue
		}
		resources = append(resources, domainmcp.ResourceSpec{
			URI:         uri,
			Name:        stringAny(raw["name"]),
			Description: stringAny(raw["description"]),
			MimeType:    stringAny(raw["mimeType"]),
		})
	}
	sort.SliceStable(resources, func(i, j int) bool { return resources[i].URI < resources[j].URI })
	return resources
}

func ExtractToolResult(result json.RawMessage) any {
	return ExtractLosslessToolResult(result).Value
}

func ExtractLosslessToolResult(result json.RawMessage) domainmcp.LosslessToolResult {
	lossless := DecodeLosslessJSONResult(result)
	payload, ok := lossless.Value.(map[string]any)
	if !ok {
		return lossless
	}
	lossless.Value = extractDecodedToolResult(payload)
	return lossless
}

func DecodeLosslessJSONResult(result json.RawMessage) domainmcp.LosslessToolResult {
	digest := sha256.Sum256(result)
	var value any
	if err := domainjsonstrict.Validate(result, strictMCPJSONOptions(false)); err != nil {
		return domainmcp.LosslessToolResult{RawResult: append(json.RawMessage(nil), result...), RawSHA256: fmt.Sprintf("%x", digest[:])}
	}
	decoder := json.NewDecoder(bytes.NewReader(result))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return domainmcp.LosslessToolResult{RawResult: append(json.RawMessage(nil), result...), RawSHA256: fmt.Sprintf("%x", digest[:])}
	}
	return domainmcp.LosslessToolResult{Value: value, RawResult: append(json.RawMessage(nil), result...), RawSHA256: fmt.Sprintf("%x", digest[:])}
}

func extractDecodedToolResult(payload map[string]any) any {
	structured, hasStructured := payload["structuredContent"]
	if content, ok := payload["content"].([]any); ok {
		texts := []string{}
		images := []any{}
		nonTextContent := []any{}
		for _, item := range content {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			contentType, _ := record["type"].(string)
			switch contentType {
			case "text":
				if text, ok := record["text"].(string); ok && text != "" {
					texts = append(texts, text)
				}
			case "image":
				if image, ok := normalizedImageContent(record); ok {
					images = append(images, image)
				} else {
					nonTextContent = append(nonTextContent, record)
				}
			case "audio", "resource", "resource_link":
				nonTextContent = append(nonTextContent, record)
			}
		}
		if len(texts) > 0 || len(images) > 0 || len(nonTextContent) > 0 {
			resultText := strings.Join(texts, "\n")
			if len(images) > 0 || len(nonTextContent) > 0 || hasStructured {
				record := map[string]any{}
				if resultText != "" {
					record["text"] = resultText
				}
				if len(images) > 0 {
					record["images"] = images
				}
				if len(nonTextContent) > 0 {
					record["content"] = nonTextContent
				}
				if hasStructured {
					record["structuredContent"] = structured
				}
				return record
			}
			return resultText
		}
	}
	if hasStructured {
		return structured
	}
	return map[string]any{}
}

func CanonicalJSONSchema(body []byte) string {
	return domainmodel.CanonicalJSONSchema(json.RawMessage(body))
}

func normalizedImageContent(record map[string]any) (map[string]any, bool) {
	data, dataOK := record["data"].(string)
	mimeType, mimeOK := record["mimeType"].(string)
	data = strings.TrimSpace(data)
	mimeType = strings.TrimSpace(mimeType)
	if !dataOK || !mimeOK || data == "" || mimeType == "" {
		return nil, false
	}
	return map[string]any{
		"type":        "image",
		"mime_type":   mimeType,
		"data_base64": data,
	}, true
}

func jsonRawValuePresent(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != "false"
}

func listAny(value any) []any {
	items, _ := value.([]any)
	return items
}

func boolAny(value any) bool {
	result, _ := value.(bool)
	return result
}

func stringAny(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}
