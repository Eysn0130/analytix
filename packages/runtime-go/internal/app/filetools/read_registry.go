package filetools

import (
	"strings"
	"sync"
)

type ReadRegistry struct {
	mu      sync.Mutex
	records map[string]map[string]ReadRecord
}

type ReadRegistryRecordInput struct {
	ThreadID string
	Key      string
	Record   ReadRecord
}

type ReadRegistryInputQuery struct {
	ThreadID     string
	TurnID       string
	Key          string
	Path         string
	RelativePath string
	Args         map[string]any
	IsGoFile     bool
}

func NewReadRegistry() *ReadRegistry {
	return &ReadRegistry{records: map[string]map[string]ReadRecord{}}
}

func (r *ReadRegistry) Clear(threadID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, strings.TrimSpace(threadID))
}

func (r *ReadRegistry) Record(input ReadRegistryRecordInput) {
	if r == nil {
		return
	}
	threadID := strings.TrimSpace(input.ThreadID)
	key := strings.TrimSpace(input.Key)
	if threadID == "" || key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records[threadID] == nil {
		r.records[threadID] = map[string]ReadRecord{}
	}
	r.records[threadID][key] = input.Record
}

func (r *ReadRegistry) Input(query ReadRegistryInputQuery) ReadBeforeChangeInput {
	var record ReadRecord
	hasRecord := false
	if r != nil {
		r.mu.Lock()
		record, hasRecord = r.records[strings.TrimSpace(query.ThreadID)][strings.TrimSpace(query.Key)]
		r.mu.Unlock()
	}
	return ReadBeforeChangeInput{
		TurnID:       strings.TrimSpace(query.TurnID),
		Path:         strings.TrimSpace(query.Path),
		RelativePath: strings.TrimSpace(query.RelativePath),
		Args:         query.Args,
		Record:       record,
		HasRecord:    hasRecord,
		IsGoFile:     query.IsGoFile,
	}
}
