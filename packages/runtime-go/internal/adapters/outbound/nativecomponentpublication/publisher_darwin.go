//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentpublication

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	securegeneration "analytix.local/runtime-go/internal/adapters/outbound/securegeneration"
	domainartifact "analytix.local/runtime-go/internal/domain/artifactgeneration"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainnativebuild "analytix.local/runtime-go/internal/domain/nativebuild"

	"golang.org/x/sys/unix"
)

const (
	RequestKindV1                     = "analytix_native_generation_publish_request"
	ResponseKindV1                    = "analytix_native_generation_publish_response"
	GenerationContextKindV1           = "analytix_native_component_generation"
	PublicationIntentKindV1           = "analytix_native_publication_intent"
	SchemaVersionV1                   = 1
	PublicNameV1                      = "current"
	GenerationContextFileNameV1       = "native-component-generation.v1.json"
	PublicationIntentFileNameV1       = "native-publication-intent.v1.json"
	FrozenManifestSHA256V4            = "7265c3508f16731b5f8dbb1e08f7d645a2e8ab6d7bf4a35cf64c887e0c129aa4"
	publicationCommitDeadlineV1       = 2 * time.Minute
	maxManifestBytes                  = 256 * 1024
	maxNativeComponentBytes     int64 = 64 << 20
	maxComponentInputTotalBytes int64 = 192 << 20
	maxPublishedTotalBytes      int64 = maxComponentInputTotalBytes + (2 << 20)
)

var (
	ErrInvalidRequest = errors.New("native_component_publication_invalid_request")
	ErrInvalidInput   = errors.New("native_component_publication_invalid_input")
	ErrProbe          = errors.New("native_component_publication_probe_failed")
	ErrPublication    = errors.New("native_component_publication_failed")
	ErrCargoExecution = errors.New("native_component_publication_cargo_execution_ineligible")
)

type ExpectedCurrentV1 struct {
	Kind         string `json:"kind"`
	GenerationID string `json:"generation_id"`
}

type ToolchainProvenanceV1 struct {
	CargoExecutableSHA256 string `json:"cargo_executable_sha256"`
	CargoVersion          string `json:"cargo_version"`
	RustcExecutableSHA256 string `json:"rustc_executable_sha256"`
	RustcVersion          string `json:"rustc_version"`
}

type AuthorityProvenanceV1 struct {
	SourceSetSHA256        string `json:"source_set_sha256"`
	BuildEnvironmentSHA256 string `json:"build_environment_sha256"`
	GoToolchainKey         string `json:"go_toolchain_key"`
	GoExecutableSHA256     string `json:"go_executable_sha256"`
}

type ComponentProvenanceV1 struct {
	ID                     string `json:"id"`
	SourceDigest           string `json:"source_digest"`
	CargoLockSHA256        string `json:"cargo_lock_sha256"`
	BuildEnvironmentSHA256 string `json:"build_environment_sha256"`
}

// PublishRequestV1 carries provenance assertions and a destination CAS only.
// Executable identity, payload identity, binary name, package path, execution
// policy, probe receipt, and authority image identity are host-derived below.
type PublishRequestV1 struct {
	Kind                   string                  `json:"kind"`
	SchemaVersion          int                     `json:"schema_version"`
	RequestNonce           string                  `json:"request_nonce"`
	PublicationRoot        string                  `json:"publication_root"`
	PublicName             string                  `json:"public_name"`
	ExpectedCurrent        ExpectedCurrentV1       `json:"expected_current"`
	TargetKey              string                  `json:"target_key"`
	SourceSetSHA256        string                  `json:"source_set_sha256"`
	BuildEnvironmentSHA256 string                  `json:"build_environment_sha256"`
	Toolchain              ToolchainProvenanceV1   `json:"toolchain"`
	AuthorityProvenance    AuthorityProvenanceV1   `json:"authority_provenance"`
	Components             []ComponentProvenanceV1 `json:"components"`
}

type AuthorityIdentityV1 struct {
	SHA256    string
	Size      int64
	Platform  string
	Arch      string
	TargetKey string
}

// PublicationAuthorityV1 is process-local authority. Permit and Binding have
// no wire representation and must come from the same Go build coordinator that
// observed Cargo execution.
type PublicationAuthorityV1 struct {
	Identity AuthorityIdentityV1
	Permit   domainnativebuild.PublicationPermitV1
	Binding  domainnativebuild.PublicationBindingV1
}

type ProbeRequestV1 struct {
	ComponentID              string
	ManifestSHA256           string
	ExpectedExecutableSHA256 string
	ExpectedExecutableSize   int64
	RequestNonce             string
}

type ProbeFuncV1 func(context.Context, *os.File, ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error)

type DescriptorInputsV1 struct {
	Manifest      *os.File
	RawComponents []*os.File
	Components    []*os.File
}

type PublishResponseV1 struct {
	Kind                     string `json:"kind"`
	SchemaVersion            int    `json:"schema_version"`
	Status                   string `json:"status"`
	RequestNonce             string `json:"request_nonce"`
	PublicationBindingSHA256 string `json:"publication_binding_sha256"`
	TargetKey                string `json:"target_key"`
	GenerationID             string `json:"generation_id"`
	InventorySHA256          string `json:"inventory_sha256"`
	GenerationReceiptSHA256  string `json:"generation_receipt_sha256"`
	ComponentReceiptSHA256   string `json:"component_receipt_sha256"`
	ManifestSHA256           string `json:"manifest_sha256"`
	AuthoritySHA256          string `json:"authority_sha256"`
	PreviousGenerationID     string `json:"previous_generation_id"`
	CleanupRecovered         bool   `json:"cleanup_recovered"`
	CleanupPending           bool   `json:"cleanup_pending"`
}

type frozenComponentV1 struct {
	id           string
	binaryName   string
	packagePath  string
	policySHA256 string
}

var frozenComponentsV1 = [...]frozenComponentV1{
	{id: "import-accelerator", binaryName: "analytix-import-accelerator", packagePath: "runtime/analytix-import-accelerator", policySHA256: "0cc3872164fdadfb687a8642846913bf0f2394cd21c26799a5d483dee1ea8d2a"},
	{id: "cleaning-ops", binaryName: "analytix-cleaning-ops", packagePath: "runtime/analytix-cleaning-ops", policySHA256: "59c4287d312b4972eb6890883865e3c37d226650caf78f65168363413ac85e35"},
	{id: "analysis-compute", binaryName: "analytix-analysis-compute", packagePath: "runtime/analytix-analysis-compute", policySHA256: "fd2227234c61de1505e50aa33e1e24c2f58c2545fe4d07e41b8064709c18f4d8"},
	{id: "data-engine", binaryName: "analytix-data-engine", packagePath: "runtime/analytix-data-engine", policySHA256: "97ce13396fb6f5ba7e9d851ab4abc2f23ac8abdd13acaa3e983a326d6bf4a1e0"},
}

type targetContractV1 struct {
	key    string
	arch   string
	triple string
}

type pinnedIdentityV1 struct {
	dev, ino          uint64
	mode              uint32
	uid, gid          uint32
	nlink             uint64
	size              int64
	mtimeSec, mtimeNS int64
	ctimeSec, ctimeNS int64
}

type generationContextComponentV1 struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
}

type generationContextV1 struct {
	Kind                   string                         `json:"kind"`
	SchemaVersion          int                            `json:"schemaVersion"`
	PublicName             string                         `json:"publicName"`
	TargetKey              string                         `json:"targetKey"`
	ManifestSHA256         string                         `json:"manifestSha256"`
	ComponentReceiptSHA256 string                         `json:"componentReceiptSha256"`
	AuthoritySHA256        string                         `json:"authoritySha256"`
	Components             []generationContextComponentV1 `json:"components"`
}

type publicationIntentV1 struct {
	Kind                     string `json:"kind"`
	SchemaVersion            int    `json:"schemaVersion"`
	RequestNonce             string `json:"requestNonce"`
	PublicationBindingSHA256 string `json:"publicationBindingSha256"`
}

type preparedComponentV1 struct {
	contract frozenComponentV1
	body     []byte
	receipt  nativecomponentregistry.ComponentReceipt
}

func PublishFromDescriptorsV1(
	ctx context.Context,
	request PublishRequestV1,
	inputs DescriptorInputsV1,
	publicationAuthority PublicationAuthorityV1,
	probe ProbeFuncV1,
) (PublishResponseV1, error) {
	if validatePublicationBindingRequestV1(publicationAuthority.Binding, request, publicationAuthority.Identity) != nil {
		return PublishResponseV1{}, ErrCargoExecution
	}
	authority := publicationAuthority.Identity
	if ctx == nil || ctx.Err() != nil || validateRequestV1(request) != nil || validateAuthorityV1(authority, request.TargetKey) != nil ||
		inputs.Manifest == nil || len(inputs.RawComponents) != len(frozenComponentsV1) ||
		len(inputs.Components) != len(frozenComponentsV1) || probe == nil {
		return PublishResponseV1{}, ErrInvalidRequest
	}
	manifest, err := readPinnedRegularV1(ctx, inputs.Manifest, maxManifestBytes, 0o644, true)
	if err != nil || digestBytesV1(manifest) != FrozenManifestSHA256V4 {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	target, ok := currentTargetV1()
	if !ok || target.key != request.TargetKey || authority.TargetKey != target.key || authority.Platform != "darwin" || authority.Arch != target.arch {
		return PublishResponseV1{}, ErrInvalidRequest
	}
	pinnedRawInputs, err := preflightComponentInputsV1(inputs.RawComponents)
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	pinnedInputs, err := preflightComponentInputsV1(inputs.Components)
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	boundOutputs, err := bindPublicationOutputsV1(
		ctx,
		request.Components,
		inputs.RawComponents,
		pinnedRawInputs,
		inputs.Components,
		pinnedInputs,
	)
	publicationBindingSHA256, bindingErr := domainnativebuild.PublicationBindingSHA256V1(publicationAuthority.Binding)
	if err != nil || bindingErr != nil || domainnativebuild.ConsumeExactPublicationPermitV1(
		publicationAuthority.Permit,
		publicationAuthority.Binding,
		boundOutputs,
	) != nil {
		return PublishResponseV1{}, errors.Join(ErrCargoExecution, err, bindingErr)
	}

	preparedComponents := make([]preparedComponentV1, 0, len(frozenComponentsV1))
	componentReceipts := make([]nativecomponentregistry.ComponentReceipt, 0, len(frozenComponentsV1))
	for index, contract := range frozenComponentsV1 {
		if ctx.Err() != nil || inputs.Components[index] == nil {
			return PublishResponseV1{}, errors.Join(ErrInvalidInput, ctx.Err())
		}
		prepared, prepareErr := prepareComponentV1(
			ctx, contract, request.Components[index], boundOutputs[index], inputs.Components[index], pinnedInputs[index], target, authority, probe,
		)
		if prepareErr != nil {
			return PublishResponseV1{}, prepareErr
		}
		preparedComponents = append(preparedComponents, prepared)
		componentReceipts = append(componentReceipts, prepared.receipt)
	}

	receipt := nativecomponentregistry.Receipt{
		SchemaVersion:          nativecomponentregistry.ReceiptSchemaVersion,
		ManifestSHA256:         FrozenManifestSHA256V4,
		SourceSetSHA256:        request.SourceSetSHA256,
		BuildEnvironmentSHA256: request.BuildEnvironmentSHA256,
		Toolchain: nativecomponentregistry.ToolchainReceipt{
			CargoExecutableSHA256: request.Toolchain.CargoExecutableSHA256,
			CargoVersion:          request.Toolchain.CargoVersion,
			RustcExecutableSHA256: request.Toolchain.RustcExecutableSHA256,
			RustcVersion:          request.Toolchain.RustcVersion,
		},
		TargetKey: target.key, TargetTriple: target.triple, Platform: "darwin", Arch: target.arch,
		ExecutionAuthority: nativecomponentregistry.ExecutionAuthorityReceipt{
			SchemaVersion: 1, TrustClass: "controlled_release", Protocol: "analytix-native-build-probe-v1",
			TargetKey: target.key, BinarySHA256: authority.SHA256, BinarySize: authority.Size,
			SourceSetSHA256:        request.AuthorityProvenance.SourceSetSHA256,
			BuildEnvironmentSHA256: request.AuthorityProvenance.BuildEnvironmentSHA256,
			GoToolchainKey:         request.AuthorityProvenance.GoToolchainKey,
			GoExecutableSHA256:     request.AuthorityProvenance.GoExecutableSHA256,
		},
		PublicationAuthority: nativecomponentregistry.PublicationAuthorityReceipt{
			SchemaVersion: 1, TrustClass: "controlled_release",
			Protocol:                    domainnativebuild.CargoExecutionAuthorityProtocol,
			TargetKey:                   target.key,
			CargoExecutionID:            publicationAuthority.Binding.ExecutionID,
			CargoExecutionReceiptSHA256: publicationAuthority.Binding.ExecutionReceiptSHA256,
			PublicationBindingSHA256:    publicationBindingSHA256,
		},
		Components: componentReceipts,
	}
	componentReceiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	componentReceiptBytes = append(componentReceiptBytes, '\n')
	if _, err := nativecomponentregistry.ParseControlledBuildReceipt(componentReceiptBytes, FrozenManifestSHA256V4, target.key); err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}

	contextComponents := make([]generationContextComponentV1, 0, len(preparedComponents))
	sourceFiles := make([]domainartifact.SourceFileV1, 0, len(preparedComponents)+3)
	for _, component := range preparedComponents {
		contextComponents = append(contextComponents, generationContextComponentV1{
			ID: component.contract.id, FileName: component.contract.binaryName,
			SHA256: component.receipt.StagedImageSHA256, Size: component.receipt.StagedImageSize,
		})
		sourceFiles = append(sourceFiles, domainartifact.SourceFileV1{
			Path: component.contract.binaryName, Type: domainartifact.FileTypeNativeExecutableV1,
			Mode: domainartifact.NativeExecutableModeV1, Body: component.body,
		})
	}
	componentReceiptSHA256 := digestBytesV1(componentReceiptBytes)
	generationContextBytes, err := json.Marshal(generationContextV1{
		Kind: GenerationContextKindV1, SchemaVersion: SchemaVersionV1, PublicName: PublicNameV1,
		TargetKey: target.key, ManifestSHA256: FrozenManifestSHA256V4,
		ComponentReceiptSHA256: componentReceiptSHA256, AuthoritySHA256: authority.SHA256,
		Components: contextComponents,
	})
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	publicationIntentBytes, err := json.Marshal(publicationIntentV1{
		Kind: PublicationIntentKindV1, SchemaVersion: SchemaVersionV1,
		RequestNonce: request.RequestNonce, PublicationBindingSHA256: publicationBindingSHA256,
	})
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	sourceFiles = append(sourceFiles,
		domainartifact.SourceFileV1{Path: GenerationContextFileNameV1, Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: generationContextBytes},
		domainartifact.SourceFileV1{Path: PublicationIntentFileNameV1, Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: publicationIntentBytes},
		domainartifact.SourceFileV1{Path: nativecomponentregistry.ReceiptFileName, Type: domainartifact.FileTypeRegularV1, Mode: domainartifact.RegularFileModeV1, Body: componentReceiptBytes},
	)
	prepared, err := domainartifact.PrepareV1(sourceFiles)
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	expected, err := secureGenerationExpectationV1(request.ExpectedCurrent)
	if err != nil {
		return PublishResponseV1{}, err
	}
	commitCtx, cancelCommit, err := admitPublicationContextV1(ctx)
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrPublication, err)
	}
	defer cancelCommit()
	store, err := openGenerationStoreV1(request.PublicationRoot, expected)
	if err != nil {
		return PublishResponseV1{}, errors.Join(ErrPublication, err)
	}
	previousGenerationID := ""
	if expected.Kind == securegeneration.ExpectedCurrentGeneration {
		previousGenerationID = expected.GenerationID
	}
	result, publishErr := store.PublishExpected(commitCtx, prepared, expected)
	cleanupPending := errors.Is(publishErr, securegeneration.ErrCommittedCleanupPending)
	if result.State != securegeneration.Committed || result.Receipt != prepared.Receipt() {
		if result.State != securegeneration.Indeterminate {
			return PublishResponseV1{}, errors.Join(ErrPublication, publishErr)
		}
		// Indeterminate state is never recovered in-line. Observe is read-only;
		// it may prove this exact generation committed, but it cannot select a
		// winner, roll back residue, or turn an unknown attempt into success.
		observation, observeErr := store.Observe(commitCtx)
		if observeErr != nil || !observation.Installed || observation.Current != prepared.Receipt() {
			return PublishResponseV1{}, errors.Join(ErrPublication, publishErr, observeErr)
		}
	}
	return PublishResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "committed",
		RequestNonce: request.RequestNonce, PublicationBindingSHA256: publicationBindingSHA256, TargetKey: target.key,
		GenerationID:            prepared.Receipt().GenerationID,
		InventorySHA256:         digestBytesV1(prepared.InventoryBytes()),
		GenerationReceiptSHA256: digestBytesV1(prepared.ReceiptBytes()),
		ComponentReceiptSHA256:  componentReceiptSHA256,
		ManifestSHA256:          FrozenManifestSHA256V4, AuthoritySHA256: authority.SHA256,
		PreviousGenerationID: previousGenerationID, CleanupRecovered: false, CleanupPending: cleanupPending,
	}, nil
}

// admitPublicationContextV1 is the final linearization point between caller
// cancellation and publication mutation. context.AfterFunc and stop share one
// internal state transition: if stop loses, cancellation won and no caller-
// visible publication path may be opened; if stop wins, a detached hard
// deadline owns the atomic commit and later caller cancellation cannot create
// a half-commit by interrupting rollback/readback.
func admitPublicationContextV1(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, nil, errors.Join(ErrPublication, contextErrorV1(ctx))
	}
	cancelObserved := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { close(cancelObserved) })
	if !stop() {
		<-cancelObserved
		return nil, nil, errors.Join(ErrPublication, ctx.Err())
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publicationCommitDeadlineV1)
	return commitCtx, cancel, nil
}

func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}

func prepareComponentV1(
	ctx context.Context,
	contract frozenComponentV1,
	provenance ComponentProvenanceV1,
	observedOutput domainnativebuild.CargoOutputV1,
	file *os.File,
	expected pinnedIdentityV1,
	target targetContractV1,
	authority AuthorityIdentityV1,
	probe ProbeFuncV1,
) (preparedComponentV1, error) {
	before, err := validateExecutableDescriptorV1(file)
	if err != nil || before != expected {
		return preparedComponentV1{}, errors.Join(ErrInvalidInput, err)
	}
	initialSHA256, err := hashPinnedFileV1(ctx, file, before.size)
	if err != nil {
		return preparedComponentV1{}, errors.Join(ErrInvalidInput, err)
	}
	nonce, err := randomDigestV1()
	if err != nil {
		return preparedComponentV1{}, errors.Join(ErrProbe, err)
	}
	probeReceipt, err := probe(ctx, file, ProbeRequestV1{
		ComponentID: contract.id, ManifestSHA256: FrozenManifestSHA256V4,
		ExpectedExecutableSHA256: initialSHA256, ExpectedExecutableSize: before.size, RequestNonce: nonce,
	})
	if err != nil || !validProbeReceiptV1(probeReceipt, contract, initialSHA256, before.size, nonce, authority) {
		return preparedComponentV1{}, errors.Join(ErrProbe, err)
	}
	body, err := readPinnedExecutableV1(ctx, file, before, initialSHA256)
	if err != nil {
		return preparedComponentV1{}, errors.Join(ErrInvalidInput, err)
	}
	payloadSHA256, payloadSize, arch, err := nativecomponentregistry.InspectDarwinPayload(file, before.size)
	if err != nil || arch != target.arch || observedOutput.ID != contract.id ||
		observedOutput.PayloadSHA256 != payloadSHA256 || observedOutput.PayloadSize != payloadSize ||
		observedOutput.Format != "mach-o" || observedOutput.Arch != arch {
		return preparedComponentV1{}, errors.Join(ErrInvalidInput, err)
	}
	finalSHA256, err := hashPinnedFileV1(ctx, file, before.size)
	after, statErr := validateExecutableDescriptorV1(file)
	if err != nil || statErr != nil || after != before || finalSHA256 != initialSHA256 || digestBytesV1(body) != initialSHA256 {
		return preparedComponentV1{}, errors.Join(ErrInvalidInput, err, statErr)
	}
	receipt := nativecomponentregistry.ComponentReceipt{
		ID: contract.id, BinaryName: contract.binaryName, PackagePath: contract.packagePath,
		SourceDigest: provenance.SourceDigest, CargoLockSHA256: provenance.CargoLockSHA256,
		BuildEnvironmentSHA256: provenance.BuildEnvironmentSHA256,
		RawBuildSHA256:         observedOutput.RawBuildSHA256, RawBuildSize: observedOutput.RawBuildSize,
		StagedImageSHA256: initialSHA256, StagedImageSize: before.size,
		PayloadSHA256: payloadSHA256, PayloadSize: payloadSize, Format: "mach-o", Arch: target.arch,
		ExecutionProbe: probeReceipt,
	}
	return preparedComponentV1{contract: contract, body: body, receipt: receipt}, nil
}

func preflightComponentInputsV1(files []*os.File) ([]pinnedIdentityV1, error) {
	if len(files) != len(frozenComponentsV1) {
		return nil, ErrInvalidInput
	}
	identities := make([]pinnedIdentityV1, len(files))
	var total int64
	for index, file := range files {
		identity, err := validateExecutableDescriptorV1(file)
		if err != nil || identity.size > maxComponentInputTotalBytes-total {
			return nil, errors.Join(ErrInvalidInput, err)
		}
		identities[index] = identity
		total += identity.size
	}
	return identities, nil
}

func bindPublicationOutputsV1(
	ctx context.Context,
	provenance []ComponentProvenanceV1,
	rawFiles []*os.File,
	rawIdentities []pinnedIdentityV1,
	files []*os.File,
	identities []pinnedIdentityV1,
) ([]domainnativebuild.CargoOutputV1, error) {
	if ctx == nil || ctx.Err() != nil || len(provenance) != len(frozenComponentsV1) ||
		len(rawFiles) != len(frozenComponentsV1) || len(rawIdentities) != len(frozenComponentsV1) ||
		len(files) != len(frozenComponentsV1) || len(identities) != len(frozenComponentsV1) {
		return nil, ErrInvalidInput
	}
	outputs := make([]domainnativebuild.CargoOutputV1, 0, len(frozenComponentsV1))
	for index, contract := range frozenComponentsV1 {
		rawBefore, err := validateExecutableDescriptorV1(rawFiles[index])
		if err != nil || rawBefore != rawIdentities[index] {
			return nil, errors.Join(ErrInvalidInput, err)
		}
		rawBuildSHA256, err := hashPinnedFileV1(ctx, rawFiles[index], rawBefore.size)
		rawPayloadSHA256, rawPayloadSize, rawArch, rawPayloadErr := nativecomponentregistry.InspectDarwinPayload(rawFiles[index], rawBefore.size)
		rawAfter, rawStatErr := validateExecutableDescriptorV1(rawFiles[index])
		if err != nil || rawPayloadErr != nil || rawStatErr != nil || rawAfter != rawBefore {
			return nil, errors.Join(ErrInvalidInput, err, rawPayloadErr, rawStatErr)
		}
		before, err := validateExecutableDescriptorV1(files[index])
		if err != nil || before != identities[index] {
			return nil, errors.Join(ErrInvalidInput, err)
		}
		payloadSHA256, payloadSize, arch, payloadErr := nativecomponentregistry.InspectDarwinPayload(files[index], before.size)
		after, statErr := validateExecutableDescriptorV1(files[index])
		if err != nil || payloadErr != nil || statErr != nil || after != before ||
			rawPayloadSHA256 != payloadSHA256 || rawPayloadSize != payloadSize || rawArch != arch {
			return nil, errors.Join(ErrInvalidInput, err, payloadErr, statErr)
		}
		component := provenance[index]
		outputs = append(outputs, domainnativebuild.CargoOutputV1{
			SchemaVersion:          1,
			ID:                     contract.id,
			SourceDigest:           component.SourceDigest,
			CargoLockSHA256:        component.CargoLockSHA256,
			BuildEnvironmentSHA256: component.BuildEnvironmentSHA256,
			RawBuildSHA256:         rawBuildSHA256,
			RawBuildSize:           rawBefore.size,
			PayloadSHA256:          rawPayloadSHA256,
			PayloadSize:            rawPayloadSize,
			Format:                 "mach-o",
			Arch:                   arch,
		})
	}
	return outputs, nil
}

func validatePublicationBindingRequestV1(
	binding domainnativebuild.PublicationBindingV1,
	request PublishRequestV1,
	authority AuthorityIdentityV1,
) error {
	target, ok := currentTargetV1()
	if !ok || binding.TargetKey != target.key || binding.TargetTriple != target.triple ||
		binding.RequestNonce != request.RequestNonce || binding.ManifestSHA256 != FrozenManifestSHA256V4 ||
		binding.SourceSetSHA256 != request.SourceSetSHA256 ||
		binding.BuildEnvironmentSHA256 != request.BuildEnvironmentSHA256 ||
		binding.Toolchain.CargoExecutableSHA256 != request.Toolchain.CargoExecutableSHA256 ||
		binding.Toolchain.CargoVersion != request.Toolchain.CargoVersion ||
		binding.Toolchain.RustcExecutableSHA256 != request.Toolchain.RustcExecutableSHA256 ||
		binding.Toolchain.RustcVersion != request.Toolchain.RustcVersion ||
		binding.Authority.Protocol != domainnativebuild.CargoExecutionAuthorityProtocol ||
		binding.Authority.TargetKey != request.TargetKey || binding.Authority.BinarySHA256 != authority.SHA256 ||
		binding.Authority.SourceSetSHA256 != request.AuthorityProvenance.SourceSetSHA256 ||
		binding.Authority.BuildEnvironmentSHA256 != request.AuthorityProvenance.BuildEnvironmentSHA256 ||
		binding.Authority.GoExecutableSHA256 != request.AuthorityProvenance.GoExecutableSHA256 ||
		len(binding.Outputs) != len(request.Components) {
		return ErrCargoExecution
	}
	for index, output := range binding.Outputs {
		component := request.Components[index]
		if output.ID != component.ID || output.SourceDigest != component.SourceDigest ||
			output.CargoLockSHA256 != component.CargoLockSHA256 ||
			output.BuildEnvironmentSHA256 != component.BuildEnvironmentSHA256 {
			return ErrCargoExecution
		}
	}
	return nil
}

func validateRequestV1(request PublishRequestV1) error {
	target, ok := currentTargetV1()
	if !ok || request.Kind != RequestKindV1 || request.SchemaVersion != SchemaVersionV1 || !validDigestV1(request.RequestNonce) ||
		request.PublicName != PublicNameV1 || request.TargetKey != target.key || !validPublicationRootV1(request.PublicationRoot) ||
		!validDigestV1(request.SourceSetSHA256) || !validDigestV1(request.BuildEnvironmentSHA256) ||
		!validToolchainV1(request.Toolchain) || !validAuthorityProvenanceV1(request.AuthorityProvenance, target.key) ||
		len(request.Components) != len(frozenComponentsV1) || validateExpectedCurrentV1(request.ExpectedCurrent) != nil {
		return ErrInvalidRequest
	}
	for index, contract := range frozenComponentsV1 {
		component := request.Components[index]
		if component.ID != contract.id || !validDigestV1(component.SourceDigest) || !validDigestV1(component.CargoLockSHA256) ||
			!validDigestV1(component.BuildEnvironmentSHA256) {
			return ErrInvalidRequest
		}
	}
	return nil
}

func validateAuthorityV1(authority AuthorityIdentityV1, targetKey string) error {
	target, ok := currentTargetV1()
	if !ok || authority.TargetKey != targetKey || authority.TargetKey != target.key || authority.Platform != "darwin" ||
		authority.Arch != target.arch || authority.Size <= 0 || authority.Size > maxNativeComponentBytes || !validDigestV1(authority.SHA256) {
		return ErrInvalidRequest
	}
	return nil
}

func validateExpectedCurrentV1(expected ExpectedCurrentV1) error {
	switch expected.Kind {
	case string(securegeneration.ExpectedCurrentAbsent):
		if expected.GenerationID == "" {
			return nil
		}
	case string(securegeneration.ExpectedCurrentGeneration):
		if validDigestV1(expected.GenerationID) {
			return nil
		}
	}
	return ErrInvalidRequest
}

func secureGenerationExpectationV1(expected ExpectedCurrentV1) (securegeneration.ExpectedCurrent, error) {
	if err := validateExpectedCurrentV1(expected); err != nil {
		return securegeneration.ExpectedCurrent{}, err
	}
	return securegeneration.ExpectedCurrent{Kind: securegeneration.ExpectedCurrentKind(expected.Kind), GenerationID: expected.GenerationID}, nil
}

func openGenerationStoreV1(root string, expected securegeneration.ExpectedCurrent) (*securegeneration.Store, error) {
	limits := securegeneration.Limits{
		MaxFiles: len(frozenComponentsV1) + 3, MaxFileBytes: maxNativeComponentBytes,
		MaxTotalBytes: maxPublishedTotalBytes, MaxDepth: 1,
	}
	if expected.Kind == securegeneration.ExpectedCurrentGeneration {
		return securegeneration.OpenExisting(root, PublicNameV1, limits)
	}
	return securegeneration.Open(root, PublicNameV1, limits)
}

func validProbeReceiptV1(
	receipt nativecomponentregistry.ExecutionProbeReceipt,
	contract frozenComponentV1,
	executableSHA256 string,
	executableSize int64,
	nonce string,
	authority AuthorityIdentityV1,
) bool {
	return receipt.Kind == "analytix_native_build_probe_receipt" && receipt.SchemaVersion == 1 && receipt.Status == "passed" &&
		receipt.ComponentID == contract.id && receipt.RequestNonce == nonce && receipt.ExecutableSHA256 == executableSHA256 &&
		receipt.ExecutableSize == executableSize && receipt.ManifestSHA256 == FrozenManifestSHA256V4 &&
		receipt.PolicySHA256 == contract.policySHA256 && receipt.AuthoritySHA256 == authority.SHA256 &&
		receipt.HostPlatform == "darwin" && receipt.HostArch == authority.Arch && receipt.LoadedImageBound &&
		receipt.WorkingDirectoryBound && receipt.GuardianAuthenticated && receipt.ProcessTreeEmpty
}

func validateExecutableDescriptorV1(file *os.File) (pinnedIdentityV1, error) {
	if file == nil {
		return pinnedIdentityV1{}, ErrInvalidInput
	}
	var stat unix.Stat_t
	if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 ||
		stat.Size <= 0 || stat.Size > maxNativeComponentBytes || uint32(stat.Mode&0o7777) != 0o755 ||
		(stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) {
		return pinnedIdentityV1{}, ErrInvalidInput
	}
	return pinnedIdentityV1{
		dev: uint64(stat.Dev), ino: stat.Ino, mode: uint32(stat.Mode), uid: stat.Uid, gid: stat.Gid,
		nlink: uint64(stat.Nlink), size: stat.Size, mtimeSec: stat.Mtim.Sec, mtimeNS: stat.Mtim.Nsec,
		ctimeSec: stat.Ctim.Sec, ctimeNS: stat.Ctim.Nsec,
	}, nil
}

func readPinnedRegularV1(ctx context.Context, file *os.File, limit int64, expectedMode uint32, requireNonEmpty bool) ([]byte, error) {
	if ctx == nil || file == nil || limit <= 0 || ctx.Err() != nil {
		return nil, ErrInvalidInput
	}
	var before unix.Stat_t
	if unix.Fstat(int(file.Fd()), &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 ||
		before.Size < 0 || before.Size > limit || requireNonEmpty && before.Size == 0 || uint32(before.Mode&0o7777) != expectedMode ||
		(before.Uid != 0 && before.Uid != uint32(os.Geteuid())) {
		return nil, ErrInvalidInput
	}
	body, err := readExactAtV1(ctx, file, before.Size)
	var after unix.Stat_t
	if err != nil || unix.Fstat(int(file.Fd()), &after) != nil || !sameUnixStatV1(before, after) {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	return body, nil
}

func readPinnedExecutableV1(ctx context.Context, file *os.File, expected pinnedIdentityV1, expectedSHA256 string) ([]byte, error) {
	current, err := validateExecutableDescriptorV1(file)
	if err != nil || current != expected {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	body, err := readExactAtV1(ctx, file, expected.size)
	after, statErr := validateExecutableDescriptorV1(file)
	if err != nil || statErr != nil || after != expected || digestBytesV1(body) != expectedSHA256 {
		return nil, errors.Join(ErrInvalidInput, err, statErr)
	}
	return body, nil
}

func readExactAtV1(ctx context.Context, file *os.File, size int64) ([]byte, error) {
	if ctx == nil || file == nil || size < 0 || size > maxNativeComponentBytes || ctx.Err() != nil {
		return nil, ErrInvalidInput
	}
	body := make([]byte, int(size))
	for offset := int64(0); offset < size; {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		end := offset + (1 << 20)
		if end > size {
			end = size
		}
		read, err := file.ReadAt(body[offset:end], offset)
		if err != nil && !errors.Is(err, io.EOF) || int64(read) != end-offset {
			return nil, ErrInvalidInput
		}
		offset = end
	}
	var extra [1]byte
	if read, err := file.ReadAt(extra[:], size); read != 0 || !errors.Is(err, io.EOF) {
		return nil, ErrInvalidInput
	}
	return body, nil
}

func hashPinnedFileV1(ctx context.Context, file *os.File, size int64) (string, error) {
	if ctx == nil || file == nil || size <= 0 || size > maxNativeComponentBytes || ctx.Err() != nil {
		return "", ErrInvalidInput
	}
	hash := sha256.New()
	buffer := make([]byte, 1<<20)
	for offset := int64(0); offset < size; {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		length := int64(len(buffer))
		if remaining := size - offset; remaining < length {
			length = remaining
		}
		read, err := file.ReadAt(buffer[:length], offset)
		if err != nil && !errors.Is(err, io.EOF) || int64(read) != length {
			return "", ErrInvalidInput
		}
		if _, err := hash.Write(buffer[:read]); err != nil {
			return "", err
		}
		offset += int64(read)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameUnixStatV1(left, right unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino && left.Mode == right.Mode && left.Uid == right.Uid &&
		left.Gid == right.Gid && left.Nlink == right.Nlink && left.Size == right.Size &&
		left.Mtim == right.Mtim && left.Ctim == right.Ctim
}

func currentTargetV1() (targetContractV1, bool) {
	switch runtime.GOARCH {
	case "arm64":
		return targetContractV1{key: "darwin-arm64", arch: "arm64", triple: "aarch64-apple-darwin"}, true
	case "amd64":
		return targetContractV1{key: "darwin-x64", arch: "x64", triple: "x86_64-apple-darwin"}, true
	default:
		return targetContractV1{}, false
	}
}

func validPublicationRootV1(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && filepath.Clean(value) == value &&
		value != string(filepath.Separator) && !strings.ContainsRune(value, 0)
}

func validToolchainV1(value ToolchainProvenanceV1) bool {
	return validDigestV1(value.CargoExecutableSHA256) && validDigestV1(value.RustcExecutableSHA256) &&
		strings.HasPrefix(value.CargoVersion, "cargo 1.94.1 ") && strings.HasPrefix(value.RustcVersion, "rustc 1.94.1 ") &&
		len(value.CargoVersion) <= 256 && len(value.RustcVersion) <= 256
}

func validAuthorityProvenanceV1(value AuthorityProvenanceV1, targetKey string) bool {
	return validDigestV1(value.SourceSetSHA256) && validDigestV1(value.BuildEnvironmentSHA256) &&
		validDigestV1(value.GoExecutableSHA256) && value.GoToolchainKey == targetKey
}

func validDigestV1(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func digestBytesV1(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func randomDigestV1() (string, error) {
	var nonce [sha256.Size]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

func DecodePublishRequestV1(raw []byte) (PublishRequestV1, error) {
	var request PublishRequestV1
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 256 * 1024, MaxDepth: 8, MaxTokens: 1024,
		MaxStringBytes: 64 * 1024, MaxNumberBytes: 32, MaxAbsExponent: 1,
	}); err != nil {
		return request, ErrInvalidRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return PublishRequestV1{}, ErrInvalidRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PublishRequestV1{}, ErrInvalidRequest
	}
	if validateRequestV1(request) != nil {
		return PublishRequestV1{}, ErrInvalidRequest
	}
	return request, nil
}
