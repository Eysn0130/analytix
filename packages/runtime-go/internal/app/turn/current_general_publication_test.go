package turn

import (
	"context"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCurrentGeneralPublicationAuthorityHoldsLeaseAcrossValidationAndPublish(t *testing.T) {
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-authority", TurnID: "turn-general-authority", WorkspaceRealPath: t.TempDir(),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	leaseHeld := false
	terminalHeld := false
	released := false
	authority := CurrentGeneralPublicationAuthority{
		AcquireContextEffect: func(ctx context.Context, frozen domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			if frozen != securityContext {
				t.Fatal("effect lease received a different frozen context")
			}
			leaseHeld = true
			return ctx, func() { leaseHeld = false; released = true }, nil
		},
		ValidateCurrent: func(_ context.Context, frozen domainsecurity.TurnSecurityContext) error {
			if !leaseHeld || frozen != securityContext {
				t.Fatal("current authority was checked outside the effect lease")
			}
			return nil
		},
		CaseLineageContains: func(threadID string) (bool, error) {
			if !leaseHeld || threadID != securityContext.ThreadID {
				t.Fatal("case lineage was checked outside the effect lease")
			}
			return false, nil
		},
		AcquireTerminalCAS: func(_ context.Context, frozen domainsecurity.TurnSecurityContext) (func(), error) {
			if !leaseHeld || terminalHeld || frozen != securityContext {
				t.Fatal("terminal permit was acquired outside the validated effect lease")
			}
			terminalHeld = true
			return func() { terminalHeld = false }, nil
		},
	}
	published := false
	if err := WithCurrentGeneralPublicationAuthority(context.Background(), authority, securityContext, func(context.Context) error {
		if !leaseHeld || !terminalHeld {
			t.Fatal("publication ran outside the effect lease or terminal permit")
		}
		published = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !published || !released || leaseHeld || terminalHeld {
		t.Fatalf("publication lease lifecycle = published:%t released:%t effect-held:%t terminal-held:%t", published, released, leaseHeld, terminalHeld)
	}
}

func TestCurrentGeneralPublicationAuthorityFailsClosedOnLineageOrInvalidLease(t *testing.T) {
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-denied", TurnID: "turn-general-denied", WorkspaceRealPath: t.TempDir(),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	base := CurrentGeneralPublicationAuthority{
		AcquireContextEffect: func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ValidateCurrent:     func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		CaseLineageContains: func(string) (bool, error) { return true, nil },
		AcquireTerminalCAS: func(context.Context, domainsecurity.TurnSecurityContext) (func(), error) {
			return func() {}, nil
		},
	}
	published := false
	if err := WithCurrentGeneralPublicationAuthority(context.Background(), base, securityContext, func(context.Context) error {
		published = true
		return nil
	}); err == nil || published {
		t.Fatal("signed case lineage reached general publication")
	}
	const sentinel = "甲公司向乙公司交付2645472件材料。张三与李四曾在同一项目工作。"
	if domainsecurity.ContainsProtectedCaseFactCandidate(sentinel) {
		t.Fatal("sentinel unexpectedly depends on the secondary text guard")
	}
	store := &finalizeStoreStub{status: "running", finishChanged: true}
	if _, err := CommitCurrentGeneralCompletion(context.Background(), base, CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, TerminalReason: "success",
	}); err == nil || len(store.atomicItems) != 0 || store.bundleCalls != 0 {
		t.Fatalf("case lineage reached general terminal CAS: items=%#v bundles=%d err=%v", store.atomicItems, store.bundleCalls, err)
	}
	base.CaseLineageContains = func(string) (bool, error) { return false, errors.New("registry unavailable") }
	if err := WithCurrentGeneralPublicationAuthority(context.Background(), base, securityContext, func(context.Context) error {
		published = true
		return nil
	}); err == nil || published {
		t.Fatal("unavailable case lineage authority reached general publication")
	}
	base.CaseLineageContains = func(string) (bool, error) { return false, nil }
	base.AcquireTerminalCAS = func(context.Context, domainsecurity.TurnSecurityContext) (func(), error) {
		return nil, context.Canceled
	}
	published = false
	if err := WithCurrentGeneralPublicationAuthority(context.Background(), base, securityContext, func(context.Context) error {
		published = true
		return nil
	}); !errors.Is(err, context.Canceled) || published {
		t.Fatalf("rejected terminal arbitration reached publication: published=%t err=%v", published, err)
	}
	base.AcquireTerminalCAS = func(context.Context, domainsecurity.TurnSecurityContext) (func(), error) {
		return func() {}, nil
	}
	base.AcquireContextEffect = func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
		return ctx, nil, nil
	}
	if err := WithCurrentGeneralPublicationAuthority(context.Background(), base, securityContext, func(context.Context) error {
		published = true
		return nil
	}); err == nil || published {
		t.Fatal("invalid effect lease reached general publication")
	}
}
