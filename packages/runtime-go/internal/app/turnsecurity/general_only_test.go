package turnsecurity

import (
	"context"
	"testing"
	"time"

	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type generalOnlyTurnObserver struct {
	observation domainsecurity.CaseBindingObservationV1
}

func (observer *generalOnlyTurnObserver) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	return observer.observation, nil
}

func TestGeneralOnlyFreezeMakesEveryCaseSignalBoundaryOnly(t *testing.T) {
	workspace := "/workspace/general-only"
	missing := generalOnlyTurnObservation(t, workspace, domainsecurity.CaseBindingStateMissing)
	tests := map[string]func(*WorkspaceSecurityAuthority){
		"risk intent":       func(value *WorkspaceSecurityAuthority) { value.RiskIntent = domainsecurity.RiskClassCase },
		"lexical injection": func(value *WorkspaceSecurityAuthority) { value.LexicalCaseRisk = true },
		"protected data":    func(value *WorkspaceSecurityAuthority) { value.ProtectedCaseData = true },
		"context change":    func(value *WorkspaceSecurityAuthority) { value.ContextChangingInput = true },
		"fork lineage":      func(value *WorkspaceSecurityAuthority) { value.TrustedCaseThread = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			observer := &generalOnlyTurnObserver{observation: missing}
			authority, err := threadriskauthorityapp.NewGeneralOnlyAuthority(observer)
			if err != nil {
				t.Fatal(err)
			}
			securityAuthority := WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority}
			mutate(&securityAuthority)
			securityContext, err := FreezeWorkspace(WorkspaceFreezeInput{
				Context: context.Background(), Authority: securityAuthority, Thread: map[string]any{},
				ThreadID: "thread-" + name, TurnID: "turn-" + name, Workspace: workspace,
				Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
			})
			if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
				domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) == nil {
				t.Fatalf("case signal did not become zero-execution boundary: context=%#v err=%v", securityContext, err)
			}
		})
	}
}

func TestGeneralOnlyFreezeRejectsCaseResumeAndNonMissingBindings(t *testing.T) {
	workspace := "/workspace/general-only-resume"
	previous, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-resume", TurnID: "turn-before-restart", WorkspaceRealPath: workspace,
		CaseID: "case-resume", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-resume-binding")),
		ContextEpoch: 4, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{
		domainsecurity.CaseBindingStateMissing, domainsecurity.CaseBindingStateValid,
		domainsecurity.CaseBindingStateInvalid, domainsecurity.CaseBindingStateUnreadable,
		domainsecurity.CaseBindingStateUnstable, domainsecurity.CaseBindingStateWorkspaceMissing,
	} {
		t.Run(state, func(t *testing.T) {
			observation := generalOnlyTurnObservation(t, workspace, state)
			observer := &generalOnlyTurnObserver{observation: observation}
			authority, err := threadriskauthorityapp.NewGeneralOnlyAuthority(observer)
			if err != nil {
				t.Fatal(err)
			}
			thread := map[string]any{}
			if state == domainsecurity.CaseBindingStateMissing {
				thread["securityState"] = PublicRecord(previous)
			}
			securityContext, err := FreezeWorkspace(WorkspaceFreezeInput{
				Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority},
				Thread: thread, ThreadID: "thread-resume", TurnID: "turn-after-restart", Workspace: workspace,
				Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
			})
			if err != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
				domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) == nil {
				t.Fatalf("resume/non-missing binding escaped boundary: state=%s context=%#v err=%v", state, securityContext, err)
			}
		})
	}
}

type forgedGeneralOnlyAuthority struct {
	head threadriskauthorityapp.Head
}

func (authority forgedGeneralOnlyAuthority) ResolveOrRaise(context.Context, threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	return authority.head, nil
}

func (forgedGeneralOnlyAuthority) ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext) error {
	return nil
}

func TestResolveRiskPublicationRejectsFakeGeneralOnlyAuthority(t *testing.T) {
	observation := generalOnlyTurnObservation(t, "/workspace/general-only", domainsecurity.CaseBindingStateMissing)
	forgedPolicy, err := domainsecurity.NewGeneralOnlyRiskPolicyV1("thread-other", observation.WorkspaceRealPath, observation.ObservationDigest)
	if err != nil {
		t.Fatal(err)
	}
	forgedBinding, err := domainsecurity.NewHostGeneralOnlyRiskAuthorityBindingV1(forgedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: forgedGeneralOnlyAuthority{head: threadriskauthorityapp.Head{
			Found: true, GeneralOnlyPolicy: forgedPolicy, RiskAuthorityBinding: forgedBinding,
		}},
		ThreadID: "thread-real", WorkspaceRealPath: observation.WorkspaceRealPath,
		Binding: observation, IssuedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("fake general-only authority for another thread was accepted")
	}
}

func generalOnlyTurnObservation(t *testing.T, workspace, state string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: state}
	if state == domainsecurity.CaseBindingStateValid {
		input.CaseID = "case-general-only"
		input.BindingSHA256 = domainsecurity.SHA256Hex([]byte("binding-file"))
		input.CaseBindingHash = domainsecurity.SHA256Hex([]byte("binding-hash"))
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
