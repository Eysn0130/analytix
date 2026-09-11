//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativebuildhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	domainnativebuild "analytix.local/runtime-go/internal/domain/nativebuild"
	portnativebuild "analytix.local/runtime-go/internal/ports/nativebuild"
)

var (
	ErrConfigInvalid        = errors.New("native_build_host_config_invalid")
	ErrAuthorityUnavailable = errors.New("native_build_host_authority_unavailable")
)

// ConfigV1 is routing data only. Paths supplied by the caller never confer
// release authority; a production host must independently pin every source,
// toolchain, output, and signing authority before returning Ready=true.
type ConfigV1 struct {
	RepositoryRoot  string
	PublicationRoot string
	TargetKey       string
}

// Host is deliberately fail-closed until an externally controlled build and
// signing authority is provisioned. Keeping this as the production adapter is
// important: Node can invoke one Go coordinator today without regaining a
// direct Cargo or standalone-publisher path, while an ordinary local machine
// cannot manufacture release eligibility from paths or JSON.
type Host struct {
	repositoryRoot  string
	publicationRoot string
	targetKey       string
}

var _ portnativebuild.Host = (*Host)(nil)

func OpenProductionV1(config ConfigV1) (*Host, error) {
	repositoryRoot, err := canonicalExistingDirectoryV1(config.RepositoryRoot)
	if err != nil {
		return nil, ErrConfigInvalid
	}
	publicationRoot, err := canonicalProspectiveDirectoryV1(config.PublicationRoot)
	if err != nil || !pathWithinV1(repositoryRoot, publicationRoot) {
		return nil, ErrConfigInvalid
	}
	wantTarget := "darwin-" + runtime.GOARCH
	if runtime.GOARCH == "amd64" {
		wantTarget = "darwin-x64"
	}
	if config.TargetKey != wantTarget {
		return nil, ErrConfigInvalid
	}
	return &Host{
		repositoryRoot:  repositoryRoot,
		publicationRoot: publicationRoot,
		targetKey:       config.TargetKey,
	}, nil
}

func (host *Host) Preflight(ctx context.Context, requestNonce string) (portnativebuild.PreflightResultV1, error) {
	if host == nil || ctx == nil || ctx.Err() != nil || !validDigestV1(requestNonce) ||
		host.repositoryRoot == "" || host.publicationRoot == "" || host.targetKey == "" {
		return portnativebuild.PreflightResultV1{}, ErrConfigInvalid
	}
	return portnativebuild.PreflightResultV1{
		Disposition: portnativebuild.EffectNoneV1,
		Assessment: portnativebuild.EnvironmentAssessmentV1{
			Ready:   false,
			Blocker: "authority_identity_unbound",
		},
	}, nil
}

func (*Host) ReconcileSessionStart(context.Context, string) error { return ErrAuthorityUnavailable }
func (*Host) CloseSession(context.Context, portnativebuild.BuildSession) error {
	return ErrAuthorityUnavailable
}
func (*Host) ReconcileSessionClose(context.Context, portnativebuild.BuildSession, string) error {
	return ErrAuthorityUnavailable
}
func (*Host) BeginSnapshot(context.Context, portnativebuild.BuildSession) (portnativebuild.SnapshotStartV1, error) {
	return portnativebuild.SnapshotStartV1{}, ErrAuthorityUnavailable
}
func (*Host) ReconcileSnapshotStart(context.Context, portnativebuild.BuildSession, string) error {
	return ErrAuthorityUnavailable
}
func (*Host) SnapshotBinding(portnativebuild.SnapshotLease) (domainnativebuild.SourceSnapshotBindingV1, bool) {
	return domainnativebuild.SourceSnapshotBindingV1{}, false
}
func (*Host) ExecuteCargoPlan(context.Context, portnativebuild.BuildSession, portnativebuild.SnapshotLease) (portnativebuild.CargoPlanResultV1, error) {
	return portnativebuild.CargoPlanResultV1{}, ErrAuthorityUnavailable
}
func (*Host) ReconcileCargoExecution(context.Context, portnativebuild.BuildSession, portnativebuild.SnapshotLease, string) error {
	return ErrAuthorityUnavailable
}
func (*Host) VerifySnapshot(context.Context, portnativebuild.SnapshotLease) error {
	return ErrAuthorityUnavailable
}
func (*Host) DiscardSnapshot(context.Context, portnativebuild.SnapshotLease) error {
	return ErrAuthorityUnavailable
}
func (*Host) ReconcileDiscard(context.Context, portnativebuild.SnapshotLease) error {
	return ErrAuthorityUnavailable
}
func (*Host) AbortArtifacts(context.Context, portnativebuild.ArtifactLease) error {
	return ErrAuthorityUnavailable
}
func (*Host) ReconcileArtifactAbort(context.Context, portnativebuild.ArtifactLease, string) error {
	return ErrAuthorityUnavailable
}
func (*Host) Publish(context.Context, portnativebuild.ArtifactLease, domainnativebuild.PublicationPermitV1, domainnativebuild.PublicationBindingV1) (portnativebuild.PublicationOutcomeV1, error) {
	return portnativebuild.PublicationOutcomeV1{}, ErrAuthorityUnavailable
}
func (*Host) ObservePublication(context.Context, string, domainnativebuild.PublicationBindingV1) (portnativebuild.PublicationOutcomeV1, error) {
	return portnativebuild.PublicationOutcomeV1{}, ErrAuthorityUnavailable
}

func canonicalExistingDirectoryV1(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", ErrConfigInvalid
	}
	real, err := filepath.EvalSymlinks(value)
	if err != nil || real != value {
		return "", ErrConfigInvalid
	}
	info, err := os.Lstat(value)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrConfigInvalid
	}
	return real, nil
}

func canonicalProspectiveDirectoryV1(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", ErrConfigInvalid
	}
	if _, err := os.Lstat(value); err == nil {
		return canonicalExistingDirectoryV1(value)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", ErrConfigInvalid
	}
	parent, err := canonicalExistingDirectoryV1(filepath.Dir(value))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(value)), nil
}

func pathWithinV1(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func validDigestV1(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}
