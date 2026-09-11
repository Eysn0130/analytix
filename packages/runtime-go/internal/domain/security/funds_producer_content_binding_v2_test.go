package security

import "testing"

func TestResolveFundsProducerContentBindingV2IsClosedAndExact(t *testing.T) {
	input := datasetSnapshotManifestInputV2ForTest(t, datasetSnapshotAuthorityTestObservation(t), "producer-binding")
	manifestV1, err := NewDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	producerV1 := input.FundsProducerContentManifest
	bindingV1, err := ResolveFundsProducerContentBindingV2(
		manifestV1,
		producerV1,
		FundsProducerContentManifestV2{},
	)
	if err != nil || bindingV1.Contract != FundsProducerContentManifestContractV1 ||
		bindingV1.ID != manifestV1.ProducerContentID ||
		bindingV1.ManifestSHA256 != manifestV1.ProducerContentManifestSHA256 ||
		bindingV1.RawArtifactManifestSHA256 != manifestV1.RawArtifactManifestSHA256 ||
		bindingV1.SourceRowCount != manifestV1.SourceRecordCount {
		t.Fatalf("exact v1 producer binding failed: binding=%#v err=%v", bindingV1, err)
	}

	input.FundsProducerContentManifest = FundsProducerContentManifestV1{}
	producerV2, err := NewFundsProducerContentManifestV2(FundsProducerContentManifestInputV2{
		CaseID: input.Binding.CaseID, SourceRevision: 1,
		RawArtifactManifestSHA256: input.RawArtifactManifestSHA256,
		NormalizedContentSHA256:   SHA256Hex([]byte("producer-binding-v2-normalized")),
		DetailContentSHA256:       SHA256Hex([]byte("producer-binding-v2-detail")),
		SourceRowCount:            input.SourceRecordCount,
		AcceptedRowCount:          input.AcceptedRecordCount,
		RejectedRowCount:          input.RejectedRecordCount,
		DuplicateRowCount:         input.DuplicateRecordCount,
		DetailRowCount:            input.AcceptedRecordCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestV2, err := NewDatasetSnapshotManifestForFundsProducerContentV2(input, producerV2)
	if err != nil {
		t.Fatal(err)
	}
	bindingV2, err := ResolveFundsProducerContentBindingV2(
		manifestV2,
		FundsProducerContentManifestV1{},
		producerV2,
	)
	if err != nil || bindingV2.Contract != FundsProducerContentManifestContractV2 ||
		bindingV2.ID != manifestV2.ProducerContentID ||
		bindingV2.ManifestSHA256 != manifestV2.ProducerContentManifestSHA256 ||
		bindingV2.RawArtifactManifestSHA256 != manifestV2.RawArtifactManifestSHA256 ||
		bindingV2.DetailRowCount != manifestV2.AcceptedRecordCount {
		t.Fatalf("exact v2 producer binding failed: binding=%#v err=%v", bindingV2, err)
	}
	if _, err := ResolveFundsProducerContentBindingV2(
		manifestV2,
		producerV1,
		producerV2,
	); err == nil {
		t.Fatal("ambiguous producer-content union was accepted")
	}
	if _, err := ResolveFundsProducerContentBindingV2(
		manifestV1,
		FundsProducerContentManifestV1{},
		FundsProducerContentManifestV2{},
	); err == nil {
		t.Fatal("empty producer-content union was accepted")
	}
}
