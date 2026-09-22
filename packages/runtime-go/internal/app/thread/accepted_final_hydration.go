package thread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

// AcceptedFinalHydrationInputV1 closes restart hydration over the exact durable
// event frontier and every accepted-final manifest within it. ProjectedThread
// is the already current public thread snapshot; AuthorityThread is the
// private host input used to bind each visible slot to its V5 record and by
// the trusted projector. It is never returned.
type AcceptedFinalHydrationInputV1 struct {
	Context           context.Context
	RouteThreadID     string
	SnapshotLatestSeq int
	AuthorityThread   map[string]any
	ProjectedThread   map[string]any
	DurableEvents     []map[string]any
	Projector         PublicProjector
}

type acceptedFinalHydrationSlotBindingV1 struct {
	TurnID              string
	PublicationCommitID string
	AssistantItem       map[string]any
}

type durableAcceptedFinalManifestV1 struct {
	Events            []map[string]any
	WireSchemaVersion int
	WirePurpose       string
}

type acceptedFinalHydrationPreparedDeliveryV1 struct {
	Manifest        []map[string]any
	ProjectedEvents []map[string]any
}

// AcceptedFinalHydrationProjectionV1 is a bounded projection over the exact
// accepted-final turns visible in one public thread snapshot. Deliveries is in
// public turn order and contains exactly one independently sealed batch for
// every visible accepted-final turn. Latest is the final element for the
// internal legacy seam; the HTTP adapter emits the singular compatibility
// alias only when Deliveries contains exactly one group.
type AcceptedFinalHydrationProjectionV1 struct {
	Latest     map[string]any
	Deliveries []map[string]any
}

// BuildLatestAcceptedFinalHydrationV1 returns the latest visible
// accepted-final delivery as a newly sealed, current-authority transport
// batch. It never derives authority from visible text or a digest alone.
func BuildLatestAcceptedFinalHydrationV1(
	input AcceptedFinalHydrationInputV1,
) (map[string]any, bool, error) {
	projection, present, err := BuildAcceptedFinalHydrationProjectionV1(input)
	return projection.Latest, present, err
}

// BuildAcceptedFinalHydrationProjectionV1 returns one independently sealed,
// current-authority transport batch for every visible accepted-final turn. It
// never lets the latest batch authorize an earlier public view or item.
func BuildAcceptedFinalHydrationProjectionV1(
	input AcceptedFinalHydrationInputV1,
) (AcceptedFinalHydrationProjectionV1, bool, error) {
	projector, ok := input.Projector.(*TrustedPublicProjector)
	if !ok || projector.retainedFactVerifier == nil || input.Context == nil {
		return buildAcceptedFinalHydrationProjectionV1(input)
	}
	if err := input.Context.Err(); err != nil {
		return AcceptedFinalHydrationProjectionV1{}, false, err
	}
	if err := validateAcceptedFinalHydrationDeliveryBoundV1(input.ProjectedThread); err != nil {
		return AcceptedFinalHydrationProjectionV1{}, false, err
	}
	retained, originals, err := projector.retainedFactsForThreadV1(input.AuthorityThread)
	if err != nil {
		return AcceptedFinalHydrationProjectionV1{}, false, err
	}
	return buildRetainedAcceptedFinalHydrationV1(input, projector, retained, originals)
}

func buildRetainedAcceptedFinalHydrationV1(input AcceptedFinalHydrationInputV1, projector *TrustedPublicProjector, retained map[string]bool, originals []retainedFactAdmissionV1) (AcceptedFinalHydrationProjectionV1, bool, error) {
	verify := func() error {
		if err := projector.verifyRetainedFactsV1(input.AuthorityThread, originals); err != nil {
			return err
		}
		return input.Context.Err()
	}
	if err := verify(); err != nil {
		return AcceptedFinalHydrationProjectionV1{}, false, err
	}
	// One synchronous operation owns all groups. Each event still undergoes
	// primary-CAS/manifest/current-case checks. Fresh historical witness and
	// current permission are checked before signing and after all groups; no
	// partially built delivery or per-request grant escapes a failed batch.
	scoped := *projector
	scoped.retainedFactVerifier = nil
	scoped.retainedProjectionThread = contracts.CloneMap(input.AuthorityThread)
	scoped.retainedProjectionFacts = retained
	input.Projector = &scoped
	projection, present, err := buildAcceptedFinalHydrationProjectionV1(input)
	if err != nil {
		return AcceptedFinalHydrationProjectionV1{}, false, err
	}
	if err := verify(); err != nil {
		return AcceptedFinalHydrationProjectionV1{}, false, err
	}
	return projection, present, nil
}

func buildAcceptedFinalHydrationProjectionV1(
	input AcceptedFinalHydrationInputV1,
) (AcceptedFinalHydrationProjectionV1, bool, error) {
	empty := AcceptedFinalHydrationProjectionV1{}
	if input.Context == nil || input.Context.Err() != nil || input.Projector == nil {
		return empty, false, errors.New("accepted final hydration authority is unavailable")
	}
	threadID := strings.TrimSpace(input.RouteThreadID)
	if threadID == "" || contracts.SafeRecordID(threadID) != threadID ||
		contracts.StringField(input.AuthorityThread, "id") != threadID ||
		contracts.StringField(input.ProjectedThread, "id") != threadID ||
		input.SnapshotLatestSeq < 0 {
		return empty, false, errors.New("accepted final hydration thread identity is invalid")
	}
	if err := validateAcceptedFinalHydrationDeliveryBoundV1(input.ProjectedThread); err != nil {
		return empty, false, err
	}

	bindings, err := acceptedFinalHydrationSlotBindingsV1(input.AuthorityThread, input.ProjectedThread)
	if err != nil {
		return empty, false, err
	}
	if len(bindings) > domainevent.AcceptedFinalDeliveryGroupLimitV1 {
		return empty, false, errors.New("accepted final hydration delivery count exceeds the public bound")
	}
	manifests, durableLatestSeq, err := durableAcceptedFinalManifestsV1(
		threadID,
		input.DurableEvents,
	)
	if err != nil {
		return empty, false, err
	}
	if durableLatestSeq != input.SnapshotLatestSeq {
		return empty, false, errors.New("accepted final hydration snapshot and event frontier are torn")
	}
	if len(bindings) == 0 && len(manifests) == 0 {
		return empty, false, nil
	}
	if len(manifests) == 0 {
		return empty, false, errors.New("accepted final hydration manifest is detached from the current snapshot: durable manifest missing")
	}

	authority, ok := input.Projector.(AcceptedFinalDeliveryAuthority)
	if !ok {
		return empty, false, errors.New("accepted final hydration authority is unavailable")
	}
	bindingByIdentity := make(map[string]acceptedFinalHydrationSlotBindingV1, len(bindings))
	for _, binding := range bindings {
		identity := acceptedFinalHydrationIdentityV1(binding.TurnID, binding.PublicationCommitID)
		if _, duplicate := bindingByIdentity[identity]; duplicate {
			return empty, false, errors.New("accepted final hydration public slot binding is duplicated")
		}
		bindingByIdentity[identity] = binding
	}

	preparedDeliveries := make([]acceptedFinalHydrationPreparedDeliveryV1, 0, len(bindings))
	matchedBindings := 0
	for _, durableManifest := range manifests {
		manifest := durableManifest.Events
		turnID := contracts.StringField(manifest[0], "turnId")
		commitID := contracts.StringField(manifest[0], "publicationCommitId")
		binding, hasBinding := bindingByIdentity[acceptedFinalHydrationIdentityV1(turnID, commitID)]
		if durableManifest.WireSchemaVersion == domainevent.AcceptedFinalDeliveryBatchV1Version {
			// A frozen V1 manifest contains the private accepted-final record. It
			// remains audit-only in its original wire family and is never projected
			// or re-sealed as a current generic V2 batch.
			if hasBinding || historicalAcceptedFinalManifestMatchesAuthorityV1(input.AuthorityThread, manifest) != nil {
				return empty, false, errors.New("historical accepted final hydration manifest claims a current public slot")
			}
			continue
		}
		projectedEvents := make([]map[string]any, 0, len(manifest))
		invisibleEvents := 0
		for _, event := range manifest {
			projected, visible, projectErr := input.Projector.ProjectEvent(threadID, input.AuthorityThread, event)
			if projectErr != nil || visible && projected == nil || !visible && projected != nil {
				return empty, false, errors.Join(errors.New("accepted final hydration public projection was rejected"), projectErr)
			}
			if !visible {
				invisibleEvents++
				continue
			}
			projectedEvents = append(projectedEvents, projected)
		}
		if !hasBinding {
			if invisibleEvents == len(manifest) {
				continue
			}
			return empty, false, errors.New("accepted final hydration manifest visibility is detached from the current snapshot")
		}
		if matchedBindings >= len(bindings) ||
			bindings[matchedBindings].TurnID != turnID ||
			bindings[matchedBindings].PublicationCommitID != commitID {
			return empty, false, errors.New("accepted final hydration manifest order is detached from the current snapshot")
		}
		if invisibleEvents != 0 {
			return empty, false, errors.New("accepted final hydration manifest is detached from the current snapshot: visible reference has invisible events")
		}
		projectedAssistant, ok := projectedEvents[0]["item"].(map[string]any)
		if !ok || !reflect.DeepEqual(projectedAssistant, binding.AssistantItem) {
			return empty, false, errors.New("accepted final hydration public item is detached from the sealed batch")
		}
		preparedDeliveries = append(preparedDeliveries, acceptedFinalHydrationPreparedDeliveryV1{
			Manifest: manifest, ProjectedEvents: projectedEvents,
		})
		matchedBindings++
	}
	if matchedBindings != len(bindings) {
		return empty, false, errors.New("accepted final hydration public slot has no exact durable manifest")
	}
	if len(preparedDeliveries) == 0 {
		return empty, false, nil
	}

	// Every visible delivery group is projected and matched before the first
	// signature is issued. A hostile later group therefore cannot leave a
	// valid-looking signed prefix behind.
	deliveries := make([]map[string]any, 0, len(preparedDeliveries))
	for _, prepared := range preparedDeliveries {
		seal, sealErr := authority.SealAcceptedFinalDelivery(input.Context, prepared.Manifest)
		if sealErr != nil {
			return empty, false, errors.Join(errors.New("accepted final hydration seal was rejected"), sealErr)
		}
		rawBatch, batchErr := domainevent.NewAcceptedFinalDeliveryBatchV2(prepared.Manifest, seal)
		if batchErr != nil || authority.ValidateAcceptedFinalDelivery(input.Context, rawBatch) != nil {
			return empty, false, errors.Join(errors.New("accepted final hydration durable batch is invalid"), batchErr)
		}
		projectedBatch, projectionErr := domainevent.NewAcceptedFinalDeliveryBatchV2(prepared.ProjectedEvents, seal)
		if projectionErr != nil || projectedBatch.BatchID != rawBatch.BatchID ||
			projectedBatch.EventManifestDigest != rawBatch.EventManifestDigest ||
			!reflect.DeepEqual(projectedBatch.PublicationAuthority, rawBatch.PublicationAuthority) ||
			authority.ValidateAcceptedFinalDelivery(input.Context, projectedBatch) != nil {
			return empty, false, errors.Join(errors.New("accepted final hydration projected batch is invalid"), projectionErr)
		}
		out := domainevent.AcceptedFinalDeliveryBatchV2Map(projectedBatch)
		if out == nil {
			return empty, false, errors.New("accepted final hydration projection is unavailable")
		}
		deliveries = append(deliveries, out)
	}
	return AcceptedFinalHydrationProjectionV1{
		Latest: deliveries[len(deliveries)-1], Deliveries: deliveries,
	}, true, nil
}

func validateAcceptedFinalHydrationDeliveryBoundV1(projectedThread map[string]any) error {
	turns, ok := projectedThread["turns"].([]any)
	if !ok {
		return errors.New("accepted final hydration turns are invalid")
	}
	visibleDeliveries := 0
	for _, value := range turns {
		turn, ok := value.(map[string]any)
		if !ok {
			return errors.New("accepted final hydration turn is invalid")
		}
		_, hasPrivateRecord := turn["acceptedFinal"]
		_, hasPublicView := turn["acceptedFinalView"]
		if !hasPrivateRecord && !hasPublicView {
			continue
		}
		visibleDeliveries++
		if visibleDeliveries > domainevent.AcceptedFinalDeliveryGroupLimitV1 {
			return errors.New("accepted final hydration delivery count exceeds the public bound")
		}
	}
	return nil
}

func acceptedFinalHydrationIdentityV1(turnID, commitID string) string {
	return turnID + "\x00" + commitID
}

func acceptedFinalHydrationSlotBindingsV1(
	authorityThread map[string]any,
	projectedThread map[string]any,
) ([]acceptedFinalHydrationSlotBindingV1, error) {
	threadID := strings.TrimSpace(contracts.StringField(projectedThread, "id"))
	if contracts.StringField(authorityThread, "id") != threadID {
		return nil, errors.New("accepted final hydration authority thread is invalid")
	}
	authorityTurns, ok := authorityThread["turns"].([]any)
	if !ok {
		return nil, errors.New("accepted final hydration authority turns are invalid")
	}
	authorityByTurn := make(map[string]map[string]any, len(authorityTurns))
	for _, value := range authorityTurns {
		turn, ok := value.(map[string]any)
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if !ok || turnID == "" || contracts.SafeRecordID(turnID) != turnID ||
			contracts.StringField(turn, "threadId") != threadID {
			return nil, errors.New("accepted final hydration authority turn is invalid")
		}
		if _, duplicate := authorityByTurn[turnID]; duplicate {
			return nil, errors.New("accepted final hydration authority turn is duplicated")
		}
		authorityByTurn[turnID] = turn
	}
	turns, ok := projectedThread["turns"].([]any)
	if !ok {
		return nil, errors.New("accepted final hydration turns are invalid")
	}
	bindings := make([]acceptedFinalHydrationSlotBindingV1, 0)
	seenTurns := make(map[string]struct{}, len(turns))
	for _, value := range turns {
		turn, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("accepted final hydration turn is invalid")
		}
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if turnID == "" || contracts.SafeRecordID(turnID) != turnID ||
			contracts.StringField(turn, "threadId") != threadID {
			return nil, errors.New("accepted final hydration turn thread is invalid")
		}
		_, hasRecord := turn["acceptedFinal"]
		rawView, hasView := turn["acceptedFinalView"]
		if hasRecord {
			return nil, errors.New("accepted final hydration public turn contains private authority")
		}
		if !hasRecord && !hasView {
			if turnItemsClaimAcceptedFinalV1(turn) {
				return nil, errors.New("accepted final hydration turn authority is torn")
			}
			continue
		}
		if _, duplicate := seenTurns[turnID]; duplicate {
			return nil, errors.New("accepted final hydration public turn is duplicated")
		}
		seenTurns[turnID] = struct{}{}
		authorityTurn := authorityByTurn[turnID]
		if authorityTurn == nil {
			return nil, errors.New("accepted final hydration public turn has no private host authority")
		}
		record, err := domainevidence.ParseAcceptedFinalRecord(authorityTurn["acceptedFinal"])
		if err != nil || record.ThreadID != threadID || record.TurnID != turnID {
			return nil, errors.Join(
				errors.New("accepted final hydration private turn authority is invalid"),
				err,
			)
		}
		privateAssistant, privateMatches, privateErr := acceptedFinalHydrationAssistantMatchesV1(authorityTurn, record)
		if privateErr != nil || privateMatches != 1 || privateAssistant == nil {
			return nil, errors.Join(
				errors.New("accepted final hydration private assistant authority is invalid"),
				privateErr,
			)
		}
		view, viewErr := domainevidence.NewAcceptedFinalPublicViewFromRecordV3(record)
		if viewErr != nil || !sameAcceptedFinalHydrationJSONV1(
			rawView,
			domainevidence.AcceptedFinalPublicViewRecordV3(view),
		) {
			return nil, errors.Join(
				errors.New("accepted final hydration public turn view is detached from private authority"),
				viewErr,
			)
		}
		assistantItem, matches, matchErr := acceptedFinalHydrationPublicAssistantMatchesV3(
			turn,
			view,
		)
		if matchErr != nil || matches != 1 {
			return nil, errors.Join(
				errors.New("accepted final hydration public assistant is detached from private authority"),
				matchErr,
			)
		}
		bindings = append(bindings, acceptedFinalHydrationSlotBindingV1{
			TurnID: turnID, PublicationCommitID: record.RecordDigest, AssistantItem: assistantItem,
		})
	}
	return bindings, nil
}

func acceptedFinalHydrationPublicAssistantMatchesV3(
	turn map[string]any,
	view domainevidence.AcceptedFinalPublicViewV3,
) (map[string]any, int, error) {
	items, ok := turn["items"].([]any)
	if !ok {
		return nil, 0, errors.New("accepted final hydration public items are invalid")
	}
	matches := 0
	var matched map[string]any
	threadID := contracts.StringField(turn, "threadId")
	turnID := contracts.StringField(turn, "id")
	expectedView := domainevidence.AcceptedFinalPublicViewRecordV3(view)
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, 0, errors.New("accepted final hydration public item is invalid")
		}
		_, hasRecord := item["acceptedFinal"]
		rawView, hasView := item["acceptedFinalView"]
		if !hasRecord && !hasView {
			continue
		}
		itemView, viewErr := domainevidence.ParseAcceptedFinalPublicViewV3Value(rawView)
		if hasRecord || !hasView || viewErr != nil || !reflect.DeepEqual(itemView, view) ||
			!sameAcceptedFinalHydrationJSONV1(rawView, expectedView) ||
			contracts.StringField(item, "kind") != "assistant_text" ||
			contracts.StringField(item, "threadId") != threadID ||
			contracts.StringField(item, "turnId") != turnID ||
			contracts.StringField(item, "finishedAt") != view.AcceptedAt {
			return nil, 0, errors.Join(errors.New("accepted final hydration public item authority is invalid"), viewErr)
		}
		matches++
		matched = contracts.CloneMap(item)
	}
	return matched, matches, nil
}

func acceptedFinalHydrationAssistantMatchesV1(
	turn map[string]any,
	record domainevidence.AcceptedFinalRecord,
) (map[string]any, int, error) {
	items, ok := turn["items"].([]any)
	if !ok {
		return nil, 0, errors.New("accepted final hydration items are invalid")
	}
	matches := 0
	var matched map[string]any
	expectedView, err := domainevidence.NewAcceptedFinalPublicViewFromRecordV2(record)
	if err != nil {
		return nil, 0, err
	}
	expectedViewRecord := domainevidence.AcceptedFinalPublicViewRecordV2(expectedView)
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, 0, errors.New("accepted final hydration item is invalid")
		}
		raw, hasRecord := item["acceptedFinal"]
		rawView, hasView := item["acceptedFinalView"]
		if !hasRecord && !hasView {
			continue
		}
		view, viewOK := rawView.(map[string]any)
		if !hasRecord || !hasView || !viewOK {
			return nil, 0, errors.New("accepted final hydration item authority is torn")
		}
		itemRecord, err := domainevidence.ParseAcceptedFinalRecord(raw)
		if err != nil || !reflect.DeepEqual(itemRecord, record) ||
			!sameAcceptedFinalHydrationJSONV1(view, expectedViewRecord) ||
			contracts.StringField(item, "kind") != "assistant_text" ||
			contracts.StringField(item, "threadId") != record.ThreadID ||
			contracts.StringField(item, "turnId") != record.TurnID {
			return nil, 0, errors.Join(errors.New("accepted final hydration item authority is invalid"), err)
		}
		matches++
		matched = contracts.CloneMap(item)
	}
	return matched, matches, nil
}

func sameAcceptedFinalHydrationJSONV1(left, right any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func turnItemsClaimAcceptedFinalV1(turn map[string]any) bool {
	items, _ := turn["items"].([]any)
	for _, value := range items {
		item, _ := value.(map[string]any)
		if _, present := item["acceptedFinal"]; present {
			return true
		}
		if _, present := item["acceptedFinalView"]; present {
			return true
		}
	}
	return false
}

func latestDurableAcceptedFinalManifestV1(
	threadID string,
	events []map[string]any,
) ([]map[string]any, bool, int, error) {
	manifests, latestSeq, err := durableAcceptedFinalManifestsV1(threadID, events)
	if err != nil || len(manifests) == 0 {
		return nil, false, latestSeq, err
	}
	return manifests[len(manifests)-1].Events, true, latestSeq, nil
}

func durableAcceptedFinalManifestsV1(
	threadID string,
	events []map[string]any,
) ([]durableAcceptedFinalManifestV1, int, error) {
	manifests := make([]durableAcceptedFinalManifestV1, 0)
	latestSeq := 0
	for index, event := range events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq != index+1 || contracts.StringField(event, "threadId") != threadID {
			return nil, 0, errors.New("accepted final hydration event frontier is invalid")
		}
		latestSeq = seq
	}
	seenCommits := map[string]struct{}{}
	for start := 0; start < len(events); {
		event := events[start]
		commitID := strings.TrimSpace(contracts.StringField(event, "publicationCommitId"))
		if commitID == "" {
			if domainevent.ContainsAcceptedFinalPublicationAuthority(event) {
				return nil, 0, errors.New("accepted final hydration durable marker is incomplete")
			}
			start++
			continue
		}
		if _, duplicate := seenCommits[commitID]; duplicate {
			return nil, 0, errors.New("accepted final hydration durable manifest is duplicated")
		}
		seenCommits[commitID] = struct{}{}
		end := start + 1
		for end < len(events) &&
			strings.TrimSpace(contracts.StringField(events[end], "publicationCommitId")) == commitID {
			end++
		}
		group := make([]map[string]any, 0, end-start)
		for _, current := range events[start:end] {
			group = append(group, contracts.CloneMap(current))
		}
		v1Err := domainevidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1(group)
		v2Err := domainevent.ValidateAcceptedFinalDeliveryEventsV2(group)
		if (v1Err == nil) == (v2Err == nil) {
			return nil, 0, errors.New("accepted final hydration durable manifest is invalid")
		}
		manifest := durableAcceptedFinalManifestV1{Events: group}
		if v1Err == nil {
			manifest.WireSchemaVersion = domainevent.AcceptedFinalDeliveryBatchV1Version
			manifest.WirePurpose = domainevent.AcceptedFinalDeliveryBatchV1Purpose
		} else {
			manifest.WireSchemaVersion = domainevent.AcceptedFinalDeliveryBatchV2Version
			manifest.WirePurpose = domainevent.AcceptedFinalDeliveryBatchV2Purpose
		}
		manifests = append(manifests, manifest)
		if len(manifests) > domainevent.AcceptedFinalDeliveryGroupLimitV1 {
			return nil, 0, errors.New("accepted final hydration durable delivery count exceeds the public bound")
		}
		start = end
	}
	return manifests, latestSeq, nil
}

func validateHistoricalAcceptedFinalManifestRecordV1(events []map[string]any) error {
	if err := domainevidence.ValidateHistoricalAcceptedFinalDeliveryEventsV1(events); err != nil {
		return err
	}
	item, _ := events[0]["item"].(map[string]any)
	record, err := domainevidence.ParseAcceptedFinalRecord(item["acceptedFinal"])
	if err != nil || record.ThreadID != contracts.StringField(events[0], "threadId") ||
		record.TurnID != contracts.StringField(events[0], "turnId") ||
		record.RecordDigest != contracts.StringField(events[0], "publicationCommitId") {
		return errors.Join(errors.New("historical accepted final hydration record is invalid"), err)
	}
	return nil
}

func historicalAcceptedFinalManifestMatchesAuthorityV1(
	authorityThread map[string]any,
	events []map[string]any,
) error {
	if err := validateHistoricalAcceptedFinalManifestRecordV1(events); err != nil {
		return err
	}
	turnID := contracts.StringField(events[0], "turnId")
	item, _ := events[0]["item"].(map[string]any)
	manifestRecord, _ := domainevidence.ParseAcceptedFinalRecord(item["acceptedFinal"])
	turns, _ := authorityThread["turns"].([]any)
	matches := 0
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if contracts.StringField(turn, "id") != turnID {
			continue
		}
		authorityRecord, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
		if err != nil || !reflect.DeepEqual(authorityRecord, manifestRecord) {
			return errors.Join(errors.New("historical accepted final hydration authority is detached"), err)
		}
		matches++
	}
	if matches != 1 {
		return errors.New("historical accepted final hydration authority is missing or duplicated")
	}
	return nil
}
