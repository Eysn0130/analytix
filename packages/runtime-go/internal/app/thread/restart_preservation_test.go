package thread

import (
	"context"
	"errors"
	"testing"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type restartHeldCaseAuthorityV1 struct {
	held      string
	knownCase bool
}

func (authority restartHeldCaseAuthorityV1) IsCaseThread(id string) bool {
	return authority.knownCase && id == authority.held
}
func (restartHeldCaseAuthorityV1) ContainsContext(domainsecurity.TurnSecurityContext) bool {
	return false
}
func (authority restartHeldCaseAuthorityV1) RestartPreservesThreadV1(id string) bool {
	return id == authority.held
}

type restartReadCounterV1 struct {
	*repositoryStub
	reads int
}

func (repo *restartReadCounterV1) GetThread(id string) (map[string]any, error) {
	repo.reads++
	return repo.repositoryStub.GetThread(id)
}
func (repo *restartReadCounterV1) ReadThreadMutationBaseline(id string) (map[string]any, string, error) {
	repo.reads++
	return repo.repositoryStub.ReadThreadMutationBaseline(id)
}

func TestRestartPreservedThreadServiceRejectsBeforeReadsAndEffects(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, operation := range []string{"patch", "delete", "rewind", "fork", "compact", "auto_compact", "set_goal", "clear_goal", "set_todos", "clear_todos"} {
			t.Run(operation+map[bool]string{false: "_unindexed", true: "_case"}[known], func(t *testing.T) {
				repo := &restartReadCounterV1{repositoryStub: &repositoryStub{}}
				service := newMutationService(repo)
				service.caseThreads = restartHeldCaseAuthorityV1{held: "held", knownCase: known}
				quiesces := 0
				service.quiesceThreadTurns = func(context.Context, string) error { quiesces++; return nil }
				var err error
				switch operation {
				case "patch":
					_, err = service.Patch(context.Background(), "held", map[string]any{"title": "changed"})
				case "delete":
					_, err = service.Delete(context.Background(), "held")
				case "rewind":
					_, err = service.Rewind(context.Background(), "held", "turn")
				case "fork":
					_, err = service.Fork("held", nil)
				case "compact":
					_, err = service.Compact(context.Background(), "held", "manual")
				case "auto_compact":
					_, err = service.AutoCompactBeforeTurnV1(context.Background(), AutoCompactionInputV1{ThreadID: "held", MainThread: true})
				case "set_goal":
					_, err = service.SetGoal("held", map[string]any{"objective": "held"})
				case "clear_goal":
					_, err = service.ClearGoal("held")
				case "set_todos":
					_, err = service.SetTodos("held", nil)
				case "clear_todos":
					_, err = service.ClearTodos("held")
				}
				if !errors.Is(err, casethreadapp.ErrRestartPreserved) || repo.reads != 0 || repo.patched || len(repo.events) != 0 || quiesces != 0 || repo.compactionCommits != 0 {
					t.Fatalf("held operation crossed admission: err=%v reads=%d patched=%v events=%d quiesces=%d", err, repo.reads, repo.patched, len(repo.events), quiesces)
				}
			})
		}
	}
	base := &repositoryStub{}
	service := newMutationService(base)
	service.caseThreads = restartHeldCaseAuthorityV1{held: "held"}
	if _, err := service.Patch(context.Background(), "active", map[string]any{"title": "independent"}); err != nil || !base.patched || len(base.events) != 1 {
		t.Fatalf("independent metadata patch failed: %v", err)
	}
}

func TestRestartHeldThreadDoesNotBlockIndependentList(t *testing.T) {
	repo := &restartReadCounterV1{repositoryStub: &repositoryStub{}}
	service := NewService(Dependencies{Repository: repo})
	service.caseThreads = restartHeldCaseAuthorityV1{held: "thr_1", knownCase: true}
	listed, err := service.List(ListInput{})
	if err != nil || len(listed) != 1 || listed[0]["id"] != "thr_2" || repo.reads != 1 {
		t.Fatalf("held primary blocked independent list or was read: listed=%#v reads=%d err=%v", listed, repo.reads, err)
	}
	repo.highestSeqErr = errors.New("independent cursor unavailable")
	if listed, err := service.List(ListInput{}); err == nil || listed != nil {
		t.Fatal("independent observation failure was hidden")
	}
}
