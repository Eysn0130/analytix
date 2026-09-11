//go:build darwin && analytix_prod

package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	_ "unsafe"

	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appidentity "analytix.local/runtime-go/internal/app/identity"
	appmodel "analytix.local/runtime-go/internal/app/model"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const rev9RetainedRuntimeServer = "/Volumes/AnalytixCache/development-v3/tmp/ab-r3-rev7-final.NSutaS/dist/mac-arm64/analytix.app/Contents/Resources/runtime-go/bin/runtime-server"

// openRev9DarwinHostCandidateForExecutable is test-only access to the exact
// production owner opener. It avoids changing the public host contract merely
// to let a Go test bind a retained packaged runtime-server executable.
//
//go:linkname openRev9DarwinHostCandidateForExecutable analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost.openDarwinHostCandidateForExecutable
func openRev9DarwinHostCandidateForExecutable(string, string) (*nativecomponenthost.Owner, error)

func TestNativeHealthRetainedPackageProductionComposition(t *testing.T) {
	counts := &rev9HealthCounts{}
	runtimeServer := strings.TrimSpace(os.Getenv("ANALYTIX_AB_R3_PACKAGE_RUNTIME_SERVER"))
	if runtimeServer != rev9RetainedRuntimeServer {
		rev9Fail(t, "owner_open", counts)
	}
	canonicalRuntimeServer, err := filepath.EvalSymlinks(runtimeServer)
	if err != nil || canonicalRuntimeServer != runtimeServer {
		rev9Fail(t, "owner_open", counts)
	}
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil || os.Chmod(profileRoot, 0o700) != nil {
		rev9Fail(t, "owner_open", counts)
	}
	profileInfo, err := os.Stat(profileRoot)
	if err != nil || profileInfo.Mode().Perm() != 0o700 {
		rev9Fail(t, "owner_open", counts)
	}
	owner, err := openRev9DarwinHostCandidateForExecutable(profileRoot, canonicalRuntimeServer)
	if err != nil || owner == nil {
		rev9Fail(t, "owner_open", counts)
	}
	counts.ownerOpen.Store(1)
	closed := false
	t.Cleanup(func() {
		if !closed && owner.Close() != nil {
			t.Errorf("health_durable_settlement_readback owner_open=%d component_currentness=%d ordinary_currentness=%d health_dependency_composition=%d health_pre_admission=%d health_runner_execute=%d health_post_admission=%d health_durable_settlement_readback=%d", counts.ownerOpen.Load(), counts.componentCurrentness.Load(), counts.ordinaryCurrentness.Load(), counts.healthDependencyComposition.Load(), counts.healthPreAdmission.Load(), counts.healthRunnerExecute.Load(), counts.healthPostAdmission.Load(), counts.healthDurableSettlementReadback.Load())
		}
	})

	if owner.ValidateCurrentDataEngine(context.Background()) != nil {
		rev9Fail(t, "component_currentness", counts)
	}
	counts.componentCurrentness.Add(1)

	now := time.Now().UTC()
	securityContext, observation, err := rev9HealthSecurityContext(now, profileRoot)
	if err != nil {
		rev9Fail(t, "ordinary_currentness", counts)
	}
	identityAuthority, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("ab-r3-rev9-isolated-installation")),
	)
	if err != nil {
		rev9Fail(t, "ordinary_currentness", counts)
	}
	riskAuthority := &rev9RiskAuthority{expected: securityContext}
	ordinaryInput := turnsecurityapp.CurrentValidationInput{
		OperationContext: context.Background(),
		Identity:         identityAuthority,
		Observer:         rev9BindingObserver{expectedWorkspace: securityContext.WorkspaceRealPath, observation: observation},
		RiskAuthority:    riskAuthority,
		Context:          securityContext,
		Workspace:        securityContext.WorkspaceRealPath,
	}
	if turnsecurityapp.ValidateCurrentOrdinaryEffect(ordinaryInput) != nil {
		rev9Fail(t, "ordinary_currentness", counts)
	}
	counts.ordinaryCurrentness.Add(1)

	store := newRev9HealthStore(securityContext)
	effectGate := effectgateapp.New()
	ordinaryHealth := nativecomponentapp.LiveAuthorityFunc(func(
		operationContext context.Context,
		current domainsecurity.TurnSecurityContext,
	) error {
		counts.ordinaryCurrentness.Add(1)
		input := ordinaryInput
		input.OperationContext = operationContext
		input.Context = current
		input.Workspace = current.WorkspaceRealPath
		return turnsecurityapp.ValidateCurrentOrdinaryEffect(input)
	})
	componentHealth := func(operationContext context.Context) error {
		counts.componentCurrentness.Add(1)
		return owner.ValidateCurrentDataEngine(operationContext)
	}
	healthOnly := nativecomponentapp.NewHealthOnlyCurrentnessV1(ordinaryHealth, componentHealth)
	if healthOnly == nil {
		rev9Fail(t, "health_dependency_composition", counts)
	}
	counts.healthDependencyComposition.Store(1)

	healthDependencies := nativecomponentapp.HealthDependencies{
		Store: store, DurableAuthority: rev9DurableAuthority{expected: securityContext},
		AcquireEffect: effectGate.AcquireEffect, Now: func() time.Time { return now },
	}
	generalLive := nativecomponentapp.LiveAuthorityFunc(func(
		context.Context,
		domainsecurity.TurnSecurityContext,
	) error {
		counts.generalLive.Add(1)
		return nativecomponentapp.ErrAuthorityInvalid
	})
	runner := &rev9HealthRunner{
		owner: owner, ordinary: &counts.ordinaryCurrentness,
		component: &counts.componentCurrentness, calls: &counts.healthRunnerExecute,
		preAdmission: &counts.healthPreAdmission,
	}

	var typedNil *rev9TypedNilCurrentness
	for _, invalidHealthOnly := range []nativecomponentapp.LiveAuthority{nil, typedNil} {
		if candidate, candidateErr := nativecomponentapp.NewReadyRuntimeAuthority(
			nativecomponentapp.RuntimeAuthorityDependencies{
				Health: healthDependencies, LiveAuthority: generalLive,
				HealthOnlyAuthority: invalidHealthOnly, Owner: runner,
			},
		); candidateErr == nil || candidate != nil || runner.calls.Load() != 0 {
			rev9Fail(t, "health_dependency_composition", counts)
		}
	}

	authority, err := nativecomponentapp.NewReadyRuntimeAuthority(
		nativecomponentapp.RuntimeAuthorityDependencies{
			Health: healthDependencies, LiveAuthority: generalLive,
			HealthOnlyAuthority: healthOnly, Owner: runner,
		},
	)
	if err != nil || authority == nil || !authority.Available() || authority.AccountFlowsAvailable() {
		rev9Fail(t, "health_dependency_composition", counts)
	}
	if _, flowErr := authority.AnalyzeAccountFlows(
		context.Background(),
		nativecomponentapp.AnalyzeAccountFlowsInput{},
		func(domainnative.AccountFlowHostEvidenceProjectionV1) error {
			counts.evidenceConsumer.Add(1)
			return nil
		},
	); !errors.Is(flowErr, nativecomponentapp.ErrUnavailable) ||
		counts.generalLive.Load() != 0 || counts.healthRunnerExecute.Load() != 0 ||
		counts.evidenceConsumer.Load() != 0 {
		rev9Fail(t, "health_dependency_composition", counts)
	}

	if authority.PrepareCaseTurn(context.Background(), securityContext) != nil {
		phase := "health_pre_admission"
		if counts.healthRunnerExecute.Load() > 0 {
			phase = "health_post_admission"
		}
		rev9Fail(t, phase, counts)
	}
	if counts.healthPreAdmission.Load() != 1 || counts.healthRunnerExecute.Load() != 1 {
		rev9Fail(t, "health_runner_execute", counts)
	}
	if counts.ordinaryCurrentness.Load() != 4 || counts.componentCurrentness.Load() != 4 ||
		counts.generalLive.Load() != 0 {
		rev9Fail(t, "health_post_admission", counts)
	}
	counts.healthPostAdmission.Store(1)

	if authority.PrepareCaseTurn(context.Background(), securityContext) != nil ||
		counts.healthRunnerExecute.Load() != 1 || counts.ordinaryCurrentness.Load() != 4 ||
		counts.componentCurrentness.Load() != 4 || !store.settledReady(securityContext) {
		rev9Fail(t, "health_durable_settlement_readback", counts)
	}
	counts.healthDurableSettlementReadback.Store(1)
	if authority.Close() != nil {
		rev9Fail(t, "health_durable_settlement_readback", counts)
	}
	closed = true

	t.Logf("health_durable_settlement_readback owner_open=%d component_currentness=%d ordinary_currentness=%d health_dependency_composition=%d health_pre_admission=%d health_runner_execute=%d health_post_admission=%d health_durable_settlement_readback=%d general_live=%d evidence_consumer=%d", counts.ownerOpen.Load(), counts.componentCurrentness.Load(), counts.ordinaryCurrentness.Load(), counts.healthDependencyComposition.Load(), counts.healthPreAdmission.Load(), counts.healthRunnerExecute.Load(), counts.healthPostAdmission.Load(), counts.healthDurableSettlementReadback.Load(), counts.generalLive.Load(), counts.evidenceConsumer.Load())
}

type rev9HealthCounts struct {
	ownerOpen                       atomic.Int32
	componentCurrentness            atomic.Int32
	ordinaryCurrentness             atomic.Int32
	healthDependencyComposition     atomic.Int32
	healthPreAdmission              atomic.Int32
	healthRunnerExecute             atomic.Int32
	healthPostAdmission             atomic.Int32
	healthDurableSettlementReadback atomic.Int32
	generalLive                     atomic.Int32
	evidenceConsumer                atomic.Int32
}

func rev9Fail(t *testing.T, phase string, counts *rev9HealthCounts) {
	t.Helper()
	t.Fatalf("%s owner_open=%d component_currentness=%d ordinary_currentness=%d health_dependency_composition=%d health_pre_admission=%d health_runner_execute=%d health_post_admission=%d health_durable_settlement_readback=%d general_live=%d evidence_consumer=%d", phase, counts.ownerOpen.Load(), counts.componentCurrentness.Load(), counts.ordinaryCurrentness.Load(), counts.healthDependencyComposition.Load(), counts.healthPreAdmission.Load(), counts.healthRunnerExecute.Load(), counts.healthPostAdmission.Load(), counts.healthDurableSettlementReadback.Load(), counts.generalLive.Load(), counts.evidenceConsumer.Load())
}

func rev9HealthSecurityContext(
	now time.Time,
	profileRoot string,
) (domainsecurity.TurnSecurityContext, domainsecurity.CaseBindingObservationV1, error) {
	const threadID = "thread-ab-r3-rev9-health"
	const turnID = "turn-ab-r3-rev9-health"
	const caseID = "case-ab-r3-rev9-health"
	workspace := filepath.Join(profileRoot, "isolated-case")
	bindingHash := domainsecurity.SHA256Hex([]byte("ab-r3-rev9-case-binding"))
	observation, err := domainsecurity.NewCaseBindingObservationV1(
		domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: workspace,
			State:             domainsecurity.CaseBindingStateValid,
			CaseID:            caseID,
			BindingSHA256:     domainsecurity.SHA256Hex([]byte("ab-r3-rev9-binding-body")),
			CaseBindingHash:   bindingHash,
		},
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, domainsecurity.CaseBindingObservationV1{}, err
	}
	policyDigest := domainsecurity.SHA256Hex([]byte("ab-r3-rev9-risk-policy"))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(
		domainsecurity.TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest:   policyDigest,
			RiskClass:                domainsecurity.RiskClassCase,
			Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
			CaseBindingState:         domainsecurity.CaseBindingStateValid,
			BindingObservationDigest: observation.ObservationDigest,
			BlockerCode:              domainsecurity.PublicationBlockerNone,
		},
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, domainsecurity.CaseBindingObservationV1{}, err
	}
	riskBinding, err := securitytest.WitnessedRiskBinding(
		threadID,
		workspace,
		domainsecurity.RiskClassCase,
		policyDigest,
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, domainsecurity.CaseBindingObservationV1{}, err
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(
		domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			CaseID: caseID, CaseBindingHash: bindingHash,
			DatasetSnapshotID:  securitytest.DatasetSnapshotID("ab-r3-rev9-health"),
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("ab-r3-rev9-source-manifest")),
			ContextEpoch:       1, IssuedAt: now,
			PublicationPolicy: publication, RiskAuthorityBinding: riskBinding,
		},
	)
	return securityContext, observation, err
}

type rev9BindingObserver struct {
	expectedWorkspace string
	observation       domainsecurity.CaseBindingObservationV1
}

func (observer rev9BindingObserver) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if workspace != observer.expectedWorkspace {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("rev9 binding mismatch")
	}
	return observer.observation, nil
}

type rev9RiskAuthority struct {
	expected domainsecurity.TurnSecurityContext
}

func (*rev9RiskAuthority) ResolveOrRaise(
	context.Context,
	threadriskauthorityapp.ResolveOrRaiseInput,
) (threadriskauthorityapp.Head, error) {
	return threadriskauthorityapp.Head{}, errors.New("rev9 risk issuance unavailable")
}

func (authority *rev9RiskAuthority) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if ctx == nil || ctx.Err() != nil || authority == nil || securityContext != authority.expected {
		return errors.New("rev9 risk currentness mismatch")
	}
	return nil
}

type rev9TypedNilCurrentness struct{}

func (*rev9TypedNilCurrentness) ValidateCurrent(
	context.Context,
	domainsecurity.TurnSecurityContext,
) error {
	return nativecomponentapp.ErrAuthorityInvalid
}

type rev9HealthRunner struct {
	owner        *nativecomponenthost.Owner
	ordinary     *atomic.Int32
	component    *atomic.Int32
	calls        *atomic.Int32
	preAdmission *atomic.Int32
}

func (runner *rev9HealthRunner) Execute(
	ctx context.Context,
	request domainnative.Request,
) (domainnative.Result, error) {
	if runner == nil || runner.owner == nil || runner.calls.Add(1) != 1 ||
		runner.ordinary.Load() != 3 || runner.component.Load() != 3 {
		return domainnative.Result{}, nativecomponentapp.ErrAuthorityInvalid
	}
	runner.preAdmission.Store(1)
	return runner.owner.Execute(ctx, request)
}

func (runner *rev9HealthRunner) Close() error {
	if runner == nil || runner.owner == nil {
		return nil
	}
	return runner.owner.Close()
}

type rev9DurableAuthority struct {
	expected domainsecurity.TurnSecurityContext
}

func (authority rev9DurableAuthority) ValidateCurrent(
	threadID string,
	thread map[string]any,
) (domainsecurity.TurnSecurityContext, error) {
	securityContext, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || threadID != authority.expected.ThreadID || securityContext != authority.expected {
		return domainsecurity.TurnSecurityContext{}, errors.New("rev9 durable context mismatch")
	}
	return securityContext, nil
}

type rev9HealthStore struct {
	mu      sync.Mutex
	thread  map[string]any
	reads   int
	appends int
	patches int
}

func newRev9HealthStore(securityContext domainsecurity.TurnSecurityContext) *rev9HealthStore {
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	return &rev9HealthStore{thread: map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath,
		"securityState": securityRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID,
			"status": "running", "securityContext": securityRecord, "items": []any{},
		}},
	}}
}

func (store *rev9HealthStore) GetThread(threadID string) (map[string]any, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.reads++
	if strings.TrimSpace(contracts.StringField(store.thread, "id")) != strings.TrimSpace(threadID) {
		return nil, errors.New("rev9 thread unavailable")
	}
	return contracts.CloneMap(store.thread), nil
}

func (store *rev9HealthStore) AppendItemToTurn(threadID, turnID string, item map[string]any) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if strings.TrimSpace(contracts.StringField(store.thread, "id")) != strings.TrimSpace(threadID) ||
		appturn.ValidateSecurityBoundAppend(store.thread, turnID, item) != nil {
		return errors.New("rev9 append rejected")
	}
	next, ok := appturn.AppendItemToTurn(appturn.AppendItemInput{
		Thread: store.thread, TurnID: turnID, Item: contracts.CloneMap(item),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if !ok {
		return errors.New("rev9 turn unavailable")
	}
	store.thread = next
	store.appends++
	return nil
}

func (store *rev9HealthStore) PatchTurnItemStatus(
	threadID string,
	turnID string,
	itemID string,
	status string,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if strings.TrimSpace(contracts.StringField(store.thread, "id")) != strings.TrimSpace(threadID) ||
		appturn.ValidatePatchTurnItemStatus(store.thread, turnID) != nil {
		return errors.New("rev9 patch rejected")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next, ok := appturn.PatchTurnItemStatus(appturn.PatchItemStatusInput{
		Thread: store.thread, TurnID: turnID, ItemID: itemID, Status: status,
		FinishedAt: now, UpdatedAt: now,
	})
	if !ok {
		return errors.New("rev9 item unavailable")
	}
	store.thread = next
	store.patches++
	return nil
}

func (store *rev9HealthStore) settledReady(
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.appends != 2 || store.patches != 1 || store.reads < 2 {
		return false
	}
	turn, ok := appmodel.TurnByID(store.thread, securityContext.TurnID)
	if !ok {
		return false
	}
	items, ok := turn["items"].([]any)
	if !ok || len(items) != 2 {
		return false
	}
	callItem, ok := items[0].(map[string]any)
	if !ok || strings.TrimSpace(contracts.StringField(callItem, "status")) != "completed" {
		return false
	}
	grant, err := domainsecurity.ParseExecutionGrant(callItem["executionGrant"])
	if err != nil {
		return false
	}
	registry, err := executiongrantapp.RegistryFromThread(
		securityContext.ThreadID,
		store.thread,
		securityContext.TurnID,
	)
	return err == nil && domainsecurity.VerifyExecutionGrantMembership(
		registry,
		securityContext.ThreadID,
		securityContext.TurnID,
		grant,
		domainsecurity.GrantRegistrySettled,
	) == nil
}
