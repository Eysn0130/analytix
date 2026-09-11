import type { BackendHealth } from "../../../services/http/system";

export function getCleaningRuntimeReasonLabel(reason: string): string {
  if (reason === "native_binary_not_found") {
    return "未找到 Rust 清洗 binary";
  }
  if (reason === "native_binary_not_file") {
    return "Rust 清洗路径不是可执行文件";
  }
  if (reason === "native_binary_not_executable") {
    return "Rust 清洗 binary 缺少执行权限";
  }
  if (reason === "CLEANING_SERVICE_UNAVAILABLE") {
    return "清洗服务尚未就绪";
  }
  return reason || "运行时未就绪";
}

export function getCleaningRuntimeUnavailableMessage(health: BackendHealth | null): string {
  if (!health) {
    return "清洗运行时状态尚未确认。请等待健康检查完成后重试。";
  }
  if (health.cleaning_native_available === true || health.legacy_python_cleaning_allowed === true) {
    return "";
  }
  const reason = getCleaningRuntimeReasonLabel(String(health.cleaning_native_reason || ""));
  return `Rust 清洗运行时不可用：${reason}。请重新构建当前平台的原生数据工具后再启动清洗。`;
}
