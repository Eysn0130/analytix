package publicationauthority

import (
	"context"
	"errors"
	"testing"
	"time"

	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type lineageStub map[string]bool

func (stub lineageStub) IsCaseThread(threadID string) bool { return stub[threadID] }

type terminalArbitratorStub struct {
	candidate func(context.Context, string, string, string) (func(), error)
	host      func(context.Context, string, string, string) (func(), error)
}

func (stub terminalArbitratorStub) AcquireCandidateTerminal(ctx context.Context, threadID, turnID, digest string) (func(), error) {
	return stub.candidate(ctx, threadID, turnID, digest)
}

func (stub terminalArbitratorStub) AcquireHostTerminal(ctx context.Context, threadID, turnID, digest string) (func(), error) {
	return stub.host(ctx, threadID, turnID, digest)
}

type finalizerStub struct {
	persist func(context.Context, evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error)
}

func (stub finalizerStub) PersistBoundary(ctx context.Context, input evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
	return stub.persist(ctx, input)
}

func TestPersistCurrentCaseCandidateHoldsEveryAuthorityThroughFinalizer(t *testing.T) {
	securityContext := caseSecurityContext(t, "thread-case-authority")
	effectHeld := false
	terminalHeld := false
	hostHeld := false
	finalized := false
	service := NewService(Dependencies{
		AcquireContextEffect: func(ctx context.Context, frozen domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			if frozen != securityContext {
				t.Fatal("context-effect lease received a different frozen context")
			}
			effectHeld = true
			return ctx, func() { effectHeld = false }, nil
		},
		CaseLineage: lineageStub{securityContext.ThreadID: true},
		TerminalArbitrator: terminalArbitratorStub{
			candidate: func(_ context.Context, threadID, turnID, digest string) (func(), error) {
				if !effectHeld || threadID != securityContext.ThreadID || turnID != securityContext.TurnID || digest != securityContext.ContextDigest {
					t.Fatal("candidate terminal permit was acquired outside the exact context-effect lease")
				}
				terminalHeld = true
				return func() { terminalHeld = false }, nil
			},
			host: func(context.Context, string, string, string) (func(), error) {
				t.Fatal("candidate publication acquired a host-fixed terminal")
				return nil, nil
			},
		},
		HostContext: func(ctx context.Context) (context.Context, context.CancelFunc) {
			if !effectHeld || !terminalHeld {
				t.Fatal("host context was created before publication authorities were held")
			}
			hostHeld = true
			hostCtx, cancel := context.WithCancel(ctx)
			return hostCtx, func() { hostHeld = false; cancel() }
		},
		CaseFinalizer: finalizerStub{persist: func(ctx context.Context, input evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
			if ctx == nil || ctx.Err() != nil || !effectHeld || !terminalHeld || !hostHeld || input.Context != securityContext {
				t.Fatal("Final Evidence Gate ran outside the complete publication authority")
			}
			finalized = true
			return evidenceapp.PersistCaseBoundaryResult{}, nil
		}},
	})

	_, err := service.PersistCurrentCaseCandidate(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Context: securityContext, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		TerminalReason: evidenceapp.TerminalSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !finalized || effectHeld || terminalHeld || hostHeld {
		t.Fatalf("authority lifecycle = finalized:%t effect:%t terminal:%t host:%t", finalized, effectHeld, terminalHeld, hostHeld)
	}
}

func TestCaseAndGeneralTerminalLineageFailClosedBeforePersistence(t *testing.T) {
	caseContext := caseSecurityContext(t, "thread-case-missing-lineage")
	generalContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-with-case-lineage", TurnID: "turn-general", WorkspaceRealPath: t.TempDir(),
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	finalizerCalled := false
	arbitratorCalled := false
	service := NewService(Dependencies{
		CaseLineage: lineageStub{generalContext.ThreadID: true},
		TerminalArbitrator: terminalArbitratorStub{
			candidate: func(context.Context, string, string, string) (func(), error) {
				arbitratorCalled = true
				return func() {}, nil
			},
			host: func(context.Context, string, string, string) (func(), error) {
				arbitratorCalled = true
				return func() {}, nil
			},
		},
		CaseFinalizer: finalizerStub{persist: func(context.Context, evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
			finalizerCalled = true
			return evidenceapp.PersistCaseBoundaryResult{}, nil
		}},
	})
	if _, err := service.PersistCurrentCaseFixed(context.Background(), evidenceapp.PersistCaseBoundaryInput{Context: caseContext}); err == nil {
		t.Fatal("case terminal without signed case lineage was accepted")
	}
	if err := service.WithCurrentGeneralFixedTerminal(context.Background(), generalContext, func(context.Context) error {
		finalizerCalled = true
		return nil
	}); err == nil {
		t.Fatal("general terminal with signed case lineage was accepted")
	}
	if finalizerCalled || arbitratorCalled {
		t.Fatalf("invalid lineage reached persistence: finalizer=%t arbitrator=%t", finalizerCalled, arbitratorCalled)
	}
}

func TestPublicationAuthorityRejectsIncompleteDependencies(t *testing.T) {
	caseContext := caseSecurityContext(t, "thread-case-incomplete-authority")
	service := NewService(Dependencies{CaseLineage: lineageStub{caseContext.ThreadID: true}})
	if _, err := service.PersistCurrentCaseCandidate(context.Background(), evidenceapp.PersistCaseBoundaryInput{Context: caseContext}); err == nil {
		t.Fatal("candidate case publication with incomplete dependencies was accepted")
	}
	if _, err := service.PersistCurrentCaseFixed(context.Background(), evidenceapp.PersistCaseBoundaryInput{Context: caseContext}); err == nil {
		t.Fatal("fixed case publication with incomplete dependencies was accepted")
	}
	if err := service.WithCurrentGeneralFixedTerminal(nil, domainsecurity.TurnSecurityContext{}, func(context.Context) error { return errors.New("unreachable") }); err == nil {
		t.Fatal("fixed general publication without a context was accepted")
	}
}

func caseSecurityContext(t *testing.T, threadID string) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-case", WorkspaceRealPath: t.TempDir(), CaseID: "case-a",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-a")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
