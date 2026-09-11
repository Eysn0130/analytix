package subagent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCaseDelegationAliasSubsetParserIsClosedAndBounded(t *testing.T) {
	aliases, err := caseDelegationAliasesFromPromptV1("compare card:2 with acct:1")
	if err != nil || !reflect.DeepEqual(aliases, []domaincaseentity.ModelEntityAliasV1{"acct:1", "card:2"}) {
		t.Fatalf("valid delegated aliases were not canonicalized: aliases=%#v err=%v", aliases, err)
	}
	for _, prompt := range []string{
		"no delegated aliases here",
		"duplicate acct:1 then acct:1",
		"unsupported person:1",
		"unsupported future:1 beside acct:1",
		"malformed acct:01",
		"overflow acct:4294967296",
		strings.Repeat("x", maxCaseDelegationRawPromptBytesV1+1),
	} {
		if aliases, err := caseDelegationAliasesFromPromptV1(prompt); err == nil || aliases != nil {
			t.Fatalf("hostile delegated alias prompt was accepted: prompt=%q aliases=%#v err=%v", prompt, aliases, err)
		}
	}
	overflow := make([]string, 0, domainjob.MaxCaseDelegationEntitiesV1+1)
	for ordinal := 1; ordinal <= domainjob.MaxCaseDelegationEntitiesV1+1; ordinal++ {
		overflow = append(overflow, fmt.Sprintf("acct:%d", ordinal))
	}
	if aliases, err := caseDelegationAliasesFromPromptV1(strings.Join(overflow, " ")); err == nil || aliases != nil {
		t.Fatalf("unbounded delegated alias subset was accepted: aliases=%#v err=%v", aliases, err)
	}
}

func TestUnboundGeneralSubagentPreservesNaturalLanguageAndLifecycleOptions(t *testing.T) {
	const raw = "Inspect the general code path and explain the retry behavior naturally."
	request := TaskRequest{
		Prompt: raw, Name: "General reviewer", Label: "General retry review",
		ContinueFrom: "job-previous", ForkFrom: "", RunInBackground: true,
	}
	bound, err := BindCaseDelegationV1(context.Background(), BindCaseDelegationInputV1{Request: request})
	if err != nil || !reflect.DeepEqual(bound.Request, request) || bound.Context != nil {
		t.Fatalf("unbound general task was changed by case delegation: bound=%#v err=%v", bound, err)
	}
	request.ContinueFrom = ""
	request.ForkFrom = "job-source"
	bound, err = BindCaseDelegationV1(context.Background(), BindCaseDelegationInputV1{Request: request})
	if err != nil || bound.Request.Prompt != raw || bound.Request.ForkFrom != "job-source" || !bound.Request.RunInBackground {
		t.Fatalf("unbound fork/background task lost compatibility: bound=%#v err=%v", bound, err)
	}
}

func TestCaseBoundProviderChildCallAuthorityDoesNotTrustPromptAliases(t *testing.T) {
	caseContext, _ := runtimeStateSecurityFixture(t, "thread-case-child-call", "turn-case-child-call", "case-current", "snapshot-current", 1)
	for _, name := range []string{"task", "delegate_task", "parallel_tasks"} {
		if !CaseBoundProviderChildCallRequiresCaseAuthorityV1(caseContext, name) {
			t.Fatalf("case-bound provider child call %q did not require case authority", name)
		}
	}
	if CaseBoundProviderChildCallRequiresCaseAuthorityV1(caseContext, "read") ||
		CaseBoundProviderChildCallRequiresCaseAuthorityV1(domainsecurity.TurnSecurityContext{CaseID: "case-shaped-only"}, "task") ||
		CaseBoundProviderChildCallRequiresCaseAuthorityV1(domainsecurity.TurnSecurityContext{CaseID: domainsecurity.UnboundCaseID}, "task") {
		t.Fatal("ordinary or unrelated tool call was upgraded to case authority")
	}
}
