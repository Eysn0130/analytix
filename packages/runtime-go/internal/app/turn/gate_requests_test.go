package turn

import "testing"

func TestApprovalRequestRecordsPersistOnlyOpaqueContinuationReference(t *testing.T) {
	receiptID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	item, event := ApprovalRequestRecords(ApprovalRequestInput{
		ThreadID:              " thread_1 ",
		TurnID:                " turn_1 ",
		ItemID:                " item_appr ",
		ApprovalID:            " appr_1 ",
		CreatedAt:             "2026-07-02T00:00:00Z",
		ToolName:              " bash ",
		ApprovalPolicy:        " on-request ",
		SandboxMode:           " workspace-write ",
		ContinuationReceiptID: receiptID,
	})
	if item["kind"] != "approval" ||
		item["summary"] != "Approve bash" ||
		event["kind"] != "approval_requested" ||
		event["approvalPolicy"] != "on-request" ||
		event["sandboxMode"] != "workspace-write" || event["continuationReceiptId"] != receiptID ||
		item["continuationReceiptId"] != receiptID {
		t.Fatalf("approval request records mismatch item=%#v event=%#v", item, event)
	}
	for _, forbidden := range []string{"turnSecurityContext", "executionGrant", "toolScope", "toolCallItemId", "guiPlan", "reasoningEffort"} {
		if _, exists := event[forbidden]; exists {
			t.Fatalf("private continuation field %q reached public approval event: %#v", forbidden, event)
		}
	}
}

func TestUserInputRequestRecordsPersistOnlyOpaqueContinuationReference(t *testing.T) {
	questions := []map[string]any{{"id": "input_123456789012_1", "question": "核对账号 6222020000000000000", "options": []map[string]string{{"label": "电话 13800138000"}}}}
	receiptID := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	item, event := UserInputRequestRecords(UserInputRequestInput{
		ThreadID:              "thread_1",
		TurnID:                "turn_1",
		ItemID:                "item_input",
		InputID:               "input_123456789012",
		CreatedAt:             "2026-07-02T00:00:00Z",
		Prompt:                " Proceed? ",
		Questions:             questions,
		ContinuationReceiptID: receiptID,
	})
	if item["kind"] != "user_input" ||
		item["prompt"] != "Proceed?" ||
		event["kind"] != "user_input_requested" ||
		event["inputId"] != "input_123456789012" || event["continuationReceiptId"] != receiptID ||
		item["continuationReceiptId"] != receiptID {
		t.Fatalf("user input request records mismatch item=%#v event=%#v", item, event)
	}
	if item["questions"] == nil || event["questions"] == nil {
		t.Fatalf("questions should be present item=%#v event=%#v", item, event)
	}
	projectedQuestions := item["questions"].([]map[string]any)
	if projectedQuestions[0]["id"] != "input_123456789012_1" || projectedQuestions[0]["question"] != "核对账号 [ACCOUNT]" {
		t.Fatalf("question PII reached ordinary record: %#v", projectedQuestions)
	}
	projectedOptions := projectedQuestions[0]["options"].([]map[string]string)
	if projectedOptions[0]["label"] != "电话 [PHONE]" {
		t.Fatalf("option PII reached ordinary record: %#v", projectedOptions)
	}
	for _, forbidden := range []string{"turnSecurityContext", "executionGrant", "toolScope", "toolCallItemId", "guiPlan", "reasoningEffort"} {
		if _, exists := event[forbidden]; exists {
			t.Fatalf("private continuation field %q reached public user-input event: %#v", forbidden, event)
		}
	}
}

func TestGateResolvedEventsAndUserInputOutput(t *testing.T) {
	approval := ApprovalResolvedEvent(PendingGateCancellation{
		ID:       " approval_1 ",
		ThreadID: " thread_1 ",
		TurnID:   " turn_1 ",
		ItemID:   " item_approval ",
		ToolName: " bash ",
	}, " allowed ")
	if approval["kind"] != "approval_resolved" ||
		approval["approvalId"] != "approval_1" ||
		approval["status"] != "allowed" ||
		approval["summary"] != "Approve bash" {
		t.Fatalf("approval resolved event mismatch: %#v", approval)
	}

	userInput := UserInputResolvedEvent(PendingGateCancellation{
		ID:       " input_1 ",
		ThreadID: " thread_1 ",
		TurnID:   " turn_1 ",
		ItemID:   " item_input ",
		Prompt:   " Continue? ",
	}, " submitted ")
	if userInput["kind"] != "user_input_resolved" ||
		userInput["inputId"] != "input_1" ||
		userInput["status"] != "submitted" ||
		userInput["prompt"] != "Continue?" {
		t.Fatalf("user input resolved event mismatch: %#v", userInput)
	}

	output := UserInputResolvedOutput(" submitted ", []map[string]string{{"q": "yes"}}, false)
	if output["status"] != "submitted" || output["answerCount"] != float64(1) || output["answers"] == nil {
		t.Fatalf("user input output mismatch: %#v", output)
	}
	privateOutput := UserInputResolvedOutput("submitted", []map[string]string{{"id": "q1", "label": "卡号 6222020000000000000", "value": "6222020000000000000"}}, false)
	privateAnswers := privateOutput["answers"].([]map[string]string)
	if privateAnswers[0]["label"] != "卡号 [ACCOUNT]" || privateAnswers[0]["value"] != "[ACCOUNT]" {
		t.Fatalf("resolved answer PII reached ordinary output: %#v", privateAnswers)
	}
	cancelled := UserInputResolvedOutput(" cancelled ", []map[string]string{{"q": "yes"}}, true)
	if cancelled["status"] != "cancelled" || cancelled["answers"] != nil || cancelled["answerCount"] != nil {
		t.Fatalf("cancelled user input output mismatch: %#v", cancelled)
	}
}
