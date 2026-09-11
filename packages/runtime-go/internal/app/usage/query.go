package usage

import (
	"sort"
	"strings"
)

type QueryRepository interface {
	ThreadExists(threadID string) (bool, error)
	LoadRecords(threadID string) ([]Record, error)
}

type QueryService struct {
	Repository QueryRepository
}

func (s QueryService) Records(threadID string) ([]Record, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		exists, err := s.Repository.ThreadExists(threadID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return []Record{}, nil
		}
	}
	records, err := s.Repository.LoadRecords(threadID)
	if err != nil {
		return nil, err
	}
	SortRecords(records)
	return records, nil
}

func SortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CompletedAt == records[j].CompletedAt {
			if records[i].ThreadID == records[j].ThreadID {
				return records[i].Model < records[j].Model
			}
			return records[i].ThreadID < records[j].ThreadID
		}
		return records[i].CompletedAt < records[j].CompletedAt
	})
}
