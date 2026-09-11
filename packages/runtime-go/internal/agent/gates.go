//go:build !analytix_prod

package agent

import controlapp "analytix.local/runtime-go/internal/app/control"

type ApprovalUserInputManager = controlapp.ApprovalUserInputManager
type GateRecord = controlapp.GateRecord

var NewApprovalUserInputManager = controlapp.NewApprovalUserInputManager
var GateRecordToolName = controlapp.GateRecordToolName
var GateRecordPrompt = controlapp.GateRecordPrompt

func RunApprovalUserInputContractExercise() map[string]any {
	return controlapp.RunApprovalUserInputContractExercise()
}

func ApprovalItem(threadID, turnID, itemID, approvalID, now string) map[string]any {
	return controlapp.ApprovalItem(threadID, turnID, itemID, approvalID, now)
}

func UserInputItem(threadID, turnID, itemID, inputID, now string) map[string]any {
	return controlapp.UserInputItem(threadID, turnID, itemID, inputID, now)
}
