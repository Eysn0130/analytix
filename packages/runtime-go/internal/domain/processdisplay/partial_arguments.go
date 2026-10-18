package processdisplay

import (
	"encoding/json"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/secretprojection"
)

// ToolPreparationSummaryV1 is a separate protected-local summary, never the
// argument envelope, provider continuation, execution input or evidence.
type ToolPreparationSummaryV1 struct {
	ToolName, Preview, Path, Classification string
	InputUtf16CodeUnits                     *int64
}

// PartialArgumentsV1 retains at most the existing process-local raw quota.
// Its reader is deliberately confined to tool summary fields and input size.
// Settlement replaces conflicting fragments; it does not start a tool.
type PartialArgumentsV1 struct {
	toolName string
	raw      strings.Builder
	sealed   bool
	withheld bool
}

func NewPartialArgumentsV1(toolName string) *PartialArgumentsV1 {
	return &PartialArgumentsV1{toolName: toolName, withheld: toolName == "" || len(toolName) > 256 || !utf8.ValidString(toolName)}
}

func (p *PartialArgumentsV1) RawBytesV1() int {
	if p == nil {
		return 0
	}
	return p.raw.Len()
}

func (p *PartialArgumentsV1) AppendV1(fragment string) bool {
	if p == nil || p.sealed || p.withheld {
		return false
	}
	if !utf8.ValidString(fragment) || len(fragment) > RawCaptureByteLimitV1-p.raw.Len() {
		p.raw.Reset()
		p.withheld = true
		return false
	}
	p.raw.WriteString(fragment)
	return true
}

func (p *PartialArgumentsV1) SettleV1(authoritative string) bool {
	if p == nil {
		return false
	}
	p.sealed = true
	if len(authoritative) > RawCaptureByteLimitV1 || !utf8.ValidString(authoritative) ||
		p.toolName == "" || len(p.toolName) > 256 || !utf8.ValidString(p.toolName) {
		p.raw.Reset()
		p.withheld = true
		return false
	}
	// Equal text retains the current source; a conflicting final replaces it.
	if p.raw.String() != authoritative {
		p.raw.Reset()
		p.raw.WriteString(authoritative)
	}
	p.withheld = false
	return true
}

type partialArgumentValueV1 struct {
	kind     byte
	text     string
	units    int64
	complete bool
	safeText bool
	fields   map[string]*partialArgumentValueV1
	values   []*partialArgumentValueV1
}

type partialArgumentScanV1 struct {
	raw     string
	at      int
	entries int
}

func (s *partialArgumentScanV1) space() {
	for s.at < len(s.raw) && strings.ContainsRune(" \n\r\t", rune(s.raw[s.at])) {
		s.at++
	}
}

func (s *partialArgumentScanV1) stringV1() (*partialArgumentValueV1, bool) {
	s.at++ // opening quote
	v := &partialArgumentValueV1{kind: 's', safeText: true}
	var text strings.Builder
	var high uint16
	finish := func(complete bool) (*partialArgumentValueV1, bool) {
		v.text, v.complete, v.safeText = text.String(), complete, v.safeText && high == 0
		return v, true
	}
	unit := func(u uint16) {
		v.units++
		if high != 0 {
			if u >= 0xdc00 && u <= 0xdfff {
				text.WriteRune(utf16.DecodeRune(rune(high), rune(u)))
				high = 0
				return
			}
			v.safeText, high = false, 0
		}
		if u >= 0xd800 && u <= 0xdbff {
			high = u
		} else if u >= 0xdc00 && u <= 0xdfff {
			v.safeText = false
		} else {
			text.WriteRune(rune(u))
		}
	}
	for s.at < len(s.raw) {
		c := s.raw[s.at]
		if c == '"' {
			s.at++
			return finish(true)
		}
		if c < 32 {
			return nil, false
		}
		if c == '\\' {
			if s.at+1 == len(s.raw) {
				s.at = len(s.raw)
				return finish(false)
			}
			escape := s.raw[s.at+1]
			if escape == 'u' {
				var decoded uint16
				for i := 0; i < 4; i++ {
					at := s.at + 2 + i
					if at == len(s.raw) {
						s.at = len(s.raw)
						return finish(false)
					}
					ch := s.raw[at]
					var digit byte
					switch {
					case ch >= '0' && ch <= '9':
						digit = ch - '0'
					case ch >= 'a' && ch <= 'f':
						digit = ch - 'a' + 10
					case ch >= 'A' && ch <= 'F':
						digit = ch - 'A' + 10
					default:
						return nil, false
					}
					decoded = decoded*16 + uint16(digit)
				}
				unit(decoded)
				s.at += 6
				continue
			}
			var decoded uint16
			switch escape {
			case '"', '\\', '/':
				decoded = uint16(escape)
			case 'b':
				decoded = '\b'
			case 'f':
				decoded = '\f'
			case 'n':
				decoded = '\n'
			case 'r':
				decoded = '\r'
			case 't':
				decoded = '\t'
			default:
				return nil, false
			}
			unit(decoded)
			s.at += 2
			continue
		}
		r, size := utf8.DecodeRuneInString(s.raw[s.at:])
		if r == utf8.RuneError && size == 1 {
			return nil, false
		}
		if high != 0 {
			v.safeText, high = false, 0
		}
		text.WriteRune(r)
		v.units++
		if r > 0xffff {
			v.units++
		}
		s.at += size
	}
	return finish(false)
}

func (s *partialArgumentScanV1) valueV1(depth int) (*partialArgumentValueV1, bool) {
	s.space()
	if depth > 32 || s.entries >= 128 || s.at == len(s.raw) {
		return nil, false
	}
	s.entries++
	c := s.raw[s.at]
	if c == '"' {
		return s.stringV1()
	}
	if c == '{' || c == '[' {
		s.at++
		v := &partialArgumentValueV1{kind: c, fields: map[string]*partialArgumentValueV1{}}
		close := byte('}')
		if c == '[' {
			close = ']'
		}
		first := true
		for {
			s.space()
			if s.at == len(s.raw) {
				return v, true
			}
			if s.raw[s.at] == close && first {
				s.at++
				v.complete = true
				return v, true
			}
			key := ""
			if c == '{' {
				if s.raw[s.at] != '"' {
					return nil, false
				}
				k, ok := s.stringV1()
				if !ok || !k.safeText {
					return nil, false
				}
				if !k.complete {
					return v, true
				}
				key = k.text
				if _, duplicate := v.fields[key]; duplicate {
					return nil, false
				}
				s.space()
				if s.at == len(s.raw) {
					return v, true
				}
				if s.raw[s.at] != ':' {
					return nil, false
				}
				s.at++
				s.space()
				if s.at == len(s.raw) {
					return v, true
				}
			}
			child, ok := s.valueV1(depth + 1)
			if !ok {
				return nil, false
			}
			if c == '{' {
				v.fields[key] = child
			} else {
				v.values = append(v.values, child)
			}
			if !child.complete {
				return v, true
			}
			s.space()
			if s.at == len(s.raw) {
				return v, true
			}
			if s.raw[s.at] == close {
				s.at++
				v.complete = true
				return v, true
			}
			if s.raw[s.at] != ',' {
				return nil, false
			}
			s.at++
			first = false
		}
	}
	start := s.at
	for s.at < len(s.raw) && !strings.ContainsRune(" \n\r\t,]}", rune(s.raw[s.at])) {
		s.at++
	}
	raw := s.raw[start:s.at]
	if json.Valid([]byte(raw)) {
		return &partialArgumentValueV1{kind: 'v', complete: true}, true
	}
	if s.at == len(s.raw) && raw != "" && (strings.HasPrefix("true", raw) || strings.HasPrefix("false", raw) || strings.HasPrefix("null", raw) || strings.Trim(raw, "0123456789.eE+-") == "") {
		return &partialArgumentValueV1{kind: 'v'}, true
	}
	return nil, false
}

func firstArgumentStringV1(root *partialArgumentValueV1, keys ...string) *partialArgumentValueV1 {
	for _, key := range keys {
		if v := root.fields[key]; v != nil && v.kind == 's' {
			return v
		}
	}
	return nil
}

func partialMutationCountV1(tool string, root *partialArgumentValueV1) *int64 {
	var total int64
	found := false
	add := func(v *partialArgumentValueV1) {
		if v != nil {
			total += v.units
			found = true
		}
	}
	switch tool {
	case "write", "write_file":
		add(firstArgumentStringV1(root, "content"))
	case "edit", "edit_file", "multi_edit":
		if edits := root.fields["edits"]; edits != nil {
			if edits.kind != '[' {
				return nil
			}
			for _, edit := range edits.values {
				if edit.kind != '{' {
					return nil
				}
				add(firstArgumentStringV1(edit, "oldText", "old_text", "old_string"))
				add(firstArgumentStringV1(edit, "newText", "new_text", "new_string"))
			}
		} else {
			add(firstArgumentStringV1(root, "oldText", "old_text", "old_string"))
			add(firstArgumentStringV1(root, "newText", "new_text", "new_string"))
		}
	default:
		return nil
	}
	if !found || total > MaxInputUtf16CodeUnitsV1 {
		return nil
	}
	return &total
}

func partialFileToolV1(tool string) bool {
	switch tool {
	case "read", "read_file", "read_image", "write", "write_file", "edit", "edit_file", "multi_edit":
		return true
	default:
		return false
	}
}

func (p *PartialArgumentsV1) ProjectV1(knownSecrets ...string) ToolPreparationSummaryV1 {
	withheld := ToolPreparationSummaryV1{Classification: "withheld"}
	if p == nil || p.withheld || len(knownSecrets) > 64 {
		return withheld
	}
	raw := p.raw.String()
	if strings.TrimSpace(raw) == "" && !p.sealed {
		name := ProjectCaptureV1(RawCaptureV1{Complete: true, Title: p.toolName}, knownSecrets...)
		if name.Classification == "withheld" || name.Title == "" {
			return withheld
		}
		return ToolPreparationSummaryV1{ToolName: name.Title, Classification: name.Classification}
	}
	s := partialArgumentScanV1{raw: raw}
	root, ok := s.valueV1(0)
	s.space()
	if !ok || root.kind != '{' || s.at != len(raw) || p.sealed && !root.complete {
		return withheld
	}
	// Discover cross-field credential assignments before selecting/bounding the
	// path or description. Original arguments remain unchanged and process-local.
	secrets := append([]string(nil), knownSecrets...)
	var gather func(*partialArgumentValueV1) bool
	gather = func(v *partialArgumentValueV1) bool {
		if v.kind == 's' && v.safeText {
			secrets = append(secrets, secretprojection.ExtractExplicitSecretsV1(v.text)...)
			if len(secrets) > 64 {
				return false
			}
		}
		for _, field := range v.fields {
			if !gather(field) {
				return false
			}
		}
		for _, value := range v.values {
			if !gather(value) {
				return false
			}
		}
		return true
	}
	if !gather(root) {
		return withheld
	}
	name := ProjectCaptureV1(RawCaptureV1{Complete: true, Title: p.toolName}, secrets...)
	if name.Classification == "withheld" || name.Title == "" {
		return withheld
	}
	out := ToolPreparationSummaryV1{ToolName: name.Title, Classification: name.Classification, InputUtf16CodeUnits: partialMutationCountV1(p.toolName, root)}
	merge := func(classification string) {
		if classification == "credential_redacted" {
			out.Classification = classification
		}
	}
	if partialFileToolV1(p.toolName) {
		path := firstArgumentStringV1(root, "path", "file_path", "filePath", "FilePath")
		if path != nil && path.complete && path.safeText {
			projected := ProjectCaptureV1(RawCaptureV1{Complete: true, Path: path.text}, secrets...)
			if projected.Classification != "withheld" {
				out.Path = strings.SplitN(projected.Path, "\n", 2)[0]
				merge(projected.Classification)
			}
		}
	} else if description := firstArgumentStringV1(root, "description"); description != nil && description.safeText {
		var projected DisplayCaptureV1
		if description.complete {
			projected = ProjectCaptureV1(RawCaptureV1{Complete: true, Body: description.text}, secrets...)
		} else {
			projected = ProjectAssistantDraftV1(description.text, secrets...)
		}
		if projected.Classification != "withheld" {
			out.Preview, _ = boundTextV1(strings.SplitN(projected.Body, "\n", 2)[0], 1024)
			merge(projected.Classification)
		}
	}
	return out
}
