//go:build darwin && analytix_prod

package runtimeapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	fundscsvsourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundscsvsource"
	fundsquerysourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	"analytix.local/runtime-go/internal/contracts"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	mcp "analytix.local/runtime-go/internal/mcp"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	toolidentity "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

const (
	rev14AccountFlowToolName   = "mcp__analytix_funds__analyze_account_flows"
	rev14PrivateAccount        = "6222021234567890"
	rev14PrivateCard           = "6222021234567890001"
	rev14PrivateCounterparty   = "6217009876543210"
	rev14PrivateSourceSentinel = "AB_R3_REV14_PRIVATE_SOURCE_SENTINEL_82d3f9a1"
)

func TestFundsAccountFlowToolRetainedPackageProductionComposition(t *testing.T) {
	ctx := context.Background()
	counts := &rev14AccountFlowCounts{}
	runtimeServer := strings.TrimSpace(os.Getenv("ANALYTIX_AB_R3_PACKAGE_RUNTIME_SERVER"))
	if runtimeServer == "" {
		rev14AccountFlowFail(t, "owner_open", counts)
	}
	canonicalRuntimeServer, err := filepath.EvalSymlinks(runtimeServer)
	if err != nil || canonicalRuntimeServer != runtimeServer {
		rev14AccountFlowFail(t, "owner_open", counts)
	}
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil || os.Chmod(profileRoot, 0o700) != nil {
		rev14AccountFlowFail(t, "owner_open", counts)
	}
	profileInfo, err := os.Stat(profileRoot)
	if err != nil || profileInfo.Mode().Perm() != 0o700 {
		rev14AccountFlowFail(t, "owner_open", counts)
	}
	owner, err := openRev9DarwinHostCandidateForExecutable(profileRoot, canonicalRuntimeServer)
	if err != nil || owner == nil {
		rev14AccountFlowFail(t, "owner_open", counts)
	}
	countedOwner := &rev14AccountFlowOwner{Owner: owner, counts: counts}
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
		_ = countedOwner.Close()
	})

	authorityProfileRoot, err := os.MkdirTemp("/private/tmp", "analytix-ab-r3-rev14-authority-")
	if err != nil || os.Chmod(authorityProfileRoot, 0o700) != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	t.Cleanup(func() { _ = os.RemoveAll(authorityProfileRoot) })
	t.Setenv("ANALYTIX_TEST_PROFILE_ROOT", authorityProfileRoot)
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	snapshotStores, err := datasetsnapshotstore.OpenStoresV2(
		filepath.Join(privateRoot, "dataset-snapshot-authority"), access,
	)
	if err != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	evidenceStores, err := openRuntimeSharedEvidenceStoresV2(fixture.DataDir, access)
	if err != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	datasetComposition, configured, err := newRuntimeSharedEvidenceDatasetSnapshotV2(
		ctx, config, fixture.Authority, snapshotStores, evidenceStores, access, filestore.CaseBindingReader{}, nil,
	)
	if err != nil || !configured || datasetComposition.evidence == nil || datasetComposition.snapshot == nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	identityAuthority, err := newRuntimeHostIdentityAuthority(fixture.Authority)
	if err != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	immutableSource, err := fundsquerysourceadapter.NewHostExactSource(profileRoot)
	if err != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	admission, err := fundscsvadmissionapp.NewServiceV1(fundscsvadmissionapp.ConfigV1{
		Observer: filestore.CaseBindingReader{}, Identity: identityAuthority,
		Evidence: datasetComposition.evidence, Snapshots: datasetComposition.snapshot,
		Materials: snapshotStores, Native: countedOwner, Source: immutableSource,
		ReadImportSource: fundscsvsourceadapter.ReadImportExactV1,
	})
	if err != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	workspace := filepath.Join(profileRoot, "isolated-case")
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	source := rev14AccountFlowCSV()
	sourcePath := filepath.Join(workspace, "synthetic-generation-one.csv")
	if os.WriteFile(sourcePath, source, 0o600) != nil {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	t.Cleanup(func() { clear(source) })
	staged, err := admission.StageMainSelectedImportV1(ctx, fundscsvadmissionapp.StageInputV1{
		WorkspaceRoot: workspace, SourcePath: sourcePath,
	})
	if err != nil || staged.Status != "ready" || staged.TotalRowCount != 1 || len(staged.Items) != 1 ||
		staged.Items[0].RowCount != 1 || staged.Items[0].Selector == "" {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	counts.stage.Store(1)
	confirmed, err := admission.ConfirmImportV1(ctx, staged.Items[0].Selector)
	if err != nil || confirmed.SourceRowCount != 1 || confirmed.SourceArtifactSHA256 != domainsecurity.SHA256Hex(source) ||
		confirmed.SourceArtifactByteLength != uint64(len(source)) {
		rev14AccountFlowFail(t, "stage_confirm", counts)
	}
	counts.confirm.Store(1)

	reader := filestore.CaseBindingReader{}
	observation, err := reader.Observe(workspace)
	if err != nil {
		rev14AccountFlowFail(t, "dsv2_current_selection", counts)
	}
	resolved, err := datasetComposition.snapshot.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation,
	})
	if err != nil || resolved.Record.DatasetSnapshotID == "" || resolved.Record.SourceManifestHash == "" {
		rev14AccountFlowFail(t, "dsv2_current_selection", counts)
	}
	counts.dsv2.Store(1)

	caseCapability, err := openRuntimeCaseEntityCapabilityV1(
		ctx, filepath.Join(privateRoot, "case-entities"), access,
	)
	if err != nil || caseCapability.store == nil || !caseCapability.semanticUse {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	t.Cleanup(func() { _ = caseCapability.Close() })
	principal, err := identityAuthority.ResolveCurrent(ctx)
	if err != nil {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	now := time.Now().UTC()
	turnFixture := rev13NewTurnStartFixture(
		t, "account-flow", now, observation, resolved, datasetComposition.snapshot, identityAuthority, principal,
	)
	validateFull := func(operationContext context.Context, current domainsecurity.TurnSecurityContext) error {
		return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
			OperationContext: operationContext, Identity: identityAuthority, Observer: reader,
			RiskAuthority: turnFixture.authority.RiskAuthority, SnapshotAuthorityV2: datasetComposition.snapshot,
			Context: current, Workspace: current.WorkspaceRealPath,
		})
	}
	fundsComposition := composeRuntimeFundsAccountFlowV1(
		profileRoot, rev14KeyedDigester{key: []byte("ab-r3-rev14-case-entity-key")}, caseCapability.store,
		datasetComposition.snapshot, reader, snapshotStores.Materials, validateFull,
	)
	if !fundsComposition.available() || fundsComposition.caseEntities == nil {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	store := &rev9HealthStore{}
	durable := &rev14DurableAuthority{}
	var manager *mcp.ProductionManager
	nativeDependencies := nativecomponentapp.RuntimeAuthorityDependencies{
		Health: nativecomponentapp.HealthDependencies{
			Store: store, DurableAuthority: durable,
			AcquireEffect: turnFixture.state.AcquireContextEffect, Now: time.Now,
		},
		LiveAuthority: nativecomponentapp.LiveAuthorityFunc(validateFull),
		HealthOnlyAuthority: nativecomponentapp.NewHealthOnlyCurrentnessV1(
			nativecomponentapp.LiveAuthorityFunc(func(operationContext context.Context, current domainsecurity.TurnSecurityContext) error {
				return turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{
					OperationContext: operationContext, Identity: identityAuthority, Observer: reader,
					RiskAuthority: turnFixture.authority.RiskAuthority, Context: current,
					Workspace: current.WorkspaceRealPath,
				})
			}),
			countedOwner.ValidateCurrentDataEngine,
		),
		Owner: countedOwner,
	}
	if !fundsComposition.applyToRuntimeAuthorityV1(
		&nativeDependencies,
		func(operationContext context.Context, current domainsecurity.TurnSecurityContext) error {
			counts.callbackCurrent.Add(1)
			err := turnsecurityapp.ValidateCurrentInsideExactDatasetCapability(turnsecurityapp.CurrentValidationInput{
				OperationContext: operationContext, Identity: identityAuthority, Observer: reader,
				RiskAuthority: turnFixture.authority.RiskAuthority, Context: current,
				Workspace: current.WorkspaceRealPath,
			})
			if err == nil {
				counts.callbackCurrentSuccess.Add(1)
			}
			return err
		},
		func(operationContext context.Context, current domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) error {
			counts.outerGrant.Add(1)
			if manager == nil {
				return errors.New("rev14 manager unavailable")
			}
			err := manager.ValidateCurrentAccountFlowOuterGrant(operationContext, current, grant)
			if err == nil {
				counts.outerGrantSuccess.Add(1)
			}
			return err
		},
	) {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	nativeDependencies.UseCurrentAccountFlowSource = rev14CountCurrentSource(
		counts, nativeDependencies.UseCurrentAccountFlowSource,
	)
	nativeDependencies.ResolveAccountFlowSubject = rev14CountSubject(
		counts, nativeDependencies.ResolveAccountFlowSubject,
	)
	nativeDependencies.ResolveAccountFlowCounterparty = rev14CountCounterparty(
		counts, nativeDependencies.ResolveAccountFlowCounterparty,
	)
	runtimeAuthority, err = nativecomponentapp.NewReadyRuntimeAuthority(nativeDependencies)
	if err != nil || runtimeAuthority == nil || !runtimeAuthority.AccountFlowsAvailable() {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	hostFixture := newBundledFundsHostValidationFixtureV1(t)
	hostSpec, err := admitBundledFundsHostForStartupWithDependenciesV1(ctx, hostFixture.config, hostFixture.dependencies)
	if err != nil || hostSpec == nil {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	baseExecutor := runtimeFundsAccountFlowExecutorV1(runtimeAuthority)
	if baseExecutor == nil {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	manager = mcp.NewProductionManagerWithOptions(nil, mcp.ProductionManagerOptions{
		DatasetAuthority: datasetComposition.snapshot, HostFundsServer: hostSpec,
		HostFundsNativeOwnerCurrent: countedOwner.ValidateCurrentDataEngine,
		AccountFlowExecutor:         rev14CountAccountFlowExecutor(counts, baseExecutor),
	})
	manager.Connect()
	t.Cleanup(manager.Disconnect)
	if !rev11SourceProbeManagerConnected(manager, turnFixture.prior) {
		rev14AccountFlowFail(t, "runtime_composition", counts)
	}
	counts.runtimeComposition.Store(1)

	productionSource := &rev13CountingProductionSource{manager: manager}
	transition, err := turnstartapp.BeginSecurityTransition(
		ctx, turnFixture.state, identityAuthority, principal, reader, productionSource, turnFixture.thread,
		turnFixture.threadID, turnFixture.turnID, turnFixture.workspace, turnFixture.at,
	)
	if err != nil || transition == nil || productionSource.callCount() != 0 {
		rev14AccountFlowFail(t, "final_tsc_source_ready", counts)
	}
	defer transition.Abort()
	var finalContext domainsecurity.TurnSecurityContext
	registrations := 0
	preparation, err := turnstartapp.PrepareSecurityTransition(
		ctx, transition, principal, reader, turnFixture.authority, productionSource, turnFixture.thread,
		turnFixture.threadID, turnFixture.turnID, turnFixture.workspace, turnFixture.at,
		rev13TurnStartRecords(), time.Second,
		func(_ context.Context, current domainsecurity.TurnSecurityContext) error {
			registrations++
			if productionSource.callCount() != 0 {
				return errors.New("rev14 source probed before registration")
			}
			finalContext = current
			return nil
		},
	)
	if err != nil || registrations != 1 || preparation.SecurityContext != finalContext ||
		!preparation.ExecutionReady || !preparation.SourceReady || preparation.SourceError != nil ||
		productionSource.callCount() != 1 {
		rev14AccountFlowFail(t, "final_tsc_source_ready", counts)
	}
	if transition.Commit() != nil || manager.ValidateCurrentProbe(ctx, "analytix_funds", finalContext) != nil {
		rev14AccountFlowFail(t, "final_tsc_source_ready", counts)
	}
	counts.finalTSCSourceReady.Store(1)

	reference, err := fundsComposition.caseEntities.BindReferenceV1(
		ctx,
		caseentityapp.NewDeriveReferenceInputV1(
			finalContext,
			domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			rev14PrivateAccount,
		),
	)
	if err != nil || domaincaseentity.ValidateReferenceV1(string(reference)) != nil {
		rev14AccountFlowFail(t, "case_subject", counts)
	}
	alias := domaincaseentity.ModelEntityAliasV1("acct:1")
	aliasUses := 0
	if err := fundsComposition.caseEntities.UseVerifiedBindingByAliasV1(
		ctx,
		caseentityapp.ResolveVerifiedBindingByAliasInputV1{SecurityContext: finalContext, Alias: alias},
		func(currentReference domaincaseentity.ReferenceV1, canonicalValue string, recordDigest string) error {
			aliasUses++
			if currentReference != reference || canonicalValue != rev14PrivateAccount ||
				!domainsecurity.IsSHA256Hex(recordDigest) {
				return errors.New("rev14 alias binding mismatch")
			}
			return nil
		},
	); err != nil || aliasUses != 1 {
		rev14AccountFlowFail(t, "case_subject", counts)
	}
	counts.caseSubject.Store(1)

	advertisements := toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, finalContext)
	if !reflect.DeepEqual(toolcatalogapp.MCPToolNamesFromAdvertisementsV1(advertisements), []string{rev14AccountFlowToolName}) ||
		!reflect.DeepEqual(manager.LiveToolsForSecurityContext(finalContext), []string{
			rev14AccountFlowToolName, "mcp__analytix_funds__count_case_rows",
		}) {
		rev14AccountFlowFail(t, "grant_catalog", counts)
	}
	arguments := map[string]any{
		"subject_alias":      alias,
		"start_inclusive":    "2026-08-27T00:00:00.000000Z",
		"end_inclusive":      "2026-08-27T23:59:59.999999Z",
		"evidence_row_limit": 1,
	}
	argumentBytes, err := json.Marshal(arguments)
	if err != nil || len(advertisements) != 1 {
		rev14AccountFlowFail(t, "grant_catalog", counts)
	}
	call := domainmodel.ToolCall{
		ID:   toolidentity.MustHostToolCallIDV1("ab-r3-rev14-account-flow"),
		Name: rev14AccountFlowToolName, Arguments: argumentBytes,
	}
	advertisement := advertisements[0]
	schemas := []domainmodel.ToolSchema{{
		Name: advertisement.Name, Description: advertisement.Description,
		Parameters:   append(json.RawMessage(nil), advertisement.InputSchema...),
		OutputSchema: append(json.RawMessage(nil), advertisement.OutputSchema...),
		Source:       "mcp", TaskSupport: string(advertisement.TaskSupport),
	}}
	issuedAt := time.Now().UTC()
	grant, err := executiongrantapp.IssueProvider(
		finalContext, "ab-r3-rev14-provider", call, schemas, []string{rev14AccountFlowToolName},
		true, "not_required", advertisement.ConnectionEpoch, advertisement.ServerIdentity, issuedAt,
	)
	if err != nil {
		rev14AccountFlowFail(t, "grant_catalog", counts)
	}
	callItem, _, err := appturn.ToolCallReadyRecords(appturn.ToolCallReadyInput{
		ThreadID: finalContext.ThreadID, TurnID: finalContext.TurnID,
		ItemID:    domaintoolcall.ToolCallItemIDV1(finalContext.TurnID, call.ID),
		CreatedAt: issuedAt.Format(time.RFC3339Nano), Call: call, ReadyCount: 1,
		ToolKind: toolcatalogapp.ToolKind(call.Name), Context: finalContext, Grant: grant,
	})
	if err != nil {
		rev14AccountFlowFail(t, "grant_catalog", counts)
	}
	store.setRev14Thread(finalContext, callItem)
	durable.set(finalContext)
	if manager.ValidateCurrentAccountFlowOuterGrant(ctx, finalContext, grant) != nil ||
		!store.hasRev14ActiveGrant(finalContext, grant) {
		rev14AccountFlowFail(t, "grant_catalog", counts)
	}
	envelope, err := domainmcp.NewHostContextEnvelope(finalContext, grant)
	if err != nil {
		rev14AccountFlowFail(t, "grant_catalog", counts)
	}
	counts.grantCatalog.Store(1)
	if !rev14EffectsZeroBeforeCall(counts) {
		rev14AccountFlowFail(t, "tool_call", counts)
	}

	result := manager.CallToolSecurityBoundContext(ctx, rev14AccountFlowToolName, true, envelope, arguments)
	if result["executed"] == true {
		counts.managerExecuted.Store(1)
	}
	if result["transportStatus"] == "success" {
		counts.managerTransportSuccess.Store(1)
	}
	if result["semanticStatus"] == "success" {
		counts.managerSemanticSuccess.Store(1)
	}
	if result["semanticStatus"] == "partial" {
		counts.managerSemanticPartial.Store(1)
	}
	if lossless, ok := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult); ok && lossless.HostPrivate != nil {
		counts.managerHostPrivate.Store(1)
	}
	if result["executed"] != true || result["transportStatus"] != "success" ||
		result["semanticStatus"] != "success" || counts.accountFlowExecutor.Load() != 1 ||
		counts.nativeAnalyze.Load() != 1 || counts.sourceUse.Load() != 1 || counts.subjectResolve.Load() != 1 ||
		counts.privateSummaryCapture.Load() != 1 || counts.privateRowCapture.Load() != 1 {
		rev14AccountFlowFail(t, "tool_call", counts)
	}
	counts.toolCall.Store(1)
	lossless, ok := result[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	if !ok || !domainmcp.ValidLosslessToolResult(lossless) || lossless.HostPrivate == nil {
		rev14AccountFlowFail(t, "semantic", counts)
	}
	semantic, ok := manager.HostFundsAccountFlowProviderSemanticV1(lossless)
	if !ok || semantic.SubjectAlias != string(alias) || semantic.StartInclusive != arguments["start_inclusive"] ||
		semantic.EndInclusive != arguments["end_inclusive"] || semantic.Currency != "CNY" || semantic.MinorUnitScale != 2 ||
		semantic.InflowMinor != "1250" || semantic.OutflowMinor != "0" || semantic.NetMinor != "1250" ||
		semantic.TransactionCount != 1 || semantic.EvidenceTransactionCount != 1 || semantic.EvidenceRowLimit != 1 ||
		!semantic.AggregateComplete || !semantic.EvidenceRowsComplete ||
		semantic.Currentness != domainnative.AccountFlowProviderCurrentnessCurrentV1 ||
		semantic.Coverage.State != domainnative.AccountFlowCoverageCompleteV1 || len(semantic.Coverage.Gaps) != 0 ||
		semantic.Coverage.NormalizedSnapshotRows != 1 || semantic.Coverage.AcceptedSnapshotRows != 1 ||
		semantic.Coverage.RejectedSnapshotRows != 0 || semantic.Coverage.DuplicateSnapshotRows != 0 ||
		semantic.Coverage.UntimedSubjectRows != 0 || semantic.Coverage.ObservedMatchingRows != 1 ||
		len(semantic.Transactions) != 1 || !domainsecurity.IsSHA256Hex(semantic.QueryHash) ||
		!domainsecurity.IsSHA256Hex(semantic.ResultHash) ||
		domainnative.ValidateAccountFlowProviderSemanticResultV1(semantic, 1, domainevidence.SemanticSuccess) != nil ||
		domainevidence.ValidateAccountFlowTypedSourceFieldReferenceV1(
			semantic.Outcome.SourceFieldReference, semantic.QueryHash, semantic.ResultHash,
		) != nil || semantic.Outcome.SourceFieldReference.Field != domainevidence.AcceptedSlotSourceFieldAccountV1 {
		rev14AccountFlowFail(t, "semantic", counts)
	}
	counts.semantic.Store(1)

	withoutPrivate := lossless
	withoutPrivate.HostPrivate = nil
	if manager.ConsumeHostFundsAccountFlowEvidenceV1(withoutPrivate, rev14NoopSummary, rev14NoopRow) == nil {
		rev14AccountFlowFail(t, "private_evidence", counts)
	}
	captured := &rev14ConsumedEvidence{}
	if err := manager.ConsumeHostFundsAccountFlowEvidenceV1(lossless, captured.consumeSummary, captured.consumeRow); err != nil ||
		captured.summaryCalls != 1 || captured.rowCalls != 1 || captured.queryHash != semantic.QueryHash ||
		captured.resultHash != semantic.ResultHash || captured.inflowMinor != semantic.InflowMinor ||
		captured.outflowMinor != semantic.OutflowMinor || captured.netMinor != semantic.NetMinor ||
		captured.transactionCount != semantic.TransactionCount || captured.evidenceRef != semantic.Transactions[0].EvidenceRef ||
		captured.sourceFileID == "" || captured.sourceRowNumber == 0 {
		rev14AccountFlowFail(t, "private_evidence", counts)
	}
	counts.evidenceConsumer.Store(1)
	if manager.ConsumeHostFundsAccountFlowEvidenceV1(lossless, rev14NoopSummary, rev14NoopRow) == nil {
		rev14AccountFlowFail(t, "private_evidence", counts)
	}
	counts.privateEvidence.Store(1)

	modelOutput := domainnative.AccountFlowProviderModelOutputV1{
		SchemaVersion: 3, Purpose: domainnative.AccountFlowProviderModelPurposeV1,
		SemanticStatus: domainevidence.SemanticSuccess, Data: semantic,
	}
	modelBytes, err := domainnative.CanonicalAccountFlowProviderModelOutputV1(modelOutput)
	if err != nil {
		rev14AccountFlowFail(t, "privacy", counts)
	}
	persistable, ok := toolcatalogapp.PersistableToolOutputForExecution(call, finalContext, grant, result).(map[string]any)
	if !ok {
		rev14AccountFlowFail(t, "privacy", counts)
	}
	if _, exists := persistable[domainmcp.HostRawToolResultKey]; exists {
		rev14AccountFlowFail(t, "privacy", counts)
	}
	if _, exists := persistable[domainmcp.HostEvidenceSettlementCarrierKey]; exists {
		rev14AccountFlowFail(t, "privacy", counts)
	}
	publicBytes, err := json.Marshal(map[string]any{
		"model": json.RawMessage(modelBytes), "persistable": persistable,
	})
	if err != nil || !rev14ValueSafeBytes(
		publicBytes, sourcePath, profileRoot, semantic.Transactions[0].EvidenceRef,
	) {
		rev14AccountFlowFail(t, "privacy", counts)
	}
	counts.privacy.Store(1)

	beforeNegative := counts.effectSnapshot()
	manager.InvalidateHostFundsSourceAfterSettledNativeFailure()
	if len(toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, finalContext)) != 0 ||
		len(manager.LiveToolsForSecurityContext(finalContext)) != 0 {
		rev14AccountFlowFail(t, "negative", counts)
	}
	negative := manager.CallToolSecurityBoundContext(ctx, rev14AccountFlowToolName, true, envelope, arguments)
	if negative["executed"] == true || beforeNegative != counts.effectSnapshot() ||
		len(toolcatalogapp.MCPToolNamesFromAdvertisementsV1(
			toolcatalogapp.ValidMCPToolAdvertisementsForSecurityContextV1(manager, finalContext),
		)) != 0 || !rev14OrdinaryCatalogAvailable() {
		rev14AccountFlowFail(t, "negative", counts)
	}
	counts.negative.Store(1)

	if runtimeAuthority.Close() != nil {
		rev14AccountFlowFail(t, "closure", counts)
	}
	ownerClosed.Store(true)
	counts.closure.Store(1)
	t.Logf("closure owner_open=%d stage=%d confirm=%d dsv2=%d runtime_composition=%d final_tsc_source_ready=%d case_subject=%d grant_catalog=%d tool_call=%d semantic=%d private_evidence=%d privacy=%d negative=%d closure=%d native_analyze=%d native_success=%d native_result_valid=%d post_native_challenge=%d post_native_challenge_success=%d callback_current=%d callback_current_success=%d outer_grant=%d outer_grant_success=%d source_use=%d subject_resolve=%d counterparty_resolve=%d private_summary_capture=%d private_row_capture=%d evidence_consumer=%d",
		counts.ownerOpen.Load(), counts.stage.Load(), counts.confirm.Load(), counts.dsv2.Load(),
		counts.runtimeComposition.Load(), counts.finalTSCSourceReady.Load(), counts.caseSubject.Load(),
		counts.grantCatalog.Load(), counts.toolCall.Load(), counts.semantic.Load(), counts.privateEvidence.Load(),
		counts.privacy.Load(), counts.negative.Load(), counts.closure.Load(), counts.nativeAnalyze.Load(),
		counts.nativeSuccess.Load(), counts.nativeResultValid.Load(), counts.postNativeChallenge.Load(),
		counts.postNativeChallengeSuccess.Load(), counts.callbackCurrent.Load(), counts.callbackCurrentSuccess.Load(),
		counts.outerGrant.Load(), counts.outerGrantSuccess.Load(),
		counts.sourceUse.Load(), counts.subjectResolve.Load(), counts.counterpartyResolve.Load(),
		counts.privateSummaryCapture.Load(), counts.privateRowCapture.Load(), counts.evidenceConsumer.Load())
}

type rev14AccountFlowCounts struct {
	ownerOpen, stage, confirm, dsv2, runtimeComposition atomic.Int32
	finalTSCSourceReady, caseSubject, grantCatalog      atomic.Int32
	toolCall, semantic, privateEvidence, privacy        atomic.Int32
	negative, closure                                   atomic.Int32
	managerExecuted, managerTransportSuccess            atomic.Int32
	managerSemanticSuccess, managerSemanticPartial      atomic.Int32
	managerHostPrivate                                  atomic.Int32
	nativeAnalyze, nativeSuccess, nativeResultValid     atomic.Int32
	postNativeChallenge, postNativeChallengeSuccess     atomic.Int32
	callbackCurrent, callbackCurrentSuccess             atomic.Int32
	outerGrant, outerGrantSuccess                       atomic.Int32
	executorAuthorityInvalid, executorGrantInvalid      atomic.Int32
	executorResultInvalid, executorNativeFailed         atomic.Int32
	executorUnavailable, executorContextFailure         atomic.Int32
	executorUnknownFailure                              atomic.Int32
	sourceRowResolve, sourceRowResolveSuccess           atomic.Int32
	sourceRowNotFound, sourceRowMismatch                atomic.Int32
	sourceRowCorrupt, sourceRowUnavailable              atomic.Int32
	sourceRowUnknownFailure                             atomic.Int32
	accountFlowExecutor, sourceUse                      atomic.Int32
	subjectResolve, counterpartyResolve                 atomic.Int32
	privateSummaryCapture, privateRowCapture            atomic.Int32
	evidenceConsumer                                    atomic.Int32
}

type rev14EffectSnapshot struct {
	nativeAnalyze, nativeSuccess, nativeResultValid               int32
	postNativeChallenge, postNativeChallengeSuccess               int32
	callbackCurrent, callbackCurrentSuccess                       int32
	outerGrant, outerGrantSuccess                                 int32
	accountFlowExecutor, sourceUse, subjectResolve                int32
	sourceRowResolve, sourceRowResolveSuccess                     int32
	sourceRowNotFound, sourceRowMismatch, sourceRowCorrupt        int32
	sourceRowUnavailable, sourceRowUnknownFailure                 int32
	counterpartyResolve, privateSummaryCapture, privateRowCapture int32
	evidenceConsumer                                              int32
}

func (counts *rev14AccountFlowCounts) effectSnapshot() rev14EffectSnapshot {
	return rev14EffectSnapshot{
		nativeAnalyze: counts.nativeAnalyze.Load(), nativeSuccess: counts.nativeSuccess.Load(),
		nativeResultValid:          counts.nativeResultValid.Load(),
		postNativeChallenge:        counts.postNativeChallenge.Load(),
		postNativeChallengeSuccess: counts.postNativeChallengeSuccess.Load(),
		callbackCurrent:            counts.callbackCurrent.Load(), callbackCurrentSuccess: counts.callbackCurrentSuccess.Load(),
		outerGrant: counts.outerGrant.Load(), outerGrantSuccess: counts.outerGrantSuccess.Load(),
		accountFlowExecutor: counts.accountFlowExecutor.Load(),
		sourceUse:           counts.sourceUse.Load(), subjectResolve: counts.subjectResolve.Load(),
		sourceRowResolve: counts.sourceRowResolve.Load(), sourceRowResolveSuccess: counts.sourceRowResolveSuccess.Load(),
		sourceRowNotFound: counts.sourceRowNotFound.Load(), sourceRowMismatch: counts.sourceRowMismatch.Load(),
		sourceRowCorrupt: counts.sourceRowCorrupt.Load(), sourceRowUnavailable: counts.sourceRowUnavailable.Load(),
		sourceRowUnknownFailure: counts.sourceRowUnknownFailure.Load(),
		counterpartyResolve:     counts.counterpartyResolve.Load(),
		privateSummaryCapture:   counts.privateSummaryCapture.Load(), privateRowCapture: counts.privateRowCapture.Load(),
		evidenceConsumer: counts.evidenceConsumer.Load(),
	}
}

func rev14EffectsZeroBeforeCall(counts *rev14AccountFlowCounts) bool {
	return counts != nil && counts.effectSnapshot() == (rev14EffectSnapshot{})
}

type rev14AccountFlowOwner struct {
	*nativecomponenthost.Owner
	counts *rev14AccountFlowCounts
}

func (owner *rev14AccountFlowOwner) AnalyzeAccountFlows(
	ctx context.Context,
	request domainnative.Request,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	if owner == nil || owner.Owner == nil || owner.counts == nil || owner.counts.nativeAnalyze.Add(1) != 1 {
		return domainnative.AnalyzeAccountFlowsResultV1{}, errors.New("rev14 native analyze unavailable")
	}
	result, err := owner.Owner.AnalyzeAccountFlows(ctx, request, descriptor, source)
	if err != nil {
		return result, err
	}
	owner.counts.nativeSuccess.Add(1)
	if request.AccountFlowArguments != nil &&
		domainnative.ValidateAnalyzeAccountFlowsResultV1(result, *request.AccountFlowArguments) == nil {
		owner.counts.nativeResultValid.Add(1)
	}
	return result, nil
}

type rev14KeyedDigester struct{ key []byte }

func (digester rev14KeyedDigester) KeyedPayloadHash(ctx context.Context, purpose string, payload []byte) (string, error) {
	if ctx == nil || ctx.Err() != nil || len(digester.key) == 0 || strings.TrimSpace(purpose) == "" || len(payload) == 0 {
		return "", errors.New("rev14 keyed digest unavailable")
	}
	mac := hmac.New(sha256.New, digester.key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return domainsecurity.SHA256Hex(mac.Sum(nil)), nil
}

type rev14DurableAuthority struct {
	mu       sync.Mutex
	expected domainsecurity.TurnSecurityContext
}

func (authority *rev14DurableAuthority) set(expected domainsecurity.TurnSecurityContext) {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	authority.expected = expected
}

func (authority *rev14DurableAuthority) ValidateCurrent(
	threadID string,
	thread map[string]any,
) (domainsecurity.TurnSecurityContext, error) {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || threadID != authority.expected.ThreadID ||
		strings.TrimSpace(contracts.StringField(thread, "id")) != threadID || current != authority.expected {
		return domainsecurity.TurnSecurityContext{}, errors.New("rev14 durable authority mismatch")
	}
	return current, nil
}

func (store *rev9HealthStore) setRev14Thread(
	securityContext domainsecurity.TurnSecurityContext,
	callItem map[string]any,
) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record := turnsecurityapp.PublicRecord(securityContext)
	store.thread = map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath,
		"securityState": record,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID,
			"status": "running", "securityContext": record, "items": []any{callItem},
		}},
	}
}

func (store *rev9HealthStore) hasRev14ActiveGrant(
	securityContext domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	registry, err := executiongrantapp.RegistryFromThread(
		securityContext.ThreadID, store.thread, securityContext.TurnID,
	)
	return err == nil && domainsecurity.VerifyExecutionGrantMembership(
		registry, securityContext.ThreadID, securityContext.TurnID, grant, domainsecurity.GrantRegistryActive,
	) == nil
}

func rev14CountCurrentSource(
	counts *rev14AccountFlowCounts,
	delegate nativecomponentapp.UseCurrentAccountFlowSource,
) nativecomponentapp.UseCurrentAccountFlowSource {
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(
			context.Context,
			domainfundsquerysource.DescriptorV1,
			fundsquerysourceport.ExactReadLease,
			domainnative.AccountFlowSourceRowResolverV1,
			func(context.Context, domainsecurity.TurnSecurityContext, domainfundsquerysource.DescriptorV1, func(context.Context) error) error,
		) error,
	) error {
		counts.sourceUse.Add(1)
		return delegate(
			ctx,
			securityContext,
			func(
				sourceCtx context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				source fundsquerysourceport.ExactReadLease,
				resolveRow domainnative.AccountFlowSourceRowResolverV1,
				revalidate func(context.Context, domainsecurity.TurnSecurityContext, domainfundsquerysource.DescriptorV1, func(context.Context) error) error,
			) error {
				countedResolveRow := func(
					sourceFileID string,
					sourceRowNumber uint64,
					consume func(string) error,
				) error {
					counts.sourceRowResolve.Add(1)
					err := resolveRow(sourceFileID, sourceRowNumber, consume)
					if err == nil {
						counts.sourceRowResolveSuccess.Add(1)
						return nil
					}
					switch {
					case errors.Is(err, fundsquerysourceport.ErrNotFound):
						counts.sourceRowNotFound.Add(1)
					case errors.Is(err, fundsquerysourceport.ErrMismatch):
						counts.sourceRowMismatch.Add(1)
					case errors.Is(err, fundsquerysourceport.ErrCorrupt):
						counts.sourceRowCorrupt.Add(1)
					case errors.Is(err, fundsquerysourceport.ErrUnavailable):
						counts.sourceRowUnavailable.Add(1)
					default:
						counts.sourceRowUnknownFailure.Add(1)
					}
					return err
				}
				return use(
					sourceCtx,
					descriptor,
					source,
					countedResolveRow,
					func(
						challengeCtx context.Context,
						current domainsecurity.TurnSecurityContext,
						currentDescriptor domainfundsquerysource.DescriptorV1,
						continuation func(context.Context) error,
					) error {
						counts.postNativeChallenge.Add(1)
						err := revalidate(challengeCtx, current, currentDescriptor, continuation)
						if err == nil {
							counts.postNativeChallengeSuccess.Add(1)
						}
						return err
					},
				)
			},
		)
	}
}

func rev14CountSubject(
	counts *rev14AccountFlowCounts,
	delegate nativecomponentapp.ResolveAccountFlowSubject,
) nativecomponentapp.ResolveAccountFlowSubject {
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		descriptor domainfundsquerysource.DescriptorV1,
		alias domaincaseentity.ModelEntityAliasV1,
		validate nativecomponentapp.ValidateCurrentAccountFlowCallback,
		use func(nativecomponentapp.AccountFlowSubjectResolutionV1) error,
	) error {
		counts.subjectResolve.Add(1)
		return delegate(ctx, securityContext, descriptor, alias, validate, use)
	}
}

func rev14CountCounterparty(
	counts *rev14AccountFlowCounts,
	delegate nativecomponentapp.ResolveAccountFlowCounterparty,
) nativecomponentapp.ResolveAccountFlowCounterparty {
	return func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		descriptor domainfundsquerysource.DescriptorV1,
		sourceExactAccount string,
		bankInstitution string,
		validate nativecomponentapp.ValidateCurrentAccountFlowCallback,
		use func(domaincaseentity.ReferenceV1, domaincaseentity.DisplayLabelV1) error,
	) error {
		counts.counterpartyResolve.Add(1)
		return delegate(ctx, securityContext, descriptor, sourceExactAccount, bankInstitution, validate, use)
	}
}

func rev14CountAccountFlowExecutor(
	counts *rev14AccountFlowCounts,
	delegate mcp.AccountFlowExecutor,
) mcp.AccountFlowExecutor {
	return func(
		ctx context.Context,
		input mcp.AccountFlowExecutionInput,
		consumeSummary domainnative.AccountFlowHostEvidenceSummaryConsumerV1,
		consumeRow domainnative.AccountFlowHostEvidenceRowConsumerV1,
	) (domainnative.AccountFlowProviderSemanticResultV1, error) {
		counts.accountFlowExecutor.Add(1)
		semantic, err := delegate(
			ctx,
			input,
			func(
				caseEntityRef, datasetSnapshotID string,
				contextEpoch uint64,
				contextDigest, caseBindingHash, startInclusive, endInclusive, timezone, currency string,
				minorUnitScale uint8,
				inflowMinor, outflowMinor, netMinor string,
				transactionCount uint64,
				aggregateComplete, evidenceRowsComplete bool,
				evidenceTransactionCount uint64,
				coverage domainnative.AccountFlowProviderSemanticCoverageV1,
				queryHash, resultHash string,
			) error {
				counts.privateSummaryCapture.Add(1)
				return consumeSummary(
					caseEntityRef, datasetSnapshotID, contextEpoch, contextDigest, caseBindingHash,
					startInclusive, endInclusive, timezone, currency, minorUnitScale,
					inflowMinor, outflowMinor, netMinor, transactionCount, aggregateComplete,
					evidenceRowsComplete, evidenceTransactionCount, coverage, queryHash, resultHash,
				)
			},
			func(
				index int,
				caseEntityRef, evidenceRef, sourceFileID string,
				sourceRowNumber uint64,
				occurredAt, direction, amountMinor, currency string,
				minorUnitScale uint8,
			) error {
				counts.privateRowCapture.Add(1)
				return consumeRow(
					index, caseEntityRef, evidenceRef, sourceFileID, sourceRowNumber,
					occurredAt, direction, amountMinor, currency, minorUnitScale,
				)
			},
		)
		if err != nil {
			switch {
			case errors.Is(err, nativecomponentapp.ErrAuthorityInvalid):
				counts.executorAuthorityInvalid.Add(1)
			case errors.Is(err, nativecomponentapp.ErrGrantInvalid):
				counts.executorGrantInvalid.Add(1)
			case errors.Is(err, nativecomponentapp.ErrResultInvalid):
				counts.executorResultInvalid.Add(1)
			case errors.Is(err, nativecomponentapp.ErrAccountFlowExecutionFailed):
				counts.executorNativeFailed.Add(1)
			case errors.Is(err, nativecomponentapp.ErrUnavailable):
				counts.executorUnavailable.Add(1)
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				counts.executorContextFailure.Add(1)
			default:
				counts.executorUnknownFailure.Add(1)
			}
		}
		return semantic, err
	}
}

type rev14ConsumedEvidence struct {
	summaryCalls, rowCalls              int
	queryHash, resultHash               string
	inflowMinor, outflowMinor, netMinor string
	transactionCount                    uint64
	evidenceRef, sourceFileID           string
	sourceRowNumber                     uint64
}

func (capture *rev14ConsumedEvidence) consumeSummary(
	_ string,
	_ string,
	_ uint64,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
	_ uint8,
	inflowMinor, outflowMinor, netMinor string,
	transactionCount uint64,
	_ bool,
	_ bool,
	_ uint64,
	_ domainnative.AccountFlowProviderSemanticCoverageV1,
	queryHash, resultHash string,
) error {
	capture.summaryCalls++
	capture.inflowMinor, capture.outflowMinor, capture.netMinor = inflowMinor, outflowMinor, netMinor
	capture.transactionCount, capture.queryHash, capture.resultHash = transactionCount, queryHash, resultHash
	return nil
}

func (capture *rev14ConsumedEvidence) consumeRow(
	index int,
	_ string,
	evidenceRef string,
	sourceFileID string,
	sourceRowNumber uint64,
	_ string,
	_ string,
	_ string,
	_ string,
	_ uint8,
) error {
	if index != 0 {
		return errors.New("rev14 evidence row index mismatch")
	}
	capture.rowCalls++
	capture.evidenceRef, capture.sourceFileID, capture.sourceRowNumber = evidenceRef, sourceFileID, sourceRowNumber
	return nil
}

func rev14NoopSummary(
	string, string, uint64, string, string, string, string, string, string, uint8,
	string, string, string, uint64, bool, bool, uint64,
	domainnative.AccountFlowProviderSemanticCoverageV1, string, string,
) error {
	return nil
}

func rev14NoopRow(int, string, string, string, uint64, string, string, string, string, uint8) error {
	return nil
}

func rev14ValueSafeBytes(body []byte, sourcePath string, profileRoot string, evidenceRef string) bool {
	lower := bytes.ToLower(body)
	for _, forbidden := range []string{
		rev14PrivateAccount, rev14PrivateCard, rev14PrivateCounterparty, rev14PrivateSourceSentinel,
		"cer1_", "duckdb", "select ", " from ", "raw row", "authorityentityref",
		sourcePath, filepath.Base(sourcePath), profileRoot,
	} {
		if bytes.Contains(lower, bytes.ToLower([]byte(forbidden))) {
			return false
		}
	}
	return bytes.Contains(body, []byte(`"subjectAlias":"acct:1"`)) &&
		bytes.Contains(body, []byte(`"sourceFieldReference"`)) &&
		strings.TrimSpace(evidenceRef) != "" && bytes.Contains(body, []byte(evidenceRef))
}

func rev14OrdinaryCatalogAvailable() bool {
	return len(toolcatalogapp.BuiltinToolSchemas(toolcatalogapp.BuiltinToolSchemaInput{})) > 0
}

func rev14AccountFlowCSV() []byte {
	columns := []string{
		"交易卡号", "交易账号", "账户开户名称", "开户人证件号码", "交易时间", "交易金额", "交易余额", "收付标志",
		"交易对手账卡号", "现金标志", "对手户名", "对手身份证号", "对手开户银行", "摘要说明", "交易币种", "交易网点名称",
		"交易网点代码", "交易发生地", "交易是否成功", "传票号", "终端号", "IP地址", "MAC地址", "对手交易余额",
		"交易流水号", "日志号", "凭证种类", "凭证号", "交易柜员号", "商户名称", "商户号", "备注", "交易类型", "查询反馈结果原因",
	}
	row := make([]string, len(columns))
	row[0], row[1], row[2], row[3] = rev14PrivateCard, rev14PrivateAccount, "合成账户甲", "SYNTHID0001"
	row[4], row[5], row[6], row[7] = "2026-08-27 10:00:00", "12.5", "1000", "进"
	row[8], row[9], row[10], row[11] = rev14PrivateCounterparty, "否", "合成对手甲", "SYNTHCPID001"
	row[12], row[13], row[14] = "合成银行", "合成交易", "CNY"
	row[31] = rev14PrivateSourceSentinel
	return []byte(strings.Join(columns, ",") + "\r\n" + strings.Join(row, ",") + "\r\n")
}

func rev14AccountFlowFail(t *testing.T, phase string, counts *rev14AccountFlowCounts) {
	t.Helper()
	if counts == nil {
		counts = &rev14AccountFlowCounts{}
	}
	t.Fatalf("%s owner_open=%d stage=%d confirm=%d dsv2=%d runtime_composition=%d final_tsc_source_ready=%d case_subject=%d grant_catalog=%d tool_call=%d semantic=%d private_evidence=%d privacy=%d negative=%d closure=%d manager_executed=%d manager_transport_success=%d manager_semantic_success=%d manager_semantic_partial=%d manager_host_private=%d native_analyze=%d native_success=%d native_result_valid=%d post_native_challenge=%d post_native_challenge_success=%d callback_current=%d callback_current_success=%d outer_grant=%d outer_grant_success=%d account_flow_executor=%d executor_authority_invalid=%d executor_grant_invalid=%d executor_result_invalid=%d executor_native_failed=%d executor_unavailable=%d executor_context_failure=%d executor_unknown_failure=%d source_use=%d source_row_resolve=%d source_row_success=%d source_row_not_found=%d source_row_mismatch=%d source_row_corrupt=%d source_row_unavailable=%d source_row_unknown=%d subject_resolve=%d counterparty_resolve=%d private_summary_capture=%d private_row_capture=%d evidence_consumer=%d",
		phase, counts.ownerOpen.Load(), counts.stage.Load(), counts.confirm.Load(), counts.dsv2.Load(),
		counts.runtimeComposition.Load(), counts.finalTSCSourceReady.Load(), counts.caseSubject.Load(),
		counts.grantCatalog.Load(), counts.toolCall.Load(), counts.semantic.Load(), counts.privateEvidence.Load(),
		counts.privacy.Load(), counts.negative.Load(), counts.closure.Load(), counts.managerExecuted.Load(),
		counts.managerTransportSuccess.Load(), counts.managerSemanticSuccess.Load(), counts.managerSemanticPartial.Load(),
		counts.managerHostPrivate.Load(), counts.nativeAnalyze.Load(),
		counts.nativeSuccess.Load(), counts.nativeResultValid.Load(), counts.postNativeChallenge.Load(),
		counts.postNativeChallengeSuccess.Load(), counts.callbackCurrent.Load(), counts.callbackCurrentSuccess.Load(),
		counts.outerGrant.Load(), counts.outerGrantSuccess.Load(),
		counts.accountFlowExecutor.Load(), counts.executorAuthorityInvalid.Load(), counts.executorGrantInvalid.Load(),
		counts.executorResultInvalid.Load(), counts.executorNativeFailed.Load(), counts.executorUnavailable.Load(),
		counts.executorContextFailure.Load(), counts.executorUnknownFailure.Load(), counts.sourceUse.Load(),
		counts.sourceRowResolve.Load(), counts.sourceRowResolveSuccess.Load(), counts.sourceRowNotFound.Load(),
		counts.sourceRowMismatch.Load(), counts.sourceRowCorrupt.Load(), counts.sourceRowUnavailable.Load(),
		counts.sourceRowUnknownFailure.Load(), counts.subjectResolve.Load(),
		counts.counterpartyResolve.Load(), counts.privateSummaryCapture.Load(), counts.privateRowCapture.Load(),
		counts.evidenceConsumer.Load())
}
