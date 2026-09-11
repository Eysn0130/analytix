import { FeedbackLottieIcon, type FeedbackLottieVariant } from "./FeedbackLottieIcon";

type WarningTekkisLottieProps = {
  variant?: FeedbackLottieVariant;
  className?: string;
};

export function WarningTekkisLottie({
  variant = "toast",
  className
}: WarningTekkisLottieProps): JSX.Element {
  return (
    <FeedbackLottieIcon
      classBase="ui-warning-tekkis-lottie"
      variant={variant}
      className={className}
      fallback={
        <svg className="ui-warning-tekkis-lottie__fallback" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M12 4.8 20 18.5H4Z" />
          <path d="M12 9.2v4.9" />
          <path d="M12 17.2h.01" />
        </svg>
      }
    />
  );
}
