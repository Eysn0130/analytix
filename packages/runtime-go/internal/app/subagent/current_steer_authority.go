package subagent

import (
	"context"
	"errors"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	apploop "analytix.local/runtime-go/internal/app/loop"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

type CurrentTaskJobSteerAuthorityDepsV1 struct {
	Jobs             TaskJobSteerStore
	ObserveFirstTurn func(context.Context, domainjob.Record) (domainsecurity.TurnSecurityContext, bool, error)
	State            *RuntimeState
	Security         turnsecurityapp.WorkspaceSecurityAuthority
	Control          *controlapp.Controller
	Blocker          func(domainjob.Record) string
}

// BeginCurrentTaskJobSteerAuthorityV1 owns current child-turn admission and
// keeps the effect lease held until the caller settles the exact durable steer.
func BeginCurrentTaskJobSteerAuthorityV1(
	ctx context.Context,
	deps CurrentTaskJobSteerAuthorityDepsV1,
	record domainjob.Record,
	message domainjob.SteerMessage,
) (TaskJobSteerAuthorization, string, error) {
	if deps.Jobs == nil || deps.ObserveFirstTurn == nil || deps.State == nil || deps.Control == nil || deps.Blocker == nil ||
		ctx == nil || ctx.Err() != nil || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil || deps.Blocker(record) != "" {
		return TaskJobSteerAuthorization{}, "", errors.New("task job steer authority is unavailable")
	}
	current, err := deps.Jobs.LoadChildRun(record.ID)
	if err != nil || !sameTaskJobSteerAuthorityRecordV1(record, current) || deps.Blocker(current) != "" {
		return TaskJobSteerAuthorization{}, "", errors.New("task job steer authority changed")
	}
	frozen, childTurnStarted, err := deps.ObserveFirstTurn(ctx, current)
	if err != nil {
		return TaskJobSteerAuthorization{}, "", errors.Join(errors.New("child first-turn observation is unavailable"), err)
	}
	var releasePending func()
	if !childTurnStarted {
		releasePending, err = deps.State.reservePendingChildSteer(ctx, current)
		if err != nil {
			return TaskJobSteerAuthorization{}, "", err
		}
		// Re-observe after winning the start boundary. Receipt association alone
		// cannot authorize queueing against a concurrently committed first turn.
		current, err = deps.Jobs.LoadChildRun(record.ID)
		if err != nil || !sameTaskJobSteerAuthorityRecordV1(record, current) || deps.Blocker(current) != "" {
			releasePending()
			return TaskJobSteerAuthorization{}, "", errors.New("pending child steer authority changed")
		}
		frozen, childTurnStarted, err = deps.ObserveFirstTurn(ctx, current)
		if err != nil || childTurnStarted {
			releasePending()
			return TaskJobSteerAuthorization{}, "", errors.New("pending child first-turn observation changed")
		}
		defer func() {
			if releasePending != nil {
				releasePending()
			}
		}()
	} else if frozen.ThreadID != record.ChildThreadID || frozen.TurnID != record.ChildTurnID ||
		!domainjob.SecurityBindingMatchesWorkspaceScope(record.SecurityBinding, frozen) {
		return TaskJobSteerAuthorization{}, "", errors.New("child frozen security authority is invalid")
	}
	lexicalCaseRisk := apploop.PromptRequiresCaseRiskAdmission(message.Text)
	protectedCaseData := domainsecurity.ContainsProtectedCaseFactCandidate(message.Text)
	effectBinding := controlapp.TaskJobSteerLogicalEffectBindingV1(
		message.Text, frozen, childTurnStarted,
		record.SecurityBinding.ParentCaseID != domainsecurity.UnboundCaseID,
		lexicalCaseRisk, protectedCaseData,
	)
	if message.LogicalEffect != "" {
		if domainjob.ValidateSteerMessageLogicalEffectBindingV1(message) != nil {
			return TaskJobSteerAuthorization{}, "", errors.New("task job steer logical effect binding is invalid")
		}
		effectBinding = controlapp.StricterTaskJobSteerLogicalEffectBindingV1(effectBinding, domainsteering.EntryLogicalEffectBinding{
			LogicalEffect: message.LogicalEffect, OrdinaryWork: message.OrdinaryWork,
		})
	}
	caseDataIntent := effectBinding.LogicalEffect != domainsecurity.LogicalEffectOrdinary
	projectedText, _, _ := privacyprojectionapp.ProjectSteeringContent(message.Text, "", nil)
	if strings.TrimSpace(projectedText) == "" {
		return TaskJobSteerAuthorization{}, "", errors.New("task job steer projection is empty")
	}
	if !childTurnStarted {
		if caseDataIntent && record.SecurityBinding.ParentCaseID == domainsecurity.UnboundCaseID {
			return TaskJobSteerAuthorization{}, controlapp.SteerBlockerCaseRiskRaise, nil
		}
		queueAuthority, err := domainjob.NewSteerQueueAuthorityV1(current, "", true)
		if err != nil {
			return TaskJobSteerAuthorization{}, "", err
		}
		release := releasePending
		releasePending = nil
		return TaskJobSteerAuthorization{
			PendingUntilChildTurn: true, ProjectedText: projectedText,
			EffectBinding: effectBinding, QueueAuthority: queueAuthority, Release: release,
		}, "", nil
	}
	admissionInput := controlapp.SteerSecurityAdmissionInput{
		Context: frozen, LexicalCaseRisk: caseDataIntent || lexicalCaseRisk, ProtectedCaseData: protectedCaseData,
	}
	preAdmission, err := controlapp.EvaluateSteerSecurityAdmission(admissionInput)
	if err != nil {
		return TaskJobSteerAuthorization{}, "", err
	}
	if preAdmission.RequiresNewTurn {
		return TaskJobSteerAuthorization{}, preAdmission.BlockerCode, nil
	}
	effectCtx, release, err := deps.State.AcquireContextEffectForAuthority(ctx, frozen, false)
	if err != nil {
		return TaskJobSteerAuthorization{}, "", err
	}
	currentInput := turnsecurityapp.CurrentValidationInput{
		OperationContext: effectCtx, Identity: deps.Security.Identity, Observer: deps.Security.Observer,
		RiskAuthority: deps.Security.RiskAuthority, SnapshotAuthority: deps.Security.SnapshotAuthority,
		SnapshotAuthorityV2: deps.Security.SnapshotAuthorityV2, Context: frozen, Workspace: frozen.WorkspaceRealPath,
	}
	if currentErr := turnsecurityapp.ValidateCurrentForEffect(currentInput, false); currentErr != nil {
		release()
		return TaskJobSteerAuthorization{}, "", errors.New("task job steer current security authority is unavailable")
	}
	current, err = deps.Jobs.LoadChildRun(record.ID)
	if err != nil || !sameTaskJobSteerAuthorityRecordV1(record, current) || deps.Blocker(current) != "" {
		release()
		return TaskJobSteerAuthorization{}, "", errors.New("task job steer authority changed")
	}
	admission, err := controlapp.EvaluateSteerSecurityAdmission(admissionInput)
	if err != nil {
		release()
		return TaskJobSteerAuthorization{}, "", err
	}
	if admission.RequiresNewTurn {
		release()
		return TaskJobSteerAuthorization{}, admission.BlockerCode, nil
	}
	releaseSteerAdmission, reservationErr := deps.Control.ReserveSteerAdmission(record.ChildThreadID, record.ChildTurnID)
	if reservationErr != nil || releaseSteerAdmission == nil {
		release()
		return TaskJobSteerAuthorization{}, "", errors.Join(errors.New("task job steer terminal arbitration failed"), reservationErr)
	}
	releaseAuthority := func() {
		releaseSteerAdmission()
		release()
	}
	queueAuthority, err := domainjob.NewSteerQueueAuthorityV1(current, frozen.ContextDigest, false)
	if err != nil {
		releaseAuthority()
		return TaskJobSteerAuthorization{}, "", err
	}
	return TaskJobSteerAuthorization{
		ExpectedContextDigest: frozen.ContextDigest, ProjectedText: projectedText,
		EffectBinding: effectBinding, QueueAuthority: queueAuthority, Release: releaseAuthority,
	}, "", nil
}

func sameTaskJobSteerAuthorityRecordV1(expected, current domainjob.Record) bool {
	return expected.ID != "" && current.ID == expected.ID && current.ParentThreadID == expected.ParentThreadID &&
		current.ParentTurnID == expected.ParentTurnID && current.ChildThreadID == expected.ChildThreadID &&
		current.ChildTurnID == expected.ChildTurnID && current.Background == expected.Background &&
		current.Status == expected.Status && current.UpdatedAt == expected.UpdatedAt &&
		domainjob.SteerSetDigestV1(current.Steers) == domainjob.SteerSetDigestV1(expected.Steers) &&
		current.SecurityBinding != nil && expected.SecurityBinding != nil &&
		current.SecurityBinding.BindingDigest == expected.SecurityBinding.BindingDigest
}
