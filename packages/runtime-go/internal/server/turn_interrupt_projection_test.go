package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	controlapp "analytix.local/runtime-go/internal/app/control"
)

func TestInterruptThreadReadFailureUsesFixedPublicProjection(t *testing.T) {
	const pathSentinel = "customer-pii-13900000059-interrupt-thread-read"
	durableRoot := filepath.Join(t.TempDir(), pathSentinel)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	workspace := t.TempDir()
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "Interrupt read projection", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_13900000059"
	before, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	threadPath := handler.store.threadPath(threadID)
	if err := os.Remove(threadPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(threadPath, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := handler.interruptRuntimeTurn(context.Background(), controlapp.InterruptTurnRequest{
		ThreadID: threadID, TurnID: turnID, Discard: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 500 || stringField(result.Body, "code") != "security_context_invalid" ||
		stringField(result.Body, "message") != "turn security context is unavailable" {
		t.Fatalf("interrupt thread-read failure result = %#v", result)
	}
	body, err := json.Marshal(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{pathSentinel, threadID, turnID, "is a directory"} {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("interrupt thread-read failure leaked %q: %s", sentinel, body)
		}
	}
	after, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("interrupt thread-read failure changed event sequence: before=%#v after=%#v", before.Events, after.Events)
	}
	if handler.runtimeControl().TurnInterruptReserved(threadID, turnID) {
		t.Fatal("interrupt thread-read failure retained terminal authority")
	}
}
