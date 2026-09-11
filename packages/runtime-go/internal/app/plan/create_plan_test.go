package plan

import (
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestFallbackArgsUsesMatchingGUIPlan(t *testing.T) {
	guiPlan := map[string]any{
		"workspaceRoot": "/tmp/work",
		"operation":     "draft",
		"planId":        "plan-1",
		"relativePath":  CurrentRelativeDir + "/one.md",
		"sourceRequest": "from gui",
		"title":         "One",
	}
	args := FallbackArgs(FallbackInput{Prompt: "from prompt", GUIPlan: guiPlan, Workspace: "/tmp/work/"}, "  body  ")
	if args["markdown"] != "body" || args["operation"] != "draft" || args["plan_id"] != "plan-1" {
		t.Fatalf("fallback args did not preserve GUI plan context: %#v", args)
	}
	if stale := FallbackArgs(FallbackInput{Prompt: "from prompt", GUIPlan: guiPlan, Workspace: "/other"}, "body"); stale["plan_id"] != nil || stale["source_request"] != "from prompt" {
		t.Fatalf("stale GUI plan should fall back to prompt context: %#v", stale)
	}
}

func TestResolveReservedTargetGuardsIdentity(t *testing.T) {
	guiPlan := map[string]any{
		"workspaceRoot": "/tmp/work",
		"operation":     "refine",
		"planId":        "plan-1",
		"relativePath":  LegacyRelativeDir + "/old.md",
	}
	target, err := ResolveReservedTarget(map[string]any{"plan_id": "plan-1"}, guiPlan, "/tmp/work", "refine")
	if err != nil {
		t.Fatalf("expected legacy refine to be accepted: %v", err)
	}
	if target.RelativePath != LegacyRelativeDir+"/old.md" || target.PlanID != "plan-1" {
		t.Fatalf("unexpected target: %#v", target)
	}
	if _, err := ResolveReservedTarget(map[string]any{}, guiPlan, "/tmp/work", "draft"); err == nil || !strings.Contains(err.Error(), "operation") {
		t.Fatalf("expected operation mismatch, got %v", err)
	}
	guiPlan["operation"] = "draft"
	if _, err := ResolveReservedTarget(map[string]any{}, guiPlan, "/tmp/work", "draft"); err == nil || !strings.Contains(err.Error(), "legacy") {
		t.Fatalf("expected legacy draft rejection, got %v", err)
	}
}

func TestResolveFreeFormTargetUsesStableNextPath(t *testing.T) {
	now := time.UnixMilli(123456789)
	existing := map[string]bool{
		CurrentRelativeDir + "/new-plan.md":   true,
		CurrentRelativeDir + "/new-plan-2.md": true,
	}
	target, err := ResolveFreeFormTarget(map[string]any{"title": "New Plan"}, "/tmp/work", "draft", existing, now)
	if err != nil {
		t.Fatalf("resolve free-form target: %v", err)
	}
	if target.RelativePath != CurrentRelativeDir+"/new-plan-3.md" {
		t.Fatalf("unexpected next path: %s", target.RelativePath)
	}
	if target.PlanID != "/tmp/work:"+CurrentRelativeDir+"/new-plan-3.md" {
		t.Fatalf("unexpected plan id: %s", target.PlanID)
	}
}

func TestPlanSandboxAndResponse(t *testing.T) {
	if block := SandboxBlock("read-only", "/tmp/work/.analytixsdd/plan/a.md"); block["code"] != "sandbox_read_only" {
		t.Fatalf("expected read-only block, got %#v", block)
	}
	if !CanMaterializeInSandbox("workspace-write") || CanMaterializeInSandbox("read-only") {
		t.Fatalf("unexpected materialize sandbox decision")
	}
	response := ToolResponse(Target{
		WorkspaceRoot: "/tmp/work",
		RelativePath:  CurrentRelativeDir + "/a.md",
		PlanID:        "plan-a",
		Operation:     "draft",
	}, map[string]any{"title": "A"}, "hello", "/tmp/work/.analytixsdd/plan/a.md", "now")
	if response["summary"] != "Created GUI plan at .analytixsdd/plan/a.md." || response["content_hash"] == "" || response["byte_size"] != float64(5) {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestPlanModeToolAllowed(t *testing.T) {
	if !PlanModeToolAllowed("create_plan", 99, "create_plan") {
		t.Fatal("create_plan should stay allowed until satisfied")
	}
	if !PlanModeToolAllowed("read", 0, "create_plan") || !PlanModeToolAllowed("read", 99, "create_plan") {
		t.Fatal("read should stay available throughout bounded plan investigation")
	}
	if PlanModeToolAllowed("bash", 0, "create_plan") {
		t.Fatal("bash must not be allowed in plan mode investigation")
	}
}

func TestFilterModeToolSchemas(t *testing.T) {
	tools := []domainmodel.ToolSchema{
		{Name: "read"},
		{Name: "bash"},
		{Name: "create_plan"},
	}
	filtered := FilterModeToolSchemas(tools, true, false, 0, "create_plan")
	if len(filtered) != 2 || filtered[0].Name != "read" || filtered[1].Name != "create_plan" {
		t.Fatalf("step 0 plan tools mismatch: %#v", filtered)
	}
	filtered = FilterModeToolSchemas(tools, true, false, 1, "create_plan")
	if len(filtered) != 2 || filtered[0].Name != "read" || filtered[1].Name != "create_plan" {
		t.Fatalf("later plan tools mismatch: %#v", filtered)
	}
	if got := FilterModeToolSchemas(tools, true, true, 2, "create_plan"); len(got) != 2 || got[0].Name != "read" || got[1].Name != "create_plan" {
		t.Fatalf("satisfied plan should retain the stable read-only plan schema: %#v", got)
	}
	if got := FilterModeToolSchemas(tools, false, false, 0, "create_plan"); len(got) != len(tools) {
		t.Fatalf("inactive plan should preserve tools: %#v", got)
	}
}
