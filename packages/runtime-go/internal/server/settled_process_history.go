package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	identity "analytix.local/runtime-go/internal/domain/identity"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	security "analytix.local/runtime-go/internal/domain/security"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
)

func processHistoryTerminalV1(node map[string]any, turnID string) (map[string]any, security.TurnSecurityContext, display.TerminalWitnessV1, error) {
	if stringField(node, "status") == "deleted" {
		return nil, security.TurnSecurityContext{}, display.TerminalWitnessV1{}, port.ErrUnavailableV1
	}
	var found map[string]any
	turns, _ := node["turns"].([]any)
	for _, raw := range turns {
		t, _ := raw.(map[string]any)
		if stringField(t, "id") == turnID {
			if found != nil {
				return nil, security.TurnSecurityContext{}, display.TerminalWitnessV1{}, port.ErrUnavailableV1
			}
			found = t
		}
	}
	original, err := turnapp.FrozenSecurityContextForTurn(node, turnID)
	if err != nil || found == nil || stringField(node, "id") != original.ThreadID {
		return nil, security.TurnSecurityContext{}, display.TerminalWitnessV1{}, port.ErrUnavailableV1
	}
	w := display.TerminalWitnessV1{Status: stringField(found, "status"), FinishedAt: stringField(found, "finishedAt")}
	general, _ := found["generalTerminalPublication"].(map[string]any)
	accepted, _ := found["acceptedFinal"].(map[string]any)
	if general != nil && accepted == nil {
		w.AuthorityDigest = stringField(general, "commitDigest")
	} else if accepted != nil && general == nil {
		w.AuthorityDigest = stringField(accepted, "recordDigest")
	}
	// Bind the actual closed Primary tool inventory as well as its terminal
	// publication authority. Later primary drift cannot retain the old anchor.
	sources := []any{}
	items, _ := found["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "tool_call" || stringField(item, "kind") == "tool_result" {
			metadata := map[string]any{}
			for _, key := range []string{"id", "kind", "threadId", "turnId", "callId", "toolName", "status", "contextDigest", "contextEpoch", "executionGrant", "executionGrantId"} {
				if value, present := item[key]; present {
					metadata[key] = value
				}
			}
			sources = append(sources, metadata)
		}
	}
	body, sourceErr := json.Marshal(sources)
	if sourceErr != nil {
		return nil, security.TurnSecurityContext{}, display.TerminalWitnessV1{}, port.ErrUnavailableV1
	}
	w.SourceDigest = security.SHA256Hex(body)
	if !display.ValidTerminalWitnessV1(w) {
		return nil, security.TurnSecurityContext{}, display.TerminalWitnessV1{}, port.ErrUnavailableV1
	}
	return found, original, w, nil
}

// Called only with the actual validated prepared Primary terminal mutation,
// outside the store lock. The storage result cannot rewrite terminal truth.
func (h *runtimeServerHandler) prepareSettledProcessHistoryV1(node map[string]any, turnID string) *display.SettledHistoryAnchorV1 {
	terminalTurn, original, terminal, err := processHistoryTerminalV1(node, turnID)
	if err != nil {
		return nil
	}
	anchor := display.SettledHistoryAnchorV1{SchemaVersion: display.SettledHistoryAnchorSchemaV1, ThreadID: original.ThreadID, TurnID: turnID, ContextDigest: original.ContextDigest, Terminal: terminal, Availability: "unavailable", Reason: "unavailable"}
	if h == nil || h.processDisplay == nil || h.processHistory == nil {
		return &anchor
	}
	if h.runtimeControl().ShuttingDown() && (!h.processDisplay.ShutdownDrainingV1() || !h.runtimeControl().OwnsTerminalClaim(original.ThreadID, turnID, original.ContextDigest)) {
		return &anchor
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil || principal.TenantID != original.TenantID || principal.UserID != original.UserID {
		return &anchor
	}
	// Freeze is a detached projected copy. It never calls a Primary reader or
	// derives a terminal from FinishV1, raw blocks or current shadow context.
	record, err := h.processDisplay.FreezeSettledV1(port.BindingV1{ThreadID: original.ThreadID, TurnID: turnID, Principal: principal, Original: original}, terminal)
	if err != nil {
		if errors.Is(err, port.ErrUnavailableV1) {
			anchor.Reason = "missing"
		}
		return &anchor
	}
	if !processHistoryReferencesV1(terminalTurn, record) {
		return &anchor
	}
	stored, err := h.processHistory.Put(ctx, record)
	if err != nil {
		if errors.Is(err, port.ErrCapacityV1) {
			anchor.Reason = "capacity"
		}
		return &anchor
	}
	actual, err := display.SettledHistoryAnchorForV1(record)
	if err != nil || actual != stored || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return &anchor
	}
	return &stored
}

func (a runtimeProcessDisplayAccessV1) ReadSettledHistoryV1(ctx context.Context, threadID, turnID string) (display.SettledHistoryV1, display.SettledHistoryAnchorV1, error) {
	h := a.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.processHistory == nil {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if err != nil {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	turn, original, witness, err := processHistoryTerminalV1(primary.Thread, turnID)
	anchor, parseErr := display.ParseSettledHistoryAnchorV1(turn[display.SettledHistoryFieldV1])
	principal, principalErr := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil || parseErr != nil || principalErr != nil || anchor.Availability != "available" || anchor.ThreadID != threadID || anchor.TurnID != turnID || anchor.ContextDigest != original.ContextDigest || anchor.Terminal != witness {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	binding := port.BindingV1{ThreadID: threadID, TurnID: turnID, Principal: principal, Original: original}
	current, release, err := a.AcquireCurrent(ctx, binding)
	if err != nil || release == nil {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	defer release()
	if current.Binding != binding || a.ValidateSettledHistoryCurrentV1(ctx, binding, anchor) != nil {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	record, err := h.processHistory.Read(ctx, anchor)
	if err != nil || !identity.SamePrincipalV1(record.Principal, principal) || !reflect.DeepEqual(record.Original, original) || record.Terminal != witness || record.ThreadID != threadID || record.TurnID != turnID || !processHistoryReferencesV1(turn, record) || a.ValidateSettledHistoryCurrentV1(ctx, binding, anchor) != nil {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	for _, entry := range record.Blocks {
		if entry.Tool == nil {
			continue
		}
		t := entry.Tool
		if a.ValidateToolCurrent(ctx, binding, display.ToolBindingV1{CallID: t.CallID, ResultItemID: t.ResultItemID, ToolName: t.ToolName, GrantID: t.GrantID, InputArgsHash: t.InputArgsHash}) != nil {
			return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
		}
	}
	if a.ValidateSettledHistoryCurrentV1(ctx, binding, anchor) != nil {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	return record, anchor, nil
}

func processHistoryReferencesV1(turn map[string]any, record display.SettledHistoryV1) bool {
	calls := map[string]map[string]any{}
	results := map[string]map[string]any{}
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		id := stringField(item, "callId")
		switch stringField(item, "kind") {
		case "tool_call":
			if id == "" || calls[id] != nil {
				return false
			}
			calls[id] = item
		case "tool_result":
			if id == "" || results[id] != nil {
				return false
			}
			results[id] = item
		}
	}
	for _, unit := range record.Units {
		for _, ref := range append(append([]display.CallAnchorV2{}, unit.Calls...), unit.PrecedingCalls...) {
			call := calls[ref.CallID]
			if call == nil || stringField(call, "id") != ref.ToolCallItemID || stringField(call, "threadId") != record.ThreadID || stringField(call, "turnId") != record.TurnID {
				return false
			}
			if ref.ToolResultItemID != "" {
				result := results[ref.CallID]
				if result == nil || stringField(result, "id") != ref.ToolResultItemID || stringField(result, "threadId") != record.ThreadID || stringField(result, "turnId") != record.TurnID {
					return false
				}
			}
		}
	}
	return true
}

func (a runtimeProcessDisplayAccessV1) ValidateSettledHistoryCurrentV1(ctx context.Context, binding port.BindingV1, anchor display.SettledHistoryAnchorV1) error {
	if a.validateBoundV1(ctx, binding) != nil || display.ValidateSettledHistoryAnchorV1(anchor) != nil || anchor.Availability != "available" {
		return port.ErrUnavailableV1
	}
	primary, err := a.handler.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil {
		return port.ErrUnavailableV1
	}
	turn, original, witness, err := processHistoryTerminalV1(primary.Thread, binding.TurnID)
	actual, parseErr := display.ParseSettledHistoryAnchorV1(turn[display.SettledHistoryFieldV1])
	if err != nil || parseErr != nil || actual != anchor || witness != anchor.Terminal || !reflect.DeepEqual(original, binding.Original) {
		return port.ErrUnavailableV1
	}
	return a.validateBoundV1(ctx, binding)
}

var _ port.SettledHistoryReadAuthorityV1 = runtimeProcessDisplayAccessV1{}
