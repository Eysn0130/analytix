package attachment

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestOwnerRecordV1SeparatesSameBlobAcrossUploadsAndBindsCase(t *testing.T) {
	observation := mustOwnerCaseObservation(t)
	base := OwnerRecordInputV1{
		OwnerNonce: "00000000000000000000000000000001", BlobSHA256: domainsecurity.SHA256Hex([]byte("blob")),
		ByteSize: 4, MIMEType: "text/plain", ThreadID: "thread-a", WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseBindingObservation: &observation, ProjectionSHA256: domainsecurity.SHA256Hex([]byte("projection")),
		CreatedAt: time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	}
	first, err := NewOwnerRecordV1(base)
	if err != nil {
		t.Fatal(err)
	}
	base.OwnerNonce = "00000000000000000000000000000002"
	second, err := NewOwnerRecordV1(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.AttachmentID == second.AttachmentID || first.OwnerDigest == second.OwnerDigest ||
		first.CaseBindingObservation == nil || first.CaseBindingObservation.CaseBindingHash != observation.CaseBindingHash {
		t.Fatalf("attachment owner identity was not upload- and case-bound: first=%#v second=%#v", first, second)
	}
}

func TestOwnerRecordV1StrictParsingRejectsTamperAndUnknownFields(t *testing.T) {
	observation := mustOwnerCaseObservation(t)
	record, err := NewOwnerRecordV1(OwnerRecordInputV1{
		OwnerNonce: "00000000000000000000000000000001", BlobSHA256: domainsecurity.SHA256Hex([]byte("blob")),
		ByteSize: 4, MIMEType: "text/plain", ThreadID: "thread-a", WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseBindingObservation: &observation, ProjectionSHA256: domainsecurity.SHA256Hex([]byte("projection")),
		CreatedAt: time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := OwnerRecordV1Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseOwnerRecordV1(body)
	if err != nil || parsed.OwnerDigest != record.OwnerDigest {
		t.Fatalf("valid owner record did not round-trip: parsed=%#v err=%v", parsed, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	raw["safeToAnswer"] = true
	tampered, _ := json.Marshal(raw)
	if _, err := ParseOwnerRecordV1(tampered); err == nil {
		t.Fatal("unknown owner field was accepted")
	}
	if _, err := ParseOwnerRecordV1(bytes.Replace(body, []byte("text/plain"), []byte("text/csv"), 1)); err == nil {
		t.Fatal("owner MIME tamper was accepted")
	}
}

func mustOwnerCaseObservation(t *testing.T) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/a", State: domainsecurity.CaseBindingStateValid, CaseID: "case-a",
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("binding-document")),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
