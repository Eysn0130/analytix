package thread

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type finiteReplayReader func(string) (map[string]any, error)

func (read finiteReplayReader) GetThread(id string) (map[string]any, error) { return read(id) }

type finiteReplayCaseAuthority struct {
	known bool
	calls int
}

func (authority *finiteReplayCaseAuthority) IsCaseThread(string) bool {
	authority.calls++
	return authority.known
}
func (*finiteReplayCaseAuthority) ContainsContext(domainsecurity.TurnSecurityContext) bool {
	return false
}
func (*finiteReplayCaseAuthority) RestartPreservesThreadV1(string) bool { return false }

func TestFinitePublicReplayPreservesProjectionAndFreshReads(t *testing.T) {
	thread, events, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	id := contracts.StringField(thread, "id")
	reads := 0
	authority := &finiteReplayCaseAuthority{}
	projector := NewTrustedPublicProjectorWithCaseThreads(nil, authority)
	reader := finiteReplayReader(func(string) (map[string]any, error) {
		reads++
		return contracts.CloneMap(thread), nil
	})
	project, finish := NewFinitePublicEventProjector(context.Background(), reader, projector)
	for _, event := range events {
		expected, expectedVisible, err := NewTrustedPublicProjector(nil).ProjectEvent(id, thread, event)
		beforeCalls := authority.calls
		actual, visible, code := project(id, event)
		if err != nil || code != "" || visible != expectedVisible || !reflect.DeepEqual(actual, expected) {
			t.Fatalf("projection diverged: actual=%#v expected=%#v visible=%v/%v code=%q err=%v", actual, expected, visible, expectedVisible, code, err)
		}
		if authority.calls <= beforeCalls {
			t.Fatal("event admission reused Case inventory membership")
		}
	}
	if reads != len(events) || authority.calls < len(events) {
		t.Fatalf("event admission reused fresh state: reads=%d classification calls=%d", reads, authority.calls)
	}
	if code := finish(id); code != "" || reads != len(events)+2 {
		t.Fatalf("final fresh gate failed: code=%q reads=%d", code, reads)
	}
	if projector.ordinaryReplayValidator != nil {
		t.Fatal("request-private preparation mutated the shared projector")
	}
}

func TestFinitePublicReplayRejectsChangesPermanently(t *testing.T) {
	for _, change := range []string{"primary", "case_inventory", "final_read"} {
		t.Run(change, func(t *testing.T) {
			thread, events, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
			id := contracts.StringField(thread, "id")
			authority := &finiteReplayCaseAuthority{}
			reads := 0
			reader := finiteReplayReader(func(string) (map[string]any, error) {
				reads++
				current := contracts.CloneMap(thread)
				if change == "final_read" && reads == 3 {
					current["updatedAt"] = "changed during final projection"
				}
				return current, nil
			})
			project, finish := NewFinitePublicEventProjector(context.Background(), reader, NewTrustedPublicProjectorWithCaseThreads(nil, authority))
			if _, visible, code := project(id, events[0]); !visible || code != "" {
				t.Fatalf("initial event rejected: visible=%v code=%q", visible, code)
			}
			switch change {
			case "primary":
				thread["updatedAt"] = "changed"
			case "case_inventory":
				authority.known = true
			}
			if code := finish(id); code != CasePublicAuthorityRejectedCode {
				t.Fatalf("changed authority/input was accepted: %q", code)
			}
			delete(thread, "updatedAt")
			authority.known = false
			if _, visible, code := project(id, events[1]); visible || code != CasePublicAuthorityRejectedCode {
				t.Fatalf("A-to-B-to-A revived the request: visible=%v code=%q", visible, code)
			}
		})
	}
}

type finiteReplayProjector struct {
	finalCalls int
	final      func(map[string]any) (map[string]any, error)
}

func (projector *finiteReplayProjector) ProjectEvent(_ string, _ map[string]any, event map[string]any) (map[string]any, bool, error) {
	return event, true, nil
}
func (projector *finiteReplayProjector) ProjectThread(thread map[string]any) (map[string]any, error) {
	projector.finalCalls++
	if projector.final != nil {
		return projector.final(thread)
	}
	return nil, errors.New("fresh authority rejected")
}

func TestFinitePublicReplayFinalGateRejectsMutationWithoutArchiveReuse(t *testing.T) {
	for _, caseThread := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_archive", true: "case"}[caseThread], func(t *testing.T) {
			thread := map[string]any{"id": "thread-final-change", "title": "before"}
			if caseThread {
				thread["caseId"] = "case-final-change"
			}
			projector := &finiteReplayProjector{final: func(input map[string]any) (map[string]any, error) {
				// Mutating both the witness input and later read catches an
				// implementation that captures its comparison only after projection.
				input["title"], thread["title"] = "after", "after"
				return contracts.CloneMap(input), nil
			}}
			_, finish := NewFinitePublicEventProjector(context.Background(), finiteReplayReader(func(string) (map[string]any, error) {
				return contracts.CloneMap(thread), nil
			}), projector)
			if code := finish("thread-final-change"); code != CasePublicAuthorityRejectedCode || projector.finalCalls != 1 {
				t.Fatalf("final witness mutation was accepted: code=%q calls=%d", code, projector.finalCalls)
			}
			thread["title"] = "before"
			if code := finish("thread-final-change"); code != CasePublicAuthorityRejectedCode || projector.finalCalls != 1 {
				t.Fatalf("final A-to-B-to-A cleared invalidation: code=%q calls=%d", code, projector.finalCalls)
			}
		})
	}
}

func TestFinitePublicReplayRunsOriginalFinalAuthority(t *testing.T) {
	projector := &finiteReplayProjector{}
	project, finish := NewFinitePublicEventProjector(context.Background(), finiteReplayReader(func(string) (map[string]any, error) {
		return map[string]any{"id": "thread-final"}, nil
	}), projector)
	if _, visible, code := project("thread-final", map[string]any{"kind": "turn_started"}); !visible || code != "" {
		t.Fatal("initial projection failed")
	}
	if code := finish("thread-final"); code != CasePublicAuthorityRejectedCode || projector.finalCalls != 1 {
		t.Fatalf("final authority was bypassed: code=%q calls=%d", code, projector.finalCalls)
	}
}

func TestFinitePublicReplayStopsAfterReadCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	projector := &finiteReplayProjector{}
	reads := 0
	project, finish := NewFinitePublicEventProjector(ctx, finiteReplayReader(func(string) (map[string]any, error) {
		reads++
		cancel()
		return map[string]any{"id": "thread-cancel"}, nil
	}), projector)
	if event, visible, code := project("thread-cancel", map[string]any{}); event != nil || visible || code != CasePublicAuthorityUnavailableCode {
		t.Fatalf("cancelled read reached projection: event=%#v visible=%v code=%q", event, visible, code)
	}
	if code := finish("thread-cancel"); code != CasePublicAuthorityUnavailableCode || reads != 1 || projector.finalCalls != 0 {
		t.Fatalf("cancelled request continued: code=%q reads=%d final=%d", code, reads, projector.finalCalls)
	}
}
