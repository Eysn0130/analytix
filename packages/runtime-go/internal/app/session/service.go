package session

import (
	"errors"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type Repository interface {
	ResumeSession(sessionID string, request map[string]any) (map[string]any, error)
}

type Service struct {
	repository Repository
}

type Dependencies struct {
	Repository Repository
}

func NewService(deps Dependencies) *Service {
	return &Service{repository: deps.Repository}
}

func (s *Service) Resume(sessionID string, request map[string]any) (map[string]any, error) {
	if s.repository == nil {
		return nil, errors.New("session repository is required")
	}
	response, err := s.repository.ResumeSession(sessionID, contracts.CloneMap(request))
	if err != nil {
		return nil, err
	}
	return contracts.CloneMap(response), nil
}
