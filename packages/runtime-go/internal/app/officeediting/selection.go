package officeediting

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	ordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

var nativeSelectionToken = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var nativeThread = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var nativePlaceholder = regexp.MustCompile(`(?i)(?:PERSON|ACCOUNT|PHONE|EMAIL|ADDRESS|IDENTITY|ID_CARD|BANK_CARD|COMPANY|ORGANIZATION|DEVICE|IP|MAC|NUMBER)_[0-9]+`)

// The native replacement protocol counts JavaScript UTF-16 units, not bytes or
// Unicode code points. Discussion captures retain their separate 64 KiB bound.
const MaxNativeSelectionUTF16 = 4096

func nativeUTF16Length(text string) int {
	units := 0
	for _, character := range text {
		units++
		if character > 0xffff {
			units++
		}
	}
	return units
}

type nativeSession struct {
	document  editingapp.Opened
	principal identitydomain.PrincipalV1
	workspace string
	release   func()
}
type nativeScope struct {
	ID, SessionID, ThreadID, SelectionToken, BaseRevision string
	Sequence                                              int64
	Editable                                              bool
	revoked                                               bool
	proposalOrder                                         []string
	Binding                                               adapterport.Binding
	Parts                                                 []editingapp.PatchPart
	protected                                             []editingapp.PatchPart // Text is private and never serialized.
	proposals                                             map[string]*nativeProposal
	operations                                            map[string]string
}
type nativeProposal struct {
	ID, Status                        string
	Parts                             []editingapp.PatchPart
	digest                            string
	operationID                       string
	decisionDigest, decisionOperation string
}

// BindSelectionHost is trusted startup composition. Neither Main nor a model can
// inject projection or mutation authority. It must run before any object opens.
func (a *Adapter) BindSelectionHost(projector editingapp.TrustedSelectionProjector, capture func(context.Context, string, string, func() error) (func(), error)) error {
	if a == nil || projector == nil || capture == nil {
		return ErrUnavailable
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.sessions) != 0 || a.projector != nil {
		return ErrBinding
	}
	a.projector, a.capture = projector, capture
	return nil
}

func nativeID() (string, error) {
	var raw [48]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", ErrUnavailable
	}
	// Letter-only opaque IDs survive ordinary PII projection unchanged. Forty-
	// eight independent symbols retain over 120 bits of entropy.
	for i := range raw {
		raw[i] = 'a' + raw[i]%6
	}
	return string(raw[:]), nil
}
func nativeLiteral(text string) bool {
	if !utf8.ValidString(text) || len(text) > editingapp.MaxPatchBytes || nativePlaceholder.MatchString(text) || strings.Contains(strings.ToLower(text), "protected_") {
		return false
	}
	for _, c := range text {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
	}
	return strings.TrimSpace(text) == "" || ordinary.ProjectTextV1(text) == text
}
func nativeParts(parts []editingapp.PatchPart) []editingapp.PatchPart {
	return append([]editingapp.PatchPart{}, parts...)
}
func (a *Adapter) selectionAuthority(session *nativeSession, scope *nativeScope) editingapp.ScopeAuthority {
	purpose := "discuss"
	if scope.Editable {
		purpose = "edit"
	}
	return editingapp.ScopeAuthority{Principal: session.principal, ObjectID: session.document.ObjectID, ThreadID: scope.ThreadID, Purpose: purpose, Workspace: session.workspace, Path: session.document.Path}
}
func (a *Adapter) validateScope(ctx context.Context, scope *nativeScope, principal identitydomain.PrincipalV1) error {
	session := a.sessions[scope.SessionID]
	if session == nil || !identitydomain.SamePrincipalV1(session.principal, principal) {
		return editingapp.ErrSession
	}
	if a.projector == nil || a.projector.ValidateCurrent(ctx, a.selectionAuthority(session, scope)) != nil {
		return editingapp.ErrProjection
	}
	// Validate disk revision without inventing a second filesystem authority.
	doc, err := a.service.Open(ctx, session.workspace, session.document.Path)
	if err != nil || doc.SessionID != scope.SessionID || doc.Revision != scope.BaseRevision {
		return editingapp.ErrDraftStale
	}
	return nil
}
func scopeOutput(scope *nativeScope) map[string]any {
	return map[string]any{"scopeId": scope.ID, "sessionId": scope.SessionID, "threadId": scope.ThreadID, "selectionToken": scope.SelectionToken, "baseRevision": scope.BaseRevision, "changeSequence": scope.Sequence, "editable": scope.Editable, "parts": nativeParts(scope.Parts)}
}
func proposalOutput(proposal *nativeProposal) map[string]any {
	return map[string]any{"proposalId": proposal.ID, "status": proposal.Status, "parts": nativeParts(proposal.Parts)}
}
func (a *Adapter) invalidateSelection(sessionID string) {
	for id, scope := range a.scopes {
		if scope.SessionID == sessionID {
			delete(a.scopes, id)
		}
	}
}
func (a *Adapter) closeSelection(sessionID string) {
	a.invalidateSelection(sessionID)
	if session := a.sessions[sessionID]; session != nil && session.release != nil {
		session.release()
	}
	delete(a.sessions, sessionID)
}

func (a *Adapter) invokeSelection(ctx context.Context, call adapterport.Call, input map[string]any) (adapterport.Result, error) {
	if call.Operation == "model-selection-read" || call.Operation == "model-selection-propose" {
		scopeID, _ := input["scopeId"].(string)
		thread, _ := input["threadId"].(string)
		operation := ""
		var parts []editingapp.PatchPart
		if call.Operation == "model-selection-propose" {
			if !exactKeys(input, "scopeId", "threadId", "operationId", "parts") {
				return failure(fileport.ErrInvalidInput)
			}
			operation, _ = input["operationId"].(string)
			if !operationPattern.MatchString(operation) {
				return failure(fileport.ErrInvalidInput)
			}
			var err error
			parts, err = decodeNativeParts(input["parts"])
			if err != nil {
				return failure(err)
			}
		} else if !exactKeys(input, "scopeId", "threadId") {
			return failure(fileport.ErrInvalidInput)
		}
		result, err := a.modelSelectionLocked(ctx, call.Principal, thread, call.Binding, scopeID, operation, parts)
		if err != nil {
			return failure(err)
		}
		return output(map[string]any{"ok": true, "result": result})
	}
	sessionID, _ := input["sessionId"].(string)
	session := a.sessions[sessionID]
	if session == nil || !identitydomain.SamePrincipalV1(session.principal, call.Principal) {
		return failure(editingapp.ErrSession)
	}
	if call.Operation == "capture-selection" {
		if !exactKeys(input, "sessionId", "threadId", "selectionToken", "changeSequence", "baseRevision", "text", "editable") {
			return failure(fileport.ErrInvalidInput)
		}
		thread, _ := input["threadId"].(string)
		token, _ := input["selectionToken"].(string)
		revision, _ := input["baseRevision"].(string)
		text, ok := input["text"].(string)
		editable, editableOK := input["editable"].(bool)
		number, numberOK := input["changeSequence"].(json.Number)
		sequence, err := number.Int64()
		if !nativeThread.MatchString(thread) || !nativeSelectionToken.MatchString(token) || !revisionPattern.MatchString(revision) || !ok || len(text) > editingapp.MaxSelectionBytes || !utf8.ValidString(text) || !editableOK || !numberOK || err != nil || sequence < 0 || sequence > 9007199254740991 {
			return failure(fileport.ErrInvalidInput)
		}
		if editable && nativeUTF16Length(text) > MaxNativeSelectionUTF16 {
			return failure(editingapp.ErrProposal)
		}
		scope := &nativeScope{SessionID: sessionID, ThreadID: thread, SelectionToken: token, BaseRevision: revision, Sequence: sequence, Editable: editable, Binding: call.Binding, proposals: map[string]*nativeProposal{}, operations: map[string]string{}}
		if err := a.validateScope(ctx, scope, call.Principal); err != nil {
			return failure(err)
		}
		ranges, err := a.projector.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: a.selectionAuthority(session, scope), Text: text})
		if err != nil || len(ranges) > editingapp.MaxProtectedSpans {
			return failure(editingapp.ErrProjection)
		}
		scope.ID, err = nativeID()
		if err != nil {
			return failure(err)
		}
		cursor := 0
		addLiteral := func(value string) bool {
			if value == "" {
				return true
			}
			if !nativeLiteral(value) {
				return false
			}
			scope.Parts = append(scope.Parts, editingapp.PatchPart{Kind: "literal", Text: strings.Clone(value)})
			return true
		}
		for _, span := range ranges {
			if span.StartByte < cursor || span.EndByte <= span.StartByte || span.EndByte > len(text) || !utf8.ValidString(text[:span.StartByte]) || !utf8.ValidString(text[:span.EndByte]) || !addLiteral(text[cursor:span.StartByte]) {
				return failure(editingapp.ErrProjection)
			}
			id, err := nativeID()
			if err != nil {
				return failure(err)
			}
			part := editingapp.PatchPart{Kind: "protected", ProtectedRef: "protected_" + id}
			scope.Parts = append(scope.Parts, part)
			part.Text = strings.Clone(text[span.StartByte:span.EndByte])
			scope.protected = append(scope.protected, part)
			cursor = span.EndByte
		}
		if !addLiteral(text[cursor:]) {
			return failure(editingapp.ErrProjection)
		}
		if _, err := renderNativeProposal(scope, scope.Parts); err != nil {
			return failure(editingapp.ErrProjection)
		}
		if editable && session.release == nil {
			if a.capture == nil {
				return failure(editingapp.ErrProjection)
			}
			release, err := a.capture(ctx, sessionID, session.document.Path, func() error {
				doc, err := a.service.Open(ctx, session.workspace, session.document.Path)
				if err != nil || doc.Revision != revision {
					return editingapp.ErrDraftStale
				}
				return nil
			})
			if err != nil {
				return failure(editingapp.ErrProjection)
			}
			session.release = release
		}
		if err := a.validateScope(ctx, scope, call.Principal); err != nil {
			return failure(err)
		}
		a.invalidateSelection(sessionID)
		a.scopes[scope.ID] = scope
		return output(map[string]any{"ok": true, "scope": scopeOutput(scope)})
	}
	scopeID, _ := input["scopeId"].(string)
	scope := a.scopes[scopeID]
	if scope == nil || scope.SessionID != sessionID || scope.Binding != call.Binding {
		return failure(editingapp.ErrScope)
	}
	if call.Operation == "selection-revoke" {
		if !exactKeys(input, "sessionId", "scopeId") {
			return failure(fileport.ErrInvalidInput)
		}
		scope.revoked = true
		scope.protected = nil
		return output(map[string]any{"ok": true, "revoked": true})
	}
	if scope.revoked && call.Operation != "proposal-read" && call.Operation != "proposal-reject" {
		return failure(editingapp.ErrScope)
	}
	if call.Operation == "proposal-reject" || call.Operation == "proposal-read" {
		if a.projector == nil || a.projector.ValidateCurrent(ctx, a.selectionAuthority(session, scope)) != nil {
			return failure(editingapp.ErrProjection)
		}
	} else if err := a.validateScope(ctx, scope, call.Principal); err != nil {
		return failure(err)
	}
	switch call.Operation {
	case "selection-read":
		if !exactKeys(input, "sessionId", "scopeId") {
			return failure(fileport.ErrInvalidInput)
		}
		return output(map[string]any{"ok": true, "scope": scopeOutput(scope)})
	case "proposal-read":
		if !exactKeys(input, "sessionId", "scopeId") {
			return failure(fileport.ErrInvalidInput)
		}
		proposals := []any{}
		for _, id := range scope.proposalOrder {
			proposals = append(proposals, proposalOutput(scope.proposals[id]))
		}
		return output(map[string]any{"ok": true, "proposals": proposals})
	case "proposal-accept", "proposal-reject":
		id, _ := input["proposalId"].(string)
		op, _ := input["operationId"].(string)
		token, revision, sequence := scope.SelectionToken, scope.BaseRevision, scope.Sequence
		if !operationPattern.MatchString(op) {
			return failure(fileport.ErrInvalidInput)
		}
		if call.Operation == "proposal-reject" {
			if !exactKeys(input, "sessionId", "scopeId", "proposalId", "operationId") {
				return failure(fileport.ErrInvalidInput)
			}
		} else {
			if !exactKeys(input, "sessionId", "scopeId", "proposalId", "operationId", "selectionToken", "changeSequence", "baseRevision") {
				return failure(fileport.ErrInvalidInput)
			}
			suppliedToken, _ := input["selectionToken"].(string)
			suppliedRevision, _ := input["baseRevision"].(string)
			number, ok := input["changeSequence"].(json.Number)
			suppliedSequence, err := number.Int64()
			if suppliedToken != token || suppliedRevision != revision || !ok || err != nil || suppliedSequence != sequence {
				return failure(editingapp.ErrDraftStale)
			}
		}
		proposal := scope.proposals[id]
		if proposal == nil {
			return failure(editingapp.ErrProposal)
		}
		encoded, _ := json.Marshal(input)
		hash := sha256.Sum256(append([]byte(call.Operation), encoded...))
		digest := hex.EncodeToString(hash[:])
		if previous, exists := scope.operations[op]; exists && previous != digest {
			return failure(fileport.ErrOperationMismatch)
		}
		if proposal.Status != "proposed" && (proposal.decisionDigest != digest || proposal.decisionOperation != op) {
			return failure(editingapp.ErrProposal)
		}
		if len(scope.operations) >= editingapp.MaxProposalOperations {
			return failure(editingapp.ErrCapacity)
		}
		if call.Operation == "proposal-reject" {
			proposal.Status = "rejected"
			proposal.decisionDigest = digest
			proposal.decisionOperation = op
			scope.operations[op] = digest
			return output(map[string]any{"ok": true, "proposal": proposalOutput(proposal)})
		}
		if !scope.Editable {
			return failure(editingapp.ErrScope)
		}
		replacement, err := renderNativeProposal(scope, proposal.Parts)
		if err != nil {
			return failure(err)
		}
		proposal.Status = "approved"
		proposal.decisionDigest = digest
		proposal.decisionOperation = op
		scope.operations[op] = digest
		return output(map[string]any{"ok": true, "replacement": map[string]any{"proposalId": id, "operationId": op, "text": replacement, "selectionToken": token, "changeSequence": sequence, "baseRevision": revision}})
	}
	return failure(fileport.ErrInvalidInput)
}

func renderNativeProposal(scope *nativeScope, parts []editingapp.PatchPart) (string, error) {
	if len(parts) > editingapp.MaxPatchParts {
		return "", editingapp.ErrProtected
	}
	var out, literals strings.Builder
	index := 0
	units := 0
	for _, part := range parts {
		switch part.Kind {
		case "literal":
			if part.Text == "" || part.ProtectedRef != "" || !nativeLiteral(part.Text) {
				return "", editingapp.ErrProtected
			}
			out.WriteString(part.Text)
			literals.WriteString(part.Text)
			units += nativeUTF16Length(part.Text)
		case "protected":
			if part.Text != "" || index >= len(scope.protected) || part.ProtectedRef != scope.protected[index].ProtectedRef {
				return "", editingapp.ErrProtected
			}
			out.WriteString(scope.protected[index].Text)
			units += nativeUTF16Length(scope.protected[index].Text)
			index++
		default:
			return "", editingapp.ErrProtected
		}
		if scope.Editable && units > MaxNativeSelectionUTF16 {
			return "", editingapp.ErrProposal
		}
		if out.Len() > editingapp.MaxPatchBytes {
			return "", editingapp.ErrProtected
		}
	}
	if index != len(scope.protected) || !nativeLiteral(literals.String()) {
		return "", editingapp.ErrProtected
	}
	for _, part := range scope.protected {
		if strings.Contains(literals.String(), part.Text) {
			return "", editingapp.ErrProtected
		}
	}
	return out.String(), nil
}

// ModelSelection is called only by the existing Harness with current principal,
// thread and package binding. Model input contains no session/path/UNO selector.
func (a *Adapter) modelSelectionLocked(ctx context.Context, principal identitydomain.PrincipalV1, threadID string, binding adapterport.Binding, scopeID, operationID string, parts []editingapp.PatchPart) (map[string]any, error) {
	scope := a.scopes[scopeID]
	if scope == nil || scope.revoked || scope.ThreadID != threadID || scope.Binding.GenerationID != binding.GenerationID || scope.Binding.ActivationRevision != binding.ActivationRevision || scope.Binding.PackageID != binding.PackageID {
		return nil, editingapp.ErrScope
	}
	if err := a.validateScope(ctx, scope, principal); err != nil {
		return nil, err
	}
	if operationID == "" {
		return map[string]any{"scopeId": scope.ID, "editable": scope.Editable, "parts": nativeParts(scope.Parts)}, nil
	}
	if !scope.Editable || !operationPattern.MatchString(operationID) {
		return nil, editingapp.ErrScope
	}
	if _, err := renderNativeProposal(scope, parts); err != nil {
		return nil, err
	}
	body, _ := json.Marshal(parts)
	hash := sha256.Sum256(body)
	digest := hex.EncodeToString(hash[:])
	for _, proposal := range scope.proposals {
		if proposal.operationID == operationID {
			if proposal.digest != digest {
				return nil, fileport.ErrOperationMismatch
			}
			return proposalOutput(proposal), nil
		}
	}
	if _, exists := scope.operations[operationID]; exists {
		return nil, fileport.ErrOperationMismatch
	}
	if len(scope.proposals) >= editingapp.MaxProposalsPerSession {
		return nil, editingapp.ErrCapacity
	}
	id, err := nativeID()
	if err != nil {
		return nil, err
	}
	proposal := &nativeProposal{ID: id, Status: "proposed", Parts: nativeParts(parts), digest: digest, operationID: operationID}
	scope.proposals[id] = proposal
	scope.proposalOrder = append(scope.proposalOrder, id)
	scope.operations[operationID] = "propose:" + digest
	return proposalOutput(proposal), nil
}

func decodeNativeParts(value any) ([]editingapp.PatchPart, error) {
	values, ok := value.([]any)
	if !ok || len(values) > editingapp.MaxPatchParts {
		return nil, fileport.ErrInvalidInput
	}
	parts := make([]editingapp.PatchPart, 0, len(values))
	total := 0
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fileport.ErrInvalidInput
		}
		kind, _ := item["kind"].(string)
		part := editingapp.PatchPart{Kind: kind}
		if kind == "literal" {
			if !exactKeys(item, "kind", "text") {
				return nil, fileport.ErrInvalidInput
			}
			part.Text, ok = item["text"].(string)
			total += len(part.Text)
		} else if kind == "protected" {
			if !exactKeys(item, "kind", "protectedRef") {
				return nil, fileport.ErrInvalidInput
			}
			part.ProtectedRef, ok = item["protectedRef"].(string)
		} else {
			return nil, fileport.ErrInvalidInput
		}
		if !ok || total > editingapp.MaxPatchBytes || len(part.ProtectedRef) > 58 {
			return nil, fileport.ErrInvalidInput
		}
		parts = append(parts, part)
	}
	return parts, nil
}
