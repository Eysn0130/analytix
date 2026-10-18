package processdisplay

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	providerstream "analytix.local/runtime-go/internal/adapters/outbound/provider/stream"
	continuation "analytix.local/runtime-go/internal/domain/continuation"
	identity "analytix.local/runtime-go/internal/domain/identity"
	job "analytix.local/runtime-go/internal/domain/job"
	model "analytix.local/runtime-go/internal/domain/model"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	security "analytix.local/runtime-go/internal/domain/security"
	streamframe "analytix.local/runtime-go/internal/domain/streamframe"
	toolcall "analytix.local/runtime-go/internal/domain/toolcall"
	toolresult "analytix.local/runtime-go/internal/domain/toolresult"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
)

type accessFixtureV1 struct {
	mu           sync.Mutex
	held         int
	denied       bool
	draftDenied  bool
	draftEntered chan struct{}
	draftWait    chan struct{}
}

func (a *accessFixtureV1) AcquireCurrent(ctx context.Context, b port.BindingV1) (port.AuthorizedV1, func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.denied || ctx.Err() != nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	a.held++
	return port.AuthorizedV1{Binding: b}, func() { a.mu.Lock(); a.held--; a.mu.Unlock() }, nil
}
func (a *accessFixtureV1) ValidateCurrent(ctx context.Context, _ port.AuthorizedV1) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.denied || ctx.Err() != nil {
		return port.ErrUnavailableV1
	}
	return nil
}
func (a *accessFixtureV1) ValidateAssistantDraftCurrent(ctx context.Context, _ port.BindingV1) error {
	a.mu.Lock()
	denied, entered, wait := a.denied || a.draftDenied, a.draftEntered, a.draftWait
	a.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if wait != nil {
		select {
		case <-wait:
		case <-ctx.Done():
			return port.ErrUnavailableV1
		}
	}
	if denied || ctx.Err() != nil {
		return port.ErrUnavailableV1
	}
	return nil
}
func (a *accessFixtureV1) ValidateToolCurrent(ctx context.Context, _ port.BindingV1, _ display.ToolBindingV1) error {
	return a.ValidateCurrent(ctx, port.AuthorizedV1{})
}
func bindingFixtureV1(t *testing.T) port.BindingV1 {
	t.Helper()
	principal, err := identity.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	frozen := security.NewTurnSecurityContext(security.TurnSecurityContextInput{ThreadID: "thread-process", TurnID: "turn-process", WorkspaceRealPath: "/workspace", TenantID: "local", UserID: "local"})
	return port.BindingV1{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Principal: principal, Original: frozen}
}

const tokenFixtureV1 = "00000000-0000-4000-8000-000000000001"
const documentFixtureV1 = "00000000-0000-4000-8000-000000000002"

func serviceFixtureV1(t *testing.T) (*ServiceV1, *ProducerV1, *accessFixtureV1) {
	t.Helper()
	authority := &accessFixtureV1{}
	s := NewServiceV1(authority, DefaultLimitsV1())
	p := s.BeginV1(bindingFixtureV1(t), "process-test-known-credential")
	if p == nil {
		t.Fatal("host binding denied")
	}
	if got := s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, p.turn.binding.ThreadID, p.turn.binding.TurnID); got.Status != "authorized" {
		t.Fatalf("authorize=%s", got.Status)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, p, authority
}

func TestCompleteRunningInputSharesStartedAnchorExpansionAndRetirementV2(t *testing.T) {
	for _, name := range []string{"read", "todo_write", "task", "bash", "web_fetch"} {
		t.Run(name, func(t *testing.T) {
			s, p, a := serviceFixtureV1(t)
			n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
			call := security.HostToolCallIDPrefixV1 + strings.Repeat("8", security.HostToolCallIDEntropyBytesV1*2)
			item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
			raw := json.RawMessage(" {\n\"number\":9007199254740993,\"case\":\"Alice\"\n} ")
			tool := display.ToolBindingV1{CallID: call, ToolName: name, GrantID: strings.Repeat("a", 64), InputArgsHash: security.CanonicalJSONHash(raw)}
			if !p.ToolReadyV2(n, call, item) || p.ToolInputV2(call, raw, tool) {
				t.Fatal("ready receipt became a running input")
			}
			if !p.ToolMetadataV2(n, call, item, "", 12) || !p.ToolInputV2(call, raw, tool) {
				t.Fatal("real running input rejected")
			}
			collapsed := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			if len(collapsed.Blocks) != 1 || collapsed.Blocks[0].Input != nil {
				t.Fatal("collapsed raw input escaped")
			}
			id := collapsed.Blocks[0].BlockID
			expanded := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if expanded.Blocks[0].Input == nil || expanded.Blocks[0].Input.InputJSON != string(raw) || expanded.Blocks[0].Source.ToolResultItemID != "" || expanded.Blocks[0].CaptureDigest != "" {
				t.Fatal("raw input lost truth or claimed settlement")
			}
			expanded.Blocks[0].Input.InputJSON = "replacement"
			again := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if again.Blocks[0].Input.InputJSON != string(raw) {
				t.Fatal("response borrowed owner input")
			}
			wrong := tool
			wrong.InputArgsHash = strings.Repeat("b", 64)
			if p.ToolInputV2(call, raw, wrong) {
				t.Fatal("substituted hash accepted")
			}
			result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
			if !p.ToolMetadataV2(n, call, item, result, 12) || p.ToolInputV2(call, raw, tool) {
				t.Fatal("sealed input reopened")
			}
			retired := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if retired.Status != "snapshot" || len(retired.Units) != 1 || len(retired.Units[0].Calls) != 1 || p.turn.rawBytes != 0 || retired.Units[0].Calls[0].ToolResultItemID != result {
				t.Fatal("settlement retained live content or raw quota")
			}
			if name == "bash" || name == "web_fetch" {
				if len(retired.Blocks) != 1 || retired.Blocks[0].Input != nil || retired.Blocks[0].State != "unknown" || retired.Blocks[0].Source.ToolResultItemID != result {
					t.Fatal("existing typed source did not retire")
				}
			} else if len(retired.Blocks) != 0 || p.turn.displayBytes != 0 {
				t.Fatal("settled input invented a process observation")
			}
			if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{strings.Repeat("f", 32)}).Status != "unavailable" || s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id}).Status != "snapshot" {
				t.Fatal("retired selection admitted an unknown ID or lost its existing pane lease")
			}
			a.mu.Lock()
			a.denied = true
			a.mu.Unlock()
			if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id}).Status != "unavailable" {
				t.Fatal("revoked lease remained readable")
			}
		})
	}
}

func TestCompleteInputLimitsAndWebUpgradeReleaseTheSameQuotaV2(t *testing.T) {
	for _, test := range []struct {
		name, arguments string
		upgrade         bool
	}{{"todo_write", `{"text":"` + strings.Repeat("x", display.ToolInputJSONByteLimitV2) + `"}`, false}, {"web_fetch", `{"url":"https://example.invalid/Alice"}`, true}, {"web_fetch", `{"url":"https://example.invalid/Alice","api_key":"synthetic-private-key"}`, false}} {
		t.Run(test.name+fmt.Sprint(test.upgrade), func(t *testing.T) {
			s, p, _ := serviceFixtureV1(t)
			n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
			call := security.HostToolCallIDPrefixV1 + strings.Repeat("7", security.HostToolCallIDEntropyBytesV1*2)
			item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
			raw := json.RawMessage(test.arguments)
			tool := display.ToolBindingV1{CallID: call, ToolName: test.name, GrantID: strings.Repeat("a", 64), InputArgsHash: security.CanonicalJSONHash(raw)}
			if !p.ToolReadyV2(n, call, item) || !p.ToolMetadataV2(n, call, item, "", 12) || !p.ToolInputV2(call, raw, tool) {
				t.Fatal("real shared input fixture rejected")
			}
			if test.name == "web_fetch" && p.WebInputV1(call, "https://example.invalid/Alice", tool) != test.upgrade {
				t.Fatal("URL upgrade changed the credential floor")
			}
			meta := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			expanded := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{meta.Blocks[0].BlockID})
			if test.name == "todo_write" && (expanded.Blocks[0].Input == nil || expanded.Blocks[0].Input.Reason != "limit" || expanded.Blocks[0].Input.InputJSON != "") {
				t.Fatal("limit manufactured a partial input")
			}
			result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
			if !p.ToolMetadataV2(n, call, item, result, 12) || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
				t.Fatal("shared input/URL quota survived settlement")
			}
		})
	}
}

func TestCompleteInputConvertsToActualTypedMutationOnlyAfterResultAnchorV2(t *testing.T) {
	for _, kind := range []string{"write", "edit"} {
		t.Run(kind, func(t *testing.T) {
			s, p, _ := serviceFixtureV1(t)
			n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
			call := security.HostToolCallIDPrefixV1 + strings.Repeat("6", security.HostToolCallIDEntropyBytesV1*2)
			item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
			arguments := json.RawMessage(`{"path":"Alice.ts","content":"actual"}`)
			tool := display.ToolBindingV1{CallID: call, ToolName: kind, GrantID: strings.Repeat("a", 64), InputArgsHash: security.CanonicalJSONHash(arguments)}
			if !p.ToolReadyV2(n, call, item) || !p.ToolMetadataV2(n, call, item, "", 12) || !p.ToolInputV2(call, arguments, tool) {
				t.Fatal("fixture input rejected")
			}
			meta := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			id := meta.Blocks[0].BlockID
			result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
			settled := tool
			settled.ResultItemID = result
			settled.InputArgsHash = ""
			raw := display.RawCaptureV1{Kind: kind, Complete: true, Status: "completed", Body: "actual typed result", Before: "before", After: "after", Path: "Alice.ts", ToolBinding: settled}
			p.ToolV1(call, raw)
			if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id}).Blocks[0].Body != nil {
				t.Fatal("effect observation invented call settlement")
			}
			if !p.ToolMetadataV2(n, call, item, result, 12) {
				t.Fatal("actual result anchor rejected")
			}
			p.ToolV1(call, raw)
			capture := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id}).Blocks[0]
			if capture.BlockID != id || capture.Kind != kind || capture.Input != nil || capture.Body == nil || capture.CaptureDigest == "" || capture.Source.ToolResultItemID != result {
				t.Fatal("typed result did not replace its own live slot")
			}
		})
	}
}
func frameFixtureV1(kind, text string) model.ProcessFrameV1 {
	return model.ProcessFrameV1{Kind: kind, Attempt: 1, NativeIndex: 3, ProviderSequence: 7, Text: text}
}

func TestNativeBlockIsNotVisibleBeforeRealEndV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("delta", "Alice: process-test-known-credential"))
	response := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(response.Blocks) != 1 || response.Blocks[0].State != "running" || response.Blocks[0].Preview != "" || response.Blocks[0].Body != nil {
		t.Fatal("partial native reasoning escaped")
	}
	id := response.Blocks[0].BlockID
	p.FrameV1(frameFixtureV1("end", ""))
	preview := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if preview.Blocks[0].Body != nil || !strings.Contains(preview.Blocks[0].Preview, "Alice") {
		t.Fatal("pane preview lease failed")
	}
	body := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
	if body.Blocks[0].Body == nil || strings.Contains(body.Blocks[0].Body.Text, "process-test-known-credential") {
		t.Fatal("completed display copy missing or unsafe")
	}
	body.Blocks[0].Body.Text = "mutated copy"
	again := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
	if again.Blocks[0].Body.Text == "mutated copy" {
		t.Fatal("response aliases the owner")
	}
	if got := s.OpenCaptureV1(context.Background(), tokenFixtureV1, id, preview.Blocks[0].CaptureDigest); got.Status != "capture" {
		t.Fatal("bound read-only capture unavailable")
	}
	if got := s.OpenCaptureV1(context.Background(), tokenFixtureV1, id, strings.Repeat("0", 64)); got.Status != "unavailable" {
		t.Fatal("substituted digest accepted")
	}
}

func TestLongPollReleasesScopeAndWakesWithoutLostRevisionV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	result := make(chan ResponseV1, 1)
	go func() { result <- s.ReadV1(context.Background(), tokenFixtureV1, before.Revision, time.Second, nil) }()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		v := s.viewers[tokenFixtureV1]
		waiting := v.polling
		s.mu.Unlock()
		a.mu.Lock()
		held := a.held
		a.mu.Unlock()
		if waiting && held == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("poll retained authority while waiting")
		}
		time.Sleep(time.Millisecond)
	}
	if got := s.ReadV1(context.Background(), tokenFixtureV1, before.Revision, 0, nil); got.Status != "busy" {
		t.Fatal("concurrent poll admitted")
	}
	p.FrameV1(frameFixtureV1("start", ""))
	select {
	case got := <-result:
		if got.Status != "snapshot" || got.Revision == before.Revision {
			t.Fatal("producer wake was lost")
		}
	case <-time.After(time.Second):
		t.Fatal("producer wake blocked")
	}
}

func TestPendingPollFreshAuthorityRevocationAndABAStayClosedV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	result := make(chan ResponseV1, 1)
	go func() { result <- s.ReadV1(context.Background(), tokenFixtureV1, before.Revision, 25*time.Second, nil) }()
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	select {
	case got := <-result:
		if got.Status != "unavailable" || got.Blocks != nil {
			t.Fatal("revoked authority returned a copy")
		}
	case <-time.After(time.Second):
		t.Fatal("principal revocation waited for long-poll timeout")
	}
	a.mu.Lock()
	a.denied = false
	a.mu.Unlock()
	if s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, p.turn.binding.ThreadID, p.turn.binding.TurnID).Status != "unavailable" {
		t.Fatal("revoked token resurrected")
	}
}

func TestReadsCannotRenewRetentionOrReviveOldProducerV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	now := time.Now()
	s.mu.Lock()
	s.now = func() time.Time { return now }
	p.turn.activity = now
	s.limits.IdleTTL = time.Second
	s.mu.Unlock()
	p.FrameV1(frameFixtureV1("start", ""))
	advanceClockFixtureV1(s, &now, 900*time.Millisecond)
	if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Status != "snapshot" {
		t.Fatal("premature expiry")
	}
	advanceClockFixtureV1(s, &now, 200*time.Millisecond)
	if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Status != "unavailable" {
		t.Fatal("poll renewed producer retention")
	}
	p.FrameV1(frameFixtureV1("delta", "old producer"))
	s.mu.Lock()
	remaining := len(s.turns)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatal("expired producer recreated its incarnation")
	}
	newProducer := s.BeginV1(bindingFixtureV1(t))
	if newProducer == nil || newProducer.turn == p.turn {
		t.Fatal("fresh host producer did not get new incarnation")
	}
	if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Status != "unavailable" {
		t.Fatal("old viewer attached to successor")
	}
}

func TestQuotaAndRetryOnlyDegradeDisplayV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	s.mu.Lock()
	s.limits.RawBytes = 8
	s.mu.Unlock()
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("delta", "private oversized body"))
	p.FrameV1(frameFixtureV1("end", ""))
	copy := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(copy.Blocks) != 1 || copy.Blocks[0].Classification != "withheld" || copy.Blocks[0].Preview != "" {
		t.Fatal("quota exposed partial prefix")
	}
	reset := frameFixtureV1("reset", "")
	reset.Attempt = 2
	p.FrameV1(reset)
	late := frameFixtureV1("delta", "late old attempt")
	p.FrameV1(late)
	copy = s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if copy.Blocks[0].State != "superseded" || copy.Blocks[0].Body != nil {
		t.Fatal("retry exposed superseded generation")
	}
	encoded, _ := json.Marshal(copy)
	if strings.Contains(string(encoded), "private oversized body") || strings.Contains(string(encoded), "late old attempt") {
		t.Fatal("withheld private bytes entered response")
	}
}

func TestMutationEffectRemainsTrueWhenSettlementFailsV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	applied := true
	toolCaptureFixtureV2(t, p, security.HostToolCallIDPrefixV1+strings.Repeat("a", security.HostToolCallIDEntropyBytesV1*2), display.RawCaptureV1{Kind: "write", Complete: true, Status: "failed", SettlementStatus: "failed", EffectApplied: &applied, Before: "old", After: "new", Diff: "real diff", AddedLines: 1, RemovedLines: 1})
	copy := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(copy.Blocks) != 1 || copy.Blocks[0].State != "failed" || copy.Blocks[0].EffectApplied == nil || !*copy.Blocks[0].EffectApplied {
		t.Fatal("settlement failure erased confirmed file effect")
	}
	if copy.Blocks[0].Body != nil {
		t.Fatal("collapsed tool leaked expanded body")
	}
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "99999", 0, nil); got.Status != "unavailable" {
		t.Fatal("future revision accepted")
	}
}

func TestFinishCannotRenewExpiredProducerAndClearsIncompleteNativeV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	now := time.Now()
	s.mu.Lock()
	s.now = func() time.Time { return now }
	s.limits.IdleTTL = time.Second
	p.turn.activity = now
	s.mu.Unlock()
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("delta", "incomplete private text"))
	advanceClockFixtureV1(s, &now, 2*time.Second)
	p.FinishV1()
	if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Status != "unavailable" || len(p.secrets) != 0 {
		t.Fatal("Finish revived expired display or retained its known credentials")
	}
	s2, p2, _ := serviceFixtureV1(t)
	p2.FrameV1(frameFixtureV1("start", ""))
	p2.FrameV1(frameFixtureV1("delta", "unfinished native text"))
	p2.FinishV1()
	got := s2.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(got.Blocks) != 1 || got.Blocks[0].State != "unknown" || got.Blocks[0].Preview != "" || p2.turn.rawBytes != 0 {
		t.Fatal("Finish did not close and wipe unfinished native capture")
	}
}

type blockedAuthorizeAccessV1 struct {
	accessFixtureV1
	entered chan struct{}
	resume  chan struct{}
}

func (a *blockedAuthorizeAccessV1) AcquireCurrent(ctx context.Context, b port.BindingV1) (port.AuthorizedV1, func(), error) {
	close(a.entered)
	<-a.resume
	return a.accessFixtureV1.AcquireCurrent(ctx, b)
}
func TestPendingAuthorizeCannotOutliveBoundedRevocationTombstoneV1(t *testing.T) {
	a := &blockedAuthorizeAccessV1{entered: make(chan struct{}), resume: make(chan struct{})}
	s := NewServiceV1(a, DefaultLimitsV1())
	defer s.Close()
	b := bindingFixtureV1(t)
	p := s.BeginV1(b)
	if p == nil {
		t.Fatal("producer unavailable")
	}
	result := make(chan ResponseV1, 1)
	go func() {
		result <- s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, b.ThreadID, b.TurnID)
	}()
	<-a.entered
	s.RevokeV1(tokenFixtureV1)
	for i := 0; i < DefaultLimitsV1().MaxTombstones+1; i++ {
		s.RevokeV1(fmt.Sprintf("10000000-0000-4000-8000-%012x", i))
	}
	close(a.resume)
	select {
	case got := <-result:
		if got.Status != "unavailable" {
			t.Fatal("evicted tombstone resurrected a pending native-document viewer")
		}
	case <-time.After(time.Second):
		t.Fatal("authorization did not complete")
	}
}

func TestInputOutcomePointersCannotMutateFrozenCaptureV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	effect := true
	exit := 7
	toolCaptureFixtureV2(t, p, security.HostToolCallIDPrefixV1+strings.Repeat("b", security.HostToolCallIDEntropyBytesV1*2), display.RawCaptureV1{Kind: "write", Complete: true, Status: "failed", SettlementStatus: "failed", EffectApplied: &effect, Before: "before", After: "after", Diff: "actual diff"})
	toolCaptureFixtureV2(t, p, security.HostToolCallIDPrefixV1+strings.Repeat("c", security.HostToolCallIDEntropyBytesV1*2), display.RawCaptureV1{Kind: "shell", Complete: true, Status: "failed", ExitCode: &exit, Body: "actual output"})
	effect = false
	exit = 0
	got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(got.Blocks) != 2 || got.Blocks[0].EffectApplied == nil || !*got.Blocks[0].EffectApplied || got.Blocks[1].ExitCode == nil || *got.Blocks[1].ExitCode != 7 {
		t.Fatal("producer pointers changed the frozen display without a revision")
	}
}

// The service janitor reads its clock under the owner mutex, so fixture clock
// movement must use the same gate rather than race a closure-captured value.
func advanceClockFixtureV1(s *ServiceV1, now *time.Time, by time.Duration) {
	s.mu.Lock()
	*now = now.Add(by)
	s.mu.Unlock()
}

func TestJanitorExpiresRawAndDisplayWithoutAPIActivityV1(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprintf("terminal=%v", terminal), func(t *testing.T) {
			limits := DefaultLimitsV1()
			limits.IdleTTL, limits.TerminalTTL = 100*time.Millisecond, 100*time.Millisecond
			s := NewServiceV1(&accessFixtureV1{}, limits)
			defer s.Close()
			b := bindingFixtureV1(t)
			p := s.BeginV1(b)
			if p == nil {
				t.Fatal("host producer unavailable")
			}
			p.FrameV1(frameFixtureV1("start", ""))
			p.FrameV1(frameFixtureV1("delta", "completed local body"))
			p.FrameV1(frameFixtureV1("end", ""))
			open := frameFixtureV1("start", "")
			open.NativeIndex = 4
			p.FrameV1(open)
			open.Kind, open.Text = "delta", "unfinished local raw"
			p.FrameV1(open)
			if terminal {
				p.FinishV1()
			}
			if s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, b.ThreadID, b.TurnID).Status != "authorized" {
				t.Fatal("native viewer unavailable before expiry")
			}
			s.mu.Lock()
			retired := p.turn
			blocks := append([]*rawBlockV1(nil), retired.blocks...)
			if len(blocks) != 2 || blocks[0].value.Body == nil || !terminal && blocks[1].raw == "" {
				s.mu.Unlock()
				t.Fatal("fixture did not retain the real completed body and active raw")
			}
			s.mu.Unlock()
			// Observe private state only. No read, producer, revoke or sweep API
			// is allowed to supply the expiry side effect under test.
			deadline := time.Now().Add(2 * time.Second)
			for {
				s.mu.Lock()
				expired := len(s.turns) == 0 && len(s.viewers) == 0 && retired.blocks == nil && retired.rawBytes == 0 && retired.displayBytes == 0 && !s.janitorRunning
				for _, block := range blocks {
					expired = expired && block.raw == "" && block.value.Body == nil && block.value.Preview == ""
				}
				s.mu.Unlock()
				if expired {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("idle service retained private raw/display bytes past its bounded janitor expiry")
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}

func TestCloseWaitsForJanitorAndIsConcurrentIdempotentV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("delta", "private raw at shutdown"))
	finished := make(chan error, 4)
	for i := 0; i < cap(finished); i++ {
		go func() { finished <- s.Close() }()
	}
	for i := 0; i < cap(finished); i++ {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("Close deadlocked while waiting for the owner janitor")
		}
	}
	select {
	case <-s.janitorDone:
	default:
		t.Fatal("Close returned with a live owner janitor")
	}
	s.mu.Lock()
	closed := s.closed && len(s.turns) == 0 && len(s.viewers) == 0 && p.turn.blocks == nil
	s.mu.Unlock()
	if !closed || s.BeginV1(bindingFixtureV1(t)) != nil {
		t.Fatal("quiescent Close retained private captures or admitted a new producer")
	}
}

func TestJanitorIsLazyAndRestartsAfterEmptyRetentionV1(t *testing.T) {
	limits := DefaultLimitsV1()
	limits.IdleTTL, limits.TerminalTTL = 50*time.Millisecond, 50*time.Millisecond
	s := NewServiceV1(&accessFixtureV1{}, limits)
	defer s.Close()
	s.mu.Lock()
	lazy := !s.janitorRunning
	initialDone := s.janitorDone
	s.mu.Unlock()
	select {
	case <-initialDone:
	default:
		t.Fatal("unused service owns an active janitor")
	}
	if !lazy {
		t.Fatal("janitor started without a host turn")
	}
	first := s.BeginV1(bindingFixtureV1(t))
	if first == nil {
		t.Fatal("first producer unavailable")
	}
	s.mu.Lock()
	firstDone := s.janitorDone
	s.mu.Unlock()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("empty-retention janitor did not exit without another API call")
	}
	second := s.BeginV1(bindingFixtureV1(t))
	if second == nil || second.turn == first.turn {
		t.Fatal("new host turn failed to receive a fresh incarnation")
	}
	s.mu.Lock()
	restarted := s.janitorRunning && s.janitorDone != firstDone
	s.mu.Unlock()
	if !restarted {
		t.Fatal("new host turn did not restart its sole active retention worker")
	}
}

func TestIndependentPaneLeasesSameDocumentV2(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	tokenB := "00000000-0000-4000-8000-000000000003"
	if got := s.AuthorizeV1(context.Background(), tokenB, documentFixtureV1, p.turn.binding.ThreadID, p.turn.binding.TurnID); got.Status != "authorized" {
		t.Fatal(got.Status)
	}
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "snapshot" {
		t.Fatalf("pane A retired by B: %s", got.Status)
	}
	s.RevokeV1(tokenB)
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "snapshot" {
		t.Fatalf("pane A retired by B close: %s", got.Status)
	}
}

func toolCaptureFixtureV2(t *testing.T, p *ProducerV1, call string, raw display.RawCaptureV1) {
	t.Helper()
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolReadyV2(n, call, item) || !p.ToolMetadataV2(n, call, item, result, 0) {
		t.Fatal("host fixture anchors rejected")
	}
	raw.ToolBinding.CallID = call
	raw.ToolBinding.ResultItemID = result
	p.ToolV1(call, raw)
}
func TestSourceCorrelationReplayConflictAndReverseCompletionV2(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("unit rejected")
	}
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("end", "native complete"))
	calls := []string{security.HostToolCallIDPrefixV1 + strings.Repeat("d", security.HostToolCallIDEntropyBytesV1*2), security.HostToolCallIDPrefixV1 + strings.Repeat("e", security.HostToolCallIDEntropyBytesV1*2)}
	for _, call := range calls {
		if !p.ToolReadyV2(n, call, toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)) {
			t.Fatal("real ready rejected")
		}
	}
	before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(before.Units) != 1 || len(before.Blocks) != 1 || before.Units[0].Placement != "anchored" {
		t.Fatal("unit/calls invented captures")
	}
	// Started callbacks arrive backwards; their actual committed sequences do
	// not change call-ready order or native start ordinal.
	for i := 1; i >= 0; i-- {
		call := calls[i]
		item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
		result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
		if !p.ToolMetadataV2(n, call, item, result, uint64(20+i)) {
			t.Fatal("actual result rejected")
		}
		p.ToolV1(call, display.RawCaptureV1{Kind: "shell", Complete: true, Status: "completed", Body: "actual same-name output", ToolBinding: display.ToolBindingV1{CallID: call, ResultItemID: result, ToolName: "bash"}})
	}
	got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(got.Blocks) != 3 || got.Units[0].Calls[0].CallID != calls[0] || got.Units[0].Calls[1].CallID != calls[1] || got.Blocks[0].Source.SourceOrdinal != before.Blocks[0].Source.SourceOrdinal {
		t.Fatal("completion or started callbacks reordered unit")
	}
	ordinal := got.Blocks[0].Source.SourceOrdinal
	ids := []string{got.Blocks[0].BlockID, got.Blocks[1].BlockID, got.Blocks[2].BlockID}
	call := calls[0]
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolReadyV2(n, call, item) || !p.ToolMetadataV2(n, call, item, result, 20) {
		t.Fatal("exact replay rejected")
	}
	wrong := n
	wrong.ProviderCallSequence++
	if p.ToolReadyV2(wrong, call, item) || p.ToolMetadataV2(n, call, item, "wrong-result", 20) || p.ToolMetadataV2(n, call, item, result, 99) {
		t.Fatal("conflicting registration accepted")
	}
	p.ToolV1(call, display.RawCaptureV1{Kind: "shell", Complete: true, Status: "completed", Body: "duplicate must not replace"})
	replay := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, ids)
	if len(replay.Units) != 1 || len(replay.Blocks) != 3 || replay.Blocks[0].Source.SourceOrdinal != ordinal {
		t.Fatal("replay grew or moved the tree")
	}
	for i, b := range replay.Blocks {
		if b.BlockID != ids[i] {
			t.Fatal("replay replaced a row")
		}
	}
	replay.Units[0].Calls[0].CallID = "mutated"
	if s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Units[0].Calls[0].CallID != call {
		t.Fatal("response aliases call anchors")
	}
	p.UnitV2(n, false)
	next := n
	next.ProviderCallSequence++
	if !p.UnitV2(next, true) {
		t.Fatal("next unit rejected")
	}
	tail := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil).Units[1]
	if len(tail.PrecedingCalls) != 2 || tail.Placement != "anchored" {
		t.Fatal("true preceding boundary absent")
	}
}
func TestPendingAuthorizeSiblingRevokeQuotaAndNoResurrectionV2(t *testing.T) {
	a := &blockedAuthorizeAccessV1{entered: make(chan struct{}), resume: make(chan struct{})}
	limits := DefaultLimitsV1()
	limits.MaxViewers = 1
	s := NewServiceV1(a, limits)
	defer s.Close()
	b := bindingFixtureV1(t)
	s.BeginV1(b)
	got := make(chan ResponseV1, 1)
	go func() {
		got <- s.AuthorizeV1(context.Background(), tokenFixtureV1, documentFixtureV1, b.ThreadID, b.TurnID)
	}()
	<-a.entered
	sibling := "00000000-0000-4000-8000-000000000003"
	// Authorizing reservations consume the same global quota; unrelated revoke
	// must not invalidate the pending pointer reservation.
	if s.AuthorizeV1(context.Background(), sibling, documentFixtureV1, b.ThreadID, b.TurnID).Status != "unavailable" {
		t.Fatal("authorizing reservation bypassed quota")
	}
	s.RevokeV1(sibling)
	close(a.resume)
	if (<-got).Status != "authorized" {
		t.Fatal("sibling revoke cancelled unrelated authorization")
	}
}

func TestNativeCoordinateCannotBeReboundByConflictingOrLateNamespaceV2(t *testing.T) {
	for _, frameFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(frameFirst), func(t *testing.T) {
			s, p, _ := serviceFixtureV1(t)
			n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
			if !frameFirst && !p.UnitV2(n, true) {
				t.Fatal("initial unit rejected")
			}
			p.FrameV1(frameFixtureV1("start", ""))
			p.FrameV1(frameFixtureV1("end", "real completed native"))
			before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			if frameFirst {
				if p.UnitV2(n, true) {
					t.Fatal("late namespace replaced existing unanchored native coordinate")
				}
			} else {
				if !p.UnitV2(n, true) {
					t.Fatal("exact unit replay rejected")
				}
				manifest, err := job.NewDelegatedToolManifestV1([]string{"read"}, security.SHA256Hex([]byte("schema")), security.SHA256Hex([]byte("mcp")))
				if err != nil {
					t.Fatal(err)
				}
				child := continuation.NewProviderContinuationNamespaceV1("subagent", "child-a", 7, manifest)
				if p.UnitV2(child, true, "read") {
					t.Fatal("conflicting valid child namespace replaced native coordinate")
				}
			}
			p.FrameV1(frameFixtureV1("start", ""))
			p.FrameV1(frameFixtureV1("end", "duplicate must not be accepted"))
			after := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected namespace or exact replay changed unit, ordinal, row or revision")
			}
		})
	}
}

func TestLateToolMetadataEnrichesExistingPrecedingAnchorV2(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	call := security.HostToolCallIDPrefixV1 + strings.Repeat("f", security.HostToolCallIDEntropyBytesV1*2)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.UnitV2(n, true) || !p.ToolReadyV2(n, call, item) {
		t.Fatal("actual host anchor rejected")
	}
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("end", ""))
	if !p.UnitV2(n, false) {
		t.Fatal("unit completion rejected")
	}
	next := n
	next.ProviderCallSequence++
	if !p.UnitV2(next, true) {
		t.Fatal("next actual unit rejected")
	}
	before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(before.Units) != 2 || len(before.Units[1].PrecedingCalls) != 1 {
		t.Fatal("real preceding anchor missing")
	}
	if !p.ToolMetadataV2(n, call, item, result, 31) {
		t.Fatal("late actual metadata rejected")
	}
	after := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if after.Units[0].Calls[0].ToolResultItemID != result || after.Units[1].PrecedingCalls[0].ToolResultItemID != result || after.Units[1].PrecedingCalls[0].StartedEventSeq != "31" {
		t.Fatal("late settlement/start metadata did not enrich the existing preceding anchor")
	}
	if after.Revision == before.Revision || len(after.Blocks) != len(before.Blocks) {
		t.Fatal("metadata did not wake without inventing capture rows")
	}
	for i := range before.Units {
		if after.Units[i].UnitID != before.Units[i].UnitID || after.Units[i].ProducerID != before.Units[i].ProducerID || after.Units[i].Placement != before.Units[i].Placement {
			t.Fatal("metadata replaced unit identity or placement")
		}
	}
	for i := range before.Blocks {
		if after.Blocks[i].BlockID != before.Blocks[i].BlockID || after.Blocks[i].Source != before.Blocks[i].Source {
			t.Fatal("metadata moved an existing source ordinal")
		}
	}
	if !p.ToolMetadataV2(n, call, item, result, 31) || p.ToolMetadataV2(n, call, item, result, 32) || p.ToolMetadataV2(n, call, item, "conflicting-result", 31) {
		t.Fatal("exact replay or identity conflict rule changed")
	}
	replay := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if !reflect.DeepEqual(after, replay) {
		t.Fatal("exact replay/conflict changed retained snapshot")
	}
}

func TestSettlementMetadataReplayDoesNotReviveProducerOrCreateTurnV2(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	call := security.HostToolCallIDPrefixV1 + strings.Repeat("a", security.HostToolCallIDEntropyBytesV1*2)
	binding := p.turn.binding
	item := toolcall.ToolCallItemIDV1(binding.TurnID, call)
	result := toolresult.ToolResultItemIDV1(binding.TurnID, call)
	if !p.UnitV2(n, true) || !p.ToolReadyV2(n, call, item) {
		t.Fatal("real ready rejected")
	}
	p.FinishV1()
	before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	s.mu.Lock()
	activity := p.turn.activity
	s.mu.Unlock()
	if !s.ToolMetadataV2(binding, n, call, item, result, 37) {
		t.Fatal("existing settled anchor rejected")
	}
	after := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if after.Revision == before.Revision || after.Units[0].Calls[0].ToolResultItemID != result {
		t.Fatal("settlement metadata did not enrich existing anchor")
	}
	wrong := binding
	wrong.Original.ContextEpoch++
	if s.ToolMetadataV2(wrong, n, call, item, result, 37) || s.ToolMetadataV2(binding, n, call, item, result, 38) || !s.ToolMetadataV2(binding, n, call, item, result, 37) {
		t.Fatal("scope/conflict/exact replay check changed")
	}
	if !reflect.DeepEqual(after, s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)) {
		t.Fatal("metadata replay changed snapshot")
	}
	s.mu.Lock()
	if len(p.turn.handles) != 0 || !p.turn.terminal || p.turn.activity != activity {
		s.mu.Unlock()
		t.Fatal("metadata revived producer or extended retention")
	}
	s.mu.Unlock()
	other := call[:len(call)-1] + "b"
	if s.ToolMetadataV2(binding, n, other, toolcall.ToolCallItemIDV1(binding.TurnID, other), toolresult.ToolResultItemIDV1(binding.TurnID, other), 0) {
		t.Fatal("metadata created an unregistered call")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s.ToolMetadataV2(binding, n, call, item, result, 37) {
		t.Fatal("metadata resurrected retired turn")
	}
}

func TestConflictingPrecedingMetadataRejectsAtomicEnrichmentV2(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	call := security.HostToolCallIDPrefixV1 + strings.Repeat("b", security.HostToolCallIDEntropyBytesV1*2)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolReadyV2(n, call, item) {
		t.Fatal("real ready rejected")
	}
	next := n
	next.ProviderCallSequence++
	if !p.UnitV2(next, true) {
		t.Fatal("later unit rejected")
	}
	s.mu.Lock()
	p.turn.units[1].value.PrecedingCalls[0].StartedEventSeq = "42"
	s.mu.Unlock()
	before := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if p.ToolMetadataV2(n, call, item, result, 41) {
		t.Fatal("conflicting preceding copy accepted")
	}
	if !reflect.DeepEqual(before, s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)) {
		t.Fatal("conflict partially enriched the origin")
	}
}

func TestMutationInputSizeIsBoundedProtectedMetadataAndImmutableV1(t *testing.T) {
	for _, tc := range []struct {
		name, kind, state string
		units             *int64
		withheld, want    bool
	}{
		{"write", "write", "completed", inputSizeFixtureV1(1025), false, true},
		{"edit zero", "edit", "completed", inputSizeFixtureV1(0), false, true},
		{"safe integer maximum", "edit", "completed", inputSizeFixtureV1(display.MaxInputUtf16CodeUnitsV1), false, true},
		{"settlement failure", "write", "failed", inputSizeFixtureV1(3), false, true},
		{"missing", "edit", "completed", nil, false, false},
		{"negative", "edit", "completed", inputSizeFixtureV1(-1), false, false},
		{"unsafe integer", "edit", "completed", inputSizeFixtureV1(display.MaxInputUtf16CodeUnitsV1 + 1), false, false},
		{"shell", "shell", "completed", inputSizeFixtureV1(3), false, false},
		{"withheld", "write", "completed", inputSizeFixtureV1(3), true, false},
		{"unknown", "edit", "unknown", inputSizeFixtureV1(3), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, authority := serviceFixtureV1(t)
			var expected int64
			if tc.units != nil {
				expected = *tc.units
			}
			toolCaptureFixtureV2(t, p, security.HostToolCallIDPrefixV1+strings.Repeat("9", security.HostToolCallIDEntropyBytesV1*2), display.RawCaptureV1{Kind: tc.kind, Complete: true, Overflow: tc.withheld, Status: tc.state, Path: "/synthetic/file", Diff: "actual diff", InputUtf16CodeUnits: tc.units})
			if tc.units != nil {
				*tc.units = 7
			}
			preview := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			if len(preview.Blocks) != 1 || (preview.Blocks[0].InputUtf16CodeUnits != nil) != tc.want || preview.Blocks[0].Body != nil {
				t.Fatal("bounded input size did not follow protected metadata classification")
			}
			id := preview.Blocks[0].BlockID
			if tc.want {
				original := *preview.Blocks[0].InputUtf16CodeUnits
				if original != expected {
					t.Fatal("service retained producer-owned size pointer")
				}
				*preview.Blocks[0].InputUtf16CodeUnits = 8
				again := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
				if again.Blocks[0].InputUtf16CodeUnits == nil || *again.Blocks[0].InputUtf16CodeUnits != original || again.Blocks[0].CaptureDigest == "" || again.Blocks[0].CaptureDigest != preview.Blocks[0].CaptureDigest {
					t.Fatal("display response mutated protected capture size or digest")
				}
				changed := again.Blocks[0]
				different := original + 1
				changed.InputUtf16CodeUnits = &different
				if display.CaptureDigestV1(changed) == again.Blocks[0].CaptureDigest {
					t.Fatal("capture digest did not bind actual input size")
				}
			}
			authority.mu.Lock()
			authority.denied = true
			authority.mu.Unlock()
			if response := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); response.Status != "unavailable" || len(response.Blocks) != 0 {
				t.Fatal("revoked source retained input size")
			}
		})
	}
}

func inputSizeFixtureV1(value int64) *int64 { return &value }

func TestDisplayQuotaWithholdsMutationInputSizeV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	s.limits.DisplayBytes = 1
	toolCaptureFixtureV2(t, p, security.HostToolCallIDPrefixV1+strings.Repeat("8", security.HostToolCallIDEntropyBytesV1*2), display.RawCaptureV1{Kind: "write", Complete: true, Status: "completed", Diff: "actual diff", InputUtf16CodeUnits: inputSizeFixtureV1(1025)})
	response := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(response.Blocks) != 1 || response.Blocks[0].Classification != "withheld" || response.Blocks[0].InputUtf16CodeUnits != nil {
		t.Fatal("quota withholding retained input size")
	}
}

func draftFixtureV1(t *testing.T, p *ProducerV1, text string, attempt uint64) {
	t.Helper()
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real draft unit rejected")
	}
	p.FrameV1(model.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", ProviderSequence: 7, Attempt: attempt, Text: text, ContentClassification: "unclassified_inline"})
}
func draftTextFixtureV1(t *testing.T, s *ServiceV1) (ResponseV1, string) {
	t.Helper()
	metadata := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	ids := []string{}
	for _, block := range metadata.Blocks {
		ids = append(ids, block.BlockID)
		if len(block.DraftSegments) != 0 {
			t.Fatal("collapsed draft exposed body")
		}
	}
	response := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, ids)
	text := ""
	for _, block := range response.Blocks {
		for _, segment := range block.DraftSegments {
			text += segment.Text
		}
	}
	return response, text
}
func TestAssistantDraftUpdatesRetireAndCannotReviveV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	draftFixtureV1(t, p, "first draft words ", 1)
	first, text := draftTextFixtureV1(t, s)
	if text != "first draft words " || len(first.Blocks) != 1 || first.Blocks[0].Source.Kind != "assistant_draft" || first.Blocks[0].NativeIndex != nil || first.Blocks[0].Body != nil || first.Blocks[0].ContentClassification != "unclassified_inline" {
		t.Fatal("draft source or first update incorrect")
	}
	draftFixtureV1(t, p, "second update ", 1)
	second, text := draftTextFixtureV1(t, s)
	if text != "first draft words second update " || first.Revision == second.Revision {
		t.Fatal("draft did not update before completion")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
	_, text = draftTextFixtureV1(t, s)
	if text != "" {
		t.Fatal("retry retained draft")
	}
	draftFixtureV1(t, p, "OLD_LATE_DRAFT ", 1)
	draftFixtureV1(t, p, "replacement draft ", 2)
	p.FrameV1(model.ProcessFrameV1{Kind: "terminal", ProviderSequence: 7, Attempt: 2})
	revision := p.turn.revision
	draftFixtureV1(t, p, "TERMINAL_LATE_DRAFT ", 2)
	_, text = draftTextFixtureV1(t, s)
	if text != "" || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 || p.turn.revision != revision {
		t.Fatal("terminal draft revived or retained bytes")
	}
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("revoked current authority delivered draft metadata")
	}
}
func TestAssistantDraftWindowsAndQuotaAndFinishV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	source := strings.Repeat("字 ", 20000)
	draftFixtureV1(t, p, source, 1)
	response, text := draftTextFixtureV1(t, s)
	if text != source || len(response.Blocks[0].DraftSegments) < 2 || response.Blocks[0].Truncated {
		t.Fatal("draft silently truncated at complete capture bound")
	}
	p.FinishV1()
	_, text = draftTextFixtureV1(t, s)
	if text != "" || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
		t.Fatal("Finish retained draft body")
	}
	p.FrameV1(model.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", ProviderSequence: 7, Attempt: 1, Text: "LATE_AFTER_FINISH ", ContentClassification: "provider_text"})
	_, text = draftTextFixtureV1(t, s)
	if text != "" {
		t.Fatal("finished producer revived")
	}
	// A distinct fresh fixture proves display overflow is explicit withholding,
	// without increasing quotas for complete tool/native captures.
	s2, p2, _ := serviceFixtureV1(t)
	s2.limits.DisplayBytes = 64
	draftFixtureV1(t, p2, "safe initial ", 1)
	draftFixtureV1(t, p2, strings.Repeat("overflow ", 20), 1)
	response, text = draftTextFixtureV1(t, s2)
	if text != "" || response.Blocks[0].Classification != "withheld" || p2.turn.displayBytes != 0 {
		t.Fatal("quota left stale partial draft")
	}
}

func TestAssistantDraftEpochLossRetiresLogicalUnitV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	draftFixtureV1(t, p, "current epoch draft ", 1)
	_, text := draftTextFixtureV1(t, s)
	if text == "" {
		t.Fatal("running draft unavailable before epoch loss")
	}
	a.mu.Lock()
	a.draftDenied = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("stale epoch delivered running draft")
	}
	a.mu.Lock()
	a.draftDenied = false
	a.mu.Unlock()
	// Even fresh restoration of current authority cannot make the same
	// logical source active again or admit a late/replacement attempt.
	p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
	draftFixtureV1(t, p, "LATE_EPOCH_DRAFT ", 2)
	if p.turn.rawBytes != 0 || p.turn.displayBytes != 0 || len(p.turn.blocks) != 1 || len(p.turn.blocks[0].value.DraftSegments) != 0 {
		t.Fatal("epoch-lost unit revived on late reset/attempt")
	}
}

func TestAssistantDraftBaseAuthorityLossCannotReviveSourceV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	draftFixtureV1(t, p, "bound source current words ", 1)
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("lost base authority delivered draft")
	}
	a.mu.Lock()
	a.denied = false
	a.mu.Unlock()
	p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
	draftFixtureV1(t, p, "LATE_BASE_AUTHORITY_DRAFT ", 2)
	if p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
		t.Fatal("restored base authority revived old source")
	}
}

func TestAssistantDraftCancelledPanePreservesSiblingSourceV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	draftFixtureV1(t, p, "siblings share the current source ", 1)
	const sibling = "11111111-1111-4111-8111-111111111111"
	if got := s.AuthorizeV1(context.Background(), sibling, documentFixtureV1, p.turn.binding.ThreadID, p.turn.binding.TurnID); got.Status != "authorized" {
		t.Fatal("sibling viewer denied")
	}
	a.mu.Lock()
	a.draftEntered = make(chan struct{}, 1)
	a.draftWait = make(chan struct{})
	entered := a.draftEntered
	a.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan ResponseV1, 1)
	go func() { done <- s.ReadV1(ctx, tokenFixtureV1, "", 0, nil) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("draft validator did not block")
	}
	cancel()
	select {
	case got := <-done:
		if got.Status != "unavailable" {
			t.Fatal("cancelled pane returned draft")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled validator did not return")
	}
	a.mu.Lock()
	a.draftEntered = nil
	a.draftWait = nil
	a.mu.Unlock()
	metadata := s.ReadV1(context.Background(), sibling, "", 0, nil)
	if metadata.Status != "snapshot" || len(metadata.Blocks) != 1 {
		t.Fatal("pane cancellation revoked sibling")
	}
	expanded := s.ReadV1(context.Background(), sibling, "", 0, []string{metadata.Blocks[0].BlockID})
	if expanded.Status != "snapshot" || len(expanded.Blocks[0].DraftSegments) == 0 {
		t.Fatal("pane cancellation retired shared source")
	}
}

func TestWebActiveInputUsesRealStartedAnchorAndSealedTargetV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	call := security.HostToolCallIDPrefixV1 + strings.Repeat("8", security.HostToolCallIDEntropyBytesV1*2)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	tool := display.ToolBindingV1{CallID: call, ToolName: "web_fetch", GrantID: strings.Repeat("a", 64), InputArgsHash: strings.Repeat("b", 64)}
	requestURL := "https://example.invalid/real/request?case=alice"
	if !p.ToolReadyV2(n, call, item) {
		t.Fatal("actual ready anchor rejected")
	}
	if p.WebInputV1(call, requestURL, tool) {
		t.Fatal("ready-only source invented a running URL")
	}
	if !p.ToolMetadataV2(n, call, item, "", 12) || !p.WebInputV1(call, requestURL, tool) {
		t.Fatal("real started input rejected")
	}
	snapshot := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if snapshot.Status != "snapshot" || len(snapshot.Blocks) != 1 {
		t.Fatal("running input missing")
	}
	block := snapshot.Blocks[0]
	if block.Kind != "web_fetch" || block.State != "running" || block.RequestURL != requestURL || block.InputDigest == "" || block.Body != nil || block.Source.ToolResultItemID != "" || block.CaptureDigest != "" {
		t.Fatal("Web input became a result capture")
	}
	encoded, _ := json.Marshal(snapshot)
	for _, private := range []string{tool.GrantID, tool.InputArgsHash, "targetUrl"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("host binding or target leaked into ordinary process DTO")
		}
	}
	target := s.OpenWebLinkV1(context.Background(), tokenFixtureV1, block.BlockID, block.InputDigest, "request")
	if target.Status != "web_link" || target.TargetURL != requestURL || target.Scope == nil || target.InputDigest != block.InputDigest {
		t.Fatal("sealed fresh target unavailable")
	}
	for _, bad := range []struct{ id, digest, role string }{{block.BlockID, strings.Repeat("0", 64), "request"}, {"different", block.InputDigest, "request"}, {block.BlockID, block.InputDigest, "result"}} {
		if got := s.OpenWebLinkV1(context.Background(), tokenFixtureV1, bad.id, bad.digest, bad.role); got.Status != "unavailable" || got.TargetURL != "" {
			t.Fatal("substituted target admitted")
		}
	}
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolMetadataV2(n, call, item, result, 12) {
		t.Fatal("actual settlement anchor rejected")
	}
	retired := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if retired.Status != "snapshot" || retired.Blocks[0].RequestURL != "" || retired.Blocks[0].InputDigest != "" || retired.Blocks[0].Classification != "withheld" || retired.Blocks[0].Source.ToolResultItemID != result {
		t.Fatal("settlement retained active source")
	}
	if got := s.OpenWebLinkV1(context.Background(), tokenFixtureV1, block.BlockID, block.InputDigest, "request"); got.Status != "unavailable" {
		t.Fatal("retired active source remained clickable")
	}
	s.mu.Lock()
	retained := p.turn.rawBytes
	s.mu.Unlock()
	if retained != 0 {
		t.Fatal("retired input raw URL remained retained")
	}
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	if got := s.OpenWebLinkV1(context.Background(), tokenFixtureV1, block.BlockID, block.InputDigest, "request"); got.Status != "unavailable" {
		t.Fatal("current lease revocation ignored")
	}
}

func TestWebActiveInputRejectsOtherToolsMaskedSourcesAndOverflowV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	call := security.HostToolCallIDPrefixV1 + strings.Repeat("9", security.HostToolCallIDEntropyBytesV1*2)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	tool := display.ToolBindingV1{CallID: call, ToolName: "web_fetch", GrantID: strings.Repeat("a", 64), InputArgsHash: strings.Repeat("b", 64)}
	if !p.ToolReadyV2(n, call, item) || !p.ToolMetadataV2(n, call, item, "", 12) {
		t.Fatal("real anchor rejected")
	}
	wrong := tool
	wrong.ToolName = "bash"
	if p.WebInputV1(call, "https://example.invalid/", wrong) || p.WebInputV1(call, "https://example.invalid/process-test-known-credential", tool) || p.WebInputV1(call, "https://example.invalid/"+strings.Repeat("x", display.WebRequestURLByteLimitV1), tool) {
		t.Fatal("wrong tool, masked or overflow URL accepted")
	}
	got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(got.Blocks) != 0 {
		t.Fatal("rejected source invented Web input")
	}
}

func TestLiveReasoningUsesRealUnitAndStableSelectedBodyV1(t *testing.T) {
	for _, source := range []string{"", "provider_reasoning"} {
		t.Run("source="+source, func(t *testing.T) {
			s, p, _ := serviceFixtureV1(t)
			n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
			if !p.UnitV2(n, true) {
				t.Fatal("real unit rejected")
			}
			frame := func(kind, text string) model.ProcessFrameV1 {
				f := frameFixtureV1(kind, text)
				f.Source = source
				if source != "" {
					f.NativeIndex = 0
					f.HostSegmentOrdinal = 1
				}
				return f
			}
			p.FrameV1(frame("start", ""))
			p.FrameV1(frame("delta", "**First**"))
			first := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			if first.Status != "snapshot" || len(first.Blocks) != 1 || first.Blocks[0].Preview != "" || first.Blocks[0].Body != nil {
				t.Fatal("unfinished line became a preview/body")
			}
			id := first.Blocks[0].BlockID
			p.FrameV1(frame("delta", "\nfirst paragraph\n\n**Latest**\nlast word "))
			metadata := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
			if metadata.Blocks[0].BlockID != id || metadata.Blocks[0].Preview != "Latest" || metadata.Blocks[0].Body != nil || metadata.Blocks[0].CaptureDigest != "" {
				t.Fatal("live preview source or collapsed boundary changed")
			}
			selected := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if selected.Blocks[0].Body == nil || !strings.Contains(selected.Blocks[0].Body.Text, "last word") {
				t.Fatal("selected live body unavailable")
			}
			if source != "" && (selected.Blocks[0].Source.Kind != source || selected.Blocks[0].NativeIndex != nil) {
				t.Fatal("host reasoning fabricated native identity")
			}
			p.FrameV1(frame("end", ""))
			settled := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if settled.Blocks[0].BlockID != id || settled.Blocks[0].State != "completed" || settled.Blocks[0].Preview != "**First**" || settled.Blocks[0].CaptureDigest == "" || settled.Blocks[0].Body == nil {
				t.Fatal("real end did not settle the same reasoning source")
			}
			if !p.UnitV2(n, false) || !p.UnitV2(n, true) {
				t.Fatal("idempotent registration failed")
			}
			before := p.turn.revision
			p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
			late := frame("start", "")
			late.Attempt = 2
			late.NativeIndex = 9
			late.HostSegmentOrdinal = 2
			p.FrameV1(late)
			late.Kind, late.Text = "delta", "LATE_RUNNING_SOURCE "
			p.FrameV1(late)
			late.Kind, late.Text = "end", ""
			p.FrameV1(late)
			after := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if p.turn.revision != before || len(after.Blocks) != 1 || after.Blocks[0].CaptureDigest != settled.Blocks[0].CaptureDigest || after.Blocks[0].Body.Text != settled.Blocks[0].Body.Text {
				t.Fatal("inactive unit or late reset replaced settled reasoning")
			}
		})
	}
}

func TestLiveReasoningPreviewUsesWireByteBudgetV1(t *testing.T) {
	for _, test := range []struct {
		name, line, want string
	}{
		{"above old limit", strings.Repeat("中🙂", 100), strings.Repeat("中🙂", 100)},
		{"exact wire limit", strings.Repeat("中🙂", 146) + "ab", strings.Repeat("中🙂", 146) + "ab"},
		{"split Chinese", strings.Repeat("中", 342), strings.Repeat("中", 341)},
		{"split emoji", strings.Repeat("中", 341) + "🙂", strings.Repeat("中", 341)},
	} {
		for _, source := range []string{"", "provider_reasoning"} {
			t.Run(source+"/"+test.name, func(t *testing.T) {
				s, p, _ := serviceFixtureV1(t)
				n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
				if !p.UnitV2(n, true) {
					t.Fatal("real unit rejected")
				}
				frame := func(kind, text string) model.ProcessFrameV1 {
					f := frameFixtureV1(kind, text)
					f.Source = source
					if source != "" {
						f.NativeIndex, f.HostSegmentOrdinal = 0, 1
					}
					return f
				}
				body := "**First**\nfirst paragraph\n\n**" + test.line + "**\nlast word \n\nunfinished"
				p.FrameV1(frame("start", ""))
				p.FrameV1(frame("delta", body))
				collapsed := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
				if len(collapsed.Blocks) != 1 {
					t.Fatal("reasoning block missing")
				}
				b := collapsed.Blocks[0]
				if b.Preview != test.want || !utf8.ValidString(b.Preview) || len(b.Preview) > 1024 || b.Body != nil || b.Truncated || b.Classification != "source_exact_local" {
					t.Fatalf("latest complete paragraph preview: bytes=%d, want=%d", len(b.Preview), len(test.want))
				}
				selected := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{b.BlockID})
				if selected.Blocks[0].Body == nil || selected.Blocks[0].Body.Text != strings.TrimSuffix(body, "unfinished") || selected.Blocks[0].Truncated || p.turn.rawBytes != len(body) {
					t.Fatal("preview changed selected body or raw accounting")
				}
				p.FrameV1(frame("end", ""))
				settled := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
				if settled.Blocks[0].Preview != "**First**" || settled.Blocks[0].Body != nil || settled.Blocks[0].Truncated {
					t.Fatal("completion lost first-line algorithm or collapsed boundary")
				}
				selected = s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{b.BlockID})
				if selected.Blocks[0].Body == nil || selected.Blocks[0].Body.Text != body || p.turn.rawBytes != 0 {
					t.Fatal("completion changed full bounded body or raw retirement")
				}
			})
		}
	}
}

func TestLiveReasoningKeepsBodyAndAggregateBudgetsV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	if s.limits.RawBytes != 1<<20 || s.limits.DisplayBytes != 256<<10 || s.limits.MaxBlocks != 128 {
		t.Fatal("existing raw, display or block budget changed")
	}
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real unit rejected")
	}
	body := strings.Repeat("中", 12000) + "\n"
	p.FrameV1(frameFixtureV1("start", ""))
	p.FrameV1(frameFixtureV1("delta", body))
	collapsed := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(collapsed.Blocks) != 1 || collapsed.Blocks[0].Body != nil {
		t.Fatal("collapsed body escaped")
	}
	id := collapsed.Blocks[0].BlockID
	checkBody := func() {
		t.Helper()
		selected := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
		b := selected.Blocks[0]
		if b.Body == nil || b.Body.Text != strings.Repeat("中", (32<<10)/3) || !utf8.ValidString(b.Body.Text) || !b.Truncated || b.Preview != strings.Repeat("中", 341) {
			t.Fatal("existing body limit or separate preview limit changed")
		}
		if p.turn.displayBytes != blockBytesV1(b) || p.turn.displayBytes > s.limits.DisplayBytes {
			t.Fatal("display accounting lost bounded preview/body")
		}
	}
	checkBody()
	if p.turn.rawBytes != len(body) {
		t.Fatal("live raw accounting changed")
	}
	p.FrameV1(frameFixtureV1("end", ""))
	checkBody()
}

func TestResponsesSummaryProducerKeepsSelectedBodyFormRetryAndAuthorityV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("actual logical unit rejected")
	}
	const body = `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"summary-item","type":"reasoning","summary":[],"encrypted_content":"OPAQUE_PROCESS_CANARY"}}

data: {"type":"response.reasoning_summary_part.added","output_index":0,"item_id":"summary-item","summary_index":0,"sequence_number":1,"part":{"type":"summary_text","text":""}}

data: {"type":"response.reasoning_summary_text.delta","output_index":0,"item_id":"summary-item","summary_index":0,"sequence_number":2,"delta":"Readable summary. \n"}

data: {"type":"response.reasoning_summary_text.done","output_index":0,"item_id":"summary-item","summary_index":0,"sequence_number":3,"text":"Readable summary. \n"}

data: {"type":"response.reasoning_summary_part.done","output_index":0,"item_id":"summary-item","summary_index":0,"sequence_number":4,"part":{"type":"summary_text","text":"Readable summary. \n"}}

data: {"type":"response.output_item.done","output_index":0,"item":{"id":"summary-item","type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"Readable summary. \n"}],"encrypted_content":"OPAQUE_PROCESS_CANARY"}}

data: {"type":"response.output_text.delta","output_index":1,"item_id":"answer","delta":"verified answer"}

data: {"type":"response.completed","response":{}}

`
	id := ""
	liveBody := ""
	var frames []model.ProcessFrameV1
	chunks, _, err := providerstream.ParseSSEWithCallback("responses", strings.NewReader(body), func(chunk model.Chunk) error {
		if chunk.ProcessFrame == nil {
			return nil
		}
		frame := *chunk.ProcessFrame
		frame.Attempt, frame.ProviderSequence = 1, 7
		frames = append(frames, frame)
		p.FrameV1(frame)
		metadata := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
		if len(metadata.Blocks) != 1 || metadata.Blocks[0].Body != nil || metadata.Blocks[0].Source.ReasoningForm != "summary" || metadata.Blocks[0].Source.Kind != "provider_reasoning" || metadata.Blocks[0].NativeIndex != nil {
			t.Fatal("real summary lost source attribution or disclosed collapsed body")
		}
		if id == "" {
			id = metadata.Blocks[0].BlockID
		}
		if metadata.Blocks[0].BlockID != id {
			t.Fatal("summary snapshots replaced their source")
		}
		if frame.Kind == "delta" {
			for _, form := range []string{"", "full"} {
				forged := frame
				forged.ReasoningForm, forged.Text = form, "CROSS_FORM_CANARY "
				p.FrameV1(forged)
			}
			selected := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
			if selected.Blocks[0].Body != nil {
				liveBody = selected.Blocks[0].Body.Text
			}
		}
		return nil
	})
	if err != nil || id == "" || !strings.Contains(liveBody, "Readable summary.") || strings.Contains(liveBody, "CANARY") {
		t.Fatalf("actual Responses summary did not reach private live body: err=%v", err)
	}
	settled := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
	if len(settled.Blocks) != 1 || settled.Blocks[0].State != "completed" || settled.Blocks[0].Title != "Reasoning summary" || settled.Blocks[0].CaptureDigest == "" || settled.Blocks[0].Body == nil || !strings.Contains(settled.Blocks[0].Body.Text, "Readable summary.") || strings.Contains(settled.Blocks[0].Body.Text, "CANARY") {
		t.Fatal("completed summary became full reasoning or lost its protected selected body")
	}
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil || chunk.Kind == model.ChunkReasoning {
			t.Fatal("private summary entered continuation")
		}
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
	for _, frame := range frames {
		p.FrameV1(frame)
	}
	retired := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{id})
	if len(retired.Blocks) != 1 || retired.Blocks[0].State != "superseded" || retired.Blocks[0].Body != nil || retired.Blocks[0].Source.ReasoningForm != "summary" {
		t.Fatal("retry revived old summary content or changed its immutable attribution")
	}
	fresh := model.ProcessFrameV1{Source: "provider_reasoning", ReasoningForm: "summary", Kind: "start", ProviderSequence: 7, Attempt: 2, HostSegmentOrdinal: 1}
	p.FrameV1(fresh)
	fresh.Kind, fresh.Text = "delta", "replacement summary "
	p.FrameV1(fresh)
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("authority loss disclosed summary")
	}
	a.mu.Lock()
	a.denied = false
	a.mu.Unlock()
	before := p.turn.revision
	fresh.Kind, fresh.Text = "end", ""
	p.FrameV1(fresh)
	if p.turn.revision != before || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
		t.Fatal("restored authority reopened a retired summary source")
	}
}

func TestLiveReasoningAuthorityLossPermanentlyRetiresUnitV1(t *testing.T) {
	for _, source := range []string{"", "provider_reasoning"} {
		for _, loss := range []string{"base", "epoch"} {
			t.Run(source+"/"+loss, func(t *testing.T) {
				s, p, a := serviceFixtureV1(t)
				n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
				if !p.UnitV2(n, true) {
					t.Fatal("real unit rejected")
				}
				f := model.ProcessFrameV1{Source: source, Kind: "start", ProviderSequence: 7, Attempt: 1, NativeIndex: 3, HostSegmentOrdinal: 1}
				p.FrameV1(f)
				f.Kind, f.Text = "delta", "current live words "
				p.FrameV1(f)
				a.mu.Lock()
				a.denied = loss == "base"
				a.draftDenied = loss == "epoch"
				a.mu.Unlock()
				if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
					t.Fatal("lost authority delivered live source")
				}
				a.mu.Lock()
				a.denied, a.draftDenied = false, false
				a.mu.Unlock()
				fresh := s.BeginV1(p.turn.binding)
				if fresh == nil || !fresh.UnitV2(n, true) {
					t.Fatal("registration replay unavailable")
				}
				defer fresh.FinishV1()
				before := p.turn.revision
				for _, producer := range []*ProducerV1{p, fresh} {
					producer.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
					f.Kind, f.Text, f.Attempt, f.NativeIndex, f.HostSegmentOrdinal = "start", "", 2, 9, 2
					producer.FrameV1(f)
					f.Kind, f.Text = "delta", "LATE_REPLACEMENT "
					producer.FrameV1(f)
					f.Kind, f.Text = "end", ""
					producer.FrameV1(f)
				}
				if p.turn.revision != before || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 || len(p.turn.blocks) != 1 || p.turn.blocks[0].value.Body != nil || p.turn.blocks[0].value.State != "unknown" {
					t.Fatal("restored authority revived a retired source")
				}
			})
		}
	}
}

func TestPreparationFinalIsNotDispatchAndToolReadyRetiresSameSourceV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real unit rejected")
	}
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("7", model.HostToolCallIDEntropyBytesV1*2)
	f := model.ProcessFrameV1{Source: "tool_preparation", Kind: "delta", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0, CallID: call, ToolName: "write", Arguments: `{"path":"actual.txt","content":"A😀`}}
	p.FrameV1(model.ProcessFrameV1{Source: "tool_preparation", Kind: "start", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0}})
	p.FrameV1(f)
	first := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if first.Status != "snapshot" || len(first.Blocks) != 1 {
		t.Fatal("named preparation absent")
	}
	b := first.Blocks[0]
	if b.State != "preparing" || b.ToolName != "write" || b.Path != "actual.txt" || b.InputUtf16CodeUnits == nil || *b.InputUtf16CodeUnits != 3 || b.Body != nil || b.CaptureDigest != "" || len(first.Units[0].Calls) != 0 {
		t.Fatal("preparation invented execution or lost decoded input size")
	}
	f.Kind = "end"
	f.ToolPreparation.Arguments = `{"path":"final.txt","content":"Z"}`
	p.FrameV1(f)
	sealed := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{b.BlockID})
	if sealed.Blocks[0].BlockID != b.BlockID || sealed.Blocks[0].State != "preparing" || sealed.Blocks[0].Path != "final.txt" || sealed.Blocks[0].InputUtf16CodeUnits == nil || *sealed.Blocks[0].InputUtf16CodeUnits != 1 || sealed.Blocks[0].Body != nil || len(sealed.Units[0].Calls) != 0 {
		t.Fatal("final arguments executed or failed to replace a conflicting prefix")
	}
	before := p.turn.revision
	f.Kind = "delta"
	f.ToolPreparation.Arguments = "LATE_ARGUMENTS"
	p.FrameV1(f)
	if p.turn.revision != before {
		t.Fatal("sealed preparation admitted a late delta")
	}
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolReadyV2(n, call, item) || !p.ToolReadyV2(n, call, item) {
		t.Fatal("actual admitted anchor unavailable")
	}
	retired := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(retired.Blocks) != 1 || retired.Blocks[0].BlockID != b.BlockID || retired.Blocks[0].State != "unknown" || retired.Blocks[0].ToolName != "" || retired.Blocks[0].Path != "" || retired.Blocks[0].InputUtf16CodeUnits != nil || p.turn.rawBytes != 0 {
		t.Fatal("ToolReady retained/repeated preparation")
	}
	before = p.turn.revision
	f.Kind = "end"
	p.FrameV1(f)
	if p.turn.revision != before {
		t.Fatal("admitted tool revived old preparation")
	}
}

func TestDispatchedShellHasCommandOnlyThenSameSettledBlockV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("6", model.HostToolCallIDEntropyBytesV1*2)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	tool := display.ToolBindingV1{CallID: call, ToolName: "bash", GrantID: strings.Repeat("a", 64), InputArgsHash: strings.Repeat("b", 64)}
	if !p.ToolReadyV2(n, call, item) {
		t.Fatal("actual call rejected")
	}
	if p.ShellInputV1(call, "echo actual", tool) {
		t.Fatal("ready without actual started anchor became running")
	}
	if !p.ToolMetadataV2(n, call, item, "", 12) {
		t.Fatal("actual started anchor rejected")
	}
	publisher := s.BeginV1(p.turn.binding, "process-test-known-credential")
	if publisher == nil || !publisher.ShellInputV1(call, "echo actual process-test-known-credential", tool) {
		t.Fatal("dispatched input rejected")
	}
	publisher.FinishV1()
	metadata := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(metadata.Blocks) != 1 {
		t.Fatal("short publisher lost command")
	}
	b := metadata.Blocks[0]
	selected := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{b.BlockID})
	if b.State != "running" || b.Classification != "credential_redacted" || !strings.Contains(b.Title, "echo actual") || strings.Contains(b.Title, "process-test-known-credential") || b.Body != nil || b.Preview != "" || b.CaptureDigest != "" || b.ExitCode != nil || selected.Blocks[0].Body != nil {
		t.Fatal("dispatched command acquired output or lost source")
	}
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolMetadataV2(n, call, item, result, 12) {
		t.Fatal("actual result anchor rejected")
	}
	between := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if between.Blocks[0].Title != "" || between.Blocks[0].Source.ToolResultItemID != result {
		t.Fatal("settled anchor retained active command")
	}
	tool.ResultItemID, tool.InputArgsHash = result, ""
	exit := 7
	p.ToolV1(call, display.RawCaptureV1{Kind: "shell", Status: "completed", Complete: true, Title: "echo actual", Body: "actual final output\n", ExitCode: &exit, ToolBinding: tool})
	settled := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{b.BlockID})
	if len(settled.Blocks) != 1 || settled.Blocks[0].BlockID != b.BlockID || settled.Blocks[0].State != "completed" || settled.Blocks[0].Body == nil || settled.Blocks[0].Body.Text != "actual final output\n" || settled.Blocks[0].ExitCode == nil || *settled.Blocks[0].ExitCode != 7 {
		t.Fatal("authentic settlement replaced/duplicated the command source")
	}
	if p.ShellInputV1(call, "late command", tool) {
		t.Fatal("result source became running again")
	}
}

func TestRetiredShellReceivesOnlyActualSettlementCoordinateV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil)
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("5", model.HostToolCallIDEntropyBytesV1*2)
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	tool := display.ToolBindingV1{CallID: call, ToolName: "bash", GrantID: strings.Repeat("a", 64), InputArgsHash: strings.Repeat("b", 64)}
	if !p.ToolReadyV2(n, call, item) || !p.ToolMetadataV2(n, call, item, "", 12) || !p.ShellInputV1(call, "echo retired command", tool) {
		t.Fatal("actual command rejected")
	}
	id := p.turn.blocks[0].value.BlockID
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("lost authority delivered command")
	}
	a.mu.Lock()
	a.denied = false
	a.mu.Unlock()
	const freshViewer = "00000000-0000-4000-8000-000000000003"
	if got := s.AuthorizeV1(context.Background(), freshViewer, documentFixtureV1, p.turn.binding.ThreadID, p.turn.binding.TurnID); got.Status != "authorized" {
		t.Fatal("fresh viewer unavailable")
	}
	result := toolresult.ToolResultItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolMetadataV2(n, call, item, result, 12) {
		t.Fatal("actual result metadata rejected")
	}
	between := s.ReadV1(context.Background(), freshViewer, "", 0, []string{id})
	if between.Status != "snapshot" || len(between.Blocks) != 1 || len(between.Units) != 1 {
		t.Fatal("retired source snapshot unavailable")
	}
	b := between.Blocks[0]
	if b.BlockID != id || b.State != "unknown" || b.Classification != "withheld" || b.Title != "" || b.Body != nil || b.CaptureDigest != "" || b.Source.ToolResultItemID != result || between.Units[0].Calls[0].ToolResultItemID != b.Source.ToolResultItemID {
		t.Fatal("settlement metadata revived or detached retired content")
	}
	tool.ResultItemID, tool.InputArgsHash = result, ""
	p.ToolV1(call, display.RawCaptureV1{Kind: "shell", Status: "completed", Complete: true, Title: "echo actual", Body: "actual settled output\n", ToolBinding: tool})
	settled := s.ReadV1(context.Background(), freshViewer, "", 0, []string{id})
	if settled.Status != "snapshot" || len(settled.Blocks) != 1 || settled.Blocks[0].BlockID != id || settled.Blocks[0].State != "completed" || settled.Blocks[0].Body == nil || settled.Blocks[0].Body.Text != "actual settled output\n" {
		t.Fatal("authentic settlement lost its stable retired block")
	}
}

func TestPreparationOriginSharesOwnerOrderAndStaysPrivateV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real unit unavailable")
	}
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("b", model.HostToolCallIDEntropyBytesV1*2)
	origin := model.ProcessFrameV1{Source: "tool_preparation", Kind: "start", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0}}
	p.FrameV1(origin)
	p.FrameV1(origin)
	hidden := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if hidden.Status != "snapshot" || len(hidden.Blocks) != 0 || len(hidden.Units[0].Calls) != 0 || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 || p.turn.sourceOrdinal != 1 {
		t.Fatal("unnamed slot was displayed, replayed or given body/dispatch authority")
	}
	native := frameFixtureV1("start", "")
	p.FrameV1(native)
	native.Kind, native.Text = "delta", "R\n"
	p.FrameV1(native)
	named := model.ProcessFrameV1{Source: "tool_preparation", Kind: "delta", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0, CallID: call, ToolName: "write", Arguments: `{"path":"a.txt","content":"x"}`}}
	p.FrameV1(named)
	snap := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(snap.Blocks) != 2 || snap.Blocks[0].Source.Kind != "native" || snap.Blocks[0].Source.SourceOrdinal != "2" || snap.Blocks[1].Source.Kind != "tool_preparation" || snap.Blocks[1].Source.SourceOrdinal != "1" || p.turn.sourceOrdinal != 2 {
		t.Fatal("named materialization lost the real earlier tool origin")
	}
	reasoningID, prepID := snap.Blocks[0].BlockID, snap.Blocks[1].BlockID
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Blocks []struct {
			Source struct{ Kind, SourceOrdinal string }
		}
	}
	if json.Unmarshal(raw, &wire) != nil || wire.Blocks[1].Source.SourceOrdinal != "1" {
		t.Fatal("preparation ordinal did not cross the actual JSON projection")
	}
	// A continuation delta for the same native item accumulates in that item;
	// it does not create a third timeline block or consume another ordinal.
	native.Text = "C\n"
	p.FrameV1(native)
	repeated := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, []string{reasoningID})
	if len(repeated.Blocks) != 2 || repeated.Blocks[0].BlockID != reasoningID || repeated.Blocks[0].Body == nil || repeated.Blocks[0].Body.Text != "R\nC\n" || p.turn.sourceOrdinal != 2 {
		t.Fatal("same native item continuation became a new ordering event")
	}
	before := p.turn.revision
	p.FrameV1(origin)
	if p.turn.revision != before || p.turn.sourceOrdinal != 2 {
		t.Fatal("origin replay moved an admitted seat")
	}
	named.Kind = "end"
	named.ToolPreparation.Arguments = `{"path":"final.txt","content":"z"}`
	p.FrameV1(named)
	sealed := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if sealed.Blocks[1].BlockID != prepID || sealed.Blocks[1].Source.SourceOrdinal != "1" || sealed.Blocks[1].Path != "final.txt" || sealed.Blocks[1].Body != nil || len(sealed.Units[0].Calls) != 0 {
		t.Fatal("sealed arguments moved or dispatched their original seat")
	}
	item := toolcall.ToolCallItemIDV1(p.turn.binding.TurnID, call)
	if !p.ToolReadyV2(n, call, item) {
		t.Fatal("actual call takeover denied")
	}
	retired := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if retired.Blocks[1].BlockID != prepID || retired.Blocks[1].Source.SourceOrdinal != "1" || retired.Blocks[1].State != "unknown" || retired.Blocks[1].ToolName != "" || retired.Blocks[1].Body != nil || p.turn.sourceOrdinal != 2 {
		t.Fatal("actual call takeover moved or duplicated its origin")
	}
}

func TestPreparationOriginAttemptAndRevocationFencesV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real unit unavailable")
	}
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("c", model.HostToolCallIDEntropyBytesV1*2)
	origin := model.ProcessFrameV1{Source: "tool_preparation", Kind: "start", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0}}
	named := model.ProcessFrameV1{Source: "tool_preparation", Kind: "delta", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0, CallID: call, ToolName: "write", Arguments: `{"path":"fresh.txt","content":"x"}`}}
	// Named bytes without a real observed slot cannot manufacture chronology.
	p.FrameV1(named)
	if len(p.turn.blocks) != 0 || p.turn.sourceOrdinal != 0 {
		t.Fatal("missing origin fabricated a seat")
	}
	p.FrameV1(origin)
	p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
	p.FrameV1(named)
	p.FrameV1(origin)
	if len(p.turn.blocks) != 0 || p.turn.sourceOrdinal != 1 {
		t.Fatal("old physical attempt revived a named seat")
	}
	origin.Attempt, named.Attempt = 2, 2
	p.FrameV1(origin)
	p.FrameV1(named)
	snapshot := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if len(snapshot.Blocks) != 1 || snapshot.Blocks[0].Source.SourceOrdinal != "2" {
		t.Fatal("new real attempt did not receive its own immutable origin")
	}
	a.mu.Lock()
	a.draftDenied = true
	a.mu.Unlock()
	lost := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil)
	if lost.Status != "unavailable" || len(p.turn.blocks) != 1 || p.turn.blocks[0].value.State != "unknown" || p.turn.blocks[0].value.ToolName != "" || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
		t.Fatal("source loss did not retire preparation")
	}
	a.mu.Lock()
	a.draftDenied = false
	a.mu.Unlock()
	p.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 3})
	origin.Attempt, named.Attempt = 3, 3
	p.FrameV1(origin)
	p.FrameV1(named)
	if p.turn.sourceOrdinal != 2 || len(p.turn.blocks) != 1 || p.turn.blocks[0].value.State != "unknown" {
		t.Fatal("restored authority reopened a retired origin")
	}
	s.RevokeV1(tokenFixtureV1)
	p.FrameV1(origin)
	p.FrameV1(named)
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("late observation revived a revoked lease")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if p.turn.preparationOrigins != nil {
		t.Fatal("turn retirement retained control origins")
	}
}

func TestPreparationOriginControlQuotaV1(t *testing.T) {
	s, p, _ := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real unit unavailable")
	}
	for index := 0; index <= s.limits.MaxBlocks; index++ {
		p.FrameV1(model.ProcessFrameV1{Source: "tool_preparation", Kind: "start", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: index}})
	}
	if len(p.turn.preparationOrigins) != s.limits.MaxBlocks || p.turn.sourceOrdinal != uint64(s.limits.MaxBlocks) || len(p.turn.blocks) != 0 || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
		t.Fatal("unnamed control slots escaped the existing bounded owner budget")
	}
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("d", model.HostToolCallIDEntropyBytesV1*2)
	p.FrameV1(model.ProcessFrameV1{Source: "tool_preparation", Kind: "delta", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: s.limits.MaxBlocks, CallID: call, ToolName: "write", Arguments: `{"path":"a.txt","content":"x"}`}})
	if len(p.turn.blocks) != 0 {
		t.Fatal("quota-denied origin later became visible")
	}
}

func TestUnnamedOriginSourceLossCannotReviveV1(t *testing.T) {
	s, p, a := serviceFixtureV1(t)
	n := continuation.NewProviderContinuationNamespaceV1("turn", "", 7, nil)
	if !p.UnitV2(n, true) {
		t.Fatal("real unit unavailable")
	}
	origin := model.ProcessFrameV1{Source: "tool_preparation", Kind: "start", ProviderSequence: 7, Attempt: 1, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0}}
	p.FrameV1(origin)
	if len(p.turn.blocks) != 0 {
		t.Fatal("unknown name was displayed")
	}
	a.mu.Lock()
	a.draftDenied = true
	a.mu.Unlock()
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("unnamed origin bypassed current source authority")
	}
	a.mu.Lock()
	a.draftDenied = false
	a.mu.Unlock()
	fresh := s.BeginV1(p.turn.binding)
	if fresh == nil || !fresh.UnitV2(n, true) {
		t.Fatal("registration replay unavailable")
	}
	defer fresh.FinishV1()
	call := model.HostToolCallIDPrefixV1 + strings.Repeat("e", model.HostToolCallIDEntropyBytesV1*2)
	before := p.turn.revision
	for _, producer := range []*ProducerV1{p, fresh} {
		producer.FrameV1(model.ProcessFrameV1{Kind: "reset", ProviderSequence: 7, Attempt: 2})
		origin.Attempt = 2
		producer.FrameV1(origin)
		producer.FrameV1(model.ProcessFrameV1{Source: "tool_preparation", Kind: "delta", ProviderSequence: 7, Attempt: 2, ToolPreparation: &streamframe.ToolPreparationV1{Index: 0, CallID: call, ToolName: "write", Arguments: `{"path":"late.txt","content":"x"}`}})
	}
	if p.turn.revision != before || p.turn.sourceOrdinal != 1 || len(p.turn.blocks) != 0 || p.turn.rawBytes != 0 || p.turn.displayBytes != 0 {
		t.Fatal("restored source or fresh handle reopened an unnamed retired origin")
	}
	if got := s.ReadV1(context.Background(), tokenFixtureV1, "", 0, nil); got.Status != "unavailable" {
		t.Fatal("source restore reopened a revoked lease")
	}
}
