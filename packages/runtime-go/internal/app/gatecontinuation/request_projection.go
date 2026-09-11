package gatecontinuation

import (
	"encoding/json"
	"errors"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

// GateRequestProjectionV1 is the deterministic public projection of one
// trusted private continuation receipt. The receipt is the durable request
// intent; retries and restart repair must never derive timestamps or display
// fields from current process state.
type GateRequestProjectionV1 struct {
	Kind         string
	GateID       string
	ManagerLabel string
	Record       controlapp.GateRecord
	Item         map[string]any
	Event        map[string]any
}

// ProjectVerifiedGateRequestV1 accepts a structurally valid receipt. Its
// caller must first pin the signature to the current installation authority;
// this pure projector deliberately has no authority or persistence side
// effects.
func ProjectVerifiedGateRequestV1(receipt domaincontinuation.Receipt) (GateRequestProjectionV1, error) {
	if err := domaincontinuation.ValidateReceipt(receipt); err != nil {
		return GateRequestProjectionV1{}, err
	}
	payload := receipt.Payload
	switch payload.Kind {
	case domaincontinuation.KindApproval:
		item, event := appturn.ApprovalRequestRecords(appturn.ApprovalRequestInput{
			ThreadID: payload.ThreadID, TurnID: payload.TurnID, ItemID: payload.ItemID,
			ApprovalID: payload.GateID, CreatedAt: payload.IssuedAt, ToolName: payload.ToolName,
			ApprovalPolicy: payload.ApprovalPolicy, SandboxMode: payload.SandboxMode,
			ContinuationReceiptID: receipt.ReceiptID,
		})
		return GateRequestProjectionV1{
			Kind: payload.Kind, GateID: payload.GateID, ManagerLabel: payload.ToolName,
			Record: controlapp.GateRecord{
				ThreadID: payload.ThreadID, TurnID: payload.TurnID, ItemID: payload.ItemID,
				ToolName: payload.ToolName, ContinuationReceiptID: receipt.ReceiptID,
			},
			Item: item, Event: event,
		}, nil
	case domaincontinuation.KindUserInput:
		args := map[string]any{}
		if err := json.Unmarshal(payload.Arguments, &args); err != nil {
			return GateRequestProjectionV1{}, errors.New("user-input continuation arguments are invalid")
		}
		prompt := privacyprojectionapp.ProjectOrdinaryText(firstText(args["prompt"], args["question"], "User input required"))
		questions := toolcatalogapp.UserInputQuestions(args, payload.GateID, prompt)
		item, event := appturn.UserInputRequestRecords(appturn.UserInputRequestInput{
			ThreadID: payload.ThreadID, TurnID: payload.TurnID, ItemID: payload.ItemID,
			InputID: payload.GateID, CreatedAt: payload.IssuedAt, Prompt: prompt,
			Questions: questions, ContinuationReceiptID: receipt.ReceiptID,
		})
		return GateRequestProjectionV1{
			Kind: payload.Kind, GateID: payload.GateID, ManagerLabel: prompt,
			Record: controlapp.GateRecord{
				ThreadID: payload.ThreadID, TurnID: payload.TurnID, ItemID: payload.ItemID,
				Prompt: prompt, ContinuationReceiptID: receipt.ReceiptID,
			},
			Item: item, Event: event,
		}, nil
	default:
		return GateRequestProjectionV1{}, errors.New("continuation request kind is unsupported")
	}
}

func validateGateRequestProjectionV1(projection GateRequestProjectionV1) error {
	if strings.TrimSpace(projection.GateID) == "" || strings.TrimSpace(projection.Record.ThreadID) == "" ||
		strings.TrimSpace(projection.Record.TurnID) == "" || strings.TrimSpace(projection.Record.ItemID) == "" ||
		strings.TrimSpace(projection.Record.ContinuationReceiptID) == "" || projection.Item == nil || projection.Event == nil {
		return errors.New("gate request projection is incomplete")
	}
	return nil
}
