package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"unicode"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

var ErrInternalEntityReferenceRendering = errors.New("internal case entity reference cannot be rendered in a public final answer")

const accountFlowAggregateGranularity = "aggregate"

// AcceptedEntitySlotBindingV1 is a host-private bridge between the stable
// case reference retained by verified claims and the deterministic slot shown
// in ordinary answer text. The reference can only be used through the
// synchronous callback and cannot enter an ordinary JSON or formatting sink.
type AcceptedEntitySlotBindingV1 struct {
	SlotID     string
	ClaimIDs   []string
	ReceiptIDs []string
	reference  domaincaseentity.ReferenceV1
}

func (AcceptedEntitySlotBindingV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("accepted entity slot binding does not support ordinary JSON serialization")
}

func (*AcceptedEntitySlotBindingV1) UnmarshalJSON([]byte) error {
	return errors.New("accepted entity slot binding does not support ordinary JSON deserialization")
}

func (AcceptedEntitySlotBindingV1) String() string {
	return "AcceptedEntitySlotBindingV1{private:[REDACTED]}"
}

func (binding AcceptedEntitySlotBindingV1) GoString() string {
	return binding.String()
}

func (binding AcceptedEntitySlotBindingV1) UseReferenceV1(use func(domaincaseentity.ReferenceV1) error) error {
	if use == nil || strings.TrimSpace(binding.SlotID) == "" ||
		domaincaseentity.ValidateReferenceV1(string(binding.reference)) != nil {
		return errors.New("accepted entity slot binding is invalid")
	}
	return use(binding.reference)
}

func RenderFinalAnswer(envelope FinalAnswerEnvelope) (string, error) {
	return RenderFinalAnswerAtVersion(envelope, FinalAnswerRendererVersion)
}

func RenderFinalAnswerAtVersion(envelope FinalAnswerEnvelope, rendererVersion string) (string, error) {
	if err := ValidateFinalAnswerEnvelope(envelope); err != nil {
		return "", err
	}
	if !validFinalAnswerRendererVersion(rendererVersion) {
		return "", errors.New("unknown final answer renderer version")
	}
	var rendered string
	switch envelope.Variant {
	case EvidenceBackedAnswer:
		lines, err := renderClaims(envelope.Claims, rendererVersion)
		if err != nil {
			return "", err
		}
		rendered = "已核验证据支持以下案件事实：\n" + strings.Join(lines, "\n")
	case PartialEvidenceAnswer:
		lines, err := renderClaims(envelope.Claims, rendererVersion)
		if err != nil {
			return "", err
		}
		rendered = "仅在已查范围内可确认以下内容，不得外推为全案结论：\n" + strings.Join(lines, "\n") + "\n未覆盖范围：" + renderSafeValues(envelope.MissingScope)
	case VerifiedNoHitAnswer:
		rendered = "未在已查范围发现匹配记录；这不等同于范围外不存在、金额为零、无关联或无异常。"
	case SourceUnavailableAnswer:
		rendered = CaseSourceUnavailableText
	case NeedsEvidenceAnswer:
		for _, missing := range envelope.MissingScope {
			if missing == "publication_receipt" {
				rendered = CaseReportUnavailableText
				break
			}
		}
		if rendered == "" {
			if rendererVersion == HistoricalFinalAnswerRendererVersion {
				rendered = LegacyCaseUnverifiedText
			} else {
				rendered = CaseUnverifiedText
			}
		}
	case GeneralGuidanceAnswer:
		if len(envelope.Guidance) == 1 && envelope.Guidance[0] == OrdinaryResultOnlyGuidanceCodeV1 {
			if rendererVersion != FinalAnswerRendererVersion || envelope.OrdinaryResult == nil {
				return "", errors.New("ordinary-only guidance requires the current renderer")
			}
			rendered = ""
		} else {
			lines := make([]string, 0, len(envelope.Guidance))
			for _, code := range envelope.Guidance {
				text, ok := generalGuidanceText(code)
				if !ok {
					return "", errors.New("general guidance contains an unknown host code")
				}
				lines = append(lines, text)
			}
			rendered = strings.Join(lines, "\n")
		}
	default:
		return "", errors.New("unknown final answer variant")
	}
	if envelope.OrdinaryResult != nil {
		if rendered == "" {
			return envelope.OrdinaryResult.Text, nil
		}
		rendered = envelope.OrdinaryResult.Text + "\n\n" + rendered
	}
	if err := domaincaseentity.ValidatePublicValueWithoutReferenceV1(rendered); err != nil {
		return "", errors.Join(ErrInternalEntityReferenceRendering, err)
	}
	return rendered, nil
}

func renderClaims(claims []ClaimRecord, rendererVersion string) ([]string, error) {
	projected := append([]ClaimRecord(nil), claims...)
	if rendererVersion == FinalAnswerRendererVersion {
		slots, err := buildAcceptedEntitySlotBindingsV1(claims)
		if err != nil {
			return nil, err
		}
		labels := make(map[domaincaseentity.ReferenceV1]string, len(slots))
		for index := range slots {
			slot := slots[index]
			if err := slot.UseReferenceV1(func(reference domaincaseentity.ReferenceV1) error {
				labels[reference] = fmt.Sprintf("〔账户槽位 %d〕", index+1)
				return nil
			}); err != nil {
				return nil, err
			}
		}
		projected = projectClaimEntitySlotsV1(claims, labels)
	}
	if err := domaincaseentity.ValidatePublicValueWithoutReferenceV1(projected); err != nil {
		return nil, errors.Join(ErrInternalEntityReferenceRendering, err)
	}
	lines := make([]string, 0, len(claims))
	consumedAggregateClaims := map[int]bool{}
	for index, claim := range projected {
		if consumedAggregateClaims[index] {
			continue
		}
		if aggregate, indexes, ok := accountFlowAggregateClaimsAt(projected, index); ok {
			line, err := renderAccountFlowAggregate(aggregate)
			if err != nil {
				return nil, err
			}
			lines = append(lines, "- "+line)
			for _, consumed := range indexes {
				consumedAggregateClaims[consumed] = true
			}
			continue
		}
		line, err := renderClaim(claim)
		if err != nil {
			return nil, err
		}
		lines = append(lines, "- "+line)
	}
	return lines, nil
}

type accountFlowAggregateClaims struct {
	inflow  ClaimRecord
	outflow ClaimRecord
	count   ClaimRecord
}

func accountFlowAggregateClaimsAt(claims []ClaimRecord, index int) (accountFlowAggregateClaims, []int, bool) {
	if index < 0 || index >= len(claims) || accountFlowAggregateClaimKind(claims[index]) == "" {
		return accountFlowAggregateClaims{}, nil, false
	}
	base := claims[index]
	group := accountFlowAggregateClaims{}
	indexes := []int{}
	amountIn, amountOut, count := 0, 0, 0
	for candidateIndex, candidate := range claims {
		if !sameAccountFlowAggregateScope(base, candidate) {
			continue
		}
		indexes = append(indexes, candidateIndex)
		switch accountFlowAggregateClaimKind(candidate) {
		case "inflow":
			amountIn++
			group.inflow = candidate
		case "outflow":
			amountOut++
			group.outflow = candidate
		case "count":
			count++
			group.count = candidate
		}
	}
	if amountIn != 1 || amountOut != 1 || count != 1 {
		return accountFlowAggregateClaims{}, nil, false
	}
	return group, indexes, true
}

func accountFlowAggregateClaimKind(claim ClaimRecord) string {
	if claim.SupportedScope == nil || (claim.SupportState != ClaimVerified && claim.SupportState != ClaimPartial) ||
		claim.NormalizedPayload.Granularity != accountFlowAggregateGranularity {
		return ""
	}
	switch claim.ClaimType {
	case ClaimAmount:
		switch claim.NormalizedPayload.Direction {
		case "in":
			return "inflow"
		case "out":
			return "outflow"
		}
	case ClaimCount:
		return "count"
	}
	return ""
}

func sameAccountFlowAggregateScope(left, right ClaimRecord) bool {
	if accountFlowAggregateClaimKind(left) == "" || accountFlowAggregateClaimKind(right) == "" ||
		left.SupportedScope == nil || right.SupportedScope == nil {
		return false
	}
	leftPayload, rightPayload := left.NormalizedPayload, right.NormalizedPayload
	return reflect.DeepEqual(*left.SupportedScope, *right.SupportedScope) &&
		leftPayload.SubjectID == rightPayload.SubjectID && leftPayload.EntityID == rightPayload.EntityID &&
		(leftPayload.AccountID == rightPayload.AccountID || leftPayload.AccountID == "" || rightPayload.AccountID == "") &&
		leftPayload.StartAt == rightPayload.StartAt &&
		leftPayload.EndAt == rightPayload.EndAt &&
		(leftPayload.Currency == rightPayload.Currency || leftPayload.Currency == "" || rightPayload.Currency == "")
}

func renderAccountFlowAggregate(group accountFlowAggregateClaims) (string, error) {
	inflow, inflowOK := new(big.Int).SetString(group.inflow.NormalizedPayload.AmountMinor, 10)
	outflow, outflowOK := new(big.Int).SetString(group.outflow.NormalizedPayload.AmountMinor, 10)
	if !inflowOK || !outflowOK {
		return "", errors.New("account-flow aggregate amount is not canonical")
	}
	net := new(big.Int).Sub(inflow, outflow)
	base := group.inflow.NormalizedPayload
	return fmt.Sprintf(
		"主体 %s 的账户 %s 资金汇总：流入 %s %s，流出 %s %s，有符号净额 %s %s，交易笔数 %s。（限定范围：%s）",
		safeRenderText(base.SubjectID), renderMaskedIdentifier(base.AccountID), inflow.String(), safeRenderText(base.Currency),
		outflow.String(), safeRenderText(base.Currency), net.String(), safeRenderText(base.Currency),
		safeRenderText(group.count.NormalizedPayload.Count), renderSupportedScope(*group.inflow.SupportedScope, base.Granularity),
	), nil
}

// BuildAcceptedEntitySlotBindingsV1 returns only host-private slot bindings
// derived from verified claim fields. Slot order is the first canonical claim
// occurrence and is therefore stable across restart, replay, and compaction.
func BuildAcceptedEntitySlotBindingsV1(envelope FinalAnswerEnvelope) ([]AcceptedEntitySlotBindingV1, error) {
	if err := ValidateFinalAnswerEnvelope(envelope); err != nil {
		return nil, err
	}
	claims := envelope.Claims
	if envelope.AccountFlowOutcome == nil {
		claims = claimsWithoutAccountFlowTypedCandidatesV1(claims)
	}
	return buildAcceptedEntitySlotBindingsV1(claims)
}

func claimsWithoutAccountFlowTypedCandidatesV1(claims []ClaimRecord) []ClaimRecord {
	filtered := make([]ClaimRecord, 0, len(claims))
	for _, claim := range claims {
		payload := claim.NormalizedPayload
		isAccountFlowAmount := claim.ClaimType == ClaimAmount &&
			(payload.Direction == "in" || payload.Direction == "out") &&
			payload.AmountMinor != "" && payload.Currency != ""
		isAccountFlowCount := claim.ClaimType == ClaimCount && payload.Direction == "" && payload.Count != ""
		if payload.Granularity == accountFlowAggregateGranularity &&
			(isAccountFlowAmount || isAccountFlowCount) {
			continue
		}
		filtered = append(filtered, claim)
	}
	return filtered
}

func buildAcceptedEntitySlotBindingsV1(claims []ClaimRecord) ([]AcceptedEntitySlotBindingV1, error) {
	references := []domaincaseentity.ReferenceV1{}
	seen := map[domaincaseentity.ReferenceV1]bool{}
	for _, claim := range claims {
		for _, value := range claimEntityReferenceCandidatesV1(claim) {
			if strings.TrimSpace(value) == "" {
				continue
			}
			reference := domaincaseentity.ReferenceV1(value)
			if domaincaseentity.ValidateReferenceV1(value) == nil {
				if !seen[reference] {
					seen[reference] = true
					references = append(references, reference)
				}
				continue
			}
			if domaincaseentity.ContainsReferenceV1(value) {
				return nil, errors.Join(ErrInternalEntityReferenceRendering, errors.New("claim contains a non-slot internal entity reference"))
			}
		}
	}

	slots := make([]AcceptedEntitySlotBindingV1, 0, len(references))
	for index, reference := range references {
		claimIDs := []string{}
		receiptSet := map[string]bool{}
		for _, claim := range claims {
			if !claimContainsExactEntityReferenceV1(claim, reference) {
				continue
			}
			claimIDs = append(claimIDs, claim.ClaimID)
			for _, receiptID := range claim.EvidenceIDs {
				receiptSet[receiptID] = true
			}
		}
		receiptIDs := make([]string, 0, len(receiptSet))
		for receiptID := range receiptSet {
			receiptIDs = append(receiptIDs, receiptID)
		}
		sort.Strings(receiptIDs)
		slots = append(slots, AcceptedEntitySlotBindingV1{
			SlotID: fmt.Sprintf("account-slot-%d", index+1), ClaimIDs: claimIDs,
			ReceiptIDs: receiptIDs, reference: reference,
		})
	}
	return slots, nil
}

func claimEntityReferenceCandidatesV1(claim ClaimRecord) []string {
	values := []string{
		claim.NormalizedPayload.SubjectID,
		claim.NormalizedPayload.EntityID,
		claim.NormalizedPayload.AccountID,
		claim.NormalizedPayload.CounterpartyID,
	}
	if claim.SupportedScope != nil {
		values = append(values, claim.SupportedScope.EntityIDs...)
		values = append(values, claim.SupportedScope.AccountIDs...)
	}
	return values
}

func claimContainsExactEntityReferenceV1(claim ClaimRecord, reference domaincaseentity.ReferenceV1) bool {
	for _, value := range claimEntityReferenceCandidatesV1(claim) {
		if value == string(reference) {
			return true
		}
	}
	return false
}

func projectClaimEntitySlotsV1(
	claims []ClaimRecord,
	labels map[domaincaseentity.ReferenceV1]string,
) []ClaimRecord {
	projected := append([]ClaimRecord(nil), claims...)
	for index := range projected {
		claim := &projected[index]
		claim.EvidenceIDs = append([]string(nil), claim.EvidenceIDs...)
		claim.CounterEvidenceIDs = append([]string(nil), claim.CounterEvidenceIDs...)
		claim.AllowedWording = append([]string(nil), claim.AllowedWording...)
		claim.ProhibitedUpgrades = append([]string(nil), claim.ProhibitedUpgrades...)
		claim.NormalizedPayload.SubjectID = projectEntitySlotValueV1(claim.NormalizedPayload.SubjectID, labels)
		claim.NormalizedPayload.EntityID = projectEntitySlotValueV1(claim.NormalizedPayload.EntityID, labels)
		claim.NormalizedPayload.AccountID = projectEntitySlotValueV1(claim.NormalizedPayload.AccountID, labels)
		claim.NormalizedPayload.CounterpartyID = projectEntitySlotValueV1(claim.NormalizedPayload.CounterpartyID, labels)
		if claim.SupportedScope != nil {
			scope := *claim.SupportedScope
			scope.EntityIDs = projectEntitySlotValuesV1(scope.EntityIDs, labels)
			scope.AccountIDs = projectEntitySlotValuesV1(scope.AccountIDs, labels)
			scope.Directions = append([]string(nil), scope.Directions...)
			scope.SourceIDs = append([]string(nil), scope.SourceIDs...)
			claim.SupportedScope = &scope
		}
	}
	return projected
}

func projectEntitySlotValuesV1(values []string, labels map[domaincaseentity.ReferenceV1]string) []string {
	projected := append([]string(nil), values...)
	for index, value := range projected {
		projected[index] = projectEntitySlotValueV1(value, labels)
	}
	return projected
}

func projectEntitySlotValueV1(value string, labels map[domaincaseentity.ReferenceV1]string) string {
	if label, ok := labels[domaincaseentity.ReferenceV1(value)]; ok {
		return label
	}
	return value
}

var _ json.Marshaler = AcceptedEntitySlotBindingV1{}
var _ json.Unmarshaler = (*AcceptedEntitySlotBindingV1)(nil)

func renderClaim(claim ClaimRecord) (string, error) {
	if claim.SupportedScope == nil {
		return "", errors.New("supported claim cannot render without explicit scope")
	}
	payload := claim.NormalizedPayload
	var text string
	switch claim.ClaimType {
	case ClaimAmount:
		text = fmt.Sprintf("主体 %s 的金额（最小货币单位）为 %s %s。", safeRenderText(payload.SubjectID), payload.AmountMinor, safeRenderText(payload.Currency))
	case ClaimCount:
		text = fmt.Sprintf("主体 %s 的记录数量为 %s。", safeRenderText(payload.SubjectID), payload.Count)
	case ClaimAccount:
		text = fmt.Sprintf("主体 %s 对应账户 %s。", safeRenderText(payload.SubjectID), renderMaskedIdentifier(payload.AccountID))
	case ClaimEntity:
		text = fmt.Sprintf("主体 %s 对应实体 %s。", safeRenderText(payload.SubjectID), safeRenderText(payload.EntityID))
	case ClaimDirection:
		text = fmt.Sprintf("主体 %s 的资金方向为 %s。", safeRenderText(payload.SubjectID), safeRenderText(payload.Direction))
	case ClaimDateRange:
		text = fmt.Sprintf("主体 %s 的已核验时间范围为 %s 至 %s。", safeRenderText(payload.SubjectID), safeRenderText(payload.StartAt), safeRenderText(payload.EndAt))
	case ClaimRelationship:
		text = fmt.Sprintf("主体 %s 与 %s 的已核验关系类型为 %s。", safeRenderText(payload.SubjectID), safeRenderText(payload.CounterpartyID), safeRenderText(payload.RelationshipType))
	case ClaimQuote:
		text = fmt.Sprintf("主体 %s 的已核验原文为：“%s”。", safeRenderText(payload.SubjectID), safeRenderText(payload.Quote))
	case ClaimDeviceIdentifier:
		text = fmt.Sprintf("主体 %s 的 %s 标识为 %s。", safeRenderText(payload.SubjectID), safeRenderText(payload.DeviceKind), renderMaskedIdentifier(payload.DeviceIdentifier))
	case ClaimOwnership, ClaimControl, ClaimAddress, ClaimChange, ClaimBidCertificate, ClaimBidEditMetadata:
		text = fmt.Sprintf("实体 %s 的字段 %s 已由证据核验；字段值仅保留于受控证据，不在普通答复中展示。", safeRenderText(payload.EntityID), safeRenderText(payload.AttributeName))
	case ClaimLegalCharacterization:
		return "", errors.New("legal characterization cannot be rendered without the dedicated human-review gate")
	default:
		return "", errors.New("unsupported verified claim type")
	}
	return text + "（限定范围：" + renderSupportedScope(*claim.SupportedScope, payload.Granularity) + "）", nil
}

func renderSupportedScope(scope EvidenceQueryRange, granularity string) string {
	parts := []string{}
	if len(scope.EntityIDs) > 0 {
		parts = append(parts, "实体 "+renderSafeValues(scope.EntityIDs))
	}
	if len(scope.AccountIDs) > 0 {
		masked := make([]string, 0, len(scope.AccountIDs))
		for _, value := range scope.AccountIDs {
			masked = append(masked, renderMaskedIdentifier(value))
		}
		parts = append(parts, "账户 "+strings.Join(masked, "、"))
	}
	if len(scope.Directions) > 0 {
		parts = append(parts, "方向 "+renderSafeValues(scope.Directions))
	}
	if scope.StartAt != "" && scope.EndAt != "" {
		parts = append(parts, "时间 "+safeRenderText(scope.StartAt)+" 至 "+safeRenderText(scope.EndAt))
	}
	if granularity = strings.TrimSpace(granularity); granularity != "" {
		parts = append(parts, "粒度 "+safeRenderText(granularity))
	}
	if len(scope.SourceIDs) > 0 {
		parts = append(parts, fmt.Sprintf("已核验来源 %d 项", len(scope.SourceIDs)))
	}
	if len(parts) == 0 {
		return "宿主回执明确记录的查询范围"
	}
	return strings.Join(parts, "；")
}

func renderSafeValues(values []string) string {
	safe := make([]string, 0, len(values))
	for _, value := range values {
		safe = append(safe, safeRenderText(value))
	}
	return strings.Join(safe, "、")
}

func safeRenderText(value string) string {
	value = domainprivacy.ProjectText(value).Text
	value = strings.NewReplacer("\r\n", `\n`, "\r", `\r`, "\n", `\n`, "\t", `\t`).Replace(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)
	return strings.NewReplacer(
		`\`, `\\`, "`", "\\`", "*", "\\*", "[", "\\[", "]", "\\]",
		"<", "&lt;", ">", "&gt;", "#", "\\#", "|", "\\|",
	).Replace(value)
}

func renderMaskedIdentifier(value string) string {
	if acceptedEntitySlotLabelV1(value) {
		return safeRenderText(value)
	}
	masked := maskIdentifier(value)
	if strings.HasPrefix(masked, "****") {
		return "****" + safeRenderText(strings.TrimPrefix(masked, "****"))
	}
	return masked
}

func acceptedEntitySlotLabelV1(value string) bool {
	const prefix = "〔账户槽位 "
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, "〕") {
		return false
	}
	ordinal := strings.TrimSuffix(strings.TrimPrefix(value, prefix), "〕")
	if ordinal == "" || ordinal[0] == '0' {
		return false
	}
	for _, character := range ordinal {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func maskIdentifier(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return "****" + string(runes[len(runes)-4:])
}

func generalGuidanceText(code string) (string, bool) {
	switch strings.TrimSpace(code) {
	case "explain_evidence_requirements":
		return "案件事实必须由同案、同轮、同数据快照的宿主证据回执支持。", true
	case "explain_checked_scope":
		return "结论只能覆盖已明确检查且完整记录的范围。", true
	case "explain_source_connection":
		return "请先连接并核验当前案件的数据源，再开展事实分析。", true
	case "explain_privacy_controls":
		return "普通聊天、模型和外部通道不得包含完整敏感信息；当前本机的类型化显示可在有效案件与快照绑定下按需展示。", true
	case "explain_agent_safety_authority":
		return AgentSafetyAuthorityUnavailableText, true
	default:
		return "", false
	}
}
