package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

func TestWorkspaceFileToolRunnerReadListAndGrep(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "notes.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	readOutput, rawContent, readErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace: workspace,
		Args:      map[string]any{"path": "notes.txt", "limit": 1},
		Mode:      filetoolsapp.ReadModeWindow,
	})
	if readErr || rawContent != "alpha" || readOutput["relative_path"] != "notes.txt" || readOutput["truncated"] != true {
		t.Fatalf("read output mismatch: output=%#v raw=%q isError=%v", readOutput, rawContent, readErr)
	}

	listOutput, listErr := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace: workspace,
		Args:      map[string]any{"path": "."},
	})
	listRecord, _ := listOutput.(map[string]any)
	if listErr || listRecord["relative_path"] != "." {
		t.Fatalf("list output mismatch: output=%#v isError=%v", listOutput, listErr)
	}

	grepOutput, grepErr := ExecuteGrepTool(WorkspaceSearchToolInput{
		Context:   context.Background(),
		Workspace: workspace,
		Args:      map[string]any{"pattern": "beta", "path": "."},
	})
	grepRecord, _ := grepOutput.(map[string]any)
	matches, _ := grepRecord["matches"].([]map[string]any)
	if grepErr || len(matches) != 1 {
		t.Fatalf("grep output mismatch: output=%#v isError=%v", grepOutput, grepErr)
	}
}

func TestWorkspaceFileToolRunnerRejectsProtectedAndEscapedPaths(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "private")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(protected, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write protected: %v", err)
	}

	protectedOutput, _, protectedErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace:         workspace,
		Args:              map[string]any{"path": "private/secret.txt"},
		ProtectedReadDirs: []string{protected},
		Mode:              filetoolsapp.ReadModeWindow,
	})
	if !protectedErr || protectedOutput["code"] != "protected_dir" {
		t.Fatalf("protected read mismatch: %#v isError=%v", protectedOutput, protectedErr)
	}

	escapedOutput, _, escapedErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace: workspace,
		Args:      map[string]any{"path": "../outside.txt"},
		Mode:      filetoolsapp.ReadModeWindow,
	})
	if !escapedErr || escapedOutput["code"] != "workspace_escape" {
		t.Fatalf("escaped read mismatch: %#v isError=%v", escapedOutput, escapedErr)
	}
}

func TestWorkspaceFileToolRunnerAllowsExternalReadsInFullAccess(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "desktop.txt")
	if err := os.WriteFile(outsideFile, []byte("external notes"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	readOutput, rawContent, readErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace:   workspace,
		Args:        map[string]any{"path": outsideFile},
		Mode:        filetoolsapp.ReadModeWindow,
		SandboxMode: "danger-full-access",
	})
	if readErr || rawContent != "external notes" || readOutput["path"] != outsideFile {
		t.Fatalf("full access external read mismatch: output=%#v raw=%q isError=%v", readOutput, rawContent, readErr)
	}

	listOutput, listErr := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace:   workspace,
		Args:        map[string]any{"path": outside},
		SandboxMode: "danger-full-access",
	})
	listRecord, _ := listOutput.(map[string]any)
	if listErr || listRecord["path"] != outside {
		t.Fatalf("full access external list mismatch: output=%#v isError=%v", listOutput, listErr)
	}

	grepOutput, grepErr := ExecuteGrepTool(WorkspaceSearchToolInput{
		Context:     context.Background(),
		Workspace:   workspace,
		Args:        map[string]any{"pattern": "external", "path": outside},
		SandboxMode: "danger-full-access",
	})
	grepRecord, _ := grepOutput.(map[string]any)
	matches, _ := grepRecord["matches"].([]map[string]any)
	if grepErr || len(matches) != 1 {
		t.Fatalf("full access external grep mismatch: output=%#v isError=%v", grepOutput, grepErr)
	}

	blockedOutput, _, blockedErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace:   workspace,
		Args:        map[string]any{"path": outsideFile},
		Mode:        filetoolsapp.ReadModeWindow,
		SandboxMode: "workspace-write",
	})
	hint, _ := blockedOutput["hint"].(string)
	if !blockedErr || blockedOutput["code"] != "workspace_escape" || hint == "" {
		t.Fatalf("workspace-write external read should include actionable escape output: %#v isError=%v", blockedOutput, blockedErr)
	}
	if strings.Contains(hint, "allow_write") {
		t.Fatalf("workspace-write external read hint should not point read tools at allow_write roots: %#v", blockedOutput)
	}
}

func TestWorkspaceFileToolRunnerAllowsConfiguredReadRoots(t *testing.T) {
	workspace := t.TempDir()
	readRoot := t.TempDir()
	outsideFile := filepath.Join(readRoot, "allowed.txt")
	if err := os.WriteFile(outsideFile, []byte("allowed notes"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	readOutput, rawContent, readErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace:   workspace,
		Args:        map[string]any{"path": outsideFile},
		ReadRoots:   []string{readRoot},
		Mode:        filetoolsapp.ReadModeWindow,
		SandboxMode: "workspace-write",
	})
	if readErr || rawContent != "allowed notes" || readOutput["path"] != outsideFile {
		t.Fatalf("configured read root output mismatch: output=%#v raw=%q isError=%v", readOutput, rawContent, readErr)
	}

	listOutput, listErr := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace:   workspace,
		Args:        map[string]any{"path": readRoot},
		ReadRoots:   []string{readRoot},
		SandboxMode: "workspace-write",
	})
	listRecord, _ := listOutput.(map[string]any)
	if listErr || listRecord["path"] != readRoot {
		t.Fatalf("configured read root list mismatch: output=%#v isError=%v", listOutput, listErr)
	}

	grepOutput, grepErr := ExecuteGrepTool(WorkspaceSearchToolInput{
		Context:     context.Background(),
		Workspace:   workspace,
		Args:        map[string]any{"pattern": "allowed", "path": readRoot},
		ReadRoots:   []string{readRoot},
		SandboxMode: "workspace-write",
	})
	grepRecord, _ := grepOutput.(map[string]any)
	matches, _ := grepRecord["matches"].([]map[string]any)
	if grepErr || len(matches) != 1 {
		t.Fatalf("configured read root grep mismatch: output=%#v isError=%v", grepOutput, grepErr)
	}

	filteredGrepOutput, filteredGrepErr := ExecuteGrepTool(WorkspaceSearchToolInput{
		Context:     context.Background(),
		Workspace:   workspace,
		Args:        map[string]any{"pattern": "allowed", "path": readRoot, "glob": "*.txt"},
		ReadRoots:   []string{readRoot},
		SandboxMode: "workspace-write",
	})
	filteredGrepRecord, _ := filteredGrepOutput.(map[string]any)
	filteredMatches, _ := filteredGrepRecord["matches"].([]map[string]any)
	if filteredGrepErr || len(filteredMatches) != 1 || filteredMatches[0]["relative_path"] != "allowed.txt" {
		t.Fatalf("configured read root grep glob should match relative to read root: output=%#v isError=%v", filteredGrepOutput, filteredGrepErr)
	}

	invalidGrepOutput, invalidGrepErr := ExecuteGrepTool(WorkspaceSearchToolInput{
		Context:     context.Background(),
		Workspace:   workspace,
		Args:        map[string]any{"pattern": "allowed", "path": readRoot, "glob": filepath.Join(readRoot, "*.txt")},
		ReadRoots:   []string{readRoot},
		SandboxMode: "workspace-write",
	})
	invalidGrepRecord, _ := invalidGrepOutput.(map[string]any)
	if !invalidGrepErr || invalidGrepRecord["error"] != "glob must be a relative pattern" {
		t.Fatalf("absolute grep glob should be rejected as a relative-pattern validation error: output=%#v isError=%v", invalidGrepOutput, invalidGrepErr)
	}
}

func TestWorkspaceFileToolRunnerAllowsExternalGlobPatterns(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	keep := filepath.Join(external, "report.pdf")
	if err := os.WriteFile(keep, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}
	if err := os.WriteFile(filepath.Join(external, "notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	output, isError := ExecuteGlobTool(WorkspaceSearchToolInput{
		Context:     context.Background(),
		Workspace:   workspace,
		Args:        map[string]any{"pattern": filepath.Join(external, "*.pdf")},
		SandboxMode: "danger-full-access",
	})
	record, _ := output.(map[string]any)
	matches, _ := record["matches"].([]map[string]any)
	if isError || record["path"] != external || record["normalized_pattern"] != "*.pdf" || len(matches) != 1 || matches[0]["path"] != keep {
		t.Fatalf("full access external glob mismatch: output=%#v isError=%v", output, isError)
	}

	alias := ExternalReadRootAlias(external)
	aliasOutput, aliasErr := ExecuteGlobTool(WorkspaceSearchToolInput{
		Context:     context.Background(),
		Workspace:   workspace,
		Args:        map[string]any{"pattern": alias + "/*.pdf"},
		ReadRoots:   []string{external},
		SandboxMode: "workspace-write",
	})
	aliasRecord, _ := aliasOutput.(map[string]any)
	aliasMatches, _ := aliasRecord["matches"].([]map[string]any)
	if aliasErr || aliasRecord["path"] != alias || len(aliasMatches) != 1 || aliasMatches[0]["path"] != alias+"/report.pdf" {
		t.Fatalf("alias external glob mismatch: output=%#v isError=%v", aliasOutput, aliasErr)
	}
}

func TestWorkspaceFileToolRunnerAllowsProtectedExternalReadsOnlyInFullAccess(t *testing.T) {
	workspace := t.TempDir()
	protected := t.TempDir()
	secret := filepath.Join(protected, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	output, rawContent, isError := ExecuteReadTextTool(ReadTextToolInput{
		Workspace:         workspace,
		Args:              map[string]any{"path": secret},
		ProtectedReadDirs: []string{protected},
		Mode:              filetoolsapp.ReadModeWindow,
		SandboxMode:       "danger-full-access",
	})
	if isError || rawContent != "secret" || output["path"] != secret {
		t.Fatalf("full access protected external read mismatch: %#v raw=%q isError=%v", output, rawContent, isError)
	}

	blockedOutput, _, blockedErr := ExecuteReadTextTool(ReadTextToolInput{
		Workspace:         workspace,
		Args:              map[string]any{"path": secret},
		ProtectedReadDirs: []string{protected},
		Mode:              filetoolsapp.ReadModeWindow,
		SandboxMode:       "workspace-write",
		ReadRoots:         []string{protected},
	})
	if !blockedErr || blockedOutput["code"] != "protected_dir" {
		t.Fatalf("workspace-write protected read should be blocked: %#v isError=%v", blockedOutput, blockedErr)
	}
}

func TestDangerFullAccessCannotReadMandatoryRuntimePrivateRoot(t *testing.T) {
	workspace := t.TempDir()
	protected := t.TempDir()
	secret := filepath.Join(protected, "final-answer-ed25519-v1.json")
	if err := os.WriteFile(secret, []byte("private-seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, rawContent, isError := ExecuteReadTextTool(ReadTextToolInput{
		Workspace: workspace, Args: map[string]any{"path": secret}, Mode: filetoolsapp.ReadModeWindow,
		ProtectedReadDirs: []string{MandatoryProtectedRoot(protected)}, SandboxMode: "danger-full-access",
	})
	if !isError || rawContent != "" || output["code"] != "protected_dir" {
		t.Fatalf("danger-full-access exposed mandatory runtime private material: output=%#v raw=%q error=%v", output, rawContent, isError)
	}
	listOutput, listError := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace: workspace, Args: map[string]any{"path": protected},
		ProtectedReadDirs: []string{MandatoryProtectedRoot(protected)}, SandboxMode: "danger-full-access",
	})
	listRecord, _ := listOutput.(map[string]any)
	if !listError || listRecord["code"] != "protected_dir" {
		t.Fatalf("danger-full-access listed mandatory runtime private root: output=%#v error=%v", listOutput, listError)
	}
}

func TestProviderFileToolsCannotReadListOrSearchWorkspaceHostMetadata(t *testing.T) {
	workspace := t.TempDir()
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	lookalike := filepath.Join(workspace, ".analytixsdd")
	if err := os.MkdirAll(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lookalike, 0o755); err != nil {
		t.Fatal(err)
	}
	const secret = "HOST_PRIVATE_BINDING_SENTINEL_43891"
	if err := os.WriteFile(filepath.Join(metadata, "host-secret.txt"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "host-secret.go"), []byte("package hidden\nfunc HostSecretSymbol() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadataAlias := filepath.Join(workspace, "metadata-alias")
	if err := os.Symlink(metadata, metadataAlias); err == nil {
		aliasOutput, aliasRaw, aliasError := ExecuteReadTextTool(ReadTextToolInput{
			Workspace: workspace, Args: map[string]any{"path": "metadata-alias/host-secret.txt"},
			Mode: filetoolsapp.ReadModeWindow, SandboxMode: "danger-full-access",
		})
		if !aliasError || aliasRaw != "" || aliasOutput["code"] != "host_metadata_protected" {
			t.Fatalf("symlink alias exposed metadata: output=%#v raw=%q error=%v", aliasOutput, aliasRaw, aliasError)
		}
	}
	if err := os.WriteFile(filepath.Join(lookalike, "visible.txt"), []byte("VISIBLE_LOOKALIKE_SENTINEL"), 0o644); err != nil {
		t.Fatal(err)
	}

	readOutput, raw, readError := ExecuteReadTextTool(ReadTextToolInput{
		Workspace: workspace, Args: map[string]any{"path": ".analytix/host-secret.txt"},
		Mode: filetoolsapp.ReadModeWindow, SandboxMode: "danger-full-access",
	})
	if !readError || raw != "" || readOutput["code"] != "host_metadata_protected" {
		t.Fatalf("direct metadata read escaped: output=%#v raw=%q error=%v", readOutput, raw, readError)
	}
	directList, directListError := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace: workspace, Args: map[string]any{"path": ".analytix"}, SandboxMode: "danger-full-access",
	})
	if record, _ := directList.(map[string]any); !directListError || record["code"] != "host_metadata_protected" {
		t.Fatalf("direct metadata list escaped: output=%#v error=%v", directList, directListError)
	}
	rootList, rootListError := ExecuteListTool(WorkspaceSearchToolInput{
		Workspace: workspace, Args: map[string]any{"path": "."}, SandboxMode: "danger-full-access",
	})
	rootBody, _ := json.Marshal(rootList)
	if rootListError || strings.Contains(string(rootBody), `"name":".analytix"`) || !strings.Contains(string(rootBody), ".analytixsdd") {
		t.Fatalf("workspace list did not hide exact metadata entry or overblocked lookalike: %s error=%v", rootBody, rootListError)
	}

	searches := []struct {
		name string
		run  func() (any, bool)
	}{
		{"find", func() (any, bool) {
			return ExecuteFindTool(WorkspaceSearchToolInput{Workspace: workspace, Args: map[string]any{"path": ".", "pattern": "*secret*"}, SandboxMode: "danger-full-access"})
		}},
		{"glob", func() (any, bool) {
			return ExecuteGlobTool(WorkspaceSearchToolInput{Context: context.Background(), Workspace: workspace, Args: map[string]any{"pattern": "**/*secret*"}, SandboxMode: "danger-full-access"})
		}},
		{"grep", func() (any, bool) {
			return ExecuteGrepTool(WorkspaceSearchToolInput{Context: context.Background(), Workspace: workspace, Args: map[string]any{"path": ".", "pattern": secret, "literal": true}, SandboxMode: "danger-full-access"})
		}},
		{"code_index", func() (any, bool) {
			return ExecuteCodeIndexTool(WorkspaceSearchToolInput{Context: context.Background(), Workspace: workspace, Args: map[string]any{"path": ".", "action": "outline"}, SandboxMode: "danger-full-access"})
		}},
	}
	for _, search := range searches {
		t.Run(search.name, func(t *testing.T) {
			output, isError := search.run()
			body, _ := json.Marshal(output)
			for _, forbidden := range []string{"host-secret.txt", "host-secret.go", "HostSecretSymbol"} {
				if strings.Contains(string(body), forbidden) {
					t.Fatalf("%s exposed host metadata %q: %s", search.name, forbidden, body)
				}
			}
			if isError {
				t.Fatalf("root search should prune metadata rather than fail: %s", body)
			}
		})
	}
}
