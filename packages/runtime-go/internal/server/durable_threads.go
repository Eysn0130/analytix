package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	threadapp "analytix.local/runtime-go/internal/app/thread"
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
	defer s.mu.Unlock()
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
		return false, err
	}
	return true, s.writeMetaNoLock(meta)
}

func (s *DurableEventSessionStore) ForkThread(threadID string, request map[string]any) (map[string]any, error) {
	return s.forkThread(context.Background(), threadID, request, nil)
}

func (s *DurableEventSessionStore) forkThread(ctx context.Context, threadID string, request map[string]any, reservation *childThreadReservationV1) (map[string]any, error) {
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
	if deriveCaseAuthority {
		// Reserve the existing counter before the immutable receipt. A crash
		// cannot reassign this target to another source snapshot.
		if err := s.writeMetaNoLock(meta); err != nil {
			return nil, err
		}
		if err := s.bindActiveHistoryNoLockV1(ctx, sourceSnapshot, fork, "fork"); err != nil {
			return nil, err
		}
	}
	if !containsString(meta.ListedThreadIDs, stringField(fork, "id")) {
		meta.ListedThreadIDs = append(meta.ListedThreadIDs, stringField(fork, "id"))
		sort.Strings(meta.ListedThreadIDs)
	}
	if err := s.upsertThreadNoLock(fork, false); err != nil {
		return nil, err
	}
	if err := s.writeMetaNoLock(meta); err != nil {
		return nil, err
	}
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
