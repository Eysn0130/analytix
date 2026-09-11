package goal

type Status string

const (
	StatusActive   Status = "active"
	StatusComplete Status = "complete"
	StatusBlocked  Status = "blocked"
)

type Goal struct {
	ID        string `json:"id"`
	Objective string `json:"objective"`
	Status    Status `json:"status"`
}

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type Todo struct {
	ID      string     `json:"id"`
	Content string     `json:"content"`
	Status  TodoStatus `json:"status"`
}

type Evidence struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}
