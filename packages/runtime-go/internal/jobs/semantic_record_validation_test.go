package jobs

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSemanticChildRecordValidationRetainsRawPauseStatus(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartChildRun(StartRequest{ParentGoalID: "goal-independent", ParentThreadID: "thread-independent", Kind: "subagent", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadChildRunIdentitySnapshotV1(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"legacy_display", "invalid_pause", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			record := snapshot.Records[0]
			record.Name = "  historical display  "
			operationCtx := ctx
			if fault == "invalid_pause" {
				record.PauseState.Status = "invalid-pause"
			}
			if fault == "cancelled" {
				var cancel context.CancelFunc
				operationCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := record
			err := ValidateChildRunSemanticSourceV1(operationCtx, record, nil)
			if fault == "legacy_display" {
				if err != nil {
					t.Fatalf("valid legacy source rejected: %v", err)
				}
				if err := ValidateCommittedChildRunRecordV1(ctx, record, nil); err == nil {
					t.Fatal("legacy source became an already committed projection")
				}
			} else if err == nil {
				t.Fatal("semantic validation erased original invalid state")
			}
			if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("semantic validation lost cancellation: %v", err)
			}
			if !reflect.DeepEqual(before, record) {
				t.Fatal("semantic validation mutated its caller's raw source")
			}
		})
	}
}
