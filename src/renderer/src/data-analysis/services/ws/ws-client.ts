import { WsConnectionStatus, WsEventEnvelope } from "./types";

function buildWsUrl(baseUrl: string, options: { caseId?: string } = {}): string {
  const browserHref = typeof window === "undefined" ? "" : String(window.location?.href || "");
  const url = new URL(baseUrl, browserHref || "http://localhost");
  const caseId = (options.caseId ?? "").trim();
  if (caseId) {
    url.searchParams.set("case_id", caseId);
  }
  return url.toString();
}

interface WsClientOptions {
  baseUrl: string;
  caseId?: string;
  isConnectionAuthorized?: () => boolean;
  reconnectMinMs?: number;
  reconnectMaxMs?: number;
}

export class WsEventClient {
  private readonly baseUrl: string;
  private readonly isConnectionAuthorized: () => boolean;
  private readonly reconnectMinMs: number;
  private readonly reconnectMaxMs: number;

  private caseId = "";
  private socket: WebSocket | null = null;
  private reconnectTimer: number | null = null;
  private reconnectAttempt = 0;
  private shouldReconnect = true;

  private readonly statusListeners = new Set<(status: WsConnectionStatus) => void>();
  private readonly eventListeners = new Set<(event: WsEventEnvelope) => void>();

  constructor(options: WsClientOptions) {
    this.baseUrl = options.baseUrl;
    this.caseId = (options.caseId ?? "").trim();
    this.isConnectionAuthorized = options.isConnectionAuthorized ?? (() => true);
    this.reconnectMinMs = options.reconnectMinMs ?? 500;
    this.reconnectMaxMs = options.reconnectMaxMs ?? 5000;
  }

  onStatus(listener: (status: WsConnectionStatus) => void): () => void {
    this.statusListeners.add(listener);
    return () => this.statusListeners.delete(listener);
  }

  onEvent(listener: (event: WsEventEnvelope) => void): () => void {
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }

  connect(): void {
    if (!this.isAuthorized()) {
      this.shouldReconnect = false;
      this.emitStatus("closed");
      return;
    }
    this.shouldReconnect = true;
    this.openSocket();
  }

  disconnect(): void {
    this.shouldReconnect = false;
    this.clearReconnectTimer();
    if (this.socket) {
      this.socket.close(1000, "manual disconnect");
      this.socket = null;
    }
    this.emitStatus("closed");
  }

  setCaseId(caseId: string): void {
    const normalized = (caseId ?? "").trim();
    if (normalized === this.caseId) {
      return;
    }
    this.caseId = normalized;

    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return;
    }

    this.socket.close(1000, "case changed");
  }

  send(payload: Record<string, unknown>): boolean {
    if (!this.isAuthorized() || !this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return false;
    }
    try {
      this.socket.send(JSON.stringify(payload ?? {}));
      return true;
    } catch {
      return false;
    }
  }

  private openSocket(): void {
    if (!this.isAuthorized()) {
      this.shouldReconnect = false;
      this.emitStatus("closed");
      return;
    }
    if (this.socket && this.socket.readyState <= WebSocket.OPEN) {
      return;
    }

    this.clearReconnectTimer();
    this.emitStatus("connecting");

    const url = buildWsUrl(this.baseUrl, {
      caseId: this.caseId || undefined,
    });
    const socket = new WebSocket(url);
    this.socket = socket;

    socket.addEventListener("open", () => {
      if (!this.isCurrentSocketAuthorized(socket)) {
        this.closeRevokedSocket(socket);
        return;
      }
      this.reconnectAttempt = 0;
      this.emitStatus("open");
    });

    socket.addEventListener("message", (event) => {
      if (!this.isCurrentSocketAuthorized(socket)) {
        this.closeRevokedSocket(socket);
        return;
      }
      try {
        const payload = JSON.parse(String(event.data)) as WsEventEnvelope;
        this.eventListeners.forEach((listener) => listener(payload));
      } catch {
        // Ignore malformed payloads in scaffold stage.
      }
    });

    socket.addEventListener("error", () => {
      if (!this.isCurrentSocketAuthorized(socket)) {
        this.closeRevokedSocket(socket);
        return;
      }
      this.emitStatus("error");
    });

    socket.addEventListener("close", () => {
      if (this.socket !== socket) {
        return;
      }
      this.emitStatus("closed");
      this.socket = null;
      if (!this.shouldReconnect || !this.isAuthorized()) {
        return;
      }
      this.scheduleReconnect();
    });
  }

  private scheduleReconnect(): void {
    this.reconnectAttempt += 1;
    const delay = Math.min(
      this.reconnectMaxMs,
      this.reconnectMinMs * Math.pow(2, Math.max(0, this.reconnectAttempt - 1))
    );

    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.shouldReconnect || !this.isAuthorized()) {
        return;
      }
      this.openSocket();
    }, delay);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private emitStatus(status: WsConnectionStatus): void {
    this.statusListeners.forEach((listener) => listener(status));
  }

  private isAuthorized(): boolean {
    try {
      return this.isConnectionAuthorized();
    } catch {
      return false;
    }
  }

  private isCurrentSocketAuthorized(socket: WebSocket): boolean {
    return this.socket === socket && this.isAuthorized();
  }

  private closeRevokedSocket(socket: WebSocket): void {
    this.shouldReconnect = false;
    if (this.socket === socket) {
      this.socket = null;
    }
    try {
      socket.close(1008, "runtime authority revoked");
    } catch {
      // The transport is already unusable; keep it detached from the client.
    }
    this.emitStatus("closed");
  }
}
