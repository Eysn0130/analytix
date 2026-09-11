package subagent

import (
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

type TaskJobPauseRuntime interface {
	RequestBackgroundJobPause(id string, pauseRequestID string, now time.Time) (BackgroundJobPauseSnapshot, bool)
	ClearBackgroundJobPauseRequest(id string, pauseRequestID string) bool
	BackgroundJobPauseSnapshot(id string) (BackgroundJobPauseSnapshot, bool)
	ResumeBackgroundJob(id string, pauseRequestID string) (BackgroundJobPauseSnapshot, bool)
}

type Service struct {
	jobs            ports.JobRepository
	provider        ports.ProviderClient
	events          ports.EventRecorder
	state           *RuntimeState
	pauseRuntime    TaskJobPauseRuntime
	worktreeManager ports.WorktreeManager
	childTodos      ChildTodoStore
	authorizeRecord func(string, domainjob.Record) bool
}

type Dependencies struct {
	Jobs            ports.JobRepository
	Provider        ports.ProviderClient
	Events          ports.EventRecorder
	State           *RuntimeState
	PauseRuntime    TaskJobPauseRuntime
	WorktreeManager ports.WorktreeManager
	ChildTodos      ChildTodoStore
	AuthorizeRecord func(parentThreadID string, record domainjob.Record) bool
}

func NewService(deps Dependencies) *Service {
	pauseRuntime := deps.PauseRuntime
	if pauseRuntime == nil {
		pauseRuntime = deps.State
	}
	return &Service{jobs: deps.Jobs, provider: deps.Provider, events: deps.Events, state: deps.State, pauseRuntime: pauseRuntime, worktreeManager: deps.WorktreeManager, childTodos: deps.ChildTodos, authorizeRecord: deps.AuthorizeRecord}
}
