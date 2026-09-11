package startup

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

type fakeFixedPointOwnerV1 struct {
	digest       string
	failValidate bool
}

func (owner *fakeFixedPointOwnerV1) Observe(context.Context) (OwnerObservationV1, error) {
	return OwnerObservationV1{Digest: owner.digest, Retired: true}, nil
}

func (owner *fakeFixedPointOwnerV1) ValidateObservation(_ context.Context, expected OwnerObservationV1) error {
	if owner.failValidate || expected.Digest != owner.digest {
		return errors.New("changed")
	}
	return nil
}

type fakeRetirementOwnerV1 struct {
	fakeFixedPointOwnerV1
	recoveryRequired bool
	retired          bool
	recoveries       int
	prepares         int
	applies          int
	mutateReadOnly   *fakeFixedPointOwnerV1
}

func (owner *fakeRetirementOwnerV1) Observe(context.Context) (OwnerObservationV1, error) {
	return OwnerObservationV1{
		Digest:           owner.digest,
		RecoveryRequired: owner.recoveryRequired,
		Retired:          owner.retired,
	}, nil
}

func (owner *fakeRetirementOwnerV1) Recover(context.Context) error {
	owner.recoveries++
	owner.recoveryRequired = false
	owner.digest = "retirement-after-recovery"
	if owner.mutateReadOnly != nil {
		owner.mutateReadOnly.digest = "read-only-mutated"
	}
	return nil
}

func (owner *fakeRetirementOwnerV1) Prepare(context.Context) (PreparedMonotonicRetirementV1, error) {
	owner.prepares++
	return fakePreparedRetirementV1{owner: owner}, nil
}

type fakePreparedRetirementV1 struct{ owner *fakeRetirementOwnerV1 }

func (plan fakePreparedRetirementV1) Validate(context.Context) error { return nil }
func (plan fakePreparedRetirementV1) Apply(context.Context) error {
	plan.owner.applies++
	plan.owner.retired = true
	plan.owner.digest = "retirement-after-apply-" + strconv.Itoa(plan.owner.applies)
	return nil
}

func TestCompositeRetirementValidatesEveryOwnerBeforeMutation(t *testing.T) {
	first := &fakeFixedPointOwnerV1{digest: "first"}
	later := &fakeFixedPointOwnerV1{digest: "later", failValidate: true}
	retirement := &fakeRetirementOwnerV1{
		fakeFixedPointOwnerV1: fakeFixedPointOwnerV1{digest: "retirement"},
	}
	if err := RunCompositeMonotonicRetirementV1(
		context.Background(), []FixedPointOwnerV1{first, later}, retirement,
	); err == nil {
		t.Fatal("later owner validation failure was accepted")
	}
	if retirement.recoveries != 0 || retirement.prepares != 0 || retirement.applies != 0 {
		t.Fatalf("mutation began before full validation: %#v", retirement)
	}
}

func TestCompositeRetirementRecoversThenAppliesUnderFreshFixedPoint(t *testing.T) {
	readOnly := &fakeFixedPointOwnerV1{digest: "read-only"}
	retirement := &fakeRetirementOwnerV1{
		fakeFixedPointOwnerV1: fakeFixedPointOwnerV1{digest: "retirement-pending"},
		recoveryRequired:      true,
	}
	if err := RunCompositeMonotonicRetirementV1(
		context.Background(), []FixedPointOwnerV1{readOnly}, retirement,
	); err != nil {
		t.Fatal(err)
	}
	if retirement.recoveries != 1 || retirement.prepares != 1 || retirement.applies != 1 || !retirement.retired {
		t.Fatalf("unexpected retirement lifecycle: %#v", retirement)
	}
}

func TestCompositeRetirementRejectsCrossOwnerRecoveryMutation(t *testing.T) {
	readOnly := &fakeFixedPointOwnerV1{digest: "read-only"}
	retirement := &fakeRetirementOwnerV1{
		fakeFixedPointOwnerV1: fakeFixedPointOwnerV1{digest: "retirement-pending"},
		recoveryRequired:      true,
		mutateReadOnly:        readOnly,
	}
	if err := RunCompositeMonotonicRetirementV1(
		context.Background(), []FixedPointOwnerV1{readOnly}, retirement,
	); err == nil {
		t.Fatal("cross-owner recovery mutation was accepted")
	}
	if retirement.prepares != 0 || retirement.applies != 0 {
		t.Fatalf("fresh plan ran after cross-owner mutation: %#v", retirement)
	}
}

func TestCompositeSemanticRetirementPreparesBeforeSemanticAndPreservesNewFixedPoint(t *testing.T) {
	managed := &fakeFixedPointOwnerV1{digest: "managed-before"}
	retirement := &fakeRetirementOwnerV1{
		fakeFixedPointOwnerV1: fakeFixedPointOwnerV1{digest: "retirement"},
	}
	semanticCalls := 0
	err := RunCompositeSemanticThenRetirementV2(
		context.Background(), []FixedPointOwnerV1{managed}, retirement,
		func(context.Context) error {
			semanticCalls++
			if retirement.prepares != 1 || retirement.applies != 0 {
				t.Fatal("retirement was not prepared read-only before semantic migration")
			}
			managed.digest = "managed-after-semantic"
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if semanticCalls != 1 || retirement.prepares != 1 || retirement.applies != 1 || !retirement.retired {
		t.Fatalf("unexpected semantic retirement lifecycle: calls=%d owner=%#v", semanticCalls, retirement)
	}
}

func TestCompositeSemanticRetirementFailureNeverAppliesPreparedRetirement(t *testing.T) {
	managed := &fakeFixedPointOwnerV1{digest: "managed"}
	retirement := &fakeRetirementOwnerV1{
		fakeFixedPointOwnerV1: fakeFixedPointOwnerV1{digest: "retirement"},
	}
	err := RunCompositeSemanticThenRetirementV2(
		context.Background(), []FixedPointOwnerV1{managed}, retirement,
		func(context.Context) error { return errors.New("semantic migration failed") },
	)
	if err == nil || retirement.prepares != 1 || retirement.applies != 0 || retirement.retired {
		t.Fatalf("semantic failure applied retirement: err=%v owner=%#v", err, retirement)
	}
}

func TestCompositeSemanticRetirementRejectsSemanticMutationOfRetirementOwner(t *testing.T) {
	managed := &fakeFixedPointOwnerV1{digest: "managed"}
	retirement := &fakeRetirementOwnerV1{
		fakeFixedPointOwnerV1: fakeFixedPointOwnerV1{digest: "retirement"},
	}
	err := RunCompositeSemanticThenRetirementV2(
		context.Background(), []FixedPointOwnerV1{managed}, retirement,
		func(context.Context) error {
			retirement.digest = "retirement-mutated"
			return nil
		},
	)
	if err == nil || retirement.applies != 0 {
		t.Fatalf("semantic retirement-owner mutation was accepted: err=%v owner=%#v", err, retirement)
	}
}
