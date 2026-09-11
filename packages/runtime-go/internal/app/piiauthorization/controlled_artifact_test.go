package piiauthorization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrepareControlledArtifactReturnsHashOnlyApprovalIntent(t *testing.T) {
	fixture := newServiceFixture(t)
	prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Metadata.FieldBindings) != 1 ||
		prepared.Metadata.FieldBindings[0].ValueSHA256 != domainsecurity.SHA256Hex([]byte(serviceTestAccountExact)) ||
		prepared.ProtectedSHA256 != prepared.Metadata.SHA256 || prepared.ProtectedByteLength != prepared.Metadata.ByteLength ||
		prepared.ProtectedByteLength == 0 {
		t.Fatalf("hash-only preparation lost exact controlled bindings: %#v", prepared)
	}
	preparedBody, err := json.Marshal(prepared)
	if err != nil || bytes.Contains(preparedBody, []byte(serviceTestAccountExact)) || strings.Contains(string(preparedBody), "exactValue") {
		t.Fatalf("prepared approval intent exposed controlled bytes: %s err=%v", preparedBody, err)
	}
	arguments, err := ParseControlledPIIApprovalArgumentsV1(prepared.Approval.PrivateArguments)
	if err != nil || arguments.ProjectedContentSHA256 != prepared.ProtectedSHA256 ||
		arguments.TargetIdentityDigest != prepared.Metadata.TargetIdentityDigest ||
		arguments.ApprovalScopeDigest != prepared.Approval.ApprovalScopeDigest {
		t.Fatalf("approval does not bind the exact controlled candidate: approval=%#v arguments=%#v err=%v", prepared.Approval, arguments, err)
	}
	if bytes.Contains(prepared.Approval.PrivateArguments, []byte(serviceTestAccountExact)) {
		t.Fatalf("approval arguments leaked the raw account: %s", prepared.Approval.PrivateArguments)
	}
	if len(fixture.grants.records) != 0 || len(fixture.ledgers.records) != 0 {
		t.Fatalf("pre-approval staging persisted publication authority: grants=%d ledgers=%d", len(fixture.grants.records), len(fixture.ledgers.records))
	}

	// A candidate hash is not a grant/audit record and cannot authorize report
	// publication by itself.
	projection := fixture.controlledProjection(t, prepared.ProtectedSHA256, prepared.ProtectedSHA256)
	if fixture.service.ValidateCurrent(context.Background(), fixture.context, projection, prepared.Metadata.TargetIdentityDigest) == nil {
		t.Fatal("protected candidate bytes became publication authority without approval and a signed grant")
	}
}

func TestPrepareControlledArtifactFailsClosedBeforeReturnOnMismatchOrPostRenderRevocation(t *testing.T) {
	t.Run("caller cannot substitute raw value through a mismatched hash", func(t *testing.T) {
		fixture := newServiceFixture(t)
		input := prepareControlledArtifactFixtureInput(fixture)
		input.FieldBindings = append([]domainpii.FieldBindingV1(nil), input.FieldBindings...)
		input.FieldBindings[0].ValueSHA256 = domainsecurity.SHA256Hex([]byte("attacker-substituted-account"))
		prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), input)
		if !errors.Is(err, ErrMismatch) || prepared.ProtectedByteLength != 0 || prepared.Metadata.SHA256 != "" {
			t.Fatalf("mismatched field returned protected material: prepared=%#v err=%v", prepared, err)
		}
	})

	t.Run("evidence revoked after render", func(t *testing.T) {
		fixture := newServiceFixture(t)
		// Call 1 validates before extraction, call 2 witnesses and renders the
		// exact source field, and call 3 is the post-render approval recheck.
		fixture.evidence.rejectOnCall = 3
		prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
		if !errors.Is(err, ErrMismatch) || prepared.ProtectedByteLength != 0 || fixture.evidence.calls != 3 {
			t.Fatalf("post-render revocation returned a candidate: prepared=%#v calls=%d err=%v", prepared, fixture.evidence.calls, err)
		}
	})

	t.Run("stale context before extraction", func(t *testing.T) {
		fixture := newServiceFixture(t)
		fixture.currentReject = true
		prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
		if !errors.Is(err, ErrMismatch) || prepared.ProtectedByteLength != 0 || fixture.evidence.calls != 0 {
			t.Fatalf("stale context reached evidence or returned material: prepared=%#v evidenceCalls=%d err=%v", prepared, fixture.evidence.calls, err)
		}
	})
}

func TestAuthorizePreparedControlledArtifactRequiresApprovalAndProducesExactGrantProjectionChain(t *testing.T) {
	fixture := newServiceFixture(t)
	prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := fixture.service.AuthorizePreparedControlledArtifact(context.Background(), AuthorizePreparedControlledArtifactInputV1{
		SecurityContext: fixture.context, ClaimLedger: fixture.input.ClaimLedger, Prepared: prepared,
		ApprovalID: fixture.input.ApprovalID, ApprovalRecordDigest: fixture.input.ApprovalRecordDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(authorized.Metadata, prepared.Metadata) || authorized.ProtectedSHA256 != prepared.ProtectedSHA256 ||
		authorized.ProtectedByteLength != prepared.ProtectedByteLength || authorized.Lease == nil ||
		authorized.Grant.ProjectedContentSHA256 != prepared.ProtectedSHA256 ||
		authorized.Grant.ApprovalScopeDigest != prepared.Approval.ApprovalScopeDigest ||
		authorized.PIIProjection.ProjectedContentSHA256 != prepared.ProtectedSHA256 ||
		authorized.PIIProjection.AuthorizationAuditDigest != authorized.Grant.RecordDigest ||
		authorized.PIIProjection.PreservedControlledFieldCount != 1 {
		t.Fatalf("authorized chain lost an exact binding: %#v", authorized)
	}
	grantBody, err := domainpii.PIIProjectionGrantV1Bytes(authorized.Grant)
	if err != nil {
		t.Fatal(err)
	}
	projectionBody, err := domainpublication.PIIProjectionV1Bytes(authorized.PIIProjection)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(grantBody, []byte(serviceTestAccountExact)) || bytes.Contains(projectionBody, []byte(serviceTestAccountExact)) {
		t.Fatalf("hash-only authority record leaked the raw account: grant=%s projection=%s", grantBody, projectionBody)
	}
	if len(fixture.grants.records) != 1 || len(fixture.ledgers.records) != 1 {
		t.Fatalf("authorized chain did not persist exact protected authorities: grants=%d ledgers=%d", len(fixture.grants.records), len(fixture.ledgers.records))
	}
	released, disposition, err := consumeAuthorizedControlledArtifactForTest(context.Background(), authorized)
	if err != nil || !bytes.Contains(released, []byte(`"exactValue":"`+serviceTestAccountExact+`"`)) ||
		disposition.Status != ControlledPIITerminalHandoffConsumedV1 {
		t.Fatalf("authorized terminal lease did not preserve exact controlled bytes: body=%s disposition=%#v err=%v", released, disposition, err)
	}
}

func TestAuthorizePreparedControlledArtifactRejectsChangedIntentAndRevokedApproval(t *testing.T) {
	t.Run("changed hash-only intent", func(t *testing.T) {
		fixture := newServiceFixture(t)
		prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
		if err != nil {
			t.Fatal(err)
		}
		prepared.ProtectedSHA256 = domainsecurity.SHA256Hex([]byte("changed-candidate"))
		authorized, err := fixture.service.AuthorizePreparedControlledArtifact(context.Background(), AuthorizePreparedControlledArtifactInputV1{
			SecurityContext: fixture.context, ClaimLedger: fixture.input.ClaimLedger, Prepared: prepared,
			ApprovalID: fixture.input.ApprovalID, ApprovalRecordDigest: fixture.input.ApprovalRecordDigest,
		})
		if !errors.Is(err, ErrMismatch) || authorized.Lease != nil || len(fixture.grants.records) != 0 {
			t.Fatalf("changed candidate received authority: authorized=%#v grants=%d err=%v", authorized, len(fixture.grants.records), err)
		}
	})

	t.Run("approval revoked after preparation", func(t *testing.T) {
		fixture := newServiceFixture(t)
		prepared, err := fixture.service.PrepareControlledArtifact(context.Background(), prepareControlledArtifactFixtureInput(fixture))
		if err != nil {
			t.Fatal(err)
		}
		fixture.approval.reject = true
		authorized, err := fixture.service.AuthorizePreparedControlledArtifact(context.Background(), AuthorizePreparedControlledArtifactInputV1{
			SecurityContext: fixture.context, ClaimLedger: fixture.input.ClaimLedger, Prepared: prepared,
			ApprovalID: fixture.input.ApprovalID, ApprovalRecordDigest: fixture.input.ApprovalRecordDigest,
		})
		if !errors.Is(err, ErrMismatch) || authorized.Lease != nil || len(fixture.grants.records) != 0 {
			t.Fatalf("revoked approval received authority: authorized=%#v grants=%d err=%v", authorized, len(fixture.grants.records), err)
		}
	})
}

func consumeAuthorizedControlledArtifactForTest(
	ctx context.Context,
	authorized AuthorizedControlledArtifactV1,
) ([]byte, ControlledPIITerminalDispositionV1, error) {
	var body []byte
	disposition, err := authorized.Lease.Consume(ctx, func(
		_ context.Context,
		reader io.Reader,
		_ ControlledPIITerminalLeaseMetadataV1,
	) error {
		var readErr error
		body, readErr = io.ReadAll(reader)
		return readErr
	})
	return body, disposition, err
}

func prepareControlledArtifactFixtureInput(fixture *serviceFixture) PrepareControlledArtifactInputV1 {
	return PrepareControlledArtifactInputV1{
		SecurityContext: fixture.input.SecurityContext, ClaimLedger: fixture.input.ClaimLedger,
		FieldBindings:         append([]domainpii.FieldBindingV1(nil), fixture.input.FieldBindings...),
		ProjectionRulesetHash: fixture.input.ProjectionRulesetHash,
		TargetIdentityDigest:  fixture.input.TargetIdentityDigest,
		AllowedAccessActions:  append([]string(nil), fixture.input.AllowedAccessActions...),
		AccessPolicyDigest:    fixture.input.AccessPolicyDigest,
		RetentionPolicyDigest: fixture.input.RetentionPolicyDigest,
		RetentionUntil:        fixture.input.RetentionUntil,
		ExpiresAt:             fixture.input.ExpiresAt,
	}
}
