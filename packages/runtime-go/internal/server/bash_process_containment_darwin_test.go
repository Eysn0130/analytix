//go:build darwin

package server

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	processadapter "analytix.local/runtime-go/internal/adapters/outbound/process"
)

func TestDangerFullAccessBashRetainsMandatoryProtectedRootsAndOrdinaryWorkspaceAccess(t *testing.T) {
	firstProtectedRoot := t.TempDir()
	secondProtectedRoot := t.TempDir()
	workspace := t.TempDir()
	protectedFile := filepath.Join(secondProtectedRoot, "sentinel.txt")
	if err := os.WriteFile(protectedFile, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := &runtimeServerHandler{
		shellRunner: processadapter.NewShellRunner(),
		protectedReadDirs: []string{
			filestore.MandatoryProtectedRoot(firstProtectedRoot),
			filestore.MandatoryProtectedRoot(secondProtectedRoot),
		},
	}
	blockedArgs := map[string]any{"command": "/bin/cat " + strconv.Quote(protectedFile)}
	blocked, isError := handler.executeBashRuntimeTool(
		context.Background(),
		runtimeBashPendingForTest(t, workspace, blockedArgs),
		blockedArgs,
	)
	blockedBody, ok := blocked.(map[string]any)
	if !isError || !ok || blockedBody["status"] != "failed" ||
		strings.Contains(stringField(blockedBody, "output"), "must-not-escape") ||
		strings.Contains(stringField(blockedBody, "error"), "must-not-escape") {
		t.Fatalf("danger-full-access bash escaped a mandatory protected root: payload=%#v isError=%t", blocked, isError)
	}

	ordinaryFile := filepath.Join(workspace, "ordinary.txt")
	ordinaryArgs := map[string]any{"command": "printf ordinary > " + strconv.Quote(ordinaryFile)}
	ordinary, isError := handler.executeBashRuntimeTool(
		context.Background(),
		runtimeBashPendingForTest(t, workspace, ordinaryArgs),
		ordinaryArgs,
	)
	if isError {
		t.Fatalf("mandatory protected roots disabled ordinary workspace bash: %#v", ordinary)
	}
	content, err := os.ReadFile(ordinaryFile)
	if err != nil || string(content) != "ordinary" {
		t.Fatalf("ordinary workspace output mismatch: content=%q err=%v", content, err)
	}
}
