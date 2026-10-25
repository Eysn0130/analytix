package subagent

import (
	"encoding/hex"
	"errors"
	"strings"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
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

// SkillResultCaptureInputV1 contains observations from the admitted producer,
// before the parent output projection withholds child association and output.
type SkillResultCaptureInputV1 struct {
	RequestedName    string
	Skill            map[string]any
	RunAs            string
	Instruction      string
	IsError          bool
	ErrorCode        string
	ErrorMessage     string
	Job              *domainjob.Record
	SecurityContext  domainsecurity.TurnSecurityContext
	ExecutionGrantID string
	CallID           string
}

func SkillResultCaptureV1(input SkillResultCaptureInputV1) (domaintoolresult.ProtectedCaptureV1, bool) {
	data := &domaintoolresult.ProtectedSkillCaptureV1{RequestedName: input.RequestedName}
	capture := domaintoolresult.ProtectedCaptureV1{Kind: "skill", Status: "completed", Skill: data}
	if input.IsError {
		capture.Status = "failed"
		if input.ErrorCode != "" {
			message, _ := domaintoolresult.BoundProtectedTextV1(input.ErrorMessage, domaintoolresult.ProtectedSnapshotBodyLimitV1)
			data.Error = &domaintoolresult.ProtectedWebErrorV1{Code: input.ErrorCode, Message: message}
		}
	}
	if input.Skill != nil {
		data.Resolved = &domaintoolresult.ProtectedSkillIdentityV1{
			ID: contracts.StringField(input.Skill, "id"), Name: contracts.StringField(input.Skill, "name"),
			RunAs: input.RunAs, PackageDigest: contracts.StringField(input.Skill, "packageDigest"),
		}
		if input.RunAs == "inline" && !input.IsError {
			capture.Body, capture.Truncated = domaintoolresult.BoundProtectedTextV1(input.Instruction, domaintoolresult.ProtectedSnapshotBodyLimitV1)
		}
	}
	if record := input.Job; record != nil && record.ID != "" {
		if data.Resolved == nil || data.Resolved.RunAs != "subagent" || record.Kind != "subagent" ||
			record.SkillPackageDigest != data.Resolved.PackageDigest ||
			!domainjob.SecurityBindingMatchesContext(record.SecurityBinding, input.SecurityContext) ||
			record.SecurityBinding.ParentExecutionGrantID != input.ExecutionGrantID || record.SecurityBinding.ParentToolCallID != input.CallID ||
			record.ParentThreadID != input.SecurityContext.ThreadID || record.ParentTurnID != input.SecurityContext.TurnID || record.ParentToolCallID != input.CallID {
			return domaintoolresult.ProtectedCaptureV1{}, false
		}
		data.Child = &domaintoolresult.ProtectedSkillChildV1{
			JobID: record.ID, Status: domainjob.PublicStatusV1(record.Status), Background: record.Background, OutputWithheld: true,
		}
		if record.ChildThreadID != "" {
			value := record.ChildThreadID
			data.Child.ChildThreadID = &value
		}
		if record.ChildTurnID != "" {
			value := record.ChildTurnID
			data.Child.ChildTurnID = &value
		}
	}
	if domaintoolresult.ValidateProtectedCaptureV1(capture) != nil {
		return domaintoolresult.ProtectedCaptureV1{}, false
	}
	return capture, true
}
