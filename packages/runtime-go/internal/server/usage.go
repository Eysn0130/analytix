package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	appcache "analytix.local/runtime-go/internal/app/cachetelemetry"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type usageSnapshot = usageapp.Snapshot
type usageRecord = usageapp.Record
type usageIndexRecord = usageapp.IndexRecord

func (s *DurableEventSessionStore) EnsureUsageIndex() error {
	return s.usageIndex.Ensure()
}

func (s *DurableEventSessionStore) LoadUsageIndexRecords(threadID string) ([]usageRecord, error) {
	return s.usageIndex.LoadRecords(threadID)
}

func (h *runtimeServerHandler) handleUsage(w http.ResponseWriter, r *http.Request) {
	handlers := h.usageHandlers()
	if strings.TrimSpace(r.URL.Query().Get("group_by")) == "thread" && h.providerUsage != nil {
		handlers.Records = func(threadID string) ([]usageRecord, error) {
			return h.conversationUsageRecordsV1(r.Context(), threadID)
		}
	}
	handlers.Handle(w, r)
}

func (h *runtimeServerHandler) usageHandlers() httpapi.UsageHandlers {
	service := h.runtimeUsageService()
	return httpapi.UsageHandlers{
		Records:         service.Records,
		RuntimeResponse: service.RuntimeResponse,
	}
}

func (h *runtimeServerHandler) threadUsageSnapshot(threadID string) (map[string]any, error) {
	snapshot, err := h.runtimeUsageService().ThreadSnapshot(threadID)
	if err != nil {
		return nil, err
	}
	return snapshot.Map(), nil
}

func (h *runtimeServerHandler) runtimeUsageService() usageapp.RuntimeService {
	return usageapp.RuntimeService{Repository: runtimeUsageRepository{store: h.store}}
}

type runtimeUsageRepository struct {
	store *DurableEventSessionStore
}

func (r runtimeUsageRepository) ThreadExists(threadID string) (bool, error) {
	if !domainthread.IsCanonicalRecordID(threadID) {
		return false, nil
	}
	thread, err := r.store.GetThread(threadID)
	if err != nil {
		return false, err
	}
	return thread != nil, nil
}

func (r runtimeUsageRepository) LoadRecords(threadID string) ([]usageRecord, error) {
	return r.store.LoadUsageIndexRecords(threadID)
}

func (r runtimeUsageRepository) ListRuntimeThreadIDs() ([]string, error) {
	threads, err := r.store.ListThreads(false, true, false, "")
	if err != nil {
		return nil, err
	}
	threadIDs := make([]string, 0, len(threads))
	for _, thread := range threads {
		if threadID := stringField(thread, "id"); threadID != "" {
			threadIDs = append(threadIDs, threadID)
		}
	}
	return threadIDs, nil
}

// ProviderUsageObserverV1 reads the existing installation-signed provider
// ledger. It is the same authority used by provider execution and closure.
type ProviderUsageObserverV1 interface {
	ObserveTurnInventoriesV1(context.Context, []domainsecurity.TurnSecurityContext) ([]appcache.TurnInventoryV1, error)
}

func (h *runtimeServerHandler) conversationUsageRecordsV1(ctx context.Context, threadID string) ([]usageRecord, error) {
	records, err := h.runtimeUsageService().Records(threadID)
	if err != nil {
		return nil, err
	}
	ids := []string{strings.TrimSpace(threadID)}
	if ids[0] == "" {
		ids, err = (runtimeUsageRepository{store: h.store}).ListRuntimeThreadIDs()
		if err != nil {
			return nil, err
		}
	}
	for _, id := range ids {
		thread, err := h.store.GetThread(id)
		if err != nil {
			return nil, err
		}
		if thread == nil {
			continue
		}
		replay, err := h.store.LoadEventsSinceContext(ctx, id, 0)
		if err != nil {
			return nil, err
		}
		if len(replay.Diagnostics) != 0 {
			return nil, errors.New("conversation usage event inventory is unavailable")
		}
		thread, err = h.conversationUsageHistoryV1(thread, replay.Events)
		if err != nil {
			return nil, err
		}
		contexts, err := conversationUsageContextsV1(thread, replay.Events)
		if err != nil {
			return nil, err
		}
		snapshots, err := h.providerUsage.ObserveTurnInventoriesV1(ctx, contexts)
		if err != nil {
			return nil, errors.New("conversation provider usage inventory is unavailable")
		}
		if len(snapshots) != len(contexts) {
			return nil, errors.New("conversation provider usage inventory is incomplete")
		}
		inventories := make([]usageapp.TurnUsageInventoryV1, 0, len(snapshots))
		for i, snapshot := range snapshots {
			inventories = append(inventories, usageapp.TurnUsageInventoryV1{TurnID: contexts[i].TurnID, Inventory: snapshot})
		}
		projection, err := usageapp.BuildConversationUsageV1(thread, replay.Events, inventories)
		if err != nil {
			return nil, err
		}
		records = append(records, usageRecord{ThreadID: id, Conversation: &projection, ProjectionOnly: true})
	}
	return records, nil
}

func conversationUsageContextsV1(thread map[string]any, events []map[string]any) ([]domainsecurity.TurnSecurityContext, error) {
	threadID := stringField(thread, "id")
	turnIDs := map[string]bool{}
	contexts := []domainsecurity.TurnSecurityContext{}
	seen := map[string]bool{}
	add := func(turn map[string]any) error {
		turnID := stringField(turn, "id")
		raw := turn["securityContext"]
		if raw == nil {
			return nil
		} // Legacy turns remain explicitly unavailable.
		frozen, err := domainsecurity.ParseTurnSecurityContext(raw)
		if err != nil || frozen.ThreadID != threadID || frozen.TurnID != turnID {
			return errors.New("conversation usage scope is invalid")
		}
		if frozen.Version != domainsecurity.TurnSecurityContextVersionV2 {
			return nil
		}
		if !seen[frozen.ContextDigest] {
			seen[frozen.ContextDigest] = true
			contexts = append(contexts, frozen)
		}
		return nil
	}
	for _, value := range listAny(thread["turns"]) {
		turn, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("conversation usage turn is invalid")
		}
		turnIDs[stringField(turn, "id")] = true
		if err := add(turn); err != nil {
			return nil, err
		}
	}
	for _, event := range events {
		if stringField(event, "kind") != "turn_started" || stringField(event, "threadId") != threadID || !turnIDs[stringField(event, "turnId")] {
			continue
		}
		turn, _ := event["turn"].(map[string]any)
		if turn == nil {
			// Protected starts intentionally persist metadata only. Their frozen
			// context is read from the durable turn, never invented from labels.
			if event["securityContext"] == nil {
				continue
			}
			turn = map[string]any{"id": stringField(event, "turnId")}
		}
		if stringField(turn, "id") != stringField(event, "turnId") {
			return nil, errors.New("conversation usage start scope is invalid")
		}
		if raw := event["securityContext"]; raw != nil {
			if embedded := turn["securityContext"]; embedded != nil {
				a, aErr := domainsecurity.ParseTurnSecurityContext(raw)
				b, bErr := domainsecurity.ParseTurnSecurityContext(embedded)
				if aErr != nil || bErr != nil || a.ContextDigest != b.ContextDigest {
					return nil, errors.New("conversation usage start scopes conflict")
				}
			}
			turn = map[string]any{"id": stringField(event, "turnId"), "securityContext": raw}
		}
		if err := add(turn); err != nil {
			return nil, err
		}
	}
	return contexts, nil
}

// The usage read view includes historical turns removed by compaction. It does
// not write a thread, restore content, or add a second persistence mechanism.
func (h *runtimeServerHandler) conversationUsageHistoryV1(thread map[string]any, events []map[string]any) (map[string]any, error) {
	threadID := stringField(thread, "id")
	view := contracts.CloneMap(thread)
	turns := []any{}
	byID := map[string]map[string]any{}
	add := func(turnID string) map[string]any {
		if turn := byID[turnID]; turn != nil {
			return turn
		}
		turn := map[string]any{"id": turnID, "threadId": threadID, "items": []any{}}
		byID[turnID] = turn
		turns = append(turns, turn)
		return turn
	}
	for _, value := range listAny(view["turns"]) {
		turn, ok := value.(map[string]any)
		if !ok || stringField(turn, "id") == "" || byID[stringField(turn, "id")] != nil {
			return nil, errors.New("conversation historical turn inventory is invalid")
		}
		byID[stringField(turn, "id")] = turn
		turns = append(turns, turn)
	}
	for _, event := range events {
		if stringField(event, "threadId") != threadID || stringField(event, "kind") != "turn_started" {
			continue
		}
		turnID := stringField(event, "turnId")
		if turnID == "" {
			return nil, errors.New("conversation historical start identity is invalid")
		}
		turn := add(turnID)
		if turn["securityContext"] == nil {
			if frozen := event["securityContext"]; frozen != nil {
				turn["securityContext"] = frozen
			} else if embedded, _ := event["turn"].(map[string]any); embedded != nil {
				turn["securityContext"] = embedded["securityContext"]
			}
		}
	}
	if registry, ok := h.caseThreads.(interface {
		CommittedContexts() []casethreadapp.CommittedContext
	}); ok {
		for _, committed := range registry.CommittedContexts() {
			frozen := committed.SecurityContext
			if frozen.ThreadID != threadID {
				continue
			}
			turn := add(frozen.TurnID)
			if turn["securityContext"] == nil {
				turn["securityContext"] = frozen
			}
		}
	}
	if raw := thread["generalTerminalPublicationArchive"]; raw != nil {
		archive, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(raw)
		if err != nil {
			return nil, errors.New("conversation historical terminal membership is invalid")
		}
		for _, commit := range archive.Commits {
			if commit.ThreadID != threadID {
				return nil, errors.New("conversation historical terminal membership is foreign")
			}
			add(commit.TurnID)
		}
	}
	view["turns"] = turns
	return view, nil
}
