// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { applyTheme } from "../../../../theme/theme";
import { THEME_TOKENS } from "../../../../theme/tokens";
import { bindFlowThemeSync } from "./embedded-flow-theme";

const cleanups: Array<() => void> = [];
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup());
  document.body.replaceChildren();
  document.documentElement.removeAttribute("style");
  delete document.documentElement.dataset.theme;
});

function createSurfaceFrame(): { surface: HTMLDivElement; target: HTMLElement } {
  const surface = document.createElement("div");
  surface.className = "data-analysis-surface";
  const frame = document.createElement("iframe");
  surface.append(frame);
  document.body.append(surface);
  cleanups.push(bindFlowThemeSync(frame.contentDocument!, surface));
  return { surface, target: frame.contentDocument!.documentElement };
}

async function flushThemeMutation(): Promise<void> {
  await new Promise<void>((resolve) => queueMicrotask(resolve));
}

describe("embedded flow display theme ownership", () => {
  it("uses the independent surface theme rather than the desktop root", async () => {
    applyTheme("dark", document.documentElement);
    const { surface, target } = createSurfaceFrame();
    applyTheme("light", surface);
    await flushThemeMutation();
    expect(target.dataset.theme).toBe("light");
    expect(target.style.colorScheme).toBe("light");
    for (const token of ["color-bg", "color-text", "color-accent", "color-success", "color-warning", "color-danger", "shadow-card", "shadow-elevated"]) {
      expect(target.style.getPropertyValue(`--${token}`)).toBe(THEME_TOKENS.light[token]);
    }
    document.documentElement.dataset.theme = "dark";
    await flushThemeMutation();
    expect(target.dataset.theme).toBe("light");
    expect(target.style.colorScheme).toBe("light");
  });

  it("updates local light and dark tokens and stops observation after cleanup", async () => {
    const { surface, target } = createSurfaceFrame();
    applyTheme("dark", surface);
    await flushThemeMutation();
    expect(target.dataset.theme).toBe("dark");
    expect(target.style.colorScheme).toBe("dark");
    expect(target.style.getPropertyValue("--color-bg")).toBe(THEME_TOKENS.dark["color-bg"]);
    cleanups.splice(0).forEach((cleanup) => cleanup());
    applyTheme("light", surface);
    await flushThemeMutation();
    expect(target.dataset.theme).toBe("dark");
    expect(target.style.colorScheme).toBe("dark");
  });

  it("keeps the root fallback and forwards only the existing display token allowlist", async () => {
    const frame = document.createElement("iframe");
    document.body.append(frame);
    const root = document.documentElement;
    applyTheme("dark", root);
    root.style.setProperty("--private-unrelated-value", "not-forwarded");
    root.style.colorScheme = "dark";
    const target = frame.contentDocument!.documentElement;
    cleanups.push(bindFlowThemeSync(frame.contentDocument!, root));
    expect(target.dataset.theme).toBe("dark");
    expect(target.style.colorScheme).toBe("dark");
    expect(target.style.colorScheme).toBe("dark");
    expect(target.style.getPropertyValue("--private-unrelated-value")).toBe("");
    root.style.removeProperty("--shadow-elevated");
    await flushThemeMutation();
    expect(target.style.getPropertyValue("--shadow-elevated")).toBe("");
  });
});
