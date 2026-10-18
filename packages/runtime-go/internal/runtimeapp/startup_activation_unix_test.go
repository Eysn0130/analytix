//go:build darwin || linux

package runtimeapp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	startupapp "analytix.local/runtime-go/internal/app/startup"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestStartupPlanningPerformsZeroMCPProviderProcessNetworkActivation(t *testing.T) {
	var networkCalls atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		networkCalls.Add(1)
	}))
	defer endpoint.Close()
	spawned := filepath.Join(t.TempDir(), "spawned")
	providers, _ := json.Marshal(map[string]any{
		"defaultProviderId": "provider-test",
		"providers": []map[string]any{{
			"id": "provider-test", "apiKey": "test-key", "baseUrl": endpoint.URL + "/v1",
			"endpointFormat": "chat_completions", "models": []string{"model-test"},
		}},
	})
	mcp, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{
		"remote":  map[string]any{"transport": "http", "url": endpoint.URL + "/mcp", "trustScope": "user"},
		"process": map[string]any{"command": "/bin/sh", "args": []string{"-c", "printf spawned > " + spawned}, "trustScope": "user"},
	}})
	config := Config{
		RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir(),
		ModelProvidersJSON: string(providers), MCPConfigJSON: string(mcp),
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease); err != nil {
		t.Fatalf("prepare runtime startup: %v", err)
	}
	if networkCalls.Load() != 0 {
		t.Fatalf("semantic planning activated provider or MCP network calls: %d", networkCalls.Load())
	}
	if _, err := os.Lstat(spawned); !os.IsNotExist(err) {
		t.Fatalf("semantic planning spawned an MCP process: %v", err)
	}
}

// The classifier must not format even an unrecognized error implementation.
type startupFailurePoisonV1 struct{}

func (startupFailurePoisonV1) Error() string { panic("startup classifier formatted its cause") }

type startupFailureWrappedV1 struct{ cause error }

func (startupFailureWrappedV1) Error() string     { panic("startup classifier formatted its wrapper") }
func (err startupFailureWrappedV1) Unwrap() error { return err.cause }

func TestStartupPreparationFailureClassificationPreservesPrimaryAndIdentity(t *testing.T) {
	poison := startupFailurePoisonV1{}
	integrity := persistencefs.IntegrityError{Code: "/private/untrusted-code", Path: "/private/untrusted-path", Err: poison}
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"typed", integrity, "persistence_integrity"},
		{"wrapped", startupFailureWrappedV1{integrity}, "persistence_integrity"},
		{"resource", persistencefs.StartupResourceLimitError{Code: "/private", Limit: -1}, "startup_resource_limit"},
		{"migration", persistencefs.SemanticStartupMigrationBlocker{Code: "/private", Artifact: "/private"}, "semantic_migration_blocked"},
		{"snapshot", fmt.Errorf("context: %w", startupapp.ErrStartupSnapshotDrift), "startup_snapshot_drift"},
		{"cas", finalauthority.ErrSecurePrivateCASIntegrity, "private_cas_integrity"},
		{"cancel", context.Canceled, "cancelled"},
		{"deadline", context.DeadlineExceeded, "deadline"},
		{"unknown", poison, "unknown"},
		{"stage", wrapRuntimeStartupPreparationV1("provider-authority", poison), "provider_authority_unknown"},
		{"hostile stage", wrapRuntimeStartupPreparationV1("/private", poison), "unknown"},
		{"inner stage", wrapRuntimeStartupPreparationV1("semantic-plan", wrapRuntimeStartupPreparationV1("runtime-components", poison)), "runtime_components_unknown"},
		{"secondary known", joinRuntimeStartupCleanupV1(poison, integrity), "unknown"},
		{"plain secondary", errors.Join(poison, integrity), "unknown"},
		{"primary known", joinRuntimeStartupCleanupV1(integrity, poison), "persistence_integrity"},
		{"cleanup only", joinRuntimeStartupCleanupV1(nil, integrity), "cleanup"},
		{"nil", nil, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code := RuntimeStartupPreparationFailureCodeV1(tc.err); code != tc.code || !IsRuntimeStartupPreparationFailureCodeV1(code) {
				t.Fatalf("classification mismatch: code=%q want=%q", code, tc.code)
			}
		})
	}
	cause := errors.New("ordinary cause")
	wrapped := wrapRuntimeStartupPreparationV1("semantic-content-migration", joinRuntimeStartupCleanupV1(cause, integrity))
	var observed persistencefs.IntegrityError
	if !errors.Is(wrapped, cause) || !errors.As(wrapped, &observed) || observed.Path != integrity.Path {
		t.Fatal("startup metadata changed the underlying error identities")
	}
	compatible := wrapRuntimeStartupPreparationV1("provider-authority", fmt.Errorf("semantic startup phase runtime-composition: %w", cause))
	if compatible.Error() != "semantic startup phase runtime-composition: ordinary cause" ||
		RuntimeStartupPreparationFailureCodeV1(compatible) != "provider_authority_unknown" {
		t.Fatal("diagnostic stage changed the existing internal simulation error text")
	}
	for _, code := range []string{"", "/private", "persistence_integrity extra", "UNKNOWN"} {
		if IsRuntimeStartupPreparationFailureCodeV1(code) {
			t.Fatal("classifier accepted an unlisted diagnostic code")
		}
	}
}

func TestStartupPersistenceRecoverySubstageClassificationIsClosed(t *testing.T) {
	stages := []struct {
		stage string
		code  string
	}{
		{"persistence-authority-advance", "persistence_authority_advance_unknown"},
		{"persistence-report-preservation", "persistence_report_preservation_unknown"},
		{"persistence-final-history-bind", "persistence_final_history_bind_unknown"},
		{"persistence-retained-original", "persistence_retained_original_unknown"},
		{"persistence-journal-load", "persistence_journal_load_unknown"},
		{"persistence-child-revalidate", "persistence_child_revalidate_unknown"},
		{"persistence-authority-revalidate", "persistence_authority_revalidate_unknown"},
		{"persistence-signed-recovery-validation", "persistence_signed_recovery_validation_unknown"},
		{"persistence-create-residues", "persistence_create_residues_unknown"},
		{"persistence-semantic-recovery", "persistence_semantic_recovery_unknown"},
		{"persistence-orphan-topology", "persistence_orphan_topology_unknown"},
		{"persistence-owner-recovery", "persistence_owner_recovery_unknown"},
		{"persistence-checkpoint-recovery", "persistence_checkpoint_recovery_unknown"},
	}
	poison := startupFailurePoisonV1{}
	integrity := persistencefs.IntegrityError{Code: "/private/untrusted-code", Path: "/private/untrusted-path", Err: poison}
	for _, stage := range stages {
		t.Run(stage.stage, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				err  error
				code string
			}{
				{"unknown", poison, stage.code},
				{"typed", integrity, "persistence_integrity"},
				{"sentinel", context.Canceled, "cancelled"},
				{"primary unknown", joinRuntimeStartupCleanupV1(poison, integrity), stage.code},
				{"primary known", joinRuntimeStartupCleanupV1(integrity, poison), "persistence_integrity"},
				{"cleanup only", joinRuntimeStartupCleanupV1(nil, integrity), "cleanup"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					err := wrapRuntimeStartupPreparationV1(stage.stage, tc.err)
					if code := RuntimeStartupPreparationFailureCodeV1(err); code != tc.code || !IsRuntimeStartupPreparationFailureCodeV1(code) {
						t.Fatalf("classification mismatch: code=%q want=%q", code, tc.code)
					}
				})
			}
			cause := errors.New("ordinary cause")
			wrapped := wrapRuntimeStartupPreparationV1(stage.stage, cause)
			if wrapped.Error() != "ordinary cause" || !errors.Is(wrapped, cause) {
				t.Fatal("substage metadata changed the cause or internal error text")
			}
			var observed persistencefs.IntegrityError
			if !errors.As(wrapRuntimeStartupPreparationV1(stage.stage, joinRuntimeStartupCleanupV1(cause, integrity)), &observed) || observed.Path != integrity.Path {
				t.Fatal("substage metadata changed the secondary typed error identity")
			}
			for _, code := range []string{stage.code + " extra", "/private/" + stage.code, "go_prepare_failed_" + stage.code} {
				if IsRuntimeStartupPreparationFailureCodeV1(code) {
					t.Fatal("classifier accepted an unlisted substage code")
				}
			}
		})
	}
	if code := RuntimeStartupPreparationFailureCodeV1(wrapRuntimeStartupPreparationV1("persistence-recovery", poison)); code != "persistence_recovery_unknown" {
		t.Fatal("legacy persistence recovery fallback changed")
	}
}

func TestStartupPreparationMetadataPreservesExistingSimulationEarlyErrors(t *testing.T) {
	for _, simulation := range []bool{false, true} {
		_, err := newRuntimeServerHandlerWithRootsModeE(context.Background(),
			Config{DevelopmentProviderAuthorityDir: "relative"}, persistencefs.RootSet{},
			nil, nil, nil, true, simulation, "", "", nil, false, nil, nil)
		if err == nil || err.Error() != "development Provider authority is unavailable" ||
			RuntimeStartupPreparationFailureCodeV1(err) != "configuration_unknown" {
			t.Fatal("startup metadata changed the original pre-simulation validator error")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newRuntimeServerHandlerWithRootsModeE(ctx, Config{}, persistencefs.RootSet{},
		nil, nil, nil, false, true, "", "", nil, false, nil, nil)
	if !errors.Is(err, context.Canceled) || err.Error() != "semantic startup phase configuration: context canceled" {
		t.Fatal("startup metadata changed the original simulation cancellation error")
	}
}

func TestStartupPreparationProviderStagePreservesLegacySimulationError(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
	if err := writeProviderRegistryCorruptionFixtureV1(filepath.Join(config.DataDir, "private", "provider-registry"), []byte("{}\n")); err != nil {
		t.Fatal("write existing invalid Registry fixture")
	}
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal("acquire isolated startup lease")
	}
	defer lease.Close()
	prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if prepared != nil {
		t.Fatal("invalid Registry unexpectedly prepared runtime startup")
	}
	if err == nil || !strings.Contains(err.Error(), "semantic startup phase runtime-composition: provider registry recovery failed") ||
		strings.Contains(err.Error(), "semantic startup phase provider-authority:") {
		t.Fatal("real Prepare changed the existing simulation Registry failure text")
	}
	if code := RuntimeStartupPreparationFailureCodeV1(err); code != "provider_authority_unknown" {
		t.Fatalf("real Prepare diagnostic code=%q", code)
	}
}

// This is a real runtimeapp lifecycle diagnostic, not a replacement Registry or
// native acceptance. The child uses the default Computer Use transport class
// (generic, eager stdio), but runs only this bounded synthetic protocol helper.
func TestRuntimeStartupOpaqueMCPManualCaptureLifecycle(t *testing.T) {
	const childMode = "ANALYTIX_STARTUP_OPAQUE_CHILD_V1"
	if root := os.Getenv(childMode); root != "" {
		runStartupOpaqueMCPChildV1(root)
		os.Exit(2)
	}
	if runtime.GOOS != "darwin" {
		t.Skip("production generic stdio containment requires macOS")
	}
	assets := os.Getenv("ANALYTIX_QA_DSH188_OFFICE_ASSETS")
	docx := os.Getenv("ANALYTIX_QA_DSH188_DOCX")
	if assets == "" || docx == "" {
		t.Skip("requires explicit qualified Office assets and the task-owned synthetic DOCX")
	}
	original, err := os.ReadFile(docx)
	if err != nil || len(original) != 8681 || fmt.Sprintf("%x", sha256.Sum256(original)) != "c69fc567230a5aa1e3606ad603d93d83ac9ae6acaa3e41b75b671a9989200c15" {
		t.Fatal("synthetic DOCX binding differs")
	}
	repository, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal("source root unavailable")
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal("source root is not canonical")
	}
	for _, deferred := range []bool{true, false} {
		name := "default_eager_stdio"
		if deferred {
			name = "deferred_then_connected_stdio"
		}
		t.Run(name, func(t *testing.T) {
			control, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal("control root unavailable")
			}
			fifo := filepath.Join(control, "lifecycle.fifo")
			if syscall.Mkfifo(fifo, 0600) != nil {
				t.Fatal("private lifecycle pipe unavailable")
			}
			fd, err := syscall.Open(fifo, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
			if err != nil {
				t.Fatal("private lifecycle reader unavailable")
			}
			defer syscall.Close(fd)
			// The helper owns the only writer and never closes it explicitly.
			// E followed by EOF therefore observes its self-exit, without using
			// Signal(0), reaping another owner's child, or claiming an exit status.
			waitLifecycle := func(want byte) {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					var b [1]byte
					n, readErr := syscall.Read(fd, b[:])
					if readErr != nil && !errors.Is(readErr, syscall.EAGAIN) && !errors.Is(readErr, syscall.EWOULDBLOCK) {
						t.Fatal("lifecycle pipe read failed")
					}
					if n == 1 {
						if b[0] != want || want == 0 {
							t.Fatal("unexpected lifecycle transition")
						}
						return
					}
					if n == 0 && readErr == nil && want == 0 {
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
				t.Fatal("lifecycle transition timed out")
			}
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal("isolated runtime root unavailable")
			}
			data := filepath.Join(home, "data")
			if os.Mkdir(data, 0700) != nil {
				t.Fatal("isolated data root unavailable")
			}
			mcpConfig, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
				"analytix-computer-use": map[string]any{
					"transport": "stdio", "command": os.Args[0],
					"args":       []string{"-test.run=^TestRuntimeStartupOpaqueMCPManualCaptureLifecycle$"},
					"env":        map[string]string{childMode: control, "ANALYTIX_COMPUTER_USE_MANAGED_BY": "analytix"},
					"trustScope": "user", "backgroundStart": deferred, "timeoutMs": 5000,
				},
			}})
			if err != nil {
				t.Fatal("synthetic MCP configuration unavailable")
			}
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: data, UserDataDir: t.TempDir(),
				DurableTempDir: t.TempDir(), DevelopmentPluginSourceRoot: repository,
				DevelopmentOfficeAssetRoot: assets, MCPConfigJSON: string(mcpConfig)}
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal("isolated startup lease unavailable")
			}
			defer lease.Close()
			prepared, err := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
			if err != nil {
				t.Fatalf("real startup preparation failed: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(control, "started")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("Prepare activated the synthetic MCP")
			}
			handler, err := prepared.Activate(config)
			if err != nil {
				t.Fatalf("real runtime activation failed: %v", err)
			}
			defer shutdownOwnedRuntimeHandler(t, handler)
			request := func(method, path string, body any, local bool, wantStatus int, output any) {
				t.Helper()
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatal("request encoding failed")
				}
				req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
				req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
				req.Header.Set("Content-Type", "application/json")
				if local {
					req.Header.Set("X-Analytix-Local-Display", "typed-v1")
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, req)
				if response.Code != wantStatus || json.Unmarshal(response.Body.Bytes(), output) != nil {
					t.Fatalf("bounded request failed: method=%s route=%s status=%d", method, path, response.Code)
				}
			}
			checkConnection := func(refresh, connected bool) {
				t.Helper()
				path := "/v1/runtime/tools"
				if refresh {
					path += "?refresh=true"
				}
				var reply struct {
					MCPServers []struct {
						ID              string `json:"id"`
						Connected       bool   `json:"connected"`
						BackgroundStart bool   `json:"backgroundStart"`
					} `json:"mcpServers"`
				}
				request(http.MethodGet, path, nil, false, http.StatusOK, &reply)
				if len(reply.MCPServers) != 1 || reply.MCPServers[0].ID != "analytix-computer-use" || reply.MCPServers[0].Connected != connected || reply.MCPServers[0].BackgroundStart != deferred {
					t.Fatal("real MCP connection classification differs")
				}
			}
			checkConnection(false, !deferred)
			if deferred {
				if _, err := os.Lstat(filepath.Join(control, "started")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("deferred activation spawned the helper")
				}
			} else {
				waitLifecycle('R')
			}
			workspace := workspacetest.New(t)
			path := filepath.Join(workspace, "Lifecycle.docx")
			if os.WriteFile(path, original, 0600) != nil {
				t.Fatal("isolated synthetic DOCX unavailable")
			}
			var thread struct {
				ID string `json:"id"`
			}
			request(http.MethodPost, "/v1/threads", map[string]any{"title": "Manual capture lifecycle", "workspace": workspace}, false, http.StatusCreated, &thread)
			if thread.ID == "" {
				t.Fatal("fresh primary thread unavailable")
			}
			type hostReply struct {
				OK       bool                  `json:"ok"`
				Packages []hostapp.PackageView `json:"packages"`
				Package  hostapp.PackageView   `json:"package"`
				Output   json.RawMessage       `json:"output"`
			}
			var listed hostReply
			request(http.MethodPost, "/v1/local-display/plugin-package-host", map[string]any{"action": "list"}, true, http.StatusOK, &listed)
			var view hostapp.PackageView
			for _, entry := range listed.Packages {
				if entry.PackageID == "analytix-documents" {
					view = entry
				}
			}
			if !listed.OK || !view.Materialized || view.GenerationID == "" {
				t.Fatal("real Documents materialization unavailable")
			}
			var enabled hostReply
			request(http.MethodPost, "/v1/local-display/plugin-package-host", map[string]any{"action": "setDesiredState", "packageId": view.PackageID, "generationId": view.GenerationID, "expectedRevision": view.ActivationRevision, "desiredState": "enabled"}, true, http.StatusOK, &enabled)
			view = enabled.Package
			if !enabled.OK || !view.Available {
				t.Fatal("real Documents adapter activation unavailable")
			}
			invoke := func(operation string, input any, output any) {
				t.Helper()
				var reply hostReply
				request(http.MethodPost, "/v1/local-display/plugin-package-host", map[string]any{"action": "invoke", "packageId": view.PackageID, "generationId": view.GenerationID, "expectedRevision": view.ActivationRevision, "contributionId": "workspace-editor", "operation": operation, "input": input}, true, http.StatusOK, &reply)
				if !reply.OK || json.Unmarshal(reply.Output, output) != nil {
					t.Fatal("protected PackageHost invocation failed")
				}
			}
			var opened struct {
				OK       bool              `json:"ok"`
				Document editingapp.Opened `json:"document"`
			}
			invoke("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}, &opened)
			if !opened.OK || opened.Document.SessionID == "" || opened.Document.Revision != fmt.Sprintf("%x", sha256.Sum256(original)) {
				t.Fatal("real Office session baseline differs")
			}
			defer func() {
				var reply struct {
					OK     bool `json:"ok"`
					Closed bool `json:"closed"`
				}
				invoke("close-object", map[string]any{"sessionId": opened.Document.SessionID}, &reply)
				if !reply.OK || !reply.Closed {
					t.Error("Office session cleanup failed")
				}
			}()
			checkManual := func(stage string) {
				t.Helper()
				trace, err := os.CreateTemp(control, "manual-phase-")
				if err != nil {
					t.Fatal("closed diagnostic capture unavailable")
				}
				var reply struct {
					OK     bool   `json:"ok"`
					Code   string `json:"code"`
					Manual struct {
						Version      int    `json:"version"`
						Origin       string `json:"origin"`
						ChangeID     string `json:"changeId"`
						BaseRevision string `json:"baseRevision"`
					} `json:"manual"`
				}
				func() {
					previous := os.Stderr
					os.Stderr = trace
					defer func() { os.Stderr = previous }()
					invoke("begin-manual-edit", map[string]any{"sessionId": opened.Document.SessionID, "threadId": thread.ID, "operationId": "runtime_manual_" + stage, "baseRevision": opened.Document.Revision}, &reply)
				}()
				if trace.Close() != nil {
					t.Fatal("closed diagnostic capture close failed")
				}
				frame, err := os.ReadFile(trace.Name())
				if err != nil {
					t.Fatal("closed diagnostic capture read failed")
				}

				if !reply.OK || reply.Manual.Version != 2 || reply.Manual.Origin != "manual" || len(reply.Manual.ChangeID) != 64 || reply.Manual.BaseRevision != opened.Document.Revision || len(frame) != 0 {
					t.Fatal("real manual capture failed")
				}
				var cancelled struct {
					OK       bool `json:"ok"`
					Recovery struct {
						Pending json.RawMessage `json:"pending"`
					} `json:"recovery"`
				}
				invoke("cancel-change", map[string]any{"sessionId": opened.Document.SessionID, "threadId": thread.ID, "changeId": reply.Manual.ChangeID, "baseRevision": reply.Manual.BaseRevision}, &cancelled)
				if !cancelled.OK || string(cancelled.Recovery.Pending) != "null" {
					t.Fatal("manual cancellation did not release its prepared change")
				}

				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, original) {
					t.Fatal("manual lifecycle changed the original DOCX")
				}
				t.Logf("MCP_MANUAL_LIFECYCLE stage=%s manualOK=true sourceUnchanged=true", stage)
			}
			if deferred {
				checkManual("unconnected")
				checkConnection(true, true)
				waitLifecycle('R')
			}
			checkManual("connected")
			var strictReply struct {
				OK   bool   `json:"ok"`
				Code string `json:"code"`
			}
			invoke("capture-selection", map[string]any{"sessionId": opened.Document.SessionID, "threadId": thread.ID, "selectionToken": "lifecycle_selection_001", "changeSequence": 0, "baseRevision": opened.Document.Revision, "text": "Ordinary discussion", "editable": true}, &strictReply)
			if strictReply.OK || strictReply.Code != "projection_unavailable" {
				t.Fatal("manual admission weakened strict Agent capture after opaque startup")
			}
			if os.WriteFile(filepath.Join(control, "exit"), []byte("exit\n"), 0600) != nil {
				t.Fatal("synthetic exit request failed")
			}
			waitLifecycle('E')
			waitLifecycle(0)
			checkManual("exited")
			t.Log("MCP_MANUAL_LIFECYCLE helperSelfExitEOF=true exitStatusObserved=false toolsExecuted=0 providerCalls=0 registryReplaced=false")
		})
	}
}

func runStartupOpaqueMCPChildV1(root string) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		os.Exit(2)
	}
	fifo, err := syscall.Open(filepath.Join(root, "lifecycle.fifo"), syscall.O_WRONLY, 0)
	if err != nil {
		os.Exit(2)
	}
	// Keep this raw sole writer open through os.Exit (no file finalizer);
	// its EOF is the parent's
	// lifecycle witness. No shell, descendants, network or tool dispatch.
	if os.WriteFile(filepath.Join(root, "started"), []byte("started\n"), 0600) != nil {
		os.Exit(2)
	}
	go func() {
		deadline := time.NewTimer(45 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline.C:
				os.Exit(3)
			case <-ticker.C:
				if _, err := os.Lstat(filepath.Join(root, "exit")); err == nil {
					if _, err := syscall.Write(fifo, []byte{'E'}); err != nil {
						os.Exit(2)
					}
					os.Exit(0)
				}
			}
		}
	}()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		if len(request.ID) == 0 {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
		switch request.Method {
		case "initialize":
			response["result"] = map[string]any{"protocolVersion": mcpprotocol.ProtocolVersion, "serverInfo": map[string]any{"name": "startup-opaque-fixture", "version": "1.0.0"}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			response["result"] = map[string]any{"tools": []any{}}
		default:
			os.Exit(2)
		}
		if encoder.Encode(response) != nil {
			os.Exit(2)
		}
		if request.Method == "tools/list" {
			if _, err := syscall.Write(fifo, []byte{'R'}); err != nil {
				os.Exit(2)
			}
		}
	}
	os.Exit(2)
}
