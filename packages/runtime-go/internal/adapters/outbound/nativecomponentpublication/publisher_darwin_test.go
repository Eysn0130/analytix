//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentpublication

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	securegeneration "analytix.local/runtime-go/internal/adapters/outbound/securegeneration"
	domainnativebuild "analytix.local/runtime-go/internal/domain/nativebuild"

	"golang.org/x/sys/unix"
)

func TestPublishFromDescriptorsUsesPinnedObjectsAndAtomicGeneration(t *testing.T) {
	request, authority := publicationRequestFixture(t)
	root := request.PublicationRoot
	manifest, rawComponents, components, originals := publicationDescriptorFixture(t)
	defer manifest.Close()
	defer closeFiles(rawComponents)
	defer closeFiles(components)

	// Replace every caller-visible source path after the descriptors are open.
	// Publication must still consume the original pinned objects.
	for index, path := range originals {
		replacement := append([]byte(nil), syntheticPublicationMachO(t)...)
		replacement[200] = byte(0x80 + index)
		if err := os.Rename(path, path+".pinned"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, replacement, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	probedFDs := make(map[uintptr]bool)
	publicationAuthority := publicationAuthorityFixture(t, request, authority, rawComponents)
	response, err := PublishFromDescriptorsV1(context.Background(), request, DescriptorInputsV1{
		Manifest: manifest, RawComponents: rawComponents, Components: components,
	}, publicationAuthority, func(_ context.Context, file *os.File, request ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error) {
		probedFDs[file.Fd()] = true
		contract := frozenComponentByID(t, request.ComponentID)
		return validProbeFixture(request, contract, authority), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "committed" || response.TargetKey != request.TargetKey || response.GenerationID == "" ||
		!validDigestV1(response.PublicationBindingSHA256) || len(probedFDs) != len(frozenComponentsV1) ||
		response.CleanupRecovered || response.CleanupPending {
		t.Fatalf("unexpected response: %#v probed=%d", response, len(probedFDs))
	}
	current := filepath.Join(root, PublicNameV1)
	for index, contract := range frozenComponentsV1 {
		published, err := os.ReadFile(filepath.Join(current, contract.binaryName))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := os.ReadFile(originals[index] + ".pinned")
		if err != nil {
			t.Fatal(err)
		}
		if digestBytesV1(published) != digestBytesV1(expected) {
			t.Fatalf("%s did not publish the pinned bytes", contract.id)
		}
		assertPublicationMode(t, filepath.Join(current, contract.binaryName), 0o700)
	}
	for _, name := range []string{
		GenerationContextFileNameV1, PublicationIntentFileNameV1, nativecomponentregistry.ReceiptFileName,
		"inventory.v1.json", "receipt.v1.json",
	} {
		assertPublicationMode(t, filepath.Join(current, name), 0o600)
	}
	inner, err := os.ReadFile(filepath.Join(current, nativecomponentregistry.ReceiptFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nativecomponentregistry.ParseControlledBuildReceipt(inner, FrozenManifestSHA256V4, request.TargetKey); err != nil {
		t.Fatalf("published inner receipt: %v", err)
	}
	intentBody, err := os.ReadFile(filepath.Join(current, PublicationIntentFileNameV1))
	if err != nil {
		t.Fatal(err)
	}
	var intent publicationIntentV1
	if err := json.Unmarshal(intentBody, &intent); err != nil || intent.Kind != PublicationIntentKindV1 ||
		intent.SchemaVersion != SchemaVersionV1 || intent.RequestNonce != request.RequestNonce ||
		intent.PublicationBindingSHA256 != response.PublicationBindingSHA256 {
		t.Fatalf("publication intent=%#v err=%v", intent, err)
	}
}

func TestPublicationAdmissionCancellationAndCommitAreLinearized(t *testing.T) {
	t.Run("cancellation wins", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		commitCtx, finish, err := admitPublicationContextV1(ctx)
		if !errors.Is(err, context.Canceled) || commitCtx != nil || finish != nil {
			t.Fatalf("commitCtx=%v finish=%v err=%v", commitCtx, finish, err)
		}
	})

	t.Run("admission wins before late cancellation", func(t *testing.T) {
		ctx, cancelCaller := context.WithCancel(context.Background())
		commitCtx, finish, err := admitPublicationContextV1(ctx)
		if err != nil {
			t.Fatalf("admit publication: %v", err)
		}
		defer finish()
		cancelCaller()
		if ctx.Err() == nil {
			t.Fatal("caller context did not cancel")
		}
		if commitCtx.Err() != nil {
			t.Fatalf("late caller cancellation interrupted admitted commit: %v", commitCtx.Err())
		}
		if deadline, ok := commitCtx.Deadline(); !ok || time.Until(deadline) <= 0 || time.Until(deadline) > publicationCommitDeadlineV1 {
			t.Fatalf("commit deadline=%v ok=%v", deadline, ok)
		}
	})
}

func TestPublishFromDescriptorsStaleCASHasNoGenerationEffect(t *testing.T) {
	request, authority := publicationRequestFixture(t)
	manifest, rawComponents, components, _ := publicationDescriptorFixture(t)
	defer manifest.Close()
	defer closeFiles(rawComponents)
	defer closeFiles(components)
	probe := probeFixture(authority)
	firstAuthority := publicationAuthorityFixture(t, request, authority, rawComponents)
	first, err := PublishFromDescriptorsV1(context.Background(), request, DescriptorInputsV1{Manifest: manifest, RawComponents: rawComponents, Components: components}, firstAuthority, probe)
	if err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(request.PublicationRoot, PublicNameV1)
	before, err := os.Stat(current)
	if err != nil {
		t.Fatal(err)
	}

	manifest2, rawComponents2, components2, paths2 := publicationDescriptorFixture(t)
	defer manifest2.Close()
	defer closeFiles(rawComponents2)
	defer closeFiles(components2)
	if err := rawComponents2[0].Close(); err != nil {
		t.Fatal(err)
	}
	if err := components2[0].Close(); err != nil {
		t.Fatal(err)
	}
	mutator, err := os.OpenFile(paths2[0], os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.WriteAt([]byte{0xff}, 200); err != nil {
		_ = mutator.Close()
		t.Fatal(err)
	}
	if err := mutator.Close(); err != nil {
		t.Fatal(err)
	}
	components2[0], err = os.Open(paths2[0])
	if err != nil {
		t.Fatal(err)
	}
	rawComponents2[0], err = os.Open(paths2[0])
	if err != nil {
		t.Fatal(err)
	}
	secondAuthority := publicationAuthorityFixture(t, request, authority, rawComponents2)
	_, err = PublishFromDescriptorsV1(context.Background(), request, DescriptorInputsV1{Manifest: manifest2, RawComponents: rawComponents2, Components: components2}, secondAuthority, probe)
	if !errors.Is(err, securegeneration.ErrCurrentGeneration) {
		t.Fatalf("stale CAS err=%v", err)
	}
	after, statErr := os.Stat(current)
	if statErr != nil || !os.SameFile(before, after) {
		t.Fatalf("stale CAS changed current identity: before=%v after=%v err=%v", before, after, statErr)
	}
	receipt, err := os.ReadFile(filepath.Join(current, "receipt.v1.json"))
	if err != nil || digestBytesV1(receipt) != first.GenerationReceiptSHA256 {
		t.Fatalf("stale CAS changed generation receipt: err=%v", err)
	}
}

func TestPublishFromDescriptorsRejectsProbeMismatchBeforeRootCreation(t *testing.T) {
	request, authority := publicationRequestFixture(t)
	manifest, rawComponents, components, _ := publicationDescriptorFixture(t)
	defer manifest.Close()
	defer closeFiles(rawComponents)
	defer closeFiles(components)
	publicationAuthority := publicationAuthorityFixture(t, request, authority, rawComponents)
	_, err := PublishFromDescriptorsV1(context.Background(), request, DescriptorInputsV1{Manifest: manifest, RawComponents: rawComponents, Components: components}, publicationAuthority,
		func(_ context.Context, _ *os.File, request ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error) {
			receipt := validProbeFixture(request, frozenComponentByID(t, request.ComponentID), authority)
			receipt.ProcessTreeEmpty = false
			return receipt, nil
		})
	if !errors.Is(err, ErrProbe) {
		t.Fatalf("probe mismatch err=%v", err)
	}
	if _, statErr := os.Lstat(request.PublicationRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid probe created publication root: %v", statErr)
	}
}

func TestPublishBindsRawBuildAndFinalSigningInvariantPayload(t *testing.T) {
	t.Run("signature bytes may differ", func(t *testing.T) {
		request, authority := publicationRequestFixture(t)
		manifest, rawComponents, components, _ := publicationDescriptorFixture(t)
		defer manifest.Close()
		defer closeFiles(rawComponents)
		defer closeFiles(components)

		rawBody, err := os.ReadFile(components[0].Name())
		if err != nil {
			t.Fatal(err)
		}
		rawBody[len(rawBody)-1] ^= 0x7f
		replaceFixtureDescriptor(t, rawComponents, 0, rawBody)
		publicationAuthority := publicationAuthorityFixture(t, request, authority, rawComponents)
		if _, err := PublishFromDescriptorsV1(
			context.Background(),
			request,
			DescriptorInputsV1{Manifest: manifest, RawComponents: rawComponents, Components: components},
			publicationAuthority,
			probeFixture(authority),
		); err != nil {
			t.Fatalf("signature-only raw/final difference rejected: %v", err)
		}
	})

	t.Run("payload difference is rejected before publication", func(t *testing.T) {
		request, authority := publicationRequestFixture(t)
		manifest, rawComponents, components, _ := publicationDescriptorFixture(t)
		defer manifest.Close()
		defer closeFiles(rawComponents)
		defer closeFiles(components)

		rawBody, err := os.ReadFile(components[0].Name())
		if err != nil {
			t.Fatal(err)
		}
		rawBody[200] ^= 0x7f
		replaceFixtureDescriptor(t, rawComponents, 0, rawBody)
		publicationAuthority := publicationAuthorityFixture(t, request, authority, rawComponents)
		probeCalls := 0
		_, err = PublishFromDescriptorsV1(
			context.Background(),
			request,
			DescriptorInputsV1{Manifest: manifest, RawComponents: rawComponents, Components: components},
			publicationAuthority,
			func(context.Context, *os.File, ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error) {
				probeCalls++
				return nativecomponentregistry.ExecutionProbeReceipt{}, nil
			},
		)
		if !errors.Is(err, ErrCargoExecution) || probeCalls != 0 {
			t.Fatalf("payload mismatch err=%v probeCalls=%d", err, probeCalls)
		}
		if _, statErr := os.Lstat(request.PublicationRoot); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("payload mismatch created publication root: %v", statErr)
		}
	})
}

func TestPublishRejectsIneligibleCargoBeforeAnyEffect(t *testing.T) {
	request, _ := publicationRequestFixture(t)
	probeCalls := 0
	_, err := PublishFromDescriptorsV1(
		context.Background(), request, DescriptorInputsV1{}, PublicationAuthorityV1{},
		func(context.Context, *os.File, ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error) {
			probeCalls++
			return nativecomponentregistry.ExecutionProbeReceipt{}, nil
		},
	)
	if !errors.Is(err, ErrCargoExecution) {
		t.Fatalf("missing Cargo permit err=%v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("missing Cargo permit reached probe %d times", probeCalls)
	}
	if _, statErr := os.Lstat(request.PublicationRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing Cargo permit created publication root: %v", statErr)
	}
}

func TestPublishRejectsRequestNonceDifferentFromPermitBeforeAnyEffect(t *testing.T) {
	request, authority := publicationRequestFixture(t)
	manifest, rawComponents, components, _ := publicationDescriptorFixture(t)
	defer manifest.Close()
	defer closeFiles(rawComponents)
	defer closeFiles(components)
	publicationAuthority := publicationAuthorityFixture(t, request, authority, rawComponents)
	request.RequestNonce = digestBytesV1([]byte("different-publication-request"))
	probeCalls := 0
	_, err := PublishFromDescriptorsV1(
		context.Background(),
		request,
		DescriptorInputsV1{Manifest: manifest, RawComponents: rawComponents, Components: components},
		publicationAuthority,
		func(context.Context, *os.File, ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error) {
			probeCalls++
			return nativecomponentregistry.ExecutionProbeReceipt{}, nil
		},
	)
	if !errors.Is(err, ErrCargoExecution) {
		t.Fatalf("mismatched request nonce err=%v", err)
	}
	if probeCalls != 0 {
		t.Fatalf("mismatched request nonce reached probe %d times", probeCalls)
	}
	if _, statErr := os.Lstat(request.PublicationRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("mismatched request nonce created publication root: %v", statErr)
	}
}

func TestDecodePublishRequestRejectsUnknownAndCallerAuthorityFields(t *testing.T) {
	request, _ := publicationRequestFixture(t)
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePublishRequestV1(body); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"binary_sha256", "binary_name", "package_path", "execution_probe",
		"cargo_execution_receipt", "release_eligible", "publication_permit",
	} {
		value[field] = stringsOfDigest(field)
		mutated, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, decodeErr := DecodePublishRequestV1(mutated); !errors.Is(decodeErr, ErrInvalidRequest) {
			t.Fatalf("caller authority field %s accepted: %v", field, decodeErr)
		}
		delete(value, field)
	}
	for _, mutation := range []struct {
		name         string
		selectObject func(map[string]any) map[string]any
	}{
		{name: "toolchain", selectObject: func(value map[string]any) map[string]any { return value["toolchain"].(map[string]any) }},
		{name: "authority provenance", selectObject: func(value map[string]any) map[string]any { return value["authority_provenance"].(map[string]any) }},
		{name: "expected current", selectObject: func(value map[string]any) map[string]any { return value["expected_current"].(map[string]any) }},
		{name: "component", selectObject: func(value map[string]any) map[string]any { return value["components"].([]any)[0].(map[string]any) }},
	} {
		var nested map[string]any
		if err := json.Unmarshal(body, &nested); err != nil {
			t.Fatal(err)
		}
		mutation.selectObject(nested)["unexpected_authority"] = stringsOfDigest(mutation.name)
		mutated, marshalErr := json.Marshal(nested)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, decodeErr := DecodePublishRequestV1(mutated); !errors.Is(decodeErr, ErrInvalidRequest) {
			t.Fatalf("nested caller authority field in %s accepted: %v", mutation.name, decodeErr)
		}
	}
}

func TestPublisherChannelAcceptsSpawnPipesAndSocketsOnly(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	openedPipe, err := openPublisherPipeV1(int(reader.Fd()), "publisher-pipe")
	if err != nil {
		t.Fatalf("spawn pipe rejected: %v", err)
	}
	if err := openedPipe.Close(); err != nil {
		t.Fatal(err)
	}

	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])
	openedSocket, err := openPublisherPipeV1(sockets[0], "publisher-socket")
	if err != nil {
		t.Fatalf("spawn socket rejected: %v", err)
	}
	if err := openedSocket.Close(); err != nil {
		t.Fatal(err)
	}

	regular, err := os.Open(filepath.Join("testdata", "not-present"))
	if err == nil {
		_ = regular.Close()
		t.Fatal("unexpected regular fixture")
	}
	regularPath := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(regularPath, []byte("frame\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	regular, err = os.Open(regularPath)
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if opened, openErr := openPublisherPipeV1(int(regular.Fd()), "publisher-regular"); !errors.Is(openErr, ErrInvalidInput) {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatalf("regular file accepted as publisher channel: %v", openErr)
	}
}

func TestPublisherFrameUsesBoundedDuplicatedPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	input, err := openPublisherPipeV1(int(reader.Fd()), "publisher-input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	frame := []byte(`{"kind":"fixture"}` + "\n")
	if _, err := writer.Write(frame); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	decoded, err := readPublisherFrameV1(ctx, input)
	if err != nil || string(decoded) != `{"kind":"fixture"}` {
		t.Fatalf("read frame=%q err=%v", decoded, err)
	}

	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer outputReader.Close()
	defer outputWriter.Close()
	output, err := openPublisherPipeV1(int(outputWriter.Fd()), "publisher-output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if err := writePublisherFrameV1(ctx, output, frame); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, len(frame))
	if _, err := outputReader.Read(received); err != nil || string(received) != string(frame) {
		t.Fatalf("write frame=%q err=%v", received, err)
	}
}

func TestPublicationBlockerIsClosedAndStable(t *testing.T) {
	for _, test := range []struct {
		err        error
		contextErr error
		expected   string
	}{
		{err: ErrCargoExecution, expected: "cargo_execution_ineligible"},
		{err: ErrInvalidRequest, expected: "publication_request_invalid"},
		{err: ErrInvalidInput, expected: "publication_input_invalid"},
		{err: ErrProbe, expected: "publication_probe_failed"},
		{err: ErrPublication, expected: "publication_commit_failed"},
		{err: errors.New("unknown"), expected: "publication_failed"},
		{contextErr: context.DeadlineExceeded, expected: "publication_deadline_exceeded"},
	} {
		if actual := publicationBlockerV1(test.err, test.contextErr); actual != test.expected {
			t.Fatalf("blocker=%q expected=%q", actual, test.expected)
		}
	}
}

func publicationRequestFixture(t *testing.T) (PublishRequestV1, AuthorityIdentityV1) {
	t.Helper()
	target, ok := currentTargetV1()
	if !ok {
		t.Fatal("unsupported Darwin architecture")
	}
	authority := AuthorityIdentityV1{
		SHA256: digestBytesV1([]byte("authority")), Size: 4096,
		Platform: "darwin", Arch: target.arch, TargetKey: target.key,
	}
	request := PublishRequestV1{
		Kind: RequestKindV1, SchemaVersion: SchemaVersionV1,
		RequestNonce:    digestBytesV1([]byte(t.Name())),
		PublicationRoot: filepath.Join(t.TempDir(), "store"), PublicName: PublicNameV1,
		ExpectedCurrent: ExpectedCurrentV1{Kind: "absent"}, TargetKey: target.key,
		SourceSetSHA256:        digestBytesV1([]byte("source-set")),
		BuildEnvironmentSHA256: digestBytesV1([]byte("build-environment")),
		Toolchain: ToolchainProvenanceV1{
			CargoExecutableSHA256: digestBytesV1([]byte("cargo")), CargoVersion: "cargo 1.94.1 (fixture)",
			RustcExecutableSHA256: digestBytesV1([]byte("rustc")), RustcVersion: "rustc 1.94.1 (fixture)",
		},
		AuthorityProvenance: AuthorityProvenanceV1{
			SourceSetSHA256:        digestBytesV1([]byte("authority-source")),
			BuildEnvironmentSHA256: digestBytesV1([]byte("authority-environment")),
			GoToolchainKey:         target.key, GoExecutableSHA256: digestBytesV1([]byte("go")),
		},
	}
	for _, contract := range frozenComponentsV1 {
		request.Components = append(request.Components, ComponentProvenanceV1{
			ID: contract.id, SourceDigest: digestBytesV1([]byte("source:" + contract.id)),
			CargoLockSHA256:        digestBytesV1([]byte("lock:" + contract.id)),
			BuildEnvironmentSHA256: digestBytesV1([]byte("environment:" + contract.id)),
		})
	}
	return request, authority
}

func publicationAuthorityFixture(
	t *testing.T,
	request PublishRequestV1,
	identity AuthorityIdentityV1,
	rawComponents []*os.File,
) PublicationAuthorityV1 {
	t.Helper()
	target, ok := currentTargetV1()
	if !ok || len(rawComponents) != len(frozenComponentsV1) {
		t.Fatal("invalid publication authority fixture target or component inventory")
	}
	outputs := make([]domainnativebuild.CargoOutputV1, 0, len(rawComponents))
	for index, rawFile := range rawComponents {
		rawPinned, err := validateExecutableDescriptorV1(rawFile)
		if err != nil {
			t.Fatalf("pin publication component %d: %v", index, err)
		}
		rawBuildSHA256, err := hashPinnedFileV1(context.Background(), rawFile, rawPinned.size)
		if err != nil {
			t.Fatalf("hash publication component %d: %v", index, err)
		}
		payloadSHA256, payloadSize, arch, err := nativecomponentregistry.InspectDarwinPayload(rawFile, rawPinned.size)
		if err != nil {
			t.Fatalf("inspect publication component %d: %v", index, err)
		}
		component := request.Components[index]
		outputs = append(outputs, domainnativebuild.CargoOutputV1{
			SchemaVersion: 1, ID: component.ID, SourceDigest: component.SourceDigest, CargoLockSHA256: component.CargoLockSHA256,
			BuildEnvironmentSHA256: component.BuildEnvironmentSHA256,
			RawBuildSHA256:         rawBuildSHA256, RawBuildSize: rawPinned.size,
			PayloadSHA256: payloadSHA256, PayloadSize: payloadSize, Format: "mach-o", Arch: arch,
		})
	}
	receipt := domainnativebuild.CargoExecutionReceiptV1{
		SchemaVersion:   domainnativebuild.CargoExecutionSchemaVersionV1,
		Purpose:         domainnativebuild.CargoExecutionPurposeV1,
		RequestNonce:    request.RequestNonce,
		Status:          domainnativebuild.CargoStatusSucceeded,
		TrustClass:      domainnativebuild.CargoTrustControlledRelease,
		ReleaseEligible: true,
		TargetKey:       target.key,
		TargetTriple:    target.triple,
		ManifestSHA256:  FrozenManifestSHA256V4,
		SourceSetSHA256: request.SourceSetSHA256,
		SourceSnapshot: domainnativebuild.SourceSnapshotBindingV1{
			GenerationID:            digestBytesV1([]byte("snapshot-generation")),
			InventorySHA256:         digestBytesV1([]byte("snapshot-inventory")),
			GenerationReceiptSHA256: digestBytesV1([]byte("snapshot-receipt")),
		},
		BuildPlanSHA256:        digestBytesV1([]byte("build-plan")),
		BuildEnvironmentSHA256: request.BuildEnvironmentSHA256,
		Toolchain: domainnativebuild.CargoToolchainIdentityV1{
			CargoExecutableSHA256: request.Toolchain.CargoExecutableSHA256,
			CargoVersion:          request.Toolchain.CargoVersion,
			RustcExecutableSHA256: request.Toolchain.RustcExecutableSHA256,
			RustcVersion:          request.Toolchain.RustcVersion,
			TreeSHA256:            digestBytesV1([]byte("toolchain-tree")), TreeFileCount: 32,
			CompilerSHA256: digestBytesV1([]byte("compiler")), ArchiverSHA256: digestBytesV1([]byte("archiver")),
			LinkerSHA256: digestBytesV1([]byte("linker")), PlatformSignerSHA256: digestBytesV1([]byte("signer")),
			SDKIdentitySHA256: digestBytesV1([]byte("sdk")),
			OwnershipClass:    domainnativebuild.ToolchainOwnershipImmutableRelease,
		},
		Containment: domainnativebuild.CargoContainmentV1{
			SourceDescriptorsHeld: true, ToolchainDescriptorsHeld: true, LoadedImagesBound: true,
			WorkingDirectoriesBound: true, TargetWriteScopeBound: true, NetworkDenied: true,
			DeadlineBound: true, ProcessTreeEmpty: true, OutputsDescriptorBound: true,
			RawBuildDescriptorsHeld: true, PayloadIdentityBound: true,
			TemporaryStateDestroyed: true, SameUIDMutationDenied: true, ProcessContainmentBound: true,
			DependencyClosureBound: true, SDKClosureBound: true,
		},
		Outputs:               outputs,
		OutputInventorySHA256: domainnativebuild.CargoOutputInventorySHA256V1(outputs),
		Authority: domainnativebuild.CargoExecutionAuthorityV1{
			Protocol:  domainnativebuild.CargoExecutionAuthorityProtocol,
			TargetKey: target.key, BinarySHA256: identity.SHA256,
			SourceSetSHA256:        request.AuthorityProvenance.SourceSetSHA256,
			BuildEnvironmentSHA256: request.AuthorityProvenance.BuildEnvironmentSHA256,
			GoExecutableSHA256:     request.AuthorityProvenance.GoExecutableSHA256,
		},
	}
	receipt, live, err := domainnativebuild.FinalizeObservedCargoExecutionV1(
		domainnativebuild.BeginCargoExecutionObservationV1(),
		receipt,
	)
	if err != nil {
		t.Fatalf("finalize cargo execution fixture: %v", err)
	}
	permit, binding, err := domainnativebuild.AuthorizeForPublicationV1(live, receipt)
	if err != nil {
		t.Fatalf("authorize cargo execution fixture: %v", err)
	}
	return PublicationAuthorityV1{Identity: identity, Permit: permit, Binding: binding}
}

func publicationDescriptorFixture(t *testing.T) (*os.File, []*os.File, []*os.File, []string) {
	t.Helper()
	directory := t.TempDir()
	manifestSource := filepath.Join("..", "..", "..", "..", "..", "..", "scripts", "native-components.json")
	manifestBody, err := os.ReadFile(manifestSource)
	if err != nil || digestBytesV1(manifestBody) != FrozenManifestSHA256V4 {
		t.Fatalf("manifest fixture err=%v digest=%s", err, digestBytesV1(manifestBody))
	}
	manifestPath := filepath.Join(directory, "native-components.json")
	if err := os.WriteFile(manifestPath, manifestBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(manifestPath, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	rawComponents := make([]*os.File, 0, len(frozenComponentsV1))
	components := make([]*os.File, 0, len(frozenComponentsV1))
	paths := make([]string, 0, len(frozenComponentsV1))
	for index, contract := range frozenComponentsV1 {
		body := append([]byte(nil), syntheticPublicationMachO(t)...)
		body[200] = byte(index + 1)
		path := filepath.Join(directory, contract.binaryName)
		if err := os.WriteFile(path, body, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
		rawFile, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			_ = rawFile.Close()
			t.Fatal(err)
		}
		rawComponents = append(rawComponents, rawFile)
		components = append(components, file)
		paths = append(paths, path)
	}
	return manifest, rawComponents, components, paths
}

func replaceFixtureDescriptor(t *testing.T, files []*os.File, index int, body []byte) {
	t.Helper()
	if index < 0 || index >= len(files) || len(body) == 0 {
		t.Fatal("invalid fixture descriptor replacement")
	}
	if err := files[index].Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "replacement")
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	files[index] = file
}

func probeFixture(authority AuthorityIdentityV1) ProbeFuncV1 {
	return func(_ context.Context, _ *os.File, request ProbeRequestV1) (nativecomponentregistry.ExecutionProbeReceipt, error) {
		for _, contract := range frozenComponentsV1 {
			if contract.id == request.ComponentID {
				return validProbeFixture(request, contract, authority), nil
			}
		}
		return nativecomponentregistry.ExecutionProbeReceipt{}, ErrProbe
	}
}

func validProbeFixture(request ProbeRequestV1, contract frozenComponentV1, authority AuthorityIdentityV1) nativecomponentregistry.ExecutionProbeReceipt {
	return nativecomponentregistry.ExecutionProbeReceipt{
		Kind: "analytix_native_build_probe_receipt", SchemaVersion: 1, Status: "passed",
		ComponentID: contract.id, RequestNonce: request.RequestNonce,
		ExecutableSHA256: request.ExpectedExecutableSHA256, ExecutableSize: request.ExpectedExecutableSize,
		ManifestSHA256: FrozenManifestSHA256V4, PolicySHA256: contract.policySHA256,
		AuthoritySHA256: authority.SHA256, HostPlatform: "darwin", HostArch: authority.Arch,
		LoadedImageBound: true, WorkingDirectoryBound: true, GuardianAuthenticated: true, ProcessTreeEmpty: true,
	}
}

func frozenComponentByID(t *testing.T, id string) frozenComponentV1 {
	t.Helper()
	for _, contract := range frozenComponentsV1 {
		if contract.id == id {
			return contract
		}
	}
	t.Fatalf("unknown component %s", id)
	return frozenComponentV1{}
}

func syntheticPublicationMachO(t *testing.T) []byte {
	t.Helper()
	const (
		textOffset       = 32
		linkEditOffset   = textOffset + 72
		signatureCommand = linkEditOffset + 72
		payloadSize      = 256
		signatureSize    = 32
	)
	body := make([]byte, payloadSize+signatureSize)
	order := binary.LittleEndian
	order.PutUint32(body[0:4], 0xfeedfacf)
	cpu := uint32(0x0100000c)
	if runtime.GOARCH == "amd64" {
		cpu = 0x01000007
	}
	order.PutUint32(body[4:8], cpu)
	order.PutUint32(body[12:16], 2)
	order.PutUint32(body[16:20], 3)
	order.PutUint32(body[20:24], 72+72+16)
	writeSegment := func(offset int, name string, fileOffset, fileSize uint64, protection uint32) {
		order.PutUint32(body[offset:offset+4], 0x19)
		order.PutUint32(body[offset+4:offset+8], 72)
		copy(body[offset+8:offset+24], []byte(name))
		order.PutUint64(body[offset+32:offset+40], fileSize)
		order.PutUint64(body[offset+40:offset+48], fileOffset)
		order.PutUint64(body[offset+48:offset+56], fileSize)
		order.PutUint32(body[offset+60:offset+64], protection)
	}
	writeSegment(textOffset, "__TEXT", 0, payloadSize, 5)
	writeSegment(linkEditOffset, "__LINKEDIT", payloadSize, signatureSize, 1)
	order.PutUint32(body[signatureCommand:signatureCommand+4], 0x1d)
	order.PutUint32(body[signatureCommand+4:signatureCommand+8], 16)
	order.PutUint32(body[signatureCommand+8:signatureCommand+12], payloadSize)
	order.PutUint32(body[signatureCommand+12:signatureCommand+16], signatureSize)
	binary.BigEndian.PutUint32(body[payloadSize:payloadSize+4], 0xfade0cc0)
	binary.BigEndian.PutUint32(body[payloadSize+4:payloadSize+8], 12)
	return body
}

func assertPublicationMode(t *testing.T, path string, expected os.FileMode) {
	t.Helper()
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if stat.Mode().Perm() != expected {
		t.Fatalf("%s mode=%#o, want %#o", path, stat.Mode().Perm(), expected)
	}
}

func closeFiles(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

func stringsOfDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
