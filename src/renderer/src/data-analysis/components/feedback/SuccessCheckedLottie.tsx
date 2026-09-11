import { FeedbackLottieIcon, type FeedbackLottieVariant } from "./FeedbackLottieIcon";

type SuccessCheckedLottieProps = {
  variant?: FeedbackLottieVariant;
  className?: string;
};

export function SuccessCheckedLottie({
  variant = "toast",
  className
}: SuccessCheckedLottieProps): JSX.Element {
  return (
    <FeedbackLottieIcon
      classBase="ui-success-check-lottie"
      variant={variant}
      className={className}
      fallback={
        <svg className="ui-success-check-lottie__fallback" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="9" />
        <path d="M8 12.4 10.7 15.1 16.4 9.4" />
        </svg>
      }
    />
  );
}
