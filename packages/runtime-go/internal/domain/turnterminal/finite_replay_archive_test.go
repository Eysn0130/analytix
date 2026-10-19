package turnterminal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestFiniteReplayArchiveParsesEquivalentContentOnceAndPreservesLegacyResults(t *testing.T) {
	thread, events := finiteReplayArchiveFixture(t)
	prepared := NewGeneralTerminalReplayArchiveV1(context.Background())
	parseCalls := countFiniteReplayArchiveParses(prepared)
	for _, raw := range events {
		fresh := cloneGeneralTerminalMap(thread)
		governed, visible, err := prepared.ValidateEvent(fresh, raw)
		legacyGoverned, legacyVisible, legacyErr := ValidateGeneralTerminalPublicEventV1(fresh, raw)
		if err != nil || legacyErr != nil || governed != legacyGoverned || visible != legacyVisible || !visible {
			t.Fatalf("prepared canonical event differs from legacy: prepared=(%t,%t,%v) legacy=(%t,%t,%v)",
				governed, visible, err, legacyGoverned, legacyVisible, legacyErr)
		}
		// Equivalent independently decoded maps must match the actual archive,
		// rather than relying on the identity of a mutable source map.
	}
	tamperedEvent := cloneGeneralTerminalMap(events[0])
	tamperedEvent["item"].(map[string]any)["text"] = "uncommitted terminal text"
	governed, visible, err := prepared.ValidateEvent(cloneGeneralTerminalMap(thread), tamperedEvent)
	legacyGoverned, legacyVisible, legacyErr := ValidateGeneralTerminalPublicEventV1(thread, tamperedEvent)
	if err != nil || legacyErr != nil || governed != legacyGoverned || visible != legacyVisible || visible {
		t.Fatalf("prepared event digest check differs from legacy: prepared=(%t,%t,%v) legacy=(%t,%t,%v)",
			governed, visible, err, legacyGoverned, legacyVisible, legacyErr)
	}
	if *parseCalls != 1 {
		t.Fatalf("unchanged archive was parsed %d times, want 1", *parseCalls)
	}
}

func TestFiniteReplayArchiveRechecksFreshTerminalWinner(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"finished-at", func(turn map[string]any) { turn["finishedAt"] = "2026-07-15T00:00:01Z" }},
		{"cas-binding", func(turn map[string]any) {
			turn["generalTerminalCASBinding"].(map[string]any)["bindingDigest"] = strings.Repeat("0", 64)
		}},
		{"canonical-item", func(turn map[string]any) {
			turn["items"].([]any)[0].(map[string]any)["text"] = "uncommitted terminal text"
		}},
		{"frozen-context", func(turn map[string]any) {
			turn["securityContext"].(map[string]any)["contextEpoch"] = json.Number("3")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread, events := finiteReplayArchiveFixture(t)
			prepared := NewGeneralTerminalReplayArchiveV1(context.Background())
			parseCalls := countFiniteReplayArchiveParses(prepared)
			if _, visible, err := prepared.ValidateEvent(thread, events[0]); err != nil || !visible {
				t.Fatalf("canonical fixture rejected: visible=%t err=%v", visible, err)
			}
			changed := cloneGeneralTerminalMap(thread)
			test.mutate(changed["turns"].([]any)[0].(map[string]any))
			governed, visible, err := prepared.ValidateEvent(changed, events[0])
			legacyGoverned, legacyVisible, legacyErr := ValidateGeneralTerminalPublicEventV1(changed, events[0])
			if err == nil || legacyErr == nil || governed != legacyGoverned || visible != legacyVisible || visible {
				t.Fatalf("fresh winner mutation reused a prior decision: prepared=(%t,%t,%v) legacy=(%t,%t,%v)",
					governed, visible, err, legacyGoverned, legacyVisible, legacyErr)
			}
			if *parseCalls != 1 {
				t.Fatalf("fresh winner check reparsed an unchanged archive %d times", *parseCalls)
			}
		})
	}
}

func TestFiniteReplayArchiveContentChangePermanentlyInvalidatesRequest(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, map[string]any)
	}{
		{"same-claimed-digest", func(_ *testing.T, thread map[string]any) {
			thread[GeneralTerminalPublicationArchiveFieldV1].(map[string]any)["unexpected"] = true
		}},
		{"different-valid-archive", func(t *testing.T, thread map[string]any) {
			empty, err := NewGeneralTerminalPublicationArchiveV1(nil)
			if err != nil {
				t.Fatal(err)
			}
			thread[GeneralTerminalPublicationArchiveFieldV1] = GeneralTerminalPublicationArchiveV1Map(empty)
		}},
		{"thread-identity", func(_ *testing.T, thread map[string]any) { thread["id"] = "another-thread" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread, events := finiteReplayArchiveFixture(t)
			prepared := NewGeneralTerminalReplayArchiveV1(context.Background())
			parseCalls := countFiniteReplayArchiveParses(prepared)
			if _, visible, err := prepared.ValidateEvent(thread, events[0]); err != nil || !visible {
				t.Fatalf("canonical fixture rejected: visible=%t err=%v", visible, err)
			}
			changed := cloneGeneralTerminalMap(thread)
			test.mutate(t, changed)
			for _, view := range []map[string]any{changed, cloneGeneralTerminalMap(thread)} {
				if _, visible, err := prepared.ValidateEvent(view, events[0]); err == nil || visible {
					t.Fatalf("archive change or A-to-B-to-A reversal escaped invalidation: visible=%t err=%v", visible, err)
				}
			}
			if *parseCalls != 1 {
				t.Fatalf("invalidated request parsed %d archives, want only its initial archive", *parseCalls)
			}
		})
	}
}

func TestFiniteReplayArchiveRejectsInvalidArchivesLikeLegacy(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, map[string]any)
	}{
		{"null", func(_ *testing.T, thread map[string]any) { thread[GeneralTerminalPublicationArchiveFieldV1] = nil }},
		{"unknown-field", func(_ *testing.T, thread map[string]any) {
			thread[GeneralTerminalPublicationArchiveFieldV1].(map[string]any)["unexpected"] = true
		}},
		{"bad-digest", func(_ *testing.T, thread map[string]any) {
			thread[GeneralTerminalPublicationArchiveFieldV1].(map[string]any)["archiveDigest"] = strings.Repeat("0", 64)
		}},
		{"foreign-thread", func(_ *testing.T, thread map[string]any) { thread["id"] = "another-thread" }},
		{"duplicate-commit", func(t *testing.T, thread map[string]any) {
			archive, err := ParseGeneralTerminalPublicationArchiveV1(thread[GeneralTerminalPublicationArchiveFieldV1])
			if err != nil {
				t.Fatal(err)
			}
			archive.Commits = append(archive.Commits, cloneGeneralTerminalPublicationCommitV1(archive.Commits[0]))
			archive.ArchiveDigest = generalTerminalPublicationArchiveDigestV1(archive)
			thread[GeneralTerminalPublicationArchiveFieldV1] = GeneralTerminalPublicationArchiveV1Map(archive)
		}},
		{"bad-inner-commit-with-valid-outer-digest", func(t *testing.T, thread map[string]any) {
			archive, err := ParseGeneralTerminalPublicationArchiveV1(thread[GeneralTerminalPublicationArchiveFieldV1])
			if err != nil {
				t.Fatal(err)
			}
			archive.Commits[0].TurnID = "tampered-turn"
			archive.ArchiveDigest = generalTerminalPublicationArchiveDigestV1(archive)
			thread[GeneralTerminalPublicationArchiveFieldV1] = GeneralTerminalPublicationArchiveV1Map(archive)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread, events := finiteReplayArchiveFixture(t)
			test.mutate(t, thread)
			governed, visible, err := NewGeneralTerminalReplayArchiveV1(context.Background()).ValidateEvent(thread, events[0])
			legacyGoverned, legacyVisible, legacyErr := ValidateGeneralTerminalPublicEventV1(thread, events[0])
			if err == nil || legacyErr == nil || governed != legacyGoverned || visible != legacyVisible || visible {
				t.Fatalf("invalid archive differs from legacy: prepared=(%t,%t,%v) legacy=(%t,%t,%v)",
					governed, visible, err, legacyGoverned, legacyVisible, legacyErr)
			}
		})
	}
}

func TestFiniteReplayArchiveDistinguishesAbsentFromNull(t *testing.T) {
	thread := map[string]any{"id": "ordinary-thread", "turns": []any{}}
	event := map[string]any{"kind": "turn_started", "threadId": thread["id"], "turnId": "ordinary-turn"}
	prepared := NewGeneralTerminalReplayArchiveV1(context.Background())
	parseCalls := countFiniteReplayArchiveParses(prepared)
	if governed, visible, err := prepared.ValidateEvent(thread, event); err != nil || governed || !visible {
		t.Fatalf("absent archive changed legacy event behavior: governed=%t visible=%t err=%v", governed, visible, err)
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = nil
	if _, visible, err := prepared.ValidateEvent(thread, event); err == nil || visible {
		t.Fatalf("present null archive was treated as absence: visible=%t err=%v", visible, err)
	}
	delete(thread, GeneralTerminalPublicationArchiveFieldV1)
	if _, visible, err := prepared.ValidateEvent(thread, event); err == nil || visible {
		t.Fatalf("absence-to-null-to-absence cleared invalidation: visible=%t err=%v", visible, err)
	}
	if *parseCalls != 0 {
		t.Fatalf("absent or already invalidated archive invoked parser %d times", *parseCalls)
	}
}

func TestFiniteReplayArchiveValidatesTheExactBytesUsedForItsKey(t *testing.T) {
	thread, events := finiteReplayArchiveFixture(t)
	empty, err := NewGeneralTerminalPublicationArchiveV1(nil)
	if err != nil {
		t.Fatal(err)
	}
	value := &finiteReplayChangingArchiveJSON{
		first: GeneralTerminalPublicationArchiveV1Map(empty),
		next:  thread[GeneralTerminalPublicationArchiveFieldV1],
	}
	thread[GeneralTerminalPublicationArchiveFieldV1] = value
	prepared := NewGeneralTerminalReplayArchiveV1(context.Background())
	parseCalls := countFiniteReplayArchiveParses(prepared)
	// The actual first bytes contain no archived winner. A second serialization
	// would supply the canonical archive and wrongly authorize this event under
	// a key derived from the first serialization.
	if _, visible, err := prepared.ValidateEvent(thread, events[0]); err == nil || visible {
		t.Fatalf("a later serialization authorized an event absent from the keyed archive: visible=%t err=%v", visible, err)
	}
	if value.marshals != 1 || *parseCalls != 1 || len(prepared.commits) != 0 {
		t.Fatalf("archive key and validated input diverged: marshals=%d parses=%d commits=%d", value.marshals, *parseCalls, len(prepared.commits))
	}
}

func TestFiniteReplayArchiveStopsForCancellation(t *testing.T) {
	t.Run("before-projection", func(t *testing.T) {
		thread, events := finiteReplayArchiveFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		prepared := NewGeneralTerminalReplayArchiveV1(ctx)
		parseCalls := countFiniteReplayArchiveParses(prepared)
		if _, visible, err := prepared.ValidateEvent(thread, events[0]); !errors.Is(err, context.Canceled) || visible || *parseCalls != 0 {
			t.Fatalf("cancelled projection parsed or exposed an event: visible=%t err=%v parses=%d", visible, err, *parseCalls)
		}
	})
	t.Run("during-real-parse", func(t *testing.T) {
		thread, events := finiteReplayArchiveFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		prepared := NewGeneralTerminalReplayArchiveV1(ctx)
		parseCalls := 0
		parseContext := &finiteReplayCancelValidationContext{Context: ctx, cancel: cancel}
		prepared.parse = func(_ context.Context, value any) (GeneralTerminalPublicationArchiveV1, error) {
			parseCalls++
			return parseGeneralTerminalPublicationArchiveContextV1(parseContext, value)
		}
		if _, visible, err := prepared.ValidateEvent(thread, events[0]); !errors.Is(err, context.Canceled) || visible {
			t.Fatalf("cancelled parser exposed an event: visible=%t err=%v", visible, err)
		}
		if parseCalls != 1 || parseContext.checks != 3 || prepared.initialized {
			t.Fatalf("cancelled parse installed reusable state: parses=%d contextChecks=%d initialized=%t", parseCalls, parseContext.checks, prepared.initialized)
		}
	})
}

func countFiniteReplayArchiveParses(prepared *GeneralTerminalReplayArchiveV1) *int {
	calls := new(int)
	parse := prepared.parse
	prepared.parse = func(ctx context.Context, value any) (GeneralTerminalPublicationArchiveV1, error) {
		*calls++
		return parse(ctx, value)
	}
	return calls
}

type finiteReplayCancelValidationContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (ctx *finiteReplayCancelValidationContext) Err() error {
	ctx.checks++
	// The actual parser checks at entry, then the archive validator checks at
	// entry and before its first commit. Cancel during that validation loop.
	if ctx.checks == 3 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

type finiteReplayChangingArchiveJSON struct {
	first    any
	next     any
	marshals int
}

func (archive *finiteReplayChangingArchiveJSON) MarshalJSON() ([]byte, error) {
	archive.marshals++
	if archive.marshals == 1 {
		return json.Marshal(archive.first)
	}
	return json.Marshal(archive.next)
}

func finiteReplayArchiveFixture(t *testing.T) (map[string]any, []map[string]any) {
	t.Helper()
	frozen, binding, drafts := generalTerminalPublicationFixture(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	const committedAt = "2026-07-15T00:00:00Z"
	commit, err := NewGeneralTerminalPublicationCommitV1(frozen, binding, committedAt, drafts)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := NewGeneralTerminalPublicationArchiveV1([]GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	turn := map[string]any{
		"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "completed", "finishedAt": committedAt,
		"securityContext": contextRecord, "items": []any{cloneGeneralTerminalMap(drafts[0].Draft["item"].(map[string]any))},
		"generalTerminalCASBinding":  GeneralTerminalCASBindingV1Map(binding),
		"generalTerminalPublication": GeneralTerminalPublicationCommitV1Map(commit),
	}
	thread := map[string]any{
		"id": frozen.ThreadID, "turns": []any{turn},
		GeneralTerminalPublicationArchiveFieldV1: GeneralTerminalPublicationArchiveV1Map(archive),
	}
	events := make([]map[string]any, 0, len(drafts))
	for index, draft := range drafts {
		event, err := GeneralTerminalPublicationEventDraftV1(commit, draft.Slot, draft.Draft)
		if err != nil {
			t.Fatal(err)
		}
		event["seq"] = index + 1
		events = append(events, event)
	}
	return thread, events
}
