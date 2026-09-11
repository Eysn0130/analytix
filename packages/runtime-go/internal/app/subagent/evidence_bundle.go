package subagent

import (
	"encoding/json"
	"strings"
)

func EvidenceBundleFromSummary(summary string) ([]any, bool) {
	for _, candidate := range evidenceBundleCandidates(summary) {
		var value any
		if err := json.Unmarshal([]byte(candidate), &value); err != nil {
			continue
		}
		switch typed := value.(type) {
		case []any:
			if len(typed) > 0 {
				return typed, true
			}
		case map[string]any:
			for _, key := range []string{"evidence", "evidenceBundle", "evidence_bundle"} {
				items, _ := typed[key].([]any)
				if len(items) > 0 {
					return items, true
				}
			}
		}
	}
	return nil, false
}

func evidenceBundleCandidates(summary string) []string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	candidates := []string{}
	if strings.HasPrefix(summary, "{") || strings.HasPrefix(summary, "[") {
		candidates = append(candidates, summary)
	}
	for _, fence := range []string{"```json", "```"} {
		search := summary
		for {
			start := strings.Index(search, fence)
			if start < 0 {
				break
			}
			body := search[start+len(fence):]
			end := strings.Index(body, "```")
			if end < 0 {
				break
			}
			candidate := strings.TrimSpace(body[:end])
			if candidate != "" {
				candidates = append(candidates, candidate)
			}
			search = body[end+len("```"):]
		}
	}
	return candidates
}
