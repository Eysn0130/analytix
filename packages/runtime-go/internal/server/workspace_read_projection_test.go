package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	readapp "analytix.local/runtime-go/internal/app/workspaceread"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// This transport has no socket/client and never delegates to a real provider.
// The real Core turn-start and provider projection run before this boundary.
type workspaceReadCanaryProvider struct {
	requests [][]byte
	calls    int
}

func (*workspaceReadCanaryProvider) RequiresDurablePipelineStagesV1() {}

func (p *workspaceReadCanaryProvider) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	p.calls++
	if request.BeforeSend != nil {
		if err := request.BeforeSend(1); err != nil {
			return domainmodel.Result{}, err
		}
	}
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	body, err := json.Marshal(struct {
		SystemPrompt string
		Messages     []domainmodel.Message
		Tools        []domainmodel.ToolSchema
	}{request.SystemPrompt, request.Messages, request.Tools})
	if err != nil {
		return domainmodel.Result{}, err
	}
	p.requests = append(p.requests, body)
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "D1_ALLOWED_RETRIEVAL_CANARY reviewed"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true}, nil
}

func TestWorkspaceReadAllowedSourceReachesRealCoreProviderAndSurvivesReplay(t *testing.T) {
	testWorkspaceReadCoreProjection(t, false)
}

// A source-dependent prompt containing protected-local facts cannot prove an
// independent ordinary subrequest. Current Core policy refuses it before the
// provider; this does not claim mixed-source completion is available.
func TestWorkspaceReadProtectedSourceRefusesProviderAndSurvivesReplay(t *testing.T) {
	testWorkspaceReadCoreProjection(t, true)
}

func testWorkspaceReadCoreProjection(t *testing.T, protectedSource bool) {
	t.Helper()
	const allowed = "D1_ALLOWED_RETRIEVAL_CANARY"
	const privateEmail = "d1-protected-local@example.invalid"
	const privatePath = "/Users/synthetic/D1_PRIVATE_LOCATOR_CANARY/reference.md"
	const excluded = "D1_EXCLUDED_PROTECTED_ROOT_CANARY"
	workspace := workspacetest.New(t)
	durableRoot, dataDir := t.TempDir(), t.TempDir()
	protectedRoot := filepath.Join(workspace, "protected-reference")
	if err := os.Mkdir(protectedRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(protectedRoot, "private.txt"), []byte(excluded), 0600); err != nil {
		t.Fatal(err)
	}
	source := allowed
	if protectedSource {
		source += "\nContact: " + privateEmail + "\nSource: " + privatePath
	}
	if err := os.WriteFile(filepath.Join(workspace, "reference.md"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	config := RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: dataDir,
		ProviderID: "workspace-canary-provider", BaseURL: "https://provider.invalid", APIKey: "synthetic-test-placeholder",
		Model: "workspace-canary-model", EndpointFormat: "chat_completions",
	}
	authorityRoot := t.TempDir()
	configure := func(handler *runtimeServerHandler, reopening bool) {
		configureServerGeneralExecution(t, handler)
		risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
		if err != nil {
			t.Fatal(err)
		}
		handler.turnSecurity.RiskAuthority = risk
		signer, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "key", "authority.json"), reopening)
		if err != nil {
			t.Fatal(err)
		}
		records, err := newServerTestCaseThreadStore(t, filepath.Join(authorityRoot, "records"))
		if err != nil {
			t.Fatal(err)
		}
		registry, err := casethreadapp.NewRegistry(context.Background(), signer, records)
		if err != nil {
			t.Fatal(err)
		}
		handler.caseThreads = registry
		handler.store.SetCaseThreadAuthority(registry)
		if reopening {
			if err := casethreadapp.RepairCommittedContexts(registry, handler.store); err != nil {
				t.Fatal(err)
			}
			inventory, err := casethreadapp.PreflightRestartInventory(registry, handler.store)
			if err != nil {
				t.Fatal(err)
			}
			if err := casethreadapp.ApplyRestartInventory(registry, inventory); err != nil {
				t.Fatal(err)
			}
		}
		current := threadapp.NewCurrentCaseThreadAuthorityValidator(registry, filestore.CaseBindingReader{}, nil)
		finalIndex, casReader := configureWorkspaceReadFinalAuthority(t, handler, authorityRoot, durableRoot, reopening)
		handler.publicProjector = threadapp.NewTrustedPublicProjectorWithPrimaryCAS(finalIndex, registry, current, casReader)
		handler.threads = nil
	}
	handler := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	configure(handler, false)
	provider := &workspaceReadCanaryProvider{}
	handler.provider = provider
	server := httptest.NewServer(handler)
	defer server.Close()
	create := func(title string) string {
		thread := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{"title": title, "workspace": workspace,
				"providerId": config.ProviderID, "model": config.Model})), http.StatusCreated)
		return stringField(thread, "id")
	}
	ownerID := create("retrieval owner")
	service := &readapp.Service{Files: filestore.NewWorkspaceReadFiles([]string{protectedRoot})}
	if err := service.BindAuthority(runtimeWorkspaceReadAuthority{handler}); err != nil {
		t.Fatal(err)
	}
	authorized, err := service.Read(context.Background(), readapp.Request{Action: "authorize", ThreadID: ownerID})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Read(context.Background(), readapp.Request{Action: "scan", ThreadID: ownerID, Binding: authorized.Binding})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Files) != 1 || string(snapshot.Files[0].Content) != source {
		t.Fatal("real local read did not preserve the allowed file or excluded the wrong source")
	}
	// Exercise normal-turn projection and durable recovery independently of the
	// auxiliary completion route. Pass exact raw local file text; this test never
	// calls a projector or masks the prompt before Core receives it. Retain this
	// synthetic thread so recovery can inspect its persisted projection.
	temporaryID := create("temporary inline completion")
	started := requestThreadSummaryJSON(t, server.URL, http.MethodPost, "/v1/threads/"+temporaryID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "Continue the draft using this local reference:\n" + string(snapshot.Files[0].Content),
			"providerId": config.ProviderID, "model": config.Model, "mode": "agent", "approvalPolicy": "never",
			"sandboxMode": "read-only", "disableUserInput": true, "maxModelSteps": 1,
		})), http.StatusAccepted)

	if stringField(started, "turnId") == "" || stringField(started, "status") != "completed" {
		t.Fatal("Core did not complete the temporary turn")
	}
	wantCalls := 1
	if protectedSource {
		wantCalls = 0
	}
	if provider.calls != wantCalls || len(provider.requests) != wantCalls {
		t.Fatalf("provider boundary calls=%d requests=%d want=%d", provider.calls, len(provider.requests), wantCalls)
	}
	forbidden := []string{privateEmail, privatePath, "D1_PRIVATE_LOCATOR_CANARY", excluded}
	assertSafe := func(label string, body []byte) {
		t.Helper()
		for _, secret := range forbidden {
			if bytes.Contains(body, []byte(secret)) {
				t.Fatalf("%s retained protected source bytes", label)
			}
		}
	}
	if !protectedSource {
		assertSafe("provider request", provider.requests[0])
		if !bytes.Contains(provider.requests[0], []byte(allowed)) {
			t.Fatal("provider request lost the allowed source")
		}
	}
	assertPublicAndDurable := func(label string, current *runtimeServerHandler, baseURL string) {
		t.Helper()
		public := requestThreadSummaryJSON(t, baseURL, http.MethodGet, "/v1/threads/"+temporaryID, nil, http.StatusOK)
		body, err := json.Marshal(public)
		if err != nil {
			t.Fatal(err)
		}
		assertSafe(label+" public history", body)
		sse := requestRuntimeSSEText(t, baseURL+"/v1/threads/"+temporaryID+"/events?since_seq=0")
		assertSafe(label+" public SSE", []byte(sse))
		if !protectedSource && !strings.Contains(sse, allowed) {
			t.Fatal(label + " replay lost the completed allowed output")
		}
		raw, err := current.store.GetThread(temporaryID)
		if err != nil {
			t.Fatal(err)
		}
		body, err = json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		assertSafe(label+" ordinary durable history", body)
		if protectedSource {
			for _, marker := range []string{"[EMAIL]", "[PRIVATE_PATH]"} {
				if !bytes.Contains(body, []byte(marker)) {
					t.Fatalf("%s durable input lost production projection marker %q", label, marker)
				}
			}
		}
		// Includes the runtime's actual event JSONL and any created data/log files.
		// The synthetic input workspace is separate and deliberately not scanned.
		assertSafe(label+" durable event logs", []byte(readCaseIngressPublicTreeV1(t, durableRoot)))
		assertSafe(label+" runtime data files", []byte(readCaseIngressPublicTreeV1(t, dataDir)))
		assertSafe(label+" signed authority records", []byte(readCaseIngressPublicTreeV1(t, authorityRoot)))
	}
	assertPublicAndDurable("before restart", handler, server.URL)
	owner, err := handler.store.GetThread(ownerID)
	if err != nil || len(listAny(owner["turns"])) != 0 {
		t.Fatal("optional retrieval mutated owner history")
	}
	server.Close()
	restarted := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	configure(restarted, true)
	replayProvider := &workspaceReadCanaryProvider{}
	restarted.provider = replayProvider
	replayServer := httptest.NewServer(restarted)
	defer replayServer.Close()
	assertPublicAndDurable("after restart", restarted, replayServer.URL)
	if replayProvider.calls != 0 {
		t.Fatal("history recovery invoked the provider")
	}
}

// The rejection turn still publishes a signed boundary final. Reopen its actual
// terminal/telemetry/private-final stores, then seed the public index only from
// the production terminal coordinator's verified recovery result.
func configureWorkspaceReadFinalAuthority(t *testing.T, handler *runtimeServerHandler, root, durableRoot string, reopening bool) (*gateprojection.TrustedFinalProjectionIndex, *finalauthority.AcceptedFinalCASReader) {
	t.Helper()
	ctx := context.Background()
	signer, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(root, "final-key", "authority.json"), reopening)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := evidenceregistry.NewStore(filepath.Join(root, "evidence"), signer)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := newServerTestPrivateFinalStore(t, filepath.Join(root, "finals"))
	if err != nil {
		t.Fatal(err)
	}
	telemetryRoot := filepath.Join(root, "telemetry")
	telemetryAccess, err := privatecastest.NewAccessAuthority(telemetryRoot)
	if err != nil {
		t.Fatal(err)
	}
	telemetryStore, err := cachetelemetrystore.NewStore(telemetryRoot, telemetryAccess)
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := cachetelemetryapp.NewDurableService(signer, telemetryStore)
	if err != nil {
		t.Fatal(err)
	}
	terminalRoot := filepath.Join(root, "terminal")
	terminalAccess, err := privatecastest.NewAccessAuthority(terminalRoot)
	if err != nil {
		t.Fatal(err)
	}
	terminalStore, err := turnterminalstore.NewStore(terminalRoot, terminalAccess)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := turnterminalapp.NewCoordinator(signer, finals, terminalStore, telemetry)
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(handler.store, casReader)
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(signer, eventIO.Readback)
	if reopening {
		records, err := finals.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := coordinator.RecoverV1(ctx, turnterminalapp.RestartRecoveryInputV1{
			CompletionStore: handler.store, CASReader: casReader, PrivateInventory: records, Candidates: records,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(recovered.Complete) != len(records) || len(recovered.LegacyQuarantined) != 0 {
			t.Fatal("terminal recovery did not retain complete signed authority")
		}
		authorities := make([]gateprojection.TerminalCompleteFinalAuthorityV1, 0, len(records))
		for _, record := range records {
			found := false
			for _, complete := range recovered.Complete {
				if complete.Persistence.AcceptedFinal.RecordDigest != record.AcceptedFinal.RecordDigest {
					continue
				}
				if found {
					t.Fatal("duplicate terminal authority")
				}
				found = true
				authorities = append(authorities, gateprojection.TerminalCompleteFinalAuthorityV1{
					PrivateFinal: record, Intent: complete.Intent, ProviderClosure: complete.ProviderClosure,
					PublicObservation: complete.PublicObservation, AcceptedFinalDisposition: complete.AcceptedFinalDisposition,
					TerminalDisposition: complete.TerminalDisposition,
				})
			}
			if !found {
				t.Fatal("missing recovered terminal authority")
			}
		}
		if err := index.SeedTerminalComplete(ctx, authorities); err != nil {
			t.Fatal(err)
		}
	}
	handler.caseFinalizer = evidenceapp.NewCasePublicationFinalizerWithPublicationSnapshots(evidence, evidence, signer, finals, eventIO, coordinator, nil, index)
	return index, casReader
}
