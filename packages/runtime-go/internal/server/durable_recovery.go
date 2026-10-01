package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnrecoveryapp "analytix.local/runtime-go/internal/app/turn"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

func (s *DurableEventSessionStore) RecoveredState(threadID string) (map[string]any, error) {
	result, err := s.LoadEventsSince(threadID, 0)
	if err != nil {
		return nil, err
	}
	return threadapp.BuildRecoveredState(threadapp.RecoveredStateInput{
		ThreadID:    threadID,
		Events:      result.Events,
		Diagnostics: result.Diagnostics,
	}), nil
}

func (s *DurableEventSessionStore) Snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	usageIndexStats := s.usageIndex.StatsOwnerLocked()
	threadSummaryIndexStats := s.threadSummaryIndex.StatsOwnerLocked()
	return map[string]any{
		"enabled":                     true,
		"mode":                        string(s.mode),
		"root":                        s.root,
		"tempDirOnly":                 s.mode == durableStoreModeTemp,
		"candidateDurableRoot":        isCandidateDurableStoreMode(s.mode),
		"writeAttempts":               s.writeAttempts,
		"publishOrders":               cloneValue(s.publishOrders),
		"realWorkspaceWriteAllowed":   false,
		"credentialReadAllowed":       false,
		"providerLiveCallsAllowed":    false,
		"defaultGoBackendEnabled":     false,
		"usageEventsIndex":            filepath.ToSlash(s.usageIndex.Path()),
		"usageIndexBackfills":         float64(usageIndexStats.Backfills),
		"usageIndexReads":             float64(usageIndexStats.Reads),
		"threadSummaryIndex":          filepath.ToSlash(s.threadSummaryIndex.Path()),
		"threadSummaryIndexBackfills": float64(threadSummaryIndexStats.Backfills),
		"threadSummaryIndexReads":     float64(threadSummaryIndexStats.Reads),
		"eventReplayReads":            float64(s.eventReplayReadCount),
		"pendingEventCount":           float64(s.pendingEventCountNoLock()),
		"eventPersistFailures":        float64(s.eventPersistFailures),
		"lastEventPersistError":       s.lastEventPersistError,
	}
}

func (s *DurableEventSessionStore) pendingEventCountNoLock() int {
	count := 0
	for _, events := range s.pendingEvents {
		count += len(events)
	}
	return count
}

func (s *DurableEventSessionStore) WriteAttempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeAttempts
}

func (s *DurableEventSessionStore) EventReplayReadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventReplayReadCount
}

func (s *DurableEventSessionStore) UsageIndexReadCount() int {
	return s.usageIndex.Stats().Reads
}

func (s *DurableEventSessionStore) upsertThreadIfAbsentNoLock(thread map[string]any, listed bool) error {
	id := stringField(thread, "id")
	if err := s.requireRestartWritableNoLockV1(id); err != nil {
		return err
	}
	if id == "" {
		return errors.New("thread id is required")
	}
	existing, err := s.readThreadNoLock(id)
	if err != nil {
		return err
	}
	if existing != nil {
		if listed {
			meta, err := s.readMetaNoLock()
			if err != nil {
				return err
			}
			if !containsString(meta.ListedThreadIDs, id) {
				meta.ListedThreadIDs = append(meta.ListedThreadIDs, id)
				if err := s.writeMetaNoLock(meta); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return s.upsertThreadNoLock(thread, listed)
}

func (s *DurableEventSessionStore) upsertThreadNoLock(thread map[string]any, listed bool) error {
	return s.upsertThreadNoLockWithAtomicWriteAuthority(thread, listed, nil)
}

func (s *DurableEventSessionStore) upsertThreadNoLockWithAtomicWriteAuthority(
	thread map[string]any,
	listed bool,
	authorize func(func() error) error,
) error {
	id := stringField(thread, "id")
	if err := s.requireRestartWritableNoLockV1(id); err != nil {
		return err
	}
	if id == "" {
		return errors.New("thread id is required")
	}
	if pending, found := s.pendingCaseCompactions[id]; found {
		incomingDigest, incomingErr := threadapp.CompactionBaselineDigest(thread)
		targetDigest, targetErr := threadapp.CompactionBaselineDigest(pending.Thread)
		if incomingErr != nil || targetErr != nil || incomingDigest != targetDigest {
			return errors.Join(threadapp.ErrCaseCompactionPendingRecovery, incomingErr, targetErr)
		}
	}
	thread, err := threadapp.NormalizeForRead(id, thread, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if err := threadapp.ValidatePublicHistory(thread); err != nil {
		return err
	}
	if _, ok := thread["turns"]; !ok {
		thread["turns"] = []any{}
	}
	// Message sidecars are untrusted recovery aids. Prove the complete closed
	// public projection before writing the primary thread so an unknown or
	// malformed hydrated item cannot leave a partially committed thread.json.
	if len(threadapp.ThreadItemsInOrder(thread)) > 0 {
		if _, _, err := threadapp.MissingMessageSidecarItems(thread, map[string]string{}); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(s.threadDir(id), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(thread)
	if err != nil {
		return err
	}
	write := func() error { return filestore.WritePrivateFileAtomic(s.threadPath(id), data) }
	if authorize != nil {
		err = authorize(write)
	} else {
		err = write()
	}
	if err != nil {
		return err
	}
	if err := s.appendThreadMetadataSidecarNoLock(thread); err != nil {
		return err
	}
	if err := s.appendMissingThreadMessageSidecarsNoLock(thread); err != nil {
		return err
	}
	if err := s.threadSummaryIndex.AppendOwnerLocked(thread); err != nil {
		return err
	}
	if listed {
		meta, err := s.readMetaNoLock()
		if err != nil {
			return err
		}
		if !containsString(meta.ListedThreadIDs, id) {
			meta.ListedThreadIDs = append(meta.ListedThreadIDs, id)
			sort.Strings(meta.ListedThreadIDs)
			if err := s.writeMetaNoLock(meta); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *DurableEventSessionStore) AllThreadIDs() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.threadIDsFromFilesystemNoLock()
}

type GeneralTerminalPublicationRecoveryPlanV1 = turnrecoveryapp.GeneralTerminalPublicationRecoveryPlanV1
type GeneralTerminalPublicationRecoveryTurnV1 = turnrecoveryapp.GeneralTerminalPublicationRecoveryTurnV1

func (s *DurableEventSessionStore) PreflightGeneralTerminalPublicationRecoveryV1() ([]GeneralTerminalPublicationRecoveryPlanV1, error) {
	return turnrecoveryapp.PreflightGeneralTerminalPublicationRecoveryV1(context.Background(), s)
}

func (s *DurableEventSessionStore) ApplyGeneralTerminalPublicationRecoveryV1(plans []GeneralTerminalPublicationRecoveryPlanV1) error {
	return turnrecoveryapp.ApplyGeneralTerminalPublicationRecoveryV1(context.Background(), s, plans)
}

func (s *DurableEventSessionStore) WithGeneralTerminalRecoveryExclusiveV1(
	ctx context.Context,
	run func(recoveryport.TransactionV1) error,
) error {
	if s == nil || ctx == nil || run == nil {
		return errors.New("general terminal recovery store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return run(generalTerminalRecoveryTransactionV1{store: s})
}

type generalTerminalRecoveryTransactionV1 struct{ store *DurableEventSessionStore }

func (tx generalTerminalRecoveryTransactionV1) RestartPreservesThreadV1(threadID string) bool {
	return tx.store.restartPreserved.OwnsThread(threadID)
}

func (tx generalTerminalRecoveryTransactionV1) RevalidateStartupPreservationV1(ctx context.Context) error {
	return tx.store.restartPreserved.Revalidate(ctx, tx.store.root)
}

func (tx generalTerminalRecoveryTransactionV1) ThreadIDs() ([]string, error) {
	return tx.store.threadIDsFromFilesystemReadOnlyNoLock()
}

func (tx generalTerminalRecoveryTransactionV1) ReadCanonicalThread(threadID string) (map[string]any, error) {
	return tx.store.readThreadNoLock(threadID)
}

func (tx generalTerminalRecoveryTransactionV1) PublicationThreadView(threadID string, thread map[string]any) (map[string]any, error) {
	return tx.store.caseThreadView(threadID, thread)
}

func (tx generalTerminalRecoveryTransactionV1) LoadEvents(threadID string) (recoveryport.ReplaySnapshotV1, error) {
	loaded, frontier, err := tx.store.loadEventsSinceFileWithFrontierNoLock(threadID, 0)
	return recoveryport.ReplaySnapshotV1{
		Events: loaded.Events, Replayable: err == nil && len(loaded.Diagnostics) == 0, EventLogSHA256: frontier.SHA256,
	}, err
}

func (tx generalTerminalRecoveryTransactionV1) ObserveEvents(ctx context.Context, threadID string) (recoveryport.ReplaySnapshotV1, error) {
	loaded, frontier, err := tx.store.eventLog.ObserveSinceWithFrontier(ctx, threadID, 0)
	return recoveryport.ReplaySnapshotV1{
		Events: loaded.Events, Replayable: err == nil && len(loaded.Diagnostics) == 0, EventLogSHA256: frontier.SHA256,
	}, err
}

func (tx generalTerminalRecoveryTransactionV1) RecordTerminalBundle(threadID, turnID string) error {
	_, err := tx.store.recordGeneralTerminalEventBundleNoLock(threadID, turnID)
	return err
}

func (tx generalTerminalRecoveryTransactionV1) SettleTerminalUsage(event map[string]any) error {
	return tx.store.usageIndex.SettleTerminalEventOwnerLocked(event)
}

func (tx generalTerminalRecoveryTransactionV1) SettleTerminalUsageBatch(events []map[string]any) error {
	return tx.store.usageIndex.SettleTerminalEventsOwnerLocked(events)
}

func (s *DurableEventSessionStore) threadIDsFromFilesystemNoLock() ([]string, error) {
	return s.threadIDsFromFilesystemReadOnlyNoLock()
}

func (s *DurableEventSessionStore) threadIDsFromFilesystemReadOnlyNoLock() ([]string, error) {
	entries, err := os.ReadDir(s.threadsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || safeDurableID(entry.Name()) != entry.Name() {
			return nil, errors.New("durable thread inventory contains an unknown entry")
		}
		thread, err := s.readThreadNoLock(entry.Name())
		if err != nil {
			return nil, err
		}
		if thread == nil || strings.TrimSpace(stringField(thread, "id")) != entry.Name() {
			return nil, errors.New("durable thread inventory identity is invalid")
		}
		ids = append(ids, entry.Name())
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *DurableEventSessionStore) importLegacyRuntimeGoThreadDirsNoLock() error {
	if err := s.restartPreserved.Revalidate(context.Background(), s.root); err != nil {
		return err
	}
	legacyThreadsDir := filepath.Join(s.root, "runtime-go", "threads")
	if sameFilesystemPath(legacyThreadsDir, s.threadsDir()) {
		return nil
	}
	entries, err := os.ReadDir(legacyThreadsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := strings.TrimSpace(entry.Name())
		if threadID == "" || strings.ContainsAny(threadID, `/\`) {
			continue
		}
		if s.restartPreserved.OwnsThread(threadID) {
			continue
		}
		sourceDir := filepath.Join(legacyThreadsDir, threadID)
		if err := copyMissingDurableDirectory(sourceDir, s.threadDir(threadID)); err != nil {
			return err
		}
		if s.restartPreserved != nil {
			if err := os.RemoveAll(sourceDir); err != nil {
				return err
			}
		}
	}
	if s.restartPreserved == nil {
		if err := os.RemoveAll(legacyThreadsDir); err != nil {
			return err
		}
	} else if err := os.Remove(legacyThreadsDir); err != nil && !errors.Is(err, os.ErrNotExist) && !isDirectoryNotEmptyError(err) {
		return err
	}
	if err := os.Remove(filepath.Dir(legacyThreadsDir)); err != nil &&
		!errors.Is(err, os.ErrNotExist) &&
		!isDirectoryNotEmptyError(err) {
		return err
	}
	return nil
}

func sameFilesystemPath(left string, right string) bool {
	return filestore.SamePath(left, right)
}

func isDirectoryNotEmptyError(err error) bool {
	return filestore.IsDirectoryNotEmptyError(err)
}

func copyMissingDurableDirectory(sourceDir string, targetDir string) error {
	return filestore.CopyMissingDirectoryLossless(sourceDir, targetDir, shouldReplacePlaceholderDurableFile)
}

func shouldReplacePlaceholderDurableFile(relativePath string, sourcePath string, targetPath string) bool {
	if filepath.Clean(relativePath) != "thread.json" {
		return false
	}
	source, err := filestore.ReadJSONMapFile(sourcePath)
	if err != nil {
		return false
	}
	target, err := filestore.ReadJSONMapFile(targetPath)
	if err != nil {
		return false
	}
	return threadapp.ShouldReplacePlaceholderThread(filepath.Clean(relativePath), source, target)
}

func (s *DurableEventSessionStore) ModeString() string {
	return string(s.mode)
}

func (s *DurableEventSessionStore) readThreadNoLock(threadID string) (map[string]any, error) {
	if !domainthread.IsCanonicalRecordID(threadID) {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(s.threadPath(threadID))
	if errors.Is(err, os.ErrNotExist) {
		return s.readThreadFromSidecarNoLock(threadID)
	}
	if err != nil {
		return nil, err
	}
	if err := domainjsonstrict.Validate(data, domainjsonstrict.Options{RequireObject: true}); err != nil {
		return nil, err
	}
	var thread map[string]any
	if err := json.Unmarshal(data, &thread); err != nil {
		return nil, err
	}
	thread, err = normalizeThreadForRead(threadID, thread)
	if err != nil {
		return nil, err
	}
	if stringField(thread, "id") != threadID {
		return nil, errors.New("durable thread route and body identity mismatch")
	}
	registeredCase := s.caseThreads != nil && s.caseThreads.IsCaseThread(threadID)
	if !registeredCase && threadapp.ThreadNeedsSidecarHydration(thread) {
		items, err := s.readThreadMessageSidecarItemsNoLock(threadID)
		if err != nil {
			return nil, err
		}
		if threadapp.SidecarItemsHaveCaseAuthority(threadID, items) {
			return nil, errors.New("messages sidecar contains forbidden case authority")
		}
		if len(items) > 0 {
			hydrated, err := normalizeThreadForRead(threadID, threadapp.HydrateSidecarItems(threadapp.HydrateSidecarInput{
				ThreadID:     threadID,
				Thread:       thread,
				Items:        items,
				FallbackTime: time.Now().UTC().Format(time.RFC3339Nano),
			}))
			if err != nil {
				return nil, err
			}
			if threadapp.ThreadHasCaseAuthorityMarkers(hydrated) {
				return nil, errors.New("messages sidecar cannot establish case authority")
			}
			thread = hydrated
		}
	}
	return thread, nil
}

func (s *DurableEventSessionStore) readThreadFromSidecarNoLock(threadID string) (map[string]any, error) {
	if s.caseThreads != nil && s.caseThreads.IsCaseThread(threadID) {
		return nil, errors.New("registered case thread primary authority is missing; sidecar recovery is forbidden")
	}
	thread, err := s.readLatestThreadMetadataSidecarNoLock(threadID)
	if err != nil || thread == nil {
		return thread, err
	}
	items, err := s.readThreadMessageSidecarItemsNoLock(threadID)
	if err != nil {
		return nil, err
	}
	if threadapp.SidecarItemsHaveCaseAuthority(threadID, items) {
		return nil, errors.New("messages sidecar contains forbidden case authority")
	}
	recovered, err := normalizeThreadForRead(threadID, threadapp.HydrateSidecarItems(threadapp.HydrateSidecarInput{
		ThreadID:     threadID,
		Thread:       thread,
		Items:        items,
		FallbackTime: time.Now().UTC().Format(time.RFC3339Nano),
	}))
	if err != nil {
		return nil, err
	}
	if threadapp.ThreadHasCaseAuthorityMarkers(recovered) {
		return nil, errors.New("case thread primary authority is missing; sidecar recovery is forbidden")
	}
	return recovered, nil
}

func (s *DurableEventSessionStore) readLatestThreadMetadataSidecarNoLock(threadID string) (map[string]any, error) {
	entries, err := filestore.ReadJSONLFileRecords[map[string]any](s.metadataPath(threadID), nil)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if threadapp.SidecarValueHasPrivateTerminalAuthority(entry) {
			return nil, errors.New("metadata sidecar contains forbidden private terminal authority")
		}
	}
	return threadapp.LatestMetadataSidecarThread(threadID, entries), nil
}

func (s *DurableEventSessionStore) readThreadMessageSidecarItemsNoLock(threadID string) ([]map[string]any, error) {
	rawItems, err := filestore.ReadJSONLFileRecords[map[string]any](s.messagesPath(threadID), nil)
	if errors.Is(err, os.ErrNotExist) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	// Validate the raw, untrusted sidecar before applying the rendering-only
	// item allowlist. Otherwise an assistant item carrying forged accepted-final
	// authority could be dropped as prose before its authority marker is seen.
	if threadapp.SidecarItemsHaveCaseAuthority(threadID, rawItems) {
		return nil, errors.New("messages sidecar contains forbidden case authority")
	}
	for _, item := range rawItems {
		if threadapp.SidecarValueHasPrivateTerminalAuthority(item) {
			return nil, errors.New("messages sidecar contains forbidden private terminal authority")
		}
	}
	return threadapp.LatestMessageSidecarItems(rawItems), nil
}

func normalizeThreadForRead(threadID string, thread map[string]any) (map[string]any, error) {
	return threadapp.NormalizeForRead(threadID, thread, time.Now().UTC().Format(time.RFC3339Nano))
}

func (s *DurableEventSessionStore) appendThreadMetadataSidecarNoLock(thread map[string]any) error {
	if err := s.requireRestartWritableNoLockV1(stringField(thread, "id")); err != nil {
		return err
	}
	entry := threadapp.MetadataSidecarEntry(thread, time.Now().UTC().Format(time.RFC3339Nano))
	if entry == nil {
		return nil
	}
	id := strings.TrimSpace(stringField(thread, "id"))
	if id == "" {
		return nil
	}
	return filestore.AppendJSONLRecord(s.metadataPath(id), entry)
}

func (s *DurableEventSessionStore) appendMissingThreadMessageSidecarsNoLock(thread map[string]any) error {
	if err := s.requireRestartWritableNoLockV1(stringField(thread, "id")); err != nil {
		return err
	}
	id := strings.TrimSpace(stringField(thread, "id"))
	if id == "" {
		return nil
	}
	if len(threadapp.ThreadItemsInOrder(thread)) == 0 {
		return nil
	}
	existing, err := s.messageSidecarJSONByIDNoLock(id)
	if err != nil {
		return err
	}
	toAppend, _, err := threadapp.MissingMessageSidecarItems(thread, existing)
	if err != nil {
		return err
	}
	for _, item := range toAppend {
		if err := filestore.AppendJSONLRecord(s.messagesPath(id), item); err != nil {
			return err
		}
	}
	return nil
}

func (s *DurableEventSessionStore) messageSidecarJSONByIDNoLock(threadID string) (map[string]string, error) {
	items, err := s.readThreadMessageSidecarItemsNoLock(threadID)
	if err != nil {
		return nil, err
	}
	return threadapp.MessageSidecarJSONByID(items)
}

func (s *DurableEventSessionStore) ThreadIsDescendantOf(threadID string, ancestorThreadID string) (bool, error) {
	threadID = strings.TrimSpace(threadID)
	ancestorThreadID = strings.TrimSpace(ancestorThreadID)
	if threadID == "" || ancestorThreadID == "" {
		return false, nil
	}
	if threadID == ancestorThreadID {
		return true, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	seen := map[string]bool{}
	queue := []string{threadID}
	for len(queue) > 0 {
		currentID := strings.TrimSpace(queue[0])
		queue = queue[1:]
		if currentID == "" || seen[currentID] {
			continue
		}
		if currentID == ancestorThreadID {
			return true, nil
		}
		seen[currentID] = true
		thread, err := s.readThreadNoLock(currentID)
		if err != nil {
			return false, err
		}
		if thread == nil {
			continue
		}
		for _, parentID := range []string{
			stringField(thread, "parentThreadId"),
			stringField(thread, "forkedFromThreadId"),
		} {
			parentID = strings.TrimSpace(parentID)
			if parentID == ancestorThreadID {
				return true, nil
			}
			if parentID != "" && !seen[parentID] {
				queue = append(queue, parentID)
			}
		}
	}
	return false, nil
}

func (s *DurableEventSessionStore) readMetaNoLock() (durableMeta, error) {
	data, err := os.ReadFile(s.metaPath())
	if errors.Is(err, os.ErrNotExist) {
		return durableMeta{ListedThreadIDs: []string{}}, nil
	}
	if err != nil {
		return durableMeta{}, err
	}
	var meta durableMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return durableMeta{}, err
	}
	return meta, nil
}

func (s *DurableEventSessionStore) writeMetaNoLock(meta durableMeta) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	sort.Strings(meta.ListedThreadIDs)
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(), data, 0o600)
}

func (s *DurableEventSessionStore) threadsDir() string {
	return filepath.Join(s.root, "threads")
}

func (s *DurableEventSessionStore) threadDir(threadID string) string {
	return s.eventLog.ThreadDir(threadID)
}

func (s *DurableEventSessionStore) eventsPath(threadID string) string {
	return s.eventLog.EventsPath(threadID)
}

func (s *DurableEventSessionStore) threadPath(threadID string) string {
	return filepath.Join(s.threadDir(threadID), "thread.json")
}

func (s *DurableEventSessionStore) metadataPath(threadID string) string {
	return filepath.Join(s.threadDir(threadID), "metadata.jsonl")
}

func (s *DurableEventSessionStore) messagesPath(threadID string) string {
	return filepath.Join(s.threadDir(threadID), "messages.jsonl")
}

func (s *DurableEventSessionStore) metaPath() string {
	return filepath.Join(s.root, "durable-meta.json")
}
