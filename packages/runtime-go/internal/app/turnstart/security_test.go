package turnstart

import (
	"context"
	"errors"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type transitionWorkspaceReaderStub struct {
	realPath      string
	realPathCalls int
	bindingCalls  int
}

func (stub *transitionWorkspaceReaderStub) WorkspaceRealPath(string) (string, error) {
	stub.realPathCalls++
	if stub.realPath == "" {
		return "", errors.New("workspace unavailable")
	}
	return stub.realPath, nil
}

func (stub *transitionWorkspaceReaderStub) ReadOptional(string) (domainsecurity.CaseBinding, bool, error) {
	stub.bindingCalls++
	return domainsecurity.CaseBinding{}, false, errors.New("case binding must not be read before writer admission")
}

type transitionSourceStub struct{ diagnosticsCalls int }

func (stub *transitionSourceStub) ServerDiagnostics() []any {
	stub.diagnosticsCalls++
	return []any{map[string]any{"forbidden": true}}
}

func TestBeginSecurityTransitionDoesNotMintProvisionalAuthority(t *testing.T) {
	state := subagentapp.NewRuntimeState()
	reader := &transitionWorkspaceReaderStub{realPath: "/workspace/canonical"}
	source := &transitionSourceStub{}
	transition, err := BeginSecurityTransition(
		context.Background(), state, testIdentityAuthority(), testIdentityPrincipal(), reader, source,
		map[string]any{"securityState": map[string]any{"invalid": "must remain unread"}},
		"thread-pre-context", "turn-pre-context", "/workspace/alias", time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer transition.Abort()
	if reader.realPathCalls != 1 || reader.bindingCalls != 0 || source.diagnosticsCalls != 0 {
		t.Fatalf("pre-context admission read authority material: realPath=%d binding=%d diagnostics=%d",
			reader.realPathCalls, reader.bindingCalls, source.diagnosticsCalls)
	}
	frozen, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pre-context", TurnID: "turn-pre-context", WorkspaceRealPath: reader.realPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transition.Prepare(context.Background(), frozen, time.Second); err != nil {
		t.Fatalf("canonical final context did not match pre-context concurrency identity: %v", err)
	}
}
