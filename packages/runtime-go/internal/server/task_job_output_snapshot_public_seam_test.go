package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	snapshotstore "analytix.local/runtime-go/internal/adapters/outbound/toolresultsnapshot"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	jobs "analytix.local/runtime-go/internal/jobs"
	"analytix.local/runtime-go/internal/ports"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func assertProtectedJobSourceIdentityV1(t *testing.T, h *runtimeServerHandler, parent runtimePendingToolCall, record domainjob.Record) {
	t.Helper()
	binding := record.SecurityBinding
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(context.Background(), record.ParentThreadID, binding.ParentWorkspaceRealPath, binding.TenantID, binding.UserID)
	if err != nil || release == nil {
		t.Fatal("source test could not acquire original scope", err)
	}
	defer release()
	source, err := h.toolSnapshots.Read(context.Background(), record.ProtectedOutputSnapshot.Binding)
	if err != nil {
		t.Fatal("real source CAS was not readable", err)
	}
	if _, ok := h.resolveRuntimeProtectedJobOutputV1(context.Background(), record); !ok {
		t.Fatal("real producer source was unavailable")
	}
	if _, ok := h.sealProtectedToolSnapshotV1(context.Background(), parent, source.Capture); ok {
		t.Fatal("producer source was admitted as original call's display result")
	}
	for _, mutation := range []string{"wrong-job", "wrong-grant", "wrong-tool", "missing-cas"} {
		if mutation == "wrong-tool" && record.Kind != "subagent" {
			continue // The closed shell source has only its exact bash producer.
		}
		candidate := record
		candidate.ProtectedOutputSnapshot = domainjob.CloneProtectedOutputSnapshotV1(record.ProtectedOutputSnapshot)
		if mutation == "missing-cas" {
			candidate.ProtectedOutputSnapshot.Binding.SnapshotDigest = strings.Repeat("f", 64)
		} else {
			wrong := source
			inspect := *source.Inspect
			wrong.Inspect = &inspect
			job := *source.Capture.Job
			job.Jobs = append([]domaintoolresult.ProtectedJobRecordV1(nil), job.Jobs...)
			wrong.Capture.Job = &job
			switch mutation {
			case "wrong-job":
				wrong.Capture.Job.Jobs[0].JobID = "another-real-looking-job"
			case "wrong-grant":
				wrong.ExecutionGrantID = strings.Repeat("f", 64)
			case "wrong-tool":
				wrong.ToolName = "delegate_task"
				if source.ToolName == wrong.ToolName {
					wrong.ToolName = "task"
				}
				wrong.Inspect.SchemaJSON = ""
			}
			ref, err := h.toolSnapshots.Put(context.Background(), wrong)
			if err != nil {
				t.Fatal("negative source fixture was not CAS-valid", mutation, err)
			}
			candidate.ProtectedOutputSnapshot.Binding = ref
		}
		if _, ok := h.resolveRuntimeProtectedJobOutputV1(context.Background(), candidate); ok {
			t.Fatal("untrusted source reference released body", mutation)
		}
	}
}

func assertMissingProtectedJobOutputQueryV1(t *testing.T, h *runtimeServerHandler, parent runtimePendingToolCall, record domainjob.Record, marker, suffix string) {
	t.Helper()
	if marker == "" || record.ProtectedOutputSnapshot != nil || record.Output != "" {
		t.Fatal("failure fixture invented a retained output source")
	}
	pending := protectedJobOutputPendingV1(t, h, parent, record.ID, suffix)
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("real unavailable output query did not settle", err)
	}
	modelOutput, err := json.Marshal(settled.Output)
	if err != nil || strings.Contains(string(modelOutput), marker) {
		t.Fatal("failed capture released body into the model-facing query result", err)
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Job == nil || len(display.Capture.Job.Jobs) != 1 || display.Capture.Job.Jobs[0].JobID != record.ID {
		t.Fatal("failed capture lost the independently observed Job")
	}
	output := display.Capture.Job.Jobs[0].Output
	if output.Availability != "unavailable" || output.Body != nil || output.Truncated != nil {
		t.Fatal("failed capture became an empty or available producer body")
	}
	assertPublicToolSeamV1(t, h, pending, marker)
}

var errProtectedJobOutputCommitAcknowledgementV1 = errors.New("controlled protected Job output commit acknowledgement failure")

type protectedJobOutputCASFaultV1 struct {
	snapshotport.Store
	callID string
	commit bool
	mu     sync.Mutex
	puts   int
	sealed domaintoolresult.ProtectedSnapshotBindingV1
}

func (s *protectedJobOutputCASFaultV1) Put(ctx context.Context, snapshot domaintoolresult.ProtectedSnapshotV1) (domaintoolresult.ProtectedSnapshotBindingV1, error) {
	if snapshot.CallID != s.callID || snapshot.Capture.Job == nil || snapshot.Capture.Job.Operation != "output" {
		return s.Store.Put(ctx, snapshot)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	if s.commit {
		binding, err := s.Store.Put(ctx, snapshot)
		if err != nil {
			return binding, err
		}
		s.sealed = binding
	}
	return domaintoolresult.ProtectedSnapshotBindingV1{}, errProtectedJobOutputCommitAcknowledgementV1
}

type protectedJobOutputReferenceFaultV1 struct {
	ports.JobRepository
	commit bool
	mu     sync.Mutex
	writes int
}

func (s *protectedJobOutputReferenceFaultV1) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	if request.ProtectedOutputSnapshot == nil {
		return s.JobRepository.UpdateChildRun(id, request)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	if s.commit {
		record, err := s.JobRepository.UpdateChildRun(id, request)
		if err != nil {
			return record, err
		}
		return record, errProtectedJobOutputCommitAcknowledgementV1
	}
	record, err := s.JobRepository.LoadChildRun(id)
	if err != nil {
		return record, err
	}
	return record, errProtectedJobOutputCommitAcknowledgementV1
}

func TestProtectedJobOutputCASCommitUncertaintyKeepsUnavailableV1(t *testing.T) {
	const marker = "JOB_CAS_COMMIT_SOURCE_V1"
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "acknowledgement-lost"}[commit], func(t *testing.T) {
			h, parent, durable, root := protectedSeamFixtureV1(t, "bash", map[string]any{"command": "printf '" + marker + "'", "run_in_background": true})
			if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent.SecurityContext, time.Second); err != nil {
				t.Fatal(err)
			}
			realStore := h.toolSnapshots
			fault := &protectedJobOutputCASFaultV1{Store: realStore, callID: parent.Call.ID, commit: commit}
			h.toolSnapshots = fault
			if settled, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil || settled.IsError {
				t.Fatal("real shell did not start", err)
			}
			records, err := h.jobs.List(parent.ThreadID)
			if err != nil || len(records) != 1 {
				t.Fatal("real shell Job is missing", err)
			}
			if settled, err := h.runtimeSubagentState().WaitBackgroundJobSettled(records[0].ID, 5*time.Second); err != nil || !settled {
				t.Fatal("real shell did not settle", err)
			}
			record, err := h.jobs.LoadChildRun(records[0].ID)
			fault.mu.Lock()
			puts, sealed := fault.puts, fault.sealed
			fault.mu.Unlock()
			if err != nil || record.Status != "completed" || record.ProtectedOutputSnapshot != nil || record.Output != "" || puts != 1 {
				t.Fatal("CAS failure changed terminal facts or retried capture", err)
			}
			if commit {
				orphan, err := realStore.Read(context.Background(), sealed)
				if err != nil || orphan.Capture.Job == nil || len(orphan.Capture.Job.Jobs) != 1 || orphan.Capture.Job.Jobs[0].Output.Body == nil || *orphan.Capture.Job.Jobs[0].Output.Body != marker {
					t.Fatal("controlled lost acknowledgement did not leave the actual orphan source", err)
				}
			}
			h.toolSnapshots = realStore
			reopenProtectedJobSnapshotOwnersV1(t, h, root, durable)
			cold, err := h.jobs.LoadChildRun(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertMissingProtectedJobOutputQueryV1(t, h, parent, cold, marker, "cas-uncertain-"+record.ID)
			if fault.puts != 1 {
				t.Fatal("query retried the uncertain producer capture")
			}
		})
	}
}

func TestProtectedJobOutputReferenceCommitUncertaintyUsesColdDurableTruthV1(t *testing.T) {
	const marker = "JOB_REFERENCE_COMMIT_SOURCE_V1"
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "acknowledgement-lost"}[commit], func(t *testing.T) {
			args := map[string]any{"command": "printf '" + marker + "'", "run_in_background": true}
			h, parent, durable, root := protectedSeamFixtureV1(t, "bash", args)
			if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent.SecurityContext, time.Second); err != nil {
				t.Fatal(err)
			}
			// The real terminal use case and native runner receive the existing
			// Manager through its port. Only terminal+ref acknowledgement faults.
			fault := &protectedJobOutputReferenceFaultV1{JobRepository: h.jobs, commit: commit}
			service := terminalapp.NewService(terminalapp.Dependencies{Jobs: fault, ShellRunner: h.shellRunner})
			toolContext, err := runtimeBashToolContext(parent)
			if err != nil {
				t.Fatal(err)
			}
			toolContext.ParentGoalID, toolContext.ParentGoalObjective = h.parentGoalForSubagent(parent.ThreadID)
			toolContext.ProtectedReadDirs = filestore.EffectiveProcessProtectedRoots(h.protectedReadDirs, toolContext.SandboxMode)
			if _, failed := service.ExecuteBashTool(context.Background(), terminalapp.BashToolRunRequest{Context: toolContext, Args: args}, h.runtimeBashToolCallbacks(parent)); failed {
				t.Fatal("real terminal use case did not start")
			}
			records, err := h.jobs.List(parent.ThreadID)
			if err != nil || len(records) != 1 {
				t.Fatal("real terminal Job missing", err)
			}
			if settled, err := h.runtimeSubagentState().WaitBackgroundJobSettled(records[0].ID, 5*time.Second); err != nil || !settled {
				t.Fatal("real worker did not settle", err)
			}
			fault.mu.Lock()
			writes := fault.writes
			fault.mu.Unlock()
			if writes != 1 {
				t.Fatal("terminal source reference was silently retried")
			}
			path := filepath.Join(h.dataDir, "child-runs", records[0].ID+".json")
			before, err := os.ReadFile(path)
			if err != nil || strings.Contains(string(before), marker) {
				t.Fatal("durable Job retained producer text", err)
			}
			reopenProtectedJobSnapshotOwnersV1(t, h, root, durable)
			cold, err := h.jobs.LoadChildRun(records[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if commit {
				if cold.Status != "completed" || cold.ProtectedOutputSnapshot == nil {
					t.Fatal("cold read lost the actual acknowledged-uncertain commit")
				}
				assertProtectedJobOutputQueryBindingV1(t, h, parent, cold, marker, marker, "ref-committed")
			} else {
				if cold.Status != "running" || cold.ProtectedOutputSnapshot != nil {
					t.Fatal("failed ref commit fabricated a terminal retained source")
				}
				assertMissingProtectedJobOutputQueryV1(t, h, parent, cold, marker, "ref-rejected")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) || fault.writes != 1 {
				t.Fatal("cold query rewrote or retried an uncertain Job reference", err)
			}
		})
	}
}

func protectedJobOutputPendingV1(t *testing.T, h *runtimeServerHandler, parent runtimePendingToolCall, jobID, suffix string) runtimePendingToolCall {
	t.Helper()
	pending := parent
	raw, _ := json.Marshal(map[string]any{"job_id": jobID})
	pending.Call = domainmodel.ToolCall{ID: serverTestHostToolCallID("protected_job_output_" + suffix), Name: "bash_output", Arguments: raw}
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, parent.ToolScope, false, false, parent.Prompt, false, nil)
	pending.ModelVisibleToolDefinition = domainmodel.FreezeModelVisibleToolDefinitionsV1(schemas)[pending.Call.Name]
	readOnly, _ := h.runtimeToolPolicy(pending.Call.Name)
	var err error
	pending.ExecutionGrant, err = executiongrantapp.IssueProvider(parent.SecurityContext, "synthetic", pending.Call, schemas, parent.ToolScope, readOnly, "not_required", 0, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	pending.ToolCallItemID, err = h.persistToolCallReady(context.Background(), parent.ThreadID, parent.TurnID, pending.Call, 1, parent.SecurityContext, pending.ExecutionGrant)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.authorizeRuntimePending(context.Background(), pending, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return pending
}

func assertProtectedJobOutputQueryBindingV1(t *testing.T, h *runtimeServerHandler, parent runtimePendingToolCall, record domainjob.Record, expected, marker, suffix string) {
	t.Helper()
	if marker == "" || !strings.Contains(expected, marker) || record.Output != "" || record.ProtectedOutputSnapshot == nil ||
		record.SecurityBinding == nil || record.SecurityBinding.ParentToolCallID != parent.Call.ID || record.SecurityBinding.ParentExecutionGrantID != parent.ExecutionGrant.GrantID {
		t.Fatal("query fixture lacks the exact retained producer and independent marker")
	}
	pending := protectedJobOutputPendingV1(t, h, parent, record.ID, suffix)
	if pending.Call.ID == parent.Call.ID || pending.ExecutionGrant.GrantID == parent.ExecutionGrant.GrantID {
		t.Fatal("query borrowed the original producer identity")
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
	if err != nil || settled.IsError {
		t.Fatal("real output query did not settle", err)
	}
	modelOutput, err := json.Marshal(settled.Output)
	if err != nil || strings.Contains(string(modelOutput), marker) {
		t.Fatal("raw producer body entered the model-facing query result", err)
	}
	status, display, _ := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.ToolName != "bash_output" || display.ResultItemID != domaintoolresult.ToolResultItemIDV1(pending.TurnID, pending.Call.ID) ||
		display.Capture.Kind != "job" || display.Capture.Job == nil || display.Capture.Job.Operation != "output" || display.Capture.Job.Total != 1 || len(display.Capture.Job.Jobs) != 1 || display.Capture.Job.Receipt != nil {
		t.Fatal("protected query lost its actual query call/result binding")
	}
	observed := display.Capture.Job.Jobs[0]
	if observed.JobID != record.ID || observed.Kind != record.Kind || observed.Status != record.Status || observed.Output.Availability != "available" ||
		observed.Output.Body == nil || *observed.Output.Body != expected || observed.Output.Truncated == nil || *observed.Output.Truncated {
		t.Fatal("query did not read this producer's exact body")
	}
	assertPublicToolSeamV1(t, h, pending, marker)
}

func reopenProtectedJobSnapshotOwnersV1(t *testing.T, h *runtimeServerHandler, root, durable string) {
	t.Helper()
	if err := h.toolSnapshots.(interface{ Close() error }).Close(); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := snapshotstore.NewStore(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	h.toolSnapshots = reopened
	h.store, err = NewTempDurableEventSessionStore(durable)
	if err != nil {
		t.Fatal(err)
	}
	h.threads = nil
	h.store.SetCaseThreadAuthority(h.caseThreads)
	reader, err := finalauthority.NewAcceptedFinalCASReader(h.store.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.BindPrimaryThreadReaderV1(reader); err != nil {
		t.Fatal(err)
	}
	// Recover only the already-recorded Job. Never reexecute its producer.
	h.jobs, err = jobs.NewManager(filepath.Join(h.dataDir, "child-runs"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestSpecializedActualBackgroundJobOutputProductionSeamV1(t *testing.T) {
	for _, test := range []struct {
		name, command, body, status, marker string
		timeout                             int
		truncated                           bool
	}{
		{"body", "printf 'JOB_NATIVE_BODY_V1\\nline 2\\n'", "JOB_NATIVE_BODY_V1\nline 2\n", "completed", "JOB_NATIVE_BODY_V1", 10, false},
		{"empty", ":", "", "completed", "", 10, false},
		{"truncated", "i=0; while [ $i -lt 12000 ]; do printf '界'; i=$((i+1)); done", strings.Repeat("界", domaintoolresult.ProtectedSnapshotBodyLimitV1/3), "completed", "界界界界界界界界界界界界", 10, true},
		{"timeout", "printf 'JOB_TIMEOUT_PREFIX_V1'; sleep 2", "JOB_TIMEOUT_PREFIX_V1", "timeout", "JOB_TIMEOUT_PREFIX_V1", 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, parent, durable, root := protectedSeamFixtureV1(t, "bash", map[string]any{"command": test.command, "run_in_background": true, "timeout": test.timeout})
			if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent.SecurityContext, time.Second); err != nil {
				t.Fatal(err)
			}
			if result, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil || result.IsError {
				t.Fatal("actual background producer did not start", err)
			}
			records, err := h.jobs.List(parent.ThreadID)
			if err != nil || len(records) != 1 {
				t.Fatal("actual shell did not produce one Job", err)
			}
			id := records[0].ID
			if settled, err := h.runtimeSubagentState().WaitBackgroundJobSettled(id, 5*time.Second); err != nil || !settled {
				t.Fatal("actual shell worker did not settle", err)
			}
			record, err := h.jobs.LoadChildRun(id)
			if err != nil || record.Status != test.status || record.Output != "" || record.ProtectedOutputSnapshot == nil {
				t.Fatal("real shell body missing from private completion source", err)
			}
			assertProtectedJobSourceIdentityV1(t, h, parent, record)
			// Cold owners can read the existing producer CAS source before the
			// first bash_output query. No new command produces historical bytes.
			reopenProtectedJobSnapshotOwnersV1(t, h, root, durable)
			pending := protectedJobOutputPendingV1(t, h, parent, id, test.name)
			result, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil || result.IsError {
				t.Fatal("protected output query failed", err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.Capture.Job == nil || len(display.Capture.Job.Jobs) != 1 {
				t.Fatal("real protected reader lost Job capture")
			}
			output := display.Capture.Job.Jobs[0].Output
			if output.Availability != "available" || output.Body == nil || output.Truncated == nil || *output.Truncated != test.truncated || !utf8.ValidString(*output.Body) {
				t.Fatal("body availability/truncation was fabricated")
			}
			if *output.Body != test.body {
				t.Fatal("protected body differs from actual shell output")
			}
			if test.marker != "" {
				modelOutput, err := json.Marshal(result.Output)
				if err != nil || strings.Contains(string(modelOutput), test.marker) {
					t.Fatal("producer marker entered model-facing output", err)
				}
				assertPublicToolSeamV1(t, h, pending, test.marker)
			}
			reopenProtectedJobSnapshotOwnersV1(t, h, root, durable)
			if status, after, _ := readProtectedSeamV1(t, h, pending); status != http.StatusOK || after.SnapshotDigest != display.SnapshotDigest {
				t.Fatal("cold reader lost immutable query body")
			}
			// A newer parent turn revokes querying the old bound Job source.
			thread, err := h.store.GetThread(parent.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			_ = protectedSeamPendingV1(t, h, thread, "read", map[string]any{"path": "code.go"}, "turn_revokes_"+test.name)
			revokedResult := h.runtimeSubagentService().OutputTaskJobProtectedV1(context.Background(), parent.ThreadID, subagentapp.TaskJobOutputRequest{JobID: id})
			if !revokedResult.IsError || revokedResult.OutputCapture != nil {
				t.Fatal("revoked old Job source remained available")
			}
		})
	}
}

func TestSpecializedActualBackgroundChildOutputColdFirstQueryV1(t *testing.T) {
	const marker = "JOB_BACKGROUND_CHILD_SOURCE_V1"
	h, parent, durable, root := protectedSeamFixtureV1(t, "task", map[string]any{"prompt": "inspect synthetic code", "run_in_background": true})
	providerClient := &providerStepRecordingProvider{responseText: func(int, domainmodel.Request) string { return marker + "\nactual child completion\n" }}
	h.provider = providerClient
	if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), parent.SecurityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	if result, err := h.executeAndSettleRuntimeTool(context.Background(), parent, nil, nil); err != nil || result.IsError {
		t.Fatal("real background child did not start", err)
	}
	records, err := h.jobs.List(parent.ThreadID)
	if err != nil || len(records) != 1 {
		t.Fatal("real background child association missing", err)
	}
	// Wait for the durable child pipeline and worker tail writes; this fixture
	// verifies retained output, without asserting a five-second latency bound.
	if settled, err := h.runtimeSubagentState().WaitBackgroundJobSettled(records[0].ID, 30*time.Second); err != nil || !settled {
		t.Fatal("real background child did not settle", err)
	}
	record, err := h.jobs.LoadChildRun(records[0].ID)
	if err != nil || record.Status != "completed" || !record.Background || record.ProtectedOutputSnapshot == nil || len(providerClient.Requests()) != 1 {
		t.Fatal("real background child did not retain one completion source", err)
	}
	expected, _, err := subagentapp.TurnCompletionFromThreadStoreV1(h.store.GetThread, record.ChildThreadID, record.ChildTurnID)
	if err != nil || !strings.Contains(expected, marker) {
		t.Fatal("exact child completion is not authoritative", err)
	}
	// Existing CAS, primary and Job owners are reopened before the first query.
	// Other scope/identity owners remain alive; this is not full-runtime restart.
	reopenProtectedJobSnapshotOwnersV1(t, h, root, durable)
	cold, err := h.jobs.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Wrong grant/tool and missing CAS checks call the resolver directly.
	// They are not HTTP or full-runtime startup evidence.
	assertProtectedJobSourceIdentityV1(t, h, parent, cold)
	assertProtectedJobOutputQueryBindingV1(t, h, parent, cold, expected, marker, "cold-background-child")
	if len(providerClient.Requests()) != 1 {
		t.Fatal("cold first query reexecuted the child to obtain historical body")
	}
}
