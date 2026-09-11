import { FeedbackLottieIcon, type FeedbackLottieVariant } from "./FeedbackLottieIcon";

type InsiderLoadingLottieProps = {
  variant?: FeedbackLottieVariant;
  className?: string;
};

export function InsiderLoadingLottie({
  variant = "toast",
  className
}: InsiderLoadingLottieProps): JSX.Element {
  return (
    <FeedbackLottieIcon
      classBase="ui-insider-loading-lottie"
      variant={variant}
      className={className}
      fallback={
        <svg className="ui-insider-loading-lottie__fallback" viewBox="0 0 36 36" aria-hidden="true">
          <circle className="ui-insider-loading-lottie__fallback-track" cx="18" cy="18" r="13.5" />
          <circle className="ui-insider-loading-lottie__fallback-indicator" cx="18" cy="18" r="13.5" />
        </svg>
      }
    />
  );
}
