package usage

import (
	"errors"
	"strings"
)

type RuntimeRepository interface {
	QueryRepository
	ListRuntimeThreadIDs() ([]string, error)
}

type RuntimeService struct {
	Repository RuntimeRepository
}

func (s RuntimeService) Records(threadID string) ([]Record, error) {
	return QueryService{Repository: s.Repository}.Records(threadID)
}

func (s RuntimeService) RuntimeResponse(records []Record) (map[string]any, error) {
	if s.Repository == nil {
		return nil, errors.New("runtime usage repository is unavailable")
	}
	threadIDs, err := s.Repository.ListRuntimeThreadIDs()
	if err != nil {
		return nil, err
	}
	return RuntimeResponse(records, threadIDs), nil
}

func (s RuntimeService) ThreadSnapshot(threadID string) (Snapshot, error) {
	records, err := s.Records(strings.TrimSpace(threadID))
	if err != nil {
		return Snapshot{}, err
	}
	return SumRecords(records), nil
}
