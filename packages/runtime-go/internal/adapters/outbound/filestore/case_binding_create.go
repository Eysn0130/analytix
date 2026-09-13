package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ObserveMissingImportWorkspace binds the private confirmation flow to the
// existing workspace's physical identity, without creating files or authority.
func (CaseBindingReader) ObserveMissingImportWorkspace(workspace string) (domainsecurity.CaseBindingObservationV1, string, error) {
	realPath, err := resolveExistingCaseWorkspace(workspace)
	if err != nil || realPath != workspace {
		return domainsecurity.CaseBindingObservationV1{}, "", ErrCaseBindingCreateUnavailable
	}
	return observeMissingImportWorkspacePlatform(workspace)
}

var ErrCaseBindingCreateUnavailable = errors.New("case binding creation is unavailable")

// CreateForMainSelectedImport creates only an empty case identity, after the
// Main-owned import flow has obtained explicit user confirmation. It never
// repairs an existing marker or initializes evidence/snapshot authority.
func (CaseBindingReader) CreateForMainSelectedImport(
	ctx context.Context,
	workspace string,
	expected domainsecurity.CaseBindingObservationV1,
	expectedWorkspaceIdentity string,
	caseID string,
	updatedAt time.Time,
) (domainsecurity.CaseBindingObservationV1, error) {
	return createCaseBindingForMainSelectedImport(ctx, workspace, expected, expectedWorkspaceIdentity, caseID, updatedAt, nil)
}

func createCaseBindingForMainSelectedImport(
	ctx context.Context,
	workspace string,
	expected domainsecurity.CaseBindingObservationV1,
	expectedWorkspaceIdentity string,
	caseID string,
	updatedAt time.Time,
	hook caseBindingReadHook,
) (domainsecurity.CaseBindingObservationV1, error) {
	if ctx == nil {
		return domainsecurity.CaseBindingObservationV1{}, ErrCaseBindingCreateUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainsecurity.CaseBindingObservationV1{}, err
	}
	if domainsecurity.ValidateCaseBindingObservationV1(expected) != nil ||
		expected.State != domainsecurity.CaseBindingStateMissing || workspace != expected.WorkspaceRealPath ||
		!domainsecurity.IsSHA256Hex(expectedWorkspaceIdentity) ||
		!caseIDPattern.MatchString(caseID) || caseID == domainsecurity.UnboundCaseID ||
		updatedAt.IsZero() || updatedAt.Year() < 1 || updatedAt.Year() > 9999 {
		return domainsecurity.CaseBindingObservationV1{}, ErrCaseBindingCreateUnavailable
	}
	realPath, err := resolveExistingCaseWorkspace(workspace)
	if err != nil || realPath != workspace {
		return domainsecurity.CaseBindingObservationV1{}, ErrCaseBindingCreateUnavailable
	}
	current, err := (CaseBindingReader{}).Observe(workspace)
	if err != nil || current != expected {
		return domainsecurity.CaseBindingObservationV1{}, ErrCaseBindingCreateUnavailable
	}
	body, err := json.Marshal(caseBindingDocument{
		Version: 1, WorkspaceRoot: workspace, CaseID: caseID,
		Source: "analytix-data-analysis", UpdatedAt: updatedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil || len(body) > maxCaseBindingBytes {
		return domainsecurity.CaseBindingObservationV1{}, ErrCaseBindingCreateUnavailable
	}
	return createCaseBindingPlatform(ctx, workspace, expected, expectedWorkspaceIdentity, caseID, body, hook)
}
