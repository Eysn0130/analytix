//go:build !analytix_prod

package agent

type UserInputAnswer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type ApprovalUserInputRouteContract struct {
	ID                 string                     `json:"id"`
	RuntimeToken       string                     `json:"runtimeToken"`
	ThreadID           string                     `json:"threadId"`
	TurnID             string                     `json:"turnId"`
	Approval           ApprovalRouteDecision      `json:"approval"`
	Replay             ApprovalUserInputReplay    `json:"replay"`
	UserInput          ApprovalUserInputCancel    `json:"userInput"`
	SubmittedUserInput ApprovalUserInputSubmitted `json:"submittedUserInput"`
	ResumePendingGates ApprovalUserInputResume    `json:"resumePendingGates"`
	AbortCleanup       ApprovalUserInputAbort     `json:"abortCleanup"`
	RemoteEntry        ApprovalUserInputRemote    `json:"remoteEntry"`
}

type ApprovalRouteDecision struct {
	ID                   string                            `json:"id"`
	ItemID               string                            `json:"itemId"`
	ToolName             string                            `json:"toolName"`
	Summary              string                            `json:"summary"`
	DecisionRequest      ApprovalDecisionRequest           `json:"decisionRequest"`
	ExpectedResponse     ApprovalUserInputExpectedResponse `json:"expectedResponse"`
	SecondDecisionStatus int                               `json:"secondDecisionStatus"`
	PendingBefore        int                               `json:"pendingBefore"`
	PendingAfter         int                               `json:"pendingAfter"`
}

type ApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type ApprovalUserInputReplay struct {
	SinceSeq             int      `json:"sinceSeq"`
	ExpectedKindsInOrder []string `json:"expectedKindsInOrder"`
}

type ApprovalUserInputCancel struct {
	ID                  string                            `json:"id"`
	ItemID              string                            `json:"itemId"`
	Prompt              string                            `json:"prompt"`
	Questions           []ApprovalUserInputQuestion       `json:"questions"`
	ResolveRequest      ApprovalUserInputResolveRequest   `json:"resolveRequest"`
	ExpectedResponse    ApprovalUserInputExpectedResponse `json:"expectedResponse"`
	SecondResolveStatus int                               `json:"secondResolveStatus"`
	PendingBefore       int                               `json:"pendingBefore"`
	PendingAfter        int                               `json:"pendingAfter"`
}

type ApprovalUserInputSubmitted struct {
	ThreadID           string                            `json:"threadId"`
	ID                 string                            `json:"id"`
	ItemID             string                            `json:"itemId"`
	Prompt             string                            `json:"prompt"`
	Questions          []ApprovalUserInputQuestion       `json:"questions"`
	ResolveRequest     ApprovalUserInputResolveRequest   `json:"resolveRequest"`
	ExpectedResolution ApprovalUserInputResolution       `json:"expectedResolution"`
	ExpectedResponse   ApprovalUserInputExpectedResponse `json:"expectedResponse"`
	PendingBefore      int                               `json:"pendingBefore"`
	PendingAfter       int                               `json:"pendingAfter"`
	ResolvedEvent      ApprovalUserInputResolvedEvent    `json:"resolvedEvent"`
}

type ApprovalUserInputQuestion struct {
	Header   string                    `json:"header"`
	ID       string                    `json:"id"`
	Question string                    `json:"question"`
	Options  []ApprovalUserInputOption `json:"options"`
}

type ApprovalUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type ApprovalUserInputResolveRequest struct {
	Cancelled bool              `json:"cancelled,omitempty"`
	Answers   []UserInputAnswer `json:"answers,omitempty"`
}

type ApprovalUserInputResolution struct {
	Status  string            `json:"status"`
	Answers []UserInputAnswer `json:"answers"`
}

type ApprovalUserInputExpectedResponse struct {
	Status int            `json:"status"`
	Body   map[string]any `json:"body"`
}

type ApprovalUserInputResolvedEvent struct {
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	IncludesAnswers bool   `json:"includesAnswers"`
}

type ApprovalUserInputAbort struct {
	ApprovalID                 string   `json:"approvalId"`
	UserInputID                string   `json:"userInputId"`
	ExpectedApprovalStatus     string   `json:"expectedApprovalStatus"`
	ExpectedUserInputStatus    string   `json:"expectedUserInputStatus"`
	LateApprovalDecisionStatus int      `json:"lateApprovalDecisionStatus"`
	LateUserInputResolveStatus int      `json:"lateUserInputResolveStatus"`
	ExpectedReplayKinds        []string `json:"expectedReplayKinds"`
}

type ApprovalUserInputResume struct {
	SourceThreadID          string                    `json:"sourceThreadId"`
	ApprovalID              string                    `json:"approvalId"`
	UserInputID             string                    `json:"userInputId"`
	ExpectedSourceStatuses  ApprovalUserInputStatuses `json:"expectedSourceStatuses"`
	ExpectedResumedStatuses ApprovalUserInputStatuses `json:"expectedResumedStatuses"`
}

type ApprovalUserInputStatuses struct {
	Approval  string `json:"approval"`
	UserInput string `json:"userInput"`
}

type ApprovalUserInputRemote struct {
	ThreadID          string         `json:"threadId"`
	StartRequest      map[string]any `json:"startRequest"`
	ExpectedPortKeys  []string       `json:"expectedPortKeys"`
	ForbiddenPortKeys []string       `json:"forbiddenPortKeys"`
	RejectedOverride  map[string]any `json:"rejectedOverride"`
}
