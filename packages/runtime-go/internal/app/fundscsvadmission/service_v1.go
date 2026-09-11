package fundscsvadmission

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

var (
	ErrInvalidRequest = errors.New("funds CSV admission request is invalid")
	ErrUnavailable    = errors.New("funds CSV admission is unavailable")
)

type NativeOwnerV1 interface {
	nativecomponentport.FundsCanonicalCSVSnapshotBuilder
	nativecomponentport.TransactionSourceRowPageRunner
}

type MaterialWriterV1 interface {
	PutFundsCanonicalCSVAdmissionMaterialV1(
		context.Context,
		domainevidence.FundsCanonicalCSVAdmissionMaterialV1,
	) error
}

type ImmutableSourceV1 interface {
	fundsquerysourceport.ImmutableSnapshotInstaller
	fundsquerysourceport.ImmutableSnapshotPrepublicationSource
}

type SnapshotAuthorityV2 interface {
	AdmitExactV2(context.Context, datasetsnapshotapp.AdmitInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error)
	AdmitAfterExactV2(context.Context, datasetsnapshotapp.AdmitAfterInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error)
	ResolveWitnessedV2(context.Context, datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error)
}

type FrozenSourceRevalidatorV1 func(
	context.Context,
	func(context.Context, []byte, string) error,
) error

type ImportSourceReaderV1 func(
	context.Context,
	string,
	string,
	func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error,
) error

type ConfigV1 struct {
	Observer         casecontextport.Observer
	Identity         identityport.Authority
	Evidence         *evidenceauthorityapp.Authority
	Snapshots        SnapshotAuthorityV2
	Materials        MaterialWriterV1
	Native           NativeOwnerV1
	Source           ImmutableSourceV1
	ReadImportSource ImportSourceReaderV1
	// Called only after a confirmed exact import has admitted its real snapshot.
	ActivateEvidenceRegistry func(context.Context, domainsecurity.CaseBindingObservationV1, string) error
	Now                      func() time.Time
	Random                   io.Reader
}

type ServiceV1 struct {
	observer                 casecontextport.Observer
	identity                 identityport.Authority
	evidence                 *evidenceauthorityapp.Authority
	snapshots                SnapshotAuthorityV2
	materials                MaterialWriterV1
	native                   NativeOwnerV1
	source                   ImmutableSourceV1
	readImportSource         ImportSourceReaderV1
	activateEvidenceRegistry func(context.Context, domainsecurity.CaseBindingObservationV1, string) error
	admissionMu              sync.Mutex
	now                      func() time.Time
	random                   io.Reader
	stagingMu                sync.Mutex
	activeImport             *activeImportGenerationV1
	stagingExpiryTimer       *time.Timer
	stageEpoch               uint64
	stageInFlight            *stageInvocationV1
	commitExact              func(context.Context, string, []byte, string, uint64) (StageResultV1, error)
}

type StageInputV1 struct {
	WorkspaceRoot string
	SourcePath    string
}

type StageResultV1 struct {
	SourceArtifactSHA256     string `json:"sourceArtifactSha256"`
	SourceArtifactByteLength uint64 `json:"sourceArtifactByteLength"`
	SourceRowCount           uint64 `json:"sourceRowCount"`
}

const (
	CleaningCommitStatusCommittedV1      = "committed"
	CleaningCommitStatusOutcomeUnknownV1 = "outcome_unknown"
	CleaningCommitStatusPreCASFailedV1   = "pre_cas_failed"
)

type CleaningCommitInputV1 struct {
	WorkspaceRoot           string
	ExpectedInputSnapshotID string
	SourceBody              []byte
	SourceSHA256            string
	SourceRowCount          uint64
	AcquiredAt              time.Time
	AcquisitionActorDigest  string
	IntentNonceDigest       string
}

type CleaningCommitResultV1 struct {
	Status                  string
	InputDatasetSnapshotID  string
	OutputDatasetSnapshotID string
}

type admissionOptionsV1 struct {
	expectedInputSnapshotID string
	acquiredAt              time.Time
	actorDigest             string
	intentDigest            string
}

type admissionResultV1 struct {
	stage            StageResultV1
	outputSnapshotID string
	status           string
}

func NewServiceV1(config ConfigV1) (*ServiceV1, error) {
	if config.Observer == nil || config.Identity == nil || config.Evidence == nil || config.Snapshots == nil ||
		config.Materials == nil || config.Native == nil || config.Source == nil || config.ReadImportSource == nil {
		return nil, ErrUnavailable
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	service := &ServiceV1{
		observer: config.Observer, identity: config.Identity, evidence: config.Evidence, snapshots: config.Snapshots,
		materials: config.Materials, native: config.Native, source: config.Source,
		readImportSource:         config.ReadImportSource,
		activateEvidenceRegistry: config.ActivateEvidenceRegistry,
		now:                      config.Now, random: config.Random,
	}
	service.commitExact = service.admitExactV1
	return service, nil
}

func (service *ServiceV1) admitExactV1(
	ctx context.Context,
	workspace string,
	sourceBody []byte,
	sourceSHA256 string,
	rowCount uint64,
) (StageResultV1, error) {
	result, err := service.admitExactWithOptionsV1(
		ctx, workspace, sourceBody, sourceSHA256, rowCount, admissionOptionsV1{},
	)
	if err != nil {
		return StageResultV1{}, err
	}
	if result.status != CleaningCommitStatusCommittedV1 {
		return StageResultV1{}, errors.Join(ErrUnavailable, errors.New("funds CSV admission outcome is unknown"))
	}
	return result.stage, nil
}

func (service *ServiceV1) CommitCleaningV1(
	ctx context.Context,
	input CleaningCommitInputV1,
) (CleaningCommitResultV1, error) {
	if service == nil || !domainsecurity.IsDatasetSnapshotIDV2Syntax(input.ExpectedInputSnapshotID) ||
		len(input.SourceBody) == 0 || !domainsecurity.IsSHA256Hex(input.SourceSHA256) ||
		domainsecurity.SHA256Hex(input.SourceBody) != input.SourceSHA256 || input.SourceRowCount == 0 ||
		!domainsecurity.IsSHA256Hex(input.AcquisitionActorDigest) ||
		!domainsecurity.IsSHA256Hex(input.IntentNonceDigest) {
		return CleaningCommitResultV1{}, ErrInvalidRequest
	}
	result, err := service.admitExactWithOptionsV1(
		ctx, input.WorkspaceRoot, input.SourceBody, input.SourceSHA256, input.SourceRowCount,
		admissionOptionsV1{
			expectedInputSnapshotID: input.ExpectedInputSnapshotID,
			acquiredAt:              input.AcquiredAt.UTC(), actorDigest: input.AcquisitionActorDigest,
			intentDigest: input.IntentNonceDigest,
		},
	)
	if err != nil {
		// admitExactWithOptionsV1 reaches the DSV2 linearization point only in
		// admitComposedCleaningV1. Cleaning-specific failures from that function
		// are returned as closed statuses after reconciliation, so an error here
		// proves that the current head was never offered to the CAS owner.
		return CleaningCommitResultV1{
			Status: CleaningCommitStatusPreCASFailedV1, InputDatasetSnapshotID: input.ExpectedInputSnapshotID,
		}, nil
	}
	return CleaningCommitResultV1{
		Status: result.status, InputDatasetSnapshotID: input.ExpectedInputSnapshotID,
		OutputDatasetSnapshotID: result.outputSnapshotID,
	}, nil
}

func (service *ServiceV1) admitExactWithOptionsV1(
	ctx context.Context,
	workspace string,
	sourceBody []byte,
	sourceSHA256 string,
	rowCount uint64,
	options admissionOptionsV1,
) (admissionResultV1, error) {
	service.admissionMu.Lock()
	defer service.admissionMu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return admissionResultV1{}, ErrInvalidRequest
	}

	observation, err := service.observer.Observe(workspace)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid ||
		observation.WorkspaceRealPath != workspace {
		return admissionResultV1{}, ErrInvalidRequest
	}
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
			CaseBindingHash:          observation.CaseBindingHash,
			BindingObservationDigest: observation.ObservationDigest,
		},
	)
	if err != nil {
		return admissionResultV1{}, ErrInvalidRequest
	}
	if _, err := service.evidence.Initialize(ctx); err != nil &&
		!errors.Is(err, evidenceauthorityapp.ErrAlreadyInitialized) {
		return admissionResultV1{}, errors.Join(ErrUnavailable, err)
	}
	sourceRevision, currentSnapshotID, currentAcquiredAt, err := service.nextSourceRevisionV1(ctx, observation)
	if err != nil {
		return admissionResultV1{}, err
	}
	if options.expectedInputSnapshotID != "" && currentSnapshotID != options.expectedInputSnapshotID {
		return admissionResultV1{}, errors.Join(ErrUnavailable, datasetsnapshotport.ErrStale)
	}
	intentNonce := options.intentDigest
	actorDigest := options.actorDigest
	acquiredAt := options.acquiredAt
	if options.expectedInputSnapshotID == "" {
		intentNonce, err = freshAdmissionDigestV1(service.random, "intent")
		if err != nil {
			return admissionResultV1{}, errors.Join(ErrUnavailable, err)
		}
		actorDigest = domainsecurity.SHA256Hex([]byte("analytix.main-owned-local-csv-admission/v1"))
		acquiredAt = service.now().UTC()
	} else if acquiredAt.IsZero() {
		acquiredAt = currentAcquiredAt
	}
	if acquiredAt.IsZero() || !domainsecurity.IsSHA256Hex(intentNonce) || !domainsecurity.IsSHA256Hex(actorDigest) {
		return admissionResultV1{}, ErrInvalidRequest
	}

	var outputSnapshotID string
	summary, err := domainevidence.ComposeFundsCanonicalCSVAdmissionV1(
		ctx,
		domainevidence.FundsCanonicalCSVAdmissionInputV1{
			Binding: binding, AcquiredAt: acquiredAt,
			AcquisitionActorDigest: actorDigest, IntentNonceDigest: intentNonce,
			SourceArtifactSHA256: sourceSHA256, SourceArtifactByteLength: uint64(len(sourceBody)),
			SourceRowCount: rowCount,
		},
		bytes.NewReader(sourceBody),
		func(
			buildCtx context.Context,
			build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
		) (domainevidence.FundsCanonicalCSVNativeBuildResultV1, error) {
			return service.buildNativeV1(buildCtx, build, sourceRevision, sourceBody)
		},
		func(material domainevidence.FundsCanonicalCSVAdmissionMaterialV1) error {
			if err := material.UseExactV1(func(
				kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1,
				_ string,
				body []byte,
			) error {
				if kind != domainevidence.FundsCanonicalCSVMaterialSnapshotManifestV1 {
					return nil
				}
				manifest, parseErr := domainsecurity.ParseDatasetSnapshotManifestV2(body)
				if parseErr != nil {
					return parseErr
				}
				outputSnapshotID = domainsecurity.DeriveDatasetSnapshotIDV2(manifest)
				return nil
			}); err != nil {
				return err
			}
			return service.materials.PutFundsCanonicalCSVAdmissionMaterialV1(ctx, material)
		},
	)
	if err != nil || !domainsecurity.IsDatasetSnapshotIDV2Syntax(outputSnapshotID) {
		return admissionResultV1{}, errors.Join(ErrUnavailable, err)
	}
	status, err := service.admitComposedCleaningV1(
		ctx, workspace, observation, summary, outputSnapshotID, options.expectedInputSnapshotID,
	)
	if err != nil {
		return admissionResultV1{}, err
	}
	return admissionResultV1{
		stage: StageResultV1{
			SourceArtifactSHA256: sourceSHA256, SourceArtifactByteLength: uint64(len(sourceBody)),
			SourceRowCount: rowCount,
		},
		outputSnapshotID: outputSnapshotID, status: status,
	}, nil
}

func (service *ServiceV1) admitComposedCleaningV1(
	ctx context.Context,
	workspace string,
	observation domainsecurity.CaseBindingObservationV1,
	summary domainevidence.FundsCanonicalCSVAdmissionSummaryV1,
	outputSnapshotID string,
	expectedInputSnapshotID string,
) (string, error) {
	admitInput := datasetsnapshotapp.AdmitInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation,
		ManifestReference: datasetsnapshotport.ExactMaterialReferenceV2{
			Address: summary.ManifestSHA256, SHA256: summary.ManifestSHA256,
			ByteLength: summary.ManifestByteLength,
		},
		FundsProducerReference: datasetsnapshotport.ExactMaterialReferenceV2{
			Address: summary.ProducerSHA256, SHA256: summary.ProducerSHA256,
			ByteLength: summary.ProducerByteLength,
		},
		AcceptedAt: service.now().UTC(),
	}
	var resolved datasetsnapshotport.ResolvedSnapshotV2
	var err error
	if expectedInputSnapshotID == "" {
		resolved, err = service.snapshots.AdmitExactV2(ctx, admitInput)
	} else {
		resolved, err = service.snapshots.AdmitAfterExactV2(ctx, datasetsnapshotapp.AdmitAfterInputV2{
			AdmitInputV2: admitInput, ExpectedCurrentDatasetSnapshotID: expectedInputSnapshotID,
		})
	}
	if err != nil || datasetsnapshotport.ValidateResolvedSnapshotV2(resolved) != nil {
		if expectedInputSnapshotID != "" {
			if service.currentSnapshotIsV1(ctx, observation, outputSnapshotID) {
				return CleaningCommitStatusOutcomeUnknownV1, nil
			}
			if service.currentSnapshotIsV1(ctx, observation, expectedInputSnapshotID) {
				return CleaningCommitStatusPreCASFailedV1, nil
			}
			return CleaningCommitStatusOutcomeUnknownV1, nil
		}
		return "", errors.Join(ErrUnavailable, err)
	}
	if resolved.Record.DatasetSnapshotID != outputSnapshotID {
		if expectedInputSnapshotID != "" {
			return CleaningCommitStatusOutcomeUnknownV1, nil
		}
		return "", ErrUnavailable
	}
	current, err := service.observer.Observe(workspace)
	if err != nil || current != observation {
		if expectedInputSnapshotID != "" {
			return CleaningCommitStatusOutcomeUnknownV1, nil
		}
		return "", errors.Join(ErrUnavailable, errors.New("case binding changed during funds CSV admission"))
	}
	if expectedInputSnapshotID == "" && service.activateEvidenceRegistry != nil {
		if err := service.activateEvidenceRegistry(ctx, observation, outputSnapshotID); err != nil {
			return "", errors.Join(ErrUnavailable, err)
		}
		current, err = service.observer.Observe(workspace)
		if err != nil || current != observation || !service.currentSnapshotIsV1(ctx, observation, outputSnapshotID) {
			return "", errors.Join(ErrUnavailable, errors.New("confirmed import authority changed during registry activation"))
		}
	}
	return CleaningCommitStatusCommittedV1, nil
}

func (service *ServiceV1) nextSourceRevisionV1(
	ctx context.Context,
	observation domainsecurity.CaseBindingObservationV1,
) (uint64, string, time.Time, error) {
	resolved, err := service.snapshots.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation,
	})
	if errors.Is(err, datasetsnapshotport.ErrUnavailable) {
		return 1, "", time.Time{}, nil
	}
	if err != nil || datasetsnapshotport.ValidateResolvedSnapshotV2(resolved) != nil {
		return 0, "", time.Time{}, errors.Join(ErrUnavailable, err)
	}
	var current uint64
	switch resolved.Manifest.ProducerContentContract {
	case domainsecurity.FundsProducerContentManifestContractV1:
		current = resolved.FundsProducerContent.SourceRevision
	case domainsecurity.FundsProducerContentManifestContractV2:
		current = resolved.FundsProducerContentV2.SourceRevision
	default:
		return 0, "", time.Time{}, ErrUnavailable
	}
	if current == 0 || current >= 9_007_199_254_740_991 {
		return 0, "", time.Time{}, ErrUnavailable
	}
	acquiredAt, parseErr := time.Parse(time.RFC3339Nano, resolved.Manifest.AcquiredAt)
	if parseErr != nil {
		return 0, "", time.Time{}, ErrUnavailable
	}
	return current + 1, resolved.Record.DatasetSnapshotID, acquiredAt.UTC(), nil
}

func (service *ServiceV1) currentSnapshotIsV1(
	ctx context.Context,
	observation domainsecurity.CaseBindingObservationV1,
	expected string,
) bool {
	resolved, err := service.snapshots.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		Observation: observation, ExpectedDatasetSnapshotID: expected,
	})
	return err == nil && datasetsnapshotport.ValidateResolvedSnapshotV2(resolved) == nil &&
		resolved.Record.DatasetSnapshotID == expected
}

func (service *ServiceV1) buildNativeV1(
	ctx context.Context,
	build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
	sourceRevision uint64,
	sourceBody []byte,
) (_ domainevidence.FundsCanonicalCSVNativeBuildResultV1, resultErr error) {
	stage := "ARGUMENTS"
	defer func() { writeNativeBuildFailureV1(os.Stderr, stage, resultErr) }()
	arguments, err := domainnative.NewFundsCanonicalCSVSnapshotBuildArgumentsV1(
		domainnative.FundsCanonicalCSVSnapshotBuildInputV1{
			Binding: build.Binding, PrivateImportFileID: build.PrivateImportFileID,
			SourceRevision: sourceRevision, RawArtifactManifestSHA256: build.RawArtifactManifestSHA256,
			SourceArtifactSHA256:     build.SourceArtifactSHA256,
			SourceArtifactByteLength: build.SourceArtifactByteLength,
			SourceRowCount:           build.SourceRowCount,
		},
	)
	if err != nil {
		return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, err
	}
	stage = "NATIVE_CALL"
	result, object, _, err := service.native.BuildFundsCanonicalCSVSnapshot(
		ctx, arguments, bytes.NewReader(sourceBody), service.source,
	)
	if err == nil {
		stage = "OBJECT_VALIDATION"
	}
	if err != nil || domainfundsquerysource.ValidateImmutableSnapshotObjectV1(object) != nil {
		return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, errors.Join(ErrUnavailable, err)
	}
	stage = "SOURCE_STATS"
	sha256, byteLength, rowCount, _, _ := result.SourceStatsV1()
	if sha256 != build.SourceArtifactSHA256 || byteLength != build.SourceArtifactByteLength ||
		rowCount != build.SourceRowCount || object.CaseID != build.Binding.CaseID {
		return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, ErrUnavailable
	}
	stage = "MATERIALIZATION"
	materialization, err := result.MaterializationV1()
	if err != nil {
		return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, err
	}
	stage = "SOURCE_ROWS"
	rows, err := service.readNativeRowsV1(ctx, build, materialization, object)
	if err != nil {
		return domainevidence.FundsCanonicalCSVNativeBuildResultV1{}, err
	}
	stage = "RESULT_CONSTRUCTION"
	return domainevidence.NewFundsCanonicalCSVNativeBuildResultV1(
		materialization, object.DuckDBSHA256, object.DuckDBByteLength, rows,
	)
}

// Diagnostics stop at fixed call sites and sentinel classes. In particular,
// never format the underlying error that the composer deliberately discards.
func writeNativeBuildFailureV1(output io.Writer, stage string, err error) {
	if err == nil {
		return
	}
	switch stage {
	case "ARGUMENTS", "NATIVE_CALL", "OBJECT_VALIDATION", "SOURCE_STATS", "MATERIALIZATION", "SOURCE_ROWS", "RESULT_CONSTRUCTION":
	default:
		stage = "UNKNOWN"
	}
	class := "UNKNOWN"
	switch {
	case errors.Is(err, nativecomponentport.ErrTerminationUnconfirmed):
		class = "TERMINATION"
	case errors.Is(err, context.Canceled):
		class = "CANCELLED"
	case errors.Is(err, context.DeadlineExceeded):
		class = "DEADLINE"
	case errors.Is(err, nativecomponentport.ErrProtocolInvalid):
		class = "PROTOCOL"
	case errors.Is(err, nativecomponentport.ErrRegistryInvalid):
		class = "REGISTRY"
	case errors.Is(err, nativecomponentport.ErrRequestInvalid):
		class = "REQUEST_INVALID"
	case errors.Is(err, fundsquerysourceport.ErrNotFound):
		class = "SOURCE_NOT_FOUND"
	case errors.Is(err, fundsquerysourceport.ErrMismatch):
		class = "SOURCE_MISMATCH"
	case errors.Is(err, fundsquerysourceport.ErrCorrupt):
		class = "SOURCE_CORRUPT"
	case errors.Is(err, nativecomponentport.ErrUnavailable), errors.Is(err, fundsquerysourceport.ErrUnavailable), errors.Is(err, ErrUnavailable):
		class = "UNAVAILABLE"
	}
	_, _ = io.WriteString(output, "[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 layer=ADMISSION stage="+stage+" class="+class+"\n")
}

func (service *ServiceV1) readNativeRowsV1(
	ctx context.Context,
	build domainevidence.FundsCanonicalCSVNativeBuildContextV1,
	materialization domainsecurity.FundsMaterializationResultV1,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
) ([]domainevidence.FundsCanonicalCSVHostRowV1, error) {
	rows := make([]domainevidence.FundsCanonicalCSVHostRowV1, 0, build.SourceRowCount)
	var cursor *domainnative.TransactionSourceRowCursorV1
	for uint64(len(rows)) < build.SourceRowCount {
		arguments, err := domainnative.NewTransactionSourceRowPageArgumentsV1(
			domainnative.TransactionSourceRowPageArgumentsInputV1{
				Binding:                        build.Binding,
				ParsedGenerationIdentitySHA256: build.ParsedGenerationIdentitySHA256,
				Materialization:                materialization, InstalledSnapshot: object,
				MaxRows: domainnative.TransactionSourceRowMaximumRowsV1, Cursor: cursor,
			},
		)
		if err != nil {
			return nil, err
		}
		var page domainnative.TransactionSourceRowPageV1
		err = service.source.WithInstalledExact(ctx, object, func(
			leaseCtx context.Context,
			lease fundsquerysourceport.ExactReadLease,
		) error {
			var runErr error
			page, runErr = service.native.TransactionSourceRowPage(
				leaseCtx, arguments, object, lease,
			)
			return runErr
		})
		if err != nil {
			return nil, err
		}
		var next *domainnative.TransactionSourceRowCursorV1
		if err := page.UseFundsCanonicalCSVAdmissionRowsV1(func(
			pageRows []domainevidence.FundsCanonicalCSVHostRowV1,
			pageNext *domainnative.TransactionSourceRowCursorV1,
			complete bool,
		) error {
			rows = append(rows, pageRows...)
			next = pageNext
			if complete != (next == nil) {
				return ErrUnavailable
			}
			return nil
		}); err != nil {
			return nil, err
		}
		if len(rows) > int(build.SourceRowCount) || next == nil && uint64(len(rows)) != build.SourceRowCount {
			return nil, ErrUnavailable
		}
		cursor = next
		if cursor == nil {
			break
		}
	}
	if uint64(len(rows)) != build.SourceRowCount {
		return nil, ErrUnavailable
	}
	return rows, nil
}

func freshAdmissionDigestV1(random io.Reader, label string) (string, error) {
	if random == nil || label == "" {
		return "", ErrUnavailable
	}
	var value [32]byte
	if _, err := io.ReadFull(random, value[:]); err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(append(
		[]byte("analytix.funds-csv-admission-"+label+"/v1\x00"),
		value[:]...,
	)), nil
}
