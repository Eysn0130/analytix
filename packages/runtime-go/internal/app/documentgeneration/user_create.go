package documentgeneration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	editingport "analytix.local/runtime-go/internal/ports/objectediting"
)

var userCreateThread = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var userCreateOperation = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)

type UserWorkspaceResolver interface {
	// The first path is the filesystem identity; the second is trusted thread spelling.
	ResolveDocumentWorkspace(context.Context, identitydomain.PrincipalV1, string) (string, string, error)
}
type UserCreator struct {
	identity identityport.Authority
	files    codecport.UserCreationFiles
	resolver UserWorkspaceResolver
	codec    codecport.Codec
	acquire  func(context.Context) (func(), error)
}

func NewUserCreator(identity identityport.Authority, files codecport.UserCreationFiles) *UserCreator {
	return &UserCreator{identity: identity, files: files}
}
func (c *UserCreator) BindHost(resolver UserWorkspaceResolver, codec codecport.Codec, acquire func(context.Context) (func(), error)) error {
	if c == nil || c.resolver != nil || resolver == nil || acquire == nil {
		return editingport.ErrInvalidInput
	}
	c.resolver, c.codec, c.acquire = resolver, codec, acquire
	return nil
}
func (c *UserCreator) Ready() bool {
	return c != nil && c.identity != nil && c.files != nil && c.resolver != nil && c.acquire != nil && codecport.Supports(c.codec, "docx")
}
func userCreateDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
func (c *UserCreator) Create(ctx context.Context, caller identitydomain.PrincipalV1, thread, operation, name string) (codecport.UserCreation, error) {
	if !c.Ready() || ctx == nil || ctx.Err() != nil {
		return codecport.UserCreation{}, editingport.ErrPersistence
	}
	if !userCreateThread.MatchString(thread) || !userCreateOperation.MatchString(operation) || !codecport.ValidUserDOCXName(name) {
		return codecport.UserCreation{}, editingport.ErrInvalidInput
	}
	principal, err := c.identity.ResolveCurrent(ctx)
	if err != nil || !identitydomain.SamePrincipalV1(principal, caller) {
		return codecport.UserCreation{}, editingport.ErrForbidden
	}
	// Creation has no existing inode to capture. Hold the ordinary workspace
	// mutation coordinator through the absent-only write instead.
	release, err := c.acquire(ctx)
	if err != nil {
		return codecport.UserCreation{}, err
	}
	defer release()
	workspace, displayWorkspace, err := c.resolver.ResolveDocumentWorkspace(ctx, principal, thread)
	if err != nil {
		return codecport.UserCreation{}, editingport.ErrForbidden
	}
	check := func() error {
		current, currentDisplay, err := c.resolver.ResolveDocumentWorkspace(ctx, principal, thread)
		if err != nil || current != workspace || c.identity.ValidateCurrent(ctx, principal) != nil || ctx.Err() != nil {
			return editingport.ErrForbidden
		}
		displayWorkspace = currentDisplay
		return nil
	}
	subject := userCreateDigest("user-blank-docx/v1\x00" + principal.PrincipalDigest + "\x00" + thread)
	request := userCreateDigest("user-blank-docx/v1\x00" + workspace + "\x00" + name)
	result, err := c.files.CreateUserDocument(ctx, codecport.UserCreateInput{Workspace: workspace, FileName: name, SubjectIdentity: subject, OperationID: operation, RequestHash: request, Encode: func(ctx context.Context) ([]byte, error) {
		return c.codec.Encode(ctx, codecport.Input{SchemaVersion: 1, Kind: "docx", Markdown: ""})
	}, CheckCurrent: check})
	if check() != nil {
		return codecport.UserCreation{}, editingport.ErrForbidden
	}
	if err != nil {
		return result, err
	}
	// Keep journal/request identity and actual writes unchanged. Only project a
	// successful receipt already bound to this exact canonical creation input.
	if result.Workspace != workspace || result.Path != filepath.Join(workspace, name) || result.OperationID != operation {
		return codecport.UserCreation{}, editingport.ErrPersistence
	}
	result.Workspace, result.Path = displayWorkspace, filepath.Join(displayWorkspace, name)
	return result, nil
}
