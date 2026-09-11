//go:build analytix_native_build_probe && !analytix_prod

package nativebuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	CargoExecutionPurposeV1         = "analytix-cargo-execution"
	CargoExecutionAuthorityProtocol = "analytix-cargo-execution-authority-v1"
	CargoExecutionSchemaVersionV1   = 1

	CargoStatusSucceeded CargoExecutionStatusV1 = "succeeded"
	CargoStatusBlocked   CargoExecutionStatusV1 = "blocked"
	CargoStatusFailed    CargoExecutionStatusV1 = "failed"
	CargoStatusCancelled CargoExecutionStatusV1 = "cancelled"
	CargoStatusTimedOut  CargoExecutionStatusV1 = "timed_out"

	CargoTrustLocalProvisional  CargoExecutionTrustClassV1 = "local_provisional"
	CargoTrustControlledRelease CargoExecutionTrustClassV1 = "controlled_release"

	ToolchainOwnershipUserWritable     ToolchainOwnershipClassV1 = "user_writable"
	ToolchainOwnershipUnbound          ToolchainOwnershipClassV1 = "unbound"
	ToolchainOwnershipImmutableRelease ToolchainOwnershipClassV1 = "immutable_release"
)

var (
	ErrCargoExecutionInvalid    = errors.New("cargo_execution_receipt_invalid")
	ErrCargoExecutionIneligible = errors.New("cargo_execution_ineligible")
)

type CargoExecutionStatusV1 string
type CargoExecutionTrustClassV1 string
type ToolchainOwnershipClassV1 string

type SourceSnapshotBindingV1 struct {
	GenerationID            string `json:"generationId"`
	InventorySHA256         string `json:"inventorySha256"`
	GenerationReceiptSHA256 string `json:"generationReceiptSha256"`
}

type CargoToolchainIdentityV1 struct {
	CargoExecutableSHA256 string                    `json:"cargoExecutableSha256"`
	CargoVersion          string                    `json:"cargoVersion"`
	RustcExecutableSHA256 string                    `json:"rustcExecutableSha256"`
	RustcVersion          string                    `json:"rustcVersion"`
	TreeSHA256            string                    `json:"treeSha256"`
	TreeFileCount         int                       `json:"treeFileCount"`
	CompilerSHA256        string                    `json:"compilerSha256"`
	ArchiverSHA256        string                    `json:"archiverSha256"`
	LinkerSHA256          string                    `json:"linkerSha256"`
	PlatformSignerSHA256  string                    `json:"platformSignerSha256"`
	SDKIdentitySHA256     string                    `json:"sdkIdentitySha256"`
	OwnershipClass        ToolchainOwnershipClassV1 `json:"ownershipClass"`
}

type CargoContainmentV1 struct {
	SourceDescriptorsHeld    bool `json:"sourceDescriptorsHeld"`
	ToolchainDescriptorsHeld bool `json:"toolchainDescriptorsHeld"`
	LoadedImagesBound        bool `json:"loadedImagesBound"`
	WorkingDirectoriesBound  bool `json:"workingDirectoriesBound"`
	TargetWriteScopeBound    bool `json:"targetWriteScopeBound"`
	NetworkDenied            bool `json:"networkDenied"`
	DeadlineBound            bool `json:"deadlineBound"`
	ProcessTreeEmpty         bool `json:"processTreeEmpty"`
	OutputsDescriptorBound   bool `json:"outputsDescriptorBound"`
	RawBuildDescriptorsHeld  bool `json:"rawBuildDescriptorsHeld"`
	PayloadIdentityBound     bool `json:"payloadIdentityBound"`
	TemporaryStateDestroyed  bool `json:"temporaryStateDestroyed"`
	SameUIDMutationDenied    bool `json:"sameUidMutationDenied"`
	ProcessContainmentBound  bool `json:"processContainmentBound"`
	DependencyClosureBound   bool `json:"dependencyClosureBound"`
	SDKClosureBound          bool `json:"sdkClosureBound"`
}

// CargoOutputV1 describes the descriptor-observed Cargo output before final
// package signing. RawBuildSHA256 is the complete build image. PayloadSHA256
// is the target parser's signing-invariant executable payload identity. The
// eventual signed package image and OS signature are separate post-sign
// evidence and must never be inferred from either value.
type CargoOutputV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	ID                     string `json:"id"`
	SourceDigest           string `json:"sourceDigest"`
	CargoLockSHA256        string `json:"cargoLockSha256"`
	BuildEnvironmentSHA256 string `json:"buildEnvironmentSha256"`
	RawBuildSHA256         string `json:"rawBuildSha256"`
	RawBuildSize           int64  `json:"rawBuildSize"`
	PayloadSHA256          string `json:"payloadSha256"`
	PayloadSize            int64  `json:"payloadSize"`
	Format                 string `json:"format"`
	Arch                   string `json:"arch"`
}

type CargoExecutionAuthorityV1 struct {
	Protocol               string `json:"protocol"`
	TargetKey              string `json:"targetKey"`
	BinarySHA256           string `json:"binarySha256"`
	SourceSetSHA256        string `json:"sourceSetSha256"`
	BuildEnvironmentSHA256 string `json:"buildEnvironmentSha256"`
	GoExecutableSHA256     string `json:"goExecutableSha256"`
}

// CargoExecutionReceiptV1 is audit evidence. ReleaseEligible is derived by
// ValidateCargoExecutionReceiptV1; it is never trusted as a caller assertion.
type CargoExecutionReceiptV1 struct {
	SchemaVersion          int                        `json:"schemaVersion"`
	Purpose                string                     `json:"purpose"`
	ExecutionID            string                     `json:"executionId"`
	RequestNonce           string                     `json:"requestNonce"`
	Status                 CargoExecutionStatusV1     `json:"status"`
	TrustClass             CargoExecutionTrustClassV1 `json:"trustClass"`
	ReleaseEligible        bool                       `json:"releaseEligible"`
	Blocker                string                     `json:"blocker"`
	TargetKey              string                     `json:"targetKey"`
	TargetTriple           string                     `json:"targetTriple"`
	ManifestSHA256         string                     `json:"manifestSha256"`
	SourceSetSHA256        string                     `json:"sourceSetSha256"`
	SourceSnapshot         SourceSnapshotBindingV1    `json:"sourceSnapshot"`
	BuildPlanSHA256        string                     `json:"buildPlanSha256"`
	BuildEnvironmentSHA256 string                     `json:"buildEnvironmentSha256"`
	Toolchain              CargoToolchainIdentityV1   `json:"toolchain"`
	Containment            CargoContainmentV1         `json:"containment"`
	Outputs                []CargoOutputV1            `json:"outputs"`
	OutputInventorySHA256  string                     `json:"outputInventorySha256"`
	Authority              CargoExecutionAuthorityV1  `json:"authority"`
}

// PublicationBindingV1 is carried only inside the Go coordinator. It is not a
// JSON request field and cannot be supplied by Node, MCP, or another process.
type PublicationBindingV1 struct {
	ExecutionID            string
	ExecutionReceiptSHA256 string
	TargetKey              string
	TargetTriple           string
	RequestNonce           string
	ManifestSHA256         string
	SourceSetSHA256        string
	SourceSnapshot         SourceSnapshotBindingV1
	BuildPlanSHA256        string
	BuildEnvironmentSHA256 string
	Toolchain              CargoToolchainIdentityV1
	Outputs                []CargoOutputV1
	OutputInventorySHA256  string
	Authority              CargoExecutionAuthorityV1
}

type publicationPermitStateV1 struct {
	consumed atomic.Bool
}

// CargoExecutionObserverV1 is process-local authority to settle exactly one
// execution observation. Its zero value and JSON-decoded values are invalid;
// copies share the same single-use state. Audit receipts never contain this
// authority and therefore cannot restore it after parsing or restart.
type CargoExecutionObserverV1 struct {
	state *cargoExecutionObserverStateV1
}

type cargoExecutionObserverStateV1 struct {
	finalized atomic.Bool
}

// LiveCargoExecutionV1 proves that the current process settled an eligible
// observation. It is consumed when a publication permit is minted. A parsed
// CargoExecutionReceiptV1 is audit evidence only and cannot recreate this
// capability.
type LiveCargoExecutionV1 struct {
	state            *liveCargoExecutionStateV1
	executionID      string
	receiptCanonical []byte
	seal             string
}

type liveCargoExecutionStateV1 struct {
	authorized atomic.Bool
}

// PublicationPermitV1 has no exported fields. Its zero value and JSON-decoded
// values are invalid, and copies share one single-use consumption state.
type PublicationPermitV1 struct {
	state            *publicationPermitStateV1
	executionID      string
	bindingCanonical []byte
	outputsCanonical []byte
	seal             string
}

var frozenOutputIDsV1 = [...]string{
	"import-accelerator",
	"cleaning-ops",
	"analysis-compute",
	"data-engine",
}

var cargoBlockersV1 = map[string]struct{}{
	"toolchain_user_writable":         {},
	"toolchain_identity_unbound":      {},
	"source_snapshot_unbound":         {},
	"cargo_execution_failed":          {},
	"cargo_execution_timeout":         {},
	"cargo_execution_cancelled":       {},
	"cargo_process_tree_unconfirmed":  {},
	"cargo_write_scope_unconfirmed":   {},
	"cargo_network_unconfirmed":       {},
	"output_inventory_mismatch":       {},
	"build_cleanup_indeterminate":     {},
	"authority_identity_unbound":      {},
	"controlled_environment_unproven": {},
}

func FinalizeCargoExecutionReceiptV1(receipt CargoExecutionReceiptV1) (CargoExecutionReceiptV1, error) {
	if receipt.ExecutionID != "" {
		return CargoExecutionReceiptV1{}, ErrCargoExecutionInvalid
	}
	receipt.Outputs = cloneOutputsV1(receipt.Outputs)
	receipt.ReleaseEligible = cargoExecutionEligibleV1(receipt)
	receipt.ExecutionID = cargoExecutionIDV1(receipt)
	if err := ValidateCargoExecutionReceiptV1(receipt); err != nil {
		return CargoExecutionReceiptV1{}, err
	}
	return receipt, nil
}

// BeginCargoExecutionObservationV1 must be called by the build coordinator
// before it asks the host adapter to perform any execution. Architecture tests
// keep this constructor and FinalizeObservedCargoExecutionV1 on that single
// production call path.
func BeginCargoExecutionObservationV1() CargoExecutionObserverV1 {
	return CargoExecutionObserverV1{state: &cargoExecutionObserverStateV1{}}
}

// FinalizeObservedCargoExecutionV1 settles one live observation and returns an
// audit receipt. Ineligible observations still produce their validated audit
// value but never produce a live publication capability.
func FinalizeObservedCargoExecutionV1(
	observer CargoExecutionObserverV1,
	receipt CargoExecutionReceiptV1,
) (CargoExecutionReceiptV1, LiveCargoExecutionV1, error) {
	// Claim the observation before validating the terminal value. A malformed
	// or adversarial first settlement attempt must burn the live authority; it
	// cannot be repaired and retried into an eligible publication capability.
	if observer.state == nil || !observer.state.finalized.CompareAndSwap(false, true) {
		return CargoExecutionReceiptV1{}, LiveCargoExecutionV1{}, ErrCargoExecutionInvalid
	}
	finalized, err := FinalizeCargoExecutionReceiptV1(receipt)
	if err != nil {
		return CargoExecutionReceiptV1{}, LiveCargoExecutionV1{}, err
	}
	canonical, err := CargoExecutionReceiptV1Bytes(finalized)
	if err != nil {
		return CargoExecutionReceiptV1{}, LiveCargoExecutionV1{}, err
	}
	if !finalized.ReleaseEligible {
		return finalized, LiveCargoExecutionV1{}, nil
	}
	live := LiveCargoExecutionV1{
		state:            &liveCargoExecutionStateV1{},
		executionID:      finalized.ExecutionID,
		receiptCanonical: append([]byte(nil), canonical...),
		seal:             liveCargoExecutionSealV1(finalized.ExecutionID, digestBytesV1(canonical)),
	}
	return finalized, live, nil
}

func ValidateCargoExecutionReceiptV1(receipt CargoExecutionReceiptV1) error {
	if receipt.SchemaVersion != CargoExecutionSchemaVersionV1 || receipt.Purpose != CargoExecutionPurposeV1 ||
		!validDigestV1(receipt.ExecutionID) || receipt.ExecutionID != cargoExecutionIDV1(receipt) || !validDigestV1(receipt.RequestNonce) ||
		!validTargetV1(receipt.TargetKey, receipt.TargetTriple) || !validDigestV1(receipt.ManifestSHA256) ||
		!validDigestV1(receipt.SourceSetSHA256) || !validSourceSnapshotV1(receipt.SourceSnapshot) ||
		!validDigestV1(receipt.BuildPlanSHA256) || !validDigestV1(receipt.BuildEnvironmentSHA256) ||
		!validToolchainV1(receipt.Toolchain) || !validOutputsForTargetV1(receipt.Outputs, receipt.TargetKey) ||
		receipt.OutputInventorySHA256 != CargoOutputInventorySHA256V1(receipt.Outputs) ||
		!validAuthorityV1(receipt.Authority, receipt.TargetKey) {
		return ErrCargoExecutionInvalid
	}
	if !validStatusV1(receipt.Status) || !validTrustClassV1(receipt.TrustClass) {
		return ErrCargoExecutionInvalid
	}
	eligible := cargoExecutionEligibleV1(receipt)
	if receipt.ReleaseEligible != eligible {
		return ErrCargoExecutionInvalid
	}
	if eligible {
		if receipt.Blocker != "" {
			return ErrCargoExecutionInvalid
		}
		return nil
	}
	if _, ok := cargoBlockersV1[receipt.Blocker]; !ok {
		return ErrCargoExecutionInvalid
	}
	return nil
}

func CargoExecutionReceiptV1Bytes(receipt CargoExecutionReceiptV1) ([]byte, error) {
	if err := ValidateCargoExecutionReceiptV1(receipt); err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return nil, ErrCargoExecutionInvalid
	}
	return append(body, '\n'), nil
}

func ParseCargoExecutionReceiptV1(body []byte) (CargoExecutionReceiptV1, error) {
	var receipt CargoExecutionReceiptV1
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       64 * 1024,
		MaxDepth:       8,
		MaxTokens:      512,
		MaxStringBytes: 1024,
		MaxNumberBytes: 32,
		MaxAbsExponent: 1,
	}); err != nil {
		return CargoExecutionReceiptV1{}, ErrCargoExecutionInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil {
		return CargoExecutionReceiptV1{}, ErrCargoExecutionInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CargoExecutionReceiptV1{}, ErrCargoExecutionInvalid
	}
	canonical, err := CargoExecutionReceiptV1Bytes(receipt)
	if err != nil || !bytes.Equal(body, canonical) {
		return CargoExecutionReceiptV1{}, ErrCargoExecutionInvalid
	}
	return receipt, nil
}

func CargoOutputInventorySHA256V1(outputs []CargoOutputV1) string {
	body, err := json.Marshal(outputs)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func AuthorizeForPublicationV1(
	live LiveCargoExecutionV1,
	receipt CargoExecutionReceiptV1,
) (PublicationPermitV1, PublicationBindingV1, error) {
	if err := ValidateCargoExecutionReceiptV1(receipt); err != nil {
		return PublicationPermitV1{}, PublicationBindingV1{}, err
	}
	if !receipt.ReleaseEligible {
		return PublicationPermitV1{}, PublicationBindingV1{}, ErrCargoExecutionIneligible
	}
	receiptCanonical, err := CargoExecutionReceiptV1Bytes(receipt)
	if err != nil || validateLiveCargoExecutionV1(live, receipt, receiptCanonical) != nil {
		return PublicationPermitV1{}, PublicationBindingV1{}, ErrCargoExecutionIneligible
	}
	binding := publicationBindingForReceiptV1(receipt, digestBytesV1(receiptCanonical))
	bindingCanonical, err := canonicalPublicationBindingV1(binding)
	if err != nil {
		return PublicationPermitV1{}, PublicationBindingV1{}, err
	}
	outputsCanonical, err := json.Marshal(binding.Outputs)
	if err != nil {
		return PublicationPermitV1{}, PublicationBindingV1{}, ErrCargoExecutionInvalid
	}
	if !live.state.authorized.CompareAndSwap(false, true) {
		return PublicationPermitV1{}, PublicationBindingV1{}, ErrCargoExecutionIneligible
	}
	bindingDigest := digestBytesV1(bindingCanonical)
	permit := PublicationPermitV1{
		state:            &publicationPermitStateV1{},
		executionID:      receipt.ExecutionID,
		bindingCanonical: append([]byte(nil), bindingCanonical...),
		outputsCanonical: append([]byte(nil), outputsCanonical...),
		seal:             permitSealV1(receipt.ExecutionID, bindingDigest),
	}
	return permit, clonePublicationBindingV1(binding), nil
}

func validateLiveCargoExecutionV1(
	live LiveCargoExecutionV1,
	receipt CargoExecutionReceiptV1,
	receiptCanonical []byte,
) error {
	if live.state == nil || live.state.authorized.Load() || live.executionID != receipt.ExecutionID ||
		!bytes.Equal(live.receiptCanonical, receiptCanonical) ||
		live.seal != liveCargoExecutionSealV1(receipt.ExecutionID, digestBytesV1(receiptCanonical)) {
		return ErrCargoExecutionIneligible
	}
	return nil
}

func liveCargoExecutionSealV1(executionID string, receiptDigest string) string {
	return digestBytesV1([]byte("analytix-live-cargo-execution-v1\x00" + executionID + "\x00" + receiptDigest))
}

// ConsumeExactPublicationPermitV1 validates the coordinator binding and the
// descriptor-observed outputs, then atomically consumes the permit. Validation
// and consumption are one state transition: the first presentation burns the
// permit even when its tuple is invalid, and every concurrent or later copy
// loses. This forbids correcting or replaying a failed publication attempt.
func ConsumeExactPublicationPermitV1(
	permit PublicationPermitV1,
	binding PublicationBindingV1,
	outputs []CargoOutputV1,
) error {
	if err := validatePermitV1(permit); err != nil {
		return err
	}
	if !permit.state.consumed.CompareAndSwap(false, true) {
		return ErrCargoExecutionIneligible
	}
	canonical, err := canonicalPublicationBindingV1(binding)
	if err != nil || !bytes.Equal(canonical, permit.bindingCanonical) {
		return ErrCargoExecutionIneligible
	}
	if !validOutputsForTargetV1(outputs, binding.TargetKey) {
		return ErrCargoExecutionIneligible
	}
	outputsCanonical, err := json.Marshal(outputs)
	if err != nil || !bytes.Equal(outputsCanonical, permit.outputsCanonical) {
		return ErrCargoExecutionIneligible
	}
	return nil
}

func clonePublicationBindingV1(binding PublicationBindingV1) PublicationBindingV1 {
	binding.Outputs = cloneOutputsV1(binding.Outputs)
	return binding
}

// PublicationBindingSHA256V1 returns the canonical intent identity used by a
// publisher response and read-only reconciliation. It contains no authority
// and cannot recreate a permit.
func PublicationBindingSHA256V1(binding PublicationBindingV1) (string, error) {
	canonical, err := canonicalPublicationBindingV1(binding)
	if err != nil {
		return "", err
	}
	return digestBytesV1(canonical), nil
}

func publicationBindingForReceiptV1(receipt CargoExecutionReceiptV1, receiptSHA256 string) PublicationBindingV1 {
	return PublicationBindingV1{
		ExecutionID:            receipt.ExecutionID,
		ExecutionReceiptSHA256: receiptSHA256,
		TargetKey:              receipt.TargetKey,
		TargetTriple:           receipt.TargetTriple,
		RequestNonce:           receipt.RequestNonce,
		ManifestSHA256:         receipt.ManifestSHA256,
		SourceSetSHA256:        receipt.SourceSetSHA256,
		SourceSnapshot:         receipt.SourceSnapshot,
		BuildPlanSHA256:        receipt.BuildPlanSHA256,
		BuildEnvironmentSHA256: receipt.BuildEnvironmentSHA256,
		Toolchain:              receipt.Toolchain,
		Outputs:                cloneOutputsV1(receipt.Outputs),
		OutputInventorySHA256:  receipt.OutputInventorySHA256,
		Authority:              receipt.Authority,
	}
}

func canonicalPublicationBindingV1(binding PublicationBindingV1) ([]byte, error) {
	if !validDigestV1(binding.ExecutionID) || !validDigestV1(binding.ExecutionReceiptSHA256) ||
		!validTargetV1(binding.TargetKey, binding.TargetTriple) || !validDigestV1(binding.RequestNonce) || !validDigestV1(binding.ManifestSHA256) ||
		!validDigestV1(binding.SourceSetSHA256) || !validSourceSnapshotV1(binding.SourceSnapshot) ||
		!validDigestV1(binding.BuildPlanSHA256) || !validDigestV1(binding.BuildEnvironmentSHA256) ||
		!validToolchainV1(binding.Toolchain) || !validOutputsForTargetV1(binding.Outputs, binding.TargetKey) ||
		binding.OutputInventorySHA256 != CargoOutputInventorySHA256V1(binding.Outputs) ||
		!validAuthorityV1(binding.Authority, binding.TargetKey) {
		return nil, ErrCargoExecutionInvalid
	}
	body, err := json.Marshal(binding)
	if err != nil {
		return nil, ErrCargoExecutionInvalid
	}
	return body, nil
}

func validatePermitV1(permit PublicationPermitV1) error {
	if permit.state == nil || !validDigestV1(permit.executionID) || len(permit.bindingCanonical) == 0 ||
		len(permit.outputsCanonical) == 0 || permit.seal != permitSealV1(permit.executionID, digestBytesV1(permit.bindingCanonical)) {
		return ErrCargoExecutionIneligible
	}
	return nil
}

func permitSealV1(executionID string, bindingDigest string) string {
	return digestBytesV1([]byte("analytix-publication-permit-v1\x00" + executionID + "\x00" + bindingDigest))
}

func cargoExecutionIDV1(receipt CargoExecutionReceiptV1) string {
	receipt.ExecutionID = ""
	receipt.Outputs = cloneOutputsV1(receipt.Outputs)
	body, err := json.Marshal(receipt)
	if err != nil {
		return ""
	}
	return digestBytesV1(body)
}

func cargoExecutionEligibleV1(receipt CargoExecutionReceiptV1) bool {
	return receipt.Status == CargoStatusSucceeded && receipt.TrustClass == CargoTrustControlledRelease &&
		receipt.Blocker == "" && receipt.Toolchain.OwnershipClass == ToolchainOwnershipImmutableRelease &&
		allContainmentV1(receipt.Containment)
}

func allContainmentV1(value CargoContainmentV1) bool {
	return value.SourceDescriptorsHeld && value.ToolchainDescriptorsHeld && value.LoadedImagesBound &&
		value.WorkingDirectoriesBound && value.TargetWriteScopeBound && value.NetworkDenied && value.DeadlineBound &&
		value.ProcessTreeEmpty && value.OutputsDescriptorBound && value.RawBuildDescriptorsHeld && value.PayloadIdentityBound &&
		value.TemporaryStateDestroyed &&
		value.SameUIDMutationDenied && value.ProcessContainmentBound && value.DependencyClosureBound && value.SDKClosureBound
}

func validStatusV1(value CargoExecutionStatusV1) bool {
	switch value {
	case CargoStatusSucceeded, CargoStatusBlocked, CargoStatusFailed, CargoStatusCancelled, CargoStatusTimedOut:
		return true
	default:
		return false
	}
}

func validTrustClassV1(value CargoExecutionTrustClassV1) bool {
	return value == CargoTrustLocalProvisional || value == CargoTrustControlledRelease
}

func validTargetV1(key string, triple string) bool {
	return key == "darwin-arm64" && triple == "aarch64-apple-darwin" ||
		key == "darwin-x64" && triple == "x86_64-apple-darwin" ||
		key == "win32-x64" && triple == "x86_64-pc-windows-msvc" ||
		key == "linux-x64" && triple == "x86_64-unknown-linux-gnu"
}

func validSourceSnapshotV1(value SourceSnapshotBindingV1) bool {
	return validDigestV1(value.GenerationID) && validDigestV1(value.InventorySHA256) &&
		validDigestV1(value.GenerationReceiptSHA256)
}

func validToolchainV1(value CargoToolchainIdentityV1) bool {
	if !validDigestV1(value.CargoExecutableSHA256) || !validTextV1(value.CargoVersion, 256) ||
		!validDigestV1(value.RustcExecutableSHA256) || !validTextV1(value.RustcVersion, 256) ||
		!validDigestV1(value.TreeSHA256) || value.TreeFileCount <= 0 || value.TreeFileCount > 4096 ||
		!validDigestV1(value.CompilerSHA256) || !validDigestV1(value.ArchiverSHA256) || !validDigestV1(value.LinkerSHA256) ||
		!validDigestV1(value.PlatformSignerSHA256) || !validDigestV1(value.SDKIdentitySHA256) {
		return false
	}
	switch value.OwnershipClass {
	case ToolchainOwnershipUserWritable, ToolchainOwnershipUnbound, ToolchainOwnershipImmutableRelease:
		return true
	default:
		return false
	}
}

func validOutputsV1(outputs []CargoOutputV1) bool {
	if len(outputs) != len(frozenOutputIDsV1) {
		return false
	}
	var total int64
	for index, output := range outputs {
		if output.SchemaVersion != 1 || output.ID != frozenOutputIDsV1[index] || !validDigestV1(output.SourceDigest) ||
			!validDigestV1(output.CargoLockSHA256) || !validDigestV1(output.BuildEnvironmentSHA256) ||
			!validDigestV1(output.RawBuildSHA256) || output.RawBuildSize <= 0 || output.RawBuildSize > 64<<20 ||
			!validDigestV1(output.PayloadSHA256) || output.PayloadSize <= 0 || output.PayloadSize > output.RawBuildSize ||
			!validOutputFormatArchV1(output.Format, output.Arch) || total > 192<<20-output.RawBuildSize {
			return false
		}
		total += output.RawBuildSize
	}
	return true
}

func validOutputsForTargetV1(outputs []CargoOutputV1, targetKey string) bool {
	if !validOutputsV1(outputs) {
		return false
	}
	expectedFormat, expectedArch := "", ""
	switch targetKey {
	case "darwin-arm64":
		expectedFormat, expectedArch = "mach-o", "arm64"
	case "darwin-x64":
		expectedFormat, expectedArch = "mach-o", "x64"
	case "win32-x64":
		expectedFormat, expectedArch = "pe", "x64"
	case "linux-x64":
		expectedFormat, expectedArch = "elf", "x64"
	default:
		return false
	}
	for _, output := range outputs {
		if output.Format != expectedFormat || output.Arch != expectedArch {
			return false
		}
	}
	return true
}

func validOutputFormatArchV1(format string, arch string) bool {
	return format == "mach-o" && (arch == "arm64" || arch == "x64") ||
		(format == "pe" || format == "elf") && arch == "x64"
}

func validAuthorityV1(value CargoExecutionAuthorityV1, targetKey string) bool {
	return value.Protocol == CargoExecutionAuthorityProtocol && value.TargetKey == targetKey &&
		validDigestV1(value.BinarySHA256) && validDigestV1(value.SourceSetSHA256) &&
		validDigestV1(value.BuildEnvironmentSHA256) && validDigestV1(value.GoExecutableSHA256)
}

func validDigestV1(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validTextV1(value string, maximum int) bool {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func cloneOutputsV1(outputs []CargoOutputV1) []CargoOutputV1 {
	if outputs == nil {
		return nil
	}
	return append([]CargoOutputV1(nil), outputs...)
}

func digestBytesV1(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
