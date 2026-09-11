package event

type Seq int

type TerminalStatus string

const (
	TerminalCompleted   TerminalStatus = "completed"
	TerminalFailed      TerminalStatus = "failed"
	TerminalAborted     TerminalStatus = "aborted"
	TerminalInterrupted TerminalStatus = "interrupted"
	TerminalKilled      TerminalStatus = "killed"
)

type RuntimeEvent struct {
	Seq       Seq            `json:"seq"`
	Kind      string         `json:"kind"`
	ThreadID  string         `json:"threadId,omitempty"`
	TurnID    string         `json:"turnId,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
	Terminal  bool           `json:"terminal,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}
