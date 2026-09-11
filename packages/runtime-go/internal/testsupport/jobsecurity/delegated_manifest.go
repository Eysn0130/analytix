package jobsecurity

import (
	"sort"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// BindDelegatedToolManifest gives hand-built security-bound job fixtures a
// valid host contract. Tests that exercise real provider advertisement must
// use toolcatalog.BuildDelegatedToolManifestV1 instead.
func BindDelegatedToolManifest(t testing.TB, record *domainjob.Record) {
	t.Helper()
	if record == nil {
		t.Fatal("job record is required")
	}
	record.ToolSchemaHash, record.DelegatedToolManifest = fixtureManifest(t, record.ToolScope)
	if record.SecurityBinding != nil && record.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID && strings.TrimSpace(record.Kind) == "subagent" {
		record.CaseDelegation = fixtureCaseDelegation(t, record.SecurityBinding)
		record.Name = domainjob.CaseDelegationNameV1
		record.Label = domainjob.CaseDelegationLabelV1
		record.Prompt = fixtureCaseDelegationPrompt(t, record.CaseDelegation, record.SecurityBinding)
	}
}

func BindDelegatedToolManifestRequest(t testing.TB, request *domainjob.StartRequest) {
	t.Helper()
	if request == nil {
		t.Fatal("job start request is required")
	}
	request.ToolSchemaHash, request.DelegatedToolManifest = fixtureManifest(t, request.ToolScope)
	if request.SecurityBinding != nil && request.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID && strings.TrimSpace(request.Kind) == "subagent" {
		request.CaseDelegation = fixtureCaseDelegation(t, request.SecurityBinding)
		request.Name = domainjob.CaseDelegationNameV1
		request.Label = domainjob.CaseDelegationLabelV1
		request.Prompt = fixtureCaseDelegationPrompt(t, request.CaseDelegation, request.SecurityBinding)
	}
}

func fixtureCaseDelegation(t testing.TB, binding *domainjob.SecurityBinding) *domainjob.CaseDelegationContextV1 {
	t.Helper()
	context, err := domainjob.NewCaseDelegationContextV1(binding, domainjob.CaseDelegationSemanticContextV1{
		TaskKind:    domainjob.CaseDelegationTaskKindV1,
		Currentness: domaincaseentity.SnapshotCurrentV1,
		Entities: []domainjob.CaseDelegatedEntitySemanticV1{{
			Alias:                domaincaseentity.ModelEntityAliasV1("acct:1"),
			EntityType:           domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			FinancialAccountType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			ResolutionDigest:     domainsecurity.SHA256Hex([]byte("analytix.test.case-delegation-resolution/v1")),
		}},
		Claims: []domainjob.CaseDelegatedClaimReferenceV1{}, Evidence: []domainjob.CaseDelegatedEvidenceReferenceV1{},
		Continuations: []domainjob.CaseDelegatedContinuationReferenceV1{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func fixtureCaseDelegationPrompt(t testing.TB, context *domainjob.CaseDelegationContextV1, binding *domainjob.SecurityBinding) string {
	t.Helper()
	prompt, err := domainjob.CaseDelegationProviderPromptV1(context, binding)
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}

func fixtureManifest(t testing.TB, scope []string) (string, *domainjob.DelegatedToolManifestV1) {
	t.Helper()
	values := append([]string(nil), scope...)
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	sort.Strings(values)
	schemaHash := domainsecurity.SHA256Hex([]byte("analytix.test.delegated-tool-schema/v1\x00" + strings.Join(values, "\x00")))
	mcpAuthorityHash := domainsecurity.SHA256Hex([]byte("analytix.test.delegated-mcp-authority/v1\x00" + strings.Join(values, "\x00")))
	manifest, err := domainjob.NewDelegatedToolManifestV1(scope, schemaHash, mcpAuthorityHash)
	if err != nil {
		t.Fatal(err)
	}
	return schemaHash, manifest
}
