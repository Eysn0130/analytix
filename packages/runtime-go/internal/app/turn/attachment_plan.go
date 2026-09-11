package turn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"analytix.local/runtime-go/internal/contracts"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	attachmentauthorityport "analytix.local/runtime-go/internal/ports/attachmentauthority"
)

const (
	attachmentPlanVersionV1           = "analytix.attachment-plan/v1"
	attachmentProjectionRecipeVersion = "analytix.attachment-projection-recipe/v1"
	AttachmentRouteImageDataV1        = "image_data_base64_v1"
	AttachmentRouteTextFallbackV1     = "text_fallback_v1"
)

var (
	ErrAttachmentRiskAuthorityUnavailable = errors.New("turn attachment risk authority is unavailable")
	ErrAttachmentRiskMetadataMissing      = errors.New("turn attachment risk metadata is missing")
	ErrAttachmentRiskMetadataInvalid      = errors.New("turn attachment risk metadata is invalid")
	ErrAttachmentRiskOwnerMismatch        = errors.New("turn attachment risk private owner is invalid")
)

// AttachmentPlanStore separates immutable metadata admission from private
// content materialization. Plan must use Metadata only; Content is reserved
// for an open attachment-use receipt held inside an effect lease.
type AttachmentPlanStore interface {
	Metadata(id string) (map[string]any, bool, error)
	Content(id string) (map[string]any, string, bool, error)
}

type AttachmentPlanItemV1 struct {
	AttachmentID       string
	Name               string
	MIMEType           string
	Route              string
	Owner              domainattachment.OwnerRecordV1
	ProjectionSHA256   string
	ProjectionByteSize int64
}

// AttachmentPlan is safe to retain across provider steps and pending gates:
// it contains no attachment bytes, extracted text, fallback content, local
// path, or provider MessagePart. Owner records remain private process state and
// are anchored to the private attachment authority before the plan is issued.
type AttachmentPlan struct {
	IDs                  []string
	Metadata             []map[string]any
	Items                []AttachmentPlanItemV1
	ModelInputModalities []string
	ModelMessageParts    []string
	PlanDigest           string
}

type AttachmentPlanInput struct {
	IDs                  []string
	SecurityContext      domainsecurity.TurnSecurityContext
	ModelInputModalities []string
	ModelMessageParts    []string
}

type AttachmentMaterializeInput struct {
	Plan            AttachmentPlan
	SecurityContext domainsecurity.TurnSecurityContext
}

type AttachmentPlanner struct {
	Store  AttachmentPlanStore
	Owners attachmentauthorityport.Store
}

// PreflightCaseRisk classifies only exact private-authority-backed attachment
// owners. A host-owned case signal short-circuits before metadata access so a
// pure case boundary never resolves attachment material; otherwise every
// metadata or owner uncertainty fails closed before turn admission.
func (planner AttachmentPlanner) PreflightCaseRisk(
	ctx context.Context,
	ids []string,
	caseRiskAlreadyKnown bool,
) (bool, error) {
	if caseRiskAlreadyKnown {
		return false, nil
	}
	ids = normalizedAttachmentIDs(ids)
	if len(ids) == 0 {
		return false, nil
	}
	if planner.Store == nil || planner.Owners == nil {
		return false, ErrAttachmentRiskAuthorityUnavailable
	}
	caseBound := false
	for _, id := range ids {
		metadata, found, err := planner.Store.Metadata(id)
		if err != nil {
			return false, fmt.Errorf("%w: %s", ErrAttachmentRiskMetadataInvalid, id)
		}
		if !found {
			return false, fmt.Errorf("%w: %s", ErrAttachmentRiskMetadataMissing, id)
		}
		owner, err := AttachmentOwnerAuthorityRecord(metadata)
		if err != nil {
			return false, fmt.Errorf("%w: %s", ErrAttachmentRiskMetadataInvalid, id)
		}
		stored, err := planner.Owners.ResolveOwner(ctx, owner.OwnerDigest)
		if err != nil || !sameAttachmentOwnerRecord(owner, stored) {
			return false, fmt.Errorf("%w: %s", ErrAttachmentRiskOwnerMismatch, id)
		}
		caseBound = caseBound || owner.CaseBindingObservation != nil
	}
	return caseBound, nil
}

func (planner AttachmentPlanner) Plan(ctx context.Context, input AttachmentPlanInput) (AttachmentPlan, error) {
	plan := AttachmentPlan{
		IDs:                  []string{},
		Metadata:             []map[string]any{},
		Items:                []AttachmentPlanItemV1{},
		ModelInputModalities: normalizedAttachmentModalities(input.ModelInputModalities),
		ModelMessageParts:    normalizedAttachmentMessageParts(input.ModelMessageParts),
	}
	ids := normalizedAttachmentIDs(input.IDs)
	if len(ids) == 0 {
		return plan, nil
	}
	if planner.Store == nil || planner.Owners == nil {
		return AttachmentPlan{}, errors.New("attachment plan authority is unavailable")
	}
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(input.SecurityContext) != nil {
		return AttachmentPlan{}, ErrAttachmentNotAuthorized
	}
	supportsImageInput := attachmentStringSliceContains(plan.ModelInputModalities, "image") &&
		attachmentModelSupportsImageParts(plan.ModelMessageParts)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return AttachmentPlan{}, err
		}
		metadata, ok, err := planner.Store.Metadata(id)
		if err != nil {
			return AttachmentPlan{}, err
		}
		if !ok {
			return AttachmentPlan{}, fmt.Errorf("%w: %s", ErrAttachmentUnavailable, id)
		}
		if !AttachmentMetadataAuthorizedForContext(metadata, input.SecurityContext) {
			return AttachmentPlan{}, fmt.Errorf("%w: %s", ErrAttachmentNotAuthorized, id)
		}
		owner, err := AttachmentOwnerAuthorityRecord(metadata)
		if err != nil {
			return AttachmentPlan{}, fmt.Errorf("%w: %s", ErrAttachmentNotAuthorized, id)
		}
		stored, err := planner.Owners.ResolveOwner(ctx, owner.OwnerDigest)
		if err != nil || !sameAttachmentOwnerRecord(stored, owner) {
			return AttachmentPlan{}, fmt.Errorf("%w: private owner %s", ErrAttachmentNotAuthorized, id)
		}
		mimeType := strings.TrimSpace(stringField(metadata, "mimeType"))
		route := AttachmentRouteTextFallbackV1
		if strings.HasPrefix(strings.ToLower(mimeType), "image/") && supportsImageInput {
			route = AttachmentRouteImageDataV1
		}
		item := AttachmentPlanItemV1{
			AttachmentID: id,
			Name:         strings.TrimSpace(stringField(metadata, "name")),
			MIMEType:     mimeType,
			Route:        route,
			Owner:        owner,
		}
		item.ProjectionSHA256 = attachmentProjectionRecipeDigest(input.SecurityContext, item, plan.ModelInputModalities, plan.ModelMessageParts)
		item.ProjectionByteSize = owner.ByteSize
		plan.IDs = append(plan.IDs, id)
		plan.Metadata = append(plan.Metadata, AttachmentPublicMetadata(metadata))
		plan.Items = append(plan.Items, item)
	}
	plan.PlanDigest = attachmentPlanDigest(input.SecurityContext, plan)
	if err := ValidateAttachmentPlanForContext(plan, input.SecurityContext); err != nil {
		return AttachmentPlan{}, err
	}
	return plan, nil
}

func (planner AttachmentPlanner) Materialize(ctx context.Context, input AttachmentMaterializeInput) (ResolvedAttachments, error) {
	if planner.Store == nil || planner.Owners == nil || len(input.Plan.Items) == 0 {
		if len(input.Plan.Items) == 0 && len(input.Plan.IDs) == 0 {
			return ResolvedAttachments{
				IDs: []string{}, Metadata: []map[string]any{}, MessageParts: []domainmodel.MessagePart{},
				ModelInputModalities: append([]string(nil), input.Plan.ModelInputModalities...),
				ModelMessageParts:    append([]string(nil), input.Plan.ModelMessageParts...),
			}, nil
		}
		return ResolvedAttachments{}, errors.New("attachment materialization authority is unavailable")
	}
	if err := ValidateAttachmentPlanForContext(input.Plan, input.SecurityContext); err != nil {
		return ResolvedAttachments{}, err
	}
	resolved := ResolvedAttachments{
		IDs: append([]string(nil), input.Plan.IDs...), Metadata: cloneAttachmentPublicMetadata(input.Plan.Metadata),
		MessageParts: []domainmodel.MessagePart{}, Parts: []ResolvedAttachmentPart{}, ImageCandidates: []ResolvedAttachmentImage{},
		MissingIDs: []string{}, ModelInputModalities: append([]string(nil), input.Plan.ModelInputModalities...),
		ModelMessageParts: append([]string(nil), input.Plan.ModelMessageParts...),
	}
	for index, item := range input.Plan.Items {
		if err := ctx.Err(); err != nil {
			return ResolvedAttachments{}, err
		}
		stored, err := planner.Owners.ResolveOwner(ctx, item.Owner.OwnerDigest)
		if err != nil || !sameAttachmentOwnerRecord(stored, item.Owner) {
			return ResolvedAttachments{}, fmt.Errorf("%w: private owner %s", ErrAttachmentNotAuthorized, item.AttachmentID)
		}
		metadata, dataBase64, ok, err := planner.Store.Content(item.AttachmentID)
		if err != nil {
			return ResolvedAttachments{}, err
		}
		if !ok || !AttachmentMetadataAuthorizedForContext(metadata, input.SecurityContext) {
			return ResolvedAttachments{}, fmt.Errorf("%w: %s", ErrAttachmentNotAuthorized, item.AttachmentID)
		}
		owner, err := AttachmentOwnerAuthorityRecord(metadata)
		if err != nil || !sameAttachmentOwnerRecord(owner, item.Owner) {
			return ResolvedAttachments{}, fmt.Errorf("%w: owner changed for %s", ErrAttachmentNotAuthorized, item.AttachmentID)
		}
		expected := item
		expected.ProjectionSHA256 = attachmentProjectionRecipeDigest(input.SecurityContext, expected, input.Plan.ModelInputModalities, input.Plan.ModelMessageParts)
		if expected.ProjectionSHA256 != item.ProjectionSHA256 || item.ProjectionByteSize != owner.ByteSize ||
			index >= len(input.Plan.IDs) || input.Plan.IDs[index] != item.AttachmentID {
			return ResolvedAttachments{}, fmt.Errorf("%w: projection changed for %s", ErrAttachmentNotAuthorized, item.AttachmentID)
		}
		isImage := strings.HasPrefix(strings.ToLower(item.MIMEType), "image/")
		if isImage {
			resolved.ImageCandidates = append(resolved.ImageCandidates, ResolvedAttachmentImage{
				ID: item.AttachmentID, Name: item.Name, MIMEType: item.MIMEType, DataBase64: dataBase64,
				LocalPath: attachmentVirtualPath(metadata), Base64Bytes: base64DecodedByteLen(dataBase64),
			})
		}
		switch item.Route {
		case AttachmentRouteImageDataV1:
			if !isImage {
				return ResolvedAttachments{}, fmt.Errorf("%w: image route mismatch for %s", ErrAttachmentNotAuthorized, item.AttachmentID)
			}
			part := domainmodel.MessagePart{Type: "image", MediaType: item.MIMEType, Data: dataBase64}
			resolved.MessageParts = append(resolved.MessageParts, part)
			resolved.Parts = append(resolved.Parts, ResolvedAttachmentPart{
				ID: item.AttachmentID, MIMEType: item.MIMEType, Source: "image", MessagePart: part,
			})
			resolved.ImageCount++
			resolved.ImageMIMETypes = append(resolved.ImageMIMETypes, item.MIMEType)
			resolved.ImageAttachmentBase64Bytes += base64DecodedByteLen(dataBase64)
			resolved.RawImageBytesSentToPrimary = true
		case AttachmentRouteTextFallbackV1:
			fallbackText, fallbackBase64Bytes, err := attachmentTextFallback(metadata, dataBase64)
			if err != nil {
				return ResolvedAttachments{}, err
			}
			part := domainmodel.MessagePart{Type: "text", Text: fallbackText}
			resolved.MessageParts = append(resolved.MessageParts, part)
			resolved.Parts = append(resolved.Parts, ResolvedAttachmentPart{
				ID: item.AttachmentID, MIMEType: item.MIMEType, Source: "text_fallback",
				IsImageFallback: isImage, TextFallbackBase64Bytes: fallbackBase64Bytes, MessagePart: part,
			})
			resolved.TextFallbackCount++
			resolved.TextMIMETypes = append(resolved.TextMIMETypes, item.MIMEType)
			resolved.TextFallbackBase64Bytes += fallbackBase64Bytes
		default:
			return ResolvedAttachments{}, fmt.Errorf("%w: unknown route for %s", ErrAttachmentNotAuthorized, item.AttachmentID)
		}
	}
	return resolved, nil
}

func ValidateAttachmentPlanForContext(plan AttachmentPlan, securityContext domainsecurity.TurnSecurityContext) error {
	if domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil ||
		len(plan.IDs) == 0 || len(plan.IDs) != len(plan.Items) || len(plan.IDs) != len(plan.Metadata) ||
		!domainsecurity.IsSHA256Hex(plan.PlanDigest) || plan.PlanDigest != attachmentPlanDigest(securityContext, plan) {
		return ErrAttachmentNotAuthorized
	}
	seen := map[string]bool{}
	for index, item := range plan.Items {
		if item.AttachmentID == "" || plan.IDs[index] != item.AttachmentID || seen[item.AttachmentID] ||
			domainattachment.ValidateOwnerRecordV1(item.Owner) != nil || item.Owner.AttachmentID != item.AttachmentID ||
			!attachmentOwnerAuthorizedForContext(item.Owner, securityContext) ||
			item.MIMEType != item.Owner.MIMEType || (item.Route != AttachmentRouteImageDataV1 && item.Route != AttachmentRouteTextFallbackV1) ||
			item.ProjectionByteSize != item.Owner.ByteSize || !domainsecurity.IsSHA256Hex(item.ProjectionSHA256) ||
			item.ProjectionSHA256 != attachmentProjectionRecipeDigest(securityContext, item, plan.ModelInputModalities, plan.ModelMessageParts) {
			return ErrAttachmentNotAuthorized
		}
		seen[item.AttachmentID] = true
	}
	return nil
}

func (plan AttachmentPlan) ProjectionSet() []domainattachment.AttachmentUseProjectionV1 {
	projections := make([]domainattachment.AttachmentUseProjectionV1, len(plan.Items))
	for index, item := range plan.Items {
		projections[index] = domainattachment.AttachmentUseProjectionV1{
			AttachmentID: item.AttachmentID, ProjectionSHA256: item.ProjectionSHA256,
			ProjectionByteSize: item.ProjectionByteSize,
		}
	}
	return projections
}

func (plan AttachmentPlan) Owners() []domainattachment.OwnerRecordV1 {
	owners := make([]domainattachment.OwnerRecordV1, len(plan.Items))
	for index, item := range plan.Items {
		owners[index] = item.Owner
	}
	return owners
}

// UsesCaseDataAuthority classifies only the provider effect required to
// consume this plan. Funds-tool advertisement remains governed by the
// independent per-turn funds policy.
func (plan AttachmentPlan) UsesCaseDataAuthority() bool {
	for _, item := range plan.Items {
		if item.Owner.CaseBindingObservation != nil {
			return true
		}
	}
	return false
}

func (plan AttachmentPlan) Empty() bool { return len(plan.IDs) == 0 }

func (plan AttachmentPlan) PipelineDetails() map[string]any {
	imageCount := 0
	textFallbackCount := 0
	imageMIMEs := []string{}
	textMIMEs := []string{}
	for _, item := range plan.Items {
		if item.Route == AttachmentRouteImageDataV1 {
			imageCount++
			imageMIMEs = append(imageMIMEs, item.MIMEType)
		} else {
			textFallbackCount++
			textMIMEs = append(textMIMEs, item.MIMEType)
		}
	}
	return map[string]any{
		"attachmentIds":               attachmentStringListAny(plan.IDs),
		"modelInputModalities":        attachmentStringListAny(plan.ModelInputModalities),
		"modelMessageParts":           attachmentStringListAny(plan.ModelMessageParts),
		"imageAttachmentCount":        float64(imageCount),
		"imageAttachmentMimeTypes":    attachmentStringListAny(imageMIMEs),
		"textFallbackCount":           float64(textFallbackCount),
		"textFallbackMimeTypes":       attachmentStringListAny(textMIMEs),
		"providerPayloadParts":        float64(len(plan.Items)),
		"materializationDeferred":     len(plan.Items) > 0,
		"privateAttachmentPlanDigest": strings.TrimSpace(plan.PlanDigest),
	}
}

func attachmentProjectionRecipeDigest(
	securityContext domainsecurity.TurnSecurityContext,
	item AttachmentPlanItemV1,
	modalities []string,
	messageParts []string,
) string {
	value := struct {
		Version            string   `json:"version"`
		ContextDigest      string   `json:"contextDigest"`
		AttachmentID       string   `json:"attachmentId"`
		OwnerDigest        string   `json:"ownerDigest"`
		BlobSHA256         string   `json:"blobSHA256"`
		BlobByteSize       int64    `json:"blobByteSize"`
		MetadataProjection string   `json:"metadataProjectionSHA256"`
		MIMEType           string   `json:"mimeType"`
		Route              string   `json:"route"`
		ModelModalities    []string `json:"modelModalities"`
		ModelMessageParts  []string `json:"modelMessageParts"`
	}{
		Version: attachmentProjectionRecipeVersion, ContextDigest: securityContext.ContextDigest,
		AttachmentID: item.AttachmentID, OwnerDigest: item.Owner.OwnerDigest, BlobSHA256: item.Owner.BlobSHA256,
		BlobByteSize: item.Owner.ByteSize, MetadataProjection: item.Owner.ProjectionSHA256,
		MIMEType: item.MIMEType, Route: item.Route,
		ModelModalities: append([]string(nil), modalities...), ModelMessageParts: append([]string(nil), messageParts...),
	}
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(body)
}

func attachmentPlanDigest(securityContext domainsecurity.TurnSecurityContext, plan AttachmentPlan) string {
	type digestItem struct {
		AttachmentID       string `json:"attachmentId"`
		OwnerDigest        string `json:"ownerDigest"`
		ProjectionSHA256   string `json:"projectionSHA256"`
		ProjectionByteSize int64  `json:"projectionByteSize"`
		Route              string `json:"route"`
	}
	items := make([]digestItem, len(plan.Items))
	for index, item := range plan.Items {
		items[index] = digestItem{
			AttachmentID: item.AttachmentID, OwnerDigest: item.Owner.OwnerDigest,
			ProjectionSHA256: item.ProjectionSHA256, ProjectionByteSize: item.ProjectionByteSize, Route: item.Route,
		}
	}
	value := struct {
		Version           string       `json:"version"`
		ContextDigest     string       `json:"contextDigest"`
		IDs               []string     `json:"ids"`
		Items             []digestItem `json:"items"`
		ModelModalities   []string     `json:"modelModalities"`
		ModelMessageParts []string     `json:"modelMessageParts"`
	}{
		Version: attachmentPlanVersionV1, ContextDigest: securityContext.ContextDigest,
		IDs: append([]string(nil), plan.IDs...), Items: items,
		ModelModalities:   append([]string(nil), plan.ModelInputModalities...),
		ModelMessageParts: append([]string(nil), plan.ModelMessageParts...),
	}
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(body)
}

func normalizedAttachmentIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func sameAttachmentOwnerRecord(left, right domainattachment.OwnerRecordV1) bool {
	leftBody, leftErr := domainattachment.OwnerRecordV1Bytes(left)
	rightBody, rightErr := domainattachment.OwnerRecordV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func cloneAttachmentPublicMetadata(values []map[string]any) []map[string]any {
	out := make([]map[string]any, len(values))
	for index, value := range values {
		out[index] = contracts.CloneMap(value)
	}
	return out
}
