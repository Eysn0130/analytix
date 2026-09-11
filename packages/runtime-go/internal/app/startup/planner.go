package startup

import (
	"context"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

var ErrStartupSnapshotDrift = errors.New("managed persistence changed during startup planning")

type ReadOnlyPlanningSessionV1 struct {
	reader   startupport.SnapshotReader
	baseline domainstartup.ManagedSnapshotV1
}

func BeginReadOnlyStartupPlanV1(ctx context.Context, reader startupport.SnapshotReader) (ReadOnlyPlanningSessionV1, error) {
	if reader == nil {
		return ReadOnlyPlanningSessionV1{}, errors.New("startup snapshot reader is required")
	}
	snapshot, err := reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		return ReadOnlyPlanningSessionV1{}, err
	}
	if err := domainstartup.ValidateManagedSnapshotV1(snapshot); err != nil {
		return ReadOnlyPlanningSessionV1{}, err
	}
	return ReadOnlyPlanningSessionV1{reader: reader, baseline: snapshot}, nil
}

func (session ReadOnlyPlanningSessionV1) BindConfiguration(ctx context.Context, configurationDigest string, now time.Time) (domainstartup.ReadOnlyStartupBaselineV1, error) {
	if session.reader == nil || domainstartup.ValidateManagedSnapshotV1(session.baseline) != nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(configurationDigest)) {
		return domainstartup.ReadOnlyStartupBaselineV1{}, errors.New("startup planning session is invalid")
	}
	verified, err := session.reader.CaptureManagedSnapshotV1(ctx)
	if err != nil {
		return domainstartup.ReadOnlyStartupBaselineV1{}, err
	}
	if domainstartup.ValidateManagedSnapshotV1(verified) != nil || verified.RootBindingDigest != session.baseline.RootBindingDigest ||
		verified.RawCaptureDigest != session.baseline.RawCaptureDigest || verified.SnapshotDigest != session.baseline.SnapshotDigest {
		return domainstartup.ReadOnlyStartupBaselineV1{}, ErrStartupSnapshotDrift
	}
	return domainstartup.NewReadOnlyStartupBaselineV1(verified, configurationDigest, now)
}

func (session ReadOnlyPlanningSessionV1) BaselineSnapshot() domainstartup.ManagedSnapshotV1 {
	return session.baseline
}

func BuildSemanticStartupPlanV1(
	ctx context.Context,
	builder startupport.SemanticPlanBuilderV1,
	baseline domainstartup.ReadOnlyStartupBaselineV1,
	configurationDigest string,
	simulate startupport.SemanticSimulationV1,
) (startupport.PreparedSemanticPlanV1, error) {
	if builder == nil || simulate == nil || domainstartup.ValidateReadOnlyStartupBaselineV1(baseline) != nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(configurationDigest)) || baseline.ConfigurationDigest != strings.TrimSpace(configurationDigest) {
		return nil, errors.New("semantic startup planning input is invalid")
	}
	prepared, err := builder.Prepare(ctx, baseline, configurationDigest, simulate)
	if err != nil {
		return nil, err
	}
	if prepared == nil || domainstartup.ValidateSemanticStartupPlanV1(prepared.Plan()) != nil ||
		prepared.Plan().BaselineDigest != baseline.ManagedSnapshotDigest || prepared.Plan().ConfigurationDigest != configurationDigest {
		if prepared != nil {
			_ = prepared.Close()
		}
		return nil, errors.New("semantic startup plan does not match the sealed baseline")
	}
	return prepared, nil
}
