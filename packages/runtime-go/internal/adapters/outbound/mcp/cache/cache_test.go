package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

func TestCacheSaveLoadCanonicalizesSchemaWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	spec := domainmcp.ServerSpec{
		ID:        "case-server",
		Transport: "streamable-http",
		URL:       "https://token@example.invalid/mcp?key=secret",
		Headers:   map[string]string{"Authorization": "Bearer secret"},
		Env:       map[string]string{"MCP_API_KEY": "secret"},
		ReadOnlyToolNames: map[string]bool{
			"lookup": true,
		},
	}
	hash := SpecFingerprint(spec)
	tools, err := CacheableTools([]domainmcp.ToolSpec{
		{Name: "lookup", Description: "Lookup", InputSchema: json.RawMessage(`{
			"required":["b","a"],
			"type":"object",
			"properties":{
				"a":{"type":"string"},
				"b":{"type":"object","properties":{"value":{"type":"string"}}}
			}
		}`), OutputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"rows":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"}}}}},
			"required":["rows"]
		}`)},
	}, spec)
	if err != nil || len(tools) != 1 {
		t.Fatalf("valid closed cache tool was rejected: tools=%#v err=%v", tools, err)
	}
	if err := Save(dir, spec.ID, Schema{SpecHash: hash, Tools: tools}); err != nil {
		t.Fatalf("save schema: %v", err)
	}
	data, err := os.ReadFile(Path(dir, spec.ID))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "token@example") {
		t.Fatalf("cached schema persisted transport secrets: %s", string(data))
	}
	loaded, ok := Load(dir, spec.ID, hash)
	if !ok {
		t.Fatalf("expected cached schema hit")
	}
	if len(loaded.Tools) != 1 || !loaded.Tools[0].ReadOnlyHint || loaded.Tools[0].TaskSupport != domainmcp.ToolTaskSupportForbidden {
		t.Fatalf("loaded tool metadata mismatch: %#v", loaded.Tools)
	}
	if got := string(loaded.Tools[0].InputSchema); got != `{"additionalProperties":false,"properties":{"a":{"type":"string"},"b":{"additionalProperties":false,"properties":{"value":{"type":"string"}},"type":"object"}},"required":["a","b"],"type":"object"}` {
		t.Fatalf("schema was not canonicalized: %s", got)
	}
	if got := string(loaded.Tools[0].OutputSchema); got != `{"additionalProperties":false,"properties":{"rows":{"items":{"additionalProperties":false,"properties":{"id":{"type":"string"}},"type":"object"},"type":"array"}},"required":["rows"],"type":"object"}` {
		t.Fatalf("output schema was not canonicalized: %s", got)
	}
}

func TestCacheableToolsRejectsMissingOrOpenEndedSchemas(t *testing.T) {
	valid := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	for name, tool := range map[string]domainmcp.ToolSpec{
		"missing input":     {Name: "lookup", OutputSchema: valid},
		"non object input":  {Name: "lookup", InputSchema: json.RawMessage(`{"type":"string"}`), OutputSchema: valid},
		"open input":        {Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`), OutputSchema: valid},
		"missing output":    {Name: "lookup", InputSchema: valid},
		"non object output": {Name: "lookup", InputSchema: valid, OutputSchema: json.RawMessage(`{"type":"string"}`)},
		"open output":       {Name: "lookup", InputSchema: valid, OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)},
	} {
		t.Run(name, func(t *testing.T) {
			tools, err := CacheableTools([]domainmcp.ToolSpec{tool}, domainmcp.ServerSpec{})
			if err == nil || len(tools) != 0 {
				t.Fatalf("invalid schema must not enter the MCP cache: tools=%#v err=%v", tools, err)
			}
		})
	}
}

func TestCacheBindsTaskSupportAndRejectsInvalidValue(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	spec := domainmcp.ServerSpec{ID: "task-cache"}
	tools, err := CacheableTools([]domainmcp.ToolSpec{{
		Name: "async_lookup", InputSchema: closed, OutputSchema: closed, TaskSupport: domainmcp.ToolTaskSupportRequired,
	}}, spec)
	if err != nil || len(tools) != 1 || tools[0].TaskSupport != domainmcp.ToolTaskSupportRequired {
		t.Fatalf("task support was not preserved in cache normalization: tools=%#v err=%v", tools, err)
	}
	dir := t.TempDir()
	hash := SpecFingerprint(spec)
	if err := Save(dir, spec.ID, Schema{SpecHash: hash, Tools: tools}); err != nil {
		t.Fatal(err)
	}
	loaded, ok := Load(dir, spec.ID, hash)
	if !ok || len(loaded.Tools) != 1 || loaded.Tools[0].TaskSupport != domainmcp.ToolTaskSupportRequired {
		t.Fatalf("cached task support was not load-bearing: loaded=%#v ok=%v", loaded, ok)
	}
	if invalid, err := CacheableTools([]domainmcp.ToolSpec{{
		Name: "invalid", InputSchema: closed, OutputSchema: closed, TaskSupport: "sometimes",
	}}, spec); err == nil || len(invalid) != 0 {
		t.Fatalf("invalid task support entered cache: tools=%#v err=%v", invalid, err)
	}
}

func TestLoadRejectsV2MissingOutputAndUnknownFields(t *testing.T) {
	dir := t.TempDir()
	serverID := "legacy"
	hash := "spec"
	path := Path(dir, serverID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"v2":                   `{"version":2,"specHash":"spec","tools":[],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"v3 missing output":    `{"version":3,"specHash":"spec","tools":[{"name":"x","description":"","inputSchema":{"type":"object","additionalProperties":false},"readOnlyHint":false,"resultText":""}],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"v3 unknown field":     `{"version":3,"specHash":"spec","tools":[],"lastValidated":"2026-07-11T00:00:00Z","secret":"x"}`,
		"duplicate spec hash":  `{"version":3,"specHash":"spec","specHash":"forged","tools":[],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"case variant version": `{"Version":3,"specHash":"spec","tools":[],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"duplicate tool name":  `{"version":3,"specHash":"spec","tools":[{"name":"x","name":"forged","description":"","inputSchema":{"type":"object","additionalProperties":false},"outputSchema":{"type":"object","additionalProperties":false},"readOnlyHint":false,"resultText":""}],"lastValidated":"2026-07-11T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if schema, ok := Load(dir, serverID, hash); ok {
				t.Fatalf("unsafe cache was accepted: %#v", schema)
			}
		})
	}
}

func TestLoadRejectsOversizedCache(t *testing.T) {
	dir := t.TempDir()
	path := Path(dir, "oversized")
	if err := os.WriteFile(path, append([]byte(`{"version":3,"specHash":"spec","tools":[],"lastValidated":"2026-07-11T00:00:00Z"}`), make([]byte, maxCacheBytes)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if schema, ok := Load(dir, "oversized", "spec"); ok {
		t.Fatalf("oversized cache was accepted: %#v", schema)
	}
}

func TestLoadRejectsNullAndWrongTypedCacheFields(t *testing.T) {
	dir := t.TempDir()
	serverID := "typed-cache"
	hash := strings.Repeat("a", 64)
	path := Path(dir, serverID)
	validTool := `{"name":"lookup","description":"","inputSchema":{"type":"object","additionalProperties":false},"outputSchema":{"type":"object","additionalProperties":false},"readOnlyHint":false,"resultText":"","taskSupport":"forbidden"}`
	for name, body := range map[string]string{
		"null version":      `{"version":null,"specHash":"` + hash + `","tools":[],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"null spec hash":    `{"version":5,"specHash":null,"tools":[],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"null tools":        `{"version":5,"specHash":"` + hash + `","tools":null,"lastValidated":"2026-07-11T00:00:00Z"}`,
		"null timestamp":    `{"version":5,"specHash":"` + hash + `","tools":[],"lastValidated":null}`,
		"null description":  `{"version":5,"specHash":"` + hash + `","tools":[` + strings.Replace(validTool, `"description":""`, `"description":null`, 1) + `],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"null readonly":     `{"version":5,"specHash":"` + hash + `","tools":[` + strings.Replace(validTool, `"readOnlyHint":false`, `"readOnlyHint":null`, 1) + `],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"null result text":  `{"version":5,"specHash":"` + hash + `","tools":[` + strings.Replace(validTool, `"resultText":""`, `"resultText":null`, 1) + `],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"null task support": `{"version":5,"specHash":"` + hash + `","tools":[` + strings.Replace(validTool, `"taskSupport":"forbidden"`, `"taskSupport":null`, 1) + `],"lastValidated":"2026-07-11T00:00:00Z"}`,
		"wrong schema type": `{"version":5,"specHash":"` + hash + `","tools":[` + strings.Replace(validTool, `"inputSchema":{"type":"object","additionalProperties":false}`, `"inputSchema":[]`, 1) + `],"lastValidated":"2026-07-11T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if schema, ok := Load(dir, serverID, hash); ok {
				t.Fatalf("wrong-typed cache was accepted: %#v", schema)
			}
		})
	}
}

func TestLoadQuarantinesInvalidSchemaAndKeepsValidSibling(t *testing.T) {
	dir := t.TempDir()
	serverID := "sibling-cache"
	hash := strings.Repeat("a", 64)
	path := Path(dir, serverID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	closed := `{"type":"object","additionalProperties":false}`
	tool := func(name, input, output string) string {
		return `{"name":"` + name + `","description":"","inputSchema":` + input + `,"outputSchema":` + output + `,"readOnlyHint":false,"resultText":"","taskSupport":"forbidden"}`
	}
	body := `{"version":5,"specHash":"` + hash + `","tools":[` +
		tool("invalid", `{"type":"object","additionalProperties":true}`, closed) + `,` +
		tool("missing", `null`, closed) + `,` +
		tool("valid", closed, closed) +
		`],"lastValidated":"2026-07-11T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, ok := Load(dir, serverID, hash)
	if !ok || len(loaded.Tools) != 1 || loaded.Tools[0].Name != "valid" {
		t.Fatalf("valid cached sibling was not preserved: loaded=%#v ok=%v", loaded, ok)
	}
	wantIssues := []mcpprotocol.ToolContractIssue{
		{Name: "invalid", Code: mcpprotocol.ToolContractInvalidInputSchema},
		{Name: "missing", Code: mcpprotocol.ToolContractMissingInputSchema},
	}
	if len(loaded.Quarantined) != len(wantIssues) ||
		loaded.Quarantined[0] != wantIssues[0] || loaded.Quarantined[1] != wantIssues[1] {
		t.Fatalf("cached quarantine diagnostics were lost: %#v", loaded.Quarantined)
	}

	allInvalid := strings.Replace(body, tool("valid", closed, closed), tool("also_invalid", closed, `{"type":"array"}`), 1)
	if err := os.WriteFile(path, []byte(allInvalid), 0o600); err != nil {
		t.Fatal(err)
	}
	if loaded, ok := Load(dir, serverID, hash); ok {
		t.Fatalf("cache with no valid tool contracts was accepted: %#v", loaded)
	}
}

func TestLoadRejectsDuplicateIdentityEvenWhenOneSchemaIsInvalid(t *testing.T) {
	dir := t.TempDir()
	serverID := "duplicate-cache"
	hash := strings.Repeat("b", 64)
	path := Path(dir, serverID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	closed := `{"type":"object","additionalProperties":false}`
	tool := func(input string) string {
		return `{"name":"same","description":"","inputSchema":` + input + `,"outputSchema":` + closed + `,"readOnlyHint":false,"resultText":"","taskSupport":"forbidden"}`
	}
	body := `{"version":5,"specHash":"` + hash + `","tools":[` +
		tool(`{"type":"string"}`) + `,` + tool(closed) +
		`],"lastValidated":"2026-07-11T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if loaded, ok := Load(dir, serverID, hash); ok {
		t.Fatalf("duplicate cached identity was hidden by quarantine: %#v", loaded)
	}
}

func TestSpecFingerprintIsStableAndLoadBearing(t *testing.T) {
	base := domainmcp.ServerSpec{
		ID:        "srv",
		Transport: "stdio",
		Command:   "node",
		Args:      []string{"server.mjs"},
		Env:       map[string]string{"B": "2", "A": "1"},
		Headers:   map[string]string{"Y": "2", "X": "1"},
		ReadOnlyToolNames: map[string]bool{
			"b": false,
			"a": true,
		},
	}
	reordered := base
	reordered.Env = map[string]string{"A": "1", "B": "2"}
	reordered.Headers = map[string]string{"X": "1", "Y": "2"}
	reordered.ReadOnlyToolNames = map[string]bool{"a": true, "b": false}
	if SpecFingerprint(base) != SpecFingerprint(reordered) {
		t.Fatalf("fingerprint must be stable across map iteration order")
	}
	changed := base
	changed.Env = map[string]string{"A": "changed", "B": "2"}
	if SpecFingerprint(base) == SpecFingerprint(changed) {
		t.Fatalf("fingerprint must change when load-bearing env changes")
	}
	for name, mutate := range map[string]func(*domainmcp.ServerSpec){
		"plugin root": func(spec *domainmcp.ServerSpec) { spec.PluginRootPath = "/plugins/funds" },
		"source tree": func(spec *domainmcp.ServerSpec) { spec.SourceTreeSHA256 = strings.Repeat("a", 64) },
		"timeout":     func(spec *domainmcp.ServerSpec) { spec.TimeoutMS = 30_000 },
		"fixture tools": func(spec *domainmcp.ServerSpec) {
			spec.Tools = []domainmcp.ToolSpec{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			variant := base
			mutate(&variant)
			if SpecFingerprint(base) == SpecFingerprint(variant) {
				t.Fatalf("fingerprint ignored %s", name)
			}
		})
	}
}

func TestPathRejectsNonCanonicalServerID(t *testing.T) {
	if path := Path("/tmp/cache-root", "../server with spaces"); path != "" {
		t.Fatalf("noncanonical server ID received a cache path: %s", path)
	}
	path := Path("/tmp/cache-root", "server-with-spaces")
	if filepath.Dir(path) != "/tmp/cache-root" {
		t.Fatalf("cache path escaped root: %s", path)
	}
	if strings.Contains(filepath.Base(path), "..") || strings.Contains(filepath.Base(path), " ") {
		t.Fatalf("cache basename was not sanitized: %s", path)
	}
}

func TestCacheRejectsAmbiguousCanonicalToolNamesAndBindsExactServerID(t *testing.T) {
	closed := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	if tools, err := CacheableTools([]domainmcp.ToolSpec{{Name: "bad name", InputSchema: closed, OutputSchema: closed}}, domainmcp.ServerSpec{ID: "foo"}); err == nil || len(tools) != 0 {
		t.Fatalf("invalid tool entered cache: tools=%#v err=%v", tools, err)
	}
	if path := Path(t.TempDir(), "foo__bar"); path != "" {
		t.Fatalf("ambiguous server ID received cache path: %q", path)
	}
	dir := t.TempDir()
	left := Path(dir, "foo-bar")
	right := Path(dir, "foo_bar")
	if left == "" || right == "" || left == right {
		t.Fatalf("cache paths do not bind exact server identity: left=%q right=%q", left, right)
	}
	base := domainmcp.ServerSpec{ID: "foo-bar", Transport: "stdio"}
	changed := base
	changed.ID = "foo_bar"
	if SpecFingerprint(base) == SpecFingerprint(changed) {
		t.Fatal("cache spec fingerprint ignored exact server ID")
	}
	baseName := strings.TrimSuffix(filepath.Base(left), ".json")
	separator := strings.LastIndex(baseName, "-")
	if separator < 0 || len(baseName[separator+1:]) != 64 {
		t.Fatalf("cache path does not contain a full server identity digest: %q", left)
	}
}
