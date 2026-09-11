package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type blockingCaseTerminalFinalizer struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (finalizer *blockingCaseTerminalFinalizer) PersistBoundary(context.Context, evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
	finalizer.mu.Lock()
	finalizer.calls++
	entered := finalizer.entered
	release := finalizer.release
	finalizer.mu.Unlock()
	if entered != nil {
		select {
		case <-entered:
		default:
			close(entered)
		}
	}
	if release != nil {
		<-release
	}
	return evidenceapp.PersistCaseBoundaryResult{}, nil
}

func (finalizer *blockingCaseTerminalFinalizer) callCount() int {
	finalizer.mu.Lock()
	defer finalizer.mu.Unlock()
	return finalizer.calls
}

func TestCaseCandidateInterruptArbitrationIsBidirectional(t *testing.T) {
	securityContext := caseTerminalSecurityContext(t)
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{securityContext.ThreadID: true}}
	input := evidenceapp.PersistCaseBoundaryInput{
		Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		TerminalReason: evidenceapp.TerminalSuccess,
	}

	t.Run("interrupt_first", func(t *testing.T) {
		finalizer := &blockingCaseTerminalFinalizer{}
		handler := &runtimeServerHandler{
			caseFinalizer: finalizer, caseThreads: caseAuthority,
			control: controlapp.NewController(runtimeControlDriver{}),
		}
		cancelled := make(chan struct{})
		if !handler.runtimeControl().RegisterTurnCancel(securityContext.ThreadID, securityContext.TurnID, func() { close(cancelled) }) {
			t.Fatal("register case execution owner")
		}
		type reservation struct {
			release func()
			err     error
		}
		reserved := make(chan reservation, 1)
		go func() {
			_, release, err := handler.runtimeControl().ReserveInterruptTerminalAndWait(context.Background(), securityContext.ThreadID, securityContext.TurnID)
			reserved <- reservation{release: release, err: err}
		}()
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("interrupt did not cancel case execution owner")
		}
		if _, err := handler.runtimePublicationAuthority().PersistCurrentCaseCandidate(context.Background(), input); !errors.Is(err, context.Canceled) {
			t.Fatalf("interrupt-first case candidate error = %v", err)
		}
		if finalizer.callCount() != 0 {
			t.Fatal("interrupt-first case candidate reached Final Evidence Gate")
		}
		handler.runtimeControl().UnregisterTurnCancel(securityContext.ThreadID, securityContext.TurnID)
		result := <-reserved
		if result.err != nil || result.release == nil {
			t.Fatalf("interrupt reservation = %#v", result)
		}
		result.release()
	})

	t.Run("candidate_first", func(t *testing.T) {
		finalizer := &blockingCaseTerminalFinalizer{entered: make(chan struct{}), release: make(chan struct{})}
		handler := &runtimeServerHandler{
			caseFinalizer: finalizer, caseThreads: caseAuthority,
			control: controlapp.NewController(runtimeControlDriver{}),
		}
		cancelled := make(chan struct{}, 1)
		if !handler.runtimeControl().RegisterTurnCancel(securityContext.ThreadID, securityContext.TurnID, func() { cancelled <- struct{}{} }) {
			t.Fatal("register case execution owner")
		}
		candidateDone := make(chan error, 1)
		go func() {
			_, err := handler.runtimePublicationAuthority().PersistCurrentCaseCandidate(context.Background(), input)
			candidateDone <- err
		}()
		select {
		case <-finalizer.entered:
		case <-time.After(time.Second):
			t.Fatal("case candidate did not enter Final Evidence Gate")
		}
		reserved := make(chan struct {
			release func()
			err     error
		}, 1)
		go func() {
			_, release, err := handler.runtimeControl().ReserveInterruptTerminalAndWait(context.Background(), securityContext.ThreadID, securityContext.TurnID)
			reserved <- struct {
				release func()
				err     error
			}{release: release, err: err}
		}()
		select {
		case <-cancelled:
			t.Fatal("candidate-first case publication was cancelled")
		case <-time.After(25 * time.Millisecond):
		}
		close(finalizer.release)
		if err := <-candidateDone; err != nil {
			t.Fatal(err)
		}
		select {
		case result := <-reserved:
			t.Fatalf("interrupt returned before case owner release: %#v", result)
		case <-time.After(25 * time.Millisecond):
		}
		handler.runtimeControl().UnregisterTurnCancel(securityContext.ThreadID, securityContext.TurnID)
		result := <-reserved
		if result.err != nil || result.release == nil {
			t.Fatalf("candidate-first interrupt result = %#v", result)
		}
		result.release()
		if finalizer.callCount() != 1 {
			t.Fatalf("case Final Evidence Gate calls = %d", finalizer.callCount())
		}
	})
}

func TestCaseTerminalRejectsMissingSignedLineage(t *testing.T) {
	securityContext := caseTerminalSecurityContext(t)
	finalizer := &blockingCaseTerminalFinalizer{}
	handler := &runtimeServerHandler{
		caseFinalizer: finalizer,
		caseThreads:   &caseThreadAuthorityStub{threads: map[string]bool{}},
		control:       controlapp.NewController(runtimeControlDriver{}),
	}
	if _, err := handler.runtimePublicationAuthority().PersistCurrentCaseFixed(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		TerminalReason: evidenceapp.TerminalCancel,
	}); err == nil {
		t.Fatal("case terminal without signed lineage was accepted")
	}
	if finalizer.callCount() != 0 {
		t.Fatal("missing case lineage reached Final Evidence Gate")
	}
}

func TestCaseCandidateHoldsContextEffectThroughFinalEvidenceGateTail(t *testing.T) {
	securityContext := caseTerminalSecurityContext(t)
	finalizer := &blockingCaseTerminalFinalizer{entered: make(chan struct{}), release: make(chan struct{})}
	handler := &runtimeServerHandler{
		caseFinalizer: finalizer,
		caseThreads:   &caseThreadAuthorityStub{threads: map[string]bool{securityContext.ThreadID: true}},
		control:       controlapp.NewController(runtimeControlDriver{}),
	}
	if !handler.runtimeControl().RegisterTurnCancel(securityContext.ThreadID, securityContext.TurnID, func() {}) {
		t.Fatal("register case execution owner")
	}
	candidateDone := make(chan error, 1)
	go func() {
		_, err := handler.runtimePublicationAuthority().PersistCurrentCaseCandidate(context.Background(), evidenceapp.PersistCaseBoundaryInput{
			Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			TerminalReason: evidenceapp.TerminalSuccess,
		})
		candidateDone <- err
	}()
	select {
	case <-finalizer.entered:
	case <-time.After(time.Second):
		t.Fatal("case Final Evidence Gate did not enter")
	}
	type transitionResult struct {
		abort func()
		err   error
	}
	transitionDone := make(chan transitionResult, 1)
	go func() {
		transition, err := handler.runtimeSubagentState().BeginSecurityScopeTransitionIdentity(
			context.Background(), securityContext.ThreadID, securityContext.WorkspaceRealPath,
			securityContext.TenantID, securityContext.UserID,
		)
		if err != nil {
			transitionDone <- transitionResult{err: err}
			return
		}
		transitionDone <- transitionResult{abort: transition.Abort}
	}()
	select {
	case result := <-transitionDone:
		if result.abort != nil {
			result.abort()
		}
		t.Fatalf("context transition crossed case terminal tail: %v", result.err)
	case <-time.After(25 * time.Millisecond):
	}
	close(finalizer.release)
	if err := <-candidateDone; err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-transitionDone:
		if result.err != nil || result.abort == nil {
			t.Fatalf("context transition failed after case tail: %v", result.err)
		}
		result.abort()
	case <-time.After(time.Second):
		t.Fatal("context transition did not resume after case tail")
	}
	handler.runtimeControl().UnregisterTurnCancel(securityContext.ThreadID, securityContext.TurnID)
}

func caseTerminalSecurityContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-terminal", TurnID: "turn-case-terminal", WorkspaceRealPath: t.TempDir(), CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
