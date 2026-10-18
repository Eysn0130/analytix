package server

import (
	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	"analytix.local/runtime-go/internal/adapters/outbound/documentcodec"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/managededitingfiles"
	"analytix.local/runtime-go/internal/adapters/outbound/officecodec"
	"analytix.local/runtime-go/internal/adapters/outbound/officeengineassets"
	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	docapp "analytix.local/runtime-go/internal/app/documentgeneration"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	officeeditingapp "analytix.local/runtime-go/internal/app/officeediting"
	materializationapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	riskapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	ordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/mcp"
	docport "analytix.local/runtime-go/internal/ports/documentgeneration"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
	"analytix.local/runtime-go/internal/provider"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type selectionObserver func(string) (domainsecurity.CaseBindingObservationV1, error)

func (observe selectionObserver) Observe(path string) (domainsecurity.CaseBindingObservationV1, error) {
	return observe(path)
}

func TestNativeSelectionCannotBypassInjectedWorkspaceObservation(t *testing.T) {
	p, _, scope := newSelectionProjectorFixture(t)
	calls := 0
	p.handler.turnSecurity.Observer = selectionObserver(func(string) (domainsecurity.CaseBindingObservationV1, error) {
		calls++
		return domainsecurity.CaseBindingObservationV1{}, errors.New("unavailable")
	})
	if p.ValidateCurrent(context.Background(), scope) == nil || calls != 1 {
		t.Fatal("selection bypassed the injected workspace observer")
	}
	p.handler.turnSecurity.Observer = nil
	if p.ValidateCurrent(context.Background(), scope) == nil {
		t.Fatal("selection inferred filesystem authority without an observer")
	}
}

// These observations delegate every decision to the production projector.
// They never retain input text, paths, principals, response bodies or error messages.
type dsh188OfficeStageProjector struct {
	runtimeObjectProjector
	observe       func(string)
	validateCalls int
}

func (p *dsh188OfficeStageProjector) ValidateCurrent(ctx context.Context, scope editingapp.ScopeAuthority) error {
	p.validateCalls++
	stage := "first_validate"
	if p.validateCalls > 1 {
		stage = "final_validate"
	}
	p.observe(stage + "_enter")
	err := p.runtimeObjectProjector.ValidateCurrent(ctx, scope)
	if err != nil {
		p.observe(stage + "_failed")
	} else {
		p.observe(stage + "_ok")
	}
	return err
}

func (p *dsh188OfficeStageProjector) AuthorizeAndProject(ctx context.Context, input editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	p.observe("authorize_project_enter")
	ranges, err := p.runtimeObjectProjector.AuthorizeAndProject(ctx, input)
	if err != nil {
		p.observe("authorize_project_failed")
	} else {
		p.observe("authorize_project_ok")
	}
	return ranges, err
}

func TestOfficeSelectionRealRuntimeAssemblySyntheticDocxV19(t *testing.T) {
	// This optional diagnostic is a controlled assembly case, not native UI acceptance.
	docxPath := os.Getenv("ANALYTIX_QA_DSH188_DOCX")
	assetRoot := os.Getenv("ANALYTIX_QA_DSH188_OFFICE_ASSETS")
	if docxPath == "" || assetRoot == "" {
		t.Skip("requires explicit task-owned synthetic DOCX and qualified Office assets")
	}
	const expectedDocxSHA = "c69fc567230a5aa1e3606ad603d93d83ac9ae6acaa3e41b75b671a9989200c15"
	const text = "DSH182 ORIGINAL sentence for native selection and replacement."
	before, err := os.ReadFile(docxPath)
	if err != nil {
		t.Fatal("synthetic_fixture_read_failed")
	}
	digest := sha256.Sum256(before)
	if len(before) != 8681 || hex.EncodeToString(digest[:]) != expectedDocxSHA {
		t.Fatal("synthetic_fixture_binding_mismatch")
	}
	archive, err := zip.NewReader(bytes.NewReader(before), int64(len(before)))
	if err != nil {
		t.Fatal("synthetic_fixture_package_invalid")
	}
	literalFound := false
	for _, entry := range archive.File {
		if entry.Name != "word/document.xml" {
			continue
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			t.Fatal("synthetic_fixture_xml_unavailable")
		}
		xml, readErr := io.ReadAll(io.LimitReader(reader, 64<<10))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal("synthetic_fixture_xml_read_failed")
		}
		literalFound = bytes.Contains(xml, []byte(text))
	}
	if !literalFound {
		t.Fatal("synthetic_original_literal_unconfirmed")
	}
	stages := []string{}
	observe := func(stage string) {
		if len(stages) >= 32 {
			t.Fatal("closed_stage_limit")
		}
		stages = append(stages, stage)
	}
	ranges, err := editingapp.ProtectedSelectionRanges(text)
	if err != nil {
		t.Fatal("synthetic_literal_projection_failed")
	}
	if len(ranges) == 0 {
		observe("synthetic_projection_empty")
	} else {
		observe("synthetic_projection_protected")
	}
	if ordinary.ProjectTextV1(text) == text {
		observe("synthetic_literal_fixedpoint")
	} else {
		observe("synthetic_literal_projected")
	}
	ctx := context.Background()
	p, thread, scope := newSelectionProjectorFixture(t)
	identity := p.handler.turnSecurity.Identity // Real InstallationLocalAuthority.
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal("real_general_risk_owner_unavailable")
	}
	p.handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{
		Identity: identity, Observer: filestore.CaseBindingReader{}, RiskAuthority: risk,
	}
	scope.Principal, err = identity.ResolveCurrent(ctx)
	if err != nil {
		t.Fatal("real_installation_identity_unavailable")
	}
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: p.handler.turnSecurity, Thread: thread,
		ThreadID: scope.ThreadID, TurnID: "office-selection-completed-turn",
		Workspace: scope.Workspace, Principal: scope.Principal, IssuedAt: time.Now().UTC(),
	})
	if err != nil || !domainsecurity.TurnSecurityContextIsGeneral(frozen) {
		t.Fatal("normal_general_freeze_failed")
	}
	record := turnsecurityapp.PublicRecord(frozen)
	if err := p.handler.store.AppendTurnToThread(scope.ThreadID, map[string]any{
		"id": frozen.TurnID, "threadId": scope.ThreadID, "status": "completed",
		"items": []any{}, "securityContext": record,
	}, "", map[string]any{"securityState": record}); err != nil {
		t.Fatal("normal_completed_turn_owner_failed")
	}
	path := filepath.Join(scope.Workspace, "dsh182-native-document.docx")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal("synthetic_fixture_copy_failed")
	}
	receiptRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("private_receipt_root_unavailable")
	}
	files, err := filestore.NewOfficeObjectEditingFiles(receiptRoot, nil, "docx")
	if err != nil {
		t.Fatal("real_office_file_owner_unavailable")
	}
	assets, err := officeengineassets.Open(ctx, assetRoot)
	if err != nil {
		t.Fatal("qualified_office_assets_unavailable")
	}
	if p.handler.workspaceMutations == nil {
		t.Fatal("shared_coordinator_unavailable")
	}
	registry := managededitingapp.New(p.handler.workspaceMutations, managededitingfiles.New())
	p.handler.managedEditing = registry
	projector := &dsh188OfficeStageProjector{runtimeObjectProjector: p, observe: observe}
	adapter := officeeditingapp.New("docx", editingapp.New(identity, files), assets.Current)
	captureCalls := 0
	if err := adapter.BindSelectionHost(projector, func(captureCtx context.Context, id, capturedPath string, readBaseline func() error) (func(), error) {
		captureCalls++
		observe("literal_render_passed") // This callback follows the real render guard.
		observe("with_capture_enter")
		release, captureErr := registry.WithCapture(captureCtx, id, capturedPath, func() error {
			observe("baseline_enter")
			baselineErr := readBaseline()
			if baselineErr != nil {
				observe("baseline_failed")
			} else {
				observe("baseline_ok")
			}
			return baselineErr
		})
		switch {
		case captureErr == nil:
			observe("with_capture_ok")
		case errors.Is(captureErr, managededitingapp.ErrUncontainedExecution):
			observe("with_capture_opaque")
		case errors.Is(captureErr, managededitingapp.ErrUnsafePath):
			observe("with_capture_unsafe_path")
		case errors.Is(captureErr, managededitingapp.ErrSessionMismatch):
			observe("with_capture_session_mismatch")
		case errors.Is(captureErr, managededitingapp.ErrCaptureFailed):
			observe("with_capture_baseline_failed")
		case errors.Is(captureErr, managededitingapp.ErrUnavailable):
			observe("with_capture_unavailable")
		default:
			observe("with_capture_other")
		}
		return release, captureErr
	}); err != nil {
		t.Fatal("real_selection_binding_failed")
	}
	type output struct {
		OK       bool              `json:"ok"`
		Code     string            `json:"code"`
		Document editingapp.Opened `json:"document"`
		Scope    json.RawMessage   `json:"scope"`
	}
	invoke := func(operation string, input map[string]any) output {
		t.Helper()
		body, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			t.Fatal("controlled_input_encoding_failed")
		}
		result, invokeErr := adapter.Invoke(ctx, adapterport.Call{
			Binding:   adapterport.Binding{PackageID: "analytix-documents"},
			Principal: scope.Principal, ContributionID: "workspace-editor",
			Operation: operation, Input: body,
			Admit: func() error { return identity.ValidateCurrent(ctx, scope.Principal) },
		})
		if invokeErr != nil {
			t.Fatalf("real_adapter_invoke_failed operation=%s stages=%v", operation, stages)
		}
		var out output
		if json.Unmarshal(result.Output, &out) != nil {
			t.Fatal("real_adapter_output_invalid")
		}
		return out
	}
	opened := invoke("open-object", map[string]any{"object": map[string]any{"workspace": scope.Workspace, "path": path}})
	if !opened.OK || opened.Document.SessionID == "" || opened.Document.Revision != expectedDocxSHA {
		t.Fatal("real_office_open_failed")
	}
	t.Cleanup(func() {
		closed := invoke("close-object", map[string]any{"sessionId": opened.Document.SessionID})
		if !closed.OK || registry.HasCaptures() {
			t.Error("real_capture_cleanup_failed")
		}
	})
	captured := invoke("capture-selection", map[string]any{
		"sessionId": opened.Document.SessionID, "threadId": scope.ThreadID,
		"selectionToken": "native_selection_01", "changeSequence": 0,
		"baseRevision": opened.Document.Revision, "text": text, "editable": true,
	})
	if !captured.OK {
		if captureCalls == 0 && strings.Contains(strings.Join(stages, ","), "authorize_project_ok") {
			observe("post_project_pre_capture_failed")
		}
		closedCode := "other"
		switch captured.Code {
		case "projection_unavailable", "invalid_request", "session_invalid", "draft_stale", "proposal_invalid", "unavailable":
			closedCode = captured.Code
		}
		t.Fatalf("capture_failed closed_code=%s stages=%v", closedCode, stages)
	}
	if len(captured.Scope) == 0 || captureCalls != 1 || projector.validateCalls != 2 || !registry.HasCaptures() {
		t.Fatalf("real_capture_completion_incomplete stages=%v", stages)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("controlled_original_changed")
	}
	t.Logf("CONTROL_ASSEMBLY_PASS stages=%v", stages)
}

// This task-owned diagnostic uses the current v8 synthetic inputs, independently
// of the historical V19 fixture. It does not claim native UI acceptance or a
// successful MCP catalog: cleanup has removed the original dependency link.
func TestOfficeSelectionCurrentV8SyntheticDocxEagerConnectAttempt(t *testing.T) {
	docxPath := os.Getenv("ANALYTIX_QA_DSH188_V8_DOCX")
	configPath := os.Getenv("ANALYTIX_QA_DSH188_V8_CONFIG")
	if docxPath == "" || configPath == "" {
		t.Skip("requires explicit current task-owned v8 synthetic inputs")
	}
	const fixtureSHA = "41515520e9b0c88b805f7878f6341027a2f95280d472cda2474cd38cdd751b33"
	const configSHA = "be5a63d98c38fb3ce8836e94a2740c80cfafb2637d61d4d4eddafd4404b467db"
	const selectedText = "This document co"
	const replacementText = "Updated document"
	before, err := os.ReadFile(docxPath)
	if err != nil {
		t.Fatal("current_synthetic_fixture_unavailable")
	}
	digest := sha256.Sum256(before)
	if len(before) != 2171 || hex.EncodeToString(digest[:]) != fixtureSHA {
		t.Fatal("current_synthetic_fixture_binding_mismatch")
	}
	archive, err := zip.NewReader(bytes.NewReader(before), int64(len(before)))
	if err != nil || len(archive.File) != 5 {
		t.Fatal("current_synthetic_docx_package_invalid")
	}
	var documentXML []byte
	for _, entry := range archive.File {
		if entry.Name != "word/document.xml" {
			continue
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			t.Fatal("current_synthetic_xml_unavailable")
		}
		documentXML, err = io.ReadAll(io.LimitReader(reader, 64<<10))
		closeErr := reader.Close()
		if err != nil || closeErr != nil || len(documentXML) >= 64<<10 {
			t.Fatal("current_synthetic_xml_read_failed")
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(documentXML))
	var plain strings.Builder
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			t.Fatal("current_synthetic_xml_invalid")
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" && start.Name.Local == "t" {
			var text string
			if decoder.DecodeElement(&text, &start) != nil {
				t.Fatal("current_synthetic_text_invalid")
			}
			plain.WriteString(text)
		}
	}
	if !strings.HasPrefix(plain.String(), "BEGIN_SUPERDOC_S") || !strings.Contains(plain.String(), "QA_SUPERDOC_V8_EDIT_1710") || strings.Count(plain.String(), selectedText) != 1 || bytes.Count(documentXML, []byte(selectedText)) != 1 {
		t.Fatal("current_synthetic_content_unconfirmed")
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal("current_task_mcp_configuration_unavailable")
	}
	configDigest := sha256.Sum256(configBytes)
	if hex.EncodeToString(configDigest[:]) != configSHA {
		t.Fatal("current_task_mcp_configuration_changed")
	}
	var enabled struct {
		Capabilities struct {
			MCP struct{ Enabled bool }
		}
	}
	if json.Unmarshal(configBytes, &enabled) != nil || !enabled.Capabilities.MCP.Enabled {
		t.Fatal("current_task_mcp_not_enabled")
	}
	specs, err := mcp.LoadMCPJSONDocument(configBytes, filepath.Dir(configPath))
	if err != nil || len(specs) != 2 {
		t.Fatal("current_task_mcp_sources_unconfirmed")
	}
	var actual []mcp.ServerSpec
	schedules := 0
	for _, spec := range specs {
		if spec.ID == "gui_schedule" {
			schedules++
		}
		if spec.ID == "analytix-computer-use" && spec.Transport == "stdio" && !spec.BackgroundStart && filepath.IsAbs(spec.Command) && spec.Command != "" && spec.AccountCredential == nil {
			actual = append(actual, spec)
		}
	}
	if len(actual) != 1 || schedules != 1 {
		t.Fatal("current_task_eager_source_type_unconfirmed")
	}
	// Preserve the actual spec. Never recreate dependency links, substitute a
	// missing command, or execute the installed target behind the removed link.
	assertCommandAbsent := func(t *testing.T) {
		t.Helper()
		if info, statErr := os.Lstat(actual[0].Command); info != nil || !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal("actual_command_not_verified_absent_no_connect_allowed")
		}
	}
	assertCommandAbsent(t)
	for _, phase := range []string{"without_eager_connect", "actual_eager_connect_attempt"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			p, threadBefore, scope := newSelectionProjectorFixture(t)
			identity := p.handler.turnSecurity.Identity
			risk, riskErr := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
			if riskErr != nil {
				t.Fatal("real_general_risk_owner_unavailable")
			}
			p.handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: identity, Observer: filestore.CaseBindingReader{}, RiskAuthority: risk}
			scope.Principal, err = identity.ResolveCurrent(ctx)
			if err != nil {
				t.Fatal("real_installation_identity_unavailable")
			}
			path := filepath.Join(scope.Workspace, "current-v8-synthetic.docx")
			if os.WriteFile(path, before, 0600) != nil {
				t.Fatal("current_synthetic_fixture_copy_failed")
			}
			receiptRoot, rootErr := filepath.EvalSymlinks(t.TempDir())
			if rootErr != nil {
				t.Fatal("private_receipt_root_unavailable")
			}
			files, filesErr := filestore.NewOfficeObjectEditingFiles(receiptRoot, nil, "docx")
			if filesErr != nil || p.handler.workspaceMutations == nil {
				t.Fatal("real_office_file_owner_unavailable")
			}
			registry := managededitingapp.New(p.handler.workspaceMutations, managededitingfiles.New())
			p.handler.managedEditing = registry
			stages := []string{}
			observe := func(stage string) {
				if len(stages) >= 64 {
					t.Fatal("closed_stage_limit")
				}
				stages = append(stages, stage)
			}
			// Schedule requires its private host binding and is not a generic
			// stdio control. Only the actual external source enters this manager.
			manager := mcp.NewProductionManagerWithOptions(actual, mcp.ProductionManagerOptions{})
			t.Cleanup(manager.Disconnect)
			admissions := 0
			if manager.BindEffectAdmission(func(admissionCtx context.Context) error {
				admissions++
				observe("actual_source_constructor_admission")
				return registry.BeginOpaque(admissionCtx)
			}) != nil {
				t.Fatal("actual_admission_binding_failed")
			}
			if phase == "actual_eager_connect_attempt" {
				assertCommandAbsent(t)
				manager.Connect()
				diagnostics := manager.ServerDiagnostics()
				if admissions != 1 || len(manager.LiveTools()) != 0 || len(diagnostics) != 1 {
					t.Fatal("actual_connect_attempt_boundary_unconfirmed")
				}
				state, ok := diagnostics[0].(map[string]any)
				if !ok || state["connected"] != false {
					t.Fatal("actual_catalog_must_not_be_claimed_connected")
				}
				observe("actual_source_not_connected")
			}
			// This is the DOCX production gate from runtimeapp/office_editing.go;
			// the separate six-asset gate is used for XLSX/PPTX, not this lane.
			current := func(currentCtx context.Context) bool {
				if currentCtx == nil || currentCtx.Err() != nil {
					return false
				}
				_, inspectionErr := packagedauthority.InspectCurrentPackageV2(currentCtx)
				return errors.Is(inspectionErr, packagedauthority.ErrNotPackagedRuntimeV2)
			}
			if !current(ctx) {
				t.Fatal("actual_docx_unpacked_gate_unavailable")
			}
			projector := &dsh188OfficeStageProjector{runtimeObjectProjector: p, observe: observe}
			adapter := officeeditingapp.New("docx", editingapp.New(identity, files), current)
			captureCalls := 0
			if adapter.BindSelectionHost(projector, func(captureCtx context.Context, id, capturedPath string, baseline func() error) (func(), error) {
				captureCalls++
				observe("literal_render_passed")
				observe("with_capture_enter")
				release, captureErr := registry.WithCapture(captureCtx, id, capturedPath, func() error {
					observe("baseline_enter")
					baselineErr := baseline()
					if baselineErr == nil {
						observe("baseline_ok")
					} else {
						observe("baseline_failed")
					}
					return baselineErr
				})
				switch {
				case captureErr == nil:
					observe("with_capture_ok")
				case errors.Is(captureErr, managededitingapp.ErrUncontainedExecution):
					observe("with_capture_opaque")
				case errors.Is(captureErr, managededitingapp.ErrUnsafePath):
					observe("with_capture_unsafe_path")
				case errors.Is(captureErr, managededitingapp.ErrSessionMismatch):
					observe("with_capture_session_mismatch")
				case errors.Is(captureErr, managededitingapp.ErrCaptureFailed):
					observe("with_capture_baseline_failed")
				case errors.Is(captureErr, managededitingapp.ErrUnavailable):
					observe("with_capture_unavailable")
				default:
					observe("with_capture_other")
				}
				return release, captureErr
			}) != nil {
				t.Fatal("real_selection_binding_failed")
			}
			invoke := func(operation string, input map[string]any) map[string]any {
				t.Helper()
				body, marshalErr := json.Marshal(input)
				if marshalErr != nil {
					t.Fatal("controlled_input_encoding_failed")
				}
				result, invokeErr := adapter.Invoke(ctx, adapterport.Call{Binding: adapterport.Binding{PackageID: "analytix-documents"}, Principal: scope.Principal, ContributionID: "workspace-editor", Operation: operation, Input: body, Admit: func() error { return identity.ValidateCurrent(ctx, scope.Principal) }})
				var out map[string]any
				if invokeErr != nil || json.Unmarshal(result.Output, &out) != nil {
					t.Fatalf("real_adapter_output_failed operation=%s stages=%v", operation, stages)
				}
				return out
			}
			ok := func(operation string, input map[string]any) map[string]any {
				t.Helper()
				out := invoke(operation, input)
				if out["ok"] != true {
					t.Fatalf("controlled_operation_refused operation=%s code=%v stages=%v", operation, out["code"], stages)
				}
				return out
			}
			object := func(out map[string]any, key string) map[string]any {
				t.Helper()
				value, present := out[key].(map[string]any)
				if !present {
					t.Fatal("controlled_result_shape_invalid")
				}
				return value
			}
			opened := object(ok("open-object", map[string]any{"object": map[string]any{"workspace": scope.Workspace, "path": path}}), "document")
			id, validID := opened["sessionId"].(string)
			if !validID || id == "" || opened["revision"] != fixtureSHA {
				t.Fatal("real_office_open_failed")
			}
			t.Cleanup(func() {
				closed := invoke("close-object", map[string]any{"sessionId": id})
				if closed["ok"] != true || registry.HasCaptures() {
					t.Error("real_capture_cleanup_failed")
				}
			})
			captured := invoke("capture-selection", map[string]any{"sessionId": id, "threadId": scope.ThreadID, "selectionToken": "native_selection_v8", "changeSequence": 0, "baseRevision": fixtureSHA, "text": selectedText, "editable": true})
			if phase == "actual_eager_connect_attempt" {
				if captured["ok"] != false || captured["code"] != "projection_unavailable" || captureCalls != 1 || registry.HasCaptures() || !strings.Contains(strings.Join(stages, ","), "with_capture_opaque") {
					t.Fatalf("actual_eager_effect_not_narrowed stages=%v", stages)
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(before, after) {
					t.Fatal("refused_capture_changed_synthetic_document")
				}
				t.Logf("CURRENT_V8_EAGER_ATTEMPT_REFUSED stages=%v", stages)
				return
			}
			if captured["ok"] != true || captureCalls != 1 || projector.validateCalls != 2 || !registry.HasCaptures() || admissions != 0 {
				t.Fatalf("current_v8_no_eager_control_failed stages=%v", stages)
			}
			threadAfter, threadErr := p.handler.store.GetThread(scope.ThreadID)
			if threadErr != nil || !reflect.DeepEqual(threadBefore, threadAfter) {
				t.Fatal("capture_created_or_changed_thread_authority")
			}
			t.Logf("CURRENT_V8_CAPTURE_PASS stages=%v", stages)
			selection := object(captured, "scope")
			scopeID := selection["scopeId"]
			read := object(ok("model-selection-read", map[string]any{"scopeId": scopeID, "threadId": scope.ThreadID}), "result")
			encodedParts, _ := json.Marshal(read["parts"])
			var parts []editingapp.PatchPart
			if json.Unmarshal(encodedParts, &parts) != nil || len(parts) != 1 || parts[0].Kind != "literal" || parts[0].Text != selectedText || parts[0].ProtectedRef != "" {
				t.Fatal("real_projection_does_not_match_known_synthetic_literal")
			}
			proposal := func(operation string) map[string]any {
				return object(ok("model-selection-propose", map[string]any{"scopeId": scopeID, "threadId": scope.ThreadID, "operationId": operation, "parts": []editingapp.PatchPart{{Kind: "literal", Text: replacementText}}}), "result")
			}
			first := proposal("proposal_v8_001")
			selectionRead := object(ok("selection-read", map[string]any{"sessionId": id, "scopeId": scopeID}), "scope")
			if selectionRead["scopeId"] != scopeID || selectionRead["selectionToken"] != selection["selectionToken"] || selectionRead["baseRevision"] != fixtureSHA || selectionRead["changeSequence"] != selection["changeSequence"] {
				t.Fatal("selection_read_does_not_match_captured_scope")
			}
			reviewOutput := ok("proposal-read", map[string]any{"sessionId": id, "scopeId": scopeID})
			proposals, proposalsOK := reviewOutput["proposals"].([]any)
			reviews, reviewsOK := reviewOutput["localReviews"].([]any)
			if !proposalsOK || !reviewsOK || len(proposals) != 1 || len(reviews) != 1 {
				t.Fatal("proposal_review_shape_invalid")
			}
			actualProposal, proposalOK := proposals[0].(map[string]any)
			actualReview, reviewOK := reviews[0].(map[string]any)
			if !proposalOK || !reviewOK || actualProposal["proposalId"] != first["proposalId"] || actualProposal["status"] != "proposed" || actualReview["proposalId"] != first["proposalId"] || actualReview["beforeText"] != selectedText || actualReview["afterText"] != replacementText {
				t.Fatal("core_review_does_not_match_synthetic_proposal")
			}
			rejected := object(ok("proposal-reject", map[string]any{"sessionId": id, "scopeId": scopeID, "proposalId": first["proposalId"], "operationId": "reject_v8_001"}), "proposal")
			afterReject, readErr := os.ReadFile(path)
			if rejected["status"] != "rejected" || readErr != nil || !bytes.Equal(before, afterReject) {
				t.Fatal("rejected_proposal_changed_synthetic_document")
			}
			second := proposal("proposal_v8_002")
			approved := object(ok("proposal-accept", map[string]any{"sessionId": id, "scopeId": scopeID, "proposalId": second["proposalId"], "operationId": "accept_v8_002", "selectionToken": selection["selectionToken"], "changeSequence": selection["changeSequence"], "baseRevision": fixtureSHA}), "replacement")
			if approved["text"] != replacementText || approved["baseRevision"] != fixtureSHA {
				t.Fatal("approved_replacement_does_not_match_synthetic_proposal")
			}
			// Construct only this known synthetic edit. Main's independent SDK
			// target/diff verification remains outside this Core diagnostic.
			var candidate bytes.Buffer
			writer := zip.NewWriter(&candidate)
			for _, entry := range archive.File {
				reader, openErr := entry.Open()
				if openErr != nil {
					t.Fatal("synthetic_candidate_entry_unavailable")
				}
				body, readErr := io.ReadAll(io.LimitReader(reader, 64<<10))
				closeErr := reader.Close()
				if readErr != nil || closeErr != nil || len(body) >= 64<<10 {
					t.Fatal("synthetic_candidate_entry_invalid")
				}
				if entry.Name == "word/document.xml" {
					body = bytes.Replace(body, []byte(selectedText), []byte(replacementText), 1)
				}
				header := entry.FileHeader
				target, createErr := writer.CreateHeader(&header)
				if createErr != nil {
					t.Fatal("synthetic_candidate_package_failed")
				}
				if _, writeErr := target.Write(body); writeErr != nil {
					t.Fatal("synthetic_candidate_write_failed")
				}
			}
			if writer.Close() != nil {
				t.Fatal("synthetic_candidate_close_failed")
			}
			candidateSHA := sha256.Sum256(candidate.Bytes())
			candidateRevision := hex.EncodeToString(candidateSHA[:])
			content := map[string]any{"encoding": "base64", "kind": "docx", "byteLength": candidate.Len(), "sha256": candidateRevision, "data": base64.StdEncoding.EncodeToString(candidate.Bytes())}
			receipt := object(ok("commit-object", map[string]any{"sessionId": id, "threadId": scope.ThreadID, "changeId": approved["changeId"], "operationId": approved["saveOperationId"], "baseRevision": fixtureSHA, "content": content}), "receipt")
			fresh := object(ok("open-object", map[string]any{"object": map[string]any{"workspace": scope.Workspace, "path": path}}), "document")
			afterCommit, readErr := os.ReadFile(path)
			if receipt["status"] != "committed" || receipt["operationId"] != approved["saveOperationId"] || receipt["revision"] != candidateRevision || fresh["revision"] != candidateRevision || fresh["content"] != content["data"] || readErr != nil || !bytes.Equal(afterCommit, candidate.Bytes()) || captureCalls != 1 {
				t.Fatal("approved_commit_fresh_read_not_closed")
			}
			t.Log("CURRENT_V8_PROPOSAL_CHAIN_PASS read_propose_review_reject_accept_commit_fresh_read")
		})
	}
}

// This regression drives the real asynchronous HTTP turn entry with an isolated model.
// Package materialization, captured selection, grants, intents and Host dispatch
// remain real; no proposal is fabricated and no accepted file write is requested.
func TestOfficeSelectionCurrentSyntheticDocxRealModelDispatch(t *testing.T) {
	fixture := os.Getenv("ANALYTIX_QA_DSH188_V8_DOCX")
	if fixture == "" {
		t.Skip("requires explicit current task-owned 2171-byte synthetic DOCX")
	}
	const fixtureSHA = "41515520e9b0c88b805f7878f6341027a2f95280d472cda2474cd38cdd751b33"
	const selectedText = "This document co"
	const replacementText = "QA_WORD_ACCEPT_V4_1710"
	const providerID = "qa-word-control"
	const model = "qa-dsh-combined-1710"
	before, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal("current_synthetic_input_unavailable")
	}
	digest := sha256.Sum256(before)
	if len(before) != 2171 || hex.EncodeToString(digest[:]) != fixtureSHA {
		t.Fatal("current_synthetic_input_binding_mismatch")
	}
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("private_test_root_unavailable")
	}
	workspace := workspacetest.New(t)
	path := filepath.Join(workspace, "SyntheticModelDispatch.docx")
	if os.WriteFile(path, before, 0600) != nil {
		t.Fatal("synthetic_input_copy_failed")
	}
	for _, name := range []string{"office", "runtime"} {
		if os.Mkdir(filepath.Join(root, name), 0700) != nil {
			t.Fatal("private_test_directory_unavailable")
		}
	}
	config := RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DataDir: filepath.Join(root, "data"), DurableTempDir: filepath.Join(root, "events"), ProviderID: providerID, Model: model, EndpointFormat: "chat_completions"}
	base := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	if closer, ok := base.provider.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	identity := base.turnSecurity.Identity
	files, err := filestore.NewOfficeObjectEditingFiles(filepath.Join(root, "office"), nil, "docx")
	if err != nil {
		t.Fatal("real_office_files_unavailable")
	}
	current := func(currentCtx context.Context) bool {
		if currentCtx == nil || currentCtx.Err() != nil {
			return false
		}
		_, inspectionErr := packagedauthority.InspectCurrentPackageV2(currentCtx)
		return errors.Is(inspectionErr, packagedauthority.ErrNotPackagedRuntimeV2)
	}
	if !current(ctx) {
		t.Fatal("production_docx_unpacked_gate_unavailable")
	}
	adapter := officeeditingapp.New("docx", editingapp.New(identity, files), current)
	repository, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal("repository_binding_unavailable")
	}
	source := filepath.Join(repository, "plugins", "analytix-documents")
	observed, err := pluginstore.InspectDevelopmentSourceTreeV1(ctx, source)
	if err != nil {
		t.Fatal("source_registration_unavailable")
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	if err != nil {
		t.Fatal("source_registration_invalid")
	}
	binding, err := materializationapp.NewDevelopmentSourceBindingV1(registration, source, domainplugin.TargetV1{Platform: runtime.GOOS, Arch: runtime.GOARCH})
	if err != nil {
		t.Fatal("source_binding_invalid")
	}
	authority, err := pluginauthority.OpenOrCreateV1(config.DataDir, false)
	if err != nil {
		t.Fatal("materialization_authority_unavailable")
	}
	state, err := pluginstore.NewPackageStoreV1(filepath.Join(root, "runtime"), "analytix-documents", nil)
	if err != nil {
		t.Fatal("package_store_unavailable")
	}
	materialization, err := materializationapp.NewDevelopmentSourceServiceV1(state, authority, binding, time.Now)
	if err != nil {
		t.Fatal("materialization_unavailable")
	}
	intent, err := binding.NewIntentV1(time.Now().UTC())
	if err != nil {
		t.Fatal("materialization_intent_unavailable")
	}
	intent, err = state.PrepareDevelopmentIntentV1(ctx, intent)
	if err != nil {
		t.Fatal("materialization_intent_invalid")
	}
	installed, err := materialization.Materialize(ctx, intent)
	if err != nil {
		t.Fatal("materialization_failed")
	}
	host, err := hostapp.New(identity, authority, []hostapp.Registration{{Identity: registration.Identity, SourceRegistrationSHA256: domainpackage.DevelopmentSourceRegistrationSHA256V1(registration), Materialization: materialization, State: state, Adapter: adapter}}, time.Now)
	if err != nil {
		t.Fatal("real_package_host_unavailable")
	}
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal("real_general_risk_owner_unavailable")
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority.json"), false)
	if err != nil {
		cause := "unclassified_private_error"
		switch err.Error() {
		case "private named authority root contains unknown residue":
			cause = "authority_root_unknown_residue"
		case "private authority root permissions are unsafe":
			cause = "authority_root_permissions_unsafe"
		case "private authority root is invalid":
			cause = "authority_root_invalid"
		}
		t.Fatalf("real_case_lineage_signing_authority_unavailable cause=%s", cause)
	}
	caseStore, err := newServerTestCaseThreadStore(t, filepath.Join(t.TempDir(), "case-contexts"))
	if err != nil {
		t.Fatal("real_case_lineage_store_unavailable")
	}
	lineageRegistry, err := casethreadapp.NewRegistry(ctx, key, caseStore)
	if err != nil {
		t.Fatal("real_case_lineage_registry_unavailable")
	}
	asyncObservations := make(chan AsyncTurnObservationV1, 4)
	value, err := NewRuntimeServerHandlerFromComponents(config, RuntimeServerComponents{
		AsyncTurnObserverV1: func(observation AsyncTurnObservationV1) { asyncObservations <- observation },
		Store:               base.store, CaseThreads: lineageRegistry, Memories: base.memories, CaseFinalizer: base.caseFinalizer,
		Jobs: base.jobs, Gate: base.gate, AutoResearch: base.autoResearch, Skills: base.skills, Subagents: base.subagents,
		Attachments: base.attachments, AttachmentAccess: base.attachmentAccess, AttachmentUses: base.attachmentUses,
		AllowWriteRoots: base.allowWriteRoots, ProtectedReadDirs: base.protectedReadDirs,
		Web: base.web, VisionBridge: base.visionBridge, StepLimits: base.stepLimits,
		Continuations: base.continuations, PendingWork: base.pendingWork, Checkpoints: base.checkpoints,
		NativeAuthority: base.nativeAuthority, SteeringAuthority: base.steeringAuthority, SubagentState: base.subagentState, G6Readiness: base.g6Readiness,
		TurnSecurity:        turnsecurityapp.WorkspaceSecurityAuthority{Identity: identity, Observer: filestore.CaseBindingReader{}, RiskAuthority: risk},
		ManagedEditingFiles: managededitingfiles.New(), OfficeAdapters: map[string]adapterport.Adapter{"analytix-documents": adapter}, OfficePackageHost: host, SkipMCPConnect: true,
	})
	if err != nil {
		t.Fatal("real_runtime_assembly_failed")
	}
	h := value.(*runtimeServerHandler)
	h.store.SetCaseThreadAuthority(lineageRegistry)
	if _, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-documents", GenerationID: installed.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1}); err != nil {
		t.Fatal("package_activation_failed")
	}
	views, err := host.List(ctx)
	if err != nil || len(views) != 1 || !views[0].Available || h.managedEditing == nil || h.officePackageHost != host {
		t.Fatal("assembled_host_binding_unavailable")
	}
	view := views[0]
	invoke := func(operation string, input map[string]any) map[string]any {
		t.Helper()
		body, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			t.Fatal("host_input_encoding_failed")
		}
		result, invokeErr := host.Invoke(ctx, hostapp.InvokeRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, ContributionID: "workspace-editor", Operation: operation, Input: body})
		var out map[string]any
		if invokeErr != nil || json.Unmarshal(result.Output, &out) != nil || out["ok"] != true {
			t.Fatalf("real_host_operation_failed operation=%s", operation)
		}
		return out
	}
	thread, err := h.store.CreateThread(map[string]any{"title": "Synthetic model dispatch", "workspace": workspace, "providerId": providerID, "model": model}, workspace)
	if err != nil {
		t.Fatal("real_thread_creation_failed")
	}
	threadID := stringField(thread, "id")
	opened := invoke("open-object", map[string]any{"object": map[string]any{"workspace": workspace, "path": path}})
	document, ok := opened["document"].(map[string]any)
	if !ok || document["revision"] != fixtureSHA || stringField(document, "sessionId") == "" {
		t.Fatal("real_office_open_failed")
	}
	sessionID := stringField(document, "sessionId")
	captureClosed := false
	var releaseModel, releaseAPI func()
	closeCapture := func() {
		invoke("close-object", map[string]any{"sessionId": sessionID})
		captureClosed = true
		if h.managedEditing.HasCaptures() {
			t.Error("real_strict_capture_cleanup_failed")
		}
	}
	defer func() {
		defer func() {
			if releaseAPI != nil {
				releaseAPI()
			}
		}()
		defer func() {
			if releaseModel != nil {
				releaseModel()
			}
		}()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelShutdown()
		if shutdownErr := h.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("async_cleanup_drain_failed class=%s", asyncTurnErrorClassV1(shutdownErr))
		} else if !captureClosed {
			closeCapture()
		}
	}()
	captured := invoke("capture-selection", map[string]any{"sessionId": sessionID, "threadId": threadID, "selectionToken": "model_dispatch_current", "changeSequence": 0, "baseRevision": fixtureSHA, "text": selectedText, "editable": true})
	selection, ok := captured["scope"].(map[string]any)
	if !ok || len(stringField(selection, "scopeId")) != 48 || !h.managedEditing.HasCaptures() {
		t.Fatal("real_strict_capture_unavailable")
	}
	scopeID := stringField(selection, "scopeId")
	// No pre-issued call/grant: the two actual model calls must own sequences 1/2.
	var modelMu sync.Mutex
	requests, mockFailures := 0, 0
	mockFailure := "none"
	var proposalParts []editingapp.PatchPart
	proposalObserved := false
	modelProposalID := ""
	modelProposalHostCallID := ""
	wireScopeID := ""
	wireReferenceMatches := false
	toolResultFailure := "none"
	actualSchemaHashes := map[string]string{}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelMu.Lock()
		defer modelMu.Unlock()
		requests++
		reject := func(code string) {
			mockFailures++
			mockFailure = code
			http.Error(w, "synthetic fixture rejected request", http.StatusBadRequest)
		}
		var request struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"tool_call_id"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name       string          `json:"name"`
					Parameters json.RawMessage `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&request) != nil || request.Model != model || !request.Stream {
			reject("model_stream_binding")
			return
		}
		if requests == 1 {
			var userText strings.Builder
			for _, message := range request.Messages {
				if message.Role != "user" {
					continue
				}
				var content string
				if json.Unmarshal(message.Content, &content) == nil {
					userText.WriteString(content)
				} else {
					var blocks []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					}
					if json.Unmarshal(message.Content, &blocks) != nil {
						reject("actual_native_user_content_invalid")
						return
					}
					for _, block := range blocks {
						if block.Type == "text" {
							userText.WriteString(block.Text)
						}
					}
				}
				userText.WriteByte('\n')
			}
			actualPrompt := userText.String()
			const scopeMarker = "Core scopeId: "
			if strings.Count(actualPrompt, scopeMarker) != 1 || !strings.Contains(actualPrompt, "[Office selection reference]") || strings.Contains(actualPrompt, "[Office discussion reference]") || !strings.Contains(actualPrompt, "native_selection_propose") || !strings.Contains(actualPrompt, "QA_JOURNEY_ACCEPT_V4_1710") {
				reject("actual_native_reference_shape_mismatch")
				return
			}
			fields := strings.Fields(strings.SplitN(actualPrompt, scopeMarker, 2)[1])
			if len(fields) == 0 || fields[0] != scopeID {
				reject("actual_native_scope_binding_mismatch")
				return
			}
			wireScopeID, wireReferenceMatches = fields[0], true
		}
		// Match the retained QA mock's shape checks and per-turn stability;
		// a historical receipt hash is not a new advertisement admission gate.
		required := func(schema map[string]any, names ...string) bool {
			values, ok := schema["required"].([]any)
			if !ok || len(values) != len(names) {
				return false
			}
			for _, name := range names {
				count := 0
				for _, value := range values {
					if value == name {
						count++
					}
				}
				if count != 1 {
					return false
				}
			}
			return true
		}
		for _, name := range []string{"native_selection_read", "native_selection_propose"} {
			found := 0
			for _, tool := range request.Tools {
				if tool.Function.Name != name {
					continue
				}
				found++
				var schema map[string]any
				if json.Unmarshal(tool.Function.Parameters, &schema) != nil {
					reject("actual_native_schema_invalid")
					return
				}
				properties, _ := schema["properties"].(map[string]any)
				scope, _ := properties["scopeId"].(map[string]any)
				if schema["type"] != "object" || schema["additionalProperties"] != false || scope["type"] != "string" || scope["pattern"] != "^[a-f0-9]{48}$" {
					reject("actual_native_scope_schema_mismatch")
					return
				}
				if name == "native_selection_read" {
					if len(properties) != 1 || !required(schema, "scopeId") {
						reject("actual_read_schema_mismatch")
						return
					}
				} else {
					operation, _ := properties["operationId"].(map[string]any)
					parts, _ := properties["parts"].(map[string]any)
					items, _ := parts["items"].(map[string]any)
					options, _ := items["oneOf"].([]any)
					if !required(schema, "scopeId", "operationId") || operation["type"] != "string" || operation["pattern"] != "^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$" || parts["type"] != "array" || parts["maxItems"] != float64(256) || len(options) != 2 {
						reject("actual_propose_schema_mismatch")
						return
					}
					literal, protected := false, false
					for _, rawOption := range options {
						option, _ := rawOption.(map[string]any)
						fields, _ := option["properties"].(map[string]any)
						kind, _ := fields["kind"].(map[string]any)
						enum, _ := kind["enum"].([]any)
						if option["type"] != "object" || option["additionalProperties"] != false || len(fields) != 2 || len(enum) != 1 {
							continue
						}
						if enum[0] == "literal" {
							text, _ := fields["text"].(map[string]any)
							literal = required(option, "kind", "text") && text["type"] == "string" && text["maxLength"] == float64(65536)
						} else if enum[0] == "protected" {
							ref, _ := fields["protectedRef"].(map[string]any)
							protected = required(option, "kind", "protectedRef") && ref["type"] == "string" && ref["pattern"] == "^protected_[a-f0-9]{48}$"
						}
					}
					branches, _ := schema["oneOf"].([]any)
					textBranch := false
					for _, rawBranch := range branches {
						branch, _ := rawBranch.(map[string]any)
						fields, _ := branch["properties"].(map[string]any)
						if branch["type"] == "object" && branch["additionalProperties"] == false && required(branch, "scopeId", "operationId", "parts") && len(fields) == 3 && reflect.DeepEqual(fields["scopeId"], properties["scopeId"]) && reflect.DeepEqual(fields["operationId"], operation) && reflect.DeepEqual(fields["parts"], parts) {
							textBranch = true
						}
					}
					if !literal || !protected || !textBranch {
						reject("actual_text_proposal_branch_mismatch")
						return
					}
				}
				hash := domainsecurity.CanonicalJSONHash(tool.Function.Parameters)
				if previous := actualSchemaHashes[name]; previous != "" && previous != hash {
					reject("native_schema_changed")
					return
				}
				actualSchemaHashes[name] = hash
			}
			if found != 1 {
				reject("actual_native_schema_missing_or_duplicated")
				return
			}
		}
		var outputs []map[string]any
		for _, message := range request.Messages {
			if message.Role == "tool" {
				var content string
				var output map[string]any
				if !strings.HasPrefix(message.ToolCallID, "call_host_") || json.Unmarshal(message.Content, &content) != nil || json.Unmarshal([]byte(content), &output) != nil || output["isError"] == true || output["ok"] == false || output["error"] != nil {
					code := stringField(output, "code")
					if strings.HasPrefix(code, "execution_grant_") || code == "tool_failed" || code == "managed_editing_mutation_blocked" {
						toolResultFailure = code
					}
					reject("actual_tool_result_invalid")
					return
				}
				outputs = append(outputs, output)
				if len(outputs) == 2 {
					modelProposalHostCallID = message.ToolCallID
				}
			}
		}
		if len(outputs) != requests-1 || requests > 3 {
			reject("actual_tool_result_count")
			return
		}
		frame := func(delta map[string]any, finish any) {
			chunk := map[string]any{"object": "chat.completion.chunk", "model": model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
			body, _ := json.Marshal(chunk)
			_, _ = w.Write(append(append([]byte("data: "), body...), []byte("\n\n")...))
		}
		issue := func(name, callID string, arguments map[string]any) {
			body, _ := json.Marshal(arguments)
			w.Header().Set("Content-Type", "text/event-stream")
			frame(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": callID, "type": "function", "function": map[string]any{"name": name, "arguments": string(body)}}}}, nil)
			frame(map[string]any{}, "tool_calls")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
		switch requests {
		case 1:
			issue("native_selection_read", "qa_journey_accept_v4_0", map[string]any{"scopeId": wireScopeID})
		case 2:
			read := outputs[0]
			body, _ := json.Marshal(read["parts"])
			if read["scopeId"] != scopeID || read["editable"] != true || json.Unmarshal(body, &proposalParts) != nil || len(proposalParts) != 1 || proposalParts[0].Kind != "literal" || proposalParts[0].Text != selectedText || proposalParts[0].ProtectedRef != "" {
				reject("actual_editable_read_required")
				return
			}
			proposalParts[0].Text = replacementText
			issue("native_selection_propose", "qa_journey_accept_v4_1", map[string]any{"scopeId": wireScopeID, "operationId": "qa_journey_accept_v4_1710", "parts": proposalParts})
		case 3:
			proposal := outputs[1]
			body, _ := json.Marshal(proposal["parts"])
			var actual []editingapp.PatchPart
			if len(stringField(proposal, "proposalId")) != 48 || proposal["status"] != "proposed" || json.Unmarshal(body, &actual) != nil || !reflect.DeepEqual(actual, proposalParts) {
				reject("actual_proposal_result_required")
				return
			}
			proposalObserved = true
			modelProposalID = stringField(proposal, "proposalId")
			w.Header().Set("Content-Type", "text/event-stream")
			frame(map[string]any{"role": "assistant", "content": "QA_JOURNEY_ACCEPT_V4_FINAL_1710: proposal awaits explicit user review."}, nil)
			frame(map[string]any{}, "stop")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
	}))
	releaseModel = fake.Close
	h.providerConfig = provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{DefaultProviderID: providerID, DefaultBaseURL: fake.URL + "/v1", DefaultAPIKey: "task-local-inert-key", DefaultEndpointFormat: "chat_completions", DefaultModel: model})
	installHandlerProviderExecutionResolverForTest(h)
	client := provider.NewHTTPProviderClient(fake.Client())
	h.provider = client
	releaseModel = func() { client.Close(); fake.Close() }
	for _, dependency := range []struct {
		name      string
		available bool
	}{
		{"primary_store", h.store != nil},
		{"case_lineage", h.caseThreads != nil},
		{"jobs", h.jobs != nil},
		{"approval_gate", h.gate != nil},
		{"pending_work", h.pendingWork != nil},
		{"continuations", h.continuations != nil},
		{"checkpoint_store", h.checkpoints.Store != nil},
		{"native_authority", h.nativeAuthority != nil},
		{"steering_authority", h.steeringAuthority != nil},
		{"context_effect_owner", h.subagentState != nil},
		{"terminal_arbitrator", h.runtimeControl() != nil},
		{"identity", h.turnSecurity.Identity != nil},
		{"workspace_observer", h.turnSecurity.Observer != nil},
		{"risk_authority", h.turnSecurity.RiskAuthority != nil},
		{"managed_editing", h.managedEditing != nil},
		{"office_package_host", h.officePackageHost != nil},
		{"provider", h.provider != nil},
		{"attachments", h.attachments != nil},
		{"attachment_access", h.attachmentAccess != nil && h.attachmentAccess.Owners != nil},
		{"attachment_uses", h.attachmentUses != nil},
	} {
		if !dependency.available {
			t.Fatalf("production_dependency_unavailable dependency=%s", dependency.name)
		}
	}
	if h.caseThreads != lineageRegistry || h.store.caseThreads != lineageRegistry {
		t.Fatal("case_lineage_owner_mismatch")
	}
	// Match the renderer's DOCX native-reference prompt and runtime body shape.
	// v9 retained reference-shape metadata, not original POST bytes.
	const instruction = "QA_JOURNEY_ACCEPT_V4_1710 将选中文本替换为合成标记 QA_WORD_ACCEPT_V4_1710，提出建议，等待我接受。"
	prompt := instruction + "\n\n[Office selection reference]\nCore scopeId: " + scopeID +
		"\nUse native_selection_read for the current model-safe selection. For this local edit, use native_selection_propose with protected parts and wait for explicit acceptance. A proposal is not an applied or saved edit."
	requestBody := caseIngressJSONV1(t, map[string]any{
		"prompt": prompt, "displayText": instruction, "mode": "agent", "async": true,
		"providerId": providerID, "model": model,
		"approvalPolicy": "on-request", "sandboxMode": "workspace-write",
	})
	decodedRequest, decodeErr := httpapi.DecodeStartTurnRequest(threadID, requestBody)
	if decodeErr != nil || !decodedRequest.Async || decodedRequest.Prompt != prompt || decodedRequest.DisplayText != instruction || decodedRequest.Mode != "agent" || decodedRequest.ProviderID != providerID || decodedRequest.Model != model || decodedRequest.ApprovalPolicy != "on-request" || decodedRequest.SandboxMode != "workspace-write" || decodedRequest.EndpointFormat != "" || decodedRequest.DisableUserInputSet || decodedRequest.MaxModelSteps != nil || len(decodedRequest.FileReferences) != 0 || len(decodedRequest.AttachmentIDs) != 0 {
		t.Fatal("production_shape_http_request_invalid")
	}
	apiServer := httptest.NewServer(h)
	releaseAPI = apiServer.Close
	streamCtx, cancelStream := context.WithTimeout(ctx, 60*time.Second)
	defer cancelStream()
	streamRequest, err := http.NewRequestWithContext(streamCtx, http.MethodGet, apiServer.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatal("async_sse_request_invalid")
	}
	streamRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	streamRequest.Header.Set("Accept", "text/event-stream")
	stream, err := apiServer.Client().Do(streamRequest)
	if err != nil {
		t.Fatalf("async_sse_connection_failed class=%s", asyncTurnErrorClassV1(err))
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("async_sse_status_invalid status=%d", stream.StatusCode)
	}
	sseFrames, sseErrors := make(chan string, 1), make(chan error, 1)
	go func() {
		frame, readErr := readSSEFrameContaining(stream.Body, "event: general_terminal_batch")
		if readErr != nil {
			sseErrors <- readErr
			return
		}
		sseFrames <- frame
	}()
	// Keep the existing request helper's auth/JSON/status contract, with a bound context.
	postRequest, err := http.NewRequestWithContext(streamCtx, http.MethodPost, apiServer.URL+"/v1/threads/"+threadID+"/turns", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal("async_post_request_invalid")
	}
	postRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	postRequest.Header.Set("Content-Type", "application/json")
	postResponse, err := apiServer.Client().Do(postRequest)
	if err != nil {
		t.Fatalf("MODEL_DISPATCH_HTTP_ADMISSION_RED transport=%s", asyncTurnErrorClassV1(err))
	}
	var startResponse map[string]any
	decodeResponseErr := json.NewDecoder(io.LimitReader(postResponse.Body, 4<<20)).Decode(&startResponse)
	postResponse.Body.Close()
	if decodeResponseErr != nil || postResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("MODEL_DISPATCH_HTTP_ADMISSION_RED status=%d publicClass=%s", postResponse.StatusCode, asyncTurnTypedErrorClassV1(stringField(startResponse, "code")))
	}
	turnID := stringField(startResponse, "turnId")
	if turnID == "" {
		t.Fatal("async_turn_identity_missing")
	}
	asyncStartedCount := 0
	var asyncCompletion AsyncTurnObservationV1
	for asyncCompletion.Stage != "finished" {
		select {
		case observation := <-asyncObservations:
			if observation.ThreadID != threadID || observation.TurnID != turnID {
				t.Fatal("async_observer_identity_mismatch")
			}
			if observation.Stage == "started" {
				asyncStartedCount++
			} else if observation.Stage == "finished" {
				asyncCompletion = observation
			}
		case <-streamCtx.Done():
			t.Fatal("async_completion_observation_timeout")
		}
	}
	t.Logf("MODEL_DISPATCH_HTTP_ASYNC phase=%s completion=%s detail=%s failureRecord=%s failureDetail=%s terminal=%s", asyncCompletion.CompletionPhase, asyncCompletion.CompletionErrorClass, asyncCompletion.CompletionDetailClass, asyncCompletion.FailureRecordErrorClass, asyncCompletion.FailureRecordDetailClass, asyncCompletion.TerminalStatus)
	if drainErr := h.runtimeControl().WaitForTurnOperations(streamCtx); drainErr != nil {
		t.Fatalf("async_operation_drain_failed class=%s", asyncTurnErrorClassV1(drainErr))
	}
	var terminalFrame string
	select {
	case terminalFrame = <-sseFrames:
	case readErr := <-sseErrors:
		t.Fatalf("async_terminal_sse_failed class=%s", asyncTurnErrorClassV1(readErr))
	case <-streamCtx.Done():
		t.Fatal("async_terminal_sse_timeout")
	}
	var batchRecord map[string]any
	for _, line := range strings.Split(terminalFrame, "\n") {
		if strings.HasPrefix(line, "data:") && json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &batchRecord) != nil {
			t.Fatal("async_terminal_sse_json_invalid")
		}
	}
	terminalBatch, batchErr := domainevent.ParseGeneralTerminalDeliveryBatchV1(batchRecord)
	if batchErr != nil || terminalBatch.ThreadID != threadID || terminalBatch.TurnID != turnID {
		t.Fatal("async_terminal_sse_batch_invalid")
	}
	sseCompleted, sseFailed := 0, 0
	for _, event := range terminalBatch.Events {
		switch stringField(event, "kind") {
		case "turn_completed":
			sseCompleted++
		case "turn_failed", "turn_aborted", "turn_cancelled", "turn_interrupted":
			sseFailed++
		}
	}
	classify := asyncTurnErrorClassV1

	modelMu.Lock()
	sends, refused, fixtureFailure, sawProposal, returnedProposalID, returnedProposalCallID, actualToolFailure, wireReferenceOK := requests, mockFailures, mockFailure, proposalObserved, modelProposalID, modelProposalHostCallID, toolResultFailure, wireReferenceMatches
	modelMu.Unlock()
	snapshot, threadErr := h.store.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if threadErr != nil {
		t.Fatalf("async_durable_readback_failed class=%s", asyncTurnErrorClassV1(threadErr))
	}
	currentThread := snapshot.Thread
	var completedTurn map[string]any
	for _, raw := range listAny(currentThread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID {
			completedTurn = turn
		}
	}
	durableCompleted := stringField(completedTurn, "status") == "completed"
	finalTextMatches := ordinaryResultAssistantText(completedTurn) == "QA_JOURNEY_ACCEPT_V4_FINAL_1710: proposal awaits explicit user review."
	userMetadataCount := 0
	for _, raw := range listAny(completedTurn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "user_message" && item["text"] == prompt && item["displayText"] == instruction {
			userMetadataCount++
		}
	}
	frozen, frozenErr := domainsecurity.ParseTurnSecurityContext(completedTurn["securityContext"])
	generalTurn := frozenErr == nil && domainsecurity.TurnSecurityContextIsGeneral(frozen) && frozen.ThreadID == threadID && frozen.TurnID == turnID
	registry, registryErr := executiongrantapp.RegistryFromThread(threadID, currentThread, turnID)
	readGrants, proposeGrants, settledPropose := 0, 0, 0
	var readSequence, proposeSequence uint64
	proposalGrantID, proposalHostCallID := "", ""
	if registryErr == nil {
		for _, entry := range registry.Entries {
			switch entry.Grant.ToolName {
			case "native_selection_read":
				readGrants++
				readSequence = entry.Sequence
			case "native_selection_propose":
				proposeGrants++
				proposeSequence = entry.Sequence
				proposalGrantID, proposalHostCallID = entry.Grant.GrantID, entry.Grant.ToolCallID
				if entry.Status == domainsecurity.GrantRegistrySettled {
					settledPropose++
				}
			}
		}
	}
	inventory, inventoryErr := h.pendingWork.TrustedInventoryV1(ctx)
	intentCount, intentClosed := 0, 0
	if inventoryErr == nil {
		for _, receipt := range inventory.Receipts {
			if receipt.Kind == "side_effect_intent" && receipt.Context.ThreadID == threadID && receipt.Context.TurnID == turnID && len(receipt.GrantMembers) == 1 && receipt.GrantMembers[0].GrantID == proposalGrantID {
				intentCount++
				if disposition, found := inventory.Dispositions[receipt.WorkID]; found && disposition.Status == "completed" {
					intentClosed++
				}
			}
		}
	} else {
		intentCount, intentClosed = -1, -1
	}
	events, eventErr := h.store.eventLog.LoadSinceContext(ctx, threadID, 0)
	ready, started, finished := 0, 0, 0
	turnStarted, turnCompleted, turnFailed, durableResultMatches := 0, 0, 0, 0
	for _, event := range events.Events {
		if stringField(event, "turnId") != turnID {
			continue
		}
		switch stringField(event, "kind") {
		case "turn_started":
			turnStarted++
		case "turn_completed":
			turnCompleted++
		case "turn_failed", "turn_aborted", "turn_cancelled", "turn_interrupted":
			turnFailed++
		}
		if stringField(event, "kind") == "pipeline_stage" {
			switch phase := stringField(event, "stage"); phase {
			case "pre_send", "post_send", "response_received", "logical_step_assembled", "logical_step_closed":
				t.Logf("MODEL_DISPATCH_DURABLE_PHASE stage=%s", phase)
			}
		}
		item, _ := event["item"].(map[string]any)
		name := stringField(event, "toolName")
		if name == "" {
			name = stringField(item, "toolName")
		}
		if name != "native_selection_propose" {
			continue
		}
		switch stringField(event, "kind") {
		case "tool_call_ready":
			ready++
		case "tool_call_started":
			started++
		case "tool_call_finished":
			finished++
			out, _ := item["output"].(map[string]any)
			if stringField(event, "itemId") != stringField(item, "id") || stringField(item, "callId") != proposalHostCallID || proposalHostCallID != returnedProposalCallID || stringField(item, "executionGrantId") != proposalGrantID || stringField(event, "executionGrantId") != proposalGrantID || stringField(item, "status") != "completed" || item["isError"] != false || out["projectionKind"] != "host_status" || out["status"] != "completed" || out["code"] != "tool_completed" || out["privatePayloadWithheld"] != true || out["factAnswerAllowed"] != false || out["evidenceAuthority"] != false {
				t.Fatal("proposal_durable_result_binding_mismatch")
			}
			for _, raw := range listAny(completedTurn["items"]) {
				primary, _ := raw.(map[string]any)
				if primary["kind"] == "tool_result" && primary["id"] == item["id"] && primary["callId"] == proposalHostCallID && primary["executionGrantId"] == proposalGrantID && primary["status"] == "completed" && primary["isError"] == false && reflect.DeepEqual(primary["output"], out) {
					durableResultMatches++
				}
			}
		}
	}
	reviewBody, _ := json.Marshal(map[string]any{"sessionId": sessionID, "scopeId": scopeID})
	reviewResult, reviewErr := host.Invoke(ctx, hostapp.InvokeRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, ContributionID: "workspace-editor", Operation: "proposal-read", Input: reviewBody})
	var review map[string]any
	reviewOK := reviewErr == nil && json.Unmarshal(reviewResult.Output, &review) == nil && review["ok"] == true
	proposals, proposalsOK := review["proposals"].([]any)
	queue := -1
	if reviewOK && proposalsOK {
		queue = len(proposals)
	}
	queueMatchesModel := false
	if queue == 1 {
		proposal, _ := proposals[0].(map[string]any)
		queueMatchesModel = proposal["proposalId"] == returnedProposalID && proposal["status"] == "proposed"
	}
	after, fileErr := os.ReadFile(path)
	unchanged := fileErr == nil && bytes.Equal(before, after)
	t.Logf("MODEL_DISPATCH_OBSERVATION route=http_async wireReference=%t phase=%s completion=%s failureRecord=%s terminal=%s sseCompleted=%d sseFailed=%d durableCompleted=%t finalText=%t userMetadata=%d generalTurn=%t turnStarted=%d turnCompleted=%d turnFailed=%d durableResultMatches=%d requests=%d mockRefusals=%d fixtureFailure=%s toolResultFailure=%s readGrants=%d proposeGrants=%d readSequence=%d proposeSequence=%d proposeSettled=%d intentReceipts=%d intentCompleted=%d durableReady=%d durableStarted=%d durableFinished=%d proposalQueue=%d strictCapture=%t diskUnchanged=%t inventoryErrorClass=%s registryErrorClass=%s", wireReferenceOK, asyncCompletion.CompletionPhase, asyncCompletion.CompletionErrorClass, asyncCompletion.FailureRecordErrorClass, asyncCompletion.TerminalStatus, sseCompleted, sseFailed, durableCompleted, finalTextMatches, userMetadataCount, generalTurn, turnStarted, turnCompleted, turnFailed, durableResultMatches, sends, refused, fixtureFailure, actualToolFailure, readGrants, proposeGrants, readSequence, proposeSequence, settledPropose, intentCount, intentClosed, ready, started, finished, queue, h.managedEditing.HasCaptures(), unchanged, classify(inventoryErr), classify(registryErr))
	if asyncStartedCount != 1 || asyncCompletion.CompletionErrorClass != "none" || asyncCompletion.FailureRecordErrorClass != "none" || asyncCompletion.TerminalStatus != "completed" || sseCompleted != 1 || sseFailed != 0 || !durableCompleted || !finalTextMatches || userMetadataCount != 1 || !generalTurn || turnStarted != 1 || turnCompleted != 1 || turnFailed != 0 || durableResultMatches != 1 {
		t.Fatalf("MODEL_DISPATCH_HTTP_ASYNC_RED phase=%s completion=%s detail=%s failureRecord=%s failureDetail=%s terminal=%s", asyncCompletion.CompletionPhase, asyncCompletion.CompletionErrorClass, asyncCompletion.CompletionDetailClass, asyncCompletion.FailureRecordErrorClass, asyncCompletion.FailureRecordDetailClass, asyncCompletion.TerminalStatus)
	}
	if !wireReferenceOK || refused != 0 || fixtureFailure != "none" || !sawProposal || sends != 3 || threadErr != nil || registryErr != nil || inventoryErr != nil || eventErr != nil || readGrants != 1 || proposeGrants != 1 || readSequence != 1 || proposeSequence != 2 || settledPropose != 1 || intentCount != 1 || intentClosed != 1 || ready != 1 || started != 1 || finished != 1 || !reviewOK || !proposalsOK || queue != 1 || !queueMatchesModel || !h.managedEditing.HasCaptures() || !unchanged {
		t.Fatal("MODEL_DISPATCH_EXPECTED_CHAIN_INCOMPLETE")
	}
	closeCapture()
	t.Log("MODEL_DISPATCH_PASS real_http_async_turn_start_sse_terminal_durable_readback_read_propose_final_real_host_intent_result_queue_no_file_write")
}

func newSelectionProjectorFixture(t *testing.T) (runtimeObjectProjector, map[string]any, editingapp.ScopeAuthority) {
	t.Helper()
	workspace := workspacetest.New(t)
	h := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	thread, err := h.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err = h.store.GetThread(stringField(thread, "id"))
	if err != nil {
		t.Fatal(err)
	}
	scope := editingapp.ScopeAuthority{Principal: testIdentityPrincipal(), ObjectID: "synthetic-object", ThreadID: stringField(thread, "id"), Purpose: "edit", Workspace: workspace, Path: "synthetic.docx"}
	return runtimeObjectProjector{handler: h}, thread, scope
}

func TestNativeSelectionFreshThreadUsesHostAuthorityWithoutCreatingHistory(t *testing.T) {
	p, before, scope := newSelectionProjectorFixture(t)
	ctx := context.Background()
	for _, purpose := range []string{"discuss", "edit"} {
		scope.Purpose = purpose
		ranges, err := p.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: scope, Text: "Contact: alice@example.com"})
		if err != nil || len(ranges) == 0 {
			t.Fatalf("fresh %s capture did not retain protected projection: ranges=%d err=%v", purpose, len(ranges), err)
		}
	}
	after, err := p.handler.store.GetThread(scope.ThreadID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("selection altered the thread or its history: err=%v", err)
	}
	// The normal turn-start authority can take over without the selection having
	// manufactured a prior turn, persisted context, or provider request.
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: p.handler.turnSecurity, Thread: after, ThreadID: scope.ThreadID,
		TurnID: "turn-first-real-send", Workspace: scope.Workspace, Principal: scope.Principal, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(frozen)
	if err := p.handler.store.AppendTurnToThread(scope.ThreadID, map[string]any{
		"id": frozen.TurnID, "threadId": scope.ThreadID, "status": "completed", "items": []any{}, "securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateCurrent(ctx, scope); err != nil {
		t.Fatalf("same-thread selection could not use normal turn authority: %v", err)
	}
}

func TestNativeSelectionFreshThreadRejectsNonfreshOrDamagedAuthority(t *testing.T) {
	p, original, scope := newSelectionProjectorFixture(t)
	cases := map[string]func(map[string]any){
		"past turn":              func(v map[string]any) { v["turns"] = []any{map[string]any{"id": "previous"}} },
		"missing turns":          func(v map[string]any) { delete(v, "turns") },
		"malformed turns":        func(v map[string]any) { v["turns"] = "invalid" },
		"running":                func(v map[string]any) { v["status"] = "running" },
		"side":                   func(v map[string]any) { v["relation"] = "side" },
		"missing relation":       func(v map[string]any) { delete(v, "relation") },
		"kind":                   func(v map[string]any) { v["kind"] = "case" },
		"null kind":              func(v map[string]any) { v["kind"] = nil },
		"case marker":            func(v map[string]any) { v["caseId"] = "synthetic-case" },
		"null case marker":       func(v map[string]any) { v["caseProjectId"] = nil },
		"parent":                 func(v map[string]any) { v["parentThreadId"] = "parent" },
		"fork":                   func(v map[string]any) { v["forkedFromThreadId"] = "source" },
		"boundary history":       func(v map[string]any) { v["historyAuthority"] = "case_boundary_only" },
		"epoch without security": func(v map[string]any) { v["contextEpochState"] = nil },
		"null security":          func(v map[string]any) { v["securityState"] = nil },
		"damaged security":       func(v map[string]any) { v["securityState"] = map[string]any{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			thread := contracts.CloneMap(original)
			mutate(thread)
			if _, err := p.selectionSecurityContext(context.Background(), thread, scope); err == nil {
				t.Fatal("unsupported thread received fresh selection authority")
			}
		})
	}
	p.handler.caseThreads.(*caseThreadAuthorityStub).threads[scope.ThreadID] = true
	if _, err := p.selectionSecurityContext(context.Background(), original, scope); err == nil {
		t.Fatal("registered case thread received fresh general authority")
	}
}

func TestNativeSelectionFreshThreadRejectsIdentityWorkspaceAndAuthorityMismatch(t *testing.T) {
	p, _, scope := newSelectionProjectorFixture(t)
	wrongPrincipal := scope
	wrongPrincipal.Principal.UserID = "other-user"
	wrongWorkspace := scope
	wrongWorkspace.Workspace = workspacetest.New(t)
	wrongThread := scope
	wrongThread.ThreadID = "missing-thread"
	for _, candidate := range []editingapp.ScopeAuthority{wrongPrincipal, wrongWorkspace, wrongThread} {
		if err := p.ValidateCurrent(context.Background(), candidate); err == nil {
			t.Fatal("mismatched selection received authority")
		}
	}
	p.handler.turnSecurity.RiskAuthority = nil
	if err := p.ValidateCurrent(context.Background(), scope); err == nil {
		t.Fatal("missing risk authority inferred a general selection")
	}
}

func TestObjectExportRealFileMixedCaseWorkspaceAndAuthorityDrift(t *testing.T) {
	p, _, base := newSelectionProjectorFixture(t)
	workspace := filepath.Join(base.Workspace, "MixedCaseWorkspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	thread, err := p.handler.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err = p.handler.store.GetThread(stringField(thread, "id"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "Draft.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := filestore.NewObjectEditingFiles(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	service := editingapp.NewWithProjector(p.handler.turnSecurity.Identity, files, p)
	ctx := context.Background()
	opened, err := service.Open(ctx, workspace, path)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := service.UpdateDraft(ctx, editingapp.UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: opened.Revision, Content: "unsaved local text"})
	if err != nil {
		t.Fatal(err)
	}
	input := editingapp.ExportSnapshotInput{SessionID: opened.SessionID, ObjectID: opened.ObjectID, ThreadID: stringField(thread, "id"), BaseRevision: opened.Revision, DraftVersion: draft.Version}
	snapshot, err := service.ReadExportSnapshot(ctx, input)
	if err != nil || snapshot.Content != draft.Content {
		t.Fatalf("real file export did not match thread workspace: %v", err)
	}
	after, err := p.handler.store.GetThread(input.ThreadID)
	if err != nil || !reflect.DeepEqual(thread, after) {
		t.Fatal("export changed thread history")
	}
	// The same real object must also use an existing turn security record,
	// without rewriting its workspace spelling or creating another turn.
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: p.handler.turnSecurity, Thread: after, ThreadID: input.ThreadID,
		TurnID: "export-existing-turn", Workspace: workspace, Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := turnsecurityapp.PublicRecord(frozen)
	if err := p.handler.store.AppendTurnToThread(input.ThreadID, map[string]any{
		"id": frozen.TurnID, "threadId": input.ThreadID, "status": "completed", "items": []any{}, "securityContext": record,
	}, "", map[string]any{"securityState": record}); err != nil {
		t.Fatal(err)
	}
	if value, err := service.ReadExportSnapshot(ctx, input); err != nil || value.Content != draft.Content {
		t.Fatalf("existing security context rejected export: %v", err)
	}
	input.ThreadID = base.ThreadID
	if value, err := service.ReadExportSnapshot(ctx, input); err == nil || value.Content != "" {
		t.Fatal("foreign workspace thread received export")
	}
	input.ThreadID = stringField(thread, "id")
	observer := p.handler.turnSecurity.Observer
	foreign := workspacetest.New(t)
	p.handler.turnSecurity.Observer = selectionObserver(func(string) (domainsecurity.CaseBindingObservationV1, error) { return observer.Observe(foreign) })
	if value, err := service.ReadExportSnapshot(ctx, input); err == nil || value.Content != "" {
		t.Fatal("injected observer mismatch received export")
	}
	calls := 0
	p.handler.turnSecurity.Observer = selectionObserver(func(path string) (domainsecurity.CaseBindingObservationV1, error) {
		calls++
		if calls >= 3 {
			return observer.Observe(foreign)
		}
		return observer.Observe(path)
	})
	if value, err := service.ReadExportSnapshot(ctx, input); err == nil || value.Content != "" {
		t.Fatal("workspace drift during gate acquisition received export")
	}
	p.handler.turnSecurity.Observer = observer
	moved := workspace + "-moved"
	if err := os.Rename(workspace, moved); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(workspace); _ = os.Rename(moved, workspace) }()
	if err := os.Symlink(foreign, workspace); err != nil {
		t.Fatal(err)
	}
	if value, err := service.ReadExportSnapshot(ctx, input); err == nil || value.Content != "" {
		t.Fatal("rebound symlink alias received export")
	}
	original, err := os.ReadFile(filepath.Join(moved, "Draft.md"))
	if err != nil || string(original) != "original" {
		t.Fatal("export altered original")
	}
}

func TestNativeSelectionDoesNotFoldDistinctCaseSensitiveWorkspaces(t *testing.T) {
	p, _, scope := newSelectionProjectorFixture(t)
	upper := filepath.Join(scope.Workspace, "CaseDirectory")
	lower := filepath.Join(scope.Workspace, "casedirectory")
	if err := os.Mkdir(upper, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lower, 0700); err != nil {
		if os.IsExist(err) {
			t.Skip("host fixture filesystem is case-insensitive")
		}
		t.Fatal(err)
	}
	thread, err := p.handler.store.CreateThread(map[string]any{"workspace": upper}, upper)
	if err != nil {
		t.Fatal(err)
	}
	scope.ThreadID = stringField(thread, "id")
	scope.Workspace = lower
	if err := p.ValidateCurrent(context.Background(), scope); err == nil {
		t.Fatal("distinct case-sensitive directory received authority")
	}
}

func TestDocumentCreationWorkspaceRetainsThreadSpellingAndRejectsForeignAuthority(t *testing.T) {
	p, _, scope := newSelectionProjectorFixture(t)
	ctx := context.Background()
	principal, err := p.handler.turnSecurity.Identity.ResolveCurrent(ctx)
	if err != nil {
		t.Fatal("current_principal_unavailable")
	}
	workspace := filepath.Join(scope.Workspace, "MixedCaseDocumentWorkspace")
	if os.Mkdir(workspace, 0700) != nil {
		t.Fatal("mixed_workspace_unavailable")
	}
	thread, err := p.handler.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal("trusted_thread_unavailable")
	}
	id := stringField(thread, "id")
	identity, display, err := p.ResolveDocumentWorkspace(ctx, principal, id)
	expectedIdentity, ok := filestore.ResolveMutationIdentityPath(workspace, workspace)
	if err != nil || !ok || identity != expectedIdentity || display != workspace {
		t.Fatal("trusted_workspace_spelling_or_identity_changed")
	}
	wrongPrincipal := principal
	wrongPrincipal.UserID = "other-user"
	if _, _, err := p.ResolveDocumentWorkspace(ctx, wrongPrincipal, id); err == nil {
		t.Fatal("foreign_principal_received_creation_workspace")
	}
	observer := p.handler.turnSecurity.Observer
	foreign := workspacetest.New(t)
	p.handler.turnSecurity.Observer = selectionObserver(func(string) (domainsecurity.CaseBindingObservationV1, error) { return observer.Observe(foreign) })
	if _, _, err := p.ResolveDocumentWorkspace(ctx, principal, id); err == nil {
		t.Fatal("foreign_workspace_received_creation_authority")
	}
	p.handler.turnSecurity.Observer = observer
	t.Run("distinct_case_sensitive_workspaces", func(t *testing.T) {
		lower := filepath.Join(scope.Workspace, "mixedcasedocumentworkspace")
		if err := os.Mkdir(lower, 0700); err != nil {
			if os.IsExist(err) {
				t.Skip("host fixture filesystem is case-insensitive")
			}
			t.Fatal("distinct_case_workspace_unavailable")
		}
		lowerIdentity, ok := filestore.ResolveMutationIdentityPath(lower, lower)
		if !ok || lowerIdentity == identity {
			t.Fatal("distinct_case_sensitive_workspace_identity_collapsed")
		}
		p.handler.turnSecurity.Observer = selectionObserver(func(string) (domainsecurity.CaseBindingObservationV1, error) { return observer.Observe(lower) })
		defer func() { p.handler.turnSecurity.Observer = observer }()
		if _, _, err := p.ResolveDocumentWorkspace(ctx, principal, id); err == nil {
			t.Fatal("distinct_case_workspace_received_creation_authority")
		}
	})
}

// The observer delegates preparation to the real session service and filestore.
// It retains only closed phase booleans, never requests, paths or error bodies.
type dshManualBeginObservedService struct {
	*editingapp.Service
	prepareEntered bool
	prepareOK      bool
}

func (s *dshManualBeginObservedService) PrepareNativeManual(ctx context.Context, id string, draft fileport.NativeManualDraft) (fileport.NativeChangeStatus, error) {
	s.prepareEntered = true
	status, err := s.Service.PrepareNativeManual(ctx, id, draft)
	s.prepareOK = err == nil
	return status, err
}

func TestOfficeManualBeginRealPackageHostSyntheticDocx(t *testing.T) {
	assetRoot := os.Getenv("ANALYTIX_QA_DSH188_OFFICE_ASSETS")
	codecExecutable, codecEntry := os.Getenv("ANALYTIX_DOCUMENT_CODEC_EXECUTABLE"), os.Getenv("ANALYTIX_DOCUMENT_CODEC_ENTRY")
	if assetRoot == "" || codecExecutable == "" || codecEntry == "" {
		t.Skip("requires explicit qualified document codec and Office assets")
	}
	ctx := context.Background()
	p, thread, scope := newSelectionProjectorFixture(t)
	// Preserve ordinary workspace spelling through creation and the real receipt.
	// Filesystem identity is observed afterward, never used as a test workaround.
	workspace := filepath.Join(scope.Workspace, "MixedCaseWordWorkspace")
	if os.Mkdir(workspace, 0700) != nil {
		t.Fatal("mixed_case_workspace_unavailable")
	}
	thread, err := p.handler.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal("normal_workspace_thread_create_failed")
	}
	thread, err = p.handler.store.GetThread(stringField(thread, "id"))
	if err != nil {
		t.Fatal("normal_workspace_thread_read_failed")
	}
	scope.ThreadID, scope.Workspace = stringField(thread, "id"), workspace
	identity := p.handler.turnSecurity.Identity
	risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
	if err != nil {
		t.Fatal("real_general_risk_owner_unavailable")
	}
	p.handler.turnSecurity = turnsecurityapp.WorkspaceSecurityAuthority{Identity: identity, Observer: filestore.CaseBindingReader{}, RiskAuthority: risk}
	scope.Principal, err = identity.ResolveCurrent(ctx)
	if err != nil {
		t.Fatal("real_installation_identity_unavailable")
	}
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: p.handler.turnSecurity, Thread: thread,
		ThreadID: scope.ThreadID, TurnID: "manual-begin-completed-turn",
		Workspace: scope.Workspace, Principal: scope.Principal, IssuedAt: time.Now().UTC(),
	})
	if err != nil || !domainsecurity.TurnSecurityContextIsGeneral(frozen) {
		t.Fatal("normal_general_freeze_failed")
	}
	record := turnsecurityapp.PublicRecord(frozen)
	if p.handler.store.AppendTurnToThread(scope.ThreadID, map[string]any{
		"id": frozen.TurnID, "threadId": scope.ThreadID, "status": "completed",
		"items": []any{}, "securityContext": record,
	}, "", map[string]any{"securityState": record}) != nil {
		t.Fatal("normal_completed_turn_owner_failed")
	}
	const fileName, createOperation = "BlankWordCreated.docx", "native_blank_create_assembly_03"
	expectedPath := filepath.Join(scope.Workspace, fileName)
	if _, err := os.Lstat(expectedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new_blank_target_not_absent")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("private_root_unavailable")
	}
	files, err := filestore.NewOfficeObjectEditingFiles(root, nil, "docx")
	if err != nil {
		t.Fatal("real_office_file_owner_unavailable")
	}
	assets, err := officeengineassets.Open(ctx, assetRoot)
	if err != nil {
		t.Fatal("qualified_office_assets_unavailable")
	}
	service := &dshManualBeginObservedService{Service: editingapp.New(identity, files)}
	registry := managededitingapp.New(p.handler.workspaceMutations, managededitingfiles.New())
	p.handler.managedEditing = registry
	adapter := officeeditingapp.New("docx", service, assets.Current)
	if adapter.BindUserCreator(docapp.NewUserCreator(identity, files)) != nil || adapter.BindUserCreationHost(p, officecodec.New(documentcodec.New(codecExecutable, codecEntry)), p.handler.workspaceMutations.Acquire) != nil {
		t.Fatal("real_user_creation_host_binding_failed")
	}
	captureEntered, baselineOK, captureAcquired := false, false, false
	if adapter.BindSelectionHost(p, registry.WithCapture) != nil {
		t.Fatal("real_selection_host_binding_failed")
	}
	if adapter.BindManualHost(func(ctx context.Context, id, path string, readBaseline func() error) (func(), error) {
		captureEntered = true
		release, captureErr := registry.WithManualCapture(ctx, id, path, func() error {
			baselineErr := readBaseline()
			baselineOK = baselineErr == nil
			return baselineErr
		})
		captureAcquired = captureErr == nil
		return release, captureErr
	}) != nil {
		t.Fatal("real_manual_host_binding_failed")
	}
	// Use the same real development-source materialization and signed activation
	// services as production. No fabricated Host state or permissive Admit callback.
	repository, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal("source_root_unavailable")
	}
	source := filepath.Join(repository, "plugins", "analytix-documents")
	observed, err := pluginstore.InspectDevelopmentSourceTreeV1(ctx, source)
	if err != nil {
		t.Fatal("source_registration_unavailable")
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	if err != nil {
		t.Fatal("source_registration_invalid")
	}
	binding, err := materializationapp.NewDevelopmentSourceBindingV1(registration, source, domainplugin.TargetV1{Platform: runtime.GOOS, Arch: runtime.GOARCH})
	if err != nil {
		t.Fatal("source_binding_invalid")
	}
	data, home := filepath.Join(root, "data"), filepath.Join(root, "runtime")
	for _, directory := range []string{data, home} {
		if os.Mkdir(directory, 0700) != nil {
			t.Fatal("isolated_host_directory_unavailable")
		}
	}
	authority, err := pluginauthority.OpenOrCreateV1(data, false)
	if err != nil {
		t.Fatal("isolated_materialization_authority_unavailable")
	}
	state, err := pluginstore.NewPackageStoreV1(home, "analytix-documents", nil)
	if err != nil {
		t.Fatal("isolated_package_store_unavailable")
	}
	materialization, err := materializationapp.NewDevelopmentSourceServiceV1(state, authority, binding, time.Now)
	if err != nil {
		t.Fatal("real_materialization_owner_unavailable")
	}
	intent, err := binding.NewIntentV1(time.Now().UTC())
	if err != nil {
		t.Fatal("source_intent_invalid")
	}
	intent, err = state.PrepareDevelopmentIntentV1(ctx, intent)
	if err != nil {
		t.Fatal("source_intent_prepare_failed")
	}
	installed, err := materialization.Materialize(ctx, intent)
	if err != nil {
		t.Fatal("real_materialization_failed")
	}
	host, err := hostapp.New(identity, authority, []hostapp.Registration{{Identity: registration.Identity,
		SourceRegistrationSHA256: domainpackage.DevelopmentSourceRegistrationSHA256V1(registration),
		Materialization:          materialization, State: state, Adapter: adapter}}, time.Now)
	if err != nil {
		t.Fatal("real_package_host_unavailable")
	}
	view, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-documents", GenerationID: installed.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1})
	if err != nil || !view.Available {
		t.Fatal("real_package_activation_failed")
	}
	p.handler.officePackageHost = host
	mux := httpapi.LocalDisplayMuxV1{RuntimeToken: "manual-begin-synthetic-token", LocalDisplay: httpapi.LocalDisplayHandlerV1{PackageHost: httpapi.PluginPackageHostHandler{Service: host}}}
	closedCode := func(code string) string {
		switch code {
		case "unavailable", "invalid_request", "identity_invalid", "package_not_found", "conflict", "disabled", "adapter_unavailable", "persistence_failure", "session_invalid", "scope_invalid", "draft_stale", "projection_unavailable", "forbidden", "not_text", "too_large", "operation_mismatch", "operation_not_found", "capacity":
			return code
		default:
			return "other"
		}
	}
	invoke := func(operation string, input map[string]any) json.RawMessage {
		t.Helper()
		body, marshalErr := json.Marshal(map[string]any{"action": "invoke", "packageId": view.PackageID, "generationId": view.GenerationID, "expectedRevision": view.ActivationRevision, "contributionId": "workspace-editor", "operation": operation, "input": input})
		if marshalErr != nil {
			t.Fatal("controlled_input_encoding_failed")
		}
		request := httptest.NewRequest(http.MethodPost, httpapi.PluginPackageHostPath, bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer manual-begin-synthetic-token")
		request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, request)
		var envelope struct {
			OK     bool            `json:"ok"`
			Code   string          `json:"code"`
			Output json.RawMessage `json:"output"`
		}
		if json.Unmarshal(reply.Body.Bytes(), &envelope) != nil || reply.Code != http.StatusOK || !envelope.OK || len(envelope.Output) == 0 {
			t.Fatalf("host_response_failed operation=%s http=%d code=%s captureEntered=%t baselineOK=%t captureAcquired=%t prepareEntered=%t prepareOK=%t", operation, reply.Code, closedCode(envelope.Code), captureEntered, baselineOK, captureAcquired, service.prepareEntered, service.prepareOK)
		}
		return envelope.Output
	}
	var created struct {
		OK       bool                 `json:"ok"`
		Code     string               `json:"code"`
		Creation docport.UserCreation `json:"creation"`
	}
	if json.Unmarshal(invoke("create-object", map[string]any{"threadId": scope.ThreadID, "operationId": createOperation, "fileName": fileName}), &created) != nil || !created.OK {
		t.Fatalf("core_blank_create_failed code=%s", closedCode(created.Code))
	}
	creation := created.Creation
	before, err := os.ReadFile(creation.Path)
	if err != nil {
		t.Fatal("created_blank_read_failed")
	}
	digest := sha256.Sum256(before)
	expectedRevision := hex.EncodeToString(digest[:])
	if creation.OperationID != createOperation || creation.Kind != "docx" || creation.ByteSize != len(before) || len(before) == 0 || creation.Revision != expectedRevision {
		t.Fatal("real_creation_receipt_binding_mismatch")
	}
	ownerIdentity, ownerIdentityOK := filestore.ResolveMutationIdentityPath(scope.Workspace, scope.Workspace)
	receiptIdentity, receiptIdentityOK := filestore.ResolveMutationIdentityPath(creation.Workspace, creation.Workspace)
	workspaceIdentityMatches := ownerIdentityOK && receiptIdentityOK && ownerIdentity == receiptIdentity
	workspaceStringMatches := creation.Workspace == scope.Workspace
	expectedInfo, expectedErr := os.Stat(expectedPath)
	createdInfo, createdErr := os.Stat(creation.Path)
	if !workspaceIdentityMatches || !workspaceStringMatches || creation.Path != expectedPath || expectedErr != nil || createdErr != nil || !os.SameFile(expectedInfo, createdInfo) {
		t.Fatalf("created_blank_workspace_binding_mismatch workspaceIdentityMatches=%t workspaceStringMatches=%t", workspaceIdentityMatches, workspaceStringMatches)
	}
	archive, err := zip.NewReader(bytes.NewReader(before), int64(len(before)))
	if err != nil {
		t.Fatal("created_blank_package_invalid")
	}
	bodyFound := false
	for _, part := range archive.File {
		if part.Name != "word/document.xml" {
			continue
		}
		reader, err := part.Open()
		if err != nil {
			t.Fatal("created_blank_body_unavailable")
		}
		decoder := xml.NewDecoder(reader)
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = reader.Close()
				t.Fatal("created_blank_body_invalid")
			}
			if start, ok := token.(xml.StartElement); ok && start.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
				if start.Name.Local == "body" {
					bodyFound = true
				}
				if start.Name.Local == "t" {
					var text string
					if decoder.DecodeElement(&text, &start) != nil || strings.TrimSpace(text) != "" {
						_ = reader.Close()
						t.Fatal("created_word_body_not_blank")
					}
				}
			}
		}
		if reader.Close() != nil {
			t.Fatal("created_blank_body_close_failed")
		}
	}
	if !bodyFound {
		t.Fatal("created_blank_body_missing")
	}
	var replayed struct {
		OK       bool                 `json:"ok"`
		Creation docport.UserCreation `json:"creation"`
	}
	// This is a known committed creation in this isolated case, never the
	// unrelated native attempt whose outcome remains unknown.
	if json.Unmarshal(invoke("create-object", map[string]any{"threadId": scope.ThreadID, "operationId": createOperation, "fileName": fileName}), &replayed) != nil || !replayed.OK || replayed.Creation != creation {
		t.Fatal("committed_creation_replay_changed_receipt")
	}
	t.Logf("CONTROL_BLANK_CREATED creationSHA=true sameFile=true blankBody=true committedReplayPreserved=true workspaceIdentityMatches=%t workspaceStringMatches=%t", workspaceIdentityMatches, workspaceStringMatches)
	var opened struct {
		OK       bool              `json:"ok"`
		Code     string            `json:"code"`
		Document editingapp.Opened `json:"document"`
	}
	if json.Unmarshal(invoke("open-object", map[string]any{"object": map[string]any{"workspace": creation.Workspace, "path": creation.Path}}), &opened) != nil || !opened.OK {
		t.Fatalf("core_blank_open_failed code=%s workspaceIdentityMatches=%t workspaceStringMatches=%t", closedCode(opened.Code), workspaceIdentityMatches, workspaceStringMatches)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			body, _ := json.Marshal(map[string]any{"sessionId": opened.Document.SessionID})
			result, closeErr := host.Invoke(ctx, hostapp.InvokeRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, ContributionID: "workspace-editor", Operation: "close-object", Input: body})
			var reply struct {
				OK     bool `json:"ok"`
				Closed bool `json:"closed"`
			}
			if closeErr != nil || json.Unmarshal(result.Output, &reply) != nil || !reply.OK || !reply.Closed || registry.HasCaptures() {
				t.Error("isolated_session_cleanup_failed")
			}
		}
	})
	openedInfo, openedErr := os.Stat(opened.Document.Path)
	openedBytes, decodeErr := base64.StdEncoding.DecodeString(opened.Document.Content)
	if opened.Document.SessionID == "" || opened.Document.Revision != expectedRevision || openedErr != nil || !os.SameFile(createdInfo, openedInfo) || decodeErr != nil || !bytes.Equal(openedBytes, before) {
		t.Fatal("real_blank_open_receipt_binding_mismatch")
	}
	if err := registry.BeginOpaque(ctx); err != nil {
		t.Fatal("real_opaque_observation_failed")
	}
	var begun struct {
		OK     bool   `json:"ok"`
		Code   string `json:"code"`
		Manual struct {
			Version         int    `json:"version"`
			Origin          string `json:"origin"`
			ChangeID        string `json:"changeId"`
			BaseRevision    string `json:"baseRevision"`
			SaveOperationID string `json:"saveOperationId"`
			UndoOperationID string `json:"undoOperationId"`
		} `json:"manual"`
	}
	beginRaw := invoke("begin-manual-edit", map[string]any{"sessionId": opened.Document.SessionID, "threadId": scope.ThreadID, "operationId": "native_blank_manual_assembly_begin_03", "baseRevision": opened.Document.Revision})
	if json.Unmarshal(beginRaw, &begun) != nil || !begun.OK {
		t.Fatalf("core_begin_failed code=%s creationSHA=true openSHA=true sameFile=true blankBody=true workspaceIdentityMatches=%t workspaceStringMatches=%t captureEntered=%t baselineOK=%t captureAcquired=%t prepareEntered=%t prepareOK=%t", closedCode(begun.Code), workspaceIdentityMatches, workspaceStringMatches, captureEntered, baselineOK, captureAcquired, service.prepareEntered, service.prepareOK)
	}
	var envelopeFields, manualFields map[string]json.RawMessage
	if json.Unmarshal(beginRaw, &envelopeFields) != nil || len(envelopeFields) != 2 || json.Unmarshal(envelopeFields["manual"], &manualFields) != nil || len(manualFields) != 6 {
		t.Fatal("manual_strict_wire_shape_invalid")
	}
	manual := begun.Manual
	if manual.Version != 2 || manual.Origin != "manual" || len(manual.ChangeID) != 64 || manual.BaseRevision != opened.Document.Revision || manual.SaveOperationID != "native_save_"+manual.ChangeID || manual.UndoOperationID != "native_undo_"+manual.ChangeID || !captureEntered || !baselineOK || !captureAcquired || !service.prepareEntered || !service.prepareOK || !registry.HasCaptures() {
		t.Fatalf("prepared_authority_mismatch captureEntered=%t baselineOK=%t captureAcquired=%t prepareEntered=%t prepareOK=%t", captureEntered, baselineOK, captureAcquired, service.prepareEntered, service.prepareOK)
	}
	var recovery struct {
		OK       bool `json:"ok"`
		Recovery struct {
			Pending *struct {
				Version      int    `json:"version"`
				Origin       string `json:"origin"`
				ChangeID     string `json:"changeId"`
				ThreadID     string `json:"threadId"`
				BaseRevision string `json:"baseRevision"`
				Status       string `json:"status"`
			} `json:"pending"`
		} `json:"recovery"`
	}
	if json.Unmarshal(invoke("object-recovery", map[string]any{"sessionId": opened.Document.SessionID, "threadId": scope.ThreadID}), &recovery) != nil || !recovery.OK || recovery.Recovery.Pending == nil {
		t.Fatal("prepared_recovery_missing")
	}
	pending := recovery.Recovery.Pending
	if pending.Version != 2 || pending.Origin != "manual" || pending.ChangeID != manual.ChangeID || pending.ThreadID != scope.ThreadID || pending.BaseRevision != manual.BaseRevision || pending.Status != "prepared" {
		t.Fatal("prepared_recovery_binding_mismatch")
	}
	// Exercise the real Status currentness check that differs from a static
	// receipt fake: after Undo, the historical save is no longer current.
	var candidate bytes.Buffer
	writer := zip.NewWriter(&candidate)
	for _, part := range archive.File {
		// CreateHeader mutates flags and owns the header. Keep the source
		// reader's descriptor expectations unchanged while copying its bytes.
		header := part.FileHeader
		header.Extra = append([]byte(nil), part.Extra...)
		out, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatal("manual_candidate_part_unavailable")
		}
		if part.Name == "word/document.xml" {
			if _, err := io.WriteString(out, `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Manual saved text</w:t></w:r></w:p><w:sectPr/></w:body></w:document>`); err != nil {
				t.Fatal("manual_candidate_body_unavailable")
			}
			continue
		}
		reader, err := part.Open()
		if err != nil {
			t.Fatal("manual_candidate_source_unavailable")
		}
		_, copyErr := io.Copy(out, reader)
		closeErr := reader.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatal("manual_candidate_copy_failed")
		}
	}
	if writer.Close() != nil || bytes.Equal(candidate.Bytes(), before) {
		t.Fatal("manual_candidate_not_changed")
	}
	var saved struct {
		OK      bool             `json:"ok"`
		Receipt fileport.Receipt `json:"receipt"`
	}
	candidateHash := sha256.Sum256(candidate.Bytes())
	content := map[string]any{"encoding": "base64", "kind": "docx", "byteLength": candidate.Len(), "sha256": hex.EncodeToString(candidateHash[:]), "data": base64.StdEncoding.EncodeToString(candidate.Bytes())}
	if json.Unmarshal(invoke("commit-object", map[string]any{"sessionId": opened.Document.SessionID, "threadId": scope.ThreadID, "changeId": manual.ChangeID, "operationId": manual.SaveOperationID, "baseRevision": manual.BaseRevision, "content": content}), &saved) != nil || !saved.OK || saved.Receipt.Status != fileport.StatusCommitted || registry.HasCaptures() {
		t.Fatal("real_manual_save_failed")
	}
	undoInput := map[string]any{"sessionId": opened.Document.SessionID, "threadId": scope.ThreadID, "changeId": manual.ChangeID, "baseRevision": saved.Receipt.Revision}
	for attempt := 0; attempt < 2; attempt++ {
		var undone struct {
			OK      bool             `json:"ok"`
			Receipt fileport.Receipt `json:"receipt"`
		}
		if json.Unmarshal(invoke("undo-change", undoInput), &undone) != nil || !undone.OK || undone.Receipt.Status != fileport.StatusCommitted || undone.Receipt.OperationID != manual.UndoOperationID || undone.Receipt.Revision != manual.BaseRevision || registry.HasCaptures() {
			t.Fatalf("real_manual_undo_failed attempt=%d", attempt)
		}
	}
	t.Log("CONTROL_REAL_MANUAL_SAVE_UNDO_REPLAY opaqueObserved=true saved=true undo=true undoReplay=true")
	var closeReply struct {
		OK     bool `json:"ok"`
		Closed bool `json:"closed"`
	}
	if json.Unmarshal(invoke("close-object", map[string]any{"sessionId": opened.Document.SessionID}), &closeReply) != nil || !closeReply.OK || !closeReply.Closed || registry.HasCaptures() {
		t.Fatal("real_host_close_did_not_release_capture")
	}
	closed = true
	after, readErr := os.ReadFile(creation.Path)
	if readErr != nil || !bytes.Equal(after, before) {
		t.Fatal("created_blank_original_changed")
	}
	t.Logf("CONTROL_REAL_BLANK_MANUAL_BEGIN_PASS hostOK=true coreOK=true creationSHA=true openSHA=true sameFile=true blankBody=true manualBaseMatchesCreation=true workspaceIdentityMatches=%t workspaceStringMatches=%t captureEntered=true baselineOK=true captureAcquired=true prepareEntered=true prepareOK=true authorityMatched=true recoveryPrepared=true captureReleased=true sourceUnchanged=true", workspaceIdentityMatches, workspaceStringMatches)
}

func TestNativeSelectionRestoredDefaultEpoch(t *testing.T) {
	p, original, scope := newSelectionProjectorFixture(t)
	at := time.Date(2026, 10, 17, 1, 2, 3, 456, time.UTC)
	state, err := contextepochapp.DefaultState(scope.ThreadID, 1, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{"discuss", "edit"} {
		thread := contracts.CloneMap(original)
		thread["contextEpochState"] = contextepochapp.PublicState(state)
		scope.Purpose = purpose
		current, err := p.selectionSecurityContext(context.Background(), thread, scope)
		if err != nil || !domainsecurity.TurnSecurityContextIsGeneral(current) || current.ContextEpoch != 1 {
			t.Fatalf("restored_default_epoch_refused purpose=%s", purpose)
		}
	}
	type candidate struct {
		name string
		raw  any
	}
	nonDefault := func(change func(*domaincontextepoch.State)) any {
		value := state
		change(&value)
		value = domaincontextepoch.SealState(value)
		if err := domaincontextepoch.ValidateState(value); err != nil {
			t.Fatal(err)
		}
		return contextepochapp.PublicState(value)
	}
	unknown := contextepochapp.PublicState(state)
	unknown["unknownField"] = true
	damaged := contextepochapp.PublicState(state)
	damaged["stateDigest"] = strings.Repeat("0", 64)
	cases := []candidate{{"null", nil}, {"damaged", damaged}, {"unknown_field", unknown},
		{"other_thread", nonDefault(func(v *domaincontextepoch.State) { v.ThreadID = "thr_other"; v.AcceptedSnapshot.ThreadID = v.ThreadID })},
		{"epoch_two", nonDefault(func(v *domaincontextepoch.State) { v.AcceptedSnapshot.Epoch = 2 })},
		{"baseline", nonDefault(func(v *domaincontextepoch.State) { v.AcceptedSnapshot.BaselineSequence = 1 })},
		{"recovery", nonDefault(func(v *domaincontextepoch.State) { v.AcceptedSnapshot.RecoveryDigest = strings.Repeat("a", 64) })},
		{"impact", nonDefault(func(v *domaincontextepoch.State) { v.AcceptedSnapshot.Impact.TurnTail = true })},
		{"reason", nonDefault(func(v *domaincontextepoch.State) {
			v.AcceptedSnapshot.ChangeReasons = []domaincontextepoch.ChangeReason{domaincontextepoch.ReasonRestartReconcile}
		})},
	}
	entry := domaincontextepoch.SourceEntry{Version: 1, SourceID: "synthetic-source", Kind: "security-context", Digest: strings.Repeat("b", 64), Sequence: 1, TrustState: domaincontextepoch.TrustTrusted, PromptBoundary: domaincontextepoch.BoundaryStoreOnly, ActivationState: domaincontextepoch.ActivationInactive}
	withSource, err := contextepochapp.BootstrapState(scope.ThreadID, 1, []domaincontextepoch.SourceEntry{entry}, at)
	if err != nil {
		t.Fatal(err)
	}
	cases = append(cases, candidate{"source", contextepochapp.PublicState(withSource)})
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			thread := contracts.CloneMap(original)
			thread["contextEpochState"] = test.raw
			if _, err := p.selectionSecurityContext(context.Background(), thread, scope); !errors.Is(err, editingapp.ErrProjection) {
				t.Fatal("non_default_epoch_authorized")
			}
		})
	}
}

// Exercise the restart/authority boundary through the real production binding,
// signed package host and protected HTTP lane. No Office engine or Provider runs.
func TestOfficeManualRestartRecoveryLifecycle(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	path := filepath.Join(workspace, "RestartManual.docx")
	nativeBytes := func(text string) []byte {
		var raw bytes.Buffer
		writer := zip.NewWriter(&raw)
		parts := []struct{ name, body string }{
			{"[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
			{"_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
			{"word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`},
		}
		for _, part := range parts {
			out, err := writer.Create(part.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(out, part.body); err != nil {
				t.Fatal(err)
			}
		}
		if writer.Close() != nil {
			t.Fatal("synthetic_docx_close_failed")
		}
		return raw.Bytes()
	}
	before, after := nativeBytes("Before restart"), nativeBytes("Saved after recovery")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"office", "runtime"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal("private_fixture_directory_unavailable")
		}
	}
	config := RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DataDir: filepath.Join(root, "data"), DurableTempDir: filepath.Join(root, "events")}
	repository, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repository, "plugins", "analytix-documents")
	observed, err := pluginstore.InspectDevelopmentSourceTreeV1(ctx, source)
	if err != nil {
		t.Fatal("source_registration_unavailable")
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	if err != nil {
		t.Fatal("source_registration_invalid")
	}
	binding, err := materializationapp.NewDevelopmentSourceBindingV1(registration, source, domainplugin.TargetV1{Platform: runtime.GOOS, Arch: runtime.GOARCH})
	if err != nil {
		t.Fatal("source_binding_invalid")
	}
	first := true
	type reply struct {
		OK       bool                    `json:"ok"`
		Code     string                  `json:"code"`
		Closed   bool                    `json:"closed"`
		Document editingapp.Opened       `json:"document"`
		Recovery fileport.NativeRecovery `json:"recovery"`
		Receipt  fileport.Receipt        `json:"receipt"`
		Manual   struct {
			Version         int    `json:"version"`
			Origin          string `json:"origin"`
			ChangeID        string `json:"changeId"`
			BaseRevision    string `json:"baseRevision"`
			SaveOperationID string `json:"saveOperationId"`
		} `json:"manual"`
	}
	type instance struct {
		h      *runtimeServerHandler
		invoke func(string, map[string]any) reply
		stop   func()
	}
	boot := func() instance {
		base := NewRuntimeServerHandler(config).(*runtimeServerHandler)
		identity := base.turnSecurity.Identity
		files, err := filestore.NewOfficeObjectEditingFiles(filepath.Join(root, "office"), nil, "docx")
		if err != nil {
			t.Fatal("office_files_unavailable")
		}
		adapter := officeeditingapp.New("docx", editingapp.New(identity, files), func(current context.Context) bool { return current != nil && current.Err() == nil })
		authority, err := pluginauthority.OpenOrCreateV1(config.DataDir, false)
		if err != nil {
			t.Fatal("materialization_authority_unavailable")
		}
		state, err := pluginstore.NewPackageStoreV1(filepath.Join(root, "runtime"), "analytix-documents", nil)
		if err != nil {
			t.Fatal("package_store_unavailable")
		}
		materialization, err := materializationapp.NewDevelopmentSourceServiceV1(state, authority, binding, time.Now)
		if err != nil {
			t.Fatal("materialization_unavailable")
		}
		var generation string
		if first {
			intent, err := binding.NewIntentV1(time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			intent, err = state.PrepareDevelopmentIntentV1(ctx, intent)
			if err != nil {
				t.Fatal(err)
			}
			installed, err := materialization.Materialize(ctx, intent)
			if err != nil {
				t.Fatal("materialization_failed")
			}
			generation = installed.Receipt.GenerationID
		}
		host, err := hostapp.New(identity, authority, []hostapp.Registration{{Identity: registration.Identity, SourceRegistrationSHA256: domainpackage.DevelopmentSourceRegistrationSHA256V1(registration), Materialization: materialization, State: state, Adapter: adapter}}, time.Now)
		if err != nil {
			t.Fatal("package_host_unavailable")
		}
		risk, err := riskapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
		if err != nil {
			t.Fatal(err)
		}
		value, err := NewRuntimeServerHandlerFromComponents(config, RuntimeServerComponents{
			Store: base.store, Memories: base.memories, CaseFinalizer: base.caseFinalizer, Continuations: base.continuations, PendingWork: base.pendingWork, Checkpoints: base.checkpoints,
			NativeAuthority: base.nativeAuthority, SteeringAuthority: base.steeringAuthority, SubagentState: base.subagentState, G6Readiness: base.g6Readiness,
			TurnSecurity:        turnsecurityapp.WorkspaceSecurityAuthority{Identity: identity, Observer: filestore.CaseBindingReader{}, RiskAuthority: risk},
			ManagedEditingFiles: managededitingfiles.New(), OfficeAdapters: map[string]adapterport.Adapter{"analytix-documents": adapter}, OfficePackageHost: host, SkipMCPConnect: true,
		})
		if err != nil {
			t.Fatal("production_runtime_assembly_failed", err)
		}
		h := value.(*runtimeServerHandler)
		if first {
			if _, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-documents", GenerationID: generation, DesiredState: domainplugin.DesiredEnabledV1}); err != nil {
				t.Fatal("package_activation_failed")
			}
			first = false
		}
		views, err := host.List(ctx)
		if err != nil || len(views) != 1 || !views[0].Available {
			t.Fatal("package_binding_unavailable")
		}
		view := views[0]
		mux := httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, Next: h, LocalDisplay: httpapi.LocalDisplayHandlerV1{PackageHost: httpapi.PluginPackageHostHandler{Service: host}}}
		sessions := map[string]bool{}
		invoke := func(operation string, input map[string]any) reply {
			t.Helper()
			body, err := json.Marshal(map[string]any{"action": "invoke", "packageId": view.PackageID, "generationId": view.GenerationID, "expectedRevision": view.ActivationRevision, "contributionId": "workspace-editor", "operation": operation, "input": input})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, httpapi.PluginPackageHostPath, bytes.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
			request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			var envelope struct {
				OK     bool            `json:"ok"`
				Output json.RawMessage `json:"output"`
			}
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &envelope) != nil || !envelope.OK {
				t.Fatalf("protected_host_call_failed operation=%s http=%d", operation, response.Code)
			}
			var result reply
			if json.Unmarshal(envelope.Output, &result) != nil {
				t.Fatal("native_reply_invalid")
			}
			if result.OK && operation == "open-object" {
				sessions[result.Document.SessionID] = true
			}
			if result.OK && operation == "close-object" {
				delete(sessions, input["sessionId"].(string))
			}
			return result
		}
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			for id := range sessions {
				if result := invoke("close-object", map[string]any{"sessionId": id}); !result.OK || !result.Closed {
					t.Error("session_cleanup_failed")
				}
			}
			if h.managedEditing.HasCaptures() {
				t.Error("capture_survived_close")
			}
			if h.Shutdown(ctx) != nil {
				t.Error("runtime_cleanup_failed")
			}
			stopped = true
		}
		t.Cleanup(stop)
		return instance{h, invoke, stop}
	}
	assertOK := func(stage string, result reply) reply {
		t.Helper()
		if !result.OK {
			t.Fatalf("%s code=%s", stage, result.Code)
		}
		return result
	}
	current := boot()
	thread, err := current.h.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	openInput := map[string]any{"object": map[string]any{"workspace": workspace, "path": path}}
	opened := assertOK("open_before_restart", current.invoke("open-object", openInput)).Document
	beginInput := func(session, operation, revision string) map[string]any {
		return map[string]any{"sessionId": session, "threadId": threadID, "operationId": operation, "baseRevision": revision}
	}
	beginA := beginInput(opened.SessionID, "manual_restart_begin_A", opened.Revision)
	a := assertOK("begin_before_restart", current.invoke("begin-manual-edit", beginA)).Manual
	if a.Version != 2 || a.Origin != "manual" || !current.h.managedEditing.HasCaptures() {
		t.Fatal("manual_A_not_established")
	}
	assertOK("close_before_restart", current.invoke("close-object", map[string]any{"sessionId": opened.SessionID}))
	current.stop()
	current = boot()
	restored, err := current.h.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := restored["securityState"]; exists || len(restored["turns"].([]any)) != 0 || restored["contextEpochState"] == nil {
		t.Fatal("normal_restore_did_not_produce_epoch_only_thread")
	}
	epochBefore := contracts.CloneMap(restored["contextEpochState"].(map[string]any))
	reopened := assertOK("open_after_restart", current.invoke("open-object", openInput)).Document
	if reopened.SessionID == opened.SessionID || reopened.ObjectID != opened.ObjectID || reopened.Revision != opened.Revision {
		t.Fatal("restart_object_session_binding_invalid")
	}
	recoveryInput := map[string]any{"sessionId": reopened.SessionID, "threadId": threadID}
	recovered := assertOK("recovery_after_restart", current.invoke("object-recovery", recoveryInput)).Recovery
	if recovered.Pending == nil || recovered.Pending.ChangeID != a.ChangeID || recovered.Pending.Status != "prepared" || !recovered.Pending.CanCancel {
		t.Fatal("pending_A_not_recoverable")
	}
	assertUnchanged := func() {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, before) {
			t.Fatal("recovery_or_cancel_changed_document")
		}
	}
	assertUnchanged()
	cancelA := map[string]any{"sessionId": reopened.SessionID, "threadId": threadID, "changeId": a.ChangeID, "baseRevision": reopened.Revision}
	wrongRevision := contracts.CloneMap(cancelA)
	wrongRevision["baseRevision"] = strings.Repeat("0", 64)
	if result := current.invoke("cancel-change", wrongRevision); result.OK || result.Code != "conflict" {
		t.Fatal("stale_revision_cancel_authorized")
	}
	other, err := current.h.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	wrongThread := contracts.CloneMap(cancelA)
	wrongThread["threadId"] = stringField(other, "id")
	if result := current.invoke("cancel-change", wrongThread); result.OK || result.Code != "operation_mismatch" {
		t.Fatal("foreign_thread_cancel_authorized")
	}
	risk := current.h.turnSecurity.RiskAuthority
	current.h.turnSecurity.RiskAuthority = nil
	for _, call := range []struct {
		operation string
		input     map[string]any
	}{{"object-recovery", recoveryInput}, {"cancel-change", cancelA}, {"begin-manual-edit", beginInput(reopened.SessionID, "manual_revoked_begin", reopened.Revision)}} {
		if result := current.invoke(call.operation, call.input); result.OK || result.Code != "projection_unavailable" {
			t.Fatal("revoked_authority_authorized", call.operation)
		}
	}
	current.h.turnSecurity.RiskAuthority = risk
	if pending := assertOK("recovery_after_refusals", current.invoke("object-recovery", recoveryInput)).Recovery.Pending; pending == nil || pending.ChangeID != a.ChangeID {
		t.Fatal("refusal_changed_pending_A")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if state := assertOK("cancel_A", current.invoke("cancel-change", cancelA)).Recovery; state.Pending != nil {
			t.Fatal("cancel_A_did_not_settle")
		}
	}
	assertUnchanged()
	beginA["sessionId"] = reopened.SessionID
	if result := current.invoke("begin-manual-edit", beginA); result.OK || result.Code != "operation_mismatch" || current.h.managedEditing.HasCaptures() {
		t.Fatal("cancelled_begin_A_revived")
	}
	b := assertOK("begin_B", current.invoke("begin-manual-edit", beginInput(reopened.SessionID, "manual_restart_begin_B", reopened.Revision))).Manual
	if b.ChangeID == a.ChangeID || !current.h.managedEditing.HasCaptures() {
		t.Fatal("manual_B_not_established")
	}
	state := assertOK("late_cancel_A", current.invoke("cancel-change", cancelA)).Recovery
	if state.Pending == nil || state.Pending.ChangeID != b.ChangeID || !current.h.managedEditing.HasCaptures() {
		t.Fatal("late_cancel_A_retired_B")
	}
	content := map[string]any{"encoding": "base64", "kind": "docx", "byteLength": len(after), "sha256": domainsecurity.SHA256Hex(after), "data": base64.StdEncoding.EncodeToString(after)}
	commit := map[string]any{"sessionId": reopened.SessionID, "threadId": threadID, "changeId": a.ChangeID, "operationId": a.SaveOperationID, "baseRevision": a.BaseRevision, "content": content}
	if result := current.invoke("commit-object", commit); result.OK || result.Code != "operation_mismatch" || !current.h.managedEditing.HasCaptures() {
		t.Fatal("cancelled_commit_A_affected_B")
	}
	if result := current.invoke("begin-manual-edit", beginA); result.OK || result.Code != "conflict" || !current.h.managedEditing.HasCaptures() {
		t.Fatal("late_begin_A_affected_B")
	}
	commit["changeId"], commit["operationId"], commit["baseRevision"] = b.ChangeID, b.SaveOperationID, b.BaseRevision
	saved := assertOK("save_B", current.invoke("commit-object", commit)).Receipt
	if saved.Status != fileport.StatusCommitted || saved.Revision != domainsecurity.SHA256Hex(after) || current.h.managedEditing.HasCaptures() {
		t.Fatal("save_B_not_settled")
	}
	status := assertOK("status_B", current.invoke("object-status", map[string]any{"sessionId": reopened.SessionID, "operationId": b.SaveOperationID})).Receipt
	if status != saved {
		t.Fatal("saved_status_changed_receipt")
	}
	assertOK("close_saved_B", current.invoke("close-object", map[string]any{"sessionId": reopened.SessionID}))
	final := assertOK("reopen_saved_B", current.invoke("open-object", openInput)).Document
	finalRecovery := assertOK("recovery_saved_B", current.invoke("object-recovery", map[string]any{"sessionId": final.SessionID, "threadId": threadID})).Recovery
	if finalRecovery.Pending != nil || finalRecovery.Current == nil || finalRecovery.Current.ChangeID != b.ChangeID || finalRecovery.Current.Status != "committed" || finalRecovery.Current.Revision != saved.Revision || finalRecovery.Current.CanCancel || final.Revision != saved.Revision {
		t.Fatal("reopened_saved_B_not_current")
	}
	finalThread, err := current.h.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := finalThread["securityState"]; exists || len(finalThread["turns"].([]any)) != 0 || !reflect.DeepEqual(finalThread["contextEpochState"], epochBefore) {
		t.Fatal("manual_recovery_changed_thread_history_or_epoch")
	}
	current.stop()
	t.Log("RESTART_MANUAL_CHAIN_PASS freshThread=true productionBind=true restoredEpoch=true freshSession=true pendingA=true cancelCAS=true cancelReplay=true beginB=true oldAIsolated=true saved=true status=true reopened=true historyUnchanged=true")
}
