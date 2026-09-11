import { useEffect, useRef, useState } from "react";
import type { AnimationItem } from "lottie-web";
import animationData from "../assets/download-file-icon-animation.json";

type LottiePlayer = {
  loadAnimation: (options: {
    container: Element;
    renderer: "svg";
    loop: boolean;
    autoplay: boolean;
    animationData: unknown;
    rendererSettings: {
      preserveAspectRatio: string;
      progressiveLoad: boolean;
    };
  }) => AnimationItem;
};

type ImportDropzoneLottieIconProps = {
  disabled?: boolean;
  emphasis: "idle" | "drag";
};

const STATIC_FRAME = 54;

function usePrefersReducedMotion(): boolean {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
      return;
    }

    const mediaQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updatePreference = (): void => {
      setPrefersReducedMotion(mediaQuery.matches);
    };

    updatePreference();
    mediaQuery.addEventListener?.("change", updatePreference);

    return () => {
      mediaQuery.removeEventListener?.("change", updatePreference);
    };
  }, []);

  return prefersReducedMotion;
}

function ImportDropzoneStaticIcon(): JSX.Element {
  return (
    <svg
      className="import-console-dropzone__icon-lottie-fallback"
      viewBox="0 0 120 120"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <rect x="24" y="16" width="72" height="88" rx="20" fill="url(#import-dropzone-icon-paper)" />
      <path
        d="M42 44H78"
        stroke="currentColor"
        strokeWidth="6"
        strokeLinecap="round"
        strokeOpacity="0.18"
      />
      <path
        d="M42 58H70"
        stroke="currentColor"
        strokeWidth="6"
        strokeLinecap="round"
        strokeOpacity="0.12"
      />
      <path
        d="M60 32V72"
        stroke="url(#import-dropzone-icon-arrow)"
        strokeWidth="7"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M45 58L60 73L75 58"
        stroke="url(#import-dropzone-icon-arrow)"
        strokeWidth="7"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M32 84C32 80.6863 34.6863 78 38 78H82C85.3137 78 88 80.6863 88 84C88 87.3137 85.3137 90 82 90H38C34.6863 90 32 87.3137 32 84Z"
        fill="url(#import-dropzone-icon-base)"
      />
      <defs>
        <linearGradient id="import-dropzone-icon-paper" x1="60" y1="16" x2="60" y2="104" gradientUnits="userSpaceOnUse">
          <stop stopColor="#FFFFFF" stopOpacity="0.98" />
          <stop offset="1" stopColor="#E7EFFF" stopOpacity="0.92" />
        </linearGradient>
        <linearGradient id="import-dropzone-icon-arrow" x1="60" y1="32" x2="60" y2="73" gradientUnits="userSpaceOnUse">
          <stop stopColor="#76A9FF" />
          <stop offset="0.55" stopColor="#4B73F3" />
          <stop offset="1" stopColor="#3458DC" />
        </linearGradient>
        <linearGradient id="import-dropzone-icon-base" x1="60" y1="78" x2="60" y2="90" gradientUnits="userSpaceOnUse">
          <stop stopColor="#7390E0" stopOpacity="0.22" />
          <stop offset="1" stopColor="#4E6CC6" stopOpacity="0.08" />
        </linearGradient>
      </defs>
    </svg>
  );
}

export function ImportDropzoneLottieIcon({
  disabled = false,
  emphasis,
}: ImportDropzoneLottieIconProps): JSX.Element {
  const containerRef = useRef<HTMLSpanElement | null>(null);
  const animationRef = useRef<AnimationItem | null>(null);
  const [animationReady, setAnimationReady] = useState(false);
  const prefersReducedMotion = usePrefersReducedMotion();

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    let disposed = false;
    let domLoadedCleanup: (() => void) | undefined;

    const markReady = (): void => {
      if (disposed) {
        return;
      }

      setAnimationReady(true);
    };

    void (async () => {
      // @ts-expect-error lottie-web does not publish typings for the ESM light player bundle.
      const lottieModule = await import("lottie-web/build/player/esm/lottie_light.min.js");
      const lottie = (("default" in lottieModule ? lottieModule.default : lottieModule) ?? null) as LottiePlayer | null;
      if (disposed) {
        return;
      }

      if (!lottie || typeof lottie.loadAnimation !== "function") {
        return;
      }

      const animation = lottie.loadAnimation({
        container,
        renderer: "svg",
        loop: true,
        autoplay: false,
        animationData,
        rendererSettings: {
          preserveAspectRatio: "xMidYMid meet",
          progressiveLoad: true,
        },
      });

      animationRef.current = animation;
      domLoadedCleanup = animation.addEventListener("DOMLoaded", () => {
        markReady();
      });
      window.requestAnimationFrame(() => {
        if (!disposed && container.querySelector("svg")) {
          markReady();
        }
      });
    })().catch(() => {
      if (!disposed) {
        animationRef.current = null;
        setAnimationReady(false);
      }
    });

    return () => {
      disposed = true;
      domLoadedCleanup?.();
      setAnimationReady(false);
      animationRef.current?.destroy();
      animationRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!animationReady) {
      return;
    }

    const animation = animationRef.current;
    if (!animation) {
      return;
    }

    if (disabled || prefersReducedMotion) {
      animation.loop = false;
      animation.stop();
      animation.goToAndStop(STATIC_FRAME, true);
      return;
    }

    animation.loop = true;
    animation.setSpeed(emphasis === "drag" ? 1.08 : 1);
    animation.play();
  }, [animationReady, disabled, emphasis, prefersReducedMotion]);

  return (
    <span className={`import-console-dropzone__icon-lottie${animationReady ? " is-ready" : ""}`} aria-hidden="true">
      <span ref={containerRef} className="import-console-dropzone__icon-lottie-canvas" />
      {!animationReady ? <ImportDropzoneStaticIcon /> : null}
    </span>
  );
}
