package server

import (
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	controlapp "analytix.local/runtime-go/internal/app/control"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	providerapi "analytix.local/runtime-go/internal/provider"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func initialSkillCatalog(t *testing.T, body string, runAs ...string) toolcatalogapp.SkillCatalog {
	t.Helper()
	catalog := syntheticSpecializedSkillCatalogV1(t, body, runAs...)
	catalog.Skills[0]["scope"] = "global"
	return catalog
}

func TestInitialUserSkillAdmissionReachesProviderAndOwnItem(t *testing.T) {
	config := RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(), ProviderID: "skill-provider", BaseURL: "https://provider.invalid", APIKey: "synthetic-key", Model: "skill-model", EndpointFormat: "chat_completions"}
	h := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	installHandlerProviderExecutionResolverForTest(h)
	provider := &providerStepRecordingProvider{}
	h.provider = provider
	h.skills = initialSkillCatalog(t, "Review the supplied code carefully. INITIAL_SKILL_BODY_CANARY Contact 6222020202020202020")
	workspace := workspacetest.New(t)
	thread, err := h.store.CreateThread(map[string]any{"title": "Initial skill", "workspace": workspace, "providerId": "skill-provider", "model": "skill-model"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	response, err := h.startRuntimeTurn(context.Background(), id, startRuntimeTurnRequest{Prompt: "/synthetic-skill review this", RequestedSkillIDs: []string{"synthetic-skill"}})
	if err != nil {
		t.Fatal(err)
	}
	requests := provider.Requests()
	if len(requests) != 1 {
		t.Fatalf("want one actual provider request, got %d", len(requests))
	}
	found := false
	for _, message := range requests[0].Messages {
		if strings.Contains(message.Content, "INITIAL_SKILL_BODY_CANARY") {
			found = true
			if message.Role != "user" {
				t.Fatal("skill elevated above user")
			}
		}
	}
	requestBytes, _ := json.Marshal(requests[0].Messages)
	if strings.Contains(string(requestBytes), "6222020202020202020") {
		t.Fatal("skill bypassed final provider privacy projection")
	}
	if !found || strings.Contains(requests[0].SystemPrompt, "INITIAL_SKILL_BODY_CANARY") {
		t.Fatal("skill did not reach user-level provider context")
	}
	raw, err := h.store.GetThread(id)
	if err != nil {
		t.Fatal(err)
	}
	public, err := h.publicProjector.ProjectThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertInitialSkillItem(t, public, stringField(response, "userMessageItemId"), true)
	events, err := h.store.LoadPublicEventsSince(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	bytes, _ := json.Marshal(events)
	if strings.Contains(string(bytes), "INITIAL_SKILL_BODY_CANARY") {
		t.Fatal("private skill body escaped public events")
	}
	if !strings.Contains(string(bytes), `"activeSkillIds":["synthetic-skill"]`) {
		t.Fatal("initial item event lost successful skill")
	}
	// A subsequent prompt is not a new selection, even when it contains a gesture.
	second, err := h.startRuntimeTurn(context.Background(), id, startRuntimeTurnRequest{Prompt: "/synthetic-skill remains plain without explicit request"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = h.store.GetThread(id)
	if err != nil {
		t.Fatal(err)
	}
	public, err = h.publicProjector.ProjectThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertInitialSkillItem(t, public, stringField(second, "userMessageItemId"), false)
	requests = provider.Requests()
	for _, message := range requests[len(requests)-1].Messages {
		if strings.Contains(message.Content, "INITIAL_SKILL_BODY_CANARY") {
			t.Fatal("old snapshot replayed without selection")
		}
	}
	reopened := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	configureServerGeneralExecution(t, reopened)
	raw, err = reopened.store.GetThread(id)
	if err != nil {
		t.Fatal(err)
	}
	public, err = reopened.publicProjector.ProjectThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertInitialSkillItem(t, public, stringField(response, "userMessageItemId"), true)
	assertInitialSkillItem(t, public, stringField(second, "userMessageItemId"), false)
}

func assertInitialSkillItem(t *testing.T, thread map[string]any, itemID string, loaded bool) {
	t.Helper()
	for _, turnRaw := range listAny(thread["turns"]) {
		turn, _ := turnRaw.(map[string]any)
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if stringField(item, "id") != itemID {
				continue
			}
			ids := listAny(item["activeSkillIds"])
			if (len(ids) == 1 && ids[0] == "synthetic-skill") != loaded {
				t.Fatal("loaded fact crossed user items")
			}
			return
		}
	}
	t.Fatal("accepted user item missing")
}

func TestInitialUserSkillRejectionsAndCancellation(t *testing.T) {
	for _, scenario := range []string{"unknown", "disabled", "child", "budget", "stale", "scope"} {
		t.Run(scenario, func(t *testing.T) {
			h := &runtimeServerHandler{skills: initialSkillCatalog(t, "Safe instructions")}
			requested := []string{"synthetic-skill"}
			switch scenario {
			case "unknown":
				requested = []string{"missing"}
			case "disabled":
				h.skills.Enabled = false
			case "child":
				h.skills = initialSkillCatalog(t, "Safe child instructions", "subagent")
			case "budget":
				h.skills = initialSkillCatalog(t, strings.Repeat("x", initialUserSkillTokenLimit*4+1))
			case "stale":
				h.skills.Skills[0]["packageDigest"] = strings.Repeat("a", 64)
			case "scope":
				delete(h.skills.Skills[0], "scope")
			}
			prepared, err := h.prepareInitialUserSkills(context.Background(), requested)
			if err != nil || len(prepared.sources) != 0 {
				t.Fatal("rejected skill produced a source", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&runtimeServerHandler{}).prepareInitialUserSkills(ctx, []string{"synthetic-skill"}); err == nil {
		t.Fatal("cancelled preparation succeeded")
	}
}

func TestInitialUserSkillEpochBudgetAndReader(t *testing.T) {
	h := &runtimeServerHandler{skills: initialSkillCatalog(t, "Safe instructions")}
	prepared, err := h.prepareInitialUserSkills(context.Background(), []string{"synthetic-skill"})
	if err != nil || len(prepared.sources) != 1 {
		t.Fatal("snapshot missing", err)
	}
	prepared.sources[0].Sequence = 1
	state, err := contextepochapp.BootstrapState("thread", 1, prepared.sources, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	providerContext, err := contextepochapp.BuildProviderContext(context.Background(), state, prepared)
	if err != nil || len(providerContext.Dynamic) != 1 {
		t.Fatal("snapshot reader rejected", err)
	}
	entry := prepared.sources[0]
	entry.Digest = strings.Repeat("f", 64)
	if _, err := prepared.ReadContextSource(context.Background(), "thread", entry); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

type initialSkillAdmissionFailureResolver struct {
	beforeIntent func() error
	handlerProviderExecutionResolverForTest
	threadID string
	cancel   context.CancelFunc
}

func (r initialSkillAdmissionFailureResolver) ResolveTurnIntent(ctx context.Context, input providerapi.TurnExecutionInput) (providerapi.TurnExecutionResult, error) {
	if r.beforeIntent != nil {
		if err := r.beforeIntent(); err != nil {
			return providerapi.TurnExecutionResult{}, err
		}
	} else if r.cancel != nil {
		r.cancel()
	} else if _, err := r.handler.store.PatchThread(r.threadID, map[string]any{"title": "Changed admission baseline"}); err != nil {
		return providerapi.TurnExecutionResult{}, err
	}
	return r.handlerProviderExecutionResolverForTest.ResolveTurnIntent(ctx, input)
}
func TestInitialUserSkillAdmissionFailurePublishesNoLoadedItem(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "CAS", true: "cancel"}[cancelled], func(t *testing.T) {
			h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(), ProviderID: "skill-provider", BaseURL: "https://provider.invalid", APIKey: "synthetic-key", Model: "skill-model", EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
			configureServerGeneralExecution(t, h)
			h.skills = initialSkillCatalog(t, "Safe skill instructions")
			workspace := workspacetest.New(t)
			thread, err := h.store.CreateThread(map[string]any{"title": "Before", "workspace": workspace}, workspace)
			if err != nil {
				t.Fatal(err)
			}
			id := stringField(thread, "id")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := initialSkillAdmissionFailureResolver{handlerProviderExecutionResolverForTest: handlerProviderExecutionResolverForTest{handler: h}, threadID: id}
			if cancelled {
				resolver.cancel = cancel
			}
			h.providerExecution = resolver
			if _, err := h.startRuntimeTurn(ctx, id, startRuntimeTurnRequest{Prompt: "/synthetic-skill", RequestedSkillIDs: []string{"synthetic-skill"}}); err == nil {
				t.Fatal("failed admission succeeded")
			}
			raw, err := h.store.GetThread(id)
			if err != nil {
				t.Fatal(err)
			}
			bytes, _ := json.Marshal(raw)
			if strings.Contains(string(bytes), `"activeSkillIds":["synthetic-skill"]`) {
				t.Fatal("failed admission published loaded fact")
			}
			events, err := h.store.LoadPublicEventsSince(id, 0)
			if err != nil {
				t.Fatal(err)
			}
			bytes, _ = json.Marshal(events)
			if strings.Contains(string(bytes), `"activeSkillIds":["synthetic-skill"]`) {
				t.Fatal("failed admission published loaded event")
			}
		})
	}
}

func TestInitialUserHostedSkillUsesExistingActivationBoundary(t *testing.T) {
	h, view, _ := officeRuntimeFixtureWithInstalledSkill(t, "docx")
	prepared, err := h.prepareInitialUserSkills(context.Background(), []string{view.PackageID})
	if err != nil || len(prepared.sources) != 1 {
		t.Fatal("enabled hosted snapshot not loaded", err)
	}
	_, err = h.officePackageHost.SetDesiredState(context.Background(), hostapp.SetDesiredStateRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, DesiredState: domainplugin.DesiredDisabledV1})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err = h.prepareInitialUserSkills(context.Background(), []string{view.PackageID})
	if err != nil || len(prepared.sources) != 0 {
		t.Fatal("disabled hosted snapshot loaded", err)
	}
}

func TestInitialUserHostedSkillChangedBeforeAppendHasNoFact(t *testing.T) {
	fixture, view, _ := officeRuntimeFixtureWithInstalledSkill(t, "docx")
	h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(), ProviderID: "skill-provider", BaseURL: "https://provider.invalid", APIKey: "synthetic-key", Model: "skill-model", EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	h.officePackageHost = fixture.officePackageHost
	recording := &providerStepRecordingProvider{}
	h.provider = recording
	workspace := workspacetest.New(t)
	thread, err := h.store.CreateThread(map[string]any{"title": "Hosted initial", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	h.providerExecution = initialSkillAdmissionFailureResolver{handlerProviderExecutionResolverForTest: handlerProviderExecutionResolverForTest{handler: h}, beforeIntent: func() error {
		_, err := h.officePackageHost.SetDesiredState(context.Background(), hostapp.SetDesiredStateRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, DesiredState: domainplugin.DesiredDisabledV1})
		return err
	}}
	if _, err := h.startRuntimeTurn(context.Background(), id, startRuntimeTurnRequest{Prompt: "Use selected skill", RequestedSkillIDs: []string{view.PackageID}}); err == nil {
		t.Fatal("changed hosted snapshot admitted")
	}
	raw, err := h.store.GetThread(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(listAny(raw["turns"])) != 0 || len(recording.Requests()) != 0 {
		t.Fatal("changed hosted preparation published or reached provider")
	}
}

func TestInitialUserSkillPendingContinuationAndRecovery(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-process", true: "restart-reconciliation"}[restart], func(t *testing.T) {
			const canary = "INITIAL_SKILL_PENDING_BODY_CANARY"
			var calls atomic.Int32
			bodies := make(chan []byte, 3)
			f := newInlineCompletionFixture(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodies <- body
				if calls.Add(1) == 1 {
					writeSyntheticQuestionCallV1(w, "request_user_input", `{"prompt":"Choose a synthetic answer","questions":[{"question":"Continue?"}]}`)
					return
				}
				writeInlineCompletionFixtureReply(w)
			})
			h := f.handler
			h.skills = initialSkillCatalog(t, canary)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			t.Cleanup(func() {
				if err := h.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			started, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "/synthetic-skill continue", RequestedSkillIDs: []string{"synthetic-skill"}})
			if err != nil || stringField(started, "status") != "waiting" || stringField(started, "pendingKind") != "user_input" {
				t.Fatal("initial skill did not reach an actual pending gate", err)
			}
			<-bodies
			if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
				t.Fatal(err)
			}
			pendingID := stringField(started, "pendingId")
			_, pending, _, pendingFound := h.gates.PeekUserInput(pendingID)
			pendingBytes, _ := json.Marshal(pending.Messages)
			if !pendingFound || !strings.Contains(string(pendingBytes), canary) {
				t.Fatal("pending snapshot lost admitted skill instructions")
			}
			thread, err := h.store.GetThread(f.request.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			before, _, err := contextepochapp.StateFromThread(thread)
			if err != nil {
				t.Fatal(err)
			}
			if restart {
				// Existing restart seam discards process-local pending ownership;
				// durable Epoch and gate reconciliation must not resume old work.
				h.gates = controlapp.NewGateRegistry[runtimePendingToolCall]()
				h.gate = controlapp.NewApprovalUserInputManager()
				if err := h.restoreRuntimeState(); err != nil {
					t.Fatal(err)
				}
				thread, err = h.store.GetThread(f.request.ThreadID)
				if err != nil {
					t.Fatal(err)
				}
				recovered, _, err := contextepochapp.StateFromThread(thread)
				if err != nil || recovered.AcceptedSnapshot.Epoch <= before.AcceptedSnapshot.Epoch {
					t.Fatal("missing memory source did not advance recovered epoch", err)
				}
				unavailable := false
				for _, source := range recovered.Registry {
					if source.Kind == contextepochapp.InitialUserSkillKind && source.TrustState == "unavailable" {
						unavailable = true
					}
				}
				if !unavailable {
					t.Fatal("restart silently reused the skill body")
				}
				public, err := h.publicProjector.ProjectThread(thread)
				if err != nil {
					t.Fatal(err)
				}
				assertInitialSkillItem(t, public, stringField(started, "userMessageItemId"), true)
			}
			result, err := h.runtimeControl().RespondUserInput(ctx, controlapp.UserInputResponse{InputID: pendingID, Answers: []map[string]string{{"id": "q1", "label": "Choice", "value": "continue"}}})
			if err != nil {
				t.Fatal(err)
			}
			if restart {
				if result.StatusCode != http.StatusConflict || calls.Load() != 1 {
					t.Fatal("restarted pending work reached Provider")
				}
				if code := stringField(result.Body, "code"); code != "gate_continuation_unavailable" && code != "gate_continuation_terminal_turn" {
					t.Fatalf("unexpected closed-gate code: %s", code)
				}
				// A new initial turn uses the recovered accepted Epoch and does
				// not recover old selection from its visible historical gesture.
				if _, err := h.startRuntimeTurn(ctx, f.request.ThreadID, startRuntimeTurnRequest{Prompt: "/synthetic-skill remains unselected"}); err != nil {
					t.Fatal("new initial rejected recovered Epoch", err)
				}
			} else if result.StatusCode != http.StatusOK {
				t.Fatal("same-process continuation rejected")
			}
			select {
			case body := <-bodies:
				found := false
				for _, message := range timedQuestionProviderMessages(t, body) {
					if strings.Contains(stringField(message, "content"), canary) {
						found = true
						if stringField(message, "role") != "user" {
							t.Fatal("pending skill priority changed")
						}
					}
				}
				if found == restart {
					t.Fatal("pending/restart skill body did not follow admitted ownership")
				}
			case <-ctx.Done():
				t.Fatal("expected production Provider step missing")
			}
			if err := h.runtimeControl().WaitForTurnOperations(ctx); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 2 {
				t.Fatal("unexpected production Provider call count")
			}
			if !restart {
				thread, err := h.store.GetThread(f.request.ThreadID)
				if err != nil {
					t.Fatal(err)
				}
				after, _, err := contextepochapp.StateFromThread(thread)
				if err != nil || after.AcceptedSnapshot.Epoch != before.AcceptedSnapshot.Epoch {
					t.Fatal("same-process continuation changed Epoch", err)
				}
			}
		})
	}
}
