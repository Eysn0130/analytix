package terminal

import "analytix.local/runtime-go/internal/ports"

type Service struct {
	jobs        ports.JobRepository
	runner      ports.ProcessRunner
	shellRunner ports.ShellRunner
	events      ports.EventRecorder
}

type Dependencies struct {
	Jobs        ports.JobRepository
	Runner      ports.ProcessRunner
	ShellRunner ports.ShellRunner
	Events      ports.EventRecorder
}

func NewService(deps Dependencies) *Service {
	return &Service{jobs: deps.Jobs, runner: deps.Runner, shellRunner: deps.ShellRunner, events: deps.Events}
}
