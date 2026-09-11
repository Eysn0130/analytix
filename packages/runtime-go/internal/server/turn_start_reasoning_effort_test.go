package server

import (
	"context"
	"strings"
	"testing"
)

func TestStartRuntimeTurnRejectsInvalidReasoningEffortBeforeRuntimeEffects(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	handler := &runtimeServerHandler{}
	for _, effort := range []string{" high ", "HIGH", sentinel} {
		if _, err := handler.startRuntimeTurn(context.Background(), "thread-1", startRuntimeTurnRequest{
			Prompt: "hello", ReasoningEffort: effort,
		}); err == nil || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("invalid reasoning effort was accepted or reflected: effort=%q err=%v", effort, err)
		}
		if handler.turnSeq != 0 || handler.control != nil {
			t.Fatalf("invalid reasoning effort mutated runtime state: turnSeq=%d control=%#v", handler.turnSeq, handler.control)
		}
	}
}
