package caseentity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
)

// Subject labels in an external vector are not an additional account identity
// dimension. The original unsupported spelling stays rejected; a separately
// declared valid spelling shared by both subjects must resolve to one entity.
func TestDeliveryVectorSharedAccountDoesNotAcquireSubjectIdentity(t *testing.T) {
	body, err := os.ReadFile("../../../../../tools/analysis_compute/tests/fixtures/core-funds-20260921/funds-facts.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != "6894ac103a21260de3260aba39a49c15749ee2ec1db52842f5764abc6fc3cc82" {
		t.Fatal("original vector changed")
	}
	accounts := map[string]string{}
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var row map[string]string
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		if row["case_id"] == "SYNTH_CASE_A" && row["snapshot_id"] == "SYNTH_SNAPSHOT_A1" {
			accounts[row["subject_key"]] = row["raw_account"]
		}
	}
	if len(accounts) != 2 || accounts["SYNTH_SUBJECT_1"] == "" || accounts["SYNTH_SUBJECT_1"] != accounts["SYNTH_SUBJECT_2"] {
		t.Fatal("vector no longer contains the shared-account identity counterexample")
	}
	service := NewService(&recordingKeyedDigesterV1{key: []byte("synthetic-vector-identity-key")})
	securityContext := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "vector-thread", TurnID: "vector-turn", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "SYNTH_CASE_A", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-a", ContextEpoch: 1,
	})
	accountType := domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
	original := accounts["SYNTH_SUBJECT_1"]
	if _, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(securityContext, accountType, original)); err == nil {
		t.Fatal("unsupported original spelling was silently accepted")
	}
	// This explicit derived fixture uses the supported numeric account format.
	// The mapping is keyed only by raw account, never by external subject label.
	derived := map[string]string{original: "6222021234567890"}
	first := derived[accounts["SYNTH_SUBJECT_1"]]
	second := derived[accounts["SYNTH_SUBJECT_2"]]
	reference, err := service.DeriveReferenceV1(context.Background(), NewDeriveReferenceInputV1(securityContext, accountType, first))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := service.resolveReferenceExactV1(context.Background(), NewResolveReferenceInputV1(securityContext, accountType, reference, []string{first, second}))
	if err != nil || resolved != first {
		t.Fatal("same canonical account was split or became ambiguous because of external subject labels")
	}
}
