import { beforeEach, describe, expect, it, vi } from "vitest";
import sharedBridgeSource from "../../../../shared/data-analysis.ts?raw";
import preloadBridgeSource from "../../../../preload/index.ts?raw";
import mainIpcSource from "../../../../main/data-analysis/ipc.ts?raw";
import browserBridgeSource from "../../lib/browser-analytix-bridge.ts?raw";
import chartDashboardSource from "../features/analysis/components/charts/ChartAnalysisDashboard.tsx?raw";
import analysisPageSource from "../features/analysis/AnalysisPage.tsx?raw";
import cleaningActionsSource from "../features/cleaning/runtime/useCleaningActions.ts?raw";
import flowExportSource from "../features/flow/graph/adapters/flow-export-adapter.ts?raw";
import importPageSource from "../features/import/ImportPage.tsx?raw";
import importApiSource from "./import/api.ts?raw";
import statsApiSource from "./analysis/stats-api.ts?raw";
import desktopClientSource from "./desktop/client.ts?raw";

const mocks = vi.hoisted(() => ({
  post: vi.fn(),
}));

vi.mock("./http/client", () => ({
  httpClient: {
    post: mocks.post,
  },
}));

import {
  createCleanedExportJob,
  createRawExportJob,
  exportCaseArchive,
  importCaseArchive,
} from "../features/cases/api";
import { createImportJob, previewImportFiles } from "./import/api";
import {
  CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
  CONTROLLED_SOURCE_INGESTION_REQUIRED,
} from "./publication-quarantine";

beforeEach(() => {
  mocks.post.mockReset();
});

describe("data analysis P0 artifact and source quarantine", () => {
  it("blocks every renderer export creation path before sending a host path to Python", async () => {
    const arbitraryPath = "/tmp/renderer-controlled/report.xlsx";
    const attempts = [
      () => createRawExportJob({
        case_id: "case_a",
        export_format: "xlsx",
        filters: {},
        output_name: "raw",
        target_dir: arbitraryPath,
      }),
      () => createCleanedExportJob({
        case_id: "case_a",
        export_format: "xlsx",
        filters: {},
        output_name: "cleaned",
        target_dir: undefined,
      }),
      () => exportCaseArchive("case_a", { target_dir: arbitraryPath }),
    ];

    for (const attempt of [...attempts, ...attempts]) {
      await expect(attempt()).rejects.toThrow(CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED);
    }
    expect(mocks.post).not.toHaveBeenCalled();
  });

  it("blocks renderer-controlled local source paths before any Python request", async () => {
    const sourcePath = "/etc/passwd";
    const attempts = [
      () => previewImportFiles("case_a", [{ file_name: "passwd", source_path: sourcePath }]),
      () => createImportJob({
        case_id: "case_a",
        files: [{ file_name: "passwd", source_path: sourcePath }],
        auto_cleaning: false,
      }),
      () => importCaseArchive({ archive_path: sourcePath }),
    ];

    for (const attempt of attempts) {
      await expect(attempt()).rejects.toThrow(CONTROLLED_SOURCE_INGESTION_REQUIRED);
    }
    expect(mocks.post).not.toHaveBeenCalled();
  });

  it("blocks drag-and-drop before reading renderer-visible local paths", () => {
    const dropStart = importPageSource.indexOf("const onDropZoneDrop");
    const dropEnd = importPageSource.indexOf("const onOpenDbDir", dropStart);
    expect(dropStart).toBeGreaterThanOrEqual(0);
    expect(dropEnd).toBeGreaterThan(dropStart);
    const dropHandler = importPageSource.slice(dropStart, dropEnd);
    expect(dropHandler).toContain("controlledSourceIngestionBlockReason()");
    expect(dropHandler).not.toContain("dataTransfer.files");
    expect(dropHandler).not.toMatch(/\.path\b/u);
    expect(dropHandler).not.toContain("previewSelectedFiles");
  });

  it("blocks legacy import retry before reading persisted source paths", () => {
    const retryStart = importPageSource.indexOf("const onRetryExecutionRow");
    const retryEnd = importPageSource.indexOf("const importPageStyle", retryStart);
    expect(retryStart).toBeGreaterThanOrEqual(0);
    expect(retryEnd).toBeGreaterThan(retryStart);
    const retryHandler = importPageSource.slice(retryStart, retryEnd);
    expect(retryHandler).toContain("controlledSourceIngestionBlockReason()");
    expect(retryHandler).not.toContain("rowPaths");
    expect(retryHandler).not.toContain("previewSelectedFiles");
    expect(retryHandler).not.toContain('tone: "success"');
  });

  it("does not expose a renderer client for Python runtime storage paths", () => {
    expect(importApiSource).not.toContain("runtime-paths");
    expect(importApiSource).not.toContain("getImportCasePaths");
    expect(importApiSource).not.toContain("ImportCasePathsDTO");
  });

  it("serializes exact top-level and archive-child sizes with their digests", () => {
    expect(importPageSource).toContain("expected_sha256: file.sha256 || undefined");
    expect(importPageSource).toContain("expected_size: file.size");
    expect(importPageSource).toContain("expected_sha256: child.sha256");
    expect(importPageSource).toContain("expected_size: child.size");
  });

  it("contains case-data exports without a renderer browser-download bypass", () => {
    for (const source of [chartDashboardSource, flowExportSource]) {
      expect(source).not.toContain("URL.createObjectURL");
      expect(source).not.toMatch(/\.download\s*=/u);
      expect(source).not.toContain("pickSaveFile");
      expect(source).not.toContain("writeFile(");
    }
    expect(chartDashboardSource).toContain("controlledArtifactPublicationBlockReason()");
    expect(flowExportSource).toContain("requireControlledArtifactPublication()");
  });

  it("removes the legacy stats query-job and renderer publication clients", () => {
    for (const source of [statsApiSource, analysisPageSource]) {
      expect(source).not.toContain("StatsV2QueryJobDTO");
      expect(source).not.toContain("result_ref");
      expect(source).not.toContain("source_result_ref");
      expect(source).not.toContain("createStatsV2QueryJob");
      expect(source).not.toContain("createStatsV2ExportJob");
      expect(source).not.toContain("getStatsV2ExportJob");
      expect(source).not.toContain("导出 Excel");
    }
  });

  it("DataAnalysisBridgeHasNoRendererOwnedPublicationChannels", () => {
    const bridgeAndHostSources = [
      sharedBridgeSource,
      preloadBridgeSource,
      mainIpcSource,
      browserBridgeSource,
      desktopClientSource,
    ];
    const rendererSources = [analysisPageSource, cleaningActionsSource, importPageSource];

    for (const source of bridgeAndHostSources) {
      expect(source).not.toContain("pickSaveFile");
      expect(source).not.toContain("data-analysis:pick-save-file");
      expect(source).not.toContain("data-analysis:write-file");
      expect(source).not.toContain("data-analysis:open-path");
      expect(source).not.toContain("openDesktopPath");
    }
    for (const source of rendererSources) {
      expect(source).not.toContain("pickSaveFile");
      expect(source).not.toContain("openDesktopPath");
      expect(source).not.toMatch(/target_dir\s*:\s*(?:picked|selected|target)Dir\b/u);
    }
  });
});
