//go:build analytix_native_build_probe && !analytix_prod

package nativebuild_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	nativebuild "analytix.local/runtime-go/internal/domain/nativebuild"
)

func TestControlledCargoExecutionPermitIsExactAndSingleUse(t *testing.T) {
	receipt, live := controlledExecutionV1(t)
	body, err := nativebuild.CargoExecutionReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatalf("encode controlled receipt: %v", err)
	}
	parsed, err := nativebuild.ParseCargoExecutionReceiptV1(body)
	if err != nil {
		t.Fatalf("parse controlled receipt: %v", err)
	}
	if _, _, err := nativebuild.AuthorizeForPublicationV1(nativebuild.LiveCargoExecutionV1{}, parsed); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("parsed audit receipt restored authority: %v", err)
	}
	permit, binding, err := nativebuild.AuthorizeForPublicationV1(live, parsed)
	if err != nil {
		t.Fatalf("authorize controlled receipt: %v", err)
	}
	if err := nativebuild.ConsumeExactPublicationPermitV1(permit, binding, binding.Outputs); err != nil {
		t.Fatalf("consume exact permit: %v", err)
	}
	receiptDigest := sha256.Sum256(body)
	if binding.ExecutionID != receipt.ExecutionID || binding.ExecutionReceiptSHA256 != hex.EncodeToString(receiptDigest[:]) {
		t.Fatalf("publication binding did not retain exact execution receipt identity")
	}
	if err := nativebuild.ConsumeExactPublicationPermitV1(permit, binding, binding.Outputs); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("permit replay error = %v, want ineligible", err)
	}

	tampered := append([]nativebuild.CargoOutputV1(nil), binding.Outputs...)
	tampered[0].PayloadSHA256 = digestV1("tampered")
	if err := nativebuild.ConsumeExactPublicationPermitV1(permit, binding, tampered); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("mismatched output error = %v, want ineligible", err)
	}
}

func TestLocalCargoExecutionIsAuditableButCannotAuthorizePublication(t *testing.T) {
	receipt := receiptFixtureV1()
	receipt.Status = nativebuild.CargoStatusSucceeded
	receipt.TrustClass = nativebuild.CargoTrustLocalProvisional
	receipt.ReleaseEligible = false
	receipt.Blocker = "toolchain_user_writable"
	receipt.Toolchain.OwnershipClass = nativebuild.ToolchainOwnershipUserWritable
	receipt = finalizeReceiptV1(t, receipt)
	if err := nativebuild.ValidateCargoExecutionReceiptV1(receipt); err != nil {
		t.Fatalf("validate local audit receipt: %v", err)
	}
	if _, _, err := nativebuild.AuthorizeForPublicationV1(nativebuild.LiveCargoExecutionV1{}, receipt); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("local receipt authorization error = %v, want ineligible", err)
	}

	receipt.ReleaseEligible = true
	if err := nativebuild.ValidateCargoExecutionReceiptV1(receipt); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("caller-upgraded local receipt error = %v, want invalid", err)
	}
}

func TestCargoOutputBindsRawPayloadFormatAndTargetArchitecture(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*nativebuild.CargoOutputV1)
	}{
		{name: "schema version", mutate: func(value *nativebuild.CargoOutputV1) { value.SchemaVersion = 2 }},
		{name: "raw build digest", mutate: func(value *nativebuild.CargoOutputV1) { value.RawBuildSHA256 = "" }},
		{name: "raw build size", mutate: func(value *nativebuild.CargoOutputV1) { value.RawBuildSize = 0 }},
		{name: "payload digest", mutate: func(value *nativebuild.CargoOutputV1) { value.PayloadSHA256 = "" }},
		{name: "payload size", mutate: func(value *nativebuild.CargoOutputV1) { value.PayloadSize = value.RawBuildSize + 1 }},
		{name: "format", mutate: func(value *nativebuild.CargoOutputV1) { value.Format = "pe" }},
		{name: "target architecture", mutate: func(value *nativebuild.CargoOutputV1) { value.Arch = "x64" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			receipt := receiptFixtureV1()
			mutation.mutate(&receipt.Outputs[0])
			receipt.OutputInventorySHA256 = nativebuild.CargoOutputInventorySHA256V1(receipt.Outputs)
			if _, err := nativebuild.FinalizeCargoExecutionReceiptV1(receipt); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
				t.Fatalf("finalize mutated output error = %v, want invalid", err)
			}
		})
	}
}

func TestCargoOutputTargetMatrixIsClosed(t *testing.T) {
	targets := []struct {
		key, triple, format, arch string
	}{
		{key: "darwin-arm64", triple: "aarch64-apple-darwin", format: "mach-o", arch: "arm64"},
		{key: "darwin-x64", triple: "x86_64-apple-darwin", format: "mach-o", arch: "x64"},
		{key: "win32-x64", triple: "x86_64-pc-windows-msvc", format: "pe", arch: "x64"},
		{key: "linux-x64", triple: "x86_64-unknown-linux-gnu", format: "elf", arch: "x64"},
	}
	for _, target := range targets {
		t.Run(target.key, func(t *testing.T) {
			receipt := receiptFixtureV1()
			receipt.TargetKey, receipt.TargetTriple = target.key, target.triple
			receipt.Authority.TargetKey = target.key
			for index := range receipt.Outputs {
				receipt.Outputs[index].Format = target.format
				receipt.Outputs[index].Arch = target.arch
			}
			receipt.OutputInventorySHA256 = nativebuild.CargoOutputInventorySHA256V1(receipt.Outputs)
			if _, err := nativebuild.FinalizeCargoExecutionReceiptV1(receipt); err != nil {
				t.Fatalf("finalize target receipt: %v", err)
			}
		})
	}
}

func TestCargoExecutionEligibilityIsDerivedFromEveryContainmentFact(t *testing.T) {
	mutations := []struct {
		name    string
		blocker string
		mutate  func(*nativebuild.CargoExecutionReceiptV1)
	}{
		{name: "source descriptors", blocker: "source_snapshot_unbound", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.SourceDescriptorsHeld = false }},
		{name: "toolchain descriptors", blocker: "toolchain_identity_unbound", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.ToolchainDescriptorsHeld = false }},
		{name: "loaded images", blocker: "toolchain_identity_unbound", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.LoadedImagesBound = false }},
		{name: "working directories", blocker: "source_snapshot_unbound", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.WorkingDirectoriesBound = false }},
		{name: "write scope", blocker: "cargo_write_scope_unconfirmed", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.TargetWriteScopeBound = false }},
		{name: "network", blocker: "cargo_network_unconfirmed", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.NetworkDenied = false }},
		{name: "deadline", blocker: "cargo_execution_timeout", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.DeadlineBound = false }},
		{name: "process tree", blocker: "cargo_process_tree_unconfirmed", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.ProcessTreeEmpty = false }},
		{name: "output descriptors", blocker: "output_inventory_mismatch", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.OutputsDescriptorBound = false }},
		{name: "raw build descriptors", blocker: "output_inventory_mismatch", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.RawBuildDescriptorsHeld = false }},
		{name: "payload identity", blocker: "output_inventory_mismatch", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.PayloadIdentityBound = false }},
		{name: "cleanup", blocker: "build_cleanup_indeterminate", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.TemporaryStateDestroyed = false }},
		{name: "same uid mutation", blocker: "controlled_environment_unproven", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.SameUIDMutationDenied = false }},
		{name: "process containment", blocker: "cargo_process_tree_unconfirmed", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.ProcessContainmentBound = false }},
		{name: "dependency closure", blocker: "controlled_environment_unproven", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.DependencyClosureBound = false }},
		{name: "sdk closure", blocker: "controlled_environment_unproven", mutate: func(value *nativebuild.CargoExecutionReceiptV1) { value.Containment.SDKClosureBound = false }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			receipt := receiptFixtureV1()
			mutation.mutate(&receipt)
			receipt.ReleaseEligible = false
			receipt.Blocker = mutation.blocker
			receipt = finalizeReceiptV1(t, receipt)
			if _, _, err := nativebuild.AuthorizeForPublicationV1(nativebuild.LiveCargoExecutionV1{}, receipt); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
				t.Fatalf("authorization error = %v, want ineligible", err)
			}
		})
	}
}

func TestCargoExecutionFailureTimeoutCancelAndTamperingFailClosed(t *testing.T) {
	for _, status := range []nativebuild.CargoExecutionStatusV1{
		nativebuild.CargoStatusFailed,
		nativebuild.CargoStatusTimedOut,
		nativebuild.CargoStatusCancelled,
		nativebuild.CargoStatusBlocked,
	} {
		t.Run(string(status), func(t *testing.T) {
			receipt := receiptFixtureV1()
			receipt.Status = status
			receipt.ReleaseEligible = false
			receipt.Blocker = map[nativebuild.CargoExecutionStatusV1]string{
				nativebuild.CargoStatusFailed:    "cargo_execution_failed",
				nativebuild.CargoStatusTimedOut:  "cargo_execution_timeout",
				nativebuild.CargoStatusCancelled: "cargo_execution_cancelled",
				nativebuild.CargoStatusBlocked:   "controlled_environment_unproven",
			}[status]
			receipt = finalizeReceiptV1(t, receipt)
			if _, _, err := nativebuild.AuthorizeForPublicationV1(nativebuild.LiveCargoExecutionV1{}, receipt); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
				t.Fatalf("authorization error = %v, want ineligible", err)
			}
		})
	}

	receipt := controlledReceiptV1(t)
	tamperedID := receipt
	tamperedID.ExecutionID = digestV1("forged")
	if err := nativebuild.ValidateCargoExecutionReceiptV1(tamperedID); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("forged execution id error = %v, want invalid", err)
	}
	reordered := receipt
	reordered.Outputs = append([]nativebuild.CargoOutputV1(nil), receipt.Outputs...)
	reordered.Outputs[0], reordered.Outputs[1] = reordered.Outputs[1], reordered.Outputs[0]
	reordered.OutputInventorySHA256 = nativebuild.CargoOutputInventorySHA256V1(reordered.Outputs)
	reordered.ExecutionID = ""
	if _, err := nativebuild.FinalizeCargoExecutionReceiptV1(reordered); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("reordered outputs error = %v, want invalid", err)
	}
	unknownJSON := append([]byte(nil), mustReceiptBytesV1(t, receipt)...)
	unknownJSON = []byte(string(unknownJSON[:len(unknownJSON)-2]) + ",\n  \"callerReleaseEligible\": true\n}\n")
	if _, err := nativebuild.ParseCargoExecutionReceiptV1(unknownJSON); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("unknown field error = %v, want invalid", err)
	}
}

func TestZeroAndMismatchedPublicationPermitsFailClosed(t *testing.T) {
	receipt, live := controlledExecutionV1(t)
	_, binding, err := nativebuild.AuthorizeForPublicationV1(live, receipt)
	if err != nil {
		t.Fatalf("authorize fixture: %v", err)
	}
	if err := nativebuild.ConsumeExactPublicationPermitV1(nativebuild.PublicationPermitV1{}, binding, binding.Outputs); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("zero permit error = %v, want ineligible", err)
	}
	receipt, live = controlledExecutionV1(t)
	permit, _, err := nativebuild.AuthorizeForPublicationV1(live, receipt)
	if err != nil {
		t.Fatalf("authorize fixture for serialization check: %v", err)
	}
	body, err := json.Marshal(permit)
	if err != nil || string(body) != "{}" {
		t.Fatalf("permit JSON = %q err=%v, want empty object", body, err)
	}
	var decoded nativebuild.PublicationPermitV1
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode empty permit: %v", err)
	}
	if err := nativebuild.ConsumeExactPublicationPermitV1(decoded, binding, binding.Outputs); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("JSON-decoded permit error = %v, want ineligible", err)
	}
	receipt, live = controlledExecutionV1(t)
	permit, binding, err = nativebuild.AuthorizeForPublicationV1(live, receipt)
	if err != nil {
		t.Fatalf("authorize fixture: %v", err)
	}
	binding.SourceSetSHA256 = digestV1("other-source")
	if err := nativebuild.ConsumeExactPublicationPermitV1(permit, binding, binding.Outputs); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("mismatched binding error = %v, want ineligible", err)
	}
	binding.SourceSetSHA256 = receipt.SourceSetSHA256
	if err := nativebuild.ConsumeExactPublicationPermitV1(permit, binding, binding.Outputs); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("permit survived mismatched first presentation: %v", err)
	}
}

func TestLiveCargoExecutionCapabilityIsNonSerializableAndSingleUse(t *testing.T) {
	observer := nativebuild.BeginCargoExecutionObservationV1()
	receipt, live, err := nativebuild.FinalizeObservedCargoExecutionV1(observer, receiptFixtureV1())
	if err != nil {
		t.Fatalf("finalize observed execution: %v", err)
	}
	if _, _, err := nativebuild.FinalizeObservedCargoExecutionV1(observer, receiptFixtureV1()); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("observer replay error = %v, want invalid", err)
	}
	body, err := json.Marshal(live)
	if err != nil || string(body) != "{}" {
		t.Fatalf("live capability JSON = %q err=%v, want empty object", body, err)
	}
	var decoded nativebuild.LiveCargoExecutionV1
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode empty live capability: %v", err)
	}
	if _, _, err := nativebuild.AuthorizeForPublicationV1(decoded, receipt); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("decoded live capability authorized: %v", err)
	}
	if _, _, err := nativebuild.AuthorizeForPublicationV1(live, receipt); err != nil {
		t.Fatalf("authorize live capability: %v", err)
	}
	if _, _, err := nativebuild.AuthorizeForPublicationV1(live, receipt); !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
		t.Fatalf("live capability replay error = %v, want ineligible", err)
	}
}

func TestInvalidFirstSettlementPermanentlyBurnsObserver(t *testing.T) {
	observer := nativebuild.BeginCargoExecutionObservationV1()
	invalid := receiptFixtureV1()
	invalid.RequestNonce = "not-a-digest"
	if _, _, err := nativebuild.FinalizeObservedCargoExecutionV1(observer, invalid); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("invalid settlement error = %v, want invalid", err)
	}
	if _, _, err := nativebuild.FinalizeObservedCargoExecutionV1(observer, receiptFixtureV1()); !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
		t.Fatalf("observer accepted repair after invalid settlement: %v", err)
	}
}

func TestLiveAuthoritiesHaveExactlyOneConcurrentWinner(t *testing.T) {
	const attempts = 32

	observer := nativebuild.BeginCargoExecutionObservationV1()
	var finalizeWinners atomic.Int32
	var finalizeGroup sync.WaitGroup
	for range attempts {
		finalizeGroup.Add(1)
		go func() {
			defer finalizeGroup.Done()
			if _, _, err := nativebuild.FinalizeObservedCargoExecutionV1(observer, receiptFixtureV1()); err == nil {
				finalizeWinners.Add(1)
			} else if !errors.Is(err, nativebuild.ErrCargoExecutionInvalid) {
				t.Errorf("concurrent finalize error = %v", err)
			}
		}()
	}
	finalizeGroup.Wait()
	if got := finalizeWinners.Load(); got != 1 {
		t.Fatalf("finalize winners = %d, want 1", got)
	}

	receipt, live := controlledExecutionV1(t)
	type authorizationV1 struct {
		permit  nativebuild.PublicationPermitV1
		binding nativebuild.PublicationBindingV1
	}
	authorizations := make(chan authorizationV1, attempts)
	var authorizeWinners atomic.Int32
	var authorizeGroup sync.WaitGroup
	for range attempts {
		authorizeGroup.Add(1)
		go func() {
			defer authorizeGroup.Done()
			permit, binding, err := nativebuild.AuthorizeForPublicationV1(live, receipt)
			if err == nil {
				authorizeWinners.Add(1)
				authorizations <- authorizationV1{permit: permit, binding: binding}
			} else if !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
				t.Errorf("concurrent authorize error = %v", err)
			}
		}()
	}
	authorizeGroup.Wait()
	close(authorizations)
	if got := authorizeWinners.Load(); got != 1 {
		t.Fatalf("authorize winners = %d, want 1", got)
	}
	authorization := <-authorizations

	var consumeWinners atomic.Int32
	var consumeGroup sync.WaitGroup
	for range attempts {
		consumeGroup.Add(1)
		go func() {
			defer consumeGroup.Done()
			if err := nativebuild.ConsumeExactPublicationPermitV1(
				authorization.permit,
				authorization.binding,
				authorization.binding.Outputs,
			); err == nil {
				consumeWinners.Add(1)
			} else if !errors.Is(err, nativebuild.ErrCargoExecutionIneligible) {
				t.Errorf("concurrent consume error = %v", err)
			}
		}()
	}
	consumeGroup.Wait()
	if got := consumeWinners.Load(); got != 1 {
		t.Fatalf("consume winners = %d, want 1", got)
	}
}

func controlledReceiptV1(t *testing.T) nativebuild.CargoExecutionReceiptV1 {
	t.Helper()
	return finalizeReceiptV1(t, receiptFixtureV1())
}

func controlledExecutionV1(t *testing.T) (nativebuild.CargoExecutionReceiptV1, nativebuild.LiveCargoExecutionV1) {
	t.Helper()
	receipt, live, err := nativebuild.FinalizeObservedCargoExecutionV1(
		nativebuild.BeginCargoExecutionObservationV1(),
		receiptFixtureV1(),
	)
	if err != nil {
		t.Fatalf("finalize observed controlled execution: %v", err)
	}
	return receipt, live
}

func receiptFixtureV1() nativebuild.CargoExecutionReceiptV1 {
	outputs := make([]nativebuild.CargoOutputV1, 0, 4)
	for _, id := range []string{"import-accelerator", "cleaning-ops", "analysis-compute", "data-engine"} {
		outputs = append(outputs, nativebuild.CargoOutputV1{
			SchemaVersion: 1, ID: id, SourceDigest: digestV1(id + "-source"), CargoLockSHA256: digestV1(id + "-lock"),
			BuildEnvironmentSHA256: digestV1(id + "-environment"),
			RawBuildSHA256:         digestV1(id + "-binary"), RawBuildSize: 1024,
			PayloadSHA256: digestV1(id + "-payload"), PayloadSize: 768, Format: "mach-o", Arch: "arm64",
		})
	}
	return nativebuild.CargoExecutionReceiptV1{
		SchemaVersion: nativebuild.CargoExecutionSchemaVersionV1, Purpose: nativebuild.CargoExecutionPurposeV1,
		RequestNonce: digestV1("request-nonce"),
		Status:       nativebuild.CargoStatusSucceeded, TrustClass: nativebuild.CargoTrustControlledRelease,
		ReleaseEligible: true, TargetKey: "darwin-arm64", TargetTriple: "aarch64-apple-darwin",
		ManifestSHA256: digestV1("manifest"), SourceSetSHA256: digestV1("source-set"),
		SourceSnapshot: nativebuild.SourceSnapshotBindingV1{
			GenerationID: digestV1("snapshot-generation"), InventorySHA256: digestV1("snapshot-inventory"),
			GenerationReceiptSHA256: digestV1("snapshot-receipt"),
		},
		BuildPlanSHA256: digestV1("build-plan"), BuildEnvironmentSHA256: digestV1("build-environment"),
		Toolchain: nativebuild.CargoToolchainIdentityV1{
			CargoExecutableSHA256: digestV1("cargo"), CargoVersion: "cargo 1.94.1",
			RustcExecutableSHA256: digestV1("rustc"), RustcVersion: "rustc 1.94.1",
			TreeSHA256: digestV1("tree"), TreeFileCount: 42, CompilerSHA256: digestV1("compiler"),
			ArchiverSHA256: digestV1("archiver"), LinkerSHA256: digestV1("linker"), PlatformSignerSHA256: digestV1("signer"),
			SDKIdentitySHA256: digestV1("sdk"), OwnershipClass: nativebuild.ToolchainOwnershipImmutableRelease,
		},
		Containment: nativebuild.CargoContainmentV1{
			SourceDescriptorsHeld: true, ToolchainDescriptorsHeld: true, LoadedImagesBound: true,
			WorkingDirectoriesBound: true, TargetWriteScopeBound: true, NetworkDenied: true, DeadlineBound: true,
			ProcessTreeEmpty: true, OutputsDescriptorBound: true, RawBuildDescriptorsHeld: true, PayloadIdentityBound: true,
			TemporaryStateDestroyed: true,
			SameUIDMutationDenied:   true, ProcessContainmentBound: true, DependencyClosureBound: true, SDKClosureBound: true,
		},
		Outputs: outputs, OutputInventorySHA256: nativebuild.CargoOutputInventorySHA256V1(outputs),
		Authority: nativebuild.CargoExecutionAuthorityV1{
			Protocol: nativebuild.CargoExecutionAuthorityProtocol, TargetKey: "darwin-arm64",
			BinarySHA256: digestV1("authority-binary"), SourceSetSHA256: digestV1("authority-source"),
			BuildEnvironmentSHA256: digestV1("authority-environment"), GoExecutableSHA256: digestV1("go"),
		},
	}
}

func finalizeReceiptV1(t *testing.T, receipt nativebuild.CargoExecutionReceiptV1) nativebuild.CargoExecutionReceiptV1 {
	t.Helper()
	receipt.ExecutionID = ""
	finalized, err := nativebuild.FinalizeCargoExecutionReceiptV1(receipt)
	if err != nil {
		t.Fatalf("finalize receipt: %v", err)
	}
	return finalized
}

func mustReceiptBytesV1(t *testing.T, receipt nativebuild.CargoExecutionReceiptV1) []byte {
	t.Helper()
	body, err := nativebuild.CargoExecutionReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatalf("encode receipt: %v", err)
	}
	return body
}

func digestV1(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
