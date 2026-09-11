package authoritymanifestfs

import (
	"bytes"
	"context"
	"errors"

	secureconfigfs "analytix.local/runtime-go/internal/adapters/outbound/secureconfigfs"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
	authoritymanifestport "analytix.local/runtime-go/internal/ports/authoritymanifest"
)

const DefaultMaxManifestBytes int64 = 256 << 10

type Reader struct {
	Root     string
	Name     string
	MaxBytes int64
	Anchor   authorityanchorport.Source
}

var (
	_ authoritymanifestport.Loader            = Reader{}
	_ authoritymanifestport.MigrationV1Loader = Reader{}
)

type secureExactReaderV1 func(secureconfigfs.ReadExactInput) ([]byte, error)

func (reader Reader) LoadAnchoredV2(ctx context.Context) (domainenrollment.AnchoredManifestV2, error) {
	return reader.loadAnchoredV2With(ctx, secureconfigfs.ReadExact)
}

func (reader Reader) loadAnchoredV2With(
	ctx context.Context,
	readExact secureExactReaderV1,
) (domainenrollment.AnchoredManifestV2, error) {
	return loadWithStableAnchor(ctx, reader, readExact, func(body []byte, anchor authorityanchorport.AnchorV1) (domainenrollment.AnchoredManifestV2, error) {
		return domainenrollment.ParseAnchoredManifestV2(
			body, anchor.InstallationID, anchor.AuthorityKeyID, anchor.AuthorityPublicKey, anchor.CurrentManifestDigest,
		)
	})
}

func (reader Reader) LoadAnchoredV1ForMigration(ctx context.Context) (domainenrollment.AnchoredManifestV1, error) {
	return reader.loadAnchoredV1ForMigrationWith(ctx, secureconfigfs.ReadExact)
}

func (reader Reader) loadAnchoredV1ForMigrationWith(
	ctx context.Context,
	readExact secureExactReaderV1,
) (domainenrollment.AnchoredManifestV1, error) {
	return loadWithStableAnchor(ctx, reader, readExact, func(body []byte, anchor authorityanchorport.AnchorV1) (domainenrollment.AnchoredManifestV1, error) {
		return domainenrollment.ParseAnchoredManifestV1(
			body, anchor.InstallationID, anchor.AuthorityKeyID, anchor.AuthorityPublicKey, anchor.CurrentManifestDigest,
		)
	})
}

func loadWithStableAnchor[T any](
	ctx context.Context,
	reader Reader,
	readExact secureExactReaderV1,
	parse func([]byte, authorityanchorport.AnchorV1) (T, error),
) (T, error) {
	var zero T
	if ctx == nil || reader.Anchor == nil || readExact == nil {
		return zero, errors.New("authority manifest loader is unavailable")
	}
	before, err := reader.Anchor.Load(ctx)
	if err != nil {
		return zero, err
	}
	maxBytes := reader.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxManifestBytes
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	body, err := readExact(secureconfigfs.ReadExactInput{
		Root: reader.Root, Target: reader.Name, AllowedNames: []string{reader.Name}, MaxBytes: maxBytes,
	})
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	anchored, err := parse(body, before)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	after, err := reader.Anchor.Load(ctx)
	if err != nil {
		return zero, err
	}
	if !sameAnchor(before, after) {
		return zero, errors.New("independent authority anchor changed during manifest read")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return anchored, nil
}

func sameAnchor(left, right authorityanchorport.AnchorV1) bool {
	return left.InstallationID == right.InstallationID && left.AuthorityKeyID == right.AuthorityKeyID &&
		left.CurrentManifestDigest == right.CurrentManifestDigest && bytes.Equal(left.AuthorityPublicKey, right.AuthorityPublicKey)
}
