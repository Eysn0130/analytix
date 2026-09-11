import { FeedbackLottieIcon, type FeedbackLottieVariant } from "./FeedbackLottieIcon";

type FailAlertLottieProps = {
  variant?: FeedbackLottieVariant;
  className?: string;
};

export function FailAlertLottie({ variant = "toast", className }: FailAlertLottieProps): JSX.Element {
  return (
    <FeedbackLottieIcon
      classBase="ui-fail-alert-lottie"
      variant={variant}
      className={className}
      fallback={
        <svg className="ui-fail-alert-lottie__fallback" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="8.6" />
        <path d="M8.9 8.9 15.1 15.1" />
        <path d="M15.1 8.9 8.9 15.1" />
        </svg>
      }
    />
  );
}
