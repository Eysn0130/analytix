package turnstart

import (
	"context"
	"errors"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

var ErrBaselineConflict = errors.New("durable thread changed before turn start commit")

type StartBaselineReaderV1 interface {
	ReadThreadStartBaseline(string) (map[string]any, string, error)
}

type StartBaselineCompactorV1 interface {
	AutoCompactBeforeTurnV1(context.Context, threadapp.AutoCompactionInputV1) (threadapp.AutoCompactionResultV1, error)
}

type PrepareStartBaselineInputV1 struct {
	Context             context.Context
	Store               StartBaselineReaderV1
	Compactor           StartBaselineCompactorV1
	ThreadID            string
	Prompt              string
	MainThread          bool
	Reserved            bool
	ValidateReserved    func(map[string]any) error
	ContextWindowTokens func(map[string]any) int
}

func PrepareStartBaselineV1(input PrepareStartBaselineInputV1) (map[string]any, string, error) {
	if input.Store == nil {
		return nil, "", errors.New("durable thread start baseline store is unavailable")
	}
	thread, digest, err := input.Store.ReadThreadStartBaseline(input.ThreadID)
	if err != nil {
		return nil, "", err
	}
	if thread == nil {
		return nil, "", controlapp.ErrThreadNotFound
	}
	if input.Reserved {
		if input.ValidateReserved == nil {
			return nil, "", errors.New("reserved turn baseline authority is unavailable")
		}
		return thread, digest, input.ValidateReserved(thread)
	}
	if input.Compactor == nil || input.ContextWindowTokens == nil {
		return nil, "", errors.New("automatic compaction authority is unavailable")
	}
	result, err := input.Compactor.AutoCompactBeforeTurnV1(input.Context, threadapp.AutoCompactionInputV1{
		ThreadID: input.ThreadID, Prompt: input.Prompt, ContextWindowTokens: input.ContextWindowTokens(thread),
		MainThread: input.MainThread,
	})
	if err != nil {
		return nil, "", err
	}
	if !result.Compacted {
		return thread, digest, nil
	}
	thread, digest, err = input.Store.ReadThreadStartBaseline(input.ThreadID)
	if err != nil || thread == nil {
		return nil, "", errors.Join(controlapp.ErrThreadNotFound, err)
	}
	return thread, digest, nil
}

func BaselineDigest(thread map[string]any) (string, error) {
	digest, err := threadapp.MutationBaselineDigest(thread)
	if err != nil {
		return "", errors.New("durable thread start baseline is not canonical")
	}
	return digest, nil
}

func ValidateBaseline(thread map[string]any, threadID, workspace, expectedDigest string) error {
	currentDigest, err := BaselineDigest(thread)
	if err != nil {
		return ErrBaselineConflict
	}
	return ValidateReadback(thread, threadID, workspace, currentDigest, expectedDigest)
}

func ValidateReadback(thread map[string]any, threadID, workspace, currentDigest, expectedDigest string) error {
	if !domainsecurity.IsSHA256Hex(expectedDigest) || currentDigest != expectedDigest ||
		strings.TrimSpace(threadappString(thread, "id")) != strings.TrimSpace(threadID) ||
		strings.EqualFold(threadappString(thread, "status"), "running") ||
		strings.TrimSpace(threadappString(thread, "workspace")) != strings.TrimSpace(workspace) {
		return ErrBaselineConflict
	}
	return nil
}

func AppendIfBaseline(input threadapp.AppendTurnInput, threadID, workspace, expectedDigest string) (map[string]any, error) {
	if err := ValidateBaseline(input.Thread, threadID, workspace, expectedDigest); err != nil {
		return nil, err
	}
	if err := ValidateUnoccupiedTurnIDV1(input.Thread, threadappString(input.Turn, "id")); err != nil {
		return nil, err
	}
	return threadapp.AppendTurn(input), nil
}

// ValidateUnoccupiedTurnIDV1 permits a new turn in an existing child thread,
// but never treats a completed or failed turn as a reusable allocation.
func ValidateUnoccupiedTurnIDV1(thread map[string]any, turnID string) error {
	if thread == nil || !domainthread.IsCanonicalRecordID(turnID) {
		return ErrBaselineConflict
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return ErrBaselineConflict
	}
	for _, value := range turns {
		turn, ok := value.(map[string]any)
		if !ok || !domainthread.IsCanonicalRecordID(threadappString(turn, "id")) || threadappString(turn, "id") == turnID {
			return ErrBaselineConflict
		}
	}
	return nil
}

func threadappString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
