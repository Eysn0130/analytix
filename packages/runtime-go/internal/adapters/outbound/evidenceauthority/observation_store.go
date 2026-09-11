package evidenceauthority

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

const (
	evidenceObservationSchemaVersion = 1
	evidenceObservationPurpose       = "analytix.evidence-authority-observation-bundle/v1"
	maxEvidenceObservationBytes      = 2 << 20
)

var _ storeport.ObservationStore = (*ObservationStore)(nil)

type evidenceObservationWireV1 struct {
	SchemaVersion int             `json:"schemaVersion"`
	Purpose       string          `json:"purpose"`
	Bundle        json.RawMessage `json:"bundle"`
	Request       json.RawMessage `json:"request"`
	Observation   json.RawMessage `json:"observation"`
}

// ObservationStore preserves the exact fresh challenge, witness response and
// selected bundle by observation digest. It is historical audit evidence and
// exposes no operation capable of choosing a current head.
type ObservationStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

func (store *ObservationStore) Close() error {
	if store == nil || store.cas == nil {
		return nil
	}
	return store.cas.Close()
}

func NewObservationStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ObservationStore, error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("evidence authority observation root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxEvidenceObservationBytes, access)
	if err != nil {
		return nil, err
	}
	return &ObservationStore{cas: cas}, nil
}

func (store *ObservationStore) HasRecords(ctx context.Context) (bool, error) {
	if store == nil || store.cas == nil || ctx == nil {
		return false, errors.New("evidence authority observation store is unavailable")
	}
	found := errors.New("evidence authority observation record found")
	err := store.cas.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error { return found })
	if errors.Is(err, found) {
		return true, nil
	}
	return false, err
}

func (store *ObservationStore) PutIfAbsent(ctx context.Context, bundle storeport.ObservationBundle) error {
	if store == nil || store.cas == nil {
		return errors.New("evidence authority observation store is unavailable")
	}
	body, err := evidenceObservationBytes(bundle)
	if err != nil || len(body) == 0 || len(body) > maxEvidenceObservationBytes {
		return errors.New("evidence authority observation bundle is invalid")
	}
	digest := bundle.Observation.ObservationDigest
	if current, readErr := store.cas.Read(ctx, digest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("evidence authority observation conflicts with its content address")
		}
		stored, parseErr := parseStoredObservation(digest, current)
		if parseErr != nil || !equalObservation(stored, bundle) {
			return errors.New("existing evidence authority observation verification failed")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := store.cas.PutIfAbsent(ctx, digest, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		current, readErr := store.cas.Read(ctx, digest)
		if readErr != nil || !bytes.Equal(current, body) {
			return errors.New("evidence authority observation conflicts with concurrently created content")
		}
	}
	written, err := store.cas.Read(ctx, digest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("evidence authority observation write readback failed")
	}
	stored, err := parseStoredObservation(digest, written)
	if err != nil || !equalObservation(stored, bundle) {
		return errors.New("evidence authority observation write semantic verification failed")
	}
	return nil
}

func (store *ObservationStore) Resolve(ctx context.Context, digest string) (storeport.ObservationBundle, error) {
	if store == nil || store.cas == nil {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation store is unavailable")
	}
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return storeport.ObservationBundle{}, err
	}
	return parseStoredObservation(digest, body)
}

func evidenceObservationBytes(bundle storeport.ObservationBundle) ([]byte, error) {
	if err := validateObservation(bundle); err != nil {
		return nil, err
	}
	bundleBody, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle.Bundle)
	if err != nil {
		return nil, err
	}
	requestBody, err := domainsecurity.MonotonicHeadObserveRequestV1Bytes(bundle.Request)
	if err != nil {
		return nil, err
	}
	observationBody, err := domainsecurity.MonotonicHeadObservationV1Bytes(bundle.Observation)
	if err != nil {
		return nil, err
	}
	return json.Marshal(evidenceObservationWireV1{
		SchemaVersion: evidenceObservationSchemaVersion,
		Purpose:       evidenceObservationPurpose,
		Bundle:        bundleBody,
		Request:       requestBody,
		Observation:   observationBody,
	})
}

func parseStoredObservation(digest string, body []byte) (storeport.ObservationBundle, error) {
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) ||
		len(body) == 0 || len(body) > maxEvidenceObservationBytes {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation CAS record is invalid")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxEvidenceObservationBytes, MaxDepth: 24, MaxTokens: 50_000, MaxStringBytes: 1 << 20,
	}); err != nil {
		return storeport.ObservationBundle{}, err
	}
	var wire evidenceObservationWireV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return storeport.ObservationBundle{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation contains trailing JSON")
	}
	if wire.SchemaVersion != evidenceObservationSchemaVersion || wire.Purpose != evidenceObservationPurpose ||
		len(wire.Bundle) == 0 || len(wire.Request) == 0 || len(wire.Observation) == 0 {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation wire is invalid")
	}
	bundle, err := domainevidence.ParseEvidenceAuthorityBundleV1(wire.Bundle)
	if err != nil {
		return storeport.ObservationBundle{}, err
	}
	request, err := domainsecurity.ParseMonotonicHeadObserveRequestV1(wire.Request)
	if err != nil {
		return storeport.ObservationBundle{}, err
	}
	observation, err := domainsecurity.ParseMonotonicHeadObservationV1(wire.Observation)
	if err != nil {
		return storeport.ObservationBundle{}, err
	}
	result := storeport.ObservationBundle{Bundle: bundle, Request: request, Observation: observation}
	if observation.ObservationDigest != digest || validateObservation(result) != nil {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation filename or witness binding is invalid")
	}
	canonical, err := evidenceObservationBytes(result)
	if err != nil || !bytes.Equal(canonical, body) {
		return storeport.ObservationBundle{}, errors.New("evidence authority observation bytes are not canonical")
	}
	return result, nil
}

func validateObservation(bundle storeport.ObservationBundle) error {
	authorityKey, authorityErr := base64.RawURLEncoding.DecodeString(bundle.Bundle.AuthorityPublicKey)
	witnessKey, witnessErr := base64.RawURLEncoding.DecodeString(bundle.Observation.Checkpoint.WitnessPublicKey)
	if authorityErr != nil || witnessErr != nil || domainevidence.ValidateEvidenceAuthorityBundleV1(bundle.Bundle) != nil ||
		domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(bundle.Bundle, bundle.Observation.Checkpoint) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			bundle.Observation, bundle.Request,
			bundle.Bundle.InstallationID, bundle.Bundle.AuthorityKeyID, authorityKey,
			bundle.Bundle.EnrollmentID, bundle.Observation.Checkpoint.WitnessKeyID, witnessKey,
		) != nil {
		return errors.New("evidence authority observation witness contract is invalid")
	}
	return nil
}

func equalObservation(left, right storeport.ObservationBundle) bool {
	leftBody, leftErr := evidenceObservationBytes(left)
	rightBody, rightErr := evidenceObservationBytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
