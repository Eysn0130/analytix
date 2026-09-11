import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { FlowTreePresentationList } from "../flow/graph/components/FlowShell";
import { buildFlowExportDownloadName, exportFlowGraphFromPayload } from "../flow/graph/adapters/flow-export-adapter";
import { FlowNodeInfoPresentation } from "../flow/graph/interactions/FlowInfoPopovers";
import { sanitizeEmbeddedFlowShellState } from "../flow/graph/runtime/embedded-flow-runtime";
import type { FlowShellBridge, FlowShellState } from "../flow/graph/runtime/flow-runtime";
import graphLabelPipelineScript from "../flow/runtime/embedded/analysis_flow/graph-label-pipeline.js?raw";
import overlayAdapterScript from "../flow/runtime/embedded/analysis_flow/overlay-adapter.js?raw";
import canvasDrillControllerScript from "../flow/runtime/embedded/analysis_flow/canvas-drill-controller.js?raw";
import flowViewSyncStoreScript from "../flow/runtime/embedded/analysis_flow/flow-view-sync-store.js?raw";
import restrictedPiiProjectionScript from "../flow/runtime/embedded/analysis_flow/restricted-pii-projection.js?raw";
import flowShellStateStoreScript from "../flow/runtime/embedded/analysis_flow/flow-shell-state-store.js?raw";
import {
  normalizeOrdinaryPresentationFieldName,
  projectDetectedOrdinaryRestrictedPii,
  projectOrdinaryFieldValue,
  resolveRestrictedPiiKind,
} from "./ordinary-pii-projection";
import { CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED } from "../../services/publication-quarantine";

type EmbeddedProjection = {
  normalizeFieldName(fieldName: unknown): string;
  resolveKind(fieldName: unknown): string | null;
  projectField(fieldName: unknown, value: unknown): string;
  projectDetected(value: unknown): string;
};

type EmbeddedWindow = {
  __ANALYTIX_ORDINARY_PII_PROJECTION__?: EmbeddedProjection;
  __ANALYTIX_FLOW_SHELL_STATE_STORE__?: {
    createFlowShellStateStore(options: Record<string, unknown>): {
      build(): unknown;
      buildLegacyTreeOuterHtml(options: Record<string, unknown>): string;
      cloneShellTreeData(): unknown[];
      presentationIdFor(kind: unknown, value: unknown): string;
      resolvePresentationId(value: unknown): string;
    };
  };
  __ANALYTIX_GRAPH_ENGINE_PRIMITIVES__?: Record<string, unknown>;
  __ANALYTIX_GRAPH_LABEL_PIPELINE__?: {
    resolveNodeLabelRows(model: Record<string, unknown>, sizeScale?: number): { rows: Array<{ text: string }> };
    resolveEdgeLabelRows(model: Record<string, unknown>): {
      label: string;
      labelTop: string;
      labelBottom: string;
    };
  };
  __ANALYTIX_FLOW_OVERLAY_ADAPTER__?: {
    buildNodeHoverPresentation(model: Record<string, unknown>, accounts: string[]): {
      title: string;
      accounts: string[];
    };
  };
  __ANALYTIX_FLOW_CANVAS_DRILL_CONTROLLER__?: {
    projectLogPayload(value: unknown): unknown;
  };
  __ANALYTIX_FLOW_VIEW_SYNC_STORE__?: {
    createFlowViewSyncStore(options: Record<string, unknown>): {
      updateViewCard(): void;
    };
  };
};

function runEmbeddedScript(script: string, targetWindow: EmbeddedWindow): void {
  Function("window", script)(targetWindow);
}

function createEmbeddedProjectionWindow(): EmbeddedWindow {
  const targetWindow: EmbeddedWindow = {};
  runEmbeddedScript(restrictedPiiProjectionScript, targetWindow);
  return targetWindow;
}

describe("ordinary PII presentation boundaries", () => {
  it("keeps TypeScript and embedded JavaScript projection vectors in parity", () => {
    const targetWindow = createEmbeddedProjectionWindow();
    const embedded = targetWindow.__ANALYTIX_ORDINARY_PII_PROJECTION__;
    expect(embedded).toBeTruthy();

    const fieldVectors: Array<[string, string]> = [
      ["counterpartyAccount", "6214600780000579708"],
      ["card\u200bNo", "６２１４６００７８００００５７９７０８"],
      ["counterpartyIdNo", "32031177070600123X"],
      ["phoneNo", "13800138000"],
      ["description", "普通记录 6214600780000579708 元"],
      ["amount", "6214600780000579708"],
      ["txnTime", "2026-07-15 12:34:56"],
    ];
    fieldVectors.forEach(([fieldName, value]) => {
      expect(embedded?.normalizeFieldName(fieldName)).toBe(normalizeOrdinaryPresentationFieldName(fieldName));
      expect(embedded?.resolveKind(fieldName)).toBe(resolveRestrictedPiiKind(fieldName));
      expect(embedded?.projectField(fieldName, value)).toBe(projectOrdinaryFieldValue(fieldName, value));
    });

    [
      "账号：6214－6007－8000－0579－708",
      "联系 13800138000，IP 192.168.10.24",
      "AA:BB:CC:DD:EE:FF",
      "6214600780000579708",
      "图6214600780000579708",
      "6.2146007800005797e+18",
      "金额：6.2146007800005797e+18",
      "账号：6.2146007800005797e+18",
      "description",
    ].forEach((value) => {
      expect(embedded?.projectDetected(value)).toBe(projectDetectedOrdinaryRestrictedPii(value));
    });
  });

  it("renders legacy tree outerHTML only from opaque tokens and masked values", () => {
    const rawAccount = "6214600780000579708";
    const rawIdentity = "32031177070600123X";
    const rawPhone = "13800138000";
    const targetWindow = createEmbeddedProjectionWindow();
    runEmbeddedScript(flowShellStateStoreScript, targetWindow);
    const state = {
      caseId: "case-a",
      tab: "byName",
      treeData: [
        {
          id: rawIdentity,
          title: `张三 ${rawPhone}`,
          meta: rawIdentity,
          extra: `<img src=x onerror=alert(1)> 账号：${rawAccount}`,
          items: [{ id: rawAccount, title: rawAccount, sub: "IP 192.168.10.24" }],
        },
      ],
    };
    let generation = 0;
    const store = targetWindow.__ANALYTIX_FLOW_SHELL_STATE_STORE__?.createFlowShellStateStore({
      state,
      deps: { allocatePresentationGeneration: () => ++generation },
    });
    const html = store?.buildLegacyTreeOuterHtml({
      search: rawAccount,
      expandedIds: [rawIdentity],
      selectedIds: [rawAccount],
      icons: { chevron: "<svg></svg>", check: "<svg></svg>" },
    });

    expect(html).toContain('data-gid="flow-tree-group-1-1"');
    expect(html).toContain('data-iid="flow-tree-account-1-2"');
    expect(html).toContain("6214***********9708");
    expect(html).toContain("320***********123X");
    expect(html).toContain("138****8000");
    expect(html).toContain("192.168.*.*");
    expect(html).toContain("&lt;img src=x onerror=alert(1)&gt;");
    expect(html).not.toContain(rawAccount);
    expect(html).not.toContain(rawIdentity);
    expect(html).not.toContain(rawPhone);
    expect(html).not.toContain("<img src=x");

    state.caseId = "case-b";
    expect(store?.resolvePresentationId("flow-tree-account-1-2")).toBe("");
    state.treeData = [
      {
        id: "group-b",
        title: "李四",
        meta: "无证件号",
        extra: "",
        items: [{ id: "account-b", title: "12345678", sub: "" }],
      },
    ];
    const nextHtml = store?.buildLegacyTreeOuterHtml({ selectedIds: ["account-b"] });
    expect(nextHtml).toContain('data-iid="flow-tree-account-3-2"');
    expect(nextHtml).not.toContain('data-iid="flow-tree-account-1-2"');
    expect(store?.resolvePresentationId("flow-tree-account-3-2")).toBe("account-b");
  });

  it("does not retain raw-to-token maps in ordinary presentation code", () => {
    for (const script of [restrictedPiiProjectionScript, flowShellStateStoreScript]) {
      expect(script).not.toContain("tokenByRaw");
      expect(script).not.toContain("rawByToken");
      expect(script).not.toContain("createTokenRegistry");
    }
  });

  it("invalidates stale presentation tokens after in-place mutation and across stores", () => {
    const targetWindow = createEmbeddedProjectionWindow();
    runEmbeddedScript(flowShellStateStoreScript, targetWindow);
    let generation = 100;
    const allocatePresentationGeneration = () => ++generation;
    const firstState = {
      caseId: "case-a",
      tab: "byName",
      treeData: [{ id: "group-a", items: [{ id: "account-a" }] }],
    };
    const firstStore = targetWindow.__ANALYTIX_FLOW_SHELL_STATE_STORE__?.createFlowShellStateStore({
      state: firstState,
      deps: { allocatePresentationGeneration },
    });
    expect(firstStore?.presentationIdFor("account", "account-a")).toBe("flow-tree-account-101-2");

    firstState.treeData[0].items[0].id = "account-mutated";
    expect(firstStore?.resolvePresentationId("flow-tree-account-101-2")).toBe("");
    expect(firstStore?.presentationIdFor("account", "account-mutated")).toBe("flow-tree-account-102-2");

    const secondStore = targetWindow.__ANALYTIX_FLOW_SHELL_STATE_STORE__?.createFlowShellStateStore({
      state: {
        caseId: "case-b",
        tab: "byName",
        treeData: [{ id: "group-b", items: [{ id: "account-b" }] }],
      },
      deps: { allocatePresentationGeneration },
    });
    expect(secondStore?.presentationIdFor("account", "account-b")).toBe("flow-tree-account-103-2");
    expect(secondStore?.resolvePresentationId("flow-tree-account-101-2")).toBe("");
  });

  it("preserves opaque account items across embedded store, host sanitizer, and React rendering", () => {
    const rawAccount = "6214600780000579708";
    const targetWindow = createEmbeddedProjectionWindow();
    runEmbeddedScript(flowShellStateStoreScript, targetWindow);
    let generation = 40;
    const store = targetWindow.__ANALYTIX_FLOW_SHELL_STATE_STORE__?.createFlowShellStateStore({
      state: {
        caseId: "case-a",
        tab: "byName",
        treeData: [
          {
            id: "group-a",
            title: "张三",
            meta: "未登记户名",
            extra: "",
            items: [{ id: rawAccount, title: rawAccount, sub: "" }],
          },
        ],
        expanded: new Set(["group-a"]),
        selected: new Set([rawAccount]),
      },
      deps: { allocatePresentationGeneration: () => ++generation },
    });
    const state = sanitizeEmbeddedFlowShellState(store?.build());
    const treeHtml = renderToStaticMarkup(
      createElement(FlowTreePresentationList, {
        bridge: { commands: {} } as unknown as FlowShellBridge,
        state,
      })
    );

    expect(state.treeData[0]?.items[0]?.id).toBe("flow-tree-account-41-2");
    expect(state.selectedIds).toEqual(["flow-tree-account-41-2"]);
    expect(treeHtml).toContain('data-iid="flow-tree-account-41-2"');
    expect(treeHtml).toContain("6214***********9708");
    expect(treeHtml).not.toContain(rawAccount);
  });

  it("renders React tree and node popover markup without raw identifiers", () => {
    const rawAccount = "6214600780000579708";
    const rawIdentity = "32031177070600123X";
    const rawPhone = "13800138000";
    const state = sanitizeEmbeddedFlowShellState({
      caseId: "case-a",
      tab: "byName",
      treeData: [
        {
          id: "flow-tree-group-1-1",
          title: `用户 ${rawPhone}`,
          meta: rawIdentity,
          extra: `账号：${rawAccount}`,
          items: [
            {
              id: "flow-tree-account-1-1",
              title: rawAccount,
              sub: `联系电话 ${rawPhone}`,
            },
          ],
        },
      ],
      expandedIds: ["flow-tree-group-1-1"],
      selectedIds: ["flow-tree-account-1-1"],
    });
    const bridge = { commands: {} } as unknown as FlowShellBridge;
    const treeHtml = renderToStaticMarkup(createElement(FlowTreePresentationList, { bridge, state }));

    const nodeInfo = {
      ...state.overlay.nodeInfo,
      preview: { size: 32, fill: "#fff", stroke: "#000", lineWidth: 2 },
      category: `联系电话 ${rawPhone}`,
      userName: `账号：${rawAccount}`,
      accountLabel: "银行账号",
      accountValue: rawAccount,
      accounts: [rawAccount],
    } satisfies FlowShellState["overlay"]["nodeInfo"];
    const popoverHtml = renderToStaticMarkup(createElement(FlowNodeInfoPresentation, { nodeInfo }));
    const outerHtml = `${treeHtml}${popoverHtml}`;

    expect(outerHtml).toContain('data-gid="flow-tree-group-1-1"');
    expect(outerHtml).toContain('data-iid="flow-tree-account-1-1"');
    expect(outerHtml).toContain("6214***********9708");
    expect(outerHtml).toContain("320***********123X");
    expect(outerHtml).toContain("138****8000");
    expect(outerHtml).not.toContain(rawAccount);
    expect(outerHtml).not.toContain(rawIdentity);
    expect(outerHtml).not.toContain(rawPhone);
  });

  it("drops raw tree identifiers at the host presentation boundary", () => {
    const rawAccount = "6214600780000579708";
    const state = sanitizeEmbeddedFlowShellState({
      caseName: `账号：${rawAccount}`,
      search: rawAccount,
      graphSearch: { query: rawAccount },
      treeData: [
        {
          id: rawAccount,
          title: rawAccount,
          meta: "未登记户名",
          items: [{ id: rawAccount, title: rawAccount, sub: "" }],
        },
      ],
      expandedIds: [rawAccount],
      selectedIds: [rawAccount],
      views: [{ id: "view-1", title: `图${rawAccount}` }],
      overlay: {
        nodeInfo: { accountValue: rawAccount, accounts: [rawAccount] },
        edgeLabel: { value: rawAccount },
        txnModal: {
          rows: [{ key: rawAccount, cells: { account_no: { text: rawAccount }, amount: { text: "2645472.25" } } }],
        },
      },
    });

    expect(state.treeData).toEqual([]);
    expect(state.expandedIds).toEqual([]);
    expect(state.selectedIds).toEqual([]);
    expect(state.overlay.txnModal.rows[0]?.key).toBe("flow-txn-row-0");
    expect(state.overlay.txnModal.rows[0]?.cells.amount?.text).toBe("2645472.25");
    expect(JSON.stringify(state)).not.toContain(rawAccount);
  });

  it("projects node and edge labels before the canvas label pipeline consumes them", () => {
    const rawAccount = "6214600780000579708";
    const targetWindow = createEmbeddedProjectionWindow();
    targetWindow.__ANALYTIX_GRAPH_ENGINE_PRIMITIVES__ = {
      clamp: (value: number, min: number, max: number) => Math.max(min, Math.min(max, value)),
      asBool: (value: unknown) => Boolean(value),
      truncateTextByWorldWidth: (value: unknown) => String(value ?? ""),
      estimateTextBox: (value: unknown) => ({ w: String(value ?? "").length, h: 12 }),
    };
    runEmbeddedScript(graphLabelPipelineScript, targetWindow);
    const pipeline = targetWindow.__ANALYTIX_GRAPH_LABEL_PIPELINE__;

    const nodeRows = pipeline?.resolveNodeLabelRows({ id: rawAccount, title: rawAccount, x: 0, y: 0 });
    const userEdge = pipeline?.resolveEdgeLabelRows({ userCreated: true, label: rawAccount });
    const amountEdge = pipeline?.resolveEdgeLabelRows({ label: rawAccount, labelTop: `账号：${rawAccount}` });

    expect(nodeRows?.rows.map((row) => row.text).join(" ")).toContain("6214***********9708");
    expect(nodeRows?.rows.map((row) => row.text).join(" ")).not.toContain(rawAccount);
    expect(userEdge?.label).toBe("6214***********9708");
    expect(amountEdge?.label).toBe("6214***********9708");
    expect(amountEdge?.labelTop).toBe("账号：6214***********9708");
  });

  it("projects hover titles and account lists before constructing hover DOM", () => {
    const rawAccount = "6214600780000579708";
    const rawPhone = "13800138000";
    const targetWindow = createEmbeddedProjectionWindow();
    runEmbeddedScript(overlayAdapterScript, targetWindow);

    const hover = targetWindow.__ANALYTIX_FLOW_OVERLAY_ADAPTER__?.buildNodeHoverPresentation(
      { title: `账户合集 ${rawPhone}` },
      [rawAccount, "0000123456789012"]
    );
    const serialized = JSON.stringify(hover);

    expect(hover?.title).toBe("账户合集 138****8000");
    expect(hover?.accounts).toEqual(["6214***********9708", "0000********9012"]);
    expect(serialized).not.toContain(rawAccount);
    expect(serialized).not.toContain(rawPhone);
  });

  it("projects drill diagnostics and legacy view titles without changing numeric metrics", () => {
    const rawAccount = "6214600780000579708";
    const targetWindow = createEmbeddedProjectionWindow();
    runEmbeddedScript(canvasDrillControllerScript, targetWindow);
    runEmbeddedScript(flowViewSyncStoreScript, targetWindow);

    const logPayload = targetWindow.__ANALYTIX_FLOW_CANVAS_DRILL_CONTROLLER__?.projectLogPayload({
      nodeId: rawAccount,
      accountKey: rawAccount,
      amount: 2645472.25,
      nested: { counterparty_acct: rawAccount, source: rawAccount },
    }) as Record<string, unknown>;
    const titleElement = { textContent: "" };
    const countElement = { textContent: "" };
    const store = targetWindow.__ANALYTIX_FLOW_VIEW_SYNC_STORE__?.createFlowViewSyncStore({
      deps: {
        getActiveView: () => ({ title: rawAccount }),
        viewCounts: () => ({ nodes: 1, edges: 2 }),
        query: (selector: string) => (selector === "#viewTitle" ? titleElement : countElement),
      },
    });
    store?.updateViewCard();

    expect(logPayload.nodeId).toBe("6214***********9708");
    expect(logPayload.accountKey).toBe("6214***********9708");
    expect(logPayload.amount).toBe(2645472.25);
    expect(JSON.stringify(logPayload)).not.toContain(rawAccount);
    expect(titleElement.textContent).toBe("6214***********9708");
  });

  it("removes raw accounts from candidate names and blocks flow publication before every side effect", async () => {
    const rawAccount = "6214600780000579708";
    const payload = {
      dataUrl: "data:image/png;base64,AA==",
      format: "png",
      caseName: "../案件:A",
      subject: rawAccount,
      title: `账号：${rawAccount}`,
    };
    const builtName = buildFlowExportDownloadName(payload, "png");
    let sideEffects = 0;
    const guardedPayload = Object.defineProperty({ ...payload }, "dataUrl", {
      get: () => {
        sideEffects += 1;
        return payload.dataUrl;
      },
    });
    await expect(exportFlowGraphFromPayload({} as Window, guardedPayload)).rejects.toThrow(
      CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED
    );

    expect(builtName).toMatch(/6214_+9708/);
    expect(builtName).not.toContain(rawAccount);
    expect(builtName).not.toMatch(/[\\/:]/);
    expect(buildFlowExportDownloadName(payload, "../../exe")).toMatch(/\.png$/);
    expect(sideEffects).toBe(0);
  });
});
