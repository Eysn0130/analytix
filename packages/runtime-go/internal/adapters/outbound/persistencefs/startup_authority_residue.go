package persistencefs

import (
	"context"
	"errors"
	"strings"
)

func startupAuthorityCreateResiduePolicy(
	targetName string,
	maxBytes int64,
	validateBody func([]byte) error,
) startupExclusiveWriteResidueInventoryPolicy {
	return startupExclusiveWriteResidueInventoryPolicy{
		EntryLimit: maxSemanticNamespaceEntries,
		Targets: map[string]startupExclusiveWriteResidueTargetPolicy{
			targetName: {
				MaxBytes: maxBytes,
				Validate: func(tempName string, body []byte) error {
					if !validStartupAuthorityCreateTemp(tempName, targetName, body) || validateBody(body) != nil {
						return errors.New("startup authority create residue is invalid")
					}
					return nil
				},
			},
		},
		AllowCommitted: func(name string, _ bool) bool {
			return !strings.HasPrefix(name, "."+targetName+"-")
		},
	}
}

func preflightStartupAuthorityCreateResidue(
	root startupAuthorityRoot,
	targetName string,
	maxBytes int64,
	validateBody func([]byte) error,
) error {
	if !startupAuthorityNamedComponent(targetName) || maxBytes <= 0 || validateBody == nil {
		return errors.New("startup authority create-residue policy is invalid")
	}
	directory, err := secureStartupOpenRootDirectory(root)
	if err != nil {
		return err
	}
	plan, err := prepareStartupExclusiveWriteResidueCleanup(
		context.Background(), directory, startupAuthorityCreateResiduePolicy(targetName, maxBytes, validateBody),
	)
	if err == nil {
		err = plan.Revalidate(context.Background())
	}
	return errors.Join(err, directory.Close())
}

func cleanStartupAuthorityCreateResidue(
	root startupAuthorityRoot,
	targetName string,
	maxBytes int64,
	validateBody func([]byte) error,
) error {
	if !startupAuthorityNamedComponent(targetName) || maxBytes <= 0 || validateBody == nil {
		return errors.New("startup authority create-residue policy is invalid")
	}
	directory, err := secureStartupOpenRootDirectory(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	policy := startupAuthorityCreateResiduePolicy(targetName, maxBytes, validateBody)
	plan, err := prepareStartupExclusiveWriteResidueCleanup(context.Background(), directory, policy)
	if err != nil {
		return err
	}
	return plan.Apply(context.Background())
}
