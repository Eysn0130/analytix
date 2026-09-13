package privacyprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

var ErrPrivateSourceProse = errors.New("ordinary prose contains a private source locator")

// ValidatePublicSourceProse is an output check, not historical admission.
// Existing sealed records retain their original PII/credential/authority gates
// and bytes. Current publication projects a detached copy before this check.
func ValidatePublicSourceProse(value any) error {
	if _, changed := projectPrivateSourceProse(value, false); changed {
		return ErrPrivateSourceProse
	}
	return nil
}

// Locators in prose are not execution authority. Keep this projection separate
// from PII detection: a local code path alone must not grant case authority.
// HTTP(S) locations and relative code paths retain their ordinary semantics.
const privateSourceStart = `(?:file://|/(?:Users|Volumes|private|var|tmp|home|cases?)/|[a-z]:[\\/]|~(?:[a-z0-9_.-]+)?[\\/]|\\\\|//)`

var privateSourceLocator = regexp.MustCompile("(?i)https?://[^\\s\"'`<>]+|(^|[\\s\"'`=(:：\\[{,，;；])" + privateSourceStart + "[^\\s\"'`<>{},;，。；！？]+")
var quotedPrivateSourceLocators = []*regexp.Regexp{
	regexp.MustCompile(`(?i)"` + privateSourceStart + `[^"\r\n]*"`),
	regexp.MustCompile(`(?i)'` + privateSourceStart + `[^'\r\n]*'`),
	regexp.MustCompile("(?i)`" + privateSourceStart + "[^`\\r\\n]*`"),
}

func ProjectPrivateSourceText(text string) string {
	if !strings.ContainsAny(text, `/\`) {
		return text
	}
	// A closed literal provides a trustworthy lexical end for filenames with
	// spaces or brackets. Do not expose the suffix of a partially masked path.
	for _, quoted := range quotedPrivateSourceLocators {
		text = quoted.ReplaceAllStringFunc(text, func(match string) string {
			return match[:1] + "[PRIVATE_PATH]" + match[len(match)-1:]
		})
	}
	if !strings.ContainsAny(text, `/\`) {
		return text
	}
	indices := privateSourceLocator.FindAllStringSubmatchIndex(text, -1)
	if len(indices) == 0 {
		return text
	}
	out := make([]byte, 0, len(text))
	cursor := 0
	for _, match := range indices {
		if match[2] < 0 { // A remote URL, not a local source locator.
			continue
		}
		out = append(out, text[cursor:match[3]]...)
		out = append(out, "[PRIVATE_PATH]"...)
		cursor = match[1]
	}
	if cursor == 0 {
		return text
	}
	out = append(out, text[cursor:]...)
	return string(out)
}

// Only explicitly prose-bearing fields gain locator projection. Typed paths
// (workspace, workspaceRoot, file references, attachment bindings) keep their
// existing owner validation and PII rules; they must not be rebound to a mask.
// Nested values inside prose are untrusted content, not same-name authority.
func projectPrivateSourceProse(value any, prose bool) (any, bool) {
	switch current := value.(type) {
	case json.RawMessage:
		if !prose {
			return value, false
		}
		if len(current) == 0 || len(current) > maxRawMessageBytes {
			return rawMessagePlaceholder(KindNumber), true
		}
		decoder := json.NewDecoder(bytes.NewReader(current))
		decoder.UseNumber()
		var decoded, trailing any
		if decoder.Decode(&decoded) != nil || decoder.Decode(&trailing) != io.EOF {
			return rawMessagePlaceholder(KindNumber), true
		}
		projected, changed := projectPrivateSourceProse(decoded, true)
		if !changed {
			return value, false
		}
		encoded, err := json.Marshal(projected)
		if err != nil || len(encoded) > maxRawMessageBytes {
			return rawMessagePlaceholder(KindNumber), true
		}
		return json.RawMessage(encoded), true
	case string:
		if !prose {
			return current, false
		}
		projected := ProjectPrivateSourceText(current)
		return projected, projected != current
	case map[string]any:
		var out map[string]any
		for key, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose || privateSourceProseKey(key))
			if changed {
				if out == nil {
					out = make(map[string]any, len(current))
					for k, v := range current {
						out[k] = v
					}
				}
				out[key] = projected
			}
		}
		if out != nil {
			return out, true
		}
	case map[string]string:
		var out map[string]string
		for key, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose || privateSourceProseKey(key))
			if changed {
				if out == nil {
					out = make(map[string]string, len(current))
					for k, v := range current {
						out[k] = v
					}
				}
				out[key] = projected.(string)
			}
		}
		if out != nil {
			return out, true
		}
	case []any:
		var out []any
		for i, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose)
			if changed {
				if out == nil {
					out = append([]any(nil), current...)
				}
				out[i] = projected
			}
		}
		if out != nil {
			return out, true
		}
	case []map[string]any:
		var out []map[string]any
		for i, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose)
			if changed {
				if out == nil {
					out = append([]map[string]any(nil), current...)
				}
				out[i] = projected.(map[string]any)
			}
		}
		if out != nil {
			return out, true
		}
	case []map[string]string:
		var out []map[string]string
		for i, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose)
			if changed {
				if out == nil {
					out = append([]map[string]string(nil), current...)
				}
				out[i] = projected.(map[string]string)
			}
		}
		if out != nil {
			return out, true
		}
	case []string:
		var out []string
		for i, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose)
			if changed {
				if out == nil {
					out = append([]string(nil), current...)
				}
				out[i] = projected.(string)
			}
		}
		if out != nil {
			return out, true
		}
	}
	return value, false
}

func privateSourceProseKey(key string) bool {
	switch normalizeFieldKey(key) {
	case "text", "displaytext", "prompt", "sourcerequest", "question", "summary", "message",
		"title", "autotitle", "description", "label", "header", "reviewtext", "quote", "reason",
		"error", "detail", "remark", "memo", "note", "instructions", "objective", "outputpreview",
		"suggestion", "摘要", "备注":
		return true
	default:
		return false
	}
}
