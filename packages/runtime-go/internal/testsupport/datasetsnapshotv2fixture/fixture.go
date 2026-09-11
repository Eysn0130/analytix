package datasetsnapshotv2fixture

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

//go:embed sealed_fixture.b64
var fixtureFS embed.FS

type Fixture struct {
	Observation       domainsecurity.CaseBindingObservationV1
	Materials         map[datasetsnapshotport.MaterialKindV2]map[string][]byte
	ManifestReference datasetsnapshotport.ExactMaterialReferenceV2
	ProducerReference datasetsnapshotport.ExactMaterialReferenceV2
}

type fixtureEnvelope struct {
	Observation domainsecurity.CaseBindingObservationV1 `json:"observation"`
	Materials   map[string]map[string]string            `json:"materials"`
}

func Load() (Fixture, error) {
	encoded, err := fixtureFS.ReadFile("sealed_fixture.b64")
	if err != nil {
		return Fixture{}, err
	}
	body, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return Fixture{}, err
	}
	var envelope fixtureEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Fixture{}, err
	}
	fixture := Fixture{
		Observation: envelope.Observation,
		Materials:   make(map[datasetsnapshotport.MaterialKindV2]map[string][]byte, len(envelope.Materials)),
	}
	for rawKind, values := range envelope.Materials {
		kind := datasetsnapshotport.MaterialKindV2(rawKind)
		fixture.Materials[kind] = make(map[string][]byte, len(values))
		for digest, encodedValue := range values {
			value, decodeErr := base64.StdEncoding.DecodeString(encodedValue)
			if decodeErr != nil || domainsecurity.SHA256Hex(value) != digest {
				return Fixture{}, errors.New("sealed dataset snapshot fixture material is invalid")
			}
			fixture.Materials[kind][digest] = value
		}
	}
	_, manifestReference, err := soleMaterial(
		fixture.Materials, datasetsnapshotport.MaterialSnapshotManifestV2,
	)
	if err != nil {
		return Fixture{}, err
	}
	_, producerReference, err := soleMaterial(
		fixture.Materials, datasetsnapshotport.MaterialFundsProducerContentV1,
	)
	if err != nil {
		return Fixture{}, err
	}
	fixture.ManifestReference = manifestReference
	fixture.ProducerReference = producerReference
	return fixture, nil
}

func soleMaterial(
	materials map[datasetsnapshotport.MaterialKindV2]map[string][]byte,
	kind datasetsnapshotport.MaterialKindV2,
) ([]byte, datasetsnapshotport.ExactMaterialReferenceV2, error) {
	values := materials[kind]
	if len(values) != 1 {
		return nil, datasetsnapshotport.ExactMaterialReferenceV2{}, errors.New("sealed dataset snapshot fixture top-level material is ambiguous")
	}
	for digest, body := range values {
		return append([]byte(nil), body...), datasetsnapshotport.ExactMaterialReferenceV2{
			Address: digest, SHA256: digest, ByteLength: uint64(len(body)),
		}, nil
	}
	return nil, datasetsnapshotport.ExactMaterialReferenceV2{}, errors.New("sealed dataset snapshot fixture material is absent")
}

// CloneManifestInputV2 returns the exact reconstructable fields of a resolved
// signed snapshot so tests can add one analytical binding and re-issue the
// manifest without exposing restricted-evidence types in higher layers.
func CloneManifestInputV2(
	base datasetsnapshotport.ResolvedSnapshotV2,
) (domainsecurity.DatasetSnapshotManifestInputV2, error) {
	manifest := base.Manifest
	acquiredAt, err := time.Parse(time.RFC3339Nano, manifest.AcquiredAt)
	if err != nil {
		return domainsecurity.DatasetSnapshotManifestInputV2{}, err
	}
	return domainsecurity.DatasetSnapshotManifestInputV2{
		Binding:                           manifest.Binding,
		AcquisitionMethod:                 manifest.AcquisitionMethod,
		AcquiredAt:                        acquiredAt,
		AcquisitionActorDigest:            manifest.AcquisitionActorDigest,
		RawArtifactManifestDigest:         manifest.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:         manifest.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:     manifest.RawArtifactManifestByteLength,
		RawArtifactCount:                  manifest.RawArtifactCount,
		FundsProducerContentManifest:      base.FundsProducerContent,
		SourceType:                        manifest.SourceType,
		ProducerPolicyID:                  manifest.ProducerPolicyID,
		ProducerPolicyDigest:              manifest.ProducerPolicyDigest,
		ProducerComponentID:               manifest.ProducerComponentID,
		ProducerComponentVersion:          manifest.ProducerComponentVersion,
		ProducerOperation:                 manifest.ProducerOperation,
		ProducerOperationSchemaHash:       manifest.ProducerOperationSchemaHash,
		ParserID:                          manifest.ParserID,
		ParserVersion:                     manifest.ParserVersion,
		ParsedGenerationReceiptDigest:     manifest.ParsedGenerationReceiptDigest,
		ParsedGenerationReceiptSHA256:     manifest.ParsedGenerationReceiptSHA256,
		ParsedGenerationReceiptByteLength: manifest.ParsedGenerationReceiptByteLength,
		ClassificationLedgerDigest:        manifest.ClassificationLedgerDigest,
		ClassificationLedgerSHA256:        manifest.ClassificationLedgerSHA256,
		ClassificationLedgerByteLength:    manifest.ClassificationLedgerByteLength,
		TimezoneSemantics:                 manifest.TimezoneSemantics,
		CurrencySemantics:                 manifest.CurrencySemantics,
		SourceRowLedgerRootDigest:         manifest.SourceRowLedgerRootDigest,
		SourceRowLedgerRootSHA256:         manifest.SourceRowLedgerRootSHA256,
		SourceRowLedgerRootByteLength:     manifest.SourceRowLedgerRootByteLength,
		SourceRowLedgerPageCount:          manifest.SourceRowLedgerPageCount,
		SourceRecordCount:                 manifest.SourceRecordCount,
		AcceptedRecordCount:               manifest.AcceptedRecordCount,
		RejectedRecordCount:               manifest.RejectedRecordCount,
		DuplicateRecordCount:              manifest.DuplicateRecordCount,
	}, nil
}

func CloneMaterials(
	input map[datasetsnapshotport.MaterialKindV2]map[string][]byte,
) map[datasetsnapshotport.MaterialKindV2]map[string][]byte {
	output := make(map[datasetsnapshotport.MaterialKindV2]map[string][]byte, len(input))
	for kind, values := range input {
		output[kind] = make(map[string][]byte, len(values))
		for digest, body := range values {
			output[kind][digest] = append([]byte(nil), body...)
		}
	}
	return output
}
