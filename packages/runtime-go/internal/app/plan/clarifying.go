package plan

import (
	"regexp"
	"strings"
)

var clarifyingCue = regexp.MustCompile(`(?i)(which|what kind|do you want|would you (like|prefer)|let me know which|prefer|哪|还是|你想要|请选择|选项)`)
var headingLine = regexp.MustCompile(`(?m)^#{1,6}\s`)

func IsClarifyingQuestion(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if headingLine.MatchString(trimmed) {
		return false
	}
	lines := strings.Split(trimmed, "\n")
	tailStart := len(lines) - 2
	if tailStart < 0 {
		tailStart = 0
	}
	tail := strings.Join(lines[tailStart:], "\n")
	if !strings.Contains(tail, "?") && !strings.Contains(tail, "？") {
		return false
	}
	return clarifyingCue.MatchString(trimmed)
}
