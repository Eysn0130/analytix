import os
from pathlib import Path

APP_NAME = "analytix"
DATA_ANALYSIS_DIR_NAME = "data-analysis"

def get_app_data_dir() -> Path:
    explicit = os.environ.get("ANALYTIX_DATA_ANALYSIS_DIR", "").strip()
    if explicit:
        return Path(explicit).expanduser()
    # 优先使用 LOCALAPPDATA（Win10/11）
    local = os.environ.get("LOCALAPPDATA")
    if local:
        return Path(local) / APP_NAME / DATA_ANALYSIS_DIR_NAME
    # 兜底：用户目录
    return Path.home() / f".{APP_NAME}" / DATA_ANALYSIS_DIR_NAME

def ensure_dir(p: Path) -> Path:
    p.mkdir(parents=True, exist_ok=True)
    return p
