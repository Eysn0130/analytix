import os
from functools import lru_cache
from pathlib import Path
from typing import List
from urllib.parse import urlsplit

from pydantic import BaseModel, Field, SecretStr


DEFAULT_MINIMAX_BASE_URL = "https://api.minimaxi.com/v1"
DEFAULT_MINIMAX_API_KEY = ""
RUNTIME_OVERRIDE_ENV_FILENAME = ".env.desktop.local"
DEFAULT_DASHSCOPE_MODELS = ["qwen3.6-plus", "qwen3.6-flash"]
DEFAULT_DEEPSEEK_MODELS = ["deepseek-v4-flash", "deepseek-v4-pro"]
DEFAULT_VISION_BRIDGE_MODEL = "qwen3-vl-plus"
DEFAULT_XCHAI_MODELS = ["claude-opus-4-6"]
DEFAULT_MINIMAX_MODELS = ["MiniMax-M2.7", "MiniMax-M2.7-highspeed"]
DEFAULT_WEB_SEARCH_SEARXNG_URL = "http://127.0.0.1:18080"
DEFAULT_DESKTOP_CORS_ORIGINS = ["null", "file://"]
LOOPBACK_CORS_HOSTS = {"127.0.0.1", "::1", "localhost"}


def _default_runtime_architecture(app_env: str) -> str:
    del app_env
    return "codex-aligned"


def _load_env_defaults() -> None:
    project_root = Path(__file__).resolve().parents[3]
    protected_keys = set(os.environ.keys())
    for filename in (".env", ".env.local", ".env.desktop.local"):
        env_path = project_root / filename
        if not env_path.exists():
            continue
        try:
            lines = env_path.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for raw_line in lines:
            line = raw_line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            key = key.strip()
            if not key or key in protected_keys:
                continue
            os.environ[key] = value.strip().strip('"').strip("'")


def get_project_root() -> Path:
    return Path(__file__).resolve().parents[3]


def get_runtime_override_env_path() -> Path:
    return get_project_root() / RUNTIME_OVERRIDE_ENV_FILENAME


def _escape_env_value(value: str) -> str:
    return value.replace("\\", "\\\\").replace('"', '\\"')


def _parse_csv_list_env(raw: str) -> list[str]:
    seen: set[str] = set()
    items: list[str] = []
    normalized = str(raw or "").replace("\n", ",")
    for chunk in normalized.split(","):
        value = chunk.strip()
        if not value or value in seen:
            continue
        seen.add(value)
        items.append(value)
    return items


def _parse_desktop_cors_origins(raw: str) -> list[str]:
    origins: list[str] = []
    for candidate in _parse_csv_list_env(raw):
        if candidate in {"null", "file://"}:
            normalized = candidate
        else:
            try:
                parsed = urlsplit(candidate)
                port = parsed.port
            except ValueError:
                continue
            hostname = (parsed.hostname or "").lower()
            if (
                parsed.scheme not in {"http", "https"}
                or hostname not in LOOPBACK_CORS_HOSTS
                or parsed.username is not None
                or parsed.password is not None
                or parsed.path not in {"", "/"}
                or parsed.query
                or parsed.fragment
            ):
                continue
            default_port = 80 if parsed.scheme == "http" else 443
            port_suffix = f":{port}" if port is not None and port != default_port else ""
            rendered_host = f"[{hostname}]" if ":" in hostname else hostname
            normalized = f"{parsed.scheme}://{rendered_host}{port_suffix}"
        if normalized not in origins:
            origins.append(normalized)
    return origins or list(DEFAULT_DESKTOP_CORS_ORIGINS)


def _first_env_value(*names: str) -> str:
    for name in names:
        value = os.getenv(name, "").strip()
        if value:
            return value
    return ""


def _first_secret_env_or_file(*, env_names: tuple[str, ...], file_env_names: tuple[str, ...]) -> str:
    value = _first_env_value(*env_names)
    if value:
        return value
    file_path = _first_env_value(*file_env_names)
    if not file_path:
        return ""
    try:
        return Path(file_path).expanduser().read_text(encoding="utf-8").strip()
    except OSError:
        return ""


def upsert_runtime_override_env_value(key: str, value: str) -> Path:
    target_path = get_runtime_override_env_path()
    key = str(key or "").strip()
    normalized_value = str(value or "").strip()
    if not key:
        raise ValueError("env_key_required")
    serialized = f'{key}="{_escape_env_value(normalized_value)}"'
    try:
        existing_lines = target_path.read_text(encoding="utf-8").splitlines() if target_path.exists() else []
    except OSError:
        existing_lines = []

    next_lines: list[str] = []
    replaced = False
    for raw_line in existing_lines:
        stripped = raw_line.strip()
        if not stripped or stripped.startswith("#") or "=" not in raw_line:
            next_lines.append(raw_line)
            continue
        candidate_key = raw_line.split("=", 1)[0].strip()
        if candidate_key != key:
            next_lines.append(raw_line)
            continue
        next_lines.append(serialized)
        replaced = True

    if not replaced:
        if next_lines and next_lines[-1].strip():
            next_lines.append("")
        next_lines.append(serialized)

    target_path.write_text("\n".join(next_lines).rstrip() + "\n", encoding="utf-8")
    return target_path


class AppSettings(BaseModel):
    app_name: str = "analytix-data-analysis"
    app_env: str = "dev"
    app_version: str = "0.4.0"
    package_edition: str = ""
    api_prefix: str = "/api/v1"
    host: str = "127.0.0.1"
    port: int = 8000
    cors_allow_origins: List[str] = Field(default_factory=lambda: list(DEFAULT_DESKTOP_CORS_ORIGINS))
    data_analysis_auth_token: SecretStr = Field(default_factory=lambda: SecretStr(""), repr=False)
    data_analysis_launch_id: str = Field(default="", repr=False)
    log_level: str = "INFO"
    ws_contract_version: str = "v1"
    stats_job_workers: int = 4
    flow_snapshot_gc_interval_s: int = 1800
    flow_snapshot_gc_startup_delay_s: int = 180
    analysis_temp_scope_gc_interval_s: int = 1800
    analysis_temp_scope_gc_startup_delay_s: int = 120
    task_store_max_records: int = 5000
    task_store_terminal_ttl_days: int = 30
    document_chunk_size: int = 1200
    document_chunk_overlap: int = 160
    document_ocr_enabled: bool = True
    document_ocr_language: str = "chi_sim+eng"
    document_vector_mode: str = "hash256"
    llm_provider: str = "dashscope-compatible"
    llm_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"
    llm_api_key: str = ""
    llm_model: str = "MiniMax-M2.7-highspeed"
    llm_relevant_memory_model: str = ""
    llm_runtime_architecture: str = "codex-aligned"
    llm_dashscope_extra_models: List[str] = Field(default_factory=list)
    llm_dashscope_hidden_models: List[str] = Field(default_factory=list)
    llm_dashscope_responses_shim_enabled: bool = True
    llm_dashscope_responses_shim_trace_enabled: bool = False
    llm_hub_gateway_base_url: str = "https://analytix.top/v1"
    llm_hub_gateway_token: str = ""
    llm_deepseek_base_url: str = "https://api.deepseek.com"
    llm_deepseek_api_key: str = ""
    llm_deepseek_extra_models: List[str] = Field(default_factory=list)
    llm_deepseek_hidden_models: List[str] = Field(default_factory=list)
    llm_deepseek_responses_shim_enabled: bool = True
    llm_deepseek_responses_shim_trace_enabled: bool = False
    llm_vision_bridge_provider: str = "openai-compatible"
    llm_vision_bridge_base_url: str = ""
    llm_vision_bridge_api_key: str = ""
    llm_vision_bridge_model: str = ""
    llm_vision_bridge_timeout_s: int = 60
    llm_computer_use_execution_mode: str = "dry_run"
    llm_minimax_base_url: str = DEFAULT_MINIMAX_BASE_URL
    llm_minimax_api_key: str = DEFAULT_MINIMAX_API_KEY
    llm_minimax_extra_models: List[str] = Field(default_factory=list)
    llm_minimax_hidden_models: List[str] = Field(default_factory=list)
    llm_minimax_responses_shim_enabled: bool = True
    llm_minimax_responses_shim_trace_enabled: bool = False
    internal_admin_token: str = ""
    license_mode: str = "dev"
    license_token: str = ""
    license_public_key: str = ""
    license_machine_fingerprint: str = ""
    license_platform: str = ""
    llm_xchai_base_url: str = "https://xchai.xyz"
    llm_xchai_api_key: str = ""
    llm_xchai_extra_models: List[str] = Field(default_factory=list)
    llm_xchai_hidden_models: List[str] = Field(default_factory=list)
    llm_ollama_base_url: str = "http://127.0.0.1:11434/v1"
    llm_timeout_s: int = 180
    llm_context_max_documents: int = 12
    llm_context_chunk_limit_per_document: int = 6
    llm_context_max_chars: int = 18000
    llm_history_message_limit: int = 12
    llm_sidecars_enabled: bool = True
    llm_extract_memories_enabled: bool = True
    llm_session_memory_scheduler_enabled: bool = True
    llm_session_memory_min_messages_to_init: int = 4
    llm_session_memory_min_messages_between_updates: int = 2
    llm_session_memory_tool_calls_between_updates: int = 1
    llm_session_memory_wait_timeout_ms: int = 1500
    llm_session_memory_stale_threshold_ms: int = 15000
    llm_session_memory_wait_poll_interval_ms: int = 50
    web_search_enabled: bool = True
    web_search_searxng_url: str = DEFAULT_WEB_SEARCH_SEARXNG_URL
    web_search_language: str = "zh-CN"
    web_search_timeout_s: int = 20
    web_search_total_timeout_s: int = 25
    web_search_retry_count: int = 1
    web_search_fetch_timeout_s: int = 10
    web_search_max_results: int = 5
    web_search_max_queries: int = 3
    web_search_fetch_pages: bool = True
    web_search_max_source_chars: int = 2000
    web_search_cache_enabled: bool = True
    web_search_cache_ttl_s: int = 900
    enterprise_lookup_api_url: str = ""
    enterprise_lookup_api_key: str = ""
    enterprise_lookup_timeout_s: int = 15
    enterprise_lookup_web_fallback_enabled: bool = True
    enterprise_lookup_browser_fetch_enabled: bool = True
    enterprise_lookup_browser_fetch_timeout_s: int = 12
    enterprise_lookup_browser_fetch_max_sources: int = 3
    enterprise_lookup_browser_fetch_max_chars: int = 4000
    enterprise_lookup_browser_fetch_wait_ms: int = 500
    enterprise_lookup_browser_sidecar_binary: str = ""
    enterprise_lookup_browser_fetch_node: str = "node"
    enterprise_lookup_browser_fetch_script: str = ""
    enterprise_lookup_user_assisted_browser_enabled: bool = True
    enterprise_lookup_user_assisted_browser_timeout_s: int = 300
    enterprise_lookup_user_assisted_browser_max_prompts: int = 1
    enterprise_lookup_user_assisted_browser_persist_profile: bool = True
    enterprise_lookup_user_assisted_browser_profile_dir: str = ""
    enterprise_lookup_user_assisted_browser_script: str = ""

    @classmethod
    def from_env(cls) -> "AppSettings":
        _load_env_defaults()
        origins = _parse_desktop_cors_origins(os.getenv("ANALYTIX_CORS_ORIGINS", ""))
        app_env = os.getenv("ANALYTIX_APP_ENV", "dev")
        default_runtime_architecture = _default_runtime_architecture(app_env)
        hub_gateway_base_url = (
            os.getenv(
                "ANALYTIX_HUB_GATEWAY_BASE_URL",
                "https://analytix.top/v1",
            ).strip()
            or "https://analytix.top/v1"
        )
        hub_gateway_token = _first_secret_env_or_file(
            env_names=("ANALYTIX_HUB_GATEWAY_TOKEN",),
            file_env_names=("ANALYTIX_HUB_GATEWAY_TOKEN_FILE",),
        )
        vision_bridge_explicit_base_url = os.getenv("ANALYTIX_VISION_BRIDGE_BASE_URL", "").strip()
        vision_bridge_explicit_key = _first_secret_env_or_file(
            env_names=("ANALYTIX_VISION_BRIDGE_API_KEY",),
            file_env_names=("ANALYTIX_VISION_BRIDGE_API_KEY_FILE",),
        )
        vision_bridge_base_url = vision_bridge_explicit_base_url or (
            hub_gateway_base_url if hub_gateway_token else ""
        )
        vision_bridge_api_key = vision_bridge_explicit_key or hub_gateway_token
        vision_bridge_model = os.getenv("ANALYTIX_VISION_BRIDGE_MODEL", "").strip() or (
            DEFAULT_VISION_BRIDGE_MODEL
            if vision_bridge_base_url and vision_bridge_api_key
            else ""
        )

        return cls(
            app_name=os.getenv("ANALYTIX_APP_NAME", "analytix-data-analysis"),
            app_env=app_env,
            app_version=os.getenv("ANALYTIX_APP_VERSION", "0.4.0"),
            package_edition=(
                os.getenv("ANALYTIX_PACKAGE_EDITION", "")
                or os.getenv("ANALYTIX_DESKTOP_PACKAGE_EDITION", "")
            ).strip().lower(),
            api_prefix=os.getenv("ANALYTIX_API_PREFIX", "/api/v1"),
            host=os.getenv("ANALYTIX_HOST", "127.0.0.1"),
            port=int(os.getenv("ANALYTIX_PORT", "8000")),
            cors_allow_origins=origins,
            data_analysis_auth_token=SecretStr(
                os.getenv("ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN", "").strip()
            ),
            data_analysis_launch_id=os.getenv("ANALYTIX_DATA_ANALYSIS_LAUNCH_ID", "").strip(),
            log_level=os.getenv("ANALYTIX_LOG_LEVEL", "INFO"),
            ws_contract_version=os.getenv("ANALYTIX_WS_VERSION", "v1"),
            stats_job_workers=max(1, int(os.getenv("ANALYTIX_STATS_JOB_WORKERS", "4"))),
            flow_snapshot_gc_interval_s=max(0, int(os.getenv("ANALYTIX_FLOW_SNAPSHOT_GC_INTERVAL_S", "1800"))),
            flow_snapshot_gc_startup_delay_s=max(0, int(os.getenv("ANALYTIX_FLOW_SNAPSHOT_GC_STARTUP_DELAY_S", "180"))),
            analysis_temp_scope_gc_interval_s=max(0, int(os.getenv("ANALYTIX_ANALYSIS_TEMP_SCOPE_GC_INTERVAL_S", "1800"))),
            analysis_temp_scope_gc_startup_delay_s=max(0, int(os.getenv("ANALYTIX_ANALYSIS_TEMP_SCOPE_GC_STARTUP_DELAY_S", "120"))),
            task_store_max_records=max(100, int(os.getenv("ANALYTIX_TASK_STORE_MAX_RECORDS", "5000"))),
            task_store_terminal_ttl_days=max(1, int(os.getenv("ANALYTIX_TASK_TERMINAL_TTL_DAYS", "30"))),
            document_chunk_size=max(400, int(os.getenv("ANALYTIX_DOCUMENT_CHUNK_SIZE", "1200"))),
            document_chunk_overlap=max(40, int(os.getenv("ANALYTIX_DOCUMENT_CHUNK_OVERLAP", "160"))),
            document_ocr_enabled=os.getenv("ANALYTIX_DOCUMENT_OCR_ENABLED", "1").strip().lower() not in {"0", "false", "off", "no"},
            document_ocr_language=os.getenv("ANALYTIX_DOCUMENT_OCR_LANGUAGE", "chi_sim+eng"),
            document_vector_mode=os.getenv("ANALYTIX_DOCUMENT_VECTOR_MODE", "hash256"),
            llm_provider=os.getenv("ANALYTIX_LLM_PROVIDER", "dashscope-compatible"),
            llm_base_url=os.getenv("ANALYTIX_LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1").strip(),
            llm_api_key=os.getenv("ANALYTIX_LLM_API_KEY", "").strip(),
            llm_model=os.getenv("ANALYTIX_LLM_MODEL", "MiniMax-M2.7-highspeed").strip() or "MiniMax-M2.7-highspeed",
            llm_relevant_memory_model=os.getenv("ANALYTIX_LLM_RELEVANT_MEMORY_MODEL", "").strip(),
            llm_runtime_architecture=os.getenv("ANALYTIX_LLM_RUNTIME_ARCHITECTURE", default_runtime_architecture).strip()
            or default_runtime_architecture,
            llm_dashscope_extra_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_DASHSCOPE_EXTRA_MODELS", "")),
            llm_dashscope_hidden_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_DASHSCOPE_HIDDEN_MODELS", "")),
            llm_dashscope_responses_shim_enabled=os.getenv(
                "ANALYTIX_LLM_DASHSCOPE_RESPONSES_SHIM_ENABLED",
                "1",
            ).strip().lower()
            in {"1", "true", "on", "yes"},
            llm_dashscope_responses_shim_trace_enabled=os.getenv(
                "ANALYTIX_LLM_DASHSCOPE_RESPONSES_SHIM_TRACE",
                "0",
            ).strip().lower()
            in {"1", "true", "on", "yes"},
            llm_hub_gateway_base_url=hub_gateway_base_url,
            llm_hub_gateway_token=hub_gateway_token,
            llm_deepseek_base_url=os.getenv("ANALYTIX_DEEPSEEK_BASE_URL", "https://api.deepseek.com").strip()
            or "https://api.deepseek.com",
            llm_deepseek_api_key=_first_secret_env_or_file(
                env_names=(
                    "ANALYTIX_AGENT_DEEPSEEK_API_KEY",
                    "ANALYTIX_DEEPSEEK_API_KEY",
                    "ANALYTIX_LLM_DEEPSEEK_API_KEY",
                    "DEEPSEEK_API_KEY",
                ),
                file_env_names=(
                    "ANALYTIX_AGENT_DEEPSEEK_API_KEY_FILE",
                    "ANALYTIX_DEEPSEEK_API_KEY_FILE",
                    "ANALYTIX_LLM_DEEPSEEK_API_KEY_FILE",
                    "DEEPSEEK_API_KEY_FILE",
                ),
            ),
            llm_deepseek_extra_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_DEEPSEEK_EXTRA_MODELS", "")),
            llm_deepseek_hidden_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_DEEPSEEK_HIDDEN_MODELS", "")),
            llm_deepseek_responses_shim_enabled=os.getenv(
                "ANALYTIX_DEEPSEEK_RESPONSES_SHIM_ENABLED",
                "1",
            ).strip().lower()
            in {"1", "true", "on", "yes"},
            llm_deepseek_responses_shim_trace_enabled=os.getenv(
                "ANALYTIX_DEEPSEEK_RESPONSES_SHIM_TRACE",
                "0",
            ).strip().lower()
            in {"1", "true", "on", "yes"},
            llm_vision_bridge_provider=os.getenv(
                "ANALYTIX_VISION_BRIDGE_PROVIDER",
                "openai-compatible",
            ).strip()
            or "openai-compatible",
            llm_vision_bridge_base_url=vision_bridge_base_url,
            llm_vision_bridge_api_key=vision_bridge_api_key,
            llm_vision_bridge_model=vision_bridge_model,
            llm_vision_bridge_timeout_s=max(
                5,
                int(os.getenv("ANALYTIX_VISION_BRIDGE_TIMEOUT_S", "60")),
            ),
            llm_computer_use_execution_mode=(
                os.getenv(
                    "ANALYTIX_COMPUTER_USE_EXECUTION_MODE",
                    "real"
                    if os.getenv("ANALYTIX_AGENT_RUNTIME_PROFILE", "")
                    .strip()
                    .lower()
                    in {
                        "bridge",
                        "codex-bridge",
                        "analytix-bridge",
                        "analytix-codex-bridge",
                        "commercial",
                        "packaged",
                        "deepseek",
                        "qwen",
                    }
                    else "dry_run",
                ).strip()
                or "dry_run"
            ),
            llm_minimax_base_url=os.getenv("ANALYTIX_MINIMAX_BASE_URL", DEFAULT_MINIMAX_BASE_URL).strip()
            or DEFAULT_MINIMAX_BASE_URL,
            llm_minimax_api_key=os.getenv("ANALYTIX_MINIMAX_API_KEY", DEFAULT_MINIMAX_API_KEY).strip()
            or DEFAULT_MINIMAX_API_KEY,
            llm_minimax_extra_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_MINIMAX_EXTRA_MODELS", "")),
            llm_minimax_hidden_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_MINIMAX_HIDDEN_MODELS", "")),
            llm_minimax_responses_shim_enabled=os.getenv(
                "ANALYTIX_MINIMAX_RESPONSES_SHIM_ENABLED",
                "1",
            ).strip().lower()
            not in {"0", "false", "off", "no"},
            llm_minimax_responses_shim_trace_enabled=os.getenv(
                "ANALYTIX_MINIMAX_RESPONSES_SHIM_TRACE",
                "0",
            ).strip().lower()
            in {"1", "true", "on", "yes"},
            internal_admin_token=os.getenv("ANALYTIX_INTERNAL_ADMIN_TOKEN", "").strip(),
            license_mode=os.getenv("ANALYTIX_LICENSE_MODE", "dev").strip().lower() or "dev",
            license_token=os.getenv("ANALYTIX_LICENSE_TOKEN", "").strip(),
            license_public_key=os.getenv("ANALYTIX_LICENSE_PUBLIC_KEY", "").strip(),
            license_machine_fingerprint=os.getenv("ANALYTIX_LICENSE_MACHINE_FINGERPRINT", "").strip(),
            license_platform=os.getenv("ANALYTIX_LICENSE_PLATFORM", "").strip(),
            llm_xchai_base_url=os.getenv("ANALYTIX_XCHAI_BASE_URL", "https://xchai.xyz").strip() or "https://xchai.xyz",
            llm_xchai_api_key=os.getenv("ANALYTIX_XCHAI_API_KEY", "").strip(),
            llm_xchai_extra_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_XCHAI_EXTRA_MODELS", "")),
            llm_xchai_hidden_models=_parse_csv_list_env(os.getenv("ANALYTIX_LLM_XCHAI_HIDDEN_MODELS", "")),
            llm_ollama_base_url=os.getenv("ANALYTIX_OLLAMA_BASE_URL", "http://127.0.0.1:11434/v1").strip()
            or "http://127.0.0.1:11434/v1",
            llm_timeout_s=max(15, int(os.getenv("ANALYTIX_LLM_TIMEOUT_S", "180"))),
            llm_context_max_documents=max(1, int(os.getenv("ANALYTIX_LLM_CONTEXT_MAX_DOCUMENTS", "12"))),
            llm_context_chunk_limit_per_document=max(1, int(os.getenv("ANALYTIX_LLM_CONTEXT_CHUNK_LIMIT_PER_DOCUMENT", "6"))),
            llm_context_max_chars=max(2000, int(os.getenv("ANALYTIX_LLM_CONTEXT_MAX_CHARS", "18000"))),
            llm_history_message_limit=max(2, int(os.getenv("ANALYTIX_LLM_HISTORY_MESSAGE_LIMIT", "12"))),
            llm_sidecars_enabled=os.getenv("ANALYTIX_LLM_SIDECARS_ENABLED", "1").strip().lower() not in {"0", "false", "off", "no"},
            llm_extract_memories_enabled=os.getenv("ANALYTIX_LLM_EXTRACT_MEMORIES_ENABLED", "1").strip().lower() not in {"0", "false", "off", "no"},
            llm_session_memory_scheduler_enabled=os.getenv("ANALYTIX_LLM_SESSION_MEMORY_SCHEDULER_ENABLED", "1").strip().lower() not in {"0", "false", "off", "no"},
            llm_session_memory_min_messages_to_init=max(1, int(os.getenv("ANALYTIX_LLM_SESSION_MEMORY_MIN_MESSAGES_TO_INIT", "4"))),
            llm_session_memory_min_messages_between_updates=max(1, int(os.getenv("ANALYTIX_LLM_SESSION_MEMORY_MIN_MESSAGES_BETWEEN_UPDATES", "2"))),
            llm_session_memory_tool_calls_between_updates=max(0, int(os.getenv("ANALYTIX_LLM_SESSION_MEMORY_TOOL_CALLS_BETWEEN_UPDATES", "1"))),
            llm_session_memory_wait_timeout_ms=max(0, int(os.getenv("ANALYTIX_LLM_SESSION_MEMORY_WAIT_TIMEOUT_MS", "1500"))),
            llm_session_memory_stale_threshold_ms=max(1, int(os.getenv("ANALYTIX_LLM_SESSION_MEMORY_STALE_THRESHOLD_MS", "15000"))),
            llm_session_memory_wait_poll_interval_ms=max(10, int(os.getenv("ANALYTIX_LLM_SESSION_MEMORY_WAIT_POLL_INTERVAL_MS", "50"))),
            web_search_enabled=os.getenv("ANALYTIX_WEB_SEARCH_ENABLED", "1").strip().lower()
            not in {"0", "false", "off", "no"},
            web_search_searxng_url=os.getenv(
                "ANALYTIX_WEB_SEARCH_SEARXNG_URL",
                DEFAULT_WEB_SEARCH_SEARXNG_URL,
            ).strip()
            or DEFAULT_WEB_SEARCH_SEARXNG_URL,
            web_search_language=os.getenv("ANALYTIX_WEB_SEARCH_LANGUAGE", "zh-CN").strip() or "zh-CN",
            web_search_timeout_s=max(2, int(os.getenv("ANALYTIX_WEB_SEARCH_TIMEOUT_S", "20"))),
            web_search_total_timeout_s=max(5, int(os.getenv("ANALYTIX_WEB_SEARCH_TOTAL_TIMEOUT_S", "25"))),
            web_search_retry_count=max(0, min(int(os.getenv("ANALYTIX_WEB_SEARCH_RETRY_COUNT", "1")), 3)),
            web_search_fetch_timeout_s=max(2, int(os.getenv("ANALYTIX_WEB_SEARCH_FETCH_TIMEOUT_S", "10"))),
            web_search_max_results=max(1, min(int(os.getenv("ANALYTIX_WEB_SEARCH_MAX_RESULTS", "5")), 10)),
            web_search_max_queries=max(1, min(int(os.getenv("ANALYTIX_WEB_SEARCH_MAX_QUERIES", "3")), 8)),
            web_search_fetch_pages=os.getenv("ANALYTIX_WEB_SEARCH_FETCH_PAGES", "1").strip().lower()
            not in {"0", "false", "off", "no"},
            web_search_max_source_chars=max(400, min(int(os.getenv("ANALYTIX_WEB_SEARCH_MAX_SOURCE_CHARS", "2000")), 8000)),
            web_search_cache_enabled=os.getenv("ANALYTIX_WEB_SEARCH_CACHE_ENABLED", "1").strip().lower()
            not in {"0", "false", "off", "no"},
            web_search_cache_ttl_s=max(0, min(int(os.getenv("ANALYTIX_WEB_SEARCH_CACHE_TTL_S", "900")), 86400)),
            enterprise_lookup_api_url=os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_API_URL", "").strip(),
            enterprise_lookup_api_key=os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_API_KEY", "").strip(),
            enterprise_lookup_timeout_s=max(2, int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_TIMEOUT_S", "15"))),
            enterprise_lookup_web_fallback_enabled=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_WEB_FALLBACK_ENABLED",
                "1",
            ).strip().lower()
            not in {"0", "false", "off", "no"},
            enterprise_lookup_browser_fetch_enabled=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_ENABLED",
                "1",
            ).strip().lower()
            not in {"0", "false", "off", "no"},
            enterprise_lookup_browser_fetch_timeout_s=max(
                2,
                int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_TIMEOUT_S", "12")),
            ),
            enterprise_lookup_browser_fetch_max_sources=max(
                0,
                min(int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_MAX_SOURCES", "3")), 8),
            ),
            enterprise_lookup_browser_fetch_max_chars=max(
                400,
                min(int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_MAX_CHARS", "4000")), 12000),
            ),
            enterprise_lookup_browser_fetch_wait_ms=max(
                0,
                min(int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_WAIT_MS", "500")), 5000),
            ),
            enterprise_lookup_browser_sidecar_binary=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_SIDECAR_BINARY",
                os.getenv("ANALYTIX_BROWSER_SIDECAR_BINARY", ""),
            ).strip(),
            enterprise_lookup_browser_fetch_node=os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_NODE", "node").strip()
            or "node",
            enterprise_lookup_browser_fetch_script=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_BROWSER_FETCH_SCRIPT",
                "",
            ).strip(),
            enterprise_lookup_user_assisted_browser_enabled=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_USER_ASSISTED_BROWSER_ENABLED",
                "1",
            ).strip().lower()
            not in {"0", "false", "off", "no"},
            enterprise_lookup_user_assisted_browser_timeout_s=max(
                30,
                min(int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_USER_ASSISTED_BROWSER_TIMEOUT_S", "300")), 1800),
            ),
            enterprise_lookup_user_assisted_browser_max_prompts=max(
                0,
                min(int(os.getenv("ANALYTIX_ENTERPRISE_LOOKUP_USER_ASSISTED_BROWSER_MAX_PROMPTS", "1")), 5),
            ),
            enterprise_lookup_user_assisted_browser_persist_profile=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_USER_ASSISTED_BROWSER_PERSIST_PROFILE",
                "1",
            ).strip().lower()
            not in {"0", "false", "off", "no"},
            enterprise_lookup_user_assisted_browser_profile_dir=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_USER_ASSISTED_BROWSER_PROFILE_DIR",
                "",
            ).strip(),
            enterprise_lookup_user_assisted_browser_script=os.getenv(
                "ANALYTIX_ENTERPRISE_LOOKUP_USER_ASSISTED_BROWSER_SCRIPT",
                "",
            ).strip(),
        )


@lru_cache(maxsize=1)
def get_settings() -> AppSettings:
    return AppSettings.from_env()
