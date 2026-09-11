package toolcatalog

import (
	"strings"
	"testing"

	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

func TestSkillCatalogResponsesAndLookup(t *testing.T) {
	catalog := SkillCatalog{
		Enabled: true,
		Roots:   []string{"/skills"},
		Skills: []map[string]any{
			{
				"id": "deep-review", "name": "Untrusted /private/name", "description": "secret diagnostic",
				"root": "/skills", "path": "/skills/deep", "entryPath": "/skills/deep/SKILL.md",
				"scope": "project", "legacy": true,
			},
		},
		ValidationErrors: []map[string]any{{"root": "/missing", "status": "missing"}},
		Reason:           "Skills are discovered from configured roots",
	}
	state := SkillCapabilityState(catalog)
	if state["status"] != "available" || state["discoveredSkills"] != float64(1) {
		t.Fatalf("skill capability mismatch: %#v", state)
	}
	if skill, ok := SkillByName(catalog, "$Deep Review"); !ok || skill["id"] != "deep-review" {
		t.Fatalf("skill lookup mismatch: skill=%#v ok=%v", skill, ok)
	}
	response := SkillResponse(catalog)
	if response["schemaVersion"] != float64(2) || response["enabled"] != true ||
		response["available"] != true || response["reasonCode"] != "available" ||
		response["configuredRootCount"] != float64(1) || response["skillCount"] != float64(1) ||
		response["validationErrorCount"] != float64(1) {
		t.Fatalf("skill response mismatch: %#v", response)
	}
	publicSkills, _ := response["skills"].([]any)
	publicSkill, _ := publicSkills[0].(map[string]any)
	if publicSkill["id"] != "deep-review" || publicSkill["name"] != "Deep Review" ||
		publicSkill["scope"] != "project" || publicSkill["legacy"] != true {
		t.Fatalf("public skill summary mismatch: %#v", publicSkill)
	}
	for _, privateField := range []string{"description", "root", "path", "entryPath", "entry", "model", "allowedTools"} {
		if _, exists := publicSkill[privateField]; exists {
			t.Fatalf("public skill summary exposed %s: %#v", privateField, publicSkill)
		}
	}
	for _, privateField := range []string{"roots", "validationErrors", "reason"} {
		if _, exists := response[privateField]; exists {
			t.Fatalf("public skill response exposed %s: %#v", privateField, response)
		}
	}
	diagnostics := SkillToolDiagnostics(catalog)
	if diagnostics["available"] != true {
		t.Fatalf("skill diagnostics mismatch: %#v", diagnostics)
	}
	ids := SkillIDs(catalog)
	if len(ids) != 1 || ids[0] != "deep-review" {
		t.Fatalf("skill ids mismatch: %#v", ids)
	}
}

func TestSkillResponseClearsDisabledAndDuplicateCatalogEntries(t *testing.T) {
	skill := map[string]any{
		"id": "review", "name": "untrusted", "scope": "global", "legacy": false,
	}
	disabled := SkillResponse(SkillCatalog{Enabled: false, Skills: []map[string]any{skill}})
	if disabled["available"] != false || disabled["reasonCode"] != "disabled_by_config" ||
		disabled["skillCount"] != float64(0) || len(disabled["skills"].([]any)) != 0 {
		t.Fatalf("disabled public skill catalog was not empty: %#v", disabled)
	}
	deduped := SkillResponse(SkillCatalog{Enabled: true, Roots: []string{"/skills"}, Skills: []map[string]any{skill, skill}})
	if deduped["skillCount"] != float64(1) || len(deduped["skills"].([]any)) != 1 {
		t.Fatalf("public skill catalog did not dedupe ids: %#v", deduped)
	}
}

func TestSkillToolListDedupeRunAsAndSlug(t *testing.T) {
	if NormalizeSkillRunAs("child-agent") != "subagent" || NormalizeSkillRunAs("default") != "inline" {
		t.Fatal("skill runAs normalization mismatch")
	}
	if NormalizeReasoningEffort("high") != "high" || NormalizeReasoningEffort("auto") != "" ||
		NormalizeReasoningEffort(" high ") != "" || NormalizeReasoningEffort("HIGH") != "" ||
		NormalizeReasoningEffort("extreme") != "" {
		t.Fatal("skill reasoning effort normalization mismatch")
	}
	tools := SkillToolList("read, grep; read web_fetch")
	if len(tools) != 3 || tools[0] != "grep" || tools[1] != "read" || tools[2] != "web_fetch" {
		t.Fatalf("skill tool list mismatch: %#v", tools)
	}
	skills := DedupeSkills([]map[string]any{
		{"id": "b", "name": "Bee"},
		{"id": "a", "name": "Aye"},
		{"id": "a", "name": "Duplicate"},
	})
	if len(skills) != 2 || skills[0]["id"] != "a" || skills[1]["id"] != "b" {
		t.Fatalf("dedupe skills mismatch: %#v", skills)
	}
	if SkillSlug("Hello, Skill!") != "hello-skill" || SkillDisplayName("deep_review") != "Deep Review" {
		t.Fatal("skill slug/display mismatch")
	}
}

func TestSkillSummaryRecordFromMarkdown(t *testing.T) {
	record, ok := SkillSummaryRecord(SkillSummarySource{
		Format:       "markdown",
		Root:         "/skills",
		Path:         "/skills/review",
		EntryPath:    "/skills/review/SKILL.md",
		Scope:        "project",
		DefaultID:    "review",
		Legacy:       true,
		MarkdownText: "---\nname: Deep Review\nrunAs: subagent\neffort: high\ntools: read, grep\n---\n# Ignore\n\nInspect changes carefully.",
	})
	if !ok {
		t.Fatal("expected markdown skill record")
	}
	if record["id"] != "deep-review" || record["name"] != "Deep Review" || record["runAs"] != "subagent" || record["effort"] != "high" {
		t.Fatalf("markdown skill record mismatch: %#v", record)
	}
	tools, _ := record["allowedTools"].([]string)
	if len(tools) != 2 || tools[0] != "grep" || tools[1] != "read" {
		t.Fatalf("markdown skill allowed tools mismatch: %#v", record["allowedTools"])
	}
	if record["description"] != "Inspect changes carefully." {
		t.Fatalf("markdown skill description mismatch: %#v", record)
	}
}

func TestSkillSummaryRecordSkipsNestedMarkdownWithoutDescription(t *testing.T) {
	if record, ok := SkillSummaryRecord(SkillSummarySource{
		Format:       "markdown",
		Root:         "/skills",
		Path:         "/skills/nested/hidden",
		EntryPath:    "/skills/nested/hidden/SKILL.md",
		Scope:        "project",
		DefaultID:    "hidden",
		Legacy:       true,
		Nested:       true,
		MarkdownText: "# Hidden\n\nOnly a heading.",
	}); ok || record != nil {
		t.Fatalf("nested skill without explicit description should be skipped: record=%#v ok=%v", record, ok)
	}
}

func TestSkillSummaryRecordFromManifestAndSubagentPrompt(t *testing.T) {
	record, ok := SkillSummaryRecord(SkillSummarySource{
		Format:    "manifest",
		Root:      "/skills",
		Path:      "/skills/summarize",
		EntryPath: "/skills/summarize/SKILL.md",
		Scope:     "global",
		DefaultID: "summarize",
		Manifest: map[string]any{
			"name":         "Summarize",
			"description":  "Summarize a document.",
			"context":      "continue",
			"allowedTools": []any{"read", "web_fetch"},
		},
	})
	if !ok {
		t.Fatal("expected manifest skill record")
	}
	if record["id"] != "summarize" || record["runAs"] != "subagent" || record["scope"] != "global" {
		t.Fatalf("manifest skill record mismatch: %#v", record)
	}
	prompt := SkillSubagentPrompt(record, "Summarize a document.", "Use it now")
	if !strings.Contains(prompt, "Use this Analytix skill playbook in an isolated child run.") ||
		!strings.Contains(prompt, "Summarize a document.") ||
		!strings.Contains(prompt, "Use it now") {
		t.Fatalf("subagent prompt missing header: %q", prompt)
	}
	if FirstMarkdownParagraph("# Title\n\nFirst paragraph\ncontinues\n\nSecond") != "First paragraph continues" {
		t.Fatal("first markdown paragraph mismatch")
	}
}

func TestSkillCatalogResolvesBodyOnlyFromPackageSnapshot(t *testing.T) {
	snapshot, err := domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{
		{RelativePath: "SKILL.md", Bytes: []byte("---\ndescription: Snapshot body\n---\nUse the frozen entry.")},
		{RelativePath: "references/a.md", Bytes: []byte("Frozen reference")},
		{RelativePath: "scripts/check.py", Bytes: []byte("print('safe')")},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{
		"id": "snapshot-skill", "name": "Snapshot Skill", "entry": "SKILL.md", "packageDigest": snapshot.Digest(),
	}
	catalog := SkillCatalog{Skills: []map[string]any{record}, snapshots: map[string]domainskill.PackageSnapshot{snapshot.Digest(): snapshot}}
	body, err := SkillEntryBody(catalog, record)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Use the frozen entry.", "## Reference: a", "Frozen reference", "## Scripts", "scripts/check.py", "Live package-path execution is disabled"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("snapshot body missing %q: %s", expected, body)
		}
	}
	unknown := map[string]any{"entry": "SKILL.md", "packageDigest": strings.Repeat("f", 64)}
	if _, err := SkillEntryBody(catalog, unknown); err == nil {
		t.Fatal("unknown package digest must fail closed")
	}
}
