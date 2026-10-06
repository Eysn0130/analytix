package toolresultsnapshot

import (
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	"context"
	"reflect"
	"strings"
	"testing"
)

type localSnapshotFixtureStore struct {
	snapshot domaintoolresult.ProtectedSnapshotV1
	order    *[]string
}

func (s localSnapshotFixtureStore) Put(context.Context, domaintoolresult.ProtectedSnapshotV1) (domaintoolresult.ProtectedSnapshotBindingV1, error) {
	panic("read cannot write")
}
func (s localSnapshotFixtureStore) Read(context.Context, domaintoolresult.ProtectedSnapshotBindingV1) (domaintoolresult.ProtectedSnapshotV1, error) {
	*s.order = append(*s.order, "read")
	return s.snapshot, nil
}

type localSnapshotFixtureAccess struct {
	authorized      snapshotport.AuthorizedV1
	order           *[]string
	checks          int
	revokeAfterRead bool
}

func (a *localSnapshotFixtureAccess) AcquireCurrent(context.Context, snapshotport.SelectorV1) (snapshotport.AuthorizedV1, func(), error) {
	*a.order = append(*a.order, "acquire")
	return a.authorized, func() { *a.order = append(*a.order, "release") }, nil
}
func (a *localSnapshotFixtureAccess) ValidateCurrent(context.Context, snapshotport.AuthorizedV1) error {
	a.checks++
	*a.order = append(*a.order, "validate")
	if a.revokeAfterRead && a.checks == 2 {
		return snapshotport.ErrUnavailable
	}
	return nil
}
func TestProtectedSnapshotReadHoldsFreshAuthorityThroughRelease(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	resultID := domaintoolresult.ToolResultItemIDV1("turn-a", callID)
	snapshot := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal, Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: resultID, ToolName: "read", ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Capture: domaintoolresult.ProtectedCaptureV1{Kind: "read", Status: "completed", Body: "package main\r\n", Label: "code.go", StartLine: 1, EndLine: 1, TotalLines: 1}}
	binding, err := domaintoolresult.ProtectedSnapshotBindingForV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	selector := snapshotport.SelectorV1{ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: resultID}
	for _, revoke := range []bool{false, true} {
		order := []string{}
		access := &localSnapshotFixtureAccess{order: &order, revokeAfterRead: revoke, authorized: snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: snapshot.Workspace, ToolName: "read", ContextDigest: snapshot.ContextDigest, ContextEpoch: 1, ExecutionGrantID: snapshot.ExecutionGrantID, CaseBindingHash: snapshot.CaseBindingHash, Binding: binding}}
		service := &Service{Store: localSnapshotFixtureStore{snapshot, &order}, Authority: access}
		display, err := service.Read(context.Background(), selector)
		if revoke {
			if err == nil || display.Capture.Body != "" {
				t.Fatal("fresh revocation released private body")
			}
		} else if err != nil || display.Capture.Body != "package main\r\n" {
			t.Fatal("legitimate body was overfiltered")
		}
		if !reflect.DeepEqual(order, []string{"acquire", "validate", "read", "validate", "release"}) {
			t.Fatal("lease or fresh check order changed")
		}
	}
}
