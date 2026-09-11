package loop

import (
	"reflect"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCasePublicationRequiredIncludesUnboundBoundaryOnlyV2(t *testing.T) {
	workspace := "/workspace/case-boundary"
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("boundary-risk-policy")),
		RiskClass:              domainsecurity.RiskClassCase,
		Disposition:            domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:       domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"boundary-binding-observation",
		)),
		BlockerCode: domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-boundary", TurnID: "turn-boundary", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 1, IssuedAt: time.Unix(1, 0), PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !CasePublicationRequired(boundary) {
		t.Fatal("unbound case boundary-only V2 context bypassed the Final Evidence Gate")
	}
	legacyCase := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: workspace, CaseID: "case-v1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-v1")), DatasetSnapshotID: "snapshot-v1",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-v1")), ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	if !CasePublicationRequired(legacyCase) {
		t.Fatal("structurally valid historical case context lost conservative final-gate classification")
	}
}

func TestDefaultSystemPromptCarriesAnalytixProviderMetadata(t *testing.T) {
	prompt := DefaultSystemPrompt("deepseek", "deepseek-v4-pro")
	for _, expected := range []string{
		"You are Analytix",
		"provider=deepseek",
		"model=deepseek-v4-pro",
		"Do not claim to be Claude or Anthropic",
		"Never put private reasoning, analysis-channel text, thinking tags, or provider-native reasoning fields into tool arguments",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("system prompt missing %q: %s", expected, prompt)
		}
	}
}

func TestCaseFundAnalysisPolicyScopesAvailableTools(t *testing.T) {
	policy := CaseFundAnalysisPolicyForWorkspace(true, "请分析资金流向 top10", []string{
		"mcp__other__rank",
		"mcp__analytix-fund-analysis__rank",
		"mcp__analytix_funds__count",
	})
	if !policy.Active {
		t.Fatal("case fund prompt in bound workspace should activate policy")
	}
	if policy.OrdinaryWorkRequested || !policy.ProviderUsesCaseDataAuthority() || policy.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("pure case request has the wrong effect disposition: %#v", policy)
	}
	if strings.Contains(policy.SystemInstruction, "tools are unavailable") {
		t.Fatalf("available tools should not use unavailable instruction: %s", policy.SystemInstruction)
	}
	want := []string{"mcp__analytix-fund-analysis__rank", "mcp__analytix_funds__count"}
	if len(policy.ToolScope) != len(want) {
		t.Fatalf("unexpected tool scope length: %#v", policy.ToolScope)
	}
	for index, expected := range want {
		if policy.ToolScope[index] != expected {
			t.Fatalf("tool scope should be sorted and filtered, got %#v", policy.ToolScope)
		}
	}
}

func TestCaseFundAnalysisPolicyUsesUnavailableSentinel(t *testing.T) {
	policy := CaseFundAnalysisPolicyForWorkspace(true, "账户交易明细", nil)
	if !policy.Active {
		t.Fatal("case fund prompt should activate even when tools are unavailable")
	}
	if !policy.SourceUnavailable || policy.BoundaryAnswer != CaseFundSourceUnavailableAnswer() {
		t.Fatalf("missing host-owned unavailable boundary: %#v", policy)
	}
	if policy.OrdinaryWorkRequested || policy.ProviderUsesCaseDataAuthority() || !policy.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("pure unavailable case request has the wrong effect disposition: %#v", policy)
	}
	if len(policy.ToolScope) != 1 || policy.ToolScope[0] != CaseFundUnavailableToolSentinel {
		t.Fatalf("missing unavailable sentinel: %#v", policy.ToolScope)
	}
	if !strings.Contains(policy.SystemInstruction, "Current-case fund-analysis tools are unavailable") {
		t.Fatalf("missing unavailable instruction: %s", policy.SystemInstruction)
	}
}

func TestFundsSentinelRejectsReadGrepAndMemoryMCP(t *testing.T) {
	tools := []string{
		"read",
		"read_file",
		"grep",
		"find",
		"glob",
		"bash",
		"mcp__memory__search",
		"mcp__other__lookup",
	}
	policy := CaseFundAnalysisPolicyForWorkspace(true, "请分析当前案件账户资金流水", tools)
	if !policy.Active || !policy.SourceUnavailable {
		t.Fatalf("non-funds tools must not satisfy current-case source readiness: %#v", policy)
	}
	if len(policy.ToolScope) != 1 || policy.ToolScope[0] != CaseFundUnavailableToolSentinel {
		t.Fatalf("only the unavailable sentinel may be advertised: %#v", policy.ToolScope)
	}
	if policy.BoundaryAnswer != CaseFundSourceUnavailableAnswer() {
		t.Fatalf("source-unavailable turns require the host-owned boundary: %q", policy.BoundaryAnswer)
	}
	for _, toolName := range tools {
		for _, advertised := range policy.ToolScope {
			if advertised == toolName {
				t.Fatalf("fallback tool %q must not be advertised for case facts", toolName)
			}
		}
	}
	for _, forbiddenFallback := range []string{"local files", "shell commands", "file reads", "search tools", "memory", "prior thread output"} {
		if !strings.Contains(policy.SystemInstruction, forbiddenFallback) {
			t.Fatalf("unavailable policy must forbid %q fallback: %s", forbiddenFallback, policy.SystemInstruction)
		}
	}
}

func TestCaseFundToolScopeCompositionNeverExpandsDelegatedAuthority(t *testing.T) {
	fundsCount := "mcp__analytix_funds__count"
	fundsRank := "mcp__analytix-fund-analysis__rank"
	policy := []string{fundsRank, fundsCount, fundsCount, "read", "mcp__memory__search"}

	if scope, ok := ComposeCaseFundToolScope(nil, policy, false); !ok || scope != nil {
		t.Fatalf("root turn should retain the permanent catalog: scope=%#v ok=%v", scope, ok)
	}
	if scope, ok := ComposeCaseFundToolScope([]string{"read", fundsCount}, policy, false); !ok ||
		len(scope) != 2 || scope[0] != "read" || scope[1] != fundsCount {
		t.Fatalf("root explicit scope should preserve ordinary and case tools: scope=%#v ok=%v", scope, ok)
	}
	if scope, ok := ComposeCaseFundToolScope([]string{fundsCount}, policy, true); !ok || len(scope) != 1 || scope[0] != fundsCount {
		t.Fatalf("exact delegated funds authority should survive: scope=%#v ok=%v", scope, ok)
	}
	for name, requested := range map[string][]string{"empty delegated authority": nil, "unavailable case tool": {"read", "mcp__analytix_funds__hidden"}} {
		t.Run(name, func(t *testing.T) {
			if scope, ok := ComposeCaseFundToolScope(requested, policy, true); ok || len(scope) != 0 {
				t.Fatalf("delegated scope composition expanded or rewrote authority: scope=%#v ok=%v", scope, ok)
			}
		})
	}
	if scope, ok := ComposeCaseFundToolScope([]string{"read", "grep", "mcp__memory__search"}, policy, true); ok ||
		len(scope) != 0 {
		t.Fatalf("ordinary-only delegated scope incorrectly opened the protected lane: scope=%#v ok=%v", scope, ok)
	}
}

func TestCaseFundPolicyBlocksUnboundHighRiskPromptAndIgnoresUnrelatedPrompt(t *testing.T) {
	policy := CaseFundAnalysisPolicyForWorkspace(false, "请核实案件银行账号、金额、MAC、亲属关系和投标报价", []string{"mcp__analytix_funds__count"})
	if !policy.Active || !policy.SourceUnavailable || policy.BoundaryAnswer != CaseFundSourceUnavailableAnswer() {
		t.Fatalf("unbound high-risk request must use a host-owned unavailable boundary: %#v", policy)
	}
	if CaseFundAnalysisPolicyForWorkspace(true, "你好", []string{"mcp__analytix_funds__count"}).Active {
		t.Fatal("unrelated prompt must not activate case fund policy")
	}
}

func TestFormalCaseReportRequiresRiskAdmissionWithoutBinding(t *testing.T) {
	for _, prompt := range []string{
		"请基于当前案件出具正式报告",
		"Prepare the investigation report appendix",
	} {
		policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
		if !policy.Active || !policy.SourceUnavailable {
			t.Fatalf("formal case report escaped risk admission: prompt=%q policy=%#v", prompt, policy)
		}
	}
	if PromptRequiresCaseRiskAdmission("为普通代码仓库生成测试报告") {
		t.Fatal("ordinary engineering report must not be classified as a case request")
	}
}

func TestCaseFundPolicyIgnoresLocalFilesystemInventory(t *testing.T) {
	prompt := `请对路径 /Users/sun/Desktop 下的所有桌面文件进行全面分析，包括文件分类汇总、目录结构分析、文档主题分析、资金分析成果证据转化.docx 和安全风险评估。`
	if CaseFundAnalysisPolicyForWorkspace(true, prompt, []string{"mcp__analytix_funds__count"}).Active {
		t.Fatal("desktop/file inventory prompts must not activate current-case fund-analysis policy")
	}
	if !PromptLooksLikeLocalFilesystemTask(prompt) {
		t.Fatal("desktop/file inventory prompt should be classified as a filesystem task")
	}
}

func TestCaseFundPolicyKeepsIndependentOrdinaryWorkWhenCaseSourceIsUnavailable(t *testing.T) {
	prompt := "请读取代码并修改测试，运行测试；同时查询当前案件账户在本月的流入、流出和净额。"
	policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, []string{"read", "bash"})
	if !policy.Active || !policy.SourceUnavailable || !policy.OrdinaryWorkRequested {
		t.Fatalf("mixed request lost its separate ordinary work: %#v", policy)
	}
	if policy.ProviderUsesCaseDataAuthority() || policy.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("mixed unavailable request was treated as a whole-turn case mode: %#v", policy)
	}
	if !PromptExplicitlyRequestsCaseFundAnalysis(prompt) || !PromptRequestsIndependentOrdinaryWork(prompt) {
		t.Fatal("mixed request classifiers did not recognize both independent parts")
	}
	if policy.OrdinaryPrompt != "请读取代码；修改测试；运行测试" {
		t.Fatalf("mixed request ordinary subrequest is not exact: %q", policy.OrdinaryPrompt)
	}

	available := CaseFundAnalysisPolicyForWorkspace(true, prompt, []string{"mcp__analytix_funds__analyze_account_flows"})
	if !available.Active || available.SourceUnavailable || !available.OrdinaryWorkRequested ||
		!available.ProviderUsesCaseDataAuthority() || available.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("available mixed request did not retain additive case authority: %#v", available)
	}
}

func TestCaseRiskClassifierPreservesOrdinaryWritingSkillsAndMCP(t *testing.T) {
	for _, prompt := range []string{
		"Use the documentation Skill to research and write up database transaction isolation.",
		"Use mcp__docs__lookup and summarize the API contract.",
		"Use Jira MCP and write a summary.",
		"请研究普通账户模块并撰写测试文档。",
		"请实现交易流水解析器并运行测试。",
		"Review the account flow parser code and run tests.",
		"Update the funds snapshot module and run tests.",
		"Inspect database transaction code and run tests.",
		"Check database transaction isolation tests.",
		"Analyze database transaction isolation.",
		"分析账户余额计算逻辑。",
		"编写交易金额格式化函数。",
	} {
		t.Run(prompt, func(t *testing.T) {
			if PromptRequiresCaseRiskAdmission(prompt) {
				t.Fatalf("ordinary Agent request was classified as protected case work: %q", prompt)
			}
			if policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil); policy.Active {
				t.Fatalf("ordinary Agent request activated a case capability: %#v", policy)
			}
			if !PromptRequestsIndependentOrdinaryWork(prompt) {
				t.Fatalf("ordinary Agent request was not recognized: %q", prompt)
			}
		})
	}
}

func TestCaseRiskClassifierPreservesPackagedMilestoneAPlanningTurn(t *testing.T) {
	historicalPrompt := strings.Join([]string{
		"Work only in this pre-existing isolated non-case code repository and inspect the task through tools.",
		"The user request is: Update more_itertools/recipes.py so take() explicitly rejects a negative n with ValueError('n must be at least 0') before asking the supplied iterable for an iterator. Preserve all behavior for nonnegative n and modify no other file.",
		`Call read exactly once for each contract-required path, with only its relative path and no optional offset or limit: "README.rst", "LICENSE", "more_itertools/more.py", "more_itertools/recipes.py", "tests/test_recipes.py".`,
		"Use no tools other than those exact read calls and one create_plan call.",
		"Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call.",
		"Complete those exact read calls before recording the plan.",
		"Call create_plan exactly once after those reads complete.",
		"Record a concrete implementation and verification plan at the GUI-reserved plan path.",
		"In the final response include the exact marker MILESTONE_A_PLAN_OK.",
		"Do not edit files, run the test, delegate, or run /compact in this planning turn.",
	}, " ")
	currentPrompt := strings.Join([]string{
		"Work only in this pre-existing isolated non-case code repository and inspect the task through tools.",
		"The user request is: Update Express content-type normalization so the reserved HTTP quality parameter name is handled case-insensitively. Add a focused regression test covering both lowercase and uppercase quality parameter names while preserving ordinary parameters, then run the focused real project test.",
		`Call read exactly once for each contract-required task inspection path, with only its relative path and no optional offset or limit: "lib/utils.js", "test/utils.js", "package.json".`,
		"Do not read the contract-bound long-context file in this planning turn; the acceptance harness validates it in a dedicated later continuation.",
		"Use no tools other than those exact read calls and one create_plan call.",
		"Do not call list, search, code index, web fetch, bash, Todo, subagent/delegation, skill, MCP, or read any other path.",
		"Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call.",
		"Complete those exact read calls before recording the plan.",
		"Call create_plan exactly once after those reads complete.",
		"Record a concrete implementation and verification plan at the GUI-reserved plan path.",
		"After create_plan returns successfully, end the turn immediately without another tool call.",
		"If every required read succeeds, do not send a final response or the completion marker until create_plan has returned successfully.",
		"In the final response include the exact marker MILESTONE_A_PLAN_OK.",
		"Do not edit files, run the test, delegate, or run /compact in this planning turn.",
	}, " ")

	for name, prompt := range map[string]string{
		"historical": historicalPrompt,
		"current":    currentPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			if PromptRequiresCaseRiskAdmission(prompt) {
				t.Fatal("packaged non-case planning turn was classified as protected case work")
			}
			if policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil); policy.Active {
				t.Fatalf("packaged non-case planning turn activated a case capability: %#v", policy)
			}
			if !PromptRequestsIndependentOrdinaryWork(prompt) {
				t.Fatal("packaged non-case planning turn was not recognized as ordinary Agent work")
			}
		})
	}
	if !PromptRequiresCaseRiskAdmission("Check the case: Alice is a relative of Bob.") {
		t.Fatal("disambiguating a filesystem relative path removed the case-relative risk cue")
	}
}

func TestCaseRiskClassifierRequiresASCIIRiskCueBoundaries(t *testing.T) {
	for _, prompt := range []string{
		"Work in a non-case repository. Read recipes.py. Include a concise final marker.",
		"Review macos_ipc.go in a non-case repository. Include a concise final marker.",
		"Prepare a non-case report about ordinary software test failures.",
		"Prepare a non case report about ordinary software test failures.",
		"Handle the reserved parameter case-insensitive and report test failures.",
		"Handle the reserved parameter case-insensitively and report test failures.",
	} {
		if PromptRequiresCaseRiskAdmission(prompt) {
			t.Fatalf("ordinary identifier activated case risk: %q", prompt)
		}
	}
	for _, prompt := range []string{
		"Check the case IP address before writing the brief.",
		"Check the case MAC address before writing the brief.",
		"Prepare the case report appendix.",
		"Prepare a non-case report, then check the case IP address.",
		"Handle the reserved parameter case-insensitively, then prepare the case report.",
	} {
		if !PromptRequiresCaseRiskAdmission(prompt) {
			t.Fatalf("bounded case risk cue was missed: %q", prompt)
		}
	}
}

func TestCaseRiskClassifierKeepsConcreteCaseInputsFailClosed(t *testing.T) {
	for _, prompt := range []string{
		"请查询当前案件账户的交易笔数。",
		"请查询银行账号 6222020000000000000 的流入和流出。",
		"交易明细表数据量多少？",
		"导出当前案件资金清洗明细。",
	} {
		t.Run(prompt, func(t *testing.T) {
			if !PromptExplicitlyRequestsCaseFundAnalysis(prompt) {
				t.Fatalf("concrete case input escaped explicit funds classification: %q", prompt)
			}
			if !PromptRequiresCaseRiskAdmission(prompt) {
				t.Fatalf("concrete case input escaped risk admission: %q", prompt)
			}
			policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
			if !policy.Active || !policy.SourceUnavailable || policy.OrdinaryWorkRequested ||
				policy.OrdinaryPrompt != "" || !policy.MustReturnBoundaryBeforeProvider() {
				t.Fatalf("concrete case input did not fail closed: %#v", policy)
			}
		})
	}
}

func TestCaseRiskClassifierKeepsOrdinarySoftwareCountAndExportGeneral(t *testing.T) {
	for _, prompt := range []string{
		"请运行交易明细表数据量统计函数的测试。",
		"导出当前案件资金分析代码的测试报告。",
		"Export the current-case transaction parser test report.",
	} {
		t.Run(prompt, func(t *testing.T) {
			if PromptExplicitlyRequestsCaseFundAnalysis(prompt) || PromptRequiresCaseRiskAdmission(prompt) {
				t.Fatalf("ordinary software/export request was classified as protected case work: %q", prompt)
			}
			if policy := CaseFundAnalysisPolicyForWorkspace(true, prompt, nil); policy.Active {
				t.Fatalf("ordinary software/export request activated case capability: %#v", policy)
			}
		})
	}
}

func TestIndependentOrdinaryPromptRetainsActionAcrossMixedConjunctions(t *testing.T) {
	for prompt, want := range map[string]string{
		"modify ordinary code and inspect current-case funds":         "modify ordinary code",
		"analyze current-case funds and then update the code":         "update the code",
		"请查询当前案件账户；并修改代码并运行测试":                                        "修改代码；运行测试",
		"update code; inspect the attached file":                      "update code",
		"read the attached file":                                      "",
		"fix the attachment parser code; query current-case funds":    "fix the attachment parser code",
		"query current-case funds; use the result to update the code": "",
		"查询当前案件账户；根据上述结果修改代码":                                         "",
		"query current-case funds; modify code with the result":       "",
		"查询当前案件账户；用结果修改代码":                                            "",
		"查询当前案件账户；据此更新测试":                                             "",
		"update files a, b, and c; query current-case funds":          "update files a, b, and c",
	} {
		t.Run(prompt, func(t *testing.T) {
			if got := IndependentOrdinaryPromptV1(prompt); got != want {
				t.Fatalf("ordinary subrequest mismatch: got=%q want=%q", got, want)
			}
		})
	}
}

func TestCaseFundBoundaryAuthorityPolicyPreservesOnlyIndependentOrdinaryWork(t *testing.T) {
	base := CaseFundAnalysisPolicy{}
	if got := CaseFundBoundaryAuthorityPolicyV1(base, false); !reflect.DeepEqual(got, base) {
		t.Fatalf("evidence-capable context changed policy: %#v", got)
	}
	pure := CaseFundBoundaryAuthorityPolicyV1(base, true)
	if !pure.SourceUnavailable || pure.OrdinaryWorkRequested || !pure.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("pure protected attachment did not close at the host boundary: %#v", pure)
	}
	unprovenMixed := CaseFundBoundaryAuthorityPolicyV1(CaseFundSourceUnavailablePolicyV1(true), true)
	if !unprovenMixed.SourceUnavailable || unprovenMixed.OrdinaryWorkRequested || !unprovenMixed.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("policy without a proven ordinary prompt acquired an ordinary lane: %#v", unprovenMixed)
	}
	active := CaseFundSourceUnavailablePolicyV1(true)
	active.OrdinaryPrompt = "update ordinary code"
	mixed := CaseFundBoundaryAuthorityPolicyV1(active, true)
	if !mixed.SourceUnavailable || !mixed.OrdinaryWorkRequested || mixed.OrdinaryPrompt != "update ordinary code" || mixed.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("frozen case policy did not retain its proven ordinary work: %#v", mixed)
	}
}

func TestCaseFundBoundaryAuthorityPolicyPreservesCurrentOrdinaryTurnWithoutLoweringThreadRisk(t *testing.T) {
	ordinary := CaseFundBoundaryAuthorityPolicyForCurrentTurnV1(
		CaseFundAnalysisPolicy{}, true, false, false, false, "read the ordinary source file", "",
	)
	if ordinary.Active || ordinary.SourceUnavailable || ordinary.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("current ordinary turn was replaced by the monotonic thread boundary: %#v", ordinary)
	}
	for name, policy := range map[string]CaseFundAnalysisPolicy{
		"current case risk": CaseFundBoundaryAuthorityPolicyForCurrentTurnV1(
			CaseFundAnalysisPolicy{}, true, true, false, false, "read the ordinary source file", "",
		),
		"attachment": CaseFundBoundaryAuthorityPolicyForCurrentTurnV1(
			CaseFundAnalysisPolicy{}, true, false, true, false, "read the ordinary source file", "",
		),
		"case binding": CaseFundBoundaryAuthorityPolicyForCurrentTurnV1(
			CaseFundAnalysisPolicy{}, true, false, false, true, "read the ordinary source file", "",
		),
		"empty request": CaseFundBoundaryAuthorityPolicyForCurrentTurnV1(
			CaseFundAnalysisPolicy{}, true, false, false, false, "", "",
		),
	} {
		t.Run(name, func(t *testing.T) {
			if !policy.SourceUnavailable || !policy.MustReturnBoundaryBeforeProvider() {
				t.Fatalf("unproven ordinary turn escaped the thread boundary: %#v", policy)
			}
		})
	}
}

func TestHostEffectPromptUsesOnlyProvenOrdinaryLaneWhenCaseSourceIsUnavailable(t *testing.T) {
	if got := HostEffectPromptV1(nil, "ordinary prompt"); got != "ordinary prompt" {
		t.Fatalf("ordinary host effect prompt changed: %q", got)
	}
	policy := CaseFundSourceUnavailablePolicyV1(true)
	policy.OrdinaryPrompt = "update ordinary code"
	if got := HostEffectPromptV1(&policy, "/goal --research blocked case clause"); got != "update ordinary code" {
		t.Fatalf("host effect prompt retained blocked case text: %q", got)
	}
	policy.OrdinaryPrompt = ""
	if got := HostEffectPromptV1(&policy, "/goal --research blocked case clause"); got != "" {
		t.Fatalf("pure case boundary retained a host effect prompt: %q", got)
	}
}

func TestPreFreezeHostEffectPromptFailsClosedBeforeCaseAuthorityFreeze(t *testing.T) {
	if got := PreFreezeHostEffectPromptV1("/goal --research ordinary topic", "", nil, false); got != "/goal --research ordinary topic" {
		t.Fatalf("ordinary pre-freeze prompt changed: %q", got)
	}
	if got := PreFreezeHostEffectPromptV1("/goal --research blocked opaque case task", "", nil, true); got != "" {
		t.Fatalf("opaque case risk reached a pre-freeze host effect: %q", got)
	}
	if got := PreFreezeHostEffectPromptV1("/goal --research update ordinary code; inspect current-case funds", "", nil, true); got != "/goal --research update ordinary code" {
		t.Fatalf("mixed case risk did not retain only its ordinary host effect: %q", got)
	}
}

func TestLocalFilesystemInventoryCannotExemptConcreteProtectedInput(t *testing.T) {
	prompt := "请扫描目录并总结银行卡号 6222020000000000000"
	if !PromptLooksLikeLocalFilesystemTask(prompt) || !PromptRequiresCaseRiskAdmission(prompt) {
		t.Fatalf("adversarial filesystem prompt did not expose both classifiers: %q", prompt)
	}
	policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
	if !policy.Active || !policy.SourceUnavailable || !policy.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("filesystem wording exempted complete protected input: %#v", policy)
	}
}

func TestCaseAdmissionFileReferencesUseHighSpecificityRiskOnly(t *testing.T) {
	ordinaryReferences := []struct {
		value any
		text  string
	}{
		{value: map[string]any{"path": "/workspace/internal/account_parser.go", "kind": "file"}, text: "/workspace/internal/account_parser.go"},
		{value: map[string]any{"name": "duckdb_adapter_test.go", "kind": "file"}, text: "duckdb_adapter_test.go"},
		{value: "/workspace/src/bank-ledger-parser.ts", text: "/workspace/src/bank-ledger-parser.ts"},
	}
	rawOrdinaryReferences := make([]any, 0, len(ordinaryReferences))
	for _, reference := range ordinaryReferences {
		rawOrdinaryReferences = append(rawOrdinaryReferences, reference.value)
	}
	ordinaryText := CaseAdmissionTextV1("Review the supplied source file.", "", rawOrdinaryReferences)
	for _, reference := range ordinaryReferences {
		if strings.Contains(ordinaryText, reference.text) {
			t.Fatalf("ordinary source filename %q entered case admission text: %q", reference.text, ordinaryText)
		}
	}
	ordinaryPolicy := CaseFundAnalysisPolicyForTurnV1(
		false, "Review the supplied source file.", "", rawOrdinaryReferences, nil,
		domainsecurity.TurnSecurityContext{}, false,
	)
	if ordinaryPolicy.Active {
		t.Fatalf("ordinary source reference activated case capability: %#v", ordinaryPolicy)
	}

	for _, reference := range []any{
		map[string]any{"path": "/workspace/bank-ledger.csv", "kind": "file"},
		map[string]any{"name": "raw-evidence.json", "kind": "file"},
		map[string]any{"displayName": "case-data.duckdb", "kind": "file"},
		"/workspace/受保护证据.xlsx",
	} {
		t.Run(strings.TrimSpace(CaseAdmissionTextV1("", "", []any{reference})), func(t *testing.T) {
			admissionText := CaseAdmissionTextV1("Summarize the supplied file.", "", []any{reference})
			if !PromptRequiresCaseRiskAdmission(admissionText) {
				t.Fatalf("high-specificity file reference escaped risk admission: %q", admissionText)
			}
			policy := CaseFundAnalysisPolicyForTurnV1(
				false, "Summarize the supplied file.", "", []any{reference}, nil,
				domainsecurity.TurnSecurityContext{}, false,
			)
			if !policy.Active || !policy.SourceUnavailable || policy.OrdinaryWorkRequested ||
				policy.OrdinaryPrompt != "" || !policy.MustReturnBoundaryBeforeProvider() {
				t.Fatalf("protected file reference did not freeze a pure boundary: %#v", policy)
			}
		})
	}

	mixed := CaseFundAnalysisPolicyForTurnV1(
		false, "Update the code; summarize the supplied file.", "",
		[]any{map[string]any{"path": "/workspace/bank-ledger.csv", "kind": "file"}}, nil,
		domainsecurity.TurnSecurityContext{}, false,
	)
	if !mixed.Active || !mixed.SourceUnavailable || !mixed.OrdinaryWorkRequested ||
		mixed.OrdinaryPrompt != "Update the code" || mixed.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("protected reference removed an independent ordinary clause: %#v", mixed)
	}
}

func TestBoundCaseFundFollowupRemainsAdditiveAcrossTurns(t *testing.T) {
	tools := []string{"mcp__analytix_funds__analyze_account_flows"}
	for _, prompt := range []string{
		"分析该账户上月有什么异常",
		"Compare this account with last month.",
		"这个账户本月多少笔交易？",
		"那净额呢？",
		"再看流出。",
	} {
		t.Run(prompt, func(t *testing.T) {
			if !PromptLooksLikeBoundCaseFundFollowupV1(prompt) {
				t.Fatalf("bound-case follow-up classifier missed %q", prompt)
			}
			if ordinary := IndependentOrdinaryPromptV1(prompt); ordinary != "" {
				t.Fatalf("pure bound-case follow-up was extracted as ordinary work: %q", ordinary)
			}
			if unbound := CaseFundAnalysisPolicyForWorkspace(false, prompt, tools); unbound.Active {
				t.Fatalf("deictic follow-up created case capability without host binding: %#v", unbound)
			}
			bound := CaseFundAnalysisPolicyForWorkspace(true, prompt, tools)
			if !bound.Active || bound.SourceUnavailable || bound.OrdinaryWorkRequested || bound.OrdinaryPrompt != "" ||
				!bound.ProviderUsesCaseDataAuthority() {
				t.Fatalf("bound-case follow-up lost additive funds capability: %#v", bound)
			}
			unavailable := CaseFundAnalysisPolicyForWorkspace(true, prompt, nil)
			if !unavailable.SourceUnavailable || !unavailable.MustReturnBoundaryBeforeProvider() ||
				unavailable.OrdinaryWorkRequested || unavailable.OrdinaryPrompt != "" {
				t.Fatalf("source-unavailable bound follow-up was not a pure boundary: %#v", unavailable)
			}
		})
	}
	if PromptLooksLikeBoundCaseFundFollowupV1("Compare this account parser in the test module.") {
		t.Fatal("software work was classified as a bound-case funds follow-up")
	}
	if ordinary := IndependentOrdinaryPromptV1("Compare this account parser in the test module."); ordinary == "" {
		t.Fatal("software account parser work was removed from the ordinary lane")
	}
	for _, prompt := range []string{"Analyze database transactions.", "Summarize transaction documentation."} {
		if PromptLooksLikeBoundCaseFundFollowupV1(prompt) || IndependentOrdinaryPromptV1(prompt) == "" {
			t.Fatalf("generic transaction work was treated as a deictic case follow-up: %q", prompt)
		}
	}
}

func TestCaseFundTurnPolicyUsesVisibleUserIntentInsteadOfManagedCodePrefix(t *testing.T) {
	protectedRequest := "Attempt a protected funds fact request for the current case. " +
		"Request inflow, outflow, net amount, transaction count, and evidence rows. " +
		"Do not use ordinary read, file, shell, Todo, Skill, subagent, or non-funds MCP tools."
	managedPrompt := "[Analytix Code managed instructions]\n\n" +
		"Read code, update files, run tests, use Todo and Skills when the user asks.\n\n" +
		"---\n[Current user request]\n" + protectedRequest

	policy := CaseFundAnalysisPolicyForTurnV1(
		false, managedPrompt, protectedRequest, nil, nil,
		domainsecurity.TurnSecurityContext{}, false,
	)
	if !policy.Active || !policy.SourceUnavailable || policy.OrdinaryWorkRequested ||
		policy.OrdinaryPrompt != "" || !policy.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("managed code prefix opened an ordinary provider lane: %#v", policy)
	}

	rawPolicy := CaseFundAnalysisPolicyForTurnV1(
		false, protectedRequest, "", nil, nil,
		domainsecurity.TurnSecurityContext{}, false,
	)
	if !rawPolicy.Active || !rawPolicy.SourceUnavailable || rawPolicy.OrdinaryWorkRequested ||
		rawPolicy.OrdinaryPrompt != "" || !rawPolicy.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("ordinary-tool prohibition opened an ordinary provider lane: %#v", rawPolicy)
	}

	mixedRequest := "Update the ordinary code; also query the current case account inflow and outflow."
	mixed := CaseFundAnalysisPolicyForTurnV1(
		false, managedPrompt+"\n"+mixedRequest, mixedRequest, nil, nil,
		domainsecurity.TurnSecurityContext{}, false,
	)
	if !mixed.Active || !mixed.SourceUnavailable || !mixed.OrdinaryWorkRequested ||
		mixed.OrdinaryPrompt != "Update the ordinary code" || mixed.MustReturnBoundaryBeforeProvider() {
		t.Fatalf("visible mixed request lost its independent ordinary lane: %#v", mixed)
	}
}

func TestIndependentOrdinaryPromptRejectsProhibitionsWithoutDroppingPositiveClauses(t *testing.T) {
	for prompt, want := range map[string]string{
		"Do not use ordinary read, file, shell, Todo, Skill, subagent, or non-funds MCP tools for this request.": "",
		"Please don't run shell or tests; update the ordinary code; query current-case funds.":                   "Please don't run shell or tests；update the ordinary code",
		"请勿使用 read、bash、Todo 或子代理；修改普通代码；查询当前案件账户流入。":                                                            "请勿使用 read、bash、Todo 或子代理；修改普通代码",
		"Update the ordinary code; do not run shell; query current-case funds.":                                  "Update the ordinary code；do not run shell",
		"Review the ordinary code and do not run tests; query current-case funds.":                               "Review the ordinary code and do not run tests",
		"Without shell, update the ordinary code; query current-case funds.":                                     "Without shell；update the ordinary code",
		"Do not run tests but update the ordinary code; query current-case funds.":                               "Do not run tests but update the ordinary code",
	} {
		t.Run(prompt, func(t *testing.T) {
			if got := IndependentOrdinaryPromptV1(prompt); got != want {
				t.Fatalf("ordinary prohibition partition mismatch: got=%q want=%q", got, want)
			}
		})
	}
	for _, prompt := range []string{
		"Do not call read or shell.",
		"Never invoke a non-funds MCP tool.",
		"不要使用 read、bash、Todo、Skill、subagent 或普通 MCP。",
	} {
		if PromptRequestsIndependentOrdinaryWork(prompt) {
			t.Fatalf("ordinary capability prohibition was classified as affirmative work: %q", prompt)
		}
	}
}

func TestCaseFundPolicyRecognizesCanonicalOrdinaryToolIntentsInMixedRequests(t *testing.T) {
	for _, prompt := range []string{
		"请用 read 查看代码，并查询当前案件账户的流入和流出。",
		"请用 bash 执行 go test，并查询当前案件账户的流入和流出。",
		"请查看待办列表，并查询当前案件账户的流入和流出。",
		"请用 todo_list 更新 Todo，并查询当前案件账户的流入和流出。",
		"请用 parallel_tasks 分解子任务，并查询当前案件账户的流入和流出。",
		"Please delegate a subtask, then query the current case account inflow and outflow.",
	} {
		t.Run(prompt, func(t *testing.T) {
			policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
			if !policy.Active || !policy.SourceUnavailable || !policy.OrdinaryWorkRequested ||
				policy.MustReturnBoundaryBeforeProvider() {
				t.Fatalf("mixed ordinary intent lost the permanent Agent base: %#v", policy)
			}
		})
	}
}

func TestExplicitOrdinaryToolScopePreservesMixedFallback(t *testing.T) {
	policy := CaseFundSourceUnavailablePolicyV1(true)
	policy.OrdinaryPrompt = "run the ordinary tests"
	resolved, scope, boundary := ResolveCaseFundToolScopeV1(
		policy,
		[]string{"bash", "mcp__analytix_funds__analyze_account_flows"},
		false,
	)
	if boundary != "" || !resolved.OrdinaryWorkRequested || resolved.OrdinaryPrompt != policy.OrdinaryPrompt || len(scope) != 2 {
		t.Fatalf("explicit ordinary scope lost mixed fallback: policy=%#v scope=%#v boundary=%q", resolved, scope, boundary)
	}

	pure, _, pureBoundary := ResolveCaseFundToolScopeV1(
		CaseFundSourceUnavailablePolicyV1(false),
		[]string{"mcp__analytix_funds__analyze_account_flows"},
		false,
	)
	if pure.OrdinaryWorkRequested || pureBoundary != CaseFundSourceUnavailableAnswer() {
		t.Fatalf("pure funds scope was widened to ordinary work: policy=%#v boundary=%q", pure, pureBoundary)
	}
}

func TestCaseFundUnsafeFinalUsesHostBoundary(t *testing.T) {
	if got := CaseFundFinalRecoveryReason("生成报告", "我会用 DuckDB SQL 继续查询"); got != "internal_support_layer_leakage" {
		t.Fatalf("expected internal leak recovery, got %q", got)
	}
	if got := CaseFundFinalRecoveryReason("生成报告", "后端服务不可用，无法生成报告"); got != "report_publication_receipt_required" {
		t.Fatalf("expected publication receipt blocker, got %q", got)
	}
	if got := CaseFundFinalRecoveryReason("生成报告", "已生成 /Users/sun/report.md，inspection passed，sha256=abc"); got != "report_publication_receipt_required" {
		t.Fatalf("path and inspection must not bypass publication receipt blocker, got %q", got)
	}
	boundary := CaseFundUnverifiedFinalAnswer()
	if strings.Contains(boundary, "继续完成") || strings.Contains(boundary, "run_full_case_analysis") {
		t.Fatalf("host boundary must not direct a model to continue case analysis: %s", boundary)
	}
	reportBoundary := CaseFundFinalBoundaryAnswer("请生成资金研判报告")
	if !strings.Contains(reportBoundary, "正式报告未发布") || strings.Contains(reportBoundary, "/Users/") {
		t.Fatalf("report boundary must be fixed and path-free: %s", reportBoundary)
	}
	instruction := caseFundSystemInstruction()
	if strings.Contains(instruction, "use write_report=false") || !strings.Contains(instruction, "do not call run_full_case_analysis") {
		t.Fatalf("case report instruction still routes the provider into a quarantined artifact tool: %s", instruction)
	}
	if !strings.Contains(instruction, "only first-stage provider-visible funds operation is analyze_account_flows") ||
		!strings.Contains(instruction, "count_case_rows is a host-only protocol canary") ||
		strings.Contains(instruction, "Workbench ladder") {
		t.Fatalf("case funds instruction did not isolate the count canary from the valuable analysis contract: %s", instruction)
	}
}

func TestCaseFundDraftAndFinalTextHelpers(t *testing.T) {
	if got := RemoveTrailingAssistantDraft("abc draft", " draft"); got != "abc" {
		t.Fatalf("unexpected trimmed draft: %q", got)
	}
	if got := RemoveTrailingAssistantDraft("abc draft", "missing"); got != "abc draft" {
		t.Fatalf("non-suffix draft should be unchanged: %q", got)
	}
	sanitized := SanitizeCaseFundFinalText("DuckDB SQL query_id analysis_txn_detail_idx follow-up source lane")
	for _, forbidden := range []string{"DuckDB", "SQL", "query_id", "analysis_txn_detail_idx", "follow-up", "source lane"} {
		if strings.Contains(sanitized, forbidden) {
			t.Fatalf("sanitized text still contains %q: %s", forbidden, sanitized)
		}
	}
}
