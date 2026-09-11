package loop

import "analytix.local/runtime-go/internal/ports"

type Service struct {
	provider ports.ProviderClient
	events   ports.EventRecorder
}

type Dependencies struct {
	Provider ports.ProviderClient
	Events   ports.EventRecorder
}

func NewService(deps Dependencies) *Service {
	return &Service{provider: deps.Provider, events: deps.Events}
}
