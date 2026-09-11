import { useCallback, useEffect, useState } from "react";
import { fetchBackendHealth, type BackendHealth } from "../../../services/http/system";
import { toErrorMessage } from "../../shared/errors";

interface CleaningRuntimeHealthState {
  cleaningRuntimeHealth: BackendHealth | null;
  cleaningRuntimeHealthError: string;
  setCleaningRuntimeHealthError: (message: string) => void;
  syncCleaningRuntimeHealth: () => Promise<BackendHealth>;
}

export function useCleaningRuntimeHealth(active: boolean): CleaningRuntimeHealthState {
  const [cleaningRuntimeHealth, setCleaningRuntimeHealth] = useState<BackendHealth | null>(null);
  const [cleaningRuntimeHealthError, setCleaningRuntimeHealthError] = useState("");

  const syncCleaningRuntimeHealth = useCallback(async (): Promise<BackendHealth> => {
    const health = await fetchBackendHealth();
    setCleaningRuntimeHealth(health);
    setCleaningRuntimeHealthError("");
    return health;
  }, []);

  useEffect(() => {
    if (!active) {
      return undefined;
    }
    let canceled = false;
    const sync = async (): Promise<void> => {
      try {
        const health = await fetchBackendHealth();
        if (!canceled) {
          setCleaningRuntimeHealth(health);
          setCleaningRuntimeHealthError("");
        }
      } catch (healthError) {
        if (!canceled) {
          setCleaningRuntimeHealthError(toErrorMessage(healthError));
        }
      }
    };
    void sync();
    const timer = window.setInterval(() => {
      void sync();
    }, 15000);
    return () => {
      canceled = true;
      window.clearInterval(timer);
    };
  }, [active]);

  return {
    cleaningRuntimeHealth,
    cleaningRuntimeHealthError,
    setCleaningRuntimeHealthError,
    syncCleaningRuntimeHealth
  };
}
