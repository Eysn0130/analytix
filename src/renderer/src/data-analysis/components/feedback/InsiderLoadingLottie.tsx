import { Loader2 } from "../../../design/AnalytixUiIcons";
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
        <Loader2 className="ui-insider-loading-lottie__fallback" aria-hidden="true" />
      }
    />
  );
}
