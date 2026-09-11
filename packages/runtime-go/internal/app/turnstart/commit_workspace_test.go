package turnstart

import (
	"context"
	"errors"
	"testing"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrepareWorkspaceBeforeFreezeRunsUnderTransitionWriter(t *testing.T) {
	state := subagentapp.NewRuntimeState()
	reader := &transitionWorkspaceReaderStub{realPath: "/workspace/canonical"}
	transition, err := BeginSecurityTransition(
		context.Background(), state, testIdentityAuthority(), testIdentityPrincipal(), reader, nil, map[string]any{},
		"thread-workspace-prepare", "turn-workspace-prepare", "/workspace/alias", time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transition.Abort()

	called := false
	err = prepareWorkspaceBeforeFreeze(PreparedCommitInput{
		Context: context.Background(), Transition: transition, Reader: reader, Workspace: "/workspace/alias",
		PrepareWorkspaceBeforeFreeze: func(ctx context.Context, workspaceRealPath string) error {
			called = true
			if workspaceRealPath != reader.realPath || reader.bindingCalls != 0 {
				return errors.New("workspace preparation did not precede the authoritative freeze")
			}
			readCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
			defer cancel()
			release, acquireErr := state.EffectGate().AcquireTransitionScopeRead(readCtx, effectgateapp.TransitionScope{
				ThreadID: "thread-workspace-prepare", WorkspaceRealPath: reader.realPath,
				TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			})
			if release != nil {
				release()
			}
			if !errors.Is(acquireErr, context.DeadlineExceeded) {
				return errors.New("workspace preparation did not hold the transition writer")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("workspace preparation hook was not called")
	}
}

func TestPrepareWorkspaceBeforeFreezeRejectsScopeDrift(t *testing.T) {
	state := subagentapp.NewRuntimeState()
	reader := &transitionWorkspaceReaderStub{realPath: "/workspace/canonical"}
	transition, err := BeginSecurityTransition(
		context.Background(), state, testIdentityAuthority(), testIdentityPrincipal(), reader, nil, map[string]any{},
		"thread-workspace-drift", "turn-workspace-drift", "/workspace/alias", time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transition.Abort()

	err = prepareWorkspaceBeforeFreeze(PreparedCommitInput{
		Context: context.Background(), Transition: transition, Reader: reader, Workspace: "/workspace/alias",
		PrepareWorkspaceBeforeFreeze: func(context.Context, string) error {
			reader.realPath = "/workspace/rebound"
			return nil
		},
	})
	if err == nil || err.Error() != "turn start workspace changed during preparation" {
		t.Fatalf("workspace identity drift was not rejected: %v", err)
	}
}
