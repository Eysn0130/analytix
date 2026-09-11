import type { FlowShellBridge, FlowShellMounts, FlowShellState } from "../runtime/flow-runtime";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";
import { EdgeTxnReadonlyPanel, FloatingPanel } from "./FlowOverlayPrimitives";

interface FlowInfoPopoversProps {
  bridge: FlowShellBridge;
  mounts: FlowShellMounts;
  state: FlowShellState;
}

function buildNodePreviewMetrics(preview: NonNullable<FlowShellState["overlay"]["nodeInfo"]["preview"]>): {
  frameSize: number;
  center: number;
  radius: number;
  haloRadius: number;
  lineWidth: number;
} {
  const size = Math.max(26, Math.min(52, Number(preview.size) || 32));
  const lineWidth = Math.max(1, Math.min(8, Number(preview.lineWidth) || 2));
  const pad = 6;
  const frameSize = size + pad * 2;
  const center = frameSize / 2;
  const radius = Math.max(2, size / 2 - lineWidth / 2);
  const haloRadius = radius + lineWidth * 0.5 + 1.25;
  return {
    frameSize,
    center,
    radius,
    haloRadius,
    lineWidth
  };
}

function NodePreviewGraphic({
  preview
}: {
  preview: NonNullable<FlowShellState["overlay"]["nodeInfo"]["preview"]>;
}): JSX.Element {
  const { frameSize, center, radius, haloRadius, lineWidth } = buildNodePreviewMetrics(preview);
  return (
    <div
      className="nodePreview"
      style={{
        width: `${frameSize}px`,
        height: `${frameSize}px`
      }}
    >
      <svg
        className="nodePreviewGraphic"
        viewBox={`0 0 ${frameSize} ${frameSize}`}
        width={frameSize}
        height={frameSize}
        aria-hidden="true"
      >
        <circle cx={center} cy={center} r={haloRadius} fill="rgba(15,23,42,0.08)" />
        <circle cx={center} cy={center} r={radius} fill={preview.fill} />
        <circle cx={center} cy={center} r={radius} fill="none" stroke={preview.stroke} strokeWidth={lineWidth} />
      </svg>
    </div>
  );
}

export function FlowNodeInfoPresentation({
  nodeInfo
}: {
  nodeInfo: FlowShellState["overlay"]["nodeInfo"];
}): JSX.Element {
  const category = projectOrdinaryFieldValue("", nodeInfo.category);
  const userName = projectOrdinaryFieldValue("", nodeInfo.userName);
  const accountValue = projectOrdinaryFieldValue("account_no", nodeInfo.accountValue);
  const accounts = nodeInfo.accounts.map((account) => projectOrdinaryFieldValue("account_no", account));
  return (
    <div className="nodeInfoGrid">
      <div className="nodeInfoLeft">
        {nodeInfo.preview ? <NodePreviewGraphic preview={nodeInfo.preview} /> : null}
        <div className="nodeCategoryLabel">类别名称</div>
        <div className="nodeCategoryValue">{category}</div>
      </div>
      <div className="nodeInfoRight">
        <div className="nodeInfoRow">
          <div className="nodeInfoLabel">用户名</div>
          <div className="nodeInfoValue">{userName}</div>
        </div>
        <div className="nodeInfoRow">
          <div className="nodeInfoLabel">{projectOrdinaryFieldValue("", nodeInfo.accountLabel)}</div>
          {accounts.length ? (
            <div className="nodeInfoList">
              {accounts.map((account, index) => (
                <span key={`${account}:${index}`} className="nodeInfoTag">
                  {account}
                </span>
              ))}
              {nodeInfo.moreCount > 0 ? <span className="nodeInfoMore">{`+${nodeInfo.moreCount}`}</span> : null}
            </div>
          ) : (
            <div
              className={`nodeInfoValue${nodeInfo.accountValueMono ? " mono" : ""}`}
              style={nodeInfo.accountValueMono ? { fontFamily: "var(--mono)" } : undefined}
            >
              {accountValue}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function NodeInfoPopoverPortal({
  mounts,
  state
}: FlowInfoPopoversProps): JSX.Element | null {
  const nodeInfo = state.overlay.nodeInfo;
  if (!nodeInfo.open || !nodeInfo.point || !nodeInfo.preview) {
    return null;
  }

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={nodeInfo.open}
      point={nodeInfo.point}
      className="analytix-react-overlay-panel analytix-react-node-info nodeInfoPopover"
      panelStyle={{ display: "block" }}
      ariaHidden={false}
    >
      <FlowNodeInfoPresentation nodeInfo={nodeInfo} />
    </FloatingPanel>
  );
}

function EdgeInfoPopoverPortal({
  bridge,
  mounts,
  state
}: FlowInfoPopoversProps): JSX.Element | null {
  const edgeInfo = state.overlay.edgeInfo;
  if (!edgeInfo.open || !edgeInfo.point) {
    return null;
  }

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={edgeInfo.open}
      point={edgeInfo.point}
      className="analytix-react-overlay-panel analytix-react-edge-info edgeInfoPopover"
      panelStyle={{ display: "block" }}
      ariaHidden={false}
    >
      <EdgeTxnReadonlyPanel data={edgeInfo} onOpenDetail={() => bridge.commands.overlay.openEdgeTxnDetail()} />
    </FloatingPanel>
  );
}

export function FlowInfoPopovers({
  bridge,
  mounts,
  state
}: FlowInfoPopoversProps): JSX.Element {
  return (
    <>
      <NodeInfoPopoverPortal bridge={bridge} mounts={mounts} state={state} />
      <EdgeInfoPopoverPortal bridge={bridge} mounts={mounts} state={state} />
    </>
  );
}
