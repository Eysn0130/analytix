package privacyprojection

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

const maxRawMessageBytes = 1 << 20

// ProjectPublicValue projects only display/content-bearing fields. Authority
// identifiers, digests, epochs, numeric measures, and protocol state are left
// byte-stable so privacy projection cannot mutate security authority.
func ProjectPublicValue(value any) (any, bool) {
	prose, proseChanged := projectPrivateSourceProse(value, false, false)
	projected, changed := projectPublicValueWithHistoryV1(prose, false, "", goalHistoryRootContextV1(prose))
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
	if publicValueNeedsProjection(value, false, "", goalHistoryRootContextV1(value)) {
		return ErrRestrictedPII
	}
	return nil
}

// publicValueNeedsProjection mirrors projectPublicValue's classification but
// does not allocate a second object tree for the overwhelmingly common valid
// case. Publication validation needs only the changed bit; render/export paths
// continue to use ProjectPublicValue when they need the projected value.
func publicValueNeedsProjection(value any, textSubtree bool, typed Kind, history goalHistoryContextV1) bool {
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
			if !textSubtree && hostStructuredStateContainerV1(key, child, history) {
				childText = false
			}
			if publicValueNeedsProjection(child, childText, childTyped, history.child(key)) {
				return true
			}
		}
		return false
	case map[string]string:
		for key, child := range current {
			if textSubtree && numericMeasureKey(key) && isNumericMeasureValue(child) {
				continue
			}
			if publicValueNeedsProjection(child, textSubtree || publicTextKey(key), typedKindForKey(key), goalHistoryContextV1{}) {
				return true
			}
		}
		return false
	case []any:
		for _, child := range current {
			childHistory, childText := history.element(child, textSubtree)
			if publicValueNeedsProjection(child, childText, "", childHistory) {
				return true
			}
		}
		return false
	case []map[string]any:
		for _, child := range current {
			childHistory, childText := history.element(child, textSubtree)
			if publicValueNeedsProjection(child, childText, "", childHistory) {
				return true
			}
		}
		return false
	case []map[string]string:
		for _, child := range current {
			if publicValueNeedsProjection(child, textSubtree, "", goalHistoryContextV1{}) {
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
	return projectPublicValueWithHistoryV1(value, textSubtree, typed, goalHistoryContextV1{})
}

func projectPublicValueWithHistoryV1(value any, textSubtree bool, typed Kind, history goalHistoryContextV1) (any, bool) {
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
			if !textSubtree && hostStructuredStateContainerV1(key, child, history) {
				childText = false
			}
			projected, childChanged := projectPublicValueWithHistoryV1(child, childText, childTyped, history.child(key))
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
			childHistory, childText := history.element(child, textSubtree)
			projected, childChanged := projectPublicValueWithHistoryV1(child, childText, "", childHistory)
			out[index] = projected
			changed = changed || childChanged
		}
		return out, changed
	case []map[string]any:
		out := make([]map[string]any, len(current))
		changed := false
		for index, child := range current {
			childHistory, childText := history.element(child, textSubtree)
			projected, childChanged := projectPublicValueWithHistoryV1(child, childText, "", childHistory)
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

func hostStructuredStateContainerV1(key string, value any, history goalHistoryContextV1) bool {
	record, ok := value.(map[string]any)
	if !ok || record == nil {
		return false
	}
	switch normalizeFieldKey(key) {
	case "goal":
		_, hasLedger := record["evidenceLedger"]
		_, hasObjective := record["objective"]
		_, hasStatus := record["status"]
		return (hasLedger && hasObjective && hasStatus) ||
			(key == "goal" && history.field == "goal")
	case "todos":
		_, hasItems := record["items"]
		return hasItems
	default:
		return false
	}
}

// History context starts only at the public entry point, at a thread root or
// its closed metadata-sidecar envelope. Every other key or nested array clears
// it; a nested thread/entry lookalike cannot grant itself structural routing.
type goalHistoryContextV1 struct {
	threadID string
	field    string
}

func (c goalHistoryContextV1) child(key string) goalHistoryContextV1 {
	if c.threadID != "" && key == c.field {
		switch key {
		case "thread":
			return goalHistoryContextV1{c.threadID, "goalHistory"}
		case "goalHistory":
			return goalHistoryContextV1{c.threadID, "entries"}
		}
	}
	return goalHistoryContextV1{}
}

func (c goalHistoryContextV1) element(value any, prose bool) (goalHistoryContextV1, bool) {
	if c.field == "entries" {
		if entry, ok := value.(map[string]any); ok {
			snapshot, _ := entry["goal"].(map[string]any)
			if hostGoalHistorySnapshotV1(entry, snapshot, c.threadID) {
				return goalHistoryContextV1{c.threadID, "goal"}, prose
			}
		}
		return goalHistoryContextV1{}, true
	}
	return goalHistoryContextV1{}, prose
}

func goalHistoryRootContextV1(value any) goalHistoryContextV1 {
	record, ok := value.(map[string]any)
	if !ok {
		return goalHistoryContextV1{}
	}
	thread, field := record, "goalHistory"
	if record["kind"] == "thread_metadata" {
		version, valid := domaingoal.IntegerV1(record["version"], 1)
		summary, summaryOK := record["summary"].(map[string]any)
		thread, ok = record["thread"].(map[string]any)
		if len(record) != 5 || !valid || version != 1 || !goalHistoryTimestampV1(record["timestamp"]) || !ok || !summaryOK || len(summary) != 4 {
			return goalHistoryContextV1{}
		}
		schema, valid := domaingoal.IntegerV1(summary["schemaVersion"], 1)
		_, previewOK := summary["preview"].(string)
		_, messagesOK := domaingoal.IntegerV1(summary["messageCount"], 0)
		_, turnsOK := domaingoal.IntegerV1(summary["turnCount"], 0)
		if !valid || schema != 1 || !previewOK || !messagesOK || !turnsOK {
			return goalHistoryContextV1{}
		}
		field = "thread"
	}
	history, historyOK := thread["goalHistory"].([]any)
	if _, hasKind := thread["kind"]; hasKind || !goalHistoryRecordIDV1(thread["id"]) ||
		!historyOK || len(history) > 500 || !goalHistoryTimestampV1(thread["createdAt"]) || !goalHistoryTimestampV1(thread["updatedAt"]) {
		return goalHistoryContextV1{}
	}
	switch thread["status"] {
	case "idle", "running", "archived", "deleted":
		return goalHistoryContextV1{thread["id"].(string), field}
	default:
		return goalHistoryContextV1{}
	}
}

func goalHistoryTimestampV1(value any) bool {
	s, ok := value.(string)
	_, err := time.Parse(time.RFC3339Nano, s)
	return ok && err == nil
}

// This local UUID grammar classifies only Goal-history metadata. It cannot
// import contracts' equivalent check because contracts depends on privacy.
func goalHistoryUUIDV1(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '4' || !strings.ContainsRune("89ab", rune(value[19])) {
		return false
	}
	for index, character := range value {
		if index != 8 && index != 13 && index != 18 && index != 23 && !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func goalHistoryRecordIDV1(value any) bool {
	id, ok := value.(string)
	return ok && domainthread.IsCanonicalRecordID(id) && (goalHistoryUUIDV1(id) || !ContainsRestrictedPII(id))
}

// Match the existing public Goal integer measures without importing contracts
// or imposing the counted-round safe-integer ceiling on budgets and usage.
func goalHistoryMeasureV1(value any, minimum int) bool {
	switch n := value.(type) {
	case int:
		return n >= minimum
	case int64:
		return n >= int64(minimum) && n <= math.MaxInt
	case float64:
		return !math.Signbit(n) && n >= float64(minimum) && n < -float64(math.MinInt) && math.Trunc(n) == n
	case json.Number:
		number, err := n.Int64()
		return err == nil && number >= int64(minimum) && number <= math.MaxInt && !(number == 0 && strings.HasPrefix(n.String(), "-"))
	default:
		return false
	}
}

// AppendHistoryV1 deliberately omits the evidence ledger. Only its closed,
// thread-bound snapshot at the exact history position gains structural routing.
// Objective and blockedReason remain content and pass through the normal scan.
func hostGoalHistorySnapshotV1(entry, snapshot map[string]any, threadID string) bool {
	text := func(value any, maximum int) bool {
		s, ok := value.(string)
		return ok && s != "" && strings.TrimSpace(s) == s && (maximum == 0 || len([]rune(s)) <= maximum)
	}
	entryID, _ := entry["id"].(string)
	if threadID == "" || !goalHistoryUUIDV1(entryID) || !goalHistoryTimestampV1(entry["createdAt"]) {
		return false
	}
	if _, present := entry["goal"]; !present {
		return false
	}
	for key := range entry {
		switch key {
		case "id", "operation", "goal", "createdAt", "ref":
		default:
			return false
		}
	}
	if snapshot == nil {
		if entry["goal"] != nil || (entry["operation"] != "clear" && entry["operation"] != "legacy_clear") {
			return false
		}
		if raw, present := entry["ref"]; present {
			record, ok := raw.(map[string]any)
			if !ok || len(record) != 2 || !goalHistoryRecordIDV1(record["id"]) {
				return false
			}
			_, valid := domaingoal.RefFromRecordV1(record)
			return valid
		}
		return true
	}
	if snapshot["threadId"] != threadID || !text(snapshot["objective"], 4000) ||
		!goalHistoryTimestampV1(snapshot["createdAt"]) || !goalHistoryTimestampV1(snapshot["updatedAt"]) {
		return false
	}
	switch entry["operation"] {
	case "create", "edit", "pause", "resume", "legacy_update", "evidence", "model_update", "round_limit", "round_started":
	default:
		return false
	}
	switch snapshot["status"] {
	case "active", "paused", "blocked", "usageLimited", "budgetLimited", "complete":
	default:
		return false
	}
	for key, value := range snapshot {
		switch key {
		case "threadId", "objective", "status", "createdAt", "updatedAt":
		case "id":
			if !goalHistoryRecordIDV1(value) {
				return false
			}
		case "revision", "maxGoalRounds", "roundsStarted":
			// Validate the coupled counters together below.
		case "tokensUsed", "timeUsedSeconds", "tokenBudget":
			if key == "tokenBudget" && value == nil {
				continue
			}
			minimum := 0
			if key == "tokenBudget" {
				minimum = 1
			}
			if !goalHistoryMeasureV1(value, minimum) {
				return false
			}
		case "blockedReason":
			if _, ok := value.(string); !ok {
				return false
			}
		default:
			return false
		}
	}
	for _, key := range []string{"tokensUsed", "timeUsedSeconds"} {
		if _, present := snapshot[key]; !present {
			return false
		}
	}
	ref, _, _, counted := domaingoal.CountedRecordV1(snapshot)
	for _, key := range []string{"revision", "maxGoalRounds", "roundsStarted"} {
		if _, present := snapshot[key]; present && !counted {
			return false
		}
	}
	if raw, present := entry["ref"]; present {
		record, ok := raw.(map[string]any)
		if !ok || len(record) != 2 {
			return false
		}
		entryRef, valid := domaingoal.RefFromRecordV1(record)
		return counted && valid && entryRef == ref
	}
	return !counted
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
		if normalizeFieldKey(key) == "options" && record["id"] == expectedID {
			projected, childChanged := projectHostUserInputOptions(child, expectedID)
			out[key] = projected
			changed = changed || childChanged
			continue
		}
		projected, childChanged := projectPublicValue(child, true, typedKindForKey(key))
		out[key] = projected
		changed = changed || childChanged
	}
	return out, changed
}

// Option ids are structural only beneath the exact host question and ordinal.
// Labels, descriptions and unknown fields remain ordinary untrusted content.
func projectHostUserInputOptions(value any, questionID string) (any, bool) {
	project := func(record map[string]any, index int) (map[string]any, bool) {
		out := make(map[string]any, len(record))
		changed := false
		expected := questionID + "_option_" + strconv.Itoa(index+1)
		for key, child := range record {
			if normalizeFieldKey(key) == "id" && child == expected {
				out[key] = child
				continue
			}
			projected, childChanged := projectPublicValue(child, true, typedKindForKey(key))
			out[key] = projected
			changed = changed || childChanged
		}
		return out, changed
	}
	switch current := value.(type) {
	case []map[string]string:
		out := make([]map[string]string, len(current))
		changed := false
		for index, option := range current {
			record := make(map[string]any, len(option))
			for key, child := range option {
				record[key] = child
			}
			projected, childChanged := project(record, index)
			out[index] = make(map[string]string, len(projected))
			for key, child := range projected {
				out[index][key], _ = child.(string)
			}
			changed = changed || childChanged
		}
		return out, changed
	case []map[string]any:
		out := make([]map[string]any, len(current))
		changed := false
		for index, option := range current {
			projected, childChanged := project(option, index)
			out[index] = projected
			changed = changed || childChanged
		}
		return out, changed
	case []any:
		out := make([]any, len(current))
		changed := false
		for index, option := range current {
			if record, ok := option.(map[string]any); ok {
				projected, childChanged := project(record, index)
				out[index] = projected
				changed = changed || childChanged
			} else {
				projected, childChanged := projectPublicValue(option, true, "")
				out[index] = projected
				changed = changed || childChanged
			}
		}
		return out, changed
	default:
		return projectPublicValue(value, true, "")
	}
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
