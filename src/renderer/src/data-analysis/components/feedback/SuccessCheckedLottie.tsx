import { CheckCircle2 } from "../../../design/AnalytixUiIcons";
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
        <CheckCircle2 className="ui-success-check-lottie__fallback" aria-hidden="true" />
      }
    />
  );
}
