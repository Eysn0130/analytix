package toolcatalog

import (
	"encoding/json"
	"reflect"
	"testing"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
)

func TestReportDeliveryToolIsLiveOnlyExplicitAndArgumentFree(t *testing.T) {
	for _, prompt := range []string{
		"请根据当前案件证据生成资金分析报告",
		"Deliver the current case report.",
	} {
		if !PromptExplicitlyRequestsReportDeliveryV1(prompt) {
			t.Fatalf("explicit report request was not recognized: %q", prompt)
		}
		tools := MaterializeToolSchemas(MaterializeInput{
			Prompt: prompt, PromptRoute: RouteToolAgent, ReportDelivery: true,
		})
		schema := schemaByNameOptional(tools, ReportDeliveryToolName)
		if schema == nil || schema.Source != "builtin" {
			t.Fatalf("live report tool was not materialized: %#v", tools)
		}
		var parameters map[string]any
		if err := json.Unmarshal(schema.Parameters, &parameters); err != nil ||
			parameters["type"] != "object" ||
			parameters["additionalProperties"] != false ||
			len(parameters["properties"].(map[string]any)) != 0 {
			t.Fatalf("report schema is not strict empty-object authority: %s err=%v", schema.Parameters, err)
		}
	}
	for _, prompt := range []string{
		"只核验当前案件，不生成任何报告",
		"Do not generate a report.",
		"当前案件有哪些资金流向？",
	} {
		if PromptExplicitlyRequestsReportDeliveryV1(prompt) {
			t.Fatalf("non-delivery prompt activated report publication: %q", prompt)
		}
		if schemaByNameOptional(MaterializeToolSchemas(MaterializeInput{
			Prompt: prompt, PromptRoute: RouteToolAgent, ReportDelivery: true,
		}), ReportDeliveryToolName) != nil {
			t.Fatalf("non-delivery prompt advertised the report tool: %q", prompt)
		}
	}
	if schemaByNameOptional(MaterializeToolSchemas(MaterializeInput{
		Prompt: "请生成案件报告", PromptRoute: RouteToolAgent,
	}), ReportDeliveryToolName) != nil {
		t.Fatal("unavailable report host advertised the report tool")
	}
	if schemaByNameOptional(MaterializeToolSchemas(MaterializeInput{
		Prompt: "请生成案件报告", PromptRoute: RouteToolAgent,
		ReportDelivery: true, Subagent: true, ToolScope: []string{ReportDeliveryToolName},
	}), ReportDeliveryToolName) != nil {
		t.Fatal("subagent received report publication authority")
	}
}

func TestReportDeliveryApprovalPolicyNeverExecutesImplicitly(t *testing.T) {
	for _, policy := range []string{"always", "on-request", "auto"} {
		if !RequiresApprovalWithHostPolicy(
			ReportDeliveryToolName, policy, "workspace-write", false,
		) {
			t.Fatalf("report delivery skipped explicit approval under %q", policy)
		}
	}
	if RequiresApprovalWithHostPolicy(
		ReportDeliveryToolName, "never", "workspace-write", false,
	) || !BlockedByApprovalNever(ReportDeliveryToolName, false, false) {
		t.Fatal("approval=never did not fail closed before report execution")
	}
}

func TestCanonicalControlledAccessActionsForPendingToolCallV1(t *testing.T) {
	for _, test := range []struct {
		name   string
		prompt string
		want   []string
	}{
		{
			name:   "Chinese display of full card number in report",
			prompt: "请生成当前案件资金分析报告，并在报告中展示完整银行卡号。",
			want:   []string{domainpii.ControlledArtifactAccessActionDisplayV1},
		},
		{
			name:   "Chinese export of full account number in report",
			prompt: "请生成当前案件报告，并导出包含完整账户号的附件。",
			want:   []string{domainpii.ControlledArtifactAccessActionExportV1},
		},
		{
			name:   "English display of full bank account number in report",
			prompt: "Generate the current case report and display the full bank account number.",
			want:   []string{domainpii.ControlledArtifactAccessActionDisplayV1},
		},
		{
			name:   "English export of complete card number in report",
			prompt: "Generate the current case report and export a report containing the complete card number.",
			want:   []string{domainpii.ControlledArtifactAccessActionExportV1},
		},
		{
			name:   "explicitly requests both canonical actions",
			prompt: "Generate the current case report, display the full account number, and export the report containing it.",
			want: []string{
				domainpii.ControlledArtifactAccessActionDisplayV1,
				domainpii.ControlledArtifactAccessActionExportV1,
			},
		},
		{
			name:   "Chinese negation",
			prompt: "请生成案件报告，但不要展示完整银行卡号。",
		},
		{
			name:   "English redaction",
			prompt: "Generate the current case report and export the redacted full bank account number.",
		},
		{
			name:   "Chinese quoted example",
			prompt: "请生成案件报告，并引用“展示完整银行卡号”的示例。",
		},
		{
			name:   "English conditional request",
			prompt: "If needed, generate the current case report and export the full account number.",
		},
		{
			name:   "ambiguous account request",
			prompt: "请生成案件报告，账号显示方式由你决定。",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending := appmodel.PendingToolCall{Prompt: test.prompt}
			if got := CanonicalControlledAccessActionsForPendingToolCallV1(pending); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("CanonicalControlledAccessActionsForPendingToolCallV1() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCanonicalControlledAccessActionsForPendingToolCallV1IgnoresModelToolArguments(t *testing.T) {
	pending := appmodel.PendingToolCall{
		Prompt: "请生成当前案件报告。",
		Call: domainmodel.ToolCall{Arguments: json.RawMessage(`{
			"action":"export",
			"field":"full bank account number"
		}`)},
	}
	if got := CanonicalControlledAccessActionsForPendingToolCallV1(pending); got != nil {
		t.Fatalf("model tool arguments expanded controlled access: %#v", got)
	}
}
