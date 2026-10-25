package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	displayapp "analytix.local/runtime-go/internal/app/userinputdisplay"
	"analytix.local/runtime-go/internal/contracts"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

type runtimeUserInputDisplayAccessV1 struct{ handler *runtimeServerHandler }

func (a runtimeUserInputDisplayAccessV1) AcquireCurrent(ctx context.Context, selector port.SelectorV1) (port.AuthorizedV1, func(), error) {
	current, release, err := a.acquireCurrent(ctx, selector, nil)
	if err == nil {
		return current, release, nil
	}
	return a.acquireHistory(ctx, selector)
}

func (a runtimeUserInputDisplayAccessV1) acquireCurrent(ctx context.Context, selector port.SelectorV1, expectedEffectScope *domainsecurity.TurnSecurityContext) (port.AuthorizedV1, func(), error) {
	h := a.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.runtimeControl().ShuttingDown() || h.store == nil || h.turnSecurity.Observer == nil || h.toolSnapshotHistoryValidate == nil || !port.ValidateSelectorV1(selector) {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	first, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, selector.ThreadID)
	if err != nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(first.Thread, "workspace"))
	if err != nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	if expectedEffectScope != nil && (selector.ThreadID != expectedEffectScope.ThreadID || observation.WorkspaceRealPath != expectedEffectScope.WorkspaceRealPath || principal.TenantID != expectedEffectScope.TenantID || principal.UserID != expectedEffectScope.UserID) {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, selector.ThreadID, observation.WorkspaceRealPath, principal.TenantID, principal.UserID)
	if err != nil || release == nil {
		if release != nil {
			release()
		}
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	current, err := a.observeCurrent(ctx, selector, principal)
	if err != nil || current.Workspace != observation.WorkspaceRealPath {
		release()
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	return current, release, nil
}

func (a runtimeUserInputDisplayAccessV1) ValidateCurrent(ctx context.Context, expected port.AuthorizedV1) error {
	if expected.ReadOnly {
		actual, err := a.observeHistory(ctx, expected.Selector, expected.Principal)
		if err != nil || !reflect.DeepEqual(actual, expected) {
			return port.ErrUnavailableV1
		}
		return nil
	}
	actual, err := a.observeCurrent(ctx, expected.Selector, expected.Principal)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return port.ErrUnavailableV1
	}
	return nil
}

func (a runtimeUserInputDisplayAccessV1) observeCurrent(ctx context.Context, selector port.SelectorV1, principal domainidentity.PrincipalV1) (port.AuthorizedV1, error) {
	h := a.handler
	unavailable := port.AuthorizedV1{}
	if ctx == nil || ctx.Err() != nil || h == nil || h.runtimeControl().ShuttingDown() || h.store == nil || h.turnSecurity.Observer == nil || h.toolSnapshotHistoryValidate == nil ||
		!port.ValidateSelectorV1(selector) || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return unavailable, port.ErrUnavailableV1
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, selector.ThreadID)
	if err != nil {
		return unavailable, port.ErrUnavailableV1
	}
	thread := snapshot.Thread
	item, err := exactUserInputDisplayItemV1(thread, selector)
	if err != nil {
		return unavailable, port.ErrUnavailableV1
	}
	frozen, err := turnapp.FrozenSecurityContextForTurn(thread, selector.TurnID)
	if err != nil || frozen.ThreadID != selector.ThreadID || frozen.TurnID != selector.TurnID || frozen.TenantID != principal.TenantID || frozen.UserID != principal.UserID {
		return unavailable, port.ErrUnavailableV1
	}
	current, err := snapshotapp.CurrentOwnContextV1(thread, frozen)
	if err != nil {
		return unavailable, port.ErrUnavailableV1
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(thread, "workspace"))
	if err != nil || observation.WorkspaceRealPath != frozen.WorkspaceRealPath {
		return unavailable, port.ErrUnavailableV1
	}
	originalPath, originalOK := filestore.ResolveMutationIdentityPath(frozen.WorkspaceRealPath, frozen.WorkspaceRealPath)
	observedPath, observedOK := filestore.ResolveMutationIdentityPath(observation.WorkspaceRealPath, observation.WorkspaceRealPath)
	if !originalOK || !observedOK || originalPath != observedPath ||
		turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: frozen.WorkspaceRealPath}) != nil ||
		h.toolSnapshotHistoryValidate(ctx, thread, frozen) != nil {
		return unavailable, port.ErrUnavailableV1
	}
	if ctx.Err() != nil || h.runtimeControl().ShuttingDown() || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return unavailable, port.ErrUnavailableV1
	}
	return port.AuthorizedV1{Selector: selector, Principal: principal, Workspace: frozen.WorkspaceRealPath, Original: frozen, ReceiptID: stringField(item, "continuationReceiptId"), Status: stringField(item, "status")}, nil
}

// Only the strict primary inventory supplies identities. Related aliases in
// another item or turn are conflicts, even when one exact candidate exists.
func exactUserInputDisplayItemV1(thread map[string]any, selector port.SelectorV1) (map[string]any, error) {
	return exactUserInputDisplayItemModeV1(thread, selector, false)
}

func exactUserInputDisplayItemModeV1(thread map[string]any, selector port.SelectorV1, history bool) (map[string]any, error) {
	if !port.ValidateSelectorV1(selector) || stringField(thread, "id") != selector.ThreadID || stringField(thread, "status") == "deleted" {
		return nil, port.ErrUnavailableV1
	}
	turns, ok := thread["turns"].([]any)
	if !ok {
		return nil, port.ErrUnavailableV1
	}
	var selected map[string]any
	seenTurn := false
	for _, rawTurn := range turns {
		turn, ok := rawTurn.(map[string]any)
		if !ok {
			return nil, port.ErrUnavailableV1
		}
		if stringField(turn, "id") == selector.TurnID {
			if seenTurn {
				return nil, port.ErrUnavailableV1
			}
			seenTurn = true
		}
		items, ok := turn["items"].([]any)
		if !ok {
			return nil, port.ErrUnavailableV1
		}
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				return nil, port.ErrUnavailableV1
			}
			if stringField(item, "id") != selector.ItemID && stringField(item, "inputId") != selector.RequestID {
				continue
			}
			if selected != nil || stringField(turn, "id") != selector.TurnID || stringField(item, "id") != selector.ItemID || stringField(item, "kind") != "user_input" ||
				stringField(item, "threadId") != selector.ThreadID || stringField(item, "turnId") != selector.TurnID || stringField(item, "inputId") != selector.RequestID ||
				(!history && !domainsecurity.IsSHA256Hex(stringField(item, "continuationReceiptId")) || history && stringField(item, "continuationReceiptId") != "") {
				return nil, port.ErrUnavailableV1
			}
			switch stringField(item, "status") {
			case "pending", "submitted", "cancelled", "continued":
			default:
				return nil, port.ErrUnavailableV1
			}
			selected = item
		}
	}
	if selected == nil || !seenTurn {
		return nil, port.ErrUnavailableV1
	}
	if history {
		if stringField(selected, "status") == "pending" {
			return nil, port.ErrUnavailableV1
		}
		return selected, nil
	}
	receiptID := stringField(selected, "continuationReceiptId")
	count := 0
	for _, rawTurn := range turns {
		turn := rawTurn.(map[string]any)
		for _, rawItem := range turn["items"].([]any) {
			item := rawItem.(map[string]any)
			if stringField(item, "continuationReceiptId") == receiptID {
				count++
			}
		}
	}
	if count != 1 {
		return nil, port.ErrUnavailableV1
	}
	return selected, nil
}

// History uses one original-source scope lease. Nested workspace read leases
// can deadlock with a queued authority writer; lineage is observed and then
// revalidated under that one lease instead.
func (a runtimeUserInputDisplayAccessV1) acquireHistory(ctx context.Context, selector port.SelectorV1) (port.AuthorizedV1, func(), error) {
	if a.handler == nil || a.handler.continuations == nil || ctx == nil || ctx.Err() != nil || !port.ValidateSelectorV1(selector) {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	origin, err := a.handler.continuations.ResolveUserInputLocalDisplayOriginHost(ctx, selector.RequestID)
	if err != nil || origin.ThreadID == selector.ThreadID {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	originalSelector := port.SelectorV1{ThreadID: origin.ThreadID, TurnID: origin.TurnID, ItemID: origin.ItemID, RequestID: origin.InputID}
	source, release, err := a.acquireCurrent(ctx, originalSelector, nil)
	if err != nil || release == nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	history, err := a.observeHistory(ctx, selector, source.Principal)
	if err != nil || history.ReceiptID != source.ReceiptID || !reflect.DeepEqual(history.Original, source.Original) {
		release()
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	return history, release, nil
}

func (a runtimeUserInputDisplayAccessV1) observeHistory(ctx context.Context, selector port.SelectorV1, principal domainidentity.PrincipalV1) (port.AuthorizedV1, error) {
	unavailable := func() (port.AuthorizedV1, error) { return port.AuthorizedV1{}, port.ErrUnavailableV1 }
	h := a.handler
	if h == nil || h.store == nil || h.store.eventLog == nil || h.continuations == nil || h.publicProjector == nil || ctx == nil || ctx.Err() != nil || !port.ValidateSelectorV1(selector) {
		return unavailable()
	}
	origin, err := h.continuations.ResolveUserInputLocalDisplayOriginHost(ctx, selector.RequestID)
	if err != nil || origin.ThreadID == selector.ThreadID || origin.TurnID != selector.TurnID || origin.ItemID != selector.ItemID {
		return unavailable()
	}
	originalSelector := port.SelectorV1{ThreadID: origin.ThreadID, TurnID: origin.TurnID, ItemID: origin.ItemID, RequestID: origin.InputID}
	source, err := a.observeCurrent(ctx, originalSelector, principal)
	if err != nil || source.ReceiptID != origin.ReceiptID || !reflect.DeepEqual(source.Original, origin.SecurityContext) {
		return unavailable()
	}
	request, err := h.continuations.ReadUserInputHistoryHost(ctx, origin)
	args := map[string]any{}
	if err != nil || json.Unmarshal(request.Arguments, &args) != nil {
		return unavailable()
	}
	args, err = toolcatalogapp.UserInputArgumentsForTool(request.ToolName, args)
	if err != nil {
		return unavailable()
	}
	prompt := privacyprojectionapp.ProjectOrdinaryText(firstNonEmptyString(stringField(args, "prompt"), stringField(args, "question"), "User input required"))
	questions := privacyprojectionapp.ProjectHostUserInputQuestions(selector.RequestID, toolcatalogapp.UserInputQuestions(args, selector.RequestID, prompt))
	questionsJSON, err := json.Marshal(questions)
	if err != nil || len(questions) == 0 {
		return unavailable()
	}
	visited := map[string]bool{}
	proofs := []map[string]any{}
	selectedStatus := ""
	for id := selector.ThreadID; id != origin.ThreadID; {
		if visited[id] || !port.ValidateSelectorV1(port.SelectorV1{ThreadID: id, TurnID: selector.TurnID, ItemID: selector.ItemID, RequestID: selector.RequestID}) {
			return unavailable()
		}
		visited[id] = true
		primary, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil || !domainsecurity.IsSHA256Hex(primary.ThreadFileSHA256) {
			return unavailable()
		}
		observation, err := h.turnSecurity.Observer.Observe(stringField(primary.Thread, "workspace"))
		observedPath, observedOK := filestore.ResolveMutationIdentityPath(observation.WorkspaceRealPath, observation.WorkspaceRealPath)
		sourcePath, sourceOK := filestore.ResolveMutationIdentityPath(source.Workspace, source.Workspace)
		if err != nil || observation.WorkspaceRealPath != source.Workspace || !observedOK || !sourceOK || observedPath != sourcePath {
			return unavailable()
		}
		loaded, frontier, err := h.store.eventLog.ObserveSinceWithFrontier(ctx, id, 0)
		if err != nil || len(loaded.Diagnostics) != 0 || !frontier.Exists || !domainsecurity.IsSHA256Hex(frontier.SHA256) {
			return unavailable()
		}
		seed, closed, err := threadapp.ReadForkCreatedSeedV1(primary.Thread, loaded.Events)
		if err != nil || seed == nil || closed <= 0 || validateQuestionHistoryCreationMarkerV1(primary.Thread, loaded.Events, closed) != nil {
			return unavailable()
		}
		copy := contracts.CloneMap(primary.Thread)
		delete(copy, forkCreationPendingFieldV1)
		public, err := h.publicProjector.ProjectThread(copy)
		if err != nil {
			return unavailable()
		}
		layerSelector := port.SelectorV1{ThreadID: id, TurnID: selector.TurnID, ItemID: selector.ItemID, RequestID: selector.RequestID}
		currentItem, err := exactUserInputDisplayItemModeV1(public, layerSelector, true)
		if err != nil {
			return unavailable()
		}
		seedItem, err := exactUserInputDisplayItemModeV1(map[string]any{"id": id, "turns": seed["turns"]}, layerSelector, true)
		if err != nil {
			return unavailable()
		}
		currentJSON, err := json.Marshal(currentItem)
		seedJSON, seedErr := json.Marshal(seedItem)
		seedQuestionsJSON, questionsErr := json.Marshal(seedItem["questions"])
		if err != nil || seedErr != nil || questionsErr != nil || string(currentJSON) != string(seedJSON) || string(seedQuestionsJSON) != string(questionsJSON) {
			return unavailable()
		}
		if selectedStatus == "" {
			selectedStatus = stringField(currentItem, "status")
		}
		proofs = append(proofs, map[string]any{"threadId": id, "primary": primary.ThreadFileSHA256, "events": frontier})
		id = stringField(seed, "sourceThreadId")
	}
	encoded, err := json.Marshal(proofs)
	if err != nil || len(proofs) == 0 || ctx.Err() != nil {
		return unavailable()
	}
	digest := sha256.Sum256(encoded)
	source.Selector, source.Origin, source.Status, source.ReadOnly = selector, originalSelector, selectedStatus, true
	source.HistoryDigest = hex.EncodeToString(digest[:])
	return source, nil
}

// Observe-only equivalent of the existing creation marker barrier. It never
// recovers a journal, clears a durable marker or publishes staged state.
func validateQuestionHistoryCreationMarkerV1(thread map[string]any, events []map[string]any, closed int) error {
	raw, present := thread[forkCreationPendingFieldV1]
	if !present {
		return nil
	}
	marker, ok := raw.(map[string]any)
	version, versionOK := numericSeq(marker["schemaVersion"])
	end, endOK := numericSeq(marker["endSeq"])
	if !ok || len(marker) != 6 || !versionOK || version != 1 || !endOK || end != closed || end > len(events) ||
		stringField(marker, "threadId") != stringField(thread, "id") || stringField(marker, "parentThreadId") != stringField(thread, "forkedFromThreadId") ||
		stringField(marker, "createdAt") != stringField(thread, "createdAt") {
		return port.ErrUnavailableV1
	}
	digest, err := forkCreationEventsDigestV1(events[:end])
	if err != nil || digest != stringField(marker, "eventBundleSHA256") {
		return port.ErrUnavailableV1
	}
	return nil
}

func (reader runtimeUserInputDisplayReaderV1) ReadUserInputHistoryHost(ctx context.Context, binding continuationapp.UserInputLocalDisplayBindingV1, selector port.SelectorV1) (displayapp.HistoryRecordV1, error) {
	unavailable := func() (displayapp.HistoryRecordV1, error) { return displayapp.HistoryRecordV1{}, port.ErrUnavailableV1 }
	h := reader.handler
	if h == nil || h.continuations == nil || ctx == nil || ctx.Err() != nil {
		return unavailable()
	}
	origin, err := h.continuations.ResolveUserInputLocalDisplayOriginHost(ctx, selector.RequestID)
	if err != nil || !reflect.DeepEqual(origin, binding) {
		return unavailable()
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return unavailable()
	}
	authorized, err := (runtimeUserInputDisplayAccessV1{h}).observeHistory(ctx, selector, principal)
	if err != nil || authorized.ReceiptID != binding.ReceiptID || !reflect.DeepEqual(authorized.Original, binding.SecurityContext) {
		return unavailable()
	}
	record, err := h.continuations.ReadUserInputHistoryHost(ctx, binding)
	if err != nil {
		return unavailable()
	}
	record.ThreadID, record.TurnID, record.ItemID, record.Status = selector.ThreadID, selector.TurnID, selector.ItemID, authorized.Status
	if record.Status == "continued" || record.Status == "submitted" {
		record.AnswerState = "unavailable"
	}
	result := displayapp.HistoryRecordV1{Record: record}
	if record.Status != "continued" {
		return result, nil
	}
	source, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil {
		return unavailable()
	}
	type layerV1 struct {
		id            string
		primary, seed map[string]any
	}
	layers := []layerV1{}
	visited := map[string]bool{}
	for id := selector.ThreadID; id != binding.ThreadID; {
		if visited[id] {
			return unavailable()
		}
		visited[id] = true
		primary, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return unavailable()
		}
		loaded, _, err := h.store.eventLog.ObserveSinceWithFrontier(ctx, id, 0)
		if err != nil || len(loaded.Diagnostics) != 0 {
			return unavailable()
		}
		seed, closed, err := threadapp.ReadForkCreatedSeedV1(primary.Thread, loaded.Events)
		if err != nil || seed == nil || validateQuestionHistoryCreationMarkerV1(primary.Thread, loaded.Events, closed) != nil {
			return unavailable()
		}
		layers = append(layers, layerV1{id, primary.Thread, seed})
		id = stringField(seed, "sourceThreadId")
	}
	if len(layers) == 0 {
		return unavailable()
	}
	// The source's signed input discriminator and the child seed jointly select
	// a fixed attempt. Other questions and newer captures cannot be candidates.
	var delivery *userInputLateReplyRecordV1
	var reply *displayapp.HistoryReplyV1
	for _, rawTurn := range listAny(source.Thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawEntry := range listAny(turn["steering"]) {
			entry, _ := rawEntry.(map[string]any)
			if stringField(entry, "userInputId") != binding.InputID || entry["status"] != "promoted" {
				continue
			}
			attempt, entryID := stringField(entry, "clientUserMessageId"), stringField(entry, "id")
			if !controlapp.IsOpaqueClientUserMessageID(attempt) {
				return unavailable()
			}
			promotedInSeed := false
			for _, rawSeedTurn := range listAny(layers[0].seed["turns"]) {
				for _, rawSeedEntry := range listAny(rawSeedTurn.(map[string]any)["steering"]) {
					seedEntry, _ := rawSeedEntry.(map[string]any)
					if seedEntry != nil && seedEntry["status"] == "promoted" && seedEntry["id"] == entryID && seedEntry["clientUserMessageId"] == attempt {
						promotedInSeed = true
					}
				}
			}
			// A queued/discarded inherited carrier remains valid no-answer history,
			// even if its original source has since promoted that same attempt.
			if !promotedInSeed {
				continue
			}
			_, _, present, err := questionHistoryReplyPairV1(map[string]any{"turns": layers[0].seed["turns"]}, layers[0].id, stringField(turn, "id"), entryID, attempt)
			if err != nil {
				return unavailable()
			}
			if !present {
				continue
			}
			trusted, err := h.store.readUserInputLateReplyV1(binding.ThreadID, binding.InputID, attempt)
			if err != nil || trusted == nil || trusted.State != "accepted" || trusted.TurnID != stringField(turn, "id") || trusted.Entry["id"] != entryID || !domainsecurity.IsSHA256Hex(trusted.CaptureReceiptID) || delivery != nil {
				return unavailable()
			}
			delivery = trusted
			reply = &displayapp.HistoryReplyV1{ItemID: entryID, ClientUserMessageID: attempt, TurnID: trusted.TurnID}
		}
	}
	if delivery == nil {
		return result, nil
	}
	// Each edge is checked against its own inclusive source boundary. Reuse the
	// normal prefix fold, including complete parent seed closure for reforks.
	for _, layer := range layers {
		seedThread := map[string]any{"turns": layer.seed["turns"]}
		seedEntry, seedItem, found, err := questionHistoryReplyPairV1(seedThread, layer.id, reply.TurnID, reply.ItemID, reply.ClientUserMessageID)
		if err != nil || !found {
			return unavailable()
		}
		primaryEntry, primaryItem, found, err := questionHistoryReplyPairV1(layer.primary, layer.id, reply.TurnID, reply.ItemID, reply.ClientUserMessageID)
		if err != nil || !found || !questionHistoryJSONEqualV1(seedEntry, primaryEntry) {
			return unavailable()
		}
		parentID := stringField(layer.seed, "sourceThreadId")
		parent, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, parentID)
		if err != nil {
			return unavailable()
		}
		// This existing helper is read-only but requires the durable store mutex.
		h.store.mu.Lock()
		questions, proofErr := h.store.forkQuestionHistoryEntriesNoLockV1(parent.Thread)
		h.store.mu.Unlock()
		if proofErr != nil {
			return unavailable()
		}
		prefix := contracts.CloneMap(parent.Thread)
		if sourceTurnID := stringField(layer.seed, "sourceTurnId"); sourceTurnID != "" {
			endSeq, ok := contracts.NumericSeq(layer.seed["sourceEndSeq"])
			if !ok {
				return unavailable()
			}
			loaded, _, err := h.store.eventLog.ObserveSinceWithFrontier(ctx, parentID, 0)
			if err != nil || len(loaded.Diagnostics) != 0 {
				return unavailable()
			}
			prefix, _, err = threadapp.BuildForkEventPrefixV1(threadapp.ForkEventPrefixInputV1{Source: parent.Thread, Events: loaded.Events,
				Boundary: threadapp.ForkBoundaryV1{TurnID: sourceTurnID, EndSeq: endSeq}, Projector: h.publicProjector, QuestionHistoryEntries: questions})
			if err != nil {
				return unavailable()
			}
		}
		if threadapp.NormalizeForkQuestionHistoryV1(prefix, questions) != nil {
			return unavailable()
		}
		parentEntry, parentItem, found, err := questionHistoryReplyPairV1(prefix, parentID, reply.TurnID, reply.ItemID, reply.ClientUserMessageID)
		if err != nil || !found {
			return unavailable()
		}
		cloned := threadapp.CloneItemForThread(parentItem, layer.id, stringField(layer.primary, "createdAt"))
		// Compare complete records in the same domain: the primary retains
		// legitimate inert verification metadata, while the seed is public-only.
		if !questionHistoryJSONEqualV1(parentEntry, seedEntry) || !questionHistoryJSONEqualV1(cloned, primaryItem) {
			return unavailable()
		}
		publicCopy := contracts.CloneMap(layer.primary)
		delete(publicCopy, forkCreationPendingFieldV1)
		public, err := h.publicProjector.ProjectThread(publicCopy)
		if err != nil {
			return unavailable()
		}
		publicItem, found := questionHistoryPublicItemV1(public, layer.id, reply)
		if !found || !questionHistoryJSONEqualV1(publicItem, seedItem) {
			return unavailable()
		}
	}
	submission, err := h.continuations.ReadLateUserInputSubmissionHost(ctx, binding, reply.ClientUserMessageID, delivery.CaptureReceiptID)
	if err != nil {
		return unavailable()
	}
	final, err := h.store.readUserInputLateReplyV1(binding.ThreadID, binding.InputID, reply.ClientUserMessageID)
	if err != nil || !sameUserInputLateReplyRecordV1(delivery, final) || ctx.Err() != nil {
		return unavailable()
	}
	result.Record.Submission, result.Record.AnswerState, result.Reply = &submission, "available", reply
	return result, nil
}

func questionHistoryJSONEqualV1(a, b map[string]any) bool {
	first, err := json.Marshal(a)
	second, secondErr := json.Marshal(b)
	return err == nil && secondErr == nil && string(first) == string(second)
}

// A public UUID alone is insufficient. Both unique immutable carriers must
// occupy the same actual turn in this layer, with their completed lifecycle.
func questionHistoryReplyPairV1(thread map[string]any, threadID, turnID, itemID, attempt string) (map[string]any, map[string]any, bool, error) {
	var entry, item map[string]any
	entryCount, itemCount := 0, 0
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawEntry := range listAny(turn["steering"]) {
			candidate, _ := rawEntry.(map[string]any)
			if candidate != nil && (stringField(candidate, "id") == itemID || stringField(candidate, "clientUserMessageId") == attempt) {
				entryCount++
				entry = candidate
				if stringField(turn, "id") != turnID {
					return nil, nil, false, port.ErrUnavailableV1
				}
			}
		}
		for _, rawItem := range listAny(turn["items"]) {
			candidate, _ := rawItem.(map[string]any)
			if candidate != nil && (stringField(candidate, "id") == itemID || stringField(candidate, "clientUserMessageId") == attempt) {
				itemCount++
				item = candidate
				if stringField(turn, "id") != turnID {
					return nil, nil, false, port.ErrUnavailableV1
				}
			}
		}
	}
	if entryCount == 0 && itemCount == 0 {
		return nil, nil, false, nil
	}
	if entryCount != 1 || itemCount != 1 || stringField(entry, "id") != itemID || stringField(entry, "clientUserMessageId") != attempt ||
		entry["status"] != "promoted" || entry["promotedItemId"] != itemID || entry["kind"] != "user_message" || entry["delivery"] != "steer" ||
		item["id"] != itemID || item["clientUserMessageId"] != attempt || item["threadId"] != threadID || item["turnId"] != turnID ||
		item["kind"] != "user_message" || item["role"] != "user" || item["delivery"] != "steer" || item["status"] != "completed" ||
		item["createdAt"] != entry["admittedAt"] || item["finishedAt"] != entry["promotedAt"] || item["text"] != entry["text"] {
		return nil, nil, false, port.ErrUnavailableV1
	}
	return entry, item, true, nil
}

func (h *runtimeServerHandler) UserInputLocalDisplayV1() http.Handler {
	if h == nil {
		return httpapi.UserInputLocalDisplayHandlerV1{}
	}
	return httpapi.UserInputLocalDisplayHandlerV1{Service: runtimeUserInputDisplayServiceV1{h}}
}

// The wrapper adds only a verified view of the existing steering delivery
// under the Service's original private Question scope lease.
type runtimeUserInputDisplayReaderV1 struct{ handler *runtimeServerHandler }

func (reader runtimeUserInputDisplayReaderV1) ReadUserInputLocalDisplayHost(ctx context.Context, binding continuationapp.UserInputLocalDisplayBindingV1) (continuationapp.UserInputLocalDisplayV1, error) {
	h := reader.handler
	record, err := h.continuations.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || record.Status != "continued" {
		return record, err
	}
	delivery, err := h.store.readUserInputLateReplyV1(binding.ThreadID, binding.InputID, "")
	if err != nil {
		return continuationapp.UserInputLocalDisplayV1{}, port.ErrUnavailableV1
	}
	latest, err := h.continuations.ReadLatestLateUserInputSubmissionHost(ctx, binding)
	if err != nil {
		return continuationapp.UserInputLocalDisplayV1{}, port.ErrUnavailableV1
	}
	if delivery == nil {
		if latest != nil {
			record.Submission, record.AnswerState = &latest.Submission, "available"
		}
		return record, nil
	}
	if latest != nil && delivery.State == "discarded" && latest.AttemptID != stringField(delivery.Entry, "clientUserMessageId") {
		capturedAt, capturedErr := time.Parse(time.RFC3339Nano, latest.CapturedAt)
		admittedAt, admittedErr := time.Parse(time.RFC3339Nano, stringField(delivery.Entry, "admittedAt"))
		if capturedErr != nil || admittedErr != nil {
			return continuationapp.UserInputLocalDisplayV1{}, port.ErrUnavailableV1
		}
		if capturedAt.After(admittedAt) {
			record.Submission, record.AnswerState = &latest.Submission, "available"
			return record, nil
		}
	}
	submission, err := h.continuations.ReadLateUserInputSubmissionHost(ctx, binding, stringField(delivery.Entry, "clientUserMessageId"), delivery.CaptureReceiptID)
	if err != nil {
		return continuationapp.UserInputLocalDisplayV1{}, port.ErrUnavailableV1
	}
	finalDelivery, err := h.store.readUserInputLateReplyV1(binding.ThreadID, binding.InputID, "")
	if err != nil || !sameUserInputLateReplyRecordV1(delivery, finalDelivery) {
		return continuationapp.UserInputLocalDisplayV1{}, port.ErrUnavailableV1
	}
	record.Submission, record.AnswerState = &submission, "available"
	if delivery.State == "queued" || delivery.State == "accepted" {
		record.Draft = nil
	}
	record.LateReply = &continuationapp.UserInputLateReplyDeliveryV1{ClientUserMessageID: stringField(delivery.Entry, "clientUserMessageId"), TurnID: delivery.TurnID, State: delivery.State}
	return record, nil
}

// The ordinary effect precedes the private metadata lease. Reversing this
// order can deadlock against a waiting security-scope transition writer.
type runtimeUserInputDisplayServiceV1 struct{ handler *runtimeServerHandler }

func (service runtimeUserInputDisplayServiceV1) hostServiceV1() *displayapp.ServiceV1 {
	result := &displayapp.ServiceV1{Reader: runtimeUserInputDisplayReaderV1{service.handler}, Authority: runtimeUserInputDisplayAccessV1{service.handler}, Discarder: runtimeUserInputDisplayDiscarderV1{service.handler}}
	if service.handler != nil {
		result.DraftSaver = service.handler.continuations
	}
	return result
}

func (service runtimeUserInputDisplayServiceV1) Read(ctx context.Context, selector port.SelectorV1) (displayapp.DisplayV1, error) {
	return service.hostServiceV1().Read(ctx, selector)
}

func (service runtimeUserInputDisplayServiceV1) DiscardLateReply(ctx context.Context, request displayapp.DiscardLateReplyRequestV1) (displayapp.DiscardLateReplyResponseV1, error) {
	h := service.handler
	unavailable := func() (displayapp.DiscardLateReplyResponseV1, error) {
		return displayapp.DiscardLateReplyResponseV1{}, port.ErrUnavailableV1
	}
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.runtimeControl().ShuttingDown() || !displayapp.ValidateDiscardLateReplyRequestV1(request) {
		return unavailable()
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, request.ThreadID)
	if err != nil {
		return unavailable()
	}
	target, err := turnapp.FrozenSecurityContextForTurn(snapshot.Thread, request.ExpectedTurnID)
	if err != nil {
		return unavailable()
	}
	current, err := snapshotapp.CurrentOwnContextV1(snapshot.Thread, target)
	if err != nil {
		return unavailable()
	}
	effectCtx, release, err := h.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, current, false)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return unavailable()
	}
	defer release()
	validate := func() error {
		return turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: effectCtx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: current.WorkspaceRealPath})
	}
	if validate() != nil {
		return unavailable()
	}
	// Bind the metadata scope to the already-held ordinary effect before
	// Service acquires its retained read lease; a rebind cannot switch scope.
	_, releaseDisplay, err := (runtimeUserInputDisplayAccessV1{h}).acquireCurrent(effectCtx, request.SelectorV1, &current)
	if err != nil || releaseDisplay == nil {
		if releaseDisplay != nil {
			releaseDisplay()
		}
		return unavailable()
	}
	defer releaseDisplay()
	response, err := service.hostServiceV1().DiscardLateReply(effectCtx, request)
	if effectCtx.Err() != nil || validate() != nil {
		return unavailable()
	}
	return response, err
}

type runtimeUserInputDisplayDiscarderV1 struct{ handler *runtimeServerHandler }

func (discarder runtimeUserInputDisplayDiscarderV1) DiscardLateReply(ctx context.Context, authorized port.AuthorizedV1, attemptID, targetTurnID string) (continuationapp.UserInputLateReplyDeliveryV1, error) {
	h := discarder.handler
	unavailable := func() (continuationapp.UserInputLateReplyDeliveryV1, error) {
		return continuationapp.UserInputLateReplyDeliveryV1{}, port.ErrUnavailableV1
	}
	request := displayapp.DiscardLateReplyRequestV1{SelectorV1: authorized.Selector, Action: "discardLateReply", ClientUserMessageID: attemptID, ExpectedTurnID: targetTurnID}
	access := runtimeUserInputDisplayAccessV1{h}
	if h == nil || h.store == nil || h.continuations == nil || authorized.Status != "continued" || !displayapp.ValidateDiscardLateReplyRequestV1(request) || access.ValidateCurrent(ctx, authorized) != nil {
		return unavailable()
	}
	previous, err := h.store.readUserInputLateReplyV1(authorized.Selector.ThreadID, authorized.Selector.RequestID, attemptID)
	if err != nil || previous == nil || previous.TurnID != targetTurnID {
		return unavailable()
	}
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: authorized.Selector.ThreadID, TurnID: authorized.Selector.TurnID, ItemID: authorized.Selector.ItemID, InputID: authorized.Selector.RequestID, ReceiptID: authorized.ReceiptID, SecurityContext: authorized.Original}
	carrier, err := h.verifiedLateUserInputSteeringV1(ctx, binding, attemptID, previous.CaptureReceiptID, targetTurnID)
	if err != nil || carrier.contextDigest != stringField(previous.Entry, "contextDigest") {
		return unavailable()
	}
	identity, present, err := domainsteering.UserInputReplyIdentityFromEntryV1(previous.Entry)
	if err != nil || !present || carrier.identity == nil || !reflect.DeepEqual(identity, *carrier.identity) || access.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	expected := domainsteering.PendingEntryExpectationV1{ID: stringField(previous.Entry, "id"), ContentDigest: stringField(previous.Entry, "contentDigest")}
	delivery, err := h.store.discardUserInputLateReplyV1(ctx, authorized.Selector.ThreadID, authorized.Selector.RequestID, attemptID, targetTurnID, previous.CaptureReceiptID, carrier.contextDigest, expected)
	if err != nil {
		if access.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
			return unavailable()
		}
		if errors.Is(err, domainsteering.ErrEntryPromoted) {
			return continuationapp.UserInputLateReplyDeliveryV1{}, displayapp.ErrDiscardConflictV1
		}
		return unavailable()
	}
	if delivery == nil || delivery.State != "discarded" || delivery.TurnID != targetTurnID || delivery.CaptureReceiptID != previous.CaptureReceiptID || stringField(delivery.Entry, "clientUserMessageId") != attemptID || access.ValidateCurrent(ctx, authorized) != nil || ctx.Err() != nil {
		return unavailable()
	}
	return continuationapp.UserInputLateReplyDeliveryV1{ClientUserMessageID: attemptID, TurnID: targetTurnID, State: "discarded"}, nil
}

func (service runtimeUserInputDisplayServiceV1) SaveDraft(ctx context.Context, request displayapp.SaveDraftRequestV1) (displayapp.SaveDraftResponseV1, error) {
	h := service.handler
	unavailable := func() (displayapp.SaveDraftResponseV1, error) {
		return displayapp.SaveDraftResponseV1{}, port.ErrUnavailableV1
	}
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.continuations == nil || h.runtimeControl().ShuttingDown() || !displayapp.ValidateSaveDraftRequestV1(request) {
		return unavailable()
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, request.ThreadID)
	if err != nil {
		return unavailable()
	}
	original, err := turnapp.FrozenSecurityContextForTurn(snapshot.Thread, request.TurnID)
	if err != nil {
		return unavailable()
	}
	current, err := snapshotapp.CurrentOwnContextV1(snapshot.Thread, original)
	if err != nil {
		return unavailable()
	}
	effectCtx, release, err := h.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, current, false)
	if err != nil || effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return unavailable()
	}
	defer release()
	validate := func() error {
		return turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: effectCtx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: current.WorkspaceRealPath})
	}
	if validate() != nil {
		return unavailable()
	}
	_, releaseDisplay, err := (runtimeUserInputDisplayAccessV1{h}).acquireCurrent(effectCtx, request.SelectorV1, &current)
	if err != nil || releaseDisplay == nil {
		if releaseDisplay != nil {
			releaseDisplay()
		}
		return unavailable()
	}
	defer releaseDisplay()
	response, err := service.hostServiceV1().SaveDraft(effectCtx, request)
	if effectCtx.Err() != nil || validate() != nil {
		return unavailable()
	}
	return response, err
}

func questionHistoryPublicItemV1(thread map[string]any, threadID string, reply *displayapp.HistoryReplyV1) (map[string]any, bool) {
	var selected map[string]any
	count := 0
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if item != nil && (item["id"] == reply.ItemID || item["clientUserMessageId"] == reply.ClientUserMessageID) {
				count++
				selected = item
				if turn["id"] != reply.TurnID {
					return nil, false
				}
			}
		}
	}
	return selected, count == 1 && selected["id"] == reply.ItemID && selected["clientUserMessageId"] == reply.ClientUserMessageID &&
		selected["threadId"] == threadID && selected["turnId"] == reply.TurnID && selected["kind"] == "user_message" && selected["status"] == "completed" && selected["delivery"] == "steer"
}
