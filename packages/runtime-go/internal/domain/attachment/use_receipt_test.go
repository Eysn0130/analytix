package attachment

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestAttachmentUseReceiptV1BindsExactOwnerSetEffectAndConsumer(t *testing.T) {
	fixture := newAttachmentUseFixture(t)
	receipt := fixture.newReceipt(t, fixture.input(1))
	if receipt.Context.ThreadID != fixture.context.ThreadID || receipt.Context.TurnID != fixture.context.TurnID ||
		receipt.Context.CaseID != fixture.context.CaseID || receipt.Context.CaseBindingHash != fixture.context.CaseBindingHash ||
		receipt.Context.ContextEpoch != fixture.context.ContextEpoch || receipt.Context.DatasetSnapshotID != fixture.context.DatasetSnapshotID ||
		receipt.Context.ContextDigest != fixture.context.ContextDigest || receipt.OwnerDigest != fixture.owners[1].OwnerDigest ||
		receipt.BlobSHA256 != fixture.owners[1].BlobSHA256 || receipt.BlobByteSize != fixture.owners[1].ByteSize ||
		receipt.MIMEType != fixture.owners[1].MIMEType || receipt.ProjectionSHA256 != fixture.projections[1].ProjectionSHA256 ||
		receipt.ProjectionByteSize != fixture.projections[1].ProjectionByteSize || receipt.BatchIndex != 1 || receipt.BatchCount != 2 ||
		receipt.AttachmentSetDigest != AttachmentUseSetDigestV1(fixture.owners, fixture.projections) {
		t.Fatalf("attachment use receipt lost an exact binding: %#v", receipt)
	}
	expectedUseID, err := ComputeAttachmentUseIDV1(fixture.input(1))
	if err != nil || receipt.UseID != expectedUseID {
		t.Fatalf("stable use identity mismatch: got=%q want=%q err=%v", receipt.UseID, expectedUseID, err)
	}

	laterInput := fixture.input(1)
	laterInput.IssuedAt = laterInput.IssuedAt.Add(time.Minute)
	later := fixture.newReceipt(t, laterInput)
	if later.UseID != receipt.UseID || later.ReceiptDigest == receipt.ReceiptDigest {
		t.Fatalf("logical use identity changed with issuance or record digest did not: first=%#v later=%#v", receipt, later)
	}

	body, err := AttachmentUseReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseAttachmentUseReceiptV1(body)
	if err != nil || parsed.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatalf("receipt did not round-trip canonically: parsed=%#v err=%v", parsed, err)
	}
	keyID, publicKey, signature, err := AttachmentUseReceiptV1AuthorityMaterial(parsed)
	if err != nil || keyID != fixture.keyID || !bytes.Equal(publicKey, fixture.publicKey) ||
		!ed25519.Verify(publicKey, AttachmentUseReceiptV1SigningBytes(parsed), signature) {
		t.Fatalf("receipt authority material is invalid: key=%q err=%v", keyID, err)
	}

	disposition := fixture.newDisposition(t, receipt, AttachmentUseDispositionConsumedV1, "provider_consumed")
	if disposition.UseID != receipt.UseID || disposition.ReceiptDigest != receipt.ReceiptDigest ||
		disposition.OwnerDigest != receipt.OwnerDigest || disposition.AttachmentSetDigest != receipt.AttachmentSetDigest ||
		disposition.EffectBindingDigest != receipt.EffectBindingDigest || disposition.ConsumerBindingDigest != receipt.ConsumerBindingDigest {
		t.Fatalf("disposition lost exact receipt bindings: %#v", disposition)
	}
	dispositionBody, err := AttachmentUseDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	parsedDisposition, err := ParseAttachmentUseDispositionV1(dispositionBody)
	if err != nil || ValidateAttachmentUseDispositionForReceiptV1(parsedDisposition, receipt) != nil {
		t.Fatalf("disposition did not round-trip against its receipt: parsed=%#v err=%v", parsedDisposition, err)
	}
	_, dispositionKey, dispositionSignature, err := AttachmentUseDispositionV1AuthorityMaterial(parsedDisposition)
	if err != nil || !bytes.Equal(dispositionKey, fixture.publicKey) ||
		!ed25519.Verify(dispositionKey, AttachmentUseDispositionV1SigningBytes(parsedDisposition), dispositionSignature) {
		t.Fatalf("disposition authority material is invalid: err=%v", err)
	}
}

func TestAttachmentUseReceiptV1SupportsOrdinaryOwnerWithoutCaseAuthority(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuedContextAt := time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-ordinary-attachment", TurnID: "turn-ordinary-attachment",
		WorkspaceRealPath: "/workspace/ordinary", ContextEpoch: 3, IssuedAt: issuedContextAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewOwnerRecordV1(OwnerRecordInputV1{
		OwnerNonce: strings.Repeat("a", 32), BlobSHA256: domainsecurity.SHA256Hex([]byte("ordinary attachment")),
		ByteSize: 19, MIMEType: "text/plain", ThreadID: securityContext.ThreadID,
		WorkspaceRealPath: securityContext.WorkspaceRealPath,
		ProjectionSHA256:  domainsecurity.SHA256Hex([]byte("ordinary owner projection")),
		CreatedAt:         issuedContextAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := AttachmentUseProjectionV1{
		AttachmentID: owner.AttachmentID, ProjectionSHA256: domainsecurity.SHA256Hex([]byte("ordinary provider projection")),
		ProjectionByteSize: 28,
	}
	input := AttachmentUseReceiptInputV1{
		SecurityContext: securityContext, Owner: owner, AttachmentSet: []OwnerRecordV1{owner},
		ProjectionSet: []AttachmentUseProjectionV1{projection}, BatchIndex: 0,
		EffectBindingDigest:   domainsecurity.SHA256Hex([]byte("ordinary attachment effect")),
		ConsumerKind:          AttachmentUseConsumerPrimaryProviderV1,
		ConsumerBindingDigest: domainsecurity.SHA256Hex([]byte("ordinary attachment consumer")),
		IssuedAt:              issuedContextAt.Add(2 * time.Minute), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey),
		AuthorityPublicKey: publicKey,
	}
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	receipt, err := NewAttachmentUseReceiptV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Context.CaseID != domainsecurity.UnboundCaseID ||
		receipt.Context.CaseBindingHash != domainsecurity.UnboundCaseBindingHash(securityContext.WorkspaceRealPath) ||
		receipt.Context.DatasetSnapshotID != domainsecurity.NoDatasetSnapshotID ||
		receipt.Context.SourceManifestHash != domainsecurity.EmptySourceManifestHash ||
		ValidateAttachmentUseReceiptForBindingsV1(
			receipt, securityContext, owner, []OwnerRecordV1{owner}, []AttachmentUseProjectionV1{projection},
			input.EffectBindingDigest, input.ConsumerKind, input.ConsumerBindingDigest,
		) != nil {
		t.Fatalf("ordinary attachment use receipt lost its exact unbound authority: %#v", receipt)
	}
	disposition, err := NewAttachmentUseDispositionV1(
		receipt, AttachmentUseDispositionConsumedV1, "provider_consumed",
		issuedContextAt.Add(3*time.Minute), domainsecurity.SHA256Hex(publicKey), publicKey, sign,
	)
	if err != nil || ValidateAttachmentUseDispositionForReceiptV1(disposition, receipt) != nil {
		t.Fatalf("ordinary attachment disposition is invalid: disposition=%#v err=%v", disposition, err)
	}

	caseFixture := newAttachmentUseFixture(t)
	caseInputWithOrdinaryOwner := caseFixture.input(0)
	caseInputWithOrdinaryOwner.Owner = owner
	caseInputWithOrdinaryOwner.AttachmentSet = []OwnerRecordV1{owner}
	caseInputWithOrdinaryOwner.ProjectionSet = []AttachmentUseProjectionV1{projection}
	caseInputWithOrdinaryOwner.BatchIndex = 0
	if _, err := NewAttachmentUseReceiptV1(caseInputWithOrdinaryOwner, caseFixture.sign); err == nil {
		t.Fatal("ordinary attachment owner crossed into a case-fact provider effect")
	}
	ordinaryInputWithCaseOwner := input
	ordinaryInputWithCaseOwner.Owner = caseFixture.owners[0]
	ordinaryInputWithCaseOwner.AttachmentSet = []OwnerRecordV1{caseFixture.owners[0]}
	ordinaryInputWithCaseOwner.ProjectionSet = []AttachmentUseProjectionV1{caseFixture.projections[0]}
	if _, err := NewAttachmentUseReceiptV1(ordinaryInputWithCaseOwner, sign); err == nil {
		t.Fatal("case-bound attachment owner crossed into an ordinary provider effect")
	}

	snapshotUnavailable := snapshotUnavailableAttachmentUseContextV2(
		t, securityContext.ThreadID, securityContext.TurnID, securityContext.WorkspaceRealPath, issuedContextAt,
	)
	if err := domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(snapshotUnavailable); err != nil {
		t.Fatalf("snapshot-unavailable boundary must retain ordinary effect authority: %v", err)
	}
	snapshotUnavailableInput := input
	snapshotUnavailableInput.SecurityContext = snapshotUnavailable
	if _, err := NewAttachmentUseReceiptV1(snapshotUnavailableInput, sign); err == nil {
		t.Fatal("ordinary attachment use receipt crossed a valid case binding without DSV2")
	}
}

func TestAttachmentUseReceiptV1StrictParsingRejectsUnknownNonCanonicalAndTamperedFields(t *testing.T) {
	fixture := newAttachmentUseFixture(t)
	receipt := fixture.newReceipt(t, fixture.input(0))
	body, err := AttachmentUseReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(body, []byte(`"purpose":`), []byte(`"safeToAnswer":true,"purpose":`), 1)
	if _, err := ParseAttachmentUseReceiptV1(unknown); err == nil {
		t.Fatal("unknown attachment use receipt field was accepted")
	}
	duplicate := bytes.Replace(body, []byte(`"useId":"`+receipt.UseID+`"`),
		[]byte(`"useId":"`+receipt.UseID+`","useId":"`+receipt.UseID+`"`), 1)
	if _, err := ParseAttachmentUseReceiptV1(duplicate); err == nil {
		t.Fatal("duplicate attachment use receipt field was accepted")
	}
	if _, err := ParseAttachmentUseReceiptV1(append(append([]byte(nil), body...), '\n')); err == nil {
		t.Fatal("non-canonical attachment use receipt encoding was accepted")
	}
	tamperedBlob := bytes.Replace(body, []byte(receipt.BlobSHA256), []byte(domainsecurity.SHA256Hex([]byte("other-blob"))), 1)
	if _, err := ParseAttachmentUseReceiptV1(tamperedBlob); err == nil {
		t.Fatal("attachment use blob tamper was accepted")
	}
	tamperedSignature := receipt
	tamperedSignature.AuthoritySignature = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if ValidateAttachmentUseReceiptV1(tamperedSignature) == nil {
		t.Fatal("attachment use signature tamper was accepted")
	}

	disposition := fixture.newDisposition(t, receipt, AttachmentUseDispositionRejectedV1, "consumer_rejected")
	dispositionBody, err := AttachmentUseDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	unknownDisposition := bytes.Replace(dispositionBody, []byte(`"purpose":`), []byte(`"factAnswerAllowed":true,"purpose":`), 1)
	if _, err := ParseAttachmentUseDispositionV1(unknownDisposition); err == nil {
		t.Fatal("unknown attachment use disposition field was accepted")
	}
	tamperedDisposition := disposition
	tamperedDisposition.ReceiptDigest = domainsecurity.SHA256Hex([]byte("other-receipt"))
	if ValidateAttachmentUseDispositionV1(tamperedDisposition) == nil {
		t.Fatal("attachment use disposition receipt tamper was accepted")
	}
}

func TestAttachmentUseReceiptV1RejectsCrossTurnEpochSnapshotContextAndProjection(t *testing.T) {
	fixture := newAttachmentUseFixture(t)
	receipt := fixture.newReceipt(t, fixture.input(0))
	validate := func(context domainsecurity.TurnSecurityContext, owner OwnerRecordV1, set []OwnerRecordV1, projections []AttachmentUseProjectionV1) error {
		return ValidateAttachmentUseReceiptForBindingsV1(
			receipt, context, owner, set, projections, fixture.effectBindingDigest,
			AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
		)
	}
	if err := validate(fixture.context, fixture.owners[0], fixture.owners, fixture.projections); err != nil {
		t.Fatalf("exact attachment bindings were rejected: %v", err)
	}

	contexts := map[string]domainsecurity.TurnSecurityContext{
		"cross turn":     fixture.contextFor(t, "turn-other", fixture.context.ContextEpoch, "snapshot-a", "manifest-a", "authority-a"),
		"cross epoch":    fixture.contextFor(t, fixture.context.TurnID, fixture.context.ContextEpoch+1, "snapshot-a", "manifest-a", "authority-a"),
		"cross snapshot": fixture.contextFor(t, fixture.context.TurnID, fixture.context.ContextEpoch, "snapshot-b", "manifest-a", "authority-a"),
		"cross context":  fixture.contextFor(t, fixture.context.TurnID, fixture.context.ContextEpoch, "snapshot-a", "manifest-b", "authority-b"),
	}
	for name, context := range contexts {
		t.Run(name, func(t *testing.T) {
			if err := validate(context, fixture.owners[0], fixture.owners, fixture.projections); err == nil {
				t.Fatal("attachment use receipt crossed its exact frozen context")
			}
		})
	}

	projectionSet := append([]AttachmentUseProjectionV1(nil), fixture.projections...)
	projectionSet[0].ProjectionSHA256 = domainsecurity.SHA256Hex([]byte("actual-provider-projection-x"))
	if err := validate(fixture.context, fixture.owners[0], fixture.owners, projectionSet); err == nil {
		t.Fatal("attachment use receipt crossed its exact provider projection")
	}
	if err := ValidateAttachmentUseReceiptForBindingsV1(
		receipt, fixture.context, fixture.owners[0], fixture.owners, fixture.projections,
		domainsecurity.SHA256Hex([]byte("different-effect")), AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
	); err == nil {
		t.Fatal("attachment use receipt crossed its effect binding")
	}
	if err := ValidateAttachmentUseReceiptForBindingsV1(
		receipt, fixture.context, fixture.owners[0], fixture.owners, fixture.projections,
		fixture.effectBindingDigest, AttachmentUseConsumerVisionBridgeV1, domainsecurity.SHA256Hex([]byte("vision-consumer")),
	); err == nil {
		t.Fatal("attachment use receipt crossed its consumer binding")
	}
}

func TestAttachmentUseReceiptV1ActualProviderProjectionChangesUseIDWithoutOwnerChange(t *testing.T) {
	fixture := newAttachmentUseFixture(t)
	baseInput := fixture.input(0)
	baseUseID, err := ComputeAttachmentUseIDV1(baseInput)
	if err != nil {
		t.Fatal(err)
	}
	changedInput := fixture.input(0)
	changedInput.ProjectionSet = append([]AttachmentUseProjectionV1(nil), changedInput.ProjectionSet...)
	changedProjection := []byte("actual-provider-projection-x")
	changedInput.ProjectionSet[0].ProjectionSHA256 = domainsecurity.SHA256Hex(changedProjection)
	changedInput.ProjectionSet[0].ProjectionByteSize = int64(len(changedProjection))
	changedUseID, err := ComputeAttachmentUseIDV1(changedInput)
	if err != nil {
		t.Fatal(err)
	}
	if changedInput.Owner.OwnerDigest != baseInput.Owner.OwnerDigest ||
		changedInput.Owner.ProjectionSHA256 != baseInput.Owner.ProjectionSHA256 || changedUseID == baseUseID {
		t.Fatalf("actual route projection was not independently bound: base=%q changed=%q", baseUseID, changedUseID)
	}
	receipt := fixture.newReceipt(t, baseInput)
	if err := ValidateAttachmentUseReceiptForBindingsV1(
		receipt, fixture.context, fixture.owners[0], fixture.owners, changedInput.ProjectionSet,
		fixture.effectBindingDigest, AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
	); err == nil {
		t.Fatal("receipt accepted a mismatched actual provider projection under the same owner projection")
	}
	changedSizeInput := fixture.input(0)
	changedSizeInput.ProjectionSet = append([]AttachmentUseProjectionV1(nil), changedSizeInput.ProjectionSet...)
	changedSizeInput.ProjectionSet[0].ProjectionByteSize++
	changedSizeUseID, err := ComputeAttachmentUseIDV1(changedSizeInput)
	if err != nil || changedSizeUseID == baseUseID {
		t.Fatalf("actual projection byte size was not use-identity bound: id=%q err=%v", changedSizeUseID, err)
	}
	for name, edit := range map[string]func(*AttachmentUseReceiptV1){
		"blob byte size": func(candidate *AttachmentUseReceiptV1) { candidate.BlobByteSize++ },
		"MIME type":      func(candidate *AttachmentUseReceiptV1) { candidate.MIMEType = "text/plain" },
	} {
		t.Run(name, func(t *testing.T) {
			forged := receipt
			edit(&forged)
			fixture.resignReceipt(&forged)
			if ValidateAttachmentUseReceiptV1(forged) != nil {
				t.Fatal("self-consistent fixture should require exact owner material validation")
			}
			if err := ValidateAttachmentUseReceiptForBindingsV1(
				forged, fixture.context, fixture.owners[0], fixture.owners, fixture.projections,
				fixture.effectBindingDigest, AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
			); err == nil {
				t.Fatal("receipt accepted owner material that did not match its exact owner")
			}
		})
	}
}

func TestAttachmentUseReceiptV1EnforcesBatchIndexCountAndOrderedSet(t *testing.T) {
	fixture := newAttachmentUseFixture(t)
	invalidIndex := fixture.input(2)
	if _, err := NewAttachmentUseReceiptV1(invalidIndex, fixture.sign); err == nil {
		t.Fatal("out-of-range attachment batch index was accepted")
	}
	duplicate := fixture.input(0)
	duplicate.AttachmentSet = []OwnerRecordV1{fixture.owners[0], fixture.owners[0]}
	duplicate.ProjectionSet = []AttachmentUseProjectionV1{fixture.projections[0], fixture.projections[0]}
	if _, err := NewAttachmentUseReceiptV1(duplicate, fixture.sign); err == nil ||
		AttachmentUseSetDigestV1(duplicate.AttachmentSet, duplicate.ProjectionSet) != "" {
		t.Fatal("duplicate attachment owner was accepted in an ordered set")
	}
	missingProjection := fixture.input(0)
	missingProjection.ProjectionSet = nil
	if _, err := NewAttachmentUseReceiptV1(missingProjection, fixture.sign); err == nil {
		t.Fatal("attachment use receipt defaulted to the owner metadata projection")
	}

	receipt := fixture.newReceipt(t, fixture.input(1))
	reordered := []OwnerRecordV1{fixture.owners[1], fixture.owners[0]}
	reorderedProjections := []AttachmentUseProjectionV1{fixture.projections[1], fixture.projections[0]}
	if err := ValidateAttachmentUseReceiptForBindingsV1(
		receipt, fixture.context, fixture.owners[1], reordered, reorderedProjections,
		fixture.effectBindingDigest, AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
	); err == nil {
		t.Fatal("reordered attachment set was accepted")
	}
	shortened := []OwnerRecordV1{fixture.owners[1]}
	shortenedProjections := []AttachmentUseProjectionV1{fixture.projections[1]}
	if err := ValidateAttachmentUseReceiptForBindingsV1(
		receipt, fixture.context, fixture.owners[1], shortened, shortenedProjections,
		fixture.effectBindingDigest, AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
	); err == nil {
		t.Fatal("shortened attachment set was accepted")
	}

	forgedCount := receipt
	forgedCount.BatchCount++
	fixture.resignReceipt(&forgedCount)
	if ValidateAttachmentUseReceiptV1(forgedCount) != nil {
		t.Fatal("self-consistent batch-count fixture should require material binding validation")
	}
	if err := ValidateAttachmentUseReceiptForBindingsV1(
		forgedCount, fixture.context, fixture.owners[1], fixture.owners, fixture.projections,
		fixture.effectBindingDigest, AttachmentUseConsumerPrimaryProviderV1, fixture.consumerBindingDigest,
	); err == nil {
		t.Fatal("receipt batch count not backed by the exact set was accepted")
	}
}

func TestAttachmentUseReceiptV1RequiresCanonicalAuthorityAndDispositionBinding(t *testing.T) {
	fixture := newAttachmentUseFixture(t)
	badKey := fixture.input(0)
	badKey.AuthorityKeyID = domainsecurity.SHA256Hex([]byte("not-the-public-key"))
	if _, err := NewAttachmentUseReceiptV1(badKey, fixture.sign); err == nil {
		t.Fatal("attachment use receipt accepted a mismatched authority key id")
	}
	if _, err := NewAttachmentUseReceiptV1(fixture.input(0), func([]byte) ([]byte, error) { return []byte("short"), nil }); err == nil {
		t.Fatal("attachment use receipt accepted a short authority signature")
	}
	receipt := fixture.newReceipt(t, fixture.input(0))
	predatesContext := receipt
	predatesContext.IssuedAt = time.Date(2026, 7, 14, 1, 30, 0, 0, time.UTC).Format(time.RFC3339Nano)
	fixture.resignReceipt(&predatesContext)
	if ValidateAttachmentUseReceiptV1(predatesContext) == nil {
		t.Fatal("attachment use receipt predating its frozen context was accepted")
	}
	lateOwner := fixture.ownerAt(t, 4, domainsecurity.SHA256Hex([]byte("late-projection")), fixture.issuedAt.Add(time.Minute))
	lateInput := fixture.input(0)
	lateInput.Owner = lateOwner
	lateInput.AttachmentSet = []OwnerRecordV1{lateOwner, fixture.owners[1]}
	lateInput.ProjectionSet[0].AttachmentID = lateOwner.AttachmentID
	if _, err := NewAttachmentUseReceiptV1(lateInput, fixture.sign); err == nil {
		t.Fatal("attachment use receipt predating one ordered-set owner was accepted")
	}
	otherPublic, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAttachmentUseDispositionV1(
		receipt, AttachmentUseDispositionConsumedV1, "consumed", fixture.disposedAt,
		domainsecurity.SHA256Hex(otherPublic), otherPublic,
		func(message []byte) ([]byte, error) { return ed25519.Sign(otherPrivate, message), nil },
	); err == nil {
		t.Fatal("attachment use disposition accepted a different authority")
	}
	disposition := fixture.newDisposition(t, receipt, AttachmentUseDispositionCancelledV1, "turn_cancelled")
	otherReceipt := fixture.newReceipt(t, fixture.input(1))
	if ValidateAttachmentUseDispositionForReceiptV1(disposition, otherReceipt) == nil {
		t.Fatal("attachment use disposition was accepted for another use receipt")
	}
	if _, err := NewAttachmentUseDispositionV1(
		receipt, AttachmentUseDispositionConsumedV1, "invalid reason", fixture.disposedAt,
		fixture.keyID, fixture.publicKey, fixture.sign,
	); err == nil {
		t.Fatal("attachment use disposition accepted a non-canonical reason code")
	}
}

type attachmentUseFixture struct {
	context               domainsecurity.TurnSecurityContext
	owners                []OwnerRecordV1
	projections           []AttachmentUseProjectionV1
	publicKey             ed25519.PublicKey
	privateKey            ed25519.PrivateKey
	keyID                 string
	effectBindingDigest   string
	consumerBindingDigest string
	issuedAt              time.Time
	disposedAt            time.Time
}

func newAttachmentUseFixture(t *testing.T) attachmentUseFixture {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture := attachmentUseFixture{
		publicKey: publicKey, privateKey: privateKey, keyID: domainsecurity.SHA256Hex(publicKey),
		effectBindingDigest:   domainsecurity.SHA256Hex([]byte("effect:primary:turn-a")),
		consumerBindingDigest: domainsecurity.SHA256Hex([]byte("provider:deepseek:model:route:payload")),
		issuedAt:              time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC),
		disposedAt:            time.Date(2026, 7, 14, 3, 1, 0, 0, time.UTC),
	}
	fixture.context = fixture.contextFor(t, "turn-a", 7, "snapshot-a", "manifest-a", "authority-a")
	fixture.owners = []OwnerRecordV1{
		fixture.owner(t, 1, domainsecurity.SHA256Hex([]byte("projection-a"))),
		fixture.owner(t, 2, domainsecurity.SHA256Hex([]byte("projection-b"))),
	}
	firstProjection := []byte("actual-provider-projection-a")
	secondProjection := []byte("actual-provider-projection-b")
	fixture.projections = []AttachmentUseProjectionV1{
		{AttachmentID: fixture.owners[0].AttachmentID, ProjectionSHA256: domainsecurity.SHA256Hex(firstProjection), ProjectionByteSize: int64(len(firstProjection))},
		{AttachmentID: fixture.owners[1].AttachmentID, ProjectionSHA256: domainsecurity.SHA256Hex(secondProjection), ProjectionByteSize: int64(len(secondProjection))},
	}
	return fixture
}

func (fixture attachmentUseFixture) input(index uint32) AttachmentUseReceiptInputV1 {
	input := AttachmentUseReceiptInputV1{
		SecurityContext: fixture.context, AttachmentSet: append([]OwnerRecordV1(nil), fixture.owners...),
		ProjectionSet: append([]AttachmentUseProjectionV1(nil), fixture.projections...),
		BatchIndex:    index, EffectBindingDigest: fixture.effectBindingDigest,
		ConsumerKind: AttachmentUseConsumerPrimaryProviderV1, ConsumerBindingDigest: fixture.consumerBindingDigest,
		IssuedAt: fixture.issuedAt, AuthorityKeyID: fixture.keyID, AuthorityPublicKey: append([]byte(nil), fixture.publicKey...),
	}
	if index < uint32(len(fixture.owners)) {
		input.Owner = fixture.owners[index]
	}
	return input
}

func (fixture attachmentUseFixture) newReceipt(t *testing.T, input AttachmentUseReceiptInputV1) AttachmentUseReceiptV1 {
	t.Helper()
	receipt, err := NewAttachmentUseReceiptV1(input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func (fixture attachmentUseFixture) newDisposition(t *testing.T, receipt AttachmentUseReceiptV1, status, reason string) AttachmentUseDispositionV1 {
	t.Helper()
	disposition, err := NewAttachmentUseDispositionV1(
		receipt, status, reason, fixture.disposedAt, fixture.keyID, fixture.publicKey, fixture.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	return disposition
}

func (fixture attachmentUseFixture) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.privateKey, message), nil
}

func (fixture attachmentUseFixture) resignReceipt(receipt *AttachmentUseReceiptV1) {
	receipt.UseID = attachmentUseIDV1(*receipt)
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(fixture.privateKey, AttachmentUseReceiptV1SigningBytes(*receipt)))
	receipt.ReceiptDigest = attachmentUseReceiptDigestV1(*receipt)
}

func snapshotUnavailableAttachmentUseContextV2(
	t *testing.T,
	threadID string,
	turnID string,
	workspace string,
	issuedAt time.Time,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	caseContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		CaseID: "case-snapshot-unavailable", ContextEpoch: 3, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   caseContext.PublicationPolicy.ThreadRiskPolicyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: caseContext.PublicationPolicy.BindingObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerDatasetSnapshotUnavailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: caseContext.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: policy, RiskAuthorityBinding: caseContext.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return boundary
}

func (fixture attachmentUseFixture) contextFor(
	t *testing.T,
	turnID string,
	epoch uint64,
	snapshotSeed string,
	manifestSeed string,
	authoritySeed string,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	threadID, workspace, caseID := "thread-a", "/cases/a", "case-a"
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("policy:" + authoritySeed)),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation:" + authoritySeed)),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding := domainsecurity.RiskAuthorityBindingV1{
		SchemaVersion: domainsecurity.RiskAuthorityBindingSchemaVersion, Purpose: domainsecurity.RiskAuthorityBindingPurpose,
		State:       domainsecurity.RiskAuthorityBindingStateWitnessed,
		IndexDigest: domainsecurity.SHA256Hex([]byte("index:" + authoritySeed)), Generation: 1,
		CheckpointDigest:  domainsecurity.SHA256Hex([]byte("checkpoint:" + authoritySeed)),
		ObservationDigest: domainsecurity.SHA256Hex([]byte("observation:" + authoritySeed)),
	}
	context, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: caseID,
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding:" + caseID)),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte(snapshotSeed)),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte(manifestSeed)), ContextEpoch: epoch,
		IssuedAt: time.Date(2026, 7, 14, 2, 0, 0, 0, time.UTC), PublicationPolicy: policy,
		RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func (fixture attachmentUseFixture) owner(t *testing.T, sequence int, projectionSHA256 string) OwnerRecordV1 {
	return fixture.ownerAt(t, sequence, projectionSHA256, time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC))
}

func (fixture attachmentUseFixture) ownerAt(t *testing.T, sequence int, projectionSHA256 string, createdAt time.Time) OwnerRecordV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: fixture.context.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid,
		CaseID: fixture.context.CaseID, BindingSHA256: domainsecurity.SHA256Hex([]byte("binding-document")),
		CaseBindingHash: fixture.context.CaseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewOwnerRecordV1(OwnerRecordInputV1{
		OwnerNonce: fmt.Sprintf("%032x", sequence), BlobSHA256: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("blob-%d", sequence))),
		ByteSize: int64(sequence), MIMEType: "application/pdf", ThreadID: fixture.context.ThreadID,
		WorkspaceRealPath: fixture.context.WorkspaceRealPath, CaseBindingObservation: &observation,
		ProjectionSHA256: projectionSHA256, CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
