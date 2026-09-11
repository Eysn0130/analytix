package sse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

type ThreadEventStore struct {
	HighestSeq                    func(threadID string) (int, error)
	LoadEventsSince               func(threadID string, afterSeq int) ([]map[string]any, error)
	LoadPublicEventsSince         func(threadID string, afterSeq int) ([]map[string]any, error)
	LoadEventsSinceContext        func(context.Context, string, int) ([]map[string]any, error)
	LoadPublicEventsSinceContext  func(context.Context, string, int) ([]map[string]any, error)
	SubscribeEvents               func(threadID string) (<-chan map[string]any, func())
	SealAcceptedFinalDelivery     func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error)
	ValidateAcceptedFinalDelivery func(domainevent.AcceptedFinalDeliveryBatchV2) error
	PreflightPublic               func(threadID string) string
	ProjectPublic                 func(threadID string, event map[string]any) (map[string]any, bool, string)
}

type ThreadEventsHandler struct {
	Store             ThreadEventStore
	HeartbeatInterval time.Duration
	Now               func() time.Time
	SanitizeError     func(error) string
}

const maxLiveReplayEvents = 1024

const (
	casePublicAuthorityRejectedCode    = "case_public_authority_rejected"
	casePublicAuthorityUnavailableCode = "case_public_authority_unavailable"
)

func (h ThreadEventsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodGet {
		httpapi.MethodNotAllowed(w)
		return
	}
	sinceSeq := SinceSeq(r)
	if WantsLive(r) {
		h.serveLive(w, r, threadID, sinceSeq)
		return
	}
	events, setupErr, revocationCode := h.replay(r.Context(), threadID, sinceSeq)
	if revocationCode != "" {
		h.writeProjectionRevokedResponse(w, threadID, revocationCode)
		return
	}
	if setupErr != nil {
		httpapi.WriteDurableSSE(w, http.StatusOK, []map[string]any{h.withSSETrace(h.setupErrorEvent(threadID, sinceSeq, setupErr.phase, setupErr.err))})
		return
	}
	httpapi.WriteDurableSSE(w, http.StatusOK, h.withSSETraceEvents(events))
}

func SinceSeq(r *http.Request) int {
	sinceSeq := httpapi.IntQuery(r.URL.Query(), "since_seq")
	if sinceSeq == 0 {
		if headerSeq, err := strconv.Atoi(r.Header.Get("Last-Event-ID")); err == nil && headerSeq > 0 {
			sinceSeq = headerSeq
		}
	}
	return sinceSeq
}

func WantsLive(r *http.Request) bool {
	return r.URL.Query().Get("live") == "1" || strings.EqualFold(r.URL.Query().Get("live"), "true")
}

type setupError struct {
	phase string
	err   error
}

func (h ThreadEventsHandler) replay(ctx context.Context, threadID string, sinceSeq int) ([]map[string]any, *setupError, string) {
	if code := h.preflightPublic(threadID); code != "" {
		return nil, nil, code
	}
	if h.Store.HighestSeq == nil {
		return nil, &setupError{phase: "highest_seq", err: errors.New("highest seq store is not configured")}, ""
	}
	highestSeq, err := h.Store.HighestSeq(threadID)
	if err != nil {
		return nil, &setupError{phase: "highest_seq", err: err}, ""
	}
	if sinceSeq >= highestSeq {
		return []map[string]any{}, nil, ""
	}
	loadEventsWithContext := h.Store.LoadPublicEventsSinceContext
	if loadEventsWithContext == nil {
		loadEventsWithContext = h.Store.LoadEventsSinceContext
	}
	loadEvents := h.Store.LoadPublicEventsSince
	if loadEvents == nil {
		loadEvents = h.Store.LoadEventsSince
	}
	if loadEventsWithContext == nil && loadEvents == nil {
		return nil, &setupError{phase: "replay", err: errors.New("event replay store is not configured")}, ""
	}
	// A terminal group contains at most four accepted-final records or three
	// ordinary general records. Read enough history to reconstruct a group when
	// the requested cursor lands inside it, then apply the exact cursor locally.
	loadAfterSeq := max(0, sinceSeq-3)
	var events []map[string]any
	if loadEventsWithContext != nil {
		events, err = loadEventsWithContext(ctx, threadID, loadAfterSeq)
	} else {
		events, err = loadEvents(threadID, loadAfterSeq)
	}
	if err != nil {
		return nil, &setupError{phase: "replay", err: err}, ""
	}
	events, err = domainevent.AtomicTerminalReplayEventsAfter(events, sinceSeq)
	if err != nil {
		return nil, nil, casePublicAuthorityRejectedCode
	}
	deliveryUnits, err := acceptedFinalDeliveryUnits(threadID, events, h.Store.SealAcceptedFinalDelivery)
	if err != nil {
		return nil, nil, casePublicAuthorityRejectedCode
	}
	public, revocationCode := h.publicEvents(threadID, deliveryUnits)
	if revocationCode != "" {
		return nil, nil, revocationCode
	}
	return public, nil, ""
}

func (h ThreadEventsHandler) serveLive(w http.ResponseWriter, r *http.Request, threadID string, sinceSeq int) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpapi.WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "streaming_unsupported", "message": "response writer does not support flushing"})
		return
	}
	if h.Store.SubscribeEvents == nil {
		httpapi.WriteDurableSSE(w, http.StatusOK, []map[string]any{h.setupErrorEvent(threadID, sinceSeq, "live", errors.New("live event store is not configured"))})
		return
	}
	events, unsubscribe := h.Store.SubscribeEvents(threadID)
	defer unsubscribe()
	replayEvents, setupErr, revocationCode := h.replay(r.Context(), threadID, sinceSeq)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if revocationCode != "" {
		h.writeProjectionRevokedEvent(w, threadID, revocationCode)
		flusher.Flush()
		return
	}
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()
	if setupErr != nil {
		httpapi.WriteDurableSSEEvent(w, h.withSSETrace(h.setupErrorEvent(threadID, sinceSeq, setupErr.phase, setupErr.err)))
		flusher.Flush()
		return
	}

	lastSeq := sinceSeq
	if len(replayEvents) > maxLiveReplayEvents {
		highestReplaySeq := lastSeq
		if seq, ok := NumericSeq(replayEvents[len(replayEvents)-1]["seq"]); ok && seq > highestReplaySeq {
			highestReplaySeq = seq
		}
		httpapi.WriteDurableSSEEvent(w, h.withSSETrace(SnapshotRequiredEvent(SnapshotRequiredInput{
			ThreadID:         threadID,
			Seq:              highestReplaySeq,
			SinceSeq:         sinceSeq,
			HighestSeq:       highestReplaySeq,
			ReplayEventCount: len(replayEvents),
			Timestamp:        h.now().UTC().Format(time.RFC3339Nano),
		})))
		lastSeq = highestReplaySeq
		flusher.Flush()
	} else {
		for _, event := range replayEvents {
			seq, _ := NumericSeq(event["seq"])
			if seq <= lastSeq {
				continue
			}
			httpapi.WriteDurableSSEEvent(w, h.withSSETrace(event))
			lastSeq = seq
			flusher.Flush()
		}
	}
	flusher.Flush()

	heartbeatInterval := h.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 15 * time.Second
	}
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	var pendingGeneralTerminal []map[string]any
	for {
		select {
		case event, ok := <-events:
			if !ok {
				if len(pendingGeneralTerminal) != 0 {
					h.writeProjectionRevokedEvent(w, threadID, casePublicAuthorityRejectedCode)
					flusher.Flush()
				}
				return
			}
			seq, _ := NumericSeq(event["seq"])
			if seq <= lastSeq {
				continue
			}
			markedGeneralTerminal := domainevent.GeneralTerminalDeliveryMarkerPresentV1(event)
			if len(pendingGeneralTerminal) != 0 || markedGeneralTerminal {
				if !markedGeneralTerminal {
					h.writeProjectionRevokedEvent(w, threadID, casePublicAuthorityRejectedCode)
					flusher.Flush()
					return
				}
				if len(pendingGeneralTerminal) == 0 {
					if _, valid := domainevent.GeneralTerminalDeliveryExpectedEventCountV1(event); !valid {
						h.writeProjectionRevokedEvent(w, threadID, casePublicAuthorityRejectedCode)
						flusher.Flush()
						return
					}
				} else if contracts.StringField(event, "generalTerminalCommitId") !=
					contracts.StringField(pendingGeneralTerminal[0], "generalTerminalCommitId") {
					h.writeProjectionRevokedEvent(w, threadID, casePublicAuthorityRejectedCode)
					flusher.Flush()
					return
				}
				pendingGeneralTerminal = append(pendingGeneralTerminal, event)
				expected, _ := domainevent.GeneralTerminalDeliveryExpectedEventCountV1(pendingGeneralTerminal[0])
				if len(pendingGeneralTerminal) > expected {
					h.writeProjectionRevokedEvent(w, threadID, casePublicAuthorityRejectedCode)
					flusher.Flush()
					return
				}
				if len(pendingGeneralTerminal) < expected {
					continue
				}
				public, revocationCode := h.publicGeneralTerminalBatch(threadID, pendingGeneralTerminal)
				if revocationCode != "" {
					h.writeProjectionRevokedEvent(w, threadID, revocationCode)
					flusher.Flush()
					return
				}
				batchSeq, _ := NumericSeq(public["seq"])
				httpapi.WriteDurableSSEEvent(w, h.withSSETrace(public))
				lastSeq = batchSeq
				pendingGeneralTerminal = nil
				flusher.Flush()
				continue
			}
			public, visible, revocationCode := h.publicEvent(threadID, event)
			if revocationCode != "" {
				h.writeProjectionRevokedEvent(w, threadID, revocationCode)
				flusher.Flush()
				return
			}
			lastSeq = seq
			if !visible {
				httpapi.WriteDurableSSEEvent(w, h.withSSETrace(h.cursorAdvancedEvent(threadID, seq)))
				flusher.Flush()
				continue
			}
			httpapi.WriteDurableSSEEvent(w, h.withSSETrace(public))
			flusher.Flush()
		case <-heartbeat.C:
			catchUpSinceSeq := lastSeq
			catchUp, setupErr, revocationCode := h.replay(r.Context(), threadID, catchUpSinceSeq)
			if revocationCode != "" {
				h.writeProjectionRevokedEvent(w, threadID, revocationCode)
				flusher.Flush()
				return
			}
			if setupErr != nil {
				httpapi.WriteDurableSSEEvent(w, h.withSSETrace(h.setupErrorEvent(
					threadID, catchUpSinceSeq, setupErr.phase, setupErr.err,
				)))
				flusher.Flush()
				return
			}
			if len(catchUp) > maxLiveReplayEvents {
				highestReplaySeq := catchUpSinceSeq
				if seq, ok := NumericSeq(catchUp[len(catchUp)-1]["seq"]); ok && seq > highestReplaySeq {
					highestReplaySeq = seq
				}
				httpapi.WriteDurableSSEEvent(w, h.withSSETrace(SnapshotRequiredEvent(SnapshotRequiredInput{
					ThreadID: threadID, Seq: highestReplaySeq, SinceSeq: catchUpSinceSeq,
					HighestSeq: highestReplaySeq, ReplayEventCount: len(catchUp),
					Timestamp: h.now().UTC().Format(time.RFC3339Nano),
				})))
				lastSeq = highestReplaySeq
				pendingGeneralTerminal = nil
				flusher.Flush()
				continue
			}
			recovered := false
			for _, event := range catchUp {
				seq, _ := NumericSeq(event["seq"])
				if seq <= lastSeq {
					continue
				}
				httpapi.WriteDurableSSEEvent(w, h.withSSETrace(event))
				lastSeq = seq
				recovered = true
			}
			if recovered {
				// Durable replay is the authority after a missed or partial live
				// notification. Any queued raw suffix is skipped by lastSeq.
				pendingGeneralTerminal = nil
				flusher.Flush()
				continue
			}
			httpapi.WriteDurableSSEEvent(w, h.withSSETrace(HeartbeatEvent(threadID, lastSeq, h.now().UTC().Format(time.RFC3339Nano))))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (h ThreadEventsHandler) publicEvents(threadID string, events []map[string]any) ([]map[string]any, string) {
	out := make([]map[string]any, 0, len(events))
	for start := 0; start < len(events); {
		event := events[start]
		if domainevent.GeneralTerminalDeliveryMarkerPresentV1(event) {
			commitID := strings.TrimSpace(contracts.StringField(event, "generalTerminalCommitId"))
			end := start + 1
			for end < len(events) && strings.TrimSpace(contracts.StringField(events[end], "generalTerminalCommitId")) == commitID {
				end++
			}
			batch, revocationCode := h.publicGeneralTerminalBatch(threadID, events[start:end])
			if revocationCode != "" {
				return nil, revocationCode
			}
			out = append(out, batch)
			start = end
			continue
		}
		public, visible, revocationCode := h.publicEvent(threadID, event)
		if revocationCode != "" {
			return nil, revocationCode
		}
		if visible {
			out = append(out, public)
		} else if seq, ok := NumericSeq(event["seq"]); ok {
			out = append(out, h.cursorAdvancedEvent(threadID, seq))
		}
		start++
	}
	return out, ""
}

func (h ThreadEventsHandler) cursorAdvancedEvent(threadID string, seq int) map[string]any {
	return map[string]any{
		"kind":      "cursor_advanced",
		"threadId":  threadID,
		"seq":       float64(seq),
		"reason":    "restricted_content_removed",
		"timestamp": h.now().UTC().Format(time.RFC3339Nano),
	}
}

func (h ThreadEventsHandler) publicEvent(threadID string, event map[string]any) (map[string]any, bool, string) {
	if contracts.StringField(event, "kind") == domainevent.AcceptedFinalDeliveryBatchKind {
		return h.publicAcceptedFinalBatch(threadID, event)
	}
	// Accepted-final publication fields are never authoritative as a standalone
	// live event. Durable replay groups them before this point, and the only live
	// producer publishes the already sealed batch. Rejecting here keeps a shape-
	// valid V3 item from using ProjectPublic (or an identity test double) as a
	// substitute for the delivery seal.
	if domainevent.ContainsAcceptedFinalPublicationAuthority(event) {
		return nil, false, casePublicAuthorityRejectedCode
	}
	public, visible := publicEvent(event)
	if !visible {
		return nil, false, ""
	}
	if h.Store.ProjectPublic == nil {
		return nil, false, casePublicAuthorityUnavailableCode
	}
	projected, visible, code := h.Store.ProjectPublic(threadID, public)
	return projected, visible, normalizeProjectionRevocationCode(code)
}

func (h ThreadEventsHandler) publicGeneralTerminalBatch(threadID string, rawEvents []map[string]any) (map[string]any, string) {
	if domainevent.ValidateGeneralTerminalDeliveryEventsV1(rawEvents) != nil || h.Store.ProjectPublic == nil {
		return nil, casePublicAuthorityRejectedCode
	}
	projectedEvents := make([]map[string]any, 0, len(rawEvents))
	for _, raw := range rawEvents {
		public, visible := publicEvent(raw)
		if !visible || public == nil {
			return nil, casePublicAuthorityRejectedCode
		}
		projected, projectedVisible, code := h.Store.ProjectPublic(threadID, public)
		if normalized := normalizeProjectionRevocationCode(code); normalized != "" {
			return nil, normalized
		}
		if !projectedVisible || projected == nil {
			return nil, casePublicAuthorityRejectedCode
		}
		projectedEvents = append(projectedEvents, projected)
	}
	batch, err := domainevent.NewGeneralTerminalDeliveryBatchV1(rawEvents, projectedEvents)
	if err != nil {
		return nil, casePublicAuthorityRejectedCode
	}
	return domainevent.GeneralTerminalDeliveryBatchV1Map(batch), ""
}

func (h ThreadEventsHandler) publicAcceptedFinalBatch(threadID string, event map[string]any) (map[string]any, bool, string) {
	batch, err := domainevent.ParseAcceptedFinalDeliveryBatchV2(event)
	if err != nil || batch.ThreadID != threadID || h.Store.ValidateAcceptedFinalDelivery == nil ||
		h.Store.ValidateAcceptedFinalDelivery(batch) != nil {
		return nil, false, casePublicAuthorityRejectedCode
	}
	projectedEvents := make([]map[string]any, 0, len(batch.Events))
	for _, nested := range batch.Events {
		public, visible := publicEvent(nested)
		if !visible || public == nil || h.Store.ProjectPublic == nil {
			return nil, false, casePublicAuthorityRejectedCode
		}
		projected, visible, code := h.Store.ProjectPublic(threadID, public)
		code = normalizeProjectionRevocationCode(code)
		if code != "" {
			return nil, false, code
		}
		if !visible || projected == nil {
			return nil, false, casePublicAuthorityRejectedCode
		}
		projectedEvents = append(projectedEvents, projected)
	}
	projectedBatch, err := domainevent.NewAcceptedFinalDeliveryBatchV2(projectedEvents, batch.PublicationAuthority)
	if err != nil || projectedBatch.BatchID != batch.BatchID || projectedBatch.EventManifestDigest != batch.EventManifestDigest {
		return nil, false, casePublicAuthorityRejectedCode
	}
	return domainevent.AcceptedFinalDeliveryBatchV2Map(projectedBatch), true, ""
}

func acceptedFinalDeliveryUnits(
	threadID string,
	events []map[string]any,
	seal func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error),
) ([]map[string]any, error) {
	groups, err := preflightAcceptedFinalDeliveryGroupsV1(threadID, events)
	if err != nil {
		return nil, err
	}
	units := make([]map[string]any, 0, len(events))
	groupIndex := 0
	for start := 0; start < len(events); {
		if groupIndex >= len(groups) || groups[groupIndex].start != start {
			units = append(units, events[start])
			start++
			continue
		}
		group := groups[groupIndex]
		if seal == nil {
			return nil, errors.New("accepted final delivery authority is unavailable")
		}
		deliverySeal, err := seal(events[group.start:group.end])
		if err != nil {
			return nil, err
		}
		batch, err := domainevent.NewAcceptedFinalDeliveryBatchV2(events[group.start:group.end], deliverySeal)
		if err != nil {
			return nil, err
		}
		units = append(units, domainevent.AcceptedFinalDeliveryBatchV2Map(batch))
		start = group.end
		groupIndex++
	}
	return units, nil
}

type acceptedFinalDeliveryGroupV1 struct {
	start int
	end   int
}

func preflightAcceptedFinalDeliveryGroupsV1(
	requestedThreadID string,
	events []map[string]any,
) ([]acceptedFinalDeliveryGroupV1, error) {
	requestedThreadID = strings.TrimSpace(requestedThreadID)
	if requestedThreadID == "" || contracts.SafeRecordID(requestedThreadID) != requestedThreadID {
		return nil, errors.New("accepted final delivery requested thread is invalid")
	}
	groups := make([]acceptedFinalDeliveryGroupV1, 0)
	seenCommits := make(map[string]struct{})
	type acceptedFinalTurnIdentity struct{ threadID, turnID string }
	seenTurns := make(map[acceptedFinalTurnIdentity]struct{})
	previousAcceptedFinalLastSeq := 0
	for start := 0; start < len(events); {
		commitID := strings.TrimSpace(contracts.StringField(events[start], "publicationCommitId"))
		if commitID == "" {
			start++
			continue
		}
		if _, duplicate := seenCommits[commitID]; duplicate {
			return nil, errors.New("accepted final delivery commit is duplicated or split")
		}
		seenCommits[commitID] = struct{}{}
		end := start + 1
		for end < len(events) && strings.TrimSpace(contracts.StringField(events[end], "publicationCommitId")) == commitID {
			end++
		}
		threadID := strings.TrimSpace(contracts.StringField(events[start], "threadId"))
		turnID := strings.TrimSpace(contracts.StringField(events[start], "turnId"))
		firstSeq, firstSeqOK := NumericSeq(events[start]["seq"])
		lastSeq, lastSeqOK := NumericSeq(events[end-1]["seq"])
		turnKey := acceptedFinalTurnIdentity{threadID: threadID, turnID: turnID}
		if threadID != requestedThreadID || turnID == "" || !firstSeqOK || !lastSeqOK ||
			firstSeq <= previousAcceptedFinalLastSeq {
			return nil, errors.New("accepted final delivery identity or frontier is invalid")
		}
		if _, duplicate := seenTurns[turnKey]; duplicate {
			return nil, errors.New("accepted final delivery turn is duplicated")
		}
		if domainevent.ValidateAcceptedFinalDeliveryEventsV2(events[start:end]) != nil {
			return nil, errors.New("accepted final delivery event group is invalid")
		}
		seenTurns[turnKey] = struct{}{}
		previousAcceptedFinalLastSeq = lastSeq
		groups = append(groups, acceptedFinalDeliveryGroupV1{start: start, end: end})
		if len(groups) > domainevent.AcceptedFinalDeliveryGroupLimitV1 {
			return nil, errors.New("accepted final delivery count exceeds the public bound")
		}
		start = end
	}
	return groups, nil
}

func (h ThreadEventsHandler) preflightPublic(threadID string) string {
	if h.Store.PreflightPublic == nil || h.Store.ProjectPublic == nil {
		return casePublicAuthorityUnavailableCode
	}
	return normalizeProjectionRevocationCode(h.Store.PreflightPublic(threadID))
}

func normalizeProjectionRevocationCode(code string) string {
	switch strings.TrimSpace(code) {
	case "":
		return ""
	case casePublicAuthorityRejectedCode:
		return casePublicAuthorityRejectedCode
	case casePublicAuthorityUnavailableCode:
		return casePublicAuthorityUnavailableCode
	default:
		return casePublicAuthorityRejectedCode
	}
}

func highestEventSeq(events []map[string]any, fallback int) int {
	highest := fallback
	for _, event := range events {
		if seq, ok := NumericSeq(event["seq"]); ok && seq > highest {
			highest = seq
		}
	}
	return highest
}

func publicEvent(event map[string]any) (map[string]any, bool) {
	if domainevent.ContainsPrivateAcceptedFinalAuthority(event) || domainevent.ValidatePublicRecord(event) != nil {
		return nil, false
	}
	kind, _ := event["kind"].(string)
	switch strings.TrimSpace(kind) {
	case "assistant_reasoning", "assistant_reasoning_delta", "agent_reasoning":
		return nil, false
	}
	out := make(map[string]any, len(event))
	for key, value := range event {
		out[key] = value
	}
	delete(out, "reasoningContent")
	if item, ok := event["item"].(map[string]any); ok {
		itemKind, _ := item["kind"].(string)
		if strings.TrimSpace(itemKind) == "assistant_reasoning" {
			return nil, false
		}
		if strings.TrimSpace(itemKind) == "assistant_text" {
			if _, present := item["acceptedFinal"]; present {
				return nil, false
			}
			if value, present := item["acceptedFinalView"]; present {
				view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(value)
				if err != nil || !acceptedFinalPublicEventIdentityV3(event, item, view) {
					return nil, false
				}
			} else if domainevent.ContainsAcceptedFinalPublicationAuthority(event) {
				return nil, false
			}
		}
		clonedItem := make(map[string]any, len(item))
		for key, value := range item {
			clonedItem[key] = value
		}
		delete(clonedItem, "reasoningContent")
		out["item"] = clonedItem
	}
	return out, true
}

func acceptedFinalPublicEventIdentityV3(
	event map[string]any,
	item map[string]any,
	view domainevidence.AcceptedFinalPublicViewV3,
) bool {
	return contracts.StringField(event, "kind") == "item_completed" &&
		contracts.StringField(event, "threadId") == contracts.StringField(item, "threadId") &&
		contracts.StringField(event, "turnId") == contracts.StringField(item, "turnId") &&
		contracts.StringField(event, "itemId") == contracts.StringField(item, "id") &&
		contracts.StringField(event, "timestamp") == view.AcceptedAt &&
		contracts.StringField(event, "acceptedFinalDigest") == view.AcceptedFinalDigest &&
		contracts.StringField(event, "publicationCommitId") == view.AcceptedFinalDigest &&
		contracts.StringField(event, "publicationSlot") == "assistant-final" &&
		contracts.StringField(item, "role") == "assistant" &&
		contracts.StringField(item, "status") == "completed" &&
		contracts.StringField(item, "finishedAt") == view.AcceptedAt
}

func (h ThreadEventsHandler) withSSETrace(event map[string]any) map[string]any {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("ANALYTIX_THREAD_TRACE")))
	if value != "1" && value != "true" && value != "yes" {
		return event
	}
	// Atomic terminal batches are already sealed over an exact closed shape.
	// Appending trace after the digest/signature step would make a valid host
	// batch fail every downstream strict contract. Their nested timestamp and
	// outer sequence remain the transport observability boundary.
	switch strings.TrimSpace(contracts.StringField(event, "kind")) {
	case domainevent.AcceptedFinalDeliveryBatchKind, domainevent.GeneralTerminalDeliveryBatchKind:
		return event
	}
	out := make(map[string]any, len(event)+1)
	for key, value := range event {
		out[key] = value
	}
	trace := map[string]any{}
	if current, ok := event["trace"].(map[string]any); ok {
		for key, value := range current {
			trace[key] = value
		}
	}
	sentAt := float64(h.now().UTC().UnixNano()) / float64(time.Millisecond)
	trace["sse_sent_at"] = sentAt
	trace["sse_live_emitted_at"] = sentAt
	out["trace"] = trace
	return out
}

func (h ThreadEventsHandler) withSSETraceEvents(events []map[string]any) []map[string]any {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("ANALYTIX_THREAD_TRACE")))
	if value != "1" && value != "true" && value != "yes" {
		return events
	}
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, h.withSSETrace(event))
	}
	return out
}

func (h ThreadEventsHandler) setupErrorEvent(threadID string, seq int, phase string, cause error) map[string]any {
	message := "SSE setup failed"
	if cause != nil && h.SanitizeError != nil {
		if sanitized := strings.TrimSpace(h.SanitizeError(cause)); sanitized != "" {
			message = sanitized
		}
	}
	return SetupErrorEvent(SetupErrorInput{
		ThreadID:  threadID,
		Seq:       seq,
		Phase:     phase,
		Message:   message,
		Timestamp: h.now().UTC().Format(time.RFC3339Nano),
	})
}

func (h ThreadEventsHandler) writeProjectionRevokedResponse(w http.ResponseWriter, threadID, code string) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusOK)
	h.writeProjectionRevokedEvent(w, threadID, code)
}

func (h ThreadEventsHandler) writeProjectionRevokedEvent(w http.ResponseWriter, threadID, code string) {
	event := PublicProjectionRevokedEvent(threadID, code)
	data, _ := json.Marshal(event)
	_, _ = w.Write([]byte("event: public_projection_revoked\n"))
	_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
}

func (h ThreadEventsHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func NumericSeq(value any) (int, bool) {
	return contracts.NumericSeq(value)
}

type SetupErrorInput struct {
	ThreadID  string
	Seq       int
	Phase     string
	Message   string
	Timestamp string
}

func SetupErrorEvent(input SetupErrorInput) map[string]any {
	seq := input.Seq
	if seq < 0 {
		seq = 0
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = "SSE setup failed"
	}
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = "setup"
	}
	return map[string]any{
		"kind":      "error",
		"seq":       float64(seq),
		"timestamp": strings.TrimSpace(input.Timestamp),
		"threadId":  input.ThreadID,
		"code":      "sse_setup_error",
		"message":   message,
		"severity":  "error",
		"terminal":  true,
		"details": map[string]any{
			"phase": phase,
		},
	}
}

type SnapshotRequiredInput struct {
	ThreadID         string
	Seq              int
	SinceSeq         int
	HighestSeq       int
	ReplayEventCount int
	Timestamp        string
}

func PublicProjectionRevokedEvent(threadID, code string) map[string]any {
	code = normalizeProjectionRevocationCode(code)
	if code == "" {
		code = casePublicAuthorityUnavailableCode
	}
	return map[string]any{
		"schemaVersion":    float64(1),
		"kind":             "public_projection_revoked",
		"threadId":         strings.TrimSpace(threadID),
		"historyAuthority": "case_boundary_only_v1",
		"code":             code,
		"action":           "purge_case_projection",
		"terminal":         true,
	}
}

func SnapshotRequiredEvent(input SnapshotRequiredInput) map[string]any {
	seq := input.Seq
	if seq < 0 {
		seq = 0
	}
	highestSeq := input.HighestSeq
	if highestSeq < seq {
		highestSeq = seq
	}
	return map[string]any{
		"kind":             "snapshot_required",
		"threadId":         input.ThreadID,
		"seq":              float64(seq),
		"sinceSeq":         float64(max(0, input.SinceSeq)),
		"highestSeq":       float64(max(0, highestSeq)),
		"replayEventCount": float64(max(0, input.ReplayEventCount)),
		"reason":           "live_replay_backlog_exceeded",
		"timestamp":        strings.TrimSpace(input.Timestamp),
	}
}

func HeartbeatEvent(threadID string, seq int, timestamp string) map[string]any {
	if seq < 0 {
		seq = 0
	}
	return map[string]any{
		"kind":      "heartbeat",
		"threadId":  threadID,
		"seq":       float64(seq),
		"timestamp": strings.TrimSpace(timestamp),
	}
}
