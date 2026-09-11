import { CSSProperties, memo, useEffect, useMemo, useRef } from "react";

type ProcessStatus = "idle" | "running" | "done" | "failed";
type ProcessNodeId =
  | "case"
  | "funds"
  | "people"
  | "materials"
  | "table-account"
  | "table-transaction"
  | "table-sub-account"
  | "table-coercive"
  | "table-person"
  | "table-address"
  | "table-contact"
  | "raw"
  | "clean";

interface ProcessMetric {
  label: string;
  value: string;
}

export interface CleaningProcessNode {
  id: ProcessNodeId;
  kind: "compact" | "table";
  eyebrow: string;
  title: string;
  caption?: string;
  status: string;
  metrics: ProcessMetric[];
  accent: string;
}

interface CleaningProcessFlowProps {
  active?: boolean;
  status: ProcessStatus;
  nodes: CleaningProcessNode[];
}

type Point = {
  x: number;
  y: number;
};

type NodeRect = {
  width: number;
  height: number;
};

type PositionMap = Record<ProcessNodeId, Point>;
type ProcessLinkTone = "blue" | "cyan" | "coral";

type ProcessLink = {
  id: string;
  d: string;
  tone: ProcessLinkTone;
  arrow?: boolean;
  width?: number;
  bend?: number;
  flowDuration?: number;
  flowDelay?: number;
  flowOffset?: number;
};

const DEFAULT_RECT: NodeRect = { width: 100, height: 38 };
const ORTHOGONAL_BEND = 20;
const RIGHT_SIDE_ROUNDED_BEND = ORTHOGONAL_BEND;
const RIGHT_SIDE_ROUNDED_LEG = RIGHT_SIDE_ROUNDED_BEND * 2;
const NODE_RECTS: Record<ProcessNodeId, NodeRect> = {
  case: { width: 134, height: 72 },
  funds: { width: 132, height: 68 },
  people: { width: 132, height: 68 },
  materials: { width: 132, height: 68 },
  "table-account": { width: 124, height: 60 },
  "table-transaction": { width: 124, height: 60 },
  "table-sub-account": { width: 124, height: 60 },
  "table-coercive": { width: 124, height: 60 },
  "table-person": { width: 126, height: 62 },
  "table-address": { width: 126, height: 62 },
  "table-contact": { width: 126, height: 62 },
  raw: { width: 138, height: 74 },
  clean: { width: 138, height: 74 }
};

const DEFAULT_POSITIONS: PositionMap = {
  case: { x: 56, y: 197 },
  funds: { x: 226, y: 79 },
  people: { x: 226, y: 199 },
  materials: { x: 226, y: 319 },
  "table-account": { x: 400, y: 44 },
  "table-transaction": { x: 544, y: 44 },
  "table-sub-account": { x: 400, y: 118 },
  "table-coercive": { x: 544, y: 118 },
  "table-person": { x: 452, y: 202 },
  "table-address": { x: 452, y: 278 },
  "table-contact": { x: 452, y: 354 },
  raw: { x: 793, y: 87 },
  clean: { x: 793, y: 257 }
};

function getNodeRect(id: ProcessNodeId): NodeRect {
  return NODE_RECTS[id] || DEFAULT_RECT;
}

function formatCoord(value: number): string {
  return value.toFixed(1).replace(/\.0$/, "");
}

function moveTowards(from: Point, to: Point, distance: number): Point {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const length = Math.hypot(dx, dy) || 1;
  return {
    x: from.x + (dx / length) * distance,
    y: from.y + (dy / length) * distance
  };
}

function pointDistance(a: Point, b: Point): number {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

function interpolate(from: Point, to: Point, factor: number): Point {
  return {
    x: from.x + (to.x - from.x) * factor,
    y: from.y + (to.y - from.y) * factor
  };
}

function softPath(points: Point[], radius = 24): string {
  if (points.length < 2) {
    return "";
  }

  if (points.length === 2) {
    return `M ${formatCoord(points[0].x)} ${formatCoord(points[0].y)} L ${formatCoord(points[1].x)} ${formatCoord(points[1].y)}`;
  }

  let d = `M ${formatCoord(points[0].x)} ${formatCoord(points[0].y)}`;

  for (let index = 1; index < points.length - 1; index += 1) {
    const prev = points[index - 1];
    const current = points[index];
    const next = points[index + 1];
    const prevIsOrthogonal = prev.x === current.x || prev.y === current.y;
    const nextIsOrthogonal = current.x === next.x || current.y === next.y;
    const isStraight =
      (prev.x === current.x && current.x === next.x) || (prev.y === current.y && current.y === next.y);

    if (!prevIsOrthogonal || !nextIsOrthogonal || isStraight) {
      d += ` L ${formatCoord(current.x)} ${formatCoord(current.y)}`;
      continue;
    }

    const cornerRadius = Math.min(radius, pointDistance(prev, current) / 2, pointDistance(current, next) / 2);
    const cornerStart = moveTowards(current, prev, cornerRadius);
    const cornerEnd = moveTowards(current, next, cornerRadius);
    const controlStart = interpolate(cornerStart, current, 0.7);
    const controlEnd = interpolate(cornerEnd, current, 0.7);

    d += ` L ${formatCoord(cornerStart.x)} ${formatCoord(cornerStart.y)}`;
    d += ` C ${formatCoord(controlStart.x)} ${formatCoord(controlStart.y)} ${formatCoord(controlEnd.x)} ${formatCoord(controlEnd.y)} ${formatCoord(cornerEnd.x)} ${formatCoord(cornerEnd.y)}`;
  }

  const last = points[points.length - 1];
  d += ` L ${formatCoord(last.x)} ${formatCoord(last.y)}`;
  return d;
}

function cubicPath(start: Point, controlStart: Point, controlEnd: Point, end: Point): string {
  return [
    `M ${formatCoord(start.x)} ${formatCoord(start.y)}`,
    `C ${formatCoord(controlStart.x)} ${formatCoord(controlStart.y)} ${formatCoord(controlEnd.x)} ${formatCoord(controlEnd.y)} ${formatCoord(end.x)} ${formatCoord(end.y)}`
  ].join(" ");
}

function horizontalCurvePath(
  start: Point,
  end: Point,
  options: {
    startPull?: number;
    endPull?: number;
  } = {}
): string {
  const direction = end.x >= start.x ? 1 : -1;
  const distanceX = Math.abs(end.x - start.x);
  const startPull = Math.min(options.startPull ?? 34, Math.max(distanceX * 0.72, 0));
  const endPull = Math.min(options.endPull ?? 32, Math.max(distanceX * 0.72, 0));

  return cubicPath(
    start,
    { x: start.x + startPull * direction, y: start.y },
    { x: end.x - endPull * direction, y: end.y },
    end
  );
}

function leftAnchor(positions: PositionMap, id: ProcessNodeId, offsetY = 0): Point {
  const rect = getNodeRect(id);
  const position = positions[id];
  return { x: position.x, y: position.y + rect.height / 2 + offsetY };
}

function rightAnchor(positions: PositionMap, id: ProcessNodeId, offsetY = 0): Point {
  const rect = getNodeRect(id);
  const position = positions[id];
  return { x: position.x + rect.width, y: position.y + rect.height / 2 + offsetY };
}

function buildLink(
  id: string,
  tone: ProcessLinkTone,
  points: Point[],
  options: Omit<ProcessLink, "id" | "tone" | "d"> = {}
): ProcessLink {
  return {
    id,
    tone,
    d: softPath(points, options.bend ?? ORTHOGONAL_BEND),
    ...options
  };
}

function buildPathLink(
  id: string,
  tone: ProcessLinkTone,
  d: string,
  options: Omit<ProcessLink, "id" | "tone" | "d"> = {}
): ProcessLink {
  return {
    id,
    tone,
    d,
    ...options
  };
}

function buildProcessLinks(positions: PositionMap): ProcessLink[] {
  const caseTop = rightAnchor(positions, "case", -16);
  const caseMiddle = rightAnchor(positions, "case", 0);
  const caseBottom = rightAnchor(positions, "case", 16);
  const caseSplitX = positions.case.x + getNodeRect("case").width + 20;

  const fundsLeft = leftAnchor(positions, "funds");
  const peopleLeft = leftAnchor(positions, "people");
  const materialsLeft = leftAnchor(positions, "materials");

  const fundsRight = rightAnchor(positions, "funds");
  const fundsSplitX = positions.funds.x + getNodeRect("funds").width + 22;
  const accountLeft = leftAnchor(positions, "table-account");
  const subAccountLeft = leftAnchor(positions, "table-sub-account");

  const peopleRight = rightAnchor(positions, "people");
  const peopleSplitX = positions.people.x + getNodeRect("people").width + 42;
  const personLeft = leftAnchor(positions, "table-person");
  const addressLeft = leftAnchor(positions, "table-address");
  const contactLeft = leftAnchor(positions, "table-contact");

  const accountRight = rightAnchor(positions, "table-account");
  const transactionLeft = leftAnchor(positions, "table-transaction");
  const subAccountRight = rightAnchor(positions, "table-sub-account");
  const coerciveLeft = leftAnchor(positions, "table-coercive");
  const transactionRight = rightAnchor(positions, "table-transaction");
  const coerciveRight = rightAnchor(positions, "table-coercive");

  const personRight = rightAnchor(positions, "table-person");
  const addressRight = rightAnchor(positions, "table-address");
  const contactRight = rightAnchor(positions, "table-contact");

  const rawBlueLeft = leftAnchor(positions, "raw", -4);
  const rawCoralLeft = leftAnchor(positions, "raw", 18);
  const cleanBlueLeft = leftAnchor(positions, "clean", -14);
  const cleanCoralLeft = leftAnchor(positions, "clean", 15);

  const fundsMergeX = positions["table-transaction"].x + getNodeRect("table-transaction").width + 28;
  const fundsOutputX = positions.raw.x - 52;
  const fundsMergeY = coerciveRight.y - 28;

  const peopleMergeX = positions.clean.x - 88;
  const peopleChannelX = peopleMergeX - 76;
  const peopleMergeY = addressLeft.y;
  const peopleMerge = { x: peopleMergeX, y: peopleMergeY };
  const fundsMerge = { x: fundsOutputX, y: fundsMergeY };
  const fundsTrunkJoin = { x: fundsMerge.x - RIGHT_SIDE_ROUNDED_BEND, y: fundsMerge.y };
  const blueCleanSplit = { x: rawBlueLeft.x - 24, y: fundsMerge.y };
  const blueCleanBranchLead = { x: blueCleanSplit.x - RIGHT_SIDE_ROUNDED_BEND, y: blueCleanSplit.y };
  const coralRawSplit = { x: cleanCoralLeft.x - 48, y: peopleMerge.y };
  const coralRawBranchLead = { x: coralRawSplit.x - RIGHT_SIDE_ROUNDED_BEND, y: coralRawSplit.y };

  return [
    buildLink("case-funds", "blue", [caseTop, { x: caseSplitX, y: caseTop.y }, { x: caseSplitX, y: fundsLeft.y }, fundsLeft], {
      arrow: true,
      bend: ORTHOGONAL_BEND,
      flowDuration: 7.2,
      flowDelay: -0.8,
      flowOffset: -26
    }),
    buildLink("case-people", "cyan", [caseMiddle, { x: caseSplitX, y: caseMiddle.y }, { x: caseSplitX, y: peopleLeft.y }, peopleLeft], {
      arrow: true,
      bend: ORTHOGONAL_BEND,
      flowDuration: 6.4,
      flowDelay: -1.7,
      flowOffset: -14
    }),
    buildLink(
      "case-materials",
      "coral",
      [caseBottom, { x: caseSplitX, y: caseBottom.y }, { x: caseSplitX, y: materialsLeft.y }, materialsLeft],
      {
        arrow: true,
        bend: ORTHOGONAL_BEND,
        flowDuration: 7.8,
        flowDelay: -2.6,
        flowOffset: -41
      }
    ),
    buildLink("funds-account", "blue", [fundsRight, { x: fundsSplitX, y: fundsRight.y }, { x: fundsSplitX, y: accountLeft.y }, accountLeft], {
      arrow: true,
      bend: ORTHOGONAL_BEND,
      flowDuration: 5.6,
      flowDelay: -1.2,
      flowOffset: -32
    }),
    buildLink(
      "funds-sub-account",
      "blue",
      [fundsRight, { x: fundsSplitX, y: fundsRight.y }, { x: fundsSplitX, y: subAccountLeft.y }, subAccountLeft],
      {
        arrow: true,
        bend: ORTHOGONAL_BEND,
        flowDuration: 5.9,
        flowDelay: -2.1,
        flowOffset: -52
      }
    ),
    buildLink("people-person", "cyan", [peopleRight, { x: peopleSplitX, y: peopleRight.y }, { x: peopleSplitX, y: personLeft.y }, personLeft], {
      arrow: true,
      bend: ORTHOGONAL_BEND,
      flowDuration: 6.1,
      flowDelay: -0.6,
      flowOffset: -24
    }),
    buildLink(
      "people-address",
      "cyan",
      [peopleRight, { x: peopleSplitX, y: peopleRight.y }, { x: peopleSplitX, y: addressLeft.y }, addressLeft],
      {
        arrow: true,
        bend: ORTHOGONAL_BEND,
        flowDuration: 6.8,
        flowDelay: -1.9,
        flowOffset: -46
      }
    ),
    buildLink(
      "people-contact",
      "cyan",
      [peopleRight, { x: peopleSplitX, y: peopleRight.y }, { x: peopleSplitX, y: contactLeft.y }, contactLeft],
      {
        arrow: true,
        bend: ORTHOGONAL_BEND,
        flowDuration: 7.4,
        flowDelay: -3.1,
        flowOffset: -68
      }
    ),
    buildLink("account-transaction", "blue", [accountRight, { x: transactionLeft.x, y: accountRight.y }, transactionLeft], {
      arrow: true,
      flowDuration: 4.9,
      flowDelay: -0.5,
      flowOffset: -18
    }),
    buildLink("sub-coercive", "blue", [subAccountRight, { x: coerciveLeft.x, y: subAccountRight.y }, coerciveLeft], {
      arrow: true,
      flowDuration: 4.7,
      flowDelay: -1.4,
      flowOffset: -26
    }),
    buildLink(
      "transaction-merge",
      "blue",
      [transactionRight, { x: fundsMergeX, y: transactionRight.y }, { x: fundsMergeX, y: fundsMergeY }, fundsTrunkJoin],
      {
        bend: ORTHOGONAL_BEND,
        flowDuration: 5.1,
        flowDelay: -1.3,
        flowOffset: -38,
        width: 1.7
      }
    ),
    buildLink(
      "coercive-merge",
      "blue",
      [coerciveRight, { x: fundsMergeX, y: coerciveRight.y }, { x: fundsMergeX, y: fundsMergeY }, fundsTrunkJoin],
      {
        bend: ORTHOGONAL_BEND,
        flowDuration: 5.7,
        flowDelay: -2.2,
        flowOffset: -57,
        width: 1.7
      }
    ),
    buildLink(
      "person-merge",
      "cyan",
      [personRight, { x: peopleChannelX, y: personRight.y }, { x: peopleChannelX, y: peopleMergeY }, peopleMerge],
      {
        bend: ORTHOGONAL_BEND,
        flowDuration: 5.3,
        flowDelay: -0.9,
        flowOffset: -31,
        width: 1.7
      }
    ),
    buildLink("address-merge", "cyan", [addressRight, peopleMerge], {
      flowDuration: 4.8,
      flowDelay: -1.8,
      flowOffset: -19,
      width: 1.7
    }),
    buildLink(
      "contact-merge",
      "cyan",
      [contactRight, { x: peopleChannelX, y: contactRight.y }, { x: peopleChannelX, y: peopleMergeY }, peopleMerge],
      {
        bend: ORTHOGONAL_BEND,
        flowDuration: 5.9,
        flowDelay: -2.5,
        flowOffset: -43,
        width: 1.7
      }
    ),
    buildLink(
      "merge-trunk-blue",
      "blue",
      [fundsTrunkJoin, fundsMerge, blueCleanSplit],
      {
        bend: RIGHT_SIDE_ROUNDED_BEND,
        flowDuration: 5.4,
        flowDelay: -2.8,
        flowOffset: -71,
        width: 2
      }
    ),
    buildLink(
      "merge-raw-blue",
      "blue",
      [blueCleanSplit, rawBlueLeft],
      {
        arrow: true,
        bend: RIGHT_SIDE_ROUNDED_BEND,
        flowDuration: 5.8,
        flowDelay: -1.9,
        flowOffset: -52,
        width: 2
      }
    ),
    buildLink(
      "merge-clean-blue",
      "blue",
      [blueCleanBranchLead, blueCleanSplit, { x: blueCleanSplit.x, y: cleanBlueLeft.y }, cleanBlueLeft],
      {
        arrow: true,
        bend: RIGHT_SIDE_ROUNDED_BEND,
        flowDuration: 6.1,
        flowDelay: -3.1,
        flowOffset: -74,
        width: 2
      }
    ),
    buildLink(
      "merge-trunk-coral",
      "cyan",
      [peopleMerge, coralRawSplit],
      {
        bend: RIGHT_SIDE_ROUNDED_BEND,
        flowDuration: 6,
        flowDelay: -1.1,
        flowOffset: -48,
        width: 2.1
      }
    ),
    buildLink(
      "merge-raw-coral",
      "cyan",
      [coralRawBranchLead, coralRawSplit, { x: coralRawSplit.x, y: rawCoralLeft.y }, rawCoralLeft],
      {
        arrow: true,
        bend: RIGHT_SIDE_ROUNDED_BEND,
        flowDuration: 6.3,
        flowDelay: -1.7,
        flowOffset: -52,
        width: 2.1
      }
    ),
    buildLink(
      "merge-clean-coral",
      "cyan",
      [coralRawSplit, cleanCoralLeft],
      {
        arrow: true,
        bend: RIGHT_SIDE_ROUNDED_BEND,
        flowDuration: 6.6,
        flowDelay: -3.1,
        flowOffset: -74,
        width: 2.1
      }
    )
  ];
}

export const CleaningProcessFlow = memo(function CleaningProcessFlow({
  active = true,
  status,
  nodes
}: CleaningProcessFlowProps): JSX.Element {
  const sceneRef = useRef<HTMLDivElement | null>(null);
  const particlesCanvasRef = useRef<HTMLCanvasElement | null>(null);
  const linkPathRefs = useRef<Map<string, SVGPathElement>>(new Map());
  const positions = DEFAULT_POSITIONS;
  const links = useMemo(() => buildProcessLinks(positions), [positions]);
  const showFlowAnimation = active;

  useEffect(() => {
    const canvas = particlesCanvasRef.current;
    const scene = sceneRef.current;
    if (!canvas || !scene) {
      return;
    }
    const gl = canvas.getContext("webgl", {
      alpha: true,
      antialias: true,
      depth: false,
      stencil: false,
      premultipliedAlpha: true,
      preserveDrawingBuffer: false
    });
    if (!gl) {
      return;
    }
    const staticMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches || navigator.webdriver;

    let rafId = 0;
    let canvasWidth = 0;
    let canvasHeight = 0;
    let dpr = 1;

    const linkMeta = links.map((link, index) => {
      const hue =
        link.tone === "cyan"
          ? { solid: [130, 243, 255, 0.72], fade: [225, 255, 255, 0.94] }
          : link.tone === "coral"
            ? { solid: [241, 174, 96, 0.68], fade: [255, 233, 205, 0.92] }
            : { solid: [127, 207, 255, 0.7], fade: [208, 248, 255, 0.94] };
      return {
        ...link,
        index,
        colors: hue
      };
    });

    const vertexSource = `
      attribute vec2 a_position;
      attribute float a_size;
      attribute vec4 a_color;
      uniform vec2 u_resolution;
      varying vec4 v_color;

      void main() {
        vec2 zeroToOne = a_position / u_resolution;
        vec2 zeroToTwo = zeroToOne * 2.0;
        vec2 clipSpace = zeroToTwo - 1.0;
        gl_Position = vec4(clipSpace * vec2(1.0, -1.0), 0.0, 1.0);
        gl_PointSize = a_size;
        v_color = a_color;
      }
    `;
    const fragmentSource = `
      precision mediump float;
      varying vec4 v_color;

      void main() {
        vec2 coord = gl_PointCoord - vec2(0.5);
        float distanceToCenter = length(coord);
        float alpha = smoothstep(0.5, 0.0, distanceToCenter);
        float core = smoothstep(0.18, 0.0, distanceToCenter);
        vec3 color = mix(v_color.rgb * 0.72, vec3(1.0), core * 0.58);
        gl_FragColor = vec4(color, v_color.a * alpha);
      }
    `;

    const compileShader = (type: number, source: string): WebGLShader | null => {
      const shader = gl.createShader(type);
      if (!shader) {
        return null;
      }
      gl.shaderSource(shader, source);
      gl.compileShader(shader);
      if (gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
        return shader;
      }
      gl.deleteShader(shader);
      return null;
    };

    const vertexShader = compileShader(gl.VERTEX_SHADER, vertexSource);
    const fragmentShader = compileShader(gl.FRAGMENT_SHADER, fragmentSource);
    if (!vertexShader || !fragmentShader) {
      if (vertexShader) {
        gl.deleteShader(vertexShader);
      }
      if (fragmentShader) {
        gl.deleteShader(fragmentShader);
      }
      return;
    }

    const program = gl.createProgram();
    if (!program) {
      gl.deleteShader(vertexShader);
      gl.deleteShader(fragmentShader);
      return;
    }

    gl.attachShader(program, vertexShader);
    gl.attachShader(program, fragmentShader);
    gl.linkProgram(program);
    gl.deleteShader(vertexShader);
    gl.deleteShader(fragmentShader);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      gl.deleteProgram(program);
      return;
    }

    const positionLocation = gl.getAttribLocation(program, "a_position");
    const sizeLocation = gl.getAttribLocation(program, "a_size");
    const colorLocation = gl.getAttribLocation(program, "a_color");
    const resolutionLocation = gl.getUniformLocation(program, "u_resolution");
    const particleBuffer = gl.createBuffer();
    if (positionLocation < 0 || sizeLocation < 0 || colorLocation < 0 || !resolutionLocation || !particleBuffer) {
      if (particleBuffer) {
        gl.deleteBuffer(particleBuffer);
      }
      gl.deleteProgram(program);
      return;
    }

    gl.useProgram(program);
    gl.bindBuffer(gl.ARRAY_BUFFER, particleBuffer);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
    gl.disable(gl.DEPTH_TEST);
    gl.disable(gl.CULL_FACE);

    const floatsPerParticle = 7;
    const particleData = new Float32Array(linkMeta.length * 6 * floatsPerParticle);

    const syncCanvasSize = (): void => {
      const rect = scene.getBoundingClientRect();
      dpr = window.devicePixelRatio || 1;
      canvasWidth = rect.width;
      canvasHeight = rect.height;
      canvas.width = Math.max(1, Math.round(rect.width * dpr));
      canvas.height = Math.max(1, Math.round(rect.height * dpr));
      canvas.style.width = `${rect.width}px`;
      canvas.style.height = `${rect.height}px`;
      gl.viewport(0, 0, canvas.width, canvas.height);
    };

    syncCanvasSize();

    if (!showFlowAnimation) {
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      return;
    }

    const drawParticles = (now: number): void => {
      let cursor = 0;

      linkMeta.forEach((link) => {
        const path = linkPathRefs.current.get(link.id);
        if (!path) {
          return;
        }

        const length = path.getTotalLength();
        if (!Number.isFinite(length) || length <= 0) {
          return;
        }

        const durationSeconds = Math.max(3.6, link.flowDuration ?? 6.4);
        const speed = length / (durationSeconds * 1000);
        const baseOffset = Math.abs(link.flowDelay ?? 0) * length * 0.16 + Math.abs(link.flowOffset ?? 0);
        const particleCount = link.width && link.width >= 2 ? 3 : 2;

        for (let particleIndex = 0; particleIndex < particleCount; particleIndex += 1) {
          const trailOffset = (length / particleCount) * particleIndex;
          const animationClock = staticMotion ? durationSeconds * 240 : now;
          const distance = (animationClock * speed + baseOffset + trailOffset + link.index * 11) % length;
          const point = path.getPointAtLength(distance);
          const baseSize = particleIndex === 0 ? 20 : 15;
          const alpha = particleIndex === 0 ? link.colors.fade[3] : link.colors.solid[3];
          const rgb = particleIndex === 0 ? link.colors.fade : link.colors.solid;
          particleData[cursor] = point.x * dpr;
          particleData[cursor + 1] = point.y * dpr;
          particleData[cursor + 2] = baseSize * dpr;
          particleData[cursor + 3] = rgb[0] / 255;
          particleData[cursor + 4] = rgb[1] / 255;
          particleData[cursor + 5] = rgb[2] / 255;
          particleData[cursor + 6] = alpha;
          cursor += floatsPerParticle;
        }
      });

      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);

      if (cursor === 0) {
        return;
      }

      const payload = particleData.subarray(0, cursor);
      gl.useProgram(program);
      gl.uniform2f(resolutionLocation, canvas.width, canvas.height);
      gl.bindBuffer(gl.ARRAY_BUFFER, particleBuffer);
      gl.bufferData(gl.ARRAY_BUFFER, payload, gl.DYNAMIC_DRAW);

      const stride = floatsPerParticle * 4;
      gl.vertexAttribPointer(positionLocation, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(positionLocation);
      gl.vertexAttribPointer(sizeLocation, 1, gl.FLOAT, false, stride, 2 * 4);
      gl.enableVertexAttribArray(sizeLocation);
      gl.vertexAttribPointer(colorLocation, 4, gl.FLOAT, false, stride, 3 * 4);
      gl.enableVertexAttribArray(colorLocation);

      gl.drawArrays(gl.POINTS, 0, cursor / floatsPerParticle);
    };

    const resizeObserver = new ResizeObserver(() => {
      syncCanvasSize();
      drawParticles(performance.now());
    });
    resizeObserver.observe(scene);

    if (staticMotion) {
      drawParticles(performance.now());
      return () => {
        resizeObserver.disconnect();
        gl.clear(gl.COLOR_BUFFER_BIT);
        gl.deleteBuffer(particleBuffer);
        gl.deleteProgram(program);
      };
    }

    const renderFrame = (now: number): void => {
      drawParticles(now);
      rafId = window.requestAnimationFrame(renderFrame);
    };

    rafId = window.requestAnimationFrame(renderFrame);

    return () => {
      window.cancelAnimationFrame(rafId);
      resizeObserver.disconnect();
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.deleteBuffer(particleBuffer);
      gl.deleteProgram(program);
    };
  }, [links, showFlowAnimation]);

  return (
    <section className={`cleaning-process-flow cleaning-process-flow--${status}`}>
      <div className="cleaning-process-flow__masthead">
        <div>
          <div className="cleaning-section__eyebrow">流程总览</div>
          <h3>数据清洗处理流程</h3>
        </div>
      </div>

      <div className="cleaning-process-flow__board">
        <div className="cleaning-process-flow__canvas">
          <div className="cleaning-process-flow__grid" aria-hidden="true" />
          <div ref={sceneRef} className="cleaning-process-flow__scene">
            <canvas ref={particlesCanvasRef} className="cleaning-process-flow__particles" aria-hidden="true" />
            <svg className="cleaning-process-flow__links" aria-hidden="true">
              <defs>
                <marker id="cleaning-process-link-arrow-blue" viewBox="0 0 6 6" refX="5.4" refY="3" markerWidth="6" markerHeight="6" orient="auto">
                  <path d="M0,0 L6,3 L0,6 Z" fill="#8bd5ff" />
                </marker>
                <marker id="cleaning-process-link-arrow-cyan" viewBox="0 0 6 6" refX="5.4" refY="3" markerWidth="6" markerHeight="6" orient="auto">
                  <path d="M0,0 L6,3 L0,6 Z" fill="#93f5ff" />
                </marker>
                <marker id="cleaning-process-link-arrow-coral" viewBox="0 0 6 6" refX="5.4" refY="3" markerWidth="6" markerHeight="6" orient="auto">
                  <path d="M0,0 L6,3 L0,6 Z" fill="#f2b276" />
                </marker>
              </defs>
              {links.map((link) => {
                const linkStyle = {
                  "--link-width": `${link.width ?? 1.9}px`,
                  "--flow-duration": `${link.flowDuration ?? 6.4}s`,
                  "--flow-delay": `${link.flowDelay ?? 0}s`,
                  "--flow-offset": `${link.flowOffset ?? 0}px`
                } as CSSProperties;

                return (
                  <g key={link.id} className={`cleaning-process-flow__link is-${link.tone}`} style={linkStyle}>
                    <path className="cleaning-process-flow__link-glow" d={link.d} />
                    <path
                      className="cleaning-process-flow__link-core"
                      d={link.d}
                      ref={(node) => {
                        if (node) {
                          linkPathRefs.current.set(link.id, node);
                          return;
                        }
                        linkPathRefs.current.delete(link.id);
                      }}
                      markerEnd={link.arrow ? `url(#cleaning-process-link-arrow-${link.tone})` : undefined}
                    />
                  </g>
                );
              })}
            </svg>

            {nodes.map((node) => {
              const position = positions[node.id];
              const rect = getNodeRect(node.id);
              const isTableNode = node.kind === "table";
              const style = {
                left: `${position.x}px`,
                top: `${position.y}px`,
                width: `${rect.width}px`,
                minHeight: `${rect.height}px`,
                "--cleaning-process-accent": node.accent
              } as CSSProperties;

              return (
                <article
                  key={node.id}
                  className={`cleaning-process-node cleaning-process-node--${node.kind} cleaning-process-node--${node.id}`}
                  style={style}
                >
                  {node.eyebrow ? <div className="cleaning-process-node__eyebrow">{node.eyebrow}</div> : null}
                  <div className="cleaning-process-node__title-row">
                    <h4>{node.title}</h4>
                    <span className="cleaning-process-node__status">{node.status}</span>
                  </div>
                  {node.caption ? <div className="cleaning-process-node__caption">{node.caption}</div> : null}
                  {isTableNode ? (
                    <div className="cleaning-process-node__table-meta">
                      {node.metrics.map((metric) => (
                        <span key={`${node.id}:${metric.label}`} className="cleaning-process-node__table-meta-item">
                          <em>{metric.label}</em>
                          <strong>{metric.value}</strong>
                        </span>
                      ))}
                    </div>
                  ) : (
                    <div className="cleaning-process-node__metrics">
                      {node.metrics.map((metric) => (
                        <div key={`${node.id}:${metric.label}`} className="cleaning-process-node__metric">
                          <span>{metric.label}</span>
                          <strong>{metric.value}</strong>
                        </div>
                      ))}
                    </div>
                  )}
                </article>
              );
            })}

          </div>
        </div>
      </div>
    </section>
  );
});
