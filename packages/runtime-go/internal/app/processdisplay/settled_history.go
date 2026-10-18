package processdisplay

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	display "analytix.local/runtime-go/internal/domain/processdisplay"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
)

// FreezeSettledV1 copies only closed projected values at the real Primary
// terminal owner. FinishV1's handles-zero flag is deliberately not authority.
// This method must be called outside the Primary store lock.
func (s *ServiceV1) FreezeSettledV1(binding port.BindingV1, terminal display.TerminalWitnessV1) (display.SettledHistoryV1, error) {
	if s == nil || !validBindingV1(binding) || !display.ValidTerminalWitnessV1(terminal) {
		return display.SettledHistoryV1{}, port.ErrUnavailableV1
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.turns[scopeKeyV1(binding.ThreadID, binding.TurnID)]
	if s.closed || t == nil || t.binding != binding {
		return display.SettledHistoryV1{}, port.ErrUnavailableV1
	}
	record := display.SettledHistoryV1{SchemaVersion: display.SettledHistorySchemaV1, ThreadID: binding.ThreadID, TurnID: binding.TurnID, Principal: binding.Principal, Original: binding.Original, Terminal: terminal, Units: []display.UnitV2{}, Blocks: []display.SettledBlockV1{}}
	for _, unit := range t.units {
		u := unit.value
		u.Calls = append([]display.CallAnchorV2{}, u.Calls...)
		u.PrecedingCalls = append([]display.CallAnchorV2{}, u.PrecedingCalls...)
		record.Units = append(record.Units, u)
	}
	for _, raw := range t.blocks {
		b := raw.value
		// Live draft/preparation and source requests are never promoted into
		// durable source facts merely because the turn is becoming terminal.
		if b.State == "running" || b.State == "preparing" || b.Kind == "tool_input" || b.Kind == "tool_preparation" || b.Kind == "web_fetch" {
			continue
		}
		entry := display.SettledBlockV1{Display: b}
		if b.Source.Kind == "tool" {
			tb := b.ToolBinding
			if tb.CallID == "" || tb.ResultItemID == "" || tb.GrantID == "" {
				continue
			}
			entry.Tool = &display.SettledToolBindingV1{CallID: tb.CallID, ResultItemID: tb.ResultItemID, ToolName: tb.ToolName, GrantID: tb.GrantID, InputArgsHash: tb.InputArgsHash}
		}
		entry.Display.ToolBinding = display.ToolBindingV1{}
		entry.Display.Input = nil
		entry.Display.DraftSegments = nil
		entry.Display.DraftBytes = ""
		entry.Display.RequestURL = ""
		entry.Display.InputDigest = ""
		record.Blocks = append(record.Blocks, entry)
	}
	// The strict canonical roundtrip detaches every pointer and slice and
	// proves the disk codec (including the original security V2 context).
	body, err := display.SettledHistoryBytesV1(record)
	if err != nil {
		return display.SettledHistoryV1{}, err
	}
	return display.ParseSettledHistoryV1(body)
}

func historyTurnV1(record display.SettledHistoryV1, anchor display.SettledHistoryAnchorV1, at time.Time) *turnV1 {
	if display.ValidateSettledHistoryV1(record) != nil || display.ValidateSettledHistoryAnchorV1(anchor) != nil || anchor.Availability != "available" || record.ThreadID != anchor.ThreadID || record.TurnID != anchor.TurnID || record.Original.ContextDigest != anchor.ContextDigest || record.Terminal != anchor.Terminal {
		return nil
	}
	body, err := display.SettledHistoryBytesV1(record)
	if err != nil {
		return nil
	}
	actual, err := display.SettledHistoryAnchorForV1(record)
	if err != nil || actual != anchor {
		return nil
	}
	// A restored turn is viewer-local, readonly and absent from the live quota.
	t := &turnV1{binding: port.BindingV1{ThreadID: record.ThreadID, TurnID: record.TurnID, Principal: record.Principal, Original: record.Original}, incarnation: randomIDV1(), revision: 1, terminal: true, activity: at, wake: make(chan struct{}), historyAnchor: &anchor, byKey: map[string]*rawBlockV1{}}
	if t.incarnation == "" {
		return nil
	}
	var detached display.SettledHistoryV1
	if json.Unmarshal(body, &detached) != nil || !reflect.DeepEqual(detached.Original, record.Original) {
		return nil
	}
	for _, u := range detached.Units {
		t.units = append(t.units, &unitV2{value: u})
	}
	for _, entry := range detached.Blocks {
		b := entry.Display
		if entry.Tool != nil {
			tb := entry.Tool
			b.ToolBinding = display.ToolBindingV1{CallID: tb.CallID, ResultItemID: tb.ResultItemID, ToolName: tb.ToolName, GrantID: tb.GrantID, InputArgsHash: tb.InputArgsHash}
		}
		t.blocks = append(t.blocks, &rawBlockV1{value: b})
	}
	return t
}

func (s *ServiceV1) validViewerTurnLockedV1(t *turnV1) bool {
	return t != nil && (t.historyAnchor != nil || s.turns[scopeKeyV1(t.binding.ThreadID, t.binding.TurnID)] == t)
}
func (s *ServiceV1) validateHistoryCurrentV1(ctx context.Context, t *turnV1) error {
	if t.historyAnchor == nil {
		return nil
	}
	a, ok := s.authority.(port.SettledHistoryReadAuthorityV1)
	if !ok {
		return port.ErrUnavailableV1
	}
	return a.ValidateSettledHistoryCurrentV1(ctx, t.binding, *t.historyAnchor)
}
func (s *ServiceV1) hasHistoryViewerLockedV1() bool {
	for _, v := range s.viewers {
		if !v.revoked && v.turn != nil && v.turn.historyAnchor != nil {
			return true
		}
	}
	return false
}
