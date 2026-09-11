package thread

type Thread struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Status    string `json:"status,omitempty"`
}

type Turn struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
	Status   string `json:"status"`
}

type ForkIdentity struct {
	SourceThreadID string `json:"sourceThreadId"`
	TargetThreadID string `json:"targetThreadId"`
}

type ResumeIdentity struct {
	SessionID string `json:"sessionId"`
	ThreadID  string `json:"threadId"`
}

type CompactionResult struct {
	ThreadID                string
	TurnID                  string
	ItemID                  string
	Summary                 string
	ReplacedTokens          int
	PinnedConstraints       []string
	SourceDigest            string
	DigestMarker            string
	SourceItemIDs           []string
	ReasoningExclusionProof string
	CompactedTurns          int
	RemainingTurns          int
}
