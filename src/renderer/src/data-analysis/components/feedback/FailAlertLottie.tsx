import { XCircle } from "../../../design/AnalytixUiIcons";
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
        <XCircle className="ui-fail-alert-lottie__fallback" aria-hidden="true" />
      }
    />
  );
}
