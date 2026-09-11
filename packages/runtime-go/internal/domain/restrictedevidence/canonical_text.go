package restrictedevidence

import (
	"strconv"
	"strings"
)

const (
	maxCanonicalTextBytes  = 64 << 20
	canonicalTextWindow    = 16 * 1024
	canonicalTextWindowHop = canonicalTextWindow / 2
)

// ValidateCanonicalText inspects a rendered/extracted ordinary surface. It
// recognizes only structured field labels with canonical private-reference
// values inside a bounded local window; prose that merely names a contract or
// field remains allowed.
func ValidateCanonicalText(text string) error {
	if len(text) > maxCanonicalTextBytes {
		return ErrInspectionLimit
	}
	if err := Validate(text); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	for start := 0; start < len(text); start += canonicalTextWindowHop {
		end := start + canonicalTextWindow
		if end > len(text) {
			end = len(text)
		}
		if isCanonicalTextWindowRestricted(text[start:end]) {
			return ErrRestrictedEvidence
		}
		if end == len(text) {
			break
		}
	}
	return nil
}

func isCanonicalTextWindowRestricted(window string) bool {
	fields := canonicalTextFields(window)
	for _, purpose := range fields["purpose"] {
		if restrictedPurpose(purpose) {
			return true
		}
	}
	values := make(map[string][]any, len(fields))
	for key, entries := range fields {
		for _, entry := range entries {
			switch {
			case canonicalTextSHA256Key(key):
				if parsed, ok := canonicalTextValueAsSHA256(entry); ok {
					values[key] = append(values[key], parsed)
				}
			case canonicalTextPositiveIntegerKey(key):
				if parsed, ok := canonicalTextValueAsPositiveInteger(entry); ok {
					values[key] = append(values[key], parsed)
				}
			default:
				values[key] = append(values[key], entry)
			}
		}
	}
	return isObject(values)
}

func canonicalTextFields(window string) map[string][]string {
	fields := map[string][]string{}
	for cursor := 0; cursor < len(window); {
		lineEnd := strings.IndexByte(window[cursor:], '\n')
		if lineEnd < 0 {
			lineEnd = len(window) - cursor
		}
		line := window[cursor : cursor+lineEnd]
		canonicalTextLineFields(line, fields)
		cursor += lineEnd + 1
	}
	return fields
}

func canonicalTextLineFields(line string, fields map[string][]string) {
	for offset, current := range []byte(line) {
		if current != ':' && current != '=' {
			continue
		}
		labelStart := offset
		for labelStart > 0 && !canonicalTextLabelBoundary(line[labelStart-1]) {
			labelStart--
		}
		key, exact := canonicalTextLabelKey(line[labelStart:offset])
		if !exact {
			continue
		}
		if !canonicalTextRelevantKey(key) {
			continue
		}
		valueEnd := offset + 1
		for valueEnd < len(line) && !canonicalTextValueBoundary(line[valueEnd]) {
			valueEnd++
		}
		value := canonicalTextFieldValue(line[offset+1 : valueEnd])
		if value != "" {
			fields[key] = append(fields[key], value)
		}
	}
	if !strings.Contains(line, "|") {
		return
	}
	cells := strings.Split(line, "|")
	for index := 0; index+1 < len(cells); index++ {
		key, exact := canonicalTextLabelKey(cells[index])
		if !exact {
			continue
		}
		if !canonicalTextRelevantKey(key) {
			continue
		}
		value := canonicalTextFieldValue(cells[index+1])
		if value != "" {
			fields[key] = append(fields[key], value)
		}
	}
}

func canonicalTextLabelKey(value string) (string, bool) {
	label := strings.Trim(value, " \t\r\"'`*_")
	key := normalizeKey(label)
	if (key == "schemaversion" && label != "schemaVersion") || (key == "kind" && label != "kind") {
		return "", false
	}
	return key, true
}

func canonicalTextLabelBoundary(value byte) bool {
	switch value {
	case ',', ';', '{', '}', '[', ']', '|', '<', '>':
		return true
	default:
		return false
	}
}

func canonicalTextValueBoundary(value byte) bool {
	switch value {
	case ',', ';', '}', ']', '|', '<', '>':
		return true
	default:
		return false
	}
}

func canonicalTextFieldValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'`*()")
}

func canonicalTextRelevantKey(key string) bool {
	if key == "schemaversion" || key == "kind" || key == "purpose" || key == "sourceexactvalue" || key == "sourceexactvaluesha256" || key == "bindingdigest" ||
		key == "rawartifactsha256" || key == "sourcerecordsha256" || key == "sourcefileiddigest" ||
		key == "sourcerownumber" || key == "locatordigest" || key == "datasetsnapshotid" || key == "authoritysignature" ||
		key == "recorddigest" || key == "snapshotauthorityrecorddigest" || key == "ledgerrootdigest" ||
		key == "ledgerindexpagedigest" || key == "ledgerpagedigest" || key == "witnessdigest" {
		return true
	}
	for _, triple := range restrictedExactReferenceTriples {
		if key == triple.digest || key == triple.sha256 || key == triple.byteLength {
			return true
		}
	}
	return key == "manifestdigest" || key == "manifestsha256" || key == "manifestbytelength"
}

func canonicalTextSHA256Key(key string) bool {
	if key == "sourceexactvaluesha256" || key == "bindingdigest" || key == "rawartifactsha256" ||
		key == "sourcerecordsha256" || key == "sourcefileiddigest" || key == "locatordigest" ||
		key == "recorddigest" || key == "snapshotauthorityrecorddigest" || key == "ledgerrootdigest" ||
		key == "ledgerindexpagedigest" || key == "ledgerpagedigest" || key == "witnessdigest" ||
		key == "manifestdigest" || key == "manifestsha256" {
		return true
	}
	for _, triple := range restrictedExactReferenceTriples {
		if key == triple.digest || key == triple.sha256 {
			return true
		}
	}
	return false
}

func canonicalTextPositiveIntegerKey(key string) bool {
	if key == "schemaversion" || key == "sourcerownumber" || key == "manifestbytelength" {
		return true
	}
	for _, triple := range restrictedExactReferenceTriples {
		if key == triple.byteLength {
			return true
		}
	}
	return false
}

func canonicalTextValueAsSHA256(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "sha256:") {
		trimmed = strings.TrimSpace(trimmed[len("sha256:"):])
	}
	values := map[string][]any{"value": {trimmed}}
	return trimmed, hasSHA256(values, "value")
}

func canonicalTextValueAsPositiveInteger(value string) (uint64, bool) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return parsed, err == nil && positiveJSONSafeInteger(parsed)
}
