// Forward display tokens from the owning DataAnalysis surface to its iframe.
const EMBEDDED_THEME_CUSTOM_PROPERTIES = [
  "--font-sans",
  "--font-mono",
  "--motion-fast",
  "--motion-medium",
  "--color-bg",
  "--color-bg-elevated",
  "--color-bg-subtle",
  "--color-text",
  "--color-text-muted",
  "--color-border",
  "--color-accent",
  "--color-accent-strong",
  "--color-success",
  "--color-warning",
  "--color-danger",
  "--shadow-card",
  "--shadow-elevated",
  "--gradient-atmosphere",
  "--program-shell-bg",
  "--program-shell-border",
  "--radius-sm",
  "--radius-md",
  "--radius-lg",
  "--radius-xl",
  "--space-1",
  "--space-2",
  "--space-3",
  "--space-4",
  "--space-5",
  "--space-6"
];

function syncEmbeddedThemeToIframe(targetDocument: Document, sourceRoot: HTMLElement): void {
  const sourceDocument = sourceRoot.ownerDocument;
  const targetRoot = targetDocument.documentElement;
  const sourceWindow = sourceDocument.defaultView || window;
  const sourceStyles = sourceWindow.getComputedStyle(sourceRoot);

  EMBEDDED_THEME_CUSTOM_PROPERTIES.forEach((propertyName) => {
    const value = sourceStyles.getPropertyValue(propertyName).trim();
    if (value) {
      targetRoot.style.setProperty(propertyName, value);
    } else {
      targetRoot.style.removeProperty(propertyName);
    }
  });

  const themeName = String(sourceRoot.dataset.theme || sourceDocument.documentElement.dataset.theme || "").trim();
  if (themeName) {
    targetRoot.dataset.theme = themeName;
  } else {
    delete targetRoot.dataset.theme;
  }

  const colorScheme = themeName === "light" || themeName === "dark"
    ? themeName
    : String(sourceStyles.getPropertyValue("color-scheme") || sourceRoot.style.colorScheme || "").trim();
  if (colorScheme) {
    targetRoot.style.colorScheme = colorScheme;
  } else {
    targetRoot.style.removeProperty("color-scheme");
  }
}

export function bindFlowThemeSync(targetDocument: Document, sourceRoot: HTMLElement): () => void {
  const sourceDocument = sourceRoot.ownerDocument;
  const sourceWindow = sourceDocument.defaultView || window;
  syncEmbeddedThemeToIframe(targetDocument, sourceRoot);

  const observer = new MutationObserver(() => {
    syncEmbeddedThemeToIframe(targetDocument, sourceRoot);
  });
  const observationOptions = { attributes: true, attributeFilter: ["data-theme", "style", "class"] };
  observer.observe(sourceRoot, observationOptions);
  if (sourceRoot !== sourceDocument.documentElement) {
    observer.observe(sourceDocument.documentElement, observationOptions);
  }

  const handleMediaChange = (): void => {
    syncEmbeddedThemeToIframe(targetDocument, sourceRoot);
  };

  const media =
    typeof sourceWindow.matchMedia === "function" ? sourceWindow.matchMedia("(prefers-color-scheme: dark)") : null;
  if (media) {
    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", handleMediaChange);
    } else if (typeof media.addListener === "function") {
      media.addListener(handleMediaChange);
    }
  }

  return () => {
    observer.disconnect();
    if (media) {
      if (typeof media.removeEventListener === "function") {
        media.removeEventListener("change", handleMediaChange);
      } else if (typeof media.removeListener === "function") {
        media.removeListener(handleMediaChange);
      }
    }
  };
}
