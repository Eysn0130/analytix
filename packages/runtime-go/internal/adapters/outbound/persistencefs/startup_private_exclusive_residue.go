package persistencefs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sort"
	"strings"
)

type startupExclusiveWriteResidueTargetPolicy struct {
	MaxBytes int64
	Validate func(tempName string, body []byte) error
}

type startupExclusiveWriteResidueInventoryPolicy struct {
	EntryLimit     int
	Targets        map[string]startupExclusiveWriteResidueTargetPolicy
	AllowCommitted func(name string, directory bool) bool
}

type startupExclusiveWriteResidueCandidate struct {
	tempName       string
	targetName     string
	body           []byte
	tempIdentity   string
	targetBody     []byte
	targetIdentity string
	targetPresent  bool
}

type startupExclusiveWriteResidueEntry struct {
	name      string
	directory bool
}

type startupExclusiveWriteResidueCleanupPlan struct {
	directory         *startupPrivateDirectory
	directoryIdentity string
	policy            startupExclusiveWriteResidueInventoryPolicy
	inventory         []startupExclusiveWriteResidueEntry
	candidates        []startupExclusiveWriteResidueCandidate
}

func prepareStartupExclusiveWriteResidueCleanup(
	ctx context.Context,
	directory *startupPrivateDirectory,
	policy startupExclusiveWriteResidueInventoryPolicy,
) (*startupExclusiveWriteResidueCleanupPlan, error) {
	if err := validateStartupExclusiveWriteResiduePolicy(policy); err != nil {
		return nil, err
	}
	entries, err := directory.ReadEntriesBoundedContext(ctx, policy.EntryLimit)
	if err != nil {
		return nil, err
	}
	plan := &startupExclusiveWriteResidueCleanupPlan{
		directory: directory, directoryIdentity: directory.Identity(), policy: policy,
		inventory: make([]startupExclusiveWriteResidueEntry, 0, len(entries)),
	}
	seenTargets := make(map[string]struct{}, len(policy.Targets))
	for _, entry := range entries {
		plan.inventory = append(plan.inventory, startupExclusiveWriteResidueEntry{
			name: entry.Name(), directory: entry.IsDir(),
		})
		target, matched, matchErr := startupExclusiveWriteResidueTarget(entry.Name(), policy.Targets)
		if matchErr != nil {
			return nil, matchErr
		}
		if !matched {
			if policy.AllowCommitted == nil || !policy.AllowCommitted(entry.Name(), entry.IsDir()) {
				return nil, errors.New("startup exclusive-write residue inventory contains an unknown entry")
			}
			continue
		}
		if entry.IsDir() {
			return nil, errors.New("startup exclusive-write residue is not a regular file")
		}
		if _, duplicate := seenTargets[target]; duplicate {
			return nil, errors.New("startup exclusive-write residue inventory is ambiguous")
		}
		seenTargets[target] = struct{}{}
		candidate, candidateErr := snapshotStartupExclusiveWriteResidue(directory, entry.Name(), target, policy.Targets[target])
		if candidateErr != nil {
			return nil, candidateErr
		}
		plan.candidates = append(plan.candidates, candidate)
	}
	sort.Slice(plan.inventory, func(left, right int) bool { return plan.inventory[left].name < plan.inventory[right].name })
	sort.Slice(plan.candidates, func(left, right int) bool { return plan.candidates[left].tempName < plan.candidates[right].tempName })
	return plan, nil
}

func (plan *startupExclusiveWriteResidueCleanupPlan) Revalidate(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if plan == nil || plan.directory == nil || plan.directory.Identity() != plan.directoryIdentity {
		return errors.New("startup exclusive-write residue directory identity changed")
	}
	entries, err := plan.directory.ReadEntriesBoundedContext(ctx, plan.policy.EntryLimit)
	if err != nil {
		return err
	}
	inventory := make([]startupExclusiveWriteResidueEntry, 0, len(entries))
	for _, entry := range entries {
		inventory = append(inventory, startupExclusiveWriteResidueEntry{name: entry.Name(), directory: entry.IsDir()})
	}
	sort.Slice(inventory, func(left, right int) bool { return inventory[left].name < inventory[right].name })
	if len(inventory) != len(plan.inventory) {
		return errors.New("startup exclusive-write residue inventory changed after preflight")
	}
	for index := range inventory {
		if inventory[index] != plan.inventory[index] {
			return errors.New("startup exclusive-write residue inventory changed after preflight")
		}
	}
	for _, candidate := range plan.candidates {
		current, err := snapshotStartupExclusiveWriteResidue(
			plan.directory, candidate.tempName, candidate.targetName, plan.policy.Targets[candidate.targetName],
		)
		if err != nil || !sameStartupExclusiveWriteResidueCandidate(candidate, current) {
			return errors.New("startup exclusive-write residue changed after preflight")
		}
	}
	return nil
}

func (plan *startupExclusiveWriteResidueCleanupPlan) Apply(ctx context.Context) error {
	if err := plan.Revalidate(ctx); err != nil {
		return err
	}
	if len(plan.candidates) == 0 {
		return nil
	}
	for _, candidate := range plan.candidates {
		current, err := snapshotStartupExclusiveWriteResidue(
			plan.directory, candidate.tempName, candidate.targetName, plan.policy.Targets[candidate.targetName],
		)
		if err != nil || !sameStartupExclusiveWriteResidueCandidate(candidate, current) {
			return errors.New("startup exclusive-write residue changed before deletion")
		}
		if err := plan.directory.removeStartupExclusiveWriteResidue(candidate.tempName, candidate.tempIdentity); err != nil {
			return err
		}
	}
	if err := plan.directory.syncStartupExclusiveWriteResidues(); err != nil {
		return err
	}
	for _, candidate := range plan.candidates {
		if _, _, err := plan.directory.ReadFile(candidate.tempName, plan.policy.Targets[candidate.targetName].MaxBytes, false); !errors.Is(err, os.ErrNotExist) {
			return errors.New("startup exclusive-write residue deletion was not durable")
		}
	}
	return nil
}

func validateStartupExclusiveWriteResiduePolicy(policy startupExclusiveWriteResidueInventoryPolicy) error {
	if policy.EntryLimit <= 0 || policy.EntryLimit > maxStartupPrivateEntries || len(policy.Targets) == 0 {
		return errors.New("startup exclusive-write residue policy is invalid")
	}
	for target, targetPolicy := range policy.Targets {
		if !startupAuthorityNamedComponent(target) || targetPolicy.MaxBytes <= 0 || targetPolicy.Validate == nil {
			return errors.New("startup exclusive-write residue target policy is invalid")
		}
	}
	return nil
}

func startupExclusiveWriteResidueTarget(
	name string,
	policies map[string]startupExclusiveWriteResidueTargetPolicy,
) (string, bool, error) {
	matched := ""
	for target := range policies {
		if strings.HasPrefix(name, "."+target+"-") {
			if matched != "" {
				return "", false, errors.New("startup exclusive-write residue target is ambiguous")
			}
			matched = target
		}
	}
	if matched == "" {
		return "", false, nil
	}
	if !startupAuthorityNamedComponent(name) {
		return "", false, errors.New("startup exclusive-write residue name is unsafe")
	}
	return matched, true, nil
}

func snapshotStartupExclusiveWriteResidue(
	directory *startupPrivateDirectory,
	tempName string,
	targetName string,
	policy startupExclusiveWriteResidueTargetPolicy,
) (startupExclusiveWriteResidueCandidate, error) {
	body, tempIdentity, err := directory.readStartupExclusiveWriteResidue(tempName, policy.MaxBytes)
	if err != nil || policy.Validate(tempName, body) != nil {
		return startupExclusiveWriteResidueCandidate{}, errors.New("startup exclusive-write residue authority is invalid")
	}
	targetBody, targetIdentity, targetErr := directory.ReadFile(targetName, policy.MaxBytes, false)
	targetPresent := targetErr == nil
	if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
		return startupExclusiveWriteResidueCandidate{}, errors.New("startup exclusive-write target is unsafe")
	}
	if targetPresent && !bytes.Equal(targetBody, body) {
		return startupExclusiveWriteResidueCandidate{}, errors.New("startup exclusive-write residue conflicts with its committed target")
	}
	return startupExclusiveWriteResidueCandidate{
		tempName: tempName, targetName: targetName, body: append([]byte(nil), body...), tempIdentity: tempIdentity,
		targetBody: append([]byte(nil), targetBody...), targetIdentity: targetIdentity, targetPresent: targetPresent,
	}, nil
}

func sameStartupExclusiveWriteResidueCandidate(
	left startupExclusiveWriteResidueCandidate,
	right startupExclusiveWriteResidueCandidate,
) bool {
	return left.tempName == right.tempName && left.targetName == right.targetName &&
		left.tempIdentity == right.tempIdentity && bytes.Equal(left.body, right.body) &&
		left.targetPresent == right.targetPresent && left.targetIdentity == right.targetIdentity &&
		bytes.Equal(left.targetBody, right.targetBody)
}
