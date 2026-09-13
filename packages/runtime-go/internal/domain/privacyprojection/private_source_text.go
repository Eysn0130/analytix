package privacyprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/restrictedevidence"
	"analytix.local/runtime-go/internal/domain/secretprojection"
)

var ErrPrivateSourceProse = errors.New("ordinary prose contains a private source locator")

// ProjectProviderSourceValue projects every string in an already-bounded JSON
// request value. Provider-bound tool arguments are output here, unlike typed
// paths used by the local GUI/execution authority or historical validators.
func ProjectProviderSourceValue(value any) (any, bool) {
	return projectPrivateSourceProse(value, true, true)
}

// ValidatePublicSourceProse is an output check, not historical admission.
// Existing sealed records retain their original PII/credential/authority gates
// and bytes. Current publication projects a detached copy before this check.
func ValidatePublicSourceProse(value any) error {
	if _, changed := projectPrivateSourceProse(value, false, false); changed {
		return ErrPrivateSourceProse
	}
	return nil
}

// Locators in prose are not execution authority. Keep this projection separate
// from PII detection: a local code path alone must not grant case authority.
// HTTP(S) locations and relative code paths retain their ordinary semantics.
const privateSourceStart = `(?:file://|/(?:Users|Volumes|private|var|tmp|home|cases?)/|[a-z]:[\\/]|~(?:[a-z0-9_.-]+)?[\\/]|\\\\|//)`

var privateSourceValueStart = regexp.MustCompile(`(?i)^` + privateSourceStart + `[^\s]`)
var privateSourceLocator = regexp.MustCompile("(?i)https?://[^\\s\"'`<>]+|(^|[\\s\"'`=(:：\\[{,，;；])" + privateSourceStart + "[^\\s\"'`<>{},;，。；！？]+")
var privateDiffHeader = regexp.MustCompile(`(?im)^((?:---|\+\+\+)[\t ]+)"?[ab]/` + privateSourceStart + `[^\r\n]*`)
var quotedPrivateSourceLocators = []*regexp.Regexp{
	regexp.MustCompile(`(?i)"` + privateSourceStart + `[^"\r\n]*"`),
	regexp.MustCompile(`(?i)'` + privateSourceStart + `[^'\r\n]*'`),
	regexp.MustCompile("(?i)`" + privateSourceStart + "[^`\\r\\n]*`"),
}

var composerMention = regexp.MustCompile(`([@$])\[((?:\\.|[^\]\\])*)\]\(((?:\\.|[^)\s\\])*)\)`)
var escapedMentionPart = regexp.MustCompile(`\\(.)`)
var mentionLabelEscaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)

// These are the closed private identity spellings, not an authority parser.
// The caseentity owner depends on privacyprojection, so this lexical sink
// check cannot call its higher-level authority validators.
var privateMentionIdentity = regexp.MustCompile(`cer1_[a-p]{64}|srow1_[0-9a-f]{64}`)
var mentionASCIIEscape = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

func containsPrivateMentionIdentity(id string) bool {
	decoded := mentionASCIIEscape.ReplaceAllStringFunc(id, func(value string) string {
		code, _ := strconv.ParseUint(value[2:], 16, 16)
		if code <= 0x7f {
			return string(rune(code))
		}
		return value
	})
	return privateMentionIdentity.MatchString(id) || privateMentionIdentity.MatchString(decoded)
}

func safeMentionID(encoded string) bool {
	id := encoded
	// Bound nested decoding without treating an encoded identifier as proof
	// that its contents are safe for ordinary output.
	for round := 0; round < 4; round++ {
		if strings.TrimSpace(id) == "" || projectPrivateSourceTextRaw(id) != id ||
			ContainsRestrictedPII(id) || secretprojection.ProjectTextV1(id) != id ||
			containsPrivateMentionIdentity(id) || restrictedevidence.Validate(id) != nil {
			return false
		}
		if !strings.Contains(id, "%") {
			return true
		}
		decoded, err := url.PathUnescape(id)
		if err != nil || !utf8.ValidString(decoded) {
			return false
		}
		id = decoded
	}
	return false
}

func ProjectPrivateSourceText(text string) string {
	var out strings.Builder
	cursor := 0
	for _, match := range composerMention.FindAllStringSubmatchIndex(text, -1) {
		marker := text[match[2]:match[3]]
		label := escapedMentionPart.ReplaceAllString(text[match[4]:match[5]], "$1")
		rawURI := text[match[6]:match[7]]
		uri := escapedMentionPart.ReplaceAllString(rawURI, "$1")
		prefix := "plugin://"
		if marker == "$" {
			prefix = "skill://"
		}
		if !strings.HasPrefix(uri, prefix) {
			continue
		}
		out.WriteString(projectPrivateSourceTextRaw(text[cursor:match[0]]))
		label = ProjectText(projectPrivateSourceTextRaw(secretprojection.ProjectTextV1(label))).Text
		if safeMentionID(strings.TrimPrefix(uri, prefix)) {
			out.WriteString(marker + "[" + mentionLabelEscaper.Replace(label) + "](" + rawURI + ")")
		} else {
			out.WriteString(label + " [PRIVATE_REFERENCE]")
		}
		cursor = match[1]
	}
	if cursor == 0 {
		return projectPrivateSourceTextRaw(text)
	}
	out.WriteString(projectPrivateSourceTextRaw(text[cursor:]))
	return out.String()
}

func projectPrivateSourceTextRaw(text string) string {
	if !strings.ContainsAny(text, `/\`) {
		return text
	}
	text = privateDiffHeader.ReplaceAllString(text, "${1}[PRIVATE_PATH]")
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
func projectPrivateSourceProse(value any, prose bool, providerJSON bool) (any, bool) {
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
		projected, changed := projectPrivateSourceProse(decoded, true, providerJSON)
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
		if providerJSON && !strings.ContainsAny(current, "\r\n") && privateSourceValueStart.MatchString(strings.TrimSpace(current)) {
			return "[PRIVATE_PATH]", true
		}
		projected := ProjectPrivateSourceText(current)
		return projected, projected != current
	case map[string]any:
		var out map[string]any
		for key, child := range current {
			projected, changed := projectPrivateSourceProse(child, prose || privateSourceProseKey(key), providerJSON)
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
			projected, changed := projectPrivateSourceProse(child, prose || privateSourceProseKey(key), providerJSON)
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
			projected, changed := projectPrivateSourceProse(child, prose, providerJSON)
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
			projected, changed := projectPrivateSourceProse(child, prose, providerJSON)
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
			projected, changed := projectPrivateSourceProse(child, prose, providerJSON)
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
			projected, changed := projectPrivateSourceProse(child, prose, providerJSON)
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
		"suggestion", "diff", "摘要", "备注":
		return true
	default:
		return false
	}
}
