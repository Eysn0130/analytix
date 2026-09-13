package fundscsvadmission

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type importCaseCreatorStubV1 struct {
	observer *importObserverStubV1
	identity string
	creates  int
}

func (stub *importCaseCreatorStubV1) ObserveMissingImportWorkspace(string) (domainsecurity.CaseBindingObservationV1, string, error) {
	return stub.observer.observation, stub.identity, nil
}

func (stub *importCaseCreatorStubV1) CreateForMainSelectedImport(ctx context.Context, workspace string, expected domainsecurity.CaseBindingObservationV1, physical, caseID string, _ time.Time) (domainsecurity.CaseBindingObservationV1, error) {
	if ctx.Err() != nil || expected != stub.observer.observation || physical != stub.identity {
		return domainsecurity.CaseBindingObservationV1{}, ErrUnavailable
	}
	stub.creates++
	created, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid, CaseID: caseID,
		BindingSHA256: strings.Repeat("b", 64), CaseBindingHash: strings.Repeat("c", 64),
	})
	stub.observer.observation = created
	return created, err
}

func TestImportCaseCreationRequiresCurrentExplicitHostIntentV1(t *testing.T) {
	for _, mode := range []string{"request", "confirm", "wrong-workspace", "existing-case", "identity-changed", "cancelled", "runtime-restart", "source-change", "expired", "principal-change"} {
		t.Run(mode, func(t *testing.T) {
			workspace := "/synthetic/first-import"
			missing, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateMissing})
			if err != nil {
				t.Fatal(err)
			}
			observer := &importObserverStubV1{observation: missing}
			identity := importIdentityForTestV1(t, "first-import")
			creator := &importCaseCreatorStubV1{observer: observer, identity: strings.Repeat("a", 64)}
			service := &ServiceV1{observer: observer, identity: identity, caseCreator: creator, now: time.Now, random: bytes.NewReader(bytes.Repeat([]byte{1}, 32))}
			input := StageInputV1{WorkspaceRoot: workspace, SourcePath: "/synthetic/source.csv"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, _, requestErr := service.resolveSelectedImportAuthorityV1(ctx, input)
			var required *CaseCreationRequiredV1
			if !errors.As(requestErr, &required) || !domainsecurity.IsSHA256Hex(required.Intent) || observer.observation != missing || creator.creates != 0 {
				t.Fatal("case request changed state or omitted its runtime-bound intent")
			}
			if mode == "request" {
				return
			}
			input.CreateCaseIntent = required.Intent
			switch mode {
			case "wrong-workspace":
				creator.identity = strings.Repeat("d", 64)
			case "existing-case":
				observer.observation = importObservationForTestV1(t, workspace, "existing-case")
			case "identity-changed":
				identity.valid = false
			case "cancelled":
				cancel()
			case "runtime-restart":
				service.pendingCaseCreation = nil
			case "source-change":
				input.SourcePath = "/synthetic/other.csv"
			case "expired":
				service.pendingCaseCreation.expiresAt = time.Now().Add(-time.Second)
			case "principal-change":
				identity.principal = importIdentityForTestV1(t, "second-principal").principal
			}
			_, got, err := service.resolveSelectedImportAuthorityV1(ctx, input)
			if mode == "confirm" {
				if err != nil || creator.creates != 1 || got.State != domainsecurity.CaseBindingStateValid {
					t.Fatal("confirmed case creation failed")
				}
				return
			}
			if err == nil || creator.creates != 0 {
				t.Fatal("unconfirmed or stale intent created authority")
			}
		})
	}
}
