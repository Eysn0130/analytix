package reportpublication

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

func TestAttemptCoreInventoryUsesEveryPendingStageAndExactCanonicalBinding(t *testing.T) {
	ctx := context.Background()
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(ctx, fixture.input); err != nil {
		t.Fatal(err)
	}
	var attempt domainpublication.PublicationAttemptV1
	if err := fixture.attempts.VisitAttempts(ctx, func(value domainpublication.PublicationAttemptV1) error { attempt = value; return nil }); err != nil {
		t.Fatal(err)
	}
	stage := fixture.stages.receipt
	for _, status := range []string{"", domainpendingwork.StatusCompleted, domainpendingwork.StatusFailed, domainpendingwork.StatusExpired, domainpendingwork.StatusOutcomeUnknown} {
		t.Run("status-"+status, func(t *testing.T) {
			pending := pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{stage}}
			if status != "" {
				issued, err := time.Parse(time.RFC3339Nano, stage.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				reason := "synthetic_linkage_control"
				if status == domainpendingwork.StatusCompleted {
					reason = "report_stage_completed"
				} else if status == domainpendingwork.StatusOutcomeUnknown {
					reason = "report_stage_outcome_unknown_after_restart"
				}
				disposition, err := domainpendingwork.NewPendingWorkDispositionV1(stage, status, reason, issued.Add(2*time.Hour), fixture.authority.KeyID(), fixture.authority.PublicKey(), func(body []byte) ([]byte, error) { return fixture.authority.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				pending.Dispositions = map[string]domainpendingwork.PendingWorkDispositionV1{stage.WorkID: disposition}
			}
			if err := VerifyAttemptCoreInventoryV1(ctx, pending, coreAttemptListV1{attempt}, fixture.authority); err != nil {
				t.Fatalf("terminal/expired stage was omitted from Core linkage: %v", err)
			}
		})
	}
	// These are self-valid signed attempts whose exact Core relation is wrong.
	// They do not claim a complete healthy publication or report-stage producer.
	for _, field := range []string{"receipt-hash", "grant-entry", "context"} {
		t.Run(field, func(t *testing.T) {
			changed := attempt
			switch field {
			case "receipt-hash":
				changed.ReportStageReceiptSHA256 = domainsecurity.SHA256Hex([]byte("other signed receipt bytes"))
			case "grant-entry":
				changed.GrantRegistryEntryDigest = domainsecurity.SHA256Hex([]byte("other grant entry"))
			case "context":
				changed.ContextDigest = domainsecurity.SHA256Hex([]byte("other context"))
			}
			changed.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(fixture.authority.privateKey, domainpublication.PublicationAttemptSigningBytesV1(changed)))
			changed.RecordDigest = ""
			body, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			changed.RecordDigest = domainsecurity.SHA256Hex(append([]byte("analytix.report-publication-attempt/digest/v1\x00"), body...))
			if err := domainpublication.ValidatePublicationAttemptV1(changed); err != nil {
				t.Fatalf("negative fixture is not independently valid: %v", err)
			}
			pending := pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{stage}}
			if err := VerifyAttemptCoreInventoryV1(ctx, pending, coreAttemptListV1{changed}, fixture.authority); !errors.Is(err, ErrAttemptCoreLinkageV1) {
				t.Fatalf("wrong exact Core relation was accepted: %v", err)
			}
		})
	}
	for _, scenario := range []string{"orphan-no-key", "missing-key", "duplicate-pending", "duplicate-attempt", "valid-prefix-bad-later"} {
		t.Run(scenario, func(t *testing.T) {
			pending := pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{stage}}
			attempts := coreAttemptListV1{attempt}
			var authority finalauthorityport.Verifier = fixture.authority
			switch scenario {
			case "orphan-no-key":
				pending.Receipts, authority = nil, nil
			case "missing-key":
				authority = nil
			case "duplicate-pending":
				pending.Receipts = append(pending.Receipts, stage)
			case "duplicate-attempt":
				attempts = append(attempts, attempt)
			case "valid-prefix-bad-later":
				attempts = append(attempts, domainpublication.PublicationAttemptV1{AttemptID: "bad later attempt"})
			}
			if err := VerifyAttemptCoreInventoryV1(ctx, pending, attempts, authority); err == nil {
				t.Fatal("incomplete or ambiguous Core association succeeded")
			}
		})
	}
	if err := VerifyAttemptCoreInventoryV1(ctx, pendingworkapp.TrustedInventoryV1{}, coreAttemptListV1{}, nil); err != nil {
		t.Fatalf("empty complete inventory requested authority: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := VerifyAttemptCoreInventoryV1(cancelled, pendingworkapp.TrustedInventoryV1{}, coreAttemptListV1{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inventory returned %v", err)
	}
}

type coreAttemptListV1 []domainpublication.PublicationAttemptV1

func (attempts coreAttemptListV1) VisitAttempts(_ context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	for _, attempt := range attempts {
		if err := visit(attempt); err != nil {
			return err
		}
	}
	return nil
}
