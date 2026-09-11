export const CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED = "controlled_artifact_publication_required";
export const CONTROLLED_SOURCE_INGESTION_REQUIRED = "controlled_source_ingestion_required";

export function controlledArtifactPublicationBlockReason(): string {
  return "受控文件发布能力尚未完成；当前不会创建导出任务或写入案件文件。";
}

export function controlledSourceIngestionBlockReason(): string {
  return "受控文件采集能力尚未完成；当前不会把本地路径交给数据分析后端读取。";
}

export function requireControlledArtifactPublication(): void {
  throw new Error(CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED);
}

export function requireControlledSourceIngestion(): void {
  throw new Error(CONTROLLED_SOURCE_INGESTION_REQUIRED);
}
