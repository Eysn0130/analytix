package processdisplay

import (
	continuation "analytix.local/runtime-go/internal/domain/continuation"
	"context"
	"strings"
	"testing"
	"time"

	model "analytix.local/runtime-go/internal/domain/model"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	security "analytix.local/runtime-go/internal/domain/security"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
)

func historyWitnessFixtureV1() display.TerminalWitnessV1 {
	return display.TerminalWitnessV1{Status: "aborted", FinishedAt: "2026-10-14T01:00:00Z", AuthorityDigest: strings.Repeat("a", 64), SourceDigest: security.SHA256Hex([]byte("[]"))}
}
func closedHistoryFixtureV1(t *testing.T) display.SettledHistoryV1 {
	t.Helper()
	s, p, _ := serviceFixtureV1(t)
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "closed"})
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "closed", Text: "Alice closed reasoning"})
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeIndex: 1, NativeID: "unfinished", Text: "unfinished source"})
	if _, err := s.FreezeSettledV1(p.BindingV2(), display.TerminalWitnessV1{Status: "running"}); err == nil {
		t.Fatal("live handles minted durable terminal")
	}
	p.FinishV1()
	v, err := s.FreezeSettledV1(p.BindingV2(), historyWitnessFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Blocks) != 2 || v.Blocks[0].Display.Body == nil || v.Blocks[1].Display.Body != nil {
		t.Fatal("closed body lost or unfinished body persisted")
	}
	if strings.Contains(v.Blocks[0].Display.Body.Text, "unfinished") {
		t.Fatal("draft became settled source")
	}
	p.turn.blocks[0].value.Body.Text = "changed live copy"
	if v.Blocks[0].Display.Body.Text == "changed live copy" {
		t.Fatal("freeze aliased live body")
	}
	return v
}

type historyAccessFixtureV1 struct {
	accessFixtureV1
	record      display.SettledHistoryV1
	anchor      display.SettledHistoryAnchorV1
	reads       int
	denyHistory bool
}

func (a *historyAccessFixtureV1) ReadSettledHistoryV1(ctx context.Context, thread, turn string) (display.SettledHistoryV1, display.SettledHistoryAnchorV1, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reads++
	if a.denied || ctx.Err() != nil || thread != a.record.ThreadID || turn != a.record.TurnID {
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
	return a.record, a.anchor, nil
}
func (a *historyAccessFixtureV1) ValidateSettledHistoryCurrentV1(ctx context.Context, b port.BindingV1, anchor display.SettledHistoryAnchorV1) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.denied || a.denyHistory || ctx.Err() != nil || anchor != a.anchor || b.Principal != a.record.Principal || b.Original != a.record.Original {
		return port.ErrUnavailableV1
	}
	return nil
}

func TestSettledHistoryRestoresFreshReadonlyViewerWithoutLiveQuotaV1(t *testing.T) {
	v := closedHistoryFixtureV1(t)
	anchor, err := display.SettledHistoryAnchorForV1(v)
	if err != nil {
		t.Fatal(err)
	}
	a := &historyAccessFixtureV1{record: v, anchor: anchor}
	limits := DefaultLimitsV1()
	limits.MaxViewers = 1
	s := NewServiceV1(a, limits)
	defer s.Close()
	got := s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, v.ThreadID, v.TurnID)
	if got.Status != "authorized" || got.Scope == nil || len(s.turns) != 0 || got.Scope.ProcessIncarnation == "" {
		t.Fatal("history restored live producer or missing fresh scope")
	}
	if got := s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, v.ThreadID, v.TurnID); got.Status != "unavailable" {
		t.Fatal("duplicate viewer admitted")
	}
	if got := s.AuthorizeV1(context.Background(), "00000000-0000-4000-8000-000000000003", documentFixtureV1, v.ThreadID, v.TurnID); got.Status != "unavailable" {
		t.Fatal("viewer quota widened")
	}
	if a.reads != 1 {
		t.Fatal("full/duplicate reservation performed historical CAS I/O")
	}
	collapsed := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if collapsed.Status != "snapshot" || len(collapsed.Blocks) != 2 || collapsed.Blocks[0].Body != nil {
		t.Fatal("collapsed history released body")
	}
	expanded := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{v.Blocks[0].Display.BlockID})
	if expanded.Status != "snapshot" || expanded.Blocks[0].Body == nil || expanded.Blocks[0].Body.Text != "Alice closed reasoning" {
		t.Fatal("closed body failed protected expanded read")
	}
	a.mu.Lock()
	a.denyHistory = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("changed/deleted Primary anchor retained viewer authority")
	}
	if s.viewers[tokenFixtureV1] != nil {
		t.Fatal("invalid historical viewer not retired")
	}
}

func TestRealProviderReasoningFrameClosesIntoStrictHistoryV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real host logical unit rejected")
	}
	f := model.ProcessFrameV1{Source: "provider_reasoning", ReasoningForm: "summary", Kind: "start", ProviderSequence: 7, Attempt: 1, HostSegmentOrdinal: 1, NativeIndex: 2}
	p.FrameV1(f)
	f.Kind = "end"
	f.Text = "actual host summary"
	p.FrameV1(f)
	if !p.UnitV2(n, false) {
		t.Fatal("real unit closure failed")
	}
	p.FinishV1()
	v, err := s.FreezeSettledV1(p.BindingV2(), historyWitnessFixtureV1())
	if err != nil || len(v.Blocks) != 1 || v.Blocks[0].Display.Source.NativeIndex != 0 || v.Blocks[0].Display.NativeIndex != nil || v.Blocks[0].Display.Source.ReasoningForm != "summary" || v.Blocks[0].Display.Body == nil || v.Blocks[0].Display.Body.Text != "actual host summary" {
		t.Fatal("real provider source did not survive strict disk codec", err)
	}
}

func TestShutdownDrainRetiresReadersButRetainsOnlyRealClosedSourcesV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	p.UnitV2(n, true)
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "closed-in-drain"})
	p.FrameV1(model.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", ProviderSequence: 1, Attempt: 1, ContentClassification: "provider_text", Text: "unfinished draft must be absent"})
	s.BeginShutdownDrainV1()
	if !s.ShutdownDrainingV1() || s.BeginV1(p.BindingV2()) != nil || s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Status != "unavailable" {
		t.Fatal("drain retained reader or new producer authority")
	}
	if s.AuthorizeV1(context.Background(), "00000000-0000-4000-8000-000000000003", documentFixtureV1, p.turn.binding.ThreadID, p.turn.binding.TurnID).Status != "unavailable" {
		t.Fatal("drain admitted a fresh viewer")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "closed-in-drain", Text: "closed source survives cancellation"})
	p.FinishV1()
	record, err := s.FreezeSettledV1(p.BindingV2(), historyWitnessFixtureV1())
	if err != nil {
		t.Fatal(err)
	}
	body, err := display.SettledHistoryBytesV1(record)
	if err != nil || !strings.Contains(string(body), "closed source survives cancellation") || strings.Contains(string(body), "unfinished draft must be absent") {
		t.Fatal("drain lost a closed source or promoted unfinished draft", err)
	}
	s.Close()
	if s.ShutdownDrainingV1() {
		t.Fatal("closed owner still grants shutdown settlement")
	}
	if _, err := s.FreezeSettledV1(p.BindingV2(), historyWitnessFixtureV1()); err == nil {
		t.Fatal("closed capture owner recovered source")
	}
}

type blockedHistoryThreadAccessV1 struct {
	*historyAccessFixtureV1
	entered chan struct{}
	resume  chan struct{}
}

func (a *blockedHistoryThreadAccessV1) ReadSettledHistoryV1(ctx context.Context, thread, turn string) (display.SettledHistoryV1, display.SettledHistoryAnchorV1, error) {
	close(a.entered)
	select {
	case <-a.resume:
		return a.historyAccessFixtureV1.ReadSettledHistoryV1(ctx, thread, turn)
	case <-ctx.Done():
		return display.SettledHistoryV1{}, display.SettledHistoryAnchorV1{}, port.ErrUnavailableV1
	}
}

func TestDeletedThreadRetiresPendingHistoricalReservationOnlyV1(t *testing.T) {
	record := closedHistoryFixtureV1(t)
	anchor, err := display.SettledHistoryAnchorForV1(record)
	if err != nil {
		t.Fatal(err)
	}
	a := &blockedHistoryThreadAccessV1{historyAccessFixtureV1: &historyAccessFixtureV1{record: record, anchor: anchor}, entered: make(chan struct{}), resume: make(chan struct{})}
	s := NewServiceV1(a, DefaultLimitsV1())
	defer s.Close()
	other := port.BindingV1{ThreadID: "thread-other", TurnID: "turn-other", Principal: record.Principal, Original: security.NewTurnSecurityContext(security.TurnSecurityContextInput{ThreadID: "thread-other", TurnID: "turn-other", WorkspaceRealPath: "/workspace", TenantID: "local", UserID: "local"})}
	p := s.BeginV1(other)
	if p == nil {
		t.Fatal("other thread producer unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan ResponseV1, 1)
	go func() { done <- s.AuthorizeV1(ctx, tokenFixtureV1, documentFixtureV1, record.ThreadID, record.TurnID) }()
	select {
	case <-a.entered:
	case <-ctx.Done():
		t.Fatal("historical reservation did not reach CAS seam")
	}
	s.RevokeThreadV1(record.ThreadID)
	close(a.resume)
	select {
	case got := <-done:
		if got.Status != "unavailable" {
			t.Fatal("late CAS result recreated deleted-thread viewer")
		}
	case <-ctx.Done():
		t.Fatal("revoked historical request did not return")
	}
	s.mu.Lock()
	if len(s.viewers) != 0 || len(s.turns) != 1 || !p.validLockedV1() {
		s.mu.Unlock()
		t.Fatal("retirement retained pending viewer or affected other thread")
	}
	s.mu.Unlock()
	s.RevokeThreadV1(other.ThreadID)
	s.mu.Lock()
	valid := p.validLockedV1()
	s.mu.Unlock()
	if valid {
		t.Fatal("deleted live thread retained producer authority")
	}
}
