import { ApiClientError } from "../../../services/http/client";
import { toErrorMessage } from "../../shared/errors";

export function toButtonActionErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "CLEANING_JOB_ACTIVE") {
      return "当前已有清洗任务在执行，请等待完成后再试。";
    }
    if (error.code === "NATIVE_CLEANING_UNAVAILABLE") {
      return "Rust 清洗运行时不可用，请重新构建当前平台的原生数据工具后再启动清洗。";
    }
    if (error.code === "EXPORT_JOB_ACTIVE") {
      return "当前已有导出任务在执行，请等待当前导出完成后再试。";
    }
    if (error.code === "DATASET_NOT_READY") {
      return "请先完成数据清洗，再导出已清洗数据。";
    }
  }
  return toErrorMessage(error);
}
