import { cloneElement, isValidElement, type ReactElement } from "react";

export type FeedbackLottieVariant = "toast" | "panel";

type FeedbackLottieFallbackElement = ReactElement<{ className?: string }>;

interface FeedbackLottieIconProps {
  classBase: string;
  fallback: FeedbackLottieFallbackElement;
  variant?: FeedbackLottieVariant;
  className?: string;
}

export function FeedbackLottieIcon({
  classBase,
  fallback,
  variant = "toast",
  className
}: FeedbackLottieIconProps): JSX.Element {
  const fallbackElement = isValidElement<{ className?: string }>(fallback) ? fallback : null;
  const fallbackNode = fallbackElement
    ? cloneElement(fallbackElement, {
        className: `${fallbackElement.props.className ? `${fallbackElement.props.className} ` : ""}ui-feedback-lottie__fallback`
      })
    : fallback;

  return (
    <span
      className={`ui-feedback-lottie ${classBase} ui-feedback-lottie--${variant} ${classBase}--${variant}${className ? ` ${className}` : ""}`}
      aria-hidden="true"
    >
      {fallbackNode}
    </span>
  );
}
