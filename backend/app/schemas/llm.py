from __future__ import annotations

from typing import Any, List, Literal

from pydantic import BaseModel, ConfigDict, Field

LlmReasoningEffort = Literal["", "none", "minimal", "low", "medium", "high", "xhigh"]


class LlmChatMessageDTO(BaseModel):
    role: Literal["user", "assistant"]
    content: str = Field(min_length=1, max_length=20000)


class LlmConversationMessageVersionDTO(BaseModel):
    content: str = ""
    createdAt: str = ""
    startedAt: str = ""
    completedAt: str = ""
    status: str = "complete"


class LlmConversationMessageDTO(BaseModel):
    id: str = Field(min_length=1, max_length=120)
    role: Literal["user", "assistant"]
    content: str = ""
    createdAt: str = ""
    startedAt: str = ""
    completedAt: str = ""
    status: str = "complete"
    persistForModel: bool = True
    skillTrace: dict[str, Any] = Field(default_factory=dict)
    versionGroupId: str = ""
    versions: List[LlmConversationMessageVersionDTO] = Field(default_factory=list)


class LlmConversationSessionMemoryDTO(BaseModel):
    focus_summary: str = ""
    conclusion_summary: str = ""
    next_steps_summary: str = ""
    covered_until_turn_id: str = ""
    covered_message_count: int = 0
    tags: List[str] = Field(default_factory=list, max_length=20)


class LlmMemoryNoteDTO(BaseModel):
    note_id: str = ""
    scope: Literal["workspace", "operator"] = "workspace"
    owner_id: str = ""
    memory_type: Literal[
        "workflow_feedback",
        "workspace_reference",
        "project_signal",
        "operator_preference",
    ] = "project_signal"
    title: str = ""
    summary: str = ""
    detail: str = ""
    freshness: Literal["stable", "time_sensitive", "volatile"] = "stable"
    trust_level: Literal["confirmed", "working", "preference"] = "working"
    required_revalidation: bool = False
    tags: List[str] = Field(default_factory=list, max_length=20)
    source: Literal["manual", "derived"] = "manual"
    status: Literal["active", "archived"] = "active"
    created_at: str = ""
    updated_at: str = ""


class LlmMemoryHygieneProposalDTO(BaseModel):
    proposal_id: str = ""
    group: Literal["promotions", "cleanup", "ambiguous", "no_action_needed"] = "cleanup"
    action: str = ""
    target_kind: str = ""
    target_id: str = ""
    title: str = ""
    scope: str = ""
    severity: Literal["info", "warning"] = "info"
    rationale: str = ""
    suggested_inputs: dict[str, Any] = Field(default_factory=dict)
    evidence: dict[str, Any] = Field(default_factory=dict)


class LlmMemoryHygieneReviewDTO(BaseModel):
    case_id: str = ""
    actor_id: str = ""
    summary: dict[str, Any] = Field(default_factory=dict)
    promotions: List[LlmMemoryHygieneProposalDTO] = Field(default_factory=list)
    cleanup: List[LlmMemoryHygieneProposalDTO] = Field(default_factory=list)
    ambiguous: List[LlmMemoryHygieneProposalDTO] = Field(default_factory=list)
    no_action_needed: List[LlmMemoryHygieneProposalDTO] = Field(default_factory=list)


class LlmConversationSessionDTO(BaseModel):
    id: str = Field(min_length=1, max_length=120)
    title: str = Field(default="新会话", max_length=200)
    createdAt: str = ""
    updatedAt: str = ""
    pinnedAt: str = ""
    documentIds: List[str] = Field(default_factory=list)
    subjectIds: List[str] = Field(default_factory=list)
    weakSubjectIds: List[str] = Field(default_factory=list)
    accountKeys: List[str] = Field(default_factory=list)
    lastPrompt: str = ""
    messages: List[LlmConversationMessageDTO] = Field(default_factory=list)
    memory: LlmConversationSessionMemoryDTO = Field(default_factory=LlmConversationSessionMemoryDTO)
    skillTrace: dict[str, Any] = Field(default_factory=dict)
    domainTrace: dict[str, Any] = Field(default_factory=dict)
    dataStalenessState: Literal["fresh", "stale", "unknown"] = "unknown"
    dataStalenessReason: str = ""
    boundSourceRevision: int = 0
    currentSourceRevision: int = 0


class LlmPromptTemplateDTO(BaseModel):
    key: str = ""
    description: str = ""
    layer_type: str = ""
    cache_scope: str = ""
    stable: bool = True
    content: str = ""


class LlmPromptRegistryDTO(BaseModel):
    version: str = ""
    source_path: str = ""
    template_count: int = 0
    template_keys: List[str] = Field(default_factory=list)
    registry_hash: str = ""


class LlmPromptLayerDebugDTO(BaseModel):
    name: str = ""
    layer_type: str = ""
    cache_scope: str = ""
    stable: bool = True
    content: str = ""
    metadata: dict[str, Any] = Field(default_factory=dict)


class LlmPromptReconstructionDTO(BaseModel):
    label: str = ""
    message_id: str = ""
    turn_index: int = 0
    role: str = "assistant"
    primary_prompt_key: str = ""
    direct_answer_guidance_enabled: bool = False
    layer_order: List[str] = Field(default_factory=list)
    layers: List[LlmPromptLayerDebugDTO] = Field(default_factory=list)
    reconstruction_notes: List[str] = Field(default_factory=list)
    prompt_router: dict[str, Any] = Field(default_factory=dict)
    prompt_mode: dict[str, Any] = Field(default_factory=dict)
    trace_source: str = ""
    prompt_contracts: dict[str, Any] = Field(default_factory=dict)
    prompt_evidence_sufficiency: dict[str, Any] = Field(default_factory=dict)
    prompt_registry: dict[str, Any] = Field(default_factory=dict)
    instruction_trace: dict[str, Any] = Field(default_factory=dict)
    runtime_input: dict[str, Any] = Field(default_factory=dict)
    prepare_chat_adapter: str = ""
    turn_runtime: dict[str, Any] = Field(default_factory=dict)
    runtime_architecture: dict[str, Any] = Field(default_factory=dict)


class LlmSessionPromptTraceDTO(BaseModel):
    prompt_router: dict[str, Any] = Field(default_factory=dict)
    prompt_mode: dict[str, Any] = Field(default_factory=dict)
    prompt_contracts: dict[str, Any] = Field(default_factory=dict)
    prompt_evidence_sufficiency: dict[str, Any] = Field(default_factory=dict)
    trace_source: str = ""
    prompt_registry: dict[str, Any] = Field(default_factory=dict)
    instruction_trace: dict[str, Any] = Field(default_factory=dict)
    runtime_input: dict[str, Any] = Field(default_factory=dict)
    prepare_chat_adapter: str = ""
    turn_runtime: dict[str, Any] = Field(default_factory=dict)
    runtime_architecture: dict[str, Any] = Field(default_factory=dict)


class LlmFollowUpPolicyDTO(BaseModel):
    require_prior_assistant: bool = True
    require_continuation_for_account_risk: bool = True
    plan_markers: List[str] = Field(default_factory=list)
    account_markers: List[str] = Field(default_factory=list)
    risk_markers: List[str] = Field(default_factory=list)
    continuation_markers: List[str] = Field(default_factory=list)


class LlmSkillCatalogDebugCandidateDTO(BaseModel):
    skill_id: str = ""
    display_name: str = ""
    action_label: str = ""
    workflow_goal: str = ""
    planner_hints: List[str] = Field(default_factory=list)
    matched_terms: List[str] = Field(default_factory=list)
    score: int = 0
    base_score: int = 0
    usage_bonus: int = 0
    usage_score: float = 0.0
    usage_count: int = 0
    strong_match: bool = False
    source: str = ""


class LlmSkillCatalogDebugSelectionDTO(BaseModel):
    selection_type: str = ""
    skill_id: str = ""
    display_name: str = ""
    action_label: str = ""
    reason: str = ""
    approval_state: str = ""
    policy_decision: str = ""


class LlmSkillCatalogDebugBatchProfileDTO(BaseModel):
    batch_id: str = ""
    batch_index: int = 0
    run_mode: str = ""
    skill_ids: List[str] = Field(default_factory=list)
    skill_display_names: List[str] = Field(default_factory=list)
    shared_engine_open_count: int = 0
    shared_engine_reuse_count: int = 0
    execution_cache_hits: int = 0
    execution_cache_misses: int = 0
    exact_cache_hits: int = 0
    exact_cache_misses: int = 0
    exact_cache_records: int = 0
    latest_cache_hits: int = 0
    latest_cache_misses: int = 0
    txn_row_cache_hits: int = 0
    txn_row_cache_misses: int = 0
    txn_row_cache_bucket_count: int = 0
    summary_text: str = ""


class LlmSkillCatalogDebugDTO(BaseModel):
    case_id: str = ""
    session_id: str = ""
    title: str = ""
    latest_user_message: str = ""
    intent_kind: str = ""
    route_kind: str = ""
    assembly_mode: str = ""
    primary_prompt_key: str = ""
    follow_up_debug: dict[str, Any] = Field(default_factory=dict)
    follow_up_policy: dict[str, Any] = Field(default_factory=dict)
    catalog_stats: dict[str, Any] = Field(default_factory=dict)
    candidates: List[LlmSkillCatalogDebugCandidateDTO] = Field(default_factory=list)
    selected_skills: List[LlmSkillCatalogDebugSelectionDTO] = Field(default_factory=list)
    batch_profiles: List[LlmSkillCatalogDebugBatchProfileDTO] = Field(default_factory=list)
    trace_source: str = ""


class LlmSessionPromptDebugDTO(BaseModel):
    case_id: str = ""
    session_id: str = ""
    title: str = ""
    registry: LlmPromptRegistryDTO = Field(default_factory=LlmPromptRegistryDTO)
    active_template_keys: List[str] = Field(default_factory=list)
    active_templates: List[LlmPromptTemplateDTO] = Field(default_factory=list)
    session_prompt_trace: LlmSessionPromptTraceDTO = Field(default_factory=LlmSessionPromptTraceDTO)
    latest_reconstruction: LlmPromptReconstructionDTO = Field(default_factory=LlmPromptReconstructionDTO)
    assistant_turns: List[LlmPromptReconstructionDTO] = Field(default_factory=list)


class LlmSessionsSyncReq(BaseModel):
    case_id: str = Field(min_length=1)
    sessions: List[LlmConversationSessionDTO] = Field(default_factory=list, max_length=300)
    replace_missing: bool = True


class LlmMemoryNoteReq(BaseModel):
    case_id: str = Field(min_length=1)
    note: LlmMemoryNoteDTO = Field(default_factory=LlmMemoryNoteDTO)


class LlmChatStreamReq(BaseModel):
    model_config = ConfigDict(extra="forbid")
    case_id: str = Field(min_length=1)
    session_id: str = Field(default="", max_length=120)
    messages: List[LlmChatMessageDTO] = Field(default_factory=list, max_length=24)
    thread_items_v3: List[dict[str, Any]] = Field(default_factory=list)
    turn_timeline_v3: List[dict[str, Any]] = Field(default_factory=list)
    latest_user_message: str = ""
    source_of_truth: str = ""
    model: str = Field(default="", max_length=120)
    document_ids: List[str] = Field(default_factory=list)
    subject_ids: List[str] = Field(default_factory=list)
    weak_subject_ids: List[str] = Field(default_factory=list)
    account_keys: List[str] = Field(default_factory=list)
    source_file_ids: List[str] = Field(default_factory=list)
    graph_request_contexts: dict[str, Any] = Field(default_factory=dict)
    approval_policy: Any = None
    approvals_reviewer: str = ""
    sandbox: Any = None
    sandbox_policy: Any = None
    model_provider: str = ""
    service_tier: str = ""
    effort: LlmReasoningEffort = ""
    summary: Any = None
    personality: str = ""
    output_schema: dict[str, Any] = Field(default_factory=dict)
    collaboration_mode: Any = None
    use_selected_documents: bool = True
    use_selected_subjects: bool = True


class LlmOllamaModelDTO(BaseModel):
    name: str
    size_bytes: int = 0
    parameter_size: str = ""
    quantization_level: str = ""
    modified_at: str = ""


class LlmOllamaStatusDTO(BaseModel):
    is_running: bool = False
    base_url: str = ""
    message: str = ""
    model_count: int = 0
    models: List[LlmOllamaModelDTO] = Field(default_factory=list)


class LlmModelConfigStatusDTO(BaseModel):
    config_key: Literal["dashscope", "xchai", "minimax"]
    label: str
    provider: str
    configured: bool = False
    masked_key: str = ""
    save_supported: bool = True
    test_supported: bool = True
    available_models: List[str] = Field(default_factory=list)
    custom_models: List[str] = Field(default_factory=list)
    hidden_models: List[str] = Field(default_factory=list)


class LlmReasoningEffortOptionDTO(BaseModel):
    reasoning_effort: str = ""
    description: str = ""


class LlmModelUpgradeInfoDTO(BaseModel):
    model: str = ""
    upgrade_copy: str = ""
    model_link: str = ""
    migration_markdown: str = ""


class LlmModelRuntimeSupportDTO(BaseModel):
    model_key: str
    provider: str = ""
    base_url: str = ""
    supported: bool = True
    runtime_path: Literal["native_responses", "responses_shim", "unsupported"] = "native_responses"
    runtime_path_label: str = ""
    reason_code: str = ""
    reason: str = ""
    suggested_model_key: str = ""
    support_tier: str = "native"
    supported_reasoning_efforts: List[LlmReasoningEffortOptionDTO] = Field(default_factory=list)
    default_reasoning_effort: str = ""
    upgrade_info: LlmModelUpgradeInfoDTO | None = None
    hidden: bool = False
    is_default: bool = False


class LlmModelConfigStatusListDTO(BaseModel):
    items: List[LlmModelConfigStatusDTO] = Field(default_factory=list)
    active_model_key: str = ""
    runtime_architecture: str = ""
    preferred_model_key: str = ""
    runtime_support: List[LlmModelRuntimeSupportDTO] = Field(default_factory=list)


class LlmModelConfigSaveReq(BaseModel):
    config_key: Literal["dashscope", "xchai", "minimax"]
    api_key: str = Field(min_length=1, max_length=4000)


class LlmModelConfigModelReq(BaseModel):
    config_key: Literal["dashscope", "xchai", "minimax"]
    model_key: str = Field(min_length=1, max_length=200)


class LlmModelConfigTestReq(BaseModel):
    model_key: str = Field(min_length=1, max_length=200)
    api_key: str = Field(default="", max_length=4000)


class LlmModelConfigTestDTO(BaseModel):
    model_key: str
    ok: bool = False
    message: str = ""


class LlmContextDocumentDTO(BaseModel):
    document_id: str
    filename: str = ""
    file_type: str = ""
    used_chars: int = 0
    chunk_count: int = 0
    truncated: bool = False


class LlmContextAccountDTO(BaseModel):
    account_key: str
    account_open_name: str = ""
    used_chars: int = 0
    txn_count: int = 0
    truncated: bool = False


class LlmPhaseJudgmentDTO(BaseModel):
    enabled: bool = False
    judgment_id: str = ""
    case_id: str = ""
    session_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    main_line_summary: str = ""
    stage_conclusion_summary: str = ""
    priority_action_summary: str = ""
    status: Literal[
        "empty",
        "session_draft",
        "working_hypothesis",
        "evidence_backed",
        "approval_blocked",
        "canonical",
        "stale",
        "conflict",
    ] = "empty"
    confidence: dict[str, float] = Field(default_factory=dict)
    tags: List[str] = Field(default_factory=list)
    source_query_ids: List[str] = Field(default_factory=list)
    source_evidence_ids: List[str] = Field(default_factory=list)
    source_path_ids: List[str] = Field(default_factory=list)
    source_report_ids: List[str] = Field(default_factory=list)
    derived_from: dict[str, Any] = Field(default_factory=dict)
    policy: dict[str, Any] = Field(default_factory=dict)
    visible: bool = False
    created_at: str = ""
    updated_at: str = ""


class LlmSessionHandoffItemDTO(BaseModel):
    item_id: str = ""
    title: str = ""
    detail: str = ""
    source: Literal["session_memory", "phase_judgment", "memory_hygiene"] = "session_memory"
    priority: Literal["high", "medium", "low"] = "medium"
    refs: List[str] = Field(default_factory=list)


class LlmSessionHandoffDTO(BaseModel):
    case_id: str = ""
    session_id: str = ""
    session_title: str = ""
    summary: str = ""
    source: Literal["model", "derived"] = "derived"
    model: str = ""
    handoff_kind: Literal["session_handoff"] = "session_handoff"
    recent_message_count: int = 0
    has_session_memory: bool = False
    has_phase_judgment: bool = False
    task_summary: str = ""
    current_conclusion: str = ""
    next_step_summary: str = ""
    session_memory: LlmConversationSessionMemoryDTO = Field(default_factory=LlmConversationSessionMemoryDTO)
    phase_judgment: LlmPhaseJudgmentDTO = Field(default_factory=LlmPhaseJudgmentDTO)
    attention_items: List[LlmSessionHandoffItemDTO] = Field(default_factory=list)
    next_actions: List[LlmSessionHandoffItemDTO] = Field(default_factory=list)
    hygiene_summary: dict[str, Any] = Field(default_factory=dict)
    hygiene_proposals: List[LlmMemoryHygieneProposalDTO] = Field(default_factory=list)
    reference_anchors: dict[str, List[str]] = Field(default_factory=dict)


class LlmOutwardStateDTO(BaseModel):
    enabled: bool = False
    version: str = "v1"
    approval_state: Literal["approval_required", "clear"] = "clear"
    evidence_state: Literal["supported", "partial", "thin"] = "thin"
    promotion_state: Literal["approval_blocked", "confirmed", "draft_only", "neutral"] = "neutral"
    next_step_state: Literal["approval", "supplement", "advance", "steady"] = "steady"
    memory_source_state: Literal["session_memory", "runtime_only"] = "runtime_only"
    composite_state: Literal[
        "approval_supported_evidence",
        "approval_query_only",
        "confirmed_compacted_auto_closed",
        "confirmed_partial_evidence",
        "confirmed_thin_evidence",
        "draft_compacted_session_memory",
        "draft_session_memory",
        "neutral_session_memory",
        "standard",
    ] = "standard"
    write_approval_count: int = 0
    confirmed_count: int = 0
    draft_count: int = 0
    confirmation_required_count: int = 0
    approval_targets: str = ""
    compacted: bool = False
    auto_closed: bool = False


class LlmRuntimeSessionApprovalDTO(BaseModel):
    approval_id: str = ""
    request_kind: str = ""
    thread_id: str = ""
    turn_id: str = ""
    item_id: str = ""
    skill_id: str = ""
    title: str = ""
    risk_type: str = ""
    risk_level: str = ""
    status: Literal["requested", "approved", "continued", "rejected"] = "requested"
    reason: str = ""
    review_reasoning: str = ""
    input_summary: str = ""
    batch_id: str = ""
    batch_index: int = 0
    blocked: bool = False
    blocked_code: str = ""
    blocked_reason: str = ""
    retryable: bool = False
    retry_class: str = ""
    duplicate_batch: bool = False
    grant_root: str = ""
    network_target: str = ""
    network_host: str = ""
    network_protocol: str = ""
    network_port: int = 0
    available_decisions: list[dict[str, Any]] = Field(default_factory=list)
    policy_amendment_options: list[dict[str, Any]] = Field(default_factory=list)
    policy_amendment: dict[str, Any] = Field(default_factory=dict)
    denied_source: str = ""
    timed_out: bool = False
    updated_at: str = ""


class LlmRuntimeInteractiveRequestDTO(BaseModel):
    request_id: str = ""
    request_kind: str = ""
    thread_id: str = ""
    turn_id: str = ""
    item_id: str = ""
    title: str = ""
    status: Literal["requested", "completed", "interrupted"] = "requested"
    request_payload: dict[str, Any] = Field(default_factory=dict)
    response_payload: dict[str, Any] = Field(default_factory=dict)
    updated_at: str = ""


class LlmRuntimeThreadDTO(BaseModel):
    id: str = ""
    forked_from_id: str = ""
    preview: str = ""
    ephemeral: bool = False
    model_provider: str = ""
    created_at: int = 0
    updated_at: int = 0
    status: str = ""
    path: str = ""
    cwd: str = ""
    cli_version: str = ""
    source: str = ""
    agent_nickname: str = ""
    agent_role: str = ""
    git_info: dict[str, Any] = Field(default_factory=dict)
    name: str = ""
    turn_count: int = 0


class LlmRuntimeSessionConfigDTO(BaseModel):
    thread_id: str = ""
    model: str = ""
    model_provider: str = ""
    cwd: str = ""
    approval_policy: Any = None
    approvals_reviewer: str = ""
    sandbox: Any = None
    sandbox_policy: Any = None
    reasoning_effort: str = ""
    summary: Any = None
    personality: str = ""
    output_schema: dict[str, Any] = Field(default_factory=dict)
    collaboration_mode: dict[str, Any] = Field(default_factory=dict)
    service_tier: str = ""
    web_search_mode: str = ""


class LlmRuntimeSessionStateDTO(BaseModel):
    persisted: bool = False
    resume_ready: bool = False
    busy: bool = False
    stream_state: str = "idle"
    waiting_on_approval: bool = False
    waiting_on_interactive: bool = False
    can_interrupt: bool = False
    last_turn_id: str = ""
    root_thread_id: str = ""
    root_turn_id: str = ""
    last_event_at: str = ""
    updated_at: str = ""


class LlmRuntimeSessionStatusDTO(BaseModel):
    case_id: str = ""
    session_id: str = ""
    runtime_mode: str = "codex_app_server"
    thread: LlmRuntimeThreadDTO = Field(default_factory=LlmRuntimeThreadDTO)
    session: LlmRuntimeSessionConfigDTO = Field(default_factory=LlmRuntimeSessionConfigDTO)
    status: LlmRuntimeSessionStateDTO = Field(default_factory=LlmRuntimeSessionStateDTO)
    replay: dict[str, Any] = Field(default_factory=dict)
    runtime_state: dict[str, Any] = Field(default_factory=dict)
    recovery: dict[str, Any] = Field(default_factory=dict)
    approvals: List[LlmRuntimeSessionApprovalDTO] = Field(default_factory=list)
    interactive_requests: List[LlmRuntimeInteractiveRequestDTO] = Field(default_factory=list)
    diagnostics: dict[str, Any] = Field(default_factory=dict)


class LlmRuntimeInteractiveResponseReq(BaseModel):
    response_payload: dict[str, Any] = Field(default_factory=dict)


class LlmRuntimeTurnSteerReq(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    expected_turn_id: str = Field(alias="expectedTurnId", min_length=1)
    input: List[dict[str, Any]] = Field(default_factory=list)

class LlmRuntimeThreadInjectItemsReq(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    items: List[dict[str, Any]] = Field(default_factory=list)


class LlmRuntimeThreadCompactReq(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    reason: str = ""


class LlmRuntimeThreadRollbackReq(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    num_turns: int = Field(alias="numTurns", ge=1)


class LlmStreamMetaDTO(BaseModel):
    provider: str
    model: str
    assembly_mode: str = ""
    reasoning_effort: str = ""
    task_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    document_count: int = 0
    account_count: int = 0
    txn_row_count: int = 0
    context_chars: int = 0
    documents: List[LlmContextDocumentDTO] = Field(default_factory=list)
    accounts: List[LlmContextAccountDTO] = Field(default_factory=list)
    skill_trace: dict[str, Any] = Field(default_factory=dict)
    compact_trace: dict[str, Any] = Field(default_factory=dict)
    instruction_trace: dict[str, Any] = Field(default_factory=dict)
    memory_trace: dict[str, Any] = Field(default_factory=dict)
    model_route_trace: dict[str, Any] = Field(default_factory=dict)
    token_budget_trace: dict[str, Any] = Field(default_factory=dict)
    cache_trace: dict[str, Any] = Field(default_factory=dict)
    run_log_trace: dict[str, Any] = Field(default_factory=dict)
    background_trace: dict[str, Any] = Field(default_factory=dict)
    context_scope: dict[str, Any] = Field(default_factory=dict)


class LlmStreamDoneDTO(BaseModel):
    provider: str
    model: str
    assembly_mode: str = ""
    task_id: str = ""
    run_id: str = ""
    turn_id: str = ""
    message: str = ""
    reasoning_content: str = ""
    finish_reason: str = ""
    document_count: int = 0
    account_count: int = 0
    txn_row_count: int = 0
    context_chars: int = 0
    skill_trace: dict[str, Any] = Field(default_factory=dict)
    compact_trace: dict[str, Any] = Field(default_factory=dict)
    instruction_trace: dict[str, Any] = Field(default_factory=dict)
    memory_trace: dict[str, Any] = Field(default_factory=dict)
    sidecar_trace: dict[str, Any] = Field(default_factory=dict)
    model_route_trace: dict[str, Any] = Field(default_factory=dict)
    token_budget_trace: dict[str, Any] = Field(default_factory=dict)
    cache_trace: dict[str, Any] = Field(default_factory=dict)
    run_log_trace: dict[str, Any] = Field(default_factory=dict)
    background_trace: dict[str, Any] = Field(default_factory=dict)
    context_scope: dict[str, Any] = Field(default_factory=dict)


class LlmReferencePreviewRefDTO(BaseModel):
    ref_type: Literal["query", "evidence", "path", "report", "snapshot"] = "query"
    ref_id: str = Field(min_length=1, max_length=200)


class LlmReferencePreviewReq(BaseModel):
    case_id: str = Field(min_length=1)
    refs: List[LlmReferencePreviewRefDTO] = Field(default_factory=list, min_length=1, max_length=20)


class LlmReferencePreviewItemDTO(BaseModel):
    ref_type: Literal["query", "evidence", "path", "report", "snapshot"] = "query"
    ref_id: str = ""
    title: str = ""
    subtitle: str = ""
    summary: str = ""
    created_at: str = ""
    status: Literal["found", "missing"] = "found"
    data: dict[str, Any] = Field(default_factory=dict)
