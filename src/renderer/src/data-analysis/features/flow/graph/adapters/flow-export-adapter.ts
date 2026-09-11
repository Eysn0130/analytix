import { requireControlledArtifactPublication } from "../../../../services/publication-quarantine";
import {
  projectDetectedOrdinaryRestrictedPii,
  projectOrdinaryRestrictedPii,
} from "../../../shared/ordinary-pii-projection";

export interface FlowExportResult {
  ok: boolean;
  path?: string;
  cancelled?: boolean;
  error?: string;
}

export interface FlowExportEnvironment {
  readonly publicationAuthority?: never;
}

export function buildFlowExportDownloadName(payload: Record<string, unknown>, extension: string): string {
  const projectFilenamePart = (value: unknown): string => {
    const projected = projectDetectedOrdinaryRestrictedPii(value)
      .replace(
        /(^|[^0-9])(\d(?:[\s\-‐‑‒–—―_/\\.·•]*\d){7,})(?=$|[^0-9])/gu,
        (_match, prefix: string, pii: string) => `${prefix}${projectOrdinaryRestrictedPii(pii, "bank-account")}`
      )
      .replace(/[<>:"/\\|?*]/g, "_");
    return Array.from(projected, (char) => (char.charCodeAt(0) <= 0x1f ? "_" : char))
      .join("")
      .replace(/^\.+|\.+$/g, "")
      .trim()
      .slice(0, 80);
  };
  const parts = [payload.caseName, payload.subject, payload.title]
    .map(projectFilenamePart)
    .filter(Boolean)
    .concat("图谱")
    .slice(0, 4);
  const stem = parts.join("-") || `analytix-flow-${Date.now()}`;
  const safeExtension = /^[a-z0-9]{1,8}$/i.test(extension) ? extension.toLowerCase() : "png";
  return `${stem}.${safeExtension}`;
}

export async function exportFlowGraphFromPayload(
  _targetWindow: Window,
  _payload: Record<string, unknown>,
  _environment: FlowExportEnvironment = {}
): Promise<FlowExportResult> {
  requireControlledArtifactPublication();
  return { ok: false, error: "controlled_artifact_publication_required" };
}
