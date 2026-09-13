package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	appplan "analytix.local/runtime-go/internal/app/plan"
	sideeffectidentityapp "analytix.local/runtime-go/internal/app/sideeffectidentity"
	"golang.org/x/text/unicode/norm"
)

func TestCreatePlanRejectsUnrepresentableTargetBeforeWriting(t *testing.T) {
	for _, route := range []string{"free-explicit", "free-auto", "reserved-explicit-id", "reserved-derived-id"} {
		t.Run(route, func(t *testing.T) {
			workspace := t.TempDir()
			for len(workspace) <= 256 {
				workspace = filepath.Join(workspace, strings.Repeat("w", 40))
			}
			if err := os.MkdirAll(workspace, 0o700); err != nil {
				t.Fatal(err)
			}
			const relative = appplan.CurrentRelativeDir + "/kept.md"
			input := CreatePlanToolInput{
				Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
				Args: map[string]any{"operation": "draft", "markdown": "new bytes", "title": "kept"},
			}
			if route != "free-auto" {
				input.Args["plan_relative_path"] = relative
			}
			if strings.HasPrefix(route, "reserved-") {
				input.GUIPlan = map[string]any{"workspaceRoot": workspace, "operation": "draft", "relativePath": relative}
				if route == "reserved-explicit-id" {
					input.GUIPlan["planId"] = strings.Repeat("i", 257)
				}
			}
			for _, existing := range []bool{false, true} {
				planPath := filepath.Join(workspace, filepath.FromSlash(relative))
				if existing {
					if err := os.MkdirAll(filepath.Dir(planPath), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(planPath, []byte("retained bytes"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				prepared, failure, failed := PrepareCreatePlanTool(input)
				if !failed || failure["code"] != "validation_error" || !reflect.DeepEqual(prepared, PreparedCreatePlanTool{}) {
					t.Fatal("unrepresentable plan target acquired prepared write authority")
				}
				if existing {
					body, err := os.ReadFile(planPath)
					entries, listErr := os.ReadDir(filepath.Dir(planPath))
					if err != nil || string(body) != "retained bytes" || listErr != nil || len(entries) != 1 {
						t.Fatal("rejected plan changed existing bytes or selected another physical target")
					}
				} else if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd")); !os.IsNotExist(err) {
					t.Fatal("rejected plan created a directory or file")
				}
			}
		})
	}
}

func TestExecuteCreatePlanToolWritesFreeFormPlan(t *testing.T) {
	workspace := t.TempDir()
	output, isError := ExecuteCreatePlanTool(CreatePlanToolInput{
		Workspace:   workspace,
		Mode:        "plan",
		SandboxMode: "workspace-write",
		Args: map[string]any{
			"operation":      "draft",
			"markdown":       "plan body",
			"title":          "New Feature",
			"source_request": "make a plan",
		},
		Now: func() time.Time { return time.UnixMilli(123456789).UTC() },
	})
	if isError {
		t.Fatalf("create_plan returned error: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["relative_path"] != appplan.CurrentRelativeDir+"/new-feature.md" || record["operation"] != "draft" {
		t.Fatalf("create_plan output mismatch: %#v", output)
	}
	path := filepath.Join(workspace, filepath.FromSlash(appplan.CurrentRelativeDir), "new-feature.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created plan: %v", err)
	}
	if string(data) != "plan body" {
		t.Fatalf("created plan content = %q", string(data))
	}
}

func TestExecuteCreatePlanToolUsesReservedGUIPlan(t *testing.T) {
	workspace := t.TempDir()
	guiPlan := map[string]any{
		"workspaceRoot": workspace,
		"operation":     "draft",
		"planId":        "plan-1",
		"relativePath":  appplan.CurrentRelativeDir + "/reserved.md",
	}
	output, isError := ExecuteCreatePlanTool(CreatePlanToolInput{
		Workspace:   workspace,
		GUIPlan:     guiPlan,
		SandboxMode: "workspace-write",
		Args: map[string]any{
			"operation":          "draft",
			"markdown":           "reserved body",
			"plan_id":            "plan-1",
			"plan_relative_path": appplan.CurrentRelativeDir + "/reserved.md",
		},
	})
	if isError {
		t.Fatalf("reserved create_plan returned error: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["plan_id"] != "plan-1" || record["relative_path"] != appplan.CurrentRelativeDir+"/reserved.md" {
		t.Fatalf("reserved create_plan output mismatch: %#v", output)
	}
}

func TestPreparedCreatePlanRejectsTargetDriftWithoutReResolvingAnotherPath(t *testing.T) {
	workspace := t.TempDir()
	input := CreatePlanToolInput{
		Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
		Args: map[string]any{
			"operation": "draft", "markdown": "prepared body", "title": "New Feature",
		},
		Now: func() time.Time { return time.UnixMilli(123456789).UTC() },
	}
	prepared, output, failed := PrepareCreatePlanTool(input)
	if failed {
		t.Fatalf("prepare create_plan: %#v", output)
	}
	projection, err := PreparedCreatePlanSemanticEffectProjection(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if projection["target"] == "" {
		t.Fatalf("unexpected stable plan projection: %#v", projection)
	}
	preparedPath := prepared.resolved.absolutePath
	if err := os.MkdirAll(filepath.Dir(preparedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preparedPath, []byte("concurrent owner"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result, isError := ExecutePreparedCreatePlanTool(prepared); !isError {
		t.Fatalf("prepared plan ignored target drift: %#v", result)
	}
	data, err := os.ReadFile(preparedPath)
	if err != nil || string(data) != "concurrent owner" {
		t.Fatalf("prepared plan overwrote changed target: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(preparedPath), "new-feature-2.md")); !os.IsNotExist(err) {
		t.Fatalf("prepared plan silently re-resolved another target: %v", err)
	}
}

func TestPreparedAutoCreatePlanRejectsCreationBetweenSelectionAndExecution(t *testing.T) {
	workspace := t.TempDir()
	prepared, output, failed := PrepareCreatePlanTool(CreatePlanToolInput{
		Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
		Args: map[string]any{"operation": "draft", "markdown": "prepared body", "title": "Race Safe"},
	})
	if failed {
		t.Fatalf("prepare auto create_plan: %#v", output)
	}
	if err := os.MkdirAll(filepath.Dir(prepared.resolved.absolutePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prepared.resolved.absolutePath, []byte("concurrent owner"), 0o644); err != nil {
		t.Fatal(err)
	}
	if result, isError := ExecutePreparedCreatePlanTool(prepared); !isError {
		t.Fatalf("prepared auto plan overwrote a post-selection owner: %#v", result)
	}
	if data, err := os.ReadFile(prepared.resolved.absolutePath); err != nil || string(data) != "concurrent owner" {
		t.Fatalf("post-selection owner changed: data=%q err=%v", data, err)
	}
}

func TestCreatePlanSemanticProjectionDoesNotChangeAfterFirstAutoTargetExists(t *testing.T) {
	workspace := t.TempDir()
	input := CreatePlanToolInput{
		Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
		Args: map[string]any{
			"operation": "draft", "markdown": "stable body", "title": "Stable Feature",
		},
		Now: func() time.Time { return time.UnixMilli(123456789).UTC() },
	}
	first, output, failed := PrepareCreatePlanTool(input)
	if failed {
		t.Fatalf("prepare first create_plan: %#v", output)
	}
	firstProjection, err := PreparedCreatePlanSemanticEffectProjection(first)
	if err != nil {
		t.Fatal(err)
	}
	if output, failed := ExecutePreparedCreatePlanTool(first); failed {
		t.Fatalf("execute first create_plan: %#v", output)
	}
	second, output, failed := PrepareCreatePlanTool(input)
	if failed {
		t.Fatalf("prepare duplicate create_plan: %#v", output)
	}
	secondProjection, err := PreparedCreatePlanSemanticEffectProjection(second)
	if err != nil {
		t.Fatal(err)
	}
	if first.resolved.absolutePath != second.resolved.absolutePath {
		t.Fatalf("identical auto request did not reuse its exact-content target: first=%q second=%q", first.resolved.absolutePath, second.resolved.absolutePath)
	}
	if firstProjection["target"] == "" {
		t.Fatalf("unexpected first semantic projection: %#v", firstProjection)
	}
	if !reflect.DeepEqual(firstProjection, secondProjection) {
		t.Fatalf("identical logical plan changed identity after first write: first=%#v second=%#v", firstProjection, secondProjection)
	}
}

func TestCreatePlanSemanticProjectionCannotBypassAutoWithExplicitTarget(t *testing.T) {
	workspace := t.TempDir()
	prepare := func(args map[string]any) map[string]any {
		t.Helper()
		prepared, output, failed := PrepareCreatePlanTool(CreatePlanToolInput{
			Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write", Args: args,
		})
		if failed {
			t.Fatalf("prepare create_plan: %#v", output)
		}
		projection, err := PreparedCreatePlanSemanticEffectProjection(prepared)
		if err != nil {
			t.Fatal(err)
		}
		return projection
	}
	auto := prepare(map[string]any{
		"operation": "draft", "markdown": "same body", "title": "Stable Feature",
	})
	explicit := prepare(map[string]any{
		"operation": "draft", "markdown": "same body",
		"plan_relative_path": appplan.CurrentRelativeDir + "/stable-feature.md",
	})
	if !reflect.DeepEqual(auto, explicit) {
		t.Fatalf("request-shape alias changed one physical plan identity: auto=%#v explicit=%#v", auto, explicit)
	}
	identity := func(args map[string]any) sideeffectidentityapp.IdentityV1 {
		t.Helper()
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := sideeffectidentityapp.ResolveV1(sideeffectidentityapp.Input{
			ToolName: "create_plan", Arguments: raw, WorkspaceRealPath: workspace,
			ResolvePlan: func(arguments map[string]any) (any, error) {
				prepared, output, failed := PrepareCreatePlanTool(CreatePlanToolInput{
					Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write", Args: arguments,
				})
				if failed {
					t.Fatalf("prepare identity create_plan: %#v", output)
				}
				return PreparedCreatePlanSemanticEffectProjection(prepared)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return resolved
	}
	if first, second := identity(map[string]any{
		"operation": "draft", "markdown": "same body", "title": "Stable Feature",
	}), identity(map[string]any{
		"operation": "draft", "markdown": "same body",
		"plan_relative_path": appplan.CurrentRelativeDir + "/stable-feature.md",
	}); first != second {
		t.Fatalf("durable create_plan identity admitted an auto/explicit alias: first=%#v second=%#v", first, second)
	}
}

func TestCreatePlanSemanticProjectionIgnoresNonPhysicalOperationAlias(t *testing.T) {
	workspace := t.TempDir()
	projection := func(operation string) map[string]any {
		t.Helper()
		prepared, output, failed := PrepareCreatePlanTool(CreatePlanToolInput{
			Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
			Args: map[string]any{
				"operation": operation, "markdown": "same body",
				"plan_relative_path": appplan.CurrentRelativeDir + "/same.md",
			},
		})
		if failed {
			t.Fatalf("prepare %s create_plan: %#v", operation, output)
		}
		result, err := PreparedCreatePlanSemanticEffectProjection(prepared)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if draft, refine := projection("draft"), projection("refine"); !reflect.DeepEqual(draft, refine) {
		t.Fatalf("response-only operation split one physical effect: draft=%#v refine=%#v", draft, refine)
	}
}

func TestCreatePlanPreoccupiedBaseCannotBypassAutoWithExplicitSelectedTarget(t *testing.T) {
	workspace := t.TempDir()
	base := filepath.Join(workspace, filepath.FromSlash(appplan.CurrentRelativeDir), "stable-feature.md")
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, []byte("pre-existing body"), 0o644); err != nil {
		t.Fatal(err)
	}
	prepare := func(args map[string]any) PreparedCreatePlanTool {
		t.Helper()
		prepared, output, failed := PrepareCreatePlanTool(CreatePlanToolInput{
			Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write", Args: args,
		})
		if failed {
			t.Fatalf("prepare create_plan: %#v", output)
		}
		return prepared
	}
	auto := prepare(map[string]any{
		"operation": "draft", "markdown": "same body", "title": "Stable Feature",
	})
	if auto.resolved.target.RelativePath != appplan.CurrentRelativeDir+"/stable-feature-2.md" {
		t.Fatalf("auto target did not avoid occupied base: %#v", auto.resolved.target)
	}
	autoProjection, err := PreparedCreatePlanSemanticEffectProjection(auto)
	if err != nil {
		t.Fatal(err)
	}
	if output, failed := ExecutePreparedCreatePlanTool(auto); failed {
		t.Fatalf("execute auto create_plan: %#v", output)
	}

	retry := prepare(map[string]any{
		"operation": "draft", "markdown": "same body", "title": "Stable Feature",
	})
	explicit := prepare(map[string]any{
		"operation": "draft", "markdown": "same body",
		"plan_relative_path": appplan.CurrentRelativeDir + "/stable-feature-2.md",
	})
	for name, prepared := range map[string]PreparedCreatePlanTool{"retry": retry, "explicit": explicit} {
		projection, projectionErr := PreparedCreatePlanSemanticEffectProjection(prepared)
		if projectionErr != nil {
			t.Fatal(projectionErr)
		}
		if !reflect.DeepEqual(autoProjection, projection) {
			t.Fatalf("%s changed the selected physical identity: auto=%#v other=%#v", name, autoProjection, projection)
		}
	}
	baseContent, err := os.ReadFile(base)
	if err != nil || string(baseContent) != "pre-existing body" {
		t.Fatalf("auto plan overwrote occupied base: content=%q err=%v", baseContent, err)
	}
}

func TestCreatePlanAutoSelectionDoesNotOverwriteFilesystemAliases(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(workspace, filepath.FromSlash(appplan.CurrentRelativeDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	caseSensitive, known := mutationPathCaseSensitive(workspace)
	if !known {
		t.Skip("host filesystem case semantics unavailable")
	}
	if !caseSensitive {
		upper := filepath.Join(dir, "CASE-ALIAS.MD")
		if err := os.WriteFile(upper, []byte("case owner"), 0o644); err != nil {
			t.Fatal(err)
		}
		output, failed := ExecuteCreatePlanTool(CreatePlanToolInput{
			Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
			Args: map[string]any{"operation": "draft", "markdown": "new body", "title": "case alias"},
		})
		if failed || output.(map[string]any)["relative_path"] != appplan.CurrentRelativeDir+"/case-alias-2.md" {
			t.Fatalf("case alias was not allocated around: output=%#v failed=%v", output, failed)
		}
		if data, readErr := os.ReadFile(upper); readErr != nil || string(data) != "case owner" {
			t.Fatalf("case alias owner was overwritten: data=%q err=%v", data, readErr)
		}
	}

	composedName := "caf\u00e9.md"
	decomposedName := norm.NFD.String(composedName)
	probe := filepath.Join(dir, composedName)
	if err := os.WriteFile(probe, []byte("probe"), 0o644); err != nil {
		t.Fatal(err)
	}
	composedInfo, composedErr := os.Lstat(probe)
	decomposedInfo, decomposedErr := os.Lstat(filepath.Join(dir, decomposedName))
	if composedErr != nil || decomposedErr != nil || !os.SameFile(composedInfo, decomposedInfo) {
		t.Skip("host filesystem does not alias NFC/NFD names")
	}
	if err := os.WriteFile(filepath.Join(dir, decomposedName), []byte("unicode owner"), 0o644); err != nil {
		t.Fatal(err)
	}
	output, failed := ExecuteCreatePlanTool(CreatePlanToolInput{
		Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
		Args: map[string]any{"operation": "draft", "markdown": "new unicode body", "title": "caf\u00e9"},
	})
	if failed || output.(map[string]any)["relative_path"] != appplan.CurrentRelativeDir+"/caf\u00e9-2.md" {
		t.Fatalf("Unicode alias was not allocated around: output=%#v failed=%v", output, failed)
	}
	if data, readErr := os.ReadFile(probe); readErr != nil || string(data) != "unicode owner" {
		t.Fatalf("Unicode alias owner was overwritten: data=%q err=%v", data, readErr)
	}
}

func TestCreatePlanSemanticProjectionMatchesHostPathAliasSemantics(t *testing.T) {
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(args map[string]any) map[string]any {
		t.Helper()
		args["operation"] = "draft"
		args["markdown"] = "body"
		prepared, output, failed := PrepareCreatePlanTool(CreatePlanToolInput{
			Workspace: workspace, Mode: "plan", SandboxMode: "workspace-write",
			Args: args,
		})
		if failed {
			t.Fatalf("prepare %#v: %#v", args, output)
		}
		projection, err := PreparedCreatePlanSemanticEffectProjection(prepared)
		if err != nil {
			t.Fatal(err)
		}
		return projection
	}
	caseSensitive, known := mutationPathCaseSensitive(workspace)
	if !known {
		t.Fatal("host filesystem case semantics are unavailable")
	}
	lower := resolve(map[string]any{"plan_relative_path": appplan.CurrentRelativeDir + "/case-alias.md"})
	upper := resolve(map[string]any{"plan_relative_path": appplan.CurrentRelativeDir + "/CASE-ALIAS.md"})
	if (lower["target"] == upper["target"]) == caseSensitive {
		t.Fatalf("plan identity diverged from host case semantics: sensitive=%v lower=%#v upper=%#v", caseSensitive, lower, upper)
	}
	composed := resolve(map[string]any{"title": "caf\u00e9"})
	decomposed := resolve(map[string]any{"title": norm.NFD.String("caf\u00e9")})
	composedPath := filepath.Join(workspace, filepath.FromSlash(appplan.CurrentRelativeDir), "caf\u00e9.md")
	if err := os.MkdirAll(filepath.Dir(composedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composedPath, []byte("probe"), 0o600); err != nil {
		t.Fatal(err)
	}
	composedInfo, composedErr := os.Lstat(composedPath)
	decomposedInfo, decomposedErr := os.Lstat(filepath.Join(filepath.Dir(composedPath), norm.NFD.String("caf\u00e9.md")))
	unicodeAliases := composedErr == nil && decomposedErr == nil && os.SameFile(composedInfo, decomposedInfo)
	if (composed["target"] == decomposed["target"]) != unicodeAliases {
		t.Fatalf("plan identity diverged from host Unicode semantics: aliases=%v composed=%#v decomposed=%#v", unicodeAliases, composed, decomposed)
	}
}

func TestExecuteCreatePlanToolRejectsInactiveOrReadOnly(t *testing.T) {
	workspace := t.TempDir()
	output, isError := ExecuteCreatePlanTool(CreatePlanToolInput{
		Workspace: workspace,
		Args:      map[string]any{"operation": "draft", "markdown": "body"},
	})
	if !isError {
		t.Fatalf("inactive create_plan should fail: %#v", output)
	}
	record, _ := output.(map[string]any)
	if record["error"] != "create_plan requires Plan mode or an active GUI plan context" {
		t.Fatalf("inactive output mismatch: %#v", output)
	}

	output, isError = ExecuteCreatePlanTool(CreatePlanToolInput{
		Workspace:   workspace,
		Mode:        "plan",
		SandboxMode: "read-only",
		Args:        map[string]any{"operation": "draft", "markdown": "body"},
	})
	if !isError {
		t.Fatalf("read-only create_plan should fail: %#v", output)
	}
	record, _ = output.(map[string]any)
	if record["code"] != "sandbox_read_only" {
		t.Fatalf("read-only output mismatch: %#v", output)
	}
}
