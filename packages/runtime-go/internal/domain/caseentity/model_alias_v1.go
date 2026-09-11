package caseentity

import (
	"errors"
	"strconv"
	"strings"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
)

const (
	ModelEntityAccountAliasPrefixV1 = "acct:"
	ModelEntityCardAliasPrefixV1    = "card:"
)

// ModelEntityAliasV1 is a provider-safe selector for one account or card in
// the frozen current case. It is not an authority reference, evidence
// identity, reverse mapping, or cross-case identifier.
type ModelEntityAliasV1 string

func NewModelEntityAliasV1(entityType string, ordinal uint32) (ModelEntityAliasV1, error) {
	prefix, err := modelEntityAliasPrefixV1(entityType)
	if err != nil || ordinal == 0 {
		return "", errors.New("case entity model alias is invalid")
	}
	alias := ModelEntityAliasV1(prefix + strconv.FormatUint(uint64(ordinal), 10))
	if _, _, err := ParseModelEntityAliasV1(string(alias)); err != nil {
		return "", errors.New("case entity model alias is invalid")
	}
	return alias, nil
}

func ParseModelEntityAliasV1(value string) (string, uint32, error) {
	if value == "" || value != strings.TrimSpace(value) {
		return "", 0, errors.New("case entity model alias is invalid")
	}
	var entityType, ordinalText string
	switch {
	case strings.HasPrefix(value, ModelEntityAccountAliasPrefixV1):
		entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
		ordinalText = strings.TrimPrefix(value, ModelEntityAccountAliasPrefixV1)
	case strings.HasPrefix(value, ModelEntityCardAliasPrefixV1):
		entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
		ordinalText = strings.TrimPrefix(value, ModelEntityCardAliasPrefixV1)
	default:
		return "", 0, errors.New("case entity model alias is invalid")
	}
	ordinal, err := strconv.ParseUint(ordinalText, 10, 32)
	if err != nil || ordinal == 0 || strconv.FormatUint(ordinal, 10) != ordinalText {
		return "", 0, errors.New("case entity model alias is invalid")
	}
	return entityType, uint32(ordinal), nil
}

func ValidateModelEntityAliasV1(value string) error {
	_, _, err := ParseModelEntityAliasV1(value)
	return err
}

func modelEntityAliasPrefixV1(entityType string) (string, error) {
	switch entityType {
	case domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1:
		return ModelEntityAccountAliasPrefixV1, nil
	case domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1:
		return ModelEntityCardAliasPrefixV1, nil
	default:
		return "", errors.New("case entity model alias type is unsupported")
	}
}
