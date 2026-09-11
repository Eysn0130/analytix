package toolcatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSecurityScopedCatalogKeepsOrdinaryBaseAndGatesOnlyCaseEffects(t *testing.T) {
	general, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-additive-general", TurnID: "turn-additive-general", WorkspaceRealPath: "/workspace/general",
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	schemas := []domainmodel.ToolSchema{
		{Name: "read", Source: "builtin"},
		{Name: "write_file", Source: "builtin"},
		{Name: "bash", Source: "builtin"},
		{Name: ForegroundTaskToolName, Source: "subagent"},
		{Name: "mcp__docs__lookup", Source: "mcp"},
		{Name: "mcp__analytix_funds__count_case_rows", Source: "mcp"},
		{Name: "mcp__analytix_funds__run_full_case_analysis", Source: "mcp"},
		{Name: ReportDeliveryToolName, Source: "builtin"},
	}
	got := SecurityScopedToolSchemas(general, 0, schemas)
	if names := toolNames(got); !reflect.DeepEqual(names, []string{
		"read", "write_file", "bash", ForegroundTaskToolName, "mcp__docs__lookup",
	}) {
		t.Fatalf("general context replaced the ordinary catalog or exposed a case effect: %#v", names)
	}
	caseContext := toolCatalogCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-additive-case", TurnID: "turn-additive-case", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-additive", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-additive")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-additive"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-additive")),
		ContextEpoch:       2, IssuedAt: time.Now().UTC(),
	})
	got = SecurityScopedToolSchemas(caseContext, 0, schemas)
	if names := toolNames(got); !reflect.DeepEqual(names, []string{
		"read", "write_file", "bash", ForegroundTaskToolName, "mcp__docs__lookup",
		"mcp__analytix_funds__count_case_rows", ReportDeliveryToolName,
	}) {
		t.Fatalf("case capability did not add to the ordinary catalog: %#v", names)
	}
	if invalid := SecurityScopedToolSchemas(domainsecurity.TurnSecurityContext{}, 0, schemas); invalid != nil {
		t.Fatalf("invalid security context returned a provider catalog: %#v", invalid)
	}
}

func TestSecurityScopedCatalogWitnessedBoundaryKeepsOnlyOrdinaryBase(t *testing.T) {
	securityContext := toolCatalogWitnessedBoundaryContext(t, "catalog")
	schemas := []domainmodel.ToolSchema{
		{Name: "read", Source: "builtin"},
		{Name: "bash", Source: "builtin"},
		{Name: ForegroundTaskToolName, Source: "subagent"},
		{Name: "mcp__docs__lookup", Source: "mcp"},
		{Name: "mcp__analytix_funds__count_case_rows", Source: "mcp"},
		{Name: ReportDeliveryToolName, Source: "builtin"},
	}
	if names := toolNames(SecurityScopedToolSchemas(securityContext, 0, schemas)); !reflect.DeepEqual(names, []string{
		"read", "bash", ForegroundTaskToolName, "mcp__docs__lookup",
	}) {
		t.Fatalf("witnessed case boundary replaced ordinary tools or exposed protected effects: %#v", names)
	}
}

func TestForegroundChildMaterializesOnlyRequiredSubmitTool(t *testing.T) {
	tools := MaterializeToolSchemas(MaterializeInput{
		PromptRoute: RouteToolAgent, Subagent: true,
		SubagentsEnabled: true, SkillToolsActive: true, GoalToolsActive: true,
		ToolScope: []string{ForegroundSubmitToolName},
	})
	if len(tools) != 1 || tools[0].Name != ForegroundSubmitToolName || tools[0].Source != "subagent" {
		t.Fatalf("foreground child catalog is not submit-only: %#v", tools)
	}
	var decoded struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		MinProperties        int                        `json:"minProperties"`
		MaxProperties        int                        `json:"maxProperties"`
		AdditionalProperties bool                       `json:"additionalProperties"`
	}
	if err := json.Unmarshal(tools[0].Parameters, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AdditionalProperties || len(decoded.Properties) != 2 || len(decoded.Required) != 0 ||
		decoded.MinProperties != 1 || decoded.MaxProperties != 1 ||
		decoded.Properties["result"] == nil || decoded.Properties["caseResult"] == nil {
		t.Fatalf("submit schema is not exact: %#v", decoded)
	}
	var caseResult struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		AdditionalProperties bool                       `json:"additionalProperties"`
	}
	if err := json.Unmarshal(decoded.Properties["caseResult"], &caseResult); err != nil {
		t.Fatal(err)
	}
	if caseResult.AdditionalProperties || len(caseResult.Properties) != 4 ||
		!reflect.DeepEqual(caseResult.Required, []string{"schemaVersion", "purpose", "delegationDigest", "answerSlotDigest"}) ||
		caseResult.Properties["delegationDigest"] == nil || caseResult.Properties["answerSlotDigest"] == nil {
		t.Fatalf("case selection is not exact and value-free: %#v", caseResult)
	}
	for _, field := range []string{"delegationDigest", "answerSlotDigest"} {
		var selector struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(caseResult.Properties[field], &selector); err != nil {
			t.Fatal(err)
		}
		if selector.Pattern != `^cmt1_[a-p]{64}$` {
			t.Fatalf("%s permits a sanitizer-unstable provider selector: %#v", field, selector)
		}
	}
	for _, forbidden := range []string{
		`"entityAliases"`, `"claims"`, `"evidence"`, `"gaps"`, `"answerSlots"`,
		`"inflowMinor"`, `"outflowMinor"`, `"netMinor"`, `"transactionCount"`,
		`"queryHash"`, `"resultHash"`, `"queryScopeRef"`,
	} {
		if strings.Contains(string(tools[0].Parameters), forbidden) {
			t.Fatalf("submit schema exposed host-private typed field %q: %s", forbidden, tools[0].Parameters)
		}
	}
}

func TestSubagentAndJobToolSchemasKeepDurableIdentityFields(t *testing.T) {
	schemas := SubagentToolSchemas(" Profiles: reviewer.")
	if len(schemas) != 3 {
		t.Fatalf("subagent schema count drifted: %d", len(schemas))
	}
	if schemas[0].Name != "task" || schemas[1].Name != "parallel_tasks" || schemas[2].Name != "delegate_task" {
		t.Fatalf("subagent schema order should prefer current Analytix tools before legacy aliases: %#v", []string{schemas[0].Name, schemas[1].Name, schemas[2].Name})
	}
	for _, schema := range schemas {
		if schema.Source != "subagent" || !strings.Contains(schema.Description, "Profiles: reviewer.") {
			t.Fatalf("subagent schema metadata drifted: %#v", schema)
		}
		if !strings.Contains(schema.Description, "Omit profile unless") ||
			!strings.Contains(schema.Description, "general file, directory, workspace, or desktop analysis") {
			t.Fatalf("subagent schema should discourage generic profile use: %#v", schema.Description)
		}
	}
	if !strings.Contains(schemaByName(t, schemas, "task").Description, "Recommended Analytix tool") ||
		!strings.Contains(schemaByName(t, schemas, "delegate_task").Description, "Legacy compatibility alias") {
		t.Fatalf("subagent descriptions should prefer task and demote delegate_task")
	}
	if !schemaHasProperty(t, schemaByName(t, schemas, "task"), "run_in_background") {
		t.Fatalf("task schema must keep background execution field")
	}
	if !schemaHasProperty(t, schemaByName(t, schemas, "task"), "isolationMode") ||
		!schemaHasProperty(t, schemaByName(t, schemas, "task"), "isolation_mode") {
		t.Fatalf("task schema must expose worktree isolation fields")
	}
	if schemaHasProperty(t, schemaByName(t, schemas, "delegate_task"), "isolationMode") ||
		schemaHasProperty(t, schemaByName(t, schemas, "parallel_tasks"), "isolationMode") {
		t.Fatalf("foreground or parallel schemas must not advertise worktree isolation")
	}
	for _, schemaName := range []string{"task", "delegate_task"} {
		if !schemaHasProperty(t, schemaByName(t, schemas, schemaName), "toolPolicy") ||
			!schemaHasProperty(t, schemaByName(t, schemas, schemaName), "tool_policy") ||
			!schemaHasProperty(t, schemaByName(t, schemas, schemaName), "maxSteps") {
			t.Fatalf("%s schema must expose subagent toolPolicy and maxSteps fields", schemaName)
		}
	}
	if !parallelTaskItemHasProperty(t, schemaByName(t, schemas, "parallel_tasks"), "toolPolicy") ||
		!parallelTaskItemHasProperty(t, schemaByName(t, schemas, "parallel_tasks"), "tool_policy") ||
		!parallelTaskItemHasProperty(t, schemaByName(t, schemas, "parallel_tasks"), "maxSteps") ||
		!parallelTaskItemHasProperty(t, schemaByName(t, schemas, "parallel_tasks"), "continue_from") ||
		!parallelTaskItemHasProperty(t, schemaByName(t, schemas, "parallel_tasks"), "fork_from") {
		t.Fatalf("parallel_tasks schema must expose per-task policy, continuation, and fork fields")
	}
	if !schemaHasProperty(t, schemaByName(t, schemas, "delegate_task"), "continue_from") ||
		!schemaHasProperty(t, schemaByName(t, schemas, "delegate_task"), "fork_from") {
		t.Fatalf("delegate_task schema must keep continuation/fork fields")
	}

	jobs := JobToolSchemas()
	if len(jobs) != 5 {
		t.Fatalf("job schema count drifted: %d", len(jobs))
	}
	for _, schema := range jobs {
		if schema.Source != "job" {
			t.Fatalf("job schema source drifted: %#v", schema)
		}
	}
	for _, schemaName := range []string{"wait", "bash_output", "kill_shell", "restart_job", "list_jobs"} {
		if schemaByName(t, jobs, schemaName).Name != schemaName {
			t.Fatalf("job schema %s missing", schemaName)
		}
	}
	if !schemaHasProperty(t, schemaByName(t, jobs, "bash_output"), "cursor") {
		t.Fatalf("bash_output schema must keep cursor continuation field")
	}
	if !schemaHasProperty(t, schemaByName(t, jobs, "bash_output"), "tail") ||
		!schemaHasProperty(t, schemaByName(t, jobs, "bash_output"), "since") {
		t.Fatalf("bash_output schema must expose tail and since controls")
	}
	if !schemaHasProperty(t, schemaByName(t, jobs, "list_jobs"), "status") ||
		!schemaHasProperty(t, schemaByName(t, jobs, "list_jobs"), "background") {
		t.Fatalf("list_jobs schema must expose status and background filters")
	}
	if !schemaHasProperty(t, schemaByName(t, jobs, "restart_job"), "jobId") {
		t.Fatalf("restart_job schema must expose camelCase job id alias")
	}
	for _, hostOnly := range []string{"steer_job", "pause_job", "resume_job"} {
		for _, schema := range jobs {
			if schema.Name == hostOnly {
				t.Fatalf("host-only control %s was advertised to the provider", hostOnly)
			}
		}
	}
}

func parallelTaskItemHasProperty(t *testing.T, schema domainmodel.ToolSchema, property string) bool {
	t.Helper()
	var decoded struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema.Parameters, &decoded); err != nil {
		t.Fatalf("decode parallel schema: %v", err)
	}
	var tasks struct {
		Items struct {
			Properties map[string]any `json:"properties"`
		} `json:"items"`
	}
	if err := json.Unmarshal(decoded.Properties["tasks"], &tasks); err != nil {
		t.Fatalf("decode parallel tasks schema: %v", err)
	}
	return tasks.Items.Properties[property] != nil
}

func TestRunSkillToolSchemaKeepsSubagentContinuationFields(t *testing.T) {
	schema := RunSkillToolSchema()
	if schema.Name != "run_skill" || schema.Source != "skill" {
		t.Fatalf("skill schema identity drifted: %#v", schema)
	}
	var decoded struct {
		Required []string       `json:"required"`
		Props    map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema.Parameters, &decoded); err != nil {
		t.Fatalf("decode skill schema: %v", err)
	}
	if len(decoded.Required) != 1 || decoded.Required[0] != "name" {
		t.Fatalf("skill schema required fields drifted: %#v", decoded.Required)
	}
	for _, property := range []string{"continue_from", "fork_from", "providerId", "endpointFormat"} {
		if decoded.Props[property] == nil {
			t.Fatalf("skill schema missing %s: %#v", property, decoded.Props)
		}
	}
}
