package control

import (
	"context"
	"sync"
	"testing"
)

func userInputAdmissionFixtureV1(t *testing.T) (*GateRegistry[int], *TurnExecutionOwnerV1, GateRecord) {
	t.Helper()
	c := NewController(nil)
	_, owner := registerTurnExecutionOwnerTestV1(t, c, context.Background(), "thread", "turn", func() {})
	return NewGateRegistry[int](), owner, GateRecord{ThreadID: "thread", TurnID: "turn", ItemID: "item"}
}

func TestUserInputAdmissionV1PreservesBindingThroughActivationAndExactRetry(t *testing.T) {
	r, owner, record := userInputAdmissionFixtureV1(t)
	if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) {
		t.Fatal("reserve")
	}
	before, exists, candidate := r.PeekUserInputAdmissionV1("input")
	if !exists || !candidate || before.Producer() != owner {
		t.Fatal("reservation did not retain its actual producer")
	}
	if _, _, _, active := r.PeekUserInput("input"); active {
		t.Fatal("admission made reservation executable")
	}
	if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) || !r.ActivateReservedUserInput("input", 7) {
		t.Fatal("exact retry or activation failed")
	}
	after, _, _ := r.PeekUserInputAdmissionV1("input")
	if before.binding != after.binding {
		t.Fatal("retry/activation replaced producer binding")
	}
	if _, ok := r.ClaimUserInputExact("input", record, 7); ok {
		t.Fatal("a response without the captured admission claimed an owned gate")
	}
	if _, ok := r.ClaimUserInputAdmissionV1("input", before); !ok {
		t.Fatal("exact reservation observation could not claim after activation")
	}
	if len(r.inputProducers) != 0 {
		t.Fatal("claim retained producer binding")
	}
}

func TestUserInputAdmissionV1RejectsReplacementEvenForSameProducerAndBytes(t *testing.T) {
	r, owner, record := userInputAdmissionFixtureV1(t)
	if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) {
		t.Fatal("reserve")
	}
	stale, _, _ := r.PeekUserInputAdmissionV1("input")
	r.DeleteUserInput("input")
	if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) || !r.ActivateReservedUserInput("input", 7) {
		t.Fatal("replacement")
	}
	if _, ok := r.ClaimUserInputAdmissionV1("input", stale); ok {
		t.Fatal("old waiter claimed a recreated gate")
	}
	fresh, _, _ := r.PeekUserInputAdmissionV1("input")
	if fresh.binding == stale.binding {
		t.Fatal("recreated gate reused the old binding instance")
	}
	if _, ok := r.ClaimUserInputAdmissionV1("input", fresh); !ok {
		t.Fatal("stale attempt consumed replacement")
	}
}

func TestUserInputAdmissionV1RejectsForeignOrUnprovenProducer(t *testing.T) {
	r, owner, record := userInputAdmissionFixtureV1(t)
	foreign := record
	foreign.TurnID = "other"
	if r.ReservePendingUserInputOwnedV1("input", foreign, 7, owner) || r.ReservePendingUserInputOwnedV1("input", record, 7, nil) {
		t.Fatal("unproven producer reserved an owned gate")
	}
	if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) {
		t.Fatal("reserve")
	}
	if r.ReservePendingUserInput("input", record, 7) {
		t.Fatal("unowned retry stripped producer binding")
	}
}

func TestUserInputAdmissionV1ConcurrentResponsesHaveOneWinner(t *testing.T) {
	r, owner, record := userInputAdmissionFixtureV1(t)
	if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) || !r.ActivateReservedUserInput("input", 7) {
		t.Fatal("activate")
	}
	admission, _, _ := r.PeekUserInputAdmissionV1("input")
	start, results := make(chan struct{}), make(chan bool, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			_, ok := r.ClaimUserInputAdmissionV1("input", admission)
			results <- ok
		}()
	}
	ready.Wait()
	close(start)
	if first, second := <-results, <-results; first == second {
		t.Fatal("duplicate responses did not have exactly one winner")
	}
}

func TestUserInputAdmissionV1CleanupFollowsGateLifecycle(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, operation := range []string{"delete", "resolve", "turn", "thread", "all"} {
			if operation == "resolve" && !active {
				continue
			}
			t.Run(operation+map[bool]string{false: "/reserved", true: "/active"}[active], func(t *testing.T) {
				r, owner, record := userInputAdmissionFixtureV1(t)
				if !r.ReservePendingUserInputOwnedV1("input", record, 7, owner) {
					t.Fatal("reserve")
				}
				if active && !r.ActivateReservedUserInput("input", 7) {
					t.Fatal("activate")
				}
				before, _, _ := r.PeekUserInputAdmissionV1("input")
				switch operation {
				case "delete":
					r.DeleteUserInput("input")
				case "resolve":
					r.ResolveUserInput("input")
				case "turn":
					r.DrainForTurn("thread", "turn")
				case "thread":
					r.DrainForThread("thread")
				case "all":
					r.DrainAll()
				}
				if len(r.inputProducers) != 0 {
					t.Fatal("settled gate retained producer binding")
				}
				if _, ok := r.ClaimUserInputAdmissionV1("input", before); ok {
					t.Fatal("settled admission remained claimable")
				}
			})
		}
	}
}
