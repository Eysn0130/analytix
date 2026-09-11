package privacyprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestProjectTextMasksRestrictedIdentifiersDeterministically(t *testing.T) {
	input := strings.Join([]string{
		"账号：6222020000000000000",
		"备用卡 6222-0200-0000-0000-000",
		"身份证号：11010519491231002X",
		"手机：13800138000",
		"邮箱：analyst@example.com",
		"MAC地址：AA:BB:CC:DD:EE:FF",
		"IP地址：10.20.30.40",
		"设备标识：550e8400-e29b-41d4-a716-446655440000",
		"住址：北京市朝阳区一号",
		"姓名：张三",
	}, "\n")

	projection := ProjectText(input)
	for _, raw := range []string{
		"6222020000000000000", "6222-0200-0000-0000-000", "11010519491231002X", "13800138000",
		"analyst@example.com", "AA:BB:CC:DD:EE:FF", "10.20.30.40", "550e8400-e29b-41d4-a716-446655440000",
		"北京市朝阳区一号", "张三",
	} {
		if strings.Contains(projection.Text, raw) {
			t.Fatalf("projection retained restricted value %q: %q", raw, projection.Text)
		}
	}
	for _, placeholder := range []string{"[ACCOUNT]", "[ID_NO]", "[PHONE]", "[EMAIL]", "[MAC]", "[IP]", "[DEVICE]", "[ADDRESS]", "[PERSON]"} {
		if !strings.Contains(projection.Text, placeholder) {
			t.Fatalf("projection omitted %s: %q", placeholder, projection.Text)
		}
	}
	if second := ProjectText(projection.Text); second.Text != projection.Text || len(second.Findings) != 0 {
		t.Fatalf("projection is not idempotent: first=%q second=%#v", projection.Text, second)
	}
}

func TestProjectTextHandlesUnicodeAndObfuscatedAccounts(t *testing.T) {
	tests := []string{
		"卡号 ６２２２０２０００００００００００００",
		"卡号 6222\u200b0200\u20600000\ufeff0000000",
		"卡号 6222\u20110200\u20120000\u22120000000",
	}
	for _, input := range tests {
		projection := ProjectText(input)
		if !strings.Contains(projection.Text, "[ACCOUNT]") || len(projection.Findings) == 0 {
			t.Fatalf("obfuscated account was not projected: input=%q projection=%#v", input, projection)
		}
	}
}

func TestProjectTextLabeledPIIPreservesOriginalUnicodeByteOffsets(t *testing.T) {
	for _, prefix := range []string{"\u212a", "\u0130"} {
		input := prefix + "ACCOUNT: opaque-value"
		projection := ProjectText(input)
		if projection.Text != prefix+"ACCOUNT: [ACCOUNT]" || len(projection.Findings) != 1 {
			t.Fatalf("unicode-prefixed label projection = %#v", projection)
		}
		valueStart := strings.Index(input, "opaque-value")
		finding := projection.Findings[0]
		if finding.Kind != KindAccount || finding.StartByte != valueStart || finding.EndByte != len(input) {
			t.Fatalf("finding did not retain original byte offsets: input=%q finding=%#v", input, finding)
		}
	}
}

func TestProjectTextMasksShortAccountsOnlyWithAccountSemantics(t *testing.T) {
	for _, input := range []string{
		"账号 00123456",
		"卡号\u200b００１２３４５６７",
		"付款账户为1234-5678",
		"account number 12345678901",
	} {
		projection := ProjectText(input)
		if !strings.Contains(projection.Text, "[ACCOUNT]") || len(projection.Findings) == 0 {
			t.Fatalf("context-bound short account was not projected: input=%q projection=%#v", input, projection)
		}
	}
	for _, input := range []string{
		"金额 12345678 元",
		"记录数 12345678",
		"日期 20260720",
		"普通编号 12345678",
	} {
		if projection := ProjectText(input); projection.Text != input || len(projection.Findings) != 0 {
			t.Fatalf("non-account short number was over-projected: input=%q projection=%#v", input, projection)
		}
	}
}

func TestProjectTextReachesFixedPointWhenEarlierMaskExposesShortAccountCue(t *testing.T) {
	input := "账户 6222020000000000000xxxxxxxxxx 12345678"
	projection := ProjectText(input)
	for _, raw := range []string{"6222020000000000000", "12345678"} {
		if strings.Contains(projection.Text, raw) {
			t.Fatalf("fixed-point projection retained %q: %#v", raw, projection)
		}
	}
	if strings.Count(projection.Text, "[ACCOUNT]") != 2 {
		t.Fatalf("fixed-point projection did not mask both accounts: %#v", projection)
	}
	if second := ProjectText(projection.Text); second.Text != projection.Text || len(second.Findings) != 0 {
		t.Fatalf("fixed-point projection is not idempotent: first=%#v second=%#v", projection, second)
	}
}

func TestProjectTextSeparatesAdjacentStructuredFieldsWithoutMaskingMeasures(t *testing.T) {
	input := "账号：1234567890：账号：1234567890：金额 1234567890：日期 20260716153045：时间戳 1784174496000"
	projection := ProjectText(input)
	want := "账号：[ACCOUNT]：账号：[ACCOUNT]：金额 1234567890：日期 20260716153045：时间戳 1784174496000"
	if projection.Text != want || len(projection.Findings) != 2 {
		t.Fatalf("structured field projection = %#v", projection)
	}
	for _, finding := range projection.Findings {
		if finding.Kind != KindAccount || finding.StartByte < 0 || finding.EndByte <= finding.StartByte || finding.EndByte > len(input) {
			t.Fatalf("structured field finding lost its original account span: %#v", projection.Findings)
		}
	}
	if err := ValidateOrdinaryText(projection.Text); err != nil {
		t.Fatalf("structured field projection retained restricted PII: %v", err)
	}
	if second := ProjectText(projection.Text); second.Text != projection.Text || len(second.Findings) != 0 {
		t.Fatalf("structured field projection is not idempotent: first=%#v second=%#v", projection, second)
	}
}

func TestProjectTextPreservesMeasuresAndCalendarValues(t *testing.T) {
	input := "金额 123456789012345678；格式化金额 12,345,678,901.23；日期 20260716153045；时间戳 1784174496000；记录数 2645472"
	if projection := ProjectText(input); projection.Text != input || len(projection.Findings) != 0 {
		t.Fatalf("ordinary measures were modified: %#v", projection)
	}
}

func TestProjectTextUsesTheClosestSemanticCueWithoutPoisoningLaterDates(t *testing.T) {
	input := "账户 ****5678；方向 out；时间 2026-01-01T00:00:00Z 至 2026-01-31T23:59:59Z"
	if projection := ProjectText(input); projection.Text != input || len(projection.Findings) != 0 {
		t.Fatalf("an earlier account cue poisoned a later timestamp: %#v", projection)
	}
}

func TestProjectPublicValueProjectsContentWithoutMutatingAuthority(t *testing.T) {
	input := map[string]any{
		"contextDigest": strings.Repeat("1", 64),
		"contextEpoch":  float64(7),
		"text":          "核查账号 6222020000000000000",
		"fileReferences": []any{map[string]any{
			"path": "/case/6222020000000000000.csv", "kind": "file",
		}},
		"accountId": "6222020000000000000",
		"output": map[string]any{
			"opaque": "对手账户：6222020000000000000",
		},
	}
	projectedValue, changed := ProjectPublicValue(input)
	projected, _ := projectedValue.(map[string]any)
	if !changed || projected["contextDigest"] != input["contextDigest"] || projected["contextEpoch"] != input["contextEpoch"] {
		t.Fatalf("public projection changed authority or missed content: %#v", projected)
	}
	if projected["accountId"] != "[ACCOUNT]" || !strings.Contains(projected["text"].(string), "[ACCOUNT]") {
		t.Fatalf("typed/text content was not projected: %#v", projected)
	}
	output, _ := projected["output"].(map[string]any)
	if output["opaque"] != "对手账户：[ACCOUNT]" {
		t.Fatalf("nested content projection = %#v", output)
	}
	if err := ValidatePublicValue(projected); err != nil {
		t.Fatalf("projected public value did not validate: %v", err)
	}
	second, secondChanged := ProjectPublicValue(projected)
	if secondChanged || !reflect.DeepEqual(second, projected) {
		t.Fatalf("public value projection is not idempotent: %#v", second)
	}
}

func TestProjectPublicValueProjectsStringMapsInsidePublicContent(t *testing.T) {
	input := map[string]any{
		// Question ids are host-issued correlation authority. A generic account
		// detector must not rewrite them even when their opaque digest happens to
		// contain a long decimal run.
		"id":       "input_123456789012_1",
		"question": "核对账号 6222020000000000000",
		"options": []map[string]string{{
			"label":       "账号 6222020000000000000",
			"description": "联系电话 13800138000",
		}},
	}
	projected, changed := ProjectPublicValue(map[string]any{
		"kind": "user_input", "inputId": "input_123456789012", "questions": []any{input},
	})
	if !changed {
		t.Fatal("nested string-map content should be projected")
	}
	questions := projected.(map[string]any)["questions"].([]any)
	question := questions[0].(map[string]any)
	if question["id"] != "input_123456789012_1" || question["question"] != "核对账号 [ACCOUNT]" {
		t.Fatalf("question authority or public text projection = %#v", question)
	}
	options := question["options"].([]map[string]string)
	if options[0]["label"] != "账号 [ACCOUNT]" || options[0]["description"] != "联系电话 [PHONE]" {
		t.Fatalf("nested option projection = %#v", options)
	}
}

func TestProjectPublicValueDoesNotPreserveUnboundUserInputQuestionID(t *testing.T) {
	projected, changed := ProjectPublicValue(map[string]any{
		"kind": "user_input", "inputId": "input_host",
		"questions": []any{map[string]any{"id": "6222020000000000000", "question": "Continue?"}},
	})
	if !changed {
		t.Fatal("unbound question id should be projected as untrusted content")
	}
	question := projected.(map[string]any)["questions"].([]any)[0].(map[string]any)
	if question["id"] != "[ACCOUNT]" {
		t.Fatalf("unbound question id escaped projection: %#v", question)
	}
}

func TestProjectPublicValueDoesNotTrustQuestionsOutsideHostUserInputRecord(t *testing.T) {
	projected, changed := ProjectPublicValue(map[string]any{
		"kind":      "provider_payload",
		"questions": []any{map[string]any{"id": "6222020000000000000", "opaque": "13800138000"}},
	})
	if !changed {
		t.Fatal("untrusted questions subtree should be projected")
	}
	question := projected.(map[string]any)["questions"].([]any)[0].(map[string]any)
	if question["id"] != "[ACCOUNT]" || question["opaque"] != "[PHONE]" {
		t.Fatalf("untrusted questions subtree escaped projection: %#v", question)
	}
}

func TestProjectPublicValuePreservesStructuredGoalAuthorityButProjectsContent(t *testing.T) {
	const account = "6222020000000000000"
	const hostCallID = "call_host_6f2945e6cef2919ef29377242527337517865eff01279cf16e81e06536809303"
	input := map[string]any{
		"id": "thr_1",
		"goal": map[string]any{
			"id": "goal_thr_1", "threadId": "thr_1", "status": "active",
			"objective": "核对账号 " + account,
			"evidenceLedger": []any{map[string]any{
				"id": "goal_ev_1", "step": "核对账号 " + account,
				"summary": "仅记录已核验证据", "toolCallId": hostCallID,
			}},
		},
	}
	projectedValue, changed := ProjectPublicValue(input)
	if !changed {
		t.Fatal("structured goal content should be privacy projected")
	}
	goal := projectedValue.(map[string]any)["goal"].(map[string]any)
	ledger := goal["evidenceLedger"].([]any)
	entry := ledger[0].(map[string]any)
	if goal["id"] != "goal_thr_1" || goal["threadId"] != "thr_1" || entry["toolCallId"] != hostCallID {
		t.Fatalf("structured goal authority changed: %#v", goal)
	}
	if strings.Contains(goal["objective"].(string), account) || strings.Contains(entry["step"].(string), account) {
		t.Fatalf("structured goal content retained restricted account: %#v", goal)
	}
	if err := ValidatePublicValue(projectedValue); err != nil {
		t.Fatalf("projected structured goal did not validate: %v", err)
	}
}

func TestProjectUntrustedValueDoesNotGrantGoalAuthorityByShape(t *testing.T) {
	const account = "6222020000000000000"
	projected, changed := ProjectUntrustedValue(map[string]any{
		"goal": map[string]any{
			"objective": "safe", "status": "active", "evidenceLedger": []any{},
			"toolCallId": account,
		},
	})
	if !changed {
		t.Fatal("untrusted goal-shaped value unexpectedly gained authority projection")
	}
	goal := projected.(map[string]any)["goal"].(map[string]any)
	if goal["toolCallId"] != "[ACCOUNT]" {
		t.Fatalf("untrusted goal-shaped authority escaped projection: %#v", goal)
	}
}

func TestProjectPublicValueFailsClosedForTypedComplexValues(t *testing.T) {
	type opaqueIdentity struct {
		Raw string
	}
	input := map[string]any{
		"account": map[string]any{"raw": "6222020000000000000"},
		"phone": []any{
			"13800138000",
			map[string]any{"raw": "13900139000"},
		},
		"identity": opaqueIdentity{Raw: "11010519491231002X"},
	}
	if err := ValidatePublicValue(input); err == nil {
		t.Fatal("typed complex PII value unexpectedly validated before projection")
	}
	projectedValue, changed := ProjectPublicValue(input)
	if !changed {
		t.Fatal("typed complex PII values were not withheld")
	}
	projected := projectedValue.(map[string]any)
	if projected["account"] != "[ACCOUNT]" || projected["identity"] != "[ID_NO]" {
		t.Fatalf("typed object or opaque value escaped fail-closed projection: %#v", projected)
	}
	phones := projected["phone"].([]any)
	if !reflect.DeepEqual(phones, []any{"[PHONE]", "[PHONE]"}) {
		t.Fatalf("typed array did not recursively fail closed: %#v", phones)
	}
	if err := ValidatePublicValue(projected); err != nil {
		t.Fatalf("projected typed complex values did not validate: %v", err)
	}
}

func TestProjectPublicValueStrictlyProjectsAndCanonicalizesRawMessages(t *testing.T) {
	input := map[string]any{
		"account": json.RawMessage(" { \"nested\" : \"6222020000000000000\" } \n"),
		"phone":   json.RawMessage("\"13800138000\" true"),
		"email":   json.RawMessage(strings.Repeat(" ", maxRawMessageBytes+1)),
		"output":  json.RawMessage(" { \"value\" : \"电话：13800138000\" } \n"),
	}
	if err := ValidatePublicValue(input); err == nil {
		t.Fatal("raw PII or non-canonical JSON unexpectedly validated")
	}
	projectedValue, changed := ProjectPublicValue(input)
	if !changed {
		t.Fatal("raw messages were not projected")
	}
	projected := projectedValue.(map[string]any)
	for key, want := range map[string]string{
		"account": `"[ACCOUNT]"`,
		"phone":   `"[PHONE]"`,
		"email":   `"[EMAIL]"`,
		"output":  `{"value":"电话：[PHONE]"}`,
	} {
		raw, ok := projected[key].(json.RawMessage)
		if !ok || string(raw) != want {
			t.Fatalf("raw projection %s = %#v, want canonical %s", key, projected[key], want)
		}
	}
	if err := ValidatePublicValue(projected); err != nil {
		t.Fatalf("projected raw messages did not validate: %v", err)
	}
	second, secondChanged := ProjectPublicValue(projected)
	if secondChanged || !reflect.DeepEqual(second, projected) {
		t.Fatalf("raw projection is not idempotent: %#v", second)
	}
}

func TestInvalidUTF8FailsClosed(t *testing.T) {
	input := string([]byte{'a', 0xff, 'b'})
	projection := ProjectText(input)
	if projection.Text != "[NUMBER]" || len(projection.Findings) != 1 || ValidateOrdinaryText(input) == nil {
		t.Fatalf("invalid UTF-8 did not fail closed: %#v", projection)
	}
}

func TestValidatePublicValueMatchesProjectionChangedSemantics(t *testing.T) {
	inputs := []any{
		nil,
		map[string]any{"kind": "heartbeat", "seq": float64(1), "status": "ok"},
		map[string]any{"text": "账号 6222020000000000000", "contextDigest": strings.Repeat("a", 64)},
		map[string]string{"label": "联系电话 13800138000", "id": "option_1"},
		[]any{map[string]any{"output": "邮箱 test@example.com"}},
		map[string]any{"account": map[string]any{"raw": "6222020000000000000"}},
		map[string]any{"output": json.RawMessage(` {"value":"safe"} `)},
		map[string]any{
			"kind": "user_input", "inputId": "input_123456789012",
			"questions": []any{map[string]any{
				"id": "input_123456789012_1", "question": "safe",
			}},
		},
	}
	for index, input := range inputs {
		_, changed := ProjectPublicValue(input)
		rejected := ValidatePublicValue(input) != nil
		if rejected != changed {
			t.Fatalf("input %d validation=%v projectionChanged=%v value=%#v", index, rejected, changed, input)
		}
	}
}
