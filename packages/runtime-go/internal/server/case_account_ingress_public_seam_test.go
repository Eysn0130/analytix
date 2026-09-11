package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	caseentityadapter "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	evidenceregistryadapter "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	gateprojectionapp "analytix.local/runtime-go/internal/app/gateprojection"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/jobs"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

var (
	caseIngressAliasPatternV1                 = regexp.MustCompile(`(?:acct|card):[1-9][0-9]{0,9}`)
	caseIngressScopeBindingDigestPatternV1    = regexp.MustCompile(`"scopeBindingDigest":"([a-f0-9]{64})"`)
	caseIngressSnapshotBindingDigestPatternV1 = regexp.MustCompile(`"snapshotBindingDigest":"([a-f0-9]{64})"`)
	caseIngressReferenceDigestPatternV1       = regexp.MustCompile(`"referenceDigest":"([a-f0-9]{64})"`)
)

func TestInitialCaseAccountIngressHTTPRetryPrivacyAndDatasetStalenessIsAdditive(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "case-ingress-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "case-ingress-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)

	dataset := newServerSignedCurrentDatasetV1(t, workspace)
	handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: testIdentityAuthority(), Observer: dataset, RiskAuthority: newServerTestRiskAuthority(),
		SnapshotAuthorityV2: dataset,
	}
	handler.threads = nil
	configureCaseIngressPublicSeamAuthoritiesV1(t, handler, durableRoot, dataset)
	privateStore := configureCaseIngressEntityServiceV1(t, handler, dataset)
	configureCaseIngressNativeAuthorityV1(t, handler)
	handler.mcp = &providerStepIntegrationMCP{
		admissionFailureMCP: &admissionFailureMCP{},
		advertisements:      providerStepAdvertisements(t),
	}

	recordingProvider := &providerStepRecordingProvider{}
	recordingProvider.onCall = func(call int, _ domainmodel.Request) error {
		if call == 1 {
			return errors.New("provider returned 503")
		}
		return nil
	}
	handler.provider = recordingProvider
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "case ingress public seam", "workspace": workspace,
			"providerId": "case-ingress-provider", "model": "case-ingress-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	if threadID == "" {
		t.Fatalf("HTTP thread creation returned no thread id: %#v", thread)
	}

	const rawAccount = "6222021234567890123"
	first := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "请查询银行账号 " + rawAccount + " 在指定期间的流入、流出、净额和交易笔数。",
			"riskIntent": "case",
		})),
		http.StatusAccepted,
	)
	firstTurnID := stringField(first, "turnId")
	if firstTurnID == "" {
		t.Fatalf("HTTP turn start returned no turn id: %#v", first)
	}

	requests := recordingProvider.Requests()
	if len(requests) != 2 {
		t.Fatalf("initial case turn physical provider calls=%d want=2", len(requests))
	}
	providerAliases := make([]string, len(requests))
	for index, request := range requests {
		prompt := lastProviderStepUserPrompt(request.Messages)
		if strings.Contains(prompt, rawAccount) || strings.Contains(prompt, "[ACCOUNT]") ||
			domaincaseentity.ContainsReferenceCandidateV1(prompt) {
			t.Fatalf("provider attempt %d received raw or low-information account projection: %q", index+1, prompt)
		}
		matches := caseIngressAliasPatternV1.FindAllString(prompt, -1)
		if len(matches) < 2 {
			t.Fatalf("provider attempt %d did not receive both the stable identity and its semantic binding: %q", index+1, prompt)
		}
		providerAliases[index] = matches[0]
		for _, current := range matches[1:] {
			if current != providerAliases[index] {
				t.Fatalf("provider attempt %d semantic binding changed identity: %#v", index+1, matches)
			}
		}
		if !strings.Contains(prompt, `"bankInstitution":"Bank of Analytix"`) ||
			!strings.Contains(prompt, `"accountType":"settlement account"`) ||
			!strings.Contains(prompt, `<analytix_host_verified_case_entity_semantics>`) {
			t.Fatalf("provider attempt %d lost trusted account semantics: %q", index+1, prompt)
		}
		if request.PrivateProviderTelemetry == nil || request.PrivateProviderTelemetry.OrdinaryEffect ||
			request.PrivateProviderTelemetry.OuterAttempt != uint32(index+1) {
			t.Fatalf("provider attempt %d telemetry=%#v", index+1, request.PrivateProviderTelemetry)
		}
		for _, toolName := range []string{"read", "bash", providerStepDocsTool, providerStepFundsTool} {
			if !providerStepRequestHasTool(request, toolName) {
				t.Fatalf("provider attempt %d lost additive tool %q: %#v", index+1, toolName, request.Tools)
			}
		}
	}
	if providerAliases[0] != providerAliases[1] {
		t.Fatalf("physical provider retry changed the stable entity alias: %q != %q", providerAliases[0], providerAliases[1])
	}
	if privateStore.putIngress.Load() != 1 || privateStore.resolveIngress.Load() != 0 {
		t.Fatalf(
			"physical retry recompiled or reread private ingress: put=%d resolve=%d",
			privateStore.putIngress.Load(), privateStore.resolveIngress.Load(),
		)
	}
	if calls := privateStore.resolverCalls.Load(); calls != 1 {
		t.Fatalf("physical retry reran private account resolution: calls=%d want=1", calls)
	}
	privateStore.mu.Lock()
	resolvedCandidates := append([]string(nil), privateStore.resolvedCandidates...)
	privateStore.mu.Unlock()
	if len(resolvedCandidates) != 1 || resolvedCandidates[0] != rawAccount {
		t.Fatalf("private account resolver did not receive the one exact candidate: count=%d", len(resolvedCandidates))
	}

	firstPublicThread := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK,
	)
	firstSSE := requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0")
	assertCaseIngressPublicSurfacesV1(t, handler, durableRoot, threadID, rawAccount, firstPublicThread, firstSSE)

	// Revoke only current DSV2 selection. The case binding, risk authority,
	// one Agent, ordinary catalog, and ordinary workspace authority stay live.
	dataset.SetDatasetRevoked(true)
	second := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "读取普通源码并运行测试，同时查询当前案件账户在指定期间的流入、流出、净额和交易笔数。",
			"riskIntent": "case",
		})),
		http.StatusAccepted,
	)
	secondTurnID := stringField(second, "turnId")
	if secondTurnID == "" {
		t.Fatalf("mixed HTTP turn returned no turn id: %#v", second)
	}

	requests = recordingProvider.Requests()
	if len(requests) != 3 {
		t.Fatalf("DSV2-stale mixed turn provider calls=%d want one additional ordinary call", len(requests))
	}
	ordinary := requests[2]
	if ordinary.PrivateProviderTelemetry == nil || !ordinary.PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("DSV2-stale mixed request did not continue as an ordinary provider effect: %#v", ordinary.PrivateProviderTelemetry)
	}
	if got := lastProviderStepUserPrompt(ordinary.Messages); got != "读取普通源码；运行测试" {
		t.Fatalf("mixed request ordinary projection=%q", got)
	}
	if providerStepRequestHasTool(ordinary, providerStepFundsTool) {
		t.Fatalf("stale DSV2 retained the funds tool: %#v", ordinary.Tools)
	}
	for _, toolName := range []string{"read", "bash", providerStepDocsTool} {
		if !providerStepRequestHasTool(ordinary, toolName) {
			t.Fatalf("stale DSV2 removed ordinary tool %q: %#v", toolName, ordinary.Tools)
		}
	}
	ordinaryMessages, err := json.Marshal(ordinary.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ordinaryMessages), rawAccount) ||
		domaincaseentity.ContainsReferenceCandidateV1(string(ordinaryMessages)) ||
		strings.Contains(string(ordinaryMessages), domaincaseentity.ReferencePrefixV1) {
		t.Fatalf("ordinary lane reused protected provider history after DSV2 revocation: %s", ordinaryMessages)
	}

	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	secondTurn, found := appmodel.TurnByID(rawThread, secondTurnID)
	if !found || stringField(secondTurn, "status") != "completed" {
		t.Fatalf("DSV2-stale mixed turn did not complete ordinary work: %#v", secondTurn)
	}
	wantFinal := "provider-step-candidate-3\n\n" + apploop.CaseFundSourceUnavailableAnswer()
	if got := acceptedFinalAssistantText(secondTurn); got != wantFinal {
		t.Fatalf("DSV2-stale mixed final=%q want=%q", got, wantFinal)
	}

	secondPublicThread := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK,
	)
	secondSSE := requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0")
	secondPublicBody, err := json.Marshal(secondPublicThread)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secondPublicBody), "provider-step-candidate-3") ||
		!strings.Contains(string(secondPublicBody), apploop.CaseFundSourceUnavailableAnswer()) {
		t.Fatalf("public history lost ordinary success or the funds boundary: %s", secondPublicBody)
	}
	assertCaseIngressPublicSurfacesV1(t, handler, durableRoot, threadID, rawAccount, secondPublicThread, secondSSE)
}

func TestCaseLongitudinalContinuityUsesProductionHTTPCompositionAcrossRestartForkResumeAndCompaction(t *testing.T) {
	const (
		rawAccount  = "6222021234567890123"
		rawCard     = "6217009876543210"
		providerID  = "case-continuity-provider"
		largeModel  = "case-continuity-large"
		smallModel  = "case-continuity-small"
		hardModel   = "case-continuity-hard"
		oldBodyMark = "AUTO_COMPACTION_OLD_PROVIDER_BODY_MUST_NOT_REPLAY"
	)
	newFixture := func(t *testing.T) (*runtimeServerHandler, *serverSignedCurrentDatasetV1, *serverCaseIngressStoreV1, *providerStepRecordingProvider, *httptest.Server, string, string) {
		t.Helper()
		workspace := writeThreadMutationCaseBinding(t)
		durableRoot := t.TempDir()
		modelProviders, err := json.Marshal(map[string]any{
			"defaultProviderId": providerID,
			"providers": []map[string]any{{
				"id": providerID, "apiKey": "test-key", "baseUrl": "https://provider.invalid",
				"endpointFormat": "chat_completions", "models": []string{largeModel, smallModel, hardModel},
				"modelProfiles": map[string]any{
					largeModel: map[string]any{"contextWindowTokens": 200_000},
					smallModel: map[string]any{"contextWindowTokens": 24_000},
					hardModel:  map[string]any{"contextWindowTokens": 4_000},
				},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		handler := NewRuntimeServerHandler(RuntimeServerConfig{
			RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
			ProviderID: providerID, BaseURL: "https://provider.invalid", APIKey: "test-key",
			Model: largeModel, EndpointFormat: "chat_completions", ModelProvidersJSON: string(modelProviders),
		}).(*runtimeServerHandler)
		dataset := newServerSignedCurrentDatasetV1(t, workspace)
		handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{
			Identity: testIdentityAuthority(), Observer: dataset, RiskAuthority: newServerTestRiskAuthority(),
			SnapshotAuthorityV2: dataset,
		}
		handler.threads = nil
		configureCaseIngressPublicSeamAuthoritiesV1(t, handler, durableRoot, dataset)
		privateRoot := t.TempDir()
		store := configureCaseIngressEntityServiceAtRootV1(t, handler, dataset, privateRoot, false)
		configureCaseIngressNativeAuthorityV1(t, handler)
		handler.mcp = &providerStepIntegrationMCP{
			admissionFailureMCP: &admissionFailureMCP{}, advertisements: providerStepAdvertisements(t),
		}
		provider := &providerStepRecordingProvider{}
		handler.provider = provider
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return handler, dataset, store, provider, server, privateRoot, durableRoot
	}
	createThread := func(t *testing.T, serverURL, workspace string) string {
		t.Helper()
		thread := requestThreadSummaryJSON(
			t, serverURL, http.MethodPost, "/v1/threads",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{
				"title": "case continuity", "workspace": workspace,
				"providerId": providerID, "model": largeModel,
			})),
			http.StatusCreated,
		)
		threadID := stringField(thread, "id")
		if threadID == "" {
			t.Fatalf("HTTP thread creation returned no id")
		}
		return threadID
	}
	startTurnWithRisk := func(t *testing.T, handler *runtimeServerHandler, serverURL, threadID, prompt, riskIntent string, models ...string) {
		t.Helper()
		payload := map[string]any{"prompt": prompt}
		if riskIntent != "" {
			payload["riskIntent"] = riskIntent
		}
		if len(models) > 0 {
			payload["model"] = models[0]
		}
		response := requestThreadSummaryJSON(
			t, serverURL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
			bytes.NewReader(caseIngressJSONV1(t, payload)),
			http.StatusAccepted,
		)
		turnID := stringField(response, "turnId")
		if turnID == "" {
			t.Fatalf("HTTP turn start returned no turn id")
		}
		waitForDurableStoreCondition(t, func() bool {
			thread, err := handler.store.GetThread(threadID)
			if err != nil {
				return false
			}
			turn, found := appmodel.TurnByID(thread, turnID)
			if !found {
				return false
			}
			status := stringField(turn, "status")
			if status == "failed" || status == "cancelled" {
				t.Fatalf("HTTP turn ended before completion: status=%s", status)
			}
			return status == "completed"
		})
	}
	startTurn := func(t *testing.T, handler *runtimeServerHandler, serverURL, threadID, prompt string, models ...string) {
		t.Helper()
		startTurnWithRisk(t, handler, serverURL, threadID, prompt, "case", models...)
	}
	assertLatestProviderAliases := func(t *testing.T, provider *providerStepRecordingProvider, before int, aliases ...string) string {
		t.Helper()
		requests := provider.Requests()
		if len(requests) != before+1 {
			t.Fatalf("provider calls=%d want=%d", len(requests), before+1)
		}
		request := requests[len(requests)-1]
		body, err := json.Marshal(struct {
			SystemPrompt string                   `json:"systemPrompt"`
			Messages     []domainmodel.Message    `json:"messages"`
			Tools        []domainmodel.ToolSchema `json:"tools"`
		}{SystemPrompt: request.SystemPrompt, Messages: request.Messages, Tools: request.Tools})
		if err != nil {
			t.Fatal(err)
		}
		providerBody := string(body)
		if strings.Contains(providerBody, rawAccount) || strings.Contains(providerBody, rawCard) ||
			domaincaseentity.ContainsReferenceCandidateV1(providerBody) ||
			strings.Contains(providerBody, domaincaseentity.ReferencePrefixV1) {
			t.Fatalf("provider request leaked source-exact or authority reference")
		}
		for _, alias := range aliases {
			if !strings.Contains(providerBody, alias) {
				t.Fatalf("provider request omitted task-selected alias %q", alias)
			}
		}
		return providerBody
	}
	assertLatestProviderLongitudinal := func(t *testing.T, provider *providerStepRecordingProvider, digests ...string) string {
		t.Helper()
		requests := provider.Requests()
		if len(requests) == 0 {
			t.Fatal("provider received no longitudinal request")
		}
		prompt := lastProviderStepUserPrompt(requests[len(requests)-1].Messages)
		for _, field := range []string{
			`"schemaVersion":1`,
			`"selectionBudget":32`,
			`"items"`,
			`"omittedCoverage"`,
		} {
			if !strings.Contains(prompt, field) {
				t.Fatalf("provider request omitted bounded typed longitudinal field %s", field)
			}
		}
		if strings.Contains(prompt, `"claims":[`) || strings.Contains(prompt, `"evidence":[`) ||
			strings.Contains(prompt, `"continuations":[`) {
			t.Fatal("provider request received legacy per-field longitudinal truncation")
		}
		if strings.Contains(prompt, `"reference":`) {
			t.Fatal("provider request reflected a raw longitudinal owner reference")
		}
		for _, digest := range digests {
			if !strings.Contains(prompt, digest) {
				t.Fatalf("provider request omitted selected owner digest %q", digest)
			}
		}
		return prompt
	}
	snapshotBindingDigest := func(t *testing.T, prompt string) string {
		t.Helper()
		match := caseIngressSnapshotBindingDigestPatternV1.FindStringSubmatch(prompt)
		if len(match) != 2 {
			t.Fatalf("provider longitudinal selection omitted a snapshot binding digest")
		}
		return match[1]
	}
	scopeBindingDigest := func(t *testing.T, prompt string) string {
		t.Helper()
		match := caseIngressScopeBindingDigestPatternV1.FindStringSubmatch(prompt)
		if len(match) != 2 {
			t.Fatal("provider longitudinal selection omitted a case-scope binding digest")
		}
		return match[1]
	}
	referenceDigest := func(t *testing.T, prompt string) string {
		t.Helper()
		match := caseIngressReferenceDigestPatternV1.FindStringSubmatch(prompt)
		if len(match) != 2 {
			t.Fatal("provider longitudinal selection omitted an opaque owner reference digest")
		}
		return match[1]
	}
	threadContext := func(t *testing.T, handler *runtimeServerHandler, threadID string) domainsecurity.TurnSecurityContext {
		t.Helper()
		thread, err := handler.store.GetThread(threadID)
		if err != nil {
			t.Fatal(err)
		}
		securityContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
		if err != nil {
			t.Fatalf("parse current thread security context: %v", err)
		}
		return securityContext
	}
	aliasReferences := func(t *testing.T, store *serverCaseIngressStoreV1, securityContext domainsecurity.TurnSecurityContext) map[string]domaincaseentity.ReferenceV1 {
		t.Helper()
		index, err := store.ResolveLatestCaseLongitudinalContext(context.Background(), securityContext)
		if err != nil {
			t.Fatalf("resolve case longitudinal index: %v", err)
		}
		result := make(map[string]domaincaseentity.ReferenceV1, len(index.EntityIdentities))
		for _, identity := range index.EntityIdentities {
			alias, aliasErr := domaincaseentity.NewModelEntityAliasV1(identity.EntityType, identity.StableOrdinal)
			if aliasErr != nil {
				t.Fatal(aliasErr)
			}
			result[string(alias)] = identity.Reference
		}
		return result
	}

	handlerA, datasetA, storeA, providerA, serverA, privateRootA, durableRootA := newFixture(t)
	workspaceA := datasetA.binding.WorkspaceRealPath
	activeStoreA := storeA
	t.Cleanup(func() {
		if activeStoreA != nil {
			if closer, ok := activeStoreA.Store.(interface{ Close() error }); ok {
				if err := closer.Close(); err != nil {
					t.Errorf("close active caseentity store: %v", err)
				}
			}
		}
	})
	originA := createThread(t, serverA.URL, workspaceA)
	before := len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, originA, "请分析银行账号 "+rawAccount+" 与银行卡号 "+rawCard+" 的资金流。")
	if len(providerA.Requests()) == before {
		t.Fatalf("origin stopped before provider: resolverCalls=%d putIngress=%d",
			activeStoreA.resolverCalls.Load(), activeStoreA.putIngress.Load())
	}
	assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
	originContextA := threadContext(t, handlerA, originA)
	referencesA := aliasReferences(t, activeStoreA, originContextA)
	if referencesA["acct:1"] == "" || referencesA["card:2"] == "" || referencesA["acct:1"] == referencesA["card:2"] {
		t.Fatalf("origin did not create distinct account/card host-private references")
	}
	originIndex, err := activeStoreA.ResolveLatestCaseLongitudinalContext(context.Background(), originContextA)
	if err != nil || len(originIndex.Continuations) == 0 {
		t.Fatalf("completed origin turn did not append its owner-produced continuation digest: %v", err)
	}
	originContinuationDigest := originIndex.Continuations[len(originIndex.Continuations)-1].ContinuationDigest
	evidenceReceipt := newCaseLongitudinalEvidenceReceiptV1(t, originContextA)
	if err := caseentityapp.AppendPersistedEvidenceReceiptV1(context.Background(), handlerA.caseEntities, caseentityapp.AppendPersistedEvidenceReceiptInputV1{
		SecurityContext: originContextA, Receipt: evidenceReceipt,
	}); err != nil {
		t.Fatalf("persisted tool evidence producer did not append its opaque digest: %v", err)
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-longitudinal-unresolved",
		Proposal: domainevidence.ClaimProposal{
			SchemaVersion: domainevidence.ClaimProposalVersion,
			ProposalID:    "proposal-longitudinal-unresolved",
			ClaimType:     domainevidence.ClaimEntity,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: "entity-longitudinal", EntityID: "entity-longitudinal", Granularity: "record",
			},
			EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
		},
		SupportState: domainevidence.ClaimUnresolved,
		EvidenceIDs:  []string{}, CounterEvidenceIDs: []string{},
		AllowedWording: []string{}, ProhibitedUpgrades: []string{},
		VerificationReason: "current owner could not verify this claim",
		VerifiedAt:         time.Date(2026, 7, 28, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("construct current claim-owner result: %v", err)
	}
	finalized := &evidenceapp.PersistCaseBoundaryResult{Boundary: evidenceapp.CaseBoundaryResult{
		Envelope: domainevidence.FinalAnswerEnvelope{Claims: []domainevidence.ClaimRecord{claim}},
	}}
	if err := caseentityapp.AppendFinalizedCaseLongitudinalStateV1(
		context.Background(),
		handlerA.caseEntities,
		handlerA.store,
		originA,
		originContextA,
		finalized,
	); err != nil {
		t.Fatalf("final claim producer did not append its opaque digest: %v", err)
	}
	ownerIndex, err := activeStoreA.ResolveLatestCaseLongitudinalContext(context.Background(), originContextA)
	if err != nil || len(ownerIndex.Claims) != 1 || ownerIndex.Claims[0].TypedState == nil ||
		ownerIndex.Claims[0].TypedState.SchemaVersion != domaincaseentity.CaseClaimTypedStateSchemaVersionV1 ||
		ownerIndex.Claims[0].TypedState.ClaimType != string(domainevidence.ClaimEntity) {
		t.Fatalf("final ClaimRecord owner did not append versioned typed state: index=%#v err=%v", ownerIndex, err)
	}
	before = len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, originA, "继续查询本案账户和银行卡的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
	preRestartPrompt := assertLatestProviderLongitudinal(
		t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest,
	)
	if !caseIngressReferenceDigestPatternV1.MatchString(preRestartPrompt) {
		t.Fatal("provider request omitted the scoped opaque owner reference digest")
	}
	preRestartSnapshotBinding := snapshotBindingDigest(t, preRestartPrompt)
	preRestartScopeBinding := scopeBindingDigest(t, preRestartPrompt)
	preRestartReferenceDigest := referenceDigest(t, preRestartPrompt)
	for _, rawOwnerReference := range []string{
		string(evidenceReceipt.ReceiptID), claim.ClaimID, originContextA.CaseID,
		originContextA.CaseBindingHash, originContextA.DatasetSnapshotID,
	} {
		if strings.Contains(preRestartPrompt, rawOwnerReference) {
			t.Fatalf("provider request reflected raw longitudinal owner reference %q", rawOwnerReference)
		}
	}

	// Reopen the real caseentity CAS before creating the independent thread.
	// The second turn does not contain an alias or complete identifier.
	if closer, ok := activeStoreA.Store.(interface{ Close() error }); !ok || closer.Close() != nil {
		t.Fatal("close caseentity store before restart")
	}
	activeStoreA = configureCaseIngressEntityServiceAtRootV1(t, handlerA, datasetA, privateRootA, false)
	independentA := createThread(t, serverA.URL, workspaceA)
	before = len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, independentA, "请查询本案账户和银行卡在指定期间的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
	independentProviderPrompt := assertLatestProviderLongitudinal(
		t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest,
	)
	if restartedSnapshotBinding := snapshotBindingDigest(t, independentProviderPrompt); restartedSnapshotBinding != preRestartSnapshotBinding {
		t.Fatalf("same-case snapshot binding changed across restart: before=%s after=%s", preRestartSnapshotBinding, restartedSnapshotBinding)
	}
	if scopeBindingDigest(t, independentProviderPrompt) != preRestartScopeBinding ||
		referenceDigest(t, independentProviderPrompt) != preRestartReferenceDigest {
		t.Fatal("same-case scope or opaque owner reference digest changed across restart")
	}
	if !strings.Contains(independentProviderPrompt, originContinuationDigest) {
		t.Fatal("independent thread did not receive the owner-produced continuation digest")
	}
	if !strings.Contains(independentProviderPrompt, evidenceReceipt.ReceiptDigest) ||
		!strings.Contains(independentProviderPrompt, claim.RecordDigest) {
		t.Fatal("independent thread did not receive current evidence and claim owner digests")
	}
	independentContextA := threadContext(t, handlerA, independentA)
	independentReferencesA := aliasReferences(t, activeStoreA, independentContextA)
	if independentReferencesA["acct:1"] != referencesA["acct:1"] || independentReferencesA["card:2"] != referencesA["card:2"] {
		t.Fatalf("same-case independent thread changed stable identity")
	}

	before = len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, independentA, "AUTO_SAFE_CASE_CONSTRAINT_V1 继续查询本案账户和银行卡的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
	replayedPrompt := assertLatestProviderLongitudinal(t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest)
	if snapshotBindingDigest(t, replayedPrompt) != preRestartSnapshotBinding {
		t.Fatal("same-case replay changed the current snapshot binding digest")
	}
	if scopeBindingDigest(t, replayedPrompt) != preRestartScopeBinding ||
		referenceDigest(t, replayedPrompt) != preRestartReferenceDigest {
		t.Fatal("same-case replay changed its scope or opaque owner reference digest")
	}
	fork := requestThreadSummaryJSON(
		t, serverA.URL, http.MethodPost, "/v1/threads/"+independentA+"/fork",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{})),
		http.StatusCreated,
	)
	forkID := stringField(fork, "id")
	before = len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, forkID, "继续查询本案账户和银行卡的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
	forkPrompt := assertLatestProviderLongitudinal(t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest)
	if snapshotBindingDigest(t, forkPrompt) != preRestartSnapshotBinding {
		t.Fatal("authorized same-case fork changed the current snapshot binding digest")
	}
	if scopeBindingDigest(t, forkPrompt) != preRestartScopeBinding ||
		referenceDigest(t, forkPrompt) != preRestartReferenceDigest {
		t.Fatal("authorized same-case fork changed its scope or opaque owner reference digest")
	}

	resume := requestThreadSummaryJSON(
		t, serverA.URL, http.MethodPost, "/v1/sessions/"+independentA+"/resume",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{})),
		http.StatusCreated,
	)
	resumeID := stringField(resume, "thread_id")
	before = len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, resumeID, "继续查询本案账户和银行卡的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
	resumePrompt := assertLatestProviderLongitudinal(t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest)
	if snapshotBindingDigest(t, resumePrompt) != preRestartSnapshotBinding {
		t.Fatal("same-case resume changed the current snapshot binding digest")
	}
	if scopeBindingDigest(t, resumePrompt) != preRestartScopeBinding ||
		referenceDigest(t, resumePrompt) != preRestartReferenceDigest {
		t.Fatal("same-case resume changed its scope or opaque owner reference digest")
	}

	defer func() {
		autoA := createThread(t, serverA.URL, workspaceA)
		requestThreadSummaryJSON(
			t, serverA.URL, http.MethodPost, "/v1/threads/"+autoA+"/goal",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{
				"objective": "Preserve the active longitudinal case objective across automatic compaction",
			})),
			http.StatusOK,
		)
		requestThreadSummaryJSON(
			t, serverA.URL, http.MethodPost, "/v1/threads/"+autoA+"/todos",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{
				"todos": []map[string]any{{
					"content": "Preserve the unfinished longitudinal case todo", "status": "pending",
				}},
			})),
			http.StatusOK,
		)
		caseTurnSecurity := handlerA.turnSecurity
		caseBindingPath := filepath.Join(workspaceA, ".analytix", "case-project.json")
		caseBindingBody, err := os.ReadFile(caseBindingPath)
		if err != nil {
			t.Fatal(err)
		}
		caseBindingRestored := false
		defer func() {
			if !caseBindingRestored {
				_ = os.WriteFile(caseBindingPath, caseBindingBody, 0o600)
			}
		}()
		if err := os.Remove(caseBindingPath); err != nil {
			t.Fatal(err)
		}
		handlerA.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{
			Identity: testIdentityAuthority(), Observer: filestore.CaseBindingReader{}, RiskAuthority: newServerTestRiskAuthority(),
		}
		handlerA.threads = nil
		before = len(providerA.Requests())
		longResponseCall := before + 1
		providerA.responseText = func(call int, _ domainmodel.Request) string {
			if call == longResponseCall {
				return oldBodyMark + strings.Repeat("界", 17_000)
			}
			return ""
		}
		startTurnWithRisk(
			t, handlerA, serverA.URL, autoA,
			"Summarize the ordinary workspace status before automatic protected compaction.", "", largeModel,
		)
		mixedRaw, err := handlerA.store.GetThreadForAuthorityRepair(autoA)
		if err != nil {
			t.Fatal(err)
		}
		mixedTurns := listAny(mixedRaw["turns"])
		mixedGeneralTurn, _ := mixedTurns[len(mixedTurns)-1].(map[string]any)
		mixedGeneralContext, err := domainsecurity.ParseTurnSecurityContext(mixedGeneralTurn["securityContext"])
		if err != nil || !domainsecurity.TurnSecurityContextIsGeneral(mixedGeneralContext) {
			t.Fatalf("mixed auto-compaction setup did not create a real general terminal: context=%#v err=%v", mixedGeneralContext, err)
		}
		mixedGeneralBody, err := json.Marshal(mixedGeneralTurn)
		if err != nil || !bytes.Contains(mixedGeneralBody, []byte(oldBodyMark)) {
			t.Fatalf("mixed auto-compaction general terminal omitted its long source: err=%v", err)
		}
		if err := os.WriteFile(caseBindingPath, caseBindingBody, 0o600); err != nil {
			t.Fatal(err)
		}
		caseBindingRestored = true
		handlerA.turnSecurity = caseTurnSecurity
		handlerA.threads = nil
		var preAutoReboundPrompt string
		for _, prompt := range []string{
			"请查询本案账户和银行卡在指定期间的流入、流出、净额和交易笔数。",
			"继续查询本案账户和银行卡的流入、流出、净额和交易笔数，并核对反证与缺口。",
			"AUTO_SAFE_CASE_CONSTRAINT_V1 继续查询本案账户和银行卡的流入、流出、净额和交易笔数。",
		} {
			before = len(providerA.Requests())
			startTurn(t, handlerA, serverA.URL, autoA, prompt, largeModel)
			assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
			preAutoReboundPrompt = assertLatestProviderLongitudinal(
				t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest,
			)
		}
		preAutoSnapshotBinding := snapshotBindingDigest(t, preAutoReboundPrompt)
		preAutoScopeBinding := scopeBindingDigest(t, preAutoReboundPrompt)
		preAutoReferenceDigest := referenceDigest(t, preAutoReboundPrompt)

		before = len(providerA.Requests())
		hardFailure := requestThreadSummaryJSON(
			t, serverA.URL, http.MethodPost, "/v1/threads/"+autoA+"/turns",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{
				"prompt":     "自动压缩后继续查询本案账户和银行卡的流入、流出、净额和交易笔数。",
				"riskIntent": "case", "model": hardModel,
			})),
			http.StatusInternalServerError,
		)
		if len(providerA.Requests()) != before {
			t.Fatalf("post-compaction hard-limit admission reached provider: before=%d after=%d", before, len(providerA.Requests()))
		}
		if stringField(hardFailure, "reasonCode") != "context_window_hard_limit" {
			t.Fatalf("post-compaction failure lost the hard-limit classification: %#v", hardFailure)
		}
		hardRaw, err := handlerA.store.GetThreadForAuthorityRepair(autoA)
		if err != nil {
			t.Fatal(err)
		}
		hardTurnID := ""
		hardFailureTurns := 0
		hardAutomaticCompactions := 0
		for _, rawTurn := range listAny(hardRaw["turns"]) {
			turn, _ := rawTurn.(map[string]any)
			for _, rawItem := range listAny(turn["items"]) {
				item, _ := rawItem.(map[string]any)
				if stringField(item, "kind") == "compaction" && item["auto"] == true {
					hardAutomaticCompactions++
				}
			}
			if stringField(turn, "status") == "failed" {
				hardFailureTurns++
				hardTurnID = stringField(turn, "id")
			}
		}
		if hardAutomaticCompactions != 1 || hardFailureTurns != 1 || hardTurnID == "" {
			t.Fatalf("hard-limit admission did not close after exactly one automatic compaction: markers=%d failures=%d turn=%q response=%#v",
				hardAutomaticCompactions, hardFailureTurns, hardTurnID, hardFailure)
		}
		hardReplay, err := handlerA.store.LoadEventsSince(autoA, 0)
		if err != nil {
			t.Fatal(err)
		}
		hardAdmissionRejected, hardTerminal, hardOtherTerminals := 0, 0, 0
		compactionCompletedIndex, hardTerminalIndex := -1, -1
		for index, event := range hardReplay.Events {
			if event["auto"] == true && stringField(event, "kind") == "compaction_completed" {
				compactionCompletedIndex = index
			}
			if stringField(event, "turnId") != hardTurnID {
				continue
			}
			if stringField(event, "stage") == "provider_admission_rejected" {
				hardAdmissionRejected++
			}
			switch stringField(event, "kind") {
			case "turn_failed":
				hardTerminal++
				hardTerminalIndex = index
			case "turn_completed", "turn_aborted":
				hardOtherTerminals++
			}
		}
		if hardAdmissionRejected != 1 || hardTerminal != 1 || hardOtherTerminals != 0 ||
			compactionCompletedIndex < 0 || hardTerminalIndex <= compactionCompletedIndex {
			t.Fatalf("post-compaction hard-limit admission was not single-terminal: rejected=%d failed=%d other=%d compact=%d terminal=%d",
				hardAdmissionRejected, hardTerminal, hardOtherTerminals, compactionCompletedIndex, hardTerminalIndex)
		}

		before = len(providerA.Requests())
		startTurn(t, handlerA, serverA.URL, autoA, "压缩失败终态后继续查询本案账户和银行卡的流入、流出、净额和交易笔数。", smallModel)
		postAutoProviderBody := assertLatestProviderAliases(t, providerA, before, "acct:1", "card:2")
		postAutoLongitudinal := assertLatestProviderLongitudinal(t, providerA, originContinuationDigest, evidenceReceipt.ReceiptDigest, claim.RecordDigest)
		postAutoSnapshotBinding := snapshotBindingDigest(t, postAutoLongitudinal)
		if postAutoSnapshotBinding == preAutoSnapshotBinding {
			t.Fatal("automatic compaction did not rebind the longitudinal selection to the new current snapshot")
		}
		if scopeBindingDigest(t, postAutoLongitudinal) != preAutoScopeBinding ||
			referenceDigest(t, postAutoLongitudinal) != preAutoReferenceDigest {
			t.Fatal("automatic compaction changed the longitudinal case scope or opaque identity binding")
		}
		trustedRaw, err := handlerA.store.GetThreadForAuthorityRepair(autoA)
		if err != nil {
			t.Fatal(err)
		}
		trustedCompactions := threadapp.TrustedCaseCompactionTurnIDsV1(trustedRaw, handlerA.caseThreads)
		if len(trustedCompactions) != 1 {
			t.Fatalf("production automatic case compaction is not a trusted provider cut: %#v", trustedCompactions)
		}
		for _, required := range []string{
			"Analytix task continuation snapshot",
			"Preserve the active longitudinal case objective across automatic compaction",
			"Preserve the unfinished longitudinal case todo",
			"AUTO_SAFE_CASE_CONSTRAINT_V1",
		} {
			if !strings.Contains(postAutoProviderBody, required) {
				t.Fatalf("post-auto provider request omitted mixed continuation %q", required)
			}
		}
		for _, forbidden := range []string{
			oldBodyMark, rawAccount, rawCard, "caseCompactionBinding", "sourceContextDigest",
			"authorityTurnIds", "PRIVATE_CASE_COMPACTION_SOURCE_MUST_NOT_CROSS_PUBLIC_SEAM",
		} {
			if strings.Contains(postAutoProviderBody, forbidden) || strings.Contains(postAutoLongitudinal, forbidden) {
				t.Fatalf("post-auto provider request replayed private or compacted source %q", forbidden)
			}
		}
		rawIndependent, err := handlerA.store.GetThreadForAuthorityRepair(autoA)
		if err != nil {
			t.Fatal(err)
		}
		automaticCompactions := 0
		for _, rawTurn := range listAny(rawIndependent["turns"]) {
			turn, _ := rawTurn.(map[string]any)
			for _, rawItem := range listAny(turn["items"]) {
				item, _ := rawItem.(map[string]any)
				if stringField(item, "kind") == "compaction" && item["auto"] == true {
					automaticCompactions++
				}
			}
		}
		replayAfterAuto, err := handlerA.store.LoadEventsSince(autoA, 0)
		if err != nil {
			t.Fatal(err)
		}
		startedAutomatic, completedAutomatic := 0, 0
		for _, event := range replayAfterAuto.Events {
			if event["auto"] != true {
				continue
			}
			switch stringField(event, "kind") {
			case "compaction_started":
				startedAutomatic++
			case "compaction_completed":
				completedAutomatic++
			}
		}
		if automaticCompactions != 1 || startedAutomatic != 1 || completedAutomatic != 1 {
			t.Fatalf("production auto case compaction was not exactly one marker and lifecycle pair: markers=%d started=%d completed=%d",
				automaticCompactions, startedAutomatic, completedAutomatic)
		}
		publicThread := requestThreadSummaryJSON(t, serverA.URL, http.MethodGet, "/v1/threads/"+autoA, nil, http.StatusOK)
		publicSSE := requestRuntimeSSEText(t, serverA.URL+"/v1/threads/"+autoA+"/events?since_seq=0")
		privateLongitudinalValues := []string{
			rawCard, string(evidenceReceipt.ReceiptID), evidenceReceipt.ReceiptDigest,
			claim.ClaimID, claim.RecordDigest, originContinuationDigest,
			preRestartScopeBinding, preRestartSnapshotBinding, preRestartReferenceDigest,
		}
		assertCaseIngressPublicSurfacesV1(
			t, handlerA, durableRootA, autoA, rawAccount, publicThread, publicSSE,
			privateLongitudinalValues...,
		)
		for name, response := range map[string]map[string]any{"fork response": fork, "resume response": resume} {
			body, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range append([]string{rawAccount}, privateLongitudinalValues...) {
				if strings.Contains(string(body), forbidden) {
					t.Fatalf("%s reflected private longitudinal owner state", name)
				}
			}
			if domaincaseentity.ContainsReferenceCandidateV1(string(body)) || strings.Contains(string(body), domaincaseentity.ReferencePrefixV1) {
				t.Fatalf("%s reflected private case authority", name)
			}
		}
	}()

	// A fork may use only refs that the source thread selected. An explicit new
	// source-exact value does not borrow the fork relation to enlarge authority.
	accountOnly := createThread(t, serverA.URL, workspaceA)
	before = len(providerA.Requests())
	startTurn(t, handlerA, serverA.URL, accountOnly, "请查询本案账户在指定期间的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerA, before, "acct:1")
	narrowFork := requestThreadSummaryJSON(
		t, serverA.URL, http.MethodPost, "/v1/threads/"+accountOnly+"/fork",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{})),
		http.StatusCreated,
	)
	before = len(providerA.Requests())
	const rawUnselectedCard = "6217001111222233"
	narrowForkID := stringField(narrowFork, "id")
	startTurn(t, handlerA, serverA.URL, narrowForkID, "请改为分析银行卡号 "+rawUnselectedCard+" 的资金流。")
	for _, request := range providerA.Requests()[before:] {
		body, err := json.Marshal(struct {
			SystemPrompt string                   `json:"systemPrompt"`
			Messages     []domainmodel.Message    `json:"messages"`
			Tools        []domainmodel.ToolSchema `json:"tools"`
		}{SystemPrompt: request.SystemPrompt, Messages: request.Messages, Tools: request.Tools})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), rawUnselectedCard) || strings.Contains(string(body), "card:3") ||
			domaincaseentity.ContainsReferenceCandidateV1(string(body)) {
			t.Fatalf("narrow fork enlarged the source thread entity authority")
		}
	}
	narrowContext := threadContext(t, handlerA, narrowForkID)
	unselectedReference, err := handlerA.caseEntities.DeriveReferenceV1(
		context.Background(),
		caseentityapp.NewDeriveReferenceInputV1(
			narrowContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1,
			rawUnselectedCard,
		),
	)
	if err != nil {
		t.Fatalf("derive unselected fork reference for negative verification: %v", err)
	}
	privateCalls := 0
	err = handlerA.caseEntities.UseVerifiedBindingByReferenceV1(
		context.Background(),
		caseentityapp.ResolveVerifiedBindingByReferenceInputV1{
			SecurityContext: narrowContext,
			Reference:       unselectedReference,
		},
		func(string, string) error {
			privateCalls++
			return nil
		},
	)
	if err == nil || privateCalls != 0 {
		t.Fatalf("rejected fork source value persisted a private binding: calls=%d err=%v", privateCalls, err)
	}

	handlerB, datasetB, storeB, providerB, serverB, _, _ := newFixture(t)
	defer func() {
		if closer, ok := storeB.Store.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	originB := createThread(t, serverB.URL, datasetB.binding.WorkspaceRealPath)
	before = len(providerB.Requests())
	startTurn(t, handlerB, serverB.URL, originB, "请分析银行账号 "+rawAccount+" 与银行卡号 "+rawCard+" 的资金流。")
	assertLatestProviderAliases(t, providerB, before, "acct:1", "card:2")
	contextB := threadContext(t, handlerB, originB)
	if contextB.DatasetSnapshotID == originContextA.DatasetSnapshotID {
		t.Fatalf("DSV2 identity was reused across cases: %s", contextB.DatasetSnapshotID)
	}
	referencesB := aliasReferences(t, storeB, contextB)
	if referencesB["acct:1"] == referencesA["acct:1"] || referencesB["card:2"] == referencesA["card:2"] {
		t.Fatalf("same canonical values were linkable across cases")
	}
	before = len(providerB.Requests())
	startTurn(t, handlerB, serverB.URL, originB, "继续查询本案账户和银行卡的流入、流出、净额和交易笔数。")
	assertLatestProviderAliases(t, providerB, before, "acct:1", "card:2")
	caseBPrompt := assertLatestProviderLongitudinal(t, providerB)
	if caseBSnapshotBinding := snapshotBindingDigest(t, caseBPrompt); caseBSnapshotBinding == preRestartSnapshotBinding {
		t.Fatalf("snapshot binding digest was reusable across cases: %s", caseBSnapshotBinding)
	}
	if caseBScopeBinding := scopeBindingDigest(t, caseBPrompt); caseBScopeBinding == preRestartScopeBinding {
		t.Fatalf("case-scoped provider selection was reusable across cases: %s", caseBScopeBinding)
	}
	for _, test := range []struct {
		name    string
		service *caseentityapp.Service
		context domainsecurity.TurnSecurityContext
		alias   domaincaseentity.ModelEntityAliasV1
		want    domaincaseentity.ReferenceV1
	}{
		{name: "case A account", service: handlerA.caseEntities, context: independentContextA, alias: "acct:1", want: referencesA["acct:1"]},
		{name: "case B account", service: handlerB.caseEntities, context: contextB, alias: "acct:1", want: referencesB["acct:1"]},
	} {
		calls := 0
		err := test.service.UseVerifiedBindingByAliasV1(context.Background(), caseentityapp.ResolveVerifiedBindingByAliasInputV1{
			SecurityContext: test.context, Alias: test.alias,
		}, func(reference domaincaseentity.ReferenceV1, _ string, _ string) error {
			calls++
			if reference != test.want {
				return errors.New("unexpected case-local reference")
			}
			return nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("%s alias did not resolve inside its current case: calls=%d err=%v", test.name, calls, err)
		}
	}
	crossCalls := 0
	if err := handlerA.caseEntities.UseVerifiedBindingByAliasV1(context.Background(), caseentityapp.ResolveVerifiedBindingByAliasInputV1{
		SecurityContext: contextB, Alias: "acct:1",
	}, func(domaincaseentity.ReferenceV1, string, string) error {
		crossCalls++
		return nil
	}); err == nil || crossCalls != 0 {
		t.Fatalf("acct:1 crossed its current case boundary: calls=%d", crossCalls)
	}
}

func newCaseLongitudinalEvidenceReceiptV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
) domainevidence.EvidenceReceipt {
	t.Helper()
	const serverVersion = "0.16.16"
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", serverVersion,
		domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-server")), 7,
	)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(json.RawMessage(`{"schemaVersion":1,"facts":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	settlementID := domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-settlement"))
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID:         domainevidence.EvidenceSettlementReceiptID(settlementID),
		Context:           securityContext,
		ExecutionGrantID:  domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-grant")),
		ToolCallID:        serverTestHostToolCallID("case-longitudinal-evidence"),
		ServerIdentity:    identity,
		ServerVersion:     serverVersion,
		ConnectionEpoch:   7,
		ToolName:          providerStepFundsTool,
		ArgsHash:          domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-args")),
		ResultHash:        domainsecurity.CanonicalJSONHash(canonical),
		SourceType:        "case_metadata",
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash:         domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-query")),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{}, AccountIDs: []string{}, Directions: []string{},
			SourceIDs:   []string{"case-longitudinal-owner"},
			FiltersHash: domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-filter")),
		},
		Granularity: "record", Timezone: "UTC",
		PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs:        []string{}, RawSHA256: domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-raw")),
		TransformationLineage: []domainevidence.TransformationLineageStep{},
		PIIClassification:     domainevidence.PIINone,
		IssuedAt:              time.Date(2026, 7, 28, 0, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("construct evidence receipt draft: %v", err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	_, receipt, err := domainevidence.RegisterEvidenceReceipt(
		registry,
		draft,
		canonical,
		domainevidence.EvidenceSettlementProof{
			SettlementID:         settlementID,
			PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("case-longitudinal-evidence-prepared")),
		},
		time.Date(2026, 7, 28, 0, 31, 0, 0, time.UTC),
	)
	if err != nil || domainevidence.ValidateEvidenceReceipt(receipt) != nil {
		t.Fatalf("seal evidence receipt fixture: %v", err)
	}
	return receipt
}

type serverSignedCurrentDatasetV1 struct {
	mu          sync.Mutex
	binding     domainsecurity.CaseBinding
	observation domainsecurity.CaseBindingObservationV1
	selection   datasetsnapshotport.CurrentSelectionV2
	revoked     bool
}

func newServerSignedCurrentDatasetV1(t *testing.T, workspace string) *serverSignedCurrentDatasetV1 {
	t.Helper()
	binding, err := (filestore.CaseBindingReader{}).ReadCurrentBinding(workspace)
	if err != nil {
		t.Fatal(err)
	}
	harness := currentdatasettest.NewHarness()
	seed, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-ingress-seed", TurnID: "turn-case-ingress-seed",
		WorkspaceRealPath: binding.WorkspaceRealPath,
		TenantID:          domainsecurity.LocalTenantID,
		UserID:            domainsecurity.LocalUserID,
		CaseID:            binding.CaseID,
		CaseBindingHash:   binding.CaseBindingHash,
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("case-ingress-seed"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("case-ingress-seed-source-manifest"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.ValidateCurrent(context.Background(), seed); err != nil {
		t.Fatal(err)
	}
	observation, err := harness.Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	var selection datasetsnapshotport.CurrentSelectionV2
	err = harness.WithCurrentSelectionV2(
		context.Background(),
		datasetsnapshotport.ResolveInputV2{
			TenantID: seed.TenantID, UserID: seed.UserID, Observation: observation,
			ExpectedDatasetSnapshotID: seed.DatasetSnapshotID,
		},
		seed,
		func(current datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			selection = cloneServerCurrentSelectionV1(current)
			return capability.UseExact(current, seed, func(context.Context) error { return nil })
		},
	)
	if err != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil {
		t.Fatalf("capture signed current dataset selection: %v", err)
	}
	return &serverSignedCurrentDatasetV1{
		binding: binding, observation: observation, selection: selection,
	}
}

func (authority *serverSignedCurrentDatasetV1) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if authority == nil || filepath.Clean(strings.TrimSpace(workspace)) != authority.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, datasetsnapshotport.ErrMismatch
	}
	return authority.observation, nil
}

func (authority *serverSignedCurrentDatasetV1) ReadCurrentBinding(workspace string) (domainsecurity.CaseBinding, error) {
	if authority == nil {
		return domainsecurity.CaseBinding{}, datasetsnapshotport.ErrUnavailable
	}
	current, err := (filestore.CaseBindingReader{}).ReadCurrentBinding(workspace)
	if err != nil || current.WorkspaceRealPath != authority.binding.WorkspaceRealPath ||
		current.CaseID != authority.binding.CaseID || current.CaseBindingHash != authority.binding.CaseBindingHash {
		return domainsecurity.CaseBinding{}, datasetsnapshotport.ErrMismatch
	}
	return current, nil
}

func (authority *serverSignedCurrentDatasetV1) ResolveWitnessedV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if ctx == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	authority.mu.Lock()
	revoked := authority.revoked
	selection := cloneServerCurrentSelectionV1(authority.selection)
	observation := authority.observation
	authority.mu.Unlock()
	if revoked {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	if input.Observation != observation ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV2(
			selection.Snapshot.Record, input.TenantID, input.UserID, input.Observation,
		) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	if input.ExpectedDatasetSnapshotID != "" &&
		input.ExpectedDatasetSnapshotID != selection.Snapshot.Record.DatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return selection.Snapshot, nil
}

func (authority *serverSignedCurrentDatasetV1) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if ctx == nil || callback == nil {
		return datasetsnapshotport.ErrUnavailable
	}
	if err := authority.validateCurrent(ctx, securityContext); err != nil {
		return err
	}
	authority.mu.Lock()
	selection := cloneServerCurrentSelectionV1(authority.selection)
	observation := authority.observation
	authority.mu.Unlock()
	if input.TenantID != securityContext.TenantID || input.UserID != securityContext.UserID ||
		input.Observation != observation || input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID {
		return datasetsnapshotport.ErrMismatch
	}
	capability := &serverSignedCurrentDatasetCapabilityV1{
		authority: authority, expected: securityContext, selection: selection, ctx: ctx,
	}
	capability.active.Store(true)
	defer capability.active.Store(false)
	return callback(cloneServerCurrentSelectionV1(selection), capability)
}

func (authority *serverSignedCurrentDatasetV1) SetDatasetRevoked(revoked bool) {
	if authority == nil {
		return
	}
	authority.mu.Lock()
	authority.revoked = revoked
	authority.mu.Unlock()
}

func (authority *serverSignedCurrentDatasetV1) validateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if authority == nil || ctx == nil {
		return datasetsnapshotport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	authority.mu.Lock()
	revoked := authority.revoked
	selection := authority.selection
	observation := authority.observation
	authority.mu.Unlock()
	record := selection.Snapshot.Record
	binding := record.Binding
	if revoked {
		return datasetsnapshotport.ErrStale
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		securityContext.TenantID != binding.TenantID || securityContext.UserID != binding.UserID ||
		securityContext.WorkspaceRealPath != binding.WorkspaceRealPath || securityContext.CaseID != binding.CaseID ||
		securityContext.CaseBindingHash != binding.CaseBindingHash ||
		securityContext.PublicationPolicy.BindingObservationDigest != observation.ObservationDigest ||
		securityContext.DatasetSnapshotID != record.DatasetSnapshotID ||
		securityContext.SourceManifestHash != record.SourceManifestHash ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil {
		return datasetsnapshotport.ErrMismatch
	}
	return nil
}

type serverSignedCurrentDatasetCapabilityV1 struct {
	authority *serverSignedCurrentDatasetV1
	expected  domainsecurity.TurnSecurityContext
	selection datasetsnapshotport.CurrentSelectionV2
	ctx       context.Context
	active    atomic.Bool
	used      atomic.Bool
}

func (capability *serverSignedCurrentDatasetCapabilityV1) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if capability == nil || use == nil || !capability.active.Load() ||
		!capability.used.CompareAndSwap(false, true) || securityContext != capability.expected ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		selection.Snapshot.Record.RecordDigest != capability.selection.Snapshot.Record.RecordDigest {
		return datasetsnapshotport.ErrMismatch
	}
	if err := capability.authority.validateCurrent(capability.ctx, securityContext); err != nil {
		return err
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	useErr := use(leaseContext)
	return errors.Join(useErr, capability.authority.validateCurrent(capability.ctx, securityContext))
}

func (capability *serverSignedCurrentDatasetCapabilityV1) CommitExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	commit func(context.Context) error,
) error {
	if capability == nil || commit == nil || !capability.active.Load() ||
		!capability.used.CompareAndSwap(false, true) || securityContext != capability.expected ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		selection.Snapshot.Record.RecordDigest != capability.selection.Snapshot.Record.RecordDigest {
		return datasetsnapshotport.ErrMismatch
	}
	if err := capability.authority.validateCurrent(capability.ctx, securityContext); err != nil {
		return err
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	return commit(leaseContext)
}

func cloneServerCurrentSelectionV1(selection datasetsnapshotport.CurrentSelectionV2) datasetsnapshotport.CurrentSelectionV2 {
	selection.DatasetIndexPath = append(
		[]domainsecurity.DatasetSnapshotIndexV1(nil),
		selection.DatasetIndexPath...,
	)
	return selection
}

type serverCaseIngressStoreV1 struct {
	caseentityport.Store
	putIngress         atomic.Int64
	resolveIngress     atomic.Int64
	resolverCalls      atomic.Int64
	mu                 sync.Mutex
	resolvedCandidates []string
}

func (store *serverCaseIngressStoreV1) PutIngressIfAbsent(
	ctx context.Context,
	record domaincaseentity.CaseIngressRecord,
) error {
	err := store.Store.PutIngressIfAbsent(ctx, record)
	if err == nil {
		store.putIngress.Add(1)
	}
	return err
}

func (store *serverCaseIngressStoreV1) ResolveIngress(
	ctx context.Context,
	key string,
) (domaincaseentity.CaseIngressRecord, error) {
	record, err := store.Store.ResolveIngress(ctx, key)
	if err == nil {
		store.resolveIngress.Add(1)
	}
	return record, err
}

type serverCaseIngressKeyedDigesterV1 struct {
	key []byte
}

func (digester serverCaseIngressKeyedDigesterV1) KeyedPayloadHash(
	_ context.Context,
	purpose string,
	payload []byte,
) (string, error) {
	mac := hmac.New(sha256.New, digester.key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func configureCaseIngressEntityServiceV1(
	t *testing.T,
	handler *runtimeServerHandler,
	dataset *serverSignedCurrentDatasetV1,
) *serverCaseIngressStoreV1 {
	t.Helper()
	return configureCaseIngressEntityServiceAtRootV1(t, handler, dataset, t.TempDir(), true)
}

func configureCaseIngressEntityServiceAtRootV1(
	t *testing.T,
	handler *runtimeServerHandler,
	dataset *serverSignedCurrentDatasetV1,
	privateRoot string,
	cleanup bool,
) *serverCaseIngressStoreV1 {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	persistent, err := caseentityadapter.NewStore(filepath.Join(privateRoot, "case-entity"), access)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup {
		t.Cleanup(func() {
			if err := persistent.Close(); err != nil {
				t.Errorf("close case entity private store: %v", err)
			}
		})
	}
	store := &serverCaseIngressStoreV1{Store: persistent}
	handler.caseEntities = caseentityapp.NewPersistentService(
		serverCaseIngressKeyedDigesterV1{key: []byte("server-case-ingress-installation-key")},
		store,
		dataset,
		dataset,
		func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: handler.turnSecurity.Identity,
				Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
				SnapshotAuthority:   handler.turnSecurity.SnapshotAuthority,
				SnapshotAuthorityV2: handler.turnSecurity.SnapshotAuthorityV2,
				Context:             securityContext, Workspace: securityContext.WorkspaceRealPath,
			})
		},
	)
	handler.resolveCaseIngress = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		candidates caseentityapp.AccountIngressCandidateBatchV1,
	) (caseentityapp.AccountIngressResolutionBatchV1, error) {
		store.resolverCalls.Add(1)
		var values []string
		if err := candidates.UseExactV1(func(current []string) error {
			values = append([]string(nil), current...)
			return nil
		}); err != nil {
			return caseentityapp.AccountIngressResolutionBatchV1{}, err
		}
		store.mu.Lock()
		store.resolvedCandidates = append([]string(nil), values...)
		store.mu.Unlock()

		dataset.mu.Lock()
		selection := cloneServerCurrentSelectionV1(dataset.selection)
		observation := dataset.observation
		dataset.mu.Unlock()
		descriptor, err := fundsquerysourceapp.DescriptorForCurrentSelectionV1(
			selection, securityContext, observation,
		)
		if err != nil {
			return caseentityapp.AccountIngressResolutionBatchV1{}, err
		}
		resolutions := make([]domainnative.AccountIngressResolutionV1, len(values))
		for index := range values {
			entityType := domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
			accountType := "settlement account"
			if len(values[index]) == 16 {
				entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
				accountType = "debit card"
			}
			resolutions[index] = domainnative.AccountIngressResolutionV1{
				Ordinal:         uint32(index),
				Disposition:     domainnative.AccountIngressResolutionDispositionResolvedV1,
				EntityType:      entityType,
				BankInstitution: "Bank of Analytix",
				AccountType:     accountType,
			}
		}
		result := domainnative.ResolveAccountIngressResultV1{
			SchemaVersion: 1,
			Contract:      domainnative.AccountIngressResolutionResultContractV1,
			Resolutions:   resolutions,
			Provenance: domainnative.AccountIngressResolutionProvenanceV1{
				DatasetSnapshotID:              securityContext.DatasetSnapshotID,
				ContextEpoch:                   securityContext.ContextEpoch,
				ContextDigest:                  securityContext.ContextDigest,
				CaseBindingHash:                securityContext.CaseBindingHash,
				ExpectedProducerContentID:      descriptor.FundsProducerContentID,
				ExpectedProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
				DuckDBContentSnapshotDigest:    descriptor.DuckDBContentSnapshotDigest,
				DuckDBSnapshotManifestSHA256:   descriptor.DuckDBSnapshotManifestSHA256,
				MaterializationIdentity:        descriptor.MaterializationIdentity,
				SourceSignature: strings.TrimPrefix(
					descriptor.MaterializationIdentity,
					domainsecurity.FundsMaterializationIdentityPrefixV1,
				),
				ResultSignature: domainsecurity.SHA256Hex(
					[]byte("server-case-ingress-result:\x00" + descriptor.DescriptorDigest),
				),
				ProducerContentID:      descriptor.FundsProducerContentID,
				ProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
				QueryContract:          domainnative.AccountIngressResolutionQueryContractV1,
				QuerySQLHash:           domainnative.AccountIngressResolutionQuerySQLHashV1,
			},
		}
		return caseentityapp.NewAccountIngressResolutionBatchV1(securityContext, descriptor, result)
	}
	return store
}

func configureCaseIngressNativeAuthorityV1(t *testing.T, handler *runtimeServerHandler) {
	t.Helper()
	nativeOwner := &providerStepNativeOwner{}
	ordinaryHealth := nativecomponentapp.LiveAuthorityFunc(func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
	) error {
		return turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{
			OperationContext: ctx, Identity: handler.turnSecurity.Identity,
			Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
			Context: securityContext, Workspace: securityContext.WorkspaceRealPath,
		})
	})
	authority, err := nativecomponentapp.NewReadyRuntimeAuthority(nativecomponentapp.RuntimeAuthorityDependencies{
		Health: nativecomponentapp.HealthDependencies{
			Store: handler.store, DurableAuthority: providerStepNativeDurableAuthority{},
			AcquireEffect: func(
				ctx context.Context,
				securityContext domainsecurity.TurnSecurityContext,
			) (context.Context, func(), error) {
				return handler.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, securityContext, true)
			},
			Now: time.Now,
		},
		LiveAuthority: nativecomponentapp.LiveAuthorityFunc(func(
			ctx context.Context,
			securityContext domainsecurity.TurnSecurityContext,
		) error {
			return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: handler.turnSecurity.Identity,
				Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
				SnapshotAuthority:   handler.turnSecurity.SnapshotAuthority,
				SnapshotAuthorityV2: handler.turnSecurity.SnapshotAuthorityV2,
				Context:             securityContext, Workspace: securityContext.WorkspaceRealPath,
			})
		}),
		HealthOnlyAuthority: nativecomponentapp.NewHealthOnlyCurrentnessV1(
			ordinaryHealth,
			nativeOwner.ValidateCurrentDataEngine,
		),
		Owner: nativeOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler.nativeAuthority = authority
}

func configureCaseIngressPublicSeamAuthoritiesV1(
	t *testing.T,
	handler *runtimeServerHandler,
	durableRoot string,
	dataset *serverSignedCurrentDatasetV1,
) {
	t.Helper()
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	caseKeyRoot := filepath.Join(privateRoot, "case-key")
	finalKeyRoot := filepath.Join(privateRoot, "final-key")
	for _, root := range []string{caseKeyRoot, finalKeyRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	caseSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(caseKeyRoot, "authority.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(privateRoot, "case-authority-records"))
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority, err := casethreadapp.NewRegistry(context.Background(), caseSigner, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)

	finalSigner, err := finalauthorityadapter.OpenOrCreateFileAuthority(
		filepath.Join(finalKeyRoot, "authority.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistryadapter.NewStore(filepath.Join(privateRoot, "evidence-registry"), finalSigner)
	if err != nil {
		t.Fatal(err)
	}
	privateFinals, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(handler.store, casReader)
	index := gateprojectionapp.NewTrustedFinalProjectionIndexWithReadback(finalSigner, eventIO.Readback)
	handler.caseFinalizer = evidenceapp.NewCasePublicationFinalizerWithPublicationSnapshots(
		registry, registry, finalSigner, privateFinals, eventIO,
		newServerTestTurnTerminalCoordinator(t, finalSigner, privateFinals), nil, index,
	)
	currentCaseAuthority := threadapp.NewCurrentCaseThreadAuthorityValidator(caseAuthority, dataset, nil)
	handler.publicProjector = threadapp.NewTrustedPublicProjectorWithPrimaryCAS(
		index, caseAuthority, currentCaseAuthority, casReader,
	)
	// Observe the fixture's real pending/job/final owners, including closed and
	// uncommitted identities. The compatibility constructor owns pending-work
	// signing separately from this helper's final signer.
	caseAuthority.SetActiveInheritedHistoryIdentityValidatorV1(func(ctx context.Context, threadID string, turnIDs []string) error {
		ids := make(map[string]bool, len(turnIDs))
		for _, turnID := range turnIDs {
			ids[turnID] = true
		}
		inventory, err := handler.pendingWork.TrustedInventoryV1(ctx)
		if err != nil {
			return err
		}
		for _, receipt := range inventory.Receipts {
			if receipt.Context.ThreadID == threadID && ids[receipt.Context.TurnID] {
				return errors.New("inherited history has a pending execution identity")
			}
			if receipt.ChildProducer != nil {
				for _, child := range receipt.ChildProducer.Children {
					if child.ChildThreadID == threadID && ids[child.ChildTurnID] {
						return errors.New("inherited history has a reserved child execution identity")
					}
				}
			}
		}
		runs, err := jobs.ReadChildRunIdentitySnapshotV1(ctx, filepath.Join(handler.dataDir, "child-runs"))
		if err != nil {
			return err
		}
		if runs.HasLegacyTypeScript {
			return errors.New("active history child identity inventory is incomplete")
		}
		for _, run := range runs.Records {
			if (run.ParentThreadID == threadID && (ids[run.ParentTurnID] || ids[run.AutoContinueTurnID])) || (run.ChildThreadID == threadID && ids[run.ChildTurnID]) {
				return errors.New("inherited history has a child job execution identity")
			}
		}
		finals, err := privateFinals.List(ctx)
		if err != nil {
			return err
		}
		for _, record := range finals {
			keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record.AcceptedFinal)
			if err != nil || finalSigner.VerifyTrusted(ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(record.AcceptedFinal), signature) != nil {
				return errors.New("active history final identity inventory is not trusted")
			}
			if record.SecurityContext.ThreadID == threadID && ids[record.SecurityContext.TurnID] {
				return errors.New("inherited history has an accepted final identity")
			}
		}
		return ctx.Err()
	})
	handler.store.SetActiveHistorySourceAdmissionV1(func(source map[string]any) error {
		if _, err := handler.publicProjector.ProjectThread(source); err != nil {
			return err
		}
		if _, present := source["securityState"]; present {
			threadID, _ := source["id"].(string)
			_, err := currentCaseAuthority.ValidateCurrent(threadID, source)
			return err
		}
		_, err := caseAuthority.SourceAdmissionDigestV1(source)
		return err
	})
	handler.threads = nil
}

func assertCaseIngressPublicSurfacesV1(
	t *testing.T,
	handler *runtimeServerHandler,
	durableRoot string,
	threadID string,
	rawAccount string,
	publicThread map[string]any,
	sse string,
	additionalForbidden ...string,
) {
	t.Helper()
	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	publicThreadBody, err := json.Marshal(publicThread)
	if err != nil {
		t.Fatal(err)
	}
	rawThreadBody, err := json.Marshal(rawThread)
	if err != nil {
		t.Fatal(err)
	}
	eventBody, err := json.Marshal(events.Events)
	if err != nil {
		t.Fatal(err)
	}
	surfaces := map[string]string{
		"GET thread":     string(publicThreadBody),
		"SSE replay":     sse,
		"durable thread": string(rawThreadBody),
		"durable events": string(eventBody),
		"durable files":  readCaseIngressPublicTreeV1(t, durableRoot),
	}
	for name, body := range surfaces {
		for _, forbidden := range append([]string{rawAccount}, additionalForbidden...) {
			if forbidden != "" && strings.Contains(body, forbidden) {
				t.Fatalf("%s leaked private case ingress/longitudinal owner state", name)
			}
		}
		if domaincaseentity.ContainsReferenceCandidateV1(body) ||
			strings.Contains(body, domaincaseentity.ReferencePrefixV1) {
			t.Fatalf("%s leaked an internal case entity reference", name)
		}
	}
}

func readCaseIngressPublicTreeV1(t *testing.T, root string) string {
	t.Helper()
	var body strings.Builder
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body.Write(data)
		body.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return body.String()
}

func caseIngressJSONV1(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

var _ datasetsnapshotport.AuthorityV2 = (*serverSignedCurrentDatasetV1)(nil)
var _ datasetsnapshotport.CurrentAuthorityV2 = (*serverSignedCurrentDatasetV1)(nil)
var _ datasetsnapshotport.CurrentSelectionCapabilityV2 = (*serverSignedCurrentDatasetCapabilityV1)(nil)
var _ caseentityport.Store = (*serverCaseIngressStoreV1)(nil)
