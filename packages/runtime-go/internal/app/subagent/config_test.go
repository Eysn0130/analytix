package subagent

import (
	"errors"
	"strings"
	"testing"
)

func TestMergeProfileSettingsParsesProfilesAndDiagnostics(t *testing.T) {
	document := map[string]any{
		"capabilities": map[string]any{
			"subagents": map[string]any{
				"enabled":             true,
				"default_tool_policy": "inherit",
				"defaultProfile":      "reviewer",
				"max_parallel":        float64(3),
				"maxChildRuns":        float64(9),
				"profiles": map[string]any{
					"reviewer": map[string]any{
						"provider_id":     "anthropic-main",
						"model":           "claude-sonnet",
						"endpoint_format": "messages",
						"modelVariant":    "fast",
						"reasoningEffort": "high",
						"preamble":        "Stay read-only.",
						"systemPrompt":    "Extra reviewer policy.",
						"tool_policy":     "readOnly",
						"tools":           []any{"read", "grep", "read"},
						"blockedTools":    []any{"bash", "write_file"},
						"blockedMcpServers": []any{
							"github",
						},
						"blockedSkills": []any{"imagegen"},
						"mode":          "all",
						"hidden":        true,
						"description":   "Deep review profile",
						"maxSteps":      float64(7),
						"tokenBudget":   float64(1234),
						"timeBudgetMs":  float64(5000),
						"color":         "blue",
						"icon":          "search",
					},
				},
			},
		},
	}
	raw, ok := ProfileConfigMap(document)
	if !ok {
		t.Fatal("expected capabilities.subagents config")
	}
	settings := MergeProfileSettings(DefaultProfileSettings(), raw, "config")
	if err := ValidateProfileSettings(settings); err != nil {
		t.Fatalf("settings should validate: %v", err)
	}
	if !settings.Enabled || settings.DefaultToolPolicy != "inherit" || settings.DefaultProfile != "reviewer" {
		t.Fatalf("settings scalar fields mismatch: %#v", settings)
	}
	if settings.MaxParallel != 3 || settings.MaxChildRuns != 9 {
		t.Fatalf("settings limits mismatch: %#v", settings)
	}
	profile := settings.Profiles["reviewer"]
	if profile.ProviderID != "anthropic-main" || profile.Model != "claude-sonnet" || profile.EndpointFormat != "messages" || profile.Variant != "fast" {
		t.Fatalf("profile execution fields mismatch: %#v", profile)
	}
	if profile.Effort != "high" || profile.ToolPolicy != "readOnly" || profile.Source != "config" {
		t.Fatalf("profile metadata mismatch: %#v", profile)
	}
	if len(profile.Tools) != 2 || profile.Tools[0] != "read" || profile.Tools[1] != "grep" {
		t.Fatalf("profile tools should be unique and ordered by input: %#v", profile.Tools)
	}
	if profile.Mode != "all" || !profile.Hidden || profile.Description != "Deep review profile" ||
		profile.SystemPrompt != "Extra reviewer policy." || profile.MaxSteps != 7 || !profile.MaxStepsSet ||
		profile.TokenBudget != 1234 || !profile.TokenBudgetSet || profile.TimeBudgetMS != 5000 || !profile.TimeBudgetMSSet ||
		profile.Color != "blue" || profile.Icon != "search" {
		t.Fatalf("profile extended fields mismatch: %#v", profile)
	}
	if len(profile.BlockedTools) != 2 || profile.BlockedTools[0] != "bash" || profile.BlockedTools[1] != "write_file" ||
		len(profile.BlockedMCPServers) != 1 || profile.BlockedMCPServers[0] != "github" ||
		len(profile.BlockedSkills) != 1 || profile.BlockedSkills[0] != "imagegen" {
		t.Fatalf("profile blocklists mismatch: %#v", profile)
	}
	diagnostics := ProfileDiagnostics(settings)
	if len(diagnostics) < 3 {
		t.Fatalf("default builtin profiles plus reviewer should be present: %#v", diagnostics)
	}
	description := ProfileDescription(settings)
	if strings.Contains(description, "Default profile: reviewer") || strings.Contains(description, "anthropic-main") || strings.Contains(description, "Deep review profile") ||
		!strings.Contains(description, "design-reviewer") {
		t.Fatalf("profile description mismatch: %q", description)
	}
}

func TestValidateProfileSettingsRejectsMissingDefaultProfile(t *testing.T) {
	err := ValidateProfileSettings(ProfileSettings{
		DefaultProfile: "missing",
		Profiles:       map[string]ProfileConfig{},
	})
	if err == nil || !strings.Contains(err.Error(), "defaultProfile") {
		t.Fatalf("missing default profile should fail, got %v", err)
	}
}

func TestLoadProfileSettingsRejectsInvalidProfileMode(t *testing.T) {
	_, err := LoadProfileSettings(map[string]any{
		"subagents": map[string]any{
			"profiles": map[string]any{
				"reviewer": map[string]any{"mode": "invalid"},
			},
		},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("invalid configured profile mode should fail, got %v", err)
	}
}

func TestLoadProfileSettingsRejectsInvalidReasoningEffort(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []any{" high ", "HIGH", "auto", sentinel, float64(1)} {
		_, err := LoadProfileSettings(map[string]any{
			"subagents": map[string]any{"profiles": map[string]any{"reviewer": map[string]any{"effort": effort}}},
		}, true)
		if !errors.Is(err, ErrReasoningEffortInvalid) || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("invalid profile effort was not rejected: effort=%#v err=%v", effort, err)
		}
	}
}

func TestLoadProfileSettingsUsesDefaultsWithoutRuntimeConfig(t *testing.T) {
	settings, err := LoadProfileSettings(nil, false)
	if err != nil {
		t.Fatalf("load profile settings: %v", err)
	}
	if !settings.Enabled || settings.DefaultToolPolicy != "readOnly" || settings.MaxParallel != 8 || settings.MaxChildRuns != 64 {
		t.Fatalf("default profile settings mismatch: %#v", settings)
	}
	if len(settings.Profiles) == 0 {
		t.Fatalf("builtin profiles should be present: %#v", settings)
	}
}

func TestLoadProfileSettingsValidatesConfiguredDefault(t *testing.T) {
	_, err := LoadProfileSettings(map[string]any{
		"subagents": map[string]any{
			"defaultProfile": "missing",
		},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "defaultProfile") {
		t.Fatalf("missing configured default should fail, got %v", err)
	}
}
