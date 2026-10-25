package mcp

import (
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// This is a fixed Main Schedule API client, not a remote MCP transport. Only
// the exact private startup binding can construct it. No configured executable,
// proxy, remote catalog, model URL or environment is used by this client.
type hostScheduleClientV1 struct {
	mu              sync.RWMutex
	closed          bool
	owner           *ProductionManager
	baseURL, secret string
	http            *http.Client
}

func exactHostScheduleClientBindingV1(specs []ServerSpec, private *ServerSpec) *ServerSpec {
	if private == nil || private.AccountCredential != nil || private.OAuthBinding != nil || len(private.Args) != 6 {
		return nil
	}
	if _, err := ValidateHostScheduleMCPBindingV1(*private); err != nil {
		return nil
	}
	var result *ServerSpec
	for i := range specs {
		projection := cloneServerSpecForAuthority(specs[i])
		projection.ExpectedServerName, projection.ExpectedServerVersion, projection.IdentitySource = "", "", ""
		projection.ReadOnlyToolNames = nil
		if hostScheduleLoopbackPortV1(specs[i]) != 0 && reflect.DeepEqual(canonicalHostScheduleMCPBindingV1(projection), canonicalHostScheduleMCPBindingV1(*private)) {
			if result != nil {
				return nil
			}
			copy := cloneServerSpecForAuthority(specs[i])
			result = &copy
		}
	}
	return result
}

func newHostScheduleClientV1(owner *ProductionManager, spec ServerSpec) *hostScheduleClientV1 {
	return &hostScheduleClientV1{owner: owner, baseURL: spec.Args[3], secret: spec.Args[5], http: &http.Client{
		Timeout:       time.Duration(hostScheduleMCPTimeoutMSV1) * time.Millisecond,
		Transport:     &http.Transport{Proxy: nil, MaxResponseHeaderBytes: 32 * 1024},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Schedule redirects are unavailable") },
	}}
}

func (c *hostScheduleClientV1) available() bool { c.mu.RLock(); defer c.mu.RUnlock(); return !c.closed }
func (c *hostScheduleClientV1) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.http.CloseIdleConnections()
}
func (c *hostScheduleClientV1) ObservedServerIdentity() domainmcp.ServerIdentity {
	if !c.available() {
		return domainmcp.ServerIdentity{}
	}
	return domainmcp.ServerIdentity{ProtocolVersion: MCPProtocolVersion, Name: hostScheduleMCPServerNameV1, Version: hostScheduleMCPServerVersionV1}
}
func (c *hostScheduleClientV1) ListTools() ([]ToolSpec, error) {
	if !c.available() {
		return nil, errors.New("Schedule client is closed")
	}
	return hostScheduleToolsV1(), nil
}
func (c *hostScheduleClientV1) ListPrompts() ([]PromptSpec, error)     { return nil, nil }
func (c *hostScheduleClientV1) ListResources() ([]ResourceSpec, error) { return nil, nil }
func (c *hostScheduleClientV1) CallTool(name string, args map[string]any) (any, error) {
	return c.CallToolContext(context.Background(), name, args)
}
func (c *hostScheduleClientV1) CallToolContext(ctx context.Context, name string, args map[string]any) (any, error) {
	return c.call(ctx, name, args, nil)
}
func (c *hostScheduleClientV1) CallToolWithHostContext(ctx context.Context, name string, args map[string]any, envelope domainmcp.HostContextEnvelope) (any, error) {
	if _, err := mcpprotocol.ToolCallParamsWithScheduleOriginV1(name, args, envelope); err != nil {
		return nil, err
	}
	return c.call(ctx, name, args, &envelope)
}

func (c *hostScheduleClientV1) call(ctx context.Context, name string, args map[string]any, envelope *domainmcp.HostContextEnvelope) (any, error) {
	if !c.available() || ctx == nil {
		return nil, errors.New("Schedule client is unavailable")
	}
	var tool *ToolSpec
	for _, candidate := range hostScheduleToolsV1() {
		if candidate.Name == name {
			copy := candidate
			tool = &copy
			break
		}
	}
	body, err := json.Marshal(args)
	if err != nil || tool == nil || domainmcp.ProviderArgumentsContainHostAuthority(args) || toolcatalogapp.ValidateJSONSchemaRawValue(body, tool.InputSchema, "arguments") != nil {
		return nil, errors.New("Schedule arguments are invalid")
	}
	operation := strings.TrimPrefix(strings.TrimPrefix(name, "gui_schedule_"), "claw_schedule_")
	payload := map[string]any{}
	switch operation {
	case "create", "update":
		input := map[string]any{}
		for key, value := range args {
			if key == "task_id" {
				continue
			}
			mapped := map[string]string{"workspace_root": "workspaceRoot", "claw_channel_id": "clawChannelId", "provider_id": "providerId", "reasoning_effort": "reasoningEffort"}[key]
			if mapped == "" {
				mapped = key
			}
			input[mapped] = value
		}
		if operation == "create" {
			payload["input"] = input
			if envelope == nil && name == "gui_schedule_create" {
				return c.failure(operation, args), nil
			}
			if envelope != nil {
				origin, err := envelope.ScheduleOriginRecordV1()
				if err != nil {
					return nil, err
				}
				delete(origin, "version")
				payload["origin"] = origin
			}
		} else {
			payload["taskId"], payload["patch"] = args["task_id"], input
		}
	case "delete":
		payload["taskId"] = args["task_id"]
	case "list":
	default:
		return nil, errors.New("Schedule operation is unavailable")
	}
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("Schedule request is invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/schedule/internal/"+operation, bytes.NewReader(requestBody))
	if err != nil {
		return nil, errors.New("Schedule request is invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secret)
	if _, ok := payload["origin"]; ok {
		req.Header.Set("X-Analytix-Schedule-Origin", "typed-v1")
	}
	domainprocess.RecordKnownSecretsV1(ctx, c.secret)
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return c.failure(operation, args), nil
	}
	defer response.Body.Close()
	const limit = 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(raw) > limit || response.StatusCode != http.StatusOK {
		return c.failure(operation, args), nil
	}
	backend, err := domainjsonstrict.DecodeObject(raw, domainjsonstrict.Options{MaxBytes: limit, MaxDepth: 20, MaxTokens: 100000, MaxStringBytes: limit})
	if err != nil || backend["ok"] != true {
		return c.failure(operation, args), nil
	}
	structured := map[string]any{"status": "success"}
	message := ""
	switch operation {
	case "list":
		tasks, ok := backend["tasks"].([]any)
		if !ok || len(backend) != 2 {
			return c.failure(operation, args), nil
		}
		for _, task := range tasks {
			rawTask, _ := json.Marshal(task)
			var typed domaintoolresult.ProtectedScheduleTaskV1
			if json.Unmarshal(rawTask, &typed) != nil {
				return c.failure(operation, args), nil
			}
		}
		message = "No scheduled tasks are configured."
		if len(tasks) > 0 {
			message = fmt.Sprintf("Found %d scheduled task(s).", len(tasks))
		}
		structured["tasks"] = tasks
	case "create", "update":
		if len(backend) != 2 {
			return c.failure(operation, args), nil
		}
		task, ok := backend["task"].(map[string]any)
		if !ok {
			return c.failure(operation, args), nil
		}
		message = "Scheduled task created: "
		if operation == "update" {
			message = "Scheduled task updated: "
		}
		title, _ := task["title"].(string)
		message += title
		structured["task"] = task
	case "delete":
		if len(backend) != 1 {
			return c.failure(operation, args), nil
		}
		message = fmt.Sprintf("Scheduled task deleted: %s", args["task_id"])
		structured["taskId"] = args["task_id"]
	}
	structured["message"] = message
	if operation != "list" {
		encoded, _ := json.Marshal(structured)
		if _, err := domaintoolresult.ParseProtectedScheduleToolOutputV1(operation, encoded, false); err != nil {
			return c.failure(operation, args), nil
		}
	}
	return scheduleResultV1(structured, false), nil
}

func (c *hostScheduleClientV1) failure(operation string, args map[string]any) any {
	object := "scheduled task"
	if operation == "list" {
		object = "scheduled tasks"
	}
	value := map[string]any{"status": "failure", "message": fmt.Sprintf("Failed to %s %s: the Analytix schedule backend did not return a valid successful response.", operation, object)}
	switch operation {
	case "list":
		value["tasks"] = []any{}
	case "delete":
		value["taskId"] = args["task_id"]
	default:
		value["task"] = nil
	}
	return scheduleResultV1(value, true)
}
func scheduleResultV1(value map[string]any, failed bool) domainmcp.LosslessToolResult {
	result := map[string]any{"content": []any{map[string]any{"type": "text", "text": value["message"]}}, "structuredContent": value}
	if failed {
		result["isError"] = true
	}
	raw, _ := json.Marshal(result)
	return mcpprotocol.ExtractLosslessToolResult(raw)
}

// Closed schemas preserve the existing Schedule bridge's provider arguments;
// Main remains the owner of calendar validation and committed task records.
func hostScheduleToolsV1() []ToolSpec {
	str := map[string]any{"type": "string"}
	nonempty := map[string]any{"type": "string", "minLength": 1}
	enum := func(values ...string) any { return map[string]any{"type": "string", "enum": values} }
	object := func(properties map[string]any, required ...string) map[string]any {
		if required == nil {
			required = []string{}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	rules := []any{
		object(map[string]any{"kind": enum("at"), "atTime": nonempty}, "kind", "atTime"),
		object(map[string]any{"kind": enum("daily"), "timeOfDay": str, "timeZone": map[string]any{"type": "string", "maxLength": 255}}, "kind", "timeOfDay", "timeZone"),
		object(map[string]any{"kind": enum("weekly"), "timeOfDay": str, "timeZone": map[string]any{"type": "string", "maxLength": 255}, "weekdays": map[string]any{"type": "array", "minItems": 1, "maxItems": 7, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 1, "maximum": 7}}}, "kind", "timeOfDay", "timeZone", "weekdays"),
		object(map[string]any{"kind": enum("cron"), "expression": map[string]any{"type": "string", "maxLength": 512}, "timeZone": map[string]any{"type": "string", "maxLength": 255}}, "kind", "expression", "timeZone"),
		object(map[string]any{"kind": enum("interval"), "everyMinutes": map[string]any{"type": "integer", "minimum": 1, "maximum": 10080}}, "kind", "everyMinutes"),
	}
	patch := map[string]any{"task_id": nonempty, "title": str, "prompt": str, "enabled": map[string]any{"type": "boolean"}, "workspace_root": str, "claw_channel_id": str, "provider_id": str, "model": str, "reasoning_effort": enum("auto", "off", "low", "medium", "high", "max"), "mode": enum("agent", "plan"), "schedule": map[string]any{"anyOf": append(append([]any{}, rules...), object(map[string]any{"kind": enum("manual")}, "kind"))}}
	create := map[string]any{}
	for k, v := range patch {
		if k != "task_id" {
			create[k] = v
		}
	}
	create["title"], create["prompt"], create["schedule"] = nonempty, nonempty, map[string]any{"anyOf": rules}
	inputs := map[string]any{"list": object(map[string]any{}), "create": object(create, "title", "prompt", "schedule"), "update": object(patch, "task_id"), "delete": object(map[string]any{"task_id": nonempty}, "task_id")}
	taskFields := map[string]any{"enabled": map[string]any{"type": "boolean"}, "schedule": object(map[string]any{"kind": enum("manual", "at", "daily", "interval", "weekly", "cron"), "everyMinutes": map[string]any{"type": "number"}, "timeOfDay": str, "atTime": str, "timeZone": map[string]any{"type": "string", "maxLength": 255}, "weekdays": map[string]any{"type": "array", "maxItems": 7, "items": map[string]any{"type": "integer", "minimum": 1, "maximum": 7}}, "expression": map[string]any{"type": "string", "maxLength": 512}}, "kind", "everyMinutes", "timeOfDay", "atTime")}
	for _, field := range []string{"id", "title", "prompt", "workspaceRoot", "clawChannelId", "model", "reasoningEffort", "mode", "createdAt", "updatedAt", "lastRunAt", "nextRunAt", "lastStatus", "lastMessage", "lastThreadId", "providerId"} {
		taskFields[field] = str
	}
	taskFields["reasoningEffort"] = enum("auto", "off", "low", "medium", "high", "max")
	taskFields["mode"] = enum("agent", "plan")
	taskFields["lastStatus"] = enum("idle", "running", "success", "error")
	requiredTask := []string{}
	for field := range taskFields {
		if field != "providerId" {
			requiredTask = append(requiredTask, field)
		}
	}
	// Stable field order is needed for frozen schema hashes.
	sort.Strings(requiredTask)
	task := object(taskFields, requiredTask...)
	base := func() map[string]any { return map[string]any{"status": enum("success", "failure"), "message": str} }
	listOut := base()
	listOut["tasks"] = map[string]any{"type": "array", "items": task}
	taskOut := base()
	taskOut["task"] = map[string]any{"anyOf": []any{task, map[string]any{"type": "null"}}}
	deleteOut := base()
	deleteOut["taskId"] = str
	outputs := map[string]any{"list": object(listOut, "status", "message", "tasks"), "create": object(taskOut, "status", "message", "task"), "update": object(taskOut, "status", "message", "task"), "delete": object(deleteOut, "status", "message", "taskId")}
	var tools []ToolSpec
	for _, operation := range []string{"list", "create", "update", "delete"} {
		for _, prefix := range []string{"claw", "gui"} {
			input, _ := json.Marshal(inputs[operation])
			output, _ := json.Marshal(outputs[operation])
			tools = append(tools, ToolSpec{Name: prefix + "_schedule_" + operation, Description: strings.ToUpper(operation[:1]) + operation[1:] + " scheduled tasks managed by the currently running Analytix app.", InputSchema: input, OutputSchema: output, ReadOnlyHint: operation == "list"})
		}
	}
	return tools
}
