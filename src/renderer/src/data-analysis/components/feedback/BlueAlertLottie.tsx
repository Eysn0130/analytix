import { Info } from "../../../design/AnalytixUiIcons";
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
        <Info className="ui-blue-alert-lottie__fallback" aria-hidden="true" />
      }
    />
  );
}
