package server

import (
	"context"
	"testing"
)

func TestChildTurnReservationsShareOrdinaryCounterAndRejectInvalidReuse(t *testing.T) {
	h := &runtimeServerHandler{}
	first, err := h.reserveRuntimeChildTurnV1(context.Background(), "thr_durable_1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.reserveRuntimeChildTurnV1(context.Background(), "thr_durable_1")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, sequence := h.nextRuntimeTurnIdentity()
	if first.turnID != "turn_1" || second.turnID != "turn_2" || ordinary != "turn_3" || sequence != 3 {
		t.Fatal("child and ordinary identity counters diverged")
	}
	if err := h.revalidateRuntimeChildTurnV1(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := (&runtimeServerHandler{}).revalidateRuntimeChildTurnV1(context.Background(), first); err == nil {
		t.Fatal("child reservation crossed handler identity")
	}
	copied := *first
	first.claim.consumed = true
	if err := h.revalidateRuntimeChildTurnV1(context.Background(), &copied); err == nil {
		t.Fatal("copied reservation lost shared consumption")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.reserveRuntimeChildTurnV1(cancelled, "thr_durable_1"); err == nil {
		t.Fatal("cancelled turn was allocated")
	}
	if h.turnSeq != 3 {
		t.Fatal("invalid allocation changed counter")
	}
}

func TestChildTurnReservationsRejectCounterExhaustion(t *testing.T) {
	for _, sequence := range []int{-1, int(^uint(0) >> 1)} {
		h := &runtimeServerHandler{turnSeq: sequence}
		if _, err := h.reserveRuntimeChildTurnV1(context.Background(), "thr_durable_1"); err == nil {
			t.Fatal("child counter wrapped")
		}
		if turnID, number := h.nextRuntimeTurnIdentity(); turnID != "" || number != 0 || h.turnSeq != sequence {
			t.Fatal("ordinary counter reused an exhausted child identity")
		}
	}
}

func TestRuntimeRestoreCannotLowerExistingChildTurnFloor(t *testing.T) {
	h := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir(), Host: "127.0.0.1"}).(*runtimeServerHandler)
	h.turnSeq = 700
	if err := h.restoreRuntimeState(); err != nil {
		t.Fatal(err)
	}
	if h.turnSeq != 700 {
		t.Fatal("runtime restore lowered the reserved turn floor")
	}
	id, sequence := h.nextRuntimeTurnIdentity()
	if id != "turn_701" || sequence != 701 {
		t.Fatal("ordinary turn reused a previously reserved identity")
	}
}
