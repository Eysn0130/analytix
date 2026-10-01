package thread

import (
	"encoding/json"
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

type HydrateSidecarInput struct {
	ThreadID     string
	Thread       map[string]any
	Items        []map[string]any
	FallbackTime string
}

func LatestMetadataSidecarThread(threadID string, entries []map[string]any) map[string]any {
	threadID = strings.TrimSpace(threadID)
	var latest map[string]any
	for _, entry := range entries {
		if stringField(entry, "kind") != "thread_metadata" {
			continue
		}
		thread, _ := entry["thread"].(map[string]any)
		if thread == nil {
			continue
		}
		id := strings.TrimSpace(stringField(thread, "id"))
		if id != "" && id != threadID {
			continue
		}
		latest = contracts.CloneMap(thread)
	}
	return latest
}

func LatestMessageSidecarItems(rawItems []map[string]any) []map[string]any {
	latestByID := map[string]map[string]any{}
	ordered := []map[string]any{}
	for _, item := range rawItems {
		if !sidecarItemAllowedV1(item) {
			continue
		}
		itemID := strings.TrimSpace(stringField(item, "id"))
		if itemID == "" {
			continue
		}
		cloned := contracts.CloneMap(item)
		latestByID[itemID] = cloned
		ordered = append(ordered, cloned)
	}
	seen := map[string]bool{}
	items := []map[string]any{}
	for index := len(ordered) - 1; index >= 0; index-- {
		id := stringField(ordered[index], "id")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		items = append([]map[string]any{contracts.CloneMap(latestByID[id])}, items...)
	}
	return items
}

// SidecarValueHasPrivateTerminalAuthority treats metadata/messages sidecars as
// untrusted rendering aids. They may never create or restore either case
// publication authority or the private ordinary terminal CAS/outbox namespace.
// Only object keys are inspected for case authority, so ordinary user text is
// never interpreted as a security contract.
func SidecarValueHasPrivateTerminalAuthority(value any) bool {
	return domainevent.ContainsPrivateTerminalAuthority(value) || sidecarValueHasCasePublicationAuthority(value)
}

func sidecarValueHasCasePublicationAuthority(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch normalizeSidecarAuthorityKey(key) {
			case "acceptedfinal", "acceptedfinalview", "acceptedfinaldigest", "publicationcommitid", "publicationeventid", "publicationpayloaddigest":
				if child != nil {
					return true
				}
			}
			if sidecarValueHasCasePublicationAuthority(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if sidecarValueHasCasePublicationAuthority(child) {
				return true
			}
		}
	}
	return false
}

func normalizeSidecarAuthorityKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	var normalized strings.Builder
	for index := 0; index < len(key); index++ {
		character := key[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			normalized.WriteByte(character)
		}
	}
	return normalized.String()
}

func MetadataSidecarEntry(thread map[string]any, now string) map[string]any {
	thread = SanitizePublicHistory(thread)
	id := strings.TrimSpace(stringField(thread, "id"))
	if id == "" {
		return nil
	}
	entry := map[string]any{
		"kind":      "thread_metadata",
		"version":   float64(1),
		"timestamp": strings.TrimSpace(now),
		"thread":    StripItemsForSidecar(thread),
		"summary":   SidecarSummary(thread),
	}
	if domainevent.ValidatePublicRecord(entry) != nil {
		return nil
	}
	return entry
}

func MessageSidecarJSONByID(items []map[string]any) (map[string]string, error) {
	existing := map[string]string{}
	for _, item := range items {
		itemID := strings.TrimSpace(stringField(item, "id"))
		if itemID == "" {
			continue
		}
		data, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		existing[itemID] = string(data)
	}
	return existing, nil
}

func MissingMessageSidecarItems(thread map[string]any, existingJSONByID map[string]string) ([]map[string]any, map[string]string, error) {
	if existingJSONByID == nil {
		existingJSONByID = map[string]string{}
	}
	if ThreadHasCaseAuthorityMarkers(thread) {
		return []map[string]any{}, existingJSONByID, nil
	}
	projected, err := ProjectPublicThread(thread)
	if err != nil {
		return nil, nil, err
	}
	out := []map[string]any{}
	for _, item := range ThreadItemsInOrder(projected) {
		if !sidecarItemAllowedV1(item) {
			continue
		}
		item, ok := SanitizePublicItem(item)
		if !ok {
			continue
		}
		itemID := strings.TrimSpace(stringField(item, "id"))
		if itemID == "" {
			continue
		}
		data, err := json.Marshal(item)
		if err != nil {
			return nil, nil, err
		}
		if existingJSONByID[itemID] == string(data) {
			continue
		}
		out = append(out, contracts.CloneMap(item))
		existingJSONByID[itemID] = string(data)
	}
	return out, existingJSONByID, nil
}

var ErrDurableHistorySanitization = errors.New("durable thread history failed closed sanitation")

func NormalizeForRead(threadID string, thread map[string]any, now string) (map[string]any, error) {
	if thread == nil {
		return nil, nil
	}
	out, err := SanitizeDurableHistory(thread)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(stringField(out, "id")) == "" {
		out["id"] = strings.TrimSpace(threadID)
	}
	if _, ok := out["turns"].([]any); !ok {
		out["turns"] = []any{}
	}
	if strings.TrimSpace(stringField(out, "title")) == "" || ShouldAutoTitleThread(out) {
		if title := TitleFromThread(out); title != "" {
			out["title"] = title
		} else if strings.TrimSpace(stringField(out, "title")) == "" {
			out["title"] = "New thread"
		}
	}
	if strings.TrimSpace(stringField(out, "workspace")) == "" {
		out["workspace"] = ""
	}
	if strings.TrimSpace(stringField(out, "model")) == "" {
		delete(out, "model")
	}
	if strings.TrimSpace(stringField(out, "mode")) == "" {
		out["mode"] = "agent"
	}
	if strings.TrimSpace(stringField(out, "status")) == "" {
		out["status"] = "idle"
	}
	if strings.TrimSpace(stringField(out, "approvalPolicy")) == "" {
		out["approvalPolicy"] = defaultApprovalPolicy
	}
	if strings.TrimSpace(stringField(out, "sandboxMode")) == "" {
		out["sandboxMode"] = defaultSandboxMode
	}
	if strings.TrimSpace(stringField(out, "relation")) == "" {
		out["relation"] = "primary"
	}
	now = strings.TrimSpace(now)
	if strings.TrimSpace(stringField(out, "createdAt")) == "" {
		out["createdAt"] = now
	}
	if strings.TrimSpace(stringField(out, "updatedAt")) == "" {
		out["updatedAt"] = stringField(out, "createdAt")
	}
	return out, nil
}

func SanitizeDurableHistory(thread map[string]any) (map[string]any, error) {
	if thread == nil {
		return nil, nil
	}
	private, ok := domainevent.SanitizeDurableValue(thread, false)
	if !ok {
		return nil, ErrDurableHistorySanitization
	}
	out, ok := private.(map[string]any)
	if !ok || out == nil {
		return nil, ErrDurableHistorySanitization
	}
	out["turns"] = durableTurnHistory(listAny(out["turns"]))
	return out, nil
}

func SanitizePublicHistory(thread map[string]any) map[string]any {
	if thread == nil {
		return nil
	}
	out, err := ProjectPublicThread(thread)
	if err != nil || out == nil {
		return map[string]any{}
	}
	return contracts.CloneMap(out)
}

func ValidatePublicHistory(thread map[string]any) error {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	if err != nil {
		return err
	}
	// The exact workspace path is host execution authority for every thread.
	// Keep it byte-stable in durable state and validate only ordinary content;
	// TrustedPublicProjector applies the public workspace projection later.
	ordinaryContent := contracts.CloneMap(thread)
	delete(ordinaryContent, "workspace")
	if rawArchive, present := ordinaryContent[domainturnterminal.GeneralTerminalPublicationArchiveFieldV1]; present {
		archive, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(rawArchive)
		if err != nil {
			return err
		}
		// The archive has a closed, digest-bound schema and can grow with
		// history. Validate each entry against the same public record policy
		// before applying the fixed limits to ordinary content below.
		for _, commit := range archive.Commits {
			if err := domainevent.ValidatePublicRecord(domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit)); err != nil {
				return err
			}
		}
		delete(ordinaryContent, domainturnterminal.GeneralTerminalPublicationArchiveFieldV1)
	}
	if caseSensitive {
		if err := validatePublicThreadRecordChunksV1(ordinaryContent); err != nil {
			return errors.Join(errors.New("case thread ordinary projection is unsafe"), err)
		}
		return nil
	}
	if err := validatePublicThreadRecordChunksV1(ordinaryContent); err != nil {
		return err
	}
	return nil
}

// Each turn and the root metadata are independently bounded public records.
// Applying one credential budget to the accumulated thread would reject valid
// history after enough turns, while a malformed or unsafe individual record
// must continue to fail closed.
func validatePublicThreadRecordChunksV1(thread map[string]any) error {
	turns, ok := thread["turns"].([]any)
	if !ok {
		return errors.New("public thread turns are invalid")
	}
	root := make(map[string]any, len(thread)-1)
	for key, value := range thread {
		if key != "turns" {
			root[key] = value
		}
	}
	if err := domainevent.ValidatePublicRecord(root); err != nil {
		return err
	}
	for _, rawTurn := range turns {
		turn, ok := rawTurn.(map[string]any)
		if !ok || turn == nil {
			return errors.New("public thread turn is invalid")
		}
		if err := domainevent.ValidatePublicRecord(turn); err != nil {
			return err
		}
	}
	return nil
}

func publicTurnHistory(turns []any) []any {
	out := make([]any, 0, len(turns))
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		clonedTurn := contracts.CloneMap(turn)
		items := []any{}
		for _, rawItem := range listAny(clonedTurn["items"]) {
			item, _ := rawItem.(map[string]any)
			if publicItem, ok := SanitizePublicItem(item); ok {
				items = append(items, publicItem)
			}
		}
		clonedTurn["items"] = items
		out = append(out, clonedTurn)
	}
	return out
}

func durableTurnHistory(turns []any) []any {
	out := make([]any, 0, len(turns))
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		clonedTurn := contracts.CloneMap(turn)
		items := []any{}
		for _, rawItem := range listAny(clonedTurn["items"]) {
			item, _ := rawItem.(map[string]any)
			private, ok := domainevent.SanitizeDurableValue(item, false)
			if !ok {
				continue
			}
			privateItem, _ := private.(map[string]any)
			if privateItem == nil || (strings.TrimSpace(stringField(privateItem, "kind")) == "assistant_text" && strings.TrimSpace(stringField(privateItem, "text")) == "") {
				continue
			}
			items = append(items, privateItem)
		}
		clonedTurn["items"] = items
		out = append(out, clonedTurn)
	}
	return out
}

func SanitizePublicItem(item map[string]any) (map[string]any, bool) {
	if item == nil {
		return nil, false
	}
	public, ok := domainevent.SanitizePublicValue(item, false)
	if !ok {
		return nil, false
	}
	out, _ := public.(map[string]any)
	if out == nil || (strings.TrimSpace(stringField(out, "kind")) == "assistant_text" && strings.TrimSpace(stringField(out, "text")) == "") {
		return nil, false
	}
	if err := domainevent.ValidatePublicRecord(out); err != nil {
		return nil, false
	}
	return out, true
}

func ThreadNeedsSidecarHydration(thread map[string]any) bool {
	if ThreadHasCaseAuthorityMarkers(thread) {
		return false
	}
	turns := listAny(thread["turns"])
	if len(turns) == 0 {
		return true
	}
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		if len(listAny(turn["items"])) == 0 {
			return true
		}
	}
	return false
}

// ThreadHasCaseAuthorityMarkers identifies primary state that must never be
// reconstructed from metadata/messages sidecars. A malformed security marker
// is treated as sensitive so normalization cannot turn corruption into a
// sidecar-repair opportunity.
func ThreadHasCaseAuthorityMarkers(thread map[string]any) bool {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	return err != nil || caseSensitive
}

func ShouldReplacePlaceholderThread(relativePath string, source map[string]any, target map[string]any) bool {
	if relativePath != "thread.json" {
		return false
	}
	sourceID := strings.TrimSpace(stringField(source, "id"))
	targetID := strings.TrimSpace(stringField(target, "id"))
	if sourceID != "" && targetID != "" && sourceID != targetID {
		return false
	}
	return IsPlaceholderThread(target) && HasRecoverableThread(source)
}

func IsPlaceholderThread(thread map[string]any) bool {
	title := strings.TrimSpace(stringField(thread, "title"))
	return (title == "" || title == "New thread") && len(listAny(thread["turns"])) == 0
}

func HasRecoverableThread(thread map[string]any) bool {
	if len(listAny(thread["turns"])) > 0 {
		return true
	}
	title := strings.TrimSpace(stringField(thread, "title"))
	return title != "" && title != "New thread"
}

func HydrateSidecarItems(input HydrateSidecarInput) map[string]any {
	hydrated := contracts.CloneMap(input.Thread)
	if len(input.Items) == 0 || ThreadHasCaseAuthorityMarkers(hydrated) {
		return hydrated
	}
	removedTurnIDs := rewindRemovedTurnIDs(hydrated)
	itemsByTurn := map[string][]map[string]any{}
	for _, item := range input.Items {
		if !sidecarItemAllowedV1(item) {
			continue
		}
		turnID := strings.TrimSpace(stringField(item, "turnId"))
		if turnID == "" || removedTurnIDs[turnID] {
			continue
		}
		itemsByTurn[turnID] = append(itemsByTurn[turnID], contracts.CloneMap(item))
	}
	turns := []any{}
	primaryTurns := listAny(hydrated["turns"])
	for _, rawTurn := range primaryTurns {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		cloned := contracts.CloneMap(turn)
		turnID := stringField(cloned, "id")
		turnItems := mapsToAny(itemsByTurn[turnID])
		if len(turnItems) > 0 && len(listAny(cloned["items"])) == 0 {
			candidate := contracts.CloneMap(cloned)
			candidate["items"] = turnItems
			if sidecarTurnHasCaseAuthority(strings.TrimSpace(input.ThreadID), candidate) {
				turns = append(turns, cloned)
				continue
			}
			cloned = candidate
			if strings.TrimSpace(stringField(cloned, "prompt")) == "" {
				if prompt := PromptFromTurnItems(turnItems); prompt != "" {
					cloned["prompt"] = prompt
				}
			}
			if len(listAny(cloned["attachmentIds"])) == 0 {
				cloned["attachmentIds"] = AttachmentIDsFromItems(turnItems)
			}
		}
		turns = append(turns, cloned)
	}
	hydrated["turns"] = turns
	return hydrated
}

// SidecarItemsHaveCaseAuthority reports whether messages.jsonl attempts to
// carry accepted-final/publication authority. Sidecars are a rendering aid;
// they may never create or restore case authority, including for a turn that
// is absent from the primary record.
func SidecarItemsHaveCaseAuthority(threadID string, items []map[string]any) bool {
	for _, item := range items {
		turnID := strings.TrimSpace(stringField(item, "turnId"))
		candidate := map[string]any{
			"id":       turnID,
			"threadId": strings.TrimSpace(threadID),
			"items":    []any{contracts.CloneMap(item)},
		}
		if sidecarTurnHasCaseAuthority(strings.TrimSpace(threadID), candidate) {
			return true
		}
	}
	return false
}

func sidecarTurnHasCaseAuthority(threadID string, turn map[string]any) bool {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(map[string]any{
		"id":    strings.TrimSpace(threadID),
		"turns": []any{turn},
	})
	return err != nil || caseSensitive
}

func rewindRemovedTurnIDs(thread map[string]any) map[string]bool {
	removed := map[string]bool{}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "kind") != "rewind_transition" {
			continue
		}
		for _, rawID := range listAny(turn["removedTurnIds"]) {
			if id, _ := rawID.(string); strings.TrimSpace(id) != "" {
				removed[strings.TrimSpace(id)] = true
			}
		}
	}
	return removed
}

func StripItemsForSidecar(thread map[string]any) map[string]any {
	stripped := contracts.CloneMap(thread)
	turns := []any{}
	for _, rawTurn := range listAny(stripped["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		cloned := contracts.CloneMap(turn)
		cloned["prompt"] = ""
		cloned["items"] = []any{}
		turns = append(turns, cloned)
	}
	stripped["turns"] = turns
	return stripped
}

func SidecarSummary(thread map[string]any) map[string]any {
	return map[string]any{
		"schemaVersion": float64(1),
		"preview":       sidecarUserPreviewV1(thread),
		"messageCount":  float64(contracts.CountThreadItems(thread)),
		"turnCount":     float64(CountTurns(thread)),
	}
}

func sidecarItemAllowedV1(item map[string]any) bool {
	if item == nil {
		return false
	}
	switch strings.TrimSpace(stringField(item, "kind")) {
	case "assistant_text", "assistant_reasoning", "content_redacted", "error":
		return false
	case "compaction":
		return domainevent.ValidGeneralCompactionProviderHistoryItemV3(item)
	default:
		_, allowed := projectOrdinaryPublicHistoryItemV1(item)
		return allowed
	}
}

// ProjectLegacyOrdinaryHistoryItemForMigrationV1 applies the same closed item
// projection used by ordinary public history. It exists only for the staged
// semantic migration: callers must first prove that the containing thread has
// no current execution or publication authority. A rejected item must never be
// repaired by inventing a host tool-call identity or lifecycle state.
func ProjectLegacyOrdinaryHistoryItemForMigrationV1(item map[string]any) (map[string]any, bool) {
	projected, ok := projectOrdinaryPublicHistoryItemV1(item)
	if !ok || projected == nil {
		return nil, false
	}
	return contracts.CloneMap(projected), true
}

// ProjectLegacyOrdinarySidecarItemForMigrationV1 is the closed messages.jsonl
// counterpart. Assistant drafts, reasoning, migration tombstones, errors, and
// every lifecycle rejected by the ordinary projector remain non-hydratable.
func ProjectLegacyOrdinarySidecarItemForMigrationV1(item map[string]any) (map[string]any, bool) {
	if !sidecarItemAllowedV1(item) {
		return nil, false
	}
	return ProjectLegacyOrdinaryHistoryItemForMigrationV1(item)
}

func sidecarUserPreviewV1(thread map[string]any) string {
	turns := listAny(thread["turns"])
	for turnIndex := len(turns) - 1; turnIndex >= 0; turnIndex-- {
		turn, _ := turns[turnIndex].(map[string]any)
		items := listAny(turn["items"])
		for itemIndex := len(items) - 1; itemIndex >= 0; itemIndex-- {
			item, _ := items[itemIndex].(map[string]any)
			if stringField(item, "kind") != "user_message" {
				continue
			}
			text := strings.Join(strings.Fields(stringField(item, "text")), " ")
			if len(text) > 160 {
				return text[:160]
			}
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func ThreadItemsInOrder(thread map[string]any) []map[string]any {
	items := []map[string]any{}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if turn == nil {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if item == nil {
				continue
			}
			items = append(items, contracts.CloneMap(item))
		}
	}
	return items
}

func TurnFromSidecarItems(threadID string, turnID string, items []map[string]any, fallbackTime string) map[string]any {
	now := strings.TrimSpace(fallbackTime)
	createdAt := now
	if len(items) > 0 && strings.TrimSpace(stringField(items[0], "createdAt")) != "" {
		createdAt = stringField(items[0], "createdAt")
	}
	finishedAt := now
	if len(items) > 0 && strings.TrimSpace(stringField(items[len(items)-1], "finishedAt")) != "" {
		finishedAt = stringField(items[len(items)-1], "finishedAt")
	}
	status := "completed"
	for _, item := range items {
		itemStatus := stringField(item, "status")
		if itemStatus == "pending" || itemStatus == "running" {
			status = "running"
			finishedAt = ""
			break
		}
		if itemStatus == "failed" || itemStatus == "aborted" {
			status = "failed"
		}
	}
	turn := map[string]any{
		"id":                strings.TrimSpace(turnID),
		"threadId":          strings.TrimSpace(threadID),
		"status":            status,
		"prompt":            firstNonEmptyString(PromptFromMapItems(items), "Turn "+strings.TrimSpace(turnID)),
		"steering":          []any{},
		"attachmentIds":     AttachmentIDsFromItems(mapsToAny(items)),
		"activeSkillIds":    []any{},
		"injectedMemoryIds": []any{},
		"createdAt":         createdAt,
		"items":             mapsToAny(items),
	}
	if finishedAt != "" {
		turn["finishedAt"] = finishedAt
	}
	return turn
}

func PromptFromTurnItems(items []any) string {
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if item != nil && stringField(item, "kind") == "user_message" {
			return stringField(item, "text")
		}
	}
	return ""
}

func PromptFromMapItems(items []map[string]any) string {
	for _, item := range items {
		if stringField(item, "kind") == "user_message" {
			return stringField(item, "text")
		}
	}
	return ""
}

func mapsToAny(items []map[string]any) []any {
	values := make([]any, 0, len(items))
	for _, item := range items {
		values = append(values, contracts.CloneMap(item))
	}
	return values
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
