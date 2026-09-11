package eventrecording

import (
	"errors"
	"strings"
	"time"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
)

type Input struct {
	Drafts              []map[string]any
	Atomic              bool
	PublicationReserved func(string) bool
	ReadThread          func(string) (map[string]any, error)
	ProjectThread       func(string, map[string]any) (map[string]any, error)
	BeforeRecord        func(map[string]any) error
	NextSequence        func(string) (int, error)
	Persist             func(string, []map[string]any, bool) error
	MissingThread       error
	Now                 func() time.Time
}

type Result struct {
	ThreadID        string
	Events          []map[string]any
	Recorded        []map[string]any
	HighestSequence int
}

// RecordBatch owns validation, public projection, contiguous sequencing, and
// the persist-before-publish contract for ordinary durable events. The caller
// must hold its event owner lock and publishes Result.Events only after this
// function succeeds.
func RecordBatch(input Input) (Result, []string, error) {
	if len(input.Drafts) == 0 || (!input.Atomic && len(input.Drafts) != 1) {
		return Result{}, nil, errors.New("at least one event is required")
	}
	if input.PublicationReserved == nil || input.ReadThread == nil ||
		input.ProjectThread == nil || input.NextSequence == nil || input.Persist == nil ||
		input.MissingThread == nil {
		return Result{}, nil, errors.New("event recording authority is unavailable")
	}
	threadID, _ := input.Drafts[0]["threadId"].(string)
	if strings.TrimSpace(threadID) == "" {
		return Result{}, nil, errors.New("threadId is required")
	}
	if input.PublicationReserved(threadID) {
		return Result{}, nil, acceptedfinaleventport.ErrPublicationReserved
	}
	thread, err := input.ReadThread(threadID)
	if err != nil {
		return Result{}, nil, err
	}
	if thread == nil {
		return Result{}, nil, input.MissingThread
	}
	if contracts.SafeRecordID(threadID) != threadID ||
		strings.TrimSpace(contracts.StringField(thread, "id")) != threadID {
		return Result{}, nil, errors.New("event thread identity is invalid")
	}
	thread, err = input.ProjectThread(threadID, thread)
	if err != nil {
		return Result{}, nil, err
	}
	now := input.Now
	if now == nil {
		now = time.Now
	}
	events := make([]map[string]any, 0, len(input.Drafts))
	for _, draft := range input.Drafts {
		draftThreadID, _ := draft["threadId"].(string)
		if draftThreadID != threadID {
			return Result{}, nil, errors.New("event bundle contains a different threadId")
		}
		if err := turnapp.ValidateGenericTerminalEventPathForThreadV1(thread, draft); err != nil {
			return Result{}, nil, err
		}
		projected, err := turnapp.SanitizeCaseEventPublication(thread, draft)
		if err != nil {
			return Result{}, nil, err
		}
		if input.BeforeRecord != nil {
			if err := input.BeforeRecord(contracts.CloneMap(projected)); err != nil {
				return Result{}, nil, err
			}
		}
		event := contracts.CloneMap(projected)
		event["threadId"] = threadID
		if _, ok := event["timestamp"].(string); !ok {
			event["timestamp"] = now().UTC().Format(time.RFC3339Nano)
		}
		events = append(events, event)
	}
	nextSequence, err := input.NextSequence(threadID)
	if err != nil {
		return Result{}, nil, err
	}
	for index := range events {
		events[index]["seq"] = float64(nextSequence + index)
	}
	order := []string{"persist", "publish"}
	if err := input.Persist(threadID, events, input.Atomic); err != nil {
		return Result{}, order, err
	}
	recorded := make([]map[string]any, 0, len(events))
	for _, event := range events {
		recorded = append(recorded, contracts.CloneMap(event))
	}
	return Result{
		ThreadID: threadID, Events: events, Recorded: recorded,
		HighestSequence: nextSequence + len(events) - 1,
	}, order, nil
}
