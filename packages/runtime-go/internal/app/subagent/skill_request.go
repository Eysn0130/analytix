package subagent

import (
	"encoding/hex"
	"errors"
	"strings"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type SkillTaskRequestInput struct {
	Skill  map[string]any
	Args   map[string]any
	Prompt string
}

func SkillNameFromArgs(args map[string]any) string {
	return firstNonEmptyAnyString(args["name"], args["skill"], args["skill_id"], args["skillId"])
}

func SkillRunAs(skill map[string]any) string {
	return toolcatalogapp.NormalizeSkillRunAs(firstNonEmptyAnyString(skill["runAs"]))
}

func SkillArguments(args map[string]any) string {
	return firstNonEmptyAnyString(args["arguments"], args["task"], args["prompt"])
}

func SkillContinueOrForkRequested(args map[string]any) bool {
	return firstNonEmptyAnyString(args["continue_from"], args["continueFrom"], args["fork_from"], args["forkFrom"]) != ""
}

func SkillTaskRequestFromArgs(input SkillTaskRequestInput) (TaskRequest, error) {
	skill := input.Skill
	args := input.Args
	task := SkillArguments(args)
	if strings.TrimSpace(task) == "" {
		return TaskRequest{}, errors.New("run_skill requires arguments for runAs=subagent skill " + contracts.StringField(skill, "id"))
	}
	packageDigest := strings.TrimSpace(contracts.StringField(skill, "packageDigest"))
	if decoded, err := hex.DecodeString(packageDigest); err != nil || len(decoded) != 32 {
		return TaskRequest{}, errors.New("run_skill requires a valid content-addressed skill package snapshot")
	}
	providerID := strings.TrimSpace(firstNonEmptyAnyString(args["providerId"], args["provider_id"], skill["providerId"], skill["provider_id"]))
	model := strings.TrimSpace(firstNonEmptyAnyString(args["model"], skill["model"]))
	endpointFormat := domainmodel.OptionalEndpointFormat(firstNonEmptyAnyString(args["endpointFormat"], args["endpoint_format"], skill["endpointFormat"], skill["endpoint_format"]))
	variant := strings.TrimSpace(firstNonEmptyAnyString(args["variant"], args["modelVariant"], args["model_variant"], skill["variant"], skill["modelVariant"], skill["model_variant"]))
	effort, effortErr := subagentReasoningEffortFromValues(args["effort"], skill["effort"])
	if effortErr != nil {
		return TaskRequest{}, effortErr
	}
	request := TaskRequest{
		Name:               contracts.StringField(skill, "id"),
		Label:              "skill: " + contracts.StringField(skill, "id"),
		Prompt:             input.Prompt,
		ProviderID:         providerID,
		Model:              model,
		EndpointFormat:     endpointFormat,
		Variant:            variant,
		Effort:             effort,
		ProfileName:        "skill:" + contracts.StringField(skill, "id"),
		Tools:              toolcatalogapp.SkillToolList(skill["allowedTools"]),
		SkillPackageDigest: packageDigest,
		ContinueFrom:       firstNonEmptyAnyString(args["continue_from"], args["continueFrom"]),
		ForkFrom:           firstNonEmptyAnyString(args["fork_from"], args["forkFrom"]),
	}
	request.ProviderExplicit = strings.TrimSpace(firstNonEmptyAnyString(args["providerId"], args["provider_id"])) != ""
	request.ModelExplicit = strings.TrimSpace(firstNonEmptyAnyString(args["model"])) != ""
	request.EndpointExplicit = domainmodel.OptionalEndpointFormat(firstNonEmptyAnyString(args["endpointFormat"], args["endpoint_format"])) != ""
	request.VariantExplicit = strings.TrimSpace(firstNonEmptyAnyString(args["variant"], args["modelVariant"], args["model_variant"])) != ""
	if !request.ProviderExplicit && strings.TrimSpace(firstNonEmptyAnyString(skill["providerId"], skill["provider_id"])) != "" {
		request.ProfileExecutionConfigured = true
	}
	if !request.ModelExplicit && strings.TrimSpace(firstNonEmptyAnyString(skill["model"])) != "" {
		request.ProfileExecutionConfigured = true
	}
	if !request.EndpointExplicit && domainmodel.OptionalEndpointFormat(firstNonEmptyAnyString(skill["endpointFormat"], skill["endpoint_format"])) != "" {
		request.ProfileExecutionConfigured = true
	}
	if !request.VariantExplicit && strings.TrimSpace(firstNonEmptyAnyString(skill["variant"], skill["modelVariant"], skill["model_variant"])) != "" {
		request.ProfileExecutionConfigured = true
	}
	return request, nil
}

func RunSkillSubagentOutput(output map[string]any, skill map[string]any) map[string]any {
	if SecurityBoundOutputMap(output) {
		return PublicChildOutputProjection(output)
	}
	if output == nil {
		output = map[string]any{}
	}
	output["kind"] = "run_skill"
	output["skillId"] = contracts.StringField(skill, "id")
	output["skillName"] = contracts.StringField(skill, "name")
	output["runAs"] = "subagent"
	return output
}

func InlineSkillOutput(skill map[string]any, args map[string]any, runAs string, instruction string) map[string]any {
	return map[string]any{
		"kind":        "run_skill",
		"skillId":     contracts.StringField(skill, "id"),
		"skillName":   contracts.StringField(skill, "name"),
		"runAs":       firstNonEmptyAnyString(runAs, "inline"),
		"instruction": instruction,
		"arguments":   SkillArguments(args),
	}
}
