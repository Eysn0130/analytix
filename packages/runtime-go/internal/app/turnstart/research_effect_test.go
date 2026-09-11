package turnstart

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type researchEffectContextKey struct{}

func TestRunResearchStateEffectValidatesAroundLeaseBoundWrite(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-research-effect", TurnID: "turn-research-effect", WorkspaceRealPath: "/workspace/research-effect",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	released := false
	err = RunResearchStateEffect(
		context.Background(), securityContext,
		func(ctx context.Context, current domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			if current != securityContext {
				return ctx, nil, errors.New("wrong context")
			}
			order = append(order, "acquire")
			return context.WithValue(ctx, researchEffectContextKey{}, true), func() {
				released = true
				order = append(order, "release")
			}, nil
		},
		func(ctx context.Context) error {
			if ctx.Value(researchEffectContextKey{}) != true {
				return errors.New("validation lacks effect lease")
			}
			order = append(order, "validate")
			return nil
		},
		func(ctx context.Context) error {
			if ctx.Value(researchEffectContextKey{}) != true {
				return errors.New("write lacks effect lease")
			}
			order = append(order, "write")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !released || !reflect.DeepEqual(order, []string{"acquire", "validate", "write", "validate", "release"}) {
		t.Fatalf("research effect ordering mismatch: order=%v released=%t", order, released)
	}
}

func TestRunResearchStateEffectRejectsBeforeWriteAndReleases(t *testing.T) {
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-research-reject", TurnID: "turn-research-reject", WorkspaceRealPath: "/workspace/research-reject",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	released, wrote := false, false
	err = RunResearchStateEffect(
		context.Background(), securityContext,
		func(ctx context.Context, _ domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
			return ctx, func() { released = true }, nil
		},
		func(context.Context) error { return errors.New("stale authority") },
		func(context.Context) error { wrote = true; return nil },
	)
	if err == nil || err.Error() != "stale authority" || wrote || !released {
		t.Fatalf("stale research effect did not fail closed: err=%v wrote=%t released=%t", err, wrote, released)
	}
}
