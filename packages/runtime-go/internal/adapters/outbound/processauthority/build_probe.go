//go:build analytix_native_build_probe && !analytix_prod

package processauthority

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	BuildProbeRequestLimit       = 4 * 1024
	BuildProbeResponseLimit      = 8 * 1024
	buildProbeMaxExecutableBytes = int64(2 * 1024 * 1024 * 1024)
	buildProbeDeadline           = 10 * time.Second
	buildProbeCleanupLimit       = 5 * time.Second
	buildProbeGuardianArg        = "--analytix-native-build-probe-guardian-v1"
	buildProbeProtocol           = "analytix-native-build-probe-v1"
)

var (
	ErrBuildProbeInvalid     = errors.New("native_build_probe_invalid")
	ErrBuildProbeUnsupported = errors.New("native_build_probe_unsupported")
)

type BuildProbeComponent string

const (
	BuildProbeImportAccelerator BuildProbeComponent = "import-accelerator"
	BuildProbeCleaningOps       BuildProbeComponent = "cleaning-ops"
	BuildProbeAnalysisCompute   BuildProbeComponent = "analysis-compute"
	BuildProbeDataEngine        BuildProbeComponent = "data-engine"
)

// BuildProbeRequestV1 is the complete build-probe request surface. The caller
// supplies only immutable artifact assertions; execution policy, argv,
// environment, working directory, and timeout remain host-owned.
type BuildProbeRequestV1 struct {
	Kind          string `json:"kind"`
	SchemaVersion int    `json:"schema_version"`
	ComponentID   string `json:"component_id"`
	// ManifestSHA256 is a caller assertion bound into the receipt. The strict
	// JavaScript manifest host, not this probe, owns registry membership.
	ManifestSHA256           string `json:"manifest_sha256"`
	ExpectedExecutableSHA256 string `json:"expected_executable_sha256"`
	ExpectedExecutableSize   int64  `json:"expected_executable_size"`
	RequestNonce             string `json:"request_nonce"`
}

type buildProbeRequest = BuildProbeRequestV1

// BuildProbeAuthorityIdentityV1 identifies the exact opened current authority
// image. HostTarget uses the native component target spelling, for example
// darwin-arm64 or darwin-x64; no executable path is exposed.
type BuildProbeAuthorityIdentityV1 struct {
	SHA256     string
	Size       int64
	HostTarget string
}

type BuildProbeReceipt struct {
	Kind             string `json:"kind"`
	SchemaVersion    int    `json:"schema_version"`
	Status           string `json:"status"`
	ComponentID      string `json:"component_id"`
	RequestNonce     string `json:"request_nonce"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ExecutableSize   int64  `json:"executable_size"`
	// ManifestSHA256 echoes and binds the caller assertion. A matching value is
	// not proof that the digest belongs to the host's authorized manifest.
	ManifestSHA256        string `json:"manifest_sha256"`
	PolicySHA256          string `json:"policy_sha256"`
	AuthoritySHA256       string `json:"authority_sha256"`
	HostPlatform          string `json:"host_platform"`
	HostArch              string `json:"host_arch"`
	LoadedImageBound      bool   `json:"loaded_image_bound"`
	WorkingDirectoryBound bool   `json:"working_directory_bound"`
	GuardianAuthenticated bool   `json:"guardian_authenticated"`
	ProcessTreeEmpty      bool   `json:"process_tree_empty"`
}

type buildProbePolicy struct {
	component    BuildProbeComponent
	arguments    []string
	environment  []string
	policySHA256 string
}

var buildProbePolicies = map[BuildProbeComponent]buildProbePolicy{
	BuildProbeImportAccelerator: newBuildProbePolicy(
		BuildProbeImportAccelerator,
		[]string{"--analytix-native-probe"},
	),
	BuildProbeCleaningOps: newBuildProbePolicy(
		BuildProbeCleaningOps,
		[]string{"--analytix-native-probe"},
	),
	BuildProbeAnalysisCompute: newBuildProbePolicy(
		BuildProbeAnalysisCompute,
		[]string{"--analytix-native-probe"},
	),
	BuildProbeDataEngine: newBuildProbePolicy(
		BuildProbeDataEngine,
		[]string{"--analytix-native-probe"},
	),
}

func newBuildProbePolicy(
	component BuildProbeComponent,
	arguments []string,
) buildProbePolicy {
	policy := buildProbePolicy{
		component: component,
		arguments: append(make([]string, 0, len(arguments)), arguments...),
		environment: []string{
			"LANG=C",
			"LC_ALL=C",
			"TZ=UTC",
		},
	}
	body, err := json.Marshal(struct {
		Protocol    string              `json:"protocol"`
		Component   BuildProbeComponent `json:"component"`
		Arguments   []string            `json:"arguments"`
		Environment []string            `json:"environment"`
	}{buildProbeProtocol, component, policy.arguments, policy.environment})
	if err != nil {
		panic("invalid native build probe policy")
	}
	sum := sha256.Sum256(body)
	policy.policySHA256 = hex.EncodeToString(sum[:])
	return policy
}

// NativeBuildProbeMain is the only build-probe command entrypoint. It accepts
// no target path, argv, environment, working directory, or timeout surface.
// The target is inherited only as descriptor 3.
type BuildProbeProtocol struct {
	EncodePing       func(string) ([]byte, error)
	ValidateReady    func([]byte, string, string, int) bool
	ValidateResponse func([]byte, string, string, int) bool
}

func (protocol BuildProbeProtocol) valid() bool {
	return protocol.EncodePing != nil && protocol.ValidateReady != nil && protocol.ValidateResponse != nil
}

// ProbePinnedBuildArtifact authenticates a caller-opened artifact descriptor
// through the fixed build-probe coordinator. The descriptor is borrowed and
// must not be mutated concurrently. This API intentionally accepts no path,
// argv, environment, working-directory, or timeout override.
func ProbePinnedBuildArtifact(
	ctx context.Context,
	target *os.File,
	request BuildProbeRequestV1,
	protocol BuildProbeProtocol,
) (BuildProbeReceipt, error) {
	if ctx == nil || target == nil || ctx.Err() != nil || !protocol.valid() ||
		validateBuildProbeRequest(request) != nil || !currentBuildProbeMainAllowed() {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	deadline, cancel := context.WithTimeout(ctx, buildProbeDeadline)
	defer cancel()
	authority, err := CurrentBuildProbeAuthorityIdentity(deadline)
	if err != nil || !validBuildProbeAuthorityIdentity(authority) {
		return BuildProbeReceipt{}, errOrBuildProbeInvalid(err)
	}
	receipt, err := probePinnedBuildArtifact(deadline, target, request, protocol)
	if err != nil {
		return BuildProbeReceipt{}, err
	}
	if deadline.Err() != nil || !validBuildProbeReceipt(receipt, request) ||
		receipt.AuthoritySHA256 != authority.SHA256 ||
		receipt.ExecutableSHA256 != request.ExpectedExecutableSHA256 ||
		receipt.ExecutableSize != request.ExpectedExecutableSize {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	return receipt, nil
}

// CurrentBuildProbeAuthorityIdentity returns the exact opened current build
// authority image identity. The implementation closes its private descriptor
// before returning and is unavailable to ordinary or production source sets.
func CurrentBuildProbeAuthorityIdentity(ctx context.Context) (BuildProbeAuthorityIdentityV1, error) {
	if ctx == nil || ctx.Err() != nil || !currentBuildProbeMainAllowed() {
		return BuildProbeAuthorityIdentityV1{}, ErrBuildProbeInvalid
	}
	deadline, cancel := context.WithTimeout(ctx, buildProbeDeadline)
	defer cancel()
	identity, err := currentBuildProbeAuthorityIdentity(deadline)
	if err != nil || deadline.Err() != nil || !validBuildProbeAuthorityIdentity(identity) {
		return BuildProbeAuthorityIdentityV1{}, errOrBuildProbeInvalid(err)
	}
	return identity, nil
}

func validBuildProbeAuthorityIdentity(identity BuildProbeAuthorityIdentityV1) bool {
	return validBuildProbeHex(identity.SHA256) && identity.Size > 0 &&
		identity.Size <= buildProbeMaxExecutableBytes && identity.HostTarget == buildProbeHostTarget()
}

func errOrBuildProbeInvalid(err error) error {
	if err != nil {
		return err
	}
	return ErrBuildProbeInvalid
}

func NativeBuildProbeMain(protocol BuildProbeProtocol) int {
	if !currentBuildProbeMainAllowed() {
		return 125
	}
	if !protocol.valid() {
		return 125
	}
	switch len(os.Args) {
	case 1:
		return runBuildProbeCoordinatorMain(protocol)
	case 2:
		if os.Args[1] == buildProbeGuardianArg {
			return runBuildProbeGuardianMain(protocol)
		}
	}
	return 125
}

func runBuildProbeCoordinatorMain(protocol BuildProbeProtocol) int {
	ctx, cancel := context.WithTimeout(context.Background(), buildProbeDeadline)
	defer cancel()
	target, err := openBuildProbeTarget()
	if err != nil || target == nil {
		return 1
	}
	defer target.Close()
	requestInput, err := openBuildProbeRequestInput(0, "analytix-native-build-probe-stdin")
	if err != nil || requestInput == nil {
		return 1
	}
	defer requestInput.Close()
	request, err := readBuildProbeRequest(ctx, requestInput)
	if err != nil {
		return 1
	}
	receipt, err := ProbePinnedBuildArtifact(ctx, target, request, protocol)
	if err != nil || ctx.Err() != nil || !validBuildProbeReceipt(receipt, request) {
		return 1
	}
	body, err := json.Marshal(receipt)
	if err != nil || len(body)+1 > BuildProbeResponseLimit {
		return 1
	}
	body = append(body, '\n')
	if err := writeBuildProbeAll(os.Stdout, body); err != nil {
		return 1
	}
	return 0
}

func readBuildProbeRequest(ctx context.Context, reader io.Reader) (buildProbeRequest, error) {
	var request buildProbeRequest
	if ctx == nil || reader == nil || ctx.Err() != nil {
		return request, ErrBuildProbeInvalid
	}
	stopDeadline, err := applyBuildProbeReadDeadline(ctx, reader)
	if err != nil {
		return request, ErrBuildProbeInvalid
	}
	defer stopDeadline()
	buffered := bufio.NewReaderSize(reader, BuildProbeRequestLimit+1)
	raw, err := buffered.ReadSlice('\n')
	if err != nil || len(raw) <= 1 || len(raw) > BuildProbeRequestLimit ||
		bytes.IndexByte(raw[:len(raw)-1], '\n') >= 0 || bytes.IndexByte(raw, '\r') >= 0 || bytes.IndexByte(raw, 0) >= 0 {
		return request, ErrBuildProbeInvalid
	}
	// A request is exactly one bounded frame followed by EOF. Requiring EOF
	// rejects both pipelined bytes and a writer that keeps a partial authority
	// request open; the fixed command deadline bounds this final read.
	if _, err := buffered.ReadByte(); !errors.Is(err, io.EOF) {
		return request, ErrBuildProbeInvalid
	}
	frame := raw[:len(raw)-1]
	if err := domainjsonstrict.Validate(frame, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       BuildProbeRequestLimit - 1,
		MaxDepth:       2,
		MaxTokens:      32,
		MaxStringBytes: 256,
		MaxNumberBytes: 32,
		MaxAbsExponent: 1,
	}); err != nil {
		return request, ErrBuildProbeInvalid
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(frame, &shape) != nil || len(shape) != 7 {
		return request, ErrBuildProbeInvalid
	}
	for _, key := range []string{
		"kind", "schema_version", "component_id", "manifest_sha256",
		"expected_executable_sha256", "expected_executable_size", "request_nonce",
	} {
		if _, ok := shape[key]; !ok {
			return request, ErrBuildProbeInvalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return buildProbeRequest{}, ErrBuildProbeInvalid
	}
	if err := validateBuildProbeRequest(request); err != nil {
		return buildProbeRequest{}, err
	}
	return request, nil
}

type buildProbeReadDeadlineSetter interface {
	SetReadDeadline(time.Time) error
}

func applyBuildProbeReadDeadline(ctx context.Context, reader io.Reader) (func(), error) {
	if ctx == nil || reader == nil {
		return func() {}, ErrBuildProbeInvalid
	}
	setter, supportsDeadline := reader.(buildProbeReadDeadlineSetter)
	if !supportsDeadline {
		return func() {}, nil
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return func() {}, ErrBuildProbeInvalid
	}
	if err := setter.SetReadDeadline(deadline); err != nil {
		if file, isFile := reader.(*os.File); isFile {
			stat, statErr := file.Stat()
			if statErr == nil && stat.Mode().IsRegular() {
				return func() {}, nil
			}
		}
		return func() {}, err
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = setter.SetReadDeadline(time.Now())
	})
	return func() {
		stopCancellation()
		_ = setter.SetReadDeadline(time.Time{})
	}, nil
}

func validateBuildProbeRequest(request buildProbeRequest) error {
	component := BuildProbeComponent(request.ComponentID)
	if request.Kind != "analytix_native_build_probe_request" || request.SchemaVersion != 1 ||
		request.ExpectedExecutableSize <= 0 || request.ExpectedExecutableSize > buildProbeMaxExecutableBytes ||
		!validBuildProbeHex(request.ManifestSHA256) || !validBuildProbeHex(request.ExpectedExecutableSHA256) ||
		!validBuildProbeHex(request.RequestNonce) {
		return ErrBuildProbeInvalid
	}
	policy, ok := buildProbePolicies[component]
	if !ok || policy.component != component || !validBuildProbeHex(policy.policySHA256) {
		return ErrBuildProbeInvalid
	}
	return nil
}

func validBuildProbeReceipt(receipt BuildProbeReceipt, request buildProbeRequest) bool {
	policy, ok := buildProbePolicies[BuildProbeComponent(request.ComponentID)]
	return ok && receipt.Kind == "analytix_native_build_probe_receipt" && receipt.SchemaVersion == 1 &&
		receipt.Status == "passed" && receipt.ComponentID == request.ComponentID &&
		receipt.RequestNonce == request.RequestNonce && receipt.ExecutableSHA256 == request.ExpectedExecutableSHA256 &&
		receipt.ExecutableSize == request.ExpectedExecutableSize && receipt.ManifestSHA256 == request.ManifestSHA256 &&
		receipt.PolicySHA256 == policy.policySHA256 && validBuildProbeHex(receipt.AuthoritySHA256) &&
		receipt.HostPlatform == runtime.GOOS && receipt.HostArch == buildProbeHostArch() && receipt.LoadedImageBound &&
		receipt.WorkingDirectoryBound && receipt.GuardianAuthenticated && receipt.ProcessTreeEmpty
}

func decodeBuildProbeReceipt(raw []byte) (BuildProbeReceipt, error) {
	var receipt BuildProbeReceipt
	if len(raw) == 0 || len(raw) >= BuildProbeResponseLimit {
		return receipt, ErrBuildProbeInvalid
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       BuildProbeResponseLimit - 1,
		MaxDepth:       2,
		MaxTokens:      64,
		MaxStringBytes: 256,
		MaxNumberBytes: 32,
		MaxAbsExponent: 1,
	}); err != nil {
		return receipt, ErrBuildProbeInvalid
	}
	var shape map[string]json.RawMessage
	if json.Unmarshal(raw, &shape) != nil || len(shape) != 16 {
		return receipt, ErrBuildProbeInvalid
	}
	for _, key := range []string{
		"kind", "schema_version", "status", "component_id", "request_nonce",
		"executable_sha256", "executable_size", "manifest_sha256", "policy_sha256",
		"authority_sha256", "host_platform", "host_arch", "loaded_image_bound",
		"working_directory_bound", "guardian_authenticated", "process_tree_empty",
	} {
		if _, ok := shape[key]; !ok {
			return BuildProbeReceipt{}, ErrBuildProbeInvalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil {
		return BuildProbeReceipt{}, ErrBuildProbeInvalid
	}
	return receipt, nil
}

func buildProbeHostArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return ""
	}
}

func buildProbeHostTarget() string {
	arch := buildProbeHostArch()
	if runtime.GOOS == "" || arch == "" {
		return ""
	}
	return runtime.GOOS + "-" + arch
}

func validBuildProbeHex(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func writeBuildProbeAll(writer io.Writer, payload []byte) error {
	if writer == nil || len(payload) == 0 {
		return ErrBuildProbeInvalid
	}
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil || written <= 0 {
			return ErrBuildProbeInvalid
		}
		payload = payload[written:]
	}
	return nil
}
