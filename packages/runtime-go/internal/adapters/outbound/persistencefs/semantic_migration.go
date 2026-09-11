package persistencefs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

var ErrSemanticStartupMigrationBlocked = errors.New("semantic startup migration blocked")

type SemanticStartupMigrationBlocker struct {
	Code            string
	Artifact        string
	FoundVersion    int
	RequiredVersion int
}

func (blocker SemanticStartupMigrationBlocker) Error() string {
	return fmt.Sprintf("%s: %s requires schema version %d", ErrSemanticStartupMigrationBlocked, blocker.Artifact, blocker.RequiredVersion)
}

func (blocker SemanticStartupMigrationBlocker) Unwrap() error {
	return ErrSemanticStartupMigrationBlocked
}

func semanticJournalV3Blocker() error {
	return SemanticStartupMigrationBlocker{
		Code: semanticJournalV3MigrationCode(), Artifact: "journal",
		FoundVersion: 3, RequiredVersion: semanticJournalSchemaVersion,
	}
}

func semanticPlanningV1Blocker() error {
	return SemanticStartupMigrationBlocker{
		Code: semanticPlanningV1MigrationCode(), Artifact: "planning",
		FoundVersion: 1, RequiredVersion: semanticPlanningMarkerVersion,
	}
}

func semanticRetirementV1Blocker() error {
	return SemanticStartupMigrationBlocker{
		Code: semanticRetirementV1MigrationCode(), Artifact: "retirement",
		FoundVersion: 1, RequiredVersion: semanticJournalRetirementSchemaVersion,
	}
}

type authenticatedSemanticJournalV3 struct {
	journal semanticJournalV1
	blocker error
}

type authenticatedSemanticRetirementV1 struct {
	record  semanticJournalRetirementV1
	blocker error
}

func (legacy authenticatedSemanticRetirementV1) Error() string {
	return legacy.blocker.Error()
}

func (legacy authenticatedSemanticRetirementV1) Unwrap() error {
	return legacy.blocker
}

func (legacy authenticatedSemanticJournalV3) Error() string {
	return legacy.blocker.Error()
}

func (legacy authenticatedSemanticJournalV3) Unwrap() error {
	return legacy.blocker
}

func parseLegacyObjectIdentityV3(value string) ([3]uint64, error) {
	values := [3]uint64{}
	parts := strings.Split(value, ":")
	if len(parts) != 3 || value != strings.TrimSpace(value) {
		return values, errors.New("legacy object identity is not canonical")
	}
	for index, part := range parts {
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil || strconv.FormatUint(parsed, 10) != part {
			return [3]uint64{}, errors.New("legacy object identity is invalid")
		}
		values[index] = parsed
	}
	if values[0] == 0 || values[1] == 0 && values[2] == 0 {
		return [3]uint64{}, errors.New("legacy object identity is zero")
	}
	return values, nil
}

func validLegacyObjectIdentityV3(value string) bool {
	_, err := parseLegacyObjectIdentityV3(value)
	return err == nil
}

func validateSemanticJournalV3(journal semanticJournalV1, authority *startupJournalAuthority) error {
	if journal.SchemaVersion != 3 || !validLegacyObjectIdentityV3(journal.JournalRootIdentity) ||
		journal.JournalStageIdentity != "" && !validLegacyObjectIdentityV3(journal.JournalStageIdentity) ||
		!domainsecurity.IsSHA256Hex(journal.RootBindingDigest) || !domainsecurity.IsSHA256Hex(journal.RootCapabilityDigest) ||
		!domainsecurity.IsSHA256Hex(journal.JournalDigest) || !domainsecurity.IsSHA256Hex(journal.AuthorityKeyID) ||
		journal.AuthoritySignature == "" || domainstartup.ValidateSemanticStartupPlanV1(journal.Plan) != nil ||
		journal.NextOperation < 0 || journal.NextOperation > len(journal.Plan.Operations) ||
		semanticJournalDigest(journal) != journal.JournalDigest ||
		validateRootPromotionIntents(journal.RootCapabilities, journal.RootPromotionIntents) != nil ||
		authority == nil || authority.verifyDomain(
		semanticJournalV3SignatureDomain, journal.AuthorityKeyID,
		semanticJournalSigningBytes(journal), journal.AuthoritySignature,
	) != nil {
		return errors.New("semantic startup V3 journal integrity is invalid")
	}
	if len(journal.RootCapabilities) == 0 {
		return errors.New("semantic startup V3 root capability inventory is empty")
	}
	capabilities := make(map[string]frozenRootCapability, len(journal.RootCapabilities))
	for _, capability := range journal.RootCapabilities {
		if capability.Root == "" || capability.Anchor == "" || !validLegacyObjectIdentityV3(capability.AnchorIdentity) ||
			capability.RootIdentity == "" && capability.MissingSuffix == "" ||
			capability.RootIdentity != "" && (capability.MissingSuffix != "" || !validLegacyObjectIdentityV3(capability.RootIdentity)) {
			return errors.New("semantic startup V3 root capability is invalid")
		}
		key := canonicalPathKey(capability.Root)
		if _, duplicate := capabilities[key]; duplicate {
			return errors.New("semantic startup V3 root capability is duplicated")
		}
		capabilities[key] = capability
	}
	if rootCapabilityDigest(capabilities) != journal.RootCapabilityDigest {
		return errors.New("semantic startup V3 root capability digest is invalid")
	}
	if err := validateSemanticJournalState(journal); err != nil {
		return err
	}
	return nil
}

func validateSemanticJournalState(journal semanticJournalV1) error {
	switch journal.State {
	case semanticJournalPreparing, semanticJournalPrepared:
		if journal.NextOperation != 0 {
			return errors.New("semantic startup journal preparation cursor is invalid")
		}
	case semanticJournalApplying:
	case semanticJournalCommitted:
		if journal.NextOperation != len(journal.Plan.Operations) {
			return errors.New("semantic startup committed cursor is invalid")
		}
	default:
		return errors.New("semantic startup journal state is invalid")
	}
	if journal.State != semanticJournalPreparing && journal.JournalStageIdentity == "" {
		return errors.New("semantic startup journal stage authority is missing")
	}
	return nil
}
