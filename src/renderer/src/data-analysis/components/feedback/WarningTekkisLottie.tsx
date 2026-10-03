import { AlertTriangle } from "../../../design/AnalytixUiIcons";
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
        <AlertTriangle className="ui-warning-tekkis-lottie__fallback" aria-hidden="true" />
      }
    />
  );
}
