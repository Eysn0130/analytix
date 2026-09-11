package threadsummary

import (
	"errors"
	"strings"
	"time"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
)

var ErrThreadNotFound = errors.New("thread not found")

type Repository interface {
	GetThread(threadID string) (map[string]any, error)
	LoadEventsSince(threadID string, afterSeq int) ([]map[string]any, error)
	HighestSeq(threadID string) (int, error)
	ListThreads(archivedOnly bool, includeArchived bool, includeSide bool, search string) ([]map[string]any, error)
}

type JobLister interface {
	ListTaskJobs(threadID string) ([]domainjob.Record, error)
}

type Service struct {
	repository      Repository
	jobs            JobLister
	now             func() time.Time
	publicProjector threadapp.PublicProjector
}

type Dependencies struct {
	Repository      Repository
	Jobs            JobLister
	Now             func() time.Time
	PublicProjector threadapp.PublicProjector
}

type Context struct {
	ThreadID          string
	Thread            map[string]any
	Events            []map[string]any
	LatestSeq         int
	Jobs              []domainjob.Record
	SideChats         []map[string]any
	ChildThreadTitles map[string]string
	CaseRestricted    bool
}

func NewService(deps Dependencies) *Service {
	projector := deps.PublicProjector
	if projector == nil {
		projector = threadapp.NewTrustedPublicProjector(nil)
	}
	return &Service{
		repository:      deps.Repository,
		jobs:            deps.Jobs,
		now:             deps.Now,
		publicProjector: projector,
	}
}

func (s *Service) LoadContext(threadID string) (Context, error) {
	thread, err := s.repository.GetThread(threadID)
	if err != nil {
		return Context{}, err
	}
	if thread == nil {
		return Context{}, ErrThreadNotFound
	}
	if strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
		return Context{}, errors.New("thread summary route and body identity mismatch")
	}
	rawThread := contracts.CloneMap(thread)
	thread, err = s.publicProjector.ProjectThread(rawThread)
	if err != nil {
		return Context{}, err
	}
	latestSeq, _ := s.repository.HighestSeq(threadID)
	if stringField(thread, "historyAuthority") == threadapp.CaseBoundaryOnlyHistoryAuthority {
		caseSubagentJobs := []domainjob.Record{}
		if s.jobs != nil {
			jobsForThread, jobsErr := s.jobs.ListTaskJobs(threadID)
			if jobsErr != nil {
				return Context{}, jobsErr
			}
			for _, record := range jobsForThread {
				if strings.TrimSpace(record.ParentThreadID) != strings.TrimSpace(threadID) ||
					(!IsSubagentJob(record) && !IsChildTaskSubagentJob(record)) {
					continue
				}
				caseSubagentJobs = append(caseSubagentJobs, caseRestrictedSubagentRecord(record))
			}
		}
		return Context{
			ThreadID: threadID, Thread: thread, Events: []map[string]any{}, LatestSeq: latestSeq,
			Jobs: caseSubagentJobs, SideChats: []map[string]any{}, ChildThreadTitles: map[string]string{},
			CaseRestricted: true,
		}, nil
	}
	events, err := s.repository.LoadEventsSince(threadID, 0)
	if err != nil {
		return Context{}, err
	}
	projectedEvents := make([]map[string]any, 0, len(events))
	for _, event := range events {
		projected, visible, projectionErr := s.publicProjector.ProjectEvent(threadID, rawThread, event)
		if projectionErr != nil {
			return Context{}, projectionErr
		}
		if visible && projected != nil {
			projectedEvents = append(projectedEvents, projected)
		}
	}
	jobsForThread, err := s.jobs.ListTaskJobs(threadID)
	if err != nil {
		return Context{}, err
	}

	childThreadIDs := ChildThreadIDs(jobsForThread, projectedEvents)
	sideChats := []map[string]any{}
	childThreadTitles := map[string]string{}
	if threads, listErr := s.repository.ListThreads(false, true, true, ""); listErr == nil {
		for _, candidate := range threads {
			publicCandidate, projectionErr := s.publicProjector.ProjectThread(candidate)
			if projectionErr != nil || publicCandidate == nil || stringField(publicCandidate, "parentThreadId") != threadID {
				continue
			}
			candidateID := stringField(publicCandidate, "id")
			if childThreadIDs[candidateID] {
				if title := stringField(publicCandidate, "title"); title != "" {
					childThreadTitles[candidateID] = title
				}
				continue
			}
			if stringField(publicCandidate, "relation") != "side" {
				continue
			}
			sideChats = append(sideChats, map[string]any{
				"threadId":       candidateID,
				"title":          stringField(publicCandidate, "title"),
				"status":         firstNonEmptyAnyString(publicCandidate["status"], "idle"),
				"relation":       "side",
				"parentThreadId": threadID,
				"messageCount":   floatFromAny(publicCandidate["messageCount"]),
				"turnCount":      floatFromAny(publicCandidate["turnCount"]),
				"createdAt":      firstNonEmptyAnyString(publicCandidate["createdAt"], s.nowUTC().Format(time.RFC3339Nano)),
				"updatedAt":      firstNonEmptyAnyString(publicCandidate["updatedAt"], s.nowUTC().Format(time.RFC3339Nano)),
			})
		}
	}
	for childThreadID := range childThreadIDs {
		child, childErr := s.repository.GetThread(childThreadID)
		if childErr != nil || child == nil {
			continue
		}
		child, childErr = s.publicProjector.ProjectThread(child)
		if childErr != nil || child == nil {
			continue
		}
		if title := stringField(child, "title"); title != "" {
			childThreadTitles[childThreadID] = title
		}
	}
	return Context{
		ThreadID:          threadID,
		Thread:            thread,
		Events:            cloneEvents(projectedEvents),
		LatestSeq:         latestSeq,
		Jobs:              append([]domainjob.Record(nil), jobsForThread...),
		SideChats:         cloneMaps(sideChats),
		ChildThreadTitles: childThreadTitles,
	}, nil
}

func caseRestrictedSubagentRecord(record domainjob.Record) domainjob.Record {
	return domainjob.Record{
		ID:               record.ID,
		ParentThreadID:   record.ParentThreadID,
		ParentTurnID:     record.ParentTurnID,
		ParentToolCallID: record.ParentToolCallID,
		ChildThreadID:    record.ChildThreadID,
		ChildTurnID:      record.ChildTurnID,
		Kind:             record.Kind,
		Status:           record.Status,
		Model:            record.Model,
		ProviderID:       record.ProviderID,
		EndpointFormat:   record.EndpointFormat,
		Variant:          record.Variant,
		ModelSource:      record.ModelSource,
		Effort:           record.Effort,
		ProfileName:      record.ProfileName,
		ToolPolicy:       record.ToolPolicy,
		ParallelGroupID:  record.ParallelGroupID,
		ParallelIndex:    record.ParallelIndex,
		Background:       record.Background,
		QueuedAt:         record.QueuedAt,
		StartedAt:        record.StartedAt,
		FinishedAt:       record.FinishedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func (s *Service) Summary(threadID string) (map[string]any, error) {
	context, err := s.LoadContext(threadID)
	if err != nil {
		return nil, err
	}
	summary := s.SummaryFromContext(context)
	if err := ValidateSummaryResponseV1(summary, threadID); err != nil {
		return nil, errors.New("thread summary failed public contract validation")
	}
	return summary, nil
}

func (s *Service) SummaryFromContext(context Context) map[string]any {
	threadID := firstNonEmptyAnyString(context.ThreadID, stringField(context.Thread, "id"))
	tasks := s.Tasks(context)
	if context.CaseRestricted {
		tasks = []any{}
	}
	summary := map[string]any{
		"threadId":            threadID,
		"generatedAt":         s.nowUTC().Format(time.RFC3339Nano),
		"latestSeq":           float64(context.LatestSeq),
		"subagents":           s.Subagents(context),
		"tasks":               tasks,
		"outputs":             []any{},
		"sources":             []any{},
		"sideChats":           cloneMaps(context.SideChats),
		"backgroundProcesses": []any{},
	}
	if context.CaseRestricted {
		summary["historyAuthority"] = threadapp.CaseBoundaryOnlyHistoryAuthority
	}
	ordinary := domainordinary.ProjectValueV1(summary)
	projected, _ := ordinary.(map[string]any)
	if projected != nil && domainevent.ValidatePublicRecord(projected) == nil {
		return projected
	}
	fallback := map[string]any{
		"threadId":            domainordinary.ProjectTextV1(threadID),
		"generatedAt":         s.nowUTC().Format(time.RFC3339Nano),
		"latestSeq":           float64(context.LatestSeq),
		"subagents":           []any{},
		"tasks":               []any{},
		"outputs":             []any{},
		"sources":             []any{},
		"sideChats":           []map[string]any{},
		"backgroundProcesses": []any{},
	}
	if context.CaseRestricted {
		fallback["historyAuthority"] = threadapp.CaseBoundaryOnlyHistoryAuthority
	}
	return fallback
}

func (s *Service) Subagents(context Context) []any {
	byKey := map[string]map[string]any{}
	appContext := subagentContext(context)
	for _, record := range context.Jobs {
		if !IsSubagentJob(record) {
			continue
		}
		UpsertByUpdatedAt(byKey, SubagentFromJob(appContext, record, s.nowUTC()))
	}
	for _, event := range context.Events {
		if child, _ := event["child"].(map[string]any); child != nil {
			childWithTimestamp := contracts.CloneMap(child)
			if firstNonEmptyAnyString(childWithTimestamp["updatedAt"], childWithTimestamp["timestamp"]) == "" {
				SetString(childWithTimestamp, "updatedAt", firstNonEmptyAnyString(event["timestamp"]))
			}
			if subagent := SubagentFromEventChild(appContext, childWithTimestamp, s.nowUTC()); subagent != nil {
				UpsertByUpdatedAt(byKey, subagent)
			}
		}
	}
	childThreadIDs := SubagentChildThreadIDs(byKey)
	for _, record := range context.Jobs {
		if !IsChildTaskSubagentJob(record) {
			continue
		}
		if childThreadID := strings.TrimSpace(record.ChildThreadID); childThreadID != "" && childThreadIDs[childThreadID] {
			continue
		}
		subagent := SubagentFromTaskJob(appContext, record, s.nowUTC())
		UpsertByUpdatedAt(byKey, subagent)
		if childThreadID := strings.TrimSpace(record.ChildThreadID); childThreadID != "" {
			childThreadIDs[childThreadID] = true
		}
	}
	ApplySubagentJobThreadAuthority(byKey, context.Jobs)
	return SortedValues(byKey)
}

func (s *Service) Tasks(context Context) []any {
	byKey := map[string]map[string]any{}
	for _, record := range context.Jobs {
		if !IsTaskJob(record) {
			continue
		}
		task := TaskFromJob(record, s.nowUTC())
		byKey[stringField(task, "id")] = task
	}
	for _, command := range s.CommandTasks(context) {
		byKey[stringField(command.Task, "id")] = command.Task
	}
	return SortedValues(byKey)
}

func (s *Service) CommandTasks(context Context) []CommandTask {
	return CommandTasks(context.Thread, s.nowUTC())
}

func (s *Service) CommandOutput(context Context, taskID string, _ int, _ int) (map[string]any, bool) {
	for _, task := range s.CommandTasks(context) {
		if stringField(task.Task, "id") != taskID {
			continue
		}
		return map[string]any{
			"schemaVersion":     1,
			"availability":      "withheld",
			"taskId":            taskID,
			"status":            stringField(task.Task, "status"),
			"reasonCode":        "tool_output_private",
			"outputWithheld":    true,
			"outputTrustStatus": "private_tool_output",
			"factAnswerAllowed": false,
			"evidenceAuthority": false,
			"canReadOutput":     false,
			"canContinueParent": false,
		}, true
	}
	return nil, false
}

func (s *Service) nowUTC() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func subagentContext(context Context) SubagentContext {
	return SubagentContext{
		ParentThreadID:    stringField(context.Thread, "id"),
		ChildThreadTitles: context.ChildThreadTitles,
	}
}

func cloneEvents(events []map[string]any) []map[string]any {
	return cloneMaps(events)
}

func cloneMaps(values []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, contracts.CloneMap(value))
	}
	return out
}
