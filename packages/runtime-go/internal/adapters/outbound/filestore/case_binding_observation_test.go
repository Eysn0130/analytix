package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCaseBindingObserverClassifiesCoreStates(t *testing.T) {
	reader := CaseBindingReader{}

	t.Run("missing metadata directory", func(t *testing.T) {
		workspace := t.TempDir()
		observation, err := reader.Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateMissing)
	})

	t.Run("missing binding file", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.Mkdir(filepath.Join(workspace, workspaceHostMetadataDir), 0o755); err != nil {
			t.Fatal(err)
		}
		observation, err := reader.Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateMissing)
	})

	t.Run("valid", func(t *testing.T) {
		workspace := t.TempDir()
		writeCaseBindingFixture(t, workspace, map[string]any{
			"version": 1, "workspaceRoot": workspace, "caseId": "case_valid", "source": "analytix-data-analysis",
		})
		observation, err := reader.Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateValid)
		if observation.CaseID != "case_valid" || !domainsecurity.IsSHA256Hex(observation.BindingSHA256) ||
			!domainsecurity.IsSHA256Hex(observation.CaseBindingHash) {
			t.Fatalf("valid observation omitted binding authority: %#v", observation)
		}
	})

	t.Run("workspace unavailable", func(t *testing.T) {
		workspace := filepath.Join(t.TempDir(), "removed")
		observation, err := reader.Observe(workspace)
		assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateWorkspaceMissing)
	})
}

func TestCaseBindingObserverRejectsAmbiguousOrMalformedJSON(t *testing.T) {
	tests := map[string]func(string) []byte{
		"duplicate key": func(workspace string) []byte {
			return []byte(fmt.Sprintf(`{"version":1,"version":1,"workspaceRoot":%q,"caseId":"case_duplicate","source":"analytix-data-analysis"}`, workspace))
		},
		"trailing JSON": func(workspace string) []byte {
			return []byte(fmt.Sprintf(`{"version":1,"workspaceRoot":%q,"caseId":"case_trailing","source":"analytix-data-analysis"} {}`, workspace))
		},
		"unknown field": func(workspace string) []byte {
			return []byte(fmt.Sprintf(`{"version":1,"workspaceRoot":%q,"caseId":"case_unknown","source":"analytix-data-analysis","safeToAnswer":true}`, workspace))
		},
		"wrong source": func(workspace string) []byte {
			return []byte(fmt.Sprintf(`{"version":1,"workspaceRoot":%q,"caseId":"case_source","source":"caller-reported"}`, workspace))
		},
	}
	for name, bodyFor := range tests {
		t.Run(name, func(t *testing.T) {
			workspace := t.TempDir()
			writeRawCaseBindingFixture(t, workspace, bodyFor(workspace), 0o644)
			observation, err := (CaseBindingReader{}).Observe(workspace)
			assertCaseBindingObservationState(t, observation, err, domainsecurity.CaseBindingStateInvalid)
			if !domainsecurity.IsSHA256Hex(observation.BindingSHA256) {
				t.Fatalf("securely read invalid content should retain only its raw digest: %#v", observation)
			}
			if _, readErr := ReadAnalytixCaseBinding(workspace); !errors.Is(readErr, ErrCaseBindingInvalid) {
				t.Fatalf("invalid binding did not preserve typed classification: %v", readErr)
			}
		})
	}
}

func TestCaseBindingErrorStateUsesOutermostObservationPhase(t *testing.T) {
	readFailure := newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, errors.New("read failed"))
	changedDuringRead := newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, readFailure)
	if state := caseBindingErrorState(changedDuringRead); state != domainsecurity.CaseBindingStateUnstable {
		t.Fatalf("nested readback failure was not classified by its outer observation phase: %s", state)
	}
	if !errors.Is(changedDuringRead, ErrCaseBindingUnstable) || errors.Is(changedDuringRead, ErrCaseBindingUnreadable) {
		t.Fatalf("typed observation classifications are not mutually exclusive: %v", changedDuringRead)
	}
}

func assertCaseBindingObservationState(t *testing.T, observation domainsecurity.CaseBindingObservationV1, err error, state string) {
	t.Helper()
	if err != nil || observation.State != state || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil {
		t.Fatalf("observation state mismatch: want=%s got=%#v err=%v", state, observation, err)
	}
}

func writeRawCaseBindingFixture(t *testing.T, workspace string, body []byte, mode os.FileMode) {
	t.Helper()
	metadata := filepath.Join(workspace, workspaceHostMetadataDir)
	if err := os.MkdirAll(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, caseBindingFileName), body, mode); err != nil {
		t.Fatal(err)
	}
}
