package threadriskauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/threadriskauthority"
)

const (
	observationBundleSchemaVersion = 1
	observationBundlePurpose       = "analytix.thread-risk-authority-observation-bundle/v1"
	maxObservationBundleBytes      = 8 << 20
)

var _ storeport.ObservationStore = (*ObservationStore)(nil)

type observationBundleWireV1 struct {
	SchemaVersion int             `json:"schemaVersion"`
	Purpose       string          `json:"purpose"`
	Index         json.RawMessage `json:"index"`
	Request       json.RawMessage `json:"request"`
	Observation   json.RawMessage `json:"observation"`
}

// ObservationStore persists the exact signed index/request/observation
// triplet by ObservationDigest. A stored bundle is issuance evidence only;
// this adapter has no operation that can select a latest observation.
type ObservationStore struct {
	cas *finalauthorityadapter.SecurePrivateCAS
}

func NewObservationStore(root string, access finalauthorityadapter.SecurePrivateCASAccessAuthority) (*ObservationStore, error) {
	if root == "" || root != strings.TrimSpace(root) || access == nil {
		return nil, errors.New("thread risk authority observation root is invalid")
	}
	cas, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxObservationBundleBytes, access)
	if err != nil {
		return nil, err
	}
	return &ObservationStore{cas: cas}, nil
}

func (store *ObservationStore) PutIfAbsent(ctx context.Context, bundle storeport.ObservationBundle) error {
	if store == nil || store.cas == nil {
		return errors.New("thread risk authority observation store is unavailable")
	}
	body, err := observationBundleBytes(bundle)
	if err != nil || len(body) == 0 || len(body) > maxObservationBundleBytes {
		return errors.New("thread risk authority observation bundle is invalid")
	}
	digest := bundle.Observation.ObservationDigest
	if current, readErr := store.cas.Read(ctx, digest); readErr == nil {
		if !bytes.Equal(current, body) {
			return errors.New("thread risk authority observation conflicts with existing content address")
		}
		stored, parseErr := parseStoredObservationBundle(digest, current)
		if parseErr != nil || !equalBundle(stored, bundle) {
			return errors.New("thread risk authority existing observation verification failed")
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
			return errors.New("thread risk authority observation conflicts with concurrently created record")
		}
	}
	written, err := store.cas.Read(ctx, digest)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("thread risk authority observation write readback failed")
	}
	stored, err := parseStoredObservationBundle(digest, written)
	if err != nil || !equalBundle(stored, bundle) {
		return errors.New("thread risk authority observation write semantic verification failed")
	}
	return nil
}

func (store *ObservationStore) Resolve(ctx context.Context, digest string) (storeport.ObservationBundle, error) {
	if store == nil || store.cas == nil {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation store is unavailable")
	}
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation content address is invalid")
	}
	body, err := store.cas.Read(ctx, digest)
	if err != nil {
		return storeport.ObservationBundle{}, err
	}
	return parseStoredObservationBundle(digest, body)
}

func observationBundleBytes(bundle storeport.ObservationBundle) ([]byte, error) {
	if err := validateObservationBundle(bundle); err != nil {
		return nil, err
	}
	indexBody, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(bundle.Index)
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
	return json.Marshal(observationBundleWireV1{
		SchemaVersion: observationBundleSchemaVersion,
		Purpose:       observationBundlePurpose,
		Index:         indexBody,
		Request:       requestBody,
		Observation:   observationBody,
	})
}

func parseStoredObservationBundle(digest string, body []byte) (storeport.ObservationBundle, error) {
	if digest == "" || digest != strings.TrimSpace(digest) || !domainsecurity.IsSHA256Hex(digest) ||
		len(body) == 0 || len(body) > maxObservationBundleBytes {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation CAS record is invalid")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxObservationBundleBytes,
		MaxDepth:       20,
		MaxTokens:      300_000,
		MaxStringBytes: 4 << 20,
	}); err != nil {
		return storeport.ObservationBundle{}, err
	}
	var wire observationBundleWireV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return storeport.ObservationBundle{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation bundle contains trailing JSON")
	}
	if wire.SchemaVersion != observationBundleSchemaVersion || wire.Purpose != observationBundlePurpose ||
		len(wire.Index) == 0 || len(wire.Request) == 0 || len(wire.Observation) == 0 {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation bundle wire is invalid")
	}
	index, err := domainsecurity.ParseThreadRiskAuthorityIndexV1(wire.Index)
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
	bundle := storeport.ObservationBundle{Index: index, Request: request, Observation: observation}
	if observation.ObservationDigest != digest || validateObservationBundle(bundle) != nil {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation filename or witness binding is invalid")
	}
	canonical, err := observationBundleBytes(bundle)
	if err != nil || !bytes.Equal(canonical, body) {
		return storeport.ObservationBundle{}, errors.New("thread risk authority observation bundle bytes are not canonical")
	}
	return bundle, nil
}

func validateObservationBundle(bundle storeport.ObservationBundle) error {
	binding, err := domainsecurity.NewWitnessedRiskAuthorityBindingV1(bundle.Index, bundle.Request, bundle.Observation)
	if err != nil {
		return err
	}
	return domainsecurity.ValidateWitnessedRiskAuthorityBindingV1(binding, bundle.Index, bundle.Request, bundle.Observation)
}

func equalBundle(left, right storeport.ObservationBundle) bool {
	leftBody, leftErr := observationBundleBytes(left)
	rightBody, rightErr := observationBundleBytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
