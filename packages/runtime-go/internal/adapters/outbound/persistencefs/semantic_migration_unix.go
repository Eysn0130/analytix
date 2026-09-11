//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func semanticJournalV3MigrationCode() string {
	return "semantic_journal_v3_unix_full_identity_migration_required"
}

func semanticPlanningV1MigrationCode() string {
	return "semantic_planning_v1_unix_identity_migration"
}

func semanticRetirementV1MigrationCode() string {
	return "semantic_retirement_v1_unix_identity_migration"
}

func migrateSemanticPlanningMarkerV1(marker semanticPlanningMarkerV1) (semanticPlanningMarkerV1, error) {
	var err error
	marker.PlanningIdentity, err = canonicalUnixLegacyIdentityV3(marker.PlanningIdentity)
	if err != nil {
		return semanticPlanningMarkerV1{}, err
	}
	marker.StageIdentity, err = canonicalUnixLegacyIdentityV3(marker.StageIdentity)
	if err != nil {
		return semanticPlanningMarkerV1{}, err
	}
	marker.DataIdentity, err = canonicalUnixLegacyIdentityV3(marker.DataIdentity)
	if err != nil {
		return semanticPlanningMarkerV1{}, err
	}
	marker.DurableIdentity, err = canonicalUnixLegacyIdentityV3(marker.DurableIdentity)
	if err != nil {
		return semanticPlanningMarkerV1{}, err
	}
	marker.SchemaVersion = semanticPlanningMarkerVersion
	marker.Purpose = semanticPlanningMarkerPurpose
	return marker, nil
}

func migrateAuthenticatedSemanticJournalV3(
	directory *startupPrivateDirectory,
	legacy semanticJournalV1,
	authority *startupJournalAuthority,
	fault func(string) error,
) (semanticJournalV1, error) {
	journal, err := migrateSemanticJournalV3InMemory(directory, legacy, authority)
	if err != nil {
		return semanticJournalV1{}, err
	}
	body, err := json.Marshal(journal)
	if err != nil || len(body) > maxSemanticJournalBytes {
		return semanticJournalV1{}, errors.New("semantic startup V4 migration journal is invalid")
	}
	if err := directory.WriteReplace("journal.json", body, maxSemanticJournalBytes, fault); err != nil {
		return semanticJournalV1{}, err
	}
	readback, _, err := directory.ReadFile("journal.json", maxSemanticJournalBytes, false)
	if err != nil || !bytes.Equal(readback, body) {
		return semanticJournalV1{}, errors.New("semantic startup V4 migration readback changed")
	}
	migrated, err := decodeSemanticJournal(readback, authority)
	if err != nil || migrated.JournalRootIdentity != directory.Identity() {
		return semanticJournalV1{}, errors.New("semantic startup V4 migration verification failed")
	}
	return migrated, nil
}

func migrateSemanticJournalV3InMemory(
	directory *startupPrivateDirectory,
	legacy semanticJournalV1,
	authority *startupJournalAuthority,
) (semanticJournalV1, error) {
	if directory == nil || authority == nil {
		return semanticJournalV1{}, semanticJournalV3Blocker()
	}
	journal := legacy
	rootIdentity, err := canonicalUnixLegacyIdentityV3(journal.JournalRootIdentity)
	if err != nil || rootIdentity != directory.Identity() {
		return semanticJournalV1{}, semanticJournalV3Blocker()
	}
	journal.JournalRootIdentity = rootIdentity
	if journal.JournalStageIdentity != "" {
		journal.JournalStageIdentity, err = canonicalUnixLegacyIdentityV3(journal.JournalStageIdentity)
		if err != nil {
			return semanticJournalV1{}, semanticJournalV3Blocker()
		}
	}
	capabilities := make(map[string]frozenRootCapability, len(journal.RootCapabilities))
	for index := range journal.RootCapabilities {
		capability := &journal.RootCapabilities[index]
		capability.AnchorIdentity, err = canonicalUnixLegacyIdentityV3(capability.AnchorIdentity)
		if err != nil {
			return semanticJournalV1{}, semanticJournalV3Blocker()
		}
		if capability.RootIdentity != "" {
			capability.RootIdentity, err = canonicalUnixLegacyIdentityV3(capability.RootIdentity)
			if err != nil {
				return semanticJournalV1{}, semanticJournalV3Blocker()
			}
		}
		key := canonicalPathKey(capability.Root)
		if _, duplicate := capabilities[key]; duplicate {
			return semanticJournalV1{}, errors.New("semantic startup V3 migration root capability is duplicated")
		}
		capabilities[key] = *capability
	}
	journal.SchemaVersion = semanticJournalSchemaVersion
	journal.RootCapabilityDigest = rootCapabilityDigest(capabilities)
	journal.RootPromotionIntents = rootPromotionIntents(journal.RootCapabilities)
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature = ""
	journal.AuthoritySignature, err = authority.sign(semanticJournalSigningBytes(journal))
	if err != nil {
		return semanticJournalV1{}, err
	}
	if err := validateSemanticJournal(journal, authority); err != nil {
		return semanticJournalV1{}, errors.New("semantic startup V4 migration verification failed")
	}
	return journal, nil
}

func decodeSemanticJournalForRetirement(
	directory *startupPrivateDirectory,
	body []byte,
	authority *startupJournalAuthority,
) (semanticJournalV1, error) {
	journal, err := decodeSemanticJournal(body, authority)
	var legacy authenticatedSemanticJournalV3
	if errors.As(err, &legacy) {
		return migrateSemanticJournalV3InMemory(directory, legacy.journal, authority)
	}
	return journal, err
}

func migrateAuthenticatedSemanticRetirementV1(
	directory *startupPrivateDirectory,
	legacy semanticJournalRetirementV1,
	authority *startupJournalAuthority,
	roots RootSet,
) (semanticJournalRetirementV1, error) {
	if directory == nil || authority == nil {
		return semanticJournalRetirementV1{}, semanticRetirementV1Blocker()
	}
	legacyRoot, err := canonicalUnixLegacyIdentityV3(legacy.JournalRootIdentity)
	if err != nil || legacyRoot != directory.Identity() || legacy.RootBindingDigest != rootBindingDigest(roots) {
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement root identity changed")
	}
	current, err := semanticRetirementRecordForDirectory(directory, authority, roots, legacy.ConfigurationDigest)
	if err != nil {
		return semanticJournalRetirementV1{}, err
	}
	switch {
	case legacy.Disposition != current.Disposition:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement disposition changed")
	case legacy.RootBindingDigest != current.RootBindingDigest:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement root binding changed")
	case legacy.ConfigurationDigest != current.ConfigurationDigest:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement configuration changed")
	case legacy.SourceEntry != current.SourceEntry:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement source changed")
	case legacy.JournalBodyDigest != current.JournalBodyDigest:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement body digest changed")
	case legacy.JournalState != current.JournalState:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement journal state changed")
	case legacy.PlanDigest != current.PlanDigest:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement plan changed")
	case legacy.NextOperation != current.NextOperation || legacy.OperationCount != current.OperationCount:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement cursor changed")
	case legacy.AuthorityKeyID != current.AuthorityKeyID:
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement authority changed")
	}
	if legacy.JournalState == semanticJournalRetirementEmpty {
		if legacy.JournalDigest != current.JournalDigest {
			return semanticJournalRetirementV1{}, errors.New("semantic startup V1 empty retirement digest changed")
		}
	} else {
		body, _, readErr := directory.ReadFile(legacy.SourceEntry, maxSemanticJournalBytes, false)
		if readErr != nil || domainsecurity.SHA256Hex(body) != legacy.JournalBodyDigest {
			return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement journal body changed")
		}
		_, decodeErr := decodeSemanticJournal(body, authority)
		var journalV3 authenticatedSemanticJournalV3
		if !errors.As(decodeErr, &journalV3) || journalV3.journal.JournalDigest != legacy.JournalDigest ||
			journalV3.journal.JournalRootIdentity != legacy.JournalRootIdentity ||
			journalV3.journal.State != legacy.JournalState || journalV3.journal.NextOperation != legacy.NextOperation ||
			len(journalV3.journal.Plan.Operations) != legacy.OperationCount || journalV3.journal.Plan.PlanDigest != legacy.PlanDigest {
			return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement journal frontier changed")
		}
	}
	entries, err := directory.ReadEntriesBounded(maxSemanticRetirementTreeEntries)
	if err != nil {
		return semanticJournalRetirementV1{}, err
	}
	visible := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() != semanticJournalRetirementFile {
			visible = append(visible, entry)
		}
	}
	residue, err := semanticRetirementResidueInventory(directory, authority, visible, current)
	if err != nil {
		return semanticJournalRetirementV1{}, err
	}
	for index := range residue {
		if residue[index].Identity == "" {
			continue
		}
		identity, parseErr := parseStrongDirectoryIdentity(residue[index].Identity)
		if parseErr != nil || identity.Kind != "unix-dev-inode-v1" {
			return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement residue identity changed")
		}
		residue[index].Identity = formatLegacyDirectoryIdentityV3(identity.Device, identity.Inode, 0)
	}
	residueBody, _ := json.Marshal(residue)
	if domainsecurity.SHA256Hex(residueBody) != legacy.ResidueDigest {
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement residue changed")
	}
	current.AuthoritySignature = ""
	current.AuthoritySignature, err = authority.signDomain(
		semanticJournalRetirementSignatureDomain,
		semanticRetirementSigningBytes(current),
	)
	if err != nil || validateSemanticRetirement(current, authority) != nil {
		return semanticJournalRetirementV1{}, errors.New("semantic startup V1 retirement migration could not issue V2 authority")
	}
	return current, nil
}

func canonicalUnixLegacyIdentityV3(value string) (string, error) {
	identity, err := parseLegacyObjectIdentityV3(value)
	if err != nil || identity[2] != 0 {
		return "", errors.New("semantic startup V3 Unix identity is invalid")
	}
	return formatUnixObjectIdentity(identity[0], identity[1]), nil
}
