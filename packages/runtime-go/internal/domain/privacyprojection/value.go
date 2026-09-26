package privacyprojection

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode"
)

const maxRawMessageBytes = 1 << 20

// ProjectPublicValue projects only display/content-bearing fields. Authority
// identifiers, digests, epochs, numeric measures, and protocol state are left
// byte-stable so privacy projection cannot mutate security authority.
func ProjectPublicValue(value any) (any, bool) {
	prose, proseChanged := projectPrivateSourceProse(value, false, false)
	projected, changed := projectPublicValue(prose, false, "")
	return projected, changed || proseChanged
}

// ProjectUntrustedValue projects untrusted provider-bound strings, with typed
// PII keys taking precedence and the closed numeric-measure exception retained.
// Unlike ProjectPublicValue, it does not trust display/protocol field routing.
// Callers still own schema validation and disclosure authorization: a numeric
// value under a measure key is not by itself permission to disclose that value.
func ProjectUntrustedValue(value any) (any, bool) {
	prose, proseChanged := projectPrivateSourceProse(value, false, false)
	projected, changed := projectPublicValue(prose, true, "")
	return projected, changed || proseChanged
}

func ValidatePublicValue(value any) error {
	if publicValueNeedsProjection(value, false, "") {
		return ErrRestrictedPII
	}
	return nil
}

// publicValueNeedsProjection mirrors projectPublicValue's classification but
// does not allocate a second object tree for the overwhelmingly common valid
// case. Publication validation needs only the changed bit; render/export paths
// continue to use ProjectPublicValue when they need the projected value.
func publicValueNeedsProjection(value any, textSubtree bool, typed Kind) bool {
	if typed != "" {
		_, changed := projectTypedValue(value, typed)
		return changed
	}
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if inputID, ok := hostUserInputQuestionsAuthority(current, key); !textSubtree && ok {
				_, changed := projectHostUserInputQuestions(child, inputID)
				if changed {
					return true
				}
				continue
			}
			if textSubtree && numericMeasureKey(key) && isNumericMeasureValue(child) {
				continue
			}
			childTyped := typedKindForKey(key)
			childText := textSubtree || publicTextKey(key)
			if !textSubtree && hostStructuredStateContainerV1(key, child) {
				childText = false
			}
			if publicValueNeedsProjection(child, childText, childTyped) {
				return true
			}
		}
		return false
	case map[string]string:
		for key, child := range current {
			if textSubtree && numericMeasureKey(key) && isNumericMeasureValue(child) {
				continue
			}
			if publicValueNeedsProjection(child, textSubtree || publicTextKey(key), typedKindForKey(key)) {
				return true
			}
		}
		return false
	case []any:
		for _, child := range current {
			if publicValueNeedsProjection(child, textSubtree, "") {
				return true
			}
		}
		return false
	case []map[string]any:
		for _, child := range current {
			if publicValueNeedsProjection(child, textSubtree, "") {
				return true
			}
		}
		return false
	case []map[string]string:
		for _, child := range current {
			if publicValueNeedsProjection(child, textSubtree, "") {
				return true
			}
		}
		return false
	case []string:
		if !textSubtree {
			return false
		}
		for _, child := range current {
			if len(ProjectText(child).Findings) > 0 {
				return true
			}
		}
		return false
	case string:
		return textSubtree && len(ProjectText(current).Findings) > 0
	case json.RawMessage:
		if !textSubtree {
			return false
		}
		_, changed := projectRawMessage(current, true, "")
		return changed
	default:
		return false
	}
}

func projectPublicValue(value any, textSubtree bool, typed Kind) (any, bool) {
	if typed != "" {
		return projectTypedValue(value, typed)
	}
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		changed := false
		for key, child := range current {
			if inputID, ok := hostUserInputQuestionsAuthority(current, key); !textSubtree && ok {
				projected, childChanged := projectHostUserInputQuestions(child, inputID)
				out[key] = projected
				changed = changed || childChanged
				continue
			}
			if textSubtree && numericMeasureKey(key) && isNumericMeasureValue(child) {
				out[key] = child
				continue
			}
			childTyped := typedKindForKey(key)
			childText := textSubtree || publicTextKey(key)
			// Goal and todo values are host-normalized structured state, not a
			// prose subtree. Preserve their authority identifiers byte-for-byte
			// while continuing to project their named content fields. A malformed
			// or provider-originated lookalike remains on the conservative text
			// path because this exception is only available from a trusted parent.
			if !textSubtree && hostStructuredStateContainerV1(key, child) {
				childText = false
			}
			projected, childChanged := projectPublicValue(child, childText, childTyped)
			out[key] = projected
			changed = changed || childChanged
		}
		return out, changed
	case map[string]string:
		out := make(map[string]string, len(current))
		changed := false
		for key, child := range current {
			if textSubtree && numericMeasureKey(key) && isNumericMeasureValue(child) {
				out[key] = child
				continue
			}
			childTyped := typedKindForKey(key)
			childText := textSubtree || publicTextKey(key)
			projected, childChanged := projectPublicValue(child, childText, childTyped)
			out[key], _ = projected.(string)
			changed = changed || childChanged
		}
		return out, changed
	case []any:
		out := make([]any, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectPublicValue(child, textSubtree, "")
			out[index] = projected
			changed = changed || childChanged
		}
		return out, changed
	case []map[string]any:
		out := make([]map[string]any, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectPublicValue(child, textSubtree, "")
			out[index], _ = projected.(map[string]any)
			changed = changed || childChanged
		}
		return out, changed
	case []map[string]string:
		out := make([]map[string]string, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectPublicValue(child, textSubtree, "")
			out[index], _ = projected.(map[string]string)
			changed = changed || childChanged
		}
		return out, changed
	case []string:
		out := make([]string, len(current))
		changed := false
		for index, child := range current {
			if textSubtree {
				projection := ProjectText(child)
				out[index] = projection.Text
				changed = changed || len(projection.Findings) > 0
			} else {
				out[index] = child
			}
		}
		return out, changed
	case string:
		if !textSubtree {
			return current, false
		}
		projection := ProjectText(current)
		return projection.Text, len(projection.Findings) > 0
	case json.RawMessage:
		if textSubtree {
			return projectRawMessage(current, true, "")
		}
		return append(json.RawMessage(nil), current...), false
	default:
		return value, false
	}
}

func hostStructuredStateContainerV1(key string, value any) bool {
	record, ok := value.(map[string]any)
	if !ok || record == nil {
		return false
	}
	switch normalizeFieldKey(key) {
	case "goal":
		_, hasLedger := record["evidenceLedger"]
		_, hasObjective := record["objective"]
		_, hasStatus := record["status"]
		return hasLedger && hasObjective && hasStatus
	case "todos":
		_, hasItems := record["items"]
		return hasItems
	default:
		return false
	}
}

func projectTypedValue(value any, kind Kind) (any, bool) {
	if value == nil {
		return nil, false
	}
	switch current := value.(type) {
	case string:
		trimmed := strings.TrimSpace(current)
		if trimmed == "" || isSafePlaceholder(trimmed) {
			return current, false
		}
		return Placeholder(kind), true
	case []any:
		out := make([]any, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectTypedValue(child, kind)
			out[index] = projected
			changed = changed || childChanged
		}
		return out, changed
	case []string:
		out := make([]string, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectTypedValue(child, kind)
			out[index], _ = projected.(string)
			changed = changed || childChanged
		}
		return out, changed
	case json.Number:
		if strings.TrimSpace(current.String()) == "" {
			return current, false
		}
		return Placeholder(kind), true
	case json.RawMessage:
		return projectRawMessage(current, true, kind)
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return Placeholder(kind), true
	default:
		// A typed PII field whose value has an object, typed collection, boolean,
		// binary, pointer, or other opaque Go shape cannot safely retain any of
		// that shape in an ordinary projection. Withhold the whole field rather
		// than relying on a partial traversal that may overlook a value or key.
		return Placeholder(kind), true
	}
}

func projectRawMessage(current json.RawMessage, textSubtree bool, typed Kind) (any, bool) {
	fallbackKind := typed
	if fallbackKind == "" {
		fallbackKind = KindNumber
	}
	fallback := rawMessagePlaceholder(fallbackKind)
	if len(current) == 0 || len(current) > maxRawMessageBytes {
		return fallback, true
	}
	decoder := json.NewDecoder(bytes.NewReader(current))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return fallback, true
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fallback, true
	}
	projected, changed := projectPublicValue(decoded, textSubtree, typed)
	canonical, err := json.Marshal(projected)
	if err != nil || len(canonical) > maxRawMessageBytes {
		return fallback, true
	}
	return json.RawMessage(canonical), changed || !bytes.Equal(current, canonical)
}

func rawMessagePlaceholder(kind Kind) json.RawMessage {
	encoded, _ := json.Marshal(Placeholder(kind))
	return json.RawMessage(encoded)
}

func typedKindForKey(key string) Kind {
	normalized := normalizeFieldKey(key)
	switch normalized {
	case "account", "accountid", "accountno", "accountnumber", "acct", "acctno", "card", "cardid", "cardno", "cardnumber", "账号", "账户", "卡号", "银行卡号":
		return KindAccount
	case "identity", "identityid", "identityno", "identitynumber", "idno", "身份证", "身份证号", "证件号":
		return KindID
	case "phone", "phonenumber", "mobile", "mobilenumber", "telephone", "tel", "电话", "手机", "手机号":
		return KindPhone
	case "email", "emailaddress", "mail", "邮箱":
		return KindEmail
	case "mac", "macaddress":
		return KindMAC
	case "ip", "ipaddress":
		return KindIP
	case "deviceid", "deviceidentifier", "devicefingerprint", "imei", "设备标识", "设备指纹":
		return KindDevice
	case "address", "homeaddress", "postaladdress", "住址", "地址", "开户地址":
		return KindAddress
	case "person", "personname", "holdername", "accountname", "nameholder", "姓名", "户名", "持卡人", "开户人":
		return KindPerson
	default:
		return ""
	}
}

func publicTextKey(key string) bool {
	switch normalizeFieldKey(key) {
	case "text", "displaytext", "prompt", "question", "questions", "summary", "message", "title", "goal", "todos",
		"description", "label", "header", "reviewtext", "quote", "reason", "error", "detail", "details",
		"path", "relativepath", "filename", "name", "remark", "memo", "note", "content", "output", "result",
		"data", "value", "target", "arguments", "answer", "answers", "objective", "step", "blockedreason",
		"command", "instructions", "paths", "findings", "parentgoalobjective", "profiledescription", "diffsummary",
		"outputpreview", "recoveryreason", "deadletterreason", "latecompletionreason", "deliveryreason", "deliveryerror",
		"autocontinuereason", "autocontinueerror", "suggestion", "artifactpath", "worktreepath", "worktreebranch",
		"changedfiles", "usage", "modelexecution", "childmodelexecution", "childlabel", "childname", "workspace",
		"sourceref", "摘要", "备注":
		return true
	default:
		return false
	}
}

// hostUserInputQuestionsAuthority identifies the two closed records whose question
// ids are generated by the host and used as signed continuation correlation
// authority. Callers must also require a trusted parent (not a prose subtree).
// A provider/content record cannot grant this exception merely by copying kind
// and inputId. Other values named questions remain untrusted text subtrees.
func hostUserInputQuestionsAuthority(record map[string]any, key string) (string, bool) {
	if normalizeFieldKey(key) != "questions" {
		return "", false
	}
	kind, _ := record["kind"].(string)
	switch strings.TrimSpace(kind) {
	case "user_input", "user_input_requested":
		inputID, _ := record["inputId"].(string)
		inputID = strings.TrimSpace(inputID)
		return inputID, inputID != ""
	default:
		return "", false
	}
}

// projectHostUserInputQuestions preserves only the structural question id.
// Every other field is treated as untrusted display content, including
// unknown extension fields, so this exception cannot create a PII bypass.
func projectHostUserInputQuestions(value any, inputID string) (any, bool) {
	switch current := value.(type) {
	case []any:
		out := make([]any, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectHostUserInputQuestion(child, inputID+"_"+strconv.Itoa(index+1))
			out[index] = projected
			changed = changed || childChanged
		}
		return out, changed
	case []map[string]any:
		out := make([]map[string]any, len(current))
		changed := false
		for index, child := range current {
			projected, childChanged := projectHostUserInputQuestion(child, inputID+"_"+strconv.Itoa(index+1))
			out[index], _ = projected.(map[string]any)
			changed = changed || childChanged
		}
		return out, changed
	default:
		return projectPublicValue(value, true, "")
	}
}

func projectHostUserInputQuestion(value any, expectedID string) (any, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return projectPublicValue(value, true, "")
	}
	out := make(map[string]any, len(record))
	changed := false
	for key, child := range record {
		if normalizeFieldKey(key) == "id" {
			if id, ok := child.(string); ok && id == expectedID {
				out[key] = child
				continue
			}
		}
		projected, childChanged := projectPublicValue(child, true, typedKindForKey(key))
		out[key] = projected
		changed = changed || childChanged
	}
	return out, changed
}

func numericMeasureKey(key string) bool {
	switch normalizeFieldKey(key) {
	case "amount", "amountminor", "balance", "balanceminor", "count", "total", "sum", "rate", "ratio", "score", "duration", "durationms", "size", "bytes", "金额", "余额", "笔数", "数量", "合计", "比例":
		return true
	default:
		return false
	}
}

func isNumericMeasureValue(value any) bool {
	switch current := value.(type) {
	case json.Number:
		return validNumericMeasureText(current.String())
	case string:
		return validNumericMeasureText(current)
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func validNumericMeasureText(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	digits := 0
	decimal := false
	for index, character := range value {
		switch {
		case character >= '0' && character <= '9':
			digits++
		case (character == '-' || character == '+') && index == 0:
		case character == '.' && !decimal:
			decimal = true
		default:
			return false
		}
	}
	return digits > 0
}

func normalizeFieldKey(value string) string {
	var out strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			out.WriteRune(character)
		}
	}
	return out.String()
}
