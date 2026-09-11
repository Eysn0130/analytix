package localdisplay

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

const (
	ResponseSchemaVersionV1     = domainlocaldisplay.ResponseSchemaVersionV1
	KindDirectSourcePreview     = domainlocaldisplay.KindDirectSourcePreviewV1
	KindAcceptedSlotDisplay     = domainlocaldisplay.KindAcceptedSlotDisplayV1
	DisplayModeFull             = domainlocaldisplay.DisplayModeFullV1
	DisplayModeMasked           = domainlocaldisplay.DisplayModeMaskedV1
	DisplayFieldAccount         = "account"
	DisplayFieldCard            = "card"
	DisplayViewTransactions     = "transactions"
	maxDirectPreviewFieldsV1    = 15
	maxDirectPreviewWorkspaceV1 = 4 * 1024
)

var (
	ErrInvalidRequest = errors.New("typed local display request is invalid")
	ErrUnavailable    = errors.New("typed local display authority is unavailable")
)

type Service struct {
	caseEntities  caseEntityResolverV1
	privateFinals PrivateFinalResolverV1
	importMapping ImportMappingPreviewDependenciesV1
	cleaningDiff  CleaningDiffPreviewDependenciesV1
	direct        DirectSourcePreviewDependenciesV1
	accepted      AcceptedSlotDisplayDependenciesV1
}

// UseCurrentLocalDisplayV1 is the existing host-owned current-case/current-
// snapshot source seam. It does not create a turn or expose a path, SQL
// handle, provider result, MCP result, evidence, or publication authority.
type UseCurrentLocalDisplayV1 func(
	context.Context,
	string,
	string,
	string,
	func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
) error

// DirectSourcePreviewDependenciesV1 is an additive capability configuration.
// NewService keeps the accepted-final path unchanged; this configuration is
// required only for the current-case typed local preview path.
type DirectSourcePreviewDependenciesV1 struct {
	Identity               identityport.Authority
	UseCurrentLocalDisplay UseCurrentLocalDisplayV1
	Runner                 nativecomponentport.DirectSourcePreviewRunner
}

type acceptedSlotEvidenceResolverV1 interface {
	Resolve(context.Context, registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error)
	ReplayAt(context.Context, domainsecurity.TurnSecurityContext, uint64) (domainevidence.EvidenceReceiptRegistry, error)
}

type UseRetainedAcceptedSlotSourceV1 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domainsecurity.TurnSecurityContext,
	[]domainevidence.AcceptedSlotSourceBindingV1,
	string,
	func(string) error,
) error

type AcceptedSlotDisplayDependenciesV1 struct {
	Identity          identityport.Authority
	Evidence          acceptedSlotEvidenceResolverV1
	UseRetainedSource UseRetainedAcceptedSlotSourceV1
}

type PrivateFinalResolverV1 interface {
	Resolve(context.Context, string) (domainevidence.PrivateAcceptedFinalRecord, error)
	ResolveDisposition(context.Context, string) (domainevidence.AcceptedFinalDispositionRecord, error)
}

type caseEntityResolverV1 interface {
	UseVerifiedBindingByReferenceV1(
		context.Context,
		caseentityapp.ResolveVerifiedBindingByReferenceInputV1,
		func(string, string) error,
	) error
}

type retainedCaseEntityResolverV1 interface {
	UseRetainedBindingByReferenceV1(
		context.Context,
		caseentityapp.UseRetainedBindingByReferenceInputV1,
		func(string, string) error,
	) error
}

type caseAcceptedDisplayBindingResolverV1 interface {
	UseCaseAcceptedDisplayBindingV1(
		context.Context,
		caseentityapp.UseCaseAcceptedDisplayBindingInputV1,
		func(domaincaseentity.CaseAcceptedDisplayBindingV1) error,
	) error
}

func NewService(caseEntities caseEntityResolverV1, privateFinals PrivateFinalResolverV1) *Service {
	return &Service{caseEntities: caseEntities, privateFinals: privateFinals}
}

// NewServiceWithDirectSourcePreview preserves NewService's existing accepted
// final behavior while adding the independent current-case typed preview
// capability. A nil or incomplete direct configuration fails closed at the
// direct route and does not affect accepted-slot display.
func NewServiceWithDirectSourcePreview(
	caseEntities caseEntityResolverV1,
	privateFinals PrivateFinalResolverV1,
	direct DirectSourcePreviewDependenciesV1,
) *Service {
	return &Service{
		caseEntities:  caseEntities,
		privateFinals: privateFinals,
		direct:        direct,
	}
}

// NewServiceWithTypedLocalDisplay composes both closed typed-local variants.
// Accepted-slot source resolution remains unavailable unless every private
// evidence and retained-source dependency is present.
func NewServiceWithTypedLocalDisplay(
	caseEntities caseEntityResolverV1,
	privateFinals PrivateFinalResolverV1,
	direct DirectSourcePreviewDependenciesV1,
	accepted AcceptedSlotDisplayDependenciesV1,
) *Service {
	return &Service{
		caseEntities: caseEntities, privateFinals: privateFinals,
		direct: direct, accepted: accepted,
	}
}

// NewServiceWithTypedLocalDataSurface composes the exact closed v1 family.
// Nil ImportMappingPreview and CleaningDiffPreview readers are the production
// fail-closed state until their host-owned producers are implemented.
func NewServiceWithTypedLocalDataSurface(
	caseEntities caseEntityResolverV1,
	privateFinals PrivateFinalResolverV1,
	importMapping ImportMappingPreviewDependenciesV1,
	cleaningDiff CleaningDiffPreviewDependenciesV1,
	direct DirectSourcePreviewDependenciesV1,
	accepted AcceptedSlotDisplayDependenciesV1,
) *Service {
	return &Service{
		caseEntities: caseEntities, privateFinals: privateFinals,
		importMapping: importMapping, cleaningDiff: cleaningDiff,
		direct: direct, accepted: accepted,
	}
}

type DirectSourcePreviewInputV1 struct {
	WorkspaceRoot string
	View          string
	Fields        []string
	RowOffset     uint32
	RowLimit      uint16
	DisplayMode   string
}

type AcceptedSlotDisplayInputV1 struct {
	ThreadID              string
	TurnID                string
	AcceptedFinalDigest   string
	DisplayMode           string
	ActiveSecurityContext domainsecurity.TurnSecurityContext
}

type CaseAcceptedSlotDisplayInputV1 struct {
	BindingDigest         string
	DisplayMode           string
	ActiveSecurityContext domainsecurity.TurnSecurityContext
}

type SlotV1 struct {
	SlotID       string   `json:"slotId"`
	Field        string   `json:"field"`
	DisplayValue string   `json:"displayValue"`
	ClaimIDs     []string `json:"claimIds"`
	ReceiptIDs   []string `json:"receiptIds"`
}

type ResponseV1 struct {
	SchemaVersion       int      `json:"schemaVersion"`
	Kind                string   `json:"kind"`
	ThreadID            string   `json:"threadId"`
	TurnID              string   `json:"turnId"`
	AcceptedFinalDigest string   `json:"acceptedFinalDigest"`
	CaseID              string   `json:"caseId"`
	DatasetSnapshotID   string   `json:"datasetSnapshotId"`
	ContextEpoch        uint64   `json:"contextEpoch"`
	DisplayMode         string   `json:"displayMode"`
	Slots               []SlotV1 `json:"slots"`
}

type DirectSourcePreviewCellV1 struct {
	Field        string `json:"field"`
	DisplayValue string `json:"displayValue"`
}

type DirectSourcePreviewRowV1 struct {
	RowIndex uint32                      `json:"rowIndex"`
	Cells    []DirectSourcePreviewCellV1 `json:"cells"`
}

// DirectSourcePreviewResponseV1 is deliberately separate from ResponseV1.
// It is a strict typed local sink and cannot carry thread, turn, entity,
// claim, receipt, query, or result identity fields.
type DirectSourcePreviewResponseV1 struct {
	SchemaVersion     int                        `json:"schemaVersion"`
	Kind              string                     `json:"kind"`
	CaseID            string                     `json:"caseId"`
	DatasetSnapshotID string                     `json:"datasetSnapshotId"`
	DisplayMode       string                     `json:"displayMode"`
	View              string                     `json:"view"`
	Fields            []string                   `json:"fields"`
	RowOffset         uint32                     `json:"rowOffset"`
	RowLimit          uint16                     `json:"rowLimit"`
	HasMore           bool                       `json:"hasMore"`
	Rows              []DirectSourcePreviewRowV1 `json:"rows"`
}

// DirectSourcePreview resolves the current case and current immutable
// snapshot through the host-owned exact-read seam. It does not read a final,
// claim, receipt, provider result, MCP result, or create an Agent turn.
func (service *Service) DirectSourcePreview(
	ctx context.Context,
	input DirectSourcePreviewInputV1,
) (DirectSourcePreviewResponseV1, error) {
	if service == nil || ctx == nil || validateDirectSourcePreviewInputV1(input) != nil {
		return DirectSourcePreviewResponseV1{}, ErrInvalidRequest
	}
	if dependencyIsNilV1(service.direct.Identity) ||
		service.direct.UseCurrentLocalDisplay == nil || dependencyIsNilV1(service.direct.Runner) {
		return DirectSourcePreviewResponseV1{}, ErrUnavailable
	}

	principal, err := service.direct.Identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		service.direct.Identity.ValidateCurrent(ctx, principal) != nil {
		return DirectSourcePreviewResponseV1{}, ErrUnavailable
	}
	nativeFields, err := directSourcePreviewFieldsV1(input.Fields)
	if err != nil {
		return DirectSourcePreviewResponseV1{}, ErrInvalidRequest
	}

	var nativeResult domainnative.DirectSourcePreviewResultV1
	var descriptor domainfundsquerysource.DescriptorV1
	var callbackCalls int
	err = service.direct.UseCurrentLocalDisplay(
		ctx,
		input.WorkspaceRoot,
		principal.TenantID,
		principal.UserID,
		func(
			sourceContext context.Context,
			currentDescriptor domainfundsquerysource.DescriptorV1,
			source fundsquerysourceport.ExactReadLease,
		) error {
			callbackCalls++
			if sourceContext == nil || dependencyIsNilV1(source) || callbackCalls != 1 {
				return errors.New("typed local display source callback is invalid")
			}
			arguments, argumentErr := domainnative.NewDirectSourcePreviewArgumentsV1(
				currentDescriptor, nativeFields, input.RowOffset, input.RowLimit,
			)
			if argumentErr != nil {
				return errors.New("typed local display source arguments are invalid")
			}
			candidate, runnerErr := service.direct.Runner.DirectSourcePreview(
				sourceContext, arguments, currentDescriptor, source,
			)
			if runnerErr != nil || validateDirectSourcePreviewResultV1(candidate, arguments, currentDescriptor) != nil {
				return errors.New("typed local display source result is unavailable")
			}
			descriptor = currentDescriptor
			nativeResult = candidate
			return nil
		},
	)
	if err != nil || callbackCalls != 1 ||
		service.direct.Identity.ValidateCurrent(ctx, principal) != nil {
		return DirectSourcePreviewResponseV1{}, ErrUnavailable
	}

	response, err := projectDirectSourcePreviewV1(input, descriptor, nativeResult)
	if err != nil {
		return DirectSourcePreviewResponseV1{}, ErrUnavailable
	}
	return response, nil
}

func validateDirectSourcePreviewInputV1(input DirectSourcePreviewInputV1) error {
	workspace := strings.TrimSpace(input.WorkspaceRoot)
	if workspace == "" || workspace != input.WorkspaceRoot ||
		len(workspace) > maxDirectPreviewWorkspaceV1 ||
		!utf8.ValidString(workspace) || strings.IndexFunc(workspace, unicode.IsControl) >= 0 ||
		input.View != DisplayViewTransactions || !validDisplayMode(input.DisplayMode) ||
		len(input.Fields) == 0 || len(input.Fields) > maxDirectPreviewFieldsV1 ||
		input.RowOffset > domainnative.DirectSourcePreviewMaximumOffsetV1 ||
		input.RowLimit == 0 || input.RowLimit > domainnative.DirectSourcePreviewMaximumRowsV1 {
		return ErrInvalidRequest
	}
	seen := make(map[string]struct{}, len(input.Fields))
	for _, field := range input.Fields {
		if _, ok := directSourcePreviewFieldV1(field); !ok {
			return ErrInvalidRequest
		}
		if _, duplicate := seen[field]; duplicate {
			return ErrInvalidRequest
		}
		seen[field] = struct{}{}
	}
	return nil
}

func directSourcePreviewFieldsV1(fields []string) ([]domainnative.DirectSourcePreviewFieldV1, error) {
	if len(fields) == 0 || len(fields) > maxDirectPreviewFieldsV1 {
		return nil, ErrInvalidRequest
	}
	result := make([]domainnative.DirectSourcePreviewFieldV1, len(fields))
	seen := make(map[domainnative.DirectSourcePreviewFieldV1]struct{}, len(fields))
	for index, field := range fields {
		nativeField, ok := directSourcePreviewFieldV1(field)
		if !ok {
			return nil, ErrInvalidRequest
		}
		if _, duplicate := seen[nativeField]; duplicate {
			return nil, ErrInvalidRequest
		}
		seen[nativeField] = struct{}{}
		result[index] = nativeField
	}
	return result, nil
}

func directSourcePreviewFieldV1(field string) (domainnative.DirectSourcePreviewFieldV1, bool) {
	value := domainnative.DirectSourcePreviewFieldV1(field)
	switch value {
	case domainnative.DirectSourcePreviewFieldTransactionTimeV1,
		domainnative.DirectSourcePreviewFieldAccountV1,
		domainnative.DirectSourcePreviewFieldCardV1,
		domainnative.DirectSourcePreviewFieldAccountNameV1,
		domainnative.DirectSourcePreviewFieldIdentityNumberV1,
		domainnative.DirectSourcePreviewFieldAmountTextV1,
		domainnative.DirectSourcePreviewFieldDirectionV1,
		domainnative.DirectSourcePreviewFieldCounterpartyAccountV1,
		domainnative.DirectSourcePreviewFieldCounterpartyNameV1,
		domainnative.DirectSourcePreviewFieldCounterpartyIdentityNumberV1,
		domainnative.DirectSourcePreviewFieldCounterpartyBankV1,
		domainnative.DirectSourcePreviewFieldSummaryV1,
		domainnative.DirectSourcePreviewFieldCurrencyV1,
		domainnative.DirectSourcePreviewFieldMerchantNameV1,
		domainnative.DirectSourcePreviewFieldRemarkV1:
		return value, true
	default:
		return "", false
	}
}

func validateDirectSourcePreviewResultV1(
	result domainnative.DirectSourcePreviewResultV1,
	arguments domainnative.DirectSourcePreviewArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
) error {
	if result.SchemaVersion != 1 || result.Purpose != domainnative.DirectSourcePreviewPurposeV1 ||
		result.DatasetSnapshotID != descriptor.DatasetSnapshotID ||
		result.RowOffset != arguments.RowOffset || result.RowLimit != arguments.RowLimit ||
		result.Currentness != domainnative.DirectSourcePreviewCurrentnessRequiredV1 ||
		!domainsecurity.IsSHA256Hex(result.QueryHash) || !domainsecurity.IsSHA256Hex(result.ResultHash) ||
		len(result.Fields) != len(arguments.Fields) || len(result.Rows) > int(arguments.RowLimit) {
		return ErrUnavailable
	}
	for index, field := range result.Fields {
		if field != arguments.Fields[index] {
			return ErrUnavailable
		}
	}
	for rowIndex, row := range result.Rows {
		if row.RowIndex > domainnative.DirectSourcePreviewMaximumOffsetV1 ||
			row.RowIndex != arguments.RowOffset+uint32(rowIndex) || len(row.Cells) != len(result.Fields) {
			return ErrUnavailable
		}
		for cellIndex, cell := range row.Cells {
			if cell.Field != result.Fields[cellIndex] {
				return ErrUnavailable
			}
		}
	}
	return nil
}

func projectDirectSourcePreviewV1(
	input DirectSourcePreviewInputV1,
	descriptor domainfundsquerysource.DescriptorV1,
	nativeResult domainnative.DirectSourcePreviewResultV1,
) (DirectSourcePreviewResponseV1, error) {
	response := DirectSourcePreviewResponseV1{
		SchemaVersion:     ResponseSchemaVersionV1,
		Kind:              KindDirectSourcePreview,
		CaseID:            descriptor.CaseID,
		DatasetSnapshotID: descriptor.DatasetSnapshotID,
		DisplayMode:       input.DisplayMode,
		View:              input.View,
		Fields:            append([]string(nil), input.Fields...),
		RowOffset:         input.RowOffset,
		RowLimit:          input.RowLimit,
		HasMore:           nativeResult.HasMore,
		Rows:              make([]DirectSourcePreviewRowV1, len(nativeResult.Rows)),
	}
	for rowIndex, row := range nativeResult.Rows {
		response.Rows[rowIndex] = DirectSourcePreviewRowV1{
			RowIndex: row.RowIndex,
			Cells:    make([]DirectSourcePreviewCellV1, len(row.Cells)),
		}
		for cellIndex, cell := range row.Cells {
			var displayValue string
			if err := cell.UseExactV1(func(exactValue string) error {
				displayValue = directSourcePreviewDisplayValueV1(
					input.DisplayMode, input.Fields[cellIndex], exactValue,
				)
				return nil
			}); err != nil {
				return DirectSourcePreviewResponseV1{}, ErrUnavailable
			}
			response.Rows[rowIndex].Cells[cellIndex] = DirectSourcePreviewCellV1{
				Field: input.Fields[cellIndex], DisplayValue: displayValue,
			}
		}
	}
	return response, nil
}

func directSourcePreviewDisplayValueV1(mode, field, exactValue string) string {
	if mode == DisplayModeFull || directSourcePreviewSafeDisplayFieldV1(field) {
		return exactValue
	}
	return localDisplayValue(exactValue, DisplayModeMasked)
}

func directSourcePreviewSafeDisplayFieldV1(field string) bool {
	switch field {
	case string(domainnative.DirectSourcePreviewFieldTransactionTimeV1),
		string(domainnative.DirectSourcePreviewFieldAmountTextV1),
		string(domainnative.DirectSourcePreviewFieldDirectionV1),
		string(domainnative.DirectSourcePreviewFieldCurrencyV1):
		return true
	default:
		return false
	}
}

func dependencyIsNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// AcceptedSlotDisplay resolves only the entity slots referenced by the exact
// committed V5 accepted final. The private final, disposition, claim records,
// receipt set, final-gate version, and current case/snapshot/epoch authority
// all remain bound before any exact value is returned.
func (service *Service) AcceptedSlotDisplay(
	ctx context.Context,
	input AcceptedSlotDisplayInputV1,
) (ResponseV1, error) {
	if service == nil || service.caseEntities == nil || service.privateFinals == nil ||
		dependencyIsNilV1(service.accepted.Identity) || dependencyIsNilV1(service.accepted.Evidence) ||
		service.accepted.UseRetainedSource == nil ||
		strings.TrimSpace(input.ThreadID) == "" || strings.TrimSpace(input.TurnID) == "" ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.AcceptedFinalDigest)) ||
		!validDisplayMode(input.DisplayMode) {
		return ResponseV1{}, ErrInvalidRequest
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.ActiveSecurityContext) != nil ||
		input.ActiveSecurityContext.ThreadID != input.ThreadID {
		return ResponseV1{}, ErrUnavailable
	}
	principal, err := service.accepted.Identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		principal.TenantID != input.ActiveSecurityContext.TenantID ||
		principal.UserID != input.ActiveSecurityContext.UserID ||
		service.accepted.Identity.ValidateCurrent(ctx, principal) != nil {
		return ResponseV1{}, ErrUnavailable
	}
	retainedCaseEntities, ok := service.caseEntities.(retainedCaseEntityResolverV1)
	if !ok || retainedCaseEntities == nil {
		return ResponseV1{}, ErrUnavailable
	}

	record, err := service.privateFinals.Resolve(ctx, input.AcceptedFinalDigest)
	if err != nil || domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil ||
		record.SchemaVersion != domainevidence.PrivateAcceptedFinalRecordVersion ||
		record.AcceptedFinal.SchemaVersion != domainevidence.AcceptedFinalRecordVersion ||
		record.AcceptedFinal.FinalGateVersion != domainevidence.FinalEvidenceGateVersion ||
		record.AcceptedFinal.RecordDigest != input.AcceptedFinalDigest ||
		record.SecurityContext.ThreadID != input.ThreadID || record.SecurityContext.TurnID != input.TurnID ||
		record.AcceptedFinal.ThreadID != input.ThreadID || record.AcceptedFinal.TurnID != input.TurnID ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil {
		return ResponseV1{}, ErrUnavailable
	}
	disposition, err := service.privateFinals.ResolveDisposition(ctx, input.AcceptedFinalDigest)
	if err != nil || domainevidence.ValidateAcceptedFinalDispositionRecord(disposition) != nil ||
		disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		disposition.State != domainevidence.AcceptedFinalCommitted ||
		disposition.AcceptedFinalDigest != record.AcceptedFinal.RecordDigest ||
		disposition.PrivateRecordDigest != record.PrivateRecordDigest ||
		disposition.ThreadID != input.ThreadID || disposition.TurnID != input.TurnID ||
		disposition.WinnerDigest != input.AcceptedFinalDigest {
		return ResponseV1{}, ErrUnavailable
	}
	registry, err := service.accepted.Evidence.ReplayAt(
		ctx, record.SecurityContext, record.RegistryHead.Sequence,
	)
	registryHead, headErr := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil || headErr != nil || registryHead != record.RegistryHead {
		return ResponseV1{}, ErrUnavailable
	}

	slots, err := resolveAcceptedEntitySlotsRetainedV1(
		ctx, retainedCaseEntities, service.accepted.Evidence,
		service.accepted.UseRetainedSource, input.ActiveSecurityContext,
		record.SecurityContext, record.Envelope, input.DisplayMode,
	)
	if err != nil || service.accepted.Identity.ValidateCurrent(ctx, principal) != nil {
		return ResponseV1{}, errors.Join(ErrUnavailable, err)
	}
	return responseForContextV1(
		KindAcceptedSlotDisplay, record.AcceptedFinal.RecordDigest,
		record.SecurityContext, input.DisplayMode, slots,
	), nil
}

// CaseAcceptedSlotDisplay is an internal host-resolved use seam. It accepts
// only the active authority plus an opaque binding digest inherited through
// the private case owner; original final/thread/turn identities are resolved
// from that owner and revalidated against the existing accepted-slot chain.
func (service *Service) CaseAcceptedSlotDisplay(
	ctx context.Context,
	input CaseAcceptedSlotDisplayInputV1,
) (ResponseV1, error) {
	if service == nil || service.caseEntities == nil || service.privateFinals == nil || ctx == nil ||
		dependencyIsNilV1(service.accepted.Identity) || dependencyIsNilV1(service.accepted.Evidence) ||
		service.accepted.UseRetainedSource == nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.BindingDigest)) ||
		input.BindingDigest != strings.TrimSpace(input.BindingDigest) ||
		!validDisplayMode(input.DisplayMode) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.ActiveSecurityContext) != nil {
		return ResponseV1{}, ErrInvalidRequest
	}
	principal, err := service.accepted.Identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		principal.TenantID != input.ActiveSecurityContext.TenantID ||
		principal.UserID != input.ActiveSecurityContext.UserID ||
		service.accepted.Identity.ValidateCurrent(ctx, principal) != nil {
		return ResponseV1{}, ErrUnavailable
	}
	longitudinal, ok := service.caseEntities.(caseAcceptedDisplayBindingResolverV1)
	retained, retainedOK := service.caseEntities.(retainedCaseEntityResolverV1)
	if !ok || longitudinal == nil || !retainedOK || retained == nil {
		return ResponseV1{}, ErrUnavailable
	}
	var response ResponseV1
	callbackCalls := 0
	err = longitudinal.UseCaseAcceptedDisplayBindingV1(
		ctx,
		caseentityapp.UseCaseAcceptedDisplayBindingInputV1{
			SecurityContext: input.ActiveSecurityContext, BindingDigest: input.BindingDigest,
		},
		func(binding domaincaseentity.CaseAcceptedDisplayBindingV1) error {
			callbackCalls++
			if callbackCalls != 1 || domaincaseentity.ValidateCaseAcceptedDisplayBindingV1(binding) != nil ||
				binding.BindingDigest != input.BindingDigest ||
				binding.CaseBindingHash != input.ActiveSecurityContext.CaseBindingHash {
				return ErrUnavailable
			}
			record, resolveErr := service.privateFinals.Resolve(ctx, binding.AcceptedFinalDigest)
			if resolveErr != nil || domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil ||
				record.SchemaVersion != domainevidence.PrivateAcceptedFinalRecordVersion ||
				record.AcceptedFinal.SchemaVersion != domainevidence.AcceptedFinalRecordVersion ||
				record.AcceptedFinal.FinalGateVersion != domainevidence.FinalEvidenceGateVersion ||
				record.AcceptedFinal.FinalGateVersion != binding.FinalGateVersion ||
				record.AcceptedFinal.RecordDigest != binding.AcceptedFinalDigest ||
				record.SecurityContext.ThreadID != binding.OriginalThreadID ||
				record.SecurityContext.TurnID != binding.OriginalTurnID ||
				record.AcceptedFinal.ThreadID != binding.OriginalThreadID ||
				record.AcceptedFinal.TurnID != binding.OriginalTurnID ||
				record.SecurityContext.CaseBindingHash != binding.CaseBindingHash ||
				record.SecurityContext.ContextDigest != binding.ContextDigest ||
				record.SecurityContext.DatasetSnapshotID != binding.DatasetSnapshotID ||
				record.SecurityContext.ContextEpoch != binding.ContextEpoch ||
				domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil {
				return ErrUnavailable
			}
			disposition, resolveErr := service.privateFinals.ResolveDisposition(ctx, binding.AcceptedFinalDigest)
			if resolveErr != nil || domainevidence.ValidateAcceptedFinalDispositionRecord(disposition) != nil ||
				disposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
				disposition.State != domainevidence.AcceptedFinalCommitted ||
				disposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
				disposition.RecordDigest != binding.DispositionDigest ||
				disposition.AcceptedFinalDigest != binding.AcceptedFinalDigest ||
				disposition.PrivateRecordDigest != record.PrivateRecordDigest ||
				disposition.ThreadID != binding.OriginalThreadID ||
				disposition.TurnID != binding.OriginalTurnID ||
				disposition.WinnerDigest != binding.AcceptedFinalDigest {
				return ErrUnavailable
			}
			registry, replayErr := service.accepted.Evidence.ReplayAt(
				ctx, record.SecurityContext, record.RegistryHead.Sequence,
			)
			registryHead, headErr := domainevidence.NewEvidenceRegistryHead(registry)
			if replayErr != nil || headErr != nil || registryHead != record.RegistryHead {
				return ErrUnavailable
			}
			slots, slotErr := domainevidence.BuildAcceptedEntitySlotBindingsV1(record.Envelope)
			if slotErr != nil {
				return ErrUnavailable
			}
			var selected *domainevidence.AcceptedEntitySlotBindingV1
			for slotIndex := range slots {
				if slots[slotIndex].SlotID != binding.SlotID {
					continue
				}
				if selected != nil {
					return ErrUnavailable
				}
				selected = &slots[slotIndex]
			}
			if selected == nil || !caseAcceptedDisplayBindingMatchesSlotV1(binding, record.Envelope, *selected) {
				return ErrUnavailable
			}
			slot, slotErr := resolveAcceptedEntitySlotRetainedV1(
				ctx, retained, service.accepted.Evidence, service.accepted.UseRetainedSource,
				input.ActiveSecurityContext, record.SecurityContext, record.Envelope,
				input.DisplayMode, *selected, &binding,
			)
			if slotErr != nil {
				return slotErr
			}
			response = responseForContextV1(
				KindAcceptedSlotDisplay, record.AcceptedFinal.RecordDigest,
				record.SecurityContext, input.DisplayMode, []SlotV1{slot},
			)
			return nil
		},
	)
	if err != nil || callbackCalls != 1 || response.SchemaVersion == 0 ||
		service.accepted.Identity.ValidateCurrent(ctx, principal) != nil {
		return ResponseV1{}, errors.Join(ErrUnavailable, err)
	}
	return response, nil
}

func resolveAcceptedEntitySlotsRetainedV1(
	ctx context.Context,
	caseEntities retainedCaseEntityResolverV1,
	evidence acceptedSlotEvidenceResolverV1,
	useRetainedSource UseRetainedAcceptedSlotSourceV1,
	activeSecurityContext domainsecurity.TurnSecurityContext,
	securityContext domainsecurity.TurnSecurityContext,
	envelope domainevidence.FinalAnswerEnvelope,
	displayMode string,
) ([]SlotV1, error) {
	if caseEntities == nil || dependencyIsNilV1(evidence) || useRetainedSource == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(activeSecurityContext) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!validDisplayMode(displayMode) {
		return nil, ErrUnavailable
	}
	bindings, err := domainevidence.BuildAcceptedEntitySlotBindingsV1(envelope)
	if err != nil || len(bindings) == 0 {
		return nil, errors.Join(ErrUnavailable, err)
	}
	slots := make([]SlotV1, 0, len(bindings))
	for _, binding := range bindings {
		slot, err := resolveAcceptedEntitySlotRetainedV1(
			ctx, caseEntities, evidence, useRetainedSource, activeSecurityContext,
			securityContext, envelope, displayMode, binding, nil,
		)
		if err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

func resolveAcceptedEntitySlotRetainedV1(
	ctx context.Context,
	caseEntities retainedCaseEntityResolverV1,
	evidence acceptedSlotEvidenceResolverV1,
	useRetainedSource UseRetainedAcceptedSlotSourceV1,
	activeSecurityContext domainsecurity.TurnSecurityContext,
	securityContext domainsecurity.TurnSecurityContext,
	envelope domainevidence.FinalAnswerEnvelope,
	displayMode string,
	binding domainevidence.AcceptedEntitySlotBindingV1,
	expected *domaincaseentity.CaseAcceptedDisplayBindingV1,
) (SlotV1, error) {
	if caseEntities == nil || dependencyIsNilV1(evidence) || useRetainedSource == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(activeSecurityContext) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!validDisplayMode(displayMode) ||
		(expected != nil && !caseAcceptedDisplayBindingMatchesSlotV1(*expected, envelope, binding)) {
		return SlotV1{}, ErrUnavailable
	}
	var displayValue string
	var displayField string
	err := binding.UseReferenceV1(func(reference domaincaseentity.ReferenceV1) error {
		sourceBindings, sourceErr := acceptedSlotSourceBindingsV1(
			ctx, evidence, securityContext, envelope, binding, reference, expected,
		)
		if sourceErr != nil {
			return sourceErr
		}
		displayField = sourceBindings[0].Field
		for _, sourceBinding := range sourceBindings[1:] {
			if sourceBinding.Field != displayField {
				return ErrUnavailable
			}
		}
		return caseEntities.UseRetainedBindingByReferenceV1(
			ctx,
			caseentityapp.UseRetainedBindingByReferenceInputV1{
				ActiveSecurityContext: activeSecurityContext, HistoricalSecurityContext: securityContext,
				Reference: reference,
			},
			func(canonicalValue string, entityBindingDigest string) error {
				if expected != nil && entityBindingDigest != expected.EntityBindingDigest {
					return ErrUnavailable
				}
				exactCallbacks := 0
				err := useRetainedSource(
					ctx, activeSecurityContext, securityContext, sourceBindings, canonicalValue,
					func(sourceExactValue string) error {
						exactCallbacks++
						if exactCallbacks != 1 || sourceExactValue == "" ||
							!utf8.ValidString(sourceExactValue) ||
							len([]byte(sourceExactValue)) > domainlocaldisplay.MaximumCellBytesV1 {
							return ErrUnavailable
						}
						displayValue = localDisplayValue(sourceExactValue, displayMode)
						return nil
					},
				)
				if err != nil || exactCallbacks != 1 {
					return errors.Join(ErrUnavailable, err)
				}
				return nil
			},
		)
	})
	if err != nil {
		return SlotV1{}, errors.Join(ErrUnavailable, err)
	}
	return SlotV1{
		SlotID: binding.SlotID, Field: displayField, DisplayValue: displayValue,
		ClaimIDs: append([]string(nil), binding.ClaimIDs...), ReceiptIDs: append([]string(nil), binding.ReceiptIDs...),
	}, nil
}

func caseAcceptedDisplayBindingMatchesSlotV1(
	expected domaincaseentity.CaseAcceptedDisplayBindingV1,
	envelope domainevidence.FinalAnswerEnvelope,
	slot domainevidence.AcceptedEntitySlotBindingV1,
) bool {
	if domaincaseentity.ValidateCaseAcceptedDisplayBindingV1(expected) != nil ||
		expected.SlotID != slot.SlotID ||
		len(expected.ClaimBindings) != len(slot.ClaimIDs) ||
		len(expected.EvidenceReceiptBindings) != len(slot.ReceiptIDs) {
		return false
	}
	claimDigests := make(map[string]string, len(envelope.Claims))
	for _, claim := range envelope.Claims {
		if _, duplicate := claimDigests[claim.ClaimID]; duplicate {
			return false
		}
		claimDigests[claim.ClaimID] = claim.RecordDigest
	}
	for _, claim := range expected.ClaimBindings {
		if claimDigests[claim.ClaimReference] != claim.ClaimDigest ||
			!containsLocalDisplayStringV1(slot.ClaimIDs, claim.ClaimReference) {
			return false
		}
	}
	for _, evidence := range expected.EvidenceReceiptBindings {
		if !containsLocalDisplayStringV1(slot.ReceiptIDs, evidence.EvidenceReference) {
			return false
		}
	}
	referenceMatches := false
	referenceCalls := 0
	if err := slot.UseReferenceV1(func(reference domaincaseentity.ReferenceV1) error {
		referenceCalls++
		referenceMatches = reference == expected.EntityReference
		return nil
	}); err != nil || referenceCalls != 1 || !referenceMatches {
		return false
	}
	return true
}

func acceptedSlotSourceBindingsV1(
	ctx context.Context,
	evidence acceptedSlotEvidenceResolverV1,
	securityContext domainsecurity.TurnSecurityContext,
	envelope domainevidence.FinalAnswerEnvelope,
	slot domainevidence.AcceptedEntitySlotBindingV1,
	reference domaincaseentity.ReferenceV1,
	expected *domaincaseentity.CaseAcceptedDisplayBindingV1,
) ([]domainevidence.AcceptedSlotSourceBindingV1, error) {
	if ctx == nil || dependencyIsNilV1(evidence) ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domaincaseentity.ValidateReferenceV1(string(reference)) != nil ||
		len(slot.ClaimIDs) == 0 || len(slot.ReceiptIDs) == 0 {
		return nil, ErrUnavailable
	}
	expectedReceiptDigests := make(map[string]string)
	if expected != nil {
		for _, binding := range expected.EvidenceReceiptBindings {
			expectedReceiptDigests[binding.EvidenceReference] = binding.EvidenceDigest
		}
		if len(expectedReceiptDigests) != len(slot.ReceiptIDs) {
			return nil, ErrUnavailable
		}
	}
	claims := make(map[string]domainevidence.ClaimRecord, len(slot.ClaimIDs))
	for _, claim := range envelope.Claims {
		if !containsLocalDisplayStringV1(slot.ClaimIDs, claim.ClaimID) ||
			!claimContainsLocalDisplayReferenceV1(claim, string(reference)) {
			continue
		}
		claims[claim.ClaimID] = claim
	}
	if len(claims) != len(slot.ClaimIDs) {
		return nil, ErrUnavailable
	}
	accountFlowGroup, accountFlowTyped, err := acceptedSlotAccountFlowGroupV1(envelope, slot, claims)
	if err != nil {
		return nil, ErrUnavailable
	}
	collected := make([]domainevidence.AcceptedSlotSourceBindingV1, 0)
	for _, receiptID := range slot.ReceiptIDs {
		registered, err := evidence.Resolve(ctx, registryport.MembershipQuery{
			Context: securityContext, ReceiptID: receiptID,
		})
		receipt := registered.Receipt
		if err != nil || registered.Revoked || receipt.ReceiptID != receiptID ||
			(expected != nil && expectedReceiptDigests[receiptID] != receipt.ReceiptDigest) ||
			domainevidence.ValidateEvidenceReceipt(receipt) != nil ||
			domainevidence.ValidateCanonicalEvidenceAgainstReceipt(receipt, registered.CanonicalEvidence) != nil ||
			receipt.ThreadID != securityContext.ThreadID || receipt.TurnID != securityContext.TurnID ||
			receipt.CaseID != securityContext.CaseID || receipt.CaseBindingHash != securityContext.CaseBindingHash ||
			receipt.ContextEpoch != securityContext.ContextEpoch || receipt.ContextDigest != securityContext.ContextDigest ||
			receipt.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
			receipt.ToolName != "mcp__analytix_funds__analyze_account_flows" ||
			(!containsLocalDisplayStringV1(receipt.QueryRange.EntityIDs, string(reference)) &&
				!containsLocalDisplayStringV1(receipt.QueryRange.AccountIDs, string(reference))) {
			return nil, ErrUnavailable
		}
		if accountFlowTyped {
			claimScope := claims[accountFlowGroup.ClaimIDs[0]].SupportedScope
			lineage := receipt.TransformationLineage
			if receiptID != accountFlowGroup.EvidenceReceiptID || claimScope == nil ||
				receipt.PaginationCompleteness != domainevidence.PaginationComplete ||
				!reflect.DeepEqual(receipt.QueryRange, *claimScope) ||
				receipt.QueryHash != accountFlowGroup.QueryHash || len(lineage) != 2 ||
				lineage[0].InputHash != receipt.RawSHA256 ||
				lineage[0].OutputHash != accountFlowGroup.ResultHash ||
				lineage[1].InputHash != accountFlowGroup.ResultHash ||
				lineage[1].OutputHash != receipt.ResultHash {
				return nil, ErrUnavailable
			}
		}
		material, err := domainevidence.ParseCanonicalEvidenceMaterial(registered.CanonicalEvidence)
		if err != nil || material.SchemaVersion != domainevidence.CanonicalEvidenceVersionV3 ||
			material.Purpose != domainevidence.CanonicalEvidencePurposeV3 {
			return nil, ErrUnavailable
		}
		matchedFacts := make(map[string]struct{})
		for _, claim := range claims {
			if !containsLocalDisplayStringV1(claim.EvidenceIDs, receiptID) {
				continue
			}
			matched := 0
			for _, fact := range material.Facts {
				if fact.ClaimType == claim.ClaimType &&
					domainevidence.ClaimPayloadExactlyMatches(fact.NormalizedPayload, claim.NormalizedPayload) {
					matchedFacts[fact.FactID] = struct{}{}
					matched++
				}
			}
			if matched != 1 {
				return nil, ErrUnavailable
			}
		}
		if len(matchedFacts) == 0 {
			return nil, ErrUnavailable
		}
		matchedBindings := 0
		for _, sourceBinding := range material.AcceptedSlotSourceBindings {
			if sourceBinding.EntityReference != string(reference) ||
				!domainevidence.IsAcceptedSlotSourceFieldV1(sourceBinding.Field) ||
				!acceptedSlotFactSetMatchesV1(sourceBinding.FactIDs, matchedFacts) {
				continue
			}
			collected = append(collected, sourceBinding)
			matchedBindings++
		}
		if matchedBindings == 0 || matchedBindings != len(receipt.SourceRecordIDs) {
			return nil, ErrUnavailable
		}
	}
	canonical, err := domainevidence.CanonicalAcceptedSlotSourceBindingsV1(collected)
	if err != nil || len(canonical) == 0 {
		return nil, ErrUnavailable
	}
	if accountFlowTyped {
		field := canonical[0].Field
		if field != accountFlowGroup.SourceFieldReference.Field ||
			domainevidence.ValidateAccountFlowTypedSourceFieldReferenceV1(
				accountFlowGroup.SourceFieldReference,
				accountFlowGroup.QueryHash,
				accountFlowGroup.ResultHash,
			) != nil {
			return nil, ErrUnavailable
		}
		for _, binding := range canonical[1:] {
			if binding.Field != field {
				return nil, ErrUnavailable
			}
		}
	}
	return canonical, nil
}

func acceptedSlotAccountFlowGroupV1(
	envelope domainevidence.FinalAnswerEnvelope,
	slot domainevidence.AcceptedEntitySlotBindingV1,
	claims map[string]domainevidence.ClaimRecord,
) (domainevidence.AccountFlowTypedAnswerGroupV1, bool, error) {
	if envelope.AccountFlowOutcome == nil {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, nil
	}
	if len(slot.ReceiptIDs) != 1 || len(slot.ClaimIDs) != 3 {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, ErrUnavailable
	}
	var matched *domainevidence.AccountFlowTypedAnswerGroupV1
	for index := range envelope.AccountFlowOutcome.Groups {
		group := &envelope.AccountFlowOutcome.Groups[index]
		if group.EvidenceReceiptID != slot.ReceiptIDs[0] {
			continue
		}
		if matched != nil || !sameLocalDisplayStringSetV1(group.ClaimIDs, slot.ClaimIDs) {
			return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, ErrUnavailable
		}
		matched = group
	}
	if matched == nil {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, ErrUnavailable
	}
	var scope *domainevidence.EvidenceQueryRange
	for _, claimID := range matched.ClaimIDs {
		claim, ok := claims[claimID]
		if !ok || len(claim.EvidenceIDs) != 1 || claim.EvidenceIDs[0] != matched.EvidenceReceiptID ||
			claim.SupportedScope == nil {
			return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, ErrUnavailable
		}
		if scope == nil {
			scope = claim.SupportedScope
		} else if !reflect.DeepEqual(*scope, *claim.SupportedScope) {
			return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, ErrUnavailable
		}
	}
	if scope == nil || len(scope.SourceIDs) != 1 ||
		!strings.HasPrefix(scope.SourceIDs[0], "qscope1_") ||
		!domainsecurity.IsSHA256Hex(strings.TrimPrefix(scope.SourceIDs[0], "qscope1_")) ||
		scope.SourceIDs[0] != matched.QueryScopeRef {
		return domainevidence.AccountFlowTypedAnswerGroupV1{}, false, ErrUnavailable
	}
	return *matched, true, nil
}

func sameLocalDisplayStringSetV1(left, right []string) bool {
	if len(left) != len(right) || len(left) == 0 {
		return false
	}
	want := make(map[string]struct{}, len(left))
	for _, value := range left {
		if value == "" {
			return false
		}
		if _, duplicate := want[value]; duplicate {
			return false
		}
		want[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := want[value]; !ok {
			return false
		}
		delete(want, value)
	}
	return len(want) == 0
}

func acceptedSlotFactSetMatchesV1(factIDs []string, expected map[string]struct{}) bool {
	if len(factIDs) != len(expected) {
		return false
	}
	for _, factID := range factIDs {
		if _, ok := expected[factID]; !ok {
			return false
		}
	}
	return true
}

func claimContainsLocalDisplayReferenceV1(claim domainevidence.ClaimRecord, reference string) bool {
	payload := claim.NormalizedPayload
	if payload.SubjectID == reference || payload.EntityID == reference ||
		payload.AccountID == reference || payload.CounterpartyID == reference {
		return true
	}
	if claim.SupportedScope == nil {
		return false
	}
	return containsLocalDisplayStringV1(claim.SupportedScope.EntityIDs, reference) ||
		containsLocalDisplayStringV1(claim.SupportedScope.AccountIDs, reference)
}

func containsLocalDisplayStringV1(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func responseForContextV1(
	kind string,
	acceptedFinalDigest string,
	securityContext domainsecurity.TurnSecurityContext,
	displayMode string,
	slots []SlotV1,
) ResponseV1 {
	return ResponseV1{
		SchemaVersion:       ResponseSchemaVersionV1,
		Kind:                kind,
		ThreadID:            securityContext.ThreadID,
		TurnID:              securityContext.TurnID,
		AcceptedFinalDigest: acceptedFinalDigest,
		CaseID:              securityContext.CaseID,
		DatasetSnapshotID:   securityContext.DatasetSnapshotID,
		ContextEpoch:        securityContext.ContextEpoch,
		DisplayMode:         displayMode,
		Slots:               append([]SlotV1(nil), slots...),
	}
}

func validDisplayMode(value string) bool {
	return value == DisplayModeFull || value == DisplayModeMasked
}

func localDisplayValue(value, mode string) string {
	if mode == DisplayModeFull {
		return value
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return "****" + string(runes[len(runes)-4:])
}
