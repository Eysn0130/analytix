package privacyprojection

import (
	"reflect"
	"testing"
)

func TestProjectTurnContentMasksContentWithoutMutatingAuthorityInputs(t *testing.T) {
	refs := []any{map[string]any{"path": "/case/6222020000000000000.csv", "relativePath": "6222020000000000000.csv", "name": "6222020000000000000.csv", "kind": "file"}}
	plan := map[string]any{"label": "电话 13800138000", "opaque": "6222020000000000000"}
	prompt, display, projectedRefs, projectedPlan := ProjectTurnContent(
		"核实账号 6222020000000000000 的金额 2645472 元",
		"持卡人：张三，卡号：6222020000000000000", refs, plan,
	)
	if prompt != "核实账号 [ACCOUNT] 的金额 2645472 元" || display != "持卡人：[PERSON]，卡号：[ACCOUNT]" {
		t.Fatalf("turn text projection = %q / %q", prompt, display)
	}
	if reflect.DeepEqual(projectedRefs, refs) || projectedPlan["label"] != "电话 [PHONE]" || projectedPlan["opaque"] != "[ACCOUNT]" {
		t.Fatalf("structured turn projection = refs %#v plan %#v", projectedRefs, projectedPlan)
	}
	if plan["opaque"] != "6222020000000000000" {
		t.Fatalf("caller-owned GUI plan was mutated: %#v", plan)
	}
}

func TestProjectSteeringContentPreservesMeasuredAmount(t *testing.T) {
	text, display, refs := ProjectSteeringContent("账号：6222020000000000000，金额：2645472 元", "", nil)
	if text != "账号：[ACCOUNT]，金额：2645472 元" || display != "" || refs != nil {
		t.Fatalf("steering projection = %q / %q / %#v", text, display, refs)
	}
}

func TestProjectTurnContentMasksContextBoundShortAccount(t *testing.T) {
	prompt, display, _, _ := ProjectTurnContent(
		"核实账号 00123456 的金额 12345678 元",
		"账户为００１２３４５６７", nil, nil,
	)
	if prompt != "核实账号 [ACCOUNT] 的金额 12345678 元" || display != "账户为[ACCOUNT]" {
		t.Fatalf("short account projection mismatch: prompt=%q display=%q", prompt, display)
	}
}

func TestProjectAttachmentMetadataReturnsDetachedMaskedCopy(t *testing.T) {
	values := []map[string]any{{"id": "att_authority", "name": "账号 6222020000000000000.txt", "mimeType": "text/plain"}}
	projected := ProjectAttachmentMetadata(values)
	if projected[0]["id"] != "att_authority" || projected[0]["name"] != "账号 [ACCOUNT].txt" {
		t.Fatalf("attachment metadata projection = %#v", projected)
	}
	if values[0]["name"] != "账号 6222020000000000000.txt" {
		t.Fatalf("caller-owned metadata was mutated: %#v", values)
	}
}

func TestProjectStringRecordsMasksEveryUntrustedAnswerField(t *testing.T) {
	answers := []map[string]string{{"id": "q_6222020000000000000", "label": "卡号 6222020000000000000", "value": "6222020000000000000"}}
	projected := ProjectStringRecords(answers)
	if projected[0]["id"] != "q_[ACCOUNT]" || projected[0]["label"] != "卡号 [ACCOUNT]" || projected[0]["value"] != "[ACCOUNT]" {
		t.Fatalf("answer projection = %#v", projected)
	}
	if answers[0]["value"] != "6222020000000000000" {
		t.Fatalf("caller-owned answers were mutated: %#v", answers)
	}
}
