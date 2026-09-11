import type { CleaningAvailability } from "./types";

export interface CleaningHeroActionViewModel {
  disabled: boolean;
  loading: boolean;
  title: string | undefined;
}

export interface CleaningHeroActionsViewModel {
  start: CleaningHeroActionViewModel;
  reclean: CleaningHeroActionViewModel;
  exportRaw: CleaningHeroActionViewModel;
  exportCleaned: CleaningHeroActionViewModel;
}

export function buildCleaningHeroActionsViewModel({
  cleanedExportButtonTitle,
  cleanedExportReady,
  cleaningAvailability,
  cleaningRuntimeUnavailable,
  cleaningRuntimeUnavailableMessage,
  currentJobActive,
  exportBusy,
  exportCleanedWorking,
  exportRawWorking,
  recleanActionWorking,
  runActionWorking
}: {
  cleanedExportButtonTitle: string;
  cleanedExportReady: boolean;
  cleaningAvailability: CleaningAvailability;
  cleaningRuntimeUnavailable: boolean;
  cleaningRuntimeUnavailableMessage: string;
  currentJobActive: boolean;
  exportBusy: boolean;
  exportCleanedWorking: boolean;
  exportRawWorking: boolean;
  recleanActionWorking: boolean;
  runActionWorking: boolean;
}): CleaningHeroActionsViewModel {
  const cleaningActionBlocked = runActionWorking || recleanActionWorking || currentJobActive || cleaningRuntimeUnavailable;
  const runtimeTitle = cleaningRuntimeUnavailableMessage || undefined;
  return {
    start: {
      disabled: cleaningActionBlocked,
      loading: runActionWorking,
      title: runtimeTitle
    },
    reclean: {
      disabled: cleaningActionBlocked || cleaningAvailability.hasTransactionData !== true,
      loading: recleanActionWorking,
      title: runtimeTitle
    },
    exportRaw: {
      disabled: exportBusy,
      loading: exportRawWorking,
      title: undefined
    },
    exportCleaned: {
      disabled: exportBusy || currentJobActive || !cleanedExportReady,
      loading: exportCleanedWorking,
      title: cleanedExportButtonTitle
    }
  };
}
