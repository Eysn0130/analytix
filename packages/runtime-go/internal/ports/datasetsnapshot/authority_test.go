package datasetsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type datasetSnapshotAuthorityPortStub struct {
	input ResolveInput
}

func TestResolvedSnapshotV2JSONPreservesV1AndBindsOnlySelectedProducer(t *testing.T) {
	legacy := ResolvedSnapshotV2{}
	gotLegacy, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	wantLegacy, err := json.Marshal(struct {
		Record               domainsecurity.DatasetSnapshotAuthorityRecordV2
		Manifest             domainsecurity.DatasetSnapshotManifestV2
		FundsProducerContent domainsecurity.FundsProducerContentManifestV1
	}{})
	if err != nil || !bytes.Equal(gotLegacy, wantLegacy) {
		t.Fatalf("legacy resolved snapshot JSON changed: got=%s want=%s err=%v", gotLegacy, wantLegacy, err)
	}

	selectedV2 := ResolvedSnapshotV2{
		Manifest: domainsecurity.DatasetSnapshotManifestV2{
			ProducerContentContract: domainsecurity.FundsProducerContentManifestContractV2,
		},
	}
	gotV2, err := json.Marshal(selectedV2)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(gotV2, &fields); err != nil {
		t.Fatal(err)
	}
	if _, found := fields["FundsProducerContentV2"]; !found {
		t.Fatal("V2 resolved snapshot JSON omitted the selected producer")
	}
	if _, found := fields["FundsProducerContent"]; found {
		t.Fatal("V2 resolved snapshot JSON serialized the unselected V1 producer")
	}
}

func (stub *datasetSnapshotAuthorityPortStub) ResolveWitnessed(_ context.Context, input ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	stub.input = input
	return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, ErrUnavailable
}

func TestAuthorityUsesExactBindingObservationAndHasTypedFailureClasses(t *testing.T) {
	var authority Authority = &datasetSnapshotAuthorityPortStub{}
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/case-a", State: domainsecurity.CaseBindingStateValid, CaseID: "case-a",
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("binding-file")),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-canonical")),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = authority.ResolveWitnessed(context.Background(), ResolveInput{TenantID: "tenant-a", UserID: "user-a", Observation: observation})
	stub := authority.(*datasetSnapshotAuthorityPortStub)
	if !errors.Is(err, ErrUnavailable) || stub.input.TenantID != "tenant-a" || stub.input.UserID != "user-a" ||
		stub.input.Observation.ObservationDigest != observation.ObservationDigest {
		t.Fatalf("authority did not receive the exact binding scope: input=%+v err=%v", stub.input, err)
	}

	errorsByClass := []error{ErrUnavailable, ErrCorrupt, ErrMismatch, ErrStale}
	for index, sentinel := range errorsByClass {
		wrapped := fmt.Errorf("resolve current: %w", sentinel)
		if !errors.Is(wrapped, sentinel) {
			t.Fatalf("typed error class %d did not survive wrapping", index)
		}
		for otherIndex, other := range errorsByClass {
			if index != otherIndex && errors.Is(wrapped, other) {
				t.Fatalf("typed error classes %d and %d collapsed", index, otherIndex)
			}
		}
	}
}
