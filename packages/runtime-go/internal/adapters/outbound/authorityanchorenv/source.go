package authorityanchorenv

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
)

const AnchorEnvelopeV1Variable = "ANALYTIX_AUTHORITY_ANCHOR_V1"

const maxEnvelopeBytes = 8 << 10

var (
	ErrNotConfigured = errors.New("protected authority anchor is not configured")
	ErrInvalid       = errors.New("protected authority anchor is invalid")
)

type LookupEnv func(string) (string, bool)

type Source struct {
	Lookup LookupEnv
}

type envelopeV1 struct {
	SchemaVersion         int    `json:"schemaVersion"`
	InstallationID        string `json:"installationId"`
	AuthorityKeyID        string `json:"authorityKeyId"`
	AuthorityPublicKey    string `json:"authorityPublicKey"`
	CurrentManifestDigest string `json:"currentManifestDigest"`
}

var _ authorityanchorport.Source = Source{}

func NewProcessSource() Source {
	return Source{Lookup: os.LookupEnv}
}

func (source Source) Load(ctx context.Context) (authorityanchorport.AnchorV1, error) {
	if ctx == nil {
		return authorityanchorport.AnchorV1{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return authorityanchorport.AnchorV1{}, err
	}
	if source.Lookup == nil {
		return authorityanchorport.AnchorV1{}, ErrNotConfigured
	}
	raw, ok := source.Lookup(AnchorEnvelopeV1Variable)
	if err := ctx.Err(); err != nil {
		return authorityanchorport.AnchorV1{}, err
	}
	if !ok {
		return authorityanchorport.AnchorV1{}, ErrNotConfigured
	}
	if raw == "" || len(raw) > maxEnvelopeBytes || raw != strings.TrimSpace(raw) {
		return authorityanchorport.AnchorV1{}, ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope envelopeV1
	if err := decoder.Decode(&envelope); err != nil {
		return authorityanchorport.AnchorV1{}, ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return authorityanchorport.AnchorV1{}, ErrInvalid
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || string(canonical) != raw || envelope.SchemaVersion != 1 ||
		envelope.InstallationID != strings.TrimSpace(envelope.InstallationID) ||
		envelope.AuthorityKeyID != strings.TrimSpace(envelope.AuthorityKeyID) ||
		envelope.CurrentManifestDigest != strings.TrimSpace(envelope.CurrentManifestDigest) ||
		!domainsecurity.IsSHA256Hex(envelope.InstallationID) ||
		!domainsecurity.IsSHA256Hex(envelope.AuthorityKeyID) ||
		!domainsecurity.IsSHA256Hex(envelope.CurrentManifestDigest) {
		return authorityanchorport.AnchorV1{}, ErrInvalid
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(envelope.AuthorityPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != envelope.AuthorityPublicKey ||
		envelope.AuthorityKeyID != domainsecurity.SHA256Hex(publicKey) {
		return authorityanchorport.AnchorV1{}, ErrInvalid
	}
	return authorityanchorport.AnchorV1{
		InstallationID: envelope.InstallationID, AuthorityKeyID: envelope.AuthorityKeyID,
		AuthorityPublicKey: append([]byte(nil), publicKey...), CurrentManifestDigest: envelope.CurrentManifestDigest,
	}, nil
}
