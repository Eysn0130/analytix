import { createContext, ReactNode, useContext, useEffect, useMemo, useReducer } from "react";
import { ThemeMode, getStoredThemeMode, resolveTheme, storeThemeMode } from "../theme/theme";
import { ThemeName } from "../theme/tokens";
import type { SafeWsEventEnvelope, WsConnectionStatus } from "../services/ws/types";
import { DEFAULT_PRODUCT_EDITION, type ProductEdition } from "../types/product-edition";

export type BackendHealthStatus = "unknown" | "ok" | "degraded" | "down";
export type ProductAuthorizationSource = "none" | "hub" | "offline" | "bypass";

export interface AppState {
  themeMode: ThemeMode;
  resolvedTheme: ThemeName;
  session: {
    activeCaseId: string;
    productEdition: ProductEdition;
    productEditionCanSwitch: boolean;
    productAuthorizationChecked: boolean;
    productAuthorizationSource: ProductAuthorizationSource;
    accountName: string;
    accountEmail: string;
  };
  runtime: {
    backendHealth: BackendHealthStatus;
    wsStatus: WsConnectionStatus;
    wsEventName: string;
    lastSyncAt: string;
    lastWsEvent: SafeWsEventEnvelope | null;
  };
}

type AppAction =
  | { type: "theme_mode_changed"; payload: ThemeMode }
  | { type: "resolved_theme_changed"; payload: ThemeName }
  | { type: "active_case_changed"; payload: string }
  | { type: "product_edition_changed"; payload: ProductEdition }
  | {
      type: "authorized_product_edition_changed";
      payload: {
        edition: ProductEdition;
        canSwitch: boolean;
        source: ProductAuthorizationSource;
        accountName?: string;
        accountEmail?: string;
      };
    }
  | { type: "product_authorization_cleared" }
  | { type: "backend_health_changed"; payload: BackendHealthStatus }
  | { type: "ws_status_changed"; payload: WsConnectionStatus }
  | { type: "ws_event_received"; payload: SafeWsEventEnvelope };

export function applySafeWsEventToRuntimeState(
  runtime: AppState["runtime"],
  event: SafeWsEventEnvelope
): AppState["runtime"] {
  return {
    ...runtime,
    wsEventName: event.event,
    lastSyncAt: event.timestamp,
    lastWsEvent: event
  };
}

function getInitialCaseId(): string {
  return "";
}

function detectPrefersDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function createInitialState(): AppState {
  const themeMode = getStoredThemeMode();
  const resolvedTheme = resolveTheme(themeMode, detectPrefersDark());
  return {
    themeMode,
    resolvedTheme,
    session: {
      activeCaseId: getInitialCaseId(),
      productEdition: DEFAULT_PRODUCT_EDITION,
      productEditionCanSwitch: false,
      productAuthorizationChecked: false,
      productAuthorizationSource: "none",
      accountName: "",
      accountEmail: ""
    },
    runtime: {
      backendHealth: "unknown",
      wsStatus: "idle",
      wsEventName: "",
      lastSyncAt: "",
      lastWsEvent: null
    }
  };
}

function reducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case "theme_mode_changed":
      return {
        ...state,
        themeMode: action.payload
      };
    case "resolved_theme_changed":
      return {
        ...state,
        resolvedTheme: action.payload
      };
    case "active_case_changed":
      return {
        ...state,
        session: {
          ...state.session,
          activeCaseId: action.payload
        }
      };
    case "product_edition_changed":
      return {
        ...state,
        session: {
          ...state.session,
          productEdition: action.payload
        }
      };
    case "authorized_product_edition_changed": {
      const shouldKeepSwitchableEdition =
        state.session.productAuthorizationSource === action.payload.source &&
        state.session.productEditionCanSwitch &&
        action.payload.canSwitch;
      return {
        ...state,
        session: {
          ...state.session,
          productEdition: shouldKeepSwitchableEdition ? state.session.productEdition : action.payload.edition,
          productEditionCanSwitch: action.payload.canSwitch,
          productAuthorizationChecked: true,
          productAuthorizationSource: action.payload.source,
          accountName: action.payload.accountName ?? state.session.accountName,
          accountEmail: action.payload.accountEmail ?? state.session.accountEmail
        }
      };
    }
    case "product_authorization_cleared":
      return {
        ...state,
        session: {
          ...state.session,
          productEdition: DEFAULT_PRODUCT_EDITION,
          productEditionCanSwitch: false,
          productAuthorizationChecked: true,
          productAuthorizationSource: "none",
          accountName: "",
          accountEmail: ""
        }
      };
    case "backend_health_changed":
      return {
        ...state,
        runtime: {
          ...state.runtime,
          backendHealth: action.payload,
          lastSyncAt: new Date().toISOString()
        }
      };
    case "ws_status_changed":
      return {
        ...state,
        runtime: {
          ...state.runtime,
          wsStatus: action.payload,
          lastSyncAt: new Date().toISOString()
        }
      };
    case "ws_event_received":
      return {
        ...state,
        runtime: applySafeWsEventToRuntimeState(state.runtime, action.payload)
      };
    default:
      return state;
  }
}

interface AppStoreValue {
  state: AppState;
  actions: {
    setThemeMode: (mode: ThemeMode) => void;
    setActiveCaseId: (caseId: string) => void;
    setProductEdition: (edition: ProductEdition) => void;
    setAuthorizedProductEdition: (
      edition: ProductEdition,
      canSwitch: boolean,
      source?: ProductAuthorizationSource,
      account?: { name?: string; email?: string }
    ) => void;
    clearProductAuthorization: () => void;
    setBackendHealth: (status: BackendHealthStatus) => void;
    setWsStatus: (status: WsConnectionStatus) => void;
    setWsEvent: (event: SafeWsEventEnvelope) => void;
  };
}

const AppStoreContext = createContext<AppStoreValue | null>(null);

export function AppStoreProvider({ children }: { children: ReactNode }): JSX.Element {
  const [state, dispatch] = useReducer(reducer, undefined, createInitialState);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");

    const updateResolved = () => {
      dispatch({
        type: "resolved_theme_changed",
        payload: resolveTheme(state.themeMode, media.matches)
      });
    };

    updateResolved();
    media.addEventListener("change", updateResolved);
    return () => media.removeEventListener("change", updateResolved);
  }, [state.themeMode]);

  useEffect(() => {
    storeThemeMode(state.themeMode);
  }, [state.themeMode]);

  const actions = useMemo<AppStoreValue["actions"]>(
    () => ({
      setThemeMode: (mode) => dispatch({ type: "theme_mode_changed", payload: mode }),
      setActiveCaseId: (caseId) => dispatch({ type: "active_case_changed", payload: caseId.trim() }),
      setProductEdition: (edition) => dispatch({ type: "product_edition_changed", payload: edition }),
      setAuthorizedProductEdition: (edition, canSwitch, source = "hub", account) =>
        dispatch({
          type: "authorized_product_edition_changed",
          payload: {
            edition,
            canSwitch,
            source,
            accountName: account?.name,
            accountEmail: account?.email
          }
        }),
      clearProductAuthorization: () => dispatch({ type: "product_authorization_cleared" }),
      setBackendHealth: (status) => dispatch({ type: "backend_health_changed", payload: status }),
      setWsStatus: (status) => dispatch({ type: "ws_status_changed", payload: status }),
      setWsEvent: (event) => dispatch({ type: "ws_event_received", payload: event })
    }),
    []
  );

  const value = useMemo<AppStoreValue>(
    () => ({
      state,
      actions
    }),
    [actions, state]
  );

  return <AppStoreContext.Provider value={value}>{children}</AppStoreContext.Provider>;
}

export function useAppStore(): AppStoreValue {
  const value = useContext(AppStoreContext);
  if (!value) {
    throw new Error("useAppStore must be used inside AppStoreProvider");
  }
  return value;
}
