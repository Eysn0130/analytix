package documentgeneration

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

var ErrArtifactUnavailable = errors.New("generated artifact unavailable")
var threadIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type ArtifactInventory interface {
	OperationGroups(context.Context) ([]domaincheckpoint.OperationGroupStateV2, error)
}

// Resolver reuses the signed creation checkpoint as the durable artifact index.
// Its access port validates the current thread/workspace, not the historical
// turn epoch. All return values belong only to the protected-local display lane.
type Resolver struct {
	Identity       identityport.Authority
	Inventory      ArtifactInventory
	Files          codecport.ArtifactFiles
	ValidateAccess func(context.Context, identitydomain.PrincipalV1, string, string) error
}

type Resolved struct {
	ArtifactID string `json:"artifactId"`
	ThreadID   string `json:"threadId"`
	codecport.ResolvedArtifact
	Workspace string `json:"workspace"`
	Kind      string `json:"kind"`
}

func (r Resolver) Resolve(ctx context.Context, threadID, artifactID string) (Resolved, error) {
	if ctx == nil || ctx.Err() != nil || r.Identity == nil || r.Inventory == nil || r.Files == nil || r.ValidateAccess == nil || !threadIDPattern.MatchString(threadID) || !domainsecurity.IsSHA256Hex(artifactID) {
		return Resolved{}, ErrArtifactUnavailable
	}
	principal, err := r.Identity.ResolveCurrent(ctx)
	if err != nil || identitydomain.ValidatePrincipalV1(principal) != nil || r.Identity.ValidateCurrent(ctx, principal) != nil {
		return Resolved{}, ErrArtifactUnavailable
	}
	groups, err := r.Inventory.OperationGroups(ctx)
	if err != nil {
		return Resolved{}, ErrArtifactUnavailable
	}
	var matched *domaincheckpoint.OperationGroupStateV2
	for index := range groups {
		if groups[index].Intent.OperationGroupID == artifactID {
			if matched != nil {
				return Resolved{}, ErrArtifactUnavailable
			}
			matched = &groups[index]
		}
	}
	if matched == nil || matched.Terminal == nil || matched.Terminal.Status != "completed" {
		return Resolved{}, ErrArtifactUnavailable
	}
	intent := matched.Intent
	if domaincheckpoint.ValidateOperationGroupIntentV2(intent) != nil || domaincheckpoint.ValidateOperationGroupTerminalForIntentV2(*matched.Terminal, intent) != nil || intent.ToolName != "generate_office_document" || intent.GenerationPrincipalDigest != principal.PrincipalDigest || intent.SecurityContext.ThreadID != threadID || intent.SecurityContext.TenantID != principal.TenantID || intent.SecurityContext.UserID != principal.UserID || len(intent.Paths) != 1 || intent.Paths[0].BeforeExisted {
		return Resolved{}, ErrArtifactUnavailable
	}
	workspace := intent.SecurityContext.WorkspaceRealPath
	if r.ValidateAccess(ctx, principal, threadID, workspace) != nil {
		return Resolved{}, ErrArtifactUnavailable
	}
	var args map[string]any
	if json.Unmarshal(intent.ArgumentsJSON, &args) != nil {
		return Resolved{}, ErrArtifactUnavailable
	}
	request, err := ParseRequest(args)
	if err != nil {
		return Resolved{}, ErrArtifactUnavailable
	}
	path := intent.Paths[0]
	authority := checkpointfileport.PathAuthority{SchemaVersion: path.PathAuthoritySchemaVersion, Kind: path.AuthorityKind, Root: path.AuthorityRoot, RootIdentity: path.AuthorityRootIdentity, RootHash: path.AuthorityRootHash, RelativePath: path.RelativePath}
	file, err := r.Files.InspectGenerated(ctx, workspace, authority, request.Kind, path.ExpectedAfterHash)
	if err != nil || r.Identity.ValidateCurrent(ctx, principal) != nil || r.ValidateAccess(ctx, principal, threadID, workspace) != nil {
		return Resolved{}, ErrArtifactUnavailable
	}
	return Resolved{ArtifactID: artifactID, ThreadID: threadID, ResolvedArtifact: file, Workspace: workspace, Kind: request.Kind}, nil
}
