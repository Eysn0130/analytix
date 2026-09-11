package terminal

import (
	"context"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

type fakeJobRepository struct {
	record    domainjob.Record
	lastStart domainjob.StartRequest
	updates   []domainjob.UpdateRequest
}

func (f *fakeJobRepository) StartChildRun(request domainjob.StartRequest) (domainjob.Record, error) {
	f.lastStart = request
	f.record = domainjob.Record{
		ID:               "job-1",
		ParentThreadID:   request.ParentThreadID,
		ParentTurnID:     request.ParentTurnID,
		ParentToolItemID: request.ParentToolItemID,
		ParentToolCallID: request.ParentToolCallID,
		SecurityBinding:  domainjob.CloneSecurityBinding(request.SecurityBinding),
		Kind:             request.Kind,
		Name:             request.Name,
		Label:            request.Label,
		Status:           request.Status,
		Workspace:        request.Workspace,
		ToolPolicy:       request.ToolPolicy,
		Background:       request.Background,
	}
	return f.record, nil
}

func (f *fakeJobRepository) LoadChildRun(id string) (domainjob.Record, error) {
	return f.record, nil
}

func (f *fakeJobRepository) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	f.updates = append(f.updates, request)
	if request.Status != "" {
		f.record.Status = request.Status
	}
	if request.Output != "" {
		f.record.Output = request.Output
	}
	if request.Error != "" {
		f.record.Error = request.Error
	}
	if request.Usage != nil {
		f.record.Usage = request.Usage
	}
	return f.record, nil
}

func (f *fakeJobRepository) List(parentThreadID string) ([]domainjob.Record, error) {
	return []domainjob.Record{f.record}, nil
}

func TestStartBackgroundBashJobBuildsDurableRequest(t *testing.T) {
	repo := &fakeJobRepository{}
	service := NewService(Dependencies{Jobs: repo})
	binding := testBackgroundBashSecurityBinding(t, "thread-1", "turn-1", "/tmp/work")
	record, err := service.StartBackgroundBashJob(StartBackgroundBashJobRequest{
		ParentGoalID:        "goal-1",
		ParentGoalObjective: "finish work",
		ParentThreadID:      "thread-1",
		ParentTurnID:        "turn-1",
		ParentToolItemID:    "item-call-1",
		ParentToolCallID:    binding.ParentToolCallID,
		Command:             "curl -H 'Authorization: Bearer start-secret-123456' https://example.test",
		Workspace:           "/tmp/work",
		LabelPrefix:         "Restart: ",
		SecurityBinding:     binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != "background-shell" || record.Name != "bash" || record.Status != "running" || !record.Background {
		t.Fatalf("unexpected background bash record: %#v", record)
	}
	if record.Label != "background shell" || record.ToolPolicy != "workspace-write" || record.Workspace != "/tmp/work" {
		t.Fatalf("background bash metadata mismatch: record=%#v start=%#v", record, repo.lastStart)
	}
	if strings.Contains(record.Label, "start-secret-123456") || strings.Contains(repo.lastStart.Label, "curl") {
		t.Fatalf("background command escaped into durable label: record=%#v request=%#v", record, repo.lastStart)
	}
	if repo.lastStart.ParentGoalID != "goal-1" || repo.lastStart.ParentToolItemID != "item-call-1" || repo.lastStart.ParentToolCallID != binding.ParentToolCallID {
		t.Fatalf("lineage must be preserved in start request: %#v", repo.lastStart)
	}
}

func TestCompleteBackgroundBashProjectsCredentialOutputAndError(t *testing.T) {
	const secret = "literal-command-secret-123456"
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-1", Status: "running"}}
	updated, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{
		write:  "stdout=" + secret,
		result: ports.ShellResult{ExitCode: 1, Error: "stderr=" + secret},
	}, repo, BackgroundBashRequest{
		Record: repo.record, Command: "API_TOKEN=" + secret + " sh -c 'echo $API_TOKEN'", Workspace: "/tmp",
	}, BackgroundBashCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(updated.Output, secret) || strings.Contains(updated.Error, secret) ||
		!strings.Contains(updated.Output, "<redacted>") || !strings.Contains(updated.Error, "<redacted>") {
		t.Fatalf("background credentials escaped projection: %#v", updated)
	}
}

func TestBackgroundBashProjectsUnterminatedPrivateKeyBeforeEveryRepositoryWrite(t *testing.T) {
	const keyFragment = "-----BEGIN OPENSSH PRIVATE KEY-----\nQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo="
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-1", Status: "running"}}
	_, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{
		write: keyFragment,
	}, repo, BackgroundBashRequest{Record: repo.record, Command: "print-key", Workspace: "/tmp"}, BackgroundBashCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.updates) == 0 {
		t.Fatal("expected at least one projected repository update")
	}
	for _, update := range repo.updates {
		if strings.Contains(update.Output, "BEGIN OPENSSH PRIVATE KEY") || strings.Contains(update.Output, "QUJDREVGR0hJ") {
			t.Fatalf("private-key chunk reached repository update: %#v", update)
		}
	}
}

func TestSecurityBoundBackgroundBashPersistsNoOutputOrError(t *testing.T) {
	const secret = "security-bound-secret-123456"
	record := domainjob.Record{ID: "job-1", Status: "running", SecurityBinding: &domainjob.SecurityBinding{}}
	repo := &fakeJobRepository{record: record}
	updated, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{
		write:  "stdout=" + secret,
		result: ports.ShellResult{ExitCode: 1, Error: "stderr=" + secret},
	}, repo, BackgroundBashRequest{Record: record, Command: "TOKEN=" + secret, Workspace: "/tmp"}, BackgroundBashCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Output != "" || updated.Error != "" {
		t.Fatalf("security-bound shell persisted content: %#v", updated)
	}
}

func TestCompleteBackgroundBashPropagatesProtectedReadDirs(t *testing.T) {
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-protected"}}
	protected := []string{"/private/one", "/private/two"}
	var observed []string
	_, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{
		onRequest: func(request ports.ShellRequest) {
			observed = append([]string(nil), request.ProtectedReadDirs...)
		},
	}, repo, BackgroundBashRequest{
		Record: repo.record, Command: "true", Workspace: "/tmp",
		ProtectedReadDirs: protected,
	}, BackgroundBashCallbacks{})
	if err != nil || len(observed) != len(protected) ||
		observed[0] != protected[0] || observed[1] != protected[1] {
		t.Fatalf("protected process roots were not propagated: roots=%#v err=%v", observed, err)
	}
}

func TestCompleteBackgroundBashUpdatesOutputAndFinalStatus(t *testing.T) {
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-1", Status: "running"}}
	progress := []string{}
	updated, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{write: "hello"}, repo, BackgroundBashRequest{
		Record:          repo.record,
		Command:         "printf hello",
		Workspace:       "/tmp",
		OutputLimit:     10,
		IncludeTimedOut: true,
	}, BackgroundBashCallbacks{
		OnProgress: func(record domainjob.Record, status string, diagnostic string) {
			progress = append(progress, status+":"+diagnostic)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "completed" || updated.Output != "hello" || updated.Usage["timedOut"] != false {
		t.Fatalf("unexpected updated record: %#v", updated)
	}
	if len(progress) != 1 || progress[0] == "" {
		t.Fatalf("expected final progress, got %#v", progress)
	}
}

func TestServiceCompleteBackgroundBashUsesConfiguredRunnerAndJobs(t *testing.T) {
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-1", Status: "running"}}
	service := NewService(Dependencies{Jobs: repo, ShellRunner: fakeShellRunner{write: "hello service"}})
	updated, err := service.CompleteBackgroundBash(context.Background(), BackgroundBashRequest{
		Record:    repo.record,
		Command:   "printf hello",
		Workspace: "/tmp",
	}, BackgroundBashCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "completed" || updated.Output != "hello service" || len(repo.updates) == 0 {
		t.Fatalf("unexpected service background update: updated=%#v repo=%#v", updated, repo)
	}
}

func TestCompleteBackgroundBashRespectsKilledStatus(t *testing.T) {
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-1", Status: "killed", Error: "killed by user"}}
	progress := []string{}
	updated, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{write: "late output"}, repo, BackgroundBashRequest{
		Record:              domainjob.Record{ID: "job-1", Status: "running"},
		Command:             "sleep",
		Workspace:           "/tmp",
		RespectKilledStatus: true,
	}, BackgroundBashCallbacks{
		OnProgress: func(record domainjob.Record, status string, diagnostic string) {
			progress = append(progress, status+":"+diagnostic)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "killed" || len(progress) != 1 || progress[0] != "killed:killed by user" {
		t.Fatalf("expected killed status to win, updated=%#v progress=%#v", updated, progress)
	}
}

func TestCompleteBackgroundBashStartFailure(t *testing.T) {
	repo := &fakeJobRepository{record: domainjob.Record{ID: "job-1", Status: "running"}}
	updated, err := CompleteBackgroundBash(context.Background(), fakeShellRunner{result: ports.ShellResult{
		ExitCode:    -1,
		Error:       "fork failed",
		StartFailed: true,
	}}, repo, BackgroundBashRequest{Record: repo.record, Command: "bad", Workspace: "/tmp"}, BackgroundBashCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "failed" || updated.Error != "fork failed" {
		t.Fatalf("unexpected failed record: %#v", updated)
	}
}
