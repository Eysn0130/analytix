package privacyprojection

import (
	"strings"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

// ProjectOrdinaryList projects a detached list of ordinary public records.
func ProjectOrdinaryList(values []any) []any {
	return projectPublicList(values)
}

// ProjectTurnContent projects the complete ordinary content slice of a start
// request after the host has classified its original risk. It deliberately
// has no dependency on transport request types.
func ProjectTurnContent(prompt, displayText string, fileReferences []any, guiPlan map[string]any) (string, string, []any, map[string]any) {
	prompt = ProjectOrdinaryText(prompt)
	displayText = ProjectOrdinaryText(displayText)
	fileReferences = projectPublicList(fileReferences)
	if guiPlan != nil {
		projected, _ := domainprivacy.ProjectUntrustedValue(guiPlan)
		guiPlan, _ = projected.(map[string]any)
	}
	return prompt, displayText, fileReferences, guiPlan
}

func ProjectSteeringContent(text, displayText string, fileReferences []any) (string, string, []any) {
	return ProjectOrdinaryText(text), ProjectOrdinaryText(displayText), projectPublicList(fileReferences)
}

func ProjectOrdinaryRecords(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	projected, _ := domainprivacy.ProjectPublicValue(values)
	out, _ := projected.([]map[string]any)
	return out
}

// ProjectHostUserInputQuestions preserves only ids derived from the supplied
// host input authority while projecting every display or extension field.
func ProjectHostUserInputQuestions(inputID string, values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	projected, _ := domainprivacy.ProjectPublicValue(map[string]any{
		"kind": "user_input", "inputId": strings.TrimSpace(inputID), "questions": values,
	})
	questions, _ := projected.(map[string]any)["questions"].([]map[string]any)
	return questions
}

func ProjectStringRecords(values []map[string]string) []map[string]string {
	if values == nil {
		return nil
	}
	// Answer records originate at the HTTP/client boundary. Even fields named
	// id are not authority until matched to a host-issued question, so inspect
	// every string and fail closed to placeholders on PII-shaped values.
	projected, _ := domainprivacy.ProjectUntrustedValue(values)
	out, _ := projected.([]map[string]string)
	return out
}

// ProjectOrdinaryUntrustedValue projects every string in a display-owned
// value. It is appropriate for extensible UI payloads whose nested keys are
// not an authority schema.
func ProjectOrdinaryUntrustedValue(value any) (any, bool) {
	return domainprivacy.ProjectUntrustedValue(value)
}

// ProjectAttachmentMetadata returns a detached public copy. The private
// attachment plan remains unchanged for its receipt-bound provider attempt.
func ProjectAttachmentMetadata(values []map[string]any) []map[string]any {
	return ProjectOrdinaryRecords(values)
}

func projectPublicList(values []any) []any {
	if values == nil {
		return nil
	}
	projected, _ := domainprivacy.ProjectPublicValue(values)
	out, _ := projected.([]any)
	return out
}
