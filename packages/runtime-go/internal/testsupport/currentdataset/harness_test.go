package currentdataset

import (
	"context"
	"strings"
	"testing"
	"time"

	fundsquerysourceapp "analytix.local/runtime-go/internal/app/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestHarnessUsesSignedAnalyticalSelectionAndProductionDescriptor(t *testing.T) {
	harness := NewHarness()
	securityContext, err := harness.NewCaseContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-current-dataset", TurnID: "turn-current-dataset",
		WorkspaceRealPath: "/cases/current-dataset", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-current-dataset", CaseBindingHash: strings.Repeat("a", 64),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("provisional"),
		SourceManifestHash: domainsecurity.SHA256Hex(
			[]byte("provisional-source-manifest"),
		),
		ContextEpoch: 1,
		IssuedAt:     time.Unix(1_700_000_000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.ValidateCurrent(context.Background(), securityContext); err != nil {
		t.Fatal(err)
	}
	observation, err := harness.Observe(securityContext.WorkspaceRealPath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := AccountIngressDescriptorV1(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	err = harness.WithCurrentSelectionV2(
		context.Background(),
		datasetsnapshotport.ResolveInputV2{
			TenantID: securityContext.TenantID, UserID: securityContext.UserID,
			Observation: observation, ExpectedDatasetSnapshotID: securityContext.DatasetSnapshotID,
		},
		securityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			if datasetsnapshotport.ValidateResolvedSnapshotV2(selection.Snapshot) != nil ||
				domainsecurity.ValidateDatasetSnapshotIndexRecordV2(
					selection.SelectedIndex,
					selection.Snapshot.Record,
				) != nil {
				t.Fatal("harness selection was not a signed DSV2/index graph")
			}
			return capability.UseExact(selection, securityContext, func(context.Context) error {
				got, descriptorErr := fundsquerysourceapp.DescriptorForCurrentSelectionV1(
					selection,
					securityContext,
					observation,
				)
				if descriptorErr != nil {
					return descriptorErr
				}
				if got != want {
					t.Fatalf("production descriptor drifted from harness descriptor: got=%#v want=%#v", got, want)
				}
				return nil
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}
