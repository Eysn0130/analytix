package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

type durableThreadDirCandidate struct {
	ID      string
	ModTime time.Time
}

func (s *DurableEventSessionStore) EnsureThreadSummaryIndex() error {
	return s.threadSummaryIndex.Ensure()
}

func (s *DurableEventSessionStore) ListThreads(archivedOnly bool, includeArchived bool, includeSide bool, search string) ([]map[string]any, error) {
	s.mu.Lock()
	meta, err := s.readMetaNoLock()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	threadIDs, err := s.threadIDsFromFilesystemReadOnlyNoLock()
	if err != nil {
		return nil, err
	}
	metaChanged := false
	for _, id := range threadIDs {
		if containsString(meta.ListedThreadIDs, id) {
			continue
		}
		meta.ListedThreadIDs = append(meta.ListedThreadIDs, id)
		metaChanged = true
	}
	if metaChanged {
		if err := s.mergeListedThreadIDs(threadIDs); err != nil {
			return nil, err
		}
	}
	needle := strings.ToLower(strings.TrimSpace(search))
	threads := []map[string]any{}
	for _, id := range threadIDs {
		thread, err := s.readThreadNoLock(id)
		if err != nil || thread == nil {
			return nil, errors.Join(err, errors.New("thread list cannot read a canonical thread"))
		}
		summary := threadSummary(thread)
		if !threadapp.IncludeThreadInList(thread, summary, threadapp.ListProjectionFilter{
			ArchivedOnly:    archivedOnly,
			IncludeArchived: includeArchived,
			IncludeSide:     includeSide,
			Search:          needle,
		}) {
			continue
		}
		threads = append(threads, summary)
	}
	threadapp.SortThreadSummaries(threads)
	return threads, nil
}

// RuntimeTurnsIdleForMaintenance scans every canonical thread under the
// durable writer lock. It does not use the public, potentially limited list.
func (s *DurableEventSessionStore) RuntimeTurnsIdleForMaintenance(ctx context.Context) (bool, error) {
	if s == nil {
		return false, errors.New("durable thread store is unavailable")
	}
	if ctx == nil {
		return false, errors.New("maintenance context is unavailable")
	}
	for !s.mu.TryLock() {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(s.pendingEvents) != 0 || len(s.pendingCaseCompactions) != 0 {
		return false, nil
	}
	if s.beforeMaintenanceReadHook != nil {
		s.beforeMaintenanceReadHook()
	}
	entries, err := os.ReadDir(s.threadsDir())
	if errors.Is(err, os.ErrNotExist) {
		return ctx.Err() == nil, ctx.Err()
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		id := entry.Name()
		if !entry.IsDir() || safeDurableID(id) != id {
			return false, errors.New("durable thread inventory contains an unknown entry")
		}
		thread, readErr := s.readThreadNoLock(id)
		if errors.Is(readErr, errForkCreationPendingV1) {
			continue
		}
		if readErr != nil || thread == nil || stringField(thread, "id") != id {
			return false, errors.Join(readErr, errors.New("canonical thread is unavailable"))
		}
		status, ok := thread["status"].(string)
		if !ok {
			return false, errors.New("canonical thread status is unavailable")
		}
		switch status {
		case "running":
			return false, nil
		case "idle", "archived", "deleted":
		default:
			return false, errors.New("canonical thread status is unknown")
		}
		turns, ok := thread["turns"].([]any)
		if !ok {
			return false, errors.New("canonical turns are unavailable")
		}
		for _, raw := range turns {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			turn, ok := raw.(map[string]any)
			if !ok {
				return false, errors.New("canonical turn is invalid")
			}
			turnStatus, ok := turn["status"].(string)
			if !ok {
				return false, errors.New("canonical turn status is unavailable")
			}
			switch turnStatus {
			case "queued", "running", "waiting":
				return false, nil
			case "completed", "failed", "aborted", "interrupted", "killed":
			default:
				return false, errors.New("canonical turn status is unknown")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *DurableEventSessionStore) ListThreadsLimited(archivedOnly bool, includeArchived bool, includeSide bool, search string, limit int) ([]map[string]any, error) {
	if limit <= 0 || strings.TrimSpace(search) != "" {
		return s.ListThreads(archivedOnly, includeArchived, includeSide, search)
	}
	if threads, ok, err := s.threadSummaryIndex.List(archivedOnly, includeArchived, includeSide, search, limit); ok {
		return threads, err
	}
	s.mu.Lock()
	if _, err := s.readMetaNoLock(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()

	candidates, err := s.recentThreadDirCandidatesReadOnly()
	if err != nil {
		return nil, err
	}
	threads := []map[string]any{}
	for _, candidate := range candidates {
		thread, err := s.readThreadNoLock(candidate.ID)
		if err != nil || thread == nil {
			continue
		}
		summary := threadSummary(thread)
		if !threadapp.IncludeThreadInList(thread, summary, threadapp.ListProjectionFilter{
			ArchivedOnly:    archivedOnly,
			IncludeArchived: includeArchived,
			IncludeSide:     includeSide,
		}) {
			continue
		}
		threads = append(threads, summary)
		if len(threads) >= limit {
			break
		}
	}
	threadapp.SortThreadSummaries(threads)
	return threads, nil
}

func (s *DurableEventSessionStore) recentThreadDirCandidatesReadOnly() ([]durableThreadDirCandidate, error) {
	entries, err := os.ReadDir(s.threadsDir())
	if os.IsNotExist(err) {
		return []durableThreadDirCandidate{}, nil
	}
	if err != nil {
		return nil, err
	}
	candidates := make([]durableThreadDirCandidate, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, durableThreadDirCandidate{
			ID:      entry.Name(),
			ModTime: info.ModTime(),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ModTime.Equal(candidates[j].ModTime) {
			return candidates[i].ID > candidates[j].ID
		}
		return candidates[i].ModTime.After(candidates[j].ModTime)
	})
	return candidates, nil
}

func (s *DurableEventSessionStore) GetThread(threadID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getThreadViewNoLock(threadID)
}

// ReadPublicThreadSnapshotV1 serializes the raw view, trusted projection and
// replay cursor with final persistence and publication-index activation.
// The callback must not reenter this store; response enrichment runs outside.
func (s *DurableEventSessionStore) ReadPublicThreadSnapshotV1(threadID string, project func(map[string]any) (map[string]any, error)) (map[string]any, int, error) {
	if project == nil {
		return nil, 0, errors.New("public thread snapshot projector is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, err := s.getThreadViewNoLock(threadID)
	if err != nil || thread == nil {
		return thread, 0, err
	}
	projected, err := project(thread)
	if err != nil {
		return nil, 0, err
	}
	seq, err := s.highestSeqNoLock(threadID)
	if err != nil {
		return nil, 0, err
	}
	return projected, seq, nil
}

func (s *DurableEventSessionStore) getThreadViewNoLock(threadID string) (map[string]any, error) {
	thread, err := s.readThreadNoLock(threadID)
	if err != nil || thread == nil {
		return thread, err
	}
	if strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
		return nil, errors.New("durable thread route and body identity mismatch")
	}
	return s.caseThreadView(threadID, thread)
}

func (s *DurableEventSessionStore) mergeListedThreadIDs(threadIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.readMetaNoLock()
	if err != nil {
		return err
	}
	changed := false
	for _, id := range threadIDs {
		if containsString(meta.ListedThreadIDs, id) {
			continue
		}
		meta.ListedThreadIDs = append(meta.ListedThreadIDs, id)
		changed = true
	}
	if !changed {
		return nil
	}
	return s.writeMetaNoLock(meta)
}

func (s *DurableEventSessionStore) CreateThread(request map[string]any, fallbackWorkspace string) (map[string]any, error) {
	return s.createThread(context.Background(), request, fallbackWorkspace, nil)
}

func (s *DurableEventSessionStore) createThread(ctx context.Context, request map[string]any, fallbackWorkspace string, reservation *childThreadReservationV1) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("thread creation context is unavailable")
	}
	meta, err := s.readMetaNoLock()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	threadID, err := s.consumeChildThreadIdentityNoLock(&meta, reservation, false)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	thread := threadapp.BuildThread(threadapp.CreateInput{
		Request:           request,
		ThreadID:          threadID,
		FallbackWorkspace: fallbackWorkspace,
		Now:               now,
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !containsString(meta.ListedThreadIDs, stringField(thread, "id")) {
		meta.ListedThreadIDs = append(meta.ListedThreadIDs, stringField(thread, "id"))
		sort.Strings(meta.ListedThreadIDs)
	}
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, err
	}
	if err := s.writeMetaNoLock(meta); err != nil {
		return nil, err
	}
	return thread, nil
}

func (s *DurableEventSessionStore) PatchThread(threadID string, patch map[string]any) (map[string]any, error) {
	return s.PatchThreadIfBaseline(threadID, patch, "")
}

func (s *DurableEventSessionStore) PatchThreadIfBaseline(threadID string, patch map[string]any, expectedDigest string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return nil, err
	}
	thread, err := s.readThreadNoLock(threadID)
	if err == nil && thread == nil {
		err = os.ErrNotExist
	}
	if err == nil {
		thread, err = threadapp.ApplyPatchMutation(thread, threadID, patch, expectedDigest, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if err == nil {
		err = s.upsertThreadNoLock(thread, false)
	}
	return thread, err
}

func (s *DurableEventSessionStore) CommitWorkspaceMutation(request threadapp.WorkspaceMutationCommitRequest) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRestartWritableNoLockV1(request.Prepared.ThreadID); err != nil {
		return nil, err
	}
	thread, err := s.readThreadNoLock(request.Prepared.ThreadID)
	if err == nil && thread == nil {
		err = os.ErrNotExist
	}
	if err == nil {
		thread, err = threadapp.ApplyWorkspaceMutationCommit(thread, request)
	}
	if err == nil {
		err = s.upsertThreadNoLock(thread, false)
	}
	return thread, err
}

func (s *DurableEventSessionStore) GetThreadForAuthorityRepair(threadID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readThreadNoLock(threadID)
}

// ReplaceThreadForAuthorityRepair is used only during exclusive startup,
// before HTTP admission, after an installation-signed context repair.
func (s *DurableEventSessionStore) ReplaceThreadForAuthorityRepair(threadID string, thread map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return err
	}
	return s.upsertThreadNoLock(thread, false)
}

func (s *DurableEventSessionStore) DeleteThread(threadID string) (bool, error) {
	return s.DeleteThreadIfBaseline(threadID, "")
}

func (s *DurableEventSessionStore) DeleteThreadIfBaseline(threadID string, expectedDigest string) (bool, error) {
	s.mu.Lock()
	tombstoned := false
	defer func() {
		retire := s.retireProcessThread
		s.mu.Unlock()
		// Revoke even when metadata persistence fails after the Primary write.
		// Never take the process owner lock under the Primary reader's lock.
		if tombstoned && retire != nil {
			retire(threadID)
		}
	}()
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return false, err
	}
	thread, err := s.readThreadNoLock(threadID)
	if err == nil && thread == nil {
		err = os.ErrNotExist
	}
	if err != nil {
		return false, err
	}
	meta, err := s.readMetaNoLock()
	if err != nil {
		return false, err
	}
	thread, meta.ListedThreadIDs, err = threadapp.ApplyDeleteMutation(
		thread, threadID, expectedDigest, time.Now().UTC().Format(time.RFC3339Nano), meta.ListedThreadIDs,
	)
	if err != nil {
		return false, err
	}
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		// Primary can already be committed when a later sidecar/index step
		// fails. Observe only the real Primary, preserve the original error,
		// and retire its private memory if the tombstone is present.
		if primary, readErr := s.ReadPrimaryThreadSnapshotV1(context.Background(), threadID); readErr == nil && stringField(primary.Thread, "id") == threadID && stringField(primary.Thread, "status") == "deleted" {
			tombstoned = true
		}
		return false, err
	}
	tombstoned = true
	return true, s.writeMetaNoLock(meta)
}

func (s *DurableEventSessionStore) ForkThread(threadID string, request map[string]any) (map[string]any, error) {
	return s.forkThread(context.Background(), threadID, request, nil)
}

func (s *DurableEventSessionStore) ForkThreadWithProjectionV1(threadID string, request map[string]any, projector threadapp.PublicProjector) (map[string]any, error) {
	return s.forkThreadProjectedV1(context.Background(), threadID, request, nil, projector)
}

func (s *DurableEventSessionStore) forkThread(ctx context.Context, threadID string, request map[string]any, reservation *childThreadReservationV1) (map[string]any, error) {
	return s.forkThreadProjectedV1(ctx, threadID, request, reservation, nil)
}

func (s *DurableEventSessionStore) forkThreadProjectedV1(ctx context.Context, threadID string, request map[string]any, reservation *childThreadReservationV1, projector threadapp.PublicProjector) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRestartWritableNoLockV1(threadID); err != nil {
		return nil, err
	}
	if s.caseThreads != nil && s.caseThreads.RestartPreservesThreadV1(threadID) {
		return nil, casethreadapp.ErrRestartPreserved
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("thread fork context is unavailable")
	}
	source, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, os.ErrNotExist
	}
	deriveCaseAuthority := s.caseThreads != nil && s.caseThreads.IsCaseThread(threadID)
	var sourceSnapshot recoveryport.PrimaryThreadSnapshotV1
	if deriveCaseAuthority {
		sourceSnapshot, err = s.activeHistorySourceNoLockV1(ctx, threadID)
		if err != nil {
			return nil, err
		}
		source = sourceSnapshot.Thread
	}
	source, err = s.caseThreadView(threadID, source)
	if err != nil {
		return nil, err
	}
	boundary, err := threadapp.ParseForkBoundaryV1(request)
	if err != nil {
		return nil, err
	}
	questionHistory := map[string]map[string]any{}
	if !deriveCaseAuthority {
		questionHistory, err = s.forkQuestionHistoryEntriesNoLockV1(source)
		if err != nil {
			return nil, err
		}
	}
	if boundary.TurnID != "" {
		if projector == nil {
			projector = threadapp.NewTrustedPublicProjector(nil)
		}
		// Only committed records count as a captured E. Pending retries are not
		// accepted history and must never seed a fork.
		loaded, loadErr := s.loadEventsSinceFileNoLock(threadID, 0)
		if loadErr != nil || len(loaded.Diagnostics) != 0 {
			return nil, threadapp.ErrForkBoundaryUnavailable
		}
		inherited := map[string]bool{}
		if deriveCaseAuthority {
			reader, ok := s.caseThreads.(casethreadapp.ActiveInheritedHistoryReaderV1)
			if !ok {
				return nil, threadapp.ErrForkBoundaryUnavailable
			}
			inherited, err = threadapp.ValidateActiveInheritedHistoryV1(ctx, source, reader, s)
			if err != nil {
				return nil, err
			}
		}
		source, boundary.EndSeq, err = threadapp.BuildForkEventPrefixV1(threadapp.ForkEventPrefixInputV1{Source: source, Events: loaded.Events, Boundary: boundary, Projector: projector, InheritedCaseTurnIDs: inherited, QuestionHistoryEntries: questionHistory})
		if err != nil {
			return nil, err
		}
	}
	meta, err := s.readMetaNoLock()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	forkID, err := s.consumeChildThreadIdentityNoLock(&meta, reservation, true)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	parentThreadID := strings.TrimSpace(stringField(request, "parentThreadId"))
	if parentThreadID == "" {
		parentThreadID = threadID
	}
	forkInput := threadapp.ForkInput{
		Source:         source,
		ForkID:         forkID,
		ParentThreadID: parentThreadID,
		Now:            now,
		Relation:       stringField(request, "relation"),
		Title:          stringField(request, "title"),
		TurnID:         stringField(request, "turnId"),
	}
	if deriveCaseAuthority {
		forkInput = threadapp.AuthorizeCaseForkV1(forkInput)
	}
	fork, err := threadapp.BuildFork(forkInput)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !deriveCaseAuthority {
		if err := threadapp.NormalizeForkQuestionHistoryV1(fork, questionHistory); err != nil {
			return nil, err
		}
	}
	created, err := threadapp.BuildForkCreatedEventsV1(fork, threadID, boundary.TurnID, boundary.EndSeq)
	if err != nil {
		return nil, err
	}
	for index, event := range created {
		created[index], err = turnapp.SanitizeCaseEventPublication(fork, event)
		if err != nil {
			return nil, err
		}
		if _, err := eventlog.EncodeEventJSONLFrameV1(created[index]); err != nil {
			return nil, err
		}
	}

	// Reserve the existing identity counter before either immutable history
	// record. A crash cannot reassign an orphaned creation seed to a new fork.
	if err := s.writeMetaNoLock(meta); err != nil {
		return nil, err
	}
	if deriveCaseAuthority {
		if err := s.bindActiveHistoryNoLockV1(ctx, sourceSnapshot, fork, "fork"); err != nil {
			return nil, err
		}
	}
	// Prepare a marked primary outside the canonical inventory, then rename
	// the complete directory. No canonical event-only or empty directory is
	// exposed by a cut before the primary-file rename.
	fork, err = s.stageForkCreationPrimaryNoLockV1(fork, created)
	if err != nil {
		return nil, err
	}
	_, frontier, err := s.loadEventsSinceFileWithFrontierNoLock(forkID, 0)
	if err != nil {
		return nil, err
	}
	if s.beforePersistEventHook != nil {
		s.beforePersistEventHook()
	}
	appendErr := error(nil)
	if s.forkCreationAtomicAppend != nil {
		appendErr = s.forkCreationAtomicAppend(forkID, created, frontier)
	} else {
		appendErr = s.eventLog.AppendEventsAtomicAtFrontier(forkID, created, frontier)
	}
	if appendErr != nil && !eventlog.AtomicAppendCommitted(appendErr) {
		return nil, appendErr
	}
	// Even an error after commit must prove the same complete bundle; never
	// append it twice or allocate a replacement identity on this path.
	if err := s.verifyForkCreationPrimaryNoLockV1(fork); err != nil {
		return nil, errors.Join(appendErr, err)
	}
	s.highestSeqByThread[forkID] = len(created)
	delete(fork, forkCreationPendingFieldV1)
	if err := s.upsertThreadNoLock(fork, false); err != nil {
		return nil, err
	}
	if !containsString(meta.ListedThreadIDs, forkID) {
		meta.ListedThreadIDs = append(meta.ListedThreadIDs, forkID)
		sort.Strings(meta.ListedThreadIDs)
	}
	if err := s.writeMetaNoLock(meta); err != nil {
		return nil, err
	}
	// Publish only the ordinary closed creation event, after full commit.
	s.publishEventNoLock(forkID, created[0])

	return fork, nil
}

func (s *DurableEventSessionStore) ResumeSession(sessionID string, request map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRestartWritableNoLockV1(sessionID); err != nil {
		return nil, err
	}
	if s.caseThreads != nil && s.caseThreads.RestartPreservesThreadV1(sessionID) {
		return nil, casethreadapp.ErrRestartPreserved
	}
	source, err := s.readThreadNoLock(sessionID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, os.ErrNotExist
	}
	deriveCaseAuthority := s.caseThreads != nil && s.caseThreads.IsCaseThread(sessionID)
	ctx := context.Background()
	var sourceSnapshot recoveryport.PrimaryThreadSnapshotV1
	if deriveCaseAuthority {
		sourceSnapshot, err = s.activeHistorySourceNoLockV1(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		source = sourceSnapshot.Thread
	}
	source, err = s.caseThreadView(sessionID, source)
	if err != nil {
		return nil, err
	}
	meta, err := s.readMetaNoLock()
	if err != nil {
		return nil, err
	}
	if meta.ResumeCounter < 0 || s.childResumeCounterFloor < 0 {
		return nil, errors.New("resume thread counter is invalid")
	}
	meta.ResumeCounter = max(meta.ResumeCounter, s.childResumeCounterFloor)
	if meta.ResumeCounter == int(^uint(0)>>1) {
		return nil, errors.New("resume thread counter is exhausted")
	}
	meta.ResumeCounter++
	s.childResumeCounterFloor = meta.ResumeCounter
	now := time.Now().UTC().Format(time.RFC3339Nano)
	threadID := fmt.Sprintf("thr_durable_resume_%d", meta.ResumeCounter)
	resumeInput := threadapp.ResumeInput{
		Source:    source,
		ThreadID:  threadID,
		SessionID: sessionID,
		Now:       now,
		Workspace: stringField(request, "workspace"),
		Model:     stringField(request, "model"),
		Mode:      stringField(request, "mode"),
	}
	if deriveCaseAuthority {
		resumeInput = threadapp.AuthorizeCaseResumeV1(resumeInput)
	}
	resumed, err := threadapp.BuildResume(resumeInput)
	if err != nil {
		return nil, err
	}
	if deriveCaseAuthority {
		if err := s.writeMetaNoLock(meta); err != nil {
			return nil, err
		}
		if err := s.bindActiveHistoryNoLockV1(ctx, sourceSnapshot, resumed, "resume"); err != nil {
			return nil, err
		}
	}
	if !containsString(meta.ListedThreadIDs, threadID) {
		meta.ListedThreadIDs = append(meta.ListedThreadIDs, threadID)
		sort.Strings(meta.ListedThreadIDs)
	}
	if err := s.upsertThreadNoLock(resumed, false); err != nil {
		return nil, err
	}
	if err := s.writeMetaNoLock(meta); err != nil {
		return nil, err
	}
	return map[string]any{
		"thread_id":     threadID,
		"session_id":    sessionID,
		"message_count": float64(countThreadItems(resumed)),
		"summary":       stringField(resumed, "title"),
	}, nil
}

const forkCreationPendingFieldV1 = "forkCreationPendingV1"

// Only the existing trusted steering read or a complete source creation
// bundle can supply the inert Question codec slot. No body shape is authority.
func (s *DurableEventSessionStore) forkQuestionHistoryEntriesNoLockV1(source map[string]any) (map[string]map[string]any, error) {
	// The trusted Question reader requires structured steering. Legacy strings
	// are inert history and still pass ordinary history validation later. Remove
	// only those strings from a verification copy; retain every turn and all
	// other entries so last-turn and conflicting-identity checks stay intact.
	verificationSource := cloneMap(source)
	for _, raw := range listAny(verificationSource["turns"]) {
		turn, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entries, ok := turn["steering"].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(entries))
		for _, entry := range entries {
			if _, legacy := entry.(string); !legacy {
				filtered = append(filtered, entry)
			}
		}
		turn["steering"] = filtered
	}
	verified := map[string]map[string]any{}
	needsSeed := false
	for _, raw := range listAny(source["turns"]) {
		turn, _ := raw.(map[string]any)
		for _, rawEntry := range listAny(turn["steering"]) {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}
			if entry["kind"] == "user_message" && entry["delivery"] == "steer" {
				needsSeed = true
			}
			if entry["userInputReplyIdentity"] == nil && entry["userInputId"] == nil && entry["userInputCaptureReceiptId"] == nil {
				continue
			}
			delivery, err := s.userInputLateReplyFromThreadNoLock(verificationSource, stringField(entry, "userInputId"), stringField(entry, "clientUserMessageId"))
			if err != nil || delivery == nil || delivery.TurnID != stringField(turn, "id") || delivery.Entry["id"] != entry["id"] {
				return nil, errors.Join(threadapp.ErrForkBoundaryUnavailable, err)
			}
			verified[stringField(entry, "id")] = delivery.Entry
		}
	}
	if !needsSeed {
		return verified, nil
	}
	loaded, err := s.loadEventsSinceFileNoLock(stringField(source, "id"), 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return nil, errors.Join(threadapp.ErrForkBoundaryUnavailable, err)
	}
	seed, _, err := threadapp.ReadForkCreatedSeedV1(source, loaded.Events)
	if err != nil || seed == nil {
		return nil, errors.Join(threadapp.ErrForkBoundaryUnavailable, err)
	}
	for _, raw := range listAny(seed["turns"]) {
		turn, _ := raw.(map[string]any)
		for _, rawEntry := range listAny(turn["steering"]) {
			entry, ok := rawEntry.(map[string]any)
			if !ok || entry["kind"] != "user_message" || entry["delivery"] != "steer" {
				continue
			}
			id := stringField(entry, "id")
			if verified[id] != nil {
				return nil, threadapp.ErrForkBoundaryUnavailable
			}
			verified[id] = entry
		}
	}
	return verified, nil
}

var errForkCreationPendingV1 = errors.New("fork creation bundle is pending")

func forkCreationEventsDigestV1(events []map[string]any) (string, error) {
	body, err := json.Marshal(events)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(body), nil
}

func (s *DurableEventSessionStore) stageForkCreationPrimaryNoLockV1(fork map[string]any, events []map[string]any) (map[string]any, error) {
	id := stringField(fork, "id")
	pending, err := threadapp.NormalizeForRead(id, fork, stringField(fork, "createdAt"))
	if err != nil {
		return nil, err
	}
	if err := threadapp.ValidatePublicHistory(pending); err != nil {
		return nil, err
	}
	if _, _, err := threadapp.MissingMessageSidecarItems(pending, map[string]string{}); err != nil {
		return nil, err
	}
	digest, err := forkCreationEventsDigestV1(events)
	if err != nil {
		return nil, err
	}
	pending[forkCreationPendingFieldV1] = map[string]any{"schemaVersion": float64(1), "threadId": id, "parentThreadId": stringField(pending, "forkedFromThreadId"), "createdAt": stringField(pending, "createdAt"), "endSeq": float64(len(events)), "eventBundleSHA256": digest}
	body, err := json.Marshal(pending)
	if err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(s.root, ".fork-create-"+id+"-")
	if err != nil {
		return nil, err
	}
	// Exact task-owned staging paths only. The private writer cleans its temp.
	defer func() { _ = os.Remove(filepath.Join(stage, "thread.json")); _ = os.Remove(stage) }()
	if err := filestore.WritePrivateFileAtomic(filepath.Join(stage, "thread.json"), body); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(s.threadDir(id)); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(err, errors.New("fork target identity is occupied"))
	}
	if err := os.Rename(stage, s.threadDir(id)); err != nil {
		return nil, err
	}
	for _, dir := range []string{s.threadsDir(), s.root} {
		folder, err := os.Open(dir)
		if err != nil {
			return nil, err
		}
		if err := errors.Join(folder.Sync(), folder.Close()); err != nil {
			return nil, err
		}
	}
	return pending, nil
}

func (s *DurableEventSessionStore) verifyForkCreationPrimaryNoLockV1(thread map[string]any) error {
	raw, present := thread[forkCreationPendingFieldV1]
	if !present {
		return nil
	}
	marker, ok := raw.(map[string]any)
	version, vok := numericSeq(marker["schemaVersion"])
	end, eok := numericSeq(marker["endSeq"])
	if !ok || len(marker) != 6 || !vok || version != 1 || !eok || end <= 0 || stringField(marker, "threadId") != stringField(thread, "id") || stringField(marker, "parentThreadId") != stringField(thread, "forkedFromThreadId") || stringField(marker, "createdAt") != stringField(thread, "createdAt") || len(stringField(marker, "eventBundleSHA256")) != 64 {
		return errors.New("fork creation marker is malformed")
	}
	loaded, err := s.loadEventsSinceFileNoLock(stringField(thread, "id"), 0)
	pending := errors.Join(errForkCreationPendingV1, threadapp.ErrPublicProjectionPending, threadapp.ErrForkBoundaryUnavailable, os.ErrNotExist)
	if err != nil || len(loaded.Diagnostics) != 0 || len(loaded.Events) < end {
		return pending
	}
	digest, err := forkCreationEventsDigestV1(loaded.Events[:end])
	if err != nil || digest != stringField(marker, "eventBundleSHA256") {
		return pending
	}
	seed, closed, err := threadapp.ReadForkCreatedSeedV1(thread, loaded.Events[:end])
	if err != nil {
		return pending
	}
	if seed == nil {
		if end != 1 || stringField(loaded.Events[0], "kind") != "thread_created" || stringField(loaded.Events[0], "threadId") != stringField(thread, "id") || stringField(loaded.Events[0], "timestamp") != stringField(thread, "createdAt") {
			return pending
		}
	} else if closed != end {
		return pending
	}
	return nil
}
