package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	inboundsse "analytix.local/runtime-go/internal/adapters/inbound/sse"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// This is an offline Go handler benchmark, not the Application/renderer fixture
// or native acceptance. Both variants use the same reopened durable store and
// actual TrustedPublicProjector. Only the finite callback composition differs.
// Run with -benchtime=1x to measure exactly one replay per variant.
func BenchmarkFiniteReplayArchiveReuseDurable(b *testing.B) {
	fixture := newFiniteReplayArchiveBenchmarkFixture(b)
	var legacyBody []byte
	for _, prepared := range []bool{false, true} {
		name := "legacy"
		if prepared {
			name = "prepared"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			reads, projections := 0, 0
			phaseMilliseconds := map[string]int64{}
			b.ResetTimer()
			b.StopTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				b.StartTimer()
				result := fixture.replay(prepared)
				b.StopTimer()
				reads += result.reads
				projections += result.projections
				for phase, milliseconds := range finiteReplayBenchmarkPhaseMilliseconds(b, result.diagnostics) {
					phaseMilliseconds[phase] += milliseconds
				}
				fixture.assertReplay(b, result.recorder)
				body := result.recorder.Body.Bytes()
				if !prepared && legacyBody == nil {
					legacyBody = bytes.Clone(body)
				}
				if !bytes.Equal(legacyBody, body) {
					b.Fatal("legacy and prepared replay changed the complete SSE response")
				}
			}
			// project_public ends before FinishPublicReplay. request_total includes
			// that final fresh authority check and the complete buffered write.
			for phase, milliseconds := range phaseMilliseconds {
				b.ReportMetric(float64(milliseconds)/float64(b.N), phase+"_ms/op")
			}
			b.ReportMetric(float64(reads)/float64(b.N), "GetThread/op")
			b.ReportMetric(float64(projections)/float64(b.N), "ProjectPublic/op")
			b.ReportMetric(float64(fixture.rawCount), "raw-events/op")
			b.ReportMetric(float64(fixture.publicCount), "public-units/op")
			b.ReportMetric(float64(len(legacyBody)), "SSE-bytes/op")
		})
		fixture.assertUnchanged(b)
	}
	b.Logf("same-input manifest: seeded_turns=26 archive_commits=26 raw=%d public=%d padding=%d highest_seq=%d primary_view_sha256=%x loaded_events_sha256=%x sse_sha256=%x thread_file_bytes=%d events_file_bytes=%d thread_file_sha256=%x events_file_sha256=%x",
		fixture.rawCount, fixture.publicCount, fixture.padding, fixture.highestSeq,
		sha256.Sum256(fixture.primary), sha256.Sum256(fixture.events), sha256.Sum256(legacyBody),
		len(fixture.rawThread), len(fixture.rawEvents), sha256.Sum256(fixture.rawThread), sha256.Sum256(fixture.rawEvents))
}

type finiteReplayArchiveBenchmarkFixture struct {
	handler               *runtimeServerHandler
	threadID              string
	rawCount, publicCount int
	highestSeq, padding   int
	primary, events       []byte
	rawThread, rawEvents  []byte
}

func newFiniteReplayArchiveBenchmarkFixture(t testing.TB) finiteReplayArchiveBenchmarkFixture {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(time.Second) })
	// No runtime turn or Provider route is invoked. The compaction service below
	// receives a deterministic summary at its existing test dependency seam.
	handler.provider = nil
	handler.providerExecution = nil
	workspace := workspacetest.New(t)
	realWorkspace, err := (filestore.CaseBindingReader{}).WorkspaceRealPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := handler.store.CreateThread(map[string]any{"title": "Finite replay benchmark", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	at := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
	contexts := make([]domainsecurity.TurnSecurityContext, 0, 26)
	for index := 1; index <= 26; index++ {
		frozen, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: fmt.Sprintf("turn_replay_%02d", index), WorkspaceRealPath: realWorkspace,
			ContextEpoch: 1, IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		contexts = append(contexts, frozen)
	}
	epochState, err := contextepochapp.BootstrapState(threadID, 1,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(contexts[len(contexts)-1])}, at)
	if err != nil {
		t.Fatal(err)
	}
	for index, frozen := range contexts {
		if err := handler.store.AppendTurnToThread(threadID, map[string]any{
			"id": frozen.TurnID, "threadId": threadID, "status": "running", "prompt": "Continue the recorded task.",
			"securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{
				"id": fmt.Sprintf("item_replay_%02d", index+1), "turnId": frozen.TurnID, "threadId": threadID,
				"kind": "user_message", "role": "user", "status": "completed",
				"text": strings.Repeat("Continue the safe task using the recorded decisions. ", 200),
			}},
		}, "fixture-provider", map[string]any{
			"securityState": turnsecurityapp.PublicRecord(frozen), "contextEpochState": contextepochapp.PublicState(epochState),
		}); err != nil {
			t.Fatal(err)
		}
		stamp := at.Add(time.Duration(index+40) * time.Second).Format(time.RFC3339Nano)
		result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
			Store: handler.store, SecurityContext: frozen, ThreadID: threadID, TurnID: frozen.TurnID,
			Model: "fixture-model", CreatedAt: stamp, FinishedAt: stamp,
		})
		if err != nil || !result.Changed {
			t.Fatalf("commit replay turn %d: changed=%t err=%v", index+1, result.Changed, err)
		}
	}
	summaryCalls := 0
	service := threadapp.NewService(threadapp.Dependencies{
		Repository: handler.store, PublicProjector: handler.publicProjector, WorkspaceReader: filestore.CaseBindingReader{},
		SummarizeCompaction: func(context.Context, threadapp.OrdinarySummaryInput) (string, error) {
			summaryCalls++
			return "Continue the safe task using the recorded decisions.", nil
		},
		ProjectCompactionHistory: handler.projectOrdinaryCompactionHistory,
		BeginTransition:          threadapp.AdaptBeginTransition(handler.runtimeSubagentState().BeginSecurityContextTransition),
	})
	if response, err := service.Compact(context.Background(), threadID, "manual"); err != nil || response == nil || summaryCalls != 1 {
		t.Fatalf("production compaction: response=%#v summary_calls=%d err=%v", response, summaryCalls, err)
	}
	reopened, err := newDurableEventSessionStore(handler.store.root, handler.store.mode, false)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetCaseThreadAuthority(handler.caseThreads)
	if err := reopened.RecoverOrdinaryCompactions(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler.store = reopened
	thread, err = reopened.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(thread[domainturnterminal.GeneralTerminalPublicationArchiveFieldV1])
	if err != nil || len(archive.Commits) != 26 || len(listAny(thread["turns"])) >= 26 {
		t.Fatalf("restart did not retain all 26 commits behind compacted primary: commits=%d turns=%d err=%v", len(archive.Commits), len(listAny(thread["turns"])), err)
	}
	loaded, err := reopened.LoadEventsSince(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		t.Fatalf("load replay input: diagnostics=%d err=%v", len(loaded.Diagnostics), err)
	}
	groups := map[string]int{}
	for _, event := range loaded.Events {
		if id := stringField(event, "generalTerminalCommitId"); id != "" {
			groups[id]++
		}
	}
	if len(groups) != 26 {
		t.Fatalf("terminal group count=%d", len(groups))
	}
	for id, count := range groups {
		if count != 3 {
			t.Fatalf("canonical terminal group %s has %d records", id, count)
		}
	}
	// Add ordinary lifecycle notices through the real durable owner. They have
	// no turn or authority fields; all sequence/timestamp values are assigned by
	// RecordEventsAtomic. This synthetic padding is not an Application transcript.
	publicCount := len(loaded.Events) - 2*len(groups)
	padding := max(0, 499-len(loaded.Events), 447-publicCount)
	drafts := make([]map[string]any, 0, padding)
	for index := 0; index < padding; index++ {
		drafts = append(drafts, map[string]any{"kind": "thread_updated", "threadId": threadID, "status": "idle"})
	}
	if len(drafts) != 0 {
		if _, _, err := reopened.RecordEventsAtomic(drafts); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err = reopened.LoadEventsSince(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		t.Fatalf("load padded replay input: diagnostics=%d err=%v", len(loaded.Diagnostics), err)
	}
	highest, err := reopened.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	primary, err := json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	events, err := json.Marshal(loaded.Events)
	if err != nil {
		t.Fatal(err)
	}
	rawThread, err := os.ReadFile(reopened.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	rawEvents, err := os.ReadFile(reopened.eventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	return finiteReplayArchiveBenchmarkFixture{handler: handler, threadID: threadID, rawCount: len(loaded.Events),
		publicCount: len(loaded.Events) - 2*len(groups), highestSeq: highest, padding: padding,
		primary: primary, events: events, rawThread: rawThread, rawEvents: rawEvents}
}

type finiteReplayBenchmarkReader struct {
	store *DurableEventSessionStore
	calls int
}

func (reader *finiteReplayBenchmarkReader) GetThread(threadID string) (map[string]any, error) {
	reader.calls++
	return reader.store.GetThread(threadID)
}

type finiteReplayBenchmarkResult struct {
	recorder           *httptest.ResponseRecorder
	reads, projections int
	diagnostics        []byte
}

func (fixture finiteReplayArchiveBenchmarkFixture) replay(prepared bool) finiteReplayBenchmarkResult {
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/"+fixture.threadID+"/events?since_seq=0", nil)
	reader := &finiteReplayBenchmarkReader{store: fixture.handler.store}
	store := inboundsse.ThreadEventStore{
		HighestSeq: fixture.handler.store.HighestSeq,
		LoadPublicEventsSinceContext: func(ctx context.Context, threadID string, afterSeq int) ([]map[string]any, error) {
			loaded, err := fixture.handler.store.LoadPublicEventsSinceContext(ctx, threadID, afterSeq)
			return loaded.Events, err
		},
		SealAcceptedFinalDelivery:     threadapp.NewAcceptedFinalDeliverySealer(request.Context(), fixture.handler.publicProjector),
		ValidateAcceptedFinalDelivery: threadapp.NewAcceptedFinalDeliveryValidator(request.Context(), fixture.handler.publicProjector),
		PreflightPublic:               threadapp.NewPublicProjectionPreflight(reader, fixture.handler.publicProjector),
		ProjectPublic:                 threadapp.NewPublicEventProjector(reader, fixture.handler.publicProjector),
	}
	if prepared {
		store.ProjectPublic, store.FinishPublicReplay = threadapp.NewFinitePublicEventProjector(request.Context(), reader, fixture.handler.publicProjector)
	}
	projections := 0
	project := store.ProjectPublic
	store.ProjectPublic = func(threadID string, event map[string]any) (map[string]any, bool, string) {
		projections++
		return project(threadID, event)
	}
	var diagnostics bytes.Buffer
	diagnostic := inboundsse.NewFiniteReplayDiagnostics(&diagnostics)
	recorder := httptest.NewRecorder()
	inboundsse.ThreadEventsHandler{Store: store, ReplayDiagnostics: diagnostic, Now: func() time.Time {
		return time.Date(2026, 7, 12, 3, 0, 0, 0, time.UTC)
	}}.ServeHTTP(recorder, request, fixture.threadID)
	diagnostic.Finish(request.Context().Err() != nil)
	return finiteReplayBenchmarkResult{recorder: recorder, reads: reader.calls, projections: projections, diagnostics: diagnostics.Bytes()}
}

func finiteReplayBenchmarkPhaseMilliseconds(t testing.TB, diagnostics []byte) map[string]int64 {
	t.Helper()
	result := map[string]int64{}
	for _, line := range strings.Split(strings.TrimSpace(string(diagnostics)), "\n") {
		if !strings.HasPrefix(line, inboundsse.FiniteReplayDiagnosticPrefix) {
			t.Fatal("benchmark diagnostics lost their fixed prefix")
		}
		var record struct {
			Phase          string `json:"phase"`
			ElapsedMs      int64  `json:"elapsedMs"`
			PhaseElapsedMs int64  `json:"phaseElapsedMs"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, inboundsse.FiniteReplayDiagnosticPrefix)), &record); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(record.Phase, "_end") && record.Phase != "request_end" {
			result[strings.TrimSuffix(record.Phase, "_end")] = record.PhaseElapsedMs
		}
		if record.Phase == "request_end" {
			result["request_total"] = record.ElapsedMs
		}
	}
	for _, required := range []string{"public_preflight", "highest_seq", "load_events", "terminal_groups", "accepted_final_groups", "project_public", "response_write", "request_total"} {
		if _, present := result[required]; !present {
			t.Fatalf("benchmark diagnostic phase missing: %s", required)
		}
	}
	return result
}

func (fixture finiteReplayArchiveBenchmarkFixture) assertReplay(t testing.TB, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE status=%d content_type=%s", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	units, groups, highest := 0, 0, 0
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		seq, ok := inboundsse.NumericSeq(event["seq"])
		if !ok || seq <= highest {
			t.Fatalf("replay cursor is not strictly increasing: seq=%d previous=%d", seq, highest)
		}
		highest = seq
		units++
		if stringField(event, "kind") == domainevent.GeneralTerminalDeliveryBatchKind {
			groups++
		}
		if stringField(event, "kind") == "cursor_advanced" || stringField(event, "kind") == "error" {
			t.Fatalf("replay substituted a failure or hidden-content cursor: kind=%s", stringField(event, "kind"))
		}
	}
	if fixture.rawCount < 499 || units < 447 || units != fixture.publicCount || groups != 26 || highest != fixture.highestSeq {
		t.Fatalf("replay scale/cursor mismatch: raw=%d units=%d expected=%d groups=%d highest=%d expected_highest=%d",
			fixture.rawCount, units, fixture.publicCount, groups, highest, fixture.highestSeq)
	}
}

func (fixture finiteReplayArchiveBenchmarkFixture) assertUnchanged(t testing.TB) {
	t.Helper()
	thread, err := fixture.handler.store.GetThread(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	primary, err := json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := fixture.handler.store.LoadEventsSince(fixture.threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		t.Fatalf("read final durable events: diagnostics=%d err=%v", len(loaded.Diagnostics), err)
	}
	events, err := json.Marshal(loaded.Events)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fixture.primary, primary) || !bytes.Equal(fixture.events, events) {
		t.Fatal("replay changed the normalized primary view or loaded events")
	}
	rawThread, err := os.ReadFile(fixture.handler.store.threadPath(fixture.threadID))
	if err != nil {
		t.Fatal(err)
	}
	rawEvents, err := os.ReadFile(fixture.handler.store.eventsPath(fixture.threadID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fixture.rawThread, rawThread) || !bytes.Equal(fixture.rawEvents, rawEvents) {
		t.Fatal("replay changed raw thread.json or events.jsonl bytes")
	}
}
