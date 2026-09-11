package privacyprojection

import (
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// PrivateProtocolSafeHistoryV1 removes provider-private tool wire history when
// the exact thinking/reasoning bytes are unavailable. Before converting a
// host-bound case result into an untrusted user-role semantic envelope, it
// validates the existing process-local provenance and rebinds only the exact
// provider-safe semantic envelope. Authority refs remain absent in V2.
func PrivateProtocolSafeHistoryV1(
	context domainsecurity.TurnSecurityContext,
	messages []domainmodel.Message,
) ([]domainmodel.Message, error) {
	hasPrivateBinding := false
	for _, message := range messages {
		if message.PrivateProviderReferenceBinding != nil {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		if message.PrivateProviderSemanticBinding != nil {
			hasPrivateBinding = true
		}
	}
	validatedSemantics := map[int]validatedAccountFlowProviderSemanticV1{}
	if hasPrivateBinding {
		if !domainsecurity.TurnSecurityContextIsCaseSensitive(context) {
			return nil, ErrProviderPrivacyAuthorityUnavailable
		}
		var err error
		_, validatedSemantics, err = collectProviderCaseReferenceProvenanceV1(
			context,
			domainmodel.Request{Messages: messages},
			true,
		)
		if err != nil {
			return nil, err
		}
	}

	projected, origins := appmodel.PrivateProtocolSafeHistoryWithOriginsV1(
		appmodel.CloneProviderMessages(messages),
	)
	originByOutput := make(map[int]int, len(origins))
	for _, origin := range origins {
		originByOutput[origin.OutputMessageIndex] = origin.SourceMessageIndex
	}
	for index := range projected {
		message := &projected[index]
		message.PrivateProviderSemanticBinding = nil
		message.PrivateProviderReferenceBinding = nil
		if sourceIndex, ok := originByOutput[index]; ok {
			if validated, trusted := validatedSemantics[sourceIndex]; trusted {
				if validated.kind == validatedProviderSemanticSafeHistoryV1 &&
					validated.sourceOriginKind == 0 && validated.sourceCanonicalHash == "" {
					message.Content = string(validated.canonical)
				}
				if err := bindAccountFlowSafeHistoryV1(context, validated, message); err != nil {
					return nil, err
				}
				continue
			}
		}
		message.Content, _ = domaincaseentity.MaskReferenceCandidatesV1(message.Content)
		for partIndex := range message.Parts {
			message.Parts[partIndex].Text, _ = domaincaseentity.MaskReferenceCandidatesV1(
				message.Parts[partIndex].Text,
			)
		}
	}
	return projected, nil
}

func bindAccountFlowSafeHistoryV1(
	context domainsecurity.TurnSecurityContext,
	source validatedAccountFlowProviderSemanticV1,
	message *domainmodel.Message,
) error {
	if message == nil || message.PrivateProviderSemanticBinding != nil ||
		message.PrivateProviderReferenceBinding != nil ||
		message.Role != "user" || message.Name != "" || message.ToolCallID != "" ||
		len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!domainsecurity.IsSHA256Hex(source.digest) || len(source.canonical) == 0 ||
		len(source.references) != 0 || domaincaseentity.ContainsReferenceCandidateV1(message.Content) {
		return ErrProviderPrivacyAuthorityUnavailable
	}
	messageSHA256, err := providerReferenceMessageSHA256V1(*message)
	if err != nil {
		return err
	}
	originKind, sourceCanonicalHash, currentAttempt, err := safeHistorySourceBindingV1(source, message.Content)
	if err != nil {
		return err
	}
	message.PrivateProviderSemanticBinding = boundAccountFlowSafeHistoryV1{
		contextDigest:       context.ContextDigest,
		messageSHA256:       messageSHA256,
		sourceCanonicalHash: sourceCanonicalHash,
		sourceOriginKind:    originKind,
		currentAttempt:      currentAttempt,
	}
	return nil
}

func validateBoundAccountFlowSafeHistoryV1(
	context domainsecurity.TurnSecurityContext,
	message domainmodel.Message,
) (validatedAccountFlowProviderSemanticV1, error) {
	binding, ok := message.PrivateProviderSemanticBinding.(boundAccountFlowSafeHistoryV1)
	if !ok ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		binding.contextDigest != context.ContextDigest ||
		!domainsecurity.IsSHA256Hex(binding.messageSHA256) ||
		!domainsecurity.IsSHA256Hex(binding.sourceCanonicalHash) ||
		!validSafeHistoryOriginKindV1(binding.sourceOriginKind) ||
		message.Role != "user" || message.Name != "" || message.ToolCallID != "" ||
		len(message.Parts) != 0 || len(message.ToolCalls) != 0 ||
		domaincaseentity.ContainsReferenceCandidateV1(message.Content) {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	messageSHA256, err := providerReferenceMessageSHA256V1(message)
	if err != nil || messageSHA256 != binding.messageSHA256 {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	canonical := []byte(message.Content)
	digest := domainsecurity.SHA256Hex(canonical)
	if !domainsecurity.IsSHA256Hex(digest) {
		return validatedAccountFlowProviderSemanticV1{}, ErrProviderPrivacyAuthorityUnavailable
	}
	return validatedAccountFlowProviderSemanticV1{
		canonical: canonical, digest: digest, references: []domaincaseentity.ReferenceV1{},
		kind: validatedProviderSemanticSafeHistoryV1, sourceOriginKind: binding.sourceOriginKind,
		sourceCanonicalHash: binding.sourceCanonicalHash, currentAttempt: binding.currentAttempt,
	}, nil
}

func safeHistorySourceBindingV1(
	source validatedAccountFlowProviderSemanticV1,
	messageContent string,
) (validatedProviderSemanticKindV1, string, *providerCurrentAttemptUseV1, error) {
	originKind := source.kind
	sourceCanonicalHash := source.digest
	if source.kind == validatedProviderSemanticSafeHistoryV1 && source.sourceOriginKind != 0 {
		originKind = source.sourceOriginKind
		sourceCanonicalHash = source.sourceCanonicalHash
	}
	if !validSafeHistoryOriginKindV1(originKind) || !domainsecurity.IsSHA256Hex(sourceCanonicalHash) {
		return 0, "", nil, ErrProviderPrivacyAuthorityUnavailable
	}
	switch originKind {
	case validatedProviderSemanticAccountFlowV1, validatedProviderSemanticCaseForegroundV1:
		expectedTool := fundsAccountFlowToolNameV1
		if originKind == validatedProviderSemanticCaseForegroundV1 {
			expectedTool = toolcatalogapp.ForegroundTaskToolName
		}
		content, ok := appmodel.CompletedPrivateProtocolSafeHistoryContentV1(messageContent, expectedTool)
		if !ok || domainsecurity.SHA256Hex([]byte(content)) != sourceCanonicalHash {
			return 0, "", nil, ErrProviderPrivacyAuthorityUnavailable
		}
	case validatedProviderSemanticHostChildPromptV1:
		if domainsecurity.SHA256Hex([]byte(messageContent)) != sourceCanonicalHash {
			return 0, "", nil, ErrProviderPrivacyAuthorityUnavailable
		}
	case validatedProviderSemanticSafeHistoryV1:
		if source.kind != validatedProviderSemanticSafeHistoryV1 ||
			domainsecurity.SHA256Hex([]byte(messageContent)) != source.digest {
			return 0, "", nil, ErrProviderPrivacyAuthorityUnavailable
		}
	}
	requiresCurrentAttempt := originKind == validatedProviderSemanticCaseForegroundV1 ||
		originKind == validatedProviderSemanticHostChildPromptV1
	if requiresCurrentAttempt != (source.currentAttempt != nil) {
		return 0, "", nil, ErrProviderPrivacyAuthorityUnavailable
	}
	return originKind, sourceCanonicalHash, source.currentAttempt, nil
}

func validSafeHistoryOriginKindV1(kind validatedProviderSemanticKindV1) bool {
	return kind == validatedProviderSemanticAccountFlowV1 ||
		kind == validatedProviderSemanticSafeHistoryV1 ||
		kind == validatedProviderSemanticCaseForegroundV1 ||
		kind == validatedProviderSemanticHostChildPromptV1
}
