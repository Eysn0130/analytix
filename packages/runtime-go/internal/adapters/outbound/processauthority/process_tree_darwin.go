//go:build darwin

package processauthority

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"syscall"
	"time"
)

var errDarwinProcessTree = errors.New("darwin process tree authority unavailable")

type darwinProcessTree struct {
	root   darwinProcessIdentity
	caller darwinProcInfoCaller
	nodes  map[int]darwinProcessIdentity
	order  []int
	frozen bool
}

func newDarwinProcessTree(root darwinProcessIdentity) *darwinProcessTree {
	return newDarwinProcessTreeWithCaller(root, realDarwinProcInfoCaller)
}

func newDarwinProcessTreeWithCaller(root darwinProcessIdentity, caller darwinProcInfoCaller) *darwinProcessTree {
	if root.PID <= 0 || root.PGID <= 0 || root.UID != uint32(os.Geteuid()) || root.StartSec == 0 ||
		root.StartUSec >= 1_000_000 || caller == nil {
		return nil
	}
	return &darwinProcessTree{
		root: root, caller: caller,
		nodes: map[int]darwinProcessIdentity{root.PID: root}, order: []int{root.PID},
	}
}

func (tree *darwinProcessTree) Freeze(ctx context.Context) error {
	if tree == nil || ctx == nil || tree.caller == nil || darwinContextFailure(ctx) != nil {
		return errDarwinProcessTree
	}
	tree.nodes = map[int]darwinProcessIdentity{tree.root.PID: tree.root}
	tree.order = []int{tree.root.PID}
	tree.frozen = false
	root, err := stopDarwinProcessIdentityWith(ctx, tree.root, tree.caller)
	if err != nil || root.Status != darwinProcStatusStopped {
		return errDarwinProcessTree
	}

	for parentIndex := 0; parentIndex < len(tree.order); parentIndex++ {
		parentID := tree.order[parentIndex]
		parent := tree.nodes[parentID]
		stable := false
		for attempt := 0; attempt < darwinMaximumChildCapacity; attempt++ {
			first, listErr := listDarwinDirectChildrenWith(ctx, parent, tree.caller)
			if listErr != nil {
				return errDarwinProcessTree
			}
			for _, child := range first {
				childIdentity := child.Identity
				if existing, ok := tree.nodes[childIdentity.PID]; ok {
					if existing != childIdentity {
						return errDarwinProcessTree
					}
				} else {
					if len(tree.nodes) >= darwinMaximumChildCapacity {
						return errDarwinProcessTree
					}
					tree.nodes[childIdentity.PID] = childIdentity
					tree.order = append(tree.order, childIdentity.PID)
				}
				stopped, stopErr := stopDarwinProcessIdentityWith(ctx, childIdentity, tree.caller)
				if stopErr != nil || stopped.Status != darwinProcStatusStopped {
					return errDarwinProcessTree
				}
			}
			second, listErr := listDarwinDirectChildrenWith(ctx, parent, tree.caller)
			if listErr != nil {
				return errDarwinProcessTree
			}
			if sameDarwinChildSet(first, second) {
				stable = true
				break
			}
		}
		if !stable {
			return errDarwinProcessTree
		}
	}

	// All discovered processes are stopped. Re-list every parent and require a
	// closed edge set so a late child, truncated list, or identity drift cannot
	// be interpreted as an empty tree.
	for _, parentID := range tree.order {
		parent := tree.nodes[parentID]
		state, stateErr := readDarwinProcessStateWith(ctx, parentID, true, tree.caller)
		if stateErr != nil || !state.sameIdentity(parent) || state.Status != darwinProcStatusStopped {
			return errDarwinProcessTree
		}
		children, listErr := listDarwinDirectChildrenWith(ctx, parent, tree.caller)
		if listErr != nil {
			return errDarwinProcessTree
		}
		for _, child := range children {
			captured, ok := tree.nodes[child.Identity.PID]
			if !ok || captured != child.Identity || child.Status != darwinProcStatusStopped {
				return errDarwinProcessTree
			}
		}
	}
	tree.frozen = true
	return nil
}

func (tree *darwinProcessTree) RequireNoDescendants() error {
	if tree == nil || !tree.frozen || len(tree.nodes) != 1 || len(tree.order) != 1 ||
		tree.nodes[tree.root.PID] != tree.root || tree.order[0] != tree.root.PID {
		return errDarwinProcessTree
	}
	return nil
}

func (tree *darwinProcessTree) ResumeRoot(ctx context.Context) error {
	if tree == nil || ctx == nil || tree.RequireNoDescendants() != nil {
		return errDarwinProcessTree
	}
	state, err := readDarwinProcessStateWith(ctx, tree.root.PID, true, tree.caller)
	if err != nil || !state.sameIdentity(tree.root) || state.Status != darwinProcStatusStopped {
		return errDarwinProcessTree
	}
	if err := syscall.Kill(tree.root.PID, syscall.SIGCONT); err != nil {
		return errDarwinProcessTree
	}
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		state, err = readDarwinProcessStateWith(ctx, tree.root.PID, true, tree.caller)
		if err != nil || !state.sameIdentity(tree.root) || state.Status == darwinProcStatusZombie {
			return errDarwinProcessTree
		}
		if state.Status != darwinProcStatusStopped {
			tree.frozen = false
			tree.nodes = map[int]darwinProcessIdentity{tree.root.PID: tree.root}
			tree.order = []int{tree.root.PID}
			return nil
		}
		time.Sleep(time.Millisecond)
	}
}

func (tree *darwinProcessTree) Terminate(ctx context.Context, process *os.Process) error {
	if tree == nil || ctx == nil || process == nil || process.Pid != tree.root.PID {
		return errDarwinProcessTree
	}
	failed := false
	if !tree.frozen {
		if err := tree.Freeze(ctx); err != nil {
			failed = true
		}
	}

	// Reverse breadth-first discovery is leaves-first for every captured edge.
	for index := len(tree.order) - 1; index >= 1; index-- {
		identity := tree.nodes[tree.order[index]]
		state, err := stopDarwinProcessIdentityWith(ctx, identity, tree.caller)
		if err != nil {
			if !errors.Is(err, syscall.ESRCH) {
				failed = true
			}
			continue
		}
		if state.Status != darwinProcStatusStopped || syscall.Kill(identity.PID, syscall.SIGKILL) != nil {
			failed = true
		}
	}

	rootState, rootErr := stopDarwinProcessIdentityWith(ctx, tree.root, tree.caller)
	if rootErr != nil || rootState.Status != darwinProcStatusStopped {
		failed = true
	}

	// The root is our retained, unreaped direct child. Its PID cannot be reused
	// before Process.Wait, so kill the exact process and original group
	// immediately even when proc_info or the stop proof failed. Never spend the
	// remaining cleanup deadline waiting for a process that has not been killed.
	if !killAndWaitDarwinProcess(process, tree.root.PID) {
		failed = true
	}

	// Freeze/stop may consume the caller deadline. Termination proof gets its
	// own bounded window after the synchronous reap; an expired operation
	// context must never turn into a skipped ESRCH check.
	verificationContext, cancelVerification := context.WithTimeout(context.Background(), darwinSessionCleanupDeadline)
	defer cancelVerification()
	for index := len(tree.order) - 1; index >= 0; index-- {
		identity := tree.nodes[tree.order[index]]
		if waitDarwinIdentityESRCHWith(verificationContext, identity, tree.caller) != nil {
			failed = true
		}
	}
	tree.frozen = false
	if failed {
		return errDarwinProcessTree
	}
	return nil
}

func stopDarwinProcessIdentityWith(
	ctx context.Context,
	identity darwinProcessIdentity,
	caller darwinProcInfoCaller,
) (darwinProcessState, error) {
	state, err := readDarwinProcessStateWith(ctx, identity.PID, true, caller)
	if err != nil {
		return darwinProcessState{}, err
	}
	if !state.sameIdentity(identity) || state.Status == darwinProcStatusZombie {
		return darwinProcessState{}, errDarwinProcessTree
	}
	if state.Status != darwinProcStatusStopped {
		if err := syscall.Kill(identity.PID, syscall.SIGSTOP); err != nil {
			return darwinProcessState{}, fmt.Errorf("%w: %w", errDarwinProcessTree, err)
		}
	}
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return darwinProcessState{}, err
		}
		state, err = readDarwinProcessStateWith(ctx, identity.PID, true, caller)
		if err != nil || !state.sameIdentity(identity) || state.Status == darwinProcStatusZombie {
			return darwinProcessState{}, errDarwinProcessTree
		}
		if state.Status == darwinProcStatusStopped {
			return state, nil
		}
		time.Sleep(time.Millisecond)
	}
}

func waitDarwinIdentityESRCHWith(
	ctx context.Context,
	identity darwinProcessIdentity,
	caller darwinProcInfoCaller,
) error {
	for {
		if err := darwinContextFailure(ctx); err != nil {
			return err
		}
		state, err := readDarwinProcessStateWith(ctx, identity.PID, true, caller)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil || !state.sameIdentity(identity) {
			return errDarwinProcessTree
		}
		time.Sleep(time.Millisecond)
	}
}

func sameDarwinChildSet(left, right []darwinProcessState) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]darwinProcessState(nil), left...)
	rightCopy := append([]darwinProcessState(nil), right...)
	sort.Slice(leftCopy, func(i, j int) bool { return leftCopy[i].Identity.PID < leftCopy[j].Identity.PID })
	sort.Slice(rightCopy, func(i, j int) bool { return rightCopy[i].Identity.PID < rightCopy[j].Identity.PID })
	for index := range leftCopy {
		if leftCopy[index].Identity != rightCopy[index].Identity {
			return false
		}
	}
	return true
}
