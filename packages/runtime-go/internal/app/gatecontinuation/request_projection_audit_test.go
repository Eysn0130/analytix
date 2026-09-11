package gatecontinuation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestProjectVerifiedGateRequestAcceptsTrustedPreBindingV3ForAudit(t *testing.T) {
	now := time.Date(2026, 7, 28, 5, 6, 7, 0, time.UTC)
	pending := cancellationPendingFixture(t, domaincontinuation.KindApproval, now)
	gateID := controlapp.SecureGateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID,
		pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	payload, err := appturn.GateContinuationPayload(
		domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	payload.LogicalEffect = ""
	payload.OrdinaryWork = false
	payload.ProviderStepExact = false
	payload.CaseSourceUnavailable = false
	payload.OrdinaryResultInputIsolated = false
	payload.ProviderStepPromptSHA256 = ""
	authority := newCancellationAuthority()
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	receipt := domaincontinuation.Receipt{
		Version: domaincontinuation.ContractVersionV3, Payload: payload,
		ReceiptID:          domainsecurity.SHA256Hex(append([]byte("analytix/gate-continuation-receipt/v3\x00"), payloadBody...)),
		AuthorityAlgorithm: domaincontinuation.AuthorityAlgorithm, AuthorityKeyID: authority.KeyID(),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(authority.PublicKey()),
	}
	signature, err := authority.Sign(context.Background(), domaincontinuation.ReceiptSigningBytes(receipt))
	if err != nil {
		t.Fatal(err)
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := domaincontinuation.ValidateReceiptForExecution(receipt); err == nil {
		t.Fatal("pre-binding V3 test receipt unexpectedly retained execution authority")
	}
	projection, err := ProjectVerifiedGateRequestV1(receipt)
	if err != nil || projection.GateID != gateID || projection.Record.ContinuationReceiptID != receipt.ReceiptID {
		t.Fatalf("pre-binding V3 request lost audit projection: projection=%#v err=%v", projection, err)
	}
}
