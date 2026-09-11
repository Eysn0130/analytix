package toolcatalog

import (
	"encoding/json"
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
)

const ReportDeliveryToolName = "stage_case_report"

// PromptExplicitlyRequestsReportDeliveryV1 is an advertisement guard, not
// case or publication authority. It requires both an affirmative delivery
// verb and a report-like artifact noun, and rejects common explicit negations.
// The frozen TurnSecurityContext, approved grant, and report owner still
// independently validate every effect.
func PromptExplicitlyRequestsReportDeliveryV1(prompt string) bool {
	body := strings.ToLower(strings.TrimSpace(prompt))
	if body == "" {
		return false
	}
	for _, denied := range []string{
		"不生成", "不要生成", "无需生成", "不出具", "不要出具", "无需出具",
		"不导出", "不要导出", "无需导出", "不制作", "不要制作", "无需制作",
		"不交付", "不要交付", "无需交付", "不需要报告", "无需报告", "不要报告",
		"do not generate", "don't generate", "do not create", "don't create",
		"do not export", "don't export", "without a report", "no report",
	} {
		if strings.Contains(body, denied) {
			return false
		}
	}
	actions := []string{
		"生成", "出具", "导出", "制作", "创建", "交付", "发布", "保存", "更新", "编制",
		"generate", "issue", "export", "create", "deliver", "publish", "save", "update", "prepare",
	}
	artifacts := []string{
		"报告", "简报", "附件", "附表", "材料", "主要资金流向表", "资金流向表",
		"report", "brief", "attachment", "appendix",
	}
	return containsAnyReportPromptCueV1(body, actions) &&
		containsAnyReportPromptCueV1(body, artifacts)
}

// CanonicalControlledAccessActionsForPendingToolCallV1 derives the controlled
// artifact actions requested by the user. It intentionally reads only the
// original pending prompt: model-provided tool arguments cannot expand a
// controlled-data release request.
//
// This is a deliberately narrow, fail-closed lexical guard. It recognizes a
// request only when a report delivery and a full account/card number are both
// explicit, with an explicit display or export action. Negated, redacted,
// referential, exemplary, conditional, meta, and otherwise ambiguous prompts
// do not authorize any controlled action.
func CanonicalControlledAccessActionsForPendingToolCallV1(pending appmodel.PendingToolCall) []string {
	body := strings.ToLower(strings.TrimSpace(pending.Prompt))
	if body == "" || controlledAccountPromptDisallowedV1(body) ||
		!PromptExplicitlyRequestsReportDeliveryV1(body) ||
		!promptExplicitlyRequestsFullAccountNumberV1(body) {
		return nil
	}

	actions := make([]string, 0, 2)
	if containsAnyReportPromptCueV1(body, []string{
		"展示", "显示", "呈现", "列示", "明示",
		"display", "show", "view",
	}) {
		actions = append(actions, domainpii.ControlledArtifactAccessActionDisplayV1)
	}
	if containsAnyReportPromptCueV1(body, []string{
		"导出", "下载", "打印", "另存", "保存为", "交付",
		"export", "download", "print", "save as", "deliver",
	}) {
		actions = append(actions, domainpii.ControlledArtifactAccessActionExportV1)
	}
	if len(actions) == 0 {
		return nil
	}
	return actions
}

func ReportDeliveryToolSchemaV1() domainmodel.ToolSchema {
	return domainmodel.ToolSchema{
		Name: ReportDeliveryToolName,
		Description: "Deliver the explicitly requested current-case report through the host Final Evidence Gate and immutable publication authority. " +
			"The call accepts no report content, facts, paths, receipt IDs, or publication options.",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Source:     "builtin",
	}
}

func containsAnyReportPromptCueV1(body string, cues []string) bool {
	for _, cue := range cues {
		if strings.Contains(body, cue) {
			return true
		}
	}
	return false
}

func promptExplicitlyRequestsFullAccountNumberV1(body string) bool {
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "的", "").Replace(body)
	if containsAnyReportPromptCueV1(compact, []string{
		"完整银行账号", "完整账户号", "完整银行账户号", "完整银行卡号", "完整卡号",
	}) {
		return true
	}
	return containsAnyReportPromptCueV1(body, []string{
		"full bank account", "complete bank account",
		"full bank account number", "complete bank account number",
		"full account number", "complete account number",
		"full card number", "complete card number",
		"full bank card number", "complete bank card number",
	})
}

func controlledAccountPromptDisallowedV1(body string) bool {
	return containsAnyReportPromptCueV1(body, []string{
		// Negation or a request to withhold the value.
		"不显示", "不展示", "不呈现", "不导出", "不交付", "不包含", "不披露",
		"不要", "不需要", "无需", "请勿", "禁止", "不得",
		"do not", "don't", "must not", "should not", "without", "never", "not ", "no ",

		// Redaction or another non-full representation.
		"脱敏", "掩码", "遮盖", "隐藏", "匿名", "去标识", "打码",
		"redact", "mask", "anonym", "de-identif", "deidentify", "obfuscat", "hide",

		// Quoted, referential, or illustrative language is not an access request.
		"引用", "引述", "转述", "示例", "例子", "例如", "样例", "模板", "演示", "\"", "“", "”", "‘", "’",
		"quote", "citation", "reference", "example", "sample", "template", "demo",

		// Conditions and prompt/meta requests require an explicit fresh request.
		"如果", "若", "假如", "条件", "仅当", "视情况", "取决于", "是否", "能否", "可否", "如何", "为什么", "什么情况下",
		"提示词", "元请求", "分类器", "分类", "检测", "测试", "判断",
		"if ", "when ", "unless", "whether", "can you", "could you", "would you", "how to", "what if",
		"prompt", "meta", "classifier", "classify", "detect", "test",
	})
}
