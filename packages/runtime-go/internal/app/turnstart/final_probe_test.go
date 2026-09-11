package turnstart

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const (
	finalProbeExact          = "exact"
	finalProbeOffline        = "offline"
	finalProbeForgedSnapshot = "forged-snapshot"
	finalProbeForgedBinding  = "forged-binding"
)

var errFinalProbeOffline = errors.New("test source is offline")

type finalProbeWorkspaceReader struct {
	workspaceRealPath string
}

func (reader *finalProbeWorkspaceReader) WorkspaceRealPath(string) (string, error) {
	return reader.workspaceRealPath, nil
}

func (reader *finalProbeWorkspaceReader) ReadOptional(string) (domainsecurity.CaseBinding, bool, error) {
	return domainsecurity.CaseBinding{}, false, errors.New("legacy workspace reader is not security authority")
}

type finalProbeObserver struct {
	observation domainsecurity.CaseBindingObservationV1
	calls       int
}

func (observer *finalProbeObserver) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	observer.calls++
	if workspace != observer.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("test workspace observation mismatch")
	}
	return observer.observation, nil
}

type finalProbeRiskAuthority struct {
	policy    domainsecurity.ThreadRiskPolicyV1
	contracts securitycontexttest.RiskAuthorityContracts
	resolves  int
	validates int
}

func (authority *finalProbeRiskAuthority) ResolveOrRaise(_ context.Context, input threadriskauthorityapp.ResolveOrRaiseInput) (threadriskauthorityapp.Head, error) {
	authority.resolves++
	if input.ThreadID != authority.policy.ThreadID || input.WorkspaceRealPath != authority.policy.WorkspaceRealPath ||
		(input.RequestedRisk == domainsecurity.RiskClassCase && authority.policy.RiskClass != domainsecurity.RiskClassCase) {
		return threadriskauthorityapp.Head{}, errors.New("test risk authority request mismatch")
	}
	return threadriskauthorityapp.Head{
		HasIndex: true, Index: authority.contracts.Index, Request: authority.contracts.Request,
		Observation: authority.contracts.Observation, RiskAuthorityBinding: authority.contracts.Binding,
		Policy: authority.policy, Found: true,
	}, nil
}

func (authority *finalProbeRiskAuthority) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	authority.validates++
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		securityContext.ThreadID != authority.policy.ThreadID || securityContext.WorkspaceRealPath != authority.policy.WorkspaceRealPath ||
		domainsecurity.ValidateTurnPublicationPolicyForThreadRiskPolicyV1(securityContext.PublicationPolicy, authority.policy) != nil ||
		domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(
			securityContext.RiskAuthorityBinding,
			authority.contracts.Index,
			authority.contracts.Request,
			authority.contracts.Observation,
		) != nil {
		return errors.New("test risk authority head mismatch")
	}
	return nil
}

type finalProbeSnapshotAuthority struct {
	resolved     datasetsnapshotport.ResolvedSnapshotV2
	calls        int
	observations []domainsecurity.CaseBindingObservationV1
}

func (authority *finalProbeSnapshotAuthority) ResolveWitnessedV2(_ context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	authority.calls++
	observation := input.Observation
	authority.observations = append(authority.observations, observation)
	if domainsecurity.ValidateDatasetSnapshotAuthorityRecordForBindingV2(authority.resolved.Record, input.TenantID, input.UserID, observation) != nil ||
		(input.ExpectedDatasetSnapshotID != "" && input.ExpectedDatasetSnapshotID != authority.resolved.Record.DatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return authority.resolved, nil
}

type finalProbeSource struct {
	mode   string
	calls  int
	inputs []sourceprobeport.Input
}

func (source *finalProbeSource) ProbeCaseSource(_ context.Context, input sourceprobeport.Input) (domainsecurity.VerifiedSourceProbe, error) {
	source.calls++
	source.inputs = append(source.inputs, input)
	if source.mode == finalProbeOffline {
		return domainsecurity.VerifiedSourceProbe{}, errFinalProbeOffline
	}
	caseID := input.CaseID
	bindingHash := input.CaseBindingHash
	snapshotID := input.DatasetSnapshotID
	switch source.mode {
	case finalProbeForgedSnapshot:
		snapshotID = securitycontexttest.DatasetSnapshotID("provider-forged-snapshot")
	case finalProbeForgedBinding:
		bindingHash = domainsecurity.SHA256Hex([]byte("provider-forged-binding"))
	case finalProbeExact, "":
	default:
		return domainsecurity.VerifiedSourceProbe{}, errors.New("unknown test source mode")
	}
	response := domainsecurity.SourceProbeResponse{
		Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.15",
		CaseID: caseID, CaseBindingHash: bindingHash, DatasetSnapshotID: snapshotID,
		Ready: true, ReadOnly: true, CheckedAt: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		input.ServerID,
		response.ServerName,
		response.ServerVersion,
		domainsecurity.SHA256Hex([]byte("turn-start-final-probe-instance")),
		7,
	)
	if err != nil {
		return domainsecurity.VerifiedSourceProbe{}, err
	}
	return domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: input.ServerID, ServerIdentity: serverIdentity, ConnectionEpoch: 7,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("turn-start-final-probe-catalog")),
		SpecFingerprint:    domainsecurity.SHA256Hex([]byte("turn-start-final-probe-spec")),
		ThreadID:           input.ThreadID, TurnID: input.TurnID, ContextEpoch: input.ContextEpoch,
		ContextDigest: input.ContextDigest, DatasetSnapshotID: snapshotID,
		CheckedAt: time.Date(2026, 7, 12, 10, 0, 1, 0, time.UTC), Response: response,
	})
}

type finalProbeCaseFixture struct {
	threadID          string
	turnID            string
	workspace         string
	at                time.Time
	thread            map[string]any
	reader            *finalProbeWorkspaceReader
	observer          *finalProbeObserver
	riskAuthority     *finalProbeRiskAuthority
	snapshotAuthority *finalProbeSnapshotAuthority
	oldContext        domainsecurity.TurnSecurityContext
	newSnapshot       datasetsnapshotport.ResolvedSnapshotV2
}

func TestPrepareSecurityTransitionRegistersAndProbesOnlyFinalEpoch(t *testing.T) {
	fixture := newFinalProbeCaseFixture(t, "success")
	source := &finalProbeSource{mode: finalProbeExact}
	transition := beginFinalProbeTransition(t, fixture.reader, fixture.threadID, fixture.turnID, fixture.workspace, fixture.at)
	defer transition.Abort()

	registrationCalls := 0
	var registered domainsecurity.TurnSecurityContext
	preparation, err := PrepareSecurityTransition(
		context.Background(), transition, testIdentityPrincipal(), fixture.reader,
		turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer: fixture.observer, RiskAuthority: fixture.riskAuthority, SnapshotAuthorityV2: fixture.snapshotAuthority,
		},
		source, fixture.thread, fixture.threadID, fixture.turnID, fixture.workspace, fixture.at,
		finalProbeRecords(), time.Second,
		func(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			registrationCalls++
			if source.calls != 0 {
				return errors.New("live source probe ran before case-thread registration")
			}
			registered = securityContext
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.oldContext.ContextEpoch != 1 || registered.ContextEpoch != 2 || preparation.EpochState.AcceptedSnapshot.Epoch != 2 {
		t.Fatalf("host snapshot change did not advance the accepted epoch before registration: old=%d registered=%d accepted=%d",
			fixture.oldContext.ContextEpoch, registered.ContextEpoch, preparation.EpochState.AcceptedSnapshot.Epoch)
	}
	if registrationCalls != 1 || preparation.SecurityContext != registered {
		t.Fatalf("registration did not receive the exact final TSC: calls=%d registered=%#v final=%#v",
			registrationCalls, registered, preparation.SecurityContext)
	}
	if registered.DatasetSnapshotID != fixture.newSnapshot.Record.DatasetSnapshotID ||
		registered.DatasetSnapshotID == fixture.oldContext.DatasetSnapshotID ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(registered.DatasetSnapshotID) {
		t.Fatalf("final TSC did not retain the host-selected dsv2 snapshot: %#v", registered)
	}
	assertFinalProbeInput(t, source, registered)
	if !preparation.SourceReady || !preparation.ExecutionReady || preparation.SourceError != nil ||
		preparation.SourceProbe.ProbeContextDigest != registered.ContextDigest ||
		preparation.SourceProbe.DatasetSnapshotID != registered.DatasetSnapshotID {
		t.Fatalf("exact final probe did not authorize readiness: %#v", preparation)
	}
}

func TestPrepareSecurityTransitionProbeFailureCannotRemintFinalContext(t *testing.T) {
	for _, mode := range []string{finalProbeOffline, finalProbeForgedSnapshot, finalProbeForgedBinding} {
		t.Run(mode, func(t *testing.T) {
			fixture := newFinalProbeCaseFixture(t, mode)
			source := &finalProbeSource{mode: mode}
			transition := beginFinalProbeTransition(t, fixture.reader, fixture.threadID, fixture.turnID, fixture.workspace, fixture.at)
			defer transition.Abort()
			var registered domainsecurity.TurnSecurityContext
			preparation, err := PrepareSecurityTransition(
				context.Background(), transition, testIdentityPrincipal(), fixture.reader,
				turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
					Observer: fixture.observer, RiskAuthority: fixture.riskAuthority, SnapshotAuthorityV2: fixture.snapshotAuthority,
				},
				source, fixture.thread, fixture.threadID, fixture.turnID, fixture.workspace, fixture.at,
				finalProbeRecords(), time.Second,
				func(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
					if source.calls != 0 {
						return errors.New("live source probe ran before case-thread registration")
					}
					registered = securityContext
					return nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if registered.ContextEpoch != 2 || preparation.SecurityContext != registered ||
				preparation.SecurityContext.ContextDigest != registered.ContextDigest ||
				preparation.SecurityContext.DatasetSnapshotID != fixture.newSnapshot.Record.DatasetSnapshotID {
				t.Fatalf("failed/mismatched probe reminted or changed the final host TSC: registered=%#v final=%#v",
					registered, preparation.SecurityContext)
			}
			assertFinalProbeInput(t, source, registered)
			if preparation.SourceReady || !preparation.ExecutionReady || preparation.SourceError == nil {
				t.Fatalf("failed/mismatched probe changed ordinary readiness or authorized case facts: %#v", preparation)
			}
			if mode == finalProbeOffline && !errors.Is(preparation.SourceError, errFinalProbeOffline) {
				t.Fatalf("offline error was not preserved: %v", preparation.SourceError)
			}
		})
	}
}

func TestPrepareSecurityTransitionSkipsProbeForGeneralAndBoundaryContexts(t *testing.T) {
	for _, test := range []struct {
		name        string
		riskClass   string
		riskIntent  string
		execution   bool
		sourceReady bool
	}{
		{name: "general", riskClass: domainsecurity.RiskClassGeneral, execution: true, sourceReady: true},
		{name: "case-boundary", riskClass: domainsecurity.RiskClassCase, riskIntent: domainsecurity.RiskClassCase, execution: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			threadID := "thread-final-probe-" + test.name
			turnID := "turn-final-probe-" + test.name
			workspace := "/workspace/final-probe-" + test.name
			at := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
			observation := newFinalProbeObservation(t, workspace, domainsecurity.CaseBindingStateMissing, "", "")
			observer := &finalProbeObserver{observation: observation}
			riskAuthority := newFinalProbeRiskAuthority(t, threadID, workspace, test.riskClass, at.Add(-time.Minute))
			reader := &finalProbeWorkspaceReader{workspaceRealPath: workspace}
			source := &finalProbeSource{mode: finalProbeExact}
			transition := beginFinalProbeTransition(t, reader, threadID, turnID, workspace, at)
			defer transition.Abort()
			preparation, err := PrepareSecurityTransition(
				context.Background(), transition, testIdentityPrincipal(), reader,
				turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
					Observer: observer, RiskAuthority: riskAuthority, RiskIntent: test.riskIntent,
				},
				source, map[string]any{}, threadID, turnID, workspace, at,
				finalProbeRecords(), time.Second,
			)
			if err != nil {
				t.Fatal(err)
			}
			if source.calls != 0 || preparation.ExecutionReady != test.execution || preparation.SourceReady != test.sourceReady ||
				preparation.SourceProbe != (domainsecurity.VerifiedSourceProbe{}) {
				t.Fatalf("non-case-evidence TSC reached a live probe or wrong readiness: calls=%d preparation=%#v", source.calls, preparation)
			}
			if test.riskClass == domainsecurity.RiskClassGeneral {
				if !domainsecurity.TurnSecurityContextIsGeneral(preparation.SecurityContext) {
					t.Fatalf("general turn was not general: %#v", preparation.SecurityContext)
				}
			} else if !domainsecurity.TurnSecurityContextIsBoundaryOnly(preparation.SecurityContext) || !preparation.ExecutionReady {
				t.Fatalf("case boundary lost ordinary readiness or acquired case authority: %#v", preparation)
			}
		})
	}
}

func newFinalProbeCaseFixture(t *testing.T, suffix string) finalProbeCaseFixture {
	t.Helper()
	threadID := "thread-final-probe-" + suffix
	turnID := "turn-final-probe-" + suffix
	workspace := "/workspace/final-probe-" + suffix
	caseID := "case-final-probe-" + suffix
	bindingHash := domainsecurity.SHA256Hex([]byte("binding-hash:" + suffix))
	at := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	observation := newFinalProbeObservation(t, workspace, domainsecurity.CaseBindingStateValid, caseID, bindingHash)
	riskAuthority := newFinalProbeRiskAuthority(t, threadID, workspace, domainsecurity.RiskClassCase, at.Add(-3*time.Minute))
	snapshotKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x53}, ed25519.SeedSize))
	oldSnapshot := newFinalProbeSnapshotRecord(t, observation, snapshotKey, "old-material-"+suffix, at.Add(-2*time.Minute))
	newSnapshot := newFinalProbeSnapshotRecord(t, observation, snapshotKey, "new-material-"+suffix, at.Add(-time.Minute))
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: riskAuthority.policy.PolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	oldContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-final-probe-old-" + suffix, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: caseID, CaseBindingHash: bindingHash, DatasetSnapshotID: oldSnapshot.Record.DatasetSnapshotID,
		SourceManifestHash: oldSnapshot.Record.SourceManifestHash, ContextEpoch: 1, IssuedAt: at.Add(-time.Minute),
		PublicationPolicy: publication, RiskAuthorityBinding: riskAuthority.contracts.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	epochState, err := contextepochapp.BootstrapState(
		threadID,
		1,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(oldContext)},
		at.Add(-time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	return finalProbeCaseFixture{
		threadID: threadID, turnID: turnID, workspace: workspace, at: at,
		thread: map[string]any{
			"securityState":     turnsecurityapp.PublicRecord(oldContext),
			"contextEpochState": contextepochapp.PublicState(epochState),
		},
		reader:        &finalProbeWorkspaceReader{workspaceRealPath: workspace},
		observer:      &finalProbeObserver{observation: observation},
		riskAuthority: riskAuthority, snapshotAuthority: &finalProbeSnapshotAuthority{resolved: newSnapshot},
		oldContext: oldContext, newSnapshot: newSnapshot,
	}
}

func newFinalProbeRiskAuthority(t *testing.T, threadID, workspace, riskClass string, issuedAt time.Time) *finalProbeRiskAuthority {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x52}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	origin := domainsecurity.RiskPolicyOriginGeneralWorkspace
	if riskClass == domainsecurity.RiskClassCase {
		origin = domainsecurity.RiskPolicyOriginValidCaseBinding
	}
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: threadID, WorkspaceRealPath: workspace, RiskClass: riskClass, Origin: origin,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("turn-start-final-probe-risk:" + threadID)), IssuedAt: issuedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := securitycontexttest.WitnessedRiskAuthorityContracts(threadID, workspace, riskClass, policy.PolicyDigest)
	if err != nil {
		t.Fatal(err)
	}
	return &finalProbeRiskAuthority{policy: policy, contracts: contracts}
}

func newFinalProbeObservation(t *testing.T, workspace, state, caseID, bindingHash string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: state}
	if state == domainsecurity.CaseBindingStateValid {
		input.CaseID = caseID
		input.BindingSHA256 = domainsecurity.SHA256Hex([]byte("binding-bytes:" + caseID))
		input.CaseBindingHash = bindingHash
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func newFinalProbeSnapshotRecord(
	t *testing.T,
	observation domainsecurity.CaseBindingObservationV1,
	privateKey ed25519.PrivateKey,
	material string,
	acceptedAt time.Time,
) datasetsnapshotport.ResolvedSnapshotV2 {
	t.Helper()
	publicKey := privateKey.Public().(ed25519.PublicKey)
	resolved, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(datasetsnapshotv2fixture.ResolvedInput{
		InstallationID: domainsecurity.SHA256Hex([]byte("turn-start-final-probe-installation")),
		TenantID:       domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, Material: material, AcceptedAt: acceptedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func beginFinalProbeTransition(
	t *testing.T,
	reader turnsecurityapp.WorkspaceReader,
	threadID, turnID, workspace string,
	at time.Time,
) *subagentapp.SecurityContextTransition {
	t.Helper()
	transition, err := BeginSecurityTransition(
		context.Background(), subagentapp.NewRuntimeState(), testIdentityAuthority(), testIdentityPrincipal(),
		reader, nil, map[string]any{},
		threadID, turnID, workspace, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	return transition
}

func finalProbeRecords() SecurityRecords {
	return SecurityRecords{Turn: map[string]any{}, TurnStartedEvent: map[string]any{}, ThreadPatch: map[string]any{}}
}

func assertFinalProbeInput(t *testing.T, source *finalProbeSource, securityContext domainsecurity.TurnSecurityContext) {
	t.Helper()
	if source.calls != 1 || len(source.inputs) != 1 {
		t.Fatalf("live source probe call count mismatch: calls=%d inputs=%d", source.calls, len(source.inputs))
	}
	input := source.inputs[0]
	if input.ServerID != "analytix_funds" || input.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		input.ThreadID != securityContext.ThreadID || input.TurnID != securityContext.TurnID ||
		input.ContextEpoch != securityContext.ContextEpoch || input.ContextDigest != securityContext.ContextDigest ||
		input.CaseID != securityContext.CaseID || input.CaseBindingHash != securityContext.CaseBindingHash ||
		input.DatasetSnapshotID != securityContext.DatasetSnapshotID || !domainsecurity.IsDatasetSnapshotIDV2Syntax(input.DatasetSnapshotID) {
		t.Fatalf("live source probe did not receive the exact final host TSC: input=%#v context=%#v", input, securityContext)
	}
}
