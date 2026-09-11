package datasetsnapshot

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestOriginalDatasetInventoryValidatesEveryBranchWithoutSelectingOne(t *testing.T) {
	ctx := context.Background()
	fixture := newDatasetSnapshotStoreFixture(t)
	files := map[string]finalauthority.SecurePrivateCASOriginalEntryV1{}
	for _, name := range []string{".", legacyRecordsLeafV2, authorityBundlesLeafV2, indexesLeafV2, materialsLeafV2} {
		files[name] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: 0o700}
	}
	if records, err := OriginalFilesHaveRecordsV1(ctx, files); err != nil || records {
		t.Fatalf("complete empty owner: %v", err)
	}
	put := func(leaf, digest string, body []byte) {
		files[leaf+"/"+digest[:2]] = finalauthority.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: 0o700}
		files[leaf+"/"+digest[:2]+"/"+digest+".json"] = finalauthority.SecurePrivateCASOriginalEntryV1{Mode: 0o600, Body: body}
	}
	body, _ := domainsecurity.DatasetSnapshotAuthorityRecordV1Bytes(fixture.record)
	put(legacyRecordsLeafV2, fixture.record.RecordDigest, body)
	body, _ = domainsecurity.DatasetSnapshotIndexV1Bytes(fixture.index)
	put(indexesLeafV2, fixture.index.IndexDigest, body)
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(fixture.private, body), nil }
	a := fixture.record
	b, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: a.InstallationID, TenantID: a.TenantID, UserID: a.UserID, WorkspaceRealPath: a.WorkspaceRealPath,
		CaseID: a.CaseID, CaseBindingHash: a.CaseBindingHash, BindingObservationDigest: a.BindingObservationDigest,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("independent source B")), RawManifestSHA256: domainsecurity.SHA256Hex([]byte("independent raw B")),
		ParserVersion: a.ParserVersion, AcceptedAt: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), PredecessorRecordDigest: a.RecordDigest,
		AuthorityKeyID: a.AuthorityKeyID, AuthorityPublicKey: fixture.public,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = domainsecurity.DatasetSnapshotAuthorityRecordV1Bytes(b)
	put(legacyRecordsLeafV2, b.RecordDigest, body)
	makeIndex := func(previous domainsecurity.DatasetSnapshotIndexV1, record, mutation string) domainsecurity.DatasetSnapshotIndexV1 {
		index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
			InstallationID: a.InstallationID, EnrollmentID: fixture.enrollmentID, Generation: previous.Generation + 1, PreviousIndexDigest: previous.IndexDigest,
			MutationID: domainsecurity.SHA256Hex([]byte(mutation)), Binding: fixture.index.Binding, SnapshotRecordDigest: record,
			AuthorityKeyID: a.AuthorityKeyID, AuthorityPublicKey: fixture.public,
		}, sign)
		if err != nil {
			t.Fatal(err)
		}
		return index
	}
	ib := makeIndex(fixture.index, b.RecordDigest, "B branch one")
	ibSibling := makeIndex(fixture.index, b.RecordDigest, "B branch two")
	for _, index := range []domainsecurity.DatasetSnapshotIndexV1{ib, ibSibling} {
		body, _ = domainsecurity.DatasetSnapshotIndexV1Bytes(index)
		put(indexesLeafV2, index.IndexDigest, body)
	}
	parse := func() (OriginalInventoryV1, error) {
		return ParseOriginalInventoryV1(ctx, files, fixture.installationID, fixture.enrollmentID, a.AuthorityKeyID, fixture.public)
	}
	if inventory, err := parse(); err != nil || len(inventory.Indexes) != 3 {
		t.Fatalf("two valid historical branches: %v", err)
	}
	ic := makeIndex(ib, a.RecordDigest, "nonadjacent replay")
	if err := domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(ib, ic); err != nil {
		t.Fatalf("replay fixture did not pass adjacent-edge check: %v", err)
	}
	body, _ = domainsecurity.DatasetSnapshotIndexV1Bytes(ic)
	put(indexesLeafV2, ic.IndexDigest, body)
	if inventory, err := parse(); err == nil || inventory.Records != nil {
		t.Fatal("nonadjacent record replay produced a usable inventory")
	}
	delete(files, indexesLeafV2+"/"+ic.IndexDigest[:2]+"/"+ic.IndexDigest+".json")
	delete(files, legacyRecordsLeafV2+"/"+a.RecordDigest[:2]+"/"+a.RecordDigest+".json")
	if _, err := parse(); err == nil {
		t.Fatal("missing original predecessor accepted")
	}
}
