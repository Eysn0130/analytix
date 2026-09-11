package finalauthority

import (
	"context"
	"errors"
	"path/filepath"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type privateCASOriginalLeafInitializationV1 struct {
	ctx          context.Context
	proof        *PreparedSecurePrivateCASOriginalCreateResiduesV1
	root         string
	afterCreated func(string) error
}

// InitializeLeafDirectoriesV1 creates only missing canonical owner/leaf
// directories under the unchanged exact-leaf write authority. It never
// consumes an original temporary directory or creates a record. The composing
// owner must first validate the independent request and complete candidate
// graph, then reobserve all leaves before opening a live store.
func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) InitializeLeafDirectoriesV1(ctx context.Context, root string, access SecurePrivateCASRecoveryAccessAuthority) error {
	return proof.initializeLeafDirectoriesV1(ctx, root, access, nil)
}

func (proof *PreparedSecurePrivateCASOriginalCreateResiduesV1) initializeLeafDirectoriesV1(ctx context.Context, root string, access SecurePrivateCASRecoveryAccessAuthority, afterCreated func(string) error) (resultErr error) {
	if ctx == nil || access == nil {
		return errors.New("original CAS leaf initialization authority is unavailable")
	}
	if _, err := proof.leafRelativeV1(root); err != nil {
		return err
	}
	if err := proof.Revalidate(ctx); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, proof.Revalidate(ctx)) }()
	if err := privateCASRecoveryExclusion.acquireLive(ctx); err != nil {
		return err
	}
	defer privateCASRecoveryExclusion.releaseLive()
	return withPrivateCASAccess(ctx, access, root, func(binding privatecasport.RootBinding) error {
		if err := proof.validateLeafBindingV1(root, binding); err != nil {
			return err
		}
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		if err := proof.revalidatePhysicalV1(ctx); err != nil {
			return err
		}
		err := securePrivateCASInitializeOriginalLeafV1(binding, privateCASOriginalLeafInitializationV1{ctx: ctx, proof: proof, root: root, afterCreated: afterCreated})
		return errors.Join(err, proof.revalidatePhysicalV1(ctx), ctx.Err())
	})
}

func (initial privateCASOriginalLeafInitializationV1) validateCreatePathV1(name string) error {
	if initial.ctx == nil || initial.proof == nil || (name != initial.proof.OwnerRootV1() && name != initial.root) {
		return errors.New("original CAS initialization escaped its canonical owner and leaf")
	}
	if _, err := initial.proof.leafRelativeV1(initial.root); err != nil {
		return err
	}
	return initial.ctx.Err()
}

func (initial privateCASOriginalLeafInitializationV1) createdV1(name string) error {
	if err := initial.ctx.Err(); err != nil {
		return err
	}
	if initial.afterCreated != nil {
		relative, err := filepath.Rel(initial.proof.DataRootV1(), name)
		if err != nil {
			return err
		}
		return initial.afterCreated(filepath.ToSlash(relative))
	}
	return nil
}
