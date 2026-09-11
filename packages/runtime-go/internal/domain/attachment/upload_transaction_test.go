package attachment

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestUploadIntentAndDispositionRejectUnknownFieldsAndDigestMismatch(t *testing.T) {
	owner := uploadTransactionOwner(t)
	metadataDigest := domainsecurity.SHA256Hex([]byte("canonical metadata"))
	intent, err := NewUploadIntentV1(owner, metadataDigest)
	if err != nil {
		t.Fatal(err)
	}
	body, err := UploadIntentV1Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseUploadIntentV1(body); err != nil || parsed != intent {
		t.Fatalf("canonical upload intent did not round trip: parsed=%#v err=%v", parsed, err)
	}
	unknown := bytes.Replace(body, []byte(`"intentDigest"`), []byte(`"unknown":true,"intentDigest"`), 1)
	if _, err := ParseUploadIntentV1(unknown); err == nil {
		t.Fatal("unknown upload intent field was accepted")
	}
	tampered := intent
	tampered.MetadataSHA256 = domainsecurity.SHA256Hex([]byte("different"))
	if err := ValidateUploadIntentV1(tampered); err == nil {
		t.Fatal("upload intent digest mismatch was accepted")
	}

	disposition, err := NewUploadDispositionV1(
		intent, UploadDispositionCommittedV1, "upload_committed",
		intent.MetadataSHA256, intent.Owner.BlobSHA256, time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := UploadDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseUploadDispositionV1(dispositionBody); err != nil || parsed != disposition {
		t.Fatalf("canonical upload disposition did not round trip: parsed=%#v err=%v", parsed, err)
	}
	unknown = bytes.Replace(dispositionBody, []byte(`"recordDigest"`), []byte(`"unknown":true,"recordDigest"`), 1)
	if _, err := ParseUploadDispositionV1(unknown); err == nil {
		t.Fatal("unknown upload disposition field was accepted")
	}
	tamperedDisposition := disposition
	tamperedDisposition.ContentSHA256 = domainsecurity.SHA256Hex([]byte("different"))
	if err := ValidateUploadDispositionForIntentV1(tamperedDisposition, intent); err == nil {
		t.Fatal("committed disposition with mismatched content digest was accepted")
	}
}

func TestUploadDispositionStatusesAreClosedAndMutuallyBound(t *testing.T) {
	owner := uploadTransactionOwner(t)
	intent, err := NewUploadIntentV1(owner, domainsecurity.SHA256Hex([]byte("metadata")))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		status string
		reason string
		valid  bool
	}{
		{UploadDispositionCommittedV1, "upload_committed", true},
		{UploadDispositionRejectedV1, "upload_store_failed", true},
		{UploadDispositionQuarantinedV1, "restart_stale_upload_binding", true},
		{"supported", "upload_committed", false},
		{UploadDispositionCommittedV1, "restart_stale_upload_binding", false},
	} {
		metadataDigest, contentDigest := "", ""
		if test.status == UploadDispositionCommittedV1 {
			metadataDigest, contentDigest = intent.MetadataSHA256, intent.Owner.BlobSHA256
		}
		_, err := NewUploadDispositionV1(intent, test.status, test.reason, metadataDigest, contentDigest, time.Now().UTC())
		if (err == nil) != test.valid {
			t.Fatalf("status=%q reason=%q valid=%v err=%v", test.status, test.reason, test.valid, err)
		}
	}
}

func uploadTransactionOwner(t *testing.T) OwnerRecordV1 {
	t.Helper()
	owner, err := NewOwnerRecordV1(OwnerRecordInputV1{
		OwnerNonce: "00112233445566778899aabbccddeeff",
		BlobSHA256: domainsecurity.SHA256Hex([]byte("attachment body")), ByteSize: int64(len("attachment body")),
		MIMEType: "text/plain", ThreadID: "thread-a", WorkspaceRealPath: "/cases/a",
		ProjectionSHA256: domainsecurity.SHA256Hex([]byte("projection")), CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Prove the fixture remains canonical when its nested owner is serialized.
	if body, err := json.Marshal(owner); err != nil || len(body) == 0 {
		t.Fatalf("owner fixture is not serializable: %v", err)
	}
	return owner
}
