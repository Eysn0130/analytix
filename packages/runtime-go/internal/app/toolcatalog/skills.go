package toolcatalog

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

type SkillCatalog struct {
	Enabled          bool
	Roots            []string
	RootDiagnostics  []map[string]any
	Skills           []map[string]any
	ValidationErrors []map[string]any
	Reason           string
	snapshots        map[string]domainskill.PackageSnapshot
}

type LoadedSkillPackage struct {
	Record   map[string]any
	Snapshot domainskill.PackageSnapshot
}

type SkillSummarySource struct {
	Format        string
	Root          string
	Path          string
	EntryPath     string
	Entry         string
	PackageDigest string
	Scope         string
	DefaultID     string
	Legacy        bool
	Nested        bool
	MarkdownText  string
	Manifest      map[string]any
	Snapshot      domainskill.PackageSnapshot
}

type SkillRecordInput struct {
	ID            string
	Name          string
	Description   string
	Root          string
	Path          string
	EntryPath     string
	Entry         string
	PackageDigest string
	Scope         string
	Legacy        bool
	Metadata      map[string]any
}

func SkillCapabilityState(catalog SkillCatalog) map[string]any {
	available := catalog.Enabled && len(catalog.Skills) > 0
	status := "disabled"
	if catalog.Enabled {
		status = "unavailable"
	}
	if available {
		status = "available"
	}
	return map[string]any{
		"status":           status,
		"enabled":          catalog.Enabled,
		"available":        available,
		"reason":           catalog.Reason,
		"configuredRoots":  float64(len(catalog.Roots)),
		"discoveredSkills": float64(len(catalog.Skills)),
	}
}

func SkillToolDiagnostics(catalog SkillCatalog) map[string]any {
	state := SkillCapabilityState(catalog)
	return map[string]any{
		"enabled":          state["enabled"],
		"available":        state["available"],
		"reason":           state["reason"],
		"roots":            mapsListAny(catalog.RootDiagnostics),
		"skills":           mapsListAny(catalog.Skills),
		"validationErrors": mapsListAny(catalog.ValidationErrors),
		"lastActivations":  []any{},
	}
}

func SkillResponse(catalog SkillCatalog) map[string]any {
	publicSkills := make([]any, 0, len(catalog.Skills))
	seenSkillIDs := map[string]struct{}{}
	if catalog.Enabled {
		for _, skill := range catalog.Skills {
			if summary, ok := PublicSkillSummaryV2(skill); ok {
				id := contracts.StringField(summary, "id")
				if _, exists := seenSkillIDs[id]; exists {
					continue
				}
				seenSkillIDs[id] = struct{}{}
				publicSkills = append(publicSkills, summary)
			}
		}
	}
	available := catalog.Enabled && len(publicSkills) > 0
	reasonCode := "disabled_by_config"
	if catalog.Enabled {
		reasonCode = "unavailable"
	}
	if available {
		reasonCode = "available"
	}
	return map[string]any{
		"schemaVersion":        float64(2),
		"enabled":              catalog.Enabled,
		"available":            available,
		"reasonCode":           reasonCode,
		"configuredRootCount":  float64(len(catalog.Roots)),
		"skillCount":           float64(len(publicSkills)),
		"validationErrorCount": float64(len(catalog.ValidationErrors)),
		"skills":               publicSkills,
	}
}

var publicSkillIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$`)

func PublicSkillSummaryV2(skill map[string]any) (map[string]any, bool) {
	id := strings.TrimSpace(contracts.StringField(skill, "id"))
	scope := strings.TrimSpace(contracts.StringField(skill, "scope"))
	if !publicSkillIDPattern.MatchString(id) || (scope != "project" && scope != "global") {
		return nil, false
	}
	legacy, _ := skill["legacy"].(bool)
	public := map[string]any{
		"id":     id,
		"name":   SkillDisplayName(id),
		"scope":  scope,
		"legacy": legacy,
	}
	return public, true
}

func SkillByName(catalog SkillCatalog, name string) (map[string]any, bool) {
	normalized := SkillSlug(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(name), "$"), "@"))
	if strings.HasPrefix(normalized, "skill-") {
		normalized = strings.TrimPrefix(normalized, "skill-")
	}
	for _, skill := range catalog.Skills {
		id := SkillSlug(contracts.StringField(skill, "id"))
		displayName := SkillSlug(contracts.StringField(skill, "name"))
		if normalized != "" && (normalized == id || normalized == displayName) {
			return contracts.CloneMap(skill), true
		}
	}
	return nil, false
}

func SkillIDs(catalog SkillCatalog) []any {
	out := make([]any, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		if id := strings.TrimSpace(contracts.StringField(skill, "id")); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func SkillSummaryRecord(source SkillSummarySource) (map[string]any, bool) {
	switch strings.TrimSpace(source.Format) {
	case "manifest":
		manifest := contracts.CloneMap(source.Manifest)
		id := SkillSlug(firstNonEmptyAnyString(manifest["id"], manifest["name"], source.DefaultID))
		name := firstNonEmptyAnyString(manifest["name"], id)
		description := strings.TrimSpace(contracts.StringField(manifest, "description"))
		return SkillRecord(SkillRecordInput{
			ID:            id,
			Name:          name,
			Description:   description,
			Root:          source.Root,
			Path:          source.Path,
			EntryPath:     source.EntryPath,
			Entry:         source.Entry,
			PackageDigest: source.PackageDigest,
			Scope:         source.Scope,
			Legacy:        source.Legacy,
			Metadata:      manifest,
		}), true
	case "markdown":
		frontmatter, body := SkillFrontmatter(source.MarkdownText)
		description := firstNonEmptyAnyString(frontmatter["description"])
		if source.Nested && description == "" {
			return nil, false
		}
		id := SkillSlug(firstNonEmptyAnyString(frontmatter["id"], frontmatter["name"], source.DefaultID))
		name := firstNonEmptyAnyString(frontmatter["name"], SkillDisplayName(id))
		description = firstNonEmptyAnyString(description, FirstMarkdownParagraph(body))
		return SkillRecord(SkillRecordInput{
			ID:            id,
			Name:          name,
			Description:   description,
			Root:          source.Root,
			Path:          source.Path,
			EntryPath:     source.EntryPath,
			Entry:         source.Entry,
			PackageDigest: source.PackageDigest,
			Scope:         source.Scope,
			Legacy:        source.Legacy,
			Metadata:      frontmatter,
		}), true
	default:
		return nil, false
	}
}

func SkillRecord(input SkillRecordInput) map[string]any {
	record := map[string]any{
		"id":            input.ID,
		"name":          input.Name,
		"root":          input.Root,
		"path":          input.Path,
		"entryPath":     input.EntryPath,
		"entry":         input.Entry,
		"packageDigest": input.PackageDigest,
		"scope":         input.Scope,
		"legacy":        input.Legacy,
	}
	if input.Description != "" {
		record["description"] = input.Description
	}
	metadata := input.Metadata
	context := strings.ToLower(strings.TrimSpace(firstNonEmptyAnyString(metadata["context"])))
	if context != "" {
		record["context"] = context
	}
	runAs := NormalizeSkillRunAs(firstNonEmptyAnyString(metadata["runAs"], metadata["run_as"], metadata["run-as"]))
	if runAs == "" && (context == "fork" || context == "continue" || context == "subagent") {
		runAs = "subagent"
	}
	if runAs != "" {
		record["runAs"] = runAs
	}
	if model := strings.TrimSpace(firstNonEmptyAnyString(metadata["model"])); model != "" {
		record["model"] = model
	}
	if effort := NormalizeReasoningEffort(firstNonEmptyAnyString(metadata["effort"], metadata["reasoningEffort"], metadata["reasoning_effort"], metadata["reasoning-effort"])); effort != "" {
		record["effort"] = effort
	}
	if allowedTools := SkillToolList(firstNonEmptyAny(metadata["allowedTools"], metadata["allowed_tools"], metadata["allowed-tools"], metadata["tools"])); len(allowedTools) > 0 {
		record["allowedTools"] = allowedTools
	}
	return record
}

func SkillEntryBody(catalog SkillCatalog, skill map[string]any) (string, error) {
	digest := strings.TrimSpace(contracts.StringField(skill, "packageDigest"))
	entry := strings.TrimSpace(contracts.StringField(skill, "entry"))
	if digest == "" || entry == "" {
		return "", errors.New("skill package snapshot authority is missing")
	}
	snapshot, ok := catalog.snapshots[digest]
	if !ok || !snapshot.Valid() || snapshot.Digest() != digest || snapshot.EntryRelativePath() != entry {
		return "", errors.New("skill package snapshot authority is invalid or stale")
	}
	entryBytes, ok := snapshot.File(entry)
	if !ok {
		return "", errors.New("skill package snapshot entry is unavailable")
	}
	_, body := SkillFrontmatter(string(entryBytes))
	parts := []string{}
	if body = strings.TrimSpace(body); body != "" {
		parts = append(parts, body)
	}
	for _, relativePath := range snapshot.Paths() {
		if !strings.HasPrefix(relativePath, "references/") {
			continue
		}
		ext := strings.ToLower(path.Ext(relativePath))
		if ext != ".md" && ext != ".txt" {
			continue
		}
		bytes, exists := snapshot.File(relativePath)
		if !exists || strings.TrimSpace(string(bytes)) == "" {
			continue
		}
		label := strings.TrimSuffix(path.Base(relativePath), ext)
		parts = append(parts, "## Reference: "+label+"\n\n"+strings.TrimSpace(string(bytes)))
	}
	scripts := []string{}
	for _, relativePath := range snapshot.Paths() {
		if strings.HasPrefix(relativePath, "scripts/") {
			scripts = append(scripts, relativePath)
		}
	}
	if len(scripts) > 0 {
		lines := []string{
			"## Scripts",
			"",
			"Helper script contents are bound to this skill package snapshot. Live package-path execution is disabled unless a host executor verifies the package digest again.",
			"",
		}
		for _, script := range scripts {
			lines = append(lines, "- "+script)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(parts) == 0 {
		return "", errors.New("skill package snapshot entry has no instructions")
	}
	return strings.Join(parts, "\n\n"), nil
}

func (catalog SkillCatalog) PackageSnapshotCount() int {
	return len(catalog.snapshots)
}

func SkillFrontmatter(text string) (map[string]any, string) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return map[string]any{}, normalized
	}
	rest := strings.TrimPrefix(normalized, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return map[string]any{}, normalized
	}
	raw := rest[:end]
	body := rest[end+len("\n---\n"):]
	out := map[string]any{}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = skillFrontmatterString(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out, body
}

func FirstMarkdownParagraph(text string) string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, " ")
}

func SkillSubagentPrompt(skill map[string]any, body string, task string) string {
	body = strings.TrimSpace(body)
	parts := []string{
		"Use this Analytix skill playbook in an isolated child run.",
		"Skill: " + firstNonEmptyAnyString(skill["name"], skill["id"]),
	}
	if body != "" {
		parts = append(parts, "Skill instructions:\n"+body)
	}
	parts = append(parts, "Task:\n"+strings.TrimSpace(task))
	return strings.Join(parts, "\n\n")
}

func NormalizeSkillRunAs(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "subagent", "sub-agent", "child-agent", "child_agent":
		return "subagent"
	case "inline", "default":
		return "inline"
	default:
		return ""
	}
}

func NormalizeReasoningEffort(value string) string {
	projected, valid := domainmodel.ProjectReasoningEffortV1(value)
	if !valid {
		return ""
	}
	switch projected {
	case "off", "low", "medium", "high", "max":
		return projected
	default:
		return ""
	}
}

func SkillToolList(value any) []string {
	values := stringList(value)
	if len(values) == 0 {
		if text, ok := value.(string); ok {
			for _, part := range strings.FieldsFunc(text, func(r rune) bool {
				return r == ',' || r == ';' || r == ' '
			}) {
				part = strings.TrimSpace(part)
				if part != "" {
					values = append(values, part)
				}
			}
		}
	}
	return UniqueStringsSorted(values)
}

func UniqueStringsSorted(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func DedupeSkills(skills []map[string]any) []map[string]any {
	seen := map[string]bool{}
	out := []map[string]any{}
	for _, skill := range skills {
		id := strings.TrimSpace(contracts.StringField(skill, "id"))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, contracts.CloneMap(skill))
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(firstNonEmptyAnyString(out[i]["name"], out[i]["id"]))
		right := strings.ToLower(firstNonEmptyAnyString(out[j]["name"], out[j]["id"]))
		return left < right
	})
	return out
}

func SkillDisplayName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "Skill"
	}
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func SkillSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-._")
	if value == "" {
		return "skill"
	}
	return value
}

func mapsListAny(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, contracts.CloneMap(item))
	}
	return out
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func skillFrontmatterString(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed
			}
		case []string:
			if len(typed) > 0 {
				return typed
			}
		case []any:
			if len(typed) > 0 {
				return typed
			}
		case nil:
			continue
		default:
			return value
		}
	}
	return nil
}

func stringList(value any) []string {
	out := []string{}
	for _, raw := range listAny(value) {
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func listAny(value any) []any {
	switch items := value.(type) {
	case []any:
		return items
	case []string:
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item)
		}
		return out
	default:
		return []any{}
	}
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func containsString(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
