package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	acceptedfinaleventadapter "analytix.local/runtime-go/internal/adapters/outbound/acceptedfinalevent"
	checkpointcaptureadapter "analytix.local/runtime-go/internal/adapters/outbound/checkpointcapture"
	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadsummaryindexfs "analytix.local/runtime-go/internal/adapters/outbound/threadsummaryindexfs"
	usageindexfs "analytix.local/runtime-go/internal/adapters/outbound/usageindexfs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	acceptedfinaleventport "analytix.local/runtime-go/internal/ports/acceptedfinalevent"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

type DurableJSONLDiagnostic = eventlog.JSONLDiagnostic

type DurableLoadEventsResult = eventlog.LoadResult

type durableMeta struct {
	ListedThreadIDs []string `json:"listedThreadIds"`
	ThreadCounter   int      `json:"threadCounter"`
	ForkCounter     int      `json:"forkCounter"`
	ResumeCounter   int      `json:"resumeCounter"`
}

type CompactThreadResult = threaddomain.CompactionResult

type durableStoreMode string

const (
	durableStoreModeTemp          durableStoreMode = "temp-durable-event-session-store"
	durableStoreModeProduction    durableStoreMode = "production-durable-event-session-store"
	durableStoreModeSemanticStage durableStoreMode = "semantic-startup-stage-event-session-store"
)

var errDurableTurnNotFound = threadapp.ErrTurnNotFound
var errDurableThreadRunning = threadapp.ErrThreadRunning
var errDurableTurnInactive = domainsteering.ErrTurnInactive
var errDurableTurnNotLatest = domainsteering.ErrTurnNotLatest
var errDurableTurnNotSteerable = errors.New("turn is not steerable")
var errDurableExpectedTurnMismatch = errors.New("expected turn id does not match active turn")
var errDurableContextDigestMismatch = domainsteering.ErrContextMismatch
var errDurableSteeringProjectionInvalid = domainsteering.ErrProjectionInvalid
var errDurableSteeringReplayMismatch = domainsteering.ErrReplayMismatch
var errDurableSteeringIDConflict = domainsteering.ErrIdentityConflict
var errDurableSteeringAuthorityUnavailable = errors.New("steering admission authority is unavailable")

type DurableEventSessionStore struct {
	primaryReader recoveryport.PrimaryThreadReaderV1
	*checkpointapp.CapturedEventReconciler
	root                         string
	restartPreserved             *eventlog.SemanticRestartPreservationV1
	mode                         durableStoreMode
	eventLog                     *eventlog.Store
	mu                           sync.Mutex
	subscribers                  map[string]map[chan map[string]any]struct{}
	highestSeqByThread           map[string]int
	pendingEvents                map[string]map[int]map[string]any
	writeAttempts                int
	publishOrders                [][]string
	caseThreads                  casethreadapp.Authority
	activeHistorySourceAdmission func(map[string]any) error
	pendingCaseCompactions       map[string]threadapp.PreparedCompaction
	steeringAuthority            *steeringauthorityapp.Service
	eventReplayReadCount         int
	usageIndex                   *usageindexfs.Store
	threadSummaryIndex           *threadsummaryindexfs.Store
	eventPersistFailures         int
	lastEventPersistError        string
	beforePersistEventHook       func()
	beforeRecordEventHook        func(map[string]any) error
	beforeUsageIndexThreadHook   func(string)
	caseCompactionCommitHook     func(string, string)
	generalTerminalAtomicAppend  func(string, []map[string]any) error
	beforeTerminalWrite          func() error
	acceptedFinalEvents          *acceptedfinaleventadapter.Store
	childThreadCounterFloor      int
	childForkCounterFloor        int
	childResumeCounterFloor      int
}

func (s *DurableEventSessionStore) SetCaseThreadAuthority(authority casethreadapp.Authority) {
	s.mu.Lock()
	s.caseThreads = authority
	s.mu.Unlock()
}

func (s *DurableEventSessionStore) requireRestartWritableNoLockV1(threadID string) error {
	// This constructor-bound guard is independent of the case registry lock:
	// signed repair may already hold that registry's recovery read lease.
	if s.restartPreserved.OwnsThread(strings.TrimSpace(threadID)) {
		return casethreadapp.ErrRestartPreserved
	}
	return nil
}

func (s *DurableEventSessionStore) SetSteeringAuthority(authority *steeringauthorityapp.Service) {
	s.mu.Lock()
	s.steeringAuthority = authority
	s.mu.Unlock()
}

func (s *DurableEventSessionStore) caseThreadView(threadID string, thread map[string]any) (map[string]any, error) {
	return threadapp.CaseThreadAuthorityView(s.caseThreads, threadID, thread)
}

func NewTempDurableEventSessionStore(root string, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newDurableEventSessionStore(root, durableStoreModeTemp, false, floors...)
}

func NewProductionDurableEventSessionStore(root string, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newDurableEventSessionStore(root, durableStoreModeProduction, false, floors...)
}

func newSemanticStartupDurableEventSessionStore(root string, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newDurableEventSessionStore(root, durableStoreModeSemanticStage, true, floors...)
}

// NewRuntimeEventSessionStoreWithPreservationV1 binds the original restart
// scope before constructor imports or any live durable owner is exposed.
func NewRuntimeEventSessionStoreWithPreservationV1(config RuntimeServerConfig, simulation bool, preserved *eventlog.SemanticRestartPreservationV1, usagePreserved *usageindexfs.RestartPreservationV1, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	root, mode := runtimeDurableStoreLocationV1(config)
	if simulation {
		mode = durableStoreModeSemanticStage
	}
	return newDurableEventSessionStoreWithOriginalUsagePreservationV1(root, mode, simulation, preserved, usagePreserved, floors...)
}

func newDurableEventSessionStore(root string, mode durableStoreMode, applySemanticStartupMigrations bool, floorInputs ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newDurableEventSessionStoreWithPreservationV1(root, mode, applySemanticStartupMigrations, nil, floorInputs...)
}

func newDurableEventSessionStoreWithPreservationV1(root string, mode durableStoreMode, applySemanticStartupMigrations bool, preserved *eventlog.SemanticRestartPreservationV1, floorInputs ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	var usagePreserved *usageindexfs.RestartPreservationV1
	if preserved != nil {
		var err error
		usagePreserved, err = usageindexfs.PrepareRestartPreservationV1(context.Background(), root, preserved.ThreadIDsV1())
		if err != nil {
			return nil, err
		}
	}
	return newDurableEventSessionStoreWithOriginalUsagePreservationV1(root, mode, applySemanticStartupMigrations, preserved, usagePreserved, floorInputs...)
}

func newDurableEventSessionStoreWithOriginalUsagePreservationV1(root string, mode durableStoreMode, applySemanticStartupMigrations bool, preserved *eventlog.SemanticRestartPreservationV1, usagePreserved *usageindexfs.RestartPreservationV1, floorInputs ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	heldIDs, usageIDs := preserved.ThreadIDsV1(), usagePreserved.ThreadIDsV1()
	if len(heldIDs) != len(usageIDs) {
		return nil, errors.New("durable usage preservation scope is incomplete")
	}
	for index, id := range heldIDs {
		if id != usageIDs[index] {
			return nil, errors.New("durable usage preservation scope changed")
		}
	}
	if err := usagePreserved.Revalidate(context.Background(), root); err != nil {
		return nil, err
	}
	floors, err := domainpendingwork.MergeChildIdentityFloorsV1(floorInputs...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("durable root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if mode == durableStoreModeTemp && !isInsideTempDir(absRoot) {
		return nil, fmt.Errorf("durable temp dir must be under os.TempDir(): %s", absRoot)
	}
	if err := validateDurableStoreRoot(absRoot, mode); err != nil {
		return nil, err
	}
	if err := preserved.Revalidate(context.Background(), absRoot); err != nil {
		return nil, err
	}
	events, err := eventlog.NewStoreWithPreservationV1(absRoot, preserved)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(absRoot, "threads"), 0o700); err != nil {
		return nil, err
	}
	store := &DurableEventSessionStore{
		restartPreserved:        preserved,
		childThreadCounterFloor: floors.ThreadSequence,
		childForkCounterFloor:   floors.ForkSequence,
		childResumeCounterFloor: floors.ResumeSequence,
		root:                    absRoot,
		mode:                    mode,
		eventLog:                events,
		subscribers:             map[string]map[chan map[string]any]struct{}{},
		highestSeqByThread:      map[string]int{},
		pendingEvents:           map[string]map[int]map[string]any{},
		pendingCaseCompactions:  map[string]threadapp.PreparedCompaction{},
	}
	store.usageIndex, err = usageindexfs.New(usageindexfs.Dependencies{
		RestartPreservation:           usagePreserved,
		Root:                          absRoot,
		Owner:                         &store.mu,
		ReadThread:                    store.readThreadNoLock,
		LoadEvents:                    store.eventLog.LoadSince,
		PendingUsageEventsOwnerLocked: store.pendingUsageEventsOwnerLocked,
		BeforeBuildThread: func(threadID string) {
			if hook := store.beforeUsageIndexThreadHook; hook != nil {
				hook(threadID)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	store.threadSummaryIndex, err = threadsummaryindexfs.New(threadsummaryindexfs.Dependencies{
		Root: absRoot, Owner: &store.mu, ReadThread: store.readThreadNoLock,
		RestartPreservation: preserved.SummaryRecordsV1(),
	})
	if err != nil {
		return nil, err
	}
	store.acceptedFinalEvents, err = acceptedfinaleventadapter.NewStore(acceptedfinaleventadapter.Dependencies{
		Owner: &store.mu, EventLog: store.eventLog,
		ReadThreadOwnerLocked: store.readThreadNoLock, NormalizeThreadOwnerLocked: store.caseThreadView,
		NextSeqOwnerLocked:      store.nextSeqNoLock,
		AppendEventsOwnerLocked: store.persistRecordedEventsUncheckedNoLock,
		SetHighestOwnerLocked:   func(threadID string, seq int) { store.highestSeqByThread[threadID] = seq },
		ValidThreadID:           func(threadID string) bool { return safeDurableID(threadID) == threadID },
		PublishBatchOwnerLocked: store.publishAcceptedFinalBatchNoLock,
	})
	if err != nil {
		return nil, err
	}
	store.CapturedEventReconciler = checkpointapp.NewCapturedEventReconciler(checkpointcaptureadapter.NewStore(&store.mu, store.eventLog, store.highestSeqByThread, store.readThreadNoLock, store.caseThreadView, turnapp.SanitizeGenericCaseEventPublication, store.LoadEventsSinceNoLock, store.nextSeqNoLock, store.persistRecordedEventsNoLock, store.publishEventNoLock))
	if applySemanticStartupMigrations {
		if err := store.importLegacyRuntimeGoThreadDirsNoLock(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *DurableEventSessionStore) AcceptedFinalEventDelivery() acceptedfinaleventport.Delivery {
	return s.acceptedFinalEvents
}

// ApplySemanticStartupMigrationsAfterAuthorityRepair must run only inside the
// isolated semantic startup stage and only after signed committed case
// contexts have repaired their public mirrors. Running execution-authority
// migration before that repair can mistake a repairable root gate for foreign
// derived authority and erase the non-executable stale-handle tombstone.
func (s *DurableEventSessionStore) ApplySemanticStartupMigrationsAfterAuthorityRepair() error {
	if s == nil || s.mode != durableStoreModeSemanticStage {
		return errors.New("post-authority semantic migrations require the semantic startup stage")
	}
	return eventlog.MigrateSemanticStartupContent(eventlog.SemanticStartupContentMigrationInput{
		Root:                   s.root,
		ThreadSummaryIndexPath: s.threadSummaryIndex.Path(),
		RestartPreservation:    s.restartPreserved,
	})
}

func isInsideTempDir(path string) bool {
	return filestore.IsInsideTempDir(path)
}
