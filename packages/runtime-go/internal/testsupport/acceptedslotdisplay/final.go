package acceptedslotdisplay

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

type WitnessedFinalInputV1 struct {
	Context            domainsecurity.TurnSecurityContext
	Registry           domainevidence.EvidenceReceiptRegistry
	Envelope           domainevidence.FinalAnswerEnvelope
	Receipt            domainevidence.EvidenceReceipt
	Snapshot           datasetsnapshotport.ResolvedSnapshotV2
	BindingObservation domainsecurity.CaseBindingObservationV1
	AuthorityKeyID     string
	AuthorityPublicKey []byte
	Sign               func([]byte) ([]byte, error)
	Now                time.Time
}

// NewWitnessedPrivateFinalV1 builds the current V5 fact-final graph used by
// cross-package production-seam tests. It deliberately reuses the supplied
// DSV2 record/signing authority and creates no alternative runtime authority.
func NewWitnessedPrivateFinalV1(input WitnessedFinalInputV1) (domainevidence.PrivateAcceptedFinalRecord, error) {
	if input.Sign == nil || input.Now.IsZero() ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		domainevidence.ValidateEvidenceReceipt(input.Receipt) != nil ||
		domainevidence.ValidateFinalAnswerEnvelope(input.Envelope) != nil ||
		input.Receipt.ThreadID != input.Context.ThreadID || input.Receipt.TurnID != input.Context.TurnID ||
		input.Snapshot.Record.DatasetSnapshotID != input.Context.DatasetSnapshotID ||
		input.Snapshot.Record.AuthorityKeyID != input.AuthorityKeyID ||
		len(input.AuthorityPublicKey) != ed25519.PublicKeySize {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("accepted-slot witnessed final input is invalid")
	}
	head, err := domainevidence.NewEvidenceRegistryHead(input.Registry)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	proof, err := domainevidence.NewPublicationSnapshotProof(domainevidence.PublicationSnapshotProofInput{
		Context: input.Context, RegistryHead: head, EvidenceReceiptIDs: input.Envelope.EvidenceReceiptIDs,
		Sources: []domainevidence.PublicationSourceSnapshot{{
			ReceiptID: input.Receipt.ReceiptID, ServerID: "analytix_funds", ServerIdentity: input.Receipt.ServerIdentity,
			ServerVersion: input.Receipt.ServerVersion, ConnectionEpoch: input.Receipt.ConnectionEpoch,
			ToolName: input.Receipt.ToolName, DatasetSnapshotID: input.Receipt.DatasetSnapshotID,
			CatalogFingerprint: domainsecurity.SHA256Hex([]byte("accepted-slot-final-catalog")),
			SpecFingerprint:    domainsecurity.SHA256Hex([]byte("accepted-slot-final-spec")),
			ProbeDigest:        domainsecurity.SHA256Hex([]byte("accepted-slot-final-probe")),
			CheckedAt:          input.Now.Format(time.RFC3339Nano),
		}},
		CheckedAt: input.Now.Add(time.Second),
	})
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	rendered, err := domainevidence.RenderFinalAnswer(input.Envelope)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: input.Now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, input.Envelope.TerminalReason)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	installationID := input.Snapshot.Record.InstallationID
	enrollmentID := domainsecurity.SHA256Hex([]byte("accepted-slot-final-enrollment"))
	datasetIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          domainsecurity.SHA256Hex([]byte("accepted-slot-final-dataset-index")),
		Binding:             input.Snapshot.Record.Binding, SnapshotRecordDigest: input.Snapshot.Record.RecordDigest,
		AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
	}, input.Sign)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		input.Context, input.Registry, input.AuthorityKeyID, input.AuthorityPublicKey, input.Sign,
	)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	registryIndex, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(
		domainevidence.EvidenceRegistryAuthorityIndexInputV2{
			InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
			PreviousIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			MutationID:          domainsecurity.SHA256Hex([]byte("accepted-slot-final-registry-index")),
		},
		capsule, input.AuthorityKeyID, input.AuthorityPublicKey, input.Sign,
	)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                 domainsecurity.SHA256Hex([]byte("accepted-slot-final-bundle")),
		DatasetSnapshotIndexDigest: datasetIndex.IndexDigest, DatasetSnapshotCount: 1,
		EvidenceRegistryIndexDigest: registryIndex.IndexDigest, EvidenceRegistryCount: 1,
		PublicationIndexDigest: domainsecurity.SHA256Hex([]byte("accepted-slot-final-publication-index")),
		AuthorityKeyID:         input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
	}, input.Sign)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x79}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	witnessKeyID := domainsecurity.SHA256Hex(witnessPublic)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace:  domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("accepted-slot-final-previous-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("accepted-slot-final-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("accepted-slot-final-fence")), MutationID: bundle.MutationID,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	observeRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID,
		Namespace:      domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("accepted-slot-final-challenge")),
		AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
	}, input.Sign)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		observeRequest, checkpoint,
		func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil },
	)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	admissionInput := domainevidence.FactFinalWitnessAdmissionInputV1{
		Context: input.Context, Envelope: input.Envelope, RenderedText: rendered, PublicationProof: &proof,
		Registry: input.Registry, Bundle: bundle, ObserveRequest: observeRequest, Observation: observation,
		RootIndex: registryIndex, SelectedIndex: registryIndex, SelectedCapsule: capsule,
		RegistryIndexPath: []domainevidence.EvidenceRegistryAuthorityIndexV2{registryIndex},
		DatasetRootIndex:  datasetIndex, SelectedDatasetIndex: datasetIndex,
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{datasetIndex},
		DatasetRecord:    input.Snapshot.Record, DatasetManifest: input.Snapshot.Manifest,
		FundsProducerContent: input.Snapshot.FundsProducerContent, BindingObservation: input.BindingObservation,
		InstallationID: installationID, EnrollmentID: enrollmentID,
		AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic, AdmittedAt: input.Now.Add(2 * time.Second),
	}
	admission, err := domainevidence.NewFactFinalWitnessAdmissionV1(admissionInput)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigestWithFactWitnessV1(
		input.Context, input.Envelope, rendered, intent, &proof, &admission,
	)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: input.Context, Envelope: input.Envelope, RenderedText: rendered, RegistryHead: head,
		PublicationSnapshotProof: &proof, FactFinalWitnessAdmission: &admission,
		FactFinalWitnessAuthority: &admissionInput, PrivateRecordDigest: privateDigest,
		AcceptedAt: input.Now.Add(2 * time.Second), AuthorityKeyID: input.AuthorityKeyID,
		AuthorityPublicKey: input.AuthorityPublicKey,
	}, input.Sign)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	return domainevidence.NewPrivateAcceptedFinalRecord(
		input.Context, input.Envelope, rendered, head, intent, accepted, &proof,
	)
}
