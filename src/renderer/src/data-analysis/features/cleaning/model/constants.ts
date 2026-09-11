import type { StepCatalogItem } from "./types";

export const STEP_CATALOG: StepCatalogItem[] = [
  { step: 1, title: "金额/余额数值化", kind: "处理", description: "修正金额与余额字段中的异常格式。" },
  { step: 2, title: "无效数据标记", kind: "标记", description: "识别缺字段、空时间与异常记录。" },
  { step: 3, title: "重复数据标记", kind: "停用", description: "保留旧版重复标记视角用于审计。" },
  { step: 4, title: "交易失败/冲正", kind: "标记", description: "标记失败流水与冲正交易。" },
  { step: 5, title: "收付标志修正", kind: "修正", description: "规范进出方向并补全收付标志。" },
  { step: 6, title: "交易卡号补全", kind: "补全", description: "根据上下文补齐缺失交易卡号。" },
  { step: 7, title: "交易明细后缀处理", kind: "处理", description: "清理卡号与账号后缀噪声。" },
  { step: 8, title: "账户信息表清洗", kind: "处理", description: "清洗账户表无效与异常字段。" },
  { step: 9, title: "账户信息后缀处理", kind: "处理", description: "统一账户表主键与后缀格式。" },
  { step: 10, title: "账户开户名称及证件号补全", kind: "补全", description: "基于交易聚合补全账户身份信息。" }
];

export const CLEANING_PAGE_DESIGN_VIEWPORT_WIDTH = 1600;
export const CLEANING_PAGE_DESIGN_CONTENT_WIDTH = 1260;
export const CLEANING_PAGE_DESIGN_VIEWPORT_HEIGHT = 920;
export const CLEANING_PAGE_MIN_SCALE = 0.78;
export const CLEANING_PAGE_MIN_CONTENT_WIDTH = Math.round(CLEANING_PAGE_DESIGN_CONTENT_WIDTH * CLEANING_PAGE_MIN_SCALE);
export const EXPORT_TRACK_POLL_OFFLINE_MS = 1200;
export const EXPORT_TRACK_POLL_WS_FALLBACK_MS = 8000;
export const CLEANING_STEP_DETAIL_INITIAL_FETCH_SIZE = 50;
export const CLEANING_STEP_DETAIL_PRIORITY_TARGET_ROWS = 500;
export const CLEANING_STEP_DETAIL_APPEND_CHUNK_SIZE = 500;
export const CLEANING_STEP_DETAIL_APPEND_CONCURRENCY = 4;
export const CLEANING_STEP_TXN_ROW_HEIGHT = 32;
export const CLEANING_STEP_TXN_HEADER_HEIGHT = 32;
export const CLEANING_STEP_TXN_OVERSCAN_ROWS = 16;
