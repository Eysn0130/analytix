package thread

import (
	"strings"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

func ShouldAutoTitleThread(thread map[string]any) bool {
	title := strings.TrimSpace(stringField(thread, "title"))
	if autoTitle, ok := thread["autoTitle"].(bool); ok && autoTitle {
		return true
	}
	return title == "" || title == "New thread"
}

func TitleFromThread(thread map[string]any) string {
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		if title := TitleFromTurn(turn); title != "" {
			return title
		}
	}
	return ""
}

func TitleFromTurn(turn map[string]any) string {
	for _, rawItem := range listAny(turn["items"]) {
		item, _ := rawItem.(map[string]any)
		if item == nil || stringField(item, "kind") != "user_message" {
			continue
		}
		if text := strings.TrimSpace(stringField(item, "displayText")); text != "" {
			return ThreadTitleFromText(text)
		}
		if text := strings.TrimSpace(stringField(item, "text")); text != "" {
			return ThreadTitleFromText(text)
		}
	}
	return ThreadTitleFromText(stringField(turn, "prompt"))
}

func ThreadTitleFromText(text string) string {
	text = projectOrdinaryThreadText(text)
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	const maxTitleRunes = 64
	if len(runes) <= maxTitleRunes {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:maxTitleRunes-3])) + "..."
}

func projectOrdinaryThreadText(text string) string {
	return domainprivacy.ProjectText(text).Text
}
