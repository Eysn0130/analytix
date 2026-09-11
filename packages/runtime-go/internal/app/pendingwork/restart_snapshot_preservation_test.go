package pendingwork

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type reportSnapshotVerificationOnlyV1 struct {
	authorityport.Authority
	signs int
}

func (authority *reportSnapshotVerificationOnlyV1) Sign(context.Context, []byte) ([]byte, error) {
	authority.signs++
	return nil, errors.New("snapshot planner cannot sign")
}

func TestReportRestartSnapshotBuildsSameHoldWithoutStoreOrSigning(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "unknown"}[unknown], func(t *testing.T) {
			ctx := context.Background()
			fixture := newServiceFixture(t)
			_, _ = beginReportRestartFixture(t, fixture)
			if unknown {
				if _, err := fixture.service.CloseAllOpenOnRestart(ctx, fixture.now.Add(time.Minute)); err != nil {
					t.Fatal(err)
				}
			}
			reader := reportRestartPrimaryFixture{threads: fixture.threads}
			live, err := fixture.service.PlanReportRestartPreservationV1(ctx, reader)
			if err != nil {
				t.Fatal(err)
			}
			receipts, dispositions, err := fixture.store.SnapshotInventory(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// The memory fixture returns shallow members; model the detached
			// copies supplied by the prepared filesystem snapshot.
			for i := range receipts {
				receipts[i] = clonePendingWorkReceipt(receipts[i])
			}
			before, _ := json.Marshal(map[string]any{"receipts": fixture.store.receipts, "dispositions": fixture.store.dispositions, "threads": fixture.threads.threads})
			verification := &reportSnapshotVerificationOnlyV1{Authority: fixture.authority}
			scope, err := PlanReportRestartPreservationSnapshotV1(ctx, receipts, dispositions, verification, reader)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(live.ThreadIDs(), scope.ThreadIDs()) || !reflect.DeepEqual(live.Contexts(), scope.Contexts()) || !reflect.DeepEqual(live.pendingDigests, scope.pendingDigests) {
				t.Fatal("prepared planner changed exact scope")
			}
			receipts[0].GrantMembers[0].GrantID = "caller mutation"
			if err := fixture.service.PreserveReportRestartScopeV1(ctx, scope); err != nil {
				t.Fatal(err)
			}
			var group sync.WaitGroup
			for i := 0; i < 4; i++ {
				group.Add(1)
				go func() {
					defer group.Done()
					if err := scope.RevalidatePrimary(ctx); err != nil {
						t.Error(err)
					}
				}()
			}
			group.Wait()
			if verification.signs != 0 {
				t.Fatal("prepared planner attempted signing")
			}
			after, _ := json.Marshal(map[string]any{"receipts": fixture.store.receipts, "dispositions": fixture.store.dispositions, "threads": fixture.threads.threads})
			if string(before) != string(after) {
				t.Fatal("prepared scope changed original authority graph")
			}
		})
	}
}

func TestReportRestartSnapshotVerifiesCompletedCoreGrantAndResult(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid_completed", true: "missing_completed_result"}[missing], func(t *testing.T) {
			ctx := context.Background()
			fixture := newServiceFixture(t)
			grant := fixture.addGrant(t, ReportStageToolName, false, "approved", fixture.now)
			receipt, err := fixture.service.Issue(ctx, fixture.issueInput(domainpendingwork.KindReportStage, []GrantReference{{GrantID: grant.GrantID}}, "report"))
			if err != nil {
				t.Fatal(err)
			}
			fixture.addResult(grant, false, "publication receipt committed", fixture.now.Add(4*time.Second))
			if _, err := fixture.service.Close(ctx, receipt.WorkID, fixture.securityContext, domainpendingwork.StatusCompleted, "report_stage_completed", fixture.now.Add(5*time.Second)); err != nil {
				t.Fatal(err)
			}
			receipts, dispositions, err := fixture.store.SnapshotInventory(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if missing {
				for _, raw := range fixture.threads.threads[fixture.securityContext.ThreadID]["turns"].([]any) {
					turn := raw.(map[string]any)
					if turn["id"] == fixture.securityContext.TurnID {
						turn["items"] = []any{}
					}
				}
			}
			before, _ := json.Marshal(fixture.threads.threads)
			scope, err := PlanReportRestartPreservationSnapshotV1(ctx, receipts, dispositions, fixture.authority, reportRestartPrimaryFixture{threads: fixture.threads})
			if missing != (err != nil) {
				t.Fatalf("completed Core authority validation: %v", err)
			}
			if len(scope.ThreadIDs()) != 0 {
				t.Fatal("completed report became an unresolved hold")
			}
			after, _ := json.Marshal(fixture.threads.threads)
			if string(before) != string(after) {
				t.Fatal("completed authority check repaired original primary")
			}
		})
	}
}

func TestReportRestartSnapshotRejectsSignatureAndPrimaryDrift(t *testing.T) {
	for _, fault := range []string{"signature", "primary_drift", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			fixture := newServiceFixture(t)
			_, _ = beginReportRestartFixture(t, fixture)
			ctx := context.Background()
			receipts, dispositions, err := fixture.store.SnapshotInventory(ctx)
			if err != nil {
				t.Fatal(err)
			}
			reader := reportRestartPrimaryFixture{threads: fixture.threads}
			switch fault {
			case "signature":
				receipts[0].AuthoritySignature = "invalid"
			case "primary_drift":
				reads := 0
				reader.beforeRead = func() {
					reads++
					if reads == 2 {
						fixture.threads.threads[fixture.securityContext.ThreadID]["title"] = "changed after first read"
					}
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			scope, err := PlanReportRestartPreservationSnapshotV1(ctx, receipts, dispositions, fixture.authority, reader)
			if err == nil || len(scope.ThreadIDs()) != 0 {
				t.Fatal("invalid snapshot produced a partial preservation scope")
			}
		})
	}
}
