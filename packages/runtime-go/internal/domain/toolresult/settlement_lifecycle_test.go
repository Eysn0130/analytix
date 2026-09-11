package toolresult

import (
	"errors"
	"testing"
)

func TestSettlementLifecycleStatusV1ClosedMapping(t *testing.T) {
	caseCompleted := PublicToolResultProjectionV1{
		SchemaVersion: 1, ProjectionKind: ProjectionCaseSourceStatus, Disclosure: MetadataOnlyDisclosure,
		MessageKey: "case_source_private", Status: "completed", Code: "case_source_result_private", PrivatePayloadWithheld: true,
	}
	caseFailed := caseCompleted
	caseFailed.MessageKey = "case_source_failed"
	caseFailed.Status = "failed"
	tests := []struct {
		name       string
		projection PublicToolResultProjectionV1
		isError    bool
		want       string
	}{
		{name: "success", projection: WithheldProjectionV1("completed", "tool_output_private"), want: "completed"},
		{name: "failed", projection: WithheldProjectionV1("failed", "tool_output_private"), isError: true, want: "failed"},
		{name: "blocked", projection: WithheldProjectionV1("blocked", "tool_output_private"), isError: true, want: "failed"},
		{name: "cancelled", projection: WithheldProjectionV1("cancelled", "tool_output_private"), isError: true, want: "failed"},
		{name: "restart outcome unknown", projection: OutcomeUnknownAfterRestartProjectionV1(), isError: true, want: "failed"},
		{name: "case source private", projection: caseCompleted, want: "completed"},
		{name: "case source failed", projection: caseFailed, isError: true, want: "failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := SettlementLifecycleStatusV1(test.projection, test.isError)
			if err != nil || got != test.want {
				t.Fatalf("status=%q err=%v", got, err)
			}
		})
	}
}

func TestSettlementLifecycleStatusV1RejectsMismatchAndForgedUnknown(t *testing.T) {
	mismatchedHostMessage := WithheldProjectionV1("completed", "tool_output_private")
	mismatchedHostMessage.ProjectionKind = ProjectionHostStatus
	mismatchedHostMessage.MessageKey = "tool_failed"
	mismatchedHostMessage.Code = "tool_failed"
	mismatchedCase := PublicToolResultProjectionV1{
		SchemaVersion: 1, ProjectionKind: ProjectionCaseSourceStatus, Disclosure: MetadataOnlyDisclosure,
		MessageKey: "case_source_private", Status: "failed", Code: "case_source_result_private", PrivatePayloadWithheld: true,
	}
	for name, input := range map[string]struct {
		projection PublicToolResultProjectionV1
		isError    bool
	}{
		"error completed": {projection: WithheldProjectionV1("completed", "tool_output_private"), isError: true},
		"success failed":  {projection: WithheldProjectionV1("failed", "tool_output_private")},
		"success blocked": {projection: WithheldProjectionV1("blocked", "tool_output_private")},
		"success unknown": {projection: OutcomeUnknownAfterRestartProjectionV1()},
		"forged unknown":  {projection: LegacyWithheldProjectionV1(), isError: true},
		"mismatched host message": {
			projection: mismatchedHostMessage,
		},
		"mismatched case status": {projection: mismatchedCase, isError: true},
		"invalid": {
			projection: PublicToolResultProjectionV1{SchemaVersion: 1, ProjectionKind: ProjectionHostStatus, Status: "completed"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if status, err := SettlementLifecycleStatusV1(input.projection, input.isError); status != "" || !errors.Is(err, ErrToolResultLifecycleInvalidV1) {
				t.Fatalf("status=%q err=%v", status, err)
			}
		})
	}
}
