package finalauthority

// This file retains the pre-V4 transaction only as a same-package fault oracle.
// It is excluded from production builds; signed-journal V4 is the sole shipped
// deletion authority.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

type preparedSecurePrivateCASLeafAuthorityV3 struct {
	plan *PreparedSecurePrivateCASRecoveryV1
}

func (authority preparedSecurePrivateCASLeafAuthorityV3) Revalidate(ctx context.Context) error {
	if authority.plan == nil {
		return errors.New("private CAS leaf recovery authority is invalid")
	}
	return authority.plan.Revalidate(ctx)
}

func (authority preparedSecurePrivateCASLeafAuthorityV3) SecurePrivateCASRecoveryPlansV2() []*PreparedSecurePrivateCASRecoveryV1 {
	if authority.plan == nil {
		return nil
	}
	return []*PreparedSecurePrivateCASRecoveryV1{authority.plan}
}

func (preparedSecurePrivateCASLeafAuthorityV3) PrivateCASRecoveryTopologiesV3() []SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return nil
}

func newPrivateCASRecoveryTransactionIDForV3Oracle() (string, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

func privateCASRecoveryV3OracleWitness(transactionID string) verifiedPrivateCASCommitWitnessV4 {
	witness := sha256.Sum256([]byte("analytix.private-cas-v3-oracle-witness\x00" + transactionID))
	target := sha256.Sum256([]byte("analytix.private-cas-v3-oracle-target\x00" + transactionID))
	return verifiedPrivateCASCommitWitnessV4{
		transactionID: transactionID, witnessDigest: hex.EncodeToString(witness[:]),
		commitTargetID: hex.EncodeToString(target[:]),
	}
}

func ApplyPreparedSecurePrivateCASRecoveryTransactionV2(
	ctx context.Context,
	plans []*PreparedSecurePrivateCASRecoveryV1,
) error {
	if len(plans) == 0 {
		return errors.New("private CAS recovery transaction has no participants")
	}
	authorities := make([]PreparedSecurePrivateCASRecoveryAuthorityV3, 0, len(plans))
	for _, plan := range plans {
		authorities = append(authorities, preparedSecurePrivateCASLeafAuthorityV3{plan: plan})
	}
	return ApplyPreparedSecurePrivateCASRecoveryTransactionV3(ctx, authorities)
}

func ApplyPreparedSecurePrivateCASRecoveryTransactionV3(
	ctx context.Context,
	authorities []PreparedSecurePrivateCASRecoveryAuthorityV3,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := privateCASRecoveryExclusion.acquireRecovery(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseRecovery()
	prepared, err := prepareSecurePrivateCASRecoveryAuthoritySetV3(ctx, authorities)
	if err != nil {
		return err
	}
	return applyPreparedSecurePrivateCASRecoveryTransactionV3Oracle(ctx, prepared)
}

func applyPreparedSecurePrivateCASRecoveryTransactionV3Oracle(
	ctx context.Context,
	prepared *preparedSecurePrivateCASRecoveryAuthoritySetV3,
) error {
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	plans := prepared.plans
	global := privateCASRecoveryTransactionState{}
	for _, plan := range plans {
		state, err := plan.transactionState()
		if err != nil {
			return err
		}
		if state.transactionID != "" {
			if global.transactionID != "" && global.transactionID != state.transactionID {
				return errors.New("private CAS recovery owners disagree on quarantine transaction")
			}
			global.transactionID = state.transactionID
		}
		global.staged += state.staged
		global.committed += state.committed
		global.plain += state.plain
		global.residues += state.residues
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	if global.committed > 1 || global.committed == 1 && global.plain != 0 {
		return errors.New("private CAS recovery commit marker topology is ambiguous")
	}
	if global.committed == 1 {
		return commitPreparedPrivateCASRecoveryWithWitnessV4(
			ctx, prepared, privateCASRecoveryV3OracleWitness(global.transactionID),
		)
	}
	if global.staged != 0 {
		if err := rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, len(plans)-1); err != nil {
			return errors.Join(errors.New("private CAS stale quarantine rollback failed"), err)
		}
	}
	transactionID, err := newPrivateCASRecoveryTransactionIDForV3Oracle()
	if err != nil {
		return err
	}
	if global.residues == 0 {
		for index, plan := range plans {
			if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
				return err
			}
			created, err := plan.createTransactionMarker(ctx, transactionID)
			if err != nil {
				return err
			}
			if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
				return err
			}
			if created {
				global.residues = 1
				break
			}
		}
		if err := prepared.revalidate(ctx); err != nil {
			return err
		}
	}
	if global.residues == 0 {
		for index, plan := range plans {
			if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
				return err
			}
			if err := revokePrivateCASRootGeneration(plan.binding, plan.rootPath); err != nil {
				return err
			}
			if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
				return err
			}
		}
		return prepared.revalidate(ctx)
	}
	for index, plan := range plans {
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return err
		}
		if err := privateCASRecoveryTransactionTestCut("before_plan_stage", index); err != nil {
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, index-1))
		}
		if err := plan.stageTransaction(ctx, transactionID); err != nil {
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, index))
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, index))
		}
		if err := privateCASRecoveryTransactionTestCut("after_plan_stage", index); err != nil {
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, index))
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, index))
		}
	}
	if err := prepared.revalidate(ctx); err != nil {
		return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, len(plans)-1))
	}
	markerPlan := -1
	for index, plan := range plans {
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, len(plans)-1))
		}
		marked, err := plan.markTransactionCommit(ctx, transactionID)
		if err != nil {
			if marked {
				return err
			}
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, len(plans)-1))
		}
		if err := prepared.revalidatePlanTopology(ctx, index); err != nil {
			if marked {
				return err
			}
			return errors.Join(err, rollbackPreparedPrivateCASRecoveryTransactionV3(ctx, prepared, len(plans)-1))
		}
		if marked {
			markerPlan = index
			break
		}
	}
	if markerPlan < 0 {
		return errors.New("private CAS recovery transaction could not persist a commit marker")
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	if err := privateCASRecoveryTransactionTestCut("after_commit_marker", markerPlan); err != nil {
		return err
	}
	if err := prepared.revalidate(ctx); err != nil {
		return err
	}
	return commitPreparedPrivateCASRecoveryWithWitnessV4(
		ctx, prepared, privateCASRecoveryV3OracleWitness(transactionID),
	)
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) Apply(ctx context.Context) error {
	if prepared == nil || prepared.access == nil || prepared.gate == nil || prepared.rootPath == "" {
		return errors.New("private CAS prepared recovery is invalid")
	}
	return ApplyPreparedSecurePrivateCASRecoveryTransactionV2(ctx, []*PreparedSecurePrivateCASRecoveryV1{prepared})
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) Apply(ctx context.Context) error {
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	if err := ApplyPreparedSecurePrivateCASRecoveryTransactionV3(
		ctx, []PreparedSecurePrivateCASRecoveryAuthorityV3{prepared},
	); err != nil {
		return err
	}
	return prepared.container.revalidate(ctx)
}

func RecoverSecurePrivateCASIfPresent(
	ctx context.Context,
	root string,
	maxBytes int,
	access SecurePrivateCASRecoveryAccessAuthority,
) error {
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, maxBytes, access)
	if err != nil {
		return err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	return prepared.Apply(ctx)
}
