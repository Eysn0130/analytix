package jobs

import (
	"reflect"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestPendingSteerTurnBindingPreservesOriginalQueueAndRequiresExactCAS(t *testing.T) {
	for _, mode := range []string{"exact", "changed_content", "changed_record", "pending_authority", "wrong_turn"} {
		t.Run(mode, func(t *testing.T) {
			manager, record := pendingSteerStartFixtureV1(t)
			record = queuePendingStartSteerV1(t, manager, record, "steer-original")
			original := record.Steers[0]
			expected := original
			authority, err := domainjob.NewSteerQueueAuthorityV1(record, strings.Repeat("b", 64), false)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "changed_content":
				expected.Text = "changed guidance"
			case "changed_record":
				_, err = manager.UpdateChildRun(record.ID, UpdateRequest{Status: "killed"})
				if err != nil {
					t.Fatal(err)
				}
			case "pending_authority":
				authority.PendingUntilChildTurn = true
			case "wrong_turn":
				authority.ChildTurnID = "turn_99"
			}
			before, err := BuildChildRunInventoryV1(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			boundRecord, bound, err := manager.BindPendingSteerMessageForTurn(record.ID, authority, expected)
			if mode != "exact" {
				if err == nil {
					t.Fatal("invalid pending turn binding reached writer")
				}
				after, readErr := BuildChildRunInventoryV1(manager.root)
				if readErr != nil || !reflect.DeepEqual(before, after) {
					t.Fatal("rejected binding changed original job bytes")
				}
				return
			}
			if err != nil || bound.ContextDigest != authority.ContextDigest || boundRecord.Steers[0].ContextDigest != authority.ContextDigest {
				t.Fatalf("exact binding failed: %v", err)
			}
			bound.ContextDigest = ""
			if !reflect.DeepEqual(bound, original) {
				t.Fatal("turn binding rewrote original queued content or authority digest")
			}
			if _, _, err := manager.BindPendingSteerMessageForTurn(record.ID, authority, expected); err == nil {
				t.Fatal("stale binding replay rewrote the queue")
			}
		})
	}
}

func TestPendingSteerTurnBindingSettlesOriginalLogicalContent(t *testing.T) {
	manager, record := pendingSteerStartFixtureV1(t)
	record = queuePendingStartSteerV1(t, manager, record, "steer-original")
	original := record.Steers[0]
	authority, err := domainjob.NewSteerQueueAuthorityV1(record, strings.Repeat("b", 64), false)
	if err != nil {
		t.Fatal(err)
	}
	_, bound, err := manager.BindPendingSteerMessageForTurn(record.ID, authority, original)
	if err != nil {
		t.Fatal(err)
	}
	settlement := domainjob.SteerPromotionSettlementV1{Version: 1, PromotionCommitID: "steer_commit_" + strings.Repeat("c", 64), PromotionEntryID: "item_steer_" + strings.Repeat("d", 64), ContextDigest: bound.ContextDigest, PromotedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_, admitted, err := manager.SettleSteerPromotionExact(record.ID, bound, settlement)
	if err != nil || admitted.Status != "admitted" || admitted.ContentDigest != original.ContentDigest || admitted.LogicalEffect != original.LogicalEffect || admitted.Text != original.Text {
		t.Fatalf("original pending guidance failed exact promotion settlement: %v", err)
	}
}
