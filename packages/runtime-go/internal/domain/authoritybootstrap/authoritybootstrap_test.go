package authoritybootstrap

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type bootstrapFixture struct {
	label               string
	input               BootstrapBindingInputV1
	binding             BootstrapBindingV1
	anchored            AnchoredBootstrapBindingV1
	anchoredManifest    domainenrollment.AnchoredManifestV2
	manifest            domainenrollment.ManifestV2
	installationPrivate ed25519.PrivateKey
	installationPublic  ed25519.PublicKey
	witnessPrivate      ed25519.PrivateKey
	witnessPublic       ed25519.PublicKey
}

type bootstrapChain struct {
	unpreparedRequest     domainsecurity.MonotonicHeadObserveRequestV1
	unprepared            BootstrapObservationV1
	prepare               BootstrapPrepareReceiptV1
	preparedRequest       domainsecurity.MonotonicHeadObserveRequestV1
	prepared              BootstrapObservationV1
	cachedPreparedRequest domainsecurity.MonotonicHeadObserveRequestV1
	cachedPrepared        BootstrapObservationV1
	commit                BootstrapCommitReceiptV1
	committedRequest      domainsecurity.MonotonicHeadObserveRequestV1
	committed             BootstrapObservationV1
}

func TestBootstrapExactMonotonicChainV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "chain")
	chain := newBootstrapChain(t, fixture, "chain")

	if err := ValidatePrepareTransitionV1(
		fixture.anchored, chain.unprepared, chain.unpreparedRequest, chain.prepare,
	); err != nil {
		t.Fatalf("ValidatePrepareTransitionV1() error = %v", err)
	}
	if err := ValidateCommitTransitionV1(
		fixture.anchored, chain.prepared, chain.preparedRequest, chain.prepare, chain.commit,
	); err != nil {
		t.Fatalf("ValidateCommitTransitionV1() error = %v", err)
	}
	if err := ValidateCommittedObservationV1(
		chain.committed, fixture.anchored, chain.committedRequest, chain.prepare, chain.commit,
	); err != nil {
		t.Fatalf("ValidateCommittedObservationV1() error = %v", err)
	}

	initial := fixture.binding.BootstrapInitialCheckpoint
	prepared := chain.prepare.AdvanceReceipt.Checkpoint
	committed := chain.commit.AdvanceReceipt.Checkpoint
	if fixture.binding.EnrollmentInitialCheckpoint.Generation != 0 ||
		fixture.binding.FloorCheckpoint.Generation == 0 ||
		!equalCheckpointV1(fixture.binding.FloorCheckpoint, fixture.binding.FloorObservation.Checkpoint) ||
		initial.Generation != 0 || prepared.Generation != 1 || committed.Generation != 2 ||
		prepared.PreviousCheckpointDigest != initial.CheckpointDigest || committed.PreviousCheckpointDigest != prepared.CheckpointDigest ||
		chain.prepare.AdvanceRequest.ExpectedFenceNonce != initial.FenceNonce ||
		chain.commit.AdvanceRequest.ExpectedFenceNonce != prepared.FenceNonce ||
		chain.prepare.AdvanceReceipt.RequestDigest != chain.prepare.AdvanceRequest.RequestDigest ||
		chain.commit.AdvanceReceipt.RequestDigest != chain.commit.AdvanceRequest.RequestDigest {
		t.Fatal("bootstrap receipts do not preserve exact checkpoint/request/fence lineage")
	}
}

func TestBootstrapOpaqueAnchorAndKeyRolesV1(t *testing.T) {
	trusted := newBootstrapFixture(t, "trusted")
	attacker := newBootstrapFixture(t, "attacker")
	if err := ValidateBootstrapBindingV1(attacker.binding); err != nil {
		t.Fatalf("self-authentic binding error = %v", err)
	}
	if err := ValidateBootstrapBindingExactV1(
		attacker.binding,
		attacker.input,
		attacker.binding.CurrentManifestDigest,
		attacker.binding.ManifestEnrollmentDigest,
	); err != nil {
		t.Fatalf("raw exact equality must remain non-authoritative data validation: %v", err)
	}
	if _, err := AnchorBootstrapBindingV1(
		attacker.binding, trusted.anchoredManifest, attacker.input,
	); !errors.Is(err, ErrAnchorMismatch) {
		t.Fatalf("attacker binding exact anchor error = %v", err)
	}
	if _, err := NewBootstrapBindingV1(
		attacker.input,
		domainenrollment.AnchoredManifestV2{},
		installationSignerV1(attacker.installationPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("zero manifest capability binding error = %v", err)
	}
	request, observation := newMonotonicObservation(t, trusted, trusted.binding.BootstrapInitialCheckpoint, "zero-anchor")
	if _, err := NewUnpreparedObservationV1(
		AnchoredBootstrapBindingV1{}, request, observation, witnessSignerV1(trusted.witnessPrivate),
	); !errors.Is(err, ErrAnchorMismatch) {
		t.Fatalf("zero anchored binding error = %v", err)
	}

	sameKeyInput := trusted.input
	sameKeyInput.InstallationAuthorityKeyID = trusted.input.WitnessKeyID
	sameKeyInput.InstallationAuthorityPublicKey = append([]byte(nil), trusted.input.WitnessPublicKey...)
	if _, err := NewBootstrapBindingV1(
		sameKeyInput,
		trusted.anchoredManifest,
		installationSignerV1(trusted.witnessPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("installation/witness role collapse error = %v", err)
	}
	aliasInput := trusted.input
	aliasInput.EnrollmentID = trusted.binding.BootstrapEnrollmentID
	if _, err := NewBootstrapBindingV1(
		aliasInput,
		trusted.anchoredManifest,
		installationSignerV1(trusted.installationPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("source/bootstrap enrollment alias error = %v", err)
	}
	tampered := trusted.binding
	tampered.EnrollmentID = tampered.BootstrapEnrollmentID
	resignBindingV1(&tampered, trusted.installationPrivate)
	if err := ValidateBootstrapBindingV1(tampered); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("re-signed source/bootstrap enrollment alias error = %v", err)
	}
}

func TestRejectOldBindingUnderRotatedManifestSameEnrollmentV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "rotated-manifest")
	rotatedPublic, rotatedPrivate := deterministicKeyV1("rotated-installation-authority")
	rotatedManifest, err := domainenrollment.NewManifestV2(domainenrollment.ManifestInputV2{
		InstallationID:                 fixture.binding.InstallationID,
		InstallationAuthorityKeyID:     domainsecurity.SHA256Hex(rotatedPublic),
		InstallationAuthorityPublicKey: rotatedPublic,
		CredentialProfileGeneration:    fixture.manifest.CredentialProfileGeneration,
		CredentialProfileDigest:        fixture.manifest.CredentialProfileDigest,
		IssuedAt:                       fixture.manifest.IssuedAt.Add(time.Minute),
		ThreadRisk:                     manifestEnrollmentInputV1(t, fixture.manifest.ThreadRisk),
		SharedEvidence:                 manifestEnrollmentInputV1(t, fixture.manifest.SharedEvidence),
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(rotatedPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rotatedManifest.ThreadRisk != fixture.manifest.ThreadRisk ||
		rotatedManifest.SharedEvidence != fixture.manifest.SharedEvidence ||
		rotatedManifest.ManifestDigest == fixture.manifest.ManifestDigest ||
		rotatedManifest.InstallationAuthorityKeyID == fixture.manifest.InstallationAuthorityKeyID {
		t.Fatal("rotated-manifest fixture did not preserve enrollment while rotating exact authority")
	}
	rotatedAnchor, err := domainenrollment.AnchorManifestForInstallationV2(
		rotatedManifest,
		fixture.binding.InstallationID,
		domainsecurity.SHA256Hex(rotatedPublic),
		rotatedPublic,
		rotatedManifest.ManifestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewBootstrapBindingV1(
		fixture.input,
		rotatedAnchor,
		installationSignerV1(fixture.installationPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("constructor accepted old authority under rotated manifest: %v", err)
	}
	if _, err := AnchorBootstrapBindingV1(
		fixture.binding,
		rotatedAnchor,
		fixture.input,
	); !errors.Is(err, ErrAnchorMismatch) {
		t.Fatalf("anchor accepted old binding under rotated manifest: %v", err)
	}

	rotatedInput := fixture.input
	rotatedInput.InstallationAuthorityKeyID = domainsecurity.SHA256Hex(rotatedPublic)
	rotatedInput.InstallationAuthorityPublicKey = rotatedPublic
	rotatedInput.FloorObserveRequest, rotatedInput.FloorObservation = newAuthorityObservationPairV1(
		t,
		fixture.binding.InstallationID,
		fixture.binding.EnrollmentID,
		fixture.binding.Namespace,
		fixture.binding.FloorCheckpoint,
		rotatedPublic,
		rotatedPrivate,
		fixture.witnessPrivate,
		"rotated-floor",
	)
	rotatedInput.BootstrapInitialObserveRequest, rotatedInput.BootstrapInitialObservation = newAuthorityObservationPairV1(
		t,
		fixture.binding.InstallationID,
		fixture.binding.BootstrapEnrollmentID,
		fixture.binding.Namespace,
		fixture.binding.BootstrapInitialCheckpoint,
		rotatedPublic,
		rotatedPrivate,
		fixture.witnessPrivate,
		"rotated-bootstrap-initial",
	)
	rotatedBinding, err := NewBootstrapBindingV1(
		rotatedInput,
		rotatedAnchor,
		installationSignerV1(rotatedPrivate),
	)
	if err != nil {
		t.Fatalf("rotated authority exact binding error = %v", err)
	}
	if _, err := AnchorBootstrapBindingV1(rotatedBinding, rotatedAnchor, rotatedInput); err != nil {
		t.Fatalf("rotated authority exact anchor error = %v", err)
	}
}

func TestBootstrapReceiptRequiresExactSignedRequestAndFenceV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "fence")
	request, monotonicObservation := newMonotonicObservation(
		t, fixture, fixture.binding.BootstrapInitialCheckpoint, "fence-unprepared",
	)
	unprepared, err := NewUnpreparedObservationV1(
		fixture.anchored, request, monotonicObservation, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	mutationID := digestV1("fence-prepare-mutation")
	nextState, err := PreparedStateDigestV1(fixture.anchored, mutationID)
	if err != nil {
		t.Fatal(err)
	}
	wrongFenceRequest := newAdvanceRequest(t, fixture, fixture.binding.BootstrapInitialCheckpoint, nextState, mutationID, digestV1("wrong-fence"))
	wrongFenceReceipt := newMonotonicAdvanceReceipt(t, fixture, fixture.binding.BootstrapInitialCheckpoint, wrongFenceRequest, "wrong-fence-next")
	if _, err := NewPrepareReceiptV1(
		fixture.anchored, unprepared, wrongFenceRequest, wrongFenceReceipt, witnessSignerV1(fixture.witnessPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("wrong-fence prepare error = %v", err)
	}

	validRequest := newAdvanceRequest(
		t, fixture, fixture.binding.BootstrapInitialCheckpoint, nextState, mutationID,
		fixture.binding.BootstrapInitialCheckpoint.FenceNonce,
	)
	validReceipt := newMonotonicAdvanceReceipt(t, fixture, fixture.binding.BootstrapInitialCheckpoint, validRequest, "valid-fence-next")
	prepare, err := NewPrepareReceiptV1(
		fixture.anchored, unprepared, validRequest, validReceipt, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateBootstrapPrepareReceiptForBindingV1(prepare, fixture.anchored); err != nil {
		t.Fatalf("valid anchored prepare error = %v", err)
	}
	wrongPreviousRequest := newAdvanceRequestWithExpectedCheckpoint(
		t,
		fixture,
		fixture.binding.BootstrapInitialCheckpoint,
		nextState,
		mutationID,
		fixture.binding.BootstrapInitialCheckpoint.FenceNonce,
		digestV1("wrong-previous-checkpoint"),
	)
	wrongPreviousReceipt := newMonotonicAdvanceReceipt(
		t, fixture, fixture.binding.BootstrapInitialCheckpoint, wrongPreviousRequest, "wrong-previous-next",
	)
	if err := domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(wrongPreviousRequest); err != nil {
		t.Fatalf("fully signed wrong-previous request is not independently valid: %v", err)
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceReceiptV1(wrongPreviousReceipt); err != nil {
		t.Fatalf("fully signed wrong-previous receipt is not independently valid: %v", err)
	}
	if _, err := NewPrepareReceiptV1(
		fixture.anchored,
		unprepared,
		wrongPreviousRequest,
		wrongPreviousReceipt,
		witnessSignerV1(fixture.witnessPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("wrong-previous prepare error = %v", err)
	}
}

func TestBootstrapFloorGenerationZeroMustEqualManifestEnrollmentV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "floor-genesis")
	forkedGenesis, err := domainsecurity.NewMonotonicHeadCheckpointV1(
		domainsecurity.MonotonicHeadCheckpointInputV1{
			InstallationID:     fixture.binding.InstallationID,
			EnrollmentID:       fixture.binding.EnrollmentID,
			Namespace:          fixture.binding.Namespace,
			Generation:         0,
			CurrentStateDigest: digestV1("forked-floor-genesis-state"),
			FenceNonce:         digestV1("forked-floor-genesis-fence"),
			WitnessKeyID:       fixture.binding.WitnessKeyID,
			WitnessPublicKey:   fixture.witnessPublic,
		},
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	forkedObservation, err := domainsecurity.NewMonotonicHeadObservationV1(
		fixture.input.FloorObserveRequest,
		forkedGenesis,
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.input
	input.FloorObservation = forkedObservation
	if _, err := NewBootstrapBindingV1(
		input,
		fixture.anchoredManifest,
		installationSignerV1(fixture.installationPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("forked generation-zero floor error = %v", err)
	}
}

func TestBootstrapInitialCheckpointIsStablePerProjectionSlotV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "stable-bootstrap-genesis")
	forkedInitial, err := domainsecurity.NewMonotonicHeadCheckpointV1(
		domainsecurity.MonotonicHeadCheckpointInputV1{
			InstallationID:     fixture.binding.InstallationID,
			EnrollmentID:       fixture.binding.BootstrapEnrollmentID,
			Namespace:          fixture.binding.Namespace,
			Generation:         0,
			CurrentStateDigest: fixture.binding.BootstrapInitialCheckpoint.CurrentStateDigest,
			FenceNonce:         digestV1("forked-bootstrap-initial-fence"),
			WitnessKeyID:       fixture.binding.WitnessKeyID,
			WitnessPublicKey:   fixture.witnessPublic,
		},
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	forkedObservation, err := domainsecurity.NewMonotonicHeadObservationV1(
		fixture.input.BootstrapInitialObserveRequest,
		forkedInitial,
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.input
	input.BootstrapInitialObservation = forkedObservation
	if _, err := NewBootstrapBindingV1(
		input,
		fixture.anchoredManifest,
		installationSignerV1(fixture.installationPrivate),
	); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("forked stable bootstrap genesis error = %v", err)
	}
}

func TestCachedPreparedObservationAfterCommitCannotYieldAuthorityV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "revalidate")
	chain := newBootstrapChain(t, fixture, "revalidate")
	requirement, err := NewPreparedRecoveryRequirementV1(
		fixture.anchored, chain.prepared, chain.preparedRequest, chain.prepare,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSameHeadObservationV1(
		fixture.anchored, chain.prepared, chain.cachedPrepared, chain.cachedPreparedRequest,
	); err != nil {
		t.Fatalf("cached observation must remain structurally authentic to exercise the stale replay threat: %v", err)
	}
	condition, err := requirement.ProjectionConditionV1()
	if err != nil {
		t.Fatal(err)
	}
	assertProjectionCondition(t, condition, fixture, PhasePrepared, chain.prepare.AdvanceReceipt.Checkpoint)
	methods := reflect.TypeOf(requirement)
	if methods.NumMethod() != 1 || methods.Method(0).Name != "ProjectionConditionV1" {
		t.Fatalf("prepared requirement exposes authority-like methods after cached replay: %v", exportedMethodNamesV1(methods))
	}
	for _, forbidden := range []string{"Fresh", "Authorize", "Target", "Ready", "Permit", "Write"} {
		if strings.Contains(methods.Method(0).Name, forbidden) {
			t.Fatalf("prepared requirement method %q carries forbidden authority semantics", methods.Method(0).Name)
		}
	}
	if _, err := (PreparedRecoveryRequirementV1{}).ProjectionConditionV1(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("zero prepared requirement error = %v", err)
	}
}

func TestCommittedThenDistinctChallengeUnpreparedRejectedV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "aba")
	chain := newBootstrapChain(t, fixture, "aba")
	requirement, err := NewCommittedFloorRequirementV1(
		fixture.anchored, chain.committed, chain.committedRequest, chain.prepare, chain.commit,
	)
	if err != nil {
		t.Fatal(err)
	}
	condition, err := requirement.ProjectionConditionV1()
	if err != nil {
		t.Fatalf("committed condition error = %v", err)
	}
	assertProjectionCondition(t, condition, fixture, PhaseCommitted, chain.commit.AdvanceReceipt.Checkpoint)

	resetRequest, resetMonotonic := newMonotonicObservation(
		t, fixture, fixture.binding.BootstrapInitialCheckpoint, "aba-reset-unprepared",
	)
	resetUnprepared, err := NewUnpreparedObservationV1(
		fixture.anchored, resetRequest, resetMonotonic, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSameHeadObservationV1(
		fixture.anchored, chain.committed, resetUnprepared, resetRequest,
	); !errors.Is(err, ErrTransitionConflict) {
		t.Fatalf("committed -> distinct-challenge unprepared ABA error = %v", err)
	}
	if _, err := (CommittedFloorRequirementV1{}).ProjectionConditionV1(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("zero committed requirement error = %v", err)
	}
}

func TestBootstrapStrictCanonicalContractsV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "canonical")
	chain := newBootstrapChain(t, fixture, "canonical")
	tests := []struct {
		name  string
		body  func() ([]byte, error)
		parse func([]byte) error
	}{
		{name: "binding", body: func() ([]byte, error) { return BootstrapBindingV1Bytes(fixture.binding) }, parse: func(body []byte) error { _, err := ParseBootstrapBindingV1(body); return err }},
		{name: "observation", body: func() ([]byte, error) { return BootstrapObservationV1Bytes(chain.committed) }, parse: func(body []byte) error { _, err := ParseBootstrapObservationV1(body); return err }},
		{name: "prepare", body: func() ([]byte, error) { return BootstrapPrepareReceiptV1Bytes(chain.prepare) }, parse: func(body []byte) error { _, err := ParseBootstrapPrepareReceiptV1(body); return err }},
		{name: "commit", body: func() ([]byte, error) { return BootstrapCommitReceiptV1Bytes(chain.commit) }, parse: func(body []byte) error { _, err := ParseBootstrapCommitReceiptV1(body); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := test.body()
			if err != nil || test.parse(body) != nil {
				t.Fatalf("canonical round trip = %v, %v", err, test.parse(body))
			}
			invalid := [][]byte{
				bytes.Replace(body, []byte(`{"schemaVersion":1,`), []byte(`{`), 1),
				bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":null`), 1),
				append([]byte(" "), body...),
				append(append([]byte(nil), body...), []byte(`{}`)...),
				append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"unknown":true}`)...),
				bytes.Replace(body, []byte(`{"schemaVersion":1`), []byte(`{"schemaVersion":1,"schemaVersion":1`), 1),
				bytes.Replace(body, []byte(`{"schemaVersion":1`), []byte(`{"schemaVersion":1,"SchemaVersion":1`), 1),
			}
			for index, candidate := range invalid {
				if err := test.parse(candidate); !errors.Is(err, ErrInvalidContract) {
					t.Fatalf("invalid[%d] error = %v", index, err)
				}
			}
		})
	}

	bindingBody, _ := BootstrapBindingV1Bytes(fixture.binding)
	nestedUnknown := bytes.Replace(bindingBody, []byte(`"floorCheckpoint":{`), []byte(`"floorCheckpoint":{"unknown":true,`), 1)
	if _, err := ParseBootstrapBindingV1(nestedUnknown); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nested unknown field error = %v", err)
	}
	nestedDuplicate := bytes.Replace(
		bindingBody,
		[]byte(`"floorCheckpoint":{"schemaVersion":1`),
		[]byte(`"floorCheckpoint":{"schemaVersion":1,"schemaVersion":1`),
		1,
	)
	if _, err := ParseBootstrapBindingV1(nestedDuplicate); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nested duplicate field error = %v", err)
	}
	observationBody, _ := BootstrapObservationV1Bytes(chain.unprepared)
	phaseBool := bytes.Replace(observationBody, []byte(`"phase":"unprepared"`), []byte(`"phase":true`), 1)
	if _, err := ParseBootstrapObservationV1(phaseBool); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("raw phase bool error = %v", err)
	}
}

func TestBootstrapExactReplayRequiresAnchoredRequestV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "replay")
	chain := newBootstrapChain(t, fixture, "replay")
	if err := ValidatePrepareReceiptExactReplayV1(fixture.anchored, chain.prepare, chain.prepare); err != nil {
		t.Fatalf("exact prepare replay error = %v", err)
	}
	if err := ValidateCommitReceiptExactReplayV1(fixture.anchored, chain.prepare, chain.commit, chain.commit); err != nil {
		t.Fatalf("exact commit replay error = %v", err)
	}
	other := newBootstrapFixture(t, "replay-other")
	otherChain := newBootstrapChainWithMutations(
		t, other, "replay-other", chain.prepare.AdvanceRequest.MutationID, chain.commit.AdvanceRequest.MutationID,
	)
	if err := ValidatePrepareReceiptExactReplayV1(fixture.anchored, chain.prepare, otherChain.prepare); !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("cross-anchor prepare replay error = %v", err)
	}
	if err := ValidateCommitReceiptExactReplayV1(fixture.anchored, chain.prepare, chain.commit, otherChain.commit); !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("cross-anchor commit replay error = %v", err)
	}
}

func TestBootstrapOuterDomainsAndTamperRejectionV1(t *testing.T) {
	fixture := newBootstrapFixture(t, "domains")
	chain := newBootstrapChain(t, fixture, "domains")
	messages := [][]byte{
		BootstrapBindingSigningBytesV1(fixture.binding),
		BootstrapObservationSigningBytesV1(chain.committed),
		BootstrapPrepareReceiptSigningBytesV1(chain.prepare),
		BootstrapCommitReceiptSigningBytesV1(chain.commit),
	}
	for left := range messages {
		for right := left + 1; right < len(messages); right++ {
			if bytes.Equal(messages[left], messages[right]) {
				t.Fatalf("outer signing domains %d and %d collided", left, right)
			}
		}
	}
	prepareSignature := decodeSignatureV1(t, chain.prepare.WitnessSignature)
	if ed25519.Verify(fixture.witnessPublic, BootstrapCommitReceiptSigningBytesV1(chain.commit), prepareSignature) {
		t.Fatal("prepare signature crossed commit domain")
	}
	tampered := chain.commit
	tampered.WitnessSignature = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if err := ValidateBootstrapCommitReceiptV1(tampered); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("outer signature tamper error = %v", err)
	}
}

func newBootstrapFixture(t *testing.T, label string) bootstrapFixture {
	t.Helper()
	installationPublic, installationPrivate := deterministicKeyV1("installation:" + label)
	witnessPublic, witnessPrivate := deterministicKeyV1("witness:" + label)
	sharedWitnessPublic, sharedWitnessPrivate := deterministicKeyV1("shared-witness:" + label)
	installationID := digestV1("installation-id:" + label)
	enrollmentID := digestV1("enrollment-id:" + label)
	namespace := domainsecurity.ThreadRiskAuthorityNamespaceV1
	enrollmentInitial, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:     installationID,
		EnrollmentID:       enrollmentID,
		Namespace:          namespace,
		Generation:         0,
		CurrentStateDigest: digestV1("authority-initial-state:" + label),
		FenceNonce:         digestV1("authority-initial-fence:" + label),
		WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublic),
		WitnessPublicKey:   witnessPublic,
	}, domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(witnessPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	floor, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           installationID,
		EnrollmentID:             enrollmentID,
		Namespace:                namespace,
		Generation:               3,
		CurrentStateDigest:       digestV1("authority-current-floor-state:" + label),
		PreviousStateDigest:      digestV1("authority-previous-floor-state:" + label),
		PreviousCheckpointDigest: digestV1("authority-previous-floor-checkpoint:" + label),
		FenceNonce:               digestV1("authority-current-floor-fence:" + label),
		MutationID:               digestV1("authority-current-floor-mutation:" + label),
		WitnessKeyID:             domainsecurity.SHA256Hex(witnessPublic),
		WitnessPublicKey:         witnessPublic,
	}, domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(witnessPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	floorObserveRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
		domainsecurity.MonotonicHeadObserveRequestInputV1{
			InstallationID:     installationID,
			EnrollmentID:       enrollmentID,
			Namespace:          namespace,
			ChallengeNonce:     digestV1("authority-current-floor-observe:" + label),
			AuthorityKeyID:     domainsecurity.SHA256Hex(installationPublic),
			AuthorityPublicKey: installationPublic,
		},
		domainsecurity.MonotonicHeadSignFunc(installationSignerV1(installationPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	floorObservation, err := domainsecurity.NewMonotonicHeadObservationV1(
		floorObserveRequest,
		floor,
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	sharedEnrollmentID := digestV1("shared-enrollment-id:" + label)
	sharedInitial, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:     installationID,
		EnrollmentID:       sharedEnrollmentID,
		Namespace:          domainsecurity.EvidenceRegistryAuthorityNamespaceV1,
		Generation:         0,
		CurrentStateDigest: digestV1("shared-initial-state:" + label),
		FenceNonce:         digestV1("shared-initial-fence:" + label),
		WitnessKeyID:       domainsecurity.SHA256Hex(sharedWitnessPublic),
		WitnessPublicKey:   sharedWitnessPublic,
	}, domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(sharedWitnessPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := domainenrollment.NewManifestV2(domainenrollment.ManifestInputV2{
		InstallationID:                 installationID,
		InstallationAuthorityKeyID:     domainsecurity.SHA256Hex(installationPublic),
		InstallationAuthorityPublicKey: installationPublic,
		CredentialProfileGeneration:    1,
		CredentialProfileDigest:        digestV1("credential-profile:" + label),
		IssuedAt:                       time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		ThreadRisk: domainenrollment.WitnessEnrollmentInputV1{
			EnrollmentID:      enrollmentID,
			EndpointOrigin:    "https://risk-witness.example.test",
			WitnessKeyID:      domainsecurity.SHA256Hex(witnessPublic),
			WitnessPublicKey:  witnessPublic,
			RootCASHA256:      digestV1("risk-root-ca:" + label),
			ServerName:        "risk-witness.example.test",
			TimeoutMS:         5_000,
			InitialCheckpoint: enrollmentInitial,
		},
		SharedEvidence: domainenrollment.WitnessEnrollmentInputV1{
			EnrollmentID:      sharedEnrollmentID,
			EndpointOrigin:    "https://evidence-witness.example.test",
			WitnessKeyID:      domainsecurity.SHA256Hex(sharedWitnessPublic),
			WitnessPublicKey:  sharedWitnessPublic,
			RootCASHA256:      digestV1("shared-root-ca:" + label),
			ServerName:        "evidence-witness.example.test",
			TimeoutMS:         5_000,
			InitialCheckpoint: sharedInitial,
		},
	}, domainenrollment.ManifestSignFuncV2(installationSignerV1(installationPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	anchoredManifest, err := domainenrollment.AnchorManifestForInstallationV2(
		manifest,
		installationID,
		domainsecurity.SHA256Hex(installationPublic),
		installationPublic,
		manifest.ManifestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	projectionSlotID := projectionSlotIDV1(installationID, namespace)
	bootstrapEnrollmentID := bootstrapEnrollmentIDV1(installationID, namespace, projectionSlotID)
	bootstrapState := unpreparedStateDigestV1(BootstrapBindingV1{
		BootstrapEnrollmentID: bootstrapEnrollmentID,
		ProjectionSlotID:      projectionSlotID,
	})
	bootstrapFence := bootstrapInitialFenceNonceV1(BootstrapBindingV1{
		BootstrapEnrollmentID: bootstrapEnrollmentID,
		ProjectionSlotID:      projectionSlotID,
	})
	bootstrapInitialCheckpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(
		domainsecurity.MonotonicHeadCheckpointInputV1{
			InstallationID:     installationID,
			EnrollmentID:       bootstrapEnrollmentID,
			Namespace:          namespace,
			Generation:         0,
			CurrentStateDigest: bootstrapState,
			FenceNonce:         bootstrapFence,
			WitnessKeyID:       domainsecurity.SHA256Hex(witnessPublic),
			WitnessPublicKey:   witnessPublic,
		},
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapInitialObserveRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
		domainsecurity.MonotonicHeadObserveRequestInputV1{
			InstallationID:     installationID,
			EnrollmentID:       bootstrapEnrollmentID,
			Namespace:          namespace,
			ChallengeNonce:     digestV1("bootstrap-initial-observe:" + label),
			AuthorityKeyID:     domainsecurity.SHA256Hex(installationPublic),
			AuthorityPublicKey: installationPublic,
		},
		domainsecurity.MonotonicHeadSignFunc(installationSignerV1(installationPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapInitialObservation, err := domainsecurity.NewMonotonicHeadObservationV1(
		bootstrapInitialObserveRequest,
		bootstrapInitialCheckpoint,
		domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	input := BootstrapBindingInputV1{
		InstallationID:                 installationID,
		Namespace:                      namespace,
		EnrollmentID:                   enrollmentID,
		WitnessKeyID:                   domainsecurity.SHA256Hex(witnessPublic),
		WitnessPublicKey:               witnessPublic,
		EnrollmentInitialCheckpoint:    enrollmentInitial,
		FloorObserveRequest:            floorObserveRequest,
		FloorObservation:               floorObservation,
		BootstrapInitialObserveRequest: bootstrapInitialObserveRequest,
		BootstrapInitialObservation:    bootstrapInitialObservation,
		InstallationAuthorityKeyID:     domainsecurity.SHA256Hex(installationPublic),
		InstallationAuthorityPublicKey: installationPublic,
	}
	binding, err := NewBootstrapBindingV1(
		input,
		anchoredManifest,
		installationSignerV1(installationPrivate),
	)
	if err != nil {
		t.Fatalf("NewBootstrapBindingV1() error = %v", err)
	}
	anchored, err := AnchorBootstrapBindingV1(binding, anchoredManifest, input)
	if err != nil {
		t.Fatalf("AnchorBootstrapBindingV1() error = %v", err)
	}
	return bootstrapFixture{
		label: label, input: input, binding: binding, anchored: anchored, anchoredManifest: anchoredManifest,
		manifest:            manifest,
		installationPrivate: installationPrivate, installationPublic: installationPublic,
		witnessPrivate: witnessPrivate, witnessPublic: witnessPublic,
	}
}

func newBootstrapChain(t *testing.T, fixture bootstrapFixture, label string) bootstrapChain {
	t.Helper()
	return newBootstrapChainWithMutations(
		t, fixture, label, digestV1("prepare-mutation:"+label), digestV1("commit-mutation:"+label),
	)
}

func newBootstrapChainWithMutations(
	t *testing.T,
	fixture bootstrapFixture,
	label, prepareMutationID, commitMutationID string,
) bootstrapChain {
	t.Helper()
	unpreparedRequest, unpreparedMonotonic := newMonotonicObservation(
		t, fixture, fixture.binding.BootstrapInitialCheckpoint, label+":unprepared",
	)
	unprepared, err := NewUnpreparedObservationV1(
		fixture.anchored, unpreparedRequest, unpreparedMonotonic, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	preparedState, err := PreparedStateDigestV1(fixture.anchored, prepareMutationID)
	if err != nil {
		t.Fatal(err)
	}
	prepareRequest, prepareMonotonicReceipt := newAdvance(
		t, fixture, fixture.binding.BootstrapInitialCheckpoint, preparedState, prepareMutationID, label+":prepare",
	)
	prepare, err := NewPrepareReceiptV1(
		fixture.anchored, unprepared, prepareRequest, prepareMonotonicReceipt, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	preparedRequest, preparedMonotonic := newMonotonicObservation(
		t, fixture, prepare.AdvanceReceipt.Checkpoint, label+":prepared",
	)
	prepared, err := NewPreparedObservationV1(
		fixture.anchored, preparedRequest, preparedMonotonic, prepare, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	cachedPreparedRequest, cachedPreparedMonotonic := newMonotonicObservation(
		t, fixture, prepare.AdvanceReceipt.Checkpoint, label+":prepared-cached-before-commit",
	)
	cachedPrepared, err := NewPreparedObservationV1(
		fixture.anchored,
		cachedPreparedRequest,
		cachedPreparedMonotonic,
		prepare,
		witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	committedState, err := CommittedStateDigestV1(fixture.anchored, prepare, commitMutationID)
	if err != nil {
		t.Fatal(err)
	}
	commitRequest, commitMonotonicReceipt := newAdvance(
		t, fixture, prepare.AdvanceReceipt.Checkpoint, committedState, commitMutationID, label+":commit",
	)
	commit, err := NewCommitReceiptV1(
		fixture.anchored, prepared, prepare, commitRequest, commitMonotonicReceipt, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	committedRequest, committedMonotonic := newMonotonicObservation(
		t, fixture, commit.AdvanceReceipt.Checkpoint, label+":committed",
	)
	committed, err := NewCommittedObservationV1(
		fixture.anchored, committedRequest, committedMonotonic, prepare, commit, witnessSignerV1(fixture.witnessPrivate),
	)
	if err != nil {
		t.Fatal(err)
	}
	return bootstrapChain{
		unpreparedRequest: unpreparedRequest, unprepared: unprepared, prepare: prepare,
		preparedRequest: preparedRequest, prepared: prepared,
		cachedPreparedRequest: cachedPreparedRequest, cachedPrepared: cachedPrepared,
		commit:           commit,
		committedRequest: committedRequest, committed: committed,
	}
}

func newMonotonicObservation(
	t *testing.T,
	fixture bootstrapFixture,
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
	label string,
) (domainsecurity.MonotonicHeadObserveRequestV1, domainsecurity.MonotonicHeadObservationV1) {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID:     fixture.binding.InstallationID,
		EnrollmentID:       fixture.binding.BootstrapEnrollmentID,
		Namespace:          fixture.binding.Namespace,
		ChallengeNonce:     digestV1("observe-challenge:" + label),
		AuthorityKeyID:     fixture.binding.InstallationAuthorityKeyID,
		AuthorityPublicKey: fixture.installationPublic,
	}, domainsecurity.MonotonicHeadSignFunc(installationSignerV1(fixture.installationPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request, checkpoint, domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, observation
}

func newAdvance(
	t *testing.T,
	fixture bootstrapFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	nextState, mutationID, label string,
) (domainsecurity.MonotonicHeadAdvanceRequestV1, domainsecurity.MonotonicHeadAdvanceReceiptV1) {
	t.Helper()
	request := newAdvanceRequest(t, fixture, previous, nextState, mutationID, previous.FenceNonce)
	receipt := newMonotonicAdvanceReceipt(t, fixture, previous, request, label)
	if err := domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
		previous, request, receipt, fixture.binding.InstallationID, fixture.binding.InstallationAuthorityKeyID,
		fixture.installationPublic, fixture.binding.BootstrapEnrollmentID, fixture.binding.WitnessKeyID, fixture.witnessPublic,
	); err != nil {
		t.Fatalf("monotonic advance error = %v", err)
	}
	return request, receipt
}

func newAdvanceRequest(
	t *testing.T,
	fixture bootstrapFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	nextState, mutationID, expectedFence string,
) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	return newAdvanceRequestWithExpectedCheckpoint(
		t, fixture, previous, nextState, mutationID, expectedFence, previous.CheckpointDigest,
	)
}

func newAdvanceRequestWithExpectedCheckpoint(
	t *testing.T,
	fixture bootstrapFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	nextState, mutationID, expectedFence, expectedCheckpointDigest string,
) domainsecurity.MonotonicHeadAdvanceRequestV1 {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID:           fixture.binding.InstallationID,
		EnrollmentID:             fixture.binding.BootstrapEnrollmentID,
		Namespace:                fixture.binding.Namespace,
		ExpectedGeneration:       previous.Generation,
		ExpectedCheckpointDigest: expectedCheckpointDigest,
		ExpectedStateDigest:      previous.CurrentStateDigest,
		NextGeneration:           previous.Generation + 1,
		NextStateDigest:          nextState,
		ExpectedFenceNonce:       expectedFence,
		MutationID:               mutationID,
		AuthorityKeyID:           fixture.binding.InstallationAuthorityKeyID,
		AuthorityPublicKey:       fixture.installationPublic,
	}, domainsecurity.MonotonicHeadSignFunc(installationSignerV1(fixture.installationPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func newMonotonicAdvanceReceipt(
	t *testing.T,
	fixture bootstrapFixture,
	previous domainsecurity.MonotonicHeadCheckpointV1,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
	label string,
) domainsecurity.MonotonicHeadAdvanceReceiptV1 {
	t.Helper()
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID:           request.InstallationID,
		EnrollmentID:             request.EnrollmentID,
		Namespace:                request.Namespace,
		Generation:               request.NextGeneration,
		CurrentStateDigest:       request.NextStateDigest,
		PreviousStateDigest:      previous.CurrentStateDigest,
		PreviousCheckpointDigest: previous.CheckpointDigest,
		FenceNonce:               digestV1("next-fence:" + label),
		MutationID:               request.MutationID,
		WitnessKeyID:             fixture.binding.WitnessKeyID,
		WitnessPublicKey:         fixture.witnessPublic,
	}, domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(
		request, checkpoint, domainsecurity.MonotonicHeadSignFunc(witnessSignerV1(fixture.witnessPrivate)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func assertProjectionCondition(
	t *testing.T,
	condition ProjectionConditionV1,
	fixture bootstrapFixture,
	phase BootstrapPhaseV1,
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
) {
	t.Helper()
	if condition.InstallationID != fixture.binding.InstallationID ||
		condition.CurrentManifestDigest != fixture.binding.CurrentManifestDigest ||
		condition.ManifestEnrollmentDigest != fixture.binding.ManifestEnrollmentDigest ||
		condition.Namespace != fixture.binding.Namespace ||
		condition.SourceEnrollmentID != fixture.binding.EnrollmentID ||
		condition.BootstrapEnrollmentID != fixture.binding.BootstrapEnrollmentID ||
		condition.ProjectionSlotID != fixture.binding.ProjectionSlotID || condition.BindingDigest != fixture.binding.BindingDigest ||
		condition.AcceptedFloorObserveRequestDigest != fixture.binding.FloorObserveRequest.RequestDigest ||
		condition.AcceptedFloorObservationDigest != fixture.binding.FloorObservation.ObservationDigest ||
		!canonicalDigest(condition.AcceptedBootstrapObserveRequestDigest) ||
		!equalCheckpointV1(condition.FloorCheckpoint, fixture.binding.FloorCheckpoint) ||
		condition.FloorProjectionDigest != fixture.binding.FloorProjectionDigest || condition.ExpectedPhase != phase ||
		condition.ExpectedGeneration != checkpoint.Generation || condition.ExpectedCheckpointDigest != checkpoint.CheckpointDigest ||
		condition.ExpectedStateDigest != checkpoint.CurrentStateDigest || condition.ExpectedFenceNonce != checkpoint.FenceNonce {
		t.Fatalf("projection condition mismatch: %#v", condition)
	}
}

func exportedMethodNamesV1(value reflect.Type) []string {
	names := make([]string, 0, value.NumMethod())
	for index := 0; index < value.NumMethod(); index++ {
		names = append(names, value.Method(index).Name)
	}
	return names
}

func manifestEnrollmentInputV1(
	t *testing.T,
	enrollment domainenrollment.WitnessEnrollmentV1,
) domainenrollment.WitnessEnrollmentInputV1 {
	t.Helper()
	publicKey, err := base64.RawURLEncoding.DecodeString(enrollment.WitnessPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return domainenrollment.WitnessEnrollmentInputV1{
		EnrollmentID:                        enrollment.EnrollmentID,
		EndpointOrigin:                      enrollment.EndpointOrigin,
		WitnessKeyID:                        enrollment.WitnessKeyID,
		WitnessPublicKey:                    publicKey,
		RootCASHA256:                        enrollment.RootCASHA256,
		MTLSClientIdentityCertificateSHA256: enrollment.MTLSClientIdentityCertificateSHA256,
		ServerName:                          enrollment.ServerName,
		TimeoutMS:                           enrollment.TimeoutMS,
		InitialCheckpoint:                   enrollment.InitialCheckpoint,
	}
}

func newAuthorityObservationPairV1(
	t *testing.T,
	installationID, enrollmentID, namespace string,
	checkpoint domainsecurity.MonotonicHeadCheckpointV1,
	authorityPublic ed25519.PublicKey,
	authorityPrivate, witnessPrivate ed25519.PrivateKey,
	label string,
) (domainsecurity.MonotonicHeadObserveRequestV1, domainsecurity.MonotonicHeadObservationV1) {
	t.Helper()
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(
		domainsecurity.MonotonicHeadObserveRequestInputV1{
			InstallationID:     installationID,
			EnrollmentID:       enrollmentID,
			Namespace:          namespace,
			ChallengeNonce:     digestV1("authority-observe:" + label),
			AuthorityKeyID:     domainsecurity.SHA256Hex(authorityPublic),
			AuthorityPublicKey: authorityPublic,
		},
		func(message []byte) ([]byte, error) { return ed25519.Sign(authorityPrivate, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(
		request,
		checkpoint,
		func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	return request, observation
}

func deterministicKeyV1(label string) (ed25519.PublicKey, ed25519.PrivateKey) {
	seed := sha256.Sum256([]byte(label))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	return append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...), privateKey
}

func installationSignerV1(privateKey ed25519.PrivateKey) InstallationSignFuncV1 {
	return func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
}

func witnessSignerV1(privateKey ed25519.PrivateKey) WitnessSignFuncV1 {
	return func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
}

func resignBindingV1(binding *BootstrapBindingV1, privateKey ed25519.PrivateKey) {
	binding.InstallationAuthoritySignature = ""
	binding.BindingDigest = ""
	binding.InstallationAuthoritySignature = base64.RawURLEncoding.EncodeToString(
		ed25519.Sign(privateKey, BootstrapBindingSigningBytesV1(*binding)),
	)
	binding.BindingDigest = bindingDigestV1(*binding)
}

func digestV1(label string) string {
	return domainsecurity.SHA256Hex([]byte(label))
}

func decodeSignatureV1(t *testing.T, encoded string) []byte {
	t.Helper()
	signature, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}
