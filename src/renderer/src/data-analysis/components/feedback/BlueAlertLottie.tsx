import { FeedbackLottieIcon, type FeedbackLottieVariant } from "./FeedbackLottieIcon";

type BlueAlertLottieProps = {
  variant?: FeedbackLottieVariant;
  className?: string;
};

export function BlueAlertLottie({
  variant = "toast",
  className
}: BlueAlertLottieProps): JSX.Element {
  return (
    <FeedbackLottieIcon
      classBase="ui-blue-alert-lottie"
      variant={variant}
      className={className}
      fallback={
        <svg className="ui-blue-alert-lottie__fallback" viewBox="0 0 24 24" aria-hidden="true">
          <circle cx="12" cy="12" r="8.2" />
          <path d="M12 10.2v5" />
          <path d="M12 7.3h.01" />
        </svg>
      }
    />
  );
}
