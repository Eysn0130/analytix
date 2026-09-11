package turnsecurity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
)

type datasetSnapshotAuthorityStub struct {
	err            error
	mutate         func(*domainsecurity.DatasetSnapshotAuthorityRecordV1)
	mutateV2       func(*datasetsnapshotport.ResolvedSnapshotV2)
	resolveV2Calls int
}

func (stub *datasetSnapshotAuthorityStub) ResolveWitnessed(_ context.Context, input datasetsnapshotport.ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	if stub == nil || stub.err != nil {
		if stub != nil && stub.err != nil {
			return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, stub.err
		}
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.New("dataset snapshot test authority unavailable")
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x63}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	observation := input.Observation
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: domainsecurity.SHA256Hex([]byte("turn-security-test-installation")),
		TenantID:       input.TenantID, UserID: input.UserID,
		WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
		CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest:" + observation.CaseBindingHash)),
		RawManifestSHA256:  domainsecurity.SHA256Hex([]byte("raw-manifest:" + observation.CaseBindingHash)),
		ParserVersion:      "turn-security-test-parser/v1", AcceptedAt: time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	if stub.mutate != nil {
		stub.mutate(&record)
	}
	return record, nil
}

func (stub *datasetSnapshotAuthorityStub) ResolveWitnessedV2(_ context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if stub != nil {
		stub.resolveV2Calls++
	}
	if stub == nil || stub.err != nil {
		if stub != nil && stub.err != nil {
			return datasetsnapshotport.ResolvedSnapshotV2{}, stub.err
		}
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.New("dataset snapshot test authority unavailable")
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x63}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		TenantID: input.TenantID, UserID: input.UserID, Observation: input.Observation,
		Material:       "turn-security:" + input.Observation.CaseBindingHash,
		InstallationID: domainsecurity.SHA256Hex([]byte("turn-security-test-installation")),
		AcceptedAt:     time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if input.ExpectedDatasetSnapshotID != "" && input.ExpectedDatasetSnapshotID != resolved.Record.DatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	if stub.mutateV2 != nil {
		stub.mutateV2(&resolved)
	}
	return resolved, nil
}

func datasetSnapshotRecordForTest(t *testing.T, authority *datasetSnapshotAuthorityStub, observer turnSecurityWorkspaceStub) datasetsnapshotport.ResolvedSnapshotV2 {
	t.Helper()
	observation, err := observer.Observe("/workspace/case-a")
	if err != nil {
		t.Fatal(err)
	}
	record, err := authority.ResolveWitnessedV2(context.Background(), datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestFreezeWorkspaceV2UsesAcceptedEpochAndBlocksWithoutSnapshotAuthority(t *testing.T) {
	bindingHash := domainsecurity.SHA256Hex([]byte("binding-a"))
	authority := &riskPolicyAuthorityStub{}
	frozen, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer:      turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case_a", CaseBindingHash: bindingHash}},
			RiskAuthority: authority,
		},
		Thread:   map[string]any{"contextEpochState": turnSecurityEpochStateForTest(t, "thread_1", 4)},
		ThreadID: "thread_1", TurnID: "turn_1", Workspace: "/workspace/case-a",
		Principal:         testIdentityPrincipal(),
		DatasetSnapshotID: "caller-forged-snapshot", IssuedAt: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Version != domainsecurity.TurnSecurityContextVersionV2 || frozen.ContextEpoch != 4 ||
		!domainsecurity.TurnSecurityContextIsBoundaryOnly(frozen) || frozen.DatasetSnapshotID != domainsecurity.NoDatasetSnapshotID ||
		frozen.SourceManifestHash != domainsecurity.EmptySourceManifestHash ||
		frozen.PublicationPolicy.BlockerCode != domainsecurity.PublicationBlockerDatasetSnapshotUnavailable {
		t.Fatalf("unknown source state was promoted or epoch diverged: %#v", frozen)
	}
	if err := domainsecurity.ValidateTurnSecurityContextForExecution(frozen); err == nil {
		t.Fatal("missing host snapshot authority authorized execution")
	}
}

type turnSecurityWorkspaceStub struct {
	binding domainsecurity.CaseBinding
	state   string
	path    string
	err     error
}

type turnSecurityProbeStub struct {
	probe    domainsecurity.VerifiedSourceProbe
	response *domainsecurity.SourceProbeResponse
	err      error
	calls    int
}

func (stub *turnSecurityProbeStub) ProbeCaseSource(_ context.Context, input sourceprobeport.Input) (domainsecurity.VerifiedSourceProbe, error) {
	stub.calls++
	if stub.err != nil {
		return domainsecurity.VerifiedSourceProbe{}, stub.err
	}
	if stub.response != nil {
		serverIdentity, identityErr := domainsecurity.NewVerifiedMCPServerIdentity(input.ServerID, stub.response.ServerName, stub.response.ServerVersion, domainsecurity.SHA256Hex([]byte("turn-security-test-instance")), 3)
		if identityErr != nil {
			return domainsecurity.VerifiedSourceProbe{}, identityErr
		}
		probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
			ServerID: input.ServerID, ServerIdentity: serverIdentity, ConnectionEpoch: 3,
			CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog")), SpecFingerprint: domainsecurity.SHA256Hex([]byte("spec")),
			ThreadID: input.ThreadID, TurnID: input.TurnID, ContextEpoch: input.ContextEpoch, ContextDigest: input.ContextDigest,
			DatasetSnapshotID: input.DatasetSnapshotID, CheckedAt: time.Now().UTC(), Response: *stub.response,
		})
		stub.probe = probe
		return probe, err
	}
	return stub.probe, stub.err
}

func (stub *turnSecurityProbeStub) ServerDiagnostics() []any {
	verifiedIdentity, _ := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix_funds", "0.16.15", domainsecurity.SHA256Hex([]byte("turn-security-test-instance")), 3)
	return []any{map[string]any{
		"id": "analytix_funds", "connected": true, "available": true, "connectionEpoch": float64(3),
		"catalogFingerprint": domainsecurity.SHA256Hex([]byte("catalog")), "specFingerprint": domainsecurity.SHA256Hex([]byte("spec")),
		"expectedServerName": "analytix_funds", "expectedServerVersion": "0.16.15", "identitySource": "installed-plugin-manifest",
		"verifiedServerIdentity": verifiedIdentity,
		"toolNames":              []any{"mcp__analytix_funds__get_case_status"}, "sourceReady": stub.err == nil && stub.probe.ProbeDigest != "",
		"sourceCaseId": stub.probe.CaseID, "datasetSnapshotId": stub.probe.DatasetSnapshotID,
		"sourceProbeDigest": stub.probe.ProbeDigest,
	}}
}

func (stub turnSecurityWorkspaceStub) ReadOptional(string) (domainsecurity.CaseBinding, bool, error) {
	return stub.binding, stub.binding.CaseID != "", nil
}

func (stub turnSecurityWorkspaceStub) WorkspaceRealPath(string) (string, error) {
	if stub.path != "" {
		return stub.path, stub.err
	}
	return "/workspace/case-a", stub.err
}

func (stub turnSecurityWorkspaceStub) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	if stub.err != nil {
		return domainsecurity.CaseBindingObservationV1{}, stub.err
	}
	workspace := stub.path
	if workspace == "" {
		workspace = "/workspace/case-a"
	}
	state := stub.state
	if state == "" {
		state = domainsecurity.CaseBindingStateMissing
		if stub.binding.CaseID != "" {
			state = domainsecurity.CaseBindingStateValid
		}
	}
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: state}
	if state == domainsecurity.CaseBindingStateValid {
		input.CaseID = stub.binding.CaseID
		input.BindingSHA256 = stub.binding.BindingSHA256
		if input.BindingSHA256 == "" {
			input.BindingSHA256 = domainsecurity.SHA256Hex([]byte("binding-body:" + stub.binding.CaseID))
		}
		input.CaseBindingHash = stub.binding.CaseBindingHash
	}
	return domainsecurity.NewCaseBindingObservationV1(input)
}

func TestSourceEpochMismatchBlocksPublish(t *testing.T) {
	bindingA := domainsecurity.SHA256Hex([]byte("binding-a"))
	bindingB := domainsecurity.SHA256Hex([]byte("binding-b"))
	authority := &riskPolicyAuthorityStub{}
	frozen, freezeErr := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer:      turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case_a", CaseBindingHash: bindingA}},
			RiskAuthority: authority,
		}, Thread: map[string]any{}, ThreadID: "thread_1", TurnID: "turn_1", Workspace: "/workspace/case-a",
		Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if freezeErr != nil {
		t.Fatal(freezeErr)
	}
	err := ValidateCurrent(CurrentValidationInput{Identity: testIdentityAuthority(),
		Observer:      turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case_b", CaseBindingHash: bindingB}},
		RiskAuthority: authority, Context: frozen, Workspace: frozen.WorkspaceRealPath,
	})
	if err == nil || err.Error() != "turn_security_case_binding_mismatch" {
		t.Fatalf("case epoch mismatch must block publication and tool settlement: %v", err)
	}
}

func TestFreezeAndAttachStartPersistsAcceptedEpochWithoutAdvancingIt(t *testing.T) {
	now := time.Now().UTC()
	binding := domainsecurity.SHA256Hex([]byte("binding-a"))
	authority := &riskPolicyAuthorityStub{}
	observer := turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case_a", CaseBindingHash: binding}}
	previous, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority},
		Thread:   map[string]any{"contextEpochState": turnSecurityEpochStateForTest(t, "thread_1", 2)},
		ThreadID: "thread_1", TurnID: "turn_0", Workspace: "/workspace/case-a",
		Principal: testIdentityPrincipal(), IssuedAt: now.Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{"securityState": PublicRecord(previous), "contextEpochState": turnSecurityEpochStateForTest(t, "thread_1", 2)}
	turn, event, patch := map[string]any{}, map[string]any{}, map[string]any{}
	frozenContext, err := FreezeAndAttachStartV2(context.Background(), WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority}, nil, thread, "thread_1", "turn_1", "/workspace/case-a", now, turn, event, patch)
	if err != nil {
		t.Fatal(err)
	}
	if frozenContext.ContextEpoch != 2 || turn["securityContext"] == nil || event["securityContext"] == nil || patch["securityState"] == nil {
		t.Fatalf("turn security context was not frozen and persisted: %#v %#v %#v %#v", frozenContext, turn, event, patch)
	}
	if frozenContext.TenantID != domainsecurity.LocalTenantID || frozenContext.UserID != domainsecurity.LocalUserID {
		t.Fatalf("production turn identity must be host-issued local authority: %#v", frozenContext)
	}
}

func TestValidateCurrentAcceptsExplicitUnboundCase(t *testing.T) {
	authority := &riskPolicyAuthorityStub{}
	observer := turnSecurityWorkspaceStub{}
	frozen, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority},
		Thread: map[string]any{}, ThreadID: "thread_1", TurnID: "turn_1", Workspace: "/workspace/case-a",
		Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCurrent(CurrentValidationInput{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority, Context: frozen, Workspace: frozen.WorkspaceRealPath}); err != nil {
		t.Fatalf("unbound case must use the same explicit host sentinel at freeze and validation: %v", err)
	}
}

func TestSourceManifestChangesWhenMCPReconnectsWithSameCatalog(t *testing.T) {
	firstIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("manifest-runtime-a")), 1)
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"id": "analytix_funds", "connected": true, "available": true,
		"catalogFingerprint": "same-catalog", "specFingerprint": "same-spec",
		"expectedServerName": "analytix_funds", "expectedServerVersion": "1.0.0", "identitySource": "installed-plugin-manifest",
		"toolNames": []any{"mcp__analytix_funds__count_case_rows"}, "connectionEpoch": float64(1), "verifiedServerIdentity": firstIdentity,
	}
	first := SourceManifestHash([]any{base})
	base["connectionEpoch"] = float64(2)
	second := SourceManifestHash([]any{base})
	if first == second {
		t.Fatal("same-catalog reconnect must change the frozen source manifest")
	}
	secondIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("manifest-runtime-b")), 1)
	if err != nil {
		t.Fatal(err)
	}
	base["connectionEpoch"] = float64(1)
	base["verifiedServerIdentity"] = secondIdentity
	if restarted := SourceManifestHash([]any{base}); restarted == first {
		t.Fatal("same numeric epoch from another runtime instance reused the frozen source manifest")
	}
}

func TestSourceManifestExcludesTurnProbeStateToAvoidContextDigestCycle(t *testing.T) {
	verifiedIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("manifest-test-instance")), 3)
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{
		"id": "analytix_funds", "connected": true, "available": true, "connectionEpoch": float64(3),
		"catalogFingerprint": domainsecurity.SHA256Hex([]byte("catalog")), "specFingerprint": domainsecurity.SHA256Hex([]byte("spec")),
		"expectedServerName": "analytix_funds", "expectedServerVersion": "1.0.0", "identitySource": "manifest",
		"toolNames": []any{"mcp__analytix_funds__query"}, "verifiedServerIdentity": verifiedIdentity,
	}
	baseline := SourceManifestHash([]any{record})
	record["sourceReady"] = true
	record["sourceCaseId"] = "case_a"
	record["datasetSnapshotId"] = "snapshot_a"
	record["sourceProbeDigest"] = domainsecurity.SHA256Hex([]byte("probe"))
	if changed := SourceManifestHash([]any{record}); changed != baseline {
		t.Fatalf("turn-local probe state reintroduced a context/probe digest cycle: before=%s after=%s", baseline, changed)
	}
}

func TestFreezeAndAttachStartUsesOnlyMatchingVerifiedDatasetProbe(t *testing.T) {
	now := time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("binding"))
	observer := turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case_1234", CaseBindingHash: bindingHash}}
	snapshotAuthority := &datasetSnapshotAuthorityStub{}
	observation, err := observer.Observe("/workspace/case-a")
	if err != nil {
		t.Fatal(err)
	}
	snapshotRecord, err := snapshotAuthority.ResolveWitnessedV2(context.Background(), datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := domainsecurity.SourceProbeResponse{
		Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.15", CaseID: "case_1234",
		CaseBindingHash: bindingHash, DatasetSnapshotID: snapshotRecord.Record.DatasetSnapshotID,
		Ready: true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
	}
	source := &turnSecurityProbeStub{response: &response}
	turn, event, patch := map[string]any{}, map[string]any{}, map[string]any{}
	authority := &riskPolicyAuthorityStub{}
	securityAuthority := WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority, SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority}
	result, err := FreezeAndAttachStartV2Result(context.Background(), securityAuthority, source, map[string]any{}, "thread_1", "turn_1", "/workspace/case-a", now, turn, event, patch)
	if err != nil {
		t.Fatal(err)
	}
	frozen := result.SecurityContext
	if source.calls != 1 || !result.SourceReady || frozen.DatasetSnapshotID != snapshotRecord.Record.DatasetSnapshotID ||
		frozen.SourceManifestHash != snapshotRecord.Record.SourceManifestHash {
		t.Fatalf("matching discovery probe did not confirm the host snapshot: calls=%d result=%#v", source.calls, result)
	}
	if source.probe.DatasetSnapshotID != frozen.DatasetSnapshotID {
		t.Fatalf("discovery probe did not echo the frozen snapshot: probe=%#v context=%#v", source.probe, frozen)
	}
	source.response.CaseBindingHash = domainsecurity.SHA256Hex([]byte("other-binding"))
	result, err = FreezeAndAttachStartV2Result(context.Background(), WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: &riskPolicyAuthorityStub{}, SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority}, source, map[string]any{}, "thread_2", "turn_2", "/workspace/case-a", now, map[string]any{}, map[string]any{}, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceReady || result.SecurityContext.DatasetSnapshotID != snapshotRecord.Record.DatasetSnapshotID {
		t.Fatalf("mismatched/tampered source probe changed host snapshot authority: %#v", result)
	}
	source.err = errors.New("offline")
	result, err = FreezeAndAttachStartV2Result(context.Background(), WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: &riskPolicyAuthorityStub{}, SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority}, source, map[string]any{}, "thread_3", "turn_3", "/workspace/case-a", now, map[string]any{}, map[string]any{}, map[string]any{})
	if err != nil || result.SourceReady || result.SecurityContext.DatasetSnapshotID != snapshotRecord.Record.DatasetSnapshotID {
		t.Fatalf("offline probe changed the frozen snapshot instead of downgrading readiness: result=%#v err=%v", result, err)
	}
}

func TestValidateCurrentInsideExactDatasetCapabilityKeepsLiveChecksWithoutResolvingInventory(t *testing.T) {
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("callback-currentness-binding"))
	observer := turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{
		CaseID: "case_callback_currentness", CaseBindingHash: bindingHash,
	}}
	snapshotAuthority := &datasetSnapshotAuthorityStub{}
	observation, err := observer.Observe("/workspace/case-callback-currentness")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotAuthority.ResolveWitnessedV2(
		context.Background(),
		datasetsnapshotport.ResolveInputV2{
			TenantID:    domainsecurity.LocalTenantID,
			UserID:      domainsecurity.LocalUserID,
			Observation: observation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	probe := &turnSecurityProbeStub{response: &domainsecurity.SourceProbeResponse{
		Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.16",
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID: snapshot.Record.DatasetSnapshotID,
		Ready:             true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
	}}
	riskAuthority := &riskPolicyAuthorityStub{}
	result, err := FreezeAndAttachStartV2Result(
		context.Background(),
		WorkspaceSecurityAuthority{
			Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: riskAuthority,
			SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority,
		},
		probe,
		map[string]any{},
		"thread-callback-currentness",
		"turn-callback-currentness",
		observation.WorkspaceRealPath,
		now,
		map[string]any{},
		map[string]any{},
		map[string]any{},
	)
	if err != nil || !result.SourceReady ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(result.SecurityContext) != nil {
		t.Fatalf("callback currentness fixture did not freeze a case fact context: result=%#v err=%v", result, err)
	}
	resolveCallsBeforeValidation := snapshotAuthority.resolveV2Calls
	input := CurrentValidationInput{
		OperationContext:    context.Background(),
		Identity:            testIdentityAuthority(),
		Observer:            observer,
		RiskAuthority:       riskAuthority,
		SnapshotAuthorityV2: snapshotAuthority,
		Context:             result.SecurityContext,
		Workspace:           result.SecurityContext.WorkspaceRealPath,
	}
	if err := ValidateCurrentInsideExactDatasetCapability(input); err != nil {
		t.Fatalf("exact callback currentness rejected healthy live authority: %v", err)
	}
	if snapshotAuthority.resolveV2Calls != resolveCallsBeforeValidation {
		t.Fatalf(
			"exact callback currentness repeated DSV2 inventory: before=%d after=%d",
			resolveCallsBeforeValidation,
			snapshotAuthority.resolveV2Calls,
		)
	}

	for _, test := range []struct {
		name   string
		mutate func(*CurrentValidationInput)
	}{
		{name: "principal", mutate: func(input *CurrentValidationInput) { input.Identity = nil }},
		{name: "risk", mutate: func(input *CurrentValidationInput) { input.RiskAuthority = &riskPolicyAuthorityStub{} }},
		{name: "case binding", mutate: func(input *CurrentValidationInput) {
			input.Observer = turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{
				CaseID: "case_other", CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")),
			}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := input
			test.mutate(&current)
			if currentErr := ValidateCurrentInsideExactDatasetCapability(current); currentErr == nil {
				t.Fatal("callback currentness accepted drifted live authority")
			}
			if snapshotAuthority.resolveV2Calls != resolveCallsBeforeValidation {
				t.Fatal("drifted callback currentness fell back to DSV2 inventory resolution")
			}
		})
	}
}

func TestFirstCaseBoundTSCRegistersBeforeLiveProbe(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	bindingHash := domainsecurity.SHA256Hex([]byte("binding-order"))
	observer := turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case-order", CaseBindingHash: bindingHash}}
	snapshotAuthority := &datasetSnapshotAuthorityStub{}
	snapshotRecord := datasetSnapshotRecordForTest(t, snapshotAuthority, observer)
	response := domainsecurity.SourceProbeResponse{
		Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.15", CaseID: "case-order",
		CaseBindingHash: bindingHash, DatasetSnapshotID: snapshotRecord.Record.DatasetSnapshotID,
		Ready: true, ReadOnly: true, CheckedAt: now.Format(time.RFC3339Nano),
	}
	source := &turnSecurityProbeStub{response: &response}
	hookCalled := false
	_, err := FreezeAndAttachStartV2(
		context.Background(), WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: &riskPolicyAuthorityStub{}, SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority},
		source, map[string]any{}, "thread-order", "turn-order", "/workspace/case-a", now, map[string]any{}, map[string]any{}, map[string]any{},
		func(_ context.Context, frozen domainsecurity.TurnSecurityContext) error {
			hookCalled = true
			if source.calls != 0 || frozen.CaseID != "case-order" {
				return errors.New("probe ran before host case-thread registration")
			}
			return nil
		},
	)
	if err != nil || !hookCalled || source.calls != 1 {
		t.Fatalf("case TSC/probe order mismatch: hook=%v calls=%d err=%v", hookCalled, source.calls, err)
	}
}

func TestCaseThreadAuthorityWriteFailurePreventsProbeAndStartAttachment(t *testing.T) {
	bindingHash := domainsecurity.SHA256Hex([]byte("binding-write-failure"))
	observer := turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case-write-failure", CaseBindingHash: bindingHash}}
	snapshotAuthority := &datasetSnapshotAuthorityStub{}
	snapshotRecord := datasetSnapshotRecordForTest(t, snapshotAuthority, observer)
	source := &turnSecurityProbeStub{response: &domainsecurity.SourceProbeResponse{
		Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.15", CaseID: "case-write-failure",
		CaseBindingHash: bindingHash, DatasetSnapshotID: snapshotRecord.Record.DatasetSnapshotID,
		Ready: true, ReadOnly: true, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}
	turn, event, patch := map[string]any{}, map[string]any{}, map[string]any{}
	_, err := FreezeAndAttachStartV2(
		context.Background(), WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: &riskPolicyAuthorityStub{}, SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority},
		source, map[string]any{}, "thread-write-failure", "turn-write-failure", "/workspace/case-a", time.Now().UTC(), turn, event, patch,
		func(context.Context, domainsecurity.TurnSecurityContext) error {
			return errors.New("authority store unavailable")
		},
	)
	if err == nil || source.calls != 0 || len(turn) != 0 || len(event) != 0 || len(patch) != 0 {
		t.Fatalf("failed authority write reached probe/start records: calls=%d turn=%#v event=%#v patch=%#v err=%v", source.calls, turn, event, patch, err)
	}
}

func TestCallerCannotOverrideRiskOrPublicationPolicy(t *testing.T) {
	authority := &riskPolicyAuthorityStub{}
	frozen, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: turnSecurityWorkspaceStub{}, RiskAuthority: authority},
		Thread: map[string]any{}, ThreadID: "thread-caller-policy", TurnID: "turn-caller-policy", Workspace: "/workspace/case-a",
		Principal: testIdentityPrincipal(), DatasetSnapshotID: "caller-snapshot",
		SourceDiagnostics: []any{map[string]any{"id": "host-source"}}, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !domainsecurity.TurnSecurityContextIsGeneral(frozen) || frozen.DatasetSnapshotID != domainsecurity.NoDatasetSnapshotID {
		t.Fatalf("caller-controlled source fields upgraded a general turn: %#v", frozen)
	}
	_, err = FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer: turnSecurityWorkspaceStub{}, RiskAuthority: authority, RiskIntent: domainsecurity.PublicationDispositionCaseEvidenceGate,
		}, Thread: map[string]any{"securityState": PublicRecord(frozen)}, ThreadID: frozen.ThreadID, TurnID: "turn-invalid-intent",
		Workspace: frozen.WorkspaceRealPath, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("caller supplied a publication disposition through risk intent")
	}
}

func TestFreezeWorkspaceRejectsV1AuditContext(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace/case-a", IssuedAt: time.Now().UTC(),
	})
	_, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: turnSecurityWorkspaceStub{}, RiskAuthority: &riskPolicyAuthorityStub{}},
		Thread: map[string]any{"securityState": PublicRecord(legacy)}, ThreadID: legacy.ThreadID, TurnID: "turn-v2",
		Workspace: legacy.WorkspaceRealPath, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if err == nil || err.Error() != "turn security context V1 is audit-only" {
		t.Fatalf("legacy context authorized a new freeze: %v", err)
	}
}

func TestChangedBindingFreezesBoundaryOnlyWithoutCaseFields(t *testing.T) {
	at := time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)
	authority := &riskPolicyAuthorityStub{}
	snapshotAuthority := &datasetSnapshotAuthorityStub{}
	oldBinding := domainsecurity.SHA256Hex([]byte("binding-old"))
	oldObserver := turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case-old", CaseBindingHash: oldBinding}}
	previous, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: oldObserver, RiskAuthority: authority, SnapshotAuthority: snapshotAuthority, SnapshotAuthorityV2: snapshotAuthority},
		Thread: map[string]any{}, ThreadID: "thread-binding-changed", TurnID: "turn-old", Workspace: "/workspace/case-a",
		Principal: testIdentityPrincipal(), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	newBinding := domainsecurity.SHA256Hex([]byte("binding-new"))
	boundary, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer:            turnSecurityWorkspaceStub{binding: domainsecurity.CaseBinding{CaseID: "case-new", CaseBindingHash: newBinding}},
			RiskAuthority:       authority,
			SnapshotAuthority:   snapshotAuthority,
			SnapshotAuthorityV2: snapshotAuthority,
		}, Thread: map[string]any{"securityState": PublicRecord(previous)}, ThreadID: previous.ThreadID, TurnID: "turn-new",
		Workspace: previous.WorkspaceRealPath, Principal: testIdentityPrincipal(),
		DatasetSnapshotID: "caller-must-not-survive", SourceDiagnostics: []any{map[string]any{"id": "source-must-not-survive"}},
		IssuedAt: at.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !domainsecurity.TurnSecurityContextIsBoundaryOnly(boundary) ||
		boundary.PublicationPolicy.CaseBindingState != domainsecurity.CaseBindingStateChangedUnaccepted ||
		boundary.PublicationPolicy.BlockerCode != domainsecurity.PublicationBlockerCaseBindingChanged ||
		boundary.CaseID != domainsecurity.UnboundCaseID || boundary.DatasetSnapshotID != domainsecurity.NoDatasetSnapshotID ||
		boundary.SourceManifestHash != domainsecurity.EmptySourceManifestHash {
		t.Fatalf("changed binding retained case/source authority: %#v", boundary)
	}
}

func TestCurrentRiskHeadRejectsHistoricalV2Context(t *testing.T) {
	at := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	authority := &riskPolicyAuthorityStub{}
	observer := turnSecurityWorkspaceStub{}
	general, err := FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(), Authority: WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: observer, RiskAuthority: authority},
		Thread: map[string]any{}, ThreadID: "thread-stale-head", TurnID: "turn-general", Workspace: "/workspace/case-a",
		Principal: testIdentityPrincipal(), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, _ := observer.Observe(general.WorkspaceRealPath)
	if _, err := ResolveRiskPublication(context.Background(), RiskPolicyResolutionInput{
		Authority: authority, ThreadID: general.ThreadID, WorkspaceRealPath: general.WorkspaceRealPath,
		Binding: observation, RiskIntent: domainsecurity.RiskClassCase, IssuedAt: at.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCurrentRiskAuthority(context.Background(), authority, general); err == nil {
		t.Fatal("historical signed V2 context remained current after risk head advanced")
	}
	if err := ValidateCurrent(CurrentValidationInput{Identity: testIdentityAuthority(),
		Observer: observer, RiskAuthority: authority, Context: general, Workspace: general.WorkspaceRealPath,
	}); err == nil {
		t.Fatal("current validation accepted a historical policy head")
	}
}

func turnSecurityEpochStateForTest(t *testing.T, threadID string, epoch uint64) map[string]any {
	t.Helper()
	snapshot := domaincontextepoch.SealSnapshot(domaincontextepoch.Snapshot{
		ThreadID: threadID, Epoch: epoch, RegistryDigest: domaincontextepoch.RegistryDigest(nil),
		AcceptedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	state := domaincontextepoch.SealState(domaincontextepoch.State{ThreadID: threadID, AcceptedSnapshot: snapshot})
	if err := domaincontextepoch.ValidateState(state); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(state)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
