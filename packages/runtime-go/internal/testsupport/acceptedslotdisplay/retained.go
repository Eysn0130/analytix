package acceptedslotdisplay

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"time"

	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	datasetsnapshotfixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontextfixture "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type RetainedSourceFixtureV1 struct {
	Service            *fundsquerysourceapp.Service
	HistoricalContext  domainsecurity.TurnSecurityContext
	ActiveContext      domainsecurity.TurnSecurityContext
	HistoricalSnapshot datasetsnapshotport.ResolvedSnapshotV2
	Binding            domainevidence.AcceptedSlotSourceBindingV1
	Observation        domainsecurity.CaseBindingObservationV1
	PrivateKey         ed25519.PrivateKey
	PublicKey          ed25519.PublicKey
	CurrentSentinel    []byte
	authority          *retainedAuthorityV1
	materials          *retainedMaterialsV1
}

func (fixture *RetainedSourceFixtureV1) SetRetainedUnavailable(err error) {
	fixture.authority.mu.Lock()
	fixture.authority.err = err
	fixture.authority.mu.Unlock()
}

func (fixture *RetainedSourceFixtureV1) CorruptParsedPage() {
	fixture.materials.mu.Lock()
	fixture.materials.corruptKind = datasetsnapshotport.MaterialParsedPageV1
	fixture.materials.mu.Unlock()
}

func (fixture *RetainedSourceFixtureV1) ClearMaterialCorruption() {
	fixture.materials.mu.Lock()
	fixture.materials.corruptKind = ""
	fixture.materials.mu.Unlock()
}

func (fixture *RetainedSourceFixtureV1) DatasetAuthorityV2() datasetsnapshotport.CurrentAuthorityV2 {
	if fixture == nil {
		return nil
	}
	return fixture.authority
}

func (fixture *RetainedSourceFixtureV1) CaseBindingObserverV1() casecontextport.Observer {
	if fixture == nil {
		return nil
	}
	return retainedBindingObserverV1{observation: fixture.Observation}
}

func NewRetainedSourceFixtureV1(
	originalExact string,
	canonical string,
	field string,
) (*RetainedSourceFixtureV1, error) {
	if originalExact == "" || canonical == "" ||
		!domainevidence.IsAcceptedSlotSourceFieldV1(field) {
		return nil, errors.New("accepted-slot retained fixture input is invalid")
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/synthetic/cases/accepted-slot-display",
		State:             domainsecurity.CaseBindingStateValid, CaseID: "case-accepted-slot-display",
		BindingSHA256: digestV1("accepted-slot-binding-file"), CaseBindingHash: digestV1("accepted-slot-binding"),
	})
	if err != nil {
		return nil, err
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x63}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	installationID := digestV1("accepted-slot-installation")
	base, err := datasetsnapshotfixture.NewResolvedSnapshotV2(datasetsnapshotfixture.ResolvedInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, Material: "accepted-slot-retained-base", InstallationID: installationID,
		AcceptedAt:     time.Date(2026, 8, 21, 1, 0, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	})
	if err != nil {
		return nil, err
	}
	admission, err := datasetsnapshotfixture.NewAcceptedSlotRetainedAdmissionV1(
		datasetsnapshotfixture.AcceptedSlotRetainedAdmissionInputV1{
			Binding: base.Record.Binding, OriginalExact: originalExact, Canonical: canonical, Field: field,
			InstallationID: installationID, AcceptedAt: time.Date(2026, 8, 21, 1, 3, 0, 0, time.UTC),
			AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
			Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
		},
	)
	if err != nil {
		return nil, err
	}
	historicalSnapshot := admission.Snapshot
	currentSnapshot, err := datasetsnapshotfixture.NewResolvedSnapshotV2(datasetsnapshotfixture.ResolvedInput{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, Material: "CURRENT-DB-99990000", InstallationID: installationID,
		AcceptedAt:     time.Date(2026, 8, 21, 1, 5, 0, 0, time.UTC),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
		Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	})
	if err != nil {
		return nil, err
	}
	historicalContext, err := contextV1("turn-accepted-slot-historical", 1, observation, historicalSnapshot)
	if err != nil {
		return nil, err
	}
	activeContext, err := contextV1("turn-accepted-slot-current", 2, observation, currentSnapshot)
	if err != nil {
		return nil, err
	}
	binding, err := domainevidence.NewAcceptedSlotSourceBindingV1(domainevidence.AcceptedSlotSourceBindingInputV1{
		FactIDs:         []string{"accepted-slot-fact"},
		EntityReference: "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceRecordID:  admission.SourceRecordID, SourceFileID: admission.SourceFileID,
		SourceRowNumber: admission.SourceRowNumber, Field: field,
	})
	if err != nil {
		return nil, err
	}
	currentSelection := datasetsnapshotport.CurrentSelectionV2{Snapshot: currentSnapshot}
	authority := &retainedAuthorityV1{selection: datasetsnapshotport.RetainedSelectionV2{
		Current: currentSelection, Snapshot: historicalSnapshot,
	}}
	materials := &retainedMaterialsV1{values: admission.Materials}
	service, err := fundsquerysourceapp.NewService(
		authority, retainedBindingObserverV1{observation: observation}, materials, retainedHostSourceV1{},
	)
	if err != nil {
		return nil, err
	}
	return &RetainedSourceFixtureV1{
		Service: service, HistoricalContext: historicalContext, ActiveContext: activeContext,
		HistoricalSnapshot: historicalSnapshot, Binding: binding, Observation: observation,
		PrivateKey: privateKey, PublicKey: publicKey,
		CurrentSentinel: []byte("CURRENT-DB-99990000"), authority: authority, materials: materials,
	}, nil
}

type retainedAuthorityV1 struct {
	mu        sync.Mutex
	selection datasetsnapshotport.RetainedSelectionV2
	err       error
}

func (authority *retainedAuthorityV1) WithCurrentSelectionV2(
	context.Context,
	datasetsnapshotport.ResolveInputV2,
	domainsecurity.TurnSecurityContext,
	func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	return datasetsnapshotport.ErrUnavailable
}

func (authority *retainedAuthorityV1) WithRetainedSelectionV2(
	ctx context.Context,
	_ datasetsnapshotport.RetainedSelectionInputV2,
	use func(context.Context, datasetsnapshotport.RetainedSelectionV2) error,
) error {
	authority.mu.Lock()
	err := authority.err
	selection := authority.selection
	authority.mu.Unlock()
	if err != nil {
		return err
	}
	if ctx == nil || ctx.Err() != nil || use == nil {
		return datasetsnapshotport.ErrUnavailable
	}
	leaseContext, cancel := context.WithCancel(ctx)
	defer cancel()
	return use(leaseContext, selection)
}

type retainedBindingObserverV1 struct {
	observation domainsecurity.CaseBindingObservationV1
}

var _ casecontextport.Observer = retainedBindingObserverV1{}

func (observer retainedBindingObserverV1) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if workspace != observer.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("accepted-slot workspace changed")
	}
	return observer.observation, nil
}

type retainedMaterialsV1 struct {
	mu          sync.Mutex
	values      map[datasetsnapshotport.MaterialKindV2]map[string][]byte
	corruptKind datasetsnapshotport.MaterialKindV2
}

func (reader *retainedMaterialsV1) ResolveExact(
	_ context.Context,
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	body, ok := reader.values[kind][reference.Address]
	if !ok || uint64(len(body)) != reference.ByteLength || domainsecurity.SHA256Hex(body) != reference.SHA256 {
		return nil, datasetsnapshotport.ErrNotFound
	}
	copy := append([]byte(nil), body...)
	if reader.corruptKind == kind && len(copy) > 0 {
		copy[len(copy)-1] ^= 1
	}
	return copy, nil
}

type retainedHostSourceV1 struct{}

func (retainedHostSourceV1) WithExact(
	context.Context,
	domainfundsquerysource.DescriptorV1,
	func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	return fundsquerysourceport.ErrUnavailable
}

func contextV1(
	turnID string,
	epoch uint64,
	observation domainsecurity.CaseBindingObservationV1,
	snapshot datasetsnapshotport.ResolvedSnapshotV2,
) (domainsecurity.TurnSecurityContext, error) {
	const threadID = "thread-accepted-slot-display"
	policyDigest := digestV1("accepted-slot-risk-policy")
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	riskBinding, err := securitycontextfixture.WitnessedRiskBinding(
		threadID, observation.WorkspaceRealPath, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: observation.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		DatasetSnapshotID: snapshot.Record.DatasetSnapshotID, SourceManifestHash: snapshot.Record.SourceManifestHash,
		ContextEpoch: epoch, IssuedAt: time.Date(2026, 8, 21, 1, 10, 0, 0, time.UTC),
		PublicationPolicy: publication, RiskAuthorityBinding: riskBinding,
	})
}

func digestV1(value string) string { return domainsecurity.SHA256Hex([]byte(value)) }

var _ datasetsnapshotport.CurrentAuthorityV2 = (*retainedAuthorityV1)(nil)
var _ datasetsnapshotport.RetainedSelectionReaderV2 = (*retainedAuthorityV1)(nil)
var _ datasetsnapshotport.AdmissionMaterialReaderV2 = (*retainedMaterialsV1)(nil)
var _ fundsquerysourceport.HostExactSource = retainedHostSourceV1{}
