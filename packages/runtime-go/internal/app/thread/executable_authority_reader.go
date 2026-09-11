package thread

import (
	"errors"
	"sort"
	"strings"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
)

type ExecutionAuthority interface {
	CanExecute(string) bool
}

type executableAuthorityReader struct {
	reader    AuthorityThreadReader
	authority ExecutionAuthority
}

func NewExecutableAuthorityReader(reader AuthorityThreadReader, authority ExecutionAuthority) AuthorityThreadReader {
	return &executableAuthorityReader{reader: reader, authority: authority}
}

func (reader *executableAuthorityReader) AllThreadIDs() ([]string, error) {
	if reader == nil || reader.reader == nil || reader.authority == nil {
		return nil, errors.New("executable authority reader is unavailable")
	}
	ids, err := reader.reader.AllThreadIDs()
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if reader.authority.CanExecute(id) {
			filtered = append(filtered, strings.TrimSpace(id))
		}
	}
	sort.Strings(filtered)
	return filtered, nil
}

func (reader *executableAuthorityReader) GetThread(threadID string) (map[string]any, error) {
	if reader == nil || reader.reader == nil || reader.authority == nil || !reader.authority.CanExecute(threadID) {
		return nil, errors.New("thread is quarantined by execution authority")
	}
	return reader.reader.GetThread(threadID)
}

func (reader *executableAuthorityReader) QuarantinedThread(threadID string) bool {
	return reader != nil && reader.authority != nil && !reader.authority.CanExecute(threadID)
}

func (reader *executableAuthorityReader) ValidateCaseCompactionAuthorityTurnV1(
	threadID string,
	thread map[string]any,
	turn map[string]any,
) error {
	if reader == nil || reader.reader == nil || reader.authority == nil || !reader.authority.CanExecute(threadID) {
		return errors.New("case compaction execution authority is unavailable")
	}
	validator, ok := reader.reader.(interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	})
	if !ok {
		return errors.New("case compaction committed authority is unavailable")
	}
	return validator.ValidateCaseCompactionAuthorityTurnV1(threadID, thread, turn)
}

func (reader *executableAuthorityReader) CommittedContext(
	threadID string,
	turnID string,
) (casethreadapp.CommittedContext, bool) {
	if reader == nil || reader.reader == nil || reader.authority == nil || !reader.authority.CanExecute(threadID) {
		return casethreadapp.CommittedContext{}, false
	}
	committedReader, ok := reader.reader.(caseCompactionCommittedContextReaderV1)
	if !ok {
		return casethreadapp.CommittedContext{}, false
	}
	return committedReader.CommittedContext(strings.TrimSpace(threadID), strings.TrimSpace(turnID))
}
