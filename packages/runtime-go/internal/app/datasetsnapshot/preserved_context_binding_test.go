package datasetsnapshot

import (
	"context"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRecordMatchesPreservedContextsRequiresExactSnapshotAndBinding(t *testing.T) {
	fixture := newDatasetSnapshotServiceFixture(t)
	observation := datasetSnapshotServiceObservation(t, "/workspace/preserved-case", "preserved-case", "preserved")
	record, err := fixture.service.Accept(context.Background(), datasetSnapshotAcceptInput(observation, "preserved", time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainsecurity.DatasetSnapshotBindingKeyFromRecordV1(record)
	if err != nil {
		t.Fatal(err)
	}
	frozen := domainsecurity.TurnSecurityContext{
		ThreadID: "held-thread", TenantID: binding.TenantID, UserID: binding.UserID,
		WorkspaceRealPath: binding.WorkspaceRealPath, CaseID: binding.CaseID,
		CaseBindingHash: binding.CaseBindingHash, DatasetSnapshotID: record.DatasetSnapshotID,
	}
	frozen.PublicationPolicy.BindingObservationDigest = binding.BindingObservationDigest
	versions := map[string]domainsecurity.VersionedDatasetSnapshotAuthorityRecord{
		"V1": {V1: &record},
		// This helper receives records after graph verification. This V2 value
		// isolates its binding decision from the separate signature verifier.
		"V2": {V2: &domainsecurity.DatasetSnapshotAuthorityRecordV2{Binding: binding, DatasetSnapshotID: record.DatasetSnapshotID}},
	}
	mutations := map[string]func(*domainsecurity.TurnSecurityContext){
		"tenant":    func(value *domainsecurity.TurnSecurityContext) { value.TenantID += "-other" },
		"user":      func(value *domainsecurity.TurnSecurityContext) { value.UserID += "-other" },
		"workspace": func(value *domainsecurity.TurnSecurityContext) { value.WorkspaceRealPath += "-other" },
		"case":      func(value *domainsecurity.TurnSecurityContext) { value.CaseID += "-other" },
		"case binding": func(value *domainsecurity.TurnSecurityContext) {
			value.CaseBindingHash = domainsecurity.SHA256Hex([]byte("other case binding"))
		},
		"observation": func(value *domainsecurity.TurnSecurityContext) {
			value.PublicationPolicy.BindingObservationDigest = domainsecurity.SHA256Hex([]byte("other observation"))
		},
		"snapshot": func(value *domainsecurity.TurnSecurityContext) { value.DatasetSnapshotID += "-other" },
	}
	for version, candidate := range versions {
		t.Run(version, func(t *testing.T) {
			if !RecordMatchesPreservedContextsV1(candidate, []domainsecurity.TurnSecurityContext{frozen}) {
				t.Fatal("exact preserved binding was not held")
			}
			if RecordMatchesPreservedContextsV1(candidate, nil) {
				t.Fatal("record was held without a preserved context")
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					other := frozen
					mutate(&other)
					if RecordMatchesPreservedContextsV1(candidate, []domainsecurity.TurnSecurityContext{other}) {
						t.Fatal("different context was treated as the preserved binding")
					}
					if !RecordMatchesPreservedContextsV1(candidate, []domainsecurity.TurnSecurityContext{other, frozen}) {
						t.Fatal("unrelated context hid the exact preserved binding")
					}
				})
			}
		})
	}
	if RecordMatchesPreservedContextsV1(domainsecurity.VersionedDatasetSnapshotAuthorityRecord{}, []domainsecurity.TurnSecurityContext{frozen}) {
		t.Fatal("missing record acquired a preserved binding")
	}
}
