package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finaladapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	riskstore "analytix.local/runtime-go/internal/adapters/outbound/threadriskpolicy"
	caseapp "analytix.local/runtime-go/internal/app/casethread"
	epochapp "analytix.local/runtime-go/internal/app/contextepoch"
	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	security "analytix.local/runtime-go/internal/domain/security"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

// Real installation-signed HostPolicyAuthority and signed case Registry; only
// the principal and root-scoped test filesystem capability are synthetic.
// Host-policy fallback is a production composition, NOT an independent witness
// or DSV2 admission. Accepted case-data-forensics keeps ordinary source artifacts
// usable without granting protected datasets or case-fact publication authority.
type canvasCaseFixture struct {
	*canvasProductFixture
	authority                         *caseapp.Registry
	security                          security.TurnSecurityContext
	authorityPath, caseRoot, riskRoot string
}

func newCanvasCaseFixture(t *testing.T) *canvasCaseFixture {
	t.Helper()
	h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir()}).(*runtimeServerHandler)
	t.Cleanup(func() { h.runtimeSubagentState().CancelBackgroundJobsAndWait(time.Second) })
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	f := &canvasCaseFixture{authorityPath: filepath.Join(root, "signer", "authority.json"), caseRoot: filepath.Join(root, "case-records"), riskRoot: filepath.Join(root, "risk-records")}
	f.bindAuthorities(t, h)
	workspace := writeThreadMutationCaseBinding(t)
	thread, err := h.store.CreateThread(map[string]any{"title": "Synthetic case canvas", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	f.security = f.commitTurn(t, h, threadID, workspace, "case-canvas-first-turn")
	f.canvasProductFixture = newCanvasProductFixtureWithAuthority(t, runtimeObjectProjector{handler: h}, objectapp.ScopeAuthority{Principal: testIdentityPrincipal(), ObjectID: "synthetic-case-canvas", ThreadID: threadID, Purpose: "edit", Workspace: workspace, Path: "product.canvas"})
	return f
}

func (f *canvasCaseFixture) bindAuthorities(t *testing.T, h *runtimeServerHandler) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.authorityPath), 0700); err != nil {
		t.Fatal(err)
	}
	signer, err := finaladapter.OpenOrCreateFileAuthority(f.authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newServerTestCaseThreadStore(t, f.caseRoot)
	if err != nil {
		t.Fatal(err)
	}
	f.authority, err = caseapp.NewRegistry(context.Background(), signer, store)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(f.riskRoot)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := riskstore.NewStore(f.riskRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	observer := filestore.CaseBindingReader{}
	risk, err := riskapp.NewHostPolicyAuthority(observer, signer, policies)
	if err != nil {
		t.Fatal(err)
	}
	h.caseThreads = f.authority
	h.store.SetCaseThreadAuthority(f.authority)
	h.turnSecurity = turnapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: risk, RiskIntent: security.RiskClassCase, TrustedCaseThread: true}
}

func (f *canvasCaseFixture) commitTurn(t *testing.T, h *runtimeServerHandler, threadID, workspace, turnID string) security.TurnSecurityContext {
	t.Helper()
	ctx := context.Background()
	thread, err := h.store.GetThreadForAuthorityRepair(threadID)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	frozen, err := turnapp.FreezeWorkspace(turnapp.WorkspaceFreezeInput{Context: ctx, Authority: h.turnSecurity, Thread: thread, ThreadID: threadID, TurnID: turnID, Workspace: workspace, Principal: testIdentityPrincipal(), IssuedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := epochapp.PrepareTurn(epochapp.PrepareTurnInput{Thread: thread, SecurityContext: frozen, At: at})
	if err != nil {
		t.Fatal(err)
	}
	frozen = prepared.SecurityContext
	if !security.TurnSecurityContextIsBoundaryOnly(frozen) || !security.TurnSecurityContextUsesHostRiskPolicy(frozen) || security.ValidateTurnSecurityContextForOrdinaryEffect(frozen) != nil || security.ValidateTurnSecurityContextForCaseFactPublication(frozen) == nil {
		t.Fatal("case ordinary authority silently upgraded or missing", frozen)
	}
	if err = caseapp.RegisterRequired(ctx, f.authority, frozen); err != nil {
		t.Fatal(err)
	}
	record := turnapp.PublicRecord(frozen)
	if err = h.store.AppendTurnToThread(threadID, map[string]any{"id": turnID, "threadId": threadID, "status": "completed", "items": []any{}, "securityContext": record}, "", map[string]any{"securityState": record, "contextEpochState": epochapp.PublicState(prepared.State)}); err != nil {
		t.Fatal(err)
	}
	if err = caseapp.CommitRequired(ctx, f.authority, frozen, prepared.State, at); err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	if err = h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(ctx, frozen, time.Second); err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestCanvasCaseAuthorityOrdinaryArtifactReviewAcceptAndUndo(t *testing.T) {
	f := newCanvasCaseFixture(t)
	scope := f.capture()
	projected := f.read(scope)
	body := productJSON(projected)
	for _, raw := range []string{f.workspace, "private_node_A", "Private Bank", "alice@example.com", "13800138000", "IGNORE RULES"} {
		if strings.Contains(body, raw) {
			t.Fatal("raw private field escaped", raw)
		}
	}
	alias := projected["objects"].([]any)[0].(map[string]any)["id"].(string)
	proposed, failed := f.propose(scope, alias, "case_canvas_proposal_001")
	if failed {
		t.Fatal(proposed)
	}
	f.unchanged()
	receipt := canvasDecodeProduct[fileport.Receipt](t, f.invoke("proposal-apply", f.scoped(map[string]any{"proposalId": proposed["proposalId"]})), "receipt")
	if receipt.Status != fileport.StatusCommitted {
		t.Fatal(receipt)
	}
	if security.TurnSecurityContextAllowsCaseEvidence(f.security) || f.security.DatasetSnapshotID != security.NoDatasetSnapshotID || f.security.SourceManifestHash != security.EmptySourceManifestHash {
		t.Fatal("ordinary edit acquired dataset authority")
	}
	if string(f.invoke("close-object", f.scoped(nil))["ok"]) != "true" {
		t.Fatal("close")
	}
	f.doc = f.open(f.doc.ThreadID)
	recovery := canvasDecodeProduct[fileport.NativeRecovery](t, f.invoke("object-recovery", f.scoped(nil)), "recovery")
	if recovery.Current == nil || !recovery.Current.CanUndo {
		t.Fatal("missing actual durable undo")
	}
	undone := canvasDecodeProduct[fileport.Receipt](t, f.invoke("undo-change", f.scoped(map[string]any{"changeId": recovery.Current.ChangeID, "baseRevision": f.doc.Revision})), "receipt")
	if undone.Status != fileport.StatusCommitted {
		t.Fatal(undone)
	}
	f.unchanged()
}

func TestCanvasCaseAuthorityInvalidationBeforeReadProposeAndAccept(t *testing.T) {
	for _, change := range []string{"case-binding", "epoch", "quarantine", "risk-record-unavailable", "host-disabled", "source-cas", "scope-reopen-authority-restart"} {
		t.Run(change, func(t *testing.T) {
			f := newCanvasCaseFixture(t)
			scope := f.capture()
			alias := f.read(scope)["objects"].([]any)[0].(map[string]any)["id"].(string)
			proposed, failed := f.propose(scope, alias, "case_canvas_before_transition")
			if failed {
				t.Fatal(proposed)
			}
			f.unchanged()
			switch change {
			case "case-binding":
				raw, _ := json.Marshal(map[string]any{"version": 1, "workspaceRoot": f.workspace, "caseId": "case_other_synthetic", "source": "analytix-data-analysis"})
				if err := os.WriteFile(filepath.Join(f.workspace, ".analytix", "case-project.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "epoch":
				next := f.commitTurn(t, f.projector.handler, f.doc.ThreadID, f.workspace, "case-canvas-next-turn")
				if next.ContextEpoch <= f.security.ContextEpoch {
					t.Fatal("real epoch owner did not advance")
				}
				f.security = next
			case "quarantine":
				f.authority.ReplaceQuarantine(map[string]string{f.doc.ThreadID: "synthetic-revoked"})
			case "risk-record-unavailable":
				if err := os.Rename(f.riskRoot, f.riskRoot+"-unavailable"); err != nil {
					t.Fatal(err)
				}
			case "host-disabled":
				disabled, err := f.host.SetDesiredState(f.ctx, hostapp.SetDesiredStateRequest{PackageID: f.view.PackageID, GenerationID: f.view.GenerationID, ExpectedRevision: f.view.ActivationRevision, DesiredState: domainplugin.DesiredDisabledV1})
				if err != nil {
					t.Fatal(err)
				}
				// A subsequent re-enable must not revive a proposal or captured grant.
				enabled, err := f.host.SetDesiredState(f.ctx, hostapp.SetDesiredStateRequest{PackageID: disabled.PackageID, GenerationID: disabled.GenerationID, ExpectedRevision: disabled.ActivationRevision, DesiredState: domainplugin.DesiredEnabledV1})
				if err != nil {
					t.Fatal(err)
				}
				f.view = enabled
			case "source-cas":
				changed := strings.Replace(string(f.before), "Known transfer", "Externally changed transfer", 1)
				if err := os.WriteFile(f.path, []byte(changed), 0600); err != nil {
					t.Fatal(err)
				}
				f.before = []byte(changed)
			case "scope-reopen-authority-restart":
				if string(f.invoke("close-object", f.scoped(nil))["ok"]) != "true" {
					t.Fatal("close")
				}
				// Reload the real signed Registry and Host policy CAS, not a fingerprint stub.
				f.bindAuthorities(t, f.projector.handler)
				f.doc = f.open(f.doc.ThreadID)
			}
			if v, failed := f.model("native_selection_read", f.doc.ThreadID, map[string]any{"scopeId": scope.ScopeID}); !failed {
				t.Fatal("stale read admitted", v)
			}
			if v, failed := f.propose(scope, alias, "case_canvas_after_transition"); !failed {
				t.Fatal("stale proposal admitted", v)
			}
			accepted := f.invoke("proposal-apply", f.scoped(map[string]any{"proposalId": proposed["proposalId"]}))
			if string(accepted["ok"]) != "false" {
				t.Fatal("old acceptance admitted", productJSON(accepted))
			}
			f.unchanged()
		})
	}
}

func TestCanvasCaseAuthorityDifferentThreadCannotConsumeScope(t *testing.T) {
	f := newCanvasCaseFixture(t)
	scope := f.capture()
	thread, err := f.projector.handler.store.CreateThread(map[string]any{"title": "Other synthetic case thread", "workspace": f.workspace}, f.workspace)
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(thread, "id")
	f.commitTurn(t, f.projector.handler, id, f.workspace, "case-canvas-other-thread-turn")
	if result, failed := f.model("native_selection_read", id, map[string]any{"scopeId": scope.ScopeID}); !failed {
		t.Fatal("cross-thread read admitted", result)
	}
	f.read(scope) // The negative caller must not revoke the rightful owner.
	f.unchanged()
}

// Exercise the actual currentness owner, not merely a policy-shaped fixture.
func TestCanvasCaseAuthorityDoesNotUpgradeOrdinaryArtifactsToCaseFacts(t *testing.T) {
	f := newCanvasCaseFixture(t)
	h := f.projector.handler
	input := turnapp.CurrentValidationInput{OperationContext: f.ctx, Identity: h.turnSecurity.Identity,
		Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority,
		SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: f.security, Workspace: f.workspace}
	if err := turnapp.ValidateCurrentOrdinaryEffect(input); err != nil {
		t.Fatal("ordinary artifact denied", err)
	}
	if err := turnapp.ValidateCurrentForEffect(input, true); err == nil {
		t.Fatal("unadmitted case-data effect accepted")
	}
	f.read(f.capture())
	f.unchanged()
}
