package filestore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

func TestSafeCheckpointApplyPathAndMutationGuards(t *testing.T) {
	workspace := t.TempDir()
	if path, err := SafeCheckpointApplyPath(workspace, "a/b.txt"); err != nil || path != filepath.Join(workspace, "a", "b.txt") {
		t.Fatalf("safe path mismatch: path=%q err=%v", path, err)
	}
	if _, err := SafeCheckpointApplyPath(workspace, "../escape.txt"); err == nil {
		t.Fatal("escaping checkpoint path should be rejected")
	}
	if err := os.Mkdir(filepath.Join(workspace, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := CheckpointPathSafeForMutation(workspace, "dir", filepath.Join(workspace, "dir")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory mutation should be rejected: %v", err)
	}
}

func TestReadHashedUTF8FileAndApplyMutation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	content, err := ReadHashedUTF8File(path, 1024, func(value string) string { return "hash:" + value })
	if err != nil || content.Hash != "hash:hello" || content.Content != "hello" {
		t.Fatalf("read hash mismatch: content=%#v err=%v", content, err)
	}
	if err := ApplyCheckpointFileMutation(checkpointapp.ApplyFilePreflight{
		Action: "restore_deleted_file", Status: "apply", AbsolutePath: filepath.Join(workspace, "nested", "a.txt"),
		TargetContent: "old", BeforeHash: checkpointapp.Hash("old"),
	}); err != nil {
		t.Fatalf("restore mutation: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(workspace, "nested", "a.txt"))
	if err != nil || string(restored) != "old" {
		t.Fatalf("restored file mismatch: %q err=%v", restored, err)
	}
	if err := ApplyCheckpointFileMutation(checkpointapp.ApplyFilePreflight{
		Action: "delete_created_file", Status: "apply", AbsolutePath: path,
		CurrentHash: checkpointapp.Hash("hello"), AfterHash: checkpointapp.Hash("hello"),
	}); !errors.Is(err, ErrAtomicTextUnsupportedPlatform) {
		t.Fatalf("conditional delete must fail closed, got: %v", err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "hello" {
		t.Fatalf("fail-closed delete changed target: body=%q err=%v", body, err)
	}
}

func TestCreateCheckpointRescueRecord(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	if err := os.WriteFile(path, []byte("current"), 0o644); err != nil {
		t.Fatalf("write current: %v", err)
	}
	rescue := CreateCheckpointRescueRecord("thr_1", "cp_1", "plan_1", workspace, "now", []checkpointapp.ApplyFilePreflight{
		{RelativePath: "a.txt", AbsolutePath: path},
		{RelativePath: "missing.txt", AbsolutePath: filepath.Join(workspace, "missing.txt")},
	})
	files, _ := rescue["files"].([]any)
	if rescue["rescueId"] != checkpointapp.RescueID("cp_1") || len(files) != 2 {
		t.Fatalf("rescue record mismatch: %#v", rescue)
	}
	first, _ := files[0].(map[string]any)
	encoded, _ := first["bytesBase64"].(string)
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	if first["relativePath"] != "a.txt" || first["existed"] != true || string(decoded) != "current" || first["encoding"] != "utf8" {
		t.Fatalf("rescue current file mismatch: %#v", first)
	}
}

func TestCheckpointRescueRecordUsesPrivateAtomicSidecar(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	target := filepath.Join(workspace, "account.txt")
	const content = "private checkpoint rescue content"
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpointID := "axcp_private_1"
	rescueID := checkpointapp.RescueID(checkpointID)
	record := CreateCheckpointRescueRecord("thr_1", checkpointID, checkpointapp.PlanID(checkpointID), workspace, "2026-07-14T00:00:00Z", []checkpointapp.ApplyFilePreflight{{
		RelativePath: "account.txt", AbsolutePath: target,
	}})
	if err := WriteCheckpointRescueRecord(dataDir, "thr_1", checkpointID, record); err != nil {
		t.Fatalf("write private rescue: %v", err)
	}
	stored, err := ReadCheckpointRescueRecord(dataDir, "thr_1", checkpointID, rescueID)
	if err != nil {
		t.Fatalf("read private rescue: %v", err)
	}
	files, _ := stored["files"].([]any)
	file, _ := files[0].(map[string]any)
	encoded, _ := file["bytesBase64"].(string)
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	if string(decoded) != content || file["encoding"] != "utf8" {
		t.Fatalf("private rescue content mismatch: %#v", stored)
	}
	info, err := os.Stat(CheckpointRescuePath(dataDir, "thr_1", checkpointID, rescueID))
	if err != nil {
		t.Fatalf("stat private rescue: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("private rescue mode mismatch: %v", info.Mode().Perm())
	}
	if err := WriteCheckpointRescueRecord(dataDir, "thr_1", "axcp_other", record); err == nil {
		t.Fatal("mismatched private rescue identity must be rejected")
	}
}

func TestCheckpointRawUTF16RestoreAndAllowWriteAuthority(t *testing.T) {
	for _, encoding := range []string{
		filetoolsapp.TextEncodingUTF16LE,
		filetoolsapp.TextEncodingUTF16BE,
		filetoolsapp.TextEncodingUTF16LENoBOM,
		filetoolsapp.TextEncodingUTF16BENoBOM,
	} {
		t.Run(encoding, func(t *testing.T) {
			workspace := t.TempDir()
			allowRoot := t.TempDir()
			path := filepath.Join(allowRoot, "account.txt")
			beforeRaw := filetoolsapp.EncodeTextBytes("6222020200000000000\n", encoding)
			afterRaw := filetoolsapp.EncodeTextBytes("6222020200000000001\n", encoding)
			if err := os.WriteFile(path, afterRaw, 0o600); err != nil {
				t.Fatal(err)
			}
			root, err := WorkspaceRealPath(allowRoot)
			if err != nil {
				t.Fatal(err)
			}
			rootIdentity, err := checkpointRootIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			rootHash := checkpointAuthorityRootHash(root, rootIdentity)
			plan := map[string]any{
				"relativePath": "account.txt", "changeKind": "modified", "action": "restore_previous_version", "status": "ready",
				"beforeHash": checkpointapp.HashBytes(beforeRaw), "afterHash": checkpointapp.HashBytes(afterRaw),
				"pathAuthoritySchemaVersion": float64(1), "authorityKind": "allow_write", "authorityRootHash": rootHash,
			}
			snapshot := map[string]any{
				"relativePath": "account.txt", "pathAuthoritySchemaVersion": float64(1), "authorityKind": "allow_write",
				"authorityRoot": root, "authorityRootIdentity": rootIdentity, "authorityRootHash": rootHash,
				"before": map[string]any{
					"schemaVersion": float64(1), "encoding": encoding,
					"bytesBase64": base64.StdEncoding.EncodeToString(beforeRaw), "hash": checkpointapp.HashBytes(beforeRaw),
				},
			}
			preflight := PreflightCheckpointApplyFile(workspace, []string{allowRoot}, plan, snapshot, checkpointGitProbeStub{})
			expectedPath, err := WorkspaceRealPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if preflight.Status != "apply" || preflight.AbsolutePath != expectedPath || preflight.AuthorityKind != "allow_write" ||
				!bytesEqual(preflight.TargetBytes, beforeRaw) {
				t.Fatalf("external raw preflight mismatch: %#v", preflight)
			}
			if err := ApplyCheckpointFileMutation(preflight); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytesEqual(got, beforeRaw) {
				t.Fatalf("raw restore changed encoding/bytes: got=%x want=%x err=%v", got, beforeRaw, err)
			}

			publicPlan := BuildCheckpointRestoreFilePlan(workspace, []string{allowRoot}, map[string]any{
				"relativePath": "account.txt", "changeKind": "modified",
				"beforeHash": checkpointapp.HashBytes(beforeRaw), "afterHash": checkpointapp.HashBytes(afterRaw),
				"pathAuthoritySchemaVersion": float64(1), "authorityKind": "allow_write", "authorityRootHash": rootHash,
			})
			body, _ := json.Marshal(publicPlan)
			if strings.Contains(string(body), root) || strings.Contains(string(body), rootIdentity) || strings.Contains(string(body), "bytesBase64") {
				t.Fatalf("public rewind plan leaked private root/raw bytes: %s", body)
			}
			blocked := BuildCheckpointRestoreFilePlan(workspace, nil, map[string]any{
				"relativePath": "account.txt", "changeKind": "modified",
				"beforeHash": checkpointapp.HashBytes(beforeRaw), "afterHash": checkpointapp.HashBytes(afterRaw),
				"pathAuthoritySchemaVersion": float64(1), "authorityKind": "allow_write", "authorityRootHash": rootHash,
			})
			if blocked["status"] != "blocked" {
				t.Fatalf("removed allow_write authority did not block rewind plan: %#v", blocked)
			}
		})
	}
	legacy := BuildCheckpointRestoreFilePlan(t.TempDir(), nil, map[string]any{
		"relativePath": "legacy.txt", "changeKind": "modified",
		"beforeHash": checkpointapp.Hash("before"), "afterHash": checkpointapp.Hash("after"),
	})
	if legacy["status"] != "blocked" || !strings.Contains(legacy["reason"].(string), "legacy checkpoint path authority") {
		t.Fatalf("legacy ambiguous rewind record did not fail closed: %#v", legacy)
	}
}

func bytesEqual(left, right []byte) bool {
	return string(left) == string(right)
}

func TestCheckpointApplyRouteMismatch(t *testing.T) {
	if mismatch := CheckpointApplyRouteMismatch("thr_1", "cp_1", "/workspace", map[string]any{"threadId": "thr_2"}); mismatch != "rewind plan thread mismatch: thr_2" {
		t.Fatalf("thread mismatch not detected: %q", mismatch)
	}
	if mismatch := CheckpointApplyRouteMismatch("thr_1", "cp_1", "/workspace", map[string]any{"threadId": "thr_1", "checkpointId": "cp_1", "workspace": "/workspace/.", "applyMode": "plan_only", "destructive": false}); mismatch != "" {
		t.Fatalf("valid route should not mismatch: %q", mismatch)
	}
}
