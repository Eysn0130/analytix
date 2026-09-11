package subagent

import (
	"fmt"
	"sort"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func DefaultProfileSettings() ProfileSettings {
	return ProfileSettings{
		Enabled:           true,
		DefaultToolPolicy: "readOnly",
		MaxParallel:       8,
		MaxChildRuns:      64,
		Profiles: map[string]ProfileConfig{
			"design-reviewer": {
				Name:           "design-reviewer",
				PromptPreamble: "Review frontend code and prototypes for concrete visual, interaction, accessibility, hierarchy, spacing, motion, and AI-artifact issues. Read only; never edit files.",
				ToolPolicy:     "readOnly",
				Source:         "builtin",
			},
			"over-engineering-reviewer": {
				Name:           "over-engineering-reviewer",
				PromptPreamble: "Review only for avoidable complexity, speculative abstraction, needless dependencies, and code that can be safely simplified. Read only; never edit files.",
				ToolPolicy:     "readOnly",
				Source:         "builtin",
			},
		},
	}
}

func ProfileConfigMap(document map[string]any) (map[string]any, bool) {
	if direct, ok := document["subagents"].(map[string]any); ok {
		return direct, true
	}
	if capabilities, ok := document["capabilities"].(map[string]any); ok {
		if nested, ok := capabilities["subagents"].(map[string]any); ok {
			return nested, true
		}
	}
	return nil, false
}

func LoadProfileSettings(document map[string]any, ok bool) (ProfileSettings, error) {
	settings := DefaultProfileSettings()
	if ok {
		if raw, configOK := ProfileConfigMap(document); configOK {
			settings = MergeProfileSettings(settings, raw, "config")
		}
	}
	return settings, ValidateProfileSettings(settings)
}

func MergeProfileSettings(base ProfileSettings, raw map[string]any, source string) ProfileSettings {
	if base.Profiles == nil {
		base.Profiles = map[string]ProfileConfig{}
	}
	if policy := NormalizeToolPolicy(firstNonEmptyAnyString(raw["defaultToolPolicy"], raw["default_tool_policy"])); policy != "" {
		base.DefaultToolPolicy = policy
	}
	if value, ok := raw["enabled"].(bool); ok {
		base.Enabled = value
	}
	if profile := strings.TrimSpace(firstNonEmptyAnyString(raw["defaultProfile"], raw["default_profile"])); profile != "" {
		base.DefaultProfile = profile
	}
	if value, ok := numericAny(raw["maxParallel"]); ok {
		base.MaxParallel = value
	} else if value, ok := numericAny(raw["max_parallel"]); ok {
		base.MaxParallel = value
	}
	if value, ok := numericAny(raw["maxChildRuns"]); ok {
		base.MaxChildRuns = value
	} else if value, ok := numericAny(raw["max_child_runs"]); ok {
		base.MaxChildRuns = value
	}
	profiles, _ := raw["profiles"].(map[string]any)
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profileName := strings.TrimSpace(name)
		if profileName == "" {
			continue
		}
		rawProfile, _ := profiles[name].(map[string]any)
		if rawProfile == nil {
			continue
		}
		maxSteps, maxStepsSet := numericAny(firstNonNilValue(rawProfile["maxSteps"], rawProfile["max_steps"]))
		tokenBudget, tokenBudgetSet := numericAny(firstNonNilValue(rawProfile["tokenBudget"], rawProfile["token_budget"]))
		timeBudgetMS, timeBudgetMSSet := numericAny(firstNonNilValue(rawProfile["timeBudgetMs"], rawProfile["time_budget_ms"]))
		if !timeBudgetMSSet {
			if seconds, ok := numericAny(firstNonNilValue(rawProfile["timeBudgetSeconds"], rawProfile["time_budget_seconds"])); ok {
				timeBudgetMS = seconds * 1000
				timeBudgetMSSet = true
			}
		}
		rawMode := strings.TrimSpace(firstNonEmptyAnyString(rawProfile["mode"]))
		mode := NormalizeProfileMode(rawMode)
		if rawMode == "" {
			mode = "subagent"
		}
		if mode == "" {
			mode = rawMode
		}
		effort, effortErr := subagentReasoningEffortFromValues(rawProfile["effort"], rawProfile["reasoningEffort"], rawProfile["reasoning_effort"])
		profile := ProfileConfig{
			Name:              profileName,
			DisplayName:       strings.TrimSpace(firstNonEmptyAnyString(rawProfile["name"], rawProfile["displayName"], rawProfile["display_name"])),
			Mode:              mode,
			Hidden:            boolField(rawProfile, "hidden"),
			Description:       strings.TrimSpace(firstNonEmptyAnyString(rawProfile["description"])),
			ProviderID:        strings.TrimSpace(firstNonEmptyAnyString(rawProfile["providerId"], rawProfile["provider_id"])),
			Model:             strings.TrimSpace(firstNonEmptyAnyString(rawProfile["model"])),
			EndpointFormat:    domainmodel.OptionalEndpointFormat(firstNonEmptyAnyString(rawProfile["endpointFormat"], rawProfile["endpoint_format"])),
			Variant:           strings.TrimSpace(firstNonEmptyAnyString(rawProfile["variant"], rawProfile["modelVariant"], rawProfile["model_variant"])),
			Effort:            effort,
			EffortInvalid:     effortErr != nil,
			PromptPreamble:    strings.TrimSpace(firstNonEmptyAnyString(rawProfile["promptPreamble"], rawProfile["prompt_preamble"], rawProfile["preamble"])),
			SystemPrompt:      strings.TrimSpace(firstNonEmptyAnyString(rawProfile["systemPrompt"], rawProfile["system_prompt"])),
			ToolPolicy:        NormalizeToolPolicy(firstNonEmptyAnyString(rawProfile["toolPolicy"], rawProfile["tool_policy"])),
			Tools:             UniqueStringList(append(stringList(rawProfile["tools"]), stringList(rawProfile["allowedTools"])...)),
			BlockedTools:      UniqueStringList(append(stringList(rawProfile["blockedTools"]), stringList(rawProfile["blocked_tools"])...)),
			BlockedMCPServers: UniqueStringList(append(stringList(rawProfile["blockedMcpServers"]), stringList(rawProfile["blocked_mcp_servers"])...)),
			BlockedSkills:     UniqueStringList(append(stringList(rawProfile["blockedSkills"]), stringList(rawProfile["blocked_skills"])...)),
			MaxSteps:          maxSteps,
			MaxStepsSet:       maxStepsSet && maxSteps > 0,
			TokenBudget:       tokenBudget,
			TokenBudgetSet:    tokenBudgetSet && tokenBudget > 0,
			TimeBudgetMS:      timeBudgetMS,
			TimeBudgetMSSet:   timeBudgetMSSet && timeBudgetMS > 0,
			Color:             strings.TrimSpace(firstNonEmptyAnyString(rawProfile["color"])),
			Icon:              strings.TrimSpace(firstNonEmptyAnyString(rawProfile["icon"])),
			Source:            source,
		}
		if profile.ToolPolicy == "" {
			profile.ToolPolicy = "readOnly"
		}
		base.Profiles[profileName] = profile
	}
	return base
}

func ValidateProfileSettings(settings ProfileSettings) error {
	if strings.TrimSpace(settings.DefaultProfile) == "" {
		return validateProfileModes(settings)
	}
	if _, ok := settings.Profiles[settings.DefaultProfile]; !ok {
		return fmt.Errorf("defaultProfile %q is not defined in subagent profiles", settings.DefaultProfile)
	}
	return validateProfileModes(settings)
}

func validateProfileModes(settings ProfileSettings) error {
	for name, profile := range settings.Profiles {
		if profile.EffortInvalid || validateSubagentReasoningEffort(profile.Effort) != nil {
			return ErrReasoningEffortInvalid
		}
		mode := firstNonEmptyAnyString(profile.Mode, "subagent")
		if NormalizeProfileMode(mode) == "" {
			return fmt.Errorf("subagent profile %q has invalid mode %q", name, profile.Mode)
		}
	}
	return nil
}

func ProfileDiagnostics(settings ProfileSettings) []any {
	names := make([]string, 0, len(settings.Profiles))
	for name := range settings.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]any, 0, len(names))
	for _, name := range names {
		profile := settings.Profiles[name]
		row := map[string]any{
			"name":       name,
			"mode":       firstNonEmptyAnyString(profile.Mode, "subagent"),
			"hidden":     profile.Hidden,
			"toolPolicy": firstNonEmptyAnyString(profile.ToolPolicy, "readOnly"),
			"source":     firstNonEmptyAnyString(profile.Source, "config"),
		}
		if strings.TrimSpace(profile.DisplayName) != "" {
			row["displayName"] = profile.DisplayName
		}
		if strings.TrimSpace(profile.Description) != "" {
			row["description"] = profile.Description
		}
		if strings.TrimSpace(profile.Model) != "" {
			row["model"] = profile.Model
		}
		if strings.TrimSpace(profile.ProviderID) != "" {
			row["providerId"] = profile.ProviderID
		}
		if strings.TrimSpace(profile.EndpointFormat) != "" {
			row["endpointFormat"] = profile.EndpointFormat
		}
		if strings.TrimSpace(profile.Variant) != "" {
			row["variant"] = profile.Variant
		}
		if strings.TrimSpace(profile.Effort) != "" {
			row["effort"] = profile.Effort
		}
		if len(profile.Tools) > 0 {
			row["allowedTools"] = stringListAny(profile.Tools)
		}
		if len(profile.BlockedTools) > 0 {
			row["blockedTools"] = stringListAny(profile.BlockedTools)
		}
		if len(profile.BlockedMCPServers) > 0 {
			row["blockedMcpServers"] = stringListAny(profile.BlockedMCPServers)
		}
		if len(profile.BlockedSkills) > 0 {
			row["blockedSkills"] = stringListAny(profile.BlockedSkills)
		}
		if strings.TrimSpace(profile.PromptPreamble) != "" {
			row["promptPreamble"] = true
		}
		if strings.TrimSpace(profile.SystemPrompt) != "" {
			row["systemPrompt"] = true
		}
		if profile.MaxStepsSet {
			row["maxSteps"] = float64(profile.MaxSteps)
		}
		if profile.TokenBudgetSet {
			row["tokenBudget"] = float64(profile.TokenBudget)
		}
		if profile.TimeBudgetMSSet {
			row["timeBudgetMs"] = float64(profile.TimeBudgetMS)
		}
		if strings.TrimSpace(profile.Color) != "" {
			row["color"] = profile.Color
		}
		if strings.TrimSpace(profile.Icon) != "" {
			row["icon"] = profile.Icon
		}
		out = append(out, row)
	}
	return out
}

func ProfileDescription(settings ProfileSettings) string {
	names := make([]string, 0, len(settings.Profiles))
	for name := range settings.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := []string{}
	for _, name := range names {
		profile := settings.Profiles[name]
		if !ProfileVisibleForDelegation(profile) {
			continue
		}
		part := name + " (" + firstNonEmptyAnyString(profile.ToolPolicy, "readOnly")
		if strings.TrimSpace(profile.ProviderID) != "" {
			part += ", " + profile.ProviderID
		}
		if strings.TrimSpace(profile.Model) != "" {
			part += ", " + profile.Model
		}
		if strings.TrimSpace(profile.Variant) != "" {
			part += ", " + profile.Variant
		}
		if strings.TrimSpace(profile.EndpointFormat) != "" {
			part += ", " + profile.EndpointFormat
		}
		if strings.TrimSpace(profile.Effort) != "" {
			part += ", " + profile.Effort
		}
		part += ")"
		if description := strings.TrimSpace(profile.Description); description != "" {
			part += " - " + description
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return ""
	}
	description := " Available profiles: " + strings.Join(parts, "; ") + "."
	if defaultProfile := strings.TrimSpace(settings.DefaultProfile); defaultProfile != "" {
		profile, ok := settings.Profiles[defaultProfile]
		if ok && ProfileVisibleForDelegation(profile) {
			description += " Default profile: " + settings.DefaultProfile + "."
		}
	}
	return description
}

func ProfileVisibleForDelegation(profile ProfileConfig) bool {
	if profile.Hidden {
		return false
	}
	mode := NormalizeProfileMode(firstNonEmptyAnyString(profile.Mode, "subagent"))
	return mode == "subagent" || mode == "all"
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
