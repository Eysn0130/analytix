#!/usr/bin/env node

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const EVAL_COVERAGE_CONTRACT_VERSION = "0.16.16";

export const EVAL_COVERAGE_CONTRACT_REQUIRED_FIELDS = [
  "min_tasks",
  "min_passive_nonfunds_tasks",
  "min_passive_near_miss_tasks",
  "passive_near_miss_terms",
  "min_cases",
  "min_tasks_per_required_case",
  "min_report_claim_review_tasks",
  "min_report_claim_review_cases",
  "required_case_ids",
  "required_task_ids",
  "passive_nonfunds_task_ids",
  "functional_closure_contract",
  "required_dimensions"
];

const REQUIRED_FUNCTIONAL_CLOSURE_SURFACES = [
  "entry_status",
  "case_context",
  "object_dossier",
  "graph_visualization",
  "visual_evidence",
  "case_workbench",
  "full_case_analysis",
  "trace_continuation",
  "report_builder",
  "claim_review",
  "passive_nonintervention"
];

const REQUIRED_FUNCTIONAL_CLOSURE_MODES = [
  "current_plugin"
];

const REQUIRED_FUNCTIONAL_CLOSURE_AUTO_FAIL_MARKERS = [
  "/goal",
  "doctor",
  "score",
  "write_blocked",
  "report gate",
  "golden",
  "oracle",
  "rubric"
];

const REQUIRED_TASK_TAXONOMY_IDS = [
  "quick_fact",
  "ranking",
  "object_dossier",
  "counterparty",
  "trace_continuation",
  "investigation_lab",
  "full_case_analysis",
  "report",
  "claim_qa",
  "visual_evidence",
  "case_workbench",
  "passive_nonintervention"
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function uniqueTextList(values) {
  const seen = new Set();
  const result = [];
  for (const value of arrayOf(values).map(text).filter(Boolean)) {
    if (seen.has(value)) continue;
    seen.add(value);
    result.push(value);
  }
  return result;
}

function sortedUniqueTextList(values) {
  return [...new Set(arrayOf(values).map(text).filter(Boolean))].sort((left, right) => left.localeCompare(right));
}

export function withEvalCoverageContract(capabilityRegistry, evalCoverageFixture = {}) {
  const registry = objectOf(capabilityRegistry);
  const fixture = objectOf(evalCoverageFixture);
  const evalCoverageContract = objectOf(fixture.eval_coverage_contract || fixture);
  const capabilityEvalTasks = objectOf(fixture.capability_eval_tasks);
  const capabilities = arrayOf(registry.capabilities).map((capability) => {
    const item = objectOf(capability);
    const evalTasks = arrayOf(item.eval_tasks).length
      ? arrayOf(item.eval_tasks)
      : arrayOf(capabilityEvalTasks[text(item.id)]);
    return evalTasks.length ? { ...item, eval_tasks: evalTasks } : item;
  });
  return {
    ...registry,
    eval_coverage_contract: evalCoverageContract,
    capabilities
  };
}

export function validateEvalCoverageContract({ capabilityRegistry, goldenAnswerSet }) {
  const registry = objectOf(capabilityRegistry);
  const contract = objectOf(registry.eval_coverage_contract);
  const goldenTasks = arrayOf(objectOf(goldenAnswerSet).tasks);
  const goldenTaskMap = new Map(goldenTasks.map((task) => [text(objectOf(task).id), objectOf(task)]).filter(([id]) => Boolean(id)));
  const goldenTaskIds = new Set(goldenTaskMap.keys());
  const goldenTaskCaseIds = new Set(goldenTasks.map((task) => text(objectOf(task).case_id)).filter(Boolean));
  const requiredTaskIds = arrayOf(contract.required_task_ids).map(text).filter(Boolean);
  const requiredCaseIds = arrayOf(contract.required_case_ids).map(text).filter(Boolean);
  const passiveNonfundsTaskIds = arrayOf(contract.passive_nonfunds_task_ids).map(text).filter(Boolean);
  const minPassiveNonfundsTasks = Number(contract.min_passive_nonfunds_tasks || 0);
  const passiveNearMissTerms = arrayOf(contract.passive_near_miss_terms).map(text).filter(Boolean);
  const minPassiveNearMissTasks = Number(contract.min_passive_near_miss_tasks || 0);
  const minTasksPerRequiredCase = Number(contract.min_tasks_per_required_case || 0);
  const minReportClaimReviewTasks = Number(contract.min_report_claim_review_tasks || 0);
  const minReportClaimReviewCases = Number(contract.min_report_claim_review_cases || 0);
  const functionalClosureContract = objectOf(contract.functional_closure_contract);
  const functionalClosureTaskIds = uniqueTextList(functionalClosureContract.task_ids);
  const functionalClosureModes = uniqueTextList(functionalClosureContract.required_modes);
  const functionalClosureSurfaces = arrayOf(functionalClosureContract.required_product_surfaces).map(objectOf);
  const functionalClosureSurfaceIds = uniqueTextList(functionalClosureSurfaces.map((surface) => surface.id));
  const functionalClosureAutoFailMarkers = uniqueTextList(functionalClosureContract.auto_fail_markers);
  const coverageDimensions = arrayOf(contract.required_dimensions).map(objectOf);
  const reportClaimReviewTaskIds = reportTaskIdsFromCapabilityRegistry(capabilityRegistry);
  const reportClaimReviewCaseIds = new Set(
    [...reportClaimReviewTaskIds]
      .map((taskId) => text(objectOf(goldenTaskMap.get(taskId)).case_id))
      .filter(Boolean)
  );
  const weakRequiredCaseCounts = requiredCaseIds
    .map((caseId) => ({
      caseId,
      count: goldenTasks.filter((task) => text(objectOf(task).case_id) === caseId).length
    }))
    .filter((item) => item.count < minTasksPerRequiredCase);
  const missingTaskIds = requiredTaskIds.filter((taskId) => !goldenTaskIds.has(taskId));
  const missingCaseIds = requiredCaseIds.filter((caseId) => !goldenTaskCaseIds.has(caseId));
  const missingPassiveTaskIds = passiveNonfundsTaskIds.filter((taskId) => !goldenTaskIds.has(taskId));
  const passiveTaskFailures = passiveNonfundsTaskIds
    .map((taskId) => objectOf(goldenTaskMap.get(taskId)))
    .filter((task) => text(task.id) && (task.plugin_expected !== false || arrayOf(task.expected_tools).length !== 0))
    .map((task) => text(task.id));
  const passiveNearMissTaskIds = passiveNonfundsTaskIds.filter((taskId) => {
    const task = objectOf(goldenTaskMap.get(taskId));
    const taskText = [
      task.id,
      task.question,
      task.standard_logic,
      arrayOf(task.scoring_markers).join(" ")
    ].map(text).join(" ");
    return passiveNearMissTerms.some((term) => taskText.includes(term));
  });
  const dimensionFailures = coverageDimensions.flatMap((dimension) => {
    const dimensionId = text(dimension.id) || "unnamed";
    const missing = arrayOf(dimension.task_ids)
      .map(text)
      .filter(Boolean)
      .filter((taskId) => !goldenTaskIds.has(taskId));
    return missing.length ? [`${dimensionId}: ${missing.join(", ")}`] : [];
  });
  const functionalClosureSurfaceFailures = functionalClosureSurfaces.flatMap((surface) => {
    const surfaceId = text(surface.id) || "unnamed";
    const surfaceTaskIds = uniqueTextList(surface.task_ids);
    const missing = surfaceTaskIds.filter((taskId) => !goldenTaskIds.has(taskId));
    const notInFunctionalSuite = surfaceTaskIds.filter((taskId) => !functionalClosureTaskIds.includes(taskId));
    return [
      !surfaceTaskIds.length ? `${surfaceId}: missing task_ids` : "",
      missing.length ? `${surfaceId}: missing golden tasks ${missing.join(", ")}` : "",
      notInFunctionalSuite.length ? `${surfaceId}: tasks not in functional closure suite ${notInFunctionalSuite.join(", ")}` : ""
    ].filter(Boolean);
  });
  const missingFunctionalClosureTasks = functionalClosureTaskIds.filter((taskId) => !goldenTaskIds.has(taskId));
  const missingFunctionalClosureModes = REQUIRED_FUNCTIONAL_CLOSURE_MODES.filter((mode) => !functionalClosureModes.includes(mode));
  const missingFunctionalClosureSurfaces = REQUIRED_FUNCTIONAL_CLOSURE_SURFACES.filter((surface) => !functionalClosureSurfaceIds.includes(surface));
  const missingFunctionalClosureAutoFailMarkers = REQUIRED_FUNCTIONAL_CLOSURE_AUTO_FAIL_MARKERS
    .filter((marker) => !functionalClosureAutoFailMarkers.includes(marker));
  const functionalClosurePassiveTasks = functionalClosureTaskIds.filter((taskId) => passiveNonfundsTaskIds.includes(taskId));
  const functionalClosureReportTasks = functionalClosureTaskIds.filter((taskId) => reportClaimReviewTaskIds.has(taskId));
  const weakTaskMetadata = goldenTasks
    .filter((task) => {
      const item = objectOf(task);
      return !text(item.id)
        || !text(item.case_id)
        || !text(item.question)
        || !text(item.standard_logic)
        || !arrayOf(item.scoring_markers).length
        || !arrayOf(item.forbidden_claims).length;
    })
    .map((task) => text(objectOf(task).id) || "<missing-id>");
  const failures = [
    goldenTasks.length < Number(contract.min_tasks || 0)
      ? `task_count ${goldenTasks.length} < min_tasks ${Number(contract.min_tasks || 0)}`
      : "",
    goldenTaskCaseIds.size < Number(contract.min_cases || 0)
      ? `case_count ${goldenTaskCaseIds.size} < min_cases ${Number(contract.min_cases || 0)}`
      : "",
    weakRequiredCaseCounts.length
      ? `required case task counts below ${minTasksPerRequiredCase}: ${weakRequiredCaseCounts.map((item) => `${item.caseId}=${item.count}`).join(", ")}`
      : "",
    reportClaimReviewTaskIds.size < minReportClaimReviewTasks
      ? `report_claim_review_task_count ${reportClaimReviewTaskIds.size} < min_report_claim_review_tasks ${minReportClaimReviewTasks}`
      : "",
    reportClaimReviewCaseIds.size < minReportClaimReviewCases
      ? `report_claim_review_case_count ${reportClaimReviewCaseIds.size} < min_report_claim_review_cases ${minReportClaimReviewCases}`
      : "",
    missingTaskIds.length ? `missing required task ids: ${missingTaskIds.join(", ")}` : "",
    missingCaseIds.length ? `missing required case ids: ${missingCaseIds.join(", ")}` : "",
    passiveNonfundsTaskIds.length < minPassiveNonfundsTasks
      ? `passive_nonfunds_task_count ${passiveNonfundsTaskIds.length} < min_passive_nonfunds_tasks ${minPassiveNonfundsTasks}`
      : "",
    missingPassiveTaskIds.length ? `missing passive task ids: ${missingPassiveTaskIds.join(", ")}` : "",
    passiveTaskFailures.length ? `passive tasks can still use plugin/tools: ${passiveTaskFailures.join(", ")}` : "",
    minPassiveNearMissTasks && !passiveNearMissTerms.length
      ? "passive_near_miss_terms is required when min_passive_near_miss_tasks > 0"
      : "",
    passiveNearMissTaskIds.length < minPassiveNearMissTasks
      ? `passive_near_miss_task_count ${passiveNearMissTaskIds.length} < min_passive_near_miss_tasks ${minPassiveNearMissTasks}`
      : "",
    !Object.keys(functionalClosureContract).length ? "missing functional_closure_contract" : "",
    missingFunctionalClosureTasks.length ? `functional closure tasks missing from golden set: ${missingFunctionalClosureTasks.join(", ")}` : "",
    missingFunctionalClosureModes.length ? `functional closure required modes missing: ${missingFunctionalClosureModes.join(", ")}` : "",
    missingFunctionalClosureSurfaces.length ? `functional closure surfaces missing: ${missingFunctionalClosureSurfaces.join(", ")}` : "",
    functionalClosureSurfaceFailures.length ? `functional closure surface gaps: ${functionalClosureSurfaceFailures.join("; ")}` : "",
    functionalClosurePassiveTasks.length < 1 ? "functional closure must include at least one passive non-funds task" : "",
    functionalClosureReportTasks.length < 1 ? "functional closure must include at least one report/claim-review task" : "",
    functionalClosureContract.allow_release_without_functional_closure !== false
      ? "functional closure must be required before release"
      : "",
    missingFunctionalClosureAutoFailMarkers.length
      ? `functional closure auto-fail markers missing: ${missingFunctionalClosureAutoFailMarkers.join(", ")}`
      : "",
    dimensionFailures.length ? `dimension gaps: ${dimensionFailures.join("; ")}` : "",
    weakTaskMetadata.length ? `tasks missing question/logic/markers/forbidden claims: ${weakTaskMetadata.join(", ")}` : ""
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    summary: {
      task_count: goldenTasks.length,
      case_count: goldenTaskCaseIds.size,
      dimension_count: coverageDimensions.length,
      passive_nonintervention_task_count: passiveNonfundsTaskIds.length,
      min_passive_nonfunds_tasks: minPassiveNonfundsTasks,
      passive_near_miss_task_count: passiveNearMissTaskIds.length,
      min_passive_near_miss_tasks: minPassiveNearMissTasks,
      passive_near_miss_terms: passiveNearMissTerms,
      min_tasks_per_required_case: minTasksPerRequiredCase,
      report_claim_review_task_count: reportClaimReviewTaskIds.size,
      report_claim_review_case_count: reportClaimReviewCaseIds.size,
      min_report_claim_review_tasks: minReportClaimReviewTasks,
      min_report_claim_review_cases: minReportClaimReviewCases,
      required_task_count: requiredTaskIds.length,
      required_case_count: requiredCaseIds.length,
      functional_closure_task_count: functionalClosureTaskIds.length,
      functional_closure_surface_count: functionalClosureSurfaceIds.length,
      functional_closure_required_modes: functionalClosureModes
    }
  };
}

export function reportTaskIdsFromCapabilityRegistry(capabilityRegistry) {
  const dimensions = arrayOf(objectOf(capabilityRegistry).eval_coverage_contract?.required_dimensions).map(objectOf);
  const reportDimension = dimensions.find((item) => text(item.id) === "report_claim_review");
  return new Set(arrayOf(reportDimension?.task_ids).map(text).filter(Boolean));
}

export function passiveNonfundsTaskIdsFromCapabilityRegistry(capabilityRegistry) {
  return new Set(
    arrayOf(objectOf(objectOf(capabilityRegistry).eval_coverage_contract).passive_nonfunds_task_ids)
      .map(text)
      .filter(Boolean)
  );
}

export function isReportTaskIdFromCapabilityRegistry(taskId, capabilityRegistry) {
  const reportTaskIds = reportTaskIdsFromCapabilityRegistry(capabilityRegistry);
  if (reportTaskIds.size) return reportTaskIds.has(text(taskId));
  return /report|claim|gate/iu.test(text(taskId));
}

export function expectedAnalytixToolBudgetForTask(taskOrTaskId, capabilityRegistry, options = {}) {
  const task = typeof taskOrTaskId === "object" ? objectOf(taskOrTaskId) : { id: taskOrTaskId };
  const taskId = text(task.id || task.task_id);
  const explicitTools = uniqueTextList(task.expectedTools || task.expected_tools);
  const pluginExpected = Object.prototype.hasOwnProperty.call(options, "pluginExpected")
    ? options.pluginExpected !== false
    : task.plugin_expected !== false && task.pluginExpected !== false;
  if (!pluginExpected) return 0;
  if (passiveNonfundsTaskIdsFromCapabilityRegistry(capabilityRegistry).has(taskId)) return 0;
  const explicitBudget = Number(task.expectedToolBudget || task.expected_tool_budget);
  if (Number.isFinite(explicitBudget) && explicitBudget >= 0) return explicitBudget;
  if (explicitTools.length > 1) return explicitTools.length;
  return isReportTaskIdFromCapabilityRegistry(taskId, capabilityRegistry) ? 2 : 1;
}

export function expectedAnalytixToolsForTask(taskOrTaskId, capabilityRegistry, options = {}) {
  const task = typeof taskOrTaskId === "object" ? objectOf(taskOrTaskId) : { id: taskOrTaskId };
  const explicitTools = uniqueTextList(task.expectedTools || task.expected_tools);
  if (explicitTools.length) return explicitTools;
  const budget = expectedAnalytixToolBudgetForTask(taskOrTaskId, capabilityRegistry, options);
  if (budget <= 0) return [];
  const taskId = text(task.id || task.task_id);
  if (/claim_review|claim_qa|mermaid_legal_guard|candidate_cash_boundary_guard/iu.test(taskId)) {
    return ["validate_report_claims"];
  }
  if (budget >= 2 || /report|claim|gate|materialize/iu.test(taskId)) {
    return ["run_full_case_analysis", "validate_report_claims"];
  }
  if (/top|rank/iu.test(taskId)) return ["rank_accounts"];
  if (/quality|import|clean|duplicate/iu.test(taskId)) return ["audit_case_data_quality", "resolve_duplicate_families"];
  if (/holder|owner|account_scope/iu.test(taskId)) return ["analyze_holder_full"];
  if (/continue|next_hop|flow|path|destination|outflow|amount/iu.test(taskId)) return ["trace_subject_top_outflows"];
  if (/negative|lab|pattern|hypothesis|multi/iu.test(taskId)) return ["hypothesis_probe"];
  return ["get_current_case"];
}

export function commandBudgetFromCapabilityRegistry(commandName, capabilityRegistry) {
  const command = text(commandName);
  const capabilities = arrayOf(objectOf(capabilityRegistry).capabilities).map(objectOf);
  const capability = objectOf(capabilities.find((item) => arrayOf(item.commands).map(text).includes(command)));
  const budget = objectOf(capability?.tool_budget);
  if (!command || !Object.keys(capability).length || !Object.keys(budget).length) return null;
  return {
    command,
    capability_id: text(capability.id),
    ordinary_max_tool_calls: Number(budget.ordinary_max_tool_calls),
    report_max_tool_calls: Number(budget.report_max_tool_calls),
    max_command_audit_tool_calls: Number(budget.max_command_audit_tool_calls || budget.report_max_tool_calls)
  };
}

export function validateCommandMetadataBudgetContract({ capabilityRegistry, commandMetadata }) {
  const commands = arrayOf(objectOf(commandMetadata).commands).map(objectOf);
  const failures = commands.flatMap((command) => {
    const commandName = text(command.command);
    const derived = commandBudgetFromCapabilityRegistry(commandName, capabilityRegistry);
    if (!derived) return [`${commandName || "<missing-command>"}: missing registry capability budget`];
    const explicit = objectOf(command.toolBudget);
    if (!Object.keys(explicit).length) return [];
    const recommended = Number(explicit.recommendedToolCalls);
    const max = Number(explicit.maxToolCalls);
    const maxWithAudit = Number(explicit.maxToolCallsWithAuditBoundaries);
    const localFailures = [
      Number.isFinite(recommended) && recommended > derived.ordinary_max_tool_calls
        ? `recommended=${explicit.recommendedToolCalls} > ordinary=${derived.ordinary_max_tool_calls}`
        : "",
      Number.isFinite(max) && max > derived.ordinary_max_tool_calls
        ? `max=${explicit.maxToolCalls} > ordinary=${derived.ordinary_max_tool_calls}`
        : "",
      Number.isFinite(maxWithAudit) && maxWithAudit > derived.report_max_tool_calls
        ? `audit=${explicit.maxToolCallsWithAuditBoundaries} > report=${derived.report_max_tool_calls}`
        : "",
      Number.isFinite(maxWithAudit) && Number.isFinite(derived.max_command_audit_tool_calls) && maxWithAudit > derived.max_command_audit_tool_calls
        ? `audit=${explicit.maxToolCallsWithAuditBoundaries} > command_audit=${derived.max_command_audit_tool_calls}`
        : ""
    ].filter(Boolean);
    return localFailures.length ? [`${commandName}: ${localFailures.join(", ")}`] : [];
  });
  return {
    ok: failures.length === 0,
    failures,
    summary: {
      command_count: commands.length,
      explicit_budget_count: commands.filter((command) => Object.keys(objectOf(command.toolBudget)).length).length,
      registry_budgeted_command_count: commands.filter((command) => commandBudgetFromCapabilityRegistry(text(command.command), capabilityRegistry)).length
    }
  };
}

export function deriveCapabilityRegistryMetadataSnapshot(capabilityRegistry) {
  const registry = objectOf(capabilityRegistry);
  const contract = objectOf(registry.tool_discovery_contract);
  const capabilities = arrayOf(registry.capabilities).map(objectOf);
  const commandRows = capabilities
    .flatMap((capability) => {
      const budget = objectOf(capability.tool_budget);
      const primaryTools = uniqueTextList(capability.primary_tools);
      const conditionalTools = uniqueTextList(capability.conditional_tools);
      return uniqueTextList(capability.commands).map((command) => ({
        command,
        capability_id: text(capability.id),
        lane: text(capability.lane),
        intent: text(capability.intent),
        primary_tools: primaryTools,
        conditional_tools: conditionalTools,
        tools: uniqueTextList([...primaryTools, ...conditionalTools]),
        tool_budget: {
          ordinary_max_tool_calls: Number(budget.ordinary_max_tool_calls || 0),
          report_max_tool_calls: Number(budget.report_max_tool_calls || 0),
          max_command_audit_tool_calls: Number(budget.max_command_audit_tool_calls || budget.report_max_tool_calls || 0)
        },
        score_dimensions: uniqueTextList(capability.score_dimensions),
        eval_task_ids: uniqueTextList(capability.eval_tasks),
        doctor_checks: uniqueTextList(capability.doctor_checks)
      }));
    })
    .sort((left, right) => left.command.localeCompare(right.command));
  const reportTaskIds = sortedUniqueTextList([...reportTaskIdsFromCapabilityRegistry(registry)]);
  const passiveNonfundsTaskIds = sortedUniqueTextList([...passiveNonfundsTaskIdsFromCapabilityRegistry(registry)]);
  const evalTaskIds = sortedUniqueTextList(capabilities.flatMap((capability) => arrayOf(capability.eval_tasks)));
  const ordinaryTaskIds = sortedUniqueTextList(
    evalTaskIds.filter((taskId) => !reportTaskIds.includes(taskId) && !passiveNonfundsTaskIds.includes(taskId))
  );
  return {
    schema_version: text(registry.schema_version),
    plugin: {
      name: text(registry.plugin?.name),
      version: text(registry.plugin?.version),
      distribution: text(registry.plugin?.distribution),
      runtime_boundary: text(registry.plugin?.runtime_boundary)
    },
    tool_discovery: {
      priority_tools: uniqueTextList(contract.priority_tools),
      default_visible_tools: uniqueTextList(contract.default_visible_tools),
      frontdoor_report_only_tools: uniqueTextList(contract.frontdoor_report_only_tools),
      full_discovery_env_vars: uniqueTextList(contract.full_discovery_env_vars),
      max_default_visible_tools: Number(contract.max_default_visible_tools || 0)
    },
    counts: {
      capability_count: capabilities.length,
      command_count: commandRows.length,
      tool_count: sortedUniqueTextList(commandRows.flatMap((row) => row.tools)).length,
      eval_task_count: evalTaskIds.length,
      passive_nonfunds_task_count: passiveNonfundsTaskIds.length,
      report_task_count: reportTaskIds.length
    },
    tools: sortedUniqueTextList(commandRows.flatMap((row) => row.tools)),
    commands: commandRows,
    eval_task_ids: evalTaskIds,
    passive_nonfunds_task_ids: passiveNonfundsTaskIds,
    report_task_ids: reportTaskIds,
    budget_profiles: {
      passive_nonfunds: {
        task_count: passiveNonfundsTaskIds.length,
        max_tool_calls: 0,
        tools: []
      },
      ordinary_funds: {
        task_count: ordinaryTaskIds.length,
        max_tool_calls: 1,
        tools: ["funds_investigate"]
      },
      report_claim_review: {
        task_count: reportTaskIds.length,
        max_tool_calls: 2,
        tools: ["funds_investigate", "validate_report_claims"]
      }
    }
  };
}

export function validateCapabilityRegistryDerivedMetadataContract({ capabilityRegistry, commandMetadata, goldenAnswerSet }) {
  const snapshot = deriveCapabilityRegistryMetadataSnapshot(capabilityRegistry);
  const commands = arrayOf(objectOf(commandMetadata).commands).map(objectOf);
  const metadataCommandNames = commands.map((command) => text(command.command)).filter(Boolean);
  const metadataCommandSet = new Set(metadataCommandNames);
  const derivedCommandNames = snapshot.commands.map((command) => text(command.command)).filter(Boolean);
  const derivedCommandSet = new Set(derivedCommandNames);
  const duplicateDerivedCommands = derivedCommandNames.filter((command, index) => derivedCommandNames.indexOf(command) !== index);
  const missingDerivedCommands = metadataCommandNames.filter((command) => !derivedCommandSet.has(command));
  const extraDerivedCommands = derivedCommandNames.filter((command) => !metadataCommandSet.has(command));
  const derivedCommandMap = new Map(snapshot.commands.map((command) => [command.command, command]));
  const metadataToolCoverageFailures = commands.flatMap((command) => {
    const commandName = text(command.command);
    const derived = derivedCommandMap.get(commandName);
    if (!commandName || !derived) return [];
    const derivedTools = new Set(uniqueTextList([...derived.primary_tools, ...derived.conditional_tools]));
    const commandTools = uniqueTextList([...arrayOf(command.primaryTools), ...arrayOf(command.conditionalTools)]);
    const missing = commandTools.filter((tool) => !derivedTools.has(tool));
    return missing.length ? [`${commandName}: ${missing.join(", ")}`] : [];
  });
  const focusedSkillIds = uniqueTextList(arrayOf(objectOf(capabilityRegistry).focused_skills).map((item) => objectOf(item).id));
  const focusedSkillSet = new Set(focusedSkillIds);
  const taskTaxonomy = arrayOf(objectOf(capabilityRegistry).task_taxonomy).map(objectOf);
  const taskTaxonomyIds = uniqueTextList(taskTaxonomy.map((item) => item.id));
  const taskTaxonomySet = new Set(taskTaxonomyIds);
  const metadataFocusedSkillFailures = commands.flatMap((command) => {
    const commandName = text(command.command) || "<missing-command>";
    const focusedSkill = text(command.focusedSkill);
    const taskType = text(command.taskType);
    const secondaryFocusedSkills = uniqueTextList(command.secondaryFocusedSkills);
    const localFailures = [
      !focusedSkill ? "missing focusedSkill" : "",
      focusedSkill && !focusedSkillSet.has(focusedSkill) ? `unknown focusedSkill ${focusedSkill}` : "",
      !taskType ? "missing taskType" : "",
      taskType && !taskTaxonomySet.has(taskType) ? `unknown taskType ${taskType}` : "",
      ...secondaryFocusedSkills
        .filter((skillName) => !focusedSkillSet.has(skillName))
        .map((skillName) => `unknown secondaryFocusedSkill ${skillName}`)
    ].filter(Boolean);
    return localFailures.length ? [`${commandName}: ${localFailures.join(", ")}`] : [];
  });
  const metadataCoveredFocusedSkills = new Set(
    commands.flatMap((command) => [
      text(command.focusedSkill),
      ...uniqueTextList(command.secondaryFocusedSkills)
    ]).filter(Boolean)
  );
  const metadataMissingFocusedSkills = focusedSkillIds.filter((skillName) => !metadataCoveredFocusedSkills.has(skillName));
  const metadataCommandSetForTaxonomy = new Set(metadataCommandNames);
  const metadataCoveredTaskTypes = new Set(commands.map((command) => text(command.taskType)).filter(Boolean));
  const taskTaxonomyFailures = taskTaxonomy.flatMap((taskType) => {
    const taskTypeId = text(taskType.id) || "<missing-task-type>";
    const unknownSkills = uniqueTextList(taskType.focused_skills).filter((skillName) => !focusedSkillSet.has(skillName));
    const missingCommands = uniqueTextList(taskType.commands).filter((command) => !metadataCommandSetForTaxonomy.has(command));
    return [
      !text(taskType.label) ? `${taskTypeId}: missing label` : "",
      !arrayOf(taskType.use_when).length ? `${taskTypeId}: missing use_when` : "",
      !text(taskType.completion_standard) ? `${taskTypeId}: missing completion_standard` : "",
      unknownSkills.length ? `${taskTypeId}: unknown focused_skills ${unknownSkills.join(", ")}` : "",
      missingCommands.length ? `${taskTypeId}: commands missing from metadata ${missingCommands.join(", ")}` : ""
    ].filter(Boolean);
  });
  const missingRequiredTaskTypes = REQUIRED_TASK_TAXONOMY_IDS.filter((taskTypeId) => !taskTaxonomySet.has(taskTypeId));
  const missingMetadataTaskTypes = taskTaxonomyIds
    .filter((taskTypeId) => taskTypeId !== "passive_nonintervention")
    .filter((taskTypeId) => !metadataCoveredTaskTypes.has(taskTypeId));
  const weakDerivedCommands = snapshot.commands
    .filter((command) => !command.score_dimensions.length || !command.eval_task_ids.length || !command.doctor_checks.length)
    .map((command) => command.command);
  const discoveryTools = new Set(snapshot.tools);
  const priorityTools = new Set(snapshot.tool_discovery.priority_tools);
  const discoveryFailures = [
    snapshot.tool_discovery.default_visible_tools.length > snapshot.tool_discovery.max_default_visible_tools
      ? `default_visible_tools ${snapshot.tool_discovery.default_visible_tools.length} > max_default_visible_tools ${snapshot.tool_discovery.max_default_visible_tools}`
      : "",
    snapshot.tool_discovery.default_visible_tools.filter((tool) => !priorityTools.has(tool)).length
      ? `default tools not in priority list: ${snapshot.tool_discovery.default_visible_tools.filter((tool) => !priorityTools.has(tool)).join(", ")}`
      : "",
    snapshot.tool_discovery.default_visible_tools.filter((tool) => !discoveryTools.has(tool)).length
      ? `default tools not owned by any capability: ${snapshot.tool_discovery.default_visible_tools.filter((tool) => !discoveryTools.has(tool)).join(", ")}`
      : "",
    snapshot.tool_discovery.frontdoor_report_only_tools.filter((tool) => !discoveryTools.has(tool)).length
      ? `report-only tools not owned by any capability: ${snapshot.tool_discovery.frontdoor_report_only_tools.filter((tool) => !discoveryTools.has(tool)).join(", ")}`
      : "",
    snapshot.tool_discovery.frontdoor_report_only_tools.join("|") !== snapshot.budget_profiles.report_claim_review.tools.join("|")
      ? `report-only tools must be ${snapshot.budget_profiles.report_claim_review.tools.join(", ")}`
      : ""
  ].filter(Boolean);
  const passiveBudgetFailures = snapshot.passive_nonfunds_task_ids
    .filter((taskId) => expectedAnalytixToolBudgetForTask(taskId, capabilityRegistry) !== 0)
    .map((taskId) => `${taskId}: expected passive budget 0`);
  const reportBudgetFailures = snapshot.report_task_ids
    .filter((taskId) => expectedAnalytixToolBudgetForTask(taskId, capabilityRegistry) !== 2)
    .map((taskId) => `${taskId}: expected report budget 2`);
  const ordinaryBudgetFailures = snapshot.eval_task_ids
    .filter((taskId) => !snapshot.passive_nonfunds_task_ids.includes(taskId) && !snapshot.report_task_ids.includes(taskId))
    .filter((taskId) => expectedAnalytixToolBudgetForTask(taskId, capabilityRegistry) !== 1)
    .map((taskId) => `${taskId}: expected ordinary budget 1`);
  const goldenTasks = arrayOf(objectOf(goldenAnswerSet).tasks).map(objectOf);
  const goldenTaskIds = new Set(goldenTasks.map((task) => text(task.id)).filter(Boolean));
  const missingGoldenTasks = goldenTaskIds.size
    ? [...goldenTaskIds].filter((taskId) => !snapshot.eval_task_ids.includes(taskId))
    : [];
  const failures = [
    duplicateDerivedCommands.length ? `duplicate derived commands: ${duplicateDerivedCommands.join(", ")}` : "",
    missingDerivedCommands.length ? `metadata commands missing from derived registry snapshot: ${missingDerivedCommands.join(", ")}` : "",
    extraDerivedCommands.length ? `derived registry commands missing from command metadata: ${extraDerivedCommands.join(", ")}` : "",
    metadataToolCoverageFailures.length ? `metadata tools not covered by derived capability owner: ${metadataToolCoverageFailures.join("; ")}` : "",
    metadataFocusedSkillFailures.length ? `metadata focused skill routing gaps: ${metadataFocusedSkillFailures.join("; ")}` : "",
    metadataMissingFocusedSkills.length ? `metadata commands do not cover focused skills: ${metadataMissingFocusedSkills.join(", ")}` : "",
    taskTaxonomyFailures.length ? `task taxonomy gaps: ${taskTaxonomyFailures.join("; ")}` : "",
    missingRequiredTaskTypes.length ? `task taxonomy missing required ids: ${missingRequiredTaskTypes.join(", ")}` : "",
    missingMetadataTaskTypes.length ? `task taxonomy ids not used by command metadata: ${missingMetadataTaskTypes.join(", ")}` : "",
    weakDerivedCommands.length ? `derived commands missing score/eval/doctor linkage: ${weakDerivedCommands.join(", ")}` : "",
    discoveryFailures.length ? `tool discovery contract: ${discoveryFailures.join("; ")}` : "",
    passiveBudgetFailures.length ? `passive budget drift: ${passiveBudgetFailures.join(", ")}` : "",
    reportBudgetFailures.length ? `report budget drift: ${reportBudgetFailures.join(", ")}` : "",
    ordinaryBudgetFailures.length ? `ordinary budget drift: ${ordinaryBudgetFailures.join(", ")}` : "",
    missingGoldenTasks.length ? `golden tasks missing from derived eval task ids: ${missingGoldenTasks.join(", ")}` : ""
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    snapshot,
    summary: {
      capability_count: snapshot.counts.capability_count,
      command_count: snapshot.counts.command_count,
      metadata_command_count: metadataCommandNames.length,
      tool_count: snapshot.counts.tool_count,
      eval_task_count: snapshot.counts.eval_task_count,
      passive_nonfunds_task_count: snapshot.counts.passive_nonfunds_task_count,
      report_task_count: snapshot.counts.report_task_count,
      metadata_focused_skill_count: metadataCoveredFocusedSkills.size,
      task_taxonomy_count: taskTaxonomyIds.length,
      metadata_task_type_count: metadataCoveredTaskTypes.size
    }
  };
}

export function validateEvalCoverageRegistrySchema({ capabilityRegistrySchema }) {
  const schema = objectOf(capabilityRegistrySchema);
  const evalCoverageSchema = objectOf(objectOf(schema.$defs).evalCoverageContract);
  const properties = objectOf(evalCoverageSchema.properties);
  const required = arrayOf(evalCoverageSchema.required).map(text).filter(Boolean);
  const missingRequired = EVAL_COVERAGE_CONTRACT_REQUIRED_FIELDS.filter((field) => !required.includes(field));
  const missingProperties = EVAL_COVERAGE_CONTRACT_REQUIRED_FIELDS.filter((field) => !Object.prototype.hasOwnProperty.call(properties, field));
  const failures = [
    evalCoverageSchema.additionalProperties !== false ? "evalCoverageContract.additionalProperties must be false" : "",
    missingRequired.length ? `schema required missing: ${missingRequired.join(", ")}` : "",
    missingProperties.length ? `schema properties missing: ${missingProperties.join(", ")}` : ""
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    failures,
    summary: {
      required_field_count: EVAL_COVERAGE_CONTRACT_REQUIRED_FIELDS.length,
      schema_required_count: required.length,
      schema_property_count: Object.keys(properties).length
    }
  };
}

export function formatEvalCoverageSummary(result) {
  const summary = objectOf(result?.summary);
  return `${summary.task_count || 0} tasks across ${summary.case_count || 0} cases cover ${summary.dimension_count || 0} eval-only contract dimensions, including ${summary.passive_nonintervention_task_count || 0} passive nonintervention tasks (${summary.passive_near_miss_task_count || 0} near-miss), ${summary.report_claim_review_task_count || 0} report claim-review tasks across ${summary.report_claim_review_case_count || 0} cases, and ${summary.functional_closure_task_count || 0} functional closure tasks across ${summary.functional_closure_surface_count || 0} product surfaces`;
}

function readJson(pathname) {
  return JSON.parse(readFileSync(pathname, "utf8"));
}

function parseCliArgs(argv) {
  return {
    json: argv.includes("--json")
  };
}

function runCli() {
  const options = parseCliArgs(process.argv.slice(2));
  const scriptDir = path.dirname(fileURLToPath(import.meta.url));
  const pluginRoot = path.resolve(scriptDir, "..");
  const capabilityRegistry = readJson(path.join(pluginRoot, "references/capability-registry.json"));
  const commandMetadata = readJson(path.join(pluginRoot, "references/command-metadata.json"));
  const capabilityRegistrySchema = readJson(path.join(pluginRoot, "references/capability-registry.schema.json"));
  const goldenAnswerSet = readJson(path.join(pluginRoot, "scripts/eval-fixtures/golden-answer-set.json"));
  const evalCoverageFixture = readJson(path.join(pluginRoot, "scripts/eval-fixtures/eval-coverage-contract.json"));
  const registryWithEvalContract = withEvalCoverageContract(capabilityRegistry, evalCoverageFixture);
  const coverage = validateEvalCoverageContract({
    capabilityRegistry: registryWithEvalContract,
    goldenAnswerSet
  });
  const derived = validateCapabilityRegistryDerivedMetadataContract({
    capabilityRegistry: registryWithEvalContract,
    commandMetadata,
    goldenAnswerSet
  });
  const schema = validateEvalCoverageRegistrySchema({ capabilityRegistrySchema });
  const failures = [
    ...arrayOf(coverage.failures).map((failure) => `coverage: ${failure}`),
    ...arrayOf(derived.failures).map((failure) => `derived: ${failure}`),
    ...arrayOf(schema.failures).map((failure) => `schema: ${failure}`)
  ];
  const result = {
    ok: failures.length === 0,
    version: EVAL_COVERAGE_CONTRACT_VERSION,
    summary: {
      coverage: coverage.summary,
      derived: derived.summary,
      schema: schema.summary,
      message: formatEvalCoverageSummary(coverage)
    },
    failures
  };
  if (options.json) {
    console.log(JSON.stringify(result, null, 2));
  } else if (result.ok) {
    console.log(`ok: ${result.summary.message}`);
  } else {
    console.error(`failures:\n${failures.map((failure) => `- ${failure}`).join("\n")}`);
  }
  if (!result.ok) process.exit(1);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCli();
}
