//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentsnapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	securegeneration "analytix.local/runtime-go/internal/adapters/outbound/securegeneration"
	domainartifact "analytix.local/runtime-go/internal/domain/artifactgeneration"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"

	"golang.org/x/sys/unix"
)

const (
	ArgumentV1                         = "--analytix-native-source-snapshot-v1"
	ArgumentDiscardV1                  = "--analytix-native-source-snapshot-discard-v1"
	RequestKindV1                      = "analytix_native_source_snapshot_request"
	ResponseKindV1                     = "analytix_native_source_snapshot_response"
	ContextKindV1                      = "analytix_native_source_snapshot"
	SchemaVersionV1                    = 1
	OperationCreateV1                  = "create"
	OperationVerifyV1                  = "verify"
	OperationDiscardV1                 = "discard"
	OperationReconcileDiscardV1        = "reconcile_discard"
	DispositionGenerationDiscardedV1   = "generation_discarded"
	DispositionEmptySessionDiscardedV1 = "empty_session_discarded"
	DispositionAlreadyDiscardedV1      = "already_discarded"
	RootNameV1                         = "source"
	PublicNameV1                       = "current"
	ContextFileNameV1                  = "native-source-snapshot.v1.json"
	ManifestSnapshotPathV1             = "scripts/native-components.json"
	maxSourceFilesV1                   = 4096
	maxSourceFileBytesV1               = int64(16 << 20)
	maxSourceTotalBytesV1              = int64(128 << 20)
	maxSourceDepthV1                   = 32
	maxSourceDirectoriesV1             = 1024
	maxSourceComponentBytesV1          = 255
	requestLimitV1                     = 4 << 10
	sessionNamePrefixV1                = "analytix-native-source-session-"
	sessionNameSuffixBytesV1           = 6
)

var (
	ErrInvalidInput  = errors.New("native_source_snapshot_invalid_input")
	ErrSourceChanged = errors.New("native_source_snapshot_source_changed")
	ErrGeneration    = errors.New("native_source_snapshot_generation_mismatch")
	ErrPublication   = errors.New("native_source_snapshot_publication_failed")
	ErrDestruction   = errors.New("native_source_snapshot_destruction_failed")
)

type RequestV1 struct {
	Kind                            string `json:"kind"`
	SchemaVersion                   int    `json:"schema_version"`
	Operation                       string `json:"operation"`
	RequestNonce                    string `json:"request_nonce"`
	ExpectedGenerationID            string `json:"expected_generation_id"`
	ExpectedInventorySHA256         string `json:"expected_inventory_sha256"`
	ExpectedGenerationReceiptSHA256 string `json:"expected_generation_receipt_sha256"`
	SessionName                     string `json:"session_name"`
}

type ComponentReceiptV1 struct {
	ID              string `json:"id"`
	SourceRoot      string `json:"source_root"`
	SourceDigest    string `json:"source_digest"`
	CargoLockSHA256 string `json:"cargo_lock_sha256"`
	FileCount       int    `json:"file_count"`
	TotalBytes      int64  `json:"total_bytes"`
}

type ResponseV1 struct {
	Kind                    string               `json:"kind"`
	SchemaVersion           int                  `json:"schema_version"`
	Status                  string               `json:"status"`
	Operation               string               `json:"operation"`
	RequestNonce            string               `json:"request_nonce"`
	SessionName             string               `json:"session_name"`
	ManifestSHA256          string               `json:"manifest_sha256"`
	GenerationID            string               `json:"generation_id"`
	InventorySHA256         string               `json:"inventory_sha256"`
	GenerationReceiptSHA256 string               `json:"generation_receipt_sha256"`
	FileCount               uint64               `json:"file_count"`
	TotalBytes              uint64               `json:"total_bytes"`
	Components              []ComponentReceiptV1 `json:"components"`
}

type DiscardResponseV1 struct {
	Kind                    string `json:"kind"`
	SchemaVersion           int    `json:"schema_version"`
	Status                  string `json:"status"`
	Operation               string `json:"operation"`
	RequestNonce            string `json:"request_nonce"`
	SessionName             string `json:"session_name"`
	Disposition             string `json:"disposition"`
	GenerationID            string `json:"generation_id"`
	InventorySHA256         string `json:"inventory_sha256"`
	GenerationReceiptSHA256 string `json:"generation_receipt_sha256"`
	SessionRemoved          bool   `json:"session_removed"`
}

type contextV1 struct {
	Kind                    string               `json:"kind"`
	SchemaVersion           int                  `json:"schemaVersion"`
	ManifestSHA256          string               `json:"manifestSha256"`
	EmptyDirectoryPolicy    string               `json:"emptyDirectoryPolicy"`
	OmittedEmptyDirectories []string             `json:"omittedEmptyDirectories"`
	Components              []contextComponentV1 `json:"components"`
}

type contextComponentV1 struct {
	ID              string `json:"id"`
	SourceRoot      string `json:"sourceRoot"`
	SourceDigest    string `json:"sourceDigest"`
	CargoLockSHA256 string `json:"cargoLockSha256"`
	FileCount       int    `json:"fileCount"`
	TotalBytes      int64  `json:"totalBytes"`
}

type sourceIdentityV1 struct {
	dev, ino          uint64
	mode              uint32
	uid, gid          uint32
	nlink             uint64
	size              int64
	mtimeSec, mtimeNS int64
	ctimeSec, ctimeNS int64
}

type pinnedDirectoryV1 struct {
	path       string
	fd         int
	identity   sourceIdentityV1
	entries    []string
	entryState map[string]sourceIdentityV1
}

type pinnedFileV1 struct {
	path     string
	fd       int
	identity sourceIdentityV1
	body     []byte
	sha256   string
}

type scannerV1 struct {
	ctx          context.Context
	repositoryFD int
	repository   sourceIdentityV1
	directories  []*pinnedDirectoryV1
	files        []*pinnedFileV1
	byPath       map[string]*pinnedFileV1
	totalBytes   int64
}

type snapshotMaterialV1 struct {
	prepared   domainartifact.PreparedV1
	components []ComponentReceiptV1
	scanner    *scannerV1
}

// CreateFromDescriptorsV1 copies the four frozen Rust source roots from an
// identity-pinned repository descriptor into an immutable generation under an
// identity-pinned private parent. No caller path, exclusion list, source
// digest, or publication name is accepted.
func CreateFromDescriptorsV1(
	ctx context.Context,
	request RequestV1,
	manifestInput *os.File,
	repository *os.File,
	privateParent *os.File,
) (response ResponseV1, returnErr error) {
	if validateRequestV1(request, OperationCreateV1) != nil || ctx == nil || manifestInput == nil ||
		repository == nil || privateParent == nil || ctx.Err() != nil {
		return ResponseV1{}, ErrInvalidInput
	}
	if err := validatePrivateParentEntriesV1(privateParent, nil); err != nil {
		return ResponseV1{}, errors.Join(ErrInvalidInput, err)
	}
	material, err := prepareSnapshotMaterialV1(ctx, manifestInput, repository)
	if err != nil {
		return ResponseV1{}, err
	}
	defer func() {
		if closeErr := material.scanner.close(); closeErr != nil {
			response = ResponseV1{}
			returnErr = errors.Join(returnErr, ErrSourceChanged, closeErr)
		}
	}()
	if err := material.scanner.revalidate(); err != nil {
		return ResponseV1{}, errors.Join(ErrSourceChanged, err)
	}
	result, err := securegeneration.PublishExpectedUnder(
		ctx,
		privateParent,
		RootNameV1,
		PublicNameV1,
		material.prepared,
		securegeneration.ExpectedCurrent{Kind: securegeneration.ExpectedCurrentAbsent},
		securegeneration.Limits{
			MaxFiles: maxSourceFilesV1, MaxFileBytes: maxSourceFileBytesV1,
			MaxTotalBytes: maxSourceTotalBytesV1, MaxDepth: maxSourceDepthV1,
		},
	)
	if err != nil || result.State != securegeneration.Committed || result.Receipt != material.prepared.Receipt() {
		return ResponseV1{}, errors.Join(ErrPublication, err)
	}
	if err := material.scanner.revalidate(); err != nil {
		return ResponseV1{}, errors.Join(ErrSourceChanged, err)
	}
	if err := validatePrivateParentEntriesV1(privateParent, []string{RootNameV1}); err != nil {
		return ResponseV1{}, errors.Join(ErrPublication, err)
	}
	return material.response(request, "committed"), nil
}

// VerifyFromDescriptorsV1 proves that the live descriptor-pinned sources and
// the immutable generation under the same private parent still describe the
// exact generation created before Cargo ran. It never creates, repairs, or
// promotes filesystem state.
func VerifyFromDescriptorsV1(
	ctx context.Context,
	request RequestV1,
	manifestInput *os.File,
	repository *os.File,
	privateParent *os.File,
) (response ResponseV1, returnErr error) {
	if validateRequestV1(request, OperationVerifyV1) != nil || ctx == nil || manifestInput == nil ||
		repository == nil || privateParent == nil || ctx.Err() != nil {
		return ResponseV1{}, ErrInvalidInput
	}
	if err := validatePrivateParentEntriesV1(privateParent, []string{RootNameV1}); err != nil {
		return ResponseV1{}, errors.Join(ErrGeneration, err)
	}
	material, err := prepareSnapshotMaterialV1(ctx, manifestInput, repository)
	if err != nil {
		return ResponseV1{}, err
	}
	defer func() {
		if closeErr := material.scanner.close(); closeErr != nil {
			response = ResponseV1{}
			returnErr = errors.Join(returnErr, ErrSourceChanged, closeErr)
		}
	}()
	if material.prepared.Receipt().GenerationID != request.ExpectedGenerationID ||
		domainartifact.DigestBytesV1(material.prepared.InventoryBytes()) != request.ExpectedInventorySHA256 ||
		domainartifact.DigestBytesV1(material.prepared.ReceiptBytes()) != request.ExpectedGenerationReceiptSHA256 {
		return ResponseV1{}, ErrGeneration
	}
	if err := material.scanner.revalidate(); err != nil {
		return ResponseV1{}, errors.Join(ErrSourceChanged, err)
	}
	observation, err := securegeneration.ObserveUnder(
		ctx, privateParent, RootNameV1, PublicNameV1,
		securegeneration.Limits{
			MaxFiles: maxSourceFilesV1, MaxFileBytes: maxSourceFileBytesV1,
			MaxTotalBytes: maxSourceTotalBytesV1, MaxDepth: maxSourceDepthV1,
		},
	)
	if err != nil || !observation.Installed || observation.Previous != nil ||
		observation.Current != material.prepared.Receipt() {
		return ResponseV1{}, errors.Join(ErrGeneration, err)
	}
	if err := material.scanner.revalidate(); err != nil {
		return ResponseV1{}, errors.Join(ErrSourceChanged, err)
	}
	if err := validatePrivateParentEntriesV1(privateParent, []string{RootNameV1}); err != nil {
		return ResponseV1{}, errors.Join(ErrGeneration, err)
	}
	return material.response(request, "verified"), nil
}

// DiscardFromDescriptorsV1 removes only the exact generation observed under
// the descriptor-bound private session. It deliberately does not rescan live
// repository sources: cleanup must remain possible after source drift or a
// Cargo failure.
func DiscardFromDescriptorsV1(
	ctx context.Context,
	request RequestV1,
	privateParent *os.File,
) (DiscardResponseV1, error) {
	if validateRequestV1(request, OperationDiscardV1) != nil || ctx == nil ||
		privateParent == nil || ctx.Err() != nil {
		return DiscardResponseV1{}, ErrInvalidInput
	}
	if err := validatePrivateParentEntriesV1(privateParent, []string{RootNameV1}); err != nil {
		return DiscardResponseV1{}, errors.Join(ErrGeneration, err)
	}
	limits := securegeneration.Limits{
		MaxFiles: maxSourceFilesV1, MaxFileBytes: maxSourceFileBytesV1,
		MaxTotalBytes: maxSourceTotalBytesV1, MaxDepth: maxSourceDepthV1,
	}
	observation, err := securegeneration.ObserveUnder(ctx, privateParent, RootNameV1, PublicNameV1, limits)
	if err != nil || !observation.Installed || observation.Previous != nil {
		return DiscardResponseV1{}, errors.Join(ErrGeneration, err)
	}
	receiptBody, err := json.Marshal(observation.Current)
	if err != nil {
		return DiscardResponseV1{}, errors.Join(ErrGeneration, err)
	}
	response := DiscardResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "discarded",
		Operation: request.Operation, RequestNonce: request.RequestNonce, SessionName: request.SessionName,
		Disposition:  DispositionGenerationDiscardedV1,
		GenerationID: observation.Current.GenerationID, InventorySHA256: observation.Current.InventoryDigest,
		GenerationReceiptSHA256: domainartifact.DigestBytesV1(receiptBody),
	}
	if response.GenerationID != request.ExpectedGenerationID ||
		response.InventorySHA256 != request.ExpectedInventorySHA256 ||
		response.GenerationReceiptSHA256 != request.ExpectedGenerationReceiptSHA256 {
		return DiscardResponseV1{}, ErrGeneration
	}
	if err := securegeneration.DiscardExpectedUnder(
		ctx, privateParent, RootNameV1, PublicNameV1, observation.Current, limits,
	); err != nil {
		return DiscardResponseV1{}, errors.Join(ErrDestruction, err)
	}
	if err := validatePrivateParentEntriesV1(privateParent, nil); err != nil {
		return DiscardResponseV1{}, errors.Join(ErrDestruction, err)
	}
	return response, nil
}

// ReconcileDiscardFromDescriptorsV1 removes only cleanup state found inside
// the already descriptor-bound private session. It accepts no caller-supplied
// generation identity and never upgrades the result of an earlier create,
// verify, or discard attempt. Unknown entries and transaction residue remain
// quarantined.
func ReconcileDiscardFromDescriptorsV1(
	ctx context.Context,
	request RequestV1,
	privateParent *os.File,
) (DiscardResponseV1, error) {
	if validateRequestV1(request, OperationReconcileDiscardV1) != nil || ctx == nil ||
		privateParent == nil || ctx.Err() != nil {
		return DiscardResponseV1{}, ErrInvalidInput
	}
	response := DiscardResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "discarded",
		Operation: request.Operation, RequestNonce: request.RequestNonce, SessionName: request.SessionName,
	}
	if err := validatePrivateParentEntriesV1(privateParent, nil); err == nil {
		response.Disposition = DispositionEmptySessionDiscardedV1
		return response, nil
	}
	if err := validatePrivateParentEntriesV1(privateParent, []string{RootNameV1}); err != nil {
		return DiscardResponseV1{}, errors.Join(ErrGeneration, err)
	}
	result, err := securegeneration.DiscardCurrentUnder(
		ctx,
		privateParent,
		RootNameV1,
		PublicNameV1,
		securegeneration.Limits{
			MaxFiles: maxSourceFilesV1, MaxFileBytes: maxSourceFileBytesV1,
			MaxTotalBytes: maxSourceTotalBytesV1, MaxDepth: maxSourceDepthV1,
		},
	)
	if err != nil {
		return DiscardResponseV1{}, errors.Join(ErrDestruction, err)
	}
	if result.Installed {
		response.Disposition = DispositionGenerationDiscardedV1
		response.GenerationID = result.Current.GenerationID
		response.InventorySHA256 = result.Current.InventoryDigest
		response.GenerationReceiptSHA256 = result.ReceiptDigest
	} else {
		response.Disposition = DispositionEmptySessionDiscardedV1
	}
	if err := validatePrivateParentEntriesV1(privateParent, nil); err != nil {
		return DiscardResponseV1{}, errors.Join(ErrDestruction, err)
	}
	return response, nil
}

func prepareSnapshotMaterialV1(
	ctx context.Context,
	manifestInput *os.File,
	repository *os.File,
) (material snapshotMaterialV1, returnErr error) {
	if ctx == nil || manifestInput == nil || repository == nil || ctx.Err() != nil {
		return snapshotMaterialV1{}, ErrInvalidInput
	}
	manifestBody, manifestIdentity, err := readPinnedManifestV1(ctx, manifestInput)
	if err != nil {
		return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
	}
	manifest, err := nativecomponentregistry.ParseFrozenManifestV4(manifestBody)
	if err != nil {
		return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
	}
	scanner, err := newScannerV1(ctx, repository)
	if err != nil {
		return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
	}
	keepScanner := false
	defer func() {
		if !keepScanner {
			returnErr = errors.Join(returnErr, scanner.close())
		}
	}()
	if err := scanner.scanRequiredRepositoryFile(ManifestSnapshotPathV1, manifestIdentity); err != nil {
		return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
	}
	components := make([]ComponentReceiptV1, 0, len(manifest.Components))
	contextComponents := make([]contextComponentV1, 0, len(manifest.Components))
	for _, component := range manifest.Components {
		if err := scanner.scanComponent(component); err != nil {
			return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
		}
		receipt, err := scanner.componentReceipt(component)
		if err != nil {
			return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
		}
		components = append(components, receipt)
		contextComponents = append(contextComponents, contextComponentV1{
			ID: receipt.ID, SourceRoot: receipt.SourceRoot, SourceDigest: receipt.SourceDigest,
			CargoLockSHA256: receipt.CargoLockSHA256, FileCount: receipt.FileCount, TotalBytes: receipt.TotalBytes,
		})
	}
	for _, path := range []string{"rust-toolchain", "rust-toolchain.toml"} {
		if err := scanner.scanOptionalRepositoryFile(path); err != nil {
			return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
		}
	}
	for _, path := range []string{".cargo", "tools/.cargo"} {
		present, err := scanner.repositoryEntryPresent(path)
		if err != nil || present {
			return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
		}
	}
	contextBody, err := json.Marshal(contextV1{
		Kind: ContextKindV1, SchemaVersion: SchemaVersionV1,
		ManifestSHA256:          nativecomponentregistry.FrozenManifestSHA256V4,
		EmptyDirectoryPolicy:    "bound_omitted_non_payload_v1",
		OmittedEmptyDirectories: scanner.omittedEmptyDirectories(),
		Components:              contextComponents,
	})
	if err != nil {
		return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
	}
	sourceFiles := make([]domainartifact.SourceFileV1, 0, len(scanner.files)+1)
	for _, file := range scanner.files {
		sourceFiles = append(sourceFiles, domainartifact.SourceFileV1{
			Path: file.path, Type: domainartifact.FileTypeRegularV1,
			Mode: domainartifact.RegularFileModeV1, Body: file.body,
		})
	}
	sourceFiles = append(sourceFiles, domainartifact.SourceFileV1{
		Path: ContextFileNameV1, Type: domainartifact.FileTypeRegularV1,
		Mode: domainartifact.RegularFileModeV1, Body: contextBody,
	})
	prepared, err := domainartifact.PrepareV1(sourceFiles)
	if err != nil {
		return snapshotMaterialV1{}, errors.Join(ErrInvalidInput, err)
	}
	keepScanner = true
	return snapshotMaterialV1{prepared: prepared, components: components, scanner: scanner}, nil
}

func (material snapshotMaterialV1) response(request RequestV1, status string) ResponseV1 {
	receipt := material.prepared.Receipt()
	return ResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: status,
		Operation: request.Operation, RequestNonce: request.RequestNonce, SessionName: request.SessionName,
		ManifestSHA256:          nativecomponentregistry.FrozenManifestSHA256V4,
		GenerationID:            receipt.GenerationID,
		InventorySHA256:         domainartifact.DigestBytesV1(material.prepared.InventoryBytes()),
		GenerationReceiptSHA256: domainartifact.DigestBytesV1(material.prepared.ReceiptBytes()),
		FileCount:               receipt.FileCount, TotalBytes: receipt.TotalBytes,
		Components: append([]ComponentReceiptV1(nil), material.components...),
	}
}

func DecodeRequestV1(raw []byte) (RequestV1, error) {
	var request RequestV1
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: requestLimitV1, MaxDepth: 2, MaxTokens: 24,
		MaxStringBytes: 128, MaxNumberBytes: 8, MaxAbsExponent: 1,
	}); err != nil {
		return RequestV1{}, ErrInvalidInput
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(raw, &shape) != nil || len(shape) != 8 {
		return RequestV1{}, ErrInvalidInput
	}
	for _, key := range []string{
		"kind", "schema_version", "operation", "request_nonce", "expected_generation_id",
		"expected_inventory_sha256", "expected_generation_receipt_sha256", "session_name",
	} {
		if _, ok := shape[key]; !ok {
			return RequestV1{}, ErrInvalidInput
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return RequestV1{}, ErrInvalidInput
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return RequestV1{}, ErrInvalidInput
	}
	if validateRequestV1(request, request.Operation) != nil {
		return RequestV1{}, ErrInvalidInput
	}
	return request, nil
}

func validateRequestV1(request RequestV1, operation string) error {
	if request.Kind != RequestKindV1 || request.SchemaVersion != SchemaVersionV1 ||
		request.Operation != operation || !validDigestV1(request.RequestNonce) || !validSessionNameV1(request.SessionName) {
		return ErrInvalidInput
	}
	switch operation {
	case OperationCreateV1:
		if request.ExpectedGenerationID != "" || request.ExpectedInventorySHA256 != "" ||
			request.ExpectedGenerationReceiptSHA256 != "" {
			return ErrInvalidInput
		}
	case OperationVerifyV1, OperationDiscardV1:
		if !validDigestV1(request.ExpectedGenerationID) || !validDigestV1(request.ExpectedInventorySHA256) ||
			!validDigestV1(request.ExpectedGenerationReceiptSHA256) {
			return ErrInvalidInput
		}
	case OperationReconcileDiscardV1:
		if request.ExpectedGenerationID != "" || request.ExpectedInventorySHA256 != "" ||
			request.ExpectedGenerationReceiptSHA256 != "" {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil

}

func validSessionNameV1(value string) bool {
	if !strings.HasPrefix(value, sessionNamePrefixV1) ||
		len(value) != len(sessionNamePrefixV1)+sessionNameSuffixBytesV1 {
		return false
	}
	for _, octet := range []byte(value[len(sessionNamePrefixV1):]) {
		if (octet < 'a' || octet > 'z') && (octet < 'A' || octet > 'Z') && (octet < '0' || octet > '9') {
			return false
		}
	}
	return true
}

func validDigestV1(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func newScannerV1(ctx context.Context, repository *os.File) (*scannerV1, error) {
	if ctx == nil || repository == nil || ctx.Err() != nil {
		return nil, ErrInvalidInput
	}
	fd, err := duplicateFileV1(repository)
	if err != nil {
		return nil, err
	}
	identity, err := validateDirectoryV1(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &scannerV1{
		ctx: ctx, repositoryFD: fd, repository: identity,
		byPath: make(map[string]*pinnedFileV1),
	}, nil
}

func (scanner *scannerV1) scanComponent(component nativecomponentregistry.FrozenComponentV4) error {
	if scanner == nil || scanner.ctx.Err() != nil || !validRelativeSourcePathV1(component.SourceRoot) ||
		component.CargoManifest != component.SourceRoot+"/Cargo.toml" {
		return ErrInvalidInput
	}
	root, err := openDirectoryPathV1(scanner.repositoryFD, component.SourceRoot)
	if err != nil {
		return err
	}
	return scanner.scanDirectory(component.SourceRoot, root, true, 1)
}

func (scanner *scannerV1) scanDirectory(path string, fd int, componentRoot bool, depth int) error {
	if scanner.ctx.Err() != nil || depth > maxSourceDepthV1 || len(scanner.directories) >= maxSourceDirectoriesV1 {
		_ = unix.Close(fd)
		return errors.Join(ErrInvalidInput, scanner.ctx.Err())
	}
	identity, err := validateDirectoryV1(fd)
	if err != nil || identity.dev != scanner.repository.dev {
		_ = unix.Close(fd)
		return errors.Join(ErrInvalidInput, err)
	}
	entries, err := readDirectoryNamesV1(fd)
	if err != nil {
		_ = unix.Close(fd)
		return err
	}
	directory := &pinnedDirectoryV1{
		path: path, fd: fd, identity: identity,
		entries: append(make([]string, 0, len(entries)), entries...), entryState: make(map[string]sourceIdentityV1, len(entries)),
	}
	scanner.directories = append(scanner.directories, directory)
	for _, name := range entries {
		if scanner.ctx.Err() != nil || !validEntryNameV1(name) {
			return errors.Join(ErrInvalidInput, scanner.ctx.Err())
		}
		stat, err := statAtV1(fd, name)
		if err != nil {
			return err
		}
		entryIdentity := identityV1(stat)
		directory.entryState[name] = entryIdentity
		entryPath := path + "/" + name
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			child, err := openDirectoryAtV1(fd, name)
			if err != nil {
				return err
			}
			if componentRoot && (name == "target" || name == ".git") {
				childIdentity, err := validateExcludedDirectoryV1(child)
				if err != nil || childIdentity != entryIdentity {
					_ = unix.Close(child)
					return errors.Join(ErrInvalidInput, err)
				}
				scanner.directories = append(scanner.directories, &pinnedDirectoryV1{
					path: entryPath, fd: child, identity: childIdentity,
				})
				continue
			}
			childIdentity, err := validateDirectoryV1(child)
			if err != nil || childIdentity != entryIdentity {
				_ = unix.Close(child)
				return errors.Join(ErrInvalidInput, err)
			}
			if strings.HasPrefix(name, ".") {
				_ = unix.Close(child)
				return ErrInvalidInput
			}
			if err := scanner.scanDirectory(entryPath, child, false, depth+1); err != nil {
				return err
			}
		case unix.S_IFREG:
			if strings.HasPrefix(name, ".") {
				return ErrInvalidInput
			}
			if err := scanner.scanFile(entryPath, fd, name, entryIdentity); err != nil {
				return err
			}
		default:
			return ErrInvalidInput
		}
	}
	return nil
}

func (scanner *scannerV1) omittedEmptyDirectories() []string {
	paths := make([]string, 0)
	for _, directory := range scanner.directories {
		if directory.entries != nil && len(directory.entries) == 0 {
			paths = append(paths, directory.path)
		}
	}
	sort.Strings(paths)
	return paths
}

func (scanner *scannerV1) scanFile(path string, parent int, name string, expected sourceIdentityV1) error {
	if len(scanner.files) >= maxSourceFilesV1-2 || scanner.ctx.Err() != nil {
		return errors.Join(ErrInvalidInput, scanner.ctx.Err())
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	identity, err := validateFileV1(fd)
	if err != nil || identity != expected || identity.size > maxSourceFileBytesV1 ||
		identity.size > maxSourceTotalBytesV1-scanner.totalBytes || identity.dev != scanner.repository.dev {
		_ = unix.Close(fd)
		return errors.Join(ErrInvalidInput, err)
	}
	body, digest, err := readFileV1(scanner.ctx, fd, identity)
	if err != nil {
		_ = unix.Close(fd)
		return err
	}
	if _, exists := scanner.byPath[path]; exists {
		_ = unix.Close(fd)
		return ErrInvalidInput
	}
	file := &pinnedFileV1{path: path, fd: fd, identity: identity, body: body, sha256: digest}
	scanner.files = append(scanner.files, file)
	scanner.byPath[path] = file
	scanner.totalBytes += identity.size
	return nil
}

func (scanner *scannerV1) scanOptionalRepositoryFile(path string) error {
	present, err := scanner.repositoryEntryPresent(path)
	if err != nil || !present {
		return err
	}
	parentPath, name := splitRelativePathV1(path)
	parent := scanner.repositoryFD
	owned := false
	if parentPath != "" {
		parent, err = openDirectoryPathV1(scanner.repositoryFD, parentPath)
		if err != nil {
			return err
		}
		owned = true
	}
	if owned {
		defer unix.Close(parent)
	}
	stat, err := statAtV1(parent, name)
	if err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.Join(ErrInvalidInput, err)
	}
	return scanner.scanFile(path, parent, name, identityV1(stat))
}

func (scanner *scannerV1) scanRequiredRepositoryFile(path string, expected sourceIdentityV1) error {
	parentPath, name := splitRelativePathV1(path)
	if name == "" || (parentPath != "" && !validRelativeSourcePathV1(parentPath)) {
		return ErrInvalidInput
	}
	parent := scanner.repositoryFD
	owned := false
	var err error
	if parentPath != "" {
		parent, err = openDirectoryPathV1(scanner.repositoryFD, parentPath)
		if err != nil {
			return err
		}
		owned = true
	}
	if owned {
		defer unix.Close(parent)
	}
	stat, err := statAtV1(parent, name)
	if err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || identityV1(stat) != expected {
		return errors.Join(ErrInvalidInput, err)
	}
	return scanner.scanFile(path, parent, name, expected)
}

func (scanner *scannerV1) repositoryEntryPresent(path string) (bool, error) {
	parentPath, name := splitRelativePathV1(path)
	if name == "" {
		return false, ErrInvalidInput
	}
	parent := scanner.repositoryFD
	owned := false
	var err error
	if parentPath != "" {
		parent, err = openDirectoryPathV1(scanner.repositoryFD, parentPath)
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		owned = true
	}
	if owned {
		defer unix.Close(parent)
	}
	_, err = statAtV1(parent, name)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	return err == nil, err
}

func (scanner *scannerV1) componentReceipt(component nativecomponentregistry.FrozenComponentV4) (ComponentReceiptV1, error) {
	prefix := component.SourceRoot + "/"
	files := make([]*pinnedFileV1, 0)
	for _, file := range scanner.files {
		if strings.HasPrefix(file.path, prefix) {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return ComponentReceiptV1{}, ErrInvalidInput
	}
	sort.Slice(files, func(left, right int) bool {
		return files[left].path < files[right].path
	})
	hash := sha256.New()
	var total int64
	for _, file := range files {
		relative := strings.TrimPrefix(file.path, prefix)
		_, _ = hash.Write([]byte(relative))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(file.identity.size, 10)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.sha256))
		_, _ = hash.Write([]byte{0})
		total += file.identity.size
	}
	lock := scanner.byPath[component.SourceRoot+"/Cargo.lock"]
	manifest := scanner.byPath[component.CargoManifest]
	if lock == nil || manifest == nil {
		return ComponentReceiptV1{}, ErrInvalidInput
	}
	return ComponentReceiptV1{
		ID: component.ID, SourceRoot: component.SourceRoot,
		SourceDigest: hex.EncodeToString(hash.Sum(nil)), CargoLockSHA256: lock.sha256,
		FileCount: len(files), TotalBytes: total,
	}, nil
}

func (scanner *scannerV1) revalidate() error {
	if scanner == nil || scanner.ctx.Err() != nil {
		return errors.Join(ErrSourceChanged, scanner.ctx.Err())
	}
	current, err := validateDirectoryV1(scanner.repositoryFD)
	if err != nil || current != scanner.repository {
		return errors.Join(ErrSourceChanged, err)
	}
	for _, directory := range scanner.directories {
		validateDirectory := validateDirectoryV1
		if directory.entries == nil {
			validateDirectory = validateExcludedDirectoryV1
		}
		current, err := validateDirectory(directory.fd)
		if err != nil || current != directory.identity {
			return errors.Join(ErrSourceChanged, err)
		}
		byPath, err := openDirectoryPathV1(scanner.repositoryFD, directory.path)
		if err != nil {
			return errors.Join(ErrSourceChanged, err)
		}
		pathIdentity, pathErr := validateDirectory(byPath)
		closeErr := unix.Close(byPath)
		if pathErr != nil || closeErr != nil || pathIdentity != directory.identity {
			return errors.Join(ErrSourceChanged, pathErr, closeErr)
		}
		if directory.entries == nil {
			continue
		}
		entries, err := readDirectoryNamesV1(directory.fd)
		if err != nil || !equalStringsV1(entries, directory.entries) {
			return errors.Join(ErrSourceChanged, err)
		}
		for _, name := range entries {
			stat, err := statAtV1(directory.fd, name)
			if err != nil || identityV1(stat) != directory.entryState[name] {
				return errors.Join(ErrSourceChanged, err)
			}
		}
	}
	for _, file := range scanner.files {
		current, err := validateFileV1(file.fd)
		if err != nil || current != file.identity {
			return errors.Join(ErrSourceChanged, err)
		}
		_, digest, err := readFileV1(scanner.ctx, file.fd, file.identity)
		if err != nil || digest != file.sha256 {
			return errors.Join(ErrSourceChanged, err)
		}
		byPath, err := openFilePathV1(scanner.repositoryFD, file.path)
		if err != nil {
			return errors.Join(ErrSourceChanged, err)
		}
		pathIdentity, pathErr := validateFileV1(byPath)
		closeErr := unix.Close(byPath)
		if pathErr != nil || closeErr != nil || pathIdentity != file.identity {
			return errors.Join(ErrSourceChanged, pathErr, closeErr)
		}
	}
	return nil
}

func (scanner *scannerV1) close() error {
	if scanner == nil {
		return nil
	}
	var result error
	for _, file := range scanner.files {
		result = errors.Join(result, unix.Close(file.fd))
	}
	for index := len(scanner.directories) - 1; index >= 0; index-- {
		result = errors.Join(result, unix.Close(scanner.directories[index].fd))
	}
	if scanner.repositoryFD >= 0 {
		result = errors.Join(result, unix.Close(scanner.repositoryFD))
		scanner.repositoryFD = -1
	}
	return result
}

func readPinnedManifestV1(ctx context.Context, file *os.File) ([]byte, sourceIdentityV1, error) {
	if ctx == nil || file == nil || ctx.Err() != nil {
		return nil, sourceIdentityV1{}, ErrInvalidInput
	}
	fd, err := duplicateFileV1(file)
	if err != nil {
		return nil, sourceIdentityV1{}, err
	}
	defer unix.Close(fd)
	identity, err := validateFileV1(fd)
	if err != nil || identity.size <= 0 || identity.size > nativecomponentregistry.MaxManifestBytesV4 {
		return nil, sourceIdentityV1{}, errors.Join(ErrInvalidInput, err)
	}
	body, _, err := readFileV1(ctx, fd, identity)
	return body, identity, err
}

func validatePrivateParentEntriesV1(parent *os.File, expected []string) error {
	fd, err := duplicateFileV1(parent)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(os.Geteuid()) || uint32(stat.Mode&0o7777) != domainartifact.DirectoryModeV1 ||
		!sourceExtendedSecuritySafeV1(fd) {
		return ErrInvalidInput
	}
	entries, err := readDirectoryNamesV1(fd)
	if err != nil || !equalStringsV1(entries, expected) {
		return errors.Join(ErrInvalidInput, err)
	}
	return nil
}

func validateSessionParentBindingV1(container, parent *os.File, sessionName string) error {
	if container == nil || parent == nil || !validSessionNameV1(sessionName) {
		return ErrInvalidInput
	}
	containerFD, err := duplicateFileV1(container)
	if err != nil {
		return err
	}
	defer unix.Close(containerFD)
	parentFD, err := duplicateFileV1(parent)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	containerIdentity, err := validateSessionContainerDirectoryV1(containerFD)
	if err != nil {
		return err
	}
	parentIdentity, err := validatePrivateSessionDirectoryV1(parentFD)
	if err != nil || parentIdentity.dev != containerIdentity.dev {
		return errors.Join(ErrInvalidInput, err)
	}
	bound, err := openDirectoryAtV1(containerFD, sessionName)
	if err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	defer unix.Close(bound)
	boundIdentity, err := validatePrivateSessionDirectoryV1(bound)
	if err != nil || boundIdentity != parentIdentity {
		return errors.Join(ErrInvalidInput, err)
	}
	return nil
}

// validateReconcileSessionParentV1 accepts exactly two cleanup states: the
// original parent is still bound at sessionName, or the same open parent has
// already been unlinked and is empty while sessionName is absent. A replaced
// name, linked alias, or residual entry is never removed.
func validateReconcileSessionParentV1(container, parent *os.File, sessionName string) (bool, error) {
	if err := validateSessionParentBindingV1(container, parent, sessionName); err == nil {
		return false, nil
	}
	if container == nil || parent == nil || !validSessionNameV1(sessionName) {
		return false, ErrInvalidInput
	}
	containerFD, err := duplicateFileV1(container)
	if err != nil {
		return false, err
	}
	defer unix.Close(containerFD)
	containerIdentity, err := validateSessionContainerDirectoryV1(containerFD)
	if err != nil {
		return false, err
	}
	parentFD, err := duplicateFileV1(parent)
	if err != nil {
		return false, err
	}
	defer unix.Close(parentFD)
	parentIdentity, err := validatePrivateSessionDirectoryV1(parentFD)
	if err != nil || parentIdentity.dev != containerIdentity.dev {
		return false, errors.Join(ErrInvalidInput, err)
	}
	if _, err := statAtV1(containerFD, sessionName); !errors.Is(err, unix.ENOENT) {
		return false, errors.Join(ErrInvalidInput, err)
	}
	if err := validateDetachedDirectoryPathV1(parentFD); err != nil {
		return false, err
	}
	if err := unix.Flock(parentFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return false, errors.Join(ErrDestruction, err)
	}
	defer unix.Flock(parentFD, unix.LOCK_UN)
	for range 2 {
		entries, readErr := readDirectoryNamesV1(parentFD)
		if readErr != nil || len(entries) != 0 {
			return false, errors.Join(ErrDestruction, readErr)
		}
		var current unix.Stat_t
		if statErr := unix.Fstat(parentFD, &current); statErr != nil || identityV1(current) != parentIdentity {
			return false, errors.Join(ErrDestruction, statErr)
		}
		if err := validateDetachedDirectoryPathV1(parentFD); err != nil {
			return false, err
		}
		if _, statErr := statAtV1(containerFD, sessionName); !errors.Is(statErr, unix.ENOENT) {
			return false, errors.Join(ErrDestruction, statErr)
		}
	}
	return true, nil
}

func validateDetachedDirectoryPathV1(fd int) error {
	if fd < 0 {
		return ErrInvalidInput
	}
	var path [unix.PathMax]byte
	_, _, errno := unix.Syscall(
		unix.SYS_FCNTL,
		uintptr(fd),
		uintptr(unix.F_GETPATH),
		uintptr(unsafe.Pointer(&path[0])),
	)
	if errno != 0 {
		if errors.Is(errno, unix.ENOENT) {
			return nil
		}
		return errors.Join(ErrDestruction, errno)
	}
	end := bytes.IndexByte(path[:], 0)
	if end <= 0 || path[0] != '/' {
		return ErrDestruction
	}
	if _, err := os.Lstat(string(path[:end])); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(ErrDestruction, err)
	}
	return nil
}

func destroySessionParentV1(container, parent *os.File, sessionName string) error {
	if err := validateSessionParentBindingV1(container, parent, sessionName); err != nil {
		return err
	}
	containerFD, err := duplicateFileV1(container)
	if err != nil {
		return err
	}
	defer unix.Close(containerFD)
	if _, err := validateSessionContainerDirectoryV1(containerFD); err != nil {
		return err
	}
	parentFD, err := duplicateFileV1(parent)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	parentIdentity, err := validatePrivateSessionDirectoryV1(parentFD)
	if err != nil {
		return err
	}
	if err := unix.Flock(parentFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.Join(ErrPublication, err)
	}
	defer unix.Flock(parentFD, unix.LOCK_UN)
	for range 2 {
		entries, readErr := readDirectoryNamesV1(parentFD)
		if readErr != nil || len(entries) != 0 {
			return errors.Join(ErrPublication, readErr)
		}
		current, statErr := statAtV1(containerFD, sessionName)
		if statErr != nil || identityV1(current) != parentIdentity || current.Mode&unix.S_IFMT != unix.S_IFDIR {
			return errors.Join(ErrPublication, statErr)
		}
	}
	if err := unix.Fsync(parentFD); err != nil {
		return errors.Join(ErrPublication, err)
	}
	if err := unix.Unlinkat(containerFD, sessionName, unix.AT_REMOVEDIR); err != nil {
		return errors.Join(ErrPublication, err)
	}
	if _, err := statAtV1(containerFD, sessionName); !errors.Is(err, unix.ENOENT) {
		return errors.Join(ErrPublication, err)
	}
	if err := unix.Fsync(containerFD); err != nil {
		return errors.Join(ErrPublication, err)
	}
	return nil
}

func validatePrivateSessionDirectoryV1(fd int) (sourceIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		stat.Uid != uint32(os.Geteuid()) || uint32(stat.Mode&0o7777) != domainartifact.DirectoryModeV1 ||
		!sourceExtendedSecuritySafeV1(fd) {
		return sourceIdentityV1{}, ErrInvalidInput
	}
	return identityV1(stat), nil
}

func validateSessionContainerDirectoryV1(fd int) (sourceIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		!sourceExtendedSecuritySafeV1(fd) {
		return sourceIdentityV1{}, ErrInvalidInput
	}
	mode := uint32(stat.Mode & 0o7777)
	if (stat.Uid != 0 || mode != 0o1777) &&
		(stat.Uid != uint32(os.Geteuid()) || mode != domainartifact.DirectoryModeV1) {
		return sourceIdentityV1{}, ErrInvalidInput
	}
	return identityV1(stat), nil
}

func duplicateFileV1(file *os.File) (int, error) {
	if file == nil {
		return -1, ErrInvalidInput
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return -1, err
	}
	duplicate := -1
	var duplicateErr error
	controlErr := raw.Control(func(fd uintptr) {
		duplicate, duplicateErr = unix.FcntlInt(fd, unix.F_DUPFD_CLOEXEC, 8)
	})
	if controlErr != nil || duplicateErr != nil || duplicate < 8 {
		if duplicate >= 0 {
			_ = unix.Close(duplicate)
		}
		return -1, errors.Join(ErrInvalidInput, controlErr, duplicateErr)
	}
	return duplicate, nil
}

func validateDirectoryV1(fd int) (sourceIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		(stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) || stat.Mode&0o022 != 0 ||
		!sourceExtendedSecuritySafeV1(fd) {
		return sourceIdentityV1{}, ErrInvalidInput
	}
	return identityV1(stat), nil
}

// Excluded component-root target/.git contents are never copied or hashed.
// Their exact directory object is still identity-pinned, but irrelevant
// metadata xattrs (for example macOS backup exclusion) are not source input.
// The validated parent source directory remains the authority for replacing
// this entry, so accepting those ignored-object xattrs does not broaden access
// to any copied source byte.
func validateExcludedDirectoryV1(fd int) (sourceIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		(stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) || stat.Mode&0o022 != 0 || stat.Flags != 0 {
		return sourceIdentityV1{}, ErrInvalidInput
	}
	return identityV1(stat), nil
}

func validateFileV1(fd int) (sourceIdentityV1, error) {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 ||
		(stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) || stat.Mode&0o022 != 0 ||
		stat.Mode&0o111 != 0 ||
		stat.Size < 0 || stat.Size > maxSourceFileBytesV1 || !sourceExtendedSecuritySafeV1(fd) {
		return sourceIdentityV1{}, ErrInvalidInput
	}
	return identityV1(stat), nil
}

func identityV1(stat unix.Stat_t) sourceIdentityV1 {
	return sourceIdentityV1{
		dev: uint64(stat.Dev), ino: stat.Ino, mode: uint32(stat.Mode), uid: stat.Uid, gid: stat.Gid,
		nlink: uint64(stat.Nlink), size: stat.Size,
		mtimeSec: stat.Mtim.Sec, mtimeNS: stat.Mtim.Nsec,
		ctimeSec: stat.Ctim.Sec, ctimeNS: stat.Ctim.Nsec,
	}
}

func readFileV1(ctx context.Context, fd int, expected sourceIdentityV1) ([]byte, string, error) {
	if ctx == nil || ctx.Err() != nil || expected.size < 0 || expected.size > maxSourceFileBytesV1 {
		return nil, "", ErrInvalidInput
	}
	body := make([]byte, int(expected.size))
	for offset := int64(0); offset < expected.size; {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		end := offset + 1<<20
		if end > expected.size {
			end = expected.size
		}
		read, err := unix.Pread(fd, body[offset:end], offset)
		if err != nil || int64(read) != end-offset {
			return nil, "", errors.Join(ErrInvalidInput, err)
		}
		offset = end
	}
	var extra [1]byte
	if read, err := unix.Pread(fd, extra[:], expected.size); read != 0 || err != nil {
		return nil, "", errors.Join(ErrInvalidInput, err)
	}
	current, err := validateFileV1(fd)
	if err != nil || current != expected {
		return nil, "", errors.Join(ErrSourceChanged, err)
	}
	digest := sha256.Sum256(body)
	return body, hex.EncodeToString(digest[:]), nil
}

func readDirectoryNamesV1(fd int) ([]string, error) {
	independent, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(independent), "native-source-directory")
	if file == nil {
		_ = unix.Close(independent)
		return nil, ErrInvalidInput
	}
	defer file.Close()
	entries, err := file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func openDirectoryPathV1(root int, path string) (int, error) {
	if !validRelativeSourcePathV1(path) {
		return -1, ErrInvalidInput
	}
	current, err := unix.FcntlInt(uintptr(root), unix.F_DUPFD_CLOEXEC, 8)
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(path, "/") {
		next, openErr := openDirectoryAtV1(current, component)
		closeErr := unix.Close(current)
		if openErr != nil || closeErr != nil {
			if next >= 0 {
				_ = unix.Close(next)
			}
			return -1, errors.Join(openErr, closeErr)
		}
		current = next
	}
	return current, nil
}

func openFilePathV1(root int, path string) (int, error) {
	parentPath, name := splitRelativePathV1(path)
	if name == "" || strings.HasPrefix(name, ".") || !validEntryNameV1(name) ||
		(parentPath != "" && !validRelativeSourcePathV1(parentPath)) {
		return -1, ErrInvalidInput
	}
	parent := root
	owned := false
	var err error
	if parentPath != "" {
		parent, err = openDirectoryPathV1(root, parentPath)
		if err != nil {
			return -1, err
		}
		owned = true
	}
	fd, openErr := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if owned {
		openErr = errors.Join(openErr, unix.Close(parent))
	}
	if openErr != nil {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
		return -1, openErr
	}
	return fd, nil
}

func openDirectoryAtV1(parent int, name string) (int, error) {
	return unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}

func statAtV1(parent int, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}

func validRelativeSourcePathV1(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.ContainsRune(path, 0) {
		return false
	}
	for _, component := range strings.Split(path, "/") {
		if !validEntryNameV1(component) || strings.HasPrefix(component, ".") {
			return false
		}
	}
	return true
}

func validEntryNameV1(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= maxSourceComponentBytesV1 &&
		!strings.Contains(name, "/") && !strings.Contains(name, "\\") && !strings.ContainsRune(name, 0)
}

func splitRelativePathV1(path string) (string, string) {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return "", path
	}
	return path[:index], path[index+1:]
}

func equalStringsV1(left, right []string) bool {
	return len(left) == len(right) && bytes.Equal([]byte(strings.Join(left, "\x00")), []byte(strings.Join(right, "\x00")))
}
