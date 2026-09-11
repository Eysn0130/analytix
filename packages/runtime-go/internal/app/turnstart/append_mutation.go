package turnstart

import threadapp "analytix.local/runtime-go/internal/app/thread"

type AppendMutationInput struct {
	Thread            map[string]any
	Turn              map[string]any
	ThreadID          string
	ProviderID        string
	ThreadPatch       map[string]any
	ExpectedWorkspace string
	ExpectedDigest    string
	UpdatedAt         string
}

func ApplyAppendMutation(input AppendMutationInput) (map[string]any, error) {
	appendInput := threadapp.AppendTurnInput{
		Thread: input.Thread, Turn: input.Turn, ProviderID: input.ProviderID,
		ThreadPatch: input.ThreadPatch, UpdatedAt: input.UpdatedAt,
	}
	if input.ExpectedDigest == "" {
		return threadapp.AppendTurn(appendInput), nil
	}
	return AppendIfBaseline(appendInput, input.ThreadID, input.ExpectedWorkspace, input.ExpectedDigest)
}
