package turnterminal

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestGeneralTerminalCASBindingBindsExactContextAndText(t *testing.T) {
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewGeneralTerminalCASBindingV1(securityContext, "done")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGeneralTerminalCASBindingForContextV1(binding, securityContext, "done", "completed"); err != nil {
		t.Fatalf("validate exact binding: %v", err)
	}
	if err := ValidateGeneralTerminalCASBindingForContextV1(binding, securityContext, "other", "completed"); err == nil {
		t.Fatal("different text must fail closed")
	}
	stale := securityContext
	stale.ContextEpoch++
	if err := ValidateGeneralTerminalCASBindingForContextV1(binding, stale, "done", "completed"); err == nil {
		t.Fatal("different epoch must fail closed")
	}
}

func TestGeneralTerminalCASBindingRejectsUnknownFields(t *testing.T) {
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := NewGeneralTerminalCASBindingV1(securityContext, "done")
	if err != nil {
		t.Fatal(err)
	}
	value := GeneralTerminalCASBindingV1Map(binding)
	value["unknown"] = true
	if _, err := ParseGeneralTerminalCASBindingV1(value); err == nil {
		t.Fatal("unknown field must fail closed")
	}
}
