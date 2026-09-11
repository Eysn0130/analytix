package server

import (
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestTaskJobSteerEffectBindingPreservesBoundCaseFollowups(t *testing.T) {
	caseContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-case", TurnID: "turn-child-case", WorkspaceRealPath: t.TempDir(),
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	boundaryContext, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child-boundary", TurnID: "turn-child-boundary", WorkspaceRealPath: t.TempDir(),
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name             string
		prompt           string
		frozen           domainsecurity.TurnSecurityContext
		childTurnStarted bool
		parentCaseBound  bool
		want             domainsecurity.LogicalEffect
		ordinary         bool
	}{
		{
			name: "queued before child turn", prompt: "那净额呢",
			parentCaseBound: true, want: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "active child turn", prompt: "分析该账户上月有什么异常",
			frozen: caseContext, childTurnStarted: true, parentCaseBound: true,
			want: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "boundary-only child turn", prompt: "再看流出",
			frozen: boundaryContext, childTurnStarted: true, parentCaseBound: true,
			want: domainsecurity.LogicalEffectFundsData,
		},
		{
			name: "ordinary software account work", prompt: "Compare this account parser in the test module.",
			frozen: caseContext, childTurnStarted: true, parentCaseBound: true,
			want: domainsecurity.LogicalEffectOrdinary, ordinary: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := controlapp.TaskJobSteerLogicalEffectBindingV1(
				test.prompt, test.frozen, test.childTurnStarted, test.parentCaseBound, false, false,
			)
			if got.LogicalEffect != test.want || got.OrdinaryWork != test.ordinary {
				t.Fatalf("task-job steer effect mismatch: got=%#v want=%s ordinary=%t", got, test.want, test.ordinary)
			}
		})
	}
}

func TestTaskJobQueuedSteerBindingCannotLowerFrozenContextRisk(t *testing.T) {
	for _, test := range []struct {
		name    string
		current domainsteering.EntryLogicalEffectBinding
		queued  domainsteering.EntryLogicalEffectBinding
		want    domainsteering.EntryLogicalEffectBinding
	}{
		{
			name: "queued ordinary cannot lower bound funds followup",
			current: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectFundsData,
			},
			queued: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
			},
			want: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
		},
		{
			name: "queued case cannot lower funds",
			current: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true,
			},
			queued: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectCaseData, OrdinaryWork: true,
			},
			want: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true,
			},
		},
		{
			name: "queued funds raises generic case",
			current: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectCaseData, OrdinaryWork: true,
			},
			queued: domainsteering.EntryLogicalEffectBinding{
				LogicalEffect: domainsecurity.LogicalEffectFundsData,
			},
			want: domainsteering.EntryLogicalEffectBinding{LogicalEffect: domainsecurity.LogicalEffectFundsData},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := controlapp.StricterTaskJobSteerLogicalEffectBindingV1(test.current, test.queued); got != test.want {
				t.Fatalf("merged task-job steer binding = %#v, want %#v", got, test.want)
			}
		})
	}
}
