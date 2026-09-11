import { RADIUS_TOKENS, SPACE_TOKENS, THEME_TOKENS, ThemeName } from "./tokens";

export type ThemeMode = "light" | "dark";

const STORAGE_KEY = "analytix:theme-mode";

export function getStoredThemeMode(): ThemeMode {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (raw === "light" || raw === "dark") {
    return raw;
  }
  return "light";
}

export function storeThemeMode(mode: ThemeMode): void {
  localStorage.setItem(STORAGE_KEY, mode);
}

export function resolveTheme(mode: ThemeMode, prefersDark: boolean): ThemeName {
  void prefersDark;
  return mode === "dark" ? "dark" : "light";
}

export function applyTheme(theme: ThemeName, target: HTMLElement): void {
  target.dataset.theme = theme;
  const tokenGroups = [THEME_TOKENS[theme], RADIUS_TOKENS, SPACE_TOKENS];
  for (const group of tokenGroups) {
    for (const [key, value] of Object.entries(group)) {
      target.style.setProperty(`--${key}`, value);
    }
  }
}
