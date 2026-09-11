package authoritybootstrap

import (
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SchemaVersionV1 = 1
	AlgorithmV1     = "Ed25519"

	BindingPurposeV1        = "analytix.authority-bootstrap-binding/v1"
	ObservationPurposeV1    = "analytix.authority-bootstrap-observation/v1"
	PrepareReceiptPurposeV1 = "analytix.authority-bootstrap-prepare-receipt/v1"
	CommitReceiptPurposeV1  = "analytix.authority-bootstrap-commit-receipt/v1"

	PhaseUnprepared BootstrapPhaseV1 = "unprepared"
	PhasePrepared   BootstrapPhaseV1 = "prepared"
	PhaseCommitted  BootstrapPhaseV1 = "committed"
)

var (
	ErrInvalidContract       = errors.New("authority bootstrap contract is invalid")
	ErrAnchorMismatch        = errors.New("authority bootstrap trust anchor mismatch")
	ErrTransitionConflict    = errors.New("authority bootstrap exact transition conflicts")
	ErrReplayConflict        = errors.New("authority bootstrap mutation replay conflicts")
	ErrLiveRevalidation      = errors.New("authority bootstrap live witness revalidation is required")
	ErrCommittedFloorMissing = errors.New("committed authority bootstrap floor is missing")
)

type BootstrapPhaseV1 string

func validBootstrapPhaseV1(phase BootstrapPhaseV1) bool {
	return phase == PhaseUnprepared || phase == PhasePrepared || phase == PhaseCommitted
}

// BootstrapBindingV1 binds the manifest-selected witness enrollment, the
// exact witness observation intended to be obtained in the current run and
// selecting the floor checkpoint to project, plus an isolated one-shot
// bootstrap monotonic-head enrollment. The target floor may be any witness
// generation; EnrollmentInitialCheckpoint is the distinct manifest-enrolled
// generation-zero checkpoint. This contract proves authenticity and exact
// binding, not when the caller obtained either observation.
type BootstrapBindingV1 struct {
	SchemaVersion                  int                                          `json:"schemaVersion"`
	Purpose                        string                                       `json:"purpose"`
	InstallationID                 string                                       `json:"installationId"`
	CurrentManifestDigest          string                                       `json:"currentManifestDigest"`
	ManifestEnrollmentDigest       string                                       `json:"manifestEnrollmentDigest"`
	Namespace                      string                                       `json:"namespace"`
	EnrollmentID                   string                                       `json:"enrollmentId"`
	BootstrapEnrollmentID          string                                       `json:"bootstrapEnrollmentId"`
	WitnessKeyID                   string                                       `json:"witnessKeyId"`
	WitnessPublicKey               string                                       `json:"witnessPublicKey"`
	EnrollmentInitialCheckpoint    domainsecurity.MonotonicHeadCheckpointV1     `json:"enrollmentInitialCheckpoint"`
	FloorObserveRequest            domainsecurity.MonotonicHeadObserveRequestV1 `json:"floorObserveRequest"`
	FloorObservation               domainsecurity.MonotonicHeadObservationV1    `json:"floorObservation"`
	FloorCheckpoint                domainsecurity.MonotonicHeadCheckpointV1     `json:"floorCheckpoint"`
	FloorProjectionDigest          string                                       `json:"floorProjectionDigest"`
	ProjectionSlotID               string                                       `json:"projectionSlotId"`
	BootstrapInitialObserveRequest domainsecurity.MonotonicHeadObserveRequestV1 `json:"bootstrapInitialObserveRequest"`
	BootstrapInitialObservation    domainsecurity.MonotonicHeadObservationV1    `json:"bootstrapInitialObservation"`
	BootstrapInitialCheckpoint     domainsecurity.MonotonicHeadCheckpointV1     `json:"bootstrapInitialCheckpoint"`
	InstallationAuthorityAlgorithm string                                       `json:"installationAuthorityAlgorithm"`
	InstallationAuthorityKeyID     string                                       `json:"installationAuthorityKeyId"`
	InstallationAuthorityPublicKey string                                       `json:"installationAuthorityPublicKey"`
	InstallationAuthoritySignature string                                       `json:"installationAuthoritySignature"`
	BindingDigest                  string                                       `json:"bindingDigest"`
}

type BootstrapBindingInputV1 struct {
	InstallationID                 string
	Namespace                      string
	EnrollmentID                   string
	WitnessKeyID                   string
	WitnessPublicKey               []byte
	EnrollmentInitialCheckpoint    domainsecurity.MonotonicHeadCheckpointV1
	FloorObserveRequest            domainsecurity.MonotonicHeadObserveRequestV1
	FloorObservation               domainsecurity.MonotonicHeadObservationV1
	BootstrapInitialObserveRequest domainsecurity.MonotonicHeadObserveRequestV1
	BootstrapInitialObservation    domainsecurity.MonotonicHeadObservationV1
	InstallationAuthorityKeyID     string
	InstallationAuthorityPublicKey []byte
}

// AnchoredBootstrapBindingV1 has no wire representation. Only exact host
// anchor validation can create a non-zero value.
type AnchoredBootstrapBindingV1 struct {
	binding BootstrapBindingV1
}

// BootstrapObservationV1 wraps the existing signed observe request,
// challenge-bound witness observation, and exact monotonic checkpoint. The
// outer witness signature separates bootstrap use from generic head use.
type BootstrapObservationV1 struct {
	SchemaVersion        int                                          `json:"schemaVersion"`
	Purpose              string                                       `json:"purpose"`
	BindingDigest        string                                       `json:"bindingDigest"`
	Phase                BootstrapPhaseV1                             `json:"phase"`
	PrepareMutationID    string                                       `json:"prepareMutationId"`
	PrepareReceiptDigest string                                       `json:"prepareReceiptDigest"`
	CommitReceiptDigest  string                                       `json:"commitReceiptDigest"`
	ObserveRequest       domainsecurity.MonotonicHeadObserveRequestV1 `json:"observeRequest"`
	MonotonicObservation domainsecurity.MonotonicHeadObservationV1    `json:"monotonicObservation"`
	WitnessAlgorithm     string                                       `json:"witnessAlgorithm"`
	WitnessKeyID         string                                       `json:"witnessKeyId"`
	WitnessPublicKey     string                                       `json:"witnessPublicKey"`
	WitnessSignature     string                                       `json:"witnessSignature"`
	ObservationDigest    string                                       `json:"observationDigest"`
}

// BootstrapPrepareReceiptV1 proves the installation-authorized exact CAS from
// the enrolled bootstrap checkpoint to prepared. Both signed monotonic request
// and witness receipt are embedded; a state-only receipt is impossible.
type BootstrapPrepareReceiptV1 struct {
	SchemaVersion             int                                          `json:"schemaVersion"`
	Purpose                   string                                       `json:"purpose"`
	BindingDigest             string                                       `json:"bindingDigest"`
	ExpectedPhase             BootstrapPhaseV1                             `json:"expectedPhase"`
	NextPhase                 BootstrapPhaseV1                             `json:"nextPhase"`
	ExpectedObservationDigest string                                       `json:"expectedObservationDigest"`
	AdvanceRequest            domainsecurity.MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	AdvanceReceipt            domainsecurity.MonotonicHeadAdvanceReceiptV1 `json:"advanceReceipt"`
	WitnessAlgorithm          string                                       `json:"witnessAlgorithm"`
	WitnessKeyID              string                                       `json:"witnessKeyId"`
	WitnessPublicKey          string                                       `json:"witnessPublicKey"`
	WitnessSignature          string                                       `json:"witnessSignature"`
	ReceiptDigest             string                                       `json:"receiptDigest"`
}

// BootstrapCommitReceiptV1 proves the installation-authorized exact CAS from
// the specific prepared checkpoint and receipt to committed.
type BootstrapCommitReceiptV1 struct {
	SchemaVersion             int                                          `json:"schemaVersion"`
	Purpose                   string                                       `json:"purpose"`
	BindingDigest             string                                       `json:"bindingDigest"`
	PrepareReceiptDigest      string                                       `json:"prepareReceiptDigest"`
	ExpectedPhase             BootstrapPhaseV1                             `json:"expectedPhase"`
	NextPhase                 BootstrapPhaseV1                             `json:"nextPhase"`
	ExpectedObservationDigest string                                       `json:"expectedObservationDigest"`
	AdvanceRequest            domainsecurity.MonotonicHeadAdvanceRequestV1 `json:"advanceRequest"`
	AdvanceReceipt            domainsecurity.MonotonicHeadAdvanceReceiptV1 `json:"advanceReceipt"`
	WitnessAlgorithm          string                                       `json:"witnessAlgorithm"`
	WitnessKeyID              string                                       `json:"witnessKeyId"`
	WitnessPublicKey          string                                       `json:"witnessPublicKey"`
	WitnessSignature          string                                       `json:"witnessSignature"`
	ReceiptDigest             string                                       `json:"receiptDigest"`
}

type InstallationSignFuncV1 func([]byte) ([]byte, error)
type WitnessSignFuncV1 func([]byte) ([]byte, error)

// ProjectionConditionV1 contains exact comparison and target data only. It is
// intentionally constructible and therefore never an authorization token. A
// future write port must accept an opaque requirement, internally generate an
// unpredictable challenge, perform a live witness observation, acquire and
// consume a remote conditional lease/CAS, and keep that cut around the local
// exact write. No current API turns this value into write permission.
type ProjectionConditionV1 struct {
	InstallationID                        string
	CurrentManifestDigest                 string
	ManifestEnrollmentDigest              string
	Namespace                             string
	SourceEnrollmentID                    string
	BootstrapEnrollmentID                 string
	ProjectionSlotID                      string
	BindingDigest                         string
	AcceptedFloorObserveRequestDigest     string
	AcceptedFloorObservationDigest        string
	AcceptedBootstrapObserveRequestDigest string
	FloorCheckpoint                       domainsecurity.MonotonicHeadCheckpointV1
	FloorProjectionDigest                 string
	ExpectedPhase                         BootstrapPhaseV1
	ExpectedGeneration                    uint64
	ExpectedCheckpointDigest              string
	ExpectedStateDigest                   string
	ExpectedFenceNonce                    string
	PrepareReceiptDigest                  string
	CommitReceiptDigest                   string
}
