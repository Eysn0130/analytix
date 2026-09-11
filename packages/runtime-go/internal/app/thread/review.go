package thread

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

func ReviewTitle(target map[string]any) string {
	switch contracts.StringField(target, "kind") {
	case "baseBranch":
		if branch := strings.TrimSpace(contracts.StringField(target, "branch")); branch != "" {
			return "Review against " + branch
		}
		return "Review base branch"
	case "commit":
		if sha := strings.TrimSpace(contracts.StringField(target, "sha")); sha != "" {
			return "Review commit " + sha
		}
		return "Review commit"
	case "custom":
		return "Review custom target"
	default:
		return "Review uncommitted changes"
	}
}

func ReviewPrompt(target map[string]any) string {
	switch contracts.StringField(target, "kind") {
	case "baseBranch":
		return "Review changes against base branch " + strings.TrimSpace(contracts.StringField(target, "branch")) + "."
	case "commit":
		return "Review commit " + strings.TrimSpace(contracts.StringField(target, "sha")) + "."
	case "custom":
		if instructions := strings.TrimSpace(contracts.StringField(target, "instructions")); instructions != "" {
			return instructions
		}
		return "Review the custom target."
	default:
		return "Review uncommitted changes."
	}
}
