//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	appidentity "analytix.local/runtime-go/internal/app/identity"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	mcp "analytix.local/runtime-go/internal/mcp"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestNativeHealthSourceProbeAcrossRetainedPackagedH1ProductionComposition(t *testing.T) {
	ctx := context.Background()
	counts := &rev11SourceProbeCounts{}
	runtimeServer := strings.TrimSpace(os.Getenv("ANALYTIX_AB_R3_PACKAGE_RUNTIME_SERVER"))
	if runtimeServer != rev9RetainedRuntimeServer {
		rev11SourceProbeFail(t, "owner_open", counts)
	}
	canonicalRuntimeServer, err := filepath.EvalSymlinks(runtimeServer)
	if err != nil || canonicalRuntimeServer != runtimeServer {
		rev11SourceProbeFail(t, "owner_open", counts)
	}
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil || os.Chmod(profileRoot, 0o700) != nil {
		rev11SourceProbeFail(t, "owner_open", counts)
	}
	profileInfo, err := os.Stat(profileRoot)
	if err != nil || profileInfo.Mode().Perm() != 0o700 {
		rev11SourceProbeFail(t, "owner_open", counts)
	}
	owner, err := openRev9DarwinHostCandidateForExecutable(profileRoot, canonicalRuntimeServer)
	if err != nil || owner == nil {
		rev11SourceProbeFail(t, "owner_open", counts)
	}
	counts.ownerOpen.Store(1)
	var runtimeAuthority *nativecomponentapp.RuntimeAuthority
	var ownerClosed atomic.Bool
	t.Cleanup(func() {
		if ownerClosed.Swap(true) {
			return
		}
		if runtimeAuthority != nil {
			_ = runtimeAuthority.Close()
			return
		}
		_ = owner.Close()
	})

	authorityProfileRoot, err := os.MkdirTemp("/private/tmp", "analytix-ab-r3-rev11-authority-")
	if err != nil || os.Chmod(authorityProfileRoot, 0o700) != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	t.Cleanup(func() { _ = os.RemoveAll(authorityProfileRoot) })
	t.Setenv("ANALYTIX_TEST_PROFILE_ROOT", authorityProfileRoot)
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	composition := runtimeWitnessedRegistryDirectCompositionV2(t, fixture, config)
	if composition.evidence == nil || composition.snapshot == nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	if _, err := composition.evidence.Initialize(ctx); err != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	workspace := filepath.Join(profileRoot, "isolated-case")
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	manifestReference, producerReference, materials := runtimeSharedEvidenceBoundMaterialsV2(
		t, observation.WorkspaceRealPath, observation, fixture.InstallationID, fixture.Authority,
	)
	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	materialCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(privateRoot, "dataset-snapshot-authority", "materials"), 16*1024*1024, access,
	)
	if err != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	for _, records := range materials {
		for address, body := range records {
			if err := materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
				rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
			}
		}
	}
	now := time.Now().UTC()
	resolved, err := composition.snapshot.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ManifestReference: manifestReference,
		FundsProducerReference: producerReference, AcceptedAt: now,
	})
	if err != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	securityContext, err := rev11SourceProbeSecurityContext(
		now, observation, resolved.Record.DatasetSnapshotID, resolved.Record.SourceManifestHash,
	)
	if err != nil {
		rev11SourceProbeFail(t, "dsv2_current_selection_ready", counts)
	}
	counts.dsv2CurrentSelectionReady.Store(1)

	hostFixture := newBundledFundsHostValidationFixtureV1(t)
	hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
		ctx, hostFixture.config, hostFixture.dependencies,
	)
	if err != nil || hostSpec == nil {
		rev11SourceProbeFail(t, "host_funds_manager_connected", counts)
	}
	componentCurrent := func(operationContext context.Context) error {
		counts.componentCurrentness.Add(1)
		return owner.ValidateCurrentDataEngine(operationContext)
	}
	manager := mcp.NewProductionManagerWithOptions(nil, mcp.ProductionManagerOptions{
		DatasetAuthority:            composition.snapshot,
		HostFundsServer:             hostSpec,
		HostFundsNativeOwnerCurrent: componentCurrent,
		AccountFlowExecutor: func(
			context.Context,
			mcp.AccountFlowExecutionInput,
			domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
			domainnative.AccountFlowHostEvidenceRowConsumerV1,
		) (domainnative.AccountFlowProviderSemanticResultV1, error) {
			counts.accountFlowExecute.Add(1)
			return domainnative.AccountFlowProviderSemanticResultV1{}, errors.New("rev11 account flow execution is closed")
		},
	})
	manager.Connect()
	t.Cleanup(manager.Disconnect)
	if !rev11SourceProbeManagerConnected(manager, securityContext) {
		rev11SourceProbeFail(t, "host_funds_manager_connected", counts)
	}
	counts.hostFundsManagerConnected.Store(1)

	probeInput := sourceprobeport.Input{
		ServerID: "analytix_funds", Context: securityContext, Binding: observation,
		WorkspaceRealPath: securityContext.WorkspaceRealPath,
		ThreadID:          securityContext.ThreadID, TurnID: securityContext.TurnID,
		CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch:      securityContext.ContextEpoch, ContextDigest: securityContext.ContextDigest,
	}
	probe, err := manager.ProbeCaseSource(ctx, probeInput)
	if err != nil || probe.ProbeDigest == "" || counts.componentCurrentness.Load() != 2 {
		rev11SourceProbeFail(t, "source_probe_admit", counts)
	}
	counts.sourceProbeAdmit.Store(1)
	if err := manager.ValidateCurrentProbe(ctx, "analytix_funds", securityContext); err != nil {
		rev11SourceProbeFail(t, "source_probe_current_before_health", counts)
	}
	counts.sourceProbeCurrentBeforeHealth.Store(1)
	beforeHealthState, ok := rev11SourceProbeManagerState(manager, securityContext)
	if !ok || !beforeHealthState.sourceReady || beforeHealthState.sourceProbeCount != 1 {
		rev11SourceProbeFail(t, "source_probe_current_before_health", counts)
	}
	beforeHealthEvents := manager.HostFundsSourceReadCapabilityEventsV1()
	beforeHealthLiveTools := manager.LiveToolsForSecurityContext(securityContext)
	beforeHealthProviderNames := toolcatalogapp.MCPToolNamesFromAdvertisementsV1(
		toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, securityContext),
	)

	identityAuthority, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("ab-r3-rev11-isolated-installation")),
	)
	if err != nil {
		rev11SourceProbeFail(t, "h1_health_prepare", counts)
	}
	ordinaryInput := turnsecurityapp.CurrentValidationInput{
		OperationContext: ctx, Identity: identityAuthority,
		Observer:      rev9BindingObserver{expectedWorkspace: securityContext.WorkspaceRealPath, observation: observation},
		RiskAuthority: &rev9RiskAuthority{expected: securityContext}, Context: securityContext,
		Workspace: securityContext.WorkspaceRealPath,
	}
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
	healthOnly := nativecomponentapp.NewHealthOnlyCurrentnessV1(ordinaryHealth, componentCurrent)
	if healthOnly == nil {
		rev11SourceProbeFail(t, "h1_health_prepare", counts)
	}
	store := newRev9HealthStore(securityContext)
	generalLive := nativecomponentapp.LiveAuthorityFunc(func(
		context.Context,
		domainsecurity.TurnSecurityContext,
	) error {
		counts.generalLive.Add(1)
		return nativecomponentapp.ErrAuthorityInvalid
	})
	runner := &rev11SourceProbeHealthRunner{
		owner: owner, ordinary: &counts.ordinaryCurrentness,
		component: &counts.componentCurrentness, calls: &counts.healthRunnerExecute,
	}
	runtimeAuthority, err = nativecomponentapp.NewReadyRuntimeAuthority(
		nativecomponentapp.RuntimeAuthorityDependencies{
			Health: nativecomponentapp.HealthDependencies{
				Store: store, DurableAuthority: rev9DurableAuthority{expected: securityContext},
				AcquireEffect: effectgateapp.New().AcquireEffect, Now: func() time.Time { return now },
			},
			LiveAuthority: generalLive, HealthOnlyAuthority: healthOnly, Owner: runner,
		},
	)
	if err != nil || runtimeAuthority == nil {
		rev11SourceProbeFail(t, "h1_health_prepare", counts)
	}
	beforeHealthWitnessAttempts := fixture.TotalAttempts()
	if runtimeAuthority.PrepareCaseTurn(ctx, securityContext) != nil {
		rev11SourceProbeFail(t, "h1_health_prepare", counts)
	}
	if fixture.TotalAttempts() != beforeHealthWitnessAttempts ||
		counts.healthRunnerExecute.Load() != 1 || counts.ordinaryCurrentness.Load() != 3 ||
		counts.componentCurrentness.Load() != 5 || counts.generalLive.Load() != 0 ||
		!store.settledReady(securityContext) {
		rev11SourceProbeFail(t, "h1_health_prepare", counts)
	}
	counts.h1HealthPrepare.Store(1)

	if err := manager.ValidateCurrentProbe(ctx, "analytix_funds", securityContext); err != nil {
		rev11SourceProbeFail(t, "source_probe_current_after_health", counts)
	}
	counts.sourceProbeCurrentAfterHealth.Store(1)
	afterHealthState, ok := rev11SourceProbeManagerState(manager, securityContext)
	if !ok || !reflect.DeepEqual(beforeHealthState, afterHealthState) ||
		!reflect.DeepEqual(beforeHealthEvents, manager.HostFundsSourceReadCapabilityEventsV1()) ||
		counts.componentCurrentness.Load() != 5 || counts.healthRunnerExecute.Load() != 1 {
		rev11SourceProbeFail(t, "source_probe_current_after_health", counts)
	}
	if !reflect.DeepEqual(beforeHealthLiveTools, []string{
		"mcp__analytix_funds__analyze_account_flows",
		"mcp__analytix_funds__count_case_rows",
	}) || !reflect.DeepEqual(manager.LiveToolsForSecurityContext(securityContext), beforeHealthLiveTools) ||
		!reflect.DeepEqual(beforeHealthProviderNames, []string{"mcp__analytix_funds__analyze_account_flows"}) ||
		!reflect.DeepEqual(
			toolcatalogapp.MCPToolNamesFromAdvertisementsV1(
				toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, securityContext),
			),
			beforeHealthProviderNames,
		) || !rev11SourceProbeEffectsRemainZero(counts) {
		rev11SourceProbeFail(t, "live_account_flow_catalog_after_health", counts)
	}
	counts.liveAccountFlowCatalogAfterHealth.Store(1)

	manager.InvalidateHostFundsSourceAfterSettledNativeFailure()
	if manager.ValidateCurrentProbe(ctx, "analytix_funds", securityContext) == nil ||
		len(manager.LiveToolsForSecurityContext(securityContext)) != 0 ||
		len(toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, securityContext)) != 0 ||
		counts.healthRunnerExecute.Load() != 1 || counts.componentCurrentness.Load() != 5 ||
		counts.ordinaryCurrentness.Load() != 3 || !rev11SourceProbeEffectsRemainZero(counts) {
		rev11SourceProbeFail(t, "closure", counts)
	}
	invalidatedState, ok := rev11SourceProbeManagerState(manager, securityContext)
	if !ok || !invalidatedState.connected || invalidatedState.connectionEpoch != beforeHealthState.connectionEpoch ||
		invalidatedState.verifiedServerIdentity != beforeHealthState.verifiedServerIdentity ||
		invalidatedState.catalogFingerprint != beforeHealthState.catalogFingerprint ||
		invalidatedState.sourceReady || invalidatedState.sourceProbeCount != 0 {
		rev11SourceProbeFail(t, "closure", counts)
	}
	counts.closure.Store(1)
	if runtimeAuthority.Close() != nil {
		rev11SourceProbeFail(t, "closure", counts)
	}
	ownerClosed.Store(true)

	t.Logf("closure owner_open=%d dsv2_current_selection_ready=%d host_funds_manager_connected=%d source_probe_admit=%d source_probe_current_before_health=%d h1_health_prepare=%d source_probe_current_after_health=%d live_account_flow_catalog_after_health=%d closure=%d component_currentness=%d ordinary_currentness=%d health_runner_execute=%d general_live=%d account_flow_execute=%d source_copy=%d subject=%d counterparty=%d projection=%d evidence_consumer=%d mcp_semantic=%d", counts.ownerOpen.Load(), counts.dsv2CurrentSelectionReady.Load(), counts.hostFundsManagerConnected.Load(), counts.sourceProbeAdmit.Load(), counts.sourceProbeCurrentBeforeHealth.Load(), counts.h1HealthPrepare.Load(), counts.sourceProbeCurrentAfterHealth.Load(), counts.liveAccountFlowCatalogAfterHealth.Load(), counts.closure.Load(), counts.componentCurrentness.Load(), counts.ordinaryCurrentness.Load(), counts.healthRunnerExecute.Load(), counts.generalLive.Load(), counts.accountFlowExecute.Load(), counts.sourceCopy.Load(), counts.subject.Load(), counts.counterparty.Load(), counts.projection.Load(), counts.evidenceConsumer.Load(), counts.mcpSemantic.Load())
}

func TestTurnStartFinalFrozenContextUsesCurrentProductionFundsSource(t *testing.T) {
	ctx := context.Background()
	counts := &rev13TurnStartCounts{}
	runtimeServer := strings.TrimSpace(os.Getenv("ANALYTIX_AB_R3_PACKAGE_RUNTIME_SERVER"))
	if runtimeServer != rev9RetainedRuntimeServer {
		rev13TurnStartFail(t, "owner_open", counts)
	}
	canonicalRuntimeServer, err := filepath.EvalSymlinks(runtimeServer)
	if err != nil || canonicalRuntimeServer != runtimeServer {
		rev13TurnStartFail(t, "owner_open", counts)
	}
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil || os.Chmod(profileRoot, 0o700) != nil {
		rev13TurnStartFail(t, "owner_open", counts)
	}
	profileInfo, err := os.Stat(profileRoot)
	if err != nil || profileInfo.Mode().Perm() != 0o700 {
		rev13TurnStartFail(t, "owner_open", counts)
	}
	owner, err := openRev9DarwinHostCandidateForExecutable(profileRoot, canonicalRuntimeServer)
	if err != nil || owner == nil {
		rev13TurnStartFail(t, "owner_open", counts)
	}
	counts.ownerOpen.Store(1)
	var ownerClosed atomic.Bool
	t.Cleanup(func() {
		if !ownerClosed.Swap(true) {
			_ = owner.Close()
		}
	})

	authorityProfileRoot, err := os.MkdirTemp("/private/tmp", "analytix-ab-r3-rev13-authority-")
	if err != nil || os.Chmod(authorityProfileRoot, 0o700) != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	t.Cleanup(func() { _ = os.RemoveAll(authorityProfileRoot) })
	t.Setenv("ANALYTIX_TEST_PROFILE_ROOT", authorityProfileRoot)
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	composition := runtimeWitnessedRegistryDirectCompositionV2(t, fixture, config)
	if composition.evidence == nil || composition.snapshot == nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	if _, err := composition.evidence.Initialize(ctx); err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	workspace := filepath.Join(profileRoot, "isolated-turn-start-case")
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	reader := filestore.CaseBindingReader{}
	observation, err := reader.Observe(workspace)
	if err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	manifestReference, producerReference, materials := runtimeSharedEvidenceBoundMaterialsV2(
		t, observation.WorkspaceRealPath, observation, fixture.InstallationID, fixture.Authority,
	)
	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	materialCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(privateRoot, "dataset-snapshot-authority", "materials"), 16*1024*1024, access,
	)
	if err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	for _, records := range materials {
		for address, body := range records {
			if err := materialCAS.PutIfAbsent(ctx, address, body); err != nil && !errors.Is(err, os.ErrExist) {
				rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
			}
		}
	}
	now := time.Now().UTC()
	resolved, err := composition.snapshot.AdmitExactV2(ctx, datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ManifestReference: manifestReference,
		FundsProducerReference: producerReference, AcceptedAt: now,
	})
	if err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	counts.dsv2CurrentSelectionReady.Store(1)

	identityAuthority, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("ab-r3-rev13-isolated-installation")),
	)
	if err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	principal, err := identityAuthority.ResolveCurrent(ctx)
	if err != nil {
		rev13TurnStartFail(t, "dsv2_current_selection_ready", counts)
	}
	positive := rev13NewTurnStartFixture(
		t, "positive", now, observation, resolved, composition.snapshot, identityAuthority, principal,
	)

	hostFixture := newBundledFundsHostValidationFixtureV1(t)
	hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(
		ctx, hostFixture.config, hostFixture.dependencies,
	)
	if err != nil || hostSpec == nil {
		rev13TurnStartFail(t, "host_funds_manager_connected", counts)
	}
	componentCurrent := func(operationContext context.Context) error {
		counts.componentCurrentness.Add(1)
		return owner.ValidateCurrentDataEngine(operationContext)
	}
	manager := mcp.NewProductionManagerWithOptions(nil, mcp.ProductionManagerOptions{
		DatasetAuthority:            composition.snapshot,
		HostFundsServer:             hostSpec,
		HostFundsNativeOwnerCurrent: componentCurrent,
		AccountFlowExecutor: func(
			context.Context,
			mcp.AccountFlowExecutionInput,
			domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
			domainnative.AccountFlowHostEvidenceRowConsumerV1,
		) (domainnative.AccountFlowProviderSemanticResultV1, error) {
			counts.accountFlowExecute.Add(1)
			return domainnative.AccountFlowProviderSemanticResultV1{}, errors.New("rev13 account flow execution is closed")
		},
	})
	manager.Connect()
	t.Cleanup(manager.Disconnect)
	if !rev11SourceProbeManagerConnected(manager, positive.prior) {
		rev13TurnStartFail(t, "host_funds_manager_connected", counts)
	}
	counts.hostFundsManagerConnected.Store(1)
	source := &rev13CountingProductionSource{manager: manager}

	transition, err := turnstartapp.BeginSecurityTransition(
		ctx, positive.state, identityAuthority, principal, reader, source, positive.thread,
		positive.threadID, positive.turnID, positive.workspace, positive.at,
	)
	if err != nil || transition == nil || source.callCount() != 0 {
		rev13TurnStartFail(t, "security_transition_begun", counts)
	}
	counts.securityTransitionBegun.Store(1)
	defer transition.Abort()

	registeredCalls := 0
	var registered domainsecurity.TurnSecurityContext
	preparation, err := turnstartapp.PrepareSecurityTransition(
		ctx, transition, principal, reader, positive.authority, source, positive.thread,
		positive.threadID, positive.turnID, positive.workspace, positive.at,
		rev13TurnStartRecords(), time.Second,
		func(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			registeredCalls++
			if source.callCount() != 0 {
				return errors.New("rev13 source probe ran before final frozen registration")
			}
			registered = securityContext
			return nil
		},
	)
	if err != nil || registeredCalls != 1 || registered == positive.prior ||
		registered.ThreadID != positive.threadID || registered.TurnID != positive.turnID ||
		registered.ContextEpoch != positive.prior.ContextEpoch ||
		registered.ContextDigest == positive.prior.ContextDigest || preparation.SecurityContext != registered {
		rev13TurnStartFail(t, "final_tsc_frozen", counts)
	}
	counts.finalTSCFrozen.Store(1)
	if source.callCount() != 1 || !source.inputMatches(0, registered) ||
		preparation.SourceProbe.ProbeContextDigest != registered.ContextDigest {
		rev13TurnStartFail(t, "production_source_probe_called", counts)
	}
	counts.productionSourceProbeCalled.Store(1)
	if !preparation.ExecutionReady || !preparation.SourceReady || preparation.SourceError != nil ||
		counts.componentCurrentness.Load() != 2 {
		rev13TurnStartFail(t, "source_ready", counts)
	}
	counts.sourceReady.Store(1)
	if preparation.SecurityContext != registered || preparation.EpochState.AcceptedSnapshot.Epoch != registered.ContextEpoch {
		rev13TurnStartFail(t, "transition_prepared", counts)
	}
	counts.transitionPrepared.Store(1)
	if err := transition.Commit(); err != nil {
		rev13TurnStartFail(t, "transition_committed", counts)
	}
	counts.transitionCommitted.Store(1)
	if err := manager.ValidateCurrentProbe(ctx, "analytix_funds", registered); err != nil ||
		source.callCount() != 1 || counts.componentCurrentness.Load() != 2 {
		rev13TurnStartFail(t, "current_probe_after_commit", counts)
	}
	positiveState, ok := rev11SourceProbeManagerState(manager, registered)
	if !ok || !positiveState.sourceReady || positiveState.sourceProbeCount != 1 ||
		!reflect.DeepEqual(
			toolcatalogapp.MCPToolNamesFromAdvertisementsV1(
				toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, registered),
			),
			[]string{"mcp__analytix_funds__analyze_account_flows"},
		) {
		rev13TurnStartFail(t, "current_probe_after_commit", counts)
	}
	counts.currentProbeAfterCommit.Store(1)

	manager.InvalidateHostFundsSourceAfterSettledNativeFailure()
	negative := rev13NewTurnStartFixture(
		t, "revoked", now.Add(time.Second), observation, resolved, composition.snapshot, identityAuthority, principal,
	)
	negativeTransition, err := turnstartapp.BeginSecurityTransition(
		ctx, negative.state, identityAuthority, principal, reader, source, negative.thread,
		negative.threadID, negative.turnID, negative.workspace, negative.at,
	)
	if err != nil || negativeTransition == nil || source.callCount() != 1 {
		rev13TurnStartFail(t, "closure", counts)
	}
	defer negativeTransition.Abort()
	negativeRegistrations := 0
	var negativeRegistered domainsecurity.TurnSecurityContext
	negativePreparation, err := turnstartapp.PrepareSecurityTransition(
		ctx, negativeTransition, principal, reader, negative.authority, source, negative.thread,
		negative.threadID, negative.turnID, negative.workspace, negative.at,
		rev13TurnStartRecords(), time.Second,
		func(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			negativeRegistrations++
			if source.callCount() != 1 {
				return errors.New("rev13 revoked source probe ran before final frozen registration")
			}
			negativeRegistered = securityContext
			return nil
		},
	)
	if err != nil || negativeRegistrations != 1 || negativePreparation.SecurityContext != negativeRegistered ||
		!negativePreparation.ExecutionReady || negativePreparation.SourceReady || negativePreparation.SourceError == nil ||
		source.callCount() != 2 || counts.componentCurrentness.Load() != 2 {
		rev13TurnStartFail(t, "closure", counts)
	}
	if err := negativeTransition.Commit(); err != nil ||
		manager.ValidateCurrentProbe(ctx, "analytix_funds", negativeRegistered) == nil ||
		len(toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, negativeRegistered)) != 0 {
		rev13TurnStartFail(t, "closure", counts)
	}
	negativeState, ok := rev11SourceProbeManagerState(manager, negativeRegistered)
	if !ok || !negativeState.connected || negativeState.sourceReady || negativeState.sourceProbeCount != 0 ||
		counts.componentCurrentness.Load() != 2 || !rev13TurnStartEffectsRemainZero(counts) {
		rev13TurnStartFail(t, "closure", counts)
	}
	counts.negativeRevoked.Store(1)
	counts.closure.Store(1)

	if err := owner.Close(); err != nil {
		rev13TurnStartFail(t, "closure", counts)
	}
	ownerClosed.Store(true)
	t.Logf("closure owner_open=%d dsv2_current_selection_ready=%d host_funds_manager_connected=%d security_transition_begun=%d final_tsc_frozen=%d production_source_probe_called=%d source_ready=%d transition_prepared=%d transition_committed=%d current_probe_after_commit=%d closure=%d negative_revoked=%d source_probe_calls=%d component_currentness=%d health_runner=%d account_flow_execute=%d source_copy=%d projection=%d counterparty=%d evidence_consumer=%d provider=%d mcp_semantic=%d", counts.ownerOpen.Load(), counts.dsv2CurrentSelectionReady.Load(), counts.hostFundsManagerConnected.Load(), counts.securityTransitionBegun.Load(), counts.finalTSCFrozen.Load(), counts.productionSourceProbeCalled.Load(), counts.sourceReady.Load(), counts.transitionPrepared.Load(), counts.transitionCommitted.Load(), counts.currentProbeAfterCommit.Load(), counts.closure.Load(), counts.negativeRevoked.Load(), source.callCount(), counts.componentCurrentness.Load(), counts.healthRunner.Load(), counts.accountFlowExecute.Load(), counts.sourceCopy.Load(), counts.projection.Load(), counts.counterparty.Load(), counts.evidenceConsumer.Load(), counts.provider.Load(), counts.mcpSemantic.Load())
}

type rev13TurnStartCounts struct {
	ownerOpen                   atomic.Int32
	dsv2CurrentSelectionReady   atomic.Int32
	hostFundsManagerConnected   atomic.Int32
	securityTransitionBegun     atomic.Int32
	finalTSCFrozen              atomic.Int32
	productionSourceProbeCalled atomic.Int32
	sourceReady                 atomic.Int32
	transitionPrepared          atomic.Int32
	transitionCommitted         atomic.Int32
	currentProbeAfterCommit     atomic.Int32
	closure                     atomic.Int32
	negativeRevoked             atomic.Int32
	componentCurrentness        atomic.Int32
	healthRunner                atomic.Int32
	accountFlowExecute          atomic.Int32
	sourceCopy                  atomic.Int32
	projection                  atomic.Int32
	counterparty                atomic.Int32
	evidenceConsumer            atomic.Int32
	provider                    atomic.Int32
	mcpSemantic                 atomic.Int32
}

type rev13CountingProductionSource struct {
	manager *mcp.ProductionManager
	mu      sync.Mutex
	inputs  []sourceprobeport.Input
}

func (source *rev13CountingProductionSource) ProbeCaseSource(
	ctx context.Context,
	input sourceprobeport.Input,
) (domainsecurity.VerifiedSourceProbe, error) {
	if source == nil || source.manager == nil {
		return domainsecurity.VerifiedSourceProbe{}, errors.New("rev13 production source is unavailable")
	}
	source.mu.Lock()
	source.inputs = append(source.inputs, input)
	source.mu.Unlock()
	return source.manager.ProbeCaseSource(ctx, input)
}

func (source *rev13CountingProductionSource) callCount() int {
	if source == nil {
		return 0
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	return len(source.inputs)
}

func (source *rev13CountingProductionSource) inputMatches(
	index int,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	if source == nil {
		return false
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	if index < 0 || index >= len(source.inputs) {
		return false
	}
	input := source.inputs[index]
	return input.ServerID == "analytix_funds" && input.Context == securityContext &&
		input.WorkspaceRealPath == securityContext.WorkspaceRealPath &&
		input.ThreadID == securityContext.ThreadID && input.TurnID == securityContext.TurnID &&
		input.CaseID == securityContext.CaseID && input.CaseBindingHash == securityContext.CaseBindingHash &&
		input.DatasetSnapshotID == securityContext.DatasetSnapshotID &&
		input.ContextEpoch == securityContext.ContextEpoch && input.ContextDigest == securityContext.ContextDigest
}

type rev13TurnStartFixture struct {
	threadID  string
	turnID    string
	workspace string
	at        time.Time
	thread    map[string]any
	prior     domainsecurity.TurnSecurityContext
	state     *subagentapp.RuntimeState
	authority turnsecurityapp.WorkspaceSecurityAuthority
}

func rev13NewTurnStartFixture(
	t *testing.T,
	suffix string,
	at time.Time,
	observation domainsecurity.CaseBindingObservationV1,
	resolved datasetsnapshotport.ResolvedSnapshotV2,
	snapshot datasetsnapshotport.AuthorityV2,
	identity identityport.Authority,
	principal domainidentity.PrincipalV1,
) rev13TurnStartFixture {
	t.Helper()
	threadID := "thread-ab-r3-rev13-" + suffix
	turnID := "turn-ab-r3-rev13-" + suffix
	risk := rev13NewRiskAuthority(t, threadID, observation, at.Add(-2*time.Minute))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: risk.policy.PolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal("rev13 prior publication fixture is invalid")
	}
	prior, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-ab-r3-rev13-prior-" + suffix,
		WorkspaceRealPath: observation.WorkspaceRealPath,
		TenantID:          principal.TenantID, UserID: principal.UserID,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID:  resolved.Record.DatasetSnapshotID,
		SourceManifestHash: resolved.Record.SourceManifestHash,
		ContextEpoch:       1, IssuedAt: at.Add(-time.Minute), PublicationPolicy: publication,
		RiskAuthorityBinding: risk.contracts.Binding,
	})
	if err != nil {
		t.Fatal("rev13 prior security context fixture is invalid")
	}
	epochState, err := contextepochapp.BootstrapState(
		threadID, prior.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(prior)},
		at.Add(-time.Minute),
	)
	if err != nil {
		t.Fatal("rev13 prior context epoch fixture is invalid")
	}
	return rev13TurnStartFixture{
		threadID: threadID, turnID: turnID, workspace: observation.WorkspaceRealPath, at: at,
		thread: map[string]any{
			"id": threadID, "workspace": observation.WorkspaceRealPath,
			"securityState":     turnsecurityapp.PublicRecord(prior),
			"contextEpochState": contextepochapp.PublicState(epochState),
		},
		prior: prior, state: subagentapp.NewRuntimeState(),
		authority: turnsecurityapp.WorkspaceSecurityAuthority{
			Identity: identity, Observer: filestore.CaseBindingReader{}, RiskAuthority: risk,
			SnapshotAuthorityV2: snapshot,
			RiskIntent:          domainsecurity.RiskClassCase, ProtectedCaseData: true, TrustedCaseThread: true,
		},
	}
}

type rev13RiskAuthority struct {
	policy    domainsecurity.ThreadRiskPolicyV1
	contracts securitytest.RiskAuthorityContracts
}

func rev13NewRiskAuthority(
	t *testing.T,
	threadID string,
	observation domainsecurity.CaseBindingObservationV1,
	issuedAt time.Time,
) *rev13RiskAuthority {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x62}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: threadID, WorkspaceRealPath: observation.WorkspaceRealPath,
		RiskClass: domainsecurity.RiskClassCase, Origin: domainsecurity.RiskPolicyOriginValidCaseBinding,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("ab-r3-rev13-risk:" + threadID)),
		IssuedAt:      issuedAt, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal("rev13 risk policy fixture is invalid")
	}
	contracts, err := securitytest.WitnessedRiskAuthorityContracts(
		threadID, observation.WorkspaceRealPath, domainsecurity.RiskClassCase, policy.PolicyDigest,
	)
	if err != nil {
		t.Fatal("rev13 risk authority fixture is invalid")
	}
	return &rev13RiskAuthority{policy: policy, contracts: contracts}
}

func (authority *rev13RiskAuthority) ResolveOrRaise(
	ctx context.Context,
	input threadriskauthorityapp.ResolveOrRaiseInput,
) (threadriskauthorityapp.Head, error) {
	if ctx == nil || ctx.Err() != nil || authority == nil ||
		input.ThreadID != authority.policy.ThreadID || input.WorkspaceRealPath != authority.policy.WorkspaceRealPath ||
		input.RequestedRisk != domainsecurity.RiskClassCase ||
		input.BindingObservation.State != domainsecurity.CaseBindingStateValid ||
		input.BindingObservation.WorkspaceRealPath != authority.policy.WorkspaceRealPath {
		return threadriskauthorityapp.Head{}, errors.New("rev13 risk authority request mismatch")
	}
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: authority.contracts.Index, Request: authority.contracts.Request,
		Observation: authority.contracts.Observation, RiskAuthorityBinding: authority.contracts.Binding,
		Policy: authority.policy, Found: true,
	}, nil
}

func (authority *rev13RiskAuthority) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if ctx == nil || ctx.Err() != nil || authority == nil ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		securityContext.ThreadID != authority.policy.ThreadID ||
		securityContext.WorkspaceRealPath != authority.policy.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(
			securityContext.PublicationPolicy, authority.policy,
		) != nil || domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
		securityContext.RiskAuthorityBinding,
		authority.contracts.Index,
		authority.contracts.Request,
		authority.contracts.Observation,
	) != nil {
		return errors.New("rev13 risk authority head mismatch")
	}
	return nil
}

func rev13TurnStartRecords() turnstartapp.SecurityRecords {
	return turnstartapp.SecurityRecords{
		Turn: map[string]any{}, TurnStartedEvent: map[string]any{}, ThreadPatch: map[string]any{},
	}
}

func rev13TurnStartEffectsRemainZero(counts *rev13TurnStartCounts) bool {
	return counts != nil && counts.healthRunner.Load() == 0 && counts.accountFlowExecute.Load() == 0 &&
		counts.sourceCopy.Load() == 0 && counts.projection.Load() == 0 && counts.counterparty.Load() == 0 &&
		counts.evidenceConsumer.Load() == 0 && counts.provider.Load() == 0 && counts.mcpSemantic.Load() == 0
}

func rev13TurnStartFail(t *testing.T, phase string, counts *rev13TurnStartCounts) {
	t.Helper()
	if counts == nil {
		counts = &rev13TurnStartCounts{}
	}
	t.Fatalf("%s owner_open=%d dsv2_current_selection_ready=%d host_funds_manager_connected=%d security_transition_begun=%d final_tsc_frozen=%d production_source_probe_called=%d source_ready=%d transition_prepared=%d transition_committed=%d current_probe_after_commit=%d closure=%d negative_revoked=%d component_currentness=%d health_runner=%d account_flow_execute=%d source_copy=%d projection=%d counterparty=%d evidence_consumer=%d provider=%d mcp_semantic=%d", phase, counts.ownerOpen.Load(), counts.dsv2CurrentSelectionReady.Load(), counts.hostFundsManagerConnected.Load(), counts.securityTransitionBegun.Load(), counts.finalTSCFrozen.Load(), counts.productionSourceProbeCalled.Load(), counts.sourceReady.Load(), counts.transitionPrepared.Load(), counts.transitionCommitted.Load(), counts.currentProbeAfterCommit.Load(), counts.closure.Load(), counts.negativeRevoked.Load(), counts.componentCurrentness.Load(), counts.healthRunner.Load(), counts.accountFlowExecute.Load(), counts.sourceCopy.Load(), counts.projection.Load(), counts.counterparty.Load(), counts.evidenceConsumer.Load(), counts.provider.Load(), counts.mcpSemantic.Load())
}

type rev11SourceProbeCounts struct {
	ownerOpen                         atomic.Int32
	dsv2CurrentSelectionReady         atomic.Int32
	hostFundsManagerConnected         atomic.Int32
	sourceProbeAdmit                  atomic.Int32
	sourceProbeCurrentBeforeHealth    atomic.Int32
	h1HealthPrepare                   atomic.Int32
	sourceProbeCurrentAfterHealth     atomic.Int32
	liveAccountFlowCatalogAfterHealth atomic.Int32
	closure                           atomic.Int32
	componentCurrentness              atomic.Int32
	ordinaryCurrentness               atomic.Int32
	healthRunnerExecute               atomic.Int32
	generalLive                       atomic.Int32
	accountFlowExecute                atomic.Int32
	sourceCopy                        atomic.Int32
	subject                           atomic.Int32
	counterparty                      atomic.Int32
	projection                        atomic.Int32
	evidenceConsumer                  atomic.Int32
	mcpSemantic                       atomic.Int32
}

func rev11SourceProbeFail(t *testing.T, phase string, counts *rev11SourceProbeCounts) {
	t.Helper()
	t.Fatalf("%s owner_open=%d dsv2_current_selection_ready=%d host_funds_manager_connected=%d source_probe_admit=%d source_probe_current_before_health=%d h1_health_prepare=%d source_probe_current_after_health=%d live_account_flow_catalog_after_health=%d closure=%d component_currentness=%d ordinary_currentness=%d health_runner_execute=%d general_live=%d account_flow_execute=%d source_copy=%d subject=%d counterparty=%d projection=%d evidence_consumer=%d mcp_semantic=%d", phase, counts.ownerOpen.Load(), counts.dsv2CurrentSelectionReady.Load(), counts.hostFundsManagerConnected.Load(), counts.sourceProbeAdmit.Load(), counts.sourceProbeCurrentBeforeHealth.Load(), counts.h1HealthPrepare.Load(), counts.sourceProbeCurrentAfterHealth.Load(), counts.liveAccountFlowCatalogAfterHealth.Load(), counts.closure.Load(), counts.componentCurrentness.Load(), counts.ordinaryCurrentness.Load(), counts.healthRunnerExecute.Load(), counts.generalLive.Load(), counts.accountFlowExecute.Load(), counts.sourceCopy.Load(), counts.subject.Load(), counts.counterparty.Load(), counts.projection.Load(), counts.evidenceConsumer.Load(), counts.mcpSemantic.Load())
}

func rev11SourceProbeEffectsRemainZero(counts *rev11SourceProbeCounts) bool {
	return counts != nil && counts.accountFlowExecute.Load() == 0 && counts.sourceCopy.Load() == 0 &&
		counts.subject.Load() == 0 && counts.counterparty.Load() == 0 && counts.projection.Load() == 0 &&
		counts.evidenceConsumer.Load() == 0 && counts.mcpSemantic.Load() == 0
}

func rev11SourceProbeSecurityContext(
	now time.Time,
	observation domainsecurity.CaseBindingObservationV1,
	datasetSnapshotID string,
	sourceManifestHash string,
) (domainsecurity.TurnSecurityContext, error) {
	const threadID = "thread-ab-r3-rev11-source-probe"
	const turnID = "turn-ab-r3-rev11-source-probe"
	policyDigest := domainsecurity.SHA256Hex([]byte("ab-r3-rev11-risk-policy"))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(
		domainsecurity.TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
			Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
			CaseBindingState:         domainsecurity.CaseBindingStateValid,
			BindingObservationDigest: observation.ObservationDigest,
			BlockerCode:              domainsecurity.PublicationBlockerNone,
		},
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	riskBinding, err := securitytest.WitnessedRiskBinding(
		threadID, observation.WorkspaceRealPath, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: observation.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID: datasetSnapshotID, SourceManifestHash: sourceManifestHash,
		ContextEpoch: 1, IssuedAt: now, PublicationPolicy: publication,
		RiskAuthorityBinding: riskBinding,
	})
}

type rev11SourceProbeManagerStateV1 struct {
	connected              bool
	connectionEpoch        float64
	verifiedServerIdentity string
	catalogFingerprint     string
	sourceReady            bool
	sourceProbeCount       float64
	sourceProbeDigest      string
	toolNames              []string
}

func rev11SourceProbeManagerState(
	manager *mcp.ProductionManager,
	securityContext domainsecurity.TurnSecurityContext,
) (rev11SourceProbeManagerStateV1, bool) {
	if manager == nil {
		return rev11SourceProbeManagerStateV1{}, false
	}
	for _, raw := range manager.ServerDiagnosticsForSecurityContext(securityContext) {
		diagnostic, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, idOK := diagnostic["id"].(string)
		if !idOK || strings.TrimSpace(id) != "analytix_funds" {
			continue
		}
		connected, connectedOK := diagnostic["connected"].(bool)
		connectionEpoch, epochOK := diagnostic["connectionEpoch"].(float64)
		verifiedServerIdentity, identityOK := diagnostic["verifiedServerIdentity"].(string)
		catalogFingerprint, catalogOK := diagnostic["catalogFingerprint"].(string)
		sourceReady, readyOK := diagnostic["sourceReady"].(bool)
		sourceProbeCount, countOK := diagnostic["sourceProbeCount"].(float64)
		sourceProbeDigest, digestOK := diagnostic["sourceProbeDigest"].(string)
		toolNames, toolsOK := diagnostic["toolNames"].([]string)
		if !connectedOK || !epochOK || !identityOK || !catalogOK || !readyOK || !countOK || !digestOK || !toolsOK {
			return rev11SourceProbeManagerStateV1{}, false
		}
		return rev11SourceProbeManagerStateV1{
			connected: connected, connectionEpoch: connectionEpoch,
			verifiedServerIdentity: verifiedServerIdentity, catalogFingerprint: catalogFingerprint,
			sourceReady: sourceReady, sourceProbeCount: sourceProbeCount,
			sourceProbeDigest: sourceProbeDigest, toolNames: append([]string(nil), toolNames...),
		}, true
	}
	return rev11SourceProbeManagerStateV1{}, false
}

func rev11SourceProbeManagerConnected(
	manager *mcp.ProductionManager,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	state, ok := rev11SourceProbeManagerState(manager, securityContext)
	return ok && state.connected && state.connectionEpoch == 1 && state.verifiedServerIdentity != "" &&
		state.catalogFingerprint != "" && !state.sourceReady && state.sourceProbeCount == 0 &&
		reflect.DeepEqual(state.toolNames, []string{
			"mcp__analytix_funds__analyze_account_flows",
			"mcp__analytix_funds__count_case_rows",
		})
}

type rev11SourceProbeHealthRunner struct {
	owner     *nativecomponenthost.Owner
	ordinary  *atomic.Int32
	component *atomic.Int32
	calls     *atomic.Int32
}

func (runner *rev11SourceProbeHealthRunner) Execute(
	ctx context.Context,
	request domainnative.Request,
) (domainnative.Result, error) {
	if runner == nil || runner.owner == nil || runner.calls.Add(1) != 1 ||
		runner.ordinary.Load() != 2 || runner.component.Load() != 4 {
		return domainnative.Result{}, nativecomponentapp.ErrAuthorityInvalid
	}
	return runner.owner.Execute(ctx, request)
}

func (runner *rev11SourceProbeHealthRunner) Close() error {
	if runner == nil || runner.owner == nil {
		return nil
	}
	return runner.owner.Close()
}
