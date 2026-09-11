package turn

import (
	"strings"
	"testing"
	"time"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestGuardGeneralOutputRejectsConcreteCaseFactCandidate(t *testing.T) {
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-output", TurnID: "turn-general-output", WorkspaceRealPath: t.TempDir(),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := GuardGeneralOutput(securityContext, "张某实际控制甲公司，涉案金额为￥2,645,472.00。")
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Blocked || decision.Text != GeneralCaseFactCandidateBlockedText {
		t.Fatalf("case fact candidate was not replaced by the host boundary: %#v", decision)
	}
}

func TestGuardGeneralOutputKeepsOrdinaryAgentAnswer(t *testing.T) {
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-code", TurnID: "turn-general-code", WorkspaceRealPath: t.TempDir(),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := GuardGeneralOutput(securityContext, "  Updated the Go test and all focused checks pass.  ")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Blocked || decision.Text != "Updated the Go test and all focused checks pass." {
		t.Fatalf("ordinary answer changed unexpectedly: %#v", decision)
	}
}

func TestGuardGeneralOutputProjectsOrdinaryPIIBeforeTerminalCAS(t *testing.T) {
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-private", TurnID: "turn-general-private", WorkspaceRealPath: t.TempDir(),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := GuardGeneralOutput(securityContext, "Updated contact 13800138000 and email analyst@example.com; 2645472 records checked.")
	if err != nil {
		t.Fatal(err)
	}
	if decision.Text != "Updated contact [PHONE] and email [EMAIL]; 2645472 records checked." || decision.Blocked {
		t.Fatalf("ordinary terminal privacy projection = %#v", decision)
	}
}

func TestCompileOrdinaryResultSlotBlocksRawPIIFactLaunderingInCaseThread(t *testing.T) {
	securityContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-ordinary-output", TurnID: "turn-case-ordinary-output", WorkspaceRealPath: t.TempDir(),
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name      string
		candidate string
		boundary  string
	}{
		{name: "raw-PII-fact", candidate: "6222021234567890123 与张某有关。", boundary: GeneralCaseFactCandidateBlockedText},
		{name: "internal-reference", candidate: "ordinary result for " + string(reference), boundary: OrdinaryProviderResultWithheldText},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			slot, withheld, err := CompileOrdinaryResultSlot(securityContext, testCase.candidate)
			if err != nil {
				t.Fatal(err)
			}
			if !withheld || slot.Text != testCase.boundary || slot.CandidateOrigin != "host_fixed" ||
				slot.EvidenceAuthority || slot.CitationAuthority || slot.FactAnswerAllowed ||
				strings.Contains(slot.Text, testCase.candidate) || strings.Contains(slot.Text, string(reference)) {
				t.Fatalf("case ordinary hostile admission result = %#v withheld=%t", slot, withheld)
			}
		})
	}
}
