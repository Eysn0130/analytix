package turn

import "analytix.local/runtime-go/internal/ports"

type Service struct {
	events ports.EventRecorder
}

func NewService(events ports.EventRecorder) *Service {
	return &Service{events: events}
}
