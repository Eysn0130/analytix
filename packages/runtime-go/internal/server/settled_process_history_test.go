package server

import (
	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	finaladapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	historystore "analytix.local/runtime-go/internal/adapters/outbound/processdisplay"
	processapp "analytix.local/runtime-go/internal/app/processdisplay"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	model "analytix.local/runtime-go/internal/domain/model"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	terminal "analytix.local/runtime-go/internal/domain/terminal"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func bindSettledHistoryPrimaryReaderV1(t *testing.T, store *DurableEventSessionStore) {
	t.Helper()
	reader, err := finaladapter.NewAcceptedFinalCASReader(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindPrimaryThreadReaderV1(reader); err != nil {
		t.Fatal(err)
	}
}

func TestSettledHistorySealWindowCannotResurrectDeletedPrimaryV1(t *testing.T) {
	s, threadID, turnID, frozen := newGeneralTerminalOutboxFixture(t)
	bindSettledHistoryPrimaryReaderV1(t, s)
	calls := 0
	s.settledProcessHistory = func(node map[string]any, id string) *display.SettledHistoryAnchorV1 {
		calls++
		_, _, w, err := processHistoryTerminalV1(node, id)
		if err != nil {
			t.Fatal(err)
		}
		deleted, err := s.DeleteThread(threadID)
		if err != nil || !deleted {
			t.Fatal("normal owner delete did not execute outside store lock", err)
		}
		return &display.SettledHistoryAnchorV1{SchemaVersion: display.SettledHistoryAnchorSchemaV1, ThreadID: threadID, TurnID: turnID, ContextDigest: frozen.ContextDigest, Terminal: w, Availability: "available", RecordDigest: strings.Repeat("a", 64)}
	}
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: s, SecurityContext: frozen, ThreadID: threadID, TurnID: turnID, FinishedAt: "2026-10-14T01:00:00Z"})
	if err == nil || result.Changed || calls != 1 {
		t.Fatal("seal/delete race admitted terminal writer")
	}
	snapshot, err := s.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
	if err != nil || stringField(snapshot.Thread, "status") != "deleted" {
		t.Fatal("terminal writer resurrected Primary", err)
	}
	turns, _ := snapshot.Thread["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if turn[display.SettledHistoryFieldV1] != nil {
			t.Fatal("history anchor patched losing/deleted turn")
		}
	}
}

func TestSettledHistoryActualPrimaryProtectedHTTPRestoreAndTombstoneV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	root := filepath.Join(t.TempDir(), "process-history-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	h.processHistory = storage
	h.store.settledProcessHistory = h.prepareSettledProcessHistoryV1
	p := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if p == nil {
		t.Fatal("actual host source denied")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "history-native"})
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "history-native", Text: "synthetic closed private reasoning"})
	p.FinishV1()
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: h.store, SecurityContext: pending.SecurityContext, ThreadID: pending.ThreadID, TurnID: pending.TurnID, FinishedAt: "2026-10-14T01:00:00Z"})
	if err != nil || !result.Changed {
		t.Fatal("real terminal/anchor commit failed", err)
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, _, err := processHistoryTerminalV1(primary.Thread, pending.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := display.ParseSettledHistoryAnchorV1(turn[display.SettledHistoryFieldV1])
	if err != nil || anchor.Availability != "available" {
		t.Fatal("terminal did not atomically contain actual CAS anchor", err)
	}
	stored, err := storage.Read(context.Background(), anchor)
	if err != nil || len(stored.Blocks) != 1 || stored.Blocks[0].Display.Body == nil {
		t.Fatal("real sealed projected source absent", err)
	}
	oldGrant := processSeamAuthorizeV1(t, h, pending, processSeamViewerV1)
	oldBody, _ := processSeamReadV1(t, h, processSeamViewerV1, stored.Blocks[0].Display.BlockID)
	if oldGrant.Status != "authorized" || oldGrant.Scope == nil || oldBody.Status != "snapshot" || len(oldBody.Blocks) != 1 || oldBody.Blocks[0].Body == nil {
		t.Fatal("old viewer was never actually granted and expanded")
	}
	if err := h.processDisplay.Close(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	storage, err = historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	h.processHistory = storage
	h.processDisplay = processapp.NewServiceV1(runtimeProcessDisplayAccessV1{h}, processapp.DefaultLimitsV1())
	defer h.processDisplay.Close()
	if response, _ := processSeamReadV1(t, h, processSeamViewerV1); response.Status != "unavailable" {
		t.Fatal("old runtime viewer handle restored")
	}
	const freshViewer = "c81f05e1-98a6-4d9b-854c-927d11591d37"
	freshGrant, _ := processSeamRequestV1(t, h, httpapi.ProcessLocalDisplayReadPathV1, map[string]any{"schemaVersion": 2, "action": "authorize", "viewerToken": freshViewer, "documentNonce": "d739761d-aa55-43be-9b60-9e6388996d85", "threadId": pending.ThreadID, "turnId": pending.TurnID})
	if freshGrant.Status != "authorized" || freshGrant.Scope == nil || freshGrant.Scope.ProcessIncarnation == oldGrant.Scope.ProcessIncarnation {
		t.Fatal("fresh original own history authorization failed")
	}
	response, _ := processSeamReadV1(t, h, freshViewer, stored.Blocks[0].Display.BlockID)
	if response.Status != "snapshot" || len(response.Blocks) != 1 || response.Blocks[0].Body == nil || response.Blocks[0].Body.Text != "synthetic closed private reasoning" {
		t.Fatal("typed protected restored body failed")
	}
	assertProcessSeamPublicIsolationV1(t, h, pending, response.Blocks[0], "synthetic closed private reasoning", display.SettledHistoryFieldV1)
	if deleted, err := h.store.DeleteThread(pending.ThreadID); err != nil || !deleted {
		t.Fatal("normal tombstone delete failed", err)
	}
	if response, _ := processSeamReadV1(t, h, freshViewer, stored.Blocks[0].Display.BlockID); response.Status != "unavailable" {
		t.Fatal("tombstone retained body read authority")
	}
	if _, err := storage.Read(context.Background(), anchor); err != nil {
		t.Fatal("first unit silently changed physical retention/purge policy")
	}
}

func TestSettledHistoryCASBlobWithoutTerminalPrimaryIsUnreadableV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	root := filepath.Join(t.TempDir(), "process-history-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	h.processHistory = storage
	defer h.processDisplay.Close()
	var sealed *display.SettledHistoryAnchorV1
	h.store.settledProcessHistory = func(node map[string]any, id string) *display.SettledHistoryAnchorV1 {
		sealed = h.prepareSettledProcessHistoryV1(node, id)
		return sealed
	}
	p := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if p == nil {
		t.Fatal("host producer denied")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "orphan"})
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "orphan", Text: "closed orphan"})
	p.FinishV1()
	h.store.beforeTerminalWrite = func() error { return errors.New("synthetic Primary cut") }
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: h.store, SecurityContext: pending.SecurityContext, ThreadID: pending.ThreadID, TurnID: pending.TurnID, FinishedAt: "2026-10-14T01:00:00Z"})
	if err == nil || result.Changed {
		t.Fatal("Primary failure disappeared")
	}
	if sealed == nil || sealed.Availability != "available" {
		t.Fatal("fixture did not reach actual seal/Put")
	}
	stored, readErr := storage.Read(context.Background(), *sealed)
	if readErr != nil || len(stored.Blocks) != 1 || stored.Blocks[0].Display.Body == nil || stored.Blocks[0].Display.Body.Text != "closed orphan" {
		t.Fatal("actual sealed orphan body absent", readErr)
	}
	primary, readErr := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, raw := range primary.Thread["turns"].([]any) {
		turn, _ := raw.(map[string]any)
		if turn[display.SettledHistoryFieldV1] != nil {
			t.Fatal("Primary cut still installed historical anchor")
		}
	}
	h.store.beforeTerminalWrite = nil
	if _, _, err := (runtimeProcessDisplayAccessV1{h}).ReadSettledHistoryV1(context.Background(), pending.ThreadID, pending.TurnID); err == nil {
		t.Fatal("orphan blob acquired historical authority")
	}
}

func TestNormalShutdownPersistsCancelledHistoryAfterOwnedTerminalDrainV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	root := filepath.Join(t.TempDir(), "process-history-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	h.processHistory = storage
	h.store.settledProcessHistory = h.prepareSettledProcessHistoryV1
	p := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if p == nil {
		t.Fatal("real producer unavailable")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "closed"})
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "closed", Text: "closed shutdown history"})
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeIndex: 1, NativeID: "unfinished", Text: "half-running shutdown source"})
	finish, admitted := h.runtimeControl().BeginTurnOperation()
	if !admitted {
		t.Fatal("real turn operation denied")
	}
	turnCtx, cancelTurn := context.WithCancel(context.Background())
	defer cancelTurn()
	if !h.runtimeControl().RegisterTurnCancel(pending.ThreadID, pending.TurnID, cancelTurn) {
		finish()
		t.Fatal("real cancellation owner denied")
	}
	settled := make(chan error, 1)
	go func() {
		defer finish()
		defer h.runtimeControl().UnregisterTurnCancel(pending.ThreadID, pending.TurnID)
		<-turnCtx.Done()
		release, err := h.runtimeControl().AcquireHostTerminal(context.Background(), pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest)
		if err != nil {
			settled <- err
			return
		}
		defer release()
		p.FinishV1()
		failure, _ := terminal.GeneralFailureRecordV1("cancel")
		result, err := turnapp.CommitGeneralFailureTerminal(turnapp.PersistFailureInput{Store: h.store, SecurityContext: pending.SecurityContext, TerminalReason: "cancel", ThreadID: pending.ThreadID, TurnID: pending.TurnID, FinishedAt: "2026-10-14T01:00:00Z", Failure: failure, Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true}})
		if err == nil && !result.Changed {
			err = errors.New("real cancellation terminal did not commit")
		}
		settled <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.Shutdown(ctx); err != nil {
		t.Fatal("normal shutdown drain failed", err)
	}
	if err := <-settled; err != nil {
		t.Fatal(err)
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, _, err := processHistoryTerminalV1(primary.Thread, pending.TurnID)
	if err != nil || stringField(turn, "status") != "aborted" {
		t.Fatal("shutdown fabricated terminal truth", err)
	}
	anchor, err := display.ParseSettledHistoryAnchorV1(turn[display.SettledHistoryFieldV1])
	if err != nil || anchor.Availability != "available" {
		t.Fatal("normal terminal drain lost private anchor", err)
	}
	reopened, err := historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	record, err := reopened.Read(context.Background(), anchor)
	body, codecErr := display.SettledHistoryBytesV1(record)
	if err != nil || codecErr != nil || !strings.Contains(string(body), "closed shutdown history") || strings.Contains(string(body), "half-running shutdown source") {
		t.Fatal("restart lost closed history or restored unfinished source", err, codecErr)
	}
	if response, _ := processSeamReadV1(t, h, processSeamViewerV1); response.Status != "unavailable" {
		t.Fatal("shutdown retained viewer authority")
	}
}

func TestShutdownDrainWithoutExactTerminalClaimCannotSealHistoryV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	root := filepath.Join(t.TempDir(), "process-history-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	h.processHistory = storage
	h.store.settledProcessHistory = h.prepareSettledProcessHistoryV1
	p := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if p == nil {
		t.Fatal("real producer unavailable")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "closed"})
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "closed", Text: "must not seal without exact claim"})
	p.FinishV1()
	h.processDisplay.BeginShutdownDrainV1()
	h.runtimeControl().BeginShutdown()
	if h.runtimeControl().OwnsTerminalClaim(pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest) {
		t.Fatal("drain invented a terminal claim")
	}
	release, err := h.runtimeControl().AcquireHostTerminal(context.Background(), pending.ThreadID, pending.TurnID, "wrong-context")
	if err != nil {
		t.Fatal("fixture did not acquire mismatched claim", err)
	}
	defer release()
	if h.runtimeControl().OwnsTerminalClaim(pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest) {
		t.Fatal("different terminal context matched")
	}
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: h.store, SecurityContext: pending.SecurityContext, ThreadID: pending.ThreadID, TurnID: pending.TurnID, FinishedAt: "2026-10-14T01:00:00Z"})
	if err != nil || !result.Changed {
		t.Fatal("history refusal changed terminal truth", err)
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, _, err := processHistoryTerminalV1(primary.Thread, pending.TurnID)
	anchor, parseErr := display.ParseSettledHistoryAnchorV1(turn[display.SettledHistoryFieldV1])
	if err != nil || parseErr != nil || anchor.Availability != "unavailable" {
		t.Fatal("global shutdown drain bypassed exact terminal claim", err, parseErr)
	}
	defer h.processDisplay.Close()
}

func TestShutdownTimeoutRetainsHistoryOwnerUntilTerminalAndRetryV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
	defer h.processDisplay.Close()
	root := filepath.Join(t.TempDir(), "process-history-v1")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := historystore.NewStore(context.Background(), root, access, historystore.LimitsV1{Bytes: 1 << 20, Records: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	h.processHistory = storage
	h.store.settledProcessHistory = h.prepareSettledProcessHistoryV1
	p := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if p == nil {
		t.Fatal("real producer unavailable")
	}
	p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "retry"})
	p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "retry", Text: "retained across failed drain"})
	finish, admitted := h.runtimeControl().BeginTurnOperation()
	if !admitted {
		t.Fatal("real turn operation denied")
	}
	defer finish()
	turnCtx, cancelTurn := context.WithCancel(context.Background())
	defer cancelTurn()
	if !h.runtimeControl().RegisterTurnCancel(pending.ThreadID, pending.TurnID, cancelTurn) {
		t.Fatal("real cancellation owner denied")
	}
	defer h.runtimeControl().UnregisterTurnCancel(pending.ThreadID, pending.TurnID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := h.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) || !h.processDisplay.ShutdownDrainingV1() || turnCtx.Err() == nil {
		t.Fatal("timeout lost bounded owner or failed to cancel actual turn", err)
	}
	release, err := h.runtimeControl().AcquireHostTerminal(context.Background(), pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	p.FinishV1()
	failure, _ := terminal.GeneralFailureRecordV1("cancel")
	result, err := turnapp.CommitGeneralFailureTerminal(turnapp.PersistFailureInput{Store: h.store, SecurityContext: pending.SecurityContext, TerminalReason: "cancel", ThreadID: pending.ThreadID, TurnID: pending.TurnID, FinishedAt: "2026-10-14T01:00:00Z", Failure: failure, Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true}})
	release()
	if err != nil || !result.Changed {
		t.Fatal("retained owner could not settle real terminal", err)
	}
	primary, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _, _, err := processHistoryTerminalV1(primary.Thread, pending.TurnID)
	anchor, parseErr := display.ParseSettledHistoryAnchorV1(turn[display.SettledHistoryFieldV1])
	if err != nil || parseErr != nil || anchor.Availability != "available" {
		t.Fatal("failed drain lost completed capture", err, parseErr)
	}
	if _, err := storage.Read(context.Background(), anchor); err != nil {
		t.Fatal("failed shutdown prematurely closed CAS owner", err)
	}
	h.runtimeControl().UnregisterTurnCancel(pending.ThreadID, pending.TurnID)
	finish()
	if err := h.Shutdown(context.Background()); err != nil || h.processDisplay.ShutdownDrainingV1() {
		t.Fatal("ordinary shutdown retry failed", err)
	}
}

func TestSettledHistorySealPanicReacquiresPrimaryMutexAndPropagatesV1(t *testing.T) {
	s, threadID, turnID, frozen := newGeneralTerminalOutboxFixture(t)
	bindSettledHistoryPrimaryReaderV1(t, s)
	sentinel := errors.New("synthetic seal panic")
	s.settledProcessHistory = func(map[string]any, string) *display.SettledHistoryAnchorV1 { panic(sentinel) }
	var caught any
	func() {
		defer func() { caught = recover() }()
		turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{Store: s, SecurityContext: frozen, ThreadID: threadID, TurnID: turnID, FinishedAt: "2026-10-14T01:00:00Z"})
	}()
	if caught != sentinel {
		t.Fatal("original seal panic was swallowed or replaced", caught)
	}
	if !s.mu.TryLock() {
		t.Fatal("panic left Primary mutex held")
	}
	s.mu.Unlock()
	primary, err := s.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range primary.Thread["turns"].([]any) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID && (stringField(turn, "status") != "running" || turn[display.SettledHistoryFieldV1] != nil) {
			t.Fatal("seal panic committed terminal or history anchor")
		}
	}
}

func TestDeleteRetiresExactProcessOwnerAfterPrimaryIncludingSidecarFailureV1(t *testing.T) {
	for _, failSidecar := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "primary-committed-sidecar-failed"}[failSidecar], func(t *testing.T) {
			h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "unused.txt"})
			defer h.processDisplay.Close()
			p := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
			if p == nil {
				t.Fatal("real capture producer denied")
			}
			p.FrameV1(model.ProcessFrameV1{Kind: "start", ProviderSequence: 1, Attempt: 1, NativeID: "delete"})
			p.FrameV1(model.ProcessFrameV1{Kind: "end", ProviderSequence: 1, Attempt: 1, NativeID: "delete", Text: "deleted private capture"})
			retired := 0
			h.store.retireProcessThread = func(id string) {
				if id != pending.ThreadID || !h.store.mu.TryLock() {
					t.Fatal("retirement ran under Primary lock or for another thread")
				}
				h.store.mu.Unlock()
				retired++
				h.processDisplay.RevokeThreadV1(id)
			}
			if _, err := h.store.DeleteThreadIfBaseline(pending.ThreadID, strings.Repeat("f", 64)); err == nil || retired != 0 {
				t.Fatal("baseline failure retired private authority")
			}
			if failSidecar {
				path := h.store.metadataPath(pending.ThreadID)
				if err := os.Rename(path, path+".exact-before-delete"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			deleted, err := h.store.DeleteThread(pending.ThreadID)
			if failSidecar && err == nil || !failSidecar && (err != nil || !deleted) || retired != 1 {
				t.Fatal("tombstone/error result or exact retirement changed", deleted, err, retired)
			}
			primary, readErr := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), pending.ThreadID)
			if readErr != nil || stringField(primary.Thread, "status") != "deleted" {
				t.Fatal("fixture did not reach real Primary tombstone", readErr)
			}
			p.FinishV1()
			if record, err := h.processDisplay.FreezeSettledV1(p.BindingV2(), display.TerminalWitnessV1{Status: "aborted", FinishedAt: "2026-10-14T01:00:00Z", AuthorityDigest: strings.Repeat("a", 64), SourceDigest: strings.Repeat("b", 64)}); err == nil || len(record.Blocks) != 0 {
				t.Fatal("deleted thread retained volatile capture")
			}
		})
	}
}
